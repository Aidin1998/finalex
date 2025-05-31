package orderqueue

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.etcd.io/etcd/clientv3"
	"go.etcd.io/etcd/clientv3/concurrency"
	"go.uber.org/zap"
)

// NodeRole represents the role of a node in the failover cluster.
type NodeRole int32

const (
	NodeRoleStandby NodeRole = iota
	NodeRoleLeader
	NodeRoleCandidate
)

// NodeStatus represents the health status of a node.
type NodeStatus int32

const (
	NodeStatusUnknown NodeStatus = iota
	NodeStatusHealthy
	NodeStatusUnhealthy
	NodeStatusFailed
)

// Node represents a node in the failover cluster.
type Node struct {
	ID       string    `json:"id"`
	Address  string    `json:"address"`
	Role     NodeRole  `json:"role"`
	Status   NodeStatus `json:"status"`
	LastSeen time.Time `json:"last_seen"`
	Priority int       `json:"priority"` // Higher values = higher priority
}

// FailoverManager orchestrates hot standby order processors and failover.
type FailoverManager struct {
	// Node identification
	nodeID   string
	nodeAddr string
	priority int

	// etcd client and concurrency
	etcdClient    *clientv3.Client
	session       *concurrency.Session
	election      *concurrency.Election
	
	// State management
	role           int32 // atomic NodeRole
	status         int32 // atomic NodeStatus
	leaderID       string
	leaderMu       sync.RWMutex
	
	// Health monitoring
	healthCheckInterval time.Duration
	nodes               map[string]*Node
	nodesMu             sync.RWMutex
	
	// State synchronization
	stateManager  *StateManager
	queue         Queue
	
	// Failover configuration
	config FailoverConfig
	logger *zap.SugaredLogger
	
	// Lifecycle management
	stopCh   chan struct{}
	stopped  bool
	stopMu   sync.RWMutex
	
	// Split-brain prevention
	splitBrainTTL       time.Duration
	leadershipLeaseTTL  time.Duration
	lastLeadershipCheck time.Time
	
	// Callbacks for failover events
	onBecomeLeader   func() error
	onBecomeStandby  func() error
	onLeaderChange   func(newLeaderID string)
}

// FailoverConfig configures failover behavior.
type FailoverConfig struct {
	EtcdEndpoints       []string
	EtcdTimeout         time.Duration
	HealthCheckInterval time.Duration
	LeadershipTTL       time.Duration
	SplitBrainTTL       time.Duration
	MaxFailoverTime     time.Duration
	AutoRecover         bool
}

// FailoverMetrics tracks failover performance metrics.
type FailoverMetrics struct {
	FailoverCount        int64         `json:"failover_count"`
	LastFailoverTime     time.Time     `json:"last_failover_time"`
	LastFailoverDuration time.Duration `json:"last_failover_duration"`
	LeadershipChanges    int64         `json:"leadership_changes"`
	SplitBrainEvents     int64         `json:"split_brain_events"`
	AverageFailoverTime  time.Duration `json:"average_failover_time"`
	HealthCheckFailures  int64         `json:"health_check_failures"`
}

// NewFailoverManager initializes a new failover manager.
func NewFailoverManager(
	nodeID, nodeAddr string,
	priority int,
	stateManager *StateManager,
	queue Queue,
	config FailoverConfig,
	logger *zap.SugaredLogger,
) (*FailoverManager, error) {
	// Set defaults
	if config.HealthCheckInterval == 0 {
		config.HealthCheckInterval = 5 * time.Second
	}
	if config.LeadershipTTL == 0 {
		config.LeadershipTTL = 30 * time.Second
	}
	if config.SplitBrainTTL == 0 {
		config.SplitBrainTTL = 10 * time.Second
	}
	if config.MaxFailoverTime == 0 {
		config.MaxFailoverTime = 30 * time.Second
	}
	if config.EtcdTimeout == 0 {
		config.EtcdTimeout = 5 * time.Second
	}

	// Initialize etcd client
	etcdClient, err := clientv3.New(clientv3.Config{
		Endpoints:   config.EtcdEndpoints,
		DialTimeout: config.EtcdTimeout,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create etcd client: %w", err)
	}

	fm := &FailoverManager{
		nodeID:              nodeID,
		nodeAddr:            nodeAddr,
		priority:            priority,
		etcdClient:          etcdClient,
		stateManager:        stateManager,
		queue:               queue,
		config:              config,
		logger:              logger,
		healthCheckInterval: config.HealthCheckInterval,
		splitBrainTTL:       config.SplitBrainTTL,
		leadershipLeaseTTL:  config.LeadershipTTL,
		nodes:               make(map[string]*Node),
		stopCh:              make(chan struct{}),
	}

	atomic.StoreInt32(&fm.role, int32(NodeRoleStandby))
	atomic.StoreInt32(&fm.status, int32(NodeStatusHealthy))

	return fm, nil
}

// SetCallbacks sets callback functions for failover events.
func (fm *FailoverManager) SetCallbacks(
	onBecomeLeader func() error,
	onBecomeStandby func() error,
	onLeaderChange func(string),
) {
	fm.onBecomeLeader = onBecomeLeader
	fm.onBecomeStandby = onBecomeStandby
	fm.onLeaderChange = onLeaderChange
}

// Start starts leader election and health monitoring for failover.
func (fm *FailoverManager) Start(ctx context.Context) error {
	fm.stopMu.Lock()
	if fm.stopped {
		fm.stopMu.Unlock()
		return fmt.Errorf("failover manager already stopped")
	}
	fm.stopMu.Unlock()

	fm.logger.Infow("Starting failover manager",
		"node_id", fm.nodeID,
		"priority", fm.priority,
		"etcd_endpoints", fm.config.EtcdEndpoints,
	)

	// Create session for leader election
	session, err := concurrency.NewSession(fm.etcdClient,
		concurrency.WithTTL(int(fm.leadershipLeaseTTL.Seconds())))
	if err != nil {
		return fmt.Errorf("failed to create etcd session: %w", err)
	}
	fm.session = session

	// Create election
	fm.election = concurrency.NewElection(session, "/failover/leader")

	// Start health monitoring
	go fm.startHealthMonitoring(ctx)

	// Start leader election
	go fm.startLeaderElection(ctx)

	// Register this node
	if err := fm.registerNode(ctx); err != nil {
		fm.logger.Errorw("Failed to register node", "error", err)
	}

	return nil
}

// IsLeader returns true if this node is currently the leader.
func (fm *FailoverManager) IsLeader() bool {
	return NodeRole(atomic.LoadInt32(&fm.role)) == NodeRoleLeader
}

// GetLeader returns the current leader node ID.
func (fm *FailoverManager) GetLeader() string {
	fm.leaderMu.RLock()
	defer fm.leaderMu.RUnlock()
	return fm.leaderID
}

// GetRole returns the current role of this node.
func (fm *FailoverManager) GetRole() NodeRole {
	return NodeRole(atomic.LoadInt32(&fm.role))
}

// GetStatus returns the current status of this node.
func (fm *FailoverManager) GetStatus() NodeStatus {
	return NodeStatus(atomic.LoadInt32(&fm.status))
}

// Failover triggers a manual failover to a standby processor.
func (fm *FailoverManager) Failover(ctx context.Context) error {
	failoverStart := time.Now()
	
	fm.logger.Warnw("Manual failover triggered")
	
	// Check if we're the leader
	if !fm.IsLeader() {
		return fmt.Errorf("only leader can trigger failover")
	}
	
	// Step down from leadership
	if err := fm.stepDown(ctx); err != nil {
		return fmt.Errorf("failed to step down from leadership: %w", err)
	}
	
	// Wait for new leader election
	timeout := time.After(fm.config.MaxFailoverTime)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-timeout:
			return fmt.Errorf("failover timeout exceeded")
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			newLeader := fm.GetLeader()
			if newLeader != "" && newLeader != fm.nodeID {
				duration := time.Since(failoverStart)
				fm.logger.Infow("Manual failover completed",
					"new_leader", newLeader,
					"duration", duration,
				)
				return nil
			}
		}
	}
}

// ForceFailover forces an immediate failover without leadership checks.
func (fm *FailoverManager) ForceFailover(ctx context.Context) error {
	fm.logger.Errorw("Force failover triggered - emergency procedure")
	
	// Mark current leader as failed
	if err := fm.markNodeFailed(ctx, fm.GetLeader()); err != nil {
		fm.logger.Errorw("Failed to mark leader as failed", "error", err)
	}
	
	// If we're not leader, try to become leader
	if !fm.IsLeader() {
		return fm.becomeLeader(ctx)
	}
	
	return nil
}

// GetClusterState returns the current state of all nodes in the cluster.
func (fm *FailoverManager) GetClusterState() map[string]*Node {
	fm.nodesMu.RLock()
	defer fm.nodesMu.RUnlock()
	
	nodes := make(map[string]*Node)
	for id, node := range fm.nodes {
		// Deep copy to avoid races
		nodes[id] = &Node{
			ID:       node.ID,
			Address:  node.Address,
			Role:     node.Role,
			Status:   node.Status,
			LastSeen: node.LastSeen,
			Priority: node.Priority,
		}
	}
	
	return nodes
}

// GetMetrics returns failover performance metrics.
func (fm *FailoverManager) GetMetrics() FailoverMetrics {
	// Implementation would track metrics over time
	// Simplified for this example
	return FailoverMetrics{
		FailoverCount:        0, // Would be tracked
		LastFailoverTime:     time.Time{},
		LastFailoverDuration: 0,
		LeadershipChanges:    0,
		SplitBrainEvents:     0,
		AverageFailoverTime:  0,
		HealthCheckFailures:  0,
	}
}

// Shutdown gracefully shuts down the failover manager.
func (fm *FailoverManager) Shutdown(ctx context.Context) error {
	fm.stopMu.Lock()
	if fm.stopped {
		fm.stopMu.Unlock()
		return nil
	}
	fm.stopped = true
	fm.stopMu.Unlock()

	fm.logger.Info("Shutting down failover manager")

	// Signal stop
	close(fm.stopCh)

	// Step down if leader
	if fm.IsLeader() {
		if err := fm.stepDown(ctx); err != nil {
			fm.logger.Errorw("Failed to step down during shutdown", "error", err)
		}
	}

	// Unregister node
	if err := fm.unregisterNode(ctx); err != nil {
		fm.logger.Errorw("Failed to unregister node", "error", err)
	}

	// Close session and client
	if fm.session != nil {
		fm.session.Close()
	}
	if fm.etcdClient != nil {
		fm.etcdClient.Close()
	}

	fm.logger.Info("Failover manager shutdown complete")
	return nil
}

// Private methods

func (fm *FailoverManager) startLeaderElection(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-fm.stopCh:
			return
		default:
			if err := fm.runLeaderElection(ctx); err != nil {
				fm.logger.Errorw("Leader election failed", "error", err)
				time.Sleep(time.Second * 5) // Backoff
			}
		}
	}
}

func (fm *FailoverManager) runLeaderElection(ctx context.Context) error {
	// Campaign for leadership
	fm.logger.Debug("Campaigning for leadership")
	atomic.StoreInt32(&fm.role, int32(NodeRoleCandidate))
	
	if err := fm.election.Campaign(ctx, fm.nodeID); err != nil {
		atomic.StoreInt32(&fm.role, int32(NodeRoleStandby))
		return fmt.Errorf("failed to campaign for leadership: %w", err)
	}

	// Became leader
	fm.logger.Infow("Became cluster leader")
	if err := fm.becomeLeader(ctx); err != nil {
		return fmt.Errorf("failed to become leader: %w", err)
	}

	// Watch for leadership changes
	watchCh := fm.election.Observe(ctx)
	for resp := range watchCh {
		if resp.Header != nil {
			leaderID := string(resp.Kvs[0].Value)
			fm.onLeadershipChange(leaderID)
		}
	}

	return nil
}

func (fm *FailoverManager) becomeLeader(ctx context.Context) error {
	atomic.StoreInt32(&fm.role, int32(NodeRoleLeader))
	
	fm.leaderMu.Lock()
	fm.leaderID = fm.nodeID
	fm.leaderMu.Unlock()

	// Execute leader callback
	if fm.onBecomeLeader != nil {
		if err := fm.onBecomeLeader(); err != nil {
			fm.logger.Errorw("Leader callback failed", "error", err)
			return err
		}
	}

	fm.logger.Info("Successfully became leader")
	return nil
}

func (fm *FailoverManager) stepDown(ctx context.Context) error {
	fm.logger.Info("Stepping down from leadership")
	
	// Resign from election
	if err := fm.election.Resign(ctx); err != nil {
		return fmt.Errorf("failed to resign from election: %w", err)
	}
	
	atomic.StoreInt32(&fm.role, int32(NodeRoleStandby))
	
	// Execute standby callback
	if fm.onBecomeStandby != nil {
		if err := fm.onBecomeStandby(); err != nil {
			fm.logger.Errorw("Standby callback failed", "error", err)
		}
	}
	
	return nil
}

func (fm *FailoverManager) onLeadershipChange(newLeaderID string) {
	fm.leaderMu.Lock()
	oldLeaderID := fm.leaderID
	fm.leaderID = newLeaderID
	fm.leaderMu.Unlock()
	
	if oldLeaderID != newLeaderID {
		fm.logger.Infow("Leadership changed",
			"old_leader", oldLeaderID,
			"new_leader", newLeaderID,
		)
		
		// If we're no longer leader, become standby
		if oldLeaderID == fm.nodeID && newLeaderID != fm.nodeID {
			atomic.StoreInt32(&fm.role, int32(NodeRoleStandby))
			if fm.onBecomeStandby != nil {
				if err := fm.onBecomeStandby(); err != nil {
					fm.logger.Errorw("Standby callback failed", "error", err)
				}
			}
		}
		
		// Execute leader change callback
		if fm.onLeaderChange != nil {
			fm.onLeaderChange(newLeaderID)
		}
	}
}

func (fm *FailoverManager) startHealthMonitoring(ctx context.Context) {
	ticker := time.NewTicker(fm.healthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-fm.stopCh:
			return
		case <-ticker.C:
			fm.performHealthCheck(ctx)
		}
	}
}

func (fm *FailoverManager) performHealthCheck(ctx context.Context) {
	// Update our own health status
	status := fm.checkOwnHealth()
	atomic.StoreInt32(&fm.status, int32(status))
	
	// Update node registration
	if err := fm.updateNodeHealth(ctx); err != nil {
		fm.logger.Errorw("Failed to update node health", "error", err)
	}
	
	// Check other nodes
	fm.checkClusterHealth(ctx)
}

func (fm *FailoverManager) checkOwnHealth() NodeStatus {
	// Perform health checks
	// This would check queue connectivity, state manager health, etc.
	return NodeStatusHealthy
}

func (fm *FailoverManager) checkClusterHealth(ctx context.Context) {
	// Get all registered nodes
	resp, err := fm.etcdClient.Get(ctx, "/failover/nodes/", clientv3.WithPrefix())
	if err != nil {
		fm.logger.Errorw("Failed to get cluster nodes", "error", err)
		return
	}
	
	now := time.Now()
	fm.nodesMu.Lock()
	defer fm.nodesMu.Unlock()
	
	// Clear old nodes
	fm.nodes = make(map[string]*Node)
	
	// Parse node information
	for _, kv := range resp.Kvs {
		var node Node
		if err := json.Unmarshal(kv.Value, &node); err != nil {
			fm.logger.Errorw("Failed to unmarshal node data", "error", err)
			continue
		}
		
		// Check if node is stale
		if now.Sub(node.LastSeen) > fm.config.HealthCheckInterval*3 {
			node.Status = NodeStatusUnhealthy
		}
		
		fm.nodes[node.ID] = &node
	}
}

func (fm *FailoverManager) registerNode(ctx context.Context) error {
	node := &Node{
		ID:       fm.nodeID,
		Address:  fm.nodeAddr,
		Role:     fm.GetRole(),
		Status:   fm.GetStatus(),
		LastSeen: time.Now(),
		Priority: fm.priority,
	}
	
	data, err := json.Marshal(node)
	if err != nil {
		return fmt.Errorf("failed to marshal node data: %w", err)
	}
	
	key := fmt.Sprintf("/failover/nodes/%s", fm.nodeID)
	_, err = fm.etcdClient.Put(ctx, key, string(data))
	if err != nil {
		return fmt.Errorf("failed to register node: %w", err)
	}
	
	fm.logger.Infow("Registered node in cluster", "node_id", fm.nodeID)
	return nil
}

func (fm *FailoverManager) updateNodeHealth(ctx context.Context) error {
	node := &Node{
		ID:       fm.nodeID,
		Address:  fm.nodeAddr,
		Role:     fm.GetRole(),
		Status:   fm.GetStatus(),
		LastSeen: time.Now(),
		Priority: fm.priority,
	}
	
	data, err := json.Marshal(node)
	if err != nil {
		return fmt.Errorf("failed to marshal node data: %w", err)
	}
	
	key := fmt.Sprintf("/failover/nodes/%s", fm.nodeID)
	_, err = fm.etcdClient.Put(ctx, key, string(data))
	return err
}

func (fm *FailoverManager) unregisterNode(ctx context.Context) error {
	key := fmt.Sprintf("/failover/nodes/%s", fm.nodeID)
	_, err := fm.etcdClient.Delete(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to unregister node: %w", err)
	}
	
	fm.logger.Infow("Unregistered node from cluster", "node_id", fm.nodeID)
	return nil
}

func (fm *FailoverManager) markNodeFailed(ctx context.Context, nodeID string) error {
	key := fmt.Sprintf("/failover/nodes/%s", nodeID)
	resp, err := fm.etcdClient.Get(ctx, key)
	if err != nil {
		return err
	}
	
	if len(resp.Kvs) == 0 {
		return fmt.Errorf("node not found: %s", nodeID)
	}
	
	var node Node
	if err := json.Unmarshal(resp.Kvs[0].Value, &node); err != nil {
		return err
	}
	
	node.Status = NodeStatusFailed
	node.LastSeen = time.Now()
	
	data, err := json.Marshal(node)
	if err != nil {
		return err
	}
	
	_, err = fm.etcdClient.Put(ctx, key, string(data))
	return err
}
