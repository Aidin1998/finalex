package integration

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/Aidin1998/pincex_unified/internal/infrastructure"
)

// IntegrationService handles integration with external systems
type IntegrationService struct {
	logger       *zap.Logger
	db           *gorm.DB
	infra        infrastructure.Service
	mu           sync.RWMutex
	running      bool
	integrations map[string]Integration
	eventQueue   chan IntegrationEvent
}

// Integration represents an external system integration
type Integration struct {
	ID       string                 `json:"id"`
	Name     string                 `json:"name"`
	Type     string                 `json:"type"` // webhook, api, message_queue
	Endpoint string                 `json:"endpoint"`
	Enabled  bool                   `json:"enabled"`
	Config   map[string]interface{} `json:"config"`
}

// IntegrationEvent represents an event to be sent to external systems
type IntegrationEvent struct {
	ID            string                 `json:"id"`
	Type          string                 `json:"type"` // trade_executed, order_placed, etc.
	Data          map[string]interface{} `json:"data"`
	TargetSystems []string               `json:"target_systems"`
	Timestamp     time.Time              `json:"timestamp"`
	Retries       int                    `json:"retries"`
}

// NewIntegrationService creates a new integration service
func NewIntegrationService(logger *zap.Logger, db *gorm.DB, infra infrastructure.Service) (*IntegrationService, error) {
	is := &IntegrationService{
		logger:       logger,
		db:           db,
		infra:        infra,
		integrations: make(map[string]Integration),
		eventQueue:   make(chan IntegrationEvent, 1000),
	}

	// Initialize default integrations
	is.initializeDefaultIntegrations()

	return is, nil
}

// initializeDefaultIntegrations sets up default integrations
func (is *IntegrationService) initializeDefaultIntegrations() {
	// Risk management system integration
	is.integrations["risk_system"] = Integration{
		ID:       "risk_system",
		Name:     "Risk Management System",
		Type:     "api",
		Endpoint: "http://risk-service:8080/api/events",
		Enabled:  true,
		Config: map[string]interface{}{
			"timeout":     30,
			"max_retries": 3,
		},
	}

	// Market data distribution
	is.integrations["market_data"] = Integration{
		ID:       "market_data",
		Name:     "Market Data Distribution",
		Type:     "websocket",
		Endpoint: "ws://marketdata-service:8080/ws",
		Enabled:  true,
		Config: map[string]interface{}{
			"buffer_size": 100,
			"batch_size":  10,
		},
	}

	// Compliance reporting
	is.integrations["compliance"] = Integration{
		ID:       "compliance",
		Name:     "Compliance Reporting",
		Type:     "webhook",
		Endpoint: "https://compliance-service/webhooks/trading",
		Enabled:  true,
		Config: map[string]interface{}{
			"signature_key": "compliance_webhook_key",
			"timeout":       60,
		},
	}
}

// PublishEvent publishes an event to external systems
func (is *IntegrationService) PublishEvent(ctx context.Context, eventType string, data map[string]interface{}, targetSystems []string) error {
	event := IntegrationEvent{
		ID:            fmt.Sprintf("event_%d", time.Now().UnixNano()),
		Type:          eventType,
		Data:          data,
		TargetSystems: targetSystems,
		Timestamp:     time.Now(),
		Retries:       0,
	}

	select {
	case is.eventQueue <- event:
		is.logger.Debug("Event queued for integration",
			zap.String("event_id", event.ID),
			zap.String("type", eventType),
			zap.Strings("targets", targetSystems))
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		return fmt.Errorf("event queue is full")
	}
}

// PublishTradeExecuted publishes a trade execution event
func (is *IntegrationService) PublishTradeExecuted(ctx context.Context, trade *Trade) error {
	data := map[string]interface{}{
		"trade_id":   trade.ID,
		"market_id":  trade.MarketID,
		"buyer_id":   trade.BuyerID,
		"seller_id":  trade.SellerID,
		"price":      trade.Price.String(),
		"amount":     trade.Amount.String(),
		"fee":        trade.Fee.String(),
		"created_at": trade.CreatedAt,
	}

	targets := []string{"risk_system", "market_data", "compliance"}
	return is.PublishEvent(ctx, "trade_executed", data, targets)
}

// PublishOrderPlaced publishes an order placement event
func (is *IntegrationService) PublishOrderPlaced(ctx context.Context, order *Order) error {
	data := map[string]interface{}{
		"order_id":   order.ID,
		"user_id":    order.UserID,
		"market_id":  order.MarketID,
		"side":       order.Side,
		"type":       order.Type,
		"price":      order.Price.String(),
		"amount":     order.Amount.String(),
		"status":     order.Status,
		"created_at": order.CreatedAt,
	}

	targets := []string{"risk_system", "market_data"}
	return is.PublishEvent(ctx, "order_placed", data, targets)
}

// processEvents processes events from the queue
func (is *IntegrationService) processEvents() {
	for is.running {
		select {
		case event := <-is.eventQueue:
			if err := is.processEvent(event); err != nil {
				is.logger.Error("Failed to process integration event",
					zap.String("event_id", event.ID),
					zap.Error(err))

				// Retry logic
				if event.Retries < 3 {
					event.Retries++
					time.Sleep(time.Second * time.Duration(event.Retries))

					select {
					case is.eventQueue <- event:
						// Event requeued for retry
					default:
						is.logger.Error("Failed to requeue event for retry",
							zap.String("event_id", event.ID))
					}
				}
			}
		case <-time.After(time.Second):
			// Periodic check to continue loop
			continue
		}
	}
}

// processEvent processes a single integration event
func (is *IntegrationService) processEvent(event IntegrationEvent) error {
	is.logger.Debug("Processing integration event",
		zap.String("event_id", event.ID),
		zap.String("type", event.Type),
		zap.Strings("targets", event.TargetSystems))

	for _, targetID := range event.TargetSystems {
		integration, exists := is.integrations[targetID]
		if !exists {
			is.logger.Warn("Integration target not found",
				zap.String("target_id", targetID))
			continue
		}

		if !integration.Enabled {
			is.logger.Debug("Integration target disabled",
				zap.String("target_id", targetID))
			continue
		}

		if err := is.sendToIntegration(event, integration); err != nil {
			return fmt.Errorf("failed to send to %s: %w", targetID, err)
		}
	}

	return nil
}

// sendToIntegration sends an event to a specific integration
func (is *IntegrationService) sendToIntegration(event IntegrationEvent, integration Integration) error {
	switch integration.Type {
	case "api":
		return is.sendToAPI(event, integration)
	case "webhook":
		return is.sendToWebhook(event, integration)
	case "websocket":
		return is.sendToWebSocket(event, integration)
	default:
		return fmt.Errorf("unsupported integration type: %s", integration.Type)
	}
}

// sendToAPI sends an event to an API endpoint
func (is *IntegrationService) sendToAPI(event IntegrationEvent, integration Integration) error {
	// Implementation would make HTTP API call
	is.logger.Debug("Sending event to API",
		zap.String("event_id", event.ID),
		zap.String("endpoint", integration.Endpoint))

	// Simulate API call
	time.Sleep(time.Millisecond * 10)
	return nil
}

// sendToWebhook sends an event to a webhook endpoint
func (is *IntegrationService) sendToWebhook(event IntegrationEvent, integration Integration) error {
	// Implementation would make HTTP POST to webhook
	is.logger.Debug("Sending event to webhook",
		zap.String("event_id", event.ID),
		zap.String("endpoint", integration.Endpoint))

	// Simulate webhook call
	time.Sleep(time.Millisecond * 15)
	return nil
}

// sendToWebSocket sends an event via WebSocket
func (is *IntegrationService) sendToWebSocket(event IntegrationEvent, integration Integration) error {
	// Implementation would send via WebSocket connection
	is.logger.Debug("Sending event to WebSocket",
		zap.String("event_id", event.ID),
		zap.String("endpoint", integration.Endpoint))

	// Use infrastructure service WebSocket capabilities
	if err := is.infra.BroadcastMessage(context.Background(), "integration_event", event.Data); err != nil {
		return fmt.Errorf("failed to broadcast via WebSocket: %w", err)
	}

	return nil
}

// AddIntegration adds a new integration
func (is *IntegrationService) AddIntegration(integration Integration) error {
	is.mu.Lock()
	defer is.mu.Unlock()

	is.integrations[integration.ID] = integration

	is.logger.Info("Integration added",
		zap.String("id", integration.ID),
		zap.String("name", integration.Name),
		zap.String("type", integration.Type))

	return nil
}

// UpdateIntegration updates an existing integration
func (is *IntegrationService) UpdateIntegration(id string, integration Integration) error {
	is.mu.Lock()
	defer is.mu.Unlock()

	if _, exists := is.integrations[id]; !exists {
		return fmt.Errorf("integration not found: %s", id)
	}

	is.integrations[id] = integration

	is.logger.Info("Integration updated",
		zap.String("id", id),
		zap.String("name", integration.Name))

	return nil
}

// RemoveIntegration removes an integration
func (is *IntegrationService) RemoveIntegration(id string) error {
	is.mu.Lock()
	defer is.mu.Unlock()

	if _, exists := is.integrations[id]; !exists {
		return fmt.Errorf("integration not found: %s", id)
	}

	delete(is.integrations, id)

	is.logger.Info("Integration removed", zap.String("id", id))
	return nil
}

// GetIntegrations returns all integrations
func (is *IntegrationService) GetIntegrations() map[string]Integration {
	is.mu.RLock()
	defer is.mu.RUnlock()

	// Return a copy to prevent external modification
	result := make(map[string]Integration)
	for k, v := range is.integrations {
		result[k] = v
	}

	return result
}

// Start starts the integration service
func (is *IntegrationService) Start() error {
	is.mu.Lock()
	defer is.mu.Unlock()

	is.running = true
	is.logger.Info("Integration service started")

	// Start event processing
	go is.processEvents()

	return nil
}

// Stop stops the integration service
func (is *IntegrationService) Stop() error {
	is.mu.Lock()
	defer is.mu.Unlock()

	is.running = false
	is.logger.Info("Integration service stopped")
	return nil
}
