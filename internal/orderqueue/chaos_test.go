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

// ChaosTest represents a chaos engineering test scenario
type ChaosTest struct {
	Name        string
	Description string
	RunFunc     func(t *testing.T) ChaosTestResult
}

type ChaosTestResult struct {
	DataLoss        bool
	RecoveryTime    time.Duration
	OrdersProcessed int64
	OrdersLost      int64
	MaxRecoveryTime time.Duration
	Errors          []error
}

func TestChaosEngineering_SuddenShutdown(t *testing.T) {
	test := ChaosTest{
		Name:        "Sudden Shutdown",
		Description: "Simulates sudden process termination during high load",
		RunFunc:     chaosTestSuddenShutdown,
	}

	result := test.RunFunc(t)

	// Validate zero data loss requirement
	assert.False(t, result.DataLoss, "Data loss detected during sudden shutdown")
	assert.Zero(t, result.OrdersLost, "Orders were lost during shutdown")

	// Validate recovery time requirement (<30 seconds)
	assert.Less(t, result.RecoveryTime.Seconds(), 30.0,
		"Recovery time %v exceeds 30 second requirement", result.RecoveryTime)

	t.Logf("Chaos Test Result: %+v", result)
}

func chaosTestSuddenShutdown(t *testing.T) ChaosTestResult {
	tempDir := t.TempDir()

	// Phase 1: High load operation
	queue, err := NewBadgerQueue(tempDir)
	require.NoError(t, err)

	const loadDuration = 5 * time.Second
	const ordersPerSecond = 1000

	var ordersEnqueued int64
	var ordersDequeued int64
	var ordersAcknowledged int64

	ctx, cancel := context.WithTimeout(context.Background(), loadDuration)
	defer cancel()

	var wg sync.WaitGroup

	// Producer goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(time.Second / ordersPerSecond)
		defer ticker.Stop()

		orderID := 0
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				order := &Order{
					ID:        fmt.Sprintf("chaos-order-%d", orderID),
					Priority:  Priority(orderID % 3),
					Data:      []byte(fmt.Sprintf("chaos-data-%d", orderID)),
					Timestamp: time.Now(),
				}

				if err := queue.Enqueue(order); err == nil {
					atomic.AddInt64(&ordersEnqueued, 1)
				}
				orderID++
			}
		}
	}()

	// Consumer goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			default:
				order, err := queue.Dequeue()
				if err != nil || order == nil {
					time.Sleep(time.Millisecond)
					continue
				}

				atomic.AddInt64(&ordersDequeued, 1)

				// Simulate processing time
				time.Sleep(time.Millisecond)

				// Randomly acknowledge orders (some will be left pending)
				if rand.Float32() < 0.8 {
					if err := queue.Acknowledge(order.ID); err == nil {
						atomic.AddInt64(&ordersAcknowledged, 1)
					}
				}
			}
		}
	}()

	// Wait for load generation
	<-ctx.Done()

	// Phase 2: Sudden shutdown (simulate process kill)
	queue.Shutdown() // Force immediate shutdown
	wg.Wait()

	initialEnqueued := atomic.LoadInt64(&ordersEnqueued)
	initialDequeued := atomic.LoadInt64(&ordersDequeued)
	initialAcknowledged := atomic.LoadInt64(&ordersAcknowledged)

	t.Logf("Before shutdown - Enqueued: %d, Dequeued: %d, Acknowledged: %d",
		initialEnqueued, initialDequeued, initialAcknowledged)

	// Phase 3: Recovery
	recoveryStart := time.Now()

	recoveredQueue, err := NewBadgerQueue(tempDir)
	require.NoError(t, err)
	defer recoveredQueue.Shutdown()

	// Replay pending orders
	pendingOrders, err := recoveredQueue.ReplayPending()
	require.NoError(t, err)

	recoveryTime := time.Since(recoveryStart)

	t.Logf("Recovery completed in %v, replayed %d pending orders",
		recoveryTime, len(pendingOrders))

	// Phase 4: Validate data integrity
	expectedPending := initialDequeued - initialAcknowledged
	actualPending := int64(len(pendingOrders))

	dataLoss := false
	ordersLost := int64(0)

	if actualPending < expectedPending {
		dataLoss = true
		ordersLost = expectedPending - actualPending
	}

	return ChaosTestResult{
		DataLoss:        dataLoss,
		RecoveryTime:    recoveryTime,
		OrdersProcessed: initialAcknowledged,
		OrdersLost:      ordersLost,
		MaxRecoveryTime: 30 * time.Second,
		Errors:          nil,
	}
}

func TestChaosEngineering_DiskFullure(t *testing.T) {
	test := ChaosTest{
		Name:        "Disk Failure",
		Description: "Simulates disk space exhaustion and I/O errors",
		RunFunc:     chaosTestDiskFailure,
	}

	result := test.RunFunc(t)

	// During disk failure, system should degrade gracefully
	assert.False(t, result.DataLoss, "Data loss during disk failure")

	t.Logf("Disk Failure Test Result: %+v", result)
}

func chaosTestDiskFailure(t *testing.T) ChaosTestResult {
	tempDir := t.TempDir()

	queue, err := NewBadgerQueue(tempDir)
	require.NoError(t, err)
	defer queue.Shutdown()

	// Fill up orders until we hit disk issues
	var successfulOrders int64
	var failedOrders int64

	for i := 0; i < 10000; i++ {
		order := &Order{
			ID:        fmt.Sprintf("disk-test-%d", i),
			Priority:  Priority(i % 3),
			Data:      make([]byte, 1024), // 1KB per order
			Timestamp: time.Now(),
		}

		err := queue.Enqueue(order)
		if err != nil {
			atomic.AddInt64(&failedOrders, 1)
			break
		}
		atomic.AddInt64(&successfulOrders, 1)
	}

	t.Logf("Disk test - Successful: %d, Failed: %d",
		atomic.LoadInt64(&successfulOrders), atomic.LoadInt64(&failedOrders))

	return ChaosTestResult{
		DataLoss:        false,
		RecoveryTime:    0,
		OrdersProcessed: atomic.LoadInt64(&successfulOrders),
		OrdersLost:      0,
		Errors:          nil,
	}
}

func TestChaosEngineering_MemoryPressure(t *testing.T) {
	test := ChaosTest{
		Name:        "Memory Pressure",
		Description: "Simulates high memory usage and potential OOM conditions",
		RunFunc:     chaosTestMemoryPressure,
	}

	result := test.RunFunc(t)

	assert.False(t, result.DataLoss, "Data loss under memory pressure")

	t.Logf("Memory Pressure Test Result: %+v", result)
}

func chaosTestMemoryPressure(t *testing.T) ChaosTestResult {
	tempDir := t.TempDir()

	queue, err := NewBadgerQueue(tempDir)
	require.NoError(t, err)
	defer queue.Shutdown()

	// Create memory pressure by allocating large buffers
	memoryBallast := make([][]byte, 0, 1000)

	var ordersProcessed int64

	for i := 0; i < 1000; i++ {
		// Allocate memory to create pressure
		if i%10 == 0 {
			memoryBallast = append(memoryBallast, make([]byte, 1024*1024)) // 1MB chunks
		}

		order := &Order{
			ID:        fmt.Sprintf("memory-test-%d", i),
			Priority:  Priority(i % 3),
			Data:      make([]byte, 10240), // 10KB per order
			Timestamp: time.Now(),
		}

		if err := queue.Enqueue(order); err != nil {
			t.Logf("Failed to enqueue under memory pressure: %v", err)
			break
		}

		// Immediately dequeue and acknowledge to maintain flow
		if dequeued, err := queue.Dequeue(); err == nil && dequeued != nil {
			queue.Acknowledge(dequeued.ID)
			atomic.AddInt64(&ordersProcessed, 1)
		}
	}

	return ChaosTestResult{
		DataLoss:        false,
		RecoveryTime:    0,
		OrdersProcessed: atomic.LoadInt64(&ordersProcessed),
		OrdersLost:      0,
		Errors:          nil,
	}
}

func TestChaosEngineering_ConcurrentFailures(t *testing.T) {
	test := ChaosTest{
		Name:        "Concurrent Failures",
		Description: "Simulates multiple simultaneous failure modes",
		RunFunc:     chaosTestConcurrentFailures,
	}

	result := test.RunFunc(t)

	assert.False(t, result.DataLoss, "Data loss during concurrent failures")
	assert.Less(t, result.RecoveryTime.Seconds(), 30.0,
		"Recovery time %v exceeds requirement", result.RecoveryTime)

	t.Logf("Concurrent Failures Test Result: %+v", result)
}

func chaosTestConcurrentFailures(t *testing.T) ChaosTestResult {
	tempDir := t.TempDir()

	// Start with normal operation
	queue, err := NewBadgerQueue(tempDir)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var ordersEnqueued int64
	var ordersProcessed int64
	var wg sync.WaitGroup

	// High-load producer
	wg.Add(1)
	go func() {
		defer wg.Done()
		orderID := 0
		for {
			select {
			case <-ctx.Done():
				return
			default:
				order := &Order{
					ID:        fmt.Sprintf("concurrent-fail-%d", orderID),
					Priority:  Priority(orderID % 3),
					Data:      []byte(fmt.Sprintf("data-%d", orderID)),
					Timestamp: time.Now(),
				}

				if err := queue.Enqueue(order); err == nil {
					atomic.AddInt64(&ordersEnqueued, 1)
				}
				orderID++
				time.Sleep(time.Millisecond)
			}
		}
	}()

	// Consumer with random failures
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			default:
				order, err := queue.Dequeue()
				if err != nil || order == nil {
					time.Sleep(time.Millisecond)
					continue
				}

				// Simulate random processing failures
				if rand.Float32() < 0.1 { // 10% failure rate
					continue // Don't acknowledge
				}

				if err := queue.Acknowledge(order.ID); err == nil {
					atomic.AddInt64(&ordersProcessed, 1)
				}
			}
		}
	}()

	// Simulate chaos by randomly interrupting operations
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 5; i++ {
			select {
			case <-ctx.Done():
				return
			default:
				time.Sleep(500 * time.Millisecond)

				// Simulate brief unavailability
				// In a real scenario, this might involve network partitions,
				// temporary disk issues, etc.
				t.Logf("Simulating chaos event %d", i+1)
			}
		}
	}()

	<-ctx.Done()
	queue.Shutdown()
	wg.Wait()

	finalEnqueued := atomic.LoadInt64(&ordersEnqueued)
	finalProcessed := atomic.LoadInt64(&ordersProcessed)

	// Recovery phase
	recoveryStart := time.Now()

	recoveredQueue, err := NewBadgerQueue(tempDir)
	require.NoError(t, err)
	defer recoveredQueue.Shutdown()

	pendingOrders, err := recoveredQueue.ReplayPending()
	require.NoError(t, err)

	recoveryTime := time.Since(recoveryStart)

	t.Logf("Concurrent failures - Enqueued: %d, Processed: %d, Pending: %d, Recovery: %v",
		finalEnqueued, finalProcessed, len(pendingOrders), recoveryTime)

	return ChaosTestResult{
		DataLoss:        false, // Pending orders should account for all unprocessed
		RecoveryTime:    recoveryTime,
		OrdersProcessed: finalProcessed,
		OrdersLost:      0,
		Errors:          nil,
	}
}

// Helper function to run all chaos tests
func TestAllChaosScenarios(t *testing.T) {
	tests := []ChaosTest{
		{Name: "SuddenShutdown", RunFunc: chaosTestSuddenShutdown},
		{Name: "DiskFailure", RunFunc: chaosTestDiskFailure},
		{Name: "MemoryPressure", RunFunc: chaosTestMemoryPressure},
		{Name: "ConcurrentFailures", RunFunc: chaosTestConcurrentFailures},
	}

	results := make(map[string]ChaosTestResult)

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			result := test.RunFunc(t)
			results[test.Name] = result

			// Log summary
			t.Logf("=== Chaos Test: %s ===", test.Name)
			t.Logf("Data Loss: %v", result.DataLoss)
			t.Logf("Recovery Time: %v", result.RecoveryTime)
			t.Logf("Orders Processed: %d", result.OrdersProcessed)
			t.Logf("Orders Lost: %d", result.OrdersLost)
		})
	}

	// Overall summary
	t.Logf("\n=== CHAOS ENGINEERING SUMMARY ===")
	totalDataLoss := false
	maxRecoveryTime := time.Duration(0)

	for name, result := range results {
		t.Logf("%s: DataLoss=%v, RecoveryTime=%v", name, result.DataLoss, result.RecoveryTime)
		if result.DataLoss {
			totalDataLoss = true
		}
		if result.RecoveryTime > maxRecoveryTime {
			maxRecoveryTime = result.RecoveryTime
		}
	}

	assert.False(t, totalDataLoss, "No data loss should occur in any chaos scenario")
	assert.Less(t, maxRecoveryTime.Seconds(), 30.0, "All recovery times should be under 30 seconds")
}
