package balancer

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

type RedisLocator struct {
	*ConsistentPartitioner

	client        *redis.Client
	partitionsKey string
	numPartitions int
}

func NewRedisLocator(client *redis.Client, numPartitions int) *RedisLocator {
	return &RedisLocator{
		ConsistentPartitioner: NewConsistentPartitioner(numPartitions),
		client:                client,
		partitionsKey:         "balancer:partitions",
		numPartitions:         numPartitions,
	}
}

// TotalPartitions returns the total number of partitions
func (rl *RedisLocator) TotalPartitions() int {
	return rl.numPartitions
}

func (rl *RedisLocator) LocateKey(ctx context.Context, key Key) (Node, error) {
	partition := rl.Partition(key)
	return rl.Node(ctx, partition)
}

func (rl *RedisLocator) Node(ctx context.Context, partition Partition) (Node, error) {
	result := rl.client.HGet(ctx, rl.partitionsKey, fmt.Sprintf("%d", partition))
	if result.Err() != nil {
		if result.Err() == redis.Nil {
			return "", fmt.Errorf("no node found for partition %d", partition)
		}
		return "", result.Err()
	}
	return result.Val(), nil
}
