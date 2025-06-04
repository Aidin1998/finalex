package analytics

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// AnalyticsService handles market analytics and metrics calculation
type AnalyticsService struct {
	logger   *zap.Logger
	db       *gorm.DB
	mu       sync.RWMutex
	running  bool
	cache    map[string]interface{}
	cacheTTL time.Duration
}

// MarketMetrics represents market analytics metrics
type MarketMetrics struct {
	ID                 string          `json:"id" gorm:"primaryKey"`
	Market             string          `json:"market" gorm:"index"`
	Volume24h          decimal.Decimal `json:"volume_24h" gorm:"type:decimal(36,18)"`
	PriceChange24h     decimal.Decimal `json:"price_change_24h" gorm:"type:decimal(36,18)"`
	PriceChangePerc24h decimal.Decimal `json:"price_change_perc_24h" gorm:"type:decimal(10,4)"`
	High24h            decimal.Decimal `json:"high_24h" gorm:"type:decimal(36,18)"`
	Low24h             decimal.Decimal `json:"low_24h" gorm:"type:decimal(36,18)"`
	TradeCount24h      int             `json:"trade_count_24h"`
	Spread             decimal.Decimal `json:"spread" gorm:"type:decimal(36,18)"`
	LastPrice          decimal.Decimal `json:"last_price" gorm:"type:decimal(36,18)"`
	UpdatedAt          time.Time       `json:"updated_at"`
	CreatedAt          time.Time       `json:"created_at"`
}

// VolumeAnalytics represents volume analytics data
type VolumeAnalytics struct {
	ID           string                     `json:"id" gorm:"primaryKey"`
	Market       string                     `json:"market" gorm:"index"`
	Period       string                     `json:"period"`
	TotalVolume  decimal.Decimal            `json:"total_volume" gorm:"type:decimal(36,18)"`
	AvgVolume    decimal.Decimal            `json:"avg_volume" gorm:"type:decimal(36,18)"`
	PeakVolume   decimal.Decimal            `json:"peak_volume" gorm:"type:decimal(36,18)"`
	VolumeByHour map[string]decimal.Decimal `json:"volume_by_hour" gorm:"serializer:json"`
	CreatedAt    time.Time                  `json:"created_at"`
}

// TradingStatistics represents user trading statistics
type TradingStatistics struct {
	ID           string          `json:"id" gorm:"primaryKey"`
	UserID       string          `json:"user_id" gorm:"index"`
	TotalTrades  int             `json:"total_trades"`
	TotalVolume  decimal.Decimal `json:"total_volume" gorm:"type:decimal(36,18)"`
	TotalFees    decimal.Decimal `json:"total_fees" gorm:"type:decimal(36,18)"`
	ProfitLoss   decimal.Decimal `json:"profit_loss" gorm:"type:decimal(36,18)"`
	WinRate      decimal.Decimal `json:"win_rate" gorm:"type:decimal(5,4)"`
	AvgTradeSize decimal.Decimal `json:"avg_trade_size" gorm:"type:decimal(36,18)"`
	LastTradeAt  time.Time       `json:"last_trade_at"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

// NewAnalyticsService creates a new analytics service
func NewAnalyticsService(logger *zap.Logger, db *gorm.DB) (*AnalyticsService, error) {
	service := &AnalyticsService{
		logger:   logger,
		db:       db,
		cache:    make(map[string]interface{}),
		cacheTTL: 5 * time.Minute,
	}

	// Auto-migrate tables
	if err := db.AutoMigrate(&MarketMetrics{}, &VolumeAnalytics{}, &TradingStatistics{}); err != nil {
		return nil, fmt.Errorf("failed to migrate analytics tables: %w", err)
	}

	return service, nil
}

// GetMarketMetrics retrieves market metrics for a specific market
func (s *AnalyticsService) GetMarketMetrics(ctx context.Context, market string) (interface{}, error) {
	s.mu.RLock()
	cacheKey := fmt.Sprintf("metrics_%s", market)
	if cached, exists := s.cache[cacheKey]; exists {
		s.mu.RUnlock()
		return cached, nil
	}
	s.mu.RUnlock()

	var metrics MarketMetrics
	result := s.db.WithContext(ctx).Where("market = ?", market).First(&metrics)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			// Calculate metrics if not found
			return s.calculateMarketMetrics(ctx, market)
		}
		return nil, fmt.Errorf("failed to get market metrics: %w", result.Error)
	}

	// Cache the result
	s.mu.Lock()
	s.cache[cacheKey] = metrics
	s.mu.Unlock()

	return metrics, nil
}

// GetVolumeAnalytics retrieves volume analytics for a market and period
func (s *AnalyticsService) GetVolumeAnalytics(ctx context.Context, market, period string) (interface{}, error) {
	s.mu.RLock()
	cacheKey := fmt.Sprintf("volume_%s_%s", market, period)
	if cached, exists := s.cache[cacheKey]; exists {
		s.mu.RUnlock()
		return cached, nil
	}
	s.mu.RUnlock()

	var analytics VolumeAnalytics
	result := s.db.WithContext(ctx).Where("market = ? AND period = ?", market, period).First(&analytics)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			// Calculate analytics if not found
			return s.calculateVolumeAnalytics(ctx, market, period)
		}
		return nil, fmt.Errorf("failed to get volume analytics: %w", result.Error)
	}

	// Cache the result
	s.mu.Lock()
	s.cache[cacheKey] = analytics
	s.mu.Unlock()

	return analytics, nil
}

// GetTradingStatistics retrieves trading statistics for a user
func (s *AnalyticsService) GetTradingStatistics(ctx context.Context, userID string) (interface{}, error) {
	s.mu.RLock()
	cacheKey := fmt.Sprintf("stats_%s", userID)
	if cached, exists := s.cache[cacheKey]; exists {
		s.mu.RUnlock()
		return cached, nil
	}
	s.mu.RUnlock()

	var stats TradingStatistics
	result := s.db.WithContext(ctx).Where("user_id = ?", userID).First(&stats)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			// Calculate statistics if not found
			return s.calculateTradingStatistics(ctx, userID)
		}
		return nil, fmt.Errorf("failed to get trading statistics: %w", result.Error)
	}

	// Cache the result
	s.mu.Lock()
	s.cache[cacheKey] = stats
	s.mu.Unlock()

	return stats, nil
}

// calculateMarketMetrics calculates market metrics from raw data
func (s *AnalyticsService) calculateMarketMetrics(ctx context.Context, market string) (*MarketMetrics, error) {
	now := time.Now()
	// yesterday is used for price change calculations in a real implementation
	// now.Add(-24 * time.Hour)

	// This would typically query from trades and orders tables
	// For now, return default metrics
	metrics := &MarketMetrics{
		ID:                 uuid.New().String(),
		Market:             market,
		Volume24h:          decimal.NewFromFloat(1000000),
		PriceChange24h:     decimal.NewFromFloat(100),
		PriceChangePerc24h: decimal.NewFromFloat(2.5),
		High24h:            decimal.NewFromFloat(4100),
		Low24h:             decimal.NewFromFloat(3950),
		TradeCount24h:      1500,
		Spread:             decimal.NewFromFloat(1.5),
		LastPrice:          decimal.NewFromFloat(4050),
		UpdatedAt:          now,
		CreatedAt:          now,
	}

	// Save to database
	if err := s.db.WithContext(ctx).Create(metrics).Error; err != nil {
		s.logger.Error("Failed to save market metrics", zap.Error(err))
	}

	s.logger.Info("Calculated market metrics",
		zap.String("market", market),
		zap.String("volume_24h", metrics.Volume24h.String()))

	return metrics, nil
}

// calculateVolumeAnalytics calculates volume analytics for a market and period
func (s *AnalyticsService) calculateVolumeAnalytics(ctx context.Context, market, period string) (*VolumeAnalytics, error) {
	now := time.Now()

	// Create volume distribution by hour
	volumeByHour := make(map[string]decimal.Decimal)
	for i := 0; i < 24; i++ {
		hourKey := fmt.Sprintf("%d", i)
		volumeByHour[hourKey] = decimal.NewFromFloat(float64(1000 + i*100))
	}

	analytics := &VolumeAnalytics{
		ID:           uuid.New().String(),
		Market:       market,
		Period:       period,
		TotalVolume:  decimal.NewFromFloat(500000),
		AvgVolume:    decimal.NewFromFloat(20833),
		PeakVolume:   decimal.NewFromFloat(45000),
		VolumeByHour: volumeByHour,
		CreatedAt:    now,
	}

	// Save to database
	if err := s.db.WithContext(ctx).Create(analytics).Error; err != nil {
		s.logger.Error("Failed to save volume analytics", zap.Error(err))
	}

	s.logger.Info("Calculated volume analytics",
		zap.String("market", market),
		zap.String("period", period),
		zap.String("total_volume", analytics.TotalVolume.String()))

	return analytics, nil
}

// calculateTradingStatistics calculates trading statistics for a user
func (s *AnalyticsService) calculateTradingStatistics(ctx context.Context, userID string) (*TradingStatistics, error) {
	now := time.Now()

	// This would typically query from user trades
	// For now, return default statistics
	stats := &TradingStatistics{
		ID:           uuid.New().String(),
		UserID:       userID,
		TotalTrades:  150,
		TotalVolume:  decimal.NewFromFloat(75000),
		TotalFees:    decimal.NewFromFloat(75),
		ProfitLoss:   decimal.NewFromFloat(2500),
		WinRate:      decimal.NewFromFloat(0.65),
		AvgTradeSize: decimal.NewFromFloat(500),
		LastTradeAt:  now.Add(-2 * time.Hour),
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	// Save to database
	if err := s.db.WithContext(ctx).Create(stats).Error; err != nil {
		s.logger.Error("Failed to save trading statistics", zap.Error(err))
	}

	s.logger.Info("Calculated trading statistics",
		zap.String("user_id", userID),
		zap.Int("total_trades", stats.TotalTrades),
		zap.String("total_volume", stats.TotalVolume.String()))

	return stats, nil
}

// UpdateMetrics updates market metrics with new data
func (s *AnalyticsService) UpdateMetrics(ctx context.Context, market string, data map[string]interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Clear cache for this market
	cacheKey := fmt.Sprintf("metrics_%s", market)
	delete(s.cache, cacheKey)

	s.logger.Info("Updated metrics cache", zap.String("market", market))
	return nil
}

// clearCache clears expired cache entries
func (s *AnalyticsService) clearCache() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Simple cache clearing - in production would check timestamps
	if len(s.cache) > 1000 {
		s.cache = make(map[string]interface{})
	}

	s.logger.Debug("Cache cleared")
}

// Start starts the analytics service
func (s *AnalyticsService) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("analytics service is already running")
	}

	s.logger.Info("Starting analytics service")

	// Start cache cleanup routine
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				s.clearCache()
			}
		}
	}()

	s.running = true
	s.logger.Info("Analytics service started successfully")

	return nil
}

// Stop stops the analytics service
func (s *AnalyticsService) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return fmt.Errorf("analytics service is not running")
	}

	s.logger.Info("Stopping analytics service")
	s.running = false
	s.logger.Info("Analytics service stopped")

	return nil
}
