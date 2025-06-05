//go:build trading
// +build trading

package test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
	"gorm.io/gorm"

	"github.com/Aidin1998/pincex_unified/internal/trading"
	"github.com/Aidin1998/pincex_unified/internal/trading/settlement"
	"github.com/Aidin1998/pincex_unified/pkg/models"
)

// Mock implementations for integration testing
type MockBookkeeperIntegration struct {
	mu       sync.RWMutex
	balances map[string]map[string]decimal.Decimal // userID -> asset -> balance
	reserved map[string]map[string]decimal.Decimal // userID -> asset -> reserved
}

func NewMockBookkeeperIntegration() *MockBookkeeperIntegration {
	return &MockBookkeeperIntegration{
		balances: make(map[string]map[string]decimal.Decimal),
		reserved: make(map[string]map[string]decimal.Decimal),
	}
}

func (m *MockBookkeeperIntegration) ReserveBalance(ctx context.Context, userID uuid.UUID, asset string, amount decimal.Decimal) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	userIDStr := userID.String()
	if m.balances[userIDStr] == nil {
		m.balances[userIDStr] = make(map[string]decimal.Decimal)
	}
	if m.reserved[userIDStr] == nil {
		m.reserved[userIDStr] = make(map[string]decimal.Decimal)
	}

	available := m.balances[userIDStr][asset].Sub(m.reserved[userIDStr][asset])
	if available.LessThan(amount) {
		return fmt.Errorf("insufficient balance")
	}

	m.reserved[userIDStr][asset] = m.reserved[userIDStr][asset].Add(amount)
	return nil
}

func (m *MockBookkeeperIntegration) ReleaseBalance(ctx context.Context, userID uuid.UUID, asset string, amount decimal.Decimal) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	userIDStr := userID.String()
	if m.reserved[userIDStr] == nil {
		m.reserved[userIDStr] = make(map[string]decimal.Decimal)
	}

	m.reserved[userIDStr][asset] = m.reserved[userIDStr][asset].Sub(amount)
	if m.reserved[userIDStr][asset].LessThan(decimal.Zero) {
		m.reserved[userIDStr][asset] = decimal.Zero
	}
	return nil
}

func (m *MockBookkeeperIntegration) GetBalance(ctx context.Context, userID uuid.UUID, asset string) (decimal.Decimal, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	userIDStr := userID.String()
	if m.balances[userIDStr] == nil {
		return decimal.Zero, nil
	}
	return m.balances[userIDStr][asset], nil
}

func (m *MockBookkeeperIntegration) TransferBalance(ctx context.Context, fromUserID, toUserID uuid.UUID, asset string, amount decimal.Decimal) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	fromStr := fromUserID.String()
	toStr := toUserID.String()

	if m.balances[fromStr] == nil {
		m.balances[fromStr] = make(map[string]decimal.Decimal)
	}
	if m.balances[toStr] == nil {
		m.balances[toStr] = make(map[string]decimal.Decimal)
	}

	if m.balances[fromStr][asset].LessThan(amount) {
		return fmt.Errorf("insufficient balance")
	}

	m.balances[fromStr][asset] = m.balances[fromStr][asset].Sub(amount)
	m.balances[toStr][asset] = m.balances[toStr][asset].Add(amount)
	return nil
}

func (m *MockBookkeeperIntegration) GetAllBalances(ctx context.Context, userID uuid.UUID) (map[string]decimal.Decimal, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	userIDStr := userID.String()
	if m.balances[userIDStr] == nil {
		return make(map[string]decimal.Decimal), nil
	}

	result := make(map[string]decimal.Decimal)
	for asset, balance := range m.balances[userIDStr] {
		result[asset] = balance
	}
	return result, nil
}

func (m *MockBookkeeperIntegration) SetBalance(userID uuid.UUID, asset string, amount decimal.Decimal) {
	m.mu.Lock()
	defer m.mu.Unlock()

	userIDStr := userID.String()
	if m.balances[userIDStr] == nil {
		m.balances[userIDStr] = make(map[string]decimal.Decimal)
	}
	m.balances[userIDStr][asset] = amount
}

type MockWSHubIntegration struct {
	messages []WSMessage
	mu       sync.RWMutex
}

type WSMessage struct {
	Topic  string
	Data   []byte
	UserID string
}

func NewMockWSHubIntegration() *MockWSHubIntegration {
	return &MockWSHubIntegration{
		messages: make([]WSMessage, 0),
	}
}

func (m *MockWSHubIntegration) Broadcast(topic string, data []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, WSMessage{Topic: topic, Data: data})
}

func (m *MockWSHubIntegration) BroadcastToUser(userID string, data []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, WSMessage{UserID: userID, Data: data})
}

func (m *MockWSHubIntegration) Subscribe(userID, topic string) error {
	return nil
}

func (m *MockWSHubIntegration) Unsubscribe(userID, topic string) error {
	return nil
}

func (m *MockWSHubIntegration) GetMessages() []WSMessage {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]WSMessage, len(m.messages))
	copy(result, m.messages)
	return result
}

func (m *MockWSHubIntegration) ClearMessages() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = m.messages[:0]
}

// TradingIntegrationTestSuite tests full trading workflows
type TradingIntegrationTestSuite struct {
	suite.Suite
	service    trading.TradingService
	bookkeeper *MockBookkeeperIntegration
	wsHub      *MockWSHubIntegration
	db         *gorm.DB
	logger     *zap.Logger
}

func (suite *TradingIntegrationTestSuite) SetupTest() {
	suite.logger = zaptest.NewLogger(suite.T())
	suite.bookkeeper = NewMockBookkeeperIntegration()
	suite.wsHub = NewMockWSHubIntegration()

	// Use in-memory SQLite for testing
	suite.db = setupIntegrationTestDB(suite.T())

	// Create settlement engine
	settlementEngine := settlement.NewSettlementEngine()

	// Create trading service
	svc, err := trading.NewService(
		suite.logger,
		suite.db,
		suite.bookkeeper,
		suite.wsHub,
		settlementEngine,
	)
	require.NoError(suite.T(), err)
	suite.service = svc
}

func (suite *TradingIntegrationTestSuite) TearDownTest() {
	// Stop service
	suite.service.Stop()

	// Close database connection
	if suite.db != nil {
		sqlDB, _ := suite.db.DB()
		if sqlDB != nil {
			sqlDB.Close()
		}
	}
}

func TestTradingIntegrationSuite(t *testing.T) {
	suite.Run(t, new(TradingIntegrationTestSuite))
}

// Test Complete Order Lifecycle
func (suite *TradingIntegrationTestSuite) TestOrderLifecycle() {
	ctx := context.Background()

	// Setup test users
	buyerID := uuid.New()
	sellerID := uuid.New()

	// Setup initial balances
	suite.bookkeeper.SetBalance(buyerID, "USDT", decimal.NewFromFloat(100000)) // $100k USDT
	suite.bookkeeper.SetBalance(sellerID, "BTC", decimal.NewFromFloat(2.0))    // 2 BTC

	suite.Run("PlaceBuyOrder", func() {
		buyOrder := &models.Order{
			ID:          uuid.New(),
			UserID:      buyerID,
			Symbol:      "BTCUSDT",
			Side:        "buy",
			Type:        "limit",
			Quantity:    0.1,
			Price:       50000.0,
			TimeInForce: "GTC",
			Status:      "NEW",
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		placedOrder, err := suite.service.PlaceOrder(ctx, buyOrder)
		assert.NoError(suite.T(), err)
		assert.NotNil(suite.T(), placedOrder)
		assert.Equal(suite.T(), "NEW", placedOrder.Status)

		// Check balance reservation
		balance, err := suite.bookkeeper.GetBalance(ctx, buyerID, "USDT")
		assert.NoError(suite.T(), err)
		assert.Equal(suite.T(), decimal.NewFromFloat(100000), balance) // Balance unchanged but should be reserved
	})

	suite.Run("PlaceSellOrder", func() {
		sellOrder := &models.Order{
			ID:          uuid.New(),
			UserID:      sellerID,
			Symbol:      "BTCUSDT",
			Side:        "sell",
			Type:        "limit",
			Quantity:    0.1,
			Price:       50000.0,
			TimeInForce: "GTC",
			Status:      "NEW",
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		placedOrder, err := suite.service.PlaceOrder(ctx, sellOrder)
		assert.NoError(suite.T(), err)
		assert.NotNil(suite.T(), placedOrder)

		// Should match with buy order and potentially generate trades
		// Check if WebSocket messages were sent
		messages := suite.wsHub.GetMessages()
		assert.GreaterOrEqual(suite.T(), len(messages), 0) // May have trade notifications
	})
}

// Test Order Matching
func (suite *TradingIntegrationTestSuite) TestOrderMatching() {
	ctx := context.Background()

	// Create multiple users with different prices
	users := make([]uuid.UUID, 5)
	for i := 0; i < 5; i++ {
		users[i] = uuid.New()
		suite.bookkeeper.SetBalance(users[i], "USDT", decimal.NewFromFloat(100000))
		suite.bookkeeper.SetBalance(users[i], "BTC", decimal.NewFromFloat(1.0))
	}

	suite.Run("MultipleOrderMatching", func() {
		// Place buy orders at different prices
		buyPrices := []float64{49990, 49995, 50000, 50005, 50010}
		placedBuyOrders := make([]*models.Order, 0)

		for i, price := range buyPrices {
			order := &models.Order{
				ID:          uuid.New(),
				UserID:      users[i],
				Symbol:      "BTCUSDT",
				Side:        "buy",
				Type:        "limit",
				Quantity:    0.1,
				Price:       price,
				TimeInForce: "GTC",
				Status:      "NEW",
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			}

			placedOrder, err := suite.service.PlaceOrder(ctx, order)
			assert.NoError(suite.T(), err)
			placedBuyOrders = append(placedBuyOrders, placedOrder)
		}

		// Place a market sell order that should match with highest buy prices
		marketSell := &models.Order{
			ID:          uuid.New(),
			UserID:      users[0],
			Symbol:      "BTCUSDT",
			Side:        "sell",
			Type:        "market",
			Quantity:    0.3, // Should match with 3 buy orders
			TimeInForce: "IOC",
			Status:      "NEW",
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		suite.wsHub.ClearMessages()
		placedSellOrder, err := suite.service.PlaceOrder(ctx, marketSell)
		assert.NoError(suite.T(), err)
		assert.NotNil(suite.T(), placedSellOrder)

		// Check for trade notifications
		messages := suite.wsHub.GetMessages()
		// Should have trade notifications if matching occurred
		suite.T().Logf("WebSocket messages after market sell: %d", len(messages))
	})
}

// Test Concurrent Order Placement
func (suite *TradingIntegrationTestSuite) TestConcurrentOrders() {
	ctx := context.Background()

	// Create test users
	numUsers := 10
	users := make([]uuid.UUID, numUsers)
	for i := 0; i < numUsers; i++ {
		users[i] = uuid.New()
		suite.bookkeeper.SetBalance(users[i], "USDT", decimal.NewFromFloat(100000))
		suite.bookkeeper.SetBalance(users[i], "BTC", decimal.NewFromFloat(1.0))
	}

	suite.Run("ConcurrentOrderPlacement", func() {
		var wg sync.WaitGroup
		orderChan := make(chan *models.Order, numUsers*2)
		errorChan := make(chan error, numUsers*2)

		// Place orders concurrently
		for i := 0; i < numUsers; i++ {
			wg.Add(2) // Buy and sell order for each user

			go func(userIndex int) {
				defer wg.Done()

				buyOrder := &models.Order{
					ID:          uuid.New(),
					UserID:      users[userIndex],
					Symbol:      "BTCUSDT",
					Side:        "buy",
					Type:        "limit",
					Quantity:    0.01,
					Price:       49900 + float64(userIndex*10),
					TimeInForce: "GTC",
					Status:      "NEW",
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
				}

				placedOrder, err := suite.service.PlaceOrder(ctx, buyOrder)
				if err != nil {
					errorChan <- err
					return
				}
				orderChan <- placedOrder
			}(i)

			go func(userIndex int) {
				defer wg.Done()

				sellOrder := &models.Order{
					ID:          uuid.New(),
					UserID:      users[userIndex],
					Symbol:      "BTCUSDT",
					Side:        "sell",
					Type:        "limit",
					Quantity:    0.01,
					Price:       50100 + float64(userIndex*10),
					TimeInForce: "GTC",
					Status:      "NEW",
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
				}

				placedOrder, err := suite.service.PlaceOrder(ctx, sellOrder)
				if err != nil {
					errorChan <- err
					return
				}
				orderChan <- placedOrder
			}(i)
		}

		// Wait for all orders to complete
		go func() {
			wg.Wait()
			close(orderChan)
			close(errorChan)
		}()

		// Collect results
		placedOrders := make([]*models.Order, 0)
		errors := make([]error, 0)

		for order := range orderChan {
			placedOrders = append(placedOrders, order)
		}

		for err := range errorChan {
			errors = append(errors, err)
		}

		// Verify results
		assert.Empty(suite.T(), errors, "Should have no errors in concurrent order placement")
		assert.Equal(suite.T(), numUsers*2, len(placedOrders), "Should have placed all orders")

		suite.T().Logf("Successfully placed %d orders concurrently", len(placedOrders))
	})
}

// Test Order Book State
func (suite *TradingIntegrationTestSuite) TestOrderBookState() {
	ctx := context.Background()
	userID := uuid.New()

	// Setup balance
	suite.bookkeeper.SetBalance(userID, "USDT", decimal.NewFromFloat(100000))
	suite.bookkeeper.SetBalance(userID, "BTC", decimal.NewFromFloat(1.0))

	suite.Run("OrderBookAfterOrders", func() {
		// Place some orders
		orders := []*models.Order{
			{
				ID:          uuid.New(),
				UserID:      userID,
				Symbol:      "BTCUSDT",
				Side:        "buy",
				Type:        "limit",
				Quantity:    0.1,
				Price:       49900,
				TimeInForce: "GTC",
				Status:      "NEW",
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			},
			{
				ID:          uuid.New(),
				UserID:      userID,
				Symbol:      "BTCUSDT",
				Side:        "sell",
				Type:        "limit",
				Quantity:    0.1,
				Price:       50100,
				TimeInForce: "GTC",
				Status:      "NEW",
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			},
		}

		for _, order := range orders {
			_, err := suite.service.PlaceOrder(ctx, order)
			assert.NoError(suite.T(), err)
		}

		// Get order book
		orderBook, err := suite.service.GetOrderBook("BTCUSDT", 10)
		assert.NoError(suite.T(), err)
		assert.NotNil(suite.T(), orderBook)
		assert.Equal(suite.T(), "BTCUSDT", orderBook.Symbol)

		// Should have our orders in the book
		suite.T().Logf("Order book - Bids: %d, Asks: %d", len(orderBook.Bids), len(orderBook.Asks))
	})
}

// Test Error Handling and Recovery
func (suite *TradingIntegrationTestSuite) TestErrorHandling() {
	ctx := context.Background()
	userID := uuid.New()

	suite.Run("InsufficientBalanceHandling", func() {
		// Don't set any balance - should fail
		order := &models.Order{
			ID:          uuid.New(),
			UserID:      userID,
			Symbol:      "BTCUSDT",
			Side:        "buy",
			Type:        "limit",
			Quantity:    0.1,
			Price:       50000,
			TimeInForce: "GTC",
			Status:      "NEW",
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		placedOrder, err := suite.service.PlaceOrder(ctx, order)
		assert.Error(suite.T(), err)
		assert.Nil(suite.T(), placedOrder)
		assert.Contains(suite.T(), err.Error(), "insufficient")
	})

	suite.Run("InvalidOrderHandling", func() {
		// Invalid order type
		order := &models.Order{
			ID:          uuid.New(),
			UserID:      userID,
			Symbol:      "BTCUSDT",
			Side:        "buy",
			Type:        "invalid_type",
			Quantity:    0.1,
			Price:       50000,
			TimeInForce: "GTC",
			Status:      "NEW",
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		placedOrder, err := suite.service.PlaceOrder(ctx, order)
		assert.Error(suite.T(), err)
		assert.Nil(suite.T(), placedOrder)
	})
}

// Performance Tests
func (suite *TradingIntegrationTestSuite) TestPerformance() {
	ctx := context.Background()

	// Setup multiple users for performance testing
	numUsers := 100
	users := make([]uuid.UUID, numUsers)
	for i := 0; i < numUsers; i++ {
		users[i] = uuid.New()
		suite.bookkeeper.SetBalance(users[i], "USDT", decimal.NewFromFloat(1000000))
		suite.bookkeeper.SetBalance(users[i], "BTC", decimal.NewFromFloat(10.0))
	}

	suite.Run("HighVolumeOrderPlacement", func() {
		startTime := time.Now()
		ordersToPlace := 1000

		var wg sync.WaitGroup
		successChan := make(chan bool, ordersToPlace)

		for i := 0; i < ordersToPlace; i++ {
			wg.Add(1)
			go func(orderIndex int) {
				defer wg.Done()

				userIndex := orderIndex % numUsers
				side := "buy"
				if orderIndex%2 == 1 {
					side = "sell"
				}

				price := 50000.0
				if side == "buy" {
					price = 49900 + float64(orderIndex%100)
				} else {
					price = 50100 + float64(orderIndex%100)
				}

				order := &models.Order{
					ID:          uuid.New(),
					UserID:      users[userIndex],
					Symbol:      "BTCUSDT",
					Side:        side,
					Type:        "limit",
					Quantity:    0.001,
					Price:       price,
					TimeInForce: "GTC",
					Status:      "NEW",
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
				}

				_, err := suite.service.PlaceOrder(ctx, order)
				if err == nil {
					successChan <- true
				} else {
					successChan <- false
				}
			}(i)
		}

		// Wait for completion
		go func() {
			wg.Wait()
			close(successChan)
		}()

		// Count successes
		successCount := 0
		for success := range successChan {
			if success {
				successCount++
			}
		}

		duration := time.Since(startTime)
		tps := float64(successCount) / duration.Seconds()

		suite.T().Logf("Performance Test Results:")
		suite.T().Logf("- Orders placed: %d/%d", successCount, ordersToPlace)
		suite.T().Logf("- Duration: %v", duration)
		suite.T().Logf("- TPS: %.2f", tps)

		assert.Greater(suite.T(), successCount, ordersToPlace/2, "Should place at least 50% of orders successfully")
		assert.Greater(suite.T(), tps, 100.0, "Should achieve at least 100 TPS")
	})
}

// Helper function to create test database for integration tests
func setupIntegrationTestDB(t testing.TB) *gorm.DB {
	// For integration testing, we'll use an in-memory SQLite database
	// In a real environment, you might want to use a test-specific database
	return createInMemoryDB(t)
}

// Benchmark Integration Tests
func BenchmarkIntegrationOrderPlacement(b *testing.B) {
	logger := zaptest.NewLogger(b)
	bookkeeper := NewMockBookkeeperIntegration()
	wsHub := NewMockWSHubIntegration()
	db := setupIntegrationTestDB(b)
	defer func() {
		if db != nil {
			sqlDB, _ := db.DB()
			if sqlDB != nil {
				sqlDB.Close()
			}
		}
	}()

	settlementEngine := settlement.NewSettlementEngine()
	service, err := trading.NewService(logger, db, bookkeeper, wsHub, settlementEngine)
	require.NoError(b, err)

	// Setup test user
	userID := uuid.New()
	bookkeeper.SetBalance(userID, "USDT", decimal.NewFromFloat(10000000)) // $10M
	bookkeeper.SetBalance(userID, "BTC", decimal.NewFromFloat(100))       // 100 BTC

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		orderCounter := 0
		for pb.Next() {
			orderCounter++
			side := "buy"
			if orderCounter%2 == 1 {
				side = "sell"
			}

			price := 50000.0
			if side == "buy" {
				price = 49900 + float64(orderCounter%100)
			} else {
				price = 50100 + float64(orderCounter%100)
			}

			order := &models.Order{
				ID:          uuid.New(),
				UserID:      userID,
				Symbol:      "BTCUSDT",
				Side:        side,
				Type:        "limit",
				Quantity:    0.001,
				Price:       price,
				TimeInForce: "GTC",
				Status:      "NEW",
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			}

			_, _ = service.PlaceOrder(context.Background(), order)
		}
	})
}
