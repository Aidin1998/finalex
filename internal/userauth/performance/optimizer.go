package performance

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/userauth/cache"
	"go.uber.org/zap"
)

// PerformanceOptimizer provides high-performance optimizations for 100k+ RPS
type PerformanceOptimizer struct {
	// Connection pooling
	connectionPools map[string]*ConnectionPool
	poolMutex       sync.RWMutex

	// Request batching
	batchProcessor *BatchProcessor

	// Circuit breaker
	circuitBreaker *CircuitBreaker

	// Local caching
	localCache *cache.ClusteredCache

	// Metrics
	metrics *PerformanceMetrics
	logger  *zap.Logger

	// Configuration
	config *OptimizationConfig
}

// OptimizationConfig holds performance optimization settings
type OptimizationConfig struct {
	// Connection pooling
	MaxConnectionsPerPool int           `yaml:"max_connections_per_pool"`
	ConnectionTimeout     time.Duration `yaml:"connection_timeout"`
	IdleTimeout           time.Duration `yaml:"idle_timeout"`

	// Batching
	BatchSize       int           `yaml:"batch_size"`
	BatchTimeout    time.Duration `yaml:"batch_timeout"`
	MaxBatchWorkers int           `yaml:"max_batch_workers"`

	// Circuit breaker
	FailureThreshold int           `yaml:"failure_threshold"`
	RecoveryTimeout  time.Duration `yaml:"recovery_timeout"`
	HalfOpenRequests int           `yaml:"half_open_requests"`

	// Caching
	CacheSize int           `yaml:"cache_size"`
	CacheTTL  time.Duration `yaml:"cache_ttl"`

	// Runtime optimization
	MaxGoroutines   int `yaml:"max_goroutines"`
	GCTargetPercent int `yaml:"gc_target_percent"`
}

// ConnectionPool manages database/service connections efficiently
type ConnectionPool struct {
	connections chan interface{}
	factory     func() (interface{}, error)
	closer      func(interface{}) error
	maxSize     int
	timeout     time.Duration
	metrics     *PoolMetrics
	mutex       sync.RWMutex
}

// PoolMetrics tracks connection pool performance
type PoolMetrics struct {
	ActiveConnections int64
	TotalConnections  int64
	AcquireCount      int64
	AcquireTime       int64 // nanoseconds
	Errors            int64
}

// BatchProcessor handles request batching for improved throughput
type BatchProcessor struct {
	batches     map[string]*Batch
	batchMutex  sync.RWMutex
	workers     chan *Batch
	config      *OptimizationConfig
	processFunc func([]interface{}) error
	metrics     *BatchMetrics
	stopChan    chan bool
}

// Batch represents a collection of requests to be processed together
type Batch struct {
	Items     []interface{}
	Timestamp time.Time
	Done      chan error
	Key       string
}

// BatchMetrics tracks batching performance
type BatchMetrics struct {
	TotalBatches     int64
	TotalItems       int64
	AverageLatency   int64
	ProcessingErrors int64
}

// CircuitBreaker prevents cascading failures
type CircuitBreaker struct {
	state           int32 // 0: closed, 1: open, 2: half-open
	failures        int32
	requests        int32
	successCount    int32
	lastFailureTime int64
	config          *OptimizationConfig
	mutex           sync.RWMutex
}

// States for circuit breaker
const (
	StateClosed = iota
	StateOpen
	StateHalfOpen
)

// PerformanceMetrics tracks overall system performance
type PerformanceMetrics struct {
	RequestsPerSecond   int64
	AverageResponseTime int64
	ErrorRate           float64
	MemoryUsage         int64
	GoroutineCount      int64
	LastUpdated         time.Time
	mutex               sync.RWMutex
}

// NewPerformanceOptimizer creates a new performance optimizer
func NewPerformanceOptimizer(config *OptimizationConfig, logger *zap.Logger) *PerformanceOptimizer {
	po := &PerformanceOptimizer{
		connectionPools: make(map[string]*ConnectionPool),
		config:          config,
		logger:          logger,
		metrics:         &PerformanceMetrics{},
	}

	// Initialize circuit breaker
	po.circuitBreaker = &CircuitBreaker{
		config: config,
	}

	// Initialize batch processor
	po.batchProcessor = &BatchProcessor{
		batches:  make(map[string]*Batch),
		workers:  make(chan *Batch, config.MaxBatchWorkers),
		config:   config,
		metrics:  &BatchMetrics{},
		stopChan: make(chan bool),
	}

	// Optimize runtime settings
	po.optimizeRuntime()

	// Start background workers
	po.startBackgroundWorkers()

	return po
}

// CreateConnectionPool creates a new connection pool
func (po *PerformanceOptimizer) CreateConnectionPool(name string, factory func() (interface{}, error), closer func(interface{}) error) {
	pool := &ConnectionPool{
		connections: make(chan interface{}, po.config.MaxConnectionsPerPool),
		factory:     factory,
		closer:      closer,
		maxSize:     po.config.MaxConnectionsPerPool,
		timeout:     po.config.ConnectionTimeout,
		metrics:     &PoolMetrics{},
	}

	// Pre-fill pool with initial connections
	for i := 0; i < po.config.MaxConnectionsPerPool/2; i++ {
		if conn, err := factory(); err == nil {
			select {
			case pool.connections <- conn:
				atomic.AddInt64(&pool.metrics.TotalConnections, 1)
			default:
				closer(conn)
			}
		}
	}

	po.poolMutex.Lock()
	po.connectionPools[name] = pool
	po.poolMutex.Unlock()

	po.logger.Info("Connection pool created",
		zap.String("name", name),
		zap.Int("initial_size", len(pool.connections)),
		zap.Int("max_size", pool.maxSize),
	)
}

// AcquireConnection gets a connection from the pool
func (po *PerformanceOptimizer) AcquireConnection(poolName string) (interface{}, error) {
	po.poolMutex.RLock()
	pool, exists := po.connectionPools[poolName]
	po.poolMutex.RUnlock()

	if !exists {
		return nil, ErrPoolNotFound
	}

	start := time.Now()
	defer func() {
		duration := time.Since(start).Nanoseconds()
		atomic.AddInt64(&pool.metrics.AcquireTime, duration)
		atomic.AddInt64(&pool.metrics.AcquireCount, 1)
	}()

	// Try to get existing connection
	select {
	case conn := <-pool.connections:
		atomic.AddInt64(&pool.metrics.ActiveConnections, 1)
		return conn, nil
	case <-time.After(pool.timeout):
		// Timeout - try to create new connection
		if atomic.LoadInt64(&pool.metrics.TotalConnections) < int64(pool.maxSize) {
			conn, err := pool.factory()
			if err != nil {
				atomic.AddInt64(&pool.metrics.Errors, 1)
				return nil, err
			}
			atomic.AddInt64(&pool.metrics.TotalConnections, 1)
			atomic.AddInt64(&pool.metrics.ActiveConnections, 1)
			return conn, nil
		}
		atomic.AddInt64(&pool.metrics.Errors, 1)
		return nil, ErrPoolTimeout
	}
}

// ReleaseConnection returns a connection to the pool
func (po *PerformanceOptimizer) ReleaseConnection(poolName string, conn interface{}) error {
	po.poolMutex.RLock()
	pool, exists := po.connectionPools[poolName]
	po.poolMutex.RUnlock()

	if !exists {
		return ErrPoolNotFound
	}

	atomic.AddInt64(&pool.metrics.ActiveConnections, -1)

	select {
	case pool.connections <- conn:
		return nil
	default:
		// Pool is full, close the connection
		if pool.closer != nil {
			return pool.closer(conn)
		}
		return nil
	}
}

// AddToBatch adds an item to a processing batch
func (po *PerformanceOptimizer) AddToBatch(key string, item interface{}) chan error {
	po.batchProcessor.batchMutex.Lock()
	defer po.batchProcessor.batchMutex.Unlock()

	batch, exists := po.batchProcessor.batches[key]
	if !exists {
		batch = &Batch{
			Items:     make([]interface{}, 0, po.config.BatchSize),
			Timestamp: time.Now(),
			Done:      make(chan error, 1),
			Key:       key,
		}
		po.batchProcessor.batches[key] = batch
	}

	batch.Items = append(batch.Items, item)
	done := batch.Done

	// Check if batch should be processed
	if len(batch.Items) >= po.config.BatchSize ||
		time.Since(batch.Timestamp) >= po.config.BatchTimeout {
		po.processBatch(batch)
		delete(po.batchProcessor.batches, key)
	}

	return done
}

// processBatch sends a batch for processing
func (po *PerformanceOptimizer) processBatch(batch *Batch) {
	select {
	case po.batchProcessor.workers <- batch:
		// Batch queued for processing
	default:
		// Workers are busy, process synchronously
		go po.processBatchSync(batch)
	}
}

// processBatchSync processes a batch synchronously
func (po *PerformanceOptimizer) processBatchSync(batch *Batch) {
	start := time.Now()

	var err error
	if po.batchProcessor.processFunc != nil {
		err = po.batchProcessor.processFunc(batch.Items)
	}

	// Update metrics
	atomic.AddInt64(&po.batchProcessor.metrics.TotalBatches, 1)
	atomic.AddInt64(&po.batchProcessor.metrics.TotalItems, int64(len(batch.Items)))

	latency := time.Since(start).Nanoseconds()
	atomic.AddInt64(&po.batchProcessor.metrics.AverageLatency, latency)

	if err != nil {
		atomic.AddInt64(&po.batchProcessor.metrics.ProcessingErrors, 1)
	}

	// Notify completion
	select {
	case batch.Done <- err:
	default:
	}
}

// ExecuteWithCircuitBreaker executes a function with circuit breaker protection
func (po *PerformanceOptimizer) ExecuteWithCircuitBreaker(fn func() error) error {
	cb := po.circuitBreaker

	// Check circuit breaker state
	if !cb.allowRequest() {
		return ErrCircuitOpen
	}

	// Execute function
	err := fn()

	// Update circuit breaker state
	if err != nil {
		cb.recordFailure()
	} else {
		cb.recordSuccess()
	}

	return err
}

// allowRequest checks if circuit breaker allows the request
func (cb *CircuitBreaker) allowRequest() bool {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	state := atomic.LoadInt32(&cb.state)
	now := time.Now().UnixNano()

	switch state {
	case StateClosed:
		return true
	case StateOpen:
		// Check if we should move to half-open
		if now-atomic.LoadInt64(&cb.lastFailureTime) > cb.config.RecoveryTimeout.Nanoseconds() {
			atomic.StoreInt32(&cb.state, StateHalfOpen)
			atomic.StoreInt32(&cb.requests, 0)
			atomic.StoreInt32(&cb.successCount, 0)
			return true
		}
		return false
	case StateHalfOpen:
		// Allow limited requests in half-open state
		return atomic.LoadInt32(&cb.requests) < int32(cb.config.HalfOpenRequests)
	}

	return false
}

// recordFailure records a failure in circuit breaker
func (cb *CircuitBreaker) recordFailure() {
	atomic.AddInt32(&cb.failures, 1)
	atomic.AddInt32(&cb.requests, 1)
	atomic.StoreInt64(&cb.lastFailureTime, time.Now().UnixNano())

	state := atomic.LoadInt32(&cb.state)
	failures := atomic.LoadInt32(&cb.failures)

	if state == StateClosed && failures >= int32(cb.config.FailureThreshold) {
		atomic.StoreInt32(&cb.state, StateOpen)
	} else if state == StateHalfOpen {
		atomic.StoreInt32(&cb.state, StateOpen)
	}
}

// recordSuccess records a success in circuit breaker
func (cb *CircuitBreaker) recordSuccess() {
	atomic.AddInt32(&cb.requests, 1)

	state := atomic.LoadInt32(&cb.state)
	if state == StateHalfOpen {
		successCount := atomic.AddInt32(&cb.successCount, 1)
		if successCount >= int32(cb.config.HalfOpenRequests) {
			atomic.StoreInt32(&cb.state, StateClosed)
			atomic.StoreInt32(&cb.failures, 0)
		}
	} else if state == StateClosed {
		// Reset failure count on success
		atomic.StoreInt32(&cb.failures, 0)
	}
}

// optimizeRuntime configures Go runtime for high performance
func (po *PerformanceOptimizer) optimizeRuntime() {
	// Set GOMAXPROCS to number of CPU cores
	if po.config.MaxGoroutines > 0 {
		runtime.GOMAXPROCS(po.config.MaxGoroutines)
	} else {
		runtime.GOMAXPROCS(runtime.NumCPU())
	}

	// Optimize garbage collector
	if po.config.GCTargetPercent > 0 {
		runtime.SetGCPercent(po.config.GCTargetPercent)
	} else {
		runtime.SetGCPercent(100) // Default optimized for throughput
	}

	po.logger.Info("Runtime optimized",
		zap.Int("gomaxprocs", runtime.GOMAXPROCS(-1)),
		zap.Int("gc_target_percent", po.config.GCTargetPercent),
	)
}

// startBackgroundWorkers starts background optimization workers
func (po *PerformanceOptimizer) startBackgroundWorkers() {
	// Batch processing workers
	for i := 0; i < po.config.MaxBatchWorkers; i++ {
		go po.batchWorker()
	}

	// Metrics collection worker
	go po.metricsWorker()

	// Batch timeout worker
	go po.batchTimeoutWorker()

	po.logger.Info("Background workers started",
		zap.Int("batch_workers", po.config.MaxBatchWorkers),
	)
}

// batchWorker processes batches from the worker queue
func (po *PerformanceOptimizer) batchWorker() {
	for {
		select {
		case batch := <-po.batchProcessor.workers:
			po.processBatchSync(batch)
		case <-po.batchProcessor.stopChan:
			return
		}
	}
}

// metricsWorker collects performance metrics
func (po *PerformanceOptimizer) metricsWorker() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	var lastRequests int64

	for {
		select {
		case <-ticker.C:
			po.updateMetrics(lastRequests)
		case <-po.batchProcessor.stopChan:
			return
		}
	}
}

// updateMetrics updates performance metrics
func (po *PerformanceOptimizer) updateMetrics(lastRequests int64) {
	po.metrics.mutex.Lock()
	defer po.metrics.mutex.Unlock()

	// Calculate requests per second
	totalRequests := atomic.LoadInt64(&po.batchProcessor.metrics.TotalItems)
	po.metrics.RequestsPerSecond = totalRequests - lastRequests

	// Update memory usage
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	po.metrics.MemoryUsage = int64(memStats.Alloc)

	// Update goroutine count
	po.metrics.GoroutineCount = int64(runtime.NumGoroutine())

	// Update average response time
	if po.batchProcessor.metrics.TotalBatches > 0 {
		po.metrics.AverageResponseTime = atomic.LoadInt64(&po.batchProcessor.metrics.AverageLatency) /
			atomic.LoadInt64(&po.batchProcessor.metrics.TotalBatches)
	}

	// Calculate error rate
	totalErrors := atomic.LoadInt64(&po.batchProcessor.metrics.ProcessingErrors)
	if totalRequests > 0 {
		po.metrics.ErrorRate = float64(totalErrors) / float64(totalRequests) * 100
	}

	po.metrics.LastUpdated = time.Now()
}

// batchTimeoutWorker handles batch timeouts
func (po *PerformanceOptimizer) batchTimeoutWorker() {
	ticker := time.NewTicker(po.config.BatchTimeout / 2)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			po.processTimedOutBatches()
		case <-po.batchProcessor.stopChan:
			return
		}
	}
}

// processTimedOutBatches processes batches that have timed out
func (po *PerformanceOptimizer) processTimedOutBatches() {
	po.batchProcessor.batchMutex.Lock()
	defer po.batchProcessor.batchMutex.Unlock()

	now := time.Now()
	for key, batch := range po.batchProcessor.batches {
		if now.Sub(batch.Timestamp) >= po.config.BatchTimeout {
			po.processBatch(batch)
			delete(po.batchProcessor.batches, key)
		}
	}
}

// GetMetrics returns current performance metrics
func (po *PerformanceOptimizer) GetMetrics() *PerformanceMetrics {
	po.metrics.mutex.RLock()
	defer po.metrics.mutex.RUnlock()

	// Create a copy to avoid race conditions
	return &PerformanceMetrics{
		RequestsPerSecond:   po.metrics.RequestsPerSecond,
		AverageResponseTime: po.metrics.AverageResponseTime,
		ErrorRate:           po.metrics.ErrorRate,
		MemoryUsage:         po.metrics.MemoryUsage,
		GoroutineCount:      po.metrics.GoroutineCount,
		LastUpdated:         po.metrics.LastUpdated,
	}
}

// GetPoolMetrics returns connection pool metrics
func (po *PerformanceOptimizer) GetPoolMetrics() map[string]*PoolMetrics {
	po.poolMutex.RLock()
	defer po.poolMutex.RUnlock()

	metrics := make(map[string]*PoolMetrics)
	for name, pool := range po.connectionPools {
		metrics[name] = &PoolMetrics{
			ActiveConnections: atomic.LoadInt64(&pool.metrics.ActiveConnections),
			TotalConnections:  atomic.LoadInt64(&pool.metrics.TotalConnections),
			AcquireCount:      atomic.LoadInt64(&pool.metrics.AcquireCount),
			AcquireTime:       atomic.LoadInt64(&pool.metrics.AcquireTime),
			Errors:            atomic.LoadInt64(&pool.metrics.Errors),
		}
	}
	return metrics
}

// SetBatchProcessor sets the batch processing function
func (po *PerformanceOptimizer) SetBatchProcessor(fn func([]interface{}) error) {
	po.batchProcessor.processFunc = fn
}

// Stop stops all background workers
func (po *PerformanceOptimizer) Stop() {
	close(po.batchProcessor.stopChan)

	// Close all connection pools
	po.poolMutex.Lock()
	for name, pool := range po.connectionPools {
		po.closePool(name, pool)
	}
	po.poolMutex.Unlock()

	po.logger.Info("Performance optimizer stopped")
}

// closePool closes a connection pool
func (po *PerformanceOptimizer) closePool(name string, pool *ConnectionPool) {
	for {
		select {
		case conn := <-pool.connections:
			if pool.closer != nil {
				pool.closer(conn)
			}
		default:
			po.logger.Info("Connection pool closed", zap.String("name", name))
			return
		}
	}
}

// Error definitions
var (
	ErrPoolNotFound = fmt.Errorf("connection pool not found")
	ErrPoolTimeout  = fmt.Errorf("connection pool timeout")
	ErrCircuitOpen  = fmt.Errorf("circuit breaker is open")
)
