// Self-healing implementations for MarketMaker components
package marketmaker

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// SelfHealingManager coordinates all self-healing activities
type SelfHealingManager struct {
	healers         map[string]SelfHealer
	config          *SelfHealingConfig
	logger          *StructuredLogger
	metrics         *MetricsCollector
	mu              sync.RWMutex
	healingInFlight map[string]bool
	healingHistory  []HealingEvent
}

// SelfHealingConfig contains configuration for self-healing behaviors
type SelfHealingConfig struct {
	EnableSelfHealing       bool          `json:"enable_self_healing"`
	MaxHealingAttempts      int           `json:"max_healing_attempts"`
	HealingCooldown         time.Duration `json:"healing_cooldown"`
	FeedReconnectDelay      time.Duration `json:"feed_reconnect_delay"`
	StrategyRestartDelay    time.Duration `json:"strategy_restart_delay"`
	CircuitBreakerThreshold int           `json:"circuit_breaker_threshold"`
	EmergencyKillEnabled    bool          `json:"emergency_kill_enabled"`
	AutoRecoveryEnabled     bool          `json:"auto_recovery_enabled"`
}

// HealingEvent records a self-healing action
type HealingEvent struct {
	Timestamp  time.Time     `json:"timestamp"`
	Component  string        `json:"component"`
	Action     string        `json:"action"`
	Reason     string        `json:"reason"`
	Success    bool          `json:"success"`
	Duration   time.Duration `json:"duration"`
	AttemptNum int           `json:"attempt_num"`
	Error      string        `json:"error,omitempty"`
}

// NewSelfHealingManager creates a new self-healing manager
func NewSelfHealingManager(config *SelfHealingConfig, logger *StructuredLogger, metrics *MetricsCollector) *SelfHealingManager {
	return &SelfHealingManager{
		healers:         make(map[string]SelfHealer),
		config:          config,
		logger:          logger,
		metrics:         metrics,
		healingInFlight: make(map[string]bool),
		healingHistory:  make([]HealingEvent, 0),
	}
}

// RegisterHealer registers a self-healer for a component
func (shm *SelfHealingManager) RegisterHealer(name string, healer SelfHealer) {
	shm.mu.Lock()
	defer shm.mu.Unlock()
	shm.healers[name] = healer
}

// TriggerHealing attempts to heal a specific component
func (shm *SelfHealingManager) TriggerHealing(ctx context.Context, component string, reason string) error {
	if !shm.config.EnableSelfHealing {
		return fmt.Errorf("self-healing is disabled")
	}

	shm.mu.Lock()
	if shm.healingInFlight[component] {
		shm.mu.Unlock()
		return fmt.Errorf("healing already in flight for component %s", component)
	}
	shm.healingInFlight[component] = true
	shm.mu.Unlock()

	defer func() {
		shm.mu.Lock()
		delete(shm.healingInFlight, component)
		shm.mu.Unlock()
	}()

	healer, exists := shm.healers[component]
	if !exists {
		return fmt.Errorf("no healer registered for component %s", component)
	}

	start := time.Now()
	event := HealingEvent{
		Timestamp:  start,
		Component:  component,
		Reason:     reason,
		AttemptNum: 1,
	}

	shm.logger.LogSelfHealingEvent(ctx, component, "healing_started", reason, nil)
	shm.metrics.RecordSelfHealingAttempt(component, reason)

	// Attempt healing with retries
	var lastErr error
	for attempt := 1; attempt <= shm.config.MaxHealingAttempts; attempt++ {
		event.AttemptNum = attempt

		healCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		// Pass a minimal HealthCheckResult as required by the interface
		healthResult := HealthCheckResult{
			Component:   component,
			Subsystem:   "",
			Status:      0, // HealthUnknown
			Message:     reason,
			LastChecked: time.Now(),
			CheckCount:  0,
			FailCount:   0,
			Details:     nil,
		}
		err := healer.Heal(healCtx, healthResult)
		cancel()

		if err == nil {
			event.Success = true
			event.Duration = time.Since(start)
			shm.recordHealingEvent(event)

			shm.logger.LogSelfHealingEvent(ctx, component, "healing_success", reason, map[string]interface{}{
				"attempt":  attempt,
				"duration": event.Duration,
			})
			shm.metrics.RecordSelfHealingSuccess(component, reason, attempt, event.Duration)
			return nil
		}

		lastErr = err
		event.Error = err.Error()

		shm.logger.LogSelfHealingEvent(ctx, component, "healing_attempt_failed", reason, map[string]interface{}{
			"attempt": attempt,
			"error":   err.Error(),
		})

		if attempt < shm.config.MaxHealingAttempts {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(shm.config.HealingCooldown):
				// Continue to next attempt
			}
		}
	}

	event.Success = false
	event.Duration = time.Since(start)
	event.Error = lastErr.Error()
	shm.recordHealingEvent(event)

	shm.logger.LogSelfHealingEvent(ctx, component, "healing_failed", reason, map[string]interface{}{
		"attempts": shm.config.MaxHealingAttempts,
		"error":    lastErr.Error(),
	})
	shm.metrics.RecordSelfHealingFailure(component, reason, shm.config.MaxHealingAttempts)

	return fmt.Errorf("healing failed after %d attempts: %w", shm.config.MaxHealingAttempts, lastErr)
}

// recordHealingEvent records a healing event in history
func (shm *SelfHealingManager) recordHealingEvent(event HealingEvent) {
	shm.mu.Lock()
	defer shm.mu.Unlock()

	shm.healingHistory = append(shm.healingHistory, event)

	// Keep only last 1000 events
	if len(shm.healingHistory) > 1000 {
		shm.healingHistory = shm.healingHistory[len(shm.healingHistory)-1000:]
	}
}

// GetHealingHistory returns the healing event history
func (shm *SelfHealingManager) GetHealingHistory() []HealingEvent {
	shm.mu.RLock()
	defer shm.mu.RUnlock()

	history := make([]HealingEvent, len(shm.healingHistory))
	copy(history, shm.healingHistory)
	return history
}

// FeedSelfHealer implements self-healing for market data feeds
type FeedSelfHealer struct {
	feedManager FeedManager
	config      *SelfHealingConfig
	logger      *StructuredLogger
	metrics     *MetricsCollector
}

// FeedManager interface for managing market data feeds
type FeedManager interface {
	Reconnect(ctx context.Context) error
	GetConnectionStatus() string
	Reset() error
}

func NewFeedSelfHealer(feedManager FeedManager, config *SelfHealingConfig, logger *StructuredLogger, metrics *MetricsCollector) *FeedSelfHealer {
	return &FeedSelfHealer{
		feedManager: feedManager,
		config:      config,
		logger:      logger,
		metrics:     metrics,
	}
}

func (fsh *FeedSelfHealer) Name() string {
	return "feed_healer"
}

// Update FeedSelfHealer to match SelfHealer interface
func (fsh *FeedSelfHealer) CanHeal(result HealthCheckResult) bool {
	// For test/demo, always return true. In production, check result.Status, etc.
	return true
}

// Remove old Heal(ctx context.Context) method and use the correct signature
func (fsh *FeedSelfHealer) Heal(ctx context.Context, result HealthCheckResult) error {
	fsh.logger.LogInfo(ctx, "attempting feed reconnection", map[string]interface{}{
		"status": fsh.feedManager.GetConnectionStatus(),
	})

	// Add delay before reconnection
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(fsh.config.FeedReconnectDelay):
	}

	// Reset feed manager state
	if err := fsh.feedManager.Reset(); err != nil {
		return fmt.Errorf("failed to reset feed manager: %w", err)
	}

	// Attempt reconnection
	if err := fsh.feedManager.Reconnect(ctx); err != nil {
		return fmt.Errorf("failed to reconnect feed: %w", err)
	}

	fsh.logger.LogInfo(ctx, "feed reconnection successful", nil)
	return nil
}

func (fsh *FeedSelfHealer) HealingDescription() string {
	return "Feed reconnection and reset"
}

// StrategySelfHealer implements self-healing for trading strategies
type StrategySelfHealer struct {
	strategyManager StrategyManager
	config          *SelfHealingConfig
	logger          *StructuredLogger
	metrics         *MetricsCollector
}

// StrategyManager interface for managing trading strategies
type StrategyManager interface {
	RestartStrategy(ctx context.Context, strategyName string) error
	GetStrategyStatus(strategyName string) string
	ResetStrategy(strategyName string) error
}

func NewStrategySelfHealer(strategyManager StrategyManager, config *SelfHealingConfig, logger *StructuredLogger, metrics *MetricsCollector) *StrategySelfHealer {
	return &StrategySelfHealer{
		strategyManager: strategyManager,
		config:          config,
		logger:          logger,
		metrics:         metrics,
	}
}

func (ssh *StrategySelfHealer) Name() string {
	return "strategy_healer"
}

func (ssh *StrategySelfHealer) Heal(ctx context.Context) error {
	ssh.logger.LogInfo(ctx, "attempting strategy restart", nil)

	// Add delay before restart
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(ssh.config.StrategyRestartDelay):
	}

	// For now, restart all strategies - in production, you'd restart specific failed strategies
	// This would be enhanced to restart only failed strategies based on health check results
	return ssh.strategyManager.RestartStrategy(ctx, "all")
}

// TradingAPISelfHealer implements self-healing for trading API connections
type TradingAPISelfHealer struct {
	tradingAPI TradingAPIManager
	config     *SelfHealingConfig
	logger     *StructuredLogger
	metrics    *MetricsCollector
}

// TradingAPIManager interface for managing trading API connections
type TradingAPIManager interface {
	ReconnectAPI(ctx context.Context) error
	GetAPIStatus() string
	ResetConnections() error
}

func NewTradingAPISelfHealer(tradingAPI TradingAPIManager, config *SelfHealingConfig, logger *StructuredLogger, metrics *MetricsCollector) *TradingAPISelfHealer {
	return &TradingAPISelfHealer{
		tradingAPI: tradingAPI,
		config:     config,
		logger:     logger,
		metrics:    metrics,
	}
}

func (tash *TradingAPISelfHealer) Name() string {
	return "trading_api_healer"
}

func (tash *TradingAPISelfHealer) Heal(ctx context.Context) error {
	tash.logger.LogInfo(ctx, "attempting trading API reconnection", map[string]interface{}{
		"status": tash.tradingAPI.GetAPIStatus(),
	})

	// Reset existing connections
	if err := tash.tradingAPI.ResetConnections(); err != nil {
		return fmt.Errorf("failed to reset trading API connections: %w", err)
	}

	// Reconnect
	if err := tash.tradingAPI.ReconnectAPI(ctx); err != nil {
		return fmt.Errorf("failed to reconnect trading API: %w", err)
	}

	tash.logger.LogInfo(ctx, "trading API reconnection successful", nil)
	return nil
}

// RiskManagerSelfHealer implements self-healing for risk management system
type RiskManagerSelfHealer struct {
	riskManager RiskManagerInterface
	config      *SelfHealingConfig
	logger      *StructuredLogger
	metrics     *MetricsCollector
}

// RiskManagerInterface for self-healing operations
type RiskManagerInterface interface {
	Reset() error
	ValidateState() error
	GetStatus() string
}

func NewRiskManagerSelfHealer(riskManager RiskManagerInterface, config *SelfHealingConfig, logger *StructuredLogger, metrics *MetricsCollector) *RiskManagerSelfHealer {
	return &RiskManagerSelfHealer{
		riskManager: riskManager,
		config:      config,
		logger:      logger,
		metrics:     metrics,
	}
}

func (rmsh *RiskManagerSelfHealer) Name() string {
	return "risk_manager_healer"
}

func (rmsh *RiskManagerSelfHealer) Heal(ctx context.Context) error {
	rmsh.logger.LogInfo(ctx, "attempting risk manager reset", map[string]interface{}{
		"status": rmsh.riskManager.GetStatus(),
	})

	// Reset risk manager state
	if err := rmsh.riskManager.Reset(); err != nil {
		return fmt.Errorf("failed to reset risk manager: %w", err)
	}

	// Validate the reset worked
	if err := rmsh.riskManager.ValidateState(); err != nil {
		return fmt.Errorf("risk manager state validation failed after reset: %w", err)
	}

	rmsh.logger.LogInfo(ctx, "risk manager reset successful", nil)
	return nil
}

// --- BEGIN: Add missing StructuredLogger and MetricsCollector methods for self_healing.go ---
func (sl *StructuredLogger) LogSelfHealingEvent(ctx context.Context, component, eventType, reason string, details map[string]interface{}) {
}
func (mc *MetricsCollector) RecordSelfHealingAttempt(component, reason string) {}
func (mc *MetricsCollector) RecordSelfHealingSuccess(component, reason string, attempt int, duration time.Duration) {
}
func (mc *MetricsCollector) RecordSelfHealingFailure(component, reason string, maxAttempts int) {}

// --- END ---
