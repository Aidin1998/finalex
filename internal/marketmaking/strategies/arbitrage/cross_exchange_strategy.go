package arbitrage

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Aidin1998/finalex/internal/marketmaking/strategies/common"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// CrossExchangeStrategy implements cross-exchange arbitrage market making
type CrossExchangeStrategy struct {
	config      common.StrategyConfig
	name        string
	version     string
	status      common.StrategyStatus
	metrics     *common.StrategyMetrics
	mu          sync.RWMutex
	baseSpread  decimal.Decimal
	maxPosition decimal.Decimal

	// Cross-exchange specific fields
	exchangeWeights  map[string]decimal.Decimal
	priceHistory     map[string][]decimal.Decimal
	lastUpdateTimes  map[string]time.Time
	arbThreshold     decimal.Decimal
	minProfitBps     decimal.Decimal
	arbOpportunities []ArbitrageOpportunity
	maxOpportunities int
	stopChan         chan struct{}
}

// ArbitrageOpportunity represents a detected arbitrage opportunity
type ArbitrageOpportunity struct {
	LocalPrice    decimal.Decimal
	ExternalPrice decimal.Decimal
	Exchange      string
	Profit        decimal.Decimal
	Size          decimal.Decimal
	Timestamp     time.Time
	Executed      bool
}

// NewCrossExchangeStrategy creates a new cross-exchange arbitrage strategy
func NewCrossExchangeStrategy(config common.StrategyConfig) (common.MarketMakingStrategy, error) {
	// Default values
	baseSpread := decimal.NewFromFloat(0.002)
	maxPosition := decimal.NewFromFloat(1000.0)
	arbThreshold := decimal.NewFromFloat(0.001)
	minProfitBps := decimal.NewFromFloat(5.0)

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
	if val, ok := config.Parameters["arb_threshold"]; ok {
		if f, ok := val.(float64); ok {
			arbThreshold = decimal.NewFromFloat(f)
		}
	}
	if val, ok := config.Parameters["min_profit_bps"]; ok {
		if f, ok := val.(float64); ok {
			minProfitBps = decimal.NewFromFloat(f)
		}
	}

	strategy := &CrossExchangeStrategy{
		config:           config,
		name:             "CrossExchange",
		version:          "1.0.0",
		status:           common.StatusStopped,
		baseSpread:       baseSpread,
		maxPosition:      maxPosition,
		arbThreshold:     arbThreshold,
		minProfitBps:     minProfitBps,
		maxOpportunities: 50,
		exchangeWeights:  make(map[string]decimal.Decimal),
		priceHistory:     make(map[string][]decimal.Decimal),
		lastUpdateTimes:  make(map[string]time.Time),
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

	// Initialize default exchange weights
	strategy.exchangeWeights["binance"] = decimal.NewFromFloat(0.4)
	strategy.exchangeWeights["coinbase"] = decimal.NewFromFloat(0.3)
	strategy.exchangeWeights["kraken"] = decimal.NewFromFloat(0.3)

	return strategy, nil
}

// Initialize prepares the strategy for trading
func (s *CrossExchangeStrategy) Initialize(ctx context.Context, config common.StrategyConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.config = config
	s.status = common.StatusStopped
	return nil
}

// Start begins strategy execution
func (s *CrossExchangeStrategy) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.status != common.StatusStopped {
		return fmt.Errorf("strategy is not in stopped state")
	}

	s.status = common.StatusRunning
	return nil
}

// Stop halts strategy execution
func (s *CrossExchangeStrategy) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.status == common.StatusStopped {
		return nil
	}

	s.status = common.StatusStopped
	close(s.stopChan)
	return nil
}

// Quote generates bid/ask quotes based on cross-exchange arbitrage opportunities
func (s *CrossExchangeStrategy) Quote(ctx context.Context, input common.QuoteInput) (*common.QuoteOutput, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.status != common.StatusRunning {
		return nil, fmt.Errorf("strategy is not running")
	}

	// Update local price and detect arbitrage opportunities
	s.updateLocalPrice(input.MidPrice)
	s.detectArbitrageOpportunity(input.MidPrice)

	// Calculate inventory skew
	inventorySkew := input.Inventory.Mul(decimal.NewFromFloat(0.001))

	// Calculate cross-exchange weighted price
	weightedPrice := s.calculateWeightedPrice()

	// Calculate arbitrage-adjusted spread
	arbSpread := s.calculateArbitrageSpread(input.MidPrice, weightedPrice)
	totalSpread := s.baseSpread.Add(arbSpread)

	// Apply inventory skew
	bidSpread := totalSpread.Sub(inventorySkew)
	askSpread := totalSpread.Add(inventorySkew)

	// Calculate final prices
	one := decimal.NewFromFloat(1.0)
	bid := input.MidPrice.Mul(one.Sub(bidSpread))
	ask := input.MidPrice.Mul(one.Add(askSpread))

	// Calculate dynamic sizing based on arbitrage opportunities and inventory
	size := s.calculateOptimalSize(input)

	// Calculate confidence based on arbitrage signal strength
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
func (s *CrossExchangeStrategy) OnMarketData(ctx context.Context, data *common.MarketData) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Update price history for cross-exchange analysis
	s.updatePriceHistory("local", data.Price)
	s.metrics.LastUpdated = time.Now()
	return nil
}

// OnOrderFill handles order fill events
func (s *CrossExchangeStrategy) OnOrderFill(ctx context.Context, fill *common.OrderFill) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.metrics.OrdersFilled++

	// Create arbitrage opportunity record if this was an arbitrage trade
	if len(s.arbOpportunities) > 0 {
		// Mark the most recent opportunity as executed
		for i := len(s.arbOpportunities) - 1; i >= 0; i-- {
			if !s.arbOpportunities[i].Executed {
				s.arbOpportunities[i].Executed = true
				s.arbOpportunities[i].LocalPrice = fill.Price
				break
			}
		}
	}

	return nil
}

// OnOrderCancel handles order cancellation events
func (s *CrossExchangeStrategy) OnOrderCancel(ctx context.Context, orderID uuid.UUID, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.metrics.OrdersCancelled++
	return nil
}

// UpdateConfig updates strategy configuration
func (s *CrossExchangeStrategy) UpdateConfig(ctx context.Context, config common.StrategyConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.config = config
	return nil
}

// GetConfig returns current configuration
func (s *CrossExchangeStrategy) GetConfig() common.StrategyConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config
}

// GetMetrics returns current metrics
func (s *CrossExchangeStrategy) GetMetrics() *common.StrategyMetrics {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.metrics
}

// GetStatus returns current status
func (s *CrossExchangeStrategy) GetStatus() common.StrategyStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.status
}

// Name returns strategy name
func (s *CrossExchangeStrategy) Name() string {
	return s.name
}

// Version returns strategy version
func (s *CrossExchangeStrategy) Version() string {
	return s.version
}

// Description returns strategy description
func (s *CrossExchangeStrategy) Description() string {
	return "Cross-exchange arbitrage market making strategy that exploits price differences across multiple exchanges"
}

// RiskLevel returns the risk level
func (s *CrossExchangeStrategy) RiskLevel() common.RiskLevel {
	return common.RiskHigh
}

// HealthCheck performs health check
func (s *CrossExchangeStrategy) HealthCheck(ctx context.Context) *common.HealthStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	isHealthy := s.status == common.StatusRunning && len(s.priceHistory) > 0

	checks := map[string]bool{
		"status":     s.status == common.StatusRunning,
		"price_data": len(s.priceHistory) > 0,
		"exchanges":  len(s.exchangeWeights) > 0,
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
func (s *CrossExchangeStrategy) Reset(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Clear price history
	for exchange := range s.priceHistory {
		s.priceHistory[exchange] = s.priceHistory[exchange][:0]
	}

	// Clear arbitrage opportunities
	s.arbOpportunities = s.arbOpportunities[:0]

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

func (s *CrossExchangeStrategy) updateLocalPrice(price decimal.Decimal) {
	s.updatePriceHistory("local", price)
}

func (s *CrossExchangeStrategy) updatePriceHistory(exchange string, price decimal.Decimal) {
	if _, exists := s.priceHistory[exchange]; !exists {
		s.priceHistory[exchange] = make([]decimal.Decimal, 0, 100)
	}

	s.priceHistory[exchange] = append(s.priceHistory[exchange], price)
	if len(s.priceHistory[exchange]) > 100 {
		s.priceHistory[exchange] = s.priceHistory[exchange][1:]
	}

	s.lastUpdateTimes[exchange] = time.Now()
}

func (s *CrossExchangeStrategy) detectArbitrageOpportunity(localPrice decimal.Decimal) {
	// Simple arbitrage detection (in real implementation, would use actual external exchange data)
	for exchange, weight := range s.exchangeWeights {
		if exchange == "local" {
			continue
		}

		// Simulate external price with some variance
		variance := decimal.NewFromFloat(0.002) // 0.2% variance
		externalPrice := localPrice.Mul(decimal.NewFromFloat(1.0).Add(variance.Mul(weight)))

		// Calculate potential profit
		priceDiff := externalPrice.Sub(localPrice).Abs()
		profitBps := priceDiff.Div(localPrice).Mul(decimal.NewFromFloat(10000))

		// Check if this meets our arbitrage threshold
		if profitBps.GreaterThan(s.minProfitBps) {
			opportunity := ArbitrageOpportunity{
				LocalPrice:    localPrice,
				ExternalPrice: externalPrice,
				Exchange:      exchange,
				Profit:        profitBps,
				Size:          s.maxPosition.Mul(decimal.NewFromFloat(0.1)),
				Timestamp:     time.Now(),
				Executed:      false,
			}

			s.arbOpportunities = append(s.arbOpportunities, opportunity)
			if len(s.arbOpportunities) > s.maxOpportunities {
				s.arbOpportunities = s.arbOpportunities[1:]
			}
		}
	}
}

func (s *CrossExchangeStrategy) calculateWeightedPrice() decimal.Decimal {
	totalWeight := decimal.Zero
	weightedSum := decimal.Zero

	for exchange, weight := range s.exchangeWeights {
		if prices, exists := s.priceHistory[exchange]; exists && len(prices) > 0 {
			lastPrice := prices[len(prices)-1]
			weightedSum = weightedSum.Add(lastPrice.Mul(weight))
			totalWeight = totalWeight.Add(weight)
		}
	}

	if totalWeight.IsZero() {
		return decimal.Zero
	}

	return weightedSum.Div(totalWeight)
}

func (s *CrossExchangeStrategy) calculateArbitrageSpread(localPrice, weightedPrice decimal.Decimal) decimal.Decimal {
	if weightedPrice.IsZero() || localPrice.IsZero() {
		return decimal.Zero
	}

	// Calculate spread adjustment based on arbitrage signal
	priceDiff := weightedPrice.Sub(localPrice).Abs()
	spreadAdjustment := priceDiff.Div(localPrice).Mul(decimal.NewFromFloat(0.5))

	return spreadAdjustment
}

func (s *CrossExchangeStrategy) calculateOptimalSize(input common.QuoteInput) decimal.Decimal {
	// Base size calculation
	baseSize := s.maxPosition.Mul(decimal.NewFromFloat(0.1))

	// Adjust based on inventory
	inventoryUsage := input.Inventory.Abs().Div(s.maxPosition)
	inventoryCapacity := decimal.NewFromFloat(1.0).Sub(inventoryUsage)

	// Adjust based on arbitrage opportunities
	arbFactor := decimal.NewFromFloat(1.0)
	if len(s.arbOpportunities) > 0 {
		// Increase size when arbitrage opportunities are present
		arbFactor = decimal.NewFromFloat(1.0).Add(decimal.NewFromFloat(0.2))
	}

	size := baseSize.Mul(inventoryCapacity).Mul(arbFactor)

	// Ensure minimum size
	minSize := decimal.NewFromFloat(0.1)
	if size.LessThan(minSize) {
		size = minSize
	}

	return size
}

func (s *CrossExchangeStrategy) calculateConfidence() float64 {
	// Base confidence
	baseConfidence := 0.6

	// Adjust based on arbitrage opportunities
	if len(s.arbOpportunities) > 0 {
		baseConfidence += 0.3
	}

	// Adjust based on price data freshness
	freshDataCount := 0
	for exchange, lastUpdate := range s.lastUpdateTimes {
		if time.Since(lastUpdate) < time.Minute && exchange != "local" {
			freshDataCount++
		}
	}

	if freshDataCount > 1 {
		baseConfidence += 0.1
	}

	if baseConfidence > 1.0 {
		baseConfidence = 1.0
	}

	return baseConfidence
}
