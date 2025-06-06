package compliance

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/pkg/errors"
	"gorm.io/gorm"

	"github.com/Aidin1998/finalex/internal/compliance/interfaces"
)

// OrchestrationService coordinates all compliance components
type OrchestrationService struct {
	db                *gorm.DB
	auditSvc          interfaces.AuditService
	complianceSvc     interfaces.ComplianceService
	manipulationSvc   interfaces.ManipulationDetectionService
	monitoringSvc     interfaces.MonitoringService
	healthCheckers    map[string]interfaces.HealthChecker
	mu                sync.RWMutex
	stopChan          chan struct{}
	healthCheckTicker *time.Ticker
}

// NewOrchestrationService creates a new orchestration service
func NewOrchestrationService(
	db *gorm.DB,
	auditSvc interfaces.AuditService,
	complianceSvc interfaces.ComplianceService,
	manipulationSvc interfaces.ManipulationDetectionService,
	monitoringSvc interfaces.MonitoringService,
) *OrchestrationService {
	return &OrchestrationService{
		db:              db,
		auditSvc:        auditSvc,
		complianceSvc:   complianceSvc,
		manipulationSvc: manipulationSvc,
		monitoringSvc:   monitoringSvc,
		healthCheckers:  make(map[string]interfaces.HealthChecker),
		stopChan:        make(chan struct{}),
	}
}

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
func (o *OrchestrationService) Stop() error {
	close(o.stopChan)

	if o.healthCheckTicker != nil {
		o.healthCheckTicker.Stop()
	}

	var errors []error

	// Stop services in reverse order
	if err := o.monitoringSvc.Stop(); err != nil {
		errors = append(errors, fmt.Errorf("monitoring service stop error: %w", err))
	}

	if err := o.manipulationSvc.Stop(); err != nil {
		errors = append(errors, fmt.Errorf("manipulation service stop error: %w", err))
	}

	if err := o.complianceSvc.Stop(); err != nil {
		errors = append(errors, fmt.Errorf("compliance service stop error: %w", err))
	}

	if err := o.auditSvc.Stop(); err != nil {
		errors = append(errors, fmt.Errorf("audit service stop error: %w", err))
	}

	if len(errors) > 0 {
		return fmt.Errorf("errors during shutdown: %v", errors)
	}

	return nil
}

// ProcessComplianceRequest orchestrates a complete compliance check
func (o *OrchestrationService) ProcessComplianceRequest(ctx context.Context, request interfaces.ComplianceRequest) (*interfaces.ComplianceResult, error) {
	// Start audit trail
	auditEvent := interfaces.AuditEvent{
		EventType:  "compliance_request_started",
		UserID:     request.UserID,
		Action:     "process_compliance",
		EntityType: "compliance_request",
		EntityID:   request.RequestID,
		Metadata:   map[string]interface{}{"request_type": request.RequestType},
		Timestamp:  time.Now(),
		Severity:   "info",
	}
	if err := o.auditSvc.LogEvent(ctx, auditEvent); err != nil {
		return nil, errors.Wrap(err, "failed to log audit event")
	}

	// Process compliance check
	result, err := o.complianceSvc.ProcessRequest(ctx, request)
	if err != nil {
		// Log error and generate monitoring alert
		o.handleComplianceError(ctx, request, err)
		return nil, errors.Wrap(err, "compliance processing failed")
	}

	// Check for manipulation patterns if this is a transaction
	if request.RequestType == "transaction" && request.TransactionData != nil {
		manipulationResult, err := o.manipulationSvc.AnalyzeTransaction(ctx, *request.TransactionData)
		if err != nil {
			// Log but don't fail the entire process
			o.logError(ctx, "manipulation analysis failed", err, request.UserID)
		} else if manipulationResult.RiskScore > 0.7 {
			// High manipulation risk - update compliance result
			result.RiskLevel = "high"
			result.RequiresManualReview = true
			result.Flags = append(result.Flags, "high_manipulation_risk")
		}
	}

	// Generate monitoring alerts based on result
	if err := o.generateMonitoringAlerts(ctx, request, *result); err != nil {
		o.logError(ctx, "failed to generate monitoring alerts", err, request.UserID)
	}

	// Complete audit trail
	auditEvent = interfaces.AuditEvent{
		EventType:  "compliance_request_completed",
		UserID:     request.UserID,
		Action:     "complete_compliance",
		EntityType: "compliance_request",
		EntityID:   request.RequestID,
		NewValue:   fmt.Sprintf("status: %s, risk: %s", result.Status, result.RiskLevel),
		Metadata:   map[string]interface{}{"result_status": result.Status, "risk_level": result.RiskLevel},
		Timestamp:  time.Now(),
		Severity:   "info",
	}
	if err := o.auditSvc.LogEvent(ctx, auditEvent); err != nil {
		o.logError(ctx, "failed to complete audit trail", err, request.UserID)
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
	auditMetrics := o.auditSvc.GetMetrics()
	complianceMetrics := o.complianceSvc.GetMetrics()
	manipulationMetrics := o.manipulationSvc.GetMetrics()
	monitoringMetrics := o.monitoringSvc.GetMetrics()

	return &interfaces.ComplianceMetrics{
		AuditEvents:           auditMetrics.EventsProcessed,
		ComplianceChecks:      complianceMetrics.RequestsProcessed,
		ManipulationDetected:  manipulationMetrics.SuspiciousActivities,
		AlertsGenerated:       monitoringMetrics.AlertsGenerated,
		AverageProcessingTime: complianceMetrics.AverageProcessingTime,
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
	// Subscribe monitoring service to compliance events
	o.monitoringSvc.Subscribe("compliance_violation", &ComplianceAlertSubscriber{
		orchestrationSvc: o,
	})

	o.monitoringSvc.Subscribe("manipulation_detected", &ManipulationAlertSubscriber{
		orchestrationSvc: o,
	})

	o.monitoringSvc.Subscribe("audit_failure", &AuditAlertSubscriber{
		orchestrationSvc: o,
	})
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
						AlertType: "service_unhealthy",
						Severity:  "high",
						Message:   fmt.Sprintf("Service %s is unhealthy: %s", serviceName, serviceHealth.Message),
						Data: map[string]interface{}{
							"service":     serviceName,
							"status":      serviceHealth.Status,
							"error_count": serviceHealth.ErrorCount,
						},
					}
					o.monitoringSvc.GenerateAlert(ctx, alert)
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
func (o *OrchestrationService) handleComplianceError(ctx context.Context, request interfaces.ComplianceRequest, err error) {
	// Log audit event
	auditEvent := interfaces.AuditEvent{
		EventType:  "compliance_request_failed",
		UserID:     request.UserID,
		Action:     "process_compliance",
		EntityType: "compliance_request",
		EntityID:   request.RequestID,
		NewValue:   err.Error(),
		Metadata:   map[string]interface{}{"error": err.Error(), "request_type": request.RequestType},
		Timestamp:  time.Now(),
		Severity:   "error",
	}
	o.auditSvc.LogEvent(ctx, auditEvent)

	// Generate monitoring alert
	alert := interfaces.MonitoringAlert{
		UserID:    request.UserID,
		AlertType: "compliance_error",
		Severity:  "high",
		Message:   fmt.Sprintf("Compliance processing failed for request %s: %s", request.RequestID, err.Error()),
		Data: map[string]interface{}{
			"request_id":   request.RequestID,
			"request_type": request.RequestType,
			"error":        err.Error(),
		},
	}
	o.monitoringSvc.GenerateAlert(ctx, alert)
}

// generateMonitoringAlerts generates appropriate monitoring alerts based on compliance results
func (o *OrchestrationService) generateMonitoringAlerts(ctx context.Context, request interfaces.ComplianceRequest, result interfaces.ComplianceResult) error {
	if result.Status == "rejected" {
		alert := interfaces.MonitoringAlert{
			UserID:    request.UserID,
			AlertType: "compliance_violation",
			Severity:  "high",
			Message:   fmt.Sprintf("Compliance violation detected for user %s", request.UserID),
			Data: map[string]interface{}{
				"request_id":   request.RequestID,
				"request_type": request.RequestType,
				"risk_level":   result.RiskLevel,
				"flags":        result.Flags,
			},
		}
		return o.monitoringSvc.GenerateAlert(ctx, alert)
	}

	if result.RequiresManualReview {
		alert := interfaces.MonitoringAlert{
			UserID:    request.UserID,
			AlertType: "manual_review_required",
			Severity:  "medium",
			Message:   fmt.Sprintf("Manual review required for user %s", request.UserID),
			Data: map[string]interface{}{
				"request_id":   request.RequestID,
				"request_type": request.RequestType,
				"risk_level":   result.RiskLevel,
				"reason":       result.ReviewReason,
			},
		}
		return o.monitoringSvc.GenerateAlert(ctx, alert)
	}

	return nil
}

// logError logs errors with proper audit trail
func (o *OrchestrationService) logError(ctx context.Context, message string, err error, userID string) {
	auditEvent := interfaces.AuditEvent{
		EventType:  "system_error",
		UserID:     userID,
		Action:     "log_error",
		EntityType: "system",
		NewValue:   fmt.Sprintf("%s: %s", message, err.Error()),
		Metadata:   map[string]interface{}{"error": err.Error(), "message": message},
		Timestamp:  time.Now(),
		Severity:   "error",
	}
	o.auditSvc.LogEvent(ctx, auditEvent)
}

// Alert subscribers for inter-service communication

// ComplianceAlertSubscriber handles compliance violation alerts
type ComplianceAlertSubscriber struct {
	orchestrationSvc *OrchestrationService
}

func (s *ComplianceAlertSubscriber) OnAlert(ctx context.Context, alert interfaces.MonitoringAlert) error {
	// Handle compliance violation - could trigger additional checks or notifications
	return nil
}

// ManipulationAlertSubscriber handles manipulation detection alerts
type ManipulationAlertSubscriber struct {
	orchestrationSvc *OrchestrationService
}

func (s *ManipulationAlertSubscriber) OnAlert(ctx context.Context, alert interfaces.MonitoringAlert) error {
	// Handle manipulation detection - could trigger enhanced monitoring
	return nil
}

// AuditAlertSubscriber handles audit system alerts
type AuditAlertSubscriber struct {
	orchestrationSvc *OrchestrationService
}

func (s *AuditAlertSubscriber) OnAlert(ctx context.Context, alert interfaces.MonitoringAlert) error {
	// Handle audit failures - critical system issue
	return nil
}
