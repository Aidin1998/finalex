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

	"github.com/Aidin1998/pincex_unified/internal/settlement"
	"github.com/Aidin1998/pincex_unified/internal/trading/eventjournal"
	"github.com/Aidin1998/pincex_unified/internal/trading/model"
	"github.com/Aidin1998/pincex_unified/internal/trading/orderbook"
	"github.com/Aidin1998/pincex_unified/internal/trading/trigger"
	"github.com/Aidin1998/pincex_unified/internal/ws"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

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

	// Monitoring and metrics
	metricsCollector   *EngineMetricsCollector
	performanceMonitor *EnginePerformanceMonitor

	// Control channels
	migrationControlChan chan MigrationControlMessage
	metricsReportChan    chan MetricsReport
	shutdownChan         chan struct{}

	// Auto-migration state
	autoMigrationEnabled int32 // atomic
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
	P99LatencyMs         float64       `json:"p99_latency_ms"`
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
) *AdaptiveMatchingEngine {
	// Before calling NewMatchingEngine, instantiate a settlement engine
	settlementEngine := settlement.NewSettlementEngine()
	
	// Create a placeholder trigger monitor for the base engine
	triggerMonitor := &trigger.TriggerMonitor{}
	
	// Create base engine with existing config
	baseEngine := NewMatchingEngine(orderRepo, tradeRepo, logger, config.Config, eventJournal, wsHub, dummyRiskManagerType{}, settlementEngine, triggerMonitor)

	ame := &AdaptiveMatchingEngine{
		MatchingEngine:       baseEngine,
		adaptiveConfig:       config,
		adaptiveOrderBooks:   make(map[string]*orderbook.AdaptiveOrderBook),
		migrationState:       make(map[string]*MigrationState),
		migrationControlChan: make(chan MigrationControlMessage, 100),
		metricsReportChan:    make(chan MetricsReport, 10),
		shutdownChan:         make(chan struct{}),
	}

	// Initialize monitoring if metrics collection is enabled
	if config.MetricsCollectionInterval > 0 {
		ame.metricsCollector = NewEngineMetricsCollector(config.MetricsCollectionInterval)
		ame.performanceMonitor = NewEnginePerformanceMonitor()
	}

	// Start background workers
	ame.startBackgroundWorkers()

	return ame
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
	ame.migrationMu.Lock()
	defer ame.migrationMu.Unlock()

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
	ame.migrationMu.RLock()
	defer ame.migrationMu.RUnlock()

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
	ame.migrationMu.Lock()
	state, exists := ame.migrationState[pair]
	if !exists {
		ame.migrationMu.Unlock()
		return fmt.Errorf("no migration state found for pair %s", pair)
	}
	if state.MigrationLocked && time.Since(state.LastUpdateTime).Nanoseconds() < ame.adaptiveConfig.MigrationCooldownPeriod.Nanoseconds() {
		ame.migrationMu.Unlock()
		return fmt.Errorf("migration for pair %s is locked (cooldown period)", pair)
	}

	state.TargetPercentage = targetPercentage
	state.Status = MigrationStatusStarting
	state.LastUpdateTime = time.Now()
	state.MigrationLocked = false
	ame.migrationMu.Unlock()

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
	ame.migrationMu.Lock()
	if state, exists := ame.migrationState[pair]; exists {
		state.CurrentPercentage = percentage
		state.TargetPercentage = percentage
		state.LastUpdateTime = time.Now()

		if percentage == 0 {
			state.Status = MigrationStatusInactive
		} else if percentage == 100 {
			state.Status = MigrationStatusCompleted
		} else {
			state.Status = MigrationStatusInProgress
		}
	}
	ame.migrationMu.Unlock()

	ame.logger.Infow("Migration percentage set",
		"pair", pair,
		"percentage", percentage,
	)

	return nil
}

// handleRollbackMigration rolls back migration for a pair
func (ame *AdaptiveMatchingEngine) handleRollbackMigration(pair string) error {
	adaptiveOB := ame.adaptiveOrderBooks[pair]
	if adaptiveOB == nil {
		return fmt.Errorf("no adaptive order book found for pair %s", pair)
	}

	// Set migration to 0% and disable new implementation
	adaptiveOB.SetMigrationPercentage(0)
	adaptiveOB.EnableNewImplementation(false)
	adaptiveOB.ResetCircuitBreaker()

	// Update migration state
	ame.migrationMu.Lock()
	if state, exists := ame.migrationState[pair]; exists {
		state.CurrentPercentage = 0
		state.TargetPercentage = 0
		state.Status = MigrationStatusInactive
		state.LastUpdateTime = time.Now()
		state.MigrationLocked = true // Lock to prevent immediate retry
	}
	ame.migrationMu.Unlock()

	ame.logger.Infow("Migration rolled back",
		"pair", pair,
	)

	return nil
}

// handlePauseMigration pauses migration for a pair
func (ame *AdaptiveMatchingEngine) handlePauseMigration(pair string) error {
	ame.migrationMu.Lock()
	defer ame.migrationMu.Unlock()

	if state, exists := ame.migrationState[pair]; exists {
		state.Status = MigrationStatusPaused
		state.LastUpdateTime = time.Now()
	}

	return nil
}

// handleResumeMigration resumes migration for a pair
func (ame *AdaptiveMatchingEngine) handleResumeMigration(pair string) error {
	ame.migrationMu.Lock()
	defer ame.migrationMu.Unlock()

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
	adaptiveOB := ame.adaptiveOrderBooks[pair]
	if adaptiveOB != nil {
		adaptiveOB.ResetCircuitBreaker()
	}

	ame.migrationMu.Lock()
	if state, exists := ame.migrationState[pair]; exists {
		state.ConsecutiveErrors = 0
		state.LastErrorTime = time.Time{}
		state.MigrationLocked = false
		state.LastUpdateTime = time.Now()
	}
	ame.migrationMu.Unlock()

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
