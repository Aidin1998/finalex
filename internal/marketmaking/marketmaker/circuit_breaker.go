// Circuit breaker implementation for MarketMaker fault tolerance
package marketmaker

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// CircuitBreakerState represents the state of a circuit breaker
type CircuitBreakerState int

const (
	CircuitBreakerClosed CircuitBreakerState = iota
	CircuitBreakerOpen
	CircuitBreakerHalfOpen
)

func (s CircuitBreakerState) String() string {
	switch s {
	case CircuitBreakerClosed:
		return "closed"
	case CircuitBreakerOpen:
		return "open"
	case CircuitBreakerHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// CircuitBreakerConfig contains circuit breaker configuration
type CircuitBreakerConfig struct {
	Threshold int                       // Number of failures before opening
	Window    time.Duration             // Time window for counting failures
	Timeout   time.Duration             // Time to wait before attempting reset
	OnTrip    func(ctx context.Context) // Called when circuit breaker trips
	OnReset   func(ctx context.Context) // Called when circuit breaker resets
}

// CircuitBreaker implements the circuit breaker pattern
type CircuitBreaker struct {
	config       CircuitBreakerConfig
	state        CircuitBreakerState
	failures     int
	successes    int
	lastFailTime time.Time
	lastSuccess  time.Time
	mu           sync.RWMutex

	// Failure tracking window
	failureWindow []time.Time
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(config CircuitBreakerConfig) *CircuitBreaker {
	if config.Timeout == 0 {
		config.Timeout = 60 * time.Second // Default timeout
	}

	return &CircuitBreaker{
		config:        config,
		state:         CircuitBreakerClosed,
		failureWindow: make([]time.Time, 0, config.Threshold*2),
	}
}

// GetState returns the current state of the circuit breaker
func (cb *CircuitBreaker) GetState() CircuitBreakerState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// CanExecute returns whether operations can be executed
func (cb *CircuitBreaker) CanExecute() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	switch cb.state {
	case CircuitBreakerClosed:
		return true
	case CircuitBreakerOpen:
		// Check if timeout has passed to move to half-open
		if time.Since(cb.lastFailTime) > cb.config.Timeout {
			return true // Will transition to half-open on next call
		}
		return false
	case CircuitBreakerHalfOpen:
		return true
	default:
		return false
	}
}

// RecordSuccess records a successful operation
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.successes++
	cb.lastSuccess = time.Now()

	switch cb.state {
	case CircuitBreakerHalfOpen:
		// Transition back to closed after successful operation in half-open state
		cb.state = CircuitBreakerClosed
		cb.failures = 0
		cb.failureWindow = cb.failureWindow[:0]

		if cb.config.OnReset != nil {
			go cb.config.OnReset(context.Background())
		}
	case CircuitBreakerClosed:
		// Clean old failures outside the window
		cb.cleanOldFailures()
	}
}

// RecordError records a failed operation
func (cb *CircuitBreaker) RecordError() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()
	cb.failures++
	cb.lastFailTime = now
	cb.failureWindow = append(cb.failureWindow, now)

	// Clean old failures outside the window
	cb.cleanOldFailures()

	// Count failures within the window
	recentFailures := len(cb.failureWindow)

	switch cb.state {
	case CircuitBreakerClosed:
		if recentFailures >= cb.config.Threshold {
			cb.state = CircuitBreakerOpen
			if cb.config.OnTrip != nil {
				go cb.config.OnTrip(context.Background())
			}
		}
	case CircuitBreakerHalfOpen:
		// Any failure in half-open state moves back to open
		cb.state = CircuitBreakerOpen
		if cb.config.OnTrip != nil {
			go cb.config.OnTrip(context.Background())
		}
	}
}

// Execute wraps an operation with circuit breaker logic
func (cb *CircuitBreaker) Execute(operation func() error) error {
	if !cb.CanExecute() {
		return ErrCircuitBreakerOpen
	}

	// Transition to half-open if needed
	cb.transitionToHalfOpenIfNeeded()

	err := operation()

	if err != nil {
		cb.RecordError()
		return err
	}

	cb.RecordSuccess()
	return nil
}

// transitionToHalfOpenIfNeeded transitions from open to half-open if timeout has passed
func (cb *CircuitBreaker) transitionToHalfOpenIfNeeded() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == CircuitBreakerOpen && time.Since(cb.lastFailTime) > cb.config.Timeout {
		cb.state = CircuitBreakerHalfOpen
	}
}

// cleanOldFailures removes failures outside the time window
func (cb *CircuitBreaker) cleanOldFailures() {
	cutoff := time.Now().Add(-cb.config.Window)

	// Find the first failure within the window
	start := 0
	for i, failTime := range cb.failureWindow {
		if failTime.After(cutoff) {
			start = i
			break
		}
	}

	// Remove old failures
	if start > 0 {
		copy(cb.failureWindow, cb.failureWindow[start:])
		cb.failureWindow = cb.failureWindow[:len(cb.failureWindow)-start]
	}
}

// Reset manually resets the circuit breaker to closed state
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.state = CircuitBreakerClosed
	cb.failures = 0
	cb.successes = 0
	cb.failureWindow = cb.failureWindow[:0]
}

// GetStats returns current statistics
func (cb *CircuitBreaker) GetStats() CircuitBreakerStats {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	recentFailures := len(cb.failureWindow)

	return CircuitBreakerStats{
		State:          cb.state,
		TotalFailures:  cb.failures,
		TotalSuccesses: cb.successes,
		RecentFailures: recentFailures,
		LastFailTime:   cb.lastFailTime,
		LastSuccess:    cb.lastSuccess,
		Threshold:      cb.config.Threshold,
		Window:         cb.config.Window,
	}
}

// CircuitBreakerStats contains circuit breaker statistics
type CircuitBreakerStats struct {
	State          CircuitBreakerState `json:"state"`
	TotalFailures  int                 `json:"total_failures"`
	TotalSuccesses int                 `json:"total_successes"`
	RecentFailures int                 `json:"recent_failures"`
	LastFailTime   time.Time           `json:"last_fail_time"`
	LastSuccess    time.Time           `json:"last_success"`
	Threshold      int                 `json:"threshold"`
	Window         time.Duration       `json:"window"`
}

// ErrCircuitBreakerOpen is returned when the circuit breaker is open
var ErrCircuitBreakerOpen = fmt.Errorf("circuit breaker is open")

// MultiCircuitBreaker manages multiple circuit breakers for different operations
type MultiCircuitBreaker struct {
	breakers map[string]*CircuitBreaker
	mu       sync.RWMutex
}

// NewMultiCircuitBreaker creates a new multi-circuit breaker
func NewMultiCircuitBreaker() *MultiCircuitBreaker {
	return &MultiCircuitBreaker{
		breakers: make(map[string]*CircuitBreaker),
	}
}

// AddBreaker adds a circuit breaker for a specific operation
func (mcb *MultiCircuitBreaker) AddBreaker(name string, config CircuitBreakerConfig) {
	mcb.mu.Lock()
	defer mcb.mu.Unlock()
	mcb.breakers[name] = NewCircuitBreaker(config)
}

// GetBreaker returns a circuit breaker by name
func (mcb *MultiCircuitBreaker) GetBreaker(name string) (*CircuitBreaker, bool) {
	mcb.mu.RLock()
	defer mcb.mu.RUnlock()
	breaker, exists := mcb.breakers[name]
	return breaker, exists
}

// Execute executes an operation with the specified circuit breaker
func (mcb *MultiCircuitBreaker) Execute(name string, operation func() error) error {
	breaker, exists := mcb.GetBreaker(name)
	if !exists {
		return fmt.Errorf("circuit breaker %s not found", name)
	}

	return breaker.Execute(operation)
}

// GetAllStats returns statistics for all circuit breakers
func (mcb *MultiCircuitBreaker) GetAllStats() map[string]CircuitBreakerStats {
	mcb.mu.RLock()
	defer mcb.mu.RUnlock()

	stats := make(map[string]CircuitBreakerStats)
	for name, breaker := range mcb.breakers {
		stats[name] = breaker.GetStats()
	}

	return stats
}

// ResetAll resets all circuit breakers
func (mcb *MultiCircuitBreaker) ResetAll() {
	mcb.mu.RLock()
	defer mcb.mu.RUnlock()

	for _, breaker := range mcb.breakers {
		breaker.Reset()
	}
}
