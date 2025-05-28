package balancer

import (
	"hash/fnv"

	"github.com/buraksezer/consistent"
)

func newRing(nodes []Node) *consistent.Consistent {
	cfg := consistent.Config{
		PartitionCount:    271,
		ReplicationFactor: 20,
		Load:              1.25,
		Hasher:            hasher{},
	}

	members := make([]consistent.Member, len(nodes))
	for i, node := range nodes {
		members[i] = nodeMember(node)
	}

	return consistent.New(members, cfg)
}

type hasher struct{}

func (h hasher) Sum64(data []byte) uint64 {
	fnvHash := fnv.New64a()
	fnvHash.Write(data)
	return fnvHash.Sum64()
}
