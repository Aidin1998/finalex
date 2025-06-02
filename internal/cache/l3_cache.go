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

// L3Cache implements long-term Redis caching with advanced compression
type L3Cache struct {
	client         redis.UniversalClient
	defaultTTL     time.Duration
	alwaysCompress bool
	keyPrefix      string

	// Statistics
	hits               int64
	misses             int64
	sets               int64
	deletes            int64
	errors             int64
	compressionSavings int64 // Bytes saved through compression
}

// l3Item represents an item in the L3 cache with metadata
type l3Item struct {
	Data           []byte            `json:"data"`
	Compressed     bool              `json:"compressed"`
	OriginalSize   int64             `json:"original_size"`
	CompressedSize int64             `json:"compressed_size"`
	CreatedAt      time.Time         `json:"created_at"`
	TTL            int64             `json:"ttl"` // TTL in seconds
	AccessCount    int64             `json:"access_count"`
	LastAccessed   time.Time         `json:"last_accessed"`
	Metadata       map[string]string `json:"metadata,omitempty"`
	Version        int               `json:"version"`
}

// NewL3Cache creates a new L3 cache instance
func NewL3Cache(client redis.UniversalClient, defaultTTL time.Duration, alwaysCompress bool) *L3Cache {
	return &L3Cache{
		client:         client,
		defaultTTL:     defaultTTL,
		alwaysCompress: alwaysCompress,
		keyPrefix:      "orbit:cache:l3:",
	}
}

// Get retrieves a value from the L3 cache
func (c *L3Cache) Get(ctx context.Context, key string) (interface{}, bool, error) {
	redisKey := c.keyPrefix + key

	data, err := c.client.Get(ctx, redisKey).Result()
	if err != nil {
		if err == redis.Nil {
			atomic.AddInt64(&c.misses, 1)
			return nil, false, nil
		}
		atomic.AddInt64(&c.errors, 1)
		return nil, false, fmt.Errorf("failed to get from L3 cache: %w", err)
	}

	// Deserialize the cached item
	var item l3Item
	if err := json.Unmarshal([]byte(data), &item); err != nil {
		atomic.AddInt64(&c.errors, 1)
		return nil, false, fmt.Errorf("failed to unmarshal L3 cache item: %w", err)
	}

	// Update access metadata in background (fire and forget)
	go c.updateAccessMetadata(ctx, redisKey, &item)

	// Decompress if necessary
	var valueData []byte
	if item.Compressed {
		valueData, err = c.decompressAdvanced(item.Data)
		if err != nil {
			atomic.AddInt64(&c.errors, 1)
			return nil, false, fmt.Errorf("failed to decompress L3 cache data: %w", err)
		}
	} else {
		valueData = item.Data
	}

	// Deserialize the actual value
	var value interface{}
	if err := json.Unmarshal(valueData, &value); err != nil {
		atomic.AddInt64(&c.errors, 1)
		return nil, false, fmt.Errorf("failed to unmarshal L3 cache value: %w", err)
	}

	atomic.AddInt64(&c.hits, 1)
	return value, true, nil
}

// Set stores a value in the L3 cache
func (c *L3Cache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	return c.SetWithMetadata(ctx, key, value, ttl, nil)
}

// SetWithMetadata stores a value with custom metadata in the L3 cache
func (c *L3Cache) SetWithMetadata(ctx context.Context, key string, value interface{}, ttl time.Duration, metadata map[string]string) error {
	if ttl == 0 {
		ttl = c.defaultTTL
	}

	// Serialize the value
	valueData, err := json.Marshal(value)
	if err != nil {
		atomic.AddInt64(&c.errors, 1)
		return fmt.Errorf("failed to marshal value for L3 cache: %w", err)
	}

	originalSize := int64(len(valueData))
	var finalData []byte
	var compressed bool
	var compressedSize int64

	// Always compress for L3 if enabled, or compress large items
	if c.alwaysCompress || originalSize > 512 {
		finalData, err = c.compressAdvanced(valueData)
		if err != nil {
			atomic.AddInt64(&c.errors, 1)
			return fmt.Errorf("failed to compress L3 cache data: %w", err)
		}
		compressed = true
		compressedSize = int64(len(finalData))

		// Track compression savings
		savings := originalSize - compressedSize
		if savings > 0 {
			atomic.AddInt64(&c.compressionSavings, savings)
		}
	} else {
		finalData = valueData
		compressed = false
		compressedSize = originalSize
	}

	// Create cache item
	item := l3Item{
		Data:           finalData,
		Compressed:     compressed,
		OriginalSize:   originalSize,
		CompressedSize: compressedSize,
		CreatedAt:      time.Now(),
		TTL:            int64(ttl.Seconds()),
		AccessCount:    0,
		LastAccessed:   time.Now(),
		Metadata:       metadata,
		Version:        1,
	}

	// Serialize the cache item
	itemData, err := json.Marshal(item)
	if err != nil {
		atomic.AddInt64(&c.errors, 1)
		return fmt.Errorf("failed to marshal L3 cache item: %w", err)
	}

	// Store in Redis
	redisKey := c.keyPrefix + key
	if err := c.client.Set(ctx, redisKey, itemData, ttl).Err(); err != nil {
		atomic.AddInt64(&c.errors, 1)
		return fmt.Errorf("failed to set L3 cache in Redis: %w", err)
	}

	atomic.AddInt64(&c.sets, 1)
	return nil
}

// Delete removes a value from the L3 cache
func (c *L3Cache) Delete(ctx context.Context, key string) error {
	redisKey := c.keyPrefix + key

	if err := c.client.Del(ctx, redisKey).Err(); err != nil {
		atomic.AddInt64(&c.errors, 1)
		return fmt.Errorf("failed to delete from L3 cache: %w", err)
	}

	atomic.AddInt64(&c.deletes, 1)
	return nil
}

// GetMetadata retrieves metadata for a cached item without the actual value
func (c *L3Cache) GetMetadata(ctx context.Context, key string) (*l3Item, error) {
	redisKey := c.keyPrefix + key

	data, err := c.client.Get(ctx, redisKey).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		atomic.AddInt64(&c.errors, 1)
		return nil, fmt.Errorf("failed to get metadata from L3 cache: %w", err)
	}

	var item l3Item
	if err := json.Unmarshal([]byte(data), &item); err != nil {
		atomic.AddInt64(&c.errors, 1)
		return nil, fmt.Errorf("failed to unmarshal L3 cache metadata: %w", err)
	}

	return &item, nil
}

// UpdateTTL extends the TTL for a key without affecting the data
func (c *L3Cache) UpdateTTL(ctx context.Context, key string, newTTL time.Duration) error {
	redisKey := c.keyPrefix + key

	if err := c.client.Expire(ctx, redisKey, newTTL).Err(); err != nil {
		atomic.AddInt64(&c.errors, 1)
		return fmt.Errorf("failed to update TTL in L3 cache: %w", err)
	}

	return nil
}

// GetKeys returns all keys matching a pattern
func (c *L3Cache) GetKeys(ctx context.Context, pattern string) ([]string, error) {
	redisPattern := c.keyPrefix + pattern
	var cursor uint64
	var keys []string

	for {
		var batch []string
		var err error
		batch, cursor, err = c.client.Scan(ctx, cursor, redisPattern, 100).Result()
		if err != nil {
			atomic.AddInt64(&c.errors, 1)
			return nil, fmt.Errorf("failed to scan keys in L3 cache: %w", err)
		}

		// Remove prefix from keys
		for _, key := range batch {
			if len(key) > len(c.keyPrefix) {
				keys = append(keys, key[len(c.keyPrefix):])
			}
		}

		if cursor == 0 {
			break
		}
	}

	return keys, nil
}

// BulkDelete removes multiple keys from the L3 cache
func (c *L3Cache) BulkDelete(ctx context.Context, keys []string) error {
	if len(keys) == 0 {
		return nil
	}

	redisKeys := make([]string, len(keys))
	for i, key := range keys {
		redisKeys[i] = c.keyPrefix + key
	}

	if err := c.client.Del(ctx, redisKeys...).Err(); err != nil {
		atomic.AddInt64(&c.errors, 1)
		return fmt.Errorf("failed to bulk delete from L3 cache: %w", err)
	}

	atomic.AddInt64(&c.deletes, int64(len(keys)))
	return nil
}

// GetCompressionStats returns compression statistics
func (c *L3Cache) GetCompressionStats() map[string]interface{} {
	return map[string]interface{}{
		"total_savings_bytes": atomic.LoadInt64(&c.compressionSavings),
		"always_compress":     c.alwaysCompress,
	}
}

// Stats returns cache statistics
func (c *L3Cache) Stats() *LevelStats {
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
	}
}

// Vacuum removes expired entries and optimizes the cache
func (c *L3Cache) Vacuum(ctx context.Context) error {
	pattern := c.keyPrefix + "*"
	var cursor uint64
	var expiredKeys []string

	for {
		var keys []string
		var err error
		keys, cursor, err = c.client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			atomic.AddInt64(&c.errors, 1)
			return fmt.Errorf("failed to scan for vacuum in L3 cache: %w", err)
		}

		for _, key := range keys {
			ttl, err := c.client.TTL(ctx, key).Result()
			if err != nil {
				continue
			}

			// Mark keys that will expire soon
			if ttl < time.Minute {
				expiredKeys = append(expiredKeys, key)
			}
		}

		if cursor == 0 {
			break
		}
	}

	// Remove expired keys
	if len(expiredKeys) > 0 {
		if err := c.client.Del(ctx, expiredKeys...).Err(); err != nil {
			atomic.AddInt64(&c.errors, 1)
			return fmt.Errorf("failed to vacuum expired keys from L3 cache: %w", err)
		}
		atomic.AddInt64(&c.deletes, int64(len(expiredKeys)))
	}

	return nil
}

// Helper methods

// updateAccessMetadata updates access count and timestamp
func (c *L3Cache) updateAccessMetadata(ctx context.Context, redisKey string, item *l3Item) {
	item.AccessCount++
	item.LastAccessed = time.Now()

	// Serialize and update
	if itemData, err := json.Marshal(item); err == nil {
		// Update with same TTL
		ttl := time.Duration(item.TTL) * time.Second
		c.client.Set(ctx, redisKey, itemData, ttl)
	}
}

// compressAdvanced compresses data using gzip with optimal settings
func (c *L3Cache) compressAdvanced(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	writer, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return nil, err
	}

	if _, err := writer.Write(data); err != nil {
		return nil, err
	}

	if err := writer.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// decompressAdvanced decompresses gzip data
func (c *L3Cache) decompressAdvanced(data []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	return io.ReadAll(reader)
}

// Clear removes all L3 cache entries (use with extreme caution)
func (c *L3Cache) Clear(ctx context.Context) error {
	pattern := c.keyPrefix + "*"
	var cursor uint64
	var allKeys []string

	for {
		var keys []string
		var err error
		keys, cursor, err = c.client.Scan(ctx, cursor, pattern, 1000).Result()
		if err != nil {
			atomic.AddInt64(&c.errors, 1)
			return fmt.Errorf("failed to scan for L3 cache clear: %w", err)
		}

		allKeys = append(allKeys, keys...)

		if cursor == 0 {
			break
		}
	}

	// Delete in batches
	batchSize := 100
	for i := 0; i < len(allKeys); i += batchSize {
		end := i + batchSize
		if end > len(allKeys) {
			end = len(allKeys)
		}

		batch := allKeys[i:end]
		if err := c.client.Del(ctx, batch...).Err(); err != nil {
			atomic.AddInt64(&c.errors, 1)
			return fmt.Errorf("failed to delete L3 cache batch: %w", err)
		}
	}

	atomic.AddInt64(&c.deletes, int64(len(allKeys)))
	return nil
}
