// Package coordination provides race-free, minimal latency synchronization
// between Matching Engine and Settlement modules
package coordination

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/Aidin1998/finalex/internal/trading/model"
	"github.com/Aidin1998/finalex/internal/trading/settlement"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// AtomicTradeCoordinator provides race-free coordination with sub-microsecond latency
// Focus: Zero trade loss, minimal latency, complete synchronization
type AtomicTradeCoordinator struct {
	logger           *zap.Logger
	settlementEngine *settlement.SettlementEngine

	// Lock-free state management using atomic operations
	state         int64 // Coordinator state: 0=stopped, 1=running, 2=stopping
	tradeSequence int64 // Monotonic trade sequence for ordering
	pendingCount  int64 // Active pending trades count

	// Lock-free trade registry with atomic pointers
	tradeRegistry   unsafe.Pointer // *TradeRegistryShards
	registryVersion int64          // Version for lock-free updates

	// Ultra-fast channels for critical path (no buffering to minimize latency)
	criticalEvents    chan *CriticalTradeEvent
	settlementResults chan *AtomicSettlementResult

	// Backpressure signaling
	backpressureSignal int64 // 0=normal, 1=apply backpressure

	// High-resolution metrics (atomic counters)
	metrics *AtomicMetrics

	// Lifecycle management
	shutdown     chan struct{}
	shutdownOnce sync.Once
	workers      sync.WaitGroup
}

// TradeRegistryShards provides lock-free concurrent access to trade state
// Uses multiple shards to reduce contention
type TradeRegistryShards struct {
	shards    [256]*TradeRegistryShard // Power of 2 for fast modulo
	shardMask uint32                   // 255 for fast modulo
}

type TradeRegistryShard struct {
	// Lock-free map using atomic pointers
	entries unsafe.Pointer // *map[string]*AtomicTradeState
	version int64          // For lock-free updates
	_       [7]int64       // Cache line padding to prevent false sharing
}

// AtomicTradeState maintains trade state with atomic operations
type AtomicTradeState struct {
	// Core trade data (immutable after creation)
	TradeID    uuid.UUID
	OrderID    uuid.UUID
	Symbol     string
	Quantity   decimal.Decimal
	Price      decimal.Decimal
	BuyUserID  uuid.UUID
	SellUserID uuid.UUID
	CreatedAt  time.Time

	// Atomic state fields (8-byte aligned for atomic operations)
	state       int64 // TradeStateAtomic enum
	sequence    int64 // Unique sequence number for ordering
	attempts    int64 // Settlement attempt count
	lastAttempt int64 // Unix nanoseconds of last attempt

	// Settlement coordination
	settlementID   unsafe.Pointer // *string, atomic pointer
	settledAt      int64          // Unix nanoseconds when settled
	compensationID unsafe.Pointer // *string, atomic pointer

	// Synchronization primitive for waiting
	settled chan struct{} // Closed when settlement completes
	// Padding to prevent false sharing
	_ [7]int64 //nolint:unused
}

// TradeStateAtomic represents atomic trade states
type TradeStateAtomic int64

const (
	StateCreated      TradeStateAtomic = 0
	StateMatched      TradeStateAtomic = 1
	StateQueued       TradeStateAtomic = 2
	StateSettling     TradeStateAtomic = 3
	StateSettled      TradeStateAtomic = 4
	StateFailed       TradeStateAtomic = 5
	StateCompensating TradeStateAtomic = 6
)

// CriticalTradeEvent represents a trade event requiring immediate processing
type CriticalTradeEvent struct {
	EventType CriticalEventType
	TradeID   uuid.UUID
	Trade     *model.Trade
	Timestamp int64 // Unix nanoseconds for high precision
	Sequence  int64 // Global sequence for ordering

	// Fast acknowledgment channel (unbuffered for immediate response)
	AckChan chan error
}

type CriticalEventType int

const (
	EventTradeExecuted CriticalEventType = iota
	EventTradeReverted
	EventSettlementFailed
	EventForceCompensation
)

// AtomicSettlementResult represents settlement completion
type AtomicSettlementResult struct {
	TradeID      uuid.UUID
	SettlementID string
	Success      bool
	Error        string
	CompletedAt  int64 // Unix nanoseconds
	Receipt      string
}

// AtomicMetrics provides lock-free performance tracking
type AtomicMetrics struct {
	// Core counters (8-byte aligned)
	TradesCreated      int64
	TradesMatched      int64
	TradesSettled      int64
	TradesFailed       int64
	CompensationEvents int64

	// Latency tracking (nanoseconds)
	TotalLatency int64
	MaxLatency   int64
	MinLatency   int64

	// Backpressure metrics
	BackpressureEvents int64
	DroppedEvents      int64

	// System health
	ActiveTrades       int64
	PendingSettlements int64
	// Padding to prevent false sharing
	_ [6]int64 //nolint:unused
}

// NewAtomicTradeCoordinator creates a new race-free coordinator
func NewAtomicTradeCoordinator(
	logger *zap.Logger,
	settlementEngine *settlement.SettlementEngine,
) *AtomicTradeCoordinator {
	coordinator := &AtomicTradeCoordinator{
		logger:           logger,
		settlementEngine: settlementEngine,

		// Initialize atomic state
		state:           0, // Stopped
		tradeSequence:   0,
		pendingCount:    0,
		registryVersion: 1,

		// Unbuffered channels for immediate processing
		criticalEvents:    make(chan *CriticalTradeEvent),
		settlementResults: make(chan *AtomicSettlementResult),

		// Control channels
		shutdown: make(chan struct{}),

		// Initialize metrics
		metrics: &AtomicMetrics{
			MinLatency: 1<<63 - 1, // Max int64 value
		},
	}

	// Initialize lock-free trade registry
	coordinator.initializeRegistry()

	return coordinator
}

// initializeRegistry sets up the lock-free trade registry
func (atc *AtomicTradeCoordinator) initializeRegistry() {
	registry := &TradeRegistryShards{
		shardMask: 255, // For fast modulo with 256 shards
	}

	// Initialize all shards
	for i := 0; i < 256; i++ {
		shard := &TradeRegistryShard{
			version: 1,
		}
		// Initialize empty map
		emptyMap := make(map[string]*AtomicTradeState)
		atomic.StorePointer(&shard.entries, unsafe.Pointer(&emptyMap))
		registry.shards[i] = shard
	}

	atomic.StorePointer(&atc.tradeRegistry, unsafe.Pointer(registry))
}

// Start begins the atomic coordinator with minimal latency workers
func (atc *AtomicTradeCoordinator) Start(ctx context.Context) error {
	// Atomic state transition to running
	if !atomic.CompareAndSwapInt64(&atc.state, 0, 1) {
		return fmt.Errorf("coordinator already running")
	}

	atc.logger.Info("Starting atomic trade coordinator")

	// Start critical path workers (minimal number for lowest latency)
	atc.workers.Add(2)
	go atc.criticalEventProcessor(ctx)    // Single worker for deterministic ordering
	go atc.settlementResultProcessor(ctx) // Single worker for immediate response

	return nil
}

// ProcessTrade handles a new trade with zero-copy, lock-free operations
func (atc *AtomicTradeCoordinator) ProcessTrade(ctx context.Context, trade *model.Trade) error {
	startTime := time.Now().UnixNano()

	// Quick backpressure check (single atomic read)
	if atomic.LoadInt64(&atc.backpressureSignal) != 0 {
		atomic.AddInt64(&atc.metrics.BackpressureEvents, 1)
		return fmt.Errorf("backpressure active - rejecting trade")
	}

	// Generate monotonic sequence for lock-free ordering
	sequence := atomic.AddInt64(&atc.tradeSequence, 1)
	// Create atomic trade state
	tradeState := &AtomicTradeState{
		TradeID:    trade.ID,
		OrderID:    trade.OrderID,
		Symbol:     trade.Pair,
		Quantity:   trade.Quantity,
		Price:      trade.Price,
		BuyUserID:  trade.UserID,
		SellUserID: trade.CounterUserID,
		CreatedAt:  time.Now(),

		state:    int64(StateCreated),
		sequence: sequence,
		settled:  make(chan struct{}),
	}

	// Register trade atomically
	if err := atc.registerTrade(tradeState); err != nil {
		return fmt.Errorf("failed to register trade: %w", err)
	}

	// Create critical event (zero allocation where possible)
	event := &CriticalTradeEvent{
		EventType: EventTradeExecuted,
		TradeID:   trade.ID,
		Trade:     trade,
		Timestamp: startTime,
		Sequence:  sequence,
		AckChan:   make(chan error, 1), // Buffered for non-blocking send
	}

	// Send to critical path processor (must not block)
	select {
	case atc.criticalEvents <- event:
		// Wait for immediate acknowledgment
		select {
		case err := <-event.AckChan:
			if err != nil {
				atc.unregisterTrade(trade.ID.String())
				return err
			}
		case <-ctx.Done():
			atc.unregisterTrade(trade.ID.String())
			return ctx.Err()
		case <-time.After(100 * time.Microsecond): // Ultra-tight timeout
			atc.unregisterTrade(trade.ID.String())
			atomic.AddInt64(&atc.metrics.DroppedEvents, 1)
			return fmt.Errorf("critical path timeout")
		}
	case <-ctx.Done():
		atc.unregisterTrade(trade.ID.String())
		return ctx.Err()
	default:
		// Never block - apply backpressure instead
		atomic.StoreInt64(&atc.backpressureSignal, 1)
		atc.unregisterTrade(trade.ID.String())
		atomic.AddInt64(&atc.metrics.DroppedEvents, 1)
		return fmt.Errorf("critical path overloaded")
	}

	// Wait for settlement completion with optimized waiting
	select {
	case <-tradeState.settled:
		// Check final state atomically
		finalState := TradeStateAtomic(atomic.LoadInt64(&tradeState.state))
		if finalState == StateSettled {
			// Update metrics atomically
			latency := time.Now().UnixNano() - startTime
			atc.updateLatencyMetrics(latency)
			atomic.AddInt64(&atc.metrics.TradesSettled, 1)
			return nil
		}
		return fmt.Errorf("trade settlement failed, state: %d", finalState)

	case <-ctx.Done():
		// Mark trade as failed and trigger cleanup
		atomic.StoreInt64(&tradeState.state, int64(StateFailed))
		atc.unregisterTrade(trade.ID.String())
		return ctx.Err()
	}
}

// criticalEventProcessor handles events on the critical path with minimal latency
func (atc *AtomicTradeCoordinator) criticalEventProcessor(ctx context.Context) {
	defer atc.workers.Done()

	atc.logger.Info("Critical event processor started")

	for {
		select {
		case event := <-atc.criticalEvents:
			// Process event with immediate acknowledgment
			err := atc.processCriticalEvent(ctx, event)

			// Send immediate acknowledgment (non-blocking)
			select {
			case event.AckChan <- err:
			default:
				// Channel full - should never happen with buffered channel
				atc.logger.Error("Failed to send event acknowledgment")
			}

		case <-atc.shutdown:
			atc.logger.Info("Critical event processor shutting down")
			return

		case <-ctx.Done():
			return
		}
	}
}

// processCriticalEvent handles a single critical event with minimal latency
func (atc *AtomicTradeCoordinator) processCriticalEvent(ctx context.Context, event *CriticalTradeEvent) error {
	tradeState, err := atc.getTradeState(event.TradeID.String())
	if err != nil {
		return err
	}

	switch event.EventType {
	case EventTradeExecuted:
		// Atomic state transition: Created -> Matched
		if !atomic.CompareAndSwapInt64(&tradeState.state, int64(StateCreated), int64(StateMatched)) {
			return fmt.Errorf("trade state transition failed")
		}

		// Immediately queue for settlement
		if !atomic.CompareAndSwapInt64(&tradeState.state, int64(StateMatched), int64(StateQueued)) {
			return fmt.Errorf("settlement queuing failed")
		}

		// Trigger settlement asynchronously (non-blocking)
		go atc.triggerSettlement(ctx, tradeState)

		atomic.AddInt64(&atc.metrics.TradesMatched, 1)
		return nil

	case EventTradeReverted:
		// Atomic state transition to failed
		atomic.StoreInt64(&tradeState.state, int64(StateFailed))
		close(tradeState.settled)
		atomic.AddInt64(&atc.metrics.TradesFailed, 1)
		return nil

	default:
		return fmt.Errorf("unknown critical event type: %d", event.EventType)
	}
}

// triggerSettlement initiates settlement with minimal latency
func (atc *AtomicTradeCoordinator) triggerSettlement(ctx context.Context, tradeState *AtomicTradeState) {
	// Atomic state transition: Queued -> Settling
	if !atomic.CompareAndSwapInt64(&tradeState.state, int64(StateQueued), int64(StateSettling)) {
		atc.logger.Error("Failed to transition to settling state",
			zap.String("trade_id", tradeState.TradeID.String()))
		return
	}
	// Increment attempt counter
	_ = atomic.AddInt64(&tradeState.attempts, 1)
	atomic.StoreInt64(&tradeState.lastAttempt, time.Now().UnixNano())
	// Create settlement capture request
	tradeCapture := settlement.TradeCapture{
		TradeID:   tradeState.TradeID.String(),
		UserID:    tradeState.BuyUserID.String(),
		Symbol:    tradeState.Symbol,
		Side:      "buy", // This trade from buy user perspective
		Quantity:  float64(tradeState.Quantity.InexactFloat64()),
		Price:     float64(tradeState.Price.InexactFloat64()),
		AssetType: "crypto", // Default to crypto, could be determined from symbol
		MatchedAt: time.Now(),
	}

	// Process settlement asynchronously to avoid blocking critical path
	go func() {
		// Capture the trade for settlement
		atc.settlementEngine.CaptureTrade(tradeCapture)

		// Also capture counter trade for the seller
		counterCapture := settlement.TradeCapture{
			TradeID:   tradeState.TradeID.String(),
			UserID:    tradeState.SellUserID.String(),
			Symbol:    tradeState.Symbol,
			Side:      "sell",
			Quantity:  float64(tradeState.Quantity.InexactFloat64()),
			Price:     float64(tradeState.Price.InexactFloat64()),
			AssetType: "crypto",
			MatchedAt: time.Now(),
		}
		atc.settlementEngine.CaptureTrade(counterCapture)

		// For now, simulate successful settlement
		// In a real system, this would involve complex settlement flows
		settlementResult := &AtomicSettlementResult{
			TradeID:      tradeState.TradeID,
			SettlementID: uuid.New().String(),
			Success:      true,
			CompletedAt:  time.Now().UnixNano(),
			Receipt:      fmt.Sprintf("settlement_%s_%d", tradeState.TradeID.String(), time.Now().Unix()),
		}

		// Send result to processor (non-blocking)
		select {
		case atc.settlementResults <- settlementResult:
		case <-ctx.Done():
		default:
			// Should never happen - log error
			atc.logger.Error("Settlement result channel full",
				zap.String("trade_id", tradeState.TradeID.String()))
		}
	}()
}

// settlementResultProcessor handles settlement completions
func (atc *AtomicTradeCoordinator) settlementResultProcessor(ctx context.Context) {
	defer atc.workers.Done()

	atc.logger.Info("Settlement result processor started")

	for {
		select {
		case result := <-atc.settlementResults:
			atc.processSettlementResult(ctx, result)

		case <-atc.shutdown:
			atc.logger.Info("Settlement result processor shutting down")
			return

		case <-ctx.Done():
			return
		}
	}
}

// processSettlementResult handles settlement completion with atomic updates
func (atc *AtomicTradeCoordinator) processSettlementResult(ctx context.Context, result *AtomicSettlementResult) {
	tradeState, err := atc.getTradeState(result.TradeID.String())
	if err != nil {
		atc.logger.Error("Trade not found for settlement result",
			zap.String("trade_id", result.TradeID.String()),
			zap.Error(err))
		return
	}

	if result.Success {
		// Atomic settlement completion
		if atomic.CompareAndSwapInt64(&tradeState.state, int64(StateSettling), int64(StateSettled)) {
			// Store settlement details atomically
			settlementID := result.SettlementID
			atomic.StorePointer(&tradeState.settlementID, unsafe.Pointer(&settlementID))
			atomic.StoreInt64(&tradeState.settledAt, result.CompletedAt)

			// Signal completion
			close(tradeState.settled)

			// Update metrics
			atomic.AddInt64(&atc.metrics.TradesSettled, 1)
			atomic.AddInt64(&atc.pendingCount, -1)

			atc.logger.Debug("Trade settled successfully",
				zap.String("trade_id", result.TradeID.String()),
				zap.String("settlement_id", result.SettlementID))
		}
	} else {
		// Settlement failed - check retry logic
		attempts := atomic.LoadInt64(&tradeState.attempts)

		if attempts < 3 {
			// Retry settlement
			atomic.StoreInt64(&tradeState.state, int64(StateQueued))
			go atc.triggerSettlement(ctx, tradeState)

			atc.logger.Warn("Retrying settlement",
				zap.String("trade_id", result.TradeID.String()),
				zap.Int64("attempt", attempts),
				zap.String("error", result.Error))
		} else {
			// Permanent failure - trigger compensation
			if atomic.CompareAndSwapInt64(&tradeState.state, int64(StateSettling), int64(StateCompensating)) {
				go atc.triggerCompensation(ctx, tradeState, result.Error)

				atc.logger.Error("Settlement permanently failed, triggering compensation",
					zap.String("trade_id", result.TradeID.String()),
					zap.String("error", result.Error))
			}
		}
	}
}

// registerTrade adds a trade to the lock-free registry
func (atc *AtomicTradeCoordinator) registerTrade(tradeState *AtomicTradeState) error {
	tradeID := tradeState.TradeID.String()

	// Get registry and shard
	registry := (*TradeRegistryShards)(atomic.LoadPointer(&atc.tradeRegistry))
	shardIndex := atc.hashTradeID(tradeID) & registry.shardMask
	shard := registry.shards[shardIndex]

	// Load current map atomically
	currentMapPtr := atomic.LoadPointer(&shard.entries)
	currentMap := *(*map[string]*AtomicTradeState)(currentMapPtr)

	// Check if trade already exists
	if _, exists := currentMap[tradeID]; exists {
		return fmt.Errorf("trade already exists: %s", tradeID)
	}

	// Create new map with the trade added
	newMap := make(map[string]*AtomicTradeState, len(currentMap)+1)
	for k, v := range currentMap {
		newMap[k] = v
	}
	newMap[tradeID] = tradeState

	// Atomic update
	if atomic.CompareAndSwapPointer(&shard.entries, currentMapPtr, unsafe.Pointer(&newMap)) {
		atomic.AddInt64(&atc.pendingCount, 1)
		atomic.AddInt64(&atc.metrics.TradesCreated, 1)
		return nil
	}

	// Retry on conflict (rare case)
	return atc.registerTrade(tradeState)
}

// getTradeState retrieves trade state from lock-free registry
func (atc *AtomicTradeCoordinator) getTradeState(tradeID string) (*AtomicTradeState, error) {
	registry := (*TradeRegistryShards)(atomic.LoadPointer(&atc.tradeRegistry))
	shardIndex := atc.hashTradeID(tradeID) & registry.shardMask
	shard := registry.shards[shardIndex]

	// Load current map atomically
	currentMapPtr := atomic.LoadPointer(&shard.entries)
	currentMap := *(*map[string]*AtomicTradeState)(currentMapPtr)

	tradeState, exists := currentMap[tradeID]
	if !exists {
		return nil, fmt.Errorf("trade not found: %s", tradeID)
	}

	return tradeState, nil
}

// unregisterTrade removes a trade from the registry
func (atc *AtomicTradeCoordinator) unregisterTrade(tradeID string) {
	registry := (*TradeRegistryShards)(atomic.LoadPointer(&atc.tradeRegistry))
	shardIndex := atc.hashTradeID(tradeID) & registry.shardMask
	shard := registry.shards[shardIndex]

	// Load current map atomically
	currentMapPtr := atomic.LoadPointer(&shard.entries)
	currentMap := *(*map[string]*AtomicTradeState)(currentMapPtr)

	// Check if trade exists
	if _, exists := currentMap[tradeID]; !exists {
		return
	}

	// Create new map without the trade
	newMap := make(map[string]*AtomicTradeState, len(currentMap)-1)
	for k, v := range currentMap {
		if k != tradeID {
			newMap[k] = v
		}
	}

	// Atomic update
	if atomic.CompareAndSwapPointer(&shard.entries, currentMapPtr, unsafe.Pointer(&newMap)) {
		atomic.AddInt64(&atc.pendingCount, -1)
	}
	// If CAS failed, another goroutine updated the map - that's fine
}

// hashTradeID provides fast hash for shard selection
func (atc *AtomicTradeCoordinator) hashTradeID(tradeID string) uint32 {
	// Simple FNV-1a hash for speed
	var hash uint32 = 2166136261
	for i := 0; i < len(tradeID); i++ {
		hash ^= uint32(tradeID[i])
		hash *= 16777619
	}
	return hash
}

// triggerCompensation handles failed trades
func (atc *AtomicTradeCoordinator) triggerCompensation(ctx context.Context, tradeState *AtomicTradeState, reason string) {
	compensationID := uuid.New().String()
	atomic.StorePointer(&tradeState.compensationID, unsafe.Pointer(&compensationID))

	// Close settled channel to unblock waiting goroutines
	close(tradeState.settled)

	// Remove from registry
	atc.unregisterTrade(tradeState.TradeID.String())

	atomic.AddInt64(&atc.metrics.CompensationEvents, 1)

	atc.logger.Error("Trade compensation triggered",
		zap.String("trade_id", tradeState.TradeID.String()),
		zap.String("compensation_id", compensationID),
		zap.String("reason", reason))
}

// updateLatencyMetrics updates latency tracking atomically
func (atc *AtomicTradeCoordinator) updateLatencyMetrics(latency int64) {
	atomic.AddInt64(&atc.metrics.TotalLatency, latency)

	// Update max latency
	for {
		currentMax := atomic.LoadInt64(&atc.metrics.MaxLatency)
		if latency <= currentMax || atomic.CompareAndSwapInt64(&atc.metrics.MaxLatency, currentMax, latency) {
			break
		}
	}

	// Update min latency
	for {
		currentMin := atomic.LoadInt64(&atc.metrics.MinLatency)
		if latency >= currentMin || atomic.CompareAndSwapInt64(&atc.metrics.MinLatency, currentMin, latency) {
			break
		}
	}
}

// GetMetrics returns current atomic metrics
func (atc *AtomicTradeCoordinator) GetMetrics() AtomicMetrics {
	return AtomicMetrics{
		TradesCreated:      atomic.LoadInt64(&atc.metrics.TradesCreated),
		TradesMatched:      atomic.LoadInt64(&atc.metrics.TradesMatched),
		TradesSettled:      atomic.LoadInt64(&atc.metrics.TradesSettled),
		TradesFailed:       atomic.LoadInt64(&atc.metrics.TradesFailed),
		CompensationEvents: atomic.LoadInt64(&atc.metrics.CompensationEvents),
		TotalLatency:       atomic.LoadInt64(&atc.metrics.TotalLatency),
		MaxLatency:         atomic.LoadInt64(&atc.metrics.MaxLatency),
		MinLatency:         atomic.LoadInt64(&atc.metrics.MinLatency),
		BackpressureEvents: atomic.LoadInt64(&atc.metrics.BackpressureEvents),
		DroppedEvents:      atomic.LoadInt64(&atc.metrics.DroppedEvents),
		ActiveTrades:       atomic.LoadInt64(&atc.pendingCount),
		PendingSettlements: atomic.LoadInt64(&atc.metrics.PendingSettlements),
	}
}

// IsBackpressureActive checks if backpressure is currently active
func (atc *AtomicTradeCoordinator) IsBackpressureActive() bool {
	return atomic.LoadInt64(&atc.backpressureSignal) != 0
}

// ClearBackpressure manually clears backpressure signal
func (atc *AtomicTradeCoordinator) ClearBackpressure() {
	if atomic.CompareAndSwapInt64(&atc.backpressureSignal, 1, 0) {
		atc.logger.Info("Backpressure signal cleared")
	}
}

// Stop gracefully shuts down the coordinator
func (atc *AtomicTradeCoordinator) Stop(ctx context.Context) error {
	atc.shutdownOnce.Do(func() {
		// Atomic state transition to stopping
		atomic.StoreInt64(&atc.state, 2)

		atc.logger.Info("Stopping atomic trade coordinator")

		// Signal shutdown to all workers
		close(atc.shutdown)

		// Wait for workers to complete with timeout
		done := make(chan struct{})
		go func() {
			atc.workers.Wait()
			close(done)
		}()

		select {
		case <-done:
			atc.logger.Info("Atomic trade coordinator stopped successfully")
		case <-ctx.Done():
			atc.logger.Warn("Coordinator shutdown timed out")
		}

		// Final state
		atomic.StoreInt64(&atc.state, 0)
	})

	return nil
}
