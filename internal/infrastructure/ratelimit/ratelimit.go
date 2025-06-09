// Package ratelimit provides distributed, multi-tier rate limiting middleware for Orbit CEX.
//
// This is the main entry point for the rate limiting system. It exposes the middleware and orchestrates the different algorithms and storage backends.
package ratelimit

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// Global instances (in real system, use DI or init)
var (
	redisClient *RedisClient
	configMgr   = &ConfigManager{Configs: make(map[string]*RateLimitConfig)}
	// Exported for test injection
	ConfigManagerInstance = configMgr
)

// RedisConfig holds Redis connection configuration
type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

// InitializeRedisClient initializes the Redis client with configuration
func InitializeRedisClient(config *RedisConfig) {
	if config == nil {
		// Default configuration
		config = &RedisConfig{
			Addr:     "localhost:6379",
			Password: "",
			DB:       0,
		}
	}
	redisClient = NewRedisClient(config.Addr, config.Password, config.DB)
}

// GetRedisClient returns the current Redis client (initialize if needed)
func GetRedisClient() *RedisClient {
	if redisClient == nil {
		InitializeRedisClient(nil)
	}
	return redisClient
}

// Middleware is the main HTTP middleware for rate limiting.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if request should bypass rate limiting
		if ShouldBypassRateLimit(r) {
			next.ServeHTTP(w, r)
			return
		}

		key := KeyFromRequest(r)
		cfg := configMgr.GetConfig(key)
		if cfg == nil || !cfg.Enabled {
			next.ServeHTTP(w, r)
			return
		}
		ctx := r.Context()
		var allowed bool
		var retryAfter time.Duration
		var headers map[string]string
		var err error

		client := GetRedisClient()
		switch cfg.Type {
		case LimiterTokenBucket:
			allowed, retryAfter, headers, err = handleTokenBucket(ctx, key, cfg, client)
		case LimiterSlidingWindow:
			allowed, retryAfter, headers, err = handleSlidingWindow(ctx, key, cfg, client)
		default:
			allowed = true
		}
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("rate limit error"))
			return
		}
		for hk, hv := range headers {
			w.Header().Set(hk, hv)
		}
		if !allowed {
			w.Header().Set("Retry-After", retryAfter.String())
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte("rate limit exceeded"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// handleTokenBucket wires Redis-backed token bucket
func handleTokenBucket(ctx context.Context, key string, cfg *RateLimitConfig, client *RedisClient) (bool, time.Duration, map[string]string, error) {
	allowed, tokensLeft, err := client.TakeTokenBucket(ctx, key, cfg.Burst, float64(cfg.Limit)/cfg.Window.Seconds(), 1)
	headers := map[string]string{
		"X-RateLimit-Limit":     fmt.Sprintf("%d", cfg.Limit),
		"X-RateLimit-Remaining": fmt.Sprintf("%.0f", tokensLeft),
	}
	var retryAfter time.Duration
	if !allowed {
		// Calculate proper retry-after based on refill rate
		refillRate := float64(cfg.Limit) / cfg.Window.Seconds()
		tokensNeeded := 1.0 - tokensLeft
		secondsToWait := tokensNeeded / refillRate
		retryAfter = time.Duration(secondsToWait * float64(time.Second))
		if retryAfter < time.Second {
			retryAfter = time.Second // Minimum 1 second
		}
	}
	return allowed, retryAfter, headers, err
}

// handleSlidingWindow wires Redis-backed sliding window
func handleSlidingWindow(ctx context.Context, key string, cfg *RateLimitConfig, client *RedisClient) (bool, time.Duration, map[string]string, error) {
	allowed, count, err := client.TakeSlidingWindow(ctx, key, cfg.Window, cfg.Limit, 1)
	headers := map[string]string{
		"X-RateLimit-Limit":     fmt.Sprintf("%d", cfg.Limit),
		"X-RateLimit-Remaining": fmt.Sprintf("%d", cfg.Limit-int(count)),
	}
	var retryAfter time.Duration
	if !allowed {
		_, reset, _ := client.PeekSlidingWindow(ctx, key, cfg.Window)
		retryAfter = time.Until(reset)
	}
	return allowed, retryAfter, headers, err
}
