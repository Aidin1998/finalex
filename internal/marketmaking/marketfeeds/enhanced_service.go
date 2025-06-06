package marketfeeds

import (
	"context"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/Aidin1998/finalex/pkg/models"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// EnhancedService implements EnhancedMarketFeedService with advanced aggregation and monitoring
type EnhancedService struct {
	logger *zap.Logger
	db     *gorm.DB
	config *ServiceConfig
	pubsub PubSubBackend

	// Exchange providers
	exchangeProviders map[string]ExchangeProvider
	exchangeMutex     sync.RWMutex

	// WebSocket providers for real-time data
	wsProviders map[string]WebSocketProvider
	wsMutex     sync.RWMutex

	// Fiat rate providers
	fiatProviders map[string]FiatRateProvider
	fiatMutex     sync.RWMutex

	// Data storage
	aggregatedTickers    map[string]*AggregatedTicker
	aggregatedOrderBooks map[string]*AggregatedOrderBook
	fiatRates            map[string]*FiatRate
	dataMutex            sync.RWMutex

	// Quality monitoring
	feedQuality        map[string]*FeedQuality
	performanceMetrics *PerformanceMetrics
	alerts             []*Alert
	monitoringMutex    sync.RWMutex

	// Subscriptions
	tickerSubscriptions    map[string][]chan *AggregatedTicker
	orderBookSubscriptions map[string][]chan *AggregatedOrderBook
	alertSubscriptions     []chan *Alert
	subscriptionMutex      sync.RWMutex

	// Control
	stopChan  chan struct{}
	isRunning bool
	runMutex  sync.Mutex

	// Performance optimization
	requestCounter  int64
	successCounter  int64
	failureCounter  int64
	latencyRecorder []time.Duration
	perfMutex       sync.Mutex
}

// NewEnhancedService creates a new enhanced market feed service
func NewEnhancedService(logger *zap.Logger, db *gorm.DB, pubsub PubSubBackend, config *ServiceConfig) (*EnhancedService, error) {
	service := &EnhancedService{
		logger: logger,
		db:     db,
		config: config,
		pubsub: pubsub,

		exchangeProviders: make(map[string]ExchangeProvider),
		wsProviders:       make(map[string]WebSocketProvider),
		fiatProviders:     make(map[string]FiatRateProvider),

		aggregatedTickers:    make(map[string]*AggregatedTicker),
		aggregatedOrderBooks: make(map[string]*AggregatedOrderBook),
		fiatRates:            make(map[string]*FiatRate),

		feedQuality: make(map[string]*FeedQuality),
		performanceMetrics: &PerformanceMetrics{
			LastUpdated: time.Now(),
		},
		alerts: make([]*Alert, 0),

		tickerSubscriptions:    make(map[string][]chan *AggregatedTicker),
		orderBookSubscriptions: make(map[string][]chan *AggregatedOrderBook),
		alertSubscriptions:     make([]chan *Alert, 0),

		stopChan:        make(chan struct{}),
		latencyRecorder: make([]time.Duration, 0, 1000),
	}

	// Initialize exchange providers
	if err := service.initializeExchangeProviders(); err != nil {
		return nil, fmt.Errorf("failed to initialize exchange providers: %w", err)
	}

	// Initialize fiat rate providers
	if err := service.initializeFiatProviders(); err != nil {
		return nil, fmt.Errorf("failed to initialize fiat providers: %w", err)
	}

	return service, nil
}

// Start starts the enhanced market feeds service
func (s *EnhancedService) Start() error {
	s.runMutex.Lock()
	defer s.runMutex.Unlock()

	if s.isRunning {
		return fmt.Errorf("enhanced market feeds service is already running")
	}

	s.logger.Info("Starting enhanced market feeds service")

	// Start data collection routines
	go s.collectMarketData()
	go s.collectFiatRates()
	go s.performQualityMonitoring()
	go s.performHealthChecks()
	go s.cleanupOldData()

	s.isRunning = true
	s.logger.Info("Enhanced market feeds service started successfully")

	return nil
}

// Stop stops the enhanced market feeds service
func (s *EnhancedService) Stop() error {
	s.runMutex.Lock()
	defer s.runMutex.Unlock()

	if !s.isRunning {
		return fmt.Errorf("enhanced market feeds service is not running")
	}

	s.logger.Info("Stopping enhanced market feeds service")
	close(s.stopChan)
	s.isRunning = false

	// Close all subscription channels
	s.subscriptionMutex.Lock()
	for _, channels := range s.tickerSubscriptions {
		for _, ch := range channels {
			close(ch)
		}
	}
	for _, channels := range s.orderBookSubscriptions {
		for _, ch := range channels {
			close(ch)
		}
	}
	for _, ch := range s.alertSubscriptions {
		close(ch)
	}
	s.subscriptionMutex.Unlock()

	s.logger.Info("Enhanced market feeds service stopped")
	return nil
}

// GetMarketPrices returns market prices (legacy compatibility)
func (s *EnhancedService) GetMarketPrices(ctx context.Context) ([]*models.MarketPrice, error) {
	s.dataMutex.RLock()
	defer s.dataMutex.RUnlock()

	prices := make([]*models.MarketPrice, 0, len(s.aggregatedTickers))
	for _, ticker := range s.aggregatedTickers {
		price := &models.MarketPrice{
			Symbol:    ticker.Symbol,
			Price:     ticker.Price.InexactFloat64(),
			Change24h: ticker.PriceChangePercent.InexactFloat64(),
			Volume24h: ticker.Volume.InexactFloat64(),
			High24h:   ticker.HighPrice.InexactFloat64(),
			Low24h:    ticker.LowPrice.InexactFloat64(),
			UpdatedAt: ticker.UpdatedAt,
		}
		prices = append(prices, price)
	}

	return prices, nil
}

// GetMarketPrice returns a single market price (legacy compatibility)
func (s *EnhancedService) GetMarketPrice(ctx context.Context, symbol string) (*models.MarketPrice, error) {
	s.dataMutex.RLock()
	ticker, exists := s.aggregatedTickers[symbol]
	s.dataMutex.RUnlock()

	if !exists {
		return nil, fmt.Errorf("market price not found for symbol %s", symbol)
	}

	return &models.MarketPrice{
		Symbol:    ticker.Symbol,
		Price:     ticker.Price.InexactFloat64(),
		Change24h: ticker.PriceChangePercent.InexactFloat64(),
		Volume24h: ticker.Volume.InexactFloat64(),
		High24h:   ticker.HighPrice.InexactFloat64(),
		Low24h:    ticker.LowPrice.InexactFloat64(),
		UpdatedAt: ticker.UpdatedAt,
	}, nil
}

// GetCandles returns candlestick data (legacy compatibility)
func (s *EnhancedService) GetCandles(ctx context.Context, symbol, interval string, limit int) ([]*models.Candle, error) {
	// Get klines from the first available exchange
	s.exchangeMutex.RLock()
	var provider ExchangeProvider
	for _, p := range s.exchangeProviders {
		if p.IsHealthy() {
			provider = p
			break
		}
	}
	s.exchangeMutex.RUnlock()

	if provider == nil {
		return nil, fmt.Errorf("no healthy exchange providers available")
	}

	klines, err := provider.GetKlines(ctx, symbol, interval, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get klines from %s: %w", provider.GetName(), err)
	}
	candles := make([]*models.Candle, len(klines))
	for i, kline := range klines {
		candles[i] = &models.Candle{
			Timestamp: time.Unix(kline.OpenTime/1000, 0),
			Open:      kline.Open.InexactFloat64(),
			High:      kline.High.InexactFloat64(),
			Low:       kline.Low.InexactFloat64(),
			Close:     kline.Close.InexactFloat64(),
			Volume:    kline.Volume.InexactFloat64(),
		}
	}

	return candles, nil
}

// GetAggregatedTicker returns aggregated ticker data
func (s *EnhancedService) GetAggregatedTicker(ctx context.Context, symbol string) (*AggregatedTicker, error) {
	s.dataMutex.RLock()
	ticker, exists := s.aggregatedTickers[symbol]
	s.dataMutex.RUnlock()

	if !exists {
		return nil, fmt.Errorf("aggregated ticker not found for symbol %s", symbol)
	}

	// Return a copy to prevent external modification
	tickerCopy := *ticker
	return &tickerCopy, nil
}

// GetAggregatedOrderBook returns aggregated order book data
func (s *EnhancedService) GetAggregatedOrderBook(ctx context.Context, symbol string, depth int) (*AggregatedOrderBook, error) {
	s.dataMutex.RLock()
	orderBook, exists := s.aggregatedOrderBooks[symbol]
	s.dataMutex.RUnlock()

	if !exists {
		return nil, fmt.Errorf("aggregated order book not found for symbol %s", symbol)
	}

	// Return a copy with limited depth
	orderBookCopy := *orderBook
	if depth > 0 && depth < len(orderBook.Bids) {
		orderBookCopy.Bids = orderBook.Bids[:depth]
	}
	if depth > 0 && depth < len(orderBook.Asks) {
		orderBookCopy.Asks = orderBook.Asks[:depth]
	}

	return &orderBookCopy, nil
}

// GetExchangeTickers returns ticker data from all exchanges
func (s *EnhancedService) GetExchangeTickers(ctx context.Context, symbol string) (map[string]*TickerData, error) {
	s.dataMutex.RLock()
	ticker, exists := s.aggregatedTickers[symbol]
	s.dataMutex.RUnlock()

	if !exists {
		return nil, fmt.Errorf("ticker data not found for symbol %s", symbol)
	}

	// Return sources data
	sources := make(map[string]*TickerData)
	for name, tickerData := range ticker.Sources {
		// Return a copy to prevent external modification
		tickerCopy := *tickerData
		sources[name] = &tickerCopy
	}

	return sources, nil
}

// GetFiatRates returns all fiat currency rates
func (s *EnhancedService) GetFiatRates(ctx context.Context) (map[string]*FiatRate, error) {
	s.dataMutex.RLock()
	defer s.dataMutex.RUnlock()

	rates := make(map[string]*FiatRate)
	for currency, rate := range s.fiatRates {
		// Return a copy to prevent external modification
		rateCopy := *rate
		rates[currency] = &rateCopy
	}

	return rates, nil
}

// GetFiatRate returns a specific fiat currency rate
func (s *EnhancedService) GetFiatRate(ctx context.Context, currency string) (*FiatRate, error) {
	s.dataMutex.RLock()
	rate, exists := s.fiatRates[currency]
	s.dataMutex.RUnlock()

	if !exists {
		return nil, fmt.Errorf("fiat rate not found for currency %s", currency)
	}

	// Return a copy to prevent external modification
	rateCopy := *rate
	return &rateCopy, nil
}

// ConvertPrice converts amount from one currency to another
func (s *EnhancedService) ConvertPrice(ctx context.Context, amount decimal.Decimal, fromCurrency, toCurrency string) (decimal.Decimal, error) {
	if fromCurrency == toCurrency {
		return amount, nil
	}

	s.dataMutex.RLock()
	defer s.dataMutex.RUnlock()

	// Handle crypto to crypto conversions
	if (fromCurrency == "BTC" || fromCurrency == "USDT") && (toCurrency == "BTC" || toCurrency == "USDT") {
		if fromCurrency == "BTC" && toCurrency == "USDT" {
			// Get BTC price in USDT
			if ticker, exists := s.aggregatedTickers["BTCUSDT"]; exists {
				return amount.Mul(ticker.Price), nil
			}
		} else if fromCurrency == "USDT" && toCurrency == "BTC" {
			// Get BTC price in USDT and invert
			if ticker, exists := s.aggregatedTickers["BTCUSDT"]; exists {
				return amount.Div(ticker.Price), nil
			}
		}
	}

	// Handle crypto to fiat conversions
	if fromCurrency == "USDT" || fromCurrency == "BTC" {
		if rate, exists := s.fiatRates[toCurrency]; exists {
			if fromCurrency == "USDT" {
				return amount.Mul(rate.RateVsUSDT), nil
			} else {
				return amount.Mul(rate.RateVsBTC), nil
			}
		}
	}

	// Handle fiat to crypto conversions
	if toCurrency == "USDT" || toCurrency == "BTC" {
		if rate, exists := s.fiatRates[fromCurrency]; exists {
			if toCurrency == "USDT" && !rate.RateVsUSDT.IsZero() {
				return amount.Div(rate.RateVsUSDT), nil
			} else if toCurrency == "BTC" && !rate.RateVsBTC.IsZero() {
				return amount.Div(rate.RateVsBTC), nil
			}
		}
	}

	// Handle fiat to fiat conversions via USD
	if fromRate, fromExists := s.fiatRates[fromCurrency]; fromExists {
		if toRate, toExists := s.fiatRates[toCurrency]; toExists {
			// Convert to USD first, then to target currency
			usdAmount := amount.Div(fromRate.Rate)
			return usdAmount.Mul(toRate.Rate), nil
		}
	}

	return decimal.Zero, fmt.Errorf("conversion not supported from %s to %s", fromCurrency, toCurrency)
}

// GetFeedQuality returns feed quality metrics
func (s *EnhancedService) GetFeedQuality(ctx context.Context) (map[string]*FeedQuality, error) {
	s.monitoringMutex.RLock()
	defer s.monitoringMutex.RUnlock()

	quality := make(map[string]*FeedQuality)
	for source, feedQuality := range s.feedQuality {
		// Return a copy to prevent external modification
		qualityCopy := *feedQuality
		quality[source] = &qualityCopy
	}

	return quality, nil
}

// GetPerformanceMetrics returns performance metrics
func (s *EnhancedService) GetPerformanceMetrics(ctx context.Context) (*PerformanceMetrics, error) {
	s.monitoringMutex.RLock()
	defer s.monitoringMutex.RUnlock()

	// Return a copy to prevent external modification
	metricsCopy := *s.performanceMetrics
	return &metricsCopy, nil
}

// GetAlerts returns current alerts
func (s *EnhancedService) GetAlerts(ctx context.Context) ([]*Alert, error) {
	s.monitoringMutex.RLock()
	defer s.monitoringMutex.RUnlock()

	alerts := make([]*Alert, len(s.alerts))
	copy(alerts, s.alerts)

	return alerts, nil
}

// AddExchangeProvider adds a new exchange provider
func (s *EnhancedService) AddExchangeProvider(provider ExchangeProvider) error {
	s.exchangeMutex.Lock()
	defer s.exchangeMutex.Unlock()

	s.exchangeProviders[provider.GetName()] = provider
	s.logger.Info("Added exchange provider", zap.String("name", provider.GetName()))

	return nil
}

// RemoveExchangeProvider removes an exchange provider
func (s *EnhancedService) RemoveExchangeProvider(name string) error {
	s.exchangeMutex.Lock()
	defer s.exchangeMutex.Unlock()

	delete(s.exchangeProviders, name)
	s.logger.Info("Removed exchange provider", zap.String("name", name))

	return nil
}

// GetExchangeStatus returns the health status of all exchanges
func (s *EnhancedService) GetExchangeStatus(ctx context.Context) (map[string]bool, error) {
	s.exchangeMutex.RLock()
	defer s.exchangeMutex.RUnlock()

	status := make(map[string]bool)
	for name, provider := range s.exchangeProviders {
		status[name] = provider.IsHealthy()
	}

	return status, nil
}

// AddFiatRateProvider adds a new fiat rate provider
func (s *EnhancedService) AddFiatRateProvider(provider FiatRateProvider) error {
	s.fiatMutex.Lock()
	defer s.fiatMutex.Unlock()

	s.fiatProviders[provider.GetName()] = provider
	s.logger.Info("Added fiat rate provider", zap.String("name", provider.GetName()))

	return nil
}

// RemoveFiatRateProvider removes a fiat rate provider
func (s *EnhancedService) RemoveFiatRateProvider(name string) error {
	s.fiatMutex.Lock()
	defer s.fiatMutex.Unlock()

	delete(s.fiatProviders, name)
	s.logger.Info("Removed fiat rate provider", zap.String("name", name))

	return nil
}

// SubscribeToTicker subscribes to ticker updates
func (s *EnhancedService) SubscribeToTicker(ctx context.Context, symbol string) (<-chan *AggregatedTicker, error) {
	s.subscriptionMutex.Lock()
	defer s.subscriptionMutex.Unlock()

	ch := make(chan *AggregatedTicker, 100) // Buffered channel

	if s.tickerSubscriptions[symbol] == nil {
		s.tickerSubscriptions[symbol] = make([]chan *AggregatedTicker, 0)
	}
	s.tickerSubscriptions[symbol] = append(s.tickerSubscriptions[symbol], ch)

	return ch, nil
}

// SubscribeToOrderBook subscribes to order book updates
func (s *EnhancedService) SubscribeToOrderBook(ctx context.Context, symbol string) (<-chan *AggregatedOrderBook, error) {
	s.subscriptionMutex.Lock()
	defer s.subscriptionMutex.Unlock()

	ch := make(chan *AggregatedOrderBook, 100) // Buffered channel

	if s.orderBookSubscriptions[symbol] == nil {
		s.orderBookSubscriptions[symbol] = make([]chan *AggregatedOrderBook, 0)
	}
	s.orderBookSubscriptions[symbol] = append(s.orderBookSubscriptions[symbol], ch)

	return ch, nil
}

// SubscribeToAlerts subscribes to alert notifications
func (s *EnhancedService) SubscribeToAlerts(ctx context.Context) (<-chan *Alert, error) {
	s.subscriptionMutex.Lock()
	defer s.subscriptionMutex.Unlock()

	ch := make(chan *Alert, 100) // Buffered channel
	s.alertSubscriptions = append(s.alertSubscriptions, ch)

	return ch, nil
}

// UpdateConfig updates the service configuration
func (s *EnhancedService) UpdateConfig(config *ServiceConfig) error {
	s.config = config
	s.logger.Info("Updated service configuration")
	return nil
}

// GetConfig returns the current service configuration
func (s *EnhancedService) GetConfig() *ServiceConfig {
	return s.config
}

// HealthCheck performs a health check on the service
func (s *EnhancedService) HealthCheck(ctx context.Context) error {
	// Check if service is running
	if !s.isRunning {
		return fmt.Errorf("service is not running")
	}

	// Check exchange providers
	s.exchangeMutex.RLock()
	healthyExchanges := 0
	for _, provider := range s.exchangeProviders {
		if provider.IsHealthy() {
			healthyExchanges++
		}
	}
	totalExchanges := len(s.exchangeProviders)
	s.exchangeMutex.RUnlock()

	if healthyExchanges == 0 {
		return fmt.Errorf("no healthy exchange providers available")
	}

	// Check fiat providers
	s.fiatMutex.RLock()
	healthyFiatProviders := 0
	for _, provider := range s.fiatProviders {
		if provider.IsHealthy() {
			healthyFiatProviders++
		}
	}
	totalFiatProviders := len(s.fiatProviders)
	s.fiatMutex.RUnlock()

	s.logger.Info("Health check completed",
		zap.Int("healthy_exchanges", healthyExchanges),
		zap.Int("total_exchanges", totalExchanges),
		zap.Int("healthy_fiat_providers", healthyFiatProviders),
		zap.Int("total_fiat_providers", totalFiatProviders),
	)

	return nil
}

// Internal helper methods

// initializeExchangeProviders initializes exchange providers based on configuration
func (s *EnhancedService) initializeExchangeProviders() error {
	for _, exchangeConfig := range s.config.Exchanges {
		if !exchangeConfig.Enabled {
			continue
		}

		var provider ExchangeProvider

		switch exchangeConfig.Name {
		case "binance":
			provider = NewBinanceProvider(s.logger, &exchangeConfig)
		case "coinbase":
			provider = NewCoinbaseProvider(s.logger, &exchangeConfig)
		case "kraken":
			provider = NewKrakenProvider(s.logger, &exchangeConfig)
		case "okx":
			provider = NewOKXProvider(s.logger, &exchangeConfig)
		case "bitstamp":
			provider = NewBitstampProvider(s.logger, &exchangeConfig)
		default:
			s.logger.Warn("Unknown exchange provider", zap.String("name", exchangeConfig.Name))
			continue
		}

		s.exchangeProviders[exchangeConfig.Name] = provider
		s.logger.Info("Initialized exchange provider", zap.String("name", exchangeConfig.Name))
	}

	return nil
}

// initializeFiatProviders initializes fiat rate providers
func (s *EnhancedService) initializeFiatProviders() error {
	// Initialize ExchangeRates API provider
	if apiKey := getEnvVar("EXCHANGERATES_API_KEY", ""); apiKey != "" {
		provider := NewExchangeRatesAPIProvider(s.logger, apiKey)
		s.fiatProviders[provider.GetName()] = provider
		s.logger.Info("Initialized fiat provider", zap.String("name", provider.GetName()))
	}

	// Initialize CoinGecko provider
	provider := NewCoinGeckoFiatProvider(s.logger)
	s.fiatProviders[provider.GetName()] = provider
	s.logger.Info("Initialized fiat provider", zap.String("name", provider.GetName()))

	// Initialize CurrencyLayer provider
	if apiKey := getEnvVar("CURRENCYLAYER_API_KEY", ""); apiKey != "" {
		provider := NewCurrencyLayerProvider(s.logger, apiKey)
		s.fiatProviders[provider.GetName()] = provider
		s.logger.Info("Initialized fiat provider", zap.String("name", provider.GetName()))
	}

	return nil
}

// collectMarketData collects market data from all exchange providers
func (s *EnhancedService) collectMarketData() {
	ticker := time.NewTicker(s.config.UpdateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.updateMarketData()
		}
	}
}

// updateMarketData updates market data from all exchanges
func (s *EnhancedService) updateMarketData() {
	start := time.Now()

	// Get trading pairs from database
	var pairs []*models.TradingPair
	if err := s.db.Where("status = ?", "active").Find(&pairs).Error; err != nil {
		s.logger.Error("Failed to load trading pairs", zap.Error(err))
		return
	}

	// Collect data from all exchanges concurrently
	var wg sync.WaitGroup

	for _, pair := range pairs {
		wg.Add(1)
		go func(symbol string) {
			defer wg.Done()
			s.updateSymbolData(symbol)
		}(pair.Symbol)
	}

	wg.Wait()

	// Update performance metrics
	s.updatePerformanceMetrics(time.Since(start))

	s.logger.Debug("Market data update completed", zap.Duration("duration", time.Since(start)))
}

// updateSymbolData updates data for a specific symbol
func (s *EnhancedService) updateSymbolData(symbol string) {
	s.exchangeMutex.RLock()
	providers := make([]ExchangeProvider, 0, len(s.exchangeProviders))
	for _, provider := range s.exchangeProviders {
		providers = append(providers, provider)
	}
	s.exchangeMutex.RUnlock()

	// Sort providers by priority
	sort.Slice(providers, func(i, j int) bool {
		return providers[i].GetPriority() < providers[j].GetPriority()
	})

	// Collect ticker data from all exchanges
	tickerData := make(map[string]*TickerData)
	orderBookData := make(map[string]*OrderBookData)

	for _, provider := range providers {
		if !provider.IsHealthy() {
			continue
		}

		// Get ticker data
		if ticker, err := provider.GetTicker(context.Background(), symbol); err == nil {
			tickerData[provider.GetName()] = ticker
			s.updateFeedQuality(provider.GetName(), symbol, time.Now(), nil)
		} else {
			s.updateFeedQuality(provider.GetName(), symbol, time.Now(), err)
		}

		// Get order book data
		if orderBook, err := provider.GetOrderBook(context.Background(), symbol, 20); err == nil {
			orderBookData[provider.GetName()] = orderBook
		}
	}

	// Aggregate ticker data
	if len(tickerData) > 0 {
		aggregatedTicker := s.aggregateTickerData(symbol, tickerData)

		s.dataMutex.Lock()
		s.aggregatedTickers[symbol] = aggregatedTicker
		s.dataMutex.Unlock()

		// Notify subscribers
		s.notifyTickerSubscribers(symbol, aggregatedTicker)

		// Publish to pubsub
		if s.pubsub != nil {
			s.pubsub.Publish(context.Background(), fmt.Sprintf("ticker:%s", symbol), aggregatedTicker)
		}
	}

	// Aggregate order book data
	if len(orderBookData) > 0 {
		aggregatedOrderBook := s.aggregateOrderBookData(symbol, orderBookData)

		s.dataMutex.Lock()
		s.aggregatedOrderBooks[symbol] = aggregatedOrderBook
		s.dataMutex.Unlock()

		// Notify subscribers
		s.notifyOrderBookSubscribers(symbol, aggregatedOrderBook)
	}
}

// aggregateTickerData aggregates ticker data from multiple exchanges
func (s *EnhancedService) aggregateTickerData(symbol string, tickerData map[string]*TickerData) *AggregatedTicker {
	if len(tickerData) == 0 {
		return nil
	}

	// Calculate volume-weighted average price
	totalVolume := decimal.Zero
	weightedPriceSum := decimal.Zero

	var highPrice, lowPrice decimal.Decimal
	var totalVolumeBase, totalVolumeQuote decimal.Decimal
	bestBid, bestAsk := decimal.Zero, decimal.NewFromInt(999999999)

	first := true

	for exchangeName, ticker := range tickerData {
		// Get exchange weight
		weight := s.getExchangeWeight(exchangeName)

		// Volume-weighted price calculation
		volume := ticker.Volume.Mul(decimal.NewFromFloat(weight))
		totalVolume = totalVolume.Add(volume)
		weightedPriceSum = weightedPriceSum.Add(ticker.Price.Mul(volume))

		// Track high/low
		if first || ticker.HighPrice.GreaterThan(highPrice) {
			highPrice = ticker.HighPrice
		}
		if first || ticker.LowPrice.LessThan(lowPrice) {
			lowPrice = ticker.LowPrice
		}

		// Best bid/ask
		if ticker.BidPrice.GreaterThan(bestBid) {
			bestBid = ticker.BidPrice
		}
		if ticker.AskPrice.LessThan(bestAsk) {
			bestAsk = ticker.AskPrice
		}

		// Total volumes
		totalVolumeBase = totalVolumeBase.Add(ticker.Volume)
		totalVolumeQuote = totalVolumeQuote.Add(ticker.QuoteVolume)

		first = false
	}

	var avgPrice decimal.Decimal
	if !totalVolume.IsZero() {
		avgPrice = weightedPriceSum.Div(totalVolume)
	}

	// Calculate spread
	spread := decimal.Zero
	if !bestAsk.IsZero() && !bestBid.IsZero() {
		spread = bestAsk.Sub(bestBid)
	}

	// Calculate quality score
	qualityScore := s.calculateQualityScore(symbol, tickerData)

	// Calculate data freshness
	dataFreshness := s.calculateDataFreshness(tickerData)

	return &AggregatedTicker{
		Symbol:             symbol,
		Price:              avgPrice,
		PriceChange:        s.calculateAveragePriceChange(tickerData),
		PriceChangePercent: s.calculateAveragePriceChangePercent(tickerData),
		WeightedAvgPrice:   avgPrice,
		BidPrice:           bestBid,
		AskPrice:           bestAsk,
		Spread:             spread,
		HighPrice:          highPrice,
		LowPrice:           lowPrice,
		Volume:             totalVolumeBase,
		QuoteVolume:        totalVolumeQuote,
		UpdatedAt:          time.Now(),
		Sources:            tickerData,
		QualityScore:       qualityScore,
		DataFreshness:      dataFreshness,
	}
}

// aggregateOrderBookData aggregates order book data from multiple exchanges
func (s *EnhancedService) aggregateOrderBookData(symbol string, orderBookData map[string]*OrderBookData) *AggregatedOrderBook {
	if len(orderBookData) == 0 {
		return nil
	}

	// Merge all bids and asks
	allBids := make([]PriceLevel, 0)
	allAsks := make([]PriceLevel, 0)

	for _, orderBook := range orderBookData {
		allBids = append(allBids, orderBook.Bids...)
		allAsks = append(allAsks, orderBook.Asks...)
	}

	// Sort bids (descending) and asks (ascending)
	sort.Slice(allBids, func(i, j int) bool {
		return allBids[i].Price.GreaterThan(allBids[j].Price)
	})
	sort.Slice(allAsks, func(i, j int) bool {
		return allAsks[i].Price.LessThan(allAsks[j].Price)
	})

	// Aggregate by price level
	aggregatedBids := s.aggregatePriceLevels(allBids, false)
	aggregatedAsks := s.aggregatePriceLevels(allAsks, true)

	// Calculate mid price and spread
	var midPrice, spread decimal.Decimal
	if len(aggregatedBids) > 0 && len(aggregatedAsks) > 0 {
		bestBid := aggregatedBids[0].Price
		bestAsk := aggregatedAsks[0].Price
		midPrice = bestBid.Add(bestAsk).Div(decimal.NewFromInt(2))
		spread = bestAsk.Sub(bestBid)
	}

	// Calculate quality score
	qualityScore := s.calculateOrderBookQualityScore(symbol, orderBookData)

	return &AggregatedOrderBook{
		Symbol:       symbol,
		Bids:         aggregatedBids,
		Asks:         aggregatedAsks,
		Spread:       spread,
		MidPrice:     midPrice,
		UpdatedAt:    time.Now(),
		Sources:      orderBookData,
		QualityScore: qualityScore,
	}
}

// aggregatePriceLevels aggregates price levels with same price
func (s *EnhancedService) aggregatePriceLevels(levels []PriceLevel, isAsk bool) []PriceLevel {
	if len(levels) == 0 {
		return levels
	}

	aggregated := make([]PriceLevel, 0)
	currentPrice := levels[0].Price
	currentQuantity := levels[0].Quantity

	for i := 1; i < len(levels); i++ {
		if levels[i].Price.Equal(currentPrice) {
			currentQuantity = currentQuantity.Add(levels[i].Quantity)
		} else {
			aggregated = append(aggregated, PriceLevel{
				Price:    currentPrice,
				Quantity: currentQuantity,
			})
			currentPrice = levels[i].Price
			currentQuantity = levels[i].Quantity
		}
	}

	// Add the last level
	aggregated = append(aggregated, PriceLevel{
		Price:    currentPrice,
		Quantity: currentQuantity,
	})

	// Limit depth to 20 levels
	if len(aggregated) > 20 {
		aggregated = aggregated[:20]
	}

	return aggregated
}

// collectFiatRates collects fiat currency rates
func (s *EnhancedService) collectFiatRates() {
	ticker := time.NewTicker(s.config.Fiat.UpdateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.updateFiatRates()
		}
	}
}

// updateFiatRates updates fiat currency rates from all providers
func (s *EnhancedService) updateFiatRates() {
	s.fiatMutex.RLock()
	providers := make([]FiatRateProvider, 0, len(s.fiatProviders))
	for _, provider := range s.fiatProviders {
		providers = append(providers, provider)
	}
	s.fiatMutex.RUnlock()

	// Get rates from all providers
	allRates := make(map[string]map[string]decimal.Decimal)

	for _, provider := range providers {
		if !provider.IsHealthy() {
			continue
		}

		rates, err := provider.GetRates(context.Background(), Top10FiatCurrencies)
		if err != nil {
			s.logger.Error("Failed to get fiat rates",
				zap.String("provider", provider.GetName()),
				zap.Error(err))
			continue
		}

		allRates[provider.GetName()] = rates
	}

	// Aggregate rates and calculate BTC/USDT rates
	s.aggregateFiatRates(allRates)
}

// aggregateFiatRates aggregates fiat rates from multiple providers
func (s *EnhancedService) aggregateFiatRates(allRates map[string]map[string]decimal.Decimal) {
	aggregatedRates := make(map[string]*FiatRate)

	for _, currency := range Top10FiatCurrencies {
		rates := make([]decimal.Decimal, 0)

		// Collect rates from all providers
		for _, providerRates := range allRates {
			if rate, exists := providerRates[currency]; exists && !rate.IsZero() {
				rates = append(rates, rate)
			}
		}

		if len(rates) == 0 {
			continue
		}

		// Calculate average rate
		sum := decimal.Zero
		for _, rate := range rates {
			sum = sum.Add(rate)
		}
		avgRate := sum.Div(decimal.NewFromInt(int64(len(rates))))
		// Get BTC/USDT prices for conversion
		btcPrice := decimal.Zero

		s.dataMutex.RLock()
		if btcTicker, exists := s.aggregatedTickers["BTCUSDT"]; exists {
			btcPrice = btcTicker.Price
		}
		s.dataMutex.RUnlock()

		// Calculate rates vs BTC and USDT
		rateVsBTC := decimal.Zero
		rateVsUSDT := avgRate // For most fiat currencies, rate vs USDT ≈ rate vs USD

		if !btcPrice.IsZero() {
			rateVsBTC = avgRate.Div(btcPrice)
		}

		aggregatedRates[currency] = &FiatRate{
			Currency:   currency,
			Rate:       avgRate,
			RateVsBTC:  rateVsBTC,
			RateVsUSDT: rateVsUSDT,
			Change24h:  s.calculateFiatRateChange(currency, avgRate),
			UpdatedAt:  time.Now(),
			Source:     "aggregated",
		}
	}

	// Update stored rates
	s.dataMutex.Lock()
	s.fiatRates = aggregatedRates
	s.dataMutex.Unlock()
}

// performQualityMonitoring performs feed quality monitoring
func (s *EnhancedService) performQualityMonitoring() {
	ticker := time.NewTicker(30 * time.Second) // Monitor every 30 seconds
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.checkFeedQuality()
		}
	}
}

// checkFeedQuality checks the quality of all feeds and generates alerts
func (s *EnhancedService) checkFeedQuality() {
	s.monitoringMutex.Lock()
	defer s.monitoringMutex.Unlock()
	// Check exchange feed quality
	s.exchangeMutex.RLock()
	for name := range s.exchangeProviders {
		if quality, exists := s.feedQuality[name]; exists {
			// Check thresholds and generate alerts
			s.checkQualityThresholds(name, quality)
		}
	}
	s.exchangeMutex.RUnlock()

	// Check fiat provider quality
	s.fiatMutex.RLock()
	for name, provider := range s.fiatProviders {
		if !provider.IsHealthy() {
			s.generateAlert("fiat_provider_unhealthy", name, "", "Fiat provider is unhealthy", "medium")
		}
	}
	s.fiatMutex.RUnlock()
}

// performHealthChecks performs periodic health checks
func (s *EnhancedService) performHealthChecks() {
	ticker := time.NewTicker(s.config.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.runHealthChecks()
		}
	}
}

// runHealthChecks runs health checks on all providers
func (s *EnhancedService) runHealthChecks() {
	// Check exchange providers
	s.exchangeMutex.RLock()
	for name, provider := range s.exchangeProviders {
		if !provider.IsHealthy() {
			s.generateAlert("exchange_unhealthy", name, "", fmt.Sprintf("Exchange %s is unhealthy", name), "high")
		}
	}
	s.exchangeMutex.RUnlock()

	// Check fiat providers
	s.fiatMutex.RLock()
	for name, provider := range s.fiatProviders {
		if !provider.IsHealthy() {
			s.generateAlert("fiat_provider_unhealthy", name, "", fmt.Sprintf("Fiat provider %s is unhealthy", name), "medium")
		}
	}
	s.fiatMutex.RUnlock()
}

// cleanupOldData cleans up old data periodically
func (s *EnhancedService) cleanupOldData() {
	ticker := time.NewTicker(10 * time.Minute) // Cleanup every 10 minutes
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.performDataCleanup()
		}
	}
}

// performDataCleanup removes old data and alerts
func (s *EnhancedService) performDataCleanup() {
	now := time.Now()
	maxAge := s.config.MaxDataAge

	s.monitoringMutex.Lock()
	defer s.monitoringMutex.Unlock()

	// Clean up old alerts
	filteredAlerts := make([]*Alert, 0)
	for _, alert := range s.alerts {
		if now.Sub(alert.Timestamp) <= 24*time.Hour { // Keep alerts for 24 hours
			filteredAlerts = append(filteredAlerts, alert)
		}
	}
	s.alerts = filteredAlerts
	// Clean up old ticker data based on maxAge
	s.dataMutex.Lock()
	for symbol, ticker := range s.aggregatedTickers {
		if now.Sub(ticker.UpdatedAt) > maxAge {
			delete(s.aggregatedTickers, symbol)
		}
	}
	s.dataMutex.Unlock()

	// Clean up performance metrics history
	s.perfMutex.Lock()
	if len(s.latencyRecorder) > 1000 {
		s.latencyRecorder = s.latencyRecorder[len(s.latencyRecorder)-500:] // Keep last 500 entries
	}
	s.perfMutex.Unlock()
}

// Helper methods for calculations and utilities

func (s *EnhancedService) getExchangeWeight(exchangeName string) float64 {
	for _, config := range s.config.Exchanges {
		if config.Name == exchangeName {
			return config.Weight
		}
	}
	return 1.0 // Default weight
}

func (s *EnhancedService) updateFeedQuality(source, symbol string, timestamp time.Time, err error) {
	s.monitoringMutex.Lock()
	defer s.monitoringMutex.Unlock()

	key := fmt.Sprintf("%s:%s", source, symbol)
	quality, exists := s.feedQuality[key]

	if !exists {
		quality = &FeedQuality{
			Source:      source,
			Symbol:      symbol,
			IsHealthy:   true,
			LastUpdated: timestamp,
		}
		s.feedQuality[key] = quality
	}

	// Update latency (simulated for now)
	quality.Latency = time.Since(timestamp)
	quality.LastUpdated = timestamp

	if err != nil {
		quality.ErrorRate += 0.1 // Increase error rate
		quality.LastError = err.Error()
		quality.IsHealthy = false
	} else {
		quality.ErrorRate = quality.ErrorRate * 0.9 // Decrease error rate
		quality.IsHealthy = quality.ErrorRate < s.config.AlertThresholds.MaxErrorRate
	}

	// Update freshness
	quality.DataFreshness = time.Since(timestamp)
}

func (s *EnhancedService) updatePerformanceMetrics(duration time.Duration) {
	s.perfMutex.Lock()
	defer s.perfMutex.Unlock()

	s.requestCounter++
	if duration < 5*time.Second { // Consider successful if under 5 seconds
		s.successCounter++
	} else {
		s.failureCounter++
	}

	// Record latency
	s.latencyRecorder = append(s.latencyRecorder, duration)

	// Update metrics
	s.monitoringMutex.Lock()
	s.performanceMetrics.TotalRequests = s.requestCounter
	s.performanceMetrics.SuccessfulRequests = s.successCounter
	s.performanceMetrics.FailedRequests = s.failureCounter

	if len(s.latencyRecorder) > 0 {
		// Calculate average latency
		sum := time.Duration(0)
		for _, latency := range s.latencyRecorder {
			sum += latency
		}
		s.performanceMetrics.AverageLatency = sum / time.Duration(len(s.latencyRecorder))

		// Calculate percentiles
		sorted := make([]time.Duration, len(s.latencyRecorder))
		copy(sorted, s.latencyRecorder)
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i] < sorted[j]
		})

		if len(sorted) > 0 {
			p95Index := int(float64(len(sorted)) * 0.95)
			p99Index := int(float64(len(sorted)) * 0.99)
			if p95Index >= len(sorted) {
				p95Index = len(sorted) - 1
			}
			if p99Index >= len(sorted) {
				p99Index = len(sorted) - 1
			}
			s.performanceMetrics.P95Latency = sorted[p95Index]
			s.performanceMetrics.P99Latency = sorted[p99Index]
		}
	}

	s.performanceMetrics.LastUpdated = time.Now()
	s.monitoringMutex.Unlock()
}

func (s *EnhancedService) calculateQualityScore(symbol string, tickerData map[string]*TickerData) float64 {
	if len(tickerData) == 0 {
		return 0.0
	}

	score := float64(len(tickerData)) / 5.0 // Base score on number of sources (max 5)
	if score > 1.0 {
		score = 1.0
	}

	// Reduce score based on price variance
	prices := make([]decimal.Decimal, 0, len(tickerData))
	for _, ticker := range tickerData {
		prices = append(prices, ticker.Price)
	}

	if len(prices) > 1 {
		variance := s.calculatePriceVariance(prices)
		if variance.GreaterThan(decimal.NewFromFloat(0.01)) { // 1% variance penalty
			score = score * 0.9
		}
	}

	return score
}

func (s *EnhancedService) calculateOrderBookQualityScore(symbol string, orderBookData map[string]*OrderBookData) float64 {
	if len(orderBookData) == 0 {
		return 0.0
	}

	return float64(len(orderBookData)) / 5.0 // Base score on number of sources
}

func (s *EnhancedService) calculateDataFreshness(tickerData map[string]*TickerData) time.Duration {
	if len(tickerData) == 0 {
		return time.Hour // Very stale
	}

	newest := time.Time{}
	for _, ticker := range tickerData {
		if ticker.Timestamp.After(newest) {
			newest = ticker.Timestamp
		}
	}

	return time.Since(newest)
}

func (s *EnhancedService) calculateAveragePriceChange(tickerData map[string]*TickerData) decimal.Decimal {
	if len(tickerData) == 0 {
		return decimal.Zero
	}

	sum := decimal.Zero
	count := 0

	for _, ticker := range tickerData {
		sum = sum.Add(ticker.PriceChange)
		count++
	}

	if count == 0 {
		return decimal.Zero
	}

	return sum.Div(decimal.NewFromInt(int64(count)))
}

func (s *EnhancedService) calculateAveragePriceChangePercent(tickerData map[string]*TickerData) decimal.Decimal {
	if len(tickerData) == 0 {
		return decimal.Zero
	}

	sum := decimal.Zero
	count := 0

	for _, ticker := range tickerData {
		sum = sum.Add(ticker.PriceChangePercent)
		count++
	}

	if count == 0 {
		return decimal.Zero
	}

	return sum.Div(decimal.NewFromInt(int64(count)))
}

func (s *EnhancedService) calculatePriceVariance(prices []decimal.Decimal) decimal.Decimal {
	if len(prices) <= 1 {
		return decimal.Zero
	}

	// Calculate mean
	sum := decimal.Zero
	for _, price := range prices {
		sum = sum.Add(price)
	}
	mean := sum.Div(decimal.NewFromInt(int64(len(prices))))

	// Calculate variance
	variance := decimal.Zero
	for _, price := range prices {
		diff := price.Sub(mean)
		variance = variance.Add(diff.Mul(diff))
	}

	return variance.Div(decimal.NewFromInt(int64(len(prices))))
}

func (s *EnhancedService) calculateFiatRateChange(currency string, currentRate decimal.Decimal) decimal.Decimal {
	// Get previous rate (simulated for now)
	s.dataMutex.RLock()
	if prevRate, exists := s.fiatRates[currency]; exists {
		s.dataMutex.RUnlock()
		return currentRate.Sub(prevRate.Rate).Div(prevRate.Rate).Mul(decimal.NewFromInt(100))
	}
	s.dataMutex.RUnlock()

	return decimal.Zero
}

func (s *EnhancedService) notifyTickerSubscribers(symbol string, ticker *AggregatedTicker) {
	s.subscriptionMutex.RLock()
	defer s.subscriptionMutex.RUnlock()

	if channels, exists := s.tickerSubscriptions[symbol]; exists {
		for _, ch := range channels {
			select {
			case ch <- ticker:
			default:
				// Channel is full, skip
			}
		}
	}
}

func (s *EnhancedService) notifyOrderBookSubscribers(symbol string, orderBook *AggregatedOrderBook) {
	s.subscriptionMutex.RLock()
	defer s.subscriptionMutex.RUnlock()

	if channels, exists := s.orderBookSubscriptions[symbol]; exists {
		for _, ch := range channels {
			select {
			case ch <- orderBook:
			default:
				// Channel is full, skip
			}
		}
	}
}

func (s *EnhancedService) checkQualityThresholds(source string, quality *FeedQuality) {
	thresholds := s.config.AlertThresholds

	// Check latency
	if quality.Latency > thresholds.MaxLatency {
		s.generateAlert("high_latency", source, quality.Symbol,
			fmt.Sprintf("High latency detected: %v", quality.Latency), "medium")
	}

	// Check error rate
	if quality.ErrorRate > thresholds.MaxErrorRate {
		s.generateAlert("high_error_rate", source, quality.Symbol,
			fmt.Sprintf("High error rate detected: %.2f%%", quality.ErrorRate*100), "high")
	}

	// Check data freshness
	if quality.DataFreshness > thresholds.MaxDataAge {
		s.generateAlert("stale_data", source, quality.Symbol,
			fmt.Sprintf("Stale data detected: %v", quality.DataFreshness), "medium")
	}
}

func (s *EnhancedService) generateAlert(alertType, source, symbol, message, severity string) {
	alert := &Alert{
		ID:        fmt.Sprintf("%s_%s_%d", alertType, source, time.Now().Unix()),
		Type:      alertType,
		Source:    source,
		Symbol:    symbol,
		Message:   message,
		Severity:  severity,
		Timestamp: time.Now(),
		Resolved:  false,
	}

	s.alerts = append(s.alerts, alert)

	// Notify alert subscribers
	s.subscriptionMutex.RLock()
	for _, ch := range s.alertSubscriptions {
		select {
		case ch <- alert:
		default:
			// Channel is full, skip
		}
	}
	s.subscriptionMutex.RUnlock()

	s.logger.Warn("Alert generated",
		zap.String("type", alertType),
		zap.String("source", source),
		zap.String("symbol", symbol),
		zap.String("message", message),
		zap.String("severity", severity),
	)
}

func getEnvVar(key string, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
