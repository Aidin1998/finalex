//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/bookkeeper"	"github.com/Aidin1998/pincex_unified/internal/core/database"
	"github.com/Aidin1998/pincex_unified/internal/core/fiat"
	"github.com/Aidin1998/pincex_unified/internal/messaging"
	"github.com/Aidin1998/pincex_unified/internal/monitoring"
	"github.com/Aidin1998/pincex_unified/internal/trading/repository"
	"github.com/Aidin1998/pincex_unified/pkg/models"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// E2ETestSuite provides a complete end-to-end testing environment
type E2ETestSuite struct {
	db               *gorm.DB
	redis            *redis.Client
	logger           *zap.Logger
	perfMonitor      *monitoring.PerformanceMonitor
	bookkeeper       *bookkeeper.Service
	tradingRepo      repository.TradingRepository
	fiatBatchService *fiat.BatchService
	messagingService *messaging.BookkeeperService
	optimizedRepo    *database.OptimizedRepository
}

// setupE2ETestSuite initializes all components for end-to-end testing
func setupE2ETestSuite(t *testing.T) *E2ETestSuite {
	// Initialize database
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Migrate all models
	err = db.AutoMigrate(
		&models.User{},
		&models.Account{},
		&models.Transaction{},
		&models.Order{},
		&models.Trade{},
	)
	require.NoError(t, err)

	// Initialize Redis (mock)
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
		DB:   1, // Use test database
	})

	// Initialize logger
	logger := zap.NewNop()

	// Initialize performance monitor
	perfMonitor := monitoring.NewPerformanceMonitor(logger)

	// Initialize services
	bookkeeper, err := bookkeeper.NewService(logger, db)
	require.NoError(t, err)

	tradingRepo := repository.NewGormRepository(db, logger)

	// Initialize optimized repository
	readWriteDB := &database.ReadWriteDB{
		Reader: db,
		Writer: db,
	}

	optimizedRepo := database.NewOptimizedRepository(
		readWriteDB,
		rdb,
		logger,
		database.DefaultOptimizedRepositoryConfig(),
	)

	return &E2ETestSuite{
		db:            db,
		redis:         rdb,
		logger:        logger,
		perfMonitor:   perfMonitor,
		bookkeeper:    bookkeeper,
		tradingRepo:   tradingRepo,
		optimizedRepo: optimizedRepo,
	}
}

// TestCompleteWorkflow tests the complete batch operations workflow
func TestCompleteWorkflow(t *testing.T) {
	suite := setupE2ETestSuite(t)
	ctx := context.Background()

	// Test scenario: High-frequency trading platform processing multiple operations
	t.Run("High_Frequency_Trading_Scenario", func(t *testing.T) {
		// Step 1: Setup trading environment
		userCount := 100
		symbols := []string{"BTC/USD", "ETH/USD", "ETH/BTC", "LTC/USD", "ADA/USD"}
		currencies := []string{"BTC", "ETH", "USD", "LTC", "ADA"}

		users := suite.createTestUsers(t, userCount)
		suite.createTestAccounts(t, users, currencies)

		t.Logf("Created %d users with accounts for %d currencies", len(users), len(currencies))

		// Step 2: Simulate order placement workflow
		t.Run("Order_Placement_Workflow", func(t *testing.T) {
			ctx := suite.perfMonitor.StartOperation("order_placement_workflow", len(users)*2)

			// Phase 1: Batch get user accounts
			userIDs := make([]string, len(users))
			for i, user := range users {
				userIDs[i] = user.ID
			}

			accounts, err := suite.bookkeeper.BatchGetAccounts(ctx.OperationType, userIDs, currencies)
			assert.NoError(t, err)
			assert.NotEmpty(t, accounts)

			// Phase 2: Create batch orders
			orders := suite.createBatchOrders(t, users, symbols)

			// Phase 3: Batch lock funds for orders
			fundsOperations := make([]bookkeeper.FundsOperation, len(orders))
			for i, order := range orders {
				fundsOperations[i] = bookkeeper.FundsOperation{
					UserID:   order.UserID,
					Currency: "USD", // Assuming all orders use USD as quote currency
					Amount:   order.RemainingAmount,
					OrderID:  order.ID,
					Reason:   "order_placement",
				}
			}

			lockResult := suite.bookkeeper.BatchLockFunds(ctx.OperationType, fundsOperations)
			assert.Greater(t, lockResult.SuccessCount, 0)

			suite.perfMonitor.EndOperation(ctx, lockResult.SuccessCount > 0, len(orders), nil)
			t.Logf("Order placement workflow: %d orders processed, %d funds locked",
				len(orders), lockResult.SuccessCount)
		})

		// Step 3: Simulate order matching and execution
		t.Run("Order_Matching_Workflow", func(t *testing.T) {
			ctx := suite.perfMonitor.StartOperation("order_matching_workflow", 50)

			// Get pending orders for matching
			var pendingOrders []models.Order
			err := suite.db.Where("status = ?", "pending").Limit(50).Find(&pendingOrders).Error
			require.NoError(t, err)

			if len(pendingOrders) == 0 {
				t.Skip("No pending orders available for matching")
				return
			}

			// Batch update order statuses to filled
			statusUpdates := make(map[string]string)
			for _, order := range pendingOrders[:25] { // Fill half the orders
				statusUpdates[order.ID] = "filled"
			}

			updateResult, err := suite.tradingRepo.BatchUpdateOrderStatusOptimized(ctx.OperationType, statusUpdates)
			assert.NoError(t, err)
			assert.Greater(t, updateResult.SuccessCount, 0)

			// Batch unlock funds for filled orders
			unlockOperations := make([]bookkeeper.FundsOperation, len(statusUpdates))
			i := 0
			for orderID := range statusUpdates {
				for _, order := range pendingOrders {
					if order.ID == orderID {
						unlockOperations[i] = bookkeeper.FundsOperation{
							UserID:   order.UserID,
							Currency: "USD",
							Amount:   order.RemainingAmount,
							OrderID:  order.ID,
							Reason:   "order_filled",
						}
						i++
						break
					}
				}
			}

			unlockResult := suite.bookkeeper.BatchUnlockFunds(ctx.OperationType, unlockOperations[:i])

			suite.perfMonitor.EndOperation(ctx, unlockResult.SuccessCount > 0, len(statusUpdates), nil)
			t.Logf("Order matching workflow: %d orders matched, %d funds unlocked",
				len(statusUpdates), unlockResult.SuccessCount)
		})

		// Step 4: Simulate settlement workflow
		t.Run("Settlement_Workflow", func(t *testing.T) {
			ctx := suite.perfMonitor.StartOperation("settlement_workflow", 100)

			// Get filled orders for settlement
			var filledOrders []models.Order
			err := suite.db.Where("status = ?", "filled").Find(&filledOrders).Error
			require.NoError(t, err)

			// Batch create settlement transactions
			transactions := make([]*fiat.BatchTransactionRequest, 0, len(filledOrders))
			for _, order := range filledOrders {
				transactions = append(transactions, &fiat.BatchTransactionRequest{
					UserID:      order.UserID,
					Type:        "settlement",
					Currency:    "USD",
					Amount:      order.RemainingAmount,
					Reference:   fmt.Sprintf("settlement_%s", order.ID),
					Description: fmt.Sprintf("Settlement for order %s", order.ID),
				})
			}

			if len(transactions) > 0 {
				// Note: Would call fiat batch service here if implemented
				t.Logf("Settlement workflow: %d settlement transactions prepared", len(transactions))
			}

			suite.perfMonitor.EndOperation(ctx, true, len(transactions), nil)
		})

		// Step 5: Performance analysis
		t.Run("Performance_Analysis", func(t *testing.T) {
			metrics := suite.perfMonitor.GetMetrics()

			t.Logf("=== End-to-End Performance Results ===")
			t.Logf("Total Operations: %d", metrics.TotalOperations)
			t.Logf("Success Rate: %.2f%%", suite.perfMonitor.GetSuccessRate()*100)
			t.Logf("Average Response Time: %v", metrics.AverageResponseTime)
			t.Logf("Operations/sec: %.2f", metrics.OperationsPerSecond)
			t.Logf("Items/sec: %.2f", metrics.ItemsPerSecond)

			// Assert performance requirements
			assert.Greater(t, suite.perfMonitor.GetSuccessRate(), 0.95, "Success rate should be > 95%")
			assert.Less(t, metrics.AverageResponseTime, time.Second, "Average response time should be < 1s")
			assert.Greater(t, metrics.OperationsPerSecond, 10.0, "Should process > 10 operations/sec")

			suite.perfMonitor.PrintSummary()
		})
	})
}

// TestScalabilityLimits tests the system under extreme load
func TestScalabilityLimits(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping scalability test in short mode")
	}

	suite := setupE2ETestSuite(t)
	ctx := context.Background()

	t.Run("Large_Scale_Operations", func(t *testing.T) {
		// Create large dataset
		userCount := 1000
		currencies := []string{"BTC", "ETH", "USD", "EUR", "GBP", "JPY", "AUD", "CAD"}

		users := suite.createTestUsers(t, userCount)
		suite.createTestAccounts(t, users, currencies)

		t.Logf("Created %d users with %d currencies each", userCount, len(currencies))

		// Test different batch sizes to find optimal configuration
		batchSizes := []int{10, 50, 100, 250, 500, 1000}

		for _, batchSize := range batchSizes {
			if batchSize > userCount {
				continue
			}

			t.Run(fmt.Sprintf("BatchSize_%d", batchSize), func(t *testing.T) {
				ctx := suite.perfMonitor.StartOperation(fmt.Sprintf("batch_%d", batchSize), batchSize)

				userIDs := make([]string, batchSize)
				for i := 0; i < batchSize; i++ {
					userIDs[i] = users[i].ID
				}

				start := time.Now()
				accounts, err := suite.bookkeeper.BatchGetAccounts(ctx.OperationType, userIDs, currencies)
				duration := time.Since(start)

				success := err == nil && len(accounts) > 0
				suite.perfMonitor.EndOperation(ctx, success, len(accounts), err)

				if success {
					throughput := float64(len(accounts)) / duration.Seconds()
					t.Logf("Batch size %d: %d accounts in %v (%.0f accounts/sec)",
						batchSize, len(accounts), duration, throughput)

					// Assert reasonable performance
					assert.Less(t, duration, 10*time.Second, "Large batches should complete within 10s")
					assert.Greater(t, throughput, 100.0, "Should process > 100 accounts/sec")
				} else {
					t.Logf("Batch size %d failed: %v", batchSize, err)
				}
			})
		}

		// Print final performance summary
		suite.perfMonitor.PrintSummary()
	})
}

// TestConcurrentOperations tests system behavior under concurrent load
func TestConcurrentOperations(t *testing.T) {
	suite := setupE2ETestSuite(t)
	ctx := context.Background()

	t.Run("Concurrent_Batch_Operations", func(t *testing.T) {
		userCount := 100
		currencies := []string{"BTC", "ETH", "USD"}

		users := suite.createTestUsers(t, userCount)
		suite.createTestAccounts(t, users, currencies)

		// Run concurrent operations
		concurrency := 10
		operations := make(chan func(), concurrency*3)
		done := make(chan bool, concurrency)

		// Start workers
		for i := 0; i < concurrency; i++ {
			go func(workerID int) {
				for op := range operations {
					op()
				}
				done <- true
			}(i)
		}

		// Submit different types of operations concurrently
		userIDs := make([]string, len(users))
		for i, user := range users {
			userIDs[i] = user.ID
		}

		for i := 0; i < concurrency; i++ {
			// Account retrieval operations
			operations <- func() {
				ctx := suite.perfMonitor.StartOperation("concurrent_get_accounts", 20)
				testUserIDs := userIDs[i*10 : (i+1)*10] // 10 users per operation
				accounts, err := suite.bookkeeper.BatchGetAccounts(ctx.OperationType, testUserIDs, currencies)
				suite.perfMonitor.EndOperation(ctx, err == nil, len(accounts), err)
			}

			// Funds locking operations
			operations <- func() {
				ctx := suite.perfMonitor.StartOperation("concurrent_lock_funds", 10)
				testUserIDs := userIDs[i*10 : (i+1)*10]
				fundsOps := make([]bookkeeper.FundsOperation, len(testUserIDs))
				for j, userID := range testUserIDs {
					fundsOps[j] = bookkeeper.FundsOperation{
						UserID:   userID,
						Currency: "USD",
						Amount:   10.0,
						OrderID:  uuid.New().String(),
						Reason:   "concurrent_test",
					}
				}
				result := suite.bookkeeper.BatchLockFunds(ctx.OperationType, fundsOps)
				suite.perfMonitor.EndOperation(ctx, result.SuccessCount > 0, result.SuccessCount, nil)
			}

			// Order operations
			operations <- func() {
				ctx := suite.perfMonitor.StartOperation("concurrent_orders", 5)
				orders := suite.createBatchOrders(t, users[i*10:(i+1)*10], []string{"BTC/USD"})
				suite.perfMonitor.EndOperation(ctx, len(orders) > 0, len(orders), nil)
			}
		}

		close(operations)

		// Wait for all workers to complete
		for i := 0; i < concurrency; i++ {
			<-done
		}

		// Analyze concurrent performance
		metrics := suite.perfMonitor.GetMetrics()
		successRate := suite.perfMonitor.GetSuccessRate()

		t.Logf("Concurrent operations completed:")
		t.Logf("Total operations: %d", metrics.TotalOperations)
		t.Logf("Success rate: %.2f%%", successRate*100)
		t.Logf("Average response time: %v", metrics.AverageResponseTime)

		// Assert acceptable performance under concurrency
		assert.Greater(t, successRate, 0.8, "Success rate should be > 80% under concurrency")
		assert.Less(t, metrics.AverageResponseTime, 5*time.Second, "Response time should be reasonable under load")

		suite.perfMonitor.PrintSummary()
	})
}

// Helper methods for test setup

func (suite *E2ETestSuite) createTestUsers(t *testing.T, count int) []*models.User {
	users := make([]*models.User, count)
	for i := 0; i < count; i++ {
		user := &models.User{
			ID:       uuid.New().String(),
			Email:    fmt.Sprintf("user%d@test.com", i),
			Username: fmt.Sprintf("user%d", i),
		}
		err := suite.db.Create(user).Error
		require.NoError(t, err)
		users[i] = user
	}
	return users
}

func (suite *E2ETestSuite) createTestAccounts(t *testing.T, users []*models.User, currencies []string) {
	for _, user := range users {
		for _, currency := range currencies {
			account := &models.Account{
				UserID:    user.ID,
				Currency:  currency,
				Balance:   10000.0, // Start with substantial balance
				Available: 10000.0,
				Locked:    0.0,
			}
			err := suite.db.Create(account).Error
			require.NoError(t, err)
		}
	}
}

func (suite *E2ETestSuite) createBatchOrders(t *testing.T, users []*models.User, symbols []string) []*models.Order {
	orders := make([]*models.Order, 0, len(users)*len(symbols))

	for _, user := range users {
		for _, symbol := range symbols {
			order := &models.Order{
				ID:              uuid.New().String(),
				UserID:          user.ID,
				Symbol:          symbol,
				Side:            "buy",
				Type:            "limit",
				Quantity:        10.0,
				Price:           100.0,
				Status:          "pending",
				FilledQuantity:  0,
				RemainingAmount: 1000.0,
			}
			err := suite.db.Create(order).Error
			require.NoError(t, err)
			orders = append(orders, order)
		}
	}

	return orders
}
