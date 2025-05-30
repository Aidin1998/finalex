// websocket_server.go: WebSocket transport for market data distribution
package transport

import (
	"net/http"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/marketdata/distribution"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

// WebSocketServer serves market data updates over WebSocket
type WebSocketServer struct {
	dist *distribution.Distributor
	sm   *distribution.SubscriptionManager
}

// NewWebSocketServer creates a new WebSocketServer
func NewWebSocketServer(dist *distribution.Distributor, sm *distribution.SubscriptionManager) *WebSocketServer {
	return &WebSocketServer{dist: dist, sm: sm}
}

// ServeHTTP handles WebSocket upgrade and subscription requests
func (s *WebSocketServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		http.Error(w, "upgrade error", http.StatusInternalServerError)
		return
	}
	defer conn.Close()

	// Parse query parameters for subscription
	symbol := r.URL.Query().Get("symbol")
	levels := []int{0} // TODO: parse levels
	clientID := conn.RemoteAddr().String()
	sub := &distribution.Subscription{ClientID: clientID, Symbol: symbol, PriceLevels: levels, Frequency: time.Second, Compression: true}
	s.sm.Subscribe(sub)
	ch := make(chan interface{}, 100)
	s.dist.RegisterClient(clientID, ch)
	defer s.dist.UnregisterClient(clientID)

	// Read loop for client messages (e.g., change subscription)
	go func() {
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			// TODO: handle subscription update messages
			_ = msg
		}
	}()

	// Write loop
	for msg := range ch {
		err := conn.WriteJSON(msg)
		if err != nil {
			return
		}
	}
}
