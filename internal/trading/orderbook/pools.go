// =============================
// Order Book Object Pools
// =============================
// This file implements comprehensive object pooling for order book structures
// to reduce GC pressure and improve performance.

package orderbook

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/Aidin1998/finalex/internal/trading/model"
)

// Pool metrics for order book structures
type OrderBookPoolMetrics struct {
	Gets        int64
	Puts        int64
	Hits        int64
	Misses      int64
	Allocations int64
}

var (
	priceLevelPoolMetrics     = &OrderBookPoolMetrics{}
	orderChunkPoolMetrics     = &OrderBookPoolMetrics{}
	safePriceLevelPoolMetrics = &OrderBookPoolMetrics{}
	snapshotSlicePoolMetrics  = &OrderBookPoolMetrics{}
)

// PriceLevelPool for standard order book price levels
var PriceLevelPool = sync.Pool{
	New: func() any {
		atomic.AddInt64(&priceLevelPoolMetrics.Allocations, 1)
		atomic.AddInt64(&priceLevelPoolMetrics.Misses, 1)
		return &PriceLevel{
			firstChunk: nil,
			Price:      "",
		}
	},
}

// OrderChunkPool for order chunk arrays
var OrderChunkPool = sync.Pool{
	New: func() any {
		atomic.AddInt64(&orderChunkPoolMetrics.Allocations, 1)
		atomic.AddInt64(&orderChunkPoolMetrics.Misses, 1)
		return &orderChunk{
			head: 0,
			tail: 0,
			next: nil,
		}
	},
}

// SafePriceLevelPool for deadlock-safe order book price levels
var SafePriceLevelPool = sync.Pool{
	New: func() any {
		atomic.AddInt64(&safePriceLevelPoolMetrics.Allocations, 1)
		atomic.AddInt64(&safePriceLevelPoolMetrics.Misses, 1)
		return &SafePriceLevel{
			Price:   "",
			orders:  make([]*model.Order, 0, 4),
			version: 0,
		}
	},
}

// Enhanced snapshot slice pool with metrics
var enhancedSnapshotSlicePool = sync.Pool{
	New: func() any {
		atomic.AddInt64(&snapshotSlicePoolMetrics.Allocations, 1)
		atomic.AddInt64(&snapshotSlicePoolMetrics.Misses, 1)
		return make([]topLevel, 0, 100)
	},
}

// GetPriceLevelFromPool returns a pooled PriceLevel
func GetPriceLevelFromPool() *PriceLevel {
	atomic.AddInt64(&priceLevelPoolMetrics.Gets, 1)
	level := PriceLevelPool.Get().(*PriceLevel)
	if level.Price != "" {
		atomic.AddInt64(&priceLevelPoolMetrics.Hits, 1)
	}
	return level
}

// PutPriceLevelToPool returns a PriceLevel to the pool
func PutPriceLevelToPool(level *PriceLevel) {
	if level != nil {
		// Reset the price level before returning to pool
		level.Price = ""
		level.firstChunk = nil
		level.lockContention = 0
		atomic.AddInt64(&priceLevelPoolMetrics.Puts, 1)
		PriceLevelPool.Put(level)
	}
}

// GetOrderChunkFromPool gets an order chunk from the pool
func GetOrderChunkFromPool() *orderChunk {
	atomic.AddInt64(&orderChunkPoolMetrics.Gets, 1)
	chunk := OrderChunkPool.Get().(*orderChunk)
	if chunk.head != 0 || chunk.tail != 0 {
		atomic.AddInt64(&orderChunkPoolMetrics.Hits, 1)
	}
	return chunk
}

// PutOrderChunkToPool returns an order chunk to the pool after resetting
func PutOrderChunkToPool(chunk *orderChunk) {
	if chunk != nil {
		ResetOrderChunk(chunk)
		atomic.AddInt64(&orderChunkPoolMetrics.Puts, 1)
		OrderChunkPool.Put(chunk)
	}
}

// GetSafePriceLevelFromPool gets a safe price level from the pool
func GetSafePriceLevelFromPool() *SafePriceLevel {
	atomic.AddInt64(&safePriceLevelPoolMetrics.Gets, 1)
	level := SafePriceLevelPool.Get().(*SafePriceLevel)
	if level.Price != "" {
		atomic.AddInt64(&safePriceLevelPoolMetrics.Hits, 1)
	}
	return level
}

// PutSafePriceLevelToPool returns a safe price level to the pool after resetting
func PutSafePriceLevelToPool(level *SafePriceLevel) {
	if level != nil {
		ResetSafePriceLevel(level)
		atomic.AddInt64(&safePriceLevelPoolMetrics.Puts, 1)
		SafePriceLevelPool.Put(level)
	}
}

// GetSnapshotSliceFromPool gets a snapshot slice from the pool
func GetSnapshotSliceFromPool() []topLevel {
	atomic.AddInt64(&snapshotSlicePoolMetrics.Gets, 1)
	slice := enhancedSnapshotSlicePool.Get().([]topLevel)
	if len(slice) > 0 {
		atomic.AddInt64(&snapshotSlicePoolMetrics.Hits, 1)
	}
	return slice[:0] // Reset length but keep capacity
}

// PutSnapshotSliceToPool returns a snapshot slice to the pool
func PutSnapshotSliceToPool(slice []topLevel) {
	if slice != nil {
		atomic.AddInt64(&snapshotSlicePoolMetrics.Puts, 1)
		enhancedSnapshotSlicePool.Put(slice)
	}
}

// Reset functions to clear object state for safe pooling

func ResetPriceLevel(level *PriceLevel) {
	// Reset all chunks in the linked list
	chunk := level.firstChunk
	for chunk != nil {
		next := chunk.next
		ResetOrderChunk(chunk)
		PutOrderChunkToPool(chunk)
		chunk = next
	}

	// Reset the price level
	level.firstChunk = nil
	level.Price = ""
	level.lockContention = 0
}

func ResetOrderChunk(chunk *orderChunk) {
	// Clear all order references
	for i := range chunk.orders {
		chunk.orders[i] = nil
	}
	chunk.head = 0
	chunk.tail = 0
	chunk.next = nil
}

func ResetSafePriceLevel(level *SafePriceLevel) {
	// Clear orders slice (keep capacity)
	level.orders = level.orders[:0]
	level.Price = ""
	level.version = 0
	level.lastAccess = 0
}

// Pool metrics collection functions

func GetPriceLevelPoolMetrics() OrderBookPoolMetrics {
	return OrderBookPoolMetrics{
		Gets:        atomic.LoadInt64(&priceLevelPoolMetrics.Gets),
		Puts:        atomic.LoadInt64(&priceLevelPoolMetrics.Puts),
		Hits:        atomic.LoadInt64(&priceLevelPoolMetrics.Hits),
		Misses:      atomic.LoadInt64(&priceLevelPoolMetrics.Misses),
		Allocations: atomic.LoadInt64(&priceLevelPoolMetrics.Allocations),
	}
}

func GetOrderChunkPoolMetrics() OrderBookPoolMetrics {
	return OrderBookPoolMetrics{
		Gets:        atomic.LoadInt64(&orderChunkPoolMetrics.Gets),
		Puts:        atomic.LoadInt64(&orderChunkPoolMetrics.Puts),
		Hits:        atomic.LoadInt64(&orderChunkPoolMetrics.Hits),
		Misses:      atomic.LoadInt64(&orderChunkPoolMetrics.Misses),
		Allocations: atomic.LoadInt64(&orderChunkPoolMetrics.Allocations),
	}
}

func GetSafePriceLevelPoolMetrics() OrderBookPoolMetrics {
	return OrderBookPoolMetrics{
		Gets:        atomic.LoadInt64(&safePriceLevelPoolMetrics.Gets),
		Puts:        atomic.LoadInt64(&safePriceLevelPoolMetrics.Puts),
		Hits:        atomic.LoadInt64(&safePriceLevelPoolMetrics.Hits),
		Misses:      atomic.LoadInt64(&safePriceLevelPoolMetrics.Misses),
		Allocations: atomic.LoadInt64(&safePriceLevelPoolMetrics.Allocations),
	}
}

func GetSnapshotSlicePoolMetrics() OrderBookPoolMetrics {
	return OrderBookPoolMetrics{
		Gets:        atomic.LoadInt64(&snapshotSlicePoolMetrics.Gets),
		Puts:        atomic.LoadInt64(&snapshotSlicePoolMetrics.Puts),
		Hits:        atomic.LoadInt64(&snapshotSlicePoolMetrics.Hits),
		Misses:      atomic.LoadInt64(&snapshotSlicePoolMetrics.Misses),
		Allocations: atomic.LoadInt64(&snapshotSlicePoolMetrics.Allocations),
	}
}

// PrewarmOrderBookPools pre-allocates objects for all order book pools
func PrewarmOrderBookPools(priceLevels, orderChunks, safePriceLevels, snapshotSlices int) {
	// Ensure minimum sizes
	if priceLevels < 100 {
		priceLevels = 100
	}
	if orderChunks < 500 {
		orderChunks = 500
	}
	if safePriceLevels < 100 {
		safePriceLevels = 100
	}
	if snapshotSlices < 50 {
		snapshotSlices = 50
	}

	// Pre-warm PriceLevelPool
	for i := 0; i < priceLevels; i++ {
		level := &PriceLevel{}
		PriceLevelPool.Put(level)
	}

	// Pre-warm OrderChunkPool
	for i := 0; i < orderChunks; i++ {
		chunk := &orderChunk{}
		OrderChunkPool.Put(chunk)
	}

	// Pre-warm SafePriceLevelPool
	for i := 0; i < safePriceLevels; i++ {
		level := &SafePriceLevel{
			orders: make([]*model.Order, 0, 4),
		}
		SafePriceLevelPool.Put(level)
	}

	// Pre-warm enhanced snapshot slice pool
	for i := 0; i < snapshotSlices; i++ {
		slice := make([]topLevel, 0, 100)
		enhancedSnapshotSlicePool.Put(slice)
	}

	fmt.Printf("Order book pools pre-warmed: %d price levels, %d order chunks, %d safe price levels, %d snapshot slices\n",
		priceLevels, orderChunks, safePriceLevels, snapshotSlices)
}

// GetAllPoolMetrics returns comprehensive metrics for all pools
func GetAllPoolMetrics() map[string]OrderBookPoolMetrics {
	return map[string]OrderBookPoolMetrics{
		"price_level":      GetPriceLevelPoolMetrics(),
		"order_chunk":      GetOrderChunkPoolMetrics(),
		"safe_price_level": GetSafePriceLevelPoolMetrics(),
		"snapshot_slice":   GetSnapshotSlicePoolMetrics(),
	}
}

// CalculatePoolHitRate calculates hit rate percentage for a pool
func CalculatePoolHitRate(metrics OrderBookPoolMetrics) float64 {
	if metrics.Gets == 0 {
		return 0.0
	}
	return float64(metrics.Hits) / float64(metrics.Gets) * 100.0
}
