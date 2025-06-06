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
	_ = snapshot // suppress unused variable warning

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

// Saga pattern interfaces and implementations for complex transaction workflows

// Saga represents a long-running transaction with compensation logic
type Saga interface {
	GetID() uuid.UUID
	HandleEvent(event Event) error
	GetState() SagaState
	GetCompensations() []CompensationStep
}

// SagaState represents the current state of a saga
type SagaState string

const (
	SagaStateInitialized SagaState = "initialized"
	SagaStateInProgress  SagaState = "in_progress"
	SagaStateCompleted   SagaState = "completed"
	SagaStateFailed      SagaState = "failed"
	SagaStateCompensated SagaState = "compensated"
)

// SagaStep represents a step in a saga transaction
type SagaStep interface {
	Execute() error
	GetCompensation() CompensationStep
}

// CompensationStep represents a compensation action for a failed saga
type CompensationStep interface {
	Execute() error
}

// SagaManager orchestrates multi-step transactions using saga pattern
type SagaManager struct {
	sagas      sync.Map // sagaID -> Saga
	eventStore *EventStore
	logger     *zap.Logger
	metrics    *SagaMetrics
}

// SagaMetrics tracks saga performance and outcomes
type SagaMetrics struct {
	SagasStarted     prometheus.Counter
	SagasCompleted   prometheus.Counter
	SagasFailed      prometheus.Counter
	SagasCompensated prometheus.Counter
	SagaDuration     prometheus.Histogram
}

// TransferSaga implements account-to-account transfer using saga pattern
type TransferSaga struct {
	BaseEvent
	sagaID        uuid.UUID
	fromUserID    uuid.UUID
	toUserID      uuid.UUID
	amount        decimal.Decimal
	currency      string
	state         SagaState
	compensations []CompensationStep
	steps         []SagaStep
	currentStep   int
	logger        *zap.Logger
	createdAt     time.Time
}

// Saga-related events
type SagaStartedEvent struct {
	BaseEvent
	SagaID     uuid.UUID              `json:"saga_id"`
	SagaType   string                 `json:"saga_type"`
	Parameters map[string]interface{} `json:"parameters"`
}

type SagaCompletedEvent struct {
	BaseEvent
	SagaID uuid.UUID   `json:"saga_id"`
	Result interface{} `json:"result"`
}

type SagaFailedEvent struct {
	BaseEvent
	SagaID uuid.UUID `json:"saga_id"`
	Error  string    `json:"error"`
}

type SagaCompensatedEvent struct {
	BaseEvent
	SagaID uuid.UUID `json:"saga_id"`
}

// Transaction-specific events
type TransferInitiatedEvent struct {
	BaseEvent
	TransferID uuid.UUID       `json:"transfer_id"`
	FromUserID uuid.UUID       `json:"from_user_id"`
	ToUserID   uuid.UUID       `json:"to_user_id"`
	Amount     decimal.Decimal `json:"amount"`
	Currency   string          `json:"currency"`
}

type FundsReservedEvent struct {
	BaseEvent
	ReservationID uuid.UUID       `json:"reservation_id"`
	UserID        uuid.UUID       `json:"user_id"`
	Amount        decimal.Decimal `json:"amount"`
	Currency      string          `json:"currency"`
}

type FundsReleasedEvent struct {
	BaseEvent
	ReservationID uuid.UUID       `json:"reservation_id"`
	UserID        uuid.UUID       `json:"user_id"`
	Amount        decimal.Decimal `json:"amount"`
	Currency      string          `json:"currency"`
}

// NewSagaManager creates a new saga manager
func NewSagaManager(eventStore *EventStore, logger *zap.Logger) *SagaManager {
	metrics := &SagaMetrics{
		SagasStarted: promauto.NewCounter(prometheus.CounterOpts{
			Name: "saga_transactions_started_total",
			Help: "Total number of saga transactions started",
		}),
		SagasCompleted: promauto.NewCounter(prometheus.CounterOpts{
			Name: "saga_transactions_completed_total",
			Help: "Total number of saga transactions completed",
		}),
		SagasFailed: promauto.NewCounter(prometheus.CounterOpts{
			Name: "saga_transactions_failed_total",
			Help: "Total number of saga transactions failed",
		}),
		SagasCompensated: promauto.NewCounter(prometheus.CounterOpts{
			Name: "saga_transactions_compensated_total",
			Help: "Total number of saga transactions compensated",
		}),
		SagaDuration: promauto.NewHistogram(prometheus.HistogramOpts{
			Name:    "saga_transaction_duration_seconds",
			Help:    "Duration of saga transactions in seconds",
			Buckets: prometheus.DefBuckets,
		}),
	}

	return &SagaManager{
		eventStore: eventStore,
		logger:     logger,
		metrics:    metrics,
	}
}

// StartSaga initiates a new saga transaction
func (sm *SagaManager) StartSaga(ctx context.Context, saga Saga) error {
	sm.metrics.SagasStarted.Inc()
	sm.sagas.Store(saga.GetID(), saga)

	// Create saga started event
	event := &SagaStartedEvent{
		BaseEvent: BaseEvent{
			ID:          uuid.New(),
			Type:        "saga_started",
			AggregateID: saga.GetID().String(),
			Version:     1,
			Timestamp:   time.Now(),
			Metadata:    map[string]interface{}{"saga_id": saga.GetID()},
		},
		SagaID:   saga.GetID(),
		SagaType: fmt.Sprintf("%T", saga),
	}

	return sm.eventStore.PublishEvent(ctx, event)
}

// HandleEvent processes events for saga orchestration
func (sm *SagaManager) HandleEvent(ctx context.Context, event Event) {
	// Check if event has saga ID in metadata
	if sagaIDStr, ok := event.GetMetadata()["saga_id"]; ok {
		if sagaID, err := uuid.Parse(fmt.Sprintf("%v", sagaIDStr)); err == nil {
			if saga, ok := sm.sagas.Load(sagaID); ok {
				if err := saga.(Saga).HandleEvent(event); err != nil {
					sm.logger.Error("Saga event handling failed",
						zap.String("saga_id", sagaID.String()),
						zap.Error(err))
					sm.handleSagaFailure(ctx, saga.(Saga), err)
					return
				}

				// Check saga state and handle completion/failure
				state := saga.(Saga).GetState()
				switch state {
				case SagaStateCompleted:
					sm.handleSagaCompletion(ctx, saga.(Saga))
				case SagaStateFailed:
					sm.handleSagaFailure(ctx, saga.(Saga), fmt.Errorf("saga failed"))
				}
			}
		}
	}
}

// handleSagaCompletion handles successful saga completion
func (sm *SagaManager) handleSagaCompletion(ctx context.Context, saga Saga) {
	sm.metrics.SagasCompleted.Inc()
	sm.sagas.Delete(saga.GetID())

	// Publish saga completed event
	event := &SagaCompletedEvent{
		BaseEvent: BaseEvent{
			ID:          uuid.New(),
			Type:        "saga_completed",
			AggregateID: saga.GetID().String(),
			Version:     1,
			Timestamp:   time.Now(),
			Metadata:    map[string]interface{}{"saga_id": saga.GetID()},
		},
		SagaID: saga.GetID(),
	}

	sm.eventStore.PublishEvent(ctx, event)
}

// handleSagaFailure handles saga failure and initiates compensation
func (sm *SagaManager) handleSagaFailure(ctx context.Context, saga Saga, err error) {
	sm.metrics.SagasFailed.Inc()

	// Execute compensations in reverse order
	compensations := saga.GetCompensations()
	for i := len(compensations) - 1; i >= 0; i-- {
		if compErr := compensations[i].Execute(); compErr != nil {
			sm.logger.Error("Compensation step failed",
				zap.String("saga_id", saga.GetID().String()),
				zap.Int("step", i),
				zap.Error(compErr))
		}
	}

	sm.metrics.SagasCompensated.Inc()
	sm.sagas.Delete(saga.GetID())

	// Publish saga compensated event
	event := &SagaCompensatedEvent{
		BaseEvent: BaseEvent{
			ID:          uuid.New(),
			Type:        "saga_compensated",
			AggregateID: saga.GetID().String(),
			Version:     1,
			Timestamp:   time.Now(),
			Metadata:    map[string]interface{}{"saga_id": saga.GetID()},
		},
		SagaID: saga.GetID(),
	}

	sm.eventStore.PublishEvent(ctx, event)
}

// NewTransferSaga creates a new transfer saga
func NewTransferSaga(fromUserID, toUserID uuid.UUID, amount decimal.Decimal, currency string, logger *zap.Logger) *TransferSaga {
	sagaID := uuid.New()
	return &TransferSaga{
		BaseEvent: BaseEvent{
			ID:          sagaID,
			Type:        "transfer_saga",
			AggregateID: sagaID.String(),
			Version:     1,
			Timestamp:   time.Now(),
			Metadata:    map[string]interface{}{"saga_id": sagaID},
		},
		sagaID:     sagaID,
		fromUserID: fromUserID,
		toUserID:   toUserID,
		amount:     amount,
		currency:   currency,
		state:      SagaStateInitialized,
		logger:     logger,
		createdAt:  time.Now(),
	}
}

// TransferSaga implementation
func (ts *TransferSaga) GetID() uuid.UUID {
	return ts.sagaID
}

func (ts *TransferSaga) HandleEvent(event Event) error {
	switch event.GetType() {
	case "transfer_initiated":
		ts.state = SagaStateInProgress
	case "transfer_completed":
		ts.state = SagaStateCompleted
	case "transfer_failed":
		ts.state = SagaStateFailed
	}
	return nil
}

func (ts *TransferSaga) GetState() SagaState {
	return ts.state
}

func (ts *TransferSaga) GetCompensations() []CompensationStep {
	return ts.compensations
}
