package test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// MarketMakerInterface defines the interface for market maker operations
type MarketMakerInterface interface {
	Start(ctx context.Context) error
	Stop() error
	UpdateConfig(cfg MarketMakerConfig) error
	GetQuotes(ctx context.Context, tradingPairID string) (Quote, error)
	GetPerformanceMetrics(ctx context.Context) (MarketMakerMetrics, error)
	GetInventory(ctx context.Context, asset string) (decimal.Decimal, error)
}

// MarketMakerConfig represents market maker configuration
type MarketMakerConfig struct {
	TradingPairs     []string        `json:"trading_pairs"`
	MinSpread        decimal.Decimal `json:"min_spread"`
	TargetSpread     decimal.Decimal `json:"target_spread"`
	MaxSpread        decimal.Decimal `json:"max_spread"`
	QuoteSize        decimal.Decimal `json:"quote_size"`
	MaxInventory     decimal.Decimal `json:"max_inventory"`
	Strategy         string          `json:"strategy"` // "basic", "dynamic", "inventory_skew"
	UpdateInterval   time.Duration   `json:"update_interval"`
	RiskEnabled      bool            `json:"risk_enabled"`
	MaxLoss          decimal.Decimal `json:"max_loss"`
	VolatilityFactor decimal.Decimal `json:"volatility_factor"`
}

// Quote represents a market maker quote
type Quote struct {
	TradingPairID string          `json:"trading_pair_id"`
	BidPrice      decimal.Decimal `json:"bid_price"`
	AskPrice      decimal.Decimal `json:"ask_price"`
	BidSize       decimal.Decimal `json:"bid_size"`
	AskSize       decimal.Decimal `json:"ask_size"`
	Spread        decimal.Decimal `json:"spread"`
	MidPrice      decimal.Decimal `json:"mid_price"`
	Timestamp     time.Time       `json:"timestamp"`
}

// MarketMakerMetrics represents performance metrics for market maker
type MarketMakerMetrics struct {
	TotalQuotes       int64           `json:"total_quotes"`
	ActiveQuotes      int64           `json:"active_quotes"`
	FilledOrders      int64           `json:"filled_orders"`
	ProfitLoss        decimal.Decimal `json:"profit_loss"`
	AverageSpread     decimal.Decimal `json:"average_spread"`
	VolumeProvided    decimal.Decimal `json:"volume_provided"`
	Uptime            time.Duration   `json:"uptime"`
	AverageLatency    time.Duration   `json:"average_latency"`
	InventoryRisk     decimal.Decimal `json:"inventory_risk"`
	CompetitiveQuotes int64           `json:"competitive_quotes"`
	RiskEvents        int64           `json:"risk_events"`
}

// MockMarketMaker implements MarketMakerInterface for testing
type MockMarketMaker struct {
	mu             sync.RWMutex
	config         MarketMakerConfig
	quotes         map[string]Quote
	metrics        MarketMakerMetrics
	inventory      map[string]decimal.Decimal
	isRunning      bool
	tradingService TradingServiceInterface
	latencyRange   time.Duration
	errorRate      float64
	startTime      time.Time
}

// NewMockMarketMaker creates a new mock market maker
func NewMockMarketMaker(tradingService TradingServiceInterface) *MockMarketMaker {
	return &MockMarketMaker{
		quotes:         make(map[string]Quote),
		inventory:      make(map[string]decimal.Decimal),
		tradingService: tradingService,
		latencyRange:   5 * time.Millisecond,
		errorRate:      0.01, // 1% error rate
	}
}

// Start implements MarketMakerInterface
func (mmm *MockMarketMaker) Start(ctx context.Context) error {
	mmm.mu.Lock()
	defer mmm.mu.Unlock()

	mmm.isRunning = true
	mmm.startTime = time.Now()

	// Start quote generation goroutine
	go mmm.generateQuotes(ctx)

	return nil
}

// Stop implements MarketMakerInterface
func (mmm *MockMarketMaker) Stop() error {
	mmm.mu.Lock()
	defer mmm.mu.Unlock()

	mmm.isRunning = false
	return nil
}

// UpdateConfig implements MarketMakerInterface
func (mmm *MockMarketMaker) UpdateConfig(cfg MarketMakerConfig) error {
	mmm.mu.Lock()
	defer mmm.mu.Unlock()

	mmm.config = cfg
	return nil
}

// GetQuotes implements MarketMakerInterface
func (mmm *MockMarketMaker) GetQuotes(ctx context.Context, tradingPairID string) (Quote, error) {
	// Simulate latency
	time.Sleep(mmm.latencyRange)

	mmm.mu.RLock()
	defer mmm.mu.RUnlock()

	if quote, exists := mmm.quotes[tradingPairID]; exists {
		return quote, nil
	}

	// Generate new quote if not exists
	quote := mmm.generateQuote(tradingPairID)
	return quote, nil
}

// GetPerformanceMetrics implements MarketMakerInterface
func (mmm *MockMarketMaker) GetPerformanceMetrics(ctx context.Context) (MarketMakerMetrics, error) {
	mmm.mu.RLock()
	defer mmm.mu.RUnlock()

	if mmm.isRunning {
		mmm.metrics.Uptime = time.Since(mmm.startTime)
	}

	return mmm.metrics, nil
}

// GetInventory implements MarketMakerInterface
func (mmm *MockMarketMaker) GetInventory(ctx context.Context, asset string) (decimal.Decimal, error) {
	mmm.mu.RLock()
	defer mmm.mu.RUnlock()

	if inventory, exists := mmm.inventory[asset]; exists {
		return inventory, nil
	}

	return decimal.Zero, nil
}

// generateQuotes runs continuously to generate quotes
func (mmm *MockMarketMaker) generateQuotes(ctx context.Context) {
	ticker := time.NewTicker(mmm.config.UpdateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			mmm.mu.Lock()
			if !mmm.isRunning {
				mmm.mu.Unlock()
				return
			}

			for _, pairID := range mmm.config.TradingPairs {
				quote := mmm.generateQuote(pairID)
				mmm.quotes[pairID] = quote
				mmm.metrics.TotalQuotes++
			}
			mmm.mu.Unlock()
		}
	}
}

// generateQuote creates a realistic quote for a trading pair
func (mmm *MockMarketMaker) generateQuote(tradingPairID string) Quote {
	basePrice := decimal.NewFromFloat(100.0) // Base price for testing
	spread := mmm.config.TargetSpread

	// Apply strategy-specific logic
	switch mmm.config.Strategy {
	case "dynamic":
		// Simulate dynamic spread based on volatility
		volatility := decimal.NewFromFloat(0.001 + 0.002*0.5) // Mock volatility
		spread = spread.Add(volatility.Mul(mmm.config.VolatilityFactor))
	case "inventory_skew":
		// Simulate inventory skew
		if inv, exists := mmm.inventory[tradingPairID]; exists {
			skew := inv.Div(mmm.config.MaxInventory).Mul(mmm.config.TargetSpread)
			spread = spread.Add(skew)
		}
	}

	halfSpread := spread.Div(decimal.NewFromInt(2))
	bidPrice := basePrice.Sub(halfSpread)
	askPrice := basePrice.Add(halfSpread)

	return Quote{
		TradingPairID: tradingPairID,
		BidPrice:      bidPrice,
		AskPrice:      askPrice,
		BidSize:       mmm.config.QuoteSize,
		AskSize:       mmm.config.QuoteSize,
		Spread:        spread,
		MidPrice:      basePrice,
		Timestamp:     time.Now(),
	}
}

// Test Market Maker Strategy Performance
func TestMarketMakerStrategyPerformance(t *testing.T) {
	env := NewTestEnvironment(DefaultTestConfig())
	marketMaker := NewMockMarketMaker(env.TradingService)

	config := MarketMakerConfig{
		TradingPairs:     []string{"BTC-USD", "ETH-USD"},
		MinSpread:        decimal.NewFromFloat(0.001),
		TargetSpread:     decimal.NewFromFloat(0.002),
		MaxSpread:        decimal.NewFromFloat(0.005),
		QuoteSize:        decimal.NewFromFloat(1.0),
		MaxInventory:     decimal.NewFromFloat(10.0),
		Strategy:         "basic",
		UpdateInterval:   100 * time.Millisecond,
		RiskEnabled:      true,
		MaxLoss:          decimal.NewFromFloat(1000.0),
		VolatilityFactor: decimal.NewFromFloat(0.5),
	}

	err := marketMaker.UpdateConfig(config)
	if err != nil {
		t.Fatalf("Failed to update config: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start market maker
	err = marketMaker.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start market maker: %v", err)
	}
	defer marketMaker.Stop()

	// Wait for quotes to be generated
	time.Sleep(1 * time.Second)

	var metrics PerformanceMetrics
	startTime := time.Now()

	// Test quote generation performance
	for i := 0; i < 100; i++ {
		start := time.Now()
		quote, err := marketMaker.GetQuotes(ctx, "BTC-USD")
		latency := time.Since(start)

		if err != nil {
			metrics.AddFailure()
			continue
		}

		metrics.AddLatency(latency)
		metrics.AddSuccess()

		// Validate quote
		if quote.BidPrice.LessThanOrEqual(decimal.Zero) || quote.AskPrice.LessThanOrEqual(decimal.Zero) {
			t.Errorf("Invalid quote prices: bid=%s, ask=%s", quote.BidPrice, quote.AskPrice)
		}

		if quote.AskPrice.LessThanOrEqual(quote.BidPrice) {
			t.Errorf("Ask price must be greater than bid price: bid=%s, ask=%s", quote.BidPrice, quote.AskPrice)
		}

		spread := quote.AskPrice.Sub(quote.BidPrice)
		if spread.LessThan(config.MinSpread) || spread.GreaterThan(config.MaxSpread) {
			t.Errorf("Spread out of bounds: %s (min: %s, max: %s)", spread, config.MinSpread, config.MaxSpread)
		}
	}

	totalDuration := time.Since(startTime)
	metrics.Calculate()

	// Performance Requirements
	if metrics.P95Latency > 50*time.Millisecond {
		t.Errorf("P95 latency too high: %v (requirement: <50ms)", metrics.P95Latency)
	}

	throughput := float64(metrics.SuccessfulOps) / totalDuration.Seconds()
	if throughput < 100 {
		t.Errorf("Throughput too low: %.2f ops/sec (requirement: >100 ops/sec)", throughput)
	}

	if metrics.ErrorRate > 5.0 {
		t.Errorf("Error rate too high: %.2f%% (requirement: <5%%)", metrics.ErrorRate)
	}

	t.Logf("Market Maker Strategy Performance Test Results:")
	t.Logf("  Total Operations: %d", metrics.TotalOperations)
	t.Logf("  Successful Operations: %d", metrics.SuccessfulOps)
	t.Logf("  Average Latency: %v", metrics.AverageLatency)
	t.Logf("  P95 Latency: %v", metrics.P95Latency)
	t.Logf("  P99 Latency: %v", metrics.P99Latency)
	t.Logf("  Throughput: %.2f ops/sec", throughput)
	t.Logf("  Error Rate: %.2f%%", metrics.ErrorRate)
}

// Test Market Maker Risk Management
func TestMarketMakerRiskManagement(t *testing.T) {
	env := NewTestEnvironment(DefaultTestConfig())
	marketMaker := NewMockMarketMaker(env.TradingService)

	config := MarketMakerConfig{
		TradingPairs:   []string{"BTC-USD"},
		TargetSpread:   decimal.NewFromFloat(0.002),
		QuoteSize:      decimal.NewFromFloat(1.0),
		MaxInventory:   decimal.NewFromFloat(5.0), // Low inventory limit for testing
		Strategy:       "inventory_skew",
		UpdateInterval: 100 * time.Millisecond,
		RiskEnabled:    true,
		MaxLoss:        decimal.NewFromFloat(100.0), // Low loss limit for testing
	}

	err := marketMaker.UpdateConfig(config)
	if err != nil {
		t.Fatalf("Failed to update config: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Start market maker
	err = marketMaker.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start market maker: %v", err)
	}
	defer marketMaker.Stop()

	// Simulate high inventory scenario
	marketMaker.inventory["BTC"] = decimal.NewFromFloat(10.0) // Exceed max inventory

	time.Sleep(500 * time.Millisecond)

	// Test that quotes are adjusted for high inventory
	quote, err := marketMaker.GetQuotes(ctx, "BTC-USD")
	if err != nil {
		t.Fatalf("Failed to get quotes: %v", err)
	}

	// With high inventory, spread should be wider or skewed
	spread := quote.AskPrice.Sub(quote.BidPrice)
	if spread.LessThanOrEqual(config.TargetSpread) {
		t.Errorf("Expected wider spread due to high inventory, got: %s", spread)
	}

	// Test inventory risk calculation
	inventory, err := marketMaker.GetInventory(ctx, "BTC")
	if err != nil {
		t.Fatalf("Failed to get inventory: %v", err)
	}

	if inventory.GreaterThan(config.MaxInventory) {
		t.Logf("Risk management test: High inventory detected: %s (max: %s)", inventory, config.MaxInventory)
	}

	t.Logf("Risk Management Test Results:")
	t.Logf("  Current Inventory: %s", inventory)
	t.Logf("  Max Inventory: %s", config.MaxInventory)
	t.Logf("  Current Spread: %s", spread)
	t.Logf("  Target Spread: %s", config.TargetSpread)
}

// Test Market Maker Concurrent Operations
func TestMarketMakerConcurrency(t *testing.T) {
	env := NewTestEnvironment(DefaultTestConfig())
	marketMaker := NewMockMarketMaker(env.TradingService)

	config := MarketMakerConfig{
		TradingPairs:     []string{"BTC-USD", "ETH-USD", "LTC-USD"},
		TargetSpread:     decimal.NewFromFloat(0.002),
		QuoteSize:        decimal.NewFromFloat(1.0),
		MaxInventory:     decimal.NewFromFloat(10.0),
		Strategy:         "dynamic",
		UpdateInterval:   50 * time.Millisecond,
		RiskEnabled:      true,
		VolatilityFactor: decimal.NewFromFloat(1.0),
	}

	err := marketMaker.UpdateConfig(config)
	if err != nil {
		t.Fatalf("Failed to update config: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start market maker
	err = marketMaker.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start market maker: %v", err)
	}
	defer marketMaker.Stop()

	// Wait for initial quotes
	time.Sleep(200 * time.Millisecond)

	var wg sync.WaitGroup
	var metrics PerformanceMetrics
	concurrentOps := 50

	startTime := time.Now()

	// Run concurrent quote requests
	for i := 0; i < concurrentOps; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			tradingPair := config.TradingPairs[index%len(config.TradingPairs)]

			start := time.Now()
			quote, err := marketMaker.GetQuotes(ctx, tradingPair)
			latency := time.Since(start)

			if err != nil {
				metrics.AddFailure()
				return
			}

			metrics.AddLatency(latency)
			metrics.AddSuccess()

			// Validate concurrent quote integrity
			if quote.BidPrice.LessThanOrEqual(decimal.Zero) || quote.AskPrice.LessThanOrEqual(decimal.Zero) {
				t.Errorf("Invalid concurrent quote: bid=%s, ask=%s", quote.BidPrice, quote.AskPrice)
			}
		}(i)
	}

	wg.Wait()
	totalDuration := time.Since(startTime)
	metrics.Calculate()

	// Performance Requirements for Concurrent Operations
	if metrics.P95Latency > 100*time.Millisecond {
		t.Errorf("Concurrent P95 latency too high: %v (requirement: <100ms)", metrics.P95Latency)
	}

	throughput := float64(metrics.SuccessfulOps) / totalDuration.Seconds()
	if throughput < 50 {
		t.Errorf("Concurrent throughput too low: %.2f ops/sec (requirement: >50 ops/sec)", throughput)
	}

	if metrics.ErrorRate > 5.0 {
		t.Errorf("Concurrent error rate too high: %.2f%% (requirement: <5%%)", metrics.ErrorRate)
	}

	t.Logf("Concurrent Operations Test Results:")
	t.Logf("  Concurrent Operations: %d", concurrentOps)
	t.Logf("  Successful Operations: %d", metrics.SuccessfulOps)
	t.Logf("  Average Latency: %v", metrics.AverageLatency)
	t.Logf("  P95 Latency: %v", metrics.P95Latency)
	t.Logf("  Throughput: %.2f ops/sec", throughput)
	t.Logf("  Error Rate: %.2f%%", metrics.ErrorRate)
}

// Test Market Maker Strategy Comparison
func TestMarketMakerStrategyComparison(t *testing.T) {
	env := NewTestEnvironment(DefaultTestConfig())

	strategies := []string{"basic", "dynamic", "inventory_skew"}
	results := make(map[string]MarketMakerMetrics)

	for _, strategy := range strategies {
		marketMaker := NewMockMarketMaker(env.TradingService)

		config := MarketMakerConfig{
			TradingPairs:     []string{"BTC-USD"},
			TargetSpread:     decimal.NewFromFloat(0.002),
			QuoteSize:        decimal.NewFromFloat(1.0),
			MaxInventory:     decimal.NewFromFloat(10.0),
			Strategy:         strategy,
			UpdateInterval:   100 * time.Millisecond,
			RiskEnabled:      true,
			VolatilityFactor: decimal.NewFromFloat(0.5),
		}

		err := marketMaker.UpdateConfig(config)
		if err != nil {
			t.Fatalf("Failed to update config for strategy %s: %v", strategy, err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)

		// Start market maker
		err = marketMaker.Start(ctx)
		if err != nil {
			cancel()
			t.Fatalf("Failed to start market maker for strategy %s: %v", strategy, err)
		}

		// Wait for quotes generation
		time.Sleep(500 * time.Millisecond)

		// Collect metrics
		metrics, err := marketMaker.GetPerformanceMetrics(ctx)
		if err != nil {
			cancel()
			marketMaker.Stop()
			t.Fatalf("Failed to get metrics for strategy %s: %v", strategy, err)
		}

		results[strategy] = metrics

		marketMaker.Stop()
		cancel()

		t.Logf("Strategy %s Results:", strategy)
		t.Logf("  Total Quotes: %d", metrics.TotalQuotes)
		t.Logf("  Uptime: %v", metrics.Uptime)
	}

	// Compare strategies
	if len(results) != len(strategies) {
		t.Errorf("Expected results for %d strategies, got %d", len(strategies), len(results))
	}

	// Validate that all strategies generated quotes
	for strategy, metrics := range results {
		if metrics.TotalQuotes == 0 {
			t.Errorf("Strategy %s generated no quotes", strategy)
		}
		if metrics.Uptime <= 0 {
			t.Errorf("Strategy %s has invalid uptime: %v", strategy, metrics.Uptime)
		}
	}
}

// Test Market Maker Integration with Trading Service
func TestMarketMakerTradingIntegration(t *testing.T) {
	env := NewTestEnvironment(DefaultTestConfig())
	marketMaker := NewMockMarketMaker(env.TradingService)

	config := MarketMakerConfig{
		TradingPairs:   []string{"BTC-USD"},
		TargetSpread:   decimal.NewFromFloat(0.002),
		QuoteSize:      decimal.NewFromFloat(1.0),
		MaxInventory:   decimal.NewFromFloat(10.0),
		Strategy:       "basic",
		UpdateInterval: 100 * time.Millisecond,
		RiskEnabled:    true,
	}

	err := marketMaker.UpdateConfig(config)
	if err != nil {
		t.Fatalf("Failed to update config: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Start market maker
	err = marketMaker.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start market maker: %v", err)
	}
	defer marketMaker.Stop()

	// Wait for initial quotes
	time.Sleep(200 * time.Millisecond)

	// Test integration by placing orders against market maker quotes
	quote, err := marketMaker.GetQuotes(ctx, "BTC-USD")
	if err != nil {
		t.Fatalf("Failed to get quotes: %v", err)
	}

	// Create test user and place order
	testUser := TestUser{
		ID:       uuid.New(),
		Username: "test_trader",
		Balance:  decimal.NewFromFloat(10000.0),
	}

	// Place buy order at bid price
	buyOrder := TestOrder{
		UserID:        testUser.ID,
		TradingPairID: "BTC-USD",
		Type:          "limit",
		Side:          "buy",
		Quantity:      decimal.NewFromFloat(0.5),
		Price:         quote.BidPrice,
	}

	placedOrder, err := env.TradingService.PlaceOrder(ctx, buyOrder)
	if err != nil {
		t.Fatalf("Failed to place order: %v", err)
	}

	if placedOrder.Status != "pending" {
		t.Errorf("Expected order status 'pending', got: %s", placedOrder.Status)
	}

	// Verify order is in the system
	userOrders, err := env.TradingService.GetUserOrders(ctx, testUser.ID)
	if err != nil {
		t.Fatalf("Failed to get user orders: %v", err)
	}

	if len(userOrders) != 1 {
		t.Errorf("Expected 1 user order, got: %d", len(userOrders))
	}

	t.Logf("Integration Test Results:")
	t.Logf("  Quote Bid: %s", quote.BidPrice)
	t.Logf("  Quote Ask: %s", quote.AskPrice)
	t.Logf("  Quote Spread: %s", quote.Spread)
	t.Logf("  Order Placed: %s", placedOrder.ID)
	t.Logf("  Order Status: %s", placedOrder.Status)
}

// Test Market Maker Performance Under Load
func TestMarketMakerLoadTesting(t *testing.T) {
	env := NewTestEnvironment(DefaultTestConfig())
	marketMaker := NewMockMarketMaker(env.TradingService)

	config := MarketMakerConfig{
		TradingPairs:     []string{"BTC-USD", "ETH-USD", "LTC-USD", "ADA-USD", "DOT-USD"},
		TargetSpread:     decimal.NewFromFloat(0.002),
		QuoteSize:        decimal.NewFromFloat(1.0),
		MaxInventory:     decimal.NewFromFloat(10.0),
		Strategy:         "dynamic",
		UpdateInterval:   25 * time.Millisecond, // High frequency updates
		RiskEnabled:      true,
		VolatilityFactor: decimal.NewFromFloat(1.0),
	}

	err := marketMaker.UpdateConfig(config)
	if err != nil {
		t.Fatalf("Failed to update config: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Start market maker
	err = marketMaker.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start market maker: %v", err)
	}
	defer marketMaker.Stop()

	// Wait for initial quotes
	time.Sleep(300 * time.Millisecond)

	var wg sync.WaitGroup
	var metrics PerformanceMetrics
	loadOps := 200 // High load

	startTime := time.Now()

	// Generate high load
	for i := 0; i < loadOps; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			tradingPair := config.TradingPairs[index%len(config.TradingPairs)]

			// Multiple operations per goroutine
			for j := 0; j < 5; j++ {
				start := time.Now()
				quote, err := marketMaker.GetQuotes(ctx, tradingPair)
				latency := time.Since(start)

				if err != nil {
					metrics.AddFailure()
					continue
				}

				metrics.AddLatency(latency)
				metrics.AddSuccess()

				// Quick validation
				if quote.BidPrice.LessThanOrEqual(decimal.Zero) || quote.AskPrice.LessThanOrEqual(quote.BidPrice) {
					metrics.AddFailure()
				}

				// Brief pause between operations
				time.Sleep(10 * time.Millisecond)
			}
		}(i)
	}

	wg.Wait()
	totalDuration := time.Since(startTime)
	metrics.Calculate()

	// Get final performance metrics from market maker
	mmMetrics, err := marketMaker.GetPerformanceMetrics(ctx)
	if err != nil {
		t.Fatalf("Failed to get market maker metrics: %v", err)
	}

	// Load Testing Performance Requirements
	if metrics.P95Latency > 200*time.Millisecond {
		t.Errorf("Load test P95 latency too high: %v (requirement: <200ms)", metrics.P95Latency)
	}

	throughput := float64(metrics.SuccessfulOps) / totalDuration.Seconds()
	if throughput < 100 {
		t.Errorf("Load test throughput too low: %.2f ops/sec (requirement: >100 ops/sec)", throughput)
	}

	if metrics.ErrorRate > 10.0 {
		t.Errorf("Load test error rate too high: %.2f%% (requirement: <10%%)", metrics.ErrorRate)
	}

	// Validate market maker internal metrics
	if mmMetrics.TotalQuotes == 0 {
		t.Error("Market maker generated no quotes during load test")
	}

	t.Logf("Load Testing Results:")
	t.Logf("  Load Operations: %d", loadOps*5)
	t.Logf("  Successful Operations: %d", metrics.SuccessfulOps)
	t.Logf("  Average Latency: %v", metrics.AverageLatency)
	t.Logf("  P95 Latency: %v", metrics.P95Latency)
	t.Logf("  P99 Latency: %v", metrics.P99Latency)
	t.Logf("  Throughput: %.2f ops/sec", throughput)
	t.Logf("  Error Rate: %.2f%%", metrics.ErrorRate)
	t.Logf("  Market Maker Total Quotes: %d", mmMetrics.TotalQuotes)
	t.Logf("  Market Maker Uptime: %v", mmMetrics.Uptime)
}
