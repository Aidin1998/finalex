package monitoring

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"gorm.io/gorm"

	"github.com/Aidin1998/finalex/internal/compliance/interfaces"
	"github.com/Aidin1998/finalex/internal/integration/infrastructure"
)

// Helper functions for enum conversions
func alertStatusFromString(s string) interfaces.AlertStatus {
	switch s {
	case "pending":
		return interfaces.AlertStatusPending
	case "acknowledged":
		return interfaces.AlertStatusAcknowledged
	case "investigating":
		return interfaces.AlertStatusInvestigating
	case "resolved":
		return interfaces.AlertStatusResolved
	case "dismissed":
		return interfaces.AlertStatusDismissed
	default:
		return interfaces.AlertStatusPending
	}
}

func alertSeverityFromString(s string) interfaces.AlertSeverity {
	switch s {
	case "low":
		return interfaces.AlertSeverityLow
	case "medium":
		return interfaces.AlertSeverityMedium
	case "high":
		return interfaces.AlertSeverityHigh
	case "critical":
		return interfaces.AlertSeverityCritical
	default:
		return interfaces.AlertSeverityLow
	}
}

// MonitoringService implements real-time compliance monitoring
type MonitoringService struct {
	db          *gorm.DB
	auditSvc    interfaces.AuditService
	subscribers map[string][]interfaces.AlertSubscriber
	alertChan   chan interfaces.MonitoringAlert
	policyCache map[string]*interfaces.MonitoringPolicy
	metrics     *MonitoringMetrics
	promMetrics *PrometheusMetrics
	mu          sync.RWMutex
	stopChan    chan struct{}
	workers     int
}

// MonitoringMetrics tracks monitoring performance
type MonitoringMetrics struct {
	AlertsGenerated    int64
	AlertsProcessed    int64
	PolicyUpdates      int64
	SubscriberNotified int64
	ProcessingLatency  time.Duration
	mu                 sync.RWMutex
}

// NewMonitoringService creates a new monitoring service
func NewMonitoringService(db *gorm.DB, auditSvc interfaces.AuditService, workers int) *MonitoringService {
	if workers <= 0 {
		workers = 4
	}
	return &MonitoringService{
		db:          db,
		auditSvc:    auditSvc,
		subscribers: make(map[string][]interfaces.AlertSubscriber),
		alertChan:   make(chan interfaces.MonitoringAlert, 1000),
		policyCache: make(map[string]*interfaces.MonitoringPolicy),
		metrics:     &MonitoringMetrics{},
		promMetrics: NewPrometheusMetrics(),
		stopChan:    make(chan struct{}),
		workers:     workers,
	}
}

// Start initializes the monitoring service
func (m *MonitoringService) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Load policies from database into cache
	if err := m.loadPolicies(ctx); err != nil {
		return errors.Wrap(err, "failed to load monitoring policies")
	}

	// Start alert processing workers
	for i := 0; i < m.workers; i++ {
		go m.alertWorker(ctx)
	}

	// Start metrics collector
	go m.metricsCollector(ctx)

	return nil
}

// Stop gracefully shuts down the monitoring service
func (m *MonitoringService) Stop(ctx context.Context) error {
	close(m.stopChan)
	return nil
}

// CheckHealth implements health checking for the monitoring service
func (m *MonitoringService) CheckHealth(ctx context.Context) (*interfaces.ServiceHealth, error) {
	start := time.Now()
	// Test database connectivity
	if err := m.db.WithContext(ctx).Exec("SELECT 1").Error; err != nil {
		return &interfaces.ServiceHealth{
			ServiceName:  "monitoring",
			Status:       "unhealthy",
			ResponseTime: float64(time.Since(start).Nanoseconds()) / 1000000, // Convert to milliseconds
			ErrorMessage: err.Error(),
			LastCheck:    time.Now(),
		}, nil
	}
	return &interfaces.ServiceHealth{
		ServiceName:  "monitoring",
		Status:       "healthy",
		ResponseTime: float64(time.Since(start).Nanoseconds()) / 1000000, // Convert to milliseconds
		LastCheck:    time.Now(),
	}, nil
}

// GetSystemHealth returns comprehensive system health information
func (m *MonitoringService) GetSystemHealth(ctx context.Context) (*interfaces.SystemHealth, error) {
	start := time.Now()
	return &interfaces.SystemHealth{
		OverallStatus: "healthy",
		Timestamp:     time.Now(),
		Services: map[string]interfaces.ServiceHealth{
			"monitoring": {
				ServiceName:  "monitoring",
				Status:       "healthy",
				ResponseTime: float64(time.Since(start).Nanoseconds()) / 1000000,
				LastCheck:    time.Now(),
			},
		},
		Metrics: []interfaces.MetricSummary{
			{
				Name:   "alerts_generated",
				Value:  float64(m.metrics.AlertsGenerated),
				Unit:   "count",
				Status: "normal",
			},
			{
				Name:   "alerts_processed",
				Value:  float64(m.metrics.AlertsProcessed),
				Unit:   "count",
				Status: "normal",
			},
		},
	}, nil
}

// RegisterMetric registers a metric with the monitoring service
func (m *MonitoringService) RegisterMetric(name string, metricType infrastructure.MetricType, value float64, labels map[string]string) {
	if m.promMetrics == nil {
		return
	}

	switch name {
	case "compliance_event":
		eventType := "unknown"
		if et, ok := labels["event_type"]; ok {
			eventType = et
		}
		// ComplianceChecks expects ["check_type", "result"]
		m.promMetrics.ComplianceChecks.WithLabelValues(eventType, "processed").Add(value)
	case "compliance_anomaly":
		eventType := "unknown"
		if et, ok := labels["event_type"]; ok {
			eventType = et
		}
		// ManipulationDetections expects ["pattern_type", "market", "severity"]
		m.promMetrics.ManipulationDetections.WithLabelValues("anomaly", eventType, "medium").Add(value)
	case "compliance_violation":
		eventType := "unknown"
		if et, ok := labels["event_type"]; ok {
			eventType = et
		}
		// PolicyViolations expects ["policy_type", "severity"]
		m.promMetrics.PolicyViolations.WithLabelValues(eventType, "medium").Add(value)
	}
}

// AcknowledgeAlert acknowledges an alert
func (m *MonitoringService) AcknowledgeAlert(ctx context.Context, alertID uuid.UUID, acknowledgedBy uuid.UUID, notes string) error {
	alert := &MonitoringAlertModel{}
	if err := m.db.WithContext(ctx).Where("id = ?", alertID.String()).First(alert).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("alert not found: %s", alertID.String())
		}
		return errors.Wrap(err, "failed to find alert")
	}

	oldStatus := alert.Status
	now := time.Now()

	alert.Status = interfaces.AlertStatusAcknowledged.String()
	alert.UpdatedAt = now

	// Update acknowledgment information in Data field
	if alert.Data == nil {
		alert.Data = make(map[string]interface{})
	}
	alert.Data["acknowledged_by"] = acknowledgedBy.String()
	alert.Data["acknowledged_at"] = now
	if notes != "" {
		alert.Data["acknowledgment_notes"] = notes
	}

	if err := m.db.WithContext(ctx).Save(alert).Error; err != nil {
		return errors.Wrap(err, "failed to update alert")
	}

	// Audit the acknowledgment
	if m.auditSvc != nil {
		oldStatusEnum := alertStatusFromString(oldStatus)
		auditEvent := &interfaces.AuditEvent{
			ID:          uuid.New(),
			UserID:      &acknowledgedBy,
			EventType:   "alert_acknowledged",
			Category:    "monitoring",
			Severity:    "info",
			Description: fmt.Sprintf("Alert acknowledged by user %s", acknowledgedBy.String()),
			Action:      "acknowledge_alert",
			Resource:    "monitoring_alert",
			ResourceID:  alertID.String(),
			OldValues: map[string]interface{}{
				"status": oldStatusEnum.String(),
			},
			NewValues: map[string]interface{}{
				"status":          interfaces.AlertStatusAcknowledged.String(),
				"acknowledged_by": acknowledgedBy.String(),
				"acknowledged_at": now,
			},
			Timestamp: now,
		}

		if err := m.auditSvc.LogEvent(ctx, auditEvent); err != nil {
			// Log but don't fail the operation
			fmt.Printf("Failed to log audit event: %v\n", err)
		}
	}

	return nil
}

// CreateAlert creates a new monitoring alert
func (m *MonitoringService) CreateAlert(ctx context.Context, alert *interfaces.MonitoringAlert) error {
	alertModel := &MonitoringAlertModel{
		ID:        alert.ID.String(),
		UserID:    alert.UserID,
		AlertType: alert.AlertType,
		Severity:  alert.Severity.String(),
		Message:   alert.Message,
		Data:      alert.Details, // MonitoringAlert uses Details, Model uses Data
		Status:    alert.Status.String(),
		Timestamp: alert.Timestamp,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := m.db.WithContext(ctx).Create(alertModel).Error; err != nil {
		return errors.Wrap(err, "failed to create alert")
	}

	// Send alert to processing channel
	select {
	case m.alertChan <- *alert:
		m.metrics.mu.Lock()
		m.metrics.AlertsGenerated++
		m.metrics.mu.Unlock()
	default:
		return errors.New("alert channel is full")
	}

	return nil
}

// UpdateAlertStatus updates the status of an alert
func (m *MonitoringService) UpdateAlertStatus(ctx context.Context, alertID uuid.UUID, status string) error {
	var newStatus interfaces.AlertStatus
	switch status {
	case "pending":
		newStatus = interfaces.AlertStatusPending
	case "acknowledged":
		newStatus = interfaces.AlertStatusAcknowledged
	case "investigating":
		newStatus = interfaces.AlertStatusInvestigating
	case "resolved":
		newStatus = interfaces.AlertStatusResolved
	case "dismissed":
		newStatus = interfaces.AlertStatusDismissed
	default:
		return fmt.Errorf("invalid status: %s", status)
	}

	alert := &MonitoringAlertModel{}
	if err := m.db.WithContext(ctx).Where("id = ?", alertID.String()).First(alert).Error; err != nil {
		return errors.Wrap(err, "failed to find alert")
	}

	oldStatus := alert.Status
	alert.Status = newStatus.String()
	alert.UpdatedAt = time.Now()

	if err := m.db.WithContext(ctx).Save(alert).Error; err != nil {
		return errors.Wrap(err, "failed to update alert status")
	}

	// Audit the status change
	if m.auditSvc != nil {
		userID, _ := uuid.Parse(alert.UserID)
		oldStatusEnum := alertStatusFromString(oldStatus)
		auditEvent := &interfaces.AuditEvent{
			ID:          uuid.New(),
			UserID:      &userID,
			EventType:   "alert_status_updated",
			Category:    "monitoring",
			Severity:    "info",
			Description: fmt.Sprintf("Alert status updated from %s to %s", oldStatusEnum.String(), newStatus.String()),
			Action:      "update_alert_status",
			Resource:    "monitoring_alert",
			ResourceID:  alertID.String(),
			OldValues: map[string]interface{}{
				"status": oldStatusEnum.String(),
			},
			NewValues: map[string]interface{}{
				"status": newStatus.String(),
			},
			Timestamp: time.Now(),
		}

		if err := m.auditSvc.LogEvent(ctx, auditEvent); err != nil {
			fmt.Printf("Failed to log audit event: %v\n", err)
		}
	}

	return nil
}

// GetAlerts retrieves alerts with filtering options
func (m *MonitoringService) GetAlerts(ctx context.Context, userID *uuid.UUID, status *interfaces.AlertStatus, limit, offset int) ([]*interfaces.MonitoringAlert, error) {
	query := m.db.WithContext(ctx).Model(&MonitoringAlertModel{})

	if userID != nil {
		query = query.Where("user_id = ?", userID.String())
	}

	if status != nil {
		query = query.Where("status = ?", status.String())
	}

	if limit > 0 {
		query = query.Limit(limit)
	}

	if offset > 0 {
		query = query.Offset(offset)
	}

	var alertModels []*MonitoringAlertModel
	if err := query.Order("created_at DESC").Find(&alertModels).Error; err != nil {
		return nil, errors.Wrap(err, "failed to retrieve alerts")
	}

	alerts := make([]*interfaces.MonitoringAlert, len(alertModels))
	for i, model := range alertModels {
		id, _ := uuid.Parse(model.ID)
		alerts[i] = &interfaces.MonitoringAlert{
			ID:        id,
			UserID:    model.UserID,
			AlertType: model.AlertType,
			Severity:  alertSeverityFromString(model.Severity),
			Message:   model.Message,
			Details:   model.Data, // MonitoringAlert uses Details, Model uses Data
			Status:    alertStatusFromString(model.Status),
			Timestamp: model.Timestamp,
		}
	}

	return alerts, nil
}

// CreatePolicy creates a new monitoring policy
func (m *MonitoringService) CreatePolicy(ctx context.Context, policy *interfaces.MonitoringPolicy) error {
	// Convert conditions and actions to JSON for storage
	conditionsJSON, err := json.Marshal(policy.Conditions)
	if err != nil {
		return errors.Wrap(err, "failed to marshal conditions")
	}

	actionsJSON, err := json.Marshal(policy.Actions)
	if err != nil {
		return errors.Wrap(err, "failed to marshal actions")
	}

	policyModel := &MonitoringPolicyModel{
		ID:          policy.ID,
		Name:        policy.Name,
		Description: policy.Description,
		Enabled:     policy.Enabled,
		Conditions: map[string]interface{}{
			"conditions_json": string(conditionsJSON),
		},
		Actions: map[string]interface{}{
			"actions_json": string(actionsJSON),
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := m.db.WithContext(ctx).Create(policyModel).Error; err != nil {
		return errors.Wrap(err, "failed to create policy")
	}

	// Update cache
	m.mu.Lock()
	m.policyCache[policy.ID] = policy
	m.mu.Unlock()

	m.metrics.mu.Lock()
	m.metrics.PolicyUpdates++
	m.metrics.mu.Unlock()

	return nil
}

// UpdatePolicy updates an existing monitoring policy
func (m *MonitoringService) UpdatePolicy(ctx context.Context, policy *interfaces.MonitoringPolicy) error {
	conditionsJSON, err := json.Marshal(policy.Conditions)
	if err != nil {
		return errors.Wrap(err, "failed to marshal conditions")
	}

	actionsJSON, err := json.Marshal(policy.Actions)
	if err != nil {
		return errors.Wrap(err, "failed to marshal actions")
	}

	updates := map[string]interface{}{
		"name":        policy.Name,
		"description": policy.Description,
		"enabled":     policy.Enabled,
		"conditions": map[string]interface{}{
			"conditions_json": string(conditionsJSON),
		},
		"actions": map[string]interface{}{
			"actions_json": string(actionsJSON),
		},
		"updated_at": time.Now(),
	}

	if err := m.db.WithContext(ctx).Model(&MonitoringPolicyModel{}).Where("id = ?", policy.ID).Updates(updates).Error; err != nil {
		return errors.Wrap(err, "failed to update policy")
	}

	// Update cache
	m.mu.Lock()
	m.policyCache[policy.ID] = policy
	m.mu.Unlock()

	m.metrics.mu.Lock()
	m.metrics.PolicyUpdates++
	m.metrics.mu.Unlock()

	return nil
}

// GetPolicy retrieves a monitoring policy by ID
func (m *MonitoringService) GetPolicy(ctx context.Context, policyID string) (*interfaces.MonitoringPolicy, error) {
	// Check cache first
	m.mu.RLock()
	if policy, exists := m.policyCache[policyID]; exists {
		m.mu.RUnlock()
		return policy, nil
	}
	m.mu.RUnlock()

	// Load from database
	var policyModel MonitoringPolicyModel
	if err := m.db.WithContext(ctx).Where("id = ?", policyID).First(&policyModel).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("policy not found: %s", policyID)
		}
		return nil, errors.Wrap(err, "failed to retrieve policy")
	}

	// Convert to interface type
	policy := &interfaces.MonitoringPolicy{
		ID:          policyModel.ID,
		Name:        policyModel.Name,
		Description: policyModel.Description,
		Enabled:     policyModel.Enabled,
	}

	// Parse conditions and actions from JSON
	if conditionsData, ok := policyModel.Conditions["conditions_json"].(string); ok {
		json.Unmarshal([]byte(conditionsData), &policy.Conditions)
	}

	if actionsData, ok := policyModel.Actions["actions_json"].(string); ok {
		json.Unmarshal([]byte(actionsData), &policy.Actions)
	}

	// Update cache
	m.mu.Lock()
	m.policyCache[policyID] = policy
	m.mu.Unlock()

	return policy, nil
}

// DeletePolicy deletes a monitoring policy by ID
func (m *MonitoringService) DeletePolicy(ctx context.Context, policyID string) error {
	if err := m.db.WithContext(ctx).Where("id = ?", policyID).Delete(&MonitoringPolicyModel{}).Error; err != nil {
		return errors.Wrap(err, "failed to delete policy")
	}

	// Remove from cache
	m.mu.Lock()
	delete(m.policyCache, policyID)
	m.mu.Unlock()

	// Audit the deletion
	if m.auditSvc != nil {
		auditEvent := &interfaces.AuditEvent{
			ID:          uuid.New(),
			EventType:   "policy_deleted",
			Category:    "monitoring",
			Severity:    "info",
			Description: fmt.Sprintf("Monitoring policy deleted: %s", policyID),
			Action:      "delete_policy",
			Resource:    "monitoring_policy",
			ResourceID:  policyID,
			Timestamp:   time.Now(),
		}

		if err := m.auditSvc.LogEvent(ctx, auditEvent); err != nil {
			fmt.Printf("Failed to log audit event: %v\n", err)
		}
	}

	return nil
}

// Subscribe adds an alert subscriber
func (m *MonitoringService) Subscribe(alertType string, subscriber interfaces.AlertSubscriber) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.subscribers[alertType] = append(m.subscribers[alertType], subscriber)
}

// Unsubscribe removes an alert subscriber
func (m *MonitoringService) Unsubscribe(alertType string, subscriber interfaces.AlertSubscriber) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if subs, exists := m.subscribers[alertType]; exists {
		for i, sub := range subs {
			if sub == subscriber {
				m.subscribers[alertType] = append(subs[:i], subs[i+1:]...)
				break
			}
		}
	}
}

// MigrateTables creates the necessary database tables
func (m *MonitoringService) MigrateTables(ctx context.Context) error {
	return m.db.WithContext(ctx).AutoMigrate(
		&MonitoringAlertModel{},
		&MonitoringPolicyModel{},
	)
}

// Private helper methods

func (m *MonitoringService) loadPolicies(ctx context.Context) error {
	var policies []MonitoringPolicyModel
	if err := m.db.WithContext(ctx).Where("enabled = ?", true).Find(&policies).Error; err != nil {
		return errors.Wrap(err, "failed to load policies")
	}

	m.policyCache = make(map[string]*interfaces.MonitoringPolicy)
	for _, p := range policies {
		// Parse conditions and actions from JSON
		var conditions []interfaces.PolicyCondition
		var actions []interfaces.PolicyAction

		if conditionsData, ok := p.Conditions["conditions_json"].(string); ok {
			json.Unmarshal([]byte(conditionsData), &conditions)
		}

		if actionsData, ok := p.Actions["actions_json"].(string); ok {
			json.Unmarshal([]byte(actionsData), &actions)
		}

		policy := &interfaces.MonitoringPolicy{
			ID:          p.ID,
			Name:        p.Name,
			Description: p.Description,
			Enabled:     p.Enabled,
			Conditions:  conditions,
			Actions:     actions,
		}

		m.policyCache[p.ID] = policy
	}

	return nil
}

func (m *MonitoringService) alertWorker(ctx context.Context) {
	for {
		select {
		case <-m.stopChan:
			return
		case <-ctx.Done():
			return
		case alert := <-m.alertChan:
			if err := m.processAlert(ctx, &alert); err != nil {
				fmt.Printf("Error processing alert: %v\n", err)
			}
		}
	}
}

func (m *MonitoringService) processAlert(ctx context.Context, alert *interfaces.MonitoringAlert) error {
	start := time.Now()
	defer func() {
		m.metrics.mu.Lock()
		m.metrics.AlertsProcessed++
		m.metrics.ProcessingLatency = time.Since(start)
		m.metrics.mu.Unlock()
	}()

	// Notify subscribers
	m.mu.RLock()
	subscribers := m.subscribers[alert.AlertType]
	m.mu.RUnlock()

	for _, subscriber := range subscribers {
		go func(sub interfaces.AlertSubscriber) {
			if err := sub.OnAlert(*alert); err != nil {
				fmt.Printf("Subscriber notification failed: %v\n", err)
			} else {
				m.metrics.mu.Lock()
				m.metrics.SubscriberNotified++
				m.metrics.mu.Unlock()
			}
		}(subscriber)
	}

	return nil
}

func (m *MonitoringService) metricsCollector(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopChan:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Collect and store metrics periodically
			m.collectMetrics(ctx)
		}
	}
}

func (m *MonitoringService) collectMetrics(ctx context.Context) {
	// Implementation for periodic metrics collection
	// This could include system metrics, alert statistics, etc.
}

// GetPrometheusMetrics returns a new PrometheusMetrics instance
func (m *MonitoringService) GetPrometheusMetrics() *PrometheusMetrics {
	return NewPrometheusMetrics()
}

// GetHealthStatus returns health status information for the monitoring service
func (m *MonitoringService) GetHealthStatus() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return map[string]interface{}{
		"service":    "healthy",
		"database":   "healthy",
		"queue_size": len(m.alertChan),
		"workers":    m.workers,
		"metrics": map[string]interface{}{
			"alerts_generated": m.metrics.AlertsGenerated,
			"alerts_processed": m.metrics.AlertsProcessed,
			"policy_updates":   m.metrics.PolicyUpdates,
		},
	}
}
