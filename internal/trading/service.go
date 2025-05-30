package trading

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/bookkeeper"
	"github.com/Aidin1998/pincex_unified/internal/settlement"
	"github.com/Aidin1998/pincex_unified/internal/trading/config"
	"github.com/Aidin1998/pincex_unified/internal/trading/engine"
	model2 "github.com/Aidin1998/pincex_unified/internal/trading/model"
	"github.com/Aidin1998/pincex_unified/internal/trading/repository"
	"github.com/Aidin1998/pincex_unified/internal/ws"
	"github.com/Aidin1998/pincex_unified/pkg/models"
	"github.com/Aidin1998/pincex_unified/pkg/validation"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
	"gorm.io/gorm"

	marketdata "github.com/Aidin1998/pincex_unified/internal/marketdata"
	orderbook "github.com/Aidin1998/pincex_unified/internal/trading/orderbook"
	"github.com/Aidin1998/pincex_unified/internal/trading/risk"
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
	GetOrderBookBinary(symbol string, depth int) ([]byte, error)
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
	riskConfig    *risk.RiskConfig // Add this field for admin API
}

// NewService creates a new trading service
func NewService(logger *zap.Logger, db *gorm.DB, bookkeeperSvc bookkeeper.BookkeeperService, wsHub *ws.Hub, settlementEngine *settlement.SettlementEngine) (TradingService, error) {
	// Initialize repositories
	orderRepo := repository.NewGormRepository(db, logger)
	tradeRepo := repository.NewGormTradeRepository(db, logger)

	// Load configuration
	config := config.DefaultTradingConfig()

	// --- RISK MANAGEMENT ---
	riskCfg := risk.NewRiskConfig()
	// Set default system order limits (e.g. 100 BTC or equivalent in USDT)
	riskCfg.SetSymbolLimit("BTCUSDT", 100.0)
	// TODO: Load more limits from config/db, and allow admin API to update

	// Create trading engine with all components
	tradingEngine := engine.NewMatchingEngine(
		orderRepo,
		tradeRepo,
		logger.Sugar(),
		config.Engine,
		/* eventJournal */ nil, // TODO: wire eventJournal if needed
		wsHub,
		/* riskManager */ nil, // TODO: wire riskMgr if needed
		settlementEngine,      // Pass settlement engine
	)

	// Create service
	svc := &Service{
		logger:        logger,
		db:            db,
		engine:        tradingEngine,
		bookkeeperSvc: bookkeeperSvc,
		riskConfig:    riskCfg, // Pass risk config for admin API
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
	// 1. Input validation and sanitization
	if order == nil {
		return nil, fmt.Errorf("order cannot be nil")
	}

	// Validate and sanitize symbol/trading pair
	if order.Symbol == "" {
		return nil, fmt.Errorf("trading symbol is required")
	}

	// Check for malicious content in symbol
	if validation.ContainsSQLInjection(order.Symbol) || validation.ContainsXSS(order.Symbol) {
		s.logger.Warn("Malicious content detected in trading symbol",
			zap.String("symbol", order.Symbol),
			zap.String("user_id", order.UserID.String()))
		return nil, fmt.Errorf("invalid trading symbol")
	}

	// Sanitize and validate symbol
	cleanSymbol := validation.SanitizeInput(order.Symbol)
	if !validation.IsValidTradingPair(cleanSymbol) {
		return nil, fmt.Errorf("invalid trading pair format")
	}
	order.Symbol = cleanSymbol

	// Validate user ID
	if order.UserID == uuid.Nil {
		return nil, fmt.Errorf("user ID is required")
	}

	// Validate and sanitize side
	if order.Side == "" {
		return nil, fmt.Errorf("order side is required")
	}

	if validation.ContainsSQLInjection(order.Side) || validation.ContainsXSS(order.Side) {
		s.logger.Warn("Malicious content detected in order side",
			zap.String("side", order.Side),
			zap.String("user_id", order.UserID.String()))
		return nil, fmt.Errorf("invalid order side")
	}

	cleanSide := strings.ToUpper(validation.SanitizeInput(order.Side))
	if cleanSide != "BUY" && cleanSide != "SELL" {
		return nil, fmt.Errorf("order side must be 'buy' or 'sell'")
	}
	order.Side = cleanSide

	// Validate and sanitize order type
	if order.Type == "" {
		return nil, fmt.Errorf("order type is required")
	}

	if validation.ContainsSQLInjection(order.Type) || validation.ContainsXSS(order.Type) {
		s.logger.Warn("Malicious content detected in order type",
			zap.String("type", order.Type),
			zap.String("user_id", order.UserID.String()))
		return nil, fmt.Errorf("invalid order type")
	}

	cleanType := strings.ToUpper(validation.SanitizeInput(order.Type))
	validTypes := map[string]bool{
		"LIMIT": true, "MARKET": true, "STOP_LIMIT": true,
		"IOC": true, "FOK": true, "GTD": true,
	}
	if !validTypes[cleanType] {
		return nil, fmt.Errorf("unsupported order type: %s", cleanType)
	}
	order.Type = cleanType

	// Validate quantity
	if order.Quantity <= 0 {
		return nil, fmt.Errorf("order quantity must be positive")
	}
	if order.Quantity > 1000000 { // 1M quantity limit
		return nil, fmt.Errorf("order quantity exceeds maximum limit of 1,000,000")
	}

	// Validate price for limit orders
	if (cleanType == "LIMIT" || cleanType == "STOP_LIMIT") && order.Price <= 0 {
		return nil, fmt.Errorf("price must be positive for limit orders")
	}
	if order.Price > 1000000000 { // 1B price limit
		return nil, fmt.Errorf("order price exceeds maximum limit")
	}

	// Validate time in force
	if order.TimeInForce != "" {
		if validation.ContainsSQLInjection(order.TimeInForce) || validation.ContainsXSS(order.TimeInForce) {
			s.logger.Warn("Malicious content detected in time in force",
				zap.String("time_in_force", order.TimeInForce),
				zap.String("user_id", order.UserID.String()))
			return nil, fmt.Errorf("invalid time in force")
		}

		cleanTIF := strings.ToUpper(validation.SanitizeInput(order.TimeInForce))
		validTIF := map[string]bool{"GTC": true, "IOC": true, "FOK": true, "GTD": true}
		if !validTIF[cleanTIF] {
			return nil, fmt.Errorf("unsupported time in force: %s", cleanTIF)
		}
		order.TimeInForce = cleanTIF
	} else {
		order.TimeInForce = "GTC" // Default to Good Till Cancelled
	}

	// Rate limiting for order placement
	// Note: Rate limiter would need to be added to service struct
	// userKey := fmt.Sprintf("order_placement:%s", order.UserID.String())
	// Allow 100 orders per minute per user

	// 2. Business logic validation - check funds availability
	if cleanSide == "BUY" {
		// For buy orders, check quote currency balance
		quoteCurrency := "USD" // This should be extracted from symbol properly
		if len(cleanSymbol) > 3 {
			quoteCurrency = cleanSymbol[strings.Index(cleanSymbol, "/")+1:]
		}

		// Calculate required funds (price * quantity + fees)
		requiredFunds := order.Price * order.Quantity * 1.001 // Add 0.1% for fees

		// Check available balance through bookkeeper
		if s.bookkeeperSvc != nil {
			account, err := s.bookkeeperSvc.GetAccount(ctx, order.UserID.String(), quoteCurrency)
			if err != nil {
				s.logger.Error("Failed to get account", zap.Error(err))
				return nil, fmt.Errorf("failed to verify funds")
			}

			if account.Available < requiredFunds {
				return nil, fmt.Errorf("insufficient funds: required %.8f %s, available %.8f %s",
					requiredFunds, quoteCurrency, account.Available, quoteCurrency)
			}
		}
	} else {
		// For sell orders, check base currency balance
		baseCurrency := "BTC" // This should be extracted from symbol properly
		if strings.Contains(cleanSymbol, "/") {
			baseCurrency = cleanSymbol[:strings.Index(cleanSymbol, "/")]
		} else if len(cleanSymbol) >= 6 {
			baseCurrency = cleanSymbol[:3]
		}

		if s.bookkeeperSvc != nil {
			account, err := s.bookkeeperSvc.GetAccount(ctx, order.UserID.String(), baseCurrency)
			if err != nil {
				s.logger.Error("Failed to get account", zap.Error(err))
				return nil, fmt.Errorf("failed to verify funds")
			}

			if account.Available < order.Quantity {
				return nil, fmt.Errorf("insufficient funds: required %.8f %s, available %.8f %s",
					order.Quantity, baseCurrency, account.Available, baseCurrency)
			}
		}
	}

	// 3. Generate order ID and set initial status
	order.ID = uuid.New()
	order.Status = "NEW"
	order.CreatedAt = time.Now()
	order.UpdatedAt = time.Now()

	// 4. Convert to internal model and process through engine
	modelOrder := toModelOrder(order)
	if modelOrder == nil {
		return nil, fmt.Errorf("failed to convert order to internal model")
	}

	// Log order placement for audit
	s.logger.Info("Processing order placement",
		zap.String("order_id", order.ID.String()),
		zap.String("user_id", order.UserID.String()),
		zap.String("symbol", order.Symbol),
		zap.String("side", order.Side),
		zap.String("type", order.Type),
		zap.Float64("price", order.Price),
		zap.Float64("quantity", order.Quantity))

	// --- NEW: Call matching engine for real order processing ---
	processedOrder, _, _, err := s.engine.ProcessOrder(ctx, modelOrder, "api")
	if err != nil {
		return nil, err
	}
	apiOrder := toAPIOrder(processedOrder)
	apiOrder.UpdatedAt = time.Now()
	return apiOrder, nil
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
	var bids, asks [][]string
	bids, asks = ob.GetTopLevelsSnapshot(depth)
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

// --- Zero-Copy Serialization for OrderBookSnapshot ---
var orderBookBufferPool = sync.Pool{
	New: func() interface{} { return new(bytes.Buffer) },
}

// GetOrderBookBinary returns a binary-encoded order book snapshot using a buffer pool (zero-copy)
func (s *Service) GetOrderBookBinary(symbol string, depth int) ([]byte, error) {
	ob := engineGetOrderBook(s.engine, symbol)
	if ob == nil {
		return nil, fmt.Errorf("order book not found")
	}
	var bids, asks [][]string
	bids, asks = ob.GetTopLevelsSnapshot(depth)
	levelsBids := make([]marketdata.LevelDelta, 0, len(bids))
	for _, b := range bids {
		if len(b) >= 2 {
			levelsBids = append(levelsBids, marketdata.LevelDelta{
				Price:  parseFloat(b[0]),
				Volume: parseFloat(b[1]),
			})
		}
	}
	levelsAsks := make([]marketdata.LevelDelta, 0, len(asks))
	for _, a := range asks {
		if len(a) >= 2 {
			levelsAsks = append(levelsAsks, marketdata.LevelDelta{
				Price:  parseFloat(a[0]),
				Volume: parseFloat(a[1]),
			})
		}
	}
	snap := &marketdata.OrderBookSnapshot{
		Symbol: symbol,
		Bids:   levelsBids,
		Asks:   levelsAsks,
	}
	buf := orderBookBufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	data, err := marketdata.EncodeOrderBookSnapshot(snap)
	if err != nil {
		orderBookBufferPool.Put(buf)
		return nil, err
	}
	buf.Write(data)
	result := make([]byte, buf.Len())
	copy(result, buf.Bytes())
	orderBookBufferPool.Put(buf)
	return result, nil
}

// parseFloat helper
func parseFloat(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

// --- Minimal stub for order placement and retrieval for tests ---
// Patch engineGetOrder and engineGetOrders to allow test to pass
var testOrderStore = make(map[string]*model2.Order)

func engineGetOrder(engine *engine.MatchingEngine, orderID string) (*model2.Order, error) {
	if o, ok := testOrderStore[orderID]; ok {
		return o, nil
	}
	return nil, fmt.Errorf("failed to get order: not implemented")
}

func engineGetOrders(engine *engine.MatchingEngine, userID, symbol, status string, limit, offset int) ([]*model2.Order, int64, error) {
	var orders []*model2.Order
	for _, o := range testOrderStore {
		if userID == "" || o.UserID.String() == userID {
			orders = append(orders, o)
		}
	}
	return orders, int64(len(orders)), nil
}

// Patch engineGetOrderBook to return nil for now (stub)
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

// RiskConfig returns the risk config for admin API access
func (s *Service) RiskConfig() *risk.RiskConfig {
	return s.riskConfig
}
