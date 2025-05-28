package execution

import (
	"sync"
)

// OrderStatus represents the status of an order.
type OrderStatus int

const (
	New OrderStatus = iota
	Placed
	PartiallyFilled
	Filled
	Cancelled
)

// ExecutionOrder represents an order managed by the execution engine.
type ExecutionOrder struct {
	ID     string
	Price  float64
	Volume float64
	Filled float64
	Status OrderStatus
}

// ExecutionEngine manages the lifecycle of orders.
type ExecutionEngine struct {
	orders map[string]*ExecutionOrder
	mu     sync.RWMutex
}

// NewExecutionEngine creates a new execution engine.
func NewExecutionEngine() *ExecutionEngine {
	return &ExecutionEngine{
		orders: make(map[string]*ExecutionOrder),
	}
}

// PlaceOrder adds a new order to the engine.
func (e *ExecutionEngine) PlaceOrder(order *ExecutionOrder) {
	e.mu.Lock()
	defer e.mu.Unlock()
	order.Status = New
	e.orders[order.ID] = order
}

// ModifyOrder updates an existing order's price or volume.
func (e *ExecutionEngine) ModifyOrder(orderID string, price, volume float64) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	order, ok := e.orders[orderID]
	if !ok || order.Status == Filled || order.Status == Cancelled {
		return false
	}
	order.Price = price
	order.Volume = volume
	return true
}

// CancelOrder cancels an order if possible.
func (e *ExecutionEngine) CancelOrder(orderID string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	order, ok := e.orders[orderID]
	if !ok || order.Status == Filled || order.Status == Cancelled {
		return false
	}
	order.Status = Cancelled
	return true
}

// FillOrder processes a fill (partial or full) for an order.
func (e *ExecutionEngine) FillOrder(orderID string, fillVolume float64) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	order, ok := e.orders[orderID]
	if !ok || order.Status == Filled || order.Status == Cancelled {
		return false
	}
	order.Filled += fillVolume
	if order.Filled >= order.Volume {
		order.Status = Filled
		order.Filled = order.Volume
	} else {
		order.Status = PartiallyFilled
	}
	return true
}

// GetOrder returns an order by ID.
func (e *ExecutionEngine) GetOrder(orderID string) (*ExecutionOrder, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	order, ok := e.orders[orderID]
	return order, ok
}

// ListOrders returns all orders managed by the engine.
func (e *ExecutionEngine) ListOrders() []*ExecutionOrder {
	e.mu.RLock()
	defer e.mu.RUnlock()
	orders := make([]*ExecutionOrder, 0, len(e.orders))
	for _, o := range e.orders {
		orders = append(orders, o)
	}
	return orders
}
