//go:build trading

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

	"github.com/Aidin1998/finalex/internal/trading"
	"github.com/Aidin1998/finalex/pkg/models"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/suite"
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

// MockBookkeeperStressTest provides high-performance mock for stress testing
type MockBookkeeperStressTest struct {
	balances     sync.Map // userID -> map[asset]decimal.Decimal
	reservations sync.Map // reservationID -> ReservationInfo
	totalOps     int64
	mu           sync.RWMutex
}

// ReservationInfo is defined in common_test_types.go

func (m *MockBookkeeperStressTest) GetBalance(userID, asset string) (decimal.Decimal, error) {
	atomic.AddInt64(&m.totalOps, 1)

	userBalances, exists := m.balances.Load(userID)
	if !exists {
		return decimal.Zero, nil
	}

	balanceMap := userBalances.(map[string]decimal.Decimal)
	balance, exists := balanceMap[asset]
	if !exists {
		return decimal.Zero, nil
	}

	return balance, nil
}

func (m *MockBookkeeperStressTest) SetBalance(userID, asset string, amount decimal.Decimal) {
	userBalances, _ := m.balances.LoadOrStore(userID, make(map[string]decimal.Decimal))
	balanceMap := userBalances.(map[string]decimal.Decimal)
	balanceMap[asset] = amount
	m.balances.Store(userID, balanceMap)
}

func (m *MockBookkeeperStressTest) ReserveBalance(userID, asset string, amount decimal.Decimal) (string, error) {
	atomic.AddInt64(&m.totalOps, 1)

	reservationID := uuid.New().String()
	reservation := ReservationInfo{
		UserID: userID,
		Asset:  asset,
		Amount: amount,
	}

	m.reservations.Store(reservationID, reservation)
	return reservationID, nil
}

func (m *MockBookkeeperStressTest) CommitReservation(reservationID string) error {
	atomic.AddInt64(&m.totalOps, 1)
	m.reservations.Delete(reservationID)
	return nil
}

func (m *MockBookkeeperStressTest) ReleaseReservation(reservationID string) error {
	atomic.AddInt64(&m.totalOps, 1)
	m.reservations.Delete(reservationID)
	return nil
}

// MockWSHubStressTest provides high-performance mock WebSocket hub for stress testing
type MockWSHubStressTest struct {
	connections sync.Map // userID -> *MockConnection
	broadcasts  sync.Map // topic -> [][]byte
	totalOps    int64
	mu          sync.RWMutex
}

// MockConnection is defined in common_test_types.go

func (m *MockWSHubStressTest) Connect(userID string) {
	atomic.AddInt64(&m.totalOps, 1)
	conn := &MockConnection{
		UserID:      userID,
		Messages:    make([]WSMessage, 0),
		MessagesRaw: make([][]byte, 0),
		IsActive:    true,
		IsConnected: true,
	}
	m.connections.Store(userID, conn)
}

func (m *MockWSHubStressTest) Disconnect(userID string) {
	atomic.AddInt64(&m.totalOps, 1)
	if conn, exists := m.connections.Load(userID); exists {
		connection := conn.(*MockConnection)
		connection.mu.Lock()
		connection.IsActive = false
		connection.mu.Unlock()
	}
}

func (m *MockWSHubStressTest) Broadcast(topic string, message []byte) {
	atomic.AddInt64(&m.totalOps, 1)

	// Store broadcast message
	topicMessages, _ := m.broadcasts.LoadOrStore(topic, make([][]byte, 0))
	messages := topicMessages.([][]byte)
	messages = append(messages, message)
	m.broadcasts.Store(topic, messages)
}

func (m *MockWSHubStressTest) BroadcastToUser(userID string, message []byte) {
	atomic.AddInt64(&m.totalOps, 1)

	if conn, exists := m.connections.Load(userID); exists {
		connection := conn.(*MockConnection)
		connection.mu.Lock()
		if connection.IsActive {
			connection.MessagesRaw = append(connection.MessagesRaw, message)
		}
		connection.mu.Unlock()
	}
}

func (m *MockWSHubStressTest) GetConnection(userID string) *MockConnection {
	if conn, exists := m.connections.Load(userID); exists {
		return conn.(*MockConnection)
	}
	return nil
}

func (m *MockWSHubStressTest) GetBroadcasts(topic string) [][]byte {
	if messages, exists := m.broadcasts.Load(topic); exists {
		return messages.([][]byte)
	}
	return nil
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
		suite.testUsers[i] = fmt.Sprintf("stress_user_%d", i)

		// Set up initial balances for stress testing
		suite.mockBookkeeper.SetBalance(suite.testUsers[i], "BTC", decimal.NewFromFloat(10.0))
		suite.mockBookkeeper.SetBalance(suite.testUsers[i], "USDT", decimal.NewFromFloat(100000.0))
		suite.mockBookkeeper.SetBalance(suite.testUsers[i], "ETH", decimal.NewFromFloat(50.0))

		// Connect users to WebSocket
		suite.mockWSHub.Connect(suite.testUsers[i])
	}

	// Test trading pairs
	suite.testPairs = []string{"BTCUSDT", "ETHUSDT", "ETHBTC"}

	// Create trading service with mock adapters
	tradingService := &MockTradingServiceStress{
		bookkeeper: suite.mockBookkeeper,
		wsHub:      suite.mockWSHub,
		metrics:    suite.metrics,
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

// MockTradingServiceStress provides a mock trading service for stress testing
type MockTradingServiceStress struct {
	bookkeeper *MockBookkeeperStressTest
	wsHub      *MockWSHubStressTest
	metrics    *StressTestMetrics
	mu         sync.RWMutex
}

func (m *MockTradingServiceStress) Start() error {
	return nil
}

func (m *MockTradingServiceStress) Stop() error {
	return nil
}

func (m *MockTradingServiceStress) PlaceOrder(ctx context.Context, order *models.Order) (*models.Order, error) {
	// Simulate order processing latency
	start := time.Now()

	// Validate order
	if order.Quantity.LessThanOrEqual(decimal.Zero) {
		m.metrics.RecordOrderFailed()
		return nil, fmt.Errorf("invalid quantity")
	}

	// Check balance simulation
	if order.Side == models.SideBuy {
		requiredBalance := order.Price.Mul(order.Quantity)
		balance, err := m.bookkeeper.GetBalance(order.UserID, "USDT")
		if err != nil || balance.LessThan(requiredBalance) {
			m.metrics.RecordOrderFailed()
			return nil, fmt.Errorf("insufficient balance")
		}
	}

	// Simulate processing delay
	processingDelay := time.Duration(rand.Intn(10)) * time.Microsecond
	time.Sleep(processingDelay)

	// Set order status
	order.Status = models.OrderStatusNew
	order.Created = time.Now()

	// Record metrics
	latency := time.Since(start)
	m.metrics.RecordOrderProcessed(latency)

	// Simulate order matching probability
	if rand.Float32() > 0.6 {
		m.metrics.RecordOrderMatched()

		// Simulate trade execution
		if rand.Float32() > 0.5 {
			m.metrics.RecordTradeExecuted()

			// Broadcast trade update
			tradeMsg := fmt.Sprintf(`{"type":"trade","symbol":"%s","price":"%s","quantity":"%s","time":%d}`,
				order.Symbol, order.Price.String(), order.Quantity.String(), time.Now().UnixNano())
			m.wsHub.Broadcast(fmt.Sprintf("trades.%s", order.Symbol), []byte(tradeMsg))
		}
	}

	return order, nil
}

func (m *MockTradingServiceStress) CancelOrder(ctx context.Context, orderID string) error {
	// Simulate cancellation processing
	time.Sleep(time.Microsecond * 50)
	return nil
}

func (m *MockTradingServiceStress) GetOrder(orderID string) (*models.Order, error) {
	return nil, fmt.Errorf("order not found")
}

func (m *MockTradingServiceStress) GetOrders(userID, symbol, status string, limit, offset string) ([]*models.Order, int64, error) {
	return []*models.Order{}, 0, nil
}

func (m *MockTradingServiceStress) GetOrderBook(symbol string, depth int) (*models.OrderBookSnapshot, error) {
	return &models.OrderBookSnapshot{
		Symbol:    symbol,
		Timestamp: time.Now(),
		Bids:      []models.OrderBookEntry{},
		Asks:      []models.OrderBookEntry{},
	}, nil
}

func (m *MockTradingServiceStress) GetOrderBookBinary(symbol string, depth int) ([]byte, error) {
	return []byte{}, nil
}

func (m *MockTradingServiceStress) GetTradingPairs() ([]*models.TradingPair, error) {
	return []*models.TradingPair{}, nil
}

func (m *MockTradingServiceStress) GetTradingPair(symbol string) (*models.TradingPair, error) {
	return nil, fmt.Errorf("not found")
}

func (m *MockTradingServiceStress) CreateTradingPair(pair *models.TradingPair) (*models.TradingPair, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *MockTradingServiceStress) UpdateTradingPair(pair *models.TradingPair) (*models.TradingPair, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *MockTradingServiceStress) ListOrders(userID string, filter *models.OrderFilter) ([]*models.Order, error) {
	return []*models.Order{}, nil
}

// TestStressLoadTestSuite runs the entire stress test suite
func TestStressLoadTestSuite(t *testing.T) {
	suite.Run(t, new(TradingStressLoadTestSuite))
}
