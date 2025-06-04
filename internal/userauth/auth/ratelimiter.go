package auth

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/Aidin1998/pincex_unified/pkg/models"
	"github.com/redis/go-redis/v9"
)

// RedisRateLimiter implements rate limiting using Redis
type RedisRateLimiter struct {
	client *redis.Client
}

// NewRedisRateLimiter creates a new Redis-based rate limiter
func NewRedisRateLimiter(client *redis.Client) RateLimiter {
	return &RedisRateLimiter{client: client}
}

// Allow checks if a request should be allowed based on rate limiting rules
func (rl *RedisRateLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	// Use sliding window rate limiting algorithm
	now := time.Now()
	windowStart := now.Add(-window)

	// Redis key for this rate limit
	redisKey := fmt.Sprintf("rate_limit:%s", key)

	// Use Redis pipeline for atomic operations
	pipe := rl.client.Pipeline()

	// Remove old entries outside the window
	pipe.ZRemRangeByScore(ctx, redisKey, "0", fmt.Sprintf("%d", windowStart.UnixNano()))

	// Count current entries in window
	countCmd := pipe.ZCard(ctx, redisKey)

	// Execute pipeline
	_, err := pipe.Exec(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to execute rate limit check: %w", err)
	}

	currentCount := countCmd.Val()

	// Check if limit is exceeded
	if currentCount >= int64(limit) {
		return false, nil
	}
	// Add current request to the window
	score := now.UnixNano()
	member := fmt.Sprintf("%d", score)

	pipe = rl.client.Pipeline()
	pipe.ZAdd(ctx, redisKey, redis.Z{Score: float64(score), Member: member})
	pipe.Expire(ctx, redisKey, window+time.Minute) // Add buffer to expiration

	_, err = pipe.Exec(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to record rate limit entry: %w", err)
	}

	return true, nil
}

// InMemoryRateLimiter implements rate limiting using in-memory storage
type InMemoryRateLimiter struct {
	entries map[string][]time.Time
}

// NewInMemoryRateLimiter creates a new in-memory rate limiter
func NewInMemoryRateLimiter() RateLimiter {
	return &InMemoryRateLimiter{
		entries: make(map[string][]time.Time),
	}
}

// Allow checks if a request should be allowed (in-memory implementation)
func (rl *InMemoryRateLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	now := time.Now()
	windowStart := now.Add(-window)

	// Get existing entries
	entries, exists := rl.entries[key]
	if !exists {
		entries = make([]time.Time, 0)
	}

	// Remove old entries
	validEntries := make([]time.Time, 0)
	for _, entry := range entries {
		if entry.After(windowStart) {
			validEntries = append(validEntries, entry)
		}
	}

	// Check if limit is exceeded
	if len(validEntries) >= limit {
		return false, nil
	}

	// Add current request
	validEntries = append(validEntries, now)
	rl.entries[key] = validEntries

	return true, nil
}

// AdvancedRateLimiter provides more sophisticated rate limiting
type AdvancedRateLimiter struct {
	client *redis.Client
}

// NewAdvancedRateLimiter creates a new advanced rate limiter
func NewAdvancedRateLimiter(client *redis.Client) *AdvancedRateLimiter {
	return &AdvancedRateLimiter{client: client}
}

// AllowWithBurst allows burst requests with token bucket algorithm
func (rl *AdvancedRateLimiter) AllowWithBurst(ctx context.Context, key string, burstLimit int, refillRate float64) (bool, error) {
	redisKey := fmt.Sprintf("token_bucket:%s", key)
	now := time.Now()

	// Lua script for atomic token bucket operations
	luaScript := `
		local key = KEYS[1]
		local burst_limit = tonumber(ARGV[1])
		local refill_rate = tonumber(ARGV[2])
		local now = tonumber(ARGV[3])
		
		local bucket = redis.call('HMGET', key, 'tokens', 'last_refill')
		local tokens = tonumber(bucket[1]) or burst_limit
		local last_refill = tonumber(bucket[2]) or now
		
		-- Calculate tokens to add based on time elapsed
		local time_passed = (now - last_refill) / 1000000000  -- Convert nanoseconds to seconds
		local tokens_to_add = time_passed * refill_rate
		tokens = math.min(burst_limit, tokens + tokens_to_add)
		
		-- Check if we can consume a token
		if tokens >= 1 then
			tokens = tokens - 1
			redis.call('HMSET', key, 'tokens', tokens, 'last_refill', now)
			redis.call('EXPIRE', key, 3600)  -- Expire after 1 hour of inactivity
			return 1  -- Allow
		else
			redis.call('HMSET', key, 'tokens', tokens, 'last_refill', now)
			redis.call('EXPIRE', key, 3600)
			return 0  -- Deny
		end
	`

	result, err := rl.client.Eval(ctx, luaScript, []string{redisKey},
		burstLimit, refillRate, now.UnixNano()).Result()
	if err != nil {
		return false, fmt.Errorf("failed to execute token bucket script: %w", err)
	}

	return result.(int64) == 1, nil
}

// AllowWithMultipleLimits checks multiple rate limits simultaneously
func (rl *AdvancedRateLimiter) AllowWithMultipleLimits(ctx context.Context, key string, limits []RateLimit) (bool, error) {
	for _, limit := range limits {
		allowed, err := rl.Allow(ctx, fmt.Sprintf("%s:%s", key, limit.Name), limit.Limit, limit.Window)
		if err != nil {
			return false, fmt.Errorf("failed to check rate limit %s: %w", limit.Name, err)
		}
		if !allowed {
			return false, nil
		}
	}
	return true, nil
}

// Allow implements the basic RateLimiter interface
func (rl *AdvancedRateLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	return NewRedisRateLimiter(rl.client).Allow(ctx, key, limit, window)
}

// RateLimit represents a rate limiting rule
type RateLimit struct {
	Name   string        `json:"name"`
	Limit  int           `json:"limit"`
	Window time.Duration `json:"window"`
}

// GetRemainingRequests returns the number of remaining requests in the current window
func (rl *AdvancedRateLimiter) GetRemainingRequests(ctx context.Context, key string, limit int, window time.Duration) (int, error) {
	redisKey := fmt.Sprintf("rate_limit:%s", key)
	windowStart := time.Now().Add(-window)

	// Count current entries in window
	count, err := rl.client.ZCount(ctx, redisKey, fmt.Sprintf("%d", windowStart.UnixNano()), "+inf").Result()
	if err != nil {
		return 0, fmt.Errorf("failed to get current count: %w", err)
	}

	remaining := limit - int(count)
	if remaining < 0 {
		remaining = 0
	}

	return remaining, nil
}

// GetRateLimitInfo returns detailed rate limit information
func (rl *AdvancedRateLimiter) GetRateLimitInfo(ctx context.Context, key string, limit int, window time.Duration) (*models.RateLimitInfo, error) {
	redisKey := fmt.Sprintf("rate_limit:%s", key)
	now := time.Now()
	windowStart := now.Add(-window)

	// Get all entries in current window
	entries, err := rl.client.ZRangeByScore(ctx, redisKey, &redis.ZRangeBy{
		Min: fmt.Sprintf("%d", windowStart.UnixNano()),
		Max: "+inf",
	}).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get rate limit entries: %w", err)
	}

	currentCount := len(entries)
	remaining := limit - currentCount
	if remaining < 0 {
		remaining = 0
	}

	var resetTime time.Time
	if len(entries) > 0 {
		// Parse the oldest entry to calculate reset time
		oldestScore, _ := strconv.ParseInt(entries[0], 10, 64)
		oldestTime := time.Unix(0, oldestScore)
		resetTime = oldestTime.Add(window)
	} else {
		resetTime = now.Add(window)
	}

	return &models.RateLimitInfo{
		Limit:     limit,
		Used:      currentCount,
		Remaining: remaining,
		ResetAt:   resetTime,
		Window:    window,
	}, nil
}

// CleanupRateLimitData removes old rate limit data
func (rl *AdvancedRateLimiter) CleanupRateLimitData(ctx context.Context, maxAge time.Duration) error {
	// Use Redis SCAN to find rate limit keys
	iter := rl.client.Scan(ctx, 0, "rate_limit:*", 0).Iterator()
	for iter.Next(ctx) {
		key := iter.Val()

		// Remove entries older than maxAge
		cutoff := time.Now().Add(-maxAge)
		_, err := rl.client.ZRemRangeByScore(ctx, key, "0", fmt.Sprintf("%d", cutoff.UnixNano())).Result()
		if err != nil {
			return fmt.Errorf("failed to cleanup rate limit data for key %s: %w", key, err)
		}

		// Remove empty sets
		count, err := rl.client.ZCard(ctx, key).Result()
		if err != nil {
			continue
		}
		if count == 0 {
			rl.client.Del(ctx, key)
		}
	}

	return iter.Err()
}

// UserRateLimiter provides user-specific rate limiting
type UserRateLimiter struct {
	*AdvancedRateLimiter
}

// NewUserRateLimiter creates a user-specific rate limiter
func NewUserRateLimiter(client *redis.Client) *UserRateLimiter {
	return &UserRateLimiter{
		AdvancedRateLimiter: NewAdvancedRateLimiter(client),
	}
}

// CheckUserRateLimit checks rate limits for a specific user and endpoint
func (rl *UserRateLimiter) CheckUserRateLimit(ctx context.Context, userID, endpoint string) (bool, *models.RateLimitInfo, error) {
	// Define rate limits per endpoint
	limits := map[string]RateLimit{
		"login": {
			Name:   "login_attempts",
			Limit:  5,
			Window: 15 * time.Minute,
		},
		"api_calls": {
			Name:   "api_calls",
			Limit:  1000,
			Window: time.Hour,
		},
		"order_placement": {
			Name:   "order_placement",
			Limit:  100,
			Window: time.Minute,
		},
		"withdrawal": {
			Name:   "withdrawal",
			Limit:  10,
			Window: 24 * time.Hour,
		},
	}

	limit, exists := limits[endpoint]
	if !exists {
		// Default rate limit
		limit = RateLimit{
			Name:   "default",
			Limit:  100,
			Window: time.Minute,
		}
	}

	key := fmt.Sprintf("user:%s:%s", userID, endpoint)
	allowed, err := rl.Allow(ctx, key, limit.Limit, limit.Window)
	if err != nil {
		return false, nil, err
	}

	info, err := rl.GetRateLimitInfo(ctx, key, limit.Limit, limit.Window)
	if err != nil {
		return allowed, nil, err
	}

	return allowed, info, nil
}
