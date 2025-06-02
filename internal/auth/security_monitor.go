package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// SecurityEventType represents different types of security events
type SecurityEventType string

const (
	EventTypeAuthenticationFailure   SecurityEventType = "authentication_failure"
	EventTypeRateLimitExceeded       SecurityEventType = "rate_limit_exceeded"
	EventTypeDDoSDetected            SecurityEventType = "ddos_detected"
	EventTypeSuspiciousIP            SecurityEventType = "suspicious_ip"
	EventTypeMaliciousIP             SecurityEventType = "malicious_ip"
	EventTypeGeoBlocked              SecurityEventType = "geo_blocked"
	EventTypeVPNDetected             SecurityEventType = "vpn_detected"
	EventTypeAPIKeyCompromised       SecurityEventType = "api_key_compromised"
	EventTypeUnusualTraffic          SecurityEventType = "unusual_traffic"
	EventTypeSecurityPolicyViolation SecurityEventType = "security_policy_violation"
	EventTypePerformanceDegradation  SecurityEventType = "performance_degradation"
)

// SecurityEventSeverity represents the severity of security events
type SecurityEventSeverity string

const (
	SecEventSeverityLow      SecurityEventSeverity = "low"
	SecEventSeverityMedium   SecurityEventSeverity = "medium"
	SecEventSeverityHigh     SecurityEventSeverity = "high"
	SecEventSeverityCritical SecurityEventSeverity = "critical"
)

// SecurityEvent represents a security event
type SecurityEvent struct {
	ID             string                 `json:"id"`
	Type           SecurityEventType      `json:"type"`
	Severity       SecurityEventSeverity  `json:"severity"`
	Timestamp      time.Time              `json:"timestamp"`
	SourceIP       string                 `json:"source_ip,omitempty"`
	UserID         string                 `json:"user_id,omitempty"`
	Endpoint       string                 `json:"endpoint,omitempty"`
	UserAgent      string                 `json:"user_agent,omitempty"`
	Message        string                 `json:"message"`
	Details        map[string]interface{} `json:"details,omitempty"`
	CountryCode    string                 `json:"country_code,omitempty"`
	ASN            string                 `json:"asn,omitempty"`
	ResponseCode   int                    `json:"response_code,omitempty"`
	ProcessingTime time.Duration          `json:"processing_time,omitempty"`
}

// SecurityMetrics represents security-related metrics
type SecurityMetrics struct {
	TotalEvents         int64            `json:"total_events"`
	EventsBySeverity    map[string]int64 `json:"events_by_severity"`
	EventsByType        map[string]int64 `json:"events_by_type"`
	EventsByHour        map[string]int64 `json:"events_by_hour"`
	TopSourceIPs        map[string]int64 `json:"top_source_ips"`
	TopCountries        map[string]int64 `json:"top_countries"`
	BlockedRequests     int64            `json:"blocked_requests"`
	RateLimitedRequests int64            `json:"rate_limited_requests"`
	PerformanceAlerts   int64            `json:"performance_alerts"`
	LastUpdated         time.Time        `json:"last_updated"`
}

// AlertRule represents an alerting rule
type AlertRule struct {
	ID                   string                 `json:"id"`
	Name                 string                 `json:"name"`
	EventType            SecurityEventType      `json:"event_type"`
	Severity             SecurityEventSeverity  `json:"severity"`
	Threshold            int                    `json:"threshold"`
	TimeWindow           time.Duration          `json:"time_window"`
	Enabled              bool                   `json:"enabled"`
	NotificationChannels []string               `json:"notification_channels"`
	Conditions           map[string]interface{} `json:"conditions,omitempty"`
}

// SecurityMonitor handles security event monitoring and alerting
type SecurityMonitor struct {
	logger      *zap.Logger
	events      chan *SecurityEvent
	metrics     *SecurityMetrics
	alertRules  []*AlertRule
	subscribers []SecurityEventSubscriber
	mu          sync.RWMutex
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
}

// SecurityEventSubscriber defines an interface for security event subscribers
type SecurityEventSubscriber interface {
	HandleSecurityEvent(event *SecurityEvent) error
}

// NewSecurityMonitor creates a new security monitor
func NewSecurityMonitor(logger *zap.Logger) *SecurityMonitor {
	ctx, cancel := context.WithCancel(context.Background())

	return &SecurityMonitor{
		logger: logger,
		events: make(chan *SecurityEvent, 1000),
		metrics: &SecurityMetrics{
			EventsBySeverity: make(map[string]int64),
			EventsByType:     make(map[string]int64),
			EventsByHour:     make(map[string]int64),
			TopSourceIPs:     make(map[string]int64),
			TopCountries:     make(map[string]int64),
		},
		alertRules:  defaultAlertRules(),
		subscribers: make([]SecurityEventSubscriber, 0),
		ctx:         ctx,
		cancel:      cancel,
	}
}

// Start starts the security monitor
func (sm *SecurityMonitor) Start() {
	sm.wg.Add(2)
	go sm.eventProcessor()
	go sm.metricsUpdater()

	sm.logger.Info("Security monitor started")
}

// Stop stops the security monitor
func (sm *SecurityMonitor) Stop() {
	sm.cancel()
	close(sm.events)
	sm.wg.Wait()
	sm.logger.Info("Security monitor stopped")
}

// LogEvent logs a security event
func (sm *SecurityMonitor) LogEvent(event *SecurityEvent) {
	if event.ID == "" {
		event.ID = generateEventID()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	select {
	case sm.events <- event:
		// Event queued successfully
	default:
		sm.logger.Warn("Security event queue full, dropping event",
			zap.String("event_type", string(event.Type)),
			zap.String("severity", string(event.Severity)),
		)
	}
}

// LogAuthenticationFailure logs an authentication failure event
func (sm *SecurityMonitor) LogAuthenticationFailure(sourceIP, userID, reason string) {
	sm.LogEvent(&SecurityEvent{
		Type:     EventTypeAuthenticationFailure,
		Severity: SecEventSeverityMedium,
		SourceIP: sourceIP,
		UserID:   userID,
		Message:  fmt.Sprintf("Authentication failed: %s", reason),
		Details: map[string]interface{}{
			"reason": reason,
		},
	})
}

// LogRateLimitExceeded logs a rate limit exceeded event
func (sm *SecurityMonitor) LogRateLimitExceeded(sourceIP, endpoint string, limit int) {
	sm.LogEvent(&SecurityEvent{
		Type:     EventTypeRateLimitExceeded,
		Severity: SecEventSeverityMedium,
		SourceIP: sourceIP,
		Endpoint: endpoint,
		Message:  fmt.Sprintf("Rate limit exceeded for endpoint %s", endpoint),
		Details: map[string]interface{}{
			"limit": limit,
		},
	})
}

// LogDDoSDetected logs a DDoS attack detection event
func (sm *SecurityMonitor) LogDDoSDetected(sourceIP string, requestCount int, timeWindow time.Duration) {
	sm.LogEvent(&SecurityEvent{
		Type:     EventTypeDDoSDetected,
		Severity: SecEventSeverityHigh,
		SourceIP: sourceIP,
		Message:  fmt.Sprintf("DDoS attack detected from %s", sourceIP),
		Details: map[string]interface{}{
			"request_count": requestCount,
			"time_window":   timeWindow.String(),
		},
	})
}

// LogSuspiciousIP logs a suspicious IP event
func (sm *SecurityMonitor) LogSuspiciousIP(sourceIP, reason string, score float64) {
	severity := SecEventSeverityMedium
	if score >= 9.0 {
		severity = SecEventSeverityHigh
	}

	sm.LogEvent(&SecurityEvent{
		Type:     EventTypeSuspiciousIP,
		Severity: severity,
		SourceIP: sourceIP,
		Message:  fmt.Sprintf("Suspicious IP detected: %s", reason),
		Details: map[string]interface{}{
			"reason": reason,
			"score":  score,
		},
	})
}

// LogGeoBlocked logs a geo-blocked request event
func (sm *SecurityMonitor) LogGeoBlocked(sourceIP, countryCode, reason string) {
	sm.LogEvent(&SecurityEvent{
		Type:        EventTypeGeoBlocked,
		Severity:    SecEventSeverityMedium,
		SourceIP:    sourceIP,
		CountryCode: countryCode,
		Message:     fmt.Sprintf("Request blocked from %s: %s", countryCode, reason),
		Details: map[string]interface{}{
			"reason": reason,
		},
	})
}

// LogVPNDetected logs a VPN detection event
func (sm *SecurityMonitor) LogVPNDetected(sourceIP, vpnProvider string, confidence float64) {
	severity := SecEventSeverityLow
	if confidence >= 0.8 {
		severity = SecEventSeverityMedium
	}

	sm.LogEvent(&SecurityEvent{
		Type:     EventTypeVPNDetected,
		Severity: severity,
		SourceIP: sourceIP,
		Message:  fmt.Sprintf("VPN detected from %s", sourceIP),
		Details: map[string]interface{}{
			"vpn_provider": vpnProvider,
			"confidence":   confidence,
		},
	})
}

// LogPerformanceDegradation logs a performance degradation event
func (sm *SecurityMonitor) LogPerformanceDegradation(endpoint string, latency time.Duration, threshold time.Duration) {
	severity := SecEventSeverityMedium
	if latency > threshold*2 {
		severity = SecEventSeverityHigh
	}

	sm.LogEvent(&SecurityEvent{
		Type:           EventTypePerformanceDegradation,
		Severity:       severity,
		Endpoint:       endpoint,
		ProcessingTime: latency,
		Message:        fmt.Sprintf("Performance degradation detected on %s", endpoint),
		Details: map[string]interface{}{
			"latency":   latency.String(),
			"threshold": threshold.String(),
		},
	})
}

// Subscribe adds a security event subscriber
func (sm *SecurityMonitor) Subscribe(subscriber SecurityEventSubscriber) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.subscribers = append(sm.subscribers, subscriber)
}

// GetMetrics returns current security metrics
func (sm *SecurityMonitor) GetMetrics() *SecurityMetrics {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	// Create a copy to avoid data races
	metricsCopy := &SecurityMetrics{
		TotalEvents:         sm.metrics.TotalEvents,
		EventsBySeverity:    make(map[string]int64),
		EventsByType:        make(map[string]int64),
		EventsByHour:        make(map[string]int64),
		TopSourceIPs:        make(map[string]int64),
		TopCountries:        make(map[string]int64),
		BlockedRequests:     sm.metrics.BlockedRequests,
		RateLimitedRequests: sm.metrics.RateLimitedRequests,
		PerformanceAlerts:   sm.metrics.PerformanceAlerts,
		LastUpdated:         sm.metrics.LastUpdated,
	}

	for k, v := range sm.metrics.EventsBySeverity {
		metricsCopy.EventsBySeverity[k] = v
	}
	for k, v := range sm.metrics.EventsByType {
		metricsCopy.EventsByType[k] = v
	}
	for k, v := range sm.metrics.EventsByHour {
		metricsCopy.EventsByHour[k] = v
	}
	for k, v := range sm.metrics.TopSourceIPs {
		metricsCopy.TopSourceIPs[k] = v
	}
	for k, v := range sm.metrics.TopCountries {
		metricsCopy.TopCountries[k] = v
	}

	return metricsCopy
}

// Convert AttackSeverity to SecurityEventSeverity
func attackSeverityToSecurityEventSeverity(severity AttackSeverity) SecurityEventSeverity {
	switch severity {
	case SeverityLow:
		return SecEventSeverityLow
	case SeverityMedium:
		return SecEventSeverityMedium
	case SeverityHigh:
		return SecEventSeverityHigh
	case SeverityCritical:
		return SecEventSeverityCritical
	default:
		return SecEventSeverityLow
	}
}

// eventProcessor processes security events
func (sm *SecurityMonitor) eventProcessor() {
	defer sm.wg.Done()

	for {
		select {
		case event, ok := <-sm.events:
			if !ok {
				return
			}
			sm.processEvent(event)
		case <-sm.ctx.Done():
			return
		}
	}
}

// processEvent processes a single security event
func (sm *SecurityMonitor) processEvent(event *SecurityEvent) {
	// Update metrics
	sm.updateMetrics(event)

	// Log the event
	sm.logEvent(event)

	// Check alert rules
	sm.checkAlertRules(event)

	// Notify subscribers
	sm.notifySubscribers(event)
}

// updateMetrics updates security metrics
func (sm *SecurityMonitor) updateMetrics(event *SecurityEvent) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.metrics.TotalEvents++
	sm.metrics.EventsBySeverity[string(event.Severity)]++
	sm.metrics.EventsByType[string(event.Type)]++

	hour := event.Timestamp.Format("2006-01-02-15")
	sm.metrics.EventsByHour[hour]++

	if event.SourceIP != "" {
		sm.metrics.TopSourceIPs[event.SourceIP]++
	}

	if event.CountryCode != "" {
		sm.metrics.TopCountries[event.CountryCode]++
	}

	// Update specific counters
	switch event.Type {
	case EventTypeRateLimitExceeded:
		sm.metrics.RateLimitedRequests++
	case EventTypeDDoSDetected, EventTypeGeoBlocked, EventTypeSuspiciousIP:
		sm.metrics.BlockedRequests++
	case EventTypePerformanceDegradation:
		sm.metrics.PerformanceAlerts++
	}

	sm.metrics.LastUpdated = time.Now()
}

// logEvent logs the security event
func (sm *SecurityMonitor) logEvent(event *SecurityEvent) {
	fields := []zap.Field{
		zap.String("event_id", event.ID),
		zap.String("event_type", string(event.Type)),
		zap.String("severity", string(event.Severity)),
		zap.Time("timestamp", event.Timestamp),
		zap.String("message", event.Message),
	}

	if event.SourceIP != "" {
		fields = append(fields, zap.String("source_ip", event.SourceIP))
	}
	if event.UserID != "" {
		fields = append(fields, zap.String("user_id", event.UserID))
	}
	if event.Endpoint != "" {
		fields = append(fields, zap.String("endpoint", event.Endpoint))
	}
	if event.CountryCode != "" {
		fields = append(fields, zap.String("country_code", event.CountryCode))
	}
	if event.ProcessingTime > 0 {
		fields = append(fields, zap.Duration("processing_time", event.ProcessingTime))
	}

	if len(event.Details) > 0 {
		detailsJSON, _ := json.Marshal(event.Details)
		fields = append(fields, zap.String("details", string(detailsJSON)))
	}
	switch event.Severity {
	case SecEventSeverityCritical:
		sm.logger.Error("Security event", fields...)
	case SecEventSeverityHigh:
		sm.logger.Warn("Security event", fields...)
	case SecEventSeverityMedium:
		sm.logger.Info("Security event", fields...)
	case SecEventSeverityLow:
		sm.logger.Debug("Security event", fields...)
	}
}

// checkAlertRules checks if any alert rules are triggered
func (sm *SecurityMonitor) checkAlertRules(event *SecurityEvent) {
	for _, rule := range sm.alertRules {
		if rule.Enabled && sm.ruleMatches(rule, event) {
			sm.triggerAlert(rule, event)
		}
	}
}

// ruleMatches checks if an alert rule matches an event
func (sm *SecurityMonitor) ruleMatches(rule *AlertRule, event *SecurityEvent) bool {
	return rule.EventType == event.Type &&
		(rule.Severity == "" || rule.Severity == event.Severity)
}

// triggerAlert triggers an alert
func (sm *SecurityMonitor) triggerAlert(rule *AlertRule, event *SecurityEvent) {
	sm.logger.Warn("Security alert triggered",
		zap.String("rule_id", rule.ID),
		zap.String("rule_name", rule.Name),
		zap.String("event_id", event.ID),
		zap.String("event_type", string(event.Type)),
		zap.String("severity", string(event.Severity)),
	)

	// Here you would implement actual alerting logic
	// (e.g., send to Slack, PagerDuty, email, etc.)
}

// notifySubscribers notifies all subscribers of the event
func (sm *SecurityMonitor) notifySubscribers(event *SecurityEvent) {
	sm.mu.RLock()
	subscribers := make([]SecurityEventSubscriber, len(sm.subscribers))
	copy(subscribers, sm.subscribers)
	sm.mu.RUnlock()

	for _, subscriber := range subscribers {
		if err := subscriber.HandleSecurityEvent(event); err != nil {
			sm.logger.Error("Failed to notify subscriber",
				zap.Error(err),
				zap.String("event_id", event.ID),
			)
		}
	}
}

// metricsUpdater periodically updates metrics
func (sm *SecurityMonitor) metricsUpdater() {
	defer sm.wg.Done()

	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			sm.cleanupOldMetrics()
		case <-sm.ctx.Done():
			return
		}
	}
}

// cleanupOldMetrics removes old metrics data
func (sm *SecurityMonitor) cleanupOldMetrics() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Keep only last 24 hours of hourly data
	cutoff := time.Now().Add(-24 * time.Hour).Format("2006-01-02-15")

	for hour := range sm.metrics.EventsByHour {
		if hour < cutoff {
			delete(sm.metrics.EventsByHour, hour)
		}
	}

	// Keep only top 100 IPs
	if len(sm.metrics.TopSourceIPs) > 100 {
		// This is a simplified cleanup - in production you'd want more sophisticated logic
		sm.metrics.TopSourceIPs = make(map[string]int64)
	}
}

// defaultAlertRules returns default alert rules
func defaultAlertRules() []*AlertRule {
	return []*AlertRule{
		{
			ID:                   "high_rate_limit_exceeded",
			Name:                 "High Rate Limit Exceeded",
			EventType:            EventTypeRateLimitExceeded,
			Severity:             SecEventSeverityHigh,
			Threshold:            100,
			TimeWindow:           time.Minute,
			Enabled:              true,
			NotificationChannels: []string{"security-team"},
		},
		{
			ID:                   "ddos_detected",
			Name:                 "DDoS Attack Detected",
			EventType:            EventTypeDDoSDetected,
			Severity:             SecEventSeverityHigh,
			Threshold:            1,
			TimeWindow:           time.Minute,
			Enabled:              true,
			NotificationChannels: []string{"security-team", "ops-team"},
		},
		{
			ID:                   "critical_performance_degradation",
			Name:                 "Critical Performance Degradation",
			EventType:            EventTypePerformanceDegradation,
			Severity:             SecEventSeverityCritical,
			Threshold:            10,
			TimeWindow:           5 * time.Minute,
			Enabled:              true,
			NotificationChannels: []string{"engineering-team", "ops-team"},
		},
	}
}

// generateEventID generates a unique event ID
func generateEventID() string {
	return fmt.Sprintf("sec_%d", time.Now().UnixNano())
}
