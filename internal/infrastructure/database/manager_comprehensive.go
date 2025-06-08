// Package database provides comprehensive database management with security, scalability, and ACID guarantees
package database

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// DatabaseInterface defines the main database interface
type DatabaseInterface interface {
	// Connection management
	DB() *gorm.DB
	Master() *gorm.DB
	Slave() *gorm.DB
	Close() error

	// Health checks
	Ping(ctx context.Context) error
	HealthCheck(ctx context.Context) HealthStatus

	// Transaction management
	Transaction(ctx context.Context, fn func(*gorm.DB) error) error
	WithTransaction(ctx context.Context) (*gorm.DB, func(), error)

	// Migration support
	AutoMigrate(models ...interface{}) error
	Migrate(version string) error

	// Sharding support
	ShardBy(key string) DatabaseInterface
	GetShard(key string) DatabaseInterface

	// Encryption support
	Encrypt(plaintext string) (string, error)
	Decrypt(ciphertext string) (string, error)

	// Monitoring
	GetMetrics() DatabaseMetrics
}

// HealthStatus represents database health status
type HealthStatus struct {
	Healthy     bool          `json:"healthy"`
	Master      bool          `json:"master"`
	Slaves      []bool        `json:"slaves"`
	Latency     time.Duration `json:"latency"`
	Connections int           `json:"connections"`
	LastError   string        `json:"last_error,omitempty"`
	CheckedAt   time.Time     `json:"checked_at"`
}

// DatabaseMetrics represents database metrics
type DatabaseMetrics struct {
	QueryCount          int64         `json:"query_count"`
	QueryDuration       time.Duration `json:"query_duration"`
	ErrorCount          int64         `json:"error_count"`
	ConnectionPoolStats PoolStats     `json:"connection_pool"`
}

// PoolStats represents connection pool statistics
type PoolStats struct {
	OpenConnections int           `json:"open_connections"`
	InUse           int           `json:"in_use"`
	Idle            int           `json:"idle"`
	WaitCount       int           `json:"wait_count"`
	WaitDuration    time.Duration `json:"wait_duration"`
}

// DatabaseManager implements comprehensive database management
type DatabaseManager struct {
	mu     sync.RWMutex
	config *DatabaseConfig
	logger *zap.Logger

	// Connections
	master *gorm.DB
	slaves []*gorm.DB
	shards map[string]*DatabaseManager

	// Encryption
	cipher cipher.AEAD

	// Metrics
	metrics *DatabaseMetricsCollector

	// Health monitoring
	healthStatus HealthStatus
	healthTicker *time.Ticker
	healthCtx    context.Context
	healthCancel context.CancelFunc

	// Connection management
	connectionRetry int
	maxRetries      int
}

// DatabaseConfig holds comprehensive database configuration
type DatabaseConfig struct {
	// Primary connection
	MasterDSN string `json:"master_dsn" validate:"required"`

	// Read replicas
	SlaveDSNs []string `json:"slave_dsns"`

	// Connection pooling
	MaxOpenConns    int           `json:"max_open_conns" validate:"min=1"`
	MaxIdleConns    int           `json:"max_idle_conns" validate:"min=1"`
	ConnMaxLifetime time.Duration `json:"conn_max_lifetime" validate:"required"`
	ConnMaxIdleTime time.Duration `json:"conn_max_idle_time" validate:"required"`

	// Health check
	HealthCheckInterval time.Duration `json:"health_check_interval"`
	ConnectionTimeout   time.Duration `json:"connection_timeout"`
	QueryTimeout        time.Duration `json:"query_timeout"`

	// Retry configuration
	MaxRetries int           `json:"max_retries" validate:"min=1"`
	RetryDelay time.Duration `json:"retry_delay"`

	// Encryption
	EncryptionKey string `json:"encryption_key" validate:"required,len=32"`

	// Sharding
	Shards []ShardConfig `json:"shards"`

	// Security
	SSLMode     string `json:"ssl_mode"`
	SSLCert     string `json:"ssl_cert"`
	SSLKey      string `json:"ssl_key"`
	SSLRootCert string `json:"ssl_root_cert"`

	// Monitoring
	EnableMetrics bool   `json:"enable_metrics"`
	MetricsPrefix string `json:"metrics_prefix"`
}

// ShardConfig represents a database shard configuration
type ShardConfig struct {
	Name           string   `json:"name" validate:"required"`
	MasterDSN      string   `json:"master_dsn" validate:"required"`
	SlaveDSNs      []string `json:"slave_dsns"`
	HashRangeStart uint32   `json:"hash_range_start"`
	HashRangeEnd   uint32   `json:"hash_range_end"`
	Weight         int      `json:"weight" validate:"min=1"`
}

// DatabaseMetricsCollector collects database metrics for Prometheus
type DatabaseMetricsCollector struct {
	// Query metrics
	QueryTotal    prometheus.CounterVec
	QueryDuration prometheus.HistogramVec
	QueryErrors   prometheus.CounterVec

	// Connection metrics
	ConnectionsOpen  prometheus.GaugeVec
	ConnectionsIdle  prometheus.GaugeVec
	ConnectionsInUse prometheus.GaugeVec

	// Health metrics
	HealthStatus  prometheus.GaugeVec
	HealthLatency prometheus.GaugeVec
}

// NewDatabaseManager creates a new database manager with comprehensive features
func NewDatabaseManager(config *DatabaseConfig, logger *zap.Logger) (*DatabaseManager, error) {
	if config == nil {
		return nil, fmt.Errorf("database config is required")
	}

	if logger == nil {
		logger = zap.NewNop()
	}

	// Initialize encryption
	aesGCM, err := initializeEncryption(config.EncryptionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize encryption: %w", err)
	}

	// Initialize metrics
	metrics := initializeMetrics(config.MetricsPrefix)

	// Create health check context
	healthCtx, healthCancel := context.WithCancel(context.Background())

	dm := &DatabaseManager{
		config:       config,
		logger:       logger.Named("database"),
		cipher:       aesGCM,
		metrics:      metrics,
		healthCtx:    healthCtx,
		healthCancel: healthCancel,
		maxRetries:   config.MaxRetries,
		shards:       make(map[string]*DatabaseManager),
	}

	// Initialize master connection
	if err := dm.initializeMaster(); err != nil {
		return nil, fmt.Errorf("failed to initialize master connection: %w", err)
	}

	// Initialize slave connections
	if err := dm.initializeSlaves(); err != nil {
		logger.Warn("failed to initialize some slave connections", zap.Error(err))
	}

	// Initialize shards if configured
	if err := dm.initializeShards(); err != nil {
		logger.Warn("failed to initialize some shards", zap.Error(err))
	}

	// Start health monitoring
	dm.startHealthMonitoring()

	return dm, nil
}

// initializeEncryption initializes AES-GCM encryption
func initializeEncryption(key string) (cipher.AEAD, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("encryption key must be exactly 32 bytes")
	}

	block, err := aes.NewCipher([]byte(key))
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	return aesGCM, nil
}

// initializeMetrics initializes Prometheus metrics
func initializeMetrics(prefix string) *DatabaseMetricsCollector {
	if prefix == "" {
		prefix = "database"
	}

	return &DatabaseMetricsCollector{
		QueryTotal: *promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: fmt.Sprintf("%s_queries_total", prefix),
				Help: "Total number of database queries",
			},
			[]string{"operation", "table", "shard"},
		),
		QueryDuration: *promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    fmt.Sprintf("%s_query_duration_seconds", prefix),
				Help:    "Database query duration in seconds",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"operation", "table", "shard"},
		),
		QueryErrors: *promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: fmt.Sprintf("%s_query_errors_total", prefix),
				Help: "Total number of database query errors",
			},
			[]string{"operation", "table", "shard", "error_type"},
		),
		ConnectionsOpen: *promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: fmt.Sprintf("%s_connections_open", prefix),
				Help: "Number of open database connections",
			},
			[]string{"database", "shard"},
		),
		ConnectionsIdle: *promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: fmt.Sprintf("%s_connections_idle", prefix),
				Help: "Number of idle database connections",
			},
			[]string{"database", "shard"},
		),
		ConnectionsInUse: *promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: fmt.Sprintf("%s_connections_in_use", prefix),
				Help: "Number of database connections in use",
			},
			[]string{"database", "shard"},
		),
		HealthStatus: *promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: fmt.Sprintf("%s_health_status", prefix),
				Help: "Database health status (1=healthy, 0=unhealthy)",
			},
			[]string{"database", "role", "shard"},
		),
		HealthLatency: *promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: fmt.Sprintf("%s_health_latency_seconds", prefix),
				Help: "Database health check latency in seconds",
			},
			[]string{"database", "role", "shard"},
		),
	}
}

// DB returns the primary database connection (alias for Master)
func (dm *DatabaseManager) DB() *gorm.DB {
	return dm.Master()
}

// Master returns the master database connection
func (dm *DatabaseManager) Master() *gorm.DB {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return dm.master
}

// Slave returns a slave database connection (load balanced)
func (dm *DatabaseManager) Slave() *gorm.DB {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	if len(dm.slaves) == 0 {
		return dm.master
	}

	// Simple round-robin selection
	// In production, implement more sophisticated load balancing
	idx := time.Now().UnixNano() % int64(len(dm.slaves))
	return dm.slaves[idx]
}

// Close closes all database connections
func (dm *DatabaseManager) Close() error {
	dm.healthCancel()

	dm.mu.Lock()
	defer dm.mu.Unlock()

	var errors []error

	// Close master
	if dm.master != nil {
		if sqlDB, err := dm.master.DB(); err == nil {
			if err := sqlDB.Close(); err != nil {
				errors = append(errors, fmt.Errorf("failed to close master: %w", err))
			}
		}
	}

	// Close slaves
	for i, slave := range dm.slaves {
		if sqlDB, err := slave.DB(); err == nil {
			if err := sqlDB.Close(); err != nil {
				errors = append(errors, fmt.Errorf("failed to close slave %d: %w", i, err))
			}
		}
	}

	// Close shards
	for name, shard := range dm.shards {
		if err := shard.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close shard %s: %w", name, err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("errors closing database connections: %v", errors)
	}

	return nil
}

// Ping checks database connectivity
func (dm *DatabaseManager) Ping(ctx context.Context) error {
	master := dm.Master()
	if master == nil {
		return fmt.Errorf("master database not available")
	}

	sqlDB, err := master.DB()
	if err != nil {
		return fmt.Errorf("failed to get SQL DB: %w", err)
	}

	if err := sqlDB.PingContext(ctx); err != nil {
		return fmt.Errorf("master ping failed: %w", err)
	}

	return nil
}

// HealthCheck performs comprehensive health check
func (dm *DatabaseManager) HealthCheck(ctx context.Context) HealthStatus {
	start := time.Now()
	status := HealthStatus{
		CheckedAt: start,
		Healthy:   true,
	}

	// Check master
	if err := dm.Ping(ctx); err != nil {
		status.Healthy = false
		status.Master = false
		status.LastError = err.Error()
	} else {
		status.Master = true
	}

	// Check slaves
	status.Slaves = make([]bool, len(dm.slaves))
	for i, slave := range dm.slaves {
		if sqlDB, err := slave.DB(); err == nil {
			if err := sqlDB.PingContext(ctx); err == nil {
				status.Slaves[i] = true
			}
		}
	}

	// Get connection stats
	if master := dm.Master(); master != nil {
		if sqlDB, err := master.DB(); err == nil {
			if stats := sqlDB.Stats(); true {
				status.Connections = stats.OpenConnections
			}
		}
	}

	status.Latency = time.Since(start)

	// Update metrics
	healthValue := float64(0)
	if status.Healthy {
		healthValue = 1
	}
	dm.metrics.HealthStatus.WithLabelValues("master", "master", "").Set(healthValue)
	dm.metrics.HealthLatency.WithLabelValues("master", "master", "").Set(status.Latency.Seconds())

	dm.mu.Lock()
	dm.healthStatus = status
	dm.mu.Unlock()

	return status
}

// Encrypt encrypts sensitive data using AES-GCM
func (dm *DatabaseManager) Encrypt(plaintext string) (string, error) {
	if dm.cipher == nil {
		return "", fmt.Errorf("encryption not configured")
	}

	nonce := make([]byte, dm.cipher.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := dm.cipher.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts sensitive data using AES-GCM
func (dm *DatabaseManager) Decrypt(ciphertext string) (string, error) {
	if dm.cipher == nil {
		return "", fmt.Errorf("encryption not configured")
	}

	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("failed to decode base64: %w", err)
	}

	nonceSize := dm.cipher.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, cipherData := data[:nonceSize], data[nonceSize:]
	plaintext, err := dm.cipher.Open(nil, nonce, cipherData, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt: %w", err)
	}

	return string(plaintext), nil
}

// Transaction executes a function within a database transaction
func (dm *DatabaseManager) Transaction(ctx context.Context, fn func(*gorm.DB) error) error {
	return dm.Master().WithContext(ctx).Transaction(fn)
}

// WithTransaction returns a transaction and commit/rollback function
func (dm *DatabaseManager) WithTransaction(ctx context.Context) (*gorm.DB, func(), error) {
	tx := dm.Master().WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, nil, fmt.Errorf("failed to begin transaction: %w", tx.Error)
	}

	cleanup := func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
		if tx.Error != nil {
			tx.Rollback()
		} else {
			tx.Commit()
		}
	}

	return tx, cleanup, nil
}

// GetMetrics returns current database metrics
func (dm *DatabaseManager) GetMetrics() DatabaseMetrics {
	metrics := DatabaseMetrics{}

	if master := dm.Master(); master != nil {
		if sqlDB, err := master.DB(); err == nil {
			stats := sqlDB.Stats()
			metrics.ConnectionPoolStats = PoolStats{
				OpenConnections: stats.OpenConnections,
				InUse:           stats.InUse,
				Idle:            stats.Idle,
				WaitCount:       int(stats.WaitCount),
				WaitDuration:    stats.WaitDuration,
			}
		}
	}

	return metrics
}
