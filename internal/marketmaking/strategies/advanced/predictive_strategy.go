package advanced

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/Finalex/internal/marketmaking/strategies/common"
)

// PredictiveStrategy uses ML-inspired algorithms for price prediction and adaptive quoting
type PredictiveStrategy struct {
	config            *PredictiveConfig
	metrics           *common.StrategyMetrics
	status            common.StrategyStatus
	priceHistory      []float64
	returnHistory     []float64
	volatilityHistory []float64
	mu                sync.RWMutex
	stopChan          chan struct{}
}

type PredictiveConfig struct {
	BaseSpread        float64 `json:"base_spread"`
	MaxInventory      float64 `json:"max_inventory"`
	PredictionAlpha   float64 `json:"prediction_alpha"`
	LookbackWindow    int     `json:"lookback_window"`
	MomentumFactor    float64 `json:"momentum_factor"`
	MeanReversionRate float64 `json:"mean_reversion_rate"`
	VolDecayFactor    float64 `json:"vol_decay_factor"`
	MinConfidence     float64 `json:"min_confidence"`
	TickSize          float64 `json:"tick_size"`
}

// NewPredictiveStrategy creates a new predictive market making strategy
func NewPredictiveStrategy() *PredictiveStrategy {
	return &PredictiveStrategy{
		config: &PredictiveConfig{
			BaseSpread:        0.001,
			MaxInventory:      1000.0,
			PredictionAlpha:   0.1,
			LookbackWindow:    50,
			MomentumFactor:    0.3,
			MeanReversionRate: 0.1,
			VolDecayFactor:    0.95,
			MinConfidence:     0.2,
			TickSize:          0.01,
		},
		metrics: &common.StrategyMetrics{
			StartTime: time.Now(),
		},
		status:            common.StatusStopped,
		priceHistory:      make([]float64, 0, 200),
		returnHistory:     make([]float64, 0, 200),
		volatilityHistory: make([]float64, 0, 100),
		stopChan:          make(chan struct{}),
	}
}

// Initialize prepares the strategy for trading
func (s *PredictiveStrategy) Initialize(ctx context.Context, config common.StrategyConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Parse predictive strategy config
	if baseSpread, ok := config.Parameters["base_spread"].(float64); ok {
		s.config.BaseSpread = baseSpread
	}
	if maxInventory, ok := config.Parameters["max_inventory"].(float64); ok {
		s.config.MaxInventory = maxInventory
	}
	if predictionAlpha, ok := config.Parameters["prediction_alpha"].(float64); ok {
		s.config.PredictionAlpha = predictionAlpha
	}
	if lookbackWindow, ok := config.Parameters["lookback_window"].(float64); ok {
		s.config.LookbackWindow = int(lookbackWindow)
	}
	if momentumFactor, ok := config.Parameters["momentum_factor"].(float64); ok {
		s.config.MomentumFactor = momentumFactor
	}
	if meanReversionRate, ok := config.Parameters["mean_reversion_rate"].(float64); ok {
		s.config.MeanReversionRate = meanReversionRate
	}

	s.status = common.StatusInitialized
	return nil
}

// Start begins strategy execution
func (s *PredictiveStrategy) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.status != common.StatusInitialized && s.status != common.StatusStopped {
		return fmt.Errorf("cannot start strategy in status: %s", s.status)
	}

	s.status = common.StatusRunning
	s.metrics.StartTime = time.Now()
	return nil
}

// Stop halts strategy execution
func (s *PredictiveStrategy) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.status != common.StatusRunning {
		return fmt.Errorf("cannot stop strategy in status: %s", s.status)
	}

	close(s.stopChan)
	s.status = common.StatusStopped
	return nil
}

// Quote generates predictive bid/ask quotes
func (s *PredictiveStrategy) Quote(input common.QuoteInput) (common.QuoteOutput, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.status != common.StatusRunning {
		return common.QuoteOutput{}, fmt.Errorf("strategy not running")
	}

	// Update price history
	s.updatePriceHistory(input.MidPrice)

	// Calculate predictive components
	momentum := s.calculateMomentum()
	meanReversion := s.calculateMeanReversion(input.MidPrice)
	volatility := s.calculateVolatility()

	// Predict short-term price movement
	predictedMove := momentum*s.config.MomentumFactor - meanReversion*s.config.MeanReversionRate

	// Calculate prediction confidence
	confidence := s.calculatePredictionConfidence(volatility)

	// Adjust spread based on prediction and confidence
	spreadMultiplier := 1.0 + math.Abs(predictedMove)*2.0 + (1.0-confidence)*0.5
	adjustedSpread := s.config.BaseSpread * spreadMultiplier

	// Inventory-based skew with predictive adjustment
	inventory := input.Inventory
	inventorySkew := inventory * 0.001
	predictiveSkew := predictedMove * confidence * 0.5
	totalSkew := inventorySkew + predictiveSkew

	// Calculate asymmetric spreads
	bidSpread := adjustedSpread/2 + math.Max(0, totalSkew)
	askSpread := adjustedSpread/2 + math.Max(0, -totalSkew)

	bid := input.MidPrice * (1 - bidSpread)
	ask := input.MidPrice * (1 + askSpread)

	// Round to tick size
	if s.config.TickSize > 0 {
		bid = s.roundToTick(bid, false)
		ask = s.roundToTick(ask, true)
	}

	// Dynamic sizing based on confidence and volatility
	baseSize := s.config.MaxInventory * 0.1
	confidenceFactor := math.Max(s.config.MinConfidence, confidence)
	volAdjustment := 1.0 / (1.0 + volatility*10.0)
	adjustedSize := baseSize * confidenceFactor * volAdjustment

	// Asymmetric sizing based on prediction
	bidSize := adjustedSize
	askSize := adjustedSize

	if predictedMove > 0 && confidence > 0.6 {
		// Expecting price to rise - favor buying
		bidSize *= 1.3
		askSize *= 0.8
	} else if predictedMove < 0 && confidence > 0.6 {
		// Expecting price to fall - favor selling
		askSize *= 1.3
		bidSize *= 0.8
	}

	output := common.QuoteOutput{
		BidPrice:   bid,
		AskPrice:   ask,
		BidSize:    bidSize,
		AskSize:    askSize,
		Confidence: confidence,
		Timestamp:  time.Now(),
	}

	// Update metrics
	s.updateMetrics(output, predictedMove, confidence)

	return output, nil
}

// OnMarketData handles market data updates
func (s *PredictiveStrategy) OnMarketData(data common.MarketData) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Update price history from market data
	if data.Price > 0 {
		s.updatePriceHistory(data.Price)
	}

	// Process recent trades for pattern recognition
	if len(data.RecentTrades) > 1 {
		s.analyzeTradingPatterns(data.RecentTrades)
	}

	return nil
}

// OnOrderFill handles order fill notifications
func (s *PredictiveStrategy) OnOrderFill(fill common.OrderFill) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Update metrics
	s.metrics.TotalVolume += fill.Quantity
	s.metrics.OrdersPlaced++

	if fill.Side == "buy" {
		s.metrics.BuyVolume += fill.Quantity
	} else {
		s.metrics.SellVolume += fill.Quantity
	}

	// Calculate PnL
	if fill.Side == "buy" {
		s.metrics.TotalPnL -= fill.Price * fill.Quantity
	} else {
		s.metrics.TotalPnL += fill.Price * fill.Quantity
	}

	return nil
}

// OnOrderCancel handles order cancellation notifications
func (s *PredictiveStrategy) OnOrderCancel(cancel common.OrderCancel) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.metrics.OrdersCancelled++
	return nil
}

// UpdateConfig updates strategy configuration
func (s *PredictiveStrategy) UpdateConfig(config common.StrategyConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if baseSpread, ok := config.Parameters["base_spread"].(float64); ok {
		s.config.BaseSpread = baseSpread
	}
	if predictionAlpha, ok := config.Parameters["prediction_alpha"].(float64); ok {
		s.config.PredictionAlpha = predictionAlpha
	}
	if momentumFactor, ok := config.Parameters["momentum_factor"].(float64); ok {
		s.config.MomentumFactor = momentumFactor
	}
	if meanReversionRate, ok := config.Parameters["mean_reversion_rate"].(float64); ok {
		s.config.MeanReversionRate = meanReversionRate
	}

	return nil
}

// GetMetrics returns current strategy metrics
func (s *PredictiveStrategy) GetMetrics() common.StrategyMetrics {
	s.mu.RLock()
	defer s.mu.RUnlock()

	metrics := *s.metrics
	metrics.Uptime = time.Since(s.metrics.StartTime)

	if s.metrics.OrdersPlaced > 0 {
		metrics.SuccessRate = float64(s.metrics.OrdersPlaced-s.metrics.OrdersCancelled) / float64(s.metrics.OrdersPlaced)
	}

	// Add predictive-specific metrics
	if len(s.volatilityHistory) > 0 {
		metrics.CurrentVolatility = s.volatilityHistory[len(s.volatilityHistory)-1]
	}

	return metrics
}

// GetStatus returns current strategy status
func (s *PredictiveStrategy) GetStatus() common.StrategyStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.status
}

// GetInfo returns strategy information
func (s *PredictiveStrategy) GetInfo() common.StrategyInfo {
	return common.StrategyInfo{
		Name:        "Predictive",
		Version:     "1.0.0",
		Description: "ML-inspired predictive market making with momentum and mean reversion",
		Author:      "Finalex Team",
		RiskLevel:   common.RiskHigh,
		Complexity:  common.ComplexityAdvanced,
		Parameters: []common.ParameterDefinition{
			{
				Name:        "base_spread",
				Type:        "float",
				Description: "Base spread as a fraction of mid price",
				Default:     0.001,
				MinValue:    0.0001,
				MaxValue:    0.1,
				Required:    true,
			},
			{
				Name:        "max_inventory",
				Type:        "float",
				Description: "Maximum inventory position",
				Default:     1000.0,
				MinValue:    100.0,
				MaxValue:    100000.0,
				Required:    true,
			},
			{
				Name:        "prediction_alpha",
				Type:        "float",
				Description: "Learning rate for predictions",
				Default:     0.1,
				MinValue:    0.01,
				MaxValue:    1.0,
				Required:    true,
			},
			{
				Name:        "lookback_window",
				Type:        "int",
				Description: "Historical data window size",
				Default:     50,
				MinValue:    10,
				MaxValue:    200,
				Required:    false,
			},
			{
				Name:        "momentum_factor",
				Type:        "float",
				Description: "Momentum component weight",
				Default:     0.3,
				MinValue:    0.0,
				MaxValue:    2.0,
				Required:    false,
			},
			{
				Name:        "mean_reversion_rate",
				Type:        "float",
				Description: "Mean reversion component weight",
				Default:     0.1,
				MinValue:    0.0,
				MaxValue:    1.0,
				Required:    false,
			},
		},
	}
}

// HealthCheck performs health validation
func (s *PredictiveStrategy) HealthCheck() common.HealthStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	health := common.HealthStatus{
		IsHealthy: true,
		Timestamp: time.Now(),
	}

	// Check data availability
	if len(s.priceHistory) < 10 {
		health.Issues = append(health.Issues, "Insufficient price history for predictions")
	}

	if len(s.priceHistory) < s.config.LookbackWindow/2 {
		health.Issues = append(health.Issues, "Limited historical data available")
	}

	// Check configuration
	if s.config.PredictionAlpha <= 0 || s.config.PredictionAlpha > 1 {
		health.IsHealthy = false
		health.Issues = append(health.Issues, "Invalid prediction alpha")
	}

	return health
}

// Helper methods

func (s *PredictiveStrategy) updatePriceHistory(price float64) {
	s.priceHistory = append(s.priceHistory, price)

	// Keep limited history
	maxHistory := s.config.LookbackWindow * 2
	if len(s.priceHistory) > maxHistory {
		s.priceHistory = s.priceHistory[len(s.priceHistory)-maxHistory:]
	}

	// Update returns
	if len(s.priceHistory) > 1 {
		prevPrice := s.priceHistory[len(s.priceHistory)-2]
		if prevPrice > 0 {
			ret := (price - prevPrice) / prevPrice
			s.returnHistory = append(s.returnHistory, ret)

			if len(s.returnHistory) > maxHistory {
				s.returnHistory = s.returnHistory[len(s.returnHistory)-maxHistory:]
			}
		}
	}
}

func (s *PredictiveStrategy) calculateMomentum() float64 {
	if len(s.priceHistory) < 20 {
		return 0.0
	}

	// Calculate weighted momentum over different horizons
	shortMomentum := s.calculateReturn(5)
	mediumMomentum := s.calculateReturn(15)

	return 0.7*shortMomentum + 0.3*mediumMomentum
}

func (s *PredictiveStrategy) calculateReturn(periods int) float64 {
	if len(s.priceHistory) < periods+1 {
		return 0.0
	}

	current := s.priceHistory[len(s.priceHistory)-1]
	past := s.priceHistory[len(s.priceHistory)-periods-1]

	if past <= 0 {
		return 0.0
	}

	return (current - past) / past
}

func (s *PredictiveStrategy) calculateMeanReversion(currentPrice float64) float64 {
	window := math.Min(float64(s.config.LookbackWindow), float64(len(s.priceHistory)))
	if window < 10 {
		return 0.0
	}

	// Calculate moving average
	sum := 0.0
	count := int(window)
	for i := len(s.priceHistory) - count; i < len(s.priceHistory); i++ {
		sum += s.priceHistory[i]
	}
	avg := sum / window

	if avg <= 0 {
		return 0.0
	}

	return (currentPrice - avg) / avg
}

func (s *PredictiveStrategy) calculateVolatility() float64 {
	if len(s.returnHistory) < 10 {
		return 0.01 // Default volatility
	}

	// Calculate rolling volatility
	window := math.Min(30, float64(len(s.returnHistory)))
	startIdx := len(s.returnHistory) - int(window)

	var sum, sumSq float64
	for i := startIdx; i < len(s.returnHistory); i++ {
		ret := s.returnHistory[i]
		sum += ret
		sumSq += ret * ret
	}

	mean := sum / window
	variance := (sumSq / window) - (mean * mean)

	if variance < 0 {
		variance = 0
	}

	vol := math.Sqrt(variance)

	// Update volatility history
	s.volatilityHistory = append(s.volatilityHistory, vol)
	if len(s.volatilityHistory) > 100 {
		s.volatilityHistory = s.volatilityHistory[1:]
	}

	return vol
}

func (s *PredictiveStrategy) calculatePredictionConfidence(volatility float64) float64 {
	// Base confidence inversely related to volatility
	baseConfidence := 1.0 / (1.0 + volatility*20.0)

	// Adjust based on data availability
	dataFactor := math.Min(1.0, float64(len(s.priceHistory))/float64(s.config.LookbackWindow))

	confidence := baseConfidence * dataFactor
	return math.Max(s.config.MinConfidence, math.Min(1.0, confidence))
}

func (s *PredictiveStrategy) analyzeTradingPatterns(trades []common.Trade) {
	// Simple pattern analysis - could be extended with more sophisticated ML
	if len(trades) < 3 {
		return
	}

	// Analyze trade size patterns, timing, etc.
	// This is a placeholder for more advanced pattern recognition
}

func (s *PredictiveStrategy) roundToTick(price float64, roundUp bool) float64 {
	if s.config.TickSize <= 0 {
		return price
	}

	if roundUp {
		return math.Ceil(price/s.config.TickSize) * s.config.TickSize
	} else {
		return math.Floor(price/s.config.TickSize) * s.config.TickSize
	}
}

func (s *PredictiveStrategy) updateMetrics(output common.QuoteOutput, predictedMove, confidence float64) {
	s.metrics.LastUpdate = time.Now()
	s.metrics.QuotesGenerated++

	if output.BidPrice > 0 && output.AskPrice > 0 {
		spread := (output.AskPrice - output.BidPrice) / output.BidPrice
		s.metrics.AvgSpread = (s.metrics.AvgSpread*float64(s.metrics.QuotesGenerated-1) + spread) / float64(s.metrics.QuotesGenerated)
	}

	// Store prediction metrics for analysis
	s.metrics.PredictionAccuracy = confidence
}
