package balancer

import (
	"context"
)

type (
	Key       = []byte
	Node      = string
	Partition = int
)

type PartitionMapping struct {
	Partition Partition
	Node      Node
}

// TODO: For now  balancer is not load aware. For more optimum load balancing
// balancer should be aware of load on each partitions.
type Balancer interface {
	AddNodes(ctx context.Context, nodes []Node) error
	RemoveNodes(ctx context.Context, node []Node) error
	SetNodes(ctx context.Context, nodes []Node) error
	PartitionMappings(ctx context.Context) ([]PartitionMapping, error)
}

type Locator interface {
	Partitioner

	LocateKey(ctx context.Context, key Key) (Node, error)
	Node(ctx context.Context, partition Partition) (Node, error)
}

type Partitioner interface {
	TotalPartitions() int
	Partition(key Key) Partition
}
