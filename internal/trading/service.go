package trading

import (
	"context"
	"fmt"
	"strings"

	"github.com/Aidin1998/pincex_unified/internal/bookkeeper"
	"github.com/Aidin1998/pincex_unified/internal/trading/engine"
	"github.com/Aidin1998/pincex_unified/pkg/models"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// TradingService defines trading operations for dependency injection
type TradingService interface {
	Start() error
	Stop() error
	PlaceOrder(ctx context.Context, order *models.Order) (*models.Order, error)
	CancelOrder(ctx context.Context, orderID string) error
	GetOrder(orderID string) (*models.Order, error)
	GetOrders(userID, symbol, status string, limit, offset string) ([]*models.Order, int64, error)
	GetOrderBook(symbol string, depth int) (*models.OrderBookSnapshot, error)
	GetTradingPairs() ([]*models.TradingPair, error)
	GetTradingPair(symbol string) (*models.TradingPair, error)
	CreateTradingPair(pair *models.TradingPair) (*models.TradingPair, error)
	UpdateTradingPair(pair *models.TradingPair) (*models.TradingPair, error)
	ListOrders(userID string, filter *models.OrderFilter) ([]*models.Order, error)
}

// Service implements TradingService
type Service struct {
	logger        *zap.Logger
	db            *gorm.DB
	engine        *engine.Engine
	bookkeeperSvc bookkeeper.BookkeeperService
}

// NewService creates a new trading service
func NewService(logger *zap.Logger, db *gorm.DB, bookkeeperSvc bookkeeper.BookkeeperService) (TradingService, error) {
	// Create trading engine
	tradingEngine, err := engine.NewEngine(logger, db)
	if err != nil {
		return nil, fmt.Errorf("failed to create trading engine: %w", err)
	}

	// Create service
	svc := &Service{
		logger:        logger,
		db:            db,
		engine:        tradingEngine,
		bookkeeperSvc: bookkeeperSvc,
	}

	return svc, nil
}

// Start starts the trading service
func (s *Service) Start() error {
	// Start trading engine
	if err := s.engine.Start(); err != nil {
		return fmt.Errorf("failed to start trading engine: %w", err)
	}

	s.logger.Info("Trading service started")
	return nil
}

// Stop stops the trading service
func (s *Service) Stop() error {
	// Stop trading engine
	if err := s.engine.Stop(); err != nil {
		return fmt.Errorf("failed to stop trading engine: %w", err)
	}

	s.logger.Info("Trading service stopped")
	return nil
}

// PlaceOrder places an order
func (s *Service) PlaceOrder(ctx context.Context, order *models.Order) (*models.Order, error) {
	// Lock funds for buy orders
	if order.Side == "buy" {
		// Calculate required funds
		requiredFunds := order.Price * order.Quantity
		if order.Type == "market" {
			// Add 10% buffer for market orders
			requiredFunds *= 1.1
		}

		// Get quote currency
		quoteCurrency := order.Symbol[3:]

		// Lock funds
		if err := s.bookkeeperSvc.LockFunds(ctx, order.UserID.String(), quoteCurrency, requiredFunds); err != nil {
			return nil, fmt.Errorf("failed to lock funds: %w", err)
		}
	} else if order.Side == "sell" {
		// Get base currency
		baseCurrency := order.Symbol[:3]

		// Lock funds
		if err := s.bookkeeperSvc.LockFunds(ctx, order.UserID.String(), baseCurrency, order.Quantity); err != nil {
			return nil, fmt.Errorf("failed to lock funds: %w", err)
		}
	}

	// Place order in trading engine
	placedOrder, err := s.engine.PlaceOrder(ctx, order)
	if err != nil {
		// Unlock funds if order placement fails
		if order.Side == "buy" {
			quoteCurrency := order.Symbol[3:]
			requiredFunds := order.Price * order.Quantity
			if order.Type == "market" {
				requiredFunds *= 1.1
			}
			if unlockErr := s.bookkeeperSvc.UnlockFunds(ctx, order.UserID.String(), quoteCurrency, requiredFunds); unlockErr != nil {
				s.logger.Error("Failed to unlock funds", zap.Error(unlockErr))
			}
		} else if order.Side == "sell" {
			baseCurrency := order.Symbol[:3]
			if unlockErr := s.bookkeeperSvc.UnlockFunds(ctx, order.UserID.String(), baseCurrency, order.Quantity); unlockErr != nil {
				s.logger.Error("Failed to unlock funds", zap.Error(unlockErr))
			}
		}
		return nil, fmt.Errorf("failed to place order: %w", err)
	}

	return placedOrder, nil
}

// CancelOrder cancels an order
func (s *Service) CancelOrder(ctx context.Context, orderID string) error {
	// Get order
	order, err := s.engine.GetOrder(orderID)
	if err != nil {
		return fmt.Errorf("failed to get order: %w", err)
	}

	// Cancel order in trading engine
	err = s.engine.CancelOrder(ctx, orderID)
	if err != nil {
		// Ignore errors for orders that cannot be canceled (e.g., already filled)
		if strings.Contains(err.Error(), "order cannot be canceled") {
			return nil
		}
		return fmt.Errorf("failed to cancel order: %w", err)
	}

	// Unlock funds
	if order.Side == "buy" {
		// Calculate remaining funds
		remainingFunds := order.Price * order.Quantity
		if order.Type == "market" {
			remainingFunds *= 1.1
		}

		// Get quote currency
		quoteCurrency := order.Symbol[3:]

		// Unlock funds
		if err := s.bookkeeperSvc.UnlockFunds(ctx, order.UserID.String(), quoteCurrency, remainingFunds); err != nil {
			s.logger.Error("Failed to unlock funds", zap.Error(err))
		}
	} else if order.Side == "sell" {
		// Get base currency
		baseCurrency := order.Symbol[:3]

		// Unlock funds
		if err := s.bookkeeperSvc.UnlockFunds(ctx, order.UserID.String(), baseCurrency, order.Quantity); err != nil {
			s.logger.Error("Failed to unlock funds", zap.Error(err))
		}
	}

	return nil
}

// GetOrder gets an order
func (s *Service) GetOrder(orderID string) (*models.Order, error) {
	return s.engine.GetOrder(orderID)
}

// GetOrders gets orders for a user
func (s *Service) GetOrders(userID, symbol, status string, limit, offset string) ([]*models.Order, int64, error) {
	// Parse limit and offset
	limitInt := 20
	offsetInt := 0
	if limit != "" {
		if _, err := fmt.Sscanf(limit, "%d", &limitInt); err != nil {
			return nil, 0, fmt.Errorf("invalid limit: %s", limit)
		}
	}
	if offset != "" {
		if _, err := fmt.Sscanf(offset, "%d", &offsetInt); err != nil {
			return nil, 0, fmt.Errorf("invalid offset: %s", offset)
		}
	}

	return s.engine.GetOrders(userID, symbol, status, limitInt, offsetInt)
}

// GetOrderBook gets the order book for a symbol
func (s *Service) GetOrderBook(symbol string, depth int) (*models.OrderBookSnapshot, error) {
	return s.engine.GetOrderBook(symbol, depth)
}

// GetTradingPairs gets all trading pairs
func (s *Service) GetTradingPairs() ([]*models.TradingPair, error) {
	return s.engine.GetTradingPairs()
}

// GetTradingPair gets a trading pair
func (s *Service) GetTradingPair(symbol string) (*models.TradingPair, error) {
	return s.engine.GetTradingPair(symbol)
}

// CreateTradingPair creates a trading pair
func (s *Service) CreateTradingPair(pair *models.TradingPair) (*models.TradingPair, error) {
	return s.engine.CreateTradingPair(pair)
}

// UpdateTradingPair updates a trading pair
func (s *Service) UpdateTradingPair(pair *models.TradingPair) (*models.TradingPair, error) {
	return s.engine.UpdateTradingPair(pair)
}

// ListOrders lists orders for a user with optional filters
func (s *Service) ListOrders(userID string, filter *models.OrderFilter) ([]*models.Order, error) {
	// Use filter fields to call GetOrders
	symbol := ""
	status := ""
	if filter != nil {
		symbol = filter.Symbol
		status = filter.Status
	}
	orders, _, err := s.GetOrders(userID, symbol, status, "20", "0")
	return orders, err
}
