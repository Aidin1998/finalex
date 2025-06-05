// Redis-based cache layer with hot/warm/cold data tiering for ultra-high concurrency
package accounts

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// Cache TTL configurations for different data tiers
const (
	HotDataTTL    = 5 * time.Minute   // Frequently accessed data (active accounts)
	WarmDataTTL   = 30 * time.Minute  // Moderately accessed data (recent transactions)
	ColdDataTTL   = 2 * time.Hour     // Rarely accessed data (historical data)
	LockTTL       = 30 * time.Second  // Distributed lock TTL
	BackgroundTTL = 24 * time.Hour    // Background job cache
)

// Redis key patterns for different data types
const (
	AccountBalanceKey     = "account:balance:%s:%s"      // user_id:currency
	AccountLockKey        = "account:lock:%s:%s"         // user_id:currency
	AccountVersionKey     = "account:version:%s:%s"      // user_id:currency
	AccountMetaKey        = "account:meta:%s:%s"         // user_id:currency
	ReservationKey        = "reservation:%s"             // reservation_id
	ReservationUserKey    = "reservation:user:%s:%s"    // user_id:currency
	TransactionKey        = "transaction:%s"             // transaction_id
	TransactionUserKey    = "transaction:user:%s"       // user_id
	BalanceSnapshotKey    = "snapshot:balance:%s:%s:%s" // user_id:currency:date
	AuditLogKey           = "audit:%s:%s"               // table:record_id
	UserPartitionKey      = "partition:user:%s"         // user_id
	CurrencyStatsKey      = "stats:currency:%s"         // currency
	SystemHealthKey       = "health:system"
	JobStatusKey          = "job:status:%s"             // job_id
)

// CachedAccount represents account data in cache
type CachedAccount struct {
	ID           uuid.UUID       `json:"id"`
	UserID       uuid.UUID       `json:"user_id"`
	Currency     string          `json:"currency"`
	Balance      decimal.Decimal `json:"balance"`
	Available    decimal.Decimal `json:"available"`
	Locked       decimal.Decimal `json:"locked"`
	Version      int64           `json:"version"`
	AccountType  string          `json:"account_type"`
	Status       string          `json:"status"`
	UpdatedAt    time.Time       `json:"updated_at"`
	CachedAt     time.Time       `json:"cached_at"`
	AccessCount  int64           `json:"access_count"`
	LastAccessed time.Time       `json:"last_accessed"`
}

// CachedReservation represents reservation data in cache
type CachedReservation struct {
	ID          uuid.UUID       `json:"id"`
	UserID      uuid.UUID       `json:"user_id"`
	Currency    string          `json:"currency"`
	Amount      decimal.Decimal `json:"amount"`
	Type        string          `json:"type"`
	ReferenceID string          `json:"reference_id"`
	Status      string          `json:"status"`
	ExpiresAt   *time.Time      `json:"expires_at"`
	Version     int64           `json:"version"`
	CachedAt    time.Time       `json:"cached_at"`
}

// CacheMetrics holds Prometheus metrics for cache operations
type CacheMetrics struct {
	CacheHits      *prometheus.CounterVec
	CacheMisses    *prometheus.CounterVec
	CacheWrites    *prometheus.CounterVec
	CacheEvictions *prometheus.CounterVec
	CacheLatency   *prometheus.HistogramVec
	CacheSize      *prometheus.GaugeVec
	HotDataRatio   prometheus.Gauge
	WarmDataRatio  prometheus.Gauge
	ColdDataRatio  prometheus.Gauge
}

// CacheLayer provides high-performance caching with data tiering
type CacheLayer struct {
	hotClient    redis.UniversalClient
	warmClient   redis.UniversalClient
	coldClient   redis.UniversalClient
	logger       *zap.Logger
	metrics      *CacheMetrics
	config       *CacheConfig
}

// CacheConfig represents cache layer configuration
type CacheConfig struct {
	HotRedisAddr    []string `json:"hot_redis_addr"`
	WarmRedisAddr   []string `json:"warm_redis_addr"`
	ColdRedisAddr   []string `json:"cold_redis_addr"`
	MaxRetries      int      `json:"max_retries"`
	DialTimeout     string   `json:"dial_timeout"`
	ReadTimeout     string   `json:"read_timeout"`
	WriteTimeout    string   `json:"write_timeout"`
	PoolSize        int      `json:"pool_size"`
	PoolTimeout     string   `json:"pool_timeout"`
	IdleTimeout     string   `json:"idle_timeout"`
	EnableMetrics   bool     `json:"enable_metrics"`
	CompressionType string   `json:"compression_type"` // none, gzip, lz4
}

// NewCacheLayer creates a new cache layer with data tiering
func NewCacheLayer(config *CacheConfig, logger *zap.Logger) (*CacheLayer, error) {
	dialTimeout, _ := time.ParseDuration(config.DialTimeout)
	readTimeout, _ := time.ParseDuration(config.ReadTimeout)
	writeTimeout, _ := time.ParseDuration(config.WriteTimeout)
	poolTimeout, _ := time.ParseDuration(config.PoolTimeout)
	idleTimeout, _ := time.ParseDuration(config.IdleTimeout)

	// Hot data client (high-performance, low latency)
	hotClient := redis.NewUniversalClient(&redis.UniversalOptions{
		Addrs:       config.HotRedisAddr,
		MaxRetries:  config.MaxRetries,
		DialTimeout: dialTimeout,
		ReadTimeout: readTimeout,
		WriteTimeout: writeTimeout,
		PoolSize:    config.PoolSize,
		PoolTimeout: poolTimeout,
		IdleTimeout: idleTimeout,
	})

	// Warm data client (balanced performance)
	warmClient := redis.NewUniversalClient(&redis.UniversalOptions{
		Addrs:       config.WarmRedisAddr,
		MaxRetries:  config.MaxRetries,
		DialTimeout: dialTimeout,
		ReadTimeout: readTimeout,
		WriteTimeout: writeTimeout,
		PoolSize:    config.PoolSize / 2,
		PoolTimeout: poolTimeout,
		IdleTimeout: idleTimeout,
	})

	// Cold data client (cost-optimized)
	coldClient := redis.NewUniversalClient(&redis.UniversalOptions{
		Addrs:       config.ColdRedisAddr,
		MaxRetries:  config.MaxRetries,
		DialTimeout: dialTimeout * 2,
		ReadTimeout: readTimeout * 2,
		WriteTimeout: writeTimeout * 2,
		PoolSize:    config.PoolSize / 4,
		PoolTimeout: poolTimeout * 2,
		IdleTimeout: idleTimeout,
	})

	// Initialize metrics
	metrics := &CacheMetrics{
		CacheHits: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "accounts_cache_hits_total",
			Help: "Total number of cache hits",
		}, []string{"tier", "operation"}),
		CacheMisses: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "accounts_cache_misses_total",
			Help: "Total number of cache misses",
		}, []string{"tier", "operation"}),
		CacheWrites: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "accounts_cache_writes_total",
			Help: "Total number of cache writes",
		}, []string{"tier", "operation"}),
		CacheEvictions: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "accounts_cache_evictions_total",
			Help: "Total number of cache evictions",
		}, []string{"tier", "reason"}),
		CacheLatency: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "accounts_cache_operation_duration_seconds",
			Help:    "Duration of cache operations",
			Buckets: prometheus.ExponentialBuckets(0.0001, 2, 12),
		}, []string{"tier", "operation"}),
		CacheSize: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "accounts_cache_size_bytes",
			Help: "Size of cache in bytes",
		}, []string{"tier"}),
		HotDataRatio: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "accounts_cache_hot_data_ratio",
			Help: "Ratio of hot data access",
		}),
		WarmDataRatio: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "accounts_cache_warm_data_ratio",
			Help: "Ratio of warm data access",
		}),
		ColdDataRatio: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "accounts_cache_cold_data_ratio",
			Help: "Ratio of cold data access",
		}),
	}

	return &CacheLayer{
		hotClient:  hotClient,
		warmClient: warmClient,
		coldClient: coldClient,
		logger:     logger,
		metrics:    metrics,
		config:     config,
	}, nil
}

// GetAccount retrieves account from appropriate cache tier
func (c *CacheLayer) GetAccount(ctx context.Context, userID uuid.UUID, currency string) (*CachedAccount, error) {
	key := fmt.Sprintf(AccountBalanceKey, userID.String(), currency)
	
	// Try hot cache first
	if account, err := c.getAccountFromTier(ctx, c.hotClient, "hot", key); err == nil {
		c.metrics.CacheHits.WithLabelValues("hot", "get_account").Inc()
		c.promoteToHot(ctx, key, account)
		return account, nil
	}
	
	// Try warm cache
	if account, err := c.getAccountFromTier(ctx, c.warmClient, "warm", key); err == nil {
		c.metrics.CacheHits.WithLabelValues("warm", "get_account").Inc()
		c.promoteToHot(ctx, key, account)
		return account, nil
	}
	
	// Try cold cache
	if account, err := c.getAccountFromTier(ctx, c.coldClient, "cold", key); err == nil {
		c.metrics.CacheHits.WithLabelValues("cold", "get_account").Inc()
		c.promoteToWarm(ctx, key, account)
		return account, nil
	}
	
	c.metrics.CacheMisses.WithLabelValues("all", "get_account").Inc()
	return nil, redis.Nil
}

// SetAccount stores account in appropriate cache tier based on access patterns
func (c *CacheLayer) SetAccount(ctx context.Context, account *CachedAccount) error {
	key := fmt.Sprintf(AccountBalanceKey, account.UserID.String(), account.Currency)
	account.CachedAt = time.Now()
	account.AccessCount++
	account.LastAccessed = time.Now()
	
	data, err := json.Marshal(account)
	if err != nil {
		return fmt.Errorf("failed to marshal account: %w", err)
	}
	
	// Determine tier based on access patterns
	tier := c.determineTier(account)
	
	switch tier {
	case "hot":
		err = c.hotClient.Set(ctx, key, data, HotDataTTL).Err()
		c.metrics.CacheWrites.WithLabelValues("hot", "set_account").Inc()
	case "warm":
		err = c.warmClient.Set(ctx, key, data, WarmDataTTL).Err()
		c.metrics.CacheWrites.WithLabelValues("warm", "set_account").Inc()
	case "cold":
		err = c.coldClient.Set(ctx, key, data, ColdDataTTL).Err()
		c.metrics.CacheWrites.WithLabelValues("cold", "set_account").Inc()
	}
	
	return err
}

// GetAccountBalance retrieves only balance information (optimized for high frequency)
func (c *CacheLayer) GetAccountBalance(ctx context.Context, userID uuid.UUID, currency string) (decimal.Decimal, decimal.Decimal, int64, error) {
	key := fmt.Sprintf(AccountBalanceKey, userID.String(), currency)
	
	// Use pipeline for atomic retrieval of balance data
	pipe := c.hotClient.Pipeline()
	balanceCmd := pipe.HMGet(ctx, key, "balance", "available", "version")
	_, err := pipe.Exec(ctx)
	
	if err != nil {
		c.metrics.CacheMisses.WithLabelValues("hot", "get_balance").Inc()
		return decimal.Zero, decimal.Zero, 0, err
	}
	
	c.metrics.CacheHits.WithLabelValues("hot", "get_balance").Inc()
	
	values := balanceCmd.Val()
	if len(values) != 3 || values[0] == nil {
		return decimal.Zero, decimal.Zero, 0, redis.Nil
	}
	
	balance, _ := decimal.NewFromString(values[0].(string))
	available, _ := decimal.NewFromString(values[1].(string))
	version, _ := strconv.ParseInt(values[2].(string), 10, 64)
	
	return balance, available, version, nil
}

// SetAccountBalance stores balance information in hot cache
func (c *CacheLayer) SetAccountBalance(ctx context.Context, userID uuid.UUID, currency string, balance, available decimal.Decimal, version int64) error {
	key := fmt.Sprintf(AccountBalanceKey, userID.String(), currency)
	
	pipe := c.hotClient.Pipeline()
	pipe.HMSet(ctx, key, map[string]interface{}{
		"balance":       balance.String(),
		"available":     available.String(),
		"version":       version,
		"updated_at":    time.Now().Format(time.RFC3339),
		"last_accessed": time.Now().Format(time.RFC3339),
	})
	pipe.Expire(ctx, key, HotDataTTL)
	_, err := pipe.Exec(ctx)
	
	if err == nil {
		c.metrics.CacheWrites.WithLabelValues("hot", "set_balance").Inc()
	}
	
	return err
}

// GetReservations retrieves user reservations from cache
func (c *CacheLayer) GetReservations(ctx context.Context, userID uuid.UUID, currency string) ([]*CachedReservation, error) {
	key := fmt.Sprintf(ReservationUserKey, userID.String(), currency)
	
	// Try hot cache first
	if reservations, err := c.getReservationsFromTier(ctx, c.hotClient, "hot", key); err == nil {
		c.metrics.CacheHits.WithLabelValues("hot", "get_reservations").Inc()
		return reservations, nil
	}
	
	// Try warm cache
	if reservations, err := c.getReservationsFromTier(ctx, c.warmClient, "warm", key); err == nil {
		c.metrics.CacheHits.WithLabelValues("warm", "get_reservations").Inc()
		// Promote to hot cache
		c.setReservationsInTier(ctx, c.hotClient, key, reservations, HotDataTTL)
		return reservations, nil
	}
	
	c.metrics.CacheMisses.WithLabelValues("all", "get_reservations").Inc()
	return nil, redis.Nil
}

// InvalidateAccount removes account from all cache tiers
func (c *CacheLayer) InvalidateAccount(ctx context.Context, userID uuid.UUID, currency string) error {
	key := fmt.Sprintf(AccountBalanceKey, userID.String(), currency)
	
	// Remove from all tiers
	c.hotClient.Del(ctx, key)
	c.warmClient.Del(ctx, key)
	c.coldClient.Del(ctx, key)
	
	return nil
}

// WarmupCache preloads frequently accessed data
func (c *CacheLayer) WarmupCache(ctx context.Context, accounts []*Account) error {
	pipe := c.hotClient.Pipeline()
	
	for _, account := range accounts {
		cachedAccount := &CachedAccount{
			ID:           account.ID,
			UserID:       account.UserID,
			Currency:     account.Currency,
			Balance:      account.Balance,
			Available:    account.Available,
			Locked:       account.Locked,
			Version:      account.Version,
			AccountType:  account.AccountType,
			Status:       account.Status,
			UpdatedAt:    account.UpdatedAt,
			CachedAt:     time.Now(),
			AccessCount:  0,
			LastAccessed: time.Now(),
		}
		
		key := fmt.Sprintf(AccountBalanceKey, account.UserID.String(), account.Currency)
		data, _ := json.Marshal(cachedAccount)
		pipe.Set(ctx, key, data, HotDataTTL)
	}
	
	_, err := pipe.Exec(ctx)
	c.logger.Info("Cache warmup completed", zap.Int("accounts", len(accounts)))
	return err
}

// Helper methods

func (c *CacheLayer) getAccountFromTier(ctx context.Context, client redis.UniversalClient, tier, key string) (*CachedAccount, error) {
	start := time.Now()
	defer func() {
		c.metrics.CacheLatency.WithLabelValues(tier, "get").Observe(time.Since(start).Seconds())
	}()
	
	data, err := client.Get(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	
	var account CachedAccount
	if err := json.Unmarshal([]byte(data), &account); err != nil {
		return nil, err
	}
	
	account.AccessCount++
	account.LastAccessed = time.Now()
	
	return &account, nil
}

func (c *CacheLayer) getReservationsFromTier(ctx context.Context, client redis.UniversalClient, tier, key string) ([]*CachedReservation, error) {
	start := time.Now()
	defer func() {
		c.metrics.CacheLatency.WithLabelValues(tier, "get_reservations").Observe(time.Since(start).Seconds())
	}()
	
	data, err := client.Get(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	
	var reservations []*CachedReservation
	if err := json.Unmarshal([]byte(data), &reservations); err != nil {
		return nil, err
	}
	
	return reservations, nil
}

func (c *CacheLayer) setReservationsInTier(ctx context.Context, client redis.UniversalClient, key string, reservations []*CachedReservation, ttl time.Duration) error {
	data, err := json.Marshal(reservations)
	if err != nil {
		return err
	}
	
	return client.Set(ctx, key, data, ttl).Err()
}

func (c *CacheLayer) promoteToHot(ctx context.Context, key string, account *CachedAccount) {
	data, _ := json.Marshal(account)
	c.hotClient.Set(ctx, key, data, HotDataTTL)
}

func (c *CacheLayer) promoteToWarm(ctx context.Context, key string, account *CachedAccount) {
	data, _ := json.Marshal(account)
	c.warmClient.Set(ctx, key, data, WarmDataTTL)
}

func (c *CacheLayer) determineTier(account *CachedAccount) string {
	// Hot tier: recently accessed or high access count
	if time.Since(account.LastAccessed) < 5*time.Minute || account.AccessCount > 100 {
		return "hot"
	}
	
	// Warm tier: moderately accessed
	if time.Since(account.LastAccessed) < 30*time.Minute || account.AccessCount > 10 {
		return "warm"
	}
	
	// Cold tier: rarely accessed
	return "cold"
}

// Health checks the cache layer connectivity
func (c *CacheLayer) Health(ctx context.Context) error {
	// Check all tiers
	if err := c.hotClient.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("hot cache unhealthy: %w", err)
	}
	
	if err := c.warmClient.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("warm cache unhealthy: %w", err)
	}
	
	if err := c.coldClient.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("cold cache unhealthy: %w", err)
	}
	
	return nil
}

// Close closes all cache connections
func (c *CacheLayer) Close() error {
	if err := c.hotClient.Close(); err != nil {
		return err
	}
	
	if err := c.warmClient.Close(); err != nil {
		return err
	}
	
	return c.coldClient.Close()
}
