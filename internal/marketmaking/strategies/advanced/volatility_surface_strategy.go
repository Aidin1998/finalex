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

// VolatilitySurfaceStrategy models volatility across different time horizons
type VolatilitySurfaceStrategy struct {
	config       common.StrategyConfig
	name         string
	version      string
	status       common.StrategyStatus
	metrics      *common.StrategyMetrics
	mu           sync.RWMutex
	baseSpread   decimal.Decimal
	maxPosition  decimal.Decimal
	volSurface   map[time.Duration]decimal.Decimal
	priceHistory []decimal.Decimal
	maxHistory   int
	lastUpdate   time.Time
	tickSize     decimal.Decimal
	stopChan     chan struct{}
}

// NewVolatilitySurfaceStrategy creates a new volatility surface strategy
func NewVolatilitySurfaceStrategy(config common.StrategyConfig) (common.MarketMakingStrategy, error) {
	// Default values
	baseSpread := decimal.NewFromFloat(0.002)
	maxPosition := decimal.NewFromFloat(1000.0)
	tickSize := decimal.NewFromFloat(0.01)

	// Parse config parameters
	if val, ok := config.Parameters["base_spread"]; ok {
		if f, ok := val.(float64); ok {
			baseSpread = decimal.NewFromFloat(f)
		}
	}
	if val, ok := config.Parameters["max_position"]; ok {
		if f, ok := val.(float64); ok {
			maxPosition = decimal.NewFromFloat(f)
		}
	}
	if val, ok := config.Parameters["tick_size"]; ok {
		if f, ok := val.(float64); ok {
			tickSize = decimal.NewFromFloat(f)
		}
	}

	strategy := &VolatilitySurfaceStrategy{
		config:      config,
		name:        "VolatilitySurface",
		version:     "1.0.0",
		status:      common.StatusStopped,
		baseSpread:  baseSpread,
		maxPosition: maxPosition,
		tickSize:    tickSize,
		maxHistory:  100,
		volSurface:  make(map[time.Duration]decimal.Decimal),
		stopChan:    make(chan struct{}),
		metrics: &common.StrategyMetrics{
			TotalPnL:          decimal.Zero,
			DailyPnL:          decimal.Zero,
			SharpeRatio:       decimal.Zero,
			MaxDrawdown:       decimal.Zero,
			WinRate:           decimal.Zero,
			OrdersPlaced:      0,
			OrdersFilled:      0,
			OrdersCancelled:   0,
			SuccessRate:       decimal.Zero,
			SpreadCapture:     decimal.Zero,
			InventoryTurnover: decimal.Zero,
			QuoteUptime:       decimal.Zero,
			LastUpdated:       time.Now(),
		},
	}

	// Initialize volatility surface with default values
	strategy.volSurface[time.Minute*5] = decimal.NewFromFloat(0.01)
	strategy.volSurface[time.Minute*15] = decimal.NewFromFloat(0.015)
	strategy.volSurface[time.Hour] = decimal.NewFromFloat(0.02)
	strategy.volSurface[time.Hour*4] = decimal.NewFromFloat(0.025)
	strategy.volSurface[time.Hour*24] = decimal.NewFromFloat(0.03)

	return strategy, nil
}

// Initialize prepares the strategy for trading
func (s *VolatilitySurfaceStrategy) Initialize(ctx context.Context, config common.StrategyConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.config = config
	s.status = common.StatusStopped
	return nil
}

// Start begins strategy execution
func (s *VolatilitySurfaceStrategy) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.status != common.StatusStopped {
		return fmt.Errorf("strategy is not in stopped state")
	}

	s.status = common.StatusRunning
	return nil
}

// Stop halts strategy execution
func (s *VolatilitySurfaceStrategy) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.status == common.StatusStopped {
		return nil
	}

	s.status = common.StatusStopped
	close(s.stopChan)
	return nil
}

// Quote generates bid/ask quotes based on volatility surface analysis
func (s *VolatilitySurfaceStrategy) Quote(ctx context.Context, input common.QuoteInput) (*common.QuoteOutput, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.status != common.StatusRunning {
		return nil, fmt.Errorf("strategy is not running")
	}

	// Update price history
	s.updatePriceHistory(input.MidPrice)

	// Calculate implied volatility from recent price movements
	currentVol := s.calculateImpliedVolatility()

	// Update volatility surface
	s.updateVolatilitySurface(currentVol)

	// Create weighted volatility measure
	weightedVol := s.calculateWeightedVolatility()

	// Calculate dynamic spread based on volatility
	volAdjustment := weightedVol.Mul(decimal.NewFromFloat(0.5))
	dynamicSpread := s.baseSpread.Add(volAdjustment)

	// Apply inventory skew
	inventoryFactor := decimal.NewFromFloat(0.01)
	inventoryRisk := input.Inventory.Abs().Div(s.maxPosition)
	inventorySkew := input.Inventory.Mul(inventoryFactor.Add(inventoryRisk.Mul(decimal.NewFromFloat(0.01))))

	// Calculate spreads with skew
	bidSpread := dynamicSpread.Sub(inventorySkew)
	askSpread := dynamicSpread.Add(inventorySkew)

	// Ensure minimum spread
	minSpread := decimal.NewFromFloat(0.0001)
	if bidSpread.LessThan(minSpread) {
		bidSpread = minSpread
	}
	if askSpread.LessThan(minSpread) {
		askSpread = minSpread
	}

	// Apply skew based on inventory imbalance
	one := decimal.NewFromFloat(1.0)
	if input.Inventory.GreaterThan(decimal.Zero) {
		bidSpread = bidSpread.Add(inventorySkew.Abs())
	} else {
		askSpread = askSpread.Add(inventorySkew.Abs())
	}

	// Calculate final prices
	bid := input.MidPrice.Mul(one.Sub(bidSpread))
	ask := input.MidPrice.Mul(one.Add(askSpread))

	// Dynamic sizing based on volatility and inventory
	baseSize := s.maxPosition.Mul(decimal.NewFromFloat(0.1))
	volSizeFactor := decimal.NewFromFloat(1.0).Sub(weightedVol.Mul(decimal.NewFromFloat(0.5)))
	size := baseSize.Mul(volSizeFactor)

	// Confidence based on volatility surface quality
	confidence := s.calculateConfidence()

	// Update metrics
	s.metrics.OrdersPlaced++
	s.metrics.LastUpdated = time.Now()

	output := &common.QuoteOutput{
		BidPrice:    bid,
		AskPrice:    ask,
		BidSize:     size,
		AskSize:     size,
		Confidence:  decimal.NewFromFloat(confidence),
		TTL:         time.Second * 30,
		GeneratedAt: time.Now(),
	}

	return output, nil
}

// OnMarketData processes incoming market data
func (s *VolatilitySurfaceStrategy) OnMarketData(ctx context.Context, data *common.MarketData) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.updatePriceHistory(data.Price)
	s.updateVolatilitySurface(data.Price.Sub(s.getLastPrice()).Abs())

	s.metrics.LastUpdated = time.Now()
	return nil
}

// OnOrderFill handles order fill events
func (s *VolatilitySurfaceStrategy) OnOrderFill(ctx context.Context, fill *common.OrderFill) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.metrics.OrdersFilled++

	// Calculate spread capture
	if len(s.priceHistory) > 0 {
		lastPrice := s.priceHistory[len(s.priceHistory)-1]
		spread := fill.Price.Sub(lastPrice).Abs().Div(lastPrice)

		if s.metrics.OrdersPlaced == 1 {
			s.metrics.SpreadCapture = spread
		} else {
			weight := decimal.NewFromFloat(1.0).Div(decimal.NewFromInt(s.metrics.OrdersPlaced))
			s.metrics.SpreadCapture = s.metrics.SpreadCapture.Mul(decimal.NewFromFloat(1.0).Sub(weight)).Add(spread.Mul(weight))
		}
	}

	return nil
}

// OnOrderCancel handles order cancellation events
func (s *VolatilitySurfaceStrategy) OnOrderCancel(ctx context.Context, orderID uuid.UUID, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.metrics.OrdersCancelled++
	return nil
}

// UpdateConfig updates strategy configuration
func (s *VolatilitySurfaceStrategy) UpdateConfig(ctx context.Context, config common.StrategyConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.config = config
	return nil
}

// GetConfig returns current configuration
func (s *VolatilitySurfaceStrategy) GetConfig() common.StrategyConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config
}

// GetMetrics returns current metrics
func (s *VolatilitySurfaceStrategy) GetMetrics() *common.StrategyMetrics {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.metrics
}

// GetStatus returns current status
func (s *VolatilitySurfaceStrategy) GetStatus() common.StrategyStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.status
}

// Name returns strategy name
func (s *VolatilitySurfaceStrategy) Name() string {
	return s.name
}

// Version returns strategy version
func (s *VolatilitySurfaceStrategy) Version() string {
	return s.version
}

// Description returns strategy description
func (s *VolatilitySurfaceStrategy) Description() string {
	return "Advanced market making strategy using volatility surface modeling across multiple time horizons"
}

// RiskLevel returns the risk level
func (s *VolatilitySurfaceStrategy) RiskLevel() common.RiskLevel {
	return common.RiskHigh
}

// HealthCheck performs health check
func (s *VolatilitySurfaceStrategy) HealthCheck(ctx context.Context) *common.HealthStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	isHealthy := s.status == common.StatusRunning && len(s.priceHistory) > 10

	checks := map[string]bool{
		"status":      s.status == common.StatusRunning,
		"price_data":  len(s.priceHistory) > 10,
		"vol_surface": len(s.volSurface) > 0,
	}

	message := "Strategy is healthy"
	if !isHealthy {
		message = "Strategy has issues"
	}

	return &common.HealthStatus{
		IsHealthy:     isHealthy,
		Status:        s.status,
		Message:       message,
		Checks:        checks,
		LastCheckTime: time.Now(),
	}
}

// Reset resets strategy state
func (s *VolatilitySurfaceStrategy) Reset(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.priceHistory = s.priceHistory[:0]
	s.volSurface = make(map[time.Duration]decimal.Decimal)
	s.lastUpdate = time.Time{}

	// Reset metrics
	s.metrics = &common.StrategyMetrics{
		TotalPnL:          decimal.Zero,
		DailyPnL:          decimal.Zero,
		SharpeRatio:       decimal.Zero,
		MaxDrawdown:       decimal.Zero,
		WinRate:           decimal.Zero,
		OrdersPlaced:      0,
		OrdersFilled:      0,
		OrdersCancelled:   0,
		SuccessRate:       decimal.Zero,
		SpreadCapture:     decimal.Zero,
		InventoryTurnover: decimal.Zero,
		QuoteUptime:       decimal.Zero,
		LastUpdated:       time.Now(),
	}

	return nil
}

// Helper methods

func (s *VolatilitySurfaceStrategy) updatePriceHistory(price decimal.Decimal) {
	s.priceHistory = append(s.priceHistory, price)
	if len(s.priceHistory) > s.maxHistory {
		s.priceHistory = s.priceHistory[1:]
	}
}

func (s *VolatilitySurfaceStrategy) getLastPrice() decimal.Decimal {
	if len(s.priceHistory) == 0 {
		return decimal.Zero
	}
	return s.priceHistory[len(s.priceHistory)-1]
}

func (s *VolatilitySurfaceStrategy) calculateImpliedVolatility() decimal.Decimal {
	if len(s.priceHistory) < 2 {
		return decimal.NewFromFloat(0.01)
	}

	// Calculate returns
	var returns []decimal.Decimal
	for i := 1; i < len(s.priceHistory); i++ {
		if s.priceHistory[i-1].IsZero() {
			continue
		}
		ret := s.priceHistory[i].Sub(s.priceHistory[i-1]).Div(s.priceHistory[i-1])
		returns = append(returns, ret)
	}

	if len(returns) == 0 {
		return decimal.NewFromFloat(0.01)
	}

	// Calculate mean
	sum := decimal.Zero
	for _, ret := range returns {
		sum = sum.Add(ret)
	}
	mean := sum.Div(decimal.NewFromInt(int64(len(returns))))

	// Calculate variance
	variance := decimal.Zero
	for _, ret := range returns {
		diff := ret.Sub(mean)
		variance = variance.Add(diff.Mul(diff))
	}
	variance = variance.Div(decimal.NewFromInt(int64(len(returns))))

	// Convert to annualized volatility (simple approximation)
	vol := variance.Mul(decimal.NewFromFloat(252)) // 252 trading days
	return vol
}

func (s *VolatilitySurfaceStrategy) updateVolatilitySurface(currentVol decimal.Decimal) {
	now := time.Now()

	// Update different tenors based on time elapsed
	for tenor := range s.volSurface {
		decay := decimal.NewFromFloat(0.95) // Decay factor
		s.volSurface[tenor] = s.volSurface[tenor].Mul(decay).Add(currentVol.Mul(decimal.NewFromFloat(0.05)))
	}

	s.lastUpdate = now
}

func (s *VolatilitySurfaceStrategy) calculateWeightedVolatility() decimal.Decimal {
	if len(s.volSurface) == 0 {
		return decimal.NewFromFloat(0.01)
	}

	totalWeight := decimal.Zero
	weightedSum := decimal.Zero

	// Weight shorter tenors more heavily
	for tenor, vol := range s.volSurface {
		weight := decimal.NewFromFloat(1.0).Div(decimal.NewFromFloat(float64(tenor.Minutes())))
		weightedSum = weightedSum.Add(vol.Mul(weight))
		totalWeight = totalWeight.Add(weight)
	}

	if totalWeight.IsZero() {
		return decimal.NewFromFloat(0.01)
	}

	return weightedSum.Div(totalWeight)
}

func (s *VolatilitySurfaceStrategy) calculateConfidence() float64 {
	// Confidence based on data quality and model fit
	baseConfidence := 0.7

	// Adjust based on price history length
	if len(s.priceHistory) > 50 {
		baseConfidence += 0.2
	} else if len(s.priceHistory) > 20 {
		baseConfidence += 0.1
	}

	// Adjust based on volatility surface completeness
	if len(s.volSurface) >= 5 {
		baseConfidence += 0.1
	}

	if baseConfidence > 1.0 {
		baseConfidence = 1.0
	}

	return baseConfidence
}
