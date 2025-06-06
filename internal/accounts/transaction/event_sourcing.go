// Asynchronous Transaction Event Sourcing & Saga Orchestration
// Implements event-driven, saga-based, async transaction processing for accounts
package transaction

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/shopspring/decimal"
)

// TransactionEventType enumerates all event types
// (extend as needed for all transaction types)
type TransactionEventType string

const (
	EventCreated   TransactionEventType = "created"
	EventReserved  TransactionEventType = "reserved"
	EventDebited   TransactionEventType = "debited"
	EventCredited  TransactionEventType = "credited"
	EventReleased  TransactionEventType = "released"
	EventFailed    TransactionEventType = "failed"
	EventCompleted TransactionEventType = "completed"
)

type TransactionEvent struct {
	EventID       uint64               `json:"event_id"`
	TransactionID uint64               `json:"transaction_id"`
	UserID        uint64               `json:"user_id"`
	EventType     TransactionEventType `json:"event_type"`
	Payload       json.RawMessage      `json:"payload"`
	Timestamp     int64                `json:"timestamp"`
	Version       uint32               `json:"version"`
}

// EventStore interface for event sourcing
// (implement with DB, WAL, or in-memory for tests)
type EventStore interface {
	AppendEvents(transactionID uint64, events []TransactionEvent) error
	ListEvents(transactionID uint64) ([]TransactionEvent, error)
}

// StateStore interface for transaction state
// (in-memory for hot path, async persistence)
type StateStore interface {
	GetState(transactionID uint64) (*TransactionState, error)
	SetState(transactionID uint64, state *TransactionState) error
	ListPendingTransactions() ([]uint64, error)
}

type TransactionState struct {
	TransactionID uint64
	State         string
	UpdatedAt     int64
	Version       uint32
}

type TransactionProcessor struct {
	eventStore  EventStore
	stateStore  StateStore
	sagaManager *SagaManager
	eventBus    chan TransactionEvent
	workers     []*TransactionWorker
	overflow    chan TransactionEvent
}

func NewTransactionProcessor(eventStore EventStore, stateStore StateStore, sagaManager *SagaManager, workerCount int) *TransactionProcessor {
	tp := &TransactionProcessor{
		eventStore:  eventStore,
		stateStore:  stateStore,
		sagaManager: sagaManager,
		eventBus:    make(chan TransactionEvent, 10000),
		overflow:    make(chan TransactionEvent, 1000),
	}
	for i := 0; i < workerCount; i++ {
		worker := &TransactionWorker{eventBus: tp.eventBus, sagaManager: sagaManager}
		tp.workers = append(tp.workers, worker)
		go worker.Start()
	}
	return tp
}

func (tp *TransactionProcessor) ProcessTransaction(tx *Transaction) error {
	events := tx.ToEvents()
	if err := tp.eventStore.AppendEvents(tx.ID, events); err != nil {
		return err
	}
	for _, event := range events {
		select {
		case tp.eventBus <- event:
		default:
			tp.overflow <- event
		}
	}
	return nil
}

// TransactionWorker processes events asynchronously
type TransactionWorker struct {
	eventBus    chan TransactionEvent
	sagaManager *SagaManager
}

func (tw *TransactionWorker) Start() {
	for event := range tw.eventBus {
		tw.sagaManager.HandleEvent(event)
	}
}

// SagaManager orchestrates multi-step transactions (saga pattern)
type SagaManager struct {
	sagas sync.Map // transactionID -> Saga
}

type Saga interface {
	HandleEvent(event TransactionEvent)
}

func (sm *SagaManager) HandleEvent(event TransactionEvent) {
	if saga, ok := sm.sagas.Load(event.TransactionID); ok {
		saga.(Saga).HandleEvent(event)
	}
}

// Example: TransferSaga for account-to-account transfer
// (extend for other transaction types)
type TransferSaga struct {
	sagaID        uint64
	fromUserID    uint64
	toUserID      uint64
	amount        decimal.Decimal
	currency      uint32
	state         string
	compensations []SagaStep
}

type SagaStep interface {
	Execute() error
	GetCompensation() CompensationStep
}

type CompensationStep interface {
	Execute() error
}

func (ts *TransferSaga) HandleEvent(event TransactionEvent) {
	// Implement state machine transitions and compensation logic here
}

// IdempotentOperation for safe retries
type IdempotentOperation struct {
	OperationID string
	UserID      uint64
	Type        string
	Payload     json.RawMessage
	Status      string
	Result      json.RawMessage
	CreatedAt   time.Time
	CompletedAt *time.Time
}

func (op *IdempotentOperation) Execute() error {
	if op.Status == "completed" {
		return nil
	}
	result, err := op.doExecute()
	if err != nil {
		op.Status = "failed"
		return err
	}
	op.Result = result
	op.Status = "completed"
	now := time.Now()
	op.CompletedAt = &now
	return nil
}

func (op *IdempotentOperation) doExecute() ([]byte, error) {
	// Implement actual operation logic here
	return nil, nil
}

// InMemoryStateStore is a high-performance state store for hot path
// (async persistence to DB via persister channel)
type InMemoryStateStore struct {
	states    sync.Map           // transactionID -> *TransactionState
	pending   chan uint64        // pending transaction IDs
	persister chan StateSnapshot // async persistence
}

type StateSnapshot struct {
	TxID  uint64
	State *TransactionState
}

func NewInMemoryStateStore() *InMemoryStateStore {
	return &InMemoryStateStore{
		pending:   make(chan uint64, 10000),
		persister: make(chan StateSnapshot, 1000),
	}
}

func (imss *InMemoryStateStore) GetState(transactionID uint64) (*TransactionState, error) {
	if state, ok := imss.states.Load(transactionID); ok {
		return state.(*TransactionState), nil
	}
	return nil, nil
}

func (imss *InMemoryStateStore) SetState(transactionID uint64, state *TransactionState) error {
	imss.states.Store(transactionID, state)
	select {
	case imss.persister <- StateSnapshot{TxID: transactionID, State: state}:
	default:
		log.Printf("State persistence queue full for txID=%d", transactionID)
	}
	return nil
}

func (imss *InMemoryStateStore) ListPendingTransactions() ([]uint64, error) {
	var ids []uint64
	for {
		select {
		case id := <-imss.pending:
			ids = append(ids, id)
		default:
			return ids, nil
		}
	}
}
