package basic

import (
	"context"
	"fmt"

	"time"

	"github.com/Aidin1998/finalex/internal/marketmaking/strategies/common"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Core interface methods

func (s *DynamicStrategy) Quote(ctx context.Context, input common.QuoteInput) (*common.QuoteOutput, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.status != common.StatusRunning {
		return nil, fmt.Errorf("strategy not running")
	}

	// Update volatility if provided
	if input.Volatility.GreaterThan(decimal.Zero) {
		s.updateVolatility(input.Volatility)
	}

	// Calculate dynamic spread based on volatility
	dynamicSpread := s.calculateDynamicSpread()

	// Calculate adaptive size
	adaptiveSize := s.calculateAdaptiveSize()

	// Calculate quotes
	halfSpread := dynamicSpread.Div(decimal.NewFromInt(2))
	bid := input.MidPrice.Mul(decimal.NewFromInt(1).Sub(halfSpread))
	ask := input.MidPrice.Mul(decimal.NewFromInt(1).Add(halfSpread))

	// Round to tick size if specified
	if s.tickSize.GreaterThan(decimal.Zero) {
		bid = s.roundToTick(bid, false)
		ask = s.roundToTick(ask, true)
	}

	// Calculate confidence based on volatility and spread
	confidence := s.calculateConfidence()

	output := &common.QuoteOutput{
		BidPrice:    bid,
		AskPrice:    ask,
		BidSize:     adaptiveSize,
		AskSize:     adaptiveSize,
		Confidence:  confidence,
		TTL:         time.Second * 20, // Shorter TTL for dynamic strategy
		GeneratedAt: time.Now(),
		Metadata: map[string]interface{}{
			"strategy_type":      "dynamic",
			"base_spread":        s.baseSpread.String(),
			"dynamic_spread":     dynamicSpread.String(),
			"current_volatility": s.currentVolatility.String(),
			"adaptive_size":      adaptiveSize.String(),
		},
	}

	// Update metrics
	s.updateQuoteMetrics(output)

	return output, nil
}

func (s *DynamicStrategy) Initialize(ctx context.Context, config common.StrategyConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.config = config
	if err := s.parseConfig(); err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	// Reset volatility tracking
	s.volatilityHistory = make([]decimal.Decimal, 0)
	s.currentVolatility = decimal.Zero

	s.status = common.StatusStarting
	s.metrics.LastUpdated = time.Now()
	return nil
}

func (s *DynamicStrategy) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.status != common.StatusStarting && s.status != common.StatusStopped {
		return fmt.Errorf("cannot start strategy in status: %s", s.status)
	}

	s.status = common.StatusRunning
	s.startTime = time.Now()
	s.metrics.LastUpdated = time.Now()
	return nil
}

func (s *DynamicStrategy) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.status != common.StatusRunning {
		return fmt.Errorf("cannot stop strategy in status: %s", s.status)
	}

	s.status = common.StatusStopped
	s.metrics.LastUpdated = time.Now()
	return nil
}

func (s *DynamicStrategy) OnMarketData(ctx context.Context, data *common.MarketData) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Update volatility based on market data
	if data.Volatility.GreaterThan(decimal.Zero) {
		s.updateVolatility(data.Volatility)
	}

	s.metrics.LastUpdated = time.Now()
	return nil
}

func (s *DynamicStrategy) OnOrderFill(ctx context.Context, fill *common.OrderFill) error {
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
	s.metrics.DailyPnL = s.metrics.DailyPnL.Add(pnl)
	s.metrics.LastTrade = time.Now()
	s.metrics.LastUpdated = time.Now()

	return nil
}

func (s *DynamicStrategy) OnOrderCancel(ctx context.Context, orderID uuid.UUID, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.metrics.OrdersCancelled++
	s.metrics.LastUpdated = time.Now()
	return nil
}

func (s *DynamicStrategy) UpdateConfig(ctx context.Context, config common.StrategyConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.config = config
	if err := s.parseConfig(); err != nil {
		return fmt.Errorf("failed to parse updated config: %w", err)
	}

	s.metrics.LastUpdated = time.Now()
	return nil
}

func (s *DynamicStrategy) GetConfig() common.StrategyConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.config
}

func (s *DynamicStrategy) GetMetrics() *common.StrategyMetrics {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Create a copy of current metrics
	metrics := *s.metrics
	metrics.StrategyUptime = time.Since(s.startTime)

	// Calculate success rate
	if s.metrics.OrdersPlaced > 0 {
		successfulOrders := s.metrics.OrdersPlaced - s.metrics.OrdersCancelled
		metrics.SuccessRate = decimal.NewFromInt(successfulOrders).Div(decimal.NewFromInt(s.metrics.OrdersPlaced))
	}

	return &metrics
}

func (s *DynamicStrategy) GetStatus() common.StrategyStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.status
}

// Metadata methods
func (s *DynamicStrategy) Name() string {
	return "Dynamic Market Making"
}

func (s *DynamicStrategy) Version() string {
	return "1.0.0"
}

func (s *DynamicStrategy) Description() string {
	return "Volatility-adaptive market making strategy with dynamic spread and size adjustments"
}

func (s *DynamicStrategy) RiskLevel() common.RiskLevel {
	return common.RiskMedium
}

func (s *DynamicStrategy) HealthCheck(ctx context.Context) *common.HealthStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	health := &common.HealthStatus{
		IsHealthy:     true,
		Status:        s.status,
		Checks:        make(map[string]bool),
		LastCheckTime: time.Now(),
	}

	// Check configuration validity
	health.Checks["valid_base_spread"] = s.baseSpread.GreaterThan(decimal.Zero)
	health.Checks["valid_volatility_factor"] = s.volatilityFactor.GreaterThan(decimal.Zero)
	health.Checks["valid_max_spread"] = s.maxSpread.GreaterThan(s.baseSpread)
	health.Checks["valid_size"] = s.baseSize.GreaterThan(decimal.Zero)
	health.Checks["strategy_running"] = s.status == common.StatusRunning
	health.Checks["has_volatility_data"] = len(s.volatilityHistory) > 0

	// Overall health
	for _, check := range health.Checks {
		if !check {
			health.IsHealthy = false
			health.Message = "Configuration or operational issues detected"
			break
		}
	}

	return health
}

func (s *DynamicStrategy) Reset(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Reset metrics but keep configuration
	s.metrics = &common.StrategyMetrics{
		LastUpdated: time.Now(),
	}
	s.startTime = time.Now()

	// Reset volatility tracking
	s.volatilityHistory = make([]decimal.Decimal, 0)
	s.currentVolatility = decimal.Zero

	return nil
}

// Helper methods

func (s *DynamicStrategy) parseConfig() error {
	params := s.config.Parameters

	if baseSpread, ok := params["base_spread"]; ok {
		if spreadFloat, ok := baseSpread.(float64); ok {
			s.baseSpread = decimal.NewFromFloat(spreadFloat)
		} else if spreadStr, ok := baseSpread.(string); ok {
			var err error
			s.baseSpread, err = decimal.NewFromString(spreadStr)
			if err != nil {
				return fmt.Errorf("invalid base_spread value: %w", err)
			}
		}
	}

	if volatilityFactor, ok := params["volatility_factor"]; ok {
		if factorFloat, ok := volatilityFactor.(float64); ok {
			s.volatilityFactor = decimal.NewFromFloat(factorFloat)
		} else if factorStr, ok := volatilityFactor.(string); ok {
			var err error
			s.volatilityFactor, err = decimal.NewFromString(factorStr)
			if err != nil {
				return fmt.Errorf("invalid volatility_factor value: %w", err)
			}
		}
	}

	if maxSpread, ok := params["max_spread"]; ok {
		if maxFloat, ok := maxSpread.(float64); ok {
			s.maxSpread = decimal.NewFromFloat(maxFloat)
		} else if maxStr, ok := maxSpread.(string); ok {
			var err error
			s.maxSpread, err = decimal.NewFromString(maxStr)
			if err != nil {
				return fmt.Errorf("invalid max_spread value: %w", err)
			}
		}
	}

	if baseSize, ok := params["base_size"]; ok {
		if sizeFloat, ok := baseSize.(float64); ok {
			s.baseSize = decimal.NewFromFloat(sizeFloat)
		} else if sizeStr, ok := baseSize.(string); ok {
			var err error
			s.baseSize, err = decimal.NewFromString(sizeStr)
			if err != nil {
				return fmt.Errorf("invalid base_size value: %w", err)
			}
		}
	}

	if tickSize, ok := params["tick_size"]; ok {
		if tickFloat, ok := tickSize.(float64); ok {
			s.tickSize = decimal.NewFromFloat(tickFloat)
		} else if tickStr, ok := tickSize.(string); ok {
			var err error
			s.tickSize, err = decimal.NewFromString(tickStr)
			if err != nil {
				return fmt.Errorf("invalid tick_size value: %w", err)
			}
		}
	}

	if maxHistory, ok := params["max_history_size"]; ok {
		if historyInt, ok := maxHistory.(int); ok {
			s.maxHistorySize = historyInt
		} else if historyFloat, ok := maxHistory.(float64); ok {
			s.maxHistorySize = int(historyFloat)
		}
	}

	return nil
}

func (s *DynamicStrategy) updateVolatility(volatility decimal.Decimal) {
	// Add to history
	s.volatilityHistory = append(s.volatilityHistory, volatility)

	// Trim history if too long
	if len(s.volatilityHistory) > s.maxHistorySize {
		s.volatilityHistory = s.volatilityHistory[1:]
	}

	// Update current volatility (moving average of recent measurements)
	if len(s.volatilityHistory) > 0 {
		sum := decimal.Zero
		count := len(s.volatilityHistory)
		windowSize := 10 // Use last 10 measurements for moving average
		if count < windowSize {
			windowSize = count
		}

		for i := count - windowSize; i < count; i++ {
			sum = sum.Add(s.volatilityHistory[i])
		}

		s.currentVolatility = sum.Div(decimal.NewFromInt(int64(windowSize)))
	}
}

func (s *DynamicStrategy) calculateDynamicSpread() decimal.Decimal {
	// Base spread + volatility adjustment
	volatilityAdjustment := s.currentVolatility.Mul(s.volatilityFactor)
	dynamicSpread := s.baseSpread.Add(volatilityAdjustment)

	// Cap at max spread
	if dynamicSpread.GreaterThan(s.maxSpread) {
		dynamicSpread = s.maxSpread
	}

	return dynamicSpread
}

func (s *DynamicStrategy) calculateAdaptiveSize() decimal.Decimal {
	// Reduce size when volatility is high
	if s.currentVolatility.IsZero() {
		return s.baseSize
	}

	// Size reduction factor: higher volatility = smaller size
	// Formula: baseSize * (1 / (1 + volatility * 10))
	volatilityImpact := s.currentVolatility.Mul(decimal.NewFromInt(10))
	sizeMultiplier := decimal.NewFromInt(1).Div(decimal.NewFromInt(1).Add(volatilityImpact))
	adaptiveSize := s.baseSize.Mul(sizeMultiplier)

	// Minimum size threshold (don't go below 10% of base size)
	minSize := s.baseSize.Mul(decimal.NewFromFloat(0.1))
	if adaptiveSize.LessThan(minSize) {
		adaptiveSize = minSize
	}

	return adaptiveSize
}

func (s *DynamicStrategy) calculateConfidence() decimal.Decimal {
	baseConfidence := decimal.NewFromFloat(0.7) // Base confidence for dynamic strategy

	// Reduce confidence with higher volatility
	if s.currentVolatility.GreaterThan(decimal.Zero) {
		volatilityPenalty := s.currentVolatility.Mul(decimal.NewFromInt(5)) // Scale factor
		confidence := baseConfidence.Sub(volatilityPenalty)

		// Floor at 0.3
		minConfidence := decimal.NewFromFloat(0.3)
		if confidence.LessThan(minConfidence) {
			confidence = minConfidence
		}

		return confidence
	}

	return baseConfidence
}

func (s *DynamicStrategy) roundToTick(price decimal.Decimal, roundUp bool) decimal.Decimal {
	if s.tickSize.LessThanOrEqual(decimal.Zero) {
		return price
	}

	ticks := price.Div(s.tickSize)
	if roundUp {
		return ticks.Ceil().Mul(s.tickSize)
	} else {
		return ticks.Floor().Mul(s.tickSize)
	}
}

func (s *DynamicStrategy) updateQuoteMetrics(output *common.QuoteOutput) {
	s.metrics.OrdersPlaced++
	s.metrics.LastUpdated = time.Now()

	// Calculate spread capture
	if output.BidPrice.GreaterThan(decimal.Zero) && output.AskPrice.GreaterThan(decimal.Zero) {
		spread := output.AskPrice.Sub(output.BidPrice).Div(output.BidPrice)
		s.metrics.SpreadCapture = spread
	}
}
