// MetricsBridge adds Prometheus functionality to MetricsCollector
package marketmaker

import (
	"context"
	"time"
)

// The methods below extend MetricsCollector to bridge to the PrometheusMetricsCollector
// This allows for backward compatibility while still providing prometheus metrics

// Prometheus metrics connector
var promMetrics *PrometheusMetricsCollector

// Start initializes and starts the metrics collection
func (mc *MetricsCollector) Start(ctx context.Context) error {
	// Initialize prometheus metrics if needed
	if promMetrics == nil {
		promMetrics = NewPrometheusMetricsCollector()
	}
	// Add initialization logic if needed
	return nil
}

// Stop stops metrics collection
func (mc *MetricsCollector) Stop(ctx context.Context) {
	// Add cleanup logic if needed
}

// RecordServiceStart records when the service starts
func (mc *MetricsCollector) RecordServiceStart() {
	// Update prometheus metrics if available
	if promMetrics != nil {
		ServiceLifecycle.WithLabelValues("start").Inc()
	}
}

// RecordServiceStop records when the service stops
func (mc *MetricsCollector) RecordServiceStop() {
	// Update prometheus metrics if available
	if promMetrics != nil {
		ServiceLifecycle.WithLabelValues("stop").Inc()
	}
}

// RecordStrategyPnL records the PnL for a strategy
func (mc *MetricsCollector) RecordStrategyPnL(strategy string, pnl float64) {
	// Update prometheus metrics if available
	if promMetrics != nil {
		promMetrics.UpdateStrategyPnL("total", strategy, pnl)
	}
}

// RecordOrderLatency records the latency of order operations
func (mc *MetricsCollector) RecordOrderLatency(operation string, duration time.Duration) {
	// Update prometheus metrics if available
	if promMetrics != nil {
		OrderPlacementLatency.WithLabelValues("total", operation, "limit").Observe(duration.Seconds())
	}
}

// RecordInventoryPosition records inventory position
func (mc *MetricsCollector) RecordInventoryPosition(pair string, amount float64) {
	// Update prometheus metrics if available
	if promMetrics != nil {
		InventoryValue.WithLabelValues(pair).Set(amount)
	}
}

// RecordRiskEvent records a risk event
func (mc *MetricsCollector) RecordRiskEvent(eventType string, value float64) {
	// Update prometheus metrics if available
	if promMetrics != nil {
		RiskMetrics.WithLabelValues(eventType).Set(value)
	}
}

// RecordRiskEventTotal records a risk event count
func (mc *MetricsCollector) RecordRiskEventTotal(eventType, severity string) {
	// Update prometheus metrics if available
	if promMetrics != nil {
		RiskEventsTotal.WithLabelValues(eventType, severity).Inc()
	}
}
