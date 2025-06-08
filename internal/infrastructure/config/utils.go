// Configuration utilities and helpers
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"
)

// ConfigBuilder provides a fluent interface for building configuration
type ConfigBuilder struct {
	manager *ConfigManager
	paths   []string
	logger  *zap.Logger
}

// NewConfigBuilder creates a new configuration builder
func NewConfigBuilder(logger *zap.Logger) *ConfigBuilder {
	return &ConfigBuilder{
		manager: NewConfigManager(logger),
		logger:  logger,
	}
}

// WithConfigPaths sets the configuration file paths
func (cb *ConfigBuilder) WithConfigPaths(paths ...string) *ConfigBuilder {
	cb.paths = paths
	return cb
}

// WithSecretProvider sets the secret provider
func (cb *ConfigBuilder) WithSecretProvider(provider SecretProvider) *ConfigBuilder {
	cb.manager.SetSecretProvider(provider)
	return cb
}

// WithReloadCallback adds a reload callback
func (cb *ConfigBuilder) WithReloadCallback(callback ReloadCallback) *ConfigBuilder {
	cb.manager.AddReloadCallback(callback)
	return cb
}

// Build loads the configuration and returns the manager
func (cb *ConfigBuilder) Build() (*ConfigManager, error) {
	if err := cb.manager.LoadConfig(cb.paths...); err != nil {
		return nil, fmt.Errorf("failed to build configuration: %w", err)
	}
	return cb.manager, nil
}

// GlobalConfigManager holds the global configuration manager instance
var GlobalConfigManager *ConfigManager

// InitializeGlobalConfig initializes the global configuration manager
func InitializeGlobalConfig(logger *zap.Logger, configPaths ...string) error {
	if GlobalConfigManager != nil {
		return fmt.Errorf("global configuration already initialized")
	}

	builder := NewConfigBuilder(logger)
	if len(configPaths) > 0 {
		builder = builder.WithConfigPaths(configPaths...)
	}

	// Create secret provider based on environment
	secretProvider := CreateDefaultSecretProvider(logger)
	if secretProvider != nil {
		builder = builder.WithSecretProvider(secretProvider)
	}

	manager, err := builder.Build()
	if err != nil {
		return fmt.Errorf("failed to initialize global configuration: %w", err)
	}

	GlobalConfigManager = manager
	return nil
}

// GetGlobalConfig returns the global configuration
func GetGlobalConfig() *PlatformConfig {
	if GlobalConfigManager == nil {
		panic("global configuration not initialized")
	}
	return GlobalConfigManager.GetConfig()
}

// GetGlobalConfigManager returns the global configuration manager
func GetGlobalConfigManager() *ConfigManager {
	if GlobalConfigManager == nil {
		panic("global configuration manager not initialized")
	}
	return GlobalConfigManager
}

// CreateDefaultSecretProvider creates a default secret provider based on environment
func CreateDefaultSecretProvider(logger *zap.Logger) SecretProvider {
	// Check for Vault configuration
	if vaultAddr := os.Getenv("VAULT_ADDR"); vaultAddr != "" {
		if vaultToken := os.Getenv("VAULT_TOKEN"); vaultToken != "" {
			vaultConfig := VaultConfig{
				Address: vaultAddr,
				Token:   vaultToken,
				Path:    getEnvWithDefault("VAULT_PATH", "secret/finalex"),
			}

			provider, err := NewVaultSecretProvider(vaultConfig, logger)
			if err != nil {
				logger.Warn("Failed to create Vault secret provider, falling back to env", zap.Error(err))
				return NewEnvSecretProvider(logger)
			}
			return provider
		}
	}

	// Check for AWS Secrets Manager configuration
	if awsRegion := os.Getenv("AWS_REGION"); awsRegion != "" {
		awsConfig := AWSSecretsConfig{
			Region:          awsRegion,
			AccessKeyID:     os.Getenv("AWS_ACCESS_KEY_ID"),
			SecretAccessKey: os.Getenv("AWS_SECRET_ACCESS_KEY"),
			SessionToken:    os.Getenv("AWS_SESSION_TOKEN"),
		}

		provider, err := NewAWSSecretsProvider(awsConfig, logger)
		if err != nil {
			logger.Warn("Failed to create AWS Secrets Manager provider, falling back to env", zap.Error(err))
			return NewEnvSecretProvider(logger)
		}
		return provider
	}

	// Default to environment variables
	return NewEnvSecretProvider(logger)
}

// getEnvWithDefault gets an environment variable with a default value
func getEnvWithDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// ValidateConfigFile validates a configuration file
func ValidateConfigFile(filePath string, logger *zap.Logger) error {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return fmt.Errorf("configuration file not found: %s", filePath)
	}

	// Create temporary config manager for validation
	manager := NewConfigManager(logger)
	if err := manager.LoadConfig(filePath); err != nil {
		return fmt.Errorf("failed to load configuration file: %w", err)
	}

	logger.Info("Configuration file is valid", zap.String("file", filePath))
	return nil
}

// GenerateConfigTemplate generates a configuration template file
func GenerateConfigTemplate(outputPath string) error {
	template := `# Finalex Platform Configuration Template
version: "1.0.0"
environment: development

# Server Configuration
server:
  host: "0.0.0.0"
  port: 8080
  tls_enabled: false
  tls_cert_file: ""
  tls_key_file: ""
  read_timeout: "30s"
  write_timeout: "30s"
  idle_timeout: "120s"
  graceful_shutdown_timeout: "15s"

# Database Configuration
database:
  driver: "postgres"
  dsn: "postgres://postgres:postgres@localhost:5432/finalex?sslmode=disable"
  max_open_conns: 25
  max_idle_conns: 5
  conn_max_lifetime: "5m"
  conn_max_idle_time: "2m"
  migrations_path: "./migrations"
  auto_migrate: false

# Redis Configuration
redis:
  address: "localhost:6379"
  password: ""
  db: 0
  pool_size: 10
  dial_timeout: "5s"
  read_timeout: "3s"
  write_timeout: "3s"

# JWT Configuration
jwt:
  secret: "secret://jwt.secret"
  expiration_hours: 24
  refresh_secret: "secret://jwt.refresh_secret"
  refresh_exp_hours: 168
  issuer: "finalex"
  audience: "finalex-users"
  algorithm: "HS256"
  clock_skew: "5m"

# Security Configuration
security:
  cors:
    allowed_origins: ["http://localhost:3000"]
    allowed_methods: ["GET", "POST", "PUT", "DELETE", "OPTIONS"]
    allowed_headers: ["*"]
    allow_credentials: true
    max_age: "12h"
  rate_limit:
    enabled: true
    requests_per_min: 100
    burst_size: 200
    cleanup_interval: "5m"

# KYC Configuration
kyc:
  provider_url: "https://kyc-provider.example.com"
  provider_api_key: "secret://kyc.api_key"
  document_base_path: "/var/lib/finalex/kyc-documents"
  max_file_size: 10485760  # 10MB
  allowed_mime_types:
    - "image/jpeg"
    - "image/png"
    - "application/pdf"
  verification_timeout: "5m"

# Kafka Configuration
kafka:
  brokers: ["localhost:9092"]
  enable_message_queue: true
  consumer_group_prefix: "finalex"
  producer:
    batch_size: 16384
    linger_ms: 5
    compression_type: "snappy"
    max_retries: 3
    retry_backoff_ms: 100
    request_timeout_ms: 30000
  consumer:
    session_timeout_ms: 10000
    heartbeat_ms: 3000
    auto_offset_reset: "latest"
    enable_auto_commit: true
    max_poll_records: 500

# Monitoring Configuration
monitoring:
  enabled: true
  metrics_port: 9090
  health_port: 9091
  prometheus_path: "/metrics"
  tracing:
    enabled: false
    service_name: "finalex"
    endpoint: ""
    sample_rate: 0.1
  alerting:
    enabled: false
    thresholds:
      error_rate: 0.05
      timeout_rate: 0.02
      response_time_ms: 1000
      memory_usage: 0.8
      cpu_usage: 0.8

# Trading Configuration
trading:
  max_order_size: 1000000
  min_order_size: 0.01
  default_fee_rate: 0.001
  max_open_orders: 100
  order_book_depth: 50
  matching_timeout: "1s"
  risk_check_enabled: true
  circuit_breaker_enabled: true

# Wallet Configuration
wallet:
  min_confirmations: 3
  max_withdrawal_amount: 100000
  withdrawal_fee_rate: 0.001
  cold_storage_threshold: 10000
  hot_wallet_limit: 50000
  processing_timeout: "5m"
  fireblocks:
    enabled: false
    api_key: "secret://fireblocks.api_key"
    secret_key: "secret://fireblocks.secret_key"
    base_url: "https://api.fireblocks.io"
    vault_account_id: ""

# Accounts Configuration
accounts:
  max_accounts_per_user: 10
  account_lock_timeout: "30m"
  balance_check_interval: "1m"
  suspicious_activity_threshold: 10000

# Compliance Configuration
compliance:
  aml_enabled: true
  transaction_reporting_enabled: true
  suspicious_activity_threshold: 10000
  reporting_interval: "24h"
  data_retention_period: "8760h"  # 1 year

# Secrets Configuration
secrets:
  provider: "env"  # Options: env, vault, aws_secrets_manager

# Logging Configuration
logging:
  level: "info"
  format: "json"
  output: "stdout"
  max_size: 100
  max_backups: 3
  max_age: 30
  compress: true
`

	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	if err := os.WriteFile(outputPath, []byte(template), 0644); err != nil {
		return fmt.Errorf("failed to write template file: %w", err)
	}

	return nil
}

// ConfigDiff represents a difference between two configurations
type ConfigDiff struct {
	Path     string
	OldValue interface{}
	NewValue interface{}
}

// CompareConfigs compares two configurations and returns differences
func CompareConfigs(oldConfig, newConfig *PlatformConfig) []ConfigDiff {
	var diffs []ConfigDiff

	// This is a simplified implementation
	// In a real scenario, you'd use reflection to compare all fields

	if oldConfig.Environment != newConfig.Environment {
		diffs = append(diffs, ConfigDiff{
			Path:     "environment",
			OldValue: oldConfig.Environment,
			NewValue: newConfig.Environment,
		})
	}

	if oldConfig.Server.Port != newConfig.Server.Port {
		diffs = append(diffs, ConfigDiff{
			Path:     "server.port",
			OldValue: oldConfig.Server.Port,
			NewValue: newConfig.Server.Port,
		})
	}

	// Add more comparisons as needed...

	return diffs
}

// ConfigHealth represents the health status of configuration
type ConfigHealth struct {
	Healthy     bool      `json:"healthy"`
	LastReload  time.Time `json:"last_reload"`
	Version     string    `json:"version"`
	Environment string    `json:"environment"`
	Issues      []string  `json:"issues,omitempty"`
}

// GetConfigHealth returns the health status of the configuration
func GetConfigHealth() ConfigHealth {
	if GlobalConfigManager == nil {
		return ConfigHealth{
			Healthy: false,
			Issues:  []string{"Configuration manager not initialized"},
		}
	}

	config := GlobalConfigManager.GetConfig()
	health := ConfigHealth{
		Healthy:     true,
		LastReload:  GlobalConfigManager.GetLastReloadTime(),
		Version:     config.Version,
		Environment: config.Environment,
		Issues:      []string{},
	}

	// Check for potential issues
	if time.Since(health.LastReload) > 24*time.Hour {
		health.Issues = append(health.Issues, "Configuration hasn't been reloaded in over 24 hours")
	}

	if config.Environment == "production" {
		if strings.Contains(config.JWT.Secret, "change-this") {
			health.Healthy = false
			health.Issues = append(health.Issues, "Production environment with insecure JWT secret")
		}

		if !config.Server.TLSEnabled {
			health.Issues = append(health.Issues, "Production environment without TLS enabled")
		}
	}

	return health
}

// ExportConfig exports the current configuration (with secrets masked)
func ExportConfig() map[string]interface{} {
	if GlobalConfigManager == nil {
		return nil
	}

	config := GlobalConfigManager.GetConfig()

	// Convert to map and mask secrets
	// This is a simplified implementation
	exported := map[string]interface{}{
		"version":     config.Version,
		"environment": config.Environment,
		"server": map[string]interface{}{
			"host": config.Server.Host,
			"port": config.Server.Port,
		},
		"database": map[string]interface{}{
			"driver":         config.Database.Driver,
			"dsn":            maskSecret(config.Database.DSN),
			"max_open_conns": config.Database.MaxOpenConns,
			"max_idle_conns": config.Database.MaxIdleConns,
		},
		"redis": map[string]interface{}{
			"address":  config.Redis.Address,
			"password": maskSecret(config.Redis.Password),
			"db":       config.Redis.DB,
		},
	}

	return exported
}

// maskSecret masks sensitive values in configuration export
func maskSecret(value string) string {
	if value == "" {
		return ""
	}
	if len(value) <= 4 {
		return "****"
	}
	return value[:2] + strings.Repeat("*", len(value)-4) + value[len(value)-2:]
}
