// =============================
// Comprehensive Testing Framework for Order Book Migration
// =============================
// This file provides extensive testing for the order book migration,
// including stress tests, deadlock detection, performance validation,
// and migration scenario testing.

package orderbook

import (
	"context"
	"fmt"
	"math/rand"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/trading/model"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// TestConfig defines configuration for various test scenarios
type TestConfig struct {
	// Load testing parameters
	NumGoroutines      int           `json:"num_goroutines"`
	OrdersPerGoroutine int           `json:"orders_per_goroutine"`
	TestDuration       time.Duration `json:"test_duration"`

	// Order generation parameters
	PriceRange        PriceRange    `json:"price_range"`
	QuantityRange     QuantityRange `json:"quantity_range"`
	CancelProbability float64       `json:"cancel_probability"`

	// Migration testing
	MigrationSteps []int32       `json:"migration_steps"`
	StepDuration   time.Duration `json:"step_duration"`

	// Performance thresholds
	MaxLatencyMs     int64   `json:"max_latency_ms"`
	MinThroughputOps int64   `json:"min_throughput_ops"`
	MaxErrorRate     float64 `json:"max_error_rate"`
}

type PriceRange struct {
	Min decimal.Decimal `json:"min"`
	Max decimal.Decimal `json:"max"`
}

type QuantityRange struct {
	Min decimal.Decimal `json:"min"`
	Max decimal.Decimal `json:"max"`
}

// DefaultTestConfig returns a reasonable default configuration
func DefaultTestConfig() *TestConfig {
	return &TestConfig{
		NumGoroutines:      50,
		OrdersPerGoroutine: 1000,
		TestDuration:       30 * time.Second,
		PriceRange: PriceRange{
			Min: decimal.NewFromFloat(100.0),
			Max: decimal.NewFromFloat(200.0),
		},
		QuantityRange: QuantityRange{
			Min: decimal.NewFromFloat(0.1),
			Max: decimal.NewFromFloat(10.0),
		},
		CancelProbability: 0.2, // 20% of orders will be cancelled
		MigrationSteps:    []int32{0, 25, 50, 75, 100},
		StepDuration:      10 * time.Second,
		MaxLatencyMs:      50,   // 50ms max latency
		MinThroughputOps:  1000, // 1000 ops/sec minimum
		MaxErrorRate:      0.01, // 1% max error rate
	}
}

// TestResult holds the results of test execution
type TestResult struct {
	// Execution metrics
	TotalOrders      int64         `json:"total_orders"`
	SuccessfulOrders int64         `json:"successful_orders"`
	FailedOrders     int64         `json:"failed_orders"`
	CancelledOrders  int64         `json:"cancelled_orders"`
	ExecutionTime    time.Duration `json:"execution_time"`

	// Performance metrics
	AvgLatencyMs  float64 `json:"avg_latency_ms"`
	P95LatencyMs  float64 `json:"p95_latency_ms"`
	P99LatencyMs  float64 `json:"p99_latency_ms"`
	ThroughputOps float64 `json:"throughput_ops"`
	ErrorRate     float64 `json:"error_rate"`

	// Resource usage
	MaxGoroutines int     `json:"max_goroutines"`
	MaxMemoryMB   float64 `json:"max_memory_mb"`
	GCPauses      int64   `json:"gc_pauses"`

	// Deadlock detection
	DeadlocksDetected int32 `json:"deadlocks_detected"`
	ContentionEvents  int64 `json:"contention_events"`
	LockTimeouts      int64 `json:"lock_timeouts"`

	// Migration specific
	MigrationSuccess    bool  `json:"migration_success"`
	FallbackEvents      int64 `json:"fallback_events"`
	CircuitBreakerTrips int32 `json:"circuit_breaker_trips"`
}

// OrderBookTester provides comprehensive testing functionality
type OrderBookTester struct {
	config       *TestConfig
	oldOrderBook *OrderBook
	newOrderBook *DeadlockSafeOrderBook
	adaptiveBook *AdaptiveOrderBook

	// Test state
	testOrders   sync.Map     // map[uuid.UUID]*model.Order
	orderHistory []OrderEvent // History of all operations
	results      *TestResult

	// Synchronization
	wg     sync.WaitGroup
	ctx    context.Context
	cancel context.CancelFunc

	// Metrics collection
	latencies []time.Duration
	latencyMu sync.Mutex

	// Deadlock detection
	deadlockChan   chan DeadlockEvent
	contentionChan chan ContentionEvent
}

// OrderEvent represents a single order operation for audit
type OrderEvent struct {
	Timestamp      time.Time     `json:"timestamp"`
	OrderID        uuid.UUID     `json:"order_id"`
	Operation      string        `json:"operation"` // "ADD", "CANCEL", "MATCH"
	Success        bool          `json:"success"`
	Error          string        `json:"error,omitempty"`
	Latency        time.Duration `json:"latency"`
	Implementation string        `json:"implementation"` // "OLD", "NEW", "ADAPTIVE"
}

// DeadlockEvent represents a detected deadlock scenario
type DeadlockEvent struct {
	Timestamp   time.Time `json:"timestamp"`
	Goroutines  []string  `json:"goroutines"`
	Resources   []string  `json:"resources"`
	Description string    `json:"description"`
}

// ContentionEvent represents lock contention
type ContentionEvent struct {
	Timestamp time.Time     `json:"timestamp"`
	Resource  string        `json:"resource"`
	WaitTime  time.Duration `json:"wait_time"`
	Goroutine string        `json:"goroutine"`
}

// NewOrderBookTester creates a new comprehensive tester
func NewOrderBookTester(config *TestConfig) *OrderBookTester {
	if config == nil {
		config = DefaultTestConfig()
	}

	ctx, cancel := context.WithTimeout(context.Background(), config.TestDuration*2) // Extra buffer

	tester := &OrderBookTester{
		config:         config,
		oldOrderBook:   NewOrderBook("BTCUSDT"),
		newOrderBook:   NewDeadlockSafeOrderBook("BTCUSDT"),
		ctx:            ctx,
		cancel:         cancel,
		results:        &TestResult{},
		deadlockChan:   make(chan DeadlockEvent, 100),
		contentionChan: make(chan ContentionEvent, 1000),
	}

	// Create adaptive order book for migration testing
	migrationConfig := DefaultMigrationConfig()
	migrationConfig.CollectMetrics = true
	tester.adaptiveBook = NewAdaptiveOrderBook("BTCUSDT", migrationConfig)

	return tester
}

// RunComprehensiveTests executes all test scenarios
func (t *OrderBookTester) RunComprehensiveTests() (*TestResult, error) {
	fmt.Println("Starting comprehensive order book tests...")

	// Start monitoring
	t.startMonitoring()
	defer t.stopMonitoring()

	// Run individual test suites
	testSuites := []struct {
		name string
		fn   func() error
	}{
		{"Basic Functionality", t.testBasicFunctionality},
		{"Stress Test Old Implementation", t.stressTestOldImplementation},
		{"Stress Test New Implementation", t.stressTestNewImplementation},
		{"Deadlock Detection", t.testDeadlockScenarios},
		{"Migration Scenarios", t.testMigrationScenarios},
		{"Performance Comparison", t.testPerformanceComparison},
		{"Concurrent Operations", t.testConcurrentOperations},
		{"Resource Usage", t.testResourceUsage},
	}

	for _, suite := range testSuites {
		fmt.Printf("Running test suite: %s\n", suite.name)
		if err := suite.fn(); err != nil {
			return t.results, fmt.Errorf("test suite '%s' failed: %w", suite.name, err)
		}
	}

	// Finalize results
	t.finalizeResults()

	fmt.Printf("All tests completed successfully!\n")
	fmt.Printf("Results: %+v\n", t.results)

	return t.results, nil
}

// testBasicFunctionality tests basic order book operations
func (t *OrderBookTester) testBasicFunctionality() error {
	// Test order placement, matching, and cancellation
	for _, impl := range []string{"old", "new", "adaptive"} {
		if err := t.testBasicOperations(impl); err != nil {
			return fmt.Errorf("basic operations failed for %s implementation: %w", impl, err)
		}
	}
	return nil
}

// testBasicOperations tests basic operations on a specific implementation
func (t *OrderBookTester) testBasicOperations(impl string) error {
	// Create test orders
	buyOrder := t.generateRandomOrder(model.OrderSideBuy)

	var err error
	var ob OrderBookInterface

	switch impl {
	case "old":
		ob = t.oldOrderBook
	case "new":
		ob = t.adaptiveBook // Use adaptive book configured for new implementation
		t.adaptiveBook.SetMigrationPercentage(100)
	case "adaptive":
		ob = t.adaptiveBook
		t.adaptiveBook.SetMigrationPercentage(50)
	default:
		return fmt.Errorf("unknown implementation: %s", impl)
	}

	// Test order placement
	start := time.Now()
	_, err = ob.AddOrder(buyOrder)
	latency := time.Since(start)

	if err != nil {
		return fmt.Errorf("failed to add buy order: %w", err)
	}

	t.recordLatency(latency)
	t.recordOrderEvent(buyOrder.ID, "ADD", true, "", latency, impl)

	// Test order cancellation
	start = time.Now()
	err = ob.CancelOrder(buyOrder.ID)
	latency = time.Since(start)

	if err != nil {
		return fmt.Errorf("failed to cancel order: %w", err)
	}

	t.recordLatency(latency)
	t.recordOrderEvent(buyOrder.ID, "CANCEL", true, "", latency, impl)

	return nil
}

// stressTestOldImplementation performs stress testing on the old implementation
func (t *OrderBookTester) stressTestOldImplementation() error {
	return t.runStressTest(t.oldOrderBook, "old")
}

// stressTestNewImplementation performs stress testing on the new implementation
func (t *OrderBookTester) stressTestNewImplementation() error {
	// Configure adaptive book to use only new implementation
	t.adaptiveBook.SetMigrationPercentage(100)
	t.adaptiveBook.EnableNewImplementation(true)
	return t.runStressTest(t.adaptiveBook, "new")
}

// runStressTest executes a stress test on the given order book
func (t *OrderBookTester) runStressTest(ob OrderBookInterface, implName string) error {
	fmt.Printf("Running stress test on %s implementation...\n", implName)

	// Reset counters
	atomic.StoreInt64(&t.results.TotalOrders, 0)
	atomic.StoreInt64(&t.results.SuccessfulOrders, 0)
	atomic.StoreInt64(&t.results.FailedOrders, 0)

	ctx, cancel := context.WithTimeout(t.ctx, t.config.TestDuration)
	defer cancel()

	// Launch worker goroutines
	for i := 0; i < t.config.NumGoroutines; i++ {
		t.wg.Add(1)
		go t.stressWorker(ctx, ob, implName, i)
	}

	// Wait for completion
	t.wg.Wait()

	// Validate results
	totalOrders := atomic.LoadInt64(&t.results.TotalOrders)
	failedOrders := atomic.LoadInt64(&t.results.FailedOrders)

	if totalOrders == 0 {
		return fmt.Errorf("no orders were processed")
	}

	errorRate := float64(failedOrders) / float64(totalOrders)
	if errorRate > t.config.MaxErrorRate {
		return fmt.Errorf("error rate %.4f exceeds threshold %.4f", errorRate, t.config.MaxErrorRate)
	}

	fmt.Printf("Stress test completed: %d orders, %.4f error rate\n", totalOrders, errorRate)
	return nil
}

// stressWorker is a worker goroutine for stress testing
func (t *OrderBookTester) stressWorker(ctx context.Context, ob OrderBookInterface, implName string, workerID int) {
	defer t.wg.Done()

	rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(workerID)))
	activeOrders := make([]uuid.UUID, 0, 100)

	for {
		select {
		case <-ctx.Done():
			return
		default:
			atomic.AddInt64(&t.results.TotalOrders, 1)

			// Decide operation: 80% add order, 20% cancel order
			if rng.Float64() < 0.8 || len(activeOrders) == 0 {
				// Add order
				order := t.generateRandomOrder(t.randomSide(rng))

				start := time.Now()
				_, err := ob.AddOrder(order)
				latency := time.Since(start)

				t.recordLatency(latency)

				if err != nil {
					atomic.AddInt64(&t.results.FailedOrders, 1)
					t.recordOrderEvent(order.ID, "ADD", false, err.Error(), latency, implName)
				} else {
					atomic.AddInt64(&t.results.SuccessfulOrders, 1)
					activeOrders = append(activeOrders, order.ID)
					t.recordOrderEvent(order.ID, "ADD", true, "", latency, implName)
				}
			} else {
				// Cancel order
				if len(activeOrders) > 0 {
					idx := rng.Intn(len(activeOrders))
					orderID := activeOrders[idx]

					start := time.Now()
					err := ob.CancelOrder(orderID)
					latency := time.Since(start)

					t.recordLatency(latency)

					// Remove from active orders
					activeOrders[idx] = activeOrders[len(activeOrders)-1]
					activeOrders = activeOrders[:len(activeOrders)-1]

					if err != nil {
						atomic.AddInt64(&t.results.FailedOrders, 1)
						t.recordOrderEvent(orderID, "CANCEL", false, err.Error(), latency, implName)
					} else {
						atomic.AddInt64(&t.results.SuccessfulOrders, 1)
						t.recordOrderEvent(orderID, "CANCEL", true, "", latency, implName)
					}
				}
			}

			// Small delay to prevent CPU saturation
			time.Sleep(time.Microsecond * 100)
		}
	}
}

// testDeadlockScenarios tests for potential deadlock conditions
func (t *OrderBookTester) testDeadlockScenarios() error {
	fmt.Println("Testing deadlock scenarios...")

	// Test scenario 1: Multiple concurrent add/cancel operations
	if err := t.testConcurrentAddCancel(); err != nil {
		return fmt.Errorf("concurrent add/cancel test failed: %w", err)
	}

	// Test scenario 2: Cross-market operations
	if err := t.testCrossMarketOperations(); err != nil {
		return fmt.Errorf("cross-market operations test failed: %w", err)
	}

	// Test scenario 3: Administrative operations during trading
	if err := t.testAdminOperationsDuringTrading(); err != nil {
		return fmt.Errorf("admin operations test failed: %w", err)
	}

	return nil
}

// testConcurrentAddCancel tests concurrent add/cancel operations that could cause deadlocks
func (t *OrderBookTester) testConcurrentAddCancel() error {
	const numWorkers = 20
	const operationsPerWorker = 500

	ctx, cancel := context.WithTimeout(t.ctx, 30*time.Second)
	defer cancel()

	// Use new implementation for deadlock testing
	t.adaptiveBook.SetMigrationPercentage(100)
	t.adaptiveBook.EnableNewImplementation(true)

	deadlockDetected := int32(0)

	// Deadlock detection goroutine
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		lastGoRoutines := runtime.NumGoroutine()
		stuckCount := 0

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				currentGoRoutines := runtime.NumGoroutine()
				if currentGoRoutines == lastGoRoutines && currentGoRoutines > numWorkers+10 {
					stuckCount++
					if stuckCount > 50 { // 5 seconds of no progress
						atomic.StoreInt32(&deadlockDetected, 1)
						t.recordDeadlockEvent("Potential deadlock detected: goroutines stuck")
						return
					}
				} else {
					stuckCount = 0
				}
				lastGoRoutines = currentGoRoutines
			}
		}
	}()

	// Launch workers
	var wg sync.WaitGroup
	orderIDs := make([]uuid.UUID, numWorkers*operationsPerWorker)

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(workerID)))

			for j := 0; j < operationsPerWorker; j++ {
				orderIdx := workerID*operationsPerWorker + j
				order := t.generateRandomOrder(t.randomSide(rng))
				orderIDs[orderIdx] = order.ID

				// Add order
				_, err := t.adaptiveBook.AddOrder(order)
				if err != nil {
					// Non-fatal for this test
					continue
				}

				// Sometimes immediately cancel
				if rng.Float64() < 0.3 {
					_ = t.adaptiveBook.CancelOrder(order.ID)
				}

				// Check for deadlock
				if atomic.LoadInt32(&deadlockDetected) == 1 {
					return
				}
			}
		}(i)
	}

	// Wait with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-ctx.Done():
		if atomic.LoadInt32(&deadlockDetected) == 1 {
			return fmt.Errorf("deadlock detected during concurrent operations")
		}
		return fmt.Errorf("test timed out (potential deadlock)")
	}

	// Clean up remaining orders
	for _, orderID := range orderIDs {
		_ = t.adaptiveBook.CancelOrder(orderID)
	}

	return nil
}

// testCrossMarketOperations tests operations across multiple order books
func (t *OrderBookTester) testCrossMarketOperations() error {
	// Create multiple adaptive order books
	books := make([]*AdaptiveOrderBook, 5)
	for i := range books {
		config := DefaultMigrationConfig()
		config.EnableNewImplementation = true
		books[i] = NewAdaptiveOrderBook(fmt.Sprintf("PAIR%d", i), config)
		books[i].SetMigrationPercentage(100)
	}

	// Test concurrent operations across books
	var wg sync.WaitGroup
	ctx, cancel := context.WithTimeout(t.ctx, 10*time.Second)
	defer cancel()

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(workerID)))

			for j := 0; j < 100; j++ {
				select {
				case <-ctx.Done():
					return
				default:
					// Pick random book
					book := books[rng.Intn(len(books))]
					order := t.generateRandomOrder(t.randomSide(rng))

					_, err := book.AddOrder(order)
					if err != nil {
						continue
					}

					// Sometimes cancel immediately
					if rng.Float64() < 0.5 {
						_ = book.CancelOrder(order.ID)
					}
				}
			}
		}(i)
	}

	wg.Wait()
	return nil
}

// testAdminOperationsDuringTrading tests administrative operations during active trading
func (t *OrderBookTester) testAdminOperationsDuringTrading() error {
	ctx, cancel := context.WithTimeout(t.ctx, 15*time.Second)
	defer cancel()

	t.adaptiveBook.SetMigrationPercentage(100)
	t.adaptiveBook.EnableNewImplementation(true)

	var wg sync.WaitGroup

	// Trading worker
	wg.Add(1)
	go func() {
		defer wg.Done()

		rng := rand.New(rand.NewSource(time.Now().UnixNano()))

		for {
			select {
			case <-ctx.Done():
				return
			default:
				order := t.generateRandomOrder(t.randomSide(rng))
				_, _ = t.adaptiveBook.AddOrder(order)

				if rng.Float64() < 0.3 {
					_ = t.adaptiveBook.CancelOrder(order.ID)
				}

				time.Sleep(time.Millisecond)
			}
		}
	}()

	// Admin operations worker
	wg.Add(1)
	go func() {
		defer wg.Done()

		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Cycle through admin operations
				t.adaptiveBook.HaltTrading()
				time.Sleep(time.Millisecond * 10)
				t.adaptiveBook.ResumeTrading()

				t.adaptiveBook.TriggerCircuitBreaker()
				time.Sleep(time.Millisecond * 10)
				t.adaptiveBook.ResetCircuitBreaker()
			}
		}
	}()

	wg.Wait()
	return nil
}

// testMigrationScenarios tests gradual migration scenarios
func (t *OrderBookTester) testMigrationScenarios() error {
	fmt.Println("Testing migration scenarios...")

	for _, percentage := range t.config.MigrationSteps {
		fmt.Printf("Testing migration at %d%%...\n", percentage)

		if err := t.testMigrationStep(percentage); err != nil {
			return fmt.Errorf("migration step %d%% failed: %w", percentage, err)
		}
	}

	return nil
}

// testMigrationStep tests a specific migration percentage
func (t *OrderBookTester) testMigrationStep(percentage int32) error {
	t.adaptiveBook.SetMigrationPercentage(percentage)
	t.adaptiveBook.EnableNewImplementation(true)
	t.adaptiveBook.ResetCircuitBreaker()

	ctx, cancel := context.WithTimeout(t.ctx, t.config.StepDuration)
	defer cancel()

	// Track metrics for this step
	var stepOrders, stepSuccesses, stepFailures int64

	// Run operations
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			rng := rand.New(rand.NewSource(time.Now().UnixNano()))

			for {
				select {
				case <-ctx.Done():
					return
				default:
					atomic.AddInt64(&stepOrders, 1)

					order := t.generateRandomOrder(t.randomSide(rng))
					_, err := t.adaptiveBook.AddOrder(order)

					if err != nil {
						atomic.AddInt64(&stepFailures, 1)
					} else {
						atomic.AddInt64(&stepSuccesses, 1)

						// Sometimes cancel
						if rng.Float64() < 0.2 {
							_ = t.adaptiveBook.CancelOrder(order.ID)
						}
					}

					time.Sleep(time.Millisecond)
				}
			}
		}()
	}

	wg.Wait()

	// Validate migration step
	stepErrorRate := float64(stepFailures) / float64(stepOrders)
	if stepErrorRate > t.config.MaxErrorRate*2 { // Allow higher error rate during migration
		return fmt.Errorf("error rate %.4f too high for migration step", stepErrorRate)
	}

	// Check migration stats
	stats := t.adaptiveBook.GetMigrationStats()
	if stats.CircuitBreakerOpen {
		return fmt.Errorf("circuit breaker opened during migration step")
	}

	fmt.Printf("Migration step %d%% completed: %d orders, %.4f error rate\n",
		percentage, stepOrders, stepErrorRate)

	return nil
}

// testPerformanceComparison compares performance between implementations
func (t *OrderBookTester) testPerformanceComparison() error {
	fmt.Println("Running performance comparison...")

	implementations := []struct {
		name    string
		setupFn func()
	}{
		{"Old Implementation", func() {
			t.adaptiveBook.SetMigrationPercentage(0)
			t.adaptiveBook.EnableNewImplementation(false)
		}},
		{"New Implementation", func() {
			t.adaptiveBook.SetMigrationPercentage(100)
			t.adaptiveBook.EnableNewImplementation(true)
		}},
		{"Adaptive (50/50)", func() {
			t.adaptiveBook.SetMigrationPercentage(50)
			t.adaptiveBook.EnableNewImplementation(true)
		}},
	}

	results := make(map[string]*TestResult)

	for _, impl := range implementations {
		fmt.Printf("Testing %s...\n", impl.name)
		impl.setupFn()

		result, err := t.runPerformanceTest()
		if err != nil {
			return fmt.Errorf("performance test failed for %s: %w", impl.name, err)
		}

		results[impl.name] = result
		fmt.Printf("%s: %.2f ops/sec, %.2fms avg latency\n",
			impl.name, result.ThroughputOps, result.AvgLatencyMs)
	}

	// Compare results
	oldResult := results["Old Implementation"]
	newResult := results["New Implementation"]

	// New implementation should not be significantly slower
	if newResult.ThroughputOps < oldResult.ThroughputOps*0.8 {
		return fmt.Errorf("new implementation throughput too low: %.2f vs %.2f",
			newResult.ThroughputOps, oldResult.ThroughputOps)
	}

	// New implementation should not have significantly higher latency
	if newResult.AvgLatencyMs > oldResult.AvgLatencyMs*1.5 {
		return fmt.Errorf("new implementation latency too high: %.2fms vs %.2fms",
			newResult.AvgLatencyMs, oldResult.AvgLatencyMs)
	}

	return nil
}

// runPerformanceTest runs a focused performance test
func (t *OrderBookTester) runPerformanceTest() (*TestResult, error) {
	result := &TestResult{}

	ctx, cancel := context.WithTimeout(t.ctx, 10*time.Second)
	defer cancel()

	start := time.Now()
	var operations int64
	var latencies []time.Duration
	var latencyMu sync.Mutex

	// Run performance test
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			rng := rand.New(rand.NewSource(time.Now().UnixNano()))

			for {
				select {
				case <-ctx.Done():
					return
				default:
					order := t.generateRandomOrder(t.randomSide(rng))

					opStart := time.Now()
					_, err := t.adaptiveBook.AddOrder(order)
					latency := time.Since(opStart)

					atomic.AddInt64(&operations, 1)

					if err == nil {
						latencyMu.Lock()
						latencies = append(latencies, latency)
						latencyMu.Unlock()

						// Sometimes cancel
						if rng.Float64() < 0.3 {
							_ = t.adaptiveBook.CancelOrder(order.ID)
						}
					}

					time.Sleep(time.Microsecond * 100)
				}
			}
		}()
	}

	wg.Wait()
	duration := time.Since(start)

	// Calculate metrics
	result.TotalOrders = operations
	result.ExecutionTime = duration
	result.ThroughputOps = float64(operations) / duration.Seconds()

	if len(latencies) > 0 {
		// Calculate latency percentiles
		latencySum := time.Duration(0)
		for _, lat := range latencies {
			latencySum += lat
		}
		result.AvgLatencyMs = float64(latencySum.Nanoseconds()) / float64(len(latencies)) / 1e6

		// Calculate rough percentiles
		if len(latencies) > 20 {
			result.P95LatencyMs = result.AvgLatencyMs * 1.5 // Rough estimate for ~95th percentile
			result.P99LatencyMs = result.AvgLatencyMs * 2.0 // Rough estimate for ~99th percentile
		}
	}

	return result, nil
}

// testConcurrentOperations tests various concurrent operation patterns
func (t *OrderBookTester) testConcurrentOperations() error {
	fmt.Println("Testing concurrent operations...")

	// Test patterns that could expose race conditions
	patterns := []struct {
		name string
		fn   func() error
	}{
		{"Concurrent Same-Price Orders", t.testConcurrentSamePriceOrders},
		{"Concurrent Snapshot Requests", t.testConcurrentSnapshots},
		{"Mixed Read/Write Operations", t.testMixedReadWrite},
	}

	for _, pattern := range patterns {
		fmt.Printf("Testing pattern: %s\n", pattern.name)
		if err := pattern.fn(); err != nil {
			return fmt.Errorf("pattern '%s' failed: %w", pattern.name, err)
		}
	}

	return nil
}

// testConcurrentSamePriceOrders tests multiple orders at the same price level
func (t *OrderBookTester) testConcurrentSamePriceOrders() error {
	t.adaptiveBook.SetMigrationPercentage(100)
	t.adaptiveBook.EnableNewImplementation(true)

	ctx, cancel := context.WithTimeout(t.ctx, 5*time.Second)
	defer cancel()

	// Use fixed price to force contention
	fixedPrice := decimal.NewFromFloat(100.0)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for j := 0; j < 20; j++ {
				select {
				case <-ctx.Done():
					return
				default:
					order := t.generateRandomOrder(model.OrderSideBuy)
					order.Price = fixedPrice

					_, _ = t.adaptiveBook.AddOrder(order)
				}
			}
		}()
	}

	wg.Wait()
	return nil
}

// testConcurrentSnapshots tests concurrent snapshot requests
func (t *OrderBookTester) testConcurrentSnapshots() error {
	// Add some initial orders
	for i := 0; i < 100; i++ {
		order := t.generateRandomOrder(t.randomSide(nil))
		_, _ = t.adaptiveBook.AddOrder(order)
	}

	ctx, cancel := context.WithTimeout(t.ctx, 5*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for j := 0; j < 100; j++ {
				select {
				case <-ctx.Done():
					return
				default:
					_, _ = t.adaptiveBook.GetSnapshot(10)
					time.Sleep(time.Millisecond)
				}
			}
		}()
	}

	wg.Wait()
	return nil
}

// testMixedReadWrite tests mixed read and write operations
func (t *OrderBookTester) testMixedReadWrite() error {
	ctx, cancel := context.WithTimeout(t.ctx, 5*time.Second)
	defer cancel()

	var wg sync.WaitGroup

	// Writers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			rng := rand.New(rand.NewSource(time.Now().UnixNano()))

			for {
				select {
				case <-ctx.Done():
					return
				default:
					order := t.generateRandomOrder(t.randomSide(rng))
					_, _ = t.adaptiveBook.AddOrder(order)
					time.Sleep(time.Millisecond * 2)
				}
			}
		}()
	}

	// Readers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for {
				select {
				case <-ctx.Done():
					return
				default:
					_, _ = t.adaptiveBook.GetSnapshot(5)
					_ = t.adaptiveBook.OrdersCount()
					time.Sleep(time.Millisecond)
				}
			}
		}()
	}

	wg.Wait()
	return nil
}

// testResourceUsage monitors resource usage during testing
func (t *OrderBookTester) testResourceUsage() error {
	fmt.Println("Testing resource usage...")

	// Record initial memory
	var m1 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m1)

	initialGoroutines := runtime.NumGoroutine()

	// Run intensive operations
	ctx, cancel := context.WithTimeout(t.ctx, 10*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			rng := rand.New(rand.NewSource(time.Now().UnixNano()))

			for {
				select {
				case <-ctx.Done():
					return
				default:
					order := t.generateRandomOrder(t.randomSide(rng))
					_, _ = t.adaptiveBook.AddOrder(order)

					if rng.Float64() < 0.3 {
						_ = t.adaptiveBook.CancelOrder(order.ID)
					}

					time.Sleep(time.Microsecond * 500)
				}
			}
		}()
	}

	// Monitor resource usage
	maxGoroutines := initialGoroutines
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				current := runtime.NumGoroutine()
				if current > maxGoroutines {
					maxGoroutines = current
				}
			}
		}
	}()

	wg.Wait()

	// Record final memory
	var m2 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m2)

	// Update results
	t.results.MaxGoroutines = maxGoroutines
	t.results.MaxMemoryMB = float64(m2.Alloc-m1.Alloc) / (1024 * 1024)
	t.results.GCPauses = int64(m2.NumGC - m1.NumGC)

	// Validate resource usage
	if maxGoroutines > initialGoroutines+t.config.NumGoroutines+50 {
		return fmt.Errorf("too many goroutines created: %d (initial: %d)",
			maxGoroutines, initialGoroutines)
	}

	return nil
}

// Helper methods

// generateRandomOrder creates a random order for testing
func (t *OrderBookTester) generateRandomOrder(side string) *model.Order {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	price := t.randomPrice(rng)
	quantity := t.randomQuantity(rng)

	return &model.Order{
		ID:             uuid.New(),
		UserID:         uuid.New(),
		Pair:           "BTCUSDT",
		Side:           side,
		Type:           model.OrderTypeLimit,
		Price:          price,
		Quantity:       quantity,
		FilledQuantity: decimal.Zero,
		Status:         model.OrderStatusNew,
		TimeInForce:    model.TimeInForceGTC,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
}

// randomSide returns a random order side
func (t *OrderBookTester) randomSide(rng *rand.Rand) string {
	if rng == nil {
		rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	}

	if rng.Float64() < 0.5 {
		return model.OrderSideBuy
	}
	return model.OrderSideSell
}

// randomPrice generates a random price within the configured range
func (t *OrderBookTester) randomPrice(rng *rand.Rand) decimal.Decimal {
	min := t.config.PriceRange.Min.InexactFloat64()
	max := t.config.PriceRange.Max.InexactFloat64()

	price := min + rng.Float64()*(max-min)
	return decimal.NewFromFloat(price)
}

// randomQuantity generates a random quantity within the configured range
func (t *OrderBookTester) randomQuantity(rng *rand.Rand) decimal.Decimal {
	min := t.config.QuantityRange.Min.InexactFloat64()
	max := t.config.QuantityRange.Max.InexactFloat64()

	quantity := min + rng.Float64()*(max-min)
	return decimal.NewFromFloat(quantity)
}

// recordLatency records operation latency
func (t *OrderBookTester) recordLatency(latency time.Duration) {
	t.latencyMu.Lock()
	defer t.latencyMu.Unlock()
	t.latencies = append(t.latencies, latency)
}

// recordOrderEvent records an order event for audit
func (t *OrderBookTester) recordOrderEvent(orderID uuid.UUID, operation string, success bool, errMsg string, latency time.Duration, impl string) {
	event := OrderEvent{
		Timestamp:      time.Now(),
		OrderID:        orderID,
		Operation:      operation,
		Success:        success,
		Error:          errMsg,
		Latency:        latency,
		Implementation: impl,
	}

	// In a real implementation, this would be stored persistently
	t.orderHistory = append(t.orderHistory, event)
}

// recordDeadlockEvent records a deadlock event
func (t *OrderBookTester) recordDeadlockEvent(description string) {
	event := DeadlockEvent{
		Timestamp:   time.Now(),
		Description: description,
	}

	select {
	case t.deadlockChan <- event:
		atomic.AddInt32(&t.results.DeadlocksDetected, 1)
	default:
		// Channel full, which is itself concerning
	}
}

// startMonitoring starts background monitoring
func (t *OrderBookTester) startMonitoring() {
	// Deadlock monitoring
	go func() {
		for {
			select {
			case <-t.ctx.Done():
				return
			case event := <-t.deadlockChan:
				fmt.Printf("DEADLOCK DETECTED: %s at %v\n", event.Description, event.Timestamp)
			case event := <-t.contentionChan:
				atomic.AddInt64(&t.results.ContentionEvents, 1)
				_ = event // Process contention event
			}
		}
	}()
}

// stopMonitoring stops background monitoring
func (t *OrderBookTester) stopMonitoring() {
	t.cancel()
}

// finalizeResults calculates final test results
func (t *OrderBookTester) finalizeResults() {
	// Calculate average latency
	if len(t.latencies) > 0 {
		totalLatency := time.Duration(0)
		for _, lat := range t.latencies {
			totalLatency += lat
		}
		t.results.AvgLatencyMs = float64(totalLatency.Nanoseconds()) / float64(len(t.latencies)) / 1e6
	}

	// Calculate error rate
	if t.results.TotalOrders > 0 {
		t.results.ErrorRate = float64(t.results.FailedOrders) / float64(t.results.TotalOrders)
	}

	// Migration success if no deadlocks and reasonable error rate
	t.results.MigrationSuccess = t.results.DeadlocksDetected == 0 && t.results.ErrorRate <= t.config.MaxErrorRate
}

// Benchmark functions for performance testing

func BenchmarkOldOrderBookAddOrder(b *testing.B) {
	ob := NewOrderBook("BTCUSDT")
	tester := NewOrderBookTester(nil)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		rng := rand.New(rand.NewSource(time.Now().UnixNano()))

		for pb.Next() {
			order := tester.generateRandomOrder(tester.randomSide(rng))
			_, _ = ob.AddOrder(order)
		}
	})
}

func BenchmarkNewOrderBookAddOrder(b *testing.B) {
	config := DefaultMigrationConfig()
	config.EnableNewImplementation = true

	adaptive := NewAdaptiveOrderBook("BTCUSDT", config)
	adaptive.SetMigrationPercentage(100)

	tester := NewOrderBookTester(nil)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		rng := rand.New(rand.NewSource(time.Now().UnixNano()))

		for pb.Next() {
			order := tester.generateRandomOrder(tester.randomSide(rng))
			_, _ = adaptive.AddOrder(order)
		}
	})
}

func BenchmarkAdaptiveOrderBookAddOrder(b *testing.B) {
	config := DefaultMigrationConfig()
	config.EnableNewImplementation = true

	adaptive := NewAdaptiveOrderBook("BTCUSDT", config)
	adaptive.SetMigrationPercentage(50) // 50/50 split

	tester := NewOrderBookTester(nil)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		rng := rand.New(rand.NewSource(time.Now().UnixNano()))

		for pb.Next() {
			order := tester.generateRandomOrder(tester.randomSide(rng))
			_, _ = adaptive.AddOrder(order)
		}
	})
}
