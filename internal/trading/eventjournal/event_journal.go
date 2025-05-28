package eventjournal

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	// TODO: implement replay logic
	return nil
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
