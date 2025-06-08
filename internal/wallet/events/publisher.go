// Package events provides event publishing for the wallet service
package events

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/Aidin1998/finalex/internal/wallet/interfaces"
	"github.com/Aidin1998/finalex/pkg/logger"
)

// EventPublisher handles publishing wallet events to multiple destinations
type EventPublisher struct {
	publishers []Publisher
	log        logger.Logger
}

// Publisher defines the interface for event publishers
type Publisher interface {
	PublishEvent(ctx context.Context, topic string, event interface{}) error
}

// NewEventPublisher creates a new event publisher
func NewEventPublisher(publishers []Publisher, log logger.Logger) *EventPublisher {
	return &EventPublisher{
		publishers: publishers,
		log:        log,
	}
}

// PublishWalletEvent publishes a wallet event to all configured publishers
func (p *EventPublisher) PublishWalletEvent(ctx context.Context, event *interfaces.WalletEvent) error {
	if event == nil {
		return fmt.Errorf("event cannot be nil")
	}

	if event.TxID != nil && *event.TxID == uuid.Nil {
		return fmt.Errorf("transaction ID is required")
	}

	var lastErr error
	successCount := 0

	for i, publisher := range p.publishers {
		if err := publisher.PublishEvent(ctx, "wallet.events", event); err != nil {
			p.log.Error("failed to publish event",
				zap.Int("publisher_index", i),
				zap.String("event_type", event.Type),
				zap.String("event_id", p.getTxIDString(event.TxID)),
				zap.Error(err),
			)
			lastErr = err
		} else {
			successCount++
		}
	}

	// Log event publication
	p.log.Info("published wallet event",
		zap.String("event_type", event.Type),
		zap.String("user_id", event.UserID.String()),
		zap.String("tx_id", p.getTxIDString(event.TxID)),
		zap.String("asset", event.Asset),
		zap.String("direction", string(event.Direction)),
		zap.String("status", event.Status),
		zap.Int("publishers_success", successCount),
		zap.Int("publishers_total", len(p.publishers)),
	)

	// Return error only if all publishers failed
	if successCount == 0 && lastErr != nil {
		return fmt.Errorf("all publishers failed, last error: %w", lastErr)
	}

	return nil
}

// getTxIDString safely converts *uuid.UUID to string
func (p *EventPublisher) getTxIDString(txID *uuid.UUID) string {
	if txID == nil {
		return ""
	}
	return txID.String()
}

// PublishTransactionEvent publishes a transaction-related event
func (p *EventPublisher) PublishTransactionEvent(
	ctx context.Context,
	tx *interfaces.WalletTransaction,
	eventType string,
	metadata map[string]interface{},
) error {
	txIDPtr := tx.ID
	amountPtr := tx.Amount

	event := &interfaces.WalletEvent{
		ID:        uuid.New(),
		Type:      eventType,
		EventType: eventType,
		UserID:    tx.UserID,
		Asset:     tx.Asset,
		Amount:    &amountPtr,
		Direction: tx.Direction,
		TxID:      &txIDPtr,
		Status:    string(tx.Status),
		Message:   fmt.Sprintf("Transaction %s", eventType),
		Metadata:  metadata,
		Timestamp: time.Now(),
	}

	return p.PublishWalletEvent(ctx, event)
}

// PublishBalanceEvent publishes a balance-related event
func (p *EventPublisher) PublishBalanceEvent(
	ctx context.Context,
	userID uuid.UUID,
	asset string,
	balance *interfaces.WalletBalance,
	eventType string,
) error {
	balancePtr := balance.Total
	genIDPtr := uuid.New()

	event := &interfaces.WalletEvent{
		ID:        uuid.New(),
		Type:      eventType,
		EventType: eventType,
		UserID:    userID,
		Asset:     asset,
		Amount:    &balancePtr,
		Direction: "",
		TxID:      &genIDPtr,
		Status:    "completed",
		Message:   fmt.Sprintf("Balance %s for %s", eventType, asset),
		Metadata: map[string]interface{}{
			"available": balance.Available.String(),
			"locked":    balance.Locked.String(),
			"total":     balance.Total.String(),
		},
		Timestamp: time.Now(),
	}

	return p.PublishWalletEvent(ctx, event)
}

// PublishWithdrawalEvent publishes a withdrawal-related event
func (p *EventPublisher) PublishWithdrawalEvent(
	ctx context.Context,
	userID uuid.UUID,
	txID uuid.UUID,
	asset string,
	amount decimal.Decimal,
	address string,
	eventType string,
) error {
	amountPtr := amount
	txIDPtr := txID

	event := &interfaces.WalletEvent{
		ID:        uuid.New(),
		Type:      eventType,
		EventType: eventType,
		UserID:    userID,
		Asset:     asset,
		Amount:    &amountPtr,
		Direction: interfaces.DirectionWithdrawal,
		TxID:      &txIDPtr,
		Status:    "pending",
		Message:   fmt.Sprintf("Withdrawal %s", eventType),
		Metadata: map[string]interface{}{
			"destination_address": address,
			"amount":              amount.String(),
		},
		Timestamp: time.Now(),
	}

	return p.PublishWalletEvent(ctx, event)
}

// KafkaPublisher implements Publisher for Apache Kafka
type KafkaPublisher struct {
	brokers []string
	writer  *kafka.Writer
	log     logger.Logger
}

// NewKafkaPublisher creates a new Kafka publisher
func NewKafkaPublisher(brokers []string, log logger.Logger) *KafkaPublisher {
	return &KafkaPublisher{
		brokers: brokers,
		log:     log,
	}
}

// PublishEvent publishes an event to Kafka
func (k *KafkaPublisher) PublishEvent(ctx context.Context, topic string, event interface{}) error {
	eventData, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	k.log.Debug("publishing event to kafka",
		zap.String("topic", topic),
		zap.Int("event_size", len(eventData)),
	)

	// Create Kafka writer if not exists
	if k.writer == nil {
		k.writer = &kafka.Writer{
			Addr:         kafka.TCP(k.brokers...),
			Topic:        topic,
			Balancer:     &kafka.CRC32Balancer{},
			BatchSize:    100,
			BatchTimeout: 10 * time.Millisecond,
			RequiredAcks: kafka.RequireOne,
			MaxAttempts:  3,
		}
	}

	// Generate event key for partitioning
	eventKey := fmt.Sprintf("%s-%d", topic, time.Now().UnixNano())

	msg := kafka.Message{
		Key:   []byte(eventKey),
		Value: eventData,
		Time:  time.Now(),
		Headers: []kafka.Header{
			{Key: "event-type", Value: []byte(topic)},
			{Key: "timestamp", Value: []byte(time.Now().Format(time.RFC3339))},
		},
	}

	return k.writer.WriteMessages(ctx, msg)
}

// RedisPublisher implements Publisher for Redis Streams
type RedisPublisher struct {
	addr   string
	client *redis.Client
	log    logger.Logger
}

// NewRedisPublisher creates a new Redis publisher
func NewRedisPublisher(addr string, log logger.Logger) *RedisPublisher {
	client := redis.NewClient(&redis.Options{
		Addr: addr,
	})

	return &RedisPublisher{
		addr:   addr,
		client: client,
		log:    log,
	}
}

// PublishEvent publishes an event to Redis Streams
func (r *RedisPublisher) PublishEvent(ctx context.Context, topic string, event interface{}) error {
	eventData, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	r.log.Debug("publishing event to redis stream",
		zap.String("stream", topic),
		zap.Int("event_size", len(eventData)),
	)

	// Create stream entry
	streamKey := fmt.Sprintf("wallet.events.%s", topic)
	eventID := fmt.Sprintf("%d-*", time.Now().UnixMilli())

	fields := map[string]interface{}{
		"event_type": topic,
		"data":       string(eventData),
		"timestamp":  time.Now().Format(time.RFC3339),
		"source":     "wallet-service",
	}

	// Add to Redis Stream with automatic ID generation
	result := r.client.XAdd(ctx, &redis.XAddArgs{
		Stream: streamKey,
		ID:     eventID,
		Values: fields,
	})

	if err := result.Err(); err != nil {
		r.log.Error("failed to publish event to redis stream",
			zap.String("stream", streamKey),
			zap.Error(err))
		return fmt.Errorf("failed to publish to redis stream: %w", err)
	}

	r.log.Debug("successfully published event to redis stream",
		zap.String("stream", streamKey),
		zap.String("message_id", result.Val()))

	return nil
}

// WebhookPublisher implements Publisher for HTTP webhooks
type WebhookPublisher struct {
	webhookURL string
	log        logger.Logger
}

// NewWebhookPublisher creates a new webhook publisher
func NewWebhookPublisher(webhookURL string, log logger.Logger) *WebhookPublisher {
	return &WebhookPublisher{
		webhookURL: webhookURL,
		log:        log,
	}
}

// PublishEvent publishes an event via HTTP webhook
func (w *WebhookPublisher) PublishEvent(ctx context.Context, topic string, event interface{}) error {
	eventData, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	w.log.Debug("publishing event to webhook",
		zap.String("url", w.webhookURL),
		zap.String("topic", topic),
		zap.Int("event_size", len(eventData)),
	)

	// Create webhook payload
	payload := map[string]interface{}{
		"topic":     topic,
		"event":     event,
		"timestamp": time.Now().Format(time.RFC3339),
		"source":    "wallet-service",
	}

	payloadData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal webhook payload: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", w.webhookURL, bytes.NewBuffer(payloadData))
	if err != nil {
		return fmt.Errorf("failed to create webhook request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Event-Type", topic)
	req.Header.Set("X-Source", "wallet-service")

	// Send request with timeout
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		w.log.Error("failed to send webhook",
			zap.String("url", w.webhookURL),
			zap.Error(err))
		return fmt.Errorf("failed to send webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		w.log.Error("webhook returned error status",
			zap.String("url", w.webhookURL),
			zap.Int("status_code", resp.StatusCode))
		return fmt.Errorf("webhook returned status code: %d", resp.StatusCode)
	}

	w.log.Debug("successfully sent webhook",
		zap.String("url", w.webhookURL),
		zap.Int("status_code", resp.StatusCode))

	return nil
}
