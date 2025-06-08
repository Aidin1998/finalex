// Package transaction implements a robust, ACID-compliant transaction coordination layer
// for the crosspair module with distributed XA/two-phase commit support
package transaction

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Aidin1998/finalex/internal/accounts/transaction"
	"github.com/Aidin1998/finalex/internal/trading/crosspair"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// CrossPairTransactionType represents types of cross-pair transactions
type CrossPairTransactionType string

const (
	TransactionTypeTrade      CrossPairTransactionType = "TRADE"
	TransactionTypeWithdrawal CrossPairTransactionType = "WITHDRAWAL"
	TransactionTypeSettlement CrossPairTransactionType = "SETTLEMENT"
	TransactionTypeTransfer   CrossPairTransactionType = "TRANSFER"
)

// TransactionState represents the state of a cross-pair transaction
type TransactionState int64

const (
	StateInitialized TransactionState = iota
	StateValidating
	StateValidated
	StatePreparing
	StatePrepared
	StateCommitting
	StateCommitted
	StateAborting
	StateAborted
	StateCompensating
	StateCompensated
	StateFailed
	StateDeadLettered
)

// String returns the string representation of the transaction state
func (s TransactionState) String() string {
	states := []string{
		"INITIALIZED", "VALIDATING", "VALIDATED", "PREPARING", "PREPARED",
		"COMMITTING", "COMMITTED", "ABORTING", "ABORTED", "COMPENSATING",
		"COMPENSATED", "FAILED", "DEAD_LETTERED",
	}
	if int(s) < len(states) {
		return states[s]
	}
	return "UNKNOWN"
}

// CrossPairTransactionContext holds context for cross-pair transactions
type CrossPairTransactionContext struct {
	// Transaction metadata
	ID        uuid.UUID                `json:"id"`
	XID       transaction.XID          `json:"xid"`
	TraceID   string                   `json:"trace_id"`
	Type      CrossPairTransactionType `json:"type"`
	UserID    uuid.UUID                `json:"user_id"`
	CreatedAt time.Time                `json:"created_at"`
	UpdatedAt time.Time                `json:"updated_at"`
	TimeoutAt time.Time                `json:"timeout_at"`

	// Atomic state management
	state       int64 // TransactionState
	attempts    int64 // Retry attempt count
	lastAttempt int64 // Unix nanoseconds of last attempt

	// Transaction data
	Order *crosspair.CrossPairOrder `json:"order,omitempty"`
	Trade *crosspair.CrossPairTrade `json:"trade,omitempty"`
	Route *crosspair.CrossPairRoute `json:"route,omitempty"`

	// Resource participation
	Resources      []CrossPairResource      `json:"resources"`
	ResourceStates map[string]ResourceState `json:"resource_states"`

	// Compensation and rollback data
	CompensationActions []CompensationAction   `json:"compensation_actions,omitempty"`
	RollbackData        map[string]interface{} `json:"rollback_data,omitempty"`

	// Synchronization
	mu            sync.RWMutex  `json:"-"`
	completedChan chan struct{} `json:"-"`

	// Metrics and audit
	Metrics *TransactionMetrics `json:"metrics"`

	// Dead letter queue information
	DeadLetterInfo *DeadLetterInfo `json:"dead_letter_info,omitempty"`
}

// ResourceState represents the state of a resource in the transaction
type ResourceState struct {
	Name       string                      `json:"name"`
	State      transaction.XAResourceState `json:"state"`
	PreparedAt *time.Time                  `json:"prepared_at,omitempty"`
	Error      string                      `json:"error,omitempty"`
}

// CompensationAction represents an action to compensate for a failed transaction
type CompensationAction struct {
	ResourceName string                 `json:"resource_name"`
	Action       string                 `json:"action"`
	Data         map[string]interface{} `json:"data"`
	ExecutedAt   *time.Time             `json:"executed_at,omitempty"`
	Error        string                 `json:"error,omitempty"`
}

// TransactionMetrics tracks performance metrics for transactions
type TransactionMetrics struct {
	StartedAt        time.Time                `json:"started_at"`
	ValidatedAt      *time.Time               `json:"validated_at,omitempty"`
	PreparedAt       *time.Time               `json:"prepared_at,omitempty"`
	CommittedAt      *time.Time               `json:"committed_at,omitempty"`
	CompletedAt      *time.Time               `json:"completed_at,omitempty"`
	ValidationTime   time.Duration            `json:"validation_time"`
	PreparationTime  time.Duration            `json:"preparation_time"`
	CommitTime       time.Duration            `json:"commit_time"`
	TotalTime        time.Duration            `json:"total_time"`
	ResourcePrepTime map[string]time.Duration `json:"resource_prep_time"`
}

// DeadLetterInfo contains information about dead-lettered transactions
type DeadLetterInfo struct {
	Reason      string     `json:"reason"`
	RetryCount  int        `json:"retry_count"`
	LastError   string     `json:"last_error"`
	DeadAt      time.Time  `json:"dead_at"`
	NextRetryAt *time.Time `json:"next_retry_at,omitempty"`
}

// CrossPairResource represents a resource that can participate in cross-pair transactions
type CrossPairResource interface {
	transaction.XAResource

	// Cross-pair specific methods
	ValidateTransaction(ctx context.Context, txnCtx *CrossPairTransactionContext) error
	EstimateGas(ctx context.Context, txnCtx *CrossPairTransactionContext) (decimal.Decimal, error)
	GetCompensationAction(ctx context.Context, txnCtx *CrossPairTransactionContext) (*CompensationAction, error)
}

// CrossPairTransactionCoordinator manages distributed transactions for cross-pair operations
type CrossPairTransactionCoordinator struct {
	logger *zap.Logger

	// XA transaction management
	xaManager *transaction.XATransactionManager

	// Cross-pair specific components
	balanceService  crosspair.BalanceService
	matchingEngines map[string]crosspair.MatchingEngine
	tradeStore      crosspair.CrossPairTradeStore

	// Resource registry
	resources   map[string]CrossPairResource
	resourcesMu sync.RWMutex

	// Transaction registry
	transactions   map[uuid.UUID]*CrossPairTransactionContext
	transactionsMu sync.RWMutex

	// Configuration
	config *CoordinatorConfig

	// Performance optimization
	pool    *TransactionPool
	metrics *CoordinatorMetrics

	// Retry and dead letter queue management
	retryQueue      chan *CrossPairTransactionContext
	deadLetterQueue chan *CrossPairTransactionContext

	// Worker management
	workerPool   *WorkerPool
	shutdown     chan struct{}
	shutdownOnce sync.Once
	wg           sync.WaitGroup

	// Audit logging
	auditLogger *TransactionAuditLogger
}

// CoordinatorConfig holds configuration for the transaction coordinator
type CoordinatorConfig struct {
	// Timeout settings
	DefaultTimeout     time.Duration `json:"default_timeout"`
	ValidationTimeout  time.Duration `json:"validation_timeout"`
	PreparationTimeout time.Duration `json:"preparation_timeout"`
	CommitTimeout      time.Duration `json:"commit_timeout"`
	RollbackTimeout    time.Duration `json:"rollback_timeout"`

	// Retry settings
	MaxRetries         int           `json:"max_retries"`
	BaseRetryDelay     time.Duration `json:"base_retry_delay"`
	MaxRetryDelay      time.Duration `json:"max_retry_delay"`
	RetryBackoffFactor float64       `json:"retry_backoff_factor"`

	// Performance settings
	MaxConcurrentTxns int `json:"max_concurrent_txns"`
	WorkerPoolSize    int `json:"worker_pool_size"`
	PreallocatedPools int `json:"preallocated_pools"`

	// Dead letter queue settings
	DeadLetterEnabled    bool          `json:"dead_letter_enabled"`
	DeadLetterRetryAfter time.Duration `json:"dead_letter_retry_after"`
	DeadLetterMaxAge     time.Duration `json:"dead_letter_max_age"`

	// Audit settings
	AuditEnabled  bool   `json:"audit_enabled"`
	AuditLogPath  string `json:"audit_log_path"`
	AuditLogLevel string `json:"audit_log_level"`
}

// DefaultCoordinatorConfig returns default configuration
func DefaultCoordinatorConfig() *CoordinatorConfig {
	return &CoordinatorConfig{
		DefaultTimeout:       30 * time.Second,
		ValidationTimeout:    5 * time.Second,
		PreparationTimeout:   10 * time.Second,
		CommitTimeout:        15 * time.Second,
		RollbackTimeout:      10 * time.Second,
		MaxRetries:           3,
		BaseRetryDelay:       100 * time.Millisecond,
		MaxRetryDelay:        5 * time.Second,
		RetryBackoffFactor:   2.0,
		MaxConcurrentTxns:    1000,
		WorkerPoolSize:       10,
		PreallocatedPools:    100,
		DeadLetterEnabled:    true,
		DeadLetterRetryAfter: 1 * time.Hour,
		DeadLetterMaxAge:     24 * time.Hour,
		AuditEnabled:         true,
		AuditLogLevel:        "INFO",
	}
}

// CoordinatorMetrics tracks coordinator performance
type CoordinatorMetrics struct {
	// Transaction counters
	TotalTransactions        int64 `json:"total_transactions"`
	CommittedTransactions    int64 `json:"committed_transactions"`
	AbortedTransactions      int64 `json:"aborted_transactions"`
	FailedTransactions       int64 `json:"failed_transactions"`
	DeadLetteredTransactions int64 `json:"dead_lettered_transactions"`

	// Performance metrics (nanoseconds)
	TotalLatency   int64 `json:"total_latency"`
	AverageLatency int64 `json:"average_latency"`
	P95Latency     int64 `json:"p95_latency"`
	P99Latency     int64 `json:"p99_latency"`
	MaxLatency     int64 `json:"max_latency"`

	// Resource metrics
	ActiveTransactions int64 `json:"active_transactions"`
	QueuedRetries      int64 `json:"queued_retries"`
	QueuedDeadLetters  int64 `json:"queued_dead_letters"`

	// Error tracking
	ValidationErrors  int64 `json:"validation_errors"`
	PreparationErrors int64 `json:"preparation_errors"`
	CommitErrors      int64 `json:"commit_errors"`
	TimeoutErrors     int64 `json:"timeout_errors"`
}

// NewCrossPairTransactionCoordinator creates a new transaction coordinator
func NewCrossPairTransactionCoordinator(
	logger *zap.Logger,
	xaManager *transaction.XATransactionManager,
	balanceService crosspair.BalanceService,
	matchingEngines map[string]crosspair.MatchingEngine,
	tradeStore crosspair.CrossPairTradeStore,
	config *CoordinatorConfig,
) (*CrossPairTransactionCoordinator, error) {
	if config == nil {
		config = DefaultCoordinatorConfig()
	}

	coordinator := &CrossPairTransactionCoordinator{
		logger:          logger.Named("crosspair-txn-coordinator"),
		xaManager:       xaManager,
		balanceService:  balanceService,
		matchingEngines: matchingEngines,
		tradeStore:      tradeStore,
		config:          config,
		resources:       make(map[string]CrossPairResource),
		transactions:    make(map[uuid.UUID]*CrossPairTransactionContext),
		retryQueue:      make(chan *CrossPairTransactionContext, config.MaxConcurrentTxns),
		deadLetterQueue: make(chan *CrossPairTransactionContext, config.MaxConcurrentTxns),
		shutdown:        make(chan struct{}),
		metrics:         &CoordinatorMetrics{},
	}

	// Initialize transaction pool for performance
	coordinator.pool = NewTransactionPool(config.PreallocatedPools)

	// Initialize worker pool
	workerPool, err := NewWorkerPool(config.WorkerPoolSize, coordinator.processTransaction)
	if err != nil {
		return nil, fmt.Errorf("failed to create worker pool: %w", err)
	}
	coordinator.workerPool = workerPool

	// Initialize audit logger if enabled
	if config.AuditEnabled {
		auditLogger, err := NewTransactionAuditLogger(logger, config)
		if err != nil {
			return nil, fmt.Errorf("failed to create audit logger: %w", err)
		}
		coordinator.auditLogger = auditLogger
	}

	return coordinator, nil
}

// RegisterResource registers a resource with the coordinator
func (c *CrossPairTransactionCoordinator) RegisterResource(resource CrossPairResource) error {
	c.resourcesMu.Lock()
	defer c.resourcesMu.Unlock()

	name := resource.GetResourceName()
	if _, exists := c.resources[name]; exists {
		return fmt.Errorf("resource %s already registered", name)
	}

	c.resources[name] = resource
	c.logger.Info("registered transaction resource", zap.String("name", name))

	return nil
}

// BeginTransaction starts a new cross-pair transaction
func (c *CrossPairTransactionCoordinator) BeginTransaction(
	ctx context.Context,
	txnType CrossPairTransactionType,
	userID uuid.UUID,
	order *crosspair.CrossPairOrder,
) (*CrossPairTransactionContext, error) {
	startTime := time.Now()

	// Generate transaction ID and XID
	txnID := uuid.New()
	xid := c.generateXID(txnID)
	traceID := c.generateTraceID(ctx, txnID)

	// Create transaction context from pool
	txnCtx := c.pool.Get()
	txnCtx.ID = txnID
	txnCtx.XID = xid
	txnCtx.TraceID = traceID
	txnCtx.Type = txnType
	txnCtx.UserID = userID
	txnCtx.CreatedAt = startTime
	txnCtx.UpdatedAt = startTime
	txnCtx.TimeoutAt = startTime.Add(c.config.DefaultTimeout)
	txnCtx.Order = order
	txnCtx.resourceStates = make(map[string]ResourceState)
	txnCtx.RollbackData = make(map[string]interface{})
	txnCtx.completedChan = make(chan struct{})
	txnCtx.Metrics = &TransactionMetrics{
		StartedAt:        startTime,
		ResourcePrepTime: make(map[string]time.Duration),
	}

	// Set initial state
	atomic.StoreInt64(&txnCtx.state, int64(StateInitialized))

	// Register transaction
	c.transactionsMu.Lock()
	c.transactions[txnID] = txnCtx
	c.transactionsMu.Unlock()

	// Update metrics
	atomic.AddInt64(&c.metrics.TotalTransactions, 1)
	atomic.AddInt64(&c.metrics.ActiveTransactions, 1)

	// Audit log
	c.auditTransaction(txnCtx, "TRANSACTION_BEGUN", nil)

	c.logger.Info("transaction begun",
		zap.String("txn_id", txnID.String()),
		zap.String("trace_id", traceID),
		zap.String("type", string(txnType)),
		zap.String("user_id", userID.String()))

	return txnCtx, nil
}

// CommitTransaction commits a transaction using two-phase commit
func (c *CrossPairTransactionCoordinator) CommitTransaction(
	ctx context.Context,
	txnCtx *CrossPairTransactionContext,
) error {
	return c.processTransactionCommit(ctx, txnCtx)
}

// AbortTransaction aborts a transaction with rollback
func (c *CrossPairTransactionCoordinator) AbortTransaction(
	ctx context.Context,
	txnCtx *CrossPairTransactionContext,
	reason string,
) error {
	return c.processTransactionAbort(ctx, txnCtx, reason)
}

// GetTransaction retrieves a transaction by ID
func (c *CrossPairTransactionCoordinator) GetTransaction(txnID uuid.UUID) (*CrossPairTransactionContext, error) {
	c.transactionsMu.RLock()
	defer c.transactionsMu.RUnlock()

	txnCtx, exists := c.transactions[txnID]
	if !exists {
		return nil, fmt.Errorf("transaction %s not found", txnID.String())
	}

	return txnCtx, nil
}

// GetMetrics returns current coordinator metrics
func (c *CrossPairTransactionCoordinator) GetMetrics() *CoordinatorMetrics {
	// Create a copy to avoid race conditions
	return &CoordinatorMetrics{
		TotalTransactions:        atomic.LoadInt64(&c.metrics.TotalTransactions),
		CommittedTransactions:    atomic.LoadInt64(&c.metrics.CommittedTransactions),
		AbortedTransactions:      atomic.LoadInt64(&c.metrics.AbortedTransactions),
		FailedTransactions:       atomic.LoadInt64(&c.metrics.FailedTransactions),
		DeadLetteredTransactions: atomic.LoadInt64(&c.metrics.DeadLetteredTransactions),
		TotalLatency:             atomic.LoadInt64(&c.metrics.TotalLatency),
		AverageLatency:           atomic.LoadInt64(&c.metrics.AverageLatency),
		P95Latency:               atomic.LoadInt64(&c.metrics.P95Latency),
		P99Latency:               atomic.LoadInt64(&c.metrics.P99Latency),
		MaxLatency:               atomic.LoadInt64(&c.metrics.MaxLatency),
		ActiveTransactions:       atomic.LoadInt64(&c.metrics.ActiveTransactions),
		QueuedRetries:            atomic.LoadInt64(&c.metrics.QueuedRetries),
		QueuedDeadLetters:        atomic.LoadInt64(&c.metrics.QueuedDeadLetters),
		ValidationErrors:         atomic.LoadInt64(&c.metrics.ValidationErrors),
		PreparationErrors:        atomic.LoadInt64(&c.metrics.PreparationErrors),
		CommitErrors:             atomic.LoadInt64(&c.metrics.CommitErrors),
		TimeoutErrors:            atomic.LoadInt64(&c.metrics.TimeoutErrors),
	}
}
