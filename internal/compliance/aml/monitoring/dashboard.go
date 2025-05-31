package monitoring

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/compliance/aml"
	"github.com/shopspring/decimal"
)

// DashboardMetrics represents real-time dashboard metrics
type DashboardMetrics struct {
	Timestamp          time.Time                  `json:"timestamp"`
	TotalUsers         int                        `json:"total_users"`
	ActivePositions    int                        `json:"active_positions"`
	TotalExposure      decimal.Decimal            `json:"total_exposure"`
	TotalVaR           decimal.Decimal            `json:"total_var"`
	AverageRiskScore   decimal.Decimal            `json:"average_risk_score"`
	ActiveAlerts       int                        `json:"active_alerts"`
	CriticalAlerts     int                        `json:"critical_alerts"`
	LimitBreaches      int                        `json:"limit_breaches"`
	ComplianceEvents   int                        `json:"compliance_events"`
	SystemHealth       string                     `json:"system_health"`
	PerformanceMetrics PerformanceMetrics         `json:"performance_metrics"`
	TopRiskyUsers      []UserRiskSummary          `json:"top_risky_users"`
	AlertsByType       map[string]int             `json:"alerts_by_type"`
	ExposureByMarket   map[string]decimal.Decimal `json:"exposure_by_market"`
}

// UserRiskSummary provides a summary of user's risk profile
type UserRiskSummary struct {
	UserID      string          `json:"user_id"`
	RiskScore   decimal.Decimal `json:"risk_score"`
	Exposure    decimal.Decimal `json:"exposure"`
	VaR         decimal.Decimal `json:"var"`
	AlertCount  int             `json:"alert_count"`
	LastUpdated time.Time       `json:"last_updated"`
}

// PerformanceMetrics tracks system performance
type PerformanceMetrics struct {
	CalculationLatency  time.Duration `json:"calculation_latency"`
	ThroughputPerSecond int           `json:"throughput_per_second"`
	MemoryUsageMB       float64       `json:"memory_usage_mb"`
	CPUUsagePercent     float64       `json:"cpu_usage_percent"`
	ErrorRate           float64       `json:"error_rate"`
	UptimeSeconds       int64         `json:"uptime_seconds"`
}

// AlertNotification represents a real-time alert notification
type AlertNotification struct {
	ID           string                 `json:"id"`
	Type         string                 `json:"type"`     // "risk", "compliance", "limit", "system"
	Priority     string                 `json:"priority"` // "low", "medium", "high", "critical"
	Title        string                 `json:"title"`
	Message      string                 `json:"message"`
	UserID       string                 `json:"user_id,omitempty"`
	Data         map[string]interface{} `json:"data"`
	Timestamp    time.Time              `json:"timestamp"`
	Acknowledged bool                   `json:"acknowledged"`
	AckedBy      string                 `json:"acked_by,omitempty"`
	AckedAt      *time.Time             `json:"acked_at,omitempty"`
}

// DashboardSubscriber represents a client subscribed to dashboard updates
type DashboardSubscriber struct {
	ID       string                 `json:"id"`
	Channel  chan DashboardUpdate   `json:"-"`
	Filters  map[string]interface{} `json:"filters"`
	LastSeen time.Time              `json:"last_seen"`
}

// DashboardUpdate represents a real-time dashboard update
type DashboardUpdate struct {
	Type      string      `json:"type"` // "metrics", "alert", "position", "compliance"
	Data      interface{} `json:"data"`
	Timestamp time.Time   `json:"timestamp"`
}

// MonitoringDashboard provides real-time risk monitoring and alerting
type MonitoringDashboard struct {
	mu               sync.RWMutex
	calculator       *aml.RiskCalculator
	complianceEngine *aml.ComplianceEngine
	positionManager  *aml.PositionManager

	// Real-time subscribers
	subscribers map[string]*DashboardSubscriber

	// Alert management
	notifications []AlertNotification
	alertRules    map[string]*AlertRule

	// Performance tracking
	startTime         time.Time
	totalCalculations int64
	totalErrors       int64

	// Configuration
	refreshInterval time.Duration
	alertBuffer     int
	maxSubscribers  int
}

// AlertRule defines conditions for generating alerts
type AlertRule struct {
	ID         string                 `json:"id"`
	Name       string                 `json:"name"`
	Type       string                 `json:"type"`
	Condition  string                 `json:"condition"` // e.g., "risk_score > 80"
	Priority   string                 `json:"priority"`
	IsActive   bool                   `json:"is_active"`
	Cooldown   time.Duration          `json:"cooldown"`
	LastFired  time.Time              `json:"last_fired"`
	Parameters map[string]interface{} `json:"parameters"`
}

// NewMonitoringDashboard creates a new risk monitoring dashboard
func NewMonitoringDashboard(calculator *aml.RiskCalculator, complianceEngine *aml.ComplianceEngine, positionManager *aml.PositionManager) *MonitoringDashboard {
	dashboard := &MonitoringDashboard{
		calculator:       calculator,
		complianceEngine: complianceEngine,
		positionManager:  positionManager,
		subscribers:      make(map[string]*DashboardSubscriber),
		notifications:    make([]AlertNotification, 0),
		alertRules:       make(map[string]*AlertRule),
		startTime:        time.Now(),
		refreshInterval:  time.Second,
		alertBuffer:      1000,
		maxSubscribers:   100,
	}

	// Initialize default alert rules
	dashboard.initializeDefaultAlertRules()

	// Start background processes
	go dashboard.runMetricsCalculation()
	go dashboard.runAlertMonitoring()
	go dashboard.runNotificationDistribution()

	return dashboard
}

// initializeDefaultAlertRules sets up default alerting rules
func (md *MonitoringDashboard) initializeDefaultAlertRules() {
	defaultRules := []*AlertRule{
		{
			ID:        "high_risk_score",
			Name:      "High Risk Score Alert",
			Type:      "risk",
			Condition: "risk_score > 85",
			Priority:  "high",
			IsActive:  true,
			Cooldown:  5 * time.Minute,
		},
		{
			ID:        "critical_risk_score",
			Name:      "Critical Risk Score Alert",
			Type:      "risk",
			Condition: "risk_score > 95",
			Priority:  "critical",
			IsActive:  true,
			Cooldown:  1 * time.Minute,
		},
		{
			ID:        "large_exposure",
			Name:      "Large Exposure Alert",
			Type:      "exposure",
			Condition: "total_exposure > 1000000",
			Priority:  "medium",
			IsActive:  true,
			Cooldown:  10 * time.Minute,
		},
		{
			ID:        "limit_breach",
			Name:      "Position Limit Breach",
			Type:      "limit",
			Condition: "position_limit_utilization > 0.9",
			Priority:  "high",
			IsActive:  true,
			Cooldown:  2 * time.Minute,
		},
		{
			ID:        "compliance_alert",
			Name:      "Compliance Violation",
			Type:      "compliance",
			Condition: "compliance_alert_severity >= medium",
			Priority:  "high",
			IsActive:  true,
			Cooldown:  time.Minute,
		},
	}

	for _, rule := range defaultRules {
		md.alertRules[rule.ID] = rule
	}
}

// GetRealTimeMetrics calculates and returns current dashboard metrics
func (md *MonitoringDashboard) GetRealTimeMetrics(ctx context.Context) (*DashboardMetrics, error) {
	defer func() {
		md.mu.Lock()
		md.totalCalculations++
		md.mu.Unlock()
	}()

	// Collect system-wide metrics
	totalUsers := md.getTotalUsers()
	activePositions := md.getActivePositions()

	// Calculate aggregated risk metrics
	totalExposure, totalVaR, avgRiskScore := md.calculateAggregatedRiskMetrics(ctx)

	// Get alert statistics
	activeAlerts, criticalAlerts := md.getAlertStatistics()

	// Get compliance events
	complianceEvents := md.getComplianceEventCount()

	// Calculate system health
	systemHealth := md.calculateSystemHealth()

	// Get performance metrics
	perfMetrics := md.getPerformanceMetrics()

	// Get top risky users
	topRiskyUsers := md.getTopRiskyUsers(ctx, 10)

	// Get alerts by type
	alertsByType := md.getAlertsByType()

	// Get exposure by market
	exposureByMarket := md.getExposureByMarket(ctx)

	return &DashboardMetrics{
		Timestamp:          time.Now(),
		TotalUsers:         totalUsers,
		ActivePositions:    activePositions,
		TotalExposure:      totalExposure,
		TotalVaR:           totalVaR,
		AverageRiskScore:   avgRiskScore,
		ActiveAlerts:       activeAlerts,
		CriticalAlerts:     criticalAlerts,
		LimitBreaches:      0, // Would be calculated from position manager
		ComplianceEvents:   complianceEvents,
		SystemHealth:       systemHealth,
		PerformanceMetrics: perfMetrics,
		TopRiskyUsers:      topRiskyUsers,
		AlertsByType:       alertsByType,
		ExposureByMarket:   exposureByMarket,
	}, nil
}

// Subscribe allows clients to receive real-time dashboard updates
func (md *MonitoringDashboard) Subscribe(ctx context.Context, subscriberID string, filters map[string]interface{}) (*DashboardSubscriber, error) {
	md.mu.Lock()
	defer md.mu.Unlock()

	if len(md.subscribers) >= md.maxSubscribers {
		return nil, fmt.Errorf("maximum subscribers reached")
	}

	subscriber := &DashboardSubscriber{
		ID:       subscriberID,
		Channel:  make(chan DashboardUpdate, 100),
		Filters:  filters,
		LastSeen: time.Now(),
	}

	md.subscribers[subscriberID] = subscriber
	return subscriber, nil
}

// Unsubscribe removes a subscriber from dashboard updates
func (md *MonitoringDashboard) Unsubscribe(subscriberID string) {
	md.mu.Lock()
	defer md.mu.Unlock()

	if subscriber, exists := md.subscribers[subscriberID]; exists {
		close(subscriber.Channel)
		delete(md.subscribers, subscriberID)
	}
}

// SendAlert creates and distributes a new alert notification
func (md *MonitoringDashboard) SendAlert(alertType, priority, title, message, userID string, data map[string]interface{}) {
	notification := AlertNotification{
		ID:        fmt.Sprintf("alert_%d", time.Now().UnixNano()),
		Type:      alertType,
		Priority:  priority,
		Title:     title,
		Message:   message,
		UserID:    userID,
		Data:      data,
		Timestamp: time.Now(),
	}

	md.mu.Lock()
	md.notifications = append(md.notifications, notification)

	// Keep only recent notifications
	if len(md.notifications) > md.alertBuffer {
		md.notifications = md.notifications[len(md.notifications)-md.alertBuffer:]
	}
	md.mu.Unlock()

	// Distribute to subscribers
	update := DashboardUpdate{
		Type:      "alert",
		Data:      notification,
		Timestamp: time.Now(),
	}

	md.distributeUpdate(update)
}

// AcknowledgeAlert marks an alert as acknowledged
func (md *MonitoringDashboard) AcknowledgeAlert(alertID, acknowledgedBy string) error {
	md.mu.Lock()
	defer md.mu.Unlock()

	for i, alert := range md.notifications {
		if alert.ID == alertID {
			now := time.Now()
			md.notifications[i].Acknowledged = true
			md.notifications[i].AckedBy = acknowledgedBy
			md.notifications[i].AckedAt = &now
			return nil
		}
	}

	return fmt.Errorf("alert not found: %s", alertID)
}

// GetAlerts returns recent alert notifications
func (md *MonitoringDashboard) GetAlerts(limit int, priority string) []AlertNotification {
	md.mu.RLock()
	defer md.mu.RUnlock()

	alerts := make([]AlertNotification, 0)
	count := 0

	// Return alerts in reverse chronological order
	for i := len(md.notifications) - 1; i >= 0 && count < limit; i-- {
		alert := md.notifications[i]
		if priority == "" || alert.Priority == priority {
			alerts = append(alerts, alert)
			count++
		}
	}

	return alerts
}

// runMetricsCalculation runs the background metrics calculation loop
func (md *MonitoringDashboard) runMetricsCalculation() {
	ticker := time.NewTicker(md.refreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ctx := context.Background()
			metrics, err := md.GetRealTimeMetrics(ctx)
			if err != nil {
				md.mu.Lock()
				md.totalErrors++
				md.mu.Unlock()
				continue
			}

			// Distribute metrics update
			update := DashboardUpdate{
				Type:      "metrics",
				Data:      metrics,
				Timestamp: time.Now(),
			}

			md.distributeUpdate(update)
		}
	}
}

// runAlertMonitoring runs the background alert monitoring loop
func (md *MonitoringDashboard) runAlertMonitoring() {
	ticker := time.NewTicker(5 * time.Second) // Check alerts every 5 seconds
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			md.checkAlertRules()
		}
	}
}

// checkAlertRules evaluates all active alert rules
func (md *MonitoringDashboard) checkAlertRules() {
	md.mu.RLock()
	activeRules := make([]*AlertRule, 0)
	for _, rule := range md.alertRules {
		if rule.IsActive && time.Since(rule.LastFired) > rule.Cooldown {
			activeRules = append(activeRules, rule)
		}
	}
	md.mu.RUnlock()

	ctx := context.Background()
	metrics, err := md.GetRealTimeMetrics(ctx)
	if err != nil {
		return
	}

	for _, rule := range activeRules {
		if md.evaluateAlertRule(rule, metrics) {
			md.fireAlert(rule, metrics)
		}
	}
}

// evaluateAlertRule checks if an alert rule condition is met
func (md *MonitoringDashboard) evaluateAlertRule(rule *AlertRule, metrics *DashboardMetrics) bool {
	// Simplified rule evaluation - in production, use a proper expression evaluator
	switch rule.ID {
	case "high_risk_score":
		return metrics.AverageRiskScore.GreaterThan(decimal.NewFromInt(85))
	case "critical_risk_score":
		return metrics.AverageRiskScore.GreaterThan(decimal.NewFromInt(95))
	case "large_exposure":
		return metrics.TotalExposure.GreaterThan(decimal.NewFromInt(1000000))
	case "limit_breach":
		return metrics.LimitBreaches > 0
	case "compliance_alert":
		return metrics.CriticalAlerts > 0
	}

	return false
}

// fireAlert triggers an alert based on a rule
func (md *MonitoringDashboard) fireAlert(rule *AlertRule, metrics *DashboardMetrics) {
	md.mu.Lock()
	rule.LastFired = time.Now()
	md.mu.Unlock()

	message := fmt.Sprintf("Alert rule '%s' triggered", rule.Name)
	data := map[string]interface{}{
		"rule_id": rule.ID,
		"metrics": metrics,
	}

	md.SendAlert("system", rule.Priority, rule.Name, message, "", data)
}

// Helper methods for metrics calculation
func (md *MonitoringDashboard) getTotalUsers() int {
	// In production, this would query the database
	return 1000 // Placeholder
}

func (md *MonitoringDashboard) getActivePositions() int {
	// In production, this would count non-zero positions
	return 500 // Placeholder
}

func (md *MonitoringDashboard) calculateAggregatedRiskMetrics(ctx context.Context) (decimal.Decimal, decimal.Decimal, decimal.Decimal) {
	// In production, this would aggregate across all users
	totalExposure := decimal.NewFromInt(10000000) // $10M
	totalVaR := decimal.NewFromInt(500000)        // $500K
	avgRiskScore := decimal.NewFromInt(65)        // 65/100

	return totalExposure, totalVaR, avgRiskScore
}

func (md *MonitoringDashboard) getAlertStatistics() (int, int) {
	md.mu.RLock()
	defer md.mu.RUnlock()

	active := 0
	critical := 0

	for _, alert := range md.notifications {
		if !alert.Acknowledged {
			active++
			if alert.Priority == "critical" {
				critical++
			}
		}
	}

	return active, critical
}

func (md *MonitoringDashboard) getComplianceEventCount() int {
	metrics := md.getPerformanceMetrics()
	if alerts, ok := metrics["total_alerts"].(int64); ok {
		return int(alerts)
	}

	return 0
}

func (md *MonitoringDashboard) calculateSystemHealth() string {
	md.mu.RLock()
	errorRate := float64(md.totalErrors) / math.Max(float64(md.totalCalculations), 1)
	md.mu.RUnlock()

	if errorRate > 0.05 {
		return "degraded"
	} else if errorRate > 0.01 {
		return "warning"
	}

	return "healthy"
}

func (md *MonitoringDashboard) getPerformanceMetrics() PerformanceMetrics {
	uptime := time.Since(md.startTime)

	return PerformanceMetrics{
		CalculationLatency:  time.Millisecond * 50, // Placeholder
		ThroughputPerSecond: 1000,                  // Placeholder
		MemoryUsageMB:       128.5,                 // Placeholder
		CPUUsagePercent:     25.0,                  // Placeholder
		ErrorRate:           0.001,                 // Placeholder
		UptimeSeconds:       int64(uptime.Seconds()),
	}
}

func (md *MonitoringDashboard) getTopRiskyUsers(ctx context.Context, limit int) []UserRiskSummary {
	// In production, this would query and sort actual user risk data
	users := make([]UserRiskSummary, 0, limit)

	for i := 0; i < limit; i++ {
		users = append(users, UserRiskSummary{
			UserID:      fmt.Sprintf("user_%d", i+1),
			RiskScore:   decimal.NewFromInt(int64(90 - i*5)),
			Exposure:    decimal.NewFromInt(int64(100000 - i*10000)),
			VaR:         decimal.NewFromInt(int64(5000 - i*500)),
			AlertCount:  3 - i/3,
			LastUpdated: time.Now().Add(-time.Duration(i) * time.Minute),
		})
	}

	return users
}

func (md *MonitoringDashboard) getAlertsByType() map[string]int {
	md.mu.RLock()
	defer md.mu.RUnlock()

	counts := make(map[string]int)

	for _, alert := range md.notifications {
		if !alert.Acknowledged {
			counts[alert.Type]++
		}
	}

	return counts
}

func (md *MonitoringDashboard) getExposureByMarket(ctx context.Context) map[string]decimal.Decimal {
	// In production, this would aggregate exposure by market
	return map[string]decimal.Decimal{
		"BTCUSDT": decimal.NewFromInt(5000000),
		"ETHUSDT": decimal.NewFromInt(3000000),
		"ADAUSDT": decimal.NewFromInt(1000000),
		"DOTUSDT": decimal.NewFromInt(1000000),
	}
}

// runNotificationDistribution distributes updates to subscribers
func (md *MonitoringDashboard) runNotificationDistribution() {
	// This method would handle the distribution queue in production
}

// distributeUpdate sends an update to all eligible subscribers
func (md *MonitoringDashboard) distributeUpdate(update DashboardUpdate) {
	md.mu.RLock()
	subscribers := make([]*DashboardSubscriber, 0, len(md.subscribers))
	for _, sub := range md.subscribers {
		subscribers = append(subscribers, sub)
	}
	md.mu.RUnlock()

	for _, subscriber := range subscribers {
		select {
		case subscriber.Channel <- update:
			// Update sent successfully
		default:
			// Channel full, skip this subscriber
		}
	}
}

// GetDashboardJSON returns dashboard metrics as JSON
func (md *MonitoringDashboard) GetDashboardJSON(ctx context.Context) ([]byte, error) {
	metrics, err := md.GetRealTimeMetrics(ctx)
	if err != nil {
		return nil, err
	}

	return json.Marshal(metrics)
}
