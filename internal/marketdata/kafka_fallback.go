package marketdata

import (
	"context"

	"github.com/segmentio/kafka-go"
)

// FetchMissedKafkaMessages fetches missed messages from Kafka by offset
func FetchMissedKafkaMessages(brokers []string, topic string, partition int, startOffset int64, maxMessages int) ([][]byte, int64, error) {
	ctx := context.Background()
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:   brokers,
		Topic:     topic,
		Partition: partition,
		MinBytes:  1,
		MaxBytes:  10e6, // 10MB
	})
	defer r.Close()
	if err := r.SetOffset(startOffset); err != nil {
		return nil, startOffset, err
	}
	var messages [][]byte
	var lastOffset = startOffset
	for i := 0; i < maxMessages; i++ {
		m, err := r.ReadMessage(ctx)
		if err != nil {
			break
		}
		messages = append(messages, m.Value)
		lastOffset = m.Offset + 1
	}
	return messages, lastOffset, nil
}
