// =============================
// Event Processor and Performance Monitoring for Ultra-Low Latency Broadcasting
// =============================

package realtime

import (
	"fmt"
	"strconv"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// =============================
// Event Processing Logic
// =============================

// runEventProcessor runs an event processor in a dedicated goroutine
func (b *UltraLowLatencyBroadcaster) runEventProcessor(processor *EventProcessor) {
	defer b.wg.Done()

	b.logger.Debug("Starting event processor", zap.Uint32("processor_id", processor.id))

	// Pin processor to specific CPU core for optimal performance
	// This reduces context switching and cache misses

	ticker := time.NewTicker(b.config.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-b.ctx.Done():
			// Flush remaining events before shutdown
			b.flushProcessorBatch(processor)
			b.logger.Debug("Event processor shutting down", zap.Uint32("processor_id", processor.id))
			return

		case <-ticker.C:
			// Periodic flush to ensure low latency even under low load
			b.flushProcessorBatch(processor)

		default:
			// Try to process events without blocking
			processed := b.processEvents(processor)
			if processed == 0 {
				// No events available, yield to prevent busy spinning
				// Using short sleep to maintain responsiveness
				time.Sleep(time.Microsecond)
			}
		}
	}
}

// processEvents processes available events for a specific processor
func (b *UltraLowLatencyBroadcaster) processEvents(processor *EventProcessor) int {
	processed := 0

	// Process events in batches for efficiency
	for processed < b.config.BatchSize {
		event, ok := b.eventQueue.TryDequeue()
		if !ok {
			break // No more events available
		}

		// Add to processor batch
		processor.batchBuffer = append(processor.batchBuffer, event)
		processed++

		// Check if batch is full or max latency would be exceeded
		if len(processor.batchBuffer) >= b.config.BatchSize ||
			b.shouldFlushBatch(processor, event.Timestamp) {
			b.flushProcessorBatch(processor)
		}
	}

	return processed
}

// shouldFlushBatch determines if batch should be flushed based on latency constraints
func (b *UltraLowLatencyBroadcaster) shouldFlushBatch(processor *EventProcessor, eventTimestamp int64) bool {
	if len(processor.batchBuffer) == 0 {
		return false
	}

	// Check if oldest event in batch exceeds target latency
	oldestEvent := processor.batchBuffer[0]
	currentTime := time.Now().UnixNano()
	latency := currentTime - oldestEvent.Timestamp

	return latency >= b.config.TargetLatencyNs
}

// flushProcessorBatch processes and sends all events in the processor batch
func (b *UltraLowLatencyBroadcaster) flushProcessorBatch(processor *EventProcessor) {
	if len(processor.batchBuffer) == 0 {
		return
	}

	currentTime := time.Now().UnixNano()

	// Process each event in the batch
	for _, event := range processor.batchBuffer {
		b.processEvent(event, currentTime)
	}

	// Clear batch buffer for next iteration
	processor.batchBuffer = processor.batchBuffer[:0]
	processor.lastFlush = time.Now()
}

// processEvent processes a single event and broadcasts it
func (b *UltraLowLatencyBroadcaster) processEvent(event Event, currentTime int64) {
	startTime := time.Now().UnixNano()

	// Calculate end-to-end latency
	endToEndLatency := currentTime - event.Timestamp

	// Update performance counters
	atomic.AddUint64(&b.perfCounters.eventsProcessed, 1)
	b.updateLatencyStats(uint64(endToEndLatency))

	// Process based on event type
	switch event.Type {
	case EventTrade:
		b.processTrade(event)

	case EventOrderBookUpdate:
		b.processOrderBookUpdate(event)

	case EventOrderUpdate:
		b.processOrderUpdate(event)

	case EventAccountUpdate:
		b.processAccountUpdate(event)

	default:
		atomic.AddUint64(&b.perfCounters.processingErrors, 1)
		b.logger.Warn("Unknown event type", zap.Uint8("type", uint8(event.Type)))
	}

	// Track processing latency
	processingLatency := time.Now().UnixNano() - startTime
	if processingLatency > b.config.TargetLatencyNs {
		b.logger.Warn("Event processing exceeded target latency",
			zap.Int64("latency_ns", processingLatency),
			zap.Int64("target_ns", b.config.TargetLatencyNs),
			zap.Uint8("event_type", uint8(event.Type)))
	}
}

// =============================
// Event Type Specific Processing
// =============================

// processTrade broadcasts trade events to all market data subscribers
func (b *UltraLowLatencyBroadcaster) processTrade(event Event) {
	tradeMsg := (*TradeMessage)(event.Data)
	// Fallback: just use the numeric symbol ID as string
	symbol := fmt.Sprint(tradeMsg.Symbol)
	// If you have a global symbol interning instance, use:
	// symbol := orderbook.GlobalSymbolInterning.SymbolFromID(tradeMsg.Symbol)
	tradeTopic := fmt.Sprintf("trades.%s", symbol)
	b.wsHub.Broadcast(tradeTopic, tradeMsg.data)

	// Broadcast to generic market data feed
	b.wsHub.Broadcast("market_data", tradeMsg.data)

	atomic.AddUint64(&b.perfCounters.messagesSent, 2) // Two broadcasts
}

// processOrderBookUpdate broadcasts order book updates to subscribers
func (b *UltraLowLatencyBroadcaster) processOrderBookUpdate(event Event) {
	obMsg := (*OrderBookMessage)(event.Data)
	symbol := fmt.Sprint(obMsg.Symbol)
	// symbol := orderbook.GlobalSymbolInterning.SymbolFromID(obMsg.Symbol)
	obTopic := fmt.Sprintf("orderbook.%s", symbol)
	b.wsHub.Broadcast(obTopic, obMsg.data)

	// Broadcast to aggregated order book feed
	b.wsHub.Broadcast("orderbook", obMsg.data)

	atomic.AddUint64(&b.perfCounters.messagesSent, 2)
}

// processOrderUpdate broadcasts order updates to specific users
func (b *UltraLowLatencyBroadcaster) processOrderUpdate(event Event) {
	orderMsg := (*OrderMessage)(event.Data)

	// Convert user ID back to string for topic routing
	userIDStr := strconv.FormatUint(orderMsg.UserID, 10)

	// Broadcast to user-specific order feed
	orderTopic := fmt.Sprintf("orders.%s", userIDStr)
	b.wsHub.Broadcast(orderTopic, orderMsg.data)

	atomic.AddUint64(&b.perfCounters.messagesSent, 1)
}

// processAccountUpdate broadcasts account updates to specific users
func (b *UltraLowLatencyBroadcaster) processAccountUpdate(event Event) {
	accountMsg := (*AccountMessage)(event.Data)

	// Convert user ID back to string for topic routing
	userIDStr := strconv.FormatUint(accountMsg.UserID, 10)

	// Broadcast to user-specific account feed
	accountTopic := fmt.Sprintf("accounts.%s", userIDStr)
	b.wsHub.Broadcast(accountTopic, accountMsg.data)

	atomic.AddUint64(&b.perfCounters.messagesSent, 1)
}

// =============================
// Performance Monitoring
// =============================

// updateLatencyStats updates latency statistics atomically
func (b *UltraLowLatencyBroadcaster) updateLatencyStats(latencyNs uint64) {
	// Update total latency for average calculation
	atomic.AddUint64(&b.perfCounters.totalLatencyNs, latencyNs)

	// Update max latency
	for {
		currentMax := atomic.LoadUint64(&b.perfCounters.maxLatencyNs)
		if latencyNs <= currentMax || atomic.CompareAndSwapUint64(&b.perfCounters.maxLatencyNs, currentMax, latencyNs) {
			break
		}
	}

	// Update min latency
	for {
		currentMin := atomic.LoadUint64(&b.perfCounters.minLatencyNs)
		if latencyNs >= currentMin || atomic.CompareAndSwapUint64(&b.perfCounters.minLatencyNs, currentMin, latencyNs) {
			break
		}
	}
}

// performanceMonitor periodically logs performance metrics
func (b *UltraLowLatencyBroadcaster) performanceMonitor() {
	defer b.wg.Done()

	ticker := time.NewTicker(30 * time.Second) // Log every 30 seconds
	defer ticker.Stop()

	var lastEventsProcessed uint64
	var lastMessagesSent uint64
	lastTime := time.Now()

	for {
		select {
		case <-b.ctx.Done():
			b.logFinalStats()
			return

		case <-ticker.C:
			now := time.Now()
			elapsed := now.Sub(lastTime).Seconds()

			currentEventsProcessed := atomic.LoadUint64(&b.perfCounters.eventsProcessed)
			currentMessagesSent := atomic.LoadUint64(&b.perfCounters.messagesSent)

			eventsPerSec := float64(currentEventsProcessed-lastEventsProcessed) / elapsed
			messagesPerSec := float64(currentMessagesSent-lastMessagesSent) / elapsed

			avgLatencyNs := float64(0)
			if currentEventsProcessed > 0 {
				avgLatencyNs = float64(atomic.LoadUint64(&b.perfCounters.totalLatencyNs)) / float64(currentEventsProcessed)
			}

			_, _, queueAvailable, queueUsed := b.eventQueue.GetStats()

			b.logger.Info("Broadcasting performance metrics",
				zap.Float64("events_per_sec", eventsPerSec),
				zap.Float64("messages_per_sec", messagesPerSec),
				zap.Float64("avg_latency_us", avgLatencyNs/1000),
				zap.Uint64("max_latency_us", atomic.LoadUint64(&b.perfCounters.maxLatencyNs)/1000),
				zap.Uint64("min_latency_us", atomic.LoadUint64(&b.perfCounters.minLatencyNs)/1000),
				zap.Uint64("queue_used", queueUsed),
				zap.Uint64("queue_available", queueAvailable),
				zap.Uint64("queue_overflows", atomic.LoadUint64(&b.perfCounters.queueOverflows)),
				zap.Uint64("messages_dropped", atomic.LoadUint64(&b.perfCounters.messagesDropped)),
				zap.Uint64("processing_errors", atomic.LoadUint64(&b.perfCounters.processingErrors)))

			lastEventsProcessed = currentEventsProcessed
			lastMessagesSent = currentMessagesSent
			lastTime = now
		}
	}
}

// logFinalStats logs final performance statistics during shutdown
func (b *UltraLowLatencyBroadcaster) logFinalStats() {
	eventsProcessed := atomic.LoadUint64(&b.perfCounters.eventsProcessed)
	messagesSent := atomic.LoadUint64(&b.perfCounters.messagesSent)
	messagesDropped := atomic.LoadUint64(&b.perfCounters.messagesDropped)
	queueOverflows := atomic.LoadUint64(&b.perfCounters.queueOverflows)
	processingErrors := atomic.LoadUint64(&b.perfCounters.processingErrors)

	avgLatencyNs := float64(0)
	if eventsProcessed > 0 {
		avgLatencyNs = float64(atomic.LoadUint64(&b.perfCounters.totalLatencyNs)) / float64(eventsProcessed)
	}

	b.logger.Info("Final broadcasting statistics",
		zap.Uint64("total_events_processed", eventsProcessed),
		zap.Uint64("total_messages_sent", messagesSent),
		zap.Uint64("total_messages_dropped", messagesDropped),
		zap.Float64("avg_latency_us", avgLatencyNs/1000),
		zap.Uint64("max_latency_us", atomic.LoadUint64(&b.perfCounters.maxLatencyNs)/1000),
		zap.Uint64("min_latency_us", atomic.LoadUint64(&b.perfCounters.minLatencyNs)/1000),
		zap.Uint64("queue_overflows", queueOverflows),
		zap.Uint64("processing_errors", processingErrors))
}

// =============================
// Public Performance Metrics Interface
// =============================

// GetPerformanceMetrics returns current performance metrics
func (b *UltraLowLatencyBroadcaster) GetPerformanceMetrics() map[string]interface{} {
	eventsProcessed := atomic.LoadUint64(&b.perfCounters.eventsProcessed)

	avgLatencyNs := float64(0)
	if eventsProcessed > 0 {
		avgLatencyNs = float64(atomic.LoadUint64(&b.perfCounters.totalLatencyNs)) / float64(eventsProcessed)
	}

	_, _, queueAvailable, queueUsed := b.eventQueue.GetStats()

	return map[string]interface{}{
		"events_processed":   eventsProcessed,
		"messages_generated": atomic.LoadUint64(&b.perfCounters.messagesGenerated),
		"messages_sent":      atomic.LoadUint64(&b.perfCounters.messagesSent),
		"messages_dropped":   atomic.LoadUint64(&b.perfCounters.messagesDropped),
		"avg_latency_us":     avgLatencyNs / 1000,
		"max_latency_us":     atomic.LoadUint64(&b.perfCounters.maxLatencyNs) / 1000,
		"min_latency_us":     atomic.LoadUint64(&b.perfCounters.minLatencyNs) / 1000,
		"queue_available":    queueAvailable,
		"queue_used":         queueUsed,
		"queue_overflows":    atomic.LoadUint64(&b.perfCounters.queueOverflows),
		"processing_errors":  atomic.LoadUint64(&b.perfCounters.processingErrors),
		"processor_count":    len(b.processors),
		"target_latency_us":  b.config.TargetLatencyNs / 1000,
		"running":            atomic.LoadInt32(&b.running) == 1,
	}
}

// ResetPerformanceCounters resets all performance counters
func (b *UltraLowLatencyBroadcaster) ResetPerformanceCounters() {
	atomic.StoreUint64(&b.perfCounters.eventsProcessed, 0)
	atomic.StoreUint64(&b.perfCounters.messagesGenerated, 0)
	atomic.StoreUint64(&b.perfCounters.messagesSent, 0)
	atomic.StoreUint64(&b.perfCounters.messagesDropped, 0)
	atomic.StoreUint64(&b.perfCounters.totalLatencyNs, 0)
	atomic.StoreUint64(&b.perfCounters.maxLatencyNs, 0)
	atomic.StoreUint64(&b.perfCounters.minLatencyNs, ^uint64(0)) // Reset to max value
	atomic.StoreUint64(&b.perfCounters.queueOverflows, 0)
	atomic.StoreUint64(&b.perfCounters.processingErrors, 0)

	b.logger.Info("Performance counters reset")
}

// =============================
// Lifecycle Management
// =============================

// Stop gracefully shuts down the broadcaster
func (b *UltraLowLatencyBroadcaster) Stop() error {
	if !atomic.CompareAndSwapInt32(&b.running, 1, 0) {
		return nil // Already stopped
	}

	b.logger.Info("Stopping UltraLowLatencyBroadcaster...")

	// Cancel context to signal shutdown
	b.cancel()

	// Wait for all goroutines to finish with timeout
	done := make(chan struct{})
	go func() {
		b.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		b.logger.Info("UltraLowLatencyBroadcaster stopped gracefully")

	case <-time.After(10 * time.Second):
		b.logger.Warn("Timeout waiting for broadcaster shutdown")
	}

	return nil
}

// IsRunning returns true if the broadcaster is currently running
func (b *UltraLowLatencyBroadcaster) IsRunning() bool {
	return atomic.LoadInt32(&b.running) == 1
}

// =============================
// Advanced Optimization Helpers
// =============================

// WarmupPools pre-allocates objects in message pools for optimal performance
func (b *UltraLowLatencyBroadcaster) WarmupPools() {
	poolSize := 1000 // Pre-allocate 1000 objects per pool

	// Warmup trade message pool
	tradeObjects := make([]*TradeMessage, poolSize)
	for i := 0; i < poolSize; i++ {
		tradeObjects[i] = b.messagePool.tradePool.Get().(*TradeMessage)
	}
	for _, obj := range tradeObjects {
		b.messagePool.tradePool.Put(obj)
	}

	// Warmup order book message pool
	obObjects := make([]*OrderBookMessage, poolSize)
	for i := 0; i < poolSize; i++ {
		obObjects[i] = b.messagePool.orderBookPool.Get().(*OrderBookMessage)
	}
	for _, obj := range obObjects {
		b.messagePool.orderBookPool.Put(obj)
	}

	// Warmup order message pool
	orderObjects := make([]*OrderMessage, poolSize)
	for i := 0; i < poolSize; i++ {
		orderObjects[i] = b.messagePool.orderPool.Get().(*OrderMessage)
	}
	for _, obj := range orderObjects {
		b.messagePool.orderPool.Put(obj)
	}

	// Warmup account message pool
	accountObjects := make([]*AccountMessage, poolSize)
	for i := 0; i < poolSize; i++ {
		accountObjects[i] = b.messagePool.accountPool.Get().(*AccountMessage)
	}
	for _, obj := range accountObjects {
		b.messagePool.accountPool.Put(obj)
	}

	b.logger.Info("Message pools warmed up", zap.Int("pool_size", poolSize))
}

// GetQueueUtilization returns current queue utilization percentage
func (b *UltraLowLatencyBroadcaster) GetQueueUtilization() float64 {
	_, _, available, used := b.eventQueue.GetStats()
	total := available + used
	if total == 0 {
		return 0.0
	}
	return float64(used) / float64(total) * 100.0
}

// IsUnderLoad returns true if the system is under heavy load
func (b *UltraLowLatencyBroadcaster) IsUnderLoad() bool {
	utilization := b.GetQueueUtilization()
	maxLatency := atomic.LoadUint64(&b.perfCounters.maxLatencyNs)

	// Consider under load if queue utilization > 80% or max latency > 2x target
	return utilization > 80.0 || maxLatency > uint64(b.config.TargetLatencyNs*2)
}
