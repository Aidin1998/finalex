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

	"github.com/finalex/internal/compliance/interfaces"
)

// MonitoringService implements real-time compliance monitoring
type MonitoringService struct {
	db          *gorm.DB
	auditSvc    interfaces.AuditService
	subscribers map[string][]interfaces.AlertSubscriber
	alertChan   chan interfaces.MonitoringAlert
	policyCache map[string]*interfaces.MonitoringPolicy
	metrics     *MonitoringMetrics
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

// NewService creates a new monitoring service
func NewService(logger *zap.Logger) *Service {
	return &Service{
		logger:  logger,
		metrics: make(map[string][]*Metric),
		alerts:  make([]*Alert, 0),
	}
}

// RegisterMetric registers a metric
func (s *Service) RegisterMetric(name string, metricType MetricType, value float64, tags map[string]string) {
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
		stopChan:    make(chan struct{}),
		workers:     workers,
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
	alert.ID = uuid.New().String()
	alert.Timestamp = time.Now()
	alert.Status = "pending"

	// Check if alert should be generated based on policies
	if !m.shouldGenerateAlert(alert) {
		return nil
	}

	// Queue alert for processing
	select {
	case m.alertChan <- alert:
		m.metrics.mu.Lock()
		m.metrics.AlertsGenerated++
		m.metrics.mu.Unlock()
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
			EventType:   "alert_status_updated",
			UserID:      alert.UserID,
			Action:      "update_alert_status",
			EntityType:  "monitoring_alert",
			EntityID:    alertID,
			OldValue:    oldStatus,
			NewValue:    status,
			Metadata:    map[string]interface{}{"alert_type": alert.AlertType},
			Timestamp:   time.Now(),
			Severity:    "info",
		}
		m.auditSvc.LogEvent(ctx, auditEvent)
	}

	return nil
}

// UpdatePolicy updates or creates a monitoring policy
func (m *MonitoringService) UpdatePolicy(ctx context.Context, policy interfaces.MonitoringPolicy) error {
	policyModel := &MonitoringPolicyModel{
		ID:          policy.ID,
		Name:        policy.Name,
		AlertType:   policy.AlertType,
		Enabled:     policy.Enabled,
		Threshold:   policy.Threshold,
		TimeWindow:  policy.TimeWindow,
		Action:      policy.Action,
		Conditions:  policy.Conditions,
		Recipients:  policy.Recipients,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
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
			EventType:   "monitoring_policy_updated",
			Action:      "update_policy",
			EntityType:  "monitoring_policy",
			EntityID:    policyModel.ID,
			NewValue:    fmt.Sprintf("%+v", policy),
			Metadata:    map[string]interface{}{"policy_name": policy.Name},
			Timestamp:   time.Now(),
			Severity:    "info",
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

// GetMetrics returns current monitoring metrics
func (m *MonitoringService) GetMetrics() interfaces.MonitoringMetrics {
	m.metrics.mu.RLock()
	defer m.metrics.mu.RUnlock()

	return interfaces.MonitoringMetrics{
		AlertsGenerated:    m.metrics.AlertsGenerated,
		AlertsProcessed:    m.metrics.AlertsProcessed,
		PolicyUpdates:      m.metrics.PolicyUpdates,
		SubscriberNotified: m.metrics.SubscriberNotified,
		ProcessingLatency:  m.metrics.ProcessingLatency,
	}
}

// alertProcessor processes alerts from the queue
func (m *MonitoringService) alertProcessor(ctx context.Context) {
	for {
		select {
		case alert := <-m.alertChan:
			start := time.Now()
			if err := m.processAlert(ctx, alert); err != nil {
				// Log error but continue processing
				continue
			}
			
			m.metrics.mu.Lock()
			m.metrics.AlertsProcessed++
			m.metrics.ProcessingLatency = time.Since(start)
			m.metrics.mu.Unlock()

		case <-ctx.Done():
			return
		case <-m.stopChan:
			return
		}
	}
}

// processAlert handles individual alert processing
func (m *MonitoringService) processAlert(ctx context.Context, alert interfaces.MonitoringAlert) error {
	// Save alert to database
	alertModel := &MonitoringAlertModel{
		ID:          alert.ID,
		UserID:      alert.UserID,
		AlertType:   alert.AlertType,
		Severity:    alert.Severity,
		Message:     alert.Message,
		Data:        alert.Data,
		Status:      alert.Status,
		Timestamp:   alert.Timestamp,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := m.db.WithContext(ctx).Create(alertModel).Error; err != nil {
		return errors.Wrap(err, "failed to save alert")
	}

	// Notify subscribers
	m.mu.RLock()
	subscribers := m.subscribers[alert.AlertType]
	m.mu.RUnlock()

	for _, subscriber := range subscribers {
		go func(sub interfaces.AlertSubscriber) {
			if err := sub.OnAlert(ctx, alert); err == nil {
				m.metrics.mu.Lock()
				m.metrics.SubscriberNotified++
				m.metrics.mu.Unlock()
			}
		}(subscriber)
	}

	// Audit the alert
	if m.auditSvc != nil {
		auditEvent := interfaces.AuditEvent{
			EventType:   "monitoring_alert_generated",
			UserID:      alert.UserID,
			Action:      "generate_alert",
			EntityType:  "monitoring_alert",
			EntityID:    alert.ID,
			NewValue:    alert.Message,
			Metadata:    map[string]interface{}{"alert_type": alert.AlertType, "severity": alert.Severity},
			Timestamp:   time.Now(),
			Severity:    alert.Severity,
		}
		m.auditSvc.LogEvent(ctx, auditEvent)
	}

	return nil
}

// shouldGenerateAlert checks if alert should be generated based on policies
func (m *MonitoringService) shouldGenerateAlert(alert interfaces.MonitoringAlert) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, policy := range m.policyCache {
		if policy.AlertType == alert.AlertType && policy.Enabled {
			// Check conditions and thresholds
			if m.evaluateConditions(alert, policy) {
				return true
			}
		}
	}

	return true // Default to generating alert if no specific policy
}

// evaluateConditions checks if alert meets policy conditions
func (m *MonitoringService) evaluateConditions(alert interfaces.MonitoringAlert, policy *interfaces.MonitoringPolicy) bool {
	// Implement policy condition evaluation logic
	// This would check thresholds, time windows, etc.
	return true
}

// loadPolicies loads monitoring policies from database into cache
func (m *MonitoringService) loadPolicies(ctx context.Context) error {
	var policies []MonitoringPolicyModel
	if err := m.db.WithContext(ctx).Find(&policies).Error; err != nil {
		return errors.Wrap(err, "failed to load policies")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.policyCache = make(map[string]*interfaces.MonitoringPolicy)
	for _, policy := range policies {
		interfacePolicy := policy.ToInterface()
		m.policyCache[policy.ID] = &interfacePolicy
	}

	return nil
}

// policyUpdateChecker periodically checks for policy updates
func (m *MonitoringService) policyUpdateChecker(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := m.loadPolicies(ctx); err != nil {
				// Log error but continue
				continue
			}
		case <-ctx.Done():
			return
		case <-m.stopChan:
			return
		}
	}
}

// metricsCollector periodically updates metrics
func (m *MonitoringService) metricsCollector(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Collect additional metrics if needed
		case <-ctx.Done():
			return
		case <-m.stopChan:
			return
		}
	}
}
