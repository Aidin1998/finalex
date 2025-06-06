// Package service provides the market making service with strategy management
package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Aidin1998/finalex/internal/marketmaking/strategies/adapter"
	"github.com/Aidin1998/finalex/internal/marketmaking/strategies/common"
	"github.com/Aidin1998/finalex/internal/marketmaking/strategies/factory"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// MarketMakingService manages market making strategies
type MarketMakingService struct {
	mu              sync.RWMutex
	strategies      map[string]common.MarketMakingStrategy
	strategyFactory *factory.StrategyFactory
	legacyFactory   common.LegacyStrategyFactory // Use interface instead of direct marketmaker import
	activeStrategy  string
	isRunning       bool
	config          ServiceConfig
	metrics         ServiceMetrics
	logger          Logger
}

// ServiceConfig contains service configuration
type ServiceConfig struct {
	DefaultStrategy     string                 `json:"default_strategy"`
	RiskLimits          *common.RiskLimits     `json:"risk_limits"`
	QuoteRefreshRate    time.Duration          `json:"quote_refresh_rate"`
	HealthCheckRate     time.Duration          `json:"health_check_rate"`
	MaxConcurrentQuotes int                    `json:"max_concurrent_quotes"`
	Parameters          map[string]interface{} `json:"parameters"`
}

// ServiceMetrics contains service-level metrics
type ServiceMetrics struct {
	TotalQuotes      int64           `json:"total_quotes"`
	ActiveStrategies int             `json:"active_strategies"`
	LastQuoteTime    time.Time       `json:"last_quote_time"`
	AverageLatency   time.Duration   `json:"average_latency"`
	ErrorRate        decimal.Decimal `json:"error_rate"`
	Uptime           time.Duration   `json:"uptime"`
	StartTime        time.Time       `json:"start_time"`
}

// Logger interface for service logging
type Logger interface {
	Info(msg string, fields ...interface{})
	Error(msg string, fields ...interface{})
	Warn(msg string, fields ...interface{})
	Debug(msg string, fields ...interface{})
}

// NewMarketMakingService creates a new market making service
func NewMarketMakingService(legacyFactory common.LegacyStrategyFactory, logger Logger) *MarketMakingService {
	return &MarketMakingService{
		strategies:      make(map[string]common.MarketMakingStrategy),
		strategyFactory: factory.NewStrategyFactory(),
		legacyFactory:   legacyFactory,
		logger:          logger,
		metrics: ServiceMetrics{
			StartTime: time.Now(),
		},
	}
}

// Start starts the market making service
func (s *MarketMakingService) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isRunning {
		return fmt.Errorf("service is already running")
	}

	// Start default strategy if configured
	if s.config.DefaultStrategy != "" {
		if err := s.startStrategy(ctx, s.config.DefaultStrategy); err != nil {
			s.logger.Error("Failed to start default strategy", "strategy", s.config.DefaultStrategy, "error", err)
			return err
		}
	}

	s.isRunning = true
	s.logger.Info("Market making service started")

	return nil
}

// Stop stops the market making service
func (s *MarketMakingService) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.isRunning {
		return nil
	}

	// Stop all active strategies
	for id, strategy := range s.strategies {
		if err := strategy.Stop(ctx); err != nil {
			s.logger.Error("Failed to stop strategy", "id", id, "error", err)
		}
	}

	s.isRunning = false
	s.logger.Info("Market making service stopped")

	return nil
}

// CreateStrategy creates a new strategy instance
func (s *MarketMakingService) CreateStrategy(ctx context.Context, strategyType string, config common.StrategyConfig) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	strategyID := uuid.New().String()
	config.ID = strategyID

	// Try new strategy factory first
	strategy, err := s.strategyFactory.CreateStrategy(strategyType, config)
	if err != nil {
		// Fallback to legacy strategy factory
		legacyStrategy, legacyErr := s.createLegacyStrategy(strategyType, config)
		if legacyErr != nil {
			return "", fmt.Errorf("failed to create strategy with both factories: new=%v, legacy=%v", err, legacyErr)
		}
		strategy = legacyStrategy
	}

	// Initialize the strategy
	if err := strategy.Initialize(ctx, config); err != nil {
		return "", fmt.Errorf("failed to initialize strategy: %w", err)
	}

	s.strategies[strategyID] = strategy
	s.logger.Info("Strategy created", "id", strategyID, "type", strategyType)

	return strategyID, nil
}

// StartStrategy starts a specific strategy
func (s *MarketMakingService) StartStrategy(ctx context.Context, strategyID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.startStrategy(ctx, strategyID)
}

// StopStrategy stops a specific strategy
func (s *MarketMakingService) StopStrategy(ctx context.Context, strategyID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	strategy, exists := s.strategies[strategyID]
	if !exists {
		return fmt.Errorf("strategy not found: %s", strategyID)
	}

	if err := strategy.Stop(ctx); err != nil {
		return fmt.Errorf("failed to stop strategy: %w", err)
	}

	if s.activeStrategy == strategyID {
		s.activeStrategy = ""
	}

	s.logger.Info("Strategy stopped", "id", strategyID)
	return nil
}

// GetQuote generates a quote using the active strategy
func (s *MarketMakingService) GetQuote(ctx context.Context, input common.QuoteInput) (*common.QuoteOutput, error) {
	s.mu.RLock()
	activeID := s.activeStrategy
	s.mu.RUnlock()

	if activeID == "" {
		return nil, fmt.Errorf("no active strategy")
	}

	strategy, exists := s.strategies[activeID]
	if !exists {
		return nil, fmt.Errorf("active strategy not found")
	}

	startTime := time.Now()

	// Apply risk checks
	if err := s.checkRiskLimits(input); err != nil {
		return nil, fmt.Errorf("risk check failed: %w", err)
	}

	// Generate quote
	quote, err := strategy.Quote(ctx, input)
	if err != nil {
		s.logger.Error("Failed to generate quote", "strategy", activeID, "error", err)
		return nil, err
	}

	// Update metrics
	s.updateMetrics(time.Since(startTime))

	return quote, nil
}

// GetStrategies returns all strategy instances
func (s *MarketMakingService) GetStrategies() map[string]common.MarketMakingStrategy {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]common.MarketMakingStrategy)
	for id, strategy := range s.strategies {
		result[id] = strategy
	}

	return result
}

// GetServiceMetrics returns service metrics
func (s *MarketMakingService) GetServiceMetrics() ServiceMetrics {
	s.mu.RLock()
	defer s.mu.RUnlock()

	metrics := s.metrics
	metrics.Uptime = time.Since(metrics.StartTime)
	metrics.ActiveStrategies = len(s.strategies)

	return metrics
}

// Private helper methods

func (s *MarketMakingService) startStrategy(ctx context.Context, strategyID string) error {
	strategy, exists := s.strategies[strategyID]
	if !exists {
		return fmt.Errorf("strategy not found: %s", strategyID)
	}

	if err := strategy.Start(ctx); err != nil {
		return fmt.Errorf("failed to start strategy: %w", err)
	}

	s.activeStrategy = strategyID
	s.logger.Info("Strategy started", "id", strategyID)

	return nil
}

func (s *MarketMakingService) createLegacyStrategy(strategyType string, config common.StrategyConfig) (common.MarketMakingStrategy, error) {
	// Convert config to legacy format
	legacyParams := make(map[string]interface{})
	for k, v := range config.Parameters {
		legacyParams[k] = v
	}

	// Create legacy strategy
	legacyStrategy, err := s.legacyFactory.CreateStrategy(strategyType, legacyParams)
	if err != nil {
		return nil, err
	}

	// Wrap in adapter
	adapter := adapter.NewLegacyStrategyAdapter(legacyStrategy, strategyType, config)

	return adapter, nil
}

func (s *MarketMakingService) checkRiskLimits(input common.QuoteInput) error {
	if s.config.RiskLimits == nil {
		return nil
	}

	// Check inventory limits
	if input.Inventory.Abs().GreaterThan(s.config.RiskLimits.MaxInventory) {
		return fmt.Errorf("inventory limit exceeded: %v > %v", input.Inventory, s.config.RiskLimits.MaxInventory)
	}

	// Check order size limits
	if input.OrderSize.GreaterThan(s.config.RiskLimits.MaxOrderSize) {
		return fmt.Errorf("order size limit exceeded: %v > %v", input.OrderSize, s.config.RiskLimits.MaxOrderSize)
	}

	return nil
}

func (s *MarketMakingService) updateMetrics(latency time.Duration) {
	s.metrics.TotalQuotes++
	s.metrics.LastQuoteTime = time.Now()

	// Update average latency (simple moving average)
	if s.metrics.AverageLatency == 0 {
		s.metrics.AverageLatency = latency
	} else {
		s.metrics.AverageLatency = (s.metrics.AverageLatency + latency) / 2
	}
}
