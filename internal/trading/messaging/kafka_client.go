package messaging

import "context"

// KafkaClient is a stub for publishing events to Kafka.
type KafkaClient struct{}

// NewKafkaClient creates a new stub Kafka client.
func NewKafkaClient(brokers []string, topic, group string) *KafkaClient {
	return &KafkaClient{}
}

// PublishEvent is a stub method for publishing an event.
func (c *KafkaClient) PublishEvent(ctx context.Context, key string, data []byte) error {
	// No-op or logging
	return nil
}
