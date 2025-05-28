package marketdata

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/marketdata"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/gorilla/websocket"
	"github.com/prometheus/client_golang/prometheus"
)

// MessageType represents the type of market data message
const (
	MsgOrderBook      = "orderbook"
	MsgTrade          = "trade"
	MsgTicker         = "ticker"
	MsgCandle         = "candle"
	MsgOrderBookDelta = "orderbook_delta"
	MsgBinary         = "binary"
)

// MarketDataMessage is the structure sent to clients
type MarketDataMessage struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

// Client represents a WebSocket client
type Client struct {
	conn      *websocket.Conn
	channels  map[string]bool
	send      chan []byte
	connected time.Time
}

// Hub manages all clients and broadcasts
type Hub struct {
	clients    map[*Client]bool
	register   chan *Client
	unregister chan *Client
	broadcast  chan []byte
	mu         sync.RWMutex
}

var (
	MarketDataConnections = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "marketdata_ws_connections",
		Help: "Current number of active market data WebSocket connections.",
	})
	MarketDataMessages = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "marketdata_messages_total",
		Help: "Total number of market data messages broadcasted.",
	})
	MarketDataBroadcastLatency = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "marketdata_broadcast_latency_seconds",
		Help:    "Latency of broadcasting market data messages to clients.",
		Buckets: prometheus.DefBuckets,
	})
	MarketDataBinaryMessages = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "marketdata_binary_messages_total",
		Help: "Total number of binary market data messages broadcasted.",
	})
	MarketDataDeltaMessages = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "marketdata_delta_messages_total",
		Help: "Total number of delta-encoded market data messages broadcasted.",
	})
)

func init() {
	prometheus.MustRegister(MarketDataConnections, MarketDataMessages, MarketDataBroadcastLatency, MarketDataBinaryMessages, MarketDataDeltaMessages)
}

func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan []byte, 1024),
	}
}

func (h *Hub) Run() {
	batchInterval := 10 * time.Millisecond // tune as needed
	var batch [][]byte
	ticker := time.NewTicker(batchInterval)
	defer ticker.Stop()
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			MarketDataConnections.Inc()
		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()
			MarketDataConnections.Dec()
		case message := <-h.broadcast:
			batch = append(batch, message)
		case <-ticker.C:
			if len(batch) > 0 {
				batched := batch
				batch = nil
				start := time.Now()
				h.mu.RLock()
				for client := range h.clients {
					select {
					case client.send <- joinBatch(batched):
					default:
						close(client.send)
						delete(h.clients, client)
					}
				}
				h.mu.RUnlock()
				MarketDataMessages.Add(float64(len(batched)))
				MarketDataBroadcastLatency.Observe(time.Since(start).Seconds())
			}
		}
	}
}

// joinBatch joins multiple JSON messages into a JSON array
func joinBatch(msgs [][]byte) []byte {
	if len(msgs) == 1 {
		return msgs[0]
	}
	result := make([]byte, 0, 2+len(msgs)*len(msgs[0]))
	result = append(result, '[')
	for i, m := range msgs {
		if i > 0 {
			result = append(result, ',')
		}
		result = append(result, m...)
	}
	result = append(result, ']')
	return result
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:    1024,
	WriteBufferSize:   1024,
	CheckOrigin:       func(r *http.Request) bool { return true },
	EnableCompression: true, // Enable permessage-deflate compression
}

// ServeWS handles WebSocket requests with protocol negotiation
func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request) {
	// Protocol negotiation: check query param or subprotocol
	protocol := r.URL.Query().Get("protocol")
	if protocol == "" {
		protocol = r.Header.Get("Sec-WebSocket-Protocol")
	}
	protocol = strings.ToLower(protocol)
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WebSocket upgrade error:", err)
		return
	}
	client := &Client{
		conn:      conn,
		channels:  make(map[string]bool),
		send:      make(chan []byte, 256),
		connected: time.Now(),
	}
	// Store protocol preference
	if protocol != "" {
		client.channels["_protocol"] = true // marker
		// Optionally store protocol in client struct
	}
	// Advanced connection management: auth, subscriptions, backpressure
	// Example: check for auth token
	token := r.URL.Query().Get("token")
	if token == "" {
		token = r.Header.Get("Authorization")
	}
	if !validateToken(token) {
		conn.WriteMessage(websocket.CloseMessage, []byte("unauthorized"))
		conn.Close()
		return
	}
	h.register <- client
	go client.writePump()
	go client.readPump(h)
}

// validateToken is a stub for authentication
func validateToken(token string) bool {
	// TODO: Implement real token validation
	return token != ""
}

func (c *Client) readPump(h *Hub) {
	defer func() {
		h.unregister <- c
		c.conn.Close()
	}()
	c.conn.SetReadLimit(512)
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})
	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			break
		}
		// Handle subscription messages (e.g., {"subscribe": ["orderbook", "trade"]})
		var req map[string][]string
		if err := json.Unmarshal(message, &req); err == nil {
			if subs, ok := req["subscribe"]; ok {
				for _, ch := range subs {
					c.channels[ch] = true
				}
			}
			if unsubs, ok := req["unsubscribe"]; ok {
				for _, ch := range unsubs {
					delete(c.channels, ch)
				}
			}
		}
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)
			w.Close()
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// BroadcastMarketData broadcasts a market data message to all clients
func (h *Hub) BroadcastMarketData(msg MarketDataMessage) {
	data, _ := json.Marshal(msg)
	h.broadcast <- data
}

// BroadcastBinaryMarketData broadcasts a binary-encoded market data message to all clients
func (h *Hub) BroadcastBinaryMarketData(data []byte) {
	h.broadcast <- data
	MarketDataBinaryMessages.Inc()
}

// BroadcastDeltaMarketData broadcasts a delta-encoded market data message to all clients
func (h *Hub) BroadcastDeltaMarketData(data []byte) {
	h.broadcast <- data
	MarketDataDeltaMessages.Inc()
}

// Add support for multi-format broadcast (JSON, binary, delta)
func (h *Hub) BroadcastOrderBook(prev, curr *marketdata.OrderBookSnapshot) {
	// JSON broadcast (default)
	msg := MarketDataMessage{Type: MsgOrderBook, Data: curr}
	data, _ := json.Marshal(msg)
	h.broadcast <- data
	MarketDataMessages.Inc()
	// Delta encoding
	delta := marketdata.ComputeDelta(prev, curr)
	if delta != nil {
		if b, err := marketdata.EncodeOrderBookDelta(delta); err == nil {
			h.BroadcastDeltaMarketData(b)
		}
	}
	// Binary snapshot
	if b, err := marketdata.EncodeOrderBookSnapshot(curr); err == nil {
		h.BroadcastBinaryMarketData(b)
	}
	// FIX gateway broadcast (if enabled)
	if fixGateway != nil {
		_ = fixGateway.BroadcastMarketDataFIX(curr)
		_ = fixGateway.BroadcastMarketDataIncrementalFIX(delta)
	}
}

// PublishOrderBookUpdate publishes an order book update to pubsub
func PublishOrderBookUpdate(pubsub PubSubBackend, symbol string, snapshot interface{}) error {
	msg := MarketDataMessage{Type: MsgOrderBook, Data: snapshot}
	return pubsub.Publish(context.Background(), "orderbook", msg)
}

// PublishTrade publishes a trade event to pubsub
func PublishTrade(pubsub PubSubBackend, symbol string, trade interface{}) error {
	msg := MarketDataMessage{Type: MsgTrade, Data: trade}
	return pubsub.Publish(context.Background(), "trade", msg)
}

// PublishTicker publishes a ticker update to pubsub
func PublishTicker(pubsub PubSubBackend, symbol string, ticker interface{}) error {
	msg := MarketDataMessage{Type: MsgTicker, Data: ticker}
	return pubsub.Publish(context.Background(), "ticker", msg)
}

// PublishCandle publishes a candle update to pubsub
func PublishCandle(pubsub PubSubBackend, symbol string, candle interface{}) error {
	msg := MarketDataMessage{Type: MsgCandle, Data: candle}
	return pubsub.Publish(context.Background(), "candle", msg)
}

// --- Rate Limiting Middleware for Gin ---
// Usage: router.Use(marketdata.RateLimitMiddleware(100, time.Second))
func RateLimitMiddleware(maxPerInterval int, interval time.Duration) gin.HandlerFunc {
	var count int32
	var lastReset int64
	return func(c *gin.Context) {
		now := time.Now().UnixNano()
		reset := atomic.LoadInt64(&lastReset)
		if now-reset > interval.Nanoseconds() {
			atomic.StoreInt32(&count, 0)
			atomic.StoreInt64(&lastReset, now)
		}
		if atomic.AddInt32(&count, 1) > int32(maxPerInterval) {
			c.AbortWithStatusJSON(429, gin.H{"error": "rate limit exceeded"})
			return
		}
		c.Next()
	}
}

// --- Advanced Per-User Rate Limiting Middleware for Gin ---
// Usage: router.Use(marketdata.PerUserRateLimitMiddleware(20, time.Second))
func PerUserRateLimitMiddleware(maxPerInterval int, interval time.Duration) gin.HandlerFunc {
	type userBucket struct {
		count    int
		lastTime time.Time
	}
	var buckets = make(map[string]*userBucket)
	var mu sync.Mutex
	return func(c *gin.Context) {
		userID := c.GetHeader("X-User-ID")
		if userID == "" {
			userID = c.ClientIP()
		}
		mu.Lock()
		bucket, ok := buckets[userID]
		if !ok || time.Since(bucket.lastTime) > interval {
			bucket = &userBucket{count: 0, lastTime: time.Now()}
			buckets[userID] = bucket
		}
		bucket.count++
		bucket.lastTime = time.Now()
		if bucket.count > maxPerInterval {
			mu.Unlock()
			c.AbortWithStatusJSON(429, gin.H{"error": "per-user rate limit exceeded"})
			return
		}
		mu.Unlock()
		c.Next()
	}
}

// --- Fallback: Redis Streams consumer for missed messages ---
// This is a basic example for orderbook channel; extend for other channels as needed.
// FetchMissedMessages fetches missed messages from a Redis Stream after a reconnect
func FetchMissedMessages(rdb *redis.Client, stream string, lastID string, count int64) ([][]byte, string, error) {
	ctx := context.Background()
	res, err := rdb.XRead(ctx, &redis.XReadArgs{
		Streams: []string{stream, lastID},
		Count:   count,
		Block:   0,
	}).Result()
	if err != nil {
		return nil, lastID, err
	}
	var messages [][]byte
	newLastID := lastID
	for _, streamRes := range res {
		for _, msg := range streamRes.Messages {
			if data, ok := msg.Values["data"].(string); ok {
				messages = append(messages, []byte(data))
				newLastID = msg.ID
			}
		}
	}
	return messages, newLastID, nil
}

// Stub for FIX gateway integration
var fixGateway *marketdata.FIXGateway

// In main or setup, initialize fixGateway and start it as needed
// fixGateway = marketdata.NewFIXGateway()
// go fixGateway.Start()
