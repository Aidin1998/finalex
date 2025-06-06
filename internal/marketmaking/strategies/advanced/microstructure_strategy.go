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

// MicroStructureStrategy focuses on order book microstructure and market impact
type MicroStructureStrategy struct {
	config      common.StrategyConfig
	name        string
	version     string
	status      common.StrategyStatus
	metrics     *common.StrategyMetrics
	mu          sync.RWMutex
	baseSpread  decimal.Decimal
	maxPosition decimal.Decimal
	tickSize    decimal.Decimal

	// Market microstructure state
	bookPressure     BookPressure
	marketImpact     decimal.Decimal
	adverseSelection decimal.Decimal

	// Order book analysis
	recentTrades []TradeEvent
	maxTrades    int
	stopChan     chan struct{}
}

// BookPressure represents order book pressure metrics
type BookPressure struct {
	BidPressure decimal.Decimal
	AskPressure decimal.Decimal
	Imbalance   decimal.Decimal
	UpdateTime  time.Time
}

// TradeEvent represents a trade for microstructure analysis
type TradeEvent struct {
	Price     decimal.Decimal
	Volume    decimal.Decimal
	Side      string // "buy" or "sell"
	Timestamp time.Time
	Impact    decimal.Decimal
}

// NewMicroStructureStrategy creates a new microstructure strategy
func NewMicroStructureStrategy(config common.StrategyConfig) (common.MarketMakingStrategy, error) {
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

	strategy := &MicroStructureStrategy{
		config:           config,
		name:             "MicroStructure",
		version:          "1.0.0",
		status:           common.StatusStopped,
		baseSpread:       baseSpread,
		maxPosition:      maxPosition,
		tickSize:         tickSize,
		maxTrades:        100,
		marketImpact:     decimal.Zero,
		adverseSelection: decimal.Zero,
		stopChan:         make(chan struct{}),
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

// Initialize prepares the strategy for trading
func (s *MicroStructureStrategy) Initialize(ctx context.Context, config common.StrategyConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.config = config
	s.status = common.StatusStopped
	return nil
}

// Start begins strategy execution
func (s *MicroStructureStrategy) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.status != common.StatusStopped {
		return fmt.Errorf("strategy is not in stopped state")
	}

	s.status = common.StatusRunning
	return nil
}

// Stop halts strategy execution
func (s *MicroStructureStrategy) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.status == common.StatusStopped {
		return nil
	}

	s.status = common.StatusStopped
	close(s.stopChan)
	return nil
}

// Quote generates bid/ask quotes based on microstructure analysis
func (s *MicroStructureStrategy) Quote(ctx context.Context, input common.QuoteInput) (*common.QuoteOutput, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.status != common.StatusRunning {
		return nil, fmt.Errorf("strategy is not running")
	}

	// Update book pressure analysis
	s.updateBookPressure(input)

	// Calculate adverse selection and market impact
	s.calculateMarketImpact(input)

	// Calculate minimum spread based on tick size
	minSpreadDecimal := s.tickSize.Div(input.MidPrice)

	// Apply inventory skew
	inventorySkew := input.Inventory.Mul(decimal.NewFromFloat(0.001))

	// Calculate microstructure-adjusted spread
	microSpread := s.calculateMicrostructureSpread(input)
	totalSkew := inventorySkew.Add(microSpread)

	// Calculate dynamic spread with minimum enforcement
	adjustedSpread := s.baseSpread.Add(totalSkew)
	if adjustedSpread.LessThan(minSpreadDecimal) {
		adjustedSpread = minSpreadDecimal
	}

	// Optimize bid/ask placement
	bid := s.optimizeBidPlacement(input.MidPrice, adjustedSpread, totalSkew)
	ask := s.optimizeAskPlacement(input.MidPrice, adjustedSpread, totalSkew)

	// Calculate optimal size based on market impact
	size := s.calculateOptimalSize(input)

	// Calculate confidence
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
func (s *MicroStructureStrategy) OnMarketData(ctx context.Context, data *common.MarketData) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !data.Volume.IsZero() {
		// Add trade event for microstructure analysis
		trade := TradeEvent{
			Price:     data.Price,
			Volume:    data.Volume,
			Side:      "unknown", // Would be determined from order book analysis
			Timestamp: data.Timestamp,
			Impact:    decimal.Zero, // Would be calculated based on price movement
		}

		s.recentTrades = append(s.recentTrades, trade)
		if len(s.recentTrades) > s.maxTrades {
			s.recentTrades = s.recentTrades[1:]
		}
	}

	s.metrics.LastUpdated = time.Now()
	return nil
}

// OnOrderFill handles order fill events
func (s *MicroStructureStrategy) OnOrderFill(ctx context.Context, fill *common.OrderFill) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.metrics.OrdersFilled++

	// Add trade to recent trades for analysis
	trade := TradeEvent{
		Price:     fill.Price,
		Volume:    fill.Quantity,
		Side:      fill.Side,
		Timestamp: fill.Timestamp,
		Impact:    decimal.Zero, // Calculate impact
	}

	s.recentTrades = append(s.recentTrades, trade)
	if len(s.recentTrades) > s.maxTrades {
		s.recentTrades = s.recentTrades[1:]
	}

	return nil
}

// OnOrderCancel handles order cancellation events
func (s *MicroStructureStrategy) OnOrderCancel(ctx context.Context, orderID uuid.UUID, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.metrics.OrdersCancelled++
	return nil
}

// UpdateConfig updates strategy configuration
func (s *MicroStructureStrategy) UpdateConfig(ctx context.Context, config common.StrategyConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.config = config
	return nil
}

// GetConfig returns current configuration
func (s *MicroStructureStrategy) GetConfig() common.StrategyConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config
}

// GetMetrics returns current metrics
func (s *MicroStructureStrategy) GetMetrics() *common.StrategyMetrics {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.metrics
}

// GetStatus returns current status
func (s *MicroStructureStrategy) GetStatus() common.StrategyStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.status
}

// Name returns strategy name
func (s *MicroStructureStrategy) Name() string {
	return s.name
}

// Version returns strategy version
func (s *MicroStructureStrategy) Version() string {
	return s.version
}

// Description returns strategy description
func (s *MicroStructureStrategy) Description() string {
	return "Advanced market making strategy using order book microstructure analysis and market impact modeling"
}

// RiskLevel returns the risk level
func (s *MicroStructureStrategy) RiskLevel() common.RiskLevel {
	return common.RiskHigh
}

// HealthCheck performs health check
func (s *MicroStructureStrategy) HealthCheck(ctx context.Context) *common.HealthStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	isHealthy := s.status == common.StatusRunning && len(s.recentTrades) > 5

	checks := map[string]bool{
		"status":         s.status == common.StatusRunning,
		"trade_data":     len(s.recentTrades) > 5,
		"microstructure": !s.marketImpact.IsZero(),
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
func (s *MicroStructureStrategy) Reset(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.recentTrades = s.recentTrades[:0]
	s.marketImpact = decimal.Zero
	s.adverseSelection = decimal.Zero
	s.bookPressure = BookPressure{}

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

func (s *MicroStructureStrategy) updateBookPressure(input common.QuoteInput) {
	// Calculate book pressure based on bid/ask imbalance
	totalVolume := input.BidPrice.Add(input.AskPrice)
	if !totalVolume.IsZero() {
		bidPressure := input.BidPrice.Div(totalVolume)
		askPressure := input.AskPrice.Div(totalVolume)

		s.bookPressure = BookPressure{
			BidPressure: bidPressure.Mul(decimal.NewFromFloat(1000.0)),
			AskPressure: askPressure.Mul(decimal.NewFromFloat(1000.0)),
			Imbalance:   bidPressure.Sub(askPressure),
			UpdateTime:  time.Now(),
		}
	}
}

func (s *MicroStructureStrategy) calculateMarketImpact(input common.QuoteInput) {
	// Simple market impact model based on recent trades
	if len(s.recentTrades) > 1 {
		// Calculate average impact from recent trades
		totalImpact := decimal.Zero
		for _, trade := range s.recentTrades {
			totalImpact = totalImpact.Add(trade.Impact)
		}
		s.marketImpact = totalImpact.Div(decimal.NewFromInt(int64(len(s.recentTrades))))
	}
}

func (s *MicroStructureStrategy) calculateMicrostructureSpread(input common.QuoteInput) decimal.Decimal {
	// Calculate spread adjustment based on microstructure signals
	baseAdjustment := decimal.NewFromFloat(0.0001)

	// Adjust for market impact
	impactAdjustment := s.marketImpact.Mul(decimal.NewFromFloat(0.5))

	// Adjust for adverse selection
	adverseAdjustment := s.adverseSelection.Mul(decimal.NewFromFloat(0.3))

	// Adjust for book pressure
	pressureAdjustment := s.bookPressure.Imbalance.Abs().Mul(decimal.NewFromFloat(0.2))

	return baseAdjustment.Add(impactAdjustment).Add(adverseAdjustment).Add(pressureAdjustment)
}

func (s *MicroStructureStrategy) optimizeBidPlacement(midPrice, spread, skew decimal.Decimal) decimal.Decimal {
	// Optimize bid placement considering tick size and market microstructure
	baseSpread := spread.Sub(skew)
	one := decimal.NewFromFloat(1.0)

	bid := midPrice.Mul(one.Sub(baseSpread))

	// Round to nearest tick
	ticks := bid.Div(s.tickSize).Round(0)
	return ticks.Mul(s.tickSize)
}

func (s *MicroStructureStrategy) optimizeAskPlacement(midPrice, spread, skew decimal.Decimal) decimal.Decimal {
	// Optimize ask placement considering tick size and market microstructure
	baseSpread := spread.Add(skew)
	one := decimal.NewFromFloat(1.0)

	ask := midPrice.Mul(one.Add(baseSpread))

	// Round to nearest tick
	ticks := ask.Div(s.tickSize).Round(0)
	return ticks.Mul(s.tickSize)
}

func (s *MicroStructureStrategy) calculateOptimalSize(input common.QuoteInput) decimal.Decimal {
	// Calculate optimal order size based on market impact and inventory
	baseSize := s.maxPosition.Mul(decimal.NewFromFloat(0.1))

	// Adjust for market impact
	impactFactor := decimal.NewFromFloat(1.0).Sub(s.marketImpact.Mul(decimal.NewFromFloat(0.5)))

	// Adjust for inventory capacity
	inventoryUsage := input.Inventory.Abs().Div(s.maxPosition)
	inventoryCapacity := decimal.NewFromFloat(1.0).Sub(inventoryUsage)

	// Calculate minimum of adjustments
	sizeFactor := impactFactor
	if inventoryCapacity.LessThan(sizeFactor) {
		sizeFactor = inventoryCapacity
	}

	// Ensure minimum size
	minSize := decimal.NewFromFloat(0.1)
	size := baseSize.Mul(sizeFactor)
	if size.LessThan(minSize) {
		size = minSize
	}

	return size
}

func (s *MicroStructureStrategy) calculateConfidence() float64 {
	// Calculate confidence based on data quality and model accuracy
	baseConfidence := 0.6

	// Adjust based on trade data availability
	if len(s.recentTrades) > 50 {
		baseConfidence += 0.3
	} else if len(s.recentTrades) > 20 {
		baseConfidence += 0.2
	} else if len(s.recentTrades) > 5 {
		baseConfidence += 0.1
	}

	// Adjust based on book pressure data age
	if time.Since(s.bookPressure.UpdateTime) < time.Minute {
		baseConfidence += 0.1
	}

	if baseConfidence > 1.0 {
		baseConfidence = 1.0
	}

	return baseConfidence
}
