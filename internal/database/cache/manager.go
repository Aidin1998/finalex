// Package cache provides a comprehensive multi-level caching system
// with intelligent invalidation and cache warming procedures
package cache

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// CacheLevel represents different cache levels
type CacheLevel int

const (
	CacheLevelL1 CacheLevel = iota // In-memory local cache (fastest)
	CacheLevelL2                   // Redis distributed cache
	CacheLevelL3                   // Compressed Redis cache for large data
)

// CacheEntry represents a cached item with metadata
type CacheEntry struct {
	Key         string        `json:"key"`
	Value       interface{}   `json:"value"`
	TTL         time.Duration `json:"ttl"`
	Level       CacheLevel    `json:"level"`
	CreatedAt   time.Time     `json:"created_at"`
	AccessedAt  time.Time     `json:"accessed_at"`
	AccessCount int64         `json:"access_count"`
	Size        int64         `json:"size"`
	Compressed  bool          `json:"compressed"`
}

// CacheStats represents cache statistics
type CacheStats struct {
	L1Stats *LevelStats `json:"l1_stats"`
	L2Stats *LevelStats `json:"l2_stats"`
	L3Stats *LevelStats `json:"l3_stats"`
	Total   *LevelStats `json:"total"`
}

// LevelStats represents statistics for a cache level
type LevelStats struct {
	Hits        int64   `json:"hits"`
	Misses      int64   `json:"misses"`
	Sets        int64   `json:"sets"`
	Deletes     int64   `json:"deletes"`
	Evictions   int64   `json:"evictions"`
	Size        int64   `json:"size"`
	ItemCount   int64   `json:"item_count"`
	HitRate     float64 `json:"hit_rate"`
	MemoryUsage int64   `json:"memory_usage"`
}

// InvalidationEvent represents a cache invalidation event
type InvalidationEvent struct {
	Type      InvalidationType `json:"type"`
	Keys      []string         `json:"keys"`
	Pattern   string           `json:"pattern,omitempty"`
	Reason    string           `json:"reason"`
	Timestamp time.Time        `json:"timestamp"`
}

// InvalidationType defines types of cache invalidation
type InvalidationType int

const (
	InvalidateKey InvalidationType = iota
	InvalidatePattern
	InvalidateTag
	InvalidateAll
)

// CacheManager provides unified cache management across multiple levels
type CacheManager struct {
	// Cache levels
	l1Cache *L1Cache
	l2Cache *L2Cache
	l3Cache *L3Cache

	// Configuration
	config *ManagerConfig

	// Statistics
	stats atomic.Value // *CacheStats

	// Event handling
	invalidationChan chan InvalidationEvent
	warmingChan      chan WarmingRequest

	// Background workers
	cleanupTicker *time.Ticker
	statsTicker   *time.Ticker
	healthTicker  *time.Ticker
	stopChan      chan struct{}
	wg            sync.WaitGroup

	// Redis client
	redis  redis.UniversalClient
	logger *zap.SugaredLogger
}

// ManagerConfig holds cache manager configuration
type ManagerConfig struct {
	// L1 Cache settings
	L1MaxSize         int64         `yaml:"l1_max_size" json:"l1_max_size"`
	L1DefaultTTL      time.Duration `yaml:"l1_default_ttl" json:"l1_default_ttl"`
	L1CleanupInterval time.Duration `yaml:"l1_cleanup_interval" json:"l1_cleanup_interval"`

	// L2 Cache settings
	L2DefaultTTL     time.Duration `yaml:"l2_default_ttl" json:"l2_default_ttl"`
	L2CompressionMin int64         `yaml:"l2_compression_min" json:"l2_compression_min"`
	L2BatchSize      int           `yaml:"l2_batch_size" json:"l2_batch_size"`

	// L3 Cache settings
	L3DefaultTTL     time.Duration `yaml:"l3_default_ttl" json:"l3_default_ttl"`
	L3AlwaysCompress bool          `yaml:"l3_always_compress" json:"l3_always_compress"`

	// Background worker settings
	StatsInterval   time.Duration `yaml:"stats_interval" json:"stats_interval"`
	HealthInterval  time.Duration `yaml:"health_interval" json:"health_interval"`
	InvalidationBuf int           `yaml:"invalidation_buffer" json:"invalidation_buffer"`
	WarmingBuf      int           `yaml:"warming_buffer" json:"warming_buffer"`
	WorkerCount     int           `yaml:"worker_count" json:"worker_count"`

	// Advanced features
	EnablePredictive bool    `yaml:"enable_predictive" json:"enable_predictive"`
	EnableWarming    bool    `yaml:"enable_warming" json:"enable_warming"`
	EnableStats      bool    `yaml:"enable_stats" json:"enable_stats"`
	StatsSampleRate  float64 `yaml:"stats_sample_rate" json:"stats_sample_rate"`
}

// DefaultManagerConfig returns default cache manager configuration
func DefaultManagerConfig() *ManagerConfig {
	return &ManagerConfig{
		// L1 settings - ultra-fast in-memory cache
		L1MaxSize:         100 * 1024 * 1024, // 100MB
		L1DefaultTTL:      time.Minute * 5,
		L1CleanupInterval: time.Minute,

		// L2 settings - Redis distributed cache
		L2DefaultTTL:     time.Minute * 15,
		L2CompressionMin: 1024, // Compress data > 1KB
		L2BatchSize:      100,

		// L3 settings - Compressed long-term cache
		L3DefaultTTL:     time.Hour,
		L3AlwaysCompress: true,

		// Background workers
		StatsInterval:   time.Minute,
		HealthInterval:  time.Second * 30,
		InvalidationBuf: 1000,
		WarmingBuf:      100,
		WorkerCount:     4,

		// Advanced features
		EnablePredictive: true,
		EnableWarming:    true,
		EnableStats:      true,
		StatsSampleRate:  0.1, // Sample 10% of operations
	}
}

// NewCacheManager creates a new cache manager instance
func NewCacheManager(config *ManagerConfig, redisClient redis.UniversalClient, logger *zap.SugaredLogger) (*CacheManager, error) {
	if config == nil {
		config = DefaultManagerConfig()
	}

	// Initialize cache levels
	l1Cache := NewL1Cache(config.L1MaxSize, config.L1DefaultTTL)
	l2Cache := NewL2Cache(redisClient, config.L2DefaultTTL, config.L2CompressionMin)
	l3Cache := NewL3Cache(redisClient, config.L3DefaultTTL, config.L3AlwaysCompress)

	cm := &CacheManager{
		l1Cache:          l1Cache,
		l2Cache:          l2Cache,
		l3Cache:          l3Cache,
		config:           config,
		invalidationChan: make(chan InvalidationEvent, config.InvalidationBuf),
		warmingChan:      make(chan WarmingRequest, config.WarmingBuf),
		stopChan:         make(chan struct{}),
		redis:            redisClient,
		logger:           logger,
	}

	// Initialize statistics
	cm.stats.Store(&CacheStats{
		L1Stats: &LevelStats{},
		L2Stats: &LevelStats{},
		L3Stats: &LevelStats{},
		Total:   &LevelStats{},
	})

	// Start background workers
	if err := cm.startBackgroundWorkers(); err != nil {
		return nil, fmt.Errorf("failed to start background workers: %w", err)
	}

	logger.Infow("Cache manager initialized",
		"l1_max_size", config.L1MaxSize,
		"l2_compression_min", config.L2CompressionMin,
		"l3_always_compress", config.L3AlwaysCompress,
		"worker_count", config.WorkerCount,
	)

	return cm, nil
}

// Get retrieves a value from the cache hierarchy (L1 -> L2 -> L3)
func (cm *CacheManager) Get(ctx context.Context, key string) (interface{}, bool, error) {
	start := time.Now()
	defer func() {
		if cm.config.EnableStats {
			cm.recordOperation("get", time.Since(start))
		}
	}()
	// Try L1 cache first
	if value, found := cm.l1Cache.Get(key); found {
		cm.recordHit(CacheLevelL1)
		return value, true, nil
	}
	cm.recordMiss(CacheLevelL1)

	// Try L2 cache
	if value, found, err := cm.l2Cache.Get(ctx, key); err != nil {
		cm.logger.Warnw("L2 cache error", "key", key, "error", err)
	} else if found {
		cm.recordHit(CacheLevelL2)
		// Promote to L1 for faster future access
		cm.l1Cache.Set(key, value, cm.config.L1DefaultTTL)
		return value, true, nil
	}
	cm.recordMiss(CacheLevelL2)

	// Try L3 cache
	if value, found, err := cm.l3Cache.Get(ctx, key); err != nil {
		cm.logger.Warnw("L3 cache error", "key", key, "error", err)
	} else if found {
		cm.recordHit(CacheLevelL3)
		// Promote to higher levels
		cm.l1Cache.Set(key, value, cm.config.L1DefaultTTL)
		if err := cm.l2Cache.Set(ctx, key, value, cm.config.L2DefaultTTL); err != nil {
			cm.logger.Warnw("Failed to promote to L2", "key", key, "error", err)
		}
		return value, true, nil
	}
	cm.recordMiss(CacheLevelL3)

	return nil, false, nil
}

// Set stores a value in all appropriate cache levels
func (cm *CacheManager) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	start := time.Now()
	defer func() {
		if cm.config.EnableStats {
			cm.recordOperation("set", time.Since(start))
		}
	}()
	// Set in L1 cache
	cm.l1Cache.Set(key, value, ttl)
	cm.recordSet(CacheLevelL1)

	// Set in L2 cache
	if err := cm.l2Cache.Set(ctx, key, value, ttl); err != nil {
		cm.logger.Warnw("Failed to set in L2 cache", "key", key, "error", err)
	} else {
		cm.recordSet(CacheLevelL2)
	}

	// Set in L3 cache for persistence
	if err := cm.l3Cache.Set(ctx, key, value, ttl); err != nil {
		cm.logger.Warnw("Failed to set in L3 cache", "key", key, "error", err)
	} else {
		cm.recordSet(CacheLevelL3)
	}

	return nil
}

// Delete removes a value from all cache levels
func (cm *CacheManager) Delete(ctx context.Context, key string) error {
	// Delete from all levels
	cm.l1Cache.Delete(key)
	cm.recordDelete(CacheLevelL1)

	if err := cm.l2Cache.Delete(ctx, key); err != nil {
		cm.logger.Warnw("Failed to delete from L2 cache", "key", key, "error", err)
	} else {
		cm.recordDelete(CacheLevelL2)
	}

	if err := cm.l3Cache.Delete(ctx, key); err != nil {
		cm.logger.Warnw("Failed to delete from L3 cache", "key", key, "error", err)
	} else {
		cm.recordDelete(CacheLevelL3)
	}

	return nil
}

// Invalidate sends an invalidation event
func (cm *CacheManager) Invalidate(event InvalidationEvent) {
	event.Timestamp = time.Now()
	select {
	case cm.invalidationChan <- event:
	default:
		cm.logger.Warnw("Invalidation channel full, dropping event", "event", event)
	}
}

// GetStats returns current cache statistics
func (cm *CacheManager) GetStats() *CacheStats {
	return cm.stats.Load().(*CacheStats)
}

// Health checks the health of all cache levels
func (cm *CacheManager) Health(ctx context.Context) error {
	// Check L2 and L3 (Redis) health
	if err := cm.redis.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("Redis health check failed: %w", err)
	}

	// L1 is always healthy if the process is running
	return nil
}

// Close shuts down the cache manager gracefully
func (cm *CacheManager) Close() error {
	cm.logger.Info("Shutting down cache manager...")

	// Stop background workers
	close(cm.stopChan)
	cm.wg.Wait()

	// Stop tickers
	if cm.cleanupTicker != nil {
		cm.cleanupTicker.Stop()
	}
	if cm.statsTicker != nil {
		cm.statsTicker.Stop()
	}
	if cm.healthTicker != nil {
		cm.healthTicker.Stop()
	}

	// Close channels
	close(cm.invalidationChan)
	close(cm.warmingChan)

	cm.logger.Info("Cache manager shutdown complete")
	return nil
}

// startBackgroundWorkers initializes and starts all background workers
func (cm *CacheManager) startBackgroundWorkers() error {
	// Start cleanup ticker
	cm.cleanupTicker = time.NewTicker(cm.config.L1CleanupInterval)

	// Start statistics ticker
	if cm.config.EnableStats {
		cm.statsTicker = time.NewTicker(cm.config.StatsInterval)
	}

	// Start health check ticker
	cm.healthTicker = time.NewTicker(cm.config.HealthInterval)

	// Start worker goroutines
	for i := 0; i < cm.config.WorkerCount; i++ {
		cm.wg.Add(1)
		go cm.backgroundWorker(i)
	}

	// Start invalidation processor
	cm.wg.Add(1)
	go cm.invalidationProcessor()

	// Start warming processor
	if cm.config.EnableWarming {
		cm.wg.Add(1)
		go cm.warmingProcessor()
	}

	cm.logger.Infow("Background workers started", "worker_count", cm.config.WorkerCount)
	return nil
}

// backgroundWorker runs background maintenance tasks
func (cm *CacheManager) backgroundWorker(workerID int) {
	defer cm.wg.Done()

	cm.logger.Debugw("Background worker started", "worker_id", workerID)

	for {
		select {
		case <-cm.stopChan:
			cm.logger.Debugw("Background worker stopping", "worker_id", workerID)
			return

		case <-cm.cleanupTicker.C:
			// L1 cleanup is handled internally by L1Cache
			// L2/L3 cleanup can be done here if needed
			cm.performMaintenance()

		case <-cm.statsTicker.C:
			if cm.config.EnableStats {
				cm.updateStatistics()
			}

		case <-cm.healthTicker.C:
			cm.performHealthCheck()
		}
	}
}

// invalidationProcessor processes cache invalidation events
func (cm *CacheManager) invalidationProcessor() {
	defer cm.wg.Done()

	cm.logger.Debug("Invalidation processor started")

	for {
		select {
		case <-cm.stopChan:
			cm.logger.Debug("Invalidation processor stopping")
			return

		case event := <-cm.invalidationChan:
			cm.processInvalidationEvent(event)
		}
	}
}

// warmingProcessor processes cache warming requests
func (cm *CacheManager) warmingProcessor() {
	defer cm.wg.Done()

	cm.logger.Debug("Warming processor started")

	for {
		select {
		case <-cm.stopChan:
			cm.logger.Debug("Warming processor stopping")
			return

		case request := <-cm.warmingChan:
			cm.processWarmingRequest(request)
		}
	}
}

// processInvalidationEvent handles a single invalidation event
func (cm *CacheManager) processInvalidationEvent(event InvalidationEvent) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	switch event.Type {
	case InvalidateKey:
		for _, key := range event.Keys {
			if err := cm.Delete(ctx, key); err != nil {
				cm.logger.Warnw("Failed to invalidate key", "key", key, "error", err)
			}
		}

	case InvalidatePattern:
		// Pattern invalidation requires scanning - implement based on your needs
		cm.logger.Debugw("Pattern invalidation requested", "pattern", event.Pattern)
		// For now, we'll invalidate L1 keys that match the pattern
		cm.invalidateL1Pattern(event.Pattern)

	case InvalidateAll:
		cm.l1Cache.Clear()
		if err := cm.l2Cache.Clear(ctx); err != nil {
			cm.logger.Warnw("Failed to clear L2 cache", "error", err)
		}
		if err := cm.l3Cache.Clear(ctx); err != nil {
			cm.logger.Warnw("Failed to clear L3 cache", "error", err)
		}
	}

	cm.logger.Debugw("Invalidation event processed", "event", event)
}

// invalidateL1Pattern invalidates L1 cache keys matching a pattern
func (cm *CacheManager) invalidateL1Pattern(pattern string) {
	keys := cm.l1Cache.Keys()
	for _, key := range keys {
		// Simple pattern matching - can be enhanced with regex
		if matchesPattern(key, pattern) {
			cm.l1Cache.Delete(key)
		}
	}
}

// matchesPattern checks if a key matches a pattern (simple implementation)
func matchesPattern(key, pattern string) bool {
	// Simple wildcard matching - replace with proper regex if needed
	if pattern == "*" {
		return true
	}
	// Add more sophisticated pattern matching as needed
	return false
}

// processWarmingRequest handles a single warming request
func (cm *CacheManager) processWarmingRequest(request WarmingRequest) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// Load data using the provided loader
	value, err := request.Loader.Load(ctx, request.Key)
	if err != nil {
		cm.logger.Warnw("Failed to load data for warming", "key", request.Key, "error", err)
		if request.Callback != nil {
			request.Callback(request.Key, err)
		}
		return
	}

	switch request.Level {
	case CacheLevelL1:
		cm.l1Cache.Set(request.Key, value, request.TTL)
	case CacheLevelL2:
		if err := cm.l2Cache.Set(ctx, request.Key, value, request.TTL); err != nil {
			cm.logger.Warnw("Failed to warm L2 cache", "key", request.Key, "error", err)
		}
	case CacheLevelL3:
		if err := cm.l3Cache.Set(ctx, request.Key, value, request.TTL); err != nil {
			cm.logger.Warnw("Failed to warm L3 cache", "key", request.Key, "error", err)
		}
	default:
		// Warm all levels
		if err := cm.Set(ctx, request.Key, value, request.TTL); err != nil {
			cm.logger.Warnw("Failed to warm cache", "key", request.Key, "error", err)
		}
	}

	if request.Callback != nil {
		request.Callback(request.Key, nil)
	}

	cm.logger.Debugw("Warming request processed", "key", request.Key, "level", request.Level)
}

// performMaintenance runs periodic maintenance tasks
func (cm *CacheManager) performMaintenance() {
	// L1 cache cleanup is handled internally
	// Additional maintenance tasks can be added here
	cm.logger.Debug("Performing cache maintenance")
}

// updateStatistics updates cache statistics
func (cm *CacheManager) updateStatistics() {
	l1Stats := cm.l1Cache.Stats()
	l2Stats := cm.l2Cache.Stats()
	l3Stats := cm.l3Cache.Stats()

	// Calculate total statistics
	totalStats := &LevelStats{
		Hits:      l1Stats.Hits + l2Stats.Hits + l3Stats.Hits,
		Misses:    l1Stats.Misses + l2Stats.Misses + l3Stats.Misses,
		Sets:      l1Stats.Sets + l2Stats.Sets + l3Stats.Sets,
		Deletes:   l1Stats.Deletes + l2Stats.Deletes + l3Stats.Deletes,
		Evictions: l1Stats.Evictions,
	}

	if totalStats.Hits+totalStats.Misses > 0 {
		totalStats.HitRate = float64(totalStats.Hits) / float64(totalStats.Hits+totalStats.Misses)
	}

	// Update statistics
	newStats := &CacheStats{
		L1Stats: l1Stats,
		L2Stats: l2Stats,
		L3Stats: l3Stats,
		Total:   totalStats,
	}

	cm.stats.Store(newStats)

	if cm.config.StatsSampleRate > 0 {
		cm.logger.Debugw("Cache statistics updated",
			"l1_hit_rate", l1Stats.HitRate,
			"l2_hit_rate", l2Stats.HitRate,
			"l3_hit_rate", l3Stats.HitRate,
			"total_hit_rate", totalStats.HitRate,
		)
	}
}

// performHealthCheck checks the health of cache components
func (cm *CacheManager) performHealthCheck() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	if err := cm.Health(ctx); err != nil {
		cm.logger.Warnw("Cache health check failed", "error", err)
	}
}

// Helper methods for statistics
func (cm *CacheManager) recordHit(level CacheLevel) {
	if !cm.config.EnableStats {
		return
	}

	stats := cm.stats.Load().(*CacheStats)
	switch level {
	case CacheLevelL1:
		atomic.AddInt64(&stats.L1Stats.Hits, 1)
	case CacheLevelL2:
		atomic.AddInt64(&stats.L2Stats.Hits, 1)
	case CacheLevelL3:
		atomic.AddInt64(&stats.L3Stats.Hits, 1)
	}
	atomic.AddInt64(&stats.Total.Hits, 1)
}

func (cm *CacheManager) recordMiss(level CacheLevel) {
	if !cm.config.EnableStats {
		return
	}

	stats := cm.stats.Load().(*CacheStats)
	switch level {
	case CacheLevelL1:
		atomic.AddInt64(&stats.L1Stats.Misses, 1)
	case CacheLevelL2:
		atomic.AddInt64(&stats.L2Stats.Misses, 1)
	case CacheLevelL3:
		atomic.AddInt64(&stats.L3Stats.Misses, 1)
	}
	atomic.AddInt64(&stats.Total.Misses, 1)
}

func (cm *CacheManager) recordSet(level CacheLevel) {
	if !cm.config.EnableStats {
		return
	}

	stats := cm.stats.Load().(*CacheStats)
	switch level {
	case CacheLevelL1:
		atomic.AddInt64(&stats.L1Stats.Sets, 1)
	case CacheLevelL2:
		atomic.AddInt64(&stats.L2Stats.Sets, 1)
	case CacheLevelL3:
		atomic.AddInt64(&stats.L3Stats.Sets, 1)
	}
	atomic.AddInt64(&stats.Total.Sets, 1)
}

func (cm *CacheManager) recordDelete(level CacheLevel) {
	if !cm.config.EnableStats {
		return
	}

	stats := cm.stats.Load().(*CacheStats)
	switch level {
	case CacheLevelL1:
		atomic.AddInt64(&stats.L1Stats.Deletes, 1)
	case CacheLevelL2:
		atomic.AddInt64(&stats.L2Stats.Deletes, 1)
	case CacheLevelL3:
		atomic.AddInt64(&stats.L3Stats.Deletes, 1)
	}
	atomic.AddInt64(&stats.Total.Deletes, 1)
}

func (cm *CacheManager) recordOperation(op string, duration time.Duration) {
	// Record operation metrics for monitoring
	cm.logger.Debugw("Cache operation completed",
		"operation", op,
		"duration_ms", duration.Milliseconds(),
	)
}

// shouldSample determines if an operation should be sampled for statistics
func (cm *CacheManager) shouldSample() bool {
	// Simple sampling based on configuration
	// In production, you might want more sophisticated sampling
	return cm.config.StatsSampleRate >= 1.0 ||
		(cm.config.StatsSampleRate > 0 &&
			float64(time.Now().UnixNano()%1000)/1000.0 < cm.config.StatsSampleRate)
}

// Warm adds a warming request to the queue
func (cm *CacheManager) Warm(request WarmingRequest) error {
	select {
	case cm.warmingChan <- request:
		return nil
	default:
		return fmt.Errorf("warming queue full")
	}
}
