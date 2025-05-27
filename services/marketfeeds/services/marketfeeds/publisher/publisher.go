package publisher

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"services/marketfeeds/models"

	"github.com/segmentio/kafka-go"
)

// KafkaPublisher wraps a Kafka writer
type KafkaPublisher struct {
	writer *kafka.Writer
}

// NewKafkaPublisher creates a new KafkaPublisher
func NewKafkaPublisher(broker string) *KafkaPublisher {
	w := kafka.NewWriter(kafka.WriterConfig{
		Brokers: []string{broker},
		Topic:   "market.ticks",
	})
	return &KafkaPublisher{writer: w}
}

// Publish sends a tick message to Kafka
func (p *KafkaPublisher) Publish(topic string, tick models.Tick) {
	err := p.writer.WriteMessages(context.Background(),
		kafka.Message{Key: []byte(tick.Symbol), Value: []byte(tick.String())},
	)
	if err != nil {
		log.Println("Kafka publish error:", err)
	}
}

// PublishAlert sends an alert message to Kafka on topic 'market.alerts'
func (p *KafkaPublisher) PublishAlert(exchange, symbol, errStr string) {
	alert := map[string]string{"exchange": exchange, "symbol": symbol, "error": errStr}
	b, _ := json.Marshal(alert)
	err := p.writer.WriteMessages(context.Background(),
		kafka.Message{Topic: "market.alerts", Value: b},
	)
	if err != nil {
		log.Println("Kafka alert error:", err)
	}
}

// PublishAggregated sends aggregated price to Kafka on topic 'market.aggregated'
func (p *KafkaPublisher) PublishAggregated(symbol string, avg float64, ts time.Time, fallback bool) {
	payload := map[string]interface{}{"symbol": symbol, "averagePrice": avg, "timestamp": ts.Format(time.RFC3339), "fallback": fallback}
	b, _ := json.Marshal(payload)
	err := p.writer.WriteMessages(context.Background(),
		kafka.Message{Topic: "market.aggregated", Value: b},
	)
	if err != nil {
		log.Println("Kafka aggregated publish error:", err)
	}
}

// Close shuts down the Kafka writer
func (p *KafkaPublisher) Close() error {
	return p.writer.Close()
}
