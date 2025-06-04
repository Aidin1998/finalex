// Enhanced WebSocket client with comprehensive lifecycle management and heartbeat
package ws

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// EnhancedClient represents an enhanced WebSocket client with comprehensive lifecycle management
type EnhancedClient struct {
	// Core connection
	id         string
	conn       *websocket.Conn
	remoteAddr string
	userAgent  string

	// State management
	state        int32 // ConnectionState stored as int32 for atomic operations
	lastActivity int64 // Unix timestamp for atomic access
	createdAt    time.Time

	// Heartbeat management
	lastHeartbeat    int64 // Unix timestamp for atomic access
	missedHeartbeats int32
	heartbeatTicker  *time.Ticker

	// Communication channels
	send    chan *Message
	receive chan *Message
	control chan *ControlMessage

	// Subscription management
	subscriptions   sync.Map // topic -> subscription info
	subscriptionsMu sync.RWMutex

	// Lifecycle management
	closeOnce    sync.Once
	shutdownChan chan struct{}
	doneChan     chan struct{}

	// Resource tracking
	metrics *ConnectionMetrics
	logger  *zap.Logger
	manager *ConnectionManager

	// Configuration
	config ConnectionConfig

	// Cleanup tracking
	goroutines   int32    // Track active goroutines for this connection
	cleanupFuncs []func() // Functions to call during cleanup
	cleanupMu    sync.Mutex
}

// ControlMessage represents control messages for the connection
type ControlMessage struct {
	Type    string                 `json:"type"`
	Payload map[string]interface{} `json:"payload,omitempty"`
}

// HeartbeatMessage represents a heartbeat message
type HeartbeatMessage struct {
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	ID        string    `json:"id,omitempty"`
}

// SubscriptionInfo tracks subscription details
type SubscriptionInfo struct {
	Topic        string    `json:"topic"`
	SubscribedAt time.Time `json:"subscribed_at"`
	MessageCount int64     `json:"message_count"`
	LastMessage  time.Time `json:"last_message"`
}

// NewEnhancedClient creates a new enhanced WebSocket client
func NewEnhancedClient(conn *websocket.Conn, config ConnectionConfig, manager *ConnectionManager, logger *zap.Logger) *EnhancedClient {
	if logger == nil {
		logger = zap.NewNop()
	}

	clientID := generateClientID()
	now := time.Now()

	client := &EnhancedClient{
		id:            clientID,
		conn:          conn,
		remoteAddr:    conn.RemoteAddr().String(),
		userAgent:     conn.Subprotocol(),
		state:         int32(StateConnecting),
		lastActivity:  now.Unix(),
		createdAt:     now,
		lastHeartbeat: now.Unix(),
		send:          make(chan *Message, config.MessageBufferSize),
		receive:       make(chan *Message, config.MessageBufferSize),
		control:       make(chan *ControlMessage, 10),
		shutdownChan:  make(chan struct{}),
		doneChan:      make(chan struct{}),
		logger:        logger.With(zap.String("client_id", clientID)),
		manager:       manager,
		config:        config,
		metrics: &ConnectionMetrics{
			CreatedAt:    now,
			LastActivity: now,
		},
	}

	// Set initial connection state
	atomic.StoreInt32(&client.state, int32(StateConnected))

	return client
}

// generateClientID generates a unique client ID
func generateClientID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// Start initializes and starts the client
func (c *EnhancedClient) Start(ctx context.Context) error {
	// Set connection parameters
	c.conn.SetReadLimit(int64(c.config.ReadBufferSize))
	c.conn.SetReadDeadline(time.Now().Add(c.config.ConnectionTimeout))
	c.conn.SetWriteDeadline(time.Now().Add(c.config.ConnectionTimeout))

	// Setup pong handler for heartbeat
	c.conn.SetPongHandler(func(appData string) error {
		c.updateLastActivity()
		c.updateLastHeartbeat()
		atomic.StoreInt32(&c.missedHeartbeats, 0)
		return nil
	})

	// Setup close handler
	c.conn.SetCloseHandler(func(code int, text string) error {
		c.logger.Info("Connection close received",
			zap.Int("code", code),
			zap.String("text", text))
		c.setState(StateClosed)
		return nil
	})

	// Start heartbeat
	c.startHeartbeat()

	// Start goroutines
	go c.readPump(ctx)
	go c.writePump(ctx)
	go c.controlHandler(ctx)

	// Register cleanup functions
	c.addCleanupFunc(func() {
		if c.heartbeatTicker != nil {
			c.heartbeatTicker.Stop()
		}
	})

	c.logger.Info("Enhanced WebSocket client started",
		zap.String("remote_addr", c.remoteAddr))

	return nil
}

// readPump handles incoming messages with comprehensive error recovery
func (c *EnhancedClient) readPump(ctx context.Context) {
	defer func() {
		atomic.AddInt32(&c.goroutines, -1)
		c.manager.resourceTracker.UntrackGoroutine()

		if r := recover(); r != nil {
			c.logger.Error("ReadPump panic recovered",
				zap.Any("panic", r))
		}
		c.Close(websocket.CloseAbnormalClosure, "Read pump terminated")
	}()

	atomic.AddInt32(&c.goroutines, 1)
	c.manager.resourceTracker.TrackGoroutine()

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.shutdownChan:
			return
		default:
			// Set read deadline
			c.conn.SetReadDeadline(time.Now().Add(c.config.ConnectionTimeout))

			messageType, data, err := c.conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					c.logger.Warn("Unexpected WebSocket close", zap.Error(err))
				}
				c.setState(StateError)
				return
			}

			c.updateLastActivity()
			atomic.AddInt64(&c.metrics.MessagesReceived, 1)
			atomic.AddInt64(&c.metrics.BytesReceived, int64(len(data)))

			// Handle different message types
			switch messageType {
			case websocket.TextMessage:
				c.handleTextMessage(data)
			case websocket.BinaryMessage:
				c.handleBinaryMessage(data)
			case websocket.PingMessage:
				c.handlePingMessage()
			case websocket.PongMessage:
				c.handlePongMessage()
			case websocket.CloseMessage:
				c.handleCloseMessage()
				return
			}
		}
	}
}

// writePump handles outgoing messages with backpressure management
func (c *EnhancedClient) writePump(ctx context.Context) {
	defer func() {
		atomic.AddInt32(&c.goroutines, -1)
		c.manager.resourceTracker.UntrackGoroutine()

		if r := recover(); r != nil {
			c.logger.Error("WritePump panic recovered",
				zap.Any("panic", r))
		}
		c.conn.Close()
	}()

	atomic.AddInt32(&c.goroutines, 1)
	c.manager.resourceTracker.TrackGoroutine()

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.shutdownChan:
			return
		case message, ok := <-c.send:
			if !ok {
				// Channel closed, send close message
				c.conn.SetWriteDeadline(time.Now().Add(c.config.ConnectionTimeout))
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			// Set write deadline
			c.conn.SetWriteDeadline(time.Now().Add(c.config.ConnectionTimeout))

			// Send message
			err := c.sendMessage(message)
			if err != nil {
				c.logger.Warn("Failed to send message", zap.Error(err))
				c.setState(StateError)
				return
			}

			c.updateLastActivity()
			atomic.AddInt64(&c.metrics.MessagesSent, 1)
			atomic.AddInt64(&c.metrics.BytesSent, int64(len(message.Data)))
		}
	}
}

// controlHandler manages control messages and connection health
func (c *EnhancedClient) controlHandler(ctx context.Context) {
	defer func() {
		atomic.AddInt32(&c.goroutines, -1)
		c.manager.resourceTracker.UntrackGoroutine()

		if r := recover(); r != nil {
			c.logger.Error("ControlHandler panic recovered",
				zap.Any("panic", r))
		}
	}()

	atomic.AddInt32(&c.goroutines, 1)
	c.manager.resourceTracker.TrackGoroutine()

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.shutdownChan:
			return
		case controlMsg, ok := <-c.control:
			if !ok {
				return
			}
			c.handleControlMessage(controlMsg)
		}
	}
}

// handleTextMessage processes incoming text messages
func (c *EnhancedClient) handleTextMessage(data []byte) {
	// Try to parse as control message first
	var controlMsg ControlMessage
	if err := json.Unmarshal(data, &controlMsg); err == nil {
		select {
		case c.control <- &controlMsg:
		default:
			c.logger.Warn("Control channel full, dropping message")
		}
		return
	}

	// Regular message
	message := &Message{
		Topic: "default", // Will be determined by message content
		Data:  data,
		Seq:   uint64(time.Now().UnixNano()),
	}

	select {
	case c.receive <- message:
	default:
		c.logger.Warn("Receive channel full, dropping message")
	}
}

// handleBinaryMessage processes incoming binary messages
func (c *EnhancedClient) handleBinaryMessage(data []byte) {
	message := &Message{
		Topic: "binary",
		Data:  data,
		Seq:   uint64(time.Now().UnixNano()),
	}

	select {
	case c.receive <- message:
	default:
		c.logger.Warn("Receive channel full, dropping binary message")
	}
}

// handlePingMessage responds to ping messages
func (c *EnhancedClient) handlePingMessage() {
	c.conn.SetWriteDeadline(time.Now().Add(c.config.ConnectionTimeout))
	if err := c.conn.WriteMessage(websocket.PongMessage, nil); err != nil {
		c.logger.Warn("Failed to send pong", zap.Error(err))
	}
}

// handlePongMessage processes pong responses
func (c *EnhancedClient) handlePongMessage() {
	c.updateLastHeartbeat()
	atomic.StoreInt32(&c.missedHeartbeats, 0)
}

// handleCloseMessage processes close messages
func (c *EnhancedClient) handleCloseMessage() {
	c.setState(StateClosing)
}

// handleControlMessage processes control messages
func (c *EnhancedClient) handleControlMessage(msg *ControlMessage) {
	switch msg.Type {
	case "subscribe":
		c.handleSubscribe(msg)
	case "unsubscribe":
		c.handleUnsubscribe(msg)
	case "ping":
		c.handleControlPing(msg)
	case "heartbeat":
		c.handleHeartbeat(msg)
	default:
		c.logger.Debug("Unknown control message type", zap.String("type", msg.Type))
	}
}

// handleSubscribe processes subscription requests
func (c *EnhancedClient) handleSubscribe(msg *ControlMessage) {
	if topics, ok := msg.Payload["topics"].([]interface{}); ok {
		for _, topicInterface := range topics {
			if topic, ok := topicInterface.(string); ok {
				info := &SubscriptionInfo{
					Topic:        topic,
					SubscribedAt: time.Now(),
				}
				c.subscriptions.Store(topic, info)
				atomic.AddInt32(&c.metrics.SubscriptionsCount, 1)

				c.logger.Debug("Client subscribed to topic", zap.String("topic", topic))

				// Send confirmation
				c.sendControlMessage("subscribed", map[string]interface{}{
					"topic":  topic,
					"status": "success",
				})
			}
		}
	}
}

// handleUnsubscribe processes unsubscription requests
func (c *EnhancedClient) handleUnsubscribe(msg *ControlMessage) {
	if topics, ok := msg.Payload["topics"].([]interface{}); ok {
		for _, topicInterface := range topics {
			if topic, ok := topicInterface.(string); ok {
				if _, exists := c.subscriptions.LoadAndDelete(topic); exists {
					atomic.AddInt32(&c.metrics.SubscriptionsCount, -1)

					c.logger.Debug("Client unsubscribed from topic", zap.String("topic", topic))

					// Send confirmation
					c.sendControlMessage("unsubscribed", map[string]interface{}{
						"topic":  topic,
						"status": "success",
					})
				}
			}
		}
	}
}

// handleControlPing responds to control ping messages
func (c *EnhancedClient) handleControlPing(msg *ControlMessage) {
	c.sendControlMessage("pong", map[string]interface{}{
		"timestamp": time.Now(),
	})
}

// handleHeartbeat processes heartbeat messages
func (c *EnhancedClient) handleHeartbeat(msg *ControlMessage) {
	c.updateLastHeartbeat()

	// Send heartbeat response
	c.sendControlMessage("heartbeat_ack", map[string]interface{}{
		"timestamp": time.Now(),
		"client_id": c.id,
	})
}

// sendControlMessage sends a control message to the client
func (c *EnhancedClient) sendControlMessage(msgType string, payload map[string]interface{}) {
	controlMsg := &ControlMessage{
		Type:    msgType,
		Payload: payload,
	}

	data, err := json.Marshal(controlMsg)
	if err != nil {
		c.logger.Error("Failed to marshal control message", zap.Error(err))
		return
	}

	message := &Message{
		Topic: "control",
		Data:  data,
		Seq:   uint64(time.Now().UnixNano()),
	}

	select {
	case c.send <- message:
	default:
		c.logger.Warn("Send channel full, dropping control message")
	}
}

// sendMessage sends a message over the WebSocket connection
func (c *EnhancedClient) sendMessage(message *Message) error {
	return c.conn.WriteMessage(websocket.TextMessage, message.Data)
}

// startHeartbeat begins the heartbeat mechanism
func (c *EnhancedClient) startHeartbeat() {
	c.heartbeatTicker = time.NewTicker(c.config.HeartbeatInterval)

	go func() {
		defer func() {
			atomic.AddInt32(&c.goroutines, -1)
			c.manager.resourceTracker.UntrackGoroutine()
		}()

		atomic.AddInt32(&c.goroutines, 1)
		c.manager.resourceTracker.TrackGoroutine()

		for {
			select {
			case <-c.heartbeatTicker.C:
				c.performHeartbeat()
			case <-c.shutdownChan:
				return
			}
		}
	}()
}

// performHeartbeat sends a heartbeat and checks for missed heartbeats
func (c *EnhancedClient) performHeartbeat() {
	// Check for missed heartbeats
	lastHeartbeat := time.Unix(atomic.LoadInt64(&c.lastHeartbeat), 0)
	if time.Since(lastHeartbeat) > c.config.HeartbeatTimeout {
		missed := atomic.AddInt32(&c.missedHeartbeats, 1)
		c.manager.metrics.HeartbeatFailures.Inc()

		if missed > int32(c.config.MaxMissedHeartbeats) {
			c.logger.Warn("Too many missed heartbeats, closing connection",
				zap.Int32("missed", missed))
			c.Close(websocket.CloseNormalClosure, "Heartbeat timeout")
			return
		}
	}

	// Send ping
	c.conn.SetWriteDeadline(time.Now().Add(c.config.ConnectionTimeout))
	if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
		c.logger.Warn("Failed to send heartbeat ping", zap.Error(err))
		c.setState(StateError)
	}
}

// SendMessage queues a message for sending
func (c *EnhancedClient) SendMessage(message *Message) error {
	if c.GetState() != StateConnected {
		return fmt.Errorf("connection not in connected state")
	}

	select {
	case c.send <- message:
		return nil
	default:
		return fmt.Errorf("send buffer full")
	}
}

// ReceiveMessage receives a message from the client
func (c *EnhancedClient) ReceiveMessage() (*Message, error) {
	select {
	case message := <-c.receive:
		return message, nil
	case <-c.shutdownChan:
		return nil, fmt.Errorf("connection closed")
	}
}

// Close gracefully closes the connection
func (c *EnhancedClient) Close(code int, text string) error {
	c.closeOnce.Do(func() {
		c.logger.Info("Closing WebSocket connection",
			zap.Int("code", code),
			zap.String("text", text))

		c.setState(StateClosing)

		// Signal shutdown to all goroutines
		close(c.shutdownChan)

		// Send close message
		if c.conn != nil {
			c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			c.conn.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(code, text))
		}

		// Perform cleanup
		c.performCleanup()

		// Set final state
		c.setState(StateClosed)

		// Unregister from manager
		if c.manager != nil {
			c.manager.UnregisterConnection(c.id)
		}

		// Signal completion
		close(c.doneChan)
	})

	return nil
}

// performCleanup executes all cleanup functions
func (c *EnhancedClient) performCleanup() {
	c.cleanupMu.Lock()
	defer c.cleanupMu.Unlock()

	// Close channels
	if c.send != nil {
		close(c.send)
	}
	if c.receive != nil {
		close(c.receive)
	}
	if c.control != nil {
		close(c.control)
	}

	// Close WebSocket connection
	if c.conn != nil {
		c.conn.Close()
	}

	// Execute cleanup functions
	for _, cleanupFunc := range c.cleanupFuncs {
		func() {
			defer func() {
				if r := recover(); r != nil {
					c.logger.Error("Cleanup function panic", zap.Any("panic", r))
				}
			}()
			cleanupFunc()
		}()
	}

	// Wait for goroutines to finish (with timeout)
	timeout := time.After(5 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			c.logger.Warn("Cleanup timeout, some goroutines may still be running",
				zap.Int32("remaining_goroutines", atomic.LoadInt32(&c.goroutines)))
			return
		case <-ticker.C:
			if atomic.LoadInt32(&c.goroutines) == 0 {
				return
			}
		}
	}
}

// addCleanupFunc adds a function to be called during cleanup
func (c *EnhancedClient) addCleanupFunc(fn func()) {
	c.cleanupMu.Lock()
	defer c.cleanupMu.Unlock()
	c.cleanupFuncs = append(c.cleanupFuncs, fn)
}

// GetID returns the client ID
func (c *EnhancedClient) GetID() string {
	return c.id
}

// GetState returns the current connection state
func (c *EnhancedClient) GetState() ConnectionState {
	return ConnectionState(atomic.LoadInt32(&c.state))
}

// setState updates the connection state
func (c *EnhancedClient) setState(state ConnectionState) {
	atomic.StoreInt32(&c.state, int32(state))
}

// GetRemoteAddr returns the remote address
func (c *EnhancedClient) GetRemoteAddr() string {
	if c.remoteAddr != "" {
		return c.remoteAddr
	}
	if c.conn != nil {
		addr := c.conn.RemoteAddr()
		if addr != nil {
			if tcpAddr, ok := addr.(*net.TCPAddr); ok {
				return tcpAddr.IP.String()
			}
			return addr.String()
		}
	}
	return "unknown"
}

// GetLastActivity returns the timestamp of the last activity
func (c *EnhancedClient) GetLastActivity() time.Time {
	return time.Unix(atomic.LoadInt64(&c.lastActivity), 0)
}

// updateLastActivity updates the last activity timestamp
func (c *EnhancedClient) updateLastActivity() {
	atomic.StoreInt64(&c.lastActivity, time.Now().Unix())
	c.metrics.LastActivity = time.Now()
}

// updateLastHeartbeat updates the last heartbeat timestamp
func (c *EnhancedClient) updateLastHeartbeat() {
	atomic.StoreInt64(&c.lastHeartbeat, time.Now().Unix())
}

// GetMissedHeartbeats returns the number of missed heartbeats
func (c *EnhancedClient) GetMissedHeartbeats() int {
	return int(atomic.LoadInt32(&c.missedHeartbeats))
}

// GetMetrics returns connection metrics
func (c *EnhancedClient) GetMetrics() *ConnectionMetrics {
	// Update dynamic metrics
	c.metrics.ConnectionDuration = time.Since(c.createdAt)
	c.metrics.MessagesSent = atomic.LoadInt64(&c.metrics.MessagesSent)
	c.metrics.MessagesReceived = atomic.LoadInt64(&c.metrics.MessagesReceived)
	c.metrics.BytesSent = atomic.LoadInt64(&c.metrics.BytesSent)
	c.metrics.BytesReceived = atomic.LoadInt64(&c.metrics.BytesReceived)
	c.metrics.SubscriptionsCount = atomic.LoadInt32(&c.metrics.SubscriptionsCount)

	return c.metrics
}

// IsSubscribed checks if the client is subscribed to a topic
func (c *EnhancedClient) IsSubscribed(topic string) bool {
	_, exists := c.subscriptions.Load(topic)
	return exists
}

// GetSubscriptions returns all current subscriptions
func (c *EnhancedClient) GetSubscriptions() map[string]*SubscriptionInfo {
	subscriptions := make(map[string]*SubscriptionInfo)
	c.subscriptions.Range(func(key, value interface{}) bool {
		if topic, ok := key.(string); ok {
			if info, ok := value.(*SubscriptionInfo); ok {
				subscriptions[topic] = info
			}
		}
		return true
	})
	return subscriptions
}

// Wait waits for the connection to close
func (c *EnhancedClient) Wait() {
	<-c.doneChan
}
