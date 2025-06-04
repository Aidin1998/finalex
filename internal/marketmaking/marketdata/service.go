package marketdata

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// MarketDataPoint represents a single market data point
type MarketDataPoint struct {
	Market    string          `json:"market"`
	Price     decimal.Decimal `json:"price"`
	Volume    decimal.Decimal `json:"volume"`
	High      decimal.Decimal `json:"high"`
	Low       decimal.Decimal `json:"low"`
	Open      decimal.Decimal `json:"open"`
	Close     decimal.Decimal `json:"close"`
	Timestamp time.Time       `json:"timestamp"`
	Interval  string          `json:"interval"`
}

// MarketDataService handles market data operations
type MarketDataService struct {
	logger     *zap.Logger
	db         *gorm.DB
	mu         sync.RWMutex
	running    bool
	dataCache  map[string]*MarketDataPoint
	collectors map[string]*DataCollector
}

// DataCollector collects market data from various sources
type DataCollector struct {
	Market   string
	Source   string
	Interval time.Duration
	LastTick time.Time
	Running  bool
	mu       sync.RWMutex
}

// NewMarketDataService creates a new market data service
func NewMarketDataService(logger *zap.Logger, db *gorm.DB) (*MarketDataService, error) {
	return &MarketDataService{
		logger:     logger,
		db:         db,
		dataCache:  make(map[string]*MarketDataPoint),
		collectors: make(map[string]*DataCollector),
	}, nil
}

// GetData retrieves market data for a specific market and interval
func (mds *MarketDataService) GetData(ctx context.Context, market, interval string) (interface{}, error) {
	mds.mu.RLock()
	defer mds.mu.RUnlock()

	key := fmt.Sprintf("%s_%s", market, interval)
	data, exists := mds.dataCache[key]
	if !exists {
		// Generate sample data if not cached
		data = &MarketDataPoint{
			Market:    market,
			Price:     decimal.NewFromFloat(50000.00),
			Volume:    decimal.NewFromFloat(1234.56),
			High:      decimal.NewFromFloat(52000.00),
			Low:       decimal.NewFromFloat(48000.00),
			Open:      decimal.NewFromFloat(49000.00),
			Close:     decimal.NewFromFloat(50000.00),
			Timestamp: time.Now(),
			Interval:  interval,
		}
		mds.dataCache[key] = data
	}

	return data, nil
}

// UpdateData updates market data for a specific market
func (mds *MarketDataService) UpdateData(market string, data *MarketDataPoint) error {
	mds.mu.Lock()
	defer mds.mu.Unlock()

	key := fmt.Sprintf("%s_%s", market, data.Interval)
	mds.dataCache[key] = data

	mds.logger.Debug("Market data updated",
		zap.String("market", market),
		zap.String("interval", data.Interval),
		zap.String("price", data.Price.String()))

	return nil
}

// StartDataCollection starts data collection for a market
func (mds *MarketDataService) StartDataCollection(market, source string, interval time.Duration) error {
	mds.mu.Lock()
	defer mds.mu.Unlock()

	collectorKey := fmt.Sprintf("%s_%s", market, source)
	if _, exists := mds.collectors[collectorKey]; exists {
		return fmt.Errorf("data collection already running for %s from %s", market, source)
	}

	collector := &DataCollector{
		Market:   market,
		Source:   source,
		Interval: interval,
		Running:  true,
	}

	mds.collectors[collectorKey] = collector

	// Start collection goroutine
	go mds.runDataCollection(collector)

	mds.logger.Info("Data collection started",
		zap.String("market", market),
		zap.String("source", source),
		zap.Duration("interval", interval))

	return nil
}

// StopDataCollection stops data collection for a market
func (mds *MarketDataService) StopDataCollection(market, source string) error {
	mds.mu.Lock()
	defer mds.mu.Unlock()

	collectorKey := fmt.Sprintf("%s_%s", market, source)
	collector, exists := mds.collectors[collectorKey]
	if !exists {
		return fmt.Errorf("no data collection running for %s from %s", market, source)
	}

	collector.mu.Lock()
	collector.Running = false
	collector.mu.Unlock()

	delete(mds.collectors, collectorKey)

	mds.logger.Info("Data collection stopped",
		zap.String("market", market),
		zap.String("source", source))

	return nil
}

// runDataCollection runs the data collection loop
func (mds *MarketDataService) runDataCollection(collector *DataCollector) {
	ticker := time.NewTicker(collector.Interval)
	defer ticker.Stop()

	for collector.Running {
		select {
		case <-ticker.C:
			if err := mds.collectData(collector); err != nil {
				mds.logger.Error("Failed to collect market data",
					zap.String("market", collector.Market),
					zap.String("source", collector.Source),
					zap.Error(err))
			}
		}
	}
}

// collectData collects data from the specified source
func (mds *MarketDataService) collectData(collector *DataCollector) error {
	collector.mu.Lock()
	defer collector.mu.Unlock()

	// Simulate data collection - in production, this would call external APIs
	now := time.Now()

	// Generate realistic price movement
	basePrice := decimal.NewFromFloat(50000.00)
	if collector.LastTick.IsZero() {
		collector.LastTick = now.Add(-collector.Interval)
	}

	// Simple random walk for price simulation
	priceChange := decimal.NewFromFloat(float64(now.Unix()%200) - 100) // -100 to +100
	newPrice := basePrice.Add(priceChange)

	data := &MarketDataPoint{
		Market:    collector.Market,
		Price:     newPrice,
		Volume:    decimal.NewFromFloat(float64(now.Unix() % 1000)),
		High:      newPrice.Add(decimal.NewFromFloat(50)),
		Low:       newPrice.Sub(decimal.NewFromFloat(50)),
		Open:      newPrice.Sub(decimal.NewFromFloat(25)),
		Close:     newPrice,
		Timestamp: now,
		Interval:  "1m",
	}

	collector.LastTick = now

	// Update the cache
	return mds.UpdateData(collector.Market, data)
}

// GetHistoricalData retrieves historical market data
func (mds *MarketDataService) GetHistoricalData(ctx context.Context, market string, from, to time.Time, interval string) ([]*MarketDataPoint, error) {
	// In production, this would query the database for historical data
	// For now, return sample data
	var data []*MarketDataPoint

	current := from
	for current.Before(to) {
		point := &MarketDataPoint{
			Market:    market,
			Price:     decimal.NewFromFloat(50000.00 + float64(current.Unix()%1000)),
			Volume:    decimal.NewFromFloat(float64(current.Unix() % 500)),
			Timestamp: current,
			Interval:  interval,
		}
		data = append(data, point)

		// Increment by interval
		switch interval {
		case "1m":
			current = current.Add(time.Minute)
		case "5m":
			current = current.Add(5 * time.Minute)
		case "1h":
			current = current.Add(time.Hour)
		case "1d":
			current = current.Add(24 * time.Hour)
		default:
			current = current.Add(time.Minute)
		}
	}

	return data, nil
}

// Start starts the market data service
func (mds *MarketDataService) Start() error {
	mds.mu.Lock()
	defer mds.mu.Unlock()

	mds.running = true
	mds.logger.Info("Market data service started")

	// Start default data collection for major markets
	go func() {
		time.Sleep(time.Second) // Allow service to fully initialize
		mds.StartDataCollection("BTC-USD", "internal", time.Minute)
		mds.StartDataCollection("ETH-USD", "internal", time.Minute)
		mds.StartDataCollection("ADA-USD", "internal", time.Minute)
	}()

	return nil
}

// Stop stops the market data service
func (mds *MarketDataService) Stop() error {
	mds.mu.Lock()
	defer mds.mu.Unlock()

	mds.running = false

	// Stop all collectors
	for key, collector := range mds.collectors {
		collector.mu.Lock()
		collector.Running = false
		collector.mu.Unlock()
		delete(mds.collectors, key)
	}

	mds.logger.Info("Market data service stopped")
	return nil
}
