// Package marketmaker - Strategy service integration
// This file provides integration between the legacy marketmaker service and the new strategy system
package marketmaker

import (
	"context"
	"time"

	"github.com/Aidin1998/finalex/internal/marketmaking/strategies/common"
	"github.com/Aidin1998/finalex/internal/marketmaking/strategies/factory"
	"github.com/Aidin1998/finalex/internal/marketmaking/strategies/service"
	"github.com/shopspring/decimal"
)

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
	// Create strategy factory
	strategyFactory := factory.NewStrategyFactory()

	// Create service config
	serviceConfig := service.ServiceConfig{
		DefaultStrategy:     "basic",
		QuoteRefreshRate:    100 * time.Millisecond,
		HealthCheckRate:     5 * time.Second,
		MaxConcurrentQuotes: 100,
		Parameters: map[string]interface{}{
			"spread":         s.cfg.TargetSpread,
			"max_inventory":  s.cfg.MaxInventory,
			"max_order_size": s.cfg.MaxOrderSize,
		},
		RiskLimits: &common.RiskLimits{
			MaxInventory:     decimal.NewFromFloat(s.cfg.MaxInventory),
			MaxDailyPnL:      decimal.NewFromFloat(s.cfg.MaxDailyDrawdown),
			MaxOrderSize:     decimal.NewFromFloat(s.cfg.MaxOrderSize),
			MaxExposure:      decimal.NewFromFloat(s.cfg.MaxInventory * 2),
			VaRLimit:         decimal.NewFromFloat(s.cfg.MaxInventory * 0.1),
			StressTestLimit:  decimal.NewFromFloat(s.cfg.MaxInventory * 0.05),
			MaxConcentration: decimal.NewFromFloat(0.3),
		},
	}

	// Create the market making service
	newStrategyService, err := service.NewMarketMakingService(serviceConfig)
	if err != nil {
		return err
	}

	// Store reference to the new service
	s.strategyService = newStrategyService

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
func (s *Service) useNewStrategyForQuoting(ctx context.Context, pair string, marketData *EnhancedMarketData) (*Quote, error) {
	newService := s.getNewStrategyService()
	if newService == nil {
		return nil, nil // Fall back to legacy system
	}

	// Convert market data to new format
	quoteInput := common.QuoteInput{
		Pair:     pair,
		BidPrice: decimal.NewFromFloat(marketData.BestBid),
		AskPrice: decimal.NewFromFloat(marketData.BestAsk),
		MidPrice: decimal.NewFromFloat((marketData.BestBid + marketData.BestAsk) / 2),
		Volume:   decimal.NewFromFloat(marketData.Volume),
		// Add other relevant fields...
	}

	// Get quote from new strategy system
	quoteOutput, err := newService.GenerateQuote(ctx, quoteInput)
	if err != nil {
		s.logger.Warnf("Failed to generate quote using new strategy system: %v", err)
		return nil, nil // Fall back to legacy
	}

	if quoteOutput == nil {
		return nil, nil // Fall back to legacy
	}

	// Convert back to legacy format
	bidPriceFloat, _ := quoteOutput.BidPrice.Float64()
	askPriceFloat, _ := quoteOutput.AskPrice.Float64()
	bidSizeFloat, _ := quoteOutput.BidSize.Float64()
	askSizeFloat, _ := quoteOutput.AskSize.Float64()

	quote := &Quote{
		BidPrice: bidPriceFloat,
		AskPrice: askPriceFloat,
		BidSize:  bidSizeFloat,
		AskSize:  askSizeFloat,
		Pair:     pair,
	}

	return quote, nil
}

// migrateToNewStrategy gradually migrates pairs from legacy to new strategy system
func (s *Service) migrateToNewStrategy(pair string) error {
	newService := s.getNewStrategyService()
	if newService == nil {
		return nil // No new service available
	}

	// Create strategy config for this pair
	config := common.StrategyConfig{
		Name: "basic", // Start with basic strategy
		Parameters: map[string]interface{}{
			"spread":         s.cfg.TargetSpread,
			"max_inventory":  s.cfg.MaxInventory,
			"max_order_size": s.cfg.MaxOrderSize,
		},
		RiskLimits: &common.RiskLimits{
			MaxInventory:     decimal.NewFromFloat(s.cfg.MaxInventory),
			MaxDailyPnL:      decimal.NewFromFloat(s.cfg.MaxDailyDrawdown),
			MaxOrderSize:     decimal.NewFromFloat(s.cfg.MaxOrderSize),
			MaxExposure:      decimal.NewFromFloat(s.cfg.MaxInventory * 2),
			VaRLimit:         decimal.NewFromFloat(s.cfg.MaxInventory * 0.1),
			StressTestLimit:  decimal.NewFromFloat(s.cfg.MaxInventory * 0.05),
			MaxConcentration: decimal.NewFromFloat(0.3),
		},
		Pair: pair,
	}

	// Add strategy for this pair
	return newService.AddStrategy(pair, config)
}
