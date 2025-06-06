// Package infrastructure provides high-performance caching for cross-module operations
package infrastructure

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// CacheManager provides distributed caching with TTL support
type CacheManager interface {
	// Basic operations
	Get(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)

	// Batch operations
	GetMulti(ctx context.Context, keys []string) (map[string][]byte, error)
	SetMulti(ctx context.Context, items map[string]CacheItem) error
	DeleteMulti(ctx context.Context, keys []string) error

	// Pattern operations
	DeletePattern(ctx context.Context, pattern string) error
	GetKeys(ctx context.Context, pattern string) ([]string, error)

	// Cache management
	Clear(ctx context.Context) error
	Size(ctx context.Context) (int64, error)
	Stats(ctx context.Context) (*CacheStats, error)

	// Lifecycle
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	HealthCheck(ctx context.Context) error
}

// CacheItem represents an item to be cached
type CacheItem struct {
	Key   string        `json:"key"`
	Value []byte        `json:"value"`
	TTL   time.Duration `json:"ttl"`
}

// CacheStats provides cache statistics
type CacheStats struct {
	Size        int64     `json:"size"`
	Hits        int64     `json:"hits"`
	Misses      int64     `json:"misses"`
	HitRate     float64   `json:"hit_rate"`
	Evictions   int64     `json:"evictions"`
	LastCleanup time.Time `json:"last_cleanup"`
}

// CacheEntry represents an internal cache entry
type CacheEntry struct {
	Value       []byte    `json:"value"`
	ExpiresAt   time.Time `json:"expires_at"`
	CreatedAt   time.Time `json:"created_at"`
	AccessCount int64     `json:"access_count"`
	LastAccess  time.Time `json:"last_access"`
}

// InMemoryCache provides an in-memory cache implementation with LRU eviction
type InMemoryCache struct {
	logger  *zap.Logger
	data    map[string]*CacheEntry
	mu      sync.RWMutex
	running bool

	// Statistics
	stats struct {
		hits      int64
		misses    int64
		evictions int64
		mu        sync.RWMutex
	}

	// Configuration
	maxSize         int64
	cleanupInterval time.Duration
	defaultTTL      time.Duration

	// Cleanup goroutine
	cleanupCtx    context.Context
	cleanupCancel context.CancelFunc
	cleanupWG     sync.WaitGroup
}

// NewInMemoryCache creates a new in-memory cache
func NewInMemoryCache(logger *zap.Logger, maxSize int64, defaultTTL time.Duration) CacheManager {
	return &InMemoryCache{
		logger:          logger,
		data:            make(map[string]*CacheEntry),
		maxSize:         maxSize,
		cleanupInterval: time.Minute * 5,
		defaultTTL:      defaultTTL,
	}
}

func (c *InMemoryCache) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		return nil
	}

	c.running = true
	c.cleanupCtx, c.cleanupCancel = context.WithCancel(ctx)

	// Start cleanup goroutine
	c.cleanupWG.Add(1)
	go c.cleanupExpiredEntries()

	c.logger.Info("Cache Manager started")
	return nil
}

func (c *InMemoryCache) Stop(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running {
		return nil
	}

	c.running = false
	c.cleanupCancel()
	c.cleanupWG.Wait()

	c.logger.Info("Cache Manager stopped")
	return nil
}

func (c *InMemoryCache) Get(ctx context.Context, key string) ([]byte, error) {
	c.mu.RLock()
	entry, exists := c.data[key]
	c.mu.RUnlock()

	if !exists {
		c.incrementMisses()
		return nil, fmt.Errorf("key not found: %s", key)
	}

	// Check expiration
	if time.Now().After(entry.ExpiresAt) {
		c.mu.Lock()
		delete(c.data, key)
		c.mu.Unlock()
		c.incrementMisses()
		return nil, fmt.Errorf("key expired: %s", key)
	}

	// Update access statistics
	entry.AccessCount++
	entry.LastAccess = time.Now()

	c.incrementHits()
	return entry.Value, nil
}

func (c *InMemoryCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running {
		return fmt.Errorf("cache not running")
	}

	if ttl <= 0 {
		ttl = c.defaultTTL
	}

	now := time.Now()
	entry := &CacheEntry{
		Value:       value,
		ExpiresAt:   now.Add(ttl),
		CreatedAt:   now,
		AccessCount: 0,
		LastAccess:  now,
	}

	// Check if we need to evict entries
	if int64(len(c.data)) >= c.maxSize {
		c.evictLRU()
	}

	c.data[key] = entry
	return nil
}

func (c *InMemoryCache) Delete(ctx context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.data, key)
	return nil
}

func (c *InMemoryCache) Exists(ctx context.Context, key string) (bool, error) {
	c.mu.RLock()
	entry, exists := c.data[key]
	c.mu.RUnlock()

	if !exists {
		return false, nil
	}

	// Check expiration
	if time.Now().After(entry.ExpiresAt) {
		c.mu.Lock()
		delete(c.data, key)
		c.mu.Unlock()
		return false, nil
	}

	return true, nil
}

func (c *InMemoryCache) GetMulti(ctx context.Context, keys []string) (map[string][]byte, error) {
	result := make(map[string][]byte)

	for _, key := range keys {
		if value, err := c.Get(ctx, key); err == nil {
			result[key] = value
		}
	}

	return result, nil
}

func (c *InMemoryCache) SetMulti(ctx context.Context, items map[string]CacheItem) error {
	for _, item := range items {
		if err := c.Set(ctx, item.Key, item.Value, item.TTL); err != nil {
			return fmt.Errorf("failed to set key %s: %w", item.Key, err)
		}
	}
	return nil
}

func (c *InMemoryCache) DeleteMulti(ctx context.Context, keys []string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, key := range keys {
		delete(c.data, key)
	}
	return nil
}

func (c *InMemoryCache) DeletePattern(ctx context.Context, pattern string) error {
	keys, err := c.GetKeys(ctx, pattern)
	if err != nil {
		return err
	}

	return c.DeleteMulti(ctx, keys)
}

func (c *InMemoryCache) GetKeys(ctx context.Context, pattern string) ([]string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var keys []string

	// Simple wildcard matching (* at end)
	if pattern == "*" {
		for key := range c.data {
			keys = append(keys, key)
		}
	} else if len(pattern) > 0 && pattern[len(pattern)-1] == '*' {
		prefix := pattern[:len(pattern)-1]
		for key := range c.data {
			if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
				keys = append(keys, key)
			}
		}
	} else {
		// Exact match
		if _, exists := c.data[pattern]; exists {
			keys = append(keys, pattern)
		}
	}

	return keys, nil
}

func (c *InMemoryCache) Clear(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.data = make(map[string]*CacheEntry)
	return nil
}

func (c *InMemoryCache) Size(ctx context.Context) (int64, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return int64(len(c.data)), nil
}

func (c *InMemoryCache) Stats(ctx context.Context) (*CacheStats, error) {
	c.stats.mu.RLock()
	defer c.stats.mu.RUnlock()

	size, _ := c.Size(ctx)

	var hitRate float64
	total := c.stats.hits + c.stats.misses
	if total > 0 {
		hitRate = float64(c.stats.hits) / float64(total)
	}

	return &CacheStats{
		Size:      size,
		Hits:      c.stats.hits,
		Misses:    c.stats.misses,
		HitRate:   hitRate,
		Evictions: c.stats.evictions,
	}, nil
}

func (c *InMemoryCache) HealthCheck(ctx context.Context) error {
	c.mu.RLock()
	running := c.running
	c.mu.RUnlock()

	if !running {
		return fmt.Errorf("cache not running")
	}

	return nil
}

func (c *InMemoryCache) incrementHits() {
	c.stats.mu.Lock()
	c.stats.hits++
	c.stats.mu.Unlock()
}

func (c *InMemoryCache) incrementMisses() {
	c.stats.mu.Lock()
	c.stats.misses++
	c.stats.mu.Unlock()
}

func (c *InMemoryCache) incrementEvictions() {
	c.stats.mu.Lock()
	c.stats.evictions++
	c.stats.mu.Unlock()
}

func (c *InMemoryCache) evictLRU() {
	// Find least recently used entry
	var lruKey string
	var lruTime time.Time
	first := true

	for key, entry := range c.data {
		if first || entry.LastAccess.Before(lruTime) {
			lruKey = key
			lruTime = entry.LastAccess
			first = false
		}
	}

	if lruKey != "" {
		delete(c.data, lruKey)
		c.incrementEvictions()
	}
}

func (c *InMemoryCache) cleanupExpiredEntries() {
	defer c.cleanupWG.Done()

	ticker := time.NewTicker(c.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.performCleanup()
		case <-c.cleanupCtx.Done():
			return
		}
	}
}

func (c *InMemoryCache) performCleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	var expiredKeys []string

	for key, entry := range c.data {
		if now.After(entry.ExpiresAt) {
			expiredKeys = append(expiredKeys, key)
		}
	}

	for _, key := range expiredKeys {
		delete(c.data, key)
	}

	if len(expiredKeys) > 0 {
		c.logger.Debug("Cleaned up expired cache entries",
			zap.Int("count", len(expiredKeys)))
	}
}

// Cache key builders for different modules
type CacheKeys struct{}

func (CacheKeys) UserAuthToken(token string) string {
	return fmt.Sprintf("userauth:token:%s", token)
}

func (CacheKeys) UserAuthAPIKey(apiKey string) string {
	return fmt.Sprintf("userauth:apikey:%s", apiKey)
}

func (CacheKeys) UserAuthPermissions(userID uuid.UUID) string {
	return fmt.Sprintf("userauth:permissions:%s", userID)
}

func (CacheKeys) UserAuthKYC(userID uuid.UUID) string {
	return fmt.Sprintf("userauth:kyc:%s", userID)
}

func (CacheKeys) AccountsBalance(userID uuid.UUID, currency string) string {
	return fmt.Sprintf("accounts:balance:%s:%s", userID, currency)
}

func (CacheKeys) AccountsUserBalances(userID uuid.UUID) string {
	return fmt.Sprintf("accounts:balances:%s", userID)
}

func (CacheKeys) TradingOrderBook(symbol string) string {
	return fmt.Sprintf("trading:orderbook:%s", symbol)
}

func (CacheKeys) TradingUserOrders(userID uuid.UUID) string {
	return fmt.Sprintf("trading:orders:%s", userID)
}

func (CacheKeys) TradingRiskProfile(userID uuid.UUID) string {
	return fmt.Sprintf("trading:risk:%s", userID)
}

func (CacheKeys) FiatRates(currency string) string {
	return fmt.Sprintf("fiat:rates:%s", currency)
}

func (CacheKeys) FiatLimits(userID uuid.UUID) string {
	return fmt.Sprintf("fiat:limits:%s", userID)
}

func (CacheKeys) FiatBankAccount(userID uuid.UUID, accountID uuid.UUID) string {
	return fmt.Sprintf("fiat:bankaccount:%s:%s", userID, accountID)
}

// Typed cache operations for better type safety
type TypedCache struct {
	cache CacheManager
}

func NewTypedCache(cache CacheManager) *TypedCache {
	return &TypedCache{cache: cache}
}

func (tc *TypedCache) GetTyped(ctx context.Context, key string, dest interface{}) error {
	data, err := tc.cache.Get(ctx, key)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, dest)
}

func (tc *TypedCache) SetTyped(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal value: %w", err)
	}

	return tc.cache.Set(ctx, key, data, ttl)
}
