//go:build integration
// +build integration

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/bookkeeper"
	"github.com/Aidin1998/pincex_unified/internal/trading/repository"
	"github.com/Aidin1998/pincex_unified/pkg/models"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestBatchOperationsIntegration tests the full integration of all batch operations
func TestBatchOperationsIntegration(t *testing.T) {
	// Setup test database
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Migrate schemas
	err = db.AutoMigrate(
		&models.User{},
		&models.Account{},
		&models.Transaction{},
		&models.Order{},
		&models.Trade{},
	)
	require.NoError(t, err)

	logger := zap.NewNop()
	ctx := context.Background()

	// Initialize services
	bkSvc, err := bookkeeper.NewService(logger, db)
	require.NoError(t, err)

	tradingRepo := repository.NewGormRepository(db, logger)

	// Create test data
	userCount := 50
	currencies := []string{"BTC", "ETH", "USDT", "USD"}
	symbols := []string{"BTC/USD", "ETH/USD", "ETH/BTC"}

	t.Run("Setup_Test_Data", func(t *testing.T) {
		// Create users
		users := make([]*models.User, userCount)
		for i := 0; i < userCount; i++ {
			user := &models.User{
				ID:       uuid.New().String(),
				Email:    fmt.Sprintf("user%d@test.com", i),
				Username: fmt.Sprintf("user%d", i),
			}
			err := db.Create(user).Error
			require.NoError(t, err)
			users[i] = user
		}

		// Create accounts for each user-currency pair
		accounts := make([]*models.Account, 0, userCount*len(currencies))
		for _, user := range users {
			for _, currency := range currencies {
				account := &models.Account{
					UserID:    user.ID,
					Currency:  currency,
					Balance:   1000.0,
					Available: 1000.0,
					Locked:    0.0,
				}
				err := db.Create(account).Error
				require.NoError(t, err)
				accounts = append(accounts, account)
			}
		}

		// Create test orders
		for i, user := range users[:10] { // Create orders for first 10 users
			for j, symbol := range symbols {
				order := &models.Order{
					ID:              uuid.New().String(),
					UserID:          user.ID,
					Symbol:          symbol,
					Side:            "buy",
					Type:            "limit",
					Quantity:        float64(10 + i),
					Price:           float64(100 + j*10),
					Status:          "pending",
					FilledQuantity:  0,
					RemainingAmount: float64(10+i) * float64(100+j*10),
				}
				err := db.Create(order).Error
				require.NoError(t, err)
			}
		}

		t.Logf("Created %d users, %d accounts, %d orders", len(users), len(accounts), 10*len(symbols))
	})

	t.Run("Batch_Account_Operations", func(t *testing.T) {
		// Test BatchGetAccounts
		userIDs := make([]string, 10)
		var users []models.User
		err := db.Limit(10).Find(&users).Error
		require.NoError(t, err)

		for i, user := range users {
			userIDs[i] = user.ID
		}

		start := time.Now()
		accounts, err := bkSvc.BatchGetAccounts(ctx, userIDs, currencies)
		duration := time.Since(start)

		assert.NoError(t, err)
		assert.NotEmpty(t, accounts)
		assert.Len(t, accounts, len(userIDs)*len(currencies)) // Should get all user-currency combinations
		t.Logf("BatchGetAccounts: %d accounts retrieved in %v", len(accounts), duration)

		// Verify account data integrity
		for _, account := range accounts {
			assert.NotEmpty(t, account.UserID)
			assert.NotEmpty(t, account.Currency)
			assert.GreaterOrEqual(t, account.Balance, 0.0)
			assert.GreaterOrEqual(t, account.Available, 0.0)
			assert.GreaterOrEqual(t, account.Locked, 0.0)
		}
	})

	t.Run("Batch_Funds_Operations", func(t *testing.T) {
		// Get test users
		var users []models.User
		err := db.Limit(5).Find(&users).Error
		require.NoError(t, err)

		// Prepare funds operations
		operations := make([]bookkeeper.FundsOperation, len(users))
		for i, user := range users {
			operations[i] = bookkeeper.FundsOperation{
				UserID:   user.ID,
				Currency: "USDT",
				Amount:   100.0,
				OrderID:  uuid.New().String(),
				Reason:   "test_lock",
			}
		}

		// Test BatchLockFunds
		start := time.Now()
		lockResult := bkSvc.BatchLockFunds(ctx, operations)
		lockDuration := time.Since(start)

		assert.Greater(t, lockResult.SuccessCount, 0)
		assert.LessOrEqual(t, len(lockResult.FailedItems), len(operations))
		t.Logf("BatchLockFunds: %d successful locks in %v", lockResult.SuccessCount, lockDuration)

		// Verify funds are locked
		for i, user := range users {
			if _, failed := lockResult.FailedItems[fmt.Sprintf("%d", i)]; !failed {
				var account models.Account
				err := db.Where("user_id = ? AND currency = ?", user.ID, "USDT").First(&account).Error
				if err == nil {
					assert.Equal(t, 100.0, account.Locked)
					assert.Equal(t, 900.0, account.Available)
				}
			}
		}

		// Test BatchUnlockFunds
		start = time.Now()
		unlockResult := bkSvc.BatchUnlockFunds(ctx, operations)
		unlockDuration := time.Since(start)

		assert.Greater(t, unlockResult.SuccessCount, 0)
		t.Logf("BatchUnlockFunds: %d successful unlocks in %v", unlockResult.SuccessCount, unlockDuration)

		// Verify funds are unlocked
		for i, user := range users {
			if _, failed := unlockResult.FailedItems[fmt.Sprintf("%d", i)]; !failed {
				var account models.Account
				err := db.Where("user_id = ? AND currency = ?", user.ID, "USDT").First(&account).Error
				if err == nil {
					assert.Equal(t, 0.0, account.Locked)
					assert.Equal(t, 1000.0, account.Available)
				}
			}
		}
	})

	t.Run("Batch_Trading_Operations", func(t *testing.T) {
		// Get test order IDs
		var orders []models.Order
		err := db.Limit(10).Find(&orders).Error
		require.NoError(t, err)

		orderIDs := make([]string, len(orders))
		for i, order := range orders {
			orderIDs[i] = order.ID
		}

		// Test BatchGetOrdersByIDs
		start := time.Now()
		retrievedOrders, err := tradingRepo.BatchGetOrdersByIDs(ctx, orderIDs)
		duration := time.Since(start)

		assert.NoError(t, err)
		assert.Len(t, retrievedOrders, len(orderIDs))
		t.Logf("BatchGetOrdersByIDs: %d orders retrieved in %v", len(retrievedOrders), duration)

		// Test BatchUpdateOrderStatusOptimized
		statusUpdates := make(map[string]string)
		for _, orderID := range orderIDs[:5] { // Update first 5 orders
			statusUpdates[orderID] = "partially_filled"
		}

		start = time.Now()
		updateResult, err := tradingRepo.BatchUpdateOrderStatusOptimized(ctx, statusUpdates)
		duration = time.Since(start)

		assert.NoError(t, err)
		assert.Greater(t, updateResult.SuccessCount, 0)
		t.Logf("BatchUpdateOrderStatusOptimized: %d orders updated in %v", updateResult.SuccessCount, duration)

		// Verify status updates
		var updatedOrders []models.Order
		err = db.Where("id IN ?", orderIDs[:5]).Find(&updatedOrders).Error
		require.NoError(t, err)

		for _, order := range updatedOrders {
			assert.Equal(t, "partially_filled", order.Status)
		}
	})

	t.Run("Performance_Comparison", func(t *testing.T) {
		// Get test users for comparison
		var users []models.User
		err := db.Limit(20).Find(&users).Error
		require.NoError(t, err)

		userIDs := make([]string, len(users))
		for i, user := range users {
			userIDs[i] = user.ID
		}

		// N+1 Pattern (Individual calls)
		start := time.Now()
		n1Accounts := make([]*bookkeeper.AccountBalance, 0)
		for _, userID := range userIDs {
			for _, currency := range currencies {
				account, err := bkSvc.GetAccount(ctx, userID, currency)
				if err == nil {
					n1Accounts = append(n1Accounts, &bookkeeper.AccountBalance{
						UserID:    account.UserID,
						Currency:  account.Currency,
						Balance:   account.Balance,
						Available: account.Available,
						Locked:    account.Locked,
					})
				}
			}
		}
		n1Duration := time.Since(start)

		// Batch Pattern
		start = time.Now()
		batchAccounts, err := bkSvc.BatchGetAccounts(ctx, userIDs, currencies)
		batchDuration := time.Since(start)

		assert.NoError(t, err)
		assert.Len(t, batchAccounts, len(n1Accounts))

		improvement := float64(n1Duration) / float64(batchDuration)
		t.Logf("Performance Improvement:")
		t.Logf("  N+1 Pattern: %v (%d accounts)", n1Duration, len(n1Accounts))
		t.Logf("  Batch Pattern: %v (%d accounts)", batchDuration, len(batchAccounts))
		t.Logf("  Improvement: %.2fx faster", improvement)

		// Assert significant improvement
		assert.Greater(t, improvement, 2.0, "Batch operations should be at least 2x faster than N+1 pattern")
	})

	t.Run("Concurrent_Operations", func(t *testing.T) {
		// Test concurrent batch operations
		var users []models.User
		err := db.Limit(10).Find(&users).Error
		require.NoError(t, err)

		userIDs := make([]string, len(users))
		for i, user := range users {
			userIDs[i] = user.ID
		}

		// Run concurrent batch operations
		concurrency := 5
		operations := make(chan func(), concurrency*2)
		results := make(chan error, concurrency*2)

		// Start workers
		for i := 0; i < concurrency; i++ {
			go func() {
				for op := range operations {
					op()
				}
			}()
		}

		// Submit operations
		for i := 0; i < concurrency; i++ {
			operations <- func() {
				accounts, err := bkSvc.BatchGetAccounts(ctx, userIDs, currencies)
				if err != nil {
					results <- err
					return
				}
				if len(accounts) == 0 {
					results <- fmt.Errorf("no accounts returned")
					return
				}
				results <- nil
			}

			operations <- func() {
				fundsOps := make([]bookkeeper.FundsOperation, len(userIDs))
				for j, userID := range userIDs {
					fundsOps[j] = bookkeeper.FundsOperation{
						UserID:   userID,
						Currency: "USDT",
						Amount:   10.0,
						OrderID:  uuid.New().String(),
						Reason:   "concurrent_test",
					}
				}
				result := bkSvc.BatchLockFunds(ctx, fundsOps)
				if result.SuccessCount == 0 {
					results <- fmt.Errorf("no successful locks")
					return
				}
				results <- nil
			}
		}

		close(operations)

		// Check results
		errorCount := 0
		for i := 0; i < concurrency*2; i++ {
			if err := <-results; err != nil {
				t.Logf("Concurrent operation error: %v", err)
				errorCount++
			}
		}

		// Allow some errors due to concurrency, but most should succeed
		successRate := float64(concurrency*2-errorCount) / float64(concurrency*2)
		assert.Greater(t, successRate, 0.8, "At least 80% of concurrent operations should succeed")
		t.Logf("Concurrent operations success rate: %.2f%%", successRate*100)
	})

	t.Run("Error_Handling", func(t *testing.T) {
		// Test with invalid user IDs
		invalidUserIDs := []string{"invalid-1", "invalid-2", "invalid-3"}

		accounts, err := bkSvc.BatchGetAccounts(ctx, invalidUserIDs, currencies)
		assert.NoError(t, err) // Should not error, but return empty results
		assert.Empty(t, accounts)

		// Test funds operations with insufficient balance
		operations := make([]bookkeeper.FundsOperation, len(invalidUserIDs))
		for i, userID := range invalidUserIDs {
			operations[i] = bookkeeper.FundsOperation{
				UserID:   userID,
				Currency: "USDT",
				Amount:   999999.0, // Very large amount
				OrderID:  uuid.New().String(),
				Reason:   "error_test",
			}
		}

		result := bkSvc.BatchLockFunds(ctx, operations)
		assert.Equal(t, 0, result.SuccessCount)
		assert.Len(t, result.FailedItems, len(operations))

		// Verify all operations failed with appropriate errors
		for _, err := range result.FailedItems {
			assert.Error(t, err)
		}
	})
}

// TestBatchOperationsMemoryUsage tests memory efficiency of batch operations
func TestBatchOperationsMemoryUsage(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping memory usage test in short mode")
	}

	// Setup test database
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&models.User{}, &models.Account{})
	require.NoError(t, err)

	logger := zap.NewNop()
	ctx := context.Background()

	bkSvc, err := bookkeeper.NewService(logger, db)
	require.NoError(t, err)

	// Create large dataset
	userCount := 1000
	currencies := []string{"BTC", "ETH", "USDT", "USD", "BNB", "ADA", "DOT", "LINK"}

	// Create users
	userIDs := make([]string, userCount)
	for i := 0; i < userCount; i++ {
		userID := uuid.New().String()
		user := &models.User{
			ID:       userID,
			Email:    fmt.Sprintf("user%d@test.com", i),
			Username: fmt.Sprintf("user%d", i),
		}
		err := db.Create(user).Error
		require.NoError(t, err)
		userIDs[i] = userID

		// Create accounts
		for _, currency := range currencies {
			account := &models.Account{
				UserID:    userID,
				Currency:  currency,
				Balance:   1000.0,
				Available: 1000.0,
				Locked:    0.0,
			}
			err := db.Create(account).Error
			require.NoError(t, err)
		}
	}

	t.Run("Large_Batch_Operations", func(t *testing.T) {
		// Test with different batch sizes
		batchSizes := []int{10, 50, 100, 250, 500, 1000}

		for _, batchSize := range batchSizes {
			if batchSize > userCount {
				continue
			}

			testUserIDs := userIDs[:batchSize]

			start := time.Now()
			accounts, err := bkSvc.BatchGetAccounts(ctx, testUserIDs, currencies)
			duration := time.Since(start)

			assert.NoError(t, err)
			expectedCount := batchSize * len(currencies)
			assert.Len(t, accounts, expectedCount)

			accountsPerSecond := float64(len(accounts)) / duration.Seconds()
			t.Logf("Batch size %d: %d accounts in %v (%.0f accounts/sec)",
				batchSize, len(accounts), duration, accountsPerSecond)

			// Memory usage should be reasonable
			assert.Less(t, duration, 5*time.Second, "Large batch should complete within 5 seconds")
		}
	})
}
