package marketdata

import (
	"net/http"
	"sync"

	"github.com/Aidin1998/pincex_unified/internal/marketdata"
	"github.com/gorilla/websocket"
	"github.com/prometheus/client_golang/prometheus"
)

// EdgeNode manages client WebSocket connections and subscriptions.
type EdgeNode struct {
	upgrader websocket.Upgrader
	clients  map[*Client]struct{}
	mu       sync.RWMutex
}

type Client struct {
	conn *websocket.Conn
	send chan []byte
	// Add fields for subscriptions, auth, etc.
}

func NewEdgeNode() *EdgeNode {
	return &EdgeNode{
		upgrader: websocket.Upgrader{},
		clients:  make(map[*Client]struct{}),
	}
}

func (e *EdgeNode) ServeWS(w http.ResponseWriter, r *http.Request) {
	conn, err := e.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	client := &Client{conn: conn, send: make(chan []byte, 256)}
	e.mu.Lock()
	e.clients[client] = struct{}{}
	e.mu.Unlock()
	go client.writePump()
	// TODO: readPump, subscription, auth, etc.
}

// Add method to subscribe to distribution tier (e.g., via PubSubBackend)
func (e *EdgeNode) SubscribeAndServe(pubsub marketdata.PubSubBackend) {
	// Example: subscribe to orderbook/ticker/trade/candle channels
	go func() {
		ch := pubsub.Subscribe("ticker")
		for msg := range ch {
			// Optionally decode, delta-encode, or binary-encode
			for client := range e.clients {
				select {
				case client.send <- msg:
				default:
					// Backpressure: drop or disconnect slow clients
				}
			}
		}
	}()
	// Repeat for other channels as needed
}

func (c *Client) writePump() {
	for msg := range c.send {
		c.conn.WriteMessage(websocket.BinaryMessage, msg)
	}
}

// Prometheus metrics for connections
var (
	ActiveConnections = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "marketdata_edge_active_connections",
			Help: "Number of active WebSocket connections",
		},
	)
)

func init() {
	prometheus.MustRegister(ActiveConnections)
}
