package marketmaking

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/marketmaking/analytics"
	"github.com/Aidin1998/pincex_unified/internal/marketmaking/marketdata"
	"github.com/Aidin1998/pincex_unified/internal/marketmaking/marketfeeds"
	"github.com/Aidin1998/pincex_unified/internal/marketmaking/marketmaker"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Implementation of the consolidated market making service
type service struct {
	logger        *zap.Logger
	db            *gorm.DB
	mu            sync.RWMutex
	running       bool
	marketData    *marketdata.MarketDataService
	feeds         *marketfeeds.FeedService
	analytics     *analytics.AnalyticsService
	marketMaker   *marketmaker.MarketMakerService
	subscriptions map[string][]func(interface{})
}

// MarketMetrics represents market analytics metrics
type MarketMetrics struct {
	Market             string          `json:"market"`
	Volume24h          decimal.Decimal `json:"volume_24h"`
	PriceChange24h     decimal.Decimal `json:"price_change_24h"`
	PriceChangePerc24h decimal.Decimal `json:"price_change_perc_24h"`
	High24h            decimal.Decimal `json:"high_24h"`
	Low24h             decimal.Decimal `json:"low_24h"`
	TradeCount24h      int             `json:"trade_count_24h"`
	Spread             decimal.Decimal `json:"spread"`
	LastPrice          decimal.Decimal `json:"last_price"`
	UpdatedAt          time.Time       `json:"updated_at"`
}

// VolumeAnalytics represents volume analytics data
type VolumeAnalytics struct {
	Market       string                  `json:"market"`
	Period       string                  `json:"period"`
	TotalVolume  decimal.Decimal         `json:"total_volume"`
	AvgVolume    decimal.Decimal         `json:"avg_volume"`
	PeakVolume   decimal.Decimal         `json:"peak_volume"`
	VolumeByHour map[int]decimal.Decimal `json:"volume_by_hour"`
	CreatedAt    time.Time               `json:"created_at"`
}

// TradingStatistics represents user trading statistics
type TradingStatistics struct {
	UserID       string          `json:"user_id"`
	TotalTrades  int             `json:"total_trades"`
	TotalVolume  decimal.Decimal `json:"total_volume"`
	TotalFees    decimal.Decimal `json:"total_fees"`
	ProfitLoss   decimal.Decimal `json:"profit_loss"`
	WinRate      decimal.Decimal `json:"win_rate"`
	AvgTradeSize decimal.Decimal `json:"avg_trade_size"`
	LastTradeAt  time.Time       `json:"last_trade_at"`
	CreatedAt    time.Time       `json:"created_at"`
}

// NewService creates a new consolidated market making service
func NewService(logger *zap.Logger, db *gorm.DB) (Service, error) {
	s := &service{
		logger:        logger,
		db:            db,
		subscriptions: make(map[string][]func(interface{})),
	}
	// Initialize sub-services
	marketDataService, err := marketdata.NewMarketDataService(logger, db)
	if err != nil {
		return nil, fmt.Errorf("failed to create market data service: %w", err)
	}
	s.marketData = marketDataService

	feedService, err := marketfeeds.NewFeedService(logger, db)
	if err != nil {
		return nil, fmt.Errorf("failed to create feed service: %w", err)
	}
	s.feeds = feedService

	analyticsService, err := analytics.NewAnalyticsService(logger, db)
	if err != nil {
		return nil, fmt.Errorf("failed to create analytics service: %w", err)
	}
	s.analytics = analyticsService

	marketMakerService, err := marketmaker.NewMarketMakerService(logger, db)
	if err != nil {
		return nil, fmt.Errorf("failed to create market maker service: %w", err)
	}
	s.marketMaker = marketMakerService

	return s, nil
}

// GetMarketData retrieves market data for a specific market and interval
func (s *service) GetMarketData(ctx context.Context, market, interval string) (interface{}, error) {
	return s.marketData.GetData(ctx, market, interval)
}

// SubscribeMarketData subscribes to market data updates
func (s *service) SubscribeMarketData(ctx context.Context, market string, callback func(interface{})) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.subscriptions[market] == nil {
		s.subscriptions[market] = make([]func(interface{}), 0)
	}

	s.subscriptions[market] = append(s.subscriptions[market], callback)

	s.logger.Info("Market data subscription added",
		zap.String("market", market),
		zap.Int("total_subscribers", len(s.subscriptions[market])))

	return nil
}

// UnsubscribeMarketData unsubscribes from market data updates
func (s *service) UnsubscribeMarketData(ctx context.Context, market string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.subscriptions, market)

	s.logger.Info("Market data subscription removed", zap.String("market", market))
	return nil
}

// AddFeed adds a new market data feed
func (s *service) AddFeed(ctx context.Context, name, source, market string, config map[string]interface{}) (string, error) {
	return s.feeds.AddFeed(ctx, name, source, market, config)
}

// GetFeed retrieves a feed by ID
func (s *service) GetFeed(ctx context.Context, feedID string) (interface{}, error) {
	return s.feeds.GetFeed(ctx, feedID)
}

// GetFeeds retrieves all feeds
func (s *service) GetFeeds(ctx context.Context) ([]interface{}, error) {
	return s.feeds.GetFeeds(ctx)
}

// UpdateFeed updates a feed configuration
func (s *service) UpdateFeed(ctx context.Context, feedID string, config map[string]interface{}) error {
	return s.feeds.UpdateFeed(ctx, feedID, config)
}

// DeleteFeed deletes a feed
func (s *service) DeleteFeed(ctx context.Context, feedID string) error {
	return s.feeds.DeleteFeed(ctx, feedID)
}

// StartFeed starts a feed
func (s *service) StartFeed(ctx context.Context, feedID string) error {
	return s.feeds.StartFeed(ctx, feedID)
}

// StopFeed stops a feed
func (s *service) StopFeed(ctx context.Context, feedID string) error {
	return s.feeds.StopFeed(ctx, feedID)
}

// GetMarketMetrics retrieves market metrics
func (s *service) GetMarketMetrics(ctx context.Context, market string) (interface{}, error) {
	return s.analytics.GetMarketMetrics(ctx, market)
}

// GetVolumeAnalytics retrieves volume analytics
func (s *service) GetVolumeAnalytics(ctx context.Context, market, period string) (interface{}, error) {
	return s.analytics.GetVolumeAnalytics(ctx, market, period)
}

// GetTradingStatistics retrieves trading statistics for a user
func (s *service) GetTradingStatistics(ctx context.Context, userID string) (interface{}, error) {
	return s.analytics.GetTradingStatistics(ctx, userID)
}

// broadcastMarketData broadcasts market data to subscribers
func (s *service) broadcastMarketData(market string, data interface{}) {
	s.mu.RLock()
	subscribers := s.subscriptions[market]
	s.mu.RUnlock()

	for _, callback := range subscribers {
		go func(cb func(interface{})) {
			defer func() {
				if r := recover(); r != nil {
					s.logger.Error("Market data callback panic",
						zap.String("market", market),
						zap.Any("panic", r))
				}
			}()
			cb(data)
		}(callback)
	}
}

// Start starts the market making service
func (s *service) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("service is already running")
	}

	s.logger.Info("Starting market making service")

	// Start sub-services
	if err := s.marketData.Start(); err != nil {
		return fmt.Errorf("failed to start market data service: %w", err)
	}

	if err := s.feeds.Start(); err != nil {
		return fmt.Errorf("failed to start feed service: %w", err)
	}

	if err := s.analytics.Start(); err != nil {
		return fmt.Errorf("failed to start analytics service: %w", err)
	}

	if err := s.marketMaker.Start(); err != nil {
		return fmt.Errorf("failed to start market maker service: %w", err)
	}

	s.running = true
	s.logger.Info("Market making service started successfully")

	return nil
}

// Stop stops the market making service
func (s *service) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return fmt.Errorf("service is not running")
	}

	s.logger.Info("Stopping market making service")

	// Stop sub-services
	if err := s.marketMaker.Stop(); err != nil {
		s.logger.Error("Failed to stop market maker service", zap.Error(err))
	}

	if err := s.analytics.Stop(); err != nil {
		s.logger.Error("Failed to stop analytics service", zap.Error(err))
	}

	if err := s.feeds.Stop(); err != nil {
		s.logger.Error("Failed to stop feed service", zap.Error(err))
	}

	if err := s.marketData.Stop(); err != nil {
		s.logger.Error("Failed to stop market data service", zap.Error(err))
	}

	s.running = false
	s.logger.Info("Market making service stopped")

	return nil
}
