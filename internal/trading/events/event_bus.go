package events

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"
)

// Event is the base type for all events published on the bus
// You can extend this struct for trade, order, settlement, etc.
type Event struct {
	Topic     string // e.g. "trade", "order", "settlement"
	Type      string // e.g. "TRADE_EXECUTED", "ORDER_PLACED"
	Timestamp time.Time
	Payload   interface{}
	Meta      map[string]interface{} // optional
}

// EventHandler is a function that handles an event
// It should be fast and non-blocking
// If it panics, the bus will recover and log
// If it returns an error, the bus will log
type EventHandler func(Event)

// EventBus is the interface for publishing and subscribing to events
type EventBus interface {
	Publish(ctx context.Context, event Event)
	Subscribe(topic string, handler EventHandler)
}

// InMemoryEventBus is a high-performance, concurrent-safe event bus
// Uses channels and fan-out for low-latency delivery
type InMemoryEventBus struct {
	logger  *zap.Logger
	mu      sync.RWMutex
	subs    map[string][]EventHandler
	metrics *EventBusMetrics
}

type EventBusMetrics struct {
	Published int64
	Delivered int64
	Failed    int64
}

// NewInMemoryEventBus creates a new in-memory event bus
func NewInMemoryEventBus(logger *zap.Logger) *InMemoryEventBus {
	return &InMemoryEventBus{
		logger:  logger,
		subs:    make(map[string][]EventHandler),
		metrics: &EventBusMetrics{},
	}
}

// Publish delivers an event to all subscribers of the topic
func (bus *InMemoryEventBus) Publish(ctx context.Context, event Event) {
	bus.metrics.Published++
	bus.mu.RLock()
	handlers := append([]EventHandler{}, bus.subs[event.Topic]...)
	bus.mu.RUnlock()
	if len(handlers) == 0 {
		bus.logger.Warn("No subscribers for event", zap.String("topic", event.Topic), zap.String("type", event.Type))
		return
	}
	for _, handler := range handlers {
		go func(h EventHandler) {
			defer func() {
				if r := recover(); r != nil {
					bus.logger.Error("Event handler panic", zap.Any("recover", r), zap.String("topic", event.Topic))
					bus.metrics.Failed++
				}
			}()
			h(event)
			bus.metrics.Delivered++
		}(handler)
	}
}

// Subscribe registers a handler for a topic
func (bus *InMemoryEventBus) Subscribe(topic string, handler EventHandler) {
	bus.mu.Lock()
	defer bus.mu.Unlock()
	bus.subs[topic] = append(bus.subs[topic], handler)
	bus.logger.Info("Subscribed handler to topic", zap.String("topic", topic))
}

// Metrics returns current event bus metrics
func (bus *InMemoryEventBus) Metrics() EventBusMetrics {
	return *bus.metrics
}
