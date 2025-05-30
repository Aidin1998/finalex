// Package transaction implements distributed transaction management with XA protocol and two-phase commit
package transaction

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// XATransactionState represents the state of an XA transaction
type XATransactionState string

const (
	XAStateActive     XATransactionState = "ACTIVE"
	XAStatePreparing  XATransactionState = "PREPARING"
	XAStatePrepared   XATransactionState = "PREPARED"
	XAStateCommitting XATransactionState = "COMMITTING"
	XAStateCommitted  XATransactionState = "COMMITTED"
	XAStateAborting   XATransactionState = "ABORTING"
	XAStateAborted    XATransactionState = "ABORTED"
	XAStateReadonly   XATransactionState = "READONLY"
	XAStateHeurCommit XATransactionState = "HEUR_COMMIT"
	XAStateHeurAbort  XATransactionState = "HEUR_ABORT"
	XAStateUnknown    XATransactionState = "UNKNOWN"
)

// XAResourceState represents the state of an XA resource
type XAResourceState string

const (
	XAResourceStateActive       XAResourceState = "ACTIVE"
	XAResourceStateIdle         XAResourceState = "IDLE"
	XAResourceStatePrepared     XAResourceState = "PREPARED"
	XAResourceStateCommitted    XAResourceState = "COMMITTED"
	XAResourceStateRolledBack   XAResourceState = "ROLLED_BACK"
	XAResourceStateRollbackOnly XAResourceState = "ROLLBACK_ONLY"
)

// XA Error codes
type XAErrorCode int

const (
	XAErrorRMFAIL      XAErrorCode = -3  // Resource manager failure
	XAErrorXAER_NOTA   XAErrorCode = -4  // XID not known by RM
	XAErrorXAER_PROTO  XAErrorCode = -6  // Protocol error
	XAErrorRB_ROLLBACK XAErrorCode = 100 // Transaction was rolled back
)

// XA Flags
type XAFlag int

const (
	XAFlagTMSUCCESS XAFlag = 0x00000000 // Normal termination
	XAFlagTMFAIL    XAFlag = 0x20000000 // Abnormal termination
)

// XAException represents an XA-specific error
type XAException struct {
	ErrorCode XAErrorCode `json:"error_code"`
	Message   string      `json:"message"`
	Cause     error       `json:"cause,omitempty"`
}

func (e *XAException) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("XA Error %d: %s (caused by: %v)", e.ErrorCode, e.Message, e.Cause)
	}
	return fmt.Sprintf("XA Error %d: %s", e.ErrorCode, e.Message)
}

// XAResource represents a resource that can participate in XA transactions
type XAResource interface {
	// Prepare phase - vote to commit or abort
	Prepare(ctx context.Context, xid XID) (bool, error)

	// Commit phase - make changes permanent
	Commit(ctx context.Context, xid XID, onePhase bool) error

	// Rollback phase - undo changes
	Rollback(ctx context.Context, xid XID) error

	// Forget - clean up heuristic completions
	Forget(ctx context.Context, xid XID) error

	// Recover - return prepared transactions
	Recover(ctx context.Context, flags int) ([]XID, error)

	// GetResourceName returns the name of this resource
	GetResourceName() string
}

// XID represents a transaction identifier
type XID struct {
	FormatID     int32  // Format identifier
	GlobalTxnID  []byte // Global transaction identifier
	BranchQualID []byte // Branch qualifier
}

func (x XID) String() string {
	return fmt.Sprintf("XID{fmt=%d,gtxn=%x,bqual=%x}", x.FormatID, x.GlobalTxnID, x.BranchQualID)
}

// XATransaction represents a distributed transaction
type XATransaction struct {
	ID        uuid.UUID
	XID       XID
	State     XATransactionState
	Resources []XAResource
	CreatedAt time.Time
	UpdatedAt time.Time
	TimeoutAt time.Time
	mu        sync.RWMutex
	logger    *zap.Logger

	// Two-phase commit tracking
	preparedResources  map[string]bool
	committedResources map[string]bool
	abortedResources   map[string]bool

	// Compensation data for saga patterns
	compensationData map[string]interface{}
}

// XATransactionManager manages distributed transactions using XA protocol
type XATransactionManager struct {
	logger       *zap.Logger
	transactions map[uuid.UUID]*XATransaction
	mu           sync.RWMutex

	// Configuration
	defaultTimeout time.Duration
	maxRetries     int

	// Recovery and monitoring
	recoveryTicker *time.Ticker
	stopChan       chan struct{}

	// Metrics
	metrics *XAMetrics
}

// XAMetrics tracks transaction manager performance
type XAMetrics struct {
	TotalTransactions     int64
	CommittedTransactions int64
	AbortedTransactions   int64
	HeuristicOutcomes     int64
	RecoveryAttempts      int64
	AverageCommitTime     time.Duration
	mu                    sync.RWMutex
}

// NewXATransactionManager creates a new XA transaction manager
func NewXATransactionManager(logger *zap.Logger, timeout time.Duration) *XATransactionManager {
	manager := &XATransactionManager{
		logger:         logger,
		transactions:   make(map[uuid.UUID]*XATransaction),
		defaultTimeout: timeout,
		maxRetries:     3,
		stopChan:       make(chan struct{}),
		metrics:        &XAMetrics{},
	}

	// Start recovery process
	manager.startRecovery()

	return manager
}

// Start begins a new distributed transaction
func (tm *XATransactionManager) Start(ctx context.Context, timeout time.Duration) (*XATransaction, error) {
	if timeout == 0 {
		timeout = tm.defaultTimeout
	}

	txnID := uuid.New()
	xid := XID{
		FormatID:     1,
		GlobalTxnID:  txnID[:],
		BranchQualID: []byte(fmt.Sprintf("branch-%d", time.Now().UnixNano())),
	}

	txn := &XATransaction{
		ID:                 txnID,
		XID:                xid,
		State:              XAStateActive,
		Resources:          make([]XAResource, 0),
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
		TimeoutAt:          time.Now().Add(timeout),
		logger:             tm.logger,
		preparedResources:  make(map[string]bool),
		committedResources: make(map[string]bool),
		abortedResources:   make(map[string]bool),
		compensationData:   make(map[string]interface{}),
	}

	tm.mu.Lock()
	tm.transactions[txnID] = txn
	tm.mu.Unlock()

	tm.metrics.mu.Lock()
	tm.metrics.TotalTransactions++
	tm.metrics.mu.Unlock()

	tm.logger.Info("Started XA transaction",
		zap.String("transaction_id", txnID.String()),
		zap.String("xid", xid.String()),
		zap.Time("timeout_at", txn.TimeoutAt))

	return txn, nil
}

// Enlist adds a resource to the transaction
func (tm *XATransactionManager) Enlist(txn *XATransaction, resource XAResource) error {
	txn.mu.Lock()
	defer txn.mu.Unlock()

	if txn.State != XAStateActive {
		return fmt.Errorf("cannot enlist resource: transaction not active (state: %s)", txn.State)
	}

	// Check if resource already enlisted
	for _, r := range txn.Resources {
		if r.GetResourceName() == resource.GetResourceName() {
			return fmt.Errorf("resource %s already enlisted", resource.GetResourceName())
		}
	}

	txn.Resources = append(txn.Resources, resource)
	txn.UpdatedAt = time.Now()

	tm.logger.Info("Enlisted resource in transaction",
		zap.String("transaction_id", txn.ID.String()),
		zap.String("resource", resource.GetResourceName()),
		zap.Int("total_resources", len(txn.Resources)))

	return nil
}

// Commit executes two-phase commit protocol
func (tm *XATransactionManager) Commit(ctx context.Context, txn *XATransaction) error {
	start := time.Now()
	defer func() {
		tm.metrics.mu.Lock()
		tm.metrics.AverageCommitTime = time.Since(start)
		tm.metrics.mu.Unlock()
	}()

	txn.mu.Lock()
	defer txn.mu.Unlock()

	if txn.State != XAStateActive {
		return fmt.Errorf("cannot commit: transaction not active (state: %s)", txn.State)
	}

	// Check timeout
	if time.Now().After(txn.TimeoutAt) {
		return tm.abort(ctx, txn, fmt.Errorf("transaction timeout"))
	}

	// Phase 1: Prepare
	if err := tm.prepare(ctx, txn); err != nil {
		return tm.abort(ctx, txn, err)
	}

	// Phase 2: Commit
	return tm.commit(ctx, txn)
}

// prepare implements the prepare phase of 2PC
func (tm *XATransactionManager) prepare(ctx context.Context, txn *XATransaction) error {
	tm.logger.Info("Starting prepare phase",
		zap.String("transaction_id", txn.ID.String()),
		zap.Int("resources", len(txn.Resources)))

	// If no resources, mark as readonly
	if len(txn.Resources) == 0 {
		txn.State = XAStateReadonly
		return nil
	}

	// If only one resource, use one-phase optimization
	if len(txn.Resources) == 1 {
		resource := txn.Resources[0]
		if err := resource.Commit(ctx, txn.XID, true); err != nil {
			tm.logger.Error("One-phase commit failed",
				zap.String("transaction_id", txn.ID.String()),
				zap.String("resource", resource.GetResourceName()),
				zap.Error(err))
			return err
		}
		txn.State = XAStateCommitted
		txn.committedResources[resource.GetResourceName()] = true
		return nil
	}

	// Two-phase commit for multiple resources
	for _, resource := range txn.Resources {
		prepared, err := resource.Prepare(ctx, txn.XID)
		if err != nil {
			tm.logger.Error("Prepare failed",
				zap.String("transaction_id", txn.ID.String()),
				zap.String("resource", resource.GetResourceName()),
				zap.Error(err))
			return err
		}

		if !prepared {
			tm.logger.Warn("Resource voted to abort",
				zap.String("transaction_id", txn.ID.String()),
				zap.String("resource", resource.GetResourceName()))
			return fmt.Errorf("resource %s voted to abort", resource.GetResourceName())
		}

		txn.preparedResources[resource.GetResourceName()] = true
		tm.logger.Debug("Resource prepared",
			zap.String("transaction_id", txn.ID.String()),
			zap.String("resource", resource.GetResourceName()))
	}

	txn.State = XAStatePrepared
	txn.UpdatedAt = time.Now()

	tm.logger.Info("All resources prepared",
		zap.String("transaction_id", txn.ID.String()))

	return nil
}

// commit implements the commit phase of 2PC
func (tm *XATransactionManager) commit(ctx context.Context, txn *XATransaction) error {
	tm.logger.Info("Starting commit phase",
		zap.String("transaction_id", txn.ID.String()))

	// If readonly, nothing to commit
	if txn.State == XAStateReadonly {
		txn.State = XAStateCommitted
		return nil
	}

	// If already committed via one-phase, we're done
	if txn.State == XAStateCommitted {
		tm.metrics.mu.Lock()
		tm.metrics.CommittedTransactions++
		tm.metrics.mu.Unlock()
		return nil
	}

	var commitErrors []error

	for _, resource := range txn.Resources {
		if err := resource.Commit(ctx, txn.XID, false); err != nil {
			tm.logger.Error("Commit failed",
				zap.String("transaction_id", txn.ID.String()),
				zap.String("resource", resource.GetResourceName()),
				zap.Error(err))
			commitErrors = append(commitErrors, err)
		} else {
			txn.committedResources[resource.GetResourceName()] = true
			tm.logger.Debug("Resource committed",
				zap.String("transaction_id", txn.ID.String()),
				zap.String("resource", resource.GetResourceName()))
		}
	}

	if len(commitErrors) > 0 {
		// Heuristic outcome - some resources committed, some failed
		txn.State = XAStateHeurCommit
		tm.metrics.mu.Lock()
		tm.metrics.HeuristicOutcomes++
		tm.metrics.mu.Unlock()

		tm.logger.Error("Heuristic commit outcome",
			zap.String("transaction_id", txn.ID.String()),
			zap.Errors("errors", commitErrors))

		return fmt.Errorf("heuristic commit: %d errors occurred", len(commitErrors))
	}

	txn.State = XAStateCommitted
	txn.UpdatedAt = time.Now()

	tm.metrics.mu.Lock()
	tm.metrics.CommittedTransactions++
	tm.metrics.mu.Unlock()

	tm.logger.Info("Transaction committed",
		zap.String("transaction_id", txn.ID.String()))

	// Clean up transaction after successful commit
	go tm.cleanup(txn)

	return nil
}

// Abort rolls back the transaction
func (tm *XATransactionManager) Abort(ctx context.Context, txn *XATransaction) error {
	txn.mu.Lock()
	defer txn.mu.Unlock()

	return tm.abort(ctx, txn, fmt.Errorf("explicit abort"))
}

// abort implements transaction rollback
func (tm *XATransactionManager) abort(ctx context.Context, txn *XATransaction, reason error) error {
	tm.logger.Info("Aborting transaction",
		zap.String("transaction_id", txn.ID.String()),
		zap.Error(reason))

	var rollbackErrors []error

	for _, resource := range txn.Resources {
		if err := resource.Rollback(ctx, txn.XID); err != nil {
			tm.logger.Error("Rollback failed",
				zap.String("transaction_id", txn.ID.String()),
				zap.String("resource", resource.GetResourceName()),
				zap.Error(err))
			rollbackErrors = append(rollbackErrors, err)
		} else {
			txn.abortedResources[resource.GetResourceName()] = true
			tm.logger.Debug("Resource rolled back",
				zap.String("transaction_id", txn.ID.String()),
				zap.String("resource", resource.GetResourceName()))
		}
	}

	if len(rollbackErrors) > 0 {
		txn.State = XAStateHeurAbort
		tm.metrics.mu.Lock()
		tm.metrics.HeuristicOutcomes++
		tm.metrics.mu.Unlock()

		tm.logger.Error("Heuristic abort outcome",
			zap.String("transaction_id", txn.ID.String()),
			zap.Errors("errors", rollbackErrors))

		return fmt.Errorf("heuristic abort: %d errors occurred", len(rollbackErrors))
	}

	txn.State = XAStateAborted
	txn.UpdatedAt = time.Now()

	tm.metrics.mu.Lock()
	tm.metrics.AbortedTransactions++
	tm.metrics.mu.Unlock()

	tm.logger.Info("Transaction aborted",
		zap.String("transaction_id", txn.ID.String()))

	// Clean up transaction after abort
	go tm.cleanup(txn)

	return nil
}

// cleanup removes completed transactions
func (tm *XATransactionManager) cleanup(txn *XATransaction) {
	// Wait a bit before cleanup to allow any pending operations
	time.Sleep(5 * time.Second)

	tm.mu.Lock()
	delete(tm.transactions, txn.ID)
	tm.mu.Unlock()

	tm.logger.Debug("Transaction cleaned up",
		zap.String("transaction_id", txn.ID.String()))
}

// GetTransaction retrieves a transaction by ID
func (tm *XATransactionManager) GetTransaction(id uuid.UUID) (*XATransaction, bool) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	txn, exists := tm.transactions[id]
	return txn, exists
}

// GetMetrics returns current transaction manager metrics
func (tm *XATransactionManager) GetMetrics() XAMetrics {
	tm.metrics.mu.RLock()
	defer tm.metrics.mu.RUnlock()

	// Create a copy without the mutex to avoid copying locks
	return XAMetrics{
		TotalTransactions:     tm.metrics.TotalTransactions,
		CommittedTransactions: tm.metrics.CommittedTransactions,
		AbortedTransactions:   tm.metrics.AbortedTransactions,
		HeuristicOutcomes:     tm.metrics.HeuristicOutcomes,
		RecoveryAttempts:      tm.metrics.RecoveryAttempts,
		AverageCommitTime:     tm.metrics.AverageCommitTime,
	}
}

// startRecovery begins the recovery process for prepared transactions
func (tm *XATransactionManager) startRecovery() {
	tm.recoveryTicker = time.NewTicker(30 * time.Second)

	go func() {
		for {
			select {
			case <-tm.recoveryTicker.C:
				tm.performRecovery()
			case <-tm.stopChan:
				tm.recoveryTicker.Stop()
				return
			}
		}
	}()
}

// performRecovery checks for and recovers orphaned prepared transactions
func (tm *XATransactionManager) performRecovery() {
	tm.logger.Debug("Starting transaction recovery")

	tm.mu.RLock()
	transactions := make([]*XATransaction, 0, len(tm.transactions))
	for _, txn := range tm.transactions {
		transactions = append(transactions, txn)
	}
	tm.mu.RUnlock()

	for _, txn := range transactions {
		txn.mu.RLock()
		state := txn.State
		timeoutAt := txn.TimeoutAt
		txn.mu.RUnlock()

		// Check for timed out transactions
		if time.Now().After(timeoutAt) && (state == XAStateActive || state == XAStatePrepared) {
			tm.logger.Warn("Found timed out transaction, aborting",
				zap.String("transaction_id", txn.ID.String()),
				zap.String("state", string(state)))

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			if err := tm.Abort(ctx, txn); err != nil {
				tm.logger.Error("Failed to abort timed out transaction",
					zap.String("transaction_id", txn.ID.String()),
					zap.Error(err))
			}
			cancel()
		}
	}

	tm.metrics.mu.Lock()
	tm.metrics.RecoveryAttempts++
	tm.metrics.mu.Unlock()
}

// Stop shuts down the transaction manager
func (tm *XATransactionManager) Stop() {
	close(tm.stopChan)

	// Abort any active transactions
	tm.mu.RLock()
	transactions := make([]*XATransaction, 0, len(tm.transactions))
	for _, txn := range tm.transactions {
		transactions = append(transactions, txn)
	}
	tm.mu.RUnlock()

	for _, txn := range transactions {
		txn.mu.RLock()
		state := txn.State
		txn.mu.RUnlock()

		if state == XAStateActive || state == XAStatePrepared {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			tm.Abort(ctx, txn)
			cancel()
		}
	}

	tm.logger.Info("XA Transaction Manager stopped")
}
