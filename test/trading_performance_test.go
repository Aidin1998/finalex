//go:build trading

package test

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Aidin1998/finalex/internal/trading"
	"github.com/Aidin1998/finalex/pkg/models"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/suite"
)

// TradingPerformanceTestSuite provides high-performance testing for trading operations
type TradingPerformanceTestSuite struct {
	suite.Suite
	service        trading.TradingService
	mockBookkeeper *MockBookkeeperPerformance
	mockWSHub      *MockWSHubPerformance
	testUsers      []string
	testPairs      []string
	ctx            context.Context
	cancel         context.CancelFunc
}

// MockBookkeeperPerformance provides high-performance mock for bookkeeper service
type MockBookkeeperPerformance struct {
	balances     sync.Map // userID -> map[asset]decimal.Decimal
	reservations sync.Map // reservationID -> ReservationInfo
	totalOps     int64
	mu           sync.RWMutex
}

type ReservationInfo struct {
	UserID string
	Asset  string
	Amount decimal.Decimal
}

func (m *MockBookkeeperPerformance) GetBalance(userID, asset string) (decimal.Decimal, error) {
	atomic.AddInt64(&m.totalOps, 1)

	userBalances, exists := m.balances.Load(userID)
	if !exists {
		return decimal.NewFromInt(1000000), nil // Default high balance for performance testing
	}

	balanceMap := userBalances.(map[string]decimal.Decimal)
	if balance, exists := balanceMap[asset]; exists {
		return balance, nil
	}

	return decimal.NewFromInt(1000000), nil // Default high balance
}

func (m *MockBookkeeperPerformance) ReserveBalance(userID, asset string, amount decimal.Decimal) (string, error) {
	atomic.AddInt64(&m.totalOps, 1)

	reservationID := fmt.Sprintf("res_%d_%s", time.Now().UnixNano(), userID)
	m.reservations.Store(reservationID, ReservationInfo{
		UserID: userID,
		Asset:  asset,
		Amount: amount,
	})

	return reservationID, nil
}

func (m *MockBookkeeperPerformance) ReleaseReservation(reservationID string) error {
	atomic.AddInt64(&m.totalOps, 1)
	m.reservations.Delete(reservationID)
	return nil
}

func (m *MockBookkeeperPerformance) TransferReservedBalance(reservationID, toUserID string) error {
	atomic.AddInt64(&m.totalOps, 1)
	m.reservations.Delete(reservationID)
	return nil
}

func (m *MockBookkeeperPerformance) GetTotalOps() int64 {
	return atomic.LoadInt64(&m.totalOps)
}

// MockWSHubPerformance provides high-performance mock for WebSocket hub
type MockWSHubPerformance struct {
	messagesSent int64
	subscribers  sync.Map // topic -> count
}

func (m *MockWSHubPerformance) PublishToUser(userID string, data interface{}) {
	atomic.AddInt64(&m.messagesSent, 1)
}

func (m *MockWSHubPerformance) PublishToTopic(topic string, data interface{}) {
	atomic.AddInt64(&m.messagesSent, 1)
}

func (m *MockWSHubPerformance) SubscribeToTopic(userID, topic string) {
	count, _ := m.subscribers.LoadOrStore(topic, int64(0))
	atomic.AddInt64(count.(*int64), 1)
}

func (m *MockWSHubPerformance) GetMessagesSent() int64 {
	return atomic.LoadInt64(&m.messagesSent)
}

func (suite *TradingPerformanceTestSuite) SetupSuite() {
	log.Println("Setting up trading performance test suite...")

	suite.ctx, suite.cancel = context.WithTimeout(context.Background(), 10*time.Minute)

	// Initialize high-performance mocks
	suite.mockBookkeeper = &MockBookkeeperPerformance{}
	suite.mockWSHub = &MockWSHubPerformance{}

	// Create trading service with performance configuration
	suite.service = trading.NewService(
		suite.mockBookkeeper,
		suite.mockWSHub,
		trading.WithHighPerformanceMode(true),
		trading.WithMaxConcurrentOrders(100000),
		trading.WithOrderBookDepth(10000),
	)

	// Setup test data
	suite.setupTestData()

	err := suite.service.Start(suite.ctx)
	suite.Require().NoError(err, "Failed to start trading service")

	log.Printf("Performance test setup complete - GOMAXPROCS: %d", runtime.GOMAXPROCS(0))
}

func (suite *TradingPerformanceTestSuite) TearDownSuite() {
	if suite.cancel != nil {
		suite.cancel()
	}
	if suite.service != nil {
		suite.service.Stop()
	}
}

func (suite *TradingPerformanceTestSuite) setupTestData() {
	// Create test users
	suite.testUsers = make([]string, 10000)
	for i := 0; i < 10000; i++ {
		suite.testUsers[i] = fmt.Sprintf("perf_user_%d", i)
	}

	// Create test trading pairs
	suite.testPairs = []string{
		"BTC/USDT", "ETH/USDT", "BNB/USDT", "ADA/USDT", "DOT/USDT",
		"LINK/USDT", "XRP/USDT", "LTC/USDT", "BCH/USDT", "UNI/USDT",
	}
}

func (suite *TradingPerformanceTestSuite) generateRandomOrder(userID string) *models.PlaceOrderRequest {
	pair := suite.testPairs[rand.Intn(len(suite.testPairs))]
	side := []models.OrderSide{models.Buy, models.Sell}[rand.Intn(2)]
	orderType := []models.OrderType{models.Market, models.Limit}[rand.Intn(2)]

	price := decimal.NewFromFloat(50000 + rand.Float64()*10000)  // Random price between 50k-60k
	quantity := decimal.NewFromFloat(0.001 + rand.Float64()*0.1) // Random quantity

	return &models.PlaceOrderRequest{
		UserID:   userID,
		Pair:     pair,
		Side:     side,
		Type:     orderType,
		Price:    price,
		Quantity: quantity,
	}
}

// TestHighVolumeOrderPlacement tests order placement under extreme load
func (suite *TradingPerformanceTestSuite) TestHighVolumeOrderPlacement() {
	log.Println("Starting high volume order placement test...")

	const (
		totalOrders  = 100000
		concurrency  = 1000
		targetTPS    = 100000
		testDuration = 10 * time.Second
	)

	// Performance metrics
	var (
		successfulOrders int64
		failedOrders     int64
		totalLatency     int64
		maxLatency       int64
		minLatency       int64 = int64(^uint64(0) >> 1) // Max int64
	)

	startTime := time.Now()

	// Create worker pool
	orderChan := make(chan *models.PlaceOrderRequest, concurrency*2)
	var wg sync.WaitGroup

	// Start workers
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for order := range orderChan {
				orderStart := time.Now()

				_, err := suite.service.PlaceOrder(suite.ctx, order)

				latency := time.Since(orderStart).Nanoseconds()
				atomic.AddInt64(&totalLatency, latency)

				// Update min/max latency
				for {
					current := atomic.LoadInt64(&maxLatency)
					if latency <= current || atomic.CompareAndSwapInt64(&maxLatency, current, latency) {
						break
					}
				}

				for {
					current := atomic.LoadInt64(&minLatency)
					if latency >= current || atomic.CompareAndSwapInt64(&minLatency, current, latency) {
						break
					}
				}

				if err != nil {
					atomic.AddInt64(&failedOrders, 1)
				} else {
					atomic.AddInt64(&successfulOrders, 1)
				}
			}
		}(i)
	}

	// Generate orders
	go func() {
		defer close(orderChan)

		for i := 0; i < totalOrders; i++ {
			userID := suite.testUsers[i%len(suite.testUsers)]
			order := suite.generateRandomOrder(userID)

			select {
			case orderChan <- order:
			case <-suite.ctx.Done():
				return
			}

			// Rate limiting to prevent overwhelming the system initially
			if i < 1000 {
				time.Sleep(time.Microsecond * 100)
			}
		}
	}()

	// Wait for completion
	wg.Wait()
	totalTime := time.Since(startTime)

	// Calculate metrics
	totalProcessed := atomic.LoadInt64(&successfulOrders) + atomic.LoadInt64(&failedOrders)
	actualTPS := float64(totalProcessed) / totalTime.Seconds()
	avgLatency := time.Duration(atomic.LoadInt64(&totalLatency) / totalProcessed)

	// Performance assertions
	suite.Assert().True(actualTPS > 10000, "TPS should be > 10,000, got %.2f", actualTPS)
	suite.Assert().True(avgLatency < 10*time.Millisecond, "Average latency should be < 10ms, got %v", avgLatency)
	suite.Assert().True(atomic.LoadInt64(&successfulOrders) > totalProcessed*8/10, "Success rate should be > 80%%")

	// Log detailed performance metrics
	log.Printf("=== HIGH VOLUME ORDER PLACEMENT RESULTS ===")
	log.Printf("Total Orders Processed: %d", totalProcessed)
	log.Printf("Successful Orders: %d (%.2f%%)", atomic.LoadInt64(&successfulOrders),
		float64(atomic.LoadInt64(&successfulOrders))/float64(totalProcessed)*100)
	log.Printf("Failed Orders: %d (%.2f%%)", atomic.LoadInt64(&failedOrders),
		float64(atomic.LoadInt64(&failedOrders))/float64(totalProcessed)*100)
	log.Printf("Actual TPS: %.2f", actualTPS)
	log.Printf("Total Duration: %v", totalTime)
	log.Printf("Average Latency: %v", avgLatency)
	log.Printf("Min Latency: %v", time.Duration(atomic.LoadInt64(&minLatency)))
	log.Printf("Max Latency: %v", time.Duration(atomic.LoadInt64(&maxLatency)))
	log.Printf("Bookkeeper Operations: %d", suite.mockBookkeeper.GetTotalOps())
	log.Printf("WebSocket Messages Sent: %d", suite.mockWSHub.GetMessagesSent())
	log.Printf("==========================================")
}

// TestSustainedHighThroughput tests sustained high throughput over longer duration
func (suite *TradingPerformanceTestSuite) TestSustainedHighThroughput() {
	log.Println("Starting sustained high throughput test...")

	const (
		testDuration = 60 * time.Second
		concurrency  = 500
		targetTPS    = 50000
	)

	var (
		totalOrders   int64
		successOrders int64
		errorOrders   int64
	)

	ctx, cancel := context.WithTimeout(suite.ctx, testDuration)
	defer cancel()

	startTime := time.Now()
	var wg sync.WaitGroup

	// Create sustained load workers
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			ticker := time.NewTicker(time.Second / time.Duration(targetTPS/concurrency))
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					userID := suite.testUsers[rand.Intn(len(suite.testUsers))]
					order := suite.generateRandomOrder(userID)

					atomic.AddInt64(&totalOrders, 1)

					_, err := suite.service.PlaceOrder(ctx, order)
					if err != nil {
						atomic.AddInt64(&errorOrders, 1)
					} else {
						atomic.AddInt64(&successOrders, 1)
					}
				}
			}
		}(i)
	}

	// Monitor performance every 10 seconds
	monitorTicker := time.NewTicker(10 * time.Second)
	go func() {
		defer monitorTicker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-monitorTicker.C:
				elapsed := time.Since(startTime)
				currentTotal := atomic.LoadInt64(&totalOrders)
				currentTPS := float64(currentTotal) / elapsed.Seconds()

				log.Printf("Sustained Test Progress - Elapsed: %v, Orders: %d, Current TPS: %.2f",
					elapsed, currentTotal, currentTPS)
			}
		}
	}()

	wg.Wait()
	totalTime := time.Since(startTime)

	// Calculate final metrics
	finalTotal := atomic.LoadInt64(&totalOrders)
	finalSuccess := atomic.LoadInt64(&successOrders)
	finalErrors := atomic.LoadInt64(&errorOrders)
	sustainedTPS := float64(finalTotal) / totalTime.Seconds()
	successRate := float64(finalSuccess) / float64(finalTotal) * 100

	// Assertions for sustained performance
	suite.Assert().True(sustainedTPS > 20000, "Sustained TPS should be > 20,000, got %.2f", sustainedTPS)
	suite.Assert().True(successRate > 90, "Success rate should be > 90%%, got %.2f%%", successRate)

	log.Printf("=== SUSTAINED HIGH THROUGHPUT RESULTS ===")
	log.Printf("Test Duration: %v", totalTime)
	log.Printf("Total Orders: %d", finalTotal)
	log.Printf("Successful Orders: %d (%.2f%%)", finalSuccess, successRate)
	log.Printf("Error Orders: %d (%.2f%%)", finalErrors, float64(finalErrors)/float64(finalTotal)*100)
	log.Printf("Sustained TPS: %.2f", sustainedTPS)
	log.Printf("=========================================")
}

// TestOrderBookPerformance tests order book operations under high load
func (suite *TradingPerformanceTestSuite) TestOrderBookPerformance() {
	log.Println("Starting order book performance test...")

	const (
		concurrency         = 200
		operationsPerWorker = 1000
	)

	var wg sync.WaitGroup
	startTime := time.Now()

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for j := 0; j < operationsPerWorker; j++ {
				pair := suite.testPairs[rand.Intn(len(suite.testPairs))]

				// Test different order book operations
				switch j % 3 {
				case 0:
					// Get order book
					_, err := suite.service.GetOrderBook(suite.ctx, pair, 100)
					suite.Assert().NoError(err)
				case 1:
					// Get order book binary
					_, err := suite.service.GetOrderBookBinary(suite.ctx, pair, 50)
					suite.Assert().NoError(err)
				case 2:
					// Place and immediately cancel an order
					userID := suite.testUsers[rand.Intn(100)] // Use smaller subset for cancellation
					order := suite.generateRandomOrder(userID)

					placedOrder, err := suite.service.PlaceOrder(suite.ctx, order)
					if err == nil && placedOrder != nil {
						suite.service.CancelOrder(suite.ctx, userID, placedOrder.ID)
					}
				}
			}
		}(i)
	}

	wg.Wait()
	duration := time.Since(startTime)

	totalOps := concurrency * operationsPerWorker
	opsPerSecond := float64(totalOps) / duration.Seconds()

	suite.Assert().True(opsPerSecond > 50000, "Order book ops/sec should be > 50,000, got %.2f", opsPerSecond)

	log.Printf("=== ORDER BOOK PERFORMANCE RESULTS ===")
	log.Printf("Total Operations: %d", totalOps)
	log.Printf("Duration: %v", duration)
	log.Printf("Operations/Second: %.2f", opsPerSecond)
	log.Printf("======================================")
}

// TestConcurrentOrderMatching tests order matching under high concurrency
func (suite *TradingPerformanceTestSuite) TestConcurrentOrderMatching() {
	log.Println("Starting concurrent order matching test...")

	const (
		buyersCount   = 500
		sellersCount  = 500
		ordersPerUser = 20
		pair          = "BTC/USDT"
	)

	var wg sync.WaitGroup
	startTime := time.Now()

	// Create buy orders
	wg.Add(buyersCount)
	for i := 0; i < buyersCount; i++ {
		go func(buyerID int) {
			defer wg.Done()

			userID := fmt.Sprintf("buyer_%d", buyerID)
			basePrice := decimal.NewFromInt(55000)

			for j := 0; j < ordersPerUser; j++ {
				order := &models.PlaceOrderRequest{
					UserID:   userID,
					Pair:     pair,
					Side:     models.Buy,
					Type:     models.Limit,
					Price:    basePrice.Add(decimal.NewFromInt(int64(rand.Intn(1000)))),
					Quantity: decimal.NewFromFloat(0.01 + rand.Float64()*0.09),
				}

				suite.service.PlaceOrder(suite.ctx, order)
			}
		}(i)
	}

	// Create sell orders with slight delay for matching
	time.Sleep(100 * time.Millisecond)

	wg.Add(sellersCount)
	for i := 0; i < sellersCount; i++ {
		go func(sellerID int) {
			defer wg.Done()

			userID := fmt.Sprintf("seller_%d", sellerID)
			basePrice := decimal.NewFromInt(55000)

			for j := 0; j < ordersPerUser; j++ {
				order := &models.PlaceOrderRequest{
					UserID:   userID,
					Pair:     pair,
					Side:     models.Sell,
					Type:     models.Limit,
					Price:    basePrice.Add(decimal.NewFromInt(int64(rand.Intn(1000)))),
					Quantity: decimal.NewFromFloat(0.01 + rand.Float64()*0.09),
				}

				suite.service.PlaceOrder(suite.ctx, order)
			}
		}(i)
	}

	wg.Wait()
	duration := time.Since(startTime)

	totalOrders := (buyersCount + sellersCount) * ordersPerUser
	ordersPerSecond := float64(totalOrders) / duration.Seconds()

	log.Printf("=== CONCURRENT ORDER MATCHING RESULTS ===")
	log.Printf("Total Orders: %d", totalOrders)
	log.Printf("Duration: %v", duration)
	log.Printf("Orders/Second: %.2f", ordersPerSecond)
	log.Printf("Buyers: %d, Sellers: %d", buyersCount, sellersCount)
	log.Printf("==========================================")
}

// BenchmarkOrderPlacementUltraHigh benchmarks order placement for ultra-high performance
func (suite *TradingPerformanceTestSuite) BenchmarkOrderPlacementUltraHigh() {
	b := &testing.B{}

	b.ResetTimer()
	b.SetParallelism(1000) // High parallelism

	b.RunParallel(func(pb *testing.PB) {
		userID := suite.testUsers[rand.Intn(len(suite.testUsers))]

		for pb.Next() {
			order := suite.generateRandomOrder(userID)
			suite.service.PlaceOrder(suite.ctx, order)
		}
	})

	log.Printf("BenchmarkOrderPlacementUltraHigh completed")
}

// BenchmarkOrderBookRetrieval benchmarks order book retrieval performance
func (suite *TradingPerformanceTestSuite) BenchmarkOrderBookRetrieval() {
	b := &testing.B{}

	b.ResetTimer()
	b.SetParallelism(100)

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			pair := suite.testPairs[rand.Intn(len(suite.testPairs))]
			suite.service.GetOrderBook(suite.ctx, pair, 100)
		}
	})

	log.Printf("BenchmarkOrderBookRetrieval completed")
}

func TestTradingPerformanceTestSuite(t *testing.T) {
	suite.Run(t, new(TradingPerformanceTestSuite))
}
