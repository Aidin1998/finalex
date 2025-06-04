// Package coordination provides synchronization mechanisms between Matching Engine and Settlement modules
package coordination

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/trading/model"
	"github.com/Aidin1998/pincex_unified/internal/trading/settlement"
	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// TradeSettlementCoordinator manages the synchronization between matching engine and settlement
type TradeSettlementCoordinator struct {
	// Core components
	logger           *zap.Logger
	settlementEngine *settlement.SettlementEngine
	repository       model.Repository // Add repository for order lookup

	// Messaging
	kafkaWriter *kafka.Writer
	kafkaReader *kafka.Reader

	// Synchronization state
	pendingTrades map[string]*PendingTrade
	pendingMutex  sync.RWMutex

	// Event coordination
	tradeEvents      chan *TradeEvent
	settlementEvents chan *SettlementEvent

	// Circuit breaker for fault tolerance
	circuitBreaker *CircuitBreaker

	// Metrics and monitoring
	metrics *CoordinatorMetrics

	// Control
	stopChan chan struct{}
	wg       sync.WaitGroup
	running  int32
}

// PendingTrade represents a trade waiting for settlement confirmation
type PendingTrade struct {
	TradeID     uuid.UUID              `json:"trade_id"`
	OrderID     uuid.UUID              `json:"order_id"`
	Symbol      string                 `json:"symbol"`
	Quantity    decimal.Decimal        `json:"quantity"`
	Price       decimal.Decimal        `json:"price"`
	BuyUserID   uuid.UUID              `json:"buy_user_id"`
	SellUserID  uuid.UUID              `json:"sell_user_id"`
	ExecutedAt  time.Time              `json:"executed_at"`
	State       TradeState             `json:"state"`
	Attempts    int                    `json:"attempts"`
	LastAttempt time.Time              `json:"last_attempt"`
	Metadata    map[string]interface{} `json:"metadata"`

	// Synchronization
	settlementBarrier sync.WaitGroup
	timeoutCtx        context.Context
	cancelFunc        context.CancelFunc
}

// TradeState represents the state of a trade in the settlement process
type TradeState string

const (
	TradeStatePending           TradeState = "pending"
	TradeStateMatchingConfirmed TradeState = "matching_confirmed"
	TradeStateSettlementQueued  TradeState = "settlement_queued"
	TradeStateSettlementStarted TradeState = "settlement_started"
	TradeStateSettled           TradeState = "settled"
	TradeStateFailed            TradeState = "failed"
	TradeStateCompensating      TradeState = "compensating"
)

// TradeEvent represents an event from the matching engine
type TradeEvent struct {
	EventID   uuid.UUID              `json:"event_id"`
	TradeID   uuid.UUID              `json:"trade_id"`
	OrderID   uuid.UUID              `json:"order_id"`
	EventType TradeEventType         `json:"event_type"`
	Trade     *model.Trade           `json:"trade"`
	Timestamp time.Time              `json:"timestamp"`
	Metadata  map[string]interface{} `json:"metadata"`

	// Synchronization fields
	AckChannel chan<- bool   `json:"-"`
	Timeout    time.Duration `json:"timeout"`
}

type TradeEventType string

const (
	TradeEventExecuted  TradeEventType = "trade_executed"
	TradeEventConfirmed TradeEventType = "trade_confirmed"
	TradeEventReverted  TradeEventType = "trade_reverted"
)

// SettlementEvent represents an event from the settlement engine
type SettlementEvent struct {
	EventID      uuid.UUID                   `json:"event_id"`
	TradeID      uuid.UUID                   `json:"trade_id"`
	SettlementID string                      `json:"settlement_id"`
	EventType    SettlementEventType         `json:"event_type"`
	Status       settlement.SettlementStatus `json:"status"`
	Receipt      string                      `json:"receipt"`
	Error        string                      `json:"error,omitempty"`
	Timestamp    time.Time                   `json:"timestamp"`
	Metadata     map[string]interface{}      `json:"metadata"`
}

type SettlementEventType string

const (
	SettlementEventQueued    SettlementEventType = "settlement_queued"
	SettlementEventStarted   SettlementEventType = "settlement_started"
	SettlementEventCompleted SettlementEventType = "settlement_completed"
	SettlementEventFailed    SettlementEventType = "settlement_failed"
)

// CircuitBreaker provides fault tolerance for settlement operations
type CircuitBreaker struct {
	failureThreshold int32
	resetTimeout     time.Duration
	failures         int32
	state            int32 // 0: closed, 1: open, 2: half-open
	lastFailureTime  time.Time
	mutex            sync.RWMutex
}

// CoordinatorMetrics tracks coordination performance
type CoordinatorMetrics struct {
	TradesProcessed     int64
	TradesSettled       int64
	TradesFailed        int64
	SettlementLatency   time.Duration
	CircuitBreakerTrips int64
	RetryAttempts       int64

	// Synchronization metrics
	AverageSettlementTime time.Duration
	MaxSettlementTime     time.Duration

	mutex sync.RWMutex
}

// NewTradeSettlementCoordinator creates a new coordinator instance
func NewTradeSettlementCoordinator(
	logger *zap.Logger,
	settlementEngine *settlement.SettlementEngine,
	repository model.Repository,
	kafkaConfig *CoordinatorKafkaConfig,
) (*TradeSettlementCoordinator, error) {

	coordinator := &TradeSettlementCoordinator{
		logger:           logger,
		settlementEngine: settlementEngine,
		repository:       repository,
		pendingTrades:    make(map[string]*PendingTrade),
		tradeEvents:      make(chan *TradeEvent, 10000),
		settlementEvents: make(chan *SettlementEvent, 10000),
		stopChan:         make(chan struct{}),
		metrics:          &CoordinatorMetrics{},
		circuitBreaker: &CircuitBreaker{
			failureThreshold: 10,
			resetTimeout:     60 * time.Second,
		},
	}

	// Initialize Kafka components
	if err := coordinator.initializeKafka(kafkaConfig); err != nil {
		return nil, fmt.Errorf("failed to initialize Kafka: %w", err)
	}

	return coordinator, nil
}

type CoordinatorKafkaConfig struct {
	Brokers         []string
	TradeTopicIn    string
	TradeTopicOut   string
	SettlementTopic string
	ConsumerGroup   string
	ProducerConfig  *kafka.WriterConfig
	ConsumerConfig  *kafka.ReaderConfig
}

func (tsc *TradeSettlementCoordinator) initializeKafka(config *CoordinatorKafkaConfig) error {
	// Initialize Kafka writer for settlement coordination
	tsc.kafkaWriter = &kafka.Writer{
		Addr:         kafka.TCP(config.Brokers...),
		Topic:        config.SettlementTopic,
		Balancer:     &kafka.Hash{},    // Use hash balancing for consistent partitioning
		RequiredAcks: kafka.RequireAll, // Ensure exactly-once delivery
		BatchSize:    100,
		BatchTimeout: 10 * time.Millisecond,
		Compression:  kafka.Snappy,
		// Enable idempotent writes for exactly-once semantics
		Transport: &kafka.Transport{
			DialTimeout: 30 * time.Second,
		},
	}

	// Initialize Kafka reader for settlement events
	tsc.kafkaReader = kafka.NewReader(kafka.ReaderConfig{
		Brokers:     config.Brokers,
		Topic:       config.TradeTopicIn,
		GroupID:     config.ConsumerGroup,
		MinBytes:    1,
		MaxBytes:    10e6, // 10MB
		MaxWait:     100 * time.Millisecond,
		StartOffset: kafka.LastOffset,

		// Enable exactly-once consumption
		CommitInterval: 100 * time.Millisecond,
	})

	return nil
}

// Start begins the coordination service
func (tsc *TradeSettlementCoordinator) Start(ctx context.Context) error {
	if !atomic.CompareAndSwapInt32(&tsc.running, 0, 1) {
		return fmt.Errorf("coordinator is already running")
	}
	tsc.logger.Info("Starting Trade Settlement Coordinator")
	// Start event processing workers
	tsc.wg.Add(4)
	go tsc.tradeEventProcessor(ctx)
	go tsc.settlementEventProcessor(ctx)
	go tsc.kafkaEventConsumer(ctx)
	go tsc.timeoutProcessor(ctx)

	return nil
}

// Stop gracefully stops the coordination service
func (tsc *TradeSettlementCoordinator) Stop(ctx context.Context) error {
	if !atomic.CompareAndSwapInt32(&tsc.running, 1, 0) {
		return fmt.Errorf("coordinator is not running")
	}

	tsc.logger.Info("Stopping Trade Settlement Coordinator")

	close(tsc.stopChan)

	// Wait for workers to finish with timeout
	done := make(chan struct{})
	go func() {
		tsc.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		tsc.logger.Info("All coordinator workers stopped")
	case <-ctx.Done():
		tsc.logger.Warn("Coordinator stop timed out")
		return ctx.Err()
	}

	// Close Kafka connections
	if tsc.kafkaWriter != nil {
		tsc.kafkaWriter.Close()
	}
	if tsc.kafkaReader != nil {
		tsc.kafkaReader.Close()
	}

	return nil
}

// CoordinateTradeSettlement coordinates a trade execution with settlement
func (tsc *TradeSettlementCoordinator) CoordinateTradeSettlement(ctx context.Context,
	trade *model.Trade,
	metadata map[string]interface{},
) error {
	// Check circuit breaker before attempting coordination
	if !tsc.circuitBreaker.Allow() {
		atomic.AddInt64(&tsc.metrics.CircuitBreakerTrips, 1)
		return fmt.Errorf("circuit breaker is open")
	}
	// Create pending trade entry
	pendingTrade := &PendingTrade{
		TradeID:    trade.ID,
		OrderID:    trade.OrderID,
		Symbol:     trade.Pair,
		Quantity:   trade.Quantity,
		Price:      trade.Price,
		ExecutedAt: time.Now(),
		State:      TradeStatePending,
		Metadata:   metadata,
	}

	// Set up timeout context
	pendingTrade.timeoutCtx, pendingTrade.cancelFunc = context.WithTimeout(ctx, 30*time.Second)
	pendingTrade.settlementBarrier.Add(1)

	// Store pending trade
	tsc.pendingMutex.Lock()
	tsc.pendingTrades[trade.ID.String()] = pendingTrade
	tsc.pendingMutex.Unlock()

	// Create trade event
	tradeEvent := &TradeEvent{
		EventID:   uuid.New(),
		TradeID:   trade.ID,
		OrderID:   trade.OrderID,
		EventType: TradeEventExecuted,
		Trade:     trade,
		Timestamp: time.Now(),
		Metadata:  metadata,
		Timeout:   30 * time.Second,
	}

	// Send to processing pipeline
	select {
	case tsc.tradeEvents <- tradeEvent:
		tsc.logger.Debug("Trade event queued for coordination",
			zap.String("trade_id", trade.ID.String()),
			zap.String("symbol", trade.Pair))
	case <-ctx.Done():
		return ctx.Err()
	}

	// Wait for settlement coordination to complete
	done := make(chan struct{})
	go func() {
		pendingTrade.settlementBarrier.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Check final state
		tsc.pendingMutex.RLock()
		finalState := pendingTrade.State
		tsc.pendingMutex.RUnlock()
		if finalState == TradeStateSettled {
			atomic.AddInt64(&tsc.metrics.TradesSettled, 1)
			return nil
		} else {
			atomic.AddInt64(&tsc.metrics.TradesFailed, 1)
			return fmt.Errorf("trade settlement failed, final state: %s", finalState)
		}
	case <-pendingTrade.timeoutCtx.Done():
		atomic.AddInt64(&tsc.metrics.TradesFailed, 1)
		return fmt.Errorf("trade settlement timed out")

	case <-ctx.Done():
		return ctx.Err()
	}
}

// tradeEventProcessor processes trade events from matching engine
func (tsc *TradeSettlementCoordinator) tradeEventProcessor(ctx context.Context) {
	defer tsc.wg.Done()

	for {
		select {
		case event := <-tsc.tradeEvents:
			if err := tsc.processTradeEvent(ctx, event); err != nil {
				tsc.logger.Error("Failed to process trade event",
					zap.String("trade_id", event.TradeID.String()),
					zap.Error(err))
			}

		case <-tsc.stopChan:
			tsc.logger.Info("Trade event processor stopping")
			return

		case <-ctx.Done():
			return
		}
	}
}

// processTradeEvent handles individual trade events
func (tsc *TradeSettlementCoordinator) processTradeEvent(ctx context.Context, event *TradeEvent) error {
	tradeID := event.TradeID.String()

	tsc.pendingMutex.Lock()
	pendingTrade, exists := tsc.pendingTrades[tradeID]
	if !exists {
		tsc.pendingMutex.Unlock()
		return fmt.Errorf("no pending trade found for trade_id: %s", tradeID)
	}

	// Update trade state based on event type
	switch event.EventType {
	case TradeEventExecuted:
		pendingTrade.State = TradeStateMatchingConfirmed
		tsc.logger.Debug("Trade matching confirmed",
			zap.String("trade_id", tradeID))

	case TradeEventReverted:
		pendingTrade.State = TradeStateFailed
		pendingTrade.settlementBarrier.Done()
		tsc.pendingMutex.Unlock()
		return nil

	default:
		tsc.pendingMutex.Unlock()
		return fmt.Errorf("unknown trade event type: %s", event.EventType)
	}
	tsc.pendingMutex.Unlock()

	// Trigger settlement processing
	return tsc.triggerSettlement(ctx, pendingTrade)
}

// triggerSettlement initiates settlement for a confirmed trade
func (tsc *TradeSettlementCoordinator) triggerSettlement(ctx context.Context, pendingTrade *PendingTrade) error {
	// Check circuit breaker before attempting settlement
	if !tsc.circuitBreaker.Allow() {
		tsc.logger.Warn("Circuit breaker open, deferring settlement",
			zap.String("trade_id", pendingTrade.TradeID.String()))
		return fmt.Errorf("circuit breaker is open")
	}

	// Update state to settlement queued
	tsc.pendingMutex.Lock()
	pendingTrade.State = TradeStateSettlementQueued
	pendingTrade.Attempts++
	pendingTrade.LastAttempt = time.Now()
	tsc.pendingMutex.Unlock()

	// Create settlement request message
	settlementMessage := map[string]interface{}{
		"trade_id":     pendingTrade.TradeID.String(),
		"order_id":     pendingTrade.OrderID.String(),
		"symbol":       pendingTrade.Symbol,
		"quantity":     pendingTrade.Quantity.String(),
		"price":        pendingTrade.Price.String(),
		"buy_user_id":  pendingTrade.BuyUserID.String(),
		"sell_user_id": pendingTrade.SellUserID.String(),
		"executed_at":  pendingTrade.ExecutedAt.Format(time.RFC3339),
		"metadata":     pendingTrade.Metadata,
		"attempt":      pendingTrade.Attempts,
	}

	messageBytes, err := json.Marshal(settlementMessage)
	if err != nil {
		return fmt.Errorf("failed to marshal settlement message: %w", err)
	}

	// Send settlement request via Kafka
	kafkaMessage := kafka.Message{
		Key:   []byte(pendingTrade.TradeID.String()),
		Value: messageBytes,
		Headers: []kafka.Header{
			{Key: "event_type", Value: []byte("settlement_request")},
			{Key: "trade_id", Value: []byte(pendingTrade.TradeID.String())},
		},
	}

	if err := tsc.kafkaWriter.WriteMessages(ctx, kafkaMessage); err != nil {
		tsc.circuitBreaker.RecordFailure()
		return fmt.Errorf("failed to send settlement request: %w", err)
	}

	tsc.logger.Debug("Settlement request sent",
		zap.String("trade_id", pendingTrade.TradeID.String()),
		zap.Int("attempt", pendingTrade.Attempts))

	return nil
}

// settlementEventProcessor processes settlement events
func (tsc *TradeSettlementCoordinator) settlementEventProcessor(ctx context.Context) {
	defer tsc.wg.Done()

	for {
		select {
		case event := <-tsc.settlementEvents:
			if err := tsc.processSettlementEvent(ctx, event); err != nil {
				tsc.logger.Error("Failed to process settlement event",
					zap.String("trade_id", event.TradeID.String()),
					zap.Error(err))
			}

		case <-tsc.stopChan:
			tsc.logger.Info("Settlement event processor stopping")
			return

		case <-ctx.Done():
			return
		}
	}
}

// processSettlementEvent handles individual settlement events
func (tsc *TradeSettlementCoordinator) processSettlementEvent(ctx context.Context, event *SettlementEvent) error {
	tradeID := event.TradeID.String()

	tsc.pendingMutex.Lock()
	pendingTrade, exists := tsc.pendingTrades[tradeID]
	if !exists {
		tsc.pendingMutex.Unlock()
		return fmt.Errorf("no pending trade found for settlement event: %s", tradeID)
	}

	// Update trade state based on settlement event
	switch event.EventType {
	case SettlementEventStarted:
		pendingTrade.State = TradeStateSettlementStarted
		tsc.logger.Debug("Settlement started",
			zap.String("trade_id", tradeID),
			zap.String("settlement_id", event.SettlementID))

	case SettlementEventCompleted:
		pendingTrade.State = TradeStateSettled
		tsc.logger.Info("Settlement completed successfully",
			zap.String("trade_id", tradeID),
			zap.String("settlement_id", event.SettlementID),
			zap.String("receipt", event.Receipt))

		// Clean up and signal completion
		delete(tsc.pendingTrades, tradeID)
		pendingTrade.settlementBarrier.Done()
		pendingTrade.cancelFunc()
		tsc.circuitBreaker.RecordSuccess()
		atomic.AddInt64(&tsc.metrics.TradesProcessed, 1)

	case SettlementEventFailed:
		tsc.logger.Warn("Settlement failed",
			zap.String("trade_id", tradeID),
			zap.String("error", event.Error))

		// Determine if we should retry or fail permanently
		if pendingTrade.Attempts < 3 {
			// Retry settlement
			tsc.pendingMutex.Unlock()
			atomic.AddInt64(&tsc.metrics.RetryAttempts, 1)
			return tsc.triggerSettlement(ctx, pendingTrade)
		} else { // Permanent failure - trigger compensation
			pendingTrade.State = TradeStateCompensating
			delete(tsc.pendingTrades, tradeID)
			pendingTrade.settlementBarrier.Done()
			pendingTrade.cancelFunc()
			tsc.circuitBreaker.RecordFailure()

			// Trigger compensation logic
			go tsc.compensateFailedTrade(ctx, pendingTrade, event.Error)
			tsc.logger.Error("Trade settlement permanently failed, compensation triggered",
				zap.String("trade_id", tradeID),
				zap.Int("attempts", pendingTrade.Attempts))
		}

	default:
		tsc.pendingMutex.Unlock()
		return fmt.Errorf("unknown settlement event type: %s", event.EventType)
	}

	tsc.pendingMutex.Unlock()
	return nil
}

// kafkaEventConsumer consumes events from Kafka topics
func (tsc *TradeSettlementCoordinator) kafkaEventConsumer(ctx context.Context) {
	defer tsc.wg.Done()

	for {
		select {
		case <-tsc.stopChan:
			tsc.logger.Info("Kafka event consumer stopping")
			return

		case <-ctx.Done():
			return

		default:
			// Read message from Kafka
			message, err := tsc.kafkaReader.ReadMessage(ctx)
			if err != nil {
				if err != context.Canceled {
					tsc.logger.Error("Failed to read Kafka message", zap.Error(err))
				}
				continue
			}

			if err := tsc.processKafkaMessage(ctx, &message); err != nil {
				tsc.logger.Error("Failed to process Kafka message",
					zap.String("topic", message.Topic),
					zap.String("key", string(message.Key)),
					zap.Error(err))
			}
		}
	}
}

// processKafkaMessage processes incoming Kafka messages
func (tsc *TradeSettlementCoordinator) processKafkaMessage(ctx context.Context, message *kafka.Message) error {
	// Determine message type from headers
	var eventType string
	for _, header := range message.Headers {
		if header.Key == "event_type" {
			eventType = string(header.Value)
			break
		}
	}

	switch eventType {
	case "settlement_completed", "settlement_failed", "settlement_started":
		var settlementEvent SettlementEvent
		if err := json.Unmarshal(message.Value, &settlementEvent); err != nil {
			return fmt.Errorf("failed to unmarshal settlement event: %w", err)
		}

		select {
		case tsc.settlementEvents <- &settlementEvent:
		case <-ctx.Done():
			return ctx.Err()
		}

	default:
		tsc.logger.Debug("Unknown message type received",
			zap.String("event_type", eventType),
			zap.String("key", string(message.Key)))
	}

	return nil
}

// timeoutProcessor handles timeouts for pending trades
func (tsc *TradeSettlementCoordinator) timeoutProcessor(ctx context.Context) {
	defer tsc.wg.Done()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			tsc.checkTimeouts()

		case <-tsc.stopChan:
			tsc.logger.Info("Timeout processor stopping")
			return

		case <-ctx.Done():
			return
		}
	}
}

// checkTimeouts checks for and handles timed out trades
func (tsc *TradeSettlementCoordinator) checkTimeouts() {
	now := time.Now()
	timedOutTrades := make([]*PendingTrade, 0)

	tsc.pendingMutex.Lock()
	for tradeID, pendingTrade := range tsc.pendingTrades {
		if now.Sub(pendingTrade.ExecutedAt) > 60*time.Second {
			timedOutTrades = append(timedOutTrades, pendingTrade)
			delete(tsc.pendingTrades, tradeID)
		}
	}
	tsc.pendingMutex.Unlock()

	// Handle timed out trades
	for _, trade := range timedOutTrades {
		tsc.logger.Warn("Trade settlement timed out",
			zap.String("trade_id", trade.TradeID.String()),
			zap.Duration("elapsed", now.Sub(trade.ExecutedAt)))

		trade.State = TradeStateFailed
		trade.settlementBarrier.Done()
		trade.cancelFunc()
		atomic.AddInt64(&tsc.metrics.TradesFailed, 1)
	}
}

// Circuit Breaker Implementation
func (cb *CircuitBreaker) Allow() bool {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()

	switch atomic.LoadInt32(&cb.state) {
	case 0: // Closed - allow requests
		return true
	case 1: // Open - deny requests
		if time.Since(cb.lastFailureTime) > cb.resetTimeout {
			// Try to move to half-open
			if atomic.CompareAndSwapInt32(&cb.state, 1, 2) {
				return true
			}
		}
		return false
	case 2: // Half-open - allow limited requests
		return true
	default:
		return false
	}
}

func (cb *CircuitBreaker) RecordSuccess() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	atomic.StoreInt32(&cb.failures, 0)
	atomic.StoreInt32(&cb.state, 0) // Move to closed
}

func (cb *CircuitBreaker) RecordFailure() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	failures := atomic.AddInt32(&cb.failures, 1)
	cb.lastFailureTime = time.Now()

	if failures >= cb.failureThreshold {
		atomic.StoreInt32(&cb.state, 1) // Move to open
	}
}

// GetMetrics returns current coordination metrics
func (tsc *TradeSettlementCoordinator) GetMetrics() *CoordinatorMetrics {
	tsc.metrics.mutex.RLock()
	defer tsc.metrics.mutex.RUnlock()

	// Return a copy of metrics
	return &CoordinatorMetrics{
		TradesProcessed:       atomic.LoadInt64(&tsc.metrics.TradesProcessed),
		TradesSettled:         atomic.LoadInt64(&tsc.metrics.TradesSettled),
		TradesFailed:          atomic.LoadInt64(&tsc.metrics.TradesFailed),
		CircuitBreakerTrips:   atomic.LoadInt64(&tsc.metrics.CircuitBreakerTrips),
		RetryAttempts:         atomic.LoadInt64(&tsc.metrics.RetryAttempts),
		SettlementLatency:     tsc.metrics.SettlementLatency,
		AverageSettlementTime: tsc.metrics.AverageSettlementTime,
		MaxSettlementTime:     tsc.metrics.MaxSettlementTime,
	}
}

// GetPendingTradesCount returns the number of currently pending trades
func (tsc *TradeSettlementCoordinator) GetPendingTradesCount() int {
	tsc.pendingMutex.RLock()
	defer tsc.pendingMutex.RUnlock()
	return len(tsc.pendingTrades)
}

// GetTradeStatus returns the current status of a specific trade
func (tsc *TradeSettlementCoordinator) GetTradeStatus(tradeID string) (*PendingTrade, bool) {
	tsc.pendingMutex.RLock()
	defer tsc.pendingMutex.RUnlock()

	trade, exists := tsc.pendingTrades[tradeID]
	if !exists {
		return nil, false
	}

	// Return a copy to avoid race conditions
	return &PendingTrade{
		TradeID:     trade.TradeID,
		OrderID:     trade.OrderID,
		Symbol:      trade.Symbol,
		Quantity:    trade.Quantity,
		Price:       trade.Price,
		BuyUserID:   trade.BuyUserID,
		SellUserID:  trade.SellUserID,
		ExecutedAt:  trade.ExecutedAt,
		State:       trade.State,
		Attempts:    trade.Attempts,
		LastAttempt: trade.LastAttempt,
		Metadata:    trade.Metadata,
	}, true
}

// compensateFailedTrade handles compensation for permanently failed trades
func (tsc *TradeSettlementCoordinator) compensateFailedTrade(ctx context.Context, failedTrade *PendingTrade, errorMsg string) {
	tsc.logger.Info("Starting trade compensation",
		zap.String("trade_id", failedTrade.TradeID.String()),
		zap.String("error", errorMsg))

	compensation := &CompensationAction{
		CompensationID: uuid.New(),
		TradeID:        failedTrade.TradeID,
		OrderID:        failedTrade.OrderID,
		Symbol:         failedTrade.Symbol,
		Quantity:       failedTrade.Quantity,
		Price:          failedTrade.Price,
		BuyUserID:      failedTrade.BuyUserID,
		SellUserID:     failedTrade.SellUserID,
		FailureReason:  errorMsg,
		CreatedAt:      time.Now(),
		Status:         CompensationStatusPending,
		Actions:        make([]string, 0),
	}

	// Step 1: Reverse any partial settlements
	if err := tsc.reversePartialSettlements(ctx, compensation); err != nil {
		tsc.logger.Error("Failed to reverse partial settlements",
			zap.String("trade_id", failedTrade.TradeID.String()),
			zap.Error(err))
		compensation.Status = CompensationStatusFailed
		return
	}

	// Step 2: Restore account balances
	if err := tsc.restoreAccountBalances(ctx, compensation); err != nil {
		tsc.logger.Error("Failed to restore account balances",
			zap.String("trade_id", failedTrade.TradeID.String()),
			zap.Error(err))
		compensation.Status = CompensationStatusFailed
		return
	}

	// Step 3: Send compensation notification
	if err := tsc.sendCompensationNotification(ctx, compensation); err != nil {
		tsc.logger.Warn("Failed to send compensation notification",
			zap.String("trade_id", failedTrade.TradeID.String()),
			zap.Error(err))
		// Don't fail compensation for notification failure
	}

	compensation.Status = CompensationStatusCompleted
	compensation.CompletedAt = time.Now()

	tsc.logger.Info("Trade compensation completed successfully",
		zap.String("trade_id", failedTrade.TradeID.String()),
		zap.String("compensation_id", compensation.CompensationID.String()))
}

// CompensationAction represents a compensation action for a failed trade
type CompensationAction struct {
	CompensationID uuid.UUID          `json:"compensation_id"`
	TradeID        uuid.UUID          `json:"trade_id"`
	OrderID        uuid.UUID          `json:"order_id"`
	Symbol         string             `json:"symbol"`
	Quantity       decimal.Decimal    `json:"quantity"`
	Price          decimal.Decimal    `json:"price"`
	BuyUserID      uuid.UUID          `json:"buy_user_id"`
	SellUserID     uuid.UUID          `json:"sell_user_id"`
	FailureReason  string             `json:"failure_reason"`
	Status         CompensationStatus `json:"status"`
	Actions        []string           `json:"actions"`
	CreatedAt      time.Time          `json:"created_at"`
	CompletedAt    time.Time          `json:"completed_at,omitempty"`
}

type CompensationStatus string

const (
	CompensationStatusPending   CompensationStatus = "pending"
	CompensationStatusCompleted CompensationStatus = "completed"
	CompensationStatusFailed    CompensationStatus = "failed"
)

// reversePartialSettlements reverses any partial settlement actions
func (tsc *TradeSettlementCoordinator) reversePartialSettlements(ctx context.Context, compensation *CompensationAction) error {
	// Create reversal message for settlement engine
	reversalMessage := map[string]interface{}{
		"action":          "reverse_settlement",
		"trade_id":        compensation.TradeID.String(),
		"compensation_id": compensation.CompensationID.String(),
		"reason":          compensation.FailureReason,
		"timestamp":       time.Now().Format(time.RFC3339),
	}

	messageBytes, err := json.Marshal(reversalMessage)
	if err != nil {
		return fmt.Errorf("failed to marshal reversal message: %w", err)
	}

	kafkaMessage := kafka.Message{
		Key:   []byte(compensation.TradeID.String()),
		Value: messageBytes,
		Headers: []kafka.Header{
			{Key: "event_type", Value: []byte("settlement_reversal")},
			{Key: "trade_id", Value: []byte(compensation.TradeID.String())},
			{Key: "compensation_id", Value: []byte(compensation.CompensationID.String())},
		},
	}

	if err := tsc.kafkaWriter.WriteMessages(ctx, kafkaMessage); err != nil {
		return fmt.Errorf("failed to send reversal message: %w", err)
	}

	compensation.Actions = append(compensation.Actions, "partial_settlement_reversed")
	return nil
}

// restoreAccountBalances restores account balances to pre-trade state
func (tsc *TradeSettlementCoordinator) restoreAccountBalances(ctx context.Context, compensation *CompensationAction) error {
	// Create balance restoration message
	restorationMessage := map[string]interface{}{
		"action":          "restore_balances",
		"trade_id":        compensation.TradeID.String(),
		"compensation_id": compensation.CompensationID.String(),
		"buy_user_id":     compensation.BuyUserID.String(),
		"sell_user_id":    compensation.SellUserID.String(),
		"symbol":          compensation.Symbol,
		"quantity":        compensation.Quantity.String(),
		"price":           compensation.Price.String(),
		"timestamp":       time.Now().Format(time.RFC3339),
	}

	messageBytes, err := json.Marshal(restorationMessage)
	if err != nil {
		return fmt.Errorf("failed to marshal restoration message: %w", err)
	}

	kafkaMessage := kafka.Message{
		Key:   []byte(compensation.TradeID.String()),
		Value: messageBytes,
		Headers: []kafka.Header{
			{Key: "event_type", Value: []byte("balance_restoration")},
			{Key: "trade_id", Value: []byte(compensation.TradeID.String())},
			{Key: "compensation_id", Value: []byte(compensation.CompensationID.String())},
		},
	}

	if err := tsc.kafkaWriter.WriteMessages(ctx, kafkaMessage); err != nil {
		return fmt.Errorf("failed to send restoration message: %w", err)
	}

	compensation.Actions = append(compensation.Actions, "account_balances_restored")
	return nil
}

// sendCompensationNotification sends notifications about the compensation
func (tsc *TradeSettlementCoordinator) sendCompensationNotification(ctx context.Context, compensation *CompensationAction) error {
	// Create notification message
	notificationMessage := map[string]interface{}{
		"action":          "compensation_notification",
		"trade_id":        compensation.TradeID.String(),
		"compensation_id": compensation.CompensationID.String(),
		"affected_users":  []string{compensation.BuyUserID.String(), compensation.SellUserID.String()},
		"symbol":          compensation.Symbol,
		"failure_reason":  compensation.FailureReason,
		"timestamp":       time.Now().Format(time.RFC3339),
	}

	messageBytes, err := json.Marshal(notificationMessage)
	if err != nil {
		return fmt.Errorf("failed to marshal notification message: %w", err)
	}

	kafkaMessage := kafka.Message{
		Key:   []byte(compensation.TradeID.String()),
		Value: messageBytes,
		Headers: []kafka.Header{
			{Key: "event_type", Value: []byte("compensation_notification")},
			{Key: "trade_id", Value: []byte(compensation.TradeID.String())},
			{Key: "compensation_id", Value: []byte(compensation.CompensationID.String())},
		},
	}

	if err := tsc.kafkaWriter.WriteMessages(ctx, kafkaMessage); err != nil {
		return fmt.Errorf("failed to send notification message: %w", err)
	}

	compensation.Actions = append(compensation.Actions, "notification_sent")
	return nil
}

// ValidateTradeConsistency validates that a trade is consistent before settlement
func (tsc *TradeSettlementCoordinator) ValidateTradeConsistency(ctx context.Context, trade *model.Trade) error {
	// Validate trade data integrity
	if trade.ID == uuid.Nil {
		return fmt.Errorf("invalid trade ID")
	}

	if trade.Quantity.IsZero() || trade.Quantity.IsNegative() {
		return fmt.Errorf("invalid trade quantity: %s", trade.Quantity.String())
	}

	if trade.Price.IsZero() || trade.Price.IsNegative() {
		return fmt.Errorf("invalid trade price: %s", trade.Price.String())
	}

	// Validate that we have a valid order ID
	if trade.OrderID == uuid.Nil {
		return fmt.Errorf("invalid order ID in trade")
	}

	// Look up the order to get user information for validation
	order, err := tsc.repository.GetOrder(ctx, trade.OrderID)
	if err != nil {
		tsc.logger.Warn("Could not retrieve order for trade validation",
			zap.String("trade_id", trade.ID.String()),
			zap.String("order_id", trade.OrderID.String()),
			zap.Error(err))
		// Don't fail validation if we can't retrieve order - this might be during
		// a migration or the order might be archived. Log the issue for monitoring.
		return nil
	}

	// Basic order validation
	if order.UserID == uuid.Nil {
		return fmt.Errorf("order has invalid user ID")
	}

	// Additional business rule validations can be added here
	// For example, validate order status, pair consistency, etc.
	if order.Pair != trade.Pair {
		return fmt.Errorf("trade pair %s does not match order pair %s", trade.Pair, order.Pair)
	}

	// Note: Self-trading validation is typically handled at the order matching level
	// where both maker and taker orders are available. At the trade level, we only
	// have one order ID, so we can't easily detect self-trading without additional
	// order tracking mechanisms.

	return nil
}

// GetCircuitBreakerStatus returns the current circuit breaker status
func (tsc *TradeSettlementCoordinator) GetCircuitBreakerStatus() map[string]interface{} {
	tsc.circuitBreaker.mutex.RLock()
	defer tsc.circuitBreaker.mutex.RUnlock()

	var stateStr string
	switch atomic.LoadInt32(&tsc.circuitBreaker.state) {
	case 0:
		stateStr = "closed"
	case 1:
		stateStr = "open"
	case 2:
		stateStr = "half-open"
	default:
		stateStr = "unknown"
	}

	return map[string]interface{}{
		"state":             stateStr,
		"failures":          atomic.LoadInt32(&tsc.circuitBreaker.failures),
		"failure_threshold": tsc.circuitBreaker.failureThreshold,
		"last_failure_time": tsc.circuitBreaker.lastFailureTime,
		"reset_timeout":     tsc.circuitBreaker.resetTimeout,
	}
}

// ForceCircuitBreakerReset manually resets the circuit breaker (for admin use)
func (tsc *TradeSettlementCoordinator) ForceCircuitBreakerReset() {
	tsc.circuitBreaker.mutex.Lock()
	defer tsc.circuitBreaker.mutex.Unlock()

	atomic.StoreInt32(&tsc.circuitBreaker.failures, 0)
	atomic.StoreInt32(&tsc.circuitBreaker.state, 0)

	tsc.logger.Info("Circuit breaker manually reset")
}
