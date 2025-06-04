package trading

import (
	"context"
)

// Service defines the consolidated trading service interface
type Service interface {
	// Order operations
	PlaceOrder(ctx context.Context, userID, market string, side string, orderType string, price, amount float64) (interface{}, error)
	CancelOrder(ctx context.Context, userID, orderID string) error
	GetOrder(ctx context.Context, orderID string) (interface{}, error)
	GetUserOrders(ctx context.Context, userID string, market string, status string, limit, offset int) ([]interface{}, error)

	// Market operations
	CreateMarket(ctx context.Context, baseCurrency, quoteCurrency string) (string, error)
	GetMarket(ctx context.Context, marketID string) (interface{}, error)
	GetMarkets(ctx context.Context) ([]interface{}, error)
	UpdateMarketStatus(ctx context.Context, marketID, status string) error

	// Trade operations
	GetTrades(ctx context.Context, market string, limit, offset int) ([]interface{}, error)
	GetUserTrades(ctx context.Context, userID, market string, limit, offset int) ([]interface{}, error)

	// Orderbook operations
	GetOrderbook(ctx context.Context, market string, depth int) (interface{}, error)

	// Settlement operations
	SettleTrade(ctx context.Context, trade interface{}) error
	GetSettlementStatus(ctx context.Context, tradeID string) (string, error)

	// Service lifecycle
	Start() error
	Stop() error
}

// NewService creates a new consolidated trading service
// func NewService(logger *zap.Logger, db *gorm.DB, bookkeeper BookkeeperService, wsHub WebSocketHub, settlement SettlementEngine) (Service, error)
