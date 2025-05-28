package recovery

import (
	"context"
	"errors"
	"sync"

	"github.com/go-redis/redis/v8"
)

// RecoveryManager handles state recovery and failover.
type RecoveryManager struct {
	redisClient *redis.Client
	leader      bool
	leaderMu    sync.RWMutex
}

// NewRecoveryManager creates a new RecoveryManager with Redis connection.
func NewRecoveryManager(redisAddr, redisPass string, db int) *RecoveryManager {
	client := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: redisPass,
		DB:       db,
	})
	return &RecoveryManager{redisClient: client}
}

// RecoverState loads state from Redis snapshot.
func (rm *RecoveryManager) RecoverState(ctx context.Context, key string, dest interface{}) error {
	_, err := rm.redisClient.Get(ctx, key).Bytes()
	if err != nil {
		return err
	}
	// Unmarshal data into dest (assume gob or json, user to implement)
	return errors.New("unmarshal logic not implemented")
}

// SaveState saves state to Redis snapshot.
func (rm *RecoveryManager) SaveState(ctx context.Context, key string, src []byte) error {
	return rm.redisClient.Set(ctx, key, src, 0).Err()
}

// ElectLeader attempts leader election using Redis SETNX.
func (rm *RecoveryManager) ElectLeader(ctx context.Context, electionKey, nodeID string) (bool, error) {
	ok, err := rm.redisClient.SetNX(ctx, electionKey, nodeID, 0).Result()
	rm.leaderMu.Lock()
	defer rm.leaderMu.Unlock()
	rm.leader = ok
	return ok, err
}

// IsLeader returns true if this node is the leader.
func (rm *RecoveryManager) IsLeader() bool {
	rm.leaderMu.RLock()
	defer rm.leaderMu.RUnlock()
	return rm.leader
}
