package crosspair

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// CrossPairEventPublisherImpl implements CrossPairEventPublisher and wires to real infra
// (WebSocketManager, EventBus, Compliance HookManager, Metrics)
type CrossPairEventPublisherImpl struct {
	WebSocketManager *WebSocketManager
	EventBus         EventBusInterface         // You may need to define or import this
	ComplianceHooks  ComplianceHookManager     // You may need to define or import this
	Metrics          MetricsCollectorInterface // Optional
}

func (p *CrossPairEventPublisherImpl) PublishCrossPairOrderEvent(ctx context.Context, event CrossPairOrderEvent) {
	// 1. WebSocket: Notify user (and optionally admin/monitoring)
	if p.WebSocketManager != nil {
		go func(ev CrossPairOrderEvent) {
			// Send to user (topic: "orders" or "crosspair.orders")
			p.WebSocketManager.broadcastToSubscribers("orders", ev)
			// Optionally: p.WebSocketManager.broadcastToSubscribers("admin", ev)
		}(event)
	}

	// 2. EventBus: Publish for admin/monitoring dashboards
	if p.EventBus != nil {
		go func(ev CrossPairOrderEvent) {
			payload, _ := json.Marshal(ev)
			event := EventBusEvent{
				ID:        uuid.New(),
				Type:      "crosspair.order_event",
				Source:    "crosspair",
				Data:      payload,
				Timestamp: time.Now(),
			}
			_ = p.EventBus.Publish(ctx, event)
		}(event)
	}

	// 3. Compliance: Log for audit/compliance
	if p.ComplianceHooks != nil {
		go func(ev CrossPairOrderEvent) {
			_ = p.ComplianceHooks.OnCrossPairOrderEvent(ctx, ev)
		}(event)
	}

	// 4. Metrics: Record event delivery
	if p.Metrics != nil {
		go func() {
			p.Metrics.IncrementCounter("crosspair_event_published", nil)
		}()
	}
}

// EventBusEvent is a generic event for the EventBus
// (You may want to use the platform's real event type)
type EventBusEvent struct {
	ID        uuid.UUID
	Type      string
	Source    string
	Data      []byte
	Timestamp time.Time
}

// EventBusInterface is a minimal interface for event bus publishing
// (Replace with the real one from infra/integration)
type EventBusInterface interface {
	Publish(ctx context.Context, event interface{}) error
}

// ComplianceHookManager is a minimal interface for compliance hooks
// (Replace with the real one from compliance/hooks)
type ComplianceHookManager interface {
	OnCrossPairOrderEvent(ctx context.Context, event CrossPairOrderEvent) error
}
