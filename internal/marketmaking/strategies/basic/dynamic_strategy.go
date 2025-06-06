package basic

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/Finalex/internal/marketmaking/strategies/common"
)

// DynamicStrategy implements volatility-adjusted market making
type DynamicStrategy struct {
	config         *DynamicConfig
	metrics        *common.StrategyMetrics
	status         common.StrategyStatus
	volatilityHist []float64
	mu             sync.RWMutex
	stopChan       chan struct{}
}

type DynamicConfig struct {
	BaseSpread     float64 `json:"base_spread"`
	VolFactor      float64 `json:"vol_factor"`
	Size           float64 `json:"size"`
	LookbackPeriod int     `json:"lookback_period"`
	TickSize       float64 `json:"tick_size"`
}

// NewDynamicStrategy creates a new dynamic market making strategy
func NewDynamicStrategy() *DynamicStrategy {
	return &DynamicStrategy{
		config: &DynamicConfig{
			BaseSpread:     0.001,
			VolFactor:      0.5,
			Size:           100.0,
			LookbackPeriod: 20,
			TickSize:       0.01,
		},
		metrics: &common.StrategyMetrics{
			StartTime: time.Now(),
		},
		status:         common.StatusStopped,
		volatilityHist: make([]float64, 0, 100),
		stopChan:       make(chan struct{}),
	}
}

// Initialize prepares the strategy for trading
func (s *DynamicStrategy) Initialize(ctx context.Context, config common.StrategyConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Parse dynamic strategy config
	if baseSpread, ok := config.Parameters["base_spread"].(float64); ok {
		s.config.BaseSpread = baseSpread
	}
	if volFactor, ok := config.Parameters["vol_factor"].(float64); ok {
		s.config.VolFactor = volFactor
	}
	if size, ok := config.Parameters["size"].(float64); ok {
		s.config.Size = size
	}
	if lookback, ok := config.Parameters["lookback_period"].(float64); ok {
		s.config.LookbackPeriod = int(lookback)
	}
	if tickSize, ok := config.Parameters["tick_size"].(float64); ok {
		s.config.TickSize = tickSize
	}

	s.status = common.StatusInitialized
	return nil
}

// Start begins strategy execution
func (s *DynamicStrategy) Start(ctx context.Context) error {
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
func (s *DynamicStrategy) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.status != common.StatusRunning {
		return fmt.Errorf("cannot stop strategy in status: %s", s.status)
	}

	close(s.stopChan)
	s.status = common.StatusStopped
	return nil
}

// Quote generates bid/ask quotes adjusted for volatility
func (s *DynamicStrategy) Quote(input common.QuoteInput) (common.QuoteOutput, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.status != common.StatusRunning {
		return common.QuoteOutput{}, fmt.Errorf("strategy not running")
	}

	// Calculate volatility-adjusted spread
	volatility := s.calculateVolatility(input)
	adjustedSpread := s.config.BaseSpread + s.config.VolFactor*volatility

	halfSpread := adjustedSpread / 2
	bid := input.MidPrice * (1 - halfSpread)
	ask := input.MidPrice * (1 + halfSpread)

	// Round to tick size
	if s.config.TickSize > 0 {
		bid = s.roundToTick(bid, false)
		ask = s.roundToTick(ask, true)
	}

	// Adjust size based on volatility (lower size in high volatility)
	volAdjustment := 1.0 / (1.0 + volatility*5.0)
	adjustedSize := s.config.Size * volAdjustment

	output := common.QuoteOutput{
		BidPrice:   bid,
		AskPrice:   ask,
		BidSize:    adjustedSize,
		AskSize:    adjustedSize,
		Confidence: s.calculateConfidence(volatility),
		Timestamp:  time.Now(),
	}

	// Update metrics
	s.updateMetrics(output, volatility)

	return output, nil
}

// OnMarketData handles market data updates
func (s *DynamicStrategy) OnMarketData(data common.MarketData) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Update volatility history from market data
	if len(data.RecentTrades) > 1 {
		// Calculate short-term volatility from recent trades
		vol := s.calculateTradeVolatility(data.RecentTrades)
		s.updateVolatilityHistory(vol)
	}

	return nil
}

// OnOrderFill handles order fill notifications
func (s *DynamicStrategy) OnOrderFill(fill common.OrderFill) error {
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
func (s *DynamicStrategy) OnOrderCancel(cancel common.OrderCancel) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.metrics.OrdersCancelled++
	return nil
}

// UpdateConfig updates strategy configuration
func (s *DynamicStrategy) UpdateConfig(config common.StrategyConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if baseSpread, ok := config.Parameters["base_spread"].(float64); ok {
		s.config.BaseSpread = baseSpread
	}
	if volFactor, ok := config.Parameters["vol_factor"].(float64); ok {
		s.config.VolFactor = volFactor
	}
	if size, ok := config.Parameters["size"].(float64); ok {
		s.config.Size = size
	}
	if lookback, ok := config.Parameters["lookback_period"].(float64); ok {
		s.config.LookbackPeriod = int(lookback)
	}

	return nil
}

// GetMetrics returns current strategy metrics
func (s *DynamicStrategy) GetMetrics() common.StrategyMetrics {
	s.mu.RLock()
	defer s.mu.RUnlock()

	metrics := *s.metrics
	metrics.Uptime = time.Since(s.metrics.StartTime)

	if s.metrics.OrdersPlaced > 0 {
		metrics.SuccessRate = float64(s.metrics.OrdersPlaced-s.metrics.OrdersCancelled) / float64(s.metrics.OrdersPlaced)
	}

	// Add volatility-specific metrics
	if len(s.volatilityHist) > 0 {
		metrics.CurrentVolatility = s.volatilityHist[len(s.volatilityHist)-1]
	}

	return metrics
}

// GetStatus returns current strategy status
func (s *DynamicStrategy) GetStatus() common.StrategyStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.status
}

// GetInfo returns strategy information
func (s *DynamicStrategy) GetInfo() common.StrategyInfo {
	return common.StrategyInfo{
		Name:        "Dynamic",
		Version:     "1.0.0",
		Description: "Volatility-adjusted market making strategy",
		Author:      "Finalex Team",
		RiskLevel:   common.RiskMedium,
		Complexity:  common.ComplexityIntermediate,
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
				Name:        "vol_factor",
				Type:        "float",
				Description: "Volatility adjustment factor",
				Default:     0.5,
				MinValue:    0.0,
				MaxValue:    5.0,
				Required:    true,
			},
			{
				Name:        "size",
				Type:        "float",
				Description: "Base order size for quotes",
				Default:     100.0,
				MinValue:    1.0,
				MaxValue:    10000.0,
				Required:    true,
			},
			{
				Name:        "lookback_period",
				Type:        "int",
				Description: "Lookback period for volatility calculation",
				Default:     20,
				MinValue:    5,
				MaxValue:    100,
				Required:    false,
			},
		},
	}
}

// HealthCheck performs health validation
func (s *DynamicStrategy) HealthCheck() common.HealthStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	health := common.HealthStatus{
		IsHealthy: true,
		Timestamp: time.Now(),
	}

	// Check configuration
	if s.config.BaseSpread <= 0 {
		health.IsHealthy = false
		health.Issues = append(health.Issues, "Invalid base spread configuration")
	}

	if s.config.VolFactor < 0 {
		health.IsHealthy = false
		health.Issues = append(health.Issues, "Invalid volatility factor")
	}

	// Check volatility data availability
	if s.status == common.StatusRunning && len(s.volatilityHist) == 0 {
		health.Issues = append(health.Issues, "No volatility data available")
	}

	return health
}

// Helper methods

func (s *DynamicStrategy) calculateVolatility(input common.QuoteInput) float64 {
	// Use provided volatility if available
	if input.Volatility > 0 {
		s.updateVolatilityHistory(input.Volatility)
		return input.Volatility
	}

	// Calculate from historical data
	if len(s.volatilityHist) == 0 {
		return 0.01 // Default volatility
	}

	// Return most recent volatility
	return s.volatilityHist[len(s.volatilityHist)-1]
}

func (s *DynamicStrategy) calculateTradeVolatility(trades []common.Trade) float64 {
	if len(trades) < 2 {
		return 0.01
	}

	// Calculate price returns
	returns := make([]float64, 0, len(trades)-1)
	for i := 1; i < len(trades); i++ {
		if trades[i-1].Price > 0 {
			ret := (trades[i].Price - trades[i-1].Price) / trades[i-1].Price
			returns = append(returns, ret)
		}
	}

	if len(returns) == 0 {
		return 0.01
	}

	// Calculate standard deviation of returns
	var sum, sumSq float64
	for _, ret := range returns {
		sum += ret
		sumSq += ret * ret
	}

	mean := sum / float64(len(returns))
	variance := (sumSq / float64(len(returns))) - (mean * mean)

	if variance < 0 {
		variance = 0
	}

	return math.Sqrt(variance)
}

func (s *DynamicStrategy) updateVolatilityHistory(vol float64) {
	s.volatilityHist = append(s.volatilityHist, vol)

	// Keep only recent history
	maxHistory := s.config.LookbackPeriod * 2
	if len(s.volatilityHist) > maxHistory {
		s.volatilityHist = s.volatilityHist[len(s.volatilityHist)-maxHistory:]
	}
}

func (s *DynamicStrategy) calculateConfidence(volatility float64) float64 {
	// Lower confidence in high volatility environments
	confidence := 1.0 / (1.0 + volatility*10.0)
	return math.Max(0.1, math.Min(1.0, confidence))
}

func (s *DynamicStrategy) roundToTick(price float64, roundUp bool) float64 {
	if s.config.TickSize <= 0 {
		return price
	}

	if roundUp {
		return math.Ceil(price/s.config.TickSize) * s.config.TickSize
	} else {
		return math.Floor(price/s.config.TickSize) * s.config.TickSize
	}
}

func (s *DynamicStrategy) updateMetrics(output common.QuoteOutput, volatility float64) {
	s.metrics.LastUpdate = time.Now()
	s.metrics.QuotesGenerated++
	s.metrics.CurrentVolatility = volatility

	if output.BidPrice > 0 && output.AskPrice > 0 {
		spread := (output.AskPrice - output.BidPrice) / output.BidPrice
		s.metrics.AvgSpread = (s.metrics.AvgSpread*float64(s.metrics.QuotesGenerated-1) + spread) / float64(s.metrics.QuotesGenerated)
	}
}
