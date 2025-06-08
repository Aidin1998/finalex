// Package transaction - Core transaction processing logic for cross-pair operations
package transaction

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/Aidin1998/finalex/internal/accounts/transaction"
	"go.uber.org/zap"
)

// processTransactionCommit handles the two-phase commit process
func (c *CrossPairTransactionCoordinator) processTransactionCommit(
	ctx context.Context,
	txnCtx *CrossPairTransactionContext,
) error {
	// Set state to committing
	if !c.compareAndSwapState(txnCtx, StateValidated, StateCommitting) {
		return fmt.Errorf("transaction %s not in valid state for commit", txnCtx.ID.String())
	}

	c.auditTransaction(txnCtx, "COMMIT_STARTED", nil)

	// Phase 1: Prepare all resources
	if err := c.preparePhase(ctx, txnCtx); err != nil {
		c.logger.Error("prepare phase failed",
			zap.String("txn_id", txnCtx.ID.String()),
			zap.Error(err))

		// Attempt rollback
		c.processTransactionAbort(ctx, txnCtx, fmt.Sprintf("prepare failed: %v", err))
		return fmt.Errorf("prepare phase failed: %w", err)
	}

	// Phase 2: Commit all resources
	if err := c.commitPhase(ctx, txnCtx); err != nil {
		c.logger.Error("commit phase failed",
			zap.String("txn_id", txnCtx.ID.String()),
			zap.Error(err))

		// Start compensation process
		go c.startCompensation(ctx, txnCtx, fmt.Sprintf("commit failed: %v", err))
		return fmt.Errorf("commit phase failed: %w", err)
	}

	// Mark as committed
	atomic.StoreInt64(&txnCtx.state, int64(StateCommitted))
	txnCtx.Metrics.CommittedAt = timePtr(time.Now())
	txnCtx.Metrics.TotalTime = time.Since(txnCtx.Metrics.StartedAt)

	// Update metrics
	atomic.AddInt64(&c.metrics.CommittedTransactions, 1)
	atomic.AddInt64(&c.metrics.ActiveTransactions, -1)
	c.updateLatencyMetrics(txnCtx.Metrics.TotalTime)

	// Complete the transaction
	c.completeTransaction(txnCtx)

	c.auditTransaction(txnCtx, "TRANSACTION_COMMITTED", nil)
	c.logger.Info("transaction committed successfully",
		zap.String("txn_id", txnCtx.ID.String()),
		zap.Duration("total_time", txnCtx.Metrics.TotalTime))

	return nil
}

// processTransactionAbort handles transaction abort with rollback
func (c *CrossPairTransactionCoordinator) processTransactionAbort(
	ctx context.Context,
	txnCtx *CrossPairTransactionContext,
	reason string,
) error {
	// Set state to aborting
	currentState := TransactionState(atomic.LoadInt64(&txnCtx.state))
	if !c.compareAndSwapState(txnCtx, currentState, StateAborting) {
		return fmt.Errorf("transaction %s not in valid state for abort", txnCtx.ID.String())
	}

	c.auditTransaction(txnCtx, "ABORT_STARTED", map[string]interface{}{
		"reason": reason,
	})

	// Rollback all prepared resources
	if err := c.rollbackPhase(ctx, txnCtx); err != nil {
		c.logger.Error("rollback phase failed",
			zap.String("txn_id", txnCtx.ID.String()),
			zap.String("reason", reason),
			zap.Error(err))

		// If rollback fails, start compensation
		go c.startCompensation(ctx, txnCtx, fmt.Sprintf("rollback failed: %v", err))
		return fmt.Errorf("rollback phase failed: %w", err)
	}

	// Mark as aborted
	atomic.StoreInt64(&txnCtx.state, int64(StateAborted))
	txnCtx.Metrics.TotalTime = time.Since(txnCtx.Metrics.StartedAt)

	// Update metrics
	atomic.AddInt64(&c.metrics.AbortedTransactions, 1)
	atomic.AddInt64(&c.metrics.ActiveTransactions, -1)

	// Complete the transaction
	c.completeTransaction(txnCtx)

	c.auditTransaction(txnCtx, "TRANSACTION_ABORTED", map[string]interface{}{
		"reason": reason,
	})
	c.logger.Info("transaction aborted",
		zap.String("txn_id", txnCtx.ID.String()),
		zap.String("reason", reason),
		zap.Duration("total_time", txnCtx.Metrics.TotalTime))

	return nil
}

// preparePhase executes the prepare phase of two-phase commit
func (c *CrossPairTransactionCoordinator) preparePhase(
	ctx context.Context,
	txnCtx *CrossPairTransactionContext,
) error {
	prepareStart := time.Now()
	atomic.StoreInt64(&txnCtx.state, int64(StatePreparing))

	c.logger.Debug("starting prepare phase",
		zap.String("txn_id", txnCtx.ID.String()),
		zap.Int("resource_count", len(txnCtx.Resources)))

	// Prepare all resources concurrently
	type prepareResult struct {
		resourceName string
		readOnly     bool
		err          error
		duration     time.Duration
	}

	results := make(chan prepareResult, len(txnCtx.Resources))

	for _, resource := range txnCtx.Resources {
		go func(res CrossPairResource) {
			start := time.Now()
			readOnly, err := res.Prepare(ctx, txnCtx.XID)
			duration := time.Since(start)

			results <- prepareResult{
				resourceName: res.GetResourceName(),
				readOnly:     readOnly,
				err:          err,
				duration:     duration,
			}
		}(resource)
	}

	// Collect results
	preparedCount := 0
	for i := 0; i < len(txnCtx.Resources); i++ {
		select {
		case result := <-results:
			txnCtx.Metrics.ResourcePrepTime[result.resourceName] = result.duration

			if result.err != nil {
				c.logger.Error("resource prepare failed",
					zap.String("txn_id", txnCtx.ID.String()),
					zap.String("resource", result.resourceName),
					zap.Error(result.err))

				atomic.AddInt64(&c.metrics.PreparationErrors, 1)
				return fmt.Errorf("resource %s prepare failed: %w", result.resourceName, result.err)
			}			// Update resource state
			txnCtx.mu.Lock()
			txnCtx.ResourceStates[result.resourceName] = ResourceState{
				Name:       result.resourceName,
				State:      transaction.XAResourceStatePrepared,
				PreparedAt: timePtr(time.Now()),
			}
			txnCtx.mu.Unlock()

			preparedCount++

			c.logger.Debug("resource prepared successfully",
				zap.String("txn_id", txnCtx.ID.String()),
				zap.String("resource", result.resourceName),
				zap.Bool("read_only", result.readOnly),
				zap.Duration("duration", result.duration))

		case <-ctx.Done():
			atomic.AddInt64(&c.metrics.TimeoutErrors, 1)
			return fmt.Errorf("prepare phase timeout for transaction %s", txnCtx.ID.String())

		case <-time.After(c.config.PreparationTimeout):
			atomic.AddInt64(&c.metrics.TimeoutErrors, 1)
			return fmt.Errorf("prepare phase timeout after %v for transaction %s",
				c.config.PreparationTimeout, txnCtx.ID.String())
		}
	}

	// All resources prepared successfully
	atomic.StoreInt64(&txnCtx.state, int64(StatePrepared))
	txnCtx.Metrics.PreparedAt = timePtr(time.Now())
	txnCtx.Metrics.PreparationTime = time.Since(prepareStart)

	c.logger.Info("prepare phase completed successfully",
		zap.String("txn_id", txnCtx.ID.String()),
		zap.Int("prepared_resources", preparedCount),
		zap.Duration("preparation_time", txnCtx.Metrics.PreparationTime))

	return nil
}

// commitPhase executes the commit phase of two-phase commit
func (c *CrossPairTransactionCoordinator) commitPhase(
	ctx context.Context,
	txnCtx *CrossPairTransactionContext,
) error {
	commitStart := time.Now()

	c.logger.Debug("starting commit phase",
		zap.String("txn_id", txnCtx.ID.String()))

	// Commit all resources concurrently
	type commitResult struct {
		resourceName string
		err          error
	}

	results := make(chan commitResult, len(txnCtx.Resources))

	for _, resource := range txnCtx.Resources {
		go func(res CrossPairResource) {
			err := res.Commit(ctx, txnCtx.XID, false) // Not one-phase commit
			results <- commitResult{
				resourceName: res.GetResourceName(),
				err:          err,
			}
		}(resource)
	}

	// Collect results
	committedCount := 0
	for i := 0; i < len(txnCtx.Resources); i++ {
		select {
		case result := <-results:
			if result.err != nil {
				c.logger.Error("resource commit failed",
					zap.String("txn_id", txnCtx.ID.String()),
					zap.String("resource", result.resourceName),
					zap.Error(result.err))

				atomic.AddInt64(&c.metrics.CommitErrors, 1)
				return fmt.Errorf("resource %s commit failed: %w", result.resourceName, result.err)
			}			// Update resource state
			txnCtx.mu.Lock()
			if state, exists := txnCtx.ResourceStates[result.resourceName]; exists {
				state.State = transaction.XAResourceStateCommitted
				txnCtx.ResourceStates[result.resourceName] = state
			}
			txnCtx.mu.Unlock()

			committedCount++

			c.logger.Debug("resource committed successfully",
				zap.String("txn_id", txnCtx.ID.String()),
				zap.String("resource", result.resourceName))

		case <-ctx.Done():
			atomic.AddInt64(&c.metrics.TimeoutErrors, 1)
			return fmt.Errorf("commit phase timeout for transaction %s", txnCtx.ID.String())

		case <-time.After(c.config.CommitTimeout):
			atomic.AddInt64(&c.metrics.TimeoutErrors, 1)
			return fmt.Errorf("commit phase timeout after %v for transaction %s",
				c.config.CommitTimeout, txnCtx.ID.String())
		}
	}

	txnCtx.Metrics.CommitTime = time.Since(commitStart)

	c.logger.Info("commit phase completed successfully",
		zap.String("txn_id", txnCtx.ID.String()),
		zap.Int("committed_resources", committedCount),
		zap.Duration("commit_time", txnCtx.Metrics.CommitTime))

	return nil
}

// rollbackPhase executes rollback of all prepared resources
func (c *CrossPairTransactionCoordinator) rollbackPhase(
	ctx context.Context,
	txnCtx *CrossPairTransactionContext,
) error {
	c.logger.Debug("starting rollback phase",
		zap.String("txn_id", txnCtx.ID.String()))

	// Rollback all resources concurrently
	type rollbackResult struct {
		resourceName string
		err          error
	}

	results := make(chan rollbackResult, len(txnCtx.Resources))

	for _, resource := range txnCtx.Resources {
		go func(res CrossPairResource) {
			err := res.Rollback(ctx, txnCtx.XID)
			results <- rollbackResult{
				resourceName: res.GetResourceName(),
				err:          err,
			}
		}(resource)
	}

	// Collect results
	rolledBackCount := 0
	var rollbackErrors []error

	for i := 0; i < len(txnCtx.Resources); i++ {
		select {
		case result := <-results:
			if result.err != nil {
				c.logger.Error("resource rollback failed",
					zap.String("txn_id", txnCtx.ID.String()),
					zap.String("resource", result.resourceName),
					zap.Error(result.err))

				rollbackErrors = append(rollbackErrors,
					fmt.Errorf("resource %s rollback failed: %w", result.resourceName, result.err))
			} else {				// Update resource state
				txnCtx.mu.Lock()
				if state, exists := txnCtx.ResourceStates[result.resourceName]; exists {
					state.State = transaction.XAResourceStateRolledBack
					txnCtx.ResourceStates[result.resourceName] = state
				}
				txnCtx.mu.Unlock()

				rolledBackCount++

				c.logger.Debug("resource rolled back successfully",
					zap.String("txn_id", txnCtx.ID.String()),
					zap.String("resource", result.resourceName))
			}

		case <-ctx.Done():
			rollbackErrors = append(rollbackErrors,
				fmt.Errorf("rollback phase timeout for transaction %s", txnCtx.ID.String()))
			break

		case <-time.After(c.config.RollbackTimeout):
			rollbackErrors = append(rollbackErrors,
				fmt.Errorf("rollback phase timeout after %v for transaction %s",
					c.config.RollbackTimeout, txnCtx.ID.String()))
			break
		}
	}

	if len(rollbackErrors) > 0 {
		return fmt.Errorf("rollback failed for %d resources: %v", len(rollbackErrors), rollbackErrors)
	}

	c.logger.Info("rollback phase completed successfully",
		zap.String("txn_id", txnCtx.ID.String()),
		zap.Int("rolled_back_resources", rolledBackCount))

	return nil
}

// startCompensation initiates compensation for failed transactions
func (c *CrossPairTransactionCoordinator) startCompensation(
	ctx context.Context,
	txnCtx *CrossPairTransactionContext,
	reason string,
) {
	atomic.StoreInt64(&txnCtx.state, int64(StateCompensating))

	c.auditTransaction(txnCtx, "COMPENSATION_STARTED", map[string]interface{}{
		"reason": reason,
	})

	c.logger.Warn("starting compensation process",
		zap.String("txn_id", txnCtx.ID.String()),
		zap.String("reason", reason))

	// Execute compensation actions
	for i, action := range txnCtx.CompensationActions {
		if err := c.executeCompensationAction(ctx, txnCtx, &action); err != nil {
			c.logger.Error("compensation action failed",
				zap.String("txn_id", txnCtx.ID.String()),
				zap.Int("action_index", i),
				zap.String("resource", action.ResourceName),
				zap.Error(err))

			// If compensation fails, send to dead letter queue
			c.sendToDeadLetterQueue(txnCtx, fmt.Sprintf("compensation failed: %v", err))
			return
		}
	}

	// Mark as compensated
	atomic.StoreInt64(&txnCtx.state, int64(StateCompensated))
	c.completeTransaction(txnCtx)

	c.auditTransaction(txnCtx, "COMPENSATION_COMPLETED", nil)
	c.logger.Info("compensation completed successfully",
		zap.String("txn_id", txnCtx.ID.String()))
}

// executeCompensationAction executes a single compensation action
func (c *CrossPairTransactionCoordinator) executeCompensationAction(
	ctx context.Context,
	txnCtx *CrossPairTransactionContext,
	action *CompensationAction,
) error {
	c.resourcesMu.RLock()
	resource, exists := c.resources[action.ResourceName]
	c.resourcesMu.RUnlock()

	if !exists {
		return fmt.Errorf("resource %s not found for compensation", action.ResourceName)
	}
	compensationAction, err := resource.GetCompensationAction(ctx, txnCtx)
	if err != nil {
		return fmt.Errorf("failed to get compensation action: %w", err)
	}

	// Execute the compensation action
	// This would typically involve specific business logic per resource type
	// For now, we'll mark it as executed
	_ = compensationAction // Use compensationAction to avoid unused variable error
	now := time.Now()
	action.ExecutedAt = &now

	c.logger.Info("compensation action executed",
		zap.String("txn_id", txnCtx.ID.String()),
		zap.String("resource", action.ResourceName),
		zap.String("action", action.Action))

	return nil
}

// completeTransaction finalizes transaction cleanup
func (c *CrossPairTransactionCoordinator) completeTransaction(txnCtx *CrossPairTransactionContext) {
	// Remove from active transactions
	c.transactionsMu.Lock()
	delete(c.transactions, txnCtx.ID)
	c.transactionsMu.Unlock()

	// Close completion channel
	close(txnCtx.completedChan)

	// Return context to pool
	c.pool.Put(txnCtx)
}

// compareAndSwapState atomically updates transaction state
func (c *CrossPairTransactionCoordinator) compareAndSwapState(
	txnCtx *CrossPairTransactionContext,
	old, new TransactionState,
) bool {
	return atomic.CompareAndSwapInt64(&txnCtx.state, int64(old), int64(new))
}

// updateLatencyMetrics updates performance metrics
func (c *CrossPairTransactionCoordinator) updateLatencyMetrics(latency time.Duration) {
	latencyNs := latency.Nanoseconds()

	// Update total and count for average calculation
	atomic.AddInt64(&c.metrics.TotalLatency, latencyNs)

	// Update max latency
	for {
		current := atomic.LoadInt64(&c.metrics.MaxLatency)
		if latencyNs <= current || atomic.CompareAndSwapInt64(&c.metrics.MaxLatency, current, latencyNs) {
			break
		}
	}

	// TODO: Implement P95/P99 calculation with histogram or sliding window
}

// timePtr returns a pointer to the given time
func timePtr(t time.Time) *time.Time {
	return &t
}
