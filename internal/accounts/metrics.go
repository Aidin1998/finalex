package accounts

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Ultra-high concurrency metrics for the accounts module
// Provides comprehensive monitoring for 100k+ RPS operations

var (
	// Operation counters
	accountOperationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "accounts_operations_total",
			Help: "Total number of account operations by type and status",
		},
		[]string{"operation", "status", "currency"},
	)

	// Latency histograms with fine-grained buckets for high-performance analysis
	accountOperationDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "accounts_operation_duration_seconds",
			Help: "Duration of account operations in seconds",
			Buckets: []float64{
				0.0001, 0.0005, 0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0,
			},
		},
		[]string{"operation", "currency"},
	)

	// Database connection pool metrics
	dbConnectionsActive = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "accounts_db_connections_active",
			Help: "Number of active database connections",
		},
		[]string{"database", "pool"},
	)

	dbConnectionsIdle = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "accounts_db_connections_idle",
			Help: "Number of idle database connections",
		},
		[]string{"database", "pool"},
	)

	dbConnectionsWaiting = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "accounts_db_connections_waiting",
			Help: "Number of connections waiting for a connection",
		},
		[]string{"database", "pool"},
	)

	// Cache performance metrics
	cacheOperationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "accounts_cache_operations_total",
			Help: "Total number of cache operations by type and result",
		},
		[]string{"operation", "result", "tier"},
	)

	cacheHitRatio = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "accounts_cache_hit_ratio",
			Help: "Cache hit ratio by tier",
		},
		[]string{"tier"},
	)

	cacheSize = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "accounts_cache_size_bytes",
			Help: "Cache size in bytes by tier",
		},
		[]string{"tier"},
	)

	// Distributed locking metrics
	lockOperationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "accounts_lock_operations_total",
			Help: "Total number of distributed lock operations by result",
		},
		[]string{"operation", "result"},
	)

	lockWaitDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "accounts_lock_wait_duration_seconds",
			Help: "Time spent waiting for distributed locks",
			Buckets: []float64{
				0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.0, 5.0,
			},
		},
		[]string{"lock_type"},
	)

	lockHoldDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "accounts_lock_hold_duration_seconds",
			Help: "Time spent holding distributed locks",
			Buckets: []float64{
				0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.0, 5.0,
			},
		},
		[]string{"lock_type"},
	)

	// Balance operation metrics
	balanceOperationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "accounts_balance_operations_total",
			Help: "Total number of balance operations by type and result",
		},
		[]string{"operation", "result", "currency"},
	)

	balanceAmount = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "accounts_balance_amount",
			Help:    "Balance amounts by operation type",
			Buckets: prometheus.ExponentialBuckets(0.01, 10, 10),
		},
		[]string{"operation", "currency"},
	)

	// Optimistic concurrency metrics
	concurrencyConflictsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "accounts_concurrency_conflicts_total",
			Help: "Total number of optimistic concurrency conflicts",
		},
		[]string{"operation", "conflict_type"},
	)

	retryAttemptsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "accounts_retry_attempts_total",
			Help: "Total number of retry attempts by operation",
		},
		[]string{"operation", "reason"},
	)

	// Partition metrics
	partitionOperationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "accounts_partition_operations_total",
			Help: "Total number of partition operations by type",
		},
		[]string{"partition", "operation"},
	)

	partitionSize = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "accounts_partition_size_bytes",
			Help: "Size of each partition in bytes",
		},
		[]string{"partition"},
	)

	// Background job metrics
	jobExecutionsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "accounts_job_executions_total",
			Help: "Total number of background job executions",
		},
		[]string{"job", "status"},
	)

	jobDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "accounts_job_duration_seconds",
			Help: "Duration of background job executions",
			Buckets: []float64{
				1, 5, 10, 30, 60, 120, 300, 600, 1200, 1800, 3600,
			},
		},
		[]string{"job"},
	)

	// Error metrics
	errorTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "accounts_errors_total",
			Help: "Total number of errors by type and severity",
		},
		[]string{"error_type", "severity", "operation"},
	)

	// Throughput metrics
	requestsPerSecond = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "accounts_requests_per_second",
			Help: "Current requests per second by operation type",
		},
		[]string{"operation"},
	)

	// System resource metrics
	goroutinesActive = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "accounts_goroutines_active",
			Help: "Number of active goroutines in accounts module",
		},
	)

	memoryUsage = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "accounts_memory_usage_bytes",
			Help: "Memory usage by component",
		},
		[]string{"component"},
	)
)

// MetricsCollector provides methods for recording metrics
type MetricsCollector struct {
	enabled bool
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector(enabled bool) *MetricsCollector {
	return &MetricsCollector{
		enabled: enabled,
	}
}

// RecordOperation records an account operation with timing and result
func (m *MetricsCollector) RecordOperation(operation, status, currency string, duration time.Duration) {
	if !m.enabled {
		return
	}

	accountOperationsTotal.WithLabelValues(operation, status, currency).Inc()
	accountOperationDuration.WithLabelValues(operation, currency).Observe(duration.Seconds())
}

// RecordCacheOperation records cache operation metrics
func (m *MetricsCollector) RecordCacheOperation(operation, result, tier string) {
	if !m.enabled {
		return
	}

	cacheOperationsTotal.WithLabelValues(operation, result, tier).Inc()
}

// UpdateCacheHitRatio updates cache hit ratio for a specific tier
func (m *MetricsCollector) UpdateCacheHitRatio(tier string, ratio float64) {
	if !m.enabled {
		return
	}

	cacheHitRatio.WithLabelValues(tier).Set(ratio)
}

// UpdateCacheSize updates cache size metrics
func (m *MetricsCollector) UpdateCacheSize(tier string, sizeBytes int64) {
	if !m.enabled {
		return
	}

	cacheSize.WithLabelValues(tier).Set(float64(sizeBytes))
}

// RecordLockOperation records distributed lock operation metrics
func (m *MetricsCollector) RecordLockOperation(operation, result string, waitDuration, holdDuration time.Duration, lockType string) {
	if !m.enabled {
		return
	}

	lockOperationsTotal.WithLabelValues(operation, result).Inc()

	if waitDuration > 0 {
		lockWaitDuration.WithLabelValues(lockType).Observe(waitDuration.Seconds())
	}

	if holdDuration > 0 {
		lockHoldDuration.WithLabelValues(lockType).Observe(holdDuration.Seconds())
	}
}

// RecordBalanceOperation records balance operation metrics
func (m *MetricsCollector) RecordBalanceOperation(operation, result, currency string, amount float64) {
	if !m.enabled {
		return
	}

	balanceOperationsTotal.WithLabelValues(operation, result, currency).Inc()

	if amount > 0 {
		balanceAmount.WithLabelValues(operation, currency).Observe(amount)
	}
}

// RecordConcurrencyConflict records optimistic concurrency conflicts
func (m *MetricsCollector) RecordConcurrencyConflict(operation, conflictType string) {
	if !m.enabled {
		return
	}

	concurrencyConflictsTotal.WithLabelValues(operation, conflictType).Inc()
}

// RecordRetryAttempt records retry attempts
func (m *MetricsCollector) RecordRetryAttempt(operation, reason string) {
	if !m.enabled {
		return
	}

	retryAttemptsTotal.WithLabelValues(operation, reason).Inc()
}

// UpdateConnectionPool updates database connection pool metrics
func (m *MetricsCollector) UpdateConnectionPool(database, pool string, active, idle, waiting int) {
	if !m.enabled {
		return
	}

	dbConnectionsActive.WithLabelValues(database, pool).Set(float64(active))
	dbConnectionsIdle.WithLabelValues(database, pool).Set(float64(idle))
	dbConnectionsWaiting.WithLabelValues(database, pool).Set(float64(waiting))
}

// RecordPartitionOperation records partition operation metrics
func (m *MetricsCollector) RecordPartitionOperation(partition, operation string) {
	if !m.enabled {
		return
	}

	partitionOperationsTotal.WithLabelValues(partition, operation).Inc()
}

// UpdatePartitionSize updates partition size metrics
func (m *MetricsCollector) UpdatePartitionSize(partition string, sizeBytes int64) {
	if !m.enabled {
		return
	}

	partitionSize.WithLabelValues(partition).Set(float64(sizeBytes))
}

// RecordJobExecution records background job execution metrics
func (m *MetricsCollector) RecordJobExecution(job, status string, duration time.Duration) {
	if !m.enabled {
		return
	}

	jobExecutionsTotal.WithLabelValues(job, status).Inc()
	jobDuration.WithLabelValues(job).Observe(duration.Seconds())
}

// RecordError records error metrics
func (m *MetricsCollector) RecordError(errorType, severity, operation string) {
	if !m.enabled {
		return
	}

	errorTotal.WithLabelValues(errorType, severity, operation).Inc()
}

// UpdateThroughput updates requests per second metrics
func (m *MetricsCollector) UpdateThroughput(operation string, rps float64) {
	if !m.enabled {
		return
	}

	requestsPerSecond.WithLabelValues(operation).Set(rps)
}

// UpdateSystemMetrics updates system resource metrics
func (m *MetricsCollector) UpdateSystemMetrics(goroutines int, memoryByComponent map[string]int64) {
	if !m.enabled {
		return
	}

	goroutinesActive.Set(float64(goroutines))

	for component, memory := range memoryByComponent {
		memoryUsage.WithLabelValues(component).Set(float64(memory))
	}
}

// Timer provides a convenient way to measure operation duration
type Timer struct {
	start     time.Time
	collector *MetricsCollector
	operation string
	currency  string
}

// NewTimer creates a new timer for an operation
func (m *MetricsCollector) NewTimer(operation, currency string) *Timer {
	return &Timer{
		start:     time.Now(),
		collector: m,
		operation: operation,
		currency:  currency,
	}
}

// Finish records the operation with success status
func (t *Timer) Finish() {
	duration := time.Since(t.start)
	t.collector.RecordOperation(t.operation, "success", t.currency, duration)
}

// FinishWithError records the operation with error status
func (t *Timer) FinishWithError(errorType string) {
	duration := time.Since(t.start)
	t.collector.RecordOperation(t.operation, "error", t.currency, duration)
	t.collector.RecordError(errorType, "error", t.operation)
}

// HealthChecker provides health check metrics and monitoring
type HealthChecker struct {
	collector *MetricsCollector
}

// NewHealthChecker creates a new health checker
func NewHealthChecker(collector *MetricsCollector) *HealthChecker {
	return &HealthChecker{
		collector: collector,
	}
}

// CheckHealth performs comprehensive health checks and updates metrics
func (h *HealthChecker) CheckHealth(ctx context.Context) error {
	timer := h.collector.NewTimer("health_check", "system")
	defer timer.Finish()

	// Implement health check logic here
	// This would check database connectivity, cache availability, etc.

	return nil
}

// Performance monitoring functions for high-frequency operations
func RecordBalanceQuery(currency string, duration time.Duration, cached bool) {
	labels := []string{"balance_query", "success", currency}
	accountOperationsTotal.WithLabelValues(labels...).Inc()
	accountOperationDuration.WithLabelValues("balance_query", currency).Observe(duration.Seconds())

	if cached {
		cacheOperationsTotal.WithLabelValues("get", "hit", "hot").Inc()
	} else {
		cacheOperationsTotal.WithLabelValues("get", "miss", "hot").Inc()
	}
}

func RecordBalanceUpdate(currency string, duration time.Duration, amount float64, conflicts int) {
	labels := []string{"balance_update", "success", currency}
	accountOperationsTotal.WithLabelValues(labels...).Inc()
	accountOperationDuration.WithLabelValues("balance_update", currency).Observe(duration.Seconds())
	balanceAmount.WithLabelValues("update", currency).Observe(amount)

	if conflicts > 0 {
		for i := 0; i < conflicts; i++ {
			concurrencyConflictsTotal.WithLabelValues("balance_update", "version_mismatch").Inc()
		}
	}
}

func RecordReservationOperation(operation, currency string, duration time.Duration, amount float64) {
	labels := []string{operation, "success", currency}
	accountOperationsTotal.WithLabelValues(labels...).Inc()
	accountOperationDuration.WithLabelValues(operation, currency).Observe(duration.Seconds())
	balanceAmount.WithLabelValues(operation, currency).Observe(amount)
}
