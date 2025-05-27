package risk

import (
	"errors"
	"sync"
)

// Inventory represents the inventory levels for the market maker bot.
type Inventory struct {
	sync.Mutex
	levels map[string]float64 // Maps asset symbols to their inventory levels
}

// NewInventory initializes a new Inventory instance.
func NewInventory() *Inventory {
	return &Inventory{
		levels: make(map[string]float64),
	}
}

// Add adds the specified amount to the inventory for the given asset.
func (inv *Inventory) Add(asset string, amount float64) {
	inv.Lock()
	defer inv.Unlock()
	inv.levels[asset] += amount
}

// Remove subtracts the specified amount from the inventory for the given asset.
// Returns an error if the amount to remove exceeds the current inventory level.
func (inv *Inventory) Remove(asset string, amount float64) error {
	inv.Lock()
	defer inv.Unlock()
	if inv.levels[asset] < amount {
		return errors.New("insufficient inventory")
	}
	inv.levels[asset] -= amount
	return nil
}

// GetLevel returns the current inventory level for the given asset.
func (inv *Inventory) GetLevel(asset string) float64 {
	inv.Lock()
	defer inv.Unlock()
	return inv.levels[asset]
}

// AssessRisk evaluates the risk exposure based on the current inventory levels.
// This is a placeholder for more complex risk assessment logic.
func (inv *Inventory) AssessRisk() float64 {
	inv.Lock()
	defer inv.Unlock()
	// Placeholder for risk assessment logic
	var totalRisk float64
	for _, level := range inv.levels {
		totalRisk += level // Simplified risk calculation
	}
	return totalRisk
}