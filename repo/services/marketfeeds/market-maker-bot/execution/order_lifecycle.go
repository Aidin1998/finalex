package execution

import (
	"errors"
	"sync"
)

// Order represents a market order with its details.
type Order struct {
	ID     string
	Status string
	Amount float64
	Price  float64
}

// OrderLifecycle manages the lifecycle of orders.
type OrderLifecycle struct {
	mu     sync.Mutex
	orders map[string]*Order
}

// NewOrderLifecycle initializes a new OrderLifecycle.
func NewOrderLifecycle() *OrderLifecycle {
	return &OrderLifecycle{
		orders: make(map[string]*Order),
	}
}

// CreateOrder creates a new order and adds it to the lifecycle.
func (ol *OrderLifecycle) CreateOrder(id string, amount float64, price float64) *Order {
	ol.mu.Lock()
	defer ol.mu.Unlock()

	order := &Order{
		ID:     id,
		Status: "created",
		Amount: amount,
		Price:  price,
	}
	ol.orders[id] = order
	return order
}

// ModifyOrder updates the details of an existing order.
func (ol *OrderLifecycle) ModifyOrder(id string, amount float64, price float64) error {
	ol.mu.Lock()
	defer ol.mu.Unlock()

	order, exists := ol.orders[id]
	if !exists {
		return errors.New("order not found")
	}

	order.Amount = amount
	order.Price = price
	order.Status = "modified"
	return nil
}

// CancelOrder cancels an existing order.
func (ol *OrderLifecycle) CancelOrder(id string) error {
	ol.mu.Lock()
	defer ol.mu.Unlock()

	order, exists := ol.orders[id]
	if !exists {
		return errors.New("order not found")
	}

	order.Status = "canceled"
	return nil
}

// GetOrder retrieves an order by its ID.
func (ol *OrderLifecycle) GetOrder(id string) (*Order, error) {
	ol.mu.Lock()
	defer ol.mu.Unlock()

	order, exists := ol.orders[id]
	if !exists {
		return nil, errors.New("order not found")
	}
	return order, nil
}