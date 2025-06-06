package monitoring

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/Aidin1998/finalex/internal/compliance/interfaces"
)

// MonitoringService implements real-time compliance monitoring
type MonitoringService struct {
	db                *gorm.DB
	auditSvc          interfaces.AuditService
	subscribers       map[string][]interfaces.AlertSubscriber
	alertChan         chan interfaces.MonitoringAlert
	policyCache       map[string]*interfaces.MonitoringPolicy
	metrics           *MonitoringMetrics
	prometheusMetrics *PrometheusMetrics
	alertingManager   *AlertingManager
	logger            *zap.Logger
	mu                sync.RWMutex
	stopChan          chan struct{}
	workers           int
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
func NewMonitoringService(db *gorm.DB, auditSvc interfaces.AuditService, logger *zap.Logger, workers int) *MonitoringService {
	if workers <= 0 {
		workers = 4
	}

	return &MonitoringService{
		db:                db,
		auditSvc:          auditSvc,
		subscribers:       make(map[string][]interfaces.AlertSubscriber),
		alertChan:         make(chan interfaces.MonitoringAlert, 1000),
		policyCache:       make(map[string]*interfaces.MonitoringPolicy),
		metrics:           &MonitoringMetrics{},
		prometheusMetrics: NewPrometheusMetrics(),
		alertingManager:   NewAlertingManager(logger),
		logger:            logger,
		stopChan:          make(chan struct{}),
		workers:           workers,
	}
}

// Start initializes the monitoring service
func (m *MonitoringService) Start(ctx context.Context) error {
	// Load monitoring policies from database
	if err := m.loadPolicies(ctx); err != nil {
		return errors.Wrap(err, "failed to load monitoring policies")
	}

	// Start alert processing workers
	for i := 0; i < m.workers; i++ {
		go m.alertProcessor(ctx)
	}

	// Start policy update checker
	go m.policyUpdateChecker(ctx)

	// Start metrics collector
	go m.metricsCollector(ctx)

	return nil
}

// Stop gracefully shuts down the monitoring service
func (m *MonitoringService) Stop() error {
	close(m.stopChan)
	return nil
}

// Subscribe registers a subscriber for specific alert types
func (m *MonitoringService) Subscribe(alertType string, subscriber interfaces.AlertSubscriber) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.subscribers[alertType] == nil {
		m.subscribers[alertType] = make([]interfaces.AlertSubscriber, 0)
	}
	m.subscribers[alertType] = append(m.subscribers[alertType], subscriber)

	return nil
}

// Unsubscribe removes a subscriber
func (m *MonitoringService) Unsubscribe(alertType string, subscriber interfaces.AlertSubscriber) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	subscribers := m.subscribers[alertType]
	for i, sub := range subscribers {
		if sub == subscriber {
			m.subscribers[alertType] = append(subscribers[:i], subscribers[i+1:]...)
			break
		}
	}

	return nil
}

// GenerateAlert creates and queues a monitoring alert
func (m *MonitoringService) GenerateAlert(ctx context.Context, alert interfaces.MonitoringAlert) error {
	alert.ID = uuid.New()
	alert.Timestamp = time.Now()
	alert.Status = interfaces.AlertStatusPending

	// Check if alert should be generated based on policies
	if !m.shouldGenerateAlert(alert) {
		return nil
	}

	// Record Prometheus metrics
	m.prometheusMetrics.RecordAlertGenerated(alert.AlertType, alert.Severity.String(), extractMarket(alert))

	// Queue alert for processing
	select {
	case m.alertChan <- alert:
		m.metrics.mu.Lock()
		m.metrics.AlertsGenerated++
		m.metrics.mu.Unlock()

		// Update queue size metric
		m.prometheusMetrics.SetQueueSize("alerts", float64(len(m.alertChan)))

		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		return errors.New("alert queue full")
	}
}

// GetAlerts retrieves alerts based on filters
func (m *MonitoringService) GetAlerts(ctx context.Context, filter interfaces.AlertFilter) ([]interfaces.MonitoringAlert, error) {
	var alerts []MonitoringAlertModel
	query := m.db.WithContext(ctx).Model(&MonitoringAlertModel{})

	if filter.UserID != "" {
		query = query.Where("user_id = ?", filter.UserID)
	}
	if filter.AlertType != "" {
		query = query.Where("alert_type = ?", filter.AlertType)
	}
	if filter.Severity != "" {
		query = query.Where("severity = ?", filter.Severity)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if !filter.StartTime.IsZero() {
		query = query.Where("timestamp >= ?", filter.StartTime)
	}
	if !filter.EndTime.IsZero() {
		query = query.Where("timestamp <= ?", filter.EndTime)
	}

	if err := query.Order("timestamp DESC").Limit(filter.Limit).Find(&alerts).Error; err != nil {
		return nil, errors.Wrap(err, "failed to retrieve alerts")
	}

	result := make([]interfaces.MonitoringAlert, len(alerts))
	for i, alert := range alerts {
		result[i] = alert.ToInterface()
	}

	return result, nil
}

// UpdateAlertStatus updates the status of an alert
func (m *MonitoringService) UpdateAlertStatus(ctx context.Context, alertID, status string) error {
	alert := &MonitoringAlertModel{}
	if err := m.db.WithContext(ctx).Where("id = ?", alertID).First(alert).Error; err != nil {
		return errors.Wrap(err, "alert not found")
	}

	oldStatus := alert.Status
	alert.Status = status
	alert.UpdatedAt = time.Now()

	if err := m.db.WithContext(ctx).Save(alert).Error; err != nil {
		return errors.Wrap(err, "failed to update alert status")
	}

	// Audit the status change
	if m.auditSvc != nil {
		auditEvent := interfaces.AuditEvent{
			EventType:  "alert_status_updated",
			UserID:     alert.UserID,
			Action:     "update_alert_status",
			EntityType: "monitoring_alert",
			EntityID:   alertID,
			OldValue:   oldStatus,
			NewValue:   status,
			Metadata:   map[string]interface{}{"alert_type": alert.AlertType},
			Timestamp:  time.Now(),
			Severity:   "info",
		}
		m.auditSvc.LogEvent(ctx, auditEvent)
	}

	return nil
}

// UpdatePolicy updates or creates a monitoring policy
func (m *MonitoringService) UpdatePolicy(ctx context.Context, policy interfaces.MonitoringPolicy) error {
	policyModel := &MonitoringPolicyModel{
		ID:         policy.ID,
		Name:       policy.Name,
		AlertType:  policy.AlertType,
		Enabled:    policy.Enabled,
		Threshold:  policy.Threshold,
		TimeWindow: policy.TimeWindow,
		Action:     policy.Action,
		Conditions: policy.Conditions,
		Recipients: policy.Recipients,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	if policy.ID == "" {
		policyModel.ID = uuid.New().String()
	}

	if err := m.db.WithContext(ctx).Save(policyModel).Error; err != nil {
		return errors.Wrap(err, "failed to save monitoring policy")
	}

	// Update cache
	m.mu.Lock()
	m.policyCache[policyModel.ID] = &policy
	m.mu.Unlock()

	m.metrics.mu.Lock()
	m.metrics.PolicyUpdates++
	m.metrics.mu.Unlock()

	// Audit the policy update
	if m.auditSvc != nil {
		auditEvent := interfaces.AuditEvent{
			EventType:  "monitoring_policy_updated",
			Action:     "update_policy",
			EntityType: "monitoring_policy",
			EntityID:   policyModel.ID,
			NewValue:   fmt.Sprintf("%+v", policy),
			Metadata:   map[string]interface{}{"policy_name": policy.Name},
			Timestamp:  time.Now(),
			Severity:   "info",
		}
		m.auditSvc.LogEvent(ctx, auditEvent)
	}

	return nil
}

// GetPolicy retrieves a monitoring policy by ID
func (m *MonitoringService) GetPolicy(ctx context.Context, policyID string) (*interfaces.MonitoringPolicy, error) {
	m.mu.RLock()
	if policy, exists := m.policyCache[policyID]; exists {
		m.mu.RUnlock()
		return policy, nil
	}
	m.mu.RUnlock()

	var policyModel MonitoringPolicyModel
	if err := m.db.WithContext(ctx).Where("id = ?", policyID).First(&policyModel).Error; err != nil {
		return nil, errors.Wrap(err, "policy not found")
	}

	policy := policyModel.ToInterface()

	// Update cache
	m.mu.Lock()
	m.policyCache[policyID] = &policy
	m.mu.Unlock()

	return &policy, nil
}

// GetMetrics returns current monitoring metrics including Prometheus metrics
func (m *MonitoringService) GetMetrics() interfaces.MonitoringMetrics {
	m.metrics.mu.RLock()
	defer m.metrics.mu.RUnlock()

	return interfaces.MonitoringMetrics{
		AlertsGenerated:     m.metrics.AlertsGenerated,
		AlertsProcessed:     m.metrics.AlertsProcessed,
		PolicyUpdates:       m.metrics.PolicyUpdates,
		SubscriberNotified:  m.metrics.SubscriberNotified,
		ProcessingLatency:   m.metrics.ProcessingLatency,
		AverageResponseTime: m.metrics.ProcessingLatency,
		ErrorRate:           m.calculateErrorRate(),
		LastUpdated:         time.Now(),
	}
}

// GetPrometheusMetrics returns the Prometheus metrics instance
func (m *MonitoringService) GetPrometheusMetrics() *PrometheusMetrics {
	return m.prometheusMetrics
}

// GetHealthStatus returns the health status of the monitoring service
func (m *MonitoringService) GetHealthStatus() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	status := map[string]interface{}{
		"service":          "healthy",
		"database":         "healthy",
		"alert_channels":   m.alertingManager.GetEnabledChannels(),
		"queue_size":       len(m.alertChan),
		"active_policies":  len(m.policyCache),
		"subscribers":      len(m.subscribers),
		"workers":          m.workers,
		"uptime":           time.Since(time.Now()), // This would be tracked from service start
		"last_policy_sync": time.Now(),             // This would be tracked
	}

	// Check database health
	if err := m.db.Exec("SELECT 1").Error; err != nil {
		status["database"] = "unhealthy"
		status["service"] = "degraded"
	}

	// Update Prometheus health metrics
	healthy := status["service"] == "healthy"
	m.prometheusMetrics.SetServiceHealth("monitoring", "core", healthy)
	m.prometheusMetrics.SetServiceHealth("monitoring", "database", status["database"] == "healthy")
	m.prometheusMetrics.SetQueueSize("alerts", float64(len(m.alertChan)))

	return status
}

// calculateErrorRate calculates the current error rate
func (m *MonitoringService) calculateErrorRate() float64 {
	// Implementation would track successful vs failed operations
	// For now, return 0 as placeholder
	return 0.0
}

// extractMarket extracts market information from alert details
func extractMarket(alert interfaces.MonitoringAlert) string {
	if market, ok := alert.Details["market"].(string); ok {
		return market
	}
	return "unknown"
}

// AlertChannelConfig holds configuration for alert channels
type AlertChannelConfig struct {
	Webhook struct {
		Enabled bool              `json:"enabled"`
		URL     string            `json:"url"`
		Method  string            `json:"method"`
		Headers map[string]string `json:"headers"`
		Timeout time.Duration     `json:"timeout"`
	} `json:"webhook"`

	Email struct {
		Enabled   bool     `json:"enabled"`
		SMTPHost  string   `json:"smtp_host"`
		SMTPPort  int      `json:"smtp_port"`
		Username  string   `json:"username"`
		Password  string   `json:"password"`
		FromEmail string   `json:"from_email"`
		ToEmails  []string `json:"to_emails"`
	} `json:"email"`

	Slack struct {
		Enabled    bool   `json:"enabled"`
		WebhookURL string `json:"webhook_url"`
		Channel    string `json:"channel"`
		Username   string `json:"username"`
		IconEmoji  string `json:"icon_emoji"`
	} `json:"slack"`
}
