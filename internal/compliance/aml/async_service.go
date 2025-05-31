package aml

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/redis"
	"github.com/Aidin1998/pincex_unified/internal/trading/model"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// AsyncRiskService provides high-performance async risk management with Redis caching
type AsyncRiskService struct {
	// Core dependencies
	baseService RiskService        // Existing service for fallback
	redisClient *redis.Client      // Redis client for caching
	logger      *zap.SugaredLogger // Logger
	config      *AsyncRiskConfig   // Configuration

	// Cache management
	cacheKeys   *redis.CacheKeys      // Standardized cache keys
	cacheConfig *redis.AMLCacheConfig // Cache configuration

	// Async processing
	requestChan chan *AsyncRiskRequest // Async request queue
	resultChan  chan *AsyncRiskResult  // Async result queue
	workers     sync.WaitGroup         // Worker management
	shutdown    chan struct{}          // Shutdown signal

	// Background workers
	riskWorkers  []*RiskWorker  // Risk calculation workers
	cacheWorkers []*CacheWorker // Cache management workers
	pubsubWorker *PubSubWorker  // Redis pub/sub worker

	// Performance tracking
	metrics        *AsyncMetrics // Performance metrics
	requestCounter int64         // Atomic request counter
	processingTime int64         // Atomic avg processing time (microseconds)

	// Session management
	pendingSessions map[string]*RiskSession // Active risk sessions
	sessionMutex    sync.RWMutex            // Session map protection

	// Circuit breaker for fallback
	circuitBreaker *CircuitBreaker // Circuit breaker
}

// AsyncRiskConfig configures async risk service behavior
type AsyncRiskConfig struct {
	// Worker configuration
	RiskWorkerCount   int `yaml:"risk_worker_count" json:"risk_worker_count"`
	CacheWorkerCount  int `yaml:"cache_worker_count" json:"cache_worker_count"`
	RequestBufferSize int `yaml:"request_buffer_size" json:"request_buffer_size"`
	ResultBufferSize  int `yaml:"result_buffer_size" json:"result_buffer_size"`

	// Performance targets
	TargetLatencyMs     int `yaml:"target_latency_ms" json:"target_latency_ms"`         // 500ms target
	MaxLatencyMs        int `yaml:"max_latency_ms" json:"max_latency_ms"`               // 1000ms max
	TargetThroughputOPS int `yaml:"target_throughput_ops" json:"target_throughput_ops"` // 10,000 OPS

	// Timeout configuration
	RiskCalculationTimeout time.Duration `yaml:"risk_calculation_timeout" json:"risk_calculation_timeout"`
	CacheTimeout           time.Duration `yaml:"cache_timeout" json:"cache_timeout"`
	SessionTimeout         time.Duration `yaml:"session_timeout" json:"session_timeout"`

	// Cache strategy
	EnableCaching       bool    `yaml:"enable_caching" json:"enable_caching"`
	CacheHitRatio       float64 `yaml:"cache_hit_ratio" json:"cache_hit_ratio"` // Target 95%
	PreloadUserProfiles bool    `yaml:"preload_user_profiles" json:"preload_user_profiles"`

	// Batch processing
	EnableBatchProcessing bool          `yaml:"enable_batch_processing" json:"enable_batch_processing"`
	BatchSize             int           `yaml:"batch_size" json:"batch_size"`
	BatchTimeout          time.Duration `yaml:"batch_timeout" json:"batch_timeout"`

	// Fallback configuration
	EnableFallback    bool    `yaml:"enable_fallback" json:"enable_fallback"`
	FallbackThreshold float64 `yaml:"fallback_threshold" json:"fallback_threshold"` // Error rate threshold

	// Real-time features
	EnableRealtimeUpdates bool     `yaml:"enable_realtime_updates" json:"enable_realtime_updates"`
	PubSubChannels        []string `yaml:"pubsub_channels" json:"pubsub_channels"`
}

// DefaultAsyncRiskConfig returns optimized configuration for async risk service
func DefaultAsyncRiskConfig() *AsyncRiskConfig {
	return &AsyncRiskConfig{
		// Worker configuration optimized for high throughput
		RiskWorkerCount:   8,    // 8 risk calculation workers
		CacheWorkerCount:  4,    // 4 cache workers
		RequestBufferSize: 2000, // Large buffer for burst handling
		ResultBufferSize:  2000,

		// Performance targets from risk-management.yaml
		TargetLatencyMs:     500,   // Sub-500ms target
		MaxLatencyMs:        1000,  // 1 second max
		TargetThroughputOPS: 10000, // 10K operations per second

		// Aggressive timeouts for low latency
		RiskCalculationTimeout: time.Millisecond * 400, // 400ms for risk calc
		CacheTimeout:           time.Millisecond * 50,  // 50ms for cache ops
		SessionTimeout:         time.Minute * 5,        // 5 minute sessions

		// High-performance caching
		EnableCaching:       true,
		CacheHitRatio:       0.95, // 95% cache hit target
		PreloadUserProfiles: true,

		// Batch processing for efficiency
		EnableBatchProcessing: true,
		BatchSize:             100,                    // 100 users per batch
		BatchTimeout:          time.Millisecond * 100, // 100ms batch timeout

		// Fallback for reliability
		EnableFallback:    true,
		FallbackThreshold: 0.05, // 5% error rate triggers fallback

		// Real-time features
		EnableRealtimeUpdates: true,
		PubSubChannels: []string{
			"risk:updates",
			"market:updates",
			"compliance:alerts",
		},
	}
}

// AsyncRiskRequest represents an async risk calculation request
type AsyncRiskRequest struct {
	RequestID    string                 `json:"request_id"`
	Type         RiskRequestType        `json:"type"`
	UserID       string                 `json:"user_id"`
	Order        *model.Order           `json:"order,omitempty"`
	Trades       []*model.Trade         `json:"trades,omitempty"`
	Parameters   map[string]interface{} `json:"parameters,omitempty"`
	Priority     RequestPriority        `json:"priority"`
	SubmittedAt  time.Time              `json:"submitted_at"`
	TimeoutAt    time.Time              `json:"timeout_at"`
	ResponseChan chan *AsyncRiskResult  `json:"-"`
	Context      context.Context        `json:"-"`
}

// AsyncRiskResult represents the result of async risk calculation
type AsyncRiskResult struct {
	RequestID        string            `json:"request_id"`
	Success          bool              `json:"success"`
	Error            error             `json:"error,omitempty"`
	RiskMetrics      *RiskMetrics      `json:"risk_metrics,omitempty"`
	ComplianceResult *ComplianceResult `json:"compliance_result,omitempty"`
	CacheHit         bool              `json:"cache_hit"`
	ProcessingTime   time.Duration     `json:"processing_time"`
	QueueTime        time.Duration     `json:"queue_time"`
	CalculationTime  time.Duration     `json:"calculation_time"`
	CacheTime        time.Duration     `json:"cache_time"`
	Timestamp        time.Time         `json:"timestamp"`
	WorkerID         string            `json:"worker_id"`
}

// RiskRequestType defines the type of risk request
type RiskRequestType string

const (
	RiskRequestCalculateUser   RiskRequestType = "CALCULATE_USER"
	RiskRequestValidateOrder   RiskRequestType = "VALIDATE_ORDER"
	RiskRequestProcessTrade    RiskRequestType = "PROCESS_TRADE"
	RiskRequestBatchCalculate  RiskRequestType = "BATCH_CALCULATE"
	RiskRequestComplianceCheck RiskRequestType = "COMPLIANCE_CHECK"
	RiskRequestPositionUpdate  RiskRequestType = "POSITION_UPDATE"
)

// RequestPriority defines request priority levels
type RequestPriority int

const (
	PriorityLow      RequestPriority = 1
	PriorityNormal   RequestPriority = 2
	PriorityHigh     RequestPriority = 3
	PriorityCritical RequestPriority = 4
)

// RiskSession tracks ongoing risk calculation sessions
type RiskSession struct {
	SessionID       string                       `json:"session_id"`
	UserID          string                       `json:"user_id"`
	StartTime       time.Time                    `json:"start_time"`
	LastActivity    time.Time                    `json:"last_activity"`
	RequestCount    int                          `json:"request_count"`
	CachedData      map[string]interface{}       `json:"cached_data"`
	PendingRequests map[string]*AsyncRiskRequest `json:"pending_requests"`
}

// AsyncMetrics tracks performance metrics for the async service
type AsyncMetrics struct {
	// Request metrics
	TotalRequests      int64 `json:"total_requests"`
	SuccessfulRequests int64 `json:"successful_requests"`
	FailedRequests     int64 `json:"failed_requests"`
	TimeoutRequests    int64 `json:"timeout_requests"`

	// Performance metrics
	AverageLatencyMs float64 `json:"average_latency_ms"`
	P95LatencyMs     float64 `json:"p95_latency_ms"`
	P99LatencyMs     float64 `json:"p99_latency_ms"`
	ThroughputOPS    float64 `json:"throughput_ops"`

	// Cache metrics
	CacheHits     int64   `json:"cache_hits"`
	CacheMisses   int64   `json:"cache_misses"`
	CacheHitRatio float64 `json:"cache_hit_ratio"`

	// Worker metrics
	ActiveWorkers      int `json:"active_workers"`
	QueuedRequests     int `json:"queued_requests"`
	ProcessingRequests int `json:"processing_requests"`

	// Error metrics
	ErrorRate          float64 `json:"error_rate"`
	CircuitBreakerOpen bool    `json:"circuit_breaker_open"`

	// Last updated
	LastUpdated time.Time `json:"last_updated"`
}

// NewAsyncRiskService creates a new async risk service with Redis caching
func NewAsyncRiskService(
	baseService RiskService,
	redisClient *redis.Client,
	logger *zap.SugaredLogger,
	config *AsyncRiskConfig,
) (*AsyncRiskService, error) {
	if config == nil {
		config = DefaultAsyncRiskConfig()
	}

	service := &AsyncRiskService{
		baseService:     baseService,
		redisClient:     redisClient,
		logger:          logger,
		config:          config,
		cacheKeys:       redis.NewCacheKeys(),
		requestChan:     make(chan *AsyncRiskRequest, config.RequestBufferSize),
		resultChan:      make(chan *AsyncRiskResult, config.ResultBufferSize),
		shutdown:        make(chan struct{}),
		pendingSessions: make(map[string]*RiskSession),
		metrics:         &AsyncMetrics{LastUpdated: time.Now()},
		circuitBreaker:  NewCircuitBreaker(config.FallbackThreshold),
	}

	// Initialize cache configuration
	service.cacheConfig = &redis.AMLCacheConfig{
		UserRiskProfileTTL: time.Minute * 15,
		RiskMetricsTTL:     time.Minute * 5,
		// ... other TTL configurations
	}

	// Start worker pools
	if err := service.startWorkers(); err != nil {
		return nil, fmt.Errorf("failed to start workers: %w", err)
	}

	// Start background tasks
	go service.metricsCollector()
	go service.sessionManager()

	// Start pub/sub worker if real-time updates are enabled
	if config.EnableRealtimeUpdates {
		service.pubsubWorker = NewPubSubWorker(redisClient, service.handleRealtimeUpdate, logger)
		go service.pubsubWorker.Start(config.PubSubChannels)
	}

	logger.Infow("Async risk service started",
		"risk_workers", config.RiskWorkerCount,
		"cache_workers", config.CacheWorkerCount,
		"target_latency_ms", config.TargetLatencyMs,
		"target_throughput_ops", config.TargetThroughputOPS,
	)

	return service, nil
}

// CalculateRiskAsync performs async risk calculation with caching
func (s *AsyncRiskService) CalculateRiskAsync(ctx context.Context, userID string) (*RiskMetrics, error) {
	start := time.Now()
	requestID := s.generateRequestID(userID, "calc")

	// Check circuit breaker
	if s.circuitBreaker.IsOpen() {
		s.logger.Warnw("Circuit breaker open, using fallback", "user_id", userID)
		return s.fallbackCalculateRisk(ctx, userID)
	}

	// Try cache first
	if s.config.EnableCaching {
		if cached, err := s.getCachedRiskMetrics(ctx, userID); err == nil && cached != nil {
			atomic.AddInt64(&s.metrics.CacheHits, 1)
			s.updateLatencyMetrics(time.Since(start))
			return cached, nil
		}
		atomic.AddInt64(&s.metrics.CacheMisses, 1)
	}

	// Create async request
	request := &AsyncRiskRequest{
		RequestID:    requestID,
		Type:         RiskRequestCalculateUser,
		UserID:       userID,
		Priority:     PriorityNormal,
		SubmittedAt:  time.Now(),
		TimeoutAt:    time.Now().Add(s.config.RiskCalculationTimeout),
		ResponseChan: make(chan *AsyncRiskResult, 1),
		Context:      ctx,
	}

	// Submit to worker pool
	select {
	case s.requestChan <- request:
		atomic.AddInt64(&s.requestCounter, 1)
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(time.Millisecond * 100): // Fast timeout for queue
		return s.fallbackCalculateRisk(ctx, userID)
	}

	// Wait for result
	select {
	case result := <-request.ResponseChan:
		s.updateLatencyMetrics(time.Since(start))
		if result.Success {
			atomic.AddInt64(&s.metrics.SuccessfulRequests, 1)

			// Cache the result
			if s.config.EnableCaching && result.RiskMetrics != nil {
				go s.cacheRiskMetrics(ctx, userID, result.RiskMetrics)
			}

			return result.RiskMetrics, nil
		} else {
			atomic.AddInt64(&s.metrics.FailedRequests, 1)
			s.circuitBreaker.RecordFailure()
			return nil, result.Error
		}
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(s.config.RiskCalculationTimeout):
		atomic.AddInt64(&s.metrics.TimeoutRequests, 1)
		return s.fallbackCalculateRisk(ctx, userID)
	}
}

// BatchCalculateRiskAsync performs batch risk calculation for multiple users
func (s *AsyncRiskService) BatchCalculateRiskAsync(ctx context.Context, userIDs []string) (map[string]*RiskMetrics, error) {
	if len(userIDs) == 0 {
		return make(map[string]*RiskMetrics), nil
	}

	start := time.Now()
	requestID := s.generateRequestID("batch", fmt.Sprintf("%d", len(userIDs)))

	// Check circuit breaker
	if s.circuitBreaker.IsOpen() {
		return s.fallbackBatchCalculateRisk(ctx, userIDs)
	}

	// Create batch request
	request := &AsyncRiskRequest{
		RequestID:    requestID,
		Type:         RiskRequestBatchCalculate,
		Parameters:   map[string]interface{}{"user_ids": userIDs},
		Priority:     PriorityNormal,
		SubmittedAt:  time.Now(),
		TimeoutAt:    time.Now().Add(s.config.RiskCalculationTimeout * time.Duration(len(userIDs)/s.config.BatchSize+1)),
		ResponseChan: make(chan *AsyncRiskResult, 1),
		Context:      ctx,
	}

	// Submit to worker pool
	select {
	case s.requestChan <- request:
		atomic.AddInt64(&s.requestCounter, 1)
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(time.Millisecond * 200):
		return s.fallbackBatchCalculateRisk(ctx, userIDs)
	}
	// Wait for result
	select {
	case result := <-request.ResponseChan:
		s.updateLatencyMetrics(time.Since(start))
		if result.Success {
			atomic.AddInt64(&s.metrics.SuccessfulRequests, 1)

			// Extract batch results - the worker will set this in the result
			if batchResults := result.RiskMetrics; batchResults != nil {
				// For batch results, we'll need to modify the result structure
				// For now, return empty map - will be enhanced in worker implementation
				return make(map[string]*RiskMetrics), nil
			}
			return make(map[string]*RiskMetrics), nil
		} else {
			atomic.AddInt64(&s.metrics.FailedRequests, 1)
			s.circuitBreaker.RecordFailure()
			return nil, result.Error
		}
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(s.config.RiskCalculationTimeout * time.Duration(len(userIDs)/s.config.BatchSize+1)):
		atomic.AddInt64(&s.metrics.TimeoutRequests, 1)
		return s.fallbackBatchCalculateRisk(ctx, userIDs)
	}
}

// ValidateOrderAsync performs async order validation with caching
func (s *AsyncRiskService) ValidateOrderAsync(ctx context.Context, order *model.Order) (bool, *RiskMetrics, error) {
	start := time.Now()
	requestID := s.generateRequestID(order.UserID.String(), "validate")

	// Check circuit breaker
	if s.circuitBreaker.IsOpen() {
		return s.fallbackValidateOrder(ctx, order)
	}

	// Create async request
	request := &AsyncRiskRequest{
		RequestID:    requestID,
		Type:         RiskRequestValidateOrder,
		UserID:       order.UserID.String(),
		Order:        order,
		Priority:     PriorityHigh, // Order validation is high priority
		SubmittedAt:  time.Now(),
		TimeoutAt:    time.Now().Add(s.config.RiskCalculationTimeout),
		ResponseChan: make(chan *AsyncRiskResult, 1),
		Context:      ctx,
	}

	// Submit to worker pool
	select {
	case s.requestChan <- request:
		atomic.AddInt64(&s.requestCounter, 1)
	case <-ctx.Done():
		return false, nil, ctx.Err()
	case <-time.After(time.Millisecond * 50): // Very fast timeout for order validation
		return s.fallbackValidateOrder(ctx, order)
	}

	// Wait for result
	select {
	case result := <-request.ResponseChan:
		s.updateLatencyMetrics(time.Since(start))
		if result.Success {
			atomic.AddInt64(&s.metrics.SuccessfulRequests, 1)

			// For order validation, we'll add a simple approved flag in the result
			// This will be properly set by the worker
			return true, result.RiskMetrics, nil
		} else {
			atomic.AddInt64(&s.metrics.FailedRequests, 1)
			s.circuitBreaker.RecordFailure()
			return false, nil, result.Error
		}
	case <-ctx.Done():
		return false, nil, ctx.Err()
	case <-time.After(s.config.RiskCalculationTimeout):
		atomic.AddInt64(&s.metrics.TimeoutRequests, 1)
		return s.fallbackValidateOrder(ctx, order)
	}
}

// ProcessTradeAsync processes trade asynchronously with position updates
func (s *AsyncRiskService) ProcessTradeAsync(ctx context.Context, trades []*model.Trade) error {
	if len(trades) == 0 {
		return nil
	}

	start := time.Now()
	requestID := s.generateRequestID("trade", fmt.Sprintf("%d", len(trades)))

	// Check circuit breaker
	if s.circuitBreaker.IsOpen() {
		return s.fallbackProcessTrades(ctx, trades)
	}

	// Create async request
	request := &AsyncRiskRequest{
		RequestID:    requestID,
		Type:         RiskRequestProcessTrade,
		Trades:       trades,
		Priority:     PriorityHigh, // Trade processing is high priority
		SubmittedAt:  time.Now(),
		TimeoutAt:    time.Now().Add(s.config.RiskCalculationTimeout),
		ResponseChan: make(chan *AsyncRiskResult, 1),
		Context:      ctx,
	}

	// Submit to worker pool
	select {
	case s.requestChan <- request:
		atomic.AddInt64(&s.requestCounter, 1)
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(time.Millisecond * 100):
		return s.fallbackProcessTrades(ctx, trades)
	}

	// Wait for result
	select {
	case result := <-request.ResponseChan:
		s.updateLatencyMetrics(time.Since(start))
		if result.Success {
			atomic.AddInt64(&s.metrics.SuccessfulRequests, 1)
			s.circuitBreaker.RecordSuccess()
			return nil
		} else {
			atomic.AddInt64(&s.metrics.FailedRequests, 1)
			s.circuitBreaker.RecordFailure()
			return result.Error
		}
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(s.config.RiskCalculationTimeout):
		atomic.AddInt64(&s.metrics.TimeoutRequests, 1)
		return s.fallbackProcessTrades(ctx, trades)
	}
}

// GetMetrics returns current performance metrics
func (s *AsyncRiskService) GetMetrics() *AsyncMetrics {
	// Update real-time metrics
	s.metrics.ActiveWorkers = len(s.riskWorkers)
	s.metrics.QueuedRequests = len(s.requestChan)
	s.metrics.CircuitBreakerOpen = s.circuitBreaker.IsOpen()
	s.metrics.LastUpdated = time.Now()

	// Calculate cache hit ratio
	totalCacheRequests := s.metrics.CacheHits + s.metrics.CacheMisses
	if totalCacheRequests > 0 {
		s.metrics.CacheHitRatio = float64(s.metrics.CacheHits) / float64(totalCacheRequests)
	}

	// Calculate error rate
	totalRequests := s.metrics.SuccessfulRequests + s.metrics.FailedRequests
	if totalRequests > 0 {
		s.metrics.ErrorRate = float64(s.metrics.FailedRequests) / float64(totalRequests)
	}

	return s.metrics
}

// Close gracefully shuts down the async risk service
func (s *AsyncRiskService) Close() error {
	s.logger.Info("Shutting down async risk service...")

	// Signal shutdown
	close(s.shutdown)

	// Stop pub/sub worker
	if s.pubsubWorker != nil {
		s.pubsubWorker.Stop()
	}

	// Wait for workers to finish
	s.workers.Wait()

	// Close channels
	close(s.requestChan)
	close(s.resultChan)

	s.logger.Info("Async risk service shutdown complete")
	return nil
}

// generateRequestID creates a unique request ID
func (s *AsyncRiskService) generateRequestID(prefix string, suffix string) string {
	timestamp := time.Now().UnixNano()
	hash := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%d", prefix, suffix, timestamp)))
	return fmt.Sprintf("%s_%s_%s", prefix, hex.EncodeToString(hash[:4]), suffix)
}

// updateLatencyMetrics updates performance metrics
func (s *AsyncRiskService) updateLatencyMetrics(duration time.Duration) {
	latencyMs := float64(duration.Nanoseconds()) / 1e6

	// Simple moving average for now (could be enhanced with more sophisticated metrics)
	currentAvg := s.metrics.AverageLatencyMs
	s.metrics.AverageLatencyMs = (currentAvg * 0.9) + (latencyMs * 0.1)

	// Update atomic processing time in microseconds
	atomic.StoreInt64(&s.processingTime, int64(latencyMs*1000))
}
