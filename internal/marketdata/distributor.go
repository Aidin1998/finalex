package marketdata

import (
	"context"
	"log"
	"sync"
)

// MarketDataDistributor wires pubsub to the WebSocket hub

type MarketDataDistributor struct {
	Hub    *Hub
	PubSub PubSubBackend
	Ctx    context.Context
}

func NewMarketDataDistributor(hub *Hub, pubsub PubSubBackend) *MarketDataDistributor {
	return &MarketDataDistributor{
		Hub:    hub,
		PubSub: pubsub,
		Ctx:    context.Background(),
	}
}

// Start subscribes to all relevant channels and broadcasts to clients
func (d *MarketDataDistributor) Start() {
	channels := []string{"orderbook", "trade", "ticker", "candle"}
	for _, ch := range channels {
		chName := ch // capture loop var
		err := d.PubSub.Subscribe(d.Ctx, chName, func(msg []byte) {
			d.Hub.BroadcastMarketDataRaw(msg)
		})
		if err != nil {
			log.Printf("Failed to subscribe to %s: %v", chName, err)
		}
	}
	log.Println("MarketDataDistributor started.")
}

// BroadcastMarketDataRaw allows direct broadcast of already-encoded messages
func (h *Hub) BroadcastMarketDataRaw(msg []byte) {
	// Use sharded broadcast method
	shard := hashMessageShard(msg)
	h.broadcastShards[shard] <- msg
}

// Efficient channel usage for burst traffic
// Use buffered channels sized for burst traffic, non-blocking sends, and fan-out pattern

// Example: Efficient broadcaster for market data
// (This can be adapted for any high-throughput event distribution)
type Broadcaster struct {
	subscribers []chan []byte
	mu          sync.Mutex
}

func NewBroadcaster() *Broadcaster {
	return &Broadcaster{
		subscribers: make([]chan []byte, 0),
	}
}

// Subscribe returns a buffered channel for a new subscriber
func (b *Broadcaster) Subscribe(buffer int) <-chan []byte {
	ch := make(chan []byte, buffer)
	b.mu.Lock()
	b.subscribers = append(b.subscribers, ch)
	b.mu.Unlock()
	return ch
}

// Publish sends data to all subscribers using non-blocking sends
func (b *Broadcaster) Publish(data []byte) {
	b.mu.Lock()
	for _, ch := range b.subscribers {
		select {
		case ch <- data:
			// sent successfully
		default:
			// channel full, drop or handle overflow
		}
	}
	b.mu.Unlock()
}

// Example initialization (to be called from main or server setup):
// var pubsub marketdata.PubSubBackend = marketdata.NewRedisPubSub(os.Getenv("REDIS_ADDR"))
// or
// var pubsub marketdata.PubSubBackend = marketdata.NewKafkaPubSub([]string{"localhost:9092"}, "marketdata", "group1")
// hub := marketdata.NewHub()
// go hub.Run()
// distributor := marketdata.NewMarketDataDistributor(hub, pubsub)
// go distributor.Start()
