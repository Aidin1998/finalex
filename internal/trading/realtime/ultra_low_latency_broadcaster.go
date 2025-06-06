// =============================
// Ultra-Low Latency Real-Time Event Broadcasting System
// =============================
// Implements sub-10μs market data broadcasting with zero-copy message distribution
// and lock-free queue processing for maximum performance.

package realtime

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"go.uber.org/zap"

	ws "github.com/Aidin1998/finalex/internal/infrastructure/ws"
	model "github.com/Aidin1998/finalex/internal/trading/model"
	orderbook "github.com/Aidin1998/finalex/internal/trading/orderbook"
)

// =============================
// Lock-Free Event Queue for Ultra-Low Latency
// =============================

// EventType represents different types of real-time events
type EventType uint8

const (
	EventTrade EventType = iota
	EventOrderBookUpdate
	EventOrderUpdate
	EventAccountUpdate
	EventMarketData
)

// Event represents a zero-copy event with pre-allocated memory pools
type Event struct {
	Type      EventType
	Timestamp int64          // nanosecond precision for latency measurement
	Symbol    uint32         // interned symbol ID for faster processing
	Data      unsafe.Pointer // zero-copy data pointer
	Size      uint32
	Priority  uint8 // 0=critical, 1=high, 2=medium, 3=low
}

// LockFreeEventQueue implements a high-performance lock-free circular buffer
type LockFreeEventQueue struct {
	buffer    []Event
	mask      uint64
	writePos  uint64 // cache-line aligned
	_padding1 [56]byte
	readPos   uint64 // cache-line aligned
	_padding2 [56]byte
}

// NewLockFreeEventQueue creates a new lock-free event queue with power-of-2 size
func NewLockFreeEventQueue(size int) *LockFreeEventQueue {
	// Ensure size is power of 2 for efficient masking
	actualSize := 1
	for actualSize < size {
		actualSize <<= 1
	}

	return &LockFreeEventQueue{
		buffer: make([]Event, actualSize),
		mask:   uint64(actualSize - 1),
	}
}

// TryEnqueue attempts to enqueue an event without blocking (lock-free)
func (q *LockFreeEventQueue) TryEnqueue(event Event) bool {
	writePos := atomic.LoadUint64(&q.writePos)
	readPos := atomic.LoadUint64(&q.readPos)

	// Check if queue is full
	if writePos-readPos >= uint64(len(q.buffer)) {
		return false // Queue full
	}

	// Try to reserve the slot
	if !atomic.CompareAndSwapUint64(&q.writePos, writePos, writePos+1) {
		return false // Another thread got there first
	}

	// Write event to reserved slot
	q.buffer[writePos&q.mask] = event
	return true
}

// TryDequeue attempts to dequeue an event without blocking (lock-free)
func (q *LockFreeEventQueue) TryDequeue() (Event, bool) {
	readPos := atomic.LoadUint64(&q.readPos)
	writePos := atomic.LoadUint64(&q.writePos)

	// Check if queue is empty
	if readPos >= writePos {
		return Event{}, false
	}

	// Try to reserve the slot
	if !atomic.CompareAndSwapUint64(&q.readPos, readPos, readPos+1) {
		return Event{}, false
	}

	// Read event from reserved slot
	event := q.buffer[readPos&q.mask]
	return event, true
}

// GetStats returns queue statistics
func (q *LockFreeEventQueue) GetStats() (writePos, readPos, available, used uint64) {
	wp := atomic.LoadUint64(&q.writePos)
	rp := atomic.LoadUint64(&q.readPos)
	used = wp - rp
	available = uint64(len(q.buffer)) - used
	return wp, rp, available, used
}

// =============================
// Zero-Copy Message Pools
// =============================

// MessagePool manages zero-copy message buffers
type MessagePool struct {
	tradePool     sync.Pool
	orderBookPool sync.Pool
	orderPool     sync.Pool
	accountPool   sync.Pool
}

// NewMessagePool creates optimized message pools
func NewMessagePool() *MessagePool {
	return &MessagePool{
		tradePool: sync.Pool{
			New: func() interface{} {
				return &TradeMessage{
					data: make([]byte, 0, 512), // Pre-allocated capacity
				}
			},
		},
		orderBookPool: sync.Pool{
			New: func() interface{} {
				return &OrderBookMessage{
					data: make([]byte, 0, 2048), // Larger for order book updates
				}
			},
		},
		orderPool: sync.Pool{
			New: func() interface{} {
				return &OrderMessage{
					data: make([]byte, 0, 1024),
				}
			},
		},
		accountPool: sync.Pool{
			New: func() interface{} {
				return &AccountMessage{
					data: make([]byte, 0, 256),
				}
			},
		},
	}
}

// TradeMessage represents a zero-copy trade message
type TradeMessage struct {
	Symbol    uint32
	Price     uint64 // fixed-point
	Quantity  uint64 // fixed-point
	Side      uint8
	TradeID   uint64
	Timestamp int64
	data      []byte // pre-serialized data
}

// OrderBookMessage represents a zero-copy order book update
type OrderBookMessage struct {
	Symbol    uint32
	Bids      []PriceLevel
	Asks      []PriceLevel
	Timestamp int64
	data      []byte
}

// OrderMessage represents a zero-copy order update
type OrderMessage struct {
	UserID    uint64
	OrderID   uint64
	Symbol    uint32
	Status    uint8
	Timestamp int64
	data      []byte
}

// AccountMessage represents a zero-copy account update
type AccountMessage struct {
	UserID    uint64
	Currency  uint32
	Balance   uint64 // fixed-point
	Timestamp int64
	data      []byte
}

// PriceLevel represents a price level in the order book
type PriceLevel struct {
	Price    uint64 // fixed-point
	Quantity uint64 // fixed-point
}

// =============================
// Ultra-Low Latency Broadcaster
// =============================

// UltraLowLatencyBroadcaster manages real-time event distribution with <10μs latency
type UltraLowLatencyBroadcaster struct {
	// Core components
	eventQueue  *LockFreeEventQueue
	messagePool *MessagePool
	wsHub       *ws.Hub
	logger      *zap.Logger

	// Sharded processors for parallel event handling
	processors    []*EventProcessor
	processorMask uint32

	// Performance tracking
	perfCounters *BroadcastCounters

	// Lifecycle management
	ctx     context.Context
	cancel  context.CancelFunc
	running int32
	wg      sync.WaitGroup

	// Configuration
	config *BroadcasterConfig
}

// BroadcasterConfig holds configuration for the broadcaster
type BroadcasterConfig struct {
	QueueSize       int           `json:"queue_size"`
	ProcessorCount  int           `json:"processor_count"`
	BatchSize       int           `json:"batch_size"`
	FlushInterval   time.Duration `json:"flush_interval"`
	MaxLatency      time.Duration `json:"max_latency"`
	EnableProfiling bool          `json:"enable_profiling"`
	TargetLatencyNs int64         `json:"target_latency_ns"` // Target <10μs
}

// DefaultBroadcasterConfig returns production-optimized configuration
func DefaultBroadcasterConfig() *BroadcasterConfig {
	return &BroadcasterConfig{
		QueueSize:       65536,                  // Large queue for burst handling
		ProcessorCount:  runtime.NumCPU() * 2,   // 2x CPU cores for optimal throughput
		BatchSize:       16,                     // Small batches for low latency
		FlushInterval:   100 * time.Microsecond, // Very frequent flushing
		MaxLatency:      10 * time.Microsecond,  // <10μs target
		EnableProfiling: false,                  // Disable in production
		TargetLatencyNs: 10000,                  // 10μs in nanoseconds
	}
}

// EventProcessor handles events in a dedicated goroutine
type EventProcessor struct {
	id          uint32
	broadcaster *UltraLowLatencyBroadcaster
	batchBuffer []Event
	lastFlush   time.Time
}

// BroadcastCounters tracks performance metrics atomically
type BroadcastCounters struct {
	eventsProcessed   uint64
	messagesGenerated uint64
	messagesSent      uint64
	messagesDropped   uint64
	totalLatencyNs    uint64
	maxLatencyNs      uint64
	minLatencyNs      uint64
	queueOverflows    uint64
	processingErrors  uint64
}

// NewUltraLowLatencyBroadcaster creates a new high-performance broadcaster
func NewUltraLowLatencyBroadcaster(
	wsHub *ws.Hub,
	logger *zap.Logger,
	config *BroadcasterConfig,
) *UltraLowLatencyBroadcaster {
	if config == nil {
		config = DefaultBroadcasterConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Ensure processor count is power of 2 for efficient masking
	processorCount := 1
	for processorCount < config.ProcessorCount {
		processorCount <<= 1
	}

	broadcaster := &UltraLowLatencyBroadcaster{
		eventQueue:    NewLockFreeEventQueue(config.QueueSize),
		messagePool:   NewMessagePool(),
		wsHub:         wsHub,
		logger:        logger,
		processorMask: uint32(processorCount - 1),
		perfCounters:  &BroadcastCounters{minLatencyNs: ^uint64(0)}, // Initialize to max value
		ctx:           ctx,
		cancel:        cancel,
		config:        config,
	}

	// Initialize processors
	broadcaster.processors = make([]*EventProcessor, processorCount)
	for i := uint32(0); i < uint32(processorCount); i++ {
		broadcaster.processors[i] = &EventProcessor{
			id:          i,
			broadcaster: broadcaster,
			batchBuffer: make([]Event, 0, config.BatchSize),
		}
	}

	return broadcaster
}

// Start begins the broadcaster and all event processors
func (b *UltraLowLatencyBroadcaster) Start() error {
	if !atomic.CompareAndSwapInt32(&b.running, 0, 1) {
		return nil // Already running
	}

	b.logger.Info("Starting UltraLowLatencyBroadcaster",
		zap.Int("queue_size", b.config.QueueSize),
		zap.Int("processor_count", len(b.processors)),
		zap.Duration("target_latency", b.config.MaxLatency))

	// Start event processors
	for _, processor := range b.processors {
		b.wg.Add(1)
		go b.runEventProcessor(processor)
	}

	// Start performance monitoring
	if b.config.EnableProfiling {
		b.wg.Add(1)
		go b.performanceMonitor()
	}

	b.logger.Info("UltraLowLatencyBroadcaster started successfully")
	return nil
}

// =============================
// Event Broadcasting Interface
// =============================

// BroadcastTrade broadcasts a trade event with ultra-low latency
func (b *UltraLowLatencyBroadcaster) BroadcastTrade(trade *model.Trade) {
	startTime := time.Now().UnixNano()

	// Get message from pool
	tradeMsg := b.messagePool.tradePool.Get().(*TradeMessage)
	defer b.messagePool.tradePool.Put(tradeMsg)

	// Convert to zero-copy format
	b.fillTradeMessage(tradeMsg, trade, startTime)

	// Create event
	event := Event{
		Type:      EventTrade,
		Timestamp: startTime,
		Symbol:    tradeMsg.Symbol,
		Data:      unsafe.Pointer(tradeMsg),
		Size:      uint32(len(tradeMsg.data)),
		Priority:  0, // Critical priority for trades
	}

	// Enqueue for processing
	if !b.eventQueue.TryEnqueue(event) {
		atomic.AddUint64(&b.perfCounters.queueOverflows, 1)
		atomic.AddUint64(&b.perfCounters.messagesDropped, 1)
		b.logger.Warn("Trade event queue overflow - message dropped",
			zap.String("trade_id", trade.ID.String()),
			zap.String("symbol", trade.Pair))
	}
}

// BroadcastOrderBookUpdate broadcasts order book changes with ultra-low latency
func (b *UltraLowLatencyBroadcaster) BroadcastOrderBookUpdate(
	symbol string,
	bids, asks []orderbook.PriceLevel,
) {
	startTime := time.Now().UnixNano()

	// Get message from pool
	obMsg := b.messagePool.orderBookPool.Get().(*OrderBookMessage)
	defer b.messagePool.orderBookPool.Put(obMsg)

	// Convert to zero-copy format
	b.fillOrderBookMessage(obMsg, symbol, bids, asks, startTime)

	// Create event
	event := Event{
		Type:      EventOrderBookUpdate,
		Timestamp: startTime,
		Symbol:    obMsg.Symbol,
		Data:      unsafe.Pointer(obMsg),
		Size:      uint32(len(obMsg.data)),
		Priority:  1, // High priority for order book updates
	}

	// Enqueue for processing
	if !b.eventQueue.TryEnqueue(event) {
		atomic.AddUint64(&b.perfCounters.queueOverflows, 1)
		atomic.AddUint64(&b.perfCounters.messagesDropped, 1)
	}
}

// BroadcastOrderUpdate broadcasts order status changes to specific users
func (b *UltraLowLatencyBroadcaster) BroadcastOrderUpdate(order *model.Order) {
	startTime := time.Now().UnixNano()

	// Get message from pool
	orderMsg := b.messagePool.orderPool.Get().(*OrderMessage)
	defer b.messagePool.orderPool.Put(orderMsg)

	// Convert to zero-copy format
	b.fillOrderMessage(orderMsg, order, startTime)

	// Create event
	event := Event{
		Type:      EventOrderUpdate,
		Timestamp: startTime,
		Symbol:    orderMsg.Symbol,
		Data:      unsafe.Pointer(orderMsg),
		Size:      uint32(len(orderMsg.data)),
		Priority:  1, // High priority for user order updates
	}

	// Enqueue for processing
	if !b.eventQueue.TryEnqueue(event) {
		atomic.AddUint64(&b.perfCounters.queueOverflows, 1)
		atomic.AddUint64(&b.perfCounters.messagesDropped, 1)
	}
}

// BroadcastAccountUpdate broadcasts account balance changes to specific users
func (b *UltraLowLatencyBroadcaster) BroadcastAccountUpdate(
	userID string,
	currency string,
	balance float64,
) {
	startTime := time.Now().UnixNano()

	// Get message from pool
	accountMsg := b.messagePool.accountPool.Get().(*AccountMessage)
	defer b.messagePool.accountPool.Put(accountMsg)

	// Convert to zero-copy format
	b.fillAccountMessage(accountMsg, userID, currency, balance, startTime)

	// Create event
	event := Event{
		Type:      EventAccountUpdate,
		Timestamp: startTime,
		Symbol:    0, // No symbol for account updates
		Data:      unsafe.Pointer(accountMsg),
		Size:      uint32(len(accountMsg.data)),
		Priority:  2, // Medium priority for account updates
	}

	// Enqueue for processing
	if !b.eventQueue.TryEnqueue(event) {
		atomic.AddUint64(&b.perfCounters.queueOverflows, 1)
		atomic.AddUint64(&b.perfCounters.messagesDropped, 1)
	}
}

// =============================
// Message Conversion Helpers (Zero-Copy)
// =============================

// fillTradeMessage converts model.Trade to zero-copy TradeMessage
func (b *UltraLowLatencyBroadcaster) fillTradeMessage(
	msg *TradeMessage,
	trade *model.Trade,
	timestamp int64,
) {
	// Use symbol interning for fast symbol lookup
	msg.Symbol = orderbook.GetGlobalSymbolInterning().RegisterSymbol(trade.Pair)
	msg.Price = uint64(trade.Price.Mul(orderbook.FixedPointScale).IntPart())
	msg.Quantity = uint64(trade.Quantity.Mul(orderbook.FixedPointScale).IntPart())
	msg.Side = orderbook.SideStringToUint8(trade.Side)
	msg.TradeID = orderbook.UuidToUint64(trade.ID)
	msg.Timestamp = timestamp

	// Pre-serialize for zero-copy transmission
	msg.data = msg.data[:0] // Reset slice but keep capacity
	msg.data = b.serializeTradeMessage(msg.data, msg)
}

// fillOrderBookMessage converts order book data to zero-copy OrderBookMessage
func (b *UltraLowLatencyBroadcaster) fillOrderBookMessage(
	msg *OrderBookMessage,
	symbol string,
	bids, asks []orderbook.PriceLevel,
	timestamp int64,
) {
	// Use symbol interning
	msg.Symbol = orderbook.GetGlobalSymbolInterning().RegisterSymbol(symbol)
	msg.Timestamp = timestamp

	// Convert price levels efficiently
	msg.Bids = msg.Bids[:0] // Reset but keep capacity
	msg.Asks = msg.Asks[:0]

	for _, bid := range bids {
		if len(msg.Bids) < cap(msg.Bids) {
			msg.Bids = append(msg.Bids, PriceLevel{
				Price:    uint64(bid.Price.Mul(orderbook.FixedPointScale).IntPart()),
				Quantity: uint64(bid.Quantity.Mul(orderbook.FixedPointScale).IntPart()),
			})
		}
	}

	for _, ask := range asks {
		if len(msg.Asks) < cap(msg.Asks) {
			msg.Asks = append(msg.Asks, PriceLevel{
				Price:    uint64(ask.Price.Mul(orderbook.FixedPointScale).IntPart()),
				Quantity: uint64(ask.Quantity.Mul(orderbook.FixedPointScale).IntPart()),
			})
		}
	}

	// Pre-serialize for zero-copy transmission
	msg.data = msg.data[:0]
	msg.data = b.serializeOrderBookMessage(msg.data, msg)
}

// fillOrderMessage converts model.Order to zero-copy OrderMessage
func (b *UltraLowLatencyBroadcaster) fillOrderMessage(
	msg *OrderMessage,
	order *model.Order,
	timestamp int64,
) {
	msg.UserID = orderbook.UuidToUint64(order.UserID)
	msg.OrderID = orderbook.UuidToUint64(order.ID)
	msg.Symbol = orderbook.GetGlobalSymbolInterning().RegisterSymbol(order.Pair)
	msg.Status = orderbook.StatusStringToUint8(order.Status)
	msg.Timestamp = timestamp

	// Pre-serialize for zero-copy transmission
	msg.data = msg.data[:0]
	msg.data = b.serializeOrderMessage(msg.data, msg)
}

// fillAccountMessage converts account data to zero-copy AccountMessage
func (b *UltraLowLatencyBroadcaster) fillAccountMessage(
	msg *AccountMessage,
	userID, currency string,
	balance float64,
	timestamp int64,
) {
	msg.UserID = orderbook.StringToUint64(userID)
	msg.Currency = orderbook.GetGlobalSymbolInterning().RegisterSymbol(currency)
	msg.Balance = uint64(balance * float64(orderbook.FixedPointScale.IntPart()))
	msg.Timestamp = timestamp

	// Pre-serialize for zero-copy transmission
	msg.data = msg.data[:0]
	msg.data = b.serializeAccountMessage(msg.data, msg)
}

// =============================
// High-Performance Serialization (Inline)
// =============================

// serializeTradeMessage serializes trade message directly to byte slice
func (b *UltraLowLatencyBroadcaster) serializeTradeMessage(
	buf []byte,
	msg *TradeMessage,
) []byte {
	// Fast JSON serialization without reflection
	buf = append(buf, `{"type":"trade","symbol":"`...)
	buf = append(buf, orderbook.GetGlobalSymbolInterning().GetSymbol(msg.Symbol)...)
	buf = append(buf, `","price":`...)
	buf = appendUint64(buf, msg.Price)
	buf = append(buf, `,"quantity":`...)
	buf = appendUint64(buf, msg.Quantity)
	buf = append(buf, `,"side":`...)
	buf = appendUint8(buf, msg.Side)
	buf = append(buf, `,"trade_id":`...)
	buf = appendUint64(buf, msg.TradeID)
	buf = append(buf, `,"timestamp":`...)
	buf = appendInt64(buf, msg.Timestamp)
	buf = append(buf, '}')
	return buf
}

// serializeOrderBookMessage serializes order book message directly to byte slice
func (b *UltraLowLatencyBroadcaster) serializeOrderBookMessage(
	buf []byte,
	msg *OrderBookMessage,
) []byte {
	buf = append(buf, `{"type":"orderbook","symbol":"`...)
	buf = append(buf, orderbook.GetGlobalSymbolInterning().GetSymbol(msg.Symbol)...)
	buf = append(buf, `","bids":[`...)

	for i, bid := range msg.Bids {
		if i > 0 {
			buf = append(buf, ',')
		}
		buf = append(buf, `[`...)
		buf = appendUint64(buf, bid.Price)
		buf = append(buf, ',')
		buf = appendUint64(buf, bid.Quantity)
		buf = append(buf, ']')
	}

	buf = append(buf, `],"asks":[`...)

	for i, ask := range msg.Asks {
		if i > 0 {
			buf = append(buf, ',')
		}
		buf = append(buf, `[`...)
		buf = appendUint64(buf, ask.Price)
		buf = append(buf, ',')
		buf = appendUint64(buf, ask.Quantity)
		buf = append(buf, ']')
	}

	buf = append(buf, `],"timestamp":`...)
	buf = appendInt64(buf, msg.Timestamp)
	buf = append(buf, '}')
	return buf
}

// serializeOrderMessage serializes order message directly to byte slice
func (b *UltraLowLatencyBroadcaster) serializeOrderMessage(
	buf []byte,
	msg *OrderMessage,
) []byte {
	buf = append(buf, `{"type":"order","user_id":`...)
	buf = appendUint64(buf, msg.UserID)
	buf = append(buf, `,"order_id":`...)
	buf = appendUint64(buf, msg.OrderID)
	buf = append(buf, `,"symbol":"`...)
	buf = append(buf, orderbook.GetGlobalSymbolInterning().GetSymbol(msg.Symbol)...)
	buf = append(buf, `","status":`...)
	buf = appendUint8(buf, msg.Status)
	buf = append(buf, `,"timestamp":`...)
	buf = appendInt64(buf, msg.Timestamp)
	buf = append(buf, '}')
	return buf
}

// serializeAccountMessage serializes account message directly to byte slice
func (b *UltraLowLatencyBroadcaster) serializeAccountMessage(
	buf []byte,
	msg *AccountMessage,
) []byte {
	buf = append(buf, `{"type":"account","user_id":`...)
	buf = appendUint64(buf, msg.UserID)
	buf = append(buf, `,"currency":"`...)
	buf = append(buf, orderbook.GetGlobalSymbolInterning().GetSymbol(msg.Currency)...)
	buf = append(buf, `","balance":`...)
	buf = appendUint64(buf, msg.Balance)
	buf = append(buf, `,"timestamp":`...)
	buf = appendInt64(buf, msg.Timestamp)
	buf = append(buf, '}')
	return buf
}

// Fast integer to string conversion helpers
func appendUint64(buf []byte, val uint64) []byte {
	if val == 0 {
		return append(buf, '0')
	}

	// Convert without allocations
	tmp := [20]byte{} // Max digits for uint64
	i := len(tmp) - 1

	for val > 0 {
		tmp[i] = byte('0' + val%10)
		val /= 10
		i--
	}

	return append(buf, tmp[i+1:]...)
}

func appendInt64(buf []byte, val int64) []byte {
	if val < 0 {
		buf = append(buf, '-')
		val = -val
	}
	return appendUint64(buf, uint64(val))
}

func appendUint8(buf []byte, val uint8) []byte {
	return appendUint64(buf, uint64(val))
}
