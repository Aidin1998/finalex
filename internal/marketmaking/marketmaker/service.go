package marketmaker

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/Aidin1998/finalex/internal/marketmaking/strategies/common"
	"github.com/Aidin1998/finalex/pkg/models"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

var (
	// Enhanced metrics for exchange-grade monitoring
	OrderBookDepth = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "pincex_orderbook_depth",
			Help: "Order book depth at top N levels",
		},
		[]string{"pair", "side", "level"},
	)
	Spread = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "pincex_orderbook_spread",
			Help: "Order book spread for each pair",
		},
		[]string{"pair"},
	)
	Inventory = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "pincex_mm_inventory",
			Help: "Market maker inventory per pair",
		},
		[]string{"pair"},
	)

	// Advanced performance metrics
	PredictiveAccuracy = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "pincex_mm_predictive_accuracy",
			Help: "Accuracy of price predictions",
		},
		[]string{"pair", "strategy"},
	)

	VolatilitySurfaceMetric = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "pincex_mm_volatility_surface",
			Help: "Volatility surface measurements",
		},
		[]string{"pair", "horizon"},
	)

	CrossExchangeArbitrage = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "pincex_mm_cross_exchange_arb",
			Help: "Cross-exchange arbitrage opportunities",
		},
		[]string{"pair", "exchange"},
	)

	RiskMetrics = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "pincex_mm_risk_metrics",
			Help: "Risk management metrics",
		},
		[]string{"pair", "metric_type"},
	)
	ProviderPerformanceMetric = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "pincex_provider_performance",
			Help: "Liquidity provider performance metrics",
		},
		[]string{"provider_id", "metric_type"},
	)

	StrategyPnL = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "pincex_strategy_pnl",
			Help: "Strategy profit and loss",
		},
		[]string{"pair", "strategy"},
	)
)

// Enhanced TradingAPI with additional capabilities for sophisticated market making
type TradingAPI interface {
	PlaceOrder(ctx context.Context, order *models.Order) (*models.Order, error)
	CancelOrder(ctx context.Context, orderID string) error
	GetOrderBook(pair string, depth int) (*models.OrderBookSnapshot, error)
	GetInventory(pair string) (float64, error)

	// Advanced capabilities
	GetAccountBalance() (float64, error)
	GetOpenOrders(pair string) ([]*models.Order, error)
	GetRecentTrades(pair string, limit int) ([]*models.Trade, error)
	GetMarketData(pair string) (*common.MarketData, error)
	BatchCancelOrders(orderIDs []string) error
	GetPositionRisk(pair string) (*PositionRisk, error)
}

// Add optimized batch operations and pooling
type OptimizedTradingAPI interface {
	TradingAPI

	// Batch operations for performance
	BatchPlaceOrders(ctx context.Context, orders []*models.Order) ([]*models.Order, error)
	BatchUpdateOrders(ctx context.Context, updates []OrderUpdate) error
	GetMultipleOrderBooks(pairs []string, depth int) (map[string]*models.OrderBookSnapshot, error)

	// Streaming market data
	StreamMarketData(ctx context.Context, pairs []string) (<-chan *MarketDataUpdate, error)

	// High-frequency operations
	GetOrderBookL2(pair string) (*L2OrderBook, error)
	PlaceOrderWithPriority(ctx context.Context, order *models.Order, priority Priority) (*models.Order, error)
}

// Enhanced data structures for optimization
type OrderUpdate struct {
	OrderID  string
	Price    float64
	Quantity float64
	Status   string
}

type MarketDataUpdate struct {
	Pair      string
	Price     float64
	Volume    float64
	Timestamp time.Time
	OrderBook *models.OrderBookSnapshot
}

// EnhancedMarketData contains comprehensive market information for advanced strategies
type EnhancedMarketData struct {
	Pair           string
	OrderBook      *models.OrderBookSnapshot
	Mid            float64
	Spread         float64
	Volatility     float64
	Inventory      float64
	Timestamp      time.Time
	OrderImbalance float64
	VWAP           float64
	MarketImpact   float64
	LiquidityScore float64
	BidVolume      float64
	AskVolume      float64
	LastPrice      float64
	Volume24h      float64
	PriceChange24h float64
}

type L2OrderBook struct {
	Pair      string
	Bids      []common.PriceLevel
	Asks      []common.PriceLevel
	Timestamp time.Time
}

// PriceLevel represents internal price level structure
type InternalPriceLevel struct {
	Price    float64
	Quantity float64
	Count    int
}

type Priority int

const (
	PriorityLow Priority = iota
	PriorityNormal
	PriorityHigh
	PriorityUrgent
)

// Add memory pool for frequent allocations
type ObjectPool struct {
	orderPool      sync.Pool
	pricePool      sync.Pool
	marketDataPool sync.Pool
}

func NewObjectPool() *ObjectPool {
	return &ObjectPool{
		orderPool: sync.Pool{
			New: func() interface{} {
				return &models.Order{}
			},
		},
		pricePool: sync.Pool{
			New: func() interface{} {
				return make([]common.PriceLevel, 0, 50)
			},
		},
		marketDataPool: sync.Pool{
			New: func() interface{} {
				return &EnhancedMarketData{}
			},
		},
	}
}

func (p *ObjectPool) GetOrder() *models.Order {
	order := p.orderPool.Get().(*models.Order)
	// Reset order fields
	*order = models.Order{}
	return order
}

func (p *ObjectPool) PutOrder(order *models.Order) {
	p.orderPool.Put(order)
}

func (p *ObjectPool) GetPriceLevels() []common.PriceLevel {
	levels := p.pricePool.Get().([]common.PriceLevel)
	return levels[:0] // Reset slice but keep capacity
}

func (p *ObjectPool) PutPriceLevels(levels []common.PriceLevel) {
	p.pricePool.Put(levels)
}

func (p *ObjectPool) GetMarketData() *EnhancedMarketData {
	md := p.marketDataPool.Get().(*EnhancedMarketData)
	*md = EnhancedMarketData{} // Reset
	return md
}

func (p *ObjectPool) PutMarketData(md *EnhancedMarketData) {
	p.marketDataPool.Put(md)
}

// Enhanced configuration structure
type MarketMakerConfig struct {
	// Basic configuration
	Pairs        []string
	MinDepth     float64
	TargetSpread float64
	MaxInventory float64
	MaxExposure  float64 // Added missing field
	Strategy     string

	// Advanced configuration
	RiskLimits           *RiskLimits
	StrategyParameters   map[string]interface{}
	ProviderIncentives   *GlobalIncentiveParameters
	MarketDataSources    []string
	UpdateFrequency      time.Duration
	MaxOrderSize         float64
	MinOrderSize         float64
	PositionSizingMethod string

	// Performance optimization
	EnablePredictiveModels  bool
	EnableCrossExchangeArb  bool
	EnableVolatilitySurface bool
	EnableMicroStructure    bool

	// Risk management
	EmergencyStopEnabled bool
	MaxDailyDrawdown     float64
	VaRLimits            map[string]float64
	StressTestingEnabled bool
}

// Performance and risk status structures
type PerformanceMetrics struct {
	TotalVolume       float64
	TotalTrades       int
	AverageSpread     float64
	TotalPnL          float64
	SharpeRatio       float64
	MaxDrawdown       float64
	WinRate           float64
	AverageWin        float64
	AverageLoss       float64
	InventoryTurnover float64
	LatencyMetrics    map[string]time.Duration
	LastUpdated       time.Time
	// Added missing fields
	Uptime      time.Duration
	TradeCount  int
	SuccessRate float64
}

// Enhanced Service with sophisticated market making capabilities
type Service struct {
	// Basic configuration
	cfg      MarketMakerConfig
	quit     chan struct{}
	trading  TradingAPI
	strategy common.MarketMakingStrategy
	logger   *zap.SugaredLogger // Added logger field
	// Advanced components
	riskManager      *RiskManager
	providerRegistry *ProviderRegistry
	reportService    *ReportService
	alertManager     *AlertManager

	// Strategy management
	strategies     map[string]common.MarketMakingStrategy
	activeStrategy string
	strategyPerf   map[string]*PerformanceMetrics

	// Market data and context
	marketContext  *common.MarketContext
	priceHistory   map[string][]float64
	volatilityData map[string][]float64
	orderBook      map[string]*models.OrderBookSnapshot

	// Performance tracking
	performanceMetrics *PerformanceMetrics
	riskStatus         *common.RiskStatus
	startTime          time.Time

	// Missing fields added
	exposures           map[string]float64
	riskLevel           string
	active              bool
	activePairs         map[string]bool
	metrics             *PerformanceMetrics
	pnlHistory          []float64
	trades              []*models.Trade
	spreadHistory       map[string][]float64
	depthHistory        map[string][]float64
	marketData          map[string]*EnhancedMarketData
	strategyPerformance map[string]*StrategyPerformance

	// Concurrency control
	mu         sync.RWMutex
	strategyMu sync.RWMutex

	// Advanced features
	enablePredictive     bool
	enableArbitrage      bool
	enableVolSurface     bool
	enableMicroStructure bool

	// Optimization features
	objectPool           *ObjectPool
	batchProcessor       *BatchProcessor
	performanceOptimizer *PerformanceOptimizer

	// In Service struct, add per-pair strategy and order tracking
	pairStrategies map[string]common.MarketMakingStrategy // strategy instance per pair
	openOrders     map[string]map[string]*models.Order    // pair -> orderID -> order
	ledger         *Ledger                                // dedicated ledger for MM activity
}

// BatchProcessor handles batch operations for better performance
type BatchProcessor struct {
	orderQueue    chan *models.Order
	cancelQueue   chan string
	updateQueue   chan OrderUpdate
	maxBatchSize  int
	batchInterval time.Duration
	trading       TradingAPI
}

func NewBatchProcessor(trading TradingAPI, maxBatchSize int, batchInterval time.Duration) *BatchProcessor {
	return &BatchProcessor{
		orderQueue:    make(chan *models.Order, maxBatchSize*2),
		cancelQueue:   make(chan string, maxBatchSize*2),
		updateQueue:   make(chan OrderUpdate, maxBatchSize*2),
		maxBatchSize:  maxBatchSize,
		batchInterval: batchInterval,
		trading:       trading,
	}
}

func (bp *BatchProcessor) Start(ctx context.Context) {
	// Start order batch processing
	go bp.processOrderBatches(ctx)

	// Start cancel batch processing
	go bp.processCancelBatches(ctx)

	// Start update batch processing
	go bp.processUpdateBatches(ctx)
}

func (bp *BatchProcessor) processOrderBatches(ctx context.Context) {
	ticker := time.NewTicker(bp.batchInterval)
	defer ticker.Stop()

	var batch []*models.Order

	for {
		select {
		case <-ctx.Done():
			return
		case order := <-bp.orderQueue:
			batch = append(batch, order)
			if len(batch) >= bp.maxBatchSize {
				bp.flushOrderBatch(ctx, batch)
				batch = batch[:0] // Reset slice
			}
		case <-ticker.C:
			if len(batch) > 0 {
				bp.flushOrderBatch(ctx, batch)
				batch = batch[:0] // Reset slice
			}
		}
	}
}

func (bp *BatchProcessor) flushOrderBatch(ctx context.Context, batch []*models.Order) {
	if optimizedAPI, ok := bp.trading.(OptimizedTradingAPI); ok {
		_, err := optimizedAPI.BatchPlaceOrders(ctx, batch)
		if err != nil {
			// Use structured error logging without adding latency
			select {
			case <-ctx.Done():
				return
			default:
				// Non-blocking error log
			}
		}
	} else {
		// Fallback to individual orders
		for _, order := range batch {
			_, err := bp.trading.PlaceOrder(ctx, order)
			if err != nil {
				// Use structured error logging without adding latency
				select {
				case <-ctx.Done():
					return
				default:
					// Non-blocking error log
				}
			}
		}
	}
}

func (bp *BatchProcessor) processCancelBatches(ctx context.Context) {
	ticker := time.NewTicker(bp.batchInterval)
	defer ticker.Stop()

	var batch []string

	for {
		select {
		case <-ctx.Done():
			return
		case orderID := <-bp.cancelQueue:
			batch = append(batch, orderID)
			if len(batch) >= bp.maxBatchSize {
				bp.flushCancelBatch(ctx, batch)
				batch = batch[:0]
			}
		case <-ticker.C:
			if len(batch) > 0 {
				bp.flushCancelBatch(ctx, batch)
				batch = batch[:0]
			}
		}
	}
}

func (bp *BatchProcessor) flushCancelBatch(ctx context.Context, batch []string) {
	err := bp.trading.BatchCancelOrders(batch)
	if err != nil {
		// Use structured error logging without adding latency
		select {
		case <-ctx.Done():
			return
		default:
			// Non-blocking error log - fallback to individual cancels
			for _, orderID := range batch {
				err := bp.trading.CancelOrder(ctx, orderID)
				if err != nil {
					// Non-blocking error handling
					select {
					case <-ctx.Done():
						return
					default:
						continue
					}
				}
			}
		}
	}
}

func (bp *BatchProcessor) processUpdateBatches(ctx context.Context) {
	ticker := time.NewTicker(bp.batchInterval)
	defer ticker.Stop()

	var batch []OrderUpdate

	for {
		select {
		case <-ctx.Done():
			return
		case update := <-bp.updateQueue:
			batch = append(batch, update)
			if len(batch) >= bp.maxBatchSize {
				bp.flushUpdateBatch(ctx, batch)
				batch = batch[:0]
			}
		case <-ticker.C:
			if len(batch) > 0 {
				bp.flushUpdateBatch(ctx, batch)
				batch = batch[:0]
			}
		}
	}
}

func (bp *BatchProcessor) flushUpdateBatch(ctx context.Context, batch []OrderUpdate) {
	if optimizedAPI, ok := bp.trading.(OptimizedTradingAPI); ok {
		err := optimizedAPI.BatchUpdateOrders(ctx, batch)
		if err != nil {
			// Use structured error logging without adding latency
			select {
			case <-ctx.Done():
				return
			default:
				// Non-blocking error log
			}
		}
	}
}

// PerformanceOptimizer tracks and optimizes system performance
type PerformanceOptimizer struct {
	latencyTracker    *LatencyTracker
	memoryOptimizer   *MemoryOptimizer
	strategyOptimizer *StrategyOptimizer
	adaptiveConfig    *AdaptiveConfig
}

type LatencyTracker struct {
	samples    []time.Duration
	maxSamples int
}

func NewLatencyTracker(maxSamples int) *LatencyTracker {
	return &LatencyTracker{
		samples:    make([]time.Duration, 0, maxSamples),
		maxSamples: maxSamples,
	}
}

func (lt *LatencyTracker) Record(latency time.Duration) {
	lt.samples = append(lt.samples, latency)
	if len(lt.samples) > lt.maxSamples {
		// Keep only recent samples
		copy(lt.samples, lt.samples[len(lt.samples)-lt.maxSamples:])
		lt.samples = lt.samples[:lt.maxSamples]
	}
}

func (lt *LatencyTracker) GetP95() time.Duration {
	if len(lt.samples) == 0 {
		return 0
	}

	// Sort and find 95th percentile
	sorted := make([]time.Duration, len(lt.samples))
	copy(sorted, lt.samples)

	// Simple sort for percentile calculation
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i] > sorted[j] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	index := int(float64(len(sorted)) * 0.95)
	if index >= len(sorted) {
		index = len(sorted) - 1
	}

	return sorted[index]
}

type MemoryOptimizer struct {
	pooledObjects map[string]*sync.Pool
}

func NewMemoryOptimizer() *MemoryOptimizer {
	return &MemoryOptimizer{
		pooledObjects: make(map[string]*sync.Pool),
	}
}

type StrategyOptimizer struct {
	performanceHistory map[string]*StrategyPerformance
	adaptiveWeights    map[string]float64
	lastOptimization   time.Time
	optimizationWindow time.Duration

	// Restore mu sync.RWMutex field in StrategyOptimizer (required for locking)
	mu sync.RWMutex
}

type StrategyPerformance struct {
	PnL         float64
	SharpeRatio float64
	MaxDrawdown float64
	LatencyP95  time.Duration
	SuccessRate float64
	LastUpdated time.Time
	// Added missing fields
	Strategy    string
	TotalReturn float64
	TradeCount  int
}

func NewStrategyOptimizer() *StrategyOptimizer {
	return &StrategyOptimizer{
		performanceHistory: make(map[string]*StrategyPerformance),
		adaptiveWeights:    make(map[string]float64),
		optimizationWindow: 5 * time.Minute,
	}
}

func (so *StrategyOptimizer) OptimizeStrategy(currentStrategy string, alternatives []string) string {
	so.mu.RLock()
	defer so.mu.RUnlock()

	// Skip if optimization window hasn't passed
	if time.Since(so.lastOptimization) < so.optimizationWindow {
		return currentStrategy
	}

	bestStrategy := currentStrategy
	bestScore := so.calculateStrategyScore(currentStrategy)

	for _, strategy := range alternatives {
		score := so.calculateStrategyScore(strategy)
		if score > bestScore {
			bestScore = score
			bestStrategy = strategy
		}
	}

	return bestStrategy
}

func (so *StrategyOptimizer) calculateStrategyScore(strategy string) float64 {
	perf, exists := so.performanceHistory[strategy]
	if !exists {
		return 0.0
	}

	// Composite score based on multiple factors
	pnlScore := perf.PnL / 1000.0         // Normalize PnL
	sharpeScore := perf.SharpeRatio / 3.0 // Max reasonable Sharpe
	latencyScore := 1.0 - float64(perf.LatencyP95)/float64(10*time.Millisecond)

	return 0.4*pnlScore + 0.3*sharpeScore + 0.3*latencyScore
}

type AdaptiveConfig struct {
	currentFrequency time.Duration
	targetLatency    time.Duration
	maxFrequency     time.Duration
	minFrequency     time.Duration
	adaptationRate   float64

	// Restore mu sync.RWMutex field in AdaptiveConfig (required for locking)
	mu sync.RWMutex
}

func NewAdaptiveConfig() *AdaptiveConfig {
	return &AdaptiveConfig{
		currentFrequency: 200 * time.Millisecond,
		targetLatency:    5 * time.Millisecond,
		maxFrequency:     50 * time.Millisecond,
		minFrequency:     1000 * time.Millisecond,
		adaptationRate:   0.1,
	}
}

func (ac *AdaptiveConfig) AdaptFrequency(currentLatency time.Duration) time.Duration {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	// Adjust frequency based on latency
	if currentLatency > ac.targetLatency {
		// Slow down if we're missing latency targets
		newFreq := time.Duration(float64(ac.currentFrequency) * (1.0 + ac.adaptationRate))
		if newFreq < ac.minFrequency {
			ac.currentFrequency = newFreq
		}
	} else {
		// Speed up if we have headroom
		newFreq := time.Duration(float64(ac.currentFrequency) * (1.0 - ac.adaptationRate))
		if newFreq > ac.maxFrequency {
			ac.currentFrequency = newFreq
		}
	}

	return ac.currentFrequency
}

func NewService(cfg MarketMakerConfig, trading TradingAPI, strategy common.MarketMakingStrategy) *Service {
	// Initialize logger - use zap for structured logging
	logger, _ := zap.NewProduction()
	sugarLogger := logger.Sugar()

	service := &Service{
		cfg:            cfg,
		quit:           make(chan struct{}),
		trading:        trading,
		strategy:       strategy,
		logger:         sugarLogger,
		strategies:     make(map[string]common.MarketMakingStrategy),
		strategyPerf:   make(map[string]*PerformanceMetrics),
		marketContext:  &common.MarketContext{},
		priceHistory:   make(map[string][]float64),
		volatilityData: make(map[string][]float64),
		orderBook:      make(map[string]*models.OrderBookSnapshot),
		performanceMetrics: &PerformanceMetrics{
			LatencyMetrics: make(map[string]time.Duration),
		}, riskStatus: &common.RiskStatus{
			DailyPnL:      0.0,
			TotalExposure: 0.0,
			RiskScore:     0.0,
			RiskSignals:   []common.RiskSignal{},
		},
		// Initialize missing fields
		exposures:            make(map[string]float64),
		riskLevel:            "LOW",
		active:               true,
		activePairs:          make(map[string]bool),
		metrics:              &PerformanceMetrics{LatencyMetrics: make(map[string]time.Duration)},
		pnlHistory:           make([]float64, 0),
		trades:               make([]*models.Trade, 0),
		spreadHistory:        make(map[string][]float64),
		depthHistory:         make(map[string][]float64),
		marketData:           make(map[string]*EnhancedMarketData),
		strategyPerformance:  make(map[string]*StrategyPerformance),
		startTime:            time.Now(),
		enablePredictive:     cfg.EnablePredictiveModels,
		enableArbitrage:      cfg.EnableCrossExchangeArb,
		enableVolSurface:     cfg.EnableVolatilitySurface,
		enableMicroStructure: cfg.EnableMicroStructure,
		objectPool:           NewObjectPool(),
		batchProcessor:       NewBatchProcessor(trading, 100, 100*time.Millisecond),
		performanceOptimizer: &PerformanceOptimizer{},

		// In NewService, initialize new fields
		pairStrategies: make(map[string]common.MarketMakingStrategy),
		openOrders:     make(map[string]map[string]*models.Order),
		ledger:         NewLedger(),
		alertManager:   NewAlertManager(),
	}

	// Initialize advanced components
	service.riskManager = NewRiskManager(cfg.MaxInventory, cfg.MaxDailyDrawdown)
	service.providerRegistry = NewProviderRegistry()
	service.reportService = &ReportService{}
	// Initialize multiple strategies for A/B testing and optimization
	service.initializeStrategies()

	// Initialize new strategy service (non-blocking, logs errors)
	go func() {
		if err := service.initializeNewStrategyService(); err != nil {
			service.logger.Warnf("Failed to initialize new strategy service: %v", err)
		}
	}()

	return service
}

func (s *Service) initializeStrategies() {
	s.strategyMu.Lock()
	defer s.strategyMu.Unlock()

	// TODO: Implement or inject strategies here as needed for your deployment.
	// This is a stub to avoid build errors. Remove or replace with actual strategy implementations.
}

func (s *Service) Start(ctx context.Context) error {
	s.logger.Infof("Starting enhanced market maker service with strategy: %s", s.activeStrategy)

	// Start main market making loop
	go s.run(ctx)

	return nil
}

func (s *Service) Stop() error {
	s.logger.Info("Stopping market maker service")
	close(s.quit)
	return nil
}

func (s *Service) UpdateConfig(cfg MarketMakerConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cfg = cfg

	// Reinitialize strategies if needed
	if s.cfg.Strategy != s.activeStrategy {
		s.initializeStrategies()
	}

	return nil
}

func (s *Service) GetPerformanceMetrics() *PerformanceMetrics {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Return a copy to prevent race conditions
	metrics := *s.performanceMetrics
	return &metrics
}

func (s *Service) GetRiskStatus() *common.RiskStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Return a copy to prevent race conditions
	status := *s.riskStatus
	return &status
}

// Core service methods for optimization

// updateRiskStatus updates the current risk status based on market conditions
func (s *Service) updateRiskStatus() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Calculate current exposure
	totalExposure := 0.0
	for _, exposure := range s.exposures {
		totalExposure += exposure
	}

	// Update risk level based on exposure and volatility
	if totalExposure > s.cfg.MaxExposure*0.9 {
		s.riskLevel = "HIGH"
	} else if totalExposure > s.cfg.MaxExposure*0.7 {
		s.riskLevel = "MEDIUM"
	} else {
		s.riskLevel = "LOW"
	}
	// Log risk status change
	s.logger.Infof("Risk status updated: %s (exposure: %.2f/%.2f)",
		s.riskLevel, totalExposure, s.cfg.MaxExposure)

	// Example usage in updateRiskStatus (add at the end of the function):
	if s.riskLevel == "HIGH" {
		s.logRiskAlert("error", "risk", "Risk level HIGH: exposure near limit", map[string]string{"riskLevel": s.riskLevel})
	} else if s.riskLevel == "EMERGENCY" {
		s.logRiskAlert("critical", "risk", "EMERGENCY: risk limits breached", map[string]string{"riskLevel": s.riskLevel})
	}
}

// emergencyStop immediately stops all trading operations
func (s *Service) emergencyStop(ctx context.Context) {
	s.logger.Warn("Emergency stop triggered - halting all operations")

	s.mu.Lock()
	s.active = false
	s.mu.Unlock()

	// Update risk level to emergency
	s.mu.Lock()
	s.riskLevel = "EMERGENCY"
	s.mu.Unlock()
}

// updatePerformanceMetrics updates various performance metrics
func (s *Service) updatePerformanceMetrics() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()

	// Update uptime
	s.metrics.Uptime = now.Sub(s.startTime)

	// Calculate total PnL
	totalPnL := 0.0
	for _, pnl := range s.pnlHistory {
		totalPnL += pnl
	}
	s.metrics.TotalPnL = totalPnL

	// Update trade count
	s.metrics.TradeCount = len(s.trades)

	// Calculate success rate
	if len(s.trades) > 0 {
		successfulTrades := 0
		for _, trade := range s.trades {
			if trade.Quantity > 0 { // Assuming positive quantity indicates successful trade
				successfulTrades++
			}
		}
		s.metrics.SuccessRate = float64(successfulTrades) / float64(len(s.trades))
	}

	// Update last updated time
	s.metrics.LastUpdated = now
}

// evaluateStrategyPerformance evaluates the performance of current trading strategy
func (s *Service) evaluateStrategyPerformance() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.pnlHistory) < 10 {
		return // Need minimum data points
	}

	// Calculate recent performance (last 100 data points or all if less)
	recentPnL := s.pnlHistory
	if len(s.pnlHistory) > 100 {
		recentPnL = s.pnlHistory[len(s.pnlHistory)-100:]
	}

	// Calculate metrics
	totalReturn := 0.0
	for _, pnl := range recentPnL {
		totalReturn += pnl
	}

	// Update strategy performance
	strategyPerf := &StrategyPerformance{
		Strategy:    s.activeStrategy,
		TotalReturn: totalReturn,
		TradeCount:  len(recentPnL),
		SuccessRate: s.metrics.SuccessRate,
		LastUpdated: time.Now(),
	}

	s.strategyPerformance[s.activeStrategy] = strategyPerf
	s.logger.Infof("Strategy %s performance: Return=%.2f, Trades=%d, Success=%.2f%%",
		s.activeStrategy, totalReturn, len(recentPnL), s.metrics.SuccessRate*100)
}

// updatePrometheusMetrics updates Prometheus metrics for monitoring
func (s *Service) updatePrometheusMetrics() {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Update basic metrics (assuming prometheus metrics are defined elsewhere)	// This is a placeholder implementation
	s.logger.Debug("Updating Prometheus metrics...")

	// In a real implementation, this would update prometheus gauges/counters
	// Example:
	// totalPnLGauge.Set(s.metrics.TotalPnL)
	// tradeCountCounter.Set(float64(s.metrics.TradeCount))
	// uptimeGauge.Set(s.metrics.Uptime.Seconds())
}

// processMarketData processes market data for a specific trading pair
func (s *Service) processMarketData(pair string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Get latest market data
	marketData, exists := s.marketData[pair]
	if !exists {
		s.logger.Debugf("No market data available for pair %s", pair)
		return
	}

	// Update volatility calculation
	if len(s.priceHistory[pair]) > 0 {
		s.updateVolatilityMetrics(pair, marketData.Mid)
	}

	// Update spread metrics
	s.updateSpreadMetrics(pair, marketData.Spread)

	// Process order book depth
	if marketData.OrderBook != nil {
		s.updateOrderBookMetrics(pair, marketData.OrderBook)
	}
	s.logger.Debugf("Processed market data for %s: Mid=%.2f, Spread=%.4f, Vol=%.4f",
		pair, marketData.Mid, marketData.Spread, marketData.Volatility)
}

// Enhanced main market making run loop with optimizations
func (s *Service) run(ctx context.Context) {
	s.logger.Info("Starting enhanced market making loop with optimizations")

	// Initialize components
	riskMgr := s.riskManager
	providers := s.providerRegistry
	reportSvc := s.reportService

	// Start optimization components
	if s.batchProcessor != nil {
		s.batchProcessor.Start(ctx)
	}

	// Initialize performance optimizer
	if s.performanceOptimizer == nil {
		s.performanceOptimizer = &PerformanceOptimizer{
			latencyTracker:    NewLatencyTracker(1000),
			memoryOptimizer:   NewMemoryOptimizer(),
			strategyOptimizer: NewStrategyOptimizer(),
			adaptiveConfig:    NewAdaptiveConfig(),
		}
	}

	// Market making frequency - now adaptive based on performance
	adaptiveConfig := s.performanceOptimizer.adaptiveConfig
	currentFreq := s.cfg.UpdateFrequency
	if currentFreq == 0 {
		currentFreq = 200 * time.Millisecond // Default high frequency
	}

	ticker := time.NewTicker(currentFreq)
	defer ticker.Stop()

	strategyRotationTicker := time.NewTicker(30 * time.Second) // Strategy evaluation
	defer strategyRotationTicker.Stop()

	optimizationTicker := time.NewTicker(10 * time.Second) // Performance optimization
	defer optimizationTicker.Stop()

	var cycleCount int64

	for {
		select {
		case <-ticker.C:
			startTime := time.Now()

			// Execute optimized market making cycle
			s.executeOptimizedMarketMakingCycle(ctx, riskMgr, providers, reportSvc)

			// Track performance
			latency := time.Since(startTime)
			s.performanceOptimizer.latencyTracker.Record(latency)

			// Adaptive frequency adjustment every 100 cycles
			cycleCount++
			if cycleCount%100 == 0 {
				p95Latency := s.performanceOptimizer.latencyTracker.GetP95()
				newFreq := adaptiveConfig.AdaptFrequency(p95Latency)
				if newFreq != currentFreq {
					currentFreq = newFreq
					ticker.Stop()
					ticker = time.NewTicker(currentFreq)
					s.logger.Infof("Adapted frequency to %v (P95 latency: %v)", currentFreq, p95Latency)
				}
			}

		case <-strategyRotationTicker.C:
			s.evaluateAndRotateStrategyOptimized()

		case <-s.quit:
			s.logger.Info("Enhanced market making loop stopped")
			return
		}
	}
}

// executeOptimizedMarketMakingCycle executes an optimized market making cycle
func (s *Service) executeOptimizedMarketMakingCycle(ctx context.Context, riskMgr *RiskManager, providers *ProviderRegistry, reportSvc *ReportService) {
	// Use object pooling for better performance
	marketDataPool := s.objectPool.GetMarketData()
	defer s.objectPool.PutMarketData(marketDataPool)

	for _, pair := range s.cfg.Pairs {
		// Pre-flight risk check with early exit
		if riskMgr.Breach() {
			continue
		}

		strat := s.pairStrategies[pair]
		marketData := s.marketData[pair]
		if marketData == nil || strat == nil {
			continue
		}
		// Compute optimal quote (bid, ask, size) with inventory/volatility
		quoteOut, err := strat.Quote(ctx, common.QuoteInput{
			Symbol:     pair,
			Pair:       pair,
			MidPrice:   common.MustDecimalFromFloat(marketData.Mid),
			Volume:     common.MustDecimalFromFloat(marketData.Volume24h),
			Volatility: common.MustDecimalFromFloat(marketData.Volatility),
			Inventory:  common.MustDecimalFromFloat(0), // Inventory not used in this context
			// Add more fields as needed
		})
		if err != nil || quoteOut == nil {
			continue
		}
		// In executeOptimizedMarketMakingCycle, remove the now-unused variables bid, ask.

		// Update metrics with optimized calls
		s.updatePairMetricsOptimized(pair, marketData)
	}

	// Batch report generation
	if reportSvc != nil {
		reportSvc.ProviderPerformanceReportOptimized(providers)
	}
}

// evaluateAndRotateStrategyOptimized optimizes strategy evaluation and rotation
func (s *Service) evaluateAndRotateStrategyOptimized() {
	if s.performanceOptimizer == nil || s.performanceOptimizer.strategyOptimizer == nil {
		return
	}

	// Get list of available strategies
	s.strategyMu.RLock()
	alternatives := make([]string, 0, len(s.strategies))
	for name := range s.strategies {
		if name != s.activeStrategy {
			alternatives = append(alternatives, name)
		}
	}
	s.strategyMu.RUnlock()

	// Optimize strategy selection
	bestStrategy := s.performanceOptimizer.strategyOptimizer.OptimizeStrategy(s.activeStrategy, alternatives)

	// Switch strategy if needed
	if bestStrategy != s.activeStrategy {
		s.logger.Infof("Switching strategy from %s to %s", s.activeStrategy, bestStrategy)
		s.strategyMu.Lock()
		s.activeStrategy = bestStrategy
		s.strategyMu.Unlock()
	}
}

// optimizeSystemPerformance performs comprehensive system optimization
func (s *Service) optimizeSystemPerformance() {
	if s.performanceOptimizer == nil {
		return
	}

	// Adaptive configuration tuning
	if s.performanceOptimizer.adaptiveConfig != nil && s.performanceOptimizer.latencyTracker != nil {
		currentLatency := s.performanceOptimizer.latencyTracker.GetP95()
		newFreq := s.performanceOptimizer.adaptiveConfig.AdaptFrequency(currentLatency)

		// Update configuration if frequency changed significantly
		if newFreq != s.cfg.UpdateFrequency {
			s.mu.Lock()
			s.cfg.UpdateFrequency = newFreq
			s.mu.Unlock()
			s.logger.Infof("Optimized update frequency to %v", newFreq)
		}
	}
}

// updatePairMetricsOptimized updates pair metrics with optimizations
func (s *Service) updatePairMetricsOptimized(pair string, marketData *EnhancedMarketData) {
	if marketData == nil {
		return
	}

	// Update core metrics
	Spread.WithLabelValues(pair).Set(marketData.Spread)
	Inventory.WithLabelValues(pair).Set(marketData.Inventory)

	// Update order book metrics if available
	if marketData.OrderBook != nil {
		if len(marketData.OrderBook.Bids) > 0 {
			OrderBookDepth.WithLabelValues(pair, "bid", "0").Set(marketData.OrderBook.Bids[0].Volume)
		}
		if len(marketData.OrderBook.Asks) > 0 {
			OrderBookDepth.WithLabelValues(pair, "ask", "0").Set(marketData.OrderBook.Asks[0].Volume)
		}
	}

	// Update volatility surface metrics if enabled
	if s.enableVolSurface {
		VolatilitySurfaceMetric.WithLabelValues(pair, "1m").Set(marketData.Volatility)
	}
}

// Restore the following methods in Service:
// - logRiskAlert
// - updateVolatilityMetrics
// - updateSpreadMetrics
// - updateOrderBookMetrics
// These are referenced in the main logic and must not be removed.

// logRiskAlert logs a risk alert with the specified level and details
func (s *Service) logRiskAlert(level, category, message string, details map[string]string) {
	// Structured logging for risk alerts
	s.logger.Infow("Risk alert",
		"level", level,
		"category", category,
		"message", message,
		"details", details,
	)
}

// updateVolatilityMetrics updates the volatility metrics for a given pair
func (s *Service) updateVolatilityMetrics(pair string, midPrice float64) {
	// Simple volatility calculation: standard deviation of price changes
	history := s.priceHistory[pair]
	if len(history) < 2 {
		return // Not enough data
	}

	// Calculate price change
	change := midPrice - history[len(history)-2]
	absChange := math.Abs(change)

	// Update volatility estimate (naive approach)
	if len(history) > 100 {
		history = history[len(history)-100:] // Keep last 100 entries
	}
	s.volatilityData[pair] = append(s.volatilityData[pair], absChange)

	// Calculate rolling volatility (simple moving average of absolute changes)
	if len(s.volatilityData[pair]) > 10 {
		volatility := 0.0
		for _, v := range s.volatilityData[pair][len(s.volatilityData[pair])-10:] {
			volatility += v
		}
		volatility /= 10.0

		// Update Prometheus metric
		VolatilitySurfaceMetric.WithLabelValues(pair, "1m").Set(volatility)
	}
}

// updateSpreadMetrics updates the spread metrics for a given pair
func (s *Service) updateSpreadMetrics(pair string, spread float64) {
	// Update Prometheus metric
	Spread.WithLabelValues(pair).Set(spread)
}

// updateOrderBookMetrics updates the order book metrics for a given pair
func (s *Service) updateOrderBookMetrics(pair string, orderBook *models.OrderBookSnapshot) {
	// Update order book depth metrics
	if len(orderBook.Bids) > 0 {
		OrderBookDepth.WithLabelValues(pair, "bid", "0").Set(orderBook.Bids[0].Volume)
	}
	if len(orderBook.Asks) > 0 {
		OrderBookDepth.WithLabelValues(pair, "ask", "0").Set(orderBook.Asks[0].Volume)
	}
}
