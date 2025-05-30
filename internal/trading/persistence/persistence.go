// Facade for async persistence layer: exposes API for order/trade persistence.
package persistence

import (
	"time"
)

// DBWriter abstracts the DB write operation for orders/trades
// (implement in database package or as adapter)
type DBWriter interface {
	WriteBatch(batch []WriteRequest) error
}

type PersistenceLayer struct {
	Buffer        *AsyncWriteBuffer
	Writer        *BatchWriter
	WAL           WAL
	Reconciler    *ReconciliationService
	BufferMetrics *BufferMetrics
	WriterMetrics *WriterMetrics
	ReconMetrics  *ReconciliationMetrics
}

func NewPersistenceLayer(db DBWriter, walPath string) (*PersistenceLayer, error) {
	wal, err := NewFileWAL(walPath)
	if err != nil {
		return nil, err
	}
	bufMetrics := &BufferMetrics{}
	writerMetrics := &WriterMetrics{}
	reconMetrics := &ReconciliationMetrics{}
	buffer := NewAsyncWriteBuffer(1000, 10*time.Millisecond, wal, bufMetrics)
	writer := NewBatchWriter(buffer, db, writerMetrics, func(err error) { /* alert hook */ })
	reconciler := NewReconciliationService(wal, db, reconMetrics)
	return &PersistenceLayer{
		Buffer:        buffer,
		Writer:        writer,
		WAL:           wal,
		Reconciler:    reconciler,
		BufferMetrics: bufMetrics,
		WriterMetrics: writerMetrics,
		ReconMetrics:  reconMetrics,
	}, nil
}

// EnqueueOrder/Trade for persistence
func (pl *PersistenceLayer) EnqueueOrder(order interface{}) error {
	return pl.Buffer.Enqueue(WriteRequest{Type: "order", Data: order})
}
func (pl *PersistenceLayer) EnqueueTrade(trade interface{}) error {
	return pl.Buffer.Enqueue(WriteRequest{Type: "trade", Data: trade})
}
