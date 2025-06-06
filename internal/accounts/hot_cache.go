// HotCache provides sub-millisecond cache access for ultra-high frequency operations
package accounts

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
	"unsafe"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// HotCacheEntry represents a single cache entry with metadata
type HotCacheEntry struct {
	Key          string        `json:"key"`
	Value        interface{}   `json:"value"`
	Size         int64         `json:"size"`
	CreatedAt    time.Time     `json:"created_at"`
	LastAccess   time.Time     `json:"last_access"`
	AccessCount  int64         `json:"access_count"`
	TTL          time.Duration `json:"ttl"`
	IsCompressed bool          `json:"is_compressed"`
}

// HotCacheStats represents cache statistics
type HotCacheStats struct {
	Size         int64         `json:"size"`
	Count        int64         `json:"count"`
	HitRate      float64       `json:"hit_rate"`
	MissRate     float64       `json:"miss_rate"`
	EvictionRate float64       `json:"eviction_rate"`
	MemoryUsage  int64         `json:"memory_usage"`
	AvgLatency   time.Duration `json:"avg_latency"`
}

// HotCache provides ultra-low latency in-memory caching
type HotCache struct {
	// Core storage
	data map[string]*HotCacheEntry
	mu   sync.RWMutex

	// Configuration
	maxSize    int64
	defaultTTL time.Duration
	logger     *zap.Logger

	// Performance tracking
	stats   HotCacheStats
	statsMu sync.RWMutex

	// Metrics
	metrics *HotCacheMetrics

	// Background cleanup
	cleanupTicker *time.Ticker
	stopCleanup   chan struct{}

	// LRU tracking
	accessOrder []string
	accessMap   map[string]int // key -> index in accessOrder
}

// HotCacheMetrics holds Prometheus metrics for hot cache
type HotCacheMetrics struct {
	CacheHits      prometheus.Counter
	CacheMisses    prometheus.Counter
	CacheEvictions prometheus.Counter
	CacheSize      prometheus.Gauge
	CacheMemory    prometheus.Gauge
	AccessLatency  prometheus.Histogram
	HitRate        prometheus.Gauge
	EvictionRate   prometheus.Gauge
}

// NewHotCache creates a new high-performance hot cache
func NewHotCache(maxSize int64, defaultTTL time.Duration, logger *zap.Logger) (*HotCache, error) {
	if maxSize <= 0 {
		return nil, fmt.Errorf("maxSize must be positive, got %d", maxSize)
	}

	metrics := &HotCacheMetrics{
		CacheHits: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "account_hot_cache_hits_total",
				Help: "Total number of hot cache hits",
			},
		),
		CacheMisses: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "account_hot_cache_misses_total",
				Help: "Total number of hot cache misses",
			},
		),
		CacheEvictions: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "account_hot_cache_evictions_total",
				Help: "Total number of hot cache evictions",
			},
		),
		CacheSize: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "account_hot_cache_size",
				Help: "Current number of entries in hot cache",
			},
		),
		CacheMemory: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "account_hot_cache_memory_bytes",
				Help: "Current memory usage of hot cache in bytes",
			},
		),
		AccessLatency: promauto.NewHistogram(
			prometheus.HistogramOpts{
				Name:    "account_hot_cache_access_latency_seconds",
				Help:    "Latency of hot cache access operations",
				Buckets: []float64{0.00001, 0.00005, 0.0001, 0.0005, 0.001, 0.005, 0.01}, // 10μs to 10ms
			},
		),
		HitRate: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "account_hot_cache_hit_rate",
				Help: "Hit rate of hot cache (0-1)",
			},
		),
		EvictionRate: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "account_hot_cache_eviction_rate",
				Help: "Eviction rate of hot cache (evictions per second)",
			},
		),
	}

	hc := &HotCache{
		data:        make(map[string]*HotCacheEntry),
		maxSize:     maxSize,
		defaultTTL:  defaultTTL,
		logger:      logger,
		metrics:     metrics,
		stopCleanup: make(chan struct{}),
		accessOrder: make([]string, 0, maxSize),
		accessMap:   make(map[string]int),
	}

	// Start background cleanup
	hc.cleanupTicker = time.NewTicker(1 * time.Second)
	go hc.backgroundCleanup()

	return hc, nil
}

// Get retrieves a value from the cache with sub-millisecond performance
func (hc *HotCache) Get(ctx context.Context, key string) (interface{}, bool) {
	start := time.Now()
	defer func() {
		hc.metrics.AccessLatency.Observe(time.Since(start).Seconds())
	}()

	hc.mu.RLock()
	entry, exists := hc.data[key]
	hc.mu.RUnlock()

	if !exists {
		hc.metrics.CacheMisses.Inc()
		hc.updateStats(false, false)
		return nil, false
	}

	// Check TTL
	if entry.TTL > 0 && time.Since(entry.CreatedAt) > entry.TTL {
		// Entry expired, remove it
		hc.Delete(ctx, key)
		hc.metrics.CacheMisses.Inc()
		hc.updateStats(false, false)
		return nil, false
	}

	// Update access tracking
	hc.mu.Lock()
	entry.LastAccess = time.Now()
	entry.AccessCount++
	hc.updateAccessOrder(key)
	hc.mu.Unlock()

	hc.metrics.CacheHits.Inc()
	hc.updateStats(true, false)

	return entry.Value, true
}

// Set stores a value in the cache
func (hc *HotCache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	if ttl == 0 {
		ttl = hc.defaultTTL
	}

	// Calculate size
	size := hc.calculateSize(value)

	entry := &HotCacheEntry{
		Key:         key,
		Value:       value,
		Size:        size,
		CreatedAt:   time.Now(),
		LastAccess:  time.Now(),
		AccessCount: 1,
		TTL:         ttl,
	}

	hc.mu.Lock()
	defer hc.mu.Unlock()

	// Check if we need to evict
	hc.ensureCapacity(size)

	// Store the entry
	oldEntry, existed := hc.data[key]
	hc.data[key] = entry

	if existed {
		// Update stats for replacement
		hc.stats.MemoryUsage = hc.stats.MemoryUsage - oldEntry.Size + size
	} else {
		// New entry
		hc.stats.Count++
		hc.stats.MemoryUsage += size
		hc.addToAccessOrder(key)
	}

	hc.updateAccessOrder(key)
	hc.updateMetricsLocked()

	return nil
}

// Delete removes a value from the cache
func (hc *HotCache) Delete(ctx context.Context, key string) bool {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	entry, existed := hc.data[key]
	if !existed {
		return false
	}

	delete(hc.data, key)
	hc.removeFromAccessOrder(key)

	hc.stats.Count--
	hc.stats.MemoryUsage -= entry.Size
	hc.updateMetricsLocked()

	return true
}

// GetAccount retrieves an account from hot cache
func (hc *HotCache) GetAccount(ctx context.Context, userID uuid.UUID, currency string) (*Account, bool) {
	key := fmt.Sprintf("account:%s:%s", userID.String(), currency)
	value, exists := hc.Get(ctx, key)
	if !exists {
		return nil, false
	}

	account, ok := value.(*Account)
	if !ok {
		// Invalid type, remove from cache
		hc.Delete(ctx, key)
		return nil, false
	}

	return account, true
}

// SetAccount stores an account in hot cache
func (hc *HotCache) SetAccount(ctx context.Context, userID uuid.UUID, currency string, account *Account, ttl time.Duration) error {
	key := fmt.Sprintf("account:%s:%s", userID.String(), currency)
	return hc.Set(ctx, key, account, ttl)
}

// GetBalance retrieves account balance from hot cache
func (hc *HotCache) GetBalance(ctx context.Context, userID uuid.UUID, currency string) (decimal.Decimal, bool) {
	key := fmt.Sprintf("balance:%s:%s", userID.String(), currency)
	value, exists := hc.Get(ctx, key)
	if !exists {
		return decimal.Zero, false
	}

	balance, ok := value.(decimal.Decimal)
	if !ok {
		// Try to convert from string
		if str, isString := value.(string); isString {
			if bal, err := decimal.NewFromString(str); err == nil {
				return bal, true
			}
		}
		// Invalid type, remove from cache
		hc.Delete(ctx, key)
		return decimal.Zero, false
	}

	return balance, true
}

// SetBalance stores account balance in hot cache
func (hc *HotCache) SetBalance(ctx context.Context, userID uuid.UUID, currency string, balance decimal.Decimal, ttl time.Duration) error {
	key := fmt.Sprintf("balance:%s:%s", userID.String(), currency)
	return hc.Set(ctx, key, balance, ttl)
}

// GetStats returns current cache statistics
func (hc *HotCache) GetStats() HotCacheStats {
	hc.statsMu.RLock()
	defer hc.statsMu.RUnlock()
	return hc.stats
}

// Clear removes all entries from the cache
func (hc *HotCache) Clear(ctx context.Context) {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	hc.data = make(map[string]*HotCacheEntry)
	hc.accessOrder = hc.accessOrder[:0]
	hc.accessMap = make(map[string]int)

	hc.stats.Count = 0
	hc.stats.MemoryUsage = 0
	hc.updateMetricsLocked()
}

// Close stops the cache and cleanup background tasks
func (hc *HotCache) Close() error {
	close(hc.stopCleanup)
	if hc.cleanupTicker != nil {
		hc.cleanupTicker.Stop()
	}
	return nil
}

// ensureCapacity ensures there's enough capacity for a new entry
func (hc *HotCache) ensureCapacity(newEntrySize int64) {
	for hc.stats.Count >= hc.maxSize || (hc.stats.MemoryUsage+newEntrySize) > (hc.maxSize*1024) {
		// Evict least recently used entry
		if len(hc.accessOrder) == 0 {
			break
		}

		lruKey := hc.accessOrder[0]
		if entry, exists := hc.data[lruKey]; exists {
			delete(hc.data, lruKey)
			hc.removeFromAccessOrder(lruKey)
			hc.stats.Count--
			hc.stats.MemoryUsage -= entry.Size
			hc.metrics.CacheEvictions.Inc()
			hc.updateStats(false, true)
		}
	}
}

// updateAccessOrder updates the LRU order for a key
func (hc *HotCache) updateAccessOrder(key string) {
	if index, exists := hc.accessMap[key]; exists {
		// Move to end (most recently used)
		hc.accessOrder = append(hc.accessOrder[:index], hc.accessOrder[index+1:]...)
		hc.accessOrder = append(hc.accessOrder, key)

		// Update indices
		for i := index; i < len(hc.accessOrder); i++ {
			hc.accessMap[hc.accessOrder[i]] = i
		}
	}
}

// addToAccessOrder adds a new key to the access order
func (hc *HotCache) addToAccessOrder(key string) {
	hc.accessOrder = append(hc.accessOrder, key)
	hc.accessMap[key] = len(hc.accessOrder) - 1
}

// removeFromAccessOrder removes a key from the access order
func (hc *HotCache) removeFromAccessOrder(key string) {
	if index, exists := hc.accessMap[key]; exists {
		hc.accessOrder = append(hc.accessOrder[:index], hc.accessOrder[index+1:]...)
		delete(hc.accessMap, key)

		// Update indices for remaining keys
		for i := index; i < len(hc.accessOrder); i++ {
			hc.accessMap[hc.accessOrder[i]] = i
		}
	}
}

// calculateSize estimates the memory size of a value
func (hc *HotCache) calculateSize(value interface{}) int64 {
	switch v := value.(type) {
	case string:
		return int64(len(v))
	case []byte:
		return int64(len(v))
	case *Account:
		return int64(unsafe.Sizeof(*v)) + int64(len(v.Currency)) + int64(len(v.AccountType)) + int64(len(v.Status))
	case decimal.Decimal:
		return int64(unsafe.Sizeof(v))
	default:
		// Estimate using JSON serialization
		if data, err := json.Marshal(value); err == nil {
			return int64(len(data))
		}
		return 256 // Default estimate
	}
}

// updateStats updates cache statistics
func (hc *HotCache) updateStats(hit, eviction bool) {
	hc.statsMu.Lock()
	defer hc.statsMu.Unlock()

	// Update hit/miss rates
	totalOps := float64(hc.metrics.CacheHits.Get() + hc.metrics.CacheMisses.Get())
	if totalOps > 0 {
		hc.stats.HitRate = float64(hc.metrics.CacheHits.Get()) / totalOps
		hc.stats.MissRate = float64(hc.metrics.CacheMisses.Get()) / totalOps
	}

	// Update eviction rate
	if eviction {
		hc.stats.EvictionRate = float64(hc.metrics.CacheEvictions.Get()) / time.Since(time.Now().Add(-time.Minute)).Seconds()
	}
}

// updateMetricsLocked updates Prometheus metrics (caller must hold lock)
func (hc *HotCache) updateMetricsLocked() {
	hc.metrics.CacheSize.Set(float64(hc.stats.Count))
	hc.metrics.CacheMemory.Set(float64(hc.stats.MemoryUsage))
	hc.metrics.HitRate.Set(hc.stats.HitRate)
	hc.metrics.EvictionRate.Set(hc.stats.EvictionRate)
}

// backgroundCleanup removes expired entries periodically
func (hc *HotCache) backgroundCleanup() {
	for {
		select {
		case <-hc.stopCleanup:
			return
		case <-hc.cleanupTicker.C:
			hc.cleanupExpiredEntries()
		}
	}
}

// cleanupExpiredEntries removes expired entries from the cache
func (hc *HotCache) cleanupExpiredEntries() {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	now := time.Now()
	keysToDelete := make([]string, 0)

	for key, entry := range hc.data {
		if entry.TTL > 0 && now.Sub(entry.CreatedAt) > entry.TTL {
			keysToDelete = append(keysToDelete, key)
		}
	}

	for _, key := range keysToDelete {
		if entry, exists := hc.data[key]; exists {
			delete(hc.data, key)
			hc.removeFromAccessOrder(key)
			hc.stats.Count--
			hc.stats.MemoryUsage -= entry.Size
		}
	}

	if len(keysToDelete) > 0 {
		hc.updateMetricsLocked()
	}
}
