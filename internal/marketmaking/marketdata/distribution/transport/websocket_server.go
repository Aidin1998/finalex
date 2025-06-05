// websocket_server.go: WebSocket transport for market data distribution
package transport

import (
	"log"
	"net/http"
	"time"

	"github.com/Aidin1998/finalex/internal/marketmaking/marketdata/distribution"
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
	sub := &distribution.Subscription{ClientID: clientID, Symbol: symbol, PriceLevels: levels, Frequency: time.Second, Compression: false}
	s.sm.Subscribe(sub)
	ch := make(chan interface{}, 100)
	s.dist.RegisterClient(clientID, ch)
	defer func() {
		s.sm.Unsubscribe(clientID)
		s.dist.UnregisterClient(clientID)
	}()

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
		log.Printf("[WebSocketServer] Writing to client %s: %+v", clientID, msg)
		err := conn.WriteJSON(msg)
		if err != nil {
			log.Printf("[WebSocketServer] Write error for client %s: %v", clientID, err)
			return
		}
	}
	log.Printf("[WebSocketServer] Channel closed for client %s", clientID)
}
