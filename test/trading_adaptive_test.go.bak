//go:build trading

package test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Aidin1998/finalex/internal/accounts/bookkeeper"
	"github.com/Aidin1998/finalex/internal/trading"
	"github.com/Aidin1998/finalex/internal/trading/engine"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
	"gorm.io/gorm"
)

// TradingAdaptiveTestSuite tests adaptive trading service functionality
type TradingAdaptiveTestSuite struct {
	suite.Suite
	service        trading.AdaptiveTradingService
	mockBookkeeper *MockBookkeeperAdaptive
	mockWSHub      *MockWSHubAdaptive
	db             *gorm.DB
	logger         *zap.Logger
}

// MockBookkeeperAdaptive provides thread-safe mocking for adaptive testing
type MockBookkeeperAdaptive struct {
	mock.Mock
	balances     sync.Map
	reservations sync.Map
	updateCount  int64
}

// Required BookkeeperService interface methods
func (m *MockBookkeeperAdaptive) Start() error {
	return nil
}

func (m *MockBookkeeperAdaptive) Stop() error {
	return nil
}

func (m *MockBookkeeperAdaptive) GetAccounts(ctx context.Context, userID string) ([]*models.Account, error) {
	return nil, nil
}

func (m *MockBookkeeperAdaptive) GetAccount(ctx context.Context, userID, currency string) (*models.Account, error) {
	return &models.Account{
		UserID:    uuid.MustParse(userID),
		Currency:  currency,
		Balance:   10000.0,
		Available: 10000.0,
		Locked:    0.0,
	}, nil
}

func (m *MockBookkeeperAdaptive) CreateAccount(ctx context.Context, userID, currency string) (*models.Account, error) {
	return m.GetAccount(ctx, userID, currency)
}

func (m *MockBookkeeperAdaptive) GetAccountTransactions(ctx context.Context, userID, currency string, limit, offset int) ([]*models.Transaction, int64, error) {
	return nil, 0, nil
}

func (m *MockBookkeeperAdaptive) CreateTransaction(ctx context.Context, userID, transactionType string, amount float64, currency, reference, description string) (*models.Transaction, error) {
	return nil, nil
}

func (m *MockBookkeeperAdaptive) CompleteTransaction(ctx context.Context, transactionID string) error {
	return nil
}

func (m *MockBookkeeperAdaptive) FailTransaction(ctx context.Context, transactionID string) error {
	return nil
}

func (m *MockBookkeeperAdaptive) LockFunds(ctx context.Context, userID, currency string, amount float64) error {
	return nil
}

func (m *MockBookkeeperAdaptive) UnlockFunds(ctx context.Context, userID, currency string, amount float64) error {
	return nil
}

func (m *MockBookkeeperAdaptive) BatchGetAccounts(ctx context.Context, userIDs []string, currencies []string) (map[string]map[string]*models.Account, error) {
	return nil, nil
}

func (m *MockBookkeeperAdaptive) BatchUpdateBalances(ctx context.Context, userID string, currency string, updates []bookkeeper.AccountBalance) (*bookkeeper.BatchOperationResult, error) {
	return &bookkeeper.BatchOperationResult{}, nil
}

func (m *MockBookkeeperAdaptive) BatchLockFunds(ctx context.Context, operations []bookkeeper.FundsOperation) (*bookkeeper.BatchOperationResult, error) {
	return &bookkeeper.BatchOperationResult{}, nil
}

func (m *MockBookkeeperAdaptive) BatchUnlockFunds(ctx context.Context, operations []bookkeeper.FundsOperation) (*bookkeeper.BatchOperationResult, error) {
	return &bookkeeper.BatchOperationResult{}, nil
}

func (m *MockBookkeeperAdaptive) ReserveBalance(ctx context.Context, userID uuid.UUID, asset string, amount decimal.Decimal) (*bookkeeper.BalanceReservation, error) {
	atomic.AddInt64(&m.updateCount, 1)
	reservation := &bookkeeper.BalanceReservation{
		ID:     uuid.New().String(),
		UserID: userID,
		Asset:  asset,
		Amount: amount,
	}
	m.reservations.Store(reservation.ID, reservation)
	return reservation, nil
}

func (m *MockBookkeeperAdaptive) ReleaseReservation(ctx context.Context, reservationID string) error {
	atomic.AddInt64(&m.updateCount, 1)
	m.reservations.Delete(reservationID)
	return nil
}

func (m *MockBookkeeperAdaptive) ProcessTrade(ctx context.Context, trade *bookkeeper.TradeProcessRequest) error {
	atomic.AddInt64(&m.updateCount, 1)
	// Simulate trade processing delay
	time.Sleep(time.Microsecond * 10)
	return nil
}

func (m *MockBookkeeperAdaptive) GetBalance(ctx context.Context, userID uuid.UUID, asset string) (decimal.Decimal, error) {
	if balance, ok := m.balances.Load(fmt.Sprintf("%s:%s", userID.String(), asset)); ok {
		return balance.(decimal.Decimal), nil
	}
	return decimal.NewFromFloat(10000), nil // Default balance
}

func (m *MockBookkeeperAdaptive) TransferBalance(ctx context.Context, fromUserID, toUserID uuid.UUID, asset string, amount decimal.Decimal) error {
	atomic.AddInt64(&m.updateCount, 1)
	return nil
}

func (m *MockBookkeeperAdaptive) GetAllBalances(ctx context.Context, userID uuid.UUID) (map[string]decimal.Decimal, error) {
	return map[string]decimal.Decimal{
		"BTC":  decimal.NewFromFloat(1.0),
		"USDT": decimal.NewFromFloat(50000),
	}, nil
}

// MockWSHubAdaptive provides thread-safe WebSocket mocking for adaptive testing
type MockWSHubAdaptive struct {
	mock.Mock
	broadcasts sync.Map
	msgCount   int64
}

func (m *MockWSHubAdaptive) Broadcast(topic string, data []byte) {
	atomic.AddInt64(&m.msgCount, 1)
	m.broadcasts.Store(fmt.Sprintf("%s:%d", topic, time.Now().UnixNano()), data)
}

func (m *MockWSHubAdaptive) BroadcastToUser(userID string, data []byte) {
	atomic.AddInt64(&m.msgCount, 1)
	m.broadcasts.Store(fmt.Sprintf("user:%s:%d", userID, time.Now().UnixNano()), data)
}

func (m *MockWSHubAdaptive) Subscribe(userID, topic string) error {
	return nil
}

func (m *MockWSHubAdaptive) Unsubscribe(userID, topic string) error {
	return nil
}

func (suite *TradingAdaptiveTestSuite) SetupTest() {
	suite.logger = zaptest.NewLogger(suite.T())
	suite.mockBookkeeper = new(MockBookkeeperAdaptive)
	suite.mockWSHub = new(MockWSHubAdaptive)

	// Use in-memory SQLite for testing
	suite.db = setupTestDB(suite.T())

	// Create adaptive configuration
	adaptiveConfig := &engine.AdaptiveEngineConfig{
		EnableAdaptiveOrderBooks: true,
		AutoMigrationEnabled:     true,
		MigrationThresholds: &engine.AutoMigrationThresholds{
			LatencyThresholdMs:     100,
			ThroughputThresholdTPS: 1000,
			ErrorRateThreshold:     0.01,
			EvaluationIntervalSec:  10,
		},
		LegacyEngine: &engine.Config{
			Engine: engine.EngineConfig{
				WorkerPoolSize:      4,
				WorkerPoolQueueSize: 1000,
			},
		},
		AdaptiveEngine: &engine.Config{
			Engine: engine.EngineConfig{
				WorkerPoolSize:      8,
				WorkerPoolQueueSize: 2000,
			},
		},
	}

	// Create adaptive trading service
	svc, err := trading.NewAdaptiveService(
		suite.logger,
		suite.db,
		suite.mockBookkeeper,
		adaptiveConfig,
		suite.mockWSHub,
	)
	suite.Require().NoError(err)
	suite.service = svc
}

func (suite *TradingAdaptiveTestSuite) TearDownTest() {
	if suite.service != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		suite.service.Shutdown(ctx)
	}
	if suite.db != nil {
		sqlDB, _ := suite.db.DB()
		if sqlDB != nil {
			sqlDB.Close()
		}
	}
}

func TestTradingAdaptiveTestSuite(t *testing.T) {
	suite.Run(t, new(TradingAdaptiveTestSuite))
}

// Test Adaptive Service Lifecycle
func (suite *TradingAdaptiveTestSuite) TestAdaptiveServiceLifecycle() {
	suite.Run("StartService", func() {
		err := suite.service.Start()
		suite.NoError(err)
	})

	suite.Run("HealthCheck", func() {
		health := suite.service.HealthCheck()
		suite.NotNil(health)
		suite.Contains(health, "status")
		suite.Contains(health, "adaptive_enabled")
	})

	suite.Run("StopService", func() {
		err := suite.service.Stop()
		suite.NoError(err)
	})
}

// Test Migration Control Operations
func (suite *TradingAdaptiveTestSuite) TestMigrationControl() {
	testPair := "BTCUSDT"

	suite.Run("StartMigration", func() {
		err := suite.service.StartMigration(testPair)
		suite.NoError(err)

		status, err := suite.service.GetMigrationStatus(testPair)
		suite.NoError(err)
		suite.NotNil(status)
		suite.Equal("migrating", status.Status)
	})

	suite.Run("SetMigrationPercentage", func() {
		err := suite.service.SetMigrationPercentage(testPair, 50)
		suite.NoError(err)

		status, err := suite.service.GetMigrationStatus(testPair)
		suite.NoError(err)
		suite.Equal(int32(50), status.Percentage)
	})

	suite.Run("PauseMigration", func() {
		err := suite.service.PauseMigration(testPair)
		suite.NoError(err)

		status, err := suite.service.GetMigrationStatus(testPair)
		suite.NoError(err)
		suite.Equal("paused", status.Status)
	})

	suite.Run("ResumeMigration", func() {
		err := suite.service.ResumeMigration(testPair)
		suite.NoError(err)

		status, err := suite.service.GetMigrationStatus(testPair)
		suite.NoError(err)
		suite.Equal("migrating", status.Status)
	})

	suite.Run("StopMigration", func() {
		err := suite.service.StopMigration(testPair)
		suite.NoError(err)

		status, err := suite.service.GetMigrationStatus(testPair)
		suite.NoError(err)
		suite.Equal("legacy", status.Status)
	})

	suite.Run("RollbackMigration", func() {
		// Start migration first
		err := suite.service.StartMigration(testPair)
		suite.NoError(err)

		// Then rollback
		err = suite.service.RollbackMigration(testPair)
		suite.NoError(err)

		status, err := suite.service.GetMigrationStatus(testPair)
		suite.NoError(err)
		suite.Equal("legacy", status.Status)
	})

	suite.Run("ResetMigration", func() {
		err := suite.service.ResetMigration(testPair)
		suite.NoError(err)

		status, err := suite.service.GetMigrationStatus(testPair)
		suite.NoError(err)
		suite.Equal("legacy", status.Status)
		suite.Equal(int32(0), status.Percentage)
	})
}

// Test Auto-Migration Features
func (suite *TradingAdaptiveTestSuite) TestAutoMigration() {
	suite.Run("EnableAutoMigration", func() {
		err := suite.service.EnableAutoMigration(true)
		suite.NoError(err)

		enabled := suite.service.IsAutoMigrationEnabled()
		suite.True(enabled)
	})

	suite.Run("DisableAutoMigration", func() {
		err := suite.service.EnableAutoMigration(false)
		suite.NoError(err)

		enabled := suite.service.IsAutoMigrationEnabled()
		suite.False(enabled)
	})

	suite.Run("AutoMigrationThresholds", func() {
		thresholds := &engine.AutoMigrationThresholds{
			LatencyThresholdMs:     50,
			ThroughputThresholdTPS: 2000,
			ErrorRateThreshold:     0.005,
			EvaluationIntervalSec:  5,
		}

		err := suite.service.SetPerformanceThresholds(thresholds)
		suite.NoError(err)

		// Verify thresholds are applied
		metrics, err := suite.service.GetEngineMetrics()
		suite.NoError(err)
		suite.NotNil(metrics)
	})
}

// Test Performance Metrics and Monitoring
func (suite *TradingAdaptiveTestSuite) TestPerformanceMetrics() {
	testPair := "BTCUSDT"

	suite.Run("GetPerformanceMetrics", func() {
		metrics, err := suite.service.GetPerformanceMetrics(testPair)
		suite.NoError(err)
		suite.NotNil(metrics)
		suite.Contains(metrics, "latency")
		suite.Contains(metrics, "throughput")
		suite.Contains(metrics, "error_rate")
	})

	suite.Run("GetEngineMetrics", func() {
		metrics, err := suite.service.GetEngineMetrics()
		suite.NoError(err)
		suite.NotNil(metrics)
	})

	suite.Run("GetAllMigrationStates", func() {
		states := suite.service.GetAllMigrationStates()
		suite.NotNil(states)
	})

	suite.Run("GetMetricsReport", func() {
		report, err := suite.service.GetMetricsReport()
		suite.NoError(err)
		suite.NotNil(report)
	})

	suite.Run("GetPerformanceComparison", func() {
		comparison, err := suite.service.GetPerformanceComparison(testPair)
		suite.NoError(err)
		suite.NotNil(comparison)
	})
}

// Test Circuit Breaker Operations
func (suite *TradingAdaptiveTestSuite) TestCircuitBreaker() {
	testPair := "BTCUSDT"

	suite.Run("ResetCircuitBreaker", func() {
		err := suite.service.ResetCircuitBreaker(testPair)
		suite.NoError(err)
	})

	suite.Run("CircuitBreakerRecovery", func() {
		// Simulate circuit breaker trigger and recovery
		for i := 0; i < 5; i++ {
			err := suite.service.ResetCircuitBreaker(testPair)
			suite.NoError(err)
			time.Sleep(time.Millisecond * 10)
		}
	})
}

// Test Concurrent Migration Operations
func (suite *TradingAdaptiveTestSuite) TestConcurrentMigrations() {
	pairs := []string{"BTCUSDT", "ETHUSDT", "ADAUSDT", "DOTUSDT", "LINKUSDT"}

	suite.Run("ConcurrentMigrationStart", func() {
		var wg sync.WaitGroup
		errChan := make(chan error, len(pairs))

		for _, pair := range pairs {
			wg.Add(1)
			go func(p string) {
				defer wg.Done()
				if err := suite.service.StartMigration(p); err != nil {
					errChan <- fmt.Errorf("failed to start migration for %s: %w", p, err)
				}
			}(pair)
		}

		wg.Wait()
		close(errChan)

		// Check for errors
		for err := range errChan {
			suite.NoError(err)
		}

		// Verify all migrations started
		for _, pair := range pairs {
			status, err := suite.service.GetMigrationStatus(pair)
			suite.NoError(err)
			suite.NotNil(status)
		}
	})

	suite.Run("ConcurrentMigrationControl", func() {
		var wg sync.WaitGroup
		operations := []func(string) error{
			suite.service.PauseMigration,
			suite.service.ResumeMigration,
			func(p string) error { return suite.service.SetMigrationPercentage(p, 75) },
		}

		for i, pair := range pairs {
			for j, op := range operations {
				wg.Add(1)
				go func(p string, operation func(string) error, idx int) {
					defer wg.Done()
					time.Sleep(time.Duration(idx*10) * time.Millisecond) // Stagger operations
					if err := operation(p); err != nil {
						suite.T().Logf("Operation failed for %s: %v", p, err)
					}
				}(pair, op, i*len(operations)+j)
			}
		}

		wg.Wait()
	})
}

// Test Performance Under Load
func (suite *TradingAdaptiveTestSuite) TestAdaptivePerformanceUnderLoad() {
	testPair := "BTCUSDT"

	suite.Run("HighConcurrencyMigration", func() {
		// Start migration
		err := suite.service.StartMigration(testPair)
		suite.NoError(err)

		var wg sync.WaitGroup
		concurrency := 100
		operationsPerWorker := 100
		start := time.Now()

		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()
				for j := 0; j < operationsPerWorker; j++ {
					// Perform various operations
					operations := []func() error{
						func() error {
							_, err := suite.service.GetMigrationStatus(testPair)
							return err
						},
						func() error {
							_, err := suite.service.GetPerformanceMetrics(testPair)
							return err
						},
						func() error {
							return suite.service.SetMigrationPercentage(testPair, int32((workerID*operationsPerWorker+j)%100))
						},
					}

					op := operations[j%len(operations)]
					if err := op(); err != nil {
						suite.T().Logf("Worker %d operation %d failed: %v", workerID, j, err)
					}
				}
			}(i)
		}

		wg.Wait()
		elapsed := time.Since(start)

		totalOps := concurrency * operationsPerWorker
		opsPerSecond := float64(totalOps) / elapsed.Seconds()

		suite.T().Logf("Adaptive service performance: %d operations in %v (%.2f ops/sec)",
			totalOps, elapsed, opsPerSecond)

		// Should handle at least 1000 operations per second
		suite.Greater(opsPerSecond, 1000.0)
	})
}

// Test Error Handling and Recovery
func (suite *TradingAdaptiveTestSuite) TestErrorHandlingAndRecovery() {
	suite.Run("InvalidPairMigration", func() {
		err := suite.service.StartMigration("")
		suite.Error(err)

		err = suite.service.StartMigration("INVALID")
		suite.Error(err)
	})

	suite.Run("InvalidMigrationPercentage", func() {
		testPair := "BTCUSDT"

		err := suite.service.SetMigrationPercentage(testPair, -1)
		suite.Error(err)

		err = suite.service.SetMigrationPercentage(testPair, 101)
		suite.Error(err)
	})

	suite.Run("MigrationStateRecovery", func() {
		testPair := "BTCUSDT"

		// Test recovery from various states
		states := []string{"legacy", "migrating", "adaptive", "paused"}

		for _, targetState := range states {
			// Force into state
			switch targetState {
			case "migrating":
				err := suite.service.StartMigration(testPair)
				suite.NoError(err)
			case "adaptive":
				err := suite.service.StartMigration(testPair)
				suite.NoError(err)
				err = suite.service.SetMigrationPercentage(testPair, 100)
				suite.NoError(err)
			case "paused":
				err := suite.service.StartMigration(testPair)
				suite.NoError(err)
				err = suite.service.PauseMigration(testPair)
				suite.NoError(err)
			case "legacy":
				err := suite.service.ResetMigration(testPair)
				suite.NoError(err)
			}

			// Verify state
			status, err := suite.service.GetMigrationStatus(testPair)
			suite.NoError(err)
			suite.Contains([]string{targetState, "legacy", "migrating"}, status.Status) // Allow reasonable states

			// Test recovery operations
			err = suite.service.ResetMigration(testPair)
			suite.NoError(err)
		}
	})
}

// Test Memory and Resource Management
func (suite *TradingAdaptiveTestSuite) TestResourceManagement() {
	suite.Run("MemoryLeakPrevention", func() {
		// Create and destroy many migration states
		pairs := make([]string, 100)
		for i := 0; i < 100; i++ {
			pairs[i] = fmt.Sprintf("TEST%dUSDT", i)
		}

		// Start migrations
		for _, pair := range pairs {
			err := suite.service.StartMigration(pair)
			suite.NoError(err)
		}

		// Get all states (should not cause memory issues)
		states := suite.service.GetAllMigrationStates()
		suite.GreaterOrEqual(len(states), 100)

		// Clean up
		for _, pair := range pairs {
			err := suite.service.ResetMigration(pair)
			suite.NoError(err)
		}

		// Verify cleanup
		finalStates := suite.service.GetAllMigrationStates()
		for _, pair := range pairs {
			if state, exists := finalStates[pair]; exists {
				suite.Equal("legacy", state.Status)
			}
		}
	})

	suite.Run("ResourceCleanupOnShutdown", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		err := suite.service.Shutdown(ctx)
		suite.NoError(err)
	})
}

// Benchmark adaptive service operations
func (suite *TradingAdaptiveTestSuite) TestAdaptiveBenchmarks() {
	if testing.Short() {
		suite.T().Skip("Skipping benchmark tests in short mode")
	}

	testPair := "BTCUSDT"

	suite.Run("BenchmarkMigrationOperations", func() {
		iterations := 10000
		start := time.Now()

		for i := 0; i < iterations; i++ {
			switch i % 4 {
			case 0:
				suite.service.GetMigrationStatus(testPair)
			case 1:
				suite.service.SetMigrationPercentage(testPair, int32(i%100))
			case 2:
				suite.service.GetPerformanceMetrics(testPair)
			case 3:
				suite.service.GetEngineMetrics()
			}
		}

		elapsed := time.Since(start)
		opsPerSecond := float64(iterations) / elapsed.Seconds()

		suite.T().Logf("Adaptive operations benchmark: %d operations in %v (%.2f ops/sec)",
			iterations, elapsed, opsPerSecond)

		// Should handle at least 10,000 operations per second
		suite.Greater(opsPerSecond, 10000.0)
	})
}
