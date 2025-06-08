package messaging

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Aidin1998/finalex/internal/accounts/bookkeeper"
	ws "github.com/Aidin1998/finalex/internal/infrastructure/ws"
	"go.uber.org/zap"
)

// MessagingFactory creates and configures the messaging system
type MessagingFactory struct {
	config *MessagingConfig
	logger *zap.Logger
}

// NewMessagingFactory creates a new messaging factory
func NewMessagingFactory(config *MessagingConfig, logger *zap.Logger) *MessagingFactory {
	if config == nil {
		config = DefaultMessagingConfig()
	}

	return &MessagingFactory{
		config: config,
		logger: logger,
	}
}

// CreateMessageBus creates and configures a complete message bus system
func (f *MessagingFactory) CreateMessageBus() (*MessageBus, error) {
	// Validate configuration
	if err := f.config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid messaging configuration: %w", err)
	}

	// Create Kafka producer
	producer, err := NewKafkaProducer(&f.config.Kafka, f.logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kafka producer: %w", err)
	}

	// Create Kafka consumer
	consumer, err := NewKafkaConsumer(&f.config.Kafka, f.logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kafka consumer: %w", err)
	}

	// Create message bus
	messageBus := NewMessageBus(producer, consumer, f.logger)

	f.logger.Info("Message bus created successfully",
		zap.Strings("brokers", f.config.Kafka.Brokers),
		zap.Bool("hft_enabled", f.config.HighFrequency.Enabled),
		zap.Int("topic_count", len(f.config.TopicConfig)))

	return messageBus, nil
}

// CreateTradingServices creates all message-driven trading services
func (f *MessagingFactory) CreateTradingServices(
	messageBus *MessageBus,
	bookkeeperSvc bookkeeper.BookkeeperService,
	wsHub *ws.Hub,
) (*TradingServices, error) {

	// Create bookkeeper message service
	bookkeeperMsgSvc := NewBookkeeperMessageService(bookkeeperSvc, messageBus, f.logger)

	// Create trading message service
	tradingMsgSvc := NewTradingMessageService(bookkeeperMsgSvc, messageBus, wsHub, f.logger)

	// Create market data message service
	marketDataMsgSvc := NewMarketDataMessageService(messageBus, wsHub, f.logger)

	// Create notification service
	notificationSvc := NewNotificationService(messageBus, f.logger)

	services := &TradingServices{
		BookkeeperService:   bookkeeperMsgSvc,
		TradingService:      tradingMsgSvc,
		MarketDataService:   marketDataMsgSvc,
		NotificationService: notificationSvc,
		MessageBus:          messageBus,
	}

	f.logger.Info("Trading services created successfully")
	return services, nil
}

// StartServices starts all messaging services and consumers
func (f *MessagingFactory) StartServices(services *TradingServices) error {
	// Start consumers for each service

	// Start trading engine consumers
	if err := services.MessageBus.StartConsumers("trading-engine"); err != nil {
		return fmt.Errorf("failed to start trading engine consumers: %w", err)
	}

	// Start bookkeeper consumers
	if err := services.MessageBus.StartConsumers("bookkeeper"); err != nil {
		return fmt.Errorf("failed to start bookkeeper consumers: %w", err)
	}

	// Start market data consumers
	if err := services.MessageBus.StartConsumers("market-data"); err != nil {
		return fmt.Errorf("failed to start market data consumers: %w", err)
	}

	// Start notification consumers
	if err := services.MessageBus.StartConsumers("notifications"); err != nil {
		return fmt.Errorf("failed to start notification consumers: %w", err)
	}

	f.logger.Info("All messaging services started successfully")
	return nil
}

// TradingServices contains all message-driven services
type TradingServices struct {
	BookkeeperService   *BookkeeperMessageService
	TradingService      *TradingMessageService
	MarketDataService   *MarketDataMessageService
	NotificationService *NotificationService
	MessageBus          *MessageBus
}

// Stop gracefully stops all services
func (ts *TradingServices) Stop() error {
	// Note: logger should be accessed through a factory reference or passed as parameter
	return ts.MessageBus.Stop()
}

// HealthCheck checks the health of all services
func (ts *TradingServices) HealthCheck() error {
	return ts.MessageBus.HealthCheck()
}

// NotificationService handles notifications via message queue
type NotificationService struct {
	messageBus *MessageBus
	logger     *zap.Logger
}

// NewNotificationService creates a new notification service
func NewNotificationService(messageBus *MessageBus, logger *zap.Logger) *NotificationService {
	service := &NotificationService{
		messageBus: messageBus,
		logger:     logger,
	}

	// Register handlers
	messageBus.RegisterHandler(MsgUserNotification, service.handleUserNotification)
	messageBus.RegisterHandler(MsgSystemAlert, service.handleSystemAlert)

	return service
}

// SendUserNotification sends a notification to a specific user
func (s *NotificationService) SendUserNotification(ctx context.Context, userID, title, message, level, category string, data map[string]interface{}) error {
	notification := &NotificationMessage{
		BaseMessage: NewBaseMessage(MsgUserNotification, "notification-service", ""),
		UserID:      userID,
		Title:       title,
		Message:     message,
		Level:       level,
		Category:    category,
		Data:        data,
	}

	s.logger.Info("Sending user notification",
		zap.String("user_id", userID),
		zap.String("title", title),
		zap.String("level", level))

	return s.messageBus.PublishNotification(ctx, notification)
}

// SendSystemAlert sends a system-wide alert
func (s *NotificationService) SendSystemAlert(ctx context.Context, title, message, level string, data map[string]interface{}) error {
	notification := &NotificationMessage{
		BaseMessage: NewBaseMessage(MsgSystemAlert, "system", ""),
		Title:       title,
		Message:     message,
		Level:       level,
		Category:    "system",
		Data:        data,
	}

	s.logger.Warn("Sending system alert",
		zap.String("title", title),
		zap.String("level", level))

	return s.messageBus.PublishNotification(ctx, notification)
}

// handleUserNotification processes user notification events
func (s *NotificationService) handleUserNotification(ctx context.Context, msg *ReceivedMessage) error {
	var notification NotificationMessage
	if err := json.Unmarshal(msg.Value, &notification); err != nil {
		return fmt.Errorf("failed to unmarshal notification: %w", err)
	}

	s.logger.Info("Processing user notification",
		zap.String("user_id", notification.UserID),
		zap.String("title", notification.Title),
		zap.String("level", notification.Level))

	// Here you would typically:
	// 1. Send email notifications
	// 2. Send push notifications
	// 3. Update notification center
	// 4. Send WebSocket notifications
	// 5. Store in database

	return nil
}

// handleSystemAlert processes system alert events
func (s *NotificationService) handleSystemAlert(ctx context.Context, msg *ReceivedMessage) error {
	var notification NotificationMessage
	if err := json.Unmarshal(msg.Value, &notification); err != nil {
		return fmt.Errorf("failed to unmarshal system alert: %w", err)
	}

	s.logger.Warn("Processing system alert",
		zap.String("title", notification.Title),
		zap.String("level", notification.Level))

	// Here you would typically:
	// 1. Send to monitoring systems
	// 2. Alert operations team
	// 3. Trigger automated responses
	// 4. Log to security systems

	return nil
}

// CreateTopics creates all required Kafka topics with proper configuration
func (f *MessagingFactory) CreateTopics(ctx context.Context) error {
	f.logger.Info("Creating Kafka topics with configuration")

	// Create admin client for topic management
	adminClient, err := NewKafkaAdminClient(&f.config.Kafka, f.logger)
	if err != nil {
		return fmt.Errorf("failed to create Kafka admin client: %w", err)
	}
	defer adminClient.Close()

	topics := []Topic{
		TopicOrderEvents,
		TopicTradeEvents,
		TopicBalanceEvents,
		TopicMarketData,
		TopicNotifications,
		TopicRiskEvents,
		TopicFundsOps,
	}

	// Convert string-keyed config to Topic-keyed config
	topicConfigs := make(map[Topic]TopicConfig)
	for _, topic := range topics {
		if config, exists := f.config.GetTopicConfig(topic); exists {
			topicConfigs[topic] = config
		} else {
			f.logger.Warn("No configuration found for topic, using defaults", zap.String("topic", string(topic)))
			topicConfigs[topic] = TopicConfig{
				Partitions:        12,
				ReplicationFactor: 3,
				RetentionMs:       7 * 24 * 60 * 60 * 1000, // 7 days
				CleanupPolicy:     "delete",
				CompressionType:   "snappy",
				MaxMessageBytes:   1048576,
			}
		}
	}

	// Create topics if they don't exist
	return adminClient.CreateTopicsIfNotExist(ctx, topics, topicConfigs)
}

// GetMetrics returns metrics for all services
func (ts *TradingServices) GetMetrics() map[string]interface{} {
	return map[string]interface{}{
		"message_bus": ts.MessageBus.GetMetrics(),
		"services": map[string]string{
			"bookkeeper":    "running",
			"trading":       "running",
			"market_data":   "running",
			"notifications": "running",
		},
	}
}
