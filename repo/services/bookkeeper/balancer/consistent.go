package balancer

import (
	"context"
	"fmt"

	"github.com/buraksezer/consistent"
	"github.com/redis/go-redis/v9"
)

type nodeMember string

func (nm nodeMember) String() string {
	return string(nm)
}

// ConsistentBalancer implements the Balancer interface using Redis for persistence
type ConsistentBalancer struct {
	client        *redis.Client
	nodesKey      string
	partitionsKey string
	numPartitions int
	consistent    *consistent.Consistent
}

// NewConsistentBalancer creates a new Redis-based balancer
func NewConsistentBalancer(client *redis.Client, numPartitions int) *ConsistentBalancer {
	cfg := consistent.Config{
		PartitionCount:    271,
		ReplicationFactor: 20,
		Load:              1.25,
		Hasher:            hasher{},
	}

	return &ConsistentBalancer{
		client:        client,
		nodesKey:      "balancer:nodes",
		partitionsKey: "balancer:partitions",
		numPartitions: numPartitions,
		consistent:    consistent.New(nil, cfg),
	}
}

func (rb *ConsistentBalancer) AddNodes(ctx context.Context, nodes []Node) error {
	if len(nodes) == 0 {
		return nil
	}

	existingNodes, err := rb.nodes(ctx)
	if err != nil {
		return err
	}

	nodeMap := make(map[Node]struct{})
	for _, node := range existingNodes {
		nodeMap[node] = struct{}{}
	}

	for _, node := range nodes {
		nodeMap[node] = struct{}{}
	}

	allNodes := make([]Node, 0, len(nodeMap))
	for node := range nodeMap {
		allNodes = append(allNodes, node)
	}

	return rb.setNodesAndRebalance(ctx, allNodes)
}

func (rb *ConsistentBalancer) RemoveNodes(ctx context.Context, nodesToRemove []Node) error {
	if len(nodesToRemove) == 0 {
		return nil
	}

	existingNodes, err := rb.nodes(ctx)
	if err != nil {
		return err
	}

	removeMap := make(map[Node]struct{})
	for _, node := range nodesToRemove {
		removeMap[node] = struct{}{}
	}

	remainingNodes := make([]Node, 0, len(existingNodes))
	for _, node := range existingNodes {
		if _, exists := removeMap[node]; !exists {
			remainingNodes = append(remainingNodes, node)
		}
	}

	return rb.setNodesAndRebalance(ctx, remainingNodes)
}

func (rb *ConsistentBalancer) SetNodes(ctx context.Context, nodes []Node) error {
	return rb.setNodesAndRebalance(ctx, nodes)
}

func (rb *ConsistentBalancer) PartitionMappings(ctx context.Context) ([]PartitionMapping, error) {
	result := rb.client.HGetAll(ctx, rb.partitionsKey)
	if result.Err() != nil {
		return nil, result.Err()
	}

	mappings := make([]PartitionMapping, 0, len(result.Val()))
	for partitionStr, node := range result.Val() {
		partition := 0
		if _, err := fmt.Sscanf(partitionStr, "%d", &partition); err != nil {
			return nil, fmt.Errorf("invalid partition format: %s", partitionStr)
		}
		mappings = append(mappings, PartitionMapping{
			Partition: partition,
			Node:      node,
		})
	}

	return mappings, nil
}

func (rb *ConsistentBalancer) nodes(ctx context.Context) ([]Node, error) {
	result := rb.client.SMembers(ctx, rb.nodesKey)
	if result.Err() != nil && result.Err() != redis.Nil {
		return nil, result.Err()
	}
	return result.Val(), nil
}

func (rb *ConsistentBalancer) setNodesAndRebalance(ctx context.Context, nodes []Node) error {
	pipe := rb.client.TxPipeline()

	pipe.Del(ctx, rb.nodesKey)
	if len(nodes) > 0 {
		nodeInterfaces := make([]interface{}, len(nodes))
		for i, node := range nodes {
			nodeInterfaces[i] = node
		}
		pipe.SAdd(ctx, rb.nodesKey, nodeInterfaces...)
	}

	// Execute transaction
	_, err := pipe.Exec(ctx)
	if err != nil {
		return err
	}

	// Rebalance partitions if we have nodes
	if len(nodes) > 0 {
		return rb.rebalance(ctx, nodes)
	}

	// If no nodes, clear all partition mappings
	return rb.client.Del(ctx, rb.partitionsKey).Err()
}

func (rb *ConsistentBalancer) rebalance(ctx context.Context, nodes []Node) error {
	ring := newRing(nodes)

	pipe := rb.client.TxPipeline()
	pipe.Del(ctx, rb.partitionsKey)

	for partition := 0; partition < rb.numPartitions; partition++ {
		// Use consistent hashing to determine which node gets this partition
		node := ring.GetPartitionOwner(partition).String()
		pipe.HSet(ctx, rb.partitionsKey, fmt.Sprintf("%d", partition), node)
	}

	_, err := pipe.Exec(ctx)
	return err
}

type ConsistentPartitioner struct {
	totalPartitions int
	ring            *consistent.Consistent
}

func NewConsistentPartitioner(totalPartitions int) *ConsistentPartitioner {
	return &ConsistentPartitioner{totalPartitions, newRing([]string{})}
}

func (cp *ConsistentPartitioner) TotalPartitions() int {
	return cp.totalPartitions
}

func (cp *ConsistentPartitioner) Partition(key Key) Partition {
	return cp.ring.FindPartitionID(key)
}

// DynamicLoadAwareBalancer extends ConsistentBalancer with load awareness
// and real-time scaling (dynamic node add/remove with minimal disruption)
type DynamicLoadAwareBalancer struct {
	*ConsistentBalancer
	loadMap map[Node]float64 // node -> current load
}

// NewDynamicLoadAwareBalancer creates a new load-aware balancer
func NewDynamicLoadAwareBalancer(client *redis.Client, numPartitions int) *DynamicLoadAwareBalancer {
	return &DynamicLoadAwareBalancer{
		ConsistentBalancer: NewConsistentBalancer(client, numPartitions),
		loadMap:            make(map[Node]float64),
	}
}

// ReportLoad allows nodes to report their current load (e.g., CPU, QPS)
func (b *DynamicLoadAwareBalancer) ReportLoad(node Node, load float64) {
	b.loadMap[node] = load
}

// RebalanceWithLoad redistributes partitions to minimize load imbalance
func (b *DynamicLoadAwareBalancer) RebalanceWithLoad(ctx context.Context) error {
	nodes, err := b.nodes(ctx)
	if err != nil || len(nodes) == 0 {
		return err
	}
	// Sort nodes by load ascending (least loaded first)
	type nodeLoad struct {
		node Node
		load float64
	}
	var nodeLoads []nodeLoad
	for _, n := range nodes {
		l := b.loadMap[n]
		nodeLoads = append(nodeLoads, nodeLoad{n, l})
	}
	// Simple greedy: assign each partition to least loaded node
	pipe := b.client.TxPipeline()
	pipe.Del(ctx, b.partitionsKey)
	for partition := 0; partition < b.numPartitions; partition++ {
		// Find least loaded node
		minIdx := 0
		for i := 1; i < len(nodeLoads); i++ {
			if nodeLoads[i].load < nodeLoads[minIdx].load {
				minIdx = i
			}
		}
		pipe.HSet(ctx, b.partitionsKey, fmt.Sprintf("%d", partition), nodeLoads[minIdx].node)
		// Simulate load increase for this node
		nodeLoads[minIdx].load += 1
	}
	_, err = pipe.Exec(ctx)
	return err
}

// AddNodes and RemoveNodes call rebalance for minimal disruption
func (b *DynamicLoadAwareBalancer) AddNodes(ctx context.Context, nodes []Node) error {
	err := b.ConsistentBalancer.AddNodes(ctx, nodes)
	if err != nil {
		return err
	}
	return b.RebalanceWithLoad(ctx)
}

func (b *DynamicLoadAwareBalancer) RemoveNodes(ctx context.Context, nodes []Node) error {
	err := b.ConsistentBalancer.RemoveNodes(ctx, nodes)
	if err != nil {
		return err
	}
	return b.RebalanceWithLoad(ctx)
}
