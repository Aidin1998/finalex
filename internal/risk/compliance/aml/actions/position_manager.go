package actions

import (
	"context"
	"sync"

	"github.com/shopspring/decimal"
)

// Position represents a user's position in a market
type Position struct {
	Quantity decimal.Decimal // net position quantity
	AvgPrice decimal.Decimal // average price for the position
}

// LimitTypes for position limits
type LimitType string

const (
	UserLimitType   LimitType = "user"
	MarketLimitType LimitType = "market"
	GlobalLimitType LimitType = "global"
)

// LimitConfig holds limit values for different scopes
type LimitConfig struct {
	UserLimits   map[string]decimal.Decimal // userID -> max allowed quantity
	MarketLimits map[string]decimal.Decimal // market -> max allowed quantity
	GlobalLimit  decimal.Decimal            // global max allowed quantity
}

// PositionManager tracks positions and enforces limits
type PositionManager struct {
	mu        sync.RWMutex
	positions map[string]map[string]*Position // userID -> market -> Position
	limits    LimitConfig
}

// NewPositionManager creates a new PositionManager with given limits
func NewPositionManager(limits LimitConfig) *PositionManager {
	return &PositionManager{
		positions: make(map[string]map[string]*Position),
		limits:    limits,
	}
}

// ProcessTrade updates position for a trade event
func (pm *PositionManager) ProcessTrade(ctx context.Context, userID, market string, qty, price decimal.Decimal) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if _, ok := pm.positions[userID]; !ok {
		pm.positions[userID] = make(map[string]*Position)
	}
	pos, ok := pm.positions[userID][market]
	if !ok {
		pos = &Position{Quantity: decimal.Zero, AvgPrice: decimal.Zero}
		pm.positions[userID][market] = pos
	}
	// New avg price calculation
	totalQty := pos.Quantity.Add(qty)
	if totalQty.IsZero() {
		// position closed
		pos.Quantity = decimal.Zero
		pos.AvgPrice = decimal.Zero
	} else {
		pos.AvgPrice = pos.AvgPrice.Mul(pos.Quantity).Add(price.Mul(qty)).Div(totalQty)
		pos.Quantity = totalQty
	}
	return nil
}

// GetPosition returns current position for a user and market
func (pm *PositionManager) GetPosition(ctx context.Context, userID, market string) *Position {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	if m, ok := pm.positions[userID]; ok {
		if pos, ok2 := m[market]; ok2 {
			return pos
		}
	}
	return &Position{Quantity: decimal.Zero, AvgPrice: decimal.Zero}
}

// ListPositions returns all positions for a user
func (pm *PositionManager) ListPositions(ctx context.Context, userID string) map[string]*Position {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	if m, ok := pm.positions[userID]; ok {
		// create a copy to avoid external mutation
		copyMap := make(map[string]*Position, len(m))
		for market, pos := range m {
			copyMap[market] = &Position{
				Quantity: pos.Quantity,
				AvgPrice: pos.AvgPrice,
			}
		}
		return copyMap
	}
	return map[string]*Position{}
}

// CheckLimit verifies if a proposed order would breach limits
func (pm *PositionManager) CheckLimit(ctx context.Context, userID, market string, orderQty decimal.Decimal) (bool, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	pos := pm.GetPosition(ctx, userID, market)
	newQty := pos.Quantity.Add(orderQty).Abs()
	// Check user limit
	if lim, ok := pm.limits.UserLimits[userID]; ok {
		if newQty.GreaterThan(lim) {
			return false, nil
		}
	}
	// Check market limit
	if lim, ok := pm.limits.MarketLimits[market]; ok {
		if newQty.GreaterThan(lim) {
			return false, nil
		}
	}
	// Check global limit
	if pm.limits.GlobalLimit.IsPositive() && newQty.GreaterThan(pm.limits.GlobalLimit) {
		return false, nil
	}
	return true, nil
}
