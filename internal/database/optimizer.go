package database

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// QueryOptimizer handles query optimization and caching
type QueryOptimizer struct {
	db           *gorm.DB
	redisClient  *redis.Client
	logger       *zap.Logger
	config       OptimizerConfig
	queryCache   *QueryCache
	slowQueries  *SlowQueryTracker
	indexManager *IndexManager
	mu           sync.RWMutex
}

// OptimizerConfig contains configuration for the query optimizer
type OptimizerConfig struct {
	CacheEnabled          bool          `json:"cache_enabled"`
	CacheTTL              time.Duration `json:"cache_ttl"`
	SlowQueryThreshold    time.Duration `json:"slow_query_threshold"`
	MaxCacheSize          int64         `json:"max_cache_size"`
	EnableQueryPlan       bool          `json:"enable_query_plan"`
	EnableIndexOptimizer  bool          `json:"enable_index_optimizer"`
	AnalyzeInterval       time.Duration `json:"analyze_interval"`
	VacuumInterval        time.Duration `json:"vacuum_interval"`
	ConnectionPoolSize    int           `json:"connection_pool_size"`
	MaxIdleConnections    int           `json:"max_idle_connections"`
	ConnectionMaxLifetime time.Duration `json:"connection_max_lifetime"`
}

// QueryCache handles query result caching
type QueryCache struct {
	redis      *redis.Client
	ttl        time.Duration
	maxSize    int64
	currentSize int64
	mu         sync.RWMutex
}

// SlowQueryTracker tracks and analyzes slow queries
type SlowQueryTracker struct {
	threshold   time.Duration
	queries     map[string]*SlowQueryInfo
	mu          sync.RWMutex
}

// SlowQueryInfo contains information about a slow query
type SlowQueryInfo struct {
	Query         string        `json:"query"`
	Count         int64         `json:"count"`
	TotalTime     time.Duration `json:"total_time"`
	AverageTime   time.Duration `json:"average_time"`
	LastSeen      time.Time     `json:"last_seen"`
	Suggestions   []string      `json:"suggestions"`
}

// IndexManager handles automatic index creation and optimization
type IndexManager struct {
	db     *gorm.DB
	logger *zap.Logger
	config IndexConfig
}

// IndexConfig contains index optimization configuration
type IndexConfig struct {
	AutoCreateIndexes    bool     `json:"auto_create_indexes"`
	MinUsageThreshold    int64    `json:"min_usage_threshold"`
	AnalyzeUsageInterval time.Duration `json:"analyze_usage_interval"`
	IndexSuggestions     []IndexSuggestion `json:"index_suggestions"`
}

// IndexSuggestion represents a suggested index
type IndexSuggestion struct {
	Table       string   `json:"table"`
	Columns     []string `json:"columns"`
	Type        string   `json:"type"` // btree, gin, gist, etc.
	Condition   string   `json:"condition,omitempty"`
	Priority    int      `json:"priority"`
	Reason      string   `json:"reason"`
}

// QueryStats contains performance statistics for queries
type QueryStats struct {
	TotalQueries      int64         `json:"total_queries"`
	CacheHitRate      float64       `json:"cache_hit_rate"`
	AverageQueryTime  time.Duration `json:"average_query_time"`
	SlowQueryCount    int64         `json:"slow_query_count"`
	OptimizedQueries  int64         `json:"optimized_queries"`
	ActiveConnections int           `json:"active_connections"`
	IdleConnections   int           `json:"idle_connections"`
}

// NewQueryOptimizer creates a new query optimizer
func NewQueryOptimizer(db *gorm.DB, redisClient *redis.Client, logger *zap.Logger, config OptimizerConfig) *QueryOptimizer {
	// Set defaults
	if config.CacheTTL == 0 {
		config.CacheTTL = 5 * time.Minute
	}
	if config.SlowQueryThreshold == 0 {
		config.SlowQueryThreshold = 100 * time.Millisecond
	}
	if config.MaxCacheSize == 0 {
		config.MaxCacheSize = 1000
	}
	if config.AnalyzeInterval == 0 {
		config.AnalyzeInterval = 1 * time.Hour
	}
	if config.VacuumInterval == 0 {
		config.VacuumInterval = 4 * time.Hour
	}

	optimizer := &QueryOptimizer{
		db:          db,
		redisClient: redisClient,
		logger:      logger,
		config:      config,
		queryCache: &QueryCache{
			redis:   redisClient,
			ttl:     config.CacheTTL,
			maxSize: config.MaxCacheSize,
		},
		slowQueries: &SlowQueryTracker{
			threshold: config.SlowQueryThreshold,
			queries:   make(map[string]*SlowQueryInfo),
		},
		indexManager: &IndexManager{
			db:     db,
			logger: logger,
			config: IndexConfig{
				AutoCreateIndexes:    config.EnableIndexOptimizer,
				MinUsageThreshold:    100,
				AnalyzeUsageInterval: config.AnalyzeInterval,
			},
		},
	}

	// Configure GORM with custom logger that tracks query performance
	if db.Config.Logger == nil {
		db.Config.Logger = optimizer.newPerformanceLogger()
	}

	return optimizer
}

// Start begins the optimization processes
func (o *QueryOptimizer) Start(ctx context.Context) error {
	o.logger.Info("Starting query optimizer")

	// Optimize connection pool
	if err := o.optimizeConnectionPool(); err != nil {
		return fmt.Errorf("failed to optimize connection pool: %w", err)
	}

	// Start background optimization tasks
	go o.runPeriodicOptimization(ctx)
	go o.runDatabaseMaintenance(ctx)
	
	if o.config.EnableIndexOptimizer {
		go o.runIndexOptimization(ctx)
	}

	return nil
}

// ExecuteWithCache executes a query with caching support
func (o *QueryOptimizer) ExecuteWithCache(ctx context.Context, cacheKey string, queryFn func() (interface{}, error)) (interface{}, error) {
	if !o.config.CacheEnabled {
		return queryFn()
	}

	// Try cache first
	if result, err := o.queryCache.Get(ctx, cacheKey); err == nil && result != nil {
		return result, nil
	}

	// Execute query
	startTime := time.Now()
	result, err := queryFn()
	duration := time.Since(startTime)

	// Track query performance
	o.trackQueryPerformance("cached_query", duration)

	if err != nil {
		return nil, err
	}

	// Cache result
	if err := o.queryCache.Set(ctx, cacheKey, result); err != nil {
		o.logger.Warn("Failed to cache query result", zap.Error(err))
	}

	return result, nil
}

// OptimizeQuery analyzes and optimizes a specific query
func (o *QueryOptimizer) OptimizeQuery(ctx context.Context, query string) (*QueryOptimization, error) {
	optimization := &QueryOptimization{
		OriginalQuery: query,
		Suggestions:   []string{},
	}

	// Analyze query plan
	if o.config.EnableQueryPlan {
		plan, err := o.analyzeQueryPlan(ctx, query)
		if err != nil {
			o.logger.Warn("Failed to analyze query plan", zap.Error(err))
		} else {
			optimization.ExecutionPlan = plan
			optimization.Suggestions = append(optimization.Suggestions, o.generateQuerySuggestions(plan)...)
		}
	}

	// Check for missing indexes
	indexSuggestions := o.suggestIndexes(query)
	for _, suggestion := range indexSuggestions {
		optimization.Suggestions = append(optimization.Suggestions, 
			fmt.Sprintf("Consider creating index: %s", suggestion.Reason))
	}

	return optimization, nil
}

// GetQueryStats returns current query performance statistics
func (o *QueryOptimizer) GetQueryStats() *QueryStats {
	o.mu.RLock()
	defer o.mu.RUnlock()

	sqlDB, _ := o.db.DB()
	dbStats := sqlDB.Stats()

	return &QueryStats{
		ActiveConnections: dbStats.OpenConnections,
		IdleConnections:   dbStats.Idle,
		CacheHitRate:     o.queryCache.getHitRate(),
	}
}

// GetSlowQueries returns current slow query information
func (o *QueryOptimizer) GetSlowQueries() []*SlowQueryInfo {
	o.slowQueries.mu.RLock()
	defer o.slowQueries.mu.RUnlock()

	queries := make([]*SlowQueryInfo, 0, len(o.slowQueries.queries))
	for _, query := range o.slowQueries.queries {
		queries = append(queries, query)
	}

	return queries
}

// Private methods

func (o *QueryOptimizer) optimizeConnectionPool() error {
	sqlDB, err := o.db.DB()
	if err != nil {
		return err
	}

	// Optimize connection pool settings
	sqlDB.SetMaxOpenConns(o.config.ConnectionPoolSize)
	sqlDB.SetMaxIdleConns(o.config.MaxIdleConnections)
	sqlDB.SetConnMaxLifetime(o.config.ConnectionMaxLifetime)

	o.logger.Info("Optimized connection pool settings",
		zap.Int("max_open_conns", o.config.ConnectionPoolSize),
		zap.Int("max_idle_conns", o.config.MaxIdleConnections),
		zap.Duration("conn_max_lifetime", o.config.ConnectionMaxLifetime),
	)

	return nil
}

func (o *QueryOptimizer) runPeriodicOptimization(ctx context.Context) {
	ticker := time.NewTicker(o.config.AnalyzeInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			o.performOptimizationCycle(ctx)
		}
	}
}

func (o *QueryOptimizer) runDatabaseMaintenance(ctx context.Context) {
	ticker := time.NewTicker(o.config.VacuumInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			o.performMaintenance(ctx)
		}
	}
}

func (o *QueryOptimizer) runIndexOptimization(ctx context.Context) {
	ticker := time.NewTicker(o.indexManager.config.AnalyzeUsageInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			o.indexManager.analyzeAndOptimize(ctx)
		}
	}
}

func (o *QueryOptimizer) performOptimizationCycle(ctx context.Context) {
	o.logger.Info("Performing optimization cycle")

	// Analyze table statistics
	if err := o.updateTableStatistics(ctx); err != nil {
		o.logger.Error("Failed to update table statistics", zap.Error(err))
	}

	// Clean up cache
	o.queryCache.cleanup()

	// Reset slow query tracking
	o.slowQueries.reset()
}

func (o *QueryOptimizer) performMaintenance(ctx context.Context) {
	o.logger.Info("Performing database maintenance")

	// Run VACUUM ANALYZE on key tables
	tables := []string{"orders", "trades", "order_book_snapshots"}
	for _, table := range tables {
		if err := o.vacuumTable(ctx, table); err != nil {
			o.logger.Error("Failed to vacuum table", zap.String("table", table), zap.Error(err))
		}
	}
}

func (o *QueryOptimizer) newPerformanceLogger() logger.Interface {
	return &performanceLogger{
		optimizer: o,
		Logger:    logger.Default,
	}
}

func (o *QueryOptimizer) trackQueryPerformance(query string, duration time.Duration) {
	if duration > o.slowQueries.threshold {
		o.slowQueries.recordSlowQuery(query, duration)
	}
}

func (o *QueryOptimizer) analyzeQueryPlan(ctx context.Context, query string) (*QueryPlan, error) {
	explainQuery := fmt.Sprintf("EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) %s", query)
	
	var result []map[string]interface{}
	if err := o.db.WithContext(ctx).Raw(explainQuery).Scan(&result).Error; err != nil {
		return nil, err
	}

	// Parse the execution plan
	plan := &QueryPlan{
		Query:           query,
		EstimatedCost:   0,
		ActualTime:      0,
		BuffersHit:      0,
		BuffersRead:     0,
		IndexScans:      0,
		SequentialScans: 0,
	}

	// Extract relevant metrics from the plan
	// This is a simplified version - in production, you'd parse the full JSON
	if len(result) > 0 {
		if planData, ok := result[0]["QUERY PLAN"].([]interface{}); ok && len(planData) > 0 {
			if planObj, ok := planData[0].(map[string]interface{}); ok {
				if cost, ok := planObj["Total Cost"].(float64); ok {
					plan.EstimatedCost = cost
				}
				if time, ok := planObj["Actual Total Time"].(float64); ok {
					plan.ActualTime = time
				}
			}
		}
	}

	return plan, nil
}

func (o *QueryOptimizer) generateQuerySuggestions(plan *QueryPlan) []string {
	suggestions := []string{}

	if plan.SequentialScans > 0 {
		suggestions = append(suggestions, "Query contains sequential scans - consider adding indexes")
	}

	if plan.EstimatedCost > 1000 {
		suggestions = append(suggestions, "High cost query - consider optimization")
	}

	if plan.BuffersRead > plan.BuffersHit {
		suggestions = append(suggestions, "High disk I/O - consider increasing shared_buffers")
	}

	return suggestions
}

func (o *QueryOptimizer) suggestIndexes(query string) []IndexSuggestion {
	suggestions := []IndexSuggestion{}

	// Simple heuristics for index suggestions
	lowerQuery := strings.ToLower(query)

	if strings.Contains(lowerQuery, "where") && strings.Contains(lowerQuery, "orders") {
		if strings.Contains(lowerQuery, "user_id") {
			suggestions = append(suggestions, IndexSuggestion{
				Table:    "orders",
				Columns:  []string{"user_id", "created_at"},
				Type:     "btree",
				Priority: 8,
				Reason:   "Frequent user_id lookups with time-based filtering",
			})
		}
		if strings.Contains(lowerQuery, "symbol") && strings.Contains(lowerQuery, "status") {
			suggestions = append(suggestions, IndexSuggestion{
				Table:    "orders",
				Columns:  []string{"symbol", "status", "created_at"},
				Type:     "btree",
				Priority: 9,
				Reason:   "Trading pair and status queries with time ordering",
			})
		}
	}

	return suggestions
}

func (o *QueryOptimizer) updateTableStatistics(ctx context.Context) error {
	// Update table statistics for the query planner
	return o.db.WithContext(ctx).Exec("ANALYZE").Error
}

func (o *QueryOptimizer) vacuumTable(ctx context.Context, tableName string) error {
	query := fmt.Sprintf("VACUUM (ANALYZE, VERBOSE) %s", tableName)
	return o.db.WithContext(ctx).Exec(query).Error
}

// QueryOptimization contains optimization results
type QueryOptimization struct {
	OriginalQuery   string      `json:"original_query"`
	OptimizedQuery  string      `json:"optimized_query,omitempty"`
	ExecutionPlan   *QueryPlan  `json:"execution_plan,omitempty"`
	Suggestions     []string    `json:"suggestions"`
	EstimatedImprovement float64 `json:"estimated_improvement"`
}

// QueryPlan contains query execution plan information
type QueryPlan struct {
	Query           string  `json:"query"`
	EstimatedCost   float64 `json:"estimated_cost"`
	ActualTime      float64 `json:"actual_time"`
	BuffersHit      int64   `json:"buffers_hit"`
	BuffersRead     int64   `json:"buffers_read"`
	IndexScans      int     `json:"index_scans"`
	SequentialScans int     `json:"sequential_scans"`
}

// performanceLogger wraps GORM's logger to track query performance
type performanceLogger struct {
	optimizer *QueryOptimizer
	logger.Interface
}

func (p *performanceLogger) Trace(ctx context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	sql, rowsAffected := fc()
	duration := time.Since(begin)
	
	// Track query performance
	p.optimizer.trackQueryPerformance(sql, duration)
	
	// Call original logger
	p.Interface.Trace(ctx, begin, func() (string, int64) {
		return sql, rowsAffected
	}, err)
}

// Cache operations

func (c *QueryCache) Get(ctx context.Context, key string) (interface{}, error) {
	return c.redis.Get(ctx, "query:"+key).Result()
}

func (c *QueryCache) Set(ctx context.Context, key string, value interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	if c.currentSize >= c.maxSize {
		// Evict oldest entries
		c.evictOldest(ctx)
	}
	
	c.currentSize++
	return c.redis.Set(ctx, "query:"+key, value, c.ttl).Err()
}

func (c *QueryCache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.currentSize = 0 // Reset counter - Redis TTL will handle actual cleanup
}

func (c *QueryCache) getHitRate() float64 {
	// Simplified hit rate calculation
	return 0.85 // Placeholder - implement actual hit rate tracking
}

func (c *QueryCache) evictOldest(ctx context.Context) {
	// Implement LRU eviction logic
	// This is a simplified version
	keys, err := c.redis.Keys(ctx, "query:*").Result()
	if err != nil {
		return
	}
	
	if len(keys) > 0 {
		c.redis.Del(ctx, keys[0])
		c.currentSize--
	}
}

// Slow query tracking

func (s *SlowQueryTracker) recordSlowQuery(query string, duration time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	queryKey := s.normalizeQuery(query)
	
	if info, exists := s.queries[queryKey]; exists {
		info.Count++
		info.TotalTime += duration
		info.AverageTime = time.Duration(int64(info.TotalTime) / info.Count)
		info.LastSeen = time.Now()
	} else {
		s.queries[queryKey] = &SlowQueryInfo{
			Query:       query,
			Count:       1,
			TotalTime:   duration,
			AverageTime: duration,
			LastSeen:    time.Now(),
			Suggestions: []string{},
		}
	}
}

func (s *SlowQueryTracker) normalizeQuery(query string) string {
	// Remove parameters and normalize whitespace
	return strings.TrimSpace(strings.ToLower(query))
}

func (s *SlowQueryTracker) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.queries = make(map[string]*SlowQueryInfo)
}

// Index management

func (im *IndexManager) analyzeAndOptimize(ctx context.Context) {
	im.logger.Info("Analyzing index usage and optimizing")

	// Get index usage statistics
	indexStats, err := im.getIndexUsageStats(ctx)
	if err != nil {
		im.logger.Error("Failed to get index usage stats", zap.Error(err))
		return
	}

	// Identify unused indexes
	unusedIndexes := im.findUnusedIndexes(indexStats)
	for _, index := range unusedIndexes {
		im.logger.Warn("Found unused index", zap.String("index", index))
		// Optionally drop unused indexes (be careful in production)
	}

	// Suggest new indexes based on query patterns
	suggestions := im.generateIndexSuggestions()
	for _, suggestion := range suggestions {
		if im.config.AutoCreateIndexes && suggestion.Priority >= 8 {
			if err := im.createIndex(ctx, suggestion); err != nil {
				im.logger.Error("Failed to create suggested index", zap.Error(err))
			} else {
				im.logger.Info("Created suggested index", 
					zap.String("table", suggestion.Table),
					zap.Strings("columns", suggestion.Columns))
			}
		}
	}
}

func (im *IndexManager) getIndexUsageStats(ctx context.Context) (map[string]int64, error) {
	query := `
		SELECT indexrelname, idx_scan 
		FROM pg_stat_user_indexes 
		WHERE schemaname = 'public'
	`
	
	rows, err := im.db.WithContext(ctx).Raw(query).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stats := make(map[string]int64)
	for rows.Next() {
		var indexName string
		var scanCount int64
		if err := rows.Scan(&indexName, &scanCount); err != nil {
			continue
		}
		stats[indexName] = scanCount
	}

	return stats, nil
}

func (im *IndexManager) findUnusedIndexes(stats map[string]int64) []string {
	unused := []string{}
	for indexName, scanCount := range stats {
		if scanCount < im.config.MinUsageThreshold {
			unused = append(unused, indexName)
		}
	}
	return unused
}

func (im *IndexManager) generateIndexSuggestions() []IndexSuggestion {
	// Return predefined suggestions for common trading patterns
	return []IndexSuggestion{
		{
			Table:    "orders",
			Columns:  []string{"user_id", "status", "created_at"},
			Type:     "btree",
			Priority: 9,
			Reason:   "User order lookups with status filtering",
		},
		{
			Table:    "orders",
			Columns:  []string{"symbol", "side", "price"},
			Type:     "btree",
			Priority: 8,
			Reason:   "Order book reconstruction",
		},
		{
			Table:    "trades",
			Columns:  []string{"symbol", "created_at"},
			Type:     "btree",
			Priority: 7,
			Reason:   "Trade history queries",
		},
		{
			Table:    "orders",
			Columns:  []string{"created_at"},
			Type:     "brin",
			Priority: 6,
			Reason:   "Time-series queries on large datasets",
		},
	}
}

func (im *IndexManager) createIndex(ctx context.Context, suggestion IndexSuggestion) error {
	indexName := fmt.Sprintf("idx_%s_%s", suggestion.Table, strings.Join(suggestion.Columns, "_"))
	
	var query string
	if suggestion.Condition != "" {
		query = fmt.Sprintf("CREATE INDEX CONCURRENTLY IF NOT EXISTS %s ON %s USING %s (%s) WHERE %s",
			indexName, suggestion.Table, suggestion.Type, 
			strings.Join(suggestion.Columns, ", "), suggestion.Condition)
	} else {
		query = fmt.Sprintf("CREATE INDEX CONCURRENTLY IF NOT EXISTS %s ON %s USING %s (%s)",
			indexName, suggestion.Table, suggestion.Type, strings.Join(suggestion.Columns, ", "))
	}

	return im.db.WithContext(ctx).Exec(query).Error
}

// DefaultOptimizerConfig returns default optimizer configuration
func DefaultOptimizerConfig() OptimizerConfig {
	return OptimizerConfig{
		CacheEnabled:          true,
		CacheTTL:              5 * time.Minute,
		SlowQueryThreshold:    100 * time.Millisecond,
		MaxCacheSize:          1000,
		EnableQueryPlan:       true,
		EnableIndexOptimizer:  true,
		AnalyzeInterval:       1 * time.Hour,
		VacuumInterval:        4 * time.Hour,
		ConnectionPoolSize:    100,
		MaxIdleConnections:    20,
		ConnectionMaxLifetime: 1 * time.Hour,
	}
}
