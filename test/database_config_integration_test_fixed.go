// Database configuration integration tests - corrected version
package test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
	"gorm.io/gorm"

	"github.com/Aidin1998/finalex/internal/infrastructure/database"
)

// TestDatabaseConfigIntegrationFixed tests database configuration integration with environment variables
func TestDatabaseConfigIntegrationFixed(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()
	tempDir := t.TempDir()

	t.Run("EnvironmentConfigurationLoad", func(t *testing.T) {
		testEnvironmentConfigurationLoadFixed(t, ctx, logger, tempDir)
	})

	t.Run("ConfigValidation", func(t *testing.T) {
		testConfigValidationFixed(t, ctx, logger, tempDir)
	})

	t.Run("ConfigDefaults", func(t *testing.T) {
		testConfigDefaultsFixed(t, ctx, logger, tempDir)
	})

	t.Run("ProductionConfigurationTest", func(t *testing.T) {
		testProductionConfigurationFixed(t, ctx, logger, tempDir)
	})
}

// testEnvironmentConfigurationLoadFixed tests loading configuration from environment variables
func testEnvironmentConfigurationLoadFixed(t *testing.T, ctx context.Context, logger *zap.Logger, tempDir string) {
	// Set environment variables
	originalEnvVars := map[string]string{
		"DB_MASTER_DSN":            os.Getenv("DB_MASTER_DSN"),
		"DB_MASTER_MAX_OPEN":       os.Getenv("DB_MASTER_MAX_OPEN"),
		"DB_MASTER_MAX_IDLE":       os.Getenv("DB_MASTER_MAX_IDLE"),
		"DB_MASTER_MAX_LIFETIME":   os.Getenv("DB_MASTER_MAX_LIFETIME"),
		"DB_HEALTH_CHECK_INTERVAL": os.Getenv("DB_HEALTH_CHECK_INTERVAL"),
		"DB_CONNECTION_TIMEOUT":    os.Getenv("DB_CONNECTION_TIMEOUT"),
		"DB_QUERY_TIMEOUT":         os.Getenv("DB_QUERY_TIMEOUT"),
		"DB_MAX_RETRIES":           os.Getenv("DB_MAX_RETRIES"),
		"DB_RETRY_DELAY":           os.Getenv("DB_RETRY_DELAY"),
		"DB_ENCRYPTION_KEY":        os.Getenv("DB_ENCRYPTION_KEY"),
		"DB_ENABLE_METRICS":        os.Getenv("DB_ENABLE_METRICS"),
		"DB_METRICS_PREFIX":        os.Getenv("DB_METRICS_PREFIX"),
		"DB_SSL_MODE":              os.Getenv("DB_SSL_MODE"),
	}

	// Cleanup environment after test
	defer func() {
		for key, value := range originalEnvVars {
			if value == "" {
				os.Unsetenv(key)
			} else {
				os.Setenv(key, value)
			}
		}
	}()

	// Set test environment variables
	testDBPath := filepath.Join(tempDir, "test_config.db")
	encryptionKey := "12345678901234567890123456789012" // 32 bytes

	os.Setenv("DB_MASTER_DSN", testDBPath)
	os.Setenv("DB_MASTER_MAX_OPEN", "20")
	os.Setenv("DB_MASTER_MAX_IDLE", "10")
	os.Setenv("DB_MASTER_MAX_LIFETIME", "1h")
	os.Setenv("DB_HEALTH_CHECK_INTERVAL", "60s")
	os.Setenv("DB_CONNECTION_TIMEOUT", "30s")
	os.Setenv("DB_QUERY_TIMEOUT", "60s")
	os.Setenv("DB_MAX_RETRIES", "5")
	os.Setenv("DB_RETRY_DELAY", "2s")
	os.Setenv("DB_ENCRYPTION_KEY", encryptionKey)
	os.Setenv("DB_ENABLE_METRICS", "true")
	os.Setenv("DB_METRICS_PREFIX", "test_db")
	os.Setenv("DB_SSL_MODE", "disable")

	// Load configuration from environment
	config := &database.DatabaseConfig{}
	err := database.LoadConfigFromEnvironment(config)
	require.NoError(t, err)

	// Validate configuration values using actual DatabaseConfig fields
	assert.Equal(t, testDBPath, config.MasterDSN)
	assert.Equal(t, 20, config.MaxOpenConns)
	assert.Equal(t, 10, config.MaxIdleConns)
	assert.Equal(t, time.Hour, config.ConnMaxLifetime)
	assert.Equal(t, 60*time.Second, config.HealthCheckInterval)
	assert.Equal(t, 30*time.Second, config.ConnectionTimeout)
	assert.Equal(t, 60*time.Second, config.QueryTimeout)
	assert.Equal(t, 5, config.MaxRetries)
	assert.Equal(t, 2*time.Second, config.RetryDelay)
	assert.Equal(t, encryptionKey, config.EncryptionKey)
	assert.True(t, config.EnableMetrics)
	assert.Equal(t, "test_db", config.MetricsPrefix)
	assert.Equal(t, "disable", config.SSLMode)

	// Test database manager creation with loaded config
	dbManager, err := database.NewDatabaseManager(config, logger)
	require.NoError(t, err)

	// Verify connection works through the interface
	err = dbManager.Ping(ctx)
	require.NoError(t, err)

	health := dbManager.HealthCheck(ctx)
	assert.True(t, health.Healthy)

	err = dbManager.Close()
	require.NoError(t, err)
}

// testConfigValidationFixed tests configuration validation with correct field names
func testConfigValidationFixed(t *testing.T, ctx context.Context, logger *zap.Logger, tempDir string) {
	// Test missing required field
	invalidConfig := &database.DatabaseConfig{
		MasterDSN: "", // Missing required DSN
	}

	err := database.ValidateConfig(invalidConfig)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "master_dsn")

	// Test invalid encryption key length
	invalidConfig2 := &database.DatabaseConfig{
		MasterDSN:       filepath.Join(tempDir, "test.db"),
		EncryptionKey:   "short_key", // Invalid length
		MaxOpenConns:    10,
		MaxIdleConns:    5,
		ConnMaxLifetime: 30 * time.Minute,
		ConnMaxIdleTime: 5 * time.Minute,
		MaxRetries:      3,
	}

	err = database.ValidateConfig(invalidConfig2)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "encryption_key")

	// Test valid configuration
	validConfig := &database.DatabaseConfig{
		MasterDSN:           filepath.Join(tempDir, "valid_test.db"),
		MaxOpenConns:        10,
		MaxIdleConns:        5,
		ConnMaxLifetime:     30 * time.Minute,
		ConnMaxIdleTime:     5 * time.Minute,
		HealthCheckInterval: 30 * time.Second,
		ConnectionTimeout:   5 * time.Second,
		QueryTimeout:        30 * time.Second,
		MaxRetries:          3,
		RetryDelay:          1 * time.Second,
		EncryptionKey:       "12345678901234567890123456789012", // 32 bytes
		EnableMetrics:       true,
		MetricsPrefix:       "test",
	}

	err = database.ValidateConfig(validConfig)
	assert.NoError(t, err)
}

// testConfigDefaultsFixed tests configuration default values
func testConfigDefaultsFixed(t *testing.T, ctx context.Context, logger *zap.Logger, tempDir string) {
	// Create minimal config
	config := &database.DatabaseConfig{
		MasterDSN:     filepath.Join(tempDir, "defaults_test.db"),
		EncryptionKey: "12345678901234567890123456789012", // Required field
	}

	// Apply defaults
	database.ApplyConfigDefaults(config)

	// Verify default values are applied
	assert.Equal(t, 25, config.MaxOpenConns)
	assert.Equal(t, 5, config.MaxIdleConns)
	assert.Equal(t, 30*time.Minute, config.ConnMaxLifetime)
	assert.Equal(t, 5*time.Minute, config.ConnMaxIdleTime)
	assert.Equal(t, 30*time.Second, config.HealthCheckInterval)
	assert.Equal(t, 10*time.Second, config.ConnectionTimeout)
	assert.Equal(t, 60*time.Second, config.QueryTimeout)
	assert.Equal(t, 3, config.MaxRetries)
	assert.Equal(t, 1*time.Second, config.RetryDelay)
	assert.False(t, config.EnableMetrics) // Default is false
	assert.Equal(t, "database", config.MetricsPrefix)
	assert.Equal(t, "prefer", config.SSLMode)
}

// testProductionConfigurationFixed tests production-ready configuration
func testProductionConfigurationFixed(t *testing.T, ctx context.Context, logger *zap.Logger, tempDir string) {
	// Create production-like configuration
	config := &database.DatabaseConfig{
		MasterDSN:           "file:" + filepath.Join(tempDir, "prod_test.db") + "?cache=shared&mode=rwc",
		SlaveDSNs:           []string{},
		MaxOpenConns:        100,
		MaxIdleConns:        25,
		ConnMaxLifetime:     time.Hour,
		ConnMaxIdleTime:     15 * time.Minute,
		HealthCheckInterval: 30 * time.Second,
		ConnectionTimeout:   10 * time.Second,
		QueryTimeout:        2 * time.Minute,
		MaxRetries:          5,
		RetryDelay:          2 * time.Second,
		EncryptionKey:       "production_key_32_bytes_exactly!", // 32 bytes
		EnableMetrics:       true,
		MetricsPrefix:       "finalex_prod",
		SSLMode:             "require",
	}

	// Validate production config
	err := database.ValidateConfig(config)
	require.NoError(t, err)

	// Create database manager with production config
	dbManager, err := database.NewDatabaseManager(config, logger)
	require.NoError(t, err)

	// Test health check
	health := dbManager.HealthCheck(ctx)
	assert.True(t, health.Healthy)
	assert.True(t, health.Master)

	// Test metrics collection
	metrics := dbManager.GetMetrics()
	assert.NotNil(t, metrics)
	assert.True(t, metrics.ConnectionPoolStats.OpenConnections >= 0)

	// Test encryption functionality
	testData := "sensitive production data"
	encrypted, err := dbManager.Encrypt(testData)
	require.NoError(t, err)
	assert.NotEqual(t, testData, encrypted)

	decrypted, err := dbManager.Decrypt(encrypted)
	require.NoError(t, err)
	assert.Equal(t, testData, decrypted)

	// Test transaction handling
	db := dbManager.DB()
	require.NotNil(t, db)

	// Create test table for production-like operations
	err = db.Exec(`
		CREATE TABLE prod_test (
			id INTEGER PRIMARY KEY,
			data TEXT NOT NULL,
			encrypted_data TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`).Error
	require.NoError(t, err)

	// Test transaction with encryption
	err = dbManager.Transaction(ctx, func(tx *gorm.DB) error {
		encryptedValue, err := dbManager.Encrypt("sensitive customer data")
		if err != nil {
			return err
		}

		return tx.Exec(
			"INSERT INTO prod_test (data, encrypted_data) VALUES (?, ?)",
			"public data", encryptedValue,
		).Error
	})
	require.NoError(t, err)

	// Verify data was inserted
	var count int64
	err = db.Raw("SELECT COUNT(*) FROM prod_test").Row().Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)

	// Verify we can decrypt the stored data
	var encryptedData string
	err = db.Raw("SELECT encrypted_data FROM prod_test LIMIT 1").Row().Scan(&encryptedData)
	require.NoError(t, err)

	decryptedData, err := dbManager.Decrypt(encryptedData)
	require.NoError(t, err)
	assert.Equal(t, "sensitive customer data", decryptedData)

	// Test connection cleanup
	err = dbManager.Close()
	require.NoError(t, err)
}
