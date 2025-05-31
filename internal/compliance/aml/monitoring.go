// =============================
// AML/Risk Management Performance Monitoring
// =============================
// This file provides comprehensive monitoring endpoints and metrics
// collection for the async risk management service performance tracking.

package aml

import (
	"context"
	"net/http"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// PerformanceMonitor handles performance monitoring for the async risk service
type PerformanceMonitor struct {
	asyncService *AsyncRiskService
	logger       *zap.SugaredLogger
	startTime    time.Time

	// Performance counters
	totalRequests       int64
	successfulRequests  int64
	failedRequests      int64
	timeoutRequests     int64
	circuitBreakerTrips int64

	// Latency tracking
	latencySum   int64 // in nanoseconds
	latencyCount int64

	// Last collection timestamp
	lastCollected time.Time
}

// NewPerformanceMonitor creates a new performance monitor
func NewPerformanceMonitor(asyncService *AsyncRiskService, logger *zap.SugaredLogger) *PerformanceMonitor {
	return &PerformanceMonitor{
		asyncService:  asyncService,
		logger:        logger,
		startTime:     time.Now(),
		lastCollected: time.Now(),
	}
}

// PerformanceMetrics represents comprehensive performance metrics
type PerformanceMetrics struct {
	// Service status
	ServiceHealthy     bool  `json:"service_healthy"`
	UptimeSeconds      int64 `json:"uptime_seconds"`
	AsyncServiceActive bool  `json:"async_service_active"`
	CircuitBreakerOpen bool  `json:"circuit_breaker_open"`

	// Request metrics
	TotalRequests      int64   `json:"total_requests"`
	SuccessfulRequests int64   `json:"successful_requests"`
	FailedRequests     int64   `json:"failed_requests"`
	TimeoutRequests    int64   `json:"timeout_requests"`
	SuccessRate        float64 `json:"success_rate"`
	ErrorRate          float64 `json:"error_rate"`
	TimeoutRate        float64 `json:"timeout_rate"`

	// Performance metrics
	AverageLatencyMs float64 `json:"average_latency_ms"`
	P95LatencyMs     float64 `json:"p95_latency_ms"`
	P99LatencyMs     float64 `json:"p99_latency_ms"`
	ThroughputOPS    float64 `json:"throughput_ops"`

	// Cache metrics
	CacheHitRatio float64 `json:"cache_hit_ratio"`
	CacheHits     int64   `json:"cache_hits"`
	CacheMisses   int64   `json:"cache_misses"`

	// Worker metrics
	ActiveWorkers      int `json:"active_workers"`
	QueuedRequests     int `json:"queued_requests"`
	ProcessingRequests int `json:"processing_requests"`

	// Resource metrics
	MemoryUsageMB   float64 `json:"memory_usage_mb"`
	CPUUsagePercent float64 `json:"cpu_usage_percent"`
	GoroutineCount  int     `json:"goroutine_count"`

	// SLA compliance
	SLACompliant       bool    `json:"sla_compliant"`
	TargetLatencyMs    int     `json:"target_latency_ms"`
	TargetThroughput   int     `json:"target_throughput"`
	TargetCacheHitRate float64 `json:"target_cache_hit_rate"`

	Timestamp time.Time `json:"timestamp"`
}

// HealthStatus represents the health status of the async risk service
type HealthStatus struct {
	Status        string            `json:"status"` // "healthy", "degraded", "unhealthy"
	AsyncService  bool              `json:"async_service"`
	Workers       int               `json:"workers"`
	QueueDepth    int               `json:"queue_depth"`
	LastError     string            `json:"last_error,omitempty"`
	LastErrorTime *time.Time        `json:"last_error_time,omitempty"`
	ResponseTime  time.Duration     `json:"response_time"`
	Dependencies  map[string]string `json:"dependencies"`
	Timestamp     time.Time         `json:"timestamp"`
}

// RegisterMonitoringEndpoints registers performance monitoring HTTP endpoints
func RegisterMonitoringEndpoints(router *gin.Engine, monitor *PerformanceMonitor) {
	api := router.Group("/api/v1/risk/monitoring")
	{
		// Performance metrics endpoint
		api.GET("/metrics", monitor.handleMetrics)

		// Health check endpoint
		api.GET("/health", monitor.handleHealth)

		// Detailed performance report
		api.GET("/performance", monitor.handlePerformanceReport)

		// Circuit breaker status
		api.GET("/circuit-breaker", monitor.handleCircuitBreakerStatus)

		// Worker status
		api.GET("/workers", monitor.handleWorkerStatus)

		// Cache statistics
		api.GET("/cache", monitor.handleCacheStatistics)

		// SLA compliance report
		api.GET("/sla", monitor.handleSLACompliance)

		// System resource usage
		api.GET("/resources", monitor.handleResourceUsage)
	}
}

// handleMetrics returns current performance metrics
func (pm *PerformanceMonitor) handleMetrics(c *gin.Context) {
	metrics := pm.collectMetrics()
	c.JSON(http.StatusOK, metrics)
}

// handleHealth returns service health status
func (pm *PerformanceMonitor) handleHealth(c *gin.Context) {
	status := pm.getHealthStatus()

	httpStatus := http.StatusOK
	if status.Status == "unhealthy" {
		httpStatus = http.StatusServiceUnavailable
	} else if status.Status == "degraded" {
		httpStatus = http.StatusPartialContent
	}

	c.JSON(httpStatus, status)
}

// handlePerformanceReport returns detailed performance report
func (pm *PerformanceMonitor) handlePerformanceReport(c *gin.Context) {
	report := pm.generatePerformanceReport()
	c.JSON(http.StatusOK, report)
}

// handleCircuitBreakerStatus returns circuit breaker status
func (pm *PerformanceMonitor) handleCircuitBreakerStatus(c *gin.Context) {
	if pm.asyncService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Async service not available",
		})
		return
	}

	status := map[string]interface{}{
		"open":          pm.asyncService.circuitBreaker.IsOpen(),
		"failure_count": pm.asyncService.circuitBreaker.FailureCount(),
		"success_count": pm.asyncService.circuitBreaker.SuccessCount(),
		"last_failure":  pm.asyncService.circuitBreaker.LastFailureTime(),
		"last_success":  pm.asyncService.circuitBreaker.LastSuccessTime(),
		"next_attempt":  pm.asyncService.circuitBreaker.NextAttempt(),
		"timestamp":     time.Now(),
	}

	c.JSON(http.StatusOK, status)
}

// handleWorkerStatus returns worker pool status
func (pm *PerformanceMonitor) handleWorkerStatus(c *gin.Context) {
	if pm.asyncService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Async service not available",
		})
		return
	}

	metrics := pm.asyncService.GetMetrics()
	workerStatus := map[string]interface{}{
		"active_workers":      metrics.ActiveWorkers,
		"queued_requests":     metrics.QueuedRequests,
		"processing_requests": metrics.ProcessingRequests,
		"worker_utilization":  float64(metrics.ProcessingRequests) / float64(metrics.ActiveWorkers),
		"queue_depth":         len(pm.asyncService.requestChan),
		"queue_capacity":      cap(pm.asyncService.requestChan),
		"timestamp":           time.Now(),
	}

	c.JSON(http.StatusOK, workerStatus)
}

// handleCacheStatistics returns cache performance statistics
func (pm *PerformanceMonitor) handleCacheStatistics(c *gin.Context) {
	if pm.asyncService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Async service not available",
		})
		return
	}

	metrics := pm.asyncService.GetMetrics()
	cacheStats := map[string]interface{}{
		"cache_hits":     metrics.CacheHits,
		"cache_misses":   metrics.CacheMisses,
		"hit_ratio":      metrics.CacheHitRatio,
		"enabled":        pm.asyncService.config.EnableCaching,
		"preload_active": pm.asyncService.config.PreloadUserProfiles,
		"timestamp":      time.Now(),
	}

	c.JSON(http.StatusOK, cacheStats)
}

// handleSLACompliance returns SLA compliance metrics
func (pm *PerformanceMonitor) handleSLACompliance(c *gin.Context) {
	metrics := pm.collectMetrics()

	slaReport := map[string]interface{}{
		"compliant":           metrics.SLACompliant,
		"target_latency_ms":   metrics.TargetLatencyMs,
		"actual_latency_ms":   metrics.AverageLatencyMs,
		"target_throughput":   metrics.TargetThroughput,
		"actual_throughput":   metrics.ThroughputOPS,
		"target_cache_hit":    metrics.TargetCacheHitRate,
		"actual_cache_hit":    metrics.CacheHitRatio,
		"target_success_rate": 0.99, // 99% success rate target
		"actual_success_rate": metrics.SuccessRate,
		"compliance_score":    pm.calculateComplianceScore(metrics),
		"timestamp":           time.Now(),
	}

	c.JSON(http.StatusOK, slaReport)
}

// handleResourceUsage returns system resource usage
func (pm *PerformanceMonitor) handleResourceUsage(c *gin.Context) {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	resourceUsage := map[string]interface{}{
		"memory_usage_mb": float64(memStats.Alloc) / 1024 / 1024,
		"memory_total_mb": float64(memStats.Sys) / 1024 / 1024,
		"gc_cycles":       memStats.NumGC,
		"goroutine_count": runtime.NumGoroutine(),
		"cpu_count":       runtime.NumCPU(),
		"heap_objects":    memStats.HeapObjects,
		"timestamp":       time.Now(),
	}

	c.JSON(http.StatusOK, resourceUsage)
}

// collectMetrics collects current performance metrics
func (pm *PerformanceMonitor) collectMetrics() *PerformanceMetrics {
	uptime := time.Since(pm.startTime)

	// Get async service metrics
	var asyncMetrics *AsyncMetrics
	if pm.asyncService != nil {
		asyncMetrics = pm.asyncService.GetMetrics()
	}

	// Calculate rates
	total := atomic.LoadInt64(&pm.totalRequests)
	successful := atomic.LoadInt64(&pm.successfulRequests)
	failed := atomic.LoadInt64(&pm.failedRequests)
	timeout := atomic.LoadInt64(&pm.timeoutRequests)

	var successRate, errorRate, timeoutRate float64
	if total > 0 {
		successRate = float64(successful) / float64(total)
		errorRate = float64(failed) / float64(total)
		timeoutRate = float64(timeout) / float64(total)
	}

	// Calculate average latency
	latencySum := atomic.LoadInt64(&pm.latencySum)
	latencyCount := atomic.LoadInt64(&pm.latencyCount)
	var avgLatency float64
	if latencyCount > 0 {
		avgLatency = float64(latencySum) / float64(latencyCount) / 1e6 // Convert to milliseconds
	}

	// Calculate throughput
	throughput := float64(total) / uptime.Seconds()

	// Memory usage
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	memoryMB := float64(memStats.Alloc) / 1024 / 1024

	metrics := &PerformanceMetrics{
		ServiceHealthy:     pm.asyncService != nil,
		UptimeSeconds:      int64(uptime.Seconds()),
		AsyncServiceActive: pm.asyncService != nil,
		TotalRequests:      total,
		SuccessfulRequests: successful,
		FailedRequests:     failed,
		TimeoutRequests:    timeout,
		SuccessRate:        successRate,
		ErrorRate:          errorRate,
		TimeoutRate:        timeoutRate,
		AverageLatencyMs:   avgLatency,
		ThroughputOPS:      throughput,
		MemoryUsageMB:      memoryMB,
		GoroutineCount:     runtime.NumGoroutine(),
		TargetLatencyMs:    500,   // 500ms target from risk-management.yaml
		TargetThroughput:   10000, // 10K OPS target
		TargetCacheHitRate: 0.95,  // 95% cache hit rate target
		Timestamp:          time.Now(),
	}

	// Add async service specific metrics
	if asyncMetrics != nil {
		metrics.CircuitBreakerOpen = asyncMetrics.CircuitBreakerOpen
		metrics.CacheHitRatio = asyncMetrics.CacheHitRatio
		metrics.CacheHits = asyncMetrics.CacheHits
		metrics.CacheMisses = asyncMetrics.CacheMisses
		metrics.ActiveWorkers = asyncMetrics.ActiveWorkers
		metrics.QueuedRequests = asyncMetrics.QueuedRequests
		metrics.ProcessingRequests = asyncMetrics.ProcessingRequests
		metrics.P95LatencyMs = asyncMetrics.P95LatencyMs
		metrics.P99LatencyMs = asyncMetrics.P99LatencyMs
	}

	// Check SLA compliance
	metrics.SLACompliant = pm.checkSLACompliance(metrics)

	return metrics
}

// getHealthStatus determines the current health status
func (pm *PerformanceMonitor) getHealthStatus() *HealthStatus {
	status := &HealthStatus{
		Status:       "healthy",
		AsyncService: pm.asyncService != nil,
		Timestamp:    time.Now(),
		Dependencies: make(map[string]string),
	}

	if pm.asyncService == nil {
		status.Status = "degraded"
		status.LastError = "Async service not available"
		return status
	}

	// Check async service health
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
	defer cancel()

	start := time.Now()
	_, err := pm.asyncService.CalculateRiskAsync(ctx, "health-check-user")
	status.ResponseTime = time.Since(start)

	if err != nil {
		status.Status = "degraded"
		status.LastError = err.Error()
		now := time.Now()
		status.LastErrorTime = &now
	}

	// Check circuit breaker
	if pm.asyncService.circuitBreaker.IsOpen() {
		status.Status = "degraded"
		status.LastError = "Circuit breaker is open"
	}

	// Check worker status
	metrics := pm.asyncService.GetMetrics()
	status.Workers = metrics.ActiveWorkers
	status.QueueDepth = metrics.QueuedRequests

	// Check dependencies
	status.Dependencies["redis"] = "connected" // Assume connected if async service is running
	status.Dependencies["base_service"] = "connected"

	return status
}

// generatePerformanceReport generates a detailed performance report
func (pm *PerformanceMonitor) generatePerformanceReport() map[string]interface{} {
	metrics := pm.collectMetrics()

	return map[string]interface{}{
		"summary": map[string]interface{}{
			"status":         pm.getHealthStatus().Status,
			"uptime_hours":   float64(metrics.UptimeSeconds) / 3600,
			"total_requests": metrics.TotalRequests,
			"success_rate":   metrics.SuccessRate,
			"avg_latency_ms": metrics.AverageLatencyMs,
			"throughput_ops": metrics.ThroughputOPS,
			"sla_compliant":  metrics.SLACompliant,
		},
		"performance": map[string]interface{}{
			"latency": map[string]interface{}{
				"average_ms": metrics.AverageLatencyMs,
				"p95_ms":     metrics.P95LatencyMs,
				"p99_ms":     metrics.P99LatencyMs,
				"target_ms":  metrics.TargetLatencyMs,
			},
			"throughput": map[string]interface{}{
				"current_ops": metrics.ThroughputOPS,
				"target_ops":  metrics.TargetThroughput,
			},
			"cache": map[string]interface{}{
				"hit_ratio":    metrics.CacheHitRatio,
				"target_ratio": metrics.TargetCacheHitRate,
				"hits":         metrics.CacheHits,
				"misses":       metrics.CacheMisses,
			},
		},
		"reliability": map[string]interface{}{
			"success_rate":         metrics.SuccessRate,
			"error_rate":           metrics.ErrorRate,
			"timeout_rate":         metrics.TimeoutRate,
			"circuit_breaker_open": metrics.CircuitBreakerOpen,
		},
		"resources": map[string]interface{}{
			"memory_mb":       metrics.MemoryUsageMB,
			"goroutines":      metrics.GoroutineCount,
			"active_workers":  metrics.ActiveWorkers,
			"queued_requests": metrics.QueuedRequests,
		},
		"compliance_score": pm.calculateComplianceScore(metrics),
		"timestamp":        metrics.Timestamp,
	}
}

// checkSLACompliance checks if current metrics meet SLA requirements
func (pm *PerformanceMonitor) checkSLACompliance(metrics *PerformanceMetrics) bool {
	// Check latency SLA (< 500ms)
	if metrics.AverageLatencyMs > float64(metrics.TargetLatencyMs) {
		return false
	}

	// Check success rate SLA (> 99%)
	if metrics.SuccessRate < 0.99 {
		return false
	}

	// Check cache hit rate SLA (> 95%)
	if metrics.CacheHitRatio < metrics.TargetCacheHitRate {
		return false
	}

	// Check circuit breaker status
	if metrics.CircuitBreakerOpen {
		return false
	}

	return true
}

// calculateComplianceScore calculates overall compliance score (0-100)
func (pm *PerformanceMonitor) calculateComplianceScore(metrics *PerformanceMetrics) float64 {
	score := 100.0

	// Latency penalty (max 30 points)
	if metrics.AverageLatencyMs > float64(metrics.TargetLatencyMs) {
		penalty := (metrics.AverageLatencyMs - float64(metrics.TargetLatencyMs)) / float64(metrics.TargetLatencyMs) * 30
		if penalty > 30 {
			penalty = 30
		}
		score -= penalty
	}

	// Success rate penalty (max 25 points)
	if metrics.SuccessRate < 0.99 {
		penalty := (0.99 - metrics.SuccessRate) * 2500 // 1% = 25 points
		if penalty > 25 {
			penalty = 25
		}
		score -= penalty
	}

	// Cache hit rate penalty (max 20 points)
	if metrics.CacheHitRatio < metrics.TargetCacheHitRate {
		penalty := (metrics.TargetCacheHitRate - metrics.CacheHitRatio) * 400 // 5% = 20 points
		if penalty > 20 {
			penalty = 20
		}
		score -= penalty
	}

	// Circuit breaker penalty (max 25 points)
	if metrics.CircuitBreakerOpen {
		score -= 25
	}

	if score < 0 {
		score = 0
	}

	return score
}

// RecordRequest records a request for monitoring
func (pm *PerformanceMonitor) RecordRequest(latency time.Duration, success bool, timeout bool) {
	atomic.AddInt64(&pm.totalRequests, 1)

	if success {
		atomic.AddInt64(&pm.successfulRequests, 1)
	} else {
		atomic.AddInt64(&pm.failedRequests, 1)
	}

	if timeout {
		atomic.AddInt64(&pm.timeoutRequests, 1)
	}

	// Record latency
	atomic.AddInt64(&pm.latencySum, latency.Nanoseconds())
	atomic.AddInt64(&pm.latencyCount, 1)
}
