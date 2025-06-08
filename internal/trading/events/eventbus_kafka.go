package events

// This file provides a placeholder for a Kafka-backed event bus implementation.
// Only the interface and stub are provided to comply with the architecture policy.
// Actual Kafka integration should be implemented in infra/integration if needed.

import (
	"context"

	"go.uber.org/zap"
)

type KafkaEventBus struct {
	logger *zap.Logger
	// Add Kafka producer/consumer fields here
}

func NewKafkaEventBus(logger *zap.Logger) *KafkaEventBus {
	return &KafkaEventBus{logger: logger}
}

func (bus *KafkaEventBus) Publish(ctx context.Context, event Event) {
	// TODO: Implement Kafka publish logic
	bus.logger.Info("KafkaEventBus.Publish called (not implemented)", zap.String("topic", event.Topic), zap.String("type", event.Type))
}

func (bus *KafkaEventBus) Subscribe(topic string, handler EventHandler) {
	// TODO: Implement Kafka subscribe logic
	bus.logger.Info("KafkaEventBus.Subscribe called (not implemented)", zap.String("topic", topic))
}
