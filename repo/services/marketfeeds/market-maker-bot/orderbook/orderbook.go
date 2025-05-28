package orderbook

import (
	"fmt"
	"sync"
)

// OrderBook manages orders using both a HashMap and a RadixTree for fast lookup and range queries.
type OrderBook struct {
	Orders    map[string]*Order // OrderID -> Order
	RadixTree *RadixTree
	mu        sync.RWMutex
}

// NewOrderBook creates a new hybrid order book.
func NewOrderBook() *OrderBook {
	return &OrderBook{
		Orders:    make(map[string]*Order),
		RadixTree: NewRadixTree(),
	}
}

// AddOrUpdateOrder adds or updates an order in the book.
func (ob *OrderBook) AddOrUpdateOrder(order *Order) {
	ob.mu.Lock()
	defer ob.mu.Unlock()
	ob.Orders[order.ID] = order
	key := ob.priceKey(order.Price)
	ob.RadixTree.Insert(order.ID, key)
}

// RemoveOrder removes an order from the book.
func (ob *OrderBook) RemoveOrder(orderID string) {
	ob.mu.Lock()
	defer ob.mu.Unlock()
	order, ok := ob.Orders[orderID]
	if !ok {
		return
	}
	key := ob.priceKey(order.Price)
	ob.RadixTree.Delete(key)
	delete(ob.Orders, orderID)
}

// GetOrderByID returns an order by its ID.
func (ob *OrderBook) GetOrderByID(orderID string) (*Order, bool) {
	ob.mu.RLock()
	defer ob.mu.RUnlock()
	order, ok := ob.Orders[orderID]
	return order, ok
}

// GetOrderIDAtPrice returns an order ID at a given price level (if any).
func (ob *OrderBook) GetOrderIDAtPrice(price float64) (string, bool) {
	ob.mu.RLock()
	defer ob.mu.RUnlock()
	key := ob.priceKey(price)
	return ob.RadixTree.Query(key)
}

// priceKey generates a key for the radix tree based on price.
func (ob *OrderBook) priceKey(price float64) string {
	return fmt.Sprintf("%.8f", price)
}

// ApplyWebSocketUpdate applies a real-time update from a WebSocket stream.
func (ob *OrderBook) ApplyWebSocketUpdate(update Order) {
	ob.AddOrUpdateOrder(&update)
}
