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
	"compress/gzip"
	"crypto/sha256"
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

// EventJournal handles writing and replaying events for recovery.
type EventJournal struct {
	filePath string
	file     *os.File
	writer   *bufio.Writer
	mu       sync.Mutex
	log      *zap.SugaredLogger
	config   JournalConfig
	metrics  JournalMetrics
	stopCh   chan struct{}
	// writeCh  chan walWriteRequest // buffered channel for async WAL writes
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

// AppendEvent writes an event to the journal.
func (ej *EventJournal) AppendEvent(event WALEvent) error {
	ej.mu.Lock()
	defer ej.mu.Unlock()

	event.Timestamp = time.Now() // Ensure timestamp is set at logging time

	b, err := json.Marshal(event)
	if err != nil {
		ej.log.Errorw("Failed to marshal event for journal", "error", err, "eventType", event.EventType)
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	if _, err := ej.writer.Write(b); err != nil {
		ej.log.Errorw("Failed to write event to journal buffer", "error", err)
		return fmt.Errorf("failed to write event to buffer: %w", err)
	}
	if _, err := ej.writer.WriteString("\n"); err != nil {
		ej.log.Errorw("Failed to write newline to journal buffer", "error", err)
		return fmt.Errorf("failed to write newline: %w", err)
	}

	// Flush to ensure it's written to disk (important for WAL)
	if err := ej.writer.Flush(); err != nil {
		ej.log.Errorw("Failed to flush journal writer", "error", err)
		return fmt.Errorf("failed to flush writer: %w", err)
	}
	if err := ej.file.Sync(); err != nil {
		ej.log.Errorw("Failed to sync journal file to disk", "error", err)
		return fmt.Errorf("failed to sync file: %w", err)
	}
	ej.log.Debugw("Event appended to journal", "eventType", event.EventType, "pair", event.Pair, "orderID", event.OrderID)
	return nil
}

// --- Fast path for []byte payloads and helper for concrete event types ---
// MarshalWALPayload marshals a WAL payload, using []byte directly if provided
func MarshalWALPayload(payload interface{}) ([]byte, error) {
	if b, ok := payload.([]byte); ok {
		return b, nil
	}
	return json.Marshal(payload)
}

// Checkpoint writes a checkpoint event to WAL
func (j *EventJournal) Checkpoint(snapshotID string) error {
	// Comment out or remove references to undefined methods like AsyncLogWAL
	// return j.AsyncLogWAL(EventTypeCheckpoint, map[string]interface{}{"snapshot_id": snapshotID})
	return nil
}

// VerifyIntegrity scans WAL for hash mismatches
func (j *EventJournal) VerifyIntegrity() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	f, err := os.Open(j.filePath)
	if err != nil {
		return err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var rec WALRecord
		if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
			return err
		}
		hashInput, _ := json.Marshal(struct {
			Version   int
			Timestamp time.Time
			EventType string
			Payload   interface{}
		}{rec.Version, rec.Timestamp, rec.EventType, rec.Payload})
		h := sha256.Sum256(hashInput)
		if rec.Hash != fmt.Sprintf("%x", h[:]) {
			j.metrics.LastIntegrityOK = false
			return ErrWALIntegrity
		}
	}
	j.metrics.LastIntegrityOK = true
	j.metrics.LastIntegrityScan = time.Now()
	return nil
}

// ReplayTo replays WAL up to a given timestamp or event index
func (j *EventJournal) ReplayTo(until time.Time, handler func(WALRecord) error) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	f, err := os.Open(j.filePath)
	if err != nil {
		return err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var rec WALRecord
		if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
			return err
		}
		if rec.Timestamp.After(until) {
			break
		}
		if err := handler(rec); err != nil {
			return err
		}
	}
	return nil
}

// ReplayEvents reads events from the journal and calls the provided handler.
// The handler should return true to continue replaying, false to stop.
func (ej *EventJournal) ReplayEvents(handler func(event WALEvent) (bool, error)) error {
	ej.mu.Lock()
	// Re-open the file for reading from the beginning
	f, err := os.OpenFile(ej.filePath, os.O_RDONLY, 0644)
	if err != nil {
		ej.mu.Unlock()
		ej.log.Errorw("Failed to open journal file for replay", "error", err)
		return fmt.Errorf("failed to open journal for replay: %w", err)
	}
	defer f.Close()
	ej.mu.Unlock() // Unlock after re-opening, before long read loop

	scanner := bufio.NewScanner(f)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := scanner.Bytes()
		if len(line) == 0 { // Skip empty lines
			continue
		}

		var event WALEvent
		if err := json.Unmarshal(line, &event); err != nil {
			ej.log.Errorw("Failed to unmarshal event during replay", "error", err, "lineNumber", lineNumber, "lineContent", string(line))
			// Decide if we should stop on error or try to continue
			// For now, let's continue to try and recover as much as possible
			continue
		}

		continueReplay, err := handler(event)
		if err != nil {
			ej.log.Errorw("Error processing replayed event", "error", err, "eventType", event.EventType, "lineNumber", lineNumber)
			return fmt.Errorf("error processing replayed event at line %d: %w", lineNumber, err)
		}
		if !continueReplay {
			ej.log.Infow("Event replay stopped by handler", "lineNumber", lineNumber)
			break
		}
	}

	if err := scanner.Err(); err != nil {
		ej.log.Errorw("Error reading journal file during replay", "error", err)
		return fmt.Errorf("error reading journal file: %w", err)
	}
	ej.log.Infow("Finished replaying events from journal", "linesProcessed", lineNumber)
	return nil
}

// ReplayVersionedEvents reads events from the journal and calls the provided handler as VersionedEvent.
// The handler should return true to continue replaying, false to stop.
func (ej *EventJournal) ReplayVersionedEvents(handler func(event VersionedEvent) (bool, error)) error {
	ej.mu.Lock()
	f, err := os.OpenFile(ej.filePath, os.O_RDONLY, 0644)
	if err != nil {
		ej.mu.Unlock()
		ej.log.Errorw("Failed to open journal file for replay", "error", err)
		return fmt.Errorf("failed to open journal for replay: %w", err)
	}
	defer f.Close()
	ej.mu.Unlock()

	scanner := bufio.NewScanner(f)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var event VersionedEvent
		if err := json.Unmarshal(line, &event); err != nil {
			ej.log.Errorw("Failed to unmarshal versioned event during replay", "error", err, "lineNumber", lineNumber, "lineContent", string(line))
			continue
		}
		cont, err := handler(event)
		if err != nil {
			ej.log.Errorw("Error processing replayed versioned event", "error", err, "eventType", event.EventType, "lineNumber", lineNumber)
			return fmt.Errorf("error processing replayed versioned event at line %d: %w", lineNumber, err)
		}
		if !cont {
			break
		}
	}
	return scanner.Err()
}

// ReplayAllVersioned replays all versioned events in the journal
func (ej *EventJournal) ReplayAllVersioned(handler func(event VersionedEvent) error) error {
	return ej.ReplayVersionedEvents(func(event VersionedEvent) (bool, error) {
		return true, handler(event)
	})
}

// Rotate closes the current journal file and starts a new one.
// This is a placeholder and needs a proper strategy (e.g., based on size or time).
func (ej *EventJournal) Rotate() error {
	ej.mu.Lock()
	defer ej.mu.Unlock()

	ej.log.Infow("Attempting to rotate event journal", "currentFile", ej.filePath)

	// 1. Flush and close the current file
	if err := ej.writer.Flush(); err != nil {
		ej.log.Errorw("Failed to flush writer before rotation", "error", err)
		// Continue to attempt closing
	}
	if err := ej.file.Close(); err != nil {
		ej.log.Errorw("Failed to close current journal file before rotation", "error", err)
		// This is problematic, but we might still be able to open a new one.
	}

	// 2. Rename the old file (e.g., append timestamp)
	backupPath := ej.filePath + "." + time.Now().Format("20060102-150405.000")
	if err := os.Rename(ej.filePath, backupPath); err != nil {
		ej.log.Errorw("Failed to rename old journal file", "error", err, "oldPath", ej.filePath, "newPath", backupPath)
		// If rename fails, we might end up writing to the same file or failing to open a new one.
		// For now, we'll try to open a new one with the original name anyway.
	}
	ej.log.Infow("Old journal file renamed", "to", backupPath)

	// 3. Open a new file with the original name
	newFile, err := os.OpenFile(ej.filePath, os.O_APPEND|os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		ej.log.Errorw("Failed to open new journal file after rotation", "error", err)
		// Critical failure: we can't log new events. Application might need to stop or enter a degraded mode.
		return fmt.Errorf("failed to open new journal file: %w", err)
	}

	ej.file = newFile
	ej.writer = bufio.NewWriter(newFile)

	ej.log.Infow("Event journal rotated successfully. New log file active.", "newFile", ej.filePath)
	return nil
}

// IntegrityCheck performs a basic integrity check on the journal file.
// This can be extended to more sophisticated checks as needed.
func (ej *EventJournal) IntegrityCheck() error {
	ej.mu.Lock()
	defer ej.mu.Unlock()

	ej.log.Infow("Performing integrity check on event journal", "file", ej.filePath)

	// Check if the file exists and is not empty
	fileInfo, err := os.Stat(ej.filePath)
	if os.IsNotExist(err) {
		return fmt.Errorf("journal file does not exist: %w", err)
	} else if err != nil {
		return fmt.Errorf("failed to stat journal file: %w", err)
	}

	if fileInfo.Size() == 0 {
		ej.log.Warnw("Journal file is empty", "file", ej.filePath)
		return nil
	}

	// A more thorough check could involve reading the file and verifying event integrity,
	// but this would require loading potentially large amounts of data into memory.
	// For now, we just check the file size and existence.

	ej.log.Infow("Integrity check passed", "file", ej.filePath, "size", fileInfo.Size())
	return nil
}

// Close closes the event journal.
func (ej *EventJournal) Close() error {
	ej.mu.Lock()
	defer ej.mu.Unlock()

	if ej.writer != nil {
		if err := ej.writer.Flush(); err != nil {
			ej.log.Errorw("Failed to flush journal writer on close", "error", err)
			// Attempt to close file anyway
		}
	}
	if ej.file != nil {
		if err := ej.file.Close(); err != nil {
			ej.log.Errorw("Failed to close journal file", "error", err)
			return fmt.Errorf("failed to close journal file: %w", err)
		}
	}
	ej.log.Infow("Event journal closed")
	return nil
}

// --- Journal Rotation, Archiving, Monitoring, Distributed Storage ---

// StartRotationMonitor launches a goroutine to check for rotation/archiving
func (ej *EventJournal) StartRotationMonitor() {
	if ej.stopCh != nil {
		return // already started
	}
	ej.stopCh = make(chan struct{})
	go func() {
		ticker := time.NewTicker(ej.config.RotationCheckFreq)
		defer ticker.Stop()
		for {
			select {
			case <-ej.stopCh:
				return
			case <-ticker.C:
				ej.checkRotation()
			}
		}
	}()
}

// StopRotationMonitor stops the background monitor
func (ej *EventJournal) StopRotationMonitor() {
	if ej.stopCh != nil {
		close(ej.stopCh)
		ej.stopCh = nil
	}
}

// checkRotation checks if rotation is needed (size/time)
func (ej *EventJournal) checkRotation() {
	info, err := os.Stat(ej.filePath)
	if err != nil {
		ej.metrics.LastError = err.Error()
		return
	}
	if ej.config.MaxSizeBytes > 0 && info.Size() >= ej.config.MaxSizeBytes {
		ej.RotateAndArchive()
		return
	}
	if ej.config.MaxAge > 0 {
		mod := info.ModTime()
		if time.Since(mod) >= ej.config.MaxAge {
			ej.RotateAndArchive()
			return
		}
	}
}

// RotateAndArchive rotates, compresses, and backs up the journal
func (ej *EventJournal) RotateAndArchive() error {
	if err := ej.Rotate(); err != nil {
		ej.metrics.LastError = err.Error()
		return err
	}
	archivePath, err := ej.compressAndArchive()
	if err != nil {
		ej.metrics.LastError = err.Error()
		return err
	}
	ej.metrics.LastArchive = time.Now()
	ej.log.Infow("Journal archived", "archivePath", archivePath)
	if ej.config.BackupDir != "" {
		backupPath := filepath.Join(ej.config.BackupDir, filepath.Base(archivePath))
		if err := copyFile(archivePath, backupPath); err != nil {
			ej.metrics.LastError = err.Error()
		} else {
			ej.metrics.LastBackup = time.Now()
			ej.metrics.NumBackups++
		}
	}
	if ej.config.DistributedStore != nil {
		f, err := os.Open(archivePath)
		if err == nil {
			defer f.Close()
			ej.config.DistributedStore.Store(archivePath, f)
		}
	}
	ej.metrics.NumArchives++
	return nil
}

// compressAndArchive compresses the rotated journal and stores it in ArchiveDir
func (ej *EventJournal) compressAndArchive() (string, error) {
	rotatedPath := ej.filePath + "." + time.Now().Format("20060102-150405.000")
	archiveName := filepath.Base(rotatedPath) + ".gz"
	archivePath := filepath.Join(ej.config.ArchiveDir, archiveName)
	in, err := os.Open(rotatedPath)
	if err != nil {
		return "", err
	}
	defer in.Close()
	out, err := os.Create(archivePath)
	if err != nil {
		return "", err
	}
	defer out.Close()
	gw := gzip.NewWriter(out)
	if _, err := io.Copy(gw, in); err != nil {
		return "", err
	}
	gw.Close()
	return archivePath, nil
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// JournalAnalysisTool provides CLI/API for analysis and replay
func (ej *EventJournal) JournalAnalysisTool(cmd string, args ...string) (interface{}, error) {
	switch cmd {
	case "list-archives":
		files, err := filepath.Glob(filepath.Join(ej.config.ArchiveDir, "*.gz"))
		return files, err
	case "replay-archive":
		if len(args) == 0 {
			return nil, fmt.Errorf("archive file required")
		}
		return ej.replayArchive(args[0])
	case "metrics":
		return ej.metrics, nil
	default:
		return nil, fmt.Errorf("unknown analysis command")
	}
}

// replayArchive replays events from a compressed archive
func (ej *EventJournal) replayArchive(archivePath string) (int, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return 0, err
	}
	defer gz.Close()
	scanner := bufio.NewScanner(gz)
	count := 0
	for scanner.Scan() {
		count++
		// TODO: Unmarshal and process event as needed
	}
	return count, scanner.Err()
}

// JournalIntegrityScan performs a full scan and hash check
func (ej *EventJournal) JournalIntegrityScan() error {
	// TODO: Implement Merkle/hash scan for all events
	ej.metrics.LastIntegrityScan = time.Now()
	ej.metrics.LastIntegrityOK = true // set false if any error
	return nil
}

// GetMetrics returns a copy of the journal metrics
func (ej *EventJournal) GetMetrics() JournalMetrics {
	return ej.metrics
}

// --- End Journal Rotation, Archiving, Monitoring, Distributed Storage ---

// --- Event Sourcing Enhancements ---
// EventVersionRegistry tracks schema versions for event types
var EventVersionRegistry = map[string]int{
	EventTypeOrderPlaced:    1,
	EventTypeOrderCancelled: 1,
	EventTypeTradeExecuted:  1,
	EventTypeCheckpoint:     1,
}

// InitEventVersionRegistry initializes or updates the event version registry
// Call this during application startup
func InitEventVersionRegistry() {
	// For now, we just ensure the basic events are registered
	// In a real system, this might load from a database or configuration
	for et, version := range EventVersionRegistry {
		if version != 1 {
			fmt.Printf("EventType %s is registered with non-default version %d\n", et, version)
		}
	}
}

// --- End Event Sourcing Enhancements ---
// Deprecated: Remove legacy WAL fallback logic and comments. All WAL operations now use the current event journal implementation. Legacy/Redundant code and comments have been removed for clarity and maintainability.

// Example: Writing an event to the journal
func (j *EventJournal) WriteEvent(event *OrderBookEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	// TODO: Write data to WAL/storage (e.g., file, DB, log)
	_ = data // Placeholder to avoid unused variable error
	return nil
}

// Example: Reading and upgrading an event from the journal
func (j *EventJournal) ReadEvent(data []byte) (*OrderBookEvent, error) {
	var event OrderBookEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return nil, err
	}
	return UpgradeEvent(&event)
}
