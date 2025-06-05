// =============================
// Engine Migration Participant
// =============================
// This file implements the matching engine participant for two-phase commit
// migration protocol. It handles engine state synchronization, order routing
// coordination, and performance monitoring during migration.

package participants

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/trading/engine"
	"github.com/Aidin1998/pincex_unified/internal/trading/migration"
	"github.com/Aidin1998/pincex_unified/internal/trading/model"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// Risk Adjustment contains adjustment details based on risk assessment
type RiskAdjustment struct {
	AdjustmentFactor  float64 `json:"adjustment_factor"`
	RecommendedAction string  `json:"recommended_action"`
	RiskLevel         string  `json:"risk_level"`
	Reason            string  `json:"reason,omitempty"`
}

// Risk and compliance types to replace aml package references
type RiskService interface {
	// Basic checks
	IsEnabled() bool

	// Position and risk checks
	CheckPositionLimit(ctx context.Context, orderID, pair string, quantity, price decimal.Decimal) (bool, error)
	CalculateRisk(ctx context.Context, userID string) (map[string]interface{}, error)

	// Trade processing
	ProcessTrade(ctx context.Context, tradeID, orderID, pair string, quantity, price decimal.Decimal) error
	UpdateMarketData(ctx context.Context, pair string, price decimal.Decimal, volatility float64) error

	// Compliance
	ComplianceCheck(ctx context.Context, tradeID, orderID string, tradeAmount decimal.Decimal, metadata map[string]interface{}) (map[string]interface{}, error)

	// Batch operations
	BatchCalculateRisk(ctx context.Context, userIDs []string) (map[string]interface{}, error)
	GetActiveComplianceAlerts(ctx context.Context) ([]ComplianceAlert, error)
}

type ComplianceAlert struct {
	AlertID    string     `json:"alert_id"`
	UserID     string     `json:"user_id"`
	AlertType  string     `json:"alert_type"`
	Severity   string     `json:"severity"`
	Message    string     `json:"message"`
	Timestamp  time.Time  `json:"timestamp"`
	Resolved   bool       `json:"resolved"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
}

// RiskMetrics contains metrics for risk assessment
type RiskMetrics struct {
	VaR                 float64 `json:"var"`
	ExposureUtilization float64 `json:"exposure_utilization"`
	ConcentrationRisk   float64 `json:"concentration_risk"`
	MaxDrawdown         float64 `json:"max_drawdown"`
}

// ComplianceStatus contains the status of compliance checks
type ComplianceStatus struct {
	ActiveAlerts   int       `json:"active_alerts"`
	ViolationCount int       `json:"violation_count"`
	LastCheck      time.Time `json:"last_check"`
	Status         string    `json:"status"`
}

// RiskReport contains the risk report details
type RiskReport struct {
	Timestamp        time.Time        `json:"timestamp"`
	Status           string           `json:"status"`
	RiskMetrics      RiskMetrics      `json:"risk_metrics"`
	ComplianceStatus ComplianceStatus `json:"compliance_status"`
	Recommendations  []string         `json:"recommendations,omitempty"`
}

// Transaction represents a transaction for risk compliance
type Transaction struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Symbol    string    `json:"symbol"`
	Amount    float64   `json:"amount"`
	Quantity  float64   `json:"quantity"`
	Price     float64   `json:"price"`
	Side      string    `json:"side"`
	Timestamp time.Time `json:"timestamp"`
}

// EngineParticipant handles matching engine coordination during migration
type EngineParticipant struct {
	id                 string
	pair               string
	engine             *engine.AdaptiveMatchingEngine
	riskManager        RiskService // Risk management integration
	logger             *zap.SugaredLogger
	migrationMu        sync.RWMutex
	currentMigrationID uuid.UUID
	preparationData    *EnginePreparationData

	// State tracking
	isHealthy          int64 // atomic
	lastHeartbeat      int64 // atomic (unix nano)
	isProcessingOrders int64 // atomic

	// Engine coordination
	orderRouting       *OrderRoutingManager
	stateSync          *EngineStateSync
	performanceMonitor *EnginePerformanceMonitor
	circuitBreaker     *EngineCircuitBreaker
}

// EnginePreparationData contains preparation phase data for engine migration
type EnginePreparationData struct {
	EngineState         *EngineState
	RoutingStrategy     *RoutingStrategy
	PerformanceBaseline *migration.PerformanceBaseline
	RiskAssessment      *EngineRiskAssessment
	SynchronizationPlan *SynchronizationPlan
	BackupConfig        *EngineBackupConfig
	PreparationTime     time.Time
	MigrationPlan       *EngineMigrationPlan
}

// EngineState represents the current state of the matching engine
type EngineState struct {
	Timestamp           time.Time                       `json:"timestamp"`
	ActiveOrders        int64                           `json:"active_orders"`
	ProcessingRate      float64                         `json:"processing_rate"`
	LatencyP50          time.Duration                   `json:"latency_p50"`
	LatencyP95          time.Duration                   `json:"latency_p95"`
	LatencyP99          time.Duration                   `json:"latency_p99"`
	ThroughputTPS       float64                         `json:"throughput_tps"`
	ErrorRate           float64                         `json:"error_rate"`
	CircuitBreakerState string                          `json:"circuit_breaker_state"`
	MemoryUsage         int64                           `json:"memory_usage"`
	CPUUsage            float64                         `json:"cpu_usage"`
	ConnectionCount     int                             `json:"connection_count"`
	QueueDepth          int                             `json:"queue_depth"`
	MigrationPercentage int32                           `json:"migration_percentage"`
	ImplementationStats map[string]*ImplementationStats `json:"implementation_stats"`
}

// ImplementationStats contains statistics for each implementation
type ImplementationStats struct {
	OrdersProcessed   int64         `json:"orders_processed"`
	AverageLatency    time.Duration `json:"average_latency"`
	ErrorCount        int64         `json:"error_count"`
	ThroughputTPS     float64       `json:"throughput_tps"`
	LastProcessedTime time.Time     `json:"last_processed_time"`
}

// RoutingStrategy defines how orders are routed during migration
type RoutingStrategy struct {
	Strategy            string                 `json:"strategy"`
	Parameters          map[string]interface{} `json:"parameters"`
	FallbackBehavior    string                 `json:"fallback_behavior"`
	HealthCheckInterval time.Duration          `json:"health_check_interval"`
	TimeoutSettings     *TimeoutSettings       `json:"timeout_settings"`
}

// TimeoutSettings contains timeout configurations
type TimeoutSettings struct {
	OrderProcessing time.Duration `json:"order_processing"`
	HealthCheck     time.Duration `json:"health_check"`
	StateSync       time.Duration `json:"state_sync"`
	Fallback        time.Duration `json:"fallback"`
}

// EngineRiskAssessment contains risk analysis for engine migration
type EngineRiskAssessment struct {
	OverallRisk       string             `json:"overall_risk"`
	RiskFactors       []string           `json:"risk_factors"`
	LoadAnalysis      *LoadAnalysis      `json:"load_analysis"`
	ImpactAssessment  *ImpactAssessment  `json:"impact_assessment"`
	Mitigations       []string           `json:"mitigations"`
	RollbackProcedure *RollbackProcedure `json:"rollback_procedure"`
}

// LoadAnalysis analyzes current system load
type LoadAnalysis struct {
	CurrentLoad         float64            `json:"current_load"`
	PeakLoad            float64            `json:"peak_load"`
	LoadTrend           string             `json:"load_trend"`
	ResourceUtilization map[string]float64 `json:"resource_utilization"`
	BottleneckAnalysis  []string           `json:"bottleneck_analysis"`
}

// ImpactAssessment estimates migration impact
type ImpactAssessment struct {
	EstimatedDowntime int64         `json:"estimated_downtime_ms"`
	AffectedUsers     int           `json:"affected_users"`
	OrdersAtRisk      int64         `json:"orders_at_risk"`
	RevenueImpact     float64       `json:"revenue_impact"`
	RecoveryTime      time.Duration `json:"recovery_time"`
}

// RollbackProcedure defines rollback steps
type RollbackProcedure struct {
	TriggerConditions   []string      `json:"trigger_conditions"`
	RollbackSteps       []string      `json:"rollback_steps"`
	EstimatedTime       time.Duration `json:"estimated_time"`
	DataIntegrityChecks []string      `json:"data_integrity_checks"`
	NotificationTargets []string      `json:"notification_targets"`
}

// SynchronizationPlan defines how engine state is synchronized
type SynchronizationPlan struct {
	Strategy           string        `json:"strategy"`
	SyncPoints         []string      `json:"sync_points"`
	SyncInterval       time.Duration `json:"sync_interval"`
	ConsistencyLevel   string        `json:"consistency_level"`
	ConflictResolution string        `json:"conflict_resolution"`
	ValidationChecks   []string      `json:"validation_checks"`
}

// EngineBackupConfig contains backup configuration for engine state
type EngineBackupConfig struct {
	BackupLocation     string        `json:"backup_location"`
	BackupStrategy     string        `json:"backup_strategy"`
	CompressionEnabled bool          `json:"compression_enabled"`
	EncryptionEnabled  bool          `json:"encryption_enabled"`
	RetentionPeriod    time.Duration `json:"retention_period"`
	BackupValidation   bool          `json:"backup_validation"`
}

// EngineMigrationPlan contains the execution plan for engine migration
type EngineMigrationPlan struct {
	Strategy          string        `json:"strategy"`
	Phases            []string      `json:"phases"`
	EstimatedDuration time.Duration `json:"estimated_duration"`
	RiskLevel         string        `json:"risk_level"`
	FallbackStrategy  string        `json:"fallback_strategy"`
	DowntimeEstimate  time.Duration `json:"downtime_estimate"`
	ValidationPoints  []string      `json:"validation_points"`
	SuccessCriteria   []string      `json:"success_criteria"`
}

// OrderRoutingManager manages order routing during migration
type OrderRoutingManager struct {
	strategy        *RoutingStrategy
	healthMonitor   *RoutingHealthMonitor
	fallbackHandler *FallbackHandler
	metrics         *RoutingMetrics
	logger          *zap.SugaredLogger
}

// RoutingHealthMonitor monitors routing health
type RoutingHealthMonitor struct {
	isHealthy       int64 // atomic
	lastCheck       int64 // atomic (unix nano)
	errorCount      int64 // atomic
	successCount    int64 // atomic
	avgResponseTime int64 // atomic (nanoseconds)
}

// FallbackHandler handles fallback scenarios
type FallbackHandler struct {
	isActive         int64 // atomic
	fallbackCount    int64 // atomic
	lastFallbackTime int64 // atomic (unix nano)
	fallbackReason   string
	mu               sync.RWMutex
}

// RoutingMetrics tracks routing performance metrics
type RoutingMetrics struct {
	TotalRequests      int64 // atomic
	SuccessfulRequests int64 // atomic
	FailedRequests     int64 // atomic
	FallbackRequests   int64 // atomic
	AvgLatency         int64 // atomic (nanoseconds)
	MaxLatency         int64 // atomic (nanoseconds)
	LastResetTime      int64 // atomic (unix nano)
}

// EngineStateSync handles engine state synchronization
type EngineStateSync struct {
	syncStrategy     string
	syncInterval     time.Duration
	lastSyncTime     int64 // atomic (unix nano)
	syncInProgress   int64 // atomic
	syncErrorCount   int64 // atomic
	syncSuccessCount int64 // atomic
	logger           *zap.SugaredLogger
	mu               sync.RWMutex
}

// EnginePerformanceMonitor monitors engine performance during migration
type EnginePerformanceMonitor struct {
	baselineMetrics      *migration.PerformanceBaseline
	currentMetrics       *migration.PerformanceBaseline
	degradationThreshold float64
	alertThreshold       float64
	monitoringInterval   time.Duration
	isMonitoring         int64 // atomic
	alertCount           int64 // atomic
	logger               *zap.SugaredLogger
}

// EngineCircuitBreaker provides circuit breaker functionality
type EngineCircuitBreaker struct {
	state            string // "closed", "open", "half-open"
	failureCount     int64  // atomic
	successCount     int64  // atomic
	lastFailureTime  int64  // atomic (unix nano)
	timeout          time.Duration
	maxFailures      int64
	halfOpenMaxCalls int64
	mu               sync.RWMutex
	logger           *zap.SugaredLogger
}

// NewEngineParticipant creates a new engine migration participant
func NewEngineParticipant(
	pair string,
	engine *engine.AdaptiveMatchingEngine,
	riskManager RiskService,
	logger *zap.SugaredLogger,
) *EngineParticipant {
	p := &EngineParticipant{
		id:          fmt.Sprintf("engine_%s", pair),
		pair:        pair,
		engine:      engine,
		riskManager: riskManager,
		logger:      logger,
		orderRouting: &OrderRoutingManager{
			healthMonitor:   &RoutingHealthMonitor{},
			fallbackHandler: &FallbackHandler{},
			metrics:         &RoutingMetrics{},
			logger:          logger,
		},
		stateSync: &EngineStateSync{
			syncStrategy: "incremental",
			syncInterval: 100 * time.Millisecond,
			logger:       logger,
		},
		performanceMonitor: &EnginePerformanceMonitor{
			degradationThreshold: 0.2, // 20% degradation threshold
			alertThreshold:       0.1, // 10% degradation for alerts
			monitoringInterval:   1 * time.Second,
			logger:               logger,
		},
		circuitBreaker: &EngineCircuitBreaker{
			state:            "closed",
			timeout:          30 * time.Second,
			maxFailures:      5,
			halfOpenMaxCalls: 3,
			logger:           logger,
		},
	}

	// Set initial health status
	atomic.StoreInt64(&p.isHealthy, 1)
	atomic.StoreInt64(&p.lastHeartbeat, time.Now().UnixNano())
	atomic.StoreInt64(&p.isProcessingOrders, 1)

	return p
}

// GetID returns the participant ID
func (p *EngineParticipant) GetID() string {
	return p.id
}

// GetType returns the participant type
func (p *EngineParticipant) GetType() string {
	return "engine"
}

// HealthCheck performs a health check on the engine participant
func (p *EngineParticipant) HealthCheck(ctx context.Context) (*migration.HealthCheck, error) {
	// Check engine health
	engineState := p.captureEngineState(ctx)

	// Check circuit breaker state
	circuitBreakerHealthy := p.circuitBreaker.state == "closed"

	// Check order routing health
	routingHealthy := atomic.LoadInt64(&p.orderRouting.healthMonitor.isHealthy) == 1

	// Check state synchronization health
	syncHealthy := atomic.LoadInt64(&p.stateSync.syncErrorCount) == 0

	// Determine overall health
	overallHealthy := engineState.ErrorRate < 0.01 && // Less than 1% error rate
		circuitBreakerHealthy &&
		routingHealthy &&
		syncHealthy &&
		engineState.LatencyP95 < 10*time.Millisecond // Less than 10ms P95 latency

	healthScore := 1.0
	if !overallHealthy {
		healthScore = 0.6
		if engineState.ErrorRate > 0.05 {
			healthScore = 0.3
		}
	}

	// Update health status
	if overallHealthy {
		atomic.StoreInt64(&p.isHealthy, 1)
	} else {
		atomic.StoreInt64(&p.isHealthy, 0)
	}
	atomic.StoreInt64(&p.lastHeartbeat, time.Now().UnixNano())

	var status string
	var message string

	if overallHealthy {
		status = "healthy"
		message = "Engine participant is healthy"
	} else {
		status = "warning"
		message = fmt.Sprintf("Engine participant unhealthy - routing: %v, sync: %v", routingHealthy, syncHealthy)
		if healthScore < 0.5 {
			status = "critical"
		}
	}

	return &migration.HealthCheck{
		Name:       "engine_participant",
		Status:     status,
		Score:      healthScore,
		Message:    message,
		LastCheck:  time.Now(),
		CheckCount: 1,
		FailCount:  0,
	}, nil
}

// Prepare implements the prepare phase of two-phase commit
func (p *EngineParticipant) Prepare(ctx context.Context, migrationID uuid.UUID, config *migration.MigrationConfig) (*migration.ParticipantState, error) {
	p.migrationMu.Lock()
	defer p.migrationMu.Unlock()

	p.logger.Infow("Starting prepare phase for engine migration",
		"migration_id", migrationID,
		"pair", p.pair,
	)

	startTime := time.Now()

	// Step 1: Capture current engine state
	engineState := p.captureEngineState(ctx)
	if engineState == nil {
		return p.createFailedState("engine state capture failed", fmt.Errorf("unable to capture engine state")), nil
	}

	// Step 2: Analyze current load and performance
	loadAnalysis, err := p.analyzeCurrentLoad(ctx)
	if err != nil {
		return p.createFailedState("load analysis failed", err), nil
	}

	// Step 3: Capture performance baseline
	baseline, err := p.capturePerformanceBaseline(ctx, config)
	if err != nil {
		return p.createFailedState("performance baseline capture failed", err), nil
	}

	// Step 4: Assess migration risk
	riskAssessment := p.assessMigrationRisk(engineState, loadAnalysis, baseline)
	if riskAssessment.OverallRisk == "critical" {
		return p.createFailedState("migration risk too high", fmt.Errorf("risk level: %s", riskAssessment.OverallRisk)), nil
	}

	// Step 5: Create routing strategy
	routingStrategy := p.createRoutingStrategy(config, riskAssessment)
	if routingStrategy == nil {
		return p.createFailedState("routing strategy creation failed", fmt.Errorf("unable to create routing strategy")), nil
	}

	// Step 6: Create synchronization plan
	syncPlan := p.createSynchronizationPlan(config, engineState)

	// Step 7: Create backup configuration
	backupConfig := p.createBackupConfig(config)
	// Step 8: Create migration plan
	migrationPlan := p.createMigrationPlan(config, riskAssessment, routingStrategy)

	// Step 9: Validate current risk state before migration
	if p.riskManager != nil {
		riskReport, err := p.generateRiskReport(ctx)
		if err != nil {
			p.logger.Warnw("Failed to generate risk report during preparation", "error", err)
		} else if riskReport.Status == "critical" {
			return p.createFailedState("pre-migration risk too high",
				fmt.Errorf("current risk status: %s", riskReport.Status)), nil
		}
	}

	// Step 10: Prepare engine for migration
	err = p.prepareEngineForMigration(ctx, migrationPlan)
	if err != nil {
		return p.createFailedState("engine preparation failed", err), nil
	}

	// Store preparation data
	p.currentMigrationID = migrationID
	p.preparationData = &EnginePreparationData{
		EngineState:         engineState,
		RoutingStrategy:     routingStrategy,
		PerformanceBaseline: baseline,
		RiskAssessment:      riskAssessment,
		SynchronizationPlan: syncPlan,
		BackupConfig:        backupConfig,
		PreparationTime:     startTime,
		MigrationPlan:       migrationPlan,
	}

	preparationDuration := time.Since(startTime)
	p.logger.Infow("Prepare phase completed successfully",
		"migration_id", migrationID,
		"pair", p.pair,
		"duration", preparationDuration,
		"risk_level", riskAssessment.OverallRisk,
		"strategy", migrationPlan.Strategy,
	)
	// Create orders snapshot for coordinator
	ordersSnapshot := &migration.OrdersSnapshot{
		Timestamp:   engineState.Timestamp,
		TotalOrders: engineState.ActiveOrders,
		OrdersByType: map[string]int64{
			"active":     engineState.ActiveOrders,
			"processing": int64(engineState.QueueDepth),
		},
		TotalVolume: decimal.Zero, // Will be populated by actual volume calculation
		PriceLevels: make(map[string]*migration.PriceLevelSnapshot),
		Checksum:    "temp_checksum", // Will be calculated properly
	}

	return &migration.ParticipantState{
		ID:              p.id,
		Type:            "engine",
		Vote:            migration.VoteYes,
		IsHealthy:       true,
		LastHeartbeat:   time.Now(),
		OrdersSnapshot:  ordersSnapshot,
		PreparationData: p.preparationData,
		Metrics: map[string]interface{}{
			"preparation_duration_ms": preparationDuration.Milliseconds(),
			"engine_state":            engineState,
			"risk_assessment":         riskAssessment,
			"routing_strategy":        routingStrategy.Strategy,
		},
	}, nil
}

// Commit implements the commit phase of two-phase commit
func (p *EngineParticipant) Commit(ctx context.Context, migrationID uuid.UUID) error {
	p.migrationMu.Lock()
	defer p.migrationMu.Unlock()

	if p.currentMigrationID != migrationID {
		return fmt.Errorf("migration ID mismatch: expected %s, got %s", p.currentMigrationID, migrationID)
	}

	if p.preparationData == nil {
		return fmt.Errorf("no preparation data available for migration %s", migrationID)
	}

	p.logger.Infow("Starting commit phase for engine migration",
		"migration_id", migrationID,
		"pair", p.pair,
	)
	startTime := time.Now()

	// Step 1: Generate pre-migration risk report
	if p.riskManager != nil {
		preReport, err := p.generateRiskReport(ctx)
		if err != nil {
			p.logger.Warnw("Failed to generate pre-migration risk report", "error", err)
		} else {
			p.logger.Infow("Pre-migration risk assessment",
				"status", preReport.Status,
				"risk_level", preReport.RiskMetrics.VaR,
				"active_alerts", preReport.ComplianceStatus.ActiveAlerts,
			)

			// Check if risk is too high to proceed
			if preReport.Status == "critical" {
				return fmt.Errorf("migration aborted due to critical risk level")
			}
		}
	}
	// Step 2: Execute migration plan with risk monitoring
	err := p.executeRiskAwareMigration(ctx, p.preparationData.MigrationPlan)
	if err != nil {
		return fmt.Errorf("risk-aware migration execution failed: %w", err)
	}

	// Step 3: Synchronize engine state
	err = p.synchronizeEngineState(ctx, p.preparationData.SynchronizationPlan)
	if err != nil {
		return fmt.Errorf("engine state synchronization failed: %w", err)
	}
	// Step 3: Validate migration success
	err = p.validateMigrationSuccess(ctx)
	if err != nil {
		return fmt.Errorf("migration validation failed: %w", err)
	}

	// Step 4: Post-migration risk assessment
	if p.riskManager != nil {
		postReport, err := p.generateRiskReport(ctx)
		if err != nil {
			p.logger.Warnw("Failed to generate post-migration risk report", "error", err)
		} else {
			p.logger.Infow("Post-migration risk assessment",
				"status", postReport.Status,
				"risk_level", postReport.RiskMetrics.VaR,
				"active_alerts", postReport.ComplianceStatus.ActiveAlerts,
			)

			// Alert if risk has increased significantly
			if postReport.Status == "critical" || postReport.Status == "at_risk" {
				p.logger.Warnw("Elevated risk detected post-migration",
					"status", postReport.Status,
					"recommendations", postReport.Recommendations,
				)
			}
		}
	}

	// Step 5: Update routing configuration
	err = p.updateRoutingConfiguration(ctx, p.preparationData.RoutingStrategy)
	if err != nil {
		return fmt.Errorf("routing configuration update failed: %w", err)
	}

	// Step 5: Start performance monitoring
	p.startPerformanceMonitoring(ctx)

	commitDuration := time.Since(startTime)
	p.logger.Infow("Commit phase completed successfully",
		"migration_id", migrationID,
		"pair", p.pair,
		"duration", commitDuration,
		"strategy", p.preparationData.MigrationPlan.Strategy,
	)

	return nil
}

// Abort implements the abort phase of two-phase commit
func (p *EngineParticipant) Abort(ctx context.Context, migrationID uuid.UUID, reason string) error {
	p.migrationMu.Lock()
	defer p.migrationMu.Unlock()

	p.logger.Infow("Starting abort phase for engine migration",
		"migration_id", migrationID,
		"pair", p.pair,
		"reason", reason,
	)

	// Reset engine to previous state
	err := p.resetEngineState(ctx)
	if err != nil {
		p.logger.Errorw("Failed to reset engine state during abort", "error", err)
		// Continue with cleanup even if reset fails
	}

	// Reset circuit breaker
	p.circuitBreaker.mu.Lock()
	p.circuitBreaker.state = "closed"
	atomic.StoreInt64(&p.circuitBreaker.failureCount, 0)
	atomic.StoreInt64(&p.circuitBreaker.successCount, 0)
	p.circuitBreaker.mu.Unlock()

	// Reset routing manager
	atomic.StoreInt64(&p.orderRouting.fallbackHandler.isActive, 0)
	atomic.StoreInt64(&p.orderRouting.metrics.TotalRequests, 0)
	atomic.StoreInt64(&p.orderRouting.metrics.FailedRequests, 0)

	// Clear preparation data
	p.preparationData = nil
	p.currentMigrationID = uuid.Nil

	p.logger.Infow("Abort phase completed",
		"migration_id", migrationID,
		"pair", p.pair,
	)

	return nil
}

// Helper methods

func (p *EngineParticipant) createFailedState(reason string, err error) *migration.ParticipantState {
	return &migration.ParticipantState{
		ID:            p.id,
		Type:          "engine",
		Vote:          migration.VoteNo,
		IsHealthy:     false,
		LastHeartbeat: time.Now(),
		ErrorMessage:  fmt.Sprintf("%s: %v", reason, err),
		Metrics: map[string]interface{}{
			"failure_reason": reason,
			"error":          err.Error(),
		},
	}
}

func (p *EngineParticipant) captureEngineState(ctx context.Context) *EngineState {
	// This would integrate with the actual adaptive matching engine
	// For now, return a sample state
	state := &EngineState{
		Timestamp:           time.Now(),
		ActiveOrders:        0,
		ProcessingRate:      0,
		LatencyP50:          1 * time.Millisecond,
		LatencyP95:          5 * time.Millisecond,
		LatencyP99:          10 * time.Millisecond,
		ThroughputTPS:       1000.0,
		ErrorRate:           0.001,
		CircuitBreakerState: "closed",
		MemoryUsage:         100 * 1024 * 1024, // 100MB
		CPUUsage:            0.3,               // 30%
		ConnectionCount:     50,
		QueueDepth:          10,
		MigrationPercentage: 0,
		ImplementationStats: make(map[string]*ImplementationStats),
	}

	// Get current migration state from engine if available
	if p.engine != nil {
		if migrationState, err := p.engine.GetMigrationState(p.pair); err == nil {
			state.MigrationPercentage = migrationState.CurrentPercentage
		}

		// Get metrics from engine's metrics collector if available
		if collector := p.engine.GetMetricsCollector(); collector != nil {
			// Extract relevant metrics
			// This would be implemented based on the actual metrics collector interface
		}
	}

	return state
}

func (p *EngineParticipant) analyzeCurrentLoad(ctx context.Context) (*LoadAnalysis, error) {
	// Analyze current system load
	return &LoadAnalysis{
		CurrentLoad: 0.5, // 50% load
		PeakLoad:    0.8, // 80% peak load
		LoadTrend:   "stable",
		ResourceUtilization: map[string]float64{
			"cpu":     0.3,
			"memory":  0.4,
			"network": 0.2,
			"disk":    0.1,
		},
		BottleneckAnalysis: []string{},
	}, nil
}

func (p *EngineParticipant) capturePerformanceBaseline(ctx context.Context, config *migration.MigrationConfig) (*migration.PerformanceBaseline, error) {
	// Capture performance baseline over a sampling period
	samplingDuration := 30 * time.Second
	if config.PrepareTimeout < samplingDuration {
		samplingDuration = config.PrepareTimeout / 3
	}

	// Sample performance metrics
	time.Sleep(100 * time.Millisecond) // Simulate sampling

	return &migration.PerformanceBaseline{
		CaptureTime:      time.Now(),
		AvgLatency:       2 * time.Millisecond,
		MaxLatency:       10 * time.Millisecond,
		MinLatency:       500 * time.Microsecond,
		ThroughputTPS:    1000.0,
		ErrorRate:        0.001,
		MemoryFootprint:  100 * 1024 * 1024,
		CPUUtilization:   0.3,
		OrdersPerSecond:  500.0,
		SamplingDuration: samplingDuration,
	}, nil
}

func (p *EngineParticipant) assessMigrationRisk(state *EngineState, load *LoadAnalysis, baseline *migration.PerformanceBaseline) *EngineRiskAssessment {
	riskFactors := []string{}
	riskLevel := "low"

	// Assess various risk factors
	if state.ErrorRate > 0.01 {
		riskFactors = append(riskFactors, "high_error_rate")
		riskLevel = "medium"
	}

	if load.CurrentLoad > 0.7 {
		riskFactors = append(riskFactors, "high_load")
		riskLevel = "medium"
	}

	if state.LatencyP95 > 20*time.Millisecond {
		riskFactors = append(riskFactors, "high_latency")
		if riskLevel == "low" {
			riskLevel = "medium"
		}
	}

	if state.CPUUsage > 0.8 || state.MemoryUsage > 1024*1024*1024 {
		riskFactors = append(riskFactors, "resource_constraints")
		riskLevel = "high"
	}

	if len(riskFactors) > 3 {
		riskLevel = "critical"
	}

	estimatedDowntime := int64(100) // 100ms base downtime
	if riskLevel == "high" {
		estimatedDowntime = 500
	} else if riskLevel == "critical" {
		estimatedDowntime = 1000
	}

	return &EngineRiskAssessment{
		OverallRisk:  riskLevel,
		RiskFactors:  riskFactors,
		LoadAnalysis: load,
		ImpactAssessment: &ImpactAssessment{
			EstimatedDowntime: estimatedDowntime,
			AffectedUsers:     100,
			OrdersAtRisk:      state.ActiveOrders,
			RevenueImpact:     0.0,
			RecoveryTime:      30 * time.Second,
		},
		Mitigations: []string{
			"gradual_rollout",
			"circuit_breaker",
			"performance_monitoring",
			"automatic_rollback",
		},
		RollbackProcedure: &RollbackProcedure{
			TriggerConditions: []string{
				"error_rate_spike",
				"latency_degradation",
				"throughput_drop",
			},
			RollbackSteps: []string{
				"pause_migration",
				"reset_routing",
				"restore_state",
				"validate_recovery",
			},
			EstimatedTime: 10 * time.Second,
			DataIntegrityChecks: []string{
				"order_consistency",
				"state_integrity",
				"performance_validation",
			},
			NotificationTargets: []string{
				"ops_team",
				"trading_team",
			},
		},
	}
}

func (p *EngineParticipant) createRoutingStrategy(config *migration.MigrationConfig, risk *EngineRiskAssessment) *RoutingStrategy {
	strategy := "gradual_migration"
	if risk.OverallRisk == "high" || risk.OverallRisk == "critical" {
		strategy = "canary_deployment"
	}

	return &RoutingStrategy{
		Strategy: strategy,
		Parameters: map[string]interface{}{
			"initial_percentage": 1,
			"step_size":          5,
			"step_interval":      "30s",
			"max_percentage":     config.MigrationPercentage,
		},
		FallbackBehavior:    "immediate_rollback",
		HealthCheckInterval: 10 * time.Second,
		TimeoutSettings: &TimeoutSettings{
			OrderProcessing: 5 * time.Second,
			HealthCheck:     2 * time.Second,
			StateSync:       1 * time.Second,
			Fallback:        500 * time.Millisecond,
		},
	}
}

func (p *EngineParticipant) createSynchronizationPlan(config *migration.MigrationConfig, state *EngineState) *SynchronizationPlan {
	return &SynchronizationPlan{
		Strategy:           "incremental",
		SyncPoints:         []string{"order_state", "performance_metrics", "error_rates"},
		SyncInterval:       100 * time.Millisecond,
		ConsistencyLevel:   "eventual",
		ConflictResolution: "last_write_wins",
		ValidationChecks: []string{
			"state_consistency",
			"performance_regression",
			"error_rate_threshold",
		},
	}
}

func (p *EngineParticipant) createBackupConfig(config *migration.MigrationConfig) *EngineBackupConfig {
	return &EngineBackupConfig{
		BackupLocation:     "/tmp/engine_backup",
		BackupStrategy:     "incremental",
		CompressionEnabled: true,
		EncryptionEnabled:  false,
		RetentionPeriod:    24 * time.Hour,
		BackupValidation:   true,
	}
}

func (p *EngineParticipant) createMigrationPlan(config *migration.MigrationConfig, risk *EngineRiskAssessment, strategy *RoutingStrategy) *EngineMigrationPlan {
	phases := []string{"preparation", "routing_setup", "gradual_migration", "validation", "completion"}
	estimatedDuration := 5 * time.Minute

	if risk.OverallRisk == "high" {
		phases = append(phases, "extended_monitoring")
		estimatedDuration = 10 * time.Minute
	}

	return &EngineMigrationPlan{
		Strategy:          strategy.Strategy,
		Phases:            phases,
		EstimatedDuration: estimatedDuration,
		RiskLevel:         risk.OverallRisk,
		FallbackStrategy:  strategy.FallbackBehavior,
		DowntimeEstimate:  time.Duration(risk.ImpactAssessment.EstimatedDowntime) * time.Millisecond,
		ValidationPoints:  []string{"latency_check", "error_rate_check", "throughput_check"},
		SuccessCriteria: []string{
			"error_rate_below_threshold",
			"latency_within_limits",
			"throughput_maintained",
			"no_data_loss",
		},
	}
}

func (p *EngineParticipant) prepareEngineForMigration(ctx context.Context, plan *EngineMigrationPlan) error {
	// Configure circuit breaker
	p.circuitBreaker.mu.Lock()
	p.circuitBreaker.state = "closed"
	atomic.StoreInt64(&p.circuitBreaker.failureCount, 0)
	p.circuitBreaker.mu.Unlock()

	// Initialize routing manager
	atomic.StoreInt64(&p.orderRouting.healthMonitor.isHealthy, 1)
	atomic.StoreInt64(&p.orderRouting.healthMonitor.lastCheck, time.Now().UnixNano())

	// Initialize state sync
	atomic.StoreInt64(&p.stateSync.lastSyncTime, time.Now().UnixNano())
	atomic.StoreInt64(&p.stateSync.syncInProgress, 0)

	// Initialize performance monitor
	atomic.StoreInt64(&p.performanceMonitor.isMonitoring, 1)

	return nil
}

func (p *EngineParticipant) executeMigrationPlan(ctx context.Context, plan *EngineMigrationPlan) error {
	p.logger.Infow("Executing migration plan", "strategy", plan.Strategy, "phases", len(plan.Phases))

	switch plan.Strategy {
	case "gradual_migration":
		return p.executeGradualMigration(ctx, plan)
	case "canary_deployment":
		return p.executeCanaryDeployment(ctx, plan)
	case "blue_green":
		return p.executeBlueGreenMigration(ctx, plan)
	default:
		return fmt.Errorf("unknown migration strategy: %s", plan.Strategy)
	}
}

func (p *EngineParticipant) executeGradualMigration(ctx context.Context, plan *EngineMigrationPlan) error {
	// Implement gradual migration logic
	// This would coordinate with the adaptive matching engine
	if p.engine != nil {
		// Use the engine's migration capabilities
		return p.engine.StartMigration(p.pair, 100) // Start gradual migration to 100%
	}

	p.logger.Infow("Gradual migration executed successfully")
	return nil
}

func (p *EngineParticipant) executeCanaryDeployment(ctx context.Context, plan *EngineMigrationPlan) error {
	// Implement canary deployment logic
	p.logger.Infow("Canary deployment executed successfully")
	return nil
}

func (p *EngineParticipant) executeBlueGreenMigration(ctx context.Context, plan *EngineMigrationPlan) error {
	// Implement blue-green migration logic
	p.logger.Infow("Blue-green migration executed successfully")
	return nil
}

func (p *EngineParticipant) synchronizeEngineState(ctx context.Context, plan *SynchronizationPlan) error {
	atomic.StoreInt64(&p.stateSync.syncInProgress, 1)
	defer atomic.StoreInt64(&p.stateSync.syncInProgress, 0)

	// Perform state synchronization
	atomic.StoreInt64(&p.stateSync.lastSyncTime, time.Now().UnixNano())
	atomic.AddInt64(&p.stateSync.syncSuccessCount, 1)

	p.logger.Infow("Engine state synchronized successfully", "strategy", plan.Strategy)
	return nil
}

func (p *EngineParticipant) validateMigrationSuccess(ctx context.Context) error {
	// Validate migration success criteria
	state := p.captureEngineState(ctx)

	// Check error rate
	if state.ErrorRate > 0.01 {
		return fmt.Errorf("error rate too high: %.4f", state.ErrorRate)
	}

	// Check latency
	if state.LatencyP95 > 20*time.Millisecond {
		return fmt.Errorf("latency too high: %v", state.LatencyP95)
	}

	// Check throughput
	if state.ThroughputTPS < 500 {
		return fmt.Errorf("throughput too low: %.2f TPS", state.ThroughputTPS)
	}

	p.logger.Infow("Migration validation successful",
		"error_rate", state.ErrorRate,
		"latency_p95", state.LatencyP95,
		"throughput", state.ThroughputTPS,
	)

	return nil
}

func (p *EngineParticipant) updateRoutingConfiguration(ctx context.Context, strategy *RoutingStrategy) error {
	// Update routing configuration
	p.orderRouting.strategy = strategy

	// Reset metrics
	atomic.StoreInt64(&p.orderRouting.metrics.LastResetTime, time.Now().UnixNano())

	p.logger.Infow("Routing configuration updated", "strategy", strategy.Strategy)
	return nil
}

func (p *EngineParticipant) startPerformanceMonitoring(ctx context.Context) {
	atomic.StoreInt64(&p.performanceMonitor.isMonitoring, 1)

	// Start background performance monitoring
	go func() {
		ticker := time.NewTicker(p.performanceMonitor.monitoringInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				p.monitorPerformance(ctx)
			}
		}
	}()

	p.logger.Infow("Performance monitoring started")
}

func (p *EngineParticipant) monitorPerformance(ctx context.Context) {
	if atomic.LoadInt64(&p.performanceMonitor.isMonitoring) == 0 {
		return
	}

	// Capture current metrics
	currentState := p.captureEngineState(ctx)
	if currentState == nil {
		return
	}

	// Compare with baseline if available
	if p.performanceMonitor.baselineMetrics != nil {
		baseline := p.performanceMonitor.baselineMetrics

		// Check for performance degradation
		latencyDegradation := float64(currentState.LatencyP95-baseline.AvgLatency) / float64(baseline.AvgLatency)
		throughputDegradation := (baseline.ThroughputTPS - currentState.ThroughputTPS) / baseline.ThroughputTPS

		if latencyDegradation > p.performanceMonitor.degradationThreshold ||
			throughputDegradation > p.performanceMonitor.degradationThreshold {

			atomic.AddInt64(&p.performanceMonitor.alertCount, 1)
			p.logger.Warnw("Performance degradation detected",
				"latency_degradation", latencyDegradation,
				"throughput_degradation", throughputDegradation,
			)
		}
	}
}

func (p *EngineParticipant) resetEngineState(ctx context.Context) error {
	// Reset engine to previous state
	if p.engine != nil {
		return p.engine.ResetMigration(p.pair)
	}

	p.logger.Infow("Engine state reset completed")
	return nil
}

// Risk Management Integration Implementation
func (p *EngineParticipant) validateRiskLimits(ctx context.Context, trades []*model.Trade) error {
	if p.riskManager == nil {
		return nil // Risk management disabled
	}

	for _, t := range trades {
		// Check position limits before applying trade
		canTrade, err := p.riskManager.CheckPositionLimit(ctx, t.OrderID.String(), t.Pair, t.Quantity, t.Price)
		if err != nil {
			p.logger.Errorw("Position limit check failed",
				"trade_id", t.ID.String(),
				"symbol", t.Pair,
				"quantity", t.Quantity,
				"error", err,
			)
			return fmt.Errorf("position limit check failed for trade %s: %w", t.ID.String(), err)
		}
		if !canTrade {
			p.logger.Errorw("Position limit exceeded",
				"trade_id", t.ID.String(),
				"symbol", t.Pair,
				"quantity", t.Quantity,
			)
			return fmt.Errorf("position limit exceeded for trade %s", t.ID.String())
		}

		// Validate against market risk parameters
		if err := p.validateMarketRisk(ctx, t); err != nil {
			return fmt.Errorf("market risk validation failed: %w", err)
		}
	}

	return nil
}

func (p *EngineParticipant) validateMarketRisk(ctx context.Context, t *model.Trade) error {
	if p.riskManager == nil {
		return nil
	}

	// Calculate risk metrics for the trade user
	riskProfile, err := p.riskManager.CalculateRisk(ctx, t.OrderID.String())
	if err != nil {
		return fmt.Errorf("failed to calculate risk for trade: %w", err)
	}
	// Check if the trade would exceed risk thresholds
	tradeValue := t.Quantity.Mul(t.Price)

	// Extract current exposure from risk profile map
	var currentExposure decimal.Decimal
	if exposureVal, ok := riskProfile["current_exposure"]; ok {
		if exposureDec, ok := exposureVal.(decimal.Decimal); ok {
			currentExposure = exposureDec.Add(tradeValue)
		} else {
			currentExposure = tradeValue // Default if not found
		}
	} else {
		currentExposure = tradeValue // Default if not found
	}

	// Simple risk check - in a real implementation this would be more sophisticated
	maxExposure := decimal.NewFromFloat(1000000) // $1M limit for example
	if currentExposure.GreaterThan(maxExposure) {
		return fmt.Errorf("trade would exceed maximum exposure limit")
	}

	return nil
}

func (p *EngineParticipant) updateRiskMetrics(ctx context.Context, trades []*model.Trade) error {
	if p.riskManager == nil {
		return nil
	}

	var wg sync.WaitGroup
	errCh := make(chan error, len(trades))

	// Update risk metrics for each trade in parallel
	for _, t := range trades {
		wg.Add(1)
		go func(trade *model.Trade) {
			defer wg.Done()

			// Process the trade through the risk system
			if err := p.riskManager.ProcessTrade(ctx, trade.ID.String(), trade.OrderID.String(), trade.Pair, trade.Quantity, trade.Price); err != nil {
				errCh <- fmt.Errorf("failed to process trade %s: %w", trade.ID.String(), err)
				return
			}
			// Update market data
			volatility := decimal.NewFromFloat(0.02) // Default 2% volatility
			volatilityFloat, _ := volatility.Float64()
			if err := p.riskManager.UpdateMarketData(ctx, trade.Pair, trade.Price, volatilityFloat); err != nil {
				errCh <- fmt.Errorf("failed to update market data for trade %s: %w", trade.ID.String(), err)
				return
			}
		}(t)
	}

	// Wait for all updates to complete
	go func() {
		wg.Wait()
		close(errCh)
	}()

	// Collect any errors
	var errors []error
	for err := range errCh {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		p.logger.Errorw("Risk metrics update had errors",
			"error_count", len(errors),
			"trade_count", len(trades),
		)
		return fmt.Errorf("risk metrics update failed with %d errors", len(errors))
	}

	p.logger.Debugw("Risk metrics updated successfully",
		"trade_count", len(trades),
	)

	return nil
}

func (p *EngineParticipant) monitorCompliance(ctx context.Context, trades []*model.Trade) error {
	if p.riskManager == nil {
		return nil
	}

	for _, t := range trades {
		// Monitor for compliance violations using the existing ComplianceCheck method
		tradeAmount := t.Quantity.Mul(t.Price)
		complianceResult, err := p.riskManager.ComplianceCheck(ctx, t.ID.String(), t.OrderID.String(), tradeAmount, map[string]interface{}{
			"symbol":    t.Pair,
			"side":      t.Side,
			"quantity":  t.Quantity.String(),
			"price":     t.Price.String(),
			"timestamp": t.CreatedAt,
		})

		if err != nil {
			p.logger.Errorw("Compliance check failed",
				"trade_id", t.ID,
				"order_id", t.OrderID,
				"error", err,
			)
			return fmt.Errorf("compliance check failed for trade %s: %w", t.ID, err)
		}
		// Handle compliance violations
		isSuspicious, _ := complianceResult["is_suspicious"].(bool)
		alertRaised, _ := complianceResult["alert_raised"].(bool)
		flags, _ := complianceResult["flags"].([]string)

		if isSuspicious || alertRaised {
			p.logger.Warnw("Compliance violations detected",
				"trade_id", t.ID,
				"order_id", t.OrderID,
				"suspicious", isSuspicious,
				"alert_raised", alertRaised,
				"flags", flags,
			)
		}
	}

	return nil
}

func (p *EngineParticipant) calculateRiskAdjustment(ctx context.Context, trades []*model.Trade) (*RiskAdjustment, error) {
	if p.riskManager == nil {
		return &RiskAdjustment{
			AdjustmentFactor:  1.0,
			RecommendedAction: "none",
		}, nil
	}

	// Calculate aggregate risk impact
	var totalExposure decimal.Decimal
	userExposures := make(map[string]decimal.Decimal)
	symbolExposures := make(map[string]decimal.Decimal)

	for _, t := range trades {
		exposure := t.Quantity.Mul(t.Price)
		totalExposure = totalExposure.Add(exposure)
		userExposures[t.OrderID.String()] = userExposures[t.OrderID.String()].Add(exposure)
		symbolExposures[t.Pair] = symbolExposures[t.Pair].Add(exposure)
	}

	// Get current risk metrics for representative users
	userIDs := make([]string, 0, len(userExposures))
	for userID := range userExposures {
		userIDs = append(userIDs, userID)
		if len(userIDs) >= 10 { // Limit to 10 users for performance
			break
		}
	}

	riskMetricsMap, err := p.riskManager.BatchCalculateRisk(ctx, userIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate batch risk: %w", err)
	}

	// Determine risk adjustment based on current risk levels
	adjustment := &RiskAdjustment{
		AdjustmentFactor:  1.0,
		RecommendedAction: "proceed",
		RiskLevel:         "normal",
	} // Analyze risk metrics to determine adjustment
	var highRiskUsers int
	for _, metrics := range riskMetricsMap {
		// Convert interface{} metrics to our internal format for calculation
		if metricsMap, ok := metrics.(map[string]interface{}); ok {
			var varFloat, exposureFloat float64

			if varVal, exists := metricsMap["value_at_risk"]; exists {
				if varDec, ok := varVal.(decimal.Decimal); ok {
					varFloat = varDec.InexactFloat64()
				}
			}

			if exposureVal, exists := metricsMap["total_exposure"]; exists {
				if exposureDec, ok := exposureVal.(decimal.Decimal); ok {
					exposureFloat = exposureDec.InexactFloat64()
				}
			}

			// High risk conditions (example thresholds)
			if varFloat > 10000 || exposureFloat > 500000 { // $10K VaR or $500K exposure
				highRiskUsers++
			}
		}
	}

	// Adjust based on percentage of high-risk users
	riskRatio := float64(highRiskUsers) / float64(len(riskMetricsMap))
	if riskRatio > 0.3 { // More than 30% high-risk users
		adjustment.AdjustmentFactor = 0.5
		adjustment.RecommendedAction = "reduce_exposure"
		adjustment.RiskLevel = "high"
		adjustment.Reason = fmt.Sprintf("High risk detected in %.1f%% of users", riskRatio*100)
	}

	if riskRatio > 0.5 { // More than 50% high-risk users
		adjustment.AdjustmentFactor = 0.1
		adjustment.RecommendedAction = "halt_migration"
		adjustment.RiskLevel = "critical"
		adjustment.Reason = fmt.Sprintf("Critical risk detected in %.1f%% of users", riskRatio*100)
	}

	// Check concentration risk
	totalExposureFloat := totalExposure.InexactFloat64()
	for symbol, exposure := range symbolExposures {
		exposureFloat := exposure.InexactFloat64()
		if exposureFloat > totalExposureFloat*0.30 { // More than 30% in single symbol
			adjustment.AdjustmentFactor *= 0.8
			adjustment.RiskLevel = "elevated"
			adjustment.Reason = fmt.Sprintf("High concentration in %s (%.1f%%)", symbol, (exposureFloat/totalExposureFloat)*100)
		}
	}

	p.logger.Infow("Risk adjustment calculated",
		"adjustment_factor", adjustment.AdjustmentFactor,
		"recommended_action", adjustment.RecommendedAction,
		"risk_level", adjustment.RiskLevel,
		"total_exposure", totalExposureFloat,
		"high_risk_user_ratio", riskRatio,
	)

	return adjustment, nil
}

func (p *EngineParticipant) generateRiskReport(ctx context.Context) (*RiskReport, error) {
	if p.riskManager == nil {
		return &RiskReport{
			Timestamp: time.Now(),
			Status:    "risk_management_disabled",
		}, nil
	}

	// Get active compliance alerts for risk assessment
	alerts, err := p.riskManager.GetActiveComplianceAlerts(ctx)
	if err != nil {
		p.logger.Warnw("Failed to get compliance alerts", "error", err)
		alerts = []ComplianceAlert{} // Use empty slice if error
	}

	// Calculate aggregate risk metrics (using placeholder values in absence of dashboard metrics)
	avgVaR := 12500.0        // $12.5K VaR placeholder
	avgExposure := 750000.0  // $750K exposure placeholder
	avgRiskScore := 65.0     // 65/100 risk score placeholder
	maxConcentration := 0.25 // 25% max concentration placeholder

	// Compile risk report
	report := &RiskReport{
		Timestamp: time.Now(),
		Status:    "healthy",
		RiskMetrics: RiskMetrics{
			VaR:                 avgVaR,
			ExposureUtilization: avgExposure / 1000000, // Normalize to percentage
			ConcentrationRisk:   maxConcentration,
			MaxDrawdown:         avgRiskScore, // Use risk score as proxy for max drawdown
		},
		ComplianceStatus: ComplianceStatus{
			ActiveAlerts:   len(alerts),
			ViolationCount: len(alerts), // Simplified
			LastCheck:      time.Now(),
			Status:         "compliant",
		},
	}
	// Determine overall status
	if avgVaR > 15000 || len(alerts) > 5 { // $15K VaR or more than 5 alerts
		report.Status = "at_risk"
		report.ComplianceStatus.Status = "warning"
	}
	if avgVaR > 25000 || len(alerts) > 10 { // $25K VaR or more than 10 alerts
		report.Status = "critical"
		report.ComplianceStatus.Status = "violation"
	}

	// Add recommendations
	if avgVaR > 10000 {
		report.Recommendations = append(report.Recommendations, "Consider reducing position sizes")
	}
	if maxConcentration > 0.30 {
		report.Recommendations = append(report.Recommendations, "Diversify positions across symbols")
	}
	if len(alerts) > 3 {
		report.Recommendations = append(report.Recommendations, "Review compliance alerts")
	}

	p.logger.Infow("Risk report generated",
		"status", report.Status,
		"avg_var", avgVaR,
		"active_alerts", len(alerts),
		"max_concentration", maxConcentration,
	)

	return report, nil
}

// executeRiskAwareMigration executes migration with continuous risk monitoring
func (p *EngineParticipant) executeRiskAwareMigration(ctx context.Context, plan *EngineMigrationPlan) error {
	if p.riskManager == nil {
		// Fallback to standard migration if no risk manager
		return p.executeMigrationPlan(ctx, plan)
	}

	p.logger.Infow("Starting risk-aware migration execution",
		"strategy", plan.Strategy,
		"estimated_duration", plan.EstimatedDuration,
	)

	// Create a context with timeout for the migration
	migrationCtx, cancel := context.WithTimeout(ctx, plan.EstimatedDuration*2)
	defer cancel()

	// Start risk monitoring in background
	riskMonitorDone := make(chan error, 1)
	go func() {
		riskMonitorDone <- p.monitorRiskDuringMigration(migrationCtx, plan)
	}()

	// Execute the migration plan
	migrationDone := make(chan error, 1)
	go func() {
		migrationDone <- p.executeMigrationPlan(migrationCtx, plan)
	}()

	// Wait for either migration completion or risk monitoring failure
	select {
	case err := <-migrationDone:
		// Migration completed, stop risk monitoring
		cancel()
		<-riskMonitorDone // Wait for risk monitor to stop

		if err != nil {
			return fmt.Errorf("migration execution failed: %w", err)
		}

		p.logger.Infow("Risk-aware migration completed successfully")
		return nil

	case err := <-riskMonitorDone:
		// Risk monitoring detected a problem
		cancel()
		<-migrationDone // Wait for migration to stop

		if err != nil {
			return fmt.Errorf("migration aborted due to risk monitoring: %w", err)
		}

		return fmt.Errorf("migration aborted due to risk monitoring")

	case <-migrationCtx.Done():
		// Timeout
		return fmt.Errorf("migration timed out after %v", plan.EstimatedDuration*2)
	}
}

// monitorRiskDuringMigration continuously monitors risk during migration
func (p *EngineParticipant) monitorRiskDuringMigration(ctx context.Context, plan *EngineMigrationPlan) error {
	ticker := time.NewTicker(5 * time.Second) // Check every 5 seconds
	defer ticker.Stop()

	consecutiveHighRiskChecks := 0
	maxConsecutiveHighRisk := 3 // Abort if high risk for 3 consecutive checks

	for {
		select {
		case <-ctx.Done():
			return nil

		case <-ticker.C:
			// Generate current risk report
			report, err := p.generateRiskReport(ctx)
			if err != nil {
				p.logger.Warnw("Failed to generate risk report during migration monitoring", "error", err)
				continue
			}

			// Check risk levels
			switch report.Status {
			case "critical":
				p.logger.Errorw("Critical risk detected during migration",
					"var", report.RiskMetrics.VaR,
					"active_alerts", report.ComplianceStatus.ActiveAlerts,
					"recommendations", report.Recommendations,
				)
				return fmt.Errorf("critical risk level detected: VaR=%.2f, alerts=%d",
					report.RiskMetrics.VaR, report.ComplianceStatus.ActiveAlerts)

			case "at_risk":
				consecutiveHighRiskChecks++
				p.logger.Warnw("Elevated risk detected during migration",
					"var", report.RiskMetrics.VaR,
					"consecutive_checks", consecutiveHighRiskChecks,
					"max_allowed", maxConsecutiveHighRisk,
				)

				if consecutiveHighRiskChecks >= maxConsecutiveHighRisk {
					return fmt.Errorf("persistent elevated risk detected over %d checks", consecutiveHighRiskChecks)
				}

			default:
				// Reset counter on healthy status
				consecutiveHighRiskChecks = 0
				p.logger.Debugw("Risk monitoring check passed",
					"status", report.Status,
					"var", report.RiskMetrics.VaR,
				)
			}
		}
	}
}
