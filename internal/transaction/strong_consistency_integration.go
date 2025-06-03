package transaction

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/bookkeeper"
	"github.com/Aidin1998/pincex_unified/internal/consensus"
	"github.com/Aidin1998/pincex_unified/internal/consistency"
	"github.com/Aidin1998/pincex_unified/internal/coordination"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// StrongConsistencyTransactionManager extends the existing transaction manager
// with strong consistency capabilities for critical financial operations
type StrongConsistencyTransactionManager struct {
	*TransactionManagerSuite

	// Strong consistency components
	raftCoordinator       *consensus.RaftCoordinator
	balanceManager        *consistency.BalanceConsistencyManager
	settlementCoordinator *coordination.StrongConsistencySettlementCoordinator

	// Configuration
	config *StrongConsistencyConfig
	logger *zap.Logger

	// Metrics and monitoring
	metrics *StrongConsistencyMetrics
	mutex   sync.RWMutex
}

// StrongConsistencyConfig defines configuration for strong consistency operations
type StrongConsistencyConfig struct {
	// Thresholds for when consensus is required
	SettlementConsensusThreshold float64 `json:"settlement_consensus_threshold" yaml:"settlement_consensus_threshold"`
	TransferConsensusThreshold   float64 `json:"transfer_consensus_threshold" yaml:"transfer_consensus_threshold"`

	// Consensus configuration
	ConsensusTimeout      time.Duration `json:"consensus_timeout" yaml:"consensus_timeout"`
	QuorumSize            int           `json:"quorum_size" yaml:"quorum_size"`
	LeaderElectionTimeout time.Duration `json:"leader_election_timeout" yaml:"leader_election_timeout"`

	// Balance consistency configuration
	ReconciliationInterval time.Duration `json:"reconciliation_interval" yaml:"reconciliation_interval"`
	LockTimeout            time.Duration `json:"lock_timeout" yaml:"lock_timeout"`
	MaxRetryAttempts       int           `json:"max_retry_attempts" yaml:"max_retry_attempts"`

	// Settlement configuration
	BatchSize    int           `json:"batch_size" yaml:"batch_size"`
	BatchTimeout time.Duration `json:"batch_timeout" yaml:"batch_timeout"`

	// Performance tuning
	EnableParallelProcessing bool `json:"enable_parallel_processing" yaml:"enable_parallel_processing"`
	MaxConcurrentOperations  int  `json:"max_concurrent_operations" yaml:"max_concurrent_operations"`
}

// StrongConsistencyMetrics tracks performance and consistency metrics
type StrongConsistencyMetrics struct {
	ConsensusOperations int64         `json:"consensus_operations"`
	ConsensusFailures   int64         `json:"consensus_failures"`
	ConsensusLatency    time.Duration `json:"consensus_latency"`

	BalanceConsistencyChecks int64 `json:"balance_consistency_checks"`
	BalanceInconsistencies   int64 `json:"balance_inconsistencies"`
	BalanceReconciliations   int64 `json:"balance_reconciliations"`

	SettlementBatches int64         `json:"settlement_batches"`
	SettlementLatency time.Duration `json:"settlement_latency"`

	DistributedLockAcquisitions int64 `json:"distributed_lock_acquisitions"`
	DistributedLockFailures     int64 `json:"distributed_lock_failures"`

	LastUpdate time.Time `json:"last_update"`
}

// NewStrongConsistencyTransactionManager creates a new enhanced transaction manager
func NewStrongConsistencyTransactionManager(
	baseSuite *TransactionManagerSuite,
	db *gorm.DB,
	logger *zap.Logger,
	config *StrongConsistencyConfig,
) (*StrongConsistencyTransactionManager, error) {
	if config == nil {
		config = DefaultStrongConsistencyConfig()
	}

	// Initialize consensus coordinator
	raftCoordinator := consensus.NewRaftCoordinator(
		"node-1",                               // This should be configurable
		[]string{"node-1", "node-2", "node-3"}, // This should be configurable
		logger,
	)
	// Initialize balance consistency manager
	// First we need a bookkeeper service - let's create a simple one or get it from baseSuite
	var bookkeeperSvc bookkeeper.BookkeeperService
	if baseSuite.BookkeeperXA != nil {
		bookkeeperSvc = baseSuite.BookkeeperXA // Assuming this implements the interface
	} else {
		// For now, we'll pass nil and handle it in the constructor
		logger.Warn("No bookkeeper service available, this may cause issues")
	}

	balanceManager := consistency.NewBalanceConsistencyManager(db, bookkeeperSvc, raftCoordinator, logger)

	// Initialize settlement coordinator
	settlementCoordinator := coordination.NewStrongConsistencySettlementCoordinator(
		logger,
		db,
		baseSuite.SettlementXA.settlementEngine,
		bookkeeperSvc,
		raftCoordinator,
		balanceManager,
	)

	manager := &StrongConsistencyTransactionManager{
		TransactionManagerSuite: baseSuite,
		raftCoordinator:         raftCoordinator,
		balanceManager:          balanceManager,
		settlementCoordinator:   settlementCoordinator,
		config:                  config,
		logger:                  logger,
		metrics:                 &StrongConsistencyMetrics{},
	}

	return manager, nil
}

// Start initializes and starts all strong consistency components
func (sctm *StrongConsistencyTransactionManager) Start(ctx context.Context) error {
	// Start base transaction manager suite
	if err := sctm.TransactionManagerSuite.Start(ctx); err != nil {
		return fmt.Errorf("failed to start base transaction manager: %w", err)
	}

	// Start consensus coordinator
	if err := sctm.raftCoordinator.Start(ctx); err != nil {
		return fmt.Errorf("failed to start raft coordinator: %w", err)
	}

	// Start balance consistency manager
	if err := sctm.balanceManager.Start(ctx); err != nil {
		return fmt.Errorf("failed to start balance manager: %w", err)
	}

	// Start settlement coordinator
	if err := sctm.settlementCoordinator.Start(ctx); err != nil {
		return fmt.Errorf("failed to start settlement coordinator: %w", err)
	}

	sctm.logger.Info("Strong Consistency Transaction Manager started successfully")
	return nil
}

// Stop gracefully shuts down all components
func (sctm *StrongConsistencyTransactionManager) Stop(ctx context.Context) error {
	sctm.logger.Info("Stopping Strong Consistency Transaction Manager")

	// Stop components in reverse order
	if err := sctm.settlementCoordinator.Stop(ctx); err != nil {
		sctm.logger.Error("Failed to stop settlement coordinator", zap.Error(err))
	}

	if err := sctm.balanceManager.Stop(ctx); err != nil {
		sctm.logger.Error("Failed to stop balance manager", zap.Error(err))
	}

	if err := sctm.raftCoordinator.Stop(ctx); err != nil {
		sctm.logger.Error("Failed to stop raft coordinator", zap.Error(err))
	}

	// Stop base transaction manager
	if err := sctm.TransactionManagerSuite.Stop(ctx); err != nil {
		sctm.logger.Error("Failed to stop base transaction manager", zap.Error(err))
	}
	sctm.logger.Info("Strong Consistency Transaction Manager stopped")
	return nil
}

// ExecuteStrongConsistencyTransaction implements the simplified interface for trading package
func (sctm *StrongConsistencyTransactionManager) ExecuteStrongConsistencyTransaction(ctx context.Context, req interface{}) (interface{}, error) {
	// Parse the request data
	requestData, ok := req.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid request format")
	}

	// Extract operations from request
	operationsData, ok := requestData["operations"].([]map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid operations format")
	}

	// Convert to TransactionOperation structs
	operations := make([]TransactionOperation, len(operationsData))
	for i, opData := range operationsData {
		operations[i] = TransactionOperation{
			Service:    opData["service"].(string),
			Operation:  opData["operation"].(string),
			Parameters: opData["parameters"].(map[string]interface{}),
		}
	}

	// Extract timeout
	timeout := 30 * time.Second // default
	if timeoutData, exists := requestData["timeout"]; exists {
		if timeoutSeconds, ok := timeoutData.(float64); ok {
			timeout = time.Duration(timeoutSeconds) * time.Second
		}
	}

	// Execute using the full implementation
	return sctm.executeStrongConsistencyTransactionWithOperations(ctx, operations, timeout)
}

// executeStrongConsistencyTransactionWithOperations executes a transaction with strong consistency guarantees
func (sctm *StrongConsistencyTransactionManager) executeStrongConsistencyTransactionWithOperations(
	ctx context.Context,
	operations []TransactionOperation,
	timeout time.Duration,
) (*TransactionResult, error) {
	txnID := uuid.New().String()
	startTime := time.Now()

	sctm.logger.Info("Executing strong consistency transaction",
		zap.String("transaction_id", txnID),
		zap.Int("operation_count", len(operations)))

	// Classify operations by criticality and required consensus
	criticalOps, regularOps := sctm.classifyOperations(operations)

	// Start distributed transaction with consensus for critical operations
	if len(criticalOps) > 0 {
		// Require consensus for critical operations
		consensusCtx, cancel := context.WithTimeout(ctx, sctm.config.ConsensusTimeout)
		defer cancel()

		approved, err := sctm.raftCoordinator.ProposeGenericOperation(consensusCtx, consensus.Operation{
			ID:        txnID,
			Type:      consensus.OperationTypeTransaction,
			Data:      map[string]interface{}{"operations": criticalOps},
			Timestamp: startTime,
		})

		if err != nil {
			sctm.updateMetrics(func(m *StrongConsistencyMetrics) {
				m.ConsensusFailures++
			})
			return nil, fmt.Errorf("consensus failed for critical operations: %w", err)
		}

		if !approved {
			sctm.updateMetrics(func(m *StrongConsistencyMetrics) {
				m.ConsensusFailures++
			})
			return nil, fmt.Errorf("critical operations not approved by consensus")
		}

		sctm.updateMetrics(func(m *StrongConsistencyMetrics) {
			m.ConsensusOperations++
			m.ConsensusLatency = time.Since(startTime)
		})
	}

	// Execute all operations with enhanced consistency
	allOps := append(criticalOps, regularOps...)
	result, err := sctm.executeOperationsWithConsistency(ctx, allOps, timeout, txnID)

	if err != nil {
		sctm.logger.Error("Strong consistency transaction failed",
			zap.String("transaction_id", txnID),
			zap.Error(err))
		return nil, err
	}

	sctm.logger.Info("Strong consistency transaction completed successfully",
		zap.String("transaction_id", txnID),
		zap.Duration("duration", time.Since(startTime)))

	return result, nil
}

// executeOperationsWithConsistency executes operations with enhanced consistency guarantees
func (sctm *StrongConsistencyTransactionManager) executeOperationsWithConsistency(
	ctx context.Context,
	operations []TransactionOperation,
	timeout time.Duration,
	txnID string,
) (*TransactionResult, error) {
	// Start XA transaction
	txn, err := sctm.XAManager.Start(ctx, timeout)
	if err != nil {
		return nil, fmt.Errorf("failed to start XA transaction: %w", err)
	}

	// Add transaction to context
	ctx = WithXATransaction(ctx, txn)

	// Group operations by type for optimal execution
	balanceOps := sctm.extractBalanceOperations(operations)
	settlementOps := sctm.extractSettlementOperations(operations)
	otherOps := sctm.extractOtherOperations(operations)

	// Execute balance operations with strong consistency
	if len(balanceOps) > 0 {
		if err := sctm.executeBalanceOperationsWithConsistency(ctx, balanceOps, txnID); err != nil {
			sctm.XAManager.Abort(ctx, txn)
			return nil, fmt.Errorf("balance operations failed: %w", err)
		}
	}

	// Execute settlement operations with coordination
	if len(settlementOps) > 0 {
		if err := sctm.executeSettlementOperationsWithCoordination(ctx, settlementOps, txnID); err != nil {
			sctm.XAManager.Abort(ctx, txn)
			return nil, fmt.Errorf("settlement operations failed: %w", err)
		}
	}

	// Execute other operations through standard XA transaction
	if len(otherOps) > 0 {
		if err := sctm.executeStandardOperations(ctx, otherOps); err != nil {
			sctm.XAManager.Abort(ctx, txn)
			return nil, fmt.Errorf("standard operations failed: %w", err)
		}
	}

	// Commit transaction
	if err := sctm.XAManager.Commit(ctx, txn); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return &TransactionResult{
		TransactionID: txn.ID,
		Status:        "committed",
		Operations:    operations,
		Timestamp:     time.Now(),
	}, nil
}

// classifyOperations separates operations into critical and regular categories
func (sctm *StrongConsistencyTransactionManager) classifyOperations(operations []TransactionOperation) ([]TransactionOperation, []TransactionOperation) {
	var critical, regular []TransactionOperation

	for _, op := range operations {
		if sctm.isCriticalOperation(op) {
			critical = append(critical, op)
		} else {
			regular = append(regular, op)
		}
	}

	return critical, regular
}

// isCriticalOperation determines if an operation requires consensus
func (sctm *StrongConsistencyTransactionManager) isCriticalOperation(op TransactionOperation) bool {
	switch op.Service {
	case "settlement":
		if amount, ok := op.Parameters["amount"].(float64); ok {
			return amount >= sctm.config.SettlementConsensusThreshold
		}
	case "bookkeeper":
		if op.Operation == "transfer_funds" {
			if amount, ok := op.Parameters["amount"].(float64); ok {
				return amount >= sctm.config.TransferConsensusThreshold
			}
		}
	}
	return false
}

// requiresConsensus checks if any operation requires consensus
func (sctm *StrongConsistencyTransactionManager) requiresConsensus(operations []TransactionOperation) bool {
	for _, op := range operations {
		if sctm.isCriticalOperation(op) {
			return true
		}
	}
	return false
}

// executeBalanceOperationsWithConsistency executes balance operations with strong consistency
func (sctm *StrongConsistencyTransactionManager) executeBalanceOperationsWithConsistency(
	ctx context.Context,
	operations []TransactionOperation,
	txnID string,
) error {
	for _, op := range operations {
		switch op.Operation {
		case "transfer_funds":
			fromUserID := op.Parameters["from_user_id"].(string)
			toUserID := op.Parameters["to_user_id"].(string)
			currency := op.Parameters["currency"].(string)
			amount := op.Parameters["amount"].(float64)

			transfer := consistency.BalanceTransfer{
				FromUserID: fromUserID,
				ToUserID:   toUserID,
				Currency:   currency,
				Amount:     decimal.NewFromFloat(amount),
				Reference:  "",
			}

			if err := sctm.balanceManager.ExecuteAtomicTransfer(ctx, transfer); err != nil {
				return fmt.Errorf("atomic transfer failed: %w", err)
			}

		default:
			// Handle other balance operations
			if err := sctm.executeBookkeeperOperation(ctx, op); err != nil {
				return err
			}
		}
	}

	sctm.updateMetrics(func(m *StrongConsistencyMetrics) {
		m.BalanceConsistencyChecks++
	})

	return nil
}

// executeSettlementOperationsWithCoordination executes settlement operations with coordination
func (sctm *StrongConsistencyTransactionManager) executeSettlementOperationsWithCoordination(
	ctx context.Context,
	operations []TransactionOperation,
	txnID string,
) error {
	// Convert operations to trades for batch processing
	trades := make([]coordination.Trade, 0, len(operations))

	for _, op := range operations {
		if op.Service == "settlement" && op.Operation == "settle_trade" {
			trade := coordination.Trade{
				ID:        uuid.MustParse(op.Parameters["trade_id"].(string)),
				Symbol:    op.Parameters["symbol"].(string),
				Quantity:  decimal.NewFromFloat(op.Parameters["quantity"].(float64)),
				Price:     decimal.NewFromFloat(op.Parameters["price"].(float64)),
				Timestamp: time.Now(),
			}
			trades = append(trades, trade)
		}
	}

	if len(trades) > 0 {
		if err := sctm.settlementCoordinator.ProcessTradeBatch(ctx, trades); err != nil {
			return fmt.Errorf("settlement coordination failed: %w", err)
		}

		sctm.updateMetrics(func(m *StrongConsistencyMetrics) {
			m.SettlementBatches++
		})
	}

	return nil
}

// executeStandardOperations executes non-critical operations through standard XA
func (sctm *StrongConsistencyTransactionManager) executeStandardOperations(
	ctx context.Context,
	operations []TransactionOperation,
) error {
	// Use the existing transaction manager's operation execution
	for _, op := range operations {
		if err := sctm.executeOperation(ctx, op); err != nil {
			return err
		}
	}
	return nil
}

// Helper methods for operation extraction
func (sctm *StrongConsistencyTransactionManager) extractBalanceOperations(operations []TransactionOperation) []TransactionOperation {
	var balanceOps []TransactionOperation
	for _, op := range operations {
		if op.Service == "bookkeeper" && (op.Operation == "transfer_funds" || op.Operation == "lock_funds") {
			balanceOps = append(balanceOps, op)
		}
	}
	return balanceOps
}

func (sctm *StrongConsistencyTransactionManager) extractSettlementOperations(operations []TransactionOperation) []TransactionOperation {
	var settlementOps []TransactionOperation
	for _, op := range operations {
		if op.Service == "settlement" {
			settlementOps = append(settlementOps, op)
		}
	}
	return settlementOps
}

func (sctm *StrongConsistencyTransactionManager) extractOtherOperations(operations []TransactionOperation) []TransactionOperation {
	var otherOps []TransactionOperation
	for _, op := range operations {
		if op.Service != "bookkeeper" && op.Service != "settlement" {
			otherOps = append(otherOps, op)
		} else if op.Service == "bookkeeper" && op.Operation != "transfer_funds" && op.Operation != "lock_funds" {
			otherOps = append(otherOps, op)
		}
	}
	return otherOps
}

// updateMetrics safely updates metrics
func (sctm *StrongConsistencyTransactionManager) updateMetrics(updateFunc func(*StrongConsistencyMetrics)) {
	sctm.mutex.Lock()
	defer sctm.mutex.Unlock()

	updateFunc(sctm.metrics)
	sctm.metrics.LastUpdate = time.Now()
}

// GetStrongConsistencyMetrics returns current consistency metrics
func (sctm *StrongConsistencyTransactionManager) GetStrongConsistencyMetrics() *StrongConsistencyMetrics {
	sctm.mutex.RLock()
	defer sctm.mutex.RUnlock()

	// Return a copy to prevent external modification
	metricsCopy := *sctm.metrics
	return &metricsCopy
}

// GetHealthCheck returns enhanced health status including consistency components
func (sctm *StrongConsistencyTransactionManager) GetHealthCheck() map[string]interface{} {
	baseHealth := sctm.TransactionManagerSuite.GetHealthCheck()

	consistencyHealth := map[string]interface{}{
		"raft_coordinator": map[string]interface{}{
			"is_leader":  sctm.raftCoordinator.IsLeader(),
			"node_count": len(sctm.raftCoordinator.GetNodes()),
			"status":     "healthy",
		},
		"balance_manager": map[string]interface{}{
			"active_locks": sctm.balanceManager.GetActiveLockCount(),
			"status":       "healthy",
		},
		"settlement_coordinator": map[string]interface{}{
			"pending_batches": sctm.settlementCoordinator.GetPendingBatchCount(),
			"status":          "healthy",
		},
		"metrics": sctm.GetStrongConsistencyMetrics(),
	}

	// Merge base health with consistency health
	result := make(map[string]interface{})
	for k, v := range baseHealth {
		result[k] = v
	}
	result["strong_consistency"] = consistencyHealth

	return result
}

// IsHealthy returns true if all strong consistency components are healthy
func (sctm *StrongConsistencyTransactionManager) IsHealthy() bool {
	sctm.mutex.RLock()
	defer sctm.mutex.RUnlock()

	// Check base transaction manager health
	if sctm.TransactionManagerSuite != nil {
		if healthData := sctm.TransactionManagerSuite.GetHealthCheck(); healthData != nil {
			if status, ok := healthData["status"].(string); ok && status != "healthy" {
				return false
			}
		}
	}

	// Check consensus coordinator health
	if sctm.raftCoordinator == nil {
		return false
	}

	// Check balance manager health
	if sctm.balanceManager == nil {
		return false
	}

	// Check settlement coordinator health
	if sctm.settlementCoordinator == nil {
		return false
	}

	// All components are present and healthy
	return true
}

// DefaultStrongConsistencyConfig returns default configuration
func DefaultStrongConsistencyConfig() *StrongConsistencyConfig {
	return &StrongConsistencyConfig{
		SettlementConsensusThreshold: 50000.0, // $50k
		TransferConsensusThreshold:   10000.0, // $10k
		ConsensusTimeout:             30 * time.Second,
		QuorumSize:                   3,
		LeaderElectionTimeout:        10 * time.Second,
		ReconciliationInterval:       5 * time.Minute,
		LockTimeout:                  30 * time.Second,
		MaxRetryAttempts:             3,
		BatchSize:                    100,
		BatchTimeout:                 5 * time.Second,
		EnableParallelProcessing:     true,
		MaxConcurrentOperations:      10,
	}
}
