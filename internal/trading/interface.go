package trading

import (
	"context"
	"time"
)

// Service defines the consolidated trading service interface with enterprise-level features
type Service interface {
	// Order operations with enhanced features
	PlaceOrder(ctx context.Context, userID, market string, side string, orderType string, price, amount float64) (interface{}, error)
	PlaceOrderBatch(ctx context.Context, orders []OrderRequest) ([]OrderResult, error)
	CancelOrder(ctx context.Context, userID, orderID string) error
	CancelOrderBatch(ctx context.Context, userID string, orderIDs []string) ([]CancelResult, error)
	CancelAllOrders(ctx context.Context, userID, market string) error
	GetOrder(ctx context.Context, orderID string) (interface{}, error)
	GetUserOrders(ctx context.Context, userID string, market string, status string, limit, offset int) ([]interface{}, error)
	GetOrderHistory(ctx context.Context, userID, market string, startTime, endTime time.Time, limit, offset int) ([]interface{}, error)

	// Advanced order operations
	PlaceStopOrder(ctx context.Context, userID, market string, side string, stopPrice, price, amount float64) (interface{}, error)
	PlaceIcebergOrder(ctx context.Context, userID, market string, side string, price, totalAmount, visibleAmount float64) (interface{}, error)
	ModifyOrder(ctx context.Context, userID, orderID string, newPrice, newAmount *float64) (interface{}, error)

	// Market operations
	CreateMarket(ctx context.Context, baseCurrency, quoteCurrency string) (string, error)
	GetMarket(ctx context.Context, marketID string) (interface{}, error)
	GetMarkets(ctx context.Context) ([]interface{}, error)
	UpdateMarketStatus(ctx context.Context, marketID, status string) error
	GetMarketStats(ctx context.Context, market string) (interface{}, error)
	GetTicker(ctx context.Context, market string) (interface{}, error)
	GetAllTickers(ctx context.Context) ([]interface{}, error)

	// Trade operations
	GetTrades(ctx context.Context, market string, limit, offset int) ([]interface{}, error)
	GetUserTrades(ctx context.Context, userID, market string, limit, offset int) ([]interface{}, error)
	GetTradeHistory(ctx context.Context, userID, market string, startTime, endTime time.Time, limit, offset int) ([]interface{}, error)
	GetAggregatedTrades(ctx context.Context, market string, startTime, endTime time.Time, limit int) ([]interface{}, error)

	// Orderbook operations with enhanced features
	GetOrderbook(ctx context.Context, market string, depth int) (interface{}, error)
	GetOrderbookStream(ctx context.Context, market string) (<-chan interface{}, error)
	GetMarketDepth(ctx context.Context, market string, limit int) (interface{}, error)

	// Kline/Candlestick data
	GetKlines(ctx context.Context, market, interval string, startTime, endTime time.Time, limit int) ([]interface{}, error)

	// Settlement operations
	SettleTrade(ctx context.Context, trade interface{}) error
	GetSettlementStatus(ctx context.Context, tradeID string) (string, error)
	BatchSettlement(ctx context.Context, trades []interface{}) error

	// Risk management operations
	GetUserRiskMetrics(ctx context.Context, userID string) (interface{}, error)
	UpdateRiskLimits(ctx context.Context, userID string, limits interface{}) error
	GetMarketRiskStatus(ctx context.Context, market string) (interface{}, error)

	// Admin operations
	GetSystemMetrics(ctx context.Context) (interface{}, error)
	GetOrderbookMetrics(ctx context.Context, market string) (interface{}, error)
	GetTradingActivity(ctx context.Context, startTime, endTime time.Time) (interface{}, error)
	HaltTrading(ctx context.Context, market string, reason string) error
	ResumeTrading(ctx context.Context, market string) error

	// Circuit breaker operations
	TriggerCircuitBreaker(ctx context.Context, market, reason string) error
	ResetCircuitBreaker(ctx context.Context, market string) error
	GetCircuitBreakerStatus(ctx context.Context, market string) (interface{}, error)

	// Service lifecycle
	Start() error
	Stop() error
	Health() error
	Ready() error
}

// OrderRequest represents a batch order request
type OrderRequest struct {
	UserID      string  `json:"user_id"`
	Market      string  `json:"market"`
	Side        string  `json:"side"`
	Type        string  `json:"type"`
	Price       float64 `json:"price"`
	Amount      float64 `json:"amount"`
	TimeInForce string  `json:"time_in_force,omitempty"`
	ClientID    string  `json:"client_id,omitempty"`
}

// OrderResult represents the result of an order operation
type OrderResult struct {
	OrderID  string      `json:"order_id"`
	ClientID string      `json:"client_id,omitempty"`
	Success  bool        `json:"success"`
	Error    string      `json:"error,omitempty"`
	Order    interface{} `json:"order,omitempty"`
}

// CancelResult represents the result of a cancel operation
type CancelResult struct {
	OrderID string `json:"order_id"`
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// NewService creates a new consolidated trading service
// func NewService(logger *zap.Logger, db *gorm.DB, bookkeeper BookkeeperService, wsHub WebSocketHub, settlement SettlementEngine) (Service, error)
