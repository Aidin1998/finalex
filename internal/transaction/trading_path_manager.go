// Package transaction provides hot/warm/cold path trading architecture
package transaction

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/trading/engine"
	"github.com/Aidin1998/pincex_unified/internal/trading/model"
	"github.com/Aidin1998/pincex_unified/pkg/models"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// TradingPathManager manages hot/warm/cold trading paths
type TradingPathManager struct {
	// Hot path - direct matching engine (microsecond latency)
	matchingEngine *engine.MatchingEngine

	// Warm path - async settlement queue
	settlementQueue chan *TradeSettlementTask

	// Cold path - reconciliation and error handling
	reconciliationQueue chan *ReconciliationTask

	// XA manager for settlement (warm path only)
	xaManager *XATransactionManager

	// Services
	db     *gorm.DB
	logger *zap.Logger

	// Control
	mu       sync.RWMutex
	shutdown chan struct{}
	wg       sync.WaitGroup

	// Metrics
	metrics *TradingPathMetrics
}

// TradeSettlementTask represents a trade that needs XA settlement
type TradeSettlementTask struct {
	TradeID     uuid.UUID              `json:"trade_id"`
	OrderID     uuid.UUID              `json:"order_id"`
	Symbol      string                 `json:"symbol"`
	BuyUserID   uuid.UUID              `json:"buy_user_id"`
	SellUserID  uuid.UUID              `json:"sell_user_id"`
	Quantity    decimal.Decimal        `json:"quantity"`
	Price       decimal.Decimal        `json:"price"`
	TakerSide   string                 `json:"taker_side"`
	Timestamp   time.Time              `json:"timestamp"`
	Metadata    map[string]interface{} `json:"metadata"`
	Retries     int                    `json:"retries"`
	MaxRetries  int                    `json:"max_retries"`
	LastAttempt time.Time              `json:"last_attempt"`
}

// ReconciliationTask represents a task that needs reconciliation
type ReconciliationTask struct {
	Type        string                 `json:"type"`
	Description string                 `json:"description"`
	Data        map[string]interface{} `json:"data"`
	Timestamp   time.Time              `json:"timestamp"`
	Priority    int                    `json:"priority"`
	Retries     int                    `json:"retries"`
	MaxRetries  int                    `json:"max_retries"`
}

// TradingPathMetrics tracks performance across all paths
type TradingPathMetrics struct {
	mu sync.RWMutex

	// Hot path metrics
	HotPathOrders     int64         `json:"hot_path_orders"`
	HotPathLatencyP50 time.Duration `json:"hot_path_latency_p50"`
	HotPathLatencyP99 time.Duration `json:"hot_path_latency_p99"`
	HotPathThroughput float64       `json:"hot_path_throughput"`

	// Warm path metrics
	WarmPathTrades  int64         `json:"warm_path_trades"`
	WarmPathLatency time.Duration `json:"warm_path_latency"`
	WarmPathBacklog int64         `json:"warm_path_backlog"`
	WarmPathErrors  int64         `json:"warm_path_errors"`

	// Cold path metrics
	ColdPathTasks        int64         `json:"cold_path_tasks"`
	ColdPathLatency      time.Duration `json:"cold_path_latency"`
	ReconciliationErrors int64         `json:"reconciliation_errors"`
}

// NewTradingPathManager creates a new trading path manager
func NewTradingPathManager(
	matchingEngine *engine.MatchingEngine,
	xaManager *XATransactionManager,
	db *gorm.DB,
	logger *zap.Logger,
) *TradingPathManager {
	return &TradingPathManager{
		matchingEngine:      matchingEngine,
		settlementQueue:     make(chan *TradeSettlementTask, 10000),
		reconciliationQueue: make(chan *ReconciliationTask, 1000),
		xaManager:           xaManager,
		db:                  db,
		logger:              logger,
		shutdown:            make(chan struct{}),
		metrics:             &TradingPathMetrics{},
	}
}

// Start initializes all trading paths
func (tpm *TradingPathManager) Start(ctx context.Context) error {
	tpm.logger.Info("Starting Trading Path Manager")

	// Start warm path workers (settlement)
	for i := 0; i < 5; i++ {
		tpm.wg.Add(1)
		go tpm.warmPathWorker(ctx, i)
	}

	// Start cold path workers (reconciliation)
	for i := 0; i < 2; i++ {
		tpm.wg.Add(1)
		go tpm.coldPathWorker(ctx, i)
	}

	// Start metrics collector
	tpm.wg.Add(1)
	go tpm.metricsCollector(ctx)

	return nil
}

// Stop gracefully shuts down all trading paths
func (tpm *TradingPathManager) Stop(ctx context.Context) error {
	tpm.logger.Info("Stopping Trading Path Manager")

	close(tpm.shutdown)

	// Wait for workers to finish with timeout
	done := make(chan struct{})
	go func() {
		tpm.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		tpm.logger.Info("Trading Path Manager stopped gracefully")
	case <-time.After(30 * time.Second):
		tpm.logger.Warn("Trading Path Manager shutdown timeout")
	}

	return nil
}

// HOT PATH: PlaceOrderHotPath places orders directly in matching engine (microsecond latency)
func (tpm *TradingPathManager) PlaceOrderHotPath(ctx context.Context, order *models.Order) (*models.Order, error) {
	start := time.Now()
	defer func() {
		latency := time.Since(start)
		tpm.updateHotPathMetrics(latency)
	}()
	// Direct matching engine placement - no XA transaction overhead
	matchOrder := &model.Order{
		ID:        order.ID,
		UserID:    order.UserID,
		Pair:      order.Symbol,
		Side:      order.Side,
		Type:      order.Type,
		Quantity:  decimal.NewFromFloat(order.Quantity),
		Price:     decimal.NewFromFloat(order.Price),
		Status:    order.Status,
		CreatedAt: order.CreatedAt,
	}
	// Place order in matching engine (hot path)
	result, trades, _, err := tpm.matchingEngine.ProcessOrder(context.Background(), matchOrder, "hot_path")
	if err != nil {
		return nil, fmt.Errorf("hot path order placement failed: %w", err)
	}

	// Update order status
	order.Status = result.Status
	order.FilledQuantity = result.FilledQuantity.InexactFloat64()
	order.UpdatedAt = time.Now()

	// If trades occurred, queue them for warm path settlement
	if len(trades) > 0 {
		for _, trade := range trades {
			task := &TradeSettlementTask{
				TradeID:    trade.ID,
				OrderID:    order.ID,
				Symbol:     trade.Pair,
				BuyUserID:  order.UserID, // Use order's user ID as placeholder
				SellUserID: order.UserID, // Use order's user ID as placeholder
				Quantity:   trade.Quantity,
				Price:      trade.Price,
				TakerSide:  trade.Side,
				Timestamp:  trade.CreatedAt,
				MaxRetries: 3,
				Metadata: map[string]interface{}{
					"order_id": trade.OrderID,
					"maker":    trade.Maker,
				},
			}

			// Non-blocking queue to warm path
			select {
			case tpm.settlementQueue <- task:
				// Successfully queued for settlement
			default:
				// Queue full - send to cold path for reconciliation
				tpm.queueReconciliation("settlement_queue_full", "Settlement queue overflow", map[string]interface{}{
					"trade_id": trade.ID,
					"order_id": order.ID,
				})
			}
		}
	}

	return order, nil
}

// CancelOrderHotPath cancels orders directly in matching engine
func (tpm *TradingPathManager) CancelOrderHotPath(ctx context.Context, orderID uuid.UUID, userID uuid.UUID) error {
	start := time.Now()
	defer func() {
		latency := time.Since(start)
		tpm.updateHotPathMetrics(latency)
	}()
	// Direct cancellation in matching engine
	cancelReq := &engine.CancelRequest{
		OrderID: orderID,
		UserID:  userID,
	}
	err := tpm.matchingEngine.CancelOrder(cancelReq)
	if err != nil {
		return fmt.Errorf("hot path order cancellation failed: %w", err)
	}

	return nil
}

// WARM PATH: Settlement worker that processes trades with XA transactions
func (tpm *TradingPathManager) warmPathWorker(ctx context.Context, workerID int) {
	defer tpm.wg.Done()

	logger := tpm.logger.With(zap.Int("warm_worker_id", workerID))
	logger.Info("Started warm path settlement worker")

	for {
		select {
		case <-tpm.shutdown:
			logger.Info("Warm path worker shutting down")
			return

		case task := <-tpm.settlementQueue:
			start := time.Now()

			if err := tpm.processTradeSettlement(ctx, task); err != nil {
				logger.Error("Trade settlement failed",
					zap.String("trade_id", task.TradeID.String()),
					zap.Error(err))

				// Retry logic
				task.Retries++
				task.LastAttempt = time.Now()

				if task.Retries < task.MaxRetries {
					// Exponential backoff retry
					time.Sleep(time.Duration(task.Retries) * time.Second)

					select {
					case tpm.settlementQueue <- task:
						// Retry queued
					default:
						// Queue for cold path reconciliation
						tpm.queueReconciliation("settlement_retry_failed", "Failed to retry settlement", map[string]interface{}{
							"trade_id": task.TradeID,
							"retries":  task.Retries,
							"error":    err.Error(),
						})
					}
				} else {
					// Max retries exceeded - send to cold path
					tpm.queueReconciliation("settlement_max_retries", "Settlement max retries exceeded", map[string]interface{}{
						"trade_id": task.TradeID,
						"retries":  task.Retries,
						"error":    err.Error(),
					})
				}

				tpm.updateWarmPathErrorMetrics()
			} else {
				tpm.updateWarmPathMetrics(time.Since(start))
			}
		}
	}
}

// processTradeSettlement handles trade settlement with XA transactions
func (tpm *TradingPathManager) processTradeSettlement(ctx context.Context, task *TradeSettlementTask) error {
	// Start XA transaction for settlement
	txn, err := tpm.xaManager.Start(ctx, 30*time.Second)
	if err != nil {
		return fmt.Errorf("failed to start XA transaction: %w", err)
	}

	// Add transaction to context
	ctx = WithXATransaction(ctx, txn)

	// This would enlist bookkeeper and settlement resources
	// (Implementation details would be in the actual settlement logic)
	// For now, simulate settlement
	tpm.logger.Debug("Processing trade settlement",
		zap.String("trade_id", task.TradeID.String()),
		zap.String("symbol", task.Symbol),
		zap.String("quantity", task.Quantity.String()),
		zap.String("price", task.Price.String()))

	// Simulate settlement work
	time.Sleep(10 * time.Millisecond)

	// Commit XA transaction
	return tpm.xaManager.Commit(ctx, txn)
}

// COLD PATH: Reconciliation worker for error handling and data consistency
func (tpm *TradingPathManager) coldPathWorker(ctx context.Context, workerID int) {
	defer tpm.wg.Done()

	logger := tpm.logger.With(zap.Int("cold_worker_id", workerID))
	logger.Info("Started cold path reconciliation worker")

	for {
		select {
		case <-tpm.shutdown:
			logger.Info("Cold path worker shutting down")
			return

		case task := <-tpm.reconciliationQueue:
			start := time.Now()

			if err := tpm.processReconciliation(ctx, task); err != nil {
				logger.Error("Reconciliation task failed",
					zap.String("type", task.Type),
					zap.Error(err))
				tpm.updateColdPathErrorMetrics()
			} else {
				tpm.updateColdPathMetrics(time.Since(start))
			}
		}
	}
}

// processReconciliation handles reconciliation tasks
func (tpm *TradingPathManager) processReconciliation(ctx context.Context, task *ReconciliationTask) error {
	tpm.logger.Info("Processing reconciliation task",
		zap.String("type", task.Type),
		zap.String("description", task.Description))

	switch task.Type {
	case "settlement_queue_full":
		// Handle settlement queue overflow
		return tpm.handleSettlementQueueOverflow(ctx, task)

	case "settlement_retry_failed":
		// Handle failed settlement retries
		return tpm.handleFailedSettlement(ctx, task)

	case "data_inconsistency":
		// Handle data inconsistency issues
		return tpm.handleDataInconsistency(ctx, task)

	default:
		tpm.logger.Warn("Unknown reconciliation task type", zap.String("type", task.Type))
		return nil
	}
}

// Helper methods for reconciliation
func (tpm *TradingPathManager) handleSettlementQueueOverflow(ctx context.Context, task *ReconciliationTask) error {
	// Implement overflow handling logic
	tpm.logger.Warn("Handling settlement queue overflow", zap.Any("data", task.Data))
	return nil
}

func (tpm *TradingPathManager) handleFailedSettlement(ctx context.Context, task *ReconciliationTask) error {
	// Implement failed settlement handling
	tpm.logger.Warn("Handling failed settlement", zap.Any("data", task.Data))
	return nil
}

func (tpm *TradingPathManager) handleDataInconsistency(ctx context.Context, task *ReconciliationTask) error {
	// Implement data inconsistency handling
	tpm.logger.Warn("Handling data inconsistency", zap.Any("data", task.Data))
	return nil
}

// queueReconciliation queues a task for cold path reconciliation
func (tpm *TradingPathManager) queueReconciliation(taskType, description string, data map[string]interface{}) {
	task := &ReconciliationTask{
		Type:        taskType,
		Description: description,
		Data:        data,
		Timestamp:   time.Now(),
		Priority:    1,
		MaxRetries:  3,
	}

	select {
	case tpm.reconciliationQueue <- task:
		// Successfully queued
	default:
		// Critical: reconciliation queue is full
		tpm.logger.Error("Reconciliation queue overflow - critical system state",
			zap.String("task_type", taskType),
			zap.String("description", description))
	}
}

// Metrics update methods
func (tpm *TradingPathManager) updateHotPathMetrics(latency time.Duration) {
	tpm.metrics.mu.Lock()
	defer tpm.metrics.mu.Unlock()

	tpm.metrics.HotPathOrders++
	// Update latency percentiles (simplified)
	tpm.metrics.HotPathLatencyP99 = latency
}

func (tpm *TradingPathManager) updateWarmPathMetrics(latency time.Duration) {
	tpm.metrics.mu.Lock()
	defer tpm.metrics.mu.Unlock()

	tpm.metrics.WarmPathTrades++
	tpm.metrics.WarmPathLatency = latency
	tpm.metrics.WarmPathBacklog = int64(len(tpm.settlementQueue))
}

func (tpm *TradingPathManager) updateWarmPathErrorMetrics() {
	tpm.metrics.mu.Lock()
	defer tpm.metrics.mu.Unlock()

	tpm.metrics.WarmPathErrors++
}

func (tpm *TradingPathManager) updateColdPathMetrics(latency time.Duration) {
	tpm.metrics.mu.Lock()
	defer tpm.metrics.mu.Unlock()

	tpm.metrics.ColdPathTasks++
	tpm.metrics.ColdPathLatency = latency
}

func (tpm *TradingPathManager) updateColdPathErrorMetrics() {
	tpm.metrics.mu.Lock()
	defer tpm.metrics.mu.Unlock()

	tpm.metrics.ReconciliationErrors++
}

// metricsCollector periodically logs metrics
func (tpm *TradingPathManager) metricsCollector(ctx context.Context) {
	defer tpm.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-tpm.shutdown:
			return
		case <-ticker.C:
			tpm.logMetrics()
		}
	}
}

func (tpm *TradingPathManager) logMetrics() {
	tpm.metrics.mu.RLock()
	defer tpm.metrics.mu.RUnlock()

	tpm.logger.Info("Trading Path Metrics",
		zap.Int64("hot_path_orders", tpm.metrics.HotPathOrders),
		zap.Duration("hot_path_latency_p99", tpm.metrics.HotPathLatencyP99),
		zap.Int64("warm_path_trades", tpm.metrics.WarmPathTrades),
		zap.Duration("warm_path_latency", tpm.metrics.WarmPathLatency),
		zap.Int64("warm_path_backlog", tpm.metrics.WarmPathBacklog),
		zap.Int64("warm_path_errors", tpm.metrics.WarmPathErrors),
		zap.Int64("cold_path_tasks", tpm.metrics.ColdPathTasks),
		zap.Int64("reconciliation_errors", tpm.metrics.ReconciliationErrors))
}

// GetMetrics returns current metrics
func (tpm *TradingPathManager) GetMetrics() *TradingPathMetrics {
	tpm.metrics.mu.RLock()
	defer tpm.metrics.mu.RUnlock()

	// Return a copy to avoid race conditions
	return &TradingPathMetrics{
		HotPathOrders:        tpm.metrics.HotPathOrders,
		HotPathLatencyP50:    tpm.metrics.HotPathLatencyP50,
		HotPathLatencyP99:    tpm.metrics.HotPathLatencyP99,
		HotPathThroughput:    tpm.metrics.HotPathThroughput,
		WarmPathTrades:       tpm.metrics.WarmPathTrades,
		WarmPathLatency:      tpm.metrics.WarmPathLatency,
		WarmPathBacklog:      tpm.metrics.WarmPathBacklog,
		WarmPathErrors:       tpm.metrics.WarmPathErrors,
		ColdPathTasks:        tpm.metrics.ColdPathTasks,
		ColdPathLatency:      tpm.metrics.ColdPathLatency,
		ReconciliationErrors: tpm.metrics.ReconciliationErrors,
	}
}
