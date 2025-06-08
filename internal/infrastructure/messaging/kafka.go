package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
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

	// Circuit breaker configuration
	CircuitBreaker EnhancedCircuitBreakerConfig `json:"circuit_breaker"`
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
		CircuitBreaker:      DefaultEnhancedCircuitBreakerConfig(),
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
	config         *KafkaConfig
	writers        map[Topic]*kafka.Writer
	buffer         *MessageBuffer
	logger         *zap.Logger
	circuitBreaker *MessagingCircuitBreaker
	metrics        *ProducerMetrics
	mu             sync.RWMutex
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
		config:         config,
		writers:        make(map[Topic]*kafka.Writer),
		logger:         logger,
		circuitBreaker: NewMessagingCircuitBreaker(config.CircuitBreaker, logger),
		metrics: &ProducerMetrics{
			ConnectionsActive: 0,
		},
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
	// Check circuit breaker
	if !p.circuitBreaker.AllowRequest() {
		atomic.AddInt64(&p.metrics.MessagesFailedToSend, 1)
		return fmt.Errorf("circuit breaker is open, rejecting publish request")
	}

	startTime := time.Now()

	data, err := json.Marshal(message)
	if err != nil {
		p.circuitBreaker.RecordFailure()
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	kafkaMsg := kafka.Message{
		Key:   []byte(key),
		Value: data,
		Time:  time.Now(),
	}

	var publishErr error
	if p.config.EnableBuffering {
		publishErr = p.buffer.addMessage(kafkaMsg, topic)
	} else {
		writer := p.getWriter(topic)
		publishErr = writer.WriteMessages(ctx, kafkaMsg)
	}

	// Record result with circuit breaker
	if publishErr != nil {
		p.circuitBreaker.RecordFailure()
		atomic.AddInt64(&p.metrics.MessagesFailedToSend, 1)
		p.logger.Error("Failed to publish message",
			zap.Error(publishErr),
			zap.String("topic", string(topic)),
			zap.String("key", key))
		return publishErr
	}

	// Record success
	responseTime := time.Since(startTime)
	p.circuitBreaker.RecordSuccess(responseTime)
	atomic.AddInt64(&p.metrics.MessagesPublished, 1)
	atomic.AddInt64(&p.metrics.MessagesSent, 1)

	return nil
}

// PublishBatch publishes multiple messages in a single batch
func (p *KafkaProducer) PublishBatch(ctx context.Context, topic Topic, messages []BatchMessage) error {
	if len(messages) == 0 {
		return nil
	}

	// Check circuit breaker
	if !p.circuitBreaker.AllowRequest() {
		atomic.AddInt64(&p.metrics.MessagesFailedToSend, int64(len(messages)))
		return fmt.Errorf("circuit breaker is open, rejecting batch publish request")
	}

	startTime := time.Now()

	kafkaMessages := make([]kafka.Message, len(messages))
	for i, msg := range messages {
		data, err := json.Marshal(msg.Message)
		if err != nil {
			p.circuitBreaker.RecordFailure()
			return fmt.Errorf("failed to marshal message %d: %w", i, err)
		}

		kafkaMessages[i] = kafka.Message{
			Key:   []byte(msg.Key),
			Value: data,
			Time:  time.Now(),
		}
	}

	writer := p.getWriter(topic)
	err := writer.WriteMessages(ctx, kafkaMessages...)

	// Record result with circuit breaker
	if err != nil {
		p.circuitBreaker.RecordFailure()
		atomic.AddInt64(&p.metrics.MessagesFailedToSend, int64(len(messages)))
		atomic.AddInt64(&p.metrics.BatchesSent, 1) // Still count as attempted batch
		p.logger.Error("Failed to publish batch",
			zap.Error(err),
			zap.String("topic", string(topic)),
			zap.Int("message_count", len(messages)))
		return err
	}

	// Record success
	responseTime := time.Since(startTime)
	p.circuitBreaker.RecordSuccess(responseTime)
	atomic.AddInt64(&p.metrics.MessagesPublished, int64(len(messages)))
	atomic.AddInt64(&p.metrics.MessagesSent, int64(len(messages)))
	atomic.AddInt64(&p.metrics.BatchesSent, 1)

	return nil
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

// GetMetrics returns current producer metrics
func (p *KafkaProducer) GetMetrics() *ProducerMetrics {
	return p.metrics
}

// GetCircuitBreakerState returns the current circuit breaker state
func (p *KafkaProducer) GetCircuitBreakerState() CircuitBreakerState {
	return p.circuitBreaker.GetState()
}

// IsHealthy returns true if the producer is healthy (circuit breaker is closed)
func (p *KafkaProducer) IsHealthy() bool {
	return p.circuitBreaker.GetState() == CircuitBreakerClosed
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

	// Check circuit breaker before flushing
	if !b.producer.circuitBreaker.AllowRequest() {
		b.producer.logger.Warn("Circuit breaker is open, skipping buffer flush")
		return fmt.Errorf("circuit breaker is open, buffer flush skipped")
	}

	startTime := time.Now()

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
	var totalMessages int64 = 0
	for topic, messages := range topicMessages {
		writer := b.producer.getWriter(topic)
		if err := writer.WriteMessages(b.ctx, messages...); err != nil {
			// Record failure with circuit breaker
			b.producer.circuitBreaker.RecordFailure()
			atomic.AddInt64(&b.producer.metrics.MessagesFailedToSend, int64(len(messages)))

			b.producer.logger.Error("Failed to flush buffered messages",
				zap.Error(err),
				zap.String("topic", string(topic)),
				zap.Int("count", len(messages)))
			return err
		}
		totalMessages += int64(len(messages))
	}

	// Record success with circuit breaker
	flushTime := time.Since(startTime)
	b.producer.circuitBreaker.RecordSuccess(flushTime)
	atomic.AddInt64(&b.producer.metrics.MessagesSent, totalMessages)
	atomic.AddInt64(&b.producer.metrics.BatchesSent, 1)

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
	config         *KafkaConfig
	readers        map[string]*kafka.Reader
	logger         *zap.Logger
	circuitBreaker *MessagingCircuitBreaker
	metrics        *ConsumerMetrics
	mu             sync.RWMutex
}

// NewKafkaConsumer creates a new Kafka consumer
func NewKafkaConsumer(config *KafkaConfig, logger *zap.Logger) (*KafkaConsumer, error) {
	if config == nil {
		config = DefaultKafkaConfig()
	}

	return &KafkaConsumer{
		config:         config,
		readers:        make(map[string]*kafka.Reader),
		logger:         logger,
		circuitBreaker: NewMessagingCircuitBreaker(config.CircuitBreaker, logger),
		metrics: &ConsumerMetrics{
			ActiveConsumers: 0,
		},
	}, nil
}

// Subscribe subscribes to multiple topics with the specified consumer group
func (c *KafkaConsumer) Subscribe(ctx context.Context, topics []Topic, groupID string, handler MessageHandler) error {
	topicStrings := make([]string, len(topics))
	for i, topic := range topics {
		topicStrings[i] = string(topic)
	}

	fullGroupID := fmt.Sprintf("%s-%s", c.config.ConsumerGroupPrefix, groupID)

	c.logger.Info("Starting Kafka consumer subscription",
		zap.Strings("topics", topicStrings),
		zap.String("group_id", fullGroupID))
	// Create a reader for each topic to handle multiple topics properly
	for _, topicStr := range topicStrings {
		reader := kafka.NewReader(kafka.ReaderConfig{
			Brokers:     c.config.Brokers,
			Topic:       topicStr,
			GroupID:     fullGroupID,
			MaxBytes:    c.config.MaxMessageBytes,
			StartOffset: kafka.LastOffset, // Start from latest for new consumers
			ErrorLogger: kafka.LoggerFunc(func(msg string, args ...interface{}) {
				c.logger.Error(fmt.Sprintf(msg, args...))
			}),
		})

		readerKey := fmt.Sprintf("%s-%s", fullGroupID, topicStr)

		c.mu.Lock()
		c.readers[readerKey] = reader
		c.mu.Unlock()

		// Start consuming in a goroutine for each topic
		go c.consumeFromTopic(ctx, reader, topicStr, handler)
	}

	return nil
}

// consumeFromTopic handles message consumption from a single topic
func (c *KafkaConsumer) consumeFromTopic(ctx context.Context, reader *kafka.Reader, topic string, handler MessageHandler) {
	defer reader.Close()

	c.logger.Info("Started consuming from topic", zap.String("topic", topic))
	atomic.AddInt32(&c.metrics.ActiveConsumers, 1)
	defer atomic.AddInt32(&c.metrics.ActiveConsumers, -1)

	for {
		select {
		case <-ctx.Done():
			c.logger.Info("Consumer context cancelled", zap.String("topic", topic))
			return
		default:
			// Check circuit breaker before processing
			if !c.circuitBreaker.AllowRequest() {
				c.logger.Warn("Circuit breaker is open, pausing consumption", zap.String("topic", topic))
				time.Sleep(time.Second) // Back off when circuit breaker is open
				continue
			}

			startTime := time.Now()
			msg, err := reader.ReadMessage(ctx)
			if err != nil {
				if err == context.Canceled {
					return
				}
				c.circuitBreaker.RecordFailure()
				c.logger.Error("Failed to read message",
					zap.Error(err),
					zap.String("topic", topic))

				// Add exponential backoff for retries
				time.Sleep(time.Second)
				continue
			}

			atomic.AddInt64(&c.metrics.MessagesConsumed, 1)

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

			// Handle message with error recovery
			if err := c.handleMessageWithRetry(ctx, receivedMsg, handler); err != nil {
				c.circuitBreaker.RecordFailure()
				atomic.AddInt64(&c.metrics.MessagesFailedToProcess, 1)
				c.logger.Error("Message handler failed after retries",
					zap.Error(err),
					zap.String("topic", msg.Topic),
					zap.String("key", string(msg.Key)),
					zap.Int64("offset", msg.Offset))
			} else {
				// Record successful processing
				processingTime := time.Since(startTime)
				c.circuitBreaker.RecordSuccess(processingTime)
				atomic.AddInt64(&c.metrics.MessagesProcessed, 1)

				// Update average processing time
				c.updateAverageProcessingTime(processingTime)
			}
		}
	}
}

// handleMessageWithRetry handles message processing with retry logic
func (c *KafkaConsumer) handleMessageWithRetry(ctx context.Context, msg *ReceivedMessage, handler MessageHandler) error {
	const maxRetries = 3

	for attempt := 1; attempt <= maxRetries; attempt++ {
		err := handler(ctx, msg)
		if err == nil {
			return nil
		}

		if attempt < maxRetries {
			backoff := time.Duration(attempt) * time.Second
			c.logger.Warn("Message handler failed, retrying",
				zap.Error(err),
				zap.Int("attempt", attempt),
				zap.Duration("backoff", backoff),
				zap.String("topic", msg.Topic),
				zap.String("key", msg.Key))

			time.Sleep(backoff)
		}
	}

	// TODO: Send to dead letter queue
	return fmt.Errorf("message processing failed after %d attempts", maxRetries)
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

// updateAverageProcessingTime updates the average processing time for the consumer
func (c *KafkaConsumer) updateAverageProcessingTime(processingTime time.Duration) {
	const alpha = 0.1 // Smoothing factor for exponential moving average

	if c.metrics.AverageProcessingTime == 0 {
		c.metrics.AverageProcessingTime = processingTime
	} else {
		c.metrics.AverageProcessingTime = time.Duration(
			float64(c.metrics.AverageProcessingTime)*(1-alpha) + float64(processingTime)*alpha)
	}
}

// GetMetrics returns current consumer metrics
func (c *KafkaConsumer) GetMetrics() *ConsumerMetrics {
	return c.metrics
}

// GetCircuitBreakerState returns the current circuit breaker state
func (c *KafkaConsumer) GetCircuitBreakerState() CircuitBreakerState {
	return c.circuitBreaker.GetState()
}

// IsHealthy returns true if the consumer is healthy (circuit breaker is closed)
func (c *KafkaConsumer) IsHealthy() bool {
	return c.circuitBreaker.GetState() == CircuitBreakerClosed
}

// KafkaAdminClient handles topic management operations
type KafkaAdminClient struct {
	conn   *kafka.Conn
	config *KafkaConfig
	logger *zap.Logger
}

// NewKafkaAdminClient creates a new Kafka admin client
func NewKafkaAdminClient(config *KafkaConfig, logger *zap.Logger) (*KafkaAdminClient, error) {
	if config == nil {
		config = DefaultKafkaConfig()
	}

	// Connect to any broker for admin operations
	conn, err := kafka.Dial("tcp", config.Brokers[0])
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Kafka broker: %w", err)
	}

	return &KafkaAdminClient{
		conn:   conn,
		config: config,
		logger: logger,
	}, nil
}

// CreateTopic creates a single topic with the specified configuration
func (a *KafkaAdminClient) CreateTopic(ctx context.Context, topic Topic, config TopicConfig) error {
	topicConfigs := []kafka.TopicConfig{
		{
			Topic:             string(topic),
			NumPartitions:     config.Partitions,
			ReplicationFactor: config.ReplicationFactor,
			ConfigEntries: []kafka.ConfigEntry{
				{ConfigName: "retention.ms", ConfigValue: fmt.Sprintf("%d", config.RetentionMs)},
				{ConfigName: "cleanup.policy", ConfigValue: config.CleanupPolicy},
				{ConfigName: "compression.type", ConfigValue: config.CompressionType},
				{ConfigName: "max.message.bytes", ConfigValue: fmt.Sprintf("%d", config.MaxMessageBytes)},
			},
		},
	}

	a.logger.Info("Creating Kafka topic",
		zap.String("topic", string(topic)),
		zap.Int("partitions", config.Partitions),
		zap.Int("replication_factor", config.ReplicationFactor))

	err := a.conn.CreateTopics(topicConfigs...)
	if err != nil {
		a.logger.Error("Failed to create topic",
			zap.String("topic", string(topic)),
			zap.Error(err))
		return fmt.Errorf("failed to create topic %s: %w", topic, err)
	}

	a.logger.Info("Successfully created topic", zap.String("topic", string(topic)))
	return nil
}

// CreateTopicsIfNotExist creates topics if they don't already exist
func (a *KafkaAdminClient) CreateTopicsIfNotExist(ctx context.Context, topics []Topic, topicConfigs map[Topic]TopicConfig) error {
	// Get existing topics
	existingTopics, err := a.ListTopics(ctx)
	if err != nil {
		return fmt.Errorf("failed to list existing topics: %w", err)
	}

	existingTopicSet := make(map[string]bool)
	for _, topic := range existingTopics {
		existingTopicSet[topic] = true
	}

	// Create missing topics
	for _, topic := range topics {
		if !existingTopicSet[string(topic)] {
			config, exists := topicConfigs[topic]
			if !exists {
				a.logger.Warn("No configuration found for topic, using defaults", zap.String("topic", string(topic)))
				config = TopicConfig{
					Partitions:        12,
					ReplicationFactor: 3,
					RetentionMs:       7 * 24 * 60 * 60 * 1000, // 7 days
					CleanupPolicy:     "delete",
					CompressionType:   "snappy",
					MaxMessageBytes:   1048576,
				}
			}

			if err := a.CreateTopic(ctx, topic, config); err != nil {
				return err
			}
		} else {
			a.logger.Debug("Topic already exists", zap.String("topic", string(topic)))
		}
	}

	return nil
}

// ListTopics returns a list of existing topics
func (a *KafkaAdminClient) ListTopics(ctx context.Context) ([]string, error) {
	partitions, err := a.conn.ReadPartitions()
	if err != nil {
		return nil, fmt.Errorf("failed to read partitions: %w", err)
	}

	topicSet := make(map[string]bool)
	for _, partition := range partitions {
		topicSet[partition.Topic] = true
	}

	topics := make([]string, 0, len(topicSet))
	for topic := range topicSet {
		topics = append(topics, topic)
	}

	return topics, nil
}

// Close closes the admin client connection
func (a *KafkaAdminClient) Close() error {
	if a.conn != nil {
		return a.conn.Close()
	}
	return nil
}

// DeadLetterQueueHandler handles failed messages
type DeadLetterQueueHandler struct {
	producer   *KafkaProducer
	dlqTopic   Topic
	logger     *zap.Logger
	maxRetries int
}

// NewDeadLetterQueueHandler creates a new dead letter queue handler
func NewDeadLetterQueueHandler(producer *KafkaProducer, dlqTopic Topic, logger *zap.Logger) *DeadLetterQueueHandler {
	return &DeadLetterQueueHandler{
		producer:   producer,
		dlqTopic:   dlqTopic,
		logger:     logger,
		maxRetries: 3,
	}
}

// SendToDeadLetterQueue sends a failed message to the dead letter queue
func (dlq *DeadLetterQueueHandler) SendToDeadLetterQueue(ctx context.Context, originalMsg *ReceivedMessage, processingError error) error {
	dlqMessage := DeadLetterMessage{
		OriginalTopic:     originalMsg.Topic,
		OriginalKey:       originalMsg.Key,
		OriginalValue:     originalMsg.Value,
		OriginalHeaders:   originalMsg.Headers,
		OriginalOffset:    originalMsg.Offset,
		OriginalPartition: originalMsg.Partition,
		FailureReason:     processingError.Error(),
		FailureTimestamp:  time.Now(),
		RetryCount:        dlq.maxRetries,
	}

	key := fmt.Sprintf("dlq_%s_%s_%d", originalMsg.Topic, originalMsg.Key, time.Now().UnixNano())

	dlq.logger.Warn("Sending message to dead letter queue",
		zap.String("original_topic", originalMsg.Topic),
		zap.String("original_key", originalMsg.Key),
		zap.String("dlq_topic", string(dlq.dlqTopic)),
		zap.String("failure_reason", processingError.Error()))

	return dlq.producer.Publish(ctx, dlq.dlqTopic, key, dlqMessage)
}

// DeadLetterMessage represents a message in the dead letter queue
type DeadLetterMessage struct {
	OriginalTopic     string            `json:"original_topic"`
	OriginalKey       string            `json:"original_key"`
	OriginalValue     []byte            `json:"original_value"`
	OriginalHeaders   map[string][]byte `json:"original_headers"`
	OriginalOffset    int64             `json:"original_offset"`
	OriginalPartition int               `json:"original_partition"`
	FailureReason     string            `json:"failure_reason"`
	FailureTimestamp  time.Time         `json:"failure_timestamp"`
	RetryCount        int               `json:"retry_count"`
}

// MessageReplayRequest represents a request to replay messages
type MessageReplayRequest struct {
	Topics      []Topic           `json:"topics"`
	GroupID     string            `json:"group_id"`
	ReplayMode  ReplayMode        `json:"replay_mode"`
	StartOffset int64             `json:"start_offset,omitempty"`
	EndOffset   int64             `json:"end_offset,omitempty"`
	StartTime   *time.Time        `json:"start_time,omitempty"`
	EndTime     *time.Time        `json:"end_time,omitempty"`
	MaxMessages int               `json:"max_messages"`
	ReplaySpeed float64           `json:"replay_speed"` // 1.0 = real-time, 2.0 = 2x speed
	Handler     MessageHandler    `json:"-"`
	Filters     []MessageFilter   `json:"filters,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// ReplayMode defines different replay modes
type ReplayMode string

const (
	ReplayModeOffset    ReplayMode = "offset"
	ReplayModeTimestamp ReplayMode = "timestamp"
	ReplayModeLatest    ReplayMode = "latest"
	ReplayModeEarliest  ReplayMode = "earliest"
)

// MessageFilter defines filters for message replay
type MessageFilter struct {
	Field    string      `json:"field"`
	Operator string      `json:"operator"` // eq, neq, contains, regex
	Value    interface{} `json:"value"`
}

// ReplaySession represents an active replay session
type ReplaySession struct {
	ID        string               `json:"id"`
	Request   MessageReplayRequest `json:"request"`
	Status    ReplayStatus         `json:"status"`
	StartTime time.Time            `json:"start_time"`
	EndTime   *time.Time           `json:"end_time,omitempty"`
	Progress  ReplayProgress       `json:"progress"`
	Metrics   ReplayMetrics        `json:"metrics"`
	ctx       context.Context      `json:"-"`
	cancel    context.CancelFunc   `json:"-"`
}

// ReplayStatus represents the status of a replay session
type ReplayStatus string

const (
	ReplayStatusPending   ReplayStatus = "pending"
	ReplayStatusRunning   ReplayStatus = "running"
	ReplayStatusPaused    ReplayStatus = "paused"
	ReplayStatusCompleted ReplayStatus = "completed"
	ReplayStatusFailed    ReplayStatus = "failed"
	ReplayStatusCancelled ReplayStatus = "cancelled"
)

// ReplayProgress tracks replay progress
type ReplayProgress struct {
	TotalMessages     int64         `json:"total_messages"`
	ProcessedMessages int64         `json:"processed_messages"`
	SkippedMessages   int64         `json:"skipped_messages"`
	ErroredMessages   int64         `json:"errored_messages"`
	ProgressPercent   float64       `json:"progress_percent"`
	CurrentOffset     int64         `json:"current_offset"`
	EstimatedTimeLeft time.Duration `json:"estimated_time_left"`
}

// ReplayMetrics tracks replay performance metrics
type ReplayMetrics struct {
	MessagesPerSecond float64       `json:"messages_per_second"`
	BytesPerSecond    float64       `json:"bytes_per_second"`
	AverageLatency    time.Duration `json:"average_latency"`
	ErrorRate         float64       `json:"error_rate"`
	FilterMatchRate   float64       `json:"filter_match_rate"`
}

// MessageReplayManager manages message replay sessions
type MessageReplayManager struct {
	consumer      *KafkaConsumer
	producer      *KafkaProducer
	logger        *zap.Logger
	config        *KafkaConfig
	sessions      map[string]*ReplaySession
	sessionsMu    sync.RWMutex
	offsetManager *OffsetManager
	metrics       *ReplayManagerMetrics
}

// OffsetManager handles offset tracking and management
type OffsetManager struct {
	brokers []string
	logger  *zap.Logger
	mu      sync.RWMutex
}

// ReplayManagerMetrics tracks overall replay manager metrics
type ReplayManagerMetrics struct {
	ActiveSessions        int64 `json:"active_sessions"`
	CompletedSessions     int64 `json:"completed_sessions"`
	FailedSessions        int64 `json:"failed_sessions"`
	TotalBytesReplayed    int64 `json:"total_bytes_replayed"`
	TotalMessagesReplayed int64 `json:"total_messages_replayed"`
}

// NewMessageReplayManager creates a new message replay manager
func NewMessageReplayManager(consumer *KafkaConsumer, producer *KafkaProducer, config *KafkaConfig, logger *zap.Logger) *MessageReplayManager {
	return &MessageReplayManager{
		consumer:      consumer,
		producer:      producer,
		logger:        logger,
		config:        config,
		sessions:      make(map[string]*ReplaySession),
		offsetManager: NewOffsetManager(config.Brokers, logger),
		metrics:       &ReplayManagerMetrics{},
	}
}

// NewOffsetManager creates a new offset manager
func NewOffsetManager(brokers []string, logger *zap.Logger) *OffsetManager {
	return &OffsetManager{
		brokers: brokers,
		logger:  logger,
	}
}

// StartReplay starts a new message replay session
func (mrm *MessageReplayManager) StartReplay(ctx context.Context, request MessageReplayRequest) (*ReplaySession, error) {
	sessionID := fmt.Sprintf("replay_%d_%s", time.Now().Unix(), uuid.New().String()[:8])

	// Validate replay request
	if err := mrm.validateReplayRequest(&request); err != nil {
		return nil, fmt.Errorf("invalid replay request: %w", err)
	}

	// Create replay session
	replayCtx, cancel := context.WithCancel(ctx)
	session := &ReplaySession{
		ID:        sessionID,
		Request:   request,
		Status:    ReplayStatusPending,
		StartTime: time.Now(),
		Progress:  ReplayProgress{},
		Metrics:   ReplayMetrics{},
		ctx:       replayCtx,
		cancel:    cancel,
	}

	// Store session
	mrm.sessionsMu.Lock()
	mrm.sessions[sessionID] = session
	mrm.sessionsMu.Unlock()

	// Start replay in background
	go mrm.executeReplay(session)

	mrm.logger.Info("Started message replay session",
		zap.String("session_id", sessionID),
		zap.String("replay_mode", string(request.ReplayMode)),
		zap.Strings("topics", topicsToStrings(request.Topics)),
		zap.Int("max_messages", request.MaxMessages))

	return session, nil
}

// validateReplayRequest validates a replay request
func (mrm *MessageReplayManager) validateReplayRequest(request *MessageReplayRequest) error {
	if len(request.Topics) == 0 {
		return fmt.Errorf("at least one topic must be specified")
	}

	if request.GroupID == "" {
		return fmt.Errorf("group ID must be specified")
	}

	if request.MaxMessages <= 0 {
		request.MaxMessages = 10000 // Default limit
	}

	if request.ReplaySpeed <= 0 {
		request.ReplaySpeed = 1.0 // Default to real-time
	}

	switch request.ReplayMode {
	case ReplayModeOffset:
		if request.StartOffset < 0 {
			return fmt.Errorf("start offset must be non-negative for offset mode")
		}
	case ReplayModeTimestamp:
		if request.StartTime == nil {
			return fmt.Errorf("start time must be specified for timestamp mode")
		}
	case ReplayModeLatest, ReplayModeEarliest:
		// No additional validation needed
	default:
		return fmt.Errorf("unsupported replay mode: %s", request.ReplayMode)
	}

	return nil
}

// executeReplay executes a replay session
func (mrm *MessageReplayManager) executeReplay(session *ReplaySession) {
	defer func() {
		session.Status = ReplayStatusCompleted
		now := time.Now()
		session.EndTime = &now
		mrm.metrics.CompletedSessions++

		mrm.logger.Info("Replay session completed",
			zap.String("session_id", session.ID),
			zap.Duration("duration", time.Since(session.StartTime)),
			zap.Int64("messages_processed", session.Progress.ProcessedMessages))
	}()

	session.Status = ReplayStatusRunning
	startTime := time.Now()

	// Get topic partitions and offsets
	topicPartitions, err := mrm.getTopicPartitions(session.Request.Topics)
	if err != nil {
		mrm.logger.Error("Failed to get topic partitions", zap.Error(err))
		session.Status = ReplayStatusFailed
		mrm.metrics.FailedSessions++
		return
	}

	// Calculate start offsets for each partition
	offsetMap, err := mrm.calculateReplayOffsets(session.Request, topicPartitions)
	if err != nil {
		mrm.logger.Error("Failed to calculate replay offsets", zap.Error(err))
		session.Status = ReplayStatusFailed
		mrm.metrics.FailedSessions++
		return
	}

	// Create readers for each topic-partition
	readers := make(map[string]*kafka.Reader)
	defer func() {
		for _, reader := range readers {
			reader.Close()
		}
	}()

	for topicPartition, startOffset := range offsetMap {
		reader := kafka.NewReader(kafka.ReaderConfig{
			Brokers:     mrm.config.Brokers,
			Topic:       topicPartition.Topic,
			Partition:   topicPartition.Partition,
			StartOffset: startOffset,
			MaxBytes:    mrm.config.MaxMessageBytes,
		})
		readers[topicPartition.String()] = reader
	}

	// Replay messages
	var totalMessages int64
	var processedBytes int64

	for len(readers) > 0 && session.Progress.ProcessedMessages < int64(session.Request.MaxMessages) {
		select {
		case <-session.ctx.Done():
			session.Status = ReplayStatusCancelled
			return
		default:
		}

		// Read from each reader in round-robin fashion
		for key, reader := range readers {
			msg, err := reader.ReadMessage(session.ctx)
			if err != nil {
				if err == io.EOF {
					// Reader exhausted, remove it
					reader.Close()
					delete(readers, key)
					continue
				}
				mrm.logger.Error("Error reading message during replay",
					zap.String("session_id", session.ID),
					zap.String("reader", key),
					zap.Error(err))
				session.Progress.ErroredMessages++
				continue
			}

			// Apply filters
			if !mrm.applyFilters(session.Request.Filters, &msg) {
				session.Progress.SkippedMessages++
				continue
			}

			// Convert to ReceivedMessage
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

			// Handle message
			if err := session.Request.Handler(session.ctx, receivedMsg); err != nil {
				mrm.logger.Error("Handler error during replay",
					zap.String("session_id", session.ID),
					zap.Error(err),
					zap.String("topic", msg.Topic),
					zap.Int64("offset", msg.Offset))
				session.Progress.ErroredMessages++
			} else {
				session.Progress.ProcessedMessages++
				processedBytes += int64(len(msg.Value))
				session.Progress.CurrentOffset = msg.Offset
			}

			totalMessages++

			// Update metrics
			mrm.updateSessionMetrics(session, startTime, processedBytes)

			// Apply replay speed control
			if session.Request.ReplaySpeed < 10.0 { // Only apply delay for reasonable speeds
				delay := time.Duration(float64(time.Millisecond) / session.Request.ReplaySpeed)
				time.Sleep(delay)
			}

			// Check if we've reached the message limit
			if session.Progress.ProcessedMessages >= int64(session.Request.MaxMessages) {
				break
			}
		}
	}

	// Final metrics update
	mrm.updateSessionMetrics(session, startTime, processedBytes)
	session.Progress.ProgressPercent = 100.0
}

// applyFilters applies message filters
func (mrm *MessageReplayManager) applyFilters(filters []MessageFilter, msg *kafka.Message) bool {
	if len(filters) == 0 {
		return true
	}

	for _, filter := range filters {
		if !mrm.applyFilter(filter, msg) {
			return false
		}
	}

	return true
}

// applyFilter applies a single message filter
func (mrm *MessageReplayManager) applyFilter(filter MessageFilter, msg *kafka.Message) bool {
	var fieldValue interface{}

	switch filter.Field {
	case "key":
		fieldValue = string(msg.Key)
	case "topic":
		fieldValue = msg.Topic
	case "partition":
		fieldValue = msg.Partition
	case "offset":
		fieldValue = msg.Offset
	case "timestamp":
		fieldValue = msg.Time
	default:
		// Try to extract from message value (assuming JSON)
		var msgData map[string]interface{}
		if err := json.Unmarshal(msg.Value, &msgData); err == nil {
			if val, exists := msgData[filter.Field]; exists {
				fieldValue = val
			}
		}
	}

	if fieldValue == nil {
		return false
	}

	switch filter.Operator {
	case "eq":
		return fieldValue == filter.Value
	case "neq":
		return fieldValue != filter.Value
	case "contains":
		if str, ok := fieldValue.(string); ok {
			if filterStr, ok := filter.Value.(string); ok {
				return strings.Contains(str, filterStr)
			}
		}
	case "regex":
		if str, ok := fieldValue.(string); ok {
			if pattern, ok := filter.Value.(string); ok {
				if matched, err := regexp.MatchString(pattern, str); err == nil {
					return matched
				}
			}
		}
	}

	return false
}

// updateSessionMetrics updates session metrics
func (mrm *MessageReplayManager) updateSessionMetrics(session *ReplaySession, startTime time.Time, processedBytes int64) {
	duration := time.Since(startTime)
	if duration.Seconds() > 0 {
		session.Metrics.MessagesPerSecond = float64(session.Progress.ProcessedMessages) / duration.Seconds()
		session.Metrics.BytesPerSecond = float64(processedBytes) / duration.Seconds()
	}

	if session.Progress.ProcessedMessages > 0 {
		session.Metrics.ErrorRate = float64(session.Progress.ErroredMessages) / float64(session.Progress.ProcessedMessages) * 100
		session.Metrics.FilterMatchRate = float64(session.Progress.ProcessedMessages) / float64(session.Progress.ProcessedMessages+session.Progress.SkippedMessages) * 100
	}

	// Estimate time left
	if session.Metrics.MessagesPerSecond > 0 {
		remaining := int64(session.Request.MaxMessages) - session.Progress.ProcessedMessages
		session.Progress.EstimatedTimeLeft = time.Duration(float64(remaining)/session.Metrics.MessagesPerSecond) * time.Second
	}

	// Update progress percentage
	session.Progress.ProgressPercent = float64(session.Progress.ProcessedMessages) / float64(session.Request.MaxMessages) * 100
}

// getTopicPartitions gets partition information for topics
func (mrm *MessageReplayManager) getTopicPartitions(topics []Topic) ([]TopicPartition, error) {
	var partitions []TopicPartition

	for _, topic := range topics {
		conn, err := kafka.Dial("tcp", mrm.config.Brokers[0])
		if err != nil {
			return nil, fmt.Errorf("failed to connect to Kafka: %w", err)
		}
		defer conn.Close()

		partitionInfos, err := conn.ReadPartitions(string(topic))
		if err != nil {
			return nil, fmt.Errorf("failed to read partitions for topic %s: %w", topic, err)
		}

		for _, partition := range partitionInfos {
			partitions = append(partitions, TopicPartition{
				Topic:     partition.Topic,
				Partition: partition.ID,
			})
		}
	}

	return partitions, nil
}

// calculateReplayOffsets calculates start offsets for replay
func (mrm *MessageReplayManager) calculateReplayOffsets(request MessageReplayRequest, partitions []TopicPartition) (map[TopicPartition]int64, error) {
	offsetMap := make(map[TopicPartition]int64)

	for _, partition := range partitions {
		var startOffset int64

		switch request.ReplayMode {
		case ReplayModeOffset:
			startOffset = request.StartOffset

		case ReplayModeTimestamp:
			offset, err := mrm.offsetManager.GetOffsetByTimestamp(partition, *request.StartTime)
			if err != nil {
				return nil, fmt.Errorf("failed to get offset by timestamp: %w", err)
			}
			startOffset = offset

		case ReplayModeEarliest:
			offset, err := mrm.offsetManager.GetEarliestOffset(partition)
			if err != nil {
				return nil, fmt.Errorf("failed to get earliest offset: %w", err)
			}
			startOffset = offset

		case ReplayModeLatest:
			offset, err := mrm.offsetManager.GetLatestOffset(partition)
			if err != nil {
				return nil, fmt.Errorf("failed to get latest offset: %w", err)
			}
			startOffset = offset

		default:
			return nil, fmt.Errorf("unsupported replay mode: %s", request.ReplayMode)
		}

		offsetMap[partition] = startOffset
	}

	return offsetMap, nil
}

// TopicPartition represents a topic-partition pair
type TopicPartition struct {
	Topic     string `json:"topic"`
	Partition int    `json:"partition"`
}

// String returns a string representation of the topic-partition
func (tp TopicPartition) String() string {
	return fmt.Sprintf("%s-%d", tp.Topic, tp.Partition)
}

// GetOffsetByTimestamp gets the offset for a given timestamp
func (om *OffsetManager) GetOffsetByTimestamp(partition TopicPartition, timestamp time.Time) (int64, error) {
	conn, err := kafka.DialLeader(context.Background(), "tcp", om.brokers[0], partition.Topic, partition.Partition)
	if err != nil {
		return 0, fmt.Errorf("failed to dial leader: %w", err)
	}
	defer conn.Close()

	offset, err := conn.ReadOffset(timestamp)
	if err != nil {
		return 0, fmt.Errorf("failed to read offset by timestamp: %w", err)
	}

	return offset, nil
}

// GetEarliestOffset gets the earliest available offset
func (om *OffsetManager) GetEarliestOffset(partition TopicPartition) (int64, error) {
	conn, err := kafka.DialLeader(context.Background(), "tcp", om.brokers[0], partition.Topic, partition.Partition)
	if err != nil {
		return 0, fmt.Errorf("failed to dial leader: %w", err)
	}
	defer conn.Close()

	first, _, err := conn.ReadOffsets()
	if err != nil {
		return 0, fmt.Errorf("failed to read offsets: %w", err)
	}

	return first, nil
}

// GetLatestOffset gets the latest available offset
func (om *OffsetManager) GetLatestOffset(partition TopicPartition) (int64, error) {
	conn, err := kafka.DialLeader(context.Background(), "tcp", om.brokers[0], partition.Topic, partition.Partition)
	if err != nil {
		return 0, fmt.Errorf("failed to dial leader: %w", err)
	}
	defer conn.Close()

	_, last, err := conn.ReadOffsets()
	if err != nil {
		return 0, fmt.Errorf("failed to read offsets: %w", err)
	}

	return last, nil
}

// Helper functions
func topicsToStrings(topics []Topic) []string {
	strings := make([]string, len(topics))
	for i, topic := range topics {
		strings[i] = string(topic)
	}
	return strings
}

// GetSession returns a replay session by ID
func (mrm *MessageReplayManager) GetSession(sessionID string) (*ReplaySession, error) {
	mrm.sessionsMu.RLock()
	defer mrm.sessionsMu.RUnlock()

	session, exists := mrm.sessions[sessionID]
	if !exists {
		return nil, fmt.Errorf("replay session not found: %s", sessionID)
	}

	return session, nil
}

// StopReplay stops a replay session
func (mrm *MessageReplayManager) StopReplay(sessionID string) error {
	mrm.sessionsMu.Lock()
	defer mrm.sessionsMu.Unlock()

	session, exists := mrm.sessions[sessionID]
	if !exists {
		return fmt.Errorf("replay session not found: %s", sessionID)
	}

	if session.Status == ReplayStatusRunning {
		session.cancel()
		session.Status = ReplayStatusCancelled
		mrm.logger.Info("Stopped replay session", zap.String("session_id", sessionID))
	}

	return nil
}

// ListSessions returns all replay sessions
func (mrm *MessageReplayManager) ListSessions() []*ReplaySession {
	mrm.sessionsMu.RLock()
	defer mrm.sessionsMu.RUnlock()

	sessions := make([]*ReplaySession, 0, len(mrm.sessions))
	for _, session := range mrm.sessions {
		sessions = append(sessions, session)
	}

	return sessions
}

// GetMetrics returns replay manager metrics
func (mrm *MessageReplayManager) GetMetrics() *ReplayManagerMetrics {
	mrm.sessionsMu.RLock()
	defer mrm.sessionsMu.RUnlock()

	// Count active sessions
	var activeSessions int64
	for _, session := range mrm.sessions {
		if session.Status == ReplayStatusRunning || session.Status == ReplayStatusPending {
			activeSessions++
		}
	}

	mrm.metrics.ActiveSessions = activeSessions
	return mrm.metrics
}

// HealthStatus represents the health status of messaging components
type HealthStatus struct {
	Healthy               bool                   `json:"healthy"`
	ProducerHealthy       bool                   `json:"producer_healthy"`
	ConsumerHealthy       bool                   `json:"consumer_healthy"`
	CircuitBreakerState   CircuitBreakerState    `json:"circuit_breaker_state"`
	ProducerMetrics       *ProducerMetrics       `json:"producer_metrics,omitempty"`
	ConsumerMetrics       *ConsumerMetrics       `json:"consumer_metrics,omitempty"`
	CircuitBreakerMetrics *CircuitBreakerMetrics `json:"circuit_breaker_metrics,omitempty"`
	LastHealthCheck       time.Time              `json:"last_health_check"`
	Issues                []string               `json:"issues,omitempty"`
}

// KafkaMessagingService provides integrated Kafka messaging with circuit breaker protection
type KafkaMessagingService struct {
	producer         *KafkaProducer
	consumer         *KafkaConsumer
	adminClient      *KafkaAdminClient
	config           *KafkaConfig
	logger           *zap.Logger
	metricsCollector *MetricsCollector
	healthStatus     *HealthStatus
	mu               sync.RWMutex
}

// NewKafkaMessagingService creates a new integrated Kafka messaging service
func NewKafkaMessagingService(config *KafkaConfig, logger *zap.Logger) (*KafkaMessagingService, error) {
	if config == nil {
		config = DefaultKafkaConfig()
	}

	producer, err := NewKafkaProducer(config, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create producer: %w", err)
	}

	consumer, err := NewKafkaConsumer(config, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create consumer: %w", err)
	}

	adminClient, err := NewKafkaAdminClient(config, logger)
	if err != nil {
		logger.Warn("Failed to create admin client", zap.Error(err))
		// Admin client is optional, continue without it
	}

	metricsCollector := NewMetricsCollector(logger)
	metricsCollector.SetCircuitBreaker(producer.circuitBreaker)

	service := &KafkaMessagingService{
		producer:         producer,
		consumer:         consumer,
		adminClient:      adminClient,
		config:           config,
		logger:           logger,
		metricsCollector: metricsCollector,
		healthStatus:     &HealthStatus{},
	}

	// Start periodic health checks
	go service.periodicHealthCheck()

	return service, nil
}

// PerformHealthCheck performs a comprehensive health check of all messaging components
func (kms *KafkaMessagingService) PerformHealthCheck() *HealthStatus {
	kms.mu.Lock()
	defer kms.mu.Unlock()

	status := &HealthStatus{
		LastHealthCheck: time.Now(),
		Issues:          make([]string, 0),
	}

	// Check producer health
	status.ProducerHealthy = kms.producer.IsHealthy()
	if !status.ProducerHealthy {
		status.Issues = append(status.Issues, fmt.Sprintf("Producer circuit breaker is %s", kms.producer.GetCircuitBreakerState()))
	}

	// Check consumer health
	status.ConsumerHealthy = kms.consumer.IsHealthy()
	if !status.ConsumerHealthy {
		status.Issues = append(status.Issues, fmt.Sprintf("Consumer circuit breaker is %s", kms.consumer.GetCircuitBreakerState()))
	}

	// Get circuit breaker state (use producer's circuit breaker as primary)
	status.CircuitBreakerState = kms.producer.GetCircuitBreakerState()

	// Overall health is true if both producer and consumer are healthy
	status.Healthy = status.ProducerHealthy && status.ConsumerHealthy

	// Collect metrics
	status.ProducerMetrics = kms.producer.GetMetrics()
	status.ConsumerMetrics = kms.consumer.GetMetrics()
	status.CircuitBreakerMetrics = kms.producer.circuitBreaker.GetMetrics()

	// Update internal health status
	kms.healthStatus = status

	return status
}

// periodicHealthCheck runs health checks periodically
func (kms *KafkaMessagingService) periodicHealthCheck() {
	ticker := time.NewTicker(30 * time.Second) // Health check every 30 seconds
	defer ticker.Stop()

	for range ticker.C {
		status := kms.PerformHealthCheck()

		if !status.Healthy {
			kms.logger.Warn("Messaging service health check failed",
				zap.Bool("producer_healthy", status.ProducerHealthy),
				zap.Bool("consumer_healthy", status.ConsumerHealthy),
				zap.String("circuit_breaker_state", status.CircuitBreakerState.String()),
				zap.Strings("issues", status.Issues))
		} else {
			kms.logger.Debug("Messaging service health check passed",
				zap.String("circuit_breaker_state", status.CircuitBreakerState.String()))
		}
	}
}

// GetProducer returns the Kafka producer
func (kms *KafkaMessagingService) GetProducer() Producer {
	return kms.producer
}

// GetConsumer returns the Kafka consumer
func (kms *KafkaMessagingService) GetConsumer() Consumer {
	return kms.consumer
}

// GetAdminClient returns the Kafka admin client
func (kms *KafkaMessagingService) GetAdminClient() *KafkaAdminClient {
	return kms.adminClient
}

// GetMetrics returns comprehensive messaging metrics
func (kms *KafkaMessagingService) GetMetrics() *MessagingMetrics {
	return kms.metricsCollector.GetMetrics()
}

// GetHealthStatus returns the current health status
func (kms *KafkaMessagingService) GetHealthStatus() *HealthStatus {
	kms.mu.RLock()
	defer kms.mu.RUnlock()

	// Return a copy to avoid race conditions
	statusCopy := *kms.healthStatus
	return &statusCopy
}

// ResetCircuitBreaker manually resets the circuit breakers
func (kms *KafkaMessagingService) ResetCircuitBreaker() {
	kms.producer.circuitBreaker.Reset()
	kms.consumer.circuitBreaker.Reset()
	kms.logger.Info("Circuit breakers manually reset")
}

// Close gracefully closes all messaging components
func (kms *KafkaMessagingService) Close() error {
	var lastErr error

	if err := kms.producer.Close(); err != nil {
		lastErr = err
		kms.logger.Error("Failed to close producer", zap.Error(err))
	}

	if err := kms.consumer.Close(); err != nil {
		lastErr = err
		kms.logger.Error("Failed to close consumer", zap.Error(err))
	}

	if kms.adminClient != nil {
		if err := kms.adminClient.Close(); err != nil {
			lastErr = err
			kms.logger.Error("Failed to close admin client", zap.Error(err))
		}
	}

	kms.logger.Info("Kafka messaging service closed")
	return lastErr
}

// ExecuteWithCircuitBreaker executes an operation with circuit breaker protection
func (kms *KafkaMessagingService) ExecuteWithCircuitBreaker(operation func() error) error {
	return kms.producer.circuitBreaker.ExecuteWithCircuitBreaker(operation)
}
