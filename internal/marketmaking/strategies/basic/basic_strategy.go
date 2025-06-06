package basic

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/Finalex/internal/marketmaking/strategies/common"
)

// BasicStrategy implements simple spread-based market making
type BasicStrategy struct {
	config   *BasicConfig
	metrics  *common.StrategyMetrics
	status   common.StrategyStatus
	mu       sync.RWMutex
	stopChan chan struct{}
}

type BasicConfig struct {
	Spread   float64 `json:"spread"`
	Size     float64 `json:"size"`
	TickSize float64 `json:"tick_size"`
}

// NewBasicStrategy creates a new basic market making strategy
func NewBasicStrategy() *BasicStrategy {
	return &BasicStrategy{
		config: &BasicConfig{
			Spread:   0.001,
			Size:     100.0,
			TickSize: 0.01,
		},
		metrics: &common.StrategyMetrics{
			StartTime: time.Now(),
		},
		status:   common.StatusStopped,
		stopChan: make(chan struct{}),
	}
}

// Initialize prepares the strategy for trading
func (s *BasicStrategy) Initialize(ctx context.Context, config common.StrategyConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Parse basic strategy config
	if spread, ok := config.Parameters["spread"].(float64); ok {
		s.config.Spread = spread
	}
	if size, ok := config.Parameters["size"].(float64); ok {
		s.config.Size = size
	}
	if tickSize, ok := config.Parameters["tick_size"].(float64); ok {
		s.config.TickSize = tickSize
	}

	s.status = common.StatusInitialized
	return nil
}

// Start begins strategy execution
func (s *BasicStrategy) Start(ctx context.Context) error {
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
func (s *BasicStrategy) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.status != common.StatusRunning {
		return fmt.Errorf("cannot stop strategy in status: %s", s.status)
	}

	close(s.stopChan)
	s.status = common.StatusStopped
	return nil
}

// Quote generates bid/ask quotes for the given market conditions
func (s *BasicStrategy) Quote(input common.QuoteInput) (common.QuoteOutput, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.status != common.StatusRunning {
		return common.QuoteOutput{}, fmt.Errorf("strategy not running")
	}

	// Simple spread-based quoting
	halfSpread := s.config.Spread / 2
	bid := input.MidPrice * (1 - halfSpread)
	ask := input.MidPrice * (1 + halfSpread)

	// Round to tick size
	if s.config.TickSize > 0 {
		bid = s.roundToTick(bid, false)
		ask = s.roundToTick(ask, true)
	}

	output := common.QuoteOutput{
		BidPrice:   bid,
		AskPrice:   ask,
		BidSize:    s.config.Size,
		AskSize:    s.config.Size,
		Confidence: 0.8, // Basic strategy has moderate confidence
		Timestamp:  time.Now(),
	}

	// Update metrics
	s.updateMetrics(output)

	return output, nil
}

// OnMarketData handles market data updates
func (s *BasicStrategy) OnMarketData(data common.MarketData) error {
	// Basic strategy doesn't need to process market data
	return nil
}

// OnOrderFill handles order fill notifications
func (s *BasicStrategy) OnOrderFill(fill common.OrderFill) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Update basic metrics
	s.metrics.TotalVolume += fill.Quantity
	s.metrics.OrdersPlaced++

	if fill.Side == "buy" {
		s.metrics.BuyVolume += fill.Quantity
	} else {
		s.metrics.SellVolume += fill.Quantity
	}

	// Simple PnL calculation
	if fill.Side == "buy" {
		s.metrics.TotalPnL -= fill.Price * fill.Quantity
	} else {
		s.metrics.TotalPnL += fill.Price * fill.Quantity
	}

	return nil
}

// OnOrderCancel handles order cancellation notifications
func (s *BasicStrategy) OnOrderCancel(cancel common.OrderCancel) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.metrics.OrdersCancelled++
	return nil
}

// UpdateConfig updates strategy configuration
func (s *BasicStrategy) UpdateConfig(config common.StrategyConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if spread, ok := config.Parameters["spread"].(float64); ok {
		s.config.Spread = spread
	}
	if size, ok := config.Parameters["size"].(float64); ok {
		s.config.Size = size
	}
	if tickSize, ok := config.Parameters["tick_size"].(float64); ok {
		s.config.TickSize = tickSize
	}

	return nil
}

// GetMetrics returns current strategy metrics
func (s *BasicStrategy) GetMetrics() common.StrategyMetrics {
	s.mu.RLock()
	defer s.mu.RUnlock()

	metrics := *s.metrics
	metrics.Uptime = time.Since(s.metrics.StartTime)

	// Calculate basic performance metrics
	if s.metrics.OrdersPlaced > 0 {
		metrics.SuccessRate = float64(s.metrics.OrdersPlaced-s.metrics.OrdersCancelled) / float64(s.metrics.OrdersPlaced)
	}

	return metrics
}

// GetStatus returns current strategy status
func (s *BasicStrategy) GetStatus() common.StrategyStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.status
}

// GetInfo returns strategy information
func (s *BasicStrategy) GetInfo() common.StrategyInfo {
	return common.StrategyInfo{
		Name:        "Basic",
		Version:     "1.0.0",
		Description: "Simple spread-based market making strategy",
		Author:      "Finalex Team",
		RiskLevel:   common.RiskLow,
		Complexity:  common.ComplexityBasic,
		Parameters: []common.ParameterDefinition{
			{
				Name:        "spread",
				Type:        "float",
				Description: "Base spread as a fraction of mid price",
				Default:     0.001,
				MinValue:    0.0001,
				MaxValue:    0.1,
				Required:    true,
			},
			{
				Name:        "size",
				Type:        "float",
				Description: "Order size for quotes",
				Default:     100.0,
				MinValue:    1.0,
				MaxValue:    10000.0,
				Required:    true,
			},
			{
				Name:        "tick_size",
				Type:        "float",
				Description: "Minimum price increment",
				Default:     0.01,
				MinValue:    0.001,
				MaxValue:    1.0,
				Required:    false,
			},
		},
	}
}

// HealthCheck performs basic health validation
func (s *BasicStrategy) HealthCheck() common.HealthStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	health := common.HealthStatus{
		IsHealthy: true,
		Timestamp: time.Now(),
	}

	// Check basic health conditions
	if s.config.Spread <= 0 {
		health.IsHealthy = false
		health.Issues = append(health.Issues, "Invalid spread configuration")
	}

	if s.config.Size <= 0 {
		health.IsHealthy = false
		health.Issues = append(health.Issues, "Invalid size configuration")
	}

	// Check for stale metrics (no activity for 5 minutes)
	if time.Since(s.metrics.LastUpdate) > 5*time.Minute && s.status == common.StatusRunning {
		health.Issues = append(health.Issues, "No recent activity detected")
	}

	return health
}

// Helper methods

func (s *BasicStrategy) roundToTick(price float64, roundUp bool) float64 {
	if s.config.TickSize <= 0 {
		return price
	}

	if roundUp {
		return math.Ceil(price/s.config.TickSize) * s.config.TickSize
	} else {
		return math.Floor(price/s.config.TickSize) * s.config.TickSize
	}
}

func (s *BasicStrategy) updateMetrics(output common.QuoteOutput) {
	s.metrics.LastUpdate = time.Now()
	s.metrics.QuotesGenerated++

	if output.BidPrice > 0 && output.AskPrice > 0 {
		spread := (output.AskPrice - output.BidPrice) / output.BidPrice
		s.metrics.AvgSpread = (s.metrics.AvgSpread*float64(s.metrics.QuotesGenerated-1) + spread) / float64(s.metrics.QuotesGenerated)
	}
}
