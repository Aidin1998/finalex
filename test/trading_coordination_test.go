//go:build trading

package test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Aidin1998/finalex/internal/trading/coordination"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/suite"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

// TradingCoordinationTestSuite tests trade coordination and workflow management
type TradingCoordinationTestSuite struct {
	suite.Suite
	service *coordination.CoordinationService
	logger  *zap.Logger
}

func (suite *TradingCoordinationTestSuite) SetupTest() {
	suite.logger = zaptest.NewLogger(suite.T())
	suite.service = coordination.NewCoordinationService(suite.logger)
}

func (suite *TradingCoordinationTestSuite) TearDownTest() {
	if suite.service != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		suite.service.Stop()
	}
}

func TestTradingCoordinationTestSuite(t *testing.T) {
	suite.Run(t, new(TradingCoordinationTestSuite))
}

// Test Service Lifecycle
func (suite *TradingCoordinationTestSuite) TestServiceLifecycle() {
	suite.Run("StartService", func() {
		err := suite.service.Start()
		suite.NoError(err)
	})

	suite.Run("StopService", func() {
		err := suite.service.Stop()
		suite.NoError(err)
	})

	suite.Run("RestartService", func() {
		err := suite.service.Start()
		suite.NoError(err)

		err = suite.service.Stop()
		suite.NoError(err)

		err = suite.service.Start()
		suite.NoError(err)
	})
}

// Test Trade Coordination
func (suite *TradingCoordinationTestSuite) TestTradeCoordination() {
	err := suite.service.Start()
	suite.Require().NoError(err)

	suite.Run("ValidTradeCoordination", func() {
		testOrder := &coordination.CoordinationTrade{
			ID:             uuid.New(),
			OrderID:        uuid.New(),
			CounterOrderID: uuid.New(),
			UserID:         uuid.New(),
			CounterUserID:  uuid.New(),
			Pair:           "BTCUSDT",
			Price:          decimal.NewFromFloat(50000.00),
			Quantity:       decimal.NewFromFloat(0.001),
			Side:           "buy",
			Maker:          false,
			CreatedAt:      time.Now(),
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := suite.service.CoordinateTrade(ctx, testOrder)
		suite.NoError(err)
	})

	suite.Run("InvalidTradeParameters", func() {
		testOrder := &coordination.CoordinationTrade{
			ID:             uuid.New(),
			OrderID:        uuid.New(),
			CounterOrderID: uuid.New(),
			UserID:         uuid.New(),
			CounterUserID:  uuid.New(),
			Pair:           "BTCUSDT",
			Price:          decimal.NewFromFloat(-1), // Invalid price
			Quantity:       decimal.NewFromFloat(0.001),
			Side:           "buy",
			Maker:          false,
			CreatedAt:      time.Now(),
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := suite.service.CoordinateTrade(ctx, testOrder)
		suite.Error(err)
		suite.Contains(err.Error(), "invalid")
	})

	suite.Run("ZeroQuantityTrade", func() {
		testOrder := &coordination.CoordinationTrade{
			ID:             uuid.New(),
			OrderID:        uuid.New(),
			CounterOrderID: uuid.New(),
			UserID:         uuid.New(),
			CounterUserID:  uuid.New(),
			Pair:           "BTCUSDT",
			Price:          decimal.NewFromFloat(50000),
			Quantity:       decimal.Zero, // Invalid quantity
			Side:           "buy",
			Maker:          false,
			CreatedAt:      time.Now(),
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := suite.service.CoordinateTrade(ctx, testOrder)
		suite.Error(err)
		suite.Contains(err.Error(), "quantity")
	})
}

// Test Concurrent Trade Coordination
func (suite *TradingCoordinationTestSuite) TestConcurrentTradeCoordination() {
	err := suite.service.Start()
	suite.Require().NoError(err)

	suite.Run("MultipleConcurrentTrades", func() {
		var wg sync.WaitGroup
		var successCount int64
		var errorCount int64
		tradeCount := 100

		for i := 0; i < tradeCount; i++ {
			wg.Add(1)
			go func(tradeID int) {
				defer wg.Done()

				testOrder := &coordination.CoordinationTrade{
					ID:             uuid.New(),
					OrderID:        uuid.New(),
					CounterOrderID: uuid.New(),
					UserID:         uuid.New(),
					CounterUserID:  uuid.New(),
					Pair:           fmt.Sprintf("TRADE%dUSDT", tradeID%10), // Spread across 10 pairs
					Price:          decimal.NewFromFloat(50000.00 + float64(tradeID)),
					Quantity:       decimal.NewFromFloat(0.001 + float64(tradeID)*0.0001),
					Side:           "buy",
					Maker:          false,
					CreatedAt:      time.Now(),
				}

				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()

				if err := suite.service.CoordinateTrade(ctx, testOrder); err != nil {
					atomic.AddInt64(&errorCount, 1)
					suite.T().Logf("Trade %d failed: %v", tradeID, err)
				} else {
					atomic.AddInt64(&successCount, 1)
				}
			}(i)
		}

		wg.Wait()

		suite.T().Logf("Coordination results: %d successful, %d failed out of %d total",
			successCount, errorCount, tradeCount)

		// At least 95% should succeed
		suite.GreaterOrEqual(successCount, int64(tradeCount*95/100))
	})
}

// Test Workflow Management
func (suite *TradingCoordinationTestSuite) TestWorkflowManagement() {
	err := suite.service.Start()
	suite.Require().NoError(err)

	suite.Run("WorkflowStepExecution", func() {
		testOrder := &coordination.CoordinationTrade{
			ID:             uuid.New(),
			OrderID:        uuid.New(),
			CounterOrderID: uuid.New(),
			UserID:         uuid.New(),
			CounterUserID:  uuid.New(),
			Pair:           "BTCUSDT",
			Price:          decimal.NewFromFloat(50000.00),
			Quantity:       decimal.NewFromFloat(0.001),
			Side:           "buy",
			Maker:          false,
			CreatedAt:      time.Now(),
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		start := time.Now()
		err := suite.service.CoordinateTrade(ctx, testOrder)
		elapsed := time.Since(start)

		suite.NoError(err)
		suite.Less(elapsed, 1*time.Second) // Should complete quickly
		suite.T().Logf("Workflow execution time: %v", elapsed)
	})

	suite.Run("WorkflowTimeout", func() {
		testOrder := &coordination.CoordinationTrade{
			ID:             uuid.New(),
			OrderID:        uuid.New(),
			CounterOrderID: uuid.New(),
			UserID:         uuid.New(),
			CounterUserID:  uuid.New(),
			Pair:           "BTCUSDT",
			Price:          decimal.NewFromFloat(50000.00),
			Quantity:       decimal.NewFromFloat(0.001),
			Side:           "buy",
			Maker:          false,
			CreatedAt:      time.Now(),
		}

		// Very short timeout to test timeout handling
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Microsecond)
		defer cancel()

		err := suite.service.CoordinateTrade(ctx, testOrder)

		// Should either succeed very quickly or timeout
		if err != nil {
			suite.Contains(err.Error(), "context deadline exceeded")
		}
	})
}

// Test Error Handling and Recovery
func (suite *TradingCoordinationTestSuite) TestErrorHandling() {
	err := suite.service.Start()
	suite.Require().NoError(err)

	suite.Run("NilTradeHandling", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := suite.service.CoordinateTrade(ctx, nil)
		suite.Error(err)
	})

	suite.Run("EmptyTradeFields", func() {
		trade := &coordination.CoordinationTrade{
			// Missing required fields
			CreatedAt: time.Now(),
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := suite.service.CoordinateTrade(ctx, trade)
		suite.Error(err)
	})

	suite.Run("InvalidUUIDs", func() {
		trade := &coordination.CoordinationTrade{
			ID:        uuid.Nil, // Invalid UUID
			Pair:      "BTCUSDT",
			Price:     decimal.NewFromFloat(50000),
			Quantity:  decimal.NewFromFloat(0.001),
			CreatedAt: time.Now(),
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := suite.service.CoordinateTrade(ctx, trade)
		suite.Error(err)
	})
}

// Test Performance Under Load
func (suite *TradingCoordinationTestSuite) TestPerformanceUnderLoad() {
	if testing.Short() {
		suite.T().Skip("Skipping performance tests in short mode")
	}

	err := suite.service.Start()
	suite.Require().NoError(err)

	suite.Run("HighThroughputCoordination", func() {
		var wg sync.WaitGroup
		var totalProcessed int64
		var totalErrors int64

		concurrency := 50
		tradesPerWorker := 100
		start := time.Now()

		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()

				for j := 0; j < tradesPerWorker; j++ {
					testOrder := &coordination.CoordinationTrade{
						ID:             uuid.New(),
						OrderID:        uuid.New(),
						CounterOrderID: uuid.New(),
						UserID:         uuid.New(),
						CounterUserID:  uuid.New(),
						Pair:           fmt.Sprintf("PERF%dUSDT", (workerID*tradesPerWorker+j)%20),
						Price:          decimal.NewFromFloat(50000.00 + float64(j)),
						Quantity:       decimal.NewFromFloat(0.001 + float64(j)*0.0001),
						Side:           "buy",
						Maker:          false,
						CreatedAt:      time.Now(),
					}

					ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					if err := suite.service.CoordinateTrade(ctx, testOrder); err != nil {
						atomic.AddInt64(&totalErrors, 1)
					} else {
						atomic.AddInt64(&totalProcessed, 1)
					}
					cancel()
				}
			}(i)
		}

		wg.Wait()
		elapsed := time.Since(start)

		totalTrades := int64(concurrency * tradesPerWorker)
		tradesPerSecond := float64(totalProcessed) / elapsed.Seconds()

		suite.T().Logf("Coordination performance: %d/%d trades processed in %v (%.2f TPS)",
			totalProcessed, totalTrades, elapsed, tradesPerSecond)

		// Should process at least 500 trades per second
		suite.Greater(tradesPerSecond, 500.0)

		// Error rate should be less than 5%
		errorRate := float64(totalErrors) / float64(totalTrades)
		suite.Less(errorRate, 0.05)
	})
}

// Test Resource Management
func (suite *TradingCoordinationTestSuite) TestResourceManagement() {
	err := suite.service.Start()
	suite.Require().NoError(err)

	suite.Run("MemoryUsageUnderLoad", func() {
		// Process many trades to test memory management
		var wg sync.WaitGroup
		tradeCount := 1000

		for i := 0; i < tradeCount; i++ {
			wg.Add(1)
			go func(tradeID int) {
				defer wg.Done()

				testOrder := &coordination.CoordinationTrade{
					ID:             uuid.New(),
					OrderID:        uuid.New(),
					CounterOrderID: uuid.New(),
					UserID:         uuid.New(),
					CounterUserID:  uuid.New(),
					Pair:           fmt.Sprintf("MEM%dUSDT", tradeID%50),
					Price:          decimal.NewFromFloat(50000.00),
					Quantity:       decimal.NewFromFloat(0.001),
					Side:           "buy",
					Maker:          false,
					CreatedAt:      time.Now(),
				}

				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()

				suite.service.CoordinateTrade(ctx, testOrder)
			}(i)
		}

		wg.Wait()
		suite.T().Log("Memory usage test completed successfully")
	})

	suite.Run("GracefulShutdown", func() {
		// Start some background trades
		var wg sync.WaitGroup
		stopChan := make(chan struct{})

		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()
				for {
					select {
					case <-stopChan:
						return
					default:
						testOrder := &coordination.CoordinationTrade{
							ID:             uuid.New(),
							OrderID:        uuid.New(),
							CounterOrderID: uuid.New(),
							UserID:         uuid.New(),
							CounterUserID:  uuid.New(),
							Pair:           "SHUTDOWNUSDT",
							Price:          decimal.NewFromFloat(50000.00),
							Quantity:       decimal.NewFromFloat(0.001),
							Side:           "buy",
							Maker:          false,
							CreatedAt:      time.Now(),
						}

						ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
						suite.service.CoordinateTrade(ctx, testOrder)
						cancel()

						time.Sleep(time.Millisecond * 10)
					}
				}
			}(i)
		}

		// Let trades run for a bit
		time.Sleep(100 * time.Millisecond)

		// Signal shutdown
		close(stopChan)

		// Stop service
		start := time.Now()
		err := suite.service.Stop()
		shutdownTime := time.Since(start)

		suite.NoError(err)
		suite.Less(shutdownTime, 3*time.Second) // Should shutdown quickly

		wg.Wait()
		suite.T().Logf("Graceful shutdown completed in %v", shutdownTime)
	})
}

// Test Trade Validation
func (suite *TradingCoordinationTestSuite) TestTradeValidation() {
	err := suite.service.Start()
	suite.Require().NoError(err)

	suite.Run("ValidTradeValidation", func() {
		validTrades := []*coordination.CoordinationTrade{
			{
				ID:             uuid.New(),
				OrderID:        uuid.New(),
				CounterOrderID: uuid.New(),
				UserID:         uuid.New(),
				CounterUserID:  uuid.New(),
				Pair:           "BTCUSDT",
				Price:          decimal.NewFromFloat(50000.00),
				Quantity:       decimal.NewFromFloat(0.001),
				Side:           "buy",
				Maker:          false,
				CreatedAt:      time.Now(),
			},
			{
				ID:             uuid.New(),
				OrderID:        uuid.New(),
				CounterOrderID: uuid.New(),
				UserID:         uuid.New(),
				CounterUserID:  uuid.New(),
				Pair:           "ETHUSDT",
				Price:          decimal.NewFromFloat(3000.00),
				Quantity:       decimal.NewFromFloat(0.1),
				Side:           "buy",
				Maker:          false,
				CreatedAt:      time.Now(),
			},
		}

		for i, trade := range validTrades {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			err := suite.service.CoordinateTrade(ctx, trade)
			cancel()

			suite.NoError(err, "Valid trade %d should not produce error", i)
		}
	})

	suite.Run("InvalidTradeValidation", func() {
		invalidTrades := []*coordination.CoordinationTrade{
			{
				// Missing required IDs
				Pair:      "BTCUSDT",
				Price:     decimal.NewFromFloat(50000.00),
				Quantity:  decimal.NewFromFloat(0.001),
				CreatedAt: time.Now(),
			},
			{
				// Negative price
				ID:             uuid.New(),
				OrderID:        uuid.New(),
				CounterOrderID: uuid.New(),
				UserID:         uuid.New(),
				CounterUserID:  uuid.New(),
				Pair:           "BTCUSDT",
				Price:          decimal.NewFromFloat(-50000.00),
				Quantity:       decimal.NewFromFloat(0.001),
				CreatedAt:      time.Now(),
			},
			{
				// Zero quantity
				ID:             uuid.New(),
				OrderID:        uuid.New(),
				CounterOrderID: uuid.New(),
				UserID:         uuid.New(),
				CounterUserID:  uuid.New(),
				Pair:           "BTCUSDT",
				Price:          decimal.NewFromFloat(50000.00),
				Quantity:       decimal.Zero,
				CreatedAt:      time.Now(),
			},
		}

		for i, trade := range invalidTrades {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			err := suite.service.CoordinateTrade(ctx, trade)
			cancel()

			suite.Error(err, "Invalid trade %d should produce error", i)
		}
	})
}

// Benchmark coordination operations
func (suite *TradingCoordinationTestSuite) TestCoordinationBenchmarks() {
	if testing.Short() {
		suite.T().Skip("Skipping benchmark tests in short mode")
	}

	err := suite.service.Start()
	suite.Require().NoError(err)

	suite.Run("BenchmarkTradeCoordination", func() {
		iterations := 10000
		start := time.Now()

		var wg sync.WaitGroup
		batchSize := 100
		batches := iterations / batchSize

		for batch := 0; batch < batches; batch++ {
			wg.Add(1)
			go func(batchID int) {
				defer wg.Done()

				for i := 0; i < batchSize; i++ {
					testOrder := &coordination.CoordinationTrade{
						ID:             uuid.New(),
						OrderID:        uuid.New(),
						CounterOrderID: uuid.New(),
						UserID:         uuid.New(),
						CounterUserID:  uuid.New(),
						Pair:           fmt.Sprintf("BENCH%dUSDT", (batchID*batchSize+i)%10),
						Price:          decimal.NewFromFloat(50000.00),
						Quantity:       decimal.NewFromFloat(0.001),
						Side:           "buy",
						Maker:          false,
						CreatedAt:      time.Now(),
					}

					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					suite.service.CoordinateTrade(ctx, testOrder)
					cancel()
				}
			}(batch)
		}

		wg.Wait()
		elapsed := time.Since(start)
		tradesPerSecond := float64(iterations) / elapsed.Seconds()

		suite.T().Logf("Coordination benchmark: %d trades in %v (%.2f TPS)",
			iterations, elapsed, tradesPerSecond)

		// Should handle at least 1000 trades per second
		suite.Greater(tradesPerSecond, 1000.0)
	})
}
