// Lock-Free Ring Buffer for Zero-Copy Order Pipeline
// Power-of-2 size, memory barriers for producer-consumer, zero allocations in hot path

package orderbook

import (
	"sync/atomic"
)

const orderRingBufferSize = 1024 // Must be power of 2

type orderRingBuffer struct {
	buffer [orderRingBufferSize]*ZeroCopyOrder
	mask   uint64
	head   uint64 // atomic
	tail   uint64 // atomic
}

func newOrderRingBuffer() *orderRingBuffer {
	return &orderRingBuffer{
		mask: orderRingBufferSize - 1,
	}
}

// Exported wrapper for test and integration
func NewOrderRingBuffer() *orderRingBuffer {
	return newOrderRingBuffer()
}

// Producer: Enqueue order (returns false if full)
func (rb *orderRingBuffer) Enqueue(order *ZeroCopyOrder) bool {
	tail := atomic.LoadUint64(&rb.tail)
	head := atomic.LoadUint64(&rb.head)
	if tail-head >= orderRingBufferSize {
		return false // full
	}
	idx := tail & rb.mask
	rb.buffer[idx] = order
	atomic.AddUint64(&rb.tail, 1)
	return true
}

// Consumer: Dequeue order (returns nil if empty)
func (rb *orderRingBuffer) Dequeue() *ZeroCopyOrder {
	head := atomic.LoadUint64(&rb.head)
	tail := atomic.LoadUint64(&rb.tail)
	if head == tail {
		return nil // empty
	}
	idx := head & rb.mask
	order := rb.buffer[idx]
	rb.buffer[idx] = nil // avoid memory leak
	atomic.AddUint64(&rb.head, 1)
	return order
}
