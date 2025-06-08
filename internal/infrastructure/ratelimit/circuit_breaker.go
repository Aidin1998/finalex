// Package ratelimit provides circuit breaker functionality for rate limiting
// to prevent cascading failures and provide graceful degradation.
package ratelimit

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// CircuitBreakerState represents the state of a circuit breaker
type CircuitBreakerState int32

const (
	// StateClosed - normal operation, requests pass through
	StateClosed CircuitBreakerState = iota
	// StateOpen - circuit is open, requests are rejected
	StateOpen
	// StateHalfOpen - testing if the service has recovered
	StateHalfOpen
)

func (s CircuitBreakerState) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// CircuitBreaker implements the circuit breaker pattern for rate limiting
type CircuitBreaker struct {
	name            string
	maxFailures     int64
	timeout         time.Duration
	halfOpenTimeout time.Duration
	maxHalfOpenReqs int64

	// State management
	state           int32 // CircuitBreakerState
	failureCount    int64
	lastFailureTime int64 // Unix nano
	halfOpenCount   int64

	// Metrics
	totalRequests   int64
	successRequests int64
	failedRequests  int64

	logger *zap.Logger
	mu     sync.RWMutex
}

// CircuitBreakerConfig holds circuit breaker configuration
type CircuitBreakerConfigDetail struct {
	Name            string        `yaml:"name" json:"name"`
	MaxFailures     int           `yaml:"max_failures" json:"max_failures"`
	Timeout         time.Duration `yaml:"timeout" json:"timeout"`
	HalfOpenTimeout time.Duration `yaml:"half_open_timeout" json:"half_open_timeout"`
	MaxHalfOpenReqs int           `yaml:"max_half_open_requests" json:"max_half_open_requests"`
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(config *CircuitBreakerConfigDetail, logger *zap.Logger) *CircuitBreaker {
	return &CircuitBreaker{
		name:            config.Name,
		maxFailures:     int64(config.MaxFailures),
		timeout:         config.Timeout,
		halfOpenTimeout: config.HalfOpenTimeout,
		maxHalfOpenReqs: int64(config.MaxHalfOpenReqs),
		state:           int32(StateClosed),
		logger:          logger,
	}
}

// Execute executes a function with circuit breaker protection
func (cb *CircuitBreaker) Execute(ctx context.Context, fn func(context.Context) error) error {
	// Check if we can proceed
	if !cb.allowRequest() {
		return &CircuitBreakerError{
			State:   cb.GetState(),
			Message: fmt.Sprintf("circuit breaker %s is %s", cb.name, cb.GetState()),
		}
	}

	// Execute the function
	err := fn(ctx)

	// Record the result
	if err != nil {
		cb.recordFailure()
	} else {
		cb.recordSuccess()
	}

	return err
}

// allowRequest determines if a request should be allowed
func (cb *CircuitBreaker) allowRequest() bool {
	atomic.AddInt64(&cb.totalRequests, 1)

	state := CircuitBreakerState(atomic.LoadInt32(&cb.state))
	now := time.Now().UnixNano()

	switch state {
	case StateClosed:
		return true

	case StateOpen:
		// Check if timeout has elapsed
		lastFailure := atomic.LoadInt64(&cb.lastFailureTime)
		if now-lastFailure >= cb.timeout.Nanoseconds() {
			// Transition to half-open
			if atomic.CompareAndSwapInt32(&cb.state, int32(StateOpen), int32(StateHalfOpen)) {
				atomic.StoreInt64(&cb.halfOpenCount, 0)
				cb.logger.Info("circuit breaker transitioning to half-open",
					zap.String("name", cb.name))
			}
			return true
		}
		return false

	case StateHalfOpen:
		// Allow limited requests in half-open state
		count := atomic.LoadInt64(&cb.halfOpenCount)
		if count < cb.maxHalfOpenReqs {
			atomic.AddInt64(&cb.halfOpenCount, 1)
			return true
		}
		return false

	default:
		return false
	}
}

// recordSuccess records a successful request
func (cb *CircuitBreaker) recordSuccess() {
	atomic.AddInt64(&cb.successRequests, 1)

	state := CircuitBreakerState(atomic.LoadInt32(&cb.state))
	if state == StateHalfOpen {
		// Check if we should close the circuit
		halfOpenCount := atomic.LoadInt64(&cb.halfOpenCount)
		if halfOpenCount >= cb.maxHalfOpenReqs {
			// All half-open requests succeeded, close the circuit
			if atomic.CompareAndSwapInt32(&cb.state, int32(StateHalfOpen), int32(StateClosed)) {
				atomic.StoreInt64(&cb.failureCount, 0)
				cb.logger.Info("circuit breaker closed after successful half-open test",
					zap.String("name", cb.name))
			}
		}
	}
}

// recordFailure records a failed request
func (cb *CircuitBreaker) recordFailure() {
	atomic.AddInt64(&cb.failedRequests, 1)
	failures := atomic.AddInt64(&cb.failureCount, 1)
	atomic.StoreInt64(&cb.lastFailureTime, time.Now().UnixNano())

	state := CircuitBreakerState(atomic.LoadInt32(&cb.state))

	// Check if we should open the circuit
	if state == StateClosed && failures >= cb.maxFailures {
		if atomic.CompareAndSwapInt32(&cb.state, int32(StateClosed), int32(StateOpen)) {
			cb.logger.Warn("circuit breaker opened due to failures",
				zap.String("name", cb.name),
				zap.Int64("failures", failures),
				zap.Int64("max_failures", cb.maxFailures))
		}
	} else if state == StateHalfOpen {
		// Failure during half-open, go back to open
		if atomic.CompareAndSwapInt32(&cb.state, int32(StateHalfOpen), int32(StateOpen)) {
			cb.logger.Warn("circuit breaker returned to open state during half-open test",
				zap.String("name", cb.name))
		}
	}
}

// GetState returns the current state of the circuit breaker
func (cb *CircuitBreaker) GetState() CircuitBreakerState {
	return CircuitBreakerState(atomic.LoadInt32(&cb.state))
}

// GetMetrics returns circuit breaker metrics
func (cb *CircuitBreaker) GetMetrics() CircuitBreakerMetrics {
	return CircuitBreakerMetrics{
		Name:            cb.name,
		State:           cb.GetState(),
		TotalRequests:   atomic.LoadInt64(&cb.totalRequests),
		SuccessRequests: atomic.LoadInt64(&cb.successRequests),
		FailedRequests:  atomic.LoadInt64(&cb.failedRequests),
		FailureCount:    atomic.LoadInt64(&cb.failureCount),
		LastFailureTime: time.Unix(0, atomic.LoadInt64(&cb.lastFailureTime)),
	}
}

// Reset resets the circuit breaker to closed state
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	atomic.StoreInt32(&cb.state, int32(StateClosed))
	atomic.StoreInt64(&cb.failureCount, 0)
	atomic.StoreInt64(&cb.halfOpenCount, 0)

	cb.logger.Info("circuit breaker manually reset",
		zap.String("name", cb.name))
}

// CircuitBreakerMetrics holds metrics for a circuit breaker
type CircuitBreakerMetrics struct {
	Name            string              `json:"name"`
	State           CircuitBreakerState `json:"state"`
	TotalRequests   int64               `json:"total_requests"`
	SuccessRequests int64               `json:"success_requests"`
	FailedRequests  int64               `json:"failed_requests"`
	FailureCount    int64               `json:"failure_count"`
	LastFailureTime time.Time           `json:"last_failure_time"`
}

// CircuitBreakerError represents an error from circuit breaker
type CircuitBreakerError struct {
	State   CircuitBreakerState
	Message string
}

func (e *CircuitBreakerError) Error() string {
	return e.Message
}

// IsCircuitBreakerError checks if an error is a circuit breaker error
func IsCircuitBreakerError(err error) bool {
	_, ok := err.(*CircuitBreakerError)
	return ok
}

// CircuitBreakerManager manages multiple circuit breakers
type CircuitBreakerManager struct {
	breakers map[string]*CircuitBreaker
	mu       sync.RWMutex
	logger   *zap.Logger
}

// NewCircuitBreakerManager creates a new circuit breaker manager
func NewCircuitBreakerManager(logger *zap.Logger) *CircuitBreakerManager {
	return &CircuitBreakerManager{
		breakers: make(map[string]*CircuitBreaker),
		logger:   logger,
	}
}

// GetOrCreate gets an existing circuit breaker or creates a new one
func (cbm *CircuitBreakerManager) GetOrCreate(name string, config *CircuitBreakerConfigDetail) *CircuitBreaker {
	cbm.mu.RLock()
	if breaker, exists := cbm.breakers[name]; exists {
		cbm.mu.RUnlock()
		return breaker
	}
	cbm.mu.RUnlock()

	cbm.mu.Lock()
	defer cbm.mu.Unlock()

	// Double-check after acquiring write lock
	if breaker, exists := cbm.breakers[name]; exists {
		return breaker
	}

	// Create new circuit breaker
	config.Name = name
	breaker := NewCircuitBreaker(config, cbm.logger)
	cbm.breakers[name] = breaker

	cbm.logger.Info("created new circuit breaker",
		zap.String("name", name),
		zap.Int("max_failures", config.MaxFailures),
		zap.Duration("timeout", config.Timeout))

	return breaker
}

// Get returns a circuit breaker by name
func (cbm *CircuitBreakerManager) Get(name string) (*CircuitBreaker, bool) {
	cbm.mu.RLock()
	defer cbm.mu.RUnlock()

	breaker, exists := cbm.breakers[name]
	return breaker, exists
}

// List returns all circuit breaker names
func (cbm *CircuitBreakerManager) List() []string {
	cbm.mu.RLock()
	defer cbm.mu.RUnlock()

	names := make([]string, 0, len(cbm.breakers))
	for name := range cbm.breakers {
		names = append(names, name)
	}
	return names
}

// GetAllMetrics returns metrics for all circuit breakers
func (cbm *CircuitBreakerManager) GetAllMetrics() map[string]CircuitBreakerMetrics {
	cbm.mu.RLock()
	defer cbm.mu.RUnlock()

	metrics := make(map[string]CircuitBreakerMetrics)
	for name, breaker := range cbm.breakers {
		metrics[name] = breaker.GetMetrics()
	}
	return metrics
}

// ResetAll resets all circuit breakers
func (cbm *CircuitBreakerManager) ResetAll() {
	cbm.mu.RLock()
	defer cbm.mu.RUnlock()

	for _, breaker := range cbm.breakers {
		breaker.Reset()
	}

	cbm.logger.Info("reset all circuit breakers")
}

// Close shuts down the circuit breaker manager
func (cbm *CircuitBreakerManager) Close() error {
	cbm.mu.Lock()
	defer cbm.mu.Unlock()

	cbm.breakers = make(map[string]*CircuitBreaker)
	cbm.logger.Info("circuit breaker manager closed")
	return nil
}

// RateLimitWithCircuitBreaker wraps rate limiting with circuit breaker protection
type RateLimitWithCircuitBreaker struct {
	strategy       RateLimitStrategy
	circuitBreaker *CircuitBreaker
	logger         *zap.Logger
}

// NewRateLimitWithCircuitBreaker creates a rate limiter with circuit breaker
func NewRateLimitWithCircuitBreaker(
	strategy RateLimitStrategy,
	cbConfig *CircuitBreakerConfigDetail,
	logger *zap.Logger,
) *RateLimitWithCircuitBreaker {
	circuitBreaker := NewCircuitBreaker(cbConfig, logger)

	return &RateLimitWithCircuitBreaker{
		strategy:       strategy,
		circuitBreaker: circuitBreaker,
		logger:         logger,
	}
}

// Name returns the strategy name with circuit breaker suffix
func (rlcb *RateLimitWithCircuitBreaker) Name() string {
	return fmt.Sprintf("%s-with-cb", rlcb.strategy.Name())
}

// Check performs rate limiting with circuit breaker protection
func (rlcb *RateLimitWithCircuitBreaker) Check(ctx context.Context, r *http.Request) (RateLimitResult, error) {
	var result RateLimitResult
	var err error

	cbErr := rlcb.circuitBreaker.Execute(ctx, func(ctx context.Context) error {
		result, err = rlcb.strategy.Check(ctx, r)
		return err
	})

	if cbErr != nil {
		// Circuit breaker is open
		if IsCircuitBreakerError(cbErr) {
			return RateLimitResult{
				Allowed:    false,
				RetryAfter: time.Minute, // Default retry after
				Headers: map[string]string{
					"X-RateLimit-CircuitBreaker": "open",
					"X-RateLimit-Error":          "service temporarily unavailable",
				},
				Reason:    "circuit_breaker_open",
				Algorithm: "circuit_breaker",
			}, nil // Don't return the circuit breaker error as HTTP error
		}
		return result, cbErr
	}

	// Add circuit breaker status to headers
	if result.Headers == nil {
		result.Headers = make(map[string]string)
	}
	result.Headers["X-RateLimit-CircuitBreaker"] = rlcb.circuitBreaker.GetState().String()

	return result, err
}

// Configure delegates configuration to the underlying strategy
func (rlcb *RateLimitWithCircuitBreaker) Configure(config map[string]interface{}) error {
	return rlcb.strategy.Configure(config)
}

// HealthCheck checks both the strategy and circuit breaker health
func (rlcb *RateLimitWithCircuitBreaker) HealthCheck(ctx context.Context) error {
	// Check if circuit breaker is allowing requests
	if rlcb.circuitBreaker.GetState() == StateOpen {
		return fmt.Errorf("circuit breaker is open")
	}

	// Check underlying strategy health
	return rlcb.strategy.HealthCheck(ctx)
}

// GetCircuitBreakerMetrics returns circuit breaker metrics
func (rlcb *RateLimitWithCircuitBreaker) GetCircuitBreakerMetrics() CircuitBreakerMetrics {
	return rlcb.circuitBreaker.GetMetrics()
}
