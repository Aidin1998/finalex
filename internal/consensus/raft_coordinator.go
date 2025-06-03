package consensus

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// OperationType represents the type of consensus operation
type OperationType string

const (
	OperationTypeOrderProcessing OperationType = "order_processing"
	OperationTypeTransaction     OperationType = "transaction"
	OperationTypeLockAcquisition OperationType = "lock_acquisition"
	OperationTypeBalanceTransfer OperationType = "balance_transfer"
	OperationTypeTradeSettlement OperationType = "trade_settlement"
	OperationTypeOrderExecution  OperationType = "order_execution"
)

// Operation represents a generic consensus operation
type Operation struct {
	ID        string                 `json:"id"`
	Type      OperationType          `json:"type"`
	Data      map[string]interface{} `json:"data"`
	Timestamp time.Time              `json:"timestamp"`
}

// CriticalOperation represents a financial operation that requires consensus
type CriticalOperation struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type"` // "balance_transfer", "trade_settlement", "order_execution"
	UserID      string                 `json:"user_id,omitempty"`
	OrderID     string                 `json:"order_id,omitempty"`
	TradeID     string                 `json:"trade_id,omitempty"`
	FromAccount string                 `json:"from_account,omitempty"`
	ToAccount   string                 `json:"to_account,omitempty"`
	Amount      decimal.Decimal        `json:"amount"`
	Currency    string                 `json:"currency"`
	Metadata    map[string]interface{} `json:"metadata"`
	Timestamp   time.Time              `json:"timestamp"`
	Priority    int                    `json:"priority"` // 1=critical, 2=high, 3=normal
}

// ConsensusResult represents the result of a consensus operation
type ConsensusResult struct {
	Operation    *CriticalOperation `json:"operation"`
	Approved     bool               `json:"approved"`
	VotingNodes  []string           `json:"voting_nodes"`
	ApprovalRate float64            `json:"approval_rate"`
	CommittedAt  time.Time          `json:"committed_at"`
	Error        error              `json:"error,omitempty"`
}

// RaftCoordinator provides consensus-based coordination for critical financial operations
type RaftCoordinator struct {
	nodeID   string
	isLeader bool
	cluster  []string
	logger   *zap.Logger

	// In-memory state for consensus operations
	pendingOps   map[string]*CriticalOperation
	committedOps map[string]*ConsensusResult
	mu           sync.RWMutex

	// Consensus configuration
	quorumSize       int
	consensusTimeout time.Duration
	retryMaxAttempts int

	// Channels for consensus coordination
	proposalCh chan *CriticalOperation
	commitCh   chan *ConsensusResult
	shutdownCh chan struct{}

	// Metrics
	consensusMetrics *ConsensusMetrics

	// Recovery and fault tolerance
	recoveryManager *RecoveryManager
}

// ConsensusMetrics tracks consensus performance
type ConsensusMetrics struct {
	OperationsProposed   int64         `json:"operations_proposed"`
	OperationsCommitted  int64         `json:"operations_committed"`
	OperationsRejected   int64         `json:"operations_rejected"`
	AverageConsensusTime time.Duration `json:"average_consensus_time"`
	QuorumFailures       int64         `json:"quorum_failures"`
	LeaderElections      int64         `json:"leader_elections"`
	mu                   sync.RWMutex
}

// NewRaftCoordinator creates a new consensus coordinator
func NewRaftCoordinator(
	nodeID string,
	cluster []string,
	logger *zap.Logger,
) *RaftCoordinator {
	quorumSize := (len(cluster) / 2) + 1

	coordinator := &RaftCoordinator{
		nodeID:           nodeID,
		cluster:          cluster,
		logger:           logger,
		pendingOps:       make(map[string]*CriticalOperation),
		committedOps:     make(map[string]*ConsensusResult),
		quorumSize:       quorumSize,
		consensusTimeout: 30 * time.Second,
		retryMaxAttempts: 3,
		proposalCh:       make(chan *CriticalOperation, 1000),
		commitCh:         make(chan *ConsensusResult, 1000),
		shutdownCh:       make(chan struct{}),
		consensusMetrics: &ConsensusMetrics{},
	}

	coordinator.recoveryManager = NewRecoveryManager(nodeID, coordinator, logger)

	return coordinator
}

// Start initializes the consensus coordinator
func (rc *RaftCoordinator) Start(ctx context.Context) error {
	rc.logger.Info("Starting Raft consensus coordinator",
		zap.String("node_id", rc.nodeID),
		zap.Int("quorum_size", rc.quorumSize),
		zap.Strings("cluster", rc.cluster))

	// Start consensus processing goroutines
	go rc.consensusWorker(ctx)
	go rc.commitWorker(ctx)
	go rc.leaderElectionWorker(ctx)

	// Start recovery manager
	if err := rc.recoveryManager.Start(ctx); err != nil {
		return fmt.Errorf("failed to start recovery manager: %w", err)
	}

	// Start partition detection monitoring
	if err := rc.recoveryManager.partitionDetector.StartMonitoring(ctx, rc.cluster); err != nil {
		return fmt.Errorf("failed to start partition monitoring: %w", err)
	}

	return nil
}

// ProposeOperation proposes a critical operation for consensus
func (rc *RaftCoordinator) ProposeOperation(ctx context.Context, op *CriticalOperation) (*ConsensusResult, error) {
	if !rc.isLeader {
		return nil, fmt.Errorf("node %s is not the leader", rc.nodeID)
	}

	// Validate operation
	if err := rc.validateOperation(op); err != nil {
		return nil, fmt.Errorf("operation validation failed: %w", err)
	}

	// Add to pending operations
	rc.mu.Lock()
	rc.pendingOps[op.ID] = op
	rc.mu.Unlock()

	// Increment metrics
	rc.consensusMetrics.mu.Lock()
	rc.consensusMetrics.OperationsProposed++
	rc.consensusMetrics.mu.Unlock()

	// Send to consensus worker
	select {
	case rc.proposalCh <- op:
		// Successfully queued
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(5 * time.Second):
		return nil, fmt.Errorf("proposal queue timeout")
	}

	// Wait for consensus result with timeout
	resultCh := make(chan *ConsensusResult, 1)
	go func() {
		for {
			select {
			case result := <-rc.commitCh:
				if result.Operation.ID == op.ID {
					resultCh <- result
					return
				}
			case <-ctx.Done():
				return
			case <-time.After(rc.consensusTimeout):
				resultCh <- &ConsensusResult{
					Operation: op,
					Approved:  false,
					Error:     fmt.Errorf("consensus timeout"),
				}
				return
			}
		}
	}()
	select {
	case result := <-resultCh:
		return result, result.Error
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// ProposeGenericOperation proposes a generic operation for consensus
func (rc *RaftCoordinator) ProposeGenericOperation(ctx context.Context, op Operation) (bool, error) {
	// Convert generic Operation to CriticalOperation
	criticalOp := &CriticalOperation{
		ID:        op.ID,
		Type:      string(op.Type),
		Timestamp: op.Timestamp,
		Metadata:  op.Data,
	}

	// Extract common fields from operation data
	if userID, ok := op.Data["user_id"].(string); ok {
		criticalOp.UserID = userID
	}
	if orderID, ok := op.Data["order_id"].(string); ok {
		criticalOp.OrderID = orderID
	}
	if tradeID, ok := op.Data["trade_id"].(string); ok {
		criticalOp.TradeID = tradeID
	}
	if fromAccount, ok := op.Data["from_account"].(string); ok {
		criticalOp.FromAccount = fromAccount
	}
	if toAccount, ok := op.Data["to_account"].(string); ok {
		criticalOp.ToAccount = toAccount
	}
	if currency, ok := op.Data["currency"].(string); ok {
		criticalOp.Currency = currency
	}
	if amountFloat, ok := op.Data["amount"].(float64); ok {
		criticalOp.Amount = decimal.NewFromFloat(amountFloat)
	}
	if priority, ok := op.Data["priority"].(int); ok {
		criticalOp.Priority = priority
	} else {
		criticalOp.Priority = 3 // Default to normal priority
	}

	// Call the existing method
	result, err := rc.ProposeOperation(ctx, criticalOp)
	if err != nil {
		return false, err
	}

	return result.Approved, result.Error
}

// consensusWorker processes consensus proposals
func (rc *RaftCoordinator) consensusWorker(ctx context.Context) {
	for {
		select {
		case op := <-rc.proposalCh:
			result := rc.processConsensus(ctx, op)

			select {
			case rc.commitCh <- result:
				// Successfully committed
			case <-ctx.Done():
				return
			}

		case <-rc.shutdownCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

// processConsensus handles the consensus process for an operation
func (rc *RaftCoordinator) processConsensus(ctx context.Context, op *CriticalOperation) *ConsensusResult {
	startTime := time.Now()

	// Check if we're in a network partition
	if rc.recoveryManager.partitionDetector.IsPartitioned() {
		// Check if we have quorum in our partition
		if !rc.recoveryManager.partitionDetector.HasQuorum(len(rc.cluster)) {
			result := &ConsensusResult{
				Operation: op,
				Approved:  false,
				Error:     fmt.Errorf("consensus failed: no quorum due to network partition"),
			}

			// Record failed operation for recovery
			rc.recoveryManager.RecordFailedOperation(op, "no quorum due to network partition", nil)

			// Trigger quorum loss recovery
			rc.recoveryManager.TriggerRecovery(RecoveryTrigger{
				Type:      QuorumLossTrigger,
				NodeID:    rc.nodeID,
				Timestamp: time.Now(),
				Metadata: map[string]interface{}{
					"operation_id": op.ID,
				},
			})

			return result
		}
	}

	// Simulate consensus voting with timeout and failure detection
	approvalCount := 0
	totalNodes := len(rc.cluster)
	nodeStates := make(map[string]string)

	// Leader automatically approves
	approvalCount = 1
	nodeStates[rc.nodeID] = "approved"

	// Simulate voting from other nodes with network partition awareness
	connectedNodes := rc.getConnectedNodes()
	for i := 1; i < totalNodes; i++ {
		nodeID := rc.cluster[i%len(rc.cluster)]
		if nodeID == rc.nodeID {
			continue
		}

		// Check if node is reachable
		if !rc.isNodeReachable(nodeID, connectedNodes) {
			nodeStates[nodeID] = "unreachable"
			continue
		}

		// Simulate voting with potential timeout
		if rc.simulateNodeVoteWithTimeout(op) {
			approvalCount++
			nodeStates[nodeID] = "approved"
		} else {
			nodeStates[nodeID] = "rejected"
		}
	}

	approvalRate := float64(approvalCount) / float64(len(connectedNodes))
	quorumReached := approvalCount >= rc.quorumSize

	result := &ConsensusResult{
		Operation:    op,
		Approved:     quorumReached,
		VotingNodes:  connectedNodes,
		ApprovalRate: approvalRate,
		CommittedAt:  time.Now(),
	}

	// Handle consensus timeout
	if time.Since(startTime) > rc.consensusTimeout {
		result.Approved = false
		result.Error = fmt.Errorf("consensus timeout after %v", time.Since(startTime))

		// Record failed operation and trigger recovery
		rc.recoveryManager.RecordFailedOperation(op, "consensus timeout", nodeStates)
		rc.recoveryManager.TriggerRecovery(RecoveryTrigger{
			Type:      ConsensusTimeoutTrigger,
			NodeID:    rc.nodeID,
			Timestamp: time.Now(),
			Metadata: map[string]interface{}{
				"operation_id":     op.ID,
				"timeout_duration": time.Since(startTime).String(),
			},
		})

		quorumReached = false
	}

	// Update metrics
	rc.consensusMetrics.mu.Lock()
	consensusTime := time.Since(startTime)
	rc.consensusMetrics.AverageConsensusTime =
		(rc.consensusMetrics.AverageConsensusTime + consensusTime) / 2

	if quorumReached {
		rc.consensusMetrics.OperationsCommitted++
	} else {
		rc.consensusMetrics.OperationsRejected++
		rc.consensusMetrics.QuorumFailures++

		if result.Error == nil {
			result.Error = fmt.Errorf("quorum not reached: %d/%d votes", approvalCount, rc.quorumSize)
		}

		// Record failed operation if not already recorded
		if result.Error.Error() != "consensus timeout" {
			rc.recoveryManager.RecordFailedOperation(op, result.Error.Error(), nodeStates)
		}
	}
	rc.consensusMetrics.mu.Unlock()

	// Store result
	rc.mu.Lock()
	rc.committedOps[op.ID] = result
	delete(rc.pendingOps, op.ID)
	rc.mu.Unlock()

	rc.logger.Info("Consensus completed",
		zap.String("operation_id", op.ID),
		zap.String("operation_type", op.Type),
		zap.Bool("approved", quorumReached),
		zap.Float64("approval_rate", approvalRate),
		zap.Duration("consensus_time", consensusTime),
		zap.Int("connected_nodes", len(connectedNodes)))

	return result
}

// simulateNodeVote simulates a node's voting decision
func (rc *RaftCoordinator) simulateNodeVote(op *CriticalOperation) bool {
	// Higher priority operations have higher approval probability
	switch op.Priority {
	case 1: // Critical
		return true // Always approve critical operations
	case 2: // High
		return time.Now().UnixNano()%10 < 8 // 80% approval
	case 3: // Normal
		return time.Now().UnixNano()%10 < 7 // 70% approval
	default:
		return time.Now().UnixNano()%10 < 5 // 50% approval
	}
}

// commitWorker processes committed consensus results
func (rc *RaftCoordinator) commitWorker(ctx context.Context) {
	for {
		select {
		case result := <-rc.commitCh:
			rc.handleCommittedResult(result)
		case <-rc.shutdownCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

// handleCommittedResult processes a committed consensus result
func (rc *RaftCoordinator) handleCommittedResult(result *ConsensusResult) {
	if result.Approved {
		rc.logger.Info("Operation committed successfully",
			zap.String("operation_id", result.Operation.ID),
			zap.String("operation_type", result.Operation.Type))
	} else {
		rc.logger.Warn("Operation rejected by consensus",
			zap.String("operation_id", result.Operation.ID),
			zap.String("operation_type", result.Operation.Type),
			zap.Error(result.Error))
	}
}

// leaderElectionWorker handles leader election process
func (rc *RaftCoordinator) leaderElectionWorker(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	lastLeaderCheck := time.Now()

	for {
		select {
		case <-ticker.C:
			// Check if current leader is still reachable
			if rc.isLeader {
				// Verify we still have connectivity to majority of nodes
				connectedNodes := rc.getConnectedNodes()
				if len(connectedNodes) < rc.quorumSize {
					rc.logger.Warn("Leader lost quorum, stepping down",
						zap.Int("connected_nodes", len(connectedNodes)),
						zap.Int("quorum_size", rc.quorumSize))

					rc.stepDownAsLeader()

					// Trigger leader failure recovery
					rc.recoveryManager.TriggerRecovery(RecoveryTrigger{
						Type:      LeaderFailureTrigger,
						NodeID:    rc.nodeID,
						Timestamp: time.Now(),
						Metadata: map[string]interface{}{
							"reason":          "quorum_lost",
							"connected_nodes": len(connectedNodes),
						},
					})
				}
			} else {
				// Check if we should become leader
				if rc.shouldBecomeLeader() && time.Since(lastLeaderCheck) > 15*time.Second {
					rc.promoteToLeader()
					lastLeaderCheck = time.Now()
				}
			}

		case <-rc.shutdownCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

// shouldBecomeLeader determines if this node should become leader
func (rc *RaftCoordinator) shouldBecomeLeader() bool {
	// Simplified leader election logic
	return time.Now().UnixNano()%10 < 2 // 20% chance
}

// promoteToLeader promotes this node to leader
func (rc *RaftCoordinator) promoteToLeader() {
	rc.mu.Lock()
	rc.isLeader = true
	rc.mu.Unlock()

	rc.consensusMetrics.mu.Lock()
	rc.consensusMetrics.LeaderElections++
	rc.consensusMetrics.mu.Unlock()

	rc.logger.Info("Node promoted to leader", zap.String("node_id", rc.nodeID))
}

// stepDownAsLeader steps down from leadership role
func (rc *RaftCoordinator) stepDownAsLeader() {
	rc.mu.Lock()
	wasLeader := rc.isLeader
	rc.isLeader = false
	rc.mu.Unlock()

	if wasLeader {
		rc.logger.Info("Node stepped down from leader role", zap.String("node_id", rc.nodeID))
	}
}

// validateOperation validates a critical operation
func (rc *RaftCoordinator) validateOperation(op *CriticalOperation) error {
	if op.ID == "" {
		return fmt.Errorf("operation ID is required")
	}
	if op.Type == "" {
		return fmt.Errorf("operation type is required")
	}
	if op.Amount.IsZero() || op.Amount.IsNegative() {
		return fmt.Errorf("invalid amount: %s", op.Amount.String())
	}
	if op.Currency == "" {
		return fmt.Errorf("currency is required")
	}
	return nil
}

// GetMetrics returns current consensus metrics
func (rc *RaftCoordinator) GetMetrics() *ConsensusMetrics {
	rc.consensusMetrics.mu.RLock()
	defer rc.consensusMetrics.mu.RUnlock()

	return &ConsensusMetrics{
		OperationsProposed:   rc.consensusMetrics.OperationsProposed,
		OperationsCommitted:  rc.consensusMetrics.OperationsCommitted,
		OperationsRejected:   rc.consensusMetrics.OperationsRejected,
		AverageConsensusTime: rc.consensusMetrics.AverageConsensusTime,
		QuorumFailures:       rc.consensusMetrics.QuorumFailures,
		LeaderElections:      rc.consensusMetrics.LeaderElections,
	}
}

// IsLeader returns whether this node is the current leader
func (rc *RaftCoordinator) IsLeader() bool {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	return rc.isLeader
}

// Stop gracefully shuts down the consensus coordinator
func (rc *RaftCoordinator) Stop(ctx context.Context) error {
	rc.logger.Info("Stopping Raft consensus coordinator")

	// Stop recovery manager first
	if rc.recoveryManager != nil {
		if err := rc.recoveryManager.Stop(ctx); err != nil {
			rc.logger.Error("Failed to stop recovery manager", zap.Error(err))
		}
	}

	close(rc.shutdownCh)

	// Wait for graceful shutdown with timeout
	select {
	case <-time.After(30 * time.Second):
		rc.logger.Warn("Consensus coordinator shutdown timeout")
	case <-ctx.Done():
		return ctx.Err()
	}

	return nil
}

// getConnectedNodes returns the list of currently connected nodes
func (rc *RaftCoordinator) getConnectedNodes() []string {
	if rc.recoveryManager == nil || rc.recoveryManager.partitionDetector == nil {
		return rc.cluster
	}

	connectivity := rc.recoveryManager.partitionDetector.GetConnectivityStatus()
	connected := []string{rc.nodeID} // Include self

	for _, nodeID := range rc.cluster {
		if nodeID == rc.nodeID {
			continue
		}

		if conn, exists := connectivity[nodeID]; exists && conn.IsReachable {
			connected = append(connected, nodeID)
		}
	}

	return connected
}

// isNodeReachable checks if a specific node is reachable
func (rc *RaftCoordinator) isNodeReachable(nodeID string, connectedNodes []string) bool {
	for _, connectedNode := range connectedNodes {
		if connectedNode == nodeID {
			return true
		}
	}
	return false
}

// simulateNodeVoteWithTimeout simulates a node's voting decision with timeout consideration
func (rc *RaftCoordinator) simulateNodeVoteWithTimeout(op *CriticalOperation) bool {
	// Simulate network delay and potential timeout
	delay := time.Duration(time.Now().UnixNano()%1000) * time.Millisecond

	// If delay is too high, consider it a timeout
	if delay > 500*time.Millisecond {
		return false
	}

	// Use existing voting logic
	return rc.simulateNodeVote(op)
}

// GetRecoveryManager returns the recovery manager instance
func (rc *RaftCoordinator) GetRecoveryManager() *RecoveryManager {
	return rc.recoveryManager
}

// ForceRecovery manually triggers a recovery operation
func (rc *RaftCoordinator) ForceRecovery(triggerType RecoveryTriggerType, metadata map[string]interface{}) {
	if rc.recoveryManager == nil {
		rc.logger.Error("Recovery manager not initialized")
		return
	}

	rc.recoveryManager.TriggerRecovery(RecoveryTrigger{
		Type:      triggerType,
		NodeID:    rc.nodeID,
		Timestamp: time.Now(),
		Metadata:  metadata,
	})
}

// GetNetworkHealth returns current network health status
func (rc *RaftCoordinator) GetNetworkHealth() NetworkHealthStatus {
	if rc.recoveryManager == nil {
		return NetworkHealthStatus{}
	}
	return rc.recoveryManager.GetNetworkHealth()
}

// IsPartitioned returns whether the cluster is currently partitioned
func (rc *RaftCoordinator) IsPartitioned() bool {
	if rc.recoveryManager == nil || rc.recoveryManager.partitionDetector == nil {
		return false
	}
	return rc.recoveryManager.partitionDetector.IsPartitioned()
}

// HasQuorum returns whether the current partition has quorum
func (rc *RaftCoordinator) HasQuorum() bool {
	if rc.recoveryManager == nil || rc.recoveryManager.partitionDetector == nil {
		return true
	}
	return rc.recoveryManager.partitionDetector.HasQuorum(len(rc.cluster))
}
