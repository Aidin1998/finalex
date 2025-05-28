package trading

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/bookkeeper"
	"github.com/Aidin1998/pincex_unified/internal/trading/engine"
	eventjournal "github.com/Aidin1998/pincex_unified/internal/trading/eventjournal"
	model2 "github.com/Aidin1998/pincex_unified/internal/trading/model"
	orderbook "github.com/Aidin1998/pincex_unified/internal/trading/orderbook"
	"github.com/Aidin1998/pincex_unified/pkg/metrics"
	"github.com/Aidin1998/pincex_unified/pkg/models"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
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
	engine        *engine.MatchingEngine
	bookkeeperSvc bookkeeper.BookkeeperService
}

// NewService creates a new trading service
func NewService(logger *zap.Logger, db *gorm.DB, bookkeeperSvc bookkeeper.BookkeeperService) (TradingService, error) {
	// Create trading engine
	// TODO: Provide real orderRepo, tradeRepo, config, eventJournal as needed
	var orderRepo model2.Repository = nil             // Replace with actual repo
	var tradeRepo engine.TradeRepository = nil        // Replace with actual repo
	var config *engine.Config = nil                   // Replace with actual config
	var eventJournal *eventjournal.EventJournal = nil // Replace with actual journal if needed
	tradingEngine := engine.NewMatchingEngine(orderRepo, tradeRepo, logger.Sugar(), config, eventJournal)

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
	s.logger.Info("Trading service started (engine Start is a no-op)")
	return nil
}

// Stop stops the trading service
func (s *Service) Stop() error {
	s.logger.Info("Trading service stopped (engine Stop is a no-op)")
	return nil
}

// Conversion helpers between models.Order and model.Order
func toModelOrder(o *models.Order) *model2.Order {
	if o == nil {
		return nil
	}
	return &model2.Order{
		ID:          o.ID,
		UserID:      o.UserID,
		Pair:        o.Symbol,
		Side:        strings.ToUpper(o.Side),
		Type:        strings.ToUpper(o.Type),
		Price:       decimal.NewFromFloat(o.Price),
		Quantity:    decimal.NewFromFloat(o.Quantity),
		TimeInForce: strings.ToUpper(o.TimeInForce),
		Status:      strings.ToUpper(o.Status),
		CreatedAt:   o.CreatedAt,
		UpdatedAt:   o.UpdatedAt,
	}
}

func toAPIOrder(o *model2.Order) *models.Order {
	if o == nil {
		return nil
	}
	return &models.Order{
		ID:          o.ID,
		UserID:      o.UserID,
		Symbol:      o.Pair,
		Side:        strings.ToLower(o.Side),
		Type:        strings.ToLower(o.Type),
		Price:       o.Price.InexactFloat64(),
		Quantity:    o.Quantity.InexactFloat64(),
		TimeInForce: strings.ToUpper(o.TimeInForce),
		Status:      strings.ToLower(o.Status),
		CreatedAt:   o.CreatedAt,
		UpdatedAt:   o.UpdatedAt,
	}
}

// PlaceOrder places an order
func (s *Service) PlaceOrder(ctx context.Context, order *models.Order) (*models.Order, error) {
	tracer := otel.Tracer("trading.Service")
	ctx, span := tracer.Start(ctx, "PlaceOrder", trace.WithAttributes(
		attribute.String("order.side", order.Side),
		attribute.String("order.type", order.Type),
		attribute.String("order.symbol", order.Symbol),
		attribute.String("order.user_id", order.UserID.String()),
	))
	defer span.End()
	start := time.Now()

	// Lock funds for buy orders
	if order.Side == "buy" {
		requiredFunds := decimal.NewFromFloat(order.Price).Mul(decimal.NewFromFloat(order.Quantity))
		if order.Type == "market" {
			requiredFunds = requiredFunds.Mul(decimal.NewFromFloat(1.1))
		}
		quoteCurrency := order.Symbol[3:]
		if err := s.bookkeeperSvc.LockFunds(ctx, order.UserID.String(), quoteCurrency, requiredFunds.InexactFloat64()); err != nil {
			return nil, fmt.Errorf("failed to lock funds: %w", err)
		}
	} else if order.Side == "sell" {
		baseCurrency := order.Symbol[:3]
		if err := s.bookkeeperSvc.LockFunds(ctx, order.UserID.String(), baseCurrency, order.Quantity); err != nil {
			return nil, fmt.Errorf("failed to lock funds: %w", err)
		}
	}

	// Place order in trading engine
	modelOrder := toModelOrder(order)
	placedOrder, _, _, err := s.engine.ProcessOrder(ctx, modelOrder, "api")
	if err != nil {
		// Unlock funds if order placement fails
		if order.Side == "buy" {
			quoteCurrency := order.Symbol[3:]
			requiredFunds := decimal.NewFromFloat(order.Price).Mul(decimal.NewFromFloat(order.Quantity))
			if order.Type == "market" {
				requiredFunds = requiredFunds.Mul(decimal.NewFromFloat(1.1))
			}
			if unlockErr := s.bookkeeperSvc.UnlockFunds(ctx, order.UserID.String(), quoteCurrency, requiredFunds.InexactFloat64()); unlockErr != nil {
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
	metrics.OrdersProcessed.WithLabelValues(order.Side).Inc()
	metrics.OrderLatency.Observe(time.Since(start).Seconds())
	return toAPIOrder(placedOrder), nil
}

// CancelOrder cancels an order
func (s *Service) CancelOrder(ctx context.Context, orderID string) error {
	order, err := engineGetOrder(s.engine, orderID)
	if err != nil {
		return fmt.Errorf("failed to get order: %w", err)
	}
	orderUUID, err := uuid.Parse(orderID)
	if err != nil {
		return fmt.Errorf("invalid order ID: %w", err)
	}
	cancelReq := &engine.CancelRequest{OrderID: orderUUID}
	err = s.engine.CancelOrder(cancelReq)
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
		remainingFunds := order.Price.Mul(order.Quantity)
		if order.Type == "market" {
			remainingFunds = remainingFunds.Mul(decimal.NewFromFloat(1.1))
		}

		// Get quote currency
		quoteCurrency := order.Pair[3:]

		// Unlock funds
		if err := s.bookkeeperSvc.UnlockFunds(ctx, order.UserID.String(), quoteCurrency, remainingFunds.InexactFloat64()); err != nil {
			s.logger.Error("Failed to unlock funds", zap.Error(err))
		}
	} else if order.Side == "sell" {
		// Get base currency
		baseCurrency := order.Pair[:3]

		// Unlock funds
		if err := s.bookkeeperSvc.UnlockFunds(ctx, order.UserID.String(), baseCurrency, order.Quantity.InexactFloat64()); err != nil {
			s.logger.Error("Failed to unlock funds", zap.Error(err))
		}
	}

	return nil
}

// GetOrder gets an order
func (s *Service) GetOrder(orderID string) (*models.Order, error) {
	order, err := engineGetOrder(s.engine, orderID)
	return toAPIOrder(order), err
}

// GetOrders gets orders for a user
func (s *Service) GetOrders(userID, symbol, status string, limit, offset string) ([]*models.Order, int64, error) {
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
	orders, total, err := engineGetOrders(s.engine, userID, symbol, status, limitInt, offsetInt)
	apiOrders := make([]*models.Order, 0, len(orders))
	for _, o := range orders {
		apiOrders = append(apiOrders, toAPIOrder(o))
	}
	return apiOrders, total, err
}

// GetOrderBook gets the order book for a symbol
func (s *Service) GetOrderBook(symbol string, depth int) (*models.OrderBookSnapshot, error) {
	ob := engineGetOrderBook(s.engine, symbol)
	if ob == nil {
		return nil, fmt.Errorf("order book not found")
	}
	bids, asks := ob.GetSnapshot(depth)
	apiBids := make([]models.OrderBookLevel, 0, len(bids))
	for _, b := range bids {
		if len(b) >= 2 {
			apiBids = append(apiBids, models.OrderBookLevel{
				Price:  parseFloat(b[0]),
				Volume: parseFloat(b[1]),
			})
		}
	}
	apiAsks := make([]models.OrderBookLevel, 0, len(asks))
	for _, a := range asks {
		if len(a) >= 2 {
			apiAsks = append(apiAsks, models.OrderBookLevel{
				Price:  parseFloat(a[0]),
				Volume: parseFloat(a[1]),
			})
		}
	}
	return &models.OrderBookSnapshot{
		Symbol:     symbol,
		Bids:       apiBids,
		Asks:       apiAsks,
		UpdateTime: time.Now(),
	}, nil
}

// parseFloat helper
func parseFloat(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

// Local engine adapter functions (not methods on MatchingEngine)
func engineGetOrder(engine *engine.MatchingEngine, orderID string) (*model2.Order, error) {
	return nil, fmt.Errorf("not implemented")
}
func engineGetOrders(engine *engine.MatchingEngine, userID, symbol, status string, limit, offset int) ([]*model2.Order, int64, error) {
	return nil, 0, fmt.Errorf("not implemented")
}
func engineGetOrderBook(engine *engine.MatchingEngine, symbol string) *orderbook.OrderBook {
	return nil
}

// Implement missing TradingService methods as stubs on *Service
func (s *Service) GetTradingPairs() ([]*models.TradingPair, error) {
	return nil, fmt.Errorf("not implemented")
}
func (s *Service) GetTradingPair(symbol string) (*models.TradingPair, error) {
	return nil, fmt.Errorf("not implemented")
}
func (s *Service) CreateTradingPair(pair *models.TradingPair) (*models.TradingPair, error) {
	return nil, fmt.Errorf("not implemented")
}
func (s *Service) UpdateTradingPair(pair *models.TradingPair) (*models.TradingPair, error) {
	return nil, fmt.Errorf("not implemented")
}

// Implement ListOrders for TradingService
func (s *Service) ListOrders(userID string, filter *models.OrderFilter) ([]*models.Order, error) {
	symbol := ""
	status := ""
	if filter != nil {
		symbol = filter.Symbol
		status = filter.Status
	}
	orders, _, err := s.GetOrders(userID, symbol, status, "20", "0")
	return orders, err
}
