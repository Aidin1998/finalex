// Package transaction - Audit logging and worker pool for cross-pair transactions
package transaction

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Aidin1998/finalex/internal/accounts/transaction"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// TransactionAuditLogger provides atomic transaction logging with trace ID support
type TransactionAuditLogger struct {
	logger   *zap.Logger
	config   *CoordinatorConfig
	mu       sync.RWMutex
	file     *os.File
	logChan  chan *AuditLogEntry
	stopChan chan struct{}
	wg       sync.WaitGroup
}

// AuditLogEntry represents a single audit log entry
type AuditLogEntry struct {
	Timestamp     time.Time              `json:"timestamp"`
	TraceID       string                 `json:"trace_id"`
	TransactionID uuid.UUID              `json:"transaction_id"`
	UserID        uuid.UUID              `json:"user_id,omitempty"`
	Event         string                 `json:"event"`
	State         string                 `json:"state,omitempty"`
	Data          map[string]interface{} `json:"data,omitempty"`
	Duration      *time.Duration         `json:"duration,omitempty"`
	Error         string                 `json:"error,omitempty"`
	Resources     []string               `json:"resources,omitempty"`
	Metrics       *TransactionMetrics    `json:"metrics,omitempty"`
}

// NewTransactionAuditLogger creates a new audit logger
func NewTransactionAuditLogger(logger *zap.Logger, config *CoordinatorConfig) (*TransactionAuditLogger, error) {
	auditLogger := &TransactionAuditLogger{
		logger:   logger.Named("audit"),
		config:   config,
		logChan:  make(chan *AuditLogEntry, 1000),
		stopChan: make(chan struct{}),
	}

	// Open audit log file if path is configured
	if config.AuditLogPath != "" {
		file, err := os.OpenFile(config.AuditLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return nil, fmt.Errorf("failed to open audit log file: %w", err)
		}
		auditLogger.file = file
	}

	// Start audit log processor
	auditLogger.wg.Add(1)
	go auditLogger.processAuditLogs()

	return auditLogger, nil
}

// LogTransaction logs a transaction event
func (al *TransactionAuditLogger) LogTransaction(
	txnCtx *CrossPairTransactionContext,
	event string,
	data map[string]interface{},
) {
	if !al.config.AuditEnabled {
		return
	}

	entry := &AuditLogEntry{
		Timestamp:     time.Now(),
		TraceID:       txnCtx.TraceID,
		TransactionID: txnCtx.ID,
		UserID:        txnCtx.UserID,
		Event:         event,
		State:         TransactionState(atomic.LoadInt64(&txnCtx.state)).String(),
		Data:          data,
	}

	// Add metrics if transaction is completed
	if txnCtx.Metrics != nil && txnCtx.Metrics.CompletedAt != nil {
		entry.Metrics = txnCtx.Metrics
		entry.Duration = &txnCtx.Metrics.TotalTime
	}

	// Extract resource names
	if len(txnCtx.Resources) > 0 {
		entry.Resources = make([]string, len(txnCtx.Resources))
		for i, resource := range txnCtx.Resources {
			entry.Resources[i] = resource.GetResourceName()
		}
	}

	select {
	case al.logChan <- entry:
	case <-al.stopChan:
	default:
		// Log channel is full, log warning
		al.logger.Warn("audit log channel full, dropping entry",
			zap.String("txn_id", txnCtx.ID.String()),
			zap.String("event", event))
	}
}

// processAuditLogs processes audit log entries
func (al *TransactionAuditLogger) processAuditLogs() {
	defer al.wg.Done()

	for {
		select {
		case entry := <-al.logChan:
			al.writeAuditEntry(entry)
		case <-al.stopChan:
			// Process remaining entries
			for {
				select {
				case entry := <-al.logChan:
					al.writeAuditEntry(entry)
				default:
					return
				}
			}
		}
	}
}

// writeAuditEntry writes an audit entry to both file and structured logger
func (al *TransactionAuditLogger) writeAuditEntry(entry *AuditLogEntry) {
	// Write to structured logger
	fields := []zap.Field{
		zap.String("trace_id", entry.TraceID),
		zap.String("transaction_id", entry.TransactionID.String()),
		zap.String("event", entry.Event),
		zap.String("state", entry.State),
	}

	if entry.UserID != uuid.Nil {
		fields = append(fields, zap.String("user_id", entry.UserID.String()))
	}

	if entry.Duration != nil {
		fields = append(fields, zap.Duration("duration", *entry.Duration))
	}

	if entry.Error != "" {
		fields = append(fields, zap.String("error", entry.Error))
	}

	if len(entry.Resources) > 0 {
		fields = append(fields, zap.Strings("resources", entry.Resources))
	}

	if entry.Data != nil {
		fields = append(fields, zap.Any("data", entry.Data))
	}

	al.logger.Info("transaction audit", fields...)

	// Write to audit file if configured
	if al.file != nil {
		al.writeToFile(entry)
	}
}

// writeToFile writes audit entry to file
func (al *TransactionAuditLogger) writeToFile(entry *AuditLogEntry) {
	al.mu.Lock()
	defer al.mu.Unlock()

	jsonData, err := json.Marshal(entry)
	if err != nil {
		al.logger.Error("failed to marshal audit entry", zap.Error(err))
		return
	}

	if _, err := al.file.Write(append(jsonData, '\n')); err != nil {
		al.logger.Error("failed to write audit entry to file", zap.Error(err))
	}
}

// Close closes the audit logger
func (al *TransactionAuditLogger) Close() error {
	close(al.stopChan)
	al.wg.Wait()

	if al.file != nil {
		return al.file.Close()
	}

	return nil
}

// auditTransaction is a helper method for the coordinator to log transaction events
func (c *CrossPairTransactionCoordinator) auditTransaction(
	txnCtx *CrossPairTransactionContext,
	event string,
	data map[string]interface{},
) {
	if c.auditLogger != nil {
		c.auditLogger.LogTransaction(txnCtx, event, data)
	}
}

// WorkerPool manages a pool of workers for processing transactions
type WorkerPool struct {
	workers    int
	jobQueue   chan *CrossPairTransactionContext
	workerQuit chan struct{}
	processor  TransactionProcessor
	wg         sync.WaitGroup
	logger     *zap.Logger
}

// TransactionProcessor defines the interface for processing transactions
type TransactionProcessor func(ctx context.Context, txnCtx *CrossPairTransactionContext) error

// NewWorkerPool creates a new worker pool
func NewWorkerPool(workers int, processor TransactionProcessor) (*WorkerPool, error) {
	if workers <= 0 {
		return nil, fmt.Errorf("worker count must be positive, got %d", workers)
	}

	wp := &WorkerPool{
		workers:    workers,
		jobQueue:   make(chan *CrossPairTransactionContext, workers*2),
		workerQuit: make(chan struct{}),
		processor:  processor,
		logger:     zap.L().Named("worker-pool"),
	}

	// Start workers
	for i := 0; i < workers; i++ {
		wp.wg.Add(1)
		go wp.worker(i)
	}

	return wp, nil
}

// Submit submits a transaction for processing
func (wp *WorkerPool) Submit(txnCtx *CrossPairTransactionContext) error {
	select {
	case wp.jobQueue <- txnCtx:
		return nil
	default:
		return fmt.Errorf("worker pool queue is full")
	}
}

// worker processes transactions from the job queue
func (wp *WorkerPool) worker(workerID int) {
	defer wp.wg.Done()

	wp.logger.Debug("worker started", zap.Int("worker_id", workerID))

	for {
		select {
		case txnCtx := <-wp.jobQueue:
			wp.processJob(workerID, txnCtx)
		case <-wp.workerQuit:
			wp.logger.Debug("worker stopping", zap.Int("worker_id", workerID))
			return
		}
	}
}

// processJob processes a single transaction
func (wp *WorkerPool) processJob(workerID int, txnCtx *CrossPairTransactionContext) {
	startTime := time.Now()

	wp.logger.Debug("processing transaction",
		zap.Int("worker_id", workerID),
		zap.String("txn_id", txnCtx.ID.String()))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := wp.processor(ctx, txnCtx); err != nil {
		wp.logger.Error("transaction processing failed",
			zap.Int("worker_id", workerID),
			zap.String("txn_id", txnCtx.ID.String()),
			zap.Error(err))
	}

	processingTime := time.Since(startTime)
	wp.logger.Debug("transaction processing completed",
		zap.Int("worker_id", workerID),
		zap.String("txn_id", txnCtx.ID.String()),
		zap.Duration("processing_time", processingTime))
}

// Stop stops the worker pool
func (wp *WorkerPool) Stop() {
	close(wp.workerQuit)
	wp.wg.Wait()
}

// processTransaction is the main transaction processor for the coordinator
func (c *CrossPairTransactionCoordinator) processTransaction(
	ctx context.Context,
	txnCtx *CrossPairTransactionContext,
) error {
	// Check if transaction has timed out
	if time.Now().After(txnCtx.TimeoutAt) {
		return c.processTransactionAbort(ctx, txnCtx, "transaction timeout")
	}

	// Process based on transaction type
	switch txnCtx.Type {
	case TransactionTypeTrade:
		return c.processTradeTransaction(ctx, txnCtx)
	case TransactionTypeWithdrawal:
		return c.processWithdrawalTransaction(ctx, txnCtx)
	case TransactionTypeSettlement:
		return c.processSettlementTransaction(ctx, txnCtx)
	case TransactionTypeTransfer:
		return c.processTransferTransaction(ctx, txnCtx)
	default:
		return fmt.Errorf("unknown transaction type: %s", txnCtx.Type)
	}
}

// processTradeTransaction processes a trade transaction
func (c *CrossPairTransactionCoordinator) processTradeTransaction(
	ctx context.Context,
	txnCtx *CrossPairTransactionContext,
) error {
	// Validate transaction
	if err := c.validateTransaction(ctx, txnCtx); err != nil {
		return fmt.Errorf("trade validation failed: %w", err)
	}

	// Execute two-phase commit
	return c.processTransactionCommit(ctx, txnCtx)
}

// processWithdrawalTransaction processes a withdrawal transaction
func (c *CrossPairTransactionCoordinator) processWithdrawalTransaction(
	ctx context.Context,
	txnCtx *CrossPairTransactionContext,
) error {
	// Validate withdrawal
	if err := c.validateTransaction(ctx, txnCtx); err != nil {
		return fmt.Errorf("withdrawal validation failed: %w", err)
	}

	// Execute two-phase commit
	return c.processTransactionCommit(ctx, txnCtx)
}

// processSettlementTransaction processes a settlement transaction
func (c *CrossPairTransactionCoordinator) processSettlementTransaction(
	ctx context.Context,
	txnCtx *CrossPairTransactionContext,
) error {
	// Validate settlement
	if err := c.validateTransaction(ctx, txnCtx); err != nil {
		return fmt.Errorf("settlement validation failed: %w", err)
	}

	// Execute two-phase commit
	return c.processTransactionCommit(ctx, txnCtx)
}

// processTransferTransaction processes a transfer transaction
func (c *CrossPairTransactionCoordinator) processTransferTransaction(
	ctx context.Context,
	txnCtx *CrossPairTransactionContext,
) error {
	// Validate transfer
	if err := c.validateTransaction(ctx, txnCtx); err != nil {
		return fmt.Errorf("transfer validation failed: %w", err)
	}

	// Execute two-phase commit
	return c.processTransactionCommit(ctx, txnCtx)
}

// validateTransaction validates a transaction before execution
func (c *CrossPairTransactionCoordinator) validateTransaction(
	ctx context.Context,
	txnCtx *CrossPairTransactionContext,
) error {
	atomic.StoreInt64(&txnCtx.state, int64(StateValidating))

	c.auditTransaction(txnCtx, "VALIDATION_STARTED", nil)

	// Validate with all registered resources
	for _, resource := range txnCtx.Resources {
		if err := resource.ValidateTransaction(ctx, txnCtx); err != nil {
			atomic.AddInt64(&c.metrics.ValidationErrors, 1)
			c.auditTransaction(txnCtx, "VALIDATION_FAILED", map[string]interface{}{
				"resource": resource.GetResourceName(),
				"error":    err.Error(),
			})
			return fmt.Errorf("validation failed for resource %s: %w", resource.GetResourceName(), err)
		}
	}

	atomic.StoreInt64(&txnCtx.state, int64(StateValidated))
	txnCtx.Metrics.ValidatedAt = timePtr(time.Now())
	txnCtx.Metrics.ValidationTime = time.Since(txnCtx.Metrics.StartedAt)

	c.auditTransaction(txnCtx, "VALIDATION_COMPLETED", nil)

	return nil
}

// TransactionPool manages a pool of transaction contexts for performance optimization
type TransactionPool struct {
	pool sync.Pool
}

// NewTransactionPool creates a new transaction pool
func NewTransactionPool(prealloc int) *TransactionPool {
	tp := &TransactionPool{
		pool: sync.Pool{
			New: func() interface{} {
				return &CrossPairTransactionContext{}
			},
		},
	}

	// Pre-allocate contexts
	for i := 0; i < prealloc; i++ {
		tp.pool.Put(&CrossPairTransactionContext{})
	}

	return tp
}

// Get gets a transaction context from the pool
func (tp *TransactionPool) Get() *CrossPairTransactionContext {
	ctx := tp.pool.Get().(*CrossPairTransactionContext)

	// Reset context
	*ctx = CrossPairTransactionContext{}

	return ctx
}

// Put returns a transaction context to the pool
func (tp *TransactionPool) Put(ctx *CrossPairTransactionContext) {
	// Clear sensitive data before returning to pool
	ctx.Order = nil
	ctx.Trade = nil
	ctx.Route = nil
	ctx.Resources = nil
	ctx.resourceStates = nil
	ctx.CompensationActions = nil
	ctx.RollbackData = nil
	ctx.Metrics = nil
	ctx.DeadLetterInfo = nil

	tp.pool.Put(ctx)
}

// Helper functions for XID and trace ID generation
func (c *CrossPairTransactionCoordinator) generateXID(txnID uuid.UUID) transaction.XID {
	return transaction.XID{
		FormatID:     1, // OSI CCR format
		GlobalTxnID:  txnID[:],
		BranchQualID: []byte("crosspair"),
	}
}

func (c *CrossPairTransactionCoordinator) generateTraceID(ctx context.Context, txnID uuid.UUID) string {
	// Try to extract trace ID from context first
	if traceID := ctx.Value("trace_id"); traceID != nil {
		if str, ok := traceID.(string); ok && str != "" {
			return str
		}
	}

	// Generate new trace ID based on transaction ID
	return fmt.Sprintf("crosspair-%s", txnID.String()[:8])
}
