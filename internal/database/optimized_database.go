package database

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
)

// OptimizedDatabase represents the complete optimized database system
type OptimizedDatabase struct {
	config           *DatabaseConfig
	connectionMgr    *ConnectionManager
	queryOptimizer   *QueryOptimizer
	enhancedRepo     *EnhancedRepository
	indexManager     *IndexManager
	queryRouter      *QueryRouter
	monitoring       *MonitoringDashboard
	schemaOptimizer  *SchemaOptimizer
	
	// Internal components
	redisClient      *redis.Client
	masterDB         *gorm.DB
	replicaDBs       []*gorm.DB
	
	// Lifecycle management
	ctx              context.Context
	cancel           context.CancelFunc
	wg               sync.WaitGroup
	started          bool
	mu               sync.RWMutex
}

// NewOptimizedDatabase creates a new optimized database instance
func NewOptimizedDatabase(config *DatabaseConfig) (*OptimizedDatabase, error) {
	if config == nil {
		config = DefaultConfig()
	}
	
	ctx, cancel := context.WithCancel(context.Background())
	
	db := &OptimizedDatabase{
		config: config,
		ctx:    ctx,
		cancel: cancel,
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
	var err error
	db.connectionMgr, err = NewConnectionManager(db.config.Master, db.config.Replica)
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
	
	var err error
	db.queryOptimizer, err = NewQueryOptimizer(
		db.masterDB,
		db.redisClient,
		db.config.QueryOptimizer.SlowQueryThreshold,
		db.config.QueryOptimizer.CacheSize,
	)
	if err != nil {
		return err
	}
	
	return nil
}

func (db *OptimizedDatabase) initializeEnhancedRepository() error {
	var err error
	db.enhancedRepo, err = NewEnhancedRepository(db.masterDB, db.redisClient)
	if err != nil {
		return err
	}
	
	return nil
}

func (db *OptimizedDatabase) initializeIndexManager() error {
	if !db.config.Index.AutoCreate && !db.config.Index.UsageAnalysis {
		return nil
	}
	
	var err error
	db.indexManager, err = NewIndexManager(db.masterDB)
	if err != nil {
		return err
	}
	
	return nil
}

func (db *OptimizedDatabase) initializeQueryRouter() error {
	if !db.config.Routing.Enabled {
		return nil
	}
	
	var err error
	db.queryRouter, err = NewQueryRouter(db.masterDB, db.replicaDBs, QueryRouterConfig{
		Strategy:          db.config.Routing.Strategy,
		ConsistencyLevel:  db.config.Routing.ConsistencyLevel,
		MaxReplicationLag: db.config.Routing.MaxReplicationLag,
		ReadPreference:    db.config.Routing.ReadPreference,
	})
	if err != nil {
		return err
	}
	
	return nil
}

func (db *OptimizedDatabase) initializeMonitoring() error {
	if !db.config.Monitoring.Enabled {
		return nil
	}
	
	var err error
	db.monitoring, err = NewMonitoringDashboard(
		db.masterDB,
		db.replicaDBs,
		db.redisClient,
		db.config.Monitoring.MetricsInterval,
		db.config.Monitoring.HistoryRetention,
	)
	if err != nil {
		return err
	}
	
	return nil
}

func (db *OptimizedDatabase) initializeSchemaOptimizer() error {
	if !db.config.Schema.Partitioning.Enabled {
		return nil
	}
	
	var err error
	db.schemaOptimizer, err = NewSchemaOptimizer(db.masterDB)
	if err != nil {
		return err
	}
	
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
			db.monitoring.Start(db.ctx)
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
		var err error
		targetDB, err = db.queryRouter.RouteQuery(ctx, query, args...)
		if err != nil {
			targetDB = db.masterDB // Fallback to master
		}
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
				// Analyze index usage
				usage, err := db.indexManager.AnalyzeIndexUsage()
				if err != nil {
					log.Printf("Index usage analysis failed: %v", err)
					continue
				}
				
				// Log unused indexes
				for _, idx := range usage {
					if idx.ScanCount == 0 && time.Since(idx.LastUsed) > time.Duration(db.config.Index.UnusedThreshold)*24*time.Hour {
						log.Printf("Unused index detected: %s on table %s (unused for %v)", 
							idx.IndexName, idx.TableName, time.Since(idx.LastUsed))
					}
				}
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
				// Create future partitions
				for tableName, config := range db.config.Schema.Partitioning.Tables {
					if db.config.Schema.Partitioning.PreCreate > 0 {
						err := db.schemaOptimizer.CreateFuturePartitions(tableName, config.Interval, db.config.Schema.Partitioning.PreCreate)
						if err != nil {
							log.Printf("Failed to create future partitions for %s: %v", tableName, err)
						}
					}
				}
				
				// Clean up old partitions based on retention policy
				for tableName, config := range db.config.Schema.Partitioning.Tables {
					err := db.schemaOptimizer.CleanupOldPartitions(tableName, config.Retention)
					if err != nil {
						log.Printf("Failed to cleanup old partitions for %s: %v", tableName, err)
					}
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
				db.checkAlertThresholds(metrics)
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
