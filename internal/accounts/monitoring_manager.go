package accounts

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"
)

// AlertSeverity defines alert severity levels
type AlertSeverity string

const (
	SeverityCritical AlertSeverity = "critical"
	SeverityHigh     AlertSeverity = "high"
	SeverityMedium   AlertSeverity = "medium"
	SeverityLow      AlertSeverity = "low"
	SeverityInfo     AlertSeverity = "info"
)

// AlertRule defines monitoring alert rules
type AlertRule struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	MetricName  string            `json:"metric_name"`
	Condition   string            `json:"condition"` // >, <, >=, <=, ==, !=
	Threshold   float64           `json:"threshold"`
	Duration    time.Duration     `json:"duration"`
	Severity    AlertSeverity     `json:"severity"`
	Enabled     bool              `json:"enabled"`
	Labels      map[string]string `json:"labels"`
	Actions     []string          `json:"actions"` // email, slack, webhook, etc.
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// Alert represents an active alert
type Alert struct {
	ID          string                 `json:"id"`
	RuleID      string                 `json:"rule_id"`
	RuleName    string                 `json:"rule_name"`
	Severity    AlertSeverity          `json:"severity"`
	Message     string                 `json:"message"`
	Value       float64                `json:"value"`
	Threshold   float64                `json:"threshold"`
	StartTime   time.Time              `json:"start_time"`
	EndTime     *time.Time             `json:"end_time,omitempty"`
	Status      string                 `json:"status"` // firing, resolved, silenced
	Labels      map[string]string      `json:"labels"`
	Annotations map[string]interface{} `json:"annotations"`
}

// MetricSnapshot represents a point-in-time metric value
type MetricSnapshot struct {
	Name      string                 `json:"name"`
	Value     float64                `json:"value"`
	Labels    map[string]string      `json:"labels"`
	Timestamp time.Time              `json:"timestamp"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// HealthCheck represents a system health check
type HealthCheck struct {
	Name         string                 `json:"name"`
	Status       string                 `json:"status"` // healthy, degraded, unhealthy
	Message      string                 `json:"message"`
	LastCheck    time.Time              `json:"last_check"`
	ResponseTime time.Duration          `json:"response_time"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// PerformanceReport contains system performance metrics
type PerformanceReport struct {
	GeneratedAt         time.Time             `json:"generated_at"`
	TimeRange           time.Duration         `json:"time_range"`
	TotalOperations     int64                 `json:"total_operations"`
	OperationsPerSecond float64               `json:"operations_per_second"`
	AverageLatency      time.Duration         `json:"average_latency"`
	P95Latency          time.Duration         `json:"p95_latency"`
	P99Latency          time.Duration         `json:"p99_latency"`
	ErrorRate           float64               `json:"error_rate"`
	CacheHitRate        float64               `json:"cache_hit_rate"`
	DatabaseConnections int                   `json:"database_connections"`
	MemoryUsage         float64               `json:"memory_usage"`
	CPUUsage            float64               `json:"cpu_usage"`
	DiskUsage           float64               `json:"disk_usage"`
	AlertsSummary       map[AlertSeverity]int `json:"alerts_summary"`
	TopErrors           []string              `json:"top_errors"`
	Recommendations     []string              `json:"recommendations"`
	MetricsSummary      map[string]float64    `json:"metrics_summary"`
}

// AdvancedMonitoringManager provides comprehensive monitoring and alerting
type AdvancedMonitoringManager struct {
	db                 *sql.DB
	logger             *zap.Logger
	alertRules         map[string]*AlertRule
	activeAlerts       map[string]*Alert
	healthChecks       map[string]*HealthCheck
	metricSnapshots    []*MetricSnapshot
	performanceHistory []*PerformanceReport
	mu                 sync.RWMutex
	alertMu            sync.RWMutex
	healthMu           sync.RWMutex

	// Configuration
	alertCheckInterval     time.Duration
	healthCheckInterval    time.Duration
	metricRetentionPeriod  time.Duration
	maxSnapshots           int
	maxReports             int
	enableAlerts           bool
	enableHealthChecks     bool
	enableMetricCollection bool

	// Background tasks
	stopChan  chan struct{}
	isRunning bool

	// External integrations
	dataManager      *AccountDataManager
	hotCache         *HotCache
	shardManager     *ShardManager
	lifecycleManager *DataLifecycleManager
	migrationManager *MigrationManager

	// Metrics
	alertsTotal        prometheus.Counter
	alertsFiring       prometheus.Gauge
	healthChecksTotal  prometheus.Counter
	healthChecksFailed prometheus.Counter
	monitoringLatency  prometheus.Histogram
	systemHealth       prometheus.Gauge
	performanceScore   prometheus.Gauge
}

// NewAdvancedMonitoringManager creates a new advanced monitoring manager
func NewAdvancedMonitoringManager(
	db *sql.DB,
	logger *zap.Logger,
	dataManager *AccountDataManager,
	hotCache *HotCache,
	shardManager *ShardManager,
	lifecycleManager *DataLifecycleManager,
	migrationManager *MigrationManager,
) *AdvancedMonitoringManager {
	amm := &AdvancedMonitoringManager{
		db:                     db,
		logger:                 logger,
		alertRules:             make(map[string]*AlertRule),
		activeAlerts:           make(map[string]*Alert),
		healthChecks:           make(map[string]*HealthCheck),
		alertCheckInterval:     time.Minute,
		healthCheckInterval:    time.Minute * 5,
		metricRetentionPeriod:  time.Hour * 24 * 7, // 7 days
		maxSnapshots:           10000,
		maxReports:             100,
		enableAlerts:           true,
		enableHealthChecks:     true,
		enableMetricCollection: true,
		stopChan:               make(chan struct{}),
		dataManager:            dataManager,
		hotCache:               hotCache,
		shardManager:           shardManager,
		lifecycleManager:       lifecycleManager,
		migrationManager:       migrationManager,
	}

	amm.initMetrics()
	amm.loadDefaultAlertRules()
	amm.initHealthChecks()

	return amm
}

// initMetrics initializes Prometheus metrics
func (amm *AdvancedMonitoringManager) initMetrics() {
	amm.alertsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "accounts_monitoring_alerts_total",
		Help: "Total number of alerts generated",
	})

	amm.alertsFiring = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "accounts_monitoring_alerts_firing",
		Help: "Number of currently firing alerts",
	})

	amm.healthChecksTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "accounts_monitoring_health_checks_total",
		Help: "Total number of health checks performed",
	})

	amm.healthChecksFailed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "accounts_monitoring_health_checks_failed_total",
		Help: "Total number of failed health checks",
	})

	amm.monitoringLatency = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "accounts_monitoring_latency_seconds",
		Help:    "Latency of monitoring operations",
		Buckets: prometheus.DefBuckets,
	})

	amm.systemHealth = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "accounts_system_health_score",
		Help: "Overall system health score (0-100)",
	})

	amm.performanceScore = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "accounts_performance_score",
		Help: "Overall system performance score (0-100)",
	})
}

// loadDefaultAlertRules loads default monitoring alert rules
func (amm *AdvancedMonitoringManager) loadDefaultAlertRules() {
	defaultRules := []*AlertRule{
		{
			ID:          "high_error_rate",
			Name:        "High Error Rate",
			Description: "Error rate exceeds 5%",
			MetricName:  "accounts_operations_errors_total",
			Condition:   ">",
			Threshold:   0.05,
			Duration:    time.Minute * 5,
			Severity:    SeverityCritical,
			Enabled:     true,
			Actions:     []string{"email", "slack"},
		},
		{
			ID:          "low_cache_hit_rate",
			Name:        "Low Cache Hit Rate",
			Description: "Cache hit rate below 80%",
			MetricName:  "accounts_cache_hit_rate",
			Condition:   "<",
			Threshold:   0.8,
			Duration:    time.Minute * 10,
			Severity:    SeverityHigh,
			Enabled:     true,
			Actions:     []string{"email"},
		},
		{
			ID:          "high_latency",
			Name:        "High Response Latency",
			Description: "P95 latency exceeds 100ms",
			MetricName:  "accounts_operation_duration_seconds",
			Condition:   ">",
			Threshold:   0.1,
			Duration:    time.Minute * 5,
			Severity:    SeverityHigh,
			Enabled:     true,
			Actions:     []string{"email"},
		},
		{
			ID:          "database_connections_high",
			Name:        "High Database Connections",
			Description: "Database connections exceed 80% of pool",
			MetricName:  "accounts_database_connections_active",
			Condition:   ">",
			Threshold:   80,
			Duration:    time.Minute * 5,
			Severity:    SeverityMedium,
			Enabled:     true,
			Actions:     []string{"email"},
		},
		{
			ID:          "shard_imbalance",
			Name:        "Shard Load Imbalance",
			Description: "Shard load factor exceeds 1.5",
			MetricName:  "accounts_shard_load_factor",
			Condition:   ">",
			Threshold:   1.5,
			Duration:    time.Minute * 15,
			Severity:    SeverityMedium,
			Enabled:     true,
			Actions:     []string{"email"},
		},
		{
			ID:          "migration_failed",
			Name:        "Migration Failed",
			Description: "Database migration has failed",
			MetricName:  "accounts_migrations_failed_total",
			Condition:   ">",
			Threshold:   0,
			Duration:    time.Minute,
			Severity:    SeverityCritical,
			Enabled:     true,
			Actions:     []string{"email", "slack"},
		},
		{
			ID:          "lifecycle_job_failed",
			Name:        "Lifecycle Job Failed",
			Description: "Data lifecycle job has failed",
			MetricName:  "accounts_lifecycle_jobs_failed_total",
			Condition:   ">",
			Threshold:   0,
			Duration:    time.Minute,
			Severity:    SeverityHigh,
			Enabled:     true,
			Actions:     []string{"email"},
		},
	}

	for _, rule := range defaultRules {
		rule.CreatedAt = time.Now()
		rule.UpdatedAt = time.Now()
		rule.Labels = make(map[string]string)
		amm.alertRules[rule.ID] = rule
	}
}

// initHealthChecks initializes system health checks
func (amm *AdvancedMonitoringManager) initHealthChecks() {
	healthChecks := map[string]*HealthCheck{
		"database": {
			Name:    "Database Connection",
			Status:  "unknown",
			Message: "Not checked yet",
		},
		"cache": {
			Name:    "Cache System",
			Status:  "unknown",
			Message: "Not checked yet",
		},
		"sharding": {
			Name:    "Sharding System",
			Status:  "unknown",
			Message: "Not checked yet",
		},
		"lifecycle": {
			Name:    "Data Lifecycle",
			Status:  "unknown",
			Message: "Not checked yet",
		},
		"migrations": {
			Name:    "Migration System",
			Status:  "unknown",
			Message: "Not checked yet",
		},
	}

	amm.healthMu.Lock()
	amm.healthChecks = healthChecks
	amm.healthMu.Unlock()
}

// Start begins the monitoring operations
func (amm *AdvancedMonitoringManager) Start(ctx context.Context) error {
	if amm.isRunning {
		return fmt.Errorf("monitoring manager is already running")
	}

	amm.isRunning = true

	amm.logger.Info("Starting advanced monitoring manager",
		zap.Duration("alert_check_interval", amm.alertCheckInterval),
		zap.Duration("health_check_interval", amm.healthCheckInterval))

	// Start background monitoring routines
	go amm.runAlertMonitoring(ctx)
	go amm.runHealthMonitoring(ctx)
	go amm.runMetricCollection(ctx)
	go amm.runPerformanceReporting(ctx)

	return nil
}

// Stop gracefully stops the monitoring manager
func (amm *AdvancedMonitoringManager) Stop() error {
	if !amm.isRunning {
		return nil
	}

	amm.logger.Info("Stopping advanced monitoring manager")

	close(amm.stopChan)
	amm.isRunning = false

	return nil
}

// runAlertMonitoring runs the alert monitoring loop
func (amm *AdvancedMonitoringManager) runAlertMonitoring(ctx context.Context) {
	ticker := time.NewTicker(amm.alertCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-amm.stopChan:
			return
		case <-ticker.C:
			if amm.enableAlerts {
				amm.checkAlertRules(ctx)
			}
		}
	}
}

// runHealthMonitoring runs the health monitoring loop
func (amm *AdvancedMonitoringManager) runHealthMonitoring(ctx context.Context) {
	ticker := time.NewTicker(amm.healthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-amm.stopChan:
			return
		case <-ticker.C:
			if amm.enableHealthChecks {
				amm.performHealthChecks(ctx)
			}
		}
	}
}

// runMetricCollection runs the metric collection loop
func (amm *AdvancedMonitoringManager) runMetricCollection(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-amm.stopChan:
			return
		case <-ticker.C:
			if amm.enableMetricCollection {
				amm.collectMetrics(ctx)
			}
		}
	}
}

// runPerformanceReporting runs the performance reporting loop
func (amm *AdvancedMonitoringManager) runPerformanceReporting(ctx context.Context) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-amm.stopChan:
			return
		case <-ticker.C:
			amm.generatePerformanceReport(ctx)
		}
	}
}

// checkAlertRules evaluates all alert rules and fires alerts if needed
func (amm *AdvancedMonitoringManager) checkAlertRules(ctx context.Context) {
	startTime := time.Now()
	defer func() {
		amm.monitoringLatency.Observe(time.Since(startTime).Seconds())
	}()

	amm.mu.RLock()
	rules := make([]*AlertRule, 0, len(amm.alertRules))
	for _, rule := range amm.alertRules {
		if rule.Enabled {
			rules = append(rules, rule)
		}
	}
	amm.mu.RUnlock()

	for _, rule := range rules {
		amm.evaluateAlertRule(ctx, rule)
	}

	// Update firing alerts metric
	amm.alertMu.RLock()
	firingCount := 0
	for _, alert := range amm.activeAlerts {
		if alert.Status == "firing" {
			firingCount++
		}
	}
	amm.alertMu.RUnlock()

	amm.alertsFiring.Set(float64(firingCount))
}

// evaluateAlertRule evaluates a single alert rule
func (amm *AdvancedMonitoringManager) evaluateAlertRule(ctx context.Context, rule *AlertRule) {
	// Get current metric value
	value, err := amm.getMetricValue(rule.MetricName)
	if err != nil {
		amm.logger.Error("Failed to get metric value for alert rule",
			zap.String("rule_id", rule.ID),
			zap.String("metric", rule.MetricName),
			zap.Error(err))
		return
	}

	// Check if condition is met
	conditionMet := amm.evaluateCondition(value, rule.Condition, rule.Threshold)

	amm.alertMu.Lock()
	existingAlert, exists := amm.activeAlerts[rule.ID]
	amm.alertMu.Unlock()

	if conditionMet {
		if !exists || existingAlert.Status != "firing" {
			// Fire new alert
			alert := &Alert{
				ID:        fmt.Sprintf("%s_%d", rule.ID, time.Now().Unix()),
				RuleID:    rule.ID,
				RuleName:  rule.Name,
				Severity:  rule.Severity,
				Message:   fmt.Sprintf("%s: %.2f %s %.2f", rule.Description, value, rule.Condition, rule.Threshold),
				Value:     value,
				Threshold: rule.Threshold,
				StartTime: time.Now(),
				Status:    "firing",
				Labels:    rule.Labels,
				Annotations: map[string]interface{}{
					"metric_name": rule.MetricName,
					"condition":   rule.Condition,
				},
			}

			amm.alertMu.Lock()
			amm.activeAlerts[rule.ID] = alert
			amm.alertMu.Unlock()

			amm.alertsTotal.Inc()

			amm.logger.Warn("Alert fired",
				zap.String("rule_id", rule.ID),
				zap.String("rule_name", rule.Name),
				zap.String("severity", string(rule.Severity)),
				zap.Float64("value", value),
				zap.Float64("threshold", rule.Threshold))

			// Execute alert actions
			amm.executeAlertActions(alert, rule.Actions)
		}
	} else if exists && existingAlert.Status == "firing" {
		// Resolve alert
		now := time.Now()
		existingAlert.EndTime = &now
		existingAlert.Status = "resolved"

		amm.logger.Info("Alert resolved",
			zap.String("rule_id", rule.ID),
			zap.String("rule_name", rule.Name),
			zap.Duration("duration", now.Sub(existingAlert.StartTime)))
	}
}

// evaluateCondition evaluates a condition
func (amm *AdvancedMonitoringManager) evaluateCondition(value float64, condition string, threshold float64) bool {
	switch condition {
	case ">":
		return value > threshold
	case "<":
		return value < threshold
	case ">=":
		return value >= threshold
	case "<=":
		return value <= threshold
	case "==":
		return value == threshold
	case "!=":
		return value != threshold
	default:
		return false
	}
}

// getMetricValue retrieves the current value of a metric
func (amm *AdvancedMonitoringManager) getMetricValue(metricName string) (float64, error) {
	// This would integrate with Prometheus or other metric stores
	// For now, return simulated values based on system state

	switch metricName {
	case "accounts_operations_errors_total":
		// Calculate error rate from data manager
		// Metrics collection for dataManager not implemented or not available
		return 0.01, nil // 1% error rate

	case "accounts_cache_hit_rate":
		if amm.hotCache != nil {
			stats := amm.hotCache.GetStats()
			return stats.HitRate, nil
		}
		return 0.85, nil // 85% hit rate

	case "accounts_operation_duration_seconds":
		// Return P95 latency
		return 0.05, nil // 50ms

	case "accounts_database_connections_active":
		return 45, nil // 45 active connections

	case "accounts_shard_load_factor":
		// Metrics collection for shardManager not implemented or not available
		return 1.2, nil

	case "accounts_migrations_failed_total":
		if amm.migrationManager != nil {
			metrics := amm.migrationManager.GetMetrics()
			if failed, ok := metrics["failed_migrations"].(int); ok {
				return float64(failed), nil
			}
		}
		return 0, nil

	case "accounts_lifecycle_jobs_failed_total":
		if amm.lifecycleManager != nil {
			metrics := amm.lifecycleManager.GetMetrics()
			if failed, ok := metrics["jobs_failed"].(int); ok {
				return float64(failed), nil
			}
		}
		return 0, nil

	default:
		return 0, fmt.Errorf("unknown metric: %s", metricName)
	}
}

// executeAlertActions executes the actions for an alert
func (amm *AdvancedMonitoringManager) executeAlertActions(alert *Alert, actions []string) {
	for _, action := range actions {
		switch action {
		case "email":
			amm.sendEmailAlert(alert)
		case "slack":
			amm.sendSlackAlert(alert)
		case "webhook":
			amm.sendWebhookAlert(alert)
		default:
			amm.logger.Warn("Unknown alert action", zap.String("action", action))
		}
	}
}

// sendEmailAlert sends an email alert (placeholder)
func (amm *AdvancedMonitoringManager) sendEmailAlert(alert *Alert) {
	amm.logger.Info("Sending email alert",
		zap.String("alert_id", alert.ID),
		zap.String("message", alert.Message))
	// Implement email sending logic
}

// sendSlackAlert sends a Slack alert (placeholder)
func (amm *AdvancedMonitoringManager) sendSlackAlert(alert *Alert) {
	amm.logger.Info("Sending Slack alert",
		zap.String("alert_id", alert.ID),
		zap.String("message", alert.Message))
	// Implement Slack integration
}

// sendWebhookAlert sends a webhook alert (placeholder)
func (amm *AdvancedMonitoringManager) sendWebhookAlert(alert *Alert) {
	amm.logger.Info("Sending webhook alert",
		zap.String("alert_id", alert.ID),
		zap.String("message", alert.Message))
	// Implement webhook integration
}

// performHealthChecks performs all configured health checks
func (amm *AdvancedMonitoringManager) performHealthChecks(ctx context.Context) {
	startTime := time.Now()

	checks := []func(context.Context) *HealthCheck{
		amm.checkDatabase,
		amm.checkCache,
		amm.checkSharding,
		amm.checkLifecycle,
		amm.checkMigrations,
	}

	for _, check := range checks {
		result := check(ctx)

		amm.healthMu.Lock()
		amm.healthChecks[result.Name] = result
		amm.healthMu.Unlock()

		amm.healthChecksTotal.Inc()
		if result.Status != "healthy" {
			amm.healthChecksFailed.Inc()
		}
	}

	// Calculate overall system health
	healthScore := amm.calculateSystemHealth()
	amm.systemHealth.Set(healthScore)

	amm.logger.Debug("Health checks completed",
		zap.Duration("duration", time.Since(startTime)),
		zap.Float64("health_score", healthScore))
}

// checkDatabase performs database health check
func (amm *AdvancedMonitoringManager) checkDatabase(ctx context.Context) *HealthCheck {
	start := time.Now()

	err := amm.db.PingContext(ctx)
	responseTime := time.Since(start)

	if err != nil {
		return &HealthCheck{
			Name:         "database",
			Status:       "unhealthy",
			Message:      fmt.Sprintf("Database ping failed: %v", err),
			LastCheck:    time.Now(),
			ResponseTime: responseTime,
		}
	}

	return &HealthCheck{
		Name:         "database",
		Status:       "healthy",
		Message:      "Database connection is healthy",
		LastCheck:    time.Now(),
		ResponseTime: responseTime,
	}
}

// checkCache performs cache health check
func (amm *AdvancedMonitoringManager) checkCache(ctx context.Context) *HealthCheck {
	start := time.Now()
	status := "healthy"
	message := "Cache system is healthy"

	if amm.hotCache != nil {
		stats := amm.hotCache.GetStats()
		if stats.HitRate < 0.5 {
			status = "degraded"
			message = fmt.Sprintf("Cache hit rate is low: %.2f%%", stats.HitRate*100)
		}
	}

	return &HealthCheck{
		Name:         "cache",
		Status:       status,
		Message:      message,
		LastCheck:    time.Now(),
		ResponseTime: time.Since(start),
	}
}

// checkSharding performs sharding health check
func (amm *AdvancedMonitoringManager) checkSharding(ctx context.Context) *HealthCheck {
	start := time.Now()
	status := "healthy"
	message := "Sharding system is healthy"

	if amm.shardManager != nil {
		metrics := amm.shardManager.GetMetrics()
		if loadFactor, ok := metrics["max_load_factor"].(float64); ok && loadFactor > 2.0 {
			status = "degraded"
			message = fmt.Sprintf("Shard load imbalance detected: %.2f", loadFactor)
		}
	}

	return &HealthCheck{
		Name:         "sharding",
		Status:       status,
		Message:      message,
		LastCheck:    time.Now(),
		ResponseTime: time.Since(start),
	}
}

// checkLifecycle performs lifecycle management health check
func (amm *AdvancedMonitoringManager) checkLifecycle(ctx context.Context) *HealthCheck {
	start := time.Now()
	status := "healthy"
	message := "Data lifecycle is healthy"

	if amm.lifecycleManager != nil {
		metrics := amm.lifecycleManager.GetMetrics()
		if failed, ok := metrics["jobs_failed"].(int); ok && failed > 0 {
			status = "degraded"
			message = fmt.Sprintf("Failed lifecycle jobs detected: %d", failed)
		}
	}

	return &HealthCheck{
		Name:         "lifecycle",
		Status:       status,
		Message:      message,
		LastCheck:    time.Now(),
		ResponseTime: time.Since(start),
	}
}

// checkMigrations performs migration system health check
func (amm *AdvancedMonitoringManager) checkMigrations(ctx context.Context) *HealthCheck {
	start := time.Now()
	status := "healthy"
	message := "Migration system is healthy"

	if amm.migrationManager != nil {
		metrics := amm.migrationManager.GetMetrics()
		if running, ok := metrics["running_migrations"].(int); ok && running > 0 {
			status = "degraded"
			message = fmt.Sprintf("Active migrations running: %d", running)
		}
		if failed, ok := metrics["failed_migrations"].(int); ok && failed > 0 {
			status = "unhealthy"
			message = fmt.Sprintf("Failed migrations detected: %d", failed)
		}
	}

	return &HealthCheck{
		Name:         "migrations",
		Status:       status,
		Message:      message,
		LastCheck:    time.Now(),
		ResponseTime: time.Since(start),
	}
}

// calculateSystemHealth calculates overall system health score (0-100)
func (amm *AdvancedMonitoringManager) calculateSystemHealth() float64 {
	amm.healthMu.RLock()
	defer amm.healthMu.RUnlock()

	if len(amm.healthChecks) == 0 {
		return 100.0
	}

	totalScore := 0.0
	for _, check := range amm.healthChecks {
		switch check.Status {
		case "healthy":
			totalScore += 100.0
		case "degraded":
			totalScore += 70.0
		case "unhealthy":
			totalScore += 0.0
		default:
			totalScore += 50.0 // unknown
		}
	}

	return totalScore / float64(len(amm.healthChecks))
}

// collectMetrics collects system metrics for analysis
func (amm *AdvancedMonitoringManager) collectMetrics(ctx context.Context) {
	snapshot := &MetricSnapshot{
		Timestamp: time.Now(),
		Labels:    make(map[string]string),
		Metadata:  make(map[string]interface{}),
	}

	// Collect metrics from various components
	if amm.dataManager != nil {
		// Metrics collection for dataManager not implemented or not available
	}

	if amm.hotCache != nil {
		stats := amm.hotCache.GetStats()
		snapshot.Metadata["hot_cache"] = stats
	}

	// Metrics collection for shardManager not implemented or not available

	// Store snapshot
	amm.mu.Lock()
	amm.metricSnapshots = append(amm.metricSnapshots, snapshot)

	// Cleanup old snapshots
	if len(amm.metricSnapshots) > amm.maxSnapshots {
		amm.metricSnapshots = amm.metricSnapshots[len(amm.metricSnapshots)-amm.maxSnapshots:]
	}
	amm.mu.Unlock()
}

// generatePerformanceReport generates a comprehensive performance report
func (amm *AdvancedMonitoringManager) generatePerformanceReport(ctx context.Context) {
	report := &PerformanceReport{
		GeneratedAt:     time.Now(),
		TimeRange:       time.Hour,
		MetricsSummary:  make(map[string]float64),
		AlertsSummary:   make(map[AlertSeverity]int),
		TopErrors:       make([]string, 0),
		Recommendations: make([]string, 0),
	}

	// Calculate performance metrics
	amm.calculatePerformanceMetrics(report)

	// Analyze alerts
	amm.analyzeAlerts(report)

	// Generate recommendations
	amm.generateRecommendations(report)

	// Store report
	amm.mu.Lock()
	amm.performanceHistory = append(amm.performanceHistory, report)

	// Cleanup old reports
	if len(amm.performanceHistory) > amm.maxReports {
		amm.performanceHistory = amm.performanceHistory[len(amm.performanceHistory)-amm.maxReports:]
	}
	amm.mu.Unlock()

	// Calculate performance score
	performanceScore := amm.calculatePerformanceScore(report)
	amm.performanceScore.Set(performanceScore)

	amm.logger.Info("Performance report generated",
		zap.Float64("performance_score", performanceScore),
		zap.Float64("operations_per_second", report.OperationsPerSecond),
		zap.Duration("average_latency", report.AverageLatency),
		zap.Float64("error_rate", report.ErrorRate))
}

// calculatePerformanceMetrics calculates key performance metrics
func (amm *AdvancedMonitoringManager) calculatePerformanceMetrics(report *PerformanceReport) {
	// Simulate performance calculations
	report.TotalOperations = 180000   // 180K operations in the last hour
	report.OperationsPerSecond = 50.0 // 50 ops/sec
	report.AverageLatency = time.Millisecond * 25
	report.P95Latency = time.Millisecond * 75
	report.P99Latency = time.Millisecond * 150
	report.ErrorRate = 0.01    // 1% error rate
	report.CacheHitRate = 0.85 // 85% cache hit rate
	report.DatabaseConnections = 45
	report.MemoryUsage = 0.75 // 75% memory usage
	report.CPUUsage = 0.60    // 60% CPU usage
	report.DiskUsage = 0.45   // 45% disk usage
}

// analyzeAlerts analyzes alert patterns
func (amm *AdvancedMonitoringManager) analyzeAlerts(report *PerformanceReport) {
	amm.alertMu.RLock()
	defer amm.alertMu.RUnlock()

	for _, alert := range amm.activeAlerts {
		if alert.StartTime.After(time.Now().Add(-report.TimeRange)) {
			report.AlertsSummary[alert.Severity]++
		}
	}
}

// generateRecommendations generates system optimization recommendations
func (amm *AdvancedMonitoringManager) generateRecommendations(report *PerformanceReport) {
	if report.ErrorRate > 0.02 {
		report.Recommendations = append(report.Recommendations,
			"High error rate detected. Consider reviewing error logs and improving error handling.")
	}

	if report.CacheHitRate < 0.8 {
		report.Recommendations = append(report.Recommendations,
			"Low cache hit rate. Consider optimizing cache strategies or increasing cache size.")
	}

	if report.P95Latency > time.Millisecond*100 {
		report.Recommendations = append(report.Recommendations,
			"High latency detected. Consider optimizing database queries or adding read replicas.")
	}

	if report.DatabaseConnections > 80 {
		report.Recommendations = append(report.Recommendations,
			"High database connection usage. Consider connection pooling optimization.")
	}
}

// calculatePerformanceScore calculates overall performance score (0-100)
func (amm *AdvancedMonitoringManager) calculatePerformanceScore(report *PerformanceReport) float64 {
	score := 100.0

	// Deduct points for issues
	if report.ErrorRate > 0.01 {
		score -= math.Min(50.0, report.ErrorRate*1000) // Up to 50 points for error rate
	}

	if report.CacheHitRate < 0.9 {
		score -= (0.9 - report.CacheHitRate) * 100 // Up to 90 points for cache hit rate
	}

	if report.P95Latency > time.Millisecond*50 {
		latencyPenalty := float64(report.P95Latency.Milliseconds()-50) / 10
		score -= math.Min(30.0, latencyPenalty) // Up to 30 points for latency
	}

	return math.Max(0.0, score)
}

// AddAlertRule adds a new alert rule
func (amm *AdvancedMonitoringManager) AddAlertRule(rule *AlertRule) error {
	if rule.ID == "" {
		return fmt.Errorf("alert rule ID is required")
	}

	rule.CreatedAt = time.Now()
	rule.UpdatedAt = time.Now()

	amm.mu.Lock()
	amm.alertRules[rule.ID] = rule
	amm.mu.Unlock()

	amm.logger.Info("Added alert rule",
		zap.String("id", rule.ID),
		zap.String("name", rule.Name),
		zap.String("severity", string(rule.Severity)))

	return nil
}

// GetAlerts returns current active alerts
func (amm *AdvancedMonitoringManager) GetAlerts() []*Alert {
	amm.alertMu.RLock()
	defer amm.alertMu.RUnlock()

	alerts := make([]*Alert, 0, len(amm.activeAlerts))
	for _, alert := range amm.activeAlerts {
		alerts = append(alerts, alert)
	}

	return alerts
}

// GetHealthStatus returns current system health status
func (amm *AdvancedMonitoringManager) GetHealthStatus() map[string]*HealthCheck {
	amm.healthMu.RLock()
	defer amm.healthMu.RUnlock()

	health := make(map[string]*HealthCheck)
	for name, check := range amm.healthChecks {
		health[name] = check
	}

	return health
}

// GetLatestPerformanceReport returns the latest performance report
func (amm *AdvancedMonitoringManager) GetLatestPerformanceReport() *PerformanceReport {
	amm.mu.RLock()
	defer amm.mu.RUnlock()

	if len(amm.performanceHistory) == 0 {
		return nil
	}

	return amm.performanceHistory[len(amm.performanceHistory)-1]
}

// GetMetrics returns monitoring manager metrics
func (amm *AdvancedMonitoringManager) GetMetrics() map[string]interface{} {
	amm.mu.RLock()
	amm.alertMu.RLock()
	amm.healthMu.RLock()
	defer amm.mu.RUnlock()
	defer amm.alertMu.RUnlock()
	defer amm.healthMu.RUnlock()

	firingAlerts := 0
	for _, alert := range amm.activeAlerts {
		if alert.Status == "firing" {
			firingAlerts++
		}
	}

	healthyChecks := 0
	for _, check := range amm.healthChecks {
		if check.Status == "healthy" {
			healthyChecks++
		}
	}

	return map[string]interface{}{
		"alert_rules_count":    len(amm.alertRules),
		"active_alerts_count":  len(amm.activeAlerts),
		"firing_alerts_count":  firingAlerts,
		"health_checks_count":  len(amm.healthChecks),
		"healthy_checks_count": healthyChecks,
		"metric_snapshots":     len(amm.metricSnapshots),
		"performance_reports":  len(amm.performanceHistory),
		"is_running":           amm.isRunning,
	}
}
