// =============================
// Engine Metrics Collection System
// =============================
// This file implements comprehensive metrics collection for the adaptive
// trading engine with support for real-time monitoring and analysis.

package engine

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/trading/model"
	"github.com/Aidin1998/pincex_unified/internal/trading/orderbook"
)

// EngineMetricsCollector collects and aggregates metrics across the engine
type EngineMetricsCollector struct {
	// Configuration
	collectionInterval time.Duration

	// Atomic counters for thread-safe operations
	totalOrdersProcessed  int64
	totalTradesExecuted   int64
	totalErrors           int64
	totalProcessingTimeNs int64

	// Per-type order tracking
	limitOrders  int64
	marketOrders int64
	iocOrders    int64
	fokOrders    int64

	// Per-pair metrics
	pairMetrics   map[string]*PairMetricsData
	pairMetricsMu sync.RWMutex

	// Performance tracking
	latencyBuckets      [10]int64 // Latency distribution buckets
	lastThroughputCheck int64     // Unix timestamp
	throughputWindow    int64     // Operations in current window

	// Control
	stopChan chan struct{}
	running  int32 // atomic
}

// PairMetricsData holds detailed metrics for a trading pair
type PairMetricsData struct {
	Pair                  string
	OrdersProcessed       int64
	TradesExecuted        int64
	Errors                int64
	TotalProcessingTimeNs int64
	LastActivityTime      int64 // Unix timestamp

	// Order type breakdown
	LimitOrders  int64
	MarketOrders int64
	IOCOrders    int64
	FOKOrders    int64

	// Performance metrics
	AvgLatencyMs   float64
	ThroughputOps  float64
	ErrorRate      float64
	LastUpdateTime time.Time

	// Migration specific
	OldImplRequests int64
	NewImplRequests int64
	MigrationRate   float64
}

// NewEngineMetricsCollector creates a new metrics collector
func NewEngineMetricsCollector(collectionInterval time.Duration) *EngineMetricsCollector {
	return &EngineMetricsCollector{
		collectionInterval:  collectionInterval,
		pairMetrics:         make(map[string]*PairMetricsData),
		stopChan:            make(chan struct{}),
		lastThroughputCheck: time.Now().Unix(),
	}
}

// Start begins metrics collection
func (emc *EngineMetricsCollector) Start() {
	if !atomic.CompareAndSwapInt32(&emc.running, 0, 1) {
		return // Already running
	}

	go emc.collectionWorker()
}

// Stop stops metrics collection
func (emc *EngineMetricsCollector) Stop() {
	if !atomic.CompareAndSwapInt32(&emc.running, 1, 0) {
		return // Not running
	}

	close(emc.stopChan)
}

// collectionWorker runs the periodic metrics collection
func (emc *EngineMetricsCollector) collectionWorker() {
	ticker := time.NewTicker(emc.collectionInterval)
	defer ticker.Stop()

	for {
		select {
		case <-emc.stopChan:
			return
		case <-ticker.C:
			emc.updateThroughputMetrics()
			emc.calculatePairMetrics()
		}
	}
}

// Order processing metrics

// RecordOrderProcessing records metrics for order processing
func (emc *EngineMetricsCollector) RecordOrderProcessing(pair string, duration time.Duration, orderType string) {
	atomic.AddInt64(&emc.totalOrdersProcessed, 1)
	atomic.AddInt64(&emc.totalProcessingTimeNs, duration.Nanoseconds())
	atomic.AddInt64(&emc.throughputWindow, 1)

	// Record order type
	switch orderType {
	case model.OrderTypeLimit:
		atomic.AddInt64(&emc.limitOrders, 1)
	case model.OrderTypeMarket:
		atomic.AddInt64(&emc.marketOrders, 1)
	case model.OrderTypeIOC:
		atomic.AddInt64(&emc.iocOrders, 1)
	case model.OrderTypeFOK:
		atomic.AddInt64(&emc.fokOrders, 1)
	}

	// Record latency bucket
	emc.recordLatencyBucket(duration)

	// Update pair-specific metrics
	emc.updatePairMetrics(pair, duration, orderType, true)
}

// RecordOrderResult records the result of order processing
func (emc *EngineMetricsCollector) RecordOrderResult(pair string, success bool, tradesCount int) {
	if !success {
		atomic.AddInt64(&emc.totalErrors, 1)
		emc.updatePairMetrics(pair, 0, model.OrderTypeUnknown, false)
		return
	}

	if tradesCount > 0 {
		atomic.AddInt64(&emc.totalTradesExecuted, int64(tradesCount))
		emc.recordTradesForPair(pair, tradesCount)
	}
}

// RecordPairMetrics records comprehensive metrics for a pair
func (emc *EngineMetricsCollector) RecordPairMetrics(pair string, perfMetrics, cacheMetrics interface{}, migrationStats orderbook.MigrationStats) {
	emc.pairMetricsMu.Lock()
	defer emc.pairMetricsMu.Unlock()

	data, exists := emc.pairMetrics[pair]
	if !exists {
		data = &PairMetricsData{
			Pair:           pair,
			LastUpdateTime: time.Now(),
		}
		emc.pairMetrics[pair] = data
	}

	// Update migration-specific metrics
	data.OldImplRequests = migrationStats.OldImplRequests
	data.NewImplRequests = migrationStats.NewImplRequests

	if migrationStats.TotalRequests > 0 {
		data.MigrationRate = float64(migrationStats.NewImplRequests) / float64(migrationStats.TotalRequests) * 100
		data.ErrorRate = float64(migrationStats.FailedRequests) / float64(migrationStats.TotalRequests) * 100
	}

	// Extract performance metrics if available
	if perfMap, ok := perfMetrics.(map[string]interface{}); ok {
		if latency, exists := perfMap["avg_latency_ms"]; exists {
			if latencyFloat, ok := latency.(float64); ok {
				data.AvgLatencyMs = latencyFloat
			}
		}

		if throughput, exists := perfMap["throughput_ops"]; exists {
			if throughputFloat, ok := throughput.(float64); ok {
				data.ThroughputOps = throughputFloat
			}
		}
	}

	data.LastUpdateTime = time.Now()
}

// Helper methods

// recordLatencyBucket records latency in appropriate bucket
func (emc *EngineMetricsCollector) recordLatencyBucket(duration time.Duration) {
	ms := duration.Milliseconds()

	bucketIndex := 0
	switch {
	case ms < 1:
		bucketIndex = 0 // < 1ms
	case ms < 5:
		bucketIndex = 1 // 1-5ms
	case ms < 10:
		bucketIndex = 2 // 5-10ms
	case ms < 25:
		bucketIndex = 3 // 10-25ms
	case ms < 50:
		bucketIndex = 4 // 25-50ms
	case ms < 100:
		bucketIndex = 5 // 50-100ms
	case ms < 250:
		bucketIndex = 6 // 100-250ms
	case ms < 500:
		bucketIndex = 7 // 250-500ms
	case ms < 1000:
		bucketIndex = 8 // 500ms-1s
	default:
		bucketIndex = 9 // > 1s
	}

	atomic.AddInt64(&emc.latencyBuckets[bucketIndex], 1)
}

// updatePairMetrics updates metrics for a specific pair
func (emc *EngineMetricsCollector) updatePairMetrics(pair string, duration time.Duration, orderType string, success bool) {
	emc.pairMetricsMu.Lock()
	defer emc.pairMetricsMu.Unlock()

	data, exists := emc.pairMetrics[pair]
	if !exists {
		data = &PairMetricsData{
			Pair:           pair,
			LastUpdateTime: time.Now(),
		}
		emc.pairMetrics[pair] = data
	}

	if success {
		data.OrdersProcessed++
		data.TotalProcessingTimeNs += duration.Nanoseconds()

		// Update order type counters
		switch orderType {
		case model.OrderTypeLimit:
			data.LimitOrders++
		case model.OrderTypeMarket:
			data.MarketOrders++
		case model.OrderTypeIOC:
			data.IOCOrders++
		case model.OrderTypeFOK:
			data.FOKOrders++
		}
	} else {
		data.Errors++
	}

	data.LastActivityTime = time.Now().Unix()
	data.LastUpdateTime = time.Now()
}

// recordTradesForPair records trade execution for a pair
func (emc *EngineMetricsCollector) recordTradesForPair(pair string, tradesCount int) {
	emc.pairMetricsMu.Lock()
	defer emc.pairMetricsMu.Unlock()

	data, exists := emc.pairMetrics[pair]
	if !exists {
		data = &PairMetricsData{
			Pair:           pair,
			LastUpdateTime: time.Now(),
		}
		emc.pairMetrics[pair] = data
	}

	data.TradesExecuted += int64(tradesCount)
	data.LastActivityTime = time.Now().Unix()
	data.LastUpdateTime = time.Now()
}

// updateThroughputMetrics updates throughput calculations
func (emc *EngineMetricsCollector) updateThroughputMetrics() {
	now := time.Now().Unix()
	lastCheck := atomic.LoadInt64(&emc.lastThroughputCheck)

	if now > lastCheck {
		windowOps := atomic.SwapInt64(&emc.throughputWindow, 0)
		timeDiff := now - lastCheck

		if timeDiff > 0 {
			// Store throughput for this period (ops per second)
			throughput := float64(windowOps) / float64(timeDiff)
			emc.updateOverallThroughput(throughput)
		}

		atomic.StoreInt64(&emc.lastThroughputCheck, now)
	}
}

// updateOverallThroughput updates overall throughput metrics
func (emc *EngineMetricsCollector) updateOverallThroughput(throughput float64) {
	// Update pair-specific throughput metrics
	emc.pairMetricsMu.Lock()
	defer emc.pairMetricsMu.Unlock()

	for _, data := range emc.pairMetrics {
		// Simple exponential moving average for throughput
		if data.ThroughputOps == 0 {
			data.ThroughputOps = throughput
		} else {
			data.ThroughputOps = data.ThroughputOps*0.8 + throughput*0.2
		}
	}
}

// calculatePairMetrics calculates derived metrics for pairs
func (emc *EngineMetricsCollector) calculatePairMetrics() {
	emc.pairMetricsMu.Lock()
	defer emc.pairMetricsMu.Unlock()

	for _, data := range emc.pairMetrics {
		// Calculate average latency
		if data.OrdersProcessed > 0 {
			data.AvgLatencyMs = float64(data.TotalProcessingTimeNs) / float64(data.OrdersProcessed) / 1000000.0
		}

		// Calculate error rate
		totalRequests := data.OrdersProcessed + data.Errors
		if totalRequests > 0 {
			data.ErrorRate = float64(data.Errors) / float64(totalRequests) * 100
		}
	}
}

// Metrics retrieval methods

// GetEngineMetrics returns overall engine metrics
func (emc *EngineMetricsCollector) GetEngineMetrics() *EngineMetrics {
	totalOrders := atomic.LoadInt64(&emc.totalOrdersProcessed)
	totalTrades := atomic.LoadInt64(&emc.totalTradesExecuted)
	totalErrors := atomic.LoadInt64(&emc.totalErrors)
	totalProcessingTime := atomic.LoadInt64(&emc.totalProcessingTimeNs)

	metrics := &EngineMetrics{
		TotalOrdersProcessed: totalOrders,
		TotalTradesExecuted:  totalTrades,
		LastUpdateTime:       time.Now(),
	}

	// Calculate averages
	if totalOrders > 0 {
		metrics.AvgProcessingTimeMs = float64(totalProcessingTime) / float64(totalOrders) / 1000000.0
		metrics.ErrorRate = float64(totalErrors) / float64(totalOrders) * 100
	}

	// Calculate throughput (simple approximation)
	if emc.collectionInterval > 0 {
		windowOps := atomic.LoadInt64(&emc.throughputWindow)
		metrics.ThroughputOpsPerSec = float64(windowOps) / emc.collectionInterval.Seconds()
	}

	return metrics
}

// GetPairMetrics returns metrics for a specific pair
func (emc *EngineMetricsCollector) GetPairMetrics(pair string) *PairMetricsData {
	emc.pairMetricsMu.RLock()
	defer emc.pairMetricsMu.RUnlock()

	if data, exists := emc.pairMetrics[pair]; exists {
		// Return a copy to avoid race conditions
		copy := *data
		return &copy
	}

	return nil
}

// GetAllPairMetrics returns metrics for all pairs
func (emc *EngineMetricsCollector) GetAllPairMetrics() map[string]*PairMetricsData {
	emc.pairMetricsMu.RLock()
	defer emc.pairMetricsMu.RUnlock()

	result := make(map[string]*PairMetricsData, len(emc.pairMetrics))
	for pair, data := range emc.pairMetrics {
		// Return copies to avoid race conditions
		copy := *data
		result[pair] = &copy
	}

	return result
}

// GetLatencyDistribution returns latency distribution across buckets
func (emc *EngineMetricsCollector) GetLatencyDistribution() map[string]int64 {
	return map[string]int64{
		"<1ms":      atomic.LoadInt64(&emc.latencyBuckets[0]),
		"1-5ms":     atomic.LoadInt64(&emc.latencyBuckets[1]),
		"5-10ms":    atomic.LoadInt64(&emc.latencyBuckets[2]),
		"10-25ms":   atomic.LoadInt64(&emc.latencyBuckets[3]),
		"25-50ms":   atomic.LoadInt64(&emc.latencyBuckets[4]),
		"50-100ms":  atomic.LoadInt64(&emc.latencyBuckets[5]),
		"100-250ms": atomic.LoadInt64(&emc.latencyBuckets[6]),
		"250-500ms": atomic.LoadInt64(&emc.latencyBuckets[7]),
		"500ms-1s":  atomic.LoadInt64(&emc.latencyBuckets[8]),
		">1s":       atomic.LoadInt64(&emc.latencyBuckets[9]),
	}
}

// GetOrderTypeDistribution returns distribution of order types
func (emc *EngineMetricsCollector) GetOrderTypeDistribution() map[string]int64 {
	return map[string]int64{
		"limit":  atomic.LoadInt64(&emc.limitOrders),
		"market": atomic.LoadInt64(&emc.marketOrders),
		"ioc":    atomic.LoadInt64(&emc.iocOrders),
		"fok":    atomic.LoadInt64(&emc.fokOrders),
	}
}

// Reset resets all metrics
func (emc *EngineMetricsCollector) Reset() {
	// Reset atomic counters
	atomic.StoreInt64(&emc.totalOrdersProcessed, 0)
	atomic.StoreInt64(&emc.totalTradesExecuted, 0)
	atomic.StoreInt64(&emc.totalErrors, 0)
	atomic.StoreInt64(&emc.totalProcessingTimeNs, 0)
	atomic.StoreInt64(&emc.limitOrders, 0)
	atomic.StoreInt64(&emc.marketOrders, 0)
	atomic.StoreInt64(&emc.iocOrders, 0)
	atomic.StoreInt64(&emc.fokOrders, 0)
	atomic.StoreInt64(&emc.throughputWindow, 0)

	// Reset latency buckets
	for i := range emc.latencyBuckets {
		atomic.StoreInt64(&emc.latencyBuckets[i], 0)
	}

	// Reset pair metrics
	emc.pairMetricsMu.Lock()
	emc.pairMetrics = make(map[string]*PairMetricsData)
	emc.pairMetricsMu.Unlock()

	atomic.StoreInt64(&emc.lastThroughputCheck, time.Now().Unix())
}
