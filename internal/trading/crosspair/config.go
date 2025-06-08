package crosspair

import (
	"fmt"
	"time"
)

// CrossPairConfig holds configuration for the cross-pair trading engine
type CrossPairConfig struct {
	// Engine settings
	MaxConcurrentOrders int           `json:"max_concurrent_orders" yaml:"max_concurrent_orders"`
	OrderTimeout        time.Duration `json:"order_timeout" yaml:"order_timeout"`
	RetryAttempts       int           `json:"retry_attempts" yaml:"retry_attempts"`
	RetryDelay          time.Duration `json:"retry_delay" yaml:"retry_delay"`
	EnableRateLimiting  bool          `json:"enable_rate_limiting" yaml:"enable_rate_limiting"`
	QueueSize           int           `json:"queue_size" yaml:"queue_size"`

	// Rate calculator settings
	RateCalculator RateCalculatorConfig `json:"rate_calculator" yaml:"rate_calculator"`

	// WebSocket settings
	WebSocket WebSocketConfig `json:"websocket" yaml:"websocket"`

	// Storage settings
	Storage StorageConfig `json:"storage" yaml:"storage"`

	// Fee settings
	Fees FeeConfig `json:"fees" yaml:"fees"`

	// Security settings
	Security SecurityConfig `json:"security" yaml:"security"`

	// Monitoring settings
	Monitoring MonitoringConfig `json:"monitoring" yaml:"monitoring"`
}

// RateCalculatorConfig holds configuration for the rate calculator
type RateCalculatorConfig struct {
	UpdateInterval      time.Duration `json:"update_interval" yaml:"update_interval"`
	ConfidenceThreshold float64       `json:"confidence_threshold" yaml:"confidence_threshold"`
	MaxSlippage         float64       `json:"max_slippage" yaml:"max_slippage"`
	EnableCaching       bool          `json:"enable_caching" yaml:"enable_caching"`
	CacheTTL            time.Duration `json:"cache_ttl" yaml:"cache_ttl"`
	MaxSubscribers      int           `json:"max_subscribers" yaml:"max_subscribers"`
}

// WebSocketConfig holds configuration for WebSocket connections
type WebSocketConfig struct {
	Enabled            bool          `json:"enabled" yaml:"enabled"`
	MaxConnections     int           `json:"max_connections" yaml:"max_connections"`
	ReadBufferSize     int           `json:"read_buffer_size" yaml:"read_buffer_size"`
	WriteBufferSize    int           `json:"write_buffer_size" yaml:"write_buffer_size"`
	PingInterval       time.Duration `json:"ping_interval" yaml:"ping_interval"`
	PongTimeout        time.Duration `json:"pong_timeout" yaml:"pong_timeout"`
	MessageRateLimit   int           `json:"message_rate_limit" yaml:"message_rate_limit"`
	BroadcastQueueSize int           `json:"broadcast_queue_size" yaml:"broadcast_queue_size"`
	AllowedOrigins     []string      `json:"allowed_origins" yaml:"allowed_origins"` // NEW: allowed websocket origins
}

// StorageConfig holds configuration for data storage
type StorageConfig struct {
	Type            string        `json:"type" yaml:"type"` // "postgres", "mysql", "memory"
	ConnectionURL   string        `json:"connection_url" yaml:"connection_url"`
	MaxConnections  int           `json:"max_connections" yaml:"max_connections"`
	MaxIdleTime     time.Duration `json:"max_idle_time" yaml:"max_idle_time"`
	MaxLifetime     time.Duration `json:"max_lifetime" yaml:"max_lifetime"`
	EnableMigration bool          `json:"enable_migration" yaml:"enable_migration"`
	EnableMetrics   bool          `json:"enable_metrics" yaml:"enable_metrics"`
}

// FeeConfig holds configuration for fee calculation
type FeeConfig struct {
	DefaultMakerFee  float64            `json:"default_maker_fee" yaml:"default_maker_fee"`
	DefaultTakerFee  float64            `json:"default_taker_fee" yaml:"default_taker_fee"`
	CrossPairMarkup  float64            `json:"cross_pair_markup" yaml:"cross_pair_markup"`
	MinimumFee       float64            `json:"minimum_fee" yaml:"minimum_fee"`
	MaximumFee       float64            `json:"maximum_fee" yaml:"maximum_fee"`
	VolumeDiscounts  []VolumeDiscount   `json:"volume_discounts" yaml:"volume_discounts"`
	PairSpecificFees map[string]float64 `json:"pair_specific_fees" yaml:"pair_specific_fees"`
}

// VolumeDiscount defines volume-based fee discounts
type VolumeDiscount struct {
	MinVolume   float64 `json:"min_volume" yaml:"min_volume"`
	Discount    float64 `json:"discount" yaml:"discount"`
	Description string  `json:"description" yaml:"description"`
}

// SecurityConfig holds security-related configuration
type SecurityConfig struct {
	EnableAuthentication bool          `json:"enable_authentication" yaml:"enable_authentication"`
	EnableAuthorization  bool          `json:"enable_authorization" yaml:"enable_authorization"`
	RequiredPermissions  []string      `json:"required_permissions" yaml:"required_permissions"`
	RateLimitPerUser     int           `json:"rate_limit_per_user" yaml:"rate_limit_per_user"`
	RateLimitWindow      time.Duration `json:"rate_limit_window" yaml:"rate_limit_window"`
	MaxOrderSize         float64       `json:"max_order_size" yaml:"max_order_size"`
	MaxDailyVolume       float64       `json:"max_daily_volume" yaml:"max_daily_volume"`
	EnableIPWhitelist    bool          `json:"enable_ip_whitelist" yaml:"enable_ip_whitelist"`
	AllowedIPs           []string      `json:"allowed_ips" yaml:"allowed_ips"`
}

// MonitoringConfig holds monitoring and metrics configuration
type MonitoringConfig struct {
	EnableMetrics     bool          `json:"enable_metrics" yaml:"enable_metrics"`
	MetricsInterval   time.Duration `json:"metrics_interval" yaml:"metrics_interval"`
	EnableHealthCheck bool          `json:"enable_health_check" yaml:"enable_health_check"`
	HealthCheckPort   int           `json:"health_check_port" yaml:"health_check_port"`
	EnableProfiling   bool          `json:"enable_profiling" yaml:"enable_profiling"`
	ProfilingPort     int           `json:"profiling_port" yaml:"profiling_port"`
	EnableTracing     bool          `json:"enable_tracing" yaml:"enable_tracing"`
	TracingEndpoint   string        `json:"tracing_endpoint" yaml:"tracing_endpoint"`
	LogLevel          string        `json:"log_level" yaml:"log_level"`
}

// DefaultConfig returns a default configuration for production
func DefaultConfig() *CrossPairConfig {
	return &CrossPairConfig{
		MaxConcurrentOrders: 100,
		OrderTimeout:        30 * time.Second,
		RetryAttempts:       3,
		RetryDelay:          1 * time.Second,
		EnableRateLimiting:  true,
		QueueSize:           1000,

		RateCalculator: RateCalculatorConfig{
			UpdateInterval:      1 * time.Second,
			ConfidenceThreshold: 0.8,
			MaxSlippage:         0.05,
			EnableCaching:       true,
			CacheTTL:            5 * time.Second,
			MaxSubscribers:      1000,
		},

		WebSocket: WebSocketConfig{
			Enabled:            true,
			MaxConnections:     10000,
			ReadBufferSize:     1024,
			WriteBufferSize:    1024,
			PingInterval:       54 * time.Second,
			PongTimeout:        60 * time.Second,
			MessageRateLimit:   100, // messages per minute
			BroadcastQueueSize: 1000,
			AllowedOrigins:     []string{"https://yourdomain.com"}, // <-- set your production domain(s) here
		},

		Storage: StorageConfig{
			Type:            "postgres",
			MaxConnections:  50,
			MaxIdleTime:     30 * time.Minute,
			MaxLifetime:     1 * time.Hour,
			EnableMigration: true,
			EnableMetrics:   true,
		},

		Fees: FeeConfig{
			DefaultMakerFee: 0.001,  // 0.1%
			DefaultTakerFee: 0.002,  // 0.2%
			CrossPairMarkup: 0.0005, // 0.05% additional for cross-pair
			MinimumFee:      0.00001,
			MaximumFee:      0.01,
			VolumeDiscounts: []VolumeDiscount{
				{MinVolume: 1000000, Discount: 0.1, Description: "1M+ volume - 10% discount"},
				{MinVolume: 10000000, Discount: 0.2, Description: "10M+ volume - 20% discount"},
			},
			PairSpecificFees: make(map[string]float64),
		},

		Security: SecurityConfig{
			EnableAuthentication: true,
			EnableAuthorization:  true,
			RequiredPermissions:  []string{"crosspair.trade"},
			RateLimitPerUser:     100,
			RateLimitWindow:      1 * time.Minute,
			MaxOrderSize:         1000000,  // $1M max order
			MaxDailyVolume:       10000000, // $10M max daily volume per user
			EnableIPWhitelist:    false,
			AllowedIPs:           []string{},
		},

		Monitoring: MonitoringConfig{
			EnableMetrics:     true,
			MetricsInterval:   30 * time.Second,
			EnableHealthCheck: true,
			HealthCheckPort:   8081,
			EnableProfiling:   false,
			ProfilingPort:     6060,
			EnableTracing:     false,
			LogLevel:          "info",
		},
	}
}

// DevelopmentConfig returns a configuration optimized for development
func DevelopmentConfig() *CrossPairConfig {
	config := DefaultConfig()

	// Relax settings for development
	config.MaxConcurrentOrders = 10
	config.OrderTimeout = 60 * time.Second
	config.EnableRateLimiting = false
	config.QueueSize = 100

	config.RateCalculator.UpdateInterval = 2 * time.Second
	config.RateCalculator.ConfidenceThreshold = 0.5
	config.RateCalculator.MaxSlippage = 0.1

	config.WebSocket.MaxConnections = 100
	config.WebSocket.MessageRateLimit = 1000

	config.Storage.Type = "memory" // Use in-memory storage for development
	config.Storage.MaxConnections = 10

	config.Security.EnableAuthentication = false
	config.Security.EnableAuthorization = false
	config.Security.RateLimitPerUser = 1000
	config.Security.MaxOrderSize = 100000
	config.Security.MaxDailyVolume = 1000000

	config.Monitoring.LogLevel = "debug"
	config.Monitoring.EnableProfiling = true

	return config
}

// TestConfig returns a configuration optimized for testing
func TestConfig() *CrossPairConfig {
	config := DefaultConfig()

	// Fast and minimal settings for testing
	config.MaxConcurrentOrders = 5
	config.OrderTimeout = 5 * time.Second
	config.RetryAttempts = 1
	config.RetryDelay = 100 * time.Millisecond
	config.EnableRateLimiting = false
	config.QueueSize = 10

	config.RateCalculator.UpdateInterval = 100 * time.Millisecond
	config.RateCalculator.ConfidenceThreshold = 0.1
	config.RateCalculator.MaxSlippage = 0.5
	config.RateCalculator.CacheTTL = 1 * time.Second

	config.WebSocket.Enabled = false

	config.Storage.Type = "memory"
	config.Storage.MaxConnections = 1

	config.Security.EnableAuthentication = false
	config.Security.EnableAuthorization = false
	config.Security.RateLimitPerUser = 10000

	config.Monitoring.EnableMetrics = false
	config.Monitoring.EnableHealthCheck = false
	config.Monitoring.LogLevel = "error"

	return config
}

// Validate validates the configuration
func (c *CrossPairConfig) Validate() error {
	if c.MaxConcurrentOrders <= 0 {
		return fmt.Errorf("max_concurrent_orders must be positive")
	}

	if c.OrderTimeout <= 0 {
		return fmt.Errorf("order_timeout must be positive")
	}

	if c.RetryAttempts < 0 {
		return fmt.Errorf("retry_attempts must be non-negative")
	}

	if c.QueueSize <= 0 {
		return fmt.Errorf("queue_size must be positive")
	}

	// Validate rate calculator config
	if err := c.RateCalculator.Validate(); err != nil {
		return fmt.Errorf("rate_calculator config: %w", err)
	}

	// Validate WebSocket config
	if err := c.WebSocket.Validate(); err != nil {
		return fmt.Errorf("websocket config: %w", err)
	}

	// Validate storage config
	if err := c.Storage.Validate(); err != nil {
		return fmt.Errorf("storage config: %w", err)
	}

	// Validate fees config
	if err := c.Fees.Validate(); err != nil {
		return fmt.Errorf("fees config: %w", err)
	}

	// Validate security config
	if err := c.Security.Validate(); err != nil {
		return fmt.Errorf("security config: %w", err)
	}

	// Validate monitoring config
	if err := c.Monitoring.Validate(); err != nil {
		return fmt.Errorf("monitoring config: %w", err)
	}

	return nil
}

// Validate validates the rate calculator configuration
func (c *RateCalculatorConfig) Validate() error {
	if c.UpdateInterval <= 0 {
		return fmt.Errorf("update_interval must be positive")
	}

	if c.ConfidenceThreshold < 0 || c.ConfidenceThreshold > 1 {
		return fmt.Errorf("confidence_threshold must be between 0 and 1")
	}

	if c.MaxSlippage < 0 || c.MaxSlippage > 1 {
		return fmt.Errorf("max_slippage must be between 0 and 1")
	}

	if c.CacheTTL <= 0 {
		return fmt.Errorf("cache_ttl must be positive")
	}

	if c.MaxSubscribers <= 0 {
		return fmt.Errorf("max_subscribers must be positive")
	}

	return nil
}

// Validate validates the WebSocket configuration
func (c *WebSocketConfig) Validate() error {
	if c.MaxConnections <= 0 {
		return fmt.Errorf("max_connections must be positive")
	}

	if c.ReadBufferSize <= 0 {
		return fmt.Errorf("read_buffer_size must be positive")
	}

	if c.WriteBufferSize <= 0 {
		return fmt.Errorf("write_buffer_size must be positive")
	}

	if c.PingInterval <= 0 {
		return fmt.Errorf("ping_interval must be positive")
	}

	if c.PongTimeout <= 0 {
		return fmt.Errorf("pong_timeout must be positive")
	}

	if c.MessageRateLimit <= 0 {
		return fmt.Errorf("message_rate_limit must be positive")
	}

	if c.BroadcastQueueSize <= 0 {
		return fmt.Errorf("broadcast_queue_size must be positive")
	}

	return nil
}

// Validate validates the storage configuration
func (c *StorageConfig) Validate() error {
	validTypes := []string{"postgres", "mysql", "memory"}
	validType := false
	for _, t := range validTypes {
		if c.Type == t {
			validType = true
			break
		}
	}
	if !validType {
		return fmt.Errorf("storage type must be one of: %v", validTypes)
	}

	if c.Type != "memory" && c.ConnectionURL == "" {
		return fmt.Errorf("connection_url is required for non-memory storage")
	}

	if c.MaxConnections <= 0 {
		return fmt.Errorf("max_connections must be positive")
	}

	return nil
}

// Validate validates the fee configuration
func (c *FeeConfig) Validate() error {
	if c.DefaultMakerFee < 0 || c.DefaultMakerFee > 1 {
		return fmt.Errorf("default_maker_fee must be between 0 and 1")
	}

	if c.DefaultTakerFee < 0 || c.DefaultTakerFee > 1 {
		return fmt.Errorf("default_taker_fee must be between 0 and 1")
	}

	if c.CrossPairMarkup < 0 || c.CrossPairMarkup > 1 {
		return fmt.Errorf("cross_pair_markup must be between 0 and 1")
	}

	if c.MinimumFee < 0 {
		return fmt.Errorf("minimum_fee must be non-negative")
	}

	if c.MaximumFee <= 0 {
		return fmt.Errorf("maximum_fee must be positive")
	}

	if c.MinimumFee >= c.MaximumFee {
		return fmt.Errorf("minimum_fee must be less than maximum_fee")
	}

	// Validate volume discounts
	for i, discount := range c.VolumeDiscounts {
		if discount.MinVolume <= 0 {
			return fmt.Errorf("volume_discount[%d]: min_volume must be positive", i)
		}
		if discount.Discount < 0 || discount.Discount > 1 {
			return fmt.Errorf("volume_discount[%d]: discount must be between 0 and 1", i)
		}
	}

	return nil
}

// Validate validates the security configuration
func (c *SecurityConfig) Validate() error {
	if c.RateLimitPerUser <= 0 {
		return fmt.Errorf("rate_limit_per_user must be positive")
	}

	if c.RateLimitWindow <= 0 {
		return fmt.Errorf("rate_limit_window must be positive")
	}

	if c.MaxOrderSize <= 0 {
		return fmt.Errorf("max_order_size must be positive")
	}

	if c.MaxDailyVolume <= 0 {
		return fmt.Errorf("max_daily_volume must be positive")
	}

	return nil
}

// Validate validates the monitoring configuration
func (c *MonitoringConfig) Validate() error {
	if c.MetricsInterval <= 0 {
		return fmt.Errorf("metrics_interval must be positive")
	}

	if c.HealthCheckPort <= 0 || c.HealthCheckPort > 65535 {
		return fmt.Errorf("health_check_port must be between 1 and 65535")
	}

	if c.ProfilingPort <= 0 || c.ProfilingPort > 65535 {
		return fmt.Errorf("profiling_port must be between 1 and 65535")
	}

	validLogLevels := []string{"debug", "info", "warn", "error"}
	validLevel := false
	for _, level := range validLogLevels {
		if c.LogLevel == level {
			validLevel = true
			break
		}
	}
	if !validLevel {
		return fmt.Errorf("log_level must be one of: %v", validLogLevels)
	}

	return nil
}
