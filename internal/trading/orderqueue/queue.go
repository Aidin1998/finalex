// Package orderqueue implements a disk-backed, persistent order queue
// supporting replay, priority recovery, and duplicate detection.
package orderqueue

import (
	"context"
	"time"
)

// Order represents a generic trading order to enqueue and process.
type Order struct {
	ID        string    // Unique order identifier
	CreatedAt time.Time // Enqueue timestamp
	Priority  int       // Lower values = higher priority
	Payload   []byte    // Raw order data (e.g., JSON)
}

// Queue defines the interface for a persistent order queue.
type Queue interface {
	// Enqueue adds an order to the queue and persists it.
	Enqueue(ctx context.Context, order Order) error

	// Dequeue fetches the next available order by priority for processing.
	Dequeue(ctx context.Context) (Order, error)

	// Acknowledge marks the order as processed and removes it from storage.
	Acknowledge(ctx context.Context, id string) error

	// ReplayPending loads unacknowledged orders (in priority order) for recovery.
	ReplayPending(ctx context.Context) ([]Order, error)

	// Shutdown gracefully shuts down the queue, ensuring all in-flight operations complete.
	Shutdown(ctx context.Context) error
}
