package risk

import (
	"sync"
)

// RiskConfig holds configuration for risk management.
type RiskConfig struct {
	MaxInventory  float64
	MaxVolatility float64
	MinInventory  float64
}

// RiskState holds the current state for risk checks.
type RiskState struct {
	Inventory  float64
	Volatility float64
}

// RiskManager manages risk parameters and mitigation.
type RiskManager struct {
	Config RiskConfig
	mu     sync.RWMutex
}

// NewRiskManager creates a new risk manager.
func NewRiskManager(cfg RiskConfig) *RiskManager {
	return &RiskManager{Config: cfg}
}

// CheckRisk returns true if within risk limits, false if action needed.
func (rm *RiskManager) CheckRisk(state RiskState) bool {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	if state.Inventory > rm.Config.MaxInventory || state.Inventory < rm.Config.MinInventory {
		return false
	}
	if state.Volatility > rm.Config.MaxVolatility {
		return false
	}
	return true
}

// MitigateRisk returns recommended action if risk is breached.
func (rm *RiskManager) MitigateRisk(state RiskState) string {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	if state.Inventory > rm.Config.MaxInventory {
		return "Reduce inventory: sell or cancel buy orders."
	}
	if state.Inventory < rm.Config.MinInventory {
		return "Increase inventory: buy or cancel sell orders."
	}
	if state.Volatility > rm.Config.MaxVolatility {
		return "Pause trading or widen spreads due to high volatility."
	}
	return "No action needed."
}
