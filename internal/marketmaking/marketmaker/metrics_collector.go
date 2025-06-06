// MetricsCollector gathers and reports advanced MarketMaker metrics
package marketmaker

import (
	"sync"
	"time"
)

// MetricsCollector provides enhanced metrics collection capabilities
type MetricsCollector struct {
	// Base metrics
	ordersPlaced    int64
	ordersCancelled int64
	ordersFilled    int64

	// Performance metrics
	operationDurations map[string][]time.Duration

	// Backtesting metrics
	backtestResults map[string]*BacktestMetrics

	// Protection
	mu sync.RWMutex
}

// BacktestMetrics contains metrics for a backtest execution
type BacktestMetrics struct {
	ID           string
	StartTime    time.Time
	EndTime      time.Time
	Duration     time.Duration
	TotalReturn  float64
	SharpeRatio  float64
	MaxDrawdown  float64
	TradeCount   int
	WinRate      float64
	StrategyType string
	PerfRating   string // A-F rating based on performance
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{
		operationDurations: make(map[string][]time.Duration),
		backtestResults:    make(map[string]*BacktestMetrics),
	}
}

// RecordOrderPlacement records an order being placed
func (mc *MetricsCollector) RecordOrderPlacement(symbol, side, orderType string, price, size float64) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	mc.ordersPlaced++
	// Prometheus metrics will be updated separately if needed
}

// RecordOrderExecution records an order being executed
func (mc *MetricsCollector) RecordOrderExecution(symbol, side string, price, size float64, latency time.Duration) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	mc.ordersFilled++
	// Prometheus metrics will be updated separately if needed
}

// RecordOperationDuration records the duration of a market making operation
func (mc *MetricsCollector) RecordOperationDuration(operation string, duration time.Duration) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	if _, exists := mc.operationDurations[operation]; !exists {
		mc.operationDurations[operation] = make([]time.Duration, 0)
	}

	mc.operationDurations[operation] = append(mc.operationDurations[operation], duration)

	// Keep only last 1000 measurements to avoid memory bloat
	if len(mc.operationDurations[operation]) > 1000 {
		mc.operationDurations[operation] = mc.operationDurations[operation][1:]
	}
	// Prometheus metrics will be updated separately if needed
}

// RecordBacktestCompletion records the completion of a backtest
func (mc *MetricsCollector) RecordBacktestCompletion(backtestID string, totalReturn, sharpeRatio float64) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	// Store backtest result
	mc.backtestResults[backtestID] = &BacktestMetrics{
		ID:          backtestID,
		EndTime:     time.Now(),
		TotalReturn: totalReturn,
		SharpeRatio: sharpeRatio,
	}
	// Prometheus metrics will be updated separately if needed
}

// GetPerformanceMetrics returns aggregated performance metrics
func (mc *MetricsCollector) GetPerformanceMetrics() map[string]interface{} {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	metrics := make(map[string]interface{})
	avgDurations := make(map[string]float64)

	// Calculate average durations
	for op, durations := range mc.operationDurations {
		var sum int64
		for _, d := range durations {
			sum += d.Nanoseconds()
		}
		if len(durations) > 0 {
			avgDurations[op] = float64(sum) / float64(len(durations)) / 1000000 // convert to ms
		}
	}

	metrics["orders_placed"] = mc.ordersPlaced
	metrics["orders_cancelled"] = mc.ordersCancelled
	metrics["orders_filled"] = mc.ordersFilled
	metrics["avg_operation_durations_ms"] = avgDurations
	metrics["backtest_count"] = len(mc.backtestResults)

	return metrics
}

// GetBacktestResults returns all stored backtest results
func (mc *MetricsCollector) GetBacktestResults() []*BacktestMetrics {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	results := make([]*BacktestMetrics, 0, len(mc.backtestResults))
	for _, result := range mc.backtestResults {
		results = append(results, result)
	}

	return results
}
