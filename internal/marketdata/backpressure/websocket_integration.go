// WebSocket integration for backpressure management in market data distribution
package backpressure

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// WebSocketIntegrator integrates backpressure management with WebSocket connections
type WebSocketIntegrator struct {
	logger  *zap.Logger
	manager *BackpressureManager

	// WebSocket message handling
	messageHandlers map[string]MessageHandler
	handlerMutex    sync.RWMutex

	// Client connection management
	clientWriters sync.Map // string (clientID) -> *ClientWriter
	writeTimeout  time.Duration

	// Performance tracking
	lastMessageTime time.Time
	messageCount    int64
	bytesSent       int64

	// Configuration
	config *WebSocketConfig

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// ClientWriter handles WebSocket writes for a specific client with backpressure
type ClientWriter struct {
	ClientID   string
	Connection *websocket.Conn
	Manager    *BackpressureManager
	Logger     *zap.Logger

	// Write coordination
	writeMutex  sync.Mutex
	lastWrite   time.Time
	writeErrors int64

	// Performance metrics
	messagesWritten int64
	bytesWritten    int64
	writeLatency    time.Duration

	// Emergency handling
	inEmergency int32 // atomic bool
	emergencyAt time.Time
	dropCount   int64
}

// MessageHandler defines how different message types are processed
type MessageHandler func(clientID string, messageType string, data []byte) (*PriorityMessage, error)

// WebSocketConfig configures WebSocket integration
type WebSocketConfig struct {
	WriteTimeout      time.Duration `json:"write_timeout"`
	PingInterval      time.Duration `json:"ping_interval"`
	MaxMessageSize    int64         `json:"max_message_size"`
	EnableCompression bool          `json:"enable_compression"`

	// Emergency handling
	MaxWriteErrors    int           `json:"max_write_errors"`
	ErrorRecoveryTime time.Duration `json:"error_recovery_time"`

	// Performance tuning
	BufferSize          int `json:"buffer_size"`
	MaxConcurrentWrites int `json:"max_concurrent_writes"`
}

// NewWebSocketIntegrator creates a new WebSocket integrator
func NewWebSocketIntegrator(manager *BackpressureManager, config *WebSocketConfig, logger *zap.Logger) *WebSocketIntegrator {
	if config == nil {
		config = getDefaultWebSocketConfig()
	}

	if logger == nil {
		logger = zap.NewNop()
	}

	ctx, cancel := context.WithCancel(context.Background())

	integrator := &WebSocketIntegrator{
		logger:          logger,
		manager:         manager,
		messageHandlers: make(map[string]MessageHandler),
		writeTimeout:    config.WriteTimeout,
		config:          config,
		ctx:             ctx,
		cancel:          cancel,
		lastMessageTime: time.Now(),
	}

	// Register default message handlers
	integrator.registerDefaultHandlers()

	return integrator
}

// Start initializes the WebSocket integrator
func (w *WebSocketIntegrator) Start(ctx context.Context) error {
	w.logger.Info("Starting WebSocket integrator")

	// Start client writers monitor
	w.wg.Add(1)
	go w.clientWritersMonitor()

	w.logger.Info("WebSocket integrator started successfully")
	return nil
}

// RegisterClient registers a WebSocket client for backpressure-managed communication
func (w *WebSocketIntegrator) RegisterClient(clientID string, conn *websocket.Conn) error {
	w.logger.Debug("Registering WebSocket client", zap.String("client_id", clientID))

	// Register with backpressure manager
	if err := w.manager.RegisterClient(clientID, conn); err != nil {
		return fmt.Errorf("failed to register client with backpressure manager: %w", err)
	}

	// Create client writer
	writer := &ClientWriter{
		ClientID:   clientID,
		Connection: conn,
		Manager:    w.manager,
		Logger:     w.logger.Named("client_writer").With(zap.String("client_id", clientID)),
		lastWrite:  time.Now(),
	}

	// Store writer
	w.clientWriters.Store(clientID, writer)

	// Start writer goroutine
	w.wg.Add(1)
	go w.clientWriterWorker(writer)

	w.logger.Info("WebSocket client registered successfully", zap.String("client_id", clientID))
	return nil
}

// UnregisterClient removes a WebSocket client
func (w *WebSocketIntegrator) UnregisterClient(clientID string) {
	w.logger.Debug("Unregistering WebSocket client", zap.String("client_id", clientID))

	// Remove client writer
	if writerInterface, exists := w.clientWriters.LoadAndDelete(clientID); exists {
		writer := writerInterface.(*ClientWriter)
		w.logger.Info("WebSocket client unregistered",
			zap.String("client_id", clientID),
			zap.Int64("messages_written", writer.messagesWritten),
			zap.Int64("bytes_written", writer.bytesWritten),
			zap.Int64("write_errors", writer.writeErrors))
	}

	// Unregister from backpressure manager
	w.manager.UnregisterClient(clientID)
}

// BroadcastMessage broadcasts a message to all clients with backpressure handling
func (w *WebSocketIntegrator) BroadcastMessage(messageType string, data interface{}) error {
	// Serialize message
	jsonData, err := json.Marshal(map[string]interface{}{
		"type":      messageType,
		"data":      data,
		"timestamp": time.Now().UnixNano(),
	})
	if err != nil {
		return fmt.Errorf("failed to serialize message: %w", err)
	}

	// Determine message priority
	priority := w.getMessagePriority(messageType)

	// Distribute via backpressure manager
	if err := w.manager.DistributeMessage(priority, jsonData, nil); err != nil {
		w.logger.Error("Failed to distribute broadcast message",
			zap.String("message_type", messageType),
			zap.Error(err))
		return err
	}

	// Update metrics
	atomic.AddInt64(&w.messageCount, 1)
	atomic.AddInt64(&w.bytesSent, int64(len(jsonData)))
	w.lastMessageTime = time.Now()

	return nil
}

// SendToClient sends a message to a specific client with backpressure handling
func (w *WebSocketIntegrator) SendToClient(clientID string, messageType string, data interface{}) error {
	// Serialize message
	jsonData, err := json.Marshal(map[string]interface{}{
		"type":      messageType,
		"data":      data,
		"timestamp": time.Now().UnixNano(),
	})
	if err != nil {
		return fmt.Errorf("failed to serialize message: %w", err)
	}

	// Determine message priority
	priority := w.getMessagePriority(messageType)

	// Distribute via backpressure manager
	if err := w.manager.DistributeMessage(priority, jsonData, []string{clientID}); err != nil {
		w.logger.Error("Failed to distribute targeted message",
			zap.String("client_id", clientID),
			zap.String("message_type", messageType),
			zap.Error(err))
		return err
	}

	// Update metrics
	atomic.AddInt64(&w.messageCount, 1)
	atomic.AddInt64(&w.bytesSent, int64(len(jsonData)))

	return nil
}

// RegisterMessageHandler registers a custom message handler
func (w *WebSocketIntegrator) RegisterMessageHandler(messageType string, handler MessageHandler) {
	w.handlerMutex.Lock()
	defer w.handlerMutex.Unlock()

	w.messageHandlers[messageType] = handler
	w.logger.Debug("Registered message handler", zap.String("message_type", messageType))
}

// clientWriterWorker handles WebSocket writes for a specific client
func (w *WebSocketIntegrator) clientWriterWorker(writer *ClientWriter) {
	defer w.wg.Done()

	writer.Logger.Debug("Starting client writer worker")

	// Get client from manager to access send channel
	clientInterface, exists := w.manager.clients.Load(writer.ClientID)
	if !exists {
		writer.Logger.Error("Client not found in manager")
		return
	}

	client := clientInterface.(*ManagedClient)

	for {
		select {
		case <-w.ctx.Done():
			writer.Logger.Debug("Client writer worker shutting down")
			return

		case msg, ok := <-client.SendChannel:
			if !ok {
				writer.Logger.Debug("Client send channel closed")
				return
			}

			if err := writer.writeMessage(msg); err != nil {
				writer.Logger.Error("Failed to write message", zap.Error(err))
				atomic.AddInt64(&writer.writeErrors, 1)

				// Check if we should enter emergency mode for this client
				if atomic.LoadInt32(&writer.inEmergency) == 0 &&
					writer.writeErrors >= int64(w.config.MaxWriteErrors) {

					atomic.StoreInt32(&writer.inEmergency, 1)
					writer.emergencyAt = time.Now()

					writer.Logger.Warn("Client writer entered emergency mode",
						zap.Int64("write_errors", writer.writeErrors))
				}
			} else {
				// Successful write
				atomic.AddInt64(&writer.messagesWritten, 1)
				atomic.AddInt64(&writer.bytesWritten, int64(len(msg.Data)))
				writer.lastWrite = time.Now()

				// Check for recovery from emergency mode
				if atomic.LoadInt32(&writer.inEmergency) == 1 &&
					time.Since(writer.emergencyAt) > w.config.ErrorRecoveryTime {

					atomic.StoreInt32(&writer.inEmergency, 0)
					writer.Logger.Info("Client writer recovered from emergency mode")
				}
			}
		}
	}
}

// writeMessage writes a message to the WebSocket connection
func (w *ClientWriter) writeMessage(msg *PriorityMessage) error {
	w.writeMutex.Lock()
	defer w.writeMutex.Unlock()

	start := time.Now()

	// Set write deadline
	if err := w.Connection.SetWriteDeadline(time.Now().Add(time.Second * 10)); err != nil {
		return fmt.Errorf("failed to set write deadline: %w", err)
	}

	// Write message
	if err := w.Connection.WriteMessage(websocket.TextMessage, msg.Data); err != nil {
		return fmt.Errorf("failed to write WebSocket message: %w", err)
	}

	// Update latency
	w.writeLatency = time.Since(start)

	return nil
}

// clientWritersMonitor monitors client writers and handles cleanup
func (w *WebSocketIntegrator) clientWritersMonitor() {
	defer w.wg.Done()

	ticker := time.NewTicker(time.Second * 30)
	defer ticker.Stop()

	w.logger.Debug("Starting client writers monitor")

	for {
		select {
		case <-w.ctx.Done():
			w.logger.Debug("Client writers monitor shutting down")
			return

		case <-ticker.C:
			w.monitorClientWriters()
		}
	}
}

// monitorClientWriters checks health of client writers
func (w *WebSocketIntegrator) monitorClientWriters() {
	now := time.Now()

	w.clientWriters.Range(func(key, value interface{}) bool {
		clientID := key.(string)
		writer := value.(*ClientWriter)

		// Check for stale writers
		if now.Sub(writer.lastWrite) > time.Minute*5 {
			w.logger.Warn("Removing stale client writer",
				zap.String("client_id", clientID),
				zap.Duration("time_since_last_write", now.Sub(writer.lastWrite)))
			w.UnregisterClient(clientID)
		}

		return true
	})
}

// getMessagePriority determines message priority based on type
func (w *WebSocketIntegrator) getMessagePriority(messageType string) MessagePriority {
	switch messageType {
	case "trade":
		return PriorityCritical
	case "orderbook":
		return PriorityHigh
	case "orderbook_delta":
		return PriorityHigh
	case "ticker":
		return PriorityMedium
	case "candle":
		return PriorityLow
	default:
		return PriorityMarket
	}
}

// registerDefaultHandlers registers default message handlers
func (w *WebSocketIntegrator) registerDefaultHandlers() {
	// Trade message handler
	w.RegisterMessageHandler("trade", func(clientID string, messageType string, data []byte) (*PriorityMessage, error) {
		return &PriorityMessage{
			Priority:  PriorityCritical,
			Data:      data,
			Timestamp: time.Now(),
			Deadline:  time.Now().Add(time.Millisecond * 100),
			Metadata:  map[string]interface{}{"type": "trade"},
		}, nil
	})

	// Order book handler
	w.RegisterMessageHandler("orderbook", func(clientID string, messageType string, data []byte) (*PriorityMessage, error) {
		return &PriorityMessage{
			Priority:  PriorityHigh,
			Data:      data,
			Timestamp: time.Now(),
			Deadline:  time.Now().Add(time.Millisecond * 500),
			Metadata:  map[string]interface{}{"type": "orderbook"},
		}, nil
	})

	// Ticker handler
	w.RegisterMessageHandler("ticker", func(clientID string, messageType string, data []byte) (*PriorityMessage, error) {
		return &PriorityMessage{
			Priority:  PriorityMedium,
			Data:      data,
			Timestamp: time.Now(),
			Deadline:  time.Now().Add(time.Second * 2),
			Metadata:  map[string]interface{}{"type": "ticker"},
		}, nil
	})
}

// Stop gracefully shuts down the WebSocket integrator
func (w *WebSocketIntegrator) Stop() error {
	w.logger.Info("Stopping WebSocket integrator")

	// Cancel context
	w.cancel()

	// Wait for all workers to finish
	w.wg.Wait()

	w.logger.Info("WebSocket integrator stopped")
	return nil
}

// GetStats returns integration statistics
func (w *WebSocketIntegrator) GetStats() map[string]interface{} {
	var activeWriters int
	w.clientWriters.Range(func(key, value interface{}) bool {
		activeWriters++
		return true
	})

	return map[string]interface{}{
		"active_writers":  activeWriters,
		"messages_sent":   atomic.LoadInt64(&w.messageCount),
		"bytes_sent":      atomic.LoadInt64(&w.bytesSent),
		"last_message_at": w.lastMessageTime,
		"emergency_mode":  w.manager.IsEmergencyMode(),
	}
}

func getDefaultWebSocketConfig() *WebSocketConfig {
	return &WebSocketConfig{
		WriteTimeout:        time.Second * 10,
		PingInterval:        time.Second * 30,
		MaxMessageSize:      1024 * 1024, // 1MB
		EnableCompression:   true,
		MaxWriteErrors:      5,
		ErrorRecoveryTime:   time.Minute * 2,
		BufferSize:          1000,
		MaxConcurrentWrites: 100,
	}
}
