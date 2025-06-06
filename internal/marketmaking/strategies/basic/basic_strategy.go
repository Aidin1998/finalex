// Package basic provides simple market making strategies
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

// BasicStrategy implements the simplest form of market making with fixed spread
type BasicStrategy struct {
	config     common.StrategyConfig
	name       string
	version    string
	status     common.StrategyStatus
	metrics    *common.StrategyMetrics
	mu         sync.RWMutex
	spread     decimal.Decimal
	size       decimal.Decimal
	tickSize   decimal.Decimal
	startTime  time.Time
	lastQuote  time.Time
	tradesLock sync.Mutex
	trades     []common.OrderFill
}

// NewBasicStrategy creates a new basic market making strategy instance
func NewBasicStrategy(config common.StrategyConfig) (*BasicStrategy, error) {
	strategy := &BasicStrategy{
		config:  config,
		name:    "Basic Strategy",
		version: "1.0.0",
		status:  common.StatusUninitialized,
		metrics: &common.StrategyMetrics{},
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

	// Calculate quotes with fixed spread
	halfSpread := s.spread.Div(decimal.NewFromInt(2))
	bid := input.MidPrice.Mul(decimal.NewFromInt(1).Sub(halfSpread))
	ask := input.MidPrice.Mul(decimal.NewFromInt(1).Add(halfSpread))

	// Round to tick size if specified
	if s.tickSize.GreaterThan(decimal.Zero) {
		bid = s.roundToTick(bid, false) // round down for bid
		ask = s.roundToTick(ask, true)  // round up for ask
	}

	// Fixed size from config
	size := s.size

	output := &common.QuoteOutput{
		BidPrice:    bid,
		AskPrice:    ask,
		BidSize:     size,
		AskSize:     size,
		Confidence:  decimal.NewFromFloat(0.9), // High confidence for simple strategy
		TTL:         10 * time.Second,
		GeneratedAt: time.Now(),
		Metadata: map[string]interface{}{
			"strategy_type": "basic",
			"spread":        s.spread.String(),
		},
	}

	s.lastQuote = time.Now()
	return output, nil
}

func (s *BasicStrategy) Initialize(ctx context.Context, config common.StrategyConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.config = config

	// Extract required parameters
	spreadParam, ok := config.Parameters["spread"]
	if !ok {
		return fmt.Errorf("missing required parameter 'spread'")
	}

	spreadFloat, ok := spreadParam.(float64)
	if !ok {
		return fmt.Errorf("invalid spread parameter type")
	}

	sizeParam, ok := config.Parameters["size"]
	if !ok {
		return fmt.Errorf("missing required parameter 'size'")
	}

	sizeFloat, ok := sizeParam.(float64)
	if !ok {
		return fmt.Errorf("invalid size parameter type")
	}

	// Optional tick size parameter
	tickSizeFloat := 0.0
	if tickSize, ok := config.Parameters["tick_size"]; ok {
		tickSizeFloat, _ = tickSize.(float64)
	}

	// Set strategy parameters
	s.spread = decimal.NewFromFloat(spreadFloat)
	s.size = decimal.NewFromFloat(sizeFloat)
	s.tickSize = decimal.NewFromFloat(tickSizeFloat)
	s.trades = make([]common.OrderFill, 0, 100)
	s.status = common.StatusInitialized

	return nil
}

func (s *BasicStrategy) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.status != common.StatusInitialized && s.status != common.StatusStopped {
		return fmt.Errorf("strategy in invalid state: %s", s.status)
	}

	s.status = common.StatusStarting
	// Any startup logic here...

	s.startTime = time.Now()
	s.status = common.StatusRunning

	return nil
}

func (s *BasicStrategy) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.status != common.StatusRunning && s.status != common.StatusPaused {
		return fmt.Errorf("strategy not running or paused")
	}

	s.status = common.StatusStopping
	// Any shutdown logic here...

	s.status = common.StatusStopped
	return nil
}

// Market event handlers

func (s *BasicStrategy) OnMarketData(ctx context.Context, data *common.MarketData) error {
	// Simple strategy doesn't need to process ongoing market data
	return nil
}

func (s *BasicStrategy) OnOrderFill(ctx context.Context, fill *common.OrderFill) error {
	s.tradesLock.Lock()
	defer s.tradesLock.Unlock()

	// Add to trade history
	s.trades = append(s.trades, *fill)
	if len(s.trades) > 100 {
		// Keep only last 100 trades
		s.trades = s.trades[1:]
	}

	// Update metrics
	s.metrics.OrdersFilled++
	s.metrics.LastTrade = time.Now()

	return nil
}

func (s *BasicStrategy) OnOrderCancel(ctx context.Context, orderID uuid.UUID, reason string) error {
	// Update metrics
	s.metrics.OrdersCancelled++
	return nil
}

// Configuration methods

func (s *BasicStrategy) UpdateConfig(ctx context.Context, config common.StrategyConfig) error {
	return s.Initialize(ctx, config)
}

func (s *BasicStrategy) GetConfig() common.StrategyConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config
}

func (s *BasicStrategy) GetMetrics() *common.StrategyMetrics {
	// Update uptime and other dynamic metrics
	s.metrics.StrategyUptime = time.Since(s.startTime)
	s.metrics.LastUpdated = time.Now()

	return s.metrics
}

func (s *BasicStrategy) GetStatus() common.StrategyStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.status
}

// Metadata methods

func (s *BasicStrategy) Name() string {
	return s.name
}

func (s *BasicStrategy) Version() string {
	return s.version
}

func (s *BasicStrategy) Description() string {
	return "Simple fixed spread market making strategy"
}

func (s *BasicStrategy) RiskLevel() common.RiskLevel {
	return common.RiskLow
}

// Health and monitoring

func (s *BasicStrategy) HealthCheck(ctx context.Context) *common.HealthStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	isHealthy := s.status == common.StatusRunning

	return &common.HealthStatus{
		IsHealthy:     isHealthy,
		Status:        s.status,
		Message:       fmt.Sprintf("Basic strategy status: %s", s.status),
		Checks:        map[string]bool{"running": isHealthy},
		LastCheckTime: time.Now(),
	}
}

func (s *BasicStrategy) Reset(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.status == common.StatusRunning || s.status == common.StatusStarting {
		return fmt.Errorf("cannot reset running strategy")
	}

	s.metrics = &common.StrategyMetrics{}
	s.trades = make([]common.OrderFill, 0, 100)
	s.status = common.StatusInitialized

	return nil
}

// Helper methods

func (s *BasicStrategy) roundToTick(price decimal.Decimal, roundUp bool) decimal.Decimal {
	if s.tickSize.IsZero() {
		return price
	}

	remainder := price.Mod(s.tickSize)
	if remainder.IsZero() {
		return price
	}

	if roundUp {
		return price.Sub(remainder).Add(s.tickSize)
	} else {
		return price.Sub(remainder)
	}
}
