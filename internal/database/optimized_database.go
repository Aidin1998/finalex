package database

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// OptimizedDatabase represents the complete optimized database system
type OptimizedDatabase struct {
	config          *DatabaseConfig
	connectionMgr   *ConnectionManager
	queryOptimizer  *QueryOptimizer
	enhancedRepo    *EnhancedRepository
	indexManager    *IndexManager
	queryRouter     *QueryRouter
	monitoring      *MonitoringDashboard
	schemaOptimizer *SchemaOptimizer
	// Internal components
	redisClient *redis.Client
	masterDB    *gorm.DB
	replicaDBs  []*gorm.DB
	logger      *zap.Logger

	// Lifecycle management
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	started bool
	mu      sync.RWMutex
}

// NewOptimizedDatabase creates a new optimized database instance
func NewOptimizedDatabase(config *DatabaseConfig) (*OptimizedDatabase, error) {
	if config == nil {
		config = DefaultConfig()
	}
	ctx, cancel := context.WithCancel(context.Background())

	// Initialize logger
	logger, err := zap.NewProduction()
	if err != nil {
		return nil, fmt.Errorf("failed to create logger: %w", err)
	}

	db := &OptimizedDatabase{
		config: config,
		ctx:    ctx,
		cancel: cancel,
		logger: logger,
	}

	if err := db.initialize(); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to initialize optimized database: %w", err)
	}

	return db, nil
}

// initialize sets up all database components
func (db *OptimizedDatabase) initialize() error {
	// Initialize Redis client
	if err := db.initializeRedis(); err != nil {
		return fmt.Errorf("failed to initialize Redis: %w", err)
	}

	// Initialize connection manager
	if err := db.initializeConnectionManager(); err != nil {
		return fmt.Errorf("failed to initialize connection manager: %w", err)
	}

	// Initialize query optimizer
	if err := db.initializeQueryOptimizer(); err != nil {
		return fmt.Errorf("failed to initialize query optimizer: %w", err)
	}

	// Initialize enhanced repository
	if err := db.initializeEnhancedRepository(); err != nil {
		return fmt.Errorf("failed to initialize enhanced repository: %w", err)
	}

	// Initialize index manager
	if err := db.initializeIndexManager(); err != nil {
		return fmt.Errorf("failed to initialize index manager: %w", err)
	}

	// Initialize query router
	if err := db.initializeQueryRouter(); err != nil {
		return fmt.Errorf("failed to initialize query router: %w", err)
	}

	// Initialize monitoring
	if err := db.initializeMonitoring(); err != nil {
		return fmt.Errorf("failed to initialize monitoring: %w", err)
	}

	// Initialize schema optimizer
	if err := db.initializeSchemaOptimizer(); err != nil {
		return fmt.Errorf("failed to initialize schema optimizer: %w", err)
	}

	return nil
}

func (db *OptimizedDatabase) initializeRedis() error {
	if !db.config.Cache.Enabled {
		return nil
	}

	db.redisClient = redis.NewClient(&redis.Options{
		Addr:         db.config.Cache.Redis.Addr,
		Password:     db.config.Cache.Redis.Password,
		DB:           db.config.Cache.Redis.DB,
		PoolSize:     db.config.Cache.Redis.PoolSize,
		MinIdleConns: db.config.Cache.Redis.MinIdleConns,
		DialTimeout:  db.config.Cache.Redis.DialTimeout,
		ReadTimeout:  db.config.Cache.Redis.ReadTimeout,
		WriteTimeout: db.config.Cache.Redis.WriteTimeout,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(db.ctx, 5*time.Second)
	defer cancel()

	if err := db.redisClient.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return nil
}

func (db *OptimizedDatabase) initializeConnectionManager() error {
	// Create ConnectionConfig from Master and Replica configs
	connConfig := ConnectionConfig{
		Master:   ConnectionPoolConfig{},   // Will be populated from db.config.Master
		Replicas: []ConnectionPoolConfig{}, // Will be populated from db.config.Replica
	}

	var err error
	db.connectionMgr, err = NewConnectionManager(connConfig, db.logger)
	if err != nil {
		return err
	}

	// Get database connections
	db.masterDB = db.connectionMgr.GetMasterDB()
	db.replicaDBs = db.connectionMgr.GetReplicaDBs()

	return nil
}

func (db *OptimizedDatabase) initializeQueryOptimizer() error {
	if !db.config.QueryOptimizer.Enabled {
		return nil
	}

	// Create OptimizerConfig from DatabaseConfig
	optimizerConfig := OptimizerConfig{
		CacheEnabled:       true,
		SlowQueryThreshold: db.config.QueryOptimizer.SlowQueryThreshold,
		MaxCacheSize:       int64(db.config.QueryOptimizer.CacheSize),
		CacheTTL:           5 * time.Minute, // Default TTL
		AnalyzeInterval:    time.Hour,       // Default analyze interval
		VacuumInterval:     24 * time.Hour,  // Default vacuum interval
	}

	db.queryOptimizer = NewQueryOptimizer(
		db.masterDB,
		db.redisClient,
		db.logger,
		optimizerConfig,
	)

	return nil
}

func (db *OptimizedDatabase) initializeEnhancedRepository() error {
	// Get ReadWriteDB from connection manager
	readWriteDB, err := db.connectionMgr.GetReadWriteDB()
	if err != nil {
		return fmt.Errorf("failed to get ReadWriteDB: %w", err)
	}

	// Create enhanced repository config
	repoConfig := &EnhancedRepositoryConfig{
		OrderCacheTTL:    30 * time.Second,
		TradeCacheTTL:    2 * time.Minute,
		UserOrdersTTL:    10 * time.Second,
		PairOrdersTTL:    5 * time.Second,
		EnableQueryCache: true,
		EnableIndexHints: true,
	}

	db.enhancedRepo = NewEnhancedRepository(
		readWriteDB,
		db.queryOptimizer,
		db.redisClient,
		db.logger,
		repoConfig,
	)

	return nil
}

func (db *OptimizedDatabase) initializeIndexManager() error {
	if !db.config.Index.AutoCreate && !db.config.Index.UsageAnalysis {
		return nil
	}
	indexConfig := &IndexManagerConfig{
		EnableConcurrentIndexing: db.config.Index.AutoCreate,
		IndexCreationTimeout:     30 * time.Second,
		AnalyzeAfterCreation:     true,
		MonitoringEnabled:        db.config.Index.UsageAnalysis,
	}

	db.indexManager = NewIndexManager(db.masterDB, db.logger, indexConfig)

	return nil
}

func (db *OptimizedDatabase) initializeQueryRouter() error {
	if !db.config.Routing.Enabled {
		return nil
	}

	// Convert []*gorm.DB to []*ReplicaDB
	var replicaDBs []*ReplicaDB
	for i, replicaDB := range db.replicaDBs {
		replica := &ReplicaDB{
			db:     replicaDB,
			name:   fmt.Sprintf("replica-%d", i),
			weight: 100, // Default weight
		}
		// Mark as healthy initially
		replica.healthy = 1
		replicaDBs = append(replicaDBs, replica)
	}

	routerConfig := &QueryRouterConfig{
		Strategy:            db.config.Routing.Strategy,
		ConsistencyLevel:    db.config.Routing.ConsistencyLevel,
		MaxReplicationLag:   db.config.Routing.MaxReplicationLag,
		ReadPreference:      db.config.Routing.ReadPreference,
		HealthCheckInterval: 30 * time.Second,
		HealthCheckTimeout:  5 * time.Second,
		MaxReplicaLatency:   100 * time.Millisecond,
	}

	db.queryRouter = NewQueryRouter(db.masterDB, replicaDBs, routerConfig, db.logger)
	return nil
}

func (db *OptimizedDatabase) initializeMonitoring() error {
	if !db.config.Monitoring.Enabled {
		return nil
	}

	// Get ReadWriteDB from connection manager
	readWriteDB, err := db.connectionMgr.GetReadWriteDB()
	if err != nil {
		return fmt.Errorf("failed to get ReadWriteDB: %w", err)
	}

	// Create monitoring config
	monitoringConfig := &MonitoringConfig{
		CollectionInterval:   db.config.Monitoring.MetricsInterval,
		RetentionPeriod:      db.config.Monitoring.HistoryRetention,
		SlowQueryThreshold:   db.config.Monitoring.SlowQueryThreshold,
		HighLatencyThreshold: 50 * time.Millisecond,
		LowCacheHitThreshold: 0.8,
		AlertsEnabled:        true,
		MetricsPrefix:        "pincex_db",
	}

	db.monitoring = NewMonitoringDashboard(
		readWriteDB,
		db.queryOptimizer,
		db.queryRouter,
		db.indexManager,
		db.redisClient,
		db.logger,
		monitoringConfig,
	)

	return nil
}

func (db *OptimizedDatabase) initializeSchemaOptimizer() error {
	if !db.config.Schema.Partitioning.Enabled {
		return nil
	}

	// Create schema optimizer config
	schemaConfig := &SchemaOptimizerConfig{
		EnablePartitioning: db.config.Schema.Partitioning.Enabled,
		PartitionInterval:  "daily",
		RetentionPeriod:    30 * 24 * time.Hour,
		EnableConstraints:  true,
		EnableCompression:  true,
		OptimizeStatistics: true,
		AutoVacuumSettings: &VacuumConfig{
			Enabled:            true,
			ScaleFactor:        0.1,
			Threshold:          50,
			AnalyzeScaleFactor: 0.05,
			AnalyzeThreshold:   50,
			VacuumCostDelay:    10,
			VacuumCostLimit:    200,
		},
	}

	db.schemaOptimizer = NewSchemaOptimizer(db.masterDB, db.logger, schemaConfig)

	return nil
}

// Start begins all background processes
func (db *OptimizedDatabase) Start() error {
	db.mu.Lock()
	defer db.mu.Unlock()

	if db.started {
		return fmt.Errorf("database already started")
	}

	// Start connection manager
	if db.connectionMgr != nil {
		db.wg.Add(1)
		go func() {
			defer db.wg.Done()
			db.connectionMgr.Start(db.ctx)
		}()
	}

	// Start query optimizer
	if db.queryOptimizer != nil && db.config.QueryOptimizer.Enabled {
		db.wg.Add(1)
		go func() {
			defer db.wg.Done()
			db.queryOptimizer.StartMaintenance(db.ctx, db.config.QueryOptimizer.MaintenanceInterval)
		}()
	}

	// Start index manager
	if db.indexManager != nil && db.config.Index.UsageAnalysis {
		db.wg.Add(1)
		go func() {
			defer db.wg.Done()
			db.startIndexMaintenance()
		}()
	}

	// Start monitoring
	if db.monitoring != nil && db.config.Monitoring.Enabled {
		db.wg.Add(1)
		go func() {
			defer db.wg.Done()
			go db.monitoring.startMetricCollection()
		}()
	}

	// Start schema optimizer
	if db.schemaOptimizer != nil && db.config.Schema.Partitioning.Enabled {
		db.wg.Add(1)
		go func() {
			defer db.wg.Done()
			db.startSchemaOptimization()
		}()
	}

	// Start alert monitoring
	if db.monitoring != nil {
		db.wg.Add(1)
		go func() {
			defer db.wg.Done()
			db.startAlertMonitoring()
		}()
	}

	db.started = true
	log.Println("Optimized database system started successfully")

	return nil
}

// Stop gracefully shuts down all components
func (db *OptimizedDatabase) Stop() error {
	db.mu.Lock()
	defer db.mu.Unlock()

	if !db.started {
		return nil
	}

	log.Println("Stopping optimized database system...")

	// Cancel context to signal shutdown
	db.cancel()

	// Wait for all goroutines to finish
	done := make(chan struct{})
	go func() {
		db.wg.Wait()
		close(done)
	}()

	// Wait for graceful shutdown or timeout
	select {
	case <-done:
		log.Println("All components stopped gracefully")
	case <-time.After(30 * time.Second):
		log.Println("Timeout waiting for components to stop")
	}

	// Close Redis connection
	if db.redisClient != nil {
		if err := db.redisClient.Close(); err != nil {
			log.Printf("Error closing Redis connection: %v", err)
		}
	}

	// Close database connections
	if db.connectionMgr != nil {
		db.connectionMgr.Close()
	}

	db.started = false
	log.Println("Optimized database system stopped")

	return nil
}

// GetRepository returns the enhanced repository
func (db *OptimizedDatabase) GetRepository() *EnhancedRepository {
	return db.enhancedRepo
}

// GetQueryRouter returns the query router for manual routing
func (db *OptimizedDatabase) GetQueryRouter() *QueryRouter {
	return db.queryRouter
}

// GetMonitoring returns the monitoring dashboard
func (db *OptimizedDatabase) GetMonitoring() *MonitoringDashboard {
	return db.monitoring
}

// ExecuteQuery executes a query with full optimization
func (db *OptimizedDatabase) ExecuteQuery(ctx context.Context, query string, args ...interface{}) (*QueryResult, error) {
	// Route query based on configuration
	var targetDB *gorm.DB
	if db.queryRouter != nil {
		// Determine query type and table name for routing
		queryType := QueryTypeRead // Default to read; you may want to parse query for type
		table := ""                // Table name extraction logic needed if available
		targetDB = db.queryRouter.RouteQuery(ctx, queryType, table)
	} else {
		targetDB = db.masterDB
	}

	// Use query optimizer if available
	if db.queryOptimizer != nil {
		return db.queryOptimizer.ExecuteOptimizedQuery(ctx, targetDB, query, args...)
	}

	// Fallback to direct execution
	start := time.Now()
	var result QueryResult
	err := targetDB.WithContext(ctx).Raw(query, args...).Scan(&result.Data).Error
	result.ExecutionTime = time.Since(start)
	result.Query = query
	result.Args = args

	return &result, err
}

// GetHealthStatus returns the health status of all components
func (db *OptimizedDatabase) GetHealthStatus() map[string]interface{} {
	status := make(map[string]interface{})

	// Connection manager health
	if db.connectionMgr != nil {
		status["connection_manager"] = db.connectionMgr.GetHealthStatus()
	}

	// Redis health
	if db.redisClient != nil {
		ctx, cancel := context.WithTimeout(db.ctx, 5*time.Second)
		defer cancel()
		err := db.redisClient.Ping(ctx).Err()
		status["redis"] = map[string]interface{}{
			"status": err == nil,
			"error":  err,
		}
	}

	// Monitoring metrics
	if db.monitoring != nil {
		status["metrics"] = db.monitoring.GetCurrentMetrics()
	}

	return status
}

// Background maintenance functions

func (db *OptimizedDatabase) startIndexMaintenance() {
	ticker := time.NewTicker(1 * time.Hour) // Check hourly
	defer ticker.Stop()

	for {
		select {
		case <-db.ctx.Done():
			return
		case <-ticker.C:
			if db.indexManager != nil {
				// Index usage analysis not implemented
				// TODO: Implement index usage analysis if needed
			}
		}
	}
}

func (db *OptimizedDatabase) startSchemaOptimization() {
	ticker := time.NewTicker(24 * time.Hour) // Check daily
	defer ticker.Stop()

	for {
		select {
		case <-db.ctx.Done():
			return
		case <-ticker.C:
			if db.schemaOptimizer != nil {
				// Future partition creation not implemented
				// TODO: Implement future partition creation if needed
				// Clean up old partitions based on retention policy
				err := db.schemaOptimizer.CleanupOldPartitions(db.ctx)
				if err != nil {
					log.Printf("Failed to cleanup old partitions: %v", err)
				}
			}
		}
	}
}

func (db *OptimizedDatabase) startAlertMonitoring() {
	ticker := time.NewTicker(db.config.Monitoring.MetricsInterval)
	defer ticker.Stop()

	for {
		select {
		case <-db.ctx.Done():
			return
		case <-ticker.C:
			if db.monitoring != nil {
				metrics := db.monitoring.GetCurrentMetrics()
				if metrics != nil {
					db.checkAlertThresholds(metrics.ToMap())
				}
			}
		}
	}
}

func (db *OptimizedDatabase) checkAlertThresholds(metrics map[string]interface{}) {
	alerts := db.config.Monitoring.AlertThresholds

	// Check query latency
	if queryMetrics, ok := metrics["query"].(map[string]interface{}); ok {
		if avgLatency, ok := queryMetrics["avg_latency"].(time.Duration); ok {
			if avgLatency > alerts.MaxQueryLatency {
				log.Printf("ALERT: Average query latency (%v) exceeds threshold (%v)",
					avgLatency, alerts.MaxQueryLatency)
			}
		}
	}

	// Check connection usage
	if connMetrics, ok := metrics["connections"].(map[string]interface{}); ok {
		if usage, ok := connMetrics["usage_percentage"].(float64); ok {
			if usage > alerts.MaxConnectionUsage {
				log.Printf("ALERT: Connection usage (%.2f%%) exceeds threshold (%.2f%%)",
					usage*100, alerts.MaxConnectionUsage*100)
			}
		}
	}

	// Check cache hit rate
	if cacheMetrics, ok := metrics["cache"].(map[string]interface{}); ok {
		if hitRate, ok := cacheMetrics["hit_rate"].(float64); ok {
			if hitRate < alerts.MinCacheHitRate {
				log.Printf("ALERT: Cache hit rate (%.2f%%) below threshold (%.2f%%)",
					hitRate*100, alerts.MinCacheHitRate*100)
			}
		}
	}

	// Check replication lag
	if replMetrics, ok := metrics["replication"].(map[string]interface{}); ok {
		if lag, ok := replMetrics["max_lag"].(time.Duration); ok {
			if lag > alerts.MaxReplicationLag {
				log.Printf("ALERT: Replication lag (%v) exceeds threshold (%v)",
					lag, alerts.MaxReplicationLag)
			}
		}
	}
}
