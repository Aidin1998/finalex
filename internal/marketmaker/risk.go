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
