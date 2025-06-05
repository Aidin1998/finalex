package ratelimit

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/userauth/cache"
	"github.com/go-redis/redis/v8"
	"go.uber.org/zap"
)

// ClusteredRateLimiter provides distributed rate limiting with local caching
type ClusteredRateLimiter struct {
	redis      *redis.ClusterClient
	localCache *cache.ClusteredCache
	logger     *zap.Logger
	rules      map[string]*RateLimitRule
	rulesMutex sync.RWMutex
	namespace  string
}

// RateLimitRule defines rate limiting parameters
type RateLimitRule struct {
	MaxRequests   int           `json:"max_requests"`
	WindowSize    time.Duration `json:"window_size"`
	BurstSize     int           `json:"burst_size"`
	Tier          string        `json:"tier"`
	Priority      int           `json:"priority"`
	ResetStrategy string        `json:"reset_strategy"` // sliding, fixed
}

// RateLimitResult contains the result of a rate limit check
type RateLimitResult struct {
	Allowed     bool          `json:"allowed"`
	Remaining   int           `json:"remaining"`
	ResetTime   int64         `json:"reset_time"`
	Tier        string        `json:"tier"`
	Retry       time.Duration `json:"retry_after"`
	WindowStart time.Time     `json:"window_start"`
}

// RateLimitState tracks the current state for a user/endpoint
type RateLimitState struct {
	Count       int       `json:"count"`
	WindowStart time.Time `json:"window_start"`
	LastAccess  time.Time `json:"last_access"`
	Tier        string    `json:"tier"`
}

// NewClusteredRateLimiter creates a new clustered rate limiter
func NewClusteredRateLimiter(
	redisCluster *redis.ClusterClient,
	localCache *cache.ClusteredCache,
	logger *zap.Logger,
	namespace string,
) *ClusteredRateLimiter {
	rl := &ClusteredRateLimiter{
		redis:      redisCluster,
		localCache: localCache,
		logger:     logger,
		rules:      make(map[string]*RateLimitRule),
		namespace:  namespace,
	}

	// Initialize default rules
	rl.initializeDefaultRules()

	return rl
}

// initializeDefaultRules sets up default rate limiting rules
func (rl *ClusteredRateLimiter) initializeDefaultRules() {
	defaultRules := map[string]*RateLimitRule{
		"api.auth.login": {
			MaxRequests:   5,
			WindowSize:    time.Minute,
			BurstSize:     2,
			Tier:          "strict",
			Priority:      1,
			ResetStrategy: "sliding",
		},
		"api.auth.register": {
			MaxRequests:   3,
			WindowSize:    time.Hour,
			BurstSize:     1,
			Tier:          "strict",
			Priority:      1,
			ResetStrategy: "fixed",
		},
		"api.user.profile": {
			MaxRequests:   100,
			WindowSize:    time.Minute,
			BurstSize:     20,
			Tier:          "normal",
			Priority:      2,
			ResetStrategy: "sliding",
		},
		"api.trading.order": {
			MaxRequests:   1000,
			WindowSize:    time.Minute,
			BurstSize:     100,
			Tier:          "high",
			Priority:      3,
			ResetStrategy: "sliding",
		},
		"api.market.data": {
			MaxRequests:   10000,
			WindowSize:    time.Minute,
			BurstSize:     1000,
			Tier:          "unlimited",
			Priority:      4,
			ResetStrategy: "sliding",
		},
		"api.default": {
			MaxRequests:   1000,
			WindowSize:    time.Hour,
			BurstSize:     100,
			Tier:          "normal",
			Priority:      5,
			ResetStrategy: "sliding",
		},
	}

	rl.rulesMutex.Lock()
	rl.rules = defaultRules
	rl.rulesMutex.Unlock()
}

// CheckRateLimit performs distributed rate limiting check
func (rl *ClusteredRateLimiter) CheckRateLimit(ctx context.Context, userID, endpoint, clientIP string) (*RateLimitResult, error) {
	// Get applicable rule
	rule := rl.getRuleForEndpoint(endpoint)
	if rule == nil {
		rl.logger.Warn("No rate limit rule found for endpoint", zap.String("endpoint", endpoint))
		return &RateLimitResult{Allowed: true}, nil
	}

	// Create cache keys
	userKey := rl.getUserKey(userID, endpoint)
	ipKey := rl.getIPKey(clientIP, endpoint)

	// Check both user and IP rate limits
	userResult, err := rl.checkLimit(ctx, userKey, rule)
	if err != nil {
		return nil, fmt.Errorf("failed to check user rate limit: %w", err)
	}

	ipResult, err := rl.checkLimit(ctx, ipKey, rule)
	if err != nil {
		return nil, fmt.Errorf("failed to check IP rate limit: %w", err)
	}

	// Use the more restrictive result
	result := userResult
	if !ipResult.Allowed || ipResult.Remaining < userResult.Remaining {
		result = ipResult
	}

	// Log rate limit decision
	if !result.Allowed {
		rl.logger.Warn("Rate limit exceeded",
			zap.String("user_id", userID),
			zap.String("endpoint", endpoint),
			zap.String("client_ip", clientIP),
			zap.String("tier", result.Tier),
			zap.Int("remaining", result.Remaining),
		)
	}

	return result, nil
}

// checkLimit performs the actual rate limiting logic
func (rl *ClusteredRateLimiter) checkLimit(ctx context.Context, key string, rule *RateLimitRule) (*RateLimitResult, error) {
	now := time.Now()

	// Try local cache first for read-heavy workloads
	if state, found := rl.getFromLocalCache(key); found {
		if rl.isValidWindow(state, rule, now) {
			return rl.evaluateLimit(state, rule, now, false)
		}
	}

	// Use Redis for distributed coordination
	return rl.checkDistributedLimit(ctx, key, rule, now)
}

// checkDistributedLimit performs distributed rate limiting using Redis
func (rl *ClusteredRateLimiter) checkDistributedLimit(ctx context.Context, key string, rule *RateLimitRule, now time.Time) (*RateLimitResult, error) {
	// Use Redis Lua script for atomic operations
	script := `
		local key = KEYS[1]
		local max_requests = tonumber(ARGV[1])
		local window_size = tonumber(ARGV[2])
		local now = tonumber(ARGV[3])
		local reset_strategy = ARGV[4]
		
		local current = redis.call('HMGET', key, 'count', 'window_start', 'last_access')
		local count = tonumber(current[1]) or 0
		local window_start = tonumber(current[2]) or now
		local last_access = tonumber(current[3]) or now
		
		local window_end = window_start + window_size
		local allowed = true
		local remaining = max_requests - count
		
		-- Check if we need to reset the window
		local needs_reset = false
		if reset_strategy == "fixed" then
			needs_reset = now >= window_end
		else -- sliding window
			needs_reset = (now - window_start) >= window_size
		end
		
		if needs_reset then
			count = 0
			window_start = now
			remaining = max_requests
		end
		
		-- Check if request is allowed
		if count >= max_requests then
			allowed = false
			remaining = 0
		else
			count = count + 1
			remaining = max_requests - count
		end
		
		-- Update Redis state
		redis.call('HMSET', key,
			'count', count,
			'window_start', window_start,
			'last_access', now
		)
		redis.call('EXPIRE', key, window_size)
		
		return {allowed and 1 or 0, remaining, window_start + window_size, count, window_start}
	`

	result, err := rl.redis.Eval(ctx, script, []string{key},
		rule.MaxRequests,
		int64(rule.WindowSize.Seconds()),
		now.Unix(),
		rule.ResetStrategy,
	).Result()

	if err != nil {
		return nil, fmt.Errorf("Redis script execution failed: %w", err)
	}

	results, ok := result.([]interface{})
	if !ok || len(results) < 5 {
		return nil, fmt.Errorf("unexpected Redis script result")
	}

	allowed := results[0].(int64) == 1
	remaining := int(results[1].(int64))
	resetTime := results[2].(int64)
	count := int(results[3].(int64))
	windowStart := time.Unix(results[4].(int64), 0)

	// Update local cache
	state := &RateLimitState{
		Count:       count,
		WindowStart: windowStart,
		LastAccess:  now,
		Tier:        rule.Tier,
	}
	rl.updateLocalCache(key, state)

	return &RateLimitResult{
		Allowed:     allowed,
		Remaining:   remaining,
		ResetTime:   resetTime,
		Tier:        rule.Tier,
		Retry:       rl.calculateRetryAfter(rule, windowStart, now),
		WindowStart: windowStart,
	}, nil
}

// GetUserRateLimitStatus returns comprehensive rate limit status for a user
func (rl *ClusteredRateLimiter) GetUserRateLimitStatus(ctx context.Context, userID string) (map[string]*RateLimitResult, error) {
	status := make(map[string]*RateLimitResult)

	rl.rulesMutex.RLock()
	defer rl.rulesMutex.RUnlock()

	for endpoint, rule := range rl.rules {
		key := rl.getUserKey(userID, endpoint)

		// Get current state from Redis
		state, err := rl.getStateFromRedis(ctx, key)
		if err != nil {
			rl.logger.Warn("Failed to get rate limit state", zap.String("key", key), zap.Error(err))
			continue
		}

		if state != nil {
			result := rl.evaluateLimit(state, rule, time.Now(), true)
			status[endpoint] = result
		} else {
			// No current state, user has full quota available
			status[endpoint] = &RateLimitResult{
				Allowed:     true,
				Remaining:   rule.MaxRequests,
				ResetTime:   time.Now().Add(rule.WindowSize).Unix(),
				Tier:        rule.Tier,
				WindowStart: time.Now(),
			}
		}
	}

	return status, nil
}

// AddRule adds or updates a rate limiting rule
func (rl *ClusteredRateLimiter) AddRule(endpoint string, rule *RateLimitRule) {
	rl.rulesMutex.Lock()
	defer rl.rulesMutex.Unlock()

	rl.rules[endpoint] = rule
	rl.logger.Info("Rate limit rule added/updated",
		zap.String("endpoint", endpoint),
		zap.Int("max_requests", rule.MaxRequests),
		zap.Duration("window_size", rule.WindowSize),
		zap.String("tier", rule.Tier),
	)
}

// RemoveRule removes a rate limiting rule
func (rl *ClusteredRateLimiter) RemoveRule(endpoint string) {
	rl.rulesMutex.Lock()
	defer rl.rulesMutex.Unlock()

	delete(rl.rules, endpoint)
	rl.logger.Info("Rate limit rule removed", zap.String("endpoint", endpoint))
}

// GetRules returns all current rate limiting rules
func (rl *ClusteredRateLimiter) GetRules() map[string]*RateLimitRule {
	rl.rulesMutex.RLock()
	defer rl.rulesMutex.RUnlock()

	rules := make(map[string]*RateLimitRule)
	for endpoint, rule := range rl.rules {
		rules[endpoint] = rule
	}
	return rules
}

// Helper methods

func (rl *ClusteredRateLimiter) getRuleForEndpoint(endpoint string) *RateLimitRule {
	rl.rulesMutex.RLock()
	defer rl.rulesMutex.RUnlock()

	// Try exact match first
	if rule, exists := rl.rules[endpoint]; exists {
		return rule
	}

	// Try pattern matching (simplified - could be enhanced with regex)
	for pattern, rule := range rl.rules {
		if pattern == "api.default" {
			continue // Skip default, use as fallback
		}
		// Add pattern matching logic here if needed
	}

	// Fall back to default rule
	return rl.rules["api.default"]
}

func (rl *ClusteredRateLimiter) getUserKey(userID, endpoint string) string {
	return fmt.Sprintf("%s:user:%s:%s", rl.namespace, userID, endpoint)
}

func (rl *ClusteredRateLimiter) getIPKey(clientIP, endpoint string) string {
	return fmt.Sprintf("%s:ip:%s:%s", rl.namespace, clientIP, endpoint)
}

func (rl *ClusteredRateLimiter) getFromLocalCache(key string) (*RateLimitState, bool) {
	if rl.localCache == nil {
		return nil, false
	}

	data, found, err := rl.localCache.Get(context.Background(), key)
	if err != nil || !found {
		return nil, false
	}

	if state, ok := data.(*RateLimitState); ok {
		return state, true
	}

	return nil, false
}

func (rl *ClusteredRateLimiter) updateLocalCache(key string, state *RateLimitState) {
	if rl.localCache == nil {
		return
	}

	// Cache for a short time to reduce Redis load
	cacheDuration := 10 * time.Second
	err := rl.localCache.Set(context.Background(), key, state, cacheDuration)
	if err != nil {
		rl.logger.Warn("Failed to update local cache", zap.String("key", key), zap.Error(err))
	}
}

func (rl *ClusteredRateLimiter) isValidWindow(state *RateLimitState, rule *RateLimitRule, now time.Time) bool {
	windowAge := now.Sub(state.WindowStart)
	return windowAge < rule.WindowSize
}

func (rl *ClusteredRateLimiter) evaluateLimit(state *RateLimitState, rule *RateLimitRule, now time.Time, readOnly bool) *RateLimitResult {
	remaining := rule.MaxRequests - state.Count
	if remaining < 0 {
		remaining = 0
	}

	allowed := state.Count < rule.MaxRequests
	if !readOnly && allowed {
		state.Count++
		remaining--
	}

	resetTime := state.WindowStart.Add(rule.WindowSize).Unix()

	return &RateLimitResult{
		Allowed:     allowed,
		Remaining:   remaining,
		ResetTime:   resetTime,
		Tier:        rule.Tier,
		Retry:       rl.calculateRetryAfter(rule, state.WindowStart, now),
		WindowStart: state.WindowStart,
	}
}

func (rl *ClusteredRateLimiter) calculateRetryAfter(rule *RateLimitRule, windowStart, now time.Time) time.Duration {
	if rule.ResetStrategy == "fixed" {
		return windowStart.Add(rule.WindowSize).Sub(now)
	}

	// For sliding window, suggest shorter retry
	return rule.WindowSize / time.Duration(rule.MaxRequests)
}

func (rl *ClusteredRateLimiter) getStateFromRedis(ctx context.Context, key string) (*RateLimitState, error) {
	result, err := rl.redis.HMGet(ctx, key, "count", "window_start", "last_access", "tier").Result()
	if err != nil {
		return nil, err
	}

	// Check if key exists
	if result[0] == nil {
		return nil, nil
	}

	count := 0
	if result[0] != nil {
		if c, ok := result[0].(string); ok {
			fmt.Sscanf(c, "%d", &count)
		}
	}

	windowStart := time.Now()
	if result[1] != nil {
		if ws, ok := result[1].(string); ok {
			var timestamp int64
			fmt.Sscanf(ws, "%d", &timestamp)
			windowStart = time.Unix(timestamp, 0)
		}
	}

	lastAccess := time.Now()
	if result[2] != nil {
		if la, ok := result[2].(string); ok {
			var timestamp int64
			fmt.Sscanf(la, "%d", &timestamp)
			lastAccess = time.Unix(timestamp, 0)
		}
	}

	tier := "normal"
	if result[3] != nil {
		if t, ok := result[3].(string); ok {
			tier = t
		}
	}

	return &RateLimitState{
		Count:       count,
		WindowStart: windowStart,
		LastAccess:  lastAccess,
		Tier:        tier,
	}, nil
}

// GetStats returns comprehensive rate limiting statistics
func (rl *ClusteredRateLimiter) GetStats(ctx context.Context) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Get rule statistics
	rl.rulesMutex.RLock()
	ruleCount := len(rl.rules)
	rules := make(map[string]*RateLimitRule)
	for k, v := range rl.rules {
		rules[k] = v
	}
	rl.rulesMutex.RUnlock()

	stats["total_rules"] = ruleCount
	stats["rules"] = rules

	// Get Redis statistics
	if rl.redis != nil {
		info, err := rl.redis.Info(ctx, "stats").Result()
		if err == nil {
			stats["redis_stats"] = info
		}
	}

	// Get local cache statistics
	if rl.localCache != nil {
		stats["local_cache_stats"] = rl.localCache.GetStats()
	}

	return stats, nil
}
