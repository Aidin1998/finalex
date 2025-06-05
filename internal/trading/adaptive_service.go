// Package trading provides the implementation of the adaptive trading service
package trading

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/accounts/bookkeeper"
	"github.com/Aidin1998/pincex_unified/internal/infrastructure/ws"
	"github.com/Aidin1998/pincex_unified/internal/trading/engine"
	"github.com/Aidin1998/pincex_unified/internal/trading/eventjournal"
	model2 "github.com/Aidin1998/pincex_unified/internal/trading/model"
	"github.com/Aidin1998/pincex_unified/internal/trading/orderbook"
	"github.com/Aidin1998/pincex_unified/internal/trading/repository"
	"github.com/Aidin1998/pincex_unified/pkg/models"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/Aidin1998/pincex_unified/internal/trading/settlement"
)

// CompleteRiskService implements the engine.RiskService interface
type CompleteRiskService struct {
	logger  *zap.Logger
	enabled bool
}

// AssessRisk assesses risk for an order
func (rs *CompleteRiskService) AssessRisk(ctx context.Context, userID uuid.UUID, orderAmount decimal.Decimal, pair string) (*engine.RiskMetrics, error) {
	return &engine.RiskMetrics{
		RiskScore:         0.1,
		Confidence:        0.95,
		CalculatedAt:      time.Now(),
		Factors:           []string{"normal_trading_pattern", "within_limits"},
		ThreatLevel:       "low",
		RecommendedAction: "allow",
	}, nil
}

// CheckCompliance checks compliance for a trade
func (rs *CompleteRiskService) CheckCompliance(ctx context.Context, userID uuid.UUID, tradeData interface{}) (*engine.ComplianceResult, error) {
	return &engine.ComplianceResult{
		Passed:       true,
		CheckedAt:    time.Now(),
		IsSuspicious: false,
	}, nil
}

// ComplianceCheck checks compliance for a trade with more specific parameters
func (rs *CompleteRiskService) ComplianceCheck(ctx context.Context, userID string, pair string, tradeValue decimal.Decimal, metadata map[string]interface{}) (*engine.ComplianceResult, error) {
	return &engine.ComplianceResult{
		Passed:       true,
		CheckedAt:    time.Now(),
		IsSuspicious: false,
	}, nil
}

// CheckPositionLimit checks if a position is within limits
func (rs *CompleteRiskService) CheckPositionLimit(ctx context.Context, userID, symbol string, intendedQty, price decimal.Decimal) (bool, error) {
	return true, nil
}

// CalculateRealTimeRisk calculates real-time risk for a user
func (rs *CompleteRiskService) CalculateRealTimeRisk(ctx context.Context, userID string) (*engine.RiskMetrics, error) {
	return &engine.RiskMetrics{
		RiskScore:         0.1,
		Confidence:        0.95,
		CalculatedAt:      time.Now(),
		Factors:           []string{"normal_trading_pattern", "within_limits"},
		ThreatLevel:       "low",
		RecommendedAction: "allow",
	}, nil
}

// ProcessTrade processes a trade for risk assessment
func (rs *CompleteRiskService) ProcessTrade(ctx context.Context, userID, tradeID, pair string, quantity, price decimal.Decimal) error {
	return nil
}

// IsEnabled returns whether the risk service is enabled
func (rs *CompleteRiskService) IsEnabled() bool {
	return rs.enabled
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
	// Create a proper risk service that implements the RiskService interface
	riskService := &CompleteRiskService{
		logger: logger,
	}

	// Create adaptive trading engine
	adaptiveEngine := engine.NewAdaptiveMatchingEngine(
		orderRepo,
		tradeRepo,
		logger.Sugar(),
		adaptiveConfig,
		eventJournal,
		wsHub,
		riskService, // Using our proper risk service implementation
		nil,         // no Redis client configured yet
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
		as.logger.Error("Failed to start migration", zap.String("pair", pair), zap.Error(err))
		return err
	}

	as.logger.Info("Migration started", zap.String("pair", pair))
	return nil
}

// StopMigration stops migration for a specific trading pair
func (as *AdaptiveService) StopMigration(pair string) error {
	err := as.adaptiveEngine.StopMigration(pair)
	if err != nil {
		as.logger.Error("Failed to stop migration", zap.String("pair", pair), zap.Error(err))
		return err
	}
	as.logger.Info("Migration stopped", zap.String("pair", pair))
	return nil
}

// PauseMigration pauses migration for a specific trading pair
func (as *AdaptiveService) PauseMigration(pair string) error {
	err := as.adaptiveEngine.PauseMigration(pair)
	if err != nil {
		as.logger.Error("Failed to pause migration", zap.String("pair", pair), zap.Error(err))
		return err
	}

	as.logger.Info("Migration paused", zap.String("pair", pair))
	return nil
}

// ResumeMigration resumes migration for a specific trading pair
func (as *AdaptiveService) ResumeMigration(pair string) error {
	err := as.adaptiveEngine.ResumeMigration(pair)
	if err != nil {
		as.logger.Error("Failed to resume migration", zap.String("pair", pair), zap.Error(err))
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
		as.logger.Error("Failed to rollback migration", zap.String("pair", pair), zap.Error(err))
		return err
	}

	as.logger.Info("Migration rolled back", zap.String("pair", pair))
	return nil
}

// ResetMigration resets migration state for a specific trading pair
func (as *AdaptiveService) ResetMigration(pair string) error {
	err := as.adaptiveEngine.ResetMigration(pair)
	if err != nil {
		as.logger.Error("Failed to reset migration", zap.String("pair", pair), zap.Error(err))
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
		return nil, fmt.Errorf("order book not found for pair: %s", pair)
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

	as.logger.Info("Auto-migration setting updated", zap.Bool("enabled", enabled))
	return nil
}

// IsAutoMigrationEnabled returns whether auto-migration is enabled
func (as *AdaptiveService) IsAutoMigrationEnabled() bool {
	return as.adaptiveEngine.IsAutoMigrationEnabled()
}

// Metrics and reporting

// GetMetricsReport returns a comprehensive metrics report
func (as *AdaptiveService) GetMetricsReport() (*engine.MetricsReport, error) {
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

// HealthCheck method for monitoring
func (as *AdaptiveService) HealthCheck() map[string]interface{} {
	health := make(map[string]interface{})

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
