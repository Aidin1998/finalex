//go:generate easyjson -all model.go

package model

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// OrderPool and TradePool provide zero-allocation pooling for hot path usage.
var OrderPool = sync.Pool{New: func() any { return new(Order) }}
var TradePool = sync.Pool{New: func() any { return new(Trade) }}

// PriceFeedAdapter adapts a context-aware price feed to the legacy interface
// (for API compatibility)
type PriceFeedAdapter struct {
	CtxAware interface {
		GetLastPrice(ctx context.Context, pair string) (decimal.Decimal, error)
	}
}

func (a *PriceFeedAdapter) GetLastPrice(pair string) (decimal.Decimal, error) {
	return a.CtxAware.GetLastPrice(context.Background(), pair)
}

// Constants for Order types, sides, statuses, and time in force options
const (
	// Order types
	OrderTypeLimit     = "LIMIT"
	OrderTypeMarket    = "MARKET"
	OrderTypeStopLimit = "STOP_LIMIT"
	OrderTypeIOC       = "IOC"
	OrderTypeFOK       = "FOK"

	// Order sides
	OrderSideBuy  = "BUY"
	OrderSideSell = "SELL"

	// Order statuses
	OrderStatusPendingNew      = "PENDING_NEW"
	OrderStatusOpen            = "OPEN"
	OrderStatusPartiallyFilled = "PARTIALLY_FILLED"
	OrderStatusFilled          = "FILLED"
	OrderStatusCancelled       = "CANCELLED"
	OrderStatusRejected        = "REJECTED"
	OrderStatusExpired         = "EXPIRED"

	// Additional order statuses for stop orders
	OrderStatusPendingTrigger = "PENDING_TRIGGER" // Waiting for stop price to be triggered
	OrderStatusTriggered      = "TRIGGERED"       // Stop price has been triggered

	// Time in force
	TimeInForceGTC = "GTC" // Good Till Cancelled
	TimeInForceIOC = "IOC" // Immediate Or Cancel
	TimeInForceFOK = "FOK" // Fill Or Kill
	TimeInForceGTD = "GTD" // Good Till Date
)

// NewOrderForTest creates a new Order with random UUIDs and decimal values for testing
func NewOrderForTest(pair, side string, priceStr, qtyStr string) *Order { // Changed price, qty to string
	price, _ := decimal.NewFromString(priceStr)
	qty, _ := decimal.NewFromString(qtyStr)
	return &Order{
		ID:        uuid.New(),
		UserID:    uuid.New(),
		Pair:      pair,
		Type:      OrderTypeLimit,
		Side:      side,
		Price:     price, // Was decimal.NewFromFloat(price)
		Quantity:  qty,   // Was decimal.NewFromFloat(qty)
		Status:    OrderStatusOpen,
		CreatedAt: time.Now(),
	}
}

// Preallocate a large number of Order and Trade objects for the pools at startup
func PreallocateObjectPools() {
	const preallocOrders = 8192
	const preallocTrades = 8192
	for i := 0; i < preallocOrders; i++ {
		OrderPool.Put(new(Order))
	}
	for i := 0; i < preallocTrades; i++ {
		TradePool.Put(new(Trade))
	}
}

// Call PreallocateObjectPools() during application startup (main.go or engine.go)

// OrderEventType represents the type of event for orderbook snapshots and recovery
// Add more event types as needed for future features
type OrderEventType int

const (
	OrderEventAdd OrderEventType = iota
	OrderEventCancel
)

// OrderEvent represents an event in the order book (add, cancel, etc.)
type OrderEvent struct {
	Type    OrderEventType
	Order   *Order
	Reason  string // Optional: reason for cancel, etc.
	Time    time.Time
	EventID int64 // Unique, monotonically increasing event ID for ordering and recovery
}

// Order represents a trading order in the system.
type Order struct {
	ID             uuid.UUID       `json:"id"`
	UserID         uuid.UUID       `json:"user_id"`
	Pair           string          `json:"pair"`
	Side           string          `json:"side"`
	Type           string          `json:"type"`
	Price          decimal.Decimal `json:"price"`
	Quantity       decimal.Decimal `json:"quantity"`
	FilledQuantity decimal.Decimal `json:"filled_quantity"`
	Status         string          `json:"status"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
	TimeInForce    string          `json:"time_in_force"`
	StopPrice      decimal.Decimal `json:"stop_price,omitempty"`
	OCOGroupID     *uuid.UUID      `json:"oco_group_id,omitempty"`
	AvgPrice       decimal.Decimal `json:"avg_price"`
	// ... add other fields as needed ...
}

// Trade represents a trade execution in the system.
type Trade struct {
	ID        uuid.UUID       `json:"id"`
	OrderID   uuid.UUID       `json:"order_id"`
	Pair      string          `json:"pair"`
	Price     decimal.Decimal `json:"price"`
	Quantity  decimal.Decimal `json:"quantity"`
	Side      string          `json:"side"`
	Maker     bool            `json:"maker"`
	CreatedAt time.Time       `json:"created_at"`
	// ... add other fields as needed ...
}
