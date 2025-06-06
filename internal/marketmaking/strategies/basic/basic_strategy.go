package basic

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Aidin1998/finalex/internal/marketmaking/strategies/common"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// BasicStrategy implements a simple fixed-spread market making strategy
type BasicStrategy struct {
	config   common.StrategyConfig
	name     string
	version  string
	status   common.StrategyStatus
	metrics  *common.StrategyMetrics
	mu       sync.RWMutex
	spread   decimal.Decimal
	size     decimal.Decimal
	tickSize decimal.Decimal
}

// NewBasicStrategy creates a new basic strategy instance
func NewBasicStrategy(config common.StrategyConfig) (common.MarketMakingStrategy, error) {
	// Default values
	spread := decimal.NewFromFloat(0.002) // 20 bps
	size := decimal.NewFromFloat(100.0)
	tickSize := decimal.NewFromFloat(0.01)

	// Parse config parameters
	if val, ok := config.Parameters["spread"]; ok {
		if f, ok := val.(float64); ok {
			spread = decimal.NewFromFloat(f)
		}
	}
	if val, ok := config.Parameters["size"]; ok {
		if f, ok := val.(float64); ok {
			size = decimal.NewFromFloat(f)
		}
	}
	if val, ok := config.Parameters["tick_size"]; ok {
		if f, ok := val.(float64); ok {
			tickSize = decimal.NewFromFloat(f)
		}
	}

	strategy := &BasicStrategy{
		config:   config,
		name:     "Basic",
		version:  "1.0.0",
		status:   common.StatusStopped,
		spread:   spread,
		size:     size,
		tickSize: tickSize,
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

	return strategy, nil
}

// Core interface methods

func (s *BasicStrategy) Quote(ctx context.Context, input common.QuoteInput) (*common.QuoteOutput, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.status != common.StatusRunning {
		return nil, fmt.Errorf("strategy not running")
	}

	// Calculate spread-based quotes
	halfSpread := s.spread.Div(decimal.NewFromInt(2))

	bid := input.MidPrice.Mul(decimal.NewFromInt(1).Sub(halfSpread))
	ask := input.MidPrice.Mul(decimal.NewFromInt(1).Add(halfSpread))

	// Round to tick size if specified
	if s.tickSize.GreaterThan(decimal.Zero) {
		bid = s.roundToTick(bid, false)
		ask = s.roundToTick(ask, true)
	}

	output := &common.QuoteOutput{
		BidPrice:    bid,
		AskPrice:    ask,
		BidSize:     s.size,
		AskSize:     s.size,
		Confidence:  decimal.NewFromFloat(0.8), // Basic strategy has moderate confidence
		TTL:         time.Second * 30,          // 30 second TTL
		GeneratedAt: time.Now(),
		Metadata: map[string]interface{}{
			"strategy_type": "basic",
			"spread":        s.spread.String(),
		},
	}

	// Update metrics
	s.updateQuoteMetrics(output)

	return output, nil
}

func (s *BasicStrategy) Initialize(ctx context.Context, config common.StrategyConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.config = config
	if err := s.parseConfig(); err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	s.status = common.StatusStarting
	s.metrics.LastUpdated = time.Now()
	return nil
}

func (s *BasicStrategy) Start(ctx context.Context) error {
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

func (s *BasicStrategy) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.status != common.StatusRunning {
		return fmt.Errorf("cannot stop strategy in status: %s", s.status)
	}

	s.status = common.StatusStopped
	s.metrics.LastUpdated = time.Now()
	return nil
}

func (s *BasicStrategy) OnMarketData(ctx context.Context, data *common.MarketData) error {
	// Basic strategy doesn't need complex market data processing
	return nil
}

func (s *BasicStrategy) OnOrderFill(ctx context.Context, fill *common.OrderFill) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Update metrics
	s.metrics.OrdersFilled++

	// Simple PnL calculation
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

func (s *BasicStrategy) OnOrderCancel(ctx context.Context, orderID uuid.UUID, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.metrics.OrdersCancelled++
	s.metrics.LastUpdated = time.Now()
	return nil
}

func (s *BasicStrategy) UpdateConfig(ctx context.Context, config common.StrategyConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.config = config
	if err := s.parseConfig(); err != nil {
		return fmt.Errorf("failed to parse updated config: %w", err)
	}

	s.metrics.LastUpdated = time.Now()
	return nil
}

func (s *BasicStrategy) GetConfig() common.StrategyConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.config
}

func (s *BasicStrategy) GetMetrics() *common.StrategyMetrics {
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

func (s *BasicStrategy) GetStatus() common.StrategyStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.status
}

// Metadata methods
func (s *BasicStrategy) Name() string {
	return "Basic Market Making"
}

func (s *BasicStrategy) Version() string {
	return "1.0.0"
}

func (s *BasicStrategy) Description() string {
	return "Simple spread-based market making strategy with fixed spreads"
}

func (s *BasicStrategy) RiskLevel() common.RiskLevel {
	return common.RiskLow
}

func (s *BasicStrategy) HealthCheck(ctx context.Context) *common.HealthStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	health := &common.HealthStatus{
		IsHealthy:     true,
		Status:        s.status,
		Checks:        make(map[string]bool),
		LastCheckTime: time.Now(),
	}

	// Check configuration validity
	health.Checks["valid_spread"] = s.spread.GreaterThan(decimal.Zero)
	health.Checks["valid_size"] = s.size.GreaterThan(decimal.Zero)
	health.Checks["strategy_running"] = s.status == common.StatusRunning

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

func (s *BasicStrategy) Reset(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Reset metrics but keep configuration
	s.metrics = &common.StrategyMetrics{
		LastUpdated: time.Now(),
	}
	s.startTime = time.Now()

	return nil
}

// Helper methods

func (s *BasicStrategy) parseConfig() error {
	params := s.config.Parameters

	if spread, ok := params["spread"]; ok {
		if spreadFloat, ok := spread.(float64); ok {
			s.spread = decimal.NewFromFloat(spreadFloat)
		} else if spreadStr, ok := spread.(string); ok {
			var err error
			s.spread, err = decimal.NewFromString(spreadStr)
			if err != nil {
				return fmt.Errorf("invalid spread value: %w", err)
			}
		}
	}

	if size, ok := params["size"]; ok {
		if sizeFloat, ok := size.(float64); ok {
			s.size = decimal.NewFromFloat(sizeFloat)
		} else if sizeStr, ok := size.(string); ok {
			var err error
			s.size, err = decimal.NewFromString(sizeStr)
			if err != nil {
				return fmt.Errorf("invalid size value: %w", err)
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

	return nil
}

func (s *BasicStrategy) roundToTick(price decimal.Decimal, roundUp bool) decimal.Decimal {
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

func (s *BasicStrategy) updateQuoteMetrics(output *common.QuoteOutput) {
	s.metrics.OrdersPlaced++
	s.metrics.LastUpdated = time.Now()

	// Calculate spread capture
	if output.BidPrice.GreaterThan(decimal.Zero) && output.AskPrice.GreaterThan(decimal.Zero) {
		spread := output.AskPrice.Sub(output.BidPrice).Div(output.BidPrice)
		s.metrics.SpreadCapture = spread
	}
}
