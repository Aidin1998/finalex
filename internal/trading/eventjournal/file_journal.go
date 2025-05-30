package eventjournal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// FileEventJournal implements persistent event journaling to disk
type FileEventJournal struct {
	logger   *zap.Logger
	filePath string
	file     *os.File
	mu       sync.Mutex
	config   Config
}

// Config holds configuration for the event journal
type Config struct {
	FilePath     string
	MaxSizeBytes int64
	MaxBackups   int
	MaxAgeDays   int
	Enabled      bool
	BufferSize   int
}

// DefaultConfig returns a default event journal configuration
func DefaultConfig() Config {
	return Config{
		FilePath:     "/var/log/trading/events.log",
		MaxSizeBytes: 100 * 1024 * 1024, // 100MB
		MaxBackups:   10,
		MaxAgeDays:   30,
		Enabled:      true,
		BufferSize:   1000,
	}
}

// NewFileEventJournal creates a new file-based event journal
func NewFileEventJournal(config Config, logger *zap.Logger) (*FileEventJournal, error) {
	if !config.Enabled {
		return &FileEventJournal{
			logger: logger,
			config: config,
		}, nil
	}

	// Ensure directory exists
	dir := filepath.Dir(config.FilePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	// Open file for appending
	file, err := os.OpenFile(config.FilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open event journal file: %w", err)
	}

	journal := &FileEventJournal{
		logger:   logger,
		filePath: config.FilePath,
		file:     file,
		config:   config,
	}

	logger.Info("Event journal initialized", zap.String("file_path", config.FilePath))
	return journal, nil
}

// WriteEvent writes an event to the journal
func (j *FileEventJournal) WriteEvent(ctx context.Context, event *WALEvent) error {
	if !j.config.Enabled || j.file == nil {
		return nil // Journal disabled
	}

	j.mu.Lock()
	defer j.mu.Unlock()

	// Serialize event to JSON
	eventData, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// Add newline for line-delimited JSON
	eventData = append(eventData, '\n')

	// Write to file
	if _, err := j.file.Write(eventData); err != nil {
		j.logger.Error("Failed to write event to journal", zap.Error(err))
		return fmt.Errorf("failed to write event: %w", err)
	}

	// Sync to disk for durability
	if err := j.file.Sync(); err != nil {
		j.logger.Error("Failed to sync journal to disk", zap.Error(err))
		return fmt.Errorf("failed to sync journal: %w", err)
	}

	return nil
}

// WriteOrderPlaced writes an order placed event
func (j *FileEventJournal) WriteOrderPlaced(ctx context.Context, orderID uuid.UUID, pair string, data interface{}) error {
	event := &WALEvent{
		Timestamp: time.Now(),
		EventType: EventTypeOrderPlaced,
		Pair:      pair,
		OrderID:   orderID,
		Data:      data,
	}
	return j.WriteEvent(ctx, event)
}

// WriteOrderCancelled writes an order cancelled event
func (j *FileEventJournal) WriteOrderCancelled(ctx context.Context, orderID uuid.UUID, pair string, data interface{}) error {
	event := &WALEvent{
		Timestamp: time.Now(),
		EventType: EventTypeOrderCancelled,
		Pair:      pair,
		OrderID:   orderID,
		Data:      data,
	}
	return j.WriteEvent(ctx, event)
}

// WriteTradeExecuted writes a trade executed event
func (j *FileEventJournal) WriteTradeExecuted(ctx context.Context, orderID uuid.UUID, pair string, data interface{}) error {
	event := &WALEvent{
		Timestamp: time.Now(),
		EventType: EventTypeTradeExecuted,
		Pair:      pair,
		OrderID:   orderID,
		Data:      data,
	}
	return j.WriteEvent(ctx, event)
}

// WriteCheckpoint writes a checkpoint event for recovery purposes
func (j *FileEventJournal) WriteCheckpoint(ctx context.Context, pair string, data interface{}) error {
	event := &WALEvent{
		Timestamp: time.Now(),
		EventType: EventTypeCheckpoint,
		Pair:      pair,
		Data:      data,
	}
	return j.WriteEvent(ctx, event)
}

// Close closes the event journal
func (j *FileEventJournal) Close() error {
	if j.file != nil {
		j.mu.Lock()
		defer j.mu.Unlock()

		if err := j.file.Sync(); err != nil {
			j.logger.Error("Failed to sync journal before close", zap.Error(err))
		}

		if err := j.file.Close(); err != nil {
			j.logger.Error("Failed to close journal file", zap.Error(err))
			return err
		}

		j.file = nil
		j.logger.Info("Event journal closed")
	}
	return nil
}

// Rotate rotates the log file when it becomes too large
func (j *FileEventJournal) Rotate() error {
	if !j.config.Enabled || j.file == nil {
		return nil
	}

	j.mu.Lock()
	defer j.mu.Unlock()

	// Check file size
	stat, err := j.file.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat journal file: %w", err)
	}

	if stat.Size() < j.config.MaxSizeBytes {
		return nil // No rotation needed
	}

	// Close current file
	if err := j.file.Close(); err != nil {
		return fmt.Errorf("failed to close current journal file: %w", err)
	}

	// Rotate files
	timestamp := time.Now().Format("20060102-150405")
	backupPath := fmt.Sprintf("%s.%s", j.filePath, timestamp)

	if err := os.Rename(j.filePath, backupPath); err != nil {
		return fmt.Errorf("failed to rotate journal file: %w", err)
	}

	// Open new file
	file, err := os.OpenFile(j.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open new journal file: %w", err)
	}

	j.file = file
	j.logger.Info("Journal file rotated",
		zap.String("old_file", backupPath),
		zap.String("new_file", j.filePath))

	// Clean up old backup files
	go j.cleanupOldBackups()

	return nil
}

// cleanupOldBackups removes old backup files based on configuration
func (j *FileEventJournal) cleanupOldBackups() {
	dir := filepath.Dir(j.filePath)
	baseName := filepath.Base(j.filePath)

	entries, err := os.ReadDir(dir)
	if err != nil {
		j.logger.Error("Failed to read journal directory for cleanup", zap.Error(err))
		return
	}

	var backupFiles []os.DirEntry
	cutoffTime := time.Now().AddDate(0, 0, -j.config.MaxAgeDays)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// Check if it's a backup file for our journal
		if len(entry.Name()) > len(baseName) && entry.Name()[:len(baseName)] == baseName {
			backupFiles = append(backupFiles, entry)
		}
	}

	// Remove files older than MaxAgeDays or beyond MaxBackups count
	if len(backupFiles) > j.config.MaxBackups {
		// Sort by modification time and remove oldest
		// For simplicity, we'll just remove files older than cutoff time
	}

	for _, backup := range backupFiles {
		info, err := backup.Info()
		if err != nil {
			continue
		}

		if info.ModTime().Before(cutoffTime) {
			backupPath := filepath.Join(dir, backup.Name())
			if err := os.Remove(backupPath); err != nil {
				j.logger.Error("Failed to remove old backup file",
					zap.Error(err),
					zap.String("file", backupPath))
			} else {
				j.logger.Info("Removed old backup file", zap.String("file", backupPath))
			}
		}
	}
}

// GetEvents reads events from the journal for recovery purposes
func (j *FileEventJournal) GetEvents(ctx context.Context, fromTime time.Time) ([]*WALEvent, error) {
	if !j.config.Enabled {
		return nil, nil
	}

	file, err := os.Open(j.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No events yet
		}
		return nil, fmt.Errorf("failed to open journal for reading: %w", err)
	}
	defer file.Close()

	var events []*WALEvent
	decoder := json.NewDecoder(file)

	for {
		var event WALEvent
		if err := decoder.Decode(&event); err != nil {
			break // End of file or parse error
		}

		if event.Timestamp.After(fromTime) {
			events = append(events, &event)
		}
	}

	return events, nil
}

// ReplayEvents replays all events from the journal with advanced error handling
func (j *FileEventJournal) ReplayEvents(ctx context.Context, handler func(*WALEvent) (bool, error)) error {
	if !j.config.Enabled {
		j.logger.Info("Journal is disabled, no events to replay")
		return nil
	}

	j.mu.Lock()
	defer j.mu.Unlock()

	// Open file for reading
	file, err := os.Open(j.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			j.logger.Info("No journal file found for replay")
			return nil
		}
		return fmt.Errorf("failed to open journal file for replay: %w", err)
	}
	defer file.Close()

	j.logger.Info("Starting event replay from file journal", zap.String("path", j.filePath))

	decoder := json.NewDecoder(file)
	eventCount := 0
	errorCount := 0

	for {
		select {
		case <-ctx.Done():
			j.logger.Warn("Replay cancelled by context")
			return ctx.Err()
		default:
		}

		var event WALEvent
		if err := decoder.Decode(&event); err != nil {
			if err == io.EOF {
				break // End of file reached
			}

			errorCount++
			j.logger.Error("Failed to decode event during replay",
				zap.Error(err),
				zap.Int("eventCount", eventCount))
			continue
		}

		eventCount++

		shouldContinue, err := handler(&event)
		if err != nil {
			j.logger.Error("Handler error during replay",
				zap.Error(err),
				zap.String("eventType", event.EventType),
				zap.Int("eventCount", eventCount))

			if !shouldContinue {
				return fmt.Errorf("replay stopped due to handler error: %w", err)
			}
		}

		if !shouldContinue {
			j.logger.Info("Replay stopped by handler", zap.Int("eventCount", eventCount))
			break
		}

		// Log progress every 1000 events
		if eventCount%1000 == 0 {
			j.logger.Info("Replay progress", zap.Int("eventsProcessed", eventCount))
		}
	}

	j.logger.Info("Event replay completed",
		zap.Int("totalEvents", eventCount),
		zap.Int("errorCount", errorCount))

	return nil
}

// ReplayEventsFromTime replays events from a specific timestamp
func (j *FileEventJournal) ReplayEventsFromTime(ctx context.Context, fromTime time.Time, handler func(*WALEvent) (bool, error)) error {
	if !j.config.Enabled {
		return nil
	}

	events, err := j.GetEvents(ctx, fromTime)
	if err != nil {
		return fmt.Errorf("failed to get events from time: %w", err)
	}

	j.logger.Info("Starting time-based replay",
		zap.Time("fromTime", fromTime),
		zap.Int("eventsToReplay", len(events)))

	for i, event := range events {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		shouldContinue, err := handler(event)
		if err != nil {
			j.logger.Error("Handler error during time-based replay",
				zap.Error(err),
				zap.String("eventType", event.EventType),
				zap.Int("eventIndex", i))

			if !shouldContinue {
				return fmt.Errorf("time-based replay stopped due to handler error: %w", err)
			}
		}

		if !shouldContinue {
			j.logger.Info("Time-based replay stopped by handler", zap.Int("eventIndex", i))
			break
		}
	}

	j.logger.Info("Time-based replay completed", zap.Int("eventsProcessed", len(events)))
	return nil
}
