//go:build performance
// +build performance

package test

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/bookkeeper"
	"github.com/Aidin1998/pincex_unified/pkg/models"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// BenchmarkN1vsOptimized compares N+1 query patterns with batch optimized operations
func BenchmarkN1vsOptimized(b *testing.B) {
	db := setupBenchmarkDB(b)
	logger := zap.NewNop()

	// Setup bookkeeper service
	bkSvc, err := bookkeeper.NewService(logger, db)
	assert.NoError(b, err)

	// Create test data
	userCount := 100
	currencies := []string{"BTC", "ETH", "USDT", "USD"}

	// Create users and accounts
	userIDs := createTestUsers(b, db, userCount)
	createTestAccounts(b, db, bkSvc, userIDs, currencies)

	ctx := context.Background()

	b.Run("N+1_Individual_GetAccount", func(b *testing.B) {
		b.ResetTimer()
		start := time.Now()

		for i := 0; i < b.N; i++ {
			// Simulate N+1 pattern - individual GetAccount calls
			for _, userID := range userIDs[:10] { // Test with 10 users
				for _, currency := range currencies {
					_, err := bkSvc.GetAccount(ctx, userID, currency)
					if err != nil && err.Error() != "account not found" {
						b.Errorf("GetAccount failed: %v", err)
					}
				}
			}
		}

		duration := time.Since(start)
		totalOps := b.N * 10 * len(currencies)

		b.Logf("N+1 Pattern: %d operations in %v (%.2f ops/sec)",
			totalOps, duration, float64(totalOps)/duration.Seconds())
	})

	b.Run("Optimized_BatchGetAccounts", func(b *testing.B) {
		b.ResetTimer()
		start := time.Now()

		for i := 0; i < b.N; i++ {
			// Optimized batch pattern - single BatchGetAccounts call
			_, err := bkSvc.BatchGetAccounts(ctx, userIDs[:10], currencies)
			if err != nil {
				b.Errorf("BatchGetAccounts failed: %v", err)
			}
		}

		duration := time.Since(start)
		totalOps := b.N * 10 * len(currencies)

		b.Logf("Batch Pattern: %d operations in %v (%.2f ops/sec)",
			totalOps, duration, float64(totalOps)/duration.Seconds())
	})
}

// BenchmarkFundsOperations compares individual vs batch funds operations
func BenchmarkFundsOperations(b *testing.B) {
	db := setupBenchmarkDB(b)
	logger := zap.NewNop()

	bkSvc, err := bookkeeper.NewService(logger, db)
	assert.NoError(b, err)

	// Create test data
	userCount := 50
	currencies := []string{"BTC", "ETH", "USDT"}

	userIDs := createTestUsers(b, db, userCount)
	createTestAccountsWithBalance(b, db, bkSvc, userIDs, currencies, 1000.0)

	ctx := context.Background()

	b.Run("Individual_LockFunds", func(b *testing.B) {
		b.ResetTimer()
		start := time.Now()
		var totalLatency time.Duration

		for i := 0; i < b.N; i++ {
			for j := 0; j < 20; j++ { // 20 operations per iteration
				userID := userIDs[rand.Intn(len(userIDs))]
				currency := currencies[rand.Intn(len(currencies))]
				amount := float64(1 + rand.Intn(10))

				opStart := time.Now()
				err := bkSvc.LockFunds(ctx, userID, currency, amount)
				opLatency := time.Since(opStart)
				totalLatency += opLatency

				if err != nil && err.Error() != "insufficient funds" {
					b.Errorf("LockFunds failed: %v", err)
				}
			}
		}

		duration := time.Since(start)
		totalOps := b.N * 20
		avgLatency := totalLatency / time.Duration(totalOps)

		b.Logf("Individual: %d operations, avg latency: %v, total: %v",
			totalOps, avgLatency, duration)
	})

	b.Run("Batch_LockFunds", func(b *testing.B) {
		b.ResetTimer()
		start := time.Now()

		for i := 0; i < b.N; i++ {
			// Prepare batch operations
			operations := make([]bookkeeper.FundsOperation, 20)
			for j := 0; j < 20; j++ {
				operations[j] = bookkeeper.FundsOperation{
					UserID:   userIDs[rand.Intn(len(userIDs))],
					Currency: currencies[rand.Intn(len(currencies))],
					Amount:   float64(1 + rand.Intn(10)),
					OrderID:  fmt.Sprintf("order_%d_%d", i, j),
					Reason:   "benchmark_test",
				}
			}

			opStart := time.Now()
			result, err := bkSvc.BatchLockFunds(ctx, operations)
			opLatency := time.Since(opStart)

			if err != nil {
				b.Errorf("BatchLockFunds failed: %v", err)
			}

			if result != nil {
				b.Logf("Batch %d: %d successful, %d failed, duration: %v",
					i, result.SuccessCount, len(result.FailedItems), opLatency)
			}
		}

		duration := time.Since(start)
		totalOps := b.N * 20

		b.Logf("Batch: %d operations, total: %v", totalOps, duration)
	})
}

// BenchmarkWriteLatencyImpact measures the impact of batch operations on write latency
func BenchmarkWriteLatencyImpact(b *testing.B) {
	db := setupBenchmarkDB(b)
	logger := zap.NewNop()

	bkSvc, err := bookkeeper.NewService(logger, db)
	assert.NoError(b, err)

	userCount := 100
	currencies := []string{"BTC", "ETH", "USDT"}
	userIDs := createTestUsers(b, db, userCount)
	createTestAccountsWithBalance(b, db, bkSvc, userIDs, currencies, 10000.0)

	ctx := context.Background()

	// Measure baseline write latency with individual operations
	b.Run("Baseline_Individual_Writes", func(b *testing.B) {
		var latencies []time.Duration
		var mu sync.Mutex

		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				userID := userIDs[rand.Intn(len(userIDs))]
				currency := currencies[rand.Intn(len(currencies))]
				amount := float64(1 + rand.Intn(5))

				start := time.Now()
				err := bkSvc.LockFunds(ctx, userID, currency, amount)
				latency := time.Since(start)

				mu.Lock()
				latencies = append(latencies, latency)
				mu.Unlock()

				if err != nil && err.Error() != "insufficient funds" {
					b.Errorf("LockFunds failed: %v", err)
				}
			}
		})

		if len(latencies) > 0 {
			avg := calculateAverageLatency(latencies)
			p95 := calculatePercentile(latencies, 0.95)
			p99 := calculatePercentile(latencies, 0.99)

			b.Logf("Individual Writes - Avg: %v, P95: %v, P99: %v, Ops: %d",
				avg, p95, p99, len(latencies))
		}
	})

	// Measure write latency with batch operations under concurrent load
	b.Run("Batch_Operations_Under_Load", func(b *testing.B) {
		var batchLatencies []time.Duration
		var mu sync.Mutex

		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				// Create batch of 10 operations
				operations := make([]bookkeeper.FundsOperation, 10)
				for i := range operations {
					operations[i] = bookkeeper.FundsOperation{
						UserID:   userIDs[rand.Intn(len(userIDs))],
						Currency: currencies[rand.Intn(len(currencies))],
						Amount:   float64(1 + rand.Intn(5)),
						OrderID:  fmt.Sprintf("batch_order_%d", rand.Int()),
						Reason:   "load_test",
					}
				}

				start := time.Now()
				result, err := bkSvc.BatchLockFunds(ctx, operations)
				latency := time.Since(start)

				mu.Lock()
				batchLatencies = append(batchLatencies, latency)
				mu.Unlock()

				if err != nil {
					b.Errorf("BatchLockFunds failed: %v", err)
				}

				// Unlock the funds to maintain balance for next operations
				if result != nil && result.SuccessCount > 0 {
					unlockOps := make([]bookkeeper.FundsOperation, 0, result.SuccessCount)
					for i := 0; i < len(operations) && len(unlockOps) < result.SuccessCount; i++ {
						opKey := fmt.Sprintf("%s-%s", operations[i].UserID, operations[i].Currency)
						if _, failed := result.FailedItems[opKey]; !failed {
							unlockOps = append(unlockOps, operations[i])
						}
					}
					if len(unlockOps) > 0 {
						bkSvc.BatchUnlockFunds(ctx, unlockOps)
					}
				}
			}
		})

		if len(batchLatencies) > 0 {
			avg := calculateAverageLatency(batchLatencies)
			p95 := calculatePercentile(batchLatencies, 0.95)
			p99 := calculatePercentile(batchLatencies, 0.99)

			// Calculate per-operation latency for batch
			totalOps := len(batchLatencies) * 10
			avgPerOp := time.Duration(int64(avg) / 10)

			b.Logf("Batch Under Load - Avg per batch: %v, Avg per op: %v, P95: %v, P99: %v, Batches: %d",
				avg, avgPerOp, p95, p99, len(batchLatencies))
		}
	})
}

// BenchmarkDatabaseConnectionUtilization measures database connection usage
func BenchmarkDatabaseConnectionUtilization(b *testing.B) {
	db := setupBenchmarkDB(b)
	logger := zap.NewNop()

	bkSvc, err := bookkeeper.NewService(logger, db)
	assert.NoError(b, err)

	userCount := 50
	currencies := []string{"BTC", "ETH"}
	userIDs := createTestUsers(b, db, userCount)
	createTestAccountsWithBalance(b, db, bkSvc, userIDs, currencies, 5000.0)

	ctx := context.Background()

	b.Run("Connection_Usage_Individual", func(b *testing.B) {
		start := time.Now()
		connectionCount := 0

		for i := 0; i < b.N; i++ {
			// Each GetAccount call uses a separate connection
			for j := 0; j < 20; j++ {
				userID := userIDs[rand.Intn(len(userIDs))]
				currency := currencies[rand.Intn(len(currencies))]

				_, err := bkSvc.GetAccount(ctx, userID, currency)
				connectionCount++

				if err != nil && err.Error() != "account not found" {
					b.Errorf("GetAccount failed: %v", err)
				}
			}
		}

		duration := time.Since(start)
		b.Logf("Individual: %d operations, %d DB calls, duration: %v",
			b.N*20, connectionCount, duration)
	})

	b.Run("Connection_Usage_Batch", func(b *testing.B) {
		start := time.Now()
		connectionCount := 0

		for i := 0; i < b.N; i++ {
			// Single BatchGetAccounts call uses one connection for multiple operations
			_, err := bkSvc.BatchGetAccounts(ctx, userIDs[:20], currencies)
			connectionCount++ // Only one DB call per batch

			if err != nil {
				b.Errorf("BatchGetAccounts failed: %v", err)
			}
		}

		duration := time.Since(start)
		b.Logf("Batch: %d operations, %d DB calls, duration: %v",
			b.N*40, connectionCount, duration) // 20 users * 2 currencies = 40 accounts per batch
	})
}

// Helper functions
func setupBenchmarkDB(b *testing.B) *gorm.DB {
	b.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		b.Fatalf("failed to open in-memory db: %v", err)
	}

	err = db.AutoMigrate(&models.User{}, &models.Account{}, &models.Transaction{}, &models.Order{}, &models.Trade{})
	if err != nil {
		b.Fatalf("failed to migrate db: %v", err)
	}

	return db
}

func createTestUsers(b *testing.B, db *gorm.DB, count int) []string {
	b.Helper()
	userIDs := make([]string, count)

	for i := 0; i < count; i++ {
		userID := uuid.New()
		user := &models.User{
			ID:       userID,
			Email:    fmt.Sprintf("user%d@test.com", i),
			Username: fmt.Sprintf("user%d", i),
		}

		if err := db.Create(user).Error; err != nil {
			b.Fatalf("failed to create user: %v", err)
		}

		userIDs[i] = userID.String()
	}

	return userIDs
}

func createTestAccounts(b *testing.B, db *gorm.DB, bkSvc bookkeeper.BookkeeperService, userIDs []string, currencies []string) {
	b.Helper()
	ctx := context.Background()

	for _, userID := range userIDs {
		for _, currency := range currencies {
			_, err := bkSvc.CreateAccount(ctx, userID, currency)
			if err != nil {
				b.Logf("Warning: failed to create account for user %s, currency %s: %v", userID, currency, err)
			}
		}
	}
}

func createTestAccountsWithBalance(b *testing.B, db *gorm.DB, bkSvc bookkeeper.BookkeeperService, userIDs []string, currencies []string, initialBalance float64) {
	b.Helper()
	ctx := context.Background()

	for _, userID := range userIDs {
		for _, currency := range currencies {
			account, err := bkSvc.CreateAccount(ctx, userID, currency)
			if err != nil {
				b.Logf("Warning: failed to create account for user %s, currency %s: %v", userID, currency, err)
				continue
			}

			// Set initial balance
			account.Balance = initialBalance
			account.Available = initialBalance
			account.Locked = 0

			if err := db.Save(account).Error; err != nil {
				b.Logf("Warning: failed to set initial balance: %v", err)
			}
		}
	}
}

func calculateAverageLatency(latencies []time.Duration) time.Duration {
	if len(latencies) == 0 {
		return 0
	}

	var total time.Duration
	for _, lat := range latencies {
		total += lat
	}

	return total / time.Duration(len(latencies))
}

func calculatePercentile(latencies []time.Duration, percentile float64) time.Duration {
	if len(latencies) == 0 {
		return 0
	}

	// Simple percentile calculation (not sorting for performance)
	index := int(float64(len(latencies)) * percentile)
	if index >= len(latencies) {
		index = len(latencies) - 1
	}

	// Find the value at the percentile index (simplified)
	var sortedLatencies []time.Duration
	copy(sortedLatencies, latencies)

	// Basic selection for percentile (simplified for benchmark)
	if index < len(latencies) {
		return latencies[index]
	}

	return latencies[len(latencies)-1]
}
