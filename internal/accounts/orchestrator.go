// Unified orchestration layer for the comprehensive account persistence system
package accounts

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// AccountOrchestrator is the main orchestration layer that coordinates all components
type AccountOrchestrator struct {
	// Core components
	dataManager       *AccountDataManager
	hotCache          *HotCache
	shardManager      *ShardManager
	partitionManager  *PartitionManager
	commandHandler    *AccountCommandHandler
	queryHandler      *AccountQueryHandler
	eventStore        *EventStore
	lifecycleManager  *DataLifecycleManager
	migrationManager  *MigrationManager
	monitoringManager *AdvancedMonitoringManager
	benchmarkFramework *BenchmarkingFramework

	// Configuration and state
	config    *OrchestratorConfig
	logger    *zap.Logger
	db        *gorm.DB
	isRunning bool
	mutex     sync.RWMutex

	// Metrics
	metrics *OrchestratorMetrics

	// Background workers
	workers map[string]*BackgroundWorker
	ctx     context.Context
	cancel  context.CancelFunc
}

// OrchestratorConfig contains configuration for the orchestrator
type OrchestratorConfig struct {
	// Performance settings
	MaxConcurrentOperations int           `yaml:"max_concurrent_operations" json:"max_concurrent_operations"`
	OperationTimeout        time.Duration `yaml:"operation_timeout" json:"operation_timeout"`
	HealthCheckInterval     time.Duration `yaml:"health_check_interval" json:"health_check_interval"`
	
	// Component configurations
	EnableHotCache      bool `yaml:"enable_hot_cache" json:"enable_hot_cache"`
	EnableEventSourcing bool `yaml:"enable_event_sourcing" json:"enable_event_sourcing"`
	EnableSharding      bool `yaml:"enable_sharding" json:"enable_sharding"`
	EnableLifecycle     bool `yaml:"enable_lifecycle" json:"enable_lifecycle"`
	EnableMigrations    bool `yaml:"enable_migrations" json:"enable_migrations"`
	EnableMonitoring    bool `yaml:"enable_monitoring" json:"enable_monitoring"`
	
	// Auto-scaling settings
	AutoScaleEnabled         bool    `yaml:"auto_scale_enabled" json:"auto_scale_enabled"`
	ScaleUpThreshold         float64 `yaml:"scale_up_threshold" json:"scale_up_threshold"`
	ScaleDownThreshold       float64 `yaml:"scale_down_threshold" json:"scale_down_threshold"`
	MinShards                int     `yaml:"min_shards" json:"min_shards"`
	MaxShards                int     `yaml:"max_shards" json:"max_shards"`
	
	// Circuit breaker settings
	CircuitBreakerEnabled    bool    `yaml:"circuit_breaker_enabled" json:"circuit_breaker_enabled"`
	FailureThreshold         float64 `yaml:"failure_threshold" json:"failure_threshold"`
	RecoveryTimeout          time.Duration `yaml:"recovery_timeout" json:"recovery_timeout"`
}

// OrchestratorMetrics contains Prometheus metrics for the orchestrator
type OrchestratorMetrics struct {
	operationsTotal      prometheus.CounterVec
	operationDuration    prometheus.HistogramVec
	componentHealth      prometheus.GaugeVec
	systemLoad           prometheus.Gauge
	errorRate            prometheus.Gauge
	throughput           prometheus.Gauge
	activeConnections    prometheus.Gauge
	memoryUsage          prometheus.Gauge
	cpuUsage             prometheus.Gauge
}

// BackgroundWorker represents a background worker process
type BackgroundWorker struct {
	name     string
	task     func(context.Context) error
	interval time.Duration
	running  bool
	mutex    sync.RWMutex
}

// NewAccountOrchestrator creates a new account orchestrator
func NewAccountOrchestrator(db *gorm.DB, logger *zap.Logger, config *OrchestratorConfig) (*AccountOrchestrator, error) {
	ctx, cancel := context.WithCancel(context.Background())
	
	orchestrator := &AccountOrchestrator{
		config:  config,
		logger:  logger,
		db:      db,
		ctx:     ctx,
		cancel:  cancel,
		workers: make(map[string]*BackgroundWorker),
		metrics: createOrchestratorMetrics(),
	}

	// Initialize all components
	if err := orchestrator.initializeComponents(); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to initialize components: %w", err)
	}

	return orchestrator, nil
}

// createOrchestratorMetrics creates Prometheus metrics for the orchestrator
func createOrchestratorMetrics() *OrchestratorMetrics {
	return &OrchestratorMetrics{
		operationsTotal: *promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "account_orchestrator_operations_total",
				Help: "Total number of operations processed by the orchestrator",
			},
			[]string{"operation", "status", "component"},
		),
		operationDuration: *promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "account_orchestrator_operation_duration_seconds",
				Help:    "Duration of operations in the orchestrator",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"operation", "component"},
		),
		componentHealth: *promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "account_orchestrator_component_health",
				Help: "Health status of orchestrator components (1=healthy, 0=unhealthy)",
			},
			[]string{"component"},
		),
		systemLoad: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "account_orchestrator_system_load",
				Help: "Current system load factor",
			},
		),
		errorRate: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "account_orchestrator_error_rate",
				Help: "Current error rate percentage",
			},
		),
		throughput: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "account_orchestrator_throughput_ops_per_second",
				Help: "Current throughput in operations per second",
			},
		),
		activeConnections: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "account_orchestrator_active_connections",
				Help: "Number of active database connections",
			},
		),
		memoryUsage: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "account_orchestrator_memory_usage_bytes",
				Help: "Current memory usage in bytes",
			},
		),
		cpuUsage: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "account_orchestrator_cpu_usage_percent",
				Help: "Current CPU usage percentage",
			},
		),
	}
}

// initializeComponents initializes all orchestrator components
func (o *AccountOrchestrator) initializeComponents() error {
	var err error

	// Initialize data manager (core component)
	if o.dataManager, err = NewAccountDataManager(o.db, o.logger, &DataManagerConfig{
		CacheSize:              100000,
		MaxConcurrentOps:       o.config.MaxConcurrentOperations,
		EnableMetrics:          true,
		EnableCircuitBreaker:   o.config.CircuitBreakerEnabled,
		CircuitBreakerTimeout:  o.config.RecoveryTimeout,
	}); err != nil {
		return fmt.Errorf("failed to create data manager: %w", err)
	}

	// Initialize hot cache if enabled
	if o.config.EnableHotCache {
		if o.hotCache, err = NewHotCache(&HotCacheConfig{
			MaxSize:       100000,
			TTL:          time.Hour,
			EnableMetrics: true,
		}); err != nil {
			return fmt.Errorf("failed to create hot cache: %w", err)
		}
	}

	// Initialize shard manager if enabled
	if o.config.EnableSharding {
		if o.shardManager, err = NewShardManager(o.db, o.logger, &ShardConfig{
			InitialShards:    o.config.MinShards,
			MaxShards:       o.config.MaxShards,
			RebalanceThreshold: 0.8,
			EnableAutoScale: o.config.AutoScaleEnabled,
		}); err != nil {
			return fmt.Errorf("failed to create shard manager: %w", err)
		}
	}

	// Initialize existing partition manager
	if o.partitionManager, err = NewPartitionManager(o.db, o.logger, &PartitionConfig{
		MaxPartitions:     1000,
		RebalanceEnabled:  true,
		MetricsEnabled:    true,
	}); err != nil {
		return fmt.Errorf("failed to create partition manager: %w", err)
	}

	// Initialize CQRS handlers
	if o.commandHandler, err = NewAccountCommandHandler(o.db, o.logger, &CommandConfig{
		MaxRetries:        3,
		IdempotencyTTL:   time.Hour,
		EnableEvents:      o.config.EnableEventSourcing,
	}); err != nil {
		return fmt.Errorf("failed to create command handler: %w", err)
	}

	if o.queryHandler, err = NewAccountQueryHandler(o.db, o.logger, &QueryConfig{
		CacheEnabled:      o.config.EnableHotCache,
		MaxConcurrency:    o.config.MaxConcurrentOperations,
		QueryTimeout:      o.config.OperationTimeout,
	}); err != nil {
		return fmt.Errorf("failed to create query handler: %w", err)
	}

	// Initialize event store if enabled
	if o.config.EnableEventSourcing {
		if o.eventStore, err = NewEventStore(o.db, o.logger, &EventStoreConfig{
			BatchSize:        1000,
			FlushInterval:    time.Second,
			EnableSnapshots:  true,
			SnapshotInterval: 1000,
		}); err != nil {
			return fmt.Errorf("failed to create event store: %w", err)
		}
	}

	// Initialize lifecycle manager if enabled
	if o.config.EnableLifecycle {
		if o.lifecycleManager, err = NewDataLifecycleManager(o.db, o.logger, &LifecycleConfig{
			ArchiveThreshold: 90 * 24 * time.Hour, // 90 days
			PurgeThreshold:   365 * 24 * time.Hour, // 1 year
			EnableCompression: true,
			EnableAutoCleanup: true,
		}); err != nil {
			return fmt.Errorf("failed to create lifecycle manager: %w", err)
		}
	}

	// Initialize migration manager if enabled
	if o.config.EnableMigrations {
		if o.migrationManager, err = NewMigrationManager(o.db, o.logger, &MigrationConfig{
			Strategy:           "blue-green",
			MaxConcurrency:     10,
			HealthCheckTimeout: 30 * time.Second,
			RollbackEnabled:    true,
		}); err != nil {
			return fmt.Errorf("failed to create migration manager: %w", err)
		}
	}

	// Initialize monitoring manager if enabled
	if o.config.EnableMonitoring {
		if o.monitoringManager, err = NewAdvancedMonitoringManager(o.logger, &MonitoringConfig{
			AlertingEnabled:    true,
			MetricsRetention:   7 * 24 * time.Hour,
			HealthCheckInterval: o.config.HealthCheckInterval,
		}); err != nil {
			return fmt.Errorf("failed to create monitoring manager: %w", err)
		}
	}

	// Initialize benchmarking framework
	if o.benchmarkFramework, err = NewBenchmarkingFramework(o.db, o.logger, &BenchmarkConfig{
		MaxConcurrency:     1000,
		TestDuration:       time.Minute,
		WarmupDuration:     10 * time.Second,
		CollectMetrics:     true,
	}); err != nil {
		return fmt.Errorf("failed to create benchmark framework: %w", err)
	}

	return nil
}

// Start starts the orchestrator and all its components
func (o *AccountOrchestrator) Start(ctx context.Context) error {
	o.mutex.Lock()
	defer o.mutex.Unlock()

	if o.isRunning {
		return fmt.Errorf("orchestrator is already running")
	}

	o.logger.Info("Starting account orchestrator")

	// Start all components
	if err := o.startComponents(ctx); err != nil {
		return fmt.Errorf("failed to start components: %w", err)
	}

	// Start background workers
	o.startBackgroundWorkers()

	// Start health monitoring
	if o.config.EnableMonitoring && o.monitoringManager != nil {
		if err := o.monitoringManager.Start(ctx); err != nil {
			o.logger.Error("Failed to start monitoring", zap.Error(err))
		}
	}

	o.isRunning = true
	o.logger.Info("Account orchestrator started successfully")

	return nil
}

// Stop stops the orchestrator and all its components
func (o *AccountOrchestrator) Stop() error {
	o.mutex.Lock()
	defer o.mutex.Unlock()

	if !o.isRunning {
		return nil
	}

	o.logger.Info("Stopping account orchestrator")

	// Cancel context to stop all operations
	o.cancel()

	// Stop background workers
	o.stopBackgroundWorkers()

	// Stop components
	if err := o.stopComponents(); err != nil {
		o.logger.Error("Error stopping components", zap.Error(err))
	}

	o.isRunning = false
	o.logger.Info("Account orchestrator stopped")

	return nil
}

// startComponents starts all orchestrator components
func (o *AccountOrchestrator) startComponents(ctx context.Context) error {
	// Start data manager
	if err := o.dataManager.Start(ctx); err != nil {
		return fmt.Errorf("failed to start data manager: %w", err)
	}

	// Start shard manager if enabled
	if o.config.EnableSharding && o.shardManager != nil {
		if err := o.shardManager.Start(ctx); err != nil {
			return fmt.Errorf("failed to start shard manager: %w", err)
		}
	}

	// Start event store if enabled
	if o.config.EnableEventSourcing && o.eventStore != nil {
		if err := o.eventStore.Start(ctx); err != nil {
			return fmt.Errorf("failed to start event store: %w", err)
		}
	}

	// Start lifecycle manager if enabled
	if o.config.EnableLifecycle && o.lifecycleManager != nil {
		if err := o.lifecycleManager.Start(ctx); err != nil {
			return fmt.Errorf("failed to start lifecycle manager: %w", err)
		}
	}

	return nil
}

// stopComponents stops all orchestrator components
func (o *AccountOrchestrator) stopComponents() error {
	var errors []error

	// Stop components in reverse order
	if o.lifecycleManager != nil {
		if err := o.lifecycleManager.Stop(); err != nil {
			errors = append(errors, fmt.Errorf("lifecycle manager stop error: %w", err))
		}
	}

	if o.eventStore != nil {
		if err := o.eventStore.Stop(); err != nil {
			errors = append(errors, fmt.Errorf("event store stop error: %w", err))
		}
	}

	if o.shardManager != nil {
		if err := o.shardManager.Stop(); err != nil {
			errors = append(errors, fmt.Errorf("shard manager stop error: %w", err))
		}
	}

	if o.dataManager != nil {
		if err := o.dataManager.Stop(); err != nil {
			errors = append(errors, fmt.Errorf("data manager stop error: %w", err))
		}
	}

	if o.monitoringManager != nil {
		if err := o.monitoringManager.Stop(); err != nil {
			errors = append(errors, fmt.Errorf("monitoring manager stop error: %w", err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("component stop errors: %v", errors)
	}

	return nil
}

// startBackgroundWorkers starts all background worker processes
func (o *AccountOrchestrator) startBackgroundWorkers() {
	// Health check worker
	o.addWorker("health-check", o.healthCheckWorker, o.config.HealthCheckInterval)

	// Metrics collection worker
	o.addWorker("metrics-collection", o.metricsCollectionWorker, 30*time.Second)

	// Auto-scaling worker
	if o.config.AutoScaleEnabled {
		o.addWorker("auto-scaling", o.autoScalingWorker, time.Minute)
	}

	// Lifecycle management worker
	if o.config.EnableLifecycle {
		o.addWorker("lifecycle-management", o.lifecycleWorker, time.Hour)
	}

	// Start all workers
	for _, worker := range o.workers {
		go o.runWorker(worker)
	}
}

// stopBackgroundWorkers stops all background worker processes
func (o *AccountOrchestrator) stopBackgroundWorkers() {
	for _, worker := range o.workers {
		worker.mutex.Lock()
		worker.running = false
		worker.mutex.Unlock()
	}
}

// addWorker adds a new background worker
func (o *AccountOrchestrator) addWorker(name string, task func(context.Context) error, interval time.Duration) {
	o.workers[name] = &BackgroundWorker{
		name:     name,
		task:     task,
		interval: interval,
		running:  true,
	}
}

// runWorker runs a background worker
func (o *AccountOrchestrator) runWorker(worker *BackgroundWorker) {
	ticker := time.NewTicker(worker.interval)
	defer ticker.Stop()

	for {
		select {
		case <-o.ctx.Done():
			return
		case <-ticker.C:
			worker.mutex.RLock()
			if !worker.running {
				worker.mutex.RUnlock()
				return
			}
			worker.mutex.RUnlock()

			if err := worker.task(o.ctx); err != nil {
				o.logger.Error("Worker task failed",
					zap.String("worker", worker.name),
					zap.Error(err))
				o.metrics.operationsTotal.WithLabelValues("worker", "error", worker.name).Inc()
			} else {
				o.metrics.operationsTotal.WithLabelValues("worker", "success", worker.name).Inc()
			}
		}
	}
}

// Worker methods for background processes

// healthCheckWorker performs periodic health checks on all components
func (o *AccountOrchestrator) healthCheckWorker(ctx context.Context) error {
	start := time.Now()
	defer func() {
		o.metrics.operationDuration.WithLabelValues("health-check", "orchestrator").Observe(time.Since(start).Seconds())
	}()

	healthStatus := o.GetHealthStatus()
	allHealthy := true

	for component, healthy := range healthStatus {
		if healthy {
			o.metrics.componentHealth.WithLabelValues(component).Set(1)
		} else {
			o.metrics.componentHealth.WithLabelValues(component).Set(0)
			allHealthy = false
			o.logger.Warn("Component health check failed", zap.String("component", component))
		}
	}

	if !allHealthy && o.monitoringManager != nil {
		o.monitoringManager.TriggerAlert("system_health", "One or more components are unhealthy", "warning")
	}

	return nil
}

// metricsCollectionWorker collects and updates system metrics
func (o *AccountOrchestrator) metricsCollectionWorker(ctx context.Context) error {
	start := time.Now()
	defer func() {
		o.metrics.operationDuration.WithLabelValues("metrics-collection", "orchestrator").Observe(time.Since(start).Seconds())
	}()

	// Update system load metric
	if o.dataManager != nil {
		metrics := o.dataManager.GetMetrics()
		if loadFactor, ok := metrics["load_factor"].(float64); ok {
			o.metrics.systemLoad.Set(loadFactor)
		}
		if errorRate, ok := metrics["error_rate"].(float64); ok {
			o.metrics.errorRate.Set(errorRate)
		}
		if throughput, ok := metrics["throughput"].(float64); ok {
			o.metrics.throughput.Set(throughput)
		}
	}

	// Update cache metrics
	if o.hotCache != nil {
		stats := o.hotCache.GetStats()
		if hitRate, ok := stats["hit_rate"].(float64); ok {
			o.metrics.componentHealth.WithLabelValues("cache_hit_rate").Set(hitRate)
		}
	}

	return nil
}

// autoScalingWorker handles automatic scaling decisions
func (o *AccountOrchestrator) autoScalingWorker(ctx context.Context) error {
	start := time.Now()
	defer func() {
		o.metrics.operationDuration.WithLabelValues("auto-scaling", "orchestrator").Observe(time.Since(start).Seconds())
	}()

	if !o.config.AutoScaleEnabled || o.shardManager == nil {
		return nil
	}

	metrics := o.shardManager.GetMetrics()
	loadFactor, ok := metrics["load_factor"].(float64)
	if !ok {
		return nil
	}

	currentShards, ok := metrics["shard_count"].(int)
	if !ok {
		return nil
	}

	// Scale up decision
	if loadFactor > o.config.ScaleUpThreshold && currentShards < o.config.MaxShards {
		o.logger.Info("Auto-scaling up", 
			zap.Float64("load_factor", loadFactor),
			zap.Int("current_shards", currentShards))
		
		if err := o.shardManager.AddShard(ctx); err != nil {
			o.logger.Error("Failed to scale up", zap.Error(err))
			return err
		}
		
		if o.monitoringManager != nil {
			o.monitoringManager.TriggerAlert("auto_scale", "System scaled up", "info")
		}
	}

	// Scale down decision
	if loadFactor < o.config.ScaleDownThreshold && currentShards > o.config.MinShards {
		o.logger.Info("Auto-scaling down", 
			zap.Float64("load_factor", loadFactor),
			zap.Int("current_shards", currentShards))
		
		if err := o.shardManager.RemoveShard(ctx); err != nil {
			o.logger.Error("Failed to scale down", zap.Error(err))
			return err
		}
		
		if o.monitoringManager != nil {
			o.monitoringManager.TriggerAlert("auto_scale", "System scaled down", "info")
		}
	}

	return nil
}

// lifecycleWorker handles data lifecycle management tasks
func (o *AccountOrchestrator) lifecycleWorker(ctx context.Context) error {
	start := time.Now()
	defer func() {
		o.metrics.operationDuration.WithLabelValues("lifecycle", "orchestrator").Observe(time.Since(start).Seconds())
	}()

	if o.lifecycleManager == nil {
		return nil
	}

	// Run lifecycle policies
	if err := o.lifecycleManager.ExecutePolicies(ctx); err != nil {
		o.logger.Error("Failed to execute lifecycle policies", zap.Error(err))
		return err
	}

	return nil
}

// Account operation interfaces

// CreateAccount creates a new account through the orchestrator
func (o *AccountOrchestrator) CreateAccount(ctx context.Context, account *Account) error {
	start := time.Now()
	defer func() {
		o.metrics.operationDuration.WithLabelValues("create_account", "orchestrator").Observe(time.Since(start).Seconds())
	}()

	// Use command handler for write operations
	if o.commandHandler != nil {
		if err := o.commandHandler.CreateAccount(ctx, account); err != nil {
			o.metrics.operationsTotal.WithLabelValues("create_account", "error", "command_handler").Inc()
			return err
		}
		o.metrics.operationsTotal.WithLabelValues("create_account", "success", "command_handler").Inc()
		return nil
	}

	// Fallback to data manager
	if err := o.dataManager.CreateAccount(ctx, account); err != nil {
		o.metrics.operationsTotal.WithLabelValues("create_account", "error", "data_manager").Inc()
		return err
	}

	o.metrics.operationsTotal.WithLabelValues("create_account", "success", "data_manager").Inc()
	return nil
}

// GetAccount retrieves an account through the orchestrator
func (o *AccountOrchestrator) GetAccount(ctx context.Context, userID string) (*Account, error) {
	start := time.Now()
	defer func() {
		o.metrics.operationDuration.WithLabelValues("get_account", "orchestrator").Observe(time.Since(start).Seconds())
	}()

	// Try hot cache first if enabled
	if o.hotCache != nil {
		if account, found := o.hotCache.GetAccount(userID); found {
			o.metrics.operationsTotal.WithLabelValues("get_account", "success", "hot_cache").Inc()
			return account, nil
		}
	}

	// Use query handler for read operations
	if o.queryHandler != nil {
		account, err := o.queryHandler.GetAccount(ctx, userID)
		if err != nil {
			o.metrics.operationsTotal.WithLabelValues("get_account", "error", "query_handler").Inc()
			return nil, err
		}

		// Cache the result if hot cache is enabled
		if o.hotCache != nil {
			o.hotCache.SetAccount(userID, account)
		}

		o.metrics.operationsTotal.WithLabelValues("get_account", "success", "query_handler").Inc()
		return account, nil
	}

	// Fallback to data manager
	account, err := o.dataManager.GetAccount(ctx, userID)
	if err != nil {
		o.metrics.operationsTotal.WithLabelValues("get_account", "error", "data_manager").Inc()
		return nil, err
	}

	o.metrics.operationsTotal.WithLabelValues("get_account", "success", "data_manager").Inc()
	return account, nil
}

// UpdateAccount updates an account through the orchestrator
func (o *AccountOrchestrator) UpdateAccount(ctx context.Context, account *Account) error {
	start := time.Now()
	defer func() {
		o.metrics.operationDuration.WithLabelValues("update_account", "orchestrator").Observe(time.Since(start).Seconds())
	}()

	// Invalidate cache if enabled
	if o.hotCache != nil {
		o.hotCache.DeleteAccount(account.UserID)
	}

	// Use command handler for write operations
	if o.commandHandler != nil {
		if err := o.commandHandler.UpdateAccount(ctx, account); err != nil {
			o.metrics.operationsTotal.WithLabelValues("update_account", "error", "command_handler").Inc()
			return err
		}
		o.metrics.operationsTotal.WithLabelValues("update_account", "success", "command_handler").Inc()
		return nil
	}

	// Fallback to data manager
	if err := o.dataManager.UpdateAccount(ctx, account); err != nil {
		o.metrics.operationsTotal.WithLabelValues("update_account", "error", "data_manager").Inc()
		return err
	}

	o.metrics.operationsTotal.WithLabelValues("update_account", "success", "data_manager").Inc()
	return nil
}

// DeleteAccount deletes an account through the orchestrator
func (o *AccountOrchestrator) DeleteAccount(ctx context.Context, userID string) error {
	start := time.Now()
	defer func() {
		o.metrics.operationDuration.WithLabelValues("delete_account", "orchestrator").Observe(time.Since(start).Seconds())
	}()

	// Invalidate cache if enabled
	if o.hotCache != nil {
		o.hotCache.DeleteAccount(userID)
	}

	// Use command handler for write operations
	if o.commandHandler != nil {
		if err := o.commandHandler.DeleteAccount(ctx, userID); err != nil {
			o.metrics.operationsTotal.WithLabelValues("delete_account", "error", "command_handler").Inc()
			return err
		}
		o.metrics.operationsTotal.WithLabelValues("delete_account", "success", "command_handler").Inc()
		return nil
	}

	// Fallback to data manager
	if err := o.dataManager.DeleteAccount(ctx, userID); err != nil {
		o.metrics.operationsTotal.WithLabelValues("delete_account", "error", "data_manager").Inc()
		return err
	}

	o.metrics.operationsTotal.WithLabelValues("delete_account", "success", "data_manager").Inc()
	return nil
}

// Balance operations

// UpdateBalance updates account balance through the orchestrator
func (o *AccountOrchestrator) UpdateBalance(ctx context.Context, userID string, currency string, amount float64, operation string) error {
	start := time.Now()
	defer func() {
		o.metrics.operationDuration.WithLabelValues("update_balance", "orchestrator").Observe(time.Since(start).Seconds())
	}()

	// Invalidate cache if enabled
	if o.hotCache != nil {
		o.hotCache.DeleteBalance(userID, currency)
	}

	// Use command handler for write operations
	if o.commandHandler != nil {
		if err := o.commandHandler.UpdateBalance(ctx, userID, currency, amount, operation); err != nil {
			o.metrics.operationsTotal.WithLabelValues("update_balance", "error", "command_handler").Inc()
			return err
		}
		o.metrics.operationsTotal.WithLabelValues("update_balance", "success", "command_handler").Inc()
		return nil
	}

	// Fallback to data manager
	if err := o.dataManager.UpdateBalance(ctx, userID, currency, amount, operation); err != nil {
		o.metrics.operationsTotal.WithLabelValues("update_balance", "error", "data_manager").Inc()
		return err
	}

	o.metrics.operationsTotal.WithLabelValues("update_balance", "success", "data_manager").Inc()
	return nil
}

// GetBalance retrieves account balance through the orchestrator
func (o *AccountOrchestrator) GetBalance(ctx context.Context, userID string, currency string) (*Balance, error) {
	start := time.Now()
	defer func() {
		o.metrics.operationDuration.WithLabelValues("get_balance", "orchestrator").Observe(time.Since(start).Seconds())
	}()

	// Try hot cache first if enabled
	if o.hotCache != nil {
		if balance, found := o.hotCache.GetBalance(userID, currency); found {
			o.metrics.operationsTotal.WithLabelValues("get_balance", "success", "hot_cache").Inc()
			return balance, nil
		}
	}

	// Use query handler for read operations
	if o.queryHandler != nil {
		balance, err := o.queryHandler.GetBalance(ctx, userID, currency)
		if err != nil {
			o.metrics.operationsTotal.WithLabelValues("get_balance", "error", "query_handler").Inc()
			return nil, err
		}

		// Cache the result if hot cache is enabled
		if o.hotCache != nil {
			o.hotCache.SetBalance(userID, currency, balance)
		}

		o.metrics.operationsTotal.WithLabelValues("get_balance", "success", "query_handler").Inc()
		return balance, nil
	}

	// Fallback to data manager
	balance, err := o.dataManager.GetBalance(ctx, userID, currency)
	if err != nil {
		o.metrics.operationsTotal.WithLabelValues("get_balance", "error", "data_manager").Inc()
		return nil, err
	}

	o.metrics.operationsTotal.WithLabelValues("get_balance", "success", "data_manager").Inc()
	return balance, nil
}

// Bulk operations

// BulkCreateAccounts creates multiple accounts through the orchestrator
func (o *AccountOrchestrator) BulkCreateAccounts(ctx context.Context, accounts []*Account) error {
	start := time.Now()
	defer func() {
		o.metrics.operationDuration.WithLabelValues("bulk_create_accounts", "orchestrator").Observe(time.Since(start).Seconds())
	}()

	// Use command handler for write operations
	if o.commandHandler != nil {
		if err := o.commandHandler.BulkCreateAccounts(ctx, accounts); err != nil {
			o.metrics.operationsTotal.WithLabelValues("bulk_create_accounts", "error", "command_handler").Inc()
			return err
		}
		o.metrics.operationsTotal.WithLabelValues("bulk_create_accounts", "success", "command_handler").Inc()
		return nil
	}

	// Fallback to data manager
	if err := o.dataManager.BulkCreateAccounts(ctx, accounts); err != nil {
		o.metrics.operationsTotal.WithLabelValues("bulk_create_accounts", "error", "data_manager").Inc()
		return err
	}

	o.metrics.operationsTotal.WithLabelValues("bulk_create_accounts", "success", "data_manager").Inc()
	return nil
}

// BulkGetAccounts retrieves multiple accounts through the orchestrator
func (o *AccountOrchestrator) BulkGetAccounts(ctx context.Context, userIDs []string) ([]*Account, error) {
	start := time.Now()
	defer func() {
		o.metrics.operationDuration.WithLabelValues("bulk_get_accounts", "orchestrator").Observe(time.Since(start).Seconds())
	}()

	// Use query handler for read operations
if o.queryHandler != nil {
		accounts, err := o.queryHandler.BulkGetAccounts(ctx, userIDs)
		if err != nil {
			o.metrics.operationsTotal.WithLabelValues("bulk_get_accounts", "error", "query_handler").Inc()
			return nil, err
		}
		o.metrics.operationsTotal.WithLabelValues("bulk_get_accounts", "success", "query_handler").Inc()
		return accounts, nil
	}

	// Fallback to data manager
	accounts, err := o.dataManager.BulkGetAccounts(ctx, userIDs)
	if err != nil {
		o.metrics.operationsTotal.WithLabelValues("bulk_get_accounts", "error", "data_manager").Inc()
		return nil, err
	}

	o.metrics.operationsTotal.WithLabelValues("bulk_get_accounts", "success", "data_manager").Inc()
	return accounts, nil
}

// Administrative operations

// RunBenchmark executes performance benchmarks
func (o *AccountOrchestrator) RunBenchmark(ctx context.Context, config BenchmarkConfig) (*BenchmarkResult, error) {
	if o.benchmarkFramework == nil {
		return nil, fmt.Errorf("benchmarking framework not available")
	}

	return o.benchmarkFramework.RunComprehensiveBenchmark(ctx, config)
}

// ExecuteMigration executes a data migration
func (o *AccountOrchestrator) ExecuteMigration(ctx context.Context, migrationConfig MigrationConfig) error {
	if o.migrationManager == nil {
		return fmt.Errorf("migration manager not available")
	}

	return o.migrationManager.ExecuteMigration(ctx, migrationConfig)
}

// GetConfiguration returns the current orchestrator configuration
func (o *AccountOrchestrator) GetConfiguration() *OrchestratorConfig {
	return o.config
}

// UpdateConfiguration updates the orchestrator configuration
func (o *AccountOrchestrator) UpdateConfiguration(config *OrchestratorConfig) error {
	o.mutex.Lock()
	defer o.mutex.Unlock()

	// Validate configuration
	if config.MaxConcurrentOperations <= 0 {
		return fmt.Errorf("max_concurrent_operations must be greater than 0")
	}
	if config.MinShards <= 0 {
		return fmt.Errorf("min_shards must be greater than 0")
	}
	if config.MaxShards < config.MinShards {
		return fmt.Errorf("max_shards must be greater than or equal to min_shards")
	}

	o.config = config
	o.logger.Info("Orchestrator configuration updated")

	return nil
}

// GetComponentStatus returns detailed status of all components
func (o *AccountOrchestrator) GetComponentStatus() map[string]interface{} {
	status := make(map[string]interface{})
	status["orchestrator"] = map[string]interface{}{
		"running": o.isRunning,
		"workers": len(o.workers),
	}

	if o.dataManager != nil {
		status["data_manager"] = o.dataManager.GetStatus()
	}

	if o.hotCache != nil {
		status["hot_cache"] = o.hotCache.GetStatus()
	}

	if o.shardManager != nil {
		status["shard_manager"] = o.shardManager.GetStatus()
	}
	if o.partitionManager != nil {
		status["partition_manager"] = map[string]interface{}{
			"active": true, // Add proper status method
		}
	}

	if o.commandHandler != nil {
		status["command_handler"] = o.commandHandler.GetStatus()
	}

	if o.queryHandler != nil {
		status["query_handler"] = o.queryHandler.GetStatus()
	}

	if o.eventStore != nil {
		status["event_store"] = o.eventStore.GetStatus()
	}

	if o.lifecycleManager != nil {
		status["lifecycle_manager"] = o.lifecycleManager.GetStatus()
	}

	if o.migrationManager != nil {
		status["migration_manager"] = o.migrationManager.GetStatus()
	}

	if o.monitoringManager != nil {
		status["monitoring_manager"] = o.monitoringManager.GetStatus()
	}

	return status
}
