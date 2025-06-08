package crosspair

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// WebSocketManager handles real-time connections for cross-pair trading
type WebSocketManager struct {
	upgrader     websocket.Upgrader
	clients      map[string]*WebSocketClient
	clientsMutex sync.RWMutex
	rateCalc     RateCalculator
	engine       *CrossPairEngine

	// Rate subscription channels
	rateUpdates  chan RateUpdate
	orderUpdates chan OrderUpdate
	tradeUpdates chan TradeUpdate
}

// WebSocketClient represents a connected WebSocket client
type WebSocketClient struct {
	ID            string
	UserID        uuid.UUID
	Conn          *websocket.Conn
	Send          chan []byte
	Subscriptions map[string]bool // subscription type -> enabled
	LastPing      time.Time
	mu            sync.RWMutex
}

// Message types for WebSocket communication
type WSMessage struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

type WSSubscribeMessage struct {
	Type         string   `json:"type"`
	Subscription string   `json:"subscription"`
	Pairs        []string `json:"pairs,omitempty"`
	UserID       string   `json:"user_id,omitempty"`
}

type WSUnsubscribeMessage struct {
	Type         string `json:"type"`
	Subscription string `json:"subscription"`
}

// Update message types
type RateUpdate struct {
	Type       string      `json:"type"`
	Pair       string      `json:"pair"`
	BuyRate    float64     `json:"buy_rate"`
	SellRate   float64     `json:"sell_rate"`
	Confidence float64     `json:"confidence"`
	Route      interface{} `json:"route"`
	Timestamp  time.Time   `json:"timestamp"`
}

type OrderUpdate struct {
	Type      string          `json:"type"`
	OrderID   uuid.UUID       `json:"order_id"`
	UserID    uuid.UUID       `json:"user_id"`
	Status    OrderStatus     `json:"status"`
	Order     *CrossPairOrder `json:"order,omitempty"`
	Timestamp time.Time       `json:"timestamp"`
}

type TradeUpdate struct {
	Type      string          `json:"type"`
	TradeID   uuid.UUID       `json:"trade_id"`
	OrderID   uuid.UUID       `json:"order_id"`
	UserID    uuid.UUID       `json:"user_id"`
	Trade     *CrossPairTrade `json:"trade"`
	Timestamp time.Time       `json:"timestamp"`
}

// NewWebSocketManager creates a new WebSocket manager
func NewWebSocketManager(rateCalc RateCalculator, engine *CrossPairEngine, allowedOrigins []string) *WebSocketManager {
	originMap := make(map[string]struct{})
	for _, o := range allowedOrigins {
		originMap[o] = struct{}{}
	}
	return &WebSocketManager{
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				origin := r.Header.Get("Origin")
				_, ok := originMap[origin]
				return ok
			},
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		},
		clients:      make(map[string]*WebSocketClient),
		rateCalc:     rateCalc,
		engine:       engine,
		rateUpdates:  make(chan RateUpdate, 1000),
		orderUpdates: make(chan OrderUpdate, 1000),
		tradeUpdates: make(chan TradeUpdate, 1000),
	}
}

// Start initializes the WebSocket manager and starts background goroutines
func (wsm *WebSocketManager) Start(ctx context.Context) {
	// Start rate update subscriber
	go wsm.handleRateUpdates(ctx)

	// Start order update subscriber
	go wsm.handleOrderUpdates(ctx)

	// Start trade update subscriber
	go wsm.handleTradeUpdates(ctx)

	// Start cleanup routine
	go wsm.cleanupRoutine(ctx)

	// Subscribe to rate calculator updates
	if wsm.rateCalc != nil {
		wsm.rateCalc.Subscribe(func(pair string, buyRate, sellRate float64, confidence float64, route interface{}) {
			select {
			case wsm.rateUpdates <- RateUpdate{
				Type:       "rate_update",
				Pair:       pair,
				BuyRate:    buyRate,
				SellRate:   sellRate,
				Confidence: confidence,
				Route:      route,
				Timestamp:  time.Now(),
			}:
			default:
				// Channel full, skip update
			}
		})
	}
}

// HandleWebSocket handles WebSocket connections
func (wsm *WebSocketManager) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Extract user ID from query parameters or headers
	userIDStr := r.URL.Query().Get("user_id")
	if userIDStr == "" {
		http.Error(w, "user_id required", http.StatusBadRequest)
		return
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		http.Error(w, "invalid user_id", http.StatusBadRequest)
		return
	}

	conn, err := wsm.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}

	clientID := uuid.New().String()
	client := &WebSocketClient{
		ID:            clientID,
		UserID:        userID,
		Conn:          conn,
		Send:          make(chan []byte, 256),
		Subscriptions: make(map[string]bool),
		LastPing:      time.Now(),
	}

	wsm.clientsMutex.Lock()
	wsm.clients[clientID] = client
	wsm.clientsMutex.Unlock()

	// Start client goroutines
	go wsm.writePump(client)
	go wsm.readPump(client)

	log.Printf("WebSocket client connected: %s (user: %s)", clientID, userID)
}

// readPump handles incoming messages from WebSocket clients
func (wsm *WebSocketManager) readPump(client *WebSocketClient) {
	defer func() {
		wsm.removeClient(client.ID)
		client.Conn.Close()
	}()

	client.Conn.SetReadLimit(512)
	client.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	client.Conn.SetPongHandler(func(string) error {
		client.LastPing = time.Now()
		client.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, message, err := client.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		wsm.handleClientMessage(client, message)
	}
}

// writePump handles outgoing messages to WebSocket clients
func (wsm *WebSocketManager) writePump(client *WebSocketClient) {
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
		ticker.Stop()
		client.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-client.Send:
			client.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				client.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := client.Conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}

		case <-ticker.C:
			client.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := client.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// handleClientMessage processes incoming messages from clients
func (wsm *WebSocketManager) handleClientMessage(client *WebSocketClient, message []byte) {
	var msg map[string]interface{}
	if err := json.Unmarshal(message, &msg); err != nil {
		log.Printf("Invalid JSON message from client %s: %v", client.ID, err)
		return
	}

	msgType, ok := msg["type"].(string)
	if !ok {
		log.Printf("Message missing type from client %s", client.ID)
		return
	}

	switch msgType {
	case "subscribe":
		wsm.handleSubscribe(client, msg)
	case "unsubscribe":
		wsm.handleUnsubscribe(client, msg)
	case "ping":
		wsm.sendToClient(client, WSMessage{Type: "pong", Data: time.Now()})
	default:
		log.Printf("Unknown message type from client %s: %s", client.ID, msgType)
	}
}

// handleSubscribe processes subscription requests
func (wsm *WebSocketManager) handleSubscribe(client *WebSocketClient, msg map[string]interface{}) {
	subscription, ok := msg["subscription"].(string)
	if !ok {
		return
	}

	client.mu.Lock()
	client.Subscriptions[subscription] = true
	client.mu.Unlock()

	response := WSMessage{
		Type: "subscription_confirmed",
		Data: map[string]interface{}{
			"subscription": subscription,
			"status":       "active",
		},
	}
	wsm.sendToClient(client, response)

	// Send initial data for certain subscription types
	switch subscription {
	case "rates":
		// Send current rates for all pairs
		wsm.sendCurrentRates(client)
	case "orders":
		// Send current orders for user
		wsm.sendCurrentOrders(client)
	}
}

// handleUnsubscribe processes unsubscription requests
func (wsm *WebSocketManager) handleUnsubscribe(client *WebSocketClient, msg map[string]interface{}) {
	subscription, ok := msg["subscription"].(string)
	if !ok {
		return
	}

	client.mu.Lock()
	delete(client.Subscriptions, subscription)
	client.mu.Unlock()

	response := WSMessage{
		Type: "subscription_cancelled",
		Data: map[string]interface{}{
			"subscription": subscription,
			"status":       "inactive",
		},
	}
	wsm.sendToClient(client, response)
}

// handleRateUpdates processes rate updates and broadcasts to subscribed clients
func (wsm *WebSocketManager) handleRateUpdates(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case update := <-wsm.rateUpdates:
			wsm.broadcastToSubscribers("rates", update)
		}
	}
}

// handleOrderUpdates processes order updates and sends to relevant clients
func (wsm *WebSocketManager) handleOrderUpdates(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case update := <-wsm.orderUpdates:
			wsm.sendToUserSubscribers(update.UserID, "orders", update)
		}
	}
}

// handleTradeUpdates processes trade updates and sends to relevant clients
func (wsm *WebSocketManager) handleTradeUpdates(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case update := <-wsm.tradeUpdates:
			wsm.sendToUserSubscribers(update.UserID, "trades", update)
		}
	}
}

// broadcastToSubscribers sends a message to all clients subscribed to a topic
func (wsm *WebSocketManager) broadcastToSubscribers(subscription string, data interface{}) {
	message := WSMessage{Type: subscription, Data: data}
	messageBytes, err := json.Marshal(message)
	if err != nil {
		log.Printf("Failed to marshal message: %v", err)
		return
	}

	wsm.clientsMutex.RLock()
	defer wsm.clientsMutex.RUnlock()

	for _, client := range wsm.clients {
		client.mu.RLock()
		subscribed := client.Subscriptions[subscription]
		client.mu.RUnlock()

		if subscribed {
			select {
			case client.Send <- messageBytes:
			default:
				// Client channel full, remove client
				go wsm.removeClient(client.ID)
			}
		}
	}
}

// sendToUserSubscribers sends a message to all clients of a specific user subscribed to a topic
func (wsm *WebSocketManager) sendToUserSubscribers(userID uuid.UUID, subscription string, data interface{}) {
	message := WSMessage{Type: subscription, Data: data}
	messageBytes, err := json.Marshal(message)
	if err != nil {
		log.Printf("Failed to marshal message: %v", err)
		return
	}

	wsm.clientsMutex.RLock()
	defer wsm.clientsMutex.RUnlock()

	for _, client := range wsm.clients {
		if client.UserID == userID {
			client.mu.RLock()
			subscribed := client.Subscriptions[subscription]
			client.mu.RUnlock()

			if subscribed {
				select {
				case client.Send <- messageBytes:
				default:
					// Client channel full, remove client
					go wsm.removeClient(client.ID)
				}
			}
		}
	}
}

// sendToClient sends a message to a specific client
func (wsm *WebSocketManager) sendToClient(client *WebSocketClient, message WSMessage) {
	messageBytes, err := json.Marshal(message)
	if err != nil {
		log.Printf("Failed to marshal message: %v", err)
		return
	}

	select {
	case client.Send <- messageBytes:
	default:
		// Client channel full, remove client
		go wsm.removeClient(client.ID)
	}
}

// sendCurrentRates sends current rates to a client
func (wsm *WebSocketManager) sendCurrentRates(client *WebSocketClient) {
	// This would fetch current rates from the rate calculator
	// Implementation depends on how rates are stored/cached
}

// sendCurrentOrders sends current orders to a client
func (wsm *WebSocketManager) sendCurrentOrders(client *WebSocketClient) {
	// This would fetch current orders for the user
	// Implementation depends on storage layer
}

// removeClient removes a client from the manager
func (wsm *WebSocketManager) removeClient(clientID string) {
	wsm.clientsMutex.Lock()
	defer wsm.clientsMutex.Unlock()

	if client, exists := wsm.clients[clientID]; exists {
		close(client.Send)
		delete(wsm.clients, clientID)
		log.Printf("WebSocket client disconnected: %s", clientID)
	}
}

// cleanupRoutine periodically removes inactive clients
func (wsm *WebSocketManager) cleanupRoutine(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			wsm.cleanupInactiveClients()
		}
	}
}

// cleanupInactiveClients removes clients that haven't pinged recently
func (wsm *WebSocketManager) cleanupInactiveClients() {
	cutoff := time.Now().Add(-2 * time.Minute)

	wsm.clientsMutex.RLock()
	inactiveClients := make([]string, 0)
	for clientID, client := range wsm.clients {
		if client.LastPing.Before(cutoff) {
			inactiveClients = append(inactiveClients, clientID)
		}
	}
	wsm.clientsMutex.RUnlock()

	for _, clientID := range inactiveClients {
		wsm.removeClient(clientID)
	}
}

// NotifyOrderUpdate sends an order update notification
func (wsm *WebSocketManager) NotifyOrderUpdate(orderID uuid.UUID, userID uuid.UUID, status OrderStatus, order *CrossPairOrder) {
	update := OrderUpdate{
		Type:      "order_update",
		OrderID:   orderID,
		UserID:    userID,
		Status:    status,
		Order:     order,
		Timestamp: time.Now(),
	}

	select {
	case wsm.orderUpdates <- update:
	default:
		// Channel full, skip update
	}
}

// NotifyTradeUpdate sends a trade update notification
func (wsm *WebSocketManager) NotifyTradeUpdate(trade *CrossPairTrade) {
	update := TradeUpdate{
		Type:      "trade_update",
		TradeID:   trade.ID,
		OrderID:   trade.OrderID,
		UserID:    trade.UserID,
		Trade:     trade,
		Timestamp: time.Now(),
	}

	select {
	case wsm.tradeUpdates <- update:
	default:
		// Channel full, skip update
	}
}

// GetConnectedClients returns the number of connected clients
func (wsm *WebSocketManager) GetConnectedClients() int {
	wsm.clientsMutex.RLock()
	defer wsm.clientsMutex.RUnlock()
	return len(wsm.clients)
}

// GetClientsByUser returns the number of connected clients for a specific user
func (wsm *WebSocketManager) GetClientsByUser(userID uuid.UUID) int {
	wsm.clientsMutex.RLock()
	defer wsm.clientsMutex.RUnlock()

	count := 0
	for _, client := range wsm.clients {
		if client.UserID == userID {
			count++
		}
	}
	return count
}

// Shutdown gracefully shuts down the WebSocket manager
func (wsm *WebSocketManager) Shutdown() {
	wsm.clientsMutex.Lock()
	defer wsm.clientsMutex.Unlock()

	for _, client := range wsm.clients {
		close(client.Send)
		client.Conn.Close()
	}

	wsm.clients = make(map[string]*WebSocketClient)
}
