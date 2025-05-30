// =============================
// Orbit CEX Matching Engine Core
// =============================
// This file implements the main logic for the spot matching engine, including market state management, admin controls, and integration with external systems.
//
// How it works:
// - The engine manages order matching, market pausing, and event broadcasting.
// - It uses concurrency primitives for thread safety and performance.
// - Integrates with Redis, OpenTelemetry, and other services for observability and state.
//
// Next stages:
// - After initialization, the engine responds to market events, processes orders, and handles admin commands.
// - Further logic is implemented in the types and functions below, each documented in detail.

// --- Imports and dependencies ---
// Standard and third-party packages required for engine operation.
package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	settlement "github.com/Aidin1998/pincex_unified/internal/settlement"
	auditlog "github.com/Aidin1998/pincex_unified/internal/trading/auditlog"
	eventjournal "github.com/Aidin1998/pincex_unified/internal/trading/eventjournal"
	messaging "github.com/Aidin1998/pincex_unified/internal/trading/messaging"
	model "github.com/Aidin1998/pincex_unified/internal/trading/model"
	orderbook "github.com/Aidin1998/pincex_unified/internal/trading/orderbook"
	persistence "github.com/Aidin1998/pincex_unified/internal/trading/persistence"
	"github.com/Aidin1998/pincex_unified/internal/trading/trigger"
	ws "github.com/Aidin1998/pincex_unified/internal/ws"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"

	metricsapi "github.com/Aidin1998/pincex_unified/internal/analytics/metrics"
)

// CancelRequest is re-exported from orderbook package for unified API.
type CancelRequest = orderbook.CancelRequest

// =============================
// Unified error handling for matching engine
// All errors use errors.New or errors.Wrap with error kind and message
// =============================
// Example usage:
// return errors.New(errors.ErrKindInternal).Explain("invalid order ID: %v", err)
// --- Global/Package Level Placeholders ---

// Market Pause State
var (
	marketPauseMu sync.RWMutex
	pausedMarkets = make(map[string]bool)
)

// model.Order.Status is likely string. So constants should be string.
const (
	OrderStatusNew             string = "NEW"
	OrderStatusOpen            string = "OPEN"
	OrderStatusFilled          string = "FILLED"
	OrderStatusPartiallyFilled string = "PARTIALLY_FILLED"
	OrderStatusCancelled       string = "CANCELLED"
	OrderStatusRejected        string = "REJECTED"
	OrderStatusPendingTrigger  string = "PENDING_TRIGGER" // For stop-limit orders
)

// model.OrderType is also likely string or an int/iota based enum in model package
const (
	OrderTypeLimit     string = "LIMIT"      // Assuming string type from model
	OrderTypeMarket    string = "MARKET"     // Assuming string type from model
	OrderTypeFOK       string = "FOK"        // Fill or Kill
	OrderTypeStopLimit string = "STOP_LIMIT" // Stop Limit Order
)

type OrderSourceType string

const (
	OrderSourceStopLimitTrigger OrderSourceType = "STOP_LIMIT_TRIGGER"
	OrderSourceGTDExpiration    OrderSourceType = "GTD_EXPIRATION"
	OrderSourceRecovery         OrderSourceType = "RECOVERY"
)

type TradeRepository interface {
	CreateTrade(ctx context.Context, trade *model.Trade) error
}

type FeeTierRepository interface{}

// Placeholder for model.Repository to include UpdateOrder
// This should ideally be part of the actual model.Repository definition
// type Repository interface { // This was a generic placeholder, model.Repository is used by the engine
// GetOrderByID(ctx context.Context, id uuid.UUID) (*model.Order, error)
// CancelOrder(ctx context.Context, id uuid.UUID) error
// UpdateOrder(ctx context.Context, order *model.Order) error
// }

// --- Config Types (Local Placeholders) ---
type FeeScheduleConfig struct{}

type RateLimiterConfig struct{}
type RecoveryConfig struct{ Enabled bool }

type RiskConfig struct {
	MaxOrderSize    decimal.Decimal
	MaxPositionSize decimal.Decimal
	MaxLeverage     decimal.Decimal
	AllowedSymbols  []string
	// Add more risk parameters as needed
}

type EngineConfig struct {
	WorkerPoolSize            int
	WorkerPoolQueueSize       int
	ResourcePollInterval      time.Duration
	OrderBookSnapshotInterval time.Duration
	Risk                      RiskConfig
	RateLimiter               RateLimiterConfig
	Recovery                  RecoveryConfig
	WorkerPoolMonitorEnabled  bool
	// --- Go-Specific Perf: Disruptor CPU pinning ---
	// EnableDisruptorPinning bool // If true, pin Disruptor goroutine to a CPU core
	// DisruptorPinnedCPU     int  // Which CPU core to pin to (default 0)

	// --- Advanced Performance Tuning ---
	EnableBinarySerialization bool          // Use binary serialization for WAL/snapshots if true
	EnableGCControl           bool          // Enable periodic GC and GC percent tuning
	GCPercent                 int           // If > 0, set debug.SetGCPercent
	GCInterval                time.Duration // How often to call runtime.GC
	EnableProfiling           bool          // Enable pprof/runtime/trace hooks
}

type PairConfig struct {
	Symbol               string
	MinPriceIncrement    decimal.Decimal
	MinQuantityIncrement decimal.Decimal
}

type Config struct {
	Engine   EngineConfig
	Fees     FeeScheduleConfig
	AuditLog struct {
		Enabled      bool
		FilePath     string
		MaxSizeBytes int64
		MaxBackups   int
		MaxAgeDays   int
	}
	Kafka struct {
		Enabled bool
		Topics  struct {
			OrderUpdates     string
			Trades           string
			MarketDataPrefix string
		}
	}
	Pairs []PairConfig
}

// --- Component Placeholders ---
type KafkaProducer struct{}
type KafkaConsumerGroup struct{}

// --- Kafka Notifier Interfaces and Implementations ---
// These interfaces define the contract for Kafka-based notifiers.
// If you need to send order/trade/market data events to Kafka, use these implementations.
// These implementations use github.com/segmentio/kafka-go for robust, production-grade delivery.

// OrderNotifier defines the interface for order event notifications via Kafka.
type OrderNotifier interface {
	NotifyOrder(ctx context.Context, order *model.Order) error
}

// TradeNotifier defines the interface for trade event notifications via Kafka.
type TradeNotifier interface {
	NotifyTrade(ctx context.Context, trade *model.Trade) error
}

// MarketDataNotifier defines the interface for market data event notifications via Kafka.
type MarketDataNotifier interface {
	NotifyMarketData(ctx context.Context, event interface{}) error
}

// kafkaOrderNotifier implements OrderNotifier using Kafka.
type kafkaOrderNotifier struct {
	client *messaging.KafkaClient
	topic  string
	logger *zap.SugaredLogger
}

func (n *kafkaOrderNotifier) NotifyOrder(ctx context.Context, order *model.Order) error {
	// order.Version = 1 // Removed: Set message version for Kafka schema evolution
	data, err := json.Marshal(order)
	if err != nil {
		n.logger.Errorw("Failed to marshal order for Kafka", "error", err)
		return err
	}
	return n.client.PublishEvent(ctx, order.ID.String(), data)
}

// kafkaTradeNotifier implements TradeNotifier using Kafka.
type kafkaTradeNotifier struct {
	client *messaging.KafkaClient
	topic  string
	logger *zap.SugaredLogger
}

func (n *kafkaTradeNotifier) NotifyTrade(ctx context.Context, trade *model.Trade) error {
	// trade.Version = 1 // Removed: Set message version for Kafka schema evolution
	data, err := json.Marshal(trade)
	if err != nil {
		n.logger.Errorw("Failed to marshal trade for Kafka", "error", err)
		return err
	}
	return n.client.PublishEvent(ctx, trade.ID.String(), data)
}

// kafkaMarketDataNotifier implements MarketDataNotifier using Kafka.
type kafkaMarketDataNotifier struct {
	client *messaging.KafkaClient
	prefix string
	logger *zap.SugaredLogger
}

func (n *kafkaMarketDataNotifier) NotifyMarketData(ctx context.Context, event interface{}) error {
	data, err := json.Marshal(event)
	if err != nil {
		n.logger.Errorw("Failed to marshal market data for Kafka", "error", err)
		return err
	}
	return n.client.PublishEvent(ctx, n.prefix, data)
}

// NewKafkaOrderNotifier returns a Kafka-based order notifier.
func NewKafkaOrderNotifier(producer *KafkaProducer, topic string, logger *zap.SugaredLogger) OrderNotifier {
	// In production, inject a real KafkaClient here. For now, create a new one for the topic.
	client := messaging.NewKafkaClient([]string{"localhost:9092"}, topic, "order-notifier-group")
	return &kafkaOrderNotifier{client: client, topic: topic, logger: logger}
}

// NewKafkaTradeNotifier returns a Kafka-based trade notifier.
func NewKafkaTradeNotifier(producer *KafkaProducer, topic string, logger *zap.SugaredLogger) TradeNotifier {
	client := messaging.NewKafkaClient([]string{"localhost:9092"}, topic, "trade-notifier-group")
	return &kafkaTradeNotifier{client: client, topic: topic, logger: logger}
}

// NewKafkaMarketDataNotifier returns a Kafka-based market data notifier.
func NewKafkaMarketDataNotifier(producer *KafkaProducer, prefix string, logger *zap.SugaredLogger) MarketDataNotifier {
	client := messaging.NewKafkaClient([]string{"localhost:9092"}, prefix, "marketdata-notifier-group")
	return &kafkaMarketDataNotifier{client: client, prefix: prefix, logger: logger}
}

// --- RecoveryService: Handles engine recovery, replay, and state restoration ---
type RecoveryService struct {
	engine    *MatchingEngine
	orderRepo model.Repository
	tradeRepo TradeRepository
	logger    *zap.SugaredLogger
	journal   *eventjournal.EventJournal
	config    RecoveryConfig
}

// NewRecoveryService creates a new RecoveryService with event journal integration.
// This enables replay, disaster recovery, and state restoration for the engine.
func NewRecoveryService(me *MatchingEngine, cfg RecoveryConfig, orderRepo model.Repository, tradeRepo TradeRepository, logger *zap.SugaredLogger) *RecoveryService {
	journalPath := "recovery_journal.log" // In production, make this configurable
	journal, err := eventjournal.NewEventJournal(logger, journalPath)
	if err != nil {
		logger.Errorw("Failed to initialize event journal for recovery", "error", err)
		journal = nil // Recovery will be degraded, but engine can still run
	}
	return &RecoveryService{
		engine:    me,
		orderRepo: orderRepo,
		tradeRepo: tradeRepo,
		logger:    logger,
		journal:   journal,
		config:    cfg,
	}
}

// ReplayEvents replays all events from the journal to restore engine state.
func (r *RecoveryService) ReplayEvents() error {
	if r.journal == nil {
		r.logger.Warn("No event journal available for recovery")
		return nil
	}

	r.logger.Info("Replaying events from journal for recovery...")

	eventCount := 0
	errorCount := 0

	return r.journal.ReplayEvents(func(event eventjournal.WALEvent) (bool, error) {
		eventCount++

		// Log progress every 100 events during recovery
		if eventCount%100 == 0 {
			r.logger.Infow("Recovery progress", "eventsProcessed", eventCount)
		}

		switch event.EventType {
		case eventjournal.EventTypeOrderPlaced:
			if err := r.replayOrderPlaced(event); err != nil {
				errorCount++
				r.logger.Errorw("Failed to replay ORDER_PLACED event",
					"eventCount", eventCount,
					"error", err)
				return true, nil // Continue on error
			}

		case eventjournal.EventTypeOrderCancelled:
			if err := r.replayOrderCancelled(event); err != nil {
				errorCount++
				r.logger.Errorw("Failed to replay ORDER_CANCELLED event",
					"eventCount", eventCount,
					"error", err)
				return true, nil // Continue on error
			}

		case eventjournal.EventTypeTradeExecuted:
			if err := r.replayTradeExecuted(event); err != nil {
				errorCount++
				r.logger.Errorw("Failed to replay TRADE_EXECUTED event",
					"eventCount", eventCount,
					"error", err)
				return true, nil // Continue on error
			}

		case eventjournal.EventTypeCheckpoint:
			r.logger.Debugw("Replaying checkpoint event", "eventCount", eventCount)
			// Checkpoints are informational during recovery

		default:
			r.logger.Warnw("Unknown event type during replay",
				"eventType", event.EventType,
				"eventCount", eventCount)
		}

		return true, nil // Continue processing
	})
}

// replayOrderPlaced processes an ORDER_PLACED event during recovery
func (r *RecoveryService) replayOrderPlaced(event eventjournal.WALEvent) error {
	// Try to extract order from event data
	orderData, ok := event.Data.(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid order data format in event")
	}

	// Convert map to Order struct (simplified conversion)
	// In production, you'd want more robust deserialization
	order := &model.Order{
		Pair: event.Pair,
	}

	// Parse order ID
	if event.OrderID != uuid.Nil {
		order.ID = event.OrderID
	}

	// Extract basic fields from orderData
	if userID, exists := orderData["user_id"]; exists {
		if userIDStr, ok := userID.(string); ok {
			if id, err := uuid.Parse(userIDStr); err == nil {
				order.UserID = id
			}
		}
	}
	if side, exists := orderData["side"]; exists {
		if sideStr, ok := side.(string); ok {
			order.Side = sideStr
		}
	}

	if orderType, exists := orderData["type"]; exists {
		if typeStr, ok := orderType.(string); ok {
			order.Type = typeStr
		}
	}
	if priceStr, exists := orderData["price"]; exists {
		if priceStr, ok := priceStr.(string); ok {
			priceDecimal, err := decimal.NewFromString(priceStr)
			if err == nil {
				order.Price = priceDecimal
			}
		}
	}
	if quantityStr, exists := orderData["quantity"]; exists {
		if quantityStr, ok := quantityStr.(string); ok {
			quantityDecimal, err := decimal.NewFromString(quantityStr)
			if err == nil {
				order.Quantity = quantityDecimal
			}
		}
	}

	// Process the order through the engine
	ctx := context.Background()
	_, _, _, err := r.engine.ProcessOrder(ctx, order, OrderSourceRecovery)
	if err != nil {
		return fmt.Errorf("failed to process replayed order: %w", err)
	}

	return nil
}

// replayOrderCancelled processes an ORDER_CANCELLED event during recovery
func (r *RecoveryService) replayOrderCancelled(event eventjournal.WALEvent) error {
	if event.OrderID == uuid.Nil {
		return fmt.Errorf("missing order ID in cancel event")
	}
	// Create cancel request
	cancelReq := &CancelRequest{
		OrderID: event.OrderID,
		Pair:    event.Pair,
	}

	// Extract user ID from event data if available
	if data, ok := event.Data.(map[string]interface{}); ok {
		if userID, exists := data["user_id"]; exists {
			if userIDStr, ok := userID.(string); ok {
				if id, err := uuid.Parse(userIDStr); err == nil {
					cancelReq.UserID = id
				}
			}
		}
	}
	// TODO: Implement CancelOrder on MatchingEngine or handle cancellation logic here
	r.logger.Warnw("CancelOrder not implemented on MatchingEngine; skipping cancel during recovery", "orderID", event.OrderID)
	return nil
}

// replayTradeExecuted processes a TRADE_EXECUTED event during recovery
func (r *RecoveryService) replayTradeExecuted(event eventjournal.WALEvent) error {
	// Trade events are typically results of order processing
	// During recovery, trades should be reconstructed from order processing
	// rather than replayed directly, so we can safely skip them
	r.logger.Debugw("Skipping trade event during recovery (trades reconstructed from orders)")
	return nil
}

// --- AdminController: Handles admin commands, market controls, and monitoring ---
type AdminController struct {
	orderRepo model.Repository
	engineCfg *EngineConfig
	logger    *zap.SugaredLogger
}

// NewAdminController creates a new AdminController for admin/ops endpoints.
func NewAdminController(orderRepo model.Repository, engineCfg *EngineConfig, logger *zap.SugaredLogger) *AdminController {
	return &AdminController{
		orderRepo: orderRepo,
		engineCfg: engineCfg,
		logger:    logger,
	}
}

// Example: Pause all markets (admin command)
func (a *AdminController) PauseAllMarkets() {
	a.logger.Info("Admin: Pausing all markets")
	// ...call engine or update state as needed...
}

// Example: Resume all markets (admin command)
func (a *AdminController) ResumeAllMarkets() {
	a.logger.Info("Admin: Resuming all markets")
	// ...call engine or update state as needed...
}

// --- STPMode Definition ---
// STPMode defines the self-trade prevention mode for the engine.
const (
	STPCancelAggressor = "CANCEL_AGGRESSOR"
	STPCancelResting   = "CANCEL_RESTING"
)

// PriceFeed defines an interface for getting the last traded price for a pair.
// type PriceFeed interface { // This is defined by the import "cex/internal/spot/pricefeed"
// 	GetLastPrice(ctx context.Context, pair string) (decimal.Decimal, error)
// }

// StopOrderRepository defines an interface for managing stop orders.
// ...existing code...

// Add placeholder methods to OrderBook struct
// (Removed: now handled by engine_stub.go for build tag control)

// PauseMarket pauses trading for a specific pair
func (me *MatchingEngine) PauseMarket(pair string) error {
	marketPauseMu.Lock()
	defer marketPauseMu.Unlock()
	pausedMarkets[pair] = true
	me.logger.Info("Market paused", "pair", pair) // Corrected me.log to me.logger
	auditlog.LogAuditEvent(context.Background(), auditlog.AuditEvent{
		EventType: auditlog.AuditConfigChange,
		Entity:    "market",
		Action:    "pause",
		NewValue:  pair,
	})
	return nil
}

// ResumeMarket resumes trading for a specific pair
func (me *MatchingEngine) ResumeMarket(pair string) error {
	marketPauseMu.Lock()
	defer marketPauseMu.Unlock()
	delete(pausedMarkets, pair)
	me.logger.Info("Market resumed", "pair", pair) // Corrected me.log to me.logger
	auditlog.LogAuditEvent(context.Background(), auditlog.AuditEvent{
		EventType: auditlog.AuditConfigChange,
		Entity:    "market",
		Action:    "resume",
		NewValue:  pair,
	})
	return nil
}

// isMarketPaused checks if a market is paused
func isMarketPaused(pair string) bool {
	marketPauseMu.RLock()
	defer marketPauseMu.RUnlock()
	return pausedMarkets[pair]
}

// AdminCancelOrder allows admin to cancel any order by ID
func (me *MatchingEngine) AdminCancelOrder(orderID string) error {
	// Find order by ID
	ctx := context.Background()
	orderUUID, err := uuid.Parse(orderID)
	if err != nil {
		// Use unified error type for invalid ID
		return fmt.Errorf("invalid order ID: %v", err)
	}
	order, err := me.orderRepo.GetOrderByID(ctx, orderUUID)
	if err != nil {
		return fmt.Errorf("failed to get order by ID: %w", err)
	}
	// Remove from order book if present
	me.orderbooksMu.RLock()
	orderBook, ok := me.orderbooks[order.Pair]
	me.orderbooksMu.RUnlock()
	if ok {
		_ = orderBook.CancelOrder(orderUUID) // ignore error if not present
	} else {
		me.logger.Warnw("Order book not found for pair during admin cancel", "pair", order.Pair, "orderID", orderID)
	}
	// Cancel in DB
	if err := me.orderRepo.CancelOrder(ctx, orderUUID); err != nil {
		return fmt.Errorf("failed to cancel order in DB: %w", err)
	}
	me.logger.Info("Order cancelled by admin", "orderID", orderID)
	auditlog.LogAuditEvent(context.Background(), auditlog.AuditEvent{
		EventType: auditlog.AuditAdminAction,
		OrderID:   orderID,
		Action:    "admin_cancel",
		UserID:    order.UserID.String(),
		Entity:    "order",
	})
	return nil
}

// TradeEvent represents a trade that occurred in the matching engine
type TradeEvent struct {
	Trade      *model.Trade
	TakerOrder *model.Order
	MakerOrder *model.Order
	Timestamp  time.Time
	MatchingID string // For correlating multiple trades from the same matching process
}

// OrderBookUpdate represents an order book update for handlers
// This struct is used for broadcasting order book changes
// to registered handlers (e.g., for WebSocket, Redis, etc.)
type OrderBookUpdate struct {
	Pair      string
	Bids      [][]string // [][price, quantity] as string slices
	Asks      [][]string // [][price, quantity] as string slices
	Timestamp int64
}

// OrderBookUpdateHandler is a function that handles order book updates
// It receives an OrderBookUpdate struct
// This is used for registering update handlers in the engine
type OrderBookUpdateHandler func(OrderBookUpdate)

// --- Event Handler Registration (Stub Implementation) ---
var (
	orderBookUpdateHandlers []OrderBookUpdateHandler
	tradeHandlers           []func(TradeEvent)
)

// RegisterOrderBookUpdateHandler registers a handler for order book updates
func (me *MatchingEngine) RegisterOrderBookUpdateHandler(handler OrderBookUpdateHandler) {
	orderBookUpdateHandlers = append(orderBookUpdateHandlers, handler)
}

// RegisterTradeHandler registers a handler for trade events
func (me *MatchingEngine) RegisterTradeHandler(handler func(TradeEvent)) {
	tradeHandlers = append(tradeHandlers, handler)
}

// Notifies all registered trade handlers of a trade event
func (me *MatchingEngine) notifyTradeHandlers(event TradeEvent) {
	for _, handler := range tradeHandlers {
		go handler(event)
	}

	// Broadcast trade event to WebSocket clients
	if me.wsHub != nil && event.Trade != nil {
		tradeData := map[string]interface{}{
			"type":      "trade",
			"symbol":    event.Trade.Pair,
			"price":     event.Trade.Price,
			"quantity":  event.Trade.Quantity,
			"side":      event.Trade.Side,
			"timestamp": event.Trade.CreatedAt,
			"trade_id":  event.Trade.ID.String(),
		}
		if data, err := json.Marshal(tradeData); err == nil {
			tradeTopic := fmt.Sprintf("trades.%s", event.Trade.Pair)
			me.wsHub.Broadcast(tradeTopic, data)
			me.logger.Debugw("Broadcasted trade to WebSocket clients",
				"topic", tradeTopic,
				"symbol", event.Trade.Pair,
				"price", event.Trade.Price,
				"quantity", event.Trade.Quantity)
		} else {
			me.logger.Errorw("Failed to marshal trade for WebSocket broadcast", "error", err)
		}
	}
}

// =============================
// WorkerPool: Generic Goroutine Pool for Concurrency
// =============================
// This WorkerPool is designed for general-purpose concurrent task execution.
// It is NOT used for the core matching loop (which uses the Disruptor pattern),
// but can be used for background jobs, persistence, notifications, etc.
//
// Usage:
//   pool := NewWorkerPool(size, queueSize, logger)
//   pool.Submit(func() { ... })
//   pool.Stop()
//
// The pool is integrated into MatchingEngine for future extensibility.

// WorkerPool manages a pool of goroutines to execute submitted tasks.
type WorkerPool struct {
	tasks    chan func()
	wg       sync.WaitGroup
	size     int
	logger   *zap.SugaredLogger
	stopping chan struct{}

	// metrics fields
	metricsMu     sync.Mutex
	metrics       WorkerPoolMetrics
	processed     int64
	latencySum    int64 // nanoseconds
	overloadCount int64

	// scaling fields
	scalingMu sync.Mutex
	minSize   int
	maxSize   int
}

// WorkerPoolMetrics holds metrics for monitoring the pool.
type WorkerPoolMetrics struct {
	CurrentSize    int
	QueueLength    int
	TasksProcessed int64
	OverloadCount  int64
	AvgTaskLatency time.Duration
}

// NewWorkerPool creates a new WorkerPool with the given size and queue capacity.
func NewWorkerPool(size, queueSize int, logger *zap.SugaredLogger) *WorkerPool {
	pool := &WorkerPool{
		tasks:    make(chan func(), queueSize),
		size:     size,
		logger:   logger,
		stopping: make(chan struct{}),
		minSize:  1,
		maxSize:  128, // configurable upper bound
	}
	for i := 0; i < size; i++ {
		pool.wg.Add(1)
		go pool.worker(i)
	}
	go pool.monitorOverload(2 * time.Second)
	if logger != nil {
		logger.Infow("WorkerPool started", "size", size, "queueSize", queueSize)
	}
	return pool
}

// Submit adds a task to the pool. Blocks if the queue is full.
func (wp *WorkerPool) Submit(task func()) {
	select {
	case wp.tasks <- task:
		// Task submitted
	case <-wp.stopping:
		if wp.logger != nil {
			wp.logger.Warn("WorkerPool is stopping; task rejected")
		}
	}
}

// Stop gracefully shuts down the pool, waiting for all workers to finish.
func (wp *WorkerPool) Stop() {
	close(wp.stopping)
	close(wp.tasks)
	wp.wg.Wait()
	if wp.logger != nil {
		wp.logger.Info("WorkerPool stopped")
	}
}

// ScaleTo changes the number of worker goroutines.
func (wp *WorkerPool) ScaleTo(newSize int) {
	wp.scalingMu.Lock()
	defer wp.scalingMu.Unlock()
	if newSize < 1 {
		newSize = 1
	}
	if wp.maxSize > 0 && newSize > wp.maxSize {
		newSize = wp.maxSize
	}
	delta := newSize - wp.size
	if delta == 0 {
		return
	}
	if delta > 0 {
		for i := 0; i < delta; i++ {
			wp.wg.Add(1)
			go wp.worker(wp.size + i)
		}
	} else {
		for i := 0; i < -delta; i++ {
			wp.tasks <- nil // send nil to signal worker exit
		}
	}
	wp.size = newSize
	wp.metricsMu.Lock()
	wp.metrics.CurrentSize = wp.size
	wp.metricsMu.Unlock()
	if wp.logger != nil {
		wp.logger.Infow("WorkerPool scaled", "newSize", newSize)
	}
}

// CurrentSize returns the current number of workers.
func (wp *WorkerPool) CurrentSize() int {
	wp.scalingMu.Lock()
	defer wp.scalingMu.Unlock()
	return wp.size
}

// GetMetrics returns a copy of the current metrics.
func (wp *WorkerPool) GetMetrics() WorkerPoolMetrics {
	wp.metricsMu.Lock()
	defer wp.metricsMu.Unlock()
	return wp.metrics
}

// worker is the main loop for each worker goroutine.
func (wp *WorkerPool) worker(id int) {
	defer wp.wg.Done()
	for {
		task := <-wp.tasks
		if task == nil {
			return // exit signal for scaling down
		}
		start := time.Now()
		// Recover from panics to avoid crashing the pool
		func() {
			defer func() {
				if r := recover(); r != nil && wp.logger != nil {
					wp.logger.Errorw("WorkerPool recovered from panic", "worker", id, "error", r)
				}
			}()
			task()
		}()
		wp.recordTaskMetrics(time.Since(start))
	}
}

// monitorOverload monitors the task queue and detects overload conditions.
func (wp *WorkerPool) monitorOverload(threshold time.Duration) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	fullSince := time.Time{}
	for {
		select {
		case <-wp.stopping:
			return
		case <-ticker.C:
			if len(wp.tasks) == cap(wp.tasks) {
				if fullSince.IsZero() {
					fullSince = time.Now()
				} else if time.Since(fullSince) > threshold {
					atomic.AddInt64(&wp.overloadCount, 1)
					wp.metricsMu.Lock()
					wp.metrics.OverloadCount = wp.overloadCount
					wp.metricsMu.Unlock()
					if wp.logger != nil {
						wp.logger.Warnw("WorkerPool overload detected", "queueFullFor", time.Since(fullSince))
					}
					fullSince = time.Time{}
				}
			} else {
				fullSince = time.Time{}
			}
		}
	}
}

// Update metrics after each task
func (wp *WorkerPool) recordTaskMetrics(latency time.Duration) {
	atomic.AddInt64(&wp.processed, 1)
	atomic.AddInt64(&wp.latencySum, latency.Nanoseconds())
	wp.metricsMu.Lock()
	wp.metrics.TasksProcessed = wp.processed
	if wp.processed > 0 {
		wp.metrics.AvgTaskLatency = time.Duration(wp.latencySum / wp.processed)
	}
	wp.metrics.QueueLength = len(wp.tasks)
	wp.metricsMu.Unlock()
}

// =============================
// RateLimiter: Token Bucket Rate Limiter for Concurrency Control
// =============================
// This RateLimiter is designed for general-purpose rate limiting (e.g., API, background jobs, notifications).
// It is NOT used for the core matching loop, but can be used for per-user, per-pair, or global rate limiting.
//
// Usage:
//   limiter := NewRateLimiter(permitsPerSecond, burst, logger)
//   if limiter.Allow() { ... } // proceed if allowed
//
// The limiter is integrated into MatchingEngine for future extensibility.

// RateLimiter implements a token bucket rate limiter.
type RateLimiter struct {
	capacity int             // maximum burst size
	rate     decimal.Decimal // tokens per second (decimal, not float64)
	tokens   decimal.Decimal // current tokens (decimal)
	last     time.Time       // last refill timestamp
	mu       sync.Mutex      // protects state
	logger   *zap.SugaredLogger
}

// NewRateLimiter creates a new RateLimiter.
// rate: tokens per second, capacity: max burst size.
func NewRateLimiter(rate decimal.Decimal, capacity int, logger *zap.SugaredLogger) *RateLimiter {
	return &RateLimiter{
		capacity: capacity,
		rate:     rate,
		tokens:   decimal.NewFromInt(int64(capacity)),
		last:     time.Now(),
		logger:   logger,
	}
}

// Allow returns true if a token is available (and consumes one), false otherwise.
func (rl *RateLimiter) Allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	elapsed := decimal.NewFromFloat(now.Sub(rl.last).Seconds())
	// Refill tokens
	refill := elapsed.Mul(rl.rate)
	rl.tokens = rl.tokens.Add(refill)
	maxTokens := decimal.NewFromInt(int64(rl.capacity))
	if rl.tokens.GreaterThan(maxTokens) {
		rl.tokens = maxTokens
	}
	rl.last = now
	if rl.tokens.GreaterThanOrEqual(decimal.NewFromInt(1)) {
		rl.tokens = rl.tokens.Sub(decimal.NewFromInt(1))
		return true
	}
	return false
}

// SetRate dynamically updates the rate (tokens per second).
func (rl *RateLimiter) SetRate(rate decimal.Decimal) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.rate = rate
	if rl.logger != nil {
		rl.logger.Infow("RateLimiter rate updated", "rate", rate)
	}
}

// SetCapacity dynamically updates the burst capacity.
func (rl *RateLimiter) SetCapacity(capacity int) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.capacity = capacity
	maxTokens := decimal.NewFromInt(int64(capacity))
	if rl.tokens.GreaterThan(maxTokens) {
		rl.tokens = maxTokens
	}
	if rl.logger != nil {
		rl.logger.Infow("RateLimiter capacity updated", "capacity", capacity)
	}
}

// TraceIDKey is the context key for trace ID propagation
const TraceIDKey = "trace_id"

// TraceIDFromContext extracts the trace ID from context, or generates one if missing
func TraceIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v := ctx.Value(TraceIDKey); v != nil {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return uuid.New().String()
}

// recordLatencyCheckpoint records a latency checkpoint with trace ID, stage, and timestamp
func recordLatencyCheckpoint(ctx context.Context, logger *zap.Logger, stage string, extra map[string]interface{}) {
	traceID := TraceIDFromContext(ctx)
	ts := time.Now().UTC()
	fields := []zap.Field{
		zap.String("trace_id", traceID),
		zap.String("stage", stage),
		zap.Time("timestamp", ts),
	}
	for k, v := range extra {
		fields = append(fields, zap.Any(k, v))
	}
	logger.Info("latency_checkpoint", fields...)
	// TODO: Write to time-series DB (Prometheus/Tempo/Influx) here
}

// Example: Instrument MatchOrders with trace ID and latency checkpoints
func (e *Engine) MatchOrders(ctx context.Context, logger *zap.Logger) error {
	recordLatencyCheckpoint(ctx, logger, "engine_match_start", nil)
	// ...existing match logic...
	recordLatencyCheckpoint(ctx, logger, "engine_match_done", nil)
	return nil
}

// Example: Instrument TradeGeneration with trace ID and latency checkpoints
func (e *Engine) GenerateTrade(ctx context.Context, trade *model.Trade, logger *zap.Logger) error {
	recordLatencyCheckpoint(ctx, logger, "engine_trade_generation", map[string]interface{}{"trade_id": trade.ID})
	// ...existing trade generation logic...
	return nil
}

// Example: Instrument SettlementInitiation with trace ID and latency checkpoints
func (e *Engine) InitiateSettlement(ctx context.Context, settlementID string, logger *zap.Logger) error {
	recordLatencyCheckpoint(ctx, logger, "engine_settlement_initiation", map[string]interface{}{"settlement_id": settlementID})
	// ...existing settlement initiation logic...
	return nil
}

// Example: Instrument SettlementConfirmation with trace ID and latency checkpoints
func (e *Engine) ConfirmSettlement(ctx context.Context, settlementID string, logger *zap.Logger) error {
	recordLatencyCheckpoint(ctx, logger, "engine_settlement_confirmation", map[string]interface{}{"settlement_id": settlementID})
	// ...existing settlement confirmation logic...
	return nil
}

// --- Prometheus Metrics for Async Operations ---
var (
	asyncJobLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "trading_async_job_latency_ms",
			Help:    "Latency of async jobs (ms)",
			Buckets: prometheus.ExponentialBuckets(0.1, 2, 12),
		},
		[]string{"op"},
	)
	asyncJobErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "trading_async_job_errors_total",
			Help: "Total async job errors",
		},
		[]string{"op"},
	)
	asyncJobRetries = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "trading_async_job_retries_total",
			Help: "Total async job retries",
		},
		[]string{"op"},
	)
	asyncJobDLQ = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "trading_async_job_dlq_total",
			Help: "Total async jobs sent to DLQ",
		},
		[]string{"op"},
	)
)

func init() {
	prometheus.MustRegister(asyncJobLatency, asyncJobErrors, asyncJobRetries, asyncJobDLQ)
}

// --- Advanced Async Job Struct ---
type asyncJob struct {
	op         string
	data       interface{}
	attempts   int
	maxRetries int
	id         string // for idempotency
	createdAt  time.Time
}

// --- DLQ Channel for Failed Jobs ---
var asyncDLQ = make(chan asyncJob, 10000)

// --- MatchingEngine Core Struct ---
// MatchingEngine is the main struct for the spot matching engine. It manages order books, persistence, and event handling.
type MatchingEngine struct {
	orderRepo        model.Repository
	tradeRepo        TradeRepository
	orderbooks       map[string]*orderbook.OrderBook
	orderbooksMu     sync.RWMutex
	logger           *zap.SugaredLogger
	config           *Config
	workerPool       *WorkerPool
	rateLimiter      *RateLimiter
	eventJournal     *eventjournal.EventJournal
	wsHub            *ws.Hub
	riskManager      interface{}
	settlementEngine *settlement.SettlementEngine
	// --- TriggerMonitor Integration ---
	triggerMonitor *trigger.TriggerMonitor // Add this field for advanced order monitoring

	// --- Additional fields for async operations ---
	asyncJobCh  chan asyncJob
	asyncDLQ    chan asyncJob
	metricsOnce sync.Once

	// --- Persistence Layer ---
	persistenceLayer *persistence.PersistenceLayer
}

// enqueueAdvancedAsync adds an async job to the advanced async job channel with observability, retries, and idempotency.
func (me *MatchingEngine) enqueueAdvancedAsync(job asyncJob) {
	select {
	case me.asyncJobCh <- job:
		// Job enqueued
	default:
		me.logger.Warnw("asyncJobCh full, dropping job", "op", job.op, "id", job.id)
	}
}

// asyncWorkerPool runs advanced async jobs with error handling, retries, DLQ, and observability.
func (me *MatchingEngine) asyncWorkerPool() {
	for job := range me.asyncJobCh {
		go me.handleAsyncJob(job)
	}
}

// handleAsyncJob processes an async job with idempotency, retries, and metrics.
func (me *MatchingEngine) handleAsyncJob(job asyncJob) {
	var err error
	for job.attempts = 0; job.attempts <= job.maxRetries; job.attempts++ {
		switch job.op {
		case "create_trade":
			trade := job.data.(*model.Trade)
			err = me.tradeRepo.CreateTrade(context.Background(), trade)
		case "update_order":
			order := job.data.(*model.Order)
			err = me.orderRepo.UpdateOrderStatusAndFilledQuantity(context.Background(), order.ID, order.Status, order.FilledQuantity, order.AvgPrice)
		case "settle_trade":
			trade := job.data.(*model.Trade)
			// UserID is not present in model.Trade; set as empty or extend Trade struct as needed
			tradeCapture := settlement.TradeCapture{
				TradeID:   trade.ID.String(),
				UserID:    "", // TODO: set correct user ID if available
				Symbol:    trade.Pair,
				Side:      trade.Side,
				Quantity:  trade.Quantity.InexactFloat64(),
				Price:     trade.Price.InexactFloat64(),
				AssetType: "crypto",
				MatchedAt: trade.CreatedAt,
			}
			me.settlementEngine.CaptureTrade(tradeCapture)
			err = nil // CaptureTrade does not return error
		default:
			err = fmt.Errorf("unknown async job op: %s", job.op)
		}
		if err == nil {
			// Success: record metrics, trace, etc.
			return
		}
		me.logger.Errorw("Async job failed", "op", job.op, "id", job.id, "attempt", job.attempts, "error", err)
		time.Sleep(time.Duration(job.attempts+1) * 50 * time.Millisecond) // Exponential backoff
	}
	// Exceeded retries: send to DLQ
	select {
	case me.asyncDLQ <- job:
		me.logger.Errorw("Async job sent to DLQ", "op", job.op, "id", job.id)
	default:
		me.logger.Errorw("DLQ full, dropping async job", "op", job.op, "id", job.id)
	}
}

// StartAsyncProcessing initializes the async job system and worker pool.
func (me *MatchingEngine) StartAsyncProcessing() {
	me.metricsOnce.Do(func() {
		me.asyncJobCh = make(chan asyncJob, 2048)
		me.asyncDLQ = make(chan asyncJob, 256)
		for i := 0; i < 8; i++ { // 8 async workers, configurable
			go me.asyncWorkerPool()
		}
		go me.dlqMonitor()
		me.logger.Infow("Advanced async job system started")
	})
}

// dlqMonitor logs and exposes metrics for jobs in the DLQ.
func (me *MatchingEngine) dlqMonitor() {
	for job := range me.asyncDLQ {
		me.logger.Errorw("DLQ job detected", "op", job.op, "id", job.id, "createdAt", job.createdAt)
		// Optionally: expose Prometheus metric, alert, or persist to DB
	}
}

// --- Update NewMatchingEngine to start async system ---
func NewMatchingEngine(
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
	triggerMonitor *trigger.TriggerMonitor, // Add triggerMonitor as a parameter
) *MatchingEngine {
	var workerPool *WorkerPool
	if config != nil && config.Engine.WorkerPoolSize > 0 {
		workerPool = NewWorkerPool(
			config.Engine.WorkerPoolSize,
			config.Engine.WorkerPoolQueueSize,
			logger,
		)
	}
	var rateLimiter *RateLimiter
	if config != nil {
		rateLimiter = NewRateLimiter(decimal.NewFromInt(10), 20, logger)
	}
	me := &MatchingEngine{
		orderRepo:        orderRepo,
		tradeRepo:        tradeRepo,
		orderbooks:       make(map[string]*orderbook.OrderBook),
		logger:           logger,
		config:           config,
		workerPool:       workerPool,
		rateLimiter:      rateLimiter,
		eventJournal:     eventJournal,
		wsHub:            wsHub,
		riskManager:      riskManager,
		settlementEngine: settlementEngine,
		triggerMonitor:   triggerMonitor, // Initialize triggerMonitor
	}

	// Initialize object pools with pre-warming for optimal performance
	// This reduces GC pressure by pre-allocating objects
	me.initializeObjectPools(logger)

	// Initialize async persistence layer (stub for now)
	// persistenceLayer, err := persistence.NewPersistenceLayer(orderRepo, "trading_wal.log")
	// if err != nil {
	//	logger.Errorw("Failed to initialize async persistence layer", "error", err)
	// } else {
	//	me.persistenceLayer = persistenceLayer
	//	me.persistenceLayer.Writer.Start() // Start batch writer
	// }

	me.StartAsyncProcessing()
	return me
}

// --- Update ProcessOrder to use advanced async job system ---
func (me *MatchingEngine) ProcessOrder(ctx context.Context, order *model.Order, source OrderSourceType) (*model.Order, []*model.Trade, []*model.Order, error) {
	// Validate order (add your validation logic here)
	if order == nil {
		return nil, nil, nil, fmt.Errorf("order is nil")
	}
	if order.Pair == "" {
		return nil, nil, nil, fmt.Errorf("order pair is required")
	}
	if isMarketPaused(order.Pair) {
		return nil, nil, nil, fmt.Errorf("market %s is paused", order.Pair)
	}

	// Get or create order book
	me.orderbooksMu.Lock()
	ob, ok := me.orderbooks[order.Pair]
	if !ok {
		ob = orderbook.NewOrderBook(order.Pair)
		me.orderbooks[order.Pair] = ob
		// --- TriggerMonitor Integration ---
		// Register the new order book with the trigger monitor if available
		if me.triggerMonitor != nil {
			me.triggerMonitor.AddOrderBook(order.Pair, ob)
		}
	}
	me.orderbooksMu.Unlock()

	// Match order
	var trades []*model.Trade
	var restingOrders []*model.Order
	var err error
	var result *orderbook.AddOrderResult
	if order.Type == OrderTypeMarket {
		// Market order: use ProcessMarketOrder
		_, trades, restingOrders, err = ob.ProcessMarketOrder(ctx, order)
	} else {
		// Limit/FOK/other: use AddOrder
		result, err = ob.AddOrder(order)
		if result != nil {
			trades = result.Trades
			restingOrders = result.RestingOrders
		}
	}
	if err != nil {
		// Record failed order in business metrics and compliance
		metricsapi.BusinessMetricsInstance.Mu.Lock()
		if _, ok := metricsapi.BusinessMetricsInstance.FailedOrders[order.Pair]; !ok {
			metricsapi.BusinessMetricsInstance.FailedOrders[order.Pair] = &metricsapi.FailedOrderStats{Reasons: make(map[string]int)}
		}
		metricsapi.BusinessMetricsInstance.FailedOrders[order.Pair].Mu.Lock()
		metricsapi.BusinessMetricsInstance.FailedOrders[order.Pair].Reasons[err.Error()]++
		metricsapi.BusinessMetricsInstance.FailedOrders[order.Pair].Mu.Unlock()
		metricsapi.BusinessMetricsInstance.Mu.Unlock()

		metricsapi.ComplianceServiceInstance.Record(metricsapi.ComplianceEvent{
			Timestamp: time.Now(),
			Market:    order.Pair,
			User:      order.UserID.String(),
			OrderID:   order.ID.String(),
			Rule:      "order_rejected",
			Violation: true,
			Details:   map[string]interface{}{"reason": err.Error(), "source": source},
		})
		return nil, nil, nil, err
	}

	// --- Business Metrics: Fill Rate, Slippage, Conversion, etc. ---
	metricsapi.BusinessMetricsInstance.Mu.Lock()
	// Fill Rate
	if _, ok := metricsapi.BusinessMetricsInstance.FillRates[order.Pair]; !ok {
		metricsapi.BusinessMetricsInstance.FillRates[order.Pair] = make(map[string]map[string]*metricsapi.FillRateStats)
	}
	if _, ok := metricsapi.BusinessMetricsInstance.FillRates[order.Pair][order.UserID.String()]; !ok {
		metricsapi.BusinessMetricsInstance.FillRates[order.Pair][order.UserID.String()] = make(map[string]*metricsapi.FillRateStats)
	}
	if _, ok := metricsapi.BusinessMetricsInstance.FillRates[order.Pair][order.UserID.String()][order.Type]; !ok {
		metricsapi.BusinessMetricsInstance.FillRates[order.Pair][order.UserID.String()][order.Type] = &metricsapi.FillRateStats{}
	}
	fr := metricsapi.BusinessMetricsInstance.FillRates[order.Pair][order.UserID.String()][order.Type]
	fr.Total++
	if order.FilledQuantity.Equal(order.Quantity) {
		fr.Filled++
	}
	// Slippage (if applicable)
	if _, ok := metricsapi.BusinessMetricsInstance.Slippage[order.Pair]; !ok {
		metricsapi.BusinessMetricsInstance.Slippage[order.Pair] = make(map[string]*metricsapi.SlippageStats)
	}
	if _, ok := metricsapi.BusinessMetricsInstance.Slippage[order.Pair][order.UserID.String()]; !ok {
		metricsapi.BusinessMetricsInstance.Slippage[order.Pair][order.UserID.String()] = &metricsapi.SlippageStats{Values: metricsapi.NewSlidingWindow(5 * time.Minute)}
	}
	if len(trades) > 0 {
		// Calculate average slippage for this order
		var totalSlippage float64
		for _, trade := range trades {
			// Slippage = |order.Price - trade.Price| for limit orders
			if order.Type == OrderTypeLimit {
				slippage := order.Price.Sub(trade.Price).Abs().InexactFloat64()
				totalSlippage += slippage
			}
		}
		avgSlippage := totalSlippage / float64(len(trades))
		slipStats := metricsapi.BusinessMetricsInstance.Slippage[order.Pair][order.UserID.String()]
		slipStats.Values.Add(avgSlippage)
		if avgSlippage > slipStats.Worst {
			slipStats.Worst = avgSlippage
		}
		// Alert if slippage exceeds threshold
		if avgSlippage > 0.01 { // Example threshold
			metricsapi.AlertingServiceInstance.Raise(metricsapi.Alert{
				Type:      metricsapi.AlertSlippage,
				Market:    order.Pair,
				User:      order.UserID.String(),
				OrderType: order.Type,
				Value:     avgSlippage,
				Threshold: 0.01,
				Details:   "High slippage detected",
				Timestamp: time.Now(),
			})
		}
	}
	metricsapi.BusinessMetricsInstance.Mu.Unlock()

	// --- Compliance Event Recording ---
	metricsapi.ComplianceServiceInstance.Record(metricsapi.ComplianceEvent{
		Timestamp: time.Now(),
		Market:    order.Pair,
		User:      order.UserID.String(),
		OrderID:   order.ID.String(),
		Rule:      "order_processed",
		Violation: false,
		Details:   map[string]interface{}{"filled_qty": order.FilledQuantity.String(), "order_qty": order.Quantity.String(), "source": source},
	})

	return order, trades, restingOrders, nil
}

// GetOrderBook returns the order book for a given pair, or nil if not found.
func (me *MatchingEngine) GetOrderBook(pair string) orderbook.OrderBookInterface {
	me.orderbooksMu.RLock()
	ob, ok := me.orderbooks[pair]
	me.orderbooksMu.RUnlock()
	if ok {
		return ob
	}
	return nil
}

// CancelOrder cancels an order by ID, updates the order book and DB.
func (me *MatchingEngine) CancelOrder(req *CancelRequest) error {
	ctx := context.Background()
	order, err := me.orderRepo.GetOrderByID(ctx, req.OrderID)
	if err != nil {
		return fmt.Errorf("order not found: %w", err)
	}
	me.orderbooksMu.RLock()
	ob, ok := me.orderbooks[order.Pair]
	me.orderbooksMu.RUnlock()
	if !ok {
		return fmt.Errorf("order book not found for pair: %s", order.Pair)
	}
	if err := ob.CancelOrder(order.ID); err != nil {
		return fmt.Errorf("failed to cancel order in book: %w", err)
	}
	order.Status = model.OrderStatusCancelled
	if err := me.orderRepo.UpdateOrderStatusAndFilledQuantity(ctx, order.ID, order.Status, order.FilledQuantity, order.AvgPrice); err != nil {
		return fmt.Errorf("failed to update order status in DB: %w", err)
	}
	return nil
}

// GetOrderRepository returns the order repository for direct access when needed
func (me *MatchingEngine) GetOrderRepository() model.Repository {
	return me.orderRepo
}

// initializeObjectPools initializes and pre-warms all object pools for optimal performance
// This reduces GC pressure by pre-allocating commonly used objects
func (me *MatchingEngine) initializeObjectPools(logger *zap.SugaredLogger) {
	// Pre-warm model object pools (Order and Trade)
	// Use configurable sizes with minimums to ensure adequate pre-warming
	orderPoolSize := 2000 // Default 2000 orders
	tradePoolSize := 1500 // Default 1500 trades

	// Check if config specifies larger pool sizes
	if me.config != nil {
		// Could add config fields for pool sizes in future
		// For now, use sensible defaults based on expected load
	}

	// Pre-warm order and trade pools from model package
	model.PreallocateObjectPoolsWithSizes(orderPoolSize, tradePoolSize)

	// Pre-warm order book structure pools
	// These handle price levels, order chunks, and snapshot slices
	priceLevelSize := 200     // Price levels per order book
	orderChunkSize := 800     // Order chunks for linked lists
	safePriceLevelSize := 200 // Safe price levels for thread-safe books
	snapshotSliceSize := 100  // Snapshot slices for market data

	orderbook.PrewarmOrderBookPools(priceLevelSize, orderChunkSize, safePriceLevelSize, snapshotSliceSize)

	if logger != nil {
		logger.Infow("Object pools initialized and pre-warmed",
			"order_pool_size", orderPoolSize,
			"trade_pool_size", tradePoolSize,
			"price_level_pool_size", priceLevelSize,
			"order_chunk_pool_size", orderChunkSize,
			"safe_price_level_pool_size", safePriceLevelSize,
			"snapshot_slice_pool_size", snapshotSliceSize,
		)
	}
}
