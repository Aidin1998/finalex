package monitoring

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/compliance/aml"
)

// AlertLevel represents the severity of an alert
type AlertLevel int

const (
	AlertLevelInfo AlertLevel = iota
	AlertLevelWarning
	AlertLevelCritical
	AlertLevelEmergency
)

// MonitoringMetrics holds real-time monitoring statistics
type MonitoringMetrics struct {
	TransactionsPerSecond   float64                    `json:"transactions_per_second"`
	SuspiciousActivityCount int64                      `json:"suspicious_activity_count"`
	ActiveInvestigations    int64                      `json:"active_investigations"`
	RiskScoreDistribution   map[string]int64           `json:"risk_score_distribution"`
	AlertsByLevel           map[AlertLevel]int64       `json:"alerts_by_level"`
	ComplianceActionStats   map[string]int64           `json:"compliance_action_stats"`
	SystemHealth            SystemHealthMetrics        `json:"system_health"`
	GeographicDistribution  map[string]TransactionData `json:"geographic_distribution"`
	CurrencyDistribution    map[string]TransactionData `json:"currency_distribution"`
	LastUpdated             time.Time                  `json:"last_updated"`
}

// SystemHealthMetrics tracks system performance
type SystemHealthMetrics struct {
	CPUUsage           float64   `json:"cpu_usage"`
	MemoryUsage        float64   `json:"memory_usage"`
	DetectionLatency   float64   `json:"detection_latency_ms"`
	ProcessingBacklog  int64     `json:"processing_backlog"`
	ErrorRate          float64   `json:"error_rate"`
	ThroughputCapacity float64   `json:"throughput_capacity"`
	LastHealthCheck    time.Time `json:"last_health_check"`
}

// TransactionData holds transaction statistics
type TransactionData struct {
	Count       int64   `json:"count"`
	Volume      float64 `json:"volume"`
	AverageSize float64 `json:"average_size"`
	RiskScore   float64 `json:"risk_score"`
}

// RealtimeAlert represents a real-time monitoring alert
type RealtimeAlert struct {
	ID         string                 `json:"id"`
	Level      AlertLevel             `json:"level"`
	Type       string                 `json:"type"`
	Message    string                 `json:"message"`
	UserID     string                 `json:"user_id,omitempty"`
	Metadata   map[string]interface{} `json:"metadata"`
	Timestamp  time.Time              `json:"timestamp"`
	Resolved   bool                   `json:"resolved"`
	ResolvedAt *time.Time             `json:"resolved_at,omitempty"`
	ResolvedBy string                 `json:"resolved_by,omitempty"`
}

// AlertSubscription represents a subscription to alerts
type AlertSubscription struct {
	ID      string      `json:"id"`
	UserID  string      `json:"user_id"`
	Filters AlertFilter `json:"filters"`
	Channel chan RealtimeAlert
	Active  bool      `json:"active"`
	Created time.Time `json:"created"`
}

// AlertFilter defines criteria for alert filtering
type AlertFilter struct {
	MinLevel AlertLevel `json:"min_level"`
	Types    []string   `json:"types"`
	UserIDs  []string   `json:"user_ids"`
	Keywords []string   `json:"keywords"`
}

// RealtimeMonitor provides real-time monitoring capabilities
type RealtimeMonitor struct {
	metrics       *MonitoringMetrics
	alerts        []RealtimeAlert
	subscriptions map[string]*AlertSubscription
	mu            sync.RWMutex
	alertMu       sync.RWMutex
	subMu         sync.RWMutex

	// Configuration
	metricsRetention time.Duration
	alertRetention   time.Duration
	maxAlerts        int

	// Channels
	alertChan   chan RealtimeAlert
	metricsChan chan func(*MonitoringMetrics)
	stopChan    chan struct{}

	// Dependencies
	storage MetricsStorage
}

// MetricsStorage interface for persisting monitoring data
type MetricsStorage interface {
	StoreMetrics(ctx context.Context, metrics *MonitoringMetrics) error
	StoreAlert(ctx context.Context, alert *RealtimeAlert) error
	GetHistoricalMetrics(ctx context.Context, from, to time.Time) ([]*MonitoringMetrics, error)
	GetAlertHistory(ctx context.Context, filter AlertFilter, limit int) ([]*RealtimeAlert, error)
}

// NewRealtimeMonitor creates a new real-time monitor
func NewRealtimeMonitor(storage MetricsStorage) *RealtimeMonitor {
	return &RealtimeMonitor{
		metrics: &MonitoringMetrics{
			RiskScoreDistribution:  make(map[string]int64),
			AlertsByLevel:          make(map[AlertLevel]int64),
			ComplianceActionStats:  make(map[string]int64),
			GeographicDistribution: make(map[string]TransactionData),
			CurrencyDistribution:   make(map[string]TransactionData),
			LastUpdated:            time.Now(),
		},
		alerts:        make([]RealtimeAlert, 0),
		subscriptions: make(map[string]*AlertSubscription),

		metricsRetention: 24 * time.Hour,
		alertRetention:   7 * 24 * time.Hour,
		maxAlerts:        10000,

		alertChan:   make(chan RealtimeAlert, 1000),
		metricsChan: make(chan func(*MonitoringMetrics), 100),
		stopChan:    make(chan struct{}),

		storage: storage,
	}
}

// Start begins the real-time monitoring process
func (rm *RealtimeMonitor) Start(ctx context.Context) error {
	log.Println("Starting real-time AML monitor")

	// Start metrics collection goroutine
	go rm.metricsProcessor(ctx)

	// Start alert processing goroutine
	go rm.alertProcessor(ctx)

	// Start cleanup goroutine
	go rm.cleanup(ctx)

	// Start health check goroutine
	go rm.healthCheck(ctx)

	return nil
}

// Stop stops the real-time monitoring
func (rm *RealtimeMonitor) Stop() {
	log.Println("Stopping real-time AML monitor")
	close(rm.stopChan)
}

// RecordTransaction records a transaction for monitoring
func (rm *RealtimeMonitor) RecordTransaction(tx *aml.Transaction) {
	rm.metricsChan <- func(m *MonitoringMetrics) {
		m.TransactionsPerSecond++

		// Update geographic distribution
		if tx.FromAddress != "" {
			country := rm.getCountryFromAddress(tx.FromAddress)
			data := m.GeographicDistribution[country]
			data.Count++
			data.Volume += tx.Amount
			data.AverageSize = data.Volume / float64(data.Count)
			m.GeographicDistribution[country] = data
		}

		// Update currency distribution
		data := m.CurrencyDistribution[tx.Asset]
		data.Count++
		data.Volume += tx.Amount
		data.AverageSize = data.Volume / float64(data.Count)
		m.CurrencyDistribution[tx.Asset] = data

		m.LastUpdated = time.Now()
	}
}

// RecordSuspiciousActivity records suspicious activity
func (rm *RealtimeMonitor) RecordSuspiciousActivity(activity *aml.SuspiciousActivity) {
	rm.metricsChan <- func(m *MonitoringMetrics) {
		m.SuspiciousActivityCount++

		// Update risk score distribution
		scoreRange := rm.getRiskScoreRange(activity.RiskScore)
		m.RiskScoreDistribution[scoreRange]++

		m.LastUpdated = time.Now()
	}

	// Generate alert for high-risk activities
	if activity.RiskScore >= 80 {
		alert := RealtimeAlert{
			ID:      fmt.Sprintf("sa_%d_%d", activity.ID, time.Now().Unix()),
			Level:   AlertLevelCritical,
			Type:    "suspicious_activity",
			Message: fmt.Sprintf("High-risk suspicious activity detected: %s", activity.Description),
			UserID:  activity.UserID,
			Metadata: map[string]interface{}{
				"activity_id": activity.ID,
				"risk_score":  activity.RiskScore,
				"type":        activity.Type,
			},
			Timestamp: time.Now(),
		}
		rm.alertChan <- alert
	}
}

// RecordComplianceAction records a compliance action
func (rm *RealtimeMonitor) RecordComplianceAction(action *aml.ComplianceAction) {
	rm.metricsChan <- func(m *MonitoringMetrics) {
		m.ComplianceActionStats[action.ActionType]++
		m.LastUpdated = time.Now()
	}

	// Generate alert for critical actions
	if action.ActionType == "FREEZE_ACCOUNT" || action.ActionType == "BLOCK_TRANSACTION" {
		alert := RealtimeAlert{
			ID:      fmt.Sprintf("ca_%d_%d", action.ID, time.Now().Unix()),
			Level:   AlertLevelWarning,
			Type:    "compliance_action",
			Message: fmt.Sprintf("Compliance action taken: %s", action.ActionType),
			UserID:  action.UserID,
			Metadata: map[string]interface{}{
				"action_id":   action.ID,
				"action_type": action.ActionType,
				"reason":      action.Reason,
			},
			Timestamp: time.Now(),
		}
		rm.alertChan <- alert
	}
}

// RecordInvestigation records investigation metrics
func (rm *RealtimeMonitor) RecordInvestigation(investigation *aml.InvestigationCase) {
	rm.metricsChan <- func(m *MonitoringMetrics) {
		if investigation.Status == "OPEN" || investigation.Status == "IN_PROGRESS" {
			m.ActiveInvestigations++
		}
		m.LastUpdated = time.Now()
	}
}

// GetMetrics returns current monitoring metrics
func (rm *RealtimeMonitor) GetMetrics() *MonitoringMetrics {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	// Create a copy to avoid race conditions
	metricsCopy := *rm.metrics
	metricsCopy.RiskScoreDistribution = make(map[string]int64)
	metricsCopy.AlertsByLevel = make(map[AlertLevel]int64)
	metricsCopy.ComplianceActionStats = make(map[string]int64)
	metricsCopy.GeographicDistribution = make(map[string]TransactionData)
	metricsCopy.CurrencyDistribution = make(map[string]TransactionData)

	for k, v := range rm.metrics.RiskScoreDistribution {
		metricsCopy.RiskScoreDistribution[k] = v
	}
	for k, v := range rm.metrics.AlertsByLevel {
		metricsCopy.AlertsByLevel[k] = v
	}
	for k, v := range rm.metrics.ComplianceActionStats {
		metricsCopy.ComplianceActionStats[k] = v
	}
	for k, v := range rm.metrics.GeographicDistribution {
		metricsCopy.GeographicDistribution[k] = v
	}
	for k, v := range rm.metrics.CurrencyDistribution {
		metricsCopy.CurrencyDistribution[k] = v
	}

	return &metricsCopy
}

// GetRecentAlerts returns recent alerts
func (rm *RealtimeMonitor) GetRecentAlerts(limit int) []RealtimeAlert {
	rm.alertMu.RLock()
	defer rm.alertMu.RUnlock()

	if limit > len(rm.alerts) {
		limit = len(rm.alerts)
	}

	result := make([]RealtimeAlert, limit)
	copy(result, rm.alerts[len(rm.alerts)-limit:])
	return result
}

// SubscribeToAlerts creates a subscription for real-time alerts
func (rm *RealtimeMonitor) SubscribeToAlerts(userID string, filter AlertFilter) (*AlertSubscription, error) {
	rm.subMu.Lock()
	defer rm.subMu.Unlock()

	subscription := &AlertSubscription{
		ID:      fmt.Sprintf("sub_%s_%d", userID, time.Now().Unix()),
		UserID:  userID,
		Filters: filter,
		Channel: make(chan RealtimeAlert, 100),
		Active:  true,
		Created: time.Now(),
	}

	rm.subscriptions[subscription.ID] = subscription

	log.Printf("Created alert subscription %s for user %s", subscription.ID, userID)
	return subscription, nil
}

// UnsubscribeFromAlerts removes an alert subscription
func (rm *RealtimeMonitor) UnsubscribeFromAlerts(subscriptionID string) error {
	rm.subMu.Lock()
	defer rm.subMu.Unlock()

	if sub, exists := rm.subscriptions[subscriptionID]; exists {
		sub.Active = false
		close(sub.Channel)
		delete(rm.subscriptions, subscriptionID)
		log.Printf("Removed alert subscription %s", subscriptionID)
		return nil
	}

	return fmt.Errorf("subscription not found: %s", subscriptionID)
}

// GenerateAlert creates a custom alert
func (rm *RealtimeMonitor) GenerateAlert(level AlertLevel, alertType, message, userID string, metadata map[string]interface{}) {
	alert := RealtimeAlert{
		ID:        fmt.Sprintf("custom_%d", time.Now().UnixNano()),
		Level:     level,
		Type:      alertType,
		Message:   message,
		UserID:    userID,
		Metadata:  metadata,
		Timestamp: time.Now(),
	}

	rm.alertChan <- alert
}

// metricsProcessor processes metrics updates
func (rm *RealtimeMonitor) metricsProcessor(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	tpsCounter := 0.0
	lastTpsReset := time.Now()

	for {
		select {
		case <-ctx.Done():
			return
		case <-rm.stopChan:
			return
		case updateFn := <-rm.metricsChan:
			rm.mu.Lock()
			updateFn(rm.metrics)
			tpsCounter++
			rm.mu.Unlock()
		case <-ticker.C:
			rm.mu.Lock()
			// Calculate TPS
			elapsed := time.Since(lastTpsReset).Seconds()
			if elapsed >= 1.0 {
				rm.metrics.TransactionsPerSecond = tpsCounter / elapsed
				tpsCounter = 0
				lastTpsReset = time.Now()
			}

			// Store metrics periodically
			if rm.storage != nil {
				go func(m *MonitoringMetrics) {
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					if err := rm.storage.StoreMetrics(ctx, m); err != nil {
						log.Printf("Failed to store metrics: %v", err)
					}
				}(rm.metrics)
			}
			rm.mu.Unlock()
		}
	}
}

// alertProcessor processes and distributes alerts
func (rm *RealtimeMonitor) alertProcessor(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-rm.stopChan:
			return
		case alert := <-rm.alertChan:
			rm.processAlert(alert)
		}
	}
}

// processAlert processes a single alert
func (rm *RealtimeMonitor) processAlert(alert RealtimeAlert) {
	// Store alert
	rm.alertMu.Lock()
	rm.alerts = append(rm.alerts, alert)

	// Maintain alert limit
	if len(rm.alerts) > rm.maxAlerts {
		rm.alerts = rm.alerts[len(rm.alerts)-rm.maxAlerts:]
	}
	rm.alertMu.Unlock()

	// Update metrics
	rm.mu.Lock()
	rm.metrics.AlertsByLevel[alert.Level]++
	rm.mu.Unlock()

	// Distribute to subscribers
	rm.subMu.RLock()
	for _, sub := range rm.subscriptions {
		if sub.Active && rm.matchesFilter(alert, sub.Filters) {
			select {
			case sub.Channel <- alert:
				// Alert sent successfully
			default:
				// Channel full, skip
				log.Printf("Alert channel full for subscription %s", sub.ID)
			}
		}
	}
	rm.subMu.RUnlock()

	// Store alert persistently
	if rm.storage != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := rm.storage.StoreAlert(ctx, &alert); err != nil {
				log.Printf("Failed to store alert: %v", err)
			}
		}()
	}
}

// cleanup performs periodic cleanup of old data
func (rm *RealtimeMonitor) cleanup(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-rm.stopChan:
			return
		case <-ticker.C:
			rm.performCleanup()
		}
	}
}

// performCleanup removes old alerts and data
func (rm *RealtimeMonitor) performCleanup() {
	cutoff := time.Now().Add(-rm.alertRetention)

	rm.alertMu.Lock()
	var filteredAlerts []RealtimeAlert
	for _, alert := range rm.alerts {
		if alert.Timestamp.After(cutoff) {
			filteredAlerts = append(filteredAlerts, alert)
		}
	}
	rm.alerts = filteredAlerts
	rm.alertMu.Unlock()

	log.Printf("Cleaned up old alerts, %d alerts remaining", len(filteredAlerts))
}

// healthCheck monitors system health
func (rm *RealtimeMonitor) healthCheck(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-rm.stopChan:
			return
		case <-ticker.C:
			rm.updateSystemHealth()
		}
	}
}

// updateSystemHealth updates system health metrics
func (rm *RealtimeMonitor) updateSystemHealth() {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	// Simulate health metrics (in real implementation, these would come from actual system monitoring)
	rm.metrics.SystemHealth = SystemHealthMetrics{
		CPUUsage:           rm.getCPUUsage(),
		MemoryUsage:        rm.getMemoryUsage(),
		DetectionLatency:   rm.getDetectionLatency(),
		ProcessingBacklog:  int64(len(rm.alertChan)),
		ErrorRate:          0.01,  // 1% error rate
		ThroughputCapacity: 10000, // transactions per second capacity
		LastHealthCheck:    time.Now(),
	}
}

// Helper methods

func (rm *RealtimeMonitor) getCountryFromAddress(address string) string {
	// Simplified country detection (in reality, this would use GeoIP or similar)
	if len(address) > 10 {
		switch address[0:2] {
		case "0x":
			return "Unknown"
		case "bc", "tb":
			return "Bitcoin Network"
		default:
			return "Unknown"
		}
	}
	return "Unknown"
}

func (rm *RealtimeMonitor) getRiskScoreRange(score float64) string {
	switch {
	case score < 20:
		return "Low (0-20)"
	case score < 40:
		return "Medium-Low (20-40)"
	case score < 60:
		return "Medium (40-60)"
	case score < 80:
		return "Medium-High (60-80)"
	default:
		return "High (80-100)"
	}
}

func (rm *RealtimeMonitor) matchesFilter(alert RealtimeAlert, filter AlertFilter) bool {
	// Check alert level
	if alert.Level < filter.MinLevel {
		return false
	}

	// Check alert types
	if len(filter.Types) > 0 {
		found := false
		for _, t := range filter.Types {
			if t == alert.Type {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check user IDs
	if len(filter.UserIDs) > 0 {
		found := false
		for _, uid := range filter.UserIDs {
			if uid == alert.UserID {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check keywords in message
	if len(filter.Keywords) > 0 {
		found := false
		for _, keyword := range filter.Keywords {
			if contains(alert.Message, keyword) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

func (rm *RealtimeMonitor) getCPUUsage() float64 {
	// Placeholder - would integrate with actual system monitoring
	return 45.5
}

func (rm *RealtimeMonitor) getMemoryUsage() float64 {
	// Placeholder - would integrate with actual system monitoring
	return 62.3
}

func (rm *RealtimeMonitor) getDetectionLatency() float64 {
	// Placeholder - would measure actual detection latency
	return 12.5
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[0:len(substr)] == substr
}

// ResolveAlert marks an alert as resolved
func (rm *RealtimeMonitor) ResolveAlert(alertID, resolvedBy string) error {
	rm.alertMu.Lock()
	defer rm.alertMu.Unlock()

	for i := range rm.alerts {
		if rm.alerts[i].ID == alertID {
			now := time.Now()
			rm.alerts[i].Resolved = true
			rm.alerts[i].ResolvedAt = &now
			rm.alerts[i].ResolvedBy = resolvedBy

			log.Printf("Alert %s resolved by %s", alertID, resolvedBy)
			return nil
		}
	}

	return fmt.Errorf("alert not found: %s", alertID)
}

// GetMetricsAsJSON returns metrics as JSON
func (rm *RealtimeMonitor) GetMetricsAsJSON() ([]byte, error) {
	metrics := rm.GetMetrics()
	return json.Marshal(metrics)
}
