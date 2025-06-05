// Package trading provides trading services and interfaces
package trading

import (
	"context"

	"github.com/Aidin1998/finalex/internal/trading/engine"
)

// AdaptiveTradingService extends TradingService with adaptive engine capabilities
// This interface provides extended trading functionality with adaptive routing features
type AdaptiveTradingService interface {
	// Embed the core trading interface
	TradingService

	// Migration control methods
	StartMigration(pair string) error
	StopMigration(pair string) error
	PauseMigration(pair string) error
	ResumeMigration(pair string) error
	SetMigrationPercentage(pair string, percentage int32) error
	RollbackMigration(pair string) error
	ResetMigration(pair string) error

	// Migration status and monitoring
	GetMigrationStatus(pair string) (*engine.MigrationState, error)
	GetAllMigrationStates() map[string]*engine.MigrationState
	GetPerformanceMetrics(pair string) (map[string]interface{}, error)
	GetEngineMetrics() (*engine.EngineMetrics, error)

	// Auto-migration controls
	EnableAutoMigration(enabled bool) error
	IsAutoMigrationEnabled() bool

	// Metrics and reporting
	GetMetricsReport() (*engine.MetricsReport, error)
	GetPerformanceComparison(pair string) (*engine.PerformanceComparison, error)

	// Administrative controls
	ResetCircuitBreaker(pair string) error
	SetPerformanceThresholds(thresholds *engine.AutoMigrationThresholds) error

	// Shutdown and health check
	Shutdown(ctx context.Context) error
	HealthCheck() map[string]interface{}
}
