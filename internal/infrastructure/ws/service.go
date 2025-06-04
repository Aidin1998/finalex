package ws

import (
	"context"
	"net/http"
	"sync"

	"go.uber.org/zap"
)

// Service manages WebSocket connections and message broadcasting
type Service struct {
	hub    *Hub
	server *http.Server
	logger *zap.Logger
	routes map[string]http.HandlerFunc
	mu     sync.RWMutex
}

// NewService creates a new WebSocket service
func NewService(logger *zap.Logger) *Service {
	hubConfig := DefaultConfig()
	hub := NewHub(logger, hubConfig)

	return &Service{
		hub:    hub,
		logger: logger,
		routes: make(map[string]http.HandlerFunc),
	}
}

// Start starts the WebSocket service on the specified port
func (s *Service) Start(port string) error {
	mux := http.NewServeMux()

	// Register the default WebSocket handler
	mux.HandleFunc("/ws", s.hub.ServeWS)

	// Register custom handlers
	s.mu.RLock()
	for path, handler := range s.routes {
		mux.HandleFunc(path, handler)
	}
	s.mu.RUnlock()

	// Create HTTP server
	s.server = &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	// Start server in a goroutine
	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("WebSocket server error", zap.Error(err))
		}
	}()

	s.logger.Info("WebSocket server started", zap.String("port", port))
	return nil
}

// Stop stops the WebSocket service
func (s *Service) Stop(ctx context.Context) error {
	// Shutdown the HTTP server
	if s.server != nil {
		if err := s.server.Shutdown(ctx); err != nil {
			s.logger.Error("Error shutting down WebSocket server", zap.Error(err))
			return err
		}
	}

	// Shutdown the WebSocket hub
	if err := s.hub.Shutdown(ctx); err != nil {
		s.logger.Error("Error shutting down WebSocket hub", zap.Error(err))
		return err
	}

	s.logger.Info("WebSocket service stopped")
	return nil
}

// Broadcast broadcasts a message to all clients subscribed to a topic
func (s *Service) Broadcast(topic string, data interface{}) {
	s.hub.Broadcast(topic, data)
}

// RegisterHandler registers a custom WebSocket handler for a path
func (s *Service) RegisterHandler(path string, handler http.HandlerFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.routes[path] = handler

	// Update server routes if already running
	if s.server != nil {
		// We can't modify routes once the server is running
		// This is a limitation that would require restarting the server
		s.logger.Warn("Cannot add WebSocket route while server is running", zap.String("path", path))
	}
}

// DefaultHandler returns the default WebSocket handler
func (s *Service) DefaultHandler() http.HandlerFunc {
	return s.hub.ServeWS
}

// SetBufferSize sets the buffer size for a topic
func (s *Service) SetBufferSize(topic string, size int) {
	s.hub.SetBufferSize(topic, size)
}
