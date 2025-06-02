package cache

import (
	"container/list"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

// L1Cache implements an ultra-fast in-memory cache with LRU eviction
type L1Cache struct {
	// Map for O(1) lookups
	items map[string]*l1Item

	// LRU list for eviction policy
	lruList *list.List

	// Memory management
	maxSize     int64 // Maximum memory usage in bytes
	currentSize int64 // Current memory usage in bytes

	// TTL management
	defaultTTL time.Duration

	// Synchronization
	mu sync.RWMutex

	// Statistics
	hits      int64
	misses    int64
	sets      int64
	deletes   int64
	evictions int64
	itemCount int64

	// Background cleanup
	cleanupTicker *time.Ticker
	stopCleanup   chan struct{}
}

// l1Item represents an item in the L1 cache
type l1Item struct {
	key        string
	value      interface{}
	size       int64
	expiresAt  time.Time
	accessedAt time.Time
	listElem   *list.Element // Pointer to LRU list element
}

// NewL1Cache creates a new L1 cache instance
func NewL1Cache(maxSize int64, defaultTTL time.Duration) *L1Cache {
	cache := &L1Cache{
		items:         make(map[string]*l1Item),
		lruList:       list.New(),
		maxSize:       maxSize,
		defaultTTL:    defaultTTL,
		stopCleanup:   make(chan struct{}),
		cleanupTicker: time.NewTicker(time.Minute), // Cleanup every minute
	}

	// Start background cleanup goroutine
	go cache.cleanupLoop()

	return cache
}

// Get retrieves a value from the L1 cache
func (c *L1Cache) Get(key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	item, exists := c.items[key]
	if !exists {
		atomic.AddInt64(&c.misses, 1)
		return nil, false
	}

	// Check if item has expired
	if time.Now().After(item.expiresAt) {
		atomic.AddInt64(&c.misses, 1)
		// Don't delete here to avoid lock upgrade, let cleanup handle it
		return nil, false
	}

	// Update access time and move to front of LRU list
	item.accessedAt = time.Now()
	c.lruList.MoveToFront(item.listElem)

	atomic.AddInt64(&c.hits, 1)
	return item.value, true
}

// Set stores a value in the L1 cache
func (c *L1Cache) Set(key string, value interface{}, ttl time.Duration) {
	if ttl == 0 {
		ttl = c.defaultTTL
	}

	// Calculate item size
	itemSize := c.calculateSize(key, value)

	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	expiresAt := now.Add(ttl)

	// Check if item already exists
	if existing, exists := c.items[key]; exists {
		// Update existing item
		c.currentSize -= existing.size
		existing.value = value
		existing.size = itemSize
		existing.expiresAt = expiresAt
		existing.accessedAt = now
		c.currentSize += itemSize

		// Move to front of LRU list
		c.lruList.MoveToFront(existing.listElem)
	} else {
		// Create new item
		item := &l1Item{
			key:        key,
			value:      value,
			size:       itemSize,
			expiresAt:  expiresAt,
			accessedAt: now,
		}

		// Add to LRU list
		item.listElem = c.lruList.PushFront(item)

		// Add to map
		c.items[key] = item
		c.currentSize += itemSize
		atomic.AddInt64(&c.itemCount, 1)
	}

	atomic.AddInt64(&c.sets, 1)

	// Evict items if necessary
	c.evictIfNeeded()
}

// Delete removes an item from the L1 cache
func (c *L1Cache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if item, exists := c.items[key]; exists {
		c.removeItem(item)
		atomic.AddInt64(&c.deletes, 1)
	}
}

// Clear removes all items from the cache
func (c *L1Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[string]*l1Item)
	c.lruList = list.New()
	atomic.StoreInt64(&c.currentSize, 0)
	atomic.StoreInt64(&c.itemCount, 0)
}

// Stats returns cache statistics
func (c *L1Cache) Stats() *LevelStats {
	hits := atomic.LoadInt64(&c.hits)
	misses := atomic.LoadInt64(&c.misses)
	total := hits + misses
	hitRate := 0.0
	if total > 0 {
		hitRate = float64(hits) / float64(total)
	}

	return &LevelStats{
		Hits:        hits,
		Misses:      misses,
		Sets:        atomic.LoadInt64(&c.sets),
		Deletes:     atomic.LoadInt64(&c.deletes),
		Evictions:   atomic.LoadInt64(&c.evictions),
		Size:        atomic.LoadInt64(&c.currentSize),
		ItemCount:   atomic.LoadInt64(&c.itemCount),
		HitRate:     hitRate,
		MemoryUsage: atomic.LoadInt64(&c.currentSize),
	}
}

// Close shuts down the L1 cache
func (c *L1Cache) Close() {
	close(c.stopCleanup)
	c.cleanupTicker.Stop()
}

// Helper methods

// removeItem removes an item from the cache (must be called with lock held)
func (c *L1Cache) removeItem(item *l1Item) {
	delete(c.items, item.key)
	c.lruList.Remove(item.listElem)
	c.currentSize -= item.size
	atomic.AddInt64(&c.itemCount, -1)
}

// evictIfNeeded evicts items if cache exceeds max size (must be called with lock held)
func (c *L1Cache) evictIfNeeded() {
	for c.currentSize > c.maxSize && c.lruList.Len() > 0 {
		// Remove least recently used item
		elem := c.lruList.Back()
		if elem != nil {
			item := elem.Value.(*l1Item)
			c.removeItem(item)
			atomic.AddInt64(&c.evictions, 1)
		}
	}
}

// calculateSize estimates the memory size of a key-value pair
func (c *L1Cache) calculateSize(key string, value interface{}) int64 {
	size := int64(len(key)) // Key size

	// Estimate value size based on type
	switch v := value.(type) {
	case string:
		size += int64(len(v))
	case []byte:
		size += int64(len(v))
	case int, int32, int64, float32, float64, bool:
		size += 8 // Approximate size for basic types
	default:
		// For complex types, use a rough estimate
		size += int64(unsafe.Sizeof(v)) + 64 // Base overhead
	}

	// Add overhead for item structure
	size += int64(unsafe.Sizeof(l1Item{}))

	return size
}

// cleanupLoop runs background cleanup for expired items
func (c *L1Cache) cleanupLoop() {
	for {
		select {
		case <-c.cleanupTicker.C:
			c.cleanupExpired()
		case <-c.stopCleanup:
			return
		}
	}
}

// cleanupExpired removes expired items from the cache
func (c *L1Cache) cleanupExpired() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	var toRemove []*l1Item

	// Collect expired items
	for _, item := range c.items {
		if now.After(item.expiresAt) {
			toRemove = append(toRemove, item)
		}
	}

	// Remove expired items
	for _, item := range toRemove {
		c.removeItem(item)
		atomic.AddInt64(&c.evictions, 1)
	}
}

// Keys returns all keys in the cache (for debugging/monitoring)
func (c *L1Cache) Keys() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	keys := make([]string, 0, len(c.items))
	for key := range c.items {
		keys = append(keys, key)
	}
	return keys
}

// Size returns the current memory usage
func (c *L1Cache) Size() int64 {
	return atomic.LoadInt64(&c.currentSize)
}

// ItemCount returns the number of items in the cache
func (c *L1Cache) ItemCount() int64 {
	return atomic.LoadInt64(&c.itemCount)
}

// MaxSize returns the maximum allowed memory usage
func (c *L1Cache) MaxSize() int64 {
	return c.maxSize
}

// SetMaxSize updates the maximum allowed memory usage
func (c *L1Cache) SetMaxSize(maxSize int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.maxSize = maxSize
	c.evictIfNeeded()
}
