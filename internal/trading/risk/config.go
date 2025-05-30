package risk

import (
	"sync"
)

// RiskConfig holds risk parameters and exemptions.
type RiskConfig struct {
	SymbolLimits   map[string]float64  // e.g. {"BTCUSDT": 100.0}
	ExemptAccounts map[string]struct{} // userID string set
	mu             sync.RWMutex
}

func NewRiskConfig() *RiskConfig {
	return &RiskConfig{
		SymbolLimits:   make(map[string]float64),
		ExemptAccounts: make(map[string]struct{}),
	}
}

func (rc *RiskConfig) SetSymbolLimit(symbol string, max float64) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.SymbolLimits[symbol] = max
}

func (rc *RiskConfig) GetSymbolLimit(symbol string) float64 {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	return rc.SymbolLimits[symbol]
}

func (rc *RiskConfig) AddExemptAccount(userID string) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.ExemptAccounts[userID] = struct{}{}
}

func (rc *RiskConfig) RemoveExemptAccount(userID string) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	delete(rc.ExemptAccounts, userID)
}

func (rc *RiskConfig) IsExempt(userID string) bool {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	_, ok := rc.ExemptAccounts[userID]
	return ok
}
