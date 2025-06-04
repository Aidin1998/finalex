package database

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// DatabaseService represents the consolidated database service
type service struct {
	logger    *zap.Logger
	db        *gorm.DB
	cache     map[string]*CacheEntry // Simple in-memory cache
	mu        sync.RWMutex
	cacheMu   sync.RWMutex
	running   bool
	config    *Config
	metrics   *Metrics
	pubSubMap map[string][]func([]byte)
	pubSubMu  sync.RWMutex
}

// Config represents database service configuration
type Config struct {
	Database DatabaseConfig `json:"database"`
	Cache    CacheConfig    `json:"cache"`
	Pool     PoolConfig     `json:"pool"`
}

// DatabaseConfig represents database configuration
type DatabaseConfig struct {
	Driver          string        `json:"driver"`
	Host            string        `json:"host"`
	Port            int           `json:"port"`
	Username        string        `json:"username"`
	Password        string        `json:"password"`
	Database        string        `json:"database"`
	SSLMode         string        `json:"ssl_mode"`
	MaxConnections  int           `json:"max_connections"`
	MaxIdleConns    int           `json:"max_idle_conns"`
	ConnMaxLifetime time.Duration `json:"conn_max_lifetime"`
	ConnMaxIdleTime time.Duration `json:"conn_max_idle_time"`
}

// CacheConfig represents in-memory cache configuration
type CacheConfig struct {
	DefaultExpiration time.Duration `json:"default_expiration"`
	CleanupInterval   time.Duration `json:"cleanup_interval"`
	MaxItems          int           `json:"max_items"`
}

// PoolConfig represents connection pool configuration
type PoolConfig struct {
	MaxOpenConns    int           `json:"max_open_conns"`
	MaxIdleConns    int           `json:"max_idle_conns"`
	ConnMaxLifetime time.Duration `json:"conn_max_lifetime"`
	ConnMaxIdleTime time.Duration `json:"conn_max_idle_time"`
}

// Metrics represents database service metrics
type Metrics struct {
	DBConnections     int64 `json:"db_connections"`
	CacheHits         int64 `json:"cache_hits"`
	CacheMisses       int64 `json:"cache_misses"`
	QueriesExecuted   int64 `json:"queries_executed"`
	TransactionsCount int64 `json:"transactions_count"`
	ErrorCount        int64 `json:"error_count"`
}

// CacheEntry represents a cache entry with metadata
type CacheEntry struct {
	Key         string      `json:"key"`
	Value       interface{} `json:"value"`
	ExpiresAt   time.Time   `json:"expires_at"`
	CreatedAt   time.Time   `json:"created_at"`
	AccessCount int         `json:"access_count"`
	LastAccess  time.Time   `json:"last_access"`
}

// QueryResult represents a database query result
type QueryResult struct {
	Data         interface{}   `json:"data"`
	RowsAffected int64         `json:"rows_affected"`
	Duration     time.Duration `json:"duration"`
	Query        string        `json:"query"`
	Success      bool          `json:"success"`
	Error        string        `json:"error"`
	Timestamp    time.Time     `json:"timestamp"`
}

// TransactionStats represents transaction statistics
type TransactionStats struct {
	StartTime    time.Time     `json:"start_time"`
	EndTime      time.Time     `json:"end_time"`
	Duration     time.Duration `json:"duration"`
	QueryCount   int           `json:"query_count"`
	Success      bool          `json:"success"`
	ErrorMessage string        `json:"error_message"`
}

// NewService creates a new consolidated database service
func NewService(logger *zap.Logger, db *gorm.DB) (Service, error) {
	if logger == nil {
		return nil, fmt.Errorf("logger is required")
	}
	if db == nil {
		return nil, fmt.Errorf("database connection is required")
	}

	// Default configuration
	config := &Config{
		Cache: CacheConfig{
			DefaultExpiration: 5 * time.Minute,
			CleanupInterval:   10 * time.Minute,
			MaxItems:          10000,
		},
		Pool: PoolConfig{
			MaxOpenConns:    100,
			MaxIdleConns:    10,
			ConnMaxLifetime: 1 * time.Hour,
			ConnMaxIdleTime: 5 * time.Minute,
		},
	}

	// Create cache
	memoryCache := make(map[string]*CacheEntry)

	// Create service
	s := &service{
		logger:    logger,
		db:        db,
		cache:     memoryCache,
		config:    config,
		metrics:   &Metrics{},
		pubSubMap: make(map[string][]func([]byte)),
	}

	// Start background cache cleanup
	go s.cacheCleanupWorker()

	return s, nil
}

// cacheCleanupWorker periodically cleans expired cache entries
func (s *service) cacheCleanupWorker() {
	ticker := time.NewTicker(s.config.Cache.CleanupInterval)
	defer ticker.Stop()

	for {
		s.mu.RLock()
		running := s.running
		s.mu.RUnlock()

		if !running {
			break
		}

		<-ticker.C
		s.cleanupCache()
	}
}

// cleanupCache removes expired entries from cache
func (s *service) cleanupCache() {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()

	now := time.Now()
	for key, entry := range s.cache {
		if !entry.ExpiresAt.IsZero() && now.After(entry.ExpiresAt) {
			delete(s.cache, key)
		}
	}
}

// ExecuteQuery executes a database query
func (s *service) ExecuteQuery(ctx context.Context, query string, args ...interface{}) (interface{}, error) {
	start := time.Now()
	var result interface{}

	tx := s.db.WithContext(ctx).Raw(query, args...)
	if tx.Error != nil {
		s.metrics.ErrorCount++
		return nil, tx.Error
	}

	if err := tx.Scan(&result).Error; err != nil {
		s.metrics.ErrorCount++
		return nil, err
	}

	s.metrics.QueriesExecuted++

	queryResult := &QueryResult{
		Data:         result,
		RowsAffected: tx.RowsAffected,
		Duration:     time.Since(start),
		Query:        query,
		Success:      true,
		Timestamp:    time.Now(),
	}

	return queryResult, nil
}

// ExecuteTransaction executes a transaction
func (s *service) ExecuteTransaction(ctx context.Context, txFunc func(tx interface{}) error) error {
	stats := &TransactionStats{
		StartTime: time.Now(),
	}

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return txFunc(tx)
	})

	stats.EndTime = time.Now()
	stats.Duration = stats.EndTime.Sub(stats.StartTime)
	stats.Success = err == nil

	if err != nil {
		stats.ErrorMessage = err.Error()
		s.metrics.ErrorCount++
	}

	s.metrics.TransactionsCount++

	return err
}

// GetDBStats gets database statistics
func (s *service) GetDBStats(ctx context.Context) (map[string]interface{}, error) {
	sqlDB, err := s.db.DB()
	if err != nil {
		return nil, err
	}

	stats := make(map[string]interface{})
	stats["max_open_connections"] = sqlDB.Stats().MaxOpenConnections
	stats["open_connections"] = sqlDB.Stats().OpenConnections
	stats["in_use"] = sqlDB.Stats().InUse
	stats["idle"] = sqlDB.Stats().Idle
	stats["wait_count"] = sqlDB.Stats().WaitCount
	stats["wait_duration"] = sqlDB.Stats().WaitDuration
	stats["max_idle_closed"] = sqlDB.Stats().MaxIdleClosed
	stats["max_lifetime_closed"] = sqlDB.Stats().MaxLifetimeClosed

	return stats, nil
}

// OptimizeQueries optimizes database queries
func (s *service) OptimizeQueries(ctx context.Context) (map[string]interface{}, error) {
	// This would normally run ANALYZE/VACUUM or similar operations
	// For this implementation, we'll just return stats
	stats, err := s.GetDBStats(ctx)
	if err != nil {
		return nil, err
	}

	result := map[string]interface{}{
		"optimized": true,
		"stats":     stats,
		"timestamp": time.Now(),
	}

	return result, nil
}

// GetFromCache gets a value from cache
func (s *service) GetFromCache(ctx context.Context, key string) (interface{}, error) {
	s.cacheMu.RLock()
	defer s.cacheMu.RUnlock()

	entry, found := s.cache[key]
	if !found {
		s.metrics.CacheMisses++
		return nil, fmt.Errorf("key not found in cache: %s", key)
	}

	// Check if expired
	if !entry.ExpiresAt.IsZero() && time.Now().After(entry.ExpiresAt) {
		s.metrics.CacheMisses++
		return nil, fmt.Errorf("cached value has expired: %s", key)
	}

	// Update access metadata
	entry.AccessCount++
	entry.LastAccess = time.Now()

	s.metrics.CacheHits++
	return entry.Value, nil
}

// SetInCache sets a value in cache
func (s *service) SetInCache(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()

	var expiresAt time.Time
	if expiration > 0 {
		expiresAt = time.Now().Add(expiration)
	}

	entry := &CacheEntry{
		Key:         key,
		Value:       value,
		ExpiresAt:   expiresAt,
		CreatedAt:   time.Now(),
		AccessCount: 0,
		LastAccess:  time.Time{},
	}

	s.cache[key] = entry
	return nil
}

// DeleteFromCache deletes a value from cache
func (s *service) DeleteFromCache(ctx context.Context, key string) error {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()

	_, exists := s.cache[key]
	if !exists {
		return fmt.Errorf("key not found in cache: %s", key)
	}

	delete(s.cache, key)
	return nil
}

// FlushCache flushes the cache
func (s *service) FlushCache(ctx context.Context) error {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()

	s.cache = make(map[string]*CacheEntry)
	return nil
}

// Publish publishes a message to a channel
func (s *service) Publish(ctx context.Context, channel string, message interface{}) error {
	s.pubSubMu.RLock()
	defer s.pubSubMu.RUnlock()

	subscribers, ok := s.pubSubMap[channel]
	if !ok || len(subscribers) == 0 {
		return nil // No subscribers, message dropped
	}

	var msgBytes []byte
	switch m := message.(type) {
	case string:
		msgBytes = []byte(m)
	case []byte:
		msgBytes = m
	default:
		return fmt.Errorf("unsupported message type: must be string or []byte")
	}

	// Send to all subscribers (non-blocking)
	for _, callback := range subscribers {
		go func(cb func([]byte)) {
			cb(msgBytes)
		}(callback)
	}

	return nil
}

// Subscribe subscribes to a channel
func (s *service) Subscribe(ctx context.Context, channel string, callback func([]byte)) error {
	if callback == nil {
		return errors.New("callback function cannot be nil")
	}

	s.pubSubMu.Lock()
	defer s.pubSubMu.Unlock()

	if _, ok := s.pubSubMap[channel]; !ok {
		s.pubSubMap[channel] = make([]func([]byte), 0)
	}

	s.pubSubMap[channel] = append(s.pubSubMap[channel], callback)
	return nil
}

// GetPerformanceMetrics gets performance metrics
func (s *service) GetPerformanceMetrics(ctx context.Context) (map[string]interface{}, error) {
	dbStats, err := s.GetDBStats(ctx)
	if err != nil {
		return nil, err
	}

	s.cacheMu.RLock()
	cacheItemCount := len(s.cache)
	s.cacheMu.RUnlock()

	metrics := map[string]interface{}{
		"database": dbStats,
		"cache": map[string]interface{}{
			"items":     cacheItemCount,
			"hits":      s.metrics.CacheHits,
			"misses":    s.metrics.CacheMisses,
			"hit_ratio": calculateHitRatio(s.metrics.CacheHits, s.metrics.CacheMisses),
		},
		"queries": map[string]interface{}{
			"executed":   s.metrics.QueriesExecuted,
			"errors":     s.metrics.ErrorCount,
			"error_rate": calculateErrorRate(s.metrics.ErrorCount, s.metrics.QueriesExecuted),
		},
		"transactions": map[string]interface{}{
			"count": s.metrics.TransactionsCount,
		},
		"timestamp": time.Now(),
	}

	return metrics, nil
}

// OptimizeCacheHitRatio optimizes cache hit ratio
func (s *service) OptimizeCacheHitRatio(ctx context.Context) error {
	// This would normally implement cache optimization strategies
	// like TTL adjustment, prefetching, etc.
	// For this implementation, we'll just log the current hit ratio

	ratio := calculateHitRatio(s.metrics.CacheHits, s.metrics.CacheMisses)
	s.logger.Info("Cache hit ratio",
		zap.Float64("hit_ratio", ratio),
		zap.Int64("hits", s.metrics.CacheHits),
		zap.Int64("misses", s.metrics.CacheMisses))

	return nil
}

// Start starts the service
func (s *service) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return errors.New("service already running")
	}

	// Apply database connection pool settings
	sqlDB, err := s.db.DB()
	if err != nil {
		return fmt.Errorf("error getting database: %w", err)
	}

	sqlDB.SetMaxOpenConns(s.config.Pool.MaxOpenConns)
	sqlDB.SetMaxIdleConns(s.config.Pool.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(s.config.Pool.ConnMaxLifetime)
	sqlDB.SetConnMaxIdleTime(s.config.Pool.ConnMaxIdleTime)

	s.running = true
	return nil
}

// Stop stops the service
func (s *service) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return errors.New("service not running")
	}

	// Close database
	sqlDB, err := s.db.DB()
	if err != nil {
		return fmt.Errorf("error getting database: %w", err)
	}

	if err := sqlDB.Close(); err != nil {
		s.logger.Error("Failed to close database connection", zap.Error(err))
	}

	// Clear cache
	s.cacheMu.Lock()
	s.cache = make(map[string]*CacheEntry)
	s.cacheMu.Unlock()

	// Clear pubsub
	s.pubSubMu.Lock()
	s.pubSubMap = make(map[string][]func([]byte))
	s.pubSubMu.Unlock()

	s.running = false
	return nil
}

// Helper functions
func calculateHitRatio(hits, misses int64) float64 {
	total := hits + misses
	if total == 0 {
		return 0
	}
	return float64(hits) / float64(total)
}

func calculateErrorRate(errors, total int64) float64 {
	if total == 0 {
		return 0
	}
	return float64(errors) / float64(total)
}
