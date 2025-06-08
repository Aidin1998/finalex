// Package ratelimit provides advanced rate limiting manager for coordinating
// multiple rate limiting strategies and providing a unified interface.
package ratelimit

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"
)

var (
	// Manager-level metrics
	managerRequests = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "orbit",
			Subsystem: "ratelimit_manager",
			Name:      "requests_total",
			Help:      "Total requests processed by rate limit manager",
		},
		[]string{"status", "strategy"},
	)

	managerDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "orbit",
			Subsystem: "ratelimit_manager",
			Name:      "processing_duration_seconds",
			Help:      "Time spent processing rate limit requests",
		},
		[]string{"strategy"},
	)

	activeStrategies = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "orbit",
			Subsystem: "ratelimit_manager",
			Name:      "active_strategies",
			Help:      "Number of active rate limiting strategies",
		},
		[]string{"type"},
	)
)

// RateLimitManager coordinates multiple rate limiting strategies
type RateLimitManager struct {
	strategies      map[string]RateLimitStrategy
	defaultStrategy string
	logger          *zap.Logger
	config          *ManagerConfig
	metrics         *ManagerMetrics
	mu              sync.RWMutex
}

// RateLimitStrategy defines the interface for different rate limiting strategies
type RateLimitStrategy interface {
	Name() string
	Check(ctx context.Context, r *http.Request) (RateLimitResult, error)
	Configure(config map[string]interface{}) error
	HealthCheck(ctx context.Context) error
}

// RateLimitResult contains the result of a rate limit check
type RateLimitResult struct {
	Allowed    bool
	RetryAfter time.Duration
	Headers    map[string]string
	Reason     string
	KeysUsed   []string
	Algorithm  string
}

// ManagerConfig holds configuration for the rate limit manager
type ManagerConfig struct {
	DefaultStrategy   string                 `yaml:"default_strategy" json:"default_strategy"`
	Strategies        map[string]interface{} `yaml:"strategies" json:"strategies"`
	FailureMode       string                 `yaml:"failure_mode" json:"failure_mode"` // "allow" or "deny"
	EnableMetrics     bool                   `yaml:"enable_metrics" json:"enable_metrics"`
	HealthCheckPeriod time.Duration          `yaml:"health_check_period" json:"health_check_period"`
	CircuitBreaker    CircuitBreakerConfig   `yaml:"circuit_breaker" json:"circuit_breaker"`
}

// CircuitBreakerConfig holds circuit breaker configuration
type CircuitBreakerConfig struct {
	Enabled           bool          `yaml:"enabled" json:"enabled"`
	FailureThreshold  int           `yaml:"failure_threshold" json:"failure_threshold"`
	RecoveryTimeout   time.Duration `yaml:"recovery_timeout" json:"recovery_timeout"`
	HalfOpenRequests  int           `yaml:"half_open_requests" json:"half_open_requests"`
}

// ManagerMetrics tracks manager-level metrics
type ManagerMetrics struct {
	RequestCount      map[string]int64
	ErrorCount        map[string]int64
	AverageLatency    map[string]time.Duration
	CircuitBreakerState map[string]string
	mu                sync.RWMutex
}

// NewRateLimitManager creates a new rate limit manager
func NewRateLimitManager(config *ManagerConfig, logger *zap.Logger) *RateLimitManager {
	manager := &RateLimitManager{
		strategies:      make(map[string]RateLimitStrategy),
		defaultStrategy: config.DefaultStrategy,
		logger:          logger,
		config:          config,
		metrics:         NewManagerMetrics(),
	}

	// Start health check routine if enabled
	if config.HealthCheckPeriod > 0 {
		go manager.healthCheckRoutine(config.HealthCheckPeriod)
	}

	return manager
}

// RegisterStrategy registers a new rate limiting strategy
func (m *RateLimitManager) RegisterStrategy(strategy RateLimitStrategy) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	name := strategy.Name()
	if _, exists := m.strategies[name]; exists {
		return fmt.Errorf("strategy %s already registered", name)
	}

	m.strategies[name] = strategy
	activeStrategies.WithLabelValues(name).Set(1)

	m.logger.Info("registered rate limiting strategy",
		zap.String("strategy", name))

	return nil
}

// Check performs rate limiting check using the appropriate strategy
func (m *RateLimitManager) Check(ctx context.Context, r *http.Request) (RateLimitResult, error) {
	start := time.Now()
	
	strategy := m.selectStrategy(r)
	strategyName := strategy.Name()

	defer func() {
		duration := time.Since(start)
		managerDuration.WithLabelValues(strategyName).Observe(duration.Seconds())
		m.metrics.UpdateLatency(strategyName, duration)
	}()

	result, err := strategy.Check(ctx, r)
	
	// Update metrics
	status := "allowed"
	if !result.Allowed {
		status = "denied"
	}
	
	managerRequests.WithLabelValues(status, strategyName).Inc()
	m.metrics.UpdateRequestCount(strategyName, status == "allowed")

	if err != nil {
		m.metrics.UpdateErrorCount(strategyName)
		
		// Apply failure mode
		if m.config.FailureMode == "deny" {
			return RateLimitResult{
				Allowed: false,
				RetryAfter: time.Minute,
				Headers: map[string]string{
					"X-RateLimit-Error": "rate limiting service error",
				},
				Reason: "service_error",
			}, err
		}
		
		// Allow on failure
		return RateLimitResult{
			Allowed: true,
			Headers: map[string]string{
				"X-RateLimit-Mode": "fail-safe",
			},
			Reason: "fail_safe",
		}, nil
	}

	// Add manager metadata to headers
	if result.Headers == nil {
		result.Headers = make(map[string]string)
	}
	result.Headers["X-RateLimit-Strategy"] = strategyName
	result.Headers["X-RateLimit-Manager"] = "orbit-cex"

	return result, nil
}

// selectStrategy selects the appropriate strategy for the request
func (m *RateLimitManager) selectStrategy(r *http.Request) RateLimitStrategy {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Custom strategy selection logic based on request properties
	if endpoint := r.Header.Get("X-Strategy-Override"); endpoint != "" {
		if strategy, exists := m.strategies[endpoint]; exists {
			return strategy
		}
	}

	// Priority-based selection
	priorities := []string{"enhanced", "tiered", "basic"}
	for _, priority := range priorities {
		if strategy, exists := m.strategies[priority]; exists {
			return strategy
		}
	}

	// Fall back to default strategy
	if strategy, exists := m.strategies[m.defaultStrategy]; exists {
		return strategy
	}

	// Return first available strategy
	for _, strategy := range m.strategies {
		return strategy
	}

	// This should never happen if strategies are properly registered
	panic("no rate limiting strategy available")
}

// GetStrategy returns a strategy by name
func (m *RateLimitManager) GetStrategy(name string) (RateLimitStrategy, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	strategy, exists := m.strategies[name]
	return strategy, exists
}

// ListStrategies returns all registered strategies
func (m *RateLimitManager) ListStrategies() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	names := make([]string, 0, len(m.strategies))
	for name := range m.strategies {
		names = append(names, name)
	}
	return names
}

// ConfigureStrategy updates configuration for a specific strategy
func (m *RateLimitManager) ConfigureStrategy(name string, config map[string]interface{}) error {
	m.mu.RLock()
	strategy, exists := m.strategies[name]
	m.mu.RUnlock()
	
	if !exists {
		return fmt.Errorf("strategy %s not found", name)
	}

	err := strategy.Configure(config)
	if err != nil {
		m.logger.Error("failed to configure strategy",
			zap.String("strategy", name),
			zap.Error(err))
		return err
	}

	m.logger.Info("configured strategy",
		zap.String("strategy", name))

	return nil
}

// HealthCheck checks the health of all strategies
func (m *RateLimitManager) HealthCheck(ctx context.Context) map[string]error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	results := make(map[string]error)
	for name, strategy := range m.strategies {
		results[name] = strategy.HealthCheck(ctx)
	}

	return results
}

// healthCheckRoutine periodically checks strategy health
func (m *RateLimitManager) healthCheckRoutine(period time.Duration) {
	ticker := time.NewTicker(period)
	defer ticker.Stop()

	for range ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		results := m.HealthCheck(ctx)
		cancel()

		for name, err := range results {
			if err != nil {
				m.logger.Warn("strategy health check failed",
					zap.String("strategy", name),
					zap.Error(err))
			}
		}
	}
}

// GetMetrics returns current manager metrics
func (m *RateLimitManager) GetMetrics() *ManagerMetrics {
	return m.metrics
}

// Close cleanly shuts down the manager
func (m *RateLimitManager) Close() error {
	m.logger.Info("shutting down rate limit manager")
	
	// Close all strategies if they implement io.Closer
	for name, strategy := range m.strategies {
		if closer, ok := strategy.(interface{ Close() error }); ok {
			if err := closer.Close(); err != nil {
				m.logger.Error("failed to close strategy",
					zap.String("strategy", name),
					zap.Error(err))
			}
		}
	}

	return nil
}

// NewManagerMetrics creates new manager metrics
func NewManagerMetrics() *ManagerMetrics {
	return &ManagerMetrics{
		RequestCount:        make(map[string]int64),
		ErrorCount:          make(map[string]int64),
		AverageLatency:      make(map[string]time.Duration),
		CircuitBreakerState: make(map[string]string),
	}
}

// UpdateRequestCount updates request count for a strategy
func (mm *ManagerMetrics) UpdateRequestCount(strategy string, allowed bool) {
	mm.mu.Lock()
	defer mm.mu.Unlock()
	
	mm.RequestCount[strategy]++
}

// UpdateErrorCount updates error count for a strategy
func (mm *ManagerMetrics) UpdateErrorCount(strategy string) {
	mm.mu.Lock()
	defer mm.mu.Unlock()
	
	mm.ErrorCount[strategy]++
}

// UpdateLatency updates average latency for a strategy
func (mm *ManagerMetrics) UpdateLatency(strategy string, latency time.Duration) {
	mm.mu.Lock()
	defer mm.mu.Unlock()
	
	// Simple moving average (could be improved with exponential smoothing)
	current := mm.AverageLatency[strategy]
	mm.AverageLatency[strategy] = (current + latency) / 2
}

// GetRequestCount returns request count for a strategy
func (mm *ManagerMetrics) GetRequestCount(strategy string) int64 {
	mm.mu.RLock()
	defer mm.mu.RUnlock()
	
	return mm.RequestCount[strategy]
}
