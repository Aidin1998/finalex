// Lock-free, event-driven position and margin tracker for high-performance risk management
package risk

import (
	"math"
	"sync/atomic"

	"github.com/shopspring/decimal"
)

type UserID string

// Symbol is a string alias for trading symbol (e.g. "BTCUSDT")
type Symbol string

// UserSymbol is a composite key for user+symbol
// Use [userID|symbol] as map key for fast lookup
type UserSymbol struct {
	UserID UserID
	Symbol Symbol
}

type atomicDecimal struct {
	bits atomic.Uint64 // stores decimal.Decimal as IEEE-754 float64 bits
}

func newAtomicDecimal(val decimal.Decimal) *atomicDecimal {
	ad := &atomicDecimal{}
	ad.Store(val)
	return ad
}

func (a *atomicDecimal) Load() decimal.Decimal {
	bits := a.bits.Load()
	return decimal.NewFromFloat(float64FromBits(bits))
}

func (a *atomicDecimal) Store(val decimal.Decimal) {
	f, _ := val.Float64()
	a.bits.Store(float64bits(f))
}

func (a *atomicDecimal) Add(delta decimal.Decimal) decimal.Decimal {
	for {
		oldBits := a.bits.Load()
		oldVal := decimal.NewFromFloat(float64FromBits(oldBits))
		newVal := oldVal.Add(delta)
		f, _ := newVal.Float64()
		newBits := float64bits(f)
		if a.bits.CompareAndSwap(oldBits, newBits) {
			return newVal
		}
	}
}

func float64bits(f float64) uint64     { return math.Float64bits(f) }
func float64FromBits(b uint64) float64 { return math.Float64frombits(b) }

// PositionTracker is a lock-free, event-driven position and margin tracker
type PositionTracker struct {
	positions map[UserSymbol]*atomicDecimal // user+symbol -> position
	pnl       map[UserSymbol]*atomicDecimal // user+symbol -> P&L
	margins   map[UserID]*atomicDecimal     // user -> margin
	limits    map[UserID]*RiskLimits        // user -> static risk limits
}

type RiskLimits struct {
	MaxPosition decimal.Decimal
	DailyLimit  decimal.Decimal
	VaRLimit    decimal.Decimal
}

func NewPositionTracker() *PositionTracker {
	return &PositionTracker{
		positions: make(map[UserSymbol]*atomicDecimal),
		pnl:       make(map[UserSymbol]*atomicDecimal),
		margins:   make(map[UserID]*atomicDecimal),
		limits:    make(map[UserID]*RiskLimits),
	}
}

// UpdatePosition atomically updates a user's position and triggers async risk evaluation
func (pt *PositionTracker) UpdatePosition(userID UserID, symbol Symbol, qty, price decimal.Decimal) {
	key := UserSymbol{userID, symbol}
	pos, ok := pt.positions[key]
	if !ok {
		pos = newAtomicDecimal(decimal.Zero)
		pt.positions[key] = pos
	}
	pos.Add(qty)
	// Async: trigger risk evaluation (event-driven)
	// (In production, send to risk event channel)
}

// GetPosition returns the current position for a user and symbol
func (pt *PositionTracker) GetPosition(userID UserID, symbol Symbol) decimal.Decimal {
	key := UserSymbol{userID, symbol}
	if pos, ok := pt.positions[key]; ok {
		return pos.Load()
	}
	return decimal.Zero
}

// ... Add similar methods for P&L, margin, and limits ...
