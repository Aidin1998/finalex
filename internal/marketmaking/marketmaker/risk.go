package marketmaker

import (
	"math"
	"sync"
	"time"

	"github.com/yourusername/yourproject/common" // Adjust the import path as necessary
)

// Enhanced risk management structures
type VaRParams struct {
	ConfidenceLevel float64 // e.g., 0.95 for 95% VaR
	TimeHorizon     time.Duration
	Method          VaRMethod
}

type VaRMethod int

const (
	HistoricalVaR VaRMethod = iota
	ParametricVaR
	MonteCarloVaR
)

type PortfolioMetrics struct {
	TotalValue     float64
	DailyVaR       float64
	ExpectedReturn float64
	Volatility     float64
	Sharpe         float64
	MaxDrawdown    float64
	Correlation    map[string]float64
	BetaExposure   float64
}

type PositionRisk struct {
	Symbol        string
	Position      float64
	MarketValue   float64
	DeltaExposure float64
	GammaExposure float64
	VegaExposure  float64
	ThetaExposure float64
}

type RiskLimits struct {
	MaxInventoryPerSymbol map[string]float64
	MaxTotalInventory     float64
	MaxDailyLoss          float64
	MaxInstrumentWeight   float64
	MaxConcentration      float64
	MaxLeverage           float64
	MaxDailyVaR           float64
	StopLossThreshold     float64
}

type RiskManager struct {
	// Basic risk tracking (existing)
	maxInventory float64
	maxLoss      float64
	inventory    float64
	pnl          float64

	// Advanced risk components
	limits            RiskLimits
	positions         map[string]*PositionRisk
	portfolio         *PortfolioMetrics
	priceHistory      map[string][]float64
	correlationMatrix map[string]map[string]float64
	varParams         VaRParams
	lastRiskUpdate    time.Time
	riskSignals       []common.RiskSignal // Use the common.RiskSignal type

	// Kelly criterion parameters
	kellyEnabled    bool
	expectedReturns map[string]float64
	returnVariances map[string]float64

	mu sync.RWMutex
}

const (
	LowRisk RiskSeverity = iota
	MediumRisk
	HighRisk
	CriticalRisk
)

func NewRiskManager(maxInv, maxLoss float64) *RiskManager {
	return &RiskManager{
		maxInventory:      maxInv,
		maxLoss:           maxLoss,
		positions:         make(map[string]*PositionRisk),
		priceHistory:      make(map[string][]float64),
		correlationMatrix: make(map[string]map[string]float64),
		expectedReturns:   make(map[string]float64),
		returnVariances:   make(map[string]float64),
		portfolio: &PortfolioMetrics{
			Correlation: make(map[string]float64),
		},
		varParams: VaRParams{
			ConfidenceLevel: 0.95,
			TimeHorizon:     24 * time.Hour,
			Method:          HistoricalVaR,
		},
		limits: RiskLimits{
			MaxInventoryPerSymbol: make(map[string]float64),
			MaxTotalInventory:     maxInv,
			MaxDailyLoss:          maxLoss,
			MaxInstrumentWeight:   0.3,
			MaxConcentration:      0.5,
			MaxLeverage:           2.0,
			MaxDailyVaR:           maxLoss * 0.5,
			StopLossThreshold:     maxLoss * 0.8,
		},
		kellyEnabled: true,
	}
}

// Enhanced risk management methods

func (r *RiskManager) UpdateInventory(delta float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.inventory += delta
}

func (r *RiskManager) UpdatePnL(delta float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pnl += delta
	r.portfolio.TotalValue += delta
}

func (r *RiskManager) UpdatePositionRisk(symbol string, position, price float64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.positions[symbol] == nil {
		r.positions[symbol] = &PositionRisk{Symbol: symbol}
	}

	pos := r.positions[symbol]
	pos.Position = position
	pos.MarketValue = position * price
	pos.DeltaExposure = position // Simplified delta = position for spot

	// Update price history for VaR calculation
	r.updatePriceHistory(symbol, price)
}

func (r *RiskManager) updatePriceHistory(symbol string, price float64) {
	if r.priceHistory[symbol] == nil {
		r.priceHistory[symbol] = make([]float64, 0, 252) // One year of daily data
	}

	r.priceHistory[symbol] = append(r.priceHistory[symbol], price)
	if len(r.priceHistory[symbol]) > 252 {
		r.priceHistory[symbol] = r.priceHistory[symbol][1:]
	}
}

// Value at Risk calculation
func (r *RiskManager) CalculateVaR(symbol string) float64 {
	r.mu.RLock()
	defer r.mu.RUnlock()

	position, exists := r.positions[symbol]
	if !exists || len(r.priceHistory[symbol]) < 30 {
		return 0.0
	}

	switch r.varParams.Method {
	case HistoricalVaR:
		return r.calculateHistoricalVaR(symbol, position)
	case ParametricVaR:
		return r.calculateParametricVaR(symbol, position)
	case MonteCarloVaR:
		return r.calculateMonteCarloVaR(symbol, position)
	default:
		return r.calculateHistoricalVaR(symbol, position)
	}
}

func (r *RiskManager) calculateHistoricalVaR(symbol string, position *PositionRisk) float64 {
	prices := r.priceHistory[symbol]
	if len(prices) < 2 {
		return 0.0
	}

	// Calculate historical returns
	returns := make([]float64, len(prices)-1)
	for i := 1; i < len(prices); i++ {
		returns[i-1] = (prices[i] - prices[i-1]) / prices[i-1]
	}

	// Sort returns for percentile calculation
	for i := 0; i < len(returns); i++ {
		for j := i + 1; j < len(returns); j++ {
			if returns[i] > returns[j] {
				returns[i], returns[j] = returns[j], returns[i]
			}
		}
	}

	// Calculate VaR at specified confidence level
	percentile := int((1.0 - r.varParams.ConfidenceLevel) * float64(len(returns)))
	if percentile < len(returns) {
		varReturn := returns[percentile]
		return math.Abs(varReturn * position.MarketValue)
	}

	return 0.0
}

func (r *RiskManager) calculateParametricVaR(symbol string, position *PositionRisk) float64 {
	prices := r.priceHistory[symbol]
	if len(prices) < 30 {
		return 0.0
	}

	// Calculate mean and standard deviation of returns
	mean, stdDev := r.calculateReturnStatistics(prices)

	// Calculate z-score for confidence level
	zScore := r.getZScore(r.varParams.ConfidenceLevel)

	// Parametric VaR = (mean - z * sigma) * position value
	varReturn := mean - zScore*stdDev
	return math.Abs(varReturn * position.MarketValue)
}

func (r *RiskManager) calculateMonteCarloVaR(symbol string, position *PositionRisk) float64 {
	// Simplified Monte Carlo simulation
	prices := r.priceHistory[symbol]
	if len(prices) < 30 {
		return 0.0
	}

	mean, stdDev := r.calculateReturnStatistics(prices)
	simulations := 10000
	results := make([]float64, simulations)

	for i := 0; i < simulations; i++ {
		// Generate random return using normal distribution
		randomReturn := mean + stdDev*r.normalRandom()
		results[i] = randomReturn * position.MarketValue
	}

	// Sort results and find VaR percentile
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[i] > results[j] {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	percentile := int((1.0 - r.varParams.ConfidenceLevel) * float64(simulations))
	if percentile < len(results) {
		return math.Abs(results[percentile])
	}

	return 0.0
}

func (r *RiskManager) calculateReturnStatistics(prices []float64) (mean, stdDev float64) {
	if len(prices) < 2 {
		return 0.0, 0.0
	}

	// Calculate returns
	returns := make([]float64, len(prices)-1)
	for i := 1; i < len(prices); i++ {
		returns[i-1] = (prices[i] - prices[i-1]) / prices[i-1]
	}

	// Calculate mean
	sum := 0.0
	for _, ret := range returns {
		sum += ret
	}
	mean = sum / float64(len(returns))

	// Calculate standard deviation
	sumSq := 0.0
	for _, ret := range returns {
		diff := ret - mean
		sumSq += diff * diff
	}
	stdDev = math.Sqrt(sumSq / float64(len(returns)-1))

	return mean, stdDev
}

func (r *RiskManager) getZScore(confidenceLevel float64) float64 {
	// Approximate z-scores for common confidence levels
	switch confidenceLevel {
	case 0.90:
		return 1.282
	case 0.95:
		return 1.645
	case 0.99:
		return 2.326
	default:
		return 1.645 // Default to 95%
	}
}

func (r *RiskManager) normalRandom() float64 {
	// Simple Box-Muller transformation for normal distribution
	u1 := math.Mod(float64(time.Now().UnixNano()), 1000000) / 1000000.0
	u2 := math.Mod(float64(time.Now().UnixNano()>>10), 1000000) / 1000000.0

	return math.Sqrt(-2*math.Log(u1)) * math.Cos(2*math.Pi*u2)
}

// Kelly Criterion position sizing
func (r *RiskManager) CalculateKellySize(symbol string, expectedReturn, variance float64) float64 {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if !r.kellyEnabled || variance <= 0 {
		return 0.0
	}

	// Kelly fraction = (expected return) / (variance)
	kellyFraction := expectedReturn / variance

	// Apply Kelly constraint (usually limited to prevent over-leverage)
	maxKellyFraction := 0.25 // Conservative limit
	kellyFraction = math.Max(-maxKellyFraction, math.Min(maxKellyFraction, kellyFraction))

	// Convert to position size based on available capital
	availableCapital := r.getAvailableCapital()
	return kellyFraction * availableCapital
}

func (r *RiskManager) getAvailableCapital() float64 {
	// Simplified available capital calculation
	return math.Max(0, r.limits.MaxTotalInventory-r.getTotalExposure())
}

func (r *RiskManager) getTotalExposure() float64 {
	totalExposure := 0.0
	for _, position := range r.positions {
		totalExposure += math.Abs(position.MarketValue)
	}
	return totalExposure
}

// Portfolio-level risk metrics
func (r *RiskManager) UpdatePortfolioMetrics() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.portfolio.TotalValue = r.calculateTotalPortfolioValue()
	r.portfolio.DailyVaR = r.calculatePortfolioVaR()
	r.portfolio.Volatility = r.calculatePortfolioVolatility()
	r.portfolio.MaxDrawdown = r.calculateMaxDrawdown()
	r.portfolio.Sharpe = r.calculateSharpeRatio()

	r.lastRiskUpdate = time.Now()
}

func (r *RiskManager) calculateTotalPortfolioValue() float64 {
	total := 0.0
	for _, position := range r.positions {
		total += position.MarketValue
	}
	return total
}

func (r *RiskManager) calculatePortfolioVaR() float64 {
	// Simplified portfolio VaR (should include correlations)
	totalVaR := 0.0
	for symbol := range r.positions {
		symbolVaR := r.CalculateVaR(symbol)
		totalVaR += symbolVaR * symbolVaR // Assuming independence for simplicity
	}
	return math.Sqrt(totalVaR)
}

func (r *RiskManager) calculatePortfolioVolatility() float64 {
	// Simplified portfolio volatility calculation
	weightedVol := 0.0
	totalValue := r.portfolio.TotalValue

	if totalValue == 0 {
		return 0.0
	}

	for symbol, position := range r.positions {
		weight := math.Abs(position.MarketValue) / totalValue
		symbolVol := r.calculateSymbolVolatility(symbol)
		weightedVol += weight * symbolVol
	}

	return weightedVol
}

func (r *RiskManager) calculateSymbolVolatility(symbol string) float64 {
	prices := r.priceHistory[symbol]
	if len(prices) < 30 {
		return 0.0
	}

	_, stdDev := r.calculateReturnStatistics(prices)
	return stdDev * math.Sqrt(252) // Annualized volatility
}

func (r *RiskManager) calculateMaxDrawdown() float64 {
	// Simplified max drawdown calculation
	return r.portfolio.MaxDrawdown // Would need PnL history for proper calculation
}

func (r *RiskManager) calculateSharpeRatio() float64 {
	if r.portfolio.Volatility == 0 {
		return 0.0
	}

	riskFreeRate := 0.02 // 2% risk-free rate
	return (r.portfolio.ExpectedReturn - riskFreeRate) / r.portfolio.Volatility
}

// Enhanced risk checking
func (r *RiskManager) Breach() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check basic limits
	if r.inventory > r.maxInventory || r.pnl < -r.maxLoss {
		return true
	}

	// Check advanced risk limits
	if r.portfolio.DailyVaR > r.limits.MaxDailyVaR {
		r.addRiskSignal(VaRBreach, HighRisk, "Portfolio VaR exceeded", "", r.portfolio.DailyVaR)
		return true
	}

	if r.pnl < -r.limits.StopLossThreshold {
		r.addRiskSignal(PnLBreach, CriticalRisk, "Stop loss threshold breached", "", r.pnl)
		return true
	}

	// Check concentration limits
	totalValue := r.portfolio.TotalValue
	if totalValue > 0 {
		for symbol, position := range r.positions {
			concentration := math.Abs(position.MarketValue) / totalValue
			if concentration > r.limits.MaxConcentration {
				r.addRiskSignal(InventoryBreach, MediumRisk, "Concentration limit exceeded", symbol, concentration)
				return true
			}
		}
	}

	return false
}

func (r *RiskManager) addRiskSignal(signalType RiskSignalType, severity RiskSeverity, message, symbol string, value float64) {
	signal := common.RiskSignal{
		Timestamp: time.Now(),
		Type:      signalType,
		Severity:  severity,
		Message:   message,
		Symbol:    symbol,
		Value:     value,
	}

	r.riskSignals = append(r.riskSignals, signal)

	// Keep only recent signals
	if len(r.riskSignals) > 1000 {
		r.riskSignals = r.riskSignals[len(r.riskSignals)-1000:]
	}
}

func (r *RiskManager) GetRiskSignals(severity RiskSeverity) []common.RiskSignal {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var filtered []common.RiskSignal
	for _, signal := range r.riskSignals {
		if signal.Severity >= severity {
			filtered = append(filtered, signal)
		}
	}

	return filtered
}

// Existing methods (preserved for compatibility)

func (r *RiskManager) CheckPositionLimit(limit float64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.inventory <= limit
}

func (r *RiskManager) CheckPnLLimit(limit float64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.pnl >= -limit
}

func (r *RiskManager) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.inventory = 0
	r.pnl = 0
	r.portfolio.TotalValue = 0

	// Clear positions but keep risk parameters
	for symbol := range r.positions {
		r.positions[symbol] = &PositionRisk{Symbol: symbol}
	}
}

func (r *RiskManager) EmergencyStop() {
	// Enhanced emergency stop with detailed logging
	r.mu.Lock()
	defer r.mu.Unlock()

	r.addRiskSignal(PnLBreach, CriticalRisk, "EMERGENCY STOP TRIGGERED", "", r.pnl)

	// In production, trigger a kill switch for all quoting
	panic("Market making risk limits breached: EMERGENCY STOP")
}
