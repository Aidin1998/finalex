// Ultra-high concurrency cache layer tests for the Accounts module
// Tests Redis-based hot/warm/cold data tiering with comprehensive scenarios
//go:build integration
// +build integration

package test

import (
	"context"
	"testing"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.uber.org/zap"

	"c:/Orbit CEX/Finalex/internal/accounts"
)

type CacheTestSuite struct {
	suite.Suite
	cache       *accounts.CacheLayer
	redisClient redis.UniversalClient
	logger      *zap.Logger
	ctx         context.Context
}

func (suite *CacheTestSuite) SetupSuite() {
	suite.ctx = context.Background()
	suite.logger = zap.NewNop()

	// Setup test Redis client
	suite.redisClient = redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "",
		DB:       1, // Use test database
	})

	// Test Redis connection
	err := suite.redisClient.Ping(suite.ctx).Err()
	require.NoError(suite.T(), err, "Redis should be available for integration tests")

	// Setup cache configuration
	config := &accounts.CacheConfig{
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

	cache, err := accounts.NewCacheLayer(config, suite.logger)
	require.NoError(suite.T(), err)
	suite.cache = cache
}

func (suite *CacheTestSuite) TearDownSuite() {
	if suite.cache != nil {
		suite.cache.Close()
	}
	if suite.redisClient != nil {
		suite.redisClient.Close()
	}
}

func (suite *CacheTestSuite) SetupTest() {
	// Clean test database before each test
	suite.redisClient.FlushDB(suite.ctx)
}

func (suite *CacheTestSuite) TestAccountCaching() {
	userID := uuid.New()
	currency := "BTC"

	// Test data
	account := &accounts.CachedAccount{
		ID:           uuid.New(),
		UserID:       userID,
		Currency:     currency,
		Balance:      decimal.NewFromFloat(1.5),
		Available:    decimal.NewFromFloat(1.2),
		Locked:       decimal.NewFromFloat(0.3),
		Version:      1,
		AccountType:  "spot",
		Status:       "active",
		UpdatedAt:    time.Now(),
		CachedAt:     time.Now(),
		AccessCount:  1,
		LastAccessed: time.Now(),
	}

	// Test SetAccount
	err := suite.cache.SetAccount(suite.ctx, account)
	assert.NoError(suite.T(), err)

	// Test GetAccount
	retrievedAccount, err := suite.cache.GetAccount(suite.ctx, userID, currency)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), retrievedAccount)
	assert.Equal(suite.T(), account.UserID, retrievedAccount.UserID)
	assert.Equal(suite.T(), account.Currency, retrievedAccount.Currency)
	assert.True(suite.T(), account.Balance.Equal(retrievedAccount.Balance))
	assert.True(suite.T(), account.Available.Equal(retrievedAccount.Available))
	assert.True(suite.T(), account.Locked.Equal(retrievedAccount.Locked))
}

func (suite *CacheTestSuite) TestAccountBalanceCaching() {
	userID := uuid.New()
	currency := "ETH"
	balance := decimal.NewFromFloat(5.5)
	available := decimal.NewFromFloat(5.0)
	version := int64(3)

	// Test SetAccountBalance
	err := suite.cache.SetAccountBalance(suite.ctx, userID, currency, balance, available, version)
	assert.NoError(suite.T(), err)

	// Test GetAccountBalance
	retrievedBalance, retrievedAvailable, retrievedVersion, err := suite.cache.GetAccountBalance(suite.ctx, userID, currency)
	assert.NoError(suite.T(), err)
	assert.True(suite.T(), balance.Equal(retrievedBalance))
	assert.True(suite.T(), available.Equal(retrievedAvailable))
	assert.Equal(suite.T(), version, retrievedVersion)
}

func (suite *CacheTestSuite) TestDataTiering() {
	userID := uuid.New()
	currency := "USDT"

	// Create account with low access count (should go to cold tier)
	coldAccount := &accounts.CachedAccount{
		ID:           uuid.New(),
		UserID:       userID,
		Currency:     currency,
		Balance:      decimal.NewFromFloat(1000.0),
		Available:    decimal.NewFromFloat(950.0),
		Locked:       decimal.NewFromFloat(50.0),
		Version:      1,
		AccessCount:  1,
		LastAccessed: time.Now().Add(-2 * time.Hour), // Old access
	}

	err := suite.cache.SetAccount(suite.ctx, coldAccount)
	assert.NoError(suite.T(), err)

	// Create account with high access count (should go to hot tier)
	hotAccount := &accounts.CachedAccount{
		ID:           uuid.New(),
		UserID:       uuid.New(),
		Currency:     currency,
		Balance:      decimal.NewFromFloat(500.0),
		Available:    decimal.NewFromFloat(400.0),
		Locked:       decimal.NewFromFloat(100.0),
		Version:      1,
		AccessCount:  150, // High access count
		LastAccessed: time.Now(),
	}

	err = suite.cache.SetAccount(suite.ctx, hotAccount)
	assert.NoError(suite.T(), err)

	// Verify both accounts can be retrieved
	retrievedCold, err := suite.cache.GetAccount(suite.ctx, coldAccount.UserID, currency)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), retrievedCold)

	retrievedHot, err := suite.cache.GetAccount(suite.ctx, hotAccount.UserID, currency)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), retrievedHot)
}

func (suite *CacheTestSuite) TestCacheInvalidation() {
	userID := uuid.New()
	currency := "BTC"

	// Set account in cache
	account := &accounts.CachedAccount{
		UserID:   userID,
		Currency: currency,
		Balance:  decimal.NewFromFloat(2.0),
	}

	err := suite.cache.SetAccount(suite.ctx, account)
	assert.NoError(suite.T(), err)

	// Verify account exists
	retrievedAccount, err := suite.cache.GetAccount(suite.ctx, userID, currency)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), retrievedAccount)

	// Invalidate cache
	err = suite.cache.InvalidateAccount(suite.ctx, userID, currency)
	assert.NoError(suite.T(), err)

	// Verify account is removed from cache
	retrievedAccount, err = suite.cache.GetAccount(suite.ctx, userID, currency)
	assert.Error(suite.T(), err)
	assert.Equal(suite.T(), redis.Nil, err)
}

func (suite *CacheTestSuite) TestCacheWarmup() {
	// Create multiple accounts for warmup
	accounts := []*accounts.Account{
		{
			ID:        uuid.New(),
			UserID:    uuid.New(),
			Currency:  "BTC",
			Balance:   decimal.NewFromFloat(1.0),
			Available: decimal.NewFromFloat(0.8),
			Locked:    decimal.NewFromFloat(0.2),
			Version:   1,
			UpdatedAt: time.Now(),
		},
		{
			ID:        uuid.New(),
			UserID:    uuid.New(),
			Currency:  "ETH",
			Balance:   decimal.NewFromFloat(10.0),
			Available: decimal.NewFromFloat(9.5),
			Locked:    decimal.NewFromFloat(0.5),
			Version:   1,
			UpdatedAt: time.Now(),
		},
	}

	// Test cache warmup
	err := suite.cache.WarmupCache(suite.ctx, accounts)
	assert.NoError(suite.T(), err)

	// Verify accounts are cached
	for _, account := range accounts {
		cachedAccount, err := suite.cache.GetAccount(suite.ctx, account.UserID, account.Currency)
		assert.NoError(suite.T(), err)
		assert.NotNil(suite.T(), cachedAccount)
		assert.Equal(suite.T(), account.UserID, cachedAccount.UserID)
		assert.Equal(suite.T(), account.Currency, cachedAccount.Currency)
	}
}

func (suite *CacheTestSuite) TestConcurrentAccess() {
	userID := uuid.New()
	currency := "BTC"
	numGoroutines := 100

	// Set initial account
	account := &accounts.CachedAccount{
		UserID:   userID,
		Currency: currency,
		Balance:  decimal.NewFromFloat(1.0),
		Version:  1,
	}

	err := suite.cache.SetAccount(suite.ctx, account)
	assert.NoError(suite.T(), err)

	// Concurrent reads
	done := make(chan bool, numGoroutines)
	errors := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer func() { done <- true }()

			_, err := suite.cache.GetAccount(suite.ctx, userID, currency)
			if err != nil {
				errors <- err
				return
			}
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Check for errors
	select {
	case err := <-errors:
		suite.T().Errorf("Concurrent access error: %v", err)
	default:
		// No errors, test passed
	}
}

func (suite *CacheTestSuite) TestCacheHealth() {
	// Test cache health check
	err := suite.cache.Health(suite.ctx)
	assert.NoError(suite.T(), err)
}

func (suite *CacheTestSuite) TestCacheMissScenarios() {
	userID := uuid.New()
	currency := "UNKNOWN"

	// Test cache miss for non-existent account
	account, err := suite.cache.GetAccount(suite.ctx, userID, currency)
	assert.Error(suite.T(), err)
	assert.Equal(suite.T(), redis.Nil, err)
	assert.Nil(suite.T(), account)

	// Test cache miss for balance
	balance, available, version, err := suite.cache.GetAccountBalance(suite.ctx, userID, currency)
	assert.Error(suite.T(), err)
	assert.True(suite.T(), balance.IsZero())
	assert.True(suite.T(), available.IsZero())
	assert.Equal(suite.T(), int64(0), version)
}

func (suite *CacheTestSuite) TestCacheExpiry() {
	userID := uuid.New()
	currency := "BTC"

	// Set account with short TTL for testing
	err := suite.cache.SetAccountBalance(suite.ctx, userID, currency,
		decimal.NewFromFloat(1.0), decimal.NewFromFloat(0.8), 1)
	assert.NoError(suite.T(), err)

	// Verify account exists
	balance, available, version, err := suite.cache.GetAccountBalance(suite.ctx, userID, currency)
	assert.NoError(suite.T(), err)
	assert.True(suite.T(), decimal.NewFromFloat(1.0).Equal(balance))
	assert.True(suite.T(), decimal.NewFromFloat(0.8).Equal(available))
	assert.Equal(suite.T(), int64(1), version)

	// Wait for TTL to expire (in real implementation, TTL would be much longer)
	// For testing, we'll manually delete to simulate expiry
	key := "account:balance:" + userID.String() + ":" + currency
	suite.redisClient.Del(suite.ctx, key)

	// Verify account is expired/missing
	balance, available, version, err = suite.cache.GetAccountBalance(suite.ctx, userID, currency)
	assert.Error(suite.T(), err)
}

func TestCacheTestSuite(t *testing.T) {
	suite.Run(t, new(CacheTestSuite))
}

// Benchmark tests for cache performance
func BenchmarkCacheOperations(b *testing.B) {
	ctx := context.Background()
	logger := zap.NewNop()

	redisClient := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
		DB:   1,
	})
	defer redisClient.Close()

	config := &accounts.CacheConfig{
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

	cache, err := accounts.NewCacheLayer(config, logger)
	if err != nil {
		b.Fatalf("Failed to create cache: %v", err)
	}
	defer cache.Close()

	// Clean database
	redisClient.FlushDB(ctx)

	userID := uuid.New()
	currency := "BTC"

	b.Run("SetAccount", func(b *testing.B) {
		account := &accounts.CachedAccount{
			ID:       uuid.New(),
			UserID:   userID,
			Currency: currency,
			Balance:  decimal.NewFromFloat(1.0),
			Version:  1,
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			cache.SetAccount(ctx, account)
		}
	})

	b.Run("GetAccount", func(b *testing.B) {
		// Pre-populate cache
		account := &accounts.CachedAccount{
			ID:       uuid.New(),
			UserID:   userID,
			Currency: currency,
			Balance:  decimal.NewFromFloat(1.0),
			Version:  1,
		}
		cache.SetAccount(ctx, account)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			cache.GetAccount(ctx, userID, currency)
		}
	})

	b.Run("SetAccountBalance", func(b *testing.B) {
		balance := decimal.NewFromFloat(1.0)
		available := decimal.NewFromFloat(0.8)
		version := int64(1)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			cache.SetAccountBalance(ctx, userID, currency, balance, available, version)
		}
	})

	b.Run("GetAccountBalance", func(b *testing.B) {
		// Pre-populate cache
		cache.SetAccountBalance(ctx, userID, currency,
			decimal.NewFromFloat(1.0), decimal.NewFromFloat(0.8), 1)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			cache.GetAccountBalance(ctx, userID, currency)
		}
	})

	b.Run("ConcurrentGetAccount", func(b *testing.B) {
		// Pre-populate cache
		account := &accounts.CachedAccount{
			ID:       uuid.New(),
			UserID:   userID,
			Currency: currency,
			Balance:  decimal.NewFromFloat(1.0),
			Version:  1,
		}
		cache.SetAccount(ctx, account)

		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				cache.GetAccount(ctx, userID, currency)
			}
		})
	})
}
