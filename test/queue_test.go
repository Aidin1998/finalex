package test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/orderqueue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBadgerQueue_BasicOperations(t *testing.T) {
	tempDir := t.TempDir()
	queue, err := orderqueue.orderqueue.NewBadgerQueue(tempDir)
	require.NoError(t, err)
	defer queue.Shutdown()

	// Test basic enqueue/dequeue
	order := &orderqueue.orderqueue.Order{
		ID:        "test-order-1",
		Priority:  orderqueue.orderqueue.HighPriority,
		Data:      []byte(`{"symbol":"BTC-USD","side":"buy","amount":100}`),
		Timestamp: time.Now(),
	}

	err = queue.Enqueue(order)
	assert.NoError(t, err)

	dequeued, err := queue.Dequeue()
	assert.NoError(t, err)
	assert.Equal(t, order.ID, dequeued.ID)
	assert.Equal(t, order.Priority, dequeued.Priority)
	assert.Equal(t, order.Data, dequeued.Data)

	// Test acknowledgment
	err = queue.Acknowledge(dequeued.ID)
	assert.NoError(t, err)

	// Queue should be empty now
	empty, err := queue.Dequeue()
	assert.NoError(t, err)
	assert.Nil(t, empty)
}

func TestBadgerQueue_PriorityOrdering(t *testing.T) {
	tempDir := t.TempDir()
	queue, err := orderqueue.NewBadgerQueue(tempDir)
	require.NoError(t, err)
	defer queue.Shutdown()

	// Enqueue orders with different priorities
	orders := []*orderqueue.Order{
		{ID: "low-1", Priority: orderqueue.LowPriority, Data: []byte("low"), Timestamp: time.Now()},
		{ID: "high-1", Priority: orderqueue.HighPriority, Data: []byte("high"), Timestamp: time.Now().Add(time.Millisecond)},
		{ID: "medium-1", Priority: orderqueue.MediumPriority, Data: []byte("medium"), Timestamp: time.Now().Add(2 * time.Millisecond)},
		{ID: "high-2", Priority: orderqueue.HighPriority, Data: []byte("high2"), Timestamp: time.Now().Add(3 * time.Millisecond)},
	}

	for _, order := range orders {
		err := queue.Enqueue(order)
		assert.NoError(t, err)
	}

	// Should dequeue in priority order: high-1, high-2, medium-1, low-1
	expectedOrder := []string{"high-1", "high-2", "medium-1", "low-1"}
	
	for _, expectedID := range expectedOrder {
		order, err := queue.Dequeue()
		assert.NoError(t, err)
		assert.NotNil(t, order)
		assert.Equal(t, expectedID, order.ID)
	}
}

func TestBadgerQueue_DuplicateDetection(t *testing.T) {
	tempDir := t.TempDir()
	queue, err := orderqueue.NewBadgerQueue(tempDir)
	require.NoError(t, err)
	defer queue.Shutdown()

	order := &orderqueue.Order{
		ID:        "duplicate-test",
		Priority:  orderqueue.MediumPriority,
		Data:      []byte("test"),
		Timestamp: time.Now(),
	}

	// First enqueue should succeed
	err = queue.Enqueue(order)
	assert.NoError(t, err)

	// Second enqueue with same ID should fail
	err = queue.Enqueue(order)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestBadgerQueue_ReplayPending(t *testing.T) {
	tempDir := t.TempDir()
	queue, err := orderqueue.NewBadgerQueue(tempDir)
	require.NoError(t, err)

	// Enqueue some orders
	orders := []*orderqueue.Order{
		{ID: "replay-1", Priority: orderqueue.HighPriority, Data: []byte("test1"), Timestamp: time.Now()},
		{ID: "replay-2", Priority: orderqueue.MediumPriority, Data: []byte("test2"), Timestamp: time.Now().Add(time.Millisecond)},
	}

	for _, order := range orders {
		err := queue.Enqueue(order)
		assert.NoError(t, err)
	}

	// Dequeue one order but don't acknowledge
	dequeued, err := queue.Dequeue()
	assert.NoError(t, err)
	assert.Equal(t, "replay-1", dequeued.ID)

	// Close and reopen queue
	queue.Shutdown()
	
	queue, err = orderqueue.NewBadgerQueue(tempDir)
	require.NoError(t, err)
	defer queue.Shutdown()

	// Replay pending orders
	replayed, err := queue.ReplayPending()
	assert.NoError(t, err)
	assert.Len(t, replayed, 2) // Both orders should be replayed since first wasn't acknowledged

	// Orders should be in priority order
	assert.Equal(t, "replay-1", replayed[0].ID)
	assert.Equal(t, "replay-2", replayed[1].ID)
}

func TestBadgerQueue_ConcurrentOperations(t *testing.T) {
	tempDir := t.TempDir()
	queue, err := orderqueue.NewBadgerQueue(tempDir)
	require.NoError(t, err)
	defer queue.Shutdown()

	const numGoroutines = 10
	const ordersPerGoroutine = 100
	
	var wg sync.WaitGroup
	wg.Add(numGoroutines * 2) // producers + consumers

	// Start producers
	for i := 0; i < numGoroutines; i++ {
		go func(producerID int) {
			defer wg.Done()
			for j := 0; j < ordersPerGoroutine; j++ {
				order := &orderqueue.Order{
					ID:        fmt.Sprintf("prod-%d-order-%d", producerID, j),
					Priority:  Priority(j % 3), // Distribute across priorities
					Data:      []byte(fmt.Sprintf("data-%d-%d", producerID, j)),
					Timestamp: time.Now(),
				}
				
				err := queue.Enqueue(order)
				assert.NoError(t, err)
			}
		}(i)
	}

	// Start consumers
	processedOrders := make(map[string]bool)
	var processedMutex sync.Mutex
	
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < ordersPerGoroutine; j++ {
				order, err := queue.Dequeue()
				if err != nil || order == nil {
					time.Sleep(10 * time.Millisecond)
					j-- // Retry
					continue
				}
				
				processedMutex.Lock()
				processedOrders[order.ID] = true
				processedMutex.Unlock()
				
				err = queue.Acknowledge(order.ID)
				assert.NoError(t, err)
			}
		}()
	}

	wg.Wait()

	// Verify all orders were processed
	assert.Len(t, processedOrders, numGoroutines*ordersPerGoroutine)
}

func BenchmarkBadgerQueue_Enqueue(b *testing.B) {
	tempDir := b.TempDir()
	queue, err := orderqueue.NewBadgerQueue(tempDir)
	require.NoError(b, err)
	defer queue.Shutdown()

	orders := make([]*Order, b.N)
	for i := 0; i < b.N; i++ {
		orders[i] = &orderqueue.Order{
			ID:        fmt.Sprintf("bench-order-%d", i),
			Priority:  Priority(i % 3),
			Data:      []byte(fmt.Sprintf("benchmark-data-%d", i)),
			Timestamp: time.Now(),
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		err := queue.Enqueue(orders[i])
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBadgerQueue_Dequeue(b *testing.B) {
	tempDir := b.TempDir()
	queue, err := orderqueue.NewBadgerQueue(tempDir)
	require.NoError(b, err)
	defer queue.Shutdown()

	// Pre-populate queue
	for i := 0; i < b.N; i++ {
		order := &orderqueue.Order{
			ID:        fmt.Sprintf("bench-order-%d", i),
			Priority:  Priority(i % 3),
			Data:      []byte(fmt.Sprintf("benchmark-data-%d", i)),
			Timestamp: time.Now(),
		}
		err := queue.Enqueue(order)
		require.NoError(b, err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		order, err := queue.Dequeue()
		if err != nil || order == nil {
			b.Fatal("Failed to dequeue")
		}
	}
}

func BenchmarkBadgerQueue_EnqueueDequeue(b *testing.B) {
	tempDir := b.TempDir()
	queue, err := orderqueue.NewBadgerQueue(tempDir)
	require.NoError(b, err)
	defer queue.Shutdown()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		order := &orderqueue.Order{
			ID:        fmt.Sprintf("bench-order-%d", i),
			Priority:  Priority(i % 3),
			Data:      []byte(fmt.Sprintf("benchmark-data-%d", i)),
			Timestamp: time.Now(),
		}
		
		err := queue.Enqueue(order)
		if err != nil {
			b.Fatal(err)
		}
		
		dequeued, err := queue.Dequeue()
		if err != nil || dequeued == nil {
			b.Fatal("Failed to dequeue")
		}
		
		err = queue.Acknowledge(dequeued.ID)
		if err != nil {
			b.Fatal(err)
		}
	}
}
