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
	"github.com/Aidin1998/pincex_unified/internal/database"
	"github.com/Aidin1998/pincex_unified/pkg/models"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// BenchmarkWriteLatencyAnalysis measures the impact of batch operations on write performance
func BenchmarkWriteLatencyAnalysis(b *testing.B) {
	db := setupWriteLatencyDB(b)
	logger := zap.NewNop()

	bkSvc, err := bookkeeper.NewService(logger, db)
	assert.NoError(b, err)

	// Create test data
	userCount := 200
	currencies := []string{"BTC", "ETH", "USDT", "USD"}
	userIDs := createTestUsers(b, db, userCount)
	createTestAccountsWithBalance(b, db, bkSvc, userIDs, currencies, 50000.0)

	ctx := context.Background()

	b.Run("Write_Latency_Individual_Operations", func(b *testing.B) {
		analyzer := database.NewPerformanceAnalyzer(logger, db)
		analyzer.StartAnalysis()

		var latencies []time.Duration
		var mu sync.Mutex

		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				userID := userIDs[rand.Intn(len(userIDs))]
				currency := currencies[rand.Intn(len(currencies))]
				amount := float64(1 + rand.Intn(100))

				// Measure individual write latency
				start := time.Now()
				err := bkSvc.LockFunds(ctx, userID, currency, amount)
				writeLatency := time.Since(start)

				mu.Lock()
				latencies = append(latencies, writeLatency)
				mu.Unlock()

				analyzer.RecordOperation(writeLatency, true, err)

				if err != nil && err.Error() != "insufficient funds" {
					b.Errorf("LockFunds failed: %v", err)
				}

				// Unlock to maintain balance
				if err == nil {
					unlockStart := time.Now()
					unlockErr := bkSvc.UnlockFunds(ctx, userID, currency, amount)
					unlockLatency := time.Since(unlockStart)

					mu.Lock()
					latencies = append(latencies, unlockLatency)
					mu.Unlock()

					analyzer.RecordOperation(unlockLatency, true, unlockErr)
				}
			}
		})

		metrics := analyzer.FinalizeAnalysis()
		logWriteLatencyMetrics(b, "Individual Operations", latencies, metrics)
	})

	b.Run("Write_Latency_Batch_Operations", func(b *testing.B) {
		analyzer := database.NewPerformanceAnalyzer(logger, db)
		analyzer.StartAnalysis()

		var batchLatencies []time.Duration
		var individualLatencies []time.Duration
		var mu sync.Mutex

		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				// Create batch of operations
				batchSize := 20
				lockOps := make([]bookkeeper.FundsOperation, batchSize)
				unlockOps := make([]bookkeeper.FundsOperation, batchSize)

				for i := 0; i < batchSize; i++ {
					userID := userIDs[rand.Intn(len(userIDs))]
					currency := currencies[rand.Intn(len(currencies))]
					amount := float64(1 + rand.Intn(50))

					op := bookkeeper.FundsOperation{
						UserID:   userID,
						Currency: currency,
						Amount:   amount,
						OrderID:  fmt.Sprintf("batch_order_%d", rand.Int()),
						Reason:   "latency_test",
					}

					lockOps[i] = op
					unlockOps[i] = op
				}

				// Measure batch lock latency
				batchStart := time.Now()
				lockResult, lockErr := bkSvc.BatchLockFunds(ctx, lockOps)
				batchLockLatency := time.Since(batchStart)

				mu.Lock()
				batchLatencies = append(batchLatencies, batchLockLatency)
				// Calculate per-operation latency for batch
				if lockResult != nil && lockResult.SuccessCount > 0 {
					perOpLatency := batchLockLatency / time.Duration(lockResult.SuccessCount)
					for i := 0; i < lockResult.SuccessCount; i++ {
						individualLatencies = append(individualLatencies, perOpLatency)
					}
				}
				mu.Unlock()

				analyzer.RecordOperation(batchLockLatency, true, lockErr)
				analyzer.RecordConnectionUsage(1) // Batch uses single connection

				// Measure batch unlock latency
				if lockResult != nil && lockResult.SuccessCount > 0 {
					// Only unlock successful operations
					successfulUnlocks := unlockOps[:lockResult.SuccessCount]

					batchUnlockStart := time.Now()
					unlockResult, unlockErr := bkSvc.BatchUnlockFunds(ctx, successfulUnlocks)
					batchUnlockLatency := time.Since(batchUnlockStart)

					mu.Lock()
					batchLatencies = append(batchLatencies, batchUnlockLatency)
					mu.Unlock()

					analyzer.RecordOperation(batchUnlockLatency, true, unlockErr)

					if unlockResult != nil {
						b.Logf("Batch unlock: %d successful, %d failed",
							unlockResult.SuccessCount, len(unlockResult.FailedItems))
					}
				}
			}
		})

		metrics := analyzer.FinalizeAnalysis()
		logWriteLatencyMetrics(b, "Batch Operations", individualLatencies, metrics)
		logBatchLatencyMetrics(b, "Batch Operations (per batch)", batchLatencies)
	})
}

// BenchmarkTransactionDurationAnalysis measures transaction duration impact
func BenchmarkTransactionDurationAnalysis(b *testing.B) {
	db := setupWriteLatencyDB(b)
	logger := zap.NewNop()

	bkSvc, err := bookkeeper.NewService(logger, db)
	assert.NoError(b, err)

	userCount := 100
	currencies := []string{"BTC", "ETH", "USDT"}
	userIDs := createTestUsers(b, db, userCount)
	createTestAccountsWithBalance(b, db, bkSvc, userIDs, currencies, 10000.0)

	ctx := context.Background()

	b.Run("Transaction_Duration_Individual", func(b *testing.B) {
		var transactionDurations []time.Duration
		var mu sync.Mutex

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			userID := userIDs[rand.Intn(len(userIDs))]
			currency := currencies[rand.Intn(len(currencies))]
			amount := float64(1 + rand.Intn(50))

			// Start measuring from transaction begin to commit
			txStart := time.Now()

			// Individual operation creates its own transaction
			err := bkSvc.LockFunds(ctx, userID, currency, amount)

			txDuration := time.Since(txStart)
			mu.Lock()
			transactionDurations = append(transactionDurations, txDuration)
			mu.Unlock()

			if err != nil && err.Error() != "insufficient funds" {
				b.Errorf("LockFunds failed: %v", err)
			}
		}

		logTransactionDurationMetrics(b, "Individual Transactions", transactionDurations)
	})

	b.Run("Transaction_Duration_Batch", func(b *testing.B) {
		var transactionDurations []time.Duration
		var mu sync.Mutex

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			batchSize := 25
			operations := make([]bookkeeper.FundsOperation, batchSize)

			for j := 0; j < batchSize; j++ {
				operations[j] = bookkeeper.FundsOperation{
					UserID:   userIDs[rand.Intn(len(userIDs))],
					Currency: currencies[rand.Intn(len(currencies))],
					Amount:   float64(1 + rand.Intn(30)),
					OrderID:  fmt.Sprintf("tx_test_%d_%d", i, j),
					Reason:   "transaction_duration_test",
				}
			}

			// Measure batch transaction duration
			txStart := time.Now()
			result, err := bkSvc.BatchLockFunds(ctx, operations)
			txDuration := time.Since(txStart)

			mu.Lock()
			transactionDurations = append(transactionDurations, txDuration)
			mu.Unlock()

			if err != nil {
				b.Errorf("BatchLockFunds failed: %v", err)
			}

			if result != nil {
				b.Logf("Batch %d: %d successful operations in %v",
					i, result.SuccessCount, txDuration)
			}
		}

		logTransactionDurationMetrics(b, "Batch Transactions", transactionDurations)
	})
}

// BenchmarkLockContentionAnalysis measures lock contention under concurrent access
func BenchmarkLockContentionAnalysis(b *testing.B) {
	db := setupWriteLatencyDB(b)
	logger := zap.NewNop()

	bkSvc, err := bookkeeper.NewService(logger, db)
	assert.NoError(b, err)

	// Create focused test data for contention testing
	userCount := 50 // Smaller user base for higher contention
	currencies := []string{"BTC", "ETH"}
	userIDs := createTestUsers(b, db, userCount)
	createTestAccountsWithBalance(b, db, bkSvc, userIDs, currencies, 100000.0)

	ctx := context.Background()

	b.Run("Lock_Contention_Individual_High_Concurrency", func(b *testing.B) {
		var contentionMetrics []ContentionMetric
		var mu sync.Mutex

		concurrency := 20 // High concurrency level

		b.ResetTimer()
		b.SetParallelism(concurrency)
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				// Target same accounts for higher contention
				userID := userIDs[rand.Intn(10)] // Focus on first 10 users
				currency := currencies[rand.Intn(len(currencies))]
				amount := float64(1 + rand.Intn(10))

				start := time.Now()
				waitStart := time.Now()

				err := bkSvc.LockFunds(ctx, userID, currency, amount)
				operationLatency := time.Since(start)

				metric := ContentionMetric{
					WaitTime:         time.Since(waitStart),
					OperationLatency: operationLatency,
					Success:          err == nil,
					Contention:       operationLatency > 10*time.Millisecond, // Threshold for contention
				}

				mu.Lock()
				contentionMetrics = append(contentionMetrics, metric)
				mu.Unlock()

				if err == nil {
					// Unlock after short delay to create contention
					time.Sleep(1 * time.Millisecond)
					bkSvc.UnlockFunds(ctx, userID, currency, amount)
				}
			}
		})

		logContentionMetrics(b, "Individual High Concurrency", contentionMetrics)
	})

	b.Run("Lock_Contention_Batch_Operations", func(b *testing.B) {
		var contentionMetrics []ContentionMetric
		var mu sync.Mutex

		concurrency := 20

		b.ResetTimer()
		b.SetParallelism(concurrency)
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				batchSize := 15
				operations := make([]bookkeeper.FundsOperation, batchSize)

				// Create batch targeting overlapping accounts for contention
				for i := 0; i < batchSize; i++ {
					operations[i] = bookkeeper.FundsOperation{
						UserID:   userIDs[rand.Intn(15)], // Overlap accounts
						Currency: currencies[rand.Intn(len(currencies))],
						Amount:   float64(1 + rand.Intn(5)),
						OrderID:  fmt.Sprintf("contention_order_%d", rand.Int()),
						Reason:   "contention_test",
					}
				}

				start := time.Now()
				waitStart := time.Now()

				result, err := bkSvc.BatchLockFunds(ctx, operations)
				operationLatency := time.Since(start)

				metric := ContentionMetric{
					WaitTime:         time.Since(waitStart),
					OperationLatency: operationLatency,
					Success:          err == nil && result != nil && result.SuccessCount > 0,
					Contention:       operationLatency > 50*time.Millisecond, // Higher threshold for batch
				}

				mu.Lock()
				contentionMetrics = append(contentionMetrics, metric)
				mu.Unlock()

				// Unlock successful operations
				if result != nil && result.SuccessCount > 0 {
					time.Sleep(2 * time.Millisecond)
					successfulOps := operations[:result.SuccessCount]
					bkSvc.BatchUnlockFunds(ctx, successfulOps)
				}
			}
		})

		logContentionMetrics(b, "Batch Operations", contentionMetrics)
	})
}

// Supporting types and helper functions
type ContentionMetric struct {
	WaitTime         time.Duration
	OperationLatency time.Duration
	Success          bool
	Contention       bool
}

func setupWriteLatencyDB(b *testing.B) *gorm.DB {
	b.Helper()

	// Use settings optimized for latency testing
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		// Disable foreign key constraints for faster inserts
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		b.Fatalf("failed to open in-memory db: %v", err)
	}

	// Set SQLite pragmas for performance
	db.Exec("PRAGMA journal_mode=WAL")
	db.Exec("PRAGMA synchronous=NORMAL")
	db.Exec("PRAGMA cache_size=10000")
	db.Exec("PRAGMA temp_store=memory")

	err = db.AutoMigrate(&models.User{}, &models.Account{}, &models.Transaction{}, &models.Order{}, &models.Trade{})
	if err != nil {
		b.Fatalf("failed to migrate db: %v", err)
	}

	return db
}

func logWriteLatencyMetrics(b *testing.B, testName string, latencies []time.Duration, metrics *database.PerformanceMetrics) {
	if len(latencies) == 0 {
		return
	}

	var totalLatency time.Duration
	minLatency := latencies[0]
	maxLatency := latencies[0]

	for _, lat := range latencies {
		totalLatency += lat
		if lat < minLatency {
			minLatency = lat
		}
		if lat > maxLatency {
			maxLatency = lat
		}
	}

	avgLatency := totalLatency / time.Duration(len(latencies))

	b.Logf("%s Write Latency Metrics:", testName)
	b.Logf("  Operations: %d", len(latencies))
	b.Logf("  Min Latency: %v", minLatency)
	b.Logf("  Max Latency: %v", maxLatency)
	b.Logf("  Avg Latency: %v", avgLatency)
	b.Logf("  P95 Latency: %v", metrics.P95Latency)
	b.Logf("  P99 Latency: %v", metrics.P99Latency)
	b.Logf("  Throughput: %.2f ops/sec", metrics.ThroughputOpsPerSec)
	b.Logf("  Error Count: %d", metrics.ErrorCount)
	b.Logf("  Transaction Count: %d", metrics.TransactionCount)
}

func logBatchLatencyMetrics(b *testing.B, testName string, batchLatencies []time.Duration) {
	if len(batchLatencies) == 0 {
		return
	}

	var totalLatency time.Duration
	for _, lat := range batchLatencies {
		totalLatency += lat
	}

	avgBatchLatency := totalLatency / time.Duration(len(batchLatencies))

	b.Logf("%s Batch Metrics:", testName)
	b.Logf("  Batch Count: %d", len(batchLatencies))
	b.Logf("  Avg Batch Latency: %v", avgBatchLatency)
	b.Logf("  Total Batch Time: %v", totalLatency)
}

func logTransactionDurationMetrics(b *testing.B, testName string, durations []time.Duration) {
	if len(durations) == 0 {
		return
	}

	var totalDuration time.Duration
	minDuration := durations[0]
	maxDuration := durations[0]

	for _, dur := range durations {
		totalDuration += dur
		if dur < minDuration {
			minDuration = dur
		}
		if dur > maxDuration {
			maxDuration = dur
		}
	}

	avgDuration := totalDuration / time.Duration(len(durations))

	b.Logf("%s Transaction Duration Metrics:", testName)
	b.Logf("  Transaction Count: %d", len(durations))
	b.Logf("  Min Duration: %v", minDuration)
	b.Logf("  Max Duration: %v", maxDuration)
	b.Logf("  Avg Duration: %v", avgDuration)
}

func logContentionMetrics(b *testing.B, testName string, metrics []ContentionMetric) {
	if len(metrics) == 0 {
		return
	}

	var totalWaitTime, totalOpLatency time.Duration
	successCount := 0
	contentionCount := 0

	for _, metric := range metrics {
		totalWaitTime += metric.WaitTime
		totalOpLatency += metric.OperationLatency
		if metric.Success {
			successCount++
		}
		if metric.Contention {
			contentionCount++
		}
	}

	avgWaitTime := totalWaitTime / time.Duration(len(metrics))
	avgOpLatency := totalOpLatency / time.Duration(len(metrics))
	successRate := float64(successCount) / float64(len(metrics)) * 100
	contentionRate := float64(contentionCount) / float64(len(metrics)) * 100

	b.Logf("%s Contention Metrics:", testName)
	b.Logf("  Total Operations: %d", len(metrics))
	b.Logf("  Success Rate: %.2f%%", successRate)
	b.Logf("  Contention Rate: %.2f%%", contentionRate)
	b.Logf("  Avg Wait Time: %v", avgWaitTime)
	b.Logf("  Avg Operation Latency: %v", avgOpLatency)
}
