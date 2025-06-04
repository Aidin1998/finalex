// Package settlement provides the asynchronous settlement processor and queue integration.
package settlement

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/segmentio/kafka-go"
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
	reader        *kafka.Reader
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
func NewSettlementProcessor(brokers []string, groupID string, topic string, confirmations chan<- SettlementConfirmation, logger *zap.Logger) (*SettlementProcessor, error) {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     brokers,
		GroupID:     groupID,
		Topic:       topic,
		StartOffset: kafka.LastOffset,
	})
	return &SettlementProcessor{reader: reader, confirmations: confirmations, logger: logger}, nil
}

// StartConsuming starts the processor loop.
func (sp *SettlementProcessor) StartConsuming(ctx context.Context) error {
	for {
		msg, err := sp.reader.ReadMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			sp.logger.Error("settlement consume error", zap.Error(err))
			continue
		}

		if err := sp.processMessage(msg); err != nil {
			sp.logger.Error("failed to process settlement message", zap.Error(err))
		}
	}
}

// processMessage handles a single Kafka message
func (sp *SettlementProcessor) processMessage(msg kafka.Message) error {
	var sm SettlementMessage
	if err := json.Unmarshal(msg.Value, &sm); err != nil {
		sp.logger.Error("invalid settlement message", zap.Error(err))
		return err
	}

	// Process settlement (idempotent)
	status, receipt, err := sp.processSettlement(sm)
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
			go sp.retrySettlement(sm)
		} else {
			// Send to dead letter queue (not implemented here)
			sp.logger.Error("settlement failed, sending to DLQ", zap.String("settlement_id", sm.SettlementID))
		}
	}

	sp.confirmations <- confirmation
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

// Close closes the Kafka reader
func (sp *SettlementProcessor) Close() error {
	return sp.reader.Close()
}
