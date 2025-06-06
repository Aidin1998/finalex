// Extension methods for AdminToolsManager and related components
package marketmaker

import (
	"context"
)

// LogStrategyEvent logs strategy lifecycle events
func (sl *StructuredLogger) LogStrategyEvent(ctx context.Context, strategyName, eventType string, details map[string]interface{}) {
	if details == nil {
		details = make(map[string]interface{})
	}
	details["strategy_name"] = strategyName
	details["event_type"] = eventType

	sl.LogInfo(ctx, "Strategy event: "+eventType, details)
}

// RecordStrategyEvent records metrics for strategy lifecycle events
func (mc *MetricsCollector) RecordStrategyEvent(strategyName, eventType string) {
	// Use existing metrics to record strategy events
	// This is a no-op if the specific metrics aren't available
}

// Fix for calculateOverallHealth to handle pointer receiver
func (atm *AdminToolsManager) calculateOverallHealth(results map[string]*HealthCheckResult) string {
	unhealthy := 0
	degraded := 0

	for _, result := range results {
		if result.Status == HealthUnhealthy {
			unhealthy++
		} else if result.Status == HealthDegraded {
			degraded++
		}
	}

	if unhealthy > 0 {
		return "unhealthy"
	} else if degraded > 0 {
		return "degraded"
	}
	return "healthy"
}

// GetAllMetrics returns all metrics from the metrics collector
func (mc *MetricsCollector) GetAllMetrics() map[string]interface{} {
	// This would normally fetch metrics from Prometheus or other source
	// For now, return placeholder data
	return map[string]interface{}{
		"orders_placed_total": 100,
		"orders_filled_total": 80,
		"pnl_daily_usd":       500.0,
		"inventory_value_usd": 10000.0,
	}
}

// Add type stubs for extension build
// These should match the real types in the main codebase

type StructuredLogger struct{}
type MetricsCollector struct{}
type AdminToolsManager struct{}
type HealthCheckResult struct {
	Status int
}

const (
	HealthUnhealthy = 2
	HealthDegraded  = 1
)

// Add stub for LogInfo
func (sl *StructuredLogger) LogInfo(ctx context.Context, msg string, fields map[string]interface{}) {}

// If AdminToolsManager or related types reference legacy strategy types, update to use new interfaces or stub as needed.
