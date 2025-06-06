// Enhanced admin tools for MarketMaker strategy lifecycle management
package marketmaker

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Aidin1998/finalex/internal/marketmaking/strategies/common"
)

// AdminToolsManager provides comprehensive admin functionality
type AdminToolsManager struct {
	marketMaker         *Service
	emergencyController *EmergencyController
	selfHealingManager  *SelfHealingManager
	healthMonitor       *HealthMonitor
	logger              *StructuredLogger
	metrics             *MetricsCollector
	mu                  sync.RWMutex

	// Strategy management
	strategies        map[string]*StrategyInstance
	strategyTemplates map[string]*StrategyTemplate
}

// StrategyInstance represents a running strategy instance
type StrategyInstance struct {
	ID          string                       `json:"id"`
	Name        string                       `json:"name"`
	Type        string                       `json:"type"`
	Status      StrategyStatus               `json:"status"`
	Config      map[string]interface{}       `json:"config"`
	Performance *StrategyInstancePerformance `json:"performance"`
	StartTime   time.Time                    `json:"start_time"`
	LastUpdate  time.Time                    `json:"last_update"`
	ErrorCount  int                          `json:"error_count"`
	LastError   string                       `json:"last_error,omitempty"`
	Strategy    common.MarketMakingStrategy  `json:"-"` // Not serialized, for runtime use
}

// StrategyStatus represents the status of a strategy
type StrategyStatus string

const (
	StrategyStatusStopped  StrategyStatus = "stopped"
	StrategyStatusStarting StrategyStatus = "starting"
	StrategyStatusRunning  StrategyStatus = "running"
	StrategyStatusStopping StrategyStatus = "stopping"
	StrategyStatusError    StrategyStatus = "error"
	StrategyStatusPaused   StrategyStatus = "paused"
)

// StrategyTemplate defines a strategy configuration template
type StrategyTemplate struct {
	Name          string                 `json:"name"`
	Type          string                 `json:"type"`
	Description   string                 `json:"description"`
	Parameters    []ParameterDefinition  `json:"parameters"`
	DefaultConfig map[string]interface{} `json:"default_config"`
}

// ParameterDefinition defines a strategy parameter
type ParameterDefinition struct {
	Name        string      `json:"name"`
	Type        string      `json:"type"`
	Description string      `json:"description"`
	Required    bool        `json:"required"`
	Default     interface{} `json:"default,omitempty"`
	Min         interface{} `json:"min,omitempty"`
	Max         interface{} `json:"max,omitempty"`
}

// StrategyInstancePerformance tracks strategy performance metrics for admin tools
// This avoids conflict with StrategyPerformance defined in service.go
type StrategyInstancePerformance struct {
	TotalPnL        float64       `json:"total_pnl"`
	DailyPnL        float64       `json:"daily_pnl"`
	OrdersPlaced    int64         `json:"orders_placed"`
	OrdersFilled    int64         `json:"orders_filled"`
	OrdersCancelled int64         `json:"orders_cancelled"`
	SuccessRate     float64       `json:"success_rate"`
	AvgFillTime     time.Duration `json:"avg_fill_time"`
	LastTrade       time.Time     `json:"last_trade"`
}

// NewAdminToolsManager creates a new admin tools manager
func NewAdminToolsManager(
	marketMaker *Service,
	emergencyController *EmergencyController,
	selfHealingManager *SelfHealingManager,
	healthMonitor *HealthMonitor,
	logger *StructuredLogger,
	metrics *MetricsCollector,
) *AdminToolsManager {
	atm := &AdminToolsManager{
		marketMaker:         marketMaker,
		emergencyController: emergencyController,
		selfHealingManager:  selfHealingManager,
		healthMonitor:       healthMonitor,
		logger:              logger,
		metrics:             metrics,
		strategies:          make(map[string]*StrategyInstance),
		strategyTemplates:   make(map[string]*StrategyTemplate),
	}

	// Initialize default strategy templates
	atm.initializeStrategyTemplates()

	return atm
}

// RegisterAdminRoutes registers all admin API routes
func (atm *AdminToolsManager) RegisterAdminRoutes(mux *http.ServeMux) {
	// Strategy management
	mux.HandleFunc("/admin/strategies", atm.handleStrategies)
	mux.HandleFunc("/admin/strategies/", atm.handleStrategyActions)
	mux.HandleFunc("/admin/strategies/templates", atm.handleStrategyTemplates)

	// Emergency controls
	mux.HandleFunc("/admin/emergency/kill", atm.handleEmergencyKill)
	mux.HandleFunc("/admin/emergency/status", atm.handleEmergencyStatus)
	mux.HandleFunc("/admin/emergency/recover", atm.handleEmergencyRecover)

	// Health and monitoring
	mux.HandleFunc("/admin/health", atm.handleHealthStatus)
	mux.HandleFunc("/admin/health/check", atm.handleHealthCheck)
	mux.HandleFunc("/admin/health/healing", atm.handleSelfHealing)

	// System controls
	mux.HandleFunc("/admin/system/status", atm.handleSystemStatus)
	mux.HandleFunc("/admin/system/metrics", atm.handleSystemMetrics)
	mux.HandleFunc("/admin/system/config", atm.handleSystemConfig)

	// Risk management
	mux.HandleFunc("/admin/risk/limits", atm.handleRiskLimits)
	// Use handleRiskLimits instead of handleRiskOverride for compatibility
	mux.HandleFunc("/admin/risk/override", atm.handleRiskLimits)

	// Backtesting
	mux.HandleFunc("/admin/backtest", atm.handleBacktest)
	mux.HandleFunc("/admin/backtest/", atm.handleBacktestActions)
}

// Strategy Management Handlers

func (atm *AdminToolsManager) handleStrategies(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		atm.listStrategies(w, r)
	case http.MethodPost:
		atm.createStrategy(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (atm *AdminToolsManager) listStrategies(w http.ResponseWriter, r *http.Request) {
	atm.mu.RLock()
	strategies := make([]*StrategyInstance, 0, len(atm.strategies))
	for _, strategy := range atm.strategies {
		strategies = append(strategies, strategy)
	}
	atm.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"strategies": strategies,
		"total":      len(strategies),
	})
}

func (atm *AdminToolsManager) createStrategy(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string                 `json:"name"`
		Type     string                 `json:"type"`
		Template string                 `json:"template,omitempty"`
		Config   map[string]interface{} `json:"config"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.Name == "" || req.Type == "" {
		http.Error(w, "Name and type are required", http.StatusBadRequest)
		return
	}

	// Check if strategy already exists
	atm.mu.RLock()
	if _, exists := atm.strategies[req.Name]; exists {
		atm.mu.RUnlock()
		http.Error(w, "Strategy already exists", http.StatusConflict)
		return
	}
	atm.mu.RUnlock()

	// Create strategy instance
	strategy := &StrategyInstance{
		ID:          req.Name,
		Name:        req.Name,
		Type:        req.Type,
		Status:      StrategyStatusStopped,
		Config:      req.Config,
		Performance: &StrategyInstancePerformance{},
		StartTime:   time.Now(),
		LastUpdate:  time.Now(),
	}

	// Apply template if specified
	if req.Template != "" {
		if err := atm.applyStrategyTemplate(strategy, req.Template); err != nil {
			http.Error(w, fmt.Sprintf("Failed to apply template: %v", err), http.StatusBadRequest)
			return
		}
	}

	atm.mu.Lock()
	atm.strategies[req.Name] = strategy
	atm.mu.Unlock()

	atm.logger.LogInfo(r.Context(), "strategy created", map[string]interface{}{
		"strategy_name": req.Name,
		"strategy_type": req.Type,
		"template":      req.Template,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(strategy)
}

func (atm *AdminToolsManager) handleStrategyActions(w http.ResponseWriter, r *http.Request) {
	// Extract strategy name from URL path
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/admin/strategies/"), "/")
	if len(pathParts) < 1 {
		http.Error(w, "Strategy name required", http.StatusBadRequest)
		return
	}

	strategyName := pathParts[0]
	action := ""
	if len(pathParts) > 1 {
		action = pathParts[1]
	}

	atm.mu.RLock()
	strategy, exists := atm.strategies[strategyName]
	atm.mu.RUnlock()

	if !exists {
		http.Error(w, "Strategy not found", http.StatusNotFound)
		return
	}

	switch r.Method {
	case http.MethodGet:
		atm.getStrategy(w, r, strategy)
	case http.MethodPut:
		atm.updateStrategy(w, r, strategy)
	case http.MethodDelete:
		atm.deleteStrategy(w, r, strategy)
	case http.MethodPost:
		atm.executeStrategyAction(w, r, strategy, action)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (atm *AdminToolsManager) getStrategy(w http.ResponseWriter, r *http.Request, strategy *StrategyInstance) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(strategy)
}

// updateStrategy updates an existing strategy configuration
func (atm *AdminToolsManager) updateStrategy(w http.ResponseWriter, r *http.Request, strategy *StrategyInstance) {
	// Parse request body
	var update struct {
		Config map[string]interface{} `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Apply update
	atm.mu.Lock()
	strategy.Config = update.Config
	strategy.LastUpdate = time.Now()
	atm.mu.Unlock()

	// Response with updated strategy
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(strategy)

	// Log update
	atm.logger.LogInfo(r.Context(), "strategy updated", map[string]interface{}{
		"strategy_id": strategy.ID,
		"name":        strategy.Name,
	})
}

func (atm *AdminToolsManager) deleteStrategy(w http.ResponseWriter, r *http.Request, strategy *StrategyInstance) {
	atm.mu.Lock()
	defer atm.mu.Unlock()

	if _, exists := atm.strategies[strategy.ID]; !exists {
		http.Error(w, "Strategy not found", http.StatusNotFound)
		return
	}

	// TODO: Add any cleanup logic if necessary

	delete(atm.strategies, strategy.ID)

	w.WriteHeader(http.StatusNoContent)
	atm.logger.LogInfo(r.Context(), "strategy deleted", map[string]interface{}{
		"strategy_id": strategy.ID,
		"name":        strategy.Name,
	})
}

func (atm *AdminToolsManager) executeStrategyAction(w http.ResponseWriter, r *http.Request, strategy *StrategyInstance, action string) {
	ctx := r.Context()

	switch action {
	case "start":
		err := atm.startStrategy(ctx, strategy)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to start strategy: %v", err), http.StatusInternalServerError)
			return
		}
	case "stop":
		err := atm.stopStrategy(ctx, strategy)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to stop strategy: %v", err), http.StatusInternalServerError)
			return
		}
	case "restart":
		err := atm.restartStrategy(ctx, strategy)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to restart strategy: %v", err), http.StatusInternalServerError)
			return
		}
	case "pause":
		err := atm.pauseStrategy(ctx, strategy)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to pause strategy: %v", err), http.StatusInternalServerError)
			return
		}
	case "resume":
		err := atm.resumeStrategy(ctx, strategy)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to resume strategy: %v", err), http.StatusInternalServerError)
			return
		}
	default:
		http.Error(w, "Invalid action", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":   "success",
		"action":   action,
		"strategy": strategy,
	})
}

// Emergency Control Handlers

func (atm *AdminToolsManager) handleEmergencyKill(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Reason   string                 `json:"reason"`
		Type     string                 `json:"type"`
		Details  map[string]interface{} `json:"details"`
		Severity string                 `json:"severity"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.Reason == "" {
		http.Error(w, "Reason is required", http.StatusBadRequest)
		return
	}

	killReason := KillReason{
		Type:     req.Type,
		Reason:   req.Reason,
		Details:  req.Details,
		Severity: req.Severity,
	}

	if killReason.Type == "" {
		killReason.Type = "manual"
	}
	if killReason.Severity == "" {
		killReason.Severity = "critical"
	}

	err := atm.emergencyController.TriggerEmergencyKill(r.Context(), killReason)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to trigger emergency kill: %v", err), http.StatusInternalServerError)
		return
	}

	atm.logger.LogEmergencyEvent(r.Context(), "manual_emergency_kill", req.Reason, map[string]interface{}{
		"type":     req.Type,
		"severity": req.Severity,
		"details":  req.Details,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "success",
		"message": "Emergency kill triggered",
		"reason":  killReason,
	})
}

func (atm *AdminToolsManager) handleEmergencyStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	status := atm.emergencyController.GetStatus()
	killReasons := atm.emergencyController.GetKillReasons()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"emergency_status": status,
		"kill_history":     killReasons,
	})
}

func (atm *AdminToolsManager) handleEmergencyRecover(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	err := atm.emergencyController.AttemptRecovery(r.Context())
	if err != nil {
		http.Error(w, fmt.Sprintf("Recovery failed: %v", err), http.StatusInternalServerError)
		return
	}

	atm.logger.LogEmergencyEvent(r.Context(), "manual_recovery", "manual recovery initiated", nil)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "success",
		"message": "Recovery completed",
	})
}

// Strategy lifecycle operations

func (atm *AdminToolsManager) startStrategy(ctx context.Context, strategy *StrategyInstance) error {
	atm.mu.Lock()
	defer atm.mu.Unlock()

	if strategy.Status != StrategyStatusStopped && strategy.Status != StrategyStatusError {
		return fmt.Errorf("strategy is not in stopped state")
	}

	strategy.Status = StrategyStatusStarting
	strategy.LastUpdate = time.Now()

	// TODO: Actually start the strategy in the market maker
	// This would integrate with your strategy management system

	strategy.Status = StrategyStatusRunning
	strategy.StartTime = time.Now()

	atm.logger.LogStrategyEvent(ctx, strategy.Name, "strategy_started", nil)
	atm.metrics.RecordStrategyEvent(strategy.Name, "started")

	return nil
}

func (atm *AdminToolsManager) stopStrategy(ctx context.Context, strategy *StrategyInstance) error {
	atm.mu.Lock()
	defer atm.mu.Unlock()

	if strategy.Status != StrategyStatusRunning && strategy.Status != StrategyStatusPaused {
		return fmt.Errorf("strategy is not running")
	}

	strategy.Status = StrategyStatusStopping
	strategy.LastUpdate = time.Now()

	// TODO: Actually stop the strategy in the market maker

	strategy.Status = StrategyStatusStopped

	atm.logger.LogStrategyEvent(ctx, strategy.Name, "strategy_stopped", nil)
	atm.metrics.RecordStrategyEvent(strategy.Name, "stopped")

	return nil
}

func (atm *AdminToolsManager) restartStrategy(ctx context.Context, strategy *StrategyInstance) error {
	if err := atm.stopStrategy(ctx, strategy); err != nil {
		return err
	}

	// Brief pause before restart
	time.Sleep(1 * time.Second)

	return atm.startStrategy(ctx, strategy)
}

func (atm *AdminToolsManager) pauseStrategy(ctx context.Context, strategy *StrategyInstance) error {
	atm.mu.Lock()
	defer atm.mu.Unlock()

	if strategy.Status != StrategyStatusRunning {
		return fmt.Errorf("strategy is not running")
	}

	strategy.Status = StrategyStatusPaused
	strategy.LastUpdate = time.Now()

	atm.logger.LogStrategyEvent(ctx, strategy.Name, "strategy_paused", nil)
	atm.metrics.RecordStrategyEvent(strategy.Name, "paused")

	return nil
}

func (atm *AdminToolsManager) resumeStrategy(ctx context.Context, strategy *StrategyInstance) error {
	atm.mu.Lock()
	defer atm.mu.Unlock()

	if strategy.Status != StrategyStatusPaused {
		return fmt.Errorf("strategy is not paused")
	}

	strategy.Status = StrategyStatusRunning
	strategy.LastUpdate = time.Now()

	atm.logger.LogStrategyEvent(ctx, strategy.Name, "strategy_resumed", nil)
	atm.metrics.RecordStrategyEvent(strategy.Name, "resumed")

	return nil
}

// Helper methods

func (atm *AdminToolsManager) initializeStrategyTemplates() {
	// Basic market making template
	atm.strategyTemplates["basic_mm"] = &StrategyTemplate{
		Name:        "basic_mm",
		Type:        "market_making",
		Description: "Basic market making strategy",
		Parameters: []ParameterDefinition{
			{Name: "spread", Type: "float", Description: "Target spread in basis points", Required: true, Default: 10.0, Min: 1.0, Max: 1000.0},
			{Name: "max_inventory", Type: "float", Description: "Maximum inventory per side", Required: true, Default: 1000.0, Min: 100.0},
			{Name: "order_size", Type: "float", Description: "Order size", Required: true, Default: 100.0, Min: 10.0},
		},
		DefaultConfig: map[string]interface{}{
			"spread":        10.0,
			"max_inventory": 1000.0,
			"order_size":    100.0,
		},
	}

	// Add more templates as needed
}

func (atm *AdminToolsManager) applyStrategyTemplate(strategy *StrategyInstance, templateName string) error {
	template, exists := atm.strategyTemplates[templateName]
	if !exists {
		return fmt.Errorf("template %s not found", templateName)
	}

	// Merge template config with strategy config
	for key, value := range template.DefaultConfig {
		if _, exists := strategy.Config[key]; !exists {
			strategy.Config[key] = value
		}
	}

	return nil
}

// Additional handlers for health, system status, etc. would go here...

func (atm *AdminToolsManager) handleHealthStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	healthResults := atm.healthMonitor.GetAllHealthResults()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"health_results": healthResults,
		"overall_status": atm.calculateOverallHealth(healthResults),
	})
}

// handleHealthCheck handles health check endpoints
func (atm *AdminToolsManager) handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	healthResults := atm.healthMonitor.GetAllHealthResults()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "success",
		"data":   healthResults,
	})
}

// handleSystemStatus handles system status requests
func (atm *AdminToolsManager) handleSystemStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "success",
		"data": map[string]interface{}{
			"uptime": time.Since(atm.marketMaker.startTime).String(),
			"state":  "running",
		},
	})
}

// handleSystemMetrics handles system metrics requests
func (atm *AdminToolsManager) handleSystemMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "success",
		"data":   atm.metrics.GetAllMetrics(),
	})
}

// handleSystemConfig handles system configuration requests
func (atm *AdminToolsManager) handleSystemConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "success",
		"data":   atm.marketMaker.cfg,
	})
}

// handleSelfHealing handles self-healing requests
func (atm *AdminToolsManager) handleSelfHealing(w http.ResponseWriter, r *http.Request) {
	// Method not allowed for GET
	if r.Method == http.MethodGet {
		// Return healing history
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "success",
			"data":   atm.selfHealingManager.GetHealingHistory(),
		})
		return
	}

	// For POST, trigger healing
	if r.Method == http.MethodPost {
		var req struct {
			Component string `json:"component"`
			Reason    string `json:"reason"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		err := atm.selfHealingManager.TriggerHealing(r.Context(), req.Component, req.Reason)
		if err != nil {
			http.Error(w, "Failed to trigger healing: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "success",
			"message": "Healing triggered",
		})
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

// handleRiskLimits handles risk limits endpoints
func (atm *AdminToolsManager) handleRiskLimits(w http.ResponseWriter, r *http.Request) {
	// For now just return success with placeholder data
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "success",
		"data": map[string]interface{}{
			"max_inventory":  100.0,
			"max_daily_loss": 1000.0,
		},
	})
}

// Stub for handleStrategyTemplates
func (atm *AdminToolsManager) handleStrategyTemplates(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
	w.Write([]byte("handleStrategyTemplates not implemented"))
}

// Stub for handleBacktest
func (atm *AdminToolsManager) handleBacktest(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
	w.Write([]byte("handleBacktest not implemented"))
}

// Stub for handleBacktestActions
func (atm *AdminToolsManager) handleBacktestActions(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
	w.Write([]byte("handleBacktestActions not implemented"))
}

// calculateOverallHealth calculates the overall health status based on component health results
func (atm *AdminToolsManager) calculateOverallHealth(results map[string]*HealthCheckResult) string {
	unhealthy := 0
	degraded := 0

	for _, result := range results {
		if result.Status == HealthUnhealthy {
			unhealthy++
		} else if result.Status == HealthDegraded {
			degraded++
		}
	}

	if unhealthy > 0 {
		return "unhealthy"
	} else if degraded > 0 {
		return "degraded"
	}
	return "healthy"
}

// GetMetricsCollector is a helper method to allow extensions to access metrics

// Stub for LogEmergencyEvent on StructuredLogger
func (sl *StructuredLogger) LogEmergencyEvent(ctx context.Context, eventType, reason string, details map[string]interface{}) {
	// No-op stub for build
}
