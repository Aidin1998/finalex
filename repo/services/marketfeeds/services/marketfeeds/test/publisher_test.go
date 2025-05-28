package test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"services/marketfeeds/models"

	"github.com/segmentio/kafka-go"
)

// fakeWriter implements the same methods as *kafka.Writer
type fakeWriter struct {
	msgs []kafka.Message
	err  error
}

func (f *fakeWriter) WriteMessages(ctx context.Context, m ...kafka.Message) error {
	f.msgs = append(f.msgs, m...)
	return f.err
}

func (f *fakeWriter) Close() error {
	return nil
}

// KafkaPublisher is a minimal stub for testing purposes.
type KafkaPublisher struct {
	writer interface {
		WriteMessages(ctx context.Context, m ...kafka.Message) error
		Close() error
	}
}

func (p *KafkaPublisher) Publish(topic string, tick models.Tick) error {
	msg := kafka.Message{
		Key:   []byte(tick.Symbol),
		Value: []byte(tick.String()),
	}
	return p.writer.WriteMessages(context.Background(), msg)
}

func (p *KafkaPublisher) PublishAlert(exchange, symbol, errStr string) error {
	payload, _ := json.Marshal(map[string]string{
		"exchange": exchange,
		"symbol":   symbol,
		"error":    errStr,
	})
	msg := kafka.Message{
		Topic: "market.alerts",
		Value: payload,
	}
	return p.writer.WriteMessages(context.Background(), msg)
}

func (p *KafkaPublisher) PublishAggregated(symbol string, avgPrice float64, ts time.Time, fallback bool) error {
	payload, _ := json.Marshal(map[string]interface{}{
		"symbol":       symbol,
		"averagePrice": avgPrice,
		"timestamp":    ts.Format(time.RFC3339),
		"fallback":     fallback,
	})
	msg := kafka.Message{
		Topic: "market.aggregated",
		Value: payload,
	}
	return p.writer.WriteMessages(context.Background(), msg)
}

func (p *KafkaPublisher) Close() error {
	return p.writer.Close()
}

func newTestPublisher() (*KafkaPublisher, *fakeWriter) {
	fw := &fakeWriter{}
	pub := &KafkaPublisher{writer: fw}
	return pub, fw
}

func TestPublishTick(t *testing.T) {
	pub, fw := newTestPublisher()
	tick := models.Tick{Symbol: "BTC/USDT", Price: 1234.5, Timestamp: time.Now()}
	pub.Publish("market.ticks", tick)

	if len(fw.msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(fw.msgs))
	}
	m := fw.msgs[0]
	if string(m.Key) != tick.Symbol {
		t.Errorf("key = %q; want %q", m.Key, tick.Symbol)
	}
	if string(m.Value) != tick.String() {
		t.Errorf("value = %q; want %q", m.Value, tick.String())
	}
	if m.Topic != "" {
		t.Errorf("default Topic should be empty on message (uses config), got %q", m.Topic)
	}
}

func TestPublishAlert(t *testing.T) {
	pub, fw := newTestPublisher()
	pub.PublishAlert("binance", "ETH/USDT", "oops")

	if len(fw.msgs) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(fw.msgs))
	}
	m := fw.msgs[0]
	if m.Topic != "market.alerts" {
		t.Errorf("Topic = %q; want %q", m.Topic, "market.alerts")
	}
	var alert map[string]string
	if err := json.Unmarshal(m.Value, &alert); err != nil {
		t.Fatal(err)
	}
	if alert["exchange"] != "binance" || alert["symbol"] != "ETH/USDT" || alert["error"] != "oops" {
		t.Errorf("unexpected alert payload: %#v", alert)
	}
}

func TestPublishAggregated(t *testing.T) {
	pub, fw := newTestPublisher()
	ts := time.Unix(1_600_000_000, 0)
	pub.PublishAggregated("ADA/USDT", 42.42, ts, true)

	if len(fw.msgs) != 1 {
		t.Fatalf("expected 1 aggregated msg, got %d", len(fw.msgs))
	}
	m := fw.msgs[0]
	if m.Topic != "market.aggregated" {
		t.Errorf("Topic = %q; want %q", m.Topic, "market.aggregated")
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(m.Value, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["symbol"] != "ADA/USDT" {
		t.Errorf("symbol = %v; want %v", payload["symbol"], "ADA/USDT")
	}
	if payload["averagePrice"] != 42.42 {
		t.Errorf("averagePrice = %v; want %v", payload["averagePrice"], 42.42)
	}
	if payload["fallback"] != true {
		t.Errorf("fallback = %v; want %v", payload["fallback"], true)
	}
	if payload["timestamp"] != ts.Format(time.RFC3339) {
		t.Errorf("timestamp = %v; want %v", payload["timestamp"], ts.Format(time.RFC3339))
	}
}

func TestClose(t *testing.T) {
	pub, _ := newTestPublisher()
	if err := pub.Close(); err != nil {
		t.Errorf("Close() error = %v; want nil", err)
	}
}
