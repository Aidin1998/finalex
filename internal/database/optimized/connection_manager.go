package database

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// ConnectionPoolStats holds connection pool statistics
type ConnectionPoolStats struct {
	Open         int32         `json:"open"`
	InUse        int32         `json:"in_use"`
	Idle         int32         `json:"idle"`
	WaitCount    int64         `json:"wait_count"`
	WaitDuration time.Duration `json:"wait_duration"`
	MaxIdleTime  time.Duration `json:"max_idle_time"`
	MaxLifetime  time.Duration `json:"max_lifetime"`
}

// ConnectionManager manages database connections with advanced pooling and health checks
type ConnectionManager struct {
	pools         map[string]*ConnectionPool
	config        ConnectionConfig
	logger        *zap.Logger
	metrics       *ConnectionMetrics
	mu            sync.RWMutex
	healthChecker *HealthChecker
}

// ConnectionConfig contains connection pool configuration
type ConnectionConfig struct {
	Master        ConnectionPoolConfig   `json:"master"`
	Replicas      []ConnectionPoolConfig `json:"replicas"`
	HealthCheck   HealthCheckConfig      `json:"health_check"`
	LoadBalancing LoadBalancingConfig    `json:"load_balancing"`
	Failover      FailoverConfig         `json:"failover"`
}

// ConnectionPoolConfig contains individual pool configuration
type ConnectionPoolConfig struct {
	DSN              string        `json:"dsn"`
	MaxOpenConns     int           `json:"max_open_conns"`
	MaxIdleConns     int           `json:"max_idle_conns"`
	ConnMaxLifetime  time.Duration `json:"conn_max_lifetime"`
	ConnMaxIdleTime  time.Duration `json:"conn_max_idle_time"`
	ConnTimeout      time.Duration `json:"conn_timeout"`
	ReadTimeout      time.Duration `json:"read_timeout"`
	WriteTimeout     time.Duration `json:"write_timeout"`
	StatementTimeout time.Duration `json:"statement_timeout"`
	Name             string        `json:"name"`
	Role             string        `json:"role"` // master, replica
	Priority         int           `json:"priority"`
	Weight           int           `json:"weight"` // for load balancing
}

// LoadBalancingConfig contains load balancing configuration
type LoadBalancingConfig struct {
	Strategy       string        `json:"strategy"` // round_robin, weighted, least_connections
	HealthyOnly    bool          `json:"healthy_only"`
	ReadPreference string        `json:"read_preference"` // primary, secondary, any
	MaxRetries     int           `json:"max_retries"`
	RetryDelay     time.Duration `json:"retry_delay"`
}

// FailoverConfig contains failover configuration
type FailoverConfig struct {
	Enabled             bool          `json:"enabled"`
	AutoFailover        bool          `json:"auto_failover"`
	FailoverTimeout     time.Duration `json:"failover_timeout"`
	RecoveryTimeout     time.Duration `json:"recovery_timeout"`
	NotificationEnabled bool          `json:"notification_enabled"`
}

// ConnectionPool represents a managed connection pool
type ConnectionPool struct {
	db           *gorm.DB
	sqlDB        *sql.DB
	config       ConnectionPoolConfig
	metrics      *PoolMetrics
	healthy      bool
	lastCheck    time.Time
	failureCount int
	successCount int
	mu           sync.RWMutex
}

// PoolMetrics contains pool-specific metrics
type PoolMetrics struct {
	OpenConnections   int           `json:"open_connections"`
	IdleConnections   int           `json:"idle_connections"`
	InUseConnections  int           `json:"in_use_connections"`
	WaitCount         int64         `json:"wait_count"`
	WaitDuration      time.Duration `json:"wait_duration"`
	MaxIdleClosed     int64         `json:"max_idle_closed"`
	MaxLifetimeClosed int64         `json:"max_lifetime_closed"`
	QueryCount        int64         `json:"query_count"`
	ErrorCount        int64         `json:"error_count"`
	AvgResponseTime   time.Duration `json:"avg_response_time"`
	LastActivity      time.Time     `json:"last_activity"`
}

// ConnectionMetrics contains overall connection metrics
type ConnectionMetrics struct {
	TotalPools       int                     `json:"total_pools"`
	HealthyPools     int                     `json:"healthy_pools"`
	UnhealthyPools   int                     `json:"unhealthy_pools"`
	TotalConnections int                     `json:"total_connections"`
	TotalQueries     int64                   `json:"total_queries"`
	TotalErrors      int64                   `json:"total_errors"`
	PoolMetrics      map[string]*PoolMetrics `json:"pool_metrics"`
	LastUpdated      time.Time               `json:"last_updated"`
	// Additional fields for monitoring dashboard
	MasterConnections  ConnectionPoolStats            `json:"master_connections"`
	ReplicaConnections map[string]ConnectionPoolStats `json:"replica_connections"`
	FailoverEvents     int64                          `json:"failover_events"`
	ConnectionErrors   int64                          `json:"connection_errors"`
	PoolUtilization    float64                        `json:"pool_utilization"`
}

// HealthChecker monitors connection pool health
type HealthChecker struct {
	config  HealthCheckConfig
	manager *ConnectionManager
	logger  *zap.Logger
	stopCh  chan struct{}
	stopped bool
	mu      sync.RWMutex
}

// ReadWriteDB provides read/write separated database access
type ReadWriteDB struct {
	manager   *ConnectionManager
	writePool *ConnectionPool
	readPools []*ConnectionPool
	rrIndex   int
	mu        sync.RWMutex
}

// NewConnectionManager creates a new connection manager
func NewConnectionManager(config ConnectionConfig, logger *zap.Logger) (*ConnectionManager, error) {
	manager := &ConnectionManager{
		pools:  make(map[string]*ConnectionPool),
		config: config,
		logger: logger,
		metrics: &ConnectionMetrics{
			PoolMetrics: make(map[string]*PoolMetrics),
		},
		healthChecker: &HealthChecker{
			config: config.HealthCheck,
			logger: logger,
			stopCh: make(chan struct{}),
		},
	}

	manager.healthChecker.manager = manager

	// Initialize connection pools
	if err := manager.initializePools(); err != nil {
		return nil, fmt.Errorf("failed to initialize connection pools: %w", err)
	}

	return manager, nil
}

// Start begins connection management and health checking
func (cm *ConnectionManager) Start(ctx context.Context) error {
	cm.logger.Info("Starting connection manager")

	// Start health checking
	if cm.config.HealthCheck.Enabled {
		go cm.healthChecker.start(ctx)
	}

	// Start metrics collection
	go cm.collectMetrics(ctx)

	return nil
}

// Stop gracefully shuts down the connection manager
func (cm *ConnectionManager) Stop() error {
	cm.logger.Info("Stopping connection manager")

	// Stop health checker
	cm.healthChecker.stop()

	// Close all pools
	cm.mu.Lock()
	defer cm.mu.Unlock()

	for name, pool := range cm.pools {
		if err := pool.close(); err != nil {
			cm.logger.Error("Failed to close pool", zap.String("pool", name), zap.Error(err))
		}
	}

	return nil
}

// GetReadWriteDB returns a read/write separated database connection
func (cm *ConnectionManager) GetReadWriteDB() (*ReadWriteDB, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	// Find write pool (master)
	var writePool *ConnectionPool
	readPools := []*ConnectionPool{}

	for _, pool := range cm.pools {
		if pool.config.Role == "master" && pool.healthy {
			writePool = pool
		} else if pool.config.Role == "replica" && pool.healthy {
			readPools = append(readPools, pool)
		}
	}

	if writePool == nil {
		return nil, fmt.Errorf("no healthy write pool available")
	}

	return &ReadWriteDB{
		manager:   cm,
		writePool: writePool,
		readPools: readPools,
	}, nil
}

// GetPool returns a specific connection pool by name
func (cm *ConnectionManager) GetPool(name string) (*ConnectionPool, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	pool, exists := cm.pools[name]
	if !exists {
		return nil, fmt.Errorf("pool %s not found", name)
	}

	if !pool.healthy {
		return nil, fmt.Errorf("pool %s is unhealthy", name)
	}

	return pool, nil
}

// GetMasterDB returns the master database connection
func (cm *ConnectionManager) GetMasterDB() *gorm.DB {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	if pool, exists := cm.pools["master"]; exists {
		return pool.db
	}
	return nil
}

// GetReplicaDBs returns all replica database connections
func (cm *ConnectionManager) GetReplicaDBs() []*gorm.DB {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	var replicas []*gorm.DB
	for name, pool := range cm.pools {
		if name != "master" && pool.db != nil {
			replicas = append(replicas, pool.db)
		}
	}
	return replicas
}

// GetHealthStatus returns the health status of all connections
func (cm *ConnectionManager) GetHealthStatus() map[string]interface{} {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	status := make(map[string]interface{})
	for name, pool := range cm.pools {
		isHealthy := true
		var err error

		if pool.db != nil {
			sqlDB, dbErr := pool.db.DB()
			if dbErr == nil {
				err = sqlDB.Ping()
				isHealthy = (err == nil)
			} else {
				err = dbErr
				isHealthy = false
			}
		} else {
			isHealthy = false
			err = fmt.Errorf("database connection is nil")
		}

		status[name] = map[string]interface{}{
			"healthy": isHealthy,
			"error":   err,
		}
	}
	return status
}

// Close closes all database connections
func (cm *ConnectionManager) Close() error {
	return cm.Stop()
}

// GetMetrics returns current connection metrics
func (cm *ConnectionManager) GetMetrics() *ConnectionMetrics {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	// Create a copy of metrics
	metrics := &ConnectionMetrics{
		TotalPools:  len(cm.pools),
		PoolMetrics: make(map[string]*PoolMetrics),
		LastUpdated: time.Now(),
	}

	healthyCount := 0
	totalConnections := 0
	totalQueries := int64(0)
	totalErrors := int64(0)

	for name, pool := range cm.pools {
		poolMetrics := pool.getMetrics()
		metrics.PoolMetrics[name] = poolMetrics

		if pool.healthy {
			healthyCount++
		}

		totalConnections += poolMetrics.OpenConnections
		totalQueries += poolMetrics.QueryCount
		totalErrors += poolMetrics.ErrorCount
	}

	metrics.HealthyPools = healthyCount
	metrics.UnhealthyPools = len(cm.pools) - healthyCount
	metrics.TotalConnections = totalConnections
	metrics.TotalQueries = totalQueries
	metrics.TotalErrors = totalErrors

	return metrics
}

// Private methods

func (cm *ConnectionManager) initializePools() error {
	// Initialize master pool
	if err := cm.createPool(cm.config.Master); err != nil {
		return fmt.Errorf("failed to create master pool: %w", err)
	}

	// Initialize replica pools
	for _, replicaConfig := range cm.config.Replicas {
		if err := cm.createPool(replicaConfig); err != nil {
			cm.logger.Error("Failed to create replica pool",
				zap.String("name", replicaConfig.Name), zap.Error(err))
		}
	}

	return nil
}

func (cm *ConnectionManager) createPool(config ConnectionPoolConfig) error {
	db, err := NewOptimizedPostgresDB(config.DSN, config)
	if err != nil {
		return err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return err
	}

	pool := &ConnectionPool{
		db:        db,
		sqlDB:     sqlDB,
		config:    config,
		healthy:   true,
		lastCheck: time.Now(),
		metrics:   &PoolMetrics{},
	}

	// Configure connection pool
	pool.configure()

	cm.pools[config.Name] = pool
	cm.logger.Info("Created connection pool",
		zap.String("name", config.Name),
		zap.String("role", config.Role),
		zap.Int("max_open_conns", config.MaxOpenConns))

	return nil
}

func (cm *ConnectionManager) collectMetrics(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cm.updateMetrics()
		}
	}
}

func (cm *ConnectionManager) updateMetrics() {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	for _, pool := range cm.pools {
		pool.updateMetrics()
	}
}

// ConnectionPool methods

func (cp *ConnectionPool) configure() {
	// Set connection pool parameters
	cp.sqlDB.SetMaxOpenConns(cp.config.MaxOpenConns)
	cp.sqlDB.SetMaxIdleConns(cp.config.MaxIdleConns)
	cp.sqlDB.SetConnMaxLifetime(cp.config.ConnMaxLifetime)
	cp.sqlDB.SetConnMaxIdleTime(cp.config.ConnMaxIdleTime)
}

func (cp *ConnectionPool) getMetrics() *PoolMetrics {
	cp.mu.RLock()
	defer cp.mu.RUnlock()

	stats := cp.sqlDB.Stats()

	return &PoolMetrics{
		OpenConnections:   stats.OpenConnections,
		IdleConnections:   stats.Idle,
		InUseConnections:  stats.InUse,
		WaitCount:         stats.WaitCount,
		WaitDuration:      stats.WaitDuration,
		MaxIdleClosed:     stats.MaxIdleClosed,
		MaxLifetimeClosed: stats.MaxLifetimeClosed,
		LastActivity:      time.Now(),
	}
}

func (cp *ConnectionPool) updateMetrics() {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	stats := cp.sqlDB.Stats()
	cp.metrics.OpenConnections = stats.OpenConnections
	cp.metrics.IdleConnections = stats.Idle
	cp.metrics.InUseConnections = stats.InUse
	cp.metrics.WaitCount = stats.WaitCount
	cp.metrics.WaitDuration = stats.WaitDuration
	cp.metrics.MaxIdleClosed = stats.MaxIdleClosed
	cp.metrics.MaxLifetimeClosed = stats.MaxLifetimeClosed
	cp.metrics.LastActivity = time.Now()
}

func (cp *ConnectionPool) isHealthy() bool {
	cp.mu.RLock()
	defer cp.mu.RUnlock()
	return cp.healthy
}

func (cp *ConnectionPool) setHealthy(healthy bool) {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	if cp.healthy != healthy {
		cp.healthy = healthy
		if healthy {
			cp.successCount++
			cp.failureCount = 0
		} else {
			cp.failureCount++
		}
	}
}

func (cp *ConnectionPool) close() error {
	return cp.sqlDB.Close()
}

// HealthChecker methods

func (hc *HealthChecker) start(ctx context.Context) {
	if !hc.config.Enabled {
		return
	}

	ticker := time.NewTicker(hc.config.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-hc.stopCh:
			return
		case <-ticker.C:
			hc.checkAllPools()
		}
	}
}

func (hc *HealthChecker) stop() {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	if !hc.stopped {
		close(hc.stopCh)
		hc.stopped = true
	}
}

func (hc *HealthChecker) checkAllPools() {
	hc.manager.mu.RLock()
	pools := make([]*ConnectionPool, 0, len(hc.manager.pools))
	for _, pool := range hc.manager.pools {
		pools = append(pools, pool)
	}
	hc.manager.mu.RUnlock()

	for _, pool := range pools {
		go hc.checkPool(pool)
	}
}

func (hc *HealthChecker) checkPool(pool *ConnectionPool) {
	ctx, cancel := context.WithTimeout(context.Background(), hc.config.Timeout)
	defer cancel()

	healthy := true

	// Basic ping test
	if err := pool.sqlDB.PingContext(ctx); err != nil {
		hc.logger.Warn("Pool ping failed",
			zap.String("pool", pool.config.Name), zap.Error(err))
		healthy = false
	}

	// Custom health check queries
	if healthy {
		for _, query := range hc.config.CustomQueries {
			if err := hc.executeHealthQuery(ctx, pool, query); err != nil {
				hc.logger.Warn("Custom health check failed",
					zap.String("pool", pool.config.Name),
					zap.String("query", query),
					zap.Error(err))
				healthy = false
				break
			}
		}
	}

	// Update pool health status
	pool.setHealthy(healthy)
	pool.lastCheck = time.Now()

	// Log health status changes
	if pool.isHealthy() != healthy {
		if healthy {
			hc.logger.Info("Pool recovered", zap.String("pool", pool.config.Name))
		} else {
			hc.logger.Error("Pool became unhealthy", zap.String("pool", pool.config.Name))
		}
	}
}

func (hc *HealthChecker) executeHealthQuery(ctx context.Context, pool *ConnectionPool, query string) error {
	rows, err := pool.sqlDB.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()

	// Ensure we can read the result
	for rows.Next() {
		// Just verify we can iterate
	}

	return rows.Err()
}

// ReadWriteDB methods

func (rw *ReadWriteDB) Write() *gorm.DB {
	return rw.writePool.db
}

func (rw *ReadWriteDB) Read() *gorm.DB {
	if len(rw.readPools) == 0 {
		// Fallback to write pool if no read replicas
		return rw.writePool.db
	}

	// Round-robin load balancing
	rw.mu.Lock()
	pool := rw.readPools[rw.rrIndex]
	rw.rrIndex = (rw.rrIndex + 1) % len(rw.readPools)
	rw.mu.Unlock()

	return pool.db
}

func (rw *ReadWriteDB) ReadPreferred() *gorm.DB {
	// Try read replicas first, fallback to write
	if len(rw.readPools) > 0 {
		return rw.Read()
	}
	return rw.Write()
}

// NewOptimizedPostgresDB creates an optimized PostgreSQL connection
func NewOptimizedPostgresDB(dsn string, config ConnectionPoolConfig) (*gorm.DB, error) {
	// Add connection-level optimizations to DSN
	optimizedDSN := dsn
	if config.ConnTimeout > 0 {
		optimizedDSN += fmt.Sprintf(" connect_timeout=%d", int(config.ConnTimeout.Seconds()))
	}
	if config.ReadTimeout > 0 {
		optimizedDSN += fmt.Sprintf(" read_timeout=%d", int(config.ReadTimeout.Seconds()))
	}
	if config.WriteTimeout > 0 {
		optimizedDSN += fmt.Sprintf(" write_timeout=%d", int(config.WriteTimeout.Seconds()))
	}
	if config.StatementTimeout > 0 {
		optimizedDSN += fmt.Sprintf(" statement_timeout=%d", int(config.StatementTimeout.Milliseconds()))
	}

	return NewPostgresDB(optimizedDSN,
		config.MaxOpenConns,
		config.MaxIdleConns,
		int(config.ConnMaxLifetime.Seconds()))
}

// DefaultConnectionConfig returns default connection configuration
func DefaultConnectionConfig() ConnectionConfig {
	return ConnectionConfig{Master: ConnectionPoolConfig{
		MaxOpenConns:     100,
		MaxIdleConns:     10,
		ConnMaxLifetime:  1 * time.Hour,
		ConnMaxIdleTime:  30 * time.Minute,
		ConnTimeout:      10 * time.Second,
		ReadTimeout:      30 * time.Second,
		WriteTimeout:     30 * time.Second,
		StatementTimeout: 60 * time.Second,
		Name:             "master",
		Role:             "master",
		Priority:         1,
		Weight:           100,
	},
		HealthCheck: HealthCheckConfig{
			Enabled:           true,
			Interval:          30 * time.Second,
			Timeout:           5 * time.Second,
			RetryAttempts:     3,
			FailureThreshold:  3,
			RecoveryThreshold: 2,
			CustomQueries:     []string{"SELECT 1"},
		},
		LoadBalancing: LoadBalancingConfig{
			Strategy:       "round_robin",
			HealthyOnly:    true,
			ReadPreference: "secondary",
			MaxRetries:     3,
			RetryDelay:     100 * time.Millisecond,
		},
		Failover: FailoverConfig{
			Enabled:             true,
			AutoFailover:        true,
			FailoverTimeout:     30 * time.Second,
			RecoveryTimeout:     2 * time.Minute,
			NotificationEnabled: true,
		},
	}
}
