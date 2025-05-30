// =============================
// Lock-Free Performance Monitoring System
// =============================
// This file implements a comprehensive performance monitoring system with:
// 1. Lock-free metrics collection using atomic operations
// 2. Real-time latency tracking with percentile calculations
// 3. Adaptive contention detection and mitigation
// 4. Memory usage optimization tracking
// 5. Throughput analysis and bottleneck identification

package orderbook

import (
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

// PerformanceMonitor tracks order book performance metrics lock-free
type PerformanceMonitor struct {
	// Core metrics (atomic counters)
	ordersProcessed int64 // Total orders processed
	tradesExecuted  int64 // Total trades executed
	cancellations   int64 // Total cancellations
	snapshots       int64 // Total snapshots generated
	errors          int64 // Total errors encountered

	// Latency tracking (atomic pointer to histogram)
	addOrderLatency unsafe.Pointer // *LatencyHistogram
	cancelLatency   unsafe.Pointer // *LatencyHistogram
	snapshotLatency unsafe.Pointer // *LatencyHistogram
	matchingLatency unsafe.Pointer // *LatencyHistogram

	// Contention tracking
	lockContentionMap sync.Map // price -> *ContentionStats
	globalContention  int64    // Global contention counter

	// Throughput tracking (sliding window)
	throughputWindow unsafe.Pointer // *ThroughputWindow

	// Memory metrics
	memoryUsage      int64 // Current memory usage estimate
	peakMemoryUsage  int64 // Peak memory usage
	garbageCollected int64 // Objects returned to pools

	// Cache metrics
	cacheHits      int64 // Cache hits
	cacheMisses    int64 // Cache misses
	cacheEvictions int64 // Cache evictions

	// Performance alerts
	alertThresholds PerformanceThresholds
	lastAlert       int64 // Last alert timestamp
	alertCallback   func(AlertType, string)

	// Background monitoring
	monitoringActive int32 // Atomic flag
	stopMonitoring   chan struct{}
	monitoringTicker *time.Ticker
}

// LatencyHistogram tracks latency distribution lock-free
type LatencyHistogram struct {
	buckets    [20]int64 // Latency buckets (atomic counters)
	totalCount int64     // Total samples
	totalTime  int64     // Total time in nanoseconds
	minTime    int64     // Minimum time seen
	maxTime    int64     // Maximum time seen
	lastUpdate int64     // Last update timestamp
}

// ContentionStats tracks contention for a specific resource
type ContentionStats struct {
	waitTime     int64 // Total wait time in nanoseconds
	waitCount    int64 // Number of times waited
	acquisitions int64 // Total acquisitions
	lastWait     int64 // Last wait timestamp
}

// ThroughputWindow tracks throughput in a sliding time window
type ThroughputWindow struct {
	windowSize  time.Duration
	buckets     [60]int64 // 60 buckets for sliding window
	currentSlot int64     // Current bucket index
	lastUpdate  int64     // Last update timestamp
	totalOps    int64     // Total operations in window
}

// PerformanceThresholds defines alert thresholds
type PerformanceThresholds struct {
	MaxLatencyNs      int64   // Maximum acceptable latency (ns)
	MaxContentionRate float64 // Maximum contention rate (0.0-1.0)
	MaxMemoryMB       int64   // Maximum memory usage (MB)
	MinThroughput     int64   // Minimum throughput (ops/sec)
	MaxCacheMissRate  float64 // Maximum cache miss rate (0.0-1.0)
}

// AlertType represents different types of performance alerts
type AlertType int

const (
	AlertHighLatency AlertType = iota
	AlertHighContention
	AlertHighMemoryUsage
	AlertLowThroughput
	AlertHighCacheMissRate
	AlertDeadlockRisk
)

// String returns string representation of AlertType
func (at AlertType) String() string {
	switch at {
	case AlertHighLatency:
		return "HIGH_LATENCY"
	case AlertHighContention:
		return "HIGH_CONTENTION"
	case AlertHighMemoryUsage:
		return "HIGH_MEMORY_USAGE"
	case AlertLowThroughput:
		return "LOW_THROUGHPUT"
	case AlertHighCacheMissRate:
		return "HIGH_CACHE_MISS_RATE"
	case AlertDeadlockRisk:
		return "DEADLOCK_RISK"
	default:
		return "UNKNOWN"
	}
}

// NewPerformanceMonitor creates a new performance monitor
func NewPerformanceMonitor() *PerformanceMonitor {
	pm := &PerformanceMonitor{
		stopMonitoring: make(chan struct{}),
		alertThresholds: PerformanceThresholds{
			MaxLatencyNs:      10 * time.Millisecond.Nanoseconds(), // 10ms
			MaxContentionRate: 0.1,                                 // 10%
			MaxMemoryMB:       1024,                                // 1GB
			MinThroughput:     1000,                                // 1000 ops/sec
			MaxCacheMissRate:  0.2,                                 // 20%
		},
	}

	// Initialize latency histograms
	pm.addOrderLatency = unsafe.Pointer(&LatencyHistogram{})
	pm.cancelLatency = unsafe.Pointer(&LatencyHistogram{})
	pm.snapshotLatency = unsafe.Pointer(&LatencyHistogram{})
	pm.matchingLatency = unsafe.Pointer(&LatencyHistogram{})

	// Initialize throughput window
	pm.throughputWindow = unsafe.Pointer(&ThroughputWindow{
		windowSize: 60 * time.Second, // 60-second window
	})

	return pm
}

// StartMonitoring begins background performance monitoring
func (pm *PerformanceMonitor) StartMonitoring(interval time.Duration) {
	if atomic.SwapInt32(&pm.monitoringActive, 1) == 1 {
		return // Already active
	}

	pm.monitoringTicker = time.NewTicker(interval)

	go func() {
		defer atomic.StoreInt32(&pm.monitoringActive, 0)

		for {
			select {
			case <-pm.monitoringTicker.C:
				pm.performMonitoringCheck()
			case <-pm.stopMonitoring:
				pm.monitoringTicker.Stop()
				return
			}
		}
	}()
}

// StopMonitoring stops background monitoring
func (pm *PerformanceMonitor) StopMonitoring() {
	if atomic.LoadInt32(&pm.monitoringActive) == 1 {
		close(pm.stopMonitoring)
	}
}

// SetAlertCallback sets the callback function for performance alerts
func (pm *PerformanceMonitor) SetAlertCallback(callback func(AlertType, string)) {
	pm.alertCallback = callback
}

// RecordAddOrder records metrics for order addition
func (pm *PerformanceMonitor) RecordAddOrder(startTime time.Time, success bool) {
	latency := time.Since(startTime).Nanoseconds()

	atomic.AddInt64(&pm.ordersProcessed, 1)
	pm.recordLatency(pm.addOrderLatency, latency)
	pm.updateThroughput()

	if !success {
		atomic.AddInt64(&pm.errors, 1)
	}
}

// RecordCancelOrder records metrics for order cancellation
func (pm *PerformanceMonitor) RecordCancelOrder(startTime time.Time, success bool) {
	latency := time.Since(startTime).Nanoseconds()

	atomic.AddInt64(&pm.cancellations, 1)
	pm.recordLatency(pm.cancelLatency, latency)
	pm.updateThroughput()

	if !success {
		atomic.AddInt64(&pm.errors, 1)
	}
}

// RecordSnapshot records metrics for snapshot generation
func (pm *PerformanceMonitor) RecordSnapshot(startTime time.Time, depth int) {
	latency := time.Since(startTime).Nanoseconds()

	atomic.AddInt64(&pm.snapshots, 1)
	pm.recordLatency(pm.snapshotLatency, latency)
}

// RecordTrade records metrics for trade execution
func (pm *PerformanceMonitor) RecordTrade(matchingTime time.Duration) {
	atomic.AddInt64(&pm.tradesExecuted, 1)
	pm.recordLatency(pm.matchingLatency, matchingTime.Nanoseconds())
}

// RecordContention records lock contention for a specific resource
func (pm *PerformanceMonitor) RecordContention(resource string, waitTime time.Duration) {
	statsValue, _ := pm.lockContentionMap.LoadOrStore(resource, &ContentionStats{})
	stats := statsValue.(*ContentionStats)

	atomic.AddInt64(&stats.waitTime, waitTime.Nanoseconds())
	atomic.AddInt64(&stats.waitCount, 1)
	atomic.AddInt64(&stats.acquisitions, 1)
	atomic.StoreInt64(&stats.lastWait, time.Now().UnixNano())

	atomic.AddInt64(&pm.globalContention, 1)
}

// RecordCacheHit records a cache hit
func (pm *PerformanceMonitor) RecordCacheHit() {
	atomic.AddInt64(&pm.cacheHits, 1)
}

// RecordCacheMiss records a cache miss
func (pm *PerformanceMonitor) RecordCacheMiss() {
	atomic.AddInt64(&pm.cacheMisses, 1)
}

// RecordMemoryUsage updates current memory usage estimate
func (pm *PerformanceMonitor) RecordMemoryUsage(bytes int64) {
	atomic.StoreInt64(&pm.memoryUsage, bytes)

	// Update peak if necessary
	for {
		current := atomic.LoadInt64(&pm.peakMemoryUsage)
		if bytes <= current || atomic.CompareAndSwapInt64(&pm.peakMemoryUsage, current, bytes) {
			break
		}
	}
}

// recordLatency records latency in the appropriate histogram
func (pm *PerformanceMonitor) recordLatency(histogramPtr unsafe.Pointer, latencyNs int64) {
	histogram := (*LatencyHistogram)(atomic.LoadPointer(&histogramPtr))

	// Update basic stats atomically
	atomic.AddInt64(&histogram.totalCount, 1)
	atomic.AddInt64(&histogram.totalTime, latencyNs)
	atomic.StoreInt64(&histogram.lastUpdate, time.Now().UnixNano())

	// Update min time
	for {
		current := atomic.LoadInt64(&histogram.minTime)
		if current != 0 && latencyNs >= current {
			break
		}
		if atomic.CompareAndSwapInt64(&histogram.minTime, current, latencyNs) {
			break
		}
	}

	// Update max time
	for {
		current := atomic.LoadInt64(&histogram.maxTime)
		if latencyNs <= current {
			break
		}
		if atomic.CompareAndSwapInt64(&histogram.maxTime, current, latencyNs) {
			break
		}
	}

	// Add to appropriate bucket
	bucketIndex := pm.getLatencyBucketIndex(latencyNs)
	atomic.AddInt64(&histogram.buckets[bucketIndex], 1)
}

// getLatencyBucketIndex determines which bucket a latency value belongs to
func (pm *PerformanceMonitor) getLatencyBucketIndex(latencyNs int64) int {
	// Logarithmic buckets: [0-1us, 1-10us, 10-100us, 100us-1ms, 1-10ms, 10-100ms, ...]
	if latencyNs < 1000 {
		return 0 // < 1μs
	} else if latencyNs < 10000 {
		return 1 // 1-10μs
	} else if latencyNs < 100000 {
		return 2 // 10-100μs
	} else if latencyNs < 1000000 {
		return 3 // 100μs-1ms
	} else if latencyNs < 10000000 {
		return 4 // 1-10ms
	} else if latencyNs < 100000000 {
		return 5 // 10-100ms
	} else if latencyNs < 1000000000 {
		return 6 // 100ms-1s
	} else {
		return 7 // > 1s
	}
}

// updateThroughput updates the throughput sliding window
func (pm *PerformanceMonitor) updateThroughput() {
	window := (*ThroughputWindow)(atomic.LoadPointer(&pm.throughputWindow))
	now := time.Now().UnixNano()

	// Calculate current slot based on time
	slotDuration := window.windowSize.Nanoseconds() / 60 // 60 buckets
	currentSlot := (now / slotDuration) % 60

	// If we've moved to a new slot, reset old slots
	lastUpdate := atomic.LoadInt64(&window.lastUpdate)
	if now-lastUpdate > slotDuration {
		// Clear old slots
		slotsToAdvance := min(60, int((now-lastUpdate)/slotDuration))
		for i := 0; i < slotsToAdvance; i++ {
			slot := (currentSlot - int64(i) + 60) % 60
			atomic.StoreInt64(&window.buckets[slot], 0)
		}
		atomic.StoreInt64(&window.lastUpdate, now)
	}

	// Increment current slot
	atomic.AddInt64(&window.buckets[currentSlot], 1)
	atomic.AddInt64(&window.totalOps, 1)
	atomic.StoreInt64(&window.currentSlot, currentSlot)
}

// performMonitoringCheck performs periodic monitoring checks and triggers alerts
func (pm *PerformanceMonitor) performMonitoringCheck() {
	// Check latency thresholds
	pm.checkLatencyAlerts()

	// Check contention levels
	pm.checkContentionAlerts()

	// Check memory usage
	pm.checkMemoryAlerts()

	// Check throughput
	pm.checkThroughputAlerts()

	// Check cache performance
	pm.checkCacheAlerts()

	// Check for deadlock risk patterns
	pm.checkDeadlockRisk()
}

// checkLatencyAlerts checks for high latency conditions
func (pm *PerformanceMonitor) checkLatencyAlerts() {
	threshold := pm.alertThresholds.MaxLatencyNs

	// Check add order latency
	addHist := (*LatencyHistogram)(atomic.LoadPointer(&pm.addOrderLatency))
	if pm.getP95Latency(addHist) > threshold {
		pm.triggerAlert(AlertHighLatency, "Add order P95 latency exceeds threshold")
	}

	// Check snapshot latency
	snapHist := (*LatencyHistogram)(atomic.LoadPointer(&pm.snapshotLatency))
	if pm.getP95Latency(snapHist) > threshold {
		pm.triggerAlert(AlertHighLatency, "Snapshot P95 latency exceeds threshold")
	}
}

// checkContentionAlerts checks for high contention levels
func (pm *PerformanceMonitor) checkContentionAlerts() {
	totalContentions := atomic.LoadInt64(&pm.globalContention)
	totalOps := atomic.LoadInt64(&pm.ordersProcessed) + atomic.LoadInt64(&pm.cancellations)

	if totalOps > 0 {
		contentionRate := float64(totalContentions) / float64(totalOps)
		if contentionRate > pm.alertThresholds.MaxContentionRate {
			pm.triggerAlert(AlertHighContention, "Lock contention rate exceeds threshold")
		}
	}
}

// checkMemoryAlerts checks for high memory usage
func (pm *PerformanceMonitor) checkMemoryAlerts() {
	memoryMB := atomic.LoadInt64(&pm.memoryUsage) / (1024 * 1024)
	if memoryMB > pm.alertThresholds.MaxMemoryMB {
		pm.triggerAlert(AlertHighMemoryUsage, "Memory usage exceeds threshold")
	}
}

// checkThroughputAlerts checks for low throughput
func (pm *PerformanceMonitor) checkThroughputAlerts() {
	throughput := pm.getCurrentThroughput()
	if throughput < pm.alertThresholds.MinThroughput {
		pm.triggerAlert(AlertLowThroughput, "Throughput below threshold")
	}
}

// checkCacheAlerts checks for poor cache performance
func (pm *PerformanceMonitor) checkCacheAlerts() {
	hits := atomic.LoadInt64(&pm.cacheHits)
	misses := atomic.LoadInt64(&pm.cacheMisses)
	total := hits + misses

	if total > 0 {
		missRate := float64(misses) / float64(total)
		if missRate > pm.alertThresholds.MaxCacheMissRate {
			pm.triggerAlert(AlertHighCacheMissRate, "Cache miss rate exceeds threshold")
		}
	}
}

// checkDeadlockRisk analyzes contention patterns for deadlock risk
func (pm *PerformanceMonitor) checkDeadlockRisk() {
	// Detect potential deadlock scenarios by analyzing contention patterns
	highContentionResources := 0

	pm.lockContentionMap.Range(func(key, value interface{}) bool {
		stats := value.(*ContentionStats)
		waitCount := atomic.LoadInt64(&stats.waitCount)
		acquisitions := atomic.LoadInt64(&stats.acquisitions)

		if acquisitions > 0 {
			waitRatio := float64(waitCount) / float64(acquisitions)
			if waitRatio > 0.5 { // More than 50% of acquisitions involve waiting
				highContentionResources++
			}
		}

		return true
	})

	// If multiple resources have high contention, there's deadlock risk
	if highContentionResources >= 3 {
		pm.triggerAlert(AlertDeadlockRisk, "Multiple high-contention resources detected")
	}
}

// triggerAlert triggers a performance alert if callback is set
func (pm *PerformanceMonitor) triggerAlert(alertType AlertType, message string) {
	now := time.Now().UnixNano()
	lastAlert := atomic.LoadInt64(&pm.lastAlert)

	// Rate limit alerts (minimum 1 second between alerts of same type)
	if now-lastAlert < time.Second.Nanoseconds() {
		return
	}

	if atomic.CompareAndSwapInt64(&pm.lastAlert, lastAlert, now) && pm.alertCallback != nil {
		go pm.alertCallback(alertType, message)
	}
}

// getCurrentThroughput calculates current throughput (ops/sec)
func (pm *PerformanceMonitor) getCurrentThroughput() int64 {
	window := (*ThroughputWindow)(atomic.LoadPointer(&pm.throughputWindow))

	// Sum operations across all buckets in the window
	totalOps := int64(0)
	for i := 0; i < 60; i++ {
		totalOps += atomic.LoadInt64(&window.buckets[i])
	}

	// Convert to ops/sec (window is typically 60 seconds)
	return totalOps
}

// getP95Latency calculates P95 latency from histogram
func (pm *PerformanceMonitor) getP95Latency(hist *LatencyHistogram) int64 {
	totalCount := atomic.LoadInt64(&hist.totalCount)
	if totalCount == 0 {
		return 0
	}

	p95Count := totalCount * 95 / 100
	currentCount := int64(0)

	for i, bucket := range hist.buckets {
		currentCount += atomic.LoadInt64(&bucket)
		if currentCount >= p95Count {
			// Return upper bound of this bucket
			return pm.getBucketUpperBound(i)
		}
	}

	return atomic.LoadInt64(&hist.maxTime)
}

// getBucketUpperBound returns the upper bound for a latency bucket
func (pm *PerformanceMonitor) getBucketUpperBound(bucketIndex int) int64 {
	bounds := []int64{
		1000,        // 1μs
		10000,       // 10μs
		100000,      // 100μs
		1000000,     // 1ms
		10000000,    // 10ms
		100000000,   // 100ms
		1000000000,  // 1s
		10000000000, // 10s
	}

	if bucketIndex < len(bounds) {
		return bounds[bucketIndex]
	}
	return 10000000000 // 10s for highest bucket
}

// GetMetrics returns current performance metrics
func (pm *PerformanceMonitor) GetMetrics() map[string]interface{} {
	addHist := (*LatencyHistogram)(atomic.LoadPointer(&pm.addOrderLatency))
	cancelHist := (*LatencyHistogram)(atomic.LoadPointer(&pm.cancelLatency))
	snapHist := (*LatencyHistogram)(atomic.LoadPointer(&pm.snapshotLatency))

	return map[string]interface{}{
		"orders_processed":            atomic.LoadInt64(&pm.ordersProcessed),
		"trades_executed":             atomic.LoadInt64(&pm.tradesExecuted),
		"cancellations":               atomic.LoadInt64(&pm.cancellations),
		"snapshots":                   atomic.LoadInt64(&pm.snapshots),
		"errors":                      atomic.LoadInt64(&pm.errors),
		"memory_usage_mb":             atomic.LoadInt64(&pm.memoryUsage) / (1024 * 1024),
		"peak_memory_mb":              atomic.LoadInt64(&pm.peakMemoryUsage) / (1024 * 1024),
		"cache_hits":                  atomic.LoadInt64(&pm.cacheHits),
		"cache_misses":                atomic.LoadInt64(&pm.cacheMisses),
		"global_contention":           atomic.LoadInt64(&pm.globalContention),
		"current_throughput":          pm.getCurrentThroughput(),
		"add_order_p95_latency_ns":    pm.getP95Latency(addHist),
		"cancel_order_p95_latency_ns": pm.getP95Latency(cancelHist),
		"snapshot_p95_latency_ns":     pm.getP95Latency(snapHist),
	}
}

// ResetMetrics resets all performance metrics
func (pm *PerformanceMonitor) ResetMetrics() {
	atomic.StoreInt64(&pm.ordersProcessed, 0)
	atomic.StoreInt64(&pm.tradesExecuted, 0)
	atomic.StoreInt64(&pm.cancellations, 0)
	atomic.StoreInt64(&pm.snapshots, 0)
	atomic.StoreInt64(&pm.errors, 0)
	atomic.StoreInt64(&pm.globalContention, 0)
	atomic.StoreInt64(&pm.cacheHits, 0)
	atomic.StoreInt64(&pm.cacheMisses, 0)

	// Reset histograms
	pm.addOrderLatency = unsafe.Pointer(&LatencyHistogram{})
	pm.cancelLatency = unsafe.Pointer(&LatencyHistogram{})
	pm.snapshotLatency = unsafe.Pointer(&LatencyHistogram{})
	pm.matchingLatency = unsafe.Pointer(&LatencyHistogram{})

	// Clear contention map
	pm.lockContentionMap = sync.Map{}

	// Reset throughput window
	pm.throughputWindow = unsafe.Pointer(&ThroughputWindow{
		windowSize: 60 * time.Second,
	})
}
