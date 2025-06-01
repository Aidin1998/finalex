package aml

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/trading/model"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// CacheKeys defines all Redis key patterns for AML/risk management
type CacheKeys struct {
	// User risk profiles
	UserRiskProfile string // "user:risk:{userID}"
	UserPositions   string // "user:positions:{userID}"
	UserLimits      string // "user:limits:{userID}"

	// Risk metrics and calculations
	RiskMetrics     string // "risk:metrics:{userID}"
	RiskCalculation string // "risk:calc:{userID}"
	MarketData      string // "market:data:{symbol}"
	VolatilityData  string // "market:volatility:{symbol}"

	// Compliance data
	ComplianceProfile string // "compliance:profile:{userID}"
	ComplianceRules   string // "compliance:rules"
	ComplianceAlerts  string // "compliance:alerts:{userID}"
	TransactionFlags  string // "compliance:flags:{transactionID}"

	// Position and limit caches
	PositionLimits string // "limits:position:{userID}:{symbol}"
	GlobalLimits   string // "limits:global"
	ExemptionList  string // "exemptions:users"

	// Session and pending operations
	PendingRiskChecks string // "pending:risk:{requestID}"
	RiskCheckResults  string // "results:risk:{requestID}"
	SessionData       string // "session:risk:{sessionID}"

	// Real-time data
	RealtimeRisk      string // "realtime:risk:{userID}"
	RealtimePositions string // "realtime:positions:{userID}"
	RealtimeAlerts    string // "realtime:alerts"

	// Performance optimization
	CalculationCache string // "cache:calc:{hash}"
	LookupCache      string // "cache:lookup:{key}"
}

// NewCacheKeys creates standardized cache keys
func NewCacheKeys() *CacheKeys {
	return &CacheKeys{
		UserRiskProfile: "user:risk:%s",
		UserPositions:   "user:positions:%s",
		UserLimits:      "user:limits:%s",

		RiskMetrics:     "risk:metrics:%s",
		RiskCalculation: "risk:calc:%s",
		MarketData:      "market:data:%s",
		VolatilityData:  "market:volatility:%s",

		ComplianceProfile: "compliance:profile:%s",
		ComplianceRules:   "compliance:rules",
		ComplianceAlerts:  "compliance:alerts:%s",
		TransactionFlags:  "compliance:flags:%s",

		PositionLimits: "limits:position:%s:%s",
		GlobalLimits:   "limits:global",
		ExemptionList:  "exemptions:users",

		PendingRiskChecks: "pending:risk:%s",
		RiskCheckResults:  "results:risk:%s",
		SessionData:       "session:risk:%s",

		RealtimeRisk:      "realtime:risk:%s",
		RealtimePositions: "realtime:positions:%s",
		RealtimeAlerts:    "realtime:alerts",

		CalculationCache: "cache:calc:%s",
		LookupCache:      "cache:lookup:%s",
	}
}

// AMLCacheConfig configures AML cache behavior
type AMLCacheConfig struct {
	// TTL settings for different data types
	UserRiskProfileTTL  time.Duration `yaml:"user_risk_profile_ttl" json:"user_risk_profile_ttl"`
	RiskMetricsTTL      time.Duration `yaml:"risk_metrics_ttl" json:"risk_metrics_ttl"`
	MarketDataTTL       time.Duration `yaml:"market_data_ttl" json:"market_data_ttl"`
	ComplianceDataTTL   time.Duration `yaml:"compliance_data_ttl" json:"compliance_data_ttl"`
	PositionDataTTL     time.Duration `yaml:"position_data_ttl" json:"position_data_ttl"`
	SessionDataTTL      time.Duration `yaml:"session_data_ttl" json:"session_data_ttl"`
	CalculationCacheTTL time.Duration `yaml:"calculation_cache_ttl" json:"calculation_cache_ttl"`

	// Cache size limits
	MaxUserProfiles     int `yaml:"max_user_profiles" json:"max_user_profiles"`
	MaxRiskCalculations int `yaml:"max_risk_calculations" json:"max_risk_calculations"`
	MaxSessionData      int `yaml:"max_session_data" json:"max_session_data"`

	// Performance settings
	EnablePipelining   bool `yaml:"enable_pipelining" json:"enable_pipelining"`
	PipelineBufferSize int  `yaml:"pipeline_buffer_size" json:"pipeline_buffer_size"`
	EnableCompression  bool `yaml:"enable_compression" json:"enable_compression"`
	EnableAsyncWrites  bool `yaml:"enable_async_writes" json:"enable_async_writes"`

	// Pub/Sub settings
	EnableRealTimeUpdates bool     `yaml:"enable_realtime_updates" json:"enable_realtime_updates"`
	PubSubChannels        []string `yaml:"pubsub_channels" json:"pubsub_channels"`
	MaxSubscribers        int      `yaml:"max_subscribers" json:"max_subscribers"`
}

// DefaultAMLCacheConfig returns optimized cache configuration for AML/risk management
func DefaultAMLCacheConfig() *AMLCacheConfig {
	return &AMLCacheConfig{
		// TTL settings optimized for risk management needs
		UserRiskProfileTTL:  time.Minute * 5,  // User profiles change frequently
		RiskMetricsTTL:      time.Minute * 2,  // Risk metrics need frequent updates
		MarketDataTTL:       time.Second * 30, // Market data changes rapidly
		ComplianceDataTTL:   time.Minute * 30, // Compliance data is more stable
		PositionDataTTL:     time.Minute * 1,  // Positions change with every trade
		SessionDataTTL:      time.Minute * 10, // Session data for pending operations
		CalculationCacheTTL: time.Minute * 5,  // Cache calculation results

		// Cache size limits
		MaxUserProfiles:     100000, // Support large user base
		MaxRiskCalculations: 50000,  // Cache frequent calculations
		MaxSessionData:      10000,  // Pending operations cache

		// Performance optimizations
		EnablePipelining:   true,
		PipelineBufferSize: 1000,
		EnableCompression:  false, // Prioritize speed over space
		EnableAsyncWrites:  true,

		// Real-time features
		EnableRealTimeUpdates: true,
		PubSubChannels: []string{
			"risk:updates",
			"compliance:alerts",
			"position:updates",
			"market:updates",
		},
		MaxSubscribers: 1000,
	}
}

// AsyncRiskService provides high-performance async risk management with Redis caching
type AsyncRiskService struct {
	// Core dependencies
	baseService RiskService           // Existing service for fallback
	redisClient redis.UniversalClient // Redis client for caching
	logger      *zap.SugaredLogger    // Logger
	config      *AsyncRiskConfig      // Configuration

	// Cache management
	cacheKeys   *CacheKeys      // Standardized cache keys
	cacheConfig *AMLCacheConfig // Cache configuration

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
	redisClient redis.UniversalClient,
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
		cacheKeys:       NewCacheKeys(),
		requestChan:     make(chan *AsyncRiskRequest, config.RequestBufferSize),
		resultChan:      make(chan *AsyncRiskResult, config.ResultBufferSize),
		shutdown:        make(chan struct{}),
		pendingSessions: make(map[string]*RiskSession),
		metrics:         &AsyncMetrics{LastUpdated: time.Now()},
		circuitBreaker:  NewCircuitBreaker(config.FallbackThreshold),
	}
	// Initialize cache configuration
	service.cacheConfig = DefaultAMLCacheConfig()

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

// startWorkers initializes and starts the worker pools
func (s *AsyncRiskService) startWorkers() error {
	s.logger.Info("Starting async risk service workers...")

	// Initialize worker slices
	s.riskWorkers = make([]*RiskWorker, s.config.RiskWorkerCount)
	s.cacheWorkers = make([]*CacheWorker, s.config.CacheWorkerCount)

	// Start risk calculation workers
	for i := 0; i < s.config.RiskWorkerCount; i++ {
		workerID := fmt.Sprintf("risk-%d", i)
		worker := NewRiskWorker(workerID, s, s.logger)
		s.riskWorkers[i] = worker
		worker.Start()
	}

	// Start cache workers
	for i := 0; i < s.config.CacheWorkerCount; i++ {
		workerID := fmt.Sprintf("cache-%d", i)
		worker := NewCacheWorker(workerID, s, s.logger)
		s.cacheWorkers[i] = worker
		worker.Start()
	}

	s.logger.Infow("Started async risk service workers",
		"risk_workers", len(s.riskWorkers),
		"cache_workers", len(s.cacheWorkers),
	)

	return nil
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

// metricsCollector runs periodic metrics collection and analysis
func (s *AsyncRiskService) metricsCollector() {
	ticker := time.NewTicker(time.Minute) // Collect metrics every minute
	defer ticker.Stop()

	for {
		select {
		case <-s.shutdown:
			return
		case <-ticker.C:
			s.collectAndUpdateMetrics()
		}
	}
}

// collectAndUpdateMetrics collects current metrics and updates aggregated data
func (s *AsyncRiskService) collectAndUpdateMetrics() {
	now := time.Now()

	// Update cache hit ratio
	totalCacheOps := atomic.LoadInt64(&s.metrics.CacheHits) + atomic.LoadInt64(&s.metrics.CacheMisses)
	if totalCacheOps > 0 {
		s.metrics.CacheHitRatio = float64(atomic.LoadInt64(&s.metrics.CacheHits)) / float64(totalCacheOps)
	}

	// Update error rate
	totalRequests := atomic.LoadInt64(&s.metrics.TotalRequests)
	if totalRequests > 0 {
		failedRequests := atomic.LoadInt64(&s.metrics.FailedRequests)
		s.metrics.ErrorRate = float64(failedRequests) / float64(totalRequests) * 100
	}

	// Update throughput (operations per second over last minute)
	timeSinceLastUpdate := now.Sub(s.metrics.LastUpdated).Seconds()
	if timeSinceLastUpdate > 0 {
		requestsSinceLastUpdate := atomic.LoadInt64(&s.requestCounter)
		s.metrics.ThroughputOPS = float64(requestsSinceLastUpdate) / timeSinceLastUpdate
	}

	// Update queue metrics
	s.metrics.QueuedRequests = len(s.requestChan)
	s.metrics.ActiveWorkers = len(s.riskWorkers) + len(s.cacheWorkers)

	// Check circuit breaker status
	s.metrics.CircuitBreakerOpen = s.circuitBreaker.IsOpen()

	s.metrics.LastUpdated = now

	s.logger.Debugw("Metrics updated",
		"cache_hit_ratio", s.metrics.CacheHitRatio,
		"error_rate", s.metrics.ErrorRate,
		"throughput_ops", s.metrics.ThroughputOPS,
		"queued_requests", s.metrics.QueuedRequests,
		"circuit_breaker_open", s.metrics.CircuitBreakerOpen,
	)
}

// sessionManager manages risk calculation sessions and cleanup
func (s *AsyncRiskService) sessionManager() {
	ticker := time.NewTicker(time.Minute * 5) // Check sessions every 5 minutes
	defer ticker.Stop()

	for {
		select {
		case <-s.shutdown:
			return
		case <-ticker.C:
			s.cleanupSessions()
		}
	}
}

// cleanupSessions removes expired sessions and their cached data
func (s *AsyncRiskService) cleanupSessions() {
	s.sessionMutex.Lock()
	defer s.sessionMutex.Unlock()

	now := time.Now()
	expiredSessions := make([]string, 0)

	for sessionID, session := range s.pendingSessions {
		if now.Sub(session.LastActivity) > s.config.SessionTimeout {
			expiredSessions = append(expiredSessions, sessionID)
		}
	}

	// Remove expired sessions
	for _, sessionID := range expiredSessions {
		delete(s.pendingSessions, sessionID)
		s.logger.Debugw("Removed expired session", "session_id", sessionID)
	}

	if len(expiredSessions) > 0 {
		s.logger.Infow("Cleaned up expired sessions", "count", len(expiredSessions))
	}
}

// fallbackValidateOrder provides fallback order validation when async processing fails
func (s *AsyncRiskService) fallbackValidateOrder(ctx context.Context, order *model.Order) (bool, *RiskMetrics, error) {
	s.logger.Warnw("Using fallback order validation", "order_id", order.ID, "user_id", order.UserID)

	// Use the base service for direct validation
	riskProfile, err := s.baseService.CalculateRisk(ctx, order.UserID.String())
	if err != nil {
		s.logger.Errorw("Fallback risk calculation failed", "error", err, "user_id", order.UserID)
		return false, nil, fmt.Errorf("fallback risk calculation failed: %w", err)
	}
	// Convert RiskProfile to RiskMetrics
	riskMetrics := &RiskMetrics{
		UserID:         riskProfile.UserID,
		TotalExposure:  riskProfile.CurrentExposure,
		ValueAtRisk:    riskProfile.ValueAtRisk,
		RiskScore:      decimal.NewFromFloat(s.calculateRiskScore(riskProfile)),
		LastCalculated: time.Now(),
	}

	// Simple validation logic - approve if risk score is acceptable
	approved := riskMetrics.RiskScore.LessThan(decimal.NewFromFloat(0.8)) // 80% risk threshold

	s.logger.Infow("Fallback order validation completed",
		"order_id", order.ID,
		"user_id", order.UserID,
		"approved", approved,
		"risk_score", riskMetrics.RiskScore,
	)

	return approved, riskMetrics, nil
}

// calculateRiskScore calculates a risk score from a risk profile
func (s *AsyncRiskService) calculateRiskScore(profile *UserRiskProfile) float64 {
	// Simple risk score calculation - can be enhanced
	if profile.CurrentExposure.IsZero() {
		return 0.0
	}

	// Risk score based on exposure to margin ratio
	if profile.MarginRequired.IsZero() {
		return 1.0 // Maximum risk if no margin
	}

	exposureRatio := profile.CurrentExposure.Div(profile.MarginRequired).InexactFloat64()

	// Cap the risk score at 1.0
	if exposureRatio > 1.0 {
		return 1.0
	}

	return exposureRatio
}

// fallbackProcessTrades provides fallback trade processing when async processing fails
func (s *AsyncRiskService) fallbackProcessTrades(ctx context.Context, trades []*model.Trade) error {
	s.logger.Warnw("Using fallback trade processing", "trade_count", len(trades))

	// Process trades synchronously using base service
	for _, trade := range trades {
		// Update user positions and risk metrics
		err := s.updateUserPositionFromTrade(ctx, trade)
		if err != nil {
			s.logger.Errorw("Failed to update user position in fallback",
				"error", err,
				"trade_id", trade.ID,
				"user_id", trade.OrderID, // Assuming OrderID relates to user
			)
			// Continue processing other trades even if one fails
			continue
		}
	}

	s.logger.Infow("Fallback trade processing completed", "trade_count", len(trades))
	return nil
}

// updateUserPositionFromTrade updates user position based on trade execution
func (s *AsyncRiskService) updateUserPositionFromTrade(ctx context.Context, trade *model.Trade) error {
	// This is a simplified implementation - in reality you'd need to:
	// 1. Identify the user from the trade
	// 2. Update their position in the database
	// 3. Recalculate risk metrics
	// 4. Update cached data

	// For now, just log the trade processing
	s.logger.Debugw("Processing trade in fallback mode",
		"trade_id", trade.ID,
		"symbol", trade.Pair,
		"quantity", trade.Quantity,
		"price", trade.Price,
	)

	// Simulate position update delay
	time.Sleep(time.Millisecond * 10)

	return nil
}
