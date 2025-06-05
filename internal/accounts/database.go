// Database configuration and connection pool management for ultra-high concurrency
package accounts

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/go-redsync/redsync/v4"
	"github.com/go-redsync/redsync/v4/redis/goredis/v8"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/plugin/prometheus"
)

// DatabaseConfig represents database configuration for ultra-high concurrency
type DatabaseConfig struct {
	// PostgreSQL configuration
	PostgreSQL PostgreSQLConfig `json:"postgresql"`
	
	// Redis configuration
	Redis RedisConfig `json:"redis"`
	
	// Connection pool configuration
	ConnectionPool ConnectionPoolConfig `json:"connection_pool"`
	
	// Partitioning configuration
	Partitioning PartitioningConfig `json:"partitioning"`
	
	// Monitoring configuration
	Monitoring MonitoringConfig `json:"monitoring"`
}

// PostgreSQLConfig represents PostgreSQL-specific configuration
type PostgreSQLConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Database string `json:"database"`
	Username string `json:"username"`
	Password string `json:"password"`
	SSLMode  string `json:"ssl_mode"`
	
	// Read replica configuration
	ReadReplicas []PostgreSQLInstance `json:"read_replicas"`
	
	// Write configuration
	MaxConnections     int           `json:"max_connections"`
	MaxIdleConnections int           `json:"max_idle_connections"`
	ConnMaxLifetime    time.Duration `json:"conn_max_lifetime"`
	ConnMaxIdleTime    time.Duration `json:"conn_max_idle_time"`
	
	// Performance tuning
	PreparedStatements bool `json:"prepared_statements"`
	PreferSimpleProtocol bool `json:"prefer_simple_protocol"`
}

// PostgreSQLInstance represents a PostgreSQL instance
type PostgreSQLInstance struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Weight   int    `json:"weight"` // Load balancing weight
	Priority int    `json:"priority"` // Failover priority
}

// RedisConfig represents Redis configuration for caching and distributed locking
type RedisConfig struct {
	// Primary Redis cluster
	Cluster RedisClusterConfig `json:"cluster"`
	
	// Sentinel configuration for high availability
	Sentinel RedisSentinelConfig `json:"sentinel"`
	
	// Connection pool
	PoolSize           int           `json:"pool_size"`
	MinIdleConnections int           `json:"min_idle_connections"`
	MaxRetries         int           `json:"max_retries"`
	DialTimeout        time.Duration `json:"dial_timeout"`
	ReadTimeout        time.Duration `json:"read_timeout"`
	WriteTimeout       time.Duration `json:"write_timeout"`
	PoolTimeout        time.Duration `json:"pool_timeout"`
	
	// Cache configuration
	DefaultTTL time.Duration `json:"default_ttl"`
	HotTTL     time.Duration `json:"hot_ttl"`
	WarmTTL    time.Duration `json:"warm_ttl"`
}

// RedisClusterConfig represents Redis cluster configuration
type RedisClusterConfig struct {
	Nodes    []string `json:"nodes"`
	Password string   `json:"password"`
	DB       int      `json:"db"`
}

// RedisSentinelConfig represents Redis Sentinel configuration
type RedisSentinelConfig struct {
	MasterName string   `json:"master_name"`
	Addresses  []string `json:"addresses"`
	Password   string   `json:"password"`
	DB         int      `json:"db"`
}

// ConnectionPoolConfig represents connection pool configuration
type ConnectionPoolConfig struct {
	// Per-service connection limits
	MaxConnectionsPerService int `json:"max_connections_per_service"`
	
	// Health check configuration
	HealthCheckInterval time.Duration `json:"health_check_interval"`
	HealthCheckTimeout  time.Duration `json:"health_check_timeout"`
	
	// Failover configuration
	FailoverEnabled         bool          `json:"failover_enabled"`
	FailoverTimeout         time.Duration `json:"failover_timeout"`
	MaxFailoverRetries      int           `json:"max_failover_retries"`
	FailoverBackoffDuration time.Duration `json:"failover_backoff_duration"`
}

// PartitioningConfig represents table partitioning configuration
type PartitioningConfig struct {
	// Account partitioning
	AccountPartitions int    `json:"account_partitions"`
	PartitionBy       string `json:"partition_by"` // user_id, currency, date
	
	// Ledger partitioning
	LedgerPartitionInterval string `json:"ledger_partition_interval"` // daily, weekly, monthly
	LedgerRetentionPeriod   string `json:"ledger_retention_period"`   // how long to keep old partitions
	
	// Auto-partition management
	AutoCreatePartitions bool `json:"auto_create_partitions"`
	PartitionMaintenanceSchedule string `json:"partition_maintenance_schedule"`
}

// MonitoringConfig represents monitoring and metrics configuration
type MonitoringConfig struct {
	PrometheusEnabled bool   `json:"prometheus_enabled"`
	MetricsNamespace  string `json:"metrics_namespace"`
	
	// Logging configuration
	SlowQueryThreshold   time.Duration `json:"slow_query_threshold"`
	LogAllQueries        bool          `json:"log_all_queries"`
	LogSlowQueriesOnly   bool          `json:"log_slow_queries_only"`
	
	// Alerting thresholds
	HighConnectionUsage    float64 `json:"high_connection_usage"`
	HighCacheMissRate      float64 `json:"high_cache_miss_rate"`
	HighTransactionLatency time.Duration `json:"high_transaction_latency"`
}

// DatabaseManager manages database connections and provides ultra-high concurrency support
type DatabaseManager struct {
	config *DatabaseConfig
	logger *zap.Logger
	
	// Database connections
	writeDB  *gorm.DB
	readDBs  []*gorm.DB
	pgxPool  *pgxpool.Pool
	
	// Redis connections
	redisClient   redis.UniversalClient
	redisLock     *redsync.Redsync
	
	// Metrics
	dbMetrics    *DatabaseMetrics
	cacheMetrics *CacheMetrics
	
	// Connection management
	connTracker *ConnectionTracker
	healthCheck *HealthChecker
}

// ConnectionTracker tracks database connections and their usage
type ConnectionTracker struct {
	activeConnections prometheus.Gauge
	totalConnections  prometheus.Counter
	connectionErrors  prometheus.Counter
	queryDuration     prometheus.HistogramVec
	
	// Connection pools
	writePool *ConnectionPool
	readPools []*ConnectionPool
}

// ConnectionPool represents a managed connection pool
type ConnectionPool struct {
	Name            string
	MaxConnections  int
	ActiveConns     int
	IdleConns       int
	WaitingQueries  int
	TotalRequests   int64
	FailedRequests  int64
	AvgResponseTime time.Duration
	LastHealthCheck time.Time
	Healthy         bool
}

// HealthChecker provides health checking for database connections
type HealthChecker struct {
	config   *DatabaseConfig
	logger   *zap.Logger
	interval time.Duration
	timeout  time.Duration
	stopChan chan bool
}

// DatabaseMetrics represents database operation metrics
type DatabaseMetrics struct {
	QueryCount       prometheus.CounterVec
	QueryDuration    prometheus.HistogramVec
	ConnectionCount  prometheus.GaugeVec
	TransactionCount prometheus.CounterVec
	ErrorCount       prometheus.CounterVec
	LockWaitTime     prometheus.HistogramVec
}

// NewDatabaseManager creates a new database manager with ultra-high concurrency support
func NewDatabaseManager(config *DatabaseConfig, logger *zap.Logger) (*DatabaseManager, error) {
	dm := &DatabaseManager{
		config: config,
		logger: logger,
	}
	
	// Initialize metrics
	if err := dm.initializeMetrics(); err != nil {
		return nil, fmt.Errorf("failed to initialize metrics: %w", err)
	}
	
	// Initialize PostgreSQL connections
	if err := dm.initializePostgreSQL(); err != nil {
		return nil, fmt.Errorf("failed to initialize PostgreSQL: %w", err)
	}
	
	// Initialize Redis connections
	if err := dm.initializeRedis(); err != nil {
		return nil, fmt.Errorf("failed to initialize Redis: %w", err)
	}
	
	// Initialize connection tracker
	if err := dm.initializeConnectionTracker(); err != nil {
		return nil, fmt.Errorf("failed to initialize connection tracker: %w", err)
	}
	
	// Start health checker
	if err := dm.startHealthChecker(); err != nil {
		return nil, fmt.Errorf("failed to start health checker: %w", err)
	}
	
	return dm, nil
}

// initializeMetrics initializes Prometheus metrics
func (dm *DatabaseManager) initializeMetrics() error {
	namespace := dm.config.Monitoring.MetricsNamespace
	if namespace == "" {
		namespace = "accounts_db"
	}
	
	dm.dbMetrics = &DatabaseMetrics{
		QueryCount: *prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "queries_total",
				Help:      "Total number of database queries",
			},
			[]string{"operation", "table", "status"},
		),
		QueryDuration: *prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "query_duration_seconds",
				Help:      "Database query duration in seconds",
				Buckets:   prometheus.ExponentialBuckets(0.001, 2, 15),
			},
			[]string{"operation", "table"},
		),
		ConnectionCount: *prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "connections_active",
				Help:      "Number of active database connections",
			},
			[]string{"pool", "type"},
		),
		TransactionCount: *prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "transactions_total",
				Help:      "Total number of database transactions",
			},
			[]string{"status"},
		),
		ErrorCount: *prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "errors_total",
				Help:      "Total number of database errors",
			},
			[]string{"operation", "error_type"},
		),
		LockWaitTime: *prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "lock_wait_duration_seconds",
				Help:      "Time spent waiting for locks",
				Buckets:   prometheus.ExponentialBuckets(0.001, 2, 15),
			},
			[]string{"lock_type"},
		),
	}
	
	// Register metrics
	if dm.config.Monitoring.PrometheusEnabled {
		prometheus.MustRegister(
			dm.dbMetrics.QueryCount,
			dm.dbMetrics.QueryDuration,
			dm.dbMetrics.ConnectionCount,
			dm.dbMetrics.TransactionCount,
			dm.dbMetrics.ErrorCount,
			dm.dbMetrics.LockWaitTime,
		)
	}
	
	return nil
}

// initializePostgreSQL initializes PostgreSQL connections with optimized settings
func (dm *DatabaseManager) initializePostgreSQL() error {
	// Build DSN for write database
	writeDSN := dm.buildPostgreSQLDSN(dm.config.PostgreSQL)
	
	// Configure GORM with optimized settings
	gormConfig := &gorm.Config{
		Logger: logger.New(
			log.New(os.Stdout, "\r\n", log.LstdFlags),
			logger.Config{
				SlowThreshold:             dm.config.Monitoring.SlowQueryThreshold,
				LogLevel:                  logger.Silent,
				IgnoreRecordNotFoundError: true,
				Colorful:                  false,
			},
		),
		PrepareStmt:              dm.config.PostgreSQL.PreparedStatements,
		DisableForeignKeyConstraintWhenMigrating: true,
	}
	
	// Open write database connection
	writeDB, err := gorm.Open(postgres.Open(writeDSN), gormConfig)
	if err != nil {
		return fmt.Errorf("failed to connect to write database: %w", err)
	}
	
	// Configure connection pool for write database
	writeSqlDB, err := writeDB.DB()
	if err != nil {
		return fmt.Errorf("failed to get write database SQL.DB: %w", err)
	}
	
	writeSqlDB.SetMaxOpenConns(dm.config.PostgreSQL.MaxConnections)
	writeSqlDB.SetMaxIdleConns(dm.config.PostgreSQL.MaxIdleConnections)
	writeSqlDB.SetConnMaxLifetime(dm.config.PostgreSQL.ConnMaxLifetime)
	writeSqlDB.SetConnMaxIdleTime(dm.config.PostgreSQL.ConnMaxIdleTime)
	
	dm.writeDB = writeDB
	
	// Add Prometheus plugin for monitoring
	if dm.config.Monitoring.PrometheusEnabled {
		if err := writeDB.Use(prometheus.New(prometheus.Config{
			DBName:          dm.config.PostgreSQL.Database,
			RefreshInterval: 15,
			MetricsCollector: []prometheus.MetricsCollector{
				&prometheus.MySQL{
					VariableNames: []string{"Threads_running"},
				},
			},
		})); err != nil {
			dm.logger.Warn("Failed to add Prometheus plugin to write DB", zap.Error(err))
		}
	}
	
	// Initialize read replica connections
	dm.readDBs = make([]*gorm.DB, len(dm.config.PostgreSQL.ReadReplicas))
	for i, replica := range dm.config.PostgreSQL.ReadReplicas {
		replicaDSN := dm.buildReplicaDSN(replica)
		readDB, err := gorm.Open(postgres.Open(replicaDSN), gormConfig)
		if err != nil {
			dm.logger.Warn("Failed to connect to read replica",
				zap.Int("replica_index", i),
				zap.String("host", replica.Host),
				zap.Error(err))
			continue
		}
		
		// Configure read replica connection pool
		readSqlDB, err := readDB.DB()
		if err != nil {
			dm.logger.Warn("Failed to get read replica SQL.DB", zap.Error(err))
			continue
		}
		
		readSqlDB.SetMaxOpenConns(dm.config.PostgreSQL.MaxConnections / 2) // Smaller pool for read replicas
		readSqlDB.SetMaxIdleConns(dm.config.PostgreSQL.MaxIdleConnections / 2)
		readSqlDB.SetConnMaxLifetime(dm.config.PostgreSQL.ConnMaxLifetime)
		readSqlDB.SetConnMaxIdleTime(dm.config.PostgreSQL.ConnMaxIdleTime)
		
		dm.readDBs[i] = readDB
	}
	
	// Initialize pgx connection pool for high-performance operations
	pgxConfig, err := pgxpool.ParseConfig(writeDSN)
	if err != nil {
		return fmt.Errorf("failed to parse pgx config: %w", err)
	}
	
	pgxConfig.MaxConns = int32(dm.config.PostgreSQL.MaxConnections)
	pgxConfig.MinConns = int32(dm.config.PostgreSQL.MaxIdleConnections)
	pgxConfig.MaxConnLifetime = dm.config.PostgreSQL.ConnMaxLifetime
	pgxConfig.MaxConnIdleTime = dm.config.PostgreSQL.ConnMaxIdleTime
	
	dm.pgxPool, err = pgxpool.NewWithConfig(context.Background(), pgxConfig)
	if err != nil {
		return fmt.Errorf("failed to create pgx pool: %w", err)
	}
	
	return nil
}

// initializeRedis initializes Redis connections for caching and distributed locking
func (dm *DatabaseManager) initializeRedis() error {
	var redisClient redis.UniversalClient
	
	// Configure Redis based on setup type
	if len(dm.config.Redis.Cluster.Nodes) > 0 {
		// Redis Cluster setup
		redisClient = redis.NewClusterClient(&redis.ClusterOptions{
			Addrs:        dm.config.Redis.Cluster.Nodes,
			Password:     dm.config.Redis.Cluster.Password,
			PoolSize:     dm.config.Redis.PoolSize,
			MinIdleConns: dm.config.Redis.MinIdleConnections,
			MaxRetries:   dm.config.Redis.MaxRetries,
			DialTimeout:  dm.config.Redis.DialTimeout,
			ReadTimeout:  dm.config.Redis.ReadTimeout,
			WriteTimeout: dm.config.Redis.WriteTimeout,
			PoolTimeout:  dm.config.Redis.PoolTimeout,
		})
	} else if len(dm.config.Redis.Sentinel.Addresses) > 0 {
		// Redis Sentinel setup
		redisClient = redis.NewFailoverClient(&redis.FailoverOptions{
			MasterName:       dm.config.Redis.Sentinel.MasterName,
			SentinelAddrs:    dm.config.Redis.Sentinel.Addresses,
			Password:         dm.config.Redis.Sentinel.Password,
			DB:               dm.config.Redis.Sentinel.DB,
			PoolSize:         dm.config.Redis.PoolSize,
			MinIdleConns:     dm.config.Redis.MinIdleConnections,
			MaxRetries:       dm.config.Redis.MaxRetries,
			DialTimeout:      dm.config.Redis.DialTimeout,
			ReadTimeout:      dm.config.Redis.ReadTimeout,
			WriteTimeout:     dm.config.Redis.WriteTimeout,
			PoolTimeout:      dm.config.Redis.PoolTimeout,
		})
	} else {
		// Single Redis instance (for development)
		redisClient = redis.NewClient(&redis.Options{
			Addr:         dm.config.Redis.Cluster.Nodes[0],
			Password:     dm.config.Redis.Cluster.Password,
			DB:           dm.config.Redis.Cluster.DB,
			PoolSize:     dm.config.Redis.PoolSize,
			MinIdleConns: dm.config.Redis.MinIdleConnections,
			MaxRetries:   dm.config.Redis.MaxRetries,
			DialTimeout:  dm.config.Redis.DialTimeout,
			ReadTimeout:  dm.config.Redis.ReadTimeout,
			WriteTimeout: dm.config.Redis.WriteTimeout,
			PoolTimeout:  dm.config.Redis.PoolTimeout,
		})
	}
	
	// Test Redis connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	if err := redisClient.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("failed to connect to Redis: %w", err)
	}
	
	dm.redisClient = redisClient
	
	// Initialize distributed locking with Redsync
	pool := goredis.NewPool(redisClient)
	dm.redisLock = redsync.New(pool)
	
	return nil
}

// initializeConnectionTracker initializes connection tracking and monitoring
func (dm *DatabaseManager) initializeConnectionTracker() error {
	dm.connTracker = &ConnectionTracker{
		activeConnections: dm.dbMetrics.ConnectionCount.WithLabelValues("total", "active"),
		totalConnections:  dm.dbMetrics.QueryCount.WithLabelValues("", "", "total"),
		connectionErrors:  dm.dbMetrics.ErrorCount.WithLabelValues("connection", "error"),
		queryDuration:     dm.dbMetrics.QueryDuration,
	}
	
	// Initialize connection pools tracking
	dm.connTracker.writePool = &ConnectionPool{
		Name:           "write_pool",
		MaxConnections: dm.config.PostgreSQL.MaxConnections,
		Healthy:        true,
	}
	
	dm.connTracker.readPools = make([]*ConnectionPool, len(dm.readDBs))
	for i := range dm.readDBs {
		dm.connTracker.readPools[i] = &ConnectionPool{
			Name:           fmt.Sprintf("read_pool_%d", i),
			MaxConnections: dm.config.PostgreSQL.MaxConnections / 2,
			Healthy:        true,
		}
	}
	
	return nil
}

// startHealthChecker starts the health checking goroutine
func (dm *DatabaseManager) startHealthChecker() error {
	dm.healthCheck = &HealthChecker{
		config:   dm.config,
		logger:   dm.logger,
		interval: dm.config.ConnectionPool.HealthCheckInterval,
		timeout:  dm.config.ConnectionPool.HealthCheckTimeout,
		stopChan: make(chan bool),
	}
	
	go dm.healthCheck.start(dm)
	
	return nil
}

// buildPostgreSQLDSN builds PostgreSQL DSN from configuration
func (dm *DatabaseManager) buildPostgreSQLDSN(config PostgreSQLConfig) string {
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		config.Host, config.Port, config.Username, config.Password, config.Database, config.SSLMode)
}

// buildReplicaDSN builds DSN for read replica
func (dm *DatabaseManager) buildReplicaDSN(replica PostgreSQLInstance) string {
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		replica.Host, replica.Port, dm.config.PostgreSQL.Username, 
		dm.config.PostgreSQL.Password, dm.config.PostgreSQL.Database, dm.config.PostgreSQL.SSLMode)
}

// GetWriteDB returns the write database connection
func (dm *DatabaseManager) GetWriteDB() *gorm.DB {
	return dm.writeDB
}

// GetReadDB returns a read database connection with load balancing
func (dm *DatabaseManager) GetReadDB() *gorm.DB {
	if len(dm.readDBs) == 0 {
		return dm.writeDB // Fallback to write DB
	}
	
	// Simple round-robin load balancing
	// In production, implement weighted round-robin based on replica health
	idx := time.Now().UnixNano() % int64(len(dm.readDBs))
	return dm.readDBs[idx]
}

// GetPgxPool returns the pgx connection pool for high-performance operations
func (dm *DatabaseManager) GetPgxPool() *pgxpool.Pool {
	return dm.pgxPool
}

// GetRedisClient returns the Redis client
func (dm *DatabaseManager) GetRedisClient() redis.UniversalClient {
	return dm.redisClient
}

// GetRedisLock returns the distributed lock manager
func (dm *DatabaseManager) GetRedisLock() *redsync.Redsync {
	return dm.redisLock
}

// Close gracefully closes all database connections
func (dm *DatabaseManager) Close() error {
	var errors []error
	
	// Stop health checker
	if dm.healthCheck != nil {
		close(dm.healthCheck.stopChan)
	}
	
	// Close write database
	if dm.writeDB != nil {
		if sqlDB, err := dm.writeDB.DB(); err == nil {
			if err := sqlDB.Close(); err != nil {
				errors = append(errors, fmt.Errorf("failed to close write DB: %w", err))
			}
		}
	}
	
	// Close read databases
	for i, readDB := range dm.readDBs {
		if readDB != nil {
			if sqlDB, err := readDB.DB(); err == nil {
				if err := sqlDB.Close(); err != nil {
					errors = append(errors, fmt.Errorf("failed to close read DB %d: %w", i, err))
				}
			}
		}
	}
	
	// Close pgx pool
	if dm.pgxPool != nil {
		dm.pgxPool.Close()
	}
	
	// Close Redis client
	if dm.redisClient != nil {
		if err := dm.redisClient.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close Redis client: %w", err))
		}
	}
	
	if len(errors) > 0 {
		return fmt.Errorf("errors during close: %v", errors)
	}
	
	return nil
}

// Health checker implementation
func (hc *HealthChecker) start(dm *DatabaseManager) {
	ticker := time.NewTicker(hc.interval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			hc.checkHealth(dm)
		case <-hc.stopChan:
			return
		}
	}
}

func (hc *HealthChecker) checkHealth(dm *DatabaseManager) {
	ctx, cancel := context.WithTimeout(context.Background(), hc.timeout)
	defer cancel()
	
	// Check write database health
	if err := dm.writeDB.WithContext(ctx).Raw("SELECT 1").Error; err != nil {
		hc.logger.Error("Write database health check failed", zap.Error(err))
		dm.connTracker.connectionErrors.Inc()
	}
	
	// Check read databases health
	for i, readDB := range dm.readDBs {
		if err := readDB.WithContext(ctx).Raw("SELECT 1").Error; err != nil {
			hc.logger.Error("Read database health check failed",
				zap.Int("replica_index", i),
				zap.Error(err))
			dm.connTracker.connectionErrors.Inc()
			dm.connTracker.readPools[i].Healthy = false
		} else {
			dm.connTracker.readPools[i].Healthy = true
		}
	}
	
	// Check Redis health
	if err := dm.redisClient.Ping(ctx).Err(); err != nil {
		hc.logger.Error("Redis health check failed", zap.Error(err))
	}
	
	// Update connection metrics
	dm.updateConnectionMetrics()
}

// updateConnectionMetrics updates connection pool metrics
func (dm *DatabaseManager) updateConnectionMetrics() {
	// Update write DB metrics
	if sqlDB, err := dm.writeDB.DB(); err == nil {
		stats := sqlDB.Stats()
		dm.dbMetrics.ConnectionCount.WithLabelValues("write", "open").Set(float64(stats.OpenConnections))
		dm.dbMetrics.ConnectionCount.WithLabelValues("write", "idle").Set(float64(stats.Idle))
		dm.dbMetrics.ConnectionCount.WithLabelValues("write", "in_use").Set(float64(stats.InUse))
	}
	
	// Update read DB metrics
	for i, readDB := range dm.readDBs {
		if sqlDB, err := readDB.DB(); err == nil {
			stats := sqlDB.Stats()
			label := fmt.Sprintf("read_%d", i)
			dm.dbMetrics.ConnectionCount.WithLabelValues(label, "open").Set(float64(stats.OpenConnections))
			dm.dbMetrics.ConnectionCount.WithLabelValues(label, "idle").Set(float64(stats.Idle))
			dm.dbMetrics.ConnectionCount.WithLabelValues(label, "in_use").Set(float64(stats.InUse))
		}
	}
	
	// Update pgx pool metrics
	if dm.pgxPool != nil {
		stats := dm.pgxPool.Stat()
		dm.dbMetrics.ConnectionCount.WithLabelValues("pgx", "acquired").Set(float64(stats.AcquiredConns()))
		dm.dbMetrics.ConnectionCount.WithLabelValues("pgx", "idle").Set(float64(stats.IdleConns()))
		dm.dbMetrics.ConnectionCount.WithLabelValues("pgx", "total").Set(float64(stats.TotalConns()))
	}
}
