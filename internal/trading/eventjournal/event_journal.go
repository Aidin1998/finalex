package eventjournal

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Event type constants
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

// EventJournal handles writing and replaying events for recovery.
type EventJournal struct {
	filePath string
	file     *os.File
	writer   *bufio.Writer
	mu       sync.Mutex
	log      *zap.SugaredLogger
}

// NewEventJournal creates or opens an event journal file.
func NewEventJournal(log *zap.SugaredLogger, journalPath string) (*EventJournal, error) {
	if err := os.MkdirAll(filepath.Dir(journalPath), 0755); err != nil {
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

// ReplayEvents replays all events from the journal to restore engine state.
func (ej *EventJournal) ReplayEvents(handler func(WALEvent) (bool, error)) error {
	ej.mu.Lock()
	defer ej.mu.Unlock()

	// Close writer and reopen file for reading
	if ej.writer != nil {
		if err := ej.writer.Flush(); err != nil {
			ej.log.Errorw("Failed to flush writer before replay", "error", err)
		}
	}

	// Open file for reading
	file, err := os.Open(ej.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			ej.log.Info("No journal file found for replay")
			return nil // No events to replay
		}
		return fmt.Errorf("failed to open journal file for replay: %w", err)
	}
	defer file.Close()

	ej.log.Info("Starting event replay from journal", zap.String("path", ej.filePath))

	scanner := bufio.NewScanner(file)
	eventCount := 0
	errorCount := 0

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue // Skip empty lines
		}

		var event WALEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			errorCount++
			ej.log.Errorw("Failed to unmarshal event during replay",
				"error", err,
				"line", line[:min(100, len(line))],
				"eventCount", eventCount)
			continue // Skip corrupted events
		}

		eventCount++

		// Call the handler with the event
		shouldContinue, err := handler(event)
		if err != nil {
			ej.log.Errorw("Handler error during event replay",
				"error", err,
				"eventType", event.EventType,
				"eventCount", eventCount)

			// Check if we should continue on error
			if !shouldContinue {
				return fmt.Errorf("replay stopped due to handler error: %w", err)
			}
		}

		if !shouldContinue {
			ej.log.Infow("Replay stopped by handler", "eventCount", eventCount)
			break
		}

		// Log progress every 1000 events
		if eventCount%1000 == 0 {
			ej.log.Infow("Replay progress", "eventsProcessed", eventCount)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading journal during replay: %w", err)
	}

	ej.log.Infow("Event replay completed",
		"totalEvents", eventCount,
		"errorCount", errorCount)

	return nil
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// WriteEvent writes a WALEvent to the journal.
func (ej *EventJournal) WriteEvent(event WALEvent) error {
	ej.mu.Lock()
	defer ej.mu.Unlock()
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	if _, err := ej.writer.Write(data); err != nil {
		return err
	}
	if _, err := ej.writer.WriteString("\n"); err != nil {
		return err
	}
	return ej.writer.Flush()
}
