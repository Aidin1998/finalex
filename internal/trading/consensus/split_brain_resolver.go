package consensus

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// SplitBrainResolver handles resolution of split-brain scenarios
type SplitBrainResolver struct {
	nodeID string
	logger *zap.Logger

	// Resolution configuration
	resolutionTimeout time.Duration
	priorityWeights   map[string]float64

	// Resolution state
	activeResolutions map[string]*ResolutionAttempt
	resolutionHistory []*ResolutionResult
	mu                sync.RWMutex
}

// ResolutionAttempt tracks an ongoing split-brain resolution
type ResolutionAttempt struct {
	PartitionID  string                 `json:"partition_id"`
	StartedAt    time.Time              `json:"started_at"`
	Strategy     ResolutionStrategy     `json:"strategy"`
	Participants []string               `json:"participants"`
	Metadata     map[string]interface{} `json:"metadata"`
	Status       ResolutionStatus       `json:"status"`
}

// ResolutionResult represents the outcome of a split-brain resolution
type ResolutionResult struct {
	PartitionID      string             `json:"partition_id"`
	Strategy         ResolutionStrategy `json:"strategy"`
	StartedAt        time.Time          `json:"started_at"`
	CompletedAt      time.Time          `json:"completed_at"`
	Success          bool               `json:"success"`
	WinningPartition []string           `json:"winning_partition"`
	DeactivatedNodes []string           `json:"deactivated_nodes"`
	Error            string             `json:"error,omitempty"`
}

// ResolutionStrategy defines different approaches to resolving split-brain
type ResolutionStrategy int

const (
	NodePriorityStrategy ResolutionStrategy = iota
	LastCommitStrategy
	ManualInterventionStrategy
	QuorumSizeStrategy
	NodeIdLexicographicStrategy
	EmergencyShutdownStrategy
)

// ResolutionStatus tracks the status of a resolution attempt
type ResolutionStatus int

const (
	ResolutionPending ResolutionStatus = iota
	ResolutionInProgress
	ResolutionCompleted
	ResolutionFailed
	ResolutionTimedOut
)

// NewSplitBrainResolver creates a new split-brain resolver
func NewSplitBrainResolver(nodeID string, logger *zap.Logger) *SplitBrainResolver {
	return &SplitBrainResolver{
		nodeID:            nodeID,
		logger:            logger.Named("split-brain-resolver"),
		resolutionTimeout: 60 * time.Second,
		priorityWeights: map[string]float64{
			"leader_history":   0.3,
			"operational_load": 0.2,
			"node_stability":   0.2,
			"data_freshness":   0.2,
			"manual_priority":  0.1,
		},
		activeResolutions: make(map[string]*ResolutionAttempt),
		resolutionHistory: make([]*ResolutionResult, 0),
	}
}

// ResolveSplitBrain attempts to resolve a split-brain scenario
func (sbr *SplitBrainResolver) ResolveSplitBrain(
	ctx context.Context,
	partition *NetworkPartition,
) error {
	sbr.logger.Warn("Attempting to resolve split-brain scenario",
		zap.String("partition_id", partition.PartitionID),
		zap.Strings("my_partition", partition.MyPartition))

	// Check if resolution is already in progress
	sbr.mu.Lock()
	if _, exists := sbr.activeResolutions[partition.PartitionID]; exists {
		sbr.mu.Unlock()
		return fmt.Errorf("resolution already in progress for partition %s", partition.PartitionID)
	}

	// Determine the best resolution strategy
	strategy := sbr.selectResolutionStrategy(partition)

	// Create resolution attempt
	attempt := &ResolutionAttempt{
		PartitionID:  partition.PartitionID,
		StartedAt:    time.Now(),
		Strategy:     strategy,
		Participants: partition.MyPartition,
		Status:       ResolutionPending,
		Metadata:     make(map[string]interface{}),
	}

	sbr.activeResolutions[partition.PartitionID] = attempt
	sbr.mu.Unlock()

	// Execute resolution with timeout
	ctx, cancel := context.WithTimeout(ctx, sbr.resolutionTimeout)
	defer cancel()

	result := sbr.executeResolution(ctx, attempt, partition)

	// Record result
	sbr.mu.Lock()
	delete(sbr.activeResolutions, partition.PartitionID)
	sbr.resolutionHistory = append(sbr.resolutionHistory, result)
	sbr.mu.Unlock()

	if !result.Success {
		return fmt.Errorf("split-brain resolution failed: %s", result.Error)
	}

	sbr.logger.Info("Split-brain resolution completed successfully",
		zap.String("partition_id", partition.PartitionID),
		zap.String("strategy", sbr.getStrategyName(strategy)),
		zap.Strings("winning_partition", result.WinningPartition),
		zap.Duration("duration", result.CompletedAt.Sub(result.StartedAt)))

	return nil
}

// selectResolutionStrategy chooses the best strategy for the given partition
func (sbr *SplitBrainResolver) selectResolutionStrategy(partition *NetworkPartition) ResolutionStrategy {
	// For critical financial operations, prioritize safety
	switch partition.SeverityLevel {
	case CriticalSeverity:
		// In critical situations, use manual intervention or emergency shutdown
		return ManualInterventionStrategy

	case HighSeverity:
		// Use last commit strategy to preserve data consistency
		return LastCommitStrategy

	case MediumSeverity:
		// Use node priority based on operational factors
		return NodePriorityStrategy

	default:
		// For low severity, use quorum size strategy
		return QuorumSizeStrategy
	}
}

// executeResolution executes the chosen resolution strategy
func (sbr *SplitBrainResolver) executeResolution(
	ctx context.Context,
	attempt *ResolutionAttempt,
	partition *NetworkPartition,
) *ResolutionResult {
	result := &ResolutionResult{
		PartitionID: partition.PartitionID,
		Strategy:    attempt.Strategy,
		StartedAt:   attempt.StartedAt,
		CompletedAt: time.Now(),
		Success:     false,
	}

	// Update attempt status
	sbr.mu.Lock()
	attempt.Status = ResolutionInProgress
	sbr.mu.Unlock()

	var err error
	switch attempt.Strategy {
	case NodePriorityStrategy:
		err = sbr.resolveByNodePriority(ctx, attempt, partition, result)
	case LastCommitStrategy:
		err = sbr.resolveByLastCommit(ctx, attempt, partition, result)
	case ManualInterventionStrategy:
		err = sbr.resolveByManualIntervention(ctx, attempt, partition, result)
	case QuorumSizeStrategy:
		err = sbr.resolveByQuorumSize(ctx, attempt, partition, result)
	case NodeIdLexicographicStrategy:
		err = sbr.resolveByNodeIdLexicographic(ctx, attempt, partition, result)
	case EmergencyShutdownStrategy:
		err = sbr.resolveByEmergencyShutdown(ctx, attempt, partition, result)
	default:
		err = fmt.Errorf("unknown resolution strategy: %d", attempt.Strategy)
	}

	result.CompletedAt = time.Now()

	if err != nil {
		result.Error = err.Error()
		attempt.Status = ResolutionFailed
	} else {
		result.Success = true
		attempt.Status = ResolutionCompleted
	}

	return result
}

// resolveByNodePriority resolves based on node priority scores
func (sbr *SplitBrainResolver) resolveByNodePriority(
	ctx context.Context,
	attempt *ResolutionAttempt,
	partition *NetworkPartition,
	result *ResolutionResult,
) error {
	sbr.logger.Info("Resolving split-brain using node priority strategy")

	// Calculate priority scores for nodes in our partition
	nodeScores := make(map[string]float64)
	for _, nodeID := range partition.MyPartition {
		score := sbr.calculateNodePriority(nodeID)
		nodeScores[nodeID] = score
	}

	// Find the highest priority node
	var highestNode string
	var highestScore float64
	for nodeID, score := range nodeScores {
		if score > highestScore {
			highestScore = score
			highestNode = nodeID
		}
	}

	// If we are the highest priority node, we become the winning partition
	if highestNode == sbr.nodeID {
		result.WinningPartition = partition.MyPartition
		sbr.logger.Info("This node has highest priority, continuing operations",
			zap.String("node_id", sbr.nodeID),
			zap.Float64("priority_score", highestScore))
	} else {
		// Deactivate ourselves in favor of higher priority node
		result.DeactivatedNodes = []string{sbr.nodeID}
		result.WinningPartition = []string{highestNode}
		sbr.logger.Info("Deactivating in favor of higher priority node",
			zap.String("higher_priority_node", highestNode),
			zap.Float64("their_score", highestScore),
			zap.Float64("our_score", nodeScores[sbr.nodeID]))
	}

	return nil
}

// resolveByLastCommit resolves based on the most recent committed operation
func (sbr *SplitBrainResolver) resolveByLastCommit(
	ctx context.Context,
	attempt *ResolutionAttempt,
	partition *NetworkPartition,
	result *ResolutionResult,
) error {
	sbr.logger.Info("Resolving split-brain using last commit strategy")

	// Get the timestamp of our last committed operation
	// This would require integration with the consensus log
	lastCommitTime := sbr.getLastCommitTime()

	// In a real implementation, we would need to communicate with other
	// partitions to compare last commit times. For now, we'll use a heuristic.

	// If we have recent commits, we likely have fresher data
	if time.Since(lastCommitTime) < 5*time.Minute {
		result.WinningPartition = partition.MyPartition
		sbr.logger.Info("This partition has recent commits, continuing operations",
			zap.Time("last_commit", lastCommitTime))
	} else {
		// Our data might be stale, better to deactivate
		result.DeactivatedNodes = partition.MyPartition
		sbr.logger.Info("This partition has stale data, deactivating",
			zap.Time("last_commit", lastCommitTime))
	}

	return nil
}

// resolveByManualIntervention waits for manual intervention
func (sbr *SplitBrainResolver) resolveByManualIntervention(
	ctx context.Context,
	attempt *ResolutionAttempt,
	partition *NetworkPartition,
	result *ResolutionResult,
) error {
	sbr.logger.Warn("Split-brain requires manual intervention",
		zap.String("partition_id", partition.PartitionID))

	// In a real implementation, this would:
	// 1. Send alerts to operators
	// 2. Wait for manual resolution commands
	// 3. Execute the manual decision

	// For now, we'll implement a safe default: deactivate to prevent data corruption
	result.DeactivatedNodes = partition.MyPartition
	result.Error = "Manual intervention required - automatically deactivating for safety"

	return fmt.Errorf("manual intervention required")
}

// resolveByQuorumSize resolves based on partition size
func (sbr *SplitBrainResolver) resolveByQuorumSize(
	ctx context.Context,
	attempt *ResolutionAttempt,
	partition *NetworkPartition,
	result *ResolutionResult,
) error {
	sbr.logger.Info("Resolving split-brain using quorum size strategy")

	// If our partition has quorum, we continue
	totalNodes := len(partition.MyPartition)
	for _, otherPartition := range partition.OtherPartitions {
		totalNodes += len(otherPartition)
	}

	quorumSize := (totalNodes / 2) + 1

	if len(partition.MyPartition) >= quorumSize {
		result.WinningPartition = partition.MyPartition
		sbr.logger.Info("This partition has quorum, continuing operations",
			zap.Int("partition_size", len(partition.MyPartition)),
			zap.Int("quorum_size", quorumSize))
	} else {
		result.DeactivatedNodes = partition.MyPartition
		sbr.logger.Info("This partition lacks quorum, deactivating",
			zap.Int("partition_size", len(partition.MyPartition)),
			zap.Int("quorum_size", quorumSize))
	}

	return nil
}

// resolveByNodeIdLexicographic resolves using lexicographic ordering of node IDs
func (sbr *SplitBrainResolver) resolveByNodeIdLexicographic(
	ctx context.Context,
	attempt *ResolutionAttempt,
	partition *NetworkPartition,
	result *ResolutionResult,
) error {
	sbr.logger.Info("Resolving split-brain using lexicographic node ID strategy")

	// Find the lexicographically smallest node ID in our partition
	smallestNode := partition.MyPartition[0]
	for _, nodeID := range partition.MyPartition[1:] {
		if nodeID < smallestNode {
			smallestNode = nodeID
		}
	}

	// If we are the smallest node, we continue
	if smallestNode == sbr.nodeID {
		result.WinningPartition = partition.MyPartition
		sbr.logger.Info("This node has smallest ID, continuing operations",
			zap.String("node_id", sbr.nodeID))
	} else {
		// Deactivate in favor of the smallest node
		result.DeactivatedNodes = []string{sbr.nodeID}
		result.WinningPartition = []string{smallestNode}
		sbr.logger.Info("Deactivating in favor of smallest node ID",
			zap.String("smallest_node", smallestNode))
	}

	return nil
}

// resolveByEmergencyShutdown performs emergency shutdown of all nodes
func (sbr *SplitBrainResolver) resolveByEmergencyShutdown(
	ctx context.Context,
	attempt *ResolutionAttempt,
	partition *NetworkPartition,
	result *ResolutionResult,
) error {
	sbr.logger.Error("Performing emergency shutdown to prevent data corruption")

	// Mark all nodes for deactivation
	result.DeactivatedNodes = partition.MyPartition

	// This would trigger an emergency shutdown of consensus operations
	// to prevent any potential data corruption

	return nil
}

// Helper methods

func (sbr *SplitBrainResolver) calculateNodePriority(nodeID string) float64 {
	// This would calculate a priority score based on various factors:
	// - Historical leadership stability
	// - Current operational load
	// - Data freshness
	// - Manual priority settings

	// For demonstration, we'll use a simple heuristic
	score := 0.0

	// Leader history weight
	if nodeID == sbr.nodeID {
		score += sbr.priorityWeights["leader_history"] * 0.8
	} else {
		score += sbr.priorityWeights["leader_history"] * 0.5
	}

	// Node stability (simulated)
	score += sbr.priorityWeights["node_stability"] * 0.7

	// Operational load (simulated)
	score += sbr.priorityWeights["operational_load"] * 0.6

	// Data freshness (simulated)
	score += sbr.priorityWeights["data_freshness"] * 0.8

	return score
}

func (sbr *SplitBrainResolver) getLastCommitTime() time.Time {
	// This would retrieve the timestamp of the last committed operation
	// from the consensus log. For now, we'll simulate it.
	return time.Now().Add(-2 * time.Minute)
}

func (sbr *SplitBrainResolver) getStrategyName(strategy ResolutionStrategy) string {
	names := map[ResolutionStrategy]string{
		NodePriorityStrategy:        "node_priority",
		LastCommitStrategy:          "last_commit",
		ManualInterventionStrategy:  "manual_intervention",
		QuorumSizeStrategy:          "quorum_size",
		NodeIdLexicographicStrategy: "node_id_lexicographic",
		EmergencyShutdownStrategy:   "emergency_shutdown",
	}

	if name, exists := names[strategy]; exists {
		return name
	}
	return "unknown"
}

// GetActiveResolutions returns currently active resolution attempts
func (sbr *SplitBrainResolver) GetActiveResolutions() map[string]*ResolutionAttempt {
	sbr.mu.RLock()
	defer sbr.mu.RUnlock()

	active := make(map[string]*ResolutionAttempt)
	for id, attempt := range sbr.activeResolutions {
		// Create a copy
		active[id] = &ResolutionAttempt{
			PartitionID:  attempt.PartitionID,
			StartedAt:    attempt.StartedAt,
			Strategy:     attempt.Strategy,
			Participants: append([]string{}, attempt.Participants...),
			Status:       attempt.Status,
			Metadata:     make(map[string]interface{}),
		}

		// Copy metadata
		for k, v := range attempt.Metadata {
			active[id].Metadata[k] = v
		}
	}

	return active
}

// GetResolutionHistory returns the history of resolution attempts
func (sbr *SplitBrainResolver) GetResolutionHistory() []*ResolutionResult {
	sbr.mu.RLock()
	defer sbr.mu.RUnlock()

	history := make([]*ResolutionResult, len(sbr.resolutionHistory))
	copy(history, sbr.resolutionHistory)
	return history
}

// SetPriorityWeights allows customization of priority calculation weights
func (sbr *SplitBrainResolver) SetPriorityWeights(weights map[string]float64) {
	sbr.mu.Lock()
	defer sbr.mu.Unlock()

	for key, weight := range weights {
		sbr.priorityWeights[key] = weight
	}
}
