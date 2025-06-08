// Configuration loading and validation for the database submodule
package database

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// LoadConfigFromEnvironment loads database configuration from environment variables
func LoadConfigFromEnvironment(config *DatabaseConfig) error {
	// Master connection configuration
	if dsn := os.Getenv("DB_MASTER_DSN"); dsn != "" {
		config.MasterDSN = dsn
	}

	if maxOpen := os.Getenv("DB_MASTER_MAX_OPEN"); maxOpen != "" {
		if val, err := strconv.Atoi(maxOpen); err == nil {
			config.MaxOpenConns = val
		}
	}

	if maxIdle := os.Getenv("DB_MASTER_MAX_IDLE"); maxIdle != "" {
		if val, err := strconv.Atoi(maxIdle); err == nil {
			config.MaxIdleConns = val
		}
	}

	if maxLifetime := os.Getenv("DB_MASTER_MAX_LIFETIME"); maxLifetime != "" {
		if val, err := time.ParseDuration(maxLifetime); err == nil {
			config.ConnMaxLifetime = val
		}
	}

	// Slave configurations
	if slaveDSNs := os.Getenv("DB_SLAVE_DSNS"); slaveDSNs != "" {
		dsns := strings.Split(slaveDSNs, ",")
		config.SlaveDSNs = make([]string, len(dsns))
		for i, dsn := range dsns {
			config.SlaveDSNs[i] = strings.TrimSpace(dsn)
		}
	}

	// Health check configuration
	if interval := os.Getenv("DB_HEALTH_CHECK_INTERVAL"); interval != "" {
		if val, err := time.ParseDuration(interval); err == nil {
			config.HealthCheckInterval = val
		}
	}

	if timeout := os.Getenv("DB_CONNECTION_TIMEOUT"); timeout != "" {
		if val, err := time.ParseDuration(timeout); err == nil {
			config.ConnectionTimeout = val
		}
	}

	if queryTimeout := os.Getenv("DB_QUERY_TIMEOUT"); queryTimeout != "" {
		if val, err := time.ParseDuration(queryTimeout); err == nil {
			config.QueryTimeout = val
		}
	}

	// Encryption configuration
	if key := os.Getenv("DB_ENCRYPTION_KEY"); key != "" {
		config.EncryptionKey = key
	}

	// Metrics configuration
	if enabled := os.Getenv("DB_METRICS_ENABLED"); enabled != "" {
		if val, err := strconv.ParseBool(enabled); err == nil {
			config.EnableMetrics = val
		}
	}

	if prefix := os.Getenv("DB_METRICS_PREFIX"); prefix != "" {
		config.MetricsPrefix = prefix
	}

	// SSL configuration
	if sslMode := os.Getenv("DB_SSL_MODE"); sslMode != "" {
		config.SSLMode = sslMode
	}

	if sslCert := os.Getenv("DB_SSL_CERT"); sslCert != "" {
		config.SSLCert = sslCert
	}

	if sslKey := os.Getenv("DB_SSL_KEY"); sslKey != "" {
		config.SSLKey = sslKey
	}

	if sslRootCert := os.Getenv("DB_SSL_ROOT_CERT"); sslRootCert != "" {
		config.SSLRootCert = sslRootCert
	}

	return nil
}

// ValidateConfig validates database configuration
func ValidateConfig(config *DatabaseConfig) error {
	// Validate master DSN
	if config.MasterDSN == "" {
		return fmt.Errorf("master DSN is required")
	}

	// Validate connection pool settings
	if config.MaxOpenConns < 0 {
		return fmt.Errorf("MaxOpenConns must be non-negative")
	}
	if config.MaxIdleConns < 0 {
		return fmt.Errorf("MaxIdleConns must be non-negative")
	}
	if config.MaxIdleConns > config.MaxOpenConns && config.MaxOpenConns > 0 {
		return fmt.Errorf("MaxIdleConns cannot be greater than MaxOpenConns")
	}

	// Validate health check settings
	if config.HealthCheckInterval > 0 && config.HealthCheckInterval < time.Second {
		return fmt.Errorf("health check interval must be at least 1 second")
	}

	// Validate encryption settings
	if config.EncryptionKey != "" && len(config.EncryptionKey) != 32 {
		return fmt.Errorf("encryption key must be exactly 32 characters")
	}

	return nil
}

// ApplyConfigDefaults applies default values to database configuration
func ApplyConfigDefaults(config *DatabaseConfig) {
	// Connection pool defaults
	if config.MaxOpenConns == 0 {
		config.MaxOpenConns = 25
	}
	if config.MaxIdleConns == 0 {
		config.MaxIdleConns = 5
	}
	if config.ConnMaxLifetime == 0 {
		config.ConnMaxLifetime = 30 * time.Minute
	}
	if config.ConnMaxIdleTime == 0 {
		config.ConnMaxIdleTime = 30 * time.Minute
	}

	// Health check defaults
	if config.HealthCheckInterval == 0 {
		config.HealthCheckInterval = 30 * time.Second
	}
	if config.ConnectionTimeout == 0 {
		config.ConnectionTimeout = 30 * time.Second
	}
	if config.QueryTimeout == 0 {
		config.QueryTimeout = 30 * time.Second
	}

	// Retry defaults
	if config.MaxRetries == 0 {
		config.MaxRetries = 3
	}
	if config.RetryDelay == 0 {
		config.RetryDelay = time.Second
	}

	// Metrics defaults
	if config.MetricsPrefix == "" {
		config.MetricsPrefix = "finalex_db"
	}
	config.EnableMetrics = true

	// SSL defaults
	if config.SSLMode == "" {
		config.SSLMode = "prefer"
	}
}
