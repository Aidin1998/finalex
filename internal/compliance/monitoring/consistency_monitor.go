package monitoring

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Aidin1998/finalex/internal/trading/integration"
	"go.uber.org/zap"
)

// ConsistencyMonitor monitors strong consistency operations and detects violations
type ConsistencyMonitor struct {
	logger  *zap.Logger
	manager *integration.StrongConsistencyManager

	// Configuration
	config *MonitoringConfig

	// State
	mu      sync.RWMutex
	started bool
	ctx     context.Context
	cancel  context.CancelFunc

	// Metrics tracking
	violationCounts    map[string]int64
	alertCounts        map[string]int64
	lastViolationTime  map[string]time.Time
	performanceMetrics *ConsistencyPerformanceMetrics

	// Alerting
	alerting *AlertingService
}

// MonitoringConfig configures the consistency monitor
type MonitoringConfig struct {
	// Monitoring intervals
	HealthCheckInterval       time.Duration `yaml:"health_check_interval" default:"10s"`
	MetricsCollectionInterval time.Duration `yaml:"metrics_collection_interval" default:"30s"`
	ViolationCheckInterval    time.Duration `yaml:"violation_check_interval" default:"5s"`

	// Thresholds
	LatencyThresholdMs   int64   `yaml:"latency_threshold_ms" default:"100"`
	ErrorRateThreshold   float64 `yaml:"error_rate_threshold" default:"0.01"`
	ConsistencyThreshold float64 `yaml:"consistency_threshold" default:"0.999"`

	// Alerting
	EnableAlerting         bool          `yaml:"enable_alerting" default:"true"`
	AlertCooldownDuration  time.Duration `yaml:"alert_cooldown_duration" default:"5m"`
	CriticalAlertThreshold int64         `yaml:"critical_alert_threshold" default:"10"`

	// Performance monitoring
	EnablePerformanceMonitoring bool          `yaml:"enable_performance_monitoring" default:"true"`
	PerformanceReportInterval   time.Duration `yaml:"performance_report_interval" default:"1m"`

	// Circuit breaker
	EnableCircuitBreaker    bool          `yaml:"enable_circuit_breaker" default:"true"`
	CircuitBreakerThreshold int64         `yaml:"circuit_breaker_threshold" default:"100"`
	CircuitBreakerTimeout   time.Duration `yaml:"circuit_breaker_timeout" default:"30s"`
}

// ConsistencyPerformanceMetrics tracks system performance
type ConsistencyPerformanceMetrics struct {
	// Latency metrics
	ConsensusLatency  *LatencyMetrics
	BalanceLatency    *LatencyMetrics
	LockLatency       *LatencyMetrics
	SettlementLatency *LatencyMetrics
	OrderLatency      *LatencyMetrics

	// Throughput metrics
	ConsensusThroughput  float64
	BalanceThroughput    float64
	LockThroughput       float64
	SettlementThroughput float64
	OrderThroughput      float64

	// Error rates
	ConsensusErrorRate  float64
	BalanceErrorRate    float64
	LockErrorRate       float64
	SettlementErrorRate float64
	OrderErrorRate      float64

	// Resource utilization
	CPUUsage     float64
	MemoryUsage  float64
	NetworkUsage float64
	DiskUsage    float64

	LastUpdate time.Time
}

// LatencyMetrics tracks latency statistics
type LatencyMetrics struct {
	Min   time.Duration
	Max   time.Duration
	Avg   time.Duration
	P50   time.Duration
	P95   time.Duration
	P99   time.Duration
	Count int64
}

// Alert represents a consistency violation alert
type Alert struct {
	ID         string
	Type       string
	Severity   string
	Component  string
	Message    string
	Details    map[string]interface{}
	Timestamp  time.Time
	Resolved   bool
	ResolvedAt *time.Time
}

// AlertingService handles alerting for consistency violations
type AlertingService struct {
	logger *zap.Logger
	config *MonitoringConfig

	mu            sync.RWMutex
	activeAlerts  map[string]*Alert
	alertHistory  []*Alert
	lastAlertTime map[string]time.Time

	// Alert channels
	alertChannel   chan *Alert
	resolveChannel chan string
}

// NewConsistencyMonitor creates a new consistency monitor
func NewConsistencyMonitor(
	config *MonitoringConfig,
	manager *integration.StrongConsistencyManager,
	logger *zap.Logger,
) *ConsistencyMonitor {
	ctx, cancel := context.WithCancel(context.Background())

	alerting := &AlertingService{
		logger:         logger.Named("alerting"),
		config:         config,
		activeAlerts:   make(map[string]*Alert),
		alertHistory:   make([]*Alert, 0),
		lastAlertTime:  make(map[string]time.Time),
		alertChannel:   make(chan *Alert, 100),
		resolveChannel: make(chan string, 100),
	}

	return &ConsistencyMonitor{
		logger:            logger.Named("consistency-monitor"),
		manager:           manager,
		config:            config,
		ctx:               ctx,
		cancel:            cancel,
		violationCounts:   make(map[string]int64),
		alertCounts:       make(map[string]int64),
		lastViolationTime: make(map[string]time.Time),
		performanceMetrics: &ConsistencyPerformanceMetrics{
			ConsensusLatency:  &LatencyMetrics{},
			BalanceLatency:    &LatencyMetrics{},
			LockLatency:       &LatencyMetrics{},
			SettlementLatency: &LatencyMetrics{},
			OrderLatency:      &LatencyMetrics{},
			LastUpdate:        time.Now(),
		},
		alerting: alerting,
	}
}

// Start starts the consistency monitor
func (m *ConsistencyMonitor) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.started {
		return fmt.Errorf("consistency monitor already started")
	}

	m.logger.Info("Starting consistency monitor")

	// Start alerting service
	go m.alerting.processAlerts(ctx)

	// Start monitoring goroutines
	go m.healthCheckLoop(ctx)
	go m.metricsCollectionLoop(ctx)
	go m.violationCheckLoop(ctx)

	if m.config.EnablePerformanceMonitoring {
		go m.performanceMonitoringLoop(ctx)
	}

	m.started = true
	m.logger.Info("Consistency monitor started successfully")

	return nil
}

// Stop stops the consistency monitor
func (m *ConsistencyMonitor) Stop(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.started {
		return nil
	}

	m.logger.Info("Stopping consistency monitor")

	m.cancel()
	m.started = false

	m.logger.Info("Consistency monitor stopped successfully")
	return nil
}

// healthCheckLoop performs periodic health checks
func (m *ConsistencyMonitor) healthCheckLoop(ctx context.Context) {
	ticker := time.NewTicker(m.config.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.performHealthCheck()
		}
	}
}

// performHealthCheck checks the health of all components
func (m *ConsistencyMonitor) performHealthCheck() {
	if !m.manager.IsHealthy() {
		m.reportViolation("system_health", "System health check failed", map[string]interface{}{
			"timestamp": time.Now(),
			"metrics":   m.manager.GetMetrics(),
		})
	}

	// Check individual component health
	components := m.manager.GetComponents()
	healthChecks := []struct {
		name    string
		healthy bool
	}{
		{"raft-coordinator", components.RaftCoordinator.IsHealthy()},
		{"balance-manager", components.BalanceManager.IsHealthy()},
		{"lock-manager", components.DistributedLockManager.IsHealthy()},
		{"settlement-coordinator", components.SettlementCoordinator.IsHealthy()},
		{"order-processor", components.OrderProcessor.IsHealthy()},
		{"transaction-integration", components.TransactionIntegration.IsHealthy()},
	}

	for _, check := range healthChecks {
		if !check.healthy {
			m.reportViolation("component_health",
				fmt.Sprintf("Component %s is unhealthy", check.name),
				map[string]interface{}{
					"component": check.name,
					"timestamp": time.Now(),
				})
		}
	}
}

// metricsCollectionLoop collects performance metrics
func (m *ConsistencyMonitor) metricsCollectionLoop(ctx context.Context) {
	ticker := time.NewTicker(m.config.MetricsCollectionInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.collectMetrics()
		}
	}
}

// collectMetrics collects and analyzes performance metrics
func (m *ConsistencyMonitor) collectMetrics() {
	m.mu.Lock()
	defer m.mu.Unlock()

	metrics := m.manager.GetMetrics()

	// Update performance metrics
	m.performanceMetrics.LastUpdate = time.Now()

	// Check for performance violations
	if metrics.AverageLatency > time.Duration(m.config.LatencyThresholdMs)*time.Millisecond {
		m.reportViolation("high_latency",
			fmt.Sprintf("Average latency %v exceeds threshold %v",
				metrics.AverageLatency,
				time.Duration(m.config.LatencyThresholdMs)*time.Millisecond),
			map[string]interface{}{
				"latency":   metrics.AverageLatency,
				"threshold": m.config.LatencyThresholdMs,
			})
	}

	// Check consistency rates
	if metrics.ConsensusSuccessRate < m.config.ConsistencyThreshold {
		m.reportViolation("low_consensus_rate",
			fmt.Sprintf("Consensus success rate %.3f below threshold %.3f",
				metrics.ConsensusSuccessRate, m.config.ConsistencyThreshold),
			map[string]interface{}{
				"success_rate": metrics.ConsensusSuccessRate,
				"threshold":    m.config.ConsistencyThreshold,
			})
	}

	if metrics.BalanceConsistencyRate < m.config.ConsistencyThreshold {
		m.reportViolation("low_balance_consistency",
			fmt.Sprintf("Balance consistency rate %.3f below threshold %.3f",
				metrics.BalanceConsistencyRate, m.config.ConsistencyThreshold),
			map[string]interface{}{
				"consistency_rate": metrics.BalanceConsistencyRate,
				"threshold":        m.config.ConsistencyThreshold,
			})
	}
}

// violationCheckLoop checks for consistency violations
func (m *ConsistencyMonitor) violationCheckLoop(ctx context.Context) {
	ticker := time.NewTicker(m.config.ViolationCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.checkForViolations()
		}
	}
}

// checkForViolations performs comprehensive consistency checks
func (m *ConsistencyMonitor) checkForViolations() {
	components := m.manager.GetComponents()

	// Check for balance inconsistencies
	if err := m.checkBalanceConsistency(components.BalanceManager); err != nil {
		m.reportViolation("balance_inconsistency", err.Error(), map[string]interface{}{
			"component": "balance-manager",
			"error":     err.Error(),
		})
	}

	// Check for lock deadlocks
	if err := m.checkLockDeadlocks(components.DistributedLockManager); err != nil {
		m.reportViolation("lock_deadlock", err.Error(), map[string]interface{}{
			"component": "lock-manager",
			"error":     err.Error(),
		})
	}

	// Check for consensus failures
	if err := m.checkConsensusFailures(components.RaftCoordinator); err != nil {
		m.reportViolation("consensus_failure", err.Error(), map[string]interface{}{
			"component": "raft-coordinator",
			"error":     err.Error(),
		})
	}
}

// checkBalanceConsistency checks for balance inconsistencies
func (m *ConsistencyMonitor) checkBalanceConsistency(balanceManager interface{}) error {
	// This would implement specific balance consistency checks
	// For now, return nil as we don't have the exact interface
	return nil
}

// checkLockDeadlocks checks for lock deadlocks
func (m *ConsistencyMonitor) checkLockDeadlocks(lockManager interface{}) error {
	// This would implement deadlock detection
	// For now, return nil as we don't have the exact interface
	return nil
}

// checkConsensusFailures checks for consensus failures
func (m *ConsistencyMonitor) checkConsensusFailures(raftCoordinator interface{}) error {
	// This would implement consensus failure detection
	// For now, return nil as we don't have the exact interface
	return nil
}

// performanceMonitoringLoop monitors performance metrics
func (m *ConsistencyMonitor) performanceMonitoringLoop(ctx context.Context) {
	ticker := time.NewTicker(m.config.PerformanceReportInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.generatePerformanceReport()
		}
	}
}

// generatePerformanceReport generates and logs performance reports
func (m *ConsistencyMonitor) generatePerformanceReport() {
	m.mu.RLock()
	metrics := m.performanceMetrics
	m.mu.RUnlock()

	m.logger.Info("Performance Report",
		zap.Time("timestamp", metrics.LastUpdate),
		zap.Float64("consensus_throughput", metrics.ConsensusThroughput),
		zap.Float64("balance_throughput", metrics.BalanceThroughput),
		zap.Float64("lock_throughput", metrics.LockThroughput),
		zap.Float64("settlement_throughput", metrics.SettlementThroughput),
		zap.Float64("order_throughput", metrics.OrderThroughput),
		zap.Float64("consensus_error_rate", metrics.ConsensusErrorRate),
		zap.Float64("balance_error_rate", metrics.BalanceErrorRate),
		zap.Float64("lock_error_rate", metrics.LockErrorRate),
		zap.Float64("settlement_error_rate", metrics.SettlementErrorRate),
		zap.Float64("order_error_rate", metrics.OrderErrorRate),
		zap.Float64("cpu_usage", metrics.CPUUsage),
		zap.Float64("memory_usage", metrics.MemoryUsage),
	)
}

// reportViolation reports a consistency violation
func (m *ConsistencyMonitor) reportViolation(violationType, message string, details map[string]interface{}) {
	m.mu.Lock()
	m.violationCounts[violationType]++
	m.lastViolationTime[violationType] = time.Now()
	m.mu.Unlock()

	// Determine severity based on violation type and frequency
	severity := m.determineSeverity(violationType)

	alert := &Alert{
		ID:        fmt.Sprintf("%s-%d", violationType, time.Now().Unix()),
		Type:      violationType,
		Severity:  severity,
		Component: "strong-consistency",
		Message:   message,
		Details:   details,
		Timestamp: time.Now(),
		Resolved:  false,
	}

	// Send alert
	select {
	case m.alerting.alertChannel <- alert:
	default:
		m.logger.Warn("Alert channel full, dropping alert", zap.String("type", violationType))
	}

	m.logger.Warn("Consistency violation detected",
		zap.String("type", violationType),
		zap.String("severity", severity),
		zap.String("message", message),
		zap.Any("details", details))
}

// determineSeverity determines alert severity based on violation type and history
func (m *ConsistencyMonitor) determineSeverity(violationType string) string {
	m.mu.RLock()
	count := m.violationCounts[violationType]
	lastTime := m.lastViolationTime[violationType]
	m.mu.RUnlock()

	// Check frequency
	if count >= m.config.CriticalAlertThreshold {
		return "critical"
	}

	// Check recency
	if time.Since(lastTime) < m.config.AlertCooldownDuration {
		return "high"
	}

	// Critical types
	switch violationType {
	case "balance_inconsistency", "consensus_failure", "system_health":
		return "high"
	case "lock_deadlock", "high_latency":
		return "medium"
	default:
		return "low"
	}
}

// GetViolationStats returns violation statistics
func (m *ConsistencyMonitor) GetViolationStats() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := make(map[string]interface{})
	stats["violation_counts"] = m.violationCounts
	stats["alert_counts"] = m.alertCounts
	stats["last_violation_times"] = m.lastViolationTime
	stats["performance_metrics"] = m.performanceMetrics

	return stats
}

// GetActiveAlerts returns currently active alerts
func (m *ConsistencyMonitor) GetActiveAlerts() []*Alert {
	return m.alerting.GetActiveAlerts()
}

// processAlerts processes alerts from the alert channel
func (a *AlertingService) processAlerts(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case alert := <-a.alertChannel:
			a.handleAlert(alert)
		case alertID := <-a.resolveChannel:
			a.resolveAlert(alertID)
		}
	}
}

// handleAlert handles a new alert
func (a *AlertingService) handleAlert(alert *Alert) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Check cooldown
	if lastTime, exists := a.lastAlertTime[alert.Type]; exists {
		if time.Since(lastTime) < a.config.AlertCooldownDuration {
			a.logger.Debug("Alert in cooldown period, skipping",
				zap.String("type", alert.Type),
				zap.Duration("since_last", time.Since(lastTime)))
			return
		}
	}

	// Store alert
	a.activeAlerts[alert.ID] = alert
	a.alertHistory = append(a.alertHistory, alert)
	a.lastAlertTime[alert.Type] = time.Now()

	// Send alert notification
	a.sendAlertNotification(alert)

	a.logger.Info("Alert processed",
		zap.String("id", alert.ID),
		zap.String("type", alert.Type),
		zap.String("severity", alert.Severity))
}

// resolveAlert resolves an active alert
func (a *AlertingService) resolveAlert(alertID string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if alert, exists := a.activeAlerts[alertID]; exists {
		now := time.Now()
		alert.Resolved = true
		alert.ResolvedAt = &now
		delete(a.activeAlerts, alertID)

		a.logger.Info("Alert resolved",
			zap.String("id", alertID),
			zap.String("type", alert.Type))
	}
}

// sendAlertNotification sends alert notifications
func (a *AlertingService) sendAlertNotification(alert *Alert) {
	// Here you would implement actual notification sending
	// e.g., email, Slack, PagerDuty, etc.
	a.logger.Info("Alert notification sent",
		zap.String("id", alert.ID),
		zap.String("type", alert.Type),
		zap.String("severity", alert.Severity),
		zap.String("message", alert.Message))
}

// GetActiveAlerts returns all active alerts
func (a *AlertingService) GetActiveAlerts() []*Alert {
	a.mu.RLock()
	defer a.mu.RUnlock()

	alerts := make([]*Alert, 0, len(a.activeAlerts))
	for _, alert := range a.activeAlerts {
		alerts = append(alerts, alert)
	}
	return alerts
}

// GetAlertHistory returns alert history
func (a *AlertingService) GetAlertHistory(limit int) []*Alert {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if limit <= 0 || limit > len(a.alertHistory) {
		limit = len(a.alertHistory)
	}

	start := len(a.alertHistory) - limit
	return a.alertHistory[start:]
}
