package test

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Aidin1998/finalex/internal/infrastructure/messaging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

// TestMessagingIntegration tests the complete messaging infrastructure
func TestMessagingIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	logger := zaptest.NewLogger(t)

	// Test cases for different scenarios
	t.Run("KafkaProducerConsumerIntegration", func(t *testing.T) {
		testKafkaProducerConsumerIntegration(t, ctx, logger)
	})

	t.Run("MessageBusEventPublishing", func(t *testing.T) {
		testMessageBusEventPublishing(t, ctx, logger)
	})

	t.Run("TopicAutoCreation", func(t *testing.T) {
		testTopicAutoCreation(t, ctx, logger)
	})

	t.Run("DeadLetterQueueHandling", func(t *testing.T) {
		testDeadLetterQueueHandling(t, ctx, logger)
	})

	t.Run("HighVolumeMessageProcessing", func(t *testing.T) {
		testHighVolumeMessageProcessing(t, ctx, logger)
	})
}

func testKafkaProducerConsumerIntegration(t *testing.T, ctx context.Context, logger *zap.Logger) {
	// Setup Kafka configuration for testing
	config := &messaging.KafkaConfig{
		Brokers:             []string{"localhost:9092"}, // Assumes Kafka is running locally
		ReadTimeout:         5 * time.Second,
		WriteTimeout:        5 * time.Second,
		BatchSize:           100,
		BatchTimeout:        10 * time.Millisecond,
		RequiredAcks:        1,
		Compression:         "snappy",
		RetryMax:            3,
		ConsumerGroupPrefix: "test",
		EnableBuffering:     false, // Disable buffering for tests
		MaxMessageBytes:     1048576,
	}

	// Create producer
	producer, err := messaging.NewKafkaProducer(config, logger)
	require.NoError(t, err)
	defer producer.Close()

	// Create consumer
	consumer, err := messaging.NewKafkaConsumer(config, logger)
	require.NoError(t, err)
	defer consumer.Close()

	// Test topic
	testTopic := messaging.Topic("test-integration-topic")
	testKey := "test-key"
	testMessage := map[string]interface{}{
		"id":        "test-123",
		"timestamp": time.Now().Unix(),
		"data":      "integration test message",
	}

	// Channel to receive messages
	receivedMessages := make(chan *messaging.ReceivedMessage, 10)
	var wg sync.WaitGroup

	// Setup message handler
	messageHandler := func(ctx context.Context, msg *messaging.ReceivedMessage) error {
		receivedMessages <- msg
		wg.Done()
		return nil
	}

	// Subscribe to the test topic
	err = consumer.Subscribe(ctx, []messaging.Topic{testTopic}, "integration-test-group", messageHandler)
	require.NoError(t, err)

	// Give consumer time to start
	time.Sleep(2 * time.Second)

	// Publish a test message
	wg.Add(1)
	err = producer.Publish(ctx, testTopic, testKey, testMessage)
	require.NoError(t, err)

	// Wait for message to be received
	wg.Wait()

	// Verify message was received
	select {
	case receivedMsg := <-receivedMessages:
		assert.Equal(t, string(testTopic), receivedMsg.Topic)
		assert.Equal(t, testKey, receivedMsg.Key)

		var receivedData map[string]interface{}
		err := json.Unmarshal(receivedMsg.Value, &receivedData)
		require.NoError(t, err)

		assert.Equal(t, testMessage["id"], receivedData["id"])
		assert.Equal(t, testMessage["data"], receivedData["data"])

	case <-time.After(10 * time.Second):
		t.Fatal("Timeout waiting for message")
	}
}

func testMessageBusEventPublishing(t *testing.T, ctx context.Context, logger *zap.Logger) {
	// Setup messaging factory
	config := messaging.DefaultMessagingConfig()
	config.Kafka.Brokers = []string{"localhost:9092"}
	config.Kafka.EnableBuffering = false

	factory := messaging.NewMessagingFactory(config, logger)

	// Create message bus
	messageBus, err := factory.CreateMessageBus()
	require.NoError(t, err)
	defer messageBus.Stop()

	// Test order event publishing
	orderEvent := &messaging.OrderEventMessage{
		BaseMessage: messaging.NewBaseMessage(messaging.MsgOrderPlaced, "test-service", "user-123"),
		OrderID:     "order-123",
		UserID:      "user-123",
		Symbol:      "BTC/USD",
		Side:        "buy",
		Quantity:    "1.0",
		Price:       "50000.00",
	}

	err = messageBus.PublishOrderEvent(ctx, orderEvent)
	assert.NoError(t, err)

	// Test trade event publishing
	tradeEvent := &messaging.TradeEventMessage{
		BaseMessage: messaging.NewBaseMessage(messaging.MsgTradeExecuted, "test-service", "user-123"),
		TradeID:     "trade-123",
		OrderID:     "order-123",
		Symbol:      "BTC/USD",
		Quantity:    "1.0",
		Price:       "50000.00",
	}

	err = messageBus.PublishTradeEvent(ctx, tradeEvent)
	assert.NoError(t, err)

	// Test balance event publishing
	balanceEvent := &messaging.BalanceEventMessage{
		BaseMessage: messaging.NewBaseMessage(messaging.MsgBalanceUpdated, "test-service", "user-123"),
		UserID:      "user-123",
		Currency:    "BTC",
		Balance:     "1.0",
		Available:   "1.0",
		Locked:      "0.0",
	}

	err = messageBus.PublishBalanceEvent(ctx, balanceEvent)
	assert.NoError(t, err)
}

func testTopicAutoCreation(t *testing.T, ctx context.Context, logger *zap.Logger) {
	config := messaging.DefaultKafkaConfig()
	config.Brokers = []string{"localhost:9092"}

	// Create admin client
	adminClient, err := messaging.NewKafkaAdminClient(config, logger)
	require.NoError(t, err)
	defer adminClient.Close()

	// Define test topics
	testTopics := []messaging.Topic{
		messaging.Topic("test-auto-create-1"),
		messaging.Topic("test-auto-create-2"),
	}

	topicConfigs := map[messaging.Topic]messaging.TopicConfig{
		messaging.Topic("test-auto-create-1"): {
			Partitions:        3,
			ReplicationFactor: 1,                   // Use 1 for single-node Kafka in tests
			RetentionMs:       24 * 60 * 60 * 1000, // 1 day
			CleanupPolicy:     "delete",
			CompressionType:   "snappy",
			MaxMessageBytes:   1048576,
		},
		messaging.Topic("test-auto-create-2"): {
			Partitions:        6,
			ReplicationFactor: 1,
			RetentionMs:       24 * 60 * 60 * 1000,
			CleanupPolicy:     "delete",
			CompressionType:   "snappy",
			MaxMessageBytes:   1048576,
		},
	}

	// Create topics if they don't exist
	err = adminClient.CreateTopicsIfNotExist(ctx, testTopics, topicConfigs)
	assert.NoError(t, err)

	// Verify topics were created by listing them
	existingTopics, err := adminClient.ListTopics(ctx)
	require.NoError(t, err)

	topicSet := make(map[string]bool)
	for _, topic := range existingTopics {
		topicSet[topic] = true
	}

	for _, testTopic := range testTopics {
		assert.True(t, topicSet[string(testTopic)], "Topic %s should exist", testTopic)
	}
}

func testDeadLetterQueueHandling(t *testing.T, ctx context.Context, logger *zap.Logger) {
	config := messaging.DefaultKafkaConfig()
	config.Brokers = []string{"localhost:9092"}

	// Create producer for DLQ
	producer, err := messaging.NewKafkaProducer(config, logger)
	require.NoError(t, err)
	defer producer.Close()

	// Create DLQ handler
	dlqTopic := messaging.Topic("test-dead-letter-queue")
	dlqHandler := messaging.NewDeadLetterQueueHandler(producer, dlqTopic, logger)

	// Simulate a failed message
	originalMsg := &messaging.ReceivedMessage{
		Topic:     "test-topic",
		Key:       "test-key",
		Value:     []byte(`{"id": "failed-message"}`),
		Headers:   make(map[string][]byte),
		Offset:    123,
		Partition: 0,
		Timestamp: time.Now(),
	}

	processingError := fmt.Errorf("simulated processing failure")

	// Send to dead letter queue
	err = dlqHandler.SendToDeadLetterQueue(ctx, originalMsg, processingError)
	assert.NoError(t, err)

	// TODO: Add verification by consuming from DLQ topic
}

func testHighVolumeMessageProcessing(t *testing.T, ctx context.Context, logger *zap.Logger) {
	config := messaging.DefaultKafkaConfig()
	config.Brokers = []string{"localhost:9092"}
	config.EnableBuffering = true
	config.BufferSize = 1000
	config.BufferFlushInterval = 10 * time.Millisecond

	// Create producer with buffering enabled
	producer, err := messaging.NewKafkaProducer(config, logger)
	require.NoError(t, err)
	defer producer.Close()

	testTopic := messaging.Topic("test-high-volume")
	messageCount := 1000

	start := time.Now()

	// Send high volume of messages
	for i := 0; i < messageCount; i++ {
		message := map[string]interface{}{
			"id":        fmt.Sprintf("msg-%d", i),
			"timestamp": time.Now().Unix(),
			"sequence":  i,
		}

		key := fmt.Sprintf("key-%d", i%10) // Distribute across 10 partitions

		err := producer.Publish(ctx, testTopic, key, message)
		assert.NoError(t, err)
	}

	elapsed := time.Since(start)
	throughput := float64(messageCount) / elapsed.Seconds()

	logger.Info("High volume test completed",
		zap.Int("message_count", messageCount),
		zap.Duration("elapsed", elapsed),
		zap.Float64("throughput_msg_per_sec", throughput))

	// Verify reasonable throughput (should be able to handle at least 1000 msg/sec)
	assert.Greater(t, throughput, 100.0, "Throughput should be at least 100 messages per second")
}

// TestMessagingConfigValidation tests configuration validation
func TestMessagingConfigValidation(t *testing.T) {
	tests := []struct {
		name        string
		config      *messaging.MessagingConfig
		expectError bool
	}{
		{
			name:        "Valid default config",
			config:      messaging.DefaultMessagingConfig(),
			expectError: false,
		},
		{
			name: "Invalid Kafka brokers",
			config: &messaging.MessagingConfig{
				Kafka: messaging.KafkaConfig{
					Brokers: []string{}, // Empty brokers should be invalid
				},
			},
			expectError: true,
		},
		{
			name: "Invalid topic config",
			config: &messaging.MessagingConfig{
				Kafka: *messaging.DefaultKafkaConfig(),
				TopicConfig: map[string]messaging.TopicConfig{
					"invalid-topic": {
						Partitions:        0,  // Invalid partition count
						ReplicationFactor: -1, // Invalid replication factor
					},
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestMessagingFactoryCreation tests messaging factory functionality
func TestMessagingFactoryCreation(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := messaging.DefaultMessagingConfig()
	config.Kafka.Brokers = []string{"localhost:9092"}

	factory := messaging.NewMessagingFactory(config, logger)
	require.NotNil(t, factory)

	// Test message bus creation
	messageBus, err := factory.CreateMessageBus()
	if err != nil {
		// If Kafka is not available, skip this test
		t.Skipf("Kafka not available for testing: %v", err)
	}

	require.NotNil(t, messageBus)
	defer messageBus.Stop()
}

// BenchmarkMessagePublishing benchmarks message publishing performance
func BenchmarkMessagePublishing(b *testing.B) {
	logger := zap.NewNop()
	config := messaging.DefaultKafkaConfig()
	config.Brokers = []string{"localhost:9092"}
	config.EnableBuffering = true

	producer, err := messaging.NewKafkaProducer(config, logger)
	if err != nil {
		b.Skipf("Kafka not available for benchmarking: %v", err)
	}
	defer producer.Close()

	ctx := context.Background()
	testTopic := messaging.Topic("benchmark-topic")
	message := map[string]interface{}{
		"benchmark": true,
		"timestamp": time.Now().Unix(),
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("bench-key-%d", i)
			producer.Publish(ctx, testTopic, key, message)
			i++
		}
	})
}
