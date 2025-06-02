// Package backpressure provides high-performance backpressure handling for market data distribution
package backpressure

import (
	"sync/atomic"
	"time"

	"unsafe"

	"go.uber.org/zap"
)

// MessagePriority defines message priority levels
type MessagePriority uint8

const (
	PriorityCritical MessagePriority = 0 // Trades, settlements - NEVER drop
	PriorityHigh     MessagePriority = 1 // Order fills, cancellations
	PriorityMedium   MessagePriority = 2 // Order book updates
	PriorityLow      MessagePriority = 3 // Statistics, heartbeats
	PriorityMarket   MessagePriority = 4 // Market data (can be dropped)
)

// PriorityMessage represents a message with priority and metadata
type PriorityMessage struct {
	Data         []byte
	Topic        string
	Priority     MessagePriority
	Timestamp    int64 // Unix nanoseconds for minimal allocation
	ClientID     string
	MessageID    uint64
	ExpiresAt    int64 // Unix nanoseconds, 0 = never expires
	Attempts     uint8
	IsRetransmit bool
}

// LockFreePriorityQueue implements a lock-free priority queue optimized for minimal latency
type LockFreePriorityQueue struct {
	// Separate queues per priority for cache efficiency
	queues [5]*lockFreeRingBuffer

	// Fast path for critical messages
	criticalFast chan *PriorityMessage

	// Metrics (atomic access only)
	stats QueueStats

	// Configuration
	config *QueueConfig
	logger *zap.Logger
}

// QueueConfig defines queue configuration
type QueueConfig struct {
	CriticalQueueSize int           // Size for critical messages (trades/settlements)
	HighQueueSize     int           // Size for high priority messages
	MediumQueueSize   int           // Size for medium priority messages
	LowQueueSize      int           // Size for low priority messages
	MarketQueueSize   int           // Size for market data messages
	FastPathEnabled   bool          // Enable ultra-fast path for critical messages
	DropPolicy        DropPolicy    // What to drop when queues are full
	MaxLatency        time.Duration // Maximum acceptable latency
}

// DropPolicy defines what to drop when queues are full
type DropPolicy int

const (
	DropOldest      DropPolicy = 0 // Drop oldest messages first
	DropLowPriority DropPolicy = 1 // Drop lower priority messages first
	DropExpired     DropPolicy = 2 // Drop expired messages first
	DropNever       DropPolicy = 3 // Never drop critical/high priority
)

// QueueStats tracks queue performance with atomic operations
type QueueStats struct {
	MessagesEnqueued [5]uint64 // Per-priority counters
	MessagesDequeued [5]uint64
	MessagesDropped  [5]uint64
	FastPathUsed     uint64
	TotalLatencyNs   uint64
	MaxLatencyNs     uint64
	QueueFullEvents  uint64
	OverflowEvents   uint64
}

// lockFreeRingBuffer implements a single-producer, single-consumer lock-free ring buffer
type lockFreeRingBuffer struct {
	buffer   []unsafe.Pointer // Store *PriorityMessage
	mask     uint64           // Size must be power of 2
	writePos uint64           // Atomic write position
	readPos  uint64           // Atomic read position
	capacity uint64

	// Padding to prevent false sharing
	_ [8]uint64
}

// NewLockFreePriorityQueue creates a new lock-free priority queue
func NewLockFreePriorityQueue(config *QueueConfig, logger *zap.Logger) *LockFreePriorityQueue {
	if config == nil {
		config = DefaultQueueConfig()
	}

	q := &LockFreePriorityQueue{
		config: config,
		logger: logger,
	}

	// Initialize per-priority queues with optimized sizes
	q.queues[PriorityCritical] = newLockFreeRingBuffer(config.CriticalQueueSize)
	q.queues[PriorityHigh] = newLockFreeRingBuffer(config.HighQueueSize)
	q.queues[PriorityMedium] = newLockFreeRingBuffer(config.MediumQueueSize)
	q.queues[PriorityLow] = newLockFreeRingBuffer(config.LowQueueSize)
	q.queues[PriorityMarket] = newLockFreeRingBuffer(config.MarketQueueSize)

	// Initialize fast path for critical messages
	if config.FastPathEnabled {
		q.criticalFast = make(chan *PriorityMessage, 16) // Small buffered channel for burst
	}

	return q
}

// DefaultQueueConfig returns optimized default configuration
func DefaultQueueConfig() *QueueConfig {
	return &QueueConfig{
		CriticalQueueSize: 4096,                   // Large buffer for trades/settlements
		HighQueueSize:     2048,                   // Medium buffer for order events
		MediumQueueSize:   1024,                   // Smaller buffer for order book updates
		LowQueueSize:      512,                    // Small buffer for statistics
		MarketQueueSize:   256,                    // Tiny buffer for market data (can drop)
		FastPathEnabled:   true,                   // Enable ultra-fast critical path
		DropPolicy:        DropNever,              // Never drop critical/high priority
		MaxLatency:        time.Microsecond * 100, // 100μs max latency target
	}
}

// newLockFreeRingBuffer creates a new lock-free ring buffer
func newLockFreeRingBuffer(size int) *lockFreeRingBuffer {
	// Ensure size is power of 2 for efficient masking
	capacity := uint64(1)
	for capacity < uint64(size) {
		capacity <<= 1
	}

	return &lockFreeRingBuffer{
		buffer:   make([]unsafe.Pointer, capacity),
		mask:     capacity - 1,
		capacity: capacity,
	}
}

// Enqueue adds a message to the appropriate priority queue
func (q *LockFreePriorityQueue) Enqueue(msg *PriorityMessage) bool {
	startTime := time.Now().UnixNano()
	priority := msg.Priority

	// Fast path for critical messages
	if priority == PriorityCritical && q.config.FastPathEnabled {
		select {
		case q.criticalFast <- msg:
			atomic.AddUint64(&q.stats.FastPathUsed, 1)
			atomic.AddUint64(&q.stats.MessagesEnqueued[priority], 1)
			return true
		default:
			// Fall through to normal path if fast path is full
		}
	}

	// Normal path - use lock-free ring buffer
	buffer := q.queues[priority]
	success := buffer.enqueue(msg)

	if success {
		atomic.AddUint64(&q.stats.MessagesEnqueued[priority], 1)

		// Update latency stats
		latency := uint64(time.Now().UnixNano() - startTime)
		atomic.AddUint64(&q.stats.TotalLatencyNs, latency)

		// Update max latency (racy but acceptable for monitoring)
		for {
			current := atomic.LoadUint64(&q.stats.MaxLatencyNs)
			if latency <= current || atomic.CompareAndSwapUint64(&q.stats.MaxLatencyNs, current, latency) {
				break
			}
		}
	} else {
		// Queue is full - apply drop policy
		success = q.handleQueueFull(msg, priority)
		if !success {
			atomic.AddUint64(&q.stats.MessagesDropped[priority], 1)
		}
	}

	return success
}

// Dequeue removes the highest priority message available
func (q *LockFreePriorityQueue) Dequeue() (*PriorityMessage, bool) {
	// Check fast path for critical messages first
	if q.config.FastPathEnabled {
		select {
		case msg := <-q.criticalFast:
			atomic.AddUint64(&q.stats.MessagesDequeued[PriorityCritical], 1)
			return msg, true
		default:
			// Continue to normal priority checking
		}
	}

	// Check priorities in order: Critical -> High -> Medium -> Low -> Market
	for priority := PriorityCritical; priority <= PriorityMarket; priority++ {
		if msg := q.queues[priority].dequeue(); msg != nil {
			atomic.AddUint64(&q.stats.MessagesDequeued[priority], 1)
			return msg, true
		}
	}

	return nil, false
}

// handleQueueFull implements the drop policy when queues are full
func (q *LockFreePriorityQueue) handleQueueFull(msg *PriorityMessage, priority MessagePriority) bool {
	atomic.AddUint64(&q.stats.QueueFullEvents, 1)

	switch q.config.DropPolicy {
	case DropNever:
		// Never drop critical or high priority messages
		if priority <= PriorityHigh {
			// Try to make space by dropping lower priority messages
			return q.makeSpaceForCritical(msg)
		}
		return false

	case DropOldest:
		// Try to drop oldest message from same priority queue
		buffer := q.queues[priority]
		if oldMsg := buffer.dequeue(); oldMsg != nil {
			atomic.AddUint64(&q.stats.MessagesDropped[priority], 1)
			return buffer.enqueue(msg)
		}
		return false

	case DropLowPriority:
		// Drop messages from lower priority queues
		return q.dropLowerPriority(msg, priority)

	case DropExpired:
		// Drop expired messages first
		return q.dropExpiredMessages(msg, priority)

	default:
		return false
	}
}

// makeSpaceForCritical makes space for critical messages by dropping lower priority ones
func (q *LockFreePriorityQueue) makeSpaceForCritical(msg *PriorityMessage) bool {
	now := time.Now().UnixNano()

	// Start from lowest priority and work up
	for priority := PriorityMarket; priority > PriorityHigh; priority-- {
		buffer := q.queues[priority]
		if oldMsg := buffer.dequeue(); oldMsg != nil {
			atomic.AddUint64(&q.stats.MessagesDropped[priority], 1)

			// Now try to enqueue the critical message
			targetBuffer := q.queues[msg.Priority]
			if targetBuffer.enqueue(msg) {
				q.logger.Warn("Dropped lower priority message for critical message",
					zap.Uint8("dropped_priority", uint8(priority)),
					zap.Uint8("critical_priority", uint8(msg.Priority)),
					zap.String("topic", msg.Topic))
				return true
			}
		}
	}

	// If we couldn't make space, try overflow handling
	return q.handleOverflow(msg, now)
}

// dropLowerPriority drops messages from lower priority queues
func (q *LockFreePriorityQueue) dropLowerPriority(msg *PriorityMessage, msgPriority MessagePriority) bool {
	// Try to drop from lower priority queues first
	for priority := PriorityMarket; priority > msgPriority; priority-- {
		buffer := q.queues[priority]
		if oldMsg := buffer.dequeue(); oldMsg != nil {
			atomic.AddUint64(&q.stats.MessagesDropped[priority], 1)

			// Now enqueue the new message
			if q.queues[msgPriority].enqueue(msg) {
				return true
			}
		}
	}
	return false
}

// dropExpiredMessages drops expired messages to make space
func (q *LockFreePriorityQueue) dropExpiredMessages(msg *PriorityMessage, priority MessagePriority) bool {
	now := time.Now().UnixNano()

	// Check all queues for expired messages, starting with lowest priority
	for p := PriorityMarket; p >= PriorityCritical; p-- {
		buffer := q.queues[p]

		// Try to find and remove expired messages
		// This is a simplified approach - in production you might want a more sophisticated expired message tracking
		if oldMsg := buffer.dequeue(); oldMsg != nil {
			if oldMsg.ExpiresAt > 0 && now > oldMsg.ExpiresAt {
				// Message is expired, drop it
				atomic.AddUint64(&q.stats.MessagesDropped[p], 1)

				// Try to enqueue new message
				if q.queues[priority].enqueue(msg) {
					return true
				}
			} else {
				// Message not expired, put it back
				buffer.enqueue(oldMsg)
			}
		}
	}

	return false
}

// handleOverflow handles overflow situations for critical messages
func (q *LockFreePriorityQueue) handleOverflow(msg *PriorityMessage, timestamp int64) bool {
	atomic.AddUint64(&q.stats.OverflowEvents, 1)

	q.logger.Error("Critical message overflow - this should never happen",
		zap.String("topic", msg.Topic),
		zap.Uint8("priority", uint8(msg.Priority)),
		zap.String("client_id", msg.ClientID))

	// As last resort for critical messages, try to force enqueue by expanding buffer temporarily
	// This is an emergency measure and indicates system overload
	if msg.Priority == PriorityCritical {
		// In production, you might implement emergency buffer expansion here
		// For now, we'll try one more time with the fast path
		if q.config.FastPathEnabled {
			select {
			case q.criticalFast <- msg:
				q.logger.Warn("Used emergency fast path for critical message overflow")
				return true
			default:
				return false
			}
		}
	}

	return false
}

// enqueue adds a message to the ring buffer (lock-free)
func (rb *lockFreeRingBuffer) enqueue(msg *PriorityMessage) bool {
	writePos := atomic.LoadUint64(&rb.writePos)
	readPos := atomic.LoadUint64(&rb.readPos)

	// Check if buffer is full
	if writePos-readPos >= rb.capacity {
		return false
	}

	// Store message
	atomic.StorePointer(&rb.buffer[writePos&rb.mask], unsafe.Pointer(msg))

	// Advance write position
	atomic.StoreUint64(&rb.writePos, writePos+1)

	return true
}

// dequeue removes a message from the ring buffer (lock-free)
func (rb *lockFreeRingBuffer) dequeue() *PriorityMessage {
	readPos := atomic.LoadUint64(&rb.readPos)
	writePos := atomic.LoadUint64(&rb.writePos)

	// Check if buffer is empty
	if readPos >= writePos {
		return nil
	}

	// Load message
	msgPtr := atomic.LoadPointer(&rb.buffer[readPos&rb.mask])
	if msgPtr == nil {
		return nil
	}

	// Clear slot
	atomic.StorePointer(&rb.buffer[readPos&rb.mask], nil)

	// Advance read position
	atomic.StoreUint64(&rb.readPos, readPos+1)

	return (*PriorityMessage)(msgPtr)
}

// GetStats returns current queue statistics
func (q *LockFreePriorityQueue) GetStats() QueueStats {
	return QueueStats{
		MessagesEnqueued: [5]uint64{
			atomic.LoadUint64(&q.stats.MessagesEnqueued[0]),
			atomic.LoadUint64(&q.stats.MessagesEnqueued[1]),
			atomic.LoadUint64(&q.stats.MessagesEnqueued[2]),
			atomic.LoadUint64(&q.stats.MessagesEnqueued[3]),
			atomic.LoadUint64(&q.stats.MessagesEnqueued[4]),
		},
		MessagesDequeued: [5]uint64{
			atomic.LoadUint64(&q.stats.MessagesDequeued[0]),
			atomic.LoadUint64(&q.stats.MessagesDequeued[1]),
			atomic.LoadUint64(&q.stats.MessagesDequeued[2]),
			atomic.LoadUint64(&q.stats.MessagesDequeued[3]),
			atomic.LoadUint64(&q.stats.MessagesDequeued[4]),
		},
		MessagesDropped: [5]uint64{
			atomic.LoadUint64(&q.stats.MessagesDropped[0]),
			atomic.LoadUint64(&q.stats.MessagesDropped[1]),
			atomic.LoadUint64(&q.stats.MessagesDropped[2]),
			atomic.LoadUint64(&q.stats.MessagesDropped[3]),
			atomic.LoadUint64(&q.stats.MessagesDropped[4]),
		},
		FastPathUsed:    atomic.LoadUint64(&q.stats.FastPathUsed),
		TotalLatencyNs:  atomic.LoadUint64(&q.stats.TotalLatencyNs),
		MaxLatencyNs:    atomic.LoadUint64(&q.stats.MaxLatencyNs),
		QueueFullEvents: atomic.LoadUint64(&q.stats.QueueFullEvents),
		OverflowEvents:  atomic.LoadUint64(&q.stats.OverflowEvents),
	}
}

// IsEmpty returns true if all queues are empty
func (q *LockFreePriorityQueue) IsEmpty() bool {
	// Check fast path first
	if q.config.FastPathEnabled && len(q.criticalFast) > 0 {
		return false
	}

	// Check all priority queues
	for _, buffer := range q.queues {
		readPos := atomic.LoadUint64(&buffer.readPos)
		writePos := atomic.LoadUint64(&buffer.writePos)
		if readPos < writePos {
			return false
		}
	}

	return true
}

// Size returns the total number of messages across all queues
func (q *LockFreePriorityQueue) Size() uint64 {
	total := uint64(0)

	// Count fast path
	if q.config.FastPathEnabled {
		total += uint64(len(q.criticalFast))
	}

	// Count all priority queues
	for _, buffer := range q.queues {
		readPos := atomic.LoadUint64(&buffer.readPos)
		writePos := atomic.LoadUint64(&buffer.writePos)
		total += writePos - readPos
	}

	return total
}
