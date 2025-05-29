package auth

import (
	"time"

	"github.com/Aidin1998/pincex_unified/pkg/models"
)

// RateLimitConfig represents configurable rate limiting settings
type RateLimitConfig struct {
	// Global settings
	Enabled         bool          `json:"enabled" yaml:"enabled"`
	RedisKeyPrefix  string        `json:"redis_key_prefix" yaml:"redis_key_prefix"`
	CleanupInterval time.Duration `json:"cleanup_interval" yaml:"cleanup_interval"`
	MaxDataAge      time.Duration `json:"max_data_age" yaml:"max_data_age"`

	// Tier configurations
	TierConfigs map[models.UserTier]TierConfig `json:"tier_configs" yaml:"tier_configs"`

	// Endpoint configurations
	EndpointConfigs map[string]EndpointConfig `json:"endpoint_configs" yaml:"endpoint_configs"`

	// Default IP rate limits
	DefaultIPLimits IPRateLimit `json:"default_ip_limits" yaml:"default_ip_limits"`

	// Emergency settings
	EmergencyMode   bool       `json:"emergency_mode" yaml:"emergency_mode"`
	EmergencyLimits TierConfig `json:"emergency_limits" yaml:"emergency_limits"`
}

// TierConfig represents configuration for a user tier
type TierConfig struct {
	APICallsPerMinute    int `json:"api_calls_per_minute" yaml:"api_calls_per_minute"`
	OrdersPerMinute      int `json:"orders_per_minute" yaml:"orders_per_minute"`
	TradesPerMinute      int `json:"trades_per_minute" yaml:"trades_per_minute"`
	WithdrawalsPerDay    int `json:"withdrawals_per_day" yaml:"withdrawals_per_day"`
	LoginAttemptsPerHour int `json:"login_attempts_per_hour" yaml:"login_attempts_per_hour"`
}

// GetDefaultRateLimitConfig returns the default rate limiting configuration
func GetDefaultRateLimitConfig() *RateLimitConfig {
	return &RateLimitConfig{
		Enabled:         true,
		RedisKeyPrefix:  "pincex_rate_limit",
		CleanupInterval: 1 * time.Hour,
		MaxDataAge:      7 * 24 * time.Hour, // 7 days

		TierConfigs: map[models.UserTier]TierConfig{
			models.TierBasic: {
				APICallsPerMinute:    10,
				OrdersPerMinute:      5,
				TradesPerMinute:      3,
				WithdrawalsPerDay:    1,
				LoginAttemptsPerHour: 5,
			},
			models.TierPremium: {
				APICallsPerMinute:    100,
				OrdersPerMinute:      50,
				TradesPerMinute:      30,
				WithdrawalsPerDay:    10,
				LoginAttemptsPerHour: 10,
			},
			models.TierVIP: {
				APICallsPerMinute:    1000,
				OrdersPerMinute:      500,
				TradesPerMinute:      300,
				WithdrawalsPerDay:    100,
				LoginAttemptsPerHour: 20,
			},
		},

		EndpointConfigs: map[string]EndpointConfig{
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

			// Admin endpoints - stricter limits
			"POST:/api/v1/admin/*": {
				Name:         "admin_operations",
				RequiresAuth: true,
				UserRateType: "api_calls",
				IPRateLimit: IPRateLimit{
					RequestsPerMinute: 10,
					BurstLimit:        5,
					Window:            time.Minute,
				},
				CustomLimits: map[models.UserTier]int{
					models.TierBasic:   1,  // Very limited admin access for basic users
					models.TierPremium: 5,  // Moderate admin access for premium
					models.TierVIP:     10, // Higher admin access for VIP
				},
			},
		},

		DefaultIPLimits: IPRateLimit{
			RequestsPerMinute: 20,
			BurstLimit:        10,
			Window:            time.Minute,
		},

		EmergencyMode: false,
		EmergencyLimits: TierConfig{
			APICallsPerMinute:    1,
			OrdersPerMinute:      1,
			TradesPerMinute:      1,
			WithdrawalsPerDay:    0,
			LoginAttemptsPerHour: 1,
		},
	}
}

// UpdateTierLimits updates the tier limits dynamically
func (c *RateLimitConfig) UpdateTierLimits(tier models.UserTier, config TierConfig) {
	if c.TierConfigs == nil {
		c.TierConfigs = make(map[models.UserTier]TierConfig)
	}
	c.TierConfigs[tier] = config
}

// UpdateEndpointConfig updates endpoint configuration dynamically
func (c *RateLimitConfig) UpdateEndpointConfig(endpoint string, config EndpointConfig) {
	if c.EndpointConfigs == nil {
		c.EndpointConfigs = make(map[string]EndpointConfig)
	}
	c.EndpointConfigs[endpoint] = config
}

// GetTierConfig returns configuration for a specific tier
func (c *RateLimitConfig) GetTierConfig(tier models.UserTier) TierConfig {
	if config, exists := c.TierConfigs[tier]; exists {
		return config
	}
	// Return basic tier config as default
	return c.TierConfigs[models.TierBasic]
}

// IsEnabled returns whether rate limiting is enabled
func (c *RateLimitConfig) IsEnabled() bool {
	return c.Enabled && !c.EmergencyMode
}

// GetEffectiveLimits returns effective limits considering emergency mode
func (c *RateLimitConfig) GetEffectiveLimits(tier models.UserTier) TierConfig {
	if c.EmergencyMode {
		return c.EmergencyLimits
	}
	return c.GetTierConfig(tier)
}
