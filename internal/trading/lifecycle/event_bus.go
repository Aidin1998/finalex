package lifecycle

import (
	"context"
	"encoding/json"

	"go.uber.org/zap"
)

// EventBus defines the interface for publishing order events
type EventBus interface {
	PublishOrderEvent(ctx context.Context, event *OrderEvent) error
	PublishStateTransition(ctx context.Context, transition *OrderStateTransition) error
}

// SimpleEventBus provides a basic implementation of EventBus
// In production, this would integrate with Kafka, Redis, or other messaging systems
type SimpleEventBus struct {
	logger *zap.Logger
}

// NewSimpleEventBus creates a new simple event bus
func NewSimpleEventBus(logger *zap.Logger) EventBus {
	return &SimpleEventBus{
		logger: logger,
	}
}

// PublishOrderEvent publishes an order event
func (seb *SimpleEventBus) PublishOrderEvent(ctx context.Context, event *OrderEvent) error {
	eventData, err := json.Marshal(event)
	if err != nil {
		seb.logger.Error("Failed to marshal order event", zap.Error(err))
		return err
	}

	seb.logger.Info("Order event published",
		zap.String("event_id", event.ID),
		zap.String("event_type", event.EventType),
		zap.String("order_id", event.OrderID),
		zap.String("user_id", event.UserID),
		zap.String("trace_id", event.TraceID),
	)

	// TODO: In production, send to Kafka, Redis Streams, or other messaging system
	seb.logger.Debug("Order event payload", zap.String("payload", string(eventData)))

	return nil
}

// PublishStateTransition publishes a state transition event
func (seb *SimpleEventBus) PublishStateTransition(ctx context.Context, transition *OrderStateTransition) error {
	transitionData, err := json.Marshal(transition)
	if err != nil {
		seb.logger.Error("Failed to marshal state transition", zap.Error(err))
		return err
	}

	seb.logger.Info("State transition published",
		zap.String("transition_id", transition.ID),
		zap.String("order_id", transition.OrderID),
		zap.String("from_state", transition.FromState),
		zap.String("to_state", transition.ToState),
		zap.String("reason", transition.Reason),
	)

	// TODO: In production, send to Kafka, Redis Streams, or other messaging system
	seb.logger.Debug("State transition payload", zap.String("payload", string(transitionData)))

	return nil
}
