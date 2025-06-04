package marketdata

import (
	"context"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/prometheus/client_golang/prometheus"
)

// EdgeNode manages client WebSocket connections and subscriptions.
type EdgeNode struct {
	upgrader websocket.Upgrader
	clients  map[*Client]struct{}
	mu       sync.RWMutex
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
func (e *EdgeNode) SubscribeAndServe(pubsub PubSubBackend) {
	// Example: subscribe to orderbook/ticker/trade/candle channels
	handler := func(msg []byte) {
		e.mu.RLock()
		defer e.mu.RUnlock()
		for client := range e.clients {
			select {
			case client.send <- msg:
			default:
				// Backpressure: drop or disconnect slow clients
			}
		}
	}
	// Subscribe to ticker channel
	_ = pubsub.Subscribe(context.Background(), "ticker", handler)
	// Repeat for other channels as needed
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

type EdgeConn struct {
	// Placeholder for real outbound WebSocket/TCP connection (e.g., *websocket.Conn)
}

var edgeConnPool = sync.Pool{
	New: func() interface{} { return &EdgeConn{} },
}

// AcquireEdgeConn gets a connection from the pool (or creates new)
func AcquireEdgeConn() *EdgeConn {
	return edgeConnPool.Get().(*EdgeConn)
}

// ReleaseEdgeConn returns a connection to the pool
func ReleaseEdgeConn(conn *EdgeConn) {
	edgeConnPool.Put(conn)
}
