package model

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// Repository defines the interface for order storage operations
type Repository interface {
	CreateOrder(ctx context.Context, order *Order) error
	GetOrderByID(ctx context.Context, orderID uuid.UUID) (*Order, error)
	GetOpenOrdersByPair(ctx context.Context, pair string) ([]*Order, error)
	GetOpenOrdersByUser(ctx context.Context, userID uuid.UUID) ([]*Order, error)
	UpdateOrderStatus(ctx context.Context, orderID uuid.UUID, status string) error
	UpdateOrderStatusAndFilledQuantity(ctx context.Context, orderID uuid.UUID, status string, filledQty, avgPrice decimal.Decimal) error
	CancelOrder(ctx context.Context, orderID uuid.UUID) error
	ConvertStopOrderToLimit(ctx context.Context, order *Order) error
	CreateTrade(ctx context.Context, trade *Trade) error
	CreateTradeTx(ctx context.Context, tx *gorm.DB, trade *Trade) error
	UpdateOrderStatusTx(ctx context.Context, tx *gorm.DB, orderID uuid.UUID, status string, filledQty, avgPrice decimal.Decimal) error
	BatchUpdateOrderStatusTx(ctx context.Context, tx *gorm.DB, updates []struct {
		OrderID uuid.UUID
		Status  string
	}) error
	ExecuteInTransaction(ctx context.Context, txFunc func(*gorm.DB) error) error
	UpdateOrderHoldID(ctx context.Context, orderID uuid.UUID, holdID string) error
	GetExpiredGTDOrders(ctx context.Context, pair string, now time.Time) ([]*Order, error)
	// Add stubs for OCO and trailing stop support for compatibility
	GetOCOSiblingOrder(ctx context.Context, ocoGroupID uuid.UUID, excludeOrderID uuid.UUID) (*Order, error)
	GetOpenTrailingStopOrders(ctx context.Context, pair string) ([]*Order, error)
}

// MatchingEngineInterface defines the public methods of the matching engine
type MatchingEngineInterface interface {
	ProcessLimitOrder(ctx context.Context, order *Order) ([]*Trade, error)
	ProcessMarketOrder(ctx context.Context, order *Order) ([]*Trade, error)
	ProcessCancelOrder(ctx context.Context, orderID uuid.UUID, pair string) error
	LoadOrderBook(ctx context.Context, pair string) error
	GetOrderBookSnapshot(pair string, depth int) interface{}
	RegisterTradeHandler(handler func(TradeEvent))
}

// StopOrderActivator defines the interface for activating stop orders
type StopOrderActivator interface {
	ActivateStopOrder(ctx context.Context, order *Order) error
}

// PriceFeed defines the interface for getting price information
type PriceFeed interface {
	GetLastPrice(pair string) (decimal.Decimal, error)
}
