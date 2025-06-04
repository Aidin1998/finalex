package consensus

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"go.uber.org/zap"
)

// PartitionDetector detects network partitions in the consensus cluster
type PartitionDetector struct {
	nodeID string
	logger *zap.Logger

	// Detection configuration
	pingInterval       time.Duration
	pingTimeout        time.Duration
	partitionThreshold int // Number of failed pings before considering partition

	// Node connectivity tracking
	nodeConnectivity     map[string]*NodeConnectivity
	lastFullConnectivity time.Time
	mu                   sync.RWMutex

	// Partition state
	currentPartition *NetworkPartition
	partitionHistory []*NetworkPartition
}

// NodeConnectivity tracks connectivity to a specific node
type NodeConnectivity struct {
	NodeID              string        `json:"node_id"`
	LastSuccessfulPing  time.Time     `json:"last_successful_ping"`
	ConsecutiveFailures int           `json:"consecutive_failures"`
	AverageLatency      time.Duration `json:"average_latency"`
	IsReachable         bool          `json:"is_reachable"`
	LastError           string        `json:"last_error,omitempty"`
}

// NetworkPartition represents a detected network partition
type NetworkPartition struct {
	DetectedAt      time.Time         `json:"detected_at"`
	PartitionID     string            `json:"partition_id"`
	MyPartition     []string          `json:"my_partition"`
	OtherPartitions [][]string        `json:"other_partitions"`
	HasSplitBrain   bool              `json:"has_split_brain"`
	QuorumLost      bool              `json:"quorum_lost"`
	SeverityLevel   PartitionSeverity `json:"severity_level"`
	ResolvedAt      *time.Time        `json:"resolved_at,omitempty"`
}

// PartitionSeverity indicates the severity of a network partition
type PartitionSeverity int

const (
	LowSeverity PartitionSeverity = iota
	MediumSeverity
	HighSeverity
	CriticalSeverity
)

// NewPartitionDetector creates a new partition detector
func NewPartitionDetector(nodeID string, logger *zap.Logger) *PartitionDetector {
	return &PartitionDetector{
		nodeID:             nodeID,
		logger:             logger.Named("partition-detector"),
		pingInterval:       5 * time.Second,
		pingTimeout:        2 * time.Second,
		partitionThreshold: 3,
		nodeConnectivity:   make(map[string]*NodeConnectivity),
		partitionHistory:   make([]*NetworkPartition, 0),
	}
}

// StartMonitoring begins partition detection monitoring
func (pd *PartitionDetector) StartMonitoring(ctx context.Context, clusterNodes []string) error {
	pd.logger.Info("Starting partition detection monitoring",
		zap.Strings("cluster_nodes", clusterNodes))

	// Initialize connectivity tracking for all nodes
	pd.mu.Lock()
	for _, nodeID := range clusterNodes {
		if nodeID != pd.nodeID {
			pd.nodeConnectivity[nodeID] = &NodeConnectivity{
				NodeID:      nodeID,
				IsReachable: true,
			}
		}
	}
	pd.lastFullConnectivity = time.Now()
	pd.mu.Unlock()

	// Start monitoring goroutine
	go pd.monitorConnectivity(ctx, clusterNodes)

	return nil
}

// monitorConnectivity continuously monitors connectivity to cluster nodes
func (pd *PartitionDetector) monitorConnectivity(ctx context.Context, clusterNodes []string) {
	ticker := time.NewTicker(pd.pingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			pd.checkNodeConnectivity(ctx, clusterNodes)
			pd.analyzePartitionState(clusterNodes)

		case <-ctx.Done():
			return
		}
	}
}

// checkNodeConnectivity checks connectivity to all cluster nodes
func (pd *PartitionDetector) checkNodeConnectivity(ctx context.Context, clusterNodes []string) {
	var wg sync.WaitGroup

	for _, nodeID := range clusterNodes {
		if nodeID == pd.nodeID {
			continue
		}

		wg.Add(1)
		go func(targetNodeID string) {
			defer wg.Done()
			pd.pingNode(ctx, targetNodeID)
		}(nodeID)
	}

	wg.Wait()
}

// pingNode performs a connectivity check to a specific node
func (pd *PartitionDetector) pingNode(ctx context.Context, nodeID string) {
	startTime := time.Now()

	// Create timeout context for ping
	pingCtx, cancel := context.WithTimeout(ctx, pd.pingTimeout)
	defer cancel()

	// Simulate node ping (in real implementation, this would be actual network call)
	// For now, we'll use a TCP connection attempt to demonstrate
	addr := fmt.Sprintf("%s:8080", nodeID) // Assuming nodes listen on port 8080

	var d net.Dialer
	conn, err := d.DialContext(pingCtx, "tcp", addr)

	latency := time.Since(startTime)

	pd.mu.Lock()
	defer pd.mu.Unlock()

	connectivity, exists := pd.nodeConnectivity[nodeID]
	if !exists {
		connectivity = &NodeConnectivity{NodeID: nodeID}
		pd.nodeConnectivity[nodeID] = connectivity
	}

	if err != nil {
		// Connection failed
		connectivity.ConsecutiveFailures++
		connectivity.IsReachable = false
		connectivity.LastError = err.Error()

		pd.logger.Debug("Node ping failed",
			zap.String("target_node", nodeID),
			zap.Int("consecutive_failures", connectivity.ConsecutiveFailures),
			zap.Error(err))
	} else {
		// Connection successful
		conn.Close()
		connectivity.ConsecutiveFailures = 0
		connectivity.IsReachable = true
		connectivity.LastSuccessfulPing = time.Now()
		connectivity.LastError = ""

		// Update average latency
		if connectivity.AverageLatency == 0 {
			connectivity.AverageLatency = latency
		} else {
			connectivity.AverageLatency = (connectivity.AverageLatency + latency) / 2
		}

		pd.logger.Debug("Node ping successful",
			zap.String("target_node", nodeID),
			zap.Duration("latency", latency))
	}
}

// analyzePartitionState analyzes current connectivity state for partitions
func (pd *PartitionDetector) analyzePartitionState(clusterNodes []string) {
	pd.mu.Lock()
	defer pd.mu.Unlock()

	reachableNodes := []string{pd.nodeID} // Include self
	unreachableNodes := []string{}

	// Categorize nodes based on connectivity
	for _, nodeID := range clusterNodes {
		if nodeID == pd.nodeID {
			continue
		}

		connectivity, exists := pd.nodeConnectivity[nodeID]
		if !exists || !connectivity.IsReachable ||
			connectivity.ConsecutiveFailures >= pd.partitionThreshold {
			unreachableNodes = append(unreachableNodes, nodeID)
		} else {
			reachableNodes = append(reachableNodes, nodeID)
		}
	}

	// Check if this represents a new partition
	if len(unreachableNodes) > 0 {
		if pd.currentPartition == nil {
			// New partition detected
			partition := &NetworkPartition{
				DetectedAt:    time.Now(),
				PartitionID:   pd.generatePartitionID(),
				MyPartition:   reachableNodes,
				QuorumLost:    len(reachableNodes) < (len(clusterNodes)/2)+1,
				SeverityLevel: pd.calculateSeverity(len(reachableNodes), len(clusterNodes)),
			}

			// Determine if there might be split-brain
			partition.HasSplitBrain = pd.detectSplitBrain(reachableNodes, unreachableNodes, clusterNodes)

			// Add unreachable nodes as separate partitions
			for _, unreachableNode := range unreachableNodes {
				partition.OtherPartitions = append(partition.OtherPartitions, []string{unreachableNode})
			}

			pd.currentPartition = partition
			pd.partitionHistory = append(pd.partitionHistory, partition)

			pd.logger.Warn("Network partition detected",
				zap.String("partition_id", partition.PartitionID),
				zap.Strings("reachable_nodes", reachableNodes),
				zap.Strings("unreachable_nodes", unreachableNodes),
				zap.Bool("quorum_lost", partition.QuorumLost),
				zap.Bool("has_split_brain", partition.HasSplitBrain))
		}
	} else {
		// Check if partition is resolved
		if pd.currentPartition != nil {
			now := time.Now()
			pd.currentPartition.ResolvedAt = &now
			pd.currentPartition = nil
			pd.lastFullConnectivity = now

			pd.logger.Info("Network partition resolved",
				zap.Strings("all_nodes_reachable", reachableNodes))
		}
	}
}

// detectSplitBrain determines if there might be a split-brain scenario
func (pd *PartitionDetector) detectSplitBrain(reachableNodes, unreachableNodes, allNodes []string) bool {
	reachableCount := len(reachableNodes)
	unreachableCount := len(unreachableNodes)
	totalNodes := len(allNodes)

	// Split-brain is possible if both partitions could have quorum
	quorumSize := (totalNodes / 2) + 1

	// If our partition has quorum and there are enough unreachable nodes
	// to potentially form their own quorum, split-brain is possible
	if reachableCount >= quorumSize && unreachableCount >= quorumSize {
		return true
	}

	// Also consider split-brain if partitions are roughly equal in size
	if reachableCount > 1 && unreachableCount > 1 {
		ratio := float64(reachableCount) / float64(unreachableCount)
		if ratio >= 0.5 && ratio <= 2.0 {
			return true
		}
	}

	return false
}

// calculateSeverity determines the severity of a partition
func (pd *PartitionDetector) calculateSeverity(reachableNodes, totalNodes int) PartitionSeverity {
	ratio := float64(reachableNodes) / float64(totalNodes)

	switch {
	case ratio >= 0.8:
		return LowSeverity
	case ratio >= 0.6:
		return MediumSeverity
	case ratio >= 0.4:
		return HighSeverity
	default:
		return CriticalSeverity
	}
}

// generatePartitionID creates a unique identifier for a partition
func (pd *PartitionDetector) generatePartitionID() string {
	return fmt.Sprintf("partition-%s-%d", pd.nodeID, time.Now().Unix())
}

// DetectPartition returns the current partition state
func (pd *PartitionDetector) DetectPartition(ctx context.Context) (*NetworkPartition, error) {
	pd.mu.RLock()
	defer pd.mu.RUnlock()

	if pd.currentPartition == nil {
		return nil, fmt.Errorf("no partition currently detected")
	}

	// Return a copy to avoid race conditions
	partition := *pd.currentPartition
	return &partition, nil
}

// GetConnectivityStatus returns current connectivity status for all nodes
func (pd *PartitionDetector) GetConnectivityStatus() map[string]*NodeConnectivity {
	pd.mu.RLock()
	defer pd.mu.RUnlock()

	status := make(map[string]*NodeConnectivity)
	for nodeID, connectivity := range pd.nodeConnectivity {
		// Create a copy
		status[nodeID] = &NodeConnectivity{
			NodeID:              connectivity.NodeID,
			LastSuccessfulPing:  connectivity.LastSuccessfulPing,
			ConsecutiveFailures: connectivity.ConsecutiveFailures,
			AverageLatency:      connectivity.AverageLatency,
			IsReachable:         connectivity.IsReachable,
			LastError:           connectivity.LastError,
		}
	}

	return status
}

// GetPartitionHistory returns the history of detected partitions
func (pd *PartitionDetector) GetPartitionHistory() []*NetworkPartition {
	pd.mu.RLock()
	defer pd.mu.RUnlock()

	history := make([]*NetworkPartition, len(pd.partitionHistory))
	copy(history, pd.partitionHistory)
	return history
}

// IsPartitioned returns whether the cluster is currently partitioned
func (pd *PartitionDetector) IsPartitioned() bool {
	pd.mu.RLock()
	defer pd.mu.RUnlock()

	return pd.currentPartition != nil
}

// HasQuorum returns whether the current partition has quorum
func (pd *PartitionDetector) HasQuorum(totalNodes int) bool {
	pd.mu.RLock()
	defer pd.mu.RUnlock()

	if pd.currentPartition == nil {
		return true // No partition means full connectivity
	}

	quorumSize := (totalNodes / 2) + 1
	return len(pd.currentPartition.MyPartition) >= quorumSize
}

// GetLastFullConnectivity returns the timestamp of the last full connectivity
func (pd *PartitionDetector) GetLastFullConnectivity() time.Time {
	pd.mu.RLock()
	defer pd.mu.RUnlock()

	return pd.lastFullConnectivity
}
