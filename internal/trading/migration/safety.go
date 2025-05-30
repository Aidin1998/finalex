// =============================
// Migration Safety Mechanisms
// =============================
// This file implements comprehensive safety mechanisms for the two-phase commit
// migration system including automatic rollback, integrity validation, performance
// comparison, circuit breakers, and health monitoring.

package migration

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// SafetyManager coordinates all safety mechanisms during migration
type SafetyManager struct {
	logger             *zap.SugaredLogger
	config             *SafetyConfig
	rollbackManager    *RollbackManager
	integrityValidator *IntegrityValidator
	performanceMonitor *PerformanceMonitor
	circuitBreaker     *CircuitBreaker
	healthMonitor      *HealthMonitor
	alertManager       *AlertManager

	// State tracking
	activeMigrations map[uuid.UUID]*MigrationSafetyState
	safetyMu         sync.RWMutex

	// Metrics
	totalViolations int64 // atomic
	totalRollbacks  int64 // atomic
	totalAlerts     int64 // atomic
}

// SafetyConfig contains configuration for safety mechanisms
type SafetyConfig struct {
	// Rollback settings
	AutoRollbackEnabled bool          `json:"auto_rollback_enabled"`
	RollbackTimeout     time.Duration `json:"rollback_timeout"`
	MaxRollbackAttempts int           `json:"max_rollback_attempts"`

	// Performance thresholds
	MaxLatencyIncrease    float64 `json:"max_latency_increase"`
	MaxThroughputDecrease float64 `json:"max_throughput_decrease"`
	MaxErrorRateIncrease  float64 `json:"max_error_rate_increase"`

	// Health check settings
	HealthCheckInterval time.Duration `json:"health_check_interval"`
	HealthCheckTimeout  time.Duration `json:"health_check_timeout"`
	MinHealthScore      float64       `json:"min_health_score"`

	// Circuit breaker settings
	CircuitBreakerEnabled bool          `json:"circuit_breaker_enabled"`
	FailureThreshold      int           `json:"failure_threshold"`
	RecoveryTimeout       time.Duration `json:"recovery_timeout"`

	// Integrity validation
	IntegrityCheckEnabled  bool          `json:"integrity_check_enabled"`
	IntegrityCheckInterval time.Duration `json:"integrity_check_interval"`
	OrderConsistencyCheck  bool          `json:"order_consistency_check"`
	DataIntegrityCheck     bool          `json:"data_integrity_check"`

	// Alert settings
	AlertingEnabled        bool     `json:"alerting_enabled"`
	AlertChannels          []string `json:"alert_channels"`
	CriticalAlertThreshold int      `json:"critical_alert_threshold"`
}

// MigrationSafetyState tracks safety state for a migration
type MigrationSafetyState struct {
	MigrationID         uuid.UUID
	StartTime           time.Time
	LastCheck           time.Time
	ViolationsCount     int64
	RollbackAttempts    int
	CurrentPhase        MigrationPhase
	SafetyStatus        SafetyStatus
	Violations          []SafetyViolation
	PerformanceBaseline *PerformanceBaseline
	CurrentMetrics      *PerformanceMetrics
	mu                  sync.RWMutex
}

// SafetyStatus represents the overall safety status
type SafetyStatus string

const (
	SafetyStatusHealthy   SafetyStatus = "healthy"
	SafetyStatusWarning   SafetyStatus = "warning"
	SafetyStatusCritical  SafetyStatus = "critical"
	SafetyStatusViolation SafetyStatus = "violation"
)

// SafetyViolation represents a safety rule violation
type SafetyViolation struct {
	ID          uuid.UUID              `json:"id"`
	Timestamp   time.Time              `json:"timestamp"`
	Type        ViolationType          `json:"type"`
	Severity    ViolationSeverity      `json:"severity"`
	Description string                 `json:"description"`
	Metrics     map[string]interface{} `json:"metrics"`
	Actions     []string               `json:"actions"`
	Resolved    bool                   `json:"resolved"`
	ResolvedAt  *time.Time             `json:"resolved_at,omitempty"`
}

// ViolationType defines types of safety violations
type ViolationType string

const (
	ViolationTypePerformance    ViolationType = "performance"
	ViolationTypeIntegrity      ViolationType = "integrity"
	ViolationTypeHealth         ViolationType = "health"
	ViolationTypeCircuitBreaker ViolationType = "circuit_breaker"
	ViolationTypeTimeout        ViolationType = "timeout"
)

// ViolationSeverity defines violation severity levels
type ViolationSeverity string

const (
	SeverityLow      ViolationSeverity = "low"
	SeverityMedium   ViolationSeverity = "medium"
	SeverityHigh     ViolationSeverity = "high"
	SeverityCritical ViolationSeverity = "critical"
)

// RollbackManager handles automatic rollback procedures
type RollbackManager struct {
	logger          *zap.SugaredLogger
	config          *SafetyConfig
	coordinator     MigrationCoordinator
	rollbackHistory map[uuid.UUID][]RollbackAttempt
	mu              sync.RWMutex
}

// RollbackAttempt tracks rollback attempts
type RollbackAttempt struct {
	ID         uuid.UUID         `json:"id"`
	Timestamp  time.Time         `json:"timestamp"`
	Reason     string            `json:"reason"`
	Trigger    RollbackTrigger   `json:"trigger"`
	Success    bool              `json:"success"`
	Duration   time.Duration     `json:"duration"`
	ErrorMsg   string            `json:"error_msg,omitempty"`
	Violations []SafetyViolation `json:"violations"`
}

// RollbackTrigger defines what triggered the rollback
type RollbackTrigger string

const (
	TriggerPerformanceDegradation RollbackTrigger = "performance_degradation"
	TriggerIntegrityViolation     RollbackTrigger = "integrity_violation"
	TriggerHealthFailure          RollbackTrigger = "health_failure"
	TriggerCircuitBreaker         RollbackTrigger = "circuit_breaker"
	TriggerManual                 RollbackTrigger = "manual"
	TriggerTimeout                RollbackTrigger = "timeout"
)

// IntegrityValidator validates order and data integrity
type IntegrityValidator struct {
	logger            *zap.SugaredLogger
	config            *SafetyConfig
	orderRepo         interface{} // Order repository interface
	tradeRepo         interface{} // Trade repository interface
	checksumValidator *ChecksumValidator
	orderValidator    *OrderValidator
	dataValidator     *DataValidator
}

// ChecksumValidator validates data checksums
type ChecksumValidator struct {
	checksums      map[string]string // Table -> Checksum
	lastValidation time.Time
	mu             sync.RWMutex
}

// OrderValidator validates order consistency
type OrderValidator struct {
	orderCounts    map[string]int64 // Status -> Count
	totalVolume    float64
	lastValidation time.Time
	mu             sync.RWMutex
}

// DataValidator validates general data integrity
type DataValidator struct {
	validationRules []ValidationRule
	lastValidation  time.Time
	violationCount  int64 // atomic
	mu              sync.RWMutex
}

// ValidationRule defines a data validation rule
type ValidationRule struct {
	Name        string                          `json:"name"`
	Description string                          `json:"description"`
	Validator   func(ctx context.Context) error `json:"-"`
	Severity    ViolationSeverity               `json:"severity"`
	Enabled     bool                            `json:"enabled"`
}

// PerformanceMonitor monitors performance during migration
type PerformanceMonitor struct {
	logger           *zap.SugaredLogger
	config           *SafetyConfig
	baselineCapture  *BaselineCapture
	metricsCollector *MetricsCollector
	comparisonEngine *ComparisonEngine
	alertThresholds  *AlertThresholds
}

// BaselineCapture captures performance baseline
type BaselineCapture struct {
	latencyBaseline    *LatencyBaseline
	throughputBaseline *ThroughputBaseline
	errorRateBaseline  *ErrorRateBaseline
	resourceBaseline   *ResourceBaseline
	captureTime        time.Time
	samplingDuration   time.Duration
	mu                 sync.RWMutex
}

// LatencyBaseline contains latency baseline metrics
type LatencyBaseline struct {
	P50         time.Duration `json:"p50"`
	P95         time.Duration `json:"p95"`
	P99         time.Duration `json:"p99"`
	Average     time.Duration `json:"average"`
	Max         time.Duration `json:"max"`
	Min         time.Duration `json:"min"`
	SampleCount int64         `json:"sample_count"`
}

// ThroughputBaseline contains throughput baseline metrics
type ThroughputBaseline struct {
	OrdersPerSecond   float64 `json:"orders_per_second"`
	TradesPerSecond   float64 `json:"trades_per_second"`
	VolumePerSecond   float64 `json:"volume_per_second"`
	PeakThroughput    float64 `json:"peak_throughput"`
	AverageThroughput float64 `json:"average_throughput"`
	SampleCount       int64   `json:"sample_count"`
}

// ErrorRateBaseline contains error rate baseline metrics
type ErrorRateBaseline struct {
	OverallErrorRate float64            `json:"overall_error_rate"`
	ErrorRateByType  map[string]float64 `json:"error_rate_by_type"`
	CriticalErrors   int64              `json:"critical_errors"`
	WarningErrors    int64              `json:"warning_errors"`
	TotalRequests    int64              `json:"total_requests"`
	TotalErrors      int64              `json:"total_errors"`
}

// ResourceBaseline contains resource usage baseline
type ResourceBaseline struct {
	CPUUsage        float64 `json:"cpu_usage"`
	MemoryUsage     int64   `json:"memory_usage"`
	NetworkUsage    int64   `json:"network_usage"`
	DiskUsage       int64   `json:"disk_usage"`
	ConnectionCount int     `json:"connection_count"`
}

// MetricsCollector collects real-time performance metrics
type MetricsCollector struct {
	currentMetrics     *PerformanceMetrics
	metricsHistory     []PerformanceMetrics
	collectionInterval time.Duration
	maxHistorySize     int
	mu                 sync.RWMutex
	isCollecting       int64 // atomic
}

// PerformanceMetrics contains current performance metrics
type PerformanceMetrics struct {
	Timestamp         time.Time          `json:"timestamp"`
	Latency           *LatencyMetrics    `json:"latency"`
	Throughput        *ThroughputMetrics `json:"throughput"`
	ErrorRate         *ErrorRateMetrics  `json:"error_rate"`
	ResourceUsage     *ResourceMetrics   `json:"resource_usage"`
	ComparisonResults *ComparisonResults `json:"comparison_results"`
}

// LatencyMetrics contains real-time latency metrics
type LatencyMetrics struct {
	P50     time.Duration `json:"p50"`
	P95     time.Duration `json:"p95"`
	P99     time.Duration `json:"p99"`
	Average time.Duration `json:"average"`
	Current time.Duration `json:"current"`
}

// ThroughputMetrics contains real-time throughput metrics
type ThroughputMetrics struct {
	Current         float64 `json:"current"`
	Average         float64 `json:"average"`
	Peak            float64 `json:"peak"`
	OrdersPerSecond float64 `json:"orders_per_second"`
	TradesPerSecond float64 `json:"trades_per_second"`
}

// ErrorRateMetrics contains real-time error rate metrics
type ErrorRateMetrics struct {
	Current         float64 `json:"current"`
	Average         float64 `json:"average"`
	TotalErrors     int64   `json:"total_errors"`
	TotalRequests   int64   `json:"total_requests"`
	ErrorsPerSecond float64 `json:"errors_per_second"`
}

// ResourceMetrics contains real-time resource metrics
type ResourceMetrics struct {
	CPUUsage        float64 `json:"cpu_usage"`
	MemoryUsage     int64   `json:"memory_usage"`
	NetworkUsage    int64   `json:"network_usage"`
	DiskUsage       int64   `json:"disk_usage"`
	ConnectionCount int     `json:"connection_count"`
}

// ComparisonResults contains performance comparison results
type ComparisonResults struct {
	LatencyComparison    *MetricComparison `json:"latency_comparison"`
	ThroughputComparison *MetricComparison `json:"throughput_comparison"`
	ErrorRateComparison  *MetricComparison `json:"error_rate_comparison"`
	ResourceComparison   *MetricComparison `json:"resource_comparison"`
	OverallScore         float64           `json:"overall_score"`
	Recommendation       string            `json:"recommendation"`
}

// MetricComparison compares a metric against baseline
type MetricComparison struct {
	BaselineValue    float64 `json:"baseline_value"`
	CurrentValue     float64 `json:"current_value"`
	ChangePercentage float64 `json:"change_percentage"`
	Trend            string  `json:"trend"`
	Status           string  `json:"status"`
	WithinThreshold  bool    `json:"within_threshold"`
}

// ComparisonEngine compares current metrics with baseline
type ComparisonEngine struct {
	logger          *zap.SugaredLogger
	thresholds      *AlertThresholds
	comparisonRules []ComparisonRule
}

// ComparisonRule defines how to compare metrics
type ComparisonRule struct {
	MetricName     string            `json:"metric_name"`
	ComparisonType string            `json:"comparison_type"`
	Threshold      float64           `json:"threshold"`
	Severity       ViolationSeverity `json:"severity"`
	Enabled        bool              `json:"enabled"`
}

// AlertThresholds defines performance alert thresholds
type AlertThresholds struct {
	LatencyIncrease    float64 `json:"latency_increase"`
	ThroughputDecrease float64 `json:"throughput_decrease"`
	ErrorRateIncrease  float64 `json:"error_rate_increase"`
	CPUIncrease        float64 `json:"cpu_increase"`
	MemoryIncrease     float64 `json:"memory_increase"`
}

// CircuitBreaker provides circuit breaker functionality for migrations
type CircuitBreaker struct {
	logger          *zap.SugaredLogger
	config          *SafetyConfig
	state           string // "closed", "open", "half-open"
	failureCount    int64  // atomic
	successCount    int64  // atomic
	lastFailureTime int64  // atomic (unix nano)
	lastSuccessTime int64  // atomic (unix nano)
	stateChanges    []StateChange
	mu              sync.RWMutex
}

// StateChange tracks circuit breaker state changes
type StateChange struct {
	Timestamp    time.Time `json:"timestamp"`
	FromState    string    `json:"from_state"`
	ToState      string    `json:"to_state"`
	Reason       string    `json:"reason"`
	FailureCount int64     `json:"failure_count"`
}

// HealthMonitor monitors overall migration health
type HealthMonitor struct {
	logger        *zap.SugaredLogger
	config        *SafetyConfig
	participants  map[string]MigrationParticipant
	healthStatus  map[string]*ParticipantHealth
	overallHealth *OverallHealth
	healthHistory []HealthSnapshot
	mu            sync.RWMutex
}

// ParticipantHealth tracks individual participant health
type ParticipantHealth struct {
	ParticipantID       string                 `json:"participant_id"`
	IsHealthy           bool                   `json:"is_healthy"`
	HealthScore         float64                `json:"health_score"`
	LastCheck           time.Time              `json:"last_check"`
	ConsecutiveFailures int                    `json:"consecutive_failures"`
	ErrorMessage        string                 `json:"error_message,omitempty"`
	Metrics             map[string]interface{} `json:"metrics"`
}

// OverallHealth represents overall migration health
type OverallHealth struct {
	IsHealthy           bool      `json:"is_healthy"`
	OverallScore        float64   `json:"overall_score"`
	HealthyParticipants int       `json:"healthy_participants"`
	TotalParticipants   int       `json:"total_participants"`
	LastCheck           time.Time `json:"last_check"`
	CriticalIssues      []string  `json:"critical_issues"`
}

// HealthSnapshot captures health at a point in time
type HealthSnapshot struct {
	Timestamp         time.Time                     `json:"timestamp"`
	OverallHealth     *OverallHealth                `json:"overall_health"`
	ParticipantHealth map[string]*ParticipantHealth `json:"participant_health"`
	SafetyViolations  []SafetyViolation             `json:"safety_violations"`
}

// AlertManager manages safety alerts and notifications
type AlertManager struct {
	logger          *zap.SugaredLogger
	config          *SafetyConfig
	alertChannels   map[string]AlertChannel
	alertHistory    []Alert
	activeAlerts    map[uuid.UUID]*Alert
	escalationRules []EscalationRule
	mu              sync.RWMutex
}

// AlertChannel defines an alert delivery channel
type AlertChannel interface {
	SendAlert(ctx context.Context, alert *Alert) error
	GetChannelType() string
	IsEnabled() bool
}

// Alert represents a safety alert
type Alert struct {
	ID             uuid.UUID              `json:"id"`
	Timestamp      time.Time              `json:"timestamp"`
	Severity       ViolationSeverity      `json:"severity"`
	Type           ViolationType          `json:"type"`
	Title          string                 `json:"title"`
	Description    string                 `json:"description"`
	MigrationID    uuid.UUID              `json:"migration_id"`
	ParticipantID  string                 `json:"participant_id,omitempty"`
	Metrics        map[string]interface{} `json:"metrics"`
	Actions        []string               `json:"actions"`
	Acknowledged   bool                   `json:"acknowledged"`
	AcknowledgedBy string                 `json:"acknowledged_by,omitempty"`
	AcknowledgedAt *time.Time             `json:"acknowledged_at,omitempty"`
	Resolved       bool                   `json:"resolved"`
	ResolvedAt     *time.Time             `json:"resolved_at,omitempty"`
}

// EscalationRule defines alert escalation rules
type EscalationRule struct {
	Severity        ViolationSeverity `json:"severity"`
	EscalationDelay time.Duration     `json:"escalation_delay"`
	EscalationLevel int               `json:"escalation_level"`
	Channels        []string          `json:"channels"`
	Enabled         bool              `json:"enabled"`
}

// NewSafetyManager creates a new migration safety manager
func NewSafetyManager(
	logger *zap.SugaredLogger,
	config *SafetyConfig,
	coordinator MigrationCoordinator,
) *SafetyManager {
	if config == nil {
		config = DefaultSafetyConfig()
	}

	sm := &SafetyManager{
		logger:           logger,
		config:           config,
		activeMigrations: make(map[uuid.UUID]*MigrationSafetyState),
		rollbackManager: &RollbackManager{
			logger:          logger,
			config:          config,
			coordinator:     coordinator,
			rollbackHistory: make(map[uuid.UUID][]RollbackAttempt),
		},
		integrityValidator: &IntegrityValidator{
			logger: logger,
			config: config,
			checksumValidator: &ChecksumValidator{
				checksums: make(map[string]string),
			},
			orderValidator: &OrderValidator{
				orderCounts: make(map[string]int64),
			},
			dataValidator: &DataValidator{
				validationRules: createDefaultValidationRules(),
			},
		},
		performanceMonitor: &PerformanceMonitor{
			logger: logger,
			config: config,
			baselineCapture: &BaselineCapture{
				latencyBaseline:    &LatencyBaseline{},
				throughputBaseline: &ThroughputBaseline{},
				errorRateBaseline:  &ErrorRateBaseline{},
				resourceBaseline:   &ResourceBaseline{},
			},
			metricsCollector: &MetricsCollector{
				currentMetrics:     &PerformanceMetrics{},
				metricsHistory:     make([]PerformanceMetrics, 0),
				collectionInterval: 1 * time.Second,
				maxHistorySize:     1000,
			},
			comparisonEngine: &ComparisonEngine{
				logger:          logger,
				comparisonRules: createDefaultComparisonRules(),
			},
			alertThresholds: &AlertThresholds{
				LatencyIncrease:    config.MaxLatencyIncrease,
				ThroughputDecrease: config.MaxThroughputDecrease,
				ErrorRateIncrease:  config.MaxErrorRateIncrease,
				CPUIncrease:        0.5,
				MemoryIncrease:     0.3,
			},
		},
		circuitBreaker: &CircuitBreaker{
			logger:       logger,
			config:       config,
			state:        "closed",
			stateChanges: make([]StateChange, 0),
		},
		healthMonitor: &HealthMonitor{
			logger:        logger,
			config:        config,
			participants:  make(map[string]MigrationParticipant),
			healthStatus:  make(map[string]*ParticipantHealth),
			overallHealth: &OverallHealth{},
			healthHistory: make([]HealthSnapshot, 0),
		},
		alertManager: &AlertManager{
			logger:          logger,
			config:          config,
			alertChannels:   make(map[string]AlertChannel),
			alertHistory:    make([]Alert, 0),
			activeAlerts:    make(map[uuid.UUID]*Alert),
			escalationRules: createDefaultEscalationRules(),
		},
	}

	// Start background monitoring
	sm.startBackgroundMonitoring()

	return sm
}

// DefaultSafetyConfig returns default safety configuration
func DefaultSafetyConfig() *SafetyConfig {
	return &SafetyConfig{
		AutoRollbackEnabled:    true,
		RollbackTimeout:        30 * time.Second,
		MaxRollbackAttempts:    3,
		MaxLatencyIncrease:     0.5, // 50% increase
		MaxThroughputDecrease:  0.3, // 30% decrease
		MaxErrorRateIncrease:   0.1, // 10% increase
		HealthCheckInterval:    10 * time.Second,
		HealthCheckTimeout:     5 * time.Second,
		MinHealthScore:         0.8, // 80% health score
		CircuitBreakerEnabled:  true,
		FailureThreshold:       5,
		RecoveryTimeout:        60 * time.Second,
		IntegrityCheckEnabled:  true,
		IntegrityCheckInterval: 30 * time.Second,
		OrderConsistencyCheck:  true,
		DataIntegrityCheck:     true,
		AlertingEnabled:        true,
		AlertChannels:          []string{"log", "metrics"},
		CriticalAlertThreshold: 3,
	}
}

// StartMigrationSafety initializes safety monitoring for a migration
func (sm *SafetyManager) StartMigrationSafety(ctx context.Context, migrationID uuid.UUID, participants []MigrationParticipant) error {
	sm.safetyMu.Lock()
	defer sm.safetyMu.Unlock()

	// Create safety state
	safetyState := &MigrationSafetyState{
		MigrationID:      migrationID,
		StartTime:        time.Now(),
		LastCheck:        time.Now(),
		ViolationsCount:  0,
		RollbackAttempts: 0,
		CurrentPhase:     PhaseIdle,
		SafetyStatus:     SafetyStatusHealthy,
		Violations:       make([]SafetyViolation, 0),
	}

	sm.activeMigrations[migrationID] = safetyState

	// Register participants with health monitor
	for _, participant := range participants {
		sm.healthMonitor.participants[participant.GetID()] = participant
		sm.healthMonitor.healthStatus[participant.GetID()] = &ParticipantHealth{
			ParticipantID:       participant.GetID(),
			IsHealthy:           true,
			HealthScore:         1.0,
			LastCheck:           time.Now(),
			ConsecutiveFailures: 0,
			Metrics:             make(map[string]interface{}),
		}
	}

	// Capture performance baseline
	baseline, err := sm.capturePerformanceBaseline(ctx)
	if err != nil {
		sm.logger.Warnw("Failed to capture performance baseline", "error", err)
	} else {
		safetyState.PerformanceBaseline = baseline
	}

	// Start metrics collection
	atomic.StoreInt64(&sm.performanceMonitor.metricsCollector.isCollecting, 1)

	sm.logger.Infow("Migration safety monitoring started",
		"migration_id", migrationID,
		"participants", len(participants),
	)

	return nil
}

// StopMigrationSafety stops safety monitoring for a migration
func (sm *SafetyManager) StopMigrationSafety(migrationID uuid.UUID) error {
	sm.safetyMu.Lock()
	defer sm.safetyMu.Unlock()

	// Remove from active migrations
	delete(sm.activeMigrations, migrationID)

	// Stop metrics collection if no active migrations
	if len(sm.activeMigrations) == 0 {
		atomic.StoreInt64(&sm.performanceMonitor.metricsCollector.isCollecting, 0)
	}

	sm.logger.Infow("Migration safety monitoring stopped", "migration_id", migrationID)
	return nil
}

// CheckSafety performs comprehensive safety checks for a migration
func (sm *SafetyManager) CheckSafety(ctx context.Context, migrationID uuid.UUID) (*SafetyReport, error) {
	sm.safetyMu.RLock()
	safetyState, exists := sm.activeMigrations[migrationID]
	sm.safetyMu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("no safety state found for migration %s", migrationID)
	}

	report := &SafetyReport{
		MigrationID: migrationID,
		Timestamp:   time.Now(),
		Checks:      make(map[string]*SafetyCheck),
	}

	// Performance safety check
	perfCheck, err := sm.checkPerformanceSafety(ctx, safetyState)
	if err != nil {
		sm.logger.Warnw("Performance safety check failed", "error", err)
	}
	report.Checks["performance"] = perfCheck

	// Integrity safety check
	integrityCheck, err := sm.checkIntegritySafety(ctx, safetyState)
	if err != nil {
		sm.logger.Warnw("Integrity safety check failed", "error", err)
	}
	report.Checks["integrity"] = integrityCheck

	// Health safety check
	healthCheck, err := sm.checkHealthSafety(ctx, safetyState)
	if err != nil {
		sm.logger.Warnw("Health safety check failed", "error", err)
	}
	report.Checks["health"] = healthCheck

	// Circuit breaker check
	circuitCheck := sm.checkCircuitBreakerSafety(ctx, safetyState)
	report.Checks["circuit_breaker"] = circuitCheck

	// Determine overall safety status
	report.OverallStatus = sm.determineOverallSafetyStatus(report.Checks)

	// Check for violations and trigger actions if needed
	violations := sm.detectViolations(report)
	if len(violations) > 0 {
		sm.handleViolations(ctx, migrationID, violations)
	}

	return report, nil
}

// SafetyReport contains comprehensive safety check results
type SafetyReport struct {
	MigrationID     uuid.UUID               `json:"migration_id"`
	Timestamp       time.Time               `json:"timestamp"`
	OverallStatus   SafetyStatus            `json:"overall_status"`
	Checks          map[string]*SafetyCheck `json:"checks"`
	Violations      []SafetyViolation       `json:"violations"`
	Recommendations []string                `json:"recommendations"`
}

// SafetyCheck represents results of a specific safety check
type SafetyCheck struct {
	CheckType     string                 `json:"check_type"`
	Status        SafetyStatus           `json:"status"`
	Score         float64                `json:"score"`
	Message       string                 `json:"message"`
	Metrics       map[string]interface{} `json:"metrics"`
	Violations    []SafetyViolation      `json:"violations"`
	LastCheck     time.Time              `json:"last_check"`
	CheckDuration time.Duration          `json:"check_duration"`
}

// Helper methods for safety checks

func (sm *SafetyManager) checkPerformanceSafety(ctx context.Context, state *MigrationSafetyState) (*SafetyCheck, error) {
	startTime := time.Now()

	// Collect current metrics
	currentMetrics, err := sm.collectCurrentMetrics(ctx)
	if err != nil {
		return &SafetyCheck{
			CheckType:     "performance",
			Status:        SafetyStatusCritical,
			Score:         0.0,
			Message:       fmt.Sprintf("Failed to collect metrics: %v", err),
			LastCheck:     time.Now(),
			CheckDuration: time.Since(startTime),
		}, err
	}

	// Compare with baseline
	comparison := sm.compareWithBaseline(currentMetrics, state.PerformanceBaseline)

	// Determine status and score
	status := SafetyStatusHealthy
	score := 1.0
	violations := make([]SafetyViolation, 0)

	// Check latency degradation
	if comparison.LatencyComparison.ChangePercentage > sm.config.MaxLatencyIncrease {
		violation := SafetyViolation{
			ID:          uuid.New(),
			Timestamp:   time.Now(),
			Type:        ViolationTypePerformance,
			Severity:    SeverityHigh,
			Description: fmt.Sprintf("Latency increased by %.2f%%", comparison.LatencyComparison.ChangePercentage*100),
			Metrics: map[string]interface{}{
				"baseline_latency": comparison.LatencyComparison.BaselineValue,
				"current_latency":  comparison.LatencyComparison.CurrentValue,
				"change_percent":   comparison.LatencyComparison.ChangePercentage,
			},
			Actions: []string{"monitor_closely", "consider_rollback"},
		}
		violations = append(violations, violation)
		status = SafetyStatusCritical
		score = 0.3
	}

	// Check throughput degradation
	if comparison.ThroughputComparison.ChangePercentage < -sm.config.MaxThroughputDecrease {
		violation := SafetyViolation{
			ID:          uuid.New(),
			Timestamp:   time.Now(),
			Type:        ViolationTypePerformance,
			Severity:    SeverityHigh,
			Description: fmt.Sprintf("Throughput decreased by %.2f%%", -comparison.ThroughputComparison.ChangePercentage*100),
			Metrics: map[string]interface{}{
				"baseline_throughput": comparison.ThroughputComparison.BaselineValue,
				"current_throughput":  comparison.ThroughputComparison.CurrentValue,
				"change_percent":      comparison.ThroughputComparison.ChangePercentage,
			},
			Actions: []string{"monitor_closely", "consider_rollback"},
		}
		violations = append(violations, violation)
		if status != SafetyStatusCritical {
			status = SafetyStatusWarning
			score = 0.6
		}
	}

	// Check error rate increase
	if comparison.ErrorRateComparison.ChangePercentage > sm.config.MaxErrorRateIncrease {
		violation := SafetyViolation{
			ID:          uuid.New(),
			Timestamp:   time.Now(),
			Type:        ViolationTypePerformance,
			Severity:    SeverityCritical,
			Description: fmt.Sprintf("Error rate increased by %.2f%%", comparison.ErrorRateComparison.ChangePercentage*100),
			Metrics: map[string]interface{}{
				"baseline_error_rate": comparison.ErrorRateComparison.BaselineValue,
				"current_error_rate":  comparison.ErrorRateComparison.CurrentValue,
				"change_percent":      comparison.ErrorRateComparison.ChangePercentage,
			},
			Actions: []string{"immediate_rollback"},
		}
		violations = append(violations, violation)
		status = SafetyStatusCritical
		score = 0.1
	}

	return &SafetyCheck{
		CheckType:     "performance",
		Status:        status,
		Score:         score,
		Message:       fmt.Sprintf("Performance check completed with %d violations", len(violations)),
		Metrics:       map[string]interface{}{"comparison": comparison},
		Violations:    violations,
		LastCheck:     time.Now(),
		CheckDuration: time.Since(startTime),
	}, nil
}

func (sm *SafetyManager) checkIntegritySafety(ctx context.Context, state *MigrationSafetyState) (*SafetyCheck, error) {
	startTime := time.Now()

	violations := make([]SafetyViolation, 0)
	status := SafetyStatusHealthy
	score := 1.0

	// Check order consistency
	if sm.config.OrderConsistencyCheck {
		if err := sm.integrityValidator.validateOrderConsistency(ctx); err != nil {
			violation := SafetyViolation{
				ID:          uuid.New(),
				Timestamp:   time.Now(),
				Type:        ViolationTypeIntegrity,
				Severity:    SeverityCritical,
				Description: fmt.Sprintf("Order consistency violation: %v", err),
				Actions:     []string{"immediate_rollback", "data_recovery"},
			}
			violations = append(violations, violation)
			status = SafetyStatusCritical
			score = 0.0
		}
	}

	// Check data integrity
	if sm.config.DataIntegrityCheck {
		if err := sm.integrityValidator.validateDataIntegrity(ctx); err != nil {
			violation := SafetyViolation{
				ID:          uuid.New(),
				Timestamp:   time.Now(),
				Type:        ViolationTypeIntegrity,
				Severity:    SeverityHigh,
				Description: fmt.Sprintf("Data integrity violation: %v", err),
				Actions:     []string{"rollback", "data_validation"},
			}
			violations = append(violations, violation)
			if status != SafetyStatusCritical {
				status = SafetyStatusWarning
				score = 0.5
			}
		}
	}

	return &SafetyCheck{
		CheckType:     "integrity",
		Status:        status,
		Score:         score,
		Message:       fmt.Sprintf("Integrity check completed with %d violations", len(violations)),
		Violations:    violations,
		LastCheck:     time.Now(),
		CheckDuration: time.Since(startTime),
	}, nil
}

func (sm *SafetyManager) checkHealthSafety(ctx context.Context, state *MigrationSafetyState) (*SafetyCheck, error) {
	startTime := time.Now()

	// Perform health checks on all participants
	healthResults := sm.healthMonitor.performHealthChecks(ctx)

	violations := make([]SafetyViolation, 0)
	status := SafetyStatusHealthy
	score := healthResults.OverallScore

	// Check overall health score
	if healthResults.OverallScore < sm.config.MinHealthScore {
		violation := SafetyViolation{
			ID:          uuid.New(),
			Timestamp:   time.Now(),
			Type:        ViolationTypeHealth,
			Severity:    SeverityHigh,
			Description: fmt.Sprintf("Overall health score below threshold: %.2f", healthResults.OverallScore),
			Metrics: map[string]interface{}{
				"health_score":         healthResults.OverallScore,
				"min_health_score":     sm.config.MinHealthScore,
				"healthy_participants": healthResults.HealthyParticipants,
				"total_participants":   healthResults.TotalParticipants,
			},
			Actions: []string{"investigate", "monitor_closely"},
		}
		violations = append(violations, violation)

		if healthResults.OverallScore < 0.5 {
			status = SafetyStatusCritical
		} else {
			status = SafetyStatusWarning
		}
	}

	// Check for critical issues
	if len(healthResults.CriticalIssues) > 0 {
		violation := SafetyViolation{
			ID:          uuid.New(),
			Timestamp:   time.Now(),
			Type:        ViolationTypeHealth,
			Severity:    SeverityCritical,
			Description: fmt.Sprintf("Critical health issues detected: %v", healthResults.CriticalIssues),
			Actions:     []string{"immediate_investigation", "consider_rollback"},
		}
		violations = append(violations, violation)
		status = SafetyStatusCritical
	}

	return &SafetyCheck{
		CheckType:     "health",
		Status:        status,
		Score:         score,
		Message:       fmt.Sprintf("Health check completed with %d violations", len(violations)),
		Metrics:       map[string]interface{}{"health_results": healthResults},
		Violations:    violations,
		LastCheck:     time.Now(),
		CheckDuration: time.Since(startTime),
	}, nil
}

func (sm *SafetyManager) checkCircuitBreakerSafety(ctx context.Context, state *MigrationSafetyState) *SafetyCheck {
	startTime := time.Now()

	violations := make([]SafetyViolation, 0)
	status := SafetyStatusHealthy
	score := 1.0

	// Check circuit breaker state
	sm.circuitBreaker.mu.RLock()
	circuitState := sm.circuitBreaker.state
	failureCount := atomic.LoadInt64(&sm.circuitBreaker.failureCount)
	sm.circuitBreaker.mu.RUnlock()

	if circuitState == "open" {
		violation := SafetyViolation{
			ID:          uuid.New(),
			Timestamp:   time.Now(),
			Type:        ViolationTypeCircuitBreaker,
			Severity:    SeverityCritical,
			Description: "Circuit breaker is open - migration protection triggered",
			Metrics: map[string]interface{}{
				"circuit_state": circuitState,
				"failure_count": failureCount,
			},
			Actions: []string{"immediate_rollback", "investigate_failures"},
		}
		violations = append(violations, violation)
		status = SafetyStatusCritical
		score = 0.0
	} else if circuitState == "half-open" {
		status = SafetyStatusWarning
		score = 0.7
	}

	return &SafetyCheck{
		CheckType: "circuit_breaker",
		Status:    status,
		Score:     score,
		Message:   fmt.Sprintf("Circuit breaker state: %s", circuitState),
		Metrics: map[string]interface{}{
			"state":         circuitState,
			"failure_count": failureCount,
		},
		Violations:    violations,
		LastCheck:     time.Now(),
		CheckDuration: time.Since(startTime),
	}
}

// Stub implementations for helper methods

func (sm *SafetyManager) capturePerformanceBaseline(ctx context.Context) (*PerformanceBaseline, error) {
	// Implement baseline capture logic
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
		SamplingDuration: 30 * time.Second,
	}, nil
}

func (sm *SafetyManager) collectCurrentMetrics(ctx context.Context) (*PerformanceMetrics, error) {
	// Implement current metrics collection
	return &PerformanceMetrics{
		Timestamp: time.Now(),
		Latency: &LatencyMetrics{
			P50:     2 * time.Millisecond,
			P95:     5 * time.Millisecond,
			P99:     10 * time.Millisecond,
			Average: 3 * time.Millisecond,
			Current: 2 * time.Millisecond,
		},
		Throughput: &ThroughputMetrics{
			Current:         1000.0,
			Average:         950.0,
			Peak:            1200.0,
			OrdersPerSecond: 500.0,
			TradesPerSecond: 100.0,
		},
		ErrorRate: &ErrorRateMetrics{
			Current:         0.001,
			Average:         0.0015,
			TotalErrors:     10,
			TotalRequests:   10000,
			ErrorsPerSecond: 0.1,
		},
		ResourceUsage: &ResourceMetrics{
			CPUUsage:        0.35,
			MemoryUsage:     110 * 1024 * 1024,
			NetworkUsage:    1024 * 1024,
			DiskUsage:       512 * 1024,
			ConnectionCount: 55,
		},
	}, nil
}

func (sm *SafetyManager) compareWithBaseline(current *PerformanceMetrics, baseline *PerformanceBaseline) *ComparisonResults {
	// Implement comparison logic
	latencyChange := (float64(current.Latency.Average) - float64(baseline.AvgLatency)) / float64(baseline.AvgLatency)
	throughputChange := (current.Throughput.Average - baseline.ThroughputTPS) / baseline.ThroughputTPS
	errorRateChange := (current.ErrorRate.Average - baseline.ErrorRate) / baseline.ErrorRate

	return &ComparisonResults{
		LatencyComparison: &MetricComparison{
			BaselineValue:    float64(baseline.AvgLatency.Nanoseconds()),
			CurrentValue:     float64(current.Latency.Average.Nanoseconds()),
			ChangePercentage: latencyChange,
			Trend:            determineTrend(latencyChange),
			Status:           determineStatus(latencyChange, sm.config.MaxLatencyIncrease),
			WithinThreshold:  latencyChange <= sm.config.MaxLatencyIncrease,
		},
		ThroughputComparison: &MetricComparison{
			BaselineValue:    baseline.ThroughputTPS,
			CurrentValue:     current.Throughput.Average,
			ChangePercentage: throughputChange,
			Trend:            determineTrend(throughputChange),
			Status:           determineStatus(-throughputChange, sm.config.MaxThroughputDecrease),
			WithinThreshold:  throughputChange >= -sm.config.MaxThroughputDecrease,
		},
		ErrorRateComparison: &MetricComparison{
			BaselineValue:    baseline.ErrorRate,
			CurrentValue:     current.ErrorRate.Average,
			ChangePercentage: errorRateChange,
			Trend:            determineTrend(errorRateChange),
			Status:           determineStatus(errorRateChange, sm.config.MaxErrorRateIncrease),
			WithinThreshold:  errorRateChange <= sm.config.MaxErrorRateIncrease,
		},
		OverallScore:   calculateOverallScore(latencyChange, throughputChange, errorRateChange),
		Recommendation: generateRecommendation(latencyChange, throughputChange, errorRateChange),
	}
}

func (sm *SafetyManager) determineOverallSafetyStatus(checks map[string]*SafetyCheck) SafetyStatus {
	criticalCount := 0
	warningCount := 0

	for _, check := range checks {
		switch check.Status {
		case SafetyStatusCritical:
			criticalCount++
		case SafetyStatusWarning:
			warningCount++
		}
	}

	if criticalCount > 0 {
		return SafetyStatusCritical
	}
	if warningCount > 0 {
		return SafetyStatusWarning
	}
	return SafetyStatusHealthy
}

func (sm *SafetyManager) detectViolations(report *SafetyReport) []SafetyViolation {
	violations := make([]SafetyViolation, 0)

	for _, check := range report.Checks {
		violations = append(violations, check.Violations...)
	}

	return violations
}

func (sm *SafetyManager) handleViolations(ctx context.Context, migrationID uuid.UUID, violations []SafetyViolation) {
	for _, violation := range violations {
		atomic.AddInt64(&sm.totalViolations, 1)

		// Create alert
		alert := &Alert{
			ID:          uuid.New(),
			Timestamp:   time.Now(),
			Severity:    violation.Severity,
			Type:        violation.Type,
			Title:       fmt.Sprintf("Safety Violation: %s", violation.Type),
			Description: violation.Description,
			MigrationID: migrationID,
			Metrics:     violation.Metrics,
			Actions:     violation.Actions,
		}

		// Send alert
		sm.alertManager.sendAlert(ctx, alert)

		// Check if automatic rollback should be triggered
		if sm.shouldTriggerAutoRollback(violation) {
			sm.triggerAutoRollback(ctx, migrationID, violation)
		}
	}
}

func (sm *SafetyManager) shouldTriggerAutoRollback(violation SafetyViolation) bool {
	if !sm.config.AutoRollbackEnabled {
		return false
	}

	// Trigger rollback for critical violations or specific violation types
	return violation.Severity == SeverityCritical ||
		violation.Type == ViolationTypeIntegrity ||
		(violation.Type == ViolationTypeCircuitBreaker)
}

func (sm *SafetyManager) triggerAutoRollback(ctx context.Context, migrationID uuid.UUID, violation SafetyViolation) {
	atomic.AddInt64(&sm.totalRollbacks, 1)

	rollbackCtx, cancel := context.WithTimeout(ctx, sm.config.RollbackTimeout)
	defer cancel()

	trigger := TriggerPerformanceDegradation
	switch violation.Type {
	case ViolationTypeIntegrity:
		trigger = TriggerIntegrityViolation
	case ViolationTypeHealth:
		trigger = TriggerHealthFailure
	case ViolationTypeCircuitBreaker:
		trigger = TriggerCircuitBreaker
	}

	attempt := RollbackAttempt{
		ID:         uuid.New(),
		Timestamp:  time.Now(),
		Reason:     violation.Description,
		Trigger:    trigger,
		Violations: []SafetyViolation{violation},
	}

	startTime := time.Now()
	err := sm.rollbackManager.coordinator.AbortMigration(rollbackCtx, migrationID, fmt.Sprintf("Auto-rollback triggered: %s", violation.Description))
	attempt.Duration = time.Since(startTime)

	if err != nil {
		attempt.Success = false
		attempt.ErrorMsg = err.Error()
		sm.logger.Errorw("Auto-rollback failed", "error", err, "migration_id", migrationID)
	} else {
		attempt.Success = true
		sm.logger.Infow("Auto-rollback completed successfully", "migration_id", migrationID, "reason", violation.Description)
	}

	// Record rollback attempt
	sm.rollbackManager.mu.Lock()
	if sm.rollbackManager.rollbackHistory[migrationID] == nil {
		sm.rollbackManager.rollbackHistory[migrationID] = make([]RollbackAttempt, 0)
	}
	sm.rollbackManager.rollbackHistory[migrationID] = append(sm.rollbackManager.rollbackHistory[migrationID], attempt)
	sm.rollbackManager.mu.Unlock()
}

func (sm *SafetyManager) startBackgroundMonitoring() {
	// Start health monitoring
	go sm.healthMonitorWorker()

	// Start metrics collection
	go sm.metricsCollectionWorker()

	// Start circuit breaker monitoring
	go sm.circuitBreakerWorker()
}

func (sm *SafetyManager) healthMonitorWorker() {
	ticker := time.NewTicker(sm.config.HealthCheckInterval)
	defer ticker.Stop()

	for range ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), sm.config.HealthCheckTimeout)
		sm.healthMonitor.performHealthChecks(ctx)
		cancel()
	}
}

func (sm *SafetyManager) metricsCollectionWorker() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		if atomic.LoadInt64(&sm.performanceMonitor.metricsCollector.isCollecting) == 1 {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			sm.collectCurrentMetrics(ctx)
			cancel()
		}
	}
}

func (sm *SafetyManager) circuitBreakerWorker() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		sm.circuitBreaker.checkState()
	}
}

// Stub implementations for various helper functions and interfaces

func createDefaultValidationRules() []ValidationRule {
	return []ValidationRule{
		{
			Name:        "order_count_consistency",
			Description: "Validate order count consistency",
			Severity:    SeverityHigh,
			Enabled:     true,
		},
		{
			Name:        "balance_consistency",
			Description: "Validate balance consistency",
			Severity:    SeverityCritical,
			Enabled:     true,
		},
	}
}

func createDefaultComparisonRules() []ComparisonRule {
	return []ComparisonRule{
		{
			MetricName:     "latency",
			ComparisonType: "percentage_increase",
			Threshold:      0.5,
			Severity:       SeverityHigh,
			Enabled:        true,
		},
		{
			MetricName:     "throughput",
			ComparisonType: "percentage_decrease",
			Threshold:      0.3,
			Severity:       SeverityMedium,
			Enabled:        true,
		},
	}
}

func createDefaultEscalationRules() []EscalationRule {
	return []EscalationRule{
		{
			Severity:        SeverityCritical,
			EscalationDelay: 1 * time.Minute,
			EscalationLevel: 1,
			Channels:        []string{"log", "metrics", "ops_team"},
			Enabled:         true,
		},
		{
			Severity:        SeverityHigh,
			EscalationDelay: 5 * time.Minute,
			EscalationLevel: 1,
			Channels:        []string{"log", "metrics"},
			Enabled:         true,
		},
	}
}

func (iv *IntegrityValidator) validateOrderConsistency(ctx context.Context) error {
	// Implement order consistency validation
	return nil
}

func (iv *IntegrityValidator) validateDataIntegrity(ctx context.Context) error {
	// Implement data integrity validation
	return nil
}

func (hm *HealthMonitor) performHealthChecks(ctx context.Context) *OverallHealth {
	// Implement health checks
	return &OverallHealth{
		IsHealthy:           true,
		OverallScore:        0.95,
		HealthyParticipants: 3,
		TotalParticipants:   3,
		LastCheck:           time.Now(),
		CriticalIssues:      []string{},
	}
}

func (cb *CircuitBreaker) checkState() {
	// Implement circuit breaker state checking
}

func (am *AlertManager) sendAlert(ctx context.Context, alert *Alert) {
	// Implement alert sending
	am.mu.Lock()
	am.activeAlerts[alert.ID] = alert
	am.alertHistory = append(am.alertHistory, *alert)
	am.mu.Unlock()

	atomic.AddInt64(&am.logger.(*SafetyManager).totalAlerts, 1)
}

func determineTrend(changePercentage float64) string {
	if changePercentage > 0.1 {
		return "increasing"
	} else if changePercentage < -0.1 {
		return "decreasing"
	}
	return "stable"
}

func determineStatus(changePercentage, threshold float64) string {
	if changePercentage > threshold {
		return "violation"
	} else if changePercentage > threshold*0.8 {
		return "warning"
	}
	return "ok"
}

func calculateOverallScore(latency, throughput, errorRate float64) float64 {
	// Simple scoring algorithm
	score := 1.0
	score -= latency * 0.3
	score += throughput * 0.3
	score -= errorRate * 0.4

	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}

	return score
}

func generateRecommendation(latency, throughput, errorRate float64) string {
	if errorRate > 0.1 {
		return "immediate_rollback"
	} else if latency > 0.5 || throughput < -0.3 {
		return "monitor_closely"
	}
	return "continue_migration"
}
