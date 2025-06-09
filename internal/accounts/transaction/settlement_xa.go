// Package transaction provides XA resource implementations for settlement engine
package transaction

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Aidin1998/finalex/internal/trading/settlement"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// TraceIDKey is the context key for trace ID propagation
const TraceIDKey = "trace_id"

// Settlement metrics for Prometheus time-series database
var (
	// Settlement latency tracking by stage
	settlementLatencyHistogram = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "settlement_xa_latency_seconds",
			Help:    "Latency of settlement XA operations by stage",
			Buckets: prometheus.ExponentialBuckets(0.001, 2, 15), // 1ms to ~16s
		},
		[]string{"stage", "trace_id", "operation_type"},
	)

	// Settlement stage checkpoints counter
	settlementStageCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "settlement_xa_stage_checkpoints_total",
			Help: "Total number of settlement stage checkpoints recorded",
		},
		[]string{"stage", "operation_type", "status"},
	)

	// Settlement operation duration gauge for real-time monitoring
	settlementOperationDuration = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "settlement_xa_operation_duration_seconds",
			Help: "Current duration of settlement operations",
		},
		[]string{"stage", "trace_id"},
	)

	// Settlement throughput gauge
	settlementThroughput = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "settlement_xa_throughput_ops_per_second",
			Help: "Settlement operations throughput per second",
		},
		[]string{"operation_type"},
	)
)

// SettlementXAResource implements XA interface for settlement operations
type SettlementXAResource struct {
	settlementEngine *settlement.SettlementEngine
	db               *gorm.DB
	logger           *zap.Logger

	// Transaction state tracking
	pendingTransactions map[string]*SettlementTransaction
	mu                  sync.RWMutex
}

// SettlementTransaction represents a settlement transaction in XA context
type SettlementTransaction struct {
	XID              XID
	Operations       []SettlementOperation
	CompensationData map[string]interface{}
	DBTransaction    *gorm.DB
	State            string
	CreatedAt        time.Time
}

// SettlementOperation represents an operation within a settlement transaction
type SettlementOperation struct {
	Type      string                 // "capture_trade", "net_positions", "clear_settle"
	Data      map[string]interface{} // Operation-specific data
	Reference string                 // Reference ID for the operation
}

// NewSettlementXAResource creates a new settlement XA resource
func NewSettlementXAResource(settlementEngine *settlement.SettlementEngine, db *gorm.DB, logger *zap.Logger) *SettlementXAResource {
	return &SettlementXAResource{
		settlementEngine:    settlementEngine,
		db:                  db,
		logger:              logger,
		pendingTransactions: make(map[string]*SettlementTransaction),
	}
}

// GetResourceName returns the name of this XA resource
func (sxa *SettlementXAResource) GetResourceName() string {
	return "SettlementEngine"
}

// Start begins a new XA transaction branch
func (sxa *SettlementXAResource) Start(ctx context.Context, xid XID) error {
	recordLatencyCheckpoint(ctx, sxa.logger, "settlement_start", map[string]interface{}{"xid": xid})

	sxa.mu.Lock()
	defer sxa.mu.Unlock()

	xidStr := xid.String()
	if _, exists := sxa.pendingTransactions[xidStr]; exists {
		return fmt.Errorf("transaction already started: %s", xidStr)
	}

	// Begin database transaction
	dbTx := sxa.db.WithContext(ctx).Begin()
	if dbTx.Error != nil {
		return fmt.Errorf("failed to begin database transaction: %w", dbTx.Error)
	}

	txn := &SettlementTransaction{
		XID:              xid,
		Operations:       make([]SettlementOperation, 0),
		CompensationData: make(map[string]interface{}),
		DBTransaction:    dbTx,
		State:            "active",
		CreatedAt:        time.Now(),
	}

	sxa.pendingTransactions[xidStr] = txn

	sxa.logger.Info("Settlement XA transaction started",
		zap.String("xid", xidStr))

	return nil
}

// End ends the association of the XA resource with the transaction
func (sxa *SettlementXAResource) End(ctx context.Context, xid XID, flags int) error {
	sxa.mu.RLock()
	defer sxa.mu.RUnlock()

	xidStr := xid.String()
	txn, exists := sxa.pendingTransactions[xidStr]
	if !exists {
		return fmt.Errorf("transaction not found: %s", xidStr)
	}

	txn.State = "ended"

	sxa.logger.Debug("Settlement XA transaction ended",
		zap.String("xid", xidStr),
		zap.Int("flags", flags))

	return nil
}

// Prepare implements XA prepare phase
func (sxa *SettlementXAResource) Prepare(ctx context.Context, xid XID) (bool, error) {
	sxa.mu.Lock()
	defer sxa.mu.Unlock()

	xidStr := xid.String()
	txn, exists := sxa.pendingTransactions[xidStr]
	if !exists {
		return false, fmt.Errorf("transaction not found: %s", xidStr)
	}

	// Validate all operations can be committed
	for _, op := range txn.Operations {
		if err := sxa.validateOperation(txn, op); err != nil {
			sxa.logger.Error("Settlement operation validation failed",
				zap.String("xid", xidStr),
				zap.String("operation_type", op.Type),
				zap.Error(err))
			return false, err
		}
	}

	txn.State = "prepared"

	sxa.logger.Info("Settlement XA transaction prepared",
		zap.String("xid", xidStr),
		zap.Int("operations", len(txn.Operations)))

	return true, nil
}

// Commit implements XA commit phase
func (sxa *SettlementXAResource) Commit(ctx context.Context, xid XID, onePhase bool) error {
	recordLatencyCheckpoint(ctx, sxa.logger, "settlement_commit", map[string]interface{}{"xid": xid})

	sxa.mu.Lock()
	defer sxa.mu.Unlock()

	xidStr := xid.String()
	txn, exists := sxa.pendingTransactions[xidStr]
	if !exists {
		return fmt.Errorf("transaction not found: %s", xidStr)
	}

	// For one-phase commit, prepare if not already done
	if onePhase && txn.State == "active" {
		if prepared, err := sxa.prepare(ctx, xid); !prepared || err != nil {
			if err := sxa.rollback(ctx, xid); err != nil {
				sxa.logger.Error("Failed to rollback after failed one-phase prepare",
					zap.String("xid", xidStr),
					zap.Error(err))
			}
			return fmt.Errorf("one-phase prepare failed: %w", err)
		}
	}

	// Commit the database transaction
	if err := txn.DBTransaction.Commit().Error; err != nil {
		sxa.logger.Error("Failed to commit database transaction",
			zap.String("xid", xidStr),
			zap.Error(err))
		return fmt.Errorf("database commit failed: %w", err)
	}

	// Execute settlement operations
	for _, op := range txn.Operations {
		if err := sxa.executeCommitOperation(ctx, op); err != nil {
			sxa.logger.Error("Failed to execute settlement operation",
				zap.String("xid", xidStr),
				zap.String("operation_type", op.Type),
				zap.Error(err))
			// Continue with other operations but log the error
		}
	}

	txn.State = "committed"

	sxa.logger.Info("Settlement XA transaction committed",
		zap.String("xid", xidStr),
		zap.Int("operations", len(txn.Operations)))

	// Clean up
	delete(sxa.pendingTransactions, xidStr)

	return nil
}

// Rollback implements XA rollback phase
func (sxa *SettlementXAResource) Rollback(ctx context.Context, xid XID) error {
	sxa.mu.Lock()
	defer sxa.mu.Unlock()

	return sxa.rollback(ctx, xid)
}

// rollback implements the actual rollback logic
func (sxa *SettlementXAResource) rollback(ctx context.Context, xid XID) error {
	xidStr := xid.String()
	txn, exists := sxa.pendingTransactions[xidStr]
	if !exists {
		// Transaction may have already been cleaned up
		return nil
	}

	// Rollback the database transaction
	if err := txn.DBTransaction.Rollback().Error; err != nil {
		sxa.logger.Error("Failed to rollback database transaction",
			zap.String("xid", xidStr),
			zap.Error(err))
	}

	// Execute compensation operations in reverse order
	for i := len(txn.Operations) - 1; i >= 0; i-- {
		op := txn.Operations[i]
		if err := sxa.executeRollbackOperation(ctx, op, txn.CompensationData); err != nil {
			sxa.logger.Error("Failed to execute settlement rollback operation",
				zap.String("xid", xidStr),
				zap.String("operation_type", op.Type),
				zap.Error(err))
			// Continue with other rollback operations
		}
	}

	txn.State = "aborted"

	sxa.logger.Info("Settlement XA transaction rolled back",
		zap.String("xid", xidStr),
		zap.Int("operations", len(txn.Operations)))

	// Clean up
	delete(sxa.pendingTransactions, xidStr)

	return nil
}

// prepare implements the actual prepare logic (internal use)
func (sxa *SettlementXAResource) prepare(ctx context.Context, xid XID) (bool, error) {
	return sxa.Prepare(ctx, xid)
}

// Forget implements XA forget phase (for heuristic completions)
func (sxa *SettlementXAResource) Forget(ctx context.Context, xid XID) error {
	sxa.mu.Lock()
	defer sxa.mu.Unlock()

	xidStr := xid.String()
	delete(sxa.pendingTransactions, xidStr)

	sxa.logger.Info("Settlement XA transaction forgotten",
		zap.String("xid", xidStr))

	return nil
}

// Recover implements XA recovery phase
func (sxa *SettlementXAResource) Recover(ctx context.Context, flags int) ([]XID, error) {
	sxa.mu.RLock()
	defer sxa.mu.RUnlock()

	var recoveredXIDs []XID
	for _, txn := range sxa.pendingTransactions {
		if txn.State == "prepared" {
			recoveredXIDs = append(recoveredXIDs, txn.XID)
		}
	}

	sxa.logger.Info("Settlement XA recovery found prepared transactions",
		zap.Int("count", len(recoveredXIDs)))

	return recoveredXIDs, nil
}

// CaptureTrade captures a trade for settlement within XA transaction
func (sxa *SettlementXAResource) CaptureTrade(ctx context.Context, xid XID, trade settlement.TradeCapture) error {
	sxa.mu.Lock()
	defer sxa.mu.Unlock()

	xidStr := xid.String()
	txn, exists := sxa.pendingTransactions[xidStr]
	if !exists {
		return fmt.Errorf("transaction not found: %s", xidStr)
	}

	// Store original state for compensation
	compensationKey := fmt.Sprintf("capture_trade_%s", trade.TradeID)
	txn.CompensationData[compensationKey] = map[string]interface{}{
		"trade_id": trade.TradeID,
		"captured": false,
	}

	// Capture the trade
	sxa.settlementEngine.CaptureTrade(trade)

	// Record the operation
	op := SettlementOperation{
		Type: "capture_trade",
		Data: map[string]interface{}{
			"trade_id":   trade.TradeID,
			"user_id":    trade.UserID,
			"symbol":     trade.Symbol,
			"side":       trade.Side,
			"quantity":   trade.Quantity,
			"price":      trade.Price,
			"asset_type": trade.AssetType,
			"matched_at": trade.MatchedAt,
		},
		Reference: trade.TradeID,
	}

	txn.Operations = append(txn.Operations, op)

	sxa.logger.Debug("Trade captured in settlement XA transaction",
		zap.String("xid", xidStr),
		zap.String("trade_id", trade.TradeID))

	return nil
}

// NetPositions performs position netting within XA transaction
func (sxa *SettlementXAResource) NetPositions(ctx context.Context, xid XID) ([]settlement.NetPosition, error) {
	sxa.mu.Lock()
	defer sxa.mu.Unlock()

	xidStr := xid.String()
	txn, exists := sxa.pendingTransactions[xidStr]
	if !exists {
		return nil, fmt.Errorf("transaction not found: %s", xidStr)
	}

	// Store original state for compensation
	compensationKey := "net_positions"
	positions := sxa.settlementEngine.NetPositions()

	txn.CompensationData[compensationKey] = map[string]interface{}{
		"original_positions": positions,
		"netted":             true,
	}

	// Record the operation
	op := SettlementOperation{
		Type: "net_positions",
		Data: map[string]interface{}{
			"position_count": len(positions),
		},
		Reference: "net_positions_" + time.Now().Format("20060102150405"),
	}

	txn.Operations = append(txn.Operations, op)

	sxa.logger.Debug("Positions netted in settlement XA transaction",
		zap.String("xid", xidStr),
		zap.Int("positions", len(positions)))

	return positions, nil
}

// ClearAndSettle performs clearing and settlement within XA transaction
func (sxa *SettlementXAResource) ClearAndSettle(ctx context.Context, xid XID) error {
	sxa.mu.Lock()
	defer sxa.mu.Unlock()

	xidStr := xid.String()
	txn, exists := sxa.pendingTransactions[xidStr]
	if !exists {
		return fmt.Errorf("transaction not found: %s", xidStr)
	}

	// Store original state for compensation
	compensationKey := "clear_settle"
	txn.CompensationData[compensationKey] = map[string]interface{}{
		"settled": false,
	}

	// Record the operation
	op := SettlementOperation{
		Type: "clear_settle",
		Data: map[string]interface{}{
			"timestamp": time.Now(),
		},
		Reference: "clear_settle_" + time.Now().Format("20060102150405"),
	}

	txn.Operations = append(txn.Operations, op)

	sxa.logger.Debug("Clear and settle queued in settlement XA transaction",
		zap.String("xid", xidStr))

	return nil
}

// validateOperation validates that an operation can be committed
func (sxa *SettlementXAResource) validateOperation(txn *SettlementTransaction, op SettlementOperation) error {
	switch op.Type {
	case "capture_trade":
		// Validate trade data is complete
		if op.Data["trade_id"] == nil || op.Data["user_id"] == nil {
			return fmt.Errorf("incomplete trade data")
		}
	case "net_positions":
		// Position netting is always valid
	case "clear_settle":
		// Settlement clearing is always valid
	default:
		return fmt.Errorf("unknown operation type: %s", op.Type)
	}
	return nil
}

// executeCommitOperation executes an operation during commit phase
func (sxa *SettlementXAResource) executeCommitOperation(ctx context.Context, op SettlementOperation) error {
	switch op.Type {
	case "capture_trade":
		// Trade already captured during operation, nothing more to do
		return nil
	case "net_positions":
		// Positions already netted during operation, nothing more to do
		return nil
	case "clear_settle":
		// Execute actual clearing and settlement
		sxa.settlementEngine.ClearAndSettle()
		return nil
	default:
		return fmt.Errorf("unknown operation type: %s", op.Type)
	}
}

// executeRollbackOperation executes compensation for an operation during rollback
func (sxa *SettlementXAResource) executeRollbackOperation(ctx context.Context, op SettlementOperation, compensationData map[string]interface{}) error {
	switch op.Type {
	case "capture_trade":
		// Remove captured trade (this would require extending settlement engine)
		sxa.logger.Warn("Cannot rollback captured trade - settlement engine limitation",
			zap.String("trade_id", op.Reference))
		return nil
	case "net_positions":
		// Reset positions (this would require extending settlement engine)
		sxa.logger.Warn("Cannot rollback netted positions - settlement engine limitation")
		return nil
	case "clear_settle":
		// Cannot rollback settlement
		sxa.logger.Warn("Cannot rollback settlement - this requires manual intervention")
		return nil
	default:
		return fmt.Errorf("unknown operation type: %s", op.Type)
	}
}

// GetPendingTransactionCount returns the number of pending transactions
func (sxa *SettlementXAResource) GetPendingTransactionCount() int {
	sxa.mu.RLock()
	defer sxa.mu.RUnlock()

	return len(sxa.pendingTransactions)
}

// GetTransactionState returns the state of a specific transaction
func (sxa *SettlementXAResource) GetTransactionState(xid XID) (string, bool) {
	sxa.mu.RLock()
	defer sxa.mu.RUnlock()

	txn, exists := sxa.pendingTransactions[xid.String()]
	if !exists {
		return "", false
	}

	return txn.State, true
}

// TraceIDFromContext extracts the trace ID from context, or generates one if missing
func TraceIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v := ctx.Value(TraceIDKey); v != nil {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return uuid.New().String()
}

// recordLatencyCheckpoint records a latency checkpoint with trace ID, stage, and timestamp
func recordLatencyCheckpoint(ctx context.Context, logger *zap.Logger, stage string, extra map[string]interface{}) {
	traceID := TraceIDFromContext(ctx)
	ts := time.Now().UTC()
	fields := []zap.Field{
		zap.String("trace_id", traceID),
		zap.String("stage", stage),
		zap.Time("timestamp", ts),
	}
	for k, v := range extra {
		fields = append(fields, zap.Any(k, v))
	}
	logger.Info("latency_checkpoint", fields...)

	// Write settlement metrics to time-series database (Prometheus)
	writeSettlementMetrics(ctx, stage, traceID, extra)
}

// writeSettlementMetrics writes settlement metrics to Prometheus time-series database
func writeSettlementMetrics(ctx context.Context, stage, traceID string, extra map[string]interface{}) {
	// Extract operation type from extra data, default to "unknown"
	operationType := "unknown"
	if opType, exists := extra["operation_type"]; exists {
		if opTypeStr, ok := opType.(string); ok && opTypeStr != "" {
			operationType = opTypeStr
		}
	}

	// Extract duration if available for latency tracking
	var duration time.Duration
	if d, exists := extra["duration"]; exists {
		switch v := d.(type) {
		case time.Duration:
			duration = v
		case float64:
			duration = time.Duration(v * float64(time.Second))
		case int64:
			duration = time.Duration(v) * time.Nanosecond
		}
	} else if startTime, exists := extra["start_time"]; exists {
		if startTimeVal, ok := startTime.(time.Time); ok {
			duration = time.Since(startTimeVal)
		}
	}

	// Record latency histogram if duration is available
	if duration > 0 {
		settlementLatencyHistogram.WithLabelValues(stage, traceID, operationType).Observe(duration.Seconds())
		settlementOperationDuration.WithLabelValues(stage, traceID).Set(duration.Seconds())
	}

	// Determine status from extra data
	status := "success"
	if statusVal, exists := extra["status"]; exists {
		if statusStr, ok := statusVal.(string); ok && statusStr != "" {
			status = statusStr
		}
	}
	if errorVal, exists := extra["error"]; exists && errorVal != nil {
		status = "error"
	}

	// Record stage checkpoint
	settlementStageCounter.WithLabelValues(stage, operationType, status).Inc()

	// Update throughput metrics if operations count is available
	if opsCount, exists := extra["operations_count"]; exists {
		if opsCountFloat, ok := opsCount.(float64); ok {
			settlementThroughput.WithLabelValues(operationType).Set(opsCountFloat)
		} else if opsCountInt, ok := opsCount.(int64); ok {
			settlementThroughput.WithLabelValues(operationType).Set(float64(opsCountInt))
		}
	}

	// Record additional custom metrics from extra data
	recordCustomSettlementMetrics(stage, traceID, operationType, extra)
}

// recordCustomSettlementMetrics records additional custom metrics from extra data
func recordCustomSettlementMetrics(stage, traceID, operationType string, extra map[string]interface{}) {
	// Record batch size if available
	if batchSize, exists := extra["batch_size"]; exists {
		if batchSizeFloat, ok := batchSize.(float64); ok {
			// Use existing histogram if available, or create a new gauge
			if batchSizeFloat > 0 {
				settlementOperationDuration.WithLabelValues(stage+"_batch_size", traceID).Set(batchSizeFloat)
			}
		}
	}

	// Record settlement amount if available
	if amount, exists := extra["amount"]; exists {
		if amountFloat, ok := amount.(float64); ok {
			settlementOperationDuration.WithLabelValues(stage+"_amount", traceID).Set(amountFloat)
		}
	}

	// Record queue depth if available
	if queueDepth, exists := extra["queue_depth"]; exists {
		if queueDepthFloat, ok := queueDepth.(float64); ok {
			settlementOperationDuration.WithLabelValues(stage+"_queue_depth", traceID).Set(queueDepthFloat)
		}
	}
	// Record error details if available
	if errorMsg, exists := extra["error_message"]; exists {
		if errorMsgStr, ok := errorMsg.(string); ok && errorMsgStr != "" {
			// Record error type for monitoring
			settlementStageCounter.WithLabelValues(stage, operationType, "error_"+errorMsgStr).Inc()
		}
	}
}
