package messaging

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

// KafkaClient provides Kafka publishing functionality for trading events.
// This implementation is optimized for high-frequency trading scenarios.
type KafkaClient struct {
	brokers []string
	topic   string
	group   string
	writer  *kafka.Writer
	logger  *zap.Logger
	mu      sync.RWMutex
	closed  bool
}

// KafkaClientConfig contains configuration options for KafkaClient
type KafkaClientConfig struct {
	BatchSize        int
	BatchTimeout     time.Duration
	WriteTimeout     time.Duration
	RequiredAcks     int
	Compression      string
	MaxMessageBytes  int
	EnableIdempotent bool
	RetryMax         int
}

// DefaultKafkaClientConfig returns optimized configuration for trading events
func DefaultKafkaClientConfig() *KafkaClientConfig {
	return &KafkaClientConfig{
		BatchSize:        100, // Smaller batches for lower latency
		BatchTimeout:     5 * time.Millisecond,
		WriteTimeout:     1 * time.Second,
		RequiredAcks:     1,        // Leader ack only for performance
		Compression:      "snappy", // Fast compression
		MaxMessageBytes:  1048576,  // 1MB
		EnableIdempotent: true,
		RetryMax:         3,
	}
}

// NewKafkaClient creates a new Kafka client with the specified configuration.
func NewKafkaClient(brokers []string, topic, group string) *KafkaClient {
	return NewKafkaClientWithConfig(brokers, topic, group, DefaultKafkaClientConfig())
}

// NewKafkaClientWithConfig creates a new Kafka client with custom configuration.
func NewKafkaClientWithConfig(brokers []string, topic, group string, config *KafkaClientConfig) *KafkaClient {
	if config == nil {
		config = DefaultKafkaClientConfig()
	}

	// Create logger - in production this should be injected
	logger, _ := zap.NewProduction()

	client := &KafkaClient{
		brokers: brokers,
		topic:   topic,
		group:   group,
		logger:  logger,
	}

	// Initialize writer with optimized settings for trading
	client.writer = &kafka.Writer{
		Addr:         kafka.TCP(brokers...),
		Topic:        topic,
		Balancer:     &kafka.CRC32Balancer{}, // Better distribution
		BatchSize:    config.BatchSize,
		BatchTimeout: config.BatchTimeout,
		WriteTimeout: config.WriteTimeout,
		RequiredAcks: kafka.RequiredAcks(config.RequiredAcks),
		MaxAttempts:  config.RetryMax,
		BatchBytes:   int64(config.MaxMessageBytes),
		Async:        false, // Synchronous for trading events
	}

	// Set compression algorithm
	switch config.Compression {
	case "gzip":
		client.writer.Compression = kafka.Gzip
	case "snappy":
		client.writer.Compression = kafka.Snappy
	case "lz4":
		client.writer.Compression = kafka.Lz4
	case "zstd":
		client.writer.Compression = kafka.Zstd
	default:
		client.writer.Compression = kafka.Snappy
	}

	return client
}

// PublishEvent publishes an event to Kafka with the specified key and data.
// This method includes proper error handling and retries for trading reliability.
func (c *KafkaClient) PublishEvent(ctx context.Context, key string, data []byte) error {
	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		return fmt.Errorf("kafka client is closed")
	}
	writer := c.writer
	c.mu.RUnlock()

	if writer == nil {
		return fmt.Errorf("kafka writer not initialized")
	}

	// Create message with headers for better observability
	message := kafka.Message{
		Key:   []byte(key),
		Value: data,
		Headers: []kafka.Header{
			{
				Key:   "source",
				Value: []byte("trading-engine"),
			},
			{
				Key:   "timestamp",
				Value: []byte(time.Now().UTC().Format(time.RFC3339Nano)),
			},
			{
				Key:   "group",
				Value: []byte(c.group),
			},
		},
		Time: time.Now(),
	}

	// Publish with context timeout
	err := writer.WriteMessages(ctx, message)
	if err != nil {
		c.logger.Error("Failed to publish trading event to Kafka",
			zap.String("topic", c.topic),
			zap.String("key", key),
			zap.String("group", c.group),
			zap.Error(err),
		)
		return fmt.Errorf("failed to publish event to kafka topic %s: %w", c.topic, err)
	}

	c.logger.Debug("Successfully published trading event",
		zap.String("topic", c.topic),
		zap.String("key", key),
		zap.Int("data_size", len(data)),
	)

	return nil
}

// PublishEventWithHeaders publishes an event with custom headers
func (c *KafkaClient) PublishEventWithHeaders(ctx context.Context, key string, data []byte, headers map[string]string) error {
	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		return fmt.Errorf("kafka client is closed")
	}
	writer := c.writer
	c.mu.RUnlock()

	if writer == nil {
		return fmt.Errorf("kafka writer not initialized")
	}

	// Convert headers to Kafka format
	kafkaHeaders := []kafka.Header{
		{Key: "source", Value: []byte("trading-engine")},
		{Key: "timestamp", Value: []byte(time.Now().UTC().Format(time.RFC3339Nano))},
		{Key: "group", Value: []byte(c.group)},
	}

	for k, v := range headers {
		kafkaHeaders = append(kafkaHeaders, kafka.Header{
			Key:   k,
			Value: []byte(v),
		})
	}

	message := kafka.Message{
		Key:     []byte(key),
		Value:   data,
		Headers: kafkaHeaders,
		Time:    time.Now(),
	}

	err := writer.WriteMessages(ctx, message)
	if err != nil {
		c.logger.Error("Failed to publish trading event with headers to Kafka",
			zap.String("topic", c.topic),
			zap.String("key", key),
			zap.Any("headers", headers),
			zap.Error(err),
		)
		return fmt.Errorf("failed to publish event with headers to kafka topic %s: %w", c.topic, err)
	}

	return nil
}

// Close closes the Kafka client and releases resources.
func (c *KafkaClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}

	c.closed = true

	if c.writer != nil {
		err := c.writer.Close()
		if err != nil {
			c.logger.Error("Error closing Kafka writer", zap.Error(err))
			return fmt.Errorf("failed to close kafka writer: %w", err)
		}
	}

	if c.logger != nil {
		_ = c.logger.Sync()
	}

	return nil
}

// Topic returns the topic this client publishes to
func (c *KafkaClient) Topic() string {
	return c.topic
}

// Group returns the consumer group identifier
func (c *KafkaClient) Group() string {
	return c.group
}

// IsHealthy performs a basic health check of the Kafka connection
func (c *KafkaClient) IsHealthy(ctx context.Context) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.closed {
		return fmt.Errorf("kafka client is closed")
	}

	if c.writer == nil {
		return fmt.Errorf("kafka writer not initialized")
	}

	// Try to get topic metadata as health check
	conn, err := kafka.DialContext(ctx, "tcp", c.brokers[0])
	if err != nil {
		return fmt.Errorf("failed to connect to kafka broker: %w", err)
	}
	defer conn.Close()

	_, err = conn.ReadPartitions(c.topic)
	if err != nil {
		return fmt.Errorf("failed to read topic partitions: %w", err)
	}

	return nil
}
