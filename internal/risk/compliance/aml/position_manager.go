package aml

import (
	"context"
	"sync"

	"github.com/shopspring/decimal"
)

// PositionManager handles position tracking and limit management
type PositionManager struct {
	mu        sync.RWMutex
	limits    LimitConfig
	positions map[string]map[string]decimal.Decimal // userID -> market -> position
}

// NewPositionManager creates a new position manager with the given limits
func NewPositionManager(limits LimitConfig) *PositionManager {
	return &PositionManager{
		limits:    limits,
		positions: make(map[string]map[string]decimal.Decimal),
	}
}

// ProcessTrade updates positions based on a trade
func (pm *PositionManager) ProcessTrade(ctx context.Context, userID, market string, quantity, price decimal.Decimal) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pm.positions[userID] == nil {
		pm.positions[userID] = make(map[string]decimal.Decimal)
	}

	// Update position
	currentPosition := pm.positions[userID][market]
	newPosition := currentPosition.Add(quantity)
	pm.positions[userID][market] = newPosition

	return nil
}

// CheckPositionLimit verifies if a new position would exceed limits
func (pm *PositionManager) CheckPositionLimit(ctx context.Context, userID, market string, quantity, price decimal.Decimal) (bool, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	// Check user limit
	if userLimit, exists := pm.limits.UserLimits[userID]; exists {
		currentExposure := pm.calculateUserExposure(userID)
		newExposure := currentExposure.Add(quantity.Mul(price).Abs())
		if newExposure.GreaterThan(userLimit) {
			return false, nil
		}
	}

	// Check market limit
	if marketLimit, exists := pm.limits.MarketLimits[market]; exists {
		currentMarketExposure := pm.calculateMarketExposure(market)
		newExposure := currentMarketExposure.Add(quantity.Mul(price).Abs())
		if newExposure.GreaterThan(marketLimit) {
			return false, nil
		}
	}

	// Check global limit
	if !pm.limits.GlobalLimit.IsZero() {
		totalExposure := pm.calculateTotalExposure()
		newExposure := totalExposure.Add(quantity.Mul(price).Abs())
		if newExposure.GreaterThan(pm.limits.GlobalLimit) {
			return false, nil
		}
	}

	return true, nil
}

// CheckLimit checks if a user can take a position within limits
func (pm *PositionManager) CheckLimit(ctx context.Context, userID, market string, quantity decimal.Decimal) (bool, error) {
	// Use the existing CheckPositionLimit method with a default price of 1
	// This is a simplified check - in practice, you'd want the actual price
	return pm.CheckPositionLimit(ctx, userID, market, quantity, decimal.NewFromInt(1))
}

// GetUserPosition returns the current position for a user in a market
func (pm *PositionManager) GetUserPosition(userID, market string) decimal.Decimal {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	if pm.positions[userID] == nil {
		return decimal.Zero
	}
	return pm.positions[userID][market]
}

// ListPositions returns all positions for a user
func (pm *PositionManager) ListPositions(ctx context.Context, userID string) map[string]decimal.Decimal {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	if pm.positions[userID] == nil {
		return make(map[string]decimal.Decimal)
	}

	// Return a copy to avoid race conditions
	result := make(map[string]decimal.Decimal)
	for market, position := range pm.positions[userID] {
		result[market] = position
	}
	return result
}

// UpdateLimits updates the limit configuration
func (pm *PositionManager) UpdateLimits(limits LimitConfig) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.limits = limits
}

// GetLimits returns the current limit configuration
func (pm *PositionManager) GetLimits() LimitConfig {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.limits
}

// calculateUserExposure calculates total exposure for a user
func (pm *PositionManager) calculateUserExposure(userID string) decimal.Decimal {
	total := decimal.Zero
	if userPositions, exists := pm.positions[userID]; exists {
		for _, position := range userPositions {
			total = total.Add(position.Abs())
		}
	}
	return total
}

// calculateMarketExposure calculates total exposure for a market
func (pm *PositionManager) calculateMarketExposure(market string) decimal.Decimal {
	total := decimal.Zero
	for _, userPositions := range pm.positions {
		if position, exists := userPositions[market]; exists {
			total = total.Add(position.Abs())
		}
	}
	return total
}

// calculateTotalExposure calculates total system exposure
func (pm *PositionManager) calculateTotalExposure() decimal.Decimal {
	total := decimal.Zero
	for _, userPositions := range pm.positions {
		for _, position := range userPositions {
			total = total.Add(position.Abs())
		}
	}
	return total
}
