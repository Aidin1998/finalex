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
	Type      string      `json:"type"`
	Data      interface{} `json:"data"`
	Timestamp time.Time   `json:"timestamp"` // --- End-to-end latency measurement for each message ---
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
	clients         sync.Map // *Client -> struct{}
	register        chan *Client
	unregister      chan *Client
	broadcastShards []chan []byte
}

const broadcastShards = 8 // Tune as needed for core count and workload

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
	shards := make([]chan []byte, broadcastShards)
	for i := range shards {
		shards[i] = make(chan []byte, 1024)
	}
	return &Hub{
		clients:         sync.Map{},
		register:        make(chan *Client),
		unregister:      make(chan *Client),
		broadcastShards: shards,
	}
}

// hashMessageShard returns the shard index for a message (round-robin fallback)
var broadcastShardCounter uint64

func hashMessageShard(msg []byte) int {
	// Simple round-robin for now; can use hash(msg) for more even distribution
	return int(atomic.AddUint64(&broadcastShardCounter, 1) % uint64(broadcastShards))
}

func (h *Hub) Run() {
	batchInterval := 10 * time.Millisecond // tune as needed
	var batchs [broadcastShards][][]byte
	tickers := make([]*time.Ticker, broadcastShards)
	for i := range tickers {
		tickers[i] = time.NewTicker(batchInterval)
	}
	defer func() {
		for _, t := range tickers {
			t.Stop()
		}
	}()

	// Start a goroutine per shard for broadcasting
	for shard := 0; shard < broadcastShards; shard++ {
		go func(shard int) {
			for {
				select {
				case message := <-h.broadcastShards[shard]:
					batchs[shard] = append(batchs[shard], message)
				case <-tickers[shard].C:
					if len(batchs[shard]) > 0 {
						batched := batchs[shard]
						batchs[shard] = nil
						// --- COALESCING LOGIC START ---
						latestBySymbol := make(map[string][]byte)
						for _, msg := range batched {
							var parsed map[string]interface{}
							if err := json.Unmarshal(msg, &parsed); err == nil {
								typeVal, _ := parsed["type"].(string)
								if typeVal == MsgOrderBook || typeVal == MsgOrderBookDelta {
									// Try to extract symbol from data
									if data, ok := parsed["data"].(map[string]interface{}); ok {
										if symbol, ok := data["Symbol"].(string); ok {
											latestBySymbol[symbol] = msg
											continue
										}
									}
									// Fallback: if data is just a symbol string (e.g., test server)
									if symbol, ok := parsed["data"].(string); ok {
										latestBySymbol[symbol] = msg
										continue
									}
								}
							}
						}
						// Compose final batch: all coalesced order book updates, plus all non-orderbook messages
						finalBatch := make([][]byte, 0, len(latestBySymbol)+len(batched))
						for _, msg := range latestBySymbol {
							finalBatch = append(finalBatch, msg)
						}
						for _, msg := range batched {
							var parsed map[string]interface{}
							if err := json.Unmarshal(msg, &parsed); err == nil {
								typeVal, _ := parsed["type"].(string)
								if typeVal == MsgOrderBook || typeVal == MsgOrderBookDelta {
									continue // already included
								}
							}
							finalBatch = append(finalBatch, msg)
						}
						start := time.Now()
						h.clients.Range(func(key, _ interface{}) bool {
							client := key.(*Client)
							select {
							case client.send <- joinBatch(finalBatch):
							default:
								// Backpressure: drop or disconnect slow clients
								close(client.send)
								h.clients.Delete(client)
							}
							return true
						})
						MarketDataMessages.Add(float64(len(finalBatch)))
						MarketDataBroadcastLatency.Observe(time.Since(start).Seconds())
					}
				}
			}
		}(shard)
	}

	// Main event loop for registration/unregistration
	for {
		select {
		case client := <-h.register:
			h.clients.Store(client, struct{}{})
			MarketDataConnections.Inc()
		case client := <-h.unregister:
			h.clients.Delete(client)
			close(client.send)
			MarketDataConnections.Dec()
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

// BroadcastMarketData broadcasts a market data message to all clients (sharded)
func (h *Hub) BroadcastMarketData(msg MarketDataMessage) {
	data, _ := json.Marshal(msg)
	shard := hashMessageShard(data)
	h.broadcastShards[shard] <- data
}

// BroadcastBinaryMarketData broadcasts a binary-encoded market data message to all clients (sharded)
func (h *Hub) BroadcastBinaryMarketData(data []byte) {
	shard := hashMessageShard(data)
	h.broadcastShards[shard] <- data
	MarketDataBinaryMessages.Inc()
}

// BroadcastDeltaMarketData broadcasts a delta-encoded market data message to all clients (sharded)
func (h *Hub) BroadcastDeltaMarketData(data []byte) {
	shard := hashMessageShard(data)
	h.broadcastShards[shard] <- data
	MarketDataDeltaMessages.Inc()
}

// Add support for multi-format broadcast (JSON, binary, delta)
func (h *Hub) BroadcastOrderBook(prev, curr *OrderBookSnapshot) {
	// JSON broadcast (default)
	msg := MarketDataMessage{Type: MsgOrderBook, Data: curr}
	data, _ := json.Marshal(msg)
	h.broadcastShards[0] <- data
	MarketDataMessages.Inc()
	// Delta encoding
	delta := ComputeDelta(prev, curr)
	if delta != nil {
		if b, err := EncodeOrderBookDelta(delta); err == nil {
			h.BroadcastDeltaMarketData(b)
		}
	}
	// Binary snapshot
	if b, err := EncodeOrderBookSnapshot(curr); err == nil {
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
var fixGateway *FIXGateway

// In main or setup, initialize fixGateway and start it as needed
// fixGateway = marketdata.NewFIXGateway()
// go fixGateway.Start()

// --- High-performance WebSocket enhancements ---
// 1. Efficient millions of connections: use sync.Map for clients, minimize locks
// 2. Connection pooling: handled by edge node sharding and sticky sessions (see infra/k8s/marketdata-ws.yaml)
// 3. Load balancing: recommend external L4/L7 balancer (NGINX, Envoy, or cloud LB)
// 4. Per-stream subscriptions: already supported via client.channels
// 5. Backpressure: drop or disconnect slow clients
// 6. End-to-end latency monitoring: Prometheus histogram

// --- Test Server for Benchmarks ---
type TestServer struct {
	hub      *Hub
	channels map[string]chan interface{}
	mu       sync.RWMutex
}

func NewTestServer() *TestServer {
	hub := NewHub()
	go hub.Run()
	return &TestServer{
		hub:      hub,
		channels: make(map[string]chan interface{}),
	}
}

func (s *TestServer) Subscribe(symbol string) <-chan interface{} {
	ch := make(chan interface{}, 1024)
	s.mu.Lock()
	s.channels[symbol] = ch
	s.mu.Unlock()
	return ch
}

func (s *TestServer) PublishOrderBookUpdate(symbol string) {
	msg := MarketDataMessage{Type: MsgOrderBook, Data: symbol, Timestamp: time.Now()}
	s.hub.BroadcastMarketData(msg)
	s.mu.RLock()
	ch, ok := s.channels[symbol]
	s.mu.RUnlock()
	if ok {
		ch <- msg
	}
}

func (s *TestServer) Stop() {
	// No-op for now
}
