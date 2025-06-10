// Package ratelimit provides comprehensive monitoring and observability
// for the rate limiting system with metrics, health checks, and alerting.
package ratelimit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

var (
	// Rate limiting operation metrics
	rateLimitOperations = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "orbit",
			Subsystem: "ratelimit",
			Name:      "operations_total",
			Help:      "Total number of rate limiting operations",
		},
		[]string{"operation", "strategy", "result"},
	)

	rateLimitLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "orbit",
			Subsystem: "ratelimit",
			Name:      "operation_duration_seconds",
			Help:      "Time spent on rate limiting operations",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0},
		},
		[]string{"operation", "strategy"},
	)

	// Redis backend metrics
	redisOperations = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "orbit",
			Subsystem: "ratelimit_redis",
			Name:      "operations_total",
			Help:      "Total number of Redis operations",
		},
		[]string{"command", "status"},
	)

	redisConnectionPool = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "orbit",
			Subsystem: "ratelimit_redis",
			Name:      "connection_pool_size",
			Help:      "Current Redis connection pool metrics",
		},
		[]string{"state"}, // "active", "idle", "total"
	)

	// Rate limit key metrics
	activeLimitKeys = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "orbit",
			Subsystem: "ratelimit",
			Name:      "active_keys_total",
			Help:      "Number of active rate limit keys",
		},
		[]string{"key_type", "algorithm"},
	)

	limitViolations = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "orbit",
			Subsystem: "ratelimit",
			Name:      "violations_total",
			Help:      "Total number of rate limit violations",
		},
		[]string{"key_type", "algorithm", "severity"},
	)

	// Circuit breaker metrics
	circuitBreakerState = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "orbit",
			Subsystem: "ratelimit",
			Name:      "circuit_breaker_state",
			Help:      "Circuit breaker state (0=closed, 1=open, 2=half-open)",
		},
		[]string{"strategy"},
	)
)

// Monitor provides comprehensive monitoring for rate limiting
type Monitor struct {
	logger         *zap.Logger
	metricsPort    int
	alertManager   *AlertManager
	healthCheckers []HealthChecker
	mu             sync.RWMutex
	started        bool
	server         *http.Server
}

// HealthChecker defines interface for health checking components
type HealthChecker interface {
	Name() string
	Check(ctx context.Context) error
}

// MonitorConfig holds monitoring configuration
type MonitorConfig struct {
	MetricsPort     int           `yaml:"metrics_port" json:"metrics_port"`
	HealthCheckPath string        `yaml:"health_check_path" json:"health_check_path"`
	MetricsPath     string        `yaml:"metrics_path" json:"metrics_path"`
	AlertManagerURL string        `yaml:"alert_manager_url" json:"alert_manager_url"`
	CheckInterval   time.Duration `yaml:"check_interval" json:"check_interval"`
	EnableProfiling bool          `yaml:"enable_profiling" json:"enable_profiling"`
}

// NewMonitor creates a new rate limiting monitor
func NewMonitor(config *MonitorConfig, logger *zap.Logger) *Monitor {
	alertManager := NewAlertManager(config.AlertManagerURL, logger)

	return &Monitor{
		logger:         logger,
		metricsPort:    config.MetricsPort,
		alertManager:   alertManager,
		healthCheckers: make([]HealthChecker, 0),
	}
}

// RegisterHealthChecker registers a health checker
func (m *Monitor) RegisterHealthChecker(checker HealthChecker) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.healthCheckers = append(m.healthCheckers, checker)
	m.logger.Info("registered health checker", zap.String("name", checker.Name()))
}

// Start starts the monitoring server
func (m *Monitor) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.started {
		return fmt.Errorf("monitor already started")
	}

	mux := http.NewServeMux()

	// Metrics endpoint
	mux.Handle("/metrics", promhttp.Handler())

	// Health check endpoint
	mux.HandleFunc("/health", m.healthCheckHandler)
	mux.HandleFunc("/health/ready", m.readinessHandler)
	mux.HandleFunc("/health/live", m.livenessHandler)

	// Rate limit specific endpoints
	mux.HandleFunc("/ratelimit/stats", m.statsHandler)
	mux.HandleFunc("/ratelimit/keys", m.keysHandler)

	m.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", m.metricsPort),
		Handler: mux,
	}

	go func() {
		m.logger.Info("starting monitoring server", zap.Int("port", m.metricsPort))
		if err := m.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			m.logger.Error("monitoring server error", zap.Error(err))
		}
	}()

	m.started = true
	return nil
}

// Stop stops the monitoring server
func (m *Monitor) Stop(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.started {
		return nil
	}

	m.logger.Info("stopping monitoring server")
	err := m.server.Shutdown(ctx)
	m.started = false
	return err
}

// healthCheckHandler handles comprehensive health checks
func (m *Monitor) healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	results := make(map[string]interface{})
	allHealthy := true

	for _, checker := range m.healthCheckers {
		err := checker.Check(ctx)
		status := "healthy"
		if err != nil {
			status = "unhealthy"
			allHealthy = false
		}

		results[checker.Name()] = map[string]interface{}{
			"status": status,
			"error":  err,
		}
	}

	results["overall"] = map[string]interface{}{
		"status":    map[bool]string{true: "healthy", false: "unhealthy"}[allHealthy],
		"timestamp": time.Now().UTC(),
		"checks":    len(m.healthCheckers),
	}

	w.Header().Set("Content-Type", "application/json")
	if !allHealthy {
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	if err := writeJSON(w, results); err != nil {
		m.logger.Error("failed to write health check response", zap.Error(err))
	}
}

// readinessHandler checks if the service is ready to serve traffic
func (m *Monitor) readinessHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// Check critical components only
	for _, checker := range m.healthCheckers {
		if err := checker.Check(ctx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			writeJSON(w, map[string]interface{}{
				"ready": false,
				"error": err.Error(),
			})
			return
		}
	}

	writeJSON(w, map[string]interface{}{
		"ready": true,
	})
}

// livenessHandler checks if the service is alive
func (m *Monitor) livenessHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]interface{}{
		"alive":     true,
		"timestamp": time.Now().UTC(),
	})
}

// statsHandler provides rate limiting statistics
func (m *Monitor) statsHandler(w http.ResponseWriter, r *http.Request) {
	// This would typically collect stats from the rate limiter
	stats := map[string]interface{}{
		"timestamp": time.Now().UTC(),
		"uptime":    time.Since(time.Now()), // Placeholder
		"version":   "1.0.0",
	}

	writeJSON(w, stats)
}

// keysHandler provides information about active rate limit keys
func (m *Monitor) keysHandler(w http.ResponseWriter, r *http.Request) {
	// This would typically show active keys from Redis
	keys := map[string]interface{}{
		"active_keys": 0, // Placeholder
		"timestamp":   time.Now().UTC(),
	}

	writeJSON(w, keys)
}

// writeJSON writes JSON response
func writeJSON(w http.ResponseWriter, data interface{}) error {
	w.Header().Set("Content-Type", "application/json")
	return fmt.Errorf("JSON encoding not implemented") // Placeholder
}

// RedisHealthChecker implements health checking for Redis
type RedisHealthChecker struct {
	name   string
	client *RedisClient
}

// NewRedisHealthChecker creates a new Redis health checker
func NewRedisHealthChecker(name string, client *RedisClient) *RedisHealthChecker {
	return &RedisHealthChecker{
		name:   name,
		client: client,
	}
}

// Name returns the health checker name
func (rhc *RedisHealthChecker) Name() string {
	return rhc.name
}

// Check performs Redis health check
func (rhc *RedisHealthChecker) Check(ctx context.Context) error {
	return rhc.client.HealthCheck(ctx)
}

// AlertManager handles rate limiting alerts
type AlertManager struct {
	url         string
	logger      *zap.Logger
	alerts      []Alert
	mu          sync.RWMutex
	config      *AlertConfig
	httpClient  *http.Client
	kafkaWriter *kafka.Writer
}

// Alert represents a rate limiting alert
type Alert struct {
	ID          string                 `json:"id"`
	Level       string                 `json:"level"` // "warning", "critical"
	Message     string                 `json:"message"`
	Component   string                 `json:"component"`
	Timestamp   time.Time              `json:"timestamp"`
	Labels      map[string]string      `json:"labels"`
	Annotations map[string]interface{} `json:"annotations"`
}

// NewAlertManager creates a new alert manager
func NewAlertManager(url string, logger *zap.Logger) *AlertManager {
	return NewAlertManagerWithConfig(url, logger, &AlertConfig{
		Enabled: true,
		Channels: []AlertChannelConfig{
			{Type: AlertChannelWebhook, Enabled: true, Config: map[string]interface{}{"url": url}},
		},
	})
}

// NewAlertManagerWithConfig creates a new alert manager with custom configuration
func NewAlertManagerWithConfig(url string, logger *zap.Logger, config *AlertConfig) *AlertManager {
	am := &AlertManager{
		url:        url,
		logger:     logger,
		alerts:     make([]Alert, 0),
		config:     config,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}

	// Initialize Kafka writer if Kafka channel is enabled
	for _, channel := range config.Channels {
		if channel.Type == AlertChannelKafka && channel.Enabled {
			if brokers, ok := channel.Config["brokers"].([]string); ok {
				if topic, ok := channel.Config["topic"].(string); ok {
					am.kafkaWriter = &kafka.Writer{
						Addr:         kafka.TCP(brokers...),
						Topic:        topic,
						Balancer:     &kafka.LeastBytes{},
						RequiredAcks: kafka.RequireOne,
						Async:        true,
					}
				}
			}
		}
	}

	return am
}

// SendAlert sends an alert
func (am *AlertManager) SendAlert(alert Alert) error {
	am.mu.Lock()
	defer am.mu.Unlock()

	alert.Timestamp = time.Now()
	am.alerts = append(am.alerts, alert)
	am.logger.Warn("rate limiting alert",
		zap.String("id", alert.ID),
		zap.String("level", alert.Level),
		zap.String("message", alert.Message),
		zap.String("component", alert.Component))

	// Send alert to external systems
	return am.sendToExternalSystems(alert)
}

// GetRecentAlerts returns recent alerts
func (am *AlertManager) GetRecentAlerts(since time.Duration) []Alert {
	am.mu.RLock()
	defer am.mu.RUnlock()

	cutoff := time.Now().Add(-since)
	recent := make([]Alert, 0)

	for _, alert := range am.alerts {
		if alert.Timestamp.After(cutoff) {
			recent = append(recent, alert)
		}
	}

	return recent
}

// AlertChannel represents different alert delivery channels
type AlertChannel string

const (
	AlertChannelWebhook    AlertChannel = "webhook"
	AlertChannelSlack      AlertChannel = "slack"
	AlertChannelPagerDuty  AlertChannel = "pagerduty"
	AlertChannelKafka      AlertChannel = "kafka"
	AlertChannelPrometheus AlertChannel = "prometheus"
)

// AlertConfig holds configuration for alert delivery
type AlertConfig struct {
	Enabled  bool                 `json:"enabled"`
	Channels []AlertChannelConfig `json:"channels"`
	Rules    []AlertRule          `json:"rules"`
}

// AlertChannelConfig holds configuration for a specific alert channel
type AlertChannelConfig struct {
	Type    AlertChannel           `json:"type"`
	Enabled bool                   `json:"enabled"`
	Config  map[string]interface{} `json:"config"`
}

// AlertRule defines when to send alerts
type AlertRule struct {
	Name       string         `json:"name"`
	Condition  string         `json:"condition"`
	Severity   string         `json:"severity"`
	Components []string       `json:"components"`
	Channels   []AlertChannel `json:"channels"`
}

// MetricsRecorder provides utilities for recording rate limiting metrics
type MetricsRecorder struct {
	logger *zap.Logger
}

// NewMetricsRecorder creates a new metrics recorder
func NewMetricsRecorder(logger *zap.Logger) *MetricsRecorder {
	return &MetricsRecorder{
		logger: logger,
	}
}

// RecordOperation records a rate limiting operation
func (mr *MetricsRecorder) RecordOperation(operation, strategy, result string, duration time.Duration) {
	rateLimitOperations.WithLabelValues(operation, strategy, result).Inc()
	rateLimitLatency.WithLabelValues(operation, strategy).Observe(duration.Seconds())
}

// RecordRedisOperation records a Redis operation
func (mr *MetricsRecorder) RecordRedisOperation(command, status string) {
	redisOperations.WithLabelValues(command, status).Inc()
}

// RecordConnectionStats records Redis connection pool stats
func (mr *MetricsRecorder) RecordConnectionStats(stats *redis.PoolStats) {
	if stats != nil {
		redisConnectionPool.WithLabelValues("hits").Set(float64(stats.Hits))
		redisConnectionPool.WithLabelValues("misses").Set(float64(stats.Misses))
		redisConnectionPool.WithLabelValues("timeouts").Set(float64(stats.Timeouts))
		redisConnectionPool.WithLabelValues("total_conns").Set(float64(stats.TotalConns))
		redisConnectionPool.WithLabelValues("idle_conns").Set(float64(stats.IdleConns))
		redisConnectionPool.WithLabelValues("stale_conns").Set(float64(stats.StaleConns))
	}
}

// RecordLimitViolation records a rate limit violation
func (mr *MetricsRecorder) RecordLimitViolation(keyType, algorithm, severity string) {
	limitViolations.WithLabelValues(keyType, algorithm, severity).Inc()
}

// UpdateActiveKeys updates the count of active rate limit keys
func (mr *MetricsRecorder) UpdateActiveKeys(keyType, algorithm string, count float64) {
	activeLimitKeys.WithLabelValues(keyType, algorithm).Set(count)
}

// UpdateCircuitBreakerState updates circuit breaker state
func (mr *MetricsRecorder) UpdateCircuitBreakerState(strategy string, state int) {
	circuitBreakerState.WithLabelValues(strategy).Set(float64(state))
}

// sendToExternalSystems sends alerts to configured external systems
func (am *AlertManager) sendToExternalSystems(alert Alert) error {
	if !am.config.Enabled {
		return nil
	}

	var errors []string

	for _, channel := range am.config.Channels {
		if !channel.Enabled {
			continue
		}

		var err error
		switch channel.Type {
		case AlertChannelWebhook:
			err = am.sendWebhook(alert, channel.Config)
		case AlertChannelSlack:
			err = am.sendSlack(alert, channel.Config)
		case AlertChannelPagerDuty:
			err = am.sendPagerDuty(alert, channel.Config)
		case AlertChannelKafka:
			err = am.sendKafka(alert, channel.Config)
		case AlertChannelPrometheus:
			err = am.sendPrometheus(alert, channel.Config)
		default:
			am.logger.Warn("unknown alert channel type", zap.String("type", string(channel.Type)))
			continue
		}

		if err != nil {
			am.logger.Error("failed to send alert to channel",
				zap.String("channel", string(channel.Type)),
				zap.String("alert_id", alert.ID),
				zap.Error(err))
			errors = append(errors, fmt.Sprintf("%s: %v", channel.Type, err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to send to channels: %s", strings.Join(errors, ", "))
	}

	return nil
}

// sendWebhook sends alert via HTTP webhook
func (am *AlertManager) sendWebhook(alert Alert, config map[string]interface{}) error {
	url, ok := config["url"].(string)
	if !ok || url == "" {
		return fmt.Errorf("webhook URL not configured")
	}

	payload, err := json.Marshal(alert)
	if err != nil {
		return fmt.Errorf("failed to marshal alert: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(payload))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Add authentication if configured
	if token, ok := config["auth_token"].(string); ok && token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := am.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	return nil
}

// sendSlack sends alert to Slack
func (am *AlertManager) sendSlack(alert Alert, config map[string]interface{}) error {
	webhookURL, ok := config["webhook_url"].(string)
	if !ok || webhookURL == "" {
		return fmt.Errorf("slack webhook URL not configured")
	}

	color := "#ffaa00" // warning color
	if alert.Level == "critical" {
		color = "#ff0000" // red
	}

	slackPayload := map[string]interface{}{
		"username": "Orbit CEX Rate Limiter",
		"attachments": []map[string]interface{}{
			{
				"color":     color,
				"title":     fmt.Sprintf("Rate Limiting Alert: %s", alert.Level),
				"text":      alert.Message,
				"timestamp": alert.Timestamp.Unix(),
				"fields": []map[string]interface{}{
					{"title": "Component", "value": alert.Component, "short": true},
					{"title": "Alert ID", "value": alert.ID, "short": true},
				},
			},
		},
	}

	payload, err := json.Marshal(slackPayload)
	if err != nil {
		return fmt.Errorf("failed to marshal Slack payload: %w", err)
	}

	resp, err := am.httpClient.Post(webhookURL, "application/json", bytes.NewBuffer(payload))
	if err != nil {
		return fmt.Errorf("failed to send Slack alert: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("slack returned status %d", resp.StatusCode)
	}

	return nil
}

// sendPagerDuty sends alert to PagerDuty
func (am *AlertManager) sendPagerDuty(alert Alert, config map[string]interface{}) error {
	integrationKey, ok := config["integration_key"].(string)
	if !ok || integrationKey == "" {
		return fmt.Errorf("pagerDuty integration key not configured")
	}

	severity := "warning"
	if alert.Level == "critical" {
		severity = "critical"
	}

	pdPayload := map[string]interface{}{
		"routing_key":  integrationKey,
		"event_action": "trigger",
		"dedup_key":    alert.ID,
		"payload": map[string]interface{}{
			"summary":        alert.Message,
			"source":         alert.Component,
			"severity":       severity,
			"timestamp":      alert.Timestamp.Format(time.RFC3339),
			"custom_details": alert.Annotations,
		},
	}

	payload, err := json.Marshal(pdPayload)
	if err != nil {
		return fmt.Errorf("failed to marshal PagerDuty payload: %w", err)
	}

	resp, err := am.httpClient.Post("https://events.pagerduty.com/v2/enqueue", "application/json", bytes.NewBuffer(payload))
	if err != nil {
		return fmt.Errorf("failed to send PagerDuty alert: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("pagerDuty returned status %d", resp.StatusCode)
	}

	return nil
}

// sendKafka sends alert to Kafka topic
func (am *AlertManager) sendKafka(alert Alert, config map[string]interface{}) error {
	if am.kafkaWriter == nil {
		return fmt.Errorf("kafka writer not initialized")
	}

	payload, err := json.Marshal(alert)
	if err != nil {
		return fmt.Errorf("failed to marshal alert for Kafka: %w", err)
	}

	message := kafka.Message{
		Key:   []byte(alert.ID),
		Value: payload,
		Headers: []kafka.Header{
			{Key: "alert_level", Value: []byte(alert.Level)},
			{Key: "component", Value: []byte(alert.Component)},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return am.kafkaWriter.WriteMessages(ctx, message)
}

// sendPrometheus sends alert as a Prometheus metric
func (am *AlertManager) sendPrometheus(alert Alert, config map[string]interface{}) error {
	// Record the alert as a Prometheus metric
	alertsTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "orbit",
			Subsystem: "ratelimit_alerts",
			Name:      "total",
			Help:      "Total number of rate limiting alerts",
		},
		[]string{"level", "component", "alert_id"},
	)

	// Register if not already registered
	prometheus.MustRegister(alertsTotal)

	alertsTotal.WithLabelValues(alert.Level, alert.Component, alert.ID).Inc()

	return nil
}

// Close cleans up resources
func (am *AlertManager) Close() error {
	if am.kafkaWriter != nil {
		return am.kafkaWriter.Close()
	}
	return nil
}
