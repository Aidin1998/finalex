// Compatibility layer for integrating MarketMaker with observability components
package marketmaker

import "context"

// --------------- Logger extensions ---------------

// LogStrategyEvent is a compatibility method for structured logger
func (sl *StructuredLogger) LogStrategyEvent(ctx context.Context, strategy, eventType string, details map[string]interface{}) {
	if details == nil {
		details = make(map[string]interface{})
	}
	details["strategy"] = strategy
	details["event_type"] = eventType
	sl.LogInfo(ctx, "Strategy event: "+eventType, details)
}

// --------------- Metrics extensions ---------------

// RecordStrategyEvent is a compatibility method for metrics collector
func (mc *MetricsCollector) RecordStrategyEvent(strategy, eventType string) {
	// This is a placeholder for actual metrics recording
}

// GetAllMetrics returns all metrics from the collector
func (mc *MetricsCollector) GetAllMetrics() map[string]interface{} {
	// Placeholder implementation
	return map[string]interface{}{
		"orders_total": 0,
		"pnl_total":    0.0,
	}
}
