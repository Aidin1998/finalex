package transaction

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// DistributedTransactionTester provides testing capabilities for distributed transactions
type DistributedTransactionTester struct {
	suite        *TransactionManagerSuite
	db           *gorm.DB
	logger       *zap.Logger
	chaosEnabled bool
	metrics      *TestingMetrics
	scenarios    map[string]*TestScenario
	mu           sync.RWMutex
}

// TestingMetrics tracks testing performance and results
type TestingMetrics struct {
	TotalTests     int64
	PassedTests    int64
	FailedTests    int64
	ChaosTests     int64
	LoadTests      int64
	AverageLatency time.Duration
	MaxLatency     time.Duration
	MinLatency     time.Duration
	LastTestRun    time.Time
	ErrorRate      float64
	ThroughputOps  float64
	mu             sync.RWMutex
}

// TestScenario defines a testing scenario
type TestScenario struct {
	ID          string
	Name        string
	Description string
	Duration    time.Duration
	Concurrency int
	Operations  []TestOperation
	ChaosConfig *ChaosConfig
}

// TestOperation defines an operation to test
type TestOperation struct {
	Type       string
	Service    string
	Parameters map[string]interface{}
	Weight     float64 // Probability weight for random selection
}

// ChaosConfig defines chaos engineering parameters
type ChaosConfig struct {
	FailureRate         float64       // Percentage of operations to fail
	LatencyInjection    time.Duration // Additional latency to inject
	ResourceExhaustion  bool          // Simulate resource exhaustion
	NetworkPartition    bool          // Simulate network issues
	RandomFailures      bool          // Enable random failures
	CircuitBreakerTrips bool          // Trigger circuit breakers
}

// TestResult represents the result of a test execution
type TestResult struct {
	ScenarioID      string                 `json:"scenario_id"`
	Success         bool                   `json:"success"`
	Duration        time.Duration          `json:"duration"`
	OperationsCount int                    `json:"operations_count"`
	SuccessfulOps   int                    `json:"successful_ops"`
	FailedOps       int                    `json:"failed_ops"`
	AverageLatency  time.Duration          `json:"average_latency"`
	MaxLatency      time.Duration          `json:"max_latency"`
	MinLatency      time.Duration          `json:"min_latency"`
	ThroughputOps   float64                `json:"throughput_ops"`
	ErrorRate       float64                `json:"error_rate"`
	Errors          []string               `json:"errors"`
	ChaosEnabled    bool                   `json:"chaos_enabled"`
	CustomMetrics   map[string]interface{} `json:"custom_metrics"`
	StartTime       time.Time              `json:"start_time"`
	EndTime         time.Time              `json:"end_time"`
}

// NewDistributedTransactionTester creates a new testing framework
func NewDistributedTransactionTester(
	suite *TransactionManagerSuite,
	db *gorm.DB,
	logger *zap.Logger,
) *DistributedTransactionTester {
	tester := &DistributedTransactionTester{
		suite:     suite,
		db:        db,
		logger:    logger,
		metrics:   &TestingMetrics{},
		scenarios: make(map[string]*TestScenario),
	}

	// Initialize default scenarios
	tester.initializeDefaultScenarios()

	return tester
}

// initializeDefaultScenarios sets up common testing scenarios
func (dtt *DistributedTransactionTester) initializeDefaultScenarios() {
	// Chaos testing scenario
	dtt.scenarios["chaos_test"] = &TestScenario{
		ID:          "chaos_test",
		Name:        "Chaos Engineering Test",
		Description: "Tests system resilience under chaotic conditions",
		Duration:    5 * time.Minute,
		Concurrency: 10,
		Operations: []TestOperation{
			{
				Type:    "trade_execution",
				Service: "trading",
				Parameters: map[string]interface{}{
					"user_id": "test_user",
					"symbol":  "BTCUSDT",
					"side":    "buy",
					"amount":  100.0,
				},
				Weight: 0.4,
			},
			{
				Type:    "balance_transfer",
				Service: "bookkeeper",
				Parameters: map[string]interface{}{
					"from_user": "user1",
					"to_user":   "user2",
					"currency":  "USDT",
					"amount":    50.0,
				},
				Weight: 0.3,
			},
			{
				Type:    "settlement",
				Service: "settlement",
				Parameters: map[string]interface{}{
					"trade_id": "test_trade",
					"amount":   100.0,
				},
				Weight: 0.3,
			},
		},
		ChaosConfig: &ChaosConfig{
			FailureRate:         0.15, // 15% failure rate
			LatencyInjection:    50 * time.Millisecond,
			RandomFailures:      true,
			CircuitBreakerTrips: true,
		},
	}

	// Load testing scenario
	dtt.scenarios["load_test"] = &TestScenario{
		ID:          "load_test",
		Name:        "Load Testing",
		Description: "Tests system performance under high load",
		Duration:    10 * time.Minute,
		Concurrency: 50,
		Operations: []TestOperation{
			{
				Type:    "multi_service",
				Service: "distributed",
				Parameters: map[string]interface{}{
					"operations_count": 5,
					"services":         []string{"bookkeeper", "trading", "settlement"},
				},
				Weight: 1.0,
			},
		},
	}

	// Multi-service transaction scenario
	dtt.scenarios["multi_service"] = &TestScenario{
		ID:          "multi_service",
		Name:        "Multi-Service Transaction",
		Description: "Tests distributed transactions across multiple services",
		Duration:    2 * time.Minute,
		Concurrency: 20,
		Operations: []TestOperation{
			{
				Type:    "distributed_transaction",
				Service: "distributed",
				Parameters: map[string]interface{}{
					"services": []string{"bookkeeper", "trading", "settlement", "fiat"},
				},
				Weight: 1.0,
			},
		},
	}
}

// EnableChaos enables or disables chaos engineering
func (dtt *DistributedTransactionTester) EnableChaos(enabled bool) {
	dtt.mu.Lock()
	defer dtt.mu.Unlock()
	dtt.chaosEnabled = enabled
	dtt.logger.Info("Chaos engineering enabled", zap.Bool("enabled", enabled))
}

// SetChaosConfig updates the chaos configuration for testing
func (dtt *DistributedTransactionTester) SetChaosConfig(config ChaosConfig) {
	dtt.mu.Lock()
	defer dtt.mu.Unlock()

	// Update all scenarios with new chaos config
	for _, scenario := range dtt.scenarios {
		if scenario.ChaosConfig != nil {
			scenario.ChaosConfig.FailureRate = config.FailureRate
			scenario.ChaosConfig.LatencyInjection = config.LatencyInjection
			scenario.ChaosConfig.ResourceExhaustion = config.ResourceExhaustion
			scenario.ChaosConfig.NetworkPartition = config.NetworkPartition
			scenario.ChaosConfig.RandomFailures = config.RandomFailures
			scenario.ChaosConfig.CircuitBreakerTrips = config.CircuitBreakerTrips
		}
	}

	dtt.logger.Info("Chaos configuration updated",
		zap.Float64("failure_rate", config.FailureRate),
		zap.Duration("latency_injection", config.LatencyInjection),
		zap.Bool("resource_exhaustion", config.ResourceExhaustion),
		zap.Bool("network_partition", config.NetworkPartition),
		zap.Bool("random_failures", config.RandomFailures),
		zap.Bool("circuit_breaker_trips", config.CircuitBreakerTrips))
}

// RunScenario executes a specific test scenario
func (dtt *DistributedTransactionTester) RunScenario(ctx context.Context, scenarioID string) (*TestResult, error) {
	dtt.mu.RLock()
	scenario, exists := dtt.scenarios[scenarioID]
	dtt.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("scenario %s not found", scenarioID)
	}

	dtt.logger.Info("Starting test scenario",
		zap.String("scenario_id", scenarioID),
		zap.String("name", scenario.Name),
		zap.Duration("duration", scenario.Duration),
		zap.Int("concurrency", scenario.Concurrency))

	startTime := time.Now()
	result := &TestResult{
		ScenarioID:    scenarioID,
		StartTime:     startTime,
		ChaosEnabled:  dtt.chaosEnabled,
		CustomMetrics: make(map[string]interface{}),
		Errors:        make([]string, 0),
	}

	// Create context with timeout
	testCtx, cancel := context.WithTimeout(ctx, scenario.Duration)
	defer cancel()

	// Run concurrent workers
	var wg sync.WaitGroup
	var totalOps, successOps, failedOps int64
	var latencies []time.Duration
	var latencyMu sync.Mutex
	errorChan := make(chan string, scenario.Concurrency*100) // Buffer for errors

	for i := 0; i < scenario.Concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			dtt.runWorker(testCtx, scenario, workerID, &totalOps, &successOps, &failedOps, &latencies, &latencyMu, errorChan)
		}(i)
	}

	// Wait for completion
	wg.Wait()
	close(errorChan)

	// Collect errors
	for err := range errorChan {
		result.Errors = append(result.Errors, err)
	}

	// Calculate metrics
	endTime := time.Now()
	result.EndTime = endTime
	result.Duration = endTime.Sub(startTime)
	result.OperationsCount = int(totalOps)
	result.SuccessfulOps = int(successOps)
	result.FailedOps = int(failedOps)
	result.Success = failedOps == 0

	if len(latencies) > 0 {
		result.AverageLatency = dtt.calculateAverageLatency(latencies)
		result.MaxLatency = dtt.findMaxLatency(latencies)
		result.MinLatency = dtt.findMinLatency(latencies)
	}

	if totalOps > 0 {
		result.ErrorRate = float64(failedOps) / float64(totalOps)
		result.ThroughputOps = float64(totalOps) / result.Duration.Seconds()
	}

	// Update testing metrics
	dtt.updateTestingMetrics(result)

	dtt.logger.Info("Test scenario completed",
		zap.String("scenario_id", scenarioID),
		zap.Bool("success", result.Success),
		zap.Duration("duration", result.Duration),
		zap.Int("total_ops", result.OperationsCount),
		zap.Float64("error_rate", result.ErrorRate),
		zap.Float64("throughput", result.ThroughputOps))

	return result, nil
}

// LoadTestTransaction runs a load test for distributed transactions
func (dtt *DistributedTransactionTester) LoadTestTransaction(
	ctx context.Context,
	scenarioID string,
	concurrency int,
	duration time.Duration,
) (*TestResult, error) {
	// Create or update scenario for load testing
	scenario := &TestScenario{
		ID:          scenarioID,
		Name:        fmt.Sprintf("Load Test - %s", scenarioID),
		Description: "Dynamic load test scenario",
		Duration:    duration,
		Concurrency: concurrency,
		Operations: []TestOperation{
			{
				Type:    "distributed_transaction",
				Service: "distributed",
				Parameters: map[string]interface{}{
					"complexity": "high",
					"services":   []string{"bookkeeper", "trading", "settlement"},
				},
				Weight: 1.0,
			},
		},
	}

	dtt.mu.Lock()
	dtt.scenarios[scenarioID] = scenario
	dtt.mu.Unlock()

	return dtt.RunScenario(ctx, scenarioID)
}

// runWorker executes operations for a single worker
func (dtt *DistributedTransactionTester) runWorker(
	ctx context.Context,
	scenario *TestScenario,
	workerID int,
	totalOps, successOps, failedOps *int64,
	latencies *[]time.Duration,
	latencyMu *sync.Mutex,
	errorChan chan<- string,
) {
	ticker := time.NewTicker(100 * time.Millisecond) // Execute every 100ms
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Select random operation based on weights
			operation := dtt.selectRandomOperation(scenario.Operations)
			if operation == nil {
				continue
			}

			// Execute operation
			startTime := time.Now()
			err := dtt.executeOperation(ctx, operation)
			latency := time.Since(startTime)

			atomic.AddInt64(totalOps, 1)

			latencyMu.Lock()
			*latencies = append(*latencies, latency)
			latencyMu.Unlock()

			if err != nil {
				atomic.AddInt64(failedOps, 1)
				select {
				case errorChan <- fmt.Sprintf("Worker %d: %v", workerID, err):
				default:
					// Channel full, skip error logging
				}
			} else {
				atomic.AddInt64(successOps, 1)
			}
		}
	}
}

// selectRandomOperation selects an operation based on weights
func (dtt *DistributedTransactionTester) selectRandomOperation(operations []TestOperation) *TestOperation {
	if len(operations) == 0 {
		return nil
	}

	// Calculate total weight
	totalWeight := 0.0
	for _, op := range operations {
		totalWeight += op.Weight
	}

	// Select random value
	randValue := rand.Float64() * totalWeight
	currentWeight := 0.0

	for _, op := range operations {
		currentWeight += op.Weight
		if randValue <= currentWeight {
			return &op
		}
	}

	// Fallback to first operation
	return &operations[0]
}

// executeOperation executes a single test operation
func (dtt *DistributedTransactionTester) executeOperation(ctx context.Context, operation *TestOperation) error {
	// Apply chaos if enabled
	if dtt.chaosEnabled {
		if err := dtt.applyChaos(ctx); err != nil {
			return fmt.Errorf("chaos injection failed: %w", err)
		}
	}

	switch operation.Type {
	case "trade_execution":
		return dtt.executeTradeOperation(ctx, operation.Parameters)
	case "balance_transfer":
		return dtt.executeBalanceOperation(ctx, operation.Parameters)
	case "settlement":
		return dtt.executeSettlementOperation(ctx, operation.Parameters)
	case "distributed_transaction":
		return dtt.executeDistributedTransaction(ctx, operation.Parameters)
	case "multi_service":
		return dtt.executeMultiServiceOperation(ctx, operation.Parameters)
	default:
		return fmt.Errorf("unknown operation type: %s", operation.Type)
	}
}

// applyChaos applies chaos engineering effects
func (dtt *DistributedTransactionTester) applyChaos(ctx context.Context) error {
	// Random failure injection
	if rand.Float64() < 0.05 { // 5% chance of random failure
		return fmt.Errorf("chaos-induced failure")
	}

	// Latency injection
	if rand.Float64() < 0.2 { // 20% chance of latency injection
		delay := time.Duration(rand.Intn(100)) * time.Millisecond
		time.Sleep(delay)
	}

	return nil
}

// executeTradeOperation simulates a trade execution
func (dtt *DistributedTransactionTester) executeTradeOperation(ctx context.Context, params map[string]interface{}) error {
	// Create mock transaction operations
	operations := []TransactionOperation{
		{
			Service:   "bookkeeper",
			Operation: "reserve_balance",
			Parameters: map[string]interface{}{
				"user_id":  params["user_id"],
				"currency": "USDT",
				"amount":   params["amount"],
			},
		},
		{
			Service:    "trading",
			Operation:  "place_order",
			Parameters: params,
		},
	}

	_, err := dtt.suite.ExecuteDistributedTransaction(ctx, operations, 30*time.Second)
	return err
}

// executeBalanceOperation simulates a balance transfer
func (dtt *DistributedTransactionTester) executeBalanceOperation(ctx context.Context, params map[string]interface{}) error {
	operations := []TransactionOperation{
		{
			Service:    "bookkeeper",
			Operation:  "transfer",
			Parameters: params,
		},
	}

	_, err := dtt.suite.ExecuteDistributedTransaction(ctx, operations, 15*time.Second)
	return err
}

// executeSettlementOperation simulates a settlement
func (dtt *DistributedTransactionTester) executeSettlementOperation(ctx context.Context, params map[string]interface{}) error {
	operations := []TransactionOperation{
		{
			Service:    "settlement",
			Operation:  "settle_trade",
			Parameters: params,
		},
	}

	_, err := dtt.suite.ExecuteDistributedTransaction(ctx, operations, 30*time.Second)
	return err
}

// executeDistributedTransaction simulates a complex distributed transaction
func (dtt *DistributedTransactionTester) executeDistributedTransaction(ctx context.Context, params map[string]interface{}) error {
	services, ok := params["services"].([]string)
	if !ok {
		services = []string{"bookkeeper", "trading"}
	}

	operations := make([]TransactionOperation, 0, len(services))
	for _, service := range services {
		operations = append(operations, TransactionOperation{
			Service:   service,
			Operation: "test_operation",
			Parameters: map[string]interface{}{
				"test_id": uuid.New().String(),
				"data":    fmt.Sprintf("test_data_%d", rand.Intn(1000)),
			},
		})
	}

	_, err := dtt.suite.ExecuteDistributedTransaction(ctx, operations, 45*time.Second)
	return err
}

// executeMultiServiceOperation simulates operations across multiple services
func (dtt *DistributedTransactionTester) executeMultiServiceOperation(ctx context.Context, params map[string]interface{}) error {
	opsCount, ok := params["operations_count"].(int)
	if !ok {
		opsCount = 3
	}

	operations := make([]TransactionOperation, 0, opsCount)
	for i := 0; i < opsCount; i++ {
		operations = append(operations, TransactionOperation{
			Service:   fmt.Sprintf("service_%d", i%3),
			Operation: "multi_test",
			Parameters: map[string]interface{}{
				"operation_id": i,
				"test_data":    fmt.Sprintf("data_%d", rand.Intn(100)),
			},
		})
	}

	_, err := dtt.suite.ExecuteDistributedTransaction(ctx, operations, 60*time.Second)
	return err
}

// Helper methods for metrics calculation
func (dtt *DistributedTransactionTester) calculateAverageLatency(latencies []time.Duration) time.Duration {
	if len(latencies) == 0 {
		return 0
	}

	total := time.Duration(0)
	for _, latency := range latencies {
		total += latency
	}
	return total / time.Duration(len(latencies))
}

func (dtt *DistributedTransactionTester) findMaxLatency(latencies []time.Duration) time.Duration {
	if len(latencies) == 0 {
		return 0
	}

	max := latencies[0]
	for _, latency := range latencies[1:] {
		if latency > max {
			max = latency
		}
	}
	return max
}

func (dtt *DistributedTransactionTester) findMinLatency(latencies []time.Duration) time.Duration {
	if len(latencies) == 0 {
		return 0
	}

	min := latencies[0]
	for _, latency := range latencies[1:] {
		if latency < min {
			min = latency
		}
	}
	return min
}

// updateTestingMetrics updates the internal testing metrics
func (dtt *DistributedTransactionTester) updateTestingMetrics(result *TestResult) {
	dtt.metrics.mu.Lock()
	defer dtt.metrics.mu.Unlock()

	dtt.metrics.TotalTests++
	if result.Success {
		dtt.metrics.PassedTests++
	} else {
		dtt.metrics.FailedTests++
	}

	if result.ChaosEnabled {
		dtt.metrics.ChaosTests++
	} else {
		dtt.metrics.LoadTests++
	}

	dtt.metrics.AverageLatency = result.AverageLatency
	if result.MaxLatency > dtt.metrics.MaxLatency {
		dtt.metrics.MaxLatency = result.MaxLatency
	}
	if dtt.metrics.MinLatency == 0 || result.MinLatency < dtt.metrics.MinLatency {
		dtt.metrics.MinLatency = result.MinLatency
	}

	dtt.metrics.LastTestRun = result.EndTime
	dtt.metrics.ErrorRate = result.ErrorRate
	dtt.metrics.ThroughputOps = result.ThroughputOps
}

// GetTestingMetrics returns current testing metrics
func (dtt *DistributedTransactionTester) GetTestingMetrics() *TestingMetrics {
	dtt.metrics.mu.RLock()
	defer dtt.metrics.mu.RUnlock()

	// Return a copy to avoid race conditions
	return &TestingMetrics{
		TotalTests:     dtt.metrics.TotalTests,
		PassedTests:    dtt.metrics.PassedTests,
		FailedTests:    dtt.metrics.FailedTests,
		ChaosTests:     dtt.metrics.ChaosTests,
		LoadTests:      dtt.metrics.LoadTests,
		AverageLatency: dtt.metrics.AverageLatency,
		MaxLatency:     dtt.metrics.MaxLatency,
		MinLatency:     dtt.metrics.MinLatency,
		LastTestRun:    dtt.metrics.LastTestRun,
		ErrorRate:      dtt.metrics.ErrorRate,
		ThroughputOps:  dtt.metrics.ThroughputOps,
	}
}
