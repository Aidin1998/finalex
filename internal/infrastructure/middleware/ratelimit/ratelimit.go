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
	redisClient = NewRedisClient("localhost:6379", "", 0) // TODO: config
	configMgr   = &ConfigManager{configs: make(map[string]*RateLimitConfig)}
	// Exported for test injection
	ConfigManagerInstance = configMgr
)

// Middleware is the main HTTP middleware for rate limiting.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		switch cfg.Type {
		case LimiterTokenBucket:
			allowed, retryAfter, headers, err = handleTokenBucket(ctx, key, cfg)
		case LimiterSlidingWindow:
			allowed, retryAfter, headers, err = handleSlidingWindow(ctx, key, cfg)
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
func handleTokenBucket(ctx context.Context, key string, cfg *RateLimitConfig) (bool, time.Duration, map[string]string, error) {
	allowed, tokensLeft, err := redisClient.TakeTokenBucket(ctx, key, cfg.Burst, float64(cfg.Limit)/cfg.Window.Seconds(), 1)
	headers := map[string]string{
		"X-RateLimit-Limit":     fmt.Sprintf("%d", cfg.Limit),
		"X-RateLimit-Remaining": fmt.Sprintf("%.0f", tokensLeft),
	}
	var retryAfter time.Duration
	if !allowed {
		retryAfter = time.Second * 1 // TODO: calculate based on refill
	}
	return allowed, retryAfter, headers, err
}

// handleSlidingWindow wires Redis-backed sliding window
func handleSlidingWindow(ctx context.Context, key string, cfg *RateLimitConfig) (bool, time.Duration, map[string]string, error) {
	allowed, count, err := redisClient.TakeSlidingWindow(ctx, key, cfg.Window, cfg.Limit, 1)
	headers := map[string]string{
		"X-RateLimit-Limit":     fmt.Sprintf("%d", cfg.Limit),
		"X-RateLimit-Remaining": fmt.Sprintf("%d", cfg.Limit-int(count)),
	}
	var retryAfter time.Duration
	if !allowed {
		_, reset, _ := redisClient.PeekSlidingWindow(ctx, key, cfg.Window)
		retryAfter = time.Until(reset)
	}
	return allowed, retryAfter, headers, err
}
