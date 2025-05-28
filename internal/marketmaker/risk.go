package marketmaker

import (
	"sync"
)

type RiskManager struct {
	maxInventory float64
	maxLoss      float64
	inventory    float64
	pnl          float64
	mu           sync.Mutex
}

func NewRiskManager(maxInv, maxLoss float64) *RiskManager {
	return &RiskManager{maxInventory: maxInv, maxLoss: maxLoss}
}

func (r *RiskManager) UpdateInventory(delta float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.inventory += delta
}

func (r *RiskManager) UpdatePnL(delta float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pnl += delta
}

func (r *RiskManager) Breach() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.inventory > r.maxInventory || r.pnl < -r.maxLoss
}

// Add position/exposure checks and kill switch
func (r *RiskManager) CheckPositionLimit(limit float64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.inventory <= limit
}

func (r *RiskManager) CheckPnLLimit(limit float64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.pnl >= -limit
}

func (r *RiskManager) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.inventory = 0
	r.pnl = 0
}

func (r *RiskManager) EmergencyStop() {
	// In production, trigger a kill switch for all quoting
	panic("Market making risk limits breached: EMERGENCY STOP")
}
