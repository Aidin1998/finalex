// Package infrastructure provides distributed transaction management for cross-module operations
package infrastructure

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// DistributedTransactionManager handles XA transactions across multiple modules
type DistributedTransactionManager interface {
	// Transaction lifecycle
	Begin(ctx context.Context, participants []string) (*DistributedTransaction, error)
	Prepare(ctx context.Context, txID uuid.UUID) error
	Commit(ctx context.Context, txID uuid.UUID) error
	Rollback(ctx context.Context, txID uuid.UUID) error

	// Transaction querying
	GetTransaction(ctx context.Context, txID uuid.UUID) (*DistributedTransaction, error)
	ListActiveTransactions(ctx context.Context) ([]*DistributedTransaction, error)

	// Resource management
	RegisterResource(name string, resource XAResource) error
	UnregisterResource(name string) error

	// Lifecycle
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	HealthCheck(ctx context.Context) error
}

// XAResource represents a resource that can participate in XA transactions
type XAResource interface {
	// XA transaction methods
	XAStart(ctx context.Context, xid XID) error
	XAEnd(ctx context.Context, xid XID) error
	XAPrepare(ctx context.Context, xid XID) error
	XACommit(ctx context.Context, xid XID) error
	XARollback(ctx context.Context, xid XID) error

	// Resource info
	GetResourceName() string
	HealthCheck(ctx context.Context) error
}

// XID represents an XA transaction identifier
type XID struct {
	FormatID        int32     `json:"format_id"`
	GlobalTxID      uuid.UUID `json:"global_tx_id"`
	BranchQualifier uuid.UUID `json:"branch_qualifier"`
}

func (xid XID) String() string {
	return fmt.Sprintf("%d:%s:%s", xid.FormatID, xid.GlobalTxID, xid.BranchQualifier)
}

// DistributedTransaction represents a distributed transaction
type DistributedTransaction struct {
	ID           uuid.UUID                     `json:"id"`
	Status       TransactionStatus             `json:"status"`
	Participants []string                      `json:"participants"`
	Branches     map[string]*TransactionBranch `json:"branches"`
	StartTime    time.Time                     `json:"start_time"`
	UpdateTime   time.Time                     `json:"update_time"`
	Timeout      time.Duration                 `json:"timeout"`
	Metadata     map[string]interface{}        `json:"metadata"`
}

// TransactionBranch represents a branch of a distributed transaction
type TransactionBranch struct {
	XID        XID               `json:"xid"`
	Resource   string            `json:"resource"`
	Status     TransactionStatus `json:"status"`
	StartTime  time.Time         `json:"start_time"`
	UpdateTime time.Time         `json:"update_time"`
}

// TransactionStatus represents the status of a transaction or branch
type TransactionStatus string

const (
	StatusActive      TransactionStatus = "active"
	StatusPreparing   TransactionStatus = "preparing"
	StatusPrepared    TransactionStatus = "prepared"
	StatusCommitting  TransactionStatus = "committing"
	StatusCommitted   TransactionStatus = "committed"
	StatusRollingBack TransactionStatus = "rolling_back"
	StatusRolledBack  TransactionStatus = "rolled_back"
	StatusFailed      TransactionStatus = "failed"
	StatusTimeout     TransactionStatus = "timeout"
)

// XATransactionManager implements DistributedTransactionManager using XA protocol
type XATransactionManager struct {
	logger       *zap.Logger
	resources    map[string]XAResource
	transactions map[uuid.UUID]*DistributedTransaction
	mu           sync.RWMutex
	running      bool

	// Configuration
	defaultTimeout  time.Duration
	cleanupInterval time.Duration

	// Cleanup goroutine
	cleanupCtx    context.Context
	cleanupCancel context.CancelFunc
	cleanupWG     sync.WaitGroup
}

// NewXATransactionManager creates a new XA transaction manager
func NewXATransactionManager(logger *zap.Logger, defaultTimeout time.Duration) DistributedTransactionManager {
	return &XATransactionManager{
		logger:          logger,
		resources:       make(map[string]XAResource),
		transactions:    make(map[uuid.UUID]*DistributedTransaction),
		defaultTimeout:  defaultTimeout,
		cleanupInterval: time.Minute * 5,
	}
}

func (tm *XATransactionManager) Start(ctx context.Context) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if tm.running {
		return nil
	}

	tm.running = true
	tm.cleanupCtx, tm.cleanupCancel = context.WithCancel(ctx)

	// Start cleanup goroutine
	tm.cleanupWG.Add(1)
	go tm.cleanupExpiredTransactions()

	tm.logger.Info("XA Transaction Manager started")
	return nil
}

func (tm *XATransactionManager) Stop(ctx context.Context) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if !tm.running {
		return nil
	}

	tm.running = false
	tm.cleanupCancel()
	tm.cleanupWG.Wait()

	// Rollback all active transactions
	for _, tx := range tm.transactions {
		if tx.Status == StatusActive || tx.Status == StatusPreparing {
			tm.rollbackTransaction(ctx, tx)
		}
	}

	tm.logger.Info("XA Transaction Manager stopped")
	return nil
}

func (tm *XATransactionManager) Begin(ctx context.Context, participants []string) (*DistributedTransaction, error) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if !tm.running {
		return nil, fmt.Errorf("transaction manager not running")
	}

	// Validate participants
	for _, participant := range participants {
		if _, exists := tm.resources[participant]; !exists {
			return nil, fmt.Errorf("resource not registered: %s", participant)
		}
	}

	txID := uuid.New()
	tx := &DistributedTransaction{
		ID:           txID,
		Status:       StatusActive,
		Participants: participants,
		Branches:     make(map[string]*TransactionBranch),
		StartTime:    time.Now(),
		UpdateTime:   time.Now(),
		Timeout:      tm.defaultTimeout,
		Metadata:     make(map[string]interface{}),
	}

	// Create branches for each participant
	for _, participant := range participants {
		xid := XID{
			FormatID:        1,
			GlobalTxID:      txID,
			BranchQualifier: uuid.New(),
		}

		branch := &TransactionBranch{
			XID:        xid,
			Resource:   participant,
			Status:     StatusActive,
			StartTime:  time.Now(),
			UpdateTime: time.Now(),
		}

		tx.Branches[participant] = branch

		// Start XA transaction on resource
		resource := tm.resources[participant]
		if err := resource.XAStart(ctx, xid); err != nil {
			tm.logger.Error("Failed to start XA transaction on resource",
				zap.String("resource", participant),
				zap.String("xid", xid.String()),
				zap.Error(err))

			// Rollback already started branches
			tm.rollbackTransaction(ctx, tx)
			return nil, fmt.Errorf("failed to start transaction on resource %s: %w", participant, err)
		}
	}

	tm.transactions[txID] = tx

	tm.logger.Info("Distributed transaction started",
		zap.String("tx_id", txID.String()),
		zap.Strings("participants", participants))

	return tx, nil
}

func (tm *XATransactionManager) Prepare(ctx context.Context, txID uuid.UUID) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	tx, exists := tm.transactions[txID]
	if !exists {
		return fmt.Errorf("transaction not found: %s", txID)
	}

	if tx.Status != StatusActive {
		return fmt.Errorf("transaction not in active state: %s", tx.Status)
	}

	tx.Status = StatusPreparing
	tx.UpdateTime = time.Now()

	// End and prepare all branches
	var preparedBranches []string
	for participant, branch := range tx.Branches {
		resource := tm.resources[participant]

		// End XA transaction
		if err := resource.XAEnd(ctx, branch.XID); err != nil {
			tm.logger.Error("Failed to end XA transaction",
				zap.String("resource", participant),
				zap.String("xid", branch.XID.String()),
				zap.Error(err))

			// Rollback all branches
			tm.rollbackTransaction(ctx, tx)
			return fmt.Errorf("failed to end transaction on resource %s: %w", participant, err)
		}

		// Prepare XA transaction
		if err := resource.XAPrepare(ctx, branch.XID); err != nil {
			tm.logger.Error("Failed to prepare XA transaction",
				zap.String("resource", participant),
				zap.String("xid", branch.XID.String()),
				zap.Error(err))

			// Rollback all branches
			tm.rollbackTransaction(ctx, tx)
			return fmt.Errorf("failed to prepare transaction on resource %s: %w", participant, err)
		}

		branch.Status = StatusPrepared
		branch.UpdateTime = time.Now()
		preparedBranches = append(preparedBranches, participant)
	}

	tx.Status = StatusPrepared
	tx.UpdateTime = time.Now()

	tm.logger.Info("Distributed transaction prepared",
		zap.String("tx_id", txID.String()),
		zap.Strings("prepared_branches", preparedBranches))

	return nil
}

func (tm *XATransactionManager) Commit(ctx context.Context, txID uuid.UUID) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	tx, exists := tm.transactions[txID]
	if !exists {
		return fmt.Errorf("transaction not found: %s", txID)
	}

	if tx.Status != StatusPrepared {
		return fmt.Errorf("transaction not prepared: %s", tx.Status)
	}

	tx.Status = StatusCommitting
	tx.UpdateTime = time.Now()

	// Commit all branches
	var committedBranches []string
	for participant, branch := range tx.Branches {
		resource := tm.resources[participant]

		if err := resource.XACommit(ctx, branch.XID); err != nil {
			tm.logger.Error("Failed to commit XA transaction",
				zap.String("resource", participant),
				zap.String("xid", branch.XID.String()),
				zap.Error(err))

			// Continue committing other branches - this is a critical error
			// but we can't rollback after prepare phase
			branch.Status = StatusFailed
		} else {
			branch.Status = StatusCommitted
			committedBranches = append(committedBranches, participant)
		}

		branch.UpdateTime = time.Now()
	}

	tx.Status = StatusCommitted
	tx.UpdateTime = time.Now()

	tm.logger.Info("Distributed transaction committed",
		zap.String("tx_id", txID.String()),
		zap.Strings("committed_branches", committedBranches))

	// Remove completed transaction after a delay for debugging
	go func() {
		time.Sleep(time.Minute * 10)
		tm.mu.Lock()
		delete(tm.transactions, txID)
		tm.mu.Unlock()
	}()

	return nil
}

func (tm *XATransactionManager) Rollback(ctx context.Context, txID uuid.UUID) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	tx, exists := tm.transactions[txID]
	if !exists {
		return fmt.Errorf("transaction not found: %s", txID)
	}

	return tm.rollbackTransaction(ctx, tx)
}

func (tm *XATransactionManager) rollbackTransaction(ctx context.Context, tx *DistributedTransaction) error {
	if tx.Status == StatusRolledBack || tx.Status == StatusCommitted {
		return nil
	}

	tx.Status = StatusRollingBack
	tx.UpdateTime = time.Now()

	// Rollback all branches
	var rolledBackBranches []string
	for participant, branch := range tx.Branches {
		resource := tm.resources[participant]

		if err := resource.XARollback(ctx, branch.XID); err != nil {
			tm.logger.Error("Failed to rollback XA transaction",
				zap.String("resource", participant),
				zap.String("xid", branch.XID.String()),
				zap.Error(err))
		} else {
			rolledBackBranches = append(rolledBackBranches, participant)
		}

		branch.Status = StatusRolledBack
		branch.UpdateTime = time.Now()
	}

	tx.Status = StatusRolledBack
	tx.UpdateTime = time.Now()

	tm.logger.Info("Distributed transaction rolled back",
		zap.String("tx_id", tx.ID.String()),
		zap.Strings("rolled_back_branches", rolledBackBranches))

	return nil
}

func (tm *XATransactionManager) GetTransaction(ctx context.Context, txID uuid.UUID) (*DistributedTransaction, error) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	tx, exists := tm.transactions[txID]
	if !exists {
		return nil, fmt.Errorf("transaction not found: %s", txID)
	}

	return tx, nil
}

func (tm *XATransactionManager) ListActiveTransactions(ctx context.Context) ([]*DistributedTransaction, error) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	var active []*DistributedTransaction
	for _, tx := range tm.transactions {
		if tx.Status == StatusActive || tx.Status == StatusPreparing || tx.Status == StatusPrepared {
			active = append(active, tx)
		}
	}

	return active, nil
}

func (tm *XATransactionManager) RegisterResource(name string, resource XAResource) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if _, exists := tm.resources[name]; exists {
		return fmt.Errorf("resource already registered: %s", name)
	}

	tm.resources[name] = resource

	tm.logger.Info("XA resource registered",
		zap.String("resource", name))

	return nil
}

func (tm *XATransactionManager) UnregisterResource(name string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if _, exists := tm.resources[name]; !exists {
		return fmt.Errorf("resource not found: %s", name)
	}

	delete(tm.resources, name)

	tm.logger.Info("XA resource unregistered",
		zap.String("resource", name))

	return nil
}

func (tm *XATransactionManager) HealthCheck(ctx context.Context) error {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	if !tm.running {
		return fmt.Errorf("transaction manager not running")
	}

	// Check health of all registered resources
	for name, resource := range tm.resources {
		if err := resource.HealthCheck(ctx); err != nil {
			return fmt.Errorf("resource %s health check failed: %w", name, err)
		}
	}

	return nil
}

func (tm *XATransactionManager) cleanupExpiredTransactions() {
	defer tm.cleanupWG.Done()

	ticker := time.NewTicker(tm.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			tm.performCleanup()
		case <-tm.cleanupCtx.Done():
			return
		}
	}
}

func (tm *XATransactionManager) performCleanup() {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	now := time.Now()
	var expiredTxs []uuid.UUID

	for txID, tx := range tm.transactions {
		if now.Sub(tx.StartTime) > tx.Timeout {
			expiredTxs = append(expiredTxs, txID)
		}
	}

	// Rollback expired transactions
	for _, txID := range expiredTxs {
		tx := tm.transactions[txID]
		tx.Status = StatusTimeout
		tm.rollbackTransaction(context.Background(), tx)

		tm.logger.Warn("Transaction expired and rolled back",
			zap.String("tx_id", txID.String()),
			zap.Duration("age", now.Sub(tx.StartTime)))
	}
}
