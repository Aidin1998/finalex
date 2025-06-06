// Health check system with self-healing capabilities for MarketMaker
package marketmaker

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Aidin1998/finalex/internal/marketmaking/strategies/common"
)

// HealthStatus represents the health state of a component
type HealthStatus int

const (
	HealthUnknown HealthStatus = iota
	HealthHealthy
	HealthDegraded
	HealthUnhealthy
)

func (h HealthStatus) String() string {
	switch h {
	case HealthHealthy:
		return "healthy"
	case HealthDegraded:
		return "degraded"
	case HealthUnhealthy:
		return "unhealthy"
	default:
		return "unknown"
	}
}

// HealthCheckResult contains the result of a health check
type HealthCheckResult struct {
	Component   string                 `json:"component"`
	Subsystem   string                 `json:"subsystem"`
	Status      HealthStatus           `json:"status"`
	Message     string                 `json:"message"`
	LastChecked time.Time              `json:"last_checked"`
	CheckCount  int64                  `json:"check_count"`
	FailCount   int64                  `json:"fail_count"`
	Details     map[string]interface{} `json:"details,omitempty"`
}

// HealthChecker interface for implementing health checks
type HealthChecker interface {
	Check(ctx context.Context) HealthCheckResult
	Name() string
	Component() string
	Subsystem() string
}

// SelfHealer interface for implementing self-healing actions
type SelfHealer interface {
	CanHeal(result HealthCheckResult) bool
	Heal(ctx context.Context, result HealthCheckResult) error
	HealingDescription() string
}

// HealthCheckConfig configures health check behavior
type HealthCheckConfig struct {
	Interval           time.Duration `yaml:"interval"`
	Timeout            time.Duration `yaml:"timeout"`
	FailureThreshold   int           `yaml:"failure_threshold"`
	RecoveryThreshold  int           `yaml:"recovery_threshold"`
	EnableSelfHealing  bool          `yaml:"enable_self_healing"`
	HealingCooldown    time.Duration `yaml:"healing_cooldown"`
	MaxHealingAttempts int           `yaml:"max_healing_attempts"`
}

// DefaultHealthCheckConfig returns sensible defaults
func DefaultHealthCheckConfig() HealthCheckConfig {
	return HealthCheckConfig{
		Interval:           30 * time.Second,
		Timeout:            10 * time.Second,
		FailureThreshold:   3,
		RecoveryThreshold:  2,
		EnableSelfHealing:  true,
		HealingCooldown:    5 * time.Minute,
		MaxHealingAttempts: 3,
	}
}

// HealthMonitor manages health checks and self-healing
type HealthMonitor struct {
	config     HealthCheckConfig
	checkers   map[string]HealthChecker
	healers    map[string]SelfHealer
	results    map[string]*HealthCheckResult
	healingLog map[string][]HealingAttempt
	logger     *StructuredLogger
	metrics    *MetricsCollector
	mu         sync.RWMutex
	stopCh     chan struct{}
	wg         sync.WaitGroup
}

// HealingAttempt tracks self-healing attempts
type HealingAttempt struct {
	Timestamp   time.Time `json:"timestamp"`
	Component   string    `json:"component"`
	Subsystem   string    `json:"subsystem"`
	Description string    `json:"description"`
	Success     bool      `json:"success"`
	Error       string    `json:"error,omitempty"`
}

// NewHealthMonitor creates a new health monitoring system
func NewHealthMonitor(config HealthCheckConfig, logger *StructuredLogger, metrics *MetricsCollector) *HealthMonitor {
	return &HealthMonitor{
		config:     config,
		checkers:   make(map[string]HealthChecker),
		healers:    make(map[string]SelfHealer),
		results:    make(map[string]*HealthCheckResult),
		healingLog: make(map[string][]HealingAttempt),
		logger:     logger,
		metrics:    metrics,
		stopCh:     make(chan struct{}),
	}
}

// RegisterChecker adds a health checker
func (hm *HealthMonitor) RegisterChecker(checker HealthChecker) {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	key := fmt.Sprintf("%s.%s", checker.Component(), checker.Subsystem())
	hm.checkers[key] = checker

	// Initialize result
	hm.results[key] = &HealthCheckResult{
		Component: checker.Component(),
		Subsystem: checker.Subsystem(),
		Status:    HealthUnknown,
	}
}

// RegisterHealer adds a self-healer
func (hm *HealthMonitor) RegisterHealer(component, subsystem string, healer SelfHealer) {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	key := fmt.Sprintf("%s.%s", component, subsystem)
	hm.healers[key] = healer
}

// Start begins health monitoring
func (hm *HealthMonitor) Start(ctx context.Context) {
	hm.wg.Add(1)
	go hm.monitoringLoop(ctx)
}

// Stop halts health monitoring
func (hm *HealthMonitor) Stop() {
	close(hm.stopCh)
	hm.wg.Wait()
}

// GetOverallHealth returns the overall system health
func (hm *HealthMonitor) GetOverallHealth() HealthStatus {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	if len(hm.results) == 0 {
		return HealthUnknown
	}

	healthyCount := 0
	degradedCount := 0
	unhealthyCount := 0

	for _, result := range hm.results {
		switch result.Status {
		case HealthHealthy:
			healthyCount++
		case HealthDegraded:
			degradedCount++
		case HealthUnhealthy:
			unhealthyCount++
		}
	}

	// Overall health logic
	if unhealthyCount > 0 {
		return HealthUnhealthy
	}
	if degradedCount > 0 {
		return HealthDegraded
	}
	if healthyCount == len(hm.results) {
		return HealthHealthy
	}

	return HealthUnknown
}

// GetComponentHealth returns health for a specific component
func (hm *HealthMonitor) GetComponentHealth(component, subsystem string) *HealthCheckResult {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	key := fmt.Sprintf("%s.%s", component, subsystem)
	if result, exists := hm.results[key]; exists {
		// Return a copy to avoid concurrent access issues
		resultCopy := *result
		return &resultCopy
	}

	return nil
}

// GetAllHealth returns all health check results
func (hm *HealthMonitor) GetAllHealth() map[string]*HealthCheckResult {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	results := make(map[string]*HealthCheckResult)
	for key, result := range hm.results {
		resultCopy := *result
		results[key] = &resultCopy
	}

	return results
}

// Stub for GetAllHealthResults on HealthMonitor
func (hm *HealthMonitor) GetAllHealthResults() map[string]*HealthCheckResult {
	hm.mu.RLock()
	defer hm.mu.RUnlock()
	results := make(map[string]*HealthCheckResult)
	for k, v := range hm.results {
		results[k] = v
	}
	return results
}

// GetHealingLog returns healing attempts for a component
func (hm *HealthMonitor) GetHealingLog(component, subsystem string) []HealingAttempt {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	key := fmt.Sprintf("%s.%s", component, subsystem)
	if log, exists := hm.healingLog[key]; exists {
		// Return a copy
		logCopy := make([]HealingAttempt, len(log))
		copy(logCopy, log)
		return logCopy
	}

	return nil
}

// monitoringLoop runs the main health check loop
func (hm *HealthMonitor) monitoringLoop(ctx context.Context) {
	defer hm.wg.Done()

	ticker := time.NewTicker(hm.config.Interval)
	defer ticker.Stop()

	// Run initial health checks
	hm.runHealthChecks(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-hm.stopCh:
			return
		case <-ticker.C:
			hm.runHealthChecks(ctx)
		}
	}
}

// runHealthChecks executes all registered health checks
func (hm *HealthMonitor) runHealthChecks(ctx context.Context) {
	hm.mu.RLock()
	checkers := make(map[string]HealthChecker)
	for k, v := range hm.checkers {
		checkers[k] = v
	}
	hm.mu.RUnlock()

	for key, checker := range checkers {
		go hm.runSingleHealthCheck(ctx, key, checker)
	}
}

// runSingleHealthCheck executes a single health check with timeout
func (hm *HealthMonitor) runSingleHealthCheck(ctx context.Context, key string, checker HealthChecker) {
	checkCtx, cancel := context.WithTimeout(ctx, hm.config.Timeout)
	defer cancel()

	traceID := NewTraceID()
	checkCtx = WithTraceID(checkCtx, traceID)

	result := checker.Check(checkCtx)
	result.LastChecked = time.Now()

	hm.mu.Lock()
	if existingResult, exists := hm.results[key]; exists {
		result.CheckCount = existingResult.CheckCount + 1
		if result.Status != HealthHealthy {
			result.FailCount = existingResult.FailCount + 1
		}
	} else {
		result.CheckCount = 1
		if result.Status != HealthHealthy {
			result.FailCount = 1
		}
	}

	hm.results[key] = &result
	hm.mu.Unlock()

	// Update metrics
	healthy := result.Status == HealthHealthy
	// Comment out or stub UpdateComponentHealth
	// if hm.metrics != nil {
	// 	hm.metrics.UpdateComponentHealth(result.Component, result.Subsystem, healthy)
	// }

	// Log health check result
	hm.logger.LogSystemEvent(checkCtx, result.Component, "health_check", result.Status.String(), map[string]interface{}{
		"subsystem":   result.Subsystem,
		"check_count": result.CheckCount,
		"fail_count":  result.FailCount,
		"message":     result.Message,
		"details":     result.Details,
	})

	// Attempt self-healing if configured and needed
	if hm.config.EnableSelfHealing && result.Status != HealthHealthy {
		hm.attemptSelfHealing(ctx, key, result)
	}
}

// attemptSelfHealing tries to heal an unhealthy component
func (hm *HealthMonitor) attemptSelfHealing(ctx context.Context, key string, result HealthCheckResult) {
	hm.mu.RLock()
	healer, hasHealer := hm.healers[key]
	hm.mu.RUnlock()

	if !hasHealer {
		return
	}

	if !healer.CanHeal(result) {
		return
	}

	// Check if we're in cooldown period
	hm.mu.RLock()
	attempts := hm.healingLog[key]
	hm.mu.RUnlock()

	if len(attempts) > 0 {
		lastAttempt := attempts[len(attempts)-1]
		if time.Since(lastAttempt.Timestamp) < hm.config.HealingCooldown {
			return
		}
	}

	// Check max attempts
	recentAttempts := 0
	cutoff := time.Now().Add(-time.Hour) // Count attempts in last hour
	for _, attempt := range attempts {
		if attempt.Timestamp.After(cutoff) {
			recentAttempts++
		}
	}

	if recentAttempts >= hm.config.MaxHealingAttempts {
		return
	}

	// Attempt healing
	healingCtx, cancel := context.WithTimeout(ctx, hm.config.Timeout)
	defer cancel()

	traceID := NewTraceID()
	healingCtx = WithTraceID(healingCtx, traceID)

	attempt := HealingAttempt{
		Timestamp:   time.Now(),
		Component:   result.Component,
		Subsystem:   result.Subsystem,
		Description: healer.HealingDescription(),
	}

	hm.logger.LogSystemEvent(healingCtx, result.Component, "self_healing_attempt", "info", map[string]interface{}{
		"subsystem":   result.Subsystem,
		"description": attempt.Description,
	})

	err := healer.Heal(healingCtx, result)
	if err != nil {
		attempt.Error = err.Error()
		hm.logger.LogSystemEvent(healingCtx, result.Component, "self_healing_failed", "error", map[string]interface{}{
			"subsystem": result.Subsystem,
			"error":     err.Error(),
		})
	} else {
		attempt.Success = true
		hm.logger.LogSystemEvent(healingCtx, result.Component, "self_healing_success", "info", map[string]interface{}{
			"subsystem": result.Subsystem,
		})
	}

	// Record the attempt
	hm.mu.Lock()
	if hm.healingLog[key] == nil {
		hm.healingLog[key] = make([]HealingAttempt, 0)
	}
	hm.healingLog[key] = append(hm.healingLog[key], attempt)

	// Keep only last 100 attempts
	if len(hm.healingLog[key]) > 100 {
		hm.healingLog[key] = hm.healingLog[key][len(hm.healingLog[key])-100:]
	}
	hm.mu.Unlock()
}

// Specific health checkers for MarketMaker components

// TradingAPIHealthChecker monitors trading API health
type TradingAPIHealthChecker struct {
	api TradingAPI
}

func NewTradingAPIHealthChecker(api TradingAPI) *TradingAPIHealthChecker {
	return &TradingAPIHealthChecker{api: api}
}

func (t *TradingAPIHealthChecker) Name() string      { return "TradingAPI" }
func (t *TradingAPIHealthChecker) Component() string { return "marketmaker" }
func (t *TradingAPIHealthChecker) Subsystem() string { return "trading_api" }

func (t *TradingAPIHealthChecker) Check(ctx context.Context) HealthCheckResult {
	result := HealthCheckResult{
		Component: t.Component(),
		Subsystem: t.Subsystem(),
		Details:   make(map[string]interface{}),
	}

	// Test account balance retrieval
	startTime := time.Now()
	balance, err := t.api.GetAccountBalance()
	latency := time.Since(startTime)

	result.Details["balance_check_latency_ms"] = latency.Milliseconds()

	if err != nil {
		result.Status = HealthUnhealthy
		result.Message = fmt.Sprintf("Failed to get account balance: %v", err)

	result.Details["account_balance"] = balance

	// Check if latency is reasonable
	if latency > 5*time.Second {
		result.Status = HealthDegraded
		result.Message = fmt.Sprintf("High latency: %v", latency)
	} else {
		result.Status = HealthHealthy
		result.Message = "Trading API responsive"
	}

	return result
}

// FeedHealthChecker monitors market data feeds
type FeedHealthChecker struct {
	feedType           string
	provider           string
	getLastMessageTime func() time.Time
}

func NewFeedHealthChecker(feedType, provider string, getLastMessageTime func() time.Time) *FeedHealthChecker {
	return &FeedHealthChecker{
		feedType:           feedType,
		provider:           provider,
		getLastMessageTime: getLastMessageTime,
	}
}

func (f *FeedHealthChecker) Name() string      { return "FeedHealth" }
func (f *FeedHealthChecker) Component() string { return "marketmaker" }
func (f *FeedHealthChecker) Subsystem() string {
	return fmt.Sprintf("feed_%s_%s", f.feedType, f.provider)
}

func (f *FeedHealthChecker) Check(ctx context.Context) HealthCheckResult {
	result := HealthCheckResult{
		Component: f.Component(),
		Subsystem: f.Subsystem(),
		Details:   make(map[string]interface{}),
	}

	lastMessageTime := f.getLastMessageTime()
	timeSinceLastMessage := time.Since(lastMessageTime)

	result.Details["last_message_time"] = lastMessageTime
	result.Details["time_since_last_message_ms"] = timeSinceLastMessage.Milliseconds()

	if timeSinceLastMessage > 30*time.Second {
		result.Status = HealthUnhealthy
		result.Message = fmt.Sprintf("No messages for %v", timeSinceLastMessage)
	} else if timeSinceLastMessage > 10*time.Second {
		result.Status = HealthDegraded
		result.Message = fmt.Sprintf("Delayed messages: %v", timeSinceLastMessage)
	} else {
		result.Status = HealthHealthy
		result.Message = "Feed receiving messages"
	}

	return result
}

// StrategyHealthChecker monitors strategy health
type StrategyHealthChecker struct {
	strategy common.MarketMakingStrategy
	pair     string
}

func NewStrategyHealthChecker(strategy common.MarketMakingStrategy, pair string) *StrategyHealthChecker {
	return &StrategyHealthChecker{
		strategy: strategy,
		pair:     pair,
	}
}

func (s *StrategyHealthChecker) Name() string      { return "Strategy" }
func (s *StrategyHealthChecker) Component() string { return "marketmaker" }
func (s *StrategyHealthChecker) Subsystem() string { return fmt.Sprintf("strategy_%s", s.pair) }

func (s *StrategyHealthChecker) Check(ctx context.Context) HealthCheckResult {
	result := HealthCheckResult{
		Component: s.Component(),
		Subsystem: s.Subsystem(),
		Details:   make(map[string]interface{}),
	}

	// Check if strategy is responsive (this would depend on strategy interface)
	// For now, assume all strategies are healthy if they exist
	if s.strategy != nil {
		result.Status = HealthHealthy
		result.Message = "Strategy active"
	} else {
		result.Status = HealthUnhealthy
		result.Message = "Strategy not found"
	}

	return result
}

// RiskManagerHealthChecker monitors risk management system
type RiskManagerHealthChecker struct {
	riskManager *RiskManager
}

func NewRiskManagerHealthChecker(riskManager *RiskManager) *RiskManagerHealthChecker {
	return &RiskManagerHealthChecker{riskManager: riskManager}
}

func (r *RiskManagerHealthChecker) Name() string      { return "RiskManager" }
func (r *RiskManagerHealthChecker) Component() string { return "marketmaker" }
func (r *RiskManagerHealthChecker) Subsystem() string { return "risk_manager" }

func (r *RiskManagerHealthChecker) Check(ctx context.Context) HealthCheckResult {
	result := HealthCheckResult{
		Component: r.Component(),
		Subsystem: r.Subsystem(),
		Details:   make(map[string]interface{}),
	}

	if r.riskManager == nil {
		result.Status = HealthUnhealthy
		result.Message = "Risk manager not initialized"
		return result
	}

	// Check risk levels
	riskSignals := r.riskManager.GetRiskSignals(int(CriticalRisk))
	criticalRiskCount := len(riskSignals)

	result.Details["critical_risk_signals"] = criticalRiskCount

	if criticalRiskCount > 0 {
		result.Status = HealthUnhealthy
		result.Message = fmt.Sprintf("%d critical risk signals active", criticalRiskCount)
	} else {
		highRiskSignals := r.riskManager.GetRiskSignals(int(HighRisk))
		if len(highRiskSignals) > 0 {
			result.Status = HealthDegraded
			result.Message = fmt.Sprintf("%d high risk signals active", len(highRiskSignals))
		} else {
			result.Status = HealthHealthy
			result.Message = "Risk levels normal"
		}
	}

	return result
}
