// =============================
// Deadlock-Free Order Book Implementation
// =============================
// This file implements a deadlock-free version of the order book with:
// 1. Consistent lock ordering to prevent circular dependencies
// 2. Minimal lock scope to reduce contention
// 3. Atomic operations for critical sections
// 4. Lock-free read paths where possible

package orderbook

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/trading/model"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/tidwall/btree"
)

// Lock ordering hierarchy (always acquire in this order to prevent deadlocks):
// 1. Administrative locks (adminState.mu)
// 2. Global order book locks (in pair alphabetical order: asksMu before bidsMu)
// 3. Price level locks (in price order: lower prices before higher prices)
// 4. Order tracking locks (ordersMu)

// DeadlockSafeOrderBook is a reimplementation of OrderBook with deadlock prevention
type DeadlockSafeOrderBook struct {
	Pair       string
	bids       *btree.Map[string, *SafePriceLevel]
	asks       *btree.Map[string, *SafePriceLevel]
	ordersByID sync.Map // Use concurrent map for ordersByID to reduce lock contention

	// Lock ordering: Always acquire asksMu before bidsMu to prevent deadlocks
	asksMu sync.RWMutex // protects asks B-tree structure
	bidsMu sync.RWMutex // protects bids B-tree structure

	// Administrative controls with separate lock
	adminState *SafeMarketAdminState

	// Atomic counters for metrics
	totalOrders int64
	totalTrades int64
	lastUpdate  int64 // Unix nano timestamp
	contention  int64 // Contention counter for adaptive behavior

	// Snapshot caching with lock-free reads
	snapshotVersion int64        // Atomic version counter
	cachedSnapshot  atomic.Value // Stores *CachedSnapshot

	// Top levels cache with separate lock
	topLevelsMu    sync.RWMutex
	topBidsCache   []TopLevel
	topAsksCache   []TopLevel
	cacheVersion   int64
	cacheThreshold int64 // Minimum operations before cache update
}

// SafePriceLevel uses atomic operations and minimal locking
type SafePriceLevel struct {
	Price      string
	orders     []*model.Order
	mu         sync.RWMutex
	version    int64 // Atomic version for optimistic reads
	lastAccess int64 // For LRU eviction
}

// SafeMarketAdminState with atomic flags for fast reads
type SafeMarketAdminState struct {
	// Atomic flags for fastest possible reads
	breakerActive int32 // 0 = inactive, 1 = active
	tradingHalted int32 // 0 = not halted, 1 = halted
	emergencyStop int32 // 0 = normal, 1 = emergency

	// Less frequently accessed data with mutex
	mu         sync.RWMutex
	fees       decimal.Decimal
	userLimits map[uuid.UUID]decimal.Decimal
	params     map[string]interface{}
	config     map[string]interface{}
}

// CachedSnapshot for lock-free snapshot reads
type CachedSnapshot struct {
	Bids      [][]string
	Asks      [][]string
	Timestamp int64
	Version   int64
}

// TopLevel represents a cached price level
type TopLevel struct {
	Price  string
	Volume string
}

// OrderMatch represents a trade match
type OrderMatch struct {
	TakerOrder *model.Order
	MakerOrder *model.Order
	Price      decimal.Decimal
	Quantity   decimal.Decimal
	Trade      *model.Trade
}

// AddOrderResult holds the result of AddOrder operation
type SafeAddOrderResult struct {
	Trades        []*model.Trade
	RestingOrders []*model.Order
	Matches       []OrderMatch
}

// NewDeadlockSafeOrderBook creates a new deadlock-safe order book
func NewDeadlockSafeOrderBook(pair string) *DeadlockSafeOrderBook {
	ob := &DeadlockSafeOrderBook{
		Pair:           pair,
		bids:           btree.NewMap[string, *SafePriceLevel](64),
		asks:           btree.NewMap[string, *SafePriceLevel](64),
		cacheThreshold: 100, // Update cache every 100 operations
		adminState: &SafeMarketAdminState{
			userLimits: make(map[uuid.UUID]decimal.Decimal),
			params:     make(map[string]interface{}),
			config:     make(map[string]interface{}),
		},
	}

	// Initialize with empty snapshot
	ob.cachedSnapshot.Store(&CachedSnapshot{
		Bids:      make([][]string, 0),
		Asks:      make([][]string, 0),
		Timestamp: time.Now().UnixNano(),
		Version:   0,
	})

	return ob
}

// IsCircuitBreakerActive checks circuit breaker status atomically
func (ob *DeadlockSafeOrderBook) IsCircuitBreakerActive() bool {
	return atomic.LoadInt32(&ob.adminState.breakerActive) == 1
}

// IsTradingHalted checks trading halt status atomically
func (ob *DeadlockSafeOrderBook) IsTradingHalted() bool {
	return atomic.LoadInt32(&ob.adminState.tradingHalted) == 1
}

// IsEmergencyStop checks emergency stop status atomically
func (ob *DeadlockSafeOrderBook) IsEmergencyStop() bool {
	return atomic.LoadInt32(&ob.adminState.emergencyStop) == 1
}

// SetCircuitBreakerActive sets circuit breaker status atomically
func (ob *DeadlockSafeOrderBook) SetCircuitBreakerActive(active bool) {
	var value int32
	if active {
		value = 1
	}
	atomic.StoreInt32(&ob.adminState.breakerActive, value)
}

// SetTradingHalted sets trading halt status atomically
func (ob *DeadlockSafeOrderBook) SetTradingHalted(halted bool) {
	var value int32
	if halted {
		value = 1
	}
	atomic.StoreInt32(&ob.adminState.tradingHalted, value)
}

// SetEmergencyStop sets emergency stop status atomically
func (ob *DeadlockSafeOrderBook) SetEmergencyStop(stop bool) {
	var value int32
	if stop {
		value = 1
	}
	atomic.StoreInt32(&ob.adminState.emergencyStop, value)
}

// addOrderToLevel adds an order to a price level (must be called with level lock held)
func (pl *SafePriceLevel) addOrderToLevel(order *model.Order) {
	pl.orders = append(pl.orders, order)
	atomic.AddInt64(&pl.version, 1)
	atomic.StoreInt64(&pl.lastAccess, time.Now().UnixNano())
}

// removeOrderFromLevel removes an order from a price level (must be called with level lock held)
func (pl *SafePriceLevel) removeOrderFromLevel(orderID uuid.UUID) bool {
	for i, order := range pl.orders {
		if order.ID == orderID {
			// Remove order by swapping with last element
			pl.orders[i] = pl.orders[len(pl.orders)-1]
			pl.orders = pl.orders[:len(pl.orders)-1]
			atomic.AddInt64(&pl.version, 1)
			return true
		}
	}
	return false
}

// getOrdersSnapshot returns a snapshot of orders (must be called with level read lock held)
func (pl *SafePriceLevel) getOrdersSnapshot() []*model.Order {
	snapshot := make([]*model.Order, len(pl.orders))
	copy(snapshot, pl.orders)
	return snapshot
}

// isEmpty checks if price level is empty (must be called with level read lock held)
func (pl *SafePriceLevel) isEmpty() bool {
	return len(pl.orders) == 0
}

// updateLastAccess updates the last access time atomically
func (pl *SafePriceLevel) updateLastAccess() {
	atomic.StoreInt64(&pl.lastAccess, time.Now().UnixNano())
}

// DEADLOCK-SAFE AddOrder implementation
func (ob *DeadlockSafeOrderBook) AddOrder(order *model.Order) (*SafeAddOrderResult, error) {
	if order == nil {
		return nil, fmt.Errorf("order is nil")
	}

	// Fast atomic checks without locks
	if ob.IsCircuitBreakerActive() {
		return nil, fmt.Errorf("circuit breaker is active")
	}
	if ob.IsTradingHalted() {
		return nil, fmt.Errorf("trading is halted")
	}
	if ob.IsEmergencyStop() {
		return nil, fmt.Errorf("emergency stop is active")
	}

	// Increment operation counter
	atomic.AddInt64(&ob.totalOrders, 1)
	atomic.StoreInt64(&ob.lastUpdate, time.Now().UnixNano())

	// Determine book sides and lock ordering
	var bookMap, oppBookMap *btree.Map[string, *SafePriceLevel]
	var bookMu, oppBookMu *sync.RWMutex
	var isBuy bool

	if order.Side == model.OrderSideBuy {
		bookMap = ob.bids
		oppBookMap = ob.asks
		bookMu = &ob.bidsMu
		oppBookMu = &ob.asksMu // Always acquire asks before bids for consistency
		isBuy = true
	} else {
		bookMap = ob.asks
		oppBookMap = ob.bids
		bookMu = &ob.asksMu
		oppBookMu = &ob.bidsMu
		isBuy = false
	}

	qtyLeft := order.Quantity.Sub(order.FilledQuantity)
	if qtyLeft.LessThanOrEqual(decimal.Zero) {
		return nil, fmt.Errorf("order quantity is zero or negative")
	}

	var matches []OrderMatch
	var trades []*model.Trade
	var restingOrders []*model.Order

	// Phase 1: Matching (read-only operations on opposite book)
	matches = ob.findMatches(order, oppBookMap, oppBookMu, isBuy, qtyLeft)

	// Phase 2: Execute matches (minimal lock scope)
	trades, qtyLeft = ob.executeMatches(matches, order, qtyLeft)

	// Phase 3: Place remaining order on book if needed
	if qtyLeft.GreaterThan(decimal.Zero) &&
		order.TimeInForce != model.TimeInForceIOC &&
		order.TimeInForce != model.TimeInForceFOK {

		err := ob.placeRestingOrder(order, bookMap, bookMu, qtyLeft)
		if err != nil {
			return nil, err
		}
		restingOrders = append(restingOrders, order)
	}

	// Phase 4: Update metrics and cache asynchronously if needed
	ob.updateMetricsAndCache()

	return &SafeAddOrderResult{
		Trades:        trades,
		RestingOrders: restingOrders,
		Matches:       matches,
	}, nil
}

// findMatches finds potential matches without holding locks for extended periods
func (ob *DeadlockSafeOrderBook) findMatches(
	order *model.Order,
	oppBookMap *btree.Map[string, *SafePriceLevel],
	oppBookMu *sync.RWMutex,
	isBuy bool,
	qtyLeft decimal.Decimal,
) []OrderMatch {
	var matches []OrderMatch

	// Read opposite book structure with minimal lock time
	oppBookMu.RLock()
	// Create a snapshot of price levels to check
	var priceLevels []string
	oppBookMap.Scan(func(price string, level *SafePriceLevel) bool {
		priceLevels = append(priceLevels, price)
		return true
	})
	oppBookMu.RUnlock()

	// Process each price level individually with minimal lock scope
	for _, priceStr := range priceLevels {
		if qtyLeft.LessThanOrEqual(decimal.Zero) {
			break
		}

		priceDec, err := decimal.NewFromString(priceStr)
		if err != nil {
			continue
		}

		// Check if price crosses
		if (isBuy && order.Price.LessThan(priceDec)) ||
			(!isBuy && order.Price.GreaterThan(priceDec)) {
			break
		}

		// Get the price level with minimal lock time
		oppBookMu.RLock()
		level, ok := oppBookMap.Get(priceStr)
		oppBookMu.RUnlock()

		if !ok {
			continue
		}

		// Check orders at this price level
		level.mu.RLock()
		orders := level.getOrdersSnapshot()
		level.mu.RUnlock()

		for _, maker := range orders {
			if maker == nil || maker.FilledQuantity.GreaterThanOrEqual(maker.Quantity) {
				continue
			}
			if maker.UserID == order.UserID {
				continue // Self-trading prevention
			}

			makerQtyLeft := maker.Quantity.Sub(maker.FilledQuantity)
			matchQty := decimal.Min(qtyLeft, makerQtyLeft)
			if matchQty.LessThanOrEqual(decimal.Zero) {
				continue
			}

			// Create match record
			trade := &model.Trade{
				ID:        uuid.New(),
				OrderID:   order.ID,
				Pair:      order.Pair,
				Price:     priceDec,
				Quantity:  matchQty,
				Side:      order.Side,
				Maker:     false,
				CreatedAt: time.Now(),
			}

			match := OrderMatch{
				TakerOrder: order,
				MakerOrder: maker,
				Price:      priceDec,
				Quantity:   matchQty,
				Trade:      trade,
			}

			matches = append(matches, match)
			qtyLeft = qtyLeft.Sub(matchQty)

			if qtyLeft.LessThanOrEqual(decimal.Zero) {
				break
			}
		}
	}

	return matches
}

// executeMatches executes the matches found, updating orders and cleaning up price levels
func (ob *DeadlockSafeOrderBook) executeMatches(
	matches []OrderMatch,
	takerOrder *model.Order,
	qtyLeft decimal.Decimal,
) ([]*model.Trade, decimal.Decimal) {
	var trades []*model.Trade
	var levelsToClean []string

	for _, match := range matches {
		// Update order quantities
		takerOrder.FilledQuantity = takerOrder.FilledQuantity.Add(match.Quantity)
		match.MakerOrder.FilledQuantity = match.MakerOrder.FilledQuantity.Add(match.Quantity)
		qtyLeft = qtyLeft.Sub(match.Quantity)

		trades = append(trades, match.Trade)

		// If maker order is fully filled, mark for removal
		if match.MakerOrder.FilledQuantity.GreaterThanOrEqual(match.MakerOrder.Quantity) {
			// Remove from ordersByID
			ob.ordersByID.Delete(match.MakerOrder.ID)

			// Mark price level for cleaning
			priceStr := match.Price.String()
			levelsToClean = append(levelsToClean, priceStr)
		}

		atomic.AddInt64(&ob.totalTrades, 1)
	}

	// Clean up empty price levels in a separate phase
	ob.cleanupEmptyLevels(levelsToClean)

	return trades, qtyLeft
}

// cleanupEmptyLevels removes fully filled orders and empty price levels
func (ob *DeadlockSafeOrderBook) cleanupEmptyLevels(priceStrings []string) {
	// Group by book type to minimize lock acquisitions
	bidPrices := make([]string, 0)
	askPrices := make([]string, 0)

	for _, priceStr := range priceStrings {
		// Determine if this price exists in bids or asks
		ob.bidsMu.RLock()
		_, inBids := ob.bids.Get(priceStr)
		ob.bidsMu.RUnlock()

		if inBids {
			bidPrices = append(bidPrices, priceStr)
		} else {
			askPrices = append(askPrices, priceStr)
		}
	}

	// Clean up bids
	if len(bidPrices) > 0 {
		ob.cleanupPriceLevels(ob.bids, &ob.bidsMu, bidPrices)
	}

	// Clean up asks
	if len(askPrices) > 0 {
		ob.cleanupPriceLevels(ob.asks, &ob.asksMu, askPrices)
	}
}

// cleanupPriceLevels removes empty price levels from a specific book
func (ob *DeadlockSafeOrderBook) cleanupPriceLevels(
	bookMap *btree.Map[string, *SafePriceLevel],
	bookMu *sync.RWMutex,
	priceStrings []string,
) {
	levelsToRemove := make([]string, 0)

	// First, check which levels are actually empty
	for _, priceStr := range priceStrings {
		bookMu.RLock()
		level, ok := bookMap.Get(priceStr)
		bookMu.RUnlock()

		if !ok {
			continue
		}

		level.mu.Lock()
		// Remove fully filled orders
		var activeOrders []*model.Order
		for _, order := range level.orders {
			if order.FilledQuantity.LessThan(order.Quantity) {
				activeOrders = append(activeOrders, order)
			}
		}
		level.orders = activeOrders

		if level.isEmpty() {
			levelsToRemove = append(levelsToRemove, priceStr)
		}
		level.mu.Unlock()
	}

	// Remove empty levels in batch
	if len(levelsToRemove) > 0 {
		bookMu.Lock()
		for _, priceStr := range levelsToRemove {
			bookMap.Delete(priceStr)
		}
		bookMu.Unlock()
	}
}

// placeRestingOrder places the remaining quantity on the appropriate book
func (ob *DeadlockSafeOrderBook) placeRestingOrder(
	order *model.Order,
	bookMap *btree.Map[string, *SafePriceLevel],
	bookMu *sync.RWMutex,
	qtyLeft decimal.Decimal,
) error {
	priceStr := order.Price.String()

	// Try to get existing level
	bookMu.RLock()
	level, exists := bookMap.Get(priceStr)
	bookMu.RUnlock()

	if !exists {
		// Create new level
		level = &SafePriceLevel{
			Price:   priceStr,
			orders:  make([]*model.Order, 0, 4),
			version: 1,
		}

		bookMu.Lock()
		bookMap.Set(priceStr, level)
		bookMu.Unlock()
	}

	// Add order to level
	level.mu.Lock()
	level.addOrderToLevel(order)
	level.mu.Unlock()

	// Store in orders map
	ob.ordersByID.Store(order.ID, order)

	return nil
}

// updateMetricsAndCache updates metrics and refreshes cache if needed
func (ob *DeadlockSafeOrderBook) updateMetricsAndCache() {
	totalOps := atomic.LoadInt64(&ob.totalOrders) + atomic.LoadInt64(&ob.totalTrades)

	// Update cache periodically based on operation count
	if totalOps%ob.cacheThreshold == 0 {
		go ob.refreshSnapshotCache(20) // Async cache refresh
	}
}

// refreshSnapshotCache refreshes the cached snapshot asynchronously
func (ob *DeadlockSafeOrderBook) refreshSnapshotCache(depth int) {
	bids, asks := ob.generateSnapshot(depth)

	newSnapshot := &CachedSnapshot{
		Bids:      bids,
		Asks:      asks,
		Timestamp: time.Now().UnixNano(),
		Version:   atomic.AddInt64(&ob.snapshotVersion, 1),
	}

	ob.cachedSnapshot.Store(newSnapshot)
}

// generateSnapshot generates a fresh snapshot with minimal locking
func (ob *DeadlockSafeOrderBook) generateSnapshot(depth int) ([][]string, [][]string) {
	if depth > MAX_SNAPSHOT_DEPTH {
		depth = MAX_SNAPSHOT_DEPTH
	}

	bids := make([][]string, 0, depth)
	asks := make([][]string, 0, depth)

	// Generate bids snapshot
	ob.bidsMu.RLock()
	ob.bids.Reverse(func(price string, level *SafePriceLevel) bool {
		level.mu.RLock()
		totalQty := decimal.Zero
		for _, order := range level.orders {
			totalQty = totalQty.Add(order.Quantity.Sub(order.FilledQuantity))
		}
		if totalQty.GreaterThan(decimal.Zero) {
			bids = append(bids, []string{price, totalQty.String()})
		}
		level.mu.RUnlock()
		level.updateLastAccess()
		return len(bids) < depth
	})
	ob.bidsMu.RUnlock()

	// Generate asks snapshot
	ob.asksMu.RLock()
	ob.asks.Scan(func(price string, level *SafePriceLevel) bool {
		level.mu.RLock()
		totalQty := decimal.Zero
		for _, order := range level.orders {
			totalQty = totalQty.Add(order.Quantity.Sub(order.FilledQuantity))
		}
		if totalQty.GreaterThan(decimal.Zero) {
			asks = append(asks, []string{price, totalQty.String()})
		}
		level.mu.RUnlock()
		level.updateLastAccess()
		return len(asks) < depth
	})
	ob.asksMu.RUnlock()

	return bids, asks
}

// GetSnapshot returns a snapshot using cached data when possible
func (ob *DeadlockSafeOrderBook) GetSnapshot(depth int) ([][]string, [][]string) {
	snapshot := ob.cachedSnapshot.Load().(*CachedSnapshot)
	// Check if cached snapshot is recent enough (less than 100ms old)
	if time.Now().UnixNano()-snapshot.Timestamp < 100*time.Millisecond.Nanoseconds() { // Return cached data
		bidsLen := len(snapshot.Bids)
		if bidsLen > depth {
			bidsLen = depth
		}
		asksLen := len(snapshot.Asks)
		if asksLen > depth {
			asksLen = depth
		}

		bidsOut := make([][]string, 0, bidsLen)
		asksOut := make([][]string, 0, asksLen)

		for i := 0; i < len(snapshot.Bids) && i < depth; i++ {
			bidsOut = append(bidsOut, snapshot.Bids[i])
		}
		for i := 0; i < len(snapshot.Asks) && i < depth; i++ {
			asksOut = append(asksOut, snapshot.Asks[i])
		}

		return bidsOut, asksOut
	}

	// Generate fresh snapshot
	return ob.generateSnapshot(depth)
}

// CancelOrder cancels an order with deadlock-safe implementation
func (ob *DeadlockSafeOrderBook) CancelOrder(orderID uuid.UUID) error {
	// Find order in the map
	orderValue, ok := ob.ordersByID.Load(orderID)
	if !ok {
		return fmt.Errorf("order not found: %v", orderID)
	}

	order := orderValue.(*model.Order)
	priceStr := order.Price.String()

	// Determine which book and lock to use
	var bookMap *btree.Map[string, *SafePriceLevel]
	var bookMu *sync.RWMutex

	if order.Side == model.OrderSideBuy {
		bookMap = ob.bids
		bookMu = &ob.bidsMu
	} else {
		bookMap = ob.asks
		bookMu = &ob.asksMu
	}

	// Find and remove from price level
	bookMu.RLock()
	level, exists := bookMap.Get(priceStr)
	bookMu.RUnlock()

	if !exists {
		// Order not in book, just remove from map
		ob.ordersByID.Delete(orderID)
		return nil
	}

	// Remove from price level
	level.mu.Lock()
	removed := level.removeOrderFromLevel(orderID)
	isEmpty := level.isEmpty()
	level.mu.Unlock()

	if !removed {
		// Order not found in price level
		ob.ordersByID.Delete(orderID)
		return nil
	}

	// Remove from orders map
	ob.ordersByID.Delete(orderID)

	// Remove empty price level
	if isEmpty {
		bookMu.Lock()
		// Double-check emptiness under write lock
		level.mu.RLock()
		stillEmpty := level.isEmpty()
		level.mu.RUnlock()

		if stillEmpty {
			bookMap.Delete(priceStr)
		}
		bookMu.Unlock()
	}

	return nil
}

// GetOrderByID retrieves an order by ID
func (ob *DeadlockSafeOrderBook) GetOrderByID(orderID uuid.UUID) (*model.Order, bool) {
	orderValue, ok := ob.ordersByID.Load(orderID)
	if !ok {
		return nil, false
	}
	return orderValue.(*model.Order), true
}

// GetTotalOrders returns the total number of orders processed
func (ob *DeadlockSafeOrderBook) GetTotalOrders() int64 {
	return atomic.LoadInt64(&ob.totalOrders)
}

// GetTotalTrades returns the total number of trades executed
func (ob *DeadlockSafeOrderBook) GetTotalTrades() int64 {
	return atomic.LoadInt64(&ob.totalTrades)
}

// GetLastUpdate returns the timestamp of the last update
func (ob *DeadlockSafeOrderBook) GetLastUpdate() time.Time {
	nanos := atomic.LoadInt64(&ob.lastUpdate)
	return time.Unix(0, nanos)
}

// OrdersCount returns the total number of orders in the orderbook
func (ob *DeadlockSafeOrderBook) OrdersCount() int {
	ob.bidsMu.RLock()
	ob.asksMu.RLock()
	defer ob.bidsMu.RUnlock()
	defer ob.asksMu.RUnlock()

	count := 0

	// Count bid orders
	ob.bids.Scan(func(price string, level *SafePriceLevel) bool {
		level.mu.RLock()
		count += len(level.orders)
		level.mu.RUnlock()
		return true
	})

	// Count ask orders
	ob.asks.Scan(func(price string, level *SafePriceLevel) bool {
		level.mu.RLock()
		count += len(level.orders)
		level.mu.RUnlock()
		return true
	})

	return count
}

// GetOrderIDs returns all order IDs in the orderbook
func (ob *DeadlockSafeOrderBook) GetOrderIDs() []uuid.UUID {
	ob.bidsMu.RLock()
	ob.asksMu.RLock()
	defer ob.bidsMu.RUnlock()
	defer ob.asksMu.RUnlock()

	var orderIDs []uuid.UUID

	// Collect bid order IDs
	ob.bids.Scan(func(price string, level *SafePriceLevel) bool {
		level.mu.RLock()
		for _, order := range level.orders {
			orderIDs = append(orderIDs, order.ID)
		}
		level.mu.RUnlock()
		return true
	})

	// Collect ask order IDs
	ob.asks.Scan(func(price string, level *SafePriceLevel) bool {
		level.mu.RLock()
		for _, order := range level.orders {
			orderIDs = append(orderIDs, order.ID)
		}
		level.mu.RUnlock()
		return true
	})

	return orderIDs
}

// CanFullyFill checks if an order can be fully filled with current liquidity
func (ob *DeadlockSafeOrderBook) CanFullyFill(order *model.Order) bool {
	if order.Type == "market" {
		// Market orders can always be partially filled
		return true
	}

	ob.bidsMu.RLock()
	ob.asksMu.RLock()
	defer ob.bidsMu.RUnlock()
	defer ob.asksMu.RUnlock()

	requiredQuantity := order.Quantity.Sub(order.FilledQuantity)
	if requiredQuantity.LessThanOrEqual(decimal.Zero) {
		return true // Already filled
	}

	availableQuantity := decimal.Zero

	if order.Side == "buy" {
		// Check available ask liquidity at or below order price
		ob.asks.Scan(func(price string, level *SafePriceLevel) bool {
			levelPrice, err := decimal.NewFromString(price)
			if err != nil {
				return true // Skip invalid prices
			}

			if levelPrice.GreaterThan(order.Price) {
				return false // Stop scanning, prices too high
			}

			level.mu.RLock()
			for _, askOrder := range level.orders {
				availableQty := askOrder.Quantity.Sub(askOrder.FilledQuantity)
				availableQuantity = availableQuantity.Add(availableQty)
			}
			level.mu.RUnlock()

			return availableQuantity.LessThan(requiredQuantity) // Continue if more needed
		})
	} else {
		// Check available bid liquidity at or above order price
		ob.bids.Scan(func(price string, level *SafePriceLevel) bool {
			levelPrice, err := decimal.NewFromString(price)
			if err != nil {
				return true // Skip invalid prices
			}

			if levelPrice.LessThan(order.Price) {
				return false // Stop scanning, prices too low
			}

			level.mu.RLock()
			for _, bidOrder := range level.orders {
				availableQty := bidOrder.Quantity.Sub(bidOrder.FilledQuantity)
				availableQuantity = availableQuantity.Add(availableQty)
			}
			level.mu.RUnlock()

			return availableQuantity.LessThan(requiredQuantity) // Continue if more needed
		})
	}

	return availableQuantity.GreaterThanOrEqual(requiredQuantity)
}
