// AsyncWriteBuffer buffers persistence requests for orders/trades and flushes them asynchronously.
// It supports concurrent enqueue, size/time-based flush, and integrates with WAL for durability.
package persistence

import (
	"sync"
	"time"
)

type WriteRequest struct {
	Type   string      // "order" or "trade"
	Data   interface{} // *model.Order or *model.Trade
	WALSeq uint64      // WAL sequence number for durability
}

type AsyncWriteBuffer struct {
	buffer      []WriteRequest
	maxSize     int
	flushTicker *time.Ticker
	flushCh     chan struct{}
	mu          sync.Mutex
	wal         WAL
	metrics     *BufferMetrics
}

func NewAsyncWriteBuffer(maxSize int, flushInterval time.Duration, wal WAL, metrics *BufferMetrics) *AsyncWriteBuffer {
	awb := &AsyncWriteBuffer{
		buffer:      make([]WriteRequest, 0, maxSize),
		maxSize:     maxSize,
		flushTicker: time.NewTicker(flushInterval),
		flushCh:     make(chan struct{}, 1),
		wal:         wal,
		metrics:     metrics,
	}
	go awb.run()
	return awb
}

// Enqueue adds a write request to the buffer and WAL, triggers flush if needed.
func (awb *AsyncWriteBuffer) Enqueue(req WriteRequest) error {
	awb.mu.Lock()
	defer awb.mu.Unlock()
	seq, err := awb.wal.Append(req)
	if err != nil {
		return err
	}
	req.WALSeq = seq
	awb.buffer = append(awb.buffer, req)
	awb.metrics.SetBufferSize(len(awb.buffer))
	if len(awb.buffer) >= awb.maxSize {
		awb.triggerFlush()
	}
	return nil
}

func (awb *AsyncWriteBuffer) triggerFlush() {
	select {
	case awb.flushCh <- struct{}{}:
	default:
	}
}

func (awb *AsyncWriteBuffer) run() {
	for {
		select {
		case <-awb.flushTicker.C:
			awb.Flush()
		case <-awb.flushCh:
			awb.Flush()
		}
	}
}

// Flush returns a batch of requests and clears the buffer. Called by BatchWriter.
func (awb *AsyncWriteBuffer) Flush() []WriteRequest {
	awb.mu.Lock()
	defer awb.mu.Unlock()
	batch := make([]WriteRequest, len(awb.buffer))
	copy(batch, awb.buffer)
	awb.buffer = awb.buffer[:0]
	awb.metrics.SetBufferSize(0)
	return batch
}
