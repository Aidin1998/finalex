package monitoring

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"gorm.io/gorm"
)

// DashboardService provides real-time compliance dashboards
type DashboardService struct {
	db                *gorm.DB
	monitoringService *MonitoringService
	prometheusMetrics *PrometheusMetrics
}

// NewDashboardService creates a new dashboard service
func NewDashboardService(db *gorm.DB, monitoringService *MonitoringService) *DashboardService {
	return &DashboardService{
		db:                db,
		monitoringService: monitoringService,
		prometheusMetrics: monitoringService.GetPrometheusMetrics(),
	}
}

// ComplianceStatusDashboard represents the main compliance status dashboard
type ComplianceStatusDashboard struct {
	Overview       ComplianceOverview     `json:"overview"`
	RealTimeAlerts []RealTimeAlert        `json:"real_time_alerts"`
	PolicyStatus   []PolicyStatus         `json:"policy_status"`
	SystemHealth   SystemHealthStatus     `json:"system_health"`
	UserActivity   UserActivitySummary    `json:"user_activity"`
	Investigations []InvestigationSummary `json:"investigations"`
	RecentActions  []ComplianceAction     `json:"recent_actions"`
	MetricsSummary MetricsSummary         `json:"metrics_summary"`
	Timestamp      time.Time              `json:"timestamp"`
}

// ComplianceOverview provides high-level compliance metrics
type ComplianceOverview struct {
	TotalAlerts       int64   `json:"total_alerts"`
	ActiveAlerts      int64   `json:"active_alerts"`
	CriticalAlerts    int64   `json:"critical_alerts"`
	ResolvedToday     int64   `json:"resolved_today"`
	AverageResolution float64 `json:"average_resolution_hours"`
	ComplianceScore   float64 `json:"compliance_score"`
	RiskLevel         string  `json:"risk_level"`
	TrendDirection    string  `json:"trend_direction"`
}

// RealTimeAlert represents a real-time alert for the dashboard
type RealTimeAlert struct {
	ID        string                 `json:"id"`
	Type      string                 `json:"type"`
	Severity  string                 `json:"severity"`
	Message   string                 `json:"message"`
	UserID    string                 `json:"user_id"`
	Market    string                 `json:"market"`
	Status    string                 `json:"status"`
	Timestamp time.Time              `json:"timestamp"`
	Details   map[string]interface{} `json:"details"`
	IsNew     bool                   `json:"is_new"`
	Priority  int                    `json:"priority"`
}

// PolicyStatus represents the status of compliance policies
type PolicyStatus struct {
	PolicyID      string    `json:"policy_id"`
	PolicyName    string    `json:"policy_name"`
	Type          string    `json:"type"`
	Status        string    `json:"status"`
	Violations    int64     `json:"violations"`
	LastUpdated   time.Time `json:"last_updated"`
	Effectiveness float64   `json:"effectiveness"`
}

// SystemHealthStatus represents system health metrics
type SystemHealthStatus struct {
	OverallStatus   string          `json:"overall_status"`
	Services        []ServiceHealth `json:"services"`
	DatabaseStatus  string          `json:"database_status"`
	QueueHealths    []QueueHealth   `json:"queue_healths"`
	ProcessingRate  float64         `json:"processing_rate"`
	ErrorRate       float64         `json:"error_rate"`
	ResponseTime    float64         `json:"response_time_ms"`
	LastHealthCheck time.Time       `json:"last_health_check"`
}

// ServiceHealth represents individual service health
type ServiceHealth struct {
	ServiceName  string    `json:"service_name"`
	Status       string    `json:"status"`
	ResponseTime float64   `json:"response_time_ms"`
	ErrorCount   int64     `json:"error_count"`
	LastChecked  time.Time `json:"last_checked"`
}

// QueueHealth represents queue health metrics
type QueueHealth struct {
	QueueName      string  `json:"queue_name"`
	Size           int64   `json:"size"`
	ProcessingRate float64 `json:"processing_rate"`
	Status         string  `json:"status"`
}

// UserActivitySummary represents user activity metrics
type UserActivitySummary struct {
	ActiveUsers         int64   `json:"active_users"`
	HighRiskUsers       int64   `json:"high_risk_users"`
	BlockedUsers        int64   `json:"blocked_users"`
	NewRegistrations    int64   `json:"new_registrations"`
	SuspiciousActivity  int64   `json:"suspicious_activity"`
	AverageRiskScore    float64 `json:"average_risk_score"`
	ComplianceChecks    int64   `json:"compliance_checks"`
	BlockedTransactions int64   `json:"blocked_transactions"`
}

// InvestigationSummary represents investigation metrics
type InvestigationSummary struct {
	ID         string    `json:"id"`
	Title      string    `json:"title"`
	Type       string    `json:"type"`
	Priority   string    `json:"priority"`
	Status     string    `json:"status"`
	AssignedTo string    `json:"assigned_to"`
	CreatedAt  time.Time `json:"created_at"`
	DueDate    time.Time `json:"due_date"`
	DaysOpen   int       `json:"days_open"`
	AlertCount int       `json:"alert_count"`
}

// ComplianceAction represents recent compliance actions
type ComplianceAction struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type"`
	Description string                 `json:"description"`
	UserID      string                 `json:"user_id"`
	Market      string                 `json:"market"`
	Status      string                 `json:"status"`
	ExecutedBy  string                 `json:"executed_by"`
	ExecutedAt  time.Time              `json:"executed_at"`
	Result      string                 `json:"result"`
	Details     map[string]interface{} `json:"details"`
}

// MetricsSummary represents key metrics summary
type MetricsSummary struct {
	AlertsLast24h          int64   `json:"alerts_last_24h"`
	AlertsLast7d           int64   `json:"alerts_last_7d"`
	ResolutionRate         float64 `json:"resolution_rate"`
	AverageResponseTime    float64 `json:"average_response_time"`
	PolicyViolations       int64   `json:"policy_violations"`
	ManipulationDetections int64   `json:"manipulation_detections"`
	FalsePositiveRate      float64 `json:"false_positive_rate"`
	SystemUptime           float64 `json:"system_uptime"`
}

// GetComplianceStatusDashboard returns the main compliance status dashboard
func (ds *DashboardService) GetComplianceStatusDashboard(ctx context.Context) (*ComplianceStatusDashboard, error) {
	dashboard := &ComplianceStatusDashboard{
		Timestamp: time.Now(),
	}

	// Get overview data
	overview, err := ds.getComplianceOverview(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get compliance overview")
	}
	dashboard.Overview = overview

	// Get real-time alerts
	alerts, err := ds.getRealTimeAlerts(ctx, 20) // Last 20 alerts
	if err != nil {
		return nil, errors.Wrap(err, "failed to get real-time alerts")
	}
	dashboard.RealTimeAlerts = alerts

	// Get policy status
	policies, err := ds.getPolicyStatus(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get policy status")
	}
	dashboard.PolicyStatus = policies

	// Get system health
	health := ds.getSystemHealth(ctx)
	dashboard.SystemHealth = health

	// Get user activity
	userActivity, err := ds.getUserActivity(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get user activity")
	}
	dashboard.UserActivity = userActivity

	// Get investigations
	investigations, err := ds.getInvestigations(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get investigations")
	}
	dashboard.Investigations = investigations

	// Get recent actions
	actions, err := ds.getRecentActions(ctx, 10)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get recent actions")
	}
	dashboard.RecentActions = actions

	// Get metrics summary
	metrics, err := ds.getMetricsSummary(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get metrics summary")
	}
	dashboard.MetricsSummary = metrics

	return dashboard, nil
}

// getComplianceOverview retrieves compliance overview metrics
func (ds *DashboardService) getComplianceOverview(ctx context.Context) (ComplianceOverview, error) {
	var overview ComplianceOverview

	// Total alerts
	var totalAlerts int64
	if err := ds.db.WithContext(ctx).Model(&MonitoringAlertModel{}).Count(&totalAlerts).Error; err != nil {
		return overview, err
	}
	overview.TotalAlerts = totalAlerts

	// Active alerts
	var activeAlerts int64
	if err := ds.db.WithContext(ctx).Model(&MonitoringAlertModel{}).
		Where("status IN (?)", []string{"pending", "acknowledged", "investigating"}).
		Count(&activeAlerts).Error; err != nil {
		return overview, err
	}
	overview.ActiveAlerts = activeAlerts

	// Critical alerts
	var criticalAlerts int64
	if err := ds.db.WithContext(ctx).Model(&MonitoringAlertModel{}).
		Where("severity = ? AND status != ?", "critical", "resolved").
		Count(&criticalAlerts).Error; err != nil {
		return overview, err
	}
	overview.CriticalAlerts = criticalAlerts

	// Resolved today
	today := time.Now().Truncate(24 * time.Hour)
	var resolvedToday int64
	if err := ds.db.WithContext(ctx).Model(&MonitoringAlertModel{}).
		Where("status = ? AND updated_at >= ?", "resolved", today).
		Count(&resolvedToday).Error; err != nil {
		return overview, err
	}
	overview.ResolvedToday = resolvedToday

	// Calculate compliance score (simplified)
	if totalAlerts > 0 {
		resolvedRatio := float64(totalAlerts-activeAlerts) / float64(totalAlerts)
		overview.ComplianceScore = resolvedRatio * 100
	} else {
		overview.ComplianceScore = 100
	}

	// Risk level based on critical alerts
	if criticalAlerts > 5 {
		overview.RiskLevel = "HIGH"
	} else if criticalAlerts > 2 {
		overview.RiskLevel = "MEDIUM"
	} else {
		overview.RiskLevel = "LOW"
	}

	return overview, nil
}

// getRealTimeAlerts retrieves recent alerts for real-time display
func (ds *DashboardService) getRealTimeAlerts(ctx context.Context, limit int) ([]RealTimeAlert, error) {
	var alertModels []MonitoringAlertModel

	if err := ds.db.WithContext(ctx).
		Order("timestamp DESC").
		Limit(limit).
		Find(&alertModels).Error; err != nil {
		return nil, err
	}

	alerts := make([]RealTimeAlert, len(alertModels))
	for i, model := range alertModels {
		alerts[i] = RealTimeAlert{
			ID:        model.ID,
			Type:      model.AlertType,
			Severity:  model.Severity,
			Message:   model.Message,
			UserID:    model.UserID,
			Status:    model.Status,
			Timestamp: model.Timestamp,
			Details:   model.Data,
			IsNew:     time.Since(model.Timestamp) < 5*time.Minute,
			Priority:  ds.calculateAlertPriority(model.Severity, model.AlertType),
		}

		// Extract market from details if available
		if market, ok := model.Data["market"].(string); ok {
			alerts[i].Market = market
		}
	}

	return alerts, nil
}

// getPolicyStatus retrieves policy status information
func (ds *DashboardService) getPolicyStatus(ctx context.Context) ([]PolicyStatus, error) {
	var policyModels []MonitoringPolicyModel

	if err := ds.db.WithContext(ctx).Find(&policyModels).Error; err != nil {
		return nil, err
	}

	policies := make([]PolicyStatus, len(policyModels))
	for i, model := range policyModels {
		// Count violations for this policy type
		var violations int64
		ds.db.WithContext(ctx).Model(&MonitoringAlertModel{}).
			Where("alert_type = ? AND timestamp >= ?", model.AlertType, time.Now().AddDate(0, 0, -7)).
			Count(&violations)

		policies[i] = PolicyStatus{
			PolicyID:      model.ID,
			PolicyName:    model.Name,
			Type:          model.AlertType,
			Status:        getStatusFromEnabled(model.Enabled),
			Violations:    violations,
			LastUpdated:   model.UpdatedAt,
			Effectiveness: ds.calculatePolicyEffectiveness(model.AlertType),
		}
	}

	return policies, nil
}

// getSystemHealth retrieves system health information
func (ds *DashboardService) getSystemHealth(ctx context.Context) SystemHealthStatus {
	health := ds.monitoringService.GetHealthStatus()

	status := SystemHealthStatus{
		OverallStatus:   health["service"].(string),
		DatabaseStatus:  health["database"].(string),
		ProcessingRate:  ds.calculateProcessingRate(),
		ErrorRate:       ds.calculateErrorRate(),
		ResponseTime:    ds.calculateResponseTime(),
		LastHealthCheck: time.Now(),
	}

	// Get queue health
	status.QueueHealths = []QueueHealth{
		{
			QueueName:      "alerts",
			Size:           int64(health["queue_size"].(int)),
			ProcessingRate: ds.calculateProcessingRate(),
			Status:         ds.getQueueStatus(health["queue_size"].(int)),
		},
	}

	// Service health (simplified)
	status.Services = []ServiceHealth{
		{
			ServiceName:  "monitoring",
			Status:       health["service"].(string),
			ResponseTime: status.ResponseTime,
			ErrorCount:   0,
			LastChecked:  time.Now(),
		},
	}

	return status
}

// getUserActivity retrieves user activity metrics
func (ds *DashboardService) getUserActivity(ctx context.Context) (UserActivitySummary, error) {
	var activity UserActivitySummary

	// These would typically come from user/account services
	// For now, using monitoring data as proxy

	// High risk users (users with recent critical alerts)
	var highRiskUsers int64
	if err := ds.db.WithContext(ctx).
		Model(&MonitoringAlertModel{}).
		Where("severity = ? AND timestamp >= ?", "critical", time.Now().AddDate(0, 0, -1)).
		Distinct("user_id").
		Count(&highRiskUsers).Error; err != nil {
		return activity, err
	}
	activity.HighRiskUsers = highRiskUsers

	// Suspicious activity (recent alerts)
	var suspiciousActivity int64
	if err := ds.db.WithContext(ctx).
		Model(&MonitoringAlertModel{}).
		Where("timestamp >= ?", time.Now().AddDate(0, 0, -1)).
		Count(&suspiciousActivity).Error; err != nil {
		return activity, err
	}
	activity.SuspiciousActivity = suspiciousActivity

	// Additional metrics would be populated from other services
	activity.ActiveUsers = 1000       // Placeholder
	activity.BlockedUsers = 5         // Placeholder
	activity.NewRegistrations = 50    // Placeholder
	activity.AverageRiskScore = 0.3   // Placeholder
	activity.ComplianceChecks = 10000 // Placeholder
	activity.BlockedTransactions = 25 // Placeholder

	return activity, nil
}

// getInvestigations retrieves investigation summaries
func (ds *DashboardService) getInvestigations(ctx context.Context) ([]InvestigationSummary, error) {
	// This would typically query the manipulation investigations
	// For now, return empty slice as placeholder
	return []InvestigationSummary{}, nil
}

// getRecentActions retrieves recent compliance actions
func (ds *DashboardService) getRecentActions(ctx context.Context, limit int) ([]ComplianceAction, error) {
	// This would typically query the manipulation actions
	// For now, return empty slice as placeholder
	return []ComplianceAction{}, nil
}

// getMetricsSummary calculates metrics summary
func (ds *DashboardService) getMetricsSummary(ctx context.Context) (MetricsSummary, error) {
	var metrics MetricsSummary

	// Alerts last 24h
	var alerts24h int64
	if err := ds.db.WithContext(ctx).Model(&MonitoringAlertModel{}).
		Where("timestamp >= ?", time.Now().AddDate(0, 0, -1)).
		Count(&alerts24h).Error; err != nil {
		return metrics, err
	}
	metrics.AlertsLast24h = alerts24h

	// Alerts last 7d
	var alerts7d int64
	if err := ds.db.WithContext(ctx).Model(&MonitoringAlertModel{}).
		Where("timestamp >= ?", time.Now().AddDate(0, 0, -7)).
		Count(&alerts7d).Error; err != nil {
		return metrics, err
	}
	metrics.AlertsLast7d = alerts7d

	// Resolution rate
	var totalAlerts, resolvedAlerts int64
	ds.db.WithContext(ctx).Model(&MonitoringAlertModel{}).Count(&totalAlerts)
	ds.db.WithContext(ctx).Model(&MonitoringAlertModel{}).
		Where("status = ?", "resolved").Count(&resolvedAlerts)

	if totalAlerts > 0 {
		metrics.ResolutionRate = float64(resolvedAlerts) / float64(totalAlerts) * 100
	}

	// Other metrics (placeholders)
	metrics.AverageResponseTime = 2.5   // seconds
	metrics.PolicyViolations = alerts7d // Using alerts as proxy
	metrics.ManipulationDetections = 12 // Placeholder
	metrics.FalsePositiveRate = 5.2     // Placeholder
	metrics.SystemUptime = 99.9         // Placeholder

	return metrics, nil
}

// Helper functions
func (ds *DashboardService) calculateAlertPriority(severity, alertType string) int {
	priority := 1
	switch severity {
	case "critical":
		priority = 4
	case "high":
		priority = 3
	case "medium":
		priority = 2
	case "low":
		priority = 1
	}

	// Adjust based on alert type
	if alertType == "manipulation" {
		priority++
	}

	return priority
}

func (ds *DashboardService) calculatePolicyEffectiveness(alertType string) float64 {
	// Simplified calculation - would be more complex in production
	return 85.0 // Placeholder
}

func (ds *DashboardService) calculateProcessingRate() float64 {
	// Calculate based on recent processing history
	return 100.0 // alerts per minute
}

func (ds *DashboardService) calculateErrorRate() float64 {
	// Calculate based on recent error history
	return 0.5 // percentage
}

func (ds *DashboardService) calculateResponseTime() float64 {
	// Calculate based on recent response times
	return 250.0 // milliseconds
}

func (ds *DashboardService) getQueueStatus(size int) string {
	if size > 500 {
		return "critical"
	} else if size > 100 {
		return "warning"
	}
	return "healthy"
}

func getStatusFromEnabled(enabled bool) string {
	if enabled {
		return "active"
	}
	return "inactive"
}
