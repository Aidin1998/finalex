// redis.go: Redis integration for distributed rate limiting (token bucket and sliding window)
package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisClient wraps go-redis for dependency injection and testability
// (can be extended for cluster/sharded setups)
type RedisClient struct {
	Client *redis.Client
}

// NewRedisClient creates a new Redis client from options
func NewRedisClient(addr, password string, db int) *RedisClient {
	return &RedisClient{
		Client: redis.NewClient(&redis.Options{
			Addr:     addr,
			Password: password,
			DB:       db,
		}),
	}
}

// --- Distributed Token Bucket ---
// Uses a Lua script for atomicity
var tokenBucketScript = redis.NewScript(`
local key = KEYS[1]
local capacity = tonumber(ARGV[1])
local refill_rate = tonumber(ARGV[2])
local now = tonumber(ARGV[3])
local requested = tonumber(ARGV[4])
local bucket = redis.call('HMGET', key, 'tokens', 'last')
local tokens = tonumber(bucket[1]) or capacity
local last = tonumber(bucket[2]) or now
local delta = math.max(0, now - last)
local refill = delta * refill_rate
local new_tokens = math.min(capacity, tokens + refill)
if new_tokens < requested then
  redis.call('HMSET', key, 'tokens', new_tokens, 'last', now)
  redis.call('EXPIRE', key, 60)
  return {0, new_tokens}
else
  new_tokens = new_tokens - requested
  redis.call('HMSET', key, 'tokens', new_tokens, 'last', now)
  redis.call('EXPIRE', key, 60)
  return {1, new_tokens}
end
`)

// TakeTokenBucket attempts to take n tokens from the distributed bucket
func (rc *RedisClient) TakeTokenBucket(ctx context.Context, key string, capacity int, refillRate float64, n int) (allowed bool, tokensLeft float64, err error) {
	now := time.Now().Unix()
	res, err := tokenBucketScript.Run(ctx, rc.Client, []string{key}, capacity, refillRate, now, n).Result()
	if err != nil {
		return false, 0, err
	}
	vals, ok := res.([]interface{})
	if !ok || len(vals) < 2 {
		return false, 0, fmt.Errorf("unexpected redis script result: %v", res)
	}
	allowedInt, _ := vals[0].(int64)
	tokensLeftF, _ := vals[1].(float64)
	return allowedInt == 1, tokensLeftF, nil
}

// --- Distributed Sliding Window ---
// Uses Redis sorted set for window, with Lua for atomicity
var slidingWindowScript = redis.NewScript(`
local key = KEYS[1]
local now = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local limit = tonumber(ARGV[3])
local n = tonumber(ARGV[4])
redis.call('ZREMRANGEBYSCORE', key, 0, now - window)
local count = redis.call('ZCARD', key)
if count + n > limit then
  return {0, count}
else
  for i=1,n do
    redis.call('ZADD', key, now, now + i)
  end
  redis.call('EXPIRE', key, math.ceil(window/1000000000))
  return {1, count + n}
end
`)

// TakeSlidingWindow attempts to record n requests in the distributed window
func (rc *RedisClient) TakeSlidingWindow(ctx context.Context, key string, window time.Duration, limit, n int) (allowed bool, count int64, err error) {
	now := time.Now().UnixNano()
	res, err := slidingWindowScript.Run(ctx, rc.Client, []string{key}, now, window.Nanoseconds(), limit, n).Result()
	if err != nil {
		return false, 0, err
	}
	vals, ok := res.([]interface{})
	if !ok || len(vals) < 2 {
		return false, 0, fmt.Errorf("unexpected redis script result: %v", res)
	}
	allowedInt, _ := vals[0].(int64)
	countInt, _ := vals[1].(int64)
	return allowedInt == 1, countInt, nil
}

// PeekSlidingWindow returns the number of requests in the window and the reset time
func (rc *RedisClient) PeekSlidingWindow(ctx context.Context, key string, window time.Duration) (count int64, reset time.Time, err error) {
	now := time.Now().UnixNano()
	min := now - window.Nanoseconds()
	z, err := rc.Client.ZRangeByScoreWithScores(ctx, key, &redis.ZRangeBy{
		Min: fmt.Sprintf("%d", min),
		Max: fmt.Sprintf("%d", now),
	}).Result()
	if err != nil {
		return 0, time.Time{}, err
	}
	count = int64(len(z))
	reset = time.Unix(0, now+window.Nanoseconds())
	return count, reset, nil
}

// --- Distributed Leaky Bucket ---
// Uses Redis sorted set with leak simulation
var leakyBucketScript = redis.NewScript(`
local key = KEYS[1]
local now = tonumber(ARGV[1])
local capacity = tonumber(ARGV[2])
local leak_rate = tonumber(ARGV[3])
local window_ns = tonumber(ARGV[4])
local requested = tonumber(ARGV[5])

-- Remove leaked items (items older than leak rate allows)
local leak_interval = window_ns / leak_rate
redis.call('ZREMRANGEBYSCORE', key, 0, now - leak_interval)

-- Get current queue size
local queue_size = redis.call('ZCARD', key)

-- Check if we can add the requested items
if queue_size + requested > capacity then
  return {0, queue_size}
else
  -- Add items to queue
  for i=1,requested do
    redis.call('ZADD', key, now + i, now + i)
  end
  redis.call('EXPIRE', key, math.ceil(window_ns/1000000000))
  return {1, queue_size + requested}
end
`)

// TakeLeakyBucket attempts to add items to the leaky bucket
func (rc *RedisClient) TakeLeakyBucket(ctx context.Context, key string, capacity, leakRate int, window time.Duration, n int) (allowed bool, queueSize int64, err error) {
	now := time.Now().UnixNano()
	res, err := leakyBucketScript.Run(ctx, rc.Client, []string{key}, now, capacity, leakRate, window.Nanoseconds(), n).Result()
	if err != nil {
		return false, 0, fmt.Errorf("leaky bucket script error: %w", err)
	}

	vals, ok := res.([]interface{})
	if !ok || len(vals) < 2 {
		return false, 0, fmt.Errorf("unexpected redis script result: %v", res)
	}

	allowedInt, _ := vals[0].(int64)
	queueSizeInt, _ := vals[1].(int64)
	return allowedInt == 1, queueSizeInt, nil
}

// Enhanced error handling and connection monitoring
func (rc *RedisClient) HealthCheck(ctx context.Context) error {
	return rc.Client.Ping(ctx).Err()
}

// GetConnectionStats returns Redis connection statistics
func (rc *RedisClient) GetConnectionStats() *redis.PoolStats {
	return rc.Client.PoolStats()
}

// Bulk operations for better performance
func (rc *RedisClient) BulkCheck(ctx context.Context, keys []string, algorithm string, config *RateLimitConfig) (map[string]bool, error) {
	results := make(map[string]bool)

	pipe := rc.Client.Pipeline()

	var cmders []redis.Cmder

	for _, key := range keys {
		switch algorithm {
		case LimiterTokenBucket:
			now := time.Now().Unix()
			refillRate := float64(config.Limit) / config.Window.Seconds()
			cmd := tokenBucketScript.Run(ctx, pipe, []string{key}, config.Burst, refillRate, now, 1)
			cmders = append(cmders, cmd)
		case LimiterSlidingWindow:
			now := time.Now().UnixNano()
			cmd := slidingWindowScript.Run(ctx, pipe, []string{key}, now, config.Window.Nanoseconds(), config.Limit, 1)
			cmders = append(cmders, cmd)
		}
	}

	_, err := pipe.Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("bulk check pipeline error: %w", err)
	}

	for i, key := range keys {
		if i < len(cmders) {
			res := cmders[i].(*redis.Cmd).Val()
			if vals, ok := res.([]interface{}); ok && len(vals) > 0 {
				if allowedInt, ok := vals[0].(int64); ok {
					results[key] = allowedInt == 1
				}
			}
		}
	}

	return results, nil
}
