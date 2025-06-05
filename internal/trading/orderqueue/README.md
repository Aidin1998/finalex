# Order Queue Module

This directory contains the implementation of the Order Queue subsystem for the trading module.

## Active Implementation

- `inmemory_queue.go` - The current active implementation using in-memory storage with the `QueueProvider` interface.

## Deprecated Implementations

The following files are kept for historical purposes only and will be removed in a future update:

- `queue.go` - Contains original interfaces marked as deprecated.
- `badger_queue.go.legacy` - Legacy implementation using BadgerDB.
- `enhanced_queue.go.legacy` - Legacy enhanced queue implementation.
- `snapshot_store.go.legacy` - Legacy snapshot store implementation.
- `failover.go.legacy` - Legacy failover implementation.

## Migration Guide

To migrate from the old implementation to the new one:

1. Use `NewInMemoryQueue()` to create a new queue instance.
2. Use the `QueueProvider` interface methods for all queue operations.
3. Use the new `Order` struct from `inmemory_queue.go` in place of the old one.

Example:

```go
// Old implementation
oldQueue, _ := NewBadgerQueue("/path/to/db")
oldOrder := Order{ID: "1", CreatedAt: time.Now(), Priority: 1, Payload: []byte("data")}
oldQueue.Enqueue(ctx, oldOrder)

// New implementation
newQueue := NewInMemoryQueue()
newOrder := Order{
    ID:        "1",
    UserID:    "user1",
    Market:    "BTC/USD",
    Side:      "buy",
    Type:      "limit",
    Price:     "10000.00",
    Amount:    "1.0",
    Status:    "pending",
    Priority:  1,
    CreatedAt: time.Now(),
}
newQueue.Enqueue(ctx, newOrder)
```

Please update all code to use the new implementation as the old implementations will be removed entirely in the future.
