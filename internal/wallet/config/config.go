// Package config provides configuration management for the wallet service
package config

import (
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

// WalletConfig holds all wallet service configuration
type WalletConfig struct {
	// Database configuration
	Database DatabaseConfig `yaml:"database" json:"database"`

	// Redis configuration
	Redis RedisConfig `yaml:"redis" json:"redis"`

	// Fireblocks configuration
	Fireblocks FireblocksConfig `yaml:"fireblocks" json:"fireblocks"`

	// API configuration
	API APIConfig `yaml:"api" json:"api"`

	// Cache configuration
	Cache CacheConfig `yaml:"cache" json:"cache"`

	// Rate limiting configuration
	RateLimit RateLimitConfig `yaml:"rate_limit" json:"rate_limit"`

	// Asset configuration
	Assets map[string]AssetConfig `yaml:"assets" json:"assets"`

	// Network configuration
	Networks map[string]NetworkConfig `yaml:"networks" json:"networks"`

	// Security configuration
	Security SecurityConfig `yaml:"security" json:"security"`

	// Monitoring configuration
	Monitoring MonitoringConfig `yaml:"monitoring" json:"monitoring"`
}

// DatabaseConfig holds database configuration
type DatabaseConfig struct {
	Host            string        `yaml:"host" json:"host"`
	Port            int           `yaml:"port" json:"port"`
	Username        string        `yaml:"username" json:"username"`
	Password        string        `yaml:"password" json:"password"`
	Database        string        `yaml:"database" json:"database"`
	SSLMode         string        `yaml:"ssl_mode" json:"ssl_mode"`
	MaxConnections  int           `yaml:"max_connections" json:"max_connections"`
	MaxIdleConns    int           `yaml:"max_idle_conns" json:"max_idle_conns"`
	ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime" json:"conn_max_lifetime"`
	Timeout         time.Duration `yaml:"timeout" json:"timeout"`
}

// RedisConfig holds Redis configuration
type RedisConfig struct {
	Addresses    []string      `yaml:"addresses" json:"addresses"`
	Password     string        `yaml:"password" json:"password"`
	Database     int           `yaml:"database" json:"database"`
	PoolSize     int           `yaml:"pool_size" json:"pool_size"`
	MinIdleConns int           `yaml:"min_idle_conns" json:"min_idle_conns"`
	DialTimeout  time.Duration `yaml:"dial_timeout" json:"dial_timeout"`
	ReadTimeout  time.Duration `yaml:"read_timeout" json:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout" json:"write_timeout"`
	MaxRetries   int           `yaml:"max_retries" json:"max_retries"`
}

// FireblocksConfig holds Fireblocks integration configuration
type FireblocksConfig struct {
	BaseURL        string        `yaml:"base_url" json:"base_url"`
	APIKey         string        `yaml:"api_key" json:"api_key"`
	PrivateKeyPath string        `yaml:"private_key_path" json:"private_key_path"`
	VaultAccountID string        `yaml:"vault_account_id" json:"vault_account_id"`
	Timeout        time.Duration `yaml:"timeout" json:"timeout"`
	MaxRetries     int           `yaml:"max_retries" json:"max_retries"`
	RetryDelay     time.Duration `yaml:"retry_delay" json:"retry_delay"`
	WebhookSecret  string        `yaml:"webhook_secret" json:"webhook_secret"`
	CallbackURL    string        `yaml:"callback_url" json:"callback_url"`
	EnableTestMode bool          `yaml:"enable_test_mode" json:"enable_test_mode"`
}

// APIConfig holds API server configuration
type APIConfig struct {
	HTTP HTTPConfig `yaml:"http" json:"http"`
	GRPC GRPCConfig `yaml:"grpc" json:"grpc"`
}

// HTTPConfig holds HTTP server configuration
type HTTPConfig struct {
	Host           string        `yaml:"host" json:"host"`
	Port           int           `yaml:"port" json:"port"`
	ReadTimeout    time.Duration `yaml:"read_timeout" json:"read_timeout"`
	WriteTimeout   time.Duration `yaml:"write_timeout" json:"write_timeout"`
	IdleTimeout    time.Duration `yaml:"idle_timeout" json:"idle_timeout"`
	MaxHeaderBytes int           `yaml:"max_header_bytes" json:"max_header_bytes"`
	EnableTLS      bool          `yaml:"enable_tls" json:"enable_tls"`
	CertFile       string        `yaml:"cert_file" json:"cert_file"`
	KeyFile        string        `yaml:"key_file" json:"key_file"`
}

// GRPCConfig holds gRPC server configuration
type GRPCConfig struct {
	Host              string        `yaml:"host" json:"host"`
	Port              int           `yaml:"port" json:"port"`
	MaxRecvMsgSize    int           `yaml:"max_recv_msg_size" json:"max_recv_msg_size"`
	MaxSendMsgSize    int           `yaml:"max_send_msg_size" json:"max_send_msg_size"`
	ConnectionTimeout time.Duration `yaml:"connection_timeout" json:"connection_timeout"`
	EnableReflection  bool          `yaml:"enable_reflection" json:"enable_reflection"`
	EnableTLS         bool          `yaml:"enable_tls" json:"enable_tls"`
	CertFile          string        `yaml:"cert_file" json:"cert_file"`
	KeyFile           string        `yaml:"key_file" json:"key_file"`
}

// CacheConfig holds cache configuration
type CacheConfig struct {
	TTL               time.Duration `yaml:"ttl" json:"ttl"`
	BalanceTTL        time.Duration `yaml:"balance_ttl" json:"balance_ttl"`
	TransactionTTL    time.Duration `yaml:"transaction_ttl" json:"transaction_ttl"`
	AddressTTL        time.Duration `yaml:"address_ttl" json:"address_ttl"`
	ValidationTTL     time.Duration `yaml:"validation_ttl" json:"validation_ttl"`
	KeyPrefix         string        `yaml:"key_prefix" json:"key_prefix"`
	EnableCompression bool          `yaml:"enable_compression" json:"enable_compression"`
}

// RateLimitConfig holds rate limiting configuration
type RateLimitConfig struct {
	Global     RateLimitRule            `yaml:"global" json:"global"`
	PerUser    RateLimitRule            `yaml:"per_user" json:"per_user"`
	PerAsset   map[string]RateLimitRule `yaml:"per_asset" json:"per_asset"`
	Withdrawal RateLimitRule            `yaml:"withdrawal" json:"withdrawal"`
	Deposit    RateLimitRule            `yaml:"deposit" json:"deposit"`
}

// RateLimitRule defines rate limiting rules
type RateLimitRule struct {
	RequestsPerSecond int           `yaml:"requests_per_second" json:"requests_per_second"`
	Burst             int           `yaml:"burst" json:"burst"`
	Window            time.Duration `yaml:"window" json:"window"`
	MaxDaily          int           `yaml:"max_daily" json:"max_daily"`
}

// AssetConfig holds asset-specific configuration
type AssetConfig struct {
	Symbol            string                  `yaml:"symbol" json:"symbol"`
	Name              string                  `yaml:"name" json:"name"`
	Decimals          int                     `yaml:"decimals" json:"decimals"`
	MinDeposit        decimal.Decimal         `yaml:"min_deposit" json:"min_deposit"`
	MinWithdrawal     decimal.Decimal         `yaml:"min_withdrawal" json:"min_withdrawal"`
	MaxWithdrawal     decimal.Decimal         `yaml:"max_withdrawal" json:"max_withdrawal"`
	WithdrawalFee     decimal.Decimal         `yaml:"withdrawal_fee" json:"withdrawal_fee"`
	WithdrawalFeeType string                  `yaml:"withdrawal_fee_type" json:"withdrawal_fee_type"` // fixed, percentage
	RequiredConfirms  int                     `yaml:"required_confirmations" json:"required_confirmations"`
	EnableDeposits    bool                    `yaml:"enable_deposits" json:"enable_deposits"`
	EnableWithdrawals bool                    `yaml:"enable_withdrawals" json:"enable_withdrawals"`
	FireblocksAssetID string                  `yaml:"fireblocks_asset_id" json:"fireblocks_asset_id"`
	ContractAddress   string                  `yaml:"contract_address" json:"contract_address"`
	AddressValidation AddressValidationConfig `yaml:"address_validation" json:"address_validation"`
}

// NetworkConfig holds network-specific configuration
type NetworkConfig struct {
	Name                string          `yaml:"name" json:"name"`
	ChainID             int64           `yaml:"chain_id" json:"chain_id"`
	BlockTime           time.Duration   `yaml:"block_time" json:"block_time"`
	RequiredConfirms    int             `yaml:"required_confirmations" json:"required_confirmations"`
	GasPrice            decimal.Decimal `yaml:"gas_price" json:"gas_price"`
	GasLimit            int64           `yaml:"gas_limit" json:"gas_limit"`
	FireblocksNetworkID string          `yaml:"fireblocks_network_id" json:"fireblocks_network_id"`
	ExplorerURL         string          `yaml:"explorer_url" json:"explorer_url"`
	IsTestnet           bool            `yaml:"is_testnet" json:"is_testnet"`
	EnabledAssets       []string        `yaml:"enabled_assets" json:"enabled_assets"`
}

// AddressValidationConfig holds address validation configuration
type AddressValidationConfig struct {
	Enabled         bool     `yaml:"enabled" json:"enabled"`
	ValidFormats    []string `yaml:"valid_formats" json:"valid_formats"`
	RequireChecksum bool     `yaml:"require_checksum" json:"require_checksum"`
	MinLength       int      `yaml:"min_length" json:"min_length"`
	MaxLength       int      `yaml:"max_length" json:"max_length"`
}

// SecurityConfig holds security configuration
type SecurityConfig struct {
	JWT                JWTConfig                `yaml:"jwt" json:"jwt"`
	TwoFactor          TwoFactorConfig          `yaml:"two_factor" json:"two_factor"`
	Encryption         EncryptionConfig         `yaml:"encryption" json:"encryption"`
	ComplianceChecks   ComplianceChecksConfig   `yaml:"compliance_checks" json:"compliance_checks"`
	WithdrawalSecurity WithdrawalSecurityConfig `yaml:"withdrawal_security" json:"withdrawal_security"`
}

// JWTConfig holds JWT configuration
type JWTConfig struct {
	Secret         string        `yaml:"secret" json:"secret"`
	ExpirationTime time.Duration `yaml:"expiration_time" json:"expiration_time"`
	RefreshTime    time.Duration `yaml:"refresh_time" json:"refresh_time"`
	Issuer         string        `yaml:"issuer" json:"issuer"`
	Audience       string        `yaml:"audience" json:"audience"`
}

// TwoFactorConfig holds 2FA configuration
type TwoFactorConfig struct {
	Required        bool          `yaml:"required" json:"required"`
	ValidityWindow  time.Duration `yaml:"validity_window" json:"validity_window"`
	MaxAttempts     int           `yaml:"max_attempts" json:"max_attempts"`
	LockoutDuration time.Duration `yaml:"lockout_duration" json:"lockout_duration"`
}

// EncryptionConfig holds encryption configuration
type EncryptionConfig struct {
	Algorithm string `yaml:"algorithm" json:"algorithm"`
	KeySize   int    `yaml:"key_size" json:"key_size"`
	Key       string `yaml:"key" json:"key"`
}

// ComplianceChecksConfig holds compliance configuration
type ComplianceChecksConfig struct {
	Enabled             bool                      `yaml:"enabled" json:"enabled"`
	Provider            string                    `yaml:"provider" json:"provider"`
	APIKey              string                    `yaml:"api_key" json:"api_key"`
	BaseURL             string                    `yaml:"base_url" json:"base_url"`
	Timeout             time.Duration             `yaml:"timeout" json:"timeout"`
	DepositThreshold    decimal.Decimal           `yaml:"deposit_threshold" json:"deposit_threshold"`
	WithdrawalThreshold decimal.Decimal           `yaml:"withdrawal_threshold" json:"withdrawal_threshold"`
	Rules               map[string]ComplianceRule `yaml:"rules" json:"rules"`
}

// ComplianceRule defines a compliance rule
type ComplianceRule struct {
	Enabled   bool            `yaml:"enabled" json:"enabled"`
	Threshold decimal.Decimal `yaml:"threshold" json:"threshold"`
	Action    string          `yaml:"action" json:"action"` // block, flag, manual_review
	Countries []string        `yaml:"countries" json:"countries"`
	Assets    []string        `yaml:"assets" json:"assets"`
}

// WithdrawalSecurityConfig holds withdrawal security configuration
type WithdrawalSecurityConfig struct {
	RequireEmailConfirmation bool            `yaml:"require_email_confirmation" json:"require_email_confirmation"`
	EmailTimeout             time.Duration   `yaml:"email_timeout" json:"email_timeout"`
	CooldownPeriod           time.Duration   `yaml:"cooldown_period" json:"cooldown_period"`
	DailyLimit               decimal.Decimal `yaml:"daily_limit" json:"daily_limit"`
	AddressWhitelist         bool            `yaml:"address_whitelist" json:"address_whitelist"`
	WhitelistCooldown        time.Duration   `yaml:"whitelist_cooldown" json:"whitelist_cooldown"`
}

// MonitoringConfig holds monitoring configuration
type MonitoringConfig struct {
	Metrics     MetricsConfig     `yaml:"metrics" json:"metrics"`
	Logging     LoggingConfig     `yaml:"logging" json:"logging"`
	Alerts      AlertsConfig      `yaml:"alerts" json:"alerts"`
	HealthCheck HealthCheckConfig `yaml:"health_check" json:"health_check"`
}

// MetricsConfig holds metrics configuration
type MetricsConfig struct {
	Enabled   bool              `yaml:"enabled" json:"enabled"`
	Endpoint  string            `yaml:"endpoint" json:"endpoint"`
	Namespace string            `yaml:"namespace" json:"namespace"`
	Labels    map[string]string `yaml:"labels" json:"labels"`
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level  string `yaml:"level" json:"level"`
	Format string `yaml:"format" json:"format"`
	Output string `yaml:"output" json:"output"`
}

// AlertsConfig holds alerting configuration
type AlertsConfig struct {
	Enabled           bool            `yaml:"enabled" json:"enabled"`
	WebhookURL        string          `yaml:"webhook_url" json:"webhook_url"`
	EmailRecipients   []string        `yaml:"email_recipients" json:"email_recipients"`
	FailedTxThreshold int             `yaml:"failed_tx_threshold" json:"failed_tx_threshold"`
	BalanceThreshold  decimal.Decimal `yaml:"balance_threshold" json:"balance_threshold"`
}

// HealthCheckConfig holds health check configuration
type HealthCheckConfig struct {
	Enabled  bool          `yaml:"enabled" json:"enabled"`
	Endpoint string        `yaml:"endpoint" json:"endpoint"`
	Interval time.Duration `yaml:"interval" json:"interval"`
	Timeout  time.Duration `yaml:"timeout" json:"timeout"`
}

// Validate validates the configuration
func (c *WalletConfig) Validate() error {
	// Database validation
	if c.Database.Host == "" {
		return fmt.Errorf("database host is required")
	}
	if c.Database.Database == "" {
		return fmt.Errorf("database name is required")
	}

	// Redis validation
	if len(c.Redis.Addresses) == 0 {
		return fmt.Errorf("redis addresses are required")
	}

	// Fireblocks validation
	if c.Fireblocks.BaseURL == "" {
		return fmt.Errorf("fireblocks base URL is required")
	}
	if c.Fireblocks.APIKey == "" {
		return fmt.Errorf("fireblocks API key is required")
	}
	if c.Fireblocks.PrivateKeyPath == "" {
		return fmt.Errorf("fireblocks private key path is required")
	}

	// Asset validation
	if len(c.Assets) == 0 {
		return fmt.Errorf("at least one asset must be configured")
	}

	for symbol, asset := range c.Assets {
		if asset.Symbol == "" {
			return fmt.Errorf("asset symbol is required for %s", symbol)
		}
		if asset.MinDeposit.IsNegative() {
			return fmt.Errorf("min deposit cannot be negative for %s", symbol)
		}
		if asset.MinWithdrawal.IsNegative() {
			return fmt.Errorf("min withdrawal cannot be negative for %s", symbol)
		}
		if asset.RequiredConfirms <= 0 {
			return fmt.Errorf("required confirmations must be positive for %s", symbol)
		}
	}

	// Network validation
	if len(c.Networks) == 0 {
		return fmt.Errorf("at least one network must be configured")
	}

	for name, network := range c.Networks {
		if network.Name == "" {
			return fmt.Errorf("network name is required for %s", name)
		}
		if network.RequiredConfirms <= 0 {
			return fmt.Errorf("required confirmations must be positive for network %s", name)
		}
	}

	return nil
}

// GetAssetConfig returns configuration for a specific asset
func (c *WalletConfig) GetAssetConfig(symbol string) (AssetConfig, bool) {
	config, exists := c.Assets[symbol]
	return config, exists
}

// GetNetworkConfig returns configuration for a specific network
func (c *WalletConfig) GetNetworkConfig(name string) (NetworkConfig, bool) {
	config, exists := c.Networks[name]
	return config, exists
}

// IsAssetEnabledForNetwork checks if an asset is enabled on a network
func (c *WalletConfig) IsAssetEnabledForNetwork(asset, network string) bool {
	networkConfig, exists := c.Networks[network]
	if !exists {
		return false
	}

	for _, enabledAsset := range networkConfig.EnabledAssets {
		if enabledAsset == asset {
			return true
		}
	}

	return false
}

// GetDatabaseDSN returns database connection string
func (c *DatabaseConfig) GetDSN() string {
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.Username, c.Password, c.Database, c.SSLMode)
}

// GetRedisAddresses returns Redis addresses
func (c *RedisConfig) GetRedisAddresses() []string {
	return c.Addresses
}

// LoadConfigFromFile loads wallet configuration from a file
func LoadConfigFromFile(configPath string) (*WalletConfig, error) {
	// For now, return a default configuration
	// Implement actual file loading with YAML/JSON parsing
	return &WalletConfig{
		Database: DatabaseConfig{
			Host:            "localhost",
			Port:            5432,
			Username:        "wallet",
			Password:        "password",
			Database:        "wallet_db",
			SSLMode:         "disable",
			MaxConnections:  100,
			MaxIdleConns:    10,
			ConnMaxLifetime: 30 * time.Minute,
			Timeout:         30 * time.Second,
		},
		Redis: RedisConfig{
			Addresses:    []string{"localhost:6379"},
			Password:     "",
			Database:     0,
			PoolSize:     100,
			MinIdleConns: 10,
			DialTimeout:  5 * time.Second,
			ReadTimeout:  3 * time.Second,
			WriteTimeout: 3 * time.Second,
			MaxRetries:   3,
		},
		Cache: CacheConfig{
			TTL:       10 * time.Minute,
			KeyPrefix: "wallet:",
		},
		Assets:   make(map[string]AssetConfig),
		Networks: make(map[string]NetworkConfig),
	}, nil
}
