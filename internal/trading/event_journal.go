// =============================
// Orbit CEX Event Journal
// =============================
// This file implements the event journal (write-ahead log) for recording all important events in the matching engine.
//
// Sections in this file:
// 1. Event types and structures: How events are represented and versioned.
// 2. EventJournal: Main type for writing and replaying events.
// 3. WAL integrity and replay: Ensures events are not lost and can be recovered after a crash.
//
// How it works:
// - All important actions (orders, trades, cancels) are written to a log file.
// - The log can be replayed to recover the engine state after a failure.
//
// Next stages:
// - Used by the matching engine for disaster recovery and auditing.
//
// See comments before each type/function for more details.

package trading

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/trading/model"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// EventType defines the type of event being logged.
const (
	EventTypeOrderPlaced    = "ORDER_PLACED"
	EventTypeOrderCancelled = "ORDER_CANCELLED"
	EventTypeTradeExecuted  = "TRADE_EXECUTED"
	EventTypeCheckpoint     = "CHECKPOINT"
)

// WALEvent represents a generic event to be written to the Write-Ahead Log.
type WALEvent struct {
	Timestamp time.Time   `json:"timestamp"`
	EventType string      `json:"event_type"`
	Pair      string      `json:"pair,omitempty"`
	OrderID   uuid.UUID   `json:"order_id,omitempty"`
	Data      interface{} `json:"data"` // Can be *model.Order, *CancelRequest, *model.Trade etc.
}

// VersionedEvent is the base for all events with versioning and schema evolution
// All domain events should embed this struct
// Version is incremented on schema changes
// EventType is a string identifier for the event
// Timestamp is the event time
// Data is the event payload (can be any struct)
type VersionedEvent struct {
	EventID   string      `json:"event_id"`
	EventType string      `json:"event_type"`
	Version   int         `json:"version"`
	Timestamp time.Time   `json:"timestamp"`
	Data      interface{} `json:"data"`
}

// OrderBookEvent represents a versioned event for event sourcing and WAL.
type OrderBookEvent struct {
	Version   int             `json:"version"`
	Type      string          `json:"type"` // e.g., "order", "cancel", etc.
	Timestamp int64           `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"` // Typed payload, versioned
}

// EventUpgrader defines a function that upgrades an event from one version to the next.
type EventUpgrader func(event *OrderBookEvent) (*OrderBookEvent, error)

// EventUpgraderRegistry holds upgraders for event schema evolution.
var EventUpgraderRegistry = map[int]EventUpgrader{}

// RegisterEventUpgrader registers an upgrader for a specific version.
func RegisterEventUpgrader(version int, upgrader EventUpgrader) {
	EventUpgraderRegistry[version] = upgrader
}

// UpgradeEvent upgrades an event to the latest version using the registry.
func UpgradeEvent(event *OrderBookEvent) (*OrderBookEvent, error) {
	for {
		upgrader, ok := EventUpgraderRegistry[event.Version]
		if !ok {
			break
		}
		var err error
		event, err = upgrader(event)
		if err != nil {
			return nil, err
		}
	}
	return event, nil
}

// EventStore defines the interface for event sourcing
// Append, ReplayAllVersioned, and IntegrityCheck are required for CQRS/event sourcing
// Implemented by EventJournal
type EventStore interface {
	AppendEvent(event VersionedEvent) error
	ReplayAllVersioned(handler func(VersionedEvent) error) error
}

// Event validation and schema evolution
// Each event type should implement this interface
type ValidatableEvent interface {
	Validate() error
	SchemaVersion() int
}

// Example: OrderPlacedEvent (versioned)
type OrderPlacedEvent struct {
	Order    *model.Order           `json:"order"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

func (e OrderPlacedEvent) Validate() error {
	if e.Order == nil || e.Order.ID == uuid.Nil {
		return fmt.Errorf("invalid order in OrderPlacedEvent")
	}
	return nil
}
func (e OrderPlacedEvent) SchemaVersion() int { return 1 }

// Example: OrderCancelledEvent (versioned)
type OrderCancelledEvent struct {
	OrderID uuid.UUID `json:"order_id"`
	Pair    string    `json:"pair"`
	UserID  uuid.UUID `json:"user_id"`
	Reason  string    `json:"reason,omitempty"`
}

func (e OrderCancelledEvent) Validate() error {
	if e.OrderID == uuid.Nil || e.Pair == "" {
		return fmt.Errorf("invalid order cancel event")
	}
	return nil
}
func (e OrderCancelledEvent) SchemaVersion() int { return 1 }

// JournalConfig holds rotation and archiving settings
// Can be extended for distributed storage, backup, etc.
type JournalConfig struct {
	MaxSizeBytes      int64                   // Rotate when file exceeds this size
	MaxAge            time.Duration           // Rotate after this duration
	ArchiveDir        string                  // Where to store compressed archives
	BackupDir         string                  // Where to store backups
	DistributedStore  DistributedJournalStore // Optional distributed storage
	RotationCheckFreq time.Duration           // How often to check for rotation
	FlushInterval     time.Duration           // WAL batch flush interval (default: 2ms, HFT: <1ms)
}

// DistributedJournalStore defines an interface for remote/distributed storage
// Implementations: S3, NFS, custom, etc.
type DistributedJournalStore interface {
	Store(filePath string, r io.Reader) error
	Fetch(archiveName string, w io.Writer) error
	ListArchives() ([]string, error)
}

// JournalMetrics tracks health and performance
// Expose via admin/monitoring endpoints
type JournalMetrics struct {
	LastRotation      time.Time
	LastArchive       time.Time
	LastBackup        time.Time
	LastError         string
	FlushLatency      time.Duration
	FlushLatencies    []time.Duration // For p99 calculation
	P99FsyncLatency   time.Duration   // p99 fsync latency
	QueueSize         int
	NumRotations      int
	NumArchives       int
	NumBackups        int
	NumErrors         int
	LastIntegrityScan time.Time
	LastIntegrityOK   bool
}

// KafkaClient is an interface for publishing/consuming events to/from Kafka.
type KafkaClient interface {
	Publish(topic string, key string, value []byte) error
	Subscribe(topic string, handler func(msg []byte) error) error
}

// CoordinatorClient is a stub interface for distributed coordination (e.g., etcd/ZooKeeper).
type CoordinatorClient interface {
	// For future: leader election, health checks, failover, etc.
	IsLeader() bool
	InstanceID() string
}

// EventJournal handles writing and replaying events for recovery.
type EventJournal struct {
	filePath string
	file     *os.File
	writer   *bufio.Writer
	mu       sync.Mutex
	log      *zap.SugaredLogger
	// --- Distributed fields ---
	InstanceID  string            // Optional: unique ID for this engine instance (for tracing, sharding)
	Kafka       KafkaClient       // Optional: Kafka client for distributed event publishing/consumption
	KafkaTopic  string            // Optional: Kafka topic for event journal
	AsyncAppend bool              // If true, use async append mode (high-throughput)
	appendCh    chan WALEvent     // Buffered channel for async appends
	Coordinator CoordinatorClient // Optional: distributed coordinator (etcd/ZooKeeper)
}

// ErrWALIntegrity is returned on hash/checksum mismatch
var ErrWALIntegrity = fmt.Errorf("WAL integrity check failed")

// WALRecord is a WAL entry with integrity hash
// All state-changing ops must use this
// Hash covers all fields except Hash itself
type WALRecord struct {
	Version   int         `json:"version"`
	Timestamp time.Time   `json:"timestamp"`
	EventType string      `json:"event_type"`
	Payload   interface{} `json:"payload"`
	Hash      string      `json:"hash"`
}

// NewEventJournal creates or opens an event journal file.
func NewEventJournal(log *zap.SugaredLogger, journalPath string) (*EventJournal, error) {
	// Ensure the directory exists
	err := os.MkdirAll(filepath.Dir(journalPath), 0755)
	if err != nil {
		return nil, fmt.Errorf("failed to create journal directory: %w", err)
	}

	f, err := os.OpenFile(journalPath, os.O_APPEND|os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open journal file: %w", err)
	}

	return &EventJournal{
		filePath: journalPath,
		file:     f,
		writer:   bufio.NewWriter(f),
		log:      log,
	}, nil
}

// NewDistributedEventJournal creates an EventJournal with distributed features enabled.
func NewDistributedEventJournal(log *zap.SugaredLogger, journalPath string, instanceID string, kafka KafkaClient, kafkaTopic string, asyncAppend bool, coordinator CoordinatorClient) (*EventJournal, error) {
	ej, err := NewEventJournal(log, journalPath)
	if err != nil {
		return nil, err
	}
	ej.InstanceID = instanceID
	ej.Kafka = kafka
	ej.KafkaTopic = kafkaTopic
	ej.AsyncAppend = asyncAppend
	ej.Coordinator = coordinator
	if asyncAppend {
		ej.appendCh = make(chan WALEvent, 4096) // Buffer size can be tuned
		go ej.asyncAppendWorker()
	}
	return ej, nil
}

// Optional: InstanceID for distributed tracing
var InstanceID string

// --- Enhanced AppendEvent ---
func (ej *EventJournal) AppendEvent(event WALEvent) error {
	if ej.InstanceID != "" {
		// Attach instance ID for distributed tracing
		if event.Data == nil {
			event.Data = map[string]interface{}{"instance_id": ej.InstanceID}
		} else if m, ok := event.Data.(map[string]interface{}); ok {
			m["instance_id"] = ej.InstanceID
		}
	}
	if ej.AsyncAppend && ej.appendCh != nil {
		select {
		case ej.appendCh <- event:
			return nil // Buffered, will be written by worker
		default:
			ej.log.Warnw("Async WAL append channel full, falling back to sync", "eventType", event.EventType)
			// Fallback to sync if channel is full
		}
	}
	return ej.appendEventInternal(event)
}

// appendEventInternal writes the event to local WAL and optionally publishes to Kafka.
func (ej *EventJournal) appendEventInternal(event WALEvent) error {
	ej.mu.Lock()
	defer ej.mu.Unlock()
	event.Timestamp = time.Now()
	b, err := json.Marshal(event)
	if err != nil {
		ej.log.Errorw("Failed to marshal event for journal", "error", err, "eventType", event.EventType)
		return err
	}
	if _, err := ej.writer.Write(b); err != nil {
		ej.log.Errorw("Failed to write event to journal buffer", "error", err)
		return err
	}
	if _, err := ej.writer.WriteString("\n"); err != nil {
		ej.log.Errorw("Failed to write newline to journal buffer", "error", err)
		return err
	}
	if err := ej.writer.Flush(); err != nil {
		ej.log.Errorw("Failed to flush journal writer", "error", err)
		return err
	}
	if err := ej.file.Sync(); err != nil {
		ej.log.Errorw("Failed to sync journal file to disk", "error", err)
		return err
	}
	if ej.Kafka != nil && ej.KafkaTopic != "" {
		if err := ej.Kafka.Publish(ej.KafkaTopic, event.EventType, b); err != nil {
			ej.log.Errorw("Failed to publish event to Kafka", "error", err, "topic", ej.KafkaTopic)
			// Non-fatal: continue
		}
	}
	ej.log.Debugw("Event appended to journal", "eventType", event.EventType, "pair", event.Pair, "orderID", event.OrderID)
	return nil
}

// asyncAppendWorker handles async WAL appends (if enabled)
func (ej *EventJournal) asyncAppendWorker() {
	for event := range ej.appendCh {
		_ = ej.appendEventInternal(event) // Errors are logged inside
	}
}

// StartKafkaConsumer starts a background Kafka consumer for distributed event sync (if enabled).
func (ej *EventJournal) StartKafkaConsumer(handler func(WALEvent) error) error {
	if ej.Kafka == nil || ej.KafkaTopic == "" {
		return nil // Not enabled
	}
	return ej.Kafka.Subscribe(ej.KafkaTopic, func(msg []byte) error {
		var event WALEvent
		if err := json.Unmarshal(msg, &event); err != nil {
			return err
		}
		return handler(event)
	})
}

// --- Distributed Event Journal Usage ---
// To enable distributed journaling:
//   - Use NewDistributedEventJournal() with KafkaClient, instanceID, and CoordinatorClient as needed.
//   - Set AsyncAppend=true for high-throughput, low-latency journaling (uses a buffered channel).
//   - Kafka integration is opt-in: if KafkaClient is nil, events are only written locally.
//   - CoordinatorClient is a stub for future distributed consensus/leader election.
//   - All distributed features are backward compatible and do not affect local-only operation.
//
// HealthCheck returns the distributed journal health for monitoring/HA.
func (ej *EventJournal) HealthCheck() map[string]interface{} {
	health := map[string]interface{}{
		"instance_id":      ej.InstanceID,
		"kafka_enabled":    ej.Kafka != nil && ej.KafkaTopic != "",
		"async_append":     ej.AsyncAppend,
		"coordinator":      ej.Coordinator != nil,
		"append_queue_len": len(ej.appendCh),
	}
	if ej.Coordinator != nil {
		health["is_leader"] = ej.Coordinator.IsLeader()
		health["coordinator_id"] = ej.Coordinator.InstanceID()
	}
	return health
}

// --- End Distributed Event Journal Usage ---
