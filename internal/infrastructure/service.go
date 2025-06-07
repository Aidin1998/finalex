package infrastructure

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"github.com/Aidin1998/finalex/internal/accounts"
	"github.com/Aidin1998/finalex/internal/fiat"
	"github.com/Aidin1998/finalex/internal/infrastructure/config"
	"github.com/Aidin1998/finalex/internal/infrastructure/server"
	"github.com/Aidin1998/finalex/internal/infrastructure/ws"
	"github.com/Aidin1998/finalex/internal/marketmaking"
	"github.com/Aidin1998/finalex/internal/trading"
	"github.com/Aidin1998/finalex/internal/userauth"
	"github.com/Aidin1998/finalex/internal/wallet"
	"go.uber.org/zap"
)

type serviceImpl struct {
	logger          *zap.Logger
	mu              sync.RWMutex
	config          map[string]interface{}
	userAuthSvc     userauth.Service
	accountsSvc     accounts.Service
	tradingSvc      trading.Service
	fiatSvc         fiat.Service
	walletSvc       wallet.Service
	marketMakingSvc marketmaking.Service
	serverSvc       *server.Service
	wsService       *ws.Service
	wsServer        *http.Server
	handlers        map[string]map[string]http.HandlerFunc
	middlewares     []func(http.Handler) http.Handler
}

// NewService creates a new consolidated infrastructure service
func NewService(
	logger *zap.Logger,
	userAuthSvc userauth.Service,
	accountsSvc accounts.Service,
	tradingSvc trading.Service,
	fiatSvc fiat.Service,
	walletSvc wallet.Service,
	marketMakingSvc marketmaking.Service,
) (Service, error) {
	svc := &serviceImpl{
		logger:          logger,
		userAuthSvc:     userAuthSvc,
		accountsSvc:     accountsSvc,
		tradingSvc:      tradingSvc,
		fiatSvc:         fiatSvc,
		walletSvc:       walletSvc,
		marketMakingSvc: marketMakingSvc,
		config:          make(map[string]interface{}),
		handlers:        make(map[string]map[string]http.HandlerFunc),
	}

	// Initialize route maps for different HTTP methods
	svc.handlers["GET"] = make(map[string]http.HandlerFunc)
	svc.handlers["POST"] = make(map[string]http.HandlerFunc)
	svc.handlers["PUT"] = make(map[string]http.HandlerFunc)
	svc.handlers["DELETE"] = make(map[string]http.HandlerFunc)
	svc.handlers["PATCH"] = make(map[string]http.HandlerFunc)

	return svc, nil
}

// StartServer starts the HTTP server
func (s *serviceImpl) StartServer(ctx context.Context, port string) error {
	s.logger.Info("Starting HTTP server", zap.String("port", port))

	serverConfig := &server.Config{
		Port:        port,
		CorsEnabled: true,
	}

	var err error
	s.serverSvc, err = server.NewService(
		s.logger,
		serverConfig,
		s.userAuthSvc,
		s.accountsSvc,
		s.tradingSvc,
		s.fiatSvc,
		s.walletSvc,
		s.marketMakingSvc,
	)
	if err != nil {
		return fmt.Errorf("failed to create server service: %w", err)
	}

	// Apply registered handlers and middlewares
	for method, routes := range s.handlers {
		for path, handler := range routes {
			s.serverSvc.RegisterHandler(path, method, handler)
		}
	}

	// Start the server
	return s.serverSvc.Start()
}

// StopServer stops the HTTP server
func (s *serviceImpl) StopServer(ctx context.Context) error {
	if s.serverSvc == nil {
		return nil
	}

	s.logger.Info("Stopping HTTP server")
	return s.serverSvc.Stop(ctx)
}

// RegisterHandler registers an HTTP handler for a specific path and method
func (s *serviceImpl) RegisterHandler(path string, method string, handler http.HandlerFunc) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	methodHandlers, ok := s.handlers[method]
	if !ok {
		s.handlers[method] = make(map[string]http.HandlerFunc)
		methodHandlers = s.handlers[method]
	}

	methodHandlers[path] = handler

	// If server is already running, register handler directly
	if s.serverSvc != nil {
		return s.serverSvc.RegisterHandler(path, method, handler)
	}

	return nil
}

// RegisterMiddleware registers middleware for the HTTP server
func (s *serviceImpl) RegisterMiddleware(middleware func(http.Handler) http.Handler) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.middlewares = append(s.middlewares, middleware)

	// If server is already running, register middleware directly
	if s.serverSvc != nil {
		// TODO: Implement middleware registration in server service
	}

	return nil
}

// StartWSServer starts the WebSocket server
func (s *serviceImpl) StartWSServer(ctx context.Context, port string) error {
	s.logger.Info("Starting WebSocket server", zap.String("port", port))

	// Create WebSocket service if not already created
	if s.wsService == nil {
		s.wsService = ws.NewService(s.logger)
	}

	// Start the WebSocket server
	return s.wsService.Start(port)
}

// StopWSServer stops the WebSocket server
func (s *serviceImpl) StopWSServer(ctx context.Context) error {
	if s.wsService == nil {
		return nil
	}

	s.logger.Info("Stopping WebSocket server")
	return s.wsService.Stop(ctx)
}

// BroadcastMessage broadcasts a message to all WebSocket clients on a channel
func (s *serviceImpl) BroadcastMessage(channel string, message interface{}) error {
	// If we don't have a WebSocket service, return an error
	if s.wsService == nil {
		return fmt.Errorf("WebSocket service not initialized")
	}

	s.wsService.Broadcast(channel, message)
	return nil
}

// RegisterWSHandler registers a WebSocket handler for a specific path
func (s *serviceImpl) RegisterWSHandler(path string, handler func(conn interface{})) error {
	// If we don't have a WebSocket service, return an error
	if s.wsService == nil {
		return fmt.Errorf("WebSocket service not initialized")
	}

	// Convert the handler to an http.HandlerFunc
	httpHandler := func(w http.ResponseWriter, r *http.Request) {
		// Use the default WebSocket handler, then call the custom handler
		s.wsService.DefaultHandler()(w, r)
		// The actual connection handling happens in the WebSocket hub
	}

	s.wsService.RegisterHandler(path, httpHandler)
	return nil
}

// PublishEvent publishes an event to a messaging system
func (s *serviceImpl) PublishEvent(ctx context.Context, topic string, event interface{}) error {
	// TODO: Implement event publishing
	return fmt.Errorf("Event publishing not implemented yet")
}

// SubscribeToEvents subscribes to events from a messaging system
func (s *serviceImpl) SubscribeToEvents(ctx context.Context, topic string, handler func(event interface{})) error {
	// TODO: Implement event subscription
	return fmt.Errorf("Event subscription not implemented yet")
}

// LoadConfig loads configuration from a file
func (s *serviceImpl) LoadConfig(ctx context.Context, configPath string) (map[string]interface{}, error) {
	conf, err := config.LoadConfigFromFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config from %s: %w", configPath, err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.config = conf
	return conf, nil
}

// GetConfigValue gets a configuration value by key
func (s *serviceImpl) GetConfigValue(key string) (interface{}, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if val, ok := s.config[key]; ok {
		return val, nil
	}

	return nil, fmt.Errorf("config key %s not found", key)
}

// SetConfigValue sets a configuration value by key
func (s *serviceImpl) SetConfigValue(key string, value interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.config[key] = value
	return nil
}

// BeginTransaction begins a transaction
func (s *serviceImpl) BeginTransaction(ctx context.Context) (context.Context, error) {
	// TODO: Implement transaction management
	return ctx, fmt.Errorf("Transaction management not implemented yet")
}

// CommitTransaction commits a transaction
func (s *serviceImpl) CommitTransaction(ctx context.Context) error {
	// TODO: Implement transaction management
	return fmt.Errorf("Transaction management not implemented yet")
}

// RollbackTransaction rolls back a transaction
func (s *serviceImpl) RollbackTransaction(ctx context.Context) error {
	// TODO: Implement transaction management
	return fmt.Errorf("Transaction management not implemented yet")
}

// Start starts the infrastructure service
func (s *serviceImpl) Start() error {
	// Start all services
	// This is a simplified implementation
	return s.StartServer(context.Background(), "8080")
}

// Stop stops the infrastructure service
func (s *serviceImpl) Stop() error {
	ctx := context.Background()

	// Stop the servers
	if err := s.StopWSServer(ctx); err != nil {
		s.logger.Error("Failed to stop WebSocket server", zap.Error(err))
	}

	if err := s.StopServer(ctx); err != nil {
		s.logger.Error("Failed to stop HTTP server", zap.Error(err))
	}

	return nil
}
