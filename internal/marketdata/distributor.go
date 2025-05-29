package marketdata

import (
	"context"
	"log"
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

// Example initialization (to be called from main or server setup):
// var pubsub marketdata.PubSubBackend = marketdata.NewRedisPubSub(os.Getenv("REDIS_ADDR"))
// or
// var pubsub marketdata.PubSubBackend = marketdata.NewKafkaPubSub([]string{"localhost:9092"}, "marketdata", "group1")
// hub := marketdata.NewHub()
// go hub.Run()
// distributor := marketdata.NewMarketDataDistributor(hub, pubsub)
// go distributor.Start()
