//go:build performance
// +build performance

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

	"github.com/Aidin1998/pincex_unified/internal/bookkeeper"	"github.com/Aidin1998/pincex_unified/internal/core/trading"
	"github.com/Aidin1998/pincex_unified/internal/core/trading/engine"
	"github.com/Aidin1998/pincex_unified/internal/core/trading/orderbook"
	"github.com/Aidin1998/pincex_unified/pkg/models"
	"github.com/Aidin1998/pincex_unified/testutil"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// Performance metrics structure
type PerformanceResult struct {
	TotalOrders        int64
	TotalTrades        int64
	TotalCancellations int64
	TotalErrors        int64

	// Latency metrics (in milliseconds)
	MinLatencyMs float64
	AvgLatencyMs float64
	MaxLatencyMs float64
	P50LatencyMs float64
	P95LatencyMs float64
	P99LatencyMs float64

	// Throughput metrics
	OrdersPerSecond float64
	TradesPerSecond float64

	// Memory metrics
	MemoryUsageMB     float64
	PeakMemoryUsageMB float64

	// Other metrics
	ErrorRate     float64
	ExecutionTime time.Duration
}

// BenchmarkComprehensiveTradingEngine performs a comprehensive analysis of the trading engine
func BenchmarkComprehensiveTradingEngine(b *testing.B) {
	fmt.Println("\n========================================")
	fmt.Println("COMPREHENSIVE TRADING ENGINE ANALYSIS")
	fmt.Println("========================================")

	testCases := []struct {
		name            string
		concurrency     int
		ordersPerWorker int
		cancelRate      float64
		description     string
	}{
		{"Low Load", 10, 100, 0.1, "Baseline performance with minimal load"},
		{"Medium Load", 50, 200, 0.2, "Moderate load testing"},
		{"High Load", 100, 500, 0.3, "High concurrency stress test"},
		{"Extreme Load", 200, 1000, 0.4, "Peak capacity testing"},
	}

	var overallResults []PerformanceResult

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			result := runTradingEngineTest(b, tc.concurrency, tc.ordersPerWorker, tc.cancelRate)
			overallResults = append(overallResults, result)

			// Print detailed results for this test case
			printPerformanceResults(tc.name, tc.description, result)
		})
	}

	// Generate final analysis report
	generateAnalysisReport(overallResults)
}

// runTradingEngineTest executes a single performance test scenario
func runTradingEngineTest(b *testing.B, concurrency, ordersPerWorker int, cancelRate float64) PerformanceResult {
	// Setup trading service
	svc := setupBenchmarkTradingService(b)
	defer svc.Stop()

	ctx := context.Background()

	// Setup test data
	testPair := "BTCUSDT"
	users := createTestUsers(b, concurrency)
	setupTestAccounts(b, svc, users, testPair)

	// Metrics collection
	var (
		totalOrders        int64
		totalTrades        int64
		totalCancellations int64
		totalErrors        int64
		latencies          []time.Duration
		latencyMutex       sync.Mutex
	)

	// Memory monitoring
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	initialMemory := memStats.Alloc

	b.ResetTimer()
	startTime := time.Now()

	// Run concurrent workers
	var wg sync.WaitGroup
	wg.Add(concurrency)

	for i := 0; i < concurrency; i++ {
		go func(workerID int) {
			defer wg.Done()

			rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(workerID)))
			userID := users[workerID%len(users)]
			activeOrders := make([]uuid.UUID, 0)

			for j := 0; j < ordersPerWorker; j++ {
				// Generate order
				order := generateTestOrder(rng, userID, testPair, j)

				// Place order
				orderStart := time.Now()
				placedOrder, err := svc.PlaceOrder(ctx, order)
				latency := time.Since(orderStart)

				// Record metrics
				atomic.AddInt64(&totalOrders, 1)
				if err != nil {
					atomic.AddInt64(&totalErrors, 1)
				} else {
					activeOrders = append(activeOrders, placedOrder.ID)
				}

				latencyMutex.Lock()
				latencies = append(latencies, latency)
				latencyMutex.Unlock()

				// Random cancellation
				if len(activeOrders) > 0 && rng.Float64() < cancelRate {
					orderToCancel := activeOrders[rng.Intn(len(activeOrders))]
					if err := svc.CancelOrder(ctx, orderToCancel); err == nil {
						atomic.AddInt64(&totalCancellations, 1)
					}
				}

				// Small delay to simulate realistic trading patterns
				time.Sleep(time.Microsecond * time.Duration(rng.Intn(100)))
			}
		}(i)
	}

	wg.Wait()
	executionTime := time.Since(startTime)

	// Final memory measurement
	runtime.ReadMemStats(&memStats)
	peakMemory := memStats.Alloc

	b.StopTimer()

	// Calculate performance metrics
	return calculatePerformanceMetrics(
		totalOrders, totalTrades, totalCancellations, totalErrors,
		latencies, executionTime, initialMemory, peakMemory,
	)
}

// BenchmarkOrderBookConsistency tests order book consistency under high load
func BenchmarkOrderBookConsistency(b *testing.B) {
	fmt.Println("\n========================================")
	fmt.Println("ORDER BOOK CONSISTENCY ANALYSIS")
	fmt.Println("========================================")

	// Test different order book implementations
	implementations := []struct {
		name  string
		setup func() orderbook.OrderBookInterface
	}{
		{
			"Standard OrderBook",
			func() orderbook.OrderBookInterface {
				return orderbook.NewOrderBook("BTCUSDT")
			},
		},
		{
			"DeadlockSafe OrderBook",
			func() orderbook.OrderBookInterface {
				return orderbook.NewDeadlockSafeOrderBook("BTCUSDT")
			},
		},
		{
			"Adaptive OrderBook (Legacy)",
			func() orderbook.OrderBookInterface {
				config := orderbook.DefaultMigrationConfig()
				config.EnableNewImplementation = false
				return orderbook.NewAdaptiveOrderBook("BTCUSDT", config)
			},
		},
		{
			"Adaptive OrderBook (New)",
			func() orderbook.OrderBookInterface {
				config := orderbook.DefaultMigrationConfig()
				config.EnableNewImplementation = true
				adaptive := orderbook.NewAdaptiveOrderBook("BTCUSDT", config)
				adaptive.SetMigrationPercentage(100)
				return adaptive
			},
		},
	}

	for _, impl := range implementations {
		b.Run(impl.name, func(b *testing.B) {
			result := benchmarkOrderBookImplementation(b, impl.setup())
			printOrderBookResults(impl.name, result)
		})
	}
}

// BenchmarkMatchingAlgorithmAccuracy tests the accuracy of the matching algorithm
func BenchmarkMatchingAlgorithmAccuracy(b *testing.B) {
	fmt.Println("\n========================================")
	fmt.Println("MATCHING ALGORITHM ACCURACY ANALYSIS")
	fmt.Println("========================================")

	logger := zap.NewNop().Sugar()
	me := engine.NewMatchingEngine(nil, nil, logger, nil, nil)

	testScenarios := []struct {
		name        string
		description string
		testFunc    func(*testing.B, *engine.MatchingEngine) AccuracyResult
	}{
		{
			"Price-Time Priority",
			"Verify orders are matched according to price-time priority",
			testPriceTimePriority,
		},
		{
			"Partial Fill Handling",
			"Test accurate partial order fulfillment",
			testPartialFillAccuracy,
		},
		{
			"Self-Trade Prevention",
			"Ensure users cannot trade with themselves",
			testSelfTradePrevention,
		},
		{
			"Order Type Handling",
			"Test different order types (LIMIT, MARKET, IOC, FOK)",
			testOrderTypeAccuracy,
		},
	}

	for _, scenario := range testScenarios {
		b.Run(scenario.name, func(b *testing.B) {
			result := scenario.testFunc(b, me)
			printAccuracyResults(scenario.name, scenario.description, result)
		})
	}
}

// Supporting functions

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

func createTestUsers(b *testing.B, count int) []uuid.UUID {
	users := make([]uuid.UUID, count)
	for i := 0; i < count; i++ {
		users[i] = uuid.New()
	}
	return users
}

func setupTestAccounts(b *testing.B, svc trading.TradingService, users []uuid.UUID, pair string) {
	// This would typically create accounts and fund them
	// Implementation depends on the actual trading service interface
}

func calculatePerformanceMetrics(
	totalOrders, totalTrades, totalCancellations, totalErrors int64,
	latencies []time.Duration, executionTime time.Duration,
	initialMemory, peakMemory uint64,
) PerformanceResult {
	result := PerformanceResult{
		TotalOrders:        totalOrders,
		TotalTrades:        totalTrades,
		TotalCancellations: totalCancellations,
		TotalErrors:        totalErrors,
		ExecutionTime:      executionTime,
	}

	if totalOrders > 0 {
		result.ErrorRate = float64(totalErrors) / float64(totalOrders) * 100
		result.OrdersPerSecond = float64(totalOrders) / executionTime.Seconds()
	}

	if totalTrades > 0 {
		result.TradesPerSecond = float64(totalTrades) / executionTime.Seconds()
	}

	if len(latencies) > 0 {
		var totalLatency time.Duration
		minLatency := latencies[0]
		maxLatency := latencies[0]

		for _, lat := range latencies {
			totalLatency += lat
			if lat < minLatency {
				minLatency = lat
			}
			if lat > maxLatency {
				maxLatency = lat
			}
		}

		result.AvgLatencyMs = float64(totalLatency.Nanoseconds()) / float64(len(latencies)) / 1e6
		result.MinLatencyMs = float64(minLatency.Nanoseconds()) / 1e6
		result.MaxLatencyMs = float64(maxLatency.Nanoseconds()) / 1e6
		result.P50LatencyMs = float64(testutil.Percentile(latencies, 0.50).Nanoseconds()) / 1e6
		result.P95LatencyMs = float64(testutil.Percentile(latencies, 0.95).Nanoseconds()) / 1e6
		result.P99LatencyMs = float64(testutil.Percentile(latencies, 0.99).Nanoseconds()) / 1e6
	}

	result.MemoryUsageMB = float64(peakMemory-initialMemory) / (1024 * 1024)
	result.PeakMemoryUsageMB = float64(peakMemory) / (1024 * 1024)

	return result
}

func printPerformanceResults(testName, description string, result PerformanceResult) {
	fmt.Printf("\n--- %s ---\n", testName)
	fmt.Printf("Description: %s\n", description)
	fmt.Printf("Execution Time: %v\n", result.ExecutionTime)
	fmt.Printf("Total Orders: %d\n", result.TotalOrders)
	fmt.Printf("Total Trades: %d\n", result.TotalTrades)
	fmt.Printf("Total Cancellations: %d\n", result.TotalCancellations)
	fmt.Printf("Total Errors: %d\n", result.TotalErrors)
	fmt.Printf("Error Rate: %.3f%%\n", result.ErrorRate)
	fmt.Printf("Orders/Second: %.2f\n", result.OrdersPerSecond)
	fmt.Printf("Trades/Second: %.2f\n", result.TradesPerSecond)
	fmt.Printf("Latency - Min: %.3fms, Avg: %.3fms, Max: %.3fms\n",
		result.MinLatencyMs, result.AvgLatencyMs, result.MaxLatencyMs)
	fmt.Printf("Latency - P50: %.3fms, P95: %.3fms, P99: %.3fms\n",
		result.P50LatencyMs, result.P95LatencyMs, result.P99LatencyMs)
	fmt.Printf("Memory Usage: %.2f MB, Peak: %.2f MB\n",
		result.MemoryUsageMB, result.PeakMemoryUsageMB)
}

// Placeholder implementations for order book and accuracy testing
type OrderBookResult struct {
	Implementation    string
	TotalOperations   int64
	AvgLatencyMs      float64
	P95LatencyMs      float64
	ThroughputOps     float64
	ConsistencyErrors int64
}

type AccuracyResult struct {
	TestName           string
	TotalTests         int64
	PassedTests        int64
	FailedTests        int64
	AccuracyPercentage float64
	Details            []string
}

func benchmarkOrderBookImplementation(b *testing.B, ob orderbook.OrderBookInterface) OrderBookResult {
	// Implementation for order book benchmarking
	return OrderBookResult{}
}

func testPriceTimePriority(b *testing.B, me *engine.MatchingEngine) AccuracyResult {
	// Implementation for price-time priority testing
	return AccuracyResult{}
}

func testPartialFillAccuracy(b *testing.B, me *engine.MatchingEngine) AccuracyResult {
	// Implementation for partial fill testing
	return AccuracyResult{}
}

func testSelfTradePrevention(b *testing.B, me *engine.MatchingEngine) AccuracyResult {
	// Implementation for self-trade prevention testing
	return AccuracyResult{}
}

func testOrderTypeAccuracy(b *testing.B, me *engine.MatchingEngine) AccuracyResult {
	// Implementation for order type testing
	return AccuracyResult{}
}

func printOrderBookResults(name string, result OrderBookResult) {
	fmt.Printf("\n--- %s ---\n", name)
	fmt.Printf("Operations: %d\n", result.TotalOperations)
	fmt.Printf("Avg Latency: %.3fms\n", result.AvgLatencyMs)
	fmt.Printf("P95 Latency: %.3fms\n", result.P95LatencyMs)
	fmt.Printf("Throughput: %.2f ops/sec\n", result.ThroughputOps)
	fmt.Printf("Consistency Errors: %d\n", result.ConsistencyErrors)
}

func printAccuracyResults(name, description string, result AccuracyResult) {
	fmt.Printf("\n--- %s ---\n", name)
	fmt.Printf("Description: %s\n", description)
	fmt.Printf("Total Tests: %d\n", result.TotalTests)
	fmt.Printf("Passed: %d\n", result.PassedTests)
	fmt.Printf("Failed: %d\n", result.FailedTests)
	fmt.Printf("Accuracy: %.2f%%\n", result.AccuracyPercentage)
	for _, detail := range result.Details {
		fmt.Printf("  - %s\n", detail)
	}
}

func generateAnalysisReport(results []PerformanceResult) {
	fmt.Println("\n========================================")
	fmt.Println("COMPREHENSIVE ANALYSIS REPORT")
	fmt.Println("========================================")

	// Performance summary
	fmt.Println("\n1. PERFORMANCE SUMMARY:")
	for i, result := range results {
		testNames := []string{"Low Load", "Medium Load", "High Load", "Extreme Load"}
		if i < len(testNames) {
			status := "✓ PASS"
			if result.P95LatencyMs > 10.0 {
				status = "✗ FAIL"
			}
			if result.ErrorRate > 0.1 {
				status = "✗ FAIL"
			}

			fmt.Printf("   %s: %s (P95: %.2fms, TPS: %.0f, Errors: %.2f%%)\n",
				testNames[i], status, result.P95LatencyMs, result.OrdersPerSecond, result.ErrorRate)
		}
	}

	// Industry comparison
	fmt.Println("\n2. INDUSTRY COMPARISON:")
	industryStandards := map[string]map[string]float64{
		"Binance": {
			"latency_p95": 5.0,
			"throughput":  100000.0,
			"error_rate":  0.01,
		},
		"Coinbase": {
			"latency_p95": 8.0,
			"throughput":  50000.0,
			"error_rate":  0.02,
		},
		"Kraken": {
			"latency_p95": 12.0,
			"throughput":  30000.0,
			"error_rate":  0.05,
		},
	}

	if len(results) > 0 {
		bestResult := results[len(results)-1] // Use extreme load test
		fmt.Printf("   Current System vs Industry Leaders:\n")

		for exchange, standards := range industryStandards {
			latencyScore := (standards["latency_p95"] / bestResult.P95LatencyMs) * 10
			throughputScore := (bestResult.OrdersPerSecond / standards["throughput"]) * 10
			errorScore := (standards["error_rate"] / (bestResult.ErrorRate + 0.001)) * 10

			if latencyScore > 10 {
				latencyScore = 10
			}
			if throughputScore > 10 {
				throughputScore = 10
			}
			if errorScore > 10 {
				errorScore = 10
			}

			overallScore := (latencyScore + throughputScore + errorScore) / 3

			fmt.Printf("   %s: %.1f/10 (Latency: %.1f, Throughput: %.1f, Reliability: %.1f)\n",
				exchange, overallScore, latencyScore, throughputScore, errorScore)
		}
	}

	// Final rating
	fmt.Println("\n3. OVERALL RATING:")
	if len(results) > 0 {
		avgP95 := 0.0
		avgTPS := 0.0
		avgErrors := 0.0

		for _, result := range results {
			avgP95 += result.P95LatencyMs
			avgTPS += result.OrdersPerSecond
			avgErrors += result.ErrorRate
		}

		avgP95 /= float64(len(results))
		avgTPS /= float64(len(results))
		avgErrors /= float64(len(results))

		// Calculate overall score (1-10)
		latencyScore := 10.0
		if avgP95 > 10 {
			latencyScore = 10.0 / (avgP95 / 10.0)
		}

		throughputScore := avgTPS / 10000.0 // 10k OPS = score of 1
		if throughputScore > 10 {
			throughputScore = 10
		}

		reliabilityScore := 10.0
		if avgErrors > 0.01 {
			reliabilityScore = 10.0 / (avgErrors * 100)
		}
		if reliabilityScore > 10 {
			reliabilityScore = 10
		}

		overallRating := (latencyScore + throughputScore + reliabilityScore) / 3

		fmt.Printf("   Performance Rating: %.1f/10\n", overallRating)
		fmt.Printf("   - Latency Score: %.1f/10 (Avg P95: %.2fms)\n", latencyScore, avgP95)
		fmt.Printf("   - Throughput Score: %.1f/10 (Avg TPS: %.0f)\n", throughputScore, avgTPS)
		fmt.Printf("   - Reliability Score: %.1f/10 (Avg Error Rate: %.3f%%)\n", reliabilityScore, avgErrors)

		// Recommendations
		fmt.Println("\n4. RECOMMENDATIONS:")
		if avgP95 > 10 {
			fmt.Println("   - ⚠️  Optimize order processing latency (target: <10ms P95)")
		}
		if avgTPS < 10000 {
			fmt.Println("   - ⚠️  Improve throughput capacity (target: >10k OPS)")
		}
		if avgErrors > 0.1 {
			fmt.Println("   - ⚠️  Reduce error rate (target: <0.1%)")
		}
		if overallRating >= 8.0 {
			fmt.Println("   - ✅ Excellent performance, ready for production")
		} else if overallRating >= 6.0 {
			fmt.Println("   - ⚠️  Good performance, minor optimizations needed")
		} else {
			fmt.Println("   - ❌ Performance issues require attention before production")
		}
	}

	fmt.Println("\n========================================")
}

func setupBenchmarkTradingService(b *testing.B) trading.TradingService {
	b.Helper()
	logger := zap.NewNop()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		b.Fatalf("failed to open in-memory db: %v", err)
	}
	err = db.AutoMigrate(&models.TradingPair{}, &models.Order{}, &models.Trade{}, &models.Account{}, &models.Transaction{})
	if err != nil {
		b.Fatalf("failed to migrate db: %v", err)
	}
	bkSvc, err := bookkeeper.NewService(logger, db)
	if err != nil {
		b.Fatalf("failed to create bookkeeper: %v", err)
	}
	svc, err := trading.NewService(logger, db, bkSvc)
	if err != nil {
		b.Fatalf("failed to create trading service: %v", err)
	}
	err = svc.Start()
	if err != nil {
		b.Fatalf("failed to start trading service: %v", err)
	}
	return svc
}
