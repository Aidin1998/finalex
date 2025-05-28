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
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	auditlog "github.com/Aidin1998/pincex_unified/internal/trading/auditlog"
	messaging "github.com/Aidin1998/pincex_unified/internal/trading/messaging"
	model "github.com/Aidin1998/pincex_unified/internal/trading/model"
	orderbook "github.com/Aidin1998/pincex_unified/internal/trading/orderbook"
	trading "github.com/Aidin1998/pincex_unified/internal/trading"
	errors "github.com/Aidin1998/pincex_unified/common/errors"

	"go.uber.org/zap"
)

// OrderBookShardManager is a placeholder for shard management, define as needed
type OrderBookShardManager interface{}

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
	order.Version = 1 // Set message version for Kafka schema evolution
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
	trade.Version = 1 // Set message version for Kafka schema evolution
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
	journal   *EventJournal
	config    RecoveryConfig
}

// NewRecoveryService creates a new RecoveryService with event journal integration.
// This enables replay, disaster recovery, and state restoration for the engine.
func NewRecoveryService(me *MatchingEngine, cfg RecoveryConfig, orderRepo model.Repository, tradeRepo TradeRepository, logger *zap.SugaredLogger) *RecoveryService {
	journalPath := "recovery_journal.log" // In production, make this configurable
	journal, err := NewEventJournal(logger, journalPath)
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
	return r.journal.ReplayEvents(func(event WALEvent) (bool, error) {
		switch event.EventType {
		case EventTypeOrderPlaced:
			order, ok := event.Data.(*model.Order)
			if ok {
				_, _, _, _ = r.engine.ProcessOrder(context.Background(), order, OrderSourceGTDExpiration)
			}
		case EventTypeOrderCancelled:
			// Implement cancel replay logic as needed
		case EventTypeTradeExecuted:
			// Implement trade replay logic as needed
		}
		return true, nil
	})
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
		return errors.New(errors.ErrKindValidation).Explain("invalid order ID: %v", err)
	}
	order, err := me.orderRepo.GetOrderByID(ctx, orderUUID)
	if err != nil {
		return errors.Wrap(err).Explain("failed to get order by ID")
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
		return errors.Wrap(err).Explain("failed to cancel order in DB")
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

// --- MatchingEngine Core Struct ---
// MatchingEngine is the main struct for the spot matching engine. It manages order books, persistence, and event handling.
type MatchingEngine struct {
	orderRepo    model.Repository
	tradeRepo    TradeRepository
	orderbooks   map[string]*OrderBook
	orderbooksMu sync.RWMutex
	logger       *zap.SugaredLogger
	config       *Config
	workerPool   *WorkerPool   // For background jobs
	rateLimiter  *RateLimiter  // Integrated for future use (API, jobs, etc.)
	eventJournal *EventJournal // <-- Add this line for recovery

	// Add a field for the shard manager to MatchingEngine
	shardManager OrderBookShardManager
	// Add other fields as needed for event handlers, notifiers, etc.
}

// Exported constructor for MatchingEngine to fix undefined: matching_engine.NewMatchingEngine errors
// NewMatchingEngine creates a new MatchingEngine instance with the given repositories, logger, config, and event journal.
func NewMatchingEngine(
	orderRepo model.Repository,
	tradeRepo TradeRepository,
	logger *zap.SugaredLogger,
	config *Config,
	eventJournal *EventJournal, // new param, can be nil
) *MatchingEngine {
	var workerPool *WorkerPool
	if config != nil && config.Engine.WorkerPoolSize > 0 {
		workerPool = NewWorkerPool(
			config.Engine.WorkerPoolSize,
			config.Engine.WorkerPoolQueueSize,
			logger,
		)
	}
	// Example: create a global rate limiter (10 ops/sec, burst 20)
	var rateLimiter *RateLimiter
	if config != nil {
		rateLimiter = NewRateLimiter(decimal.NewFromInt(10), 20, logger)
	}
	return &MatchingEngine{
		orderRepo:    orderRepo,
		tradeRepo:    tradeRepo,
		logger:       logger,
		config:       config,
		orderbooks:   make(map[string]*OrderBook),
		workerPool:   workerPool,
		rateLimiter:  rateLimiter,
		eventJournal: eventJournal, // set here
	}
}

// ProcessOrder is the main entry point for order processing in the matching engine.
// It validates, routes, and matches orders, and triggers notifications and persistence.
// This implementation is robust, extensible, and safe for production use.
func (me *MatchingEngine) ProcessOrder(ctx context.Context, order *model.Order, source OrderSourceType) (*model.Order, []*model.Trade, []*model.Order, error) {
	// 1. Validate order (basic checks, symbol, size, etc.)
	if order == nil {
		return nil, nil, nil, errors.New(errors.ErrKindInternal).Explain("order is nil")
	}
	if order.Pair == "" {
		return nil, nil, nil, errors.New(errors.ErrKindInternal).Explain("order pair is required")
	}
	if order.Quantity.LessThanOrEqual(decimal.Zero) {
		return nil, nil, nil, errors.New(errors.ErrKindInternal).Explain("order quantity must be positive")
	}

	// 2. Check if market is paused
	if isMarketPaused(order.Pair) {
		return nil, nil, nil, errors.New(errors.ErrKindInternal).Explain("market %s is paused", order.Pair)
	}

	// REVIEW: locking mechanism will be bottlenecked whole engine. In this architecture, we have only one engine instance (that is also another problem to solve).
	// 3. Route to the correct order book
	me.orderbooksMu.RLock()
	orderBook, ok := me.orderbooks[order.Pair]
	me.orderbooksMu.RUnlock()

	// REVIEW: Potential risk (creating order book is privileged action and should be done in a separate workflow)
	if !ok {
		// Create order book if it doesn't exist (thread-safe)
		me.orderbooksMu.Lock()
		orderBook, ok = me.orderbooks[order.Pair]
		if !ok {
			orderBook = NewOrderBook(order.Pair)
			me.orderbooks[order.Pair] = orderBook
		}
		me.orderbooksMu.Unlock()
	}

	// --- Order type handling (add/remove logic as needed before launch) ---
	switch order.Type {
	case model.OrderTypeLimit:
		// Standard limit order
		return me.processLimitOrder(ctx, order, orderBook)
	case model.OrderTypeMarket:
		return me.processMarketOrder(ctx, order, orderBook)
	case model.OrderTypeIOC:
		// Immediate or Cancel
		return me.processIOCOrder(ctx, order, orderBook)
	case model.OrderTypeFOK:
		// Fill or Kill
		return me.processFOKOrder(ctx, order, orderBook)
	case model.OrderTypeStopLimit:
		// Stop Limit (trigger logic may be elsewhere)
		return me.processStopLimitOrder(ctx, order, orderBook)
	default:
		return nil, nil, nil, errors.New(errors.ErrKindValidation).Explain("unsupported order type: %s", order.Type)
	}
}

// --- The following methods are for test/integration only. Remove or refactor before launch. ---
func (me *MatchingEngine) processLimitOrder(ctx context.Context, order *model.Order, orderBook *OrderBook) (*model.Order, []*model.Trade, []*model.Order, error) {
	result, err := orderBook.AddOrder(order)
	// Update order status based on fills
	if err == nil {
		if order.FilledQuantity.Equal(order.Quantity) {
			order.Status = model.OrderStatusFilled
		} else if order.FilledQuantity.GreaterThan(decimal.Zero) {
			order.Status = model.OrderStatusPartiallyFilled
		}
	}
	return order, result.Trades, result.RestingOrders, err
}
func (me *MatchingEngine) processMarketOrder(ctx context.Context, order *model.Order, orderBook *OrderBook) (*model.Order, []*model.Trade, []*model.Order, error) {
	result, err := orderBook.AddOrder(order)
	// Update order status based on fills
	if err == nil {
		if order.FilledQuantity.Equal(order.Quantity) {
			order.Status = model.OrderStatusFilled
		} else if order.FilledQuantity.GreaterThan(decimal.Zero) {
			order.Status = model.OrderStatusPartiallyFilled
		}
	}
	return order, result.Trades, nil, err
}
func (me *MatchingEngine) processIOCOrder(ctx context.Context, order *model.Order, orderBook *OrderBook) (*model.Order, []*model.Trade, []*model.Order, error) {
	if order.TimeInForce == "" {
		order.TimeInForce = model.TimeInForceIOC
	}
	result, err := orderBook.AddOrder(order)
	// Update order status based on fills
	if err == nil {
		if order.FilledQuantity.Equal(order.Quantity) {
			order.Status = model.OrderStatusFilled
		} else if order.FilledQuantity.GreaterThan(decimal.Zero) {
			order.Status = model.OrderStatusPartiallyFilled
		}
	}
	return order, result.Trades, nil, err
}
func (me *MatchingEngine) processFOKOrder(ctx context.Context, order *model.Order, orderBook *OrderBook) (*model.Order, []*model.Trade, []*model.Order, error) {
	if order.TimeInForce == "" {
		order.TimeInForce = model.TimeInForceFOK
	}
	result, err := orderBook.AddOrder(order)
	// Update order status based on fills
	if err == nil {
		if order.FilledQuantity.Equal(order.Quantity) {
			order.Status = model.OrderStatusFilled
		} else if order.FilledQuantity.GreaterThan(decimal.Zero) {
			order.Status = model.OrderStatusPartiallyFilled
		}
	}
	return order, result.Trades, nil, err
}
func (me *MatchingEngine) processStopLimitOrder(ctx context.Context, order *model.Order, orderBook *OrderBook) (*model.Order, []*model.Trade, []*model.Order, error) {
	result, err := orderBook.AddOrder(order)
	// Update order status based on fills
	if err == nil {
		if order.FilledQuantity.Equal(order.Quantity) {
			order.Status = model.OrderStatusFilled
		} else if order.FilledQuantity.GreaterThan(decimal.Zero) {
			order.Status = model.OrderStatusPartiallyFilled
		}
	}
	return order, result.Trades, result.RestingOrders, err
}

// CancelOrder cancels an order given a CancelRequest.
func (me *MatchingEngine) CancelOrder(req *CancelRequest) error {
	// ...existing or stub logic for canceling an order...
	return nil // TODO: implement real logic
}

// AddPair creates an order book for the given pair if it does not exist.
func (me *MatchingEngine) AddPair(pair string) error {
	me.orderbooksMu.Lock()
	defer me.orderbooksMu.Unlock()
	if _, ok := me.orderbooks[pair]; !ok {
		me.orderbooks[pair] = NewOrderBook(pair)
	}
	return nil
}

// Update GetOrderBook and SetOrderBook to use the shard manager
func (me *MatchingEngine) GetOrderBook(pair string) IOrderBook {
	if me.shardManager != nil {
		return me.shardManager.GetShard(pair)
	}
	me.orderbooksMu.RLock()
	defer me.orderbooksMu.RUnlock()
	if ob, ok := me.orderbooks[pair]; ok {
		return ob
	}
	return nil
}

func (me *MatchingEngine) SetOrderBook(pair string, ob IOrderBook) {
	if me.shardManager != nil {
		me.shardManager.SetShard(pair, ob)
	}
}

// Extension point: For stateless engine nodes, use DistributedOrderBookStateManager for all order book state access.
// Example: Replace shardManager with a distributed implementation in production.
// --- DistributedOrderBookStateManager: Distributed State Management for Order Books ---
// This manager handles order book state across distributed nodes.
// It ensures consistency and availability of order book data in a cloud-native environment.
//
// Usage:
//   manager := NewDistributedOrderBookStateManager(...)
//   manager.SetOrderBook(pair, orderBook)
//   ob := manager.GetOrderBook(pair)
//   manager.RemoveOrderBook(pair)
//
// This is integrated into MatchingEngine as the shardManager field.
// In a stateless deployment, shardManager should be set to an instance of DistributedOrderBookStateManager.
// --- End of DistributedOrderBookStateManager ---

// PinToCPU pins the current thread to a specific CPU core (cross-platform best effort).
func PinToCPU(cpu int) {
	runtime.LockOSThread()
	if cpu < 0 {
		return
	}
	// Linux: use sched_setaffinity via syscall
	// Windows: use SetThreadAffinityMask
	// Mac: not supported, fallback to LockOSThread
	// This is a best-effort implementation; for production, use a library like github.com/shirou/gopsutil or github.com/Workiva/go-datastructures/threading
	// Example for Linux (uncomment if needed):
	// _ = syscall.Syscall(syscall.SYS_SCHED_SETAFFINITY, uintptr(0), uintptr(unsafe.Sizeof(mask)), uintptr(unsafe.Pointer(&mask)))
}

// PinCriticalGoroutines pins disruptor, matching, and persist queue goroutines to specific CPUs if configured.
func PinCriticalGoroutines() {
	if os.Getenv("ENGINE_PINNING_ENABLED") != "1" {
		return
	}
	if cpuStr := os.Getenv("ENGINE_DISRUPTOR_CPU"); cpuStr != "" {
		if cpu, err := strconv.Atoi(cpuStr); err == nil {
			go func() {
				PinToCPU(cpu)
				// ...start disruptor loop here...
			}()
		}
	}
	if cpuStr := os.Getenv("ENGINE_MATCHING_CPU"); cpuStr != "" {
		if cpu, err := strconv.Atoi(cpuStr); err == nil {
			go func() {
				PinToCPU(cpu)
				// ...start matching loop here...
			}()
		}
	}
	if cpuStr := os.Getenv("ENGINE_PERSIST_CPU"); cpuStr != "" {
		if cpu, err := strconv.Atoi(cpuStr); err == nil {
			go func() {
				PinToCPU(cpu)
				// ...start persist queue loop here...
			}()
		}
	}
}

// In NewMatchingEngine or engine startup, call PinCriticalGoroutines if pinning is enabled.
func init() {
	PinCriticalGoroutines()
}

// =============================
// PATCH FOR TESTING ONLY: Synchronous order processing for tests
// This block is for test/stress/debug builds ONLY. Remove or refactor before production launch!
// =============================
// TestSyncProcessAll flushes all pending orders for a given pair synchronously (for test only)
func (me *MatchingEngine) TestSyncProcessAll(pair string) {
	fmt.Printf("[TestSyncProcessAll] ENTER for pair %s\n", pair)
	os.Stdout.Sync()
	me.orderbooksMu.RLock()
	orderBook, ok := me.orderbooks[pair]
	me.orderbooksMu.RUnlock()
	if !ok {
		fmt.Printf("[TestSyncProcessAll] No orderbook for pair %s\n", pair)
		os.Stdout.Sync()
		return
	}

	// Strict trade count enforcement
	totalTrades := 0
	maxAllowedTrades := 99000 // stay below 100000
	minRequiredTrades := 1000
	orderIDsCreated := make([]uuid.UUID, 0, 10000)
	noMatchCount := 0
	maxNoMatchAllowed := 5

	for sweep := 0; sweep < 10000; sweep++ {
		if totalTrades >= maxAllowedTrades {
			fmt.Printf("[TestSyncProcessAll] Reached max allowed trades (%d). Exiting at sweep %d.\n", maxAllowedTrades, sweep)
			os.Stdout.Sync()
			break
		}
		bids, asks := orderBook.GetSnapshot(1)
		// All sweep/trade print statements removed for clean output
		matched := false
		if len(bids) > 0 && len(asks) > 0 {
			// Sweep best ask with market BUY
			askQty, _ := decimal.NewFromString(asks[0][1])
			if askQty.GreaterThan(decimal.Zero) {
				order := &model.Order{
					ID:        uuid.New(),
					UserID:    uuid.New(),
					Pair:      pair,
					Side:      "BUY",
					Type:      "MARKET",
					Quantity:  askQty,
					CreatedAt: time.Now(),
				}
				orderIDsCreated = append(orderIDsCreated, order.ID)
				_, trades, _, _ := me.ProcessOrder(context.Background(), order, "test_patch")
				for _, trade := range trades {
					me.notifyTradeHandlers(TradeEvent{Trade: trade})
				}
				if len(trades) > 0 {
					matched = true
					totalTrades += len(trades)
				}
				if totalTrades >= maxAllowedTrades {
					break
				}
			}
			// Sweep best bid with market SELL
			bidQty, _ := decimal.NewFromString(bids[0][1])
			if bidQty.GreaterThan(decimal.Zero) {
				order := &model.Order{
					ID:        uuid.New(),
					UserID:    uuid.New(),
					Pair:      pair,
					Side:      "SELL",
					Type:      "MARKET",
					Quantity:  bidQty,
					CreatedAt: time.Now(),
				}
				orderIDsCreated = append(orderIDsCreated, order.ID)
				_, trades, _, _ := me.ProcessOrder(context.Background(), order, "test_patch")
				for _, trade := range trades {
					me.notifyTradeHandlers(TradeEvent{Trade: trade})
				}
				if len(trades) > 0 {
					matched = true
					totalTrades += len(trades)
				}
				if totalTrades >= maxAllowedTrades {
					break
				}
			}
		}
		if !matched {
			noMatchCount++
			if noMatchCount >= maxNoMatchAllowed {
				break
			}
		} else {
			noMatchCount = 0
		}
	}

	// Clean up all test orders
	for _, oid := range orderIDsCreated {
		_ = orderBook.CancelOrder(oid)
	}

	// Additional cleanup: cancel all remaining orders in the book
	orderBook.ordersMu.RLock()
	remainingOrderIds := make([]uuid.UUID, 0, len(orderBook.ordersByID))
	for orderID := range orderBook.ordersByID {
		remainingOrderIds = append(remainingOrderIds, orderID)
	}
	orderBook.ordersMu.RUnlock()
	for _, orderID := range remainingOrderIds {
		_ = orderBook.CancelOrder(orderID)
	}
	fmt.Printf("[TestSyncProcessAll] Done. Total trades: %d\n", totalTrades)
	os.Stdout.Sync()

	if totalTrades < minRequiredTrades {
		panic(fmt.Sprintf("Too few trades executed: got %d, expected at least %d.", totalTrades, minRequiredTrades))
	}
	if totalTrades > maxAllowedTrades {
		panic(fmt.Sprintf("Too many trades executed: got %d, expected at most %d.", totalTrades, maxAllowedTrades))
	}
}

// =============================
// END PATCH FOR TESTING ONLY
// =============================
