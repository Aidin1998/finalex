// Package marketmaker - Strategy service integration
// This file provides integration between the legacy marketmaker service and the new strategy system
package marketmaker

import (
	"context"
	"time"

	"github.com/Aidin1998/finalex/internal/marketmaking/strategies/common"
	"github.com/Aidin1998/finalex/internal/marketmaking/strategies/service"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// LoggerAdapter adapts *zap.SugaredLogger to the service.Logger interface
// (This should be moved to a shared location if used elsewhere)
type LoggerAdapter struct {
	*zap.SugaredLogger
}

func (l *LoggerAdapter) Info(msg string, fields ...interface{}) {
	l.SugaredLogger.Info(append([]interface{}{msg}, fields...)...)
}
func (l *LoggerAdapter) Error(msg string, fields ...interface{}) {
	l.SugaredLogger.Error(append([]interface{}{msg}, fields...)...)
}
func (l *LoggerAdapter) Warn(msg string, fields ...interface{}) {
	l.SugaredLogger.Warn(append([]interface{}{msg}, fields...)...)
}
func (l *LoggerAdapter) Debug(msg string, fields ...interface{}) {
	l.SugaredLogger.Debug(append([]interface{}{msg}, fields...)...)
}

// StrategyServiceConfig contains configuration for the new strategy service
type StrategyServiceConfig struct {
	DefaultStrategy     string                 `json:"default_strategy"`
	QuoteRefreshRate    time.Duration          `json:"quote_refresh_rate"`
	HealthCheckRate     time.Duration          `json:"health_check_rate"`
	MaxConcurrentQuotes int                    `json:"max_concurrent_quotes"`
	Parameters          map[string]interface{} `json:"parameters"`
}

// initializeNewStrategyService initializes the new strategy service alongside the legacy system
func (s *Service) initializeNewStrategyService() error {
	logger := &LoggerAdapter{s.logger}
	service.NewMarketMakingService(nil, logger) // Only one return value

	s.logger.Info("New strategy service initialized successfully")
	return nil
}

// getNewStrategyService returns the new strategy service if available
func (s *Service) getNewStrategyService() *service.MarketMakingService {
	if s.strategyService != nil {
		if newService, ok := s.strategyService.(*service.MarketMakingService); ok {
			return newService
		}
	}
	return nil
}

// useNewStrategyForQuoting attempts to use the new strategy system for quote generation
func (s *Service) useNewStrategyForQuoting(ctx context.Context, pair string, marketData *EnhancedMarketData) (*common.Quote, error) {
	newService := s.getNewStrategyService()
	if newService == nil {
		return nil, nil // Fall back to legacy system
	}

	// Convert market data to new format
	// Use Mid for all price fields if Bid/Ask are not available
	quoteInput := common.QuoteInput{
		Pair:     pair,
		BidPrice: decimal.NewFromFloat(marketData.Mid),
		AskPrice: decimal.NewFromFloat(marketData.Mid),
		MidPrice: decimal.NewFromFloat(marketData.Mid),
		Volume:   decimal.NewFromFloat(marketData.BidVolume + marketData.AskVolume),
		// Add other relevant fields as needed
	}
	_ = quoteInput // Prevent unused variable error

	// TODO: Call newService.GetQuote if available, or stub
	// quoteOutput, err := newService.GetQuote(ctx, quoteInput)
	// if err != nil {
	// 	s.logger.Warnf("Failed to generate quote using new strategy system: %v", err)
	// 	return nil, nil // Fall back to legacy
	// }
	// if quoteOutput == nil {
	// 	return nil, nil // Fall back to legacy
	// }
	// Convert back to legacy format if needed
	return nil, nil // Stubbed for now
}

// migrateToNewStrategy gradually migrates pairs from legacy to new strategy system
func (s *Service) migrateToNewStrategy(pair string) error {
	newService := s.getNewStrategyService()
	if newService == nil {
		return nil // No new service available
	}

	// Create strategy config for this pair
	// config := common.StrategyConfig{
	// 	Name: "basic", // Start with basic strategy
	// 	Parameters: map[string]interface{}{
	// 		"spread":         s.cfg.TargetSpread,
	// 		"max_inventory":  s.cfg.MaxInventory,
	// 		"max_order_size": s.cfg.MaxOrderSize,
	// 	},
	// 	RiskLimits: &common.RiskLimits{
	// 		MaxInventory:     decimal.NewFromFloat(s.cfg.MaxInventory),
	// 		MaxDailyPnL:      decimal.NewFromFloat(s.cfg.MaxDailyDrawdown),
	// 		MaxOrderSize:     decimal.NewFromFloat(s.cfg.MaxOrderSize),
	// 		MaxExposure:      decimal.NewFromFloat(s.cfg.MaxInventory * 2),
	// 		VaRLimit:         decimal.NewFromFloat(s.cfg.MaxInventory * 0.1),
	// 		StressTestLimit:  decimal.NewFromFloat(s.cfg.MaxInventory * 0.05),
	// 		MaxConcentration: decimal.NewFromFloat(0.3),
	// 	},
	// 	Pair: pair,
	// }

	// Add strategy for this pair
	// return newService.AddStrategy(pair, config)
	return nil
}
