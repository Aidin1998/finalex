// Package transaction - Retry and dead letter queue management for cross-pair transactions
package transaction

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// RetryStrategy defines how transaction retries should be handled
type RetryStrategy struct {
	MaxRetries        int           `json:"max_retries"`
	BaseDelay         time.Duration `json:"base_delay"`
	MaxDelay          time.Duration `json:"max_delay"`
	BackoffMultiplier float64       `json:"backoff_multiplier"`
	JitterEnabled     bool          `json:"jitter_enabled"`
	RetryableErrors   []string      `json:"retryable_errors"`
}

// DefaultRetryStrategy returns a default retry strategy
func DefaultRetryStrategy() *RetryStrategy {
	return &RetryStrategy{
		MaxRetries:        3,
		BaseDelay:         100 * time.Millisecond,
		MaxDelay:          5 * time.Second,
		BackoffMultiplier: 2.0,
		JitterEnabled:     true,
		RetryableErrors: []string{
			"TIMEOUT",
			"TEMPORARY_FAILURE",
			"RESOURCE_UNAVAILABLE",
			"NETWORK_ERROR",
		},
	}
}

// RetryManager handles transaction retry logic
type RetryManager struct {
	coordinator *CrossPairTransactionCoordinator
	strategy    *RetryStrategy
	logger      *zap.Logger

	// Retry queue processing
	retryQueue     chan *CrossPairTransactionContext
	retryWorkers   int
	stopRetry      chan struct{}
	retryWaitGroup sync.WaitGroup
}

// NewRetryManager creates a new retry manager
func NewRetryManager(
	coordinator *CrossPairTransactionCoordinator,
	strategy *RetryStrategy,
	logger *zap.Logger,
	workers int,
) *RetryManager {
	if strategy == nil {
		strategy = DefaultRetryStrategy()
	}

	rm := &RetryManager{
		coordinator:  coordinator,
		strategy:     strategy,
		logger:       logger.Named("retry-manager"),
		retryQueue:   make(chan *CrossPairTransactionContext, 1000),
		retryWorkers: workers,
		stopRetry:    make(chan struct{}),
	}

	// Start retry workers
	for i := 0; i < workers; i++ {
		rm.retryWaitGroup.Add(1)
		go rm.retryWorker(i)
	}

	return rm
}

// ScheduleRetry schedules a transaction for retry
func (rm *RetryManager) ScheduleRetry(
	ctx context.Context,
	txnCtx *CrossPairTransactionContext,
	err error,
) error {
	attempts := atomic.LoadInt64(&txnCtx.attempts)

	// Check if we've exceeded max retries
	if int(attempts) >= rm.strategy.MaxRetries {
		rm.logger.Info("transaction exceeded max retries, sending to dead letter queue",
			zap.String("txn_id", txnCtx.ID.String()),
			zap.Int64("attempts", attempts),
			zap.Error(err))

		return rm.coordinator.sendToDeadLetterQueue(txnCtx,
			fmt.Sprintf("exceeded max retries (%d): %v", attempts, err))
	}

	// Check if error is retryable
	if !rm.isRetryableError(err) {
		rm.logger.Info("transaction error is not retryable, sending to dead letter queue",
			zap.String("txn_id", txnCtx.ID.String()),
			zap.Error(err))

		return rm.coordinator.sendToDeadLetterQueue(txnCtx,
			fmt.Sprintf("non-retryable error: %v", err))
	}

	// Calculate retry delay
	delay := rm.calculateRetryDelay(int(attempts))

	rm.logger.Info("scheduling transaction retry",
		zap.String("txn_id", txnCtx.ID.String()),
		zap.Int64("attempt", attempts+1),
		zap.Duration("delay", delay),
		zap.Error(err))

	// Schedule retry after delay
	go func() {
		select {
		case <-time.After(delay):
			atomic.AddInt64(&txnCtx.attempts, 1)
			atomic.StoreInt64(&txnCtx.lastAttempt, time.Now().UnixNano())

			select {
			case rm.retryQueue <- txnCtx:
				atomic.AddInt64(&rm.coordinator.metrics.QueuedRetries, 1)
			case <-rm.stopRetry:
				return
			default:
				// Retry queue is full, send to dead letter queue
				rm.coordinator.sendToDeadLetterQueue(txnCtx, "retry queue full")
			}

		case <-rm.stopRetry:
			return
		}
	}()

	return nil
}

// retryWorker processes retry queue
func (rm *RetryManager) retryWorker(workerID int) {
	defer rm.retryWaitGroup.Done()

	rm.logger.Info("retry worker started", zap.Int("worker_id", workerID))

	for {
		select {
		case txnCtx := <-rm.retryQueue:
			atomic.AddInt64(&rm.coordinator.metrics.QueuedRetries, -1)
			rm.processRetry(txnCtx)

		case <-rm.stopRetry:
			rm.logger.Info("retry worker stopping", zap.Int("worker_id", workerID))
			return
		}
	}
}

// processRetry processes a single retry attempt
func (rm *RetryManager) processRetry(txnCtx *CrossPairTransactionContext) {
	rm.logger.Debug("processing transaction retry",
		zap.String("txn_id", txnCtx.ID.String()),
		zap.Int64("attempt", atomic.LoadInt64(&txnCtx.attempts)))

	// Reset transaction state for retry
	atomic.StoreInt64(&txnCtx.state, int64(StateInitialized))
	txnCtx.UpdatedAt = time.Now()

	// Submit to worker pool for processing
	if err := rm.coordinator.workerPool.Submit(txnCtx); err != nil {
		rm.logger.Error("failed to submit retry to worker pool",
			zap.String("txn_id", txnCtx.ID.String()),
			zap.Error(err))

		// Send to dead letter queue if worker pool is full
		rm.coordinator.sendToDeadLetterQueue(txnCtx, fmt.Sprintf("worker pool full: %v", err))
	}
}

// calculateRetryDelay calculates the delay for retry based on attempt number
func (rm *RetryManager) calculateRetryDelay(attempt int) time.Duration {
	delay := float64(rm.strategy.BaseDelay) * math.Pow(rm.strategy.BackoffMultiplier, float64(attempt))

	if delay > float64(rm.strategy.MaxDelay) {
		delay = float64(rm.strategy.MaxDelay)
	}

	// Add jitter if enabled
	if rm.strategy.JitterEnabled {
		jitter := delay * 0.1 * (2*rand.Float64() - 1) // ±10% jitter
		delay += jitter
	}

	return time.Duration(delay)
}

// isRetryableError checks if an error is retryable
func (rm *RetryManager) isRetryableError(err error) bool {
	errStr := err.Error()

	for _, retryableErr := range rm.strategy.RetryableErrors {
		if contains(errStr, retryableErr) {
			return true
		}
	}

	return false
}

// Stop stops the retry manager
func (rm *RetryManager) Stop() {
	close(rm.stopRetry)
	rm.retryWaitGroup.Wait()
}

// DeadLetterQueue manages failed transactions that cannot be retried
type DeadLetterQueue struct {
	coordinator *CrossPairTransactionCoordinator
	logger      *zap.Logger

	// Dead letter queue processing
	deadLetterQueue chan *CrossPairTransactionContext
	dlqWorkers      int
	stopDLQ         chan struct{}
	dlqWaitGroup    sync.WaitGroup

	// Configuration
	maxAge          time.Duration
	retryAfter      time.Duration
	cleanupInterval time.Duration

	// Storage for dead lettered transactions
	deadLetterStore map[uuid.UUID]*DeadLetterEntry
	storeMutex      sync.RWMutex
}

// DeadLetterEntry represents a dead lettered transaction
type DeadLetterEntry struct {
	TransactionContext *CrossPairTransactionContext `json:"transaction_context"`
	DeadAt             time.Time                    `json:"dead_at"`
	Reason             string                       `json:"reason"`
	RetryCount         int                          `json:"retry_count"`
	NextRetryAt        *time.Time                   `json:"next_retry_at,omitempty"`
	LastError          string                       `json:"last_error"`
}

// NewDeadLetterQueue creates a new dead letter queue
func NewDeadLetterQueue(
	coordinator *CrossPairTransactionCoordinator,
	logger *zap.Logger,
	workers int,
) *DeadLetterQueue {
	dlq := &DeadLetterQueue{
		coordinator:     coordinator,
		logger:          logger.Named("dead-letter-queue"),
		deadLetterQueue: make(chan *CrossPairTransactionContext, 1000),
		dlqWorkers:      workers,
		stopDLQ:         make(chan struct{}),
		maxAge:          coordinator.config.DeadLetterMaxAge,
		retryAfter:      coordinator.config.DeadLetterRetryAfter,
		cleanupInterval: 1 * time.Hour,
		deadLetterStore: make(map[uuid.UUID]*DeadLetterEntry),
	}

	// Start DLQ workers
	for i := 0; i < workers; i++ {
		dlq.dlqWaitGroup.Add(1)
		go dlq.dlqWorker(i)
	}

	// Start cleanup goroutine
	go dlq.cleanupExpiredEntries()

	return dlq
}

// sendToDeadLetterQueue sends a transaction to the dead letter queue
func (c *CrossPairTransactionCoordinator) sendToDeadLetterQueue(
	txnCtx *CrossPairTransactionContext,
	reason string,
) error {
	c.logger.Warn("sending transaction to dead letter queue",
		zap.String("txn_id", txnCtx.ID.String()),
		zap.String("reason", reason))

	// Set transaction state
	atomic.StoreInt64(&txnCtx.state, int64(StateDeadLettered))

	// Create dead letter info
	txnCtx.DeadLetterInfo = &DeadLetterInfo{
		Reason:     reason,
		RetryCount: int(atomic.LoadInt64(&txnCtx.attempts)),
		LastError:  reason,
		DeadAt:     time.Now(),
	}

	// If dead letter queue is enabled, send to DLQ
	if c.config.DeadLetterEnabled {
		select {
		case c.deadLetterQueue <- txnCtx:
			atomic.AddInt64(&c.metrics.QueuedDeadLetters, 1)
			atomic.AddInt64(&c.metrics.DeadLetteredTransactions, 1)
		default:
			c.logger.Error("dead letter queue is full",
				zap.String("txn_id", txnCtx.ID.String()))
			return fmt.Errorf("dead letter queue full for transaction %s", txnCtx.ID.String())
		}
	}

	c.auditTransaction(txnCtx, "TRANSACTION_DEAD_LETTERED", map[string]interface{}{
		"reason": reason,
	})

	return nil
}

// dlqWorker processes dead letter queue
func (dlq *DeadLetterQueue) dlqWorker(workerID int) {
	defer dlq.dlqWaitGroup.Done()

	dlq.logger.Info("dead letter queue worker started", zap.Int("worker_id", workerID))

	for {
		select {
		case txnCtx := <-dlq.deadLetterQueue:
			atomic.AddInt64(&dlq.coordinator.metrics.QueuedDeadLetters, -1)
			dlq.processDeadLetter(txnCtx)

		case <-dlq.stopDLQ:
			dlq.logger.Info("dead letter queue worker stopping", zap.Int("worker_id", workerID))
			return
		}
	}
}

// processDeadLetter processes a dead lettered transaction
func (dlq *DeadLetterQueue) processDeadLetter(txnCtx *CrossPairTransactionContext) {
	dlq.logger.Info("processing dead lettered transaction",
		zap.String("txn_id", txnCtx.ID.String()),
		zap.String("reason", txnCtx.DeadLetterInfo.Reason))

	// Store in dead letter store
	entry := &DeadLetterEntry{
		TransactionContext: txnCtx,
		DeadAt:             txnCtx.DeadLetterInfo.DeadAt,
		Reason:             txnCtx.DeadLetterInfo.Reason,
		RetryCount:         txnCtx.DeadLetterInfo.RetryCount,
		LastError:          txnCtx.DeadLetterInfo.LastError,
	}

	// Set next retry time if configured
	if dlq.retryAfter > 0 {
		nextRetry := time.Now().Add(dlq.retryAfter)
		entry.NextRetryAt = &nextRetry
		txnCtx.DeadLetterInfo.NextRetryAt = &nextRetry
	}

	dlq.storeMutex.Lock()
	dlq.deadLetterStore[txnCtx.ID] = entry
	dlq.storeMutex.Unlock()

	// TODO: Persist to external storage (database, file, etc.)
	// This would depend on the specific requirements and infrastructure

	dlq.logger.Info("transaction stored in dead letter queue",
		zap.String("txn_id", txnCtx.ID.String()),
		zap.Time("next_retry_at", *entry.NextRetryAt))
}

// cleanupExpiredEntries removes expired dead letter entries
func (dlq *DeadLetterQueue) cleanupExpiredEntries() {
	ticker := time.NewTicker(dlq.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			dlq.performCleanup()
		case <-dlq.stopDLQ:
			return
		}
	}
}

// performCleanup removes expired entries from dead letter store
func (dlq *DeadLetterQueue) performCleanup() {
	now := time.Now()
	expiredCount := 0

	dlq.storeMutex.Lock()
	defer dlq.storeMutex.Unlock()

	for txnID, entry := range dlq.deadLetterStore {
		if now.Sub(entry.DeadAt) > dlq.maxAge {
			delete(dlq.deadLetterStore, txnID)
			expiredCount++
		}
	}

	if expiredCount > 0 {
		dlq.logger.Info("cleaned up expired dead letter entries",
			zap.Int("expired_count", expiredCount))
	}
}

// GetDeadLetterEntry retrieves a dead letter entry by transaction ID
func (dlq *DeadLetterQueue) GetDeadLetterEntry(txnID uuid.UUID) (*DeadLetterEntry, error) {
	dlq.storeMutex.RLock()
	defer dlq.storeMutex.RUnlock()

	entry, exists := dlq.deadLetterStore[txnID]
	if !exists {
		return nil, fmt.Errorf("dead letter entry not found for transaction %s", txnID.String())
	}

	return entry, nil
}

// ListDeadLetterEntries returns all dead letter entries
func (dlq *DeadLetterQueue) ListDeadLetterEntries() []*DeadLetterEntry {
	dlq.storeMutex.RLock()
	defer dlq.storeMutex.RUnlock()

	entries := make([]*DeadLetterEntry, 0, len(dlq.deadLetterStore))
	for _, entry := range dlq.deadLetterStore {
		entries = append(entries, entry)
	}

	return entries
}

// RetryDeadLetter attempts to retry a dead lettered transaction
func (dlq *DeadLetterQueue) RetryDeadLetter(txnID uuid.UUID) error {
	dlq.storeMutex.Lock()
	entry, exists := dlq.deadLetterStore[txnID]
	if !exists {
		dlq.storeMutex.Unlock()
		return fmt.Errorf("dead letter entry not found for transaction %s", txnID.String())
	}

	// Remove from dead letter store
	delete(dlq.deadLetterStore, txnID)
	dlq.storeMutex.Unlock()

	// Reset transaction for retry
	txnCtx := entry.TransactionContext
	atomic.StoreInt64(&txnCtx.state, int64(StateInitialized))
	txnCtx.UpdatedAt = time.Now()
	txnCtx.DeadLetterInfo = nil

	// Submit to worker pool
	if err := dlq.coordinator.workerPool.Submit(txnCtx); err != nil {
		return fmt.Errorf("failed to submit dead letter retry: %w", err)
	}

	dlq.logger.Info("dead lettered transaction resubmitted for retry",
		zap.String("txn_id", txnID.String()))

	return nil
}

// Stop stops the dead letter queue
func (dlq *DeadLetterQueue) Stop() {
	close(dlq.stopDLQ)
	dlq.dlqWaitGroup.Wait()
}

// contains checks if a string contains a substring (case-insensitive)
func contains(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
