// =============================
// Engine Performance Monitoring System
// =============================
// This file implements comprehensive performance monitoring for the adaptive
// trading engine with real-time analysis and alerting capabilities.

package engine

import (
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// EnginePerformanceMonitor monitors overall engine performance
type EnginePerformanceMonitor struct {
	// Configuration
	monitoringInterval time.Duration
	alertThresholds    *PerformanceThresholds
	logger             *zap.SugaredLogger

	// Performance metrics (atomic for thread safety)
	totalOrdersProcessed  int64
	totalTradesExecuted   int64
	totalProcessingTimeNs int64
	totalErrors           int64

	// Throughput tracking
	throughputWindow    int64 // Operations in current window
	lastThroughputCheck int64 // Unix timestamp
	avgThroughputOps    int64 // Moving average throughput (ops * 1000)
	peakThroughputOps   int64 // Peak throughput observed

	// Latency tracking
	totalLatencyMeasurements int64
	latencySum               int64 // Sum of all latencies in nanoseconds
	minLatencyNs             int64 // Minimum latency observed
	maxLatencyNs             int64 // Maximum latency observed

	// System resource monitoring
	memoryUsageBytes     int64
	peakMemoryUsageBytes int64
	goroutineCount       int64
	gcPauseTimeNs        int64
	lastGCTime           int64

	// Performance degradation tracking
	performanceDegradations int64
	lastDegradationTime     int64
	consecutiveDegradations int64

	// Alert system
	alertCallback   func(AlertType, AlertSeverity, string)
	alertHistory    []PerformanceAlert
	alertHistoryMu  sync.RWMutex
	maxAlertHistory int

	// Control
	stopChan  chan struct{}
	running   int32 // atomic
	startTime time.Time
}

// PerformanceThresholds defines thresholds for performance monitoring
type PerformanceThresholds struct {
	// Latency thresholds (nanoseconds)
	MaxAvgLatencyNs int64 `json:"max_avg_latency_ns"`
	MaxP95LatencyNs int64 `json:"max_p95_latency_ns"`
	MaxP99LatencyNs int64 `json:"max_p99_latency_ns"`

	// Throughput thresholds
	MinThroughputOps         float64 `json:"min_throughput_ops"`
	ThroughputDegradationPct float64 `json:"throughput_degradation_pct"`

	// Error rate thresholds
	MaxErrorRatePct      float64 `json:"max_error_rate_pct"`
	MaxConsecutiveErrors int64   `json:"max_consecutive_errors"`

	// Resource usage thresholds
	MaxMemoryUsageMB  int64 `json:"max_memory_usage_mb"`
	MaxGoroutineCount int64 `json:"max_goroutine_count"`
	MaxGCPauseMs      int64 `json:"max_gc_pause_ms"`

	// Performance degradation thresholds
	MaxConsecutiveDegradations int64 `json:"max_consecutive_degradations"`
	DegradationTimeWindowSec   int64 `json:"degradation_time_window_sec"`
}

// AlertType represents the type of performance alert
type AlertType string

const (
	AlertTypeLatency                AlertType = "LATENCY"
	AlertTypeThroughput             AlertType = "THROUGHPUT"
	AlertTypeErrorRate              AlertType = "ERROR_RATE"
	AlertTypeMemoryUsage            AlertType = "MEMORY_USAGE"
	AlertTypeGoroutineCount         AlertType = "GOROUTINE_COUNT"
	AlertTypeGCPause                AlertType = "GC_PAUSE"
	AlertTypePerformanceDegradation AlertType = "PERFORMANCE_DEGRADATION"
)

// AlertSeverity represents the severity of an alert
type AlertSeverity string

const (
	AlertSeverityInfo     AlertSeverity = "INFO"
	AlertSeverityWarning  AlertSeverity = "WARNING"
	AlertSeverityCritical AlertSeverity = "CRITICAL"
)

// PerformanceAlert represents a performance alert
type PerformanceAlert struct {
	Type       AlertType     `json:"type"`
	Severity   AlertSeverity `json:"severity"`
	Message    string        `json:"message"`
	Timestamp  time.Time     `json:"timestamp"`
	Value      float64       `json:"value"`
	Threshold  float64       `json:"threshold"`
	Resolution string        `json:"resolution,omitempty"`
}

// EnginePerformanceMetrics represents comprehensive engine performance metrics
type EnginePerformanceMetrics struct {
	// Basic metrics
	TotalOrdersProcessed int64   `json:"total_orders_processed"`
	TotalTradesExecuted  int64   `json:"total_trades_executed"`
	TotalErrors          int64   `json:"total_errors"`
	AvgProcessingTimeMs  float64 `json:"avg_processing_time_ms"`
	ThroughputOpsPerSec  float64 `json:"throughput_ops_per_sec"`
	ErrorRate            float64 `json:"error_rate"`

	// Latency metrics
	AvgLatencyMs float64 `json:"avg_latency_ms"`
	MinLatencyMs float64 `json:"min_latency_ms"`
	MaxLatencyMs float64 `json:"max_latency_ms"`

	// Resource metrics
	MemoryUsageMB     float64 `json:"memory_usage_mb"`
	PeakMemoryUsageMB float64 `json:"peak_memory_usage_mb"`
	GoroutineCount    int64   `json:"goroutine_count"`
	LastGCPauseMs     float64 `json:"last_gc_pause_ms"`

	// Performance health
	PerformanceDegradations int64         `json:"performance_degradations"`
	ConsecutiveDegradations int64         `json:"consecutive_degradations"`
	UpTime                  time.Duration `json:"uptime"`
	LastUpdateTime          time.Time     `json:"last_update_time"`
}

// NewEnginePerformanceMonitor creates a new performance monitor
func NewEnginePerformanceMonitor() *EnginePerformanceMonitor {
	return &EnginePerformanceMonitor{
		monitoringInterval:  time.Second * 5, // Monitor every 5 seconds
		stopChan:            make(chan struct{}),
		maxAlertHistory:     100,
		startTime:           time.Now(),
		lastThroughputCheck: time.Now().Unix(),
		minLatencyNs:        int64(^uint64(0) >> 1), // Max int64
		alertThresholds:     DefaultPerformanceThresholds(),
	}
}

// DefaultPerformanceThresholds returns default performance thresholds
func DefaultPerformanceThresholds() *PerformanceThresholds {
	return &PerformanceThresholds{
		MaxAvgLatencyNs:            10 * time.Millisecond.Nanoseconds(),  // 10ms
		MaxP95LatencyNs:            50 * time.Millisecond.Nanoseconds(),  // 50ms
		MaxP99LatencyNs:            100 * time.Millisecond.Nanoseconds(), // 100ms
		MinThroughputOps:           1000.0,                               // 1000 ops/sec
		ThroughputDegradationPct:   30.0,                                 // 30% degradation
		MaxErrorRatePct:            2.0,                                  // 2% error rate
		MaxConsecutiveErrors:       10,
		MaxMemoryUsageMB:           1024, // 1GB
		MaxGoroutineCount:          1000,
		MaxGCPauseMs:               10, // 10ms
		MaxConsecutiveDegradations: 3,
		DegradationTimeWindowSec:   300, // 5 minutes
	}
}

// Start begins performance monitoring
func (epm *EnginePerformanceMonitor) Start() {
	if !atomic.CompareAndSwapInt32(&epm.running, 0, 1) {
		return // Already running
	}

	epm.startTime = time.Now()
	go epm.monitoringWorker()
}

// Stop stops performance monitoring
func (epm *EnginePerformanceMonitor) Stop() {
	if !atomic.CompareAndSwapInt32(&epm.running, 1, 0) {
		return // Not running
	}

	close(epm.stopChan)
}

// Configuration methods

// SetAlertCallback sets the callback for performance alerts
func (epm *EnginePerformanceMonitor) SetAlertCallback(callback func(AlertType, AlertSeverity, string)) {
	epm.alertCallback = callback
}

// SetThresholds updates performance thresholds
func (epm *EnginePerformanceMonitor) SetThresholds(thresholds *PerformanceThresholds) {
	epm.alertThresholds = thresholds
}

// SetLogger sets the logger for performance monitoring
func (epm *EnginePerformanceMonitor) SetLogger(logger *zap.SugaredLogger) {
	epm.logger = logger
}

// Recording methods

// RecordOrderProcessing records metrics for order processing
func (epm *EnginePerformanceMonitor) RecordOrderProcessing(duration time.Duration, success bool) {
	atomic.AddInt64(&epm.totalOrdersProcessed, 1)
	atomic.AddInt64(&epm.throughputWindow, 1)

	latencyNs := duration.Nanoseconds()
	atomic.AddInt64(&epm.totalProcessingTimeNs, latencyNs)
	atomic.AddInt64(&epm.totalLatencyMeasurements, 1)
	atomic.AddInt64(&epm.latencySum, latencyNs)

	// Update min/max latency
	epm.updateMinLatency(latencyNs)
	epm.updateMaxLatency(latencyNs)

	if !success {
		atomic.AddInt64(&epm.totalErrors, 1)
	}
}

// RecordTradeExecution records trade execution metrics
func (epm *EnginePerformanceMonitor) RecordTradeExecution(count int) {
	atomic.AddInt64(&epm.totalTradesExecuted, int64(count))
}

// RecordPerformanceDegradation records a performance degradation event
func (epm *EnginePerformanceMonitor) RecordPerformanceDegradation() {
	now := time.Now().Unix()
	lastDegradation := atomic.LoadInt64(&epm.lastDegradationTime)

	atomic.AddInt64(&epm.performanceDegradations, 1)
	atomic.StoreInt64(&epm.lastDegradationTime, now)

	// Check if this is consecutive
	windowSec := epm.alertThresholds.DegradationTimeWindowSec
	if now-lastDegradation <= windowSec {
		consecutive := atomic.AddInt64(&epm.consecutiveDegradations, 1)

		if consecutive >= epm.alertThresholds.MaxConsecutiveDegradations {
			epm.triggerAlert(AlertTypePerformanceDegradation, AlertSeverityCritical,
				"Consecutive performance degradations detected")
		}
	} else {
		atomic.StoreInt64(&epm.consecutiveDegradations, 1)
	}
}

// Helper methods for atomic min/max updates

// updateMinLatency atomically updates minimum latency
func (epm *EnginePerformanceMonitor) updateMinLatency(latencyNs int64) {
	for {
		current := atomic.LoadInt64(&epm.minLatencyNs)
		if latencyNs >= current || atomic.CompareAndSwapInt64(&epm.minLatencyNs, current, latencyNs) {
			break
		}
	}
}

// updateMaxLatency atomically updates maximum latency
func (epm *EnginePerformanceMonitor) updateMaxLatency(latencyNs int64) {
	for {
		current := atomic.LoadInt64(&epm.maxLatencyNs)
		if latencyNs <= current || atomic.CompareAndSwapInt64(&epm.maxLatencyNs, current, latencyNs) {
			break
		}
	}
}

// Monitoring worker

// monitoringWorker runs the main monitoring loop
func (epm *EnginePerformanceMonitor) monitoringWorker() {
	ticker := time.NewTicker(epm.monitoringInterval)
	defer ticker.Stop()

	for {
		select {
		case <-epm.stopChan:
			return
		case <-ticker.C:
			epm.collectSystemMetrics()
			epm.updateThroughputMetrics()
			epm.checkPerformanceThresholds()
		}
	}
}

// collectSystemMetrics collects system-level metrics
func (epm *EnginePerformanceMonitor) collectSystemMetrics() {
	// Collect memory statistics
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	memoryBytes := int64(m.Alloc)
	atomic.StoreInt64(&epm.memoryUsageBytes, memoryBytes)

	// Update peak memory usage
	for {
		current := atomic.LoadInt64(&epm.peakMemoryUsageBytes)
		if memoryBytes <= current || atomic.CompareAndSwapInt64(&epm.peakMemoryUsageBytes, current, memoryBytes) {
			break
		}
	}

	// Collect goroutine count
	goroutines := int64(runtime.NumGoroutine())
	atomic.StoreInt64(&epm.goroutineCount, goroutines)

	// Collect GC statistics
	if m.NumGC > 0 {
		// Get most recent GC pause time
		pauseTime := int64(m.PauseNs[(m.NumGC+255)%256])
		atomic.StoreInt64(&epm.gcPauseTimeNs, pauseTime)
		atomic.StoreInt64(&epm.lastGCTime, time.Now().Unix())
	}
}

// updateThroughputMetrics updates throughput calculations
func (epm *EnginePerformanceMonitor) updateThroughputMetrics() {
	now := time.Now().Unix()
	lastCheck := atomic.LoadInt64(&epm.lastThroughputCheck)

	if now > lastCheck {
		windowOps := atomic.SwapInt64(&epm.throughputWindow, 0)
		timeDiff := now - lastCheck

		if timeDiff > 0 {
			// Calculate current throughput (ops per second)
			currentThroughput := float64(windowOps) / float64(timeDiff)

			// Update moving average (exponential moving average with α = 0.3)
			currentAvg := float64(atomic.LoadInt64(&epm.avgThroughputOps)) / 1000.0
			if currentAvg == 0 {
				currentAvg = currentThroughput
			} else {
				currentAvg = currentAvg*0.7 + currentThroughput*0.3
			}

			atomic.StoreInt64(&epm.avgThroughputOps, int64(currentAvg*1000))

			// Update peak throughput
			peakOps := int64(currentThroughput * 1000)
			for {
				current := atomic.LoadInt64(&epm.peakThroughputOps)
				if peakOps <= current || atomic.CompareAndSwapInt64(&epm.peakThroughputOps, current, peakOps) {
					break
				}
			}
		}

		atomic.StoreInt64(&epm.lastThroughputCheck, now)
	}
}

// checkPerformanceThresholds checks all performance thresholds and triggers alerts
func (epm *EnginePerformanceMonitor) checkPerformanceThresholds() {
	if epm.alertThresholds == nil {
		return
	}

	thresholds := epm.alertThresholds

	// Check latency thresholds
	epm.checkLatencyThresholds(thresholds)

	// Check throughput thresholds
	epm.checkThroughputThresholds(thresholds)

	// Check error rate thresholds
	epm.checkErrorRateThresholds(thresholds)

	// Check resource usage thresholds
	epm.checkResourceThresholds(thresholds)
}

// checkLatencyThresholds checks latency-related thresholds
func (epm *EnginePerformanceMonitor) checkLatencyThresholds(thresholds *PerformanceThresholds) {
	measurements := atomic.LoadInt64(&epm.totalLatencyMeasurements)
	if measurements == 0 {
		return
	}

	// Check average latency
	avgLatencyNs := atomic.LoadInt64(&epm.latencySum) / measurements
	if avgLatencyNs > thresholds.MaxAvgLatencyNs {
		epm.triggerAlert(AlertTypeLatency, AlertSeverityWarning,
			"Average latency exceeds threshold")
	}

	// Check maximum latency (approximation for P99)
	maxLatencyNs := atomic.LoadInt64(&epm.maxLatencyNs)
	if maxLatencyNs > thresholds.MaxP99LatencyNs {
		epm.triggerAlert(AlertTypeLatency, AlertSeverityCritical,
			"P99 latency exceeds threshold")
	}
}

// checkThroughputThresholds checks throughput-related thresholds
func (epm *EnginePerformanceMonitor) checkThroughputThresholds(thresholds *PerformanceThresholds) {
	currentThroughput := float64(atomic.LoadInt64(&epm.avgThroughputOps)) / 1000.0

	if currentThroughput < thresholds.MinThroughputOps {
		epm.triggerAlert(AlertTypeThroughput, AlertSeverityWarning,
			"Throughput below minimum threshold")
	}

	// Check for throughput degradation
	peakThroughput := float64(atomic.LoadInt64(&epm.peakThroughputOps)) / 1000.0
	if peakThroughput > 0 {
		degradationPct := ((peakThroughput - currentThroughput) / peakThroughput) * 100
		if degradationPct > thresholds.ThroughputDegradationPct {
			epm.triggerAlert(AlertTypeThroughput, AlertSeverityWarning,
				"Significant throughput degradation detected")
			epm.RecordPerformanceDegradation()
		}
	}
}

// checkErrorRateThresholds checks error rate thresholds
func (epm *EnginePerformanceMonitor) checkErrorRateThresholds(thresholds *PerformanceThresholds) {
	totalOrders := atomic.LoadInt64(&epm.totalOrdersProcessed)
	totalErrors := atomic.LoadInt64(&epm.totalErrors)

	if totalOrders > 0 {
		errorRate := (float64(totalErrors) / float64(totalOrders)) * 100
		if errorRate > thresholds.MaxErrorRatePct {
			severity := AlertSeverityWarning
			if errorRate > thresholds.MaxErrorRatePct*2 {
				severity = AlertSeverityCritical
			}
			epm.triggerAlert(AlertTypeErrorRate, severity,
				"Error rate exceeds threshold")
		}
	}
}

// checkResourceThresholds checks system resource thresholds
func (epm *EnginePerformanceMonitor) checkResourceThresholds(thresholds *PerformanceThresholds) {
	// Check memory usage
	memoryMB := atomic.LoadInt64(&epm.memoryUsageBytes) / (1024 * 1024)
	if memoryMB > thresholds.MaxMemoryUsageMB {
		epm.triggerAlert(AlertTypeMemoryUsage, AlertSeverityWarning,
			"Memory usage exceeds threshold")
	}

	// Check goroutine count
	goroutines := atomic.LoadInt64(&epm.goroutineCount)
	if goroutines > thresholds.MaxGoroutineCount {
		epm.triggerAlert(AlertTypeGoroutineCount, AlertSeverityWarning,
			"Goroutine count exceeds threshold")
	}

	// Check GC pause time
	gcPauseMs := atomic.LoadInt64(&epm.gcPauseTimeNs) / 1000000
	if gcPauseMs > thresholds.MaxGCPauseMs {
		epm.triggerAlert(AlertTypeGCPause, AlertSeverityWarning,
			"GC pause time exceeds threshold")
	}
}

// Alert management

// triggerAlert triggers a performance alert
func (epm *EnginePerformanceMonitor) triggerAlert(alertType AlertType, severity AlertSeverity, message string) {
	alert := PerformanceAlert{
		Type:      alertType,
		Severity:  severity,
		Message:   message,
		Timestamp: time.Now(),
	}

	// Add to alert history
	epm.alertHistoryMu.Lock()
	epm.alertHistory = append(epm.alertHistory, alert)
	if len(epm.alertHistory) > epm.maxAlertHistory {
		epm.alertHistory = epm.alertHistory[1:]
	}
	epm.alertHistoryMu.Unlock()

	// Log the alert
	if epm.logger != nil {
		epm.logger.Warnw("Performance alert triggered",
			"type", alertType,
			"severity", severity,
			"message", message,
		)
	}

	// Call alert callback if set
	if epm.alertCallback != nil {
		epm.alertCallback(alertType, severity, message)
	}
}

// GetAlertHistory returns recent performance alerts
func (epm *EnginePerformanceMonitor) GetAlertHistory() []PerformanceAlert {
	epm.alertHistoryMu.RLock()
	defer epm.alertHistoryMu.RUnlock()

	// Return a copy to avoid race conditions
	history := make([]PerformanceAlert, len(epm.alertHistory))
	copy(history, epm.alertHistory)
	return history
}

// Metrics collection

// CollectMetrics returns comprehensive performance metrics
func (epm *EnginePerformanceMonitor) CollectMetrics() *EnginePerformanceMetrics {
	totalOrders := atomic.LoadInt64(&epm.totalOrdersProcessed)
	totalTrades := atomic.LoadInt64(&epm.totalTradesExecuted)
	totalErrors := atomic.LoadInt64(&epm.totalErrors)
	totalProcessingTime := atomic.LoadInt64(&epm.totalProcessingTimeNs)
	totalLatencyMeasurements := atomic.LoadInt64(&epm.totalLatencyMeasurements)
	latencySum := atomic.LoadInt64(&epm.latencySum)

	metrics := &EnginePerformanceMetrics{
		TotalOrdersProcessed:    totalOrders,
		TotalTradesExecuted:     totalTrades,
		TotalErrors:             totalErrors,
		ThroughputOpsPerSec:     float64(atomic.LoadInt64(&epm.avgThroughputOps)) / 1000.0,
		MemoryUsageMB:           float64(atomic.LoadInt64(&epm.memoryUsageBytes)) / (1024 * 1024),
		PeakMemoryUsageMB:       float64(atomic.LoadInt64(&epm.peakMemoryUsageBytes)) / (1024 * 1024),
		GoroutineCount:          atomic.LoadInt64(&epm.goroutineCount),
		LastGCPauseMs:           float64(atomic.LoadInt64(&epm.gcPauseTimeNs)) / 1000000.0,
		PerformanceDegradations: atomic.LoadInt64(&epm.performanceDegradations),
		ConsecutiveDegradations: atomic.LoadInt64(&epm.consecutiveDegradations),
		UpTime:                  time.Since(epm.startTime),
		LastUpdateTime:          time.Now(),
	}

	// Calculate derived metrics
	if totalOrders > 0 {
		metrics.AvgProcessingTimeMs = float64(totalProcessingTime) / float64(totalOrders) / 1000000.0
		metrics.ErrorRate = (float64(totalErrors) / float64(totalOrders)) * 100
	}

	if totalLatencyMeasurements > 0 {
		metrics.AvgLatencyMs = float64(latencySum) / float64(totalLatencyMeasurements) / 1000000.0
		metrics.MinLatencyMs = float64(atomic.LoadInt64(&epm.minLatencyNs)) / 1000000.0
		metrics.MaxLatencyMs = float64(atomic.LoadInt64(&epm.maxLatencyNs)) / 1000000.0
	}

	return metrics
}

// Reset resets all performance metrics
func (epm *EnginePerformanceMonitor) Reset() {
	// Reset atomic counters
	atomic.StoreInt64(&epm.totalOrdersProcessed, 0)
	atomic.StoreInt64(&epm.totalTradesExecuted, 0)
	atomic.StoreInt64(&epm.totalProcessingTimeNs, 0)
	atomic.StoreInt64(&epm.totalErrors, 0)
	atomic.StoreInt64(&epm.throughputWindow, 0)
	atomic.StoreInt64(&epm.avgThroughputOps, 0)
	atomic.StoreInt64(&epm.peakThroughputOps, 0)
	atomic.StoreInt64(&epm.totalLatencyMeasurements, 0)
	atomic.StoreInt64(&epm.latencySum, 0)
	atomic.StoreInt64(&epm.minLatencyNs, int64(^uint64(0)>>1))
	atomic.StoreInt64(&epm.maxLatencyNs, 0)
	atomic.StoreInt64(&epm.memoryUsageBytes, 0)
	atomic.StoreInt64(&epm.peakMemoryUsageBytes, 0)
	atomic.StoreInt64(&epm.goroutineCount, 0)
	atomic.StoreInt64(&epm.gcPauseTimeNs, 0)
	atomic.StoreInt64(&epm.lastGCTime, 0)
	atomic.StoreInt64(&epm.performanceDegradations, 0)
	atomic.StoreInt64(&epm.lastDegradationTime, 0)
	atomic.StoreInt64(&epm.consecutiveDegradations, 0)
	atomic.StoreInt64(&epm.lastThroughputCheck, time.Now().Unix())

	// Reset alert history
	epm.alertHistoryMu.Lock()
	epm.alertHistory = nil
	epm.alertHistoryMu.Unlock()

	epm.startTime = time.Now()
}
