//go:generate easyjson -all model.go

package model

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Enhanced Object Pools with Metrics and Pre-warming
// OrderPool and TradePool provide zero-allocation pooling for hot path usage.

// Pool metrics for monitoring hit/miss rates and performance
type PoolMetrics struct {
	Gets        int64 // Total gets from pool
	Puts        int64 // Total puts to pool
	Hits        int64 // Objects served from pool (not newly created)
	Misses      int64 // Objects created because pool was empty
	Allocations int64 // Total new objects allocated
}

var (
	orderPoolMetrics = &PoolMetrics{}
	tradePoolMetrics = &PoolMetrics{}
)

// Enhanced OrderPool with proper reset and metrics
var OrderPool = sync.Pool{
	New: func() any {
		atomic.AddInt64(&orderPoolMetrics.Allocations, 1)
		atomic.AddInt64(&orderPoolMetrics.Misses, 1)
		return new(Order)
	},
}

// Enhanced TradePool with proper reset and metrics
var TradePool = sync.Pool{
	New: func() any {
		atomic.AddInt64(&tradePoolMetrics.Allocations, 1)
		atomic.AddInt64(&tradePoolMetrics.Misses, 1)
		return new(Trade)
	},
}

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
	OrderTypeUnknown   = "UNKNOWN"
	OrderTypeLimit     = "LIMIT"
	OrderTypeMarket    = "MARKET"
	OrderTypeStopLimit = "STOP_LIMIT"
	OrderTypeIOC       = "IOC"
	OrderTypeFOK       = "FOK"
	OrderTypeIceberg   = "ICEBERG"
	OrderTypeHidden    = "HIDDEN"
	OrderTypeGTD       = "GTD"
	OrderTypeOCO       = "OCO"
	OrderTypeTrailing  = "TRAILING_STOP"
	OrderTypeTWAP      = "TWAP"
	OrderTypeVWAP      = "VWAP"

	// Order sides
	OrderSideBuy  = "BUY"
	OrderSideSell = "SELL"

	// Order statuses
	OrderStatusNew             = "NEW"
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

	// Algorithmic and conditional order statuses
	OrderStatusAlgoPending  = "ALGO_PENDING"  // For TWAP/VWAP not yet started
	OrderStatusAlgoActive   = "ALGO_ACTIVE"   // For TWAP/VWAP in progress
	OrderStatusAlgoComplete = "ALGO_COMPLETE" // For TWAP/VWAP done
	OrderStatusOCOTriggered = "OCO_TRIGGERED" // For OCO leg triggered
	OrderStatusHidden       = "HIDDEN"        // For hidden/iceberg orders

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

// Enhanced preallocation with configurable pool sizes
func PreallocateObjectPools() {
	PreallocateObjectPoolsWithSizes(8192, 8192)
}

// PreallocateObjectPoolsWithSizes allows configurable pool pre-warming
func PreallocateObjectPoolsWithSizes(preallocOrders, preallocTrades int) {
	// Ensure minimum of 1000 objects as per requirements
	if preallocOrders < 1000 {
		preallocOrders = 1000
	}
	if preallocTrades < 1000 {
		preallocTrades = 1000
	}

	// Pre-warm OrderPool
	for i := 0; i < preallocOrders; i++ {
		order := &Order{}
		OrderPool.Put(order)
	}

	// Pre-warm TradePool
	for i := 0; i < preallocTrades; i++ {
		trade := &Trade{}
		TradePool.Put(trade)
	}

	fmt.Printf("Object pools pre-warmed: %d orders, %d trades\n", preallocOrders, preallocTrades)
}

// Call PreallocateObjectPools() during application startup (main.go or engine.go)

// GetOrderFromPool gets an order from the pool with metrics tracking
func GetOrderFromPool() *Order {
	atomic.AddInt64(&orderPoolMetrics.Gets, 1)
	order := OrderPool.Get().(*Order)
	if order.ID != uuid.Nil {
		// Object was reused from pool
		atomic.AddInt64(&orderPoolMetrics.Hits, 1)
	}
	return order
}

// PutOrderToPool returns an order to the pool after resetting it
func PutOrderToPool(order *Order) {
	if order != nil {
		ResetOrder(order)
		atomic.AddInt64(&orderPoolMetrics.Puts, 1)
		OrderPool.Put(order)
	}
}

// GetTradeFromPool gets a trade from the pool with metrics tracking
func GetTradeFromPool() *Trade {
	atomic.AddInt64(&tradePoolMetrics.Gets, 1)
	trade := TradePool.Get().(*Trade)
	if trade.ID != uuid.Nil {
		// Object was reused from pool
		atomic.AddInt64(&tradePoolMetrics.Hits, 1)
	}
	return trade
}

// PutTradeToPool returns a trade to the pool after resetting it
func PutTradeToPool(trade *Trade) {
	if trade != nil {
		ResetTrade(trade)
		atomic.AddInt64(&tradePoolMetrics.Puts, 1)
		TradePool.Put(trade)
	}
}

// ResetOrder clears all fields of an order for safe pooling
func ResetOrder(order *Order) {
	*order = Order{}
}

// ResetTrade clears all fields of a trade for safe pooling
func ResetTrade(trade *Trade) {
	*trade = Trade{}
}

// GetOrderPoolMetrics returns current pool metrics for monitoring
func GetOrderPoolMetrics() PoolMetrics {
	return PoolMetrics{
		Gets:        atomic.LoadInt64(&orderPoolMetrics.Gets),
		Puts:        atomic.LoadInt64(&orderPoolMetrics.Puts),
		Hits:        atomic.LoadInt64(&orderPoolMetrics.Hits),
		Misses:      atomic.LoadInt64(&orderPoolMetrics.Misses),
		Allocations: atomic.LoadInt64(&orderPoolMetrics.Allocations),
	}
}

// GetTradePoolMetrics returns current pool metrics for monitoring
func GetTradePoolMetrics() PoolMetrics {
	return PoolMetrics{
		Gets:        atomic.LoadInt64(&tradePoolMetrics.Gets),
		Puts:        atomic.LoadInt64(&tradePoolMetrics.Puts),
		Hits:        atomic.LoadInt64(&tradePoolMetrics.Hits),
		Misses:      atomic.LoadInt64(&tradePoolMetrics.Misses),
		Allocations: atomic.LoadInt64(&tradePoolMetrics.Allocations),
	}
}

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
	ID              uuid.UUID              `json:"id"`
	UserID          uuid.UUID              `json:"user_id"`
	Pair            string                 `json:"pair"`
	Side            string                 `json:"side"`
	Type            string                 `json:"type"`
	Price           decimal.Decimal        `json:"price"`
	Quantity        decimal.Decimal        `json:"quantity"`
	FilledQuantity  decimal.Decimal        `json:"filled_quantity"`
	Status          string                 `json:"status"`
	CreatedAt       time.Time              `json:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at"`
	TimeInForce     string                 `json:"time_in_force"`
	StopPrice       decimal.Decimal        `json:"stop_price,omitempty"`
	OCOGroupID      *uuid.UUID             `json:"oco_group_id,omitempty"`
	AvgPrice        decimal.Decimal        `json:"avg_price"`
	DisplayQuantity decimal.Decimal        `json:"display_quantity,omitempty"` // For iceberg/hidden orders
	ExpireAt        *time.Time             `json:"expire_at,omitempty"`        // For GTD orders
	TrailingOffset  decimal.Decimal        `json:"trailing_offset,omitempty"`  // For trailing stop
	AlgoParams      map[string]interface{} `json:"algo_params,omitempty"`      // For TWAP/VWAP
	ParentOrderID   *uuid.UUID             `json:"parent_order_id,omitempty"`  // For OCO/algos
	Hidden          bool                   `json:"hidden,omitempty"`           // For hidden/iceberg
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

// ValidateAdvancedOrder checks all advanced order types and parameters
func (o *Order) ValidateAdvanced() error {
	if o.Quantity.LessThanOrEqual(decimal.Zero) {
		return fmt.Errorf("quantity must be positive")
	}
	if o.Type == OrderTypeIceberg || o.Type == OrderTypeHidden {
		if o.DisplayQuantity.LessThanOrEqual(decimal.Zero) || o.DisplayQuantity.GreaterThan(o.Quantity) {
			return fmt.Errorf("invalid display quantity for iceberg/hidden order")
		}
	}
	if o.Type == OrderTypeFOK && o.TimeInForce != TimeInForceFOK {
		return fmt.Errorf("FOK order must have TimeInForce=FOK")
	}
	if o.Type == OrderTypeGTD && o.ExpireAt == nil {
		return fmt.Errorf("GTD order must have ExpireAt timestamp")
	}
	if o.Type == OrderTypeOCO && o.OCOGroupID == nil {
		return fmt.Errorf("OCO order must have OCOGroupID")
	}
	if o.Type == OrderTypeTrailing && o.TrailingOffset.LessThanOrEqual(decimal.Zero) {
		return fmt.Errorf("trailing stop must have positive offset")
	}
	if (o.Type == OrderTypeTWAP || o.Type == OrderTypeVWAP) && len(o.AlgoParams) == 0 {
		return fmt.Errorf("TWAP/VWAP order must have algo params")
	}
	// Prevent invalid combinations
	if o.Type == OrderTypeFOK || o.Type == OrderTypeGTD {
		// These are valid individual types, check for other invalid combinations
		if o.Type == OrderTypeFOK && o.Side == "" {
			return fmt.Errorf("FOK order must have valid side")
		}
		if o.Type == OrderTypeGTD && o.ExpireAt == nil {
			return fmt.Errorf("GTD order must have expiration time")
		}
	}
	if o.Type == OrderTypeOCO && o.ParentOrderID != nil {
		return fmt.Errorf("OCO child cannot have ParentOrderID set")
	}
	// TODO: Add trading pair specific rules (min/max, increments) here
	return nil
}
