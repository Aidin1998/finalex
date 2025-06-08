// Config loader with hot-reload and validation capabilities
package config

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

// LoadConfig loads configuration from multiple sources with validation
func (cm *ConfigManager) LoadConfig(configPaths ...string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.logger.Info("Loading platform configuration", zap.Strings("paths", configPaths))

	// Initialize viper
	cm.setupViper()

	// Load configuration from files
	if err := cm.loadConfigFiles(configPaths...); err != nil {
		return fmt.Errorf("failed to load config files: %w", err)
	}

	// Load from environment variables
	cm.loadEnvironmentVariables()

	// Create config struct
	var config PlatformConfig
	if err := cm.viper.Unmarshal(&config); err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Set defaults
	cm.setDefaults(&config)

	// Load secrets
	if err := cm.loadSecrets(context.Background(), &config); err != nil {
		return fmt.Errorf("failed to load secrets: %w", err)
	}

	// Validate configuration
	if err := cm.validateConfig(&config); err != nil {
		return fmt.Errorf("configuration validation failed: %w", err)
	}

	// Start file watcher for hot-reload
	if err := cm.startWatcher(configPaths...); err != nil {
		return fmt.Errorf("failed to start file watcher: %w", err)
	}

	cm.config = &config
	cm.initialized = true
	cm.lastReload = time.Now()

	cm.logger.Info("Configuration loaded successfully",
		zap.String("version", config.Version),
		zap.String("environment", config.Environment),
		zap.Time("loaded_at", cm.lastReload))

	return nil
}

// setupViper configures viper settings
func (cm *ConfigManager) setupViper() {
	cm.viper.SetConfigType("yaml")
	cm.viper.AutomaticEnv()
	cm.viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	cm.viper.SetEnvPrefix("FINALEX")
}

// loadConfigFiles loads configuration from YAML files
func (cm *ConfigManager) loadConfigFiles(configPaths ...string) error {
	if len(configPaths) == 0 {
		// Default config paths
		configPaths = []string{
			"./config.yaml",
			"./configs/config.yaml",
			"/etc/finalex/config.yaml",
		}
	}

	var loadedFiles []string

	for _, path := range configPaths {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			cm.logger.Debug("Config file not found, skipping", zap.String("path", path))
			continue
		}

		cm.viper.SetConfigFile(path)
		if err := cm.viper.MergeInConfig(); err != nil {
			return fmt.Errorf("failed to load config file %s: %w", path, err)
		}

		loadedFiles = append(loadedFiles, path)
		cm.watchPaths = append(cm.watchPaths, path)
	}

	if len(loadedFiles) == 0 {
		cm.logger.Warn("No configuration files found, using defaults and environment variables")
	} else {
		cm.logger.Info("Loaded configuration files", zap.Strings("files", loadedFiles))
	}

	return nil
}

// loadEnvironmentVariables loads configuration from environment variables
func (cm *ConfigManager) loadEnvironmentVariables() {
	// Define environment variable mappings
	envMappings := map[string]string{
		"FINALEX_VERSION":     "version",
		"FINALEX_ENVIRONMENT": "environment",

		// Server
		"FINALEX_SERVER_HOST":          "server.host",
		"FINALEX_SERVER_PORT":          "server.port",
		"FINALEX_SERVER_TLS_ENABLED":   "server.tls_enabled",
		"FINALEX_SERVER_TLS_CERT_FILE": "server.tls_cert_file",
		"FINALEX_SERVER_TLS_KEY_FILE":  "server.tls_key_file",

		// Database
		"FINALEX_DATABASE_DRIVER":         "database.driver",
		"FINALEX_DATABASE_DSN":            "database.dsn",
		"FINALEX_DATABASE_COCKROACH_DSN":  "database.cockroach_dsn",
		"FINALEX_DATABASE_MAX_OPEN_CONNS": "database.max_open_conns",
		"FINALEX_DATABASE_MAX_IDLE_CONNS": "database.max_idle_conns",

		// Redis
		"FINALEX_REDIS_ADDRESS":  "redis.address",
		"FINALEX_REDIS_PASSWORD": "redis.password",
		"FINALEX_REDIS_DB":       "redis.db",

		// JWT
		"FINALEX_JWT_SECRET":            "jwt.secret",
		"FINALEX_JWT_EXPIRATION_HOURS":  "jwt.expiration_hours",
		"FINALEX_JWT_REFRESH_SECRET":    "jwt.refresh_secret",
		"FINALEX_JWT_REFRESH_EXP_HOURS": "jwt.refresh_exp_hours",
		"FINALEX_JWT_ISSUER":            "jwt.issuer",
		"FINALEX_JWT_AUDIENCE":          "jwt.audience",

		// KYC
		"FINALEX_KYC_PROVIDER_URL":       "kyc.provider_url",
		"FINALEX_KYC_PROVIDER_API_KEY":   "kyc.provider_api_key",
		"FINALEX_KYC_DOCUMENT_BASE_PATH": "kyc.document_base_path",

		// Kafka
		"FINALEX_KAFKA_BROKERS":               "kafka.brokers",
		"FINALEX_KAFKA_ENABLE_MESSAGE_QUEUE":  "kafka.enable_message_queue",
		"FINALEX_KAFKA_CONSUMER_GROUP_PREFIX": "kafka.consumer_group_prefix",

		// Monitoring
		"FINALEX_MONITORING_ENABLED":      "monitoring.enabled",
		"FINALEX_MONITORING_METRICS_PORT": "monitoring.metrics_port",
		"FINALEX_MONITORING_HEALTH_PORT":  "monitoring.health_port",

		// Secrets
		"FINALEX_SECRETS_PROVIDER": "secrets.provider",
		"FINALEX_VAULT_ADDRESS":    "secrets.vault.address",
		"FINALEX_VAULT_TOKEN":      "secrets.vault.token",
		"FINALEX_VAULT_PATH":       "secrets.vault.path",

		// Logging
		"FINALEX_LOGGING_LEVEL":     "logging.level",
		"FINALEX_LOGGING_FORMAT":    "logging.format",
		"FINALEX_LOGGING_OUTPUT":    "logging.output",
		"FINALEX_LOGGING_FILE_PATH": "logging.file_path",
	}

	for envVar, configKey := range envMappings {
		if value := os.Getenv(envVar); value != "" {
			cm.viper.Set(configKey, value)
		}
	}

	cm.logger.Debug("Environment variables loaded")
}

// setDefaults sets default configuration values
func (cm *ConfigManager) setDefaults(config *PlatformConfig) {
	if config.Version == "" {
		config.Version = ConfigVersion
	}

	if config.Environment == "" {
		config.Environment = "development"
	}

	// Server defaults
	if config.Server.Host == "" {
		config.Server.Host = "0.0.0.0"
	}
	if config.Server.Port == 0 {
		config.Server.Port = 8080
	}
	if config.Server.ReadTimeout == 0 {
		config.Server.ReadTimeout = 30 * time.Second
	}
	if config.Server.WriteTimeout == 0 {
		config.Server.WriteTimeout = 30 * time.Second
	}
	if config.Server.IdleTimeout == 0 {
		config.Server.IdleTimeout = 120 * time.Second
	}
	if config.Server.GracefulShutdownTimeout == 0 {
		config.Server.GracefulShutdownTimeout = 15 * time.Second
	}

	// Database defaults
	if config.Database.Driver == "" {
		config.Database.Driver = "postgres"
	}
	if config.Database.DSN == "" {
		config.Database.DSN = "postgres://postgres:postgres@localhost:5432/finalex?sslmode=disable"
	}
	if config.Database.MaxOpenConns == 0 {
		config.Database.MaxOpenConns = 25
	}
	if config.Database.MaxIdleConns == 0 {
		config.Database.MaxIdleConns = 5
	}
	if config.Database.ConnMaxLifetime == 0 {
		config.Database.ConnMaxLifetime = 5 * time.Minute
	}
	if config.Database.ConnMaxIdleTime == 0 {
		config.Database.ConnMaxIdleTime = 2 * time.Minute
	}
	if config.Database.MigrationsPath == "" {
		config.Database.MigrationsPath = "./migrations"
	}

	// Redis defaults
	if config.Redis.Address == "" {
		config.Redis.Address = "localhost:6379"
	}
	if config.Redis.PoolSize == 0 {
		config.Redis.PoolSize = 10
	}
	if config.Redis.DialTimeout == 0 {
		config.Redis.DialTimeout = 5 * time.Second
	}
	if config.Redis.ReadTimeout == 0 {
		config.Redis.ReadTimeout = 3 * time.Second
	}
	if config.Redis.WriteTimeout == 0 {
		config.Redis.WriteTimeout = 3 * time.Second
	}

	// JWT defaults
	if config.JWT.Secret == "" {
		config.JWT.Secret = "your-super-secret-jwt-key-change-this-in-production"
	}
	if config.JWT.ExpirationHours == 0 {
		config.JWT.ExpirationHours = 24
	}
	if config.JWT.RefreshSecret == "" {
		config.JWT.RefreshSecret = "your-super-secret-refresh-key-change-this-in-production"
	}
	if config.JWT.RefreshExpHours == 0 {
		config.JWT.RefreshExpHours = 168 // 7 days
	}
	if config.JWT.Issuer == "" {
		config.JWT.Issuer = "finalex"
	}
	if config.JWT.Audience == "" {
		config.JWT.Audience = "finalex-users"
	}
	if config.JWT.Algorithm == "" {
		config.JWT.Algorithm = "HS256"
	}
	if config.JWT.ClockSkew == 0 {
		config.JWT.ClockSkew = 5 * time.Minute
	}

	// Security defaults
	if len(config.Security.CORS.AllowedOrigins) == 0 {
		config.Security.CORS.AllowedOrigins = []string{"*"}
	}
	if len(config.Security.CORS.AllowedMethods) == 0 {
		config.Security.CORS.AllowedMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
	}
	if len(config.Security.CORS.AllowedHeaders) == 0 {
		config.Security.CORS.AllowedHeaders = []string{"*"}
	}
	if config.Security.CORS.MaxAge == 0 {
		config.Security.CORS.MaxAge = 12 * time.Hour
	}

	if config.Security.RateLimit.RequestsPerMin == 0 {
		config.Security.RateLimit.RequestsPerMin = 100
	}
	if config.Security.RateLimit.BurstSize == 0 {
		config.Security.RateLimit.BurstSize = 200
	}
	if config.Security.RateLimit.CleanupInterval == 0 {
		config.Security.RateLimit.CleanupInterval = 5 * time.Minute
	}

	// KYC defaults
	if config.KYC.ProviderURL == "" {
		config.KYC.ProviderURL = "https://kyc-provider.example.com"
	}
	if config.KYC.DocumentBasePath == "" {
		config.KYC.DocumentBasePath = "/var/lib/finalex/kyc-documents"
	}
	if config.KYC.MaxFileSize == 0 {
		config.KYC.MaxFileSize = 10 * 1024 * 1024 // 10MB
	}
	if len(config.KYC.AllowedMimeTypes) == 0 {
		config.KYC.AllowedMimeTypes = []string{
			"image/jpeg", "image/png", "image/gif",
			"application/pdf", "application/msword",
		}
	}
	if config.KYC.VerificationTimeout == 0 {
		config.KYC.VerificationTimeout = 5 * time.Minute
	}

	// Kafka defaults
	if len(config.Kafka.Brokers) == 0 {
		config.Kafka.Brokers = []string{"localhost:9092"}
	}
	if config.Kafka.ConsumerGroupPrefix == "" {
		config.Kafka.ConsumerGroupPrefix = "finalex"
	}

	// Producer defaults
	if config.Kafka.ProducerConfig.BatchSize == 0 {
		config.Kafka.ProducerConfig.BatchSize = 16384
	}
	if config.Kafka.ProducerConfig.LingerMs == 0 {
		config.Kafka.ProducerConfig.LingerMs = 5
	}
	if config.Kafka.ProducerConfig.CompressionType == "" {
		config.Kafka.ProducerConfig.CompressionType = "snappy"
	}
	if config.Kafka.ProducerConfig.MaxRetries == 0 {
		config.Kafka.ProducerConfig.MaxRetries = 3
	}
	if config.Kafka.ProducerConfig.RetryBackoffMs == 0 {
		config.Kafka.ProducerConfig.RetryBackoffMs = 100
	}
	if config.Kafka.ProducerConfig.RequestTimeoutMs == 0 {
		config.Kafka.ProducerConfig.RequestTimeoutMs = 30000
	}

	// Consumer defaults
	if config.Kafka.ConsumerConfig.SessionTimeoutMs == 0 {
		config.Kafka.ConsumerConfig.SessionTimeoutMs = 10000
	}
	if config.Kafka.ConsumerConfig.HeartbeatMs == 0 {
		config.Kafka.ConsumerConfig.HeartbeatMs = 3000
	}
	if config.Kafka.ConsumerConfig.AutoOffsetReset == "" {
		config.Kafka.ConsumerConfig.AutoOffsetReset = "latest"
	}
	if config.Kafka.ConsumerConfig.MaxPollRecords == 0 {
		config.Kafka.ConsumerConfig.MaxPollRecords = 500
	}

	// Monitoring defaults
	if config.Monitoring.MetricsPort == 0 {
		config.Monitoring.MetricsPort = 9090
	}
	if config.Monitoring.HealthPort == 0 {
		config.Monitoring.HealthPort = 9091
	}
	if config.Monitoring.PrometheusPath == "" {
		config.Monitoring.PrometheusPath = "/metrics"
	}

	// Tracing defaults
	if config.Monitoring.Tracing.ServiceName == "" {
		config.Monitoring.Tracing.ServiceName = "finalex"
	}
	if config.Monitoring.Tracing.SampleRate == 0 {
		config.Monitoring.Tracing.SampleRate = 0.1
	}

	// Alert thresholds defaults
	if config.Monitoring.Alerting.Thresholds.ErrorRate == 0 {
		config.Monitoring.Alerting.Thresholds.ErrorRate = 0.05
	}
	if config.Monitoring.Alerting.Thresholds.TimeoutRate == 0 {
		config.Monitoring.Alerting.Thresholds.TimeoutRate = 0.02
	}
	if config.Monitoring.Alerting.Thresholds.ResponseTime == 0 {
		config.Monitoring.Alerting.Thresholds.ResponseTime = 1000
	}
	if config.Monitoring.Alerting.Thresholds.MemoryUsage == 0 {
		config.Monitoring.Alerting.Thresholds.MemoryUsage = 0.8
	}
	if config.Monitoring.Alerting.Thresholds.CPUUsage == 0 {
		config.Monitoring.Alerting.Thresholds.CPUUsage = 0.8
	}

	// Trading defaults
	if config.Trading.MaxOrderSize == 0 {
		config.Trading.MaxOrderSize = 1000000
	}
	if config.Trading.MinOrderSize == 0 {
		config.Trading.MinOrderSize = 0.01
	}
	if config.Trading.DefaultFeeRate == 0 {
		config.Trading.DefaultFeeRate = 0.001
	}
	if config.Trading.MaxOpenOrders == 0 {
		config.Trading.MaxOpenOrders = 100
	}
	if config.Trading.OrderBookDepth == 0 {
		config.Trading.OrderBookDepth = 50
	}
	if config.Trading.MatchingTimeout == 0 {
		config.Trading.MatchingTimeout = 1 * time.Second
	}

	// Wallet defaults
	if config.Wallet.MinConfirmations == 0 {
		config.Wallet.MinConfirmations = 3
	}
	if config.Wallet.MaxWithdrawalAmount == 0 {
		config.Wallet.MaxWithdrawalAmount = 100000
	}
	if config.Wallet.WithdrawalFeeRate == 0 {
		config.Wallet.WithdrawalFeeRate = 0.001
	}
	if config.Wallet.ColdStorageThreshold == 0 {
		config.Wallet.ColdStorageThreshold = 10000
	}
	if config.Wallet.HotWalletLimit == 0 {
		config.Wallet.HotWalletLimit = 50000
	}
	if config.Wallet.ProcessingTimeout == 0 {
		config.Wallet.ProcessingTimeout = 5 * time.Minute
	}
	if config.Wallet.Fireblocks.BaseURL == "" {
		config.Wallet.Fireblocks.BaseURL = "https://api.fireblocks.io"
	}

	// Accounts defaults
	if config.Accounts.MaxAccountsPerUser == 0 {
		config.Accounts.MaxAccountsPerUser = 10
	}
	if config.Accounts.AccountLockTimeout == 0 {
		config.Accounts.AccountLockTimeout = 30 * time.Minute
	}
	if config.Accounts.BalanceCheckInterval == 0 {
		config.Accounts.BalanceCheckInterval = 1 * time.Minute
	}
	if config.Accounts.SuspiciousActivityThreshold == 0 {
		config.Accounts.SuspiciousActivityThreshold = 10000
	}

	// Compliance defaults
	if config.Compliance.SuspiciousActivityThreshold == 0 {
		config.Compliance.SuspiciousActivityThreshold = 10000
	}
	if config.Compliance.ReportingInterval == 0 {
		config.Compliance.ReportingInterval = 24 * time.Hour
	}
	if config.Compliance.DataRetentionPeriod == 0 {
		config.Compliance.DataRetentionPeriod = 365 * 24 * time.Hour // 1 year
	}

	// Secrets defaults
	if config.Secrets.Provider == "" {
		config.Secrets.Provider = "env"
	}

	// Logging defaults
	if config.Logging.Level == "" {
		config.Logging.Level = "info"
	}
	if config.Logging.Format == "" {
		config.Logging.Format = "json"
	}
	if config.Logging.Output == "" {
		config.Logging.Output = "stdout"
	}
	if config.Logging.MaxSize == 0 {
		config.Logging.MaxSize = 100
	}
	if config.Logging.MaxBackups == 0 {
		config.Logging.MaxBackups = 3
	}
	if config.Logging.MaxAge == 0 {
		config.Logging.MaxAge = 30
	}
}

// validateConfig validates the configuration
func (cm *ConfigManager) validateConfig(config *PlatformConfig) error {
	if err := cm.validator.Struct(config); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Custom validation rules
	if err := cm.validateCustomRules(config); err != nil {
		return fmt.Errorf("custom validation failed: %w", err)
	}

	return nil
}

// validateCustomRules performs additional custom validation
func (cm *ConfigManager) validateCustomRules(config *PlatformConfig) error {
	// Validate environment-specific rules
	switch config.Environment {
	case "production":
		if strings.Contains(config.JWT.Secret, "change-this") ||
			strings.Contains(config.JWT.RefreshSecret, "change-this") {
			return fmt.Errorf("production environment requires secure JWT secrets")
		}

		if !config.Server.TLSEnabled {
			return fmt.Errorf("production environment requires TLS to be enabled")
		}

		if config.Security.CORS.AllowedOrigins[0] == "*" {
			return fmt.Errorf("production environment should not allow all CORS origins")
		}

	case "staging":
		if strings.Contains(config.JWT.Secret, "change-this") {
			cm.logger.Warn("Staging environment should use secure JWT secrets")
		}
	}

	// Validate TLS configuration
	if config.Server.TLSEnabled {
		if config.Server.TLSCertFile == "" || config.Server.TLSKeyFile == "" {
			return fmt.Errorf("TLS is enabled but cert/key files are not specified")
		}
	}

	// Validate database sharding
	if len(config.Database.Shards) > 0 {
		for i, shard := range config.Database.Shards {
			if shard.Name == "" {
				return fmt.Errorf("shard %d has empty name", i)
			}
			if shard.MasterDSN == "" {
				return fmt.Errorf("shard %s has empty master DSN", shard.Name)
			}
		}
	}

	// Validate Kafka configuration
	if config.Kafka.EnableMessageQueue && len(config.Kafka.Brokers) == 0 {
		return fmt.Errorf("Kafka is enabled but no brokers are configured")
	}

	// Validate monitoring ports
	if config.Monitoring.MetricsPort == config.Monitoring.HealthPort {
		return fmt.Errorf("metrics and health ports cannot be the same")
	}

	if config.Monitoring.MetricsPort == config.Server.Port ||
		config.Monitoring.HealthPort == config.Server.Port {
		return fmt.Errorf("monitoring ports cannot be the same as server port")
	}

	return nil
}

// startWatcher starts the file watcher for hot-reload
func (cm *ConfigManager) startWatcher(configPaths ...string) error {
	if len(cm.watchPaths) == 0 {
		cm.logger.Info("No config files to watch, hot-reload disabled")
		return nil
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to create file watcher: %w", err)
	}

	cm.watcher = watcher

	// Add paths to watcher
	for _, path := range cm.watchPaths {
		if err := watcher.Add(path); err != nil {
			cm.logger.Warn("Failed to watch config file", zap.String("path", path), zap.Error(err))
		} else {
			cm.logger.Debug("Watching config file", zap.String("path", path))
		}
	}

	// Start watcher goroutine
	go cm.watchForChanges()

	cm.logger.Info("File watcher started for hot-reload", zap.Strings("paths", cm.watchPaths))

	return nil
}

// watchForChanges handles file system events for configuration hot-reload
func (cm *ConfigManager) watchForChanges() {
	debounceTimer := time.NewTimer(0)
	debounceTimer.Stop()

	for {
		select {
		case <-cm.ctx.Done():
			return

		case event, ok := <-cm.watcher.Events:
			if !ok {
				return
			}

			if event.Op&fsnotify.Write == fsnotify.Write ||
				event.Op&fsnotify.Create == fsnotify.Create {

				cm.logger.Debug("Config file changed",
					zap.String("file", event.Name),
					zap.String("operation", event.Op.String()))

				// Debounce rapid file changes
				debounceTimer.Reset(500 * time.Millisecond)
			}

		case err, ok := <-cm.watcher.Errors:
			if !ok {
				return
			}
			cm.logger.Error("File watcher error", zap.Error(err))

		case <-debounceTimer.C:
			if err := cm.reloadConfig(); err != nil {
				cm.logger.Error("Failed to reload configuration", zap.Error(err))
			}
		}
	}
}

// reloadConfig reloads the configuration
func (cm *ConfigManager) reloadConfig() error {
	cm.logger.Info("Reloading configuration...")

	// Get current config for comparison
	cm.mu.RLock()
	oldConfig := cm.config
	cm.mu.RUnlock()

	// Create new viper instance for reload
	newViper := viper.New()
	cm.setupViper()

	// Temporarily replace viper
	originalViper := cm.viper
	cm.viper = newViper

	// Load configuration
	if err := cm.loadConfigFiles(cm.watchPaths...); err != nil {
		cm.viper = originalViper // Restore original viper
		return fmt.Errorf("failed to reload config files: %w", err)
	}

	cm.loadEnvironmentVariables()

	// Create new config struct
	var newConfig PlatformConfig
	if err := cm.viper.Unmarshal(&newConfig); err != nil {
		cm.viper = originalViper // Restore original viper
		return fmt.Errorf("failed to unmarshal reloaded config: %w", err)
	}

	// Set defaults and load secrets
	cm.setDefaults(&newConfig)

	if err := cm.loadSecrets(context.Background(), &newConfig); err != nil {
		cm.viper = originalViper // Restore original viper
		return fmt.Errorf("failed to load secrets during reload: %w", err)
	}

	// Validate new configuration
	if err := cm.validateConfig(&newConfig); err != nil {
		cm.viper = originalViper // Restore original viper
		return fmt.Errorf("reloaded configuration validation failed: %w", err)
	}

	// Execute reload callbacks
	cm.mu.RLock()
	callbacks := make([]ReloadCallback, len(cm.reloadCallbacks))
	copy(callbacks, cm.reloadCallbacks)
	cm.mu.RUnlock()

	for _, callback := range callbacks {
		if err := callback(oldConfig, &newConfig); err != nil {
			cm.viper = originalViper // Restore original viper
			return fmt.Errorf("reload callback failed: %w", err)
		}
	}

	// Update configuration atomically
	cm.mu.Lock()
	cm.config = &newConfig
	cm.lastReload = time.Now()
	cm.mu.Unlock()

	cm.logger.Info("Configuration reloaded successfully",
		zap.Time("reloaded_at", cm.lastReload))

	return nil
}
