// Package marketmaker - Strategy service integration
// This file provides integration between the legacy marketmaker service and the new strategy system
package marketmaker

import (
	"time"

	"github.com/Aidin1998/finalex/internal/marketmaking/strategies/service"
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

// Remove all unused functions:
// - getNewStrategyService
// - useNewStrategyForQuoting
// - migrateToNewStrategy
