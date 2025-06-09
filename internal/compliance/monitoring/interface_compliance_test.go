package monitoring

import (
	"context"
	"time"

	"github.com/Aidin1998/finalex/internal/compliance/interfaces"
	"github.com/google/uuid"
)

// This file ensures that MonitoringService implements the MonitoringService interface
// If this compiles, then the interface is fully implemented

func verifyInterfaceCompliance() {
	var _ interfaces.MonitoringService = (*MonitoringService)(nil)
}

// Verify we can call all interface methods
func testAllMethods(ctx context.Context, svc interfaces.MonitoringService) {
	// Event processing
	_ = svc.IngestEvent(ctx, &interfaces.ComplianceEvent{})
	_ = svc.IngestBatch(ctx, []*interfaces.ComplianceEvent{})

	// Real-time monitoring
	_ = svc.StartMonitoring(ctx)
	_ = svc.StopMonitoring(ctx)
	_, _ = svc.GetMonitoringStatus(ctx)

	// Alerting
	_ = svc.RegisterAlertHandler(nil)
	_, _ = svc.GetActiveAlerts(ctx)
	_, _ = svc.GetAlerts(ctx, &interfaces.AlertFilter{})
	_, _ = svc.GetAlert(ctx, uuid.Nil)
	_ = svc.UpdateAlertStatus(ctx, uuid.Nil, "")
	_ = svc.GenerateAlert(ctx, &interfaces.MonitoringAlert{})

	// Dashboard and metrics
	_, _ = svc.GetDashboard(ctx)
	_, _ = svc.GetMetrics(ctx)
	_, _ = svc.GenerateReport(ctx, "", time.Time{}, time.Time{})

	// Policy management
	_, _ = svc.GetPolicies(ctx)
	_ = svc.CreatePolicy(ctx, &interfaces.MonitoringPolicy{})
	_ = svc.UpdatePolicy(ctx, "", &interfaces.MonitoringPolicy{})
	_ = svc.DeletePolicy(ctx, "")

	// Configuration
	_ = svc.UpdateMonitoringRules(ctx, []interface{}{})
	_, _ = svc.GetConfiguration(ctx)

	// Service lifecycle
	_ = svc.Start(ctx)
	_ = svc.Stop(ctx)
	_ = svc.HealthCheck(ctx)
	_, _ = svc.CheckHealth(ctx)
}
