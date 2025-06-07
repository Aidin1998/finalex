package compliance

import (
	"context"
	"fmt"

	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"

	"github.com/Aidin1998/finalex/internal/compliance/interfaces"
)

// Start initializes all compliance services
func (o *OrchestrationService) Start(ctx context.Context) error {
	// Start audit service
	if err := o.auditSvc.Start(ctx); err != nil {
		return errors.Wrap(err, "failed to start audit service")
	}

	// Start compliance service
	if err := o.complianceSvc.Start(ctx); err != nil {
		return errors.Wrap(err, "failed to start compliance service")
	}

	// Start manipulation detection service
	if err := o.manipulationSvc.Start(ctx); err != nil {
		return errors.Wrap(err, "failed to start manipulation detection service")
	}

	// Start monitoring service
	if err := o.monitoringSvc.Start(ctx); err != nil {
		return errors.Wrap(err, "failed to start monitoring service")
	}

	// Register health checkers
	o.registerHealthCheckers()

	// Start health check routine
	o.healthCheckTicker = time.NewTicker(30 * time.Second)
	go o.healthCheckRoutine(ctx)

	// Setup inter-service communication
	o.setupInterServiceCommunication()

	return nil
}

// Stop gracefully shuts down all compliance services
func (o *OrchestrationService) Stop(ctx context.Context) error {
	close(o.stopChan)

	if o.healthCheckTicker != nil {
		o.healthCheckTicker.Stop()
	}

	var errors []error

	// Stop services in reverse order
	if err := o.monitoringSvc.Stop(ctx); err != nil {
		errors = append(errors, fmt.Errorf("monitoring service stop error: %w", err))
	}

	if err := o.manipulationSvc.Stop(ctx); err != nil {
		errors = append(errors, fmt.Errorf("manipulation service stop error: %w", err))
	}

	if err := o.complianceSvc.Stop(ctx); err != nil {
		errors = append(errors, fmt.Errorf("compliance service stop error: %w", err))
	}

	if err := o.auditSvc.Stop(ctx); err != nil {
		errors = append(errors, fmt.Errorf("audit service stop error: %w", err))
	}

	if len(errors) > 0 {
		return fmt.Errorf("errors during shutdown: %v", errors)
	}

	return nil
}

// ProcessComplianceRequest orchestrates a complete compliance check
func (o *OrchestrationService) ProcessComplianceRequest(ctx context.Context, request *interfaces.ComplianceRequest) (*interfaces.ComplianceResult, error) {
	// Start audit trail
	auditEvent := interfaces.AuditEvent{
		ID:          uuid.New(),
		UserID:      &request.UserID,
		EventType:   "compliance_request_started",
		Category:    "compliance",
		Severity:    "info",
		Description: fmt.Sprintf("Processing compliance request for user %s", request.UserID.String()),
		Metadata:    map[string]interface{}{"activity_type": request.ActivityType.String()},
		Timestamp:   time.Now(),
	}
	if err := o.auditSvc.LogEvent(ctx, &auditEvent); err != nil {
		return nil, errors.Wrap(err, "failed to log audit event")
	}

	// Process compliance check
	result, err := o.complianceSvc.CheckCompliance(ctx, request)
	if err != nil {
		// Log error and generate monitoring alert
		o.handleComplianceError(ctx, request, err)
		return nil, errors.Wrap(err, "compliance processing failed")
	}

	// Check for manipulation patterns if this is a trade or transfer
	if (request.ActivityType == interfaces.ActivityTrade || request.ActivityType == interfaces.ActivityTransfer) && request.Amount != nil {
		alertFilter := &interfaces.AlertFilter{
			UserID: request.UserID.String(),
		}
		alerts, err := o.manipulationSvc.GetAlerts(ctx, alertFilter)
		if err != nil {
			// Log but don't fail the entire process
			o.logError(ctx, "manipulation analysis failed", err, request.UserID.String())
		} else if len(alerts) > 0 {
			// High manipulation risk - update compliance result
			result.RiskLevel = interfaces.RiskLevelHigh
			result.RequiresReview = true
			result.Flags = append(result.Flags, "high_manipulation_risk")
		}
	}

	// Generate monitoring alerts based on result
	if err := o.generateMonitoringAlerts(ctx, request, result); err != nil {
		o.logError(ctx, "failed to generate monitoring alerts", err, request.UserID.String())
	}

	// Complete audit trail
	auditEvent = interfaces.AuditEvent{
		ID:          uuid.New(),
		UserID:      &request.UserID,
		EventType:   "compliance_request_completed",
		Category:    "compliance",
		Severity:    "info",
		Description: fmt.Sprintf("Completed compliance request - Status: %s, Risk: %s", result.Status.String(), result.RiskLevel.String()),
		Metadata:    map[string]interface{}{"status": result.Status.String(), "risk_level": result.RiskLevel.String()},
		Timestamp:   time.Now(),
	}
	if err := o.auditSvc.LogEvent(ctx, &auditEvent); err != nil {
		o.logError(ctx, "failed to complete audit trail", err, request.UserID.String())
	}

	return result, nil
}

// GetSystemHealth returns the health status of all compliance services
func (o *OrchestrationService) GetSystemHealth(ctx context.Context) (*interfaces.SystemHealth, error) {
	health := &interfaces.SystemHealth{
		Timestamp: time.Now(),
		Services:  make(map[string]interfaces.ServiceHealth),
	}

	o.mu.RLock()
	defer o.mu.RUnlock()

	// Check each registered health checker
	for serviceName, checker := range o.healthCheckers {
		serviceHealth := checker.CheckHealth(ctx)
		health.Services[serviceName] = serviceHealth

		if serviceHealth.Status != "healthy" {
			health.OverallStatus = "degraded"
		}
	}

	if health.OverallStatus == "" {
		health.OverallStatus = "healthy"
	}

	return health, nil
}

// GetComplianceMetrics returns aggregated compliance metrics
func (o *OrchestrationService) GetComplianceMetrics(ctx context.Context) (*interfaces.ComplianceMetrics, error) {
	auditMetrics, err := o.auditSvc.GetMetrics(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get audit metrics")
	}

	monitoringMetrics, err := o.monitoringSvc.GetMetrics(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get monitoring metrics")
	}

	return &interfaces.ComplianceMetrics{
		AuditEvents:           auditMetrics.TotalEvents,
		ComplianceChecks:      0, // Default value as compliance service doesn't have GetMetrics
		ManipulationDetected:  0, // Default value as manipulation service doesn't have GetMetrics
		AlertsGenerated:       monitoringMetrics.AlertsGenerated,
		AverageProcessingTime: auditMetrics.ProcessingLatency,
		Timestamp:             time.Now(),
	}, nil
}

// RegisterHealthChecker registers a health checker for a service
func (o *OrchestrationService) RegisterHealthChecker(serviceName string, checker interfaces.HealthChecker) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.healthCheckers[serviceName] = checker
}

// setupInterServiceCommunication configures communication between services
func (o *OrchestrationService) setupInterServiceCommunication() {
	// Setup alert handlers using registration pattern instead of Subscribe
	// This would be implemented based on the actual monitoring service interface
}

// registerHealthCheckers registers health checkers for all services
func (o *OrchestrationService) registerHealthCheckers() {
	if auditor, ok := o.auditSvc.(interfaces.HealthChecker); ok {
		o.RegisterHealthChecker("audit", auditor)
	}

	if complier, ok := o.complianceSvc.(interfaces.HealthChecker); ok {
		o.RegisterHealthChecker("compliance", complier)
	}

	if manipulator, ok := o.manipulationSvc.(interfaces.HealthChecker); ok {
		o.RegisterHealthChecker("manipulation", manipulator)
	}

	if monitor, ok := o.monitoringSvc.(interfaces.HealthChecker); ok {
		o.RegisterHealthChecker("monitoring", monitor)
	}
}

// healthCheckRoutine periodically checks system health
func (o *OrchestrationService) healthCheckRoutine(ctx context.Context) {
	for {
		select {
		case <-o.healthCheckTicker.C:
			health, err := o.GetSystemHealth(ctx)
			if err != nil {
				o.logError(ctx, "health check failed", err, "")
				continue
			}

			// Generate alerts for unhealthy services
			for serviceName, serviceHealth := range health.Services {
				if serviceHealth.Status != "healthy" {
					alert := interfaces.MonitoringAlert{
						ID:        uuid.New(),
						AlertType: "service_unhealthy",
						Severity:  interfaces.AlertSeverityHigh,
						Status:    interfaces.AlertStatusPending,
						Message:   fmt.Sprintf("Service %s is unhealthy", serviceName),
						Details: map[string]interface{}{
							"service": serviceName,
							"status":  serviceHealth.Status,
						},
						Timestamp: time.Now(),
					}
					o.monitoringSvc.GenerateAlert(ctx, &alert)
				}
			}

		case <-ctx.Done():
			return
		case <-o.stopChan:
			return
		}
	}
}

// handleComplianceError handles errors during compliance processing
func (o *OrchestrationService) handleComplianceError(ctx context.Context, request *interfaces.ComplianceRequest, err error) {
	// Log audit event
	auditEvent := interfaces.AuditEvent{
		ID:          uuid.New(),
		UserID:      &request.UserID,
		EventType:   "compliance_request_failed",
		Category:    "compliance",
		Severity:    "error",
		Description: fmt.Sprintf("Compliance processing failed: %s", err.Error()),
		Metadata:    map[string]interface{}{"error": err.Error(), "activity_type": request.ActivityType.String()},
		Timestamp:   time.Now(),
	}
	o.auditSvc.LogEvent(ctx, &auditEvent)

	// Generate monitoring alert
	alert := interfaces.MonitoringAlert{
		ID:        uuid.New(),
		UserID:    request.UserID.String(),
		AlertType: "compliance_error",
		Severity:  interfaces.AlertSeverityHigh,
		Status:    interfaces.AlertStatusPending,
		Message:   fmt.Sprintf("Compliance processing failed for user %s: %s", request.UserID.String(), err.Error()),
		Details: map[string]interface{}{
			"user_id":       request.UserID.String(),
			"activity_type": request.ActivityType.String(),
			"error":         err.Error(),
		},
		Timestamp: time.Now(),
	}
	o.monitoringSvc.GenerateAlert(ctx, &alert)
}

// generateMonitoringAlerts generates appropriate monitoring alerts based on compliance results
func (o *OrchestrationService) generateMonitoringAlerts(ctx context.Context, request *interfaces.ComplianceRequest, result *interfaces.ComplianceResult) error {
	if result.Status == interfaces.ComplianceStatusRejected {
		alert := interfaces.MonitoringAlert{
			ID:        uuid.New(),
			UserID:    request.UserID.String(),
			AlertType: "compliance_violation",
			Severity:  interfaces.AlertSeverityHigh,
			Status:    interfaces.AlertStatusPending,
			Message:   fmt.Sprintf("Compliance violation detected for user %s", request.UserID.String()),
			Details: map[string]interface{}{
				"user_id":       request.UserID.String(),
				"activity_type": request.ActivityType.String(),
				"risk_level":    result.RiskLevel.String(),
				"flags":         result.Flags,
			},
			Timestamp: time.Now(),
		}
		return o.monitoringSvc.GenerateAlert(ctx, &alert)
	}

	if result.RequiresReview {
		alert := interfaces.MonitoringAlert{
			ID:        uuid.New(),
			UserID:    request.UserID.String(),
			AlertType: "manual_review_required",
			Severity:  interfaces.AlertSeverityMedium,
			Status:    interfaces.AlertStatusPending,
			Message:   fmt.Sprintf("Manual review required for user %s", request.UserID.String()),
			Details: map[string]interface{}{
				"user_id":       request.UserID.String(),
				"activity_type": request.ActivityType.String(),
				"risk_level":    result.RiskLevel.String(),
				"reason":        result.Reason,
			},
			Timestamp: time.Now(),
		}
		return o.monitoringSvc.GenerateAlert(ctx, &alert)
	}

	return nil
}

// logError logs errors with proper audit trail
func (o *OrchestrationService) logError(ctx context.Context, message string, err error, userID string) {
	var userUUID *uuid.UUID
	if userID != "" {
		if id, parseErr := uuid.Parse(userID); parseErr == nil {
			userUUID = &id
		}
	}

	auditEvent := interfaces.AuditEvent{
		ID:          uuid.New(),
		UserID:      userUUID,
		EventType:   "system_error",
		Category:    "system",
		Severity:    "error",
		Description: fmt.Sprintf("%s: %s", message, err.Error()),
		Metadata:    map[string]interface{}{"error": err.Error(), "message": message},
		Timestamp:   time.Now(),
	}
	o.auditSvc.LogEvent(ctx, &auditEvent)
}

// Alert subscribers for inter-service communication

// ComplianceAlertSubscriber handles compliance violation alerts

func (s *ComplianceAlertSubscriber) OnAlert(ctx context.Context, alert interfaces.MonitoringAlert) error {
	// Handle compliance violation - could trigger additional checks or notifications
	return nil
}

func (s *ManipulationAlertSubscriber) OnAlert(ctx context.Context, alert interfaces.MonitoringAlert) error {
	// Handle manipulation detection - could trigger enhanced monitoring
	return nil
}

func (s *AuditAlertSubscriber) OnAlert(ctx context.Context, alert interfaces.MonitoringAlert) error {
	// Handle audit failures - critical system issue
	return nil
}
