//go:build integration
// +build integration

package test

import (
	"context"
	"fmt"
	"math/rand"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/bookkeeper"
	"github.com/Aidin1998/pincex_unified/internal/trading"
	"github.com/Aidin1998/pincex_unified/internal/trading/model"
	"github.com/Aidin1998/pincex_unified/internal/trading/orderbook"
	"github.com/Aidin1998/pincex_unified/pkg/models"
	"github.com/Aidin1998/pincex_unified/testutil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// PoolPerformanceResult captures pool performance metrics
type PoolPerformanceResult struct {
	TestName          string
	ExecutionTime     time.Duration
	TotalOperations   int64
	TotalGCPauses     uint32
	TotalGCPauseTime  time.Duration
	MaxGCPauseTime    time.Duration
	AvgGCPauseTime    time.Duration
	TotalAllocations  uint64
	TotalMemoryUsedMB float64
	PeakMemoryUsedMB  float64
	PoolHitRate       float64
	OrdersPerSecond   float64
	TradesPerSecond   float64
	AvgLatencyMs      float64
	P95LatencyMs      float64
	P99LatencyMs      float64
}

// GCStats tracks garbage collection metrics
type GCStats struct {
	initialStats runtime.MemStats
	finalStats   runtime.MemStats
	gcPauses     []time.Duration
}

// TestObjectPoolIntegration tests the complete object pooling system
func TestObjectPoolIntegration(t *testing.T) {
	fmt.Println("\n========================================")
	fmt.Println("OBJECT POOL INTEGRATION TEST")
	fmt.Println("========================================")

	testCases := []struct {
		name                string
		concurrency         int
		operationsPerWorker int
		description         string
	}{
		{"Light Load", 5, 100, "Basic pool functionality validation"},
		{"Medium Load", 25, 500, "Moderate load testing"},
		{"Heavy Load", 50, 1000, "High concurrency stress test"},
		{"Extreme Load", 100, 2000, "Peak capacity testing"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := runPoolPerformanceTest(t, tc.concurrency, tc.operationsPerWorker)
			printPoolPerformanceResults(tc.name, tc.description, result)

			// Validate pool performance requirements
			validatePoolPerformance(t, tc.name, result)
		})
	}
}

// TestPoolMetricsAccuracy validates pool metrics tracking
func TestPoolMetricsAccuracy(t *testing.T) {
	fmt.Println("\n========================================")
	fmt.Println("POOL METRICS ACCURACY TEST")
	fmt.Println("========================================")

	// Pre-warm pools with known quantities
	model.PreallocateObjectPools(100, 100)
	orderbook.PrewarmOrderBookPools(50, 200, 50, 30)

	// Get initial metrics
	initialOrderMetrics := model.GetOrderPoolMetrics()
	initialTradeMetrics := model.GetTradePoolMetrics()
	initialOBMetrics := orderbook.GetAllPoolMetrics()

	// Perform controlled operations
	const numOperations = 50
	orders := make([]*model.Order, 0, numOperations)
	trades := make([]*model.Trade, 0, numOperations)

	// Get objects from pools
	for i := 0; i < numOperations; i++ {
		order := model.GetOrderFromPool()
		trade := model.GetTradeFromPool()
		orders = append(orders, order)
		trades = append(trades, trade)
	}

	// Return half to pools
	for i := 0; i < numOperations/2; i++ {
		model.PutOrderToPool(orders[i])
		model.PutTradeToPool(trades[i])
	}

	// Get final metrics
	finalOrderMetrics := model.GetOrderPoolMetrics()
	finalTradeMetrics := model.GetTradePoolMetrics()

	// Validate metrics accuracy
	expectedOrderGets := initialOrderMetrics.Gets + int64(numOperations)
	expectedOrderPuts := initialOrderMetrics.Puts + int64(numOperations/2)
	expectedTradeGets := initialTradeMetrics.Gets + int64(numOperations)
	expectedTradePuts := initialTradeMetrics.Puts + int64(numOperations/2)

	assert.Equal(t, expectedOrderGets, finalOrderMetrics.Gets, "Order pool gets mismatch")
	assert.Equal(t, expectedOrderPuts, finalOrderMetrics.Puts, "Order pool puts mismatch")
	assert.Equal(t, expectedTradeGets, finalTradeMetrics.Gets, "Trade pool gets mismatch")
	assert.Equal(t, expectedTradePuts, finalTradeMetrics.Puts, "Trade pool puts mismatch")

	// Calculate and validate hit rates
	orderHitRate := model.CalculateOrderPoolHitRate()
	tradeHitRate := model.CalculateTradePoolHitRate()
	obHitRate := orderbook.CalculatePoolHitRate()

	assert.Greater(t, orderHitRate, 0.0, "Order pool hit rate should be positive")
	assert.Greater(t, tradeHitRate, 0.0, "Trade pool hit rate should be positive")
	assert.Greater(t, obHitRate, 0.0, "OrderBook pool hit rate should be positive")

	fmt.Printf("Order Pool Hit Rate: %.2f%%\n", orderHitRate)
	fmt.Printf("Trade Pool Hit Rate: %.2f%%\n", tradeHitRate)
	fmt.Printf("OrderBook Pool Hit Rate: %.2f%%\n", obHitRate)

	// Test orderbook pools specifically
	priceLevels := make([]*orderbook.PriceLevel, 0, 20)
	chunks := make([]*orderbook.OrderChunk, 0, 20)

	for i := 0; i < 20; i++ {
		pl := orderbook.GetPriceLevelFromPool()
		chunk := orderbook.GetOrderChunkFromPool()
		priceLevels = append(priceLevels, pl)
		chunks = append(chunks, chunk)
	}

	// Return to pools
	for _, pl := range priceLevels {
		orderbook.PutPriceLevelToPool(pl)
	}
	for _, chunk := range chunks {
		orderbook.PutOrderChunkToPool(chunk)
	}

	finalOBMetrics = orderbook.GetAllPoolMetrics()
	assert.Greater(t, finalOBMetrics.PriceLevelPool.Gets, initialOBMetrics.PriceLevelPool.Gets, "PriceLevel pool should show increased gets")
	assert.Greater(t, finalOBMetrics.OrderChunkPool.Gets, initialOBMetrics.OrderChunkPool.Gets, "OrderChunk pool should show increased gets")

	// Clean up remaining objects
	for i := numOperations / 2; i < numOperations; i++ {
		model.PutOrderToPool(orders[i])
		model.PutTradeToPool(trades[i])
	}

	t.Logf("Pool metrics validation completed successfully")
}

// TestGCPressureReduction validates GC pressure reduction goals
func TestGCPressureReduction(t *testing.T) {
	fmt.Println("\n========================================")
	fmt.Println("GC PRESSURE REDUCTION VALIDATION")
	fmt.Println("========================================")

	// Test with and without pools to compare GC behavior
	testScenarios := []struct {
		name       string
		usePool    bool
		operations int
	}{
		{"Without Pool", false, 10000},
		{"With Pool", true, 10000},
	}

	results := make(map[string]PoolPerformanceResult)

	for _, scenario := range testScenarios {
		t.Run(scenario.name, func(t *testing.T) {
			result := runGCPressureTest(t, scenario.usePool, scenario.operations)
			results[scenario.name] = result
			printGCResults(scenario.name, result)
		})
	}

	// Compare results
	withoutPool := results["Without Pool"]
	withPool := results["With Pool"]

	// Calculate improvements
	gcPauseReduction := float64(withoutPool.TotalGCPauseTime-withPool.TotalGCPauseTime) / float64(withoutPool.TotalGCPauseTime) * 100
	maxPauseReduction := float64(withoutPool.MaxGCPauseTime-withPool.MaxGCPauseTime) / float64(withoutPool.MaxGCPauseTime) * 100
	allocationReduction := float64(withoutPool.TotalAllocations-withPool.TotalAllocations) / float64(withoutPool.TotalAllocations) * 100

	fmt.Printf("\n--- GC PRESSURE REDUCTION ANALYSIS ---\n")
	fmt.Printf("Total GC Pause Time Reduction: %.2f%%\n", gcPauseReduction)
	fmt.Printf("Max GC Pause Time Reduction: %.2f%%\n", maxPauseReduction)
	fmt.Printf("Allocation Reduction: %.2f%%\n", allocationReduction)

	// Validate target goals (40-60% GC pressure reduction)
	assert.GreaterOrEqual(t, gcPauseReduction, 40.0, "Should achieve at least 40%% GC pause time reduction")
	assert.LessOrEqual(t, withPool.MaxGCPauseTime, 100*time.Microsecond, "Max GC pause should be under 100μs")

	if gcPauseReduction >= 60.0 {
		t.Logf("✅ EXCELLENT: Achieved %.2f%% GC pressure reduction (target: 40-60%%)", gcPauseReduction)
	} else if gcPauseReduction >= 40.0 {
		t.Logf("✅ SUCCESS: Achieved %.2f%% GC pressure reduction (target: 40-60%%)", gcPauseReduction)
	} else {
		t.Errorf("❌ FAILED: Only achieved %.2f%% GC pressure reduction (target: 40-60%%)", gcPauseReduction)
	}
}

// TestHighFrequencyTrading validates performance under high-frequency trading loads
func TestHighFrequencyTrading(t *testing.T) {
	fmt.Println("\n========================================")
	fmt.Println("HIGH FREQUENCY TRADING VALIDATION")
	fmt.Println("========================================")

	// Setup trading service with enhanced pools
	svc := setupEnhancedTradingService(t)
	defer svc.Stop()

	ctx := context.Background()
	testPair := "BTCUSDT"

	// Create test users and accounts
	users := createTestUsers(t, 50)
	setupTestAccounts(t, svc, users, testPair)

	// High-frequency trading simulation
	const targetOPS = 10000 // 10K operations per second
	const testDuration = 5 * time.Second

	var totalOperations int64
	var totalLatencies []time.Duration
	var latencyMutex sync.Mutex

	gcStats := &GCStats{}
	runtime.ReadMemStats(&gcStats.initialStats)
	runtime.GC() // Force GC before test

	startTime := time.Now()
	endTime := startTime.Add(testDuration)

	// Launch high-frequency workers
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(workerID)))
			userID := users[workerID%len(users)]

			for time.Now().Before(endTime) {
				// Generate and place order
				order := generateTestOrder(rng, userID, testPair, int(atomic.LoadInt64(&totalOperations)))

				opStart := time.Now()
				_, err := svc.PlaceOrder(ctx, order)
				latency := time.Since(opStart)

				if err == nil {
					atomic.AddInt64(&totalOperations, 1)
					latencyMutex.Lock()
					totalLatencies = append(totalLatencies, latency)
					latencyMutex.Unlock()
				}

				// Small delay to control rate
				time.Sleep(time.Microsecond * time.Duration(100+rng.Intn(50)))
			}
		}(i)
	}

	wg.Wait()
	executionTime := time.Since(startTime)

	runtime.ReadMemStats(&gcStats.finalStats)

	// Calculate metrics
	opsPerSecond := float64(totalOperations) / executionTime.Seconds()

	var avgLatency time.Duration
	if len(totalLatencies) > 0 {
		var totalLatency time.Duration
		for _, lat := range totalLatencies {
			totalLatency += lat
		}
		avgLatency = totalLatency / time.Duration(len(totalLatencies))
	}

	p95Latency := testutil.Percentile(totalLatencies, 0.95)
	p99Latency := testutil.Percentile(totalLatencies, 0.99)

	// Pool metrics
	orderMetrics := model.GetOrderPoolMetrics()
	tradeMetrics := model.GetTradePoolMetrics()
	obMetrics := orderbook.GetAllPoolMetrics()

	orderHitRate := model.CalculateOrderPoolHitRate()
	tradeHitRate := model.CalculateTradePoolHitRate()
	obHitRate := orderbook.CalculatePoolHitRate()

	fmt.Printf("\n--- HIGH FREQUENCY TRADING RESULTS ---\n")
	fmt.Printf("Total Operations: %d\n", totalOperations)
	fmt.Printf("Execution Time: %v\n", executionTime)
	fmt.Printf("Operations/Second: %.2f\n", opsPerSecond)
	fmt.Printf("Average Latency: %v\n", avgLatency)
	fmt.Printf("P95 Latency: %v\n", p95Latency)
	fmt.Printf("P99 Latency: %v\n", p99Latency)
	fmt.Printf("Order Pool Hit Rate: %.2f%%\n", orderHitRate)
	fmt.Printf("Trade Pool Hit Rate: %.2f%%\n", tradeHitRate)
	fmt.Printf("OrderBook Pool Hit Rate: %.2f%%\n", obHitRate)

	// Validate performance requirements
	assert.GreaterOrEqual(t, opsPerSecond, float64(targetOPS*0.8), "Should achieve at least 80%% of target OPS")
	assert.LessOrEqual(t, avgLatency, 5*time.Millisecond, "Average latency should be under 5ms")
	assert.LessOrEqual(t, p95Latency, 10*time.Millisecond, "P95 latency should be under 10ms")
	assert.GreaterOrEqual(t, orderHitRate, 70.0, "Order pool hit rate should be above 70%%")
	assert.GreaterOrEqual(t, tradeHitRate, 70.0, "Trade pool hit rate should be above 70%%")

	// GC validation
	gcPauses := gcStats.finalStats.NumGC - gcStats.initialStats.NumGC
	totalGCTime := time.Duration(gcStats.finalStats.PauseTotalNs - gcStats.initialStats.PauseTotalNs)

	if gcPauses > 0 {
		avgGCPause := totalGCTime / time.Duration(gcPauses)
		assert.LessOrEqual(t, avgGCPause, 100*time.Microsecond, "Average GC pause should be under 100μs")
		fmt.Printf("GC Pauses: %d, Total GC Time: %v, Avg GC Pause: %v\n", gcPauses, totalGCTime, avgGCPause)
	}
}

// Helper functions

func runPoolPerformanceTest(t *testing.T, concurrency, operationsPerWorker int) PoolPerformanceResult {
	// Initialize pools with adequate pre-warming
	model.PreallocateObjectPools(1000, 1000)
	orderbook.PrewarmOrderBookPools(500, 2000, 500, 300)

	gcStats := &GCStats{}
	runtime.ReadMemStats(&gcStats.initialStats)
	runtime.GC() // Force GC before test

	var totalOperations int64
	var latencies []time.Duration
	var latencyMutex sync.Mutex

	startTime := time.Now()

	var wg sync.WaitGroup
	wg.Add(concurrency)

	// Run concurrent workers
	for i := 0; i < concurrency; i++ {
		go func(workerID int) {
			defer wg.Done()

			rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(workerID)))

			for j := 0; j < operationsPerWorker; j++ {
				opStart := time.Now()

				// Mix of pool operations
				switch rng.Intn(4) {
				case 0: // Order operations
					order := model.GetOrderFromPool()
					// Simulate order processing
					order.ID = uuid.New()
					order.UserID = uuid.New()
					order.Pair = "BTCUSDT"
					order.Side = "buy"
					order.Type = "limit"
					model.PutOrderToPool(order)

				case 1: // Trade operations
					trade := model.GetTradeFromPool()
					// Simulate trade processing
					trade.ID = uuid.New()
					trade.BuyOrderID = uuid.New()
					trade.SellOrderID = uuid.New()
					trade.Pair = "BTCUSDT"
					model.PutTradeToPool(trade)

				case 2: // Price level operations
					pl := orderbook.GetPriceLevelFromPool()
					// Simulate price level usage
					pl.Price = 50000.0
					pl.Orders = nil
					orderbook.PutPriceLevelToPool(pl)

				case 3: // Order chunk operations
					chunk := orderbook.GetOrderChunkFromPool()
					// Simulate chunk usage
					for k := range chunk.Orders {
						chunk.Orders[k] = nil
					}
					chunk.Count = 0
					orderbook.PutOrderChunkToPool(chunk)
				}

				latency := time.Since(opStart)
				atomic.AddInt64(&totalOperations, 1)

				latencyMutex.Lock()
				latencies = append(latencies, latency)
				latencyMutex.Unlock()

				// Small delay to prevent CPU saturation
				if j%100 == 0 {
					time.Sleep(time.Microsecond)
				}
			}
		}(i)
	}

	wg.Wait()
	executionTime := time.Since(startTime)

	runtime.ReadMemStats(&gcStats.finalStats)
	runtime.GC() // Force final GC

	// Calculate metrics
	result := PoolPerformanceResult{
		ExecutionTime:   executionTime,
		TotalOperations: totalOperations,
		OrdersPerSecond: float64(totalOperations) / executionTime.Seconds(),
	}

	// Calculate latency metrics
	if len(latencies) > 0 {
		var totalLatency time.Duration
		for _, lat := range latencies {
			totalLatency += lat
		}
		result.AvgLatencyMs = float64(totalLatency.Nanoseconds()) / float64(len(latencies)) / 1e6
		result.P95LatencyMs = float64(testutil.Percentile(latencies, 0.95).Nanoseconds()) / 1e6
		result.P99LatencyMs = float64(testutil.Percentile(latencies, 0.99).Nanoseconds()) / 1e6
	}

	// Calculate GC metrics
	result.TotalGCPauses = gcStats.finalStats.NumGC - gcStats.initialStats.NumGC
	result.TotalGCPauseTime = time.Duration(gcStats.finalStats.PauseTotalNs - gcStats.initialStats.PauseTotalNs)
	result.TotalAllocations = gcStats.finalStats.TotalAlloc - gcStats.initialStats.TotalAlloc
	result.TotalMemoryUsedMB = float64(gcStats.finalStats.TotalAlloc-gcStats.initialStats.TotalAlloc) / (1024 * 1024)
	result.PeakMemoryUsedMB = float64(gcStats.finalStats.Sys) / (1024 * 1024)

	if result.TotalGCPauses > 0 {
		result.AvgGCPauseTime = result.TotalGCPauseTime / time.Duration(result.TotalGCPauses)
		// Find max pause time from recent pauses
		if gcStats.finalStats.NumGC > 256 {
			result.MaxGCPauseTime = time.Duration(gcStats.finalStats.PauseNs[(gcStats.finalStats.NumGC+255)%256])
		}
	}

	// Pool hit rates
	result.PoolHitRate = (model.CalculateOrderPoolHitRate() + model.CalculateTradePoolHitRate() + orderbook.CalculatePoolHitRate()) / 3.0

	return result
}

func runGCPressureTest(t *testing.T, usePool bool, operations int) PoolPerformanceResult {
	gcStats := &GCStats{}
	runtime.ReadMemStats(&gcStats.initialStats)
	runtime.GC() // Force GC before test

	startTime := time.Now()

	if usePool {
		// Pre-warm pools
		model.PreallocateObjectPools(1000, 1000)
		orderbook.PrewarmOrderBookPools(500, 2000, 500, 300)

		// Use pooled objects
		for i := 0; i < operations; i++ {
			order := model.GetOrderFromPool()
			trade := model.GetTradeFromPool()
			pl := orderbook.GetPriceLevelFromPool()

			// Simulate usage
			order.ID = uuid.New()
			trade.ID = uuid.New()
			pl.Price = float64(i)

			// Return to pool
			model.PutOrderToPool(order)
			model.PutTradeToPool(trade)
			orderbook.PutPriceLevelToPool(pl)

			if i%1000 == 0 {
				runtime.Gosched() // Allow other goroutines to run
			}
		}
	} else {
		// Allocate new objects each time
		for i := 0; i < operations; i++ {
			_ = &model.Order{
				ID:     uuid.New(),
				UserID: uuid.New(),
				Pair:   "BTCUSDT",
				Side:   "buy",
				Type:   "limit",
			}
			_ = &model.Trade{
				ID:          uuid.New(),
				BuyOrderID:  uuid.New(),
				SellOrderID: uuid.New(),
				Pair:        "BTCUSDT",
			}
			_ = &orderbook.PriceLevel{
				Price:  float64(i),
				Orders: make([]*model.Order, 0),
			}

			if i%1000 == 0 {
				runtime.Gosched() // Allow other goroutines to run
			}
		}
	}

	executionTime := time.Since(startTime)
	runtime.ReadMemStats(&gcStats.finalStats)
	runtime.GC() // Force final GC

	result := PoolPerformanceResult{
		ExecutionTime:     executionTime,
		TotalOperations:   int64(operations),
		TotalGCPauses:     gcStats.finalStats.NumGC - gcStats.initialStats.NumGC,
		TotalGCPauseTime:  time.Duration(gcStats.finalStats.PauseTotalNs - gcStats.initialStats.PauseTotalNs),
		TotalAllocations:  gcStats.finalStats.TotalAlloc - gcStats.initialStats.TotalAlloc,
		TotalMemoryUsedMB: float64(gcStats.finalStats.TotalAlloc-gcStats.initialStats.TotalAlloc) / (1024 * 1024),
		PeakMemoryUsedMB:  float64(gcStats.finalStats.Sys) / (1024 * 1024),
	}

	if result.TotalGCPauses > 0 {
		result.AvgGCPauseTime = result.TotalGCPauseTime / time.Duration(result.TotalGCPauses)
		// Find max pause time from recent pauses
		if gcStats.finalStats.NumGC > 256 {
			result.MaxGCPauseTime = time.Duration(gcStats.finalStats.PauseNs[(gcStats.finalStats.NumGC+255)%256])
		}
	}

	if usePool {
		result.PoolHitRate = (model.CalculateOrderPoolHitRate() + model.CalculateTradePoolHitRate() + orderbook.CalculatePoolHitRate()) / 3.0
	}

	return result
}

func setupEnhancedTradingService(t *testing.T) trading.TradingService {
	t.Helper()
	logger := zap.NewNop()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&models.TradingPair{}, &models.Order{}, &models.Trade{}, &models.Account{}, &models.Transaction{})
	require.NoError(t, err)

	bkSvc, err := bookkeeper.NewService(logger, db)
	require.NoError(t, err)

	svc, err := trading.NewService(logger, db, bkSvc)
	require.NoError(t, err)

	err = svc.Start()
	require.NoError(t, err)

	return svc
}

func createTestUsers(t *testing.T, count int) []uuid.UUID {
	users := make([]uuid.UUID, count)
	for i := 0; i < count; i++ {
		users[i] = uuid.New()
	}
	return users
}

func setupTestAccounts(t *testing.T, svc trading.TradingService, users []uuid.UUID, pair string) {
	// This would typically create accounts and fund them
	// Implementation depends on the actual trading service interface
	// For now, this is a placeholder that matches the existing pattern
}

func generateTestOrder(rng *rand.Rand, userID uuid.UUID, pair string, sequence int) *models.Order {
	side := "buy"
	if rng.Intn(2) == 1 {
		side = "sell"
	}

	basePrice := 50000.0
	priceVariation := rng.Float64() * 1000.0 // ±$1000 variation
	if side == "sell" {
		basePrice += priceVariation
	} else {
		basePrice -= priceVariation
	}

	return &models.Order{
		ID:          uuid.New(),
		UserID:      userID,
		Symbol:      pair,
		Side:        side,
		Type:        "limit",
		Price:       basePrice,
		Quantity:    0.001 + rng.Float64()*0.1, // 0.001 to 0.101 BTC
		TimeInForce: "GTC",
		Status:      "new",
	}
}

func printPoolPerformanceResults(testName, description string, result PoolPerformanceResult) {
	fmt.Printf("\n--- %s ---\n", testName)
	fmt.Printf("Description: %s\n", description)
	fmt.Printf("Execution Time: %v\n", result.ExecutionTime)
	fmt.Printf("Total Operations: %d\n", result.TotalOperations)
	fmt.Printf("Operations/Second: %.2f\n", result.OrdersPerSecond)
	fmt.Printf("Average Latency: %.3fms\n", result.AvgLatencyMs)
	fmt.Printf("P95 Latency: %.3fms\n", result.P95LatencyMs)
	fmt.Printf("P99 Latency: %.3fms\n", result.P99LatencyMs)
	fmt.Printf("Pool Hit Rate: %.2f%%\n", result.PoolHitRate)
	fmt.Printf("GC Pauses: %d\n", result.TotalGCPauses)
	fmt.Printf("Total GC Time: %v\n", result.TotalGCPauseTime)
	fmt.Printf("Avg GC Pause: %v\n", result.AvgGCPauseTime)
	fmt.Printf("Max GC Pause: %v\n", result.MaxGCPauseTime)
	fmt.Printf("Total Allocations: %d\n", result.TotalAllocations)
	fmt.Printf("Memory Used: %.2f MB\n", result.TotalMemoryUsedMB)
	fmt.Printf("Peak Memory: %.2f MB\n", result.PeakMemoryUsedMB)
}

func printGCResults(testName string, result PoolPerformanceResult) {
	fmt.Printf("\n--- %s GC Analysis ---\n", testName)
	fmt.Printf("Execution Time: %v\n", result.ExecutionTime)
	fmt.Printf("GC Pauses: %d\n", result.TotalGCPauses)
	fmt.Printf("Total GC Time: %v\n", result.TotalGCPauseTime)
	fmt.Printf("Avg GC Pause: %v\n", result.AvgGCPauseTime)
	fmt.Printf("Max GC Pause: %v\n", result.MaxGCPauseTime)
	fmt.Printf("Total Allocations: %d\n", result.TotalAllocations)
	fmt.Printf("Memory Used: %.2f MB\n", result.TotalMemoryUsedMB)
	if result.PoolHitRate > 0 {
		fmt.Printf("Pool Hit Rate: %.2f%%\n", result.PoolHitRate)
	}
}

func validatePoolPerformance(t *testing.T, testName string, result PoolPerformanceResult) {
	// Validate basic performance requirements
	assert.Greater(t, result.OrdersPerSecond, 1000.0, "%s: Operations per second too low", testName)
	assert.Less(t, result.AvgLatencyMs, 10.0, "%s: Average latency too high", testName)
	assert.Less(t, result.P95LatencyMs, 50.0, "%s: P95 latency too high", testName)

	// Validate pool efficiency
	if result.PoolHitRate > 0 {
		assert.Greater(t, result.PoolHitRate, 50.0, "%s: Pool hit rate too low", testName)
	}

	// Validate GC performance
	if result.TotalGCPauses > 0 {
		assert.Less(t, result.AvgGCPauseTime, 200*time.Microsecond, "%s: Average GC pause too long", testName)
		if result.MaxGCPauseTime > 0 {
			assert.Less(t, result.MaxGCPauseTime, 500*time.Microsecond, "%s: Max GC pause too long", testName)
		}
	}
}
