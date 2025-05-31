package database

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// MonitoringDashboard provides comprehensive database performance monitoring
type MonitoringDashboard struct {
	db           *ReadWriteDB
	optimizer    *QueryOptimizer
	router       *QueryRouter
	indexManager *IndexManager
	redis        *redis.Client
	logger       *zap.Logger
	config       *MonitoringConfig
	metrics      *DashboardMetrics
	alerts       *AlertManager
	mu           sync.RWMutex
}

// MonitoringConfig holds configuration for monitoring dashboard
type MonitoringConfig struct {
	CollectionInterval   time.Duration `yaml:"collection_interval" json:"collection_interval"`
	RetentionPeriod      time.Duration `yaml:"retention_period" json:"retention_period"`
	SlowQueryThreshold   time.Duration `yaml:"slow_query_threshold" json:"slow_query_threshold"`
	HighLatencyThreshold time.Duration `yaml:"high_latency_threshold" json:"high_latency_threshold"`
	LowCacheHitThreshold float64       `yaml:"low_cache_hit_threshold" json:"low_cache_hit_threshold"`
	AlertsEnabled        bool          `yaml:"alerts_enabled" json:"alerts_enabled"`
	MetricsPrefix        string        `yaml:"metrics_prefix" json:"metrics_prefix"`
}

// DefaultMonitoringConfig returns default monitoring configuration
func DefaultMonitoringConfig() *MonitoringConfig {
	return &MonitoringConfig{
		CollectionInterval:   30 * time.Second,
		RetentionPeriod:      24 * time.Hour,
		SlowQueryThreshold:   100 * time.Millisecond,
		HighLatencyThreshold: 50 * time.Millisecond,
		LowCacheHitThreshold: 0.8, // 80%
		AlertsEnabled:        true,
		MetricsPrefix:        "pincex_db",
	}
}

// DashboardMetrics holds all dashboard metrics
type DashboardMetrics struct {
	DatabaseMetrics    *DatabaseMetrics    `json:"database_metrics"`
	QueryMetrics       *QueryMetrics       `json:"query_metrics"`
	CacheMetrics       *CacheMetrics       `json:"cache_metrics"`
	ConnectionMetrics  *ConnectionMetrics  `json:"connection_metrics"`
	IndexMetrics       *IndexMetrics       `json:"index_metrics"`
	ReplicationMetrics *ReplicationMetrics `json:"replication_metrics"`
	AlertMetrics       *AlertMetrics       `json:"alert_metrics"`
	Timestamp          time.Time           `json:"timestamp"`
}

// DatabaseMetrics holds core database performance metrics
type DatabaseMetrics struct {
	TotalConnections  int64                 `json:"total_connections"`
	ActiveConnections int64                 `json:"active_connections"`
	IdleConnections   int64                 `json:"idle_connections"`
	TotalQueries      int64                 `json:"total_queries"`
	QueriesPerSecond  float64               `json:"queries_per_second"`
	AverageQueryTime  float64               `json:"average_query_time_ms"`
	DatabaseSize      int64                 `json:"database_size_bytes"`
	TableSizes        map[string]int64      `json:"table_sizes_bytes"`
	TupleStats        map[string]TupleStats `json:"tuple_stats"`
}

// QueryMetrics holds query performance metrics
type QueryMetrics struct {
	SlowQueries        int64            `json:"slow_queries"`
	FastestQuery       float64          `json:"fastest_query_ms"`
	SlowestQuery       float64          `json:"slowest_query_ms"`
	QueryTypeBreakdown map[string]int64 `json:"query_type_breakdown"`
	TopSlowQueries     []SlowQuery      `json:"top_slow_queries"`
	QueryLatencyP50    float64          `json:"query_latency_p50_ms"`
	QueryLatencyP95    float64          `json:"query_latency_p95_ms"`
	QueryLatencyP99    float64          `json:"query_latency_p99_ms"`
}

// CacheMetrics holds cache performance metrics
type CacheMetrics struct {
	HitRate           float64          `json:"hit_rate"`
	MissRate          float64          `json:"miss_rate"`
	TotalKeys         int64            `json:"total_keys"`
	MemoryUsage       int64            `json:"memory_usage_bytes"`
	EvictedKeys       int64            `json:"evicted_keys"`
	ExpiredKeys       int64            `json:"expired_keys"`
	CacheOperations   map[string]int64 `json:"cache_operations"`
	AverageGetLatency float64          `json:"average_get_latency_ms"`
	AverageSetLatency float64          `json:"average_set_latency_ms"`
}

// ConnectionMetrics holds connection pool metrics
type ConnectionMetrics struct {
	MasterConnections  ConnectionPoolStats            `json:"master_connections"`
	ReplicaConnections map[string]ConnectionPoolStats `json:"replica_connections"`
	FailoverEvents     int64                          `json:"failover_events"`
	ConnectionErrors   int64                          `json:"connection_errors"`
	PoolUtilization    float64                        `json:"pool_utilization"`
}

// IndexMetrics holds index performance metrics
type IndexMetrics struct {
	TotalIndexes        int64            `json:"total_indexes"`
	UnusedIndexes       int64            `json:"unused_indexes"`
	IndexSizeTotal      int64            `json:"index_size_total_bytes"`
	IndexUsageStats     []IndexUsageStat `json:"index_usage_stats"`
	IndexEfficiency     float64          `json:"index_efficiency"`
	RecommendationCount int64            `json:"recommendation_count"`
}

// ReplicationMetrics holds replication lag and status metrics
type ReplicationMetrics struct {
	ReplicaLag        map[string]time.Duration `json:"replica_lag"`
	ReplicationStatus map[string]string        `json:"replication_status"`
	ReadWriteRatio    float64                  `json:"read_write_ratio"`
	ReplicaHealth     map[string]bool          `json:"replica_health"`
}

// AlertMetrics holds alerting statistics
type AlertMetrics struct {
	ActiveAlerts   int64            `json:"active_alerts"`
	TotalAlerts    int64            `json:"total_alerts"`
	AlertsByType   map[string]int64 `json:"alerts_by_type"`
	LastAlertTime  time.Time        `json:"last_alert_time"`
	ResolvedAlerts int64            `json:"resolved_alerts"`
}

// TupleStats holds table statistics
type TupleStats struct {
	Live    int64 `json:"live"`
	Dead    int64 `json:"dead"`
	Inserts int64 `json:"inserts"`
	Updates int64 `json:"updates"`
	Deletes int64 `json:"deletes"`
}

// SlowQuery represents a slow query entry
type SlowQuery struct {
	Query     string    `json:"query"`
	Duration  float64   `json:"duration_ms"`
	Frequency int64     `json:"frequency"`
	LastSeen  time.Time `json:"last_seen"`
	Table     string    `json:"table"`
	Operation string    `json:"operation"`
}

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

// NewMonitoringDashboard creates a new monitoring dashboard
func NewMonitoringDashboard(
	db *ReadWriteDB,
	optimizer *QueryOptimizer,
	router *QueryRouter,
	indexManager *IndexManager,
	redis *redis.Client,
	logger *zap.Logger,
	config *MonitoringConfig,
) *MonitoringDashboard {
	if config == nil {
		config = DefaultMonitoringConfig()
	}

	dashboard := &MonitoringDashboard{
		db:           db,
		optimizer:    optimizer,
		router:       router,
		indexManager: indexManager,
		redis:        redis,
		logger:       logger,
		config:       config,
		metrics:      &DashboardMetrics{},
		alerts:       NewAlertManager(logger),
	}

	// Start background metric collection
	go dashboard.startMetricCollection()

	return dashboard
}

// startMetricCollection starts the background metric collection
func (md *MonitoringDashboard) startMetricCollection() {
	ticker := time.NewTicker(md.config.CollectionInterval)
	defer ticker.Stop()

	for range ticker.C {
		ctx := context.Background()
		if err := md.collectMetrics(ctx); err != nil {
			md.logger.Error("Failed to collect metrics", zap.Error(err))
		}
	}
}

// collectMetrics collects all metrics
func (md *MonitoringDashboard) collectMetrics(ctx context.Context) error {
	md.mu.Lock()
	defer md.mu.Unlock()

	metrics := &DashboardMetrics{
		Timestamp: time.Now(),
	}

	// Collect database metrics
	dbMetrics, err := md.collectDatabaseMetrics(ctx)
	if err != nil {
		md.logger.Error("Failed to collect database metrics", zap.Error(err))
	} else {
		metrics.DatabaseMetrics = dbMetrics
	}

	// Collect query metrics
	queryMetrics, err := md.collectQueryMetrics(ctx)
	if err != nil {
		md.logger.Error("Failed to collect query metrics", zap.Error(err))
	} else {
		metrics.QueryMetrics = queryMetrics
	}

	// Collect cache metrics
	cacheMetrics, err := md.collectCacheMetrics(ctx)
	if err != nil {
		md.logger.Error("Failed to collect cache metrics", zap.Error(err))
	} else {
		metrics.CacheMetrics = cacheMetrics
	}

	// Collect connection metrics
	connectionMetrics, err := md.collectConnectionMetrics(ctx)
	if err != nil {
		md.logger.Error("Failed to collect connection metrics", zap.Error(err))
	} else {
		metrics.ConnectionMetrics = connectionMetrics
	}

	// Collect index metrics
	indexMetrics, err := md.collectIndexMetrics(ctx)
	if err != nil {
		md.logger.Error("Failed to collect index metrics", zap.Error(err))
	} else {
		metrics.IndexMetrics = indexMetrics
	}

	// Collect replication metrics
	replicationMetrics, err := md.collectReplicationMetrics(ctx)
	if err != nil {
		md.logger.Error("Failed to collect replication metrics", zap.Error(err))
	} else {
		metrics.ReplicationMetrics = replicationMetrics
	}

	// Collect alert metrics
	metrics.AlertMetrics = md.alerts.GetMetrics()

	md.metrics = metrics

	// Store metrics in Redis for historical tracking
	md.storeMetricsInRedis(ctx, metrics)

	// Check for alerts
	if md.config.AlertsEnabled {
		md.checkAlerts(metrics)
	}

	return nil
}

// collectDatabaseMetrics collects core database metrics
func (md *MonitoringDashboard) collectDatabaseMetrics(ctx context.Context) (*DatabaseMetrics, error) {
	metrics := &DatabaseMetrics{
		TableSizes: make(map[string]int64),
		TupleStats: make(map[string]TupleStats),
	}

	// Get connection stats
	sqlDB, err := md.db.Writer().DB()
	if err == nil {
		stats := sqlDB.Stats()
		metrics.TotalConnections = int64(stats.MaxOpenConnections)
		metrics.ActiveConnections = int64(stats.InUse)
		metrics.IdleConnections = int64(stats.Idle)
	}

	// Get database size
	var dbSize int64
	err = md.db.Reader().WithContext(ctx).Raw(`
		SELECT pg_database_size(current_database())
	`).Scan(&dbSize).Error
	if err == nil {
		metrics.DatabaseSize = dbSize
	}

	// Get table sizes
	var tableSizes []struct {
		TableName string `json:"table_name"`
		Size      int64  `json:"size"`
	}
	err = md.db.Reader().WithContext(ctx).Raw(`
		SELECT 
			tablename as table_name,
			pg_total_relation_size(schemaname||'.'||tablename) as size
		FROM pg_tables 
		WHERE schemaname = 'public'
		  AND tablename IN ('orders', 'trades')
	`).Scan(&tableSizes).Error
	if err == nil {
		for _, ts := range tableSizes {
			metrics.TableSizes[ts.TableName] = ts.Size
		}
	}

	// Get tuple statistics
	var tupleStats []struct {
		TableName string `json:"table_name"`
		Live      int64  `json:"n_tup_ins"`
		Dead      int64  `json:"n_tup_upd"`
		Inserts   int64  `json:"n_tup_del"`
		Updates   int64  `json:"n_live_tup"`
		Deletes   int64  `json:"n_dead_tup"`
	}
	err = md.db.Reader().WithContext(ctx).Raw(`
		SELECT 
			relname as table_name,
			n_tup_ins as inserts,
			n_tup_upd as updates,
			n_tup_del as deletes,
			n_live_tup as live,
			n_dead_tup as dead
		FROM pg_stat_user_tables
		WHERE relname IN ('orders', 'trades')
	`).Scan(&tupleStats).Error
	if err == nil {
		for _, ts := range tupleStats {
			metrics.TupleStats[ts.TableName] = TupleStats{
				Live:    ts.Live,
				Dead:    ts.Dead,
				Inserts: ts.Inserts,
				Updates: ts.Updates,
				Deletes: ts.Deletes,
			}
		}
	}

	return metrics, nil
}

// collectQueryMetrics collects query performance metrics
func (md *MonitoringDashboard) collectQueryMetrics(ctx context.Context) (*QueryMetrics, error) {
	metrics := &QueryMetrics{
		QueryTypeBreakdown: make(map[string]int64),
	}

	// Get optimizer metrics if available
	if md.optimizer != nil {
		optimizerMetrics := md.optimizer.GetMetrics()
		metrics.SlowQueries = optimizerMetrics.SlowQueries
		metrics.QueryLatencyP50 = optimizerMetrics.AvgQueryTime

		// Get slow queries
		slowQueries := md.optimizer.GetSlowQueries(10)
		metrics.TopSlowQueries = make([]SlowQuery, len(slowQueries))
		for i, sq := range slowQueries {
			metrics.TopSlowQueries[i] = SlowQuery{
				Query:     sq.Query,
				Duration:  float64(sq.Duration.Nanoseconds()) / 1e6, // Convert to ms
				Frequency: sq.Count,
				LastSeen:  sq.LastSeen,
			}
		}
	}

	// Get database query statistics
	var queryStats []struct {
		Query  string  `json:"query"`
		Calls  int64   `json:"calls"`
		Mean   float64 `json:"mean_exec_time"`
		Stddev float64 `json:"stddev_exec_time"`
	}

	// This requires pg_stat_statements extension
	err := md.db.Reader().WithContext(ctx).Raw(`
		SELECT 
			query,
			calls,
			mean_exec_time,
			stddev_exec_time
		FROM pg_stat_statements 
		WHERE query LIKE '%orders%' OR query LIKE '%trades%'
		ORDER BY mean_exec_time DESC
		LIMIT 10
	`).Scan(&queryStats).Error

	if err == nil && len(queryStats) > 0 {
		metrics.SlowestQuery = queryStats[0].Mean
		if len(queryStats) > 1 {
			metrics.FastestQuery = queryStats[len(queryStats)-1].Mean
		}
	}

	return metrics, nil
}

// collectCacheMetrics collects cache performance metrics
func (md *MonitoringDashboard) collectCacheMetrics(ctx context.Context) (*CacheMetrics, error) {
	metrics := &CacheMetrics{
		CacheOperations: make(map[string]int64),
	}

	// Get Redis info
	info := md.redis.Info(ctx, "stats", "memory", "keyspace").Val()

	// Parse Redis info (simplified parsing)
	if info != "" {
		// This would need proper parsing of Redis INFO output
		// For now, we'll use basic commands

		// Get keyspace info
		dbInfo := md.redis.Info(ctx, "keyspace").Val()
		if dbInfo != "" {
			// Parse keyspace info to get key count
			// Simplified implementation
		}

		// Get memory usage
		memoryInfo := md.redis.Info(ctx, "memory").Val()
		if memoryInfo != "" {
			// Parse memory info
			// Simplified implementation
		}
	}

	// Calculate hit rate if we have the data
	if md.optimizer != nil {
		cacheStats := md.optimizer.GetCacheStats()
		if hits, ok := cacheStats["hits"].(int64); ok {
			if misses, ok := cacheStats["misses"].(int64); ok {
				total := hits + misses
				if total > 0 {
					metrics.HitRate = float64(hits) / float64(total)
					metrics.MissRate = float64(misses) / float64(total)
				}
			}
		}
	}

	return metrics, nil
}

// collectConnectionMetrics collects connection pool metrics
func (md *MonitoringDashboard) collectConnectionMetrics(ctx context.Context) (*ConnectionMetrics, error) {
	metrics := &ConnectionMetrics{
		ReplicaConnections: make(map[string]ConnectionPoolStats),
	}

	// Get master connection stats
	if sqlDB, err := md.db.Writer().DB(); err == nil {
		stats := sqlDB.Stats()
		metrics.MasterConnections = ConnectionPoolStats{
			Open:         int32(stats.OpenConnections),
			InUse:        int32(stats.InUse),
			Idle:         int32(stats.Idle),
			WaitCount:    stats.WaitCount,
			WaitDuration: stats.WaitDuration,
			MaxIdleTime:  stats.MaxIdleClosed,
			MaxLifetime:  stats.MaxLifetimeClosed,
		}

		if stats.OpenConnections > 0 {
			metrics.PoolUtilization = float64(stats.InUse) / float64(stats.OpenConnections)
		}
	}

	// Get router metrics if available
	if md.router != nil {
		routerMetrics := md.router.GetMetrics()
		metrics.FailoverEvents = routerMetrics.FailoverCount

		// Get replica status
		replicaStatus := md.router.GetReplicaStatus()
		for _, replica := range replicaStatus {
			// Would need access to replica connection pools
			// Simplified implementation
			metrics.ReplicaConnections[replica.Name] = ConnectionPoolStats{}
		}
	}

	return metrics, nil
}

// collectIndexMetrics collects index performance metrics
func (md *MonitoringDashboard) collectIndexMetrics(ctx context.Context) (*IndexMetrics, error) {
	metrics := &IndexMetrics{}

	if md.indexManager != nil {
		// Get usage stats
		usageStats, err := md.indexManager.GetIndexUsageStats(ctx)
		if err == nil {
			metrics.IndexUsageStats = usageStats
			metrics.TotalIndexes = int64(len(usageStats))
		}

		// Get unused indexes
		unusedIndexes, err := md.indexManager.GetUnusedIndexes(ctx)
		if err == nil {
			metrics.UnusedIndexes = int64(len(unusedIndexes))
		}

		// Get index sizes
		indexSizes, err := md.indexManager.GetIndexSizes(ctx)
		if err == nil {
			var totalSize int64
			for _, size := range indexSizes {
				totalSize += size.SizeBytes
			}
			metrics.IndexSizeTotal = totalSize
		}

		// Calculate efficiency
		if len(usageStats) > 0 {
			var totalScans int64
			for _, stat := range usageStats {
				totalScans += stat.IdxScan
			}
			if totalScans > 0 {
				metrics.IndexEfficiency = float64(totalScans) / float64(len(usageStats))
			}
		}
	}

	return metrics, nil
}

// collectReplicationMetrics collects replication metrics
func (md *MonitoringDashboard) collectReplicationMetrics(ctx context.Context) (*ReplicationMetrics, error) {
	metrics := &ReplicationMetrics{
		ReplicaLag:        make(map[string]time.Duration),
		ReplicationStatus: make(map[string]string),
		ReplicaHealth:     make(map[string]bool),
	}

	if md.router != nil {
		routerMetrics := md.router.GetMetrics()
		total := routerMetrics.MasterQueries + routerMetrics.ReplicaQueries
		if total > 0 {
			metrics.ReadWriteRatio = float64(routerMetrics.ReplicaQueries) / float64(total)
		}

		// Get replica status
		replicaStatus := md.router.GetReplicaStatus()
		for _, replica := range replicaStatus {
			metrics.ReplicaLag[replica.Name] = replica.Latency
			metrics.ReplicaHealth[replica.Name] = replica.Healthy
			if replica.Healthy {
				metrics.ReplicationStatus[replica.Name] = "healthy"
			} else {
				metrics.ReplicationStatus[replica.Name] = "unhealthy"
			}
		}
	}

	return metrics, nil
}

// storeMetricsInRedis stores metrics in Redis for historical tracking
func (md *MonitoringDashboard) storeMetricsInRedis(ctx context.Context, metrics *DashboardMetrics) {
	key := fmt.Sprintf("%s:metrics:%d", md.config.MetricsPrefix, metrics.Timestamp.Unix())

	data, err := json.Marshal(metrics)
	if err != nil {
		md.logger.Error("Failed to marshal metrics", zap.Error(err))
		return
	}

	// Store with TTL
	err = md.redis.Set(ctx, key, data, md.config.RetentionPeriod).Err()
	if err != nil {
		md.logger.Error("Failed to store metrics in Redis", zap.Error(err))
	}
}

// checkAlerts checks metrics against alert thresholds
func (md *MonitoringDashboard) checkAlerts(metrics *DashboardMetrics) {
	// Check slow query threshold
	if metrics.QueryMetrics != nil && metrics.QueryMetrics.SlowQueries > 0 {
		md.alerts.TriggerAlert("slow_queries", fmt.Sprintf("Detected %d slow queries", metrics.QueryMetrics.SlowQueries))
	}

	// Check cache hit rate
	if metrics.CacheMetrics != nil && metrics.CacheMetrics.HitRate < md.config.LowCacheHitThreshold {
		md.alerts.TriggerAlert("low_cache_hit_rate", fmt.Sprintf("Cache hit rate is %.2f%%, below threshold of %.2f%%",
			metrics.CacheMetrics.HitRate*100, md.config.LowCacheHitThreshold*100))
	}

	// Check replica health
	if metrics.ReplicationMetrics != nil {
		for name, healthy := range metrics.ReplicationMetrics.ReplicaHealth {
			if !healthy {
				md.alerts.TriggerAlert("replica_unhealthy", fmt.Sprintf("Replica %s is unhealthy", name))
			}
		}
	}

	// Check high latency
	if metrics.ReplicationMetrics != nil {
		for name, lag := range metrics.ReplicationMetrics.ReplicaLag {
			if lag > md.config.HighLatencyThreshold {
				md.alerts.TriggerAlert("high_replica_latency", fmt.Sprintf("Replica %s has high latency: %v", name, lag))
			}
		}
	}
}

// GetCurrentMetrics returns the current metrics
func (md *MonitoringDashboard) GetCurrentMetrics() *DashboardMetrics {
	md.mu.RLock()
	defer md.mu.RUnlock()
	return md.metrics
}

// GetHistoricalMetrics returns historical metrics from Redis
func (md *MonitoringDashboard) GetHistoricalMetrics(ctx context.Context, duration time.Duration) ([]*DashboardMetrics, error) {
	now := time.Now()
	start := now.Add(-duration)

	pattern := fmt.Sprintf("%s:metrics:*", md.config.MetricsPrefix)
	keys, err := md.redis.Keys(ctx, pattern).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get metrics keys: %w", err)
	}

	var metrics []*DashboardMetrics
	for _, key := range keys {
		data, err := md.redis.Get(ctx, key).Result()
		if err != nil {
			continue
		}

		var metric DashboardMetrics
		if err := json.Unmarshal([]byte(data), &metric); err != nil {
			continue
		}

		if metric.Timestamp.After(start) && metric.Timestamp.Before(now) {
			metrics = append(metrics, &metric)
		}
	}

	return metrics, nil
}

// AlertManager manages system alerts
type AlertManager struct {
	alerts map[string]*Alert
	logger *zap.Logger
	mu     sync.RWMutex
}

// Alert represents a system alert
type Alert struct {
	ID         string     `json:"id"`
	Type       string     `json:"type"`
	Message    string     `json:"message"`
	Severity   string     `json:"severity"`
	Timestamp  time.Time  `json:"timestamp"`
	Resolved   bool       `json:"resolved"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
}

// NewAlertManager creates a new alert manager
func NewAlertManager(logger *zap.Logger) *AlertManager {
	return &AlertManager{
		alerts: make(map[string]*Alert),
		logger: logger,
	}
}

// TriggerAlert triggers a new alert
func (am *AlertManager) TriggerAlert(alertType, message string) {
	am.mu.Lock()
	defer am.mu.Unlock()

	alertID := fmt.Sprintf("%s_%d", alertType, time.Now().Unix())
	alert := &Alert{
		ID:        alertID,
		Type:      alertType,
		Message:   message,
		Severity:  "warning",
		Timestamp: time.Now(),
		Resolved:  false,
	}

	am.alerts[alertID] = alert
	am.logger.Warn("Alert triggered", zap.String("type", alertType), zap.String("message", message))
}

// GetMetrics returns alert metrics
func (am *AlertManager) GetMetrics() *AlertMetrics {
	am.mu.RLock()
	defer am.mu.RUnlock()

	metrics := &AlertMetrics{
		AlertsByType: make(map[string]int64),
	}

	var lastAlertTime time.Time
	for _, alert := range am.alerts {
		metrics.TotalAlerts++
		if !alert.Resolved {
			metrics.ActiveAlerts++
		} else {
			metrics.ResolvedAlerts++
		}

		metrics.AlertsByType[alert.Type]++

		if alert.Timestamp.After(lastAlertTime) {
			lastAlertTime = alert.Timestamp
		}
	}

	metrics.LastAlertTime = lastAlertTime
	return metrics
}
