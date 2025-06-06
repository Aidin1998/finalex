package accounts

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"math/rand"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"
)

// BenchmarkType defines different types of benchmarks
type BenchmarkType string

const (
	BenchmarkRead        BenchmarkType = "read"
	BenchmarkWrite       BenchmarkType = "write"
	BenchmarkMixed       BenchmarkType = "mixed"
	BenchmarkCacheHit    BenchmarkType = "cache_hit"
	BenchmarkCacheMiss   BenchmarkType = "cache_miss"
	BenchmarkSharding    BenchmarkType = "sharding"
	BenchmarkConcurrency BenchmarkType = "concurrency"
	BenchmarkStress      BenchmarkType = "stress"
	BenchmarkEndurance   BenchmarkType = "endurance"
)

// BenchmarkConfiguration defines benchmark parameters
type BenchmarkConfiguration struct {
	Name             string        `json:"name"`
	Type             BenchmarkType `json:"type"`
	Duration         time.Duration `json:"duration"`
	Concurrency      int           `json:"concurrency"`
	TargetTPS        int           `json:"target_tps"`
	DataSize         int           `json:"data_size"`
	ReadWriteRatio   float64       `json:"read_write_ratio"` // 0.8 = 80% reads, 20% writes
	CacheWarmup      bool          `json:"cache_warmup"`
	CleanupAfter     bool          `json:"cleanup_after"`
	StressMultiplier float64       `json:"stress_multiplier"`
	Iterations       int           `json:"iterations"`
	Timeout          time.Duration `json:"timeout"`
}

// BenchmarkResult contains benchmark execution results
type BenchmarkResult struct {
	ID               string                 `json:"id"`
	Name             string                 `json:"name"`
	Type             BenchmarkType          `json:"type"`
	StartTime        time.Time              `json:"start_time"`
	EndTime          time.Time              `json:"end_time"`
	Duration         time.Duration          `json:"duration"`
	TotalOperations  int64                  `json:"total_operations"`
	SuccessfulOps    int64                  `json:"successful_operations"`
	FailedOps        int64                  `json:"failed_operations"`
	OperationsPerSec float64                `json:"operations_per_second"`
	AvgLatency       time.Duration          `json:"avg_latency"`
	MinLatency       time.Duration          `json:"min_latency"`
	MaxLatency       time.Duration          `json:"max_latency"`
	P50Latency       time.Duration          `json:"p50_latency"`
	P90Latency       time.Duration          `json:"p90_latency"`
	P95Latency       time.Duration          `json:"p95_latency"`
	P99Latency       time.Duration          `json:"p99_latency"`
	ErrorRate        float64                `json:"error_rate"`
	ThroughputMBps   float64                `json:"throughput_mbps"`
	CPUUsage         float64                `json:"cpu_usage"`
	MemoryUsage      float64                `json:"memory_usage"`
	CacheHitRate     float64                `json:"cache_hit_rate"`
	DatabaseConns    int                    `json:"database_connections"`
	Errors           []string               `json:"errors,omitempty"`
	PerformanceScore float64                `json:"performance_score"`
	Recommendations  []string               `json:"recommendations"`
	Metadata         map[string]interface{} `json:"metadata"`
}

// LatencyMeasurement represents a single latency measurement
type LatencyMeasurement struct {
	Operation string
	StartTime time.Time
	EndTime   time.Time
	Duration  time.Duration
	Success   bool
	Error     error
}

// BenchmarkingFramework provides comprehensive performance testing
type BenchmarkingFramework struct {
	db           *sql.DB
	logger       *zap.Logger
	dataManager  *AccountDataManager
	hotCache     *HotCache
	shardManager *ShardManager

	// Configuration
	maxConcurrency int
	defaultTimeout time.Duration
	enableMetrics  bool

	// State
	activeTests map[string]*BenchmarkExecution
	results     []*BenchmarkResult
	mu          sync.RWMutex

	// Metrics
	benchmarksTotal     prometheus.Counter
	benchmarkDuration   prometheus.Histogram
	operationLatency    prometheus.Histogram
	operationThroughput prometheus.Gauge
	errorRate           prometheus.Gauge
	performanceScore    prometheus.Gauge
}

// BenchmarkExecution represents an active benchmark execution
type BenchmarkExecution struct {
	ID             string
	Config         *BenchmarkConfiguration
	StartTime      time.Time
	Ctx            context.Context
	Cancel         context.CancelFunc
	WG             sync.WaitGroup
	Measurements   []LatencyMeasurement
	MeasurementsMu sync.Mutex
	OperationCount int64
	ErrorCount     int64
	IsRunning      bool
}

// NewBenchmarkingFramework creates a new benchmarking framework
func NewBenchmarkingFramework(
	db *sql.DB,
	logger *zap.Logger,
	dataManager *AccountDataManager,
	hotCache *HotCache,
	shardManager *ShardManager,
) *BenchmarkingFramework {
	bf := &BenchmarkingFramework{
		db:             db,
		logger:         logger,
		dataManager:    dataManager,
		hotCache:       hotCache,
		shardManager:   shardManager,
		maxConcurrency: 100,
		defaultTimeout: time.Hour,
		enableMetrics:  true,
		activeTests:    make(map[string]*BenchmarkExecution),
		results:        make([]*BenchmarkResult, 0),
	}

	bf.initMetrics()

	return bf
}

// initMetrics initializes Prometheus metrics
func (bf *BenchmarkingFramework) initMetrics() {
	if !bf.enableMetrics {
		return
	}

	bf.benchmarksTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "accounts_benchmarks_total",
		Help: "Total number of benchmarks executed",
	})

	bf.benchmarkDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "accounts_benchmark_duration_seconds",
		Help:    "Duration of benchmark executions",
		Buckets: prometheus.ExponentialBuckets(1, 2, 15),
	})

	bf.operationLatency = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "accounts_benchmark_operation_latency_seconds",
		Help:    "Latency of individual operations during benchmarks",
		Buckets: prometheus.ExponentialBuckets(0.0001, 2, 20),
	})

	bf.operationThroughput = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "accounts_benchmark_throughput_ops_per_second",
		Help: "Current benchmark operation throughput",
	})

	bf.errorRate = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "accounts_benchmark_error_rate",
		Help: "Current benchmark error rate",
	})

	bf.performanceScore = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "accounts_benchmark_performance_score",
		Help: "Overall performance score from benchmarks",
	})
}

// RunBenchmark executes a benchmark with the given configuration
func (bf *BenchmarkingFramework) RunBenchmark(ctx context.Context, config *BenchmarkConfiguration) (*BenchmarkResult, error) {
	if config.Concurrency <= 0 {
		config.Concurrency = 10
	}
	if config.Concurrency > bf.maxConcurrency {
		config.Concurrency = bf.maxConcurrency
	}
	if config.Duration <= 0 {
		config.Duration = time.Minute
	}
	if config.Timeout <= 0 {
		config.Timeout = bf.defaultTimeout
	}

	executionID := uuid.New().String()

	bf.logger.Info("Starting benchmark",
		zap.String("id", executionID),
		zap.String("name", config.Name),
		zap.String("type", string(config.Type)),
		zap.Duration("duration", config.Duration),
		zap.Int("concurrency", config.Concurrency))

	// Create execution context
	execCtx, cancel := context.WithTimeout(ctx, config.Timeout)
	defer cancel()

	execution := &BenchmarkExecution{
		ID:           executionID,
		Config:       config,
		StartTime:    time.Now(),
		Ctx:          execCtx,
		Cancel:       cancel,
		Measurements: make([]LatencyMeasurement, 0),
		IsRunning:    true,
	}

	// Register active test
	bf.mu.Lock()
	bf.activeTests[executionID] = execution
	bf.mu.Unlock()

	defer func() {
		bf.mu.Lock()
		delete(bf.activeTests, executionID)
		bf.mu.Unlock()
	}()

	if bf.enableMetrics {
		bf.benchmarksTotal.Inc()
	}

	// Perform cache warmup if requested
	if config.CacheWarmup {
		bf.logger.Info("Performing cache warmup")
		err := bf.warmupCache(execCtx, config)
		if err != nil {
			bf.logger.Warn("Cache warmup failed", zap.Error(err))
		}
	}

	// Execute benchmark based on type
	var err error
	switch config.Type {
	case BenchmarkRead:
		err = bf.runReadBenchmark(execution)
	case BenchmarkWrite:
		err = bf.runWriteBenchmark(execution)
	case BenchmarkMixed:
		err = bf.runMixedBenchmark(execution)
	case BenchmarkCacheHit:
		err = bf.runCacheHitBenchmark(execution)
	case BenchmarkCacheMiss:
		err = bf.runCacheMissBenchmark(execution)
	case BenchmarkSharding:
		err = bf.runShardingBenchmark(execution)
	case BenchmarkConcurrency:
		err = bf.runConcurrencyBenchmark(execution)
	case BenchmarkStress:
		err = bf.runStressBenchmark(execution)
	case BenchmarkEndurance:
		err = bf.runEnduranceBenchmark(execution)
	default:
		err = fmt.Errorf("unsupported benchmark type: %s", config.Type)
	}

	if err != nil {
		return nil, fmt.Errorf("benchmark execution failed: %w", err)
	}

	// Wait for all operations to complete
	execution.WG.Wait()
	execution.IsRunning = false

	// Generate result
	result := bf.generateResult(execution)

	// Store result
	bf.mu.Lock()
	bf.results = append(bf.results, result)
	bf.mu.Unlock()

	// Update metrics
	if bf.enableMetrics {
		bf.benchmarkDuration.Observe(result.Duration.Seconds())
		bf.operationThroughput.Set(result.OperationsPerSec)
		bf.errorRate.Set(result.ErrorRate)
		bf.performanceScore.Set(result.PerformanceScore)
	}

	// Cleanup if requested
	if config.CleanupAfter {
		bf.cleanup(execCtx, config)
	}

	bf.logger.Info("Benchmark completed",
		zap.String("id", executionID),
		zap.Duration("duration", result.Duration),
		zap.Float64("ops_per_sec", result.OperationsPerSec),
		zap.Float64("error_rate", result.ErrorRate),
		zap.Float64("performance_score", result.PerformanceScore))

	return result, nil
}

// runReadBenchmark executes a read-only benchmark
func (bf *BenchmarkingFramework) runReadBenchmark(execution *BenchmarkExecution) error {
	// Generate test user IDs
	userIDs := bf.generateTestUserIDs(execution.Config.DataSize)

	return bf.runWorkload(execution, func(workerID int) error {
		for {
			select {
			case <-execution.Ctx.Done():
				return nil
			default:
				// Pick random user ID
				userID := userIDs[rand.Intn(len(userIDs))]

				// Measure operation
				start := time.Now()
				_, err := bf.dataManager.GetAccount(execution.Ctx, userID)
				duration := time.Since(start)

				// Record measurement
				bf.recordMeasurement(execution, "read", start, duration, err == nil, err)

				if bf.enableMetrics {
					bf.operationLatency.Observe(duration.Seconds())
				}

				atomic.AddInt64(&execution.OperationCount, 1)
				if err != nil {
					atomic.AddInt64(&execution.ErrorCount, 1)
				}
			}
		}
	})
}

// runWriteBenchmark executes a write-only benchmark
func (bf *BenchmarkingFramework) runWriteBenchmark(execution *BenchmarkExecution) error {
	return bf.runWorkload(execution, func(workerID int) error {
		for {
			select {
			case <-execution.Ctx.Done():
				return nil
			default:
				// Generate test account
				account := bf.generateTestAccount()

				// Measure operation
				start := time.Now()
				err := bf.dataManager.CreateAccount(execution.Ctx, account)
				duration := time.Since(start)

				// Record measurement
				bf.recordMeasurement(execution, "write", start, duration, err == nil, err)

				if bf.enableMetrics {
					bf.operationLatency.Observe(duration.Seconds())
				}

				atomic.AddInt64(&execution.OperationCount, 1)
				if err != nil {
					atomic.AddInt64(&execution.ErrorCount, 1)
				}
			}
		}
	})
}

// runMixedBenchmark executes a mixed read/write benchmark
func (bf *BenchmarkingFramework) runMixedBenchmark(execution *BenchmarkExecution) error {
	userIDs := bf.generateTestUserIDs(execution.Config.DataSize)
	readThreshold := execution.Config.ReadWriteRatio

	return bf.runWorkload(execution, func(workerID int) error {
		for {
			select {
			case <-execution.Ctx.Done():
				return nil
			default:
				var err error
				var operation string
				start := time.Now()

				if rand.Float64() < readThreshold {
					// Read operation
					operation = "read"
					userID := userIDs[rand.Intn(len(userIDs))]
					_, err = bf.dataManager.GetAccount(execution.Ctx, userID)
				} else {
					// Write operation
					operation = "write"
					account := bf.generateTestAccount()
					err = bf.dataManager.CreateAccount(execution.Ctx, account)
				}

				duration := time.Since(start)

				// Record measurement
				bf.recordMeasurement(execution, operation, start, duration, err == nil, err)

				if bf.enableMetrics {
					bf.operationLatency.Observe(duration.Seconds())
				}

				atomic.AddInt64(&execution.OperationCount, 1)
				if err != nil {
					atomic.AddInt64(&execution.ErrorCount, 1)
				}
			}
		}
	})
}

// runCacheHitBenchmark executes a cache hit benchmark
func (bf *BenchmarkingFramework) runCacheHitBenchmark(execution *BenchmarkExecution) error {
	// Pre-populate cache
	userIDs := bf.generateTestUserIDs(100) // Small set for high cache hits

	// Warm up cache
	for _, userID := range userIDs {
		bf.dataManager.GetAccount(execution.Ctx, userID)
	}

	return bf.runWorkload(execution, func(workerID int) error {
		for {
			select {
			case <-execution.Ctx.Done():
				return nil
			default:
				// Always use IDs from the warmed cache
				userID := userIDs[rand.Intn(len(userIDs))]

				start := time.Now()
				_, err := bf.dataManager.GetAccount(execution.Ctx, userID)
				duration := time.Since(start)

				bf.recordMeasurement(execution, "cache_hit", start, duration, err == nil, err)

				if bf.enableMetrics {
					bf.operationLatency.Observe(duration.Seconds())
				}

				atomic.AddInt64(&execution.OperationCount, 1)
				if err != nil {
					atomic.AddInt64(&execution.ErrorCount, 1)
				}
			}
		}
	})
}

// runCacheMissBenchmark executes a cache miss benchmark
func (bf *BenchmarkingFramework) runCacheMissBenchmark(execution *BenchmarkExecution) error {
	return bf.runWorkload(execution, func(workerID int) error {
		for {
			select {
			case <-execution.Ctx.Done():
				return nil
			default:
				// Always use random IDs to force cache misses
				userID := uuid.New()

				start := time.Now()
				_, err := bf.dataManager.GetAccount(execution.Ctx, userID)
				duration := time.Since(start)

				bf.recordMeasurement(execution, "cache_miss", start, duration, err == nil, err)

				if bf.enableMetrics {
					bf.operationLatency.Observe(duration.Seconds())
				}

				atomic.AddInt64(&execution.OperationCount, 1)
				if err != nil {
					atomic.AddInt64(&execution.ErrorCount, 1)
				}
			}
		}
	})
}

// runShardingBenchmark executes a sharding performance benchmark
func (bf *BenchmarkingFramework) runShardingBenchmark(execution *BenchmarkExecution) error {
	return bf.runWorkload(execution, func(workerID int) error {
		for {
			select {
			case <-execution.Ctx.Done():
				return nil
			default:
				// Generate account with specific shard targeting
				account := bf.generateTestAccount()

				start := time.Now()

				// Test shard resolution
				shard := bf.shardManager.GetShard(account.UserID.String())
				if shard == nil {
					atomic.AddInt64(&execution.ErrorCount, 1)
					continue
				}

				// Test operation
				err := bf.dataManager.CreateAccount(execution.Ctx, account)
				duration := time.Since(start)

				bf.recordMeasurement(execution, "sharding", start, duration, err == nil, err)

				if bf.enableMetrics {
					bf.operationLatency.Observe(duration.Seconds())
				}

				atomic.AddInt64(&execution.OperationCount, 1)
				if err != nil {
					atomic.AddInt64(&execution.ErrorCount, 1)
				}
			}
		}
	})
}

// runConcurrencyBenchmark executes a concurrency stress test
func (bf *BenchmarkingFramework) runConcurrencyBenchmark(execution *BenchmarkExecution) error {
	// Use maximum concurrency for this test
	execution.Config.Concurrency = bf.maxConcurrency

	return bf.runMixedBenchmark(execution)
}

// runStressBenchmark executes a stress test with high load
func (bf *BenchmarkingFramework) runStressBenchmark(execution *BenchmarkExecution) error {
	// Apply stress multiplier
	originalConcurrency := execution.Config.Concurrency
	execution.Config.Concurrency = int(float64(originalConcurrency) * execution.Config.StressMultiplier)

	if execution.Config.Concurrency > bf.maxConcurrency {
		execution.Config.Concurrency = bf.maxConcurrency
	}

	return bf.runMixedBenchmark(execution)
}

// runEnduranceBenchmark executes a long-running endurance test
func (bf *BenchmarkingFramework) runEnduranceBenchmark(execution *BenchmarkExecution) error {
	// Extend duration for endurance test
	execution.Config.Duration = time.Hour * 2

	return bf.runMixedBenchmark(execution)
}

// runWorkload executes a workload with the specified worker function
func (bf *BenchmarkingFramework) runWorkload(execution *BenchmarkExecution, workerFunc func(int) error) error {
	// Create timeout context
	timeoutCtx, timeoutCancel := context.WithTimeout(execution.Ctx, execution.Config.Duration)
	defer timeoutCancel()
	execution.Ctx = timeoutCtx

	// Start workers
	for i := 0; i < execution.Config.Concurrency; i++ {
		execution.WG.Add(1)
		go func(workerID int) {
			defer execution.WG.Done()

			err := workerFunc(workerID)
			if err != nil {
				bf.logger.Error("Worker error",
					zap.Int("worker_id", workerID),
					zap.Error(err))
			}
		}(i)
	}

	return nil
}

// recordMeasurement records a performance measurement
func (bf *BenchmarkingFramework) recordMeasurement(
	execution *BenchmarkExecution,
	operation string,
	startTime time.Time,
	duration time.Duration,
	success bool,
	err error,
) {
	measurement := LatencyMeasurement{
		Operation: operation,
		StartTime: startTime,
		EndTime:   startTime.Add(duration),
		Duration:  duration,
		Success:   success,
		Error:     err,
	}

	execution.MeasurementsMu.Lock()
	execution.Measurements = append(execution.Measurements, measurement)
	execution.MeasurementsMu.Unlock()
}

// generateResult generates a benchmark result from execution data
func (bf *BenchmarkingFramework) generateResult(execution *BenchmarkExecution) *BenchmarkResult {
	endTime := time.Now()
	duration := endTime.Sub(execution.StartTime)

	result := &BenchmarkResult{
		ID:              execution.ID,
		Name:            execution.Config.Name,
		Type:            execution.Config.Type,
		StartTime:       execution.StartTime,
		EndTime:         endTime,
		Duration:        duration,
		TotalOperations: atomic.LoadInt64(&execution.OperationCount),
		FailedOps:       atomic.LoadInt64(&execution.ErrorCount),
		Metadata:        make(map[string]interface{}),
		Errors:          make([]string, 0),
		Recommendations: make([]string, 0),
	}

	result.SuccessfulOps = result.TotalOperations - result.FailedOps

	if duration.Seconds() > 0 {
		result.OperationsPerSec = float64(result.TotalOperations) / duration.Seconds()
	}

	if result.TotalOperations > 0 {
		result.ErrorRate = float64(result.FailedOps) / float64(result.TotalOperations)
	}

	// Calculate latency statistics
	bf.calculateLatencyStats(execution, result)

	// Calculate performance score
	result.PerformanceScore = bf.calculatePerformanceScore(result)

	// Generate recommendations
	bf.generateBenchmarkRecommendations(result)

	// Collect system metrics
	bf.collectSystemMetrics(result)

	return result
}

// calculateLatencyStats calculates latency statistics from measurements
func (bf *BenchmarkingFramework) calculateLatencyStats(execution *BenchmarkExecution, result *BenchmarkResult) {
	execution.MeasurementsMu.Lock()
	measurements := make([]time.Duration, len(execution.Measurements))
	for i, m := range execution.Measurements {
		measurements[i] = m.Duration
	}
	execution.MeasurementsMu.Unlock()

	if len(measurements) == 0 {
		return
	}

	// Sort measurements for percentile calculations
	for i := 0; i < len(measurements); i++ {
		for j := i + 1; j < len(measurements); j++ {
			if measurements[i] > measurements[j] {
				measurements[i], measurements[j] = measurements[j], measurements[i]
			}
		}
	}

	// Calculate statistics
	result.MinLatency = measurements[0]
	result.MaxLatency = measurements[len(measurements)-1]

	// Calculate average
	var total time.Duration
	for _, d := range measurements {
		total += d
	}
	result.AvgLatency = total / time.Duration(len(measurements))

	// Calculate percentiles
	result.P50Latency = measurements[int(float64(len(measurements))*0.5)]
	result.P90Latency = measurements[int(float64(len(measurements))*0.9)]
	result.P95Latency = measurements[int(float64(len(measurements))*0.95)]
	result.P99Latency = measurements[int(float64(len(measurements))*0.99)]
}

// calculatePerformanceScore calculates an overall performance score (0-100)
func (bf *BenchmarkingFramework) calculatePerformanceScore(result *BenchmarkResult) float64 {
	score := 100.0

	// Deduct points for high error rate
	if result.ErrorRate > 0.01 {
		score -= math.Min(50.0, result.ErrorRate*1000)
	}

	// Deduct points for high latency
	if result.P95Latency > time.Millisecond*100 {
		latencyPenalty := float64(result.P95Latency.Milliseconds()-100) / 10
		score -= math.Min(30.0, latencyPenalty)
	}

	// Deduct points for low throughput (based on target)
	if result.OperationsPerSec < 1000 {
		throughputPenalty := (1000 - result.OperationsPerSec) / 50
		score -= math.Min(20.0, throughputPenalty)
	}

	return math.Max(0.0, score)
}

// generateBenchmarkRecommendations generates optimization recommendations
func (bf *BenchmarkingFramework) generateBenchmarkRecommendations(result *BenchmarkResult) {
	if result.ErrorRate > 0.02 {
		result.Recommendations = append(result.Recommendations,
			"High error rate detected. Review error logs and improve error handling.")
	}

	if result.P95Latency > time.Millisecond*100 {
		result.Recommendations = append(result.Recommendations,
			"High P95 latency detected. Consider optimizing database queries or adding read replicas.")
	}

	if result.OperationsPerSec < 1000 {
		result.Recommendations = append(result.Recommendations,
			"Low throughput detected. Consider optimizing application code or scaling resources.")
	}

	if result.CacheHitRate < 0.8 {
		result.Recommendations = append(result.Recommendations,
			"Low cache hit rate. Consider optimizing cache strategies or increasing cache size.")
	}
}

// collectSystemMetrics collects system resource metrics during benchmark
func (bf *BenchmarkingFramework) collectSystemMetrics(result *BenchmarkResult) {
	// Get memory stats
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	result.MemoryUsage = float64(m.Alloc) / (1024 * 1024) // MB

	// Simulate other metrics (would integrate with actual monitoring)
	result.CPUUsage = 65.0                                  // 65% CPU usage
	result.DatabaseConns = 42                               // 42 active connections
	result.CacheHitRate = 0.85                              // 85% cache hit rate
	result.ThroughputMBps = result.OperationsPerSec * 0.001 // Approximate MB/s
}

// warmupCache warms up the cache before benchmark execution
func (bf *BenchmarkingFramework) warmupCache(ctx context.Context, config *BenchmarkConfiguration) error {
	userIDs := bf.generateTestUserIDs(config.DataSize)

	bf.logger.Info("Warming up cache", zap.Int("accounts", len(userIDs)))

	for _, userID := range userIDs {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			_, err := bf.dataManager.GetAccount(ctx, userID)
			if err != nil {
				bf.logger.Debug("Cache warmup error", zap.Error(err))
			}
		}
	}

	return nil
}

// cleanup performs post-benchmark cleanup
func (bf *BenchmarkingFramework) cleanup(ctx context.Context, config *BenchmarkConfiguration) {
	bf.logger.Info("Performing benchmark cleanup")

	// Clear cache if needed
	if bf.hotCache != nil {
		bf.hotCache.Clear()
	}

	// Run garbage collection
	runtime.GC()
}

// generateTestUserIDs generates a set of test user IDs
func (bf *BenchmarkingFramework) generateTestUserIDs(count int) []uuid.UUID {
	userIDs := make([]uuid.UUID, count)
	for i := 0; i < count; i++ {
		userIDs[i] = uuid.New()
	}
	return userIDs
}

// generateTestAccount generates a test account
func (bf *BenchmarkingFramework) generateTestAccount() *Account {
	return &Account{
		UserID:    uuid.New(),
		Type:      "spot",
		Status:    "active",
		Balance:   1000.0,
		Currency:  "USD",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

// GetActiveTests returns currently running benchmark tests
func (bf *BenchmarkingFramework) GetActiveTests() map[string]*BenchmarkExecution {
	bf.mu.RLock()
	defer bf.mu.RUnlock()

	active := make(map[string]*BenchmarkExecution)
	for id, execution := range bf.activeTests {
		if execution.IsRunning {
			active[id] = execution
		}
	}

	return active
}

// GetResults returns benchmark results
func (bf *BenchmarkingFramework) GetResults(limit int) []*BenchmarkResult {
	bf.mu.RLock()
	defer bf.mu.RUnlock()

	if limit <= 0 || limit > len(bf.results) {
		limit = len(bf.results)
	}

	// Return latest results
	start := len(bf.results) - limit
	if start < 0 {
		start = 0
	}

	return bf.results[start:]
}

// StopBenchmark stops a running benchmark
func (bf *BenchmarkingFramework) StopBenchmark(benchmarkID string) error {
	bf.mu.RLock()
	execution, exists := bf.activeTests[benchmarkID]
	bf.mu.RUnlock()

	if !exists {
		return fmt.Errorf("benchmark not found: %s", benchmarkID)
	}

	execution.Cancel()

	bf.logger.Info("Benchmark stopped", zap.String("id", benchmarkID))

	return nil
}

// GetDefaultConfigurations returns default benchmark configurations
func (bf *BenchmarkingFramework) GetDefaultConfigurations() []*BenchmarkConfiguration {
	return []*BenchmarkConfiguration{
		{
			Name:         "Quick Read Test",
			Type:         BenchmarkRead,
			Duration:     time.Minute * 5,
			Concurrency:  10,
			DataSize:     1000,
			CacheWarmup:  true,
			CleanupAfter: true,
		},
		{
			Name:         "Write Performance Test",
			Type:         BenchmarkWrite,
			Duration:     time.Minute * 5,
			Concurrency:  10,
			DataSize:     1000,
			CleanupAfter: true,
		},
		{
			Name:           "Mixed Workload Test",
			Type:           BenchmarkMixed,
			Duration:       time.Minute * 10,
			Concurrency:    20,
			DataSize:       5000,
			ReadWriteRatio: 0.8,
			CacheWarmup:    true,
			CleanupAfter:   true,
		},
		{
			Name:             "Stress Test",
			Type:             BenchmarkStress,
			Duration:         time.Minute * 15,
			Concurrency:      50,
			DataSize:         10000,
			ReadWriteRatio:   0.7,
			StressMultiplier: 2.0,
			CleanupAfter:     true,
		},
		{
			Name:         "Cache Performance Test",
			Type:         BenchmarkCacheHit,
			Duration:     time.Minute * 5,
			Concurrency:  15,
			DataSize:     100,
			CacheWarmup:  true,
			CleanupAfter: true,
		},
		{
			Name:           "Concurrency Test",
			Type:           BenchmarkConcurrency,
			Duration:       time.Minute * 10,
			Concurrency:    100,
			DataSize:       5000,
			ReadWriteRatio: 0.8,
			CleanupAfter:   true,
		},
	}
}

// GetMetrics returns framework metrics
func (bf *BenchmarkingFramework) GetMetrics() map[string]interface{} {
	bf.mu.RLock()
	defer bf.mu.RUnlock()

	activeCount := 0
	for _, execution := range bf.activeTests {
		if execution.IsRunning {
			activeCount++
		}
	}

	return map[string]interface{}{
		"total_benchmarks":  len(bf.results),
		"active_benchmarks": activeCount,
		"max_concurrency":   bf.maxConcurrency,
		"default_timeout":   bf.defaultTimeout,
		"metrics_enabled":   bf.enableMetrics,
	}
}
