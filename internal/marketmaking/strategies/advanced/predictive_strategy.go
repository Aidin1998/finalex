package advanced

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Aidin1998/finalex/internal/marketmaking/strategies/common"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// PredictiveStrategy implements a machine learning-based market making strategy
type PredictiveStrategy struct {
	config             common.StrategyConfig
	name               string
	version            string
	status             common.StrategyStatus
	metrics            *common.StrategyMetrics
	mu                 sync.RWMutex
	maxPosition        decimal.Decimal
	targetSpread       decimal.Decimal
	predictionAlpha    decimal.Decimal
	priceHistory       []decimal.Decimal
	volatilityHistory  []decimal.Decimal
	maxHistoryLength   int
	modelWeights       []decimal.Decimal
	rebalanceThreshold decimal.Decimal
	minConfidence      decimal.Decimal
	baseSpread         decimal.Decimal
	stopChan           chan struct{}
}

// NewPredictiveStrategy creates a new predictive market making strategy
func NewPredictiveStrategy(config common.StrategyConfig) (common.MarketMakingStrategy, error) {
	// Default values
	maxPosition := decimal.NewFromFloat(1000.0)
	targetSpread := decimal.NewFromFloat(0.002)
	predictionAlpha := decimal.NewFromFloat(0.1)
	baseSpread := decimal.NewFromFloat(0.001)
	minConfidence := decimal.NewFromFloat(0.2)

	// Parse config parameters
	if val, ok := config.Parameters["max_position"]; ok {
		if f, ok := val.(float64); ok {
			maxPosition = decimal.NewFromFloat(f)
		}
	}
	if val, ok := config.Parameters["target_spread"]; ok {
		if f, ok := val.(float64); ok {
			targetSpread = decimal.NewFromFloat(f)
		}
	}
	if val, ok := config.Parameters["prediction_alpha"]; ok {
		if f, ok := val.(float64); ok {
			predictionAlpha = decimal.NewFromFloat(f)
		}
	}
	if val, ok := config.Parameters["base_spread"]; ok {
		if f, ok := val.(float64); ok {
			baseSpread = decimal.NewFromFloat(f)
		}
	}

	strategy := &PredictiveStrategy{
		config:             config,
		name:               "PredictiveStrategy",
		version:            "1.0.0",
		status:             common.StatusStopped,
		metrics:            &common.StrategyMetrics{},
		maxPosition:        maxPosition,
		targetSpread:       targetSpread,
		predictionAlpha:    predictionAlpha,
		baseSpread:         baseSpread,
		minConfidence:      minConfidence,
		priceHistory:       make([]decimal.Decimal, 0, 200),
		volatilityHistory:  make([]decimal.Decimal, 0, 100),
		maxHistoryLength:   200,
		modelWeights:       make([]decimal.Decimal, 10),
		rebalanceThreshold: decimal.NewFromFloat(0.05),
		stopChan:           make(chan struct{}),
	}

	// Initialize model weights with default values
	for i := range strategy.modelWeights {
		strategy.modelWeights[i] = decimal.NewFromFloat(0.1)
	}

	return strategy, nil
}

// Initialize prepares the strategy for trading
func (s *PredictiveStrategy) Initialize(ctx context.Context, config common.StrategyConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.config = config
	s.status = common.StatusStopped
	return nil
}

// Start begins strategy execution
func (s *PredictiveStrategy) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.status != common.StatusStopped {
		return fmt.Errorf("cannot start strategy in status: %s", s.status)
	}

	s.status = common.StatusRunning
	return nil
}

// Stop halts strategy execution
func (s *PredictiveStrategy) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.status != common.StatusRunning {
		return fmt.Errorf("cannot stop strategy in status: %s", s.status)
	}

	if s.stopChan != nil {
		close(s.stopChan)
		s.stopChan = make(chan struct{})
	}
	s.status = common.StatusStopped
	return nil
}

// Quote generates predictive bid/ask quotes
func (s *PredictiveStrategy) Quote(ctx context.Context, input common.QuoteInput) (*common.QuoteOutput, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.status != common.StatusRunning {
		return nil, fmt.Errorf("strategy not running")
	}

	// Update price history
	s.updatePriceHistory(input.MidPrice)

	// Calculate predictive components
	momentum := s.calculateMomentum()
	meanReversion := s.calculateMeanReversion(input.MidPrice)

	// Predict short-term price movement
	momentumWeight := decimal.NewFromFloat(0.3)
	reversionWeight := decimal.NewFromFloat(0.1)
	predictedMove := momentum.Mul(momentumWeight).Sub(meanReversion.Mul(reversionWeight))

	// Calculate prediction confidence
	confidence := s.calculatePredictionConfidence()

	// Adjust spread based on prediction and confidence
	one := decimal.NewFromFloat(1.0)
	two := decimal.NewFromFloat(2.0)
	half := decimal.NewFromFloat(0.5)

	spreadMultiplier := one.Add(predictedMove.Abs().Mul(two)).Add(one.Sub(confidence).Mul(half))
	adjustedSpread := s.baseSpread.Mul(spreadMultiplier)

	// Inventory-based skew with predictive adjustment
	inventoryFactor := decimal.NewFromFloat(0.001)
	inventorySkew := input.Inventory.Mul(inventoryFactor)
	predictiveSkew := predictedMove.Mul(confidence).Mul(half)
	totalSkew := inventorySkew.Add(predictiveSkew)

	// Calculate asymmetric spreads using max function equivalent
	bidSpread := adjustedSpread.Div(two)
	if totalSkew.GreaterThan(decimal.Zero) {
		bidSpread = bidSpread.Add(totalSkew)
	}

	askSpread := adjustedSpread.Div(two)
	negTotalSkew := totalSkew.Neg()
	if negTotalSkew.GreaterThan(decimal.Zero) {
		askSpread = askSpread.Add(negTotalSkew)
	}

	bid := input.MidPrice.Mul(one.Sub(bidSpread))
	ask := input.MidPrice.Mul(one.Add(askSpread))

	// Dynamic sizing based on confidence
	baseSize := s.maxPosition.Mul(decimal.NewFromFloat(0.1))
	confidenceFactor := confidence
	if s.minConfidence.GreaterThan(confidence) {
		confidenceFactor = s.minConfidence
	}
	adjustedSize := baseSize.Mul(confidenceFactor)

	// Asymmetric sizing based on prediction
	bidSize := adjustedSize
	askSize := adjustedSize

	confidenceThreshold := decimal.NewFromFloat(0.6)
	if predictedMove.GreaterThan(decimal.Zero) && confidence.GreaterThan(confidenceThreshold) {
		// Expecting price to rise - favor buying
		bidSize = bidSize.Mul(decimal.NewFromFloat(1.3))
		askSize = askSize.Mul(decimal.NewFromFloat(0.8))
	} else if predictedMove.LessThan(decimal.Zero) && confidence.GreaterThan(confidenceThreshold) {
		// Expecting price to fall - favor selling
		askSize = askSize.Mul(decimal.NewFromFloat(1.3))
		bidSize = bidSize.Mul(decimal.NewFromFloat(0.8))
	}

	output := &common.QuoteOutput{
		BidPrice:    bid,
		AskPrice:    ask,
		BidSize:     bidSize,
		AskSize:     askSize,
		Confidence:  confidence,
		TTL:         5 * time.Second,
		GeneratedAt: time.Now(),
	}

	// Update metrics
	s.updateMetrics(output)

	return output, nil
}

// OnMarketData handles market data updates
func (s *PredictiveStrategy) OnMarketData(ctx context.Context, data *common.MarketData) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Update price history from market data
	if data.Price.GreaterThan(decimal.Zero) {
		s.updatePriceHistory(data.Price)
	}

	return nil
}

// OnOrderFill handles order fill notifications
func (s *PredictiveStrategy) OnOrderFill(ctx context.Context, fill *common.OrderFill) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Update metrics
	s.metrics.OrdersFilled++

	// Calculate PnL
	pnl := decimal.Zero
	if fill.Side == "buy" {
		pnl = fill.Price.Mul(fill.Quantity).Neg()
	} else {
		pnl = fill.Price.Mul(fill.Quantity)
	}
	s.metrics.TotalPnL = s.metrics.TotalPnL.Add(pnl)

	return nil
}

// OnOrderCancel handles order cancellation notifications
func (s *PredictiveStrategy) OnOrderCancel(ctx context.Context, orderID uuid.UUID, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.metrics.OrdersCancelled++
	return nil
}

// UpdateConfig updates strategy configuration
func (s *PredictiveStrategy) UpdateConfig(ctx context.Context, config common.StrategyConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.config = config

	// Update parameters from config
	if val, ok := config.Parameters["base_spread"]; ok {
		if f, ok := val.(float64); ok {
			s.baseSpread = decimal.NewFromFloat(f)
		}
	}
	if val, ok := config.Parameters["prediction_alpha"]; ok {
		if f, ok := val.(float64); ok {
			s.predictionAlpha = decimal.NewFromFloat(f)
		}
	}

	return nil
}

// GetConfig returns current strategy configuration
func (s *PredictiveStrategy) GetConfig() common.StrategyConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config
}

// GetMetrics returns current strategy metrics
func (s *PredictiveStrategy) GetMetrics() *common.StrategyMetrics {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Return a copy of the metrics
	metricsCopy := *s.metrics
	return &metricsCopy
}

// GetStatus returns current strategy status
func (s *PredictiveStrategy) GetStatus() common.StrategyStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.status
}

// Name returns the strategy name
func (s *PredictiveStrategy) Name() string {
	return s.name
}

// Version returns the strategy version
func (s *PredictiveStrategy) Version() string {
	return s.version
}

// Description returns the strategy description
func (s *PredictiveStrategy) Description() string {
	return "ML-inspired predictive market making with momentum and mean reversion analysis"
}

// RiskLevel returns the strategy risk level
func (s *PredictiveStrategy) RiskLevel() common.RiskLevel {
	return common.RiskHigh
}

// HealthCheck performs health validation
func (s *PredictiveStrategy) HealthCheck(ctx context.Context) *common.HealthStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	health := &common.HealthStatus{
		IsHealthy:     true,
		Status:        s.status,
		Message:       "Strategy is healthy",
		Checks:        make(map[string]bool),
		LastCheckTime: time.Now(),
	}

	// Check data availability
	health.Checks["sufficient_price_history"] = len(s.priceHistory) >= 10
	if !health.Checks["sufficient_price_history"] {
		health.IsHealthy = false
		health.Message = "Insufficient price history for predictions"
	}

	// Check configuration
	health.Checks["valid_alpha"] = s.predictionAlpha.GreaterThan(decimal.Zero) && s.predictionAlpha.LessThanOrEqual(decimal.NewFromFloat(1.0))
	if !health.Checks["valid_alpha"] {
		health.IsHealthy = false
		health.Message = "Invalid prediction alpha"
	}

	return health
}

// Reset resets the strategy state
func (s *PredictiveStrategy) Reset(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.priceHistory = s.priceHistory[:0]
	s.volatilityHistory = s.volatilityHistory[:0]
	s.metrics = &common.StrategyMetrics{}

	return nil
}

// Helper methods

func (s *PredictiveStrategy) updatePriceHistory(price decimal.Decimal) {
	s.priceHistory = append(s.priceHistory, price)

	// Keep limited history
	if len(s.priceHistory) > s.maxHistoryLength {
		s.priceHistory = s.priceHistory[len(s.priceHistory)-s.maxHistoryLength:]
	}
}

func (s *PredictiveStrategy) calculateMomentum() decimal.Decimal {
	if len(s.priceHistory) < 20 {
		return decimal.Zero
	}

	// Calculate weighted momentum over different horizons
	shortMomentum := s.calculateReturn(5)
	mediumMomentum := s.calculateReturn(15)

	shortWeight := decimal.NewFromFloat(0.7)
	mediumWeight := decimal.NewFromFloat(0.3)

	return shortMomentum.Mul(shortWeight).Add(mediumMomentum.Mul(mediumWeight))
}

func (s *PredictiveStrategy) calculateReturn(periods int) decimal.Decimal {
	if len(s.priceHistory) < periods+1 {
		return decimal.Zero
	}

	current := s.priceHistory[len(s.priceHistory)-1]
	past := s.priceHistory[len(s.priceHistory)-periods-1]

	if past.Equal(decimal.Zero) {
		return decimal.Zero
	}

	return current.Sub(past).Div(past)
}

func (s *PredictiveStrategy) calculateMeanReversion(currentPrice decimal.Decimal) decimal.Decimal {
	window := 50
	if len(s.priceHistory) < window {
		window = len(s.priceHistory)
	}
	if window < 10 {
		return decimal.Zero
	}

	// Calculate moving average
	sum := decimal.Zero
	for i := len(s.priceHistory) - window; i < len(s.priceHistory); i++ {
		sum = sum.Add(s.priceHistory[i])
	}
	avg := sum.Div(decimal.NewFromInt(int64(window)))

	if avg.Equal(decimal.Zero) {
		return decimal.Zero
	}

	return currentPrice.Sub(avg).Div(avg)
}

func (s *PredictiveStrategy) calculatePredictionConfidence() decimal.Decimal {
	// Base confidence based on data availability
	dataFactor := decimal.NewFromFloat(float64(len(s.priceHistory)) / float64(s.maxHistoryLength))
	if dataFactor.GreaterThan(decimal.NewFromFloat(1.0)) {
		dataFactor = decimal.NewFromFloat(1.0)
	}

	// Adjust based on volatility if available
	volFactor := decimal.NewFromFloat(1.0)
	if len(s.volatilityHistory) > 0 {
		recentVol := s.volatilityHistory[len(s.volatilityHistory)-1]
		volFactor = decimal.NewFromFloat(1.0).Div(decimal.NewFromFloat(1.0).Add(recentVol.Mul(decimal.NewFromFloat(20.0))))
	}

	confidence := dataFactor.Mul(volFactor)

	// Apply min/max bounds manually
	if confidence.LessThan(s.minConfidence) {
		confidence = s.minConfidence
	}
	if confidence.GreaterThan(decimal.NewFromFloat(1.0)) {
		confidence = decimal.NewFromFloat(1.0)
	}

	return confidence
}

func (s *PredictiveStrategy) updateMetrics(output *common.QuoteOutput) {
	s.metrics.LastUpdated = time.Now()
	s.metrics.OrdersPlaced++

	// Calculate spread if prices are valid
	if output.BidPrice.GreaterThan(decimal.Zero) && output.AskPrice.GreaterThan(decimal.Zero) {
		spread := output.AskPrice.Sub(output.BidPrice).Div(output.BidPrice)

		// Update running average of spread capture
		if s.metrics.OrdersPlaced == 1 {
			s.metrics.SpreadCapture = spread
		} else {
			// Simple moving average
			weight := decimal.NewFromFloat(1.0).Div(decimal.NewFromInt(s.metrics.OrdersPlaced))
			s.metrics.SpreadCapture = s.metrics.SpreadCapture.Mul(decimal.NewFromFloat(1.0).Sub(weight)).Add(spread.Mul(weight))
		}
	}
}
