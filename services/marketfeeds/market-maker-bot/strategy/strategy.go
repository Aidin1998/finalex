package strategy

import (
	"math"
)

// StrategyType defines the type of market making strategy.
type StrategyType int

const (
	SpreadBased StrategyType = iota
	InventoryAware
	DynamicPricing
)

// StrategyConfig holds configuration for a strategy.
type StrategyConfig struct {
	BaseSpread      float64
	InventoryTarget float64
	MaxPosition     float64
	Volatility      float64
}

// MarketState holds current market and inventory data.
type MarketState struct {
	BestBid    float64
	BestAsk    float64
	Inventory  float64
	MidPrice   float64
	Volatility float64
}

// Strategy is the interface for all market making strategies.
type Strategy interface {
	Quote(state MarketState) (bid, ask float64)
}

// SpreadBasedStrategy implements a fixed spread market making strategy.
type SpreadBasedStrategy struct {
	Config StrategyConfig
}

func (s *SpreadBasedStrategy) Quote(state MarketState) (bid, ask float64) {
	spread := s.Config.BaseSpread
	mid := (state.BestBid + state.BestAsk) / 2
	return mid - spread/2, mid + spread/2
}

// InventoryAwareStrategy adjusts quotes based on inventory.
type InventoryAwareStrategy struct {
	Config StrategyConfig
}

func (s *InventoryAwareStrategy) Quote(state MarketState) (bid, ask float64) {
	spread := s.Config.BaseSpread
	invAdj := (state.Inventory - s.Config.InventoryTarget) / s.Config.MaxPosition
	adj := spread * invAdj
	mid := (state.BestBid + state.BestAsk) / 2
	return mid - (spread / 2) + adj, mid + (spread / 2) + adj
}

// DynamicPricingStrategy adjusts spread based on volatility and inventory.
type DynamicPricingStrategy struct {
	Config StrategyConfig
}

func (s *DynamicPricingStrategy) Quote(state MarketState) (bid, ask float64) {
	volAdj := math.Min(1.0, state.Volatility/s.Config.Volatility)
	spread := s.Config.BaseSpread * (1 + volAdj)
	invAdj := (state.Inventory - s.Config.InventoryTarget) / s.Config.MaxPosition
	adj := spread * invAdj
	mid := (state.BestBid + state.BestAsk) / 2
	return mid - (spread / 2) + adj, mid + (spread / 2) + adj
}

// NewStrategy returns a strategy implementation based on type.
func NewStrategy(t StrategyType, cfg StrategyConfig) Strategy {
	switch t {
	case SpreadBased:
		return &SpreadBasedStrategy{Config: cfg}
	case InventoryAware:
		return &InventoryAwareStrategy{Config: cfg}
	case DynamicPricing:
		return &DynamicPricingStrategy{Config: cfg}
	default:
		return &SpreadBasedStrategy{Config: cfg}
	}
}
