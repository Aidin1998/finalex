// CrossExchangeArbitrageStrategy - Advanced cross-exchange arbitrage market making
package advanced

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/Orbit-CEX/Finalex/internal/marketmaking/strategies/common"
)

// CrossExchangeArbitrageStrategy monitors price differences across exchanges
type CrossExchangeArbitrageStrategy struct {
	mu     sync.RWMutex
	config common.StrategyConfig

	// Strategy parameters
	baseSpread   float64
	maxPosition  float64
	arbThreshold float64 // Minimum arbitrage opportunity
	maxArbAge    time.Duration

	// Exchange configuration
	exchangeWeights map[string]float64
	exchangePrices  map[string]ExchangePrice

	// Arbitrage state
	fairValue      float64
	arbOpportunity float64
	lastArbUpdate  time.Time

	// Metrics
	metrics   common.StrategyMetrics
	status    common.StrategyStatus
	isRunning bool

	// Performance tracking
	arbTrades    []ArbitrageEvent
	maxArbTrades int
}

// ExchangePrice represents price data from an exchange
type ExchangePrice struct {
	Price     float64
	Volume    float64
	Timestamp time.Time
	Spread    float64
	IsValid   bool
}

// ArbitrageEvent represents an arbitrage opportunity
type ArbitrageEvent struct {
	Opportunity float64
	FairValue   float64
	LocalPrice  float64
	Profit      float64
	Timestamp   time.Time
	Executed    bool
}

// NewCrossExchangeArbitrageStrategy creates a new cross-exchange arbitrage strategy
func NewCrossExchangeArbitrageStrategy(config common.StrategyConfig) (common.MarketMakingStrategy, error) {
	// Validate required parameters
	baseSpread, ok := config.Parameters["base_spread"].(float64)
	if !ok {
		return nil, fmt.Errorf("base_spread parameter is required")
	}

	maxPosition, ok := config.Parameters["max_position"].(float64)
	if !ok {
		return nil, fmt.Errorf("max_position parameter is required")
	}

	arbThreshold, ok := config.Parameters["arbitrage_threshold"].(float64)
	if !ok {
		arbThreshold = 0.001 // Default 0.1% arbitrage threshold
	}

	maxArbAge := 5 * time.Second
	if age, exists := config.Parameters["max_arbitrage_age_seconds"].(float64); exists {
		maxArbAge = time.Duration(age) * time.Second
	}

	maxArbTrades := 1000
	if mat, exists := config.Parameters["max_arbitrage_trades"].(int); exists {
		maxArbTrades = mat
	}

	// Default exchange weights
	exchangeWeights := map[string]float64{
		"binance":  0.4,
		"coinbase": 0.3,
		"kraken":   0.2,
		"okx":      0.1,
	}

	// Override with config if provided
	if weights, exists := config.Parameters["exchange_weights"].(map[string]float64); exists {
		exchangeWeights = weights
	}

	strategy := &CrossExchangeArbitrageStrategy{
		config:          config,
		baseSpread:      baseSpread,
		maxPosition:     maxPosition,
		arbThreshold:    arbThreshold,
		maxArbAge:       maxArbAge,
		exchangeWeights: exchangeWeights,
		exchangePrices:  make(map[string]ExchangePrice),
		maxArbTrades:    maxArbTrades,
		arbTrades:       make([]ArbitrageEvent, 0, maxArbTrades),
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
func (s *CrossExchangeArbitrageStrategy) Initialize() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Initialize exchange price tracking
	for exchange := range s.exchangeWeights {
		s.exchangePrices[exchange] = ExchangePrice{
			Price:     0.0,
			Volume:    0.0,
			Timestamp: time.Now(),
			Spread:    0.0,
			IsValid:   false,
		}
	}

	s.fairValue = 0.0
	s.arbOpportunity = 0.0
	s.lastArbUpdate = time.Now()

	s.status = common.StatusInitialized
	return nil
}

// Start begins strategy operation
func (s *CrossExchangeArbitrageStrategy) Start() error {
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
func (s *CrossExchangeArbitrageStrategy) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.isRunning = false
	s.status = common.StatusStopped
	return nil
}

// Quote generates bid/ask quotes based on cross-exchange arbitrage analysis
func (s *CrossExchangeArbitrageStrategy) Quote(input common.QuoteInput) (common.QuoteOutput, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.isRunning {
		return common.QuoteOutput{}, fmt.Errorf("strategy is not running")
	}

	// Update local exchange price
	s.updateLocalPrice(input.MidPrice, input.Volatility)

	// Calculate cross-exchange fair value
	s.calculateFairValue()

	// Detect arbitrage opportunities
	s.detectArbitrageOpportunity(input.MidPrice)

	// Adjust spread based on arbitrage signal
	spreadAdjustment := math.Abs(s.arbOpportunity) * 2.0 // Tighten spread when arbitrage detected
	adjustedSpread := s.baseSpread * (1.0 - spreadAdjustment)
	adjustedSpread = math.Max(adjustedSpread, s.baseSpread*0.3) // Minimum spread

	// Skew quotes toward arbitrage direction
	arbSkew := s.arbOpportunity * 0.5
	inventorySkew := input.Inventory * 0.001
	totalSkew := arbSkew + inventorySkew

	// Calculate quotes
	bid := input.MidPrice * (1 - adjustedSpread/2 - totalSkew)
	ask := input.MidPrice * (1 + adjustedSpread/2 + totalSkew)

	// Size based on arbitrage strength and inventory capacity
	arbStrength := math.Abs(s.arbOpportunity) / s.arbThreshold
	inventoryCapacity := math.Max(0.1, 1.0-math.Abs(input.Inventory)/s.maxPosition)
	size := s.maxPosition * 0.1 * arbStrength * inventoryCapacity

	// Calculate confidence based on arbitrage data quality
	confidence := s.calculateConfidence()

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

// OnMarketData processes incoming market data (potentially from multiple exchanges)
func (s *CrossExchangeArbitrageStrategy) OnMarketData(data common.MarketData) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Update exchange price if exchange is specified
	if data.Exchange != "" {
		s.updateExchangePrice(data.Exchange, data.Price, data.Volume, data.Spread)
	}

	return nil
}

// OnOrderFill handles order fill events
func (s *CrossExchangeArbitrageStrategy) OnOrderFill(fill common.OrderFill) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.metrics.SuccessfulTrades++
	s.metrics.ProfitLoss += fill.PnL

	// Record arbitrage trade if opportunity was present
	if math.Abs(s.arbOpportunity) > s.arbThreshold {
		arbEvent := ArbitrageEvent{
			Opportunity: s.arbOpportunity,
			FairValue:   s.fairValue,
			LocalPrice:  fill.Price,
			Profit:      fill.PnL,
			Timestamp:   fill.Timestamp,
			Executed:    true,
		}

		s.addArbitrageEvent(arbEvent)
	}

	return nil
}

// OnOrderCancel handles order cancellation events
func (s *CrossExchangeArbitrageStrategy) OnOrderCancel(cancel common.OrderCancel) error {
	// No specific action needed for arbitrage strategy
	return nil
}

// UpdateConfig updates strategy configuration
func (s *CrossExchangeArbitrageStrategy) UpdateConfig(config common.StrategyConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Update parameters if provided
	if baseSpread, ok := config.Parameters["base_spread"].(float64); ok {
		s.baseSpread = baseSpread
	}
	if maxPosition, ok := config.Parameters["max_position"].(float64); ok {
		s.maxPosition = maxPosition
	}
	if arbThreshold, ok := config.Parameters["arbitrage_threshold"].(float64); ok {
		s.arbThreshold = arbThreshold
	}
	if weights, ok := config.Parameters["exchange_weights"].(map[string]float64); ok {
		s.exchangeWeights = weights
	}

	s.config = config
	return nil
}

// GetMetrics returns current strategy metrics
func (s *CrossExchangeArbitrageStrategy) GetMetrics() common.StrategyMetrics {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.metrics
}

// GetStatus returns current strategy status
func (s *CrossExchangeArbitrageStrategy) GetStatus() common.StrategyStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.status
}

// GetInfo returns strategy information
func (s *CrossExchangeArbitrageStrategy) GetInfo() common.StrategyInfo {
	return common.StrategyInfo{
		Name:        "Cross-Exchange Arbitrage Strategy",
		Description: "Advanced cross-exchange arbitrage market making",
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
				Name:        "arbitrage_threshold",
				Type:        "float",
				Default:     0.001,
				MinValue:    0.0001,
				MaxValue:    0.01,
				Description: "Minimum arbitrage opportunity threshold",
				Required:    false,
			},
			{
				Name:        "max_arbitrage_age_seconds",
				Type:        "float",
				Default:     5.0,
				MinValue:    1.0,
				MaxValue:    60.0,
				Description: "Maximum age of arbitrage data in seconds",
				Required:    false,
			},
		},
	}
}

// HealthCheck performs strategy health verification
func (s *CrossExchangeArbitrageStrategy) HealthCheck() common.HealthStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.isRunning {
		return common.HealthStatus{
			IsHealthy: false,
			Message:   "Strategy is not running",
			LastCheck: time.Now(),
		}
	}

	// Check if we have recent price data from exchanges
	validExchanges := 0
	for _, price := range s.exchangePrices {
		if price.IsValid && time.Since(price.Timestamp) < s.maxArbAge {
			validExchanges++
		}
	}

	if validExchanges == 0 {
		return common.HealthStatus{
			IsHealthy: false,
			Message:   "No valid exchange price data",
			LastCheck: time.Now(),
		}
	}

	return common.HealthStatus{
		IsHealthy: true,
		Message:   fmt.Sprintf("Strategy is healthy with %d valid exchanges", validExchanges),
		LastCheck: time.Now(),
	}
}

// Helper methods

func (s *CrossExchangeArbitrageStrategy) updateLocalPrice(price, volatility float64) {
	// Update local exchange (current exchange) price
	s.exchangePrices["local"] = ExchangePrice{
		Price:     price,
		Volume:    0.0, // Unknown for local
		Timestamp: time.Now(),
		Spread:    volatility, // Use volatility as proxy for spread
		IsValid:   true,
	}
}

func (s *CrossExchangeArbitrageStrategy) updateExchangePrice(exchange string, price, volume, spread float64) {
	s.exchangePrices[exchange] = ExchangePrice{
		Price:     price,
		Volume:    volume,
		Timestamp: time.Now(),
		Spread:    spread,
		IsValid:   price > 0,
	}
}

func (s *CrossExchangeArbitrageStrategy) calculateFairValue() {
	weightedSum := 0.0
	totalWeight := 0.0
	now := time.Now()

	for exchange, price := range s.exchangePrices {
		if !price.IsValid || now.Sub(price.Timestamp) > s.maxArbAge {
			continue
		}

		weight := s.exchangeWeights[exchange]
		if weight == 0 {
			weight = 0.1 // Default weight for unknown exchanges
		}

		// Adjust weight based on data freshness
		ageSeconds := now.Sub(price.Timestamp).Seconds()
		freshnessWeight := math.Exp(-ageSeconds / 10.0) // Decay over 10 seconds

		finalWeight := weight * freshnessWeight
		weightedSum += price.Price * finalWeight
		totalWeight += finalWeight
	}

	if totalWeight > 0 {
		s.fairValue = weightedSum / totalWeight
	} else {
		s.fairValue = 0.0
	}
}

func (s *CrossExchangeArbitrageStrategy) detectArbitrageOpportunity(localPrice float64) {
	if s.fairValue == 0 || localPrice == 0 {
		s.arbOpportunity = 0.0
		return
	}

	// Calculate arbitrage opportunity
	opportunity := (s.fairValue - localPrice) / localPrice

	// Apply threshold filter
	if math.Abs(opportunity) >= s.arbThreshold {
		s.arbOpportunity = opportunity
	} else {
		s.arbOpportunity = 0.0
	}

	s.lastArbUpdate = time.Now()

	// Record significant opportunities
	if math.Abs(opportunity) >= s.arbThreshold {
		arbEvent := ArbitrageEvent{
			Opportunity: opportunity,
			FairValue:   s.fairValue,
			LocalPrice:  localPrice,
			Profit:      0.0, // Will be updated on execution
			Timestamp:   time.Now(),
			Executed:    false,
		}

		s.addArbitrageEvent(arbEvent)
	}
}

func (s *CrossExchangeArbitrageStrategy) addArbitrageEvent(event ArbitrageEvent) {
	s.arbTrades = append(s.arbTrades, event)
	if len(s.arbTrades) > s.maxArbTrades {
		s.arbTrades = s.arbTrades[1:]
	}
}

func (s *CrossExchangeArbitrageStrategy) calculateConfidence() float64 {
	// Confidence based on data quality and arbitrage strength
	validExchanges := 0
	totalWeight := 0.0
	now := time.Now()

	for exchange, price := range s.exchangePrices {
		if price.IsValid && now.Sub(price.Timestamp) < s.maxArbAge {
			validExchanges++
			totalWeight += s.exchangeWeights[exchange]
		}
	}

	// Data quality component
	dataQuality := float64(validExchanges) / float64(len(s.exchangeWeights))
	weightCoverage := math.Min(1.0, totalWeight)

	// Arbitrage signal strength
	signalStrength := math.Min(1.0, math.Abs(s.arbOpportunity)/s.arbThreshold)

	return math.Min(1.0, dataQuality*weightCoverage*(0.5+0.5*signalStrength))
}
