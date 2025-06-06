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

// InventorySkewStrategy implements inventory-aware market making
// Implements common.MarketMakingStrategy

type InventorySkewStrategy struct {
	config           common.StrategyConfig
	name             string
	version          string
	status           common.StrategyStatus
	metrics          *common.StrategyMetrics
	mu               sync.RWMutex
	baseSpread       decimal.Decimal
	inventoryFactor  decimal.Decimal
	size             decimal.Decimal
	maxInventory     decimal.Decimal
	maxSkew          decimal.Decimal
	tickSize         decimal.Decimal
	startTime        time.Time
	lastQuote        time.Time
	currentInventory decimal.Decimal
	baseSize         decimal.Decimal // Added missing field
}

// NewInventorySkewStrategy creates a new inventory skew strategy instance
func NewInventorySkewStrategy(config common.StrategyConfig) (*InventorySkewStrategy, error) {
	strategy := &InventorySkewStrategy{
		config:  config,
		name:    "Inventory Skew Strategy",
		version: "1.0.0",
		status:  common.StatusUninitialized,
		metrics: &common.StrategyMetrics{},
	}
	return strategy, nil
}

// Core interface methods

func (s *InventorySkewStrategy) Quote(ctx context.Context, input common.QuoteInput) (*common.QuoteOutput, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.status != common.StatusRunning {
		return nil, fmt.Errorf("strategy not running")
	}

	// Calculate inventory ratio (-1 to 1)
	inventoryRatio := decimal.Zero
	if s.maxInventory.GreaterThan(decimal.Zero) {
		inventoryRatio = s.currentInventory.Div(s.maxInventory)
		// Clamp to [-1, 1]
		if inventoryRatio.GreaterThan(decimal.NewFromInt(1)) {
			inventoryRatio = decimal.NewFromInt(1)
		} else if inventoryRatio.LessThan(decimal.NewFromInt(-1)) {
			inventoryRatio = decimal.NewFromInt(-1)
		}
	}

	// Calculate skew based on inventory
	skew := inventoryRatio.Mul(s.inventoryFactor).Mul(s.maxSkew)

	// Calculate asymmetric spreads
	bidSpread := s.baseSpread.Add(skew) // Higher inventory -> wider bid spread
	askSpread := s.baseSpread.Sub(skew) // Higher inventory -> tighter ask spread

	// Ensure minimum spread
	minSpread := s.baseSpread.Div(decimal.NewFromInt(2))
	if bidSpread.LessThan(minSpread) {
		bidSpread = minSpread
	}
	if askSpread.LessThan(minSpread) {
		askSpread = minSpread
	}

	// Calculate quotes
	bidPrice := input.MidPrice.Mul(decimal.NewFromInt(1).Sub(bidSpread))
	askPrice := input.MidPrice.Mul(decimal.NewFromInt(1).Add(askSpread))

	// Round to tick size if specified
	if s.tickSize.GreaterThan(decimal.Zero) {
		bidPrice = s.roundToTick(bidPrice, false)
		askPrice = s.roundToTick(askPrice, true)
	}

	// Calculate adaptive sizes based on inventory
	bidSize, askSize := s.calculateAdaptiveSizes(inventoryRatio)

	// Calculate confidence
	confidence := s.calculateConfidence(inventoryRatio)

	output := &common.QuoteOutput{
		BidPrice:    bidPrice,
		AskPrice:    askPrice,
		BidSize:     bidSize,
		AskSize:     askSize,
		Confidence:  confidence,
		TTL:         time.Second * 25,
		GeneratedAt: time.Now(),
		Metadata: map[string]interface{}{
			"strategy_type":     "inventory_skew",
			"base_spread":       s.baseSpread.String(),
			"inventory_ratio":   inventoryRatio.String(),
			"skew":              skew.String(),
			"current_inventory": s.currentInventory.String(),
			"bid_spread":        bidSpread.String(),
			"ask_spread":        askSpread.String(),
		},
	}

	// Update metrics
	s.updateQuoteMetrics(output, inventoryRatio)

	return output, nil
}

func (s *InventorySkewStrategy) Initialize(ctx context.Context, config common.StrategyConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.config = config
	if err := s.parseConfig(); err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	// Reset inventory
	s.currentInventory = decimal.Zero

	s.status = common.StatusStarting
	s.metrics.LastUpdated = time.Now()
	return nil
}

func (s *InventorySkewStrategy) Start(ctx context.Context) error {
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

func (s *InventorySkewStrategy) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.status != common.StatusRunning {
		return fmt.Errorf("cannot stop strategy in status: %s", s.status)
	}

	s.status = common.StatusStopped
	s.metrics.LastUpdated = time.Now()
	return nil
}

func (s *InventorySkewStrategy) OnMarketData(ctx context.Context, data *common.MarketData) error {
	// Inventory skew strategy doesn't need special market data processing
	return nil
}

func (s *InventorySkewStrategy) OnOrderFill(ctx context.Context, fill *common.OrderFill) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Update inventory
	if fill.Side == "buy" {
		s.currentInventory = s.currentInventory.Add(fill.Quantity)
	} else {
		s.currentInventory = s.currentInventory.Sub(fill.Quantity)
	}

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

func (s *InventorySkewStrategy) OnOrderCancel(ctx context.Context, orderID uuid.UUID, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.metrics.OrdersCancelled++
	s.metrics.LastUpdated = time.Now()
	return nil
}

func (s *InventorySkewStrategy) UpdateConfig(ctx context.Context, config common.StrategyConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.config = config
	if err := s.parseConfig(); err != nil {
		return fmt.Errorf("failed to parse updated config: %w", err)
	}

	s.metrics.LastUpdated = time.Now()
	return nil
}

func (s *InventorySkewStrategy) GetConfig() common.StrategyConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.config
}

func (s *InventorySkewStrategy) GetMetrics() *common.StrategyMetrics {
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

func (s *InventorySkewStrategy) GetStatus() common.StrategyStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.status
}

// Metadata methods
func (s *InventorySkewStrategy) Name() string {
	return "Inventory Skew Market Making"
}

func (s *InventorySkewStrategy) Version() string {
	return "1.0.0"
}

func (s *InventorySkewStrategy) Description() string {
	return "Inventory-aware market making with position bias and adaptive spreads"
}

func (s *InventorySkewStrategy) RiskLevel() common.RiskLevel {
	return common.RiskMedium
}

func (s *InventorySkewStrategy) HealthCheck(ctx context.Context) *common.HealthStatus {
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
	health.Checks["valid_inventory_factor"] = s.inventoryFactor.GreaterThan(decimal.Zero)
	health.Checks["valid_max_inventory"] = s.maxInventory.GreaterThan(decimal.Zero)
	health.Checks["valid_size"] = s.baseSize.GreaterThan(decimal.Zero)
	health.Checks["strategy_running"] = s.status == common.StatusRunning

	// Check inventory levels
	inventoryRatio := s.currentInventory.Div(s.maxInventory).Abs()
	health.Checks["inventory_within_limits"] = inventoryRatio.LessThanOrEqual(decimal.NewFromInt(1))
	health.Checks["inventory_not_critical"] = inventoryRatio.LessThan(decimal.NewFromFloat(0.9))

	// Overall health
	for _, check := range health.Checks {
		if !check {
			health.IsHealthy = false
			health.Message = "Configuration or inventory issues detected"
			break
		}
	}

	return health
}

func (s *InventorySkewStrategy) Reset(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Reset metrics but keep configuration
	s.metrics = &common.StrategyMetrics{
		LastUpdated: time.Now(),
	}
	s.startTime = time.Now()

	// Reset inventory
	s.currentInventory = decimal.Zero

	return nil
}

// Helper methods

func (s *InventorySkewStrategy) parseConfig() error {
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

	if inventoryFactor, ok := params["inventory_factor"]; ok {
		if factorFloat, ok := inventoryFactor.(float64); ok {
			s.inventoryFactor = decimal.NewFromFloat(factorFloat)
		} else if factorStr, ok := inventoryFactor.(string); ok {
			var err error
			s.inventoryFactor, err = decimal.NewFromString(factorStr)
			if err != nil {
				return fmt.Errorf("invalid inventory_factor value: %w", err)
			}
		}
	}

	if maxInventory, ok := params["max_inventory"]; ok {
		if maxFloat, ok := maxInventory.(float64); ok {
			s.maxInventory = decimal.NewFromFloat(maxFloat)
		} else if maxStr, ok := maxInventory.(string); ok {
			var err error
			s.maxInventory, err = decimal.NewFromString(maxStr)
			if err != nil {
				return fmt.Errorf("invalid max_inventory value: %w", err)
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

	if maxSkew, ok := params["max_skew"]; ok {
		if skewFloat, ok := maxSkew.(float64); ok {
			s.maxSkew = decimal.NewFromFloat(skewFloat)
		} else if skewStr, ok := maxSkew.(string); ok {
			var err error
			s.maxSkew, err = decimal.NewFromString(skewStr)
			if err != nil {
				return fmt.Errorf("invalid max_skew value: %w", err)
			}
		}
	}

	return nil
}

func (s *InventorySkewStrategy) calculateAdaptiveSizes(inventoryRatio decimal.Decimal) (bidSize, askSize decimal.Decimal) {
	// Adjust sizes based on inventory to promote inventory neutrality
	// Higher inventory -> smaller bid size, larger ask size
	// Lower inventory -> larger bid size, smaller ask size

	inventoryAbs := inventoryRatio.Abs()

	// Size adjustment factor (0.5 to 1.5)
	adjustmentFactor := decimal.NewFromFloat(0.5).Add(inventoryAbs)

	if inventoryRatio.GreaterThan(decimal.Zero) {
		// Positive inventory - reduce bid size, increase ask size
		bidSize = s.baseSize.Div(decimal.NewFromInt(1).Add(inventoryAbs))
		askSize = s.baseSize.Mul(adjustmentFactor)
	} else if inventoryRatio.LessThan(decimal.Zero) {
		// Negative inventory - increase bid size, reduce ask size
		bidSize = s.baseSize.Mul(adjustmentFactor)
		askSize = s.baseSize.Div(decimal.NewFromInt(1).Add(inventoryAbs))
	} else {
		// Neutral inventory
		bidSize = s.baseSize
		askSize = s.baseSize
	}

	// Ensure minimum sizes (10% of base size)
	minSize := s.baseSize.Mul(decimal.NewFromFloat(0.1))
	if bidSize.LessThan(minSize) {
		bidSize = minSize
	}
	if askSize.LessThan(minSize) {
		askSize = minSize
	}

	return bidSize, askSize
}

func (s *InventorySkewStrategy) calculateConfidence(inventoryRatio decimal.Decimal) decimal.Decimal {
	baseConfidence := decimal.NewFromFloat(0.75)

	// Reduce confidence as inventory approaches limits
	inventoryPenalty := inventoryRatio.Abs().Mul(decimal.NewFromFloat(0.3))
	confidence := baseConfidence.Sub(inventoryPenalty)

	// Floor at 0.3
	minConfidence := decimal.NewFromFloat(0.3)
	if confidence.LessThan(minConfidence) {
		confidence = minConfidence
	}

	return confidence
}

func (s *InventorySkewStrategy) roundToTick(price decimal.Decimal, roundUp bool) decimal.Decimal {
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

func (s *InventorySkewStrategy) updateQuoteMetrics(output *common.QuoteOutput, inventoryRatio decimal.Decimal) {
	s.metrics.OrdersPlaced++
	s.metrics.LastUpdated = time.Now()

	// Calculate spread capture
	if output.BidPrice.GreaterThan(decimal.Zero) && output.AskPrice.GreaterThan(decimal.Zero) {
		spread := output.AskPrice.Sub(output.BidPrice).Div(output.BidPrice)
		s.metrics.SpreadCapture = spread
	}
}
