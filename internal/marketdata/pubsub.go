package marketdata

import (
	"context"
	"encoding/json"
	"log"

	"github.com/go-redis/redis/v8"
	"github.com/segmentio/kafka-go"
)

// PubSubBackend abstracts pub/sub for Redis and Kafka
// Use Redis for low-latency, Kafka for persistence/scalability

type PubSubBackend interface {
	Publish(ctx context.Context, channel string, msg interface{}) error
	Subscribe(ctx context.Context, channel string, handler func([]byte)) error
}

// RedisPubSub implements PubSubBackend using Redis

type RedisPubSub struct {
	client *redis.Client
}

func NewRedisPubSub(addr string) *RedisPubSub {
	return &RedisPubSub{
		client: redis.NewClient(&redis.Options{
			Addr: addr,
		}),
	}
}

func (r *RedisPubSub) Publish(ctx context.Context, channel string, msg interface{}) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return r.client.Publish(ctx, channel, data).Err()
}

func (r *RedisPubSub) Subscribe(ctx context.Context, channel string, handler func([]byte)) error {
	pubsub := r.client.Subscribe(ctx, channel)
	ch := pubsub.Channel()
	go func() {
		for msg := range ch {
			handler([]byte(msg.Payload))
		}
	}()
	return nil
}

// KafkaPubSub implements PubSubBackend using Kafka

type KafkaPubSub struct {
	writer *kafka.Writer
	reader *kafka.Reader
}

func NewKafkaPubSub(brokers []string, topic string, groupID string) *KafkaPubSub {
	return &KafkaPubSub{
		writer: &kafka.Writer{
			Addr:     kafka.TCP(brokers...),
			Topic:    topic,
			Balancer: &kafka.LeastBytes{},
		},
		reader: kafka.NewReader(kafka.ReaderConfig{
			Brokers: brokers,
			Topic:   topic,
			GroupID: groupID,
		}),
	}
}

func (k *KafkaPubSub) Publish(ctx context.Context, channel string, msg interface{}) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return k.writer.WriteMessages(ctx, kafka.Message{Value: data})
}

func (k *KafkaPubSub) Subscribe(ctx context.Context, channel string, handler func([]byte)) error {
	go func() {
		for {
			m, err := k.reader.ReadMessage(ctx)
			if err != nil {
				log.Println("Kafka read error:", err)
				break
			}
			handler(m.Value)
		}
	}()
	return nil
}

// Example usage:
// var pubsub PubSubBackend = NewRedisPubSub("localhost:6379")
// or
// var pubsub PubSubBackend = NewKafkaPubSub([]string{"localhost:9092"}, "marketdata", "group1")
