package cache

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

// L2Cache implements Redis-based distributed caching with compression
type L2Cache struct {
	client         redis.UniversalClient
	defaultTTL     time.Duration
	compressionMin int64 // Minimum size for compression
	keyPrefix      string

	// Statistics
	hits    int64
	misses  int64
	sets    int64
	deletes int64
	errors  int64

	// Batch operations
	batchSize int
}

// l2Item represents an item in the L2 cache
type l2Item struct {
	Data       []byte    `json:"data"`
	Compressed bool      `json:"compressed"`
	CreatedAt  time.Time `json:"created_at"`
	TTL        int64     `json:"ttl"` // TTL in seconds
	Size       int64     `json:"size"`
}

// NewL2Cache creates a new L2 cache instance
func NewL2Cache(client redis.UniversalClient, defaultTTL time.Duration, compressionMin int64) *L2Cache {
	return &L2Cache{
		client:         client,
		defaultTTL:     defaultTTL,
		compressionMin: compressionMin,
		keyPrefix:      "orbit:cache:l2:",
		batchSize:      100,
	}
}

// Get retrieves a value from the L2 cache
func (c *L2Cache) Get(ctx context.Context, key string) (interface{}, bool, error) {
	redisKey := c.keyPrefix + key

	data, err := c.client.Get(ctx, redisKey).Result()
	if err != nil {
		if err == redis.Nil {
			atomic.AddInt64(&c.misses, 1)
			return nil, false, nil
		}
		atomic.AddInt64(&c.errors, 1)
		return nil, false, fmt.Errorf("failed to get from L2 cache: %w", err)
	}

	// Deserialize the cached item
	var item l2Item
	if err := json.Unmarshal([]byte(data), &item); err != nil {
		atomic.AddInt64(&c.errors, 1)
		return nil, false, fmt.Errorf("failed to unmarshal L2 cache item: %w", err)
	}

	// Decompress if necessary
	var valueData []byte
	if item.Compressed {
		valueData, err = c.decompress(item.Data)
		if err != nil {
			atomic.AddInt64(&c.errors, 1)
			return nil, false, fmt.Errorf("failed to decompress L2 cache data: %w", err)
		}
	} else {
		valueData = item.Data
	}

	// Deserialize the actual value
	var value interface{}
	if err := json.Unmarshal(valueData, &value); err != nil {
		atomic.AddInt64(&c.errors, 1)
		return nil, false, fmt.Errorf("failed to unmarshal L2 cache value: %w", err)
	}

	atomic.AddInt64(&c.hits, 1)
	return value, true, nil
}

// Set stores a value in the L2 cache
func (c *L2Cache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	if ttl == 0 {
		ttl = c.defaultTTL
	}

	// Serialize the value
	valueData, err := json.Marshal(value)
	if err != nil {
		atomic.AddInt64(&c.errors, 1)
		return fmt.Errorf("failed to marshal value for L2 cache: %w", err)
	}

	// Compress if data is large enough
	var finalData []byte
	var compressed bool
	if int64(len(valueData)) >= c.compressionMin {
		finalData, err = c.compress(valueData)
		if err != nil {
			atomic.AddInt64(&c.errors, 1)
			return fmt.Errorf("failed to compress L2 cache data: %w", err)
		}
		compressed = true
	} else {
		finalData = valueData
		compressed = false
	}

	// Create cache item
	item := l2Item{
		Data:       finalData,
		Compressed: compressed,
		CreatedAt:  time.Now(),
		TTL:        int64(ttl.Seconds()),
		Size:       int64(len(finalData)),
	}

	// Serialize the cache item
	itemData, err := json.Marshal(item)
	if err != nil {
		atomic.AddInt64(&c.errors, 1)
		return fmt.Errorf("failed to marshal L2 cache item: %w", err)
	}

	// Store in Redis
	redisKey := c.keyPrefix + key
	if err := c.client.Set(ctx, redisKey, itemData, ttl).Err(); err != nil {
		atomic.AddInt64(&c.errors, 1)
		return fmt.Errorf("failed to set L2 cache in Redis: %w", err)
	}

	atomic.AddInt64(&c.sets, 1)
	return nil
}

// Delete removes a value from the L2 cache
func (c *L2Cache) Delete(ctx context.Context, key string) error {
	redisKey := c.keyPrefix + key

	if err := c.client.Del(ctx, redisKey).Err(); err != nil {
		atomic.AddInt64(&c.errors, 1)
		return fmt.Errorf("failed to delete from L2 cache: %w", err)
	}

	atomic.AddInt64(&c.deletes, 1)
	return nil
}

// DeletePattern removes all keys matching a pattern
func (c *L2Cache) DeletePattern(ctx context.Context, pattern string) error {
	redisPattern := c.keyPrefix + pattern

	// Use SCAN to find matching keys
	var cursor uint64
	var keys []string

	for {
		var batch []string
		var err error
		batch, cursor, err = c.client.Scan(ctx, cursor, redisPattern, 100).Result()
		if err != nil {
			atomic.AddInt64(&c.errors, 1)
			return fmt.Errorf("failed to scan for pattern in L2 cache: %w", err)
		}

		keys = append(keys, batch...)

		if cursor == 0 {
			break
		}
	}

	// Delete keys in batches
	if len(keys) > 0 {
		if err := c.client.Del(ctx, keys...).Err(); err != nil {
			atomic.AddInt64(&c.errors, 1)
			return fmt.Errorf("failed to delete pattern keys from L2 cache: %w", err)
		}
		atomic.AddInt64(&c.deletes, int64(len(keys)))
	}

	return nil
}

// Exists checks if a key exists in the L2 cache
func (c *L2Cache) Exists(ctx context.Context, key string) (bool, error) {
	redisKey := c.keyPrefix + key

	count, err := c.client.Exists(ctx, redisKey).Result()
	if err != nil {
		atomic.AddInt64(&c.errors, 1)
		return false, fmt.Errorf("failed to check existence in L2 cache: %w", err)
	}

	return count > 0, nil
}

// TTL returns the remaining TTL for a key
func (c *L2Cache) TTL(ctx context.Context, key string) (time.Duration, error) {
	redisKey := c.keyPrefix + key

	ttl, err := c.client.TTL(ctx, redisKey).Result()
	if err != nil {
		atomic.AddInt64(&c.errors, 1)
		return 0, fmt.Errorf("failed to get TTL from L2 cache: %w", err)
	}

	return ttl, nil
}

// Extend extends the TTL for a key
func (c *L2Cache) Extend(ctx context.Context, key string, extension time.Duration) error {
	redisKey := c.keyPrefix + key

	if err := c.client.Expire(ctx, redisKey, extension).Err(); err != nil {
		atomic.AddInt64(&c.errors, 1)
		return fmt.Errorf("failed to extend TTL in L2 cache: %w", err)
	}

	return nil
}

// MGet retrieves multiple values from the L2 cache
func (c *L2Cache) MGet(ctx context.Context, keys []string) (map[string]interface{}, error) {
	if len(keys) == 0 {
		return make(map[string]interface{}), nil
	}

	// Convert to Redis keys
	redisKeys := make([]string, len(keys))
	for i, key := range keys {
		redisKeys[i] = c.keyPrefix + key
	}

	// Get all values
	values, err := c.client.MGet(ctx, redisKeys...).Result()
	if err != nil {
		atomic.AddInt64(&c.errors, 1)
		return nil, fmt.Errorf("failed to mget from L2 cache: %w", err)
	}

	// Process results
	result := make(map[string]interface{})
	for i, value := range values {
		if value == nil {
			atomic.AddInt64(&c.misses, 1)
			continue
		}

		// Deserialize the cached item
		var item l2Item
		if err := json.Unmarshal([]byte(value.(string)), &item); err != nil {
			atomic.AddInt64(&c.errors, 1)
			continue
		}

		// Decompress if necessary
		var valueData []byte
		if item.Compressed {
			valueData, err = c.decompress(item.Data)
			if err != nil {
				atomic.AddInt64(&c.errors, 1)
				continue
			}
		} else {
			valueData = item.Data
		}

		// Deserialize the actual value
		var actualValue interface{}
		if err := json.Unmarshal(valueData, &actualValue); err != nil {
			atomic.AddInt64(&c.errors, 1)
			continue
		}

		result[keys[i]] = actualValue
		atomic.AddInt64(&c.hits, 1)
	}

	return result, nil
}

// MSet stores multiple values in the L2 cache
func (c *L2Cache) MSet(ctx context.Context, items map[string]interface{}, ttl time.Duration) error {
	if len(items) == 0 {
		return nil
	}

	if ttl == 0 {
		ttl = c.defaultTTL
	}

	// Process in batches
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}

	for i := 0; i < len(keys); i += c.batchSize {
		end := i + c.batchSize
		if end > len(keys) {
			end = len(keys)
		}

		batchKeys := keys[i:end]
		if err := c.msetBatch(ctx, batchKeys, items, ttl); err != nil {
			return err
		}
	}

	return nil
}

// msetBatch processes a batch of items for MSet
func (c *L2Cache) msetBatch(ctx context.Context, keys []string, items map[string]interface{}, ttl time.Duration) error {
	pipe := c.client.Pipeline()

	for _, key := range keys {
		value := items[key]

		// Serialize the value
		valueData, err := json.Marshal(value)
		if err != nil {
			atomic.AddInt64(&c.errors, 1)
			return fmt.Errorf("failed to marshal value for L2 cache batch: %w", err)
		}

		// Compress if data is large enough
		var finalData []byte
		var compressed bool
		if int64(len(valueData)) >= c.compressionMin {
			finalData, err = c.compress(valueData)
			if err != nil {
				atomic.AddInt64(&c.errors, 1)
				return fmt.Errorf("failed to compress L2 cache batch data: %w", err)
			}
			compressed = true
		} else {
			finalData = valueData
			compressed = false
		}

		// Create cache item
		item := l2Item{
			Data:       finalData,
			Compressed: compressed,
			CreatedAt:  time.Now(),
			TTL:        int64(ttl.Seconds()),
			Size:       int64(len(finalData)),
		}

		// Serialize the cache item
		itemData, err := json.Marshal(item)
		if err != nil {
			atomic.AddInt64(&c.errors, 1)
			return fmt.Errorf("failed to marshal L2 cache batch item: %w", err)
		}

		// Add to pipeline
		redisKey := c.keyPrefix + key
		pipe.Set(ctx, redisKey, itemData, ttl)
	}

	// Execute pipeline
	if _, err := pipe.Exec(ctx); err != nil {
		atomic.AddInt64(&c.errors, 1)
		return fmt.Errorf("failed to execute L2 cache batch pipeline: %w", err)
	}

	atomic.AddInt64(&c.sets, int64(len(keys)))
	return nil
}

// Stats returns cache statistics
func (c *L2Cache) Stats() *LevelStats {
	hits := atomic.LoadInt64(&c.hits)
	misses := atomic.LoadInt64(&c.misses)
	total := hits + misses
	hitRate := 0.0
	if total > 0 {
		hitRate = float64(hits) / float64(total)
	}

	return &LevelStats{
		Hits:    hits,
		Misses:  misses,
		Sets:    atomic.LoadInt64(&c.sets),
		Deletes: atomic.LoadInt64(&c.deletes),
		HitRate: hitRate,
		// Note: Size and ItemCount would require additional Redis operations
		// They can be implemented separately if needed
	}
}

// compress compresses data using gzip
func (c *L2Cache) compress(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	writer := gzip.NewWriter(&buf)

	if _, err := writer.Write(data); err != nil {
		return nil, err
	}

	if err := writer.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// decompress decompresses gzip data
func (c *L2Cache) decompress(data []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	return io.ReadAll(reader)
}

// Clear removes all L2 cache entries (use with caution)
func (c *L2Cache) Clear(ctx context.Context) error {
	pattern := c.keyPrefix + "*"

	// Use SCAN to find all keys
	var cursor uint64
	var allKeys []string

	for {
		var keys []string
		var err error
		keys, cursor, err = c.client.Scan(ctx, cursor, pattern, 1000).Result()
		if err != nil {
			atomic.AddInt64(&c.errors, 1)
			return fmt.Errorf("failed to scan for L2 cache clear: %w", err)
		}

		allKeys = append(allKeys, keys...)

		if cursor == 0 {
			break
		}
	}

	// Delete in batches
	for i := 0; i < len(allKeys); i += c.batchSize {
		end := i + c.batchSize
		if end > len(allKeys) {
			end = len(allKeys)
		}

		batch := allKeys[i:end]
		if err := c.client.Del(ctx, batch...).Err(); err != nil {
			atomic.AddInt64(&c.errors, 1)
			return fmt.Errorf("failed to delete L2 cache batch: %w", err)
		}
	}

	atomic.AddInt64(&c.deletes, int64(len(allKeys)))
	return nil
}
