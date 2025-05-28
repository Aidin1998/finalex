package orderbook

import (
	"sync"
)

// Order represents a single order in the order book.
type Order struct {
	ID     string
	Price  float64
	Volume float64
}

// AggregatedData holds the aggregated order book data.
type AggregatedData struct {
	TotalVolume float64
	Depth       map[float64]float64 // Price to volume mapping
}

// Aggregator is responsible for aggregating order book data.
type Aggregator struct {
	mu      sync.RWMutex
	orders  map[string]*Order
	aggData AggregatedData
}

// NewAggregator creates a new instance of Aggregator.
func NewAggregator() *Aggregator {
	return &Aggregator{
		orders: make(map[string]*Order),
		aggData: AggregatedData{
			Depth: make(map[float64]float64),
		},
	}
}

// AddOrder adds a new order to the aggregator and updates the aggregated data.
func (a *Aggregator) AddOrder(order *Order) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.orders[order.ID] = order
	a.aggData.TotalVolume += order.Volume
	a.aggData.Depth[order.Price] += order.Volume
}

// RemoveOrder removes an order from the aggregator and updates the aggregated data.
func (a *Aggregator) RemoveOrder(orderID string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	order, exists := a.orders[orderID]
	if exists {
		a.aggData.TotalVolume -= order.Volume
		a.aggData.Depth[order.Price] -= order.Volume
		if a.aggData.Depth[order.Price] == 0 {
			delete(a.aggData.Depth, order.Price)
		}
		delete(a.orders, orderID)
	}
}

// GetAggregatedData returns the current aggregated order book data.
func (a *Aggregator) GetAggregatedData() AggregatedData {
	a.mu.RLock()
	defer a.mu.RUnlock()

	return a.aggData
}