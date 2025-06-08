// Package ratelimit provides enhanced distributed, multi-tier rate limiting infrastructure for Orbit CEX.
//
// This enhanced version provides:
// - Global distributed rate limiting with Redis backend
// - Configurable limits (per-IP, per-user, per-endpoint, per-role)
// - Both leaky-bucket and token-bucket algorithms
// - Prometheus metrics integration
// - Fail-safe mechanisms
// - Comprehensive testing support
package ratelimit

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

var (
	tracer = otel.Tracer("ratelimit")

	// Prometheus metrics
	rateLimitRequests = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "orbit",
			Subsystem: "ratelimit",
			Name:      "requests_total",
			Help:      "Total number of rate limit checks",
		},
		[]string{"algorithm", "key_type", "status"},
	)

	rateLimitDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "orbit",
			Subsystem: "ratelimit",
			Name:      "check_duration_seconds",
			Help:      "Time spent checking rate limits",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"algorithm", "key_type"},
	)

	rateLimitActiveKeys = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "orbit",
			Subsystem: "ratelimit",
			Name:      "active_keys",
			Help:      "Number of active rate limit keys",
		},
		[]string{"algorithm", "key_type"},
	)

	rateLimitRedisErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "orbit",
			Subsystem: "ratelimit",
			Name:      "redis_errors_total",
			Help:      "Total number of Redis errors in rate limiting",
		},
		[]string{"operation", "error_type"},
	)
)

// EnhancedRateLimiter provides comprehensive rate limiting with fail-safe mechanisms
type EnhancedRateLimiter struct {
	redis         *RedisClient
	configManager *ConfigManager
	metrics       *MetricsCollector
	failureMode   string
	cleanupTicker *time.Ticker
	stopCleanup   chan bool
	mu            sync.RWMutex
	localCache    map[string]*LocalCacheEntry
	cacheExpiry   time.Duration
}

// LocalCacheEntry represents a local cache entry for fail-safe mode
type LocalCacheEntry struct {
	Count     int64
	ResetTime time.Time
	LastSeen  time.Time
}

// MetricsCollector collects and exports rate limiting metrics
type MetricsCollector struct {
	enabled    bool
	redis      *RedisClient
	mu         sync.RWMutex
	keyMetrics map[string]*KeyMetrics
}

// KeyMetrics tracks metrics for a specific rate limit key
type KeyMetrics struct {
	Requests   int64
	Allowed    int64
	Denied     int64
	LastAccess time.Time
}

// NewEnhancedRateLimiter creates a new enhanced rate limiter
func NewEnhancedRateLimiter(redisClient *RedisClient, configManager *ConfigManager, failureMode string) *EnhancedRateLimiter {
	limiter := &EnhancedRateLimiter{
		redis:         redisClient,
		configManager: configManager,
		metrics:       NewMetricsCollector(redisClient, true),
		failureMode:   failureMode,
		localCache:    make(map[string]*LocalCacheEntry),
		cacheExpiry:   5 * time.Minute,
		stopCleanup:   make(chan bool),
	}

	// Start cleanup goroutine
	limiter.cleanupTicker = time.NewTicker(time.Minute)
	go limiter.cleanupRoutine()

	return limiter
}

// Check performs a comprehensive rate limit check
func (erl *EnhancedRateLimiter) Check(ctx context.Context, r *http.Request) (allowed bool, retryAfter time.Duration, headers map[string]string, err error) {
	ctx, span := tracer.Start(ctx, "ratelimit.Check")
	defer span.End()

	start := time.Now()
	defer func() {
		duration := time.Since(start)
		rateLimitDuration.WithLabelValues("enhanced", "composite").Observe(duration.Seconds())
	}()

	// Extract rate limit keys
	keys := erl.extractKeys(r)
	headers = make(map[string]string)

	// Check all applicable rate limits
	for keyType, key := range keys {
		config := erl.configManager.GetConfig(key)
		if config == nil || !config.Enabled {
			continue
		}

		span.SetAttributes(
			attribute.String("key_type", keyType),
			attribute.String("algorithm", config.Type),
		)

		// Perform rate limit check
		keyAllowed, keyRetryAfter, keyHeaders, keyErr := erl.checkSingleKey(ctx, key, config, keyType)

		// Merge headers
		for k, v := range keyHeaders {
			headers[k] = v
		}

		// Update metrics
		status := "allowed"
		if !keyAllowed {
			status = "denied"
		}
		rateLimitRequests.WithLabelValues(config.Type, keyType, status).Inc()

		if keyErr != nil {
			rateLimitRedisErrors.WithLabelValues("check", keyErr.Error()).Inc()
			// Handle failure mode
			if erl.failureMode == "deny" {
				return false, time.Minute, headers, keyErr
			}
			// Continue with fail-safe mode (allow)
			continue
		}

		if !keyAllowed {
			return false, keyRetryAfter, headers, nil
		}

		if keyRetryAfter > retryAfter {
			retryAfter = keyRetryAfter
		}
	}

	return true, retryAfter, headers, nil
}

// checkSingleKey checks rate limit for a single key
func (erl *EnhancedRateLimiter) checkSingleKey(ctx context.Context, key string, config *RateLimitConfig, keyType string) (bool, time.Duration, map[string]string, error) {
	// Try Redis first
	allowed, retryAfter, headers, err := erl.checkRedis(ctx, key, config)

	if err != nil {
		// Fall back to local cache
		return erl.checkLocalCache(key, config, keyType)
	}

	// Update local cache for fail-safe
	erl.updateLocalCache(key, !allowed)

	// Update metrics
	erl.metrics.UpdateKeyMetrics(key, allowed)

	return allowed, retryAfter, headers, nil
}

// checkRedis performs Redis-based rate limiting
func (erl *EnhancedRateLimiter) checkRedis(ctx context.Context, key string, config *RateLimitConfig) (bool, time.Duration, map[string]string, error) {
	switch config.Type {
	case LimiterTokenBucket:
		return erl.checkTokenBucket(ctx, key, config)
	case LimiterSlidingWindow:
		return erl.checkSlidingWindow(ctx, key, config)
	case "leaky_bucket":
		return erl.checkLeakyBucket(ctx, key, config)
	default:
		return true, 0, nil, fmt.Errorf("unknown algorithm: %s", config.Type)
	}
}

// checkTokenBucket implements token bucket algorithm
func (erl *EnhancedRateLimiter) checkTokenBucket(ctx context.Context, key string, config *RateLimitConfig) (bool, time.Duration, map[string]string, error) {
	refillRate := float64(config.Limit) / config.Window.Seconds()
	allowed, tokensLeft, err := erl.redis.TakeTokenBucket(ctx, key, config.Burst, refillRate, 1)

	headers := map[string]string{
		"X-RateLimit-Limit":     fmt.Sprintf("%d", config.Limit),
		"X-RateLimit-Remaining": fmt.Sprintf("%.0f", tokensLeft),
		"X-RateLimit-Algorithm": "token_bucket",
	}

	var retryAfter time.Duration
	if !allowed {
		// Calculate retry after based on token refill rate
		tokensNeeded := 1.0
		retryAfter = time.Duration(tokensNeeded/refillRate) * time.Second
		headers["X-RateLimit-Retry-After"] = fmt.Sprintf("%.0f", retryAfter.Seconds())
	}

	return allowed, retryAfter, headers, err
}

// checkSlidingWindow implements sliding window algorithm
func (erl *EnhancedRateLimiter) checkSlidingWindow(ctx context.Context, key string, config *RateLimitConfig) (bool, time.Duration, map[string]string, error) {
	allowed, count, err := erl.redis.TakeSlidingWindow(ctx, key, config.Window, config.Limit, 1)

	headers := map[string]string{
		"X-RateLimit-Limit":     fmt.Sprintf("%d", config.Limit),
		"X-RateLimit-Remaining": fmt.Sprintf("%d", config.Limit-int(count)),
		"X-RateLimit-Algorithm": "sliding_window",
	}

	var retryAfter time.Duration
	if !allowed {
		// Calculate when the window resets
		_, reset, _ := erl.redis.PeekSlidingWindow(ctx, key, config.Window)
		retryAfter = time.Until(reset)
		headers["X-RateLimit-Reset"] = fmt.Sprintf("%d", reset.Unix())
	}

	return allowed, retryAfter, headers, err
}

// checkLeakyBucket implements leaky bucket algorithm
func (erl *EnhancedRateLimiter) checkLeakyBucket(ctx context.Context, key string, config *RateLimitConfig) (bool, time.Duration, map[string]string, error) {
	allowed, queueSize, err := erl.redis.TakeLeakyBucket(ctx, key, config.Burst, config.Limit, config.Window, 1)

	headers := map[string]string{
		"X-RateLimit-Limit":     fmt.Sprintf("%d", config.Limit),
		"X-RateLimit-Queue":     fmt.Sprintf("%d", queueSize),
		"X-RateLimit-Algorithm": "leaky_bucket",
	}

	var retryAfter time.Duration
	if !allowed {
		// Calculate retry after based on leak rate
		leakRate := float64(config.Limit) / config.Window.Seconds()
		retryAfter = time.Duration(float64(queueSize-int64(config.Burst))/leakRate) * time.Second
		headers["X-RateLimit-Retry-After"] = fmt.Sprintf("%.0f", retryAfter.Seconds())
	}

	return allowed, retryAfter, headers, err
}

// checkLocalCache implements local cache-based rate limiting for fail-safe mode
func (erl *EnhancedRateLimiter) checkLocalCache(key string, config *RateLimitConfig, keyType string) (bool, time.Duration, map[string]string, error) {
	erl.mu.Lock()
	defer erl.mu.Unlock()

	now := time.Now()
	entry, exists := erl.localCache[key]

	if !exists || now.After(entry.ResetTime) {
		// Create new entry
		entry = &LocalCacheEntry{
			Count:     1,
			ResetTime: now.Add(config.Window),
			LastSeen:  now,
		}
		erl.localCache[key] = entry
		return true, 0, map[string]string{
			"X-RateLimit-Mode": "local-cache",
		}, nil
	}

	entry.LastSeen = now
	if entry.Count >= int64(config.Limit) {
		retryAfter := time.Until(entry.ResetTime)
		return false, retryAfter, map[string]string{
			"X-RateLimit-Mode":        "local-cache",
			"X-RateLimit-Retry-After": fmt.Sprintf("%.0f", retryAfter.Seconds()),
		}, nil
	}

	entry.Count++
	return true, 0, map[string]string{
		"X-RateLimit-Mode": "local-cache",
	}, nil
}

// updateLocalCache updates the local cache entry
func (erl *EnhancedRateLimiter) updateLocalCache(key string, denied bool) {
	erl.mu.Lock()
	defer erl.mu.Unlock()

	now := time.Now()
	entry, exists := erl.localCache[key]
	if !exists {
		entry = &LocalCacheEntry{
			Count:     0,
			ResetTime: now.Add(time.Minute),
			LastSeen:  now,
		}
		erl.localCache[key] = entry
	}

	entry.LastSeen = now
	if denied {
		entry.Count++
	}
}

// extractKeys extracts all relevant rate limiting keys from the request
func (erl *EnhancedRateLimiter) extractKeys(r *http.Request) map[string]string {
	keys := make(map[string]string)

	// IP-based key
	if ip := extractClientIP(r); ip != "" {
		keys["ip"] = fmt.Sprintf("ip:%s", ip)
	}

	// User-based key
	if userID := extractUserID(r); userID != "" {
		keys["user"] = fmt.Sprintf("user:%s", userID)

		// User tier-based key
		if tier := extractUserTier(r); tier != "" {
			keys["user_tier"] = fmt.Sprintf("tier:%s:%s", tier, userID)
		}
	}

	// API key-based key
	if apiKey := extractAPIKey(r); apiKey != "" {
		keys["api_key"] = fmt.Sprintf("api:%s", apiKey)
	}

	// Endpoint-based key
	endpoint := fmt.Sprintf("%s:%s", r.Method, r.URL.Path)
	keys["endpoint"] = fmt.Sprintf("endpoint:%s", endpoint)

	// Role-based key
	if role := extractUserRole(r); role != "" {
		keys["role"] = fmt.Sprintf("role:%s", role)
	}

	// Global key
	keys["global"] = "global:requests"

	return keys
}

// Helper functions to extract information from request
func extractClientIP(r *http.Request) string {
	// Check various headers for the real IP
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		parts := strings.Split(ip, ",")
		return strings.TrimSpace(parts[0])
	}
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	if ip := r.Header.Get("CF-Connecting-IP"); ip != "" {
		return ip
	}
	return strings.Split(r.RemoteAddr, ":")[0]
}

func extractUserID(r *http.Request) string {
	// Try to get user ID from context or JWT token
	if userID := r.Header.Get("X-User-ID"); userID != "" {
		return userID
	}
	// Additional logic to extract from JWT or session
	return ""
}

func extractUserTier(r *http.Request) string {
	// Extract user tier from context or headers
	if tier := r.Header.Get("X-User-Tier"); tier != "" {
		return tier
	}
	return ""
}

func extractAPIKey(r *http.Request) string {
	if apiKey := r.Header.Get("X-API-Key"); apiKey != "" {
		return apiKey
	}
	if apiKey := r.Header.Get("Authorization"); apiKey != "" && strings.HasPrefix(apiKey, "Bearer ") {
		return strings.TrimPrefix(apiKey, "Bearer ")
	}
	return ""
}

func extractUserRole(r *http.Request) string {
	if role := r.Header.Get("X-User-Role"); role != "" {
		return role
	}
	return ""
}

// cleanupRoutine periodically cleans up expired local cache entries
func (erl *EnhancedRateLimiter) cleanupRoutine() {
	for {
		select {
		case <-erl.cleanupTicker.C:
			erl.cleanupExpiredEntries()
		case <-erl.stopCleanup:
			erl.cleanupTicker.Stop()
			return
		}
	}
}

// cleanupExpiredEntries removes expired entries from local cache
func (erl *EnhancedRateLimiter) cleanupExpiredEntries() {
	erl.mu.Lock()
	defer erl.mu.Unlock()

	now := time.Now()
	for key, entry := range erl.localCache {
		if now.Sub(entry.LastSeen) > erl.cacheExpiry {
			delete(erl.localCache, key)
		}
	}
}

// Close cleanly shuts down the rate limiter
func (erl *EnhancedRateLimiter) Close() {
	close(erl.stopCleanup)
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector(redis *RedisClient, enabled bool) *MetricsCollector {
	return &MetricsCollector{
		enabled:    enabled,
		redis:      redis,
		keyMetrics: make(map[string]*KeyMetrics),
	}
}

// UpdateKeyMetrics updates metrics for a specific key
func (mc *MetricsCollector) UpdateKeyMetrics(key string, allowed bool) {
	if !mc.enabled {
		return
	}

	mc.mu.Lock()
	defer mc.mu.Unlock()

	metrics, exists := mc.keyMetrics[key]
	if !exists {
		metrics = &KeyMetrics{}
		mc.keyMetrics[key] = metrics
	}

	metrics.Requests++
	metrics.LastAccess = time.Now()

	if allowed {
		metrics.Allowed++
	} else {
		metrics.Denied++
	}
}

// GetKeyMetrics returns metrics for a specific key
func (mc *MetricsCollector) GetKeyMetrics(key string) *KeyMetrics {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	if metrics, exists := mc.keyMetrics[key]; exists {
		return metrics
	}
	return nil
}
