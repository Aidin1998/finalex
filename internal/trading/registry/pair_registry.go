package registry

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Aidin1998/finalex/internal/trading/orderbook"
	"github.com/Aidin1998/finalex/pkg/models"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// PairRegistry defines the interface for high-performance pair/market discovery
type PairRegistry interface {
	// RegisterPair registers a new trading pair with its orderbook
	RegisterPair(ctx context.Context, pair *models.TradingPair, ob orderbook.OrderBookInterface) error

	// GetOrderBook retrieves the orderbook for a specific pair (O(1) lookup)
	GetOrderBook(symbol string) (orderbook.OrderBookInterface, error)

	// ListPairs returns all available trading pairs with optional filtering
	ListPairs(ctx context.Context, filter *PairFilter) ([]*models.TradingPair, error)

	// IsPairAvailable checks if a trading pair is available for trading (O(1) lookup)
	IsPairAvailable(symbol string) bool

	// FindCommonBase finds pairs that share a common base currency for cross-pair routing
	FindCommonBase(fromSymbol, toSymbol string) ([]CrossPairRoute, error)

	// GetPairMetrics returns performance metrics for monitoring
	GetPairMetrics() *PairRegistryMetrics

	// UnregisterPair removes a pair from the registry
	UnregisterPair(symbol string) error

	// UpdatePairStatus updates trading status for a pair
	UpdatePairStatus(symbol string, status models.TradingStatus) error

	// GetActivePairs returns all pairs with ACTIVE status
	GetActivePairs() []string

	// Shutdown gracefully shuts down the registry
	Shutdown(ctx context.Context) error
}

// PairFilter provides filtering options for ListPairs
type PairFilter struct {
	BaseAsset  string               `json:"base_asset,omitempty"`
	QuoteAsset string               `json:"quote_asset,omitempty"`
	Status     models.TradingStatus `json:"status,omitempty"`
	MinVolume  decimal.Decimal      `json:"min_volume,omitempty"`
	Limit      int                  `json:"limit,omitempty"`
	Offset     int                  `json:"offset,omitempty"`
}

// CrossPairRoute represents a routing path for cross-pair operations
type CrossPairRoute struct {
	FromPair          string          `json:"from_pair"`
	ToPair            string          `json:"to_pair"`
	IntermediatePair  string          `json:"intermediate_pair"`
	CommonBase        string          `json:"common_base"`
	EstimatedSlippage decimal.Decimal `json:"estimated_slippage"`
	LiquidityScore    float64         `json:"liquidity_score"`
}

// PairRegistryMetrics provides performance monitoring data
type PairRegistryMetrics struct {
	TotalPairs        int64         `json:"total_pairs"`
	ActivePairs       int64         `json:"active_pairs"`
	TotalLookups      int64         `json:"total_lookups"`
	AverageLookupTime time.Duration `json:"average_lookup_time"`
	SuccessfulLookups int64         `json:"successful_lookups"`
	FailedLookups     int64         `json:"failed_lookups"`
	LastUpdateTime    time.Time     `json:"last_update_time"`
	MemoryUsage       int64         `json:"memory_usage_bytes"`
}

// HighPerformancePairRegistry implements PairRegistry with O(1) lookup performance
type HighPerformancePairRegistry struct {
	// Core data structures for O(1) lookups
	pairs           sync.Map // map[string]*registeredPair - thread-safe for concurrent access
	orderBooks      sync.Map // map[string]orderbook.OrderBookInterface
	baseAssetIndex  sync.Map // map[string][]string - base asset to pairs mapping
	quoteAssetIndex sync.Map // map[string][]string - quote asset to pairs mapping

	// Metrics and monitoring
	metrics       *PairRegistryMetrics
	metricsMu     sync.RWMutex
	lookupLatency *LatencyTracker

	// Configuration
	config *RegistryConfig
	logger *zap.Logger

	// Lifecycle management
	ctx          context.Context
	cancel       context.CancelFunc
	shutdownOnce sync.Once
	running      int64 // atomic flag
}

// registeredPair represents a pair entry in the registry
type registeredPair struct {
	TradingPair  *models.TradingPair          `json:"trading_pair"`
	OrderBook    orderbook.OrderBookInterface `json:"-"`
	RegisteredAt time.Time                    `json:"registered_at"`
	LastActivity time.Time                    `json:"last_activity"`
	IsActive     bool                         `json:"is_active"`
	mu           sync.RWMutex                 `json:"-"`
}

// RegistryConfig holds configuration for the pair registry
type RegistryConfig struct {
	MetricsUpdateInterval time.Duration `json:"metrics_update_interval"`
	CleanupInterval       time.Duration `json:"cleanup_interval"`
	MaxPairs              int           `json:"max_pairs"`
	EnableCrossRouting    bool          `json:"enable_cross_routing"`
	MaxRoutingDepth       int           `json:"max_routing_depth"`
	CacheExpiryTime       time.Duration `json:"cache_expiry_time"`
}

// LatencyTracker tracks lookup latency with sliding window
type LatencyTracker struct {
	samples []time.Duration
	index   int
	mu      sync.Mutex
	size    int
}

// DefaultRegistryConfig returns default configuration
func DefaultRegistryConfig() *RegistryConfig {
	return &RegistryConfig{
		MetricsUpdateInterval: 30 * time.Second,
		CleanupInterval:       5 * time.Minute,
		MaxPairs:              10000,
		EnableCrossRouting:    true,
		MaxRoutingDepth:       3,
		CacheExpiryTime:       1 * time.Hour,
	}
}

// NewHighPerformancePairRegistry creates a new high-performance pair registry
func NewHighPerformancePairRegistry(config *RegistryConfig, logger *zap.Logger) *HighPerformancePairRegistry {
	if config == nil {
		config = DefaultRegistryConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	registry := &HighPerformancePairRegistry{
		config:        config,
		logger:        logger.Named("pair_registry"),
		ctx:           ctx,
		cancel:        cancel,
		lookupLatency: NewLatencyTracker(1000), // Track last 1000 lookups
		metrics: &PairRegistryMetrics{
			LastUpdateTime: time.Now(),
		},
	}

	atomic.StoreInt64(&registry.running, 1)

	// Start background routines
	go registry.metricsUpdater()
	go registry.cleanupRoutine()

	registry.logger.Info("High-performance pair registry initialized",
		zap.Int("max_pairs", config.MaxPairs),
		zap.Bool("cross_routing_enabled", config.EnableCrossRouting))

	return registry
}

// RegisterPair registers a new trading pair with its orderbook
func (hpr *HighPerformancePairRegistry) RegisterPair(ctx context.Context, pair *models.TradingPair, ob orderbook.OrderBookInterface) error {
	if atomic.LoadInt64(&hpr.running) == 0 {
		return fmt.Errorf("registry is shutting down")
	}

	if pair == nil {
		return fmt.Errorf("trading pair cannot be nil")
	}

	if ob == nil {
		return fmt.Errorf("orderbook cannot be nil")
	}

	symbol := pair.Symbol
	if symbol == "" {
		return fmt.Errorf("trading pair symbol cannot be empty")
	}

	// Check if pair already exists
	if _, exists := hpr.pairs.Load(symbol); exists {
		return fmt.Errorf("pair %s is already registered", symbol)
	}

	// Check max pairs limit
	hpr.metricsMu.RLock()
	totalPairs := hpr.metrics.TotalPairs
	hpr.metricsMu.RUnlock()

	if totalPairs >= int64(hpr.config.MaxPairs) {
		return fmt.Errorf("maximum pairs limit (%d) reached", hpr.config.MaxPairs)
	}

	// Create registered pair entry
	regPair := &registeredPair{
		TradingPair:  pair,
		OrderBook:    ob,
		RegisteredAt: time.Now(),
		LastActivity: time.Now(),
		IsActive:     pair.Status == models.TradingStatusActive,
	}

	// Store pair and orderbook with atomic operations
	hpr.pairs.Store(symbol, regPair)
	hpr.orderBooks.Store(symbol, ob)

	// Update indices for fast lookups
	hpr.updateIndices(pair, true)

	// Update metrics
	hpr.metricsMu.Lock()
	hpr.metrics.TotalPairs++
	if regPair.IsActive {
		hpr.metrics.ActivePairs++
	}
	hpr.metrics.LastUpdateTime = time.Now()
	hpr.metricsMu.Unlock()

	hpr.logger.Info("Registered trading pair",
		zap.String("symbol", symbol), zap.String("base_currency", pair.BaseCurrency),
		zap.String("quote_currency", pair.QuoteCurrency),
		zap.String("status", string(pair.Status)))

	return nil
}

// GetOrderBook retrieves the orderbook for a specific pair (O(1) lookup)
func (hpr *HighPerformancePairRegistry) GetOrderBook(symbol string) (orderbook.OrderBookInterface, error) {
	start := time.Now()
	defer func() {
		hpr.lookupLatency.AddSample(time.Since(start))
		atomic.AddInt64(&hpr.metrics.TotalLookups, 1)
	}()

	if atomic.LoadInt64(&hpr.running) == 0 {
		atomic.AddInt64(&hpr.metrics.FailedLookups, 1)
		return nil, fmt.Errorf("registry is shutting down")
	}

	if symbol == "" {
		atomic.AddInt64(&hpr.metrics.FailedLookups, 1)
		return nil, fmt.Errorf("symbol cannot be empty")
	}

	// O(1) lookup
	ob, exists := hpr.orderBooks.Load(symbol)
	if !exists {
		atomic.AddInt64(&hpr.metrics.FailedLookups, 1)
		return nil, fmt.Errorf("orderbook not found for pair: %s", symbol)
	}

	// Update last activity
	if pairValue, pairExists := hpr.pairs.Load(symbol); pairExists {
		regPair := pairValue.(*registeredPair)
		regPair.mu.Lock()
		regPair.LastActivity = time.Now()
		regPair.mu.Unlock()
	}

	atomic.AddInt64(&hpr.metrics.SuccessfulLookups, 1)
	return ob.(orderbook.OrderBookInterface), nil
}

// IsPairAvailable checks if a trading pair is available for trading (O(1) lookup)
func (hpr *HighPerformancePairRegistry) IsPairAvailable(symbol string) bool {
	if atomic.LoadInt64(&hpr.running) == 0 {
		return false
	}

	pairValue, exists := hpr.pairs.Load(symbol)
	if !exists {
		return false
	}

	regPair := pairValue.(*registeredPair)
	regPair.mu.RLock()
	isActive := regPair.IsActive && regPair.TradingPair.Status == models.TradingStatusActive
	regPair.mu.RUnlock()

	return isActive
}

// ListPairs returns all available trading pairs with optional filtering
func (hpr *HighPerformancePairRegistry) ListPairs(ctx context.Context, filter *PairFilter) ([]*models.TradingPair, error) {
	if atomic.LoadInt64(&hpr.running) == 0 {
		return nil, fmt.Errorf("registry is shutting down")
	}

	var pairs []*models.TradingPair

	hpr.pairs.Range(func(key, value interface{}) bool {
		select {
		case <-ctx.Done():
			return false
		default:
		}

		regPair := value.(*registeredPair)
		regPair.mu.RLock()
		pair := regPair.TradingPair
		regPair.mu.RUnlock()

		// Apply filters
		if filter != nil {
			if filter.BaseAsset != "" && pair.BaseCurrency != filter.BaseAsset {
				return true
			}
			if filter.QuoteAsset != "" && pair.QuoteCurrency != filter.QuoteAsset {
				return true
			}
			if filter.Status != "" && pair.Status != filter.Status {
				return true
			}
			// TODO: Add volume filtering when Volume24h field is available
			// if !filter.MinVolume.IsZero() && pair.Volume24h.LessThan(filter.MinVolume) {
			//     return true
			// }
		}

		pairs = append(pairs, pair)
		return true
	})

	// Sort by symbol for consistent output
	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i].Symbol < pairs[j].Symbol
	})

	// Apply pagination
	if filter != nil && (filter.Limit > 0 || filter.Offset > 0) {
		start := filter.Offset
		if start >= len(pairs) {
			return []*models.TradingPair{}, nil
		}

		end := len(pairs)
		if filter.Limit > 0 && start+filter.Limit < end {
			end = start + filter.Limit
		}

		pairs = pairs[start:end]
	}

	return pairs, nil
}

// FindCommonBase finds pairs that share a common base currency for cross-pair routing
func (hpr *HighPerformancePairRegistry) FindCommonBase(fromSymbol, toSymbol string) ([]CrossPairRoute, error) {
	if atomic.LoadInt64(&hpr.running) == 0 {
		return nil, fmt.Errorf("registry is shutting down")
	}

	if !hpr.config.EnableCrossRouting {
		return []CrossPairRoute{}, nil
	}

	if fromSymbol == toSymbol {
		return []CrossPairRoute{}, nil
	}

	// Get pair information
	fromPairValue, fromExists := hpr.pairs.Load(fromSymbol)
	if !fromExists {
		return nil, fmt.Errorf("from pair %s not found", fromSymbol)
	}

	toPairValue, toExists := hpr.pairs.Load(toSymbol)
	if !toExists {
		return nil, fmt.Errorf("to pair %s not found", toSymbol)
	}

	fromPair := fromPairValue.(*registeredPair).TradingPair
	toPair := toPairValue.(*registeredPair).TradingPair

	var routes []CrossPairRoute

	// Check for direct common base/quote currencies
	commonBases := []string{}
	// Case 1: fromPair.BaseCurrency == toPair.BaseCurrency
	if fromPair.BaseCurrency == toPair.BaseCurrency {
		commonBases = append(commonBases, fromPair.BaseCurrency)
	}

	// Case 2: fromPair.BaseCurrency == toPair.QuoteCurrency
	if fromPair.BaseCurrency == toPair.QuoteCurrency {
		commonBases = append(commonBases, fromPair.BaseCurrency)
	}

	// Case 3: fromPair.QuoteCurrency == toPair.BaseCurrency
	if fromPair.QuoteCurrency == toPair.BaseCurrency {
		commonBases = append(commonBases, fromPair.QuoteCurrency)
	}

	// Case 4: fromPair.QuoteCurrency == toPair.QuoteCurrency
	if fromPair.QuoteCurrency == toPair.QuoteCurrency {
		commonBases = append(commonBases, fromPair.QuoteCurrency)
	}

	// Find intermediate pairs for each common base
	for _, commonBase := range commonBases {
		// Find intermediate pairs that can connect fromSymbol and toSymbol
		intermediates := hpr.findIntermediatePairs(fromSymbol, toSymbol, commonBase)

		for _, intermediate := range intermediates {
			route := CrossPairRoute{
				FromPair:          fromSymbol,
				ToPair:            toSymbol,
				IntermediatePair:  intermediate,
				CommonBase:        commonBase,
				EstimatedSlippage: hpr.calculateEstimatedSlippage(fromSymbol, intermediate, toSymbol),
				LiquidityScore:    hpr.calculateLiquidityScore(fromSymbol, intermediate, toSymbol),
			}
			routes = append(routes, route)
		}
	}

	// Sort routes by liquidity score (higher is better)
	sort.Slice(routes, func(i, j int) bool {
		return routes[i].LiquidityScore > routes[j].LiquidityScore
	})

	return routes, nil
}

// GetPairMetrics returns performance metrics for monitoring
func (hpr *HighPerformancePairRegistry) GetPairMetrics() *PairRegistryMetrics {
	hpr.metricsMu.RLock()
	defer hpr.metricsMu.RUnlock()

	// Create a copy to avoid race conditions
	return &PairRegistryMetrics{
		TotalPairs:        hpr.metrics.TotalPairs,
		ActivePairs:       hpr.metrics.ActivePairs,
		TotalLookups:      hpr.metrics.TotalLookups,
		AverageLookupTime: hpr.lookupLatency.GetAverage(),
		SuccessfulLookups: hpr.metrics.SuccessfulLookups,
		FailedLookups:     hpr.metrics.FailedLookups,
		LastUpdateTime:    hpr.metrics.LastUpdateTime,
		MemoryUsage:       hpr.estimateMemoryUsage(),
	}
}

// UnregisterPair removes a pair from the registry
func (hpr *HighPerformancePairRegistry) UnregisterPair(symbol string) error {
	if atomic.LoadInt64(&hpr.running) == 0 {
		return fmt.Errorf("registry is shutting down")
	}

	pairValue, exists := hpr.pairs.Load(symbol)
	if !exists {
		return fmt.Errorf("pair %s not found", symbol)
	}

	regPair := pairValue.(*registeredPair)

	// Remove from main storage
	hpr.pairs.Delete(symbol)
	hpr.orderBooks.Delete(symbol)

	// Update indices
	hpr.updateIndices(regPair.TradingPair, false)

	// Update metrics
	hpr.metricsMu.Lock()
	hpr.metrics.TotalPairs--
	if regPair.IsActive {
		hpr.metrics.ActivePairs--
	}
	hpr.metrics.LastUpdateTime = time.Now()
	hpr.metricsMu.Unlock()

	hpr.logger.Info("Unregistered trading pair", zap.String("symbol", symbol))

	return nil
}

// UpdatePairStatus updates trading status for a pair
func (hpr *HighPerformancePairRegistry) UpdatePairStatus(symbol string, status models.TradingStatus) error {
	if atomic.LoadInt64(&hpr.running) == 0 {
		return fmt.Errorf("registry is shutting down")
	}

	pairValue, exists := hpr.pairs.Load(symbol)
	if !exists {
		return fmt.Errorf("pair %s not found", symbol)
	}

	regPair := pairValue.(*registeredPair)
	regPair.mu.Lock()
	oldStatus := regPair.TradingPair.Status
	regPair.TradingPair.Status = status
	regPair.IsActive = status == models.TradingStatusActive
	regPair.mu.Unlock()

	// Update active pairs count
	hpr.metricsMu.Lock()
	if oldStatus == models.TradingStatusActive && status != models.TradingStatusActive {
		hpr.metrics.ActivePairs--
	} else if oldStatus != models.TradingStatusActive && status == models.TradingStatusActive {
		hpr.metrics.ActivePairs++
	}
	hpr.metrics.LastUpdateTime = time.Now()
	hpr.metricsMu.Unlock()

	hpr.logger.Info("Updated pair status",
		zap.String("symbol", symbol),
		zap.String("old_status", string(oldStatus)),
		zap.String("new_status", string(status)))

	return nil
}

// GetActivePairs returns all pairs with ACTIVE status
func (hpr *HighPerformancePairRegistry) GetActivePairs() []string {
	var activePairs []string

	hpr.pairs.Range(func(key, value interface{}) bool {
		regPair := value.(*registeredPair)
		regPair.mu.RLock()
		if regPair.IsActive && regPair.TradingPair.Status == models.TradingStatusActive {
			activePairs = append(activePairs, key.(string))
		}
		regPair.mu.RUnlock()
		return true
	})

	sort.Strings(activePairs)
	return activePairs
}

// Shutdown gracefully shuts down the registry
func (hpr *HighPerformancePairRegistry) Shutdown(ctx context.Context) error {
	var shutdownErr error

	hpr.shutdownOnce.Do(func() {
		hpr.logger.Info("Shutting down high-performance pair registry")

		// Mark as not running
		atomic.StoreInt64(&hpr.running, 0)

		// Cancel background routines
		hpr.cancel()

		// Wait for shutdown with timeout
		done := make(chan struct{})
		go func() {
			defer close(done)
			// Additional cleanup can be added here
		}()

		select {
		case <-done:
			hpr.logger.Info("Pair registry shutdown completed")
		case <-ctx.Done():
			shutdownErr = fmt.Errorf("shutdown timeout: %w", ctx.Err())
		}
	})

	return shutdownErr
}

// Helper methods

// updateIndices updates base and quote asset indices for fast filtering
func (hpr *HighPerformancePairRegistry) updateIndices(pair *models.TradingPair, add bool) {
	symbol := pair.Symbol
	// Update base asset index
	hpr.updateAssetIndex(&hpr.baseAssetIndex, pair.BaseCurrency, symbol, add)

	// Update quote asset index
	hpr.updateAssetIndex(&hpr.quoteAssetIndex, pair.QuoteCurrency, symbol, add)
}

// updateAssetIndex updates a specific asset index
func (hpr *HighPerformancePairRegistry) updateAssetIndex(index *sync.Map, asset, symbol string, add bool) {
	if add {
		// Add symbol to asset index
		value, _ := index.LoadOrStore(asset, []string{})
		symbols := value.([]string)

		// Check if symbol already exists
		for _, s := range symbols {
			if s == symbol {
				return
			}
		}

		symbols = append(symbols, symbol)
		index.Store(asset, symbols)
	} else {
		// Remove symbol from asset index
		value, exists := index.Load(asset)
		if !exists {
			return
		}

		symbols := value.([]string)
		var newSymbols []string
		for _, s := range symbols {
			if s != symbol {
				newSymbols = append(newSymbols, s)
			}
		}

		if len(newSymbols) == 0 {
			index.Delete(asset)
		} else {
			index.Store(asset, newSymbols)
		}
	}
}

// findIntermediatePairs finds pairs that can serve as intermediates for cross-pair routing
func (hpr *HighPerformancePairRegistry) findIntermediatePairs(fromSymbol, toSymbol, commonBase string) []string {
	var intermediates []string

	// Look for pairs that have the common base as either base or quote asset
	if baseSymbols, exists := hpr.baseAssetIndex.Load(commonBase); exists {
		for _, symbol := range baseSymbols.([]string) {
			if symbol != fromSymbol && symbol != toSymbol && hpr.IsPairAvailable(symbol) {
				intermediates = append(intermediates, symbol)
			}
		}
	}

	if quoteSymbols, exists := hpr.quoteAssetIndex.Load(commonBase); exists {
		for _, symbol := range quoteSymbols.([]string) {
			if symbol != fromSymbol && symbol != toSymbol && hpr.IsPairAvailable(symbol) {
				// Avoid duplicates
				found := false
				for _, existing := range intermediates {
					if existing == symbol {
						found = true
						break
					}
				}
				if !found {
					intermediates = append(intermediates, symbol)
				}
			}
		}
	}

	return intermediates
}

// calculateEstimatedSlippage estimates slippage for a cross-pair route
func (hpr *HighPerformancePairRegistry) calculateEstimatedSlippage(fromSymbol, intermediateSymbol, toSymbol string) decimal.Decimal {
	// Simplified slippage estimation - in production this would use actual orderbook depth
	baseSlippage := decimal.NewFromFloat(0.001) // 0.1% base slippage

	// Add complexity penalty for routing
	if intermediateSymbol != "" {
		return baseSlippage.Mul(decimal.NewFromInt(2))
	}

	return baseSlippage
}

// calculateLiquidityScore calculates a liquidity score for cross-pair routing
func (hpr *HighPerformancePairRegistry) calculateLiquidityScore(fromSymbol, intermediateSymbol, toSymbol string) float64 {
	// Simplified liquidity scoring - in production this would analyze actual orderbook liquidity
	baseScore := 1.0

	// Penalty for using intermediate pairs
	if intermediateSymbol != "" {
		baseScore *= 0.8
	}

	// Bonus for active pairs
	if hpr.IsPairAvailable(fromSymbol) && hpr.IsPairAvailable(toSymbol) {
		baseScore *= 1.2
	}

	return baseScore
}

// estimateMemoryUsage estimates current memory usage
func (hpr *HighPerformancePairRegistry) estimateMemoryUsage() int64 {
	// Simplified memory estimation
	var memoryUsage int64

	hpr.pairs.Range(func(key, value interface{}) bool {
		// Estimate ~1KB per pair entry
		memoryUsage += 1024
		return true
	})

	return memoryUsage
}

// metricsUpdater periodically updates metrics
func (hpr *HighPerformancePairRegistry) metricsUpdater() {
	ticker := time.NewTicker(hpr.config.MetricsUpdateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-hpr.ctx.Done():
			return
		case <-ticker.C:
			hpr.updateMetrics()
		}
	}
}

// updateMetrics updates internal metrics
func (hpr *HighPerformancePairRegistry) updateMetrics() {
	hpr.metricsMu.Lock()
	defer hpr.metricsMu.Unlock()

	hpr.metrics.LastUpdateTime = time.Now()
	hpr.metrics.AverageLookupTime = hpr.lookupLatency.GetAverage()
	hpr.metrics.MemoryUsage = hpr.estimateMemoryUsage()
}

// cleanupRoutine performs periodic cleanup tasks
func (hpr *HighPerformancePairRegistry) cleanupRoutine() {
	ticker := time.NewTicker(hpr.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-hpr.ctx.Done():
			return
		case <-ticker.C:
			hpr.performCleanup()
		}
	}
}

// performCleanup performs cleanup tasks
func (hpr *HighPerformancePairRegistry) performCleanup() {
	// Reset lookup latency tracker periodically to prevent memory growth
	hpr.lookupLatency.Reset()

	hpr.logger.Debug("Performed registry cleanup")
}

// NewLatencyTracker creates a new latency tracker
func NewLatencyTracker(size int) *LatencyTracker {
	return &LatencyTracker{
		samples: make([]time.Duration, size),
		size:    size,
	}
}

// AddSample adds a latency sample
func (lt *LatencyTracker) AddSample(duration time.Duration) {
	lt.mu.Lock()
	defer lt.mu.Unlock()

	lt.samples[lt.index] = duration
	lt.index = (lt.index + 1) % lt.size
}

// GetAverage returns the average latency
func (lt *LatencyTracker) GetAverage() time.Duration {
	lt.mu.Lock()
	defer lt.mu.Unlock()

	var total time.Duration
	count := 0

	for _, sample := range lt.samples {
		if sample > 0 {
			total += sample
			count++
		}
	}

	if count == 0 {
		return 0
	}

	return total / time.Duration(count)
}

// Reset clears all samples
func (lt *LatencyTracker) Reset() {
	lt.mu.Lock()
	defer lt.mu.Unlock()

	for i := range lt.samples {
		lt.samples[i] = 0
	}
	lt.index = 0
}
