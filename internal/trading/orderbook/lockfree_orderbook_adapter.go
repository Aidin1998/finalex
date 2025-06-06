// =============================
// Lock-Free Order Book Adapter
// =============================
// This file provides an adapter for the lock-free order book implementation.
// It allows the lock-free order book to be used via the OrderBookInterface.

package orderbook

import (
	"context"
	"fmt"
	"sync"

	"github.com/Aidin1998/finalex/internal/trading/model"
	"github.com/google/uuid"
)

// LockFreeOrderBookAdapter adapts LockFreeOrderBook to OrderBookInterface.
type LockFreeOrderBookAdapter struct {
	lfob *LockFreeOrderBook
}

func NewLockFreeOrderBookAdapter(pair string) *LockFreeOrderBookAdapter {
	return &LockFreeOrderBookAdapter{
		lfob: NewLockFreeOrderBook(pair),
	}
}

func (a *LockFreeOrderBookAdapter) AddOrder(order *model.Order) (*AddOrderResult, error) {
	return a.lfob.AddOrder(order)
}

func (a *LockFreeOrderBookAdapter) CancelOrder(orderID uuid.UUID) error {
	// TODO: Implement lock-free CancelOrder logic in LockFreeOrderBook
	return fmt.Errorf("CancelOrder not implemented for lock-free order book")
}

func (a *LockFreeOrderBookAdapter) GetSnapshot(depth int) ([][]string, [][]string) {
	return a.lfob.GetSnapshot(depth)
}

func (a *LockFreeOrderBookAdapter) ProcessLimitOrder(ctx context.Context, order *model.Order) (*model.Order, []*model.Trade, []*model.Order, error) {
	result, err := a.lfob.AddOrder(order)
	return order, result.Trades, result.RestingOrders, err
}

func (a *LockFreeOrderBookAdapter) ProcessMarketOrder(ctx context.Context, order *model.Order) (*model.Order, []*model.Trade, []*model.Order, error) {
	result, err := a.lfob.AddOrder(order)
	return order, result.Trades, nil, err
}

func (a *LockFreeOrderBookAdapter) CanFullyFill(order *model.Order) bool {
	// TODO: Implement real logic. For now, always return false (conservative)
	return false
}

func (a *LockFreeOrderBookAdapter) OrdersCount() int {
	// TODO: Implement real logic. For now, return 0
	return 0
}

func (a *LockFreeOrderBookAdapter) GetOrderIDs() []uuid.UUID {
	// TODO: Implement real logic. For now, return empty slice
	return nil
}

func (a *LockFreeOrderBookAdapter) GetMutex() *sync.RWMutex {
	// Lock-free: return a dummy mutex for compatibility
	return &sync.RWMutex{}
}

// Admin controls: no-ops for lock-free implementation
func (a *LockFreeOrderBookAdapter) HaltTrading()           {}
func (a *LockFreeOrderBookAdapter) ResumeTrading()         {}
func (a *LockFreeOrderBookAdapter) TriggerCircuitBreaker() {}
func (a *LockFreeOrderBookAdapter) EmergencyShutdown()     {}
