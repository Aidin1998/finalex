// =============================
// Advanced Order Trigger Monitoring Service
// =============================
// This service monitors market conditions and triggers advanced orders
// including stop-loss, take-profit, and iceberg orders with <100ms latency

package trigger

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	model "github.com/Aidin1998/pincex_unified/internal/trading/model"
	orderbook "github.com/Aidin1998/pincex_unified/internal/trading/orderbook"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// TriggerType represents different trigger conditions
type TriggerType int

const (
	TriggerTypeStopLoss TriggerType = iota
	TriggerTypeTakeProfit
	TriggerTypeIcebergRefill
	TriggerTypeTrailingStop
	TriggerTypeOCO
)

// TriggerCondition defines conditions for order triggering
type TriggerCondition struct {
	ID           uuid.UUID       `json:"id"`
	OrderID      uuid.UUID       `json:"order_id"`
	UserID       uuid.UUID       `json:"user_id"`
	Pair         string          `json:"pair"`
	Type         TriggerType     `json:"type"`
	TriggerPrice decimal.Decimal `json:"trigger_price"`
	Direction    string          `json:"direction"` // "above" or "below"
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`

	// Advanced fields
	TrailingOffset    decimal.Decimal `json:"trailing_offset,omitempty"`
	LastTrailingPrice decimal.Decimal `json:"last_trailing_price,omitempty"`

	// Iceberg specific
	DisplayQuantity   decimal.Decimal `json:"display_quantity,omitempty"`
	RemainingQuantity decimal.Decimal `json:"remaining_quantity,omitempty"`

	// Callback for execution
	OnTrigger func(condition *TriggerCondition, currentPrice decimal.Decimal) error `json:"-"`
}

// IcebergOrderState tracks iceberg order execution state
type IcebergOrderState struct {
	OrderID           uuid.UUID       `json:"order_id"`
	UserID            uuid.UUID       `json:"user_id"`
	Pair              string          `json:"pair"`
	Side              string          `json:"side"`
	TotalQuantity     decimal.Decimal `json:"total_quantity"`
	DisplayQuantity   decimal.Decimal `json:"display_quantity"`
	RemainingQuantity decimal.Decimal `json:"remaining_quantity"`
	FilledQuantity    decimal.Decimal `json:"filled_quantity"`
	Price             decimal.Decimal `json:"price"`
	CurrentSliceID    uuid.UUID       `json:"current_slice_id"`
	SliceCount        int             `json:"slice_count"`
	RandomizeSlices   bool            `json:"randomize_slices"`
	MinSliceSize      decimal.Decimal `json:"min_slice_size"`
	MaxSliceSize      decimal.Decimal `json:"max_slice_size"`
	LastRefill        time.Time       `json:"last_refill"`
	Status            string          `json:"status"`
}

// TriggerMonitor manages order trigger conditions
type TriggerMonitor struct {
	// Core components
	logger       *zap.SugaredLogger
	orderRepo    model.Repository
	orderBooks   map[string]orderbook.OrderBookInterface
	orderBooksMu sync.RWMutex

	// Monitoring data
	conditions    map[uuid.UUID]*TriggerCondition
	conditionsMu  sync.RWMutex
	icebergOrders map[uuid.UUID]*IcebergOrderState
	icebergMu     sync.RWMutex

	// Market data tracking
	lastPrices map[string]decimal.Decimal
	pricesMu   sync.RWMutex

	// Performance metrics
	triggersProcessed int64
	avgLatencyNs      int64
	maxLatencyNs      int64

	// Configuration
	monitorInterval time.Duration
	maxConditions   int

	// Control
	running  int32
	stopChan chan struct{}
	workerWg sync.WaitGroup

	// Callbacks
	onOrderTrigger func(order *model.Order) error
	onIcebergSlice func(state *IcebergOrderState, newOrder *model.Order) error
}

// NewTriggerMonitor creates a new trigger monitoring service
func NewTriggerMonitor(
	logger *zap.SugaredLogger,
	orderRepo model.Repository,
	monitorInterval time.Duration,
) *TriggerMonitor {
	return &TriggerMonitor{
		logger:          logger,
		orderRepo:       orderRepo,
		orderBooks:      make(map[string]orderbook.OrderBookInterface),
		conditions:      make(map[uuid.UUID]*TriggerCondition),
		icebergOrders:   make(map[uuid.UUID]*IcebergOrderState),
		lastPrices:      make(map[string]decimal.Decimal),
		monitorInterval: monitorInterval,
		maxConditions:   10000, // Support 10K+ monitored orders
		stopChan:        make(chan struct{}),
	}
}

// Start begins the trigger monitoring service
func (tm *TriggerMonitor) Start(ctx context.Context) error {
	if !atomic.CompareAndSwapInt32(&tm.running, 0, 1) {
		return nil // Already running
	}

	tm.logger.Info("Starting trigger monitoring service")

	// Start monitoring workers
	for i := 0; i < 4; i++ { // Multiple workers for parallel processing
		tm.workerWg.Add(1)
		go tm.monitoringWorker(ctx, i)
	}

	// Start iceberg management worker
	tm.workerWg.Add(1)
	go tm.icebergWorker(ctx)

	// Start metrics collection worker
	tm.workerWg.Add(1)
	go tm.metricsWorker(ctx)

	return nil
}

// Stop stops the trigger monitoring service
func (tm *TriggerMonitor) Stop() error {
	if !atomic.CompareAndSwapInt32(&tm.running, 1, 0) {
		return nil // Not running
	}

	tm.logger.Info("Stopping trigger monitoring service")
	close(tm.stopChan)
	tm.workerWg.Wait()

	return nil
}

// AddOrderBook registers an order book for price monitoring
func (tm *TriggerMonitor) AddOrderBook(pair string, ob orderbook.OrderBookInterface) {
	tm.orderBooksMu.Lock()
	defer tm.orderBooksMu.Unlock()
	tm.orderBooks[pair] = ob
}

// SetOrderTriggerCallback sets the callback for triggered orders
func (tm *TriggerMonitor) SetOrderTriggerCallback(callback func(order *model.Order) error) {
	tm.onOrderTrigger = callback
}

// SetIcebergSliceCallback sets the callback for iceberg slice creation
func (tm *TriggerMonitor) SetIcebergSliceCallback(callback func(state *IcebergOrderState, newOrder *model.Order) error) {
	tm.onIcebergSlice = callback
}

// AddStopLossOrder adds a stop-loss trigger condition
func (tm *TriggerMonitor) AddStopLossOrder(order *model.Order) error {
	if order.StopPrice.IsZero() {
		return nil // No stop price specified
	}

	direction := "below"
	if order.Side == model.OrderSideBuy {
		direction = "above" // Buy stop: trigger when price goes above stop price
	}

	condition := &TriggerCondition{
		ID:           uuid.New(),
		OrderID:      order.ID,
		UserID:       order.UserID,
		Pair:         order.Pair,
		Type:         TriggerTypeStopLoss,
		TriggerPrice: order.StopPrice,
		Direction:    direction,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		OnTrigger:    tm.triggerStopLoss,
	}

	tm.conditionsMu.Lock()
	defer tm.conditionsMu.Unlock()

	if len(tm.conditions) >= tm.maxConditions {
		return model.ErrTooManyConditions
	}

	tm.conditions[condition.ID] = condition
	tm.logger.Debugw("Added stop-loss trigger",
		"order_id", order.ID,
		"trigger_price", order.StopPrice,
		"direction", direction)

	return nil
}

// AddTakeProfitOrder adds a take-profit trigger condition
func (tm *TriggerMonitor) AddTakeProfitOrder(order *model.Order, profitPrice decimal.Decimal) error {
	if profitPrice.IsZero() {
		return nil // No profit price specified
	}

	direction := "above"
	if order.Side == model.OrderSideBuy {
		direction = "above" // Buy: take profit when price goes above target
	} else {
		direction = "below" // Sell: take profit when price goes below target
	}

	condition := &TriggerCondition{
		ID:           uuid.New(),
		OrderID:      order.ID,
		UserID:       order.UserID,
		Pair:         order.Pair,
		Type:         TriggerTypeTakeProfit,
		TriggerPrice: profitPrice,
		Direction:    direction,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		OnTrigger:    tm.triggerTakeProfit,
	}

	tm.conditionsMu.Lock()
	defer tm.conditionsMu.Unlock()

	if len(tm.conditions) >= tm.maxConditions {
		return model.ErrTooManyConditions
	}

	tm.conditions[condition.ID] = condition
	tm.logger.Debugw("Added take-profit trigger",
		"order_id", order.ID,
		"trigger_price", profitPrice,
		"direction", direction)

	return nil
}

// AddIcebergOrder adds an iceberg order for slice management
func (tm *TriggerMonitor) AddIcebergOrder(order *model.Order) error {
	if order.Type != model.OrderTypeIceberg {
		return nil
	}

	if order.DisplayQuantity.IsZero() || order.DisplayQuantity.GreaterThan(order.Quantity) {
		return model.ErrInvalidDisplayQuantity
	}

	state := &IcebergOrderState{
		OrderID:           order.ID,
		UserID:            order.UserID,
		Pair:              order.Pair,
		Side:              order.Side,
		TotalQuantity:     order.Quantity,
		DisplayQuantity:   order.DisplayQuantity,
		RemainingQuantity: order.Quantity,
		FilledQuantity:    decimal.Zero,
		Price:             order.Price,
		CurrentSliceID:    uuid.New(),
		SliceCount:        1,
		RandomizeSlices:   false, // Can be configured
		MinSliceSize:      order.DisplayQuantity.Div(decimal.NewFromInt(2)),
		MaxSliceSize:      order.DisplayQuantity.Mul(decimal.NewFromInt(2)),
		LastRefill:        time.Now(),
		Status:            "active",
	}

	tm.icebergMu.Lock()
	defer tm.icebergMu.Unlock()

	tm.icebergOrders[order.ID] = state
	tm.logger.Debugw("Added iceberg order",
		"order_id", order.ID,
		"total_quantity", order.Quantity,
		"display_quantity", order.DisplayQuantity)

	return tm.createIcebergSlice(state)
}

// AddTrailingStopOrder adds a trailing stop order
func (tm *TriggerMonitor) AddTrailingStopOrder(order *model.Order) error {
	if order.Type != model.OrderTypeTrailing || order.TrailingOffset.IsZero() {
		return nil
	}

	// Get current market price to set initial trailing price
	currentPrice := tm.getCurrentPrice(order.Pair)
	if currentPrice.IsZero() {
		return model.ErrNoMarketPrice
	}

	var triggerPrice decimal.Decimal
	direction := "below"

	if order.Side == model.OrderSideBuy {
		// Buy trailing stop: trigger when price goes above (current - offset)
		triggerPrice = currentPrice.Add(order.TrailingOffset)
		direction = "above"
	} else {
		// Sell trailing stop: trigger when price goes below (current - offset)
		triggerPrice = currentPrice.Sub(order.TrailingOffset)
		direction = "below"
	}

	condition := &TriggerCondition{
		ID:                uuid.New(),
		OrderID:           order.ID,
		UserID:            order.UserID,
		Pair:              order.Pair,
		Type:              TriggerTypeTrailingStop,
		TriggerPrice:      triggerPrice,
		Direction:         direction,
		TrailingOffset:    order.TrailingOffset,
		LastTrailingPrice: currentPrice,
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
		OnTrigger:         tm.triggerTrailingStop,
	}

	tm.conditionsMu.Lock()
	defer tm.conditionsMu.Unlock()

	if len(tm.conditions) >= tm.maxConditions {
		return model.ErrTooManyConditions
	}

	tm.conditions[condition.ID] = condition
	tm.logger.Debugw("Added trailing stop trigger",
		"order_id", order.ID,
		"trigger_price", triggerPrice,
		"trailing_offset", order.TrailingOffset,
		"direction", direction)

	return nil
}

// RemoveCondition removes a trigger condition
func (tm *TriggerMonitor) RemoveCondition(orderID uuid.UUID) {
	tm.conditionsMu.Lock()
	defer tm.conditionsMu.Unlock()

	for id, condition := range tm.conditions {
		if condition.OrderID == orderID {
			delete(tm.conditions, id)
			tm.logger.Debugw("Removed trigger condition", "order_id", orderID)
			break
		}
	}
}

// RemoveIcebergOrder removes an iceberg order
func (tm *TriggerMonitor) RemoveIcebergOrder(orderID uuid.UUID) {
	tm.icebergMu.Lock()
	defer tm.icebergMu.Unlock()

	if state, exists := tm.icebergOrders[orderID]; exists {
		state.Status = "cancelled"
		delete(tm.icebergOrders, orderID)
		tm.logger.Debugw("Removed iceberg order", "order_id", orderID)
	}
}

// UpdateIcebergFill updates iceberg order after a fill
func (tm *TriggerMonitor) UpdateIcebergFill(orderID uuid.UUID, filledQuantity decimal.Decimal) error {
	tm.icebergMu.Lock()
	defer tm.icebergMu.Unlock()

	state, exists := tm.icebergOrders[orderID]
	if !exists {
		return model.ErrOrderNotFound
	}

	state.FilledQuantity = state.FilledQuantity.Add(filledQuantity)
	state.RemainingQuantity = state.TotalQuantity.Sub(state.FilledQuantity)

	tm.logger.Debugw("Updated iceberg fill",
		"order_id", orderID,
		"filled", filledQuantity,
		"remaining", state.RemainingQuantity)

	// Check if we need to create a new slice
	if state.RemainingQuantity.GreaterThan(decimal.Zero) {
		return tm.createIcebergSlice(state)
	} else {
		state.Status = "filled"
		delete(tm.icebergOrders, orderID)
	}

	return nil
}

// GetTriggerStats returns trigger monitoring statistics
func (tm *TriggerMonitor) GetTriggerStats() map[string]interface{} {
	tm.conditionsMu.RLock()
	conditionsCount := len(tm.conditions)
	tm.conditionsMu.RUnlock()

	tm.icebergMu.RLock()
	icebergCount := len(tm.icebergOrders)
	tm.icebergMu.RUnlock()

	return map[string]interface{}{
		"active_conditions":   conditionsCount,
		"active_icebergs":     icebergCount,
		"triggers_processed":  atomic.LoadInt64(&tm.triggersProcessed),
		"avg_latency_ns":      atomic.LoadInt64(&tm.avgLatencyNs),
		"max_latency_ns":      atomic.LoadInt64(&tm.maxLatencyNs),
		"max_conditions":      tm.maxConditions,
		"monitor_interval_ms": tm.monitorInterval.Milliseconds(),
	}
}
