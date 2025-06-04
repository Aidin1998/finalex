package marketfeeds

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Aidin1998/pincex_unified/pkg/models"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// MarketFeedService defines market feed operations.
type MarketFeedService interface {
	Start() error
	Stop() error
	GetMarketPrices(ctx context.Context) ([]*models.MarketPrice, error)
	GetMarketPrice(ctx context.Context, symbol string) (*models.MarketPrice, error)
	GetCandles(ctx context.Context, symbol, interval string, limit int) ([]*models.Candle, error)
}

// Service implements MarketFeedService
// Add pubsub for distribution

type PubSubBackend interface {
	Publish(ctx context.Context, channel string, msg interface{}) error
}

type Service struct {
	logger    *zap.Logger
	db        *gorm.DB
	prices    map[string]*models.MarketPrice
	candles   map[string]map[string][]*models.Candle // symbol -> interval -> candles
	mutex     sync.RWMutex
	stopChan  chan struct{}
	isRunning bool
	pubsub    PubSubBackend // new: publish updates
}

// NewService creates a new MarketFeedService
func NewService(logger *zap.Logger, db *gorm.DB, pubsub PubSubBackend) (MarketFeedService, error) {
	// Create service
	svc := &Service{
		logger:    logger,
		db:        db,
		prices:    make(map[string]*models.MarketPrice),
		candles:   make(map[string]map[string][]*models.Candle),
		stopChan:  make(chan struct{}),
		isRunning: false,
		pubsub:    pubsub,
	}

	return svc, nil
}

// Start starts the market feeds service
func (s *Service) Start() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.isRunning {
		return fmt.Errorf("market feeds service is already running")
	}

	// Load trading pairs
	var pairs []*models.TradingPair
	if err := s.db.Where("status = ?", "active").Find(&pairs).Error; err != nil {
		return fmt.Errorf("failed to load trading pairs: %w", err)
	}

	// Initialize prices and candles
	for _, pair := range pairs {
		// Initialize price
		s.prices[pair.Symbol] = &models.MarketPrice{
			Symbol:    pair.Symbol,
			Price:     0,
			Change24h: 0,
			Volume24h: 0,
			High24h:   0,
			Low24h:    0,
			UpdatedAt: time.Now(),
		}

		// Initialize candles
		s.candles[pair.Symbol] = make(map[string][]*models.Candle)
		intervals := []string{"1m", "5m", "15m", "30m", "1h", "4h", "1d", "1w"}
		for _, interval := range intervals {
			s.candles[pair.Symbol][interval] = make([]*models.Candle, 0)
		}
	}

	// Start price updater
	go s.updatePrices()

	s.isRunning = true
	s.logger.Info("Market feeds service started")

	return nil
}

// Stop stops the market feeds service
func (s *Service) Stop() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if !s.isRunning {
		return fmt.Errorf("market feeds service is not running")
	}

	// Stop price updater
	close(s.stopChan)

	s.isRunning = false
	s.logger.Info("Market feeds service stopped")

	return nil
}

// GetMarketPrices gets all market prices
func (s *Service) GetMarketPrices(ctx context.Context) ([]*models.MarketPrice, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	// Get prices
	prices := make([]*models.MarketPrice, 0, len(s.prices))
	for _, price := range s.prices {
		prices = append(prices, price)
	}

	return prices, nil
}

// GetMarketPrice gets a market price
func (s *Service) GetMarketPrice(ctx context.Context, symbol string) (*models.MarketPrice, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	// Get price
	price, ok := s.prices[symbol]
	if !ok {
		return nil, fmt.Errorf("market price not found: %s", symbol)
	}

	return price, nil
}

// GetCandles gets candles for a symbol
func (s *Service) GetCandles(ctx context.Context, symbol, interval string, limit int) ([]*models.Candle, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	// Check if symbol exists
	symbolCandles, ok := s.candles[symbol]
	if !ok {
		return nil, fmt.Errorf("candles not found for symbol: %s", symbol)
	}

	// Check if interval exists
	candles, ok := symbolCandles[interval]
	if !ok {
		return nil, fmt.Errorf("candles not found for interval: %s", interval)
	}

	// Get candles
	result := make([]*models.Candle, 0, limit)
	for i := len(candles) - 1; i >= 0 && len(result) < limit; i-- {
		result = append(result, candles[i])
	}

	return result, nil
}

// AddExternalPrice adds an external price
func (s *Service) AddExternalPrice(ctx context.Context, symbol string, price float64) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Check if symbol exists
	_, ok := s.prices[symbol]
	if !ok {
		return fmt.Errorf("market price not found: %s", symbol)
	}

	// Update price
	now := time.Now()
	s.prices[symbol] = &models.MarketPrice{
		Symbol:    symbol,
		Price:     price,
		Change24h: 0, // Would be calculated based on previous price
		Volume24h: 0, // Would be updated based on trades
		High24h:   price,
		Low24h:    price,
		UpdatedAt: now,
	}

	// Add candle
	for interval := range s.candles[symbol] {
		// In a real implementation, this would aggregate candles based on the interval
		candle := &models.Candle{
			Timestamp: now,
			Open:      price,
			High:      price,
			Low:       price,
			Close:     price,
			Volume:    0,
		}
		s.candles[symbol][interval] = append(s.candles[symbol][interval], candle)
	}

	// Publish normalized update to pubsub
	if s.pubsub != nil {
		_ = s.pubsub.Publish(ctx, "ticker", s.prices[symbol])
		// Optionally publish orderbook/candle/trade as needed
	}

	return nil
}

// updatePrices updates market prices periodically
func (s *Service) updatePrices() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.fetchExternalPrices()
		case <-s.stopChan:
			return
		}
	}
}

// fetchExternalPrices fetches prices from external sources
func (s *Service) fetchExternalPrices() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// In a real implementation, this would fetch prices from external APIs
	// For now, we'll just update the prices with random values
	for symbol, price := range s.prices {
		// Update price with a small random change
		newPrice := price.Price * (1 + (float64(time.Now().UnixNano()%200-100) / 10000))
		if newPrice <= 0 {
			newPrice = 0.01
		}

		// Update high and low
		high := price.High24h
		if newPrice > high {
			high = newPrice
		}
		low := price.Low24h
		if low == 0 || newPrice < low {
			low = newPrice
		}

		// Update price
		now := time.Now()
		s.prices[symbol] = &models.MarketPrice{
			Symbol:    symbol,
			Price:     newPrice,
			Change24h: (newPrice - price.Price) / price.Price * 100,
			Volume24h: price.Volume24h + float64(time.Now().UnixNano()%1000)/100,
			High24h:   high,
			Low24h:    low,
			UpdatedAt: now,
		}

		// Add candle
		for interval := range s.candles[symbol] {
			// In a real implementation, this would aggregate candles based on the interval
			candle := &models.Candle{
				Timestamp: now,
				Open:      price.Price,
				High:      high,
				Low:       low,
				Close:     newPrice,
				Volume:    float64(time.Now().UnixNano()%1000) / 100,
			}
			s.candles[symbol][interval] = append(s.candles[symbol][interval], candle)

			// Limit candles to 1000
			if len(s.candles[symbol][interval]) > 1000 {
				s.candles[symbol][interval] = s.candles[symbol][interval][len(s.candles[symbol][interval])-1000:]
			}
		}

		// After updating s.prices[symbol] and s.candles[symbol][interval]:
		if s.pubsub != nil {
			_ = s.pubsub.Publish(context.Background(), "ticker", s.prices[symbol])
			// Optionally publish orderbook/candle/trade as needed
		}
	}
}

// Add your imports and package declaration here

// Service represents the marketfeeds service

// MarketSummary represents a summary of a market
type MarketSummary struct {
	Symbol    string      `json:"symbol"`
	Price     float64     `json:"price"`
	Change24h float64     `json:"change_24h"`
	Volume24h float64     `json:"volume_24h"`
	High24h   float64     `json:"high_24h"`
	Low24h    float64     `json:"low_24h"`
	UpdatedAt interface{} `json:"updated_at"`
}

// GetMarketSummary returns a summary for the given symbol
func (s *Service) GetMarketSummary(symbol string) (*MarketSummary, error) {
	// TODO: Implement actual logic to fetch market summary for the symbol
	// This is a stub implementation for compilation
	return &MarketSummary{
		Symbol:    symbol,
		Price:     0,
		Change24h: 0,
		Volume24h: 0,
		High24h:   0,
		Low24h:    0,
		UpdatedAt: nil,
	}, nil
}
