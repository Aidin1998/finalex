// Package settlement implements T+0 settlement, netting, and reconciliation for all asset types.
package settlement

import (
	"sync"
	"time"
)

// TradeCapture represents a matched trade to be settled.
type TradeCapture struct {
	TradeID   string
	UserID    string
	Symbol    string
	Side      string // "buy" or "sell"
	Quantity  float64
	Price     float64
	AssetType string // "crypto", "fiat", etc.
	MatchedAt time.Time
}

// NetPosition represents a user's netted position for a symbol in a settlement cycle.
type NetPosition struct {
	UserID    string
	Symbol    string
	NetQty    float64
	AssetType string
}

// SettlementEngine handles trade capture, netting, clearing, and reconciliation.
type SettlementEngine struct {
	mu        sync.Mutex
	trades    []TradeCapture
	positions map[string]map[string]float64 // userID -> symbol -> net qty
}

func NewSettlementEngine() *SettlementEngine {
	return &SettlementEngine{
		positions: make(map[string]map[string]float64),
	}
}

// CaptureTrade records a matched trade for settlement.
func (se *SettlementEngine) CaptureTrade(trade TradeCapture) {
	se.mu.Lock()
	defer se.mu.Unlock()
	se.trades = append(se.trades, trade)
}

// NetPositions nets all trades for T+0 settlement.
func (se *SettlementEngine) NetPositions() []NetPosition {
	se.mu.Lock()
	defer se.mu.Unlock()
	se.positions = make(map[string]map[string]float64)
	for _, t := range se.trades {
		if se.positions[t.UserID] == nil {
			se.positions[t.UserID] = make(map[string]float64)
		}
		qty := t.Quantity
		if t.Side == "sell" {
			qty = -qty
		}
		se.positions[t.UserID][t.Symbol] += qty
	}
	var netted []NetPosition
	for user, m := range se.positions {
		for symbol, netQty := range m {
			netted = append(netted, NetPosition{UserID: user, Symbol: symbol, NetQty: netQty})
		}
	}
	return netted
}

// ClearAndSettle processes netted positions and updates balances (stub: to be integrated with bookkeeper/wallet).
func (se *SettlementEngine) ClearAndSettle() {
	// TODO: Integrate with bookkeeper/wallet for actual asset transfer
	// For each NetPosition, move netQty between accounts
}

// Reconcile checks that positions and balances match executed trades and settlement records.
func (se *SettlementEngine) Reconcile() error {
	// TODO: Implement reconciliation logic
	return nil
}
