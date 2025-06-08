// Package config provides centralized configuration management for the platform
package config

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/go-playground/validator/v10"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

// Version represents the config version for backward compatibility
const ConfigVersion = "1.0.0"

// SecretProvider interface for secure secret management
type SecretProvider interface {
	GetSecret(ctx context.Context, key string) (string, error)
	ListSecrets(ctx context.Context, prefix string) (map[string]string, error)
	Close() error
}

// ConfigManager handles centralized configuration with hot-reload capabilities
type ConfigManager struct {
	mu             sync.RWMutex
	config         *PlatformConfig
	viper          *viper.Viper
	validator      *validator.Validate
	secretProvider SecretProvider
	logger         *zap.Logger
	
	// Hot reload
	watcher        *fsnotify.Watcher
	watchPaths     []string
	reloadCallbacks []ReloadCallback
	ctx            context.Context
	cancel         context.CancelFunc
	
	// State
	initialized    bool
	lastReload     time.Time
}

// ReloadCallback is called when configuration is reloaded
type ReloadCallback func(oldConfig, newConfig *PlatformConfig) error

// PlatformConfig represents the complete platform configuration
type PlatformConfig struct {
	Version     string                 `mapstructure:"version" yaml:"version" validate:"required"`
	Environment string                 `mapstructure:"environment" yaml:"environment" validate:"required,oneof=development staging production"`
	
	// Core Services
	Server   ServerConfig   `mapstructure:"server" yaml:"server" validate:"required"`
	Database DatabaseConfig `mapstructure:"database" yaml:"database" validate:"required"`
	Redis    RedisConfig    `mapstructure:"redis" yaml:"redis" validate:"required"`
	
	// Security
	JWT      JWTConfig      `mapstructure:"jwt" yaml:"jwt" validate:"required"`
	Security SecurityConfig `mapstructure:"security" yaml:"security" validate:"required"`
	
	// External Services
	KYC         KYCConfig         `mapstructure:"kyc" yaml:"kyc" validate:"required"`
	Kafka       KafkaConfig       `mapstructure:"kafka" yaml:"kafka" validate:"required"`
	Monitoring  MonitoringConfig  `mapstructure:"monitoring" yaml:"monitoring" validate:"required"`
	
	// Platform Modules
	Trading     TradingConfig     `mapstructure:"trading" yaml:"trading" validate:"required"`
	Wallet      WalletConfig      `mapstructure:"wallet" yaml:"wallet" validate:"required"`
	Accounts    AccountsConfig    `mapstructure:"accounts" yaml:"accounts" validate:"required"`
	Compliance  ComplianceConfig  `mapstructure:"compliance" yaml:"compliance" validate:"required"`
	
	// Infrastructure
	Secrets     SecretsConfig     `mapstructure:"secrets" yaml:"secrets" validate:"required"`
	Logging     LoggingConfig     `mapstructure:"logging" yaml:"logging" validate:"required"`
}

// ServerConfig holds server configuration
type ServerConfig struct {
	Host         string        `mapstructure:"host" yaml:"host" validate:"required"`
	Port         int           `mapstructure:"port" yaml:"port" validate:"required,min=1,max=65535"`
	TLSEnabled   bool          `mapstructure:"tls_enabled" yaml:"tls_enabled"`
	TLSCertFile  string        `mapstructure:"tls_cert_file" yaml:"tls_cert_file"`
	TLSKeyFile   string        `mapstructure:"tls_key_file" yaml:"tls_key_file"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout" yaml:"read_timeout" validate:"required"`
	WriteTimeout time.Duration `mapstructure:"write_timeout" yaml:"write_timeout" validate:"required"`
	IdleTimeout  time.Duration `mapstructure:"idle_timeout" yaml:"idle_timeout" validate:"required"`
	GracefulShutdownTimeout time.Duration `mapstructure:"graceful_shutdown_timeout" yaml:"graceful_shutdown_timeout" validate:"required"`
}

// DatabaseConfig holds database configuration
type DatabaseConfig struct {
	Driver          string        `mapstructure:"driver" yaml:"driver" validate:"required,oneof=postgres cockroach"`
	DSN             string        `mapstructure:"dsn" yaml:"dsn" validate:"required"`
	CockroachDSN    string        `mapstructure:"cockroach_dsn" yaml:"cockroach_dsn"`
	MaxOpenConns    int           `mapstructure:"max_open_conns" yaml:"max_open_conns" validate:"min=1"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns" yaml:"max_idle_conns" validate:"min=1"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime" yaml:"conn_max_lifetime" validate:"required"`
	ConnMaxIdleTime time.Duration `mapstructure:"conn_max_idle_time" yaml:"conn_max_idle_time" validate:"required"`
	
	// Sharding
	Shards   []ShardConfig `mapstructure:"shards" yaml:"shards"`
	Pool     PoolConfig    `mapstructure:"pool" yaml:"pool" validate:"required"`
	
	// Migrations
	MigrationsPath string `mapstructure:"migrations_path" yaml:"migrations_path" validate:"required"`
	AutoMigrate    bool   `mapstructure:"auto_migrate" yaml:"auto_migrate"`
}

// ShardConfig represents a database shard configuration
type ShardConfig struct {
	Name       string `mapstructure:"name" yaml:"name" validate:"required"`
	MasterDSN  string `mapstructure:"master_dsn" yaml:"master_dsn" validate:"required"`
	ReplicaDSN string `mapstructure:"replica_dsn" yaml:"replica_dsn"`
	Weight     int    `mapstructure:"weight" yaml:"weight" validate:"min=1"`
}

// PoolConfig represents database connection pool configuration
type PoolConfig struct {
	MaxOpenConns    int           `mapstructure:"max_open_conns" yaml:"max_open_conns" validate:"min=1"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns" yaml:"max_idle_conns" validate:"min=1"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime" yaml:"conn_max_lifetime" validate:"required"`
}

// RedisConfig holds Redis configuration
type RedisConfig struct {
	Address     string            `mapstructure:"address" yaml:"address" validate:"required"`
	Password    string            `mapstructure:"password" yaml:"password"`
	DB          int               `mapstructure:"db" yaml:"db" validate:"min=0"`
	PoolSize    int               `mapstructure:"pool_size" yaml:"pool_size" validate:"min=1"`
	MinIdleConns int              `mapstructure:"min_idle_conns" yaml:"min_idle_conns" validate:"min=0"`
	DialTimeout time.Duration     `mapstructure:"dial_timeout" yaml:"dial_timeout" validate:"required"`
	ReadTimeout time.Duration     `mapstructure:"read_timeout" yaml:"read_timeout" validate:"required"`
	WriteTimeout time.Duration    `mapstructure:"write_timeout" yaml:"write_timeout" validate:"required"`
	
	// Cluster
	Cluster      ClusterConfig `mapstructure:"cluster" yaml:"cluster"`
	
	// Sentinel
	Sentinel     SentinelConfig `mapstructure:"sentinel" yaml:"sentinel"`
}

// ClusterConfig represents Redis cluster configuration
type ClusterConfig struct {
	Enabled   bool     `mapstructure:"enabled" yaml:"enabled"`
	Addrs     []string `mapstructure:"addrs" yaml:"addrs"`
	Password  string   `mapstructure:"password" yaml:"password"`
	MaxRedirects int   `mapstructure:"max_redirects" yaml:"max_redirects" validate:"min=1"`
}

// SentinelConfig represents Redis Sentinel configuration
type SentinelConfig struct {
	Enabled    bool     `mapstructure:"enabled" yaml:"enabled"`
	MasterName string   `mapstructure:"master_name" yaml:"master_name"`
	Addrs      []string `mapstructure:"addrs" yaml:"addrs"`
	Password   string   `mapstructure:"password" yaml:"password"`
}

// JWTConfig holds JWT configuration
type JWTConfig struct {
	Secret           string        `mapstructure:"secret" yaml:"secret" validate:"required,min=32"`
	ExpirationHours  int           `mapstructure:"expiration_hours" yaml:"expiration_hours" validate:"required,min=1"`
	RefreshSecret    string        `mapstructure:"refresh_secret" yaml:"refresh_secret" validate:"required,min=32"`
	RefreshExpHours  int           `mapstructure:"refresh_exp_hours" yaml:"refresh_exp_hours" validate:"required,min=1"`
	Issuer           string        `mapstructure:"issuer" yaml:"issuer" validate:"required"`
	Audience         string        `mapstructure:"audience" yaml:"audience" validate:"required"`
	Algorithm        string        `mapstructure:"algorithm" yaml:"algorithm" validate:"required,oneof=HS256 HS384 HS512 RS256 RS384 RS512"`
	ClockSkew        time.Duration `mapstructure:"clock_skew" yaml:"clock_skew" validate:"required"`
}

// SecurityConfig holds security-related configuration
type SecurityConfig struct {
	CORS           CORSConfig           `mapstructure:"cors" yaml:"cors" validate:"required"`
	RateLimit      RateLimitConfig      `mapstructure:"rate_limit" yaml:"rate_limit" validate:"required"`
	Encryption     EncryptionConfig     `mapstructure:"encryption" yaml:"encryption" validate:"required"`
	Authentication AuthenticationConfig `mapstructure:"authentication" yaml:"authentication" validate:"required"`
}

// CORSConfig represents CORS configuration
type CORSConfig struct {
	AllowedOrigins     []string      `mapstructure:"allowed_origins" yaml:"allowed_origins" validate:"required"`
	AllowedMethods     []string      `mapstructure:"allowed_methods" yaml:"allowed_methods" validate:"required"`
	AllowedHeaders     []string      `mapstructure:"allowed_headers" yaml:"allowed_headers" validate:"required"`
	ExposedHeaders     []string      `mapstructure:"exposed_headers" yaml:"exposed_headers"`
	AllowCredentials   bool          `mapstructure:"allow_credentials" yaml:"allow_credentials"`
	MaxAge             time.Duration `mapstructure:"max_age" yaml:"max_age" validate:"required"`
}

// RateLimitConfig represents rate limiting configuration
type RateLimitConfig struct {
	Enabled         bool          `mapstructure:"enabled" yaml:"enabled"`
	RequestsPerMin  int           `mapstructure:"requests_per_min" yaml:"requests_per_min" validate:"min=1"`
	BurstSize       int           `mapstructure:"burst_size" yaml:"burst_size" validate:"min=1"`
	CleanupInterval time.Duration `mapstructure:"cleanup_interval" yaml:"cleanup_interval" validate:"required"`
}

// EncryptionConfig represents encryption configuration
type EncryptionConfig struct {
	AESKey        string `mapstructure:"aes_key" yaml:"aes_key" validate:"required,len=32"`
	RSAPublicKey  string `mapstructure:"rsa_public_key" yaml:"rsa_public_key"`
	RSAPrivateKey string `mapstructure:"rsa_private_key" yaml:"rsa_private_key"`
}

// AuthenticationConfig represents authentication configuration
type AuthenticationConfig struct {
	SessionTimeout    time.Duration `mapstructure:"session_timeout" yaml:"session_timeout" validate:"required"`
	MaxLoginAttempts  int           `mapstructure:"max_login_attempts" yaml:"max_login_attempts" validate:"min=1"`
	LockoutDuration   time.Duration `mapstructure:"lockout_duration" yaml:"lockout_duration" validate:"required"`
	PasswordMinLength int           `mapstructure:"password_min_length" yaml:"password_min_length" validate:"min=8"`
	RequireMFA        bool          `mapstructure:"require_mfa" yaml:"require_mfa"`
}

// KYCConfig holds KYC configuration
type KYCConfig struct {
	ProviderURL        string        `mapstructure:"provider_url" yaml:"provider_url" validate:"required,url"`
	ProviderAPIKey     string        `mapstructure:"provider_api_key" yaml:"provider_api_key" validate:"required"`
	DocumentBasePath   string        `mapstructure:"document_base_path" yaml:"document_base_path" validate:"required"`
	MaxFileSize        int64         `mapstructure:"max_file_size" yaml:"max_file_size" validate:"min=1"`
	AllowedMimeTypes   []string      `mapstructure:"allowed_mime_types" yaml:"allowed_mime_types" validate:"required"`
	VerificationTimeout time.Duration `mapstructure:"verification_timeout" yaml:"verification_timeout" validate:"required"`
}

// KafkaConfig holds Kafka configuration
type KafkaConfig struct {
	Brokers             []string      `mapstructure:"brokers" yaml:"brokers" validate:"required,min=1"`
	EnableMessageQueue  bool          `mapstructure:"enable_message_queue" yaml:"enable_message_queue"`
	ConsumerGroupPrefix string        `mapstructure:"consumer_group_prefix" yaml:"consumer_group_prefix" validate:"required"`
	ProducerConfig      ProducerConfig `mapstructure:"producer" yaml:"producer" validate:"required"`
	ConsumerConfig      ConsumerConfig `mapstructure:"consumer" yaml:"consumer" validate:"required"`
	
	// Security
	SASL           SASLConfig    `mapstructure:"sasl" yaml:"sasl"`
	TLS            TLSConfig     `mapstructure:"tls" yaml:"tls"`
}

// ProducerConfig represents Kafka producer configuration
type ProducerConfig struct {
	BatchSize        int           `mapstructure:"batch_size" yaml:"batch_size" validate:"min=1"`
	LingerMs         int           `mapstructure:"linger_ms" yaml:"linger_ms" validate:"min=0"`
	CompressionType  string        `mapstructure:"compression_type" yaml:"compression_type" validate:"oneof=none gzip snappy lz4 zstd"`
	MaxRetries       int           `mapstructure:"max_retries" yaml:"max_retries" validate:"min=0"`
	RetryBackoffMs   int           `mapstructure:"retry_backoff_ms" yaml:"retry_backoff_ms" validate:"min=1"`
	RequestTimeoutMs int           `mapstructure:"request_timeout_ms" yaml:"request_timeout_ms" validate:"min=1"`
}

// ConsumerConfig represents Kafka consumer configuration
type ConsumerConfig struct {
	SessionTimeoutMs int    `mapstructure:"session_timeout_ms" yaml:"session_timeout_ms" validate:"min=1"`
	HeartbeatMs      int    `mapstructure:"heartbeat_ms" yaml:"heartbeat_ms" validate:"min=1"`
	AutoOffsetReset  string `mapstructure:"auto_offset_reset" yaml:"auto_offset_reset" validate:"oneof=earliest latest none"`
	EnableAutoCommit bool   `mapstructure:"enable_auto_commit" yaml:"enable_auto_commit"`
	MaxPollRecords   int    `mapstructure:"max_poll_records" yaml:"max_poll_records" validate:"min=1"`
}

// SASLConfig represents SASL authentication configuration
type SASLConfig struct {
	Enabled   bool   `mapstructure:"enabled" yaml:"enabled"`
	Mechanism string `mapstructure:"mechanism" yaml:"mechanism" validate:"oneof=PLAIN SCRAM-SHA-256 SCRAM-SHA-512"`
	Username  string `mapstructure:"username" yaml:"username"`
	Password  string `mapstructure:"password" yaml:"password"`
}

// TLSConfig represents TLS configuration
type TLSConfig struct {
	Enabled            bool   `mapstructure:"enabled" yaml:"enabled"`
	CertFile           string `mapstructure:"cert_file" yaml:"cert_file"`
	KeyFile            string `mapstructure:"key_file" yaml:"key_file"`
	CAFile             string `mapstructure:"ca_file" yaml:"ca_file"`
	InsecureSkipVerify bool   `mapstructure:"insecure_skip_verify" yaml:"insecure_skip_verify"`
}

// MonitoringConfig holds monitoring configuration
type MonitoringConfig struct {
	Enabled        bool               `mapstructure:"enabled" yaml:"enabled"`
	MetricsPort    int                `mapstructure:"metrics_port" yaml:"metrics_port" validate:"min=1,max=65535"`
	HealthPort     int                `mapstructure:"health_port" yaml:"health_port" validate:"min=1,max=65535"`
	PrometheusPath string             `mapstructure:"prometheus_path" yaml:"prometheus_path" validate:"required"`
	
	// Tracing
	Tracing        TracingConfig      `mapstructure:"tracing" yaml:"tracing" validate:"required"`
	
	// Alerting
	Alerting       AlertingConfig     `mapstructure:"alerting" yaml:"alerting" validate:"required"`
}

// TracingConfig represents distributed tracing configuration
type TracingConfig struct {
	Enabled     bool    `mapstructure:"enabled" yaml:"enabled"`
	ServiceName string  `mapstructure:"service_name" yaml:"service_name" validate:"required"`
	Endpoint    string  `mapstructure:"endpoint" yaml:"endpoint"`
	SampleRate  float64 `mapstructure:"sample_rate" yaml:"sample_rate" validate:"min=0,max=1"`
}

// AlertingConfig represents alerting configuration
type AlertingConfig struct {
	Enabled       bool                 `mapstructure:"enabled" yaml:"enabled"`
	WebhookURL    string               `mapstructure:"webhook_url" yaml:"webhook_url"`
	EmailConfig   EmailAlertConfig     `mapstructure:"email" yaml:"email"`
	SlackConfig   SlackAlertConfig     `mapstructure:"slack" yaml:"slack"`
	Thresholds    AlertThresholds      `mapstructure:"thresholds" yaml:"thresholds" validate:"required"`
}

// EmailAlertConfig represents email alerting configuration
type EmailAlertConfig struct {
	Enabled     bool     `mapstructure:"enabled" yaml:"enabled"`
	SMTPHost    string   `mapstructure:"smtp_host" yaml:"smtp_host"`
	SMTPPort    int      `mapstructure:"smtp_port" yaml:"smtp_port" validate:"min=1,max=65535"`
	Username    string   `mapstructure:"username" yaml:"username"`
	Password    string   `mapstructure:"password" yaml:"password"`
	FromAddress string   `mapstructure:"from_address" yaml:"from_address" validate:"email"`
	ToAddresses []string `mapstructure:"to_addresses" yaml:"to_addresses"`
}

// SlackAlertConfig represents Slack alerting configuration
type SlackAlertConfig struct {
	Enabled    bool   `mapstructure:"enabled" yaml:"enabled"`
	WebhookURL string `mapstructure:"webhook_url" yaml:"webhook_url" validate:"url"`
	Channel    string `mapstructure:"channel" yaml:"channel"`
}

// AlertThresholds represents alerting thresholds
type AlertThresholds struct {
	ErrorRate    float64 `mapstructure:"error_rate" yaml:"error_rate" validate:"min=0,max=1"`
	TimeoutRate  float64 `mapstructure:"timeout_rate" yaml:"timeout_rate" validate:"min=0,max=1"`
	ResponseTime int     `mapstructure:"response_time_ms" yaml:"response_time_ms" validate:"min=1"`
	MemoryUsage  float64 `mapstructure:"memory_usage" yaml:"memory_usage" validate:"min=0,max=1"`
	CPUUsage     float64 `mapstructure:"cpu_usage" yaml:"cpu_usage" validate:"min=0,max=1"`
}

// TradingConfig holds trading-specific configuration
type TradingConfig struct {
	MaxOrderSize        float64       `mapstructure:"max_order_size" yaml:"max_order_size" validate:"min=0"`
	MinOrderSize        float64       `mapstructure:"min_order_size" yaml:"min_order_size" validate:"min=0"`
	DefaultFeeRate      float64       `mapstructure:"default_fee_rate" yaml:"default_fee_rate" validate:"min=0,max=1"`
	MaxOpenOrders       int           `mapstructure:"max_open_orders" yaml:"max_open_orders" validate:"min=1"`
	OrderBookDepth      int           `mapstructure:"order_book_depth" yaml:"order_book_depth" validate:"min=1"`
	MatchingTimeout     time.Duration `mapstructure:"matching_timeout" yaml:"matching_timeout" validate:"required"`
	RiskCheckEnabled    bool          `mapstructure:"risk_check_enabled" yaml:"risk_check_enabled"`
	CircuitBreakerEnabled bool        `mapstructure:"circuit_breaker_enabled" yaml:"circuit_breaker_enabled"`
}

// WalletConfig holds wallet-specific configuration
type WalletConfig struct {
	MinConfirmations    int           `mapstructure:"min_confirmations" yaml:"min_confirmations" validate:"min=1"`
	MaxWithdrawalAmount float64       `mapstructure:"max_withdrawal_amount" yaml:"max_withdrawal_amount" validate:"min=0"`
	WithdrawalFeeRate   float64       `mapstructure:"withdrawal_fee_rate" yaml:"withdrawal_fee_rate" validate:"min=0,max=1"`
	ColdStorageThreshold float64      `mapstructure:"cold_storage_threshold" yaml:"cold_storage_threshold" validate:"min=0"`
	HotWalletLimit      float64       `mapstructure:"hot_wallet_limit" yaml:"hot_wallet_limit" validate:"min=0"`
	ProcessingTimeout   time.Duration `mapstructure:"processing_timeout" yaml:"processing_timeout" validate:"required"`
	
	// Fireblocks integration
	Fireblocks          FireblocksConfig `mapstructure:"fireblocks" yaml:"fireblocks" validate:"required"`
}

// FireblocksConfig represents Fireblocks configuration
type FireblocksConfig struct {
	Enabled       bool   `mapstructure:"enabled" yaml:"enabled"`
	APIKey        string `mapstructure:"api_key" yaml:"api_key"`
	SecretKey     string `mapstructure:"secret_key" yaml:"secret_key"`
	BaseURL       string `mapstructure:"base_url" yaml:"base_url" validate:"url"`
	VaultAccountID string `mapstructure:"vault_account_id" yaml:"vault_account_id"`
}

// AccountsConfig holds accounts-specific configuration
type AccountsConfig struct {
	MaxAccountsPerUser   int           `mapstructure:"max_accounts_per_user" yaml:"max_accounts_per_user" validate:"min=1"`
	AccountLockTimeout   time.Duration `mapstructure:"account_lock_timeout" yaml:"account_lock_timeout" validate:"required"`
	BalanceCheckInterval time.Duration `mapstructure:"balance_check_interval" yaml:"balance_check_interval" validate:"required"`
	SuspiciousActivityThreshold float64 `mapstructure:"suspicious_activity_threshold" yaml:"suspicious_activity_threshold" validate:"min=0"`
}

// ComplianceConfig holds compliance-specific configuration
type ComplianceConfig struct {
	AMLEnabled               bool          `mapstructure:"aml_enabled" yaml:"aml_enabled"`
	TransactionReportingEnabled bool       `mapstructure:"transaction_reporting_enabled" yaml:"transaction_reporting_enabled"`
	SuspiciousActivityThreshold float64    `mapstructure:"suspicious_activity_threshold" yaml:"suspicious_activity_threshold" validate:"min=0"`
	ReportingInterval        time.Duration `mapstructure:"reporting_interval" yaml:"reporting_interval" validate:"required"`
	DataRetentionPeriod      time.Duration `mapstructure:"data_retention_period" yaml:"data_retention_period" validate:"required"`
}

// SecretsConfig holds secrets management configuration
type SecretsConfig struct {
	Provider    string               `mapstructure:"provider" yaml:"provider" validate:"required,oneof=env vault aws_secrets_manager"`
	VaultConfig VaultConfig          `mapstructure:"vault" yaml:"vault"`
	AWSConfig   AWSSecretsConfig     `mapstructure:"aws" yaml:"aws"`
}

// VaultConfig represents HashiCorp Vault configuration
type VaultConfig struct {
	Address   string `mapstructure:"address" yaml:"address" validate:"url"`
	Token     string `mapstructure:"token" yaml:"token"`
	Path      string `mapstructure:"path" yaml:"path"`
	TLSConfig TLSConfig `mapstructure:"tls" yaml:"tls"`
}

// AWSSecretsConfig represents AWS Secrets Manager configuration
type AWSSecretsConfig struct {
	Region          string `mapstructure:"region" yaml:"region"`
	AccessKeyID     string `mapstructure:"access_key_id" yaml:"access_key_id"`
	SecretAccessKey string `mapstructure:"secret_access_key" yaml:"secret_access_key"`
	SessionToken    string `mapstructure:"session_token" yaml:"session_token"`
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level       string `mapstructure:"level" yaml:"level" validate:"required,oneof=debug info warn error fatal"`
	Format      string `mapstructure:"format" yaml:"format" validate:"required,oneof=json console"`
	Output      string `mapstructure:"output" yaml:"output" validate:"required,oneof=stdout stderr file"`
	FilePath    string `mapstructure:"file_path" yaml:"file_path"`
	MaxSize     int    `mapstructure:"max_size" yaml:"max_size" validate:"min=1"`
	MaxBackups  int    `mapstructure:"max_backups" yaml:"max_backups" validate:"min=0"`
	MaxAge      int    `mapstructure:"max_age" yaml:"max_age" validate:"min=1"`
	Compress    bool   `mapstructure:"compress" yaml:"compress"`
}

// NewConfigManager creates a new configuration manager
func NewConfigManager(logger *zap.Logger) *ConfigManager {
	ctx, cancel := context.WithCancel(context.Background())
	
	return &ConfigManager{
		viper:           viper.New(),
		validator:       validator.New(),
		logger:          logger.Named("config"),
		ctx:             ctx,
		cancel:          cancel,
		reloadCallbacks: make([]ReloadCallback, 0),
		watchPaths:      make([]string, 0),
	}
}

// SetSecretProvider sets the secret provider for secure secret management
func (cm *ConfigManager) SetSecretProvider(provider SecretProvider) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.secretProvider = provider
}

// AddReloadCallback adds a callback to be called when configuration is reloaded
func (cm *ConfigManager) AddReloadCallback(callback ReloadCallback) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.reloadCallbacks = append(cm.reloadCallbacks, callback)
}

// GetConfig returns the current configuration (thread-safe)
func (cm *ConfigManager) GetConfig() *PlatformConfig {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.config
}

// IsInitialized returns whether the configuration has been loaded
func (cm *ConfigManager) IsInitialized() bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.initialized
}

// GetLastReloadTime returns the time of the last configuration reload
func (cm *ConfigManager) GetLastReloadTime() time.Time {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.lastReload
}

// Close shuts down the configuration manager
func (cm *ConfigManager) Close() error {
	cm.cancel()
	
	cm.mu.Lock()
	defer cm.mu.Unlock()
	
	if cm.watcher != nil {
		if err := cm.watcher.Close(); err != nil {
			return fmt.Errorf("failed to close file watcher: %w", err)
		}
	}
	
	if cm.secretProvider != nil {
		if err := cm.secretProvider.Close(); err != nil {
			return fmt.Errorf("failed to close secret provider: %w", err)
		}
	}
	
	return nil
}
