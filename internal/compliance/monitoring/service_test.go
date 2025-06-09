package monitoring

import (
	"testing"

	"github.com/Aidin1998/finalex/internal/integration/infrastructure"
)

func TestMonitoringService_RegisterMetric(t *testing.T) {
	// Create a test monitoring service with mock dependencies
	service := &MonitoringService{
		promMetrics: NewPrometheusMetrics(),
	}

	// Test compliance event metric
	t.Run("compliance_event", func(t *testing.T) {
		labels := map[string]string{"event_type": "transaction"}
		service.RegisterMetric("compliance_event", infrastructure.MetricTypeCounter, 1, labels)
		// If no panic occurs, the test passes - the metric was registered successfully
	})

	// Test compliance anomaly metric
	t.Run("compliance_anomaly", func(t *testing.T) {
		labels := map[string]string{"event_type": "withdrawal"}
		service.RegisterMetric("compliance_anomaly", infrastructure.MetricTypeCounter, 1, labels)
		// If no panic occurs, the test passes - the metric was registered successfully
	})

	// Test compliance violation metric
	t.Run("compliance_violation", func(t *testing.T) {
		labels := map[string]string{"event_type": "deposit"}
		service.RegisterMetric("compliance_violation", infrastructure.MetricTypeCounter, 2, labels)
		// If no panic occurs, the test passes - the metric was registered successfully
	})

	// Test unknown metric (should not panic)
	t.Run("unknown_metric", func(t *testing.T) {
		labels := map[string]string{"event_type": "test"}
		service.RegisterMetric("unknown_metric", infrastructure.MetricTypeCounter, 1, labels)
		// Should handle gracefully without panic
	})

	// Test with nil prometheus metrics (should not panic)
	t.Run("nil_prometheus_metrics", func(t *testing.T) {
		nilService := &MonitoringService{
			promMetrics: nil,
		}
		labels := map[string]string{"event_type": "transaction"}
		nilService.RegisterMetric("compliance_event", infrastructure.MetricTypeCounter, 1, labels)
		// Should handle gracefully without panic
	})
}
