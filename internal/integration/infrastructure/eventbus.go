// Package infrastructure provides core infrastructure components for service integration
package infrastructure

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// EventBus provides event-driven communication between modules
type EventBus interface {
	// Publishing
	Publish(ctx context.Context, event Event) error
	PublishAsync(ctx context.Context, event Event) error

	// Subscription management
	Subscribe(eventType string, handler EventHandler) error
	Unsubscribe(eventType string, handlerID string) error

	// Lifecycle
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	HealthCheck(ctx context.Context) error
}

// Event represents a domain event in the system
type Event struct {
	ID            uuid.UUID              `json:"id"`
	Type          string                 `json:"type"`
	Source        string                 `json:"source"`
	Data          json.RawMessage        `json:"data"`
	Metadata      map[string]interface{} `json:"metadata"`
	Timestamp     time.Time              `json:"timestamp"`
	CorrelationID uuid.UUID              `json:"correlation_id"`
	Version       string                 `json:"version"`
}

// EventHandler processes events
type EventHandler interface {
	Handle(ctx context.Context, event Event) error
	GetID() string
	GetEventTypes() []string
}

// EventHandlerFunc implements EventHandler for functions
type EventHandlerFunc struct {
	id         string
	eventTypes []string
	handler    func(ctx context.Context, event Event) error
}

func (h *EventHandlerFunc) Handle(ctx context.Context, event Event) error {
	return h.handler(ctx, event)
}

func (h *EventHandlerFunc) GetID() string {
	return h.id
}

func (h *EventHandlerFunc) GetEventTypes() []string {
	return h.eventTypes
}

// NewEventHandlerFunc creates a new function-based event handler
func NewEventHandlerFunc(id string, eventTypes []string, handler func(ctx context.Context, event Event) error) EventHandler {
	return &EventHandlerFunc{
		id:         id,
		eventTypes: eventTypes,
		handler:    handler,
	}
}

// InMemoryEventBus provides an in-memory event bus implementation
type InMemoryEventBus struct {
	logger      *zap.Logger
	handlers    map[string][]EventHandler
	asyncBuffer chan Event
	bufferSize  int
	running     bool
	mu          sync.RWMutex
	wg          sync.WaitGroup
}

// NewInMemoryEventBus creates a new in-memory event bus
func NewInMemoryEventBus(logger *zap.Logger, bufferSize int) EventBus {
	return &InMemoryEventBus{
		logger:      logger,
		handlers:    make(map[string][]EventHandler),
		asyncBuffer: make(chan Event, bufferSize),
		bufferSize:  bufferSize,
	}
}

func (eb *InMemoryEventBus) Start(ctx context.Context) error {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	if eb.running {
		return nil
	}

	eb.running = true

	// Start async event processor
	eb.wg.Add(1)
	go eb.processAsyncEvents(ctx)

	eb.logger.Info("EventBus started")
	return nil
}

func (eb *InMemoryEventBus) Stop(ctx context.Context) error {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	if !eb.running {
		return nil
	}

	eb.running = false
	close(eb.asyncBuffer)
	eb.wg.Wait()

	eb.logger.Info("EventBus stopped")
	return nil
}

func (eb *InMemoryEventBus) Publish(ctx context.Context, event Event) error {
	eb.mu.RLock()
	handlers := eb.handlers[event.Type]
	eb.mu.RUnlock()

	if len(handlers) == 0 {
		eb.logger.Debug("No handlers for event type", zap.String("type", event.Type))
		return nil
	}

	// Process handlers synchronously
	for _, handler := range handlers {
		if err := handler.Handle(ctx, event); err != nil {
			eb.logger.Error("Handler failed to process event",
				zap.String("event_type", event.Type),
				zap.String("handler_id", handler.GetID()),
				zap.Error(err))
			// Continue processing other handlers
		}
	}

	return nil
}

func (eb *InMemoryEventBus) PublishAsync(ctx context.Context, event Event) error {
	eb.mu.RLock()
	running := eb.running
	eb.mu.RUnlock()

	if !running {
		return fmt.Errorf("event bus not running")
	}

	select {
	case eb.asyncBuffer <- event:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		return fmt.Errorf("async buffer full")
	}
}

func (eb *InMemoryEventBus) Subscribe(eventType string, handler EventHandler) error {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	if eb.handlers[eventType] == nil {
		eb.handlers[eventType] = make([]EventHandler, 0)
	}

	eb.handlers[eventType] = append(eb.handlers[eventType], handler)

	eb.logger.Info("Handler subscribed to event type",
		zap.String("event_type", eventType),
		zap.String("handler_id", handler.GetID()))

	return nil
}

func (eb *InMemoryEventBus) Unsubscribe(eventType string, handlerID string) error {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	handlers := eb.handlers[eventType]
	if handlers == nil {
		return fmt.Errorf("no handlers for event type: %s", eventType)
	}

	for i, handler := range handlers {
		if handler.GetID() == handlerID {
			eb.handlers[eventType] = append(handlers[:i], handlers[i+1:]...)
			eb.logger.Info("Handler unsubscribed from event type",
				zap.String("event_type", eventType),
				zap.String("handler_id", handlerID))
			return nil
		}
	}

	return fmt.Errorf("handler not found: %s", handlerID)
}

func (eb *InMemoryEventBus) HealthCheck(ctx context.Context) error {
	eb.mu.RLock()
	running := eb.running
	eb.mu.RUnlock()

	if !running {
		return fmt.Errorf("event bus not running")
	}

	return nil
}

func (eb *InMemoryEventBus) processAsyncEvents(ctx context.Context) {
	defer eb.wg.Done()

	for {
		select {
		case event, ok := <-eb.asyncBuffer:
			if !ok {
				return // Channel closed
			}

			if err := eb.Publish(ctx, event); err != nil {
				eb.logger.Error("Failed to process async event",
					zap.String("event_id", event.ID.String()),
					zap.String("event_type", event.Type),
					zap.Error(err))
			}

		case <-ctx.Done():
			return
		}
	}
}

// Event type constants for cross-module coordination
const (
	// User events
	EventTypeUserCreated    = "user.created"
	EventTypeUserUpdated    = "user.updated"
	EventTypeUserDeleted    = "user.deleted"
	EventTypeKYCUpdated     = "user.kyc_updated"
	EventTypeSessionCreated = "user.session_created"
	EventTypeSessionExpired = "user.session_expired"

	// Account events
	EventTypeBalanceUpdated    = "account.balance_updated"
	EventTypeBalanceReserved   = "account.balance_reserved"
	EventTypeBalanceReleased   = "account.balance_released"
	EventTypeTransferCompleted = "account.transfer_completed"
	EventTypeTransferFailed    = "account.transfer_failed"

	// Trading events
	EventTypeOrderPlaced     = "trading.order_placed"
	EventTypeOrderCancelled  = "trading.order_cancelled"
	EventTypeOrderFilled     = "trading.order_filled"
	EventTypeTradeExecuted   = "trading.trade_executed"
	EventTypePositionChanged = "trading.position_changed"

	// Fiat events
	EventTypeFiatDepositReceived     = "fiat.deposit_received"
	EventTypeFiatDepositConfirmed    = "fiat.deposit_confirmed"
	EventTypeFiatWithdrawalRequested = "fiat.withdrawal_requested"
	EventTypeFiatWithdrawalCompleted = "fiat.withdrawal_completed"
	EventTypeFiatKYCRequired         = "fiat.kyc_required"

	// System events
	EventTypeReconciliationRequired = "system.reconciliation_required"
	EventTypeHealthCheckFailed      = "system.health_check_failed"
	EventTypePerformanceAlert       = "system.performance_alert"
)

// Helper functions for creating common events
func NewUserCreatedEvent(userID uuid.UUID, correlationID uuid.UUID, userData interface{}) Event {
	data, _ := json.Marshal(userData)
	return Event{
		ID:            uuid.New(),
		Type:          EventTypeUserCreated,
		Source:        "userauth",
		Data:          data,
		Timestamp:     time.Now(),
		CorrelationID: correlationID,
		Version:       "1.0",
		Metadata: map[string]interface{}{
			"user_id": userID.String(),
		},
	}
}

func NewBalanceUpdatedEvent(userID uuid.UUID, currency string, correlationID uuid.UUID, balanceData interface{}) Event {
	data, _ := json.Marshal(balanceData)
	return Event{
		ID:            uuid.New(),
		Type:          EventTypeBalanceUpdated,
		Source:        "accounts",
		Data:          data,
		Timestamp:     time.Now(),
		CorrelationID: correlationID,
		Version:       "1.0",
		Metadata: map[string]interface{}{
			"user_id":  userID.String(),
			"currency": currency,
		},
	}
}

func NewTradeExecutedEvent(userID uuid.UUID, correlationID uuid.UUID, tradeData interface{}) Event {
	data, _ := json.Marshal(tradeData)
	return Event{
		ID:            uuid.New(),
		Type:          EventTypeTradeExecuted,
		Source:        "trading",
		Data:          data,
		Timestamp:     time.Now(),
		CorrelationID: correlationID,
		Version:       "1.0",
		Metadata: map[string]interface{}{
			"user_id": userID.String(),
		},
	}
}

func NewFiatDepositEvent(userID uuid.UUID, correlationID uuid.UUID, depositData interface{}) Event {
	data, _ := json.Marshal(depositData)
	return Event{
		ID:            uuid.New(),
		Type:          EventTypeFiatDepositReceived,
		Source:        "fiat",
		Data:          data,
		Timestamp:     time.Now(),
		CorrelationID: correlationID,
		Version:       "1.0",
		Metadata: map[string]interface{}{
			"user_id": userID.String(),
		},
	}
}
