// Server integration for enhanced market data hub with backpressure management
package marketdata

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/auth"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// EnhancedMarketDataServer provides market data distribution with integrated backpressure management
type EnhancedMarketDataServer struct {
	enhancedHub *EnhancedHub
	logger      *zap.Logger
	config      *ServerConfig

	// Server state
	ctx    context.Context
	cancel context.CancelFunc
}

// ServerConfig provides configuration for the enhanced market data server
type ServerConfig struct {
	// Server settings
	Port         int           `json:"port" yaml:"port"`
	Host         string        `json:"host" yaml:"host"`
	ReadTimeout  time.Duration `json:"read_timeout" yaml:"read_timeout"`
	WriteTimeout time.Duration `json:"write_timeout" yaml:"write_timeout"`

	// WebSocket settings
	EnableCompression bool `json:"enable_compression" yaml:"enable_compression"`
	ReadBufferSize    int  `json:"read_buffer_size" yaml:"read_buffer_size"`
	WriteBufferSize   int  `json:"write_buffer_size" yaml:"write_buffer_size"`

	// Enhanced hub configuration
	EnhancedHubConfig *EnhancedHubConfig `json:"enhanced_hub_config" yaml:"enhanced_hub_config"`

	// Authentication
	RequireAuth         bool   `json:"require_auth" yaml:"require_auth"`
	AuthTokenHeader     string `json:"auth_token_header" yaml:"auth_token_header"`
	AuthTokenQueryParam string `json:"auth_token_query_param" yaml:"auth_token_query_param"`
}

// NewEnhancedMarketDataServer creates a new enhanced market data server
func NewEnhancedMarketDataServer(authService auth.AuthService, config *ServerConfig, logger *zap.Logger) (*EnhancedMarketDataServer, error) {
	if config == nil {
		config = getDefaultServerConfig()
	}

	if logger == nil {
		logger = zap.NewNop()
	}

	// Create enhanced hub
	enhancedHub, err := NewEnhancedHub(authService, config.EnhancedHubConfig, logger.Named("enhanced_hub"))
	if err != nil {
		return nil, fmt.Errorf("failed to create enhanced hub: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	server := &EnhancedMarketDataServer{
		enhancedHub: enhancedHub,
		logger:      logger,
		config:      config,
		ctx:         ctx,
		cancel:      cancel,
	}

	return server, nil
}

// Start initializes and starts the enhanced market data server
func (s *EnhancedMarketDataServer) Start(ctx context.Context) error {
	s.logger.Info("Starting enhanced market data server",
		zap.String("host", s.config.Host),
		zap.Int("port", s.config.Port))

	// Start enhanced hub
	if err := s.enhancedHub.Start(ctx); err != nil {
		return fmt.Errorf("failed to start enhanced hub: %w", err)
	}

	s.logger.Info("Enhanced market data server started successfully")
	return nil
}

// Stop gracefully shuts down the enhanced market data server
func (s *EnhancedMarketDataServer) Stop(ctx context.Context) error {
	s.logger.Info("Stopping enhanced market data server")

	// Cancel context
	s.cancel()

	// Stop enhanced hub
	if err := s.enhancedHub.Stop(ctx); err != nil {
		s.logger.Error("Error stopping enhanced hub", zap.Error(err))
		return err
	}

	s.logger.Info("Enhanced market data server stopped successfully")
	return nil
}

// SetupRoutes configures the HTTP routes for the enhanced market data server
func (s *EnhancedMarketDataServer) SetupRoutes(router *gin.Engine) {
	// WebSocket endpoint with enhanced backpressure management
	router.GET("/ws", s.handleWebSocketConnection)

	// Health check endpoint
	router.GET("/health", s.handleHealthCheck)

	// Metrics endpoint for backpressure monitoring
	router.GET("/metrics/backpressure", s.handleBackpressureMetrics)

	// Admin endpoints for backpressure management
	admin := router.Group("/admin")
	{
		admin.GET("/clients", s.handleListClients)
		admin.POST("/emergency-stop", s.handleEmergencyStop)
		admin.POST("/emergency-recovery", s.handleEmergencyRecovery)
		admin.GET("/system-status", s.handleSystemStatus)
	}
}

// handleWebSocketConnection handles WebSocket connections with enhanced backpressure management
func (s *EnhancedMarketDataServer) handleWebSocketConnection(c *gin.Context) {
	// Create upgrader with configuration
	upgrader := websocket.Upgrader{
		ReadBufferSize:    s.config.ReadBufferSize,
		WriteBufferSize:   s.config.WriteBufferSize,
		CheckOrigin:       func(r *http.Request) bool { return true },
		EnableCompression: s.config.EnableCompression,
	}

	// Upgrade connection
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		s.logger.Error("WebSocket upgrade failed", zap.Error(err))
		return
	}
	defer conn.Close()

	// Extract user information
	userID := s.extractUserID(c)
	if s.config.RequireAuth && userID == "" {
		s.logger.Warn("Unauthorized WebSocket connection attempt")
		conn.WriteMessage(websocket.CloseMessage, []byte("unauthorized"))
		return
	}

	// Register client with enhanced hub
	if err := s.enhancedHub.RegisterClientEnhanced(conn, userID); err != nil {
		s.logger.Error("Failed to register enhanced client",
			zap.String("user_id", userID),
			zap.Error(err))
		conn.WriteMessage(websocket.CloseMessage, []byte("registration_failed"))
		return
	}

	s.logger.Info("Enhanced WebSocket client connected",
		zap.String("user_id", userID),
		zap.String("remote_addr", c.Request.RemoteAddr))
}

// handleHealthCheck provides health status for the enhanced server
func (s *EnhancedMarketDataServer) handleHealthCheck(c *gin.Context) {
	status := map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now().UTC(),
		"server":    "enhanced_market_data",
		"backpressure": map[string]interface{}{
			"enabled": true,
			"status":  s.enhancedHub.backpressureManager.GetStatus(),
		},
	}

	c.JSON(http.StatusOK, status)
}

// handleBackpressureMetrics provides detailed backpressure metrics
func (s *EnhancedMarketDataServer) handleBackpressureMetrics(c *gin.Context) {
	metrics := s.enhancedHub.backpressureManager.GetMetrics()
	c.JSON(http.StatusOK, metrics)
}

// handleListClients provides information about connected clients
func (s *EnhancedMarketDataServer) handleListClients(c *gin.Context) {
	clients := s.enhancedHub.GetClientStats()
	c.JSON(http.StatusOK, map[string]interface{}{
		"clients": clients,
		"total":   len(clients),
	})
}

// handleEmergencyStop triggers emergency stop mode
func (s *EnhancedMarketDataServer) handleEmergencyStop(c *gin.Context) {
	reason := c.Query("reason")
	if reason == "" {
		reason = "Manual emergency stop"
	}

	s.enhancedHub.backpressureManager.TriggerEmergencyStop(reason)

	s.logger.Warn("Emergency stop triggered", zap.String("reason", reason))
	c.JSON(http.StatusOK, map[string]string{
		"status": "emergency_stop_triggered",
		"reason": reason,
	})
}

// handleEmergencyRecovery triggers emergency recovery
func (s *EnhancedMarketDataServer) handleEmergencyRecovery(c *gin.Context) {
	if err := s.enhancedHub.backpressureManager.RecoverFromEmergency(); err != nil {
		s.logger.Error("Emergency recovery failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, map[string]string{
			"status": "recovery_failed",
			"error":  err.Error(),
		})
		return
	}

	s.logger.Info("Emergency recovery completed")
	c.JSON(http.StatusOK, map[string]string{
		"status": "recovery_completed",
	})
}

// handleSystemStatus provides comprehensive system status
func (s *EnhancedMarketDataServer) handleSystemStatus(c *gin.Context) {
	status := map[string]interface{}{
		"server": map[string]interface{}{
			"status":     "running",
			"start_time": time.Now().UTC(), // TODO: Track actual start time
		},
		"backpressure": s.enhancedHub.backpressureManager.GetStatus(),
		"clients":      s.enhancedHub.GetClientStats(),
		"performance":  s.enhancedHub.GetPerformanceStats(),
	}

	c.JSON(http.StatusOK, status)
}

// extractUserID extracts user ID from the request context
func (s *EnhancedMarketDataServer) extractUserID(c *gin.Context) string {
	// Try to get from JWT claims first
	if userID, exists := c.Get("user_id"); exists {
		if id, ok := userID.(string); ok {
			return id
		}
	}

	// Fallback to token extraction
	token := c.Query(s.config.AuthTokenQueryParam)
	if token == "" {
		token = c.GetHeader(s.config.AuthTokenHeader)
		if strings.HasPrefix(token, "Bearer ") {
			token = token[7:]
		}
	}

	if token != "" {
		// TODO: Extract user ID from token
		// For now, return a placeholder
		return fmt.Sprintf("user_%s", token[:8])
	}

	return "anonymous"
}

// BroadcastMessage broadcasts a message through the enhanced hub
func (s *EnhancedMarketDataServer) BroadcastMessage(msgType string, data interface{}) {
	s.enhancedHub.BroadcastEnhanced(msgType, data)
}

// BroadcastToChannel broadcasts a message to a specific channel
func (s *EnhancedMarketDataServer) BroadcastToChannel(channel, msgType string, data interface{}) {
	s.enhancedHub.BroadcastToChannelEnhanced(channel, msgType, data)
}

// GetConnectedClients returns the number of connected clients
func (s *EnhancedMarketDataServer) GetConnectedClients() int {
	return s.enhancedHub.GetConnectedClientsCount()
}

// GetBackpressureStatus returns the current backpressure status
func (s *EnhancedMarketDataServer) GetBackpressureStatus() map[string]interface{} {
	return s.enhancedHub.backpressureManager.GetStatus()
}

// Default configuration functions
func getDefaultServerConfig() *ServerConfig {
	return &ServerConfig{
		Port:                8080,
		Host:                "0.0.0.0",
		ReadTimeout:         30 * time.Second,
		WriteTimeout:        30 * time.Second,
		EnableCompression:   true,
		ReadBufferSize:      1024,
		WriteBufferSize:     1024,
		EnhancedHubConfig:   getDefaultEnhancedHubConfig(),
		RequireAuth:         false,
		AuthTokenHeader:     "Authorization",
		AuthTokenQueryParam: "token",
	}
}
