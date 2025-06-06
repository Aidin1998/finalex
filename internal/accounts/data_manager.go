// AccountDataManager provides comprehensive multi-tier data architecture with CQRS pattern
package accounts

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// DataTier represents different data storage tiers
type DataTier int

const (
	HotTier DataTier = iota
	WarmTier
	ColdTier
	ArchiveTier
)

// AccountDataManager orchestrates data across multiple tiers with CQRS pattern
type AccountDataManager struct {
	// Core components
	repository     *Repository
	cache          *CacheLayer
	hotCache       *HotCache
	shardManager   *ShardManager
	eventStore     *EventStore
	commandHandler *AccountCommandHandler
	queryHandler   *AccountQueryHandler

	// Data lifecycle management
	lifecycleManager *DataLifecycleManager
	migrationManager *MigrationManager

	// Monitoring and metrics
	logger  *zap.Logger
	metrics *DataManagerMetrics

	// Configuration
	config *DataManagerConfig

	// Internal state
	mu     sync.RWMutex
	status DataManagerStatus
}

// DataManagerConfig represents configuration for the data manager
type DataManagerConfig struct {
	// Performance settings
	MaxConcurrentOperations int           `json:"max_concurrent_operations"`
	BatchSize               int           `json:"batch_size"`
	FlushInterval           time.Duration `json:"flush_interval"`

	// Cache settings
	HotCacheSize int           `json:"hot_cache_size"`
	HotCacheTTL  time.Duration `json:"hot_cache_ttl"`
	WarmCacheTTL time.Duration `json:"warm_cache_ttl"`
	ColdCacheTTL time.Duration `json:"cold_cache_ttl"`

	// Sharding settings
	ShardCount           int     `json:"shard_count"`
	RebalanceThreshold   float64 `json:"rebalance_threshold"`
	AutoRebalanceEnabled bool    `json:"auto_rebalance_enabled"`

	// Data lifecycle settings
	ArchiveAfterDays   int  `json:"archive_after_days"`
	PurgeAfterDays     int  `json:"purge_after_days"`
	CompressionEnabled bool `json:"compression_enabled"`

	// Event sourcing settings
	EventBatchSize   int           `json:"event_batch_size"`
	EventRetention   time.Duration `json:"event_retention"`
	SnapshotInterval int           `json:"snapshot_interval"`

	// Migration settings
	ZeroDowntimeEnabled bool          `json:"zero_downtime_enabled"`
	MigrationTimeout    time.Duration `json:"migration_timeout"`
	RollbackEnabled     bool          `json:"rollback_enabled"`
}

// DataManagerStatus represents the current status of the data manager
type DataManagerStatus struct {
	IsHealthy           bool      `json:"is_healthy"`
	ActiveConnections   int       `json:"active_connections"`
	QueuedOperations    int       `json:"queued_operations"`
	LastHealthCheck     time.Time `json:"last_health_check"`
	ThroughputPerSecond int64     `json:"throughput_per_second"`
	ErrorRate           float64   `json:"error_rate"`
}

// DataManagerMetrics holds Prometheus metrics for the data manager
type DataManagerMetrics struct {
	// Operation metrics
	OperationsTotal   *prometheus.CounterVec
	OperationDuration *prometheus.HistogramVec
	OperationErrors   *prometheus.CounterVec

	// Cache metrics
	CacheHitRate *prometheus.GaugeVec
	CacheLatency *prometheus.HistogramVec
	CacheSize    *prometheus.GaugeVec

	// Shard metrics
	ShardDistribution *prometheus.GaugeVec
	ShardRebalances   *prometheus.CounterVec
	ShardMigrations   *prometheus.CounterVec

	// Data lifecycle metrics
	DataTierDistribution *prometheus.GaugeVec
	ArchiveOperations    *prometheus.CounterVec
	PurgeOperations      *prometheus.CounterVec

	// Event sourcing metrics
	EventsProcessed  *prometheus.CounterVec
	EventLatency     *prometheus.HistogramVec
	SnapshotsCreated *prometheus.CounterVec

	// Health metrics
	HealthStatus    prometheus.Gauge
	ThroughputGauge prometheus.Gauge
	ErrorRateGauge  prometheus.Gauge
}

// NewAccountDataManager creates a new account data manager
func NewAccountDataManager(
	repository *Repository,
	cache *CacheLayer,
	config *DataManagerConfig,
	logger *zap.Logger,
) (*AccountDataManager, error) {
	// Initialize metrics
	metrics := &DataManagerMetrics{
		OperationsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "account_data_operations_total",
				Help: "Total number of data operations",
			},
			[]string{"operation", "tier", "status"},
		),
		OperationDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "account_data_operation_duration_seconds",
				Help:    "Duration of data operations",
				Buckets: prometheus.ExponentialBuckets(0.0001, 2, 15), // 0.1ms to ~3s
			},
			[]string{"operation", "tier"},
		),
		OperationErrors: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "account_data_operation_errors_total",
				Help: "Total number of data operation errors",
			},
			[]string{"operation", "tier", "error_type"},
		),
		CacheHitRate: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "account_cache_hit_rate",
				Help: "Cache hit rate by tier",
			},
			[]string{"tier"},
		),
		CacheLatency: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "account_cache_latency_seconds",
				Help:    "Cache operation latency",
				Buckets: prometheus.ExponentialBuckets(0.00001, 2, 20), // 0.01ms to ~10s
			},
			[]string{"operation", "tier"},
		),
		CacheSize: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "account_cache_size_bytes",
				Help: "Cache size in bytes by tier",
			},
			[]string{"tier"},
		),
		ShardDistribution: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "account_shard_distribution",
				Help: "Distribution of data across shards",
			},
			[]string{"shard_id"},
		),
		ShardRebalances: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "account_shard_rebalances_total",
				Help: "Total number of shard rebalances",
			},
			[]string{"from_shard", "to_shard"},
		),
		ShardMigrations: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "account_shard_migrations_total",
				Help: "Total number of shard migrations",
			},
			[]string{"migration_type", "status"},
		),
		DataTierDistribution: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "account_data_tier_distribution",
				Help: "Distribution of data across tiers",
			},
			[]string{"tier"},
		),
		ArchiveOperations: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "account_archive_operations_total",
				Help: "Total number of archive operations",
			},
			[]string{"operation", "status"},
		),
		PurgeOperations: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "account_purge_operations_total",
				Help: "Total number of purge operations",
			},
			[]string{"operation", "status"},
		),
		EventsProcessed: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "account_events_processed_total",
				Help: "Total number of events processed",
			},
			[]string{"event_type", "status"},
		),
		EventLatency: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "account_event_latency_seconds",
				Help:    "Event processing latency",
				Buckets: prometheus.ExponentialBuckets(0.0001, 2, 15),
			},
			[]string{"event_type"},
		),
		SnapshotsCreated: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "account_snapshots_created_total",
				Help: "Total number of snapshots created",
			},
			[]string{"snapshot_type", "status"},
		),
		HealthStatus: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "account_data_manager_health",
				Help: "Health status of the data manager (1=healthy, 0=unhealthy)",
			},
		),
		ThroughputGauge: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "account_data_manager_throughput_ops_per_second",
				Help: "Current throughput in operations per second",
			},
		),
		ErrorRateGauge: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "account_data_manager_error_rate",
				Help: "Current error rate (0-1)",
			},
		),
	}

	// Initialize hot cache
	hotCache, err := NewHotCache(config.HotCacheSize, config.HotCacheTTL, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create hot cache: %w", err)
	}

	// Initialize shard manager
	shardManager, err := NewShardManager(config.ShardCount, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create shard manager: %w", err)
	}

	// Initialize event store
	eventStore, err := NewEventStore(repository, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create event store: %w", err)
	}

	dm := &AccountDataManager{
		repository:   repository,
		cache:        cache,
		hotCache:     hotCache,
		shardManager: shardManager,
		eventStore:   eventStore,
		logger:       logger,
		metrics:      metrics,
		config:       config,
		status: DataManagerStatus{
			IsHealthy:       true,
			LastHealthCheck: time.Now(),
		},
	}

	// Initialize CQRS handlers
	dm.commandHandler = NewAccountCommandHandler(dm, logger)
	dm.queryHandler = NewAccountQueryHandler(dm, logger)

	// Initialize lifecycle manager
	dm.lifecycleManager = NewDataLifecycleManager(dm, config, logger)

	// Initialize migration manager
	dm.migrationManager = NewMigrationManager(dm, config, logger)

	return dm, nil
}

// GetAccount retrieves an account using the optimal data tier
func (dm *AccountDataManager) GetAccount(ctx context.Context, userID uuid.UUID, currency string) (*Account, error) {
	return dm.queryHandler.GetAccount(ctx, userID, currency)
}

// UpdateBalance updates an account balance atomically
func (dm *AccountDataManager) UpdateBalance(ctx context.Context, update *BalanceUpdate) error {
	return dm.commandHandler.UpdateBalance(ctx, update)
}

// CreateReservation creates a new fund reservation
func (dm *AccountDataManager) CreateReservation(ctx context.Context, reservation *Reservation) error {
	return dm.commandHandler.CreateReservation(ctx, reservation)
}

// ReleaseReservation releases a fund reservation
func (dm *AccountDataManager) ReleaseReservation(ctx context.Context, reservationID uuid.UUID) error {
	return dm.commandHandler.ReleaseReservation(ctx, reservationID)
}

// GetBalance retrieves account balance with sub-millisecond performance
func (dm *AccountDataManager) GetBalance(ctx context.Context, userID uuid.UUID, currency string) (decimal.Decimal, error) {
	return dm.queryHandler.GetBalance(ctx, userID, currency)
}

// GetAccountHistory retrieves account transaction history
func (dm *AccountDataManager) GetAccountHistory(ctx context.Context, userID uuid.UUID, currency string, limit int, offset int) ([]*LedgerTransaction, error) {
	return dm.queryHandler.GetAccountHistory(ctx, userID, currency, limit, offset)
}

// StartBackgroundTasks starts all background processing tasks
func (dm *AccountDataManager) StartBackgroundTasks(ctx context.Context) error {
	// Start lifecycle management
	if err := dm.lifecycleManager.Start(ctx); err != nil {
		return fmt.Errorf("failed to start lifecycle manager: %w", err)
	}

	// Start shard rebalancing if enabled
	if dm.config.AutoRebalanceEnabled {
		if err := dm.shardManager.StartAutoRebalance(ctx); err != nil {
			return fmt.Errorf("failed to start shard rebalancing: %w", err)
		}
	}

	// Start health monitoring
	go dm.monitorHealth(ctx)

	dm.logger.Info("Account data manager background tasks started")
	return nil
}

// Stop gracefully stops the data manager
func (dm *AccountDataManager) Stop(ctx context.Context) error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	dm.status.IsHealthy = false

	// Stop lifecycle manager
	if err := dm.lifecycleManager.Stop(ctx); err != nil {
		dm.logger.Error("Failed to stop lifecycle manager", zap.Error(err))
	}

	// Stop shard manager
	if err := dm.shardManager.Stop(ctx); err != nil {
		dm.logger.Error("Failed to stop shard manager", zap.Error(err))
	}

	// Stop event store
	if err := dm.eventStore.Stop(ctx); err != nil {
		dm.logger.Error("Failed to stop event store", zap.Error(err))
	}

	dm.logger.Info("Account data manager stopped")
	return nil
}

// GetHealthStatus returns the current health status
func (dm *AccountDataManager) GetHealthStatus() DataManagerStatus {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return dm.status
}

// monitorHealth continuously monitors the health of the data manager
func (dm *AccountDataManager) monitorHealth(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			dm.updateHealthStatus()
		}
	}
}

// updateHealthStatus updates the current health status
func (dm *AccountDataManager) updateHealthStatus() {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	// Check component health
	isHealthy := true

	// Check cache health
	if dm.cache != nil {
		// Implement cache health check
		// isHealthy = isHealthy && dm.cache.IsHealthy()
	}

	// Check repository health
	if dm.repository != nil {
		// Implement repository health check
		// isHealthy = isHealthy && dm.repository.IsHealthy()
	}

	// Update status
	dm.status.IsHealthy = isHealthy
	dm.status.LastHealthCheck = time.Now()

	// Update metrics
	if isHealthy {
		dm.metrics.HealthStatus.Set(1)
	} else {
		dm.metrics.HealthStatus.Set(0)
	}
}
