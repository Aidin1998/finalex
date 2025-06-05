// Package trading provides trading services and interfaces
package trading

import (
	"github.com/Aidin1998/pincex_unified/internal/trading/engine"
)

// This file contains type aliases to break circular dependencies between
// trading and engine packages

// MigrationState is a simple alias to the engine.MigrationState
type MigrationState = engine.MigrationState

// EngineMetrics is a simple alias to the engine.EngineMetrics
type EngineMetrics = engine.EngineMetrics

// MetricsReport is a simple alias to the engine.MetricsReport
type MetricsReport = engine.MetricsReport

// PerformanceComparison is a simple alias to the engine.PerformanceComparison
type PerformanceComparison = engine.PerformanceComparison

// AutoMigrationThresholds is a simple alias to the engine.AutoMigrationThresholds
type AutoMigrationThresholds = engine.AutoMigrationThresholds

// AlertType is a simple alias to the engine.AlertType
type AlertType = engine.AlertType

// AlertSeverity is a simple alias to the engine.AlertSeverity
type AlertSeverity = engine.AlertSeverity
