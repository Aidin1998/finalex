// types.go: Core types, enums, and interfaces for rate limiting
package ratelimit

import (
	"context"
	"time"
)

// LimiterType enumerates supported rate limiting algorithms
const (
	LimiterTokenBucket   = "token_bucket"
	LimiterSlidingWindow = "sliding_window"
)

// RateLimitConfig holds config for a single rate limit rule
// (per user, per IP, per API key, per route, etc)
type RateLimitConfig struct {
	Name        string
	Type        string // LimiterType
	Limit       int
	Window      time.Duration
	Burst       int // for token bucket
	Enabled     bool
	Description string
}

// Limiter is the interface for all rate limiters
// (TokenBucket, SlidingWindow, etc)
type Limiter interface {
	Allow(ctx context.Context, key string, n int) (allowed bool, retryAfter time.Duration, headers map[string]string, err error)
	Peek(ctx context.Context, key string) (remaining int, reset time.Time, err error)
}

// DistributedStore abstracts the backend (e.g., Redis)
type DistributedStore interface {
	Get(ctx context.Context, key string) (val []byte, err error)
	Set(ctx context.Context, key string, val []byte, ttl time.Duration) error
	EvalScript(ctx context.Context, script string, keys []string, args ...interface{}) (interface{}, error)
}
