// Package transaction_test - Comprehensive tests for cross-pair transaction coordination
package transaction_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Aidin1998/finalex/internal/trading/crosspair"
	crosspairTxn "github.com/Aidin1998/finalex/internal/trading/crosspair/transaction"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

// TransactionCoordinatorTestSuite provides comprehensive testing for transaction coordination
type TransactionCoordinatorTestSuite struct {
	suite.Suite
	coordinator        *crosspairTxn.CrossPairTransactionCoordinator
	api                *crosspairTxn.TransactionAPI
	retryManager       *crosspairTxn.RetryManager
	deadLetterQueue    *crosspairTxn.DeadLetterQueue
	mockXAManager      *MockXAManager
	mockBalanceService *MockBalanceService
	mockMatchingEngine *MockMatchingEngine
	mockTradeStore     *MockTradeStore
	logger             *zap.Logger
}

// SetupSuite initializes the test suite
func (suite *TransactionCoordinatorTestSuite) SetupSuite() {
	suite.logger = zaptest.NewLogger(suite.T())

	// Create mocks
	suite.mockXAManager = NewMockXAManager()
	suite.mockBalanceService = NewMockBalanceService()
	suite.mockMatchingEngine = NewMockMatchingEngine()
	suite.mockTradeStore = NewMockTradeStore()

	// Create coordinator
	matchingEngines := map[string]crosspair.MatchingEngine{
		"BTC/USD": suite.mockMatchingEngine,
		"ETH/USD": suite.mockMatchingEngine,
	}

	config := crosspairTxn.DefaultCoordinatorConfig()
	config.MaxRetries = 3
	config.DefaultTimeout = 10 * time.Second

	var err error
	suite.coordinator, err = crosspairTxn.NewCrossPairTransactionCoordinator(
		suite.logger,
		suite.mockXAManager,
		suite.mockBalanceService,
		matchingEngines,
		suite.mockTradeStore,
		config,
	)
	require.NoError(suite.T(), err)

	// Create retry manager and dead letter queue
	suite.retryManager = crosspairTxn.NewRetryManager(
		suite.coordinator,
		crosspairTxn.DefaultRetryStrategy(),
		suite.logger,
		2, // workers
	)

	suite.deadLetterQueue = crosspairTxn.NewDeadLetterQueue(
		suite.coordinator,
		suite.logger,
		2, // workers
	)

	// Create API
	suite.api = crosspairTxn.NewTransactionAPI(
		suite.coordinator,
		suite.retryManager,
		suite.deadLetterQueue,
		suite.logger,
	)
}

// TearDownSuite cleans up the test suite
func (suite *TransactionCoordinatorTestSuite) TearDownSuite() {
	if suite.retryManager != nil {
		suite.retryManager.Stop()
	}
	if suite.deadLetterQueue != nil {
		suite.deadLetterQueue.Stop()
	}
}

// TestBasicTransactionLifecycle tests the basic transaction lifecycle
func (suite *TransactionCoordinatorTestSuite) TestBasicTransactionLifecycle() {
	ctx := context.Background()

	// Create test order
	order := &crosspair.CrossPairOrder{
		ID:        uuid.New(),
		UserID:    uuid.New(),
		FromAsset: "BTC",
		ToAsset:   "USD",
		Quantity:  decimal.NewFromFloat(1.0),
		Type:      crosspair.CrossPairMarketOrder,
		Status:    crosspair.CrossPairOrderPending,
		CreatedAt: time.Now(),
	}

	// Set up mocks for successful transaction
	suite.setupSuccessfulMocks()

	// Begin transaction
	req := &crosspairTxn.TransactionRequest{
		Type:   crosspairTxn.TransactionTypeTrade,
		UserID: order.UserID,
		Order:  order,
	}

	resp, err := suite.api.BeginTransaction(ctx, req)
	require.NoError(suite.T(), err)
	assert.NotEqual(suite.T(), uuid.Nil, resp.TransactionID)
	assert.Equal(suite.T(), "INITIALIZED", resp.State)

	// Wait for completion
	completedResp, err := suite.api.WaitForTransaction(ctx, resp.TransactionID, 5*time.Second)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), "COMMITTED", completedResp.State)
	assert.NotNil(suite.T(), completedResp.CompletedAt)
}

// TestTransactionRollback tests transaction rollback scenarios
func (suite *TransactionCoordinatorTestSuite) TestTransactionRollback() {
	ctx := context.Background()

	// Create test order
	order := &crosspair.CrossPairOrder{
		ID:        uuid.New(),
		UserID:    uuid.New(),
		FromAsset: "BTC",
		ToAsset:   "USD",
		Quantity:  decimal.NewFromFloat(1.0),
		Type:      crosspair.CrossPairMarketOrder,
		Status:    crosspair.CrossPairOrderPending,
		CreatedAt: time.Now(),
	}

	// Set up mocks for failed prepare phase
	suite.setupFailedPrepareMocks()

	// Begin transaction
	req := &crosspairTxn.TransactionRequest{
		Type:   crosspairTxn.TransactionTypeTrade,
		UserID: order.UserID,
		Order:  order,
	}

	resp, err := suite.api.BeginTransaction(ctx, req)
	require.NoError(suite.T(), err)

	// Wait for completion (should abort)
	completedResp, err := suite.api.WaitForTransaction(ctx, resp.TransactionID, 5*time.Second)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), "ABORTED", completedResp.State)
	assert.NotEmpty(suite.T(), completedResp.Error)
}

// TestConcurrentTransactions tests race conditions with concurrent transactions
func (suite *TransactionCoordinatorTestSuite) TestConcurrentTransactions() {
	ctx := context.Background()
	numTransactions := 10

	suite.setupSuccessfulMocks()

	var wg sync.WaitGroup
	results := make(chan *crosspairTxn.TransactionResponse, numTransactions)
	errors := make(chan error, numTransactions)

	// Launch concurrent transactions
	for i := 0; i < numTransactions; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			order := &crosspair.CrossPairOrder{
				ID:        uuid.New(),
				UserID:    uuid.New(),
				FromAsset: "BTC",
				ToAsset:   "USD",
				Quantity:  decimal.NewFromFloat(1.0 + float64(index)*0.1),
				Type:      crosspair.CrossPairMarketOrder,
				Status:    crosspair.CrossPairOrderPending,
				CreatedAt: time.Now(),
			}

			req := &crosspairTxn.TransactionRequest{
				Type:   crosspairTxn.TransactionTypeTrade,
				UserID: order.UserID,
				Order:  order,
			}

			resp, err := suite.api.BeginTransaction(ctx, req)
			if err != nil {
				errors <- err
				return
			}

			// Wait for completion
			completedResp, err := suite.api.WaitForTransaction(ctx, resp.TransactionID, 10*time.Second)
			if err != nil {
				errors <- err
				return
			}

			results <- completedResp
		}(i)
	}

	wg.Wait()
	close(results)
	close(errors)

	// Check results
	successCount := 0
	for resp := range results {
		if resp.State == "COMMITTED" {
			successCount++
		}
	}

	errorCount := 0
	for err := range errors {
		suite.T().Logf("Transaction error: %v", err)
		errorCount++
	}

	// All transactions should succeed or fail gracefully
	assert.Equal(suite.T(), numTransactions, successCount+errorCount)
	assert.GreaterOrEqual(suite.T(), successCount, numTransactions/2) // At least half should succeed
}

// TestRetryMechanism tests the retry mechanism for failed transactions
func (suite *TransactionCoordinatorTestSuite) TestRetryMechanism() {
	ctx := context.Background()

	order := &crosspair.CrossPairOrder{
		ID:        uuid.New(),
		UserID:    uuid.New(),
		FromAsset: "BTC",
		ToAsset:   "USD",
		Quantity:  decimal.NewFromFloat(1.0),
		Type:      crosspair.CrossPairMarketOrder,
		Status:    crosspair.CrossPairOrderPending,
		CreatedAt: time.Now(),
	}

	// Set up mocks to fail first 2 attempts, succeed on 3rd
	attempt := int64(0)
	suite.mockBalanceService.On("ValidateTransaction", mock.Anything, mock.Anything).Return(func(ctx context.Context, txnCtx interface{}) error {
		attemptNum := atomic.AddInt64(&attempt, 1)
		if attemptNum <= 2 {
			return fmt.Errorf("temporary failure")
		}
		return nil
	})

	// Set up other mocks for success
	suite.setupOtherSuccessfulMocks()

	// Begin transaction
	req := &crosspairTxn.TransactionRequest{
		Type:   crosspairTxn.TransactionTypeTrade,
		UserID: order.UserID,
		Order:  order,
	}

	resp, err := suite.api.BeginTransaction(ctx, req)
	require.NoError(suite.T(), err)

	// Wait longer to allow retries
	completedResp, err := suite.api.WaitForTransaction(ctx, resp.TransactionID, 30*time.Second)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), "COMMITTED", completedResp.State)

	// Verify retry count
	finalAttempt := atomic.LoadInt64(&attempt)
	assert.Equal(suite.T(), int64(3), finalAttempt)
}

// TestDeadLetterQueue tests dead letter queue functionality
func (suite *TransactionCoordinatorTestSuite) TestDeadLetterQueue() {
	ctx := context.Background()

	order := &crosspair.CrossPairOrder{
		ID:        uuid.New(),
		UserID:    uuid.New(),
		FromAsset: "BTC",
		ToAsset:   "USD",
		Quantity:  decimal.NewFromFloat(1.0),
		Type:      crosspair.CrossPairMarketOrder,
		Status:    crosspair.CrossPairOrderPending,
		CreatedAt: time.Now(),
	}

	// Set up mocks to always fail
	suite.mockBalanceService.On("ValidateTransaction", mock.Anything, mock.Anything).Return(
		fmt.Errorf("persistent failure"))

	// Begin transaction
	req := &crosspairTxn.TransactionRequest{
		Type:   crosspairTxn.TransactionTypeTrade,
		UserID: order.UserID,
		Order:  order,
	}

	resp, err := suite.api.BeginTransaction(ctx, req)
	require.NoError(suite.T(), err)

	// Wait for it to be dead lettered
	time.Sleep(5 * time.Second)

	// Check dead letter queue
	deadLetters, err := suite.api.GetDeadLetters(ctx)
	require.NoError(suite.T(), err)
	assert.Len(suite.T(), deadLetters, 1)
	assert.Equal(suite.T(), resp.TransactionID, deadLetters[0].TransactionContext.ID)
}

// TestCompensationFlow tests the compensation mechanism
func (suite *TransactionCoordinatorTestSuite) TestCompensationFlow() {
	ctx := context.Background()

	order := &crosspair.CrossPairOrder{
		ID:        uuid.New(),
		UserID:    uuid.New(),
		FromAsset: "BTC",
		ToAsset:   "USD",
		Quantity:  decimal.NewFromFloat(1.0),
		Type:      crosspair.CrossPairMarketOrder,
		Status:    crosspair.CrossPairOrderPending,
		CreatedAt: time.Now(),
	}

	// Set up mocks: prepare succeeds, commit fails
	suite.mockBalanceService.On("ValidateTransaction", mock.Anything, mock.Anything).Return(nil)
	suite.mockBalanceService.On("Prepare", mock.Anything, mock.Anything).Return(true, nil)
	suite.mockBalanceService.On("Commit", mock.Anything, mock.Anything, mock.Anything).Return(
		fmt.Errorf("commit failed"))
	suite.mockBalanceService.On("GetCompensationAction", mock.Anything, mock.Anything).Return(
		&crosspairTxn.CompensationAction{
			ResourceName: "BALANCE_SERVICE",
			Action:       "RESTORE_BALANCE",
		}, nil)

	suite.setupOtherSuccessfulMocks()

	// Begin transaction
	req := &crosspairTxn.TransactionRequest{
		Type:   crosspairTxn.TransactionTypeTrade,
		UserID: order.UserID,
		Order:  order,
	}

	resp, err := suite.api.BeginTransaction(ctx, req)
	require.NoError(suite.T(), err)

	// Wait for compensation to complete
	time.Sleep(5 * time.Second)

	// Check final state
	finalResp, err := suite.api.GetTransaction(ctx, resp.TransactionID)
	require.NoError(suite.T(), err)
	// Should be either COMPENSATED or in dead letter queue
	assert.Contains(suite.T(), []string{"COMPENSATED", "DEAD_LETTERED"}, finalResp.State)
}

// TestTimeoutHandling tests transaction timeout scenarios
func (suite *TransactionCoordinatorTestSuite) TestTimeoutHandling() {
	ctx := context.Background()

	order := &crosspair.CrossPairOrder{
		ID:        uuid.New(),
		UserID:    uuid.New(),
		FromAsset: "BTC",
		ToAsset:   "USD",
		Quantity:  decimal.NewFromFloat(1.0),
		Type:      crosspair.CrossPairMarketOrder,
		Status:    crosspair.CrossPairOrderPending,
		CreatedAt: time.Now(),
	}

	// Set up mocks to cause timeout
	suite.mockBalanceService.On("ValidateTransaction", mock.Anything, mock.Anything).Return(nil)
	suite.mockBalanceService.On("Prepare", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		time.Sleep(2 * time.Second) // Longer than timeout
	}).Return(true, nil)

	// Begin transaction with short timeout
	timeout := 1 * time.Second
	req := &crosspairTxn.TransactionRequest{
		Type:    crosspairTxn.TransactionTypeTrade,
		UserID:  order.UserID,
		Order:   order,
		Timeout: &timeout,
	}

	resp, err := suite.api.BeginTransaction(ctx, req)
	require.NoError(suite.T(), err)

	// Wait for timeout
	finalResp, err := suite.api.WaitForTransaction(ctx, resp.TransactionID, 5*time.Second)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), "ABORTED", finalResp.State)
	assert.Contains(suite.T(), finalResp.Error, "timeout")
}

// TestLatencyBenchmark benchmarks transaction latency
func (suite *TransactionCoordinatorTestSuite) TestLatencyBenchmark() {
	ctx := context.Background()
	suite.setupSuccessfulMocks()

	numTransactions := 100
	latencies := make([]time.Duration, numTransactions)

	for i := 0; i < numTransactions; i++ {
		start := time.Now()

		order := &crosspair.CrossPairOrder{
			ID:        uuid.New(),
			UserID:    uuid.New(),
			FromAsset: "BTC",
			ToAsset:   "USD",
			Quantity:  decimal.NewFromFloat(1.0),
			Type:      crosspair.CrossPairMarketOrder,
			Status:    crosspair.CrossPairOrderPending,
			CreatedAt: time.Now(),
		}

		req := &crosspairTxn.TransactionRequest{
			Type:   crosspairTxn.TransactionTypeTrade,
			UserID: order.UserID,
			Order:  order,
		}

		resp, err := suite.api.BeginTransaction(ctx, req)
		require.NoError(suite.T(), err)

		_, err = suite.api.WaitForTransaction(ctx, resp.TransactionID, 5*time.Second)
		require.NoError(suite.T(), err)

		latencies[i] = time.Since(start)
	}

	// Calculate statistics
	var totalLatency time.Duration
	var maxLatency time.Duration

	for _, latency := range latencies {
		totalLatency += latency
		if latency > maxLatency {
			maxLatency = latency
		}
	}

	avgLatency := totalLatency / time.Duration(numTransactions)

	suite.T().Logf("Latency Statistics:")
	suite.T().Logf("  Average: %v", avgLatency)
	suite.T().Logf("  Maximum: %v", maxLatency)

	// Calculate P95 and P99
	// Sort latencies
	for i := 0; i < len(latencies)-1; i++ {
		for j := i + 1; j < len(latencies); j++ {
			if latencies[i] > latencies[j] {
				latencies[i], latencies[j] = latencies[j], latencies[i]
			}
		}
	}

	p95Index := int(float64(numTransactions) * 0.95)
	p99Index := int(float64(numTransactions) * 0.99)

	suite.T().Logf("  P95: %v", latencies[p95Index])
	suite.T().Logf("  P99: %v", latencies[p99Index])

	// Assert that 99% of transactions complete within 2ms (as per requirement)
	assert.Less(suite.T(), latencies[p99Index], 2*time.Millisecond,
		"99%% of transactions should complete within 2ms")
}

// TestStressTest performs stress testing under high load
func (suite *TransactionCoordinatorTestSuite) TestStressTest() {
	ctx := context.Background()
	suite.setupSuccessfulMocks()

	numWorkers := 50
	transactionsPerWorker := 20
	totalTransactions := numWorkers * transactionsPerWorker

	var wg sync.WaitGroup
	successCount := int64(0)
	errorCount := int64(0)

	start := time.Now()

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for j := 0; j < transactionsPerWorker; j++ {
				order := &crosspair.CrossPairOrder{
					ID:        uuid.New(),
					UserID:    uuid.New(),
					FromAsset: "BTC",
					ToAsset:   "USD",
					Quantity:  decimal.NewFromFloat(1.0),
					Type:      crosspair.CrossPairMarketOrder,
					Status:    crosspair.CrossPairOrderPending,
					CreatedAt: time.Now(),
				}

				req := &crosspairTxn.TransactionRequest{
					Type:   crosspairTxn.TransactionTypeTrade,
					UserID: order.UserID,
					Order:  order,
				}

				resp, err := suite.api.BeginTransaction(ctx, req)
				if err != nil {
					atomic.AddInt64(&errorCount, 1)
					continue
				}

				_, err = suite.api.WaitForTransaction(ctx, resp.TransactionID, 10*time.Second)
				if err != nil {
					atomic.AddInt64(&errorCount, 1)
				} else {
					atomic.AddInt64(&successCount, 1)
				}
			}
		}(i)
	}

	wg.Wait()
	duration := time.Since(start)

	suite.T().Logf("Stress Test Results:")
	suite.T().Logf("  Total Transactions: %d", totalTransactions)
	suite.T().Logf("  Successful: %d", successCount)
	suite.T().Logf("  Failed: %d", errorCount)
	suite.T().Logf("  Success Rate: %.2f%%", float64(successCount)/float64(totalTransactions)*100)
	suite.T().Logf("  Duration: %v", duration)
	suite.T().Logf("  Throughput: %.2f tx/sec", float64(totalTransactions)/duration.Seconds())

	// Assert reasonable success rate under stress
	successRate := float64(successCount) / float64(totalTransactions)
	assert.GreaterOrEqual(suite.T(), successRate, 0.8, "Success rate should be at least 80% under stress")
}

// Helper methods for setting up mocks

func (suite *TransactionCoordinatorTestSuite) setupSuccessfulMocks() {
	suite.mockBalanceService.On("ValidateTransaction", mock.Anything, mock.Anything).Return(nil)
	suite.setupOtherSuccessfulMocks()
}

func (suite *TransactionCoordinatorTestSuite) setupOtherSuccessfulMocks() {
	// Balance service mocks
	suite.mockBalanceService.On("Prepare", mock.Anything, mock.Anything).Return(true, nil)
	suite.mockBalanceService.On("Commit", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	suite.mockBalanceService.On("Rollback", mock.Anything, mock.Anything).Return(nil)
	suite.mockBalanceService.On("Forget", mock.Anything, mock.Anything).Return(nil)
	suite.mockBalanceService.On("GetResourceName").Return("BALANCE_SERVICE")
	suite.mockBalanceService.On("EstimateGas", mock.Anything, mock.Anything).Return(decimal.NewFromFloat(0.001), nil)
	suite.mockBalanceService.On("GetCompensationAction", mock.Anything, mock.Anything).Return(
		&crosspairTxn.CompensationAction{}, nil)

	// Matching engine mocks
	suite.mockMatchingEngine.On("ValidateTransaction", mock.Anything, mock.Anything).Return(nil)
	suite.mockMatchingEngine.On("Prepare", mock.Anything, mock.Anything).Return(false, nil)
	suite.mockMatchingEngine.On("Commit", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	suite.mockMatchingEngine.On("Rollback", mock.Anything, mock.Anything).Return(nil)
	suite.mockMatchingEngine.On("Forget", mock.Anything, mock.Anything).Return(nil)
	suite.mockMatchingEngine.On("GetResourceName").Return("MATCHING_ENGINE")
	suite.mockMatchingEngine.On("EstimateGas", mock.Anything, mock.Anything).Return(decimal.NewFromFloat(0.01), nil)
	suite.mockMatchingEngine.On("GetCompensationAction", mock.Anything, mock.Anything).Return(
		&crosspairTxn.CompensationAction{}, nil)

	// Trade store mocks
	suite.mockTradeStore.On("ValidateTransaction", mock.Anything, mock.Anything).Return(nil)
	suite.mockTradeStore.On("Prepare", mock.Anything, mock.Anything).Return(false, nil)
	suite.mockTradeStore.On("Commit", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	suite.mockTradeStore.On("Rollback", mock.Anything, mock.Anything).Return(nil)
	suite.mockTradeStore.On("Forget", mock.Anything, mock.Anything).Return(nil)
	suite.mockTradeStore.On("GetResourceName").Return("TRADE_STORE")
	suite.mockTradeStore.On("EstimateGas", mock.Anything, mock.Anything).Return(decimal.NewFromFloat(0.005), nil)
	suite.mockTradeStore.On("GetCompensationAction", mock.Anything, mock.Anything).Return(
		&crosspairTxn.CompensationAction{}, nil)
	suite.mockTradeStore.On("Delete", mock.Anything, mock.Anything).Return(nil)
}

func (suite *TransactionCoordinatorTestSuite) setupFailedPrepareMocks() {
	suite.mockBalanceService.On("ValidateTransaction", mock.Anything, mock.Anything).Return(nil)
	suite.mockBalanceService.On("Prepare", mock.Anything, mock.Anything).Return(false, fmt.Errorf("prepare failed"))
	suite.mockBalanceService.On("Rollback", mock.Anything, mock.Anything).Return(nil)
	suite.mockBalanceService.On("GetResourceName").Return("BALANCE_SERVICE")
	suite.mockBalanceService.On("EstimateGas", mock.Anything, mock.Anything).Return(decimal.NewFromFloat(0.001), nil)
	suite.mockBalanceService.On("GetCompensationAction", mock.Anything, mock.Anything).Return(
		&crosspairTxn.CompensationAction{}, nil)
}

// Run the test suite
func TestTransactionCoordinatorSuite(t *testing.T) {
	suite.Run(t, new(TransactionCoordinatorTestSuite))
}
