// types.go: Core types, enums, and interfaces for rate limiting
package ratelimit

import (
	"context"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"go.uber.org/zap"
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

// TODO: Add more types for metrics, admin API, etc.

// BypassConfig holds configuration for rate limit bypass rules
type BypassConfig struct {
	AllowPrivateIPs       bool `json:"allow_private_ips"`
	RequireAdminAuth      bool `json:"require_admin_auth"`
	AllowHealthChecks     bool `json:"allow_health_checks"`
	AllowInternalRequests bool `json:"allow_internal_requests"`
}

// RateLimitMetrics holds metrics for rate limiting
type RateLimitMetrics struct {
	TotalRequests    int64            `json:"total_requests"`
	AllowedRequests  int64            `json:"allowed_requests"`
	BlockedRequests  int64            `json:"blocked_requests"`
	BypassedRequests int64            `json:"bypassed_requests"`
	ErrorCount       int64            `json:"error_count"`
	ResponseTimes    []time.Duration  `json:"response_times"`
	AlgorithmMetrics map[string]int64 `json:"algorithm_metrics"`
	LastReset        time.Time        `json:"last_reset"`
}

// AdminAPIResponse represents a standardized admin API response
type AdminAPIResponse struct {
	Success   bool        `json:"success"`
	Message   string      `json:"message"`
	Data      interface{} `json:"data,omitempty"`
	Error     string      `json:"error,omitempty"`
	Timestamp time.Time   `json:"timestamp"`
	RequestID string      `json:"request_id,omitempty"`
}

// RateLimitStatus represents the current status of a rate limit
type RateLimitStatus struct {
	Key           string                 `json:"key"`
	Algorithm     string                 `json:"algorithm"`
	Config        *RateLimitConfig       `json:"config"`
	Current       int                    `json:"current"`
	Remaining     int                    `json:"remaining"`
	ResetTime     time.Time              `json:"reset_time"`
	IsBlocked     bool                   `json:"is_blocked"`
	NextAllowTime *time.Time             `json:"next_allow_time,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

// ConfigUpdate represents a configuration update request
type ConfigUpdate struct {
	Name        string         `json:"name"`
	Enabled     *bool          `json:"enabled,omitempty"`
	Limit       *int           `json:"limit,omitempty"`
	Window      *time.Duration `json:"window,omitempty"`
	Burst       *int           `json:"burst,omitempty"`
	Description *string        `json:"description,omitempty"`
}

// ConfigManager manages all rate limit configs and live reload
// Supports per-user, per-IP, per-route, global, etc.
type ConfigManager struct {
	Configs   map[string]*RateLimitConfig `json:"configs"`
	Mu        sync.RWMutex                `json:"-"`
	Watcher   *fsnotify.Watcher           `json:"-"`
	ConfigDir string                      `json:"config_dir"`
	Logger    *zap.Logger                 `json:"-"`
	Callbacks []ConfigChangeCallback      `json:"-"`
}

// ConfigChangeCallback is called when configuration changes
type ConfigChangeCallback func(key string, old, new *RateLimitConfig)
