package basic

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/Finalex/internal/marketmaking/strategies/common"
)

// InventorySkewStrategy implements inventory-aware market making
type InventorySkewStrategy struct {
	config    *InventorySkewConfig
	metrics   *common.StrategyMetrics
	status    common.StrategyStatus
	inventory float64
	mu        sync.RWMutex
	stopChan  chan struct{}
}

type InventorySkewConfig struct {
	BaseSpread   float64 `json:"base_spread"`
	InvFactor    float64 `json:"inv_factor"`
	Size         float64 `json:"size"`
	MaxSkew      float64 `json:"max_skew"`
	MaxInventory float64 `json:"max_inventory"`
	TickSize     float64 `json:"tick_size"`
}

// NewInventorySkewStrategy creates a new inventory-aware market making strategy
func NewInventorySkewStrategy() *InventorySkewStrategy {
	return &InventorySkewStrategy{
		config: &InventorySkewConfig{
			BaseSpread:   0.001,
			InvFactor:    0.001,
			Size:         100.0,
			MaxSkew:      0.5,
			MaxInventory: 1000.0,
			TickSize:     0.01,
		},
		metrics: &common.StrategyMetrics{
			StartTime: time.Now(),
		},
		status:   common.StatusStopped,
		stopChan: make(chan struct{}),
	}
}

// Initialize prepares the strategy for trading
func (s *InventorySkewStrategy) Initialize(ctx context.Context, config common.StrategyConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Parse inventory skew strategy config
	if baseSpread, ok := config.Parameters["base_spread"].(float64); ok {
		s.config.BaseSpread = baseSpread
	}
	if invFactor, ok := config.Parameters["inv_factor"].(float64); ok {
		s.config.InvFactor = invFactor
	}
	if size, ok := config.Parameters["size"].(float64); ok {
		s.config.Size = size
	}
	if maxSkew, ok := config.Parameters["max_skew"].(float64); ok {
		s.config.MaxSkew = maxSkew
	}
	if maxInventory, ok := config.Parameters["max_inventory"].(float64); ok {
		s.config.MaxInventory = maxInventory
	}
	if tickSize, ok := config.Parameters["tick_size"].(float64); ok {
		s.config.TickSize = tickSize
	}

	s.status = common.StatusInitialized
	return nil
}

// Start begins strategy execution
func (s *InventorySkewStrategy) Start(ctx context.Context) error {
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
func (s *InventorySkewStrategy) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.status != common.StatusRunning {
		return fmt.Errorf("cannot stop strategy in status: %s", s.status)
	}

	close(s.stopChan)
	s.status = common.StatusStopped
	return nil
}

// Quote generates inventory-skewed bid/ask quotes
func (s *InventorySkewStrategy) Quote(input common.QuoteInput) (common.QuoteOutput, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.status != common.StatusRunning {
		return common.QuoteOutput{}, fmt.Errorf("strategy not running")
	}

	// Use provided inventory or internal tracking
	inventory := input.Inventory
	if inventory == 0 {
		inventory = s.inventory
	}

	// Calculate inventory skew
	inventoryRatio := inventory / s.config.MaxInventory
	invSkew := s.config.InvFactor * inventoryRatio

	// Cap the skew to prevent extreme adjustments
	invSkew = math.Max(-s.config.MaxSkew, math.Min(s.config.MaxSkew, invSkew))

	// Apply skew to spread (positive inventory widens bid-ask, makes selling easier)
	bidSpread := s.config.BaseSpread/2 + math.Max(0, invSkew)
	askSpread := s.config.BaseSpread/2 + math.Max(0, -invSkew)

	bid := input.MidPrice * (1 - bidSpread)
	ask := input.MidPrice * (1 + askSpread)

	// Round to tick size
	if s.config.TickSize > 0 {
		bid = s.roundToTick(bid, false)
		ask = s.roundToTick(ask, true)
	}

	// Adjust size based on inventory risk
	inventoryPenalty := math.Abs(inventoryRatio)
	sizeMultiplier := math.Max(0.1, 1.0-inventoryPenalty*0.5)
	adjustedSize := s.config.Size * sizeMultiplier

	// Asymmetric sizing - favor trades that reduce inventory
	bidSize := adjustedSize
	askSize := adjustedSize

	if inventory > 0 {
		// Long inventory - favor selling (larger ask size)
		askSize *= 1.5
		bidSize *= 0.7
	} else if inventory < 0 {
		// Short inventory - favor buying (larger bid size)
		bidSize *= 1.5
		askSize *= 0.7
	}

	output := common.QuoteOutput{
		BidPrice:   bid,
		AskPrice:   ask,
		BidSize:    bidSize,
		AskSize:    askSize,
		Confidence: s.calculateConfidence(inventoryRatio),
		Timestamp:  time.Now(),
	}

	// Update metrics
	s.updateMetrics(output, inventory)

	return output, nil
}

// OnMarketData handles market data updates
func (s *InventorySkewStrategy) OnMarketData(data common.MarketData) error {
	// Inventory skew strategy doesn't need special market data processing
	return nil
}

// OnOrderFill handles order fill notifications and updates inventory
func (s *InventorySkewStrategy) OnOrderFill(fill common.OrderFill) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Update inventory tracking
	if fill.Side == "buy" {
		s.inventory += fill.Quantity
		s.metrics.BuyVolume += fill.Quantity
	} else {
		s.inventory -= fill.Quantity
		s.metrics.SellVolume += fill.Quantity
	}

	// Update metrics
	s.metrics.TotalVolume += fill.Quantity
	s.metrics.OrdersPlaced++

	// Calculate PnL
	if fill.Side == "buy" {
		s.metrics.TotalPnL -= fill.Price * fill.Quantity
	} else {
		s.metrics.TotalPnL += fill.Price * fill.Quantity
	}

	return nil
}

// OnOrderCancel handles order cancellation notifications
func (s *InventorySkewStrategy) OnOrderCancel(cancel common.OrderCancel) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.metrics.OrdersCancelled++
	return nil
}

// UpdateConfig updates strategy configuration
func (s *InventorySkewStrategy) UpdateConfig(config common.StrategyConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if baseSpread, ok := config.Parameters["base_spread"].(float64); ok {
		s.config.BaseSpread = baseSpread
	}
	if invFactor, ok := config.Parameters["inv_factor"].(float64); ok {
		s.config.InvFactor = invFactor
	}
	if size, ok := config.Parameters["size"].(float64); ok {
		s.config.Size = size
	}
	if maxSkew, ok := config.Parameters["max_skew"].(float64); ok {
		s.config.MaxSkew = maxSkew
	}
	if maxInventory, ok := config.Parameters["max_inventory"].(float64); ok {
		s.config.MaxInventory = maxInventory
	}

	return nil
}

// GetMetrics returns current strategy metrics
func (s *InventorySkewStrategy) GetMetrics() common.StrategyMetrics {
	s.mu.RLock()
	defer s.mu.RUnlock()

	metrics := *s.metrics
	metrics.Uptime = time.Since(s.metrics.StartTime)
	metrics.CurrentInventory = s.inventory

	if s.metrics.OrdersPlaced > 0 {
		metrics.SuccessRate = float64(s.metrics.OrdersPlaced-s.metrics.OrdersCancelled) / float64(s.metrics.OrdersPlaced)
	}

	// Calculate inventory utilization
	if s.config.MaxInventory > 0 {
		metrics.InventoryUtilization = math.Abs(s.inventory) / s.config.MaxInventory
	}

	return metrics
}

// GetStatus returns current strategy status
func (s *InventorySkewStrategy) GetStatus() common.StrategyStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.status
}

// GetInfo returns strategy information
func (s *InventorySkewStrategy) GetInfo() common.StrategyInfo {
	return common.StrategyInfo{
		Name:        "InventorySkew",
		Version:     "1.0.0",
		Description: "Inventory-aware market making with asymmetric quoting",
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
				Name:        "inv_factor",
				Type:        "float",
				Description: "Inventory skew factor",
				Default:     0.001,
				MinValue:    0.0,
				MaxValue:    0.01,
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
				Name:        "max_skew",
				Type:        "float",
				Description: "Maximum allowed skew",
				Default:     0.5,
				MinValue:    0.0,
				MaxValue:    2.0,
				Required:    false,
			},
			{
				Name:        "max_inventory",
				Type:        "float",
				Description: "Maximum inventory position",
				Default:     1000.0,
				MinValue:    100.0,
				MaxValue:    100000.0,
				Required:    false,
			},
		},
	}
}

// HealthCheck performs health validation
func (s *InventorySkewStrategy) HealthCheck() common.HealthStatus {
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

	if s.config.InvFactor < 0 {
		health.IsHealthy = false
		health.Issues = append(health.Issues, "Invalid inventory factor")
	}

	// Check inventory levels
	inventoryRatio := math.Abs(s.inventory) / s.config.MaxInventory
	if inventoryRatio > 0.9 {
		health.Issues = append(health.Issues, fmt.Sprintf("High inventory utilization: %.1f%%", inventoryRatio*100))
	}

	if inventoryRatio > 1.0 {
		health.IsHealthy = false
		health.Issues = append(health.Issues, "Inventory limit exceeded")
	}

	return health
}

// GetInventory returns current inventory position
func (s *InventorySkewStrategy) GetInventory() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.inventory
}

// SetInventory sets current inventory position (for external inventory management)
func (s *InventorySkewStrategy) SetInventory(inventory float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inventory = inventory
}

// Helper methods

func (s *InventorySkewStrategy) calculateConfidence(inventoryRatio float64) float64 {
	// Lower confidence when inventory is high
	confidence := 1.0 - math.Abs(inventoryRatio)*0.5
	return math.Max(0.1, math.Min(1.0, confidence))
}

func (s *InventorySkewStrategy) roundToTick(price float64, roundUp bool) float64 {
	if s.config.TickSize <= 0 {
		return price
	}

	if roundUp {
		return math.Ceil(price/s.config.TickSize) * s.config.TickSize
	} else {
		return math.Floor(price/s.config.TickSize) * s.config.TickSize
	}
}

func (s *InventorySkewStrategy) updateMetrics(output common.QuoteOutput, inventory float64) {
	s.metrics.LastUpdate = time.Now()
	s.metrics.QuotesGenerated++
	s.metrics.CurrentInventory = inventory

	if output.BidPrice > 0 && output.AskPrice > 0 {
		spread := (output.AskPrice - output.BidPrice) / output.BidPrice
		s.metrics.AvgSpread = (s.metrics.AvgSpread*float64(s.metrics.QuotesGenerated-1) + spread) / float64(s.metrics.QuotesGenerated)
	}
}
