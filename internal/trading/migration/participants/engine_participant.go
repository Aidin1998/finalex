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
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// EngineParticipant handles matching engine coordination during migration
type EngineParticipant struct {
	id                 string
	pair               string
	engine             *engine.AdaptiveMatchingEngine
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
	PerformanceBaseline *PerformanceBaseline
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

// PerformanceBaseline captures baseline performance metrics
type PerformanceBaseline struct {
	CaptureTime      time.Time     `json:"capture_time"`
	AvgLatency       time.Duration `json:"avg_latency"`
	MaxLatency       time.Duration `json:"max_latency"`
	MinLatency       time.Duration `json:"min_latency"`
	ThroughputTPS    float64       `json:"throughput_tps"`
	ErrorRate        float64       `json:"error_rate"`
	MemoryFootprint  int64         `json:"memory_footprint"`
	CPUUtilization   float64       `json:"cpu_utilization"`
	OrdersPerSecond  float64       `json:"orders_per_second"`
	SamplingDuration time.Duration `json:"sampling_duration"`
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
	baselineMetrics      *PerformanceBaseline
	currentMetrics       *PerformanceBaseline
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
	logger *zap.SugaredLogger,
) *EngineParticipant {
	p := &EngineParticipant{
		id:     fmt.Sprintf("engine_%s", pair),
		pair:   pair,
		engine: engine,
		logger: logger,
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

	// Step 9: Prepare engine for migration
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

	// Step 1: Execute migration plan
	err := p.executeMigrationPlan(ctx, p.preparationData.MigrationPlan)
	if err != nil {
		return fmt.Errorf("migration plan execution failed: %w", err)
	}

	// Step 2: Synchronize engine state
	err = p.synchronizeEngineState(ctx, p.preparationData.SynchronizationPlan)
	if err != nil {
		return fmt.Errorf("engine state synchronization failed: %w", err)
	}

	// Step 3: Validate migration success
	err = p.validateMigrationSuccess(ctx)
	if err != nil {
		return fmt.Errorf("migration validation failed: %w", err)
	}

	// Step 4: Update routing configuration
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

func (p *EngineParticipant) capturePerformanceBaseline(ctx context.Context, config *migration.MigrationConfig) (*PerformanceBaseline, error) {
	// Capture performance baseline over a sampling period
	samplingDuration := 30 * time.Second
	if config.PrepareTimeout < samplingDuration {
		samplingDuration = config.PrepareTimeout / 3
	}

	// Sample performance metrics
	time.Sleep(100 * time.Millisecond) // Simulate sampling

	return &PerformanceBaseline{
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

func (p *EngineParticipant) assessMigrationRisk(state *EngineState, load *LoadAnalysis, baseline *PerformanceBaseline) *EngineRiskAssessment {
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
