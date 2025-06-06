// MicroStructureStrategy - Advanced order book microstructure market making
package advanced

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/Orbit-CEX/Finalex/internal/marketmaking/strategies/common"
)

// MicroStructureStrategy focuses on order book microstructure and market impact
type MicroStructureStrategy struct {
	mu     sync.RWMutex
	config common.StrategyConfig

	// Strategy parameters
	baseSpread      float64
	maxPosition     float64
	tickSize        float64
	minSpreadTicks  int
	bookDepthLevels int
	impactDecayRate float64

	// Market microstructure state
	bookPressure     BookPressure
	marketImpact     float64
	adverseSelection float64

	// Metrics
	metrics   common.StrategyMetrics
	status    common.StrategyStatus
	isRunning bool

	// Order book analysis
	recentTrades []TradeEvent
	maxTrades    int
}

// BookPressure represents order book pressure metrics
type BookPressure struct {
	BidPressure float64
	AskPressure float64
	Imbalance   float64
	UpdateTime  time.Time
}

// TradeEvent represents a trade for microstructure analysis
type TradeEvent struct {
	Price     float64
	Volume    float64
	Side      string
	Timestamp time.Time
	Impact    float64
}

// NewMicroStructureStrategy creates a new microstructure strategy
func NewMicroStructureStrategy(config common.StrategyConfig) (common.MarketMakingStrategy, error) {
	// Validate required parameters
	baseSpread, ok := config.Parameters["base_spread"].(float64)
	if !ok {
		return nil, fmt.Errorf("base_spread parameter is required")
	}

	maxPosition, ok := config.Parameters["max_position"].(float64)
	if !ok {
		return nil, fmt.Errorf("max_position parameter is required")
	}

	tickSize, ok := config.Parameters["tick_size"].(float64)
	if !ok {
		return nil, fmt.Errorf("tick_size parameter is required")
	}

	minSpreadTicks := 2
	if mst, exists := config.Parameters["min_spread_ticks"].(int); exists {
		minSpreadTicks = mst
	}

	bookDepthLevels := 10
	if bdl, exists := config.Parameters["book_depth_levels"].(int); exists {
		bookDepthLevels = bdl
	}

	impactDecayRate := 0.1
	if idr, exists := config.Parameters["impact_decay_rate"].(float64); exists {
		impactDecayRate = idr
	}

	maxTrades := 1000
	if mt, exists := config.Parameters["max_trades"].(int); exists {
		maxTrades = mt
	}

	strategy := &MicroStructureStrategy{
		config:          config,
		baseSpread:      baseSpread,
		maxPosition:     maxPosition,
		tickSize:        tickSize,
		minSpreadTicks:  minSpreadTicks,
		bookDepthLevels: bookDepthLevels,
		impactDecayRate: impactDecayRate,
		maxTrades:       maxTrades,
		recentTrades:    make([]TradeEvent, 0, maxTrades),
		status:          common.StatusStopped,
		metrics: common.StrategyMetrics{
			TotalQuotes:      0,
			SuccessfulTrades: 0,
			ProfitLoss:       0.0,
			Sharpe:           0.0,
			MaxDrawdown:      0.0,
			Confidence:       1.0,
		},
	}

	return strategy, nil
}

// Initialize prepares the strategy for operation
func (s *MicroStructureStrategy) Initialize() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Initialize microstructure metrics
	s.bookPressure = BookPressure{
		BidPressure: 0.0,
		AskPressure: 0.0,
		Imbalance:   0.0,
		UpdateTime:  time.Now(),
	}

	s.marketImpact = 0.0
	s.adverseSelection = 0.0

	s.status = common.StatusInitialized
	return nil
}

// Start begins strategy operation
func (s *MicroStructureStrategy) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.status != common.StatusInitialized && s.status != common.StatusStopped {
		return fmt.Errorf("strategy must be initialized before starting")
	}

	s.isRunning = true
	s.status = common.StatusRunning
	return nil
}

// Stop halts strategy operation
func (s *MicroStructureStrategy) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.isRunning = false
	s.status = common.StatusStopped
	return nil
}

// Quote generates bid/ask quotes based on microstructure analysis
func (s *MicroStructureStrategy) Quote(input common.QuoteInput) (common.QuoteOutput, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.isRunning {
		return common.QuoteOutput{}, fmt.Errorf("strategy is not running")
	}

	// Analyze order book microstructure
	s.updateBookPressure(input)
	s.updateMarketImpact()
	s.updateAdverseSelection()

	// Base spread with microstructure adjustments
	microSpread := s.baseSpread + s.marketImpact*0.5 + s.adverseSelection*0.3

	// Ensure minimum tick size compliance
	minSpread := float64(s.minSpreadTicks) * s.tickSize / input.MidPrice
	adjustedSpread := math.Max(microSpread, minSpread)

	// Asymmetric spread based on order book pressure
	pressureSkew := (s.bookPressure.BidPressure - s.bookPressure.AskPressure) * 0.0005
	inventorySkew := input.Inventory * 0.001
	totalSkew := pressureSkew + inventorySkew

	// Calculate optimal quote placement
	bid := s.optimizeBidPlacement(input.MidPrice, adjustedSpread, totalSkew)
	ask := s.optimizeAskPlacement(input.MidPrice, adjustedSpread, totalSkew)

	// Position sizing based on microstructure signals
	signalStrength := math.Abs(pressureSkew) + math.Abs(s.adverseSelection)
	confidenceFactor := 1.0 / (1.0 + signalStrength*10.0)
	size := s.maxPosition * 0.02 * confidenceFactor

	// Calculate confidence based on microstructure stability
	confidence := s.calculateConfidence(signalStrength, s.marketImpact)

	// Update metrics
	s.metrics.TotalQuotes++
	s.metrics.Confidence = confidence

	return common.QuoteOutput{
		BidPrice:   bid,
		AskPrice:   ask,
		BidSize:    size,
		AskSize:    size,
		Confidence: confidence,
		Timestamp:  time.Now(),
	}, nil
}

// OnMarketData processes incoming market data
func (s *MicroStructureStrategy) OnMarketData(data common.MarketData) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Record trade for microstructure analysis
	if data.Volume > 0 {
		trade := TradeEvent{
			Price:     data.Price,
			Volume:    data.Volume,
			Side:      "unknown", // Would be determined from order book
			Timestamp: time.Now(),
			Impact:    data.Impact,
		}

		s.addTradeEvent(trade)
	}

	return nil
}

// OnOrderFill handles order fill events
func (s *MicroStructureStrategy) OnOrderFill(fill common.OrderFill) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.metrics.SuccessfulTrades++
	s.metrics.ProfitLoss += fill.PnL

	// Record fill for microstructure analysis
	trade := TradeEvent{
		Price:     fill.Price,
		Volume:    fill.Quantity,
		Side:      fill.Side,
		Timestamp: fill.Timestamp,
		Impact:    0.0, // Calculate impact
	}

	s.addTradeEvent(trade)

	return nil
}

// OnOrderCancel handles order cancellation events
func (s *MicroStructureStrategy) OnOrderCancel(cancel common.OrderCancel) error {
	// Cancellations might indicate adverse selection
	s.mu.Lock()
	defer s.mu.Unlock()

	s.adverseSelection += 0.001 // Small penalty for cancellations

	return nil
}

// UpdateConfig updates strategy configuration
func (s *MicroStructureStrategy) UpdateConfig(config common.StrategyConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Update parameters if provided
	if baseSpread, ok := config.Parameters["base_spread"].(float64); ok {
		s.baseSpread = baseSpread
	}
	if maxPosition, ok := config.Parameters["max_position"].(float64); ok {
		s.maxPosition = maxPosition
	}
	if tickSize, ok := config.Parameters["tick_size"].(float64); ok {
		s.tickSize = tickSize
	}

	s.config = config
	return nil
}

// GetMetrics returns current strategy metrics
func (s *MicroStructureStrategy) GetMetrics() common.StrategyMetrics {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.metrics
}

// GetStatus returns current strategy status
func (s *MicroStructureStrategy) GetStatus() common.StrategyStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.status
}

// GetInfo returns strategy information
func (s *MicroStructureStrategy) GetInfo() common.StrategyInfo {
	return common.StrategyInfo{
		Name:        "MicroStructure Strategy",
		Description: "Advanced order book microstructure and market impact analysis",
		RiskLevel:   common.RiskHigh,
		Complexity:  common.ComplexityHigh,
		Version:     "1.0.0",
		Parameters: []common.ParameterDefinition{
			{
				Name:        "base_spread",
				Type:        "float",
				Default:     0.001,
				MinValue:    0.0001,
				MaxValue:    0.01,
				Description: "Base spread percentage",
				Required:    true,
			},
			{
				Name:        "max_position",
				Type:        "float",
				Default:     1000.0,
				MinValue:    100.0,
				MaxValue:    100000.0,
				Description: "Maximum position size",
				Required:    true,
			},
			{
				Name:        "tick_size",
				Type:        "float",
				Default:     0.01,
				MinValue:    0.001,
				MaxValue:    1.0,
				Description: "Minimum price increment",
				Required:    true,
			},
			{
				Name:        "min_spread_ticks",
				Type:        "int",
				Default:     2,
				MinValue:    1,
				MaxValue:    10,
				Description: "Minimum spread in ticks",
				Required:    false,
			},
			{
				Name:        "book_depth_levels",
				Type:        "int",
				Default:     10,
				MinValue:    5,
				MaxValue:    50,
				Description: "Order book depth levels to analyze",
				Required:    false,
			},
		},
	}
}

// HealthCheck performs strategy health verification
func (s *MicroStructureStrategy) HealthCheck() common.HealthStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.isRunning {
		return common.HealthStatus{
			IsHealthy: false,
			Message:   "Strategy is not running",
			LastCheck: time.Now(),
		}
	}

	// Check if tick size is valid
	if s.tickSize <= 0 {
		return common.HealthStatus{
			IsHealthy: false,
			Message:   "Invalid tick size",
			LastCheck: time.Now(),
		}
	}

	return common.HealthStatus{
		IsHealthy: true,
		Message:   "Strategy is healthy",
		LastCheck: time.Now(),
	}
}

// Helper methods

func (s *MicroStructureStrategy) updateBookPressure(input common.QuoteInput) {
	// Simplified book pressure calculation
	// In production, this would analyze actual order book data

	// Mock pressure based on spread and volume
	bidPressure := 1000.0 * (1.0 + input.Volatility)
	askPressure := 1000.0 * (1.0 - input.Volatility)

	s.bookPressure = BookPressure{
		BidPressure: bidPressure,
		AskPressure: askPressure,
		Imbalance:   (bidPressure - askPressure) / (bidPressure + askPressure),
		UpdateTime:  time.Now(),
	}
}

func (s *MicroStructureStrategy) updateMarketImpact() {
	// Calculate market impact based on recent trades
	if len(s.recentTrades) == 0 {
		s.marketImpact = 0.0
		return
	}

	// Exponentially weighted impact
	totalImpact := 0.0
	totalWeight := 0.0
	now := time.Now()

	for i := len(s.recentTrades) - 1; i >= 0; i-- {
		trade := s.recentTrades[i]
		age := now.Sub(trade.Timestamp).Seconds()
		weight := math.Exp(-age * s.impactDecayRate)

		totalImpact += trade.Impact * weight
		totalWeight += weight
	}

	if totalWeight > 0 {
		s.marketImpact = totalImpact / totalWeight
	}
}

func (s *MicroStructureStrategy) updateAdverseSelection() {
	// Calculate adverse selection based on trade patterns
	if len(s.recentTrades) < 10 {
		s.adverseSelection = 0.0
		return
	}

	// Simplified adverse selection metric
	recentTrades := s.recentTrades[len(s.recentTrades)-10:]
	priceVariation := 0.0

	for i := 1; i < len(recentTrades); i++ {
		priceChange := math.Abs(recentTrades[i].Price - recentTrades[i-1].Price)
		priceVariation += priceChange
	}

	s.adverseSelection = priceVariation / float64(len(recentTrades)-1) * 0.1
}

func (s *MicroStructureStrategy) optimizeBidPlacement(mid, spread, skew float64) float64 {
	targetBid := mid * (1 - spread/2 - skew)
	return s.roundToTickSize(targetBid)
}

func (s *MicroStructureStrategy) optimizeAskPlacement(mid, spread, skew float64) float64 {
	targetAsk := mid * (1 + spread/2 + skew)
	return s.roundToTickSize(targetAsk)
}

func (s *MicroStructureStrategy) roundToTickSize(price float64) float64 {
	if s.tickSize <= 0 {
		return price
	}
	return math.Round(price/s.tickSize) * s.tickSize
}

func (s *MicroStructureStrategy) addTradeEvent(trade TradeEvent) {
	s.recentTrades = append(s.recentTrades, trade)
	if len(s.recentTrades) > s.maxTrades {
		s.recentTrades = s.recentTrades[1:]
	}
}

func (s *MicroStructureStrategy) calculateConfidence(signalStrength, impact float64) float64 {
	// Confidence decreases with high signal noise and market impact
	signalComponent := 1.0 / (1.0 + signalStrength*10.0)
	impactComponent := 1.0 / (1.0 + impact*50.0)

	return math.Min(1.0, signalComponent*impactComponent)
}
