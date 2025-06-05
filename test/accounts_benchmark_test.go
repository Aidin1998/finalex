// Ultra-high concurrency benchmark tests for 100k+ RPS validation
// Comprehensive performance testing suite for the Accounts module
//go:build benchmark
// +build benchmark

package test

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/go-redsync/redsync/v4"
	"github.com/go-redsync/redsync/v4/redis/goredis/v8"
	goredislib "github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"c:/Orbit CEX/Finalex/internal/accounts"
)

// BenchmarkSuite provides comprehensive performance testing for 100k+ RPS
type BenchmarkSuite struct {
	repo        *accounts.Repository
	writeDB     *gorm.DB
	readDB      *gorm.DB
	pgxPool     *pgxpool.Pool
	cache       *accounts.CacheLayer
	redsync     *redsync.Redsync
	redisClient goredislib.UniversalClient
	logger      *zap.Logger
	ctx         context.Context
	testAccounts []TestAccount
}

type TestAccount struct {
	UserID   uuid.UUID
	Currency string
	Balance  decimal.Decimal
}

func setupBenchmarkSuite(b *testing.B) *BenchmarkSuite {
	ctx := context.Background()
	logger := zap.NewNop()

	// PostgreSQL setup with optimized settings for high concurrency
	dsn := "host=localhost user=test password=test dbname=test_accounts port=5432 sslmode=disable " +
		"pool_max_conns=1000 pool_min_conns=100 pool_max_conn_lifetime=1h " +
		"pool_max_conn_idle_time=30m application_name=accounts_benchmark"

	writeDB, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		PrepareStmt:              true,
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	require.NoError(b, err)

	// Configure GORM for high performance
	sqlDB, _ := writeDB.DB()
	sqlDB.SetMaxOpenConns(1000)
	sqlDB.SetMaxIdleConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)
	sqlDB.SetConnMaxIdleTime(30 * time.Minute)

	// Setup pgx pool for raw SQL operations
	pgxConfig, err := pgxpool.ParseConfig(dsn)
	require.NoError(b, err)
	pgxConfig.MaxConns = 1000
	pgxConfig.MinConns = 100
	pgxConfig.MaxConnLifetime = time.Hour
	pgxConfig.MaxConnIdleTime = 30 * time.Minute

	pgxPool, err := pgxpool.NewWithConfig(ctx, pgxConfig)
	require.NoError(b, err)

	// Redis setup with clustering simulation
	redisClient := goredislib.NewClusterClient(&goredislib.ClusterOptions{
		Addrs:    []string{"localhost:6379"}, // In real setup, this would be multiple nodes
		Password: "",
		PoolSize: 1000,
		MinIdleConns: 100,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  1 * time.Second,
		WriteTimeout: 1 * time.Second,
		PoolTimeout:  2 * time.Second,
		IdleTimeout:  10 * time.Minute,
	})

	// Fallback to single Redis if cluster not available
	if err := redisClient.Ping(ctx).Err(); err != nil {
		redisClient.Close()
		redisClient = goredislib.NewClient(&goredislib.Options{
			Addr:         "localhost:6379",
			DB:           1,
			PoolSize:     1000,
			MinIdleConns: 100,
			DialTimeout:  5 * time.Second,
			ReadTimeout:  1 * time.Second,
			WriteTimeout: 1 * time.Second,
			PoolTimeout:  2 * time.Second,
			IdleTimeout:  10 * time.Minute,
		})
	}

	require.NoError(b, redisClient.Ping(ctx).Err())

	// Setup distributed locking
	pool := goredis.NewPool(redisClient)
	redsyncClient := redsync.New(pool)

	// Cache setup for ultra-high concurrency
	cacheConfig := &accounts.CacheConfig{
		HotRedisAddr:  []string{"localhost:6379"},
		WarmRedisAddr: []string{"localhost:6379"},
		ColdRedisAddr: []string{"localhost:6379"},
		PoolSize:      1000,
		MaxRetries:    3,
		DialTimeout:   "5s",
		ReadTimeout:   "500ms",
		WriteTimeout:  "500ms",
		PoolTimeout:   "1s",
		IdleTimeout:   "10m",
	}

	cache, err := accounts.NewCacheLayer(cacheConfig, logger)
	require.NoError(b, err)

	repo := accounts.NewRepository(writeDB, writeDB, pgxPool, cache, redsyncClient, logger)

	suite := &BenchmarkSuite{
		repo:        repo,
		writeDB:     writeDB,
		readDB:      writeDB,
		pgxPool:     pgxPool,
		cache:       cache,
		redsync:     redsyncClient,
		redisClient: redisClient,
		logger:      logger,
		ctx:         ctx,
	}

	// Setup test data
	suite.setupTestData(b)

	return suite
}

func (s *BenchmarkSuite) setupTestData(b *testing.B) {
	// Create test tables
	err := s.writeDB.AutoMigrate(&accounts.Account{}, &accounts.Reservation{})
	require.NoError(b, err)

	// Clean existing data
	s.writeDB.Exec("TRUNCATE TABLE accounts CASCADE")
	s.redisClient.FlushDB(s.ctx)

	// Create test accounts for concurrent access
	numAccounts := 10000
	currencies := []string{"BTC", "ETH", "USDT", "ADA", "DOT", "LINK", "UNI", "AAVE", "SUSHI", "CRV"}

	s.testAccounts = make([]TestAccount, 0, numAccounts)

	for i := 0; i < numAccounts; i++ {
		userID := uuid.New()
		currency := currencies[i%len(currencies)]
		balance := decimal.NewFromFloat(1000.0 + rand.Float64()*9000.0) // Random balance 1000-10000

		account := &accounts.Account{
			ID:        uuid.New(),
			UserID:    userID,
			Currency:  currency,
			Balance:   balance,
			Available: balance.Mul(decimal.NewFromFloat(0.8)), // 80% available
			Locked:    balance.Mul(decimal.NewFromFloat(0.2)), // 20% locked
			Version:   1,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		err := s.writeDB.Create(account).Error
		require.NoError(b, err)

		s.testAccounts = append(s.testAccounts, TestAccount{
			UserID:   userID,
			Currency: currency,
			Balance:  balance,
		})
	}

	b.Logf("Created %d test accounts for benchmarking", numAccounts)
}

func (s *BenchmarkSuite) cleanup() {
	if s.cache != nil {
		s.cache.Close()
	}
	if s.redisClient != nil {
		s.redisClient.Close()
	}
	if s.pgxPool != nil {
		s.pgxPool.Close()
	}
}

func (s *BenchmarkSuite) getRandomAccount() TestAccount {
	return s.testAccounts[rand.Intn(len(s.testAccounts))]
}

// Test 100k+ RPS for account balance queries (most frequent operation)
func BenchmarkAccountBalanceQueries100K(b *testing.B) {
	suite := setupBenchmarkSuite(b)
	defer suite.cleanup()

	b.SetParallelism(runtime.NumCPU() * 4) // Maximize parallelism
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			account := suite.getRandomAccount()
			_, _, _, err := suite.repo.GetAccountBalance(suite.ctx, account.UserID, account.Currency)
			if err != nil {
				b.Errorf("GetAccountBalance failed: %v", err)
			}
		}
	})

	// Calculate and report RPS
	rps := float64(b.N) / b.Elapsed().Seconds()
	b.ReportMetric(rps, "rps")
	
	if rps < 100000 {
		b.Logf("WARNING: RPS %.0f is below 100k target", rps)
	} else {
		b.Logf("SUCCESS: Achieved %.0f RPS (>100k)", rps)
	}
}

// Test high-concurrency account updates with distributed locking
func BenchmarkAccountUpdates(b *testing.B) {
	suite := setupBenchmarkSuite(b)
	defer suite.cleanup()

	b.SetParallelism(runtime.NumCPU() * 2) // Moderate parallelism for updates
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			account := suite.getRandomAccount()
			
			update := &accounts.BalanceUpdate{
				UserID:       account.UserID,
				Currency:     account.Currency,
				BalanceDelta: decimal.NewFromFloat(rand.Float64() * 10), // Random small update
				LockedDelta:  decimal.Zero,
				Type:         "benchmark",
				ReferenceID:  fmt.Sprintf("bench_%d", rand.Int63()),
			}

			err := suite.repo.UpdateBalanceAtomic(suite.ctx, update)
			if err != nil {
				b.Errorf("UpdateBalanceAtomic failed: %v", err)
			}
		}
	})

	rps := float64(b.N) / b.Elapsed().Seconds()
	b.ReportMetric(rps, "rps")
	b.Logf("Account updates: %.0f RPS", rps)
}

// Test cache performance under extreme load
func BenchmarkCacheOperations100K(b *testing.B) {
	suite := setupBenchmarkSuite(b)
	defer suite.cleanup()

	b.SetParallelism(runtime.NumCPU() * 8) // High parallelism for cache
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			account := suite.getRandomAccount()
			
			// Mix of cache operations
			switch rand.Intn(3) {
			case 0: // Get account balance (most common)
				suite.cache.GetAccountBalance(suite.ctx, account.UserID, account.Currency)
			case 1: // Get full account
				suite.cache.GetAccount(suite.ctx, account.UserID, account.Currency)
			case 2: // Set balance (updates)
				suite.cache.SetAccountBalance(suite.ctx, account.UserID, account.Currency,
					account.Balance, account.Balance.Mul(decimal.NewFromFloat(0.8)), 1)
			}
		}
	})

	rps := float64(b.N) / b.Elapsed().Seconds()
	b.ReportMetric(rps, "rps")
	
	if rps < 100000 {
		b.Logf("WARNING: Cache RPS %.0f is below 100k target", rps)
	} else {
		b.Logf("SUCCESS: Cache achieved %.0f RPS (>100k)", rps)
	}
}

// Test mixed workload simulating real trading activity
func BenchmarkMixedWorkload(b *testing.B) {
	suite := setupBenchmarkSuite(b)
	defer suite.cleanup()

	b.SetParallelism(runtime.NumCPU() * 4)
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			account := suite.getRandomAccount()
			
			// Simulate real trading workload distribution:
			// 70% reads, 20% updates, 10% reservations
			switch rand.Intn(10) {
			case 0, 1, 2, 3, 4, 5, 6: // 70% reads
				suite.repo.GetAccountBalance(suite.ctx, account.UserID, account.Currency)
			case 7, 8: // 20% updates
				update := &accounts.BalanceUpdate{
					UserID:       account.UserID,
					Currency:     account.Currency,
					BalanceDelta: decimal.NewFromFloat((rand.Float64() - 0.5) * 100), // +/- updates
					LockedDelta:  decimal.Zero,
					Type:         "trade",
					ReferenceID:  fmt.Sprintf("trade_%d", rand.Int63()),
				}
				suite.repo.UpdateBalanceAtomic(suite.ctx, update)
			case 9: // 10% reservations
				reservation := &accounts.Reservation{
					ID:          uuid.New(),
					UserID:      account.UserID,
					Currency:    account.Currency,
					Amount:      decimal.NewFromFloat(rand.Float64() * 10),
					Type:        "order",
					ReferenceID: fmt.Sprintf("order_%d", rand.Int63()),
					Status:      "active",
					Version:     1,
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
				}
				suite.repo.CreateReservation(suite.ctx, reservation)
			}
		}
	})

	rps := float64(b.N) / b.Elapsed().Seconds()
	b.ReportMetric(rps, "rps")
	b.Logf("Mixed workload: %.0f RPS", rps)
}

// Test system under sustained load for endurance
func BenchmarkSustainedLoad(b *testing.B) {
	suite := setupBenchmarkSuite(b)
	defer suite.cleanup()

	duration := 60 * time.Second // 1 minute sustained load
	b.Logf("Running sustained load test for %v", duration)

	ctx, cancel := context.WithTimeout(suite.ctx, duration)
	defer cancel()

	var totalOps int64
	var wg sync.WaitGroup

	// Start multiple workers
	numWorkers := runtime.NumCPU() * 4
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ops := 0
			
			for {
				select {
				case <-ctx.Done():
					return
				default:
					account := suite.getRandomAccount()
					suite.repo.GetAccountBalance(suite.ctx, account.UserID, account.Currency)
					ops++
					
					if ops%1000 == 0 {
						// Periodically add to total
						totalOps += 1000
						ops = 0
					}
				}
			}
		}()
	}

	wg.Wait()
	
	rps := float64(totalOps) / duration.Seconds()
	b.ReportMetric(rps, "sustained_rps")
	b.Logf("Sustained load: %.0f RPS over %v", rps, duration)
}

// Test resource usage and memory efficiency
func BenchmarkResourceUsage(b *testing.B) {
	suite := setupBenchmarkSuite(b)
	defer suite.cleanup()

	var startMem, endMem runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&startMem)

	b.ResetTimer()
	b.SetParallelism(runtime.NumCPU() * 2)

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			account := suite.getRandomAccount()
			suite.repo.GetAccountBalance(suite.ctx, account.UserID, account.Currency)
		}
	})

	runtime.GC()
	runtime.ReadMemStats(&endMem)

	memUsed := endMem.Alloc - startMem.Alloc
	allocsPerOp := float64(endMem.Mallocs-startMem.Mallocs) / float64(b.N)

	b.ReportMetric(float64(memUsed), "bytes_used")
	b.ReportMetric(allocsPerOp, "allocs_per_op")
	b.Logf("Memory used: %d bytes, Allocs per op: %.2f", memUsed, allocsPerOp)
}

// Test connection pool efficiency under high load
func BenchmarkConnectionPool(b *testing.B) {
	suite := setupBenchmarkSuite(b)
	defer suite.cleanup()

	b.SetParallelism(runtime.NumCPU() * 10) // High connection pressure
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			account := suite.getRandomAccount()
			
			// Direct database query to test connection pool
			var balance decimal.Decimal
			err := suite.pgxPool.QueryRow(suite.ctx,
				"SELECT balance FROM accounts WHERE user_id = $1 AND currency = $2",
				account.UserID, account.Currency).Scan(&balance)
			
			if err != nil && err != sql.ErrNoRows {
				b.Errorf("Database query failed: %v", err)
			}
		}
	})

	// Check connection pool stats
	stats := suite.pgxPool.Stat()
	b.Logf("Connection pool - Total: %d, Idle: %d, Used: %d", 
		stats.TotalConns(), stats.IdleConns(), stats.AcquiredConns())
}

// Test distributed locking performance
func BenchmarkDistributedLocking(b *testing.B) {
	suite := setupBenchmarkSuite(b)
	defer suite.cleanup()

	b.SetParallelism(runtime.NumCPU())
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			account := suite.getRandomAccount()
			lockKey := fmt.Sprintf("account:lock:%s:%s", account.UserID.String(), account.Currency)
			
			mutex := suite.redsync.NewMutex(lockKey)
			if err := mutex.Lock(); err == nil {
				// Simulate work under lock
				time.Sleep(time.Microsecond * 100)
				mutex.Unlock()
			}
		}
	})

	rps := float64(b.N) / b.Elapsed().Seconds()
	b.ReportMetric(rps, "locks_per_second")
	b.Logf("Distributed locks: %.0f locks/sec", rps)
}

// Test error handling under extreme conditions
func BenchmarkErrorHandling(b *testing.B) {
	suite := setupBenchmarkSuite(b)
	defer suite.cleanup()

	b.SetParallelism(runtime.NumCPU() * 2)
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			// Try operations on non-existent accounts to test error paths
			nonExistentUserID := uuid.New()
			
			_, _, _, err := suite.repo.GetAccountBalance(suite.ctx, nonExistentUserID, "BTC")
			// Error is expected, just ensure it's handled gracefully
			_ = err
		}
	})

	rps := float64(b.N) / b.Elapsed().Seconds()
	b.ReportMetric(rps, "error_handling_rps")
	b.Logf("Error handling: %.0f RPS", rps)
}

// Comprehensive system benchmark combining all operations
func BenchmarkFullSystem100K(b *testing.B) {
	suite := setupBenchmarkSuite(b)
	defer suite.cleanup()

	b.Logf("Starting comprehensive 100k+ RPS system benchmark")
	b.SetParallelism(runtime.NumCPU() * 6)
	b.ResetTimer()

	var operations [8]int64 // Track different operation counts

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			account := suite.getRandomAccount()
			
			// Distribute operations based on real-world patterns
			opType := rand.Intn(100)
			
			switch {
			case opType < 50: // 50% balance queries
				suite.repo.GetAccountBalance(suite.ctx, account.UserID, account.Currency)
				operations[0]++
			case opType < 75: // 25% account queries
				suite.repo.GetAccount(suite.ctx, account.UserID, account.Currency)
				operations[1]++
			case opType < 90: // 15% cache operations
				suite.cache.GetAccountBalance(suite.ctx, account.UserID, account.Currency)
				operations[2]++
			case opType < 95: // 5% balance updates
				update := &accounts.BalanceUpdate{
					UserID:       account.UserID,
					Currency:     account.Currency,
					BalanceDelta: decimal.NewFromFloat((rand.Float64() - 0.5) * 10),
					LockedDelta:  decimal.Zero,
					Type:         "system_test",
					ReferenceID:  fmt.Sprintf("sys_%d", rand.Int63()),
				}
				suite.repo.UpdateBalanceAtomic(suite.ctx, update)
				operations[3]++
			case opType < 98: // 3% user account queries
				suite.repo.GetUserAccounts(suite.ctx, account.UserID)
				operations[4]++
			default: // 2% reservations
				reservation := &accounts.Reservation{
					ID:          uuid.New(),
					UserID:      account.UserID,
					Currency:    account.Currency,
					Amount:      decimal.NewFromFloat(rand.Float64() * 5),
					Type:        "system_test",
					ReferenceID: fmt.Sprintf("sys_res_%d", rand.Int63()),
					Status:      "active",
					Version:     1,
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
				}
				suite.repo.CreateReservation(suite.ctx, reservation)
				operations[5]++
			}
		}
	})

	totalRps := float64(b.N) / b.Elapsed().Seconds()
	b.ReportMetric(totalRps, "total_rps")

	if totalRps >= 100000 {
		b.Logf("✅ SUCCESS: System achieved %.0f RPS (>100k target)", totalRps)
	} else {
		b.Logf("❌ WARNING: System achieved %.0f RPS (<100k target)", totalRps)
	}

	b.Logf("Operation breakdown:")
	b.Logf("  Balance queries: %d", operations[0])
	b.Logf("  Account queries: %d", operations[1])
	b.Logf("  Cache operations: %d", operations[2])
	b.Logf("  Balance updates: %d", operations[3])
	b.Logf("  User account queries: %d", operations[4])
	b.Logf("  Reservations: %d", operations[5])
}
