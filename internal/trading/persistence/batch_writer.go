// BatchWriter consumes from AsyncWriteBuffer and writes to the DB in batches.
// Handles retries, backoff, and circuit breaker logic.
package persistence

import (
	"sync/atomic"
	"time"
)

type BatchWriter struct {
	buffer      *AsyncWriteBuffer
	db          DBWriter
	metrics     *WriterMetrics
	circuitOpen atomic.Bool
	backoff     time.Duration
	maxBackoff  time.Duration
	alertFunc   func(error)
}

func NewBatchWriter(buffer *AsyncWriteBuffer, db DBWriter, metrics *WriterMetrics, alertFunc func(error)) *BatchWriter {
	return &BatchWriter{
		buffer:     buffer,
		db:         db,
		metrics:    metrics,
		backoff:    100 * time.Millisecond,
		maxBackoff: 5 * time.Second,
		alertFunc:  alertFunc,
	}
}

func (bw *BatchWriter) Start() {
	go func() {
		for {
			batch := bw.buffer.Flush()
			if len(batch) == 0 {
				time.Sleep(10 * time.Millisecond)
				continue
			}
			if bw.circuitOpen.Load() {
				bw.metrics.IncCircuitBreakerTrips()
				time.Sleep(bw.backoff)
				continue
			}
			err := bw.db.WriteBatch(batch)
			if err != nil {
				bw.handleWriteError(err)
				continue
			}
			bw.metrics.RecordBatch(len(batch))
			bw.backoff = 100 * time.Millisecond // reset
		}
	}()
}

func (bw *BatchWriter) handleWriteError(err error) {
	bw.metrics.IncWriteErrors()
	bw.alertFunc(err)
	bw.backoff *= 2
	if bw.backoff > bw.maxBackoff {
		bw.backoff = bw.maxBackoff
	}
	if bw.backoff == bw.maxBackoff {
		bw.circuitOpen.Store(true)
		// Optionally: start a timer to auto-close circuit after cooldown
	}
}

func (bw *BatchWriter) CloseCircuit() {
	bw.circuitOpen.Store(false)
	bw.backoff = 100 * time.Millisecond
}
