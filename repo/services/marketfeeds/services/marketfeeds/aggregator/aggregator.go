package aggregator

import (
	"log"
	"time"

	"services/marketfeeds/clients"
	"services/marketfeeds/models"
	"services/marketfeeds/publisher"

	"github.com/gorilla/websocket"
)

// Aggregator handles WebSocket data, normalization, and publishes
type Aggregator struct {
	wsURL, apiKey, secret, passphrase string
	pub                               *publisher.KafkaPublisher
}

// New constructs an Aggregator
func New(wsURL, apiKey, secret, passphrase string, pub *publisher.KafkaPublisher) *Aggregator {
	return &Aggregator{wsURL: wsURL, apiKey: apiKey, secret: secret, passphrase: passphrase, pub: pub}
}

// Run starts the aggregation loop
func (a *Aggregator) Run() {
	for {
		conn, err := clients.DialCoinbase(a.wsURL)
		if err != nil {
			log.Println("Dial error:", err)
			time.Sleep(time.Second)
			continue
		}
		a.subscribe(conn)
		a.readLoop(conn)
	}
}

func (a *Aggregator) subscribe(conn *websocket.Conn) {
	// send subscribe message to level2 channel...
}

func (a *Aggregator) readLoop(conn *websocket.Conn) {
	for {
		var msg models.Tick
		if err := conn.ReadJSON(&msg); err != nil {
			log.Println("Read error:", err)
			return
		}
		// gap detection & normalization
		a.pub.Publish("market.ticks", msg)
	}
}

// Note: Connector service consumer management moved to consumer_manager.go
