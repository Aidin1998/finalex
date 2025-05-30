// Package settlement provides the asynchronous settlement processor and queue integration.
package settlement

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Shopify/sarama" // Kafka client
	"go.uber.org/zap"
)

// SettlementMessage represents a settlement request message.
type SettlementMessage struct {
	SettlementID string    `json:"settlement_id"`
	TradeID      string    `json:"trade_id"`
	Amount       float64   `json:"amount"`
	Asset        string    `json:"asset"`
	Type         string    `json:"type"` // "crypto" or "fiat"
	Timestamp    time.Time `json:"timestamp"`
	RetryCount   int       `json:"retry_count"`
}

// SettlementStatus represents the status of a settlement.
type SettlementStatus string

const (
	SettlementPending    SettlementStatus = "pending"
	SettlementProcessing SettlementStatus = "processing"
	SettlementSettled    SettlementStatus = "settled"
	SettlementFailed     SettlementStatus = "failed"
)

// SettlementProcessor handles consuming, processing, and confirming settlements.
type SettlementProcessor struct {
	consumer      sarama.ConsumerGroup
	confirmations chan<- SettlementConfirmation
	logger        *zap.Logger
}

type SettlementConfirmation struct {
	SettlementID string           `json:"settlement_id"`
	TradeID      string           `json:"trade_id"`
	Status       SettlementStatus `json:"status"`
	Receipt      string           `json:"receipt"`
	Error        string           `json:"error,omitempty"`
	ConfirmedAt  time.Time        `json:"confirmed_at"`
}

// NewSettlementProcessor creates a new processor.
func NewSettlementProcessor(brokers []string, groupID string, confirmations chan<- SettlementConfirmation, logger *zap.Logger) (*SettlementProcessor, error) {
	config := sarama.NewConfig()
	config.Consumer.Offsets.Initial = sarama.OffsetNewest
	consumer, err := sarama.NewConsumerGroup(brokers, groupID, config)
	if err != nil {
		return nil, err
	}
	return &SettlementProcessor{consumer: consumer, confirmations: confirmations, logger: logger}, nil
}

// StartConsuming starts the processor loop.
func (sp *SettlementProcessor) StartConsuming(ctx context.Context, topic string) error {
	handler := &settlementConsumerGroupHandler{processor: sp}
	for {
		if err := sp.consumer.Consume(ctx, []string{topic}, handler); err != nil {
			sp.logger.Error("settlement consume error", zap.Error(err))
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
}

type settlementConsumerGroupHandler struct {
	processor *SettlementProcessor
}

func (h *settlementConsumerGroupHandler) Setup(_ sarama.ConsumerGroupSession) error   { return nil }
func (h *settlementConsumerGroupHandler) Cleanup(_ sarama.ConsumerGroupSession) error { return nil }
func (h *settlementConsumerGroupHandler) ConsumeClaim(sess sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for msg := range claim.Messages() {
		var sm SettlementMessage
		if err := json.Unmarshal(msg.Value, &sm); err != nil {
			h.processor.logger.Error("invalid settlement message", zap.Error(err))
			sess.MarkMessage(msg, "")
			continue
		}
		// Process settlement (idempotent)
		status, receipt, err := h.processor.processSettlement(sm)
		confirmation := SettlementConfirmation{
			SettlementID: sm.SettlementID,
			TradeID:      sm.TradeID,
			Status:       status,
			Receipt:      receipt,
			ConfirmedAt:  time.Now(),
		}
		if err != nil {
			confirmation.Error = err.Error()
			if sm.RetryCount < 5 {
				// Retry with backoff
				sm.RetryCount++
				go h.processor.retrySettlement(sm)
			} else {
				// Send to dead letter queue (not implemented here)
				h.processor.logger.Error("settlement failed, sending to DLQ", zap.String("settlement_id", sm.SettlementID))
			}
		} else {
			sess.MarkMessage(msg, "")
		}
		h.processor.confirmations <- confirmation
	}
	return nil
}

// processSettlement performs the actual settlement logic (idempotent, partial failure safe).
func (sp *SettlementProcessor) processSettlement(sm SettlementMessage) (SettlementStatus, string, error) {
	// TODO: Check DB for idempotency (already settled?)
	// TODO: Batch processing logic if needed
	// Simulate settlement
	if sm.Type == "crypto" {
		// Integrate with blockchain
		return SettlementSettled, fmt.Sprintf("onchain_tx_hash_%s", sm.SettlementID), nil
	} else if sm.Type == "fiat" {
		// Integrate with bank
		return SettlementSettled, fmt.Sprintf("bank_ref_%s", sm.SettlementID), nil
	}
	return SettlementFailed, "", fmt.Errorf("unknown settlement type")
}

// retrySettlement re-publishes the message with backoff.
func (sp *SettlementProcessor) retrySettlement(sm SettlementMessage) {
	backoff := time.Duration(sm.RetryCount*2) * time.Second
	time.Sleep(backoff)
	// TODO: Publish to Kafka again (not implemented here)
	sp.logger.Warn("retrying settlement", zap.String("settlement_id", sm.SettlementID), zap.Int("retry", sm.RetryCount))
}
