// Package ratelimit provides concrete strategy implementations for different
// rate limiting approaches.
package ratelimit

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"
)

// EnhancedStrategy implements RateLimitStrategy using the enhanced rate limiter
type EnhancedStrategy struct {
	name    string
	limiter *EnhancedRateLimiter
	config  *EnhancedStrategyConfig
	logger  *zap.Logger
}

// EnhancedStrategyConfig holds configuration for the enhanced strategy
type EnhancedStrategyConfig struct {
	RedisAddr      string                      `yaml:"redis_addr" json:"redis_addr"`
	RedisPassword  string                      `yaml:"redis_password" json:"redis_password"`
	RedisDB        int                         `yaml:"redis_db" json:"redis_db"`
	FailureMode    string                      `yaml:"failure_mode" json:"failure_mode"`
	ConfigFile     string                      `yaml:"config_file" json:"config_file"`
	DefaultConfigs map[string]*RateLimitConfig `yaml:"default_configs" json:"default_configs"`
}

// NewEnhancedStrategy creates a new enhanced strategy
func NewEnhancedStrategy(config *EnhancedStrategyConfig, logger *zap.Logger) (*EnhancedStrategy, error) {
	redisClient := NewRedisClient(config.RedisAddr, config.RedisPassword, config.RedisDB)
	configManager := &ConfigManager{configs: make(map[string]*RateLimitConfig)}

	// Load default configurations
	for key, cfg := range config.DefaultConfigs {
		configManager.SetConfig(key, cfg)
	}

	// Load from file if specified
	if config.ConfigFile != "" {
		if err := configManager.LoadFromFile(config.ConfigFile); err != nil {
			logger.Warn("failed to load config file, using defaults",
				zap.String("file", config.ConfigFile),
				zap.Error(err))
		}
	}

	limiter := NewEnhancedRateLimiter(redisClient, configManager, config.FailureMode)

	return &EnhancedStrategy{
		name:    "enhanced",
		limiter: limiter,
		config:  config,
		logger:  logger,
	}, nil
}

// Name returns the strategy name
func (es *EnhancedStrategy) Name() string {
	return es.name
}

// Check performs rate limiting check
func (es *EnhancedStrategy) Check(ctx context.Context, r *http.Request) (RateLimitResult, error) {
	allowed, retryAfter, headers, err := es.limiter.Check(ctx, r)

	if err != nil {
		es.logger.Error("enhanced strategy check failed",
			zap.String("path", r.URL.Path),
			zap.String("method", r.Method),
			zap.Error(err))
	}

	return RateLimitResult{
		Allowed:    allowed,
		RetryAfter: retryAfter,
		Headers:    headers,
		Reason:     "enhanced_check",
		Algorithm:  "multi-algorithm",
	}, err
}

// Configure updates the strategy configuration
func (es *EnhancedStrategy) Configure(config map[string]interface{}) error {
	// Update configuration based on provided map
	// This is a simplified implementation
	es.logger.Info("enhanced strategy configured",
		zap.Any("config", config))
	return nil
}

// HealthCheck verifies the strategy is healthy
func (es *EnhancedStrategy) HealthCheck(ctx context.Context) error {
	// Check Redis connectivity
	if err := es.limiter.redis.HealthCheck(ctx); err != nil {
		return fmt.Errorf("redis health check failed: %w", err)
	}
	return nil
}

// Close cleanly shuts down the strategy
func (es *EnhancedStrategy) Close() error {
	es.limiter.Close()
	return nil
}

// TieredStrategy implements user tier-based rate limiting
type TieredStrategy struct {
	name   string
	redis  *RedisClient
	config *TieredStrategyConfig
	logger *zap.Logger
	tiers  map[string]*TierConfig
}

// TieredStrategyConfig holds configuration for the tiered strategy
type TieredStrategyConfig struct {
	RedisAddr     string                 `yaml:"redis_addr" json:"redis_addr"`
	RedisPassword string                 `yaml:"redis_password" json:"redis_password"`
	RedisDB       int                    `yaml:"redis_db" json:"redis_db"`
	DefaultTier   string                 `yaml:"default_tier" json:"default_tier"`
	Tiers         map[string]*TierConfig `yaml:"tiers" json:"tiers"`
}

// TierConfig defines rate limits for a specific user tier
type TierConfig struct {
	Name            string `yaml:"name" json:"name"`
	RequestsPerMin  int    `yaml:"requests_per_min" json:"requests_per_min"`
	RequestsPerHour int    `yaml:"requests_per_hour" json:"requests_per_hour"`
	RequestsPerDay  int    `yaml:"requests_per_day" json:"requests_per_day"`
	BurstLimit      int    `yaml:"burst_limit" json:"burst_limit"`
	Algorithm       string `yaml:"algorithm" json:"algorithm"`
	Priority        int    `yaml:"priority" json:"priority"`
}

// NewTieredStrategy creates a new tiered strategy
func NewTieredStrategy(config *TieredStrategyConfig, logger *zap.Logger) *TieredStrategy {
	redisClient := NewRedisClient(config.RedisAddr, config.RedisPassword, config.RedisDB)

	return &TieredStrategy{
		name:   "tiered",
		redis:  redisClient,
		config: config,
		logger: logger,
		tiers:  config.Tiers,
	}
}

// Name returns the strategy name
func (ts *TieredStrategy) Name() string {
	return ts.name
}

// Check performs tiered rate limiting check
func (ts *TieredStrategy) Check(ctx context.Context, r *http.Request) (RateLimitResult, error) {
	userTier := extractUserTier(r)
	if userTier == "" {
		userTier = ts.config.DefaultTier
	}

	tierConfig, exists := ts.tiers[userTier]
	if !exists {
		tierConfig = ts.tiers[ts.config.DefaultTier]
	}

	userID := extractUserID(r)
	if userID == "" {
		userID = extractClientIP(r)
	}

	// Check multiple time windows
	checks := []struct {
		window time.Duration
		limit  int
		suffix string
	}{
		{time.Minute, tierConfig.RequestsPerMin, "min"},
		{time.Hour, tierConfig.RequestsPerHour, "hour"},
		{24 * time.Hour, tierConfig.RequestsPerDay, "day"},
	}

	headers := map[string]string{
		"X-RateLimit-Tier": userTier,
	}

	for _, check := range checks {
		if check.limit <= 0 {
			continue
		}

		key := fmt.Sprintf("tier:%s:%s:%s", userTier, userID, check.suffix)

		var allowed bool
		var count int64
		var err error

		switch tierConfig.Algorithm {
		case LimiterTokenBucket:
			refillRate := float64(check.limit) / check.window.Seconds()
			allowed, _, err = ts.redis.TakeTokenBucket(ctx, key, tierConfig.BurstLimit, refillRate, 1)
		default: // sliding window
			allowed, count, err = ts.redis.TakeSlidingWindow(ctx, key, check.window, check.limit, 1)
		}

		if err != nil {
			ts.logger.Error("tiered strategy check failed",
				zap.String("tier", userTier),
				zap.String("user", userID),
				zap.String("window", check.suffix),
				zap.Error(err))
			return RateLimitResult{}, err
		}

		// Update headers with window-specific limits
		headers[fmt.Sprintf("X-RateLimit-%s-Limit", check.suffix)] = fmt.Sprintf("%d", check.limit)
		headers[fmt.Sprintf("X-RateLimit-%s-Remaining", check.suffix)] = fmt.Sprintf("%d", check.limit-int(count))

		if !allowed {
			return RateLimitResult{
				Allowed:    false,
				RetryAfter: check.window / time.Duration(check.limit), // Approximate
				Headers:    headers,
				Reason:     fmt.Sprintf("tier_limit_%s", check.suffix),
				Algorithm:  tierConfig.Algorithm,
			}, nil
		}
	}

	return RateLimitResult{
		Allowed:   true,
		Headers:   headers,
		Reason:    "tier_allowed",
		Algorithm: tierConfig.Algorithm,
	}, nil
}

// Configure updates the tiered strategy configuration
func (ts *TieredStrategy) Configure(config map[string]interface{}) error {
	ts.logger.Info("tiered strategy configured",
		zap.Any("config", config))
	// Update tier configurations based on provided map
	return nil
}

// HealthCheck verifies the tiered strategy is healthy
func (ts *TieredStrategy) HealthCheck(ctx context.Context) error {
	return ts.redis.HealthCheck(ctx)
}

// BasicStrategy implements simple rate limiting
type BasicStrategy struct {
	name   string
	redis  *RedisClient
	config *BasicStrategyConfig
	logger *zap.Logger
}

// BasicStrategyConfig holds configuration for the basic strategy
type BasicStrategyConfig struct {
	RedisAddr       string `yaml:"redis_addr" json:"redis_addr"`
	RedisPassword   string `yaml:"redis_password" json:"redis_password"`
	RedisDB         int    `yaml:"redis_db" json:"redis_db"`
	RequestsPerMin  int    `yaml:"requests_per_min" json:"requests_per_min"`
	RequestsPerHour int    `yaml:"requests_per_hour" json:"requests_per_hour"`
	Algorithm       string `yaml:"algorithm" json:"algorithm"`
	BurstLimit      int    `yaml:"burst_limit" json:"burst_limit"`
}

// NewBasicStrategy creates a new basic strategy
func NewBasicStrategy(config *BasicStrategyConfig, logger *zap.Logger) *BasicStrategy {
	redisClient := NewRedisClient(config.RedisAddr, config.RedisPassword, config.RedisDB)

	return &BasicStrategy{
		name:   "basic",
		redis:  redisClient,
		config: config,
		logger: logger,
	}
}

// Name returns the strategy name
func (bs *BasicStrategy) Name() string {
	return bs.name
}

// Check performs basic rate limiting check
func (bs *BasicStrategy) Check(ctx context.Context, r *http.Request) (RateLimitResult, error) {
	clientIP := extractClientIP(r)
	key := fmt.Sprintf("basic:%s", clientIP)

	var allowed bool
	var count int64
	var err error

	switch bs.config.Algorithm {
	case LimiterTokenBucket:
		refillRate := float64(bs.config.RequestsPerMin) / time.Minute.Seconds()
		allowed, _, err = bs.redis.TakeTokenBucket(ctx, key, bs.config.BurstLimit, refillRate, 1)
	default: // sliding window
		allowed, count, err = bs.redis.TakeSlidingWindow(ctx, key, time.Minute, bs.config.RequestsPerMin, 1)
	}

	if err != nil {
		bs.logger.Error("basic strategy check failed",
			zap.String("ip", clientIP),
			zap.Error(err))
		return RateLimitResult{}, err
	}

	headers := map[string]string{
		"X-RateLimit-Limit":     fmt.Sprintf("%d", bs.config.RequestsPerMin),
		"X-RateLimit-Remaining": fmt.Sprintf("%d", bs.config.RequestsPerMin-int(count)),
	}

	if !allowed {
		return RateLimitResult{
			Allowed:    false,
			RetryAfter: time.Minute / time.Duration(bs.config.RequestsPerMin),
			Headers:    headers,
			Reason:     "basic_limit",
			Algorithm:  bs.config.Algorithm,
		}, nil
	}

	return RateLimitResult{
		Allowed:   true,
		Headers:   headers,
		Reason:    "basic_allowed",
		Algorithm: bs.config.Algorithm,
	}, nil
}

// Configure updates the basic strategy configuration
func (bs *BasicStrategy) Configure(config map[string]interface{}) error {
	bs.logger.Info("basic strategy configured",
		zap.Any("config", config))
	return nil
}

// HealthCheck verifies the basic strategy is healthy
func (bs *BasicStrategy) HealthCheck(ctx context.Context) error {
	return bs.redis.HealthCheck(ctx)
}
