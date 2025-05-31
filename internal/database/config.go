package database

import (
	"time"
)

// DatabaseConfig holds all database optimization configurations
type DatabaseConfig struct {
	// Connection pool settings
	Master  MasterPoolConfig  `json:"master" yaml:"master"`
	Replica ReplicaPoolConfig `json:"replica" yaml:"replica"`

	// Cache configuration
	Cache CacheConfig `json:"cache" yaml:"cache"`

	// Query optimization settings
	QueryOptimizer QueryOptimizerConfig `json:"query_optimizer" yaml:"query_optimizer"`

	// Monitoring configuration
	Monitoring MonitoringConfig `json:"monitoring" yaml:"monitoring"`

	// Schema optimization settings
	Schema SchemaConfig `json:"schema" yaml:"schema"`

	// Index management settings
	Index IndexConfig `json:"index" yaml:"index"`

	// Query routing configuration
	Routing RoutingConfig `json:"routing" yaml:"routing"`
}

type MasterPoolConfig struct {
	MaxOpenConns    int               `json:"max_open_conns" yaml:"max_open_conns"`
	MaxIdleConns    int               `json:"max_idle_conns" yaml:"max_idle_conns"`
	ConnMaxLifetime time.Duration     `json:"conn_max_lifetime" yaml:"conn_max_lifetime"`
	ConnMaxIdleTime time.Duration     `json:"conn_max_idle_time" yaml:"conn_max_idle_time"`
	ConnTimeout     time.Duration     `json:"conn_timeout" yaml:"conn_timeout"`
	ReadTimeout     time.Duration     `json:"read_timeout" yaml:"read_timeout"`
	WriteTimeout    time.Duration     `json:"write_timeout" yaml:"write_timeout"`
	HealthCheck     HealthCheckConfig `json:"health_check" yaml:"health_check"`
}

type ReplicaPoolConfig struct {
	MaxOpenConns    int               `json:"max_open_conns" yaml:"max_open_conns"`
	MaxIdleConns    int               `json:"max_idle_conns" yaml:"max_idle_conns"`
	ConnMaxLifetime time.Duration     `json:"conn_max_lifetime" yaml:"conn_max_lifetime"`
	ConnMaxIdleTime time.Duration     `json:"conn_max_idle_time" yaml:"conn_max_idle_time"`
	ConnTimeout     time.Duration     `json:"conn_timeout" yaml:"conn_timeout"`
	ReadTimeout     time.Duration     `json:"read_timeout" yaml:"read_timeout"`
	WriteTimeout    time.Duration     `json:"write_timeout" yaml:"write_timeout"`
	HealthCheck     HealthCheckConfig `json:"health_check" yaml:"health_check"`
	Endpoints       []ReplicaEndpoint `json:"endpoints" yaml:"endpoints"`
}

type ReplicaEndpoint struct {
	Host     string `json:"host" yaml:"host"`
	Port     int    `json:"port" yaml:"port"`
	Weight   int    `json:"weight" yaml:"weight"`
	Priority int    `json:"priority" yaml:"priority"`
}

type HealthCheckConfig struct {
	Enabled          bool          `json:"enabled" yaml:"enabled"`
	Interval         time.Duration `json:"interval" yaml:"interval"`
	Timeout          time.Duration `json:"timeout" yaml:"timeout"`
	RetryCount       int           `json:"retry_count" yaml:"retry_count"`
	CustomQuery      string        `json:"custom_query" yaml:"custom_query"`
	FailureThreshold int           `json:"failure_threshold" yaml:"failure_threshold"`
}

type CacheConfig struct {
	Redis    RedisCacheConfig  `json:"redis" yaml:"redis"`
	Memory   MemoryCacheConfig `json:"memory" yaml:"memory"`
	Enabled  bool              `json:"enabled" yaml:"enabled"`
	Strategy string            `json:"strategy" yaml:"strategy"` // "redis", "memory", "hybrid"
}

type RedisCacheConfig struct {
	Addr         string         `json:"addr" yaml:"addr"`
	Password     string         `json:"password" yaml:"password"`
	DB           int            `json:"db" yaml:"db"`
	PoolSize     int            `json:"pool_size" yaml:"pool_size"`
	MinIdleConns int            `json:"min_idle_conns" yaml:"min_idle_conns"`
	DialTimeout  time.Duration  `json:"dial_timeout" yaml:"dial_timeout"`
	ReadTimeout  time.Duration  `json:"read_timeout" yaml:"read_timeout"`
	WriteTimeout time.Duration  `json:"write_timeout" yaml:"write_timeout"`
	TTL          CacheTTLConfig `json:"ttl" yaml:"ttl"`
}

type MemoryCacheConfig struct {
	MaxSize         int           `json:"max_size" yaml:"max_size"`
	TTL             time.Duration `json:"ttl" yaml:"ttl"`
	CleanupInterval time.Duration `json:"cleanup_interval" yaml:"cleanup_interval"`
}

type CacheTTLConfig struct {
	QueryResults time.Duration `json:"query_results" yaml:"query_results"`
	Orders       time.Duration `json:"orders" yaml:"orders"`
	Trades       time.Duration `json:"trades" yaml:"trades"`
	Users        time.Duration `json:"users" yaml:"users"`
	Markets      time.Duration `json:"markets" yaml:"markets"`
	Positions    time.Duration `json:"positions" yaml:"positions"`
}

type QueryOptimizerConfig struct {
	Enabled             bool          `json:"enabled" yaml:"enabled"`
	SlowQueryThreshold  time.Duration `json:"slow_query_threshold" yaml:"slow_query_threshold"`
	CacheSize           int           `json:"cache_size" yaml:"cache_size"`
	LogSlowQueries      bool          `json:"log_slow_queries" yaml:"log_slow_queries"`
	AnalyzeQueryPlans   bool          `json:"analyze_query_plans" yaml:"analyze_query_plans"`
	AutoOptimizeIndexes bool          `json:"auto_optimize_indexes" yaml:"auto_optimize_indexes"`
	MaintenanceInterval time.Duration `json:"maintenance_interval" yaml:"maintenance_interval"`
	VacuumSchedule      string        `json:"vacuum_schedule" yaml:"vacuum_schedule"`   // cron format
	AnalyzeSchedule     string        `json:"analyze_schedule" yaml:"analyze_schedule"` // cron format
}

type MonitoringConfig struct {
	Enabled                  bool          `json:"enabled" yaml:"enabled"`
	MetricsInterval          time.Duration `json:"metrics_interval" yaml:"metrics_interval"`
	HistoryRetention         time.Duration `json:"history_retention" yaml:"history_retention"`
	AlertThresholds          AlertConfig   `json:"alert_thresholds" yaml:"alert_thresholds"`
	SlowQueryLogging         bool          `json:"slow_query_logging" yaml:"slow_query_logging"`
	IndexUsageAnalysis       bool          `json:"index_usage_analysis" yaml:"index_usage_analysis"`
	ReplicationLagMonitoring bool          `json:"replication_lag_monitoring" yaml:"replication_lag_monitoring"`
}

type AlertConfig struct {
	MaxQueryLatency    time.Duration `json:"max_query_latency" yaml:"max_query_latency"`
	MaxConnectionUsage float64       `json:"max_connection_usage" yaml:"max_connection_usage"`
	MinCacheHitRate    float64       `json:"min_cache_hit_rate" yaml:"min_cache_hit_rate"`
	MaxReplicationLag  time.Duration `json:"max_replication_lag" yaml:"max_replication_lag"`
	MaxIndexUnusedDays int           `json:"max_index_unused_days" yaml:"max_index_unused_days"`
	MaxLockWaitTime    time.Duration `json:"max_lock_wait_time" yaml:"max_lock_wait_time"`
}

type SchemaConfig struct {
	Partitioning      PartitioningConfig `json:"partitioning" yaml:"partitioning"`
	Compression       CompressionConfig  `json:"compression" yaml:"compression"`
	AutoVacuum        AutoVacuumConfig   `json:"auto_vacuum" yaml:"auto_vacuum"`
	RetentionPolicies RetentionConfig    `json:"retention_policies" yaml:"retention_policies"`
}

type PartitioningConfig struct {
	Enabled    bool                      `json:"enabled" yaml:"enabled"`
	Strategy   string                    `json:"strategy" yaml:"strategy"` // "time", "hash", "range"
	Tables     map[string]TablePartition `json:"tables" yaml:"tables"`
	AutoCreate bool                      `json:"auto_create" yaml:"auto_create"`
	PreCreate  int                       `json:"pre_create" yaml:"pre_create"` // number of future partitions
}

type TablePartition struct {
	Interval    string `json:"interval" yaml:"interval"` // "daily", "weekly", "monthly"
	Column      string `json:"column" yaml:"column"`
	Retention   string `json:"retention" yaml:"retention"` // "90d", "1y", etc.
	Compression bool   `json:"compression" yaml:"compression"`
}

type CompressionConfig struct {
	Enabled   bool            `json:"enabled" yaml:"enabled"`
	Algorithm string          `json:"algorithm" yaml:"algorithm"` // "lz4", "zstd", "pglz"
	Tables    map[string]bool `json:"tables" yaml:"tables"`
	MinSize   int64           `json:"min_size" yaml:"min_size"` // minimum table size for compression
}

type AutoVacuumConfig struct {
	Enabled            bool    `json:"enabled" yaml:"enabled"`
	VacuumThreshold    int     `json:"vacuum_threshold" yaml:"vacuum_threshold"`
	VacuumScaleFactor  float64 `json:"vacuum_scale_factor" yaml:"vacuum_scale_factor"`
	AnalyzeThreshold   int     `json:"analyze_threshold" yaml:"analyze_threshold"`
	AnalyzeScaleFactor float64 `json:"analyze_scale_factor" yaml:"analyze_scale_factor"`
	VacuumCostLimit    int     `json:"vacuum_cost_limit" yaml:"vacuum_cost_limit"`
	VacuumCostDelay    int     `json:"vacuum_cost_delay" yaml:"vacuum_cost_delay"`
}

type RetentionConfig struct {
	Trades       string `json:"trades" yaml:"trades"`
	Orders       string `json:"orders" yaml:"orders"`
	UserActivity string `json:"user_activity" yaml:"user_activity"`
	AuditLogs    string `json:"audit_logs" yaml:"audit_logs"`
	Metrics      string `json:"metrics" yaml:"metrics"`
}

type IndexConfig struct {
	AutoCreate        bool          `json:"auto_create" yaml:"auto_create"`
	AutoDrop          bool          `json:"auto_drop" yaml:"auto_drop"`
	UsageAnalysis     bool          `json:"usage_analysis" yaml:"usage_analysis"`
	MaintenanceWindow string        `json:"maintenance_window" yaml:"maintenance_window"` // cron format
	UnusedThreshold   int           `json:"unused_threshold" yaml:"unused_threshold"`     // days
	CustomIndexes     []CustomIndex `json:"custom_indexes" yaml:"custom_indexes"`
}

type CustomIndex struct {
	Table   string   `json:"table" yaml:"table"`
	Name    string   `json:"name" yaml:"name"`
	Columns []string `json:"columns" yaml:"columns"`
	Type    string   `json:"type" yaml:"type"` // "btree", "gin", "gist", "brin"
	Unique  bool     `json:"unique" yaml:"unique"`
	Partial string   `json:"partial" yaml:"partial"` // WHERE clause for partial index
	Include []string `json:"include" yaml:"include"` // covering index columns
}

type RoutingConfig struct {
	Enabled           bool          `json:"enabled" yaml:"enabled"`
	Strategy          string        `json:"strategy" yaml:"strategy"`                   // "latency", "weighted", "round_robin"
	ConsistencyLevel  string        `json:"consistency_level" yaml:"consistency_level"` // "eventual", "strong", "bounded"
	MaxReplicationLag time.Duration `json:"max_replication_lag" yaml:"max_replication_lag"`
	ReadPreference    string        `json:"read_preference" yaml:"read_preference"` // "replica", "master", "both"
	CriticalTables    []string      `json:"critical_tables" yaml:"critical_tables"`
	RoutingRules      []RoutingRule `json:"routing_rules" yaml:"routing_rules"`
}

type RoutingRule struct {
	Pattern     string `json:"pattern" yaml:"pattern"`         // SQL pattern to match
	Destination string `json:"destination" yaml:"destination"` // "master", "replica", "both"
	Priority    int    `json:"priority" yaml:"priority"`
}

// DefaultConfig returns a production-ready default configuration
func DefaultConfig() *DatabaseConfig {
	return &DatabaseConfig{
		Master: MasterPoolConfig{
			MaxOpenConns:    50,
			MaxIdleConns:    10,
			ConnMaxLifetime: 30 * time.Minute,
			ConnMaxIdleTime: 5 * time.Minute,
			ConnTimeout:     10 * time.Second,
			ReadTimeout:     30 * time.Second,
			WriteTimeout:    30 * time.Second,
			HealthCheck: HealthCheckConfig{
				Enabled:          true,
				Interval:         30 * time.Second,
				Timeout:          5 * time.Second,
				RetryCount:       3,
				CustomQuery:      "SELECT 1",
				FailureThreshold: 3,
			},
		},
		Replica: ReplicaPoolConfig{
			MaxOpenConns:    100,
			MaxIdleConns:    20,
			ConnMaxLifetime: 30 * time.Minute,
			ConnMaxIdleTime: 5 * time.Minute,
			ConnTimeout:     10 * time.Second,
			ReadTimeout:     30 * time.Second,
			WriteTimeout:    30 * time.Second,
			HealthCheck: HealthCheckConfig{
				Enabled:          true,
				Interval:         30 * time.Second,
				Timeout:          5 * time.Second,
				RetryCount:       3,
				CustomQuery:      "SELECT 1",
				FailureThreshold: 3,
			},
		},
		Cache: CacheConfig{
			Enabled:  true,
			Strategy: "redis",
			Redis: RedisCacheConfig{
				Addr:         "localhost:6379",
				DB:           0,
				PoolSize:     100,
				MinIdleConns: 10,
				DialTimeout:  5 * time.Second,
				ReadTimeout:  3 * time.Second,
				WriteTimeout: 3 * time.Second,
				TTL: CacheTTLConfig{
					QueryResults: 5 * time.Minute,
					Orders:       30 * time.Second,
					Trades:       2 * time.Minute,
					Users:        10 * time.Second,
					Markets:      1 * time.Minute,
					Positions:    15 * time.Second,
				},
			},
			Memory: MemoryCacheConfig{
				MaxSize:         10000,
				TTL:             5 * time.Minute,
				CleanupInterval: 1 * time.Minute,
			},
		},
		QueryOptimizer: QueryOptimizerConfig{
			Enabled:             true,
			SlowQueryThreshold:  100 * time.Millisecond,
			CacheSize:           10000,
			LogSlowQueries:      true,
			AnalyzeQueryPlans:   true,
			AutoOptimizeIndexes: true,
			MaintenanceInterval: 1 * time.Hour,
			VacuumSchedule:      "0 2 * * *", // 2 AM daily
			AnalyzeSchedule:     "0 3 * * *", // 3 AM daily
		},
		Monitoring: MonitoringConfig{
			Enabled:                  true,
			MetricsInterval:          10 * time.Second,
			HistoryRetention:         24 * time.Hour,
			SlowQueryLogging:         true,
			IndexUsageAnalysis:       true,
			ReplicationLagMonitoring: true,
			AlertThresholds: AlertConfig{
				MaxQueryLatency:    1 * time.Millisecond,
				MaxConnectionUsage: 0.8,
				MinCacheHitRate:    0.9,
				MaxReplicationLag:  5 * time.Second,
				MaxIndexUnusedDays: 30,
				MaxLockWaitTime:    1 * time.Second,
			},
		},
		Schema: SchemaConfig{
			Partitioning: PartitioningConfig{
				Enabled:    true,
				Strategy:   "time",
				AutoCreate: true,
				PreCreate:  3,
				Tables: map[string]TablePartition{
					"trades": {
						Interval:    "daily",
						Column:      "created_at",
						Retention:   "90d",
						Compression: true,
					},
					"orders": {
						Interval:    "daily",
						Column:      "created_at",
						Retention:   "180d",
						Compression: true,
					},
					"user_activity": {
						Interval:    "weekly",
						Column:      "created_at",
						Retention:   "1y",
						Compression: true,
					},
				},
			},
			Compression: CompressionConfig{
				Enabled:   true,
				Algorithm: "lz4",
				MinSize:   1024 * 1024, // 1MB
				Tables: map[string]bool{
					"trades":        true,
					"orders":        true,
					"user_activity": true,
					"audit_logs":    true,
				},
			},
			AutoVacuum: AutoVacuumConfig{
				Enabled:            true,
				VacuumThreshold:    50,
				VacuumScaleFactor:  0.2,
				AnalyzeThreshold:   50,
				AnalyzeScaleFactor: 0.1,
				VacuumCostLimit:    200,
				VacuumCostDelay:    20,
			},
			RetentionPolicies: RetentionConfig{
				Trades:       "90d",
				Orders:       "180d",
				UserActivity: "1y",
				AuditLogs:    "2y",
				Metrics:      "30d",
			},
		},
		Index: IndexConfig{
			AutoCreate:        true,
			AutoDrop:          false, // Safer to manually review
			UsageAnalysis:     true,
			UnusedThreshold:   30,
			MaintenanceWindow: "0 1 * * 0", // 1 AM on Sundays
		},
		Routing: RoutingConfig{
			Enabled:           true,
			Strategy:          "latency",
			ConsistencyLevel:  "eventual",
			MaxReplicationLag: 5 * time.Second,
			ReadPreference:    "replica",
			CriticalTables:    []string{"orders", "trades", "positions"},
			RoutingRules: []RoutingRule{
				{
					Pattern:     "INSERT|UPDATE|DELETE",
					Destination: "master",
					Priority:    100,
				},
				{
					Pattern:     "SELECT.*FROM.*(orders|trades|positions)",
					Destination: "both",
					Priority:    90,
				},
				{
					Pattern:     "SELECT",
					Destination: "replica",
					Priority:    10,
				},
			},
		},
	}
}
