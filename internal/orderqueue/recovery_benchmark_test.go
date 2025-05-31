package orderqueue

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// RecoveryBenchmark measures recovery performance under various conditions
type RecoveryBenchmark struct {
	Name              string
	OrderCount        int
	PendingRatio      float64 // Ratio of orders left pending
	PriorityDistrib   []float64 // Distribution across priorities [low, medium, high]
	DataSize          int // Size of order data in bytes
}

func BenchmarkRecovery_SmallDataset(b *testing.B) {
	benchmark := RecoveryBenchmark{
		Name:            "Small Dataset",
		OrderCount:      1000,
		PendingRatio:    0.2, // 20% pending
		PriorityDistrib: []float64{0.3, 0.5, 0.2}, // 30% low, 50% medium, 20% high
		DataSize:        256,
	}
	
	runRecoveryBenchmark(b, benchmark)
}

func BenchmarkRecovery_MediumDataset(b *testing.B) {
	benchmark := RecoveryBenchmark{
		Name:            "Medium Dataset",
		OrderCount:      10000,
		PendingRatio:    0.15,
		PriorityDistrib: []float64{0.4, 0.4, 0.2},
		DataSize:        512,
	}
	
	runRecoveryBenchmark(b, benchmark)
}

func BenchmarkRecovery_LargeDataset(b *testing.B) {
	benchmark := RecoveryBenchmark{
		Name:            "Large Dataset",
		OrderCount:      100000,
		PendingRatio:    0.1,
		PriorityDistrib: []float64{0.5, 0.3, 0.2},
		DataSize:        1024,
	}
	
	runRecoveryBenchmark(b, benchmark)
}

func BenchmarkRecovery_HighPendingRatio(b *testing.B) {
	benchmark := RecoveryBenchmark{
		Name:            "High Pending Ratio",
		OrderCount:      50000,
		PendingRatio:    0.8, // 80% pending - worst case scenario
		PriorityDistrib: []float64{0.33, 0.33, 0.34},
		DataSize:        512,
	}
	
	runRecoveryBenchmark(b, benchmark)
}

func runRecoveryBenchmark(b *testing.B, benchmark RecoveryBenchmark) {
	for i := 0; i < b.N; i++ {
		tempDir := b.TempDir()
		
		// Setup phase: create queue and populate with orders
		setupStart := time.Now()
		
		queue, err := NewBadgerQueue(tempDir)
		require.NoError(b, err)
		
		// Generate and enqueue orders
		orders := generateTestOrders(benchmark)
		for _, order := range orders {
			err := queue.Enqueue(order)
			require.NoError(b, err)
		}
		
		// Simulate processing (dequeue some orders, acknowledge some)
		processOrders(queue, orders, benchmark.PendingRatio)
		
		setupTime := time.Since(setupStart)
		queue.Shutdown()
		
		// Recovery phase: measure recovery time
		b.ResetTimer()
		recoveryStart := time.Now()
		
		// Reopen queue and replay pending orders
		recoveredQueue, err := NewBadgerQueue(tempDir)
		require.NoError(b, err)
		
		pendingOrders, err := recoveredQueue.ReplayPending()
		require.NoError(b, err)
		
		recoveryTime := time.Since(recoveryStart)
		b.StopTimer()
		
		recoveredQueue.Shutdown()
		
		// Validate recovery requirements
		expectedPending := int(float64(benchmark.OrderCount) * benchmark.PendingRatio)
		actualPending := len(pendingOrders)
		
		tolerance := int(float64(expectedPending) * 0.1) // 10% tolerance
		if abs(actualPending-expectedPending) > tolerance {
			b.Errorf("Recovery validation failed: expected ~%d pending orders, got %d", 
				expectedPending, actualPending)
		}
		
		// Check 30-second recovery requirement
		if recoveryTime > 30*time.Second {
			b.Errorf("Recovery time %v exceeds 30-second requirement", recoveryTime)
		}
		
		b.ReportMetric(float64(recoveryTime.Milliseconds()), "recovery_ms")
		b.ReportMetric(float64(actualPending), "pending_orders")
		b.ReportMetric(float64(setupTime.Milliseconds()), "setup_ms")
		
		b.Logf("Benchmark: %s - Orders: %d, Pending: %d, Recovery: %v, Setup: %v",
			benchmark.Name, benchmark.OrderCount, actualPending, recoveryTime, setupTime)
	}
}

func generateTestOrders(benchmark RecoveryBenchmark) []*Order {
	orders := make([]*Order, benchmark.OrderCount)
	
	for i := 0; i < benchmark.OrderCount; i++ {
		// Determine priority based on distribution
		priority := determinePriority(benchmark.PriorityDistrib, rand.Float64())
		
		orders[i] = &Order{
			ID:        fmt.Sprintf("bench-order-%d", i),
			Priority:  priority,
			Data:      make([]byte, benchmark.DataSize),
			Timestamp: time.Now().Add(time.Duration(i) * time.Microsecond),
		}
		
		// Fill data with some pattern for validation
		for j := range orders[i].Data {
			orders[i].Data[j] = byte(i % 256)
		}
	}
	
	return orders
}

func determinePriority(distribution []float64, random float64) Priority {
	cumulative := 0.0
	for i, prob := range distribution {
		cumulative += prob
		if random <= cumulative {
			return Priority(i)
		}
	}
	return HighPriority // fallback
}

func processOrders(queue Queue, orders []*Order, pendingRatio float64) {
	processCount := int(float64(len(orders)) * (1.0 - pendingRatio))
	
	for i := 0; i < processCount; i++ {
		order, err := queue.Dequeue()
		if err != nil || order == nil {
			break
		}
		
		// Acknowledge the order (some orders will remain pending)
		queue.Acknowledge(order.ID)
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// Load testing during recovery scenarios
func TestLoadDuringRecovery(t *testing.T) {
	tempDir := t.TempDir()
	
	// Phase 1: Setup initial state with pending orders
	initialQueue, err := NewBadgerQueue(tempDir)
	require.NoError(t, err)
	
	const initialOrders = 10000
	for i := 0; i < initialOrders; i++ {
		order := &Order{
			ID:        fmt.Sprintf("initial-order-%d", i),
			Priority:  Priority(i % 3),
			Data:      []byte(fmt.Sprintf("initial-data-%d", i)),
			Timestamp: time.Now(),
		}
		err := initialQueue.Enqueue(order)
		require.NoError(t, err)
	}
	
	// Process some orders, leave some pending
	for i := 0; i < initialOrders/2; i++ {
		order, err := initialQueue.Dequeue()
		require.NoError(t, err)
		if i%3 != 0 { // Don't acknowledge every 3rd order
			initialQueue.Acknowledge(order.ID)
		}
	}
	
	initialQueue.Shutdown()
	
	// Phase 2: Recovery with concurrent load
	recoveryStart := time.Now()
	
	recoveredQueue, err := NewBadgerQueue(tempDir)
	require.NoError(t, err)
	defer recoveredQueue.Shutdown()
	
	// Start concurrent load while recovery is happening
	var newOrdersEnqueued int64
	var newOrdersProcessed int64
	
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	var wg sync.WaitGroup
	
	// Replay pending orders first
	pendingOrders, err := recoveredQueue.ReplayPending()
	require.NoError(t, err)
	recoveryTime := time.Since(recoveryStart)
	
	t.Logf("Recovery completed in %v, replayed %d orders", recoveryTime, len(pendingOrders))
	
	// Producer of new orders during recovery
	wg.Add(1)
	go func() {
		defer wg.Done()
		orderID := initialOrders
		ticker := time.NewTicker(time.Millisecond)
		defer ticker.Stop()
		
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				order := &Order{
					ID:        fmt.Sprintf("recovery-load-order-%d", orderID),
					Priority:  Priority(orderID % 3),
					Data:      []byte(fmt.Sprintf("recovery-data-%d", orderID)),
					Timestamp: time.Now(),
				}
				
				if err := recoveredQueue.Enqueue(order); err == nil {
					atomic.AddInt64(&newOrdersEnqueued, 1)
				}
				orderID++
			}
		}
	}()
	
	// Consumer processing all orders (pending + new)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			default:
				order, err := recoveredQueue.Dequeue()
				if err != nil || order == nil {
					time.Sleep(time.Millisecond)
					continue
				}
				
				// Simulate processing
				time.Sleep(100 * time.Microsecond)
				
				if err := recoveredQueue.Acknowledge(order.ID); err == nil {
					atomic.AddInt64(&newOrdersProcessed, 1)
				}
			}
		}
	}()
	
	<-ctx.Done()
	wg.Wait()
	
	finalEnqueued := atomic.LoadInt64(&newOrdersEnqueued)
	finalProcessed := atomic.LoadInt64(&newOrdersProcessed)
	
	t.Logf("Load during recovery - New orders: %d, Processed: %d, Recovery time: %v",
		finalEnqueued, finalProcessed, recoveryTime)
	
	// Validate performance requirements
	assert.Less(t, recoveryTime.Seconds(), 30.0, 
		"Recovery with concurrent load should complete within 30 seconds")
	assert.Greater(t, finalProcessed, int64(len(pendingOrders)), 
		"Should process at least all pending orders")
}

// Benchmark concurrent operations during recovery
func BenchmarkConcurrentRecovery(b *testing.B) {
	for i := 0; i < b.N; i++ {
		tempDir := b.TempDir()
		
		// Setup phase
		setupQueue, err := NewBadgerQueue(tempDir)
		require.NoError(b, err)
		
		const setupOrders = 5000
		for j := 0; j < setupOrders; j++ {
			order := &Order{
				ID:        fmt.Sprintf("concurrent-setup-%d", j),
				Priority:  Priority(j % 3),
				Data:      []byte(fmt.Sprintf("data-%d", j)),
				Timestamp: time.Now(),
			}
			setupQueue.Enqueue(order)
		}
		
		// Leave some orders pending
		for j := 0; j < setupOrders/3; j++ {
			order, _ := setupQueue.Dequeue()
			if order != nil && j%2 == 0 {
				setupQueue.Acknowledge(order.ID)
			}
		}
		
		setupQueue.Shutdown()
		
		// Benchmark recovery with concurrent operations
		b.ResetTimer()
		
		recoveryQueue, err := NewBadgerQueue(tempDir)
		require.NoError(b, err)
		
		var wg sync.WaitGroup
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		
		// Recovery
		wg.Add(1)
		go func() {
			defer wg.Done()
			pendingOrders, _ := recoveryQueue.ReplayPending()
			b.ReportMetric(float64(len(pendingOrders)), "pending_orders")
		}()
		
		// Concurrent enqueue operations
		wg.Add(1)
		go func() {
			defer wg.Done()
			orderID := setupOrders
			for {
				select {
				case <-ctx.Done():
					return
				default:
					order := &Order{
						ID:        fmt.Sprintf("concurrent-new-%d", orderID),
						Priority:  Priority(orderID % 3),
						Data:      []byte(fmt.Sprintf("concurrent-data-%d", orderID)),
						Timestamp: time.Now(),
					}
					recoveryQueue.Enqueue(order)
					orderID++
				}
			}
		}()
		
		// Concurrent dequeue operations
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					order, err := recoveryQueue.Dequeue()
					if err == nil && order != nil {
						recoveryQueue.Acknowledge(order.ID)
					}
				}
			}
		}()
		
		wg.Wait()
		cancel()
		
		b.StopTimer()
		recoveryQueue.Shutdown()
	}
}

// Memory usage benchmark during recovery
func BenchmarkRecoveryMemoryUsage(b *testing.B) {
	for i := 0; i < b.N; i++ {
		tempDir := b.TempDir()
		
		// Create large dataset
		setupQueue, err := NewBadgerQueue(tempDir)
		require.NoError(b, err)
		
		const largeDatasetSize = 50000
		for j := 0; j < largeDatasetSize; j++ {
			order := &Order{
				ID:        fmt.Sprintf("memory-test-%d", j),
				Priority:  Priority(j % 3),
				Data:      make([]byte, 2048), // 2KB per order
				Timestamp: time.Now(),
			}
			setupQueue.Enqueue(order)
		}
		
		// Process half, leave half pending
		for j := 0; j < largeDatasetSize/2; j++ {
			order, _ := setupQueue.Dequeue()
			if order != nil && j%3 != 0 {
				setupQueue.Acknowledge(order.ID)
			}
		}
		
		setupQueue.Shutdown()
		
		// Benchmark memory-efficient recovery
		b.ResetTimer()
		
		recoveryQueue, err := NewBadgerQueue(tempDir)
		require.NoError(b, err)
		
		// Measure replay with large dataset
		pendingOrders, err := recoveryQueue.ReplayPending()
		require.NoError(b, err)
		
		b.StopTimer()
		
		b.ReportMetric(float64(len(pendingOrders)), "replayed_orders")
		b.ReportMetric(float64(len(pendingOrders)*2048), "replayed_data_bytes")
		
		recoveryQueue.Shutdown()
	}
}
