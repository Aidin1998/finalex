package transaction

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// PerformanceMetricsCollector collects and aggregates performance metrics
type PerformanceMetricsCollector struct {
	db                *gorm.DB
	metrics           *TransactionMetrics
	realTimeMetrics   *RealTimeMetrics
	historicalMetrics map[string]*HistoricalMetrics
	observers         []MetricsObserver
	mu                sync.RWMutex
	running           bool
	stopChan          chan struct{}
	aggregationTicker *time.Ticker
}

// TransactionMetrics contains comprehensive transaction performance metrics
type TransactionMetrics struct {
	// Counters
	TotalTransactions      int64
	SuccessfulTransactions int64
	FailedTransactions     int64
	TimeoutTransactions    int64
	RollbackTransactions   int64

	// Timing metrics
	AverageTransactionTime time.Duration
	MinTransactionTime     time.Duration
	MaxTransactionTime     time.Duration
	P50TransactionTime     time.Duration
	P95TransactionTime     time.Duration
	P99TransactionTime     time.Duration

	// Phase-specific timing
	AveragePrepareTime  time.Duration
	AverageCommitTime   time.Duration
	AverageRollbackTime time.Duration

	// Resource metrics
	ResourceMetrics map[string]*ResourceMetrics

	// Lock metrics
	LockMetrics *LockMetrics

	// Throughput metrics
	TransactionsPerSecond float64
	PeakTPS               float64

	// Error metrics
	ErrorRates map[string]float64

	// System health
	SystemHealth *SystemHealthMetrics

	mu sync.RWMutex
}

// ResourceMetrics contains metrics for individual resources
type ResourceMetrics struct {
	PrepareLatency    MetricStats `json:"prepare_latency"`
	CommitLatency     MetricStats `json:"commit_latency"`
	RollbackLatency   MetricStats `json:"rollback_latency"`
	SuccessRate       float64     `json:"success_rate"`
	FailureRate       float64     `json:"failure_rate"`
	TimeoutRate       float64     `json:"timeout_rate"`
	TotalOperations   int64       `json:"total_operations"`
	FailedOperations  int64       `json:"failed_operations"`
	TimeoutOperations int64       `json:"timeout_operations"`
	LastOperationTime time.Time   `json:"last_operation_time"`
}

// MetricStats contains statistical metrics
type MetricStats struct {
	Count   int64         `json:"count"`
	Sum     time.Duration `json:"sum"`
	Min     time.Duration `json:"min"`
	Max     time.Duration `json:"max"`
	Average time.Duration `json:"average"`
	P50     time.Duration `json:"p50"`
	P95     time.Duration `json:"p95"`
	P99     time.Duration `json:"p99"`
	StdDev  float64       `json:"std_dev"`
}

// LockMetrics contains distributed lock performance metrics
type LockMetrics struct {
	TotalLockAcquisitions  int64         `json:"total_lock_acquisitions"`
	SuccessfulAcquisitions int64         `json:"successful_acquisitions"`
	FailedAcquisitions     int64         `json:"failed_acquisitions"`
	AverageLockTime        time.Duration `json:"average_lock_time"`
	MaxLockTime            time.Duration `json:"max_lock_time"`
	AverageLockHoldTime    time.Duration `json:"average_lock_hold_time"`
	MaxLockHoldTime        time.Duration `json:"max_lock_hold_time"`
	DeadlockCount          int64         `json:"deadlock_count"`
	TimeoutCount           int64         `json:"timeout_count"`
	CurrentActiveLocks     int64         `json:"current_active_locks"`
	LockContentionRate     float64       `json:"lock_contention_rate"`
}

// SystemHealthMetrics contains system-wide health metrics
type SystemHealthMetrics struct {
	CPUUsage            float64   `json:"cpu_usage"`
	MemoryUsage         float64   `json:"memory_usage"`
	DiskUsage           float64   `json:"disk_usage"`
	NetworkLatency      float64   `json:"network_latency"`
	DatabaseConnections int       `json:"database_connections"`
	ActiveConnections   int       `json:"active_connections"`
	QueueDepth          int       `json:"queue_depth"`
	ErrorRate           float64   `json:"error_rate"`
	ResponseTime        float64   `json:"response_time"`
	Availability        float64   `json:"availability"`
	LastHealthCheck     time.Time `json:"last_health_check"`
}

// RealTimeMetrics contains real-time streaming metrics
type RealTimeMetrics struct {
	CurrentTPS           float64
	ActiveTransactions   int64
	PendingTransactions  int64
	QueuedRequests       int64
	CurrentLatency       time.Duration
	ErrorsInLastMinute   int64
	SuccessInLastMinute  int64
	ResourceAvailability map[string]bool
	LastUpdate           time.Time
	mu                   sync.RWMutex
}

// HistoricalMetrics contains time-series metrics
type HistoricalMetrics struct {
	Timestamp          time.Time            `json:"timestamp"`
	TimeWindow         time.Duration        `json:"time_window"`
	TransactionMetrics *TransactionMetrics  `json:"transaction_metrics"`
	SystemMetrics      *SystemHealthMetrics `json:"system_metrics"`
}

// MetricsObserver interface for metrics change notifications
type MetricsObserver interface {
	OnMetricsUpdate(metrics *TransactionMetrics) error
	OnThresholdExceeded(metricName string, value interface{}, threshold interface{}) error
	GetObserverName() string
}

// MetricsSnapshot represents a point-in-time snapshot of metrics
type MetricsSnapshot struct {
	ID                uint      `gorm:"primaryKey"`
	Timestamp         time.Time `gorm:"index"`
	MetricsJSON       string    `gorm:"type:text"`
	AggregationPeriod string    `gorm:"index"`
	Version           string
}

// TransactionTrace represents detailed tracing information for a transaction
type TransactionTrace struct {
	ID            uint   `gorm:"primaryKey"`
	TransactionID string `gorm:"index"`
	TraceID       string `gorm:"index"`
	SpanID        string
	ParentSpanID  string
	OperationType string    `gorm:"index"`
	ResourceID    string    `gorm:"index"`
	StartTime     time.Time `gorm:"index"`
	EndTime       time.Time
	Duration      int64 // microseconds
	Status        string
	ErrorMessage  string
	Metadata      string `gorm:"type:text"` // JSON metadata
}

// NewPerformanceMetricsCollector creates a new metrics collector
func NewPerformanceMetricsCollector(db *gorm.DB) *PerformanceMetricsCollector {
	// Auto-migrate metrics tables
	db.AutoMigrate(&MetricsSnapshot{}, &TransactionTrace{})

	return &PerformanceMetricsCollector{
		db:                db,
		metrics:           NewTransactionMetrics(),
		realTimeMetrics:   NewRealTimeMetrics(),
		historicalMetrics: make(map[string]*HistoricalMetrics),
		observers:         make([]MetricsObserver, 0),
		stopChan:          make(chan struct{}),
	}
}

// NewTransactionMetrics creates a new transaction metrics instance
func NewTransactionMetrics() *TransactionMetrics {
	return &TransactionMetrics{
		ResourceMetrics:    make(map[string]*ResourceMetrics),
		ErrorRates:         make(map[string]float64),
		LockMetrics:        &LockMetrics{},
		SystemHealth:       &SystemHealthMetrics{},
		MinTransactionTime: time.Hour, // Initialize to a high value
	}
}

// NewRealTimeMetrics creates a new real-time metrics instance
func NewRealTimeMetrics() *RealTimeMetrics {
	return &RealTimeMetrics{
		ResourceAvailability: make(map[string]bool),
		LastUpdate:           time.Now(),
	}
}

// Start begins metrics collection
func (pmc *PerformanceMetricsCollector) Start(ctx context.Context) error {
	pmc.mu.Lock()
	defer pmc.mu.Unlock()

	if pmc.running {
		return fmt.Errorf("metrics collector is already running")
	}

	pmc.running = true
	pmc.aggregationTicker = time.NewTicker(1 * time.Minute)

	// Start collection loops
	go pmc.metricsAggregationLoop(ctx)
	go pmc.realTimeMetricsLoop(ctx)
	go pmc.historicalDataLoop(ctx)
	go pmc.cleanupLoop(ctx)

	log.Println("Performance metrics collector started")
	return nil
}

// Stop stops metrics collection
func (pmc *PerformanceMetricsCollector) Stop() {
	pmc.mu.Lock()
	defer pmc.mu.Unlock()

	if !pmc.running {
		return
	}

	if pmc.aggregationTicker != nil {
		pmc.aggregationTicker.Stop()
	}

	close(pmc.stopChan)
	pmc.running = false
	log.Println("Performance metrics collector stopped")
}

// RecordTransactionStart records the start of a transaction
func (pmc *PerformanceMetricsCollector) RecordTransactionStart(txnID string, resources []string) {
	atomic.AddInt64(&pmc.realTimeMetrics.ActiveTransactions, 1)
	atomic.AddInt64(&pmc.metrics.TotalTransactions, 1)

	// Create trace entry
	trace := &TransactionTrace{
		TransactionID: txnID,
		TraceID:       uuid.New().String(),
		SpanID:        uuid.New().String(),
		OperationType: "transaction_start",
		StartTime:     time.Now(),
		Status:        "active",
		Metadata:      fmt.Sprintf(`{"resources": %v}`, resources),
	}

	pmc.db.Create(trace)
}

// RecordTransactionEnd records the completion of a transaction
func (pmc *PerformanceMetricsCollector) RecordTransactionEnd(txnID string, success bool, duration time.Duration, errorMsg string) {
	atomic.AddInt64(&pmc.realTimeMetrics.ActiveTransactions, -1)

	if success {
		atomic.AddInt64(&pmc.metrics.SuccessfulTransactions, 1)
		atomic.AddInt64(&pmc.realTimeMetrics.SuccessInLastMinute, 1)
	} else {
		atomic.AddInt64(&pmc.metrics.FailedTransactions, 1)
		atomic.AddInt64(&pmc.realTimeMetrics.ErrorsInLastMinute, 1)
	}

	// Update timing metrics
	pmc.updateTimingMetrics(duration)

	// Update trace
	status := "success"
	if !success {
		status = "failed"
	}

	trace := &TransactionTrace{
		TransactionID: txnID,
		TraceID:       uuid.New().String(),
		SpanID:        uuid.New().String(),
		OperationType: "transaction_end",
		StartTime:     time.Now().Add(-duration),
		EndTime:       time.Now(),
		Duration:      duration.Microseconds(),
		Status:        status,
		ErrorMessage:  errorMsg,
	}

	pmc.db.Create(trace)
}

// RecordResourceOperation records a resource operation
func (pmc *PerformanceMetricsCollector) RecordResourceOperation(resourceID, operation string, duration time.Duration, success bool) {
	pmc.metrics.mu.Lock()
	defer pmc.metrics.mu.Unlock()

	resourceMetrics, exists := pmc.metrics.ResourceMetrics[resourceID]
	if !exists {
		resourceMetrics = &ResourceMetrics{
			PrepareLatency:    MetricStats{Min: time.Hour},
			CommitLatency:     MetricStats{Min: time.Hour},
			RollbackLatency:   MetricStats{Min: time.Hour},
			LastOperationTime: time.Now(),
		}
		pmc.metrics.ResourceMetrics[resourceID] = resourceMetrics
	}

	// Update operation-specific metrics
	var stats *MetricStats
	switch operation {
	case "prepare":
		stats = &resourceMetrics.PrepareLatency
	case "commit":
		stats = &resourceMetrics.CommitLatency
	case "rollback":
		stats = &resourceMetrics.RollbackLatency
	}

	if stats != nil {
		pmc.updateMetricStats(stats, duration)
	}

	// Update counters
	resourceMetrics.TotalOperations++
	if !success {
		resourceMetrics.FailedOperations++
	}
	resourceMetrics.LastOperationTime = time.Now()

	// Calculate rates
	if resourceMetrics.TotalOperations > 0 {
		resourceMetrics.SuccessRate = float64(resourceMetrics.TotalOperations-resourceMetrics.FailedOperations) / float64(resourceMetrics.TotalOperations)
		resourceMetrics.FailureRate = float64(resourceMetrics.FailedOperations) / float64(resourceMetrics.TotalOperations)
	}
}

// RecordLockOperation records a lock operation
func (pmc *PerformanceMetricsCollector) RecordLockOperation(operation string, duration time.Duration, success bool) {
	pmc.metrics.mu.Lock()
	defer pmc.metrics.mu.Unlock()

	lockMetrics := pmc.metrics.LockMetrics

	switch operation {
	case "acquire":
		lockMetrics.TotalLockAcquisitions++
		if success {
			lockMetrics.SuccessfulAcquisitions++
			pmc.updateDurationMetric(&lockMetrics.AverageLockTime, duration, lockMetrics.SuccessfulAcquisitions)
			if duration > lockMetrics.MaxLockTime {
				lockMetrics.MaxLockTime = duration
			}
		} else {
			lockMetrics.FailedAcquisitions++
		}

	case "hold":
		pmc.updateDurationMetric(&lockMetrics.AverageLockHoldTime, duration, lockMetrics.SuccessfulAcquisitions)
		if duration > lockMetrics.MaxLockHoldTime {
			lockMetrics.MaxLockHoldTime = duration
		}

	case "deadlock":
		lockMetrics.DeadlockCount++

	case "timeout":
		lockMetrics.TimeoutCount++
	}

	// Calculate contention rate
	if lockMetrics.TotalLockAcquisitions > 0 {
		lockMetrics.LockContentionRate = float64(lockMetrics.FailedAcquisitions) / float64(lockMetrics.TotalLockAcquisitions)
	}
}

// updateTimingMetrics updates transaction timing metrics
func (pmc *PerformanceMetricsCollector) updateTimingMetrics(duration time.Duration) {
	pmc.metrics.mu.Lock()
	defer pmc.metrics.mu.Unlock()

	totalTxns := atomic.LoadInt64(&pmc.metrics.TotalTransactions)

	// Update average
	if totalTxns > 1 {
		prevAvg := pmc.metrics.AverageTransactionTime
		pmc.metrics.AverageTransactionTime = time.Duration((int64(prevAvg)*(totalTxns-1) + int64(duration)) / totalTxns)
	} else {
		pmc.metrics.AverageTransactionTime = duration
	}

	// Update min/max
	if duration < pmc.metrics.MinTransactionTime {
		pmc.metrics.MinTransactionTime = duration
	}
	if duration > pmc.metrics.MaxTransactionTime {
		pmc.metrics.MaxTransactionTime = duration
	}

	// Update current latency for real-time metrics
	pmc.realTimeMetrics.mu.Lock()
	pmc.realTimeMetrics.CurrentLatency = duration
	pmc.realTimeMetrics.LastUpdate = time.Now()
	pmc.realTimeMetrics.mu.Unlock()
}

// updateMetricStats updates statistical metrics for a metric
func (pmc *PerformanceMetricsCollector) updateMetricStats(stats *MetricStats, value time.Duration) {
	stats.Count++
	stats.Sum += value

	if value < stats.Min {
		stats.Min = value
	}
	if value > stats.Max {
		stats.Max = value
	}

	if stats.Count > 0 {
		stats.Average = time.Duration(int64(stats.Sum) / stats.Count)
	}

	// For percentiles, we would typically maintain a histogram or use a streaming algorithm
	// For simplicity, we'll approximate based on min/max/average
	stats.P50 = stats.Average
	stats.P95 = time.Duration(int64(stats.Average) + (int64(stats.Max-stats.Average) * 75 / 100))
	stats.P99 = time.Duration(int64(stats.Average) + (int64(stats.Max-stats.Average) * 95 / 100))
}

// updateDurationMetric updates a duration metric with moving average
func (pmc *PerformanceMetricsCollector) updateDurationMetric(current *time.Duration, newValue time.Duration, count int64) {
	if count <= 1 {
		*current = newValue
	} else {
		*current = time.Duration((int64(*current)*(count-1) + int64(newValue)) / count)
	}
}

// metricsAggregationLoop runs the metrics aggregation process
func (pmc *PerformanceMetricsCollector) metricsAggregationLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-pmc.stopChan:
			return
		case <-pmc.aggregationTicker.C:
			pmc.aggregateMetrics()
		}
	}
}

// aggregateMetrics performs periodic metrics aggregation
func (pmc *PerformanceMetricsCollector) aggregateMetrics() {
	// Calculate TPS
	now := time.Now()
	if lastUpdate := pmc.realTimeMetrics.LastUpdate; !lastUpdate.IsZero() {
		duration := now.Sub(lastUpdate).Seconds()
		if duration > 0 {
			successCount := atomic.SwapInt64(&pmc.realTimeMetrics.SuccessInLastMinute, 0)
			currentTPS := float64(successCount) / duration

			pmc.realTimeMetrics.mu.Lock()
			pmc.realTimeMetrics.CurrentTPS = currentTPS
			if currentTPS > pmc.metrics.PeakTPS {
				pmc.metrics.PeakTPS = currentTPS
			}
			pmc.realTimeMetrics.mu.Unlock()
		}
	}

	// Update TPS in main metrics
	pmc.metrics.mu.Lock()
	pmc.metrics.TransactionsPerSecond = pmc.realTimeMetrics.CurrentTPS
	pmc.metrics.mu.Unlock()

	// Notify observers
	pmc.notifyObservers()

	// Save snapshot
	pmc.saveMetricsSnapshot("1m")
}

// realTimeMetricsLoop updates real-time metrics
func (pmc *PerformanceMetricsCollector) realTimeMetricsLoop(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-pmc.stopChan:
			return
		case <-ticker.C:
			pmc.updateRealTimeMetrics()
		}
	}
}

// updateRealTimeMetrics updates real-time system metrics
func (pmc *PerformanceMetricsCollector) updateRealTimeMetrics() {
	pmc.realTimeMetrics.mu.Lock()
	defer pmc.realTimeMetrics.mu.Unlock()

	// Update pending transactions (would query from database)
	var pendingCount int64
	pmc.db.Model(&XATransaction{}).Where("state IN ?", []XAState{XAStatePreparing, XAStatePrepared}).Count(&pendingCount)
	pmc.realTimeMetrics.PendingTransactions = pendingCount

	// Update system health metrics
	pmc.updateSystemHealthMetrics()

	pmc.realTimeMetrics.LastUpdate = time.Now()
}

// updateSystemHealthMetrics updates system health metrics
func (pmc *PerformanceMetricsCollector) updateSystemHealthMetrics() {
	// This would typically integrate with system monitoring tools
	// For now, we'll simulate some basic metrics

	pmc.metrics.SystemHealth.LastHealthCheck = time.Now()
	pmc.metrics.SystemHealth.Availability = 99.9 // Simulated availability

	// In a real implementation, these would be gathered from system APIs
	// pmc.metrics.SystemHealth.CPUUsage = getCPUUsage()
	// pmc.metrics.SystemHealth.MemoryUsage = getMemoryUsage()
	// pmc.metrics.SystemHealth.DiskUsage = getDiskUsage()
}

// historicalDataLoop manages historical metrics storage
func (pmc *PerformanceMetricsCollector) historicalDataLoop(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-pmc.stopChan:
			return
		case <-ticker.C:
			pmc.saveHistoricalMetrics("15m")
		}
	}
}

// saveHistoricalMetrics saves historical metrics data
func (pmc *PerformanceMetricsCollector) saveHistoricalMetrics(period string) {
	historical := &HistoricalMetrics{
		Timestamp:          time.Now(),
		TimeWindow:         15 * time.Minute,
		TransactionMetrics: pmc.GetMetricsSnapshot(),
		SystemMetrics:      pmc.metrics.SystemHealth,
	}

	pmc.mu.Lock()
	pmc.historicalMetrics[period] = historical
	pmc.mu.Unlock()

	// Save to database
	pmc.saveMetricsSnapshot(period)
}

// cleanupLoop cleans up old metrics data
func (pmc *PerformanceMetricsCollector) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-pmc.stopChan:
			return
		case <-ticker.C:
			pmc.cleanupOldData()
		}
	}
}

// cleanupOldData removes old metrics data
func (pmc *PerformanceMetricsCollector) cleanupOldData() {
	// Remove metrics snapshots older than 7 days
	cutoff := time.Now().Add(-7 * 24 * time.Hour)
	pmc.db.Where("timestamp < ?", cutoff).Delete(&MetricsSnapshot{})

	// Remove transaction traces older than 24 hours
	traceCutoff := time.Now().Add(-24 * time.Hour)
	pmc.db.Where("start_time < ?", traceCutoff).Delete(&TransactionTrace{})

	log.Println("Cleaned up old metrics data")
}

// saveMetricsSnapshot saves a metrics snapshot to database
func (pmc *PerformanceMetricsCollector) saveMetricsSnapshot(period string) {
	metricsJSON, err := json.Marshal(pmc.metrics)
	if err != nil {
		log.Printf("Error marshaling metrics: %v", err)
		return
	}

	snapshot := &MetricsSnapshot{
		Timestamp:         time.Now(),
		MetricsJSON:       string(metricsJSON),
		AggregationPeriod: period,
		Version:           "1.0",
	}

	if err := pmc.db.Create(snapshot).Error; err != nil {
		log.Printf("Error saving metrics snapshot: %v", err)
	}
}

// notifyObservers notifies all registered observers
func (pmc *PerformanceMetricsCollector) notifyObservers() {
	metrics := pmc.GetMetricsSnapshot()

	for _, observer := range pmc.observers {
		go func(obs MetricsObserver) {
			if err := obs.OnMetricsUpdate(metrics); err != nil {
				log.Printf("Error notifying observer %s: %v", obs.GetObserverName(), err)
			}
		}(observer)
	}
}

// RegisterObserver registers a metrics observer
func (pmc *PerformanceMetricsCollector) RegisterObserver(observer MetricsObserver) {
	pmc.mu.Lock()
	defer pmc.mu.Unlock()
	pmc.observers = append(pmc.observers, observer)
}

// GetMetricsSnapshot returns a snapshot of current metrics
func (pmc *PerformanceMetricsCollector) GetMetricsSnapshot() *TransactionMetrics {
	pmc.metrics.mu.RLock()
	defer pmc.metrics.mu.RUnlock()

	// Create a deep copy
	metricsJSON, _ := json.Marshal(pmc.metrics)
	var snapshot TransactionMetrics
	json.Unmarshal(metricsJSON, &snapshot)

	return &snapshot
}

// GetRealTimeMetrics returns current real-time metrics
func (pmc *PerformanceMetricsCollector) GetRealTimeMetrics() *RealTimeMetrics {
	pmc.realTimeMetrics.mu.RLock()
	defer pmc.realTimeMetrics.mu.RUnlock()

	// Create a copy
	metricsJSON, _ := json.Marshal(pmc.realTimeMetrics)
	var snapshot RealTimeMetrics
	json.Unmarshal(metricsJSON, &snapshot)

	return &snapshot
}

// GetHistoricalMetrics returns historical metrics for a time period
func (pmc *PerformanceMetricsCollector) GetHistoricalMetrics(period string, limit int) ([]*HistoricalMetrics, error) {
	var snapshots []*MetricsSnapshot

	query := pmc.db.Where("aggregation_period = ?", period).Order("timestamp DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Find(&snapshots).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get historical metrics: %w", err)
	}

	historical := make([]*HistoricalMetrics, len(snapshots))
	for i, snapshot := range snapshots {
		var metrics TransactionMetrics
		if err := json.Unmarshal([]byte(snapshot.MetricsJSON), &metrics); err != nil {
			continue
		}

		historical[i] = &HistoricalMetrics{
			Timestamp:          snapshot.Timestamp,
			TransactionMetrics: &metrics,
		}
	}

	return historical, nil
}

// GetTransactionTrace returns trace information for a transaction
func (pmc *PerformanceMetricsCollector) GetTransactionTrace(txnID string) ([]*TransactionTrace, error) {
	var traces []*TransactionTrace

	err := pmc.db.Where("transaction_id = ?", txnID).
		Order("start_time ASC").
		Find(&traces).Error

	if err != nil {
		return nil, fmt.Errorf("failed to get transaction trace: %w", err)
	}

	return traces, nil
}

// GetResourceMetrics returns metrics for a specific resource
func (pmc *PerformanceMetricsCollector) GetResourceMetrics(resourceID string) (*ResourceMetrics, error) {
	pmc.metrics.mu.RLock()
	defer pmc.metrics.mu.RUnlock()

	metrics, exists := pmc.metrics.ResourceMetrics[resourceID]
	if !exists {
		return nil, fmt.Errorf("no metrics found for resource %s", resourceID)
	}

	// Return a copy
	metricsJSON, _ := json.Marshal(metrics)
	var copy ResourceMetrics
	json.Unmarshal(metricsJSON, &copy)

	return &copy, nil
}

// ExportMetrics exports metrics in various formats
func (pmc *PerformanceMetricsCollector) ExportMetrics(format string) ([]byte, error) {
	metrics := pmc.GetMetricsSnapshot()

	switch format {
	case "json":
		return json.MarshalIndent(metrics, "", "  ")
	case "prometheus":
		return pmc.exportPrometheusMetrics(metrics), nil
	default:
		return nil, fmt.Errorf("unsupported export format: %s", format)
	}
}

// exportPrometheusMetrics exports metrics in Prometheus format
func (pmc *PerformanceMetricsCollector) exportPrometheusMetrics(metrics *TransactionMetrics) []byte {
	var output []byte

	// Add basic counters
	output = append(output, []byte(fmt.Sprintf("# HELP transaction_total Total number of transactions\n"))...)
	output = append(output, []byte(fmt.Sprintf("# TYPE transaction_total counter\n"))...)
	output = append(output, []byte(fmt.Sprintf("transaction_total %d\n", metrics.TotalTransactions))...)

	output = append(output, []byte(fmt.Sprintf("# HELP transaction_successful_total Total number of successful transactions\n"))...)
	output = append(output, []byte(fmt.Sprintf("# TYPE transaction_successful_total counter\n"))...)
	output = append(output, []byte(fmt.Sprintf("transaction_successful_total %d\n", metrics.SuccessfulTransactions))...)

	output = append(output, []byte(fmt.Sprintf("# HELP transaction_failed_total Total number of failed transactions\n"))...)
	output = append(output, []byte(fmt.Sprintf("# TYPE transaction_failed_total counter\n"))...)
	output = append(output, []byte(fmt.Sprintf("transaction_failed_total %d\n", metrics.FailedTransactions))...)

	// Add timing metrics
	output = append(output, []byte(fmt.Sprintf("# HELP transaction_duration_seconds Average transaction duration\n"))...)
	output = append(output, []byte(fmt.Sprintf("# TYPE transaction_duration_seconds gauge\n"))...)
	output = append(output, []byte(fmt.Sprintf("transaction_duration_seconds %.6f\n", metrics.AverageTransactionTime.Seconds()))...)

	// Add TPS metrics
	output = append(output, []byte(fmt.Sprintf("# HELP transaction_rate_per_second Current transaction rate\n"))...)
	output = append(output, []byte(fmt.Sprintf("# TYPE transaction_rate_per_second gauge\n"))...)
	output = append(output, []byte(fmt.Sprintf("transaction_rate_per_second %.2f\n", metrics.TransactionsPerSecond))...)

	return output
}

// Example metrics observer implementation
type AlertingMetricsObserver struct {
	alertManager *AlertManager
	thresholds   map[string]interface{}
}

func (o *AlertingMetricsObserver) OnMetricsUpdate(metrics *TransactionMetrics) error {
	// Check various thresholds and trigger alerts
	if metrics.TransactionsPerSecond > 1000 {
		alert := &Alert{
			ID:       uuid.New().String(),
			Type:     AlertTypePerformance,
			Severity: SeverityHigh,
			Title:    "High Transaction Rate",
			Message:  fmt.Sprintf("TPS (%.2f) exceeds threshold (1000)", metrics.TransactionsPerSecond),
			Data: map[string]interface{}{
				"current_tps": metrics.TransactionsPerSecond,
				"threshold":   1000,
			},
			CreatedAt: time.Now(),
		}

		o.alertManager.TriggerAlert(context.Background(), alert)
	}

	return nil
}

func (o *AlertingMetricsObserver) OnThresholdExceeded(metricName string, value interface{}, threshold interface{}) error {
	// Handle threshold violations
	return nil
}

func (o *AlertingMetricsObserver) GetObserverName() string {
	return "AlertingMetricsObserver"
}
