package marketmaker

import (
	"math"
	"math/rand"
	"sync"
	"time"
)

type Strategy interface {
	Quote(mid, volatility, inventory float64) (bid, ask float64, size float64)
}

// Market data structure for advanced strategies
type MarketData struct {
	Price          float64
	Volume         float64
	Timestamp      time.Time
	BidBookDepth   []PriceLevel
	AskBookDepth   []PriceLevel
	OrderImbalance float64
	Vwap           float64
}

type PriceLevel struct {
	Price  float64
	Volume float64
}

// Advanced market context for sophisticated strategies
type MarketContext struct {
	RecentTrades      []TradeData
	VolatilitySurface map[time.Duration]float64
	OrderFlow         OrderFlowMetrics
	MarketRegime      RegimeType
	CrossExchangeData map[string]float64
	mu                sync.RWMutex
}

type TradeData struct {
	Price     float64
	Volume    float64
	Side      string // "buy" or "sell"
	Timestamp time.Time
}

type OrderFlowMetrics struct {
	BuyImbalance     float64
	SellImbalance    float64
	ToxicFlow        float64
	InformedTrading  float64
	MicroPriceImpact float64
}

type RegimeType int

const (
	TrendingRegime RegimeType = iota
	MeanRevertingRegime
	HighVolatilityRegime
	LowLiquidityRegime
)

// Basic strategies (existing)
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

// ADVANCED STRATEGIES - Exchange Grade Performance

// PredictiveStrategy uses machine learning-inspired algorithms for price prediction
type PredictiveStrategy struct {
	BaseSpread        float64
	PredictionHorizon time.Duration
	VolDecayFactor    float64
	MomentumFactor    float64
	MeanReversionRate float64
	MaxPosition       float64
	Context           *MarketContext
	PriceHistory      []float64
	mu                sync.RWMutex
}

func NewPredictiveStrategy(baseSpread float64, maxPosition float64) *PredictiveStrategy {
	return &PredictiveStrategy{
		BaseSpread:        baseSpread,
		PredictionHorizon: 100 * time.Millisecond,
		VolDecayFactor:    0.95,
		MomentumFactor:    0.3,
		MeanReversionRate: 0.1,
		MaxPosition:       maxPosition,
		Context:           &MarketContext{},
		PriceHistory:      make([]float64, 0, 100),
	}
}

func (s *PredictiveStrategy) Quote(mid, volatility, inventory float64) (bid, ask float64, size float64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Update price history
	s.updatePriceHistory(mid)

	// Calculate predictive components
	momentum := s.calculateMomentum()
	meanReversion := s.calculateMeanReversion(mid)
	orderFlowImpact := s.calculateOrderFlowImpact()
	regimeAdjustment := s.calculateRegimeAdjustment()

	// Predict short-term price movement
	predictedMove := momentum*s.MomentumFactor - meanReversion*s.MeanReversionRate + orderFlowImpact

	// Adjust spread based on prediction confidence and market regime
	spreadMultiplier := 1.0 + math.Abs(predictedMove)*2.0 + regimeAdjustment
	adjustedSpread := s.BaseSpread * spreadMultiplier

	// Inventory-based skew with predictive adjustment
	inventorySkew := inventory * 0.001    // Base inventory penalty
	predictiveSkew := predictedMove * 0.5 // Skew quotes toward expected price movement
	totalSkew := inventorySkew + predictiveSkew

	// Calculate quotes with asymmetric spreads
	bidSpread := adjustedSpread/2 + math.Max(0, totalSkew)
	askSpread := adjustedSpread/2 + math.Max(0, -totalSkew)

	bid = mid * (1 - bidSpread)
	ask = mid * (1 + askSpread)

	// Dynamic sizing based on confidence and volatility
	confidenceFactor := 1.0 / (1.0 + volatility*10.0)
	baseSize := s.MaxPosition * 0.1
	size = baseSize * confidenceFactor

	return bid, ask, size
}

func (s *PredictiveStrategy) updatePriceHistory(price float64) {
	s.PriceHistory = append(s.PriceHistory, price)
	if len(s.PriceHistory) > 100 {
		s.PriceHistory = s.PriceHistory[1:]
	}
}

func (s *PredictiveStrategy) calculateMomentum() float64 {
	if len(s.PriceHistory) < 20 {
		return 0.0
	}

	// Calculate weighted momentum over different time horizons
	shortMomentum := s.calculateReturn(5)
	mediumMomentum := s.calculateReturn(15)

	return 0.7*shortMomentum + 0.3*mediumMomentum
}

func (s *PredictiveStrategy) calculateReturn(periods int) float64 {
	if len(s.PriceHistory) < periods+1 {
		return 0.0
	}

	current := s.PriceHistory[len(s.PriceHistory)-1]
	past := s.PriceHistory[len(s.PriceHistory)-periods-1]

	return (current - past) / past
}

func (s *PredictiveStrategy) calculateMeanReversion(currentPrice float64) float64 {
	if len(s.PriceHistory) < 50 {
		return 0.0
	}

	// Calculate moving average
	sum := 0.0
	count := math.Min(50, float64(len(s.PriceHistory)))
	for i := len(s.PriceHistory) - int(count); i < len(s.PriceHistory); i++ {
		sum += s.PriceHistory[i]
	}
	avg := sum / count

	return (currentPrice - avg) / avg
}

func (s *PredictiveStrategy) calculateOrderFlowImpact() float64 {
	s.Context.mu.RLock()
	defer s.Context.mu.RUnlock()

	// Simplified order flow impact calculation
	return s.Context.OrderFlow.BuyImbalance*0.1 - s.Context.OrderFlow.SellImbalance*0.1
}

func (s *PredictiveStrategy) calculateRegimeAdjustment() float64 {
	s.Context.mu.RLock()
	defer s.Context.mu.RUnlock()

	switch s.Context.MarketRegime {
	case HighVolatilityRegime:
		return 0.5 // Wider spreads in high volatility
	case LowLiquidityRegime:
		return 0.3 // Wider spreads in low liquidity
	case TrendingRegime:
		return -0.1 // Tighter spreads in trending markets
	default:
		return 0.0
	}
}

// VolatilitySurfaceStrategy models volatility across different time horizons
type VolatilitySurfaceStrategy struct {
	BaseSpread  float64
	MaxPosition float64
	VolSurface  map[time.Duration]float64
	LastUpdate  time.Time
	Context     *MarketContext
	mu          sync.RWMutex
}

func NewVolatilitySurfaceStrategy(baseSpread, maxPosition float64) *VolatilitySurfaceStrategy {
	return &VolatilitySurfaceStrategy{
		BaseSpread:  baseSpread,
		MaxPosition: maxPosition,
		VolSurface:  make(map[time.Duration]float64),
		Context:     &MarketContext{},
	}
}

func (s *VolatilitySurfaceStrategy) Quote(mid, volatility, inventory float64) (bid, ask float64, size float64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Update volatility surface
	s.updateVolatilitySurface(volatility)

	// Calculate term structure of volatility
	shortTermVol := s.getVolatility(time.Minute)
	mediumTermVol := s.getVolatility(5 * time.Minute)
	longTermVol := s.getVolatility(15 * time.Minute)

	// Volatility term structure analysis
	volSlope := (longTermVol - shortTermVol) / shortTermVol
	volConvexity := mediumTermVol - (shortTermVol+longTermVol)/2

	// Adjust spread based on volatility surface characteristics
	spreadMultiplier := 1.0 + shortTermVol*5.0 + math.Abs(volSlope)*2.0 + math.Abs(volConvexity)*3.0
	adjustedSpread := s.BaseSpread * spreadMultiplier

	// Inventory management with volatility adjustment
	inventoryRisk := math.Abs(inventory) * shortTermVol
	inventorySkew := inventory * (0.001 + inventoryRisk*0.01)

	bid = mid * (1 - adjustedSpread/2 - inventorySkew)
	ask = mid * (1 + adjustedSpread/2 + inventorySkew)

	// Size based on volatility and capacity
	volAdjustment := 1.0 / (1.0 + shortTermVol*20.0)
	inventoryAdjustment := math.Max(0.1, 1.0-math.Abs(inventory)/s.MaxPosition)
	size = s.MaxPosition * 0.05 * volAdjustment * inventoryAdjustment

	return bid, ask, size
}

func (s *VolatilitySurfaceStrategy) updateVolatilitySurface(currentVol float64) {
	now := time.Now()

	// Update different time horizon volatilities with decay
	horizons := []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute, time.Hour}

	for _, horizon := range horizons {
		if existing, exists := s.VolSurface[horizon]; exists {
			// Exponential decay based on time horizon
			decayFactor := math.Exp(-float64(now.Sub(s.LastUpdate)) / float64(horizon))
			s.VolSurface[horizon] = existing*decayFactor + currentVol*(1-decayFactor)
		} else {
			s.VolSurface[horizon] = currentVol
		}
	}

	s.LastUpdate = now
}

func (s *VolatilitySurfaceStrategy) getVolatility(horizon time.Duration) float64 {
	if vol, exists := s.VolSurface[horizon]; exists {
		return vol
	}
	return 0.01 // Default volatility
}

// MicroStructureStrategy focuses on order book microstructure and market impact
type MicroStructureStrategy struct {
	BaseSpread      float64
	MaxPosition     float64
	TickSize        float64
	MinSpreadTicks  int
	BookDepthLevels int
	ImpactDecayRate float64
	Context         *MarketContext
	mu              sync.RWMutex
}

func NewMicroStructureStrategy(baseSpread, maxPosition, tickSize float64) *MicroStructureStrategy {
	return &MicroStructureStrategy{
		BaseSpread:      baseSpread,
		MaxPosition:     maxPosition,
		TickSize:        tickSize,
		MinSpreadTicks:  2,
		BookDepthLevels: 10,
		ImpactDecayRate: 0.1,
		Context:         &MarketContext{},
	}
}

func (s *MicroStructureStrategy) Quote(mid, volatility, inventory float64) (bid, ask float64, size float64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Analyze order book microstructure
	bidPressure, askPressure := s.calculateBookPressure()
	marketImpact := s.calculateMarketImpact()
	adverseSelection := s.calculateAdverseSelection()

	// Base spread with microstructure adjustments
	microSpread := s.BaseSpread + marketImpact*0.5 + adverseSelection*0.3

	// Ensure minimum tick size compliance
	minSpread := float64(s.MinSpreadTicks) * s.TickSize / mid
	adjustedSpread := math.Max(microSpread, minSpread)

	// Asymmetric spread based on order book pressure
	pressureSkew := (bidPressure - askPressure) * 0.0005
	inventorySkew := inventory * 0.001
	totalSkew := pressureSkew + inventorySkew

	// Calculate optimal quote placement
	bid = s.optimizeBidPlacement(mid, adjustedSpread, totalSkew)
	ask = s.optimizeAskPlacement(mid, adjustedSpread, totalSkew)

	// Position sizing based on microstructure signals
	signalStrength := math.Abs(pressureSkew) + math.Abs(adverseSelection)
	confidenceFactor := 1.0 / (1.0 + signalStrength*10.0)
	size = s.MaxPosition * 0.02 * confidenceFactor

	return bid, ask, size
}

func (s *MicroStructureStrategy) calculateBookPressure() (bidPressure, askPressure float64) {
	s.Context.mu.RLock()
	defer s.Context.mu.RUnlock()

	// Calculate weighted pressure from order book depth
	bidSum, askSum := 0.0, 0.0
	bidWeight, askWeight := 0.0, 0.0

	maxLevels := math.Min(float64(len(s.Context.RecentTrades)), float64(s.BookDepthLevels))

	for i := 0; i < int(maxLevels); i++ {
		weight := 1.0 / (1.0 + float64(i)*0.1) // Exponential decay by level

		// Simplified book pressure calculation
		bidSum += weight * float64(i+1) * 1000 // Mock bid volume
		askSum += weight * float64(i+1) * 1000 // Mock ask volume
		bidWeight += weight
		askWeight += weight
	}

	if bidWeight > 0 {
		bidPressure = bidSum / bidWeight
	}
	if askWeight > 0 {
		askPressure = askSum / askWeight
	}

	return bidPressure, askPressure
}

func (s *MicroStructureStrategy) calculateMarketImpact() float64 {
	s.Context.mu.RLock()
	defer s.Context.mu.RUnlock()

	// Estimate market impact based on recent order flow
	return s.Context.OrderFlow.MicroPriceImpact * 0.1
}

func (s *MicroStructureStrategy) calculateAdverseSelection() float64 {
	s.Context.mu.RLock()
	defer s.Context.mu.RUnlock()

	// Measure adverse selection from informed trading
	return s.Context.OrderFlow.InformedTrading * 0.05
}

func (s *MicroStructureStrategy) optimizeBidPlacement(mid, spread, skew float64) float64 {
	targetBid := mid * (1 - spread/2 - skew)

	// Round to tick size
	ticks := math.Floor(targetBid/s.TickSize + 0.5)
	return ticks * s.TickSize
}

func (s *MicroStructureStrategy) optimizeAskPlacement(mid, spread, skew float64) float64 {
	targetAsk := mid * (1 + spread/2 + skew)

	// Round to tick size
	ticks := math.Ceil(targetAsk/s.TickSize - 0.5)
	return ticks * s.TickSize
}

// CrossExchangeArbitrageStrategy monitors price differences across exchanges
type CrossExchangeArbitrageStrategy struct {
	BaseSpread      float64
	MaxPosition     float64
	ArbThreshold    float64 // Minimum arbitrage opportunity
	ExchangeWeights map[string]float64
	Context         *MarketContext
	mu              sync.RWMutex
}

func NewCrossExchangeArbitrageStrategy(baseSpread, maxPosition, arbThreshold float64) *CrossExchangeArbitrageStrategy {
	return &CrossExchangeArbitrageStrategy{
		BaseSpread:   baseSpread,
		MaxPosition:  maxPosition,
		ArbThreshold: arbThreshold,
		ExchangeWeights: map[string]float64{
			"binance":  0.4,
			"coinbase": 0.3,
			"kraken":   0.2,
			"okx":      0.1,
		},
		Context: &MarketContext{},
	}
}

func (s *CrossExchangeArbitrageStrategy) Quote(mid, volatility, inventory float64) (bid, ask float64, size float64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Calculate cross-exchange fair value
	fairValue := s.calculateCrossExchangeFairValue()
	if fairValue == 0 {
		fairValue = mid // Fallback to current mid
	}

	// Detect arbitrage opportunities
	arbOpportunity := (fairValue - mid) / mid
	arbSignal := math.Max(-s.ArbThreshold, math.Min(s.ArbThreshold, arbOpportunity))

	// Adjust spread based on arbitrage signal
	spreadAdjustment := math.Abs(arbSignal) * 2.0 // Tighten spread when arbitrage detected
	adjustedSpread := s.BaseSpread * (1.0 - spreadAdjustment)
	adjustedSpread = math.Max(adjustedSpread, s.BaseSpread*0.3) // Minimum spread

	// Skew quotes toward arbitrage direction
	arbSkew := arbSignal * 0.5
	inventorySkew := inventory * 0.001
	totalSkew := arbSkew + inventorySkew

	bid = mid * (1 - adjustedSpread/2 - totalSkew)
	ask = mid * (1 + adjustedSpread/2 + totalSkew)

	// Size based on arbitrage strength and inventory capacity
	arbStrength := math.Abs(arbSignal) / s.ArbThreshold
	inventoryCapacity := math.Max(0.1, 1.0-math.Abs(inventory)/s.MaxPosition)
	size = s.MaxPosition * 0.1 * arbStrength * inventoryCapacity

	return bid, ask, size
}

func (s *CrossExchangeArbitrageStrategy) calculateCrossExchangeFairValue() float64 {
	s.Context.mu.RLock()
	defer s.Context.mu.RUnlock()

	if len(s.Context.CrossExchangeData) == 0 {
		return 0.0
	}

	weightedSum := 0.0
	totalWeight := 0.0

	for exchange, price := range s.Context.CrossExchangeData {
		if weight, exists := s.ExchangeWeights[exchange]; exists && price > 0 {
			weightedSum += price * weight
			totalWeight += weight
		}
	}

	if totalWeight > 0 {
		return weightedSum / totalWeight
	}

	return 0.0
}

// Example: random strategy for testing (existing)
func RandomStrategy(mid float64) (bid, ask, size float64) {
	spread := 0.001 + rand.Float64()*0.002
	bid = mid * (1 - spread/2)
	ask = mid * (1 + spread/2)
	size = 100.0 + rand.Float64()*200.0
	return bid, ask, size
}
