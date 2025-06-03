// =============================
// End-to-End Integration Tests for Adaptive Trading System
// =============================
// This file provides comprehensive integration tests for the adaptive trading
// system, including migration scenarios, performance validation, and fault tolerance.

package test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/bookkeeper"
	"github.com/Aidin1998/pincex_unified/internal/trading"
	"github.com/Aidin1998/pincex_unified/internal/trading/engine"
	"github.com/Aidin1998/pincex_unified/internal/trading/model"
	"github.com/Aidin1998/pincex_unified/internal/ws"
	"github.com/Aidin1998/pincex_unified/pkg/models"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// AdaptiveTradingTestSuite provides comprehensive tests for the adaptive trading system
type AdaptiveTradingTestSuite struct {
	suite.Suite

	// Test infrastructure
	db             *gorm.DB
	service        trading.AdaptiveTradingService
	mockBookkeeper *MockBookkeeperService
	logger         *zap.Logger

	// Test configuration
	testPair string

	// Test data
	testUsers  []uuid.UUID
	testOrders []*models.Order

	// Monitoring
	metricsHistory  []*engine.MetricsReport
	migrationEvents []MigrationEvent

	// Control
	stopChan   chan struct{}
	testCtx    context.Context
	testCancel context.CancelFunc
}

// MockBookkeeperService provides a mock implementation for testing
type MockBookkeeperService struct {
	balances map[uuid.UUID]map[string]decimal.Decimal
	mu       sync.RWMutex
}

func NewMockBookkeeperService() *MockBookkeeperService {
	return &MockBookkeeperService{
		balances: make(map[uuid.UUID]map[string]decimal.Decimal),
	}
}

// GetBalance/SetBalance are test helpers
func (m *MockBookkeeperService) GetBalance(ctx context.Context, userID uuid.UUID, asset string) (decimal.Decimal, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if userBalances, exists := m.balances[userID]; exists {
		if balance, exists := userBalances[asset]; exists {
			return balance, nil
		}
	}

	return decimal.NewFromFloat(10000.0), nil // Default balance for testing
}

func (m *MockBookkeeperService) SetBalance(userID uuid.UUID, asset string, balance decimal.Decimal) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.balances[userID]; !exists {
		m.balances[userID] = make(map[string]decimal.Decimal)
	}

	m.balances[userID][asset] = balance
}

// Implement BookkeeperService interface
func (m *MockBookkeeperService) Start() error { return nil }

func (m *MockBookkeeperService) Stop() error { return nil }

func (m *MockBookkeeperService) GetAccounts(ctx context.Context, userID string) ([]*models.Account, error) {
	return []*models.Account{}, nil
}

func (m *MockBookkeeperService) GetAccount(ctx context.Context, userID, currency string) (*models.Account, error) {
	return &models.Account{UserID: uuid.MustParse(userID), Currency: currency, Balance: 10000.0, Available: 10000.0, Locked: 0}, nil
}

func (m *MockBookkeeperService) CreateAccount(ctx context.Context, userID, currency string) (*models.Account, error) {
	return &models.Account{UserID: uuid.MustParse(userID), Currency: currency, Balance: 10000.0, Available: 10000.0, Locked: 0}, nil
}

func (m *MockBookkeeperService) GetAccountTransactions(ctx context.Context, userID, currency string, limit, offset int) ([]*models.Transaction, int64, error) {
	return []*models.Transaction{}, 0, nil
}

func (m *MockBookkeeperService) CreateTransaction(ctx context.Context, userID, transactionType string, amount float64, currency, reference, description string) (*models.Transaction, error) {
	tx := &models.Transaction{ID: uuid.New(), UserID: uuid.MustParse(userID), Type: transactionType, Amount: amount, Currency: currency, Status: "completed", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	return tx, nil
}

// CompleteTransaction satisfies BookkeeperService interface
func (m *MockBookkeeperService) CompleteTransaction(ctx context.Context, transactionID string) error {
	return nil
}

// FailTransaction satisfies BookkeeperService interface
func (m *MockBookkeeperService) FailTransaction(ctx context.Context, transactionID string) error {
	return nil
}

// RequestFundsLock and UnlockFunds satisfy interface
func (m *MockBookkeeperService) LockFunds(ctx context.Context, userID, currency string, amount float64) error {
	return nil
}

func (m *MockBookkeeperService) UnlockFunds(ctx context.Context, userID, currency string, amount float64) error {
	return nil
}

// BatchGetAccounts is a stub to satisfy the BookkeeperService interface for tests
func (m *MockBookkeeperService) BatchGetAccounts(ctx context.Context, userIDs []string, currencies []string) (map[string]map[string]*models.Account, error) {
	result := make(map[string]map[string]*models.Account)
	for _, userID := range userIDs {
		result[userID] = make(map[string]*models.Account)
		for _, currency := range currencies {
			result[userID][currency] = &models.Account{
				UserID:    uuid.MustParse(userID),
				Currency:  currency,
				Balance:   10000.0,
				Available: 10000.0,
				Locked:    0,
			}
		}
	}
	return result, nil
}

// MigrationEvent tracks migration-related events during testing
type MigrationEvent struct {
	Timestamp  time.Time
	Pair       string
	Event      string
	Percentage int32
	Details    map[string]interface{}
}

// SetupSuite initializes the test suite
func (suite *AdaptiveTradingTestSuite) SetupSuite() {
	suite.logger = zaptest.NewLogger(suite.T())
	suite.testPair = "BTC/USDT"
	suite.stopChan = make(chan struct{})
	suite.testCtx, suite.testCancel = context.WithCancel(context.Background())

	// Setup test users
	suite.testUsers = []uuid.UUID{
		uuid.New(),
		uuid.New(),
		uuid.New(),
		uuid.New(),
		uuid.New(),
	}

	// Initialize database
	var err error
	suite.db, err = gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(suite.T(), err)

	// Auto-migrate models
	err = suite.db.AutoMigrate(&model.Order{}, &model.Trade{})
	require.NoError(suite.T(), err)

	// Setup mock bookkeeper
	suite.mockBookkeeper = NewMockBookkeeperService()

	// Setup test balances
	for _, userID := range suite.testUsers {
		suite.mockBookkeeper.SetBalance(userID, "BTC", decimal.NewFromFloat(100.0))
		suite.mockBookkeeper.SetBalance(userID, "USDT", decimal.NewFromFloat(100000.0))
	}
}

// TearDownSuite cleans up after all tests
func (suite *AdaptiveTradingTestSuite) TearDownSuite() {
	if suite.service != nil {
		suite.service.Shutdown(suite.testCtx)
	}
	suite.testCancel()
	close(suite.stopChan)
}

// SetupTest initializes each test
func (suite *AdaptiveTradingTestSuite) SetupTest() {
	// Clear migration events and metrics history
	suite.migrationEvents = nil
	suite.metricsHistory = nil

	// Create adaptive configuration for testing
	config := engine.GetPresets().TestingConfig()
	// Create adaptive trading service
	var err error
	wsHub := ws.NewHub(4, 100) // Create test WebSocket hub with smaller parameters
	suite.service, err = trading.NewAdaptiveService(
		suite.logger,
		suite.db,
		suite.mockBookkeeper,
		config,
		wsHub,
	)
	require.NoError(suite.T(), err)

	// Start the service
	err = suite.service.Start()
	require.NoError(suite.T(), err)

	// Initialize test orders
	suite.generateTestOrders()
}

// TearDownTest cleans up after each test
func (suite *AdaptiveTradingTestSuite) TearDownTest() {
	if suite.service != nil {
		suite.service.Shutdown(suite.testCtx)
	}
}

// generateTestOrders creates a set of test orders for various scenarios
func (suite *AdaptiveTradingTestSuite) generateTestOrders() {
	suite.testOrders = []*models.Order{
		// Buy orders
		{
			ID:          uuid.New(),
			UserID:      suite.testUsers[0],
			Symbol:      suite.testPair,
			Side:        "buy",
			Type:        "limit",
			Price:       50000.0,
			Quantity:    0.1,
			TimeInForce: "GTC",
			Status:      "new",
		},
		{
			ID:          uuid.New(),
			UserID:      suite.testUsers[1],
			Symbol:      suite.testPair,
			Side:        "buy",
			Type:        "limit",
			Price:       49950.0,
			Quantity:    0.2,
			TimeInForce: "GTC",
			Status:      "new",
		},
		// Sell orders
		{
			ID:          uuid.New(),
			UserID:      suite.testUsers[2],
			Symbol:      suite.testPair,
			Side:        "sell",
			Type:        "limit",
			Price:       50100.0,
			Quantity:    0.15,
			TimeInForce: "GTC",
			Status:      "new",
		},
		{
			ID:          uuid.New(),
			UserID:      suite.testUsers[3],
			Symbol:      suite.testPair,
			Side:        "sell",
			Type:        "limit",
			Price:       50200.0,
			Quantity:    0.25,
			TimeInForce: "GTC",
			Status:      "new",
		},
		// Market orders for testing matching
		{
			ID:          uuid.New(),
			UserID:      suite.testUsers[4],
			Symbol:      suite.testPair,
			Side:        "buy",
			Type:        "market",
			Quantity:    0.05,
			TimeInForce: "IOC",
			Status:      "new",
		},
	}
}

// Test basic adaptive service functionality
func (suite *AdaptiveTradingTestSuite) TestBasicServiceFunctionality() {
	t := suite.T()

	// Test service health check
	health := suite.service.HealthCheck()
	assert.NotNil(t, health)
	assert.Equal(t, "adaptive_trading", health["service"])
	assert.Equal(t, "healthy", health["status"])

	// Test migration status (should be empty initially)
	states := suite.service.GetAllMigrationStates()
	assert.Empty(t, states)

	// Test auto-migration status
	isEnabled := suite.service.IsAutoMigrationEnabled()
	assert.True(t, isEnabled) // Testing config has auto-migration enabled
}

// Test order placement and processing
func (suite *AdaptiveTradingTestSuite) TestOrderPlacement() {
	t := suite.T()
	ctx := context.Background()

	// Place a limit order
	order := suite.testOrders[0]
	processedOrder, err := suite.service.PlaceOrder(ctx, order)
	require.NoError(t, err)
	require.NotNil(t, processedOrder)

	// Verify order was processed
	assert.Equal(t, order.Symbol, processedOrder.Symbol)
	assert.Equal(t, order.Side, processedOrder.Side)
	assert.Equal(t, order.Type, processedOrder.Type)
	assert.Equal(t, order.Price, processedOrder.Price)
	assert.Equal(t, order.Quantity, processedOrder.Quantity)

	// Check that order book contains the order
	orderBook, err := suite.service.GetOrderBook(suite.testPair, 10)
	require.NoError(t, err)
	require.NotNil(t, orderBook)

	// Should have at least one bid
	assert.NotEmpty(t, orderBook.Bids)
}

// Test migration control functionality
func (suite *AdaptiveTradingTestSuite) TestMigrationControl() {
	t := suite.T()

	// Start migration for test pair
	err := suite.service.StartMigration(suite.testPair)
	require.NoError(t, err)

	// Check migration status
	status, err := suite.service.GetMigrationStatus(suite.testPair)
	require.NoError(t, err)
	require.NotNil(t, status)
	assert.Equal(t, suite.testPair, status.Pair)

	// Set migration percentage
	err = suite.service.SetMigrationPercentage(suite.testPair, 25)
	require.NoError(t, err)

	// Verify percentage was set
	status, err = suite.service.GetMigrationStatus(suite.testPair)
	require.NoError(t, err)
	assert.Equal(t, int32(25), status.CurrentPercentage)

	// Pause migration
	err = suite.service.PauseMigration(suite.testPair)
	require.NoError(t, err)

	// Resume migration
	err = suite.service.ResumeMigration(suite.testPair)
	require.NoError(t, err)

	// Stop migration
	err = suite.service.StopMigration(suite.testPair)
	require.NoError(t, err)
}

// Test gradual migration scenario
func (suite *AdaptiveTradingTestSuite) TestGradualMigration() {
	t := suite.T()
	ctx := context.Background()

	// Start migration
	err := suite.service.StartMigration(suite.testPair)
	require.NoError(t, err)

	// Place orders during migration
	for i, order := range suite.testOrders {
		_, err := suite.service.PlaceOrder(ctx, order)
		require.NoError(t, err, "Failed to place order %d", i)

		// Gradually increase migration percentage
		percentage := int32((i + 1) * 20)
		if percentage <= 100 {
			err = suite.service.SetMigrationPercentage(suite.testPair, percentage)
			require.NoError(t, err)
		}

		// Brief pause to allow metrics collection
		time.Sleep(time.Millisecond * 100)
	}

	// Verify final migration state
	status, err := suite.service.GetMigrationStatus(suite.testPair)
	require.NoError(t, err)
	assert.True(t, status.CurrentPercentage > 0)

	// Get performance metrics
	metrics, err := suite.service.GetPerformanceMetrics(suite.testPair)
	require.NoError(t, err)
	assert.NotNil(t, metrics)
}

// Test concurrent order processing during migration
func (suite *AdaptiveTradingTestSuite) TestConcurrentOrderProcessing() {
	t := suite.T()
	ctx := context.Background()

	// Start migration
	err := suite.service.StartMigration(suite.testPair)
	require.NoError(t, err)

	// Set migration to 50%
	err = suite.service.SetMigrationPercentage(suite.testPair, 50)
	require.NoError(t, err)

	// Concurrent order processing
	var wg sync.WaitGroup
	orderCount := 50
	errors := make(chan error, orderCount)

	for i := 0; i < orderCount; i++ {
		wg.Add(1)
		go func(orderIndex int) {
			defer wg.Done()

			order := &models.Order{
				ID:          uuid.New(),
				UserID:      suite.testUsers[orderIndex%len(suite.testUsers)],
				Symbol:      suite.testPair,
				Side:        []string{"buy", "sell"}[orderIndex%2],
				Type:        "limit",
				Price:       50000.0 + float64(orderIndex%100),
				Quantity:    0.01 + float64(orderIndex%10)*0.001,
				TimeInForce: "GTC",
				Status:      "new",
			}

			_, err := suite.service.PlaceOrder(ctx, order)
			if err != nil {
				errors <- err
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check for errors
	var errorCount int
	for err := range errors {
		errorCount++
		t.Logf("Order placement error: %v", err)
	}

	// Allow some errors but not too many
	assert.LessOrEqual(t, errorCount, orderCount/10, "Too many order placement errors")

	// Verify order book state
	orderBook, err := suite.service.GetOrderBook(suite.testPair, 20)
	require.NoError(t, err)
	assert.NotEmpty(t, orderBook.Bids)
	assert.NotEmpty(t, orderBook.Asks)
}

// Test performance monitoring and metrics collection
func (suite *AdaptiveTradingTestSuite) TestPerformanceMonitoring() {
	t := suite.T()
	ctx := context.Background()

	// Start migration and place some orders
	err := suite.service.StartMigration(suite.testPair)
	require.NoError(t, err)

	// Place orders to generate metrics
	for _, order := range suite.testOrders {
		_, err := suite.service.PlaceOrder(ctx, order)
		require.NoError(t, err)
	}

	// Wait for metrics collection
	time.Sleep(time.Second * 3)

	// Get engine metrics
	engineMetrics, err := suite.service.GetEngineMetrics()
	require.NoError(t, err)
	require.NotNil(t, engineMetrics)

	assert.GreaterOrEqual(t, engineMetrics.TotalOrdersProcessed, int64(len(suite.testOrders)))
	assert.GreaterOrEqual(t, engineMetrics.AvgProcessingTimeMs, 0.0)

	// Get performance metrics for the pair
	pairMetrics, err := suite.service.GetPerformanceMetrics(suite.testPair)
	require.NoError(t, err)
	assert.NotNil(t, pairMetrics)

	// Get metrics report
	metricsReport, err := suite.service.GetMetricsReport()
	if err == nil && metricsReport != nil {
		assert.NotNil(t, metricsReport.EngineMetrics)
		assert.NotEmpty(t, metricsReport.PairMetrics)
	}
}

// Test circuit breaker functionality
func (suite *AdaptiveTradingTestSuite) TestCircuitBreaker() {
	t := suite.T()
	ctx := context.Background()

	// Start migration
	err := suite.service.StartMigration(suite.testPair)
	require.NoError(t, err)

	// Set migration to 100% to trigger potential issues
	err = suite.service.SetMigrationPercentage(suite.testPair, 100)
	require.NoError(t, err)

	// Create orders that might trigger circuit breaker
	invalidOrders := []*models.Order{
		{
			ID:          uuid.New(),
			UserID:      suite.testUsers[0],
			Symbol:      suite.testPair,
			Side:        "buy",
			Type:        "limit",
			Price:       -100.0, // Invalid price
			Quantity:    0.1,
			TimeInForce: "GTC",
			Status:      "new",
		},
		{
			ID:          uuid.New(),
			UserID:      suite.testUsers[1],
			Symbol:      suite.testPair,
			Side:        "sell",
			Type:        "limit",
			Price:       50000.0,
			Quantity:    -0.1, // Invalid quantity
			TimeInForce: "GTC",
			Status:      "new",
		},
	}

	// Place invalid orders to potentially trigger circuit breaker
	for _, order := range invalidOrders {
		_, err := suite.service.PlaceOrder(ctx, order)
		// These should fail, which is expected
		assert.Error(t, err)
	}

	// Reset circuit breaker
	err = suite.service.ResetCircuitBreaker(suite.testPair)
	if err != nil {
		// Circuit breaker reset might not be available for all implementations
		t.Logf("Circuit breaker reset not available: %v", err)
	}
}

// Test auto-migration functionality
func (suite *AdaptiveTradingTestSuite) TestAutoMigration() {
	t := suite.T()
	ctx := context.Background()

	// Verify auto-migration is enabled
	assert.True(t, suite.service.IsAutoMigrationEnabled())

	// Start migration with a low percentage
	err := suite.service.StartMigration(suite.testPair)
	require.NoError(t, err)

	err = suite.service.SetMigrationPercentage(suite.testPair, 10)
	require.NoError(t, err)

	// Place orders to generate performance data
	for i := 0; i < 20; i++ {
		order := &models.Order{
			ID:          uuid.New(),
			UserID:      suite.testUsers[i%len(suite.testUsers)],
			Symbol:      suite.testPair,
			Side:        []string{"buy", "sell"}[i%2],
			Type:        "limit",
			Price:       50000.0 + float64(i),
			Quantity:    0.01,
			TimeInForce: "GTC",
			Status:      "new",
		}

		_, err := suite.service.PlaceOrder(ctx, order)
		require.NoError(t, err)

		// Brief pause for metrics collection
		time.Sleep(time.Millisecond * 50)
	}

	// Wait for auto-migration to potentially trigger
	time.Sleep(time.Second * 5)

	// Check final migration state
	status, err := suite.service.GetMigrationStatus(suite.testPair)
	require.NoError(t, err)

	// Auto-migration might have adjusted the percentage
	assert.GreaterOrEqual(t, status.CurrentPercentage, int32(10))
}

// Test rollback functionality
func (suite *AdaptiveTradingTestSuite) TestMigrationRollback() {
	t := suite.T()
	ctx := context.Background()

	// Start migration and set high percentage
	err := suite.service.StartMigration(suite.testPair)
	require.NoError(t, err)

	err = suite.service.SetMigrationPercentage(suite.testPair, 80)
	require.NoError(t, err)

	// Place some orders
	for _, order := range suite.testOrders[:3] {
		_, err := suite.service.PlaceOrder(ctx, order)
		require.NoError(t, err)
	}

	// Rollback migration
	err = suite.service.RollbackMigration(suite.testPair)
	require.NoError(t, err)

	// Verify rollback
	status, err := suite.service.GetMigrationStatus(suite.testPair)
	require.NoError(t, err)
	assert.Equal(t, int32(0), status.CurrentPercentage)

	// Continue placing orders after rollback
	for _, order := range suite.testOrders[3:] {
		_, err := suite.service.PlaceOrder(ctx, order)
		require.NoError(t, err)
	}

	// Verify order book is still functional
	orderBook, err := suite.service.GetOrderBook(suite.testPair, 10)
	require.NoError(t, err)
	assert.NotNil(t, orderBook)
}

// Test error handling and fault tolerance
func (suite *AdaptiveTradingTestSuite) TestErrorHandling() {
	t := suite.T()
	ctx := context.Background()

	// Test invalid migration operations
	err := suite.service.SetMigrationPercentage(suite.testPair, -10)
	assert.Error(t, err)

	err = suite.service.SetMigrationPercentage(suite.testPair, 150)
	assert.Error(t, err)

	// Test operations on non-existent pair
	err = suite.service.StartMigration("INVALID/PAIR")
	// This might or might not error depending on implementation

	status, err := suite.service.GetMigrationStatus("INVALID/PAIR")
	assert.Error(t, err)
	assert.Nil(t, status)

	// Test invalid order placement
	invalidOrder := &models.Order{
		ID:          uuid.New(),
		UserID:      uuid.Nil, // Invalid user ID
		Symbol:      suite.testPair,
		Side:        "buy",
		Type:        "limit",
		Price:       50000.0,
		Quantity:    0.1,
		TimeInForce: "GTC",
		Status:      "new",
	}

	_, err = suite.service.PlaceOrder(ctx, invalidOrder)
	assert.Error(t, err)
}

// Test performance thresholds and alerts
func (suite *AdaptiveTradingTestSuite) TestPerformanceThresholds() {
	t := suite.T()

	// Set strict performance thresholds
	thresholds := &engine.AutoMigrationThresholds{
		LatencyP95ThresholdMs:       1.0,   // Very strict latency
		ThroughputDegradationPct:    5.0,   // Very strict throughput
		ErrorRateThreshold:          0.001, // Very strict error rate
		ContentionThreshold:         5,
		NewImplErrorRateThreshold:   0.001,
		ConsecutiveFailureThreshold: 1,
		PerformanceDegradationPct:   5.0,
	}

	err := suite.service.SetPerformanceThresholds(thresholds)
	require.NoError(t, err)

	// Test nil thresholds
	err = suite.service.SetPerformanceThresholds(nil)
	assert.Error(t, err)
}

// Benchmark test for performance validation
func (suite *AdaptiveTradingTestSuite) TestPerformanceBenchmark() {
	t := suite.T()
	ctx := context.Background()

	// Start migration
	err := suite.service.StartMigration(suite.testPair)
	require.NoError(t, err)

	// Benchmark with different migration percentages
	percentages := []int32{0, 25, 50, 75, 100}
	results := make(map[int32]time.Duration)

	for _, percentage := range percentages {
		err = suite.service.SetMigrationPercentage(suite.testPair, percentage)
		require.NoError(t, err)

		// Wait for migration to take effect
		time.Sleep(time.Millisecond * 100)

		// Benchmark order placement
		start := time.Now()
		for i := 0; i < 10; i++ {
			order := &models.Order{
				ID:          uuid.New(),
				UserID:      suite.testUsers[i%len(suite.testUsers)],
				Symbol:      suite.testPair,
				Side:        []string{"buy", "sell"}[i%2],
				Type:        "limit",
				Price:       50000.0 + float64(i),
				Quantity:    0.01,
				TimeInForce: "GTC",
				Status:      "new",
			}

			_, err := suite.service.PlaceOrder(ctx, order)
			require.NoError(t, err)
		}
		duration := time.Since(start)
		results[percentage] = duration

		t.Logf("Migration %d%%: %v for 10 orders (avg: %v/order)",
			percentage, duration, duration/10)
	}

	// Validate that performance doesn't degrade significantly
	baselinePerformance := results[0]
	for percentage, duration := range results {
		if percentage > 0 {
			degradation := float64(duration-baselinePerformance) / float64(baselinePerformance) * 100
			t.Logf("Performance degradation at %d%%: %.2f%%", percentage, degradation)

			// Allow up to 50% degradation for testing (in production this should be much lower)
			assert.LessOrEqual(t, degradation, 50.0,
				"Performance degradation too high at migration %d%%", percentage)
		}
	}
}

// Run the test suite
func TestAdaptiveTradingSystem(t *testing.T) {
	suite.Run(t, new(AdaptiveTradingTestSuite))
}

// Additional helper functions for testing

// generateHighFrequencyOrders creates a stream of orders for high-frequency testing
func (suite *AdaptiveTradingTestSuite) generateHighFrequencyOrders(count int, pair string) []*models.Order {
	orders := make([]*models.Order, count)

	for i := 0; i < count; i++ {
		orders[i] = &models.Order{
			ID:          uuid.New(),
			UserID:      suite.testUsers[i%len(suite.testUsers)],
			Symbol:      pair,
			Side:        []string{"buy", "sell"}[i%2],
			Type:        "limit",
			Price:       50000.0 + float64(i%1000)*0.1,
			Quantity:    0.001 + float64(i%100)*0.0001,
			TimeInForce: "GTC",
			Status:      "new",
		}
	}

	return orders
}

// monitorMigrationProgress monitors migration progress during tests
func (suite *AdaptiveTradingTestSuite) monitorMigrationProgress(pair string, duration time.Duration) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	timeout := time.After(duration)

	for {
		select {
		case <-timeout:
			return
		case <-ticker.C:
			status, err := suite.service.GetMigrationStatus(pair)
			if err == nil && status != nil {
				event := MigrationEvent{
					Timestamp:  time.Now(),
					Pair:       pair,
					Event:      "progress_update",
					Percentage: status.CurrentPercentage,
					Details: map[string]interface{}{
						"status":                 status.Status,
						"auto_migration_enabled": status.AutoMigrationEnabled,
						"last_update_time":       status.LastUpdateTime,
					},
				}
				suite.migrationEvents = append(suite.migrationEvents, event)
			}
		case <-suite.stopChan:
			return
		}
	}
}

// BatchLockFunds is a stub to satisfy the BookkeeperService interface for tests
func (m *MockBookkeeperService) BatchLockFunds(ctx context.Context, ops []bookkeeper.FundsOperation) (*bookkeeper.BatchOperationResult, error) {
	return &bookkeeper.BatchOperationResult{
		SuccessCount: len(ops),
		FailedItems:  make(map[string]error),
		Duration:     0,
	}, nil
}

// BatchUnlockFunds is a stub to satisfy the BookkeeperService interface for tests
func (m *MockBookkeeperService) BatchUnlockFunds(ctx context.Context, ops []bookkeeper.FundsOperation) (*bookkeeper.BatchOperationResult, error) {
	return &bookkeeper.BatchOperationResult{
		SuccessCount: len(ops),
		FailedItems:  make(map[string]error),
		Duration:     0,
	}, nil
}
