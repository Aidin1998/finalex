// =============================
// Trading Engine Integration for Adaptive Order Book
// =============================
// This file provides integration between the trading engine and the
// new adaptive order book system, allowing gradual migration and
// enhanced monitoring capabilities.

package engine

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/compliance/aml"
	"github.com/Aidin1998/pincex_unified/internal/redis"
	"github.com/Aidin1998/pincex_unified/internal/settlement"
	"github.com/Aidin1998/pincex_unified/internal/trading/eventjournal"
	"github.com/Aidin1998/pincex_unified/internal/trading/model"
	"github.com/Aidin1998/pincex_unified/internal/trading/orderbook"
	"github.com/Aidin1998/pincex_unified/internal/trading/trigger"
	"github.com/Aidin1998/pincex_unified/internal/ws"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// Async Risk Checking Data Structures

// RiskCheckRequest represents an async risk check request
type RiskCheckRequest struct {
	RequestID string         `json:"request_id"`
	Order     *model.Order   `json:"order"`
	Trades    []*model.Trade `json:"trades"`
	Timestamp time.Time      `json:"timestamp"`
	UserID    string         `json:"user_id"`
}

// RiskCheckResult represents the result of an async risk check
type RiskCheckResult struct {
	RequestID        string                `json:"request_id"`
	Approved         bool                  `json:"approved"`
	Reason           string                `json:"reason,omitempty"`
	RiskMetrics      *aml.RiskMetrics      `json:"risk_metrics,omitempty"`
	ComplianceResult *aml.ComplianceResult `json:"compliance_result,omitempty"`
	ProcessingTime   time.Duration         `json:"processing_time"`
	Timestamp        time.Time             `json:"timestamp"`
	Error            error                 `json:"error,omitempty"`
}

// PendingOrder tracks orders waiting for risk check completion
type PendingOrder struct {
	Order         *model.Order   `json:"order"`
	Trades        []*model.Trade `json:"trades"`
	RestingOrders []*model.Order `json:"resting_orders"`
	RequestID     string         `json:"request_id"`
	SubmittedAt   time.Time      `json:"submitted_at"`
	ExpiresAt     time.Time      `json:"expires_at"`
}

// RiskCheckConfig configures async risk checking behavior
type RiskCheckConfig struct {
	Enabled             bool          `json:"enabled"`
	Timeout             time.Duration `json:"timeout"`
	WorkerCount         int           `json:"worker_count"`
	BufferSize          int           `json:"buffer_size"`
	RetryAttempts       int           `json:"retry_attempts"`
	RetryDelay          time.Duration `json:"retry_delay"`
	EnableTradeReversal bool          `json:"enable_trade_reversal"`
	MaxPendingOrders    int           `json:"max_pending_orders"`
}

// DefaultRiskCheckConfig returns default configuration for async risk checking
func DefaultRiskCheckConfig() *RiskCheckConfig {
	return &RiskCheckConfig{
		Enabled:             true,
		Timeout:             time.Second * 10,       // 10 second timeout
		WorkerCount:         4,                      // 4 parallel workers
		BufferSize:          1000,                   // 1000 pending requests buffer
		RetryAttempts:       3,                      // 3 retry attempts
		RetryDelay:          time.Millisecond * 100, // 100ms retry delay
		EnableTradeReversal: true,                   // Enable trade reversal on risk failure
		MaxPendingOrders:    5000,                   // Max 5000 pending orders
	}
}

// AdaptiveEngineConfig extends the existing Config with adaptive features
type AdaptiveEngineConfig struct {
	*Config // Embed existing config

	// Migration settings
	EnableAdaptiveOrderBooks bool                                  `json:"enable_adaptive_order_books"`
	DefaultMigrationConfig   *orderbook.MigrationConfig            `json:"default_migration_config"`
	PairSpecificConfigs      map[string]*orderbook.MigrationConfig `json:"pair_specific_configs"`

	// Monitoring settings
	MetricsCollectionInterval time.Duration            `json:"metrics_collection_interval"`
	PerformanceReportInterval time.Duration            `json:"performance_report_interval"`
	AutoMigrationEnabled      bool                     `json:"auto_migration_enabled"`
	AutoMigrationThresholds   *AutoMigrationThresholds `json:"auto_migration_thresholds"`

	// Safety settings
	MaxMigrationPercentageStep int32         `json:"max_migration_percentage_step"`
	MigrationCooldownPeriod    time.Duration `json:"migration_cooldown_period"`
	FallbackOnHighErrorRate    bool          `json:"fallback_on_high_error_rate"`
	ErrorRateThreshold         float64       `json:"error_rate_threshold"`

	// Async Risk Checking settings
	RiskCheckConfig *RiskCheckConfig `json:"risk_check_config"`
}

// AutoMigrationThresholds defines thresholds for automatic migration decisions
type AutoMigrationThresholds struct {
	// Performance thresholds to trigger migration to new implementation
	LatencyP95ThresholdMs    float64 `json:"latency_p95_threshold_ms"`
	ThroughputDegradationPct float64 `json:"throughput_degradation_pct"`
	ErrorRateThreshold       float64 `json:"error_rate_threshold"`
	ContentionThreshold      int64   `json:"contention_threshold"`

	// Thresholds to trigger rollback to old implementation
	NewImplErrorRateThreshold   float64 `json:"new_impl_error_rate_threshold"`
	ConsecutiveFailureThreshold int32   `json:"consecutive_failure_threshold"`
	PerformanceDegradationPct   float64 `json:"performance_degradation_pct"`
}

// DefaultAdaptiveEngineConfig returns safe defaults for adaptive engine
func DefaultAdaptiveEngineConfig() *AdaptiveEngineConfig {
	return &AdaptiveEngineConfig{
		Config:                     &Config{}, // Use existing config defaults
		EnableAdaptiveOrderBooks:   false,     // Start conservatively
		DefaultMigrationConfig:     orderbook.DefaultMigrationConfig(),
		PairSpecificConfigs:        make(map[string]*orderbook.MigrationConfig),
		MetricsCollectionInterval:  time.Second * 10,
		PerformanceReportInterval:  time.Minute * 5,
		AutoMigrationEnabled:       false, // Manual migration initially
		MaxMigrationPercentageStep: 10,    // 10% max steps
		MigrationCooldownPeriod:    time.Minute * 5,
		FallbackOnHighErrorRate:    true,
		ErrorRateThreshold:         0.02, // 2% error rate threshold
		RiskCheckConfig:            DefaultRiskCheckConfig(),
		AutoMigrationThresholds: &AutoMigrationThresholds{
			LatencyP95ThresholdMs:       50.0,
			ThroughputDegradationPct:    20.0,
			ErrorRateThreshold:          0.01,
			ContentionThreshold:         100,
			NewImplErrorRateThreshold:   0.05,
			ConsecutiveFailureThreshold: 5,
			PerformanceDegradationPct:   30.0,
		},
	}
}

// AdaptiveMatchingEngine extends MatchingEngine with adaptive capabilities
type AdaptiveMatchingEngine struct {
	*MatchingEngine // Embed existing engine

	// Adaptive configuration
	adaptiveConfig *AdaptiveEngineConfig

	// Adaptive order books
	adaptiveOrderBooks map[string]*orderbook.AdaptiveOrderBook
	adaptiveMu         sync.RWMutex

	// Migration state tracking
	migrationState map[string]*MigrationState
	migrationMu    sync.RWMutex
	// Per-pair locks for migration and order book operations
	pairLocks map[string]*sync.RWMutex

	// Monitoring and metrics
	metricsCollector   *EngineMetricsCollector
	performanceMonitor *EnginePerformanceMonitor

	// Control channels
	migrationControlChan chan MigrationControlMessage
	metricsReportChan    chan MetricsReport
	shutdownChan         chan struct{}

	// Auto-migration state
	autoMigrationEnabled int32 // atomic

	// Async risk checking
	riskService        aml.RiskService
	asyncRiskService   *aml.AsyncRiskService  // Enhanced async service with Redis
	riskCheckConfig    *RiskCheckConfig
	pendingOrders      map[string]*PendingOrder
	riskResults        map[string]*RiskCheckResult
	riskMu             sync.RWMutex
	riskRequestChan    chan RiskCheckRequest
	riskResultChan     chan RiskCheckResult
	riskWorkerShutdown chan struct{}
	
	// Performance monitoring
	perfMonitor        *aml.PerformanceMonitor
	riskMgmtConfig     *aml.RiskManagementConfig
}

// MigrationState tracks the migration status for a trading pair
type MigrationState struct {
	Pair                  string                   `json:"pair"`
	CurrentPercentage     int32                    `json:"current_percentage"`
	TargetPercentage      int32                    `json:"target_percentage"`
	LastUpdateTime        time.Time                `json:"last_update_time"`
	Status                MigrationStatus          `json:"status"`
	Stats                 orderbook.MigrationStats `json:"stats"`
	PerformanceComparison *PerformanceComparison   `json:"performance_comparison"`

	// Control flags
	AutoMigrationEnabled bool      `json:"auto_migration_enabled"`
	MigrationLocked      bool      `json:"migration_locked"`
	LastErrorTime        time.Time `json:"last_error_time"`
	ConsecutiveErrors    int32     `json:"consecutive_errors"`
}

// MigrationStatus represents the current migration status
type MigrationStatus string

const (
	MigrationStatusInactive    MigrationStatus = "INACTIVE"
	MigrationStatusStarting    MigrationStatus = "STARTING"
	MigrationStatusInProgress  MigrationStatus = "IN_PROGRESS"
	MigrationStatusCompleted   MigrationStatus = "COMPLETED"
	MigrationStatusRollingBack MigrationStatus = "ROLLING_BACK"
	MigrationStatusFailed      MigrationStatus = "FAILED"
	MigrationStatusPaused      MigrationStatus = "PAUSED"
)

// PerformanceComparison holds performance comparison between implementations
type PerformanceComparison struct {
	OldImplMetrics        *ImplMetrics `json:"old_impl_metrics"`
	NewImplMetrics        *ImplMetrics `json:"new_impl_metrics"`
	ImprovementPct        float64      `json:"improvement_pct"`
	LatencyImprovementPct float64      `json:"latency_improvement_pct"`
	LastComparisonTime    time.Time    `json:"last_comparison_time"`
}

// ImplMetrics holds metrics for a specific implementation
type ImplMetrics struct {
	AvgLatencyMs         float64       `json:"avg_latency_ms"`
	P95LatencyMs         float64       `json:"p95_latency_ms"`
	ThroughputOps        float64       `json:"throughput_ops"`
	ErrorRate            float64       `json:"error_rate"`
	ContentionEvents     int64         `json:"contention_events"`
	SuccessfulOperations int64         `json:"successful_operations"`
	FailedOperations     int64         `json:"failed_operations"`
	SamplePeriod         time.Duration `json:"sample_period"`
}

// MigrationControlMessage represents a migration control command
type MigrationControlMessage struct {
	Type         MigrationControlType `json:"type"`
	Pair         string               `json:"pair"`
	Percentage   int32                `json:"percentage,omitempty"`
	Force        bool                 `json:"force,omitempty"`
	ResponseChan chan error           `json:"-"`
}

// MigrationControlType represents different migration control commands
type MigrationControlType string

const (
	MigrationControlStart      MigrationControlType = "START"
	MigrationControlStop       MigrationControlType = "STOP"
	MigrationControlPause      MigrationControlType = "PAUSE"
	MigrationControlResume     MigrationControlType = "RESUME"
	MigrationControlSetPercent MigrationControlType = "SET_PERCENT"
	MigrationControlRollback   MigrationControlType = "ROLLBACK"
	MigrationControlReset      MigrationControlType = "RESET"
)

// MetricsReport represents periodic metrics reporting
type MetricsReport struct {
	Timestamp          time.Time                  `json:"timestamp"`
	EngineMetrics      *EngineMetrics             `json:"engine_metrics"`
	PairMetrics        map[string]*PairMetrics    `json:"pair_metrics"`
	MigrationStates    map[string]*MigrationState `json:"migration_states"`
	OverallPerformance *OverallPerformanceMetrics `json:"overall_performance"`
}

// dummyRiskManagerType implements the riskManager interface but does nothing
// This is a type, not a variable.
type dummyRiskManagerType struct{}

func (dummyRiskManagerType) CheckPositionLimit(userID, symbol string, intendedQty float64) error {
	return nil
}

// NewAdaptiveMatchingEngine creates a new adaptive matching engine
func NewAdaptiveMatchingEngine(
	orderRepo model.Repository,
	tradeRepo TradeRepository,
	logger *zap.SugaredLogger,
	config *AdaptiveEngineConfig,
	eventJournal *eventjournal.EventJournal,
	wsHub *ws.Hub,
	riskService aml.RiskService,
	redisClient *redis.Client, // Add Redis client parameter
) *AdaptiveMatchingEngine {
	// Before calling NewMatchingEngine, instantiate a settlement engine
	settlementEngine := settlement.NewSettlementEngine()

	// Create a placeholder trigger monitor for the base engine
	triggerMonitor := &trigger.TriggerMonitor{}

	// Create base engine with existing config
	baseEngine := NewMatchingEngine(orderRepo, tradeRepo, logger, config.Config, eventJournal, wsHub, dummyRiskManagerType{}, settlementEngine, triggerMonitor)

	// Load risk management configuration
	riskMgmtConfig, err := aml.LoadRiskManagementConfig("")
	if err != nil {
		logger.Warnw("Failed to load risk management config, using defaults", "error", err)
		riskMgmtConfig = aml.GetDefaultConfig()
	}

	ame := &AdaptiveMatchingEngine{
		MatchingEngine:       baseEngine,
		adaptiveConfig:       config,
		adaptiveOrderBooks:   make(map[string]*orderbook.AdaptiveOrderBook),
		migrationState:       make(map[string]*MigrationState),
		pairLocks:            make(map[string]*sync.RWMutex),
		migrationControlChan: make(chan MigrationControlMessage, 100),
		metricsReportChan:    make(chan MetricsReport, 10),
		shutdownChan:         make(chan struct{}),
		riskService:          riskService,
		riskCheckConfig:      config.RiskCheckConfig,
		riskMgmtConfig:       riskMgmtConfig,
	}

	// Initialize async risk service if Redis is available and risk checking is enabled
	if redisClient != nil && config.RiskCheckConfig.Enabled {
		// Use configuration from risk-management.yaml
		asyncConfig := riskMgmtConfig.ToAsyncRiskConfig()
		
		// Override with any engine-specific config values
		if config.RiskCheckConfig.Timeout > 0 {
			asyncConfig.RiskCalculationTimeout = config.RiskCheckConfig.Timeout
		}
		if config.RiskCheckConfig.WorkerCount > 0 {
			asyncConfig.RiskWorkerCount = config.RiskCheckConfig.WorkerCount
		}
		
		var err error
		ame.asyncRiskService, err = aml.NewAsyncRiskService(
			riskService,
			redisClient,
			logger,
			asyncConfig,
		)
		if err != nil {
			logger.Errorw("Failed to initialize async risk service, falling back to synchronous", "error", err)
			ame.asyncRiskService = nil
		} else {
			logger.Infow("Async risk service initialized successfully",
				"workers", asyncConfig.RiskWorkerCount,
				"cache_enabled", asyncConfig.EnableCaching,
				"target_latency_ms", asyncConfig.TargetLatencyMs,
				"target_throughput", asyncConfig.TargetThroughputOPS)
			
			// Initialize performance monitoring
			ame.perfMonitor = aml.NewPerformanceMonitor(ame.asyncRiskService, logger)
		}
	}

	// Initialize monitoring if metrics collection is enabled
	if config.MetricsCollectionInterval > 0 {
		ame.metricsCollector = NewEngineMetricsCollector(config.MetricsCollectionInterval)
		ame.performanceMonitor = NewEnginePerformanceMonitor()
	}

	// Initialize risk checking if enabled
	if config.RiskCheckConfig.Enabled {
		ame.pendingOrders = make(map[string]*PendingOrder)
		ame.riskResults = make(map[string]*RiskCheckResult)
		ame.riskRequestChan = make(chan RiskCheckRequest, config.RiskCheckConfig.BufferSize)
		ame.riskResultChan = make(chan RiskCheckResult, config.RiskCheckConfig.BufferSize)
		ame.riskWorkerShutdown = make(chan struct{})
	}

	// Start background workers
	ame.startBackgroundWorkers()

	return ame
}

// getPairLock returns the per-pair RWMutex, falling back to global migrationMu if not found
func (ame *AdaptiveMatchingEngine) getPairLock(pair string) *sync.RWMutex {
	ame.adaptiveMu.RLock()
	lock, exists := ame.pairLocks[pair]
	ame.adaptiveMu.RUnlock()
	if !exists {
		return &ame.migrationMu
	}
	return lock
}

// GetOrderBook overrides the base method to return adaptive order book
func (ame *AdaptiveMatchingEngine) GetOrderBook(pair string) orderbook.OrderBookInterface {
	if !ame.adaptiveConfig.EnableAdaptiveOrderBooks {
		// Fall back to base implementation
		return ame.MatchingEngine.GetOrderBook(pair)
	}

	ame.adaptiveMu.RLock()
	adaptiveOB, exists := ame.adaptiveOrderBooks[pair]
	ame.adaptiveMu.RUnlock()

	if exists {
		return adaptiveOB
	}

	// Create new adaptive order book
	ame.adaptiveMu.Lock()
	defer ame.adaptiveMu.Unlock()

	// Double-check after acquiring write lock
	if adaptiveOB, exists = ame.adaptiveOrderBooks[pair]; exists {
		return adaptiveOB
	}

	// Get pair-specific config or use default
	migrationConfig := ame.adaptiveConfig.DefaultMigrationConfig
	if pairConfig, exists := ame.adaptiveConfig.PairSpecificConfigs[pair]; exists {
		migrationConfig = pairConfig
	}

	// Create adaptive order book
	adaptiveOB = orderbook.NewAdaptiveOrderBook(pair, migrationConfig)
	ame.adaptiveOrderBooks[pair] = adaptiveOB

	// Initialize migration state
	ame.initializeMigrationState(pair, migrationConfig)

	ame.logger.Infow("Created adaptive order book",
		"pair", pair,
		"migration_enabled", migrationConfig.EnableNewImplementation,
		"initial_percentage", migrationConfig.MigrationPercentage,
	)

	return adaptiveOB
}

// initializeMigrationState sets up initial migration state for a pair
func (ame *AdaptiveMatchingEngine) initializeMigrationState(pair string, config *orderbook.MigrationConfig) {
	// protect maps; initialize per-pair lock
	ame.adaptiveMu.Lock()
	ame.pairLocks[pair] = &sync.RWMutex{}
	ame.adaptiveMu.Unlock()

	// initialize state under per-pair lock
	lock := ame.getPairLock(pair)
	lock.Lock()
	defer lock.Unlock()

	ame.migrationState[pair] = &MigrationState{
		Pair:                 pair,
		CurrentPercentage:    int32(config.MigrationPercentage),
		TargetPercentage:     int32(config.MigrationPercentage),
		LastUpdateTime:       time.Now(),
		Status:               MigrationStatusInactive,
		AutoMigrationEnabled: ame.adaptiveConfig.AutoMigrationEnabled,
		PerformanceComparison: &PerformanceComparison{
			OldImplMetrics: &ImplMetrics{},
			NewImplMetrics: &ImplMetrics{},
		},
	}
}

// ProcessOrder overrides the base method with adaptive routing and monitoring
func (ame *AdaptiveMatchingEngine) ProcessOrder(ctx context.Context, order *model.Order, source OrderSourceType) (*model.Order, []*model.Trade, []*model.Order, error) {
	startTime := time.Now()

	// Get adaptive order book
	orderBook := ame.GetOrderBook(order.Pair)

	// Record metrics if collector is available
	if ame.metricsCollector != nil {
		defer func() {
			duration := time.Since(startTime)
			ame.metricsCollector.RecordOrderProcessing(order.Pair, duration, order.Type)
		}()
	}

	// Use base engine logic with adaptive order book
	return ame.processOrderWithAdaptiveBook(ctx, order, orderBook, source)
}

// processOrderWithAdaptiveBook processes an order using the adaptive order book
func (ame *AdaptiveMatchingEngine) processOrderWithAdaptiveBook(ctx context.Context, order *model.Order, orderBook orderbook.OrderBookInterface, source OrderSourceType) (*model.Order, []*model.Trade, []*model.Order, error) {
	// Validate order (reuse base engine validation)
	if order == nil {
		return nil, nil, nil, fmt.Errorf("order is nil")
	}
	if order.Pair == "" {
		return nil, nil, nil, fmt.Errorf("order pair is required")
	}

	// Check if market is paused (reuse base engine logic)
	if isMarketPaused(order.Pair) {
		return nil, nil, nil, fmt.Errorf("market %s is paused", order.Pair)
	}

	// Advanced order validation
	if err := order.ValidateAdvanced(); err != nil {
		return nil, nil, nil, fmt.Errorf("order validation failed: %w", err)
	}

	// Route to appropriate order type handler
	switch order.Type {
	case model.OrderTypeLimit:
		return ame.processLimitOrderAdaptive(ctx, order, orderBook)
	case model.OrderTypeMarket:
		return ame.processMarketOrderAdaptive(ctx, order, orderBook)
	case model.OrderTypeIOC:
		return ame.processIOCOrderAdaptive(ctx, order, orderBook)
	case model.OrderTypeFOK:
		return ame.processFOKOrderAdaptive(ctx, order, orderBook)
	default:
		// Fall back to base engine for advanced order types
		return ame.MatchingEngine.ProcessOrder(ctx, order, source)
	}
}

// processLimitOrderAdaptive processes a limit order with adaptive monitoring
func (ame *AdaptiveMatchingEngine) processLimitOrderAdaptive(ctx context.Context, order *model.Order, orderBook orderbook.OrderBookInterface) (*model.Order, []*model.Trade, []*model.Order, error) {
	result, err := orderBook.AddOrder(order)

	// Update order status based on fills
	if err == nil {
		if order.FilledQuantity.Equal(order.Quantity) {
			order.Status = model.OrderStatusFilled
		} else if order.FilledQuantity.GreaterThan(decimal.Zero) {
			order.Status = model.OrderStatusPartiallyFilled
		}
	}

	// Record operation metrics
	if ame.metricsCollector != nil {
		ame.metricsCollector.RecordOrderResult(order.Pair, err == nil, len(result.Trades))
	}

	return order, result.Trades, result.RestingOrders, err
}

// processMarketOrderAdaptive processes a market order with adaptive monitoring
func (ame *AdaptiveMatchingEngine) processMarketOrderAdaptive(ctx context.Context, order *model.Order, orderBook orderbook.OrderBookInterface) (*model.Order, []*model.Trade, []*model.Order, error) {
	result, err := orderBook.AddOrder(order)

	// Update order status based on fills
	if err == nil {
		if order.FilledQuantity.Equal(order.Quantity) {
			order.Status = model.OrderStatusFilled
		} else if order.FilledQuantity.GreaterThan(decimal.Zero) {
			order.Status = model.OrderStatusPartiallyFilled
		}
	}

	// Record operation metrics
	if ame.metricsCollector != nil {
		ame.metricsCollector.RecordOrderResult(order.Pair, err == nil, len(result.Trades))
	}

	return order, result.Trades, nil, err // Market orders don't rest
}

// processIOCOrderAdaptive processes an IOC order with adaptive monitoring
func (ame *AdaptiveMatchingEngine) processIOCOrderAdaptive(ctx context.Context, order *model.Order, orderBook orderbook.OrderBookInterface) (*model.Order, []*model.Trade, []*model.Order, error) {
	if order.TimeInForce == "" {
		order.TimeInForce = model.TimeInForceIOC
	}

	result, err := orderBook.AddOrder(order)

	// Update order status based on fills
	if err == nil {
		if order.FilledQuantity.Equal(order.Quantity) {
			order.Status = model.OrderStatusFilled
		} else if order.FilledQuantity.GreaterThan(decimal.Zero) {
			order.Status = model.OrderStatusPartiallyFilled
		}
	}

	// Record operation metrics
	if ame.metricsCollector != nil {
		ame.metricsCollector.RecordOrderResult(order.Pair, err == nil, len(result.Trades))
	}

	return order, result.Trades, nil, err // IOC orders don't rest
}

// processFOKOrderAdaptive processes a FOK order with adaptive monitoring
func (ame *AdaptiveMatchingEngine) processFOKOrderAdaptive(ctx context.Context, order *model.Order, orderBook orderbook.OrderBookInterface) (*model.Order, []*model.Trade, []*model.Order, error) {
	if order.TimeInForce == "" {
		order.TimeInForce = model.TimeInForceFOK
	}

	result, err := orderBook.AddOrder(order)

	// Update order status based on fills
	if err == nil {
		if order.FilledQuantity.Equal(order.Quantity) {
			order.Status = model.OrderStatusFilled
		} else if order.FilledQuantity.GreaterThan(decimal.Zero) {
			order.Status = model.OrderStatusPartiallyFilled
		}
	}

	// Record operation metrics
	if ame.metricsCollector != nil {
		ame.metricsCollector.RecordOrderResult(order.Pair, err == nil, len(result.Trades))
	}

	return order, result.Trades, nil, err // FOK orders don't rest
}

// Migration control methods

// StartMigration initiates migration for a specific pair
func (ame *AdaptiveMatchingEngine) StartMigration(pair string, targetPercentage int32) error {
	responseChan := make(chan error, 1)

	msg := MigrationControlMessage{
		Type:         MigrationControlStart,
		Pair:         pair,
		Percentage:   targetPercentage,
		ResponseChan: responseChan,
	}

	select {
	case ame.migrationControlChan <- msg:
		return <-responseChan
	case <-time.After(5 * time.Second):
		return fmt.Errorf("migration control timeout")
	}
}

// StopMigration stops migration for a specific pair
func (ame *AdaptiveMatchingEngine) StopMigration(pair string) error {
	responseChan := make(chan error, 1)
	msg := MigrationControlMessage{Type: MigrationControlStop, Pair: pair, ResponseChan: responseChan}
	select {
	case ame.migrationControlChan <- msg:
		return <-responseChan
	case <-time.After(5 * time.Second):
		return fmt.Errorf("migration control timeout")
	}
}

// SetMigrationPercentage sets the migration percentage for a pair
func (ame *AdaptiveMatchingEngine) SetMigrationPercentage(pair string, percentage int32) error {
	responseChan := make(chan error, 1)

	msg := MigrationControlMessage{
		Type:         MigrationControlSetPercent,
		Pair:         pair,
		Percentage:   percentage,
		ResponseChan: responseChan,
	}

	select {
	case ame.migrationControlChan <- msg:
		return <-responseChan
	case <-time.After(5 * time.Second):
		return fmt.Errorf("migration control timeout")
	}
}

// RollbackMigration rolls back migration for a pair
func (ame *AdaptiveMatchingEngine) RollbackMigration(pair string) error {
	responseChan := make(chan error, 1)

	msg := MigrationControlMessage{
		Type:         MigrationControlRollback,
		Pair:         pair,
		ResponseChan: responseChan,
	}

	select {
	case ame.migrationControlChan <- msg:
		return <-responseChan
	case <-time.After(5 * time.Second):
		return fmt.Errorf("migration control timeout")
	}
}

// PauseMigration pauses migration for a specific pair
func (ame *AdaptiveMatchingEngine) PauseMigration(pair string) error {
	responseChan := make(chan error, 1)
	msg := MigrationControlMessage{Type: MigrationControlPause, Pair: pair, ResponseChan: responseChan}
	select {
	case ame.migrationControlChan <- msg:
		return <-responseChan
	case <-time.After(5 * time.Second):
		return fmt.Errorf("migration control timeout")
	}
}

// ResumeMigration resumes migration for a specific pair
func (ame *AdaptiveMatchingEngine) ResumeMigration(pair string) error {
	responseChan := make(chan error, 1)
	msg := MigrationControlMessage{Type: MigrationControlResume, Pair: pair, ResponseChan: responseChan}
	select {
	case ame.migrationControlChan <- msg:
		return <-responseChan
	case <-time.After(5 * time.Second):
		return fmt.Errorf("migration control timeout")
	}
}

// ResetMigration resets migration state for a specific pair
func (ame *AdaptiveMatchingEngine) ResetMigration(pair string) error {
	responseChan := make(chan error, 1)
	msg := MigrationControlMessage{Type: MigrationControlReset, Pair: pair, ResponseChan: responseChan}
	select {
	case ame.migrationControlChan <- msg:
		return <-responseChan
	case <-time.After(5 * time.Second):
		return fmt.Errorf("migration control timeout")
	}
}

// GetMigrationState returns the current migration state for a pair
func (ame *AdaptiveMatchingEngine) GetMigrationState(pair string) (*MigrationState, error) {
	lock := ame.getPairLock(pair)
	lock.RLock()
	defer lock.RUnlock()

	state, exists := ame.migrationState[pair]
	if !exists {
		return nil, fmt.Errorf("no migration state found for pair %s", pair)
	}

	// Update with current adaptive order book stats
	if adaptiveOB, exists := ame.adaptiveOrderBooks[pair]; exists {
		state.Stats = adaptiveOB.GetMigrationStats()
	}

	return state, nil
}

// GetAllMigrationStates returns migration states for all pairs
func (ame *AdaptiveMatchingEngine) GetAllMigrationStates() map[string]*MigrationState {
	ame.migrationMu.RLock()
	defer ame.migrationMu.RUnlock()
	return ame.migrationState
}

// GetMetricsCollector returns the metrics collector
func (ame *AdaptiveMatchingEngine) GetMetricsCollector() *EngineMetricsCollector {
	return ame.metricsCollector
}

// SetAutoMigrationEnabled enables or disables auto-migration
func (ame *AdaptiveMatchingEngine) SetAutoMigrationEnabled(enabled bool) {
	var flag int32
	if enabled {
		flag = 1
	}
	atomic.StoreInt32(&ame.autoMigrationEnabled, flag)
}

// IsAutoMigrationEnabled returns whether auto-migration is enabled
func (ame *AdaptiveMatchingEngine) IsAutoMigrationEnabled() bool {
	return atomic.LoadInt32(&ame.autoMigrationEnabled) == 1
}

// GetMetricsReportChan returns the channel for metrics reports
func (ame *AdaptiveMatchingEngine) GetMetricsReportChan() <-chan MetricsReport {
	return ame.metricsReportChan
}

// Background workers

// startBackgroundWorkers starts all background worker goroutines
func (ame *AdaptiveMatchingEngine) startBackgroundWorkers() {
	// Migration control worker
	go ame.migrationControlWorker()

	// Metrics collection worker
	if ame.metricsCollector != nil {
		go ame.metricsCollectionWorker()
	}

	// Performance monitoring worker
	if ame.performanceMonitor != nil {
		go ame.performanceMonitoringWorker()
	}

	// Auto-migration worker
	go ame.autoMigrationWorker()

	// Metrics reporting worker
	go ame.metricsReportingWorker()

	// Risk checking workers
	if ame.adaptiveConfig.RiskCheckConfig.Enabled {
		// Start multiple risk checking workers
		for i := 0; i < ame.adaptiveConfig.RiskCheckConfig.WorkerCount; i++ {
			go ame.riskCheckWorker()
		}
		// Start risk result processor
		go ame.riskResultProcessor()
		// Start pending order cleanup worker
		go ame.pendingOrderCleanupWorker()
	}
}

// migrationControlWorker handles migration control messages
func (ame *AdaptiveMatchingEngine) migrationControlWorker() {
	for {
		select {
		case <-ame.shutdownChan:
			return
		case msg := <-ame.migrationControlChan:
			err := ame.handleMigrationControlMessage(msg)
			if msg.ResponseChan != nil {
				msg.ResponseChan <- err
			}
		}
	}
}

// handleMigrationControlMessage processes a migration control message
func (ame *AdaptiveMatchingEngine) handleMigrationControlMessage(msg MigrationControlMessage) error {
	switch msg.Type {
	case MigrationControlStart:
		return ame.handleStartMigration(msg.Pair, msg.Percentage)
	case MigrationControlSetPercent:
		return ame.handleSetMigrationPercentage(msg.Pair, msg.Percentage)
	case MigrationControlRollback:
		return ame.handleRollbackMigration(msg.Pair)
	case MigrationControlPause:
		return ame.handlePauseMigration(msg.Pair)
	case MigrationControlResume:
		return ame.handleResumeMigration(msg.Pair)
	case MigrationControlReset:
		return ame.handleResetMigration(msg.Pair)
	default:
		return fmt.Errorf("unknown migration control type: %s", msg.Type)
	}
}

// handleStartMigration starts migration for a pair
func (ame *AdaptiveMatchingEngine) handleStartMigration(pair string, targetPercentage int32) error {
	// Get or create adaptive order book
	orderBook := ame.GetOrderBook(pair)
	adaptiveOB, ok := orderBook.(*orderbook.AdaptiveOrderBook)
	if !ok {
		return fmt.Errorf("pair %s does not use adaptive order book", pair)
	}

	// Update migration state
	lock := ame.getPairLock(pair)
	lock.Lock()
	defer lock.Unlock()

	state, exists := ame.migrationState[pair]
	if !exists {
		lock.Unlock()
		return fmt.Errorf("no migration state found for pair %s", pair)
	}
	if state.MigrationLocked && time.Since(state.LastUpdateTime).Nanoseconds() < ame.adaptiveConfig.MigrationCooldownPeriod.Nanoseconds() {
		lock.Unlock()
		return fmt.Errorf("migration for pair %s is locked (cooldown period)", pair)
	}

	state.TargetPercentage = targetPercentage
	state.Status = MigrationStatusStarting
	state.LastUpdateTime = time.Now()
	state.MigrationLocked = false
	lock.Unlock()

	// Enable new implementation and start gradual migration
	adaptiveOB.EnableNewImplementation(true)
	adaptiveOB.SetMigrationPercentage(0) // Start at 0%

	ame.logger.Infow("Started migration",
		"pair", pair,
		"target_percentage", targetPercentage,
	)

	// Begin gradual migration in background
	go ame.executeGradualMigration(pair, targetPercentage)

	return nil
}

// executeGradualMigration performs gradual migration for a pair
func (ame *AdaptiveMatchingEngine) executeGradualMigration(pair string, targetPercentage int32) {
	adaptiveOB := ame.adaptiveOrderBooks[pair]
	if adaptiveOB == nil {
		return
	}

	currentPercentage := int32(0)
	stepSize := ame.adaptiveConfig.MaxMigrationPercentageStep

	for currentPercentage < targetPercentage {
		// Calculate next step
		nextPercentage := currentPercentage + stepSize
		if nextPercentage > targetPercentage {
			nextPercentage = targetPercentage
		}

		// Apply migration step
		adaptiveOB.SetMigrationPercentage(nextPercentage)

		// Update migration state
		ame.migrationMu.Lock()
		if state, exists := ame.migrationState[pair]; exists {
			state.CurrentPercentage = nextPercentage
			state.LastUpdateTime = time.Now()
			if nextPercentage == targetPercentage {
				state.Status = MigrationStatusCompleted
			} else {
				state.Status = MigrationStatusInProgress
			}
		}
		ame.migrationMu.Unlock()

		ame.logger.Infow("Migration step completed",
			"pair", pair,
			"current_percentage", nextPercentage,
			"target_percentage", targetPercentage,
		)

		currentPercentage = nextPercentage

		// Wait for step duration before next step
		if currentPercentage < targetPercentage {
			time.Sleep(ame.adaptiveConfig.MigrationCooldownPeriod)
		}
	}
}

// handleSetMigrationPercentage sets migration percentage directly
func (ame *AdaptiveMatchingEngine) handleSetMigrationPercentage(pair string, percentage int32) error {
	lock := ame.getPairLock(pair)
	lock.Lock()
	defer lock.Unlock()

	adaptiveOB := ame.adaptiveOrderBooks[pair]
	if adaptiveOB == nil {
		return fmt.Errorf("no adaptive order book found for pair %s", pair)
	}

	// Validate percentage
	if percentage < 0 || percentage > 100 {
		return fmt.Errorf("invalid migration percentage: %d", percentage)
	}

	// Update order book
	adaptiveOB.SetMigrationPercentage(percentage)

	// Update migration state
	if state, exists := ame.migrationState[pair]; exists {
		state.CurrentPercentage = percentage
		state.TargetPercentage = percentage
		state.LastUpdateTime = time.Now()

		switch {
		case percentage == 0:
			state.Status = MigrationStatusInactive
		case percentage == 100:
			state.Status = MigrationStatusCompleted
		default:
			state.Status = MigrationStatusInProgress
		}
	}

	ame.logger.Infow("Migration percentage set",
		"pair", pair,
		"percentage", percentage,
	)

	return nil
}

// handleRollbackMigration rolls back migration for a pair
func (ame *AdaptiveMatchingEngine) handleRollbackMigration(pair string) error {
	lock := ame.getPairLock(pair)
	lock.Lock()
	defer lock.Unlock()

	adaptiveOB := ame.adaptiveOrderBooks[pair]
	if adaptiveOB == nil {
		return fmt.Errorf("no adaptive order book found for pair %s", pair)
	}

	// Set migration to 0% and disable new implementation
	adaptiveOB.SetMigrationPercentage(0)
	adaptiveOB.EnableNewImplementation(false)
	adaptiveOB.ResetCircuitBreaker()

	// Update migration state
	if state, exists := ame.migrationState[pair]; exists {
		state.CurrentPercentage = 0
		state.TargetPercentage = 0
		state.Status = MigrationStatusInactive
		state.LastUpdateTime = time.Now()
		state.MigrationLocked = true // Lock to prevent immediate retry
	}
	ame.logger.Infow("Migration rolled back",
		"pair", pair,
	)

	return nil
}

// handlePauseMigration pauses migration for a pair
func (ame *AdaptiveMatchingEngine) handlePauseMigration(pair string) error {
	lock := ame.getPairLock(pair)
	lock.Lock()
	defer lock.Unlock()

	if state, exists := ame.migrationState[pair]; exists {
		state.Status = MigrationStatusPaused
		state.LastUpdateTime = time.Now()
	}

	return nil
}

// handleResumeMigration resumes migration for a pair
func (ame *AdaptiveMatchingEngine) handleResumeMigration(pair string) error {
	lock := ame.getPairLock(pair)
	lock.Lock()
	defer lock.Unlock()

	if state, exists := ame.migrationState[pair]; exists {
		if state.Status == MigrationStatusPaused {
			state.Status = MigrationStatusInProgress
			state.LastUpdateTime = time.Now()
		}
	}

	return nil
}

// handleResetMigration resets migration state for a pair
func (ame *AdaptiveMatchingEngine) handleResetMigration(pair string) error {
	lock := ame.getPairLock(pair)
	lock.Lock()
	defer lock.Unlock()

	if state, exists := ame.migrationState[pair]; exists {
		state.ConsecutiveErrors = 0
		state.LastErrorTime = time.Time{}
		state.MigrationLocked = false
		state.LastUpdateTime = time.Now()
	}
	ame.logger.Infow("Migration state reset",
		"pair", pair,
	)

	return nil
}

// Monitoring workers

// metricsCollectionWorker collects metrics periodically
func (ame *AdaptiveMatchingEngine) metricsCollectionWorker() {
	ticker := time.NewTicker(ame.adaptiveConfig.MetricsCollectionInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ame.shutdownChan:
			return
		case <-ticker.C:
			ame.collectAndAnalyzeMetrics()
		}
	}
}

// collectAndAnalyzeMetrics collects metrics from all adaptive order books
func (ame *AdaptiveMatchingEngine) collectAndAnalyzeMetrics() {
	ame.adaptiveMu.RLock()
	defer ame.adaptiveMu.RUnlock()

	for pair, adaptiveOB := range ame.adaptiveOrderBooks {
		// Collect performance metrics
		perfMetrics := adaptiveOB.GetPerformanceMetrics()
		cacheMetrics := adaptiveOB.GetSnapshotCacheMetrics()
		migrationStats := adaptiveOB.GetMigrationStats()

		// Update migration state with latest performance data
		ame.updatePerformanceComparison(pair, perfMetrics)

		// Record in metrics collector
		if ame.metricsCollector != nil {
			ame.metricsCollector.RecordPairMetrics(pair, perfMetrics, cacheMetrics, migrationStats)
		}

		// Check for auto-migration triggers
		if ame.IsAutoMigrationEnabled() {
			ame.checkAutoMigrationTriggers(pair, perfMetrics, migrationStats)
		}
	}
}

// updatePerformanceComparison updates performance comparison for a pair
func (ame *AdaptiveMatchingEngine) updatePerformanceComparison(pair string, perfMetrics map[string]interface{}) {
	ame.migrationMu.Lock()
	defer ame.migrationMu.Unlock()

	state, exists := ame.migrationState[pair]
	if !exists {
		return
	}

	// Extract metrics for old and new implementations
	// This would be implemented based on the actual structure of perfMetrics
	// For now, we'll create a placeholder implementation

	comparison := state.PerformanceComparison
	if comparison == nil {
		comparison = &PerformanceComparison{
			OldImplMetrics: &ImplMetrics{},
			NewImplMetrics: &ImplMetrics{},
		}
		state.PerformanceComparison = comparison
	}

	comparison.LastComparisonTime = time.Now()

	// Calculate improvement percentages
	if comparison.OldImplMetrics.ThroughputOps > 0 {
		comparison.ImprovementPct = ((comparison.NewImplMetrics.ThroughputOps - comparison.OldImplMetrics.ThroughputOps) / comparison.OldImplMetrics.ThroughputOps) * 100
	}

	if comparison.OldImplMetrics.AvgLatencyMs > 0 {
		comparison.LatencyImprovementPct = ((comparison.OldImplMetrics.AvgLatencyMs - comparison.NewImplMetrics.AvgLatencyMs) / comparison.OldImplMetrics.AvgLatencyMs) * 100
	}
}

// checkAutoMigrationTriggers checks if auto-migration should be triggered
func (ame *AdaptiveMatchingEngine) checkAutoMigrationTriggers(pair string, perfMetrics map[string]interface{}, migrationStats orderbook.MigrationStats) {
	thresholds := ame.adaptiveConfig.AutoMigrationThresholds

	ame.migrationMu.RLock()
	state, exists := ame.migrationState[pair]
	ame.migrationMu.RUnlock()

	if !exists || !state.AutoMigrationEnabled {
		return
	}

	// Check for migration triggers based on performance
	shouldMigrate := false
	shouldRollback := false

	// Check error rate threshold
	if migrationStats.TotalRequests > 0 {
		errorRate := float64(migrationStats.FailedRequests) / float64(migrationStats.TotalRequests)

		if errorRate > thresholds.NewImplErrorRateThreshold && state.CurrentPercentage > 0 {
			shouldRollback = true
			ame.logger.Warnw("Auto-migration rollback triggered by high error rate",
				"pair", pair,
				"error_rate", errorRate,
				"threshold", thresholds.NewImplErrorRateThreshold,
			)
		}
	}

	// Check consecutive failures
	if migrationStats.ConsecutiveFailures >= thresholds.ConsecutiveFailureThreshold && state.CurrentPercentage > 0 {
		shouldRollback = true
		ame.logger.Warnw("Auto-migration rollback triggered by consecutive failures",
			"pair", pair,
			"consecutive_failures", migrationStats.ConsecutiveFailures,
			"threshold", thresholds.ConsecutiveFailureThreshold,
		)
	}

	// Execute auto-migration decision
	if shouldRollback {
		ame.logger.Infow("Executing auto-rollback", "pair", pair)
		_ = ame.RollbackMigration(pair)
	} else if shouldMigrate && state.CurrentPercentage < 100 {
		// Gradually increase migration percentage
		newPercentage := state.CurrentPercentage + ame.adaptiveConfig.MaxMigrationPercentageStep
		if newPercentage > 100 {
			newPercentage = 100
		}

		ame.logger.Infow("Executing auto-migration step",
			"pair", pair,
			"new_percentage", newPercentage,
		)
		_ = ame.SetMigrationPercentage(pair, newPercentage)
	}
}

// performanceMonitoringWorker monitors overall engine performance
func (ame *AdaptiveMatchingEngine) performanceMonitoringWorker() {
	ticker := time.NewTicker(time.Minute) // Monitor every minute
	defer ticker.Stop()

	for {
		select {
		case <-ame.shutdownChan:
			return
		case <-ticker.C:
			ame.monitorOverallPerformance()
		}
	}
}

// monitorOverallPerformance monitors overall engine performance
func (ame *AdaptiveMatchingEngine) monitorOverallPerformance() {
	if ame.performanceMonitor == nil {
		return
	}

	// Collect overall performance metrics
	metrics := ame.performanceMonitor.CollectMetrics()

	// Log performance summary
	ame.logger.Infow("Engine performance summary",
		"total_orders_processed", metrics.TotalOrdersProcessed,
		"avg_processing_time_ms", metrics.AvgProcessingTimeMs,
		"throughput_ops_per_sec", metrics.ThroughputOpsPerSec,
		"active_migrations", len(ame.getActiveMigrations()),
	)
}

// getActiveMigrations returns pairs with active migrations
func (ame *AdaptiveMatchingEngine) getActiveMigrations() []string {
	ame.migrationMu.RLock()
	defer ame.migrationMu.RUnlock()

	var activeMigrations []string
	for pair, state := range ame.migrationState {
		if state.Status == MigrationStatusInProgress || state.Status == MigrationStatusStarting {
			activeMigrations = append(activeMigrations, pair)
		}
	}

	return activeMigrations
}

// autoMigrationWorker handles automatic migration decisions
func (ame *AdaptiveMatchingEngine) autoMigrationWorker() {
	ticker := time.NewTicker(time.Minute * 2) // Check every 2 minutes
	defer ticker.Stop()

	for {
		select {
		case <-ame.shutdownChan:
			return
		case <-ticker.C:
			ame.evaluateAutoMigrationOpportunities()
		}
	}
}

// evaluateAutoMigrationOpportunities evaluates opportunities for auto-migration
func (ame *AdaptiveMatchingEngine) evaluateAutoMigrationOpportunities() {
	ame.adaptiveMu.RLock()
	defer ame.adaptiveMu.RUnlock()

	for pair, adaptiveOB := range ame.adaptiveOrderBooks {
		migrationStats := adaptiveOB.GetMigrationStats()
		perfMetrics := adaptiveOB.GetPerformanceMetrics()

		// Evaluate if this pair is a good candidate for migration
		ame.checkAutoMigrationTriggers(pair, perfMetrics, migrationStats)
	}
}

// metricsReportingWorker generates periodic performance reports
func (ame *AdaptiveMatchingEngine) metricsReportingWorker() {
	ticker := time.NewTicker(ame.adaptiveConfig.PerformanceReportInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ame.shutdownChan:
			return
		case <-ticker.C:
			report := ame.generateMetricsReport()

			select {
			case ame.metricsReportChan <- report:
				// Report sent successfully
			default:
				// Channel full, skip this report
				ame.logger.Warnw("Metrics report channel full, skipping report")
			}
		}
	}
}

// generateMetricsReport generates a comprehensive metrics report
func (ame *AdaptiveMatchingEngine) generateMetricsReport() MetricsReport {
	report := MetricsReport{
		Timestamp:       time.Now(),
		PairMetrics:     make(map[string]*PairMetrics),
		MigrationStates: ame.GetAllMigrationStates(),
	}

	// Collect engine-wide metrics
	if ame.metricsCollector != nil {
		report.EngineMetrics = ame.metricsCollector.GetEngineMetrics()
	}

	// Collect per-pair metrics
	ame.adaptiveMu.RLock()
	for pair, adaptiveOB := range ame.adaptiveOrderBooks {
		pairMetrics := &PairMetrics{
			Pair:               pair,
			PerformanceMetrics: adaptiveOB.GetPerformanceMetrics(),
			CacheMetrics:       adaptiveOB.GetSnapshotCacheMetrics(),
			MigrationStats:     adaptiveOB.GetMigrationStats(),
		}
		report.PairMetrics[pair] = pairMetrics
	}
	ame.adaptiveMu.RUnlock()

	// Calculate overall performance
	report.OverallPerformance = ame.calculateOverallPerformance(report.PairMetrics)

	return report
}

// calculateOverallPerformance calculates overall engine performance metrics
func (ame *AdaptiveMatchingEngine) calculateOverallPerformance(pairMetrics map[string]*PairMetrics) *OverallPerformanceMetrics {
	overall := &OverallPerformanceMetrics{
		TotalPairs:           len(pairMetrics),
		TotalOrdersProcessed: 0,
		TotalTradesExecuted:  0,
		AvgLatencyMs:         0,
		OverallThroughput:    0,
		ActiveMigrations:     0,
	}

	var totalLatency float64
	var pairsWithLatency int

	for _, metrics := range pairMetrics {
		// Aggregate basic counts
		overall.TotalOrdersProcessed += metrics.MigrationStats.TotalRequests

		// Calculate weighted averages for latency and throughput
		if perfMap, ok := metrics.PerformanceMetrics.(map[string]interface{}); ok {
			if latency, exists := perfMap["avg_latency_ms"]; exists {
				if latencyFloat, ok := latency.(float64); ok && latencyFloat > 0 {
					totalLatency += latencyFloat
					pairsWithLatency++
				}
			}

			if throughput, exists := perfMap["throughput_ops"]; exists {
				if throughputFloat, ok := throughput.(float64); ok {
					overall.OverallThroughput += throughputFloat
				}
			}
		}

		// Count active migrations
		if metrics.MigrationStats.MigrationPercentage > 0 && metrics.MigrationStats.MigrationPercentage < 100 {
			overall.ActiveMigrations++
		}
	}

	// Calculate averages
	if pairsWithLatency > 0 {
		overall.AvgLatencyMs = totalLatency / float64(pairsWithLatency)
	}

	// Calculate migration health score (0-100)
	if overall.TotalPairs > 0 {
		successfulMigrations := overall.TotalPairs - overall.ActiveMigrations
		overall.MigrationHealthScore = float64(successfulMigrations) / float64(overall.TotalPairs) * 100
	}

	return overall
}

// Cleanup and shutdown

// Shutdown gracefully shuts down the adaptive matching engine
func (ame *AdaptiveMatchingEngine) Shutdown(ctx context.Context) error {
	ame.logger.Infow("Shutting down adaptive matching engine")

	// Signal all workers to stop
	close(ame.shutdownChan)

	// Wait for graceful shutdown with timeout
	shutdownComplete := make(chan struct{})
	go func() {
		// Perform cleanup operations
		ame.cleanupResources()
		close(shutdownComplete)
	}()

	select {
	case <-shutdownComplete:
		ame.logger.Infow("Adaptive matching engine shutdown completed")
		return nil
	case <-ctx.Done():
		ame.logger.Warnw("Adaptive matching engine shutdown timed out")
		return ctx.Err()
	}
}

// cleanupResources performs cleanup of engine resources
func (ame *AdaptiveMatchingEngine) cleanupResources() {
	// Stop metrics collection
	if ame.metricsCollector != nil {
		ame.metricsCollector.Stop()
	}

	// Stop performance monitoring
	if ame.performanceMonitor != nil {
		ame.performanceMonitor.Stop()
	}

	// Final metrics report
	finalReport := ame.generateMetricsReport()
	ame.logger.Infow("Final metrics report",
		"total_pairs", finalReport.OverallPerformance.TotalPairs,
		"total_orders", finalReport.OverallPerformance.TotalOrdersProcessed,
		"avg_latency_ms", finalReport.OverallPerformance.AvgLatencyMs,
		"overall_throughput", finalReport.OverallPerformance.OverallThroughput,
	)
}

// Additional data structures for metrics and monitoring

// EngineMetrics represents engine-wide metrics
type EngineMetrics struct {
	TotalOrdersProcessed int64     `json:"total_orders_processed"`
	TotalTradesExecuted  int64     `json:"total_trades_executed"`
	AvgProcessingTimeMs  float64   `json:"avg_processing_time_ms"`
	ThroughputOpsPerSec  float64   `json:"throughput_ops_per_sec"`
	ErrorRate            float64   `json:"error_rate"`
	LastUpdateTime       time.Time `json:"last_update_time"`
}

// PairMetrics represents metrics for a specific trading pair
type PairMetrics struct {
	Pair               string                   `json:"pair"`
	PerformanceMetrics interface{}              `json:"performance_metrics"`
	CacheMetrics       interface{}              `json:"cache_metrics"`
	MigrationStats     orderbook.MigrationStats `json:"migration_stats"`
}

// OverallPerformanceMetrics represents overall engine performance
type OverallPerformanceMetrics struct {
	TotalPairs           int     `json:"total_pairs"`
	TotalOrdersProcessed int64   `json:"total_orders_processed"`
	TotalTradesExecuted  int64   `json:"total_trades_executed"`
	AvgLatencyMs         float64 `json:"avg_latency_ms"`
	OverallThroughput    float64 `json:"overall_throughput"`
	ActiveMigrations     int     `json:"active_migrations"`
	MigrationHealthScore float64 `json:"migration_health_score"`
}

// Async Risk Checking Implementation

// riskCheckWorker processes async risk check requests
func (ame *AdaptiveMatchingEngine) riskCheckWorker() {
	for {
		select {
		case <-ame.riskWorkerShutdown:
			return
		case request := <-ame.riskRequestChan:
			result := ame.processRiskCheckRequest(request)

			select {
			case ame.riskResultChan <- result:
				// Result sent successfully
			case <-time.After(time.Second):
				// Timeout sending result, log error
				ame.logger.Errorw("Failed to send risk check result",
					"request_id", request.RequestID,
					"timeout", "1s",
				)
			}
		}
	}
}

// processRiskCheckRequest performs the actual risk check using async service when available
func (ame *AdaptiveMatchingEngine) processRiskCheckRequest(request RiskCheckRequest) RiskCheckResult {
	startTime := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), ame.riskCheckConfig.Timeout)
	defer cancel()

	result := RiskCheckResult{
		RequestID: request.RequestID,
		Timestamp: time.Now(),
		Approved:  false,
	}

	// Use async risk service if available for better performance
	if ame.asyncRiskService != nil {
		result = ame.processRiskCheckWithAsyncService(ctx, request, result, startTime)
	} else {
		// Fallback to synchronous processing
		result = ame.processRiskCheckSynchronous(ctx, request, result, startTime)
	}
	
	// Record performance metrics
	processingTime := time.Since(startTime)
	success := result.Approved && result.Error == nil
	timeout := processingTime > ame.riskCheckConfig.Timeout
	
	ame.RecordRiskCheckPerformance(processingTime, success, timeout)
	
	return result
}

// processRiskCheckWithAsyncService uses the enhanced async risk service
func (ame *AdaptiveMatchingEngine) processRiskCheckWithAsyncService(ctx context.Context, request RiskCheckRequest, result RiskCheckResult, startTime time.Time) RiskCheckResult {
	// Validate order using async service
	if request.Order != nil {
		approved, riskMetrics, err := ame.asyncRiskService.ValidateOrderAsync(ctx, request.Order)
		if err != nil {
			result.Error = err
			result.Reason = fmt.Sprintf("Async order validation failed: %v", err)
			result.ProcessingTime = time.Since(startTime)
			return result
		}

		result.RiskMetrics = riskMetrics
		if !approved {
			result.Reason = "Order validation failed - position limit or risk threshold exceeded"
			result.ProcessingTime = time.Since(startTime)
			return result
		}
	}

	// Process trades if any
	if len(request.Trades) > 0 {
		err := ame.asyncRiskService.ProcessTradeAsync(ctx, request.Trades)
		if err != nil {
			result.Error = err
			result.Reason = fmt.Sprintf("Async trade processing failed: %v", err)
			result.ProcessingTime = time.Since(startTime)
			return result
		}

		// Perform compliance checks on trades
		for _, trade := range request.Trades {
			tradeValue := trade.Quantity.Mul(trade.Price)
			
			// Create async compliance check request
			complianceCtx, cancel := context.WithTimeout(ctx, time.Millisecond*200) // Fast compliance check
			complianceResult, err := ame.riskService.ComplianceCheck(
				complianceCtx,
				trade.ID.String(),
				request.UserID,
				tradeValue,
				map[string]interface{}{
					"pair":     trade.Pair,
					"order_id": trade.OrderID.String(),
					"trade_id": trade.ID.String(),
					"quantity": trade.Quantity.String(),
					"price":    trade.Price.String(),
					"timestamp": trade.CreatedAt,
				},
			)
			cancel()

			if err != nil {
				result.Error = err
				result.Reason = fmt.Sprintf("Compliance check failed: %v", err)
				result.ProcessingTime = time.Since(startTime)
				return result
			}

			result.ComplianceResult = complianceResult

			// If compliance check fails, reject the risk check
			if complianceResult != nil && complianceResult.IsSuspicious {
				result.Reason = fmt.Sprintf("Compliance violation detected: %v", complianceResult.Flags)
				result.ProcessingTime = time.Since(startTime)
				return result
			}
		}
	}

	// Get updated risk metrics after processing
	if result.RiskMetrics == nil {
		riskMetrics, err := ame.asyncRiskService.CalculateRiskAsync(ctx, request.UserID)
		if err != nil {
			// Log warning but don't fail the request
			ame.logger.Warnw("Failed to get updated risk metrics", "user_id", request.UserID, "error", err)
		} else {
			result.RiskMetrics = riskMetrics
		}
	}

	// All checks passed
	result.Approved = true
	result.ProcessingTime = time.Since(startTime)
	return result
}

// processRiskCheckSynchronous performs synchronous risk checking (fallback)
func (ame *AdaptiveMatchingEngine) processRiskCheckSynchronous(ctx context.Context, request RiskCheckRequest, result RiskCheckResult, startTime time.Time) RiskCheckResult {
	// Perform position limit check
	if request.Order != nil {
		positionAllowed, err := ame.riskService.CheckPositionLimit(
			ctx,
			request.UserID,
			request.Order.Pair,
			request.Order.Quantity,
			request.Order.Price,
		)

		if err != nil {
			result.Error = err
			result.Reason = fmt.Sprintf("Position limit check failed: %v", err)
			result.ProcessingTime = time.Since(startTime)
			return result
		}

		if !positionAllowed {
			result.Reason = "Position limit exceeded"
			result.ProcessingTime = time.Since(startTime)
			return result
		}
	}

	// Calculate risk metrics
	riskMetrics, err := ame.riskService.CalculateRealTimeRisk(ctx, request.UserID)
	if err != nil {
		result.Error = err
		result.Reason = fmt.Sprintf("Risk calculation failed: %v", err)
		result.ProcessingTime = time.Since(startTime)
		return result
	}

	result.RiskMetrics = riskMetrics

	// Perform compliance check if trades were executed
	if len(request.Trades) > 0 {
		for _, trade := range request.Trades {
			tradeValue := trade.Quantity.Mul(trade.Price)
			complianceResult, err := ame.riskService.ComplianceCheck(
				ctx,
				trade.ID.String(),
				request.UserID,
				tradeValue,
				map[string]interface{}{
					"pair":     trade.Pair,
					"order_id": trade.OrderID.String(),
					"trade_id": trade.ID.String(),
					"quantity": trade.Quantity.String(),
					"price":    trade.Price.String(),
				},
			)

			if err != nil {
				result.Error = err
				result.Reason = fmt.Sprintf("Compliance check failed: %v", err)
				result.ProcessingTime = time.Since(startTime)
				return result
			}

			result.ComplianceResult = complianceResult

			// If compliance check fails, reject the risk check
			if complianceResult != nil && complianceResult.IsSuspicious {
				result.Reason = fmt.Sprintf("Compliance violation: %t", complianceResult.AlertRaised)
				result.ProcessingTime = time.Since(startTime)
				return result
			}
		}

		// Process trades in risk system
		for _, trade := range request.Trades {
			err := ame.riskService.ProcessTrade(
				ctx,
				trade.ID.String(),
				request.UserID,
				trade.Pair,
				trade.Quantity,
				trade.Price,
			)

			if err != nil {
				ame.logger.Errorw("Failed to process trade in risk system",
					"trade_id", trade.ID.String(),
					"user_id", request.UserID,
					"error", err,
				)
				// Continue processing - don't fail the entire risk check for this
			}
		}
	}

	// All checks passed
	result.Approved = true
	result.ProcessingTime = time.Since(startTime)

	ame.logger.Debugw("Risk check completed",
		"request_id", request.RequestID,
		"user_id", request.UserID,
		"approved", result.Approved,
		"processing_time", result.ProcessingTime,
	)

	return result
}

// riskResultProcessor processes async risk check results
func (ame *AdaptiveMatchingEngine) riskResultProcessor() {
	for {
		select {
		case <-ame.riskWorkerShutdown:
			return
		case result := <-ame.riskResultChan:
			ame.handleRiskCheckResult(result)
		}
	}
}

// handleRiskCheckResult handles the result of a risk check
func (ame *AdaptiveMatchingEngine) handleRiskCheckResult(result RiskCheckResult) {
	ame.riskMu.Lock()
	defer ame.riskMu.Unlock()

	// Store the result
	ame.riskResults[result.RequestID] = &result

	// Get the pending order
	pendingOrder, exists := ame.pendingOrders[result.RequestID]
	if !exists {
		ame.logger.Warnw("No pending order found for risk check result",
			"request_id", result.RequestID,
		)
		return
	}

	// Remove from pending orders
	delete(ame.pendingOrders, result.RequestID)

	if !result.Approved {
		// Risk check failed - cancel the order and reverse trades if necessary
		ame.handleRiskCheckFailure(pendingOrder, result)
	} else {
		// Risk check passed - log success
		ame.logger.Debugw("Risk check passed for order",
			"request_id", result.RequestID,
			"order_id", pendingOrder.Order.ID,
			"user_id", pendingOrder.Order.UserID,
			"processing_time", result.ProcessingTime,
		)
	}
}

// handleRiskCheckFailure handles a failed risk check
func (ame *AdaptiveMatchingEngine) handleRiskCheckFailure(pendingOrder *PendingOrder, result RiskCheckResult) {
	ame.logger.Warnw("Risk check failed for order",
		"request_id", result.RequestID,
		"order_id", pendingOrder.Order.ID,
		"user_id", pendingOrder.Order.UserID,
		"reason", result.Reason,
		"processing_time", result.ProcessingTime,
	)

	// Cancel the order
	err := ame.cancelOrderDueToRisk(pendingOrder.Order)
	if err != nil {
		ame.logger.Errorw("Failed to cancel order after risk check failure",
			"order_id", pendingOrder.Order.ID,
			"error", err,
		)
	}

	// Reverse trades if enabled and trades were executed
	if ame.riskCheckConfig.EnableTradeReversal && len(pendingOrder.Trades) > 0 {
		err := ame.reverseTradesForRiskFailure(pendingOrder.Trades, result.Reason)
		if err != nil {
			ame.logger.Errorw("Failed to reverse trades after risk check failure",
				"request_id", result.RequestID,
				"trade_count", len(pendingOrder.Trades),
				"error", err,
			)
		}
	}
}

// cancelOrderDueToRisk cancels an order due to risk check failure
func (ame *AdaptiveMatchingEngine) cancelOrderDueToRisk(order *model.Order) error {
	// Update order status to cancelled
	order.Status = model.OrderStatusCancelled

	// Get the order book and remove any remaining quantity
	orderBook := ame.GetOrderBook(order.Pair)

	// Try to cancel the order from the order book
	err := orderBook.CancelOrder(order.ID)
	if err != nil {
		// Order might have been fully filled already, which is okay
		ame.logger.Debugw("Could not cancel order from book (might be fully filled)",
			"order_id", order.ID,
			"error", err,
		)
	}

	// Save the updated order
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	err = ame.orderRepo.UpdateOrder(ctx, order)
	if err != nil {
		return fmt.Errorf("failed to update cancelled order: %w", err)
	}

	ame.logger.Infow("Order cancelled due to risk check failure",
		"order_id", order.ID,
		"user_id", order.UserID,
		"pair", order.Pair,
	)

	return nil
}

// reverseTradesForRiskFailure reverses trades when risk check fails
func (ame *AdaptiveMatchingEngine) reverseTradesForRiskFailure(trades []*model.Trade, reason string) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	var reversalErrors []error

	for _, trade := range trades {
		// Create a reversal trade
		reversalTrade := &model.Trade{
			ID:        uuid.New(),
			OrderID:   trade.OrderID,
			Pair:      trade.Pair,
			Quantity:  trade.Quantity.Neg(), // Negative quantity for reversal
			Price:     trade.Price,
			Side:      getOppositeSide(trade.Side),
			CreatedAt: time.Now(),
		}

		// Save the reversal trade
		err := ame.tradeRepo.CreateTrade(ctx, reversalTrade)
		if err != nil {
			reversalErrors = append(reversalErrors, fmt.Errorf("failed to create reversal trade for %s: %w", trade.ID, err))
			continue
		}

		ame.logger.Infow("Trade reversed due to risk check failure",
			"original_trade_id", trade.ID,
			"reversal_trade_id", reversalTrade.ID,
			"reason", reason,
		)
	}

	if len(reversalErrors) > 0 {
		return fmt.Errorf("trade reversal errors: %v", reversalErrors)
	}

	return nil
}

// getOppositeSide returns the opposite side for trade reversal
func getOppositeSide(side string) string {
	if side == model.OrderSideBuy {
		return model.OrderSideSell
	}
	return model.OrderSideBuy
}

// pendingOrderCleanupWorker cleans up expired pending orders
func (ame *AdaptiveMatchingEngine) pendingOrderCleanupWorker() {
	ticker := time.NewTicker(time.Minute) // Clean up every minute
	defer ticker.Stop()

	for {
		select {
		case <-ame.riskWorkerShutdown:
			return
		case <-ticker.C:
			ame.cleanupExpiredPendingOrders()
		}
	}
}

// cleanupExpiredPendingOrders removes expired pending orders
func (ame *AdaptiveMatchingEngine) cleanupExpiredPendingOrders() {
	ame.riskMu.Lock()
	defer ame.riskMu.Unlock()

	now := time.Now()
	var expiredRequestIDs []string

	for requestID, pendingOrder := range ame.pendingOrders {
		if now.After(pendingOrder.ExpiresAt) {
			expiredRequestIDs = append(expiredRequestIDs, requestID)
		}
	}

	for _, requestID := range expiredRequestIDs {
		pendingOrder := ame.pendingOrders[requestID]
		delete(ame.pendingOrders, requestID)

		ame.logger.Warnw("Pending order expired before risk check completion",
			"request_id", requestID,
			"order_id", pendingOrder.Order.ID,
			"submitted_at", pendingOrder.SubmittedAt,
			"expires_at", pendingOrder.ExpiresAt,
		)

		// Cancel the expired order
		err := ame.cancelOrderDueToRisk(pendingOrder.Order)
		if err != nil {
			ame.logger.Errorw("Failed to cancel expired pending order",
				"order_id", pendingOrder.Order.ID,
				"error", err,
			)
		}
	}

	if len(expiredRequestIDs) > 0 {
		ame.logger.Infow("Cleaned up expired pending orders",
			"count", len(expiredRequestIDs),
		)
	}
}

// submitAsyncRiskCheck submits an order for async risk checking
func (ame *AdaptiveMatchingEngine) submitAsyncRiskCheck(order *model.Order, trades []*model.Trade) (string, error) {
	if !ame.riskCheckConfig.Enabled {
		return "", nil // Risk checking disabled
	}

	// Check if we're at max pending orders limit
	ame.riskMu.RLock()
	pendingCount := len(ame.pendingOrders)
	ame.riskMu.RUnlock()

	if pendingCount >= ame.riskCheckConfig.MaxPendingOrders {
		return "", fmt.Errorf("maximum pending orders limit reached (%d)", ame.riskCheckConfig.MaxPendingOrders)
	}

	// Generate unique request ID
	requestID := fmt.Sprintf("risk_%s_%d", order.ID, time.Now().UnixNano())

	// Create risk check request
	request := RiskCheckRequest{
		RequestID: requestID,
		Order:     order,
		Trades:    trades,
		Timestamp: time.Now(),
		UserID:    order.UserID.String(),
	}

	// Create pending order tracking
	pendingOrder := &PendingOrder{
		Order:       order,
		Trades:      trades,
		RequestID:   requestID,
		SubmittedAt: time.Now(),
		ExpiresAt:   time.Now().Add(ame.riskCheckConfig.Timeout * 2), // Double timeout for expiry
	}

	// Store pending order
	ame.riskMu.Lock()
	ame.pendingOrders[requestID] = pendingOrder
	ame.riskMu.Unlock()

	// Submit risk check request
	select {
	case ame.riskRequestChan <- request:
		ame.logger.Debugw("Async risk check submitted",
			"request_id", requestID,
			"order_id", order.ID,
			"user_id", order.UserID,
			"trade_count", len(trades),
		)
		return requestID, nil
	case <-time.After(time.Millisecond * 100):
		// Remove from pending orders if we can't submit
		ame.riskMu.Lock()
		delete(ame.pendingOrders, requestID)
		ame.riskMu.Unlock()

		return "", fmt.Errorf("risk check request queue full")
	}
}

// GetRiskCheckStatus returns the status of a risk check
func (ame *AdaptiveMatchingEngine) GetRiskCheckStatus(requestID string) (*RiskCheckResult, bool) {
	if !ame.riskCheckConfig.Enabled {
		return nil, false
	}

	ame.riskMu.RLock()
	defer ame.riskMu.RUnlock()

	result, exists := ame.riskResults[requestID]
	return result, exists
}

// GetPendingOrdersCount returns the count of pending orders awaiting risk check
func (ame *AdaptiveMatchingEngine) GetPendingOrdersCount() int {
	if !ame.riskCheckConfig.Enabled {
		return 0
	}

	ame.riskMu.RLock()
	defer ame.riskMu.RUnlock()

	return len(ame.pendingOrders)
}

// GetAsyncRiskMetrics returns performance metrics from the async risk service
func (ame *AdaptiveMatchingEngine) GetAsyncRiskMetrics() *aml.AsyncMetrics {
	if ame.asyncRiskService != nil {
		return ame.asyncRiskService.GetMetrics()
	}
	return nil
}

// IsAsyncRiskEnabled returns true if async risk service is enabled and operational
func (ame *AdaptiveMatchingEngine) IsAsyncRiskEnabled() bool {
	return ame.asyncRiskService != nil
}

// GetAsyncRiskServiceHealthStatus returns health status of async risk service
func (ame *AdaptiveMatchingEngine) GetAsyncRiskServiceHealthStatus() map[string]interface{} {
	if ame.asyncRiskService == nil {
		return map[string]interface{}{
			"status": "unavailable",
			"reason": "async risk service not initialized",
		}
	}
	
	if ame.perfMonitor == nil {
		return map[string]interface{}{
			"status": "degraded",
			"reason": "performance monitor not available",
		}
	}
	
	// Get detailed health status
	healthStatus := ame.perfMonitor.getHealthStatus()
	
	return map[string]interface{}{
		"status":              healthStatus.Status,
		"async_service":       healthStatus.AsyncService,
		"workers":             healthStatus.Workers,
		"queue_depth":         healthStatus.QueueDepth,
		"response_time_ms":    float64(healthStatus.ResponseTime.Nanoseconds()) / 1e6,
		"last_error":          healthStatus.LastError,
		"last_error_time":     healthStatus.LastErrorTime,
		"dependencies":        healthStatus.Dependencies,
		"timestamp":           healthStatus.Timestamp,
	}
}

// GetPerformanceMetrics returns comprehensive performance metrics
func (ame *AdaptiveMatchingEngine) GetPerformanceMetrics() map[string]interface{} {
	if ame.perfMonitor == nil {
		return map[string]interface{}{
			"error": "performance monitoring not available",
		}
	}
	
	return ame.perfMonitor.generatePerformanceReport()
}

// RegisterPerformanceEndpoints registers performance monitoring endpoints with a router
func (ame *AdaptiveMatchingEngine) RegisterPerformanceEndpoints(router interface{}) error {
	if ame.perfMonitor == nil {
		return fmt.Errorf("performance monitor not initialized")
	}
	
	// Type assertion to gin.Engine
	if ginRouter, ok := router.(*gin.Engine); ok {
		aml.RegisterMonitoringEndpoints(ginRouter, ame.perfMonitor)
		return nil
	}
	
	return fmt.Errorf("unsupported router type")
}

// RecordRiskCheckPerformance records performance metrics for risk checks
func (ame *AdaptiveMatchingEngine) RecordRiskCheckPerformance(latency time.Duration, success bool, timeout bool) {
	if ame.perfMonitor != nil {
		ame.perfMonitor.RecordRequest(latency, success, timeout)
	}
}

// Async Risk Checking Integration

// submitAsyncRiskCheckIntegration submits an order for async risk checking with integration
func (ame *AdaptiveMatchingEngine) submitAsyncRiskCheckIntegration(order *model.Order, trades []*model.Trade) (string, error) {
	if !ame.riskCheckConfig.Enabled {
		return "", nil // Risk checking disabled
	}

	// Check if we're at max pending orders limit
	ame.riskMu.RLock()
	pendingCount := len(ame.pendingOrders)
	ame.riskMu.RUnlock()

	if pendingCount >= ame.riskCheckConfig.MaxPendingOrders {
		return "", fmt.Errorf("maximum pending orders limit reached (%d)", ame.riskCheckConfig.MaxPendingOrders)
	}

	// Generate unique request ID
	requestID := fmt.Sprintf("risk_%s_%d", order.ID, time.Now().UnixNano())

	// Create risk check request
	request := RiskCheckRequest{
		RequestID: requestID,
		Order:     order,
		Trades:    trades,
		Timestamp: time.Now(),
		UserID:    order.UserID.String(),
	}

	// Create pending order tracking
	pendingOrder := &PendingOrder{
		Order:       order,
		Trades:      trades,
		RequestID:   requestID,
		SubmittedAt: time.Now(),
		ExpiresAt:   time.Now().Add(ame.riskCheckConfig.Timeout * 2), // Double timeout for expiry
	}

	// Store pending order
	ame.riskMu.Lock()
	ame.pendingOrders[requestID] = pendingOrder
	ame.riskMu.Unlock()

	// Submit risk check request to async service
	if ame.asyncRiskService != nil {
		ame.logger.Debugw("Submitting async risk check to Redis",
			"request_id", requestID,
			"order_id", order.ID,
			"user_id", order.UserID,
			"trade_count", len(trades),
		)

		// Submit to Redis stream
		err := ame.asyncRiskService.SubmitRiskCheck(request)
		if err != nil {
			ame.logger.Errorw("Failed to submit risk check to Redis",
				"request_id", requestID,
				"error", err,
			)

			// Cleanup pending order on failure
			ame.riskMu.Lock()
			delete(ame.pendingOrders, requestID)
			ame.riskMu.Unlock()

			return "", fmt.Errorf("failed to submit risk check: %w", err)
		}

		return requestID, nil
	}

	return "", fmt.Errorf("async risk service not available")
}

// processAsyncRiskCheckResult processes the result of an async risk check from Redis
func (ame *AdaptiveMatchingEngine) processAsyncRiskCheckResult(result RiskCheckResult) {
	ame.riskMu.Lock()
	defer ame.riskMu.Unlock()

	// Store the result
	ame.riskResults[result.RequestID] = &result

	// Get the pending order
	pendingOrder, exists := ame.pendingOrders[result.RequestID]
	if !exists {
		ame.logger.Warnw("No pending order found for async risk check result",
			"request_id", result.RequestID,
		)
		return
	}

	// Remove from pending orders
	delete(ame.pendingOrders, result.RequestID)

	if !result.Approved {
		// Risk check failed - cancel the order and reverse trades if necessary
		ame.handleRiskCheckFailure(pendingOrder, result)
	} else {
		// Risk check passed - log success
		ame.logger.Debugw("Async risk check passed for order",
			"request_id", result.RequestID,
			"order_id", pendingOrder.Order