package infrastructure

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/Aidin1998/finalex/internal/accounts"
	"github.com/Aidin1998/finalex/internal/fiat"
	"github.com/Aidin1998/finalex/internal/infrastructure/config"
	"github.com/Aidin1998/finalex/internal/infrastructure/messaging"
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

	// Messaging infrastructure
	messageBus    *messaging.MessageBus
	kafkaProducer *messaging.KafkaProducer
	msgConfig     *messaging.MessagingConfig
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
	// Initialize messaging configuration
	msgConfig := messaging.DefaultMessagingConfig()

	// Initialize messaging factory
	factory := messaging.NewMessagingFactory(msgConfig, logger)

	// Create message bus with both producer and consumer
	messageBus, err := factory.CreateMessageBus()
	if err != nil {
		logger.Warn("Failed to create message bus, messaging features will be limited", zap.Error(err))
		// Continue without messaging - don't fail service creation
	}

	// Initialize Kafka producer for direct event publishing
	var kafkaProducer *messaging.KafkaProducer
	if messageBus != nil {
		kafkaProducer, err = messaging.NewKafkaProducer(&msgConfig.Kafka, logger)
		if err != nil {
			logger.Warn("Failed to create Kafka producer", zap.Error(err))
		}
	}

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

		// Initialize messaging infrastructure
		messageBus:    messageBus,
		kafkaProducer: kafkaProducer,
		msgConfig:     msgConfig,
	}

	// Initialize route maps for different HTTP methods
	svc.handlers["GET"] = make(map[string]http.HandlerFunc)
	svc.handlers["POST"] = make(map[string]http.HandlerFunc)
	svc.handlers["PUT"] = make(map[string]http.HandlerFunc)
	svc.handlers["DELETE"] = make(map[string]http.HandlerFunc)
	svc.handlers["PATCH"] = make(map[string]http.HandlerFunc)

	// Initialize messaging system if available
	if messageBus != nil {
		if err := svc.initializeMessaging(); err != nil {
			logger.Warn("Failed to initialize messaging system", zap.Error(err))
		}
	}

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
	if s.messageBus == nil {
		s.logger.Warn("Messaging bus not available, event not published",
			zap.String("topic", topic))
		return fmt.Errorf("messaging bus not initialized")
	}

	// Convert topic string to Topic type and publish through message bus
	kafkaTopic := messaging.Topic(topic)

	// Generate a key for the message (can be improved based on event type)
	key := fmt.Sprintf("event_%d", time.Now().UnixNano())

	s.logger.Debug("Publishing event to messaging system",
		zap.String("topic", topic),
		zap.String("key", key),
		zap.String("event_type", fmt.Sprintf("%T", event)))

	err := s.kafkaProducer.Publish(ctx, kafkaTopic, key, event)
	if err != nil {
		s.logger.Error("Failed to publish event",
			zap.String("topic", topic),
			zap.Error(err))
		return fmt.Errorf("failed to publish event to topic %s: %w", topic, err)
	}

	s.logger.Info("Event published successfully",
		zap.String("topic", topic),
		zap.String("key", key))

	return nil
}

// SubscribeToEvents subscribes to events from a messaging system
func (s *serviceImpl) SubscribeToEvents(ctx context.Context, topic string, handler func(event interface{})) error {
	if s.messageBus == nil {
		s.logger.Warn("Messaging bus not available, cannot subscribe",
			zap.String("topic", topic))
		return fmt.Errorf("messaging bus not initialized")
	}

	s.logger.Info("Setting up event subscription",
		zap.String("topic", topic))

	// Convert the generic handler to MessageHandler format
	messageHandler := func(ctx context.Context, msg *messaging.ReceivedMessage) error {
		s.logger.Debug("Received message from subscription",
			zap.String("topic", msg.Topic),
			zap.String("key", msg.Key),
			zap.Int("value_size", len(msg.Value)))

		// Deserialize the message based on topic type
		var event interface{}
		if err := json.Unmarshal(msg.Value, &event); err != nil {
			s.logger.Error("Failed to deserialize message",
				zap.String("topic", msg.Topic),
				zap.String("key", msg.Key),
				zap.Error(err))
			return err
		}

		// Call the user-provided handler
		handler(event)
		return nil
	}

	// Register the handler with the message bus
	s.messageBus.RegisterHandler(messaging.MessageType(topic), messageHandler)

	s.logger.Info("Successfully registered event subscription",
		zap.String("topic", topic))

	return nil
}

// LoadConfig loads configuration from a file
func (s *serviceImpl) LoadConfig(ctx context.Context, configPath string) (map[string]interface{}, error) {
	// For now, load default configuration and convert to map
	// In a production system, this would load from the specified file path
	conf, err := config.LoadConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Convert the structured config to a generic map
	configMap := make(map[string]interface{})
	configMap["server"] = conf.Server
	configMap["database"] = conf.Database
	configMap["redis"] = conf.Redis
	configMap["kafka"] = conf.Kafka
	configMap["jwt"] = conf.JWT
	configMap["kyc"] = conf.KYC

	s.mu.Lock()
	defer s.mu.Unlock()

	s.config = configMap
	return configMap, nil
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

	s.logger.Info("Stopping infrastructure service")

	// Stop messaging system first
	if s.messageBus != nil {
		if err := s.messageBus.Stop(); err != nil {
			s.logger.Error("Failed to stop message bus", zap.Error(err))
		}
	}

	if s.kafkaProducer != nil {
		if err := s.kafkaProducer.Close(); err != nil {
			s.logger.Error("Failed to close Kafka producer", zap.Error(err))
		}
	}

	// Stop the servers
	if err := s.StopWSServer(ctx); err != nil {
		s.logger.Error("Failed to stop WebSocket server", zap.Error(err))
	}

	if err := s.StopServer(ctx); err != nil {
		s.logger.Error("Failed to stop HTTP server", zap.Error(err))
	}

	s.logger.Info("Infrastructure service stopped successfully")
	return nil
}

// initializeMessaging sets up the messaging system including topic creation and consumer startup
func (s *serviceImpl) initializeMessaging() error {
	if s.messageBus == nil {
		return fmt.Errorf("message bus not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	s.logger.Info("Initializing messaging system")

	// Create messaging factory for topic management
	factory := messaging.NewMessagingFactory(s.msgConfig, s.logger)

	// Create required topics
	if err := factory.CreateTopics(ctx); err != nil {
		s.logger.Warn("Failed to create topics, continuing without topic auto-creation", zap.Error(err))
		// Don't fail initialization if topic creation fails
	}

	// Start message consumers with a default group ID
	groupID := "infrastructure-service"
	if err := s.messageBus.StartConsumers(groupID); err != nil {
		s.logger.Warn("Failed to start message consumers", zap.Error(err))
		// Don't fail initialization if consumer startup fails
	}

	s.logger.Info("Messaging system initialized successfully")
	return nil
}
