// Package transaction provides middleware and interceptors for distributed transactions
package transaction

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// TransactionMiddleware provides distributed transaction management for HTTP requests
type TransactionMiddleware struct {
	xaManager   *XATransactionManager
	lockManager *DistributedLockManager
	logger      *zap.Logger

	// Configuration
	autoCommit      bool
	timeoutDuration time.Duration
	requiresLocking map[string][]string // endpoint -> resources to lock
}

// NewTransactionMiddleware creates a new transaction middleware
func NewTransactionMiddleware(
	xaManager *XATransactionManager,
	lockManager *DistributedLockManager,
	logger *zap.Logger,
) *TransactionMiddleware {
	return &TransactionMiddleware{
		xaManager:       xaManager,
		lockManager:     lockManager,
		logger:          logger,
		autoCommit:      true,
		timeoutDuration: 30 * time.Second,
		requiresLocking: make(map[string][]string),
	}
}

// WithAutoCommit sets whether transactions should auto-commit
func (tm *TransactionMiddleware) WithAutoCommit(autoCommit bool) *TransactionMiddleware {
	tm.autoCommit = autoCommit
	return tm
}

// WithTimeout sets the transaction timeout
func (tm *TransactionMiddleware) WithTimeout(timeout time.Duration) *TransactionMiddleware {
	tm.timeoutDuration = timeout
	return tm
}

// WithLocking configures which endpoints require distributed locking
func (tm *TransactionMiddleware) WithLocking(endpoint string, resources []string) *TransactionMiddleware {
	tm.requiresLocking[endpoint] = resources
	return tm
}

// TransactionHandler returns a Gin middleware for transaction management
func (tm *TransactionMiddleware) TransactionHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check if this endpoint requires transaction management
		if !tm.requiresTransaction(c) {
			c.Next()
			return
		} // Create transaction context
		ctx := c.Request.Context()

		// Start distributed transaction
		xaTx, err := tm.xaManager.Start(ctx, tm.timeoutDuration)
		if err != nil {
			tm.logger.Error("Failed to begin XA transaction", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to begin transaction",
			})
			c.Abort()
			return
		}

		// Acquire distributed locks if required
		locks := make([]*DistributedLock, 0)
		if resources, exists := tm.requiresLocking[c.FullPath()]; exists {
			for _, resource := range resources {
				lock, err := tm.lockManager.AcquireLock(ctx, resource, xaTx.ID.String(), tm.timeoutDuration)
				if err != nil {
					tm.logger.Error("Failed to acquire distributed lock",
						zap.String("resource", resource),
						zap.Error(err))

					// Release any already acquired locks
					tm.releaseLocks(ctx, locks)

					// Abort transaction
					tm.xaManager.Abort(ctx, xaTx)

					c.JSON(http.StatusConflict, gin.H{
						"error": fmt.Sprintf("Resource %s is currently locked", resource),
					})
					c.Abort()
					return
				}
				locks = append(locks, lock)
			}
		}
		// Add transaction info to context
		c.Set("xa_transaction", xaTx)
		c.Set("xa_transaction_id", xaTx.ID)
		c.Set("distributed_locks", locks)

		// Set up cleanup
		defer func() {
			// Release locks
			tm.releaseLocks(ctx, locks)

			// Handle transaction completion based on response status
			if c.Writer.Status() >= 400 {
				// Error occurred, abort transaction
				if err := tm.xaManager.Abort(ctx, xaTx); err != nil {
					tm.logger.Error("Failed to abort XA transaction",
						zap.String("transaction_id", xaTx.ID.String()),
						zap.Error(err))
				}
			} else if tm.autoCommit {
				// Success and auto-commit enabled, commit transaction
				if err := tm.xaManager.Commit(ctx, xaTx); err != nil {
					tm.logger.Error("Failed to commit XA transaction",
						zap.String("transaction_id", xaTx.ID.String()),
						zap.Error(err))

					// Change response to error
					c.JSON(http.StatusInternalServerError, gin.H{
						"error": "Transaction commit failed",
					})
				}
			}
		}()

		// Continue with request processing
		c.Next()
	}
}

// requiresTransaction determines if the current request requires transaction management
func (tm *TransactionMiddleware) requiresTransaction(c *gin.Context) bool {
	// Only handle write operations
	method := c.Request.Method
	if method == "GET" || method == "HEAD" || method == "OPTIONS" {
		return false
	}

	// Check if path requires transactions
	path := c.FullPath()
	transactionPaths := []string{
		"/trade/order",
		"/fiat/deposit",
		"/fiat/withdraw",
		"/wallet/withdraw",
		"/account/transfer",
	}

	for _, txPath := range transactionPaths {
		if strings.Contains(path, txPath) {
			return true
		}
	}

	return false
}

// releaseLocks releases all distributed locks
func (tm *TransactionMiddleware) releaseLocks(ctx context.Context, locks []*DistributedLock) {
	for _, lock := range locks {
		if err := tm.lockManager.ReleaseLock(ctx, lock.ID); err != nil {
			tm.logger.Error("Failed to release distributed lock",
				zap.String("lock_id", lock.ID),
				zap.String("resource", lock.Resource),
				zap.Error(err))
		}
	}
}

// TransactionInterceptor provides transaction interception for service calls
type TransactionInterceptor struct {
	xaManager *XATransactionManager
	logger    *zap.Logger
}

// NewTransactionInterceptor creates a new transaction interceptor
func NewTransactionInterceptor(xaManager *XATransactionManager, logger *zap.Logger) *TransactionInterceptor {
	return &TransactionInterceptor{
		xaManager: xaManager,
		logger:    logger,
	}
}

// InterceptServiceCall wraps service calls with transaction management
func (ti *TransactionInterceptor) InterceptServiceCall(
	ctx context.Context,
	serviceName string,
	operation string,
	call func(ctx context.Context) error,
) error {
	// Check if context has an active XA transaction
	if xaTx := GetXATransactionFromContext(ctx); xaTx != nil {
		ti.logger.Debug("Service call within XA transaction",
			zap.String("service", serviceName),
			zap.String("operation", operation),
			zap.String("xa_transaction_id", xaTx.ID.String()))

		// Execute call within transaction context
		return call(ctx)
	}

	// No active transaction, execute normally
	return call(ctx)
}

// TransactionRecovery provides transaction recovery and rollback mechanisms
type TransactionRecovery struct {
	db        *gorm.DB
	xaManager *XATransactionManager
	logger    *zap.Logger

	// Recovery configuration
	recoveryInterval time.Duration
	maxRecoveryAge   time.Duration
	ticker           *time.Ticker
	stopChan         chan struct{}
}

// NewTransactionRecovery creates a new transaction recovery manager
func NewTransactionRecovery(
	db *gorm.DB,
	xaManager *XATransactionManager,
	logger *zap.Logger,
) *TransactionRecovery {
	tr := &TransactionRecovery{
		db:               db,
		xaManager:        xaManager,
		logger:           logger,
		recoveryInterval: 5 * time.Minute,
		maxRecoveryAge:   24 * time.Hour,
		stopChan:         make(chan struct{}),
	}

	tr.ticker = time.NewTicker(tr.recoveryInterval)
	go tr.recoveryProcess()

	return tr
}

// recoveryProcess runs periodic transaction recovery
func (tr *TransactionRecovery) recoveryProcess() {
	for {
		select {
		case <-tr.ticker.C:
			tr.recoverTransactions()
		case <-tr.stopChan:
			return
		}
	}
}

// recoverTransactions attempts to recover stuck or failed transactions
func (tr *TransactionRecovery) recoverTransactions() {
	tr.logger.Info("Starting transaction recovery process")

	// TODO: Implement resource registry in XATransactionManager to support recovery
	// The current XATransactionManager doesn't maintain a registry of all resources
	// This functionality should be implemented as part of the XA resource management
	tr.logger.Warn("Resource recovery not yet implemented - requires XA resource registry")

	/*
		// Future implementation would look like:
		// ctx := context.Background()
		// resources := tr.xaManager.GetAllResources()
		// for _, resource := range resources {
		//     xids, err := resource.Recover(ctx, 0)
		//     // ... recovery logic
		// }
	*/
}

// resolveTransaction attempts to resolve a prepared transaction
func (tr *TransactionRecovery) resolveTransaction(ctx context.Context, resource XAResource, xid XID) error {
	// Check transaction state in the transaction manager
	// This is a simplified implementation - in practice, you'd need to
	// coordinate with a transaction log or coordinator

	// For now, we'll try to commit if the transaction is not too old
	// In a real system, you'd check the transaction log or coordinator

	// Try to commit the transaction
	err := resource.Commit(ctx, xid, false)
	if err != nil {
		tr.logger.Warn("Failed to commit during recovery, attempting rollback",
			zap.String("resource", resource.GetResourceName()),
			zap.String("xid", xid.String()),
			zap.Error(err))

		// If commit fails, try to rollback
		if rollbackErr := resource.Rollback(ctx, xid); rollbackErr != nil {
			tr.logger.Error("Failed to rollback during recovery",
				zap.String("resource", resource.GetResourceName()),
				zap.String("xid", xid.String()),
				zap.Error(rollbackErr))
			return rollbackErr
		}
	}

	tr.logger.Info("Successfully resolved transaction during recovery",
		zap.String("resource", resource.GetResourceName()),
		zap.String("xid", xid.String()))

	return nil
}

// Stop stops the recovery process
func (tr *TransactionRecovery) Stop() {
	tr.ticker.Stop()
	close(tr.stopChan)
	tr.logger.Info("Transaction recovery stopped")
}

// Context helpers for transaction management
type contextKey string

const (
	xaTransactionKey   contextKey = "xa_transaction"
	xaTransactionIDKey contextKey = "xa_transaction_id"
)

// WithXATransaction adds an XA transaction to the context
func WithXATransaction(ctx context.Context, xaTx *XATransaction) context.Context {
	return context.WithValue(ctx, xaTransactionKey, xaTx)
}

// GetXATransactionFromContext retrieves an XA transaction from the context
func GetXATransactionFromContext(ctx context.Context) *XATransaction {
	if xaTx, ok := ctx.Value(xaTransactionKey).(*XATransaction); ok {
		return xaTx
	}
	return nil
}

// WithXATransactionID adds an XA transaction ID to the context
func WithXATransactionID(ctx context.Context, txID uuid.UUID) context.Context {
	return context.WithValue(ctx, xaTransactionIDKey, txID)
}

// GetXATransactionIDFromContext retrieves an XA transaction ID from the context
func GetXATransactionIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	if txID, ok := ctx.Value(xaTransactionIDKey).(uuid.UUID); ok {
		return txID, true
	}
	return uuid.UUID{}, false
}

// TransactionTimeout represents a timeout configuration for transactions
type TransactionTimeout struct {
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	CommitTimeout   time.Duration
	RollbackTimeout time.Duration
}

// DefaultTransactionTimeout returns default timeout values
func DefaultTransactionTimeout() TransactionTimeout {
	return TransactionTimeout{
		ReadTimeout:     10 * time.Second,
		WriteTimeout:    30 * time.Second,
		CommitTimeout:   60 * time.Second,
		RollbackTimeout: 30 * time.Second,
	}
}

// TransactionMiddlewareMetrics tracks metrics for transaction middleware
type TransactionMiddlewareMetrics struct {
	TotalRequests         int64
	TransactionalRequests int64
	CommittedTransactions int64
	AbortedTransactions   int64
	LocksAcquired         int64
	LockFailures          int64
	RecoveryAttempts      int64
	RecoverySuccesses     int64
	RecoveryFailures      int64
}

// GetMetrics returns current transaction metrics
func (tm *TransactionMiddleware) GetMetrics() TransactionMiddlewareMetrics {
	// This would typically be implemented with atomic counters
	// For now, return empty metrics
	return TransactionMiddlewareMetrics{}
}
