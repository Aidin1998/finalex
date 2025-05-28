package marketmaker

import (
	"math/rand"
)

type Strategy interface {
	Quote(mid, volatility, inventory float64) (bid, ask float64, size float64)
}

type BasicStrategy struct {
	Spread float64
	Size   float64
}

func (s *BasicStrategy) Quote(mid, volatility, inventory float64) (bid, ask float64, size float64) {
	bid = mid * (1 - s.Spread/2)
	ask = mid * (1 + s.Spread/2)
	return bid, ask, s.Size
}

type DynamicStrategy struct {
	BaseSpread float64
	VolFactor  float64
	Size       float64
}

func (s *DynamicStrategy) Quote(mid, volatility, inventory float64) (bid, ask float64, size float64) {
	spread := s.BaseSpread + s.VolFactor*volatility
	bid = mid * (1 - spread/2)
	ask = mid * (1 + spread/2)
	return bid, ask, s.Size
}

type InventorySkewStrategy struct {
	BaseSpread float64
	InvFactor  float64
	Size       float64
}

func (s *InventorySkewStrategy) Quote(mid, volatility, inventory float64) (bid, ask float64, size float64) {
	invSkew := s.InvFactor * inventory
	bid = mid * (1 - (s.BaseSpread+invSkew)/2)
	ask = mid * (1 + (s.BaseSpread-invSkew)/2)
	return bid, ask, s.Size
}

// Example: random strategy for testing
func RandomStrategy(mid float64) (bid, ask, size float64) {
	spread := 0.001 + rand.Float64()*0.002
	bid = mid * (1 - spread/2)
	ask = mid * (1 + spread/2)
	size = 1 + rand.Float64()*2
	return
}
