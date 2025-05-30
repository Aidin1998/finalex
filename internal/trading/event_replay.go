package trading

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/trading/model"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// EventReplayManager handles comprehensive event replay functionality
type EventReplayManager struct {
	logger          *zap.SugaredLogger
	versionManager  *EventVersionManager
	checkpointStore CheckpointStore
	replayConfig    ReplayConfig
	mu              sync.RWMutex
	eventHandlers   map[string]EventHandler
	errorHandlers   map[string]ErrorHandler
	stats           ReplayStatistics
}

// ReplayConfig holds configuration for event replay
type ReplayConfig struct {
	MaxReplayEvents       int           `json:"max_replay_events"`
	ReplayTimeout         time.Duration `json:"replay_timeout"`
	BatchSize             int           `json:"batch_size"`
	CheckpointInterval    int           `json:"checkpoint_interval"`
	ContinueOnError       bool          `json:"continue_on_error"`
	SkipCorruptedEvents   bool          `json:"skip_corrupted_events"`
	ValidateEventSchemas  bool          `json:"validate_event_schemas"`
	UseParallelProcessing bool          `json:"use_parallel_processing"`
	WorkerCount           int           `json:"worker_count"`
}

// DefaultReplayConfig returns a sensible default configuration
func DefaultReplayConfig() ReplayConfig {
	return ReplayConfig{
		MaxReplayEvents:       1000000,   // 1M events max
		ReplayTimeout:         time.Hour, // 1 hour timeout
		BatchSize:             1000,      // Process 1000 events at a time
		CheckpointInterval:    10000,     // Checkpoint every 10k events
		ContinueOnError:       true,      // Continue on non-critical errors
		SkipCorruptedEvents:   true,      // Skip corrupted events
		ValidateEventSchemas:  true,      // Validate against schemas
		UseParallelProcessing: false,     // Sequential by default for data consistency
		WorkerCount:           4,         // Number of parallel workers
	}
}

// EventHandler defines a function that processes a specific event type
type EventHandler func(ctx context.Context, event *VersionedEvent) error

// ErrorHandler defines a function that handles errors during replay
type ErrorHandler func(ctx context.Context, event *VersionedEvent, err error) bool // returns true to continue

// CheckpointStore defines interface for managing replay checkpoints
type CheckpointStore interface {
	SaveCheckpoint(ctx context.Context, checkpoint *ReplayCheckpoint) error
	LoadCheckpoint(ctx context.Context, checkpointID string) (*ReplayCheckpoint, error)
	ListCheckpoints(ctx context.Context) ([]*ReplayCheckpoint, error)
	DeleteCheckpoint(ctx context.Context, checkpointID string) error
}

// ReplayCheckpoint represents a replay checkpoint for resumable replay
type ReplayCheckpoint struct {
	ID              string                 `json:"id"`
	EventSequence   int64                  `json:"event_sequence"`
	EventTimestamp  time.Time              `json:"event_timestamp"`
	EventsProcessed int64                  `json:"events_processed"`
	LastEventID     string                 `json:"last_event_id"`
	State           map[string]interface{} `json:"state"`
	CreatedAt       time.Time              `json:"created_at"`
	ReplayContext   string                 `json:"replay_context"`
	EngineState     *EngineStateSnapshot   `json:"engine_state,omitempty"`
}

// EngineStateSnapshot captures engine state for checkpointing
type EngineStateSnapshot struct {
	OrderBookStates map[string]interface{}  `json:"order_book_states"`
	ActiveOrders    map[string]*model.Order `json:"active_orders"`
	LastTradeID     string                  `json:"last_trade_id"`
	LastSequenceNum int64                   `json:"last_sequence_num"`
	TotalVolume     map[string]string       `json:"total_volume"`
	OpenOrdersCount int                     `json:"open_orders_count"`
	ProcessedEvents int64                   `json:"processed_events"`
}

// ReplayStatistics tracks replay performance and progress
type ReplayStatistics struct {
	StartTime           time.Time        `json:"start_time"`
	EndTime             time.Time        `json:"end_time"`
	Duration            time.Duration    `json:"duration"`
	EventsProcessed     int64            `json:"events_processed"`
	EventsSkipped       int64            `json:"events_skipped"`
	EventsErrored       int64            `json:"events_errored"`
	CheckpointsCreated  int64            `json:"checkpoints_created"`
	EventTypeStats      map[string]int64 `json:"event_type_stats"`
	ErrorStats          map[string]int64 `json:"error_stats"`
	ThroughputPerSecond float64          `json:"throughput_per_second"`
	LastProcessedEvent  string           `json:"last_processed_event"`
	CurrentCheckpoint   string           `json:"current_checkpoint"`
}

// NewEventReplayManager creates a new replay manager
func NewEventReplayManager(
	logger *zap.SugaredLogger,
	versionManager *EventVersionManager,
	checkpointStore CheckpointStore,
	config ReplayConfig,
) *EventReplayManager {
	return &EventReplayManager{
		logger:          logger,
		versionManager:  versionManager,
		checkpointStore: checkpointStore,
		replayConfig:    config,
		eventHandlers:   make(map[string]EventHandler),
		errorHandlers:   make(map[string]ErrorHandler),
		stats: ReplayStatistics{
			EventTypeStats: make(map[string]int64),
			ErrorStats:     make(map[string]int64),
		},
	}
}

// RegisterEventHandler registers a handler for a specific event type
func (erm *EventReplayManager) RegisterEventHandler(eventType string, handler EventHandler) {
	erm.mu.Lock()
	defer erm.mu.Unlock()
	erm.eventHandlers[eventType] = handler
	erm.logger.Infow("Registered event handler", "eventType", eventType)
}

// RegisterErrorHandler registers an error handler for a specific event type
func (erm *EventReplayManager) RegisterErrorHandler(eventType string, handler ErrorHandler) {
	erm.mu.Lock()
	defer erm.mu.Unlock()
	erm.errorHandlers[eventType] = handler
	erm.logger.Infow("Registered error handler", "eventType", eventType)
}

// ReplayFromJournal replays events from a journal file
func (erm *EventReplayManager) ReplayFromJournal(ctx context.Context, journalPath string) error {
	erm.logger.Infow("Starting event replay from journal", "path", journalPath)
	erm.stats.StartTime = time.Now()

	file, err := os.Open(journalPath)
	if err != nil {
		return fmt.Errorf("failed to open journal file: %w", err)
	}
	defer file.Close()

	return erm.replayFromReader(ctx, file, "file:"+journalPath)
}

// ReplayFromCheckpoint resumes replay from a specific checkpoint
func (erm *EventReplayManager) ReplayFromCheckpoint(ctx context.Context, checkpointID string, journalPath string) error {
	erm.logger.Infow("Starting event replay from checkpoint", "checkpointID", checkpointID, "path", journalPath)

	checkpoint, err := erm.checkpointStore.LoadCheckpoint(ctx, checkpointID)
	if err != nil {
		return fmt.Errorf("failed to load checkpoint: %w", err)
	}

	erm.stats.CurrentCheckpoint = checkpointID
	erm.logger.Infow("Loaded checkpoint",
		"checkpointID", checkpointID,
		"eventsProcessed", checkpoint.EventsProcessed,
		"lastEventID", checkpoint.LastEventID)

	file, err := os.Open(journalPath)
	if err != nil {
		return fmt.Errorf("failed to open journal file: %w", err)
	}
	defer file.Close()

	// Skip to checkpoint position
	if err := erm.seekToCheckpoint(file, checkpoint); err != nil {
		return fmt.Errorf("failed to seek to checkpoint: %w", err)
	}

	return erm.replayFromReader(ctx, file, "checkpoint:"+checkpointID)
}

// ReplayEventBatch replays a batch of events with proper error handling
func (erm *EventReplayManager) ReplayEventBatch(ctx context.Context, events []*VersionedEvent) error {
	erm.logger.Debugw("Replaying event batch", "batchSize", len(events))

	for i, event := range events {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err := erm.processEvent(ctx, event); err != nil {
			erm.stats.EventsErrored++
			erm.stats.ErrorStats[event.EventType]++

			if !erm.replayConfig.ContinueOnError {
				return fmt.Errorf("failed to process event %d in batch: %w", i, err)
			}

			erm.logger.Errorw("Error processing event in batch",
				"eventIndex", i,
				"eventID", event.EventID,
				"eventType", event.EventType,
				"error", err)
		} else {
			erm.stats.EventsProcessed++
			erm.stats.EventTypeStats[event.EventType]++
		}
	}

	return nil
}

// processEvent processes a single versioned event
func (erm *EventReplayManager) processEvent(ctx context.Context, event *VersionedEvent) error {
	// Validate event schema if enabled
	if erm.replayConfig.ValidateEventSchemas && erm.versionManager != nil {
		if err := erm.versionManager.GetSchemaRegistry().ValidateEvent(event); err != nil {
			if erm.replayConfig.SkipCorruptedEvents {
				erm.logger.Warnw("Skipping corrupted event",
					"eventID", event.EventID,
					"eventType", event.EventType,
					"error", err)
				erm.stats.EventsSkipped++
				return nil
			}
			return fmt.Errorf("event validation failed: %w", err)
		}
	}

	// Get handler for this event type
	erm.mu.RLock()
	handler, exists := erm.eventHandlers[event.EventType]
	errorHandler := erm.errorHandlers[event.EventType]
	erm.mu.RUnlock()

	if !exists {
		erm.logger.Warnw("No handler for event type", "eventType", event.EventType)
		erm.stats.EventsSkipped++
		return nil
	}

	// Process the event
	if err := handler(ctx, event); err != nil {
		// Try event-specific error handler
		if errorHandler != nil {
			if shouldContinue := errorHandler(ctx, event, err); shouldContinue {
				erm.logger.Warnw("Error handler chose to continue",
					"eventID", event.EventID,
					"eventType", event.EventType,
					"error", err)
				return nil
			}
		}
		return err
	}

	erm.stats.LastProcessedEvent = event.EventID
	return nil
}

// replayFromReader implements the core replay logic
func (erm *EventReplayManager) replayFromReader(ctx context.Context, reader io.Reader, source string) error {
	scanner := bufio.NewScanner(reader)
	var batch []*VersionedEvent
	eventCount := 0

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		event, err := erm.parseEvent(line)
		if err != nil {
			if erm.replayConfig.SkipCorruptedEvents {
				erm.logger.Warnw("Skipping corrupted event line", "error", err, "line", line[:min(100, len(line))])
				erm.stats.EventsSkipped++
				continue
			}
			return fmt.Errorf("failed to parse event: %w", err)
		}

		batch = append(batch, event)
		eventCount++

		// Process batch when it reaches configured size
		if len(batch) >= erm.replayConfig.BatchSize {
			if err := erm.ReplayEventBatch(ctx, batch); err != nil {
				return err
			}
			batch = batch[:0] // Reset batch

			// Create checkpoint if needed
			if eventCount%erm.replayConfig.CheckpointInterval == 0 {
				if err := erm.createCheckpoint(ctx, event, eventCount); err != nil {
					erm.logger.Errorw("Failed to create checkpoint", "error", err)
					// Continue even if checkpointing fails
				}
			}
		}

		// Check replay limits
		if eventCount >= erm.replayConfig.MaxReplayEvents {
			erm.logger.Warnw("Reached maximum replay events limit", "limit", erm.replayConfig.MaxReplayEvents)
			break
		}
	}

	// Process remaining events in batch
	if len(batch) > 0 {
		if err := erm.ReplayEventBatch(ctx, batch); err != nil {
			return err
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading journal: %w", err)
	}

	erm.stats.EndTime = time.Now()
	erm.stats.Duration = erm.stats.EndTime.Sub(erm.stats.StartTime)
	erm.calculateThroughput()

	erm.logger.Infow("Event replay completed",
		"source", source,
		"eventsProcessed", erm.stats.EventsProcessed,
		"eventsSkipped", erm.stats.EventsSkipped,
		"eventsErrored", erm.stats.EventsErrored,
		"duration", erm.stats.Duration,
		"throughputPerSecond", erm.stats.ThroughputPerSecond)

	return nil
}

// parseEvent converts a journal line to a VersionedEvent
func (erm *EventReplayManager) parseEvent(line string) (*VersionedEvent, error) {
	// First try to parse as WALEvent (the common format)
	var walEvent WALEvent
	if err := json.Unmarshal([]byte(line), &walEvent); err == nil {
		// Convert WALEvent to VersionedEvent
		return &VersionedEvent{
			EventID:   uuid.New().String(), // Generate new ID if not present
			EventType: walEvent.EventType,
			Version:   1, // Default version
			Timestamp: walEvent.Timestamp,
			Data:      walEvent.Data,
		}, nil
	}

	// Try to parse as VersionedEvent directly
	var versionedEvent VersionedEvent
	if err := json.Unmarshal([]byte(line), &versionedEvent); err == nil {
		return &versionedEvent, nil
	}

	// Try to parse as OrderBookEvent
	var orderBookEvent OrderBookEvent
	if err := json.Unmarshal([]byte(line), &orderBookEvent); err == nil {
		var data interface{}
		if err := json.Unmarshal(orderBookEvent.Payload, &data); err != nil {
			return nil, fmt.Errorf("failed to unmarshal OrderBookEvent payload: %w", err)
		}

		return &VersionedEvent{
			EventID:   uuid.New().String(),
			EventType: orderBookEvent.Type,
			Version:   orderBookEvent.Version,
			Timestamp: time.Unix(orderBookEvent.Timestamp, 0),
			Data:      data,
		}, nil
	}

	return nil, fmt.Errorf("unable to parse event from line")
}

// seekToCheckpoint positions the file reader at the checkpoint location
func (erm *EventReplayManager) seekToCheckpoint(file *os.File, checkpoint *ReplayCheckpoint) error {
	// For now, scan through file to find the checkpoint position
	// In production, you'd want to use more efficient seeking methods
	scanner := bufio.NewScanner(file)
	lineCount := 0

	for scanner.Scan() {
		event, err := erm.parseEvent(scanner.Text())
		if err != nil {
			continue
		}

		// Found the checkpoint event
		if event.EventID == checkpoint.LastEventID ||
			event.Timestamp.Equal(checkpoint.EventTimestamp) {
			erm.logger.Infow("Found checkpoint position",
				"lineCount", lineCount,
				"eventID", event.EventID)
			return nil
		}

		lineCount++
	}

	return fmt.Errorf("checkpoint position not found")
}

// createCheckpoint creates a new replay checkpoint
func (erm *EventReplayManager) createCheckpoint(ctx context.Context, lastEvent *VersionedEvent, eventCount int) error {
	checkpointID := fmt.Sprintf("replay_%d_%s", time.Now().Unix(), uuid.New().String()[:8])

	checkpoint := &ReplayCheckpoint{
		ID:              checkpointID,
		EventSequence:   int64(eventCount),
		EventTimestamp:  lastEvent.Timestamp,
		EventsProcessed: erm.stats.EventsProcessed,
		LastEventID:     lastEvent.EventID,
		CreatedAt:       time.Now(),
		ReplayContext:   "automatic_checkpoint",
		State: map[string]interface{}{
			"events_processed": erm.stats.EventsProcessed,
			"events_skipped":   erm.stats.EventsSkipped,
			"events_errored":   erm.stats.EventsErrored,
			"event_type_stats": erm.stats.EventTypeStats,
		},
	}

	if err := erm.checkpointStore.SaveCheckpoint(ctx, checkpoint); err != nil {
		return err
	}

	erm.stats.CheckpointsCreated++
	erm.stats.CurrentCheckpoint = checkpointID
	erm.logger.Infow("Created replay checkpoint",
		"checkpointID", checkpointID,
		"eventsProcessed", checkpoint.EventsProcessed)

	return nil
}

// calculateThroughput calculates replay throughput
func (erm *EventReplayManager) calculateThroughput() {
	if erm.stats.Duration.Seconds() > 0 {
		erm.stats.ThroughputPerSecond = float64(erm.stats.EventsProcessed) / erm.stats.Duration.Seconds()
	}
}

// GetStatistics returns current replay statistics
func (erm *EventReplayManager) GetStatistics() ReplayStatistics {
	erm.mu.RLock()
	defer erm.mu.RUnlock()
	return erm.stats
}

// ResetStatistics resets replay statistics
func (erm *EventReplayManager) ResetStatistics() {
	erm.mu.Lock()
	defer erm.mu.Unlock()
	erm.stats = ReplayStatistics{
		EventTypeStats: make(map[string]int64),
		ErrorStats:     make(map[string]int64),
	}
}

// FileCheckpointStore implements CheckpointStore using local files
type FileCheckpointStore struct {
	checkpointDir string
	logger        *zap.SugaredLogger
	mu            sync.RWMutex
}

// NewFileCheckpointStore creates a new file-based checkpoint store
func NewFileCheckpointStore(checkpointDir string, logger *zap.SugaredLogger) (*FileCheckpointStore, error) {
	if err := os.MkdirAll(checkpointDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create checkpoint directory: %w", err)
	}

	return &FileCheckpointStore{
		checkpointDir: checkpointDir,
		logger:        logger,
	}, nil
}

// SaveCheckpoint saves a checkpoint to file
func (fcs *FileCheckpointStore) SaveCheckpoint(ctx context.Context, checkpoint *ReplayCheckpoint) error {
	fcs.mu.Lock()
	defer fcs.mu.Unlock()

	filePath := filepath.Join(fcs.checkpointDir, checkpoint.ID+".json")
	data, err := json.MarshalIndent(checkpoint, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal checkpoint: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write checkpoint file: %w", err)
	}

	fcs.logger.Infow("Saved checkpoint", "checkpointID", checkpoint.ID, "path", filePath)
	return nil
}

// LoadCheckpoint loads a checkpoint from file
func (fcs *FileCheckpointStore) LoadCheckpoint(ctx context.Context, checkpointID string) (*ReplayCheckpoint, error) {
	fcs.mu.RLock()
	defer fcs.mu.RUnlock()

	filePath := filepath.Join(fcs.checkpointDir, checkpointID+".json")
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read checkpoint file: %w", err)
	}

	var checkpoint ReplayCheckpoint
	if err := json.Unmarshal(data, &checkpoint); err != nil {
		return nil, fmt.Errorf("failed to unmarshal checkpoint: %w", err)
	}

	return &checkpoint, nil
}

// ListCheckpoints lists all available checkpoints
func (fcs *FileCheckpointStore) ListCheckpoints(ctx context.Context) ([]*ReplayCheckpoint, error) {
	fcs.mu.RLock()
	defer fcs.mu.RUnlock()

	files, err := os.ReadDir(fcs.checkpointDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read checkpoint directory: %w", err)
	}

	var checkpoints []*ReplayCheckpoint
	for _, file := range files {
		if !strings.HasSuffix(file.Name(), ".json") {
			continue
		}

		checkpointID := strings.TrimSuffix(file.Name(), ".json")
		checkpoint, err := fcs.LoadCheckpoint(ctx, checkpointID)
		if err != nil {
			fcs.logger.Warnw("Failed to load checkpoint", "checkpointID", checkpointID, "error", err)
			continue
		}

		checkpoints = append(checkpoints, checkpoint)
	}

	return checkpoints, nil
}

// DeleteCheckpoint deletes a checkpoint file
func (fcs *FileCheckpointStore) DeleteCheckpoint(ctx context.Context, checkpointID string) error {
	fcs.mu.Lock()
	defer fcs.mu.Unlock()

	filePath := filepath.Join(fcs.checkpointDir, checkpointID+".json")
	if err := os.Remove(filePath); err != nil {
		return fmt.Errorf("failed to delete checkpoint file: %w", err)
	}

	fcs.logger.Infow("Deleted checkpoint", "checkpointID", checkpointID)
	return nil
}

// Helper function
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
