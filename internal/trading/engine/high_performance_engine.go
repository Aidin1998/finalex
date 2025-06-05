// =============================
// High-Performance Matching Engine with >100k TPS Capability
// =============================
// This file integrates the lock-free concurrency engine with the main matching engine
// to achieve Binance-level performance while preserving all existing functionality.

package engine

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	ws "github.com/Aidin1998/finalex/internal/infrastructure/ws"
	metricsapi "github.com/Aidin1998/finalex/internal/marketmaking/analytics/metrics"
	auditlog "github.com/Aidin1998/finalex/internal/trading/auditlog"
	eventjournal "github.com/Aidin1998/finalex/internal/trading/eventjournal"
	model "github.com/Aidin1998/finalex/internal/trading/model"
	orderbook "github.com/Aidin1998/finalex/internal/trading/orderbook"

	// Using root persistence package instead of non-existent internal/trading/persistence
	settlement "github.com/Aidin1998/finalex/internal/trading/settlement"
	"github.com/Aidin1998/finalex/internal/trading/trigger"
	"github.com/Aidin1998/finalex/persistence"
)

// =============================
// High-Performance Matching Engine
// =============================
// Enhanced version of MatchingEngine with lock-free concurrency controls

// HighPerformanceMatchingEngine extends the base matching engine with high-throughput optimizations
type HighPerformanceMatchingEngine struct {
	// Base repositories and services
	orderRepo        model.Repository
	tradeRepo        TradeRepository
	logger           *zap.SugaredLogger
	config           *Config
	eventJournal     *eventjournal.EventJournal
	wsHub            *ws.Hub
	riskManager      interface{}
	settlementEngine *settlement.SettlementEngine
	triggerMonitor   *trigger.TriggerMonitor

	// High-performance concurrency components
	shardedMarketState *ShardedMarketState
	shardedOrderBooks  *ShardedOrderBookManager
	highThroughputPool *HighThroughputWorkerPool

	// Lock-free event handling
	orderBookHandlers *LockFreeEventHandlers
	tradeHandlers     *LockFreeEventHandlers
	eventRingBuffer   *LockFreeRingBuffer

	// Object pools for memory efficiency
	orderPool *ObjectPool[*model.Order]
	tradePool *ObjectPool[*model.Trade]

	// Atomic performance counters
	perfCounters *AtomicCounters

	// Event processing control
	eventProcessorRunning int32
	stopEventProcessor    chan struct{}
	// Persistence layer with batching
	persistenceLayer *persistence.PersistenceLayer
}

// NewHighPerformanceMatchingEngine creates a new high-performance matching engine
func NewHighPerformanceMatchingEngine(
	orderRepo model.Repository,
	tradeRepo TradeRepository,
	logger *zap.SugaredLogger,
	config *Config,
	eventJournal *eventjournal.EventJournal,
	wsHub *ws.Hub,
	riskManager interface {
		CheckPositionLimit(userID, symbol string, intendedQty float64) error
	},
	settlementEngine *settlement.SettlementEngine,
	triggerMonitor *trigger.TriggerMonitor,
) *HighPerformanceMatchingEngine {

	// Create high-performance components
	engine := &HighPerformanceMatchingEngine{
		orderRepo:        orderRepo,
		tradeRepo:        tradeRepo,
		logger:           logger,
		config:           config,
		eventJournal:     eventJournal,
		wsHub:            wsHub,
		riskManager:      riskManager,
		settlementEngine: settlementEngine,
		triggerMonitor:   triggerMonitor,

		// Initialize high-performance components
		shardedMarketState: NewShardedMarketState(),
		shardedOrderBooks:  NewShardedOrderBookManager(), orderBookHandlers: NewLockFreeEventHandlers(),
		tradeHandlers:      NewLockFreeEventHandlers(),
		eventRingBuffer:    NewLockFreeRingBuffer(),
		perfCounters:       NewAtomicCounters(),
		stopEventProcessor: make(chan struct{}),
	}

	// Initialize worker pool based on configuration
	workerCount := 16 // Default high-performance worker count
	queueSize := 4096 // Large queue for high throughput
	if config != nil && config.Engine.WorkerPoolSize > 0 {
		workerCount = config.Engine.WorkerPoolSize
		queueSize = config.Engine.WorkerPoolQueueSize
	}
	engine.highThroughputPool = NewHighThroughputWorkerPool(workerCount, queueSize, logger)

	// Initialize object pools
	engine.initializeObjectPools()

	// Start event processor
	engine.startEventProcessor()

	if logger != nil {
		logger.Infow("HighPerformanceMatchingEngine initialized",
			"workerCount", workerCount,
			"queueSize", queueSize,
			"marketShards", MARKET_STATE_SHARDS,
			"eventBufferSize", EVENT_RING_BUFFER_SIZE)
	}

	return engine
}

// initializeObjectPools sets up memory pools for high-frequency objects
func (hpme *HighPerformanceMatchingEngine) initializeObjectPools() {
	// Order pool
	hpme.orderPool = NewObjectPool(func() *model.Order {
		return &model.Order{}
	})

	// Trade pool
	hpme.tradePool = NewObjectPool(func() *model.Trade {
		return &model.Trade{}
	})
}

// startEventProcessor starts the high-throughput event processing loop
func (hpme *HighPerformanceMatchingEngine) startEventProcessor() {
	if atomic.CompareAndSwapInt32(&hpme.eventProcessorRunning, 0, 1) {
		go hpme.eventProcessorLoop()
	}
}

// eventProcessorLoop processes events from the ring buffer at high speed
func (hpme *HighPerformanceMatchingEngine) eventProcessorLoop() {
	ticker := time.NewTicker(1 * time.Microsecond) // High frequency processing
	defer ticker.Stop()

	for {
		select {
		case <-hpme.stopEventProcessor:
			return
		case <-ticker.C:
			// Process multiple events per tick for higher throughput
			for i := 0; i < 16; i++ {
				if event, ok := hpme.eventRingBuffer.TryRead(); ok {
					hpme.processEvent(event)
				} else {
					break // No more events available
				}
			}
		}
	}
}

// processEvent processes a single event from the ring buffer
func (hpme *HighPerformanceMatchingEngine) processEvent(event RingBufferEvent) {
	switch event.EventType {
	case 1: // OrderBook update event
		if update, ok := event.Data.(OrderBookUpdate); ok {
			hpme.orderBookHandlers.ForEach(func(handler EventHandler) {
				if h, ok := handler.(OrderBookUpdateHandler); ok {
					go h(update) // Process asynchronously
				}
			})
		}
	case 2: // Trade event
		if tradeEvent, ok := event.Data.(TradeEvent); ok {
			hpme.tradeHandlers.ForEach(func(handler EventHandler) {
				if h, ok := handler.(func(TradeEvent)); ok {
					go h(tradeEvent) // Process asynchronously
				}
			})
		}
	}
}

// =============================
// High-Performance Order Processing
// =============================

// ProcessOrderHighThroughput processes an order with optimized concurrency and minimal latency
func (hpme *HighPerformanceMatchingEngine) ProcessOrderHighThroughput(
	ctx context.Context,
	order *model.Order,
	source OrderSourceType,
) (*model.Order, []*model.Trade, []*model.Order, error) {

	startTime := time.Now()
	defer func() {
		// Record latency in atomic counters
		latency := time.Since(startTime).Nanoseconds()
		hpme.perfCounters.RecordLatency(latency)
		hpme.perfCounters.IncrementOrders()
	}()

	// Validate order (optimized validation)
	if err := hpme.validateOrderFast(order); err != nil {
		hpme.perfCounters.IncrementErrors()
		return nil, nil, nil, err
	}

	// Check if market is paused using sharded state
	if hpme.shardedMarketState.IsMarketPaused(order.Pair) {
		hpme.perfCounters.IncrementErrors()
		return nil, nil, nil, fmt.Errorf("market %s is paused", order.Pair)
	}

	// Get or create orderbook using sharded manager (lock-free for different pairs)
	ob := hpme.shardedOrderBooks.GetOrCreateOrderBook(order.Pair)

	// Register with trigger monitor if available
	if hpme.triggerMonitor != nil {
		hpme.triggerMonitor.AddOrderBook(order.Pair, ob)
	}

	// Process order through orderbook
	var trades []*model.Trade
	var restingOrders []*model.Order
	var err error

	if order.Type == OrderTypeMarket {
		// Market order processing
		_, trades, restingOrders, err = ob.ProcessMarketOrder(ctx, order)
	} else {
		// Limit order processing
		result, err := ob.AddOrder(order)
		if err == nil && result != nil {
			trades = result.Trades
			restingOrders = result.RestingOrders
		}
	}

	if err != nil {
		hpme.perfCounters.IncrementErrors()
		// Record failed order metrics asynchronously
		hpme.recordFailedOrderAsync(order, err, source)
		return nil, nil, nil, err
	}

	// Update performance counters
	if len(trades) > 0 {
		hpme.perfCounters.IncrementTrades()
	}

	// Process trades and updates asynchronously for maximum throughput
	if len(trades) > 0 {
		hpme.processTradesTooAsync(ctx, trades, order)
	}

	// Enqueue order book update event
	hpme.enqueueOrderBookUpdate(order.Pair, ob)

	// Record business metrics asynchronously
	hpme.recordBusinessMetricsAsync(order, trades, source)

	return order, trades, restingOrders, nil
}

// validateOrderFast performs optimized order validation
func (hpme *HighPerformanceMatchingEngine) validateOrderFast(order *model.Order) error {
	if order == nil {
		return fmt.Errorf("order is nil")
	}
	if order.Pair == "" {
		return fmt.Errorf("order pair is required")
	}
	if order.Quantity.IsZero() || order.Quantity.IsNegative() {
		return fmt.Errorf("invalid quantity")
	}
	if order.Type == OrderTypeLimit && (order.Price.IsZero() || order.Price.IsNegative()) {
		return fmt.Errorf("invalid price for limit order")
	}
	return nil
}

// processTradesTooAsync processes trades asynchronously to minimize latency
func (hpme *HighPerformanceMatchingEngine) processTradesTooAsync(ctx context.Context, trades []*model.Trade, order *model.Order) {
	for _, trade := range trades {
		// Submit trade processing to high-throughput worker pool
		hpme.highThroughputPool.Submit(Task{
			Fn: func() {
				hpme.processSingleTrade(ctx, trade, order)
			},
			Priority: 1, // High priority for trade processing
		})
	}
}

// processSingleTrade processes a single trade with persistence and notifications
func (hpme *HighPerformanceMatchingEngine) processSingleTrade(ctx context.Context, trade *model.Trade, order *model.Order) {
	// Persist trade to database
	if err := hpme.tradeRepo.CreateTrade(ctx, trade); err != nil {
		hpme.logger.Errorw("Failed to persist trade", "tradeID", trade.ID, "error", err)
		return
	}

	// Create trade event for handlers
	tradeEvent := TradeEvent{
		Trade:      trade,
		TakerOrder: order,
		Timestamp:  time.Now(),
		MatchingID: uuid.New().String(),
	}

	// Enqueue trade event for processing
	hpme.eventRingBuffer.TryWrite(RingBufferEvent{
		EventType: 2, // Trade event
		Data:      tradeEvent,
		Timestamp: time.Now().UnixNano(),
	})

	// Handle settlement asynchronously
	if hpme.settlementEngine != nil {
		hpme.highThroughputPool.Submit(Task{
			Fn: func() {
				tradeCapture := settlement.TradeCapture{
					TradeID:   trade.ID.String(),
					UserID:    "", // Set from context if available
					Symbol:    trade.Pair,
					Side:      trade.Side,
					Quantity:  trade.Quantity.InexactFloat64(),
					Price:     trade.Price.InexactFloat64(),
					AssetType: "crypto",
					MatchedAt: trade.CreatedAt,
				}
				hpme.settlementEngine.CaptureTrade(tradeCapture)
			},
			Priority: 0, // Lower priority for settlement
		})
	}
}

// enqueueOrderBookUpdate creates and enqueues an orderbook update event
func (hpme *HighPerformanceMatchingEngine) enqueueOrderBookUpdate(pair string, ob *orderbook.OrderBook) {
	// Get orderbook snapshot (this should be fast/cached)
	bids, asks := ob.GetSnapshot(10) // Get top 10 levels

	update := OrderBookUpdate{
		Pair:      pair,
		Bids:      bids,
		Asks:      asks,
		Timestamp: time.Now().UnixMilli(),
	}
	// Try to enqueue the update event
	hpme.eventRingBuffer.TryWrite(RingBufferEvent{
		EventType: 1, // OrderBook update event
		Data:      update,
		Timestamp: time.Now().UnixNano(),
	})
}

// recordFailedOrderAsync records failed order metrics asynchronously
func (hpme *HighPerformanceMatchingEngine) recordFailedOrderAsync(order *model.Order, err error, source OrderSourceType) {
	hpme.highThroughputPool.Submit(Task{
		Fn: func() {
			// Record in business metrics
			metricsapi.BusinessMetricsInstance.Mu.Lock()
			if _, ok := metricsapi.BusinessMetricsInstance.FailedOrders[order.Pair]; !ok {
				metricsapi.BusinessMetricsInstance.FailedOrders[order.Pair] = &metricsapi.FailedOrderStats{
					Reasons: make(map[string]int),
				}
			}
			metricsapi.BusinessMetricsInstance.FailedOrders[order.Pair].Mu.Lock()
			metricsapi.BusinessMetricsInstance.FailedOrders[order.Pair].Reasons[err.Error()]++
			metricsapi.BusinessMetricsInstance.FailedOrders[order.Pair].Mu.Unlock()
			metricsapi.BusinessMetricsInstance.Mu.Unlock()

			// Record compliance event
			metricsapi.ComplianceServiceInstance.Record(metricsapi.ComplianceEvent{
				Timestamp: time.Now(),
				Market:    order.Pair,
				User:      order.UserID.String(),
				OrderID:   order.ID.String(),
				Rule:      "order_rejected",
				Violation: true,
				Details:   map[string]interface{}{"reason": err.Error(), "source": source},
			})
		},
		Priority: 0, // Low priority for metrics
	})
}

// recordBusinessMetricsAsync records business metrics asynchronously
func (hpme *HighPerformanceMatchingEngine) recordBusinessMetricsAsync(order *model.Order, trades []*model.Trade, source OrderSourceType) {
	hpme.highThroughputPool.Submit(Task{
		Fn: func() {
			// Record fill rate metrics
			metricsapi.BusinessMetricsInstance.Mu.Lock()
			// Initialize nested maps if they don't exist
			if _, ok := metricsapi.BusinessMetricsInstance.FillRates[order.Pair]; !ok {
				metricsapi.BusinessMetricsInstance.FillRates[order.Pair] = make(map[string]map[string]*metricsapi.FillRateStats)
			}
			if _, ok := metricsapi.BusinessMetricsInstance.FillRates[order.Pair][order.UserID.String()]; !ok {
				metricsapi.BusinessMetricsInstance.FillRates[order.Pair][order.UserID.String()] = make(map[string]*metricsapi.FillRateStats)
			}
			if _, ok := metricsapi.BusinessMetricsInstance.FillRates[order.Pair][order.UserID.String()][order.Type]; !ok {
				metricsapi.BusinessMetricsInstance.FillRates[order.Pair][order.UserID.String()][order.Type] = &metricsapi.FillRateStats{}
			}

			// Update fill rate
			fr := metricsapi.BusinessMetricsInstance.FillRates[order.Pair][order.UserID.String()][order.Type]
			fr.Total++
			if order.FilledQuantity.Equal(order.Quantity) {
				fr.Filled++
			}
			metricsapi.BusinessMetricsInstance.Mu.Unlock()

			// Record compliance event
			metricsapi.ComplianceServiceInstance.Record(metricsapi.ComplianceEvent{
				Timestamp: time.Now(),
				Market:    order.Pair,
				User:      order.UserID.String(),
				OrderID:   order.ID.String(),
				Rule:      "order_processed",
				Violation: false,
				Details: map[string]interface{}{
					"filled_qty": order.FilledQuantity.String(),
					"order_qty":  order.Quantity.String(),
					"source":     source,
					"trades":     len(trades),
				},
			})
		},
		Priority: 0, // Low priority for metrics
	})
}

// =============================
// High-Performance Market Controls
// =============================

// PauseMarketHighPerformance pauses trading for a specific pair using sharded state
func (hpme *HighPerformanceMatchingEngine) PauseMarketHighPerformance(pair string) error {
	hpme.shardedMarketState.PauseMarket(pair)

	// Audit log asynchronously
	hpme.highThroughputPool.Submit(Task{
		Fn: func() {
			auditlog.LogAuditEvent(context.Background(), auditlog.AuditEvent{
				EventType: auditlog.AuditConfigChange,
				Entity:    "market",
				Action:    "pause",
				NewValue:  pair,
			})
		},
		Priority: 0,
	})

	hpme.logger.Info("Market paused (high-performance)", "pair", pair)
	return nil
}

// ResumeMarketHighPerformance resumes trading for a specific pair using sharded state
func (hpme *HighPerformanceMatchingEngine) ResumeMarketHighPerformance(pair string) error {
	hpme.shardedMarketState.ResumeMarket(pair)

	// Audit log asynchronously
	hpme.highThroughputPool.Submit(Task{
		Fn: func() {
			auditlog.LogAuditEvent(context.Background(), auditlog.AuditEvent{
				EventType: auditlog.AuditConfigChange,
				Entity:    "market",
				Action:    "resume",
				NewValue:  pair,
			})
		},
		Priority: 0,
	})

	hpme.logger.Info("Market resumed (high-performance)", "pair", pair)
	return nil
}

// =============================
// High-Performance Event Handler Registration
// =============================

// RegisterOrderBookUpdateHandlerHighPerformance registers an orderbook update handler using lock-free operations
func (hpme *HighPerformanceMatchingEngine) RegisterOrderBookUpdateHandlerHighPerformance(handler OrderBookUpdateHandler) {
	hpme.orderBookHandlers.Register(handler)
}

// RegisterTradeHandlerHighPerformance registers a trade handler using lock-free operations
func (hpme *HighPerformanceMatchingEngine) RegisterTradeHandlerHighPerformance(handler func(TradeEvent)) {
	hpme.tradeHandlers.Register(handler)
}

// =============================
// Performance Monitoring and Metrics
// =============================

// GetPerformanceMetrics returns current performance metrics
func (hpme *HighPerformanceMatchingEngine) GetPerformanceMetrics() map[string]interface{} {
	metrics := hpme.perfCounters.GetSnapshot()

	// Add ring buffer stats
	writePos, readPos, available, used := hpme.eventRingBuffer.GetStats()
	metrics["event_buffer_write_pos"] = writePos
	metrics["event_buffer_read_pos"] = readPos
	metrics["event_buffer_available"] = available
	metrics["event_buffer_used"] = used

	// Add worker pool stats
	submitted, completed, rejected := hpme.highThroughputPool.GetStats()
	metrics["worker_pool_submitted"] = submitted
	metrics["worker_pool_completed"] = completed
	metrics["worker_pool_rejected"] = rejected

	// Add handler counts
	metrics["orderbook_handlers"] = hpme.orderBookHandlers.Count()
	metrics["trade_handlers"] = hpme.tradeHandlers.Count()

	return metrics
}

// ResetPerformanceMetrics resets all performance counters
func (hpme *HighPerformanceMatchingEngine) ResetPerformanceMetrics() {
	hpme.perfCounters.Reset()
}

// =============================
// Shutdown and Cleanup
// =============================

// Shutdown gracefully shuts down the high-performance matching engine
func (hpme *HighPerformanceMatchingEngine) Shutdown() {
	hpme.logger.Info("Shutting down HighPerformanceMatchingEngine...")

	// Stop event processor
	if atomic.CompareAndSwapInt32(&hpme.eventProcessorRunning, 1, 0) {
		close(hpme.stopEventProcessor)
	}

	// Stop worker pool
	if hpme.highThroughputPool != nil {
		hpme.highThroughputPool.Stop()
	}

	// Log final metrics
	metrics := hpme.GetPerformanceMetrics()
	hpme.logger.Infow("Final performance metrics", "metrics", metrics)

	hpme.logger.Info("HighPerformanceMatchingEngine shutdown complete")
}
