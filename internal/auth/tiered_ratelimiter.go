package auth

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Aidin1998/pincex_unified/pkg/models"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"go.uber.org/zap"
)

// TieredRateLimiter provides comprehensive rate limiting with user tiers and IP-based limits
type TieredRateLimiter struct {
	redisClient *redis.Client
	logger      *zap.Logger
	userService UserService // Interface to get user information
	config      *RateLimitConfig
}

// UserService interface for getting user information
type UserService interface {
	GetUserByID(ctx context.Context, userID string) (*models.User, error)
}

// NewTieredRateLimiter creates a new tiered rate limiter
func NewTieredRateLimiter(redisClient *redis.Client, logger *zap.Logger, userService UserService) *TieredRateLimiter {
	return &TieredRateLimiter{
		redisClient: redisClient,
		logger:      logger,
		userService: userService,
		config:      GetDefaultRateLimitConfig(),
	}
}

// NewTieredRateLimiterWithConfig creates a new tiered rate limiter with custom config
func NewTieredRateLimiterWithConfig(redisClient *redis.Client, logger *zap.Logger, userService UserService, config *RateLimitConfig) *TieredRateLimiter {
	return &TieredRateLimiter{
		redisClient: redisClient,
		logger:      logger,
		userService: userService,
		config:      config,
	}
}

// EndpointConfig defines rate limiting configuration for an endpoint
type EndpointConfig struct {
	Name         string
	RequiresAuth bool
	UserRateType string // api_calls, orders, trades, withdrawals, login_attempts
	IPRateLimit  IPRateLimit
	CustomLimits map[models.UserTier]int // Override default tier limits if needed
}

// IPRateLimit defines IP-based rate limiting
type IPRateLimit struct {
	RequestsPerMinute int
	BurstLimit        int
	Window            time.Duration
}

// RateLimitResult contains the result of a rate limit check
type RateLimitResult struct {
	Allowed    bool                  `json:"allowed"`
	UserLimit  *models.RateLimitInfo `json:"user_limit,omitempty"`
	IPLimit    *models.RateLimitInfo `json:"ip_limit,omitempty"`
	RetryAfter *time.Duration        `json:"retry_after,omitempty"`
	LimitType  string                `json:"limit_type,omitempty"` // "user", "ip", "both"
}

// Default endpoint configurations
var defaultEndpointConfigs = map[string]EndpointConfig{
	// Public endpoints
	"GET:/api/v1/market/prices": {
		Name:         "market_prices",
		RequiresAuth: false,
		IPRateLimit: IPRateLimit{
			RequestsPerMinute: 60,
			BurstLimit:        20,
			Window:            time.Minute,
		},
	},
	"GET:/api/v1/market/candles/*": {
		Name:         "market_candles",
		RequiresAuth: false,
		IPRateLimit: IPRateLimit{
			RequestsPerMinute: 30,
			BurstLimit:        10,
			Window:            time.Minute,
		},
	},
	"GET:/api/v1/trading/pairs": {
		Name:         "trading_pairs",
		RequiresAuth: false,
		IPRateLimit: IPRateLimit{
			RequestsPerMinute: 100,
			BurstLimit:        30,
			Window:            time.Minute,
		},
	},

	// Authentication endpoints
	"POST:/api/v1/identities/login": {
		Name:         "login",
		RequiresAuth: false,
		UserRateType: "login_attempts",
		IPRateLimit: IPRateLimit{
			RequestsPerMinute: 10,
			BurstLimit:        5,
			Window:            time.Minute,
		},
	},
	"POST:/api/v1/identities/register": {
		Name:         "register",
		RequiresAuth: false,
		IPRateLimit: IPRateLimit{
			RequestsPerMinute: 5,
			BurstLimit:        2,
			Window:            time.Minute,
		},
	},

	// Trading endpoints
	"POST:/api/v1/trading/orders": {
		Name:         "place_order",
		RequiresAuth: true,
		UserRateType: "orders",
		IPRateLimit: IPRateLimit{
			RequestsPerMinute: 200,
			BurstLimit:        50,
			Window:            time.Minute,
		},
	},
	"GET:/api/v1/trading/orders": {
		Name:         "get_orders",
		RequiresAuth: true,
		UserRateType: "api_calls",
		IPRateLimit: IPRateLimit{
			RequestsPerMinute: 100,
			BurstLimit:        30,
			Window:            time.Minute,
		},
	},
	"DELETE:/api/v1/trading/orders/*": {
		Name:         "cancel_order",
		RequiresAuth: true,
		UserRateType: "orders",
		IPRateLimit: IPRateLimit{
			RequestsPerMinute: 100,
			BurstLimit:        30,
			Window:            time.Minute,
		},
	},

	// Account endpoints
	"GET:/api/v1/accounts": {
		Name:         "get_accounts",
		RequiresAuth: true,
		UserRateType: "api_calls",
		IPRateLimit: IPRateLimit{
			RequestsPerMinute: 60,
			BurstLimit:        20,
			Window:            time.Minute,
		},
	},

	// Fiat endpoints
	"POST:/api/v1/fiat/withdraw": {
		Name:         "fiat_withdraw",
		RequiresAuth: true,
		UserRateType: "withdrawals",
		IPRateLimit: IPRateLimit{
			RequestsPerMinute: 5,
			BurstLimit:        2,
			Window:            time.Minute,
		},
	},

	// Default for unmatched endpoints
	"*": {
		Name:         "default",
		RequiresAuth: true,
		UserRateType: "api_calls",
		IPRateLimit: IPRateLimit{
			RequestsPerMinute: 20,
			BurstLimit:        10,
			Window:            time.Minute,
		},
	},
}

// CheckRateLimit performs comprehensive rate limiting check
func (trl *TieredRateLimiter) CheckRateLimit(ctx context.Context, userID, endpoint, clientIP string) (*RateLimitResult, error) {
	// Get endpoint configuration
	config := trl.getEndpointConfig(endpoint)

	result := &RateLimitResult{
		Allowed: true,
	}

	// Check IP rate limit first
	ipAllowed, ipInfo, err := trl.checkIPRateLimit(ctx, clientIP, config)
	if err != nil {
		return nil, fmt.Errorf("failed to check IP rate limit: %w", err)
	}

	result.IPLimit = ipInfo
	if !ipAllowed {
		result.Allowed = false
		result.LimitType = "ip"
		if ipInfo != nil && ipInfo.ResetAt.After(time.Now()) {
			retryAfter := time.Until(ipInfo.ResetAt)
			result.RetryAfter = &retryAfter
		}
	}

	// Check user rate limit if authentication is required
	if config.RequiresAuth && userID != "" {
		userAllowed, userInfo, err := trl.checkUserRateLimit(ctx, userID, config)
		if err != nil {
			return nil, fmt.Errorf("failed to check user rate limit: %w", err)
		}

		result.UserLimit = userInfo
		if !userAllowed {
			result.Allowed = false
			if result.LimitType == "ip" {
				result.LimitType = "both"
			} else {
				result.LimitType = "user"
			}

			if userInfo != nil && userInfo.ResetAt.After(time.Now()) {
				userRetryAfter := time.Until(userInfo.ResetAt)
				if result.RetryAfter == nil || userRetryAfter > *result.RetryAfter {
					result.RetryAfter = &userRetryAfter
				}
			}
		}
	}

	return result, nil
}

// checkIPRateLimit checks IP-based rate limits
func (trl *TieredRateLimiter) checkIPRateLimit(ctx context.Context, clientIP string, config EndpointConfig) (bool, *models.RateLimitInfo, error) {
	if config.IPRateLimit.RequestsPerMinute <= 0 {
		return true, nil, nil
	}

	// Normalize IP (remove port if present)
	if host, _, err := net.SplitHostPort(clientIP); err == nil {
		clientIP = host
	}

	key := fmt.Sprintf("%s:ip_rate_limit:%s:%s", trl.config.RedisKeyPrefix, clientIP, config.Name)
	limiter := NewAdvancedRateLimiter(trl.redisClient)

	allowed, err := limiter.Allow(ctx, key, config.IPRateLimit.RequestsPerMinute, config.IPRateLimit.Window)
	if err != nil {
		return false, nil, err
	}

	info, err := limiter.GetRateLimitInfo(ctx, key, config.IPRateLimit.RequestsPerMinute, config.IPRateLimit.Window)
	if err != nil {
		return allowed, nil, err
	}

	return allowed, info, nil
}

// checkUserRateLimit checks user-based rate limits with tier consideration
func (trl *TieredRateLimiter) checkUserRateLimit(ctx context.Context, userID string, config EndpointConfig) (bool, *models.RateLimitInfo, error) {
	if config.UserRateType == "" {
		return true, nil, nil
	}

	// Get user information to determine tier
	user, err := trl.userService.GetUserByID(ctx, userID)
	if err != nil {
		trl.logger.Warn("Failed to get user for rate limiting, using basic tier", zap.String("user_id", userID), zap.Error(err))
		user = &models.User{Tier: string(models.TierBasic)}
	}

	userTier := models.UserTier(user.Tier)
	if userTier == "" {
		userTier = models.TierBasic
	}

	// Get effective tier limits (considering emergency mode)
	tierLimits := trl.config.GetEffectiveLimits(userTier)

	// Get the specific limit for this rate type
	var limit int
	var window time.Duration

	// Check if there's a custom limit for this tier and endpoint
	if customLimit, exists := config.CustomLimits[userTier]; exists {
		limit = customLimit
		window = time.Minute
	} else {
		// Use tier-based limits from config
		switch config.UserRateType {
		case "api_calls":
			limit = tierLimits.APICallsPerMinute
			window = time.Minute
		case "orders":
			limit = tierLimits.OrdersPerMinute
			window = time.Minute
		case "trades":
			limit = tierLimits.TradesPerMinute
			window = time.Minute
		case "withdrawals":
			limit = tierLimits.WithdrawalsPerDay
			window = 24 * time.Hour
		case "login_attempts":
			limit = tierLimits.LoginAttemptsPerHour
			window = time.Hour
		default:
			limit = tierLimits.APICallsPerMinute
			window = time.Minute
		}
	}

	if limit <= 0 {
		return true, nil, nil
	}

	key := fmt.Sprintf("%s:user_rate_limit:%s:%s", trl.config.RedisKeyPrefix, userID, config.UserRateType)
	limiter := NewAdvancedRateLimiter(trl.redisClient)

	allowed, err := limiter.Allow(ctx, key, limit, window)
	if err != nil {
		return false, nil, err
	}

	info, err := limiter.GetRateLimitInfo(ctx, key, limit, window)
	if err != nil {
		return allowed, nil, err
	}

	return allowed, info, nil
}

// getEndpointConfig returns the configuration for an endpoint
func (trl *TieredRateLimiter) getEndpointConfig(endpoint string) EndpointConfig {
	// Check if rate limiting is enabled
	if !trl.config.IsEnabled() {
		// Return a permissive config when disabled
		return EndpointConfig{
			Name:         "disabled",
			RequiresAuth: false,
			IPRateLimit: IPRateLimit{
				RequestsPerMinute: 1000000, // Very high limit
				BurstLimit:        1000000,
				Window:            time.Minute,
			},
		}
	}

	// Try exact match first from config
	if config, exists := trl.config.EndpointConfigs[endpoint]; exists {
		return config
	}

	// Try wildcard matches from config
	for pattern, config := range trl.config.EndpointConfigs {
		if trl.matchEndpoint(pattern, endpoint) {
			return config
		}
	}

	// Fall back to default endpoint configs
	if config, exists := defaultEndpointConfigs[endpoint]; exists {
		return config
	}

	// Try wildcard matches in default configs
	for pattern, config := range defaultEndpointConfigs {
		if trl.matchEndpoint(pattern, endpoint) {
			return config
		}
	}

	// Return default config with default IP limits
	return EndpointConfig{
		Name:         "default",
		RequiresAuth: true,
		UserRateType: "api_calls",
		IPRateLimit:  trl.config.DefaultIPLimits,
	}
}

// matchEndpoint checks if an endpoint matches a pattern
func (trl *TieredRateLimiter) matchEndpoint(pattern, endpoint string) bool {
	if pattern == "*" {
		return true
	}

	if strings.Contains(pattern, "*") {
		// Simple wildcard matching
		prefix := strings.Split(pattern, "*")[0]
		return strings.HasPrefix(endpoint, prefix)
	}

	return pattern == endpoint
}

// Middleware creates a Gin middleware for tiered rate limiting
func (trl *TieredRateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()

		// Get client IP
		clientIP := c.ClientIP()

		// Get user ID from context (set by auth middleware)
		userID := ""
		if userIDVal, exists := c.Get("userID"); exists {
			if uid, ok := userIDVal.(string); ok {
				userID = uid
			}
		}

		// Build endpoint key
		endpoint := fmt.Sprintf("%s:%s", c.Request.Method, c.Request.URL.Path)

		// Check rate limits
		result, err := trl.CheckRateLimit(ctx, userID, endpoint, clientIP)
		if err != nil {
			trl.logger.Error("Rate limit check failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Rate limit check failed"})
			c.Abort()
			return
		}

		// Set rate limit headers
		trl.setRateLimitHeaders(c, result)

		// Check if request is allowed
		if !result.Allowed {
			status := http.StatusTooManyRequests
			response := gin.H{
				"error":      "Rate limit exceeded",
				"limit_type": result.LimitType,
			}

			if result.RetryAfter != nil {
				c.Header("Retry-After", strconv.Itoa(int(result.RetryAfter.Seconds())))
				response["retry_after_seconds"] = int(result.RetryAfter.Seconds())
			}

			c.JSON(status, response)
			c.Abort()
			return
		}

		c.Next()
	}
}

// setRateLimitHeaders sets standard rate limit headers
func (trl *TieredRateLimiter) setRateLimitHeaders(c *gin.Context, result *RateLimitResult) {
	if result.UserLimit != nil {
		c.Header("X-RateLimit-User-Limit", strconv.Itoa(result.UserLimit.Limit))
		c.Header("X-RateLimit-User-Remaining", strconv.Itoa(result.UserLimit.Remaining))
		c.Header("X-RateLimit-User-Reset", strconv.FormatInt(result.UserLimit.ResetAt.Unix(), 10))
	}

	if result.IPLimit != nil {
		c.Header("X-RateLimit-IP-Limit", strconv.Itoa(result.IPLimit.Limit))
		c.Header("X-RateLimit-IP-Remaining", strconv.Itoa(result.IPLimit.Remaining))
		c.Header("X-RateLimit-IP-Reset", strconv.FormatInt(result.IPLimit.ResetAt.Unix(), 10))
	}
}

// Administrative methods for managing rate limits

// UpdateConfig updates the rate limit configuration
func (trl *TieredRateLimiter) UpdateConfig(config *RateLimitConfig) {
	trl.config = config
}

// GetConfig returns the current rate limit configuration
func (trl *TieredRateLimiter) GetConfig() *RateLimitConfig {
	return trl.config
}

// SetEmergencyMode enables or disables emergency mode
func (trl *TieredRateLimiter) SetEmergencyMode(enabled bool) {
	trl.config.EmergencyMode = enabled
	trl.logger.Info("Emergency mode updated", zap.Bool("enabled", enabled))
}

// UpdateTierLimits updates limits for a specific tier
func (trl *TieredRateLimiter) UpdateTierLimits(tier models.UserTier, limits TierConfig) {
	trl.config.UpdateTierLimits(tier, limits)
	trl.logger.Info("Tier limits updated", zap.String("tier", string(tier)), zap.Any("limits", limits))
}

// UpdateEndpointConfig updates configuration for a specific endpoint
func (trl *TieredRateLimiter) UpdateEndpointConfig(endpoint string, config EndpointConfig) {
	trl.config.UpdateEndpointConfig(endpoint, config)
	trl.logger.Info("Endpoint config updated", zap.String("endpoint", endpoint), zap.Any("config", config))
}

// GetUserRateLimitStatus returns current rate limit status for a user
func (trl *TieredRateLimiter) GetUserRateLimitStatus(ctx context.Context, userID string) (map[string]*models.RateLimitInfo, error) {
	user, err := trl.userService.GetUserByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	userTier := models.UserTier(user.Tier)
	if userTier == "" {
		userTier = models.TierBasic
	}

	tierLimits := trl.config.GetEffectiveLimits(userTier)
	limiter := NewAdvancedRateLimiter(trl.redisClient)

	status := make(map[string]*models.RateLimitInfo)

	// Check API calls
	key := fmt.Sprintf("%s:user_rate_limit:%s:api_calls", trl.config.RedisKeyPrefix, userID)
	if info, err := limiter.GetRateLimitInfo(ctx, key, tierLimits.APICallsPerMinute, time.Minute); err == nil {
		status["api_calls"] = info
	}

	// Check orders
	key = fmt.Sprintf("%s:user_rate_limit:%s:orders", trl.config.RedisKeyPrefix, userID)
	if info, err := limiter.GetRateLimitInfo(ctx, key, tierLimits.OrdersPerMinute, time.Minute); err == nil {
		status["orders"] = info
	}

	// Check trades
	key = fmt.Sprintf("%s:user_rate_limit:%s:trades", trl.config.RedisKeyPrefix, userID)
	if info, err := limiter.GetRateLimitInfo(ctx, key, tierLimits.TradesPerMinute, time.Minute); err == nil {
		status["trades"] = info
	}

	// Check withdrawals
	key = fmt.Sprintf("%s:user_rate_limit:%s:withdrawals", trl.config.RedisKeyPrefix, userID)
	if info, err := limiter.GetRateLimitInfo(ctx, key, tierLimits.WithdrawalsPerDay, 24*time.Hour); err == nil {
		status["withdrawals"] = info
	}

	// Check login attempts
	key = fmt.Sprintf("%s:user_rate_limit:%s:login_attempts", trl.config.RedisKeyPrefix, userID)
	if info, err := limiter.GetRateLimitInfo(ctx, key, tierLimits.LoginAttemptsPerHour, time.Hour); err == nil {
		status["login_attempts"] = info
	}

	return status, nil
}

// GetIPRateLimitStatus returns current rate limit status for an IP
func (trl *TieredRateLimiter) GetIPRateLimitStatus(ctx context.Context, clientIP string) (map[string]*models.RateLimitInfo, error) {
	// Normalize IP
	if host, _, err := net.SplitHostPort(clientIP); err == nil {
		clientIP = host
	}

	limiter := NewAdvancedRateLimiter(trl.redisClient)
	status := make(map[string]*models.RateLimitInfo)

	// Check common endpoint patterns
	endpoints := []struct {
		name   string
		config EndpointConfig
	}{
		{"market_prices", trl.config.EndpointConfigs["GET:/api/v1/market/prices"]},
		{"trading_pairs", trl.config.EndpointConfigs["GET:/api/v1/trading/pairs"]},
		{"login", trl.config.EndpointConfigs["POST:/api/v1/identities/login"]},
	}

	for _, endpoint := range endpoints {
		if endpoint.config.IPRateLimit.RequestsPerMinute > 0 {
			key := fmt.Sprintf("%s:ip_rate_limit:%s:%s", trl.config.RedisKeyPrefix, clientIP, endpoint.config.Name)
			if info, err := limiter.GetRateLimitInfo(ctx, key, endpoint.config.IPRateLimit.RequestsPerMinute, endpoint.config.IPRateLimit.Window); err == nil {
				status[endpoint.name] = info
			}
		}
	}

	return status, nil
}

// ResetUserRateLimit resets rate limits for a specific user and rate type
func (trl *TieredRateLimiter) ResetUserRateLimit(ctx context.Context, userID, rateType string) error {
	key := fmt.Sprintf("%s:user_rate_limit:%s:%s", trl.config.RedisKeyPrefix, userID, rateType)
	err := trl.redisClient.Del(ctx, key).Err()
	if err != nil {
		return fmt.Errorf("failed to reset user rate limit: %w", err)
	}

	trl.logger.Info("User rate limit reset", zap.String("user_id", userID), zap.String("rate_type", rateType))
	return nil
}

// ResetIPRateLimit resets rate limits for a specific IP and endpoint
func (trl *TieredRateLimiter) ResetIPRateLimit(ctx context.Context, clientIP, endpoint string) error {
	// Normalize IP
	if host, _, err := net.SplitHostPort(clientIP); err == nil {
		clientIP = host
	}

	key := fmt.Sprintf("%s:ip_rate_limit:%s:%s", trl.config.RedisKeyPrefix, clientIP, endpoint)
	err := trl.redisClient.Del(ctx, key).Err()
	if err != nil {
		return fmt.Errorf("failed to reset IP rate limit: %w", err)
	}

	trl.logger.Info("IP rate limit reset", zap.String("client_ip", clientIP), zap.String("endpoint", endpoint))
	return nil
}

// CleanupExpiredData removes expired rate limit data
func (trl *TieredRateLimiter) CleanupExpiredData(ctx context.Context) error {
	limiter := NewAdvancedRateLimiter(trl.redisClient)
	err := limiter.CleanupRateLimitData(ctx, trl.config.MaxDataAge)
	if err != nil {
		return fmt.Errorf("failed to cleanup expired data: %w", err)
	}

	trl.logger.Info("Rate limit data cleanup completed")
	return nil
}

// StartBackgroundCleanup starts a background goroutine for periodic cleanup
func (trl *TieredRateLimiter) StartBackgroundCleanup(ctx context.Context) {
	ticker := time.NewTicker(trl.config.CleanupInterval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := trl.CleanupExpiredData(ctx); err != nil {
					trl.logger.Error("Background cleanup failed", zap.Error(err))
				}
			}
		}
	}()
}
