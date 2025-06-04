// =============================
// Adaptive Trading Service Integration
// =============================
// This file provides service layer integration for the adaptive trading engine,
// allowing seamless migration between old and new implementations with
// comprehensive monitoring and administrative controls.

package trading

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/accounts/bookkeeper"
	"github.com/Aidin1998/pincex_unified/internal/infrastructure/ws"
	"github.com/Aidin1998/pincex_unified/internal/risk/compliance/aml"
	"github.com/Aidin1998/pincex_unified/internal/trading/engine"
	"github.com/Aidin1998/pincex_unified/internal/trading/eventjournal"
	model2 "github.com/Aidin1998/pincex_unified/internal/trading/model"
	"github.com/Aidin1998/pincex_unified/internal/trading/orderbook"
	"github.com/Aidin1998/pincex_unified/internal/trading/repository"
	"github.com/Aidin1998/pincex_unified/pkg/models"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/Aidin1998/pincex_unified/internal/trading/settlement"
)

// AdaptiveTradingService extends TradingService with adaptive engine capabilities
type AdaptiveTradingService interface {
	TradingService // Embed existing interface

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

// AdaptiveService implements AdaptiveTradingService
type AdaptiveService struct {
	*Service // Embed existing service for TradingService methods

	adaptiveEngine *engine.AdaptiveMatchingEngine
	adaptiveConfig *engine.AdaptiveEngineConfig
	logger         *zap.Logger

	// Alert handling
	alertHandler func(engine.AlertType, engine.AlertSeverity, string)
}

// NewAdaptiveService creates a new adaptive trading service
func NewAdaptiveService(logger *zap.Logger, db *gorm.DB, bookkeeperSvc bookkeeper.BookkeeperService, adaptiveConfig *engine.AdaptiveEngineConfig, wsHub *ws.Hub) (AdaptiveTradingService, error) {
	// Initialize settlement engine
	settlementEngine := settlement.NewSettlementEngine()
	// Initialize base trading service
	baseSvcIface, err := NewService(logger, db, bookkeeperSvc, wsHub, settlementEngine)
	if err != nil {
		return nil, fmt.Errorf("failed to create base trading service: %w", err)
	}
	baseSvc := baseSvcIface.(*Service)

	// Initialize repositories
	orderRepo := repository.NewGormRepository(db, logger)
	tradeRepo := repository.NewGormTradeRepository(db, logger)
	// Validate adaptive configuration
	validator := engine.NewValidation()
	if err := validator.ValidateConfig(adaptiveConfig); err != nil {
		return nil, fmt.Errorf("invalid adaptive configuration: %w", err)
	}

	// Initialize event journal for adaptive engine
	eventJournal, err := eventjournal.NewEventJournal(logger.Sugar(), "./logs/trading/adaptive_events.log")
	if err != nil {
		return nil, fmt.Errorf("failed to create event journal: %w", err)
	}
	// Create adaptive trading engine
	adaptiveEngine := engine.NewAdaptiveMatchingEngine(
		orderRepo,
		tradeRepo,
		logger.Sugar(),
		adaptiveConfig,
		eventJournal,
		wsHub,
		aml.NewRiskService(),
		nil, // no Redis client configured yet
	)

	// Create adaptive service
	adaptiveSvc := &AdaptiveService{
		Service:        baseSvc,
		adaptiveEngine: adaptiveEngine,
		adaptiveConfig: adaptiveConfig,
		logger:         logger,
	}

	// Set up alert handling
	adaptiveSvc.setupAlertHandling()

	logger.Info("Adaptive trading service created successfully",
		zap.Bool("adaptive_enabled", adaptiveConfig.EnableAdaptiveOrderBooks),
		zap.Bool("auto_migration_enabled", adaptiveConfig.AutoMigrationEnabled),
	)

	return adaptiveSvc, nil
}

// setupAlertHandling configures alert handling for the adaptive engine
func (as *AdaptiveService) setupAlertHandling() {
	as.alertHandler = func(alertType engine.AlertType, severity engine.AlertSeverity, message string) {
		as.logger.Warn("Adaptive engine alert",
			zap.String("type", string(alertType)),
			zap.String("severity", string(severity)),
			zap.String("message", message),
		)

		// Could integrate with external alerting systems here
		// e.g., send to Slack, PagerDuty, etc.
	}
}

// Migration control methods

// StartMigration starts migration for a specific trading pair
func (as *AdaptiveService) StartMigration(pair string) error {
	if !as.adaptiveConfig.EnableAdaptiveOrderBooks {
		return fmt.Errorf("adaptive order books are not enabled")
	}

	// Use configured initial migration percentage
	percentage := as.adaptiveConfig.DefaultMigrationConfig.MigrationPercentage
	err := as.adaptiveEngine.StartMigration(pair, percentage)
	if err != nil {
		as.logger.Error("Failed to start migration",
			zap.String("pair", pair),
			zap.Error(err),
		)
		return err
	}

	as.logger.Info("Migration started", zap.String("pair", pair))
	return nil
}

// StopMigration stops migration for a specific trading pair
func (as *AdaptiveService) StopMigration(pair string) error {
	err := as.adaptiveEngine.StopMigration(pair)
	if err != nil {
		as.logger.Error("Failed to stop migration",
			zap.String("pair", pair), zap.Error(err),
		)
		return err
	}
	as.logger.Info("Migration stopped", zap.String("pair", pair))
	return nil
}

// PauseMigration pauses migration for a specific trading pair
func (as *AdaptiveService) PauseMigration(pair string) error {
	err := as.adaptiveEngine.PauseMigration(pair)
	if err != nil {
		as.logger.Error("Failed to pause migration",
			zap.String("pair", pair),
			zap.Error(err),
		)
		return err
	}

	as.logger.Info("Migration paused", zap.String("pair", pair))
	return nil
}

// ResumeMigration resumes migration for a specific trading pair
func (as *AdaptiveService) ResumeMigration(pair string) error {
	err := as.adaptiveEngine.ResumeMigration(pair)
	if err != nil {
		as.logger.Error("Failed to resume migration",
			zap.String("pair", pair),
			zap.Error(err),
		)
		return err
	}

	as.logger.Info("Migration resumed", zap.String("pair", pair))
	return nil
}

// SetMigrationPercentage sets the migration percentage for a specific trading pair
func (as *AdaptiveService) SetMigrationPercentage(pair string, percentage int32) error {
	if percentage < 0 || percentage > 100 {
		return fmt.Errorf("migration percentage must be between 0 and 100")
	}

	err := as.adaptiveEngine.SetMigrationPercentage(pair, percentage)
	if err != nil {
		as.logger.Error("Failed to set migration percentage",
			zap.String("pair", pair),
			zap.Int32("percentage", percentage),
			zap.Error(err),
		)
		return err
	}

	as.logger.Info("Migration percentage updated",
		zap.String("pair", pair),
		zap.Int32("percentage", percentage),
	)
	return nil
}

// RollbackMigration rolls back migration for a specific trading pair
func (as *AdaptiveService) RollbackMigration(pair string) error {
	err := as.adaptiveEngine.RollbackMigration(pair)
	if err != nil {
		as.logger.Error("Failed to rollback migration",
			zap.String("pair", pair),
			zap.Error(err),
		)
		return err
	}

	as.logger.Info("Migration rolled back", zap.String("pair", pair))
	return nil
}

// ResetMigration resets migration state for a specific trading pair
func (as *AdaptiveService) ResetMigration(pair string) error {
	err := as.adaptiveEngine.ResetMigration(pair)
	if err != nil {
		as.logger.Error("Failed to reset migration",
			zap.String("pair", pair),
			zap.Error(err),
		)
		return err
	}

	as.logger.Info("Migration reset", zap.String("pair", pair))
	return nil
}

// Migration status and monitoring

// GetMigrationStatus returns the migration status for a specific trading pair
func (as *AdaptiveService) GetMigrationStatus(pair string) (*engine.MigrationState, error) {
	state, err := as.adaptiveEngine.GetMigrationState(pair)
	if err != nil {
		return nil, err
	}
	if state == nil {
		return nil, fmt.Errorf("no migration state found for pair: %s", pair)
	}
	return state, nil
}

// GetAllMigrationStates returns migration states for all pairs
func (as *AdaptiveService) GetAllMigrationStates() map[string]*engine.MigrationState {
	return as.adaptiveEngine.GetAllMigrationStates()
}

// GetPerformanceMetrics returns performance metrics for a specific trading pair
func (as *AdaptiveService) GetPerformanceMetrics(pair string) (map[string]interface{}, error) {
	orderBook := as.adaptiveEngine.GetOrderBook(pair)
	if orderBook == nil {
		return nil, fmt.Errorf("no order book found for pair: %s", pair)
	}

	// If it's an adaptive order book, get its metrics
	if adaptiveOB, ok := orderBook.(*orderbook.AdaptiveOrderBook); ok {
		return adaptiveOB.GetPerformanceMetrics(), nil
	}

	// For regular order books, return basic metrics (if available)
	return map[string]interface{}{
		"type":    "standard",
		"message": "performance metrics not available for standard order book",
	}, nil
}

// GetEngineMetrics returns overall engine metrics
func (as *AdaptiveService) GetEngineMetrics() (*engine.EngineMetrics, error) {
	metricsCollector := as.adaptiveEngine.GetMetricsCollector()
	if metricsCollector == nil {
		return nil, fmt.Errorf("metrics collector not available")
	}

	return metricsCollector.GetEngineMetrics(), nil
}

// Auto-migration controls

// EnableAutoMigration enables or disables automatic migration
func (as *AdaptiveService) EnableAutoMigration(enabled bool) error {
	as.adaptiveEngine.SetAutoMigrationEnabled(enabled)

	as.logger.Info("Auto-migration setting updated",
		zap.Bool("enabled", enabled),
	)
	return nil
}

// IsAutoMigrationEnabled returns whether auto-migration is enabled
func (as *AdaptiveService) IsAutoMigrationEnabled() bool {
	return as.adaptiveEngine.IsAutoMigrationEnabled()
}

// Metrics and reporting

// GetMetricsReport returns a comprehensive metrics report
func (as *AdaptiveService) GetMetricsReport() (*engine.MetricsReport, error) {
	// This would typically be received from the metrics reporting channel
	// For now, we'll generate one on demand
	select {
	case report := <-as.adaptiveEngine.GetMetricsReportChan():
		return &report, nil
	case <-time.After(time.Second * 5):
		return nil, fmt.Errorf("timeout waiting for metrics report")
	}
}

// GetPerformanceComparison returns performance comparison for a specific pair
func (as *AdaptiveService) GetPerformanceComparison(pair string) (*engine.PerformanceComparison, error) {
	state, err := as.adaptiveEngine.GetMigrationState(pair)
	if err != nil {
		return nil, err
	}
	if state == nil {
		return nil, fmt.Errorf("no migration state found for pair: %s", pair)
	}
	return state.PerformanceComparison, nil
}

// Administrative controls

// ResetCircuitBreaker resets the circuit breaker for a specific trading pair
func (as *AdaptiveService) ResetCircuitBreaker(pair string) error {
	orderBook := as.adaptiveEngine.GetOrderBook(pair)
	if orderBook == nil {
		return fmt.Errorf("no order book found for pair: %s", pair)
	}

	// If it's an adaptive order book, reset its circuit breaker
	if adaptiveOB, ok := orderBook.(*orderbook.AdaptiveOrderBook); ok {
		adaptiveOB.ResetCircuitBreaker()
		as.logger.Info("Circuit breaker reset", zap.String("pair", pair))
		return nil
	}

	return fmt.Errorf("circuit breaker not available for standard order book")
}

// SetPerformanceThresholds updates performance thresholds for auto-migration
func (as *AdaptiveService) SetPerformanceThresholds(thresholds *engine.AutoMigrationThresholds) error {
	if thresholds == nil {
		return fmt.Errorf("thresholds cannot be nil")
	}

	// Update the configuration
	as.adaptiveConfig.AutoMigrationThresholds = thresholds

	as.logger.Info("Performance thresholds updated",
		zap.Float64("latency_p95_threshold_ms", thresholds.LatencyP95ThresholdMs),
		zap.Float64("throughput_degradation_pct", thresholds.ThroughputDegradationPct),
		zap.Float64("error_rate_threshold", thresholds.ErrorRateThreshold),
	)

	return nil
}

// Enhanced order processing methods that leverage adaptive features

// PlaceOrder overrides the base method to use adaptive routing
func (as *AdaptiveService) PlaceOrder(ctx context.Context, order *models.Order) (*models.Order, error) {
	// Perform the same validation as the base service
	// (This could be extracted to a shared validation method)

	// Convert to internal model
	internalOrder := toModelOrder(order)
	if internalOrder == nil {
		return nil, fmt.Errorf("failed to convert order")
	}

	// Generate order ID if not provided
	if internalOrder.ID == uuid.Nil {
		internalOrder.ID = uuid.New()
	}

	// Set order status and timestamps
	internalOrder.Status = model2.OrderStatusNew
	internalOrder.CreatedAt = time.Now()
	internalOrder.UpdatedAt = time.Now()

	// Use adaptive engine for processing
	processedOrder, trades, restingOrders, err := as.adaptiveEngine.ProcessOrder(
		ctx,
		internalOrder,
		engine.OrderSourceType("API"),
	)

	if err != nil {
		as.logger.Error("Failed to process order",
			zap.String("order_id", internalOrder.ID.String()),
			zap.String("pair", internalOrder.Pair),
			zap.Error(err),
		)
		return nil, err
	}

	// Log successful processing
	as.logger.Info("Order processed successfully",
		zap.String("order_id", processedOrder.ID.String()),
		zap.String("pair", processedOrder.Pair),
		zap.String("status", processedOrder.Status),
		zap.Int("trades_count", len(trades)),
		zap.Int("resting_orders_count", len(restingOrders)),
	)

	// Convert back to API model
	return toAPIOrder(processedOrder), nil
}

// GetOrderBook overrides the base method to use adaptive order book
func (as *AdaptiveService) GetOrderBook(symbol string, depth int) (*models.OrderBookSnapshot, error) {
	if symbol == "" {
		return nil, fmt.Errorf("symbol is required")
	}
	if depth <= 0 {
		depth = 20
	}
	orderBook := as.adaptiveEngine.GetOrderBook(symbol)
	if orderBook == nil {
		return nil, fmt.Errorf("order book not found for symbol: %s", symbol)
	}
	// Get bids and asks snapshots
	bids, asks := orderBook.GetSnapshot(depth)
	apiSnapshot := &models.OrderBookSnapshot{
		Symbol:     symbol,
		UpdateTime: time.Now(),
		Bids:       make([]models.OrderBookLevel, len(bids)),
		Asks:       make([]models.OrderBookLevel, len(asks)),
	}
	for i, lvl := range bids {
		price, _ := strconv.ParseFloat(lvl[0], 64)
		volume, _ := strconv.ParseFloat(lvl[1], 64)
		apiSnapshot.Bids[i] = models.OrderBookLevel{Price: price, Volume: volume}
	}
	for i, lvl := range asks {
		price, _ := strconv.ParseFloat(lvl[0], 64)
		volume, _ := strconv.ParseFloat(lvl[1], 64)
		apiSnapshot.Asks[i] = models.OrderBookLevel{Price: price, Volume: volume}
	}
	return apiSnapshot, nil
}

// Utility methods

// GetMetricsCollector returns the metrics collector for external use
func (as *AdaptiveService) GetMetricsCollector() *engine.EngineMetricsCollector {
	return as.adaptiveEngine.GetMetricsCollector()
}

// GetAdaptiveEngine returns the adaptive engine for advanced operations
func (as *AdaptiveService) GetAdaptiveEngine() *engine.AdaptiveMatchingEngine {
	return as.adaptiveEngine
}

// Shutdown gracefully shuts down the adaptive service
func (as *AdaptiveService) Shutdown(ctx context.Context) error {
	as.logger.Info("Shutting down adaptive trading service")

	// Shutdown adaptive engine
	if err := as.adaptiveEngine.Shutdown(ctx); err != nil {
		as.logger.Error("Error shutting down adaptive engine", zap.Error(err))
		return err
	}

	// Call base service Stop
	if err := as.Service.Stop(); err != nil {
		as.logger.Error("Error stopping base service", zap.Error(err))
		return err
	}

	as.logger.Info("Adaptive trading service shutdown complete")
	return nil
}

// Health check method for monitoring
func (as *AdaptiveService) HealthCheck() map[string]interface{} {
	health := make(map[string]interface{}) // initialize health map directly

	// add adaptive fields
	health["adaptive_enabled"] = as.adaptiveConfig.EnableAdaptiveOrderBooks
	health["auto_migration"] = as.IsAutoMigrationEnabled()

	states := as.GetAllMigrationStates()
	health["migration_states_count"] = len(states)

	activeMigrations := 0
	for _, state := range states {
		if state.CurrentPercentage > 0 && state.CurrentPercentage < 100 {
			activeMigrations++
		}
	}
	health["active_migrations"] = activeMigrations

	// Add engine metrics if available
	if metrics, err := as.GetEngineMetrics(); err == nil {
		health["total_orders_processed"] = metrics.TotalOrdersProcessed
		health["total_trades_executed"] = metrics.TotalTradesExecuted
		health["avg_processing_time_ms"] = metrics.AvgProcessingTimeMs
		health["throughput_ops_per_sec"] = metrics.ThroughputOpsPerSec
		health["error_rate"] = metrics.ErrorRate
	}

	return health
}
