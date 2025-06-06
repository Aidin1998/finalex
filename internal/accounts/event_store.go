// Event Store implementation for event sourcing and audit trail
package accounts

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// Event represents a domain event in the system
type Event interface {
	GetID() uuid.UUID
	GetType() string
	GetAggregateID() string
	GetVersion() int64
	GetTimestamp() time.Time
	GetPayload() interface{}
	GetMetadata() map[string]interface{}
}

// BaseEvent provides common event functionality
type BaseEvent struct {
	ID          uuid.UUID              `json:"id"`
	Type        string                 `json:"type"`
	AggregateID string                 `json:"aggregate_id"`
	Version     int64                  `json:"version"`
	Timestamp   time.Time              `json:"timestamp"`
	Payload     interface{}            `json:"payload"`
	Metadata    map[string]interface{} `json:"metadata"`
}

func (e *BaseEvent) GetID() uuid.UUID                    { return e.ID }
func (e *BaseEvent) GetType() string                     { return e.Type }
func (e *BaseEvent) GetAggregateID() string              { return e.AggregateID }
func (e *BaseEvent) GetVersion() int64                   { return e.Version }
func (e *BaseEvent) GetTimestamp() time.Time             { return e.Timestamp }
func (e *BaseEvent) GetPayload() interface{}             { return e.Payload }
func (e *BaseEvent) GetMetadata() map[string]interface{} { return e.Metadata }

// Account Domain Events

// BalanceUpdatedEvent represents a balance update event
type BalanceUpdatedEvent struct {
	BaseEvent
	UserID       uuid.UUID       `json:"user_id"`
	Currency     string          `json:"currency"`
	BalanceDelta decimal.Decimal `json:"balance_delta"`
	LockedDelta  decimal.Decimal `json:"locked_delta"`
	Type         string          `json:"transaction_type"`
	ReferenceID  string          `json:"reference_id"`
	TraceID      string          `json:"trace_id"`
	Timestamp    time.Time       `json:"timestamp"`
}

// ReservationCreatedEvent represents a reservation creation event
type ReservationCreatedEvent struct {
	BaseEvent
	ReservationID uuid.UUID       `json:"reservation_id"`
	UserID        uuid.UUID       `json:"user_id"`
	Currency      string          `json:"currency"`
	Amount        decimal.Decimal `json:"amount"`
	Type          string          `json:"type"`
	ReferenceID   string          `json:"reference_id"`
	TraceID       string          `json:"trace_id"`
	Timestamp     time.Time       `json:"timestamp"`
}

// ReservationReleasedEvent represents a reservation release event
type ReservationReleasedEvent struct {
	BaseEvent
	ReservationID uuid.UUID       `json:"reservation_id"`
	UserID        uuid.UUID       `json:"user_id"`
	Currency      string          `json:"currency"`
	Amount        decimal.Decimal `json:"amount"`
	TraceID       string          `json:"trace_id"`
	Timestamp     time.Time       `json:"timestamp"`
}

// AccountCreatedEvent represents an account creation event
type AccountCreatedEvent struct {
	BaseEvent
	AccountID   uuid.UUID `json:"account_id"`
	UserID      uuid.UUID `json:"user_id"`
	Currency    string    `json:"currency"`
	AccountType string    `json:"account_type"`
	TraceID     string    `json:"trace_id"`
	Timestamp   time.Time `json:"timestamp"`
}

// EventStore manages event persistence and streaming
type EventStore struct {
	repository *Repository
	cache      *CacheLayer
	logger     *zap.Logger
	metrics    *EventStoreMetrics

	// Event streaming
	subscribers   map[string][]EventSubscriber
	subscribersMu sync.RWMutex

	// Event batching
	eventBatch    []Event
	batchMu       sync.Mutex
	batchSize     int
	flushInterval time.Duration

	// Background processing
	flushTicker  *time.Ticker
	stopFlushing chan struct{}

	// Snapshot management
	snapshotInterval int
	lastSnapshot     map[string]int64 // aggregate_id -> last_snapshot_version
	snapshotMu       sync.RWMutex
}

// EventStoreMetrics holds Prometheus metrics for the event store
type EventStoreMetrics struct {
	EventsPublished     *prometheus.CounterVec
	EventsProcessed     *prometheus.CounterVec
	EventProcessingTime *prometheus.HistogramVec
	EventBatchSize      prometheus.Histogram
	EventStoreLatency   *prometheus.HistogramVec
	SnapshotsCreated    *prometheus.CounterVec
	SubscriberCount     prometheus.Gauge
}

// EventSubscriber represents an event subscriber
type EventSubscriber interface {
	Handle(ctx context.Context, event Event) error
	GetSubscriptionID() string
	GetEventTypes() []string
}

// EventSnapshot represents an aggregate snapshot
type EventSnapshot struct {
	ID            uuid.UUID `json:"id" gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	AggregateID   string    `json:"aggregate_id" gorm:"type:varchar(255);index:idx_snapshot_aggregate;not null"`
	AggregateType string    `json:"aggregate_type" gorm:"type:varchar(100);not null"`
	Version       int64     `json:"version" gorm:"not null"`
	Data          []byte    `json:"data" gorm:"type:jsonb;not null"`
	CreatedAt     time.Time `json:"created_at" gorm:"index:idx_snapshot_created"`
}

// StoredEvent represents a persisted event
type StoredEvent struct {
	ID            uuid.UUID `json:"id" gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	EventType     string    `json:"event_type" gorm:"type:varchar(100);index:idx_event_type;not null"`
	AggregateID   string    `json:"aggregate_id" gorm:"type:varchar(255);index:idx_event_aggregate;not null"`
	AggregateType string    `json:"aggregate_type" gorm:"type:varchar(100);not null"`
	Version       int64     `json:"version" gorm:"not null"`
	Data          []byte    `json:"data" gorm:"type:jsonb;not null"`
	Metadata      []byte    `json:"metadata" gorm:"type:jsonb"`
	CreatedAt     time.Time `json:"created_at" gorm:"index:idx_event_created"`
}

// NewEventStore creates a new event store
func NewEventStore(repository *Repository, logger *zap.Logger) (*EventStore, error) {
	metrics := &EventStoreMetrics{
		EventsPublished: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "account_events_published_total",
				Help: "Total number of events published",
			},
			[]string{"event_type", "status"},
		),
		EventsProcessed: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "account_events_processed_total",
				Help: "Total number of events processed",
			},
			[]string{"event_type", "status"},
		),
		EventProcessingTime: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "account_event_processing_time_seconds",
				Help:    "Time taken to process events",
				Buckets: prometheus.ExponentialBuckets(0.001, 2, 15), // 1ms to ~32s
			},
			[]string{"event_type"},
		),
		EventBatchSize: promauto.NewHistogram(
			prometheus.HistogramOpts{
				Name:    "account_event_batch_size",
				Help:    "Size of event batches processed",
				Buckets: prometheus.LinearBuckets(1, 10, 20), // 1 to 200
			},
		),
		EventStoreLatency: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "account_event_store_latency_seconds",
				Help:    "Latency of event store operations",
				Buckets: prometheus.ExponentialBuckets(0.001, 2, 12), // 1ms to ~4s
			},
			[]string{"operation"},
		),
		SnapshotsCreated: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "account_snapshots_created_total",
				Help: "Total number of snapshots created",
			},
			[]string{"aggregate_type", "status"},
		),
		SubscriberCount: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "account_event_subscribers",
				Help: "Number of active event subscribers",
			},
		),
	}

	es := &EventStore{
		repository:       repository,
		logger:           logger,
		metrics:          metrics,
		subscribers:      make(map[string][]EventSubscriber),
		eventBatch:       make([]Event, 0, 100),
		batchSize:        100,
		flushInterval:    1 * time.Second,
		stopFlushing:     make(chan struct{}),
		snapshotInterval: 100, // Create snapshot every 100 events
		lastSnapshot:     make(map[string]int64),
	}

	// Start background flushing
	es.flushTicker = time.NewTicker(es.flushInterval)
	go es.backgroundFlush()

	return es, nil
}

// PublishEvent publishes an event to the event store
func (es *EventStore) PublishEvent(ctx context.Context, event Event) error {
	start := time.Now()
	defer func() {
		es.metrics.EventStoreLatency.WithLabelValues("publish").Observe(time.Since(start).Seconds())
	}()

	// Add to batch
	es.batchMu.Lock()
	es.eventBatch = append(es.eventBatch, event)
	shouldFlush := len(es.eventBatch) >= es.batchSize
	es.batchMu.Unlock()

	// Flush if batch is full
	if shouldFlush {
		if err := es.flushEvents(ctx); err != nil {
			es.metrics.EventsPublished.WithLabelValues(event.GetType(), "failed").Inc()
			return fmt.Errorf("failed to flush events: %w", err)
		}
	}

	// Notify subscribers asynchronously
	go es.notifySubscribers(ctx, event)

	es.metrics.EventsPublished.WithLabelValues(event.GetType(), "success").Inc()
	return nil
}

// Subscribe adds an event subscriber
func (es *EventStore) Subscribe(subscriber EventSubscriber) {
	es.subscribersMu.Lock()
	defer es.subscribersMu.Unlock()

	for _, eventType := range subscriber.GetEventTypes() {
		es.subscribers[eventType] = append(es.subscribers[eventType], subscriber)
	}

	// Update metrics
	totalSubscribers := 0
	for _, subs := range es.subscribers {
		totalSubscribers += len(subs)
	}
	es.metrics.SubscriberCount.Set(float64(totalSubscribers))

	es.logger.Info("Event subscriber added",
		zap.String("subscription_id", subscriber.GetSubscriptionID()),
		zap.Strings("event_types", subscriber.GetEventTypes()))
}

// Unsubscribe removes an event subscriber
func (es *EventStore) Unsubscribe(subscriptionID string) {
	es.subscribersMu.Lock()
	defer es.subscribersMu.Unlock()

	for eventType, subscribers := range es.subscribers {
		newSubscribers := make([]EventSubscriber, 0, len(subscribers))
		for _, sub := range subscribers {
			if sub.GetSubscriptionID() != subscriptionID {
				newSubscribers = append(newSubscribers, sub)
			}
		}
		es.subscribers[eventType] = newSubscribers
	}

	// Update metrics
	totalSubscribers := 0
	for _, subs := range es.subscribers {
		totalSubscribers += len(subs)
	}
	es.metrics.SubscriberCount.Set(float64(totalSubscribers))

	es.logger.Info("Event subscriber removed", zap.String("subscription_id", subscriptionID))
}

// GetEvents retrieves events for an aggregate
func (es *EventStore) GetEvents(ctx context.Context, aggregateID string, fromVersion int64) ([]Event, error) {
	start := time.Now()
	defer func() {
		es.metrics.EventStoreLatency.WithLabelValues("get_events").Observe(time.Since(start).Seconds())
	}()

	// TODO: Implement database query to get events
	// This would query the stored_events table
	es.logger.Debug("Getting events for aggregate",
		zap.String("aggregate_id", aggregateID),
		zap.Int64("from_version", fromVersion))

	return []Event{}, nil
}

// CreateSnapshot creates an aggregate snapshot
func (es *EventStore) CreateSnapshot(ctx context.Context, aggregateID string, aggregateType string, version int64, data interface{}) error {
	start := time.Now()
	defer func() {
		es.metrics.EventStoreLatency.WithLabelValues("create_snapshot").Observe(time.Since(start).Seconds())
	}()

	// Serialize data
	serializedData, err := json.Marshal(data)
	if err != nil {
		es.metrics.SnapshotsCreated.WithLabelValues(aggregateType, "failed").Inc()
		return fmt.Errorf("failed to serialize snapshot data: %w", err)
	}

	// Create snapshot record
	snapshot := &EventSnapshot{
		ID:            uuid.New(),
		AggregateID:   aggregateID,
		AggregateType: aggregateType,
		Version:       version,
		Data:          serializedData,
		CreatedAt:     time.Now(),
	}

	// TODO: Implement database save
	// This would save to the event_snapshots table
	es.logger.Info("Snapshot created",
		zap.String("aggregate_id", aggregateID),
		zap.String("aggregate_type", aggregateType),
		zap.Int64("version", version))

	// Update last snapshot tracking
	es.snapshotMu.Lock()
	es.lastSnapshot[aggregateID] = version
	es.snapshotMu.Unlock()

	es.metrics.SnapshotsCreated.WithLabelValues(aggregateType, "success").Inc()
	return nil
}

// GetSnapshot retrieves the latest snapshot for an aggregate
func (es *EventStore) GetSnapshot(ctx context.Context, aggregateID string) (*EventSnapshot, error) {
	start := time.Now()
	defer func() {
		es.metrics.EventStoreLatency.WithLabelValues("get_snapshot").Observe(time.Since(start).Seconds())
	}()

	// TODO: Implement database query to get latest snapshot
	es.logger.Debug("Getting snapshot for aggregate", zap.String("aggregate_id", aggregateID))

	return nil, nil
}

// Stop stops the event store
func (es *EventStore) Stop(ctx context.Context) error {
	// Stop background flushing
	if es.flushTicker != nil {
		es.flushTicker.Stop()
	}
	close(es.stopFlushing)

	// Flush remaining events
	if err := es.flushEvents(ctx); err != nil {
		es.logger.Error("Failed to flush remaining events during shutdown", zap.Error(err))
	}

	es.logger.Info("Event store stopped")
	return nil
}

// flushEvents persists batched events to storage
func (es *EventStore) flushEvents(ctx context.Context) error {
	es.batchMu.Lock()
	events := make([]Event, len(es.eventBatch))
	copy(events, es.eventBatch)
	es.eventBatch = es.eventBatch[:0] // Clear batch
	es.batchMu.Unlock()

	if len(events) == 0 {
		return nil
	}

	start := time.Now()
	defer func() {
		es.metrics.EventBatchSize.Observe(float64(len(events)))
		es.metrics.EventStoreLatency.WithLabelValues("flush").Observe(time.Since(start).Seconds())
	}()

	// Convert events to stored events
	storedEvents := make([]*StoredEvent, len(events))
	for i, event := range events {
		data, err := json.Marshal(event.GetPayload())
		if err != nil {
			es.logger.Error("Failed to marshal event payload", zap.Error(err))
			continue
		}

		metadata, err := json.Marshal(event.GetMetadata())
		if err != nil {
			metadata = []byte("{}")
		}

		storedEvents[i] = &StoredEvent{
			ID:            event.GetID(),
			EventType:     event.GetType(),
			AggregateID:   event.GetAggregateID(),
			AggregateType: "account", // Default for now
			Version:       event.GetVersion(),
			Data:          data,
			Metadata:      metadata,
			CreatedAt:     event.GetTimestamp(),
		}
	}

	// TODO: Implement batch insert to database
	// This would insert into the stored_events table
	es.logger.Debug("Flushed events to storage", zap.Int("count", len(events)))

	// Check if snapshots are needed
	for _, event := range events {
		es.checkAndCreateSnapshot(ctx, event)
	}

	return nil
}

// backgroundFlush periodically flushes events
func (es *EventStore) backgroundFlush() {
	for {
		select {
		case <-es.stopFlushing:
			return
		case <-es.flushTicker.C:
			if err := es.flushEvents(context.Background()); err != nil {
				es.logger.Error("Background event flush failed", zap.Error(err))
			}
		}
	}
}

// notifySubscribers notifies all subscribers of an event
func (es *EventStore) notifySubscribers(ctx context.Context, event Event) {
	es.subscribersMu.RLock()
	subscribers := es.subscribers[event.GetType()]
	es.subscribersMu.RUnlock()

	for _, subscriber := range subscribers {
		go func(sub EventSubscriber) {
			start := time.Now()
			defer func() {
				es.metrics.EventProcessingTime.WithLabelValues(event.GetType()).Observe(time.Since(start).Seconds())
			}()

			if err := sub.Handle(ctx, event); err != nil {
				es.metrics.EventsProcessed.WithLabelValues(event.GetType(), "failed").Inc()
				es.logger.Error("Event subscriber failed to handle event",
					zap.String("subscription_id", sub.GetSubscriptionID()),
					zap.String("event_type", event.GetType()),
					zap.Error(err))
			} else {
				es.metrics.EventsProcessed.WithLabelValues(event.GetType(), "success").Inc()
			}
		}(subscriber)
	}
}

// checkAndCreateSnapshot checks if a snapshot should be created for an aggregate
func (es *EventStore) checkAndCreateSnapshot(ctx context.Context, event Event) {
	es.snapshotMu.RLock()
	lastVersion, exists := es.lastSnapshot[event.GetAggregateID()]
	es.snapshotMu.RUnlock()

	var shouldSnapshot bool
	if !exists {
		shouldSnapshot = event.GetVersion() >= int64(es.snapshotInterval)
	} else {
		shouldSnapshot = event.GetVersion()-lastVersion >= int64(es.snapshotInterval)
	}

	if shouldSnapshot {
		// TODO: Implement aggregate reconstruction and snapshot creation
		es.logger.Debug("Snapshot needed",
			zap.String("aggregate_id", event.GetAggregateID()),
			zap.Int64("version", event.GetVersion()))
	}
}

// Helper function to create domain events

// NewBalanceUpdatedEvent creates a new balance updated event
func NewBalanceUpdatedEvent(userID uuid.UUID, currency string, balanceDelta, lockedDelta decimal.Decimal, txType, referenceID, traceID string) *BalanceUpdatedEvent {
	return &BalanceUpdatedEvent{
		BaseEvent: BaseEvent{
			ID:          uuid.New(),
			Type:        "balance_updated",
			AggregateID: fmt.Sprintf("account:%s:%s", userID.String(), currency),
			Version:     1, // This should be incremented based on aggregate version
			Timestamp:   time.Now(),
			Metadata:    make(map[string]interface{}),
		},
		UserID:       userID,
		Currency:     currency,
		BalanceDelta: balanceDelta,
		LockedDelta:  lockedDelta,
		Type:         txType,
		ReferenceID:  referenceID,
		TraceID:      traceID,
		Timestamp:    time.Now(),
	}
}

// NewReservationCreatedEvent creates a new reservation created event
func NewReservationCreatedEvent(reservationID, userID uuid.UUID, currency string, amount decimal.Decimal, resType, referenceID, traceID string) *ReservationCreatedEvent {
	return &ReservationCreatedEvent{
		BaseEvent: BaseEvent{
			ID:          uuid.New(),
			Type:        "reservation_created",
			AggregateID: fmt.Sprintf("reservation:%s", reservationID.String()),
			Version:     1,
			Timestamp:   time.Now(),
			Metadata:    make(map[string]interface{}),
		},
		ReservationID: reservationID,
		UserID:        userID,
		Currency:      currency,
		Amount:        amount,
		Type:          resType,
		ReferenceID:   referenceID,
		TraceID:       traceID,
		Timestamp:     time.Now(),
	}
}

// NewAccountCreatedEvent creates a new account created event
func NewAccountCreatedEvent(accountID, userID uuid.UUID, currency, accountType, traceID string) *AccountCreatedEvent {
	return &AccountCreatedEvent{
		BaseEvent: BaseEvent{
			ID:          uuid.New(),
			Type:        "account_created",
			AggregateID: fmt.Sprintf("account:%s:%s", userID.String(), currency),
			Version:     1,
			Timestamp:   time.Now(),
			Metadata:    make(map[string]interface{}),
		},
		AccountID:   accountID,
		UserID:      userID,
		Currency:    currency,
		AccountType: accountType,
		TraceID:     traceID,
		Timestamp:   time.Now(),
	}
}
