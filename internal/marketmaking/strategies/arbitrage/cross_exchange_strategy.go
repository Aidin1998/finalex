package arbitrage

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/Aidin1998/finalex/internal/marketmaking/strategies/common"
)

// CrossExchangeStrategy monitors price differences across exchanges for arbitrage opportunities
type CrossExchangeStrategy struct {
	config          *CrossExchangeConfig
	metrics         *common.StrategyMetrics
	status          common.StrategyStatus
	exchangePrices  map[string]float64
	exchangeWeights map[string]float64
	priceHistory    map[string][]float64
	lastUpdateTimes map[string]time.Time
	mu              sync.RWMutex
	stopChan        chan struct{}
}

type CrossExchangeConfig struct {
	BaseSpread      float64            `json:"base_spread"`
	MaxInventory    float64            `json:"max_inventory"`
	ArbThreshold    float64            `json:"arb_threshold"`
	MinProfitBps    float64            `json:"min_profit_bps"`
	MaxLatency      time.Duration      `json:"max_latency"`
	ExchangeWeights map[string]float64 `json:"exchange_weights"`
	PriceStaleTime  time.Duration      `json:"price_stale_time"`
	TickSize        float64            `json:"tick_size"`
}

// NewCrossExchangeStrategy creates a new cross-exchange arbitrage strategy
func NewCrossExchangeStrategy() *CrossExchangeStrategy {
	return &CrossExchangeStrategy{
		config: &CrossExchangeConfig{
			BaseSpread:     0.001,
			MaxInventory:   1000.0,
			ArbThreshold:   0.0005, // 5 bps minimum arbitrage
			MinProfitBps:   10.0,   // 10 bps minimum profit
			MaxLatency:     100 * time.Millisecond,
			PriceStaleTime: 5 * time.Second,
			TickSize:       0.01,
			ExchangeWeights: map[string]float64{
				"binance":  0.4,
				"coinbase": 0.3,
				"kraken":   0.2,
				"okx":      0.1,
			},
		},
		metrics: &common.StrategyMetrics{
			StartTime: time.Now(),
		},
		status:          common.StatusStopped,
		exchangePrices:  make(map[string]float64),
		exchangeWeights: make(map[string]float64),
		priceHistory:    make(map[string][]float64),
		lastUpdateTimes: make(map[string]time.Time),
		stopChan:        make(chan struct{}),
	}
}

// Initialize prepares the strategy for trading
func (s *CrossExchangeStrategy) Initialize(ctx context.Context, config common.StrategyConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Parse cross-exchange strategy config
	if baseSpread, ok := config.Parameters["base_spread"].(float64); ok {
		s.config.BaseSpread = baseSpread
	}
	if maxInventory, ok := config.Parameters["max_inventory"].(float64); ok {
		s.config.MaxInventory = maxInventory
	}
	if arbThreshold, ok := config.Parameters["arb_threshold"].(float64); ok {
		s.config.ArbThreshold = arbThreshold
	}
	if minProfitBps, ok := config.Parameters["min_profit_bps"].(float64); ok {
		s.config.MinProfitBps = minProfitBps
	}

	// Initialize exchange weights from config
	for exchange, weight := range s.config.ExchangeWeights {
		s.exchangeWeights[exchange] = weight
		s.priceHistory[exchange] = make([]float64, 0, 100)
	}

	s.status = common.StatusInitialized
	return nil
}

// Start begins strategy execution
func (s *CrossExchangeStrategy) Start(ctx context.Context) error {
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
func (s *CrossExchangeStrategy) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.status != common.StatusRunning {
		return fmt.Errorf("cannot stop strategy in status: %s", s.status)
	}

	close(s.stopChan)
	s.status = common.StatusStopped
	return nil
}

// Quote generates arbitrage-aware bid/ask quotes
func (s *CrossExchangeStrategy) Quote(input common.QuoteInput) (common.QuoteOutput, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.status != common.StatusRunning {
		return common.QuoteOutput{}, fmt.Errorf("strategy not running")
	}

	// Calculate cross-exchange fair value
	fairValue := s.calculateCrossExchangeFairValue()
	if fairValue == 0 {
		fairValue = input.MidPrice // Fallback to current mid
	}

	// Detect arbitrage opportunities
	arbOpportunity := (fairValue - input.MidPrice) / input.MidPrice
	arbSignal := math.Max(-s.config.ArbThreshold, math.Min(s.config.ArbThreshold, arbOpportunity))

	// Calculate arbitrage strength and confidence
	arbStrength := math.Abs(arbSignal) / s.config.ArbThreshold
	confidence := s.calculateArbConfidence(arbStrength)

	// Adjust spread based on arbitrage signal
	spreadAdjustment := arbStrength * 0.5 // Tighten spread when arbitrage detected
	adjustedSpread := s.config.BaseSpread * (1.0 - spreadAdjustment)
	adjustedSpread = math.Max(adjustedSpread, s.config.BaseSpread*0.2) // Minimum spread

	// Skew quotes toward arbitrage direction
	arbSkew := arbSignal * confidence * 0.8
	inventorySkew := input.Inventory * 0.001
	totalSkew := arbSkew + inventorySkew

	// Calculate asymmetric spreads
	bidSpread := adjustedSpread/2 + math.Max(0, totalSkew)
	askSpread := adjustedSpread/2 + math.Max(0, -totalSkew)

	bid := input.MidPrice * (1 - bidSpread)
	ask := input.MidPrice * (1 + askSpread)

	// Round to tick size
	if s.config.TickSize > 0 {
		bid = s.roundToTick(bid, false)
		ask = s.roundToTick(ask, true)
	}

	// Size based on arbitrage strength and inventory capacity
	baseSize := s.config.MaxInventory * 0.1
	inventoryCapacity := math.Max(0.1, 1.0-math.Abs(input.Inventory)/s.config.MaxInventory)
	arbSizeMultiplier := 1.0 + arbStrength*2.0 // Increase size for strong arbitrage
	adjustedSize := baseSize * inventoryCapacity * arbSizeMultiplier

	// Asymmetric sizing based on arbitrage direction
	bidSize := adjustedSize
	askSize := adjustedSize

	if arbSignal > 0 && confidence > 0.7 {
		// Fair value higher than current - favor buying
		bidSize *= 1.5
		askSize *= 0.7
	} else if arbSignal < 0 && confidence > 0.7 {
		// Fair value lower than current - favor selling
		askSize *= 1.5
		bidSize *= 0.7
	}

	output := common.QuoteOutput{
		BidPrice:   bid,
		AskPrice:   ask,
		BidSize:    bidSize,
		AskSize:    askSize,
		Confidence: confidence,
		Timestamp:  time.Now(),
	}

	// Update metrics
	s.updateMetrics(output, arbOpportunity, arbStrength)

	return output, nil
}

// OnMarketData handles market data updates
func (s *CrossExchangeStrategy) OnMarketData(data common.MarketData) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Update exchange price data if available
	if data.Exchange != "" && data.Price > 0 {
		s.updateExchangePrice(data.Exchange, data.Price)
	}

	return nil
}

// OnOrderFill handles order fill notifications
func (s *CrossExchangeStrategy) OnOrderFill(fill common.OrderFill) error {
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

	// Calculate PnL (enhanced for arbitrage tracking)
	if fill.Side == "buy" {
		s.metrics.TotalPnL -= fill.Price * fill.Quantity
	} else {
		s.metrics.TotalPnL += fill.Price * fill.Quantity
	}

	// Track arbitrage performance
	s.metrics.ArbitrageOpportunities++

	return nil
}

// OnOrderCancel handles order cancellation notifications
func (s *CrossExchangeStrategy) OnOrderCancel(cancel common.OrderCancel) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.metrics.OrdersCancelled++
	return nil
}

// UpdateConfig updates strategy configuration
func (s *CrossExchangeStrategy) UpdateConfig(config common.StrategyConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if baseSpread, ok := config.Parameters["base_spread"].(float64); ok {
		s.config.BaseSpread = baseSpread
	}
	if arbThreshold, ok := config.Parameters["arb_threshold"].(float64); ok {
		s.config.ArbThreshold = arbThreshold
	}
	if minProfitBps, ok := config.Parameters["min_profit_bps"].(float64); ok {
		s.config.MinProfitBps = minProfitBps
	}

	return nil
}

// GetMetrics returns current strategy metrics
func (s *CrossExchangeStrategy) GetMetrics() common.StrategyMetrics {
	s.mu.RLock()
	defer s.mu.RUnlock()

	metrics := *s.metrics
	metrics.Uptime = time.Since(s.metrics.StartTime)

	if s.metrics.OrdersPlaced > 0 {
		metrics.SuccessRate = float64(s.metrics.OrdersPlaced-s.metrics.OrdersCancelled) / float64(s.metrics.OrdersPlaced)
	}

	// Add arbitrage-specific metrics
	metrics.ActiveExchanges = len(s.getActiveExchanges())

	return metrics
}

// GetStatus returns current strategy status
func (s *CrossExchangeStrategy) GetStatus() common.StrategyStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.status
}

// GetInfo returns strategy information
func (s *CrossExchangeStrategy) GetInfo() common.StrategyInfo {
	return common.StrategyInfo{
		Name:        "CrossExchange",
		Version:     "1.0.0",
		Description: "Cross-exchange arbitrage market making strategy",
		Author:      "Finalex Team",
		RiskLevel:   common.RiskMedium,
		Complexity:  common.ComplexityAdvanced,
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
				Name:        "max_inventory",
				Type:        "float",
				Description: "Maximum inventory position",
				Default:     1000.0,
				MinValue:    100.0,
				MaxValue:    100000.0,
				Required:    true,
			},
			{
				Name:        "arb_threshold",
				Type:        "float",
				Description: "Minimum arbitrage opportunity threshold",
				Default:     0.0005,
				MinValue:    0.0001,
				MaxValue:    0.01,
				Required:    true,
			},
			{
				Name:        "min_profit_bps",
				Type:        "float",
				Description: "Minimum profit in basis points",
				Default:     10.0,
				MinValue:    1.0,
				MaxValue:    100.0,
				Required:    false,
			},
		},
	}
}

// HealthCheck performs health validation
func (s *CrossExchangeStrategy) HealthCheck() common.HealthStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	health := common.HealthStatus{
		IsHealthy: true,
		Timestamp: time.Now(),
	}

	// Check exchange connectivity
	activeExchanges := s.getActiveExchanges()
	if len(activeExchanges) < 2 {
		health.IsHealthy = false
		health.Issues = append(health.Issues, "Insufficient exchange connections for arbitrage")
	}

	// Check price staleness
	now := time.Now()
	for exchange, lastUpdate := range s.lastUpdateTimes {
		if now.Sub(lastUpdate) > s.config.PriceStaleTime {
			health.Issues = append(health.Issues, fmt.Sprintf("Stale price data from %s", exchange))
		}
	}

	// Check configuration
	if s.config.ArbThreshold <= 0 {
		health.IsHealthy = false
		health.Issues = append(health.Issues, "Invalid arbitrage threshold")
	}

	return health
}

// UpdateExchangePrice updates price data for a specific exchange
func (s *CrossExchangeStrategy) UpdateExchangePrice(exchange string, price float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updateExchangePrice(exchange, price)
}

// GetArbitrageOpportunities returns current arbitrage opportunities
func (s *CrossExchangeStrategy) GetArbitrageOpportunities() map[string]float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	opportunities := make(map[string]float64)
	fairValue := s.calculateCrossExchangeFairValue()

	if fairValue > 0 {
		for exchange, price := range s.exchangePrices {
			if price > 0 {
				opportunity := (price - fairValue) / fairValue
				if math.Abs(opportunity) >= s.config.ArbThreshold {
					opportunities[exchange] = opportunity
				}
			}
		}
	}

	return opportunities
}

// Helper methods

func (s *CrossExchangeStrategy) updateExchangePrice(exchange string, price float64) {
	s.exchangePrices[exchange] = price
	s.lastUpdateTimes[exchange] = time.Now()

	// Update price history
	if history, exists := s.priceHistory[exchange]; exists {
		history = append(history, price)
		if len(history) > 100 {
			history = history[1:]
		}
		s.priceHistory[exchange] = history
	} else {
		s.priceHistory[exchange] = []float64{price}
	}
}

func (s *CrossExchangeStrategy) calculateCrossExchangeFairValue() float64 {
	if len(s.exchangePrices) == 0 {
		return 0.0
	}

	weightedSum := 0.0
	totalWeight := 0.0
	now := time.Now()

	for exchange, price := range s.exchangePrices {
		if price <= 0 {
			continue
		}

		// Check if price is stale
		if lastUpdate, exists := s.lastUpdateTimes[exchange]; exists {
			if now.Sub(lastUpdate) > s.config.PriceStaleTime {
				continue // Skip stale prices
			}
		}

		weight := s.exchangeWeights[exchange]
		if weight <= 0 {
			weight = 1.0 // Default weight
		}

		weightedSum += price * weight
		totalWeight += weight
	}

	if totalWeight > 0 {
		return weightedSum / totalWeight
	}

	return 0.0
}

func (s *CrossExchangeStrategy) calculateArbConfidence(arbStrength float64) float64 {
	// Base confidence from arbitrage strength
	strengthConfidence := math.Min(1.0, arbStrength*2.0)

	// Adjust based on data quality
	activeExchanges := len(s.getActiveExchanges())
	dataQuality := math.Min(1.0, float64(activeExchanges)/4.0) // Assume 4 exchanges is optimal

	// Combine factors
	confidence := strengthConfidence * dataQuality
	return math.Max(0.1, math.Min(1.0, confidence))
}

func (s *CrossExchangeStrategy) getActiveExchanges() []string {
	var active []string
	now := time.Now()

	for exchange, price := range s.exchangePrices {
		if price <= 0 {
			continue
		}

		if lastUpdate, exists := s.lastUpdateTimes[exchange]; exists {
			if now.Sub(lastUpdate) <= s.config.PriceStaleTime {
				active = append(active, exchange)
			}
		}
	}

	return active
}

func (s *CrossExchangeStrategy) roundToTick(price float64, roundUp bool) float64 {
	if s.config.TickSize <= 0 {
		return price
	}

	if roundUp {
		return math.Ceil(price/s.config.TickSize) * s.config.TickSize
	} else {
		return math.Floor(price/s.config.TickSize) * s.config.TickSize
	}
}

func (s *CrossExchangeStrategy) updateMetrics(output common.QuoteOutput, arbOpportunity, arbStrength float64) {
	s.metrics.LastUpdate = time.Now()
	s.metrics.QuotesGenerated++

	if output.BidPrice > 0 && output.AskPrice > 0 {
		spread := (output.AskPrice - output.BidPrice) / output.BidPrice
		s.metrics.AvgSpread = (s.metrics.AvgSpread*float64(s.metrics.QuotesGenerated-1) + spread) / float64(s.metrics.QuotesGenerated)
	}

	// Track arbitrage metrics
	if math.Abs(arbOpportunity) >= s.config.ArbThreshold {
		s.metrics.ArbitrageOpportunities++
	}
}
