// Enhanced market data server with integrated backpressure management
package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/auth"
	"github.com/Aidin1998/pincex_unified/internal/marketdata/backpressure"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// EnhancedHub provides market data distribution with comprehensive backpressure management
type EnhancedHub struct {
	// Original hub functionality
	*Hub

	// Backpressure management
	backpressureManager *backpressure.BackpressureManager
	wsIntegrator        *backpressure.WebSocketIntegrator

	// Enhanced client management
	enhancedClients sync.Map // clientID -> *EnhancedClient
	clientIDCounter int64

	// Configuration
	config *EnhancedHubConfig
	logger *zap.Logger

	// Lifecycle management
	ctx        context.Context
	cancel     context.CancelFunc
	shutdownWG sync.WaitGroup
}

// EnhancedClient extends the original Client with backpressure capabilities
type EnhancedClient struct {
	*Client
	ID              string
	BackpressureCtx *backpressure.ManagedClient
	LastActivity    time.Time
	MessageCount    int64
	BytesSent       int64
	Classification  backpressure.ClientType
}

// EnhancedHubConfig configures the enhanced hub
type EnhancedHubConfig struct {
	// Backpressure configuration
	BackpressureConfigPath string `json:"backpressure_config_path"`

	// Enhanced features
	EnablePerformanceMonitoring bool `json:"enable_performance_monitoring"`
	EnableAdaptiveRateLimiting  bool `json:"enable_adaptive_rate_limiting"`
	EnableCrossServiceCoord     bool `json:"enable_cross_service_coordination"`

	// Client management
	AutoClientClassification bool          `json:"auto_client_classification"`
	ClientTimeoutExtended    time.Duration `json:"client_timeout_extended"`
	MaxClientsPerInstance    int           `json:"max_clients_per_instance"`

	// Performance tuning
	MessageBatchSize    int  `json:"message_batch_size"`
	BroadcastCoalescing bool `json:"broadcast_coalescing"`
	CoalescingWindowMs  int  `json:"coalescing_window_ms"`
}

// NewEnhancedHub creates a new market data hub with backpressure management
func NewEnhancedHub(authService auth.AuthService, config *EnhancedHubConfig, logger *zap.Logger) (*EnhancedHub, error) {
	if config == nil {
		config = getDefaultEnhancedHubConfig()
	}

	if logger == nil {
		logger = zap.NewNop()
	}

	// Create original hub
	originalHub := NewHub(authService)

	// Load backpressure configuration
	backpressureConfig, err := backpressure.LoadBackpressureConfig(config.BackpressureConfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load backpressure config: %w", err)
	}

	// Create backpressure manager
	backpressureManager, err := backpressure.NewBackpressureManager(backpressureConfig, logger.Named("backpressure"))
	if err != nil {
		return nil, fmt.Errorf("failed to create backpressure manager: %w", err)
	}

	// Create WebSocket integrator
	wsIntegrator := backpressure.NewWebSocketIntegrator(backpressureManager, &backpressureConfig.WebSocket, logger.Named("ws_integrator"))

	ctx, cancel := context.WithCancel(context.Background())

	enhancedHub := &EnhancedHub{
		Hub:                 originalHub,
		backpressureManager: backpressureManager,
		wsIntegrator:        wsIntegrator,
		config:              config,
		logger:              logger,
		ctx:                 ctx,
		cancel:              cancel,
	}

	return enhancedHub, nil
}

// Start initializes and starts the enhanced hub with backpressure management
func (h *EnhancedHub) Start(ctx context.Context) error {
	h.logger.Info("Starting enhanced market data hub with backpressure management")

	// Start backpressure manager
	if err := h.backpressureManager.Start(ctx); err != nil {
		return fmt.Errorf("failed to start backpressure manager: %w", err)
	}

	// Start WebSocket integrator
	if err := h.wsIntegrator.Start(ctx); err != nil {
		return fmt.Errorf("failed to start WebSocket integrator: %w", err)
	}

	// Start original hub
	go h.Hub.Run()

	// Start enhanced monitoring
	if h.config.EnablePerformanceMonitoring {
		h.shutdownWG.Add(1)
		go h.performanceMonitor()
	}

	h.logger.Info("Enhanced market data hub started successfully")
	return nil
}

// RegisterClientEnhanced registers a client with full backpressure management
func (h *EnhancedHub) RegisterClientEnhanced(conn *websocket.Conn, userID string) error {
	// Generate unique client ID
	clientID := fmt.Sprintf("%s_%d_%d", userID, time.Now().UnixNano(), h.clientIDCounter)
	h.clientIDCounter++

	h.logger.Debug("Registering enhanced client",
		zap.String("client_id", clientID),
		zap.String("user_id", userID))

	// Create original client
	originalClient := &Client{
		conn:      conn,
		channels:  make(map[string]bool),
		send:      make(chan []byte, 256),
		connected: time.Now(),
	}

	// Register with WebSocket integrator (which handles backpressure registration)
	if err := h.wsIntegrator.RegisterClient(clientID, conn); err != nil {
		return fmt.Errorf("failed to register client with WebSocket integrator: %w", err)
	}

	// Create enhanced client
	enhancedClient := &EnhancedClient{
		Client:         originalClient,
		ID:             clientID,
		LastActivity:   time.Now(),
		Classification: backpressure.ClientTypeUnknown, // Will be updated by auto-classification
	}

	// Store enhanced client
	h.enhancedClients.Store(clientID, enhancedClient)

	// Register with original hub
	h.Hub.clients.Store(originalClient, struct{}{})

	// Update metrics
	MarketDataConnections.Inc()

	h.logger.Info("Enhanced client registered successfully",
		zap.String("client_id", clientID),
		zap.String("user_id", userID))

	return nil
}

// UnregisterClientEnhanced removes a client from enhanced management
func (h *EnhancedHub) UnregisterClientEnhanced(conn *websocket.Conn) {
	h.logger.Debug("Unregistering enhanced client")

	// Find client by connection
	var clientID string
	var enhancedClient *EnhancedClient
	h.enhancedClients.Range(func(key, value interface{}) bool {
		client := value.(*EnhancedClient)
		if client.Client.conn == conn {
			clientID = key.(string)
			enhancedClient = client
			return false
		}
		return true
	})

	if clientID == "" {
		h.logger.Warn("Client not found for unregistration")
		return
	}

	// Unregister from WebSocket integrator
	h.wsIntegrator.UnregisterClient(clientID)

	// Remove from enhanced clients
	h.enhancedClients.Delete(clientID)

	// Remove from original hub
	h.Hub.clients.Delete(enhancedClient.Client)

	// Update metrics
	MarketDataConnections.Dec()

	h.logger.Info("Enhanced client unregistered",
		zap.String("client_id", clientID),
		zap.Int64("messages_sent", enhancedClient.MessageCount),
		zap.Int64("bytes_sent", enhancedClient.BytesSent))
}

// BroadcastEnhanced broadcasts a message using backpressure management
func (h *EnhancedHub) BroadcastEnhanced(messageType string, data interface{}) error {
	start := time.Now()

	// Use WebSocket integrator for backpressure-managed broadcast
	if err := h.wsIntegrator.BroadcastMessage(messageType, data); err != nil {
		MarketDataDroppedMessages.Inc()
		return fmt.Errorf("failed to broadcast message: %w", err)
	}

	// Update metrics
	MarketDataMessages.Inc()
	MarketDataBroadcastLatency.Observe(time.Since(start).Seconds())

	return nil
}

// BroadcastToChannelEnhanced broadcasts a message to a specific channel with backpressure management
func (h *EnhancedHub) BroadcastToChannelEnhanced(channel, messageType string, data interface{}) error {
	start := time.Now()

	// Serialize message
	messageData, err := h.serializeMessage(messageType, data)
	if err != nil {
		return fmt.Errorf("failed to serialize message: %w", err)
	}

	// Use WebSocket integrator for channel-specific broadcast
	if err := h.wsIntegrator.BroadcastMessage(messageType, messageData); err != nil {
		MarketDataDroppedMessages.Inc()
		return fmt.Errorf("failed to broadcast to channel %s: %w", channel, err)
	}

	// Update metrics
	MarketDataMessages.Inc()
	MarketDataBroadcastLatency.Observe(time.Since(start).Seconds())

	return nil
}

// SendToClientEnhanced sends a message to a specific client with backpressure handling
func (h *EnhancedHub) SendToClientEnhanced(clientID string, messageType string, data interface{}) error {
	messageData, err := h.serializeMessage(messageType, data)
	if err != nil {
		return fmt.Errorf("failed to serialize message: %w", err)
	}

	return h.wsIntegrator.SendToClient(clientID, messageType, messageData)
}

// GetConnectedClientsCount returns the number of connected clients
func (h *EnhancedHub) GetConnectedClientsCount() int {
	count := 0
	h.enhancedClients.Range(func(key, value interface{}) bool {
		count++
		return true
	})
	return count
}

// GetClientStats returns detailed statistics for all connected clients
func (h *EnhancedHub) GetClientStats() []map[string]interface{} {
	var stats []map[string]interface{}

	h.enhancedClients.Range(func(key, value interface{}) bool {
		clientID := key.(string)
		client := value.(*EnhancedClient)

		clientStat := map[string]interface{}{
			"id":             clientID,
			"connected_at":   client.connected,
			"last_activity":  client.LastActivity,
			"message_count":  client.MessageCount,
			"bytes_sent":     client.BytesSent,
			"classification": int(client.Classification),
		}

		// Add capability info if available
		if capability, err := h.getClientCapabilityWrapper(clientID); err == nil && capability != nil {
			clientStat["capability"] = map[string]interface{}{
				"bandwidth":        capability.CurrentBandwidth,
				"processing_speed": capability.CurrentProcessingSpeed,
				"latency":          capability.CurrentLatency,
			}
		}

		stats = append(stats, clientStat)
		return true
	})

	return stats
}

// GetPerformanceStats returns overall performance statistics
func (h *EnhancedHub) GetPerformanceStats() map[string]interface{} {
	// Calculate aggregated statistics
	var totalBandwidth int64
	var totalProcessingSpeed int64
	var totalLatency int64
	var clientCount int
	var activeConnections int

	h.enhancedClients.Range(func(key, value interface{}) bool {
		clientID := key.(string)
		client := value.(*EnhancedClient)
		clientCount++

		// Check if client is active (sent message in last minute)
		if time.Since(client.LastActivity) < time.Minute {
			activeConnections++
		}

		// Add capability metrics
		if capability, err := h.getClientCapabilityWrapper(clientID); err == nil && capability != nil {
			totalBandwidth += capability.CurrentBandwidth
			totalProcessingSpeed += capability.CurrentProcessingSpeed
			totalLatency += capability.CurrentLatency
		}

		return true
	})

	avgLatency := int64(0)
	avgBandwidth := int64(0)
	avgProcessingSpeed := int64(0)

	if clientCount > 0 {
		avgLatency = totalLatency / int64(clientCount)
		avgBandwidth = totalBandwidth / int64(clientCount)
		avgProcessingSpeed = totalProcessingSpeed / int64(clientCount)
	}

	return map[string]interface{}{
		"total_clients":                  clientCount,
		"active_clients":                 activeConnections,
		"avg_latency_ns":                 avgLatency,
		"avg_bandwidth_bps":              avgBandwidth,
		"avg_processing_speed":           avgProcessingSpeed,
		"emergency_mode":                 h.isEmergencyModeWrapper(),
		"performance_class_distribution": h.getClientClassDistribution(),
	}
}

// getClientClassDistribution returns distribution of client performance classes
func (h *EnhancedHub) getClientClassDistribution() map[string]int {
	distribution := map[string]int{
		"HFT":           0,
		"MarketMaker":   0,
		"Institutional": 0,
		"Retail":        0,
		"Unknown":       0,
	}

	h.enhancedClients.Range(func(key, value interface{}) bool {
		client := value.(*EnhancedClient)

		switch client.Classification {
		case backpressure.ClientTypeHFT:
			distribution["HFT"]++
		case backpressure.ClientTypeMarketMaker:
			distribution["MarketMaker"]++
		case backpressure.ClientTypeInstitutional:
			distribution["Institutional"]++
		case backpressure.ClientTypeRetail:
			distribution["Retail"]++
		default:
			distribution["Unknown"]++
		}

		return true
	})

	return distribution
}

// performanceMonitor continuously monitors system performance
func (h *EnhancedHub) performanceMonitor() {
	defer h.shutdownWG.Done()

	ticker := time.NewTicker(time.Second * 10)
	defer ticker.Stop()

	h.logger.Debug("Starting performance monitor")

	for {
		select {
		case <-h.ctx.Done():
			h.logger.Debug("Performance monitor shutting down")
			return

		case <-ticker.C:
			h.collectPerformanceMetrics()
		}
	}
}

// collectPerformanceMetrics gathers and updates performance metrics
func (h *EnhancedHub) collectPerformanceMetrics() {
	var activeClients int

	h.enhancedClients.Range(func(key, value interface{}) bool {
		clientID := key.(string)
		client := value.(*EnhancedClient)
		activeClients++

		// Update client activity
		client.LastActivity = time.Now()

		// Auto-classify client if enabled
		if h.config.AutoClientClassification {
			if capability, err := h.getClientCapabilityWrapper(clientID); err == nil && capability != nil {
				newClass := h.classifyClient(capability)
				if newClass != client.Classification {
					client.Classification = newClass
					h.logger.Debug("Client reclassified",
						zap.String("client_id", clientID),
						zap.Int("old_class", int(client.Classification)),
						zap.Int("new_class", int(newClass)))
				}
			}
		}

		return true
	})

	h.logger.Debug("Performance metrics collected", zap.Int("active_clients", activeClients))
}

// classifyClient automatically classifies a client based on performance
func (h *EnhancedHub) classifyClient(capability *backpressure.ClientCapability) backpressure.ClientType {
	// Classification logic based on thresholds
	if capability.CurrentBandwidth > 10*1024*1024 && // 10MB/s
		capability.CurrentProcessingSpeed > 10000 && // 10k msgs/s
		capability.CurrentLatency < int64(5*time.Millisecond) { // < 5ms
		return backpressure.ClientTypeHFT
	}

	if capability.CurrentBandwidth > 5*1024*1024 && // 5MB/s
		capability.CurrentProcessingSpeed > 5000 && // 5k msgs/s
		capability.CurrentLatency < int64(10*time.Millisecond) { // < 10ms
		return backpressure.ClientTypeMarketMaker
	}

	if capability.CurrentBandwidth > 2*1024*1024 && // 2MB/s
		capability.CurrentProcessingSpeed > 2000 && // 2k msgs/s
		capability.CurrentLatency < int64(25*time.Millisecond) { // < 25ms
		return backpressure.ClientTypeInstitutional
	}

	return backpressure.ClientTypeRetail
}

// Helper methods to wrap functionality that may not be available
func (h *EnhancedHub) getClientCapabilityWrapper(clientID string) (*backpressure.ClientCapability, error) {
	// Try to get capability from detector directly
	if h.backpressureManager != nil {
		return h.backpressureManager.GetClientCapability(clientID)
	}
	return nil, fmt.Errorf("backpressure manager not available")
}

func (h *EnhancedHub) isEmergencyModeWrapper() bool {
	if h.backpressureManager != nil {
		return h.backpressureManager.IsEmergencyMode()
	}
	return false
}

func (h *EnhancedHub) serializeMessage(messageType string, data interface{}) ([]byte, error) {
	message := map[string]interface{}{
		"type":      messageType,
		"data":      data,
		"timestamp": time.Now(),
	}

	return json.Marshal(message)
}

// Stop gracefully shuts down the enhanced hub
func (h *EnhancedHub) Stop(ctx context.Context) error {
	h.logger.Info("Stopping enhanced market data hub")

	// Cancel context
	h.cancel()

	// Stop WebSocket integrator
	if h.wsIntegrator != nil {
		if err := h.wsIntegrator.Stop(); err != nil {
			h.logger.Error("Error stopping WebSocket integrator", zap.Error(err))
		}
	}

	// Stop backpressure manager
	if h.backpressureManager != nil {
		if err := h.backpressureManager.Stop(); err != nil {
			h.logger.Error("Error stopping backpressure manager", zap.Error(err))
		}
	}

	// Wait for monitoring goroutines
	h.shutdownWG.Wait()

	h.logger.Info("Enhanced market data hub stopped")
	return nil
}

func getDefaultEnhancedHubConfig() *EnhancedHubConfig {
	return &EnhancedHubConfig{
		BackpressureConfigPath:      "configs/backpressure.yaml",
		EnablePerformanceMonitoring: true,
		EnableAdaptiveRateLimiting:  true,
		EnableCrossServiceCoord:     true,
		AutoClientClassification:    true,
		ClientTimeoutExtended:       time.Minute * 10,
		MaxClientsPerInstance:       10000,
		MessageBatchSize:            100,
		BroadcastCoalescing:         true,
		CoalescingWindowMs:          10,
	}
}
