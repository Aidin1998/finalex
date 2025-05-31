package orderqueue

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBasicQueueOperations(t *testing.T) {
	tempDir := t.TempDir()
	queue, err := NewBadgerQueue(tempDir)
	require.NoError(t, err)

	ctx := context.Background()
	defer queue.Shutdown(ctx)

	// Test basic enqueue/dequeue
	order := Order{
		ID:        "test-order-1",
		Priority:  1, // High priority (lower number = higher priority)
		Payload:   []byte(`{"symbol":"BTC-USD","side":"buy","amount":100}`),
		CreatedAt: time.Now(),
	}

	err = queue.Enqueue(ctx, order)
	assert.NoError(t, err)

	dequeued, err := queue.Dequeue(ctx)
	assert.NoError(t, err)
	assert.Equal(t, order.ID, dequeued.ID)
	assert.Equal(t, order.Priority, dequeued.Priority)
	assert.Equal(t, order.Payload, dequeued.Payload)

	// Test acknowledgment
	err = queue.Acknowledge(ctx, dequeued.ID)
	assert.NoError(t, err)

	// Queue should be empty now
	empty, err := queue.Dequeue(ctx)
	assert.Error(t, err)          // Should return error when empty
	assert.Equal(t, "", empty.ID) // Empty order
}

func TestPriorityOrdering(t *testing.T) {
	tempDir := t.TempDir()
	queue, err := NewBadgerQueue(tempDir)
	require.NoError(t, err)

	ctx := context.Background()
	defer queue.Shutdown(ctx)

	// Enqueue orders with different priorities (lower number = higher priority)
	orders := []Order{
		{ID: "low-1", Priority: 3, Payload: []byte("low"), CreatedAt: time.Now()},
		{ID: "high-1", Priority: 1, Payload: []byte("high"), CreatedAt: time.Now().Add(time.Millisecond)},
		{ID: "medium-1", Priority: 2, Payload: []byte("medium"), CreatedAt: time.Now().Add(2 * time.Millisecond)},
		{ID: "high-2", Priority: 1, Payload: []byte("high2"), CreatedAt: time.Now().Add(3 * time.Millisecond)},
	}

	for _, order := range orders {
		err := queue.Enqueue(ctx, order)
		assert.NoError(t, err)
	}

	// Should dequeue in priority order: high-1, high-2, medium-1, low-1
	expectedOrder := []string{"high-1", "high-2", "medium-1", "low-1"}

	for _, expectedID := range expectedOrder {
		order, err := queue.Dequeue(ctx)
		assert.NoError(t, err)
		assert.Equal(t, expectedID, order.ID)
	}
}

func TestRecoveryWithPendingOrders(t *testing.T) {
	tempDir := t.TempDir()

	ctx := context.Background()

	// Phase 1: Setup with pending orders
	queue, err := NewBadgerQueue(tempDir)
	require.NoError(t, err)

	orders := []Order{
		{ID: "replay-1", Priority: 1, Payload: []byte("test1"), CreatedAt: time.Now()},
		{ID: "replay-2", Priority: 2, Payload: []byte("test2"), CreatedAt: time.Now().Add(time.Millisecond)},
	}

	for _, order := range orders {
		err := queue.Enqueue(ctx, order)
		assert.NoError(t, err)
	}

	// Dequeue one order but don't acknowledge
	dequeued, err := queue.Dequeue(ctx)
	assert.NoError(t, err)
	assert.Equal(t, "replay-1", dequeued.ID)

	// Close queue
	queue.Shutdown(ctx)

	// Phase 2: Recovery
	queue, err = NewBadgerQueue(tempDir)
	require.NoError(t, err)
	defer queue.Shutdown(ctx)

	// Replay pending orders
	replayed, err := queue.ReplayPending(ctx)
	assert.NoError(t, err)
	assert.Len(t, replayed, 2) // Both orders should be replayed since first wasn't acknowledged

	// Orders should be in priority order
	assert.Equal(t, "replay-1", replayed[0].ID)
	assert.Equal(t, "replay-2", replayed[1].ID)
}
