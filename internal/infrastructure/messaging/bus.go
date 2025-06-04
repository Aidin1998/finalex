package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// MessageBus coordinates message publishing and consumption across services
type MessageBus struct {
	producer Producer
	consumer Consumer
	logger   *zap.Logger
	handlers map[MessageType][]MessageHandler
	mu       sync.RWMutex
	ctx      context.Context
	cancel   context.CancelFunc
}

// NewMessageBus creates a new message bus instance
func NewMessageBus(producer Producer, consumer Consumer, logger *zap.Logger) *MessageBus {
	ctx, cancel := context.WithCancel(context.Background())

	return &MessageBus{
		producer: producer,
		consumer: consumer,
		logger:   logger,
		handlers: make(map[MessageType][]MessageHandler),
		ctx:      ctx,
		cancel:   cancel,
	}
}

// PublishOrderEvent publishes an order-related event
func (mb *MessageBus) PublishOrderEvent(ctx context.Context, event *OrderEventMessage) error {
	topic := GetTopic(MessageType(event.Type))
	key := fmt.Sprintf("%s:%s", event.UserID, event.OrderID)

	mb.logger.Debug("Publishing order event",
		zap.String("type", string(event.Type)),
		zap.String("order_id", event.OrderID),
		zap.String("user_id", event.UserID))

	return mb.producer.Publish(ctx, topic, key, event)
}

// PublishTradeEvent publishes a trade-related event
func (mb *MessageBus) PublishTradeEvent(ctx context.Context, event *TradeEventMessage) error {
	topic := GetTopic(event.Type)
	key := fmt.Sprintf("%s:%s", event.Symbol, event.TradeID)

	mb.logger.Debug("Publishing trade event",
		zap.String("type", string(event.Type)),
		zap.String("trade_id", event.TradeID),
		zap.String("symbol", event.Symbol))

	return mb.producer.Publish(ctx, topic, key, event)
}

// PublishBalanceEvent publishes a balance-related event
func (mb *MessageBus) PublishBalanceEvent(ctx context.Context, event *BalanceEventMessage) error {
	topic := GetTopic(event.Type)
	key := fmt.Sprintf("%s:%s", event.UserID, event.Currency)

	mb.logger.Debug("Publishing balance event",
		zap.String("type", string(event.Type)),
		zap.String("user_id", event.UserID),
		zap.String("currency", event.Currency))

	return mb.producer.Publish(ctx, topic, key, event)
}

// PublishFundsOperationEvent publishes a funds operation event
func (mb *MessageBus) PublishFundsOperationEvent(ctx context.Context, event *FundsOperationMessage) error {
	topic := GetTopic(event.Type)
	key := fmt.Sprintf("%s:%s", event.UserID, event.Currency)

	mb.logger.Debug("Publishing funds operation event",
		zap.String("type", string(event.Type)),
		zap.String("user_id", event.UserID),
		zap.String("currency", event.Currency))

	return mb.producer.Publish(ctx, topic, key, event)
}

// PublishMarketDataEvent publishes a market data event
func (mb *MessageBus) PublishMarketDataEvent(ctx context.Context, msgType MessageType, symbol string, data interface{}) error {
	topic := GetTopic(msgType)
	key := symbol

	baseMsg := NewBaseMessage(msgType, "market-data", "")

	var event interface{}
	switch msgType {
	case MsgOrderBookUpdate:
		if update, ok := data.(*OrderBookUpdateMessage); ok {
			update.BaseMessage = baseMsg
			event = update
		} else {
			return fmt.Errorf("invalid order book update data type")
		}
	case MsgTicker:
		if ticker, ok := data.(*TickerUpdateMessage); ok {
			ticker.BaseMessage = baseMsg
			event = ticker
		} else {
			return fmt.Errorf("invalid ticker update data type")
		}
	case MsgCandle:
		if candle, ok := data.(*CandleUpdateMessage); ok {
			candle.BaseMessage = baseMsg
			event = candle
		} else {
			return fmt.Errorf("invalid candle update data type")
		}
	default:
		return fmt.Errorf("unsupported market data message type: %s", msgType)
	}

	mb.logger.Debug("Publishing market data event",
		zap.String("type", string(msgType)),
		zap.String("symbol", symbol))

	return mb.producer.Publish(ctx, topic, key, event)
}

// PublishNotification publishes a notification event
func (mb *MessageBus) PublishNotification(ctx context.Context, event *NotificationMessage) error {
	topic := GetTopic(event.Type)
	key := event.UserID
	if key == "" {
		key = "system"
	}

	mb.logger.Debug("Publishing notification",
		zap.String("type", string(event.Type)),
		zap.String("user_id", event.UserID),
		zap.String("level", event.Level))

	return mb.producer.Publish(ctx, topic, key, event)
}

// PublishRiskEvent publishes a risk management event
func (mb *MessageBus) PublishRiskEvent(ctx context.Context, event *RiskEventMessage) error {
	topic := GetTopic(event.Type)
	key := fmt.Sprintf("%s:%s", event.UserID, event.RiskType)

	mb.logger.Warn("Publishing risk event",
		zap.String("type", string(event.Type)),
		zap.String("user_id", event.UserID),
		zap.String("risk_type", event.RiskType),
		zap.String("severity", event.Severity))

	return mb.producer.Publish(ctx, topic, key, event)
}

// PublishBatch publishes multiple events of the same type in a batch
func (mb *MessageBus) PublishBatch(ctx context.Context, topic Topic, events []BatchMessage) error {
	mb.logger.Debug("Publishing batch",
		zap.String("topic", string(topic)),
		zap.Int("count", len(events)))

	return mb.producer.PublishBatch(ctx, topic, events)
}

// RegisterHandler registers a message handler for a specific message type
func (mb *MessageBus) RegisterHandler(msgType MessageType, handler MessageHandler) {
	mb.mu.Lock()
	defer mb.mu.Unlock()

	mb.handlers[msgType] = append(mb.handlers[msgType], handler)

	mb.logger.Info("Registered message handler",
		zap.String("type", string(msgType)))
}

// StartConsumers starts consuming messages for all registered handlers
func (mb *MessageBus) StartConsumers(groupID string) error {
	mb.mu.RLock()
	defer mb.mu.RUnlock()

	if len(mb.handlers) == 0 {
		mb.logger.Warn("No message handlers registered")
		return nil
	}

	// Group topics by the message types we handle
	topicSet := make(map[Topic]bool)
	for msgType := range mb.handlers {
		topicSet[GetTopic(msgType)] = true
	}

	topics := make([]Topic, 0, len(topicSet))
	for topic := range topicSet {
		topics = append(topics, topic)
	}

	mb.logger.Info("Starting message consumers",
		zap.String("group_id", groupID),
		zap.Int("topic_count", len(topics)),
		zap.Int("handler_count", len(mb.handlers)))

	return mb.consumer.Subscribe(mb.ctx, topics, groupID, mb.handleMessage)
}

// handleMessage processes incoming messages and routes them to registered handlers
func (mb *MessageBus) handleMessage(ctx context.Context, msg *ReceivedMessage) error {
	start := time.Now()

	// Parse the message to determine its type
	var baseMsg BaseMessage
	if err := parseJSON(msg.Value, &baseMsg); err != nil {
		mb.logger.Error("Failed to parse message",
			zap.Error(err),
			zap.String("topic", msg.Topic))
		return err
	}

	mb.mu.RLock()
	handlers, exists := mb.handlers[baseMsg.Type]
	mb.mu.RUnlock()

	if !exists {
		mb.logger.Debug("No handlers registered for message type",
			zap.String("type", string(baseMsg.Type)))
		return nil
	}

	// Execute all handlers for this message type
	var lastErr error
	for _, handler := range handlers {
		if err := handler(ctx, msg); err != nil {
			lastErr = err
			mb.logger.Error("Message handler failed",
				zap.Error(err),
				zap.String("type", string(baseMsg.Type)),
				zap.String("topic", msg.Topic))
		}
	}

	duration := time.Since(start)
	mb.logger.Debug("Message processed",
		zap.String("type", string(baseMsg.Type)),
		zap.Duration("duration", duration),
		zap.Bool("success", lastErr == nil))

	return lastErr
}

// Stop gracefully stops the message bus
func (mb *MessageBus) Stop() error {
	mb.logger.Info("Stopping message bus")

	mb.cancel()

	var producerErr, consumerErr error
	if mb.producer != nil {
		producerErr = mb.producer.Close()
	}
	if mb.consumer != nil {
		consumerErr = mb.consumer.Close()
	}

	if producerErr != nil {
		return producerErr
	}
	return consumerErr
}

// Health check methods
func (mb *MessageBus) HealthCheck() error {
	// Simple health check - could be enhanced to check Kafka connectivity
	select {
	case <-mb.ctx.Done():
		return fmt.Errorf("message bus is stopped")
	default:
		return nil
	}
}

// GetMetrics returns basic metrics about the message bus
func (mb *MessageBus) GetMetrics() map[string]interface{} {
	mb.mu.RLock()
	defer mb.mu.RUnlock()

	return map[string]interface{}{
		"handlers_registered": len(mb.handlers),
		"status":              "running",
		"uptime":              time.Since(time.Now()), // This would need proper tracking
	}
}

// Helper function to parse JSON with better error handling
func parseJSON(data []byte, v interface{}) error {
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("json unmarshal failed: %w", err)
	}
	return nil
}
