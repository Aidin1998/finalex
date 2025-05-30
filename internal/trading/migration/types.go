// =============================
// Two-Phase Commit Migration Types
// =============================
// This file defines core types for the two-phase commit migration protocol
// that ensures zero-downtime order book migrations with distributed consensus.

package migration

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// MigrationPhase represents the current phase of migration
type MigrationPhase int

const (
	PhaseIdle MigrationPhase = iota
	PhasePrepare
	PhaseCommit
	PhaseAbort
	PhaseCompleted
	PhaseFailed
)

func (p MigrationPhase) String() string {
	switch p {
	case PhaseIdle:
		return "idle"
	case PhasePrepare:
		return "prepare"
	case PhaseCommit:
		return "commit"
	case PhaseAbort:
		return "abort"
	case PhaseCompleted:
		return "completed"
	case PhaseFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// MigrationStatus represents the overall status of migration
type MigrationStatus string

const (
	StatusPending    MigrationStatus = "pending"
	StatusPreparing  MigrationStatus = "preparing"
	StatusPrepared   MigrationStatus = "prepared"
	StatusCommitting MigrationStatus = "committing"
	StatusCommitted  MigrationStatus = "committed"
	StatusAborting   MigrationStatus = "aborting"
	StatusAborted    MigrationStatus = "aborted"
	StatusCompleted  MigrationStatus = "completed"
	StatusFailed     MigrationStatus = "failed"
)

// ParticipantVote represents a participant's vote in the prepare phase
type ParticipantVote string

const (
	VoteYes     ParticipantVote = "yes"
	VoteNo      ParticipantVote = "no"
	VotePending ParticipantVote = "pending"
	VoteTimeout ParticipantVote = "timeout"
)

// MigrationRequest represents a migration request
type MigrationRequest struct {
	ID          uuid.UUID              `json:"id"`
	Pair        string                 `json:"pair"`
	Priority    int                    `json:"priority"`
	Config      *MigrationConfig       `json:"config"`
	Metadata    map[string]interface{} `json:"metadata"`
	RequestedBy string                 `json:"requested_by"`
	RequestedAt time.Time              `json:"requested_at"`
}

// MigrationConfig contains configuration for a migration
type MigrationConfig struct {
	// Migration parameters
	TargetImplementation string `json:"target_implementation"`
	MigrationPercentage  int    `json:"migration_percentage"`
	BatchSize            int    `json:"batch_size"`
	ValidationEnabled    bool   `json:"validation_enabled"`

	// Timeouts
	PrepareTimeout time.Duration `json:"prepare_timeout"`
	CommitTimeout  time.Duration `json:"commit_timeout"`
	OverallTimeout time.Duration `json:"overall_timeout"`

	// Safety mechanisms
	MaxRetries          int           `json:"max_retries"`
	AutoRollbackOnFail  bool          `json:"auto_rollback_on_fail"`
	HealthCheckInterval time.Duration `json:"health_check_interval"`

	// Performance requirements
	MaxDowntimeMs      int64   `json:"max_downtime_ms"`
	MaxLatencyIncrease float64 `json:"max_latency_increase"`
	MinSuccessRate     float64 `json:"min_success_rate"`
}

// DefaultMigrationConfig returns safe defaults
func DefaultMigrationConfig() *MigrationConfig {
	return &MigrationConfig{
		TargetImplementation: "deadlock_safe",
		MigrationPercentage:  100,
		BatchSize:            1000,
		ValidationEnabled:    true,
		PrepareTimeout:       30 * time.Second,
		CommitTimeout:        10 * time.Second,
		OverallTimeout:       5 * time.Minute,
		MaxRetries:           3,
		AutoRollbackOnFail:   true,
		HealthCheckInterval:  5 * time.Second,
		MaxDowntimeMs:        100,
		MaxLatencyIncrease:   0.1,   // 10%
		MinSuccessRate:       0.999, // 99.9%
	}
}

// MigrationState represents the current state of a migration
type MigrationState struct {
	ID     uuid.UUID        `json:"id"`
	Pair   string           `json:"pair"`
	Phase  MigrationPhase   `json:"phase"`
	Status MigrationStatus  `json:"status"`
	Config *MigrationConfig `json:"config"`

	// Timing
	StartTime   time.Time  `json:"start_time"`
	PrepareTime *time.Time `json:"prepare_time,omitempty"`
	CommitTime  *time.Time `json:"commit_time,omitempty"`
	EndTime     *time.Time `json:"end_time,omitempty"`

	// Participants and voting
	Participants map[string]*ParticipantState `json:"participants"`
	VotesSummary VotesSummary                 `json:"votes_summary"`

	// Progress tracking
	Progress       float64 `json:"progress"`
	OrdersMigrated int64   `json:"orders_migrated"`
	TotalOrders    int64   `json:"total_orders"`
	ErrorCount     int64   `json:"error_count"`
	RetryCount     int64   `json:"retry_count"`

	// Performance metrics
	Metrics      *MigrationMetrics `json:"metrics"`
	HealthStatus *HealthStatus     `json:"health_status"`

	// Rollback information
	CanRollback    bool   `json:"can_rollback"`
	RollbackReason string `json:"rollback_reason,omitempty"`

	// Synchronization
	mu sync.RWMutex `json:"-"`
}

// ParticipantState represents the state of a migration participant
type ParticipantState struct {
	ID              string          `json:"id"`
	Type            string          `json:"type"` // coordinator, order_book, persistence, etc.
	Vote            ParticipantVote `json:"vote"`
	VoteTime        *time.Time      `json:"vote_time,omitempty"`
	LastHeartbeat   time.Time       `json:"last_heartbeat"`
	IsHealthy       bool            `json:"is_healthy"`
	ErrorMessage    string          `json:"error_message,omitempty"`
	OrdersSnapshot  *OrdersSnapshot `json:"orders_snapshot,omitempty"`
	PreparationData interface{}     `json:"preparation_data,omitempty"`
}

// VotesSummary summarizes the voting results
type VotesSummary struct {
	TotalParticipants int             `json:"total_participants"`
	YesVotes          int             `json:"yes_votes"`
	NoVotes           int             `json:"no_votes"`
	PendingVotes      int             `json:"pending_votes"`
	TimeoutVotes      int             `json:"timeout_votes"`
	AllVotesIn        bool            `json:"all_votes_in"`
	Consensus         ParticipantVote `json:"consensus"`
}

// OrdersSnapshot represents a snapshot of orders for validation
type OrdersSnapshot struct {
	Timestamp    time.Time                      `json:"timestamp"`
	TotalOrders  int64                          `json:"total_orders"`
	OrdersByType map[string]int64               `json:"orders_by_type"`
	TotalVolume  decimal.Decimal                `json:"total_volume"`
	PriceLevels  map[string]*PriceLevelSnapshot `json:"price_levels"`
	Checksum     string                         `json:"checksum"`
}

// PriceLevelSnapshot represents a snapshot of a price level
type PriceLevelSnapshot struct {
	Price      decimal.Decimal `json:"price"`
	Volume     decimal.Decimal `json:"volume"`
	OrderCount int             `json:"order_count"`
	FirstOrder time.Time       `json:"first_order"`
	LastOrder  time.Time       `json:"last_order"`
}

// MigrationMetrics contains performance metrics for migration
type MigrationMetrics struct {
	// Latency metrics
	AvgLatencyMs float64 `json:"avg_latency_ms"`
	P50LatencyMs float64 `json:"p50_latency_ms"`
	P95LatencyMs float64 `json:"p95_latency_ms"`
	P99LatencyMs float64 `json:"p99_latency_ms"`
	MaxLatencyMs float64 `json:"max_latency_ms"`

	// Throughput metrics
	OrdersPerSecond float64 `json:"orders_per_second"`
	TradesPerSecond float64 `json:"trades_per_second"`

	// Error metrics
	ErrorRate   float64 `json:"error_rate"`
	TimeoutRate float64 `json:"timeout_rate"`

	// Downtime metrics
	DowntimeMs     int64 `json:"downtime_ms"`
	ReadOnlyModeMs int64 `json:"readonly_mode_ms"`

	// Comparison with baseline
	LatencyImprovement    float64 `json:"latency_improvement"`
	ThroughputImprovement float64 `json:"throughput_improvement"`
	ErrorRateImprovement  float64 `json:"error_rate_improvement"`
}

// HealthStatus represents the health status of the migration
type HealthStatus struct {
	IsHealthy      bool                    `json:"is_healthy"`
	LastCheck      time.Time               `json:"last_check"`
	HealthChecks   map[string]*HealthCheck `json:"health_checks"`
	OverallScore   float64                 `json:"overall_score"`
	CriticalIssues []string                `json:"critical_issues"`
	Warnings       []string                `json:"warnings"`
}

// HealthCheck represents an individual health check
type HealthCheck struct {
	Name       string    `json:"name"`
	Status     string    `json:"status"` // healthy, warning, critical
	Score      float64   `json:"score"`  // 0.0 - 1.0
	Message    string    `json:"message"`
	LastCheck  time.Time `json:"last_check"`
	CheckCount int64     `json:"check_count"`
	FailCount  int64     `json:"fail_count"`
}

// MigrationEvent represents an event in the migration lifecycle
type MigrationEvent struct {
	ID          uuid.UUID              `json:"id"`
	MigrationID uuid.UUID              `json:"migration_id"`
	Timestamp   time.Time              `json:"timestamp"`
	Type        string                 `json:"type"`
	Phase       MigrationPhase         `json:"phase"`
	Participant string                 `json:"participant,omitempty"`
	Data        map[string]interface{} `json:"data"`
	Error       string                 `json:"error,omitempty"`
}

// LockInfo represents information about a distributed lock
type LockInfo struct {
	ID         uuid.UUID              `json:"id"`
	Resource   string                 `json:"resource"`
	Owner      string                 `json:"owner"`
	AcquiredAt time.Time              `json:"acquired_at"`
	ExpiresAt  time.Time              `json:"expires_at"`
	TTL        time.Duration          `json:"ttl"`
	IsActive   bool                   `json:"is_active"`
	Metadata   map[string]interface{} `json:"metadata"`
}

// MigrationParticipant defines the interface for migration participants
type MigrationParticipant interface {
	// Identification
	GetID() string
	GetType() string

	// Two-phase commit protocol
	Prepare(ctx context.Context, migrationID uuid.UUID, config *MigrationConfig) (*ParticipantState, error)
	Commit(ctx context.Context, migrationID uuid.UUID) error
	Abort(ctx context.Context, migrationID uuid.UUID) error

	// Health and status
	HealthCheck(ctx context.Context) (*HealthCheck, error)
	GetSnapshot(ctx context.Context) (*OrdersSnapshot, error)

	// Cleanup
	Cleanup(ctx context.Context, migrationID uuid.UUID) error
}

// MigrationCoordinator defines the interface for the migration coordinator
type MigrationCoordinator interface {
	// Migration lifecycle
	StartMigration(ctx context.Context, request *MigrationRequest) (*MigrationState, error)
	GetMigrationState(migrationID uuid.UUID) (*MigrationState, error)
	ListMigrations(filter *MigrationFilter) ([]*MigrationState, error)

	// Control operations
	AbortMigration(ctx context.Context, migrationID uuid.UUID, reason string) error
	RetryMigration(ctx context.Context, migrationID uuid.UUID) error

	// Participant management
	RegisterParticipant(participant MigrationParticipant) error
	UnregisterParticipant(participantID string) error

	// Monitoring
	GetMetrics(migrationID uuid.UUID) (*MigrationMetrics, error)
	GetHealthStatus(migrationID uuid.UUID) (*HealthStatus, error)

	// Event streaming
	Subscribe(ctx context.Context, migrationID uuid.UUID) (<-chan *MigrationEvent, error)
}

// MigrationFilter for querying migrations
type MigrationFilter struct {
	Pair      string          `json:"pair,omitempty"`
	Status    MigrationStatus `json:"status,omitempty"`
	Phase     MigrationPhase  `json:"phase,omitempty"`
	StartTime *time.Time      `json:"start_time,omitempty"`
	EndTime   *time.Time      `json:"end_time,omitempty"`
	Limit     int             `json:"limit,omitempty"`
	Offset    int             `json:"offset,omitempty"`
}

// StateChangeCallback is called when migration state changes
type StateChangeCallback func(oldState, newState *MigrationState)

// EventCallback is called when migration events occur
type EventCallback func(event *MigrationEvent)
