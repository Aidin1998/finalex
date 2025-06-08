package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/Aidin1998/finalex/internal/infrastructure/config"
	"github.com/Aidin1998/finalex/internal/infrastructure/middleware"
	"github.com/Aidin1998/finalex/internal/infrastructure/ratelimit"
	"github.com/Aidin1998/finalex/internal/infrastructure/ws"
	"go.uber.org/zap"
)

// WebSocketServer represents an enhanced WebSocket server that wraps the ws.Service
type WebSocketServer struct {
	config     *config.WSServerConfig
	logger     *zap.Logger
	wsService  *ws.Service
	middleware *middleware.UnifiedMiddleware
	server     *http.Server
	mu         sync.RWMutex
	running    bool
}

// WebSocketServerOptions contains options for creating a WebSocketServer
type WebSocketServerOptions struct {
	Config      *config.WSServerConfig
	Logger      *zap.Logger
	Middleware  *middleware.UnifiedMiddleware
	RateLimiter *ratelimit.RateLimitManager
}

// NewWebSocketServer creates a new enhanced WebSocket server
func NewWebSocketServer(opts WebSocketServerOptions) (*WebSocketServer, error) {
	if opts.Config == nil {
		return nil, fmt.Errorf("WebSocket server config is required")
	}
	if opts.Logger == nil {
		return nil, fmt.Errorf("logger is required")
	}

	// Create WebSocket service
	wsService := ws.NewService(opts.Logger)

	server := &WebSocketServer{
		config:     opts.Config,
		logger:     opts.Logger,
		wsService:  wsService,
		middleware: opts.Middleware,
		running:    false,
	}

	if err := server.setupServer(); err != nil {
		return nil, fmt.Errorf("failed to setup WebSocket server: %w", err)
	}

	return server, nil
}

// setupServer configures the HTTP server for WebSocket connections
func (ws *WebSocketServer) setupServer() error {
	mux := http.NewServeMux()

	// Default WebSocket handler
	wsHandler := ws.createWebSocketHandler()
	// Apply middleware if available
	var finalHandler http.Handler
	if ws.middleware != nil {
		finalHandler = ws.middleware.Handler()(wsHandler)
	} else {
		finalHandler = wsHandler
	}

	mux.Handle("/ws", finalHandler)

	// Health check endpoint
	mux.HandleFunc("/ws/health", ws.healthHandler)

	// Statistics endpoint
	mux.HandleFunc("/ws/stats", ws.statsHandler)

	ws.server = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", ws.config.Host, ws.config.Port),
		Handler:      mux,
		ReadTimeout:  ws.config.ReadTimeout,
		WriteTimeout: ws.config.WriteTimeout,
		IdleTimeout:  ws.config.IdleTimeout,
	}

	return nil
}

// createWebSocketHandler creates the main WebSocket handler
func (ws *WebSocketServer) createWebSocketHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Use the default handler from ws.Service which delegates to hub.ServeWS
		ws.wsService.DefaultHandler()(w, r)
	}
}

// extractClientID extracts client ID from the request
func (ws *WebSocketServer) extractClientID(r *http.Request) string {
	// Try query parameter first
	if clientID := r.URL.Query().Get("client_id"); clientID != "" {
		return clientID
	}

	// Try header
	if clientID := r.Header.Get("X-Client-ID"); clientID != "" {
		return clientID
	}

	// Try getting from user context if available
	if userID := r.Header.Get("X-User-ID"); userID != "" {
		return userID
	}

	// Generate a unique ID for anonymous connections
	return fmt.Sprintf("anon_%d", ws.generateConnectionID())
}

// generateConnectionID generates a unique connection ID
func (ws *WebSocketServer) generateConnectionID() int64 {
	// Simple counter - in production you might want UUID or similar
	ws.mu.Lock()
	defer ws.mu.Unlock()

	// Use current time as base for unique ID
	return time.Now().UnixNano()
}

// healthHandler handles health check requests
func (ws *WebSocketServer) healthHandler(w http.ResponseWriter, r *http.Request) {
	status := map[string]interface{}{
		"status":      "healthy",
		"service":     "websocket",
		"version":     "1.0.0",
		"connections": ws.GetConnectionCount(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// statsHandler handles statistics requests
func (ws *WebSocketServer) statsHandler(w http.ResponseWriter, r *http.Request) {
	stats := ws.GetStats()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// Start starts the WebSocket server
func (ws *WebSocketServer) Start(ctx context.Context) error {
	ws.mu.Lock()
	if ws.running {
		ws.mu.Unlock()
		return fmt.Errorf("WebSocket server is already running")
	}
	ws.running = true
	ws.mu.Unlock()

	ws.logger.Info("Starting WebSocket server",
		zap.String("host", ws.config.Host),
		zap.Int("port", ws.config.Port))

	// Start the underlying WebSocket service
	if err := ws.wsService.Start(fmt.Sprintf("%d", ws.config.Port)); err != nil {
		return fmt.Errorf("failed to start WebSocket service: %w", err)
	}

	// Start the HTTP server in a goroutine
	go func() {
		if err := ws.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			ws.logger.Error("WebSocket server error", zap.Error(err))
		}
	}()

	ws.logger.Info("WebSocket server started successfully")
	return nil
}

// Stop stops the WebSocket server gracefully
func (ws *WebSocketServer) Stop(ctx context.Context) error {
	ws.mu.Lock()
	if !ws.running {
		ws.mu.Unlock()
		return nil
	}
	ws.running = false
	ws.mu.Unlock()

	ws.logger.Info("Stopping WebSocket server...")

	// Create a context with timeout for graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(ctx, ws.config.ShutdownTimeout)
	defer cancel()

	// Stop the HTTP server
	if err := ws.server.Shutdown(shutdownCtx); err != nil {
		ws.logger.Error("Error shutting down WebSocket HTTP server", zap.Error(err))
	}

	// Stop the WebSocket service
	if err := ws.wsService.Stop(shutdownCtx); err != nil {
		ws.logger.Error("Error shutting down WebSocket service", zap.Error(err))
		return err
	}

	ws.logger.Info("WebSocket server stopped successfully")
	return nil
}

// Broadcast broadcasts a message to all clients subscribed to a topic
func (ws *WebSocketServer) Broadcast(topic string, data interface{}) error {
	ws.wsService.Broadcast(topic, data)
	return nil
}

// BroadcastToUser broadcasts a message to a specific user
func (ws *WebSocketServer) BroadcastToUser(userID string, data interface{}) error {
	// The ws package doesn't have user-specific broadcasting built-in,
	// but we can use topics with user prefixes
	topic := fmt.Sprintf("user:%s", userID)
	return ws.Broadcast(topic, data)
}

// RegisterHandler registers a custom WebSocket handler
func (ws *WebSocketServer) RegisterHandler(path string, handler http.HandlerFunc) {
	ws.wsService.RegisterHandler(path, handler)
}

// GetConnectionCount returns the current number of active connections
func (ws *WebSocketServer) GetConnectionCount() int {
	// Use the connection stats from the ws service
	stats := ws.wsService.GetConnectionStats()
	if count, ok := stats["active_connections"].(int); ok {
		return count
	}
	return 0
}

// GetStats returns WebSocket server statistics
func (ws *WebSocketServer) GetStats() map[string]interface{} {
	stats := ws.wsService.GetConnectionStats()

	// Add server-specific stats
	stats["service"] = "websocket"
	stats["host"] = ws.config.Host
	stats["port"] = ws.config.Port
	stats["max_connections"] = ws.config.MaxConnections
	stats["max_message_size"] = ws.config.MaxMessageSize
	stats["ping_interval"] = ws.config.PingInterval.String()
	stats["pong_timeout"] = ws.config.PongTimeout.String()
	stats["compression"] = ws.config.EnableCompression
	stats["running"] = ws.running

	return stats
}
