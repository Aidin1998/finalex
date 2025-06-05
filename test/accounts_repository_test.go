// Ultra-high concurrency repository layer tests for the Accounts module
// Tests atomic operations, distributed locking, and optimistic concurrency
//go:build integration
// +build integration

package test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	goredislib "github.com/go-redis/redis/v8"
	"github.com/go-redsync/redsync/v4"
	"github.com/go-redsync/redsync/v4/redis/goredis/v8"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"c:/Orbit CEX/Finalex/internal/accounts"
)

type RepositoryTestSuite struct {
	suite.Suite
	repo        *accounts.Repository
	writeDB     *gorm.DB
	readDB      *gorm.DB
	pgxPool     *pgxpool.Pool
	cache       *accounts.CacheLayer
	redsync     *redsync.Redsync
	redisClient goredislib.UniversalClient
	logger      *zap.Logger
	ctx         context.Context
}

func (suite *RepositoryTestSuite) SetupSuite() {
	suite.ctx = context.Background()
	suite.logger = zap.NewNop()

	// Setup PostgreSQL connection for testing
	dsn := "host=localhost user=test password=test dbname=test_accounts port=5432 sslmode=disable"

	writeDB, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(suite.T(), err, "PostgreSQL should be available for integration tests")
	suite.writeDB = writeDB
	suite.readDB = writeDB // Use same DB for read/write in tests

	// Setup pgx pool
	pgxPool, err := pgxpool.New(suite.ctx, dsn)
	require.NoError(suite.T(), err)
	suite.pgxPool = pgxPool

	// Setup Redis for distributed locking
	suite.redisClient = goredislib.NewClient(&goredislib.Options{
		Addr: "localhost:6379",
		DB:   1,
	})

	err = suite.redisClient.Ping(suite.ctx).Err()
	require.NoError(suite.T(), err, "Redis should be available for integration tests")

	// Setup Redsync for distributed locking
	pool := goredis.NewPool(suite.redisClient)
	suite.redsync = redsync.New(pool)

	// Setup cache
	cacheConfig := &accounts.CacheConfig{
		HotRedisAddr:  []string{"localhost:6379"},
		WarmRedisAddr: []string{"localhost:6379"},
		ColdRedisAddr: []string{"localhost:6379"},
		PoolSize:      10,
		MaxRetries:    3,
		DialTimeout:   "5s",
		ReadTimeout:   "3s",
		WriteTimeout:  "3s",
		PoolTimeout:   "4s",
		IdleTimeout:   "300s",
	}

	cache, err := accounts.NewCacheLayer(cacheConfig, suite.logger)
	require.NoError(suite.T(), err)
	suite.cache = cache

	// Create repository
	suite.repo = accounts.NewRepository(
		suite.writeDB, suite.readDB, suite.pgxPool,
		suite.cache, suite.redsync, suite.logger)

	// Create test tables
	suite.createTestTables()
}

func (suite *RepositoryTestSuite) TearDownSuite() {
	if suite.cache != nil {
		suite.cache.Close()
	}
	if suite.redisClient != nil {
		suite.redisClient.Close()
	}
	if suite.pgxPool != nil {
		suite.pgxPool.Close()
	}
}

func (suite *RepositoryTestSuite) SetupTest() {
	// Clean test data before each test
	suite.cleanTestData()
}

func (suite *RepositoryTestSuite) createTestTables() {
	// Create test tables with partitioning
	err := suite.writeDB.AutoMigrate(
		&accounts.Account{},
		&accounts.Reservation{},
		&accounts.LedgerTransaction{},
		&accounts.TransactionJournal{},
		&accounts.BalanceSnapshot{},
		&accounts.AuditLog{},
	)
	require.NoError(suite.T(), err)
}

func (suite *RepositoryTestSuite) cleanTestData() {
	// Clean all test tables
	tables := []string{
		"accounts", "reservations", "ledger_transactions",
		"transaction_journal", "balance_snapshots", "audit_logs",
	}

	for _, table := range tables {
		suite.writeDB.Exec(fmt.Sprintf("TRUNCATE TABLE %s CASCADE", table))
	}

	// Clean Redis cache
	suite.redisClient.FlushDB(suite.ctx)
}

func (suite *RepositoryTestSuite) TestGetAccount() {
	userID := uuid.New()
	currency := "BTC"

	// Create test account
	account := &accounts.Account{
		ID:        uuid.New(),
		UserID:    userID,
		Currency:  currency,
		Balance:   decimal.NewFromFloat(1.5),
		Available: decimal.NewFromFloat(1.2),
		Locked:    decimal.NewFromFloat(0.3),
		Version:   1,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err := suite.writeDB.Create(account).Error
	require.NoError(suite.T(), err)

	// Test GetAccount
	retrievedAccount, err := suite.repo.GetAccount(suite.ctx, userID, currency)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), retrievedAccount)
	assert.Equal(suite.T(), account.UserID, retrievedAccount.UserID)
	assert.Equal(suite.T(), account.Currency, retrievedAccount.Currency)
	assert.True(suite.T(), account.Balance.Equal(retrievedAccount.Balance))
	assert.True(suite.T(), account.Available.Equal(retrievedAccount.Available))
	assert.True(suite.T(), account.Locked.Equal(retrievedAccount.Locked))
	assert.Equal(suite.T(), account.Version, retrievedAccount.Version)
}

func (suite *RepositoryTestSuite) TestGetAccountBalance() {
	userID := uuid.New()
	currency := "ETH"

	// Create test account
	account := &accounts.Account{
		ID:        uuid.New(),
		UserID:    userID,
		Currency:  currency,
		Balance:   decimal.NewFromFloat(10.5),
		Available: decimal.NewFromFloat(9.8),
		Locked:    decimal.NewFromFloat(0.7),
		Version:   2,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err := suite.writeDB.Create(account).Error
	require.NoError(suite.T(), err)

	// Test GetAccountBalance
	balance, available, version, err := suite.repo.GetAccountBalance(suite.ctx, userID, currency)
	assert.NoError(suite.T(), err)
	assert.True(suite.T(), account.Balance.Equal(balance))
	assert.True(suite.T(), account.Available.Equal(available))
	assert.Equal(suite.T(), account.Version, version)
}

func (suite *RepositoryTestSuite) TestUpdateBalanceAtomic() {
	userID := uuid.New()
	currency := "USDT"

	// Create test account
	account := &accounts.Account{
		ID:        uuid.New(),
		UserID:    userID,
		Currency:  currency,
		Balance:   decimal.NewFromFloat(1000.0),
		Available: decimal.NewFromFloat(900.0),
		Locked:    decimal.NewFromFloat(100.0),
		Version:   1,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err := suite.writeDB.Create(account).Error
	require.NoError(suite.T(), err)

	// Test balance update
	update := &accounts.BalanceUpdate{
		UserID:          userID,
		Currency:        currency,
		BalanceDelta:    decimal.NewFromFloat(500.0),
		LockedDelta:     decimal.NewFromFloat(-50.0),
		Type:            "deposit",
		ReferenceID:     "deposit_123",
		ExpectedVersion: 1,
	}

	err = suite.repo.UpdateBalanceAtomic(suite.ctx, update)
	assert.NoError(suite.T(), err)

	// Verify updated balance
	balance, available, version, err := suite.repo.GetAccountBalance(suite.ctx, userID, currency)
	assert.NoError(suite.T(), err)
	assert.True(suite.T(), decimal.NewFromFloat(1500.0).Equal(balance))
	assert.True(suite.T(), decimal.NewFromFloat(1450.0).Equal(available)) // 1500 - 50
	assert.Equal(suite.T(), int64(2), version)                            // Version incremented
}

func (suite *RepositoryTestSuite) TestOptimisticConcurrencyControl() {
	userID := uuid.New()
	currency := "BTC"

	// Create test account
	account := &accounts.Account{
		ID:        uuid.New(),
		UserID:    userID,
		Currency:  currency,
		Balance:   decimal.NewFromFloat(1.0),
		Available: decimal.NewFromFloat(0.8),
		Locked:    decimal.NewFromFloat(0.2),
		Version:   1,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err := suite.writeDB.Create(account).Error
	require.NoError(suite.T(), err)

	// First update should succeed
	update1 := &accounts.BalanceUpdate{
		UserID:          userID,
		Currency:        currency,
		BalanceDelta:    decimal.NewFromFloat(0.5),
		LockedDelta:     decimal.Zero,
		Type:            "deposit",
		ReferenceID:     "deposit_1",
		ExpectedVersion: 1,
	}

	err = suite.repo.UpdateBalanceAtomic(suite.ctx, update1)
	assert.NoError(suite.T(), err)

	// Second update with stale version should fail
	update2 := &accounts.BalanceUpdate{
		UserID:          userID,
		Currency:        currency,
		BalanceDelta:    decimal.NewFromFloat(0.2),
		LockedDelta:     decimal.Zero,
		Type:            "deposit",
		ReferenceID:     "deposit_2",
		ExpectedVersion: 1, // Stale version
	}

	err = suite.repo.UpdateBalanceAtomic(suite.ctx, update2)
	assert.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "optimistic lock failed")
}

func (suite *RepositoryTestSuite) TestConcurrentBalanceUpdates() {
	userID := uuid.New()
	currency := "ETH"

	// Create test account
	account := &accounts.Account{
		ID:        uuid.New(),
		UserID:    userID,
		Currency:  currency,
		Balance:   decimal.NewFromFloat(100.0),
		Available: decimal.NewFromFloat(100.0),
		Locked:    decimal.Zero,
		Version:   1,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err := suite.writeDB.Create(account).Error
	require.NoError(suite.T(), err)

	// Concurrent updates
	numUpdates := 10
	updateAmount := decimal.NewFromFloat(1.0)

	var wg sync.WaitGroup
	successCount := int32(0)
	errorCount := int32(0)

	for i := 0; i < numUpdates; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			update := &accounts.BalanceUpdate{
				UserID:       userID,
				Currency:     currency,
				BalanceDelta: updateAmount,
				LockedDelta:  decimal.Zero,
				Type:         "deposit",
				ReferenceID:  fmt.Sprintf("concurrent_deposit_%d", index),
			}

			err := suite.repo.UpdateBalanceAtomic(suite.ctx, update)
			if err != nil {
				errorCount++
			} else {
				successCount++
			}
		}(i)
	}

	wg.Wait()

	// All updates should succeed due to optimistic concurrency control retries
	assert.Equal(suite.T(), int32(numUpdates), successCount)
	assert.Equal(suite.T(), int32(0), errorCount)

	// Verify final balance
	balance, _, _, err := suite.repo.GetAccountBalance(suite.ctx, userID, currency)
	assert.NoError(suite.T(), err)
	expectedBalance := decimal.NewFromFloat(100.0).Add(updateAmount.Mul(decimal.NewFromInt(int64(numUpdates))))
	assert.True(suite.T(), expectedBalance.Equal(balance))
}

func (suite *RepositoryTestSuite) TestCreateReservation() {
	userID := uuid.New()
	currency := "BTC"

	// Create test account
	account := &accounts.Account{
		ID:        uuid.New(),
		UserID:    userID,
		Currency:  currency,
		Balance:   decimal.NewFromFloat(2.0),
		Available: decimal.NewFromFloat(2.0),
		Locked:    decimal.Zero,
		Version:   1,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err := suite.writeDB.Create(account).Error
	require.NoError(suite.T(), err)

	// Create reservation
	reservation := &accounts.Reservation{
		ID:          uuid.New(),
		UserID:      userID,
		Currency:    currency,
		Amount:      decimal.NewFromFloat(0.5),
		Type:        "order",
		ReferenceID: "order_123",
		Status:      "active",
		Version:     1,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	err = suite.repo.CreateReservation(suite.ctx, reservation)
	assert.NoError(suite.T(), err)

	// Verify reservation was created
	var retrievedReservation accounts.Reservation
	err = suite.writeDB.Where("id = ?", reservation.ID).First(&retrievedReservation).Error
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), reservation.UserID, retrievedReservation.UserID)
	assert.Equal(suite.T(), reservation.Currency, retrievedReservation.Currency)
	assert.True(suite.T(), reservation.Amount.Equal(retrievedReservation.Amount))

	// Verify account balance was updated (funds locked)
	balance, available, _, err := suite.repo.GetAccountBalance(suite.ctx, userID, currency)
	assert.NoError(suite.T(), err)
	assert.True(suite.T(), decimal.NewFromFloat(2.0).Equal(balance))   // Balance unchanged
	assert.True(suite.T(), decimal.NewFromFloat(1.5).Equal(available)) // Available reduced
}

func (suite *RepositoryTestSuite) TestReleaseReservation() {
	userID := uuid.New()
	currency := "ETH"

	// Create test account with locked funds
	account := &accounts.Account{
		ID:        uuid.New(),
		UserID:    userID,
		Currency:  currency,
		Balance:   decimal.NewFromFloat(10.0),
		Available: decimal.NewFromFloat(8.0),
		Locked:    decimal.NewFromFloat(2.0),
		Version:   1,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err := suite.writeDB.Create(account).Error
	require.NoError(suite.T(), err)

	// Create reservation
	reservation := &accounts.Reservation{
		ID:          uuid.New(),
		UserID:      userID,
		Currency:    currency,
		Amount:      decimal.NewFromFloat(2.0),
		Type:        "order",
		ReferenceID: "order_456",
		Status:      "active",
		Version:     1,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	err = suite.writeDB.Create(reservation).Error
	require.NoError(suite.T(), err)

	// Release reservation
	err = suite.repo.ReleaseReservation(suite.ctx, reservation.ID)
	assert.NoError(suite.T(), err)

	// Verify reservation status changed
	var updatedReservation accounts.Reservation
	err = suite.writeDB.Where("id = ?", reservation.ID).First(&updatedReservation).Error
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), "released", updatedReservation.Status)

	// Verify funds were unlocked
	balance, available, _, err := suite.repo.GetAccountBalance(suite.ctx, userID, currency)
	assert.NoError(suite.T(), err)
	assert.True(suite.T(), decimal.NewFromFloat(10.0).Equal(balance))   // Balance unchanged
	assert.True(suite.T(), decimal.NewFromFloat(10.0).Equal(available)) // Available increased
}

func (suite *RepositoryTestSuite) TestGetUserAccounts() {
	userID := uuid.New()

	// Create multiple accounts for the user
	currencies := []string{"BTC", "ETH", "USDT"}
	for _, currency := range currencies {
		account := &accounts.Account{
			ID:        uuid.New(),
			UserID:    userID,
			Currency:  currency,
			Balance:   decimal.NewFromFloat(100.0),
			Available: decimal.NewFromFloat(80.0),
			Locked:    decimal.NewFromFloat(20.0),
			Version:   1,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		err := suite.writeDB.Create(account).Error
		require.NoError(suite.T(), err)
	}

	// Test GetUserAccounts
	accounts, err := suite.repo.GetUserAccounts(suite.ctx, userID)
	assert.NoError(suite.T(), err)
	assert.Len(suite.T(), accounts, len(currencies))

	// Verify all currencies are present
	foundCurrencies := make(map[string]bool)
	for _, account := range accounts {
		foundCurrencies[account.Currency] = true
		assert.Equal(suite.T(), userID, account.UserID)
		assert.True(suite.T(), decimal.NewFromFloat(100.0).Equal(account.Balance))
	}

	for _, currency := range currencies {
		assert.True(suite.T(), foundCurrencies[currency],
			fmt.Sprintf("Currency %s should be found", currency))
	}
}

func (suite *RepositoryTestSuite) TestRepositoryHealth() {
	// Test repository health check
	err := suite.repo.Health(suite.ctx)
	assert.NoError(suite.T(), err)
}

func (suite *RepositoryTestSuite) TestInsufficientFunds() {
	userID := uuid.New()
	currency := "BTC"

	// Create test account with limited balance
	account := &accounts.Account{
		ID:        uuid.New(),
		UserID:    userID,
		Currency:  currency,
		Balance:   decimal.NewFromFloat(1.0),
		Available: decimal.NewFromFloat(1.0),
		Locked:    decimal.Zero,
		Version:   1,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err := suite.writeDB.Create(account).Error
	require.NoError(suite.T(), err)

	// Try to withdraw more than available
	update := &accounts.BalanceUpdate{
		UserID:       userID,
		Currency:     currency,
		BalanceDelta: decimal.NewFromFloat(-2.0), // Withdraw more than balance
		LockedDelta:  decimal.Zero,
		Type:         "withdrawal",
		ReferenceID:  "withdrawal_123",
	}

	err = suite.repo.UpdateBalanceAtomic(suite.ctx, update)
	assert.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "insufficient funds")
}

func TestRepositoryTestSuite(t *testing.T) {
	suite.Run(t, new(RepositoryTestSuite))
}

// Benchmark tests for repository performance
func BenchmarkRepositoryOperations(b *testing.B) {
	// Skip if not running benchmarks
	if testing.Short() {
		b.Skip("Skipping benchmark in short mode")
	}

	ctx := context.Background()
	logger := zap.NewNop()

	// Setup test database (same as test suite)
	dsn := "host=localhost user=test password=test dbname=test_accounts port=5432 sslmode=disable"
	writeDB, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		b.Skipf("PostgreSQL not available: %v", err)
	}

	pgxPool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		b.Skipf("pgx pool creation failed: %v", err)
	}
	defer pgxPool.Close()

	redisClient := goredislib.NewClient(&goredislib.Options{
		Addr: "localhost:6379",
		DB:   1,
	})
	defer redisClient.Close()

	if err := redisClient.Ping(ctx).Err(); err != nil {
		b.Skipf("Redis not available: %v", err)
	}

	pool := goredis.NewPool(redisClient)
	redsyncClient := redsync.New(pool)

	cacheConfig := &accounts.CacheConfig{
		HotRedisAddr:  []string{"localhost:6379"},
		WarmRedisAddr: []string{"localhost:6379"},
		ColdRedisAddr: []string{"localhost:6379"},
		PoolSize:      50,
		MaxRetries:    3,
		DialTimeout:   "5s",
		ReadTimeout:   "1s",
		WriteTimeout:  "1s",
		PoolTimeout:   "2s",
		IdleTimeout:   "300s",
	}

	cache, err := accounts.NewCacheLayer(cacheConfig, logger)
	if err != nil {
		b.Fatalf("Failed to create cache: %v", err)
	}
	defer cache.Close()

	repo := accounts.NewRepository(writeDB, writeDB, pgxPool, cache, redsyncClient, logger)

	// Clean test data
	writeDB.Exec("TRUNCATE TABLE accounts CASCADE")
	redisClient.FlushDB(ctx)

	// Setup test account
	userID := uuid.New()
	currency := "BTC"
	account := &accounts.Account{
		ID:        uuid.New(),
		UserID:    userID,
		Currency:  currency,
		Balance:   decimal.NewFromFloat(1000.0),
		Available: decimal.NewFromFloat(1000.0),
		Locked:    decimal.Zero,
		Version:   1,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	writeDB.Create(account)

	b.Run("GetAccount", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			repo.GetAccount(ctx, userID, currency)
		}
	})

	b.Run("GetAccountBalance", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			repo.GetAccountBalance(ctx, userID, currency)
		}
	})

	b.Run("UpdateBalanceAtomic", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			update := &accounts.BalanceUpdate{
				UserID:       userID,
				Currency:     currency,
				BalanceDelta: decimal.NewFromFloat(0.001),
				LockedDelta:  decimal.Zero,
				Type:         "test",
				ReferenceID:  fmt.Sprintf("bench_%d", i),
			}
			repo.UpdateBalanceAtomic(ctx, update)
		}
	})

	b.Run("ConcurrentGetAccount", func(b *testing.B) {
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				repo.GetAccount(ctx, userID, currency)
			}
		})
	})
}
