//go:build test
// +build test

// filepath: c:\Orbit CEX\pincex_unified\internal\trading\inmemory_repository.go
package trading

import (
	"context"
	"sync"
	"time"

	"cex/internal/spot/model"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type InMemoryRepository struct {
	orders map[uuid.UUID]*model.Order
	trades map[uuid.UUID]*model.Trade
	mu     sync.RWMutex
}

func NewInMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{
		orders: make(map[uuid.UUID]*model.Order),
		trades: make(map[uuid.UUID]*model.Trade),
	}
}

func (r *InMemoryRepository) CreateOrder(ctx context.Context, order *model.Order) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.orders[order.ID] = order
	return nil
}

func (r *InMemoryRepository) GetOrderByID(ctx context.Context, orderID uuid.UUID) (*model.Order, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	order, ok := r.orders[orderID]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	return order, nil
}

func (r *InMemoryRepository) GetOpenOrdersByPair(ctx context.Context, pair string) ([]*model.Order, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []*model.Order
	for _, o := range r.orders {
		if o.Symbol == pair && o.Status == "open" {
			result = append(result, o)
		}
	}
	return result, nil
}

func (r *InMemoryRepository) GetOpenOrdersByUser(ctx context.Context, userID uuid.UUID) ([]*model.Order, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []*model.Order
	for _, o := range r.orders {
		if o.UserID == userID && o.Status == "open" {
			result = append(result, o)
		}
	}
	return result, nil
}

func (r *InMemoryRepository) UpdateOrderStatus(ctx context.Context, orderID uuid.UUID, status string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if o, ok := r.orders[orderID]; ok {
		o.Status = status
		return nil
	}
	return gorm.ErrRecordNotFound
}

func (r *InMemoryRepository) UpdateOrderStatusAndFilledQuantity(ctx context.Context, orderID uuid.UUID, status string, filledQty, avgPrice decimal.Decimal) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if o, ok := r.orders[orderID]; ok {
		o.Status = status
		o.FilledQuantity = filledQty
		o.AvgPrice = avgPrice
		return nil
	}
	return gorm.ErrRecordNotFound
}

func (r *InMemoryRepository) CancelOrder(ctx context.Context, orderID uuid.UUID) error {
	return r.UpdateOrderStatus(ctx, orderID, "cancelled")
}

func (r *InMemoryRepository) ConvertStopOrderToLimit(ctx context.Context, order *model.Order) error {
	return nil
}

func (r *InMemoryRepository) CreateTrade(ctx context.Context, trade *model.Trade) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.trades[trade.ID] = trade
	return nil
}

func (r *InMemoryRepository) CreateTradeTx(ctx context.Context, tx *gorm.DB, trade *model.Trade) error {
	return r.CreateTrade(ctx, trade)
}

func (r *InMemoryRepository) UpdateOrderStatusTx(ctx context.Context, tx *gorm.DB, orderID uuid.UUID, status string, filledQty, avgPrice decimal.Decimal) error {
	return r.UpdateOrderStatusAndFilledQuantity(ctx, orderID, status, filledQty, avgPrice)
}

func (r *InMemoryRepository) BatchUpdateOrderStatusTx(ctx context.Context, tx *gorm.DB, updates []struct {
	OrderID uuid.UUID
	Status  string
}) error {
	for _, u := range updates {
		_ = r.UpdateOrderStatus(ctx, u.OrderID, u.Status)
	}
	return nil
}

func (r *InMemoryRepository) ExecuteInTransaction(ctx context.Context, txFunc func(*gorm.DB) error) error {
	return txFunc(nil)
}

func (r *InMemoryRepository) UpdateOrderHoldID(ctx context.Context, orderID uuid.UUID, holdID string) error {
	return nil
}

func (r *InMemoryRepository) GetExpiredGTDOrders(ctx context.Context, pair string, now time.Time) ([]*model.Order, error) {
	return nil, nil
}

func (r *InMemoryRepository) GetOCOSiblingOrder(ctx context.Context, ocoGroupID uuid.UUID, excludeOrderID uuid.UUID) (*model.Order, error) {
	return nil, nil
}

func (r *InMemoryRepository) GetOpenTrailingStopOrders(ctx context.Context, pair string) ([]*model.Order, error) {
	return nil, nil
}
