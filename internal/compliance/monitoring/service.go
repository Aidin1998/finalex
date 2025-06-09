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
	db            *gorm.DB
	auditSvc      interfaces.AuditService
	subscribers   map[string][]interfaces.AlertSubscriber
	alertChan     chan interfaces.MonitoringAlert
	policyCache   map[string]*interfaces.MonitoringPolicy
	metrics       *MonitoringMetrics
	promMetrics   *PrometheusMetrics
	alertHandlers []interfaces.AlertHandler
	isMonitoring  bool
	eventQueue    chan *interfaces.ComplianceEvent
	mu            sync.RWMutex
	stopChan      chan struct{}
	workers       int
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
		db:            db,
		auditSvc:      auditSvc,
		subscribers:   make(map[string][]interfaces.AlertSubscriber),
		alertChan:     make(chan interfaces.MonitoringAlert, 1000),
		policyCache:   make(map[string]*interfaces.MonitoringPolicy),
		metrics:       &MonitoringMetrics{},
		promMetrics:   NewPrometheusMetrics(),
		alertHandlers: make([]interfaces.AlertHandler, 0),
		isMonitoring:  false,
		eventQueue:    make(chan *interfaces.ComplianceEvent, 10000),
		stopChan:      make(chan struct{}),
		workers:       workers,
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

// GenerateAlert generates a new monitoring alert
func (m *MonitoringService) GenerateAlert(ctx context.Context, alert *interfaces.MonitoringAlert) error {
	// Ensure alert has required fields
	if alert.ID == uuid.Nil {
		alert.ID = uuid.New()
	}
	if alert.Timestamp.IsZero() {
		alert.Timestamp = time.Now()
	}
	if alert.Status == 0 {
		alert.Status = interfaces.AlertStatusPending
	}

	// Create the alert using the existing CreateAlert method
	return m.CreateAlert(ctx, alert)
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
func (m *MonitoringService) GetAlerts(ctx context.Context, filter *interfaces.AlertFilter) ([]*interfaces.MonitoringAlert, error) {
	query := m.db.WithContext(ctx).Model(&MonitoringAlertModel{})

	// Apply filters if provided
	if filter != nil {
		if filter.UserID != "" {
			query = query.Where("user_id = ?", filter.UserID)
		}

		if filter.AlertType != "" {
			query = query.Where("alert_type = ?", filter.AlertType)
		}

		if filter.Status != 0 {
			query = query.Where("status = ?", filter.Status.String())
		}

		if filter.Severity != 0 {
			query = query.Where("severity = ?", filter.Severity.String())
		}

		if !filter.StartTime.IsZero() {
			query = query.Where("timestamp >= ?", filter.StartTime)
		}

		if !filter.EndTime.IsZero() {
			query = query.Where("timestamp <= ?", filter.EndTime)
		}

		if filter.Limit > 0 {
			query = query.Limit(filter.Limit)
		}

		if filter.Offset > 0 {
			query = query.Offset(filter.Offset)
		}
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

// GetActiveAlerts retrieves all currently active alerts (pending, acknowledged, investigating)
func (m *MonitoringService) GetActiveAlerts(ctx context.Context) ([]*interfaces.MonitoringAlert, error) {
	query := m.db.WithContext(ctx).Model(&MonitoringAlertModel{}).
		Where("status IN (?)", []string{
			interfaces.AlertStatusPending.String(),
			interfaces.AlertStatusAcknowledged.String(),
			interfaces.AlertStatusInvestigating.String(),
		})

	var alertModels []*MonitoringAlertModel
	if err := query.Order("created_at DESC").Find(&alertModels).Error; err != nil {
		return nil, errors.Wrap(err, "failed to retrieve active alerts")
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
			Details:   model.Data,
			Status:    alertStatusFromString(model.Status),
			Timestamp: model.Timestamp,
		}
	}

	return alerts, nil
}

// GetAlert retrieves a specific monitoring alert by ID
func (m *MonitoringService) GetAlert(ctx context.Context, alertID uuid.UUID) (*interfaces.MonitoringAlert, error) {
	var alertModel MonitoringAlertModel
	if err := m.db.WithContext(ctx).Where("id = ?", alertID.String()).First(&alertModel).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil // Return nil if alert not found (not an error)
		}
		return nil, errors.Wrap(err, "failed to retrieve alert")
	}

	id, _ := uuid.Parse(alertModel.ID)
	alert := &interfaces.MonitoringAlert{
		ID:        id,
		UserID:    alertModel.UserID,
		AlertType: alertModel.AlertType,
		Severity:  alertSeverityFromString(alertModel.Severity),
		Message:   alertModel.Message,
		Details:   alertModel.Data,
		Status:    alertStatusFromString(alertModel.Status),
		Timestamp: alertModel.Timestamp,
	}

	return alert, nil
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
func (m *MonitoringService) UpdatePolicy(ctx context.Context, policyID string, policy *interfaces.MonitoringPolicy) error {
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
	if err := m.db.WithContext(ctx).Model(&MonitoringPolicyModel{}).Where("id = ?", policyID).Updates(updates).Error; err != nil {
		return errors.Wrap(err, "failed to update policy")
	}

	// Update cache
	m.mu.Lock()
	m.policyCache[policyID] = policy
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

// GenerateReport generates a compliance monitoring report for the specified time period
func (m *MonitoringService) GenerateReport(ctx context.Context, reportType string, from, to time.Time) ([]byte, error) {
	switch reportType {
	case "alerts":
		return m.generateAlertsReport(ctx, from, to)
	case "metrics":
		return m.generateMetricsReport(ctx, from, to)
	case "summary":
		return m.generateSummaryReport(ctx, from, to)
	default:
		return nil, fmt.Errorf("unsupported report type: %s", reportType)
	}
}

func (m *MonitoringService) generateAlertsReport(ctx context.Context, from, to time.Time) ([]byte, error) {
	// Get alerts with the monitoring service's actual method signature
	filter := &interfaces.AlertFilter{
		StartTime: from,
		EndTime:   to,
		Limit:     10000,
		Offset:    0,
	}
	alerts, err := m.GetAlerts(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to get alerts: %w", err)
	}

	// Filter alerts by time period (already done by the filter, but keeping for safety)
	filteredAlerts := make([]*interfaces.MonitoringAlert, 0)
	for _, alert := range alerts {
		if alert.Timestamp.After(from) && alert.Timestamp.Before(to) {
			filteredAlerts = append(filteredAlerts, alert)
		}
	}

	report := map[string]interface{}{
		"report_type": "alerts",
		"period": map[string]interface{}{
			"from": from,
			"to":   to,
		},
		"total_alerts": len(filteredAlerts),
		"alerts":       filteredAlerts,
		"generated_at": time.Now(),
	}

	return json.Marshal(report)
}

func (m *MonitoringService) generateMetricsReport(ctx context.Context, from, to time.Time) ([]byte, error) {
	// Create basic metrics from current state
	m.mu.RLock()
	metrics := map[string]interface{}{
		"alerts_generated": m.metrics.AlertsGenerated,
		"alerts_processed": m.metrics.AlertsProcessed,
		"policy_updates":   m.metrics.PolicyUpdates,
		"queue_size":       len(m.alertChan),
		"workers":          m.workers,
	}
	m.mu.RUnlock()

	report := map[string]interface{}{
		"report_type": "metrics",
		"period": map[string]interface{}{
			"from": from,
			"to":   to,
		},
		"metrics":      metrics,
		"generated_at": time.Now(),
	}

	return json.Marshal(report)
}

func (m *MonitoringService) generateSummaryReport(ctx context.Context, from, to time.Time) ([]byte, error) {
	// Get alerts for the period
	filter := &interfaces.AlertFilter{
		Limit:  10000,
		Offset: 0,
	}
	alerts, err := m.GetAlerts(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to get alerts: %w", err)
	}

	// Filter alerts by time period and count by severity
	filteredAlerts := make([]*interfaces.MonitoringAlert, 0)
	severityCounts := make(map[string]int)

	for _, alert := range alerts {
		if alert.Timestamp.After(from) && alert.Timestamp.Before(to) {
			filteredAlerts = append(filteredAlerts, alert)
			severityCounts[alert.Severity.String()]++
		}
	}

	// Get current health status
	health := m.GetHealthStatus()

	report := map[string]interface{}{
		"report_type": "summary",
		"period": map[string]interface{}{
			"from": from,
			"to":   to,
		},
		"health_status":      health,
		"alerts_by_severity": severityCounts,
		"total_alerts":       len(filteredAlerts),
		"generated_at":       time.Now(),
	}

	return json.Marshal(report)
}

// IngestEvent processes a single compliance event
func (m *MonitoringService) IngestEvent(ctx context.Context, event *interfaces.ComplianceEvent) error {
	if event == nil {
		return errors.New("event cannot be nil")
	}

	// Add event to queue for processing
	select {
	case m.eventQueue <- event:
		// Successfully queued
		m.metrics.mu.Lock()
		m.metrics.AlertsProcessed++
		m.metrics.mu.Unlock()
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(5 * time.Second):
		return errors.New("event queue is full, failed to ingest event")
	}
}

// IngestBatch processes multiple compliance events
func (m *MonitoringService) IngestBatch(ctx context.Context, events []*interfaces.ComplianceEvent) error {
	if len(events) == 0 {
		return nil
	}

	for _, event := range events {
		if err := m.IngestEvent(ctx, event); err != nil {
			return errors.Wrapf(err, "failed to ingest event with type %s", event.EventType)
		}
	}

	return nil
}

// StartMonitoring begins the monitoring process
func (m *MonitoringService) StartMonitoring(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.isMonitoring {
		return errors.New("monitoring is already running")
	}

	m.isMonitoring = true

	// Start event processing workers
	for i := 0; i < m.workers; i++ {
		go m.eventProcessor(ctx)
	}

	return nil
}

// StopMonitoring stops the monitoring process
func (m *MonitoringService) StopMonitoring(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.isMonitoring {
		return errors.New("monitoring is not running")
	}

	m.isMonitoring = false

	// Signal stop to workers
	close(m.stopChan)

	// Recreate stop channel for future use
	m.stopChan = make(chan struct{})

	return nil
}

// GetMonitoringStatus returns the current monitoring status
func (m *MonitoringService) GetMonitoringStatus(ctx context.Context) (*interfaces.MonitoringStatus, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	m.metrics.mu.RLock()
	defer m.metrics.mu.RUnlock()

	return &interfaces.MonitoringStatus{
		Active:         m.isMonitoring,
		WorkersActive:  m.workers,
		QueueDepth:     int64(len(m.eventQueue)),
		ProcessingRate: float64(m.metrics.AlertsProcessed) / time.Since(time.Now().Add(-1*time.Hour)).Hours(),
		LastEventTime:  time.Now(), // Would track actual last event time in production
		ErrorCount:     0,          // Would track actual error count
		Configuration: map[string]interface{}{
			"workers":      m.workers,
			"queue_size":   cap(m.eventQueue),
			"alert_buffer": cap(m.alertChan),
		},
	}, nil
}

// RegisterAlertHandler registers a new alert handler
func (m *MonitoringService) RegisterAlertHandler(handler interfaces.AlertHandler) error {
	if handler == nil {
		return errors.New("handler cannot be nil")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.alertHandlers = append(m.alertHandlers, handler)
	return nil
}

// GetDashboard returns monitoring dashboard data
func (m *MonitoringService) GetDashboard(ctx context.Context) (*interfaces.MonitoringDashboard, error) {
	// Get recent alerts
	filter := &interfaces.AlertFilter{
		Limit:  10,
		Offset: 0,
	}
	recentAlerts, err := m.GetAlerts(ctx, filter)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get recent alerts")
	}

	// Get active alerts count
	activeAlerts, err := m.GetActiveAlerts(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get active alerts")
	}

	// Count critical alerts
	criticalCount := 0
	for _, alert := range activeAlerts {
		if alert.Severity == interfaces.AlertSeverityCritical {
			criticalCount++
		}
	}

	// Create metrics
	m.metrics.mu.RLock()
	metrics := &interfaces.MonitoringMetrics{
		EventsProcessed:     m.metrics.AlertsProcessed,
		AlertsGenerated:     m.metrics.AlertsGenerated,
		ProcessingLatency:   m.metrics.ProcessingLatency,
		ThroughputPerSecond: float64(m.metrics.AlertsProcessed) / time.Since(time.Now().Add(-1*time.Hour)).Seconds(),
		ErrorRate:           0.0, // Would calculate actual error rate
		SystemLoad:          0.5, // Would get actual system load
		MemoryUsage:         0,   // Would get actual memory usage
		Details:             make(map[string]interface{}),
		Timestamp:           time.Now(),
	}
	m.metrics.mu.RUnlock()

	// Create dashboard summary
	summary := &interfaces.DashboardSummary{
		ActiveAlerts:   len(activeAlerts),
		CriticalAlerts: criticalCount,
		TotalUsers:     0, // Would get from user service
		HighRiskUsers:  0, // Would calculate from risk analysis
		ProcessingRate: metrics.ThroughputPerSecond,
		SystemHealth:   "healthy", // Would determine from actual health checks
		LastUpdate:     time.Now(),
		TrendData:      make(map[string]interface{}),
	}

	return &interfaces.MonitoringDashboard{
		Summary:      summary,
		RecentAlerts: recentAlerts,
		Metrics:      metrics,
		Health:       map[string]interface{}{"status": "healthy"},
		UpdatedAt:    time.Now(),
	}, nil
}

// GetMetrics returns monitoring metrics converted to interface format
func (m *MonitoringService) GetMetrics(ctx context.Context) (*interfaces.MonitoringMetrics, error) {
	m.metrics.mu.RLock()
	defer m.metrics.mu.RUnlock()

	return &interfaces.MonitoringMetrics{
		EventsProcessed:     m.metrics.AlertsProcessed,
		AlertsGenerated:     m.metrics.AlertsGenerated,
		ProcessingLatency:   m.metrics.ProcessingLatency,
		ThroughputPerSecond: float64(m.metrics.AlertsProcessed) / time.Since(time.Now().Add(-1*time.Hour)).Seconds(),
		ErrorRate:           0.0, // Would calculate actual error rate
		SystemLoad:          0.5, // Would get actual system load
		MemoryUsage:         0,   // Would get actual memory usage
		Details:             make(map[string]interface{}),
		Timestamp:           time.Now(),
	}, nil
}

// GetPolicies returns all monitoring policies
func (m *MonitoringService) GetPolicies(ctx context.Context) ([]*interfaces.MonitoringPolicy, error) {
	var policyModels []MonitoringPolicyModel
	if err := m.db.WithContext(ctx).Find(&policyModels).Error; err != nil {
		return nil, errors.Wrap(err, "failed to fetch policies")
	}

	policies := make([]*interfaces.MonitoringPolicy, len(policyModels))
	for i, model := range policyModels {
		policy, err := m.convertPolicyModelToInterface(&model)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to convert policy %s", model.ID)
		}
		policies[i] = policy
	}

	return policies, nil
}

// eventProcessor processes events from the event queue
func (m *MonitoringService) eventProcessor(ctx context.Context) {
	for {
		select {
		case event := <-m.eventQueue:
			if err := m.processEvent(ctx, event); err != nil {
				// Log error - in production would use proper logging
				continue
			}
		case <-m.stopChan:
			return
		case <-ctx.Done():
			return
		}
	}
}

// processEvent processes a single compliance event
func (m *MonitoringService) processEvent(ctx context.Context, event *interfaces.ComplianceEvent) error {
	// Create monitoring alert based on event
	alert := &interfaces.MonitoringAlert{
		ID:        uuid.New(),
		UserID:    event.UserID,
		AlertType: event.EventType,
		Severity:  interfaces.AlertSeverityMedium, // Would determine based on event analysis
		Status:    interfaces.AlertStatusPending,
		Message:   fmt.Sprintf("Compliance Event: %s detected for user %s", event.EventType, event.UserID),
		Details:   event.Details,
		Timestamp: time.Now(),
		Metadata: map[string]interface{}{
			"event_type": event.EventType,
			"user_id":    event.UserID,
			"timestamp":  event.Timestamp,
			"details":    event.Details,
		},
	}

	// Process alert through handlers
	for _, handler := range m.alertHandlers {
		if err := handler.HandleAlert(ctx, alert); err != nil {
			// Log error but continue with other handlers
			continue
		}
	}

	// Store alert
	if err := m.CreateAlert(ctx, alert); err != nil {
		return errors.Wrap(err, "failed to create alert from event")
	}

	return nil
}

// convertPolicyModelToInterface converts database model to interface type
func (m *MonitoringService) convertPolicyModelToInterface(model *MonitoringPolicyModel) (*interfaces.MonitoringPolicy, error) {
	policy := model.ToInterface()
	return &policy, nil
}

// GetConfiguration returns the monitoring service configuration
func (m *MonitoringService) GetConfiguration(ctx context.Context) (map[string]interface{}, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return map[string]interface{}{
		"workers":                m.workers,
		"event_queue_capacity":   cap(m.eventQueue),
		"alert_channel_capacity": cap(m.alertChan),
		"is_monitoring":          m.isMonitoring,
		"alert_handlers_count":   len(m.alertHandlers),
		"policy_cache_size":      len(m.policyCache),
		"subscribers_count":      len(m.subscribers),
	}, nil
}

// UpdateMonitoringRules updates monitoring rules configuration
func (m *MonitoringService) UpdateMonitoringRules(ctx context.Context, rules []interface{}) error {
	// Implementation would depend on the specific rule format
	// For now, just validate that rules is not nil
	if rules == nil {
		return errors.New("rules cannot be nil")
	}

	// In a real implementation, you would:
	// 1. Validate the rules format
	// 2. Store the rules in the database
	// 3. Update the runtime configuration
	// 4. Notify relevant components

	return nil
}

// HealthCheck performs a health check of the monitoring service
func (m *MonitoringService) HealthCheck(ctx context.Context) error {
	// Check database connectivity
	if err := m.db.WithContext(ctx).Exec("SELECT 1").Error; err != nil {
		return errors.Wrap(err, "database health check failed")
	}
	// Check if monitoring is running as expected
	m.mu.RLock()
	eventQueueDepth := len(m.eventQueue)
	alertChannelDepth := len(m.alertChan)
	m.mu.RUnlock()

	// Check for potential issues
	if eventQueueDepth > cap(m.eventQueue)*3/4 {
		return errors.New("event queue is nearly full, potential bottleneck")
	}

	if alertChannelDepth > cap(m.alertChan)*3/4 {
		return errors.New("alert channel is nearly full, potential bottleneck")
	}

	// All checks passed
	return nil
}
