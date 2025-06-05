// Package orderqueue implements a queue system for orders
package orderqueue

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// QueueProvider represents the interface for order queue providers
type QueueProvider interface {
	Enqueue(ctx context.Context, order Order) error
	Dequeue(ctx context.Context) (Order, error)
	Peek(ctx context.Context) (Order, bool, error)
	Size(ctx context.Context) (int, error)
	Close() error
}

// Order represents a generic trading order to enqueue and process
type Order struct {
	ID          string
	UserID      string
	Market      string
	Side        string
	Type        string
	Price       string
	Amount      string
	Status      string
	Priority    int
	CreatedAt   time.Time
	ExpiresAt   *time.Time
	ComplexData map[string]interface{} `json:"complex_data,omitempty"`
}

// InMemoryQueue provides an in-memory implementation of QueueProvider
type InMemoryQueue struct {
	orders     []Order
	mu         sync.RWMutex
	closed     bool
	closedLock sync.RWMutex
}

// NewInMemoryQueue creates a new in-memory queue
func NewInMemoryQueue() *InMemoryQueue {
	return &InMemoryQueue{
		orders: make([]Order, 0),
	}
}

// Enqueue adds an order to the queue
func (q *InMemoryQueue) Enqueue(ctx context.Context, order Order) error {
	q.closedLock.RLock()
	if q.closed {
		q.closedLock.RUnlock()
		return errors.New("queue is closed")
	}
	q.closedLock.RUnlock()

	// Ensure the order has an ID
	if order.ID == "" {
		order.ID = uuid.New().String()
	}

	// Set creation time if not set
	if order.CreatedAt.IsZero() {
		order.CreatedAt = time.Now()
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	// Check for duplicates
	for _, o := range q.orders {
		if o.ID == order.ID {
			return fmt.Errorf("order duplicate: %s", order.ID)
		}
	}

	// Add the order and sort by priority (higher priority first, then by creation time)
	q.orders = append(q.orders, order)
	sortOrdersByPriority(q.orders)

	return nil
}

// Dequeue removes and returns the highest priority order
func (q *InMemoryQueue) Dequeue(ctx context.Context) (Order, error) {
	q.closedLock.RLock()
	if q.closed {
		q.closedLock.RUnlock()
		return Order{}, errors.New("queue is closed")
	}
	q.closedLock.RUnlock()

	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.orders) == 0 {
		return Order{}, errors.New("queue is empty")
	}

	// Get the highest priority order (already sorted)
	order := q.orders[0]
	q.orders = q.orders[1:]

	return order, nil
}

// Peek returns the highest priority order without removing it
func (q *InMemoryQueue) Peek(ctx context.Context) (Order, bool, error) {
	q.closedLock.RLock()
	if q.closed {
		q.closedLock.RUnlock()
		return Order{}, false, errors.New("queue is closed")
	}
	q.closedLock.RUnlock()

	q.mu.RLock()
	defer q.mu.RUnlock()

	if len(q.orders) == 0 {
		return Order{}, false, nil
	}

	return q.orders[0], true, nil
}

// Size returns the number of orders in the queue
func (q *InMemoryQueue) Size(ctx context.Context) (int, error) {
	q.closedLock.RLock()
	if q.closed {
		q.closedLock.RUnlock()
		return 0, errors.New("queue is closed")
	}
	q.closedLock.RUnlock()

	q.mu.RLock()
	defer q.mu.RUnlock()

	return len(q.orders), nil
}

// Close closes the queue
func (q *InMemoryQueue) Close() error {
	q.closedLock.Lock()
	defer q.closedLock.Unlock()

	q.closed = true
	return nil
}

// sortOrdersByPriority sorts orders by priority (higher first) and then by creation time
func sortOrdersByPriority(orders []Order) {
	// Simple insertion sort for a small number of orders
	for i := 1; i < len(orders); i++ {
		j := i
		for j > 0 {
			// Higher priority first
			if orders[j-1].Priority < orders[j].Priority {
				orders[j-1], orders[j] = orders[j], orders[j-1]
			} else if orders[j-1].Priority == orders[j].Priority {
				// Same priority, older orders first
				if orders[j-1].CreatedAt.After(orders[j].CreatedAt) {
					orders[j-1], orders[j] = orders[j], orders[j-1]
				} else {
					break
				}
			} else {
				break
			}
			j--
		}
	}
}
