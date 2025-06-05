//go:build trading

package test

import (
	"context"
	"fmt"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Aidin1998/finalex/internal/trading"
	"github.com/Aidin1998/finalex/internal/trading/settlement"
	"github.com/Aidin1998/finalex/pkg/models"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/suite"
	"go.uber.org/zap"
)

// TradingStressLoadTestSuite provides comprehensive stress and load testing for trading operations
type TradingStressLoadTestSuite struct {
	suite.Suite
	service        trading.TradingService
	mockBookkeeper *MockBookkeeperStressTest
	mockWSHub      *MockWSHubStressTest
	testUsers      []string
	testPairs      []string
	ctx            context.Context
	cancel         context.CancelFunc

	// Metrics tracking
	metrics *StressTestMetrics
}

// StressTestMetrics tracks performance metrics during stress testing
type StressTestMetrics struct {
	OrdersSubmitted int64
	OrdersProcessed int64
	OrdersMatched   int64
	OrdersFailed    int64
	TradesExecuted  int64
	TotalLatency    int64
	MaxLatency      int64
	MinLatency      int64
	ErrorCount      int64
	StartTime       time.Time
	EndTime         time.Time
	mu              sync.RWMutex
}

func (m *StressTestMetrics) RecordOrderSubmission() {
	atomic.AddInt64(&m.OrdersSubmitted, 1)
}

func (m *StressTestMetrics) RecordOrderProcessed(latency time.Duration) {
	atomic.AddInt64(&m.OrdersProcessed, 1)
	latencyNs := latency.Nanoseconds()
	atomic.AddInt64(&m.TotalLatency, latencyNs)

	for {
		current := atomic.LoadInt64(&m.MaxLatency)
		if latencyNs <= current || atomic.CompareAndSwapInt64(&m.MaxLatency, current, latencyNs) {
			break
		}
	}

	for {
		current := atomic.LoadInt64(&m.MinLatency)
		if current == 0 {
			if atomic.CompareAndSwapInt64(&m.MinLatency, 0, latencyNs) {
				break
			}
			continue
		}
		if latencyNs >= current || atomic.CompareAndSwapInt64(&m.MinLatency, current, latencyNs) {
			break
		}
	}
}

func (m *StressTestMetrics) RecordOrderMatched() {
	atomic.AddInt64(&m.OrdersMatched, 1)
}

func (m *StressTestMetrics) RecordOrderFailed() {
	atomic.AddInt64(&m.OrdersFailed, 1)
}

func (m *StressTestMetrics) RecordTradeExecuted() {
	atomic.AddInt64(&m.TradesExecuted, 1)
}

func (m *StressTestMetrics) RecordError() {
	atomic.AddInt64(&m.ErrorCount, 1)
}

func (m *StressTestMetrics) GetSummary() map[string]interface{} {
	duration := m.EndTime.Sub(m.StartTime)
	ordersProcessed := atomic.LoadInt64(&m.OrdersProcessed)
	totalLatency := atomic.LoadInt64(&m.TotalLatency)

	avgLatency := int64(0)
	if ordersProcessed > 0 {
		avgLatency = totalLatency / ordersProcessed
	}

	return map[string]interface{}{
		"orders_submitted":  atomic.LoadInt64(&m.OrdersSubmitted),
		"orders_processed":  ordersProcessed,
		"orders_matched":    atomic.LoadInt64(&m.OrdersMatched),
		"orders_failed":     atomic.LoadInt64(&m.OrdersFailed),
		"trades_executed":   atomic.LoadInt64(&m.TradesExecuted),
		"error_count":       atomic.LoadInt64(&m.ErrorCount),
		"duration_seconds":  duration.Seconds(),
		"orders_per_second": float64(ordersProcessed) / duration.Seconds(),
		"avg_latency_ms":    float64(avgLatency) / 1e6,
		"max_latency_ms":    float64(atomic.LoadInt64(&m.MaxLatency)) / 1e6,
		"min_latency_ms":    float64(atomic.LoadInt64(&m.MinLatency)) / 1e6,
		"success_rate":      float64(ordersProcessed) / float64(atomic.LoadInt64(&m.OrdersSubmitted)),
	}
}

// SetupSuite initializes the test suite
func (suite *TradingStressLoadTestSuite) SetupSuite() {
	suite.ctx, suite.cancel = context.WithCancel(context.Background())

	// Initialize metrics
	suite.metrics = &StressTestMetrics{
		StartTime: time.Now(),
	}

	// Initialize mock services
	suite.mockBookkeeper = &MockBookkeeperStressTest{}
	suite.mockWSHub = &MockWSHubStressTest{}

	// Create test users
	suite.testUsers = make([]string, 1000) // Large user base for stress testing
	for i := 0; i < len(suite.testUsers); i++ {
		userID := uuid.New()
		suite.testUsers[i] = userID.String()

		// Set up initial balances for stress testing
		suite.mockBookkeeper.SetBalance(suite.testUsers[i], "BTC", decimal.NewFromFloat(10.0))
		suite.mockBookkeeper.SetBalance(suite.testUsers[i], "USDT", decimal.NewFromFloat(100000.0))
		suite.mockBookkeeper.SetBalance(suite.testUsers[i], "ETH", decimal.NewFromFloat(50.0))

		// Connect users to WebSocket
		suite.mockWSHub.Connect(suite.testUsers[i])
	}

	// Test trading pairs
	suite.testPairs = []string{"BTCUSDT", "ETHUSDT", "ETHBTC"}

	// Initialize logger and database
	logger, _ := zap.NewDevelopment()
	db := createInMemoryDB()

	// Create settlement engine
	settlementEngine := (*settlement.SettlementEngine)(nil)

	// Create trading service with real implementation
	tradingService, err := trading.NewService(logger, db, suite.mockBookkeeper, suite.mockWSHub, settlementEngine)
	if err != nil {
		suite.Fail("Failed to create trading service: %v", err)
	}
	suite.service = tradingService
}

// TearDownSuite cleans up after all tests
func (suite *TradingStressLoadTestSuite) TearDownSuite() {
	suite.metrics.EndTime = time.Now()

	// Print final metrics
	summary := suite.metrics.GetSummary()
	fmt.Println("\n=== STRESS TEST FINAL METRICS ===")
	for key, value := range summary {
		fmt.Printf("%s: %v\n", key, value)
	}

	suite.cancel()
}

// TestHighVolumeOrderProcessing tests processing of high volumes of orders
func (suite *TradingStressLoadTestSuite) TestHighVolumeOrderProcessing() {
	suite.Run("ConcurrentOrderSubmission", func() {
		numWorkers := runtime.NumCPU() * 2
		ordersPerWorker := 1000
		totalOrders := numWorkers * ordersPerWorker

		fmt.Printf("Testing high volume order processing: %d workers, %d orders each (%d total)\n",
			numWorkers, ordersPerWorker, totalOrders)

		var wg sync.WaitGroup
		startTime := time.Now()

		// Launch concurrent workers
		for i := 0; i < numWorkers; i++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()

				for j := 0; j < ordersPerWorker; j++ {
					suite.submitRandomOrder(workerID, j)
				}
			}(i)
		}

		wg.Wait()
		duration := time.Since(startTime)

		ordersPerSecond := float64(totalOrders) / duration.Seconds()

		fmt.Printf("High volume test completed: %d orders in %v (%.2f orders/sec)\n",
			totalOrders, duration, ordersPerSecond)

		// Verify performance benchmarks
		suite.Assert().Greater(ordersPerSecond, 1000.0, "Should process at least 1000 orders per second")
		suite.Assert().Less(duration.Seconds(), 30.0, "Should complete within 30 seconds")
	})
}

// TestConcurrentMatching tests concurrent order matching under high load
func (suite *TradingStressLoadTestSuite) TestConcurrentMatching() {
	suite.Run("HighFrequencyMatching", func() {
		numPairs := 3
		ordersPerPair := 500

		fmt.Printf("Testing concurrent matching: %d pairs, %d orders each\n", numPairs, ordersPerPair)

		var wg sync.WaitGroup
		startTime := time.Now()

		// Submit orders for multiple pairs concurrently
		for pairIndex, pair := range suite.testPairs[:numPairs] {
			wg.Add(1)
			go func(pair string, pairIdx int) {
				defer wg.Done()
				suite.submitMatchingOrdersForPair(pair, ordersPerPair)
			}(pair, pairIndex)
		}

		wg.Wait()
		duration := time.Since(startTime)

		fmt.Printf("Concurrent matching test completed in %v\n", duration)

		// Verify matching performance
		suite.Assert().Less(duration.Seconds(), 20.0, "Should complete concurrent matching within 20 seconds")
		suite.Assert().Greater(atomic.LoadInt64(&suite.metrics.TradesExecuted), int64(0), "Should execute trades")
	})
}

// TestMemoryUsageUnderLoad tests memory usage under sustained load
func (suite *TradingStressLoadTestSuite) TestMemoryUsageUnderLoad() {
	suite.Run("SustainedLoadMemoryTest", func() {
		runtime.GC() // Start with clean memory
		var m1 runtime.MemStats
		runtime.ReadMemStats(&m1)
		initialMemory := m1.Alloc

		fmt.Printf("Initial memory usage: %d bytes\n", initialMemory)

		// Submit sustained load for extended period
		duration := 30 * time.Second
		orderRate := 100 // orders per second

		startTime := time.Now()
		orderCount := 0

		ticker := time.NewTicker(time.Duration(1000/orderRate) * time.Millisecond)
		defer ticker.Stop()

		for time.Since(startTime) < duration {
			select {
			case <-ticker.C:
				suite.submitRandomOrder(0, orderCount)
				orderCount++
			case <-suite.ctx.Done():
				return
			}
		}

		runtime.GC() // Force garbage collection
		var m2 runtime.MemStats
		runtime.ReadMemStats(&m2)
		finalMemory := m2.Alloc

		memoryIncrease := finalMemory - initialMemory
		memoryIncreasePercent := float64(memoryIncrease) / float64(initialMemory) * 100

		fmt.Printf("Final memory usage: %d bytes (increase: %d bytes, %.2f%%)\n",
			finalMemory, memoryIncrease, memoryIncreasePercent)

		// Verify memory usage stays reasonable
		suite.Assert().Less(memoryIncreasePercent, 200.0, "Memory increase should be less than 200%")
	})
}

// TestLatencyUnderLoad tests response latency under various load conditions
func (suite *TradingStressLoadTestSuite) TestLatencyUnderLoad() {
	suite.Run("LatencyBenchmark", func() {
		loadLevels := []int{10, 50, 100, 500, 1000} // orders per second

		for _, orderRate := range loadLevels {
			suite.Run(fmt.Sprintf("Load_%d_OPS", orderRate), func() {
				suite.measureLatencyAtLoad(orderRate, 10*time.Second)
			})
		}
	})
}

// TestMarketDataBroadcastPerformance tests WebSocket broadcast performance under load
func (suite *TradingStressLoadTestSuite) TestMarketDataBroadcastPerformance() {
	suite.Run("HighFrequencyBroadcast", func() {
		numClients := 1000
		messagesPerSecond := 500
		testDuration := 10 * time.Second

		fmt.Printf("Testing broadcast performance: %d clients, %d msg/sec for %v\n",
			numClients, messagesPerSecond, testDuration)

		// Connect additional clients for broadcast testing
		broadcastClients := make([]string, numClients)
		for i := 0; i < numClients; i++ {
			broadcastClients[i] = fmt.Sprintf("broadcast_client_%d", i)
			suite.mockWSHub.Connect(broadcastClients[i])
		}

		startTime := time.Now()
		messageCount := 0

		ticker := time.NewTicker(time.Duration(1000/messagesPerSecond) * time.Millisecond)
		defer ticker.Stop()

		for time.Since(startTime) < testDuration {
			select {
			case <-ticker.C:
				message := fmt.Sprintf(`{"type":"trade","pair":"BTCUSDT","price":"50000","qty":"0.001","time":%d}`,
					time.Now().UnixNano())
				suite.mockWSHub.Broadcast("trades.BTCUSDT", []byte(message))
				messageCount++
			case <-suite.ctx.Done():
				return
			}
		}

		actualDuration := time.Since(startTime)
		actualRate := float64(messageCount) / actualDuration.Seconds()

		fmt.Printf("Broadcast test completed: %d messages in %v (%.2f msg/sec)\n",
			messageCount, actualDuration, actualRate)

		// Verify broadcast performance
		suite.Assert().Greater(actualRate, float64(messagesPerSecond)*0.9, "Should achieve 90% of target broadcast rate")

		// Verify message delivery
		broadcasts := suite.mockWSHub.GetBroadcasts("trades.BTCUSDT")
		suite.Assert().GreaterOrEqual(len(broadcasts), messageCount, "All broadcasts should be recorded")
	})
}

// Helper methods for stress testing

// submitRandomOrder submits a random order for stress testing
func (suite *TradingStressLoadTestSuite) submitRandomOrder(workerID, orderID int) {
	suite.metrics.RecordOrderSubmission()

	userIndex := (workerID + orderID) % len(suite.testUsers)
	userID := suite.testUsers[userIndex]
	pair := suite.testPairs[orderID%len(suite.testPairs)]
	// Generate random order parameters
	sides := []string{"BUY", "SELL"}
	side := sides[orderID%2]

	orderTypes := []string{"LIMIT", "MARKET"}
	orderType := orderTypes[orderID%2]

	var price float64
	var quantity float64

	switch pair {
	case "BTCUSDT":
		price = 45000 + float64(orderID%10000)         // Price range: 45000-55000
		quantity = 0.001 + float64(orderID%100)/100000 // Small quantities	case "ETHUSDT":
		price = 2800 + float64(orderID%1000)           // Price range: 2800-3800
		quantity = 0.01 + float64(orderID%100)/10000
	case "ETHBTC":
		price = 0.065 + float64(orderID%100)/100000 // Price range around 0.065-0.075
		quantity = 0.1 + float64(orderID%100)/1000
	}

	order := &models.Order{
		ID:          uuid.New(),
		UserID:      uuid.MustParse(userID),
		Symbol:      pair,
		Side:        side,
		Type:        orderType,
		Price:       price,
		Quantity:    quantity,
		Status:      "NEW",
		Created:     time.Now(),
		TimeInForce: "GTC",
	}

	// Submit order
	_, err := suite.service.PlaceOrder(suite.ctx, order)
	if err != nil {
		suite.metrics.RecordOrderFailed()
	}
}

// submitMatchingOrdersForPair submits matching buy/sell orders for a specific trading pair
func (suite *TradingStressLoadTestSuite) submitMatchingOrdersForPair(pair string, numOrders int) {
	basePrice := suite.getBasePriceForPair(pair)

	for i := 0; i < numOrders; i++ { // Submit buy order
		buyUserIndex := (i * 2) % len(suite.testUsers)
		buyOrder := &models.Order{
			ID:          uuid.New(),
			UserID:      uuid.MustParse(suite.testUsers[buyUserIndex]),
			Symbol:      pair,
			Side:        "BUY",
			Type:        "LIMIT",
			Price:       basePrice + float64(i%50), // Slightly increasing prices
			Quantity:    0.001 + float64(i%100)/100000,
			Status:      "NEW",
			Created:     time.Now(),
			TimeInForce: "GTC",
		}

		// Submit sell order
		sellUserIndex := (i*2 + 1) % len(suite.testUsers)
		sellOrder := &models.Order{
			ID:          uuid.New(),
			UserID:      uuid.MustParse(suite.testUsers[sellUserIndex]),
			Symbol:      pair,
			Side:        "SELL",
			Type:        "LIMIT",
			Price:       basePrice + float64(i%50), // Same price for matching
			Quantity:    0.001 + float64(i%100)/100000,
			Status:      "NEW",
			Created:     time.Now(),
			TimeInForce: "GTC",
		}

		suite.metrics.RecordOrderSubmission()
		suite.service.PlaceOrder(suite.ctx, buyOrder)

		suite.metrics.RecordOrderSubmission()
		suite.service.PlaceOrder(suite.ctx, sellOrder)
	}
}

// getBasePriceForPair returns the base price for different trading pairs
func (suite *TradingStressLoadTestSuite) getBasePriceForPair(pair string) float64 {
	switch pair {
	case "BTCUSDT":
		return 50000.0
	case "ETHUSDT":
		return 3000.0
	case "ETHBTC":
		return 0.07
	default:
		return 1.0
	}
}

// measureLatencyAtLoad measures latency at a specific order submission rate
func (suite *TradingStressLoadTestSuite) measureLatencyAtLoad(orderRate int, duration time.Duration) {
	fmt.Printf("Measuring latency at %d orders/sec for %v\n", orderRate, duration)

	var latencies []time.Duration
	var mu sync.Mutex

	startTime := time.Now()
	orderCount := 0

	ticker := time.NewTicker(time.Duration(1000/orderRate) * time.Millisecond)
	defer ticker.Stop()

	for time.Since(startTime) < duration {
		select {
		case <-ticker.C:
			go func(orderNum int) {
				orderStart := time.Now()
				suite.submitRandomOrder(0, orderNum)
				latency := time.Since(orderStart)

				mu.Lock()
				latencies = append(latencies, latency)
				mu.Unlock()
			}(orderCount)
			orderCount++
		case <-suite.ctx.Done():
			return
		}
	}

	// Wait for remaining orders to complete
	time.Sleep(time.Second)

	if len(latencies) > 0 {
		// Calculate latency statistics
		sort.Slice(latencies, func(i, j int) bool {
			return latencies[i] < latencies[j]
		})

		avgLatency := time.Duration(0)
		for _, lat := range latencies {
			avgLatency += lat
		}
		avgLatency /= time.Duration(len(latencies))

		p50 := latencies[len(latencies)/2]
		p95 := latencies[int(float64(len(latencies))*0.95)]
		p99 := latencies[int(float64(len(latencies))*0.99)]

		fmt.Printf("Latency at %d OPS - Avg: %v, P50: %v, P95: %v, P99: %v\n",
			orderRate, avgLatency, p50, p95, p99)

		// Verify latency requirements
		suite.Assert().Less(avgLatency.Milliseconds(), int64(100), "Average latency should be under 100ms")
		suite.Assert().Less(p95.Milliseconds(), int64(200), "P95 latency should be under 200ms")
		suite.Assert().Less(p99.Milliseconds(), int64(500), "P99 latency should be under 500ms")
	}
}
