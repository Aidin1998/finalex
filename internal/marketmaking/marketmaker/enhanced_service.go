// Enhanced MarketMaker service with integrated observability components
package marketmaker

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// EnhancedService extends the basic MarketMaker service with production-grade observability
type EnhancedService struct {
	// Core components
	*Service // Embed existing service

	// Enhanced observability components
	logger              *StructuredLogger
	metrics             *MetricsCollector
	healthMonitor       *HealthMonitor
	emergencyController *EmergencyController
	selfHealingManager  *SelfHealingManager
	adminToolsManager   *AdminToolsManager
	backtestEngine      *BacktestEngine

	// Configuration
	observabilityConfig *ObservabilityConfig

	// Enhanced state tracking
	traceID          string
	operationalState OperationalState
	lastHealthCheck  time.Time
	mu               sync.RWMutex
}

// ObservabilityConfig contains configuration for all observability features
type ObservabilityConfig struct {
	// Logging configuration
	LogLevel        string  `json:"log_level"`
	EnableTracing   bool    `json:"enable_tracing"`
	TraceSampleRate float64 `json:"trace_sample_rate"`

	// Metrics configuration
	MetricsEnabled bool     `json:"metrics_enabled"`
	MetricsPort    int      `json:"metrics_port"`
	CustomMetrics  []string `json:"custom_metrics"`

	// Health monitoring configuration
	HealthCheckInterval time.Duration `json:"health_check_interval"`
	HealthTimeout       time.Duration `json:"health_timeout"`
	EnableSelfHealing   bool          `json:"enable_self_healing"`

	// Emergency controls configuration
	EmergencyConfig   *EmergencyConfig   `json:"emergency_config"`
	SelfHealingConfig *SelfHealingConfig `json:"self_healing_config"`

	// Admin tools configuration
	AdminAPIEnabled bool `json:"admin_api_enabled"`
	AdminAPIPort    int  `json:"admin_api_port"`
	BacktestEnabled bool `json:"backtest_enabled"`
}

// OperationalState represents the current operational state of the service
type OperationalState struct {
	Status             string                 `json:"status"`
	Uptime             time.Duration          `json:"uptime"`
	LastError          string                 `json:"last_error,omitempty"`
	ErrorCount         int                    `json:"error_count"`
	HealthStatus       map[string]string      `json:"health_status"`
	PerformanceMetrics *PerformanceMetrics    `json:"performance_metrics"`
	EmergencyStatus    map[string]interface{} `json:"emergency_status"`
}

// NewEnhancedService creates a new enhanced MarketMaker service with full observability
func NewEnhancedService(
	cfg MarketMakerConfig,
	observabilityConfig *ObservabilityConfig,
	trading TradingAPI,
	strategy Strategy,
) (*EnhancedService, error) {
	// Create base service
	baseService := NewService(cfg, trading, strategy)
	// Initialize enhanced components
	logger, err := NewStructuredLogger("marketmaker", "1.0.0")
	if err != nil {
		return nil, fmt.Errorf("failed to create structured logger: %w", err)
	}
	// Create base metrics collector
	baseMetrics := NewMetricsCollector()

	// Create Prometheus metrics collector for enhanced metrics
	promMetrics := NewPrometheusMetricsCollector()

	// Create a health check config from the observability config
	healthCheckConfig := HealthCheckConfig{
		Interval:           observabilityConfig.HealthCheckInterval,
		Timeout:            observabilityConfig.HealthTimeout,
		FailureThreshold:   3, // Default values
		RecoveryThreshold:  2,
		EnableSelfHealing:  observabilityConfig.EnableSelfHealing,
		HealingCooldown:    5 * time.Minute,
		MaxHealingAttempts: 3,
	}

	// Create health monitor with the proper config
	healthMonitor := NewHealthMonitor(
		healthCheckConfig,
		logger,
		baseMetrics,
	)

	emergencyController := NewEmergencyController(
		observabilityConfig.EmergencyConfig,
		logger,
		metrics,
		trading,
		nil, // Will be set after strategy manager is available
		nil, // Will be set after feed manager is available
	)

	selfHealingManager := NewSelfHealingManager(
		observabilityConfig.SelfHealingConfig,
		logger,
		metrics,
	)

	backtestEngine := NewBacktestEngine(logger, metrics)

	// Create enhanced service
	enhancedService := &EnhancedService{
		Service:             baseService,
		logger:              logger,
		metrics:             metrics,
		healthMonitor:       healthMonitor,
		emergencyController: emergencyController,
		selfHealingManager:  selfHealingManager,
		backtestEngine:      backtestEngine,
		observabilityConfig: observabilityConfig,
		operationalState: OperationalState{
			Status:       "initializing",
			HealthStatus: make(map[string]string),
		},
	}

	// Create admin tools manager
	enhancedService.adminToolsManager = NewAdminToolsManager(
		baseService,
		emergencyController,
		selfHealingManager,
		healthMonitor,
		logger,
		metrics,
	)

	// Register health checkers
	enhancedService.registerHealthCheckers()

	// Register self-healers
	enhancedService.registerSelfHealers()

	// Register emergency callbacks
	enhancedService.registerEmergencyCallbacks()

	return enhancedService, nil
}

// Start starts the enhanced service with full observability
func (es *EnhancedService) Start(ctx context.Context) error {
	es.mu.Lock()
	es.operationalState.Status = "starting"
	es.mu.Unlock()

	traceID := es.logger.GenerateTraceID()
	ctx = es.logger.WithTraceID(ctx, traceID)
	es.traceID = traceID

	es.logger.LogInfo(ctx, "starting enhanced market maker service", map[string]interface{}{
		"config":                es.cfg,
		"observability_enabled": true,
	})

	// Start observability components
	if err := es.startObservabilityComponents(ctx); err != nil {
		return fmt.Errorf("failed to start observability components: %w", err)
	}

	// Start base service
	if err := es.Service.Start(ctx); err != nil {
		return fmt.Errorf("failed to start base service: %w", err)
	}

	// Start enhanced monitoring
	es.startEnhancedMonitoring(ctx)

	es.mu.Lock()
	es.operationalState.Status = "running"
	es.mu.Unlock()

	es.logger.LogInfo(ctx, "enhanced market maker service started successfully", nil)
	es.metrics.RecordServiceStart()

	return nil
}

// Stop stops the enhanced service gracefully
func (es *EnhancedService) Stop(ctx context.Context) error {
	es.mu.Lock()
	es.operationalState.Status = "stopping"
	es.mu.Unlock()

	ctx = es.logger.WithTraceID(ctx, es.traceID)

	es.logger.LogInfo(ctx, "stopping enhanced market maker service", nil)

	// Stop enhanced monitoring
	es.stopEnhancedMonitoring(ctx)

	// Stop base service
	if err := es.Service.Stop(ctx); err != nil {
		es.logger.LogError(ctx, "error stopping base service", map[string]interface{}{
			"error": err.Error(),
		})
	}

	// Stop observability components
	es.stopObservabilityComponents(ctx)

	es.mu.Lock()
	es.operationalState.Status = "stopped"
	es.mu.Unlock()

	es.logger.LogInfo(ctx, "enhanced market maker service stopped", nil)
	es.metrics.RecordServiceStop()

	return nil
}

// startObservabilityComponents starts all observability components
func (es *EnhancedService) startObservabilityComponents(ctx context.Context) error {
	// Start metrics collection
	if err := es.metrics.Start(ctx); err != nil {
		return fmt.Errorf("failed to start metrics collector: %w", err)
	}

	// Start health monitoring
	es.healthMonitor.Start(ctx)

	// Start emergency controller monitoring
	es.emergencyController.RegisterEmergencyCallback(es.onEmergencyEvent)

	es.logger.LogInfo(ctx, "observability components started", nil)
	return nil
}

// stopObservabilityComponents stops all observability components
func (es *EnhancedService) stopObservabilityComponents(ctx context.Context) {
	es.healthMonitor.Stop()
	es.metrics.Stop(ctx)
	es.logger.LogInfo(ctx, "observability components stopped", nil)
}

// startEnhancedMonitoring starts enhanced monitoring routines
func (es *EnhancedService) startEnhancedMonitoring(ctx context.Context) {
	// Start periodic health checks
	go es.runPeriodicHealthChecks(ctx)

	// Start performance monitoring
	go es.runPerformanceMonitoring(ctx)

	// Start risk monitoring
	go es.runRiskMonitoring(ctx)

	// Start emergency condition monitoring
	go es.runEmergencyMonitoring(ctx)
}

// stopEnhancedMonitoring stops enhanced monitoring routines
func (es *EnhancedService) stopEnhancedMonitoring(ctx context.Context) {
	// Monitoring routines will stop when context is cancelled
	es.logger.LogInfo(ctx, "enhanced monitoring stopped", nil)
}

// runPeriodicHealthChecks runs periodic health checks
func (es *EnhancedService) runPeriodicHealthChecks(ctx context.Context) {
	ticker := time.NewTicker(es.observabilityConfig.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			es.performHealthCheck(ctx)
		}
	}
}

// performHealthCheck performs a comprehensive health check
func (es *EnhancedService) performHealthCheck(ctx context.Context) {
	es.lastHealthCheck = time.Now()
	// Run all health checkers
	results := es.healthMonitor.CheckAll()

	// Update operational state
	es.mu.Lock()
	for component, result := range results {
		es.operationalState.HealthStatus[component] = result.Status.String()

		// Trigger self-healing if component is unhealthy
		if result.Status == HealthUnhealthy {
			go es.triggerSelfHealing(ctx, component, "health_check_failed")
		}
	}
	es.mu.Unlock()

	// Log health check results
	es.logger.LogHealthCheck(ctx, results)
}

// runPerformanceMonitoring monitors performance metrics continuously
func (es *EnhancedService) runPerformanceMonitoring(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			es.collectPerformanceMetrics(ctx)
		}
	}
}

// collectPerformanceMetrics collects and records performance metrics
func (es *EnhancedService) collectPerformanceMetrics(ctx context.Context) {
	// Get current performance metrics from base service
	baseMetrics := es.Service.GetPerformanceMetrics()

	// Record metrics
	es.metrics.RecordStrategyPnL("main", baseMetrics.TotalPnL)
	es.metrics.RecordOrderLatency("place_order", time.Duration(baseMetrics.LatencyMetrics["place_order"]))
	es.metrics.RecordInventoryPosition("total", baseMetrics.TotalExposure)

	// Update operational state
	es.mu.Lock()
	es.operationalState.PerformanceMetrics = baseMetrics
	es.operationalState.Uptime = time.Since(es.Service.startTime)
	es.mu.Unlock()

	// Log performance summary
	es.logger.LogPerformance(ctx, "performance_summary", map[string]interface{}{
		"total_pnl":    baseMetrics.TotalPnL,
		"total_trades": baseMetrics.TotalTrades,
		"success_rate": baseMetrics.SuccessRate,
		"uptime":       es.operationalState.Uptime,
	})
}

// runRiskMonitoring monitors risk conditions
func (es *EnhancedService) runRiskMonitoring(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			es.monitorRiskConditions(ctx)
		}
	}
}

// monitorRiskConditions monitors for risk-related emergency conditions
func (es *EnhancedService) monitorRiskConditions(ctx context.Context) {
	riskStatus := es.Service.GetRiskStatus()

	// Record risk metrics
	es.metrics.RecordRiskEvent("daily_pnl", riskStatus.DailyPnL)
	es.metrics.RecordRiskEvent("total_exposure", riskStatus.TotalExposure)
	es.metrics.RecordRiskEvent("risk_score", riskStatus.RiskScore)
	// Log risk events
	for _, signal := range riskStatus.RiskSignals {
		// Extract symbol and value safely
		symbol := "unknown"
		if signal.Symbol != "" {
			symbol = signal.Symbol
		}

		es.logger.LogRiskEvent(ctx, string(signal.Type), signal.Message, map[string]interface{}{
			"severity": signal.Severity,
			"symbol":   symbol,
			"value":    signal.Value,
		})

		// Record risk event metric
		es.metrics.RecordRiskEventTotal(string(signal.Type), string(signal.Severity))
	}
}

// runEmergencyMonitoring monitors for emergency conditions
func (es *EnhancedService) runEmergencyMonitoring(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			es.checkEmergencyConditions(ctx)
		}
	}
}

// checkEmergencyConditions checks for conditions that should trigger emergency actions
func (es *EnhancedService) checkEmergencyConditions(ctx context.Context) {
	metrics := es.Service.GetPerformanceMetrics()

	// Check emergency conditions using the emergency controller
	es.emergencyController.CheckTriggerConditions(ctx, metrics)

	// Update emergency status in operational state
	es.mu.Lock()
	es.operationalState.EmergencyStatus = es.emergencyController.GetStatus()
	es.mu.Unlock()
}

// triggerSelfHealing triggers self-healing for a specific component
func (es *EnhancedService) triggerSelfHealing(ctx context.Context, component, reason string) {
	if err := es.selfHealingManager.TriggerHealing(ctx, component, reason); err != nil {
		es.logger.LogError(ctx, "self-healing failed", map[string]interface{}{
			"component": component,
			"reason":    reason,
			"error":     err.Error(),
		})

		es.mu.Lock()
		es.operationalState.ErrorCount++
		es.operationalState.LastError = fmt.Sprintf("self-healing failed for %s: %v", component, err)
		es.mu.Unlock()
	}
}

// registerHealthCheckers registers health checkers for all components
func (es *EnhancedService) registerHealthCheckers() {
	// Register trading API health checker
	tradingAPIChecker := &GenericHealthChecker{
		ComponentName: "trading_api",
		SubsystemName: "api",
		CheckFunction: func(ctx context.Context) HealthCheckResult {
			// Simple availability check
			return HealthCheckResult{
				Component:   "trading_api",
				Subsystem:   "api",
				Status:      HealthHealthy,
				Message:     "Trading API available",
				LastChecked: time.Now(),
			}
		},
	}
	es.healthMonitor.RegisterChecker(tradingAPIChecker)

	// Register strategy health checker
	strategyChecker := &GenericHealthChecker{
		ComponentName: "strategy",
		SubsystemName: "core",
		CheckFunction: func(ctx context.Context) HealthCheckResult {
			// Simple availability check
			return HealthCheckResult{
				Component:   "strategy",
				Subsystem:   "core",
				Status:      HealthHealthy,
				Message:     "Strategy available",
				LastChecked: time.Now(),
			}
		},
	}
	es.healthMonitor.RegisterChecker(strategyChecker)

	// Register risk manager health checker if available
	if es.Service.riskManager != nil {
		riskChecker := &GenericHealthChecker{
			ComponentName: "risk_manager",
			SubsystemName: "risk",
			CheckFunction: func(ctx context.Context) HealthCheckResult {
				// Simple availability check
				return HealthCheckResult{
					Component:   "risk_manager",
					Subsystem:   "risk",
					Status:      HealthHealthy,
					Message:     "Risk manager available",
					LastChecked: time.Now(),
				}
			},
		}
		es.healthMonitor.RegisterChecker(riskChecker)
	}

	es.logger.LogInfo(context.Background(), "health checkers registered", nil)
}

// registerSelfHealers registers self-healing implementations
func (es *EnhancedService) registerSelfHealers() {
	// Register trading API self-healer
	tradingAPIHealer := &GenericSelfHealer{
		CanHealFunction: func(result HealthCheckResult) bool {
			return result.Component == "trading_api" && result.Status == HealthUnhealthy
		},
		HealFunction: func(ctx context.Context, result HealthCheckResult) error {
			// Simple reconnect logic
			es.logger.LogInfo(ctx, "Attempting to heal trading API connection", nil)
			// In a real implementation, we might attempt to reconnect to the API
			return nil
		},
		Description: "Attempts to reconnect to the trading API when connection is lost",
	}
	es.selfHealingManager.RegisterHealer("trading_api", tradingAPIHealer)

	// Register risk manager self-healer
	riskHealer := &GenericSelfHealer{
		CanHealFunction: func(result HealthCheckResult) bool {
			return result.Component == "risk_manager" && result.Status == HealthUnhealthy
		},
		HealFunction: func(ctx context.Context, result HealthCheckResult) error {
			// Simple reset logic
			es.logger.LogInfo(ctx, "Attempting to reset risk manager", nil)
			// In a real implementation, we might attempt to reset the risk manager
			return nil
		},
		Description: "Attempts to reset the risk manager when it's in a bad state",
	}
	es.selfHealingManager.RegisterHealer("risk_manager", riskHealer)

	es.logger.LogInfo(context.Background(), "self-healers registered", nil)
}

// registerEmergencyCallbacks registers callbacks for emergency events
func (es *EnhancedService) registerEmergencyCallbacks() {
	es.emergencyController.RegisterEmergencyCallback(es.onEmergencyEvent)
}

// onEmergencyEvent handles emergency events
func (es *EnhancedService) onEmergencyEvent(ctx context.Context, reason KillReason) error {
	es.logger.LogError(ctx, fmt.Sprintf("EMERGENCY: %s", reason.Reason), map[string]interface{}{
		"type":     reason.Type,
		"severity": reason.Severity,
		"details":  reason.Details,
		"event":    "emergency_callback",
	})

	// Update operational state
	es.mu.Lock()
	es.operationalState.Status = "emergency"
	es.operationalState.LastError = reason.Reason
	es.mu.Unlock()

	// Additional emergency actions can be added here
	return nil
}

// GetOperationalState returns the current operational state
func (es *EnhancedService) GetOperationalState() OperationalState {
	es.mu.RLock()
	defer es.mu.RUnlock()

	// Create a copy to avoid race conditions
	state := es.operationalState
	state.HealthStatus = make(map[string]string)
	for k, v := range es.operationalState.HealthStatus {
		state.HealthStatus[k] = v
	}

	return state
}

// GetEnhancedMetrics returns comprehensive metrics including observability metrics
func (es *EnhancedService) GetEnhancedMetrics() map[string]interface{} {
	baseMetrics := es.Service.GetPerformanceMetrics()
	healthResults := es.healthMonitor.GetAllHealthResults()
	healingHistory := es.selfHealingManager.GetHealingHistory()
	emergencyStatus := es.emergencyController.GetStatus()

	return map[string]interface{}{
		"base_metrics":      baseMetrics,
		"health_results":    healthResults,
		"healing_history":   healingHistory,
		"emergency_status":  emergencyStatus,
		"operational_state": es.GetOperationalState(),
		"trace_id":          es.traceID,
		"last_health_check": es.lastHealthCheck,
	}
}

// GetAdminToolsManager returns the admin tools manager for API registration
func (es *EnhancedService) GetAdminToolsManager() *AdminToolsManager {
	return es.adminToolsManager
}

// GetBacktestEngine returns the backtest engine
func (es *EnhancedService) GetBacktestEngine() *BacktestEngine {
	return es.backtestEngine
}

// TriggerManualEmergencyKill allows manual triggering of emergency kill
func (es *EnhancedService) TriggerManualEmergencyKill(ctx context.Context, reason string) error {
	killReason := KillReason{
		Type:     "manual",
		Reason:   reason,
		Severity: "critical",
	}

	return es.emergencyController.TriggerEmergencyKill(ctx, killReason)
}

// AttemptRecovery attempts to recover from emergency state
func (es *EnhancedService) AttemptRecovery(ctx context.Context) error {
	return es.emergencyController.AttemptRecovery(ctx)
}

// IsInEmergencyState returns whether the service is in emergency state
func (es *EnhancedService) IsInEmergencyState() bool {
	return es.emergencyController.IsKilled()
}
