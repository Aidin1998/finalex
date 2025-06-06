package marketmaker

import (
	"sync"
	"time"
)

type LedgerEvent struct {
	Timestamp time.Time
	Pair      string
	Action    string // create, update, cancel, etc.
	OrderID   string
	Details   map[string]interface{}
}

type Ledger struct {
	mu     sync.Mutex
	events []LedgerEvent
}

func NewLedger() *Ledger {
	return &Ledger{
		events: make([]LedgerEvent, 0, 1024),
	}
}

func (l *Ledger) Record(event LedgerEvent) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = append(l.events, event)
}

func (l *Ledger) Events() []LedgerEvent {
	l.mu.Lock()
	defer l.mu.Unlock()
	copyEvents := make([]LedgerEvent, len(l.events))
	copy(copyEvents, l.events)
	return copyEvents
}

// TODO: Add event publishing hooks (Kafka, Redis, etc.) for audit/monitoring
