// Emergency kill-switch and circuit breaker system for MarketMaker
package marketmaker

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// EmergencyController manages emergency shutdown and circuit breaker functionality
type EmergencyController struct {
	isKilled       int32 // atomic boolean
	circuitBreaker *CircuitBreaker
	killReasons    []KillReason
	config         *EmergencyConfig
	logger         *StructuredLogger
	metrics        *MetricsCollector
	mu             sync.RWMutex

	// Components to shutdown
	tradingAPI      TradingAPI
	strategyManager StrategyManager
	feedManager     FeedManager

	// Callbacks for emergency actions
	emergencyCallbacks []EmergencyCallback
}

// EmergencyConfig contains emergency control configuration
type EmergencyConfig struct {
	EnableEmergencyKill     bool          `json:"enable_emergency_kill"`
	CircuitBreakerThreshold int           `json:"circuit_breaker_threshold"`
	CircuitBreakerWindow    time.Duration `json:"circuit_breaker_window"`
	AutoRecoveryEnabled     bool          `json:"auto_recovery_enabled"`
	AutoRecoveryDelay       time.Duration `json:"auto_recovery_delay"`
	MaxLossThreshold        float64       `json:"max_loss_threshold"`
	PositionSizeThreshold   float64       `json:"position_size_threshold"`
	APIErrorThreshold       int           `json:"api_error_threshold"`
	FeedDisconnectThreshold time.Duration `json:"feed_disconnect_threshold"`
}

// KillReason represents why the emergency kill was triggered
type KillReason struct {
	Timestamp time.Time              `json:"timestamp"`
	Type      string                 `json:"type"`
	Reason    string                 `json:"reason"`
	Details   map[string]interface{} `json:"details,omitempty"`
	Severity  string                 `json:"severity"`
}

// EmergencyCallback is called when emergency actions are triggered
type EmergencyCallback func(ctx context.Context, reason KillReason) error

// NewEmergencyController creates a new emergency controller
func NewEmergencyController(
	config *EmergencyConfig,
	logger *StructuredLogger,
	metrics *MetricsCollector,
	tradingAPI TradingAPI,
	strategyManager StrategyManager,
	feedManager FeedManager,
) *EmergencyController {
	ec := &EmergencyController{
		config:             config,
		logger:             logger,
		metrics:            metrics,
		tradingAPI:         tradingAPI,
		strategyManager:    strategyManager,
		feedManager:        feedManager,
		killReasons:        make([]KillReason, 0),
		emergencyCallbacks: make([]EmergencyCallback, 0),
	}

	// Initialize circuit breaker
	ec.circuitBreaker = NewCircuitBreaker(CircuitBreakerConfig{
		Threshold: config.CircuitBreakerThreshold,
		Window:    config.CircuitBreakerWindow,
		OnTrip:    ec.onCircuitBreakerTrip,
		OnReset:   ec.onCircuitBreakerReset,
	})

	return ec
}

// RegisterEmergencyCallback registers a callback for emergency events
func (ec *EmergencyController) RegisterEmergencyCallback(callback EmergencyCallback) {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	ec.emergencyCallbacks = append(ec.emergencyCallbacks, callback)
}

// IsKilled returns whether the emergency kill switch is activated
func (ec *EmergencyController) IsKilled() bool {
	return atomic.LoadInt32(&ec.isKilled) == 1
}

// TriggerEmergencyKill activates the emergency kill switch
func (ec *EmergencyController) TriggerEmergencyKill(ctx context.Context, reason KillReason) error {
	if !ec.config.EnableEmergencyKill {
		return fmt.Errorf("emergency kill is disabled")
	}

	// Set the kill flag atomically
	if !atomic.CompareAndSwapInt32(&ec.isKilled, 0, 1) {
		return fmt.Errorf("emergency kill already activated")
	}

	reason.Timestamp = time.Now()

	ec.mu.Lock()
	ec.killReasons = append(ec.killReasons, reason)
	ec.mu.Unlock()

	ec.logger.LogEmergencyEvent(ctx, "kill_switch_activated", reason.Reason, map[string]interface{}{
		"type":     reason.Type,
		"severity": reason.Severity,
		"details":  reason.Details,
	})
	ec.metrics.RecordEmergencyKill(reason.Type, reason.Reason)

	// Execute emergency shutdown sequence
	return ec.executeEmergencyShutdown(ctx, reason)
}

// executeEmergencyShutdown performs the emergency shutdown sequence
func (ec *EmergencyController) executeEmergencyShutdown(ctx context.Context, reason KillReason) error {
	ec.logger.LogEmergencyEvent(ctx, "emergency_shutdown_started", "beginning shutdown sequence", nil)

	// 1. Cancel all open orders immediately
	if err := ec.cancelAllOrders(ctx); err != nil {
		ec.logger.LogError(ctx, "failed to cancel all orders during emergency shutdown", map[string]interface{}{
			"error": err.Error(),
		})
	}

	// 2. Stop all trading strategies
	if err := ec.stopAllStrategies(ctx); err != nil {
		ec.logger.LogError(ctx, "failed to stop strategies during emergency shutdown", map[string]interface{}{
			"error": err.Error(),
		})
	}

	// 3. Disconnect feeds to prevent new signals
	if err := ec.disconnectFeeds(ctx); err != nil {
		ec.logger.LogError(ctx, "failed to disconnect feeds during emergency shutdown", map[string]interface{}{
			"error": err.Error(),
		})
	}

	// 4. Execute emergency callbacks
	ec.executeEmergencyCallbacks(ctx, reason)

	ec.logger.LogEmergencyEvent(ctx, "emergency_shutdown_completed", "shutdown sequence completed", nil)
	ec.metrics.RecordEmergencyShutdownDuration(time.Since(reason.Timestamp))

	// 5. Schedule auto-recovery if enabled
	if ec.config.AutoRecoveryEnabled {
		go ec.scheduleAutoRecovery(ctx, reason)
	}

	return nil
}

// cancelAllOrders cancels all open orders across all pairs
func (ec *EmergencyController) cancelAllOrders(ctx context.Context) error {
	ec.logger.LogInfo(ctx, "cancelling all open orders", nil)

	// This would need to be implemented based on your TradingAPI interface
	// For now, it's a placeholder that would batch cancel all orders
	if batchAPI, ok := ec.tradingAPI.(interface {
		BatchCancelAllOrders(ctx context.Context) error
	}); ok {
		return batchAPI.BatchCancelAllOrders(ctx)
	}

	// Fallback: cancel orders pair by pair (this would need pair list)
	return fmt.Errorf("batch cancel not implemented")
}

// stopAllStrategies stops all running trading strategies
func (ec *EmergencyController) stopAllStrategies(ctx context.Context) error {
	ec.logger.LogInfo(ctx, "stopping all trading strategies", nil)

	if stopAPI, ok := ec.strategyManager.(interface {
		StopAllStrategies(ctx context.Context) error
	}); ok {
		return stopAPI.StopAllStrategies(ctx)
	}

	return fmt.Errorf("stop all strategies not implemented")
}

// disconnectFeeds disconnects all market data feeds
func (ec *EmergencyController) disconnectFeeds(ctx context.Context) error {
	ec.logger.LogInfo(ctx, "disconnecting market data feeds", nil)

	if disconnectAPI, ok := ec.feedManager.(interface {
		DisconnectAll(ctx context.Context) error
	}); ok {
		return disconnectAPI.DisconnectAll(ctx)
	}

	return fmt.Errorf("disconnect all feeds not implemented")
}

// executeEmergencyCallbacks executes all registered emergency callbacks
func (ec *EmergencyController) executeEmergencyCallbacks(ctx context.Context, reason KillReason) {
	ec.mu.RLock()
	callbacks := make([]EmergencyCallback, len(ec.emergencyCallbacks))
	copy(callbacks, ec.emergencyCallbacks)
	ec.mu.RUnlock()

	for i, callback := range callbacks {
		if err := callback(ctx, reason); err != nil {
			ec.logger.LogError(ctx, "emergency callback failed", map[string]interface{}{
				"callback_index": i,
				"error":          err.Error(),
			})
		}
	}
}

// scheduleAutoRecovery schedules automatic recovery if enabled
func (ec *EmergencyController) scheduleAutoRecovery(ctx context.Context, reason KillReason) {
	ec.logger.LogInfo(ctx, "scheduling auto-recovery", map[string]interface{}{
		"delay": ec.config.AutoRecoveryDelay,
	})

	time.Sleep(ec.config.AutoRecoveryDelay)

	if err := ec.AttemptRecovery(ctx); err != nil {
		ec.logger.LogError(ctx, "auto-recovery failed", map[string]interface{}{
			"error": err.Error(),
		})
	}
}

// AttemptRecovery attempts to recover from emergency kill state
func (ec *EmergencyController) AttemptRecovery(ctx context.Context) error {
	if !ec.IsKilled() {
		return fmt.Errorf("system is not in emergency kill state")
	}

	ec.logger.LogEmergencyEvent(ctx, "recovery_attempt_started", "attempting system recovery", nil)

	// Verify system health before recovery
	if err := ec.verifySystemHealth(ctx); err != nil {
		return fmt.Errorf("system health check failed: %w", err)
	}

	// Reset the kill flag
	atomic.StoreInt32(&ec.isKilled, 0)

	// Reset circuit breaker
	ec.circuitBreaker.Reset()

	ec.logger.LogEmergencyEvent(ctx, "recovery_completed", "system recovered from emergency state", nil)
	ec.metrics.RecordEmergencyRecovery()

	return nil
}

// verifySystemHealth checks if the system is healthy enough for recovery
func (ec *EmergencyController) verifySystemHealth(ctx context.Context) error {
	// Check trading API health
	if statusAPI, ok := ec.tradingAPI.(interface {
		HealthCheck(ctx context.Context) error
	}); ok {
		if err := statusAPI.HealthCheck(ctx); err != nil {
			return fmt.Errorf("trading API health check failed: %w", err)
		}
	}

	// Check feed manager health
	if statusFeed, ok := ec.feedManager.(interface {
		HealthCheck(ctx context.Context) error
	}); ok {
		if err := statusFeed.HealthCheck(ctx); err != nil {
			return fmt.Errorf("feed manager health check failed: %w", err)
		}
	}

	return nil
}

// GetKillReasons returns the history of kill reasons
func (ec *EmergencyController) GetKillReasons() []KillReason {
	ec.mu.RLock()
	defer ec.mu.RUnlock()

	reasons := make([]KillReason, len(ec.killReasons))
	copy(reasons, ec.killReasons)
	return reasons
}

// CheckTriggerConditions checks various conditions that might trigger emergency kill
func (ec *EmergencyController) CheckTriggerConditions(ctx context.Context, metrics *PerformanceMetrics) {
	if ec.IsKilled() {
		return
	}

	// Check loss threshold
	if metrics.UnrealizedPnL < -ec.config.MaxLossThreshold {
		reason := KillReason{
			Type:     "loss_threshold",
			Reason:   fmt.Sprintf("unrealized PnL %.2f exceeds max loss threshold %.2f", metrics.UnrealizedPnL, ec.config.MaxLossThreshold),
			Severity: "critical",
			Details: map[string]interface{}{
				"unrealized_pnl": metrics.UnrealizedPnL,
				"loss_threshold": ec.config.MaxLossThreshold,
			},
		}
		ec.TriggerEmergencyKill(ctx, reason)
		return
	}

	// Check position size threshold
	if metrics.TotalExposure > ec.config.PositionSizeThreshold {
		reason := KillReason{
			Type:     "position_size",
			Reason:   fmt.Sprintf("total exposure %.2f exceeds threshold %.2f", metrics.TotalExposure, ec.config.PositionSizeThreshold),
			Severity: "critical",
			Details: map[string]interface{}{
				"total_exposure":     metrics.TotalExposure,
				"position_threshold": ec.config.PositionSizeThreshold,
			},
		}
		ec.TriggerEmergencyKill(ctx, reason)
		return
	}
}

// ReportError reports an error to the circuit breaker
func (ec *EmergencyController) ReportError(ctx context.Context, errorType string, err error) {
	ec.circuitBreaker.RecordError()

	ec.logger.LogError(ctx, "error reported to emergency controller", map[string]interface{}{
		"error_type":            errorType,
		"error":                 err.Error(),
		"circuit_breaker_state": ec.circuitBreaker.GetState(),
	})
}

// ReportSuccess reports a successful operation to the circuit breaker
func (ec *EmergencyController) ReportSuccess() {
	ec.circuitBreaker.RecordSuccess()
}

// onCircuitBreakerTrip is called when the circuit breaker trips
func (ec *EmergencyController) onCircuitBreakerTrip(ctx context.Context) {
	reason := KillReason{
		Type:     "circuit_breaker",
		Reason:   "circuit breaker tripped due to too many failures",
		Severity: "critical",
		Details: map[string]interface{}{
			"threshold": ec.config.CircuitBreakerThreshold,
			"window":    ec.config.CircuitBreakerWindow,
		},
	}

	if err := ec.TriggerEmergencyKill(ctx, reason); err != nil {
		ec.logger.LogError(ctx, "failed to trigger emergency kill from circuit breaker", map[string]interface{}{
			"error": err.Error(),
		})
	}
}

// onCircuitBreakerReset is called when the circuit breaker resets
func (ec *EmergencyController) onCircuitBreakerReset(ctx context.Context) {
	ec.logger.LogInfo(ctx, "circuit breaker reset", nil)
	ec.metrics.RecordCircuitBreakerReset()
}

// GetStatus returns the current emergency controller status
func (ec *EmergencyController) GetStatus() map[string]interface{} {
	ec.mu.RLock()
	defer ec.mu.RUnlock()

	return map[string]interface{}{
		"is_killed":             ec.IsKilled(),
		"circuit_breaker_state": ec.circuitBreaker.GetState(),
		"kill_count":            len(ec.killReasons),
		"auto_recovery_enabled": ec.config.AutoRecoveryEnabled,
		"last_kill_reason": func() interface{} {
			if len(ec.killReasons) > 0 {
				return ec.killReasons[len(ec.killReasons)-1]
			}
			return nil
		}(),
	}
}
