package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
	"go.uber.org/zap"
)

// ClusterConfig holds Redis cluster configuration
type ClusterConfig struct {
	Addrs        []string      `yaml:"addrs"`
	Password     string        `yaml:"password"`
	MaxRetries   int           `yaml:"max_retries"`
	PoolSize     int           `yaml:"pool_size"`
	MinIdleConns int           `yaml:"min_idle_conns"`
	DialTimeout  time.Duration `yaml:"dial_timeout"`
	ReadTimeout  time.Duration `yaml:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout"`
	MaxConnAge   time.Duration `yaml:"max_conn_age"`
}

// LocalCacheConfig holds local cache configuration
type LocalCacheConfig struct {
	MaxSize         int           `yaml:"max_size"`
	TTL             time.Duration `yaml:"ttl"`
	CleanupInterval time.Duration `yaml:"cleanup_interval"`
}

// CacheItem represents a cached item with metadata
type CacheItem struct {
	Data      interface{} `json:"data"`
	ExpiresAt time.Time   `json:"expires_at"`
	CreatedAt time.Time   `json:"created_at"`
}

// ClusteredCache provides high-performance caching with Redis cluster and local cache
type ClusteredCache struct {
	redisCluster *redis.ClusterClient
	localCache   *LocalCache
	logger       *zap.Logger
	namespace    string
}

// LocalCache provides in-memory caching with LRU eviction
type LocalCache struct {
	items       map[string]*CacheItem
	accessOrder []string
	mutex       sync.RWMutex
	maxSize     int
	ttl         time.Duration
	cleanupDone chan bool
}

// NewClusteredCache creates a new clustered cache instance
func NewClusteredCache(clusterConfig ClusterConfig, localConfig LocalCacheConfig, namespace string, logger *zap.Logger) (*ClusteredCache, error) {
	// Initialize Redis cluster client
	rdb := redis.NewClusterClient(&redis.ClusterOptions{
		Addrs:        clusterConfig.Addrs,
		Password:     clusterConfig.Password,
		MaxRetries:   clusterConfig.MaxRetries,
		PoolSize:     clusterConfig.PoolSize,
		MinIdleConns: clusterConfig.MinIdleConns,
		DialTimeout:  clusterConfig.DialTimeout,
		ReadTimeout:  clusterConfig.ReadTimeout,
		WriteTimeout: clusterConfig.WriteTimeout,
		MaxConnAge:   clusterConfig.MaxConnAge,
	})

	// Test cluster connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		logger.Warn("Redis cluster connection failed, cache will operate in local-only mode", zap.Error(err))
	}

	// Initialize local cache
	localCache := &LocalCache{
		items:       make(map[string]*CacheItem),
		accessOrder: make([]string, 0),
		maxSize:     localConfig.MaxSize,
		ttl:         localConfig.TTL,
		cleanupDone: make(chan bool),
	}

	// Start cleanup goroutine
	go localCache.startCleanup(localConfig.CleanupInterval)

	return &ClusteredCache{
		redisCluster: rdb,
		localCache:   localCache,
		logger:       logger,
		namespace:    namespace,
	}, nil
}

// Get retrieves a value from cache (local first, then Redis)
func (c *ClusteredCache) Get(ctx context.Context, key string) (interface{}, bool, error) {
	fullKey := c.getFullKey(key)

	// Try local cache first
	if value, found := c.localCache.Get(key); found {
		c.logger.Debug("Cache hit (local)", zap.String("key", key))
		return value, true, nil
	}

	// Try Redis cluster
	data, err := c.redisCluster.Get(ctx, fullKey).Result()
	if err == redis.Nil {
		c.logger.Debug("Cache miss", zap.String("key", key))
		return nil, false, nil
	} else if err != nil {
		c.logger.Warn("Redis get error", zap.String("key", key), zap.Error(err))
		return nil, false, err
	}

	// Deserialize and store in local cache
	var item CacheItem
	if err := json.Unmarshal([]byte(data), &item); err != nil {
		c.logger.Warn("Failed to deserialize cache item", zap.String("key", key), zap.Error(err))
		return nil, false, err
	}

	// Check if item has expired
	if time.Now().After(item.ExpiresAt) {
		c.logger.Debug("Cache item expired", zap.String("key", key))
		// Clean up expired item asynchronously
		go c.Delete(context.Background(), key)
		return nil, false, nil
	}

	// Store in local cache for future requests
	c.localCache.Set(key, item.Data, c.localCache.ttl)

	c.logger.Debug("Cache hit (Redis)", zap.String("key", key))
	return item.Data, true, nil
}

// Set stores a value in both local and Redis cache
func (c *ClusteredCache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	fullKey := c.getFullKey(key)

	// Create cache item
	item := CacheItem{
		Data:      value,
		ExpiresAt: time.Now().Add(ttl),
		CreatedAt: time.Now(),
	}

	// Serialize for Redis
	data, err := json.Marshal(item)
	if err != nil {
		c.logger.Warn("Failed to serialize cache item", zap.String("key", key), zap.Error(err))
		return err
	}

	// Store in Redis cluster
	if err := c.redisCluster.Set(ctx, fullKey, data, ttl).Err(); err != nil {
		c.logger.Warn("Redis set error", zap.String("key", key), zap.Error(err))
		// Continue with local cache even if Redis fails
	}

	// Store in local cache
	c.localCache.Set(key, value, ttl)

	c.logger.Debug("Cache set", zap.String("key", key), zap.Duration("ttl", ttl))
	return nil
}

// Delete removes a value from both caches
func (c *ClusteredCache) Delete(ctx context.Context, key string) error {
	fullKey := c.getFullKey(key)

	// Delete from Redis cluster
	if err := c.redisCluster.Del(ctx, fullKey).Err(); err != nil {
		c.logger.Warn("Redis delete error", zap.String("key", key), zap.Error(err))
	}

	// Delete from local cache
	c.localCache.Delete(key)

	c.logger.Debug("Cache delete", zap.String("key", key))
	return nil
}

// GetMulti retrieves multiple values efficiently
func (c *ClusteredCache) GetMulti(ctx context.Context, keys []string) (map[string]interface{}, error) {
	result := make(map[string]interface{})
	missingKeys := make([]string, 0)

	// Check local cache first
	for _, key := range keys {
		if value, found := c.localCache.Get(key); found {
			result[key] = value
		} else {
			missingKeys = append(missingKeys, key)
		}
	}

	if len(missingKeys) == 0 {
		return result, nil
	}

	// Get missing keys from Redis
	fullKeys := make([]string, len(missingKeys))
	for i, key := range missingKeys {
		fullKeys[i] = c.getFullKey(key)
	}

	pipe := c.redisCluster.Pipeline()
	cmds := make([]*redis.StringCmd, len(fullKeys))
	for i, fullKey := range fullKeys {
		cmds[i] = pipe.Get(ctx, fullKey)
	}

	_, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		c.logger.Warn("Redis pipeline error", zap.Error(err))
		return result, err
	}

	// Process results
	for i, cmd := range cmds {
		if cmd.Err() == nil {
			var item CacheItem
			if err := json.Unmarshal([]byte(cmd.Val()), &item); err == nil {
				if time.Now().Before(item.ExpiresAt) {
					key := missingKeys[i]
					result[key] = item.Data
					c.localCache.Set(key, item.Data, c.localCache.ttl)
				}
			}
		}
	}

	return result, nil
}

// SetMulti stores multiple values efficiently
func (c *ClusteredCache) SetMulti(ctx context.Context, items map[string]interface{}, ttl time.Duration) error {
	pipe := c.redisCluster.Pipeline()

	for key, value := range items {
		fullKey := c.getFullKey(key)

		item := CacheItem{
			Data:      value,
			ExpiresAt: time.Now().Add(ttl),
			CreatedAt: time.Now(),
		}

		if data, err := json.Marshal(item); err == nil {
			pipe.Set(ctx, fullKey, data, ttl)
			c.localCache.Set(key, value, ttl)
		}
	}

	_, err := pipe.Exec(ctx)
	if err != nil {
		c.logger.Warn("Redis pipeline set error", zap.Error(err))
	}

	return err
}

// Flush clears all cache entries
func (c *ClusteredCache) Flush(ctx context.Context) error {
	// Clear local cache
	c.localCache.Flush()

	// Clear Redis namespace
	pattern := c.namespace + ":*"
	iter := c.redisCluster.Scan(ctx, 0, pattern, 0).Iterator()
	keys := make([]string, 0)

	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
		if len(keys) >= 1000 { // Batch delete
			if err := c.redisCluster.Del(ctx, keys...).Err(); err != nil {
				c.logger.Warn("Redis batch delete error", zap.Error(err))
			}
			keys = keys[:0]
		}
	}

	if len(keys) > 0 {
		if err := c.redisCluster.Del(ctx, keys...).Err(); err != nil {
			c.logger.Warn("Redis final delete error", zap.Error(err))
		}
	}

	return iter.Err()
}

// GetStats returns cache statistics
func (c *ClusteredCache) GetStats() map[string]interface{} {
	stats := make(map[string]interface{})

	// Local cache stats
	c.localCache.mutex.RLock()
	stats["local_cache_size"] = len(c.localCache.items)
	stats["local_cache_max_size"] = c.localCache.maxSize
	c.localCache.mutex.RUnlock()

	// Redis cluster stats
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	info, err := c.redisCluster.Info(ctx).Result()
	if err == nil {
		stats["redis_info"] = info
	}

	return stats
}

// Close closes the cache connections
func (c *ClusteredCache) Close() error {
	c.localCache.cleanup()
	return c.redisCluster.Close()
}

// getFullKey creates a namespaced key
func (c *ClusteredCache) getFullKey(key string) string {
	return fmt.Sprintf("%s:%s", c.namespace, key)
}

// Local cache methods

// Get retrieves a value from local cache
func (lc *LocalCache) Get(key string) (interface{}, bool) {
	lc.mutex.RLock()
	defer lc.mutex.RUnlock()

	item, exists := lc.items[key]
	if !exists {
		return nil, false
	}

	// Check expiration
	if time.Now().After(item.ExpiresAt) {
		return nil, false
	}

	// Update access order
	lc.updateAccessOrder(key)
	return item.Data, true
}

// Set stores a value in local cache
func (lc *LocalCache) Set(key string, value interface{}, ttl time.Duration) {
	lc.mutex.Lock()
	defer lc.mutex.Unlock()

	// Check if we need to evict
	if len(lc.items) >= lc.maxSize {
		lc.evictLRU()
	}

	lc.items[key] = &CacheItem{
		Data:      value,
		ExpiresAt: time.Now().Add(ttl),
		CreatedAt: time.Now(),
	}

	lc.updateAccessOrder(key)
}

// Delete removes a value from local cache
func (lc *LocalCache) Delete(key string) {
	lc.mutex.Lock()
	defer lc.mutex.Unlock()

	delete(lc.items, key)
	lc.removeFromAccessOrder(key)
}

// Flush clears all items from local cache
func (lc *LocalCache) Flush() {
	lc.mutex.Lock()
	defer lc.mutex.Unlock()

	lc.items = make(map[string]*CacheItem)
	lc.accessOrder = lc.accessOrder[:0]
}

// updateAccessOrder updates the access order for LRU
func (lc *LocalCache) updateAccessOrder(key string) {
	// Remove if exists
	lc.removeFromAccessOrder(key)
	// Add to end
	lc.accessOrder = append(lc.accessOrder, key)
}

// removeFromAccessOrder removes a key from access order
func (lc *LocalCache) removeFromAccessOrder(key string) {
	for i, k := range lc.accessOrder {
		if k == key {
			lc.accessOrder = append(lc.accessOrder[:i], lc.accessOrder[i+1:]...)
			break
		}
	}
}

// evictLRU removes the least recently used item
func (lc *LocalCache) evictLRU() {
	if len(lc.accessOrder) > 0 {
		key := lc.accessOrder[0]
		delete(lc.items, key)
		lc.accessOrder = lc.accessOrder[1:]
	}
}

// startCleanup starts the background cleanup routine
func (lc *LocalCache) startCleanup(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			lc.cleanupExpired()
		case <-lc.cleanupDone:
			return
		}
	}
}

// cleanupExpired removes expired items
func (lc *LocalCache) cleanupExpired() {
	lc.mutex.Lock()
	defer lc.mutex.Unlock()

	now := time.Now()
	for key, item := range lc.items {
		if now.After(item.ExpiresAt) {
			delete(lc.items, key)
			lc.removeFromAccessOrder(key)
		}
	}
}

// cleanup stops the cleanup routine
func (lc *LocalCache) cleanup() {
	close(lc.cleanupDone)
}
