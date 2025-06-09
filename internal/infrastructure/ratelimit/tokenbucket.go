// tokenbucket.go: Token bucket algorithm implementation
package ratelimit

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// TokenBucket implements a thread-safe, in-memory token bucket rate limiter.
type TokenBucket struct {
	capacity   int        // max tokens
	tokens     float64    // current tokens (float for partial refill)
	rate       float64    // tokens per second
	lastRefill time.Time  // last refill timestamp
	mu         sync.Mutex // protects state
}

// NewTokenBucket creates a new token bucket with the given capacity and refill rate (tokens per second).
func NewTokenBucket(capacity int, rate float64) *TokenBucket {
	return &TokenBucket{
		capacity:   capacity,
		tokens:     float64(capacity),
		rate:       rate,
		lastRefill: time.Now(),
	}
}

// Allow checks if a token can be consumed (does not consume).
func (tb *TokenBucket) Allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.refillLocked()
	return tb.tokens >= 1
}

// Take attempts to consume a token. Returns true if allowed, false if rate limited.
func (tb *TokenBucket) Take() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.refillLocked()
	if tb.tokens >= 1 {
		tb.tokens -= 1
		return true
	}
	return false
}

// refillLocked refills tokens based on elapsed time. Caller must hold tb.mu.
func (tb *TokenBucket) refillLocked() {
	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()
	if elapsed > 0 {
		tb.tokens += elapsed * tb.rate
		if tb.tokens > float64(tb.capacity) {
			tb.tokens = float64(tb.capacity)
		}
		tb.lastRefill = now
	}
}

// SetRate dynamically updates the refill rate (tokens/sec).
func (tb *TokenBucket) SetRate(rate float64) {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.refillLocked()
	tb.rate = rate
}

// SetCapacity dynamically updates the bucket capacity.
func (tb *TokenBucket) SetCapacity(cap int) {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.refillLocked()
	tb.capacity = cap
	if tb.tokens > float64(cap) {
		tb.tokens = float64(cap)
	}
}

// Remaining returns the number of tokens left (rounded down).
func (tb *TokenBucket) Remaining() int {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.refillLocked()
	return int(tb.tokens)
}

// Reset sets the bucket to full capacity.
func (tb *TokenBucket) Reset() {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.tokens = float64(tb.capacity)
	tb.lastRefill = time.Now()
}

// TokenBucket implements Limiter using the token bucket algorithm
// Supports distributed state via DistributedStore (e.g., Redis)
type DistributedTokenBucket struct {
	Config *RateLimitConfig
	Store  DistributedStore
}

// Allow checks if n tokens can be consumed, updates state if allowed
func (tb *DistributedTokenBucket) Allow(ctx context.Context, key string, n int) (bool, time.Duration, map[string]string, error) {
	refillRate := float64(tb.Config.Limit) / tb.Config.Window.Seconds()

	// Use Redis client to check and consume tokens
	if redisStore, ok := tb.Store.(*RedisClient); ok {
		allowed, tokensLeft, err := redisStore.TakeTokenBucket(ctx, key, tb.Config.Burst, refillRate, n)
		if err != nil {
			return false, 0, nil, fmt.Errorf("distributed token bucket error: %w", err)
		}

		headers := map[string]string{
			"X-RateLimit-Limit":     fmt.Sprintf("%d", tb.Config.Limit),
			"X-RateLimit-Remaining": fmt.Sprintf("%.0f", tokensLeft),
			"X-RateLimit-Reset":     fmt.Sprintf("%d", time.Now().Add(tb.Config.Window).Unix()),
		}

		var retryAfter time.Duration
		if !allowed {
			// Calculate retry-after based on refill rate
			tokensNeeded := float64(n) - tokensLeft
			secondsToWait := tokensNeeded / refillRate
			retryAfter = time.Duration(secondsToWait * float64(time.Second))
		}

		return allowed, retryAfter, headers, nil
	}

	return false, 0, nil, fmt.Errorf("unsupported distributed store type")
}

// Peek returns remaining tokens and reset time
func (tb *DistributedTokenBucket) Peek(ctx context.Context, key string) (int, time.Time, error) {
	refillRate := float64(tb.Config.Limit) / tb.Config.Window.Seconds()

	// Use Redis client to peek at token count
	if redisStore, ok := tb.Store.(*RedisClient); ok {
		tokensLeft, nextRefill, err := redisStore.PeekTokenBucket(ctx, key, tb.Config.Burst, refillRate)
		if err != nil {
			return 0, time.Time{}, fmt.Errorf("distributed token bucket peek error: %w", err)
		}

		return int(tokensLeft), nextRefill, nil
	}

	return 0, time.Time{}, fmt.Errorf("unsupported distributed store type")
}
