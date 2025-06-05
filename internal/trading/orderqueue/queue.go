// Package orderqueue implements order queue interfaces and implementations
// for trading system order processing.
package orderqueue

import (
	"context"
	"time"
)

// DEPRECATED: This file contains deprecated types.
// Please use the interfaces and implementations in inmemory_queue.go instead.

// LegacyOrder represents a generic trading order to enqueue and process.
// DEPRECATED: Use Order from inmemory_queue.go instead.
type LegacyOrder struct {
	ID        string    // Unique order identifier
	CreatedAt time.Time // Enqueue timestamp
	Priority  int       // Lower values = higher priority
	Payload   []byte    // Raw order data (e.g., JSON)
}

// LegacyQueue defines the interface for a persistent order queue.
// DEPRECATED: Use QueueProvider from inmemory_queue.go instead.
type LegacyQueue interface {
	// Enqueue adds an order to the queue and persists it.
	Enqueue(ctx context.Context, order LegacyOrder) error

	// Dequeue fetches the next available order by priority for processing.
	Dequeue(ctx context.Context) (LegacyOrder, error)

	// Acknowledge marks the order as processed and removes it from storage.
	Acknowledge(ctx context.Context, id string) error

	// ReplayPending loads unacknowledged orders (in priority order) for recovery.
	ReplayPending(ctx context.Context) ([]LegacyOrder, error)

	// Shutdown gracefully shuts down the queue, ensuring all in-flight operations complete.
	Shutdown(ctx context.Context) error
}
