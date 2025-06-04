// Package ws provides a high-performance, sharded WebSocket Hub with replay buffers and lifecycle management.
package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

// EnhancedMessage extends Message with additional metadata for enhanced clients
type EnhancedMessage struct {
	Message
	ClientID  string            `json:"client_id,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	Priority  int               `json:"priority,omitempty"`
	Retries   int               `json:"retries,omitempty"`
	ExpiresAt *time.Time        `json:"expires_at,omitempty"`
}

// Message wraps a WebSocket payload with sequencing for replay.
type Message struct {
	Topic string `json:"topic"`
	Seq   uint64 `json:"seq"`
	Data  []byte `json:"data"`
}

// ringBuffer holds the last N messages for a topic.
type ringBuffer struct {
	mu    sync.RWMutex
	buf   []Message
	size  int
	start int
	count int
}

func newRingBuffer(size int) *ringBuffer {
	return &ringBuffer{buf: make([]Message, size), size: size}
}

// add appends a message, overwriting old entries when full.
func (r *ringBuffer) add(msg Message) {
	r.mu.Lock()
	defer r.mu.Unlock()
	idx := (r.start + r.count) % r.size
	if r.count == r.size {
		r.start = (r.start + 1) % r.size
		r.count--
	}
	r.buf[idx] = msg
	r.count++
}

// addEnhanced adds an enhanced message to the buffer
func (r *ringBuffer) addEnhanced(msg EnhancedMessage) {
	r.add(msg.Message)
}

// getSince returns messages with Seq > since.
func (r *ringBuffer) getSince(since uint64) []Message {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []Message
	for i := 0; i < r.count; i++ {
		msg := r.buf[(r.start+i)%r.size]
		if msg.Seq > since {
			out = append(out, msg)
		}
	}
	return out
}

// Client represents a single WebSocket connection.
type Client struct {
	id            string // unique client identifier
	conn          *websocket.Conn
	send          chan Message
	subscriptions map[string]uint64 // topic->latest seq acked
	hub           *Hub

	// Enhanced client integration
	enhanced   *EnhancedClient // Reference to enhanced client if available
	isEnhanced bool            // Whether this client uses enhanced features
}

// Hub manages all WebSocket clients, sharded for concurrency with enhanced connection management.
type Hub struct {
	shards     []*hubShard
	shardCount uint32

	register   chan *Client
	unregister chan *Client
	broadcast  chan Message

	// Enhanced broadcasting
	enhancedBroadcast chan EnhancedMessage

	buffers map[string]*ringBuffer
	bufMu   sync.Mutex
	seqMu   sync.Mutex
	nextSeq uint64

	upgrader websocket.Upgrader

	// Enhanced connection management
	connManager *ConnectionManager
	logger      *zap.Logger

	// Metrics and monitoring
	metrics struct {
		connectionsTotal    prometheus.Counter
		messagesTotal       prometheus.Counter
		disconnectionsTotal prometheus.Counter
		broadcastLatency    prometheus.Histogram
	}

	// Lifecycle management
	ctx      context.Context
	cancel   context.CancelFunc
	shutdown chan struct{}
	wg       sync.WaitGroup
}

type hubShard struct {
	mu              sync.RWMutex
	clients         map[*Client]struct{}
	enhancedClients map[string]*EnhancedClient // client ID -> enhanced client

	// Shard-level metrics
	connectionCount int64
	messageCount    int64
}

// NewHub creates a Hub with given shard count and replay buffer size per topic.
func NewHub(shardCount int, replaySize int) *Hub {
	return NewHubWithLogger(shardCount, replaySize, zap.NewNop())
}

// NewHubWithLogger creates a Hub with enhanced connection management and logging.
func NewHubWithLogger(shardCount int, replaySize int, logger *zap.Logger) *Hub {
	ctx, cancel := context.WithCancel(context.Background())

	// Initialize connection manager with production-optimized config
	connConfig := DefaultConnectionConfig()
	connConfig.MaxConnections = 10000
	connConfig.MaxConnectionsPerIP = 100
	connConfig.CleanupInterval = 30 * time.Second
	connConfig.HeartbeatInterval = 30 * time.Second
	connConfig.HeartbeatTimeout = 60 * time.Second

	connManager := NewConnectionManager(connConfig, logger)

	h := &Hub{
		shards:            make([]*hubShard, shardCount),
		shardCount:        uint32(shardCount),
		register:          make(chan *Client, 1000),
		unregister:        make(chan *Client, 1000),
		broadcast:         make(chan Message, 10000),
		enhancedBroadcast: make(chan EnhancedMessage, 10000),
		buffers:           make(map[string]*ringBuffer),
		nextSeq:           1,
		connManager:       connManager,
		logger:            logger,
		ctx:               ctx,
		cancel:            cancel,
		shutdown:          make(chan struct{}),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  4096,
			WriteBufferSize: 4096,
			CheckOrigin:     func(r *http.Request) bool { return true },
		},
	}

	// Initialize metrics
	h.initMetrics()

	// Initialize shards
	for i := range h.shards {
		h.shards[i] = &hubShard{
			clients:         make(map[*Client]struct{}),
			enhancedClients: make(map[string]*EnhancedClient),
		}
	}

	// Start the hub goroutine
	h.wg.Add(1)
	go h.run()

	// Start the connection manager
	connManager.Start(ctx)

	return h
}

// initMetrics initializes Prometheus metrics
func (h *Hub) initMetrics() {
	h.metrics.connectionsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "websocket_connections_total",
		Help: "Total number of WebSocket connections",
	})

	h.metrics.messagesTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "websocket_messages_total",
		Help: "Total number of WebSocket messages sent",
	})

	h.metrics.disconnectionsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "websocket_disconnections_total",
		Help: "Total number of WebSocket disconnections",
	})

	h.metrics.broadcastLatency = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "websocket_broadcast_latency_seconds",
		Help:    "WebSocket broadcast latency in seconds",
		Buckets: prometheus.DefBuckets,
	})

	// Register metrics
	prometheus.MustRegister(
		h.metrics.connectionsTotal,
		h.metrics.messagesTotal,
		h.metrics.disconnectionsTotal,
		h.metrics.broadcastLatency,
	)
}

// run handles registration, unregistration, and broadcasting with enhanced connection management.
func (h *Hub) run() {
	defer h.wg.Done()
	defer h.logger.Info("Hub run loop stopped")

	h.logger.Info("Hub run loop started")

	for {
		select {
		case <-h.ctx.Done():
			h.logger.Info("Hub context cancelled, stopping run loop")
			return
		case <-h.shutdown:
			h.logger.Info("Hub shutdown signal received")
			return
		case client := <-h.register:
			h.handleClientRegistration(client)
		case client := <-h.unregister:
			h.handleClientUnregistration(client)
		case msg := <-h.broadcast:
			h.handleBroadcast(msg)
		case enhancedMsg := <-h.enhancedBroadcast:
			h.handleEnhancedBroadcast(enhancedMsg)
		}
	}
}

// handleClientRegistration handles client registration with enhanced tracking
func (h *Hub) handleClientRegistration(client *Client) {
	sh := h.shardFor(client.id)
	sh.mu.Lock()
	sh.clients[client] = struct{}{}
	atomic.AddInt64(&sh.connectionCount, 1)
	sh.mu.Unlock()

	// Update metrics
	h.metrics.connectionsTotal.Inc()

	h.logger.Debug("Client registered",
		zap.String("client_id", client.id),
		zap.Bool("enhanced", client.isEnhanced))
}

// handleClientUnregistration handles client unregistration with proper cleanup
func (h *Hub) handleClientUnregistration(client *Client) {
	sh := h.shardFor(client.id)
	sh.mu.Lock()
	delete(sh.clients, client)
	atomic.AddInt64(&sh.connectionCount, -1)

	// Remove enhanced client if present
	if client.enhanced != nil {
		delete(sh.enhancedClients, client.id)
		// Unregister from connection manager
		h.connManager.UnregisterConnection(client.id)
	}
	sh.mu.Unlock()

	// Close client channel safely
	h.closeClientChannel(client)

	// Update metrics
	h.metrics.disconnectionsTotal.Inc()

	h.logger.Debug("Client unregistered",
		zap.String("client_id", client.id),
		zap.Bool("enhanced", client.isEnhanced))
}

// closeClientChannel safely closes a client's send channel
func (h *Hub) closeClientChannel(client *Client) {
	defer func() {
		if r := recover(); r != nil {
			h.logger.Warn("Panic while closing client channel",
				zap.String("client_id", client.id),
				zap.Any("panic", r))
		}
	}()

	select {
	case <-client.send:
		// Channel already closed
	default:
		close(client.send)
	}
}

// handleBroadcast handles regular message broadcasting
func (h *Hub) handleBroadcast(msg Message) {
	start := time.Now()
	defer func() {
		h.metrics.broadcastLatency.Observe(time.Since(start).Seconds())
	}()

	// Store in replay buffer
	h.bufMu.Lock()
	buf, ok := h.buffers[msg.Topic]
	if !ok {
		buf = newRingBuffer(1000)
		h.buffers[msg.Topic] = buf
	}
	buf.add(msg)
	h.bufMu.Unlock()

	// Fan-out to all shards
	h.broadcastToShards(msg)

	// Update metrics
	h.metrics.messagesTotal.Inc()
}

// handleEnhancedBroadcast handles enhanced message broadcasting
func (h *Hub) handleEnhancedBroadcast(enhancedMsg EnhancedMessage) {
	start := time.Now()
	defer func() {
		h.metrics.broadcastLatency.Observe(time.Since(start).Seconds())
	}()

	// Store in replay buffer
	h.bufMu.Lock()
	buf, ok := h.buffers[enhancedMsg.Topic]
	if !ok {
		buf = newRingBuffer(1000)
		h.buffers[enhancedMsg.Topic] = buf
	}
	buf.addEnhanced(enhancedMsg)
	h.bufMu.Unlock()

	// Handle enhanced features (priority, expiration, retries)
	if enhancedMsg.ExpiresAt != nil && time.Now().After(*enhancedMsg.ExpiresAt) {
		h.logger.Debug("Enhanced message expired, skipping broadcast",
			zap.String("topic", enhancedMsg.Topic),
			zap.Uint64("seq", enhancedMsg.Seq))
		return
	}

	// Fan-out to all shards with enhanced features
	h.broadcastEnhancedToShards(enhancedMsg)

	// Update metrics
	h.metrics.messagesTotal.Inc()
}

// broadcastToShards broadcasts a message to all relevant shards
func (h *Hub) broadcastToShards(msg Message) {
	for _, sh := range h.shards {
		sh.mu.RLock()
		for c := range sh.clients {
			if _, sub := c.subscriptions[msg.Topic]; sub {
				select {
				case c.send <- msg:
					// Message sent successfully
				default:
					// Client channel is full, skip this slow client
					h.logger.Warn("Dropping message for slow client",
						zap.String("client_id", c.id),
						zap.String("topic", msg.Topic))
				}
			}
		}
		sh.mu.RUnlock()
	}
}

// broadcastEnhancedToShards broadcasts an enhanced message to all relevant shards
func (h *Hub) broadcastEnhancedToShards(enhancedMsg EnhancedMessage) {
	msg := enhancedMsg.Message

	for _, sh := range h.shards {
		sh.mu.RLock()
		// Handle enhanced clients first
		for connID, enhancedClient := range sh.enhancedClients {
			if enhancedClient.IsSubscribed(msg.Topic) {
				// Create enhanced message payload that includes metadata and priority
				enhancedPayload := map[string]interface{}{
					"type":     "enhanced",
					"data":     json.RawMessage(msg.Data),
					"metadata": enhancedMsg.Metadata,
					"priority": enhancedMsg.Priority,
				}

				// Serialize the enhanced payload
				enhancedData, err := json.Marshal(enhancedPayload)
				if err != nil {
					h.logger.Warn("Failed to marshal enhanced message payload",
						zap.String("client_id", connID),
						zap.String("topic", msg.Topic),
						zap.Error(err))
					continue
				}

				// Create the message with enhanced data
				enhancedMessage := &Message{
					Topic: msg.Topic,
					Seq:   msg.Seq,
					Data:  enhancedData,
				}

				if err := enhancedClient.SendMessage(enhancedMessage); err != nil {
					h.logger.Warn("Failed to send enhanced message",
						zap.String("client_id", connID),
						zap.String("topic", msg.Topic),
						zap.Error(err))
				}
			}
		}

		// Handle legacy clients
		for c := range sh.clients {
			if !c.isEnhanced {
				if _, sub := c.subscriptions[msg.Topic]; sub {
					select {
					case c.send <- msg:
						// Message sent successfully
					default:
						// Client channel is full, skip this slow client
						h.logger.Warn("Dropping enhanced message for slow legacy client",
							zap.String("client_id", c.id),
							zap.String("topic", msg.Topic))
					}
				}
			}
		}
		sh.mu.RUnlock()
	}
}

func (h *Hub) shardFor(key string) *hubShard {
	hasher := fnv.New32a()
	hasher.Write([]byte(key))
	idx := hasher.Sum32() % h.shardCount
	return h.shards[idx]
}

// ServeWS upgrades HTTP to WS and registers the client under given clientID.
func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request, clientID string) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	c := &Client{
		id:            clientID,
		conn:          conn,
		send:          make(chan Message, 256),
		subscriptions: make(map[string]uint64),
		hub:           h,
	}
	h.register <- c
	go c.writePump()
	go c.readPump()
}

// Broadcast publishes a message to a topic for all subscribed clients.
func (h *Hub) Broadcast(topic string, data []byte) {
	h.seqMu.Lock()
	seq := h.nextSeq
	h.nextSeq++
	h.seqMu.Unlock()
	h.broadcast <- Message{Topic: topic, Seq: seq, Data: data}
}

// BroadcastEnhanced publishes an enhanced message to a topic for all subscribed clients
func (h *Hub) BroadcastEnhanced(topic string, data []byte, metadata map[string]string, priority int) {
	h.seqMu.Lock()
	seq := h.nextSeq
	h.nextSeq++
	h.seqMu.Unlock()

	enhancedMsg := EnhancedMessage{
		Message: Message{
			Topic: topic,
			Seq:   seq,
			Data:  data,
		},
		Metadata: metadata,
		Priority: priority,
	}

	select {
	case h.enhancedBroadcast <- enhancedMsg:
		// Queued successfully
	default:
		h.logger.Warn("Enhanced broadcast channel full, dropping message",
			zap.String("topic", topic),
			zap.Uint64("seq", seq))
	}
}

// Replay returns buffered messages for topic since the given sequence.
func (h *Hub) Replay(topic string, since uint64) []Message {
	h.bufMu.Lock()
	defer h.bufMu.Unlock()
	if buf, ok := h.buffers[topic]; ok {
		return buf.getSince(since)
	}
	return nil
}

// GetConnectionStats returns connection statistics
func (h *Hub) GetConnectionStats() map[string]interface{} {
	stats := make(map[string]interface{})

	totalClients := 0
	totalEnhanced := 0

	for i, sh := range h.shards {
		sh.mu.RLock()
		shardClients := len(sh.clients)
		shardEnhanced := len(sh.enhancedClients)
		sh.mu.RUnlock()

		totalClients += shardClients
		totalEnhanced += shardEnhanced

		stats[fmt.Sprintf("shard_%d_clients", i)] = shardClients
		stats[fmt.Sprintf("shard_%d_enhanced", i)] = shardEnhanced
	}

	stats["total_clients"] = totalClients
	stats["total_enhanced"] = totalEnhanced
	stats["total_topics"] = len(h.buffers)

	// Add connection manager stats if available
	if h.connManager != nil {
		cmStats := h.connManager.GetResourceStats()
		for k, v := range cmStats {
			stats["cm_"+k] = v
		}
	}

	return stats
}

// readPump handles incoming control frames and subscription requests.
func (c *Client) readPump() {
	defer func() { c.hub.unregister <- c; c.conn.Close() }()
	c.conn.SetReadLimit(512)
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})
	for {
		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			break
		}
		// interpret msg as JSON subscription: {"subscribe":["topic1"]}
		var req map[string][]string
		if err := json.Unmarshal(msg, &req); err == nil {
			if subs, ok := req["subscribe"]; ok {
				for _, topic := range subs {
					c.subscriptions[topic] = 0 // start from beginning
					// send replay
					for _, m := range c.hub.Replay(topic, 0) {
						c.send <- m
					}
				}
			}
			if unsubs, ok := req["unsubscribe"]; ok {
				for _, topic := range unsubs {
					delete(c.subscriptions, topic)
				}
			}
		}
	}
}

// writePump sends messages and heartbeats to the client.
func (c *Client) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() { ticker.Stop(); c.conn.Close() }()
	for {
		select {
		case msg, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg.Data); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// Shutdown gracefully stops the hub
func (h *Hub) Shutdown() error {
	h.logger.Info("Shutting down WebSocket hub")

	// Signal shutdown
	close(h.shutdown)

	// Cancel context to stop all background processes
	h.cancel()

	// Stop connection manager
	if h.connManager != nil {
		h.connManager.Stop()
	}

	// Wait for hub goroutine to finish
	h.wg.Wait()

	// Close all client connections
	for _, sh := range h.shards {
		sh.mu.Lock()
		for client := range sh.clients {
			if client.conn != nil {
				client.conn.Close()
			}
		}
		for _, enhancedClient := range sh.enhancedClients {
			enhancedClient.Close(websocket.CloseGoingAway, "Hub shutdown")
		}
		sh.mu.Unlock()
	}

	h.logger.Info("WebSocket hub shutdown complete")
	return nil
}
