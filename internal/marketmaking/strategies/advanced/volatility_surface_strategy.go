// VolatilitySurfaceStrategy - Advanced volatility term structure market making
package advanced

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/Orbit-CEX/Finalex/internal/marketmaking/strategies/common"
)

// VolatilitySurfaceStrategy models volatility across different time horizons
type VolatilitySurfaceStrategy struct {
	mu       sync.RWMutex
	config   common.StrategyConfig
	tickSize float64

	// Strategy parameters
	baseSpread  float64
	maxPosition float64

	// Volatility surface modeling
	volSurface map[time.Duration]float64
	lastUpdate time.Time

	// Metrics
	metrics   common.StrategyMetrics
	status    common.StrategyStatus
	isRunning bool

	// Price history for analysis
	priceHistory []float64
	maxHistory   int
}

// NewVolatilitySurfaceStrategy creates a new volatility surface strategy
func NewVolatilitySurfaceStrategy(config common.StrategyConfig) (common.MarketMakingStrategy, error) {
	// Validate required parameters
	baseSpread, ok := config.Parameters["base_spread"].(float64)
	if !ok {
		return nil, fmt.Errorf("base_spread parameter is required")
	}

	maxPosition, ok := config.Parameters["max_position"].(float64)
	if !ok {
		return nil, fmt.Errorf("max_position parameter is required")
	}

	tickSize := 0.01
	if ts, exists := config.Parameters["tick_size"].(float64); exists {
		tickSize = ts
	}

	maxHistory := 1000
	if mh, exists := config.Parameters["max_history"].(int); exists {
		maxHistory = mh
	}

	strategy := &VolatilitySurfaceStrategy{
		config:       config,
		tickSize:     tickSize,
		baseSpread:   baseSpread,
		maxPosition:  maxPosition,
		volSurface:   make(map[time.Duration]float64),
		maxHistory:   maxHistory,
		priceHistory: make([]float64, 0, maxHistory),
		status:       common.StatusStopped,
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
func (s *VolatilitySurfaceStrategy) Initialize() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Initialize volatility surface with default values
	horizons := []time.Duration{
		time.Minute,
		5 * time.Minute,
		15 * time.Minute,
		time.Hour,
	}

	for _, horizon := range horizons {
		s.volSurface[horizon] = 0.01 // Default 1% volatility
	}

	s.status = common.StatusInitialized
	return nil
}

// Start begins strategy operation
func (s *VolatilitySurfaceStrategy) Start() error {
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
func (s *VolatilitySurfaceStrategy) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.isRunning = false
	s.status = common.StatusStopped
	return nil
}

// Quote generates bid/ask quotes based on volatility surface analysis
func (s *VolatilitySurfaceStrategy) Quote(input common.QuoteInput) (common.QuoteOutput, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.isRunning {
		return common.QuoteOutput{}, fmt.Errorf("strategy is not running")
	}

	// Update price history and volatility surface
	s.updatePriceHistory(input.MidPrice)
	s.updateVolatilitySurface(input.Volatility)

	// Calculate term structure of volatility
	shortTermVol := s.getVolatility(time.Minute)
	mediumTermVol := s.getVolatility(5 * time.Minute)
	longTermVol := s.getVolatility(15 * time.Minute)

	// Volatility term structure analysis
	volSlope := (longTermVol - shortTermVol) / shortTermVol
	volConvexity := mediumTermVol - (shortTermVol+longTermVol)/2

	// Adjust spread based on volatility surface characteristics
	spreadMultiplier := 1.0 + shortTermVol*5.0 + math.Abs(volSlope)*2.0 + math.Abs(volConvexity)*3.0
	adjustedSpread := s.baseSpread * spreadMultiplier

	// Inventory management with volatility adjustment
	inventoryRisk := math.Abs(input.Inventory) * shortTermVol
	inventorySkew := input.Inventory * (0.001 + inventoryRisk*0.01)

	// Calculate quotes
	bid := input.MidPrice * (1 - adjustedSpread/2 - inventorySkew)
	ask := input.MidPrice * (1 + adjustedSpread/2 + inventorySkew)

	// Round to tick size
	bid = s.roundToTickSize(bid)
	ask = s.roundToTickSize(ask)

	// Dynamic sizing based on volatility and capacity
	volAdjustment := 1.0 / (1.0 + shortTermVol*20.0)
	inventoryAdjustment := math.Max(0.1, 1.0-math.Abs(input.Inventory)/s.maxPosition)
	size := s.maxPosition * 0.05 * volAdjustment * inventoryAdjustment

	// Calculate confidence based on volatility surface stability
	confidence := s.calculateConfidence(shortTermVol, volSlope, volConvexity)

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
func (s *VolatilitySurfaceStrategy) OnMarketData(data common.MarketData) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.updatePriceHistory(data.Price)
	s.updateVolatilitySurface(data.Volatility)

	return nil
}

// OnOrderFill handles order fill events
func (s *VolatilitySurfaceStrategy) OnOrderFill(fill common.OrderFill) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.metrics.SuccessfulTrades++
	s.metrics.ProfitLoss += fill.PnL

	return nil
}

// OnOrderCancel handles order cancellation events
func (s *VolatilitySurfaceStrategy) OnOrderCancel(cancel common.OrderCancel) error {
	// No specific action needed for volatility surface strategy
	return nil
}

// UpdateConfig updates strategy configuration
func (s *VolatilitySurfaceStrategy) UpdateConfig(config common.StrategyConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Update parameters if provided
	if baseSpread, ok := config.Parameters["base_spread"].(float64); ok {
		s.baseSpread = baseSpread
	}
	if maxPosition, ok := config.Parameters["max_position"].(float64); ok {
		s.maxPosition = maxPosition
	}

	s.config = config
	return nil
}

// GetMetrics returns current strategy metrics
func (s *VolatilitySurfaceStrategy) GetMetrics() common.StrategyMetrics {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.metrics
}

// GetStatus returns current strategy status
func (s *VolatilitySurfaceStrategy) GetStatus() common.StrategyStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.status
}

// GetInfo returns strategy information
func (s *VolatilitySurfaceStrategy) GetInfo() common.StrategyInfo {
	return common.StrategyInfo{
		Name:        "Volatility Surface Strategy",
		Description: "Advanced volatility term structure market making",
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
				Required:    false,
			},
		},
	}
}

// HealthCheck performs strategy health verification
func (s *VolatilitySurfaceStrategy) HealthCheck() common.HealthStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.isRunning {
		return common.HealthStatus{
			IsHealthy: false,
			Message:   "Strategy is not running",
			LastCheck: time.Now(),
		}
	}

	// Check if volatility surface is properly initialized
	if len(s.volSurface) == 0 {
		return common.HealthStatus{
			IsHealthy: false,
			Message:   "Volatility surface not initialized",
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

func (s *VolatilitySurfaceStrategy) updatePriceHistory(price float64) {
	s.priceHistory = append(s.priceHistory, price)
	if len(s.priceHistory) > s.maxHistory {
		s.priceHistory = s.priceHistory[1:]
	}
}

func (s *VolatilitySurfaceStrategy) updateVolatilitySurface(currentVol float64) {
	now := time.Now()

	// Update different time horizon volatilities with decay
	horizons := []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute, time.Hour}

	for _, horizon := range horizons {
		if existing, exists := s.volSurface[horizon]; exists {
			// Exponential decay based on time horizon
			decayFactor := math.Exp(-float64(now.Sub(s.lastUpdate)) / float64(horizon))
			s.volSurface[horizon] = existing*decayFactor + currentVol*(1-decayFactor)
		} else {
			s.volSurface[horizon] = currentVol
		}
	}

	s.lastUpdate = now
}

func (s *VolatilitySurfaceStrategy) getVolatility(horizon time.Duration) float64 {
	if vol, exists := s.volSurface[horizon]; exists {
		return vol
	}
	return 0.01 // Default volatility
}

func (s *VolatilitySurfaceStrategy) roundToTickSize(price float64) float64 {
	if s.tickSize <= 0 {
		return price
	}
	return math.Round(price/s.tickSize) * s.tickSize
}

func (s *VolatilitySurfaceStrategy) calculateConfidence(vol, slope, convexity float64) float64 {
	// Confidence decreases with high volatility and extreme term structure shapes
	volComponent := 1.0 / (1.0 + vol*20.0)
	structureComponent := 1.0 / (1.0 + math.Abs(slope)*10.0 + math.Abs(convexity)*10.0)

	return math.Min(1.0, volComponent*structureComponent)
}
