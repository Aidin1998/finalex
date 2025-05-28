package risk

import (
	"errors"
	"math"
)

// VolatilityRiskManager manages volatility risk.
type VolatilityRiskManager struct {
	historicalPrices []float64
}

// NewVolatilityRiskManager creates a new instance of VolatilityRiskManager.
func NewVolatilityRiskManager() *VolatilityRiskManager {
	return &VolatilityRiskManager{
		historicalPrices: []float64{},
	}
}

// AddPrice adds a new price to the historical prices.
func (vrm *VolatilityRiskManager) AddPrice(price float64) {
	vrm.historicalPrices = append(vrm.historicalPrices, price)
}

// CalculateVolatility calculates the volatility based on historical prices.
func (vrm *VolatilityRiskManager) CalculateVolatility() (float64, error) {
	n := len(vrm.historicalPrices)
	if n < 2 {
		return 0, errors.New("not enough data to calculate volatility")
	}

	mean := vrm.calculateMean()
	variance := 0.0

	for _, price := range vrm.historicalPrices {
		variance += math.Pow(price-mean, 2)
	}

	variance /= float64(n)
	return math.Sqrt(variance), nil
}

// calculateMean calculates the mean of the historical prices.
func (vrm *VolatilityRiskManager) calculateMean() float64 {
	sum := 0.0
	for _, price := range vrm.historicalPrices {
		sum += price
	}
	return sum / float64(len(vrm.historicalPrices))
}

// ImplementAutomatedControl checks volatility and implements control mechanisms.
func (vrm *VolatilityRiskManager) ImplementAutomatedControl(threshold float64) (bool, error) {
	volatility, err := vrm.CalculateVolatility()
	if err != nil {
		return false, err
	}

	if volatility > threshold {
		// Implement control mechanism, e.g., reduce position size or halt trading
		return true, nil
	}
	return false, nil
}