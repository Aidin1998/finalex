package risk

import (
	"fmt"
	"sync"
)

type PositionManager struct {
	positions map[string]map[string]float64 // userID -> symbol -> position
	cfg       *RiskConfig
	mu        sync.RWMutex
}

func NewPositionManager(cfg *RiskConfig) *PositionManager {
	return &PositionManager{
		positions: make(map[string]map[string]float64),
		cfg:       cfg,
	}
}

// UpdatePosition should be called after every fill.
func (pm *PositionManager) UpdatePosition(userID, symbol string, delta float64) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if pm.positions[userID] == nil {
		pm.positions[userID] = make(map[string]float64)
	}
	pm.positions[userID][symbol] += delta
}

// GetPosition returns the current position for a user and symbol.
func (pm *PositionManager) GetPosition(userID, symbol string) float64 {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	if pm.positions[userID] == nil {
		return 0
	}
	return pm.positions[userID][symbol]
}

// CheckPositionLimit enforces per-symbol limits for non-exempt users.
func (pm *PositionManager) CheckPositionLimit(userID, symbol string, intendedQty float64) error {
	if pm.cfg.IsExempt(userID) {
		return nil
	}
	max := pm.cfg.GetSymbolLimit(symbol)
	if max == 0 {
		return nil // no limit set
	}
	pos := pm.GetPosition(userID, symbol)
	if pos+intendedQty > max || pos+intendedQty < -max {
		return fmt.Errorf("risk: position limit exceeded for %s: limit %.2f, attempted %.2f", symbol, max, pos+intendedQty)
	}
	return nil
}
