// Package ratelimit provides comprehensive monitoring and observability
// for the rate limiting system with metrics, health checks, and alerting.
package ratelimit

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
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
	url    string
	logger *zap.Logger
	alerts []Alert
	mu     sync.RWMutex
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
	return &AlertManager{
		url:    url,
		logger: logger,
		alerts: make([]Alert, 0),
	}
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

	// TODO: Implement actual alert sending to external systems
	return nil
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
