package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

// KafkaConfig contains configuration for Kafka connection
type KafkaConfig struct {
	Brokers             []string      `json:"brokers"`
	ReadTimeout         time.Duration `json:"read_timeout"`
	WriteTimeout        time.Duration `json:"write_timeout"`
	BatchSize           int           `json:"batch_size"`
	BatchTimeout        time.Duration `json:"batch_timeout"`
	RequiredAcks        int           `json:"required_acks"`
	Compression         string        `json:"compression"`
	RetryMax            int           `json:"retry_max"`
	RetryBackoffMin     time.Duration `json:"retry_backoff_min"`
	RetryBackoffMax     time.Duration `json:"retry_backoff_max"`
	EnableIdempotent    bool          `json:"enable_idempotent"`
	ConsumerGroupPrefix string        `json:"consumer_group_prefix"`

	// High-frequency trading optimizations
	EnableBuffering     bool          `json:"enable_buffering"`
	BufferSize          int           `json:"buffer_size"`
	BufferFlushInterval time.Duration `json:"buffer_flush_interval"`
	MaxMessageBytes     int           `json:"max_message_bytes"`
}

// DefaultKafkaConfig returns default configuration optimized for high-frequency trading
func DefaultKafkaConfig() *KafkaConfig {
	return &KafkaConfig{
		Brokers:             []string{"localhost:9092"},
		ReadTimeout:         10 * time.Second,
		WriteTimeout:        1 * time.Second,
		BatchSize:           1000,
		BatchTimeout:        10 * time.Millisecond,
		RequiredAcks:        1, // Leader acknowledgment only for better performance
		Compression:         "snappy",
		RetryMax:            3,
		RetryBackoffMin:     100 * time.Millisecond,
		RetryBackoffMax:     1 * time.Second,
		EnableIdempotent:    true,
		ConsumerGroupPrefix: "pincex",
		EnableBuffering:     true,
		BufferSize:          10000,
		BufferFlushInterval: 5 * time.Millisecond,
		MaxMessageBytes:     1048576, // 1MB
	}
}

// Producer interface defines message publishing operations
type Producer interface {
	Publish(ctx context.Context, topic Topic, key string, message interface{}) error
	PublishBatch(ctx context.Context, topic Topic, messages []BatchMessage) error
	Close() error
}

// Consumer interface defines message consumption operations
type Consumer interface {
	Subscribe(ctx context.Context, topics []Topic, groupID string, handler MessageHandler) error
	Close() error
}

// MessageHandler defines the callback function for processing messages
type MessageHandler func(ctx context.Context, msg *ReceivedMessage) error

// ReceivedMessage represents a received message with metadata
type ReceivedMessage struct {
	Topic     string
	Key       string
	Value     []byte
	Headers   map[string][]byte
	Offset    int64
	Partition int
	Timestamp time.Time
}

// BatchMessage represents a message in a batch operation
type BatchMessage struct {
	Key     string
	Message interface{}
}

// KafkaProducer implements Producer interface
type KafkaProducer struct {
	config  *KafkaConfig
	writers map[Topic]*kafka.Writer
	buffer  *MessageBuffer
	logger  *zap.Logger
	mu      sync.RWMutex
}

// MessageBuffer handles buffered message publishing for high-frequency scenarios
type MessageBuffer struct {
	messages    []kafka.Message
	mu          sync.Mutex
	flushTicker *time.Ticker
	producer    *KafkaProducer
	ctx         context.Context
	cancel      context.CancelFunc
}

// NewKafkaProducer creates a new Kafka producer
func NewKafkaProducer(config *KafkaConfig, logger *zap.Logger) (*KafkaProducer, error) {
	if config == nil {
		config = DefaultKafkaConfig()
	}

	producer := &KafkaProducer{
		config:  config,
		writers: make(map[Topic]*kafka.Writer),
		logger:  logger,
	}

	if config.EnableBuffering {
		ctx, cancel := context.WithCancel(context.Background())
		buffer := &MessageBuffer{
			messages: make([]kafka.Message, 0, config.BufferSize),
			producer: producer,
			ctx:      ctx,
			cancel:   cancel,
		}
		buffer.flushTicker = time.NewTicker(config.BufferFlushInterval)
		go buffer.flushWorker()
		producer.buffer = buffer
	}

	return producer, nil
}

// getWriter returns or creates a writer for the specified topic
func (p *KafkaProducer) getWriter(topic Topic) *kafka.Writer {
	p.mu.RLock()
	writer, exists := p.writers[topic]
	p.mu.RUnlock()

	if exists {
		return writer
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check pattern
	if writer, exists := p.writers[topic]; exists {
		return writer
	}

	writer = &kafka.Writer{
		Addr:         kafka.TCP(p.config.Brokers...),
		Topic:        string(topic),
		Balancer:     &kafka.CRC32Balancer{}, // Better distribution for high-frequency
		BatchSize:    p.config.BatchSize,
		BatchTimeout: p.config.BatchTimeout,
		ReadTimeout:  p.config.ReadTimeout,
		WriteTimeout: p.config.WriteTimeout,
		RequiredAcks: kafka.RequiredAcks(p.config.RequiredAcks),
		MaxAttempts:  p.config.RetryMax,
		BatchBytes:   int64(p.config.MaxMessageBytes),
		Async:        p.config.EnableBuffering, // Use async for buffered mode
		Completion: func(messages []kafka.Message, err error) {
			if err != nil {
				p.logger.Error("Failed to publish messages", zap.Error(err), zap.Int("count", len(messages)))
			}
		},
	}

	// Set compression
	switch p.config.Compression {
	case "gzip":
		writer.Compression = kafka.Gzip
	case "snappy":
		writer.Compression = kafka.Snappy
	case "lz4":
		writer.Compression = kafka.Lz4
	case "zstd":
		writer.Compression = kafka.Zstd
	default:
		writer.Compression = kafka.Snappy
	}

	p.writers[topic] = writer
	return writer
}

// Publish publishes a single message to the specified topic
func (p *KafkaProducer) Publish(ctx context.Context, topic Topic, key string, message interface{}) error {
	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	kafkaMsg := kafka.Message{
		Key:   []byte(key),
		Value: data,
		Time:  time.Now(),
	}

	if p.config.EnableBuffering {
		return p.buffer.addMessage(kafkaMsg, topic)
	}

	writer := p.getWriter(topic)
	return writer.WriteMessages(ctx, kafkaMsg)
}

// PublishBatch publishes multiple messages in a single batch
func (p *KafkaProducer) PublishBatch(ctx context.Context, topic Topic, messages []BatchMessage) error {
	if len(messages) == 0 {
		return nil
	}

	kafkaMessages := make([]kafka.Message, len(messages))
	for i, msg := range messages {
		data, err := json.Marshal(msg.Message)
		if err != nil {
			return fmt.Errorf("failed to marshal message %d: %w", i, err)
		}

		kafkaMessages[i] = kafka.Message{
			Key:   []byte(msg.Key),
			Value: data,
			Time:  time.Now(),
		}
	}

	writer := p.getWriter(topic)
	return writer.WriteMessages(ctx, kafkaMessages...)
}

// Close closes the producer and all its writers
func (p *KafkaProducer) Close() error {
	if p.buffer != nil {
		p.buffer.cancel()
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	var lastErr error
	for _, writer := range p.writers {
		if err := writer.Close(); err != nil {
			lastErr = err
			p.logger.Error("Failed to close writer", zap.Error(err))
		}
	}

	return lastErr
}

// addMessage adds a message to the buffer
func (b *MessageBuffer) addMessage(msg kafka.Message, topic Topic) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Add topic to message headers for routing
	if msg.Headers == nil {
		msg.Headers = make([]kafka.Header, 0, 1)
	}
	msg.Headers = append(msg.Headers, kafka.Header{
		Key:   "topic",
		Value: []byte(topic),
	})

	b.messages = append(b.messages, msg)

	// Flush if buffer is full
	if len(b.messages) >= b.producer.config.BufferSize {
		return b.flush()
	}

	return nil
}

// flush sends all buffered messages
func (b *MessageBuffer) flush() error {
	if len(b.messages) == 0 {
		return nil
	}

	// Group messages by topic
	topicMessages := make(map[Topic][]kafka.Message)
	for _, msg := range b.messages {
		topic := Topic("default")
		for _, header := range msg.Headers {
			if string(header.Key) == "topic" {
				topic = Topic(header.Value)
				break
			}
		}
		topicMessages[topic] = append(topicMessages[topic], msg)
	}

	// Send messages for each topic
	for topic, messages := range topicMessages {
		writer := b.producer.getWriter(topic)
		if err := writer.WriteMessages(b.ctx, messages...); err != nil {
			b.producer.logger.Error("Failed to flush buffered messages",
				zap.Error(err),
				zap.String("topic", string(topic)),
				zap.Int("count", len(messages)))
			return err
		}
	}

	// Clear buffer
	b.messages = b.messages[:0]
	return nil
}

// flushWorker periodically flushes the buffer
func (b *MessageBuffer) flushWorker() {
	defer b.flushTicker.Stop()

	for {
		select {
		case <-b.flushTicker.C:
			b.mu.Lock()
			if err := b.flush(); err != nil {
				b.producer.logger.Error("Periodic flush failed", zap.Error(err))
			}
			b.mu.Unlock()
		case <-b.ctx.Done():
			// Final flush on shutdown
			b.mu.Lock()
			if err := b.flush(); err != nil {
				b.producer.logger.Error("Final flush failed", zap.Error(err))
			}
			b.mu.Unlock()
			return
		}
	}
}

// KafkaConsumer implements Consumer interface
type KafkaConsumer struct {
	config  *KafkaConfig
	readers map[string]*kafka.Reader
	logger  *zap.Logger
	mu      sync.RWMutex
}

// NewKafkaConsumer creates a new Kafka consumer
func NewKafkaConsumer(config *KafkaConfig, logger *zap.Logger) (*KafkaConsumer, error) {
	if config == nil {
		config = DefaultKafkaConfig()
	}

	return &KafkaConsumer{
		config:  config,
		readers: make(map[string]*kafka.Reader),
		logger:  logger,
	}, nil
}

// Subscribe subscribes to multiple topics with the specified consumer group
func (c *KafkaConsumer) Subscribe(ctx context.Context, topics []Topic, groupID string, handler MessageHandler) error {
	topicStrings := make([]string, len(topics))
	for i, topic := range topics {
		topicStrings[i] = string(topic)
	}

	fullGroupID := fmt.Sprintf("%s-%s", c.config.ConsumerGroupPrefix, groupID)
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  c.config.Brokers,
		Topic:    topicStrings[0], // kafka-go reader typically handles one topic
		GroupID:  fullGroupID,
		MaxBytes: c.config.MaxMessageBytes,
		ErrorLogger: kafka.LoggerFunc(func(msg string, args ...interface{}) {
			c.logger.Error(fmt.Sprintf(msg, args...))
		}),
	})

	c.mu.Lock()
	c.readers[fullGroupID] = reader
	c.mu.Unlock()

	// Start consuming in a goroutine
	go func() {
		defer reader.Close()

		for {
			select {
			case <-ctx.Done():
				return
			default:
				msg, err := reader.ReadMessage(ctx)
				if err != nil {
					if err == context.Canceled {
						return
					}
					c.logger.Error("Failed to read message", zap.Error(err))
					continue
				}

				receivedMsg := &ReceivedMessage{
					Topic:     msg.Topic,
					Key:       string(msg.Key),
					Value:     msg.Value,
					Headers:   make(map[string][]byte),
					Offset:    msg.Offset,
					Partition: msg.Partition,
					Timestamp: msg.Time,
				}

				// Convert headers
				for _, header := range msg.Headers {
					receivedMsg.Headers[header.Key] = header.Value
				}

				if err := handler(ctx, receivedMsg); err != nil {
					c.logger.Error("Message handler failed",
						zap.Error(err),
						zap.String("topic", msg.Topic),
						zap.Int64("offset", msg.Offset))
				}
			}
		}
	}()

	return nil
}

// Close closes all consumer readers
func (c *KafkaConsumer) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var lastErr error
	for groupID, reader := range c.readers {
		if err := reader.Close(); err != nil {
			lastErr = err
			c.logger.Error("Failed to close reader", zap.Error(err), zap.String("group", groupID))
		}
	}

	return lastErr
}
