package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// Config holds Redis configuration
type Config struct {
	// Connection settings
	Addr     string `yaml:"addr" json:"addr"`
	Password string `yaml:"password" json:"password"`
	DB       int    `yaml:"db" json:"db"`

	// Pool settings
	PoolSize           int           `yaml:"pool_size" json:"pool_size"`
	MinIdleConns       int           `yaml:"min_idle_conns" json:"min_idle_conns"`
	MaxConnAge         time.Duration `yaml:"max_conn_age" json:"max_conn_age"`
	PoolTimeout        time.Duration `yaml:"pool_timeout" json:"pool_timeout"`
	IdleTimeout        time.Duration `yaml:"idle_timeout" json:"idle_timeout"`
	IdleCheckFrequency time.Duration `yaml:"idle_check_frequency" json:"idle_check_frequency"`

	// Operational settings
	MaxRetries      int           `yaml:"max_retries" json:"max_retries"`
	MinRetryBackoff time.Duration `yaml:"min_retry_backoff" json:"min_retry_backoff"`
	MaxRetryBackoff time.Duration `yaml:"max_retry_backoff" json:"max_retry_backoff"`

	// Timeout settings
	DialTimeout  time.Duration `yaml:"dial_timeout" json:"dial_timeout"`
	ReadTimeout  time.Duration `yaml:"read_timeout" json:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout" json:"write_timeout"`

	// Cache settings
	DefaultTTL time.Duration `yaml:"default_ttl" json:"default_ttl"`

	// Cluster settings (for future scaling)
	EnableCluster bool     `yaml:"enable_cluster" json:"enable_cluster"`
	ClusterAddrs  []string `yaml:"cluster_addrs" json:"cluster_addrs"`

	// Sentinel settings (for high availability)
	EnableSentinel   bool     `yaml:"enable_sentinel" json:"enable_sentinel"`
	SentinelAddrs    []string `yaml:"sentinel_addrs" json:"sentinel_addrs"`
	SentinelPassword string   `yaml:"sentinel_password" json:"sentinel_password"`
	MasterName       string   `yaml:"master_name" json:"master_name"`
}

// DefaultConfig returns default Redis configuration optimized for trading workloads
func DefaultConfig() *Config {
	return &Config{
		Addr:     "localhost:6379",
		Password: "",
		DB:       0,

		// Optimized pool settings for high-frequency trading
		PoolSize:           100,              // High pool size for concurrent access
		MinIdleConns:       20,               // Keep connections warm
		MaxConnAge:         time.Hour * 24,   // Rotate connections daily
		PoolTimeout:        time.Second * 4,  // Fast pool timeout
		IdleTimeout:        time.Minute * 5,  // Keep idle connections
		IdleCheckFrequency: time.Minute,      // Check idle connections frequently

		// Fast retry settings
		MaxRetries:      3,
		MinRetryBackoff: time.Millisecond * 8,
		MaxRetryBackoff: time.Millisecond * 512,

		// Aggressive timeout settings for low latency
		DialTimeout:  time.Second * 5,
		ReadTimeout:  time.Millisecond * 500,  // 500ms read timeout
		WriteTimeout: time.Millisecond * 500,  // 500ms write timeout

		// Cache settings
		DefaultTTL: time.Minute * 15, // 15 minute default TTL

		// Cluster disabled by default
		EnableCluster: false,
		ClusterAddrs:  nil,

		// Sentinel disabled by default
		EnableSentinel:   false,
		SentinelAddrs:    nil,
		SentinelPassword: "",
		MasterName:       "",
	}
}

// Client wraps Redis client with additional functionality
type Client struct {
	rdb    redis.UniversalClient
	config *Config
	logger *zap.SugaredLogger
}

// NewClient creates a new Redis client with the given configuration
func NewClient(config *Config, logger *zap.SugaredLogger) (*Client, error) {
	var rdb redis.UniversalClient

	if config.EnableCluster {
		// Create cluster client
		rdb = redis.NewClusterClient(&redis.ClusterOptions{
			Addrs:    config.ClusterAddrs,
			Password: config.Password,

			// Pool settings
			PoolSize:           config.PoolSize,
			MinIdleConns:       config.MinIdleConns,
			MaxConnAge:         config.MaxConnAge,
			PoolTimeout:        config.PoolTimeout,
			IdleTimeout:        config.IdleTimeout,
			IdleCheckFrequency: config.IdleCheckFrequency,

			// Retry settings
			MaxRetries:      config.MaxRetries,
			MinRetryBackoff: config.MinRetryBackoff,
			MaxRetryBackoff: config.MaxRetryBackoff,

			// Timeout settings
			DialTimeout:  config.DialTimeout,
			ReadTimeout:  config.ReadTimeout,
			WriteTimeout: config.WriteTimeout,
		})
	} else if config.EnableSentinel {
		// Create failover client with sentinel
		rdb = redis.NewFailoverClient(&redis.FailoverOptions{
			MasterName:       config.MasterName,
			SentinelAddrs:    config.SentinelAddrs,
			SentinelPassword: config.SentinelPassword,
			Password:         config.Password,
			DB:               config.DB,

			// Pool settings
			PoolSize:           config.PoolSize,
			MinIdleConns:       config.MinIdleConns,
			MaxConnAge:         config.MaxConnAge,
			PoolTimeout:        config.PoolTimeout,
			IdleTimeout:        config.IdleTimeout,
			IdleCheckFrequency: config.IdleCheckFrequency,

			// Retry settings
			MaxRetries:      config.MaxRetries,
			MinRetryBackoff: config.MinRetryBackoff,
			MaxRetryBackoff: config.MaxRetryBackoff,

			// Timeout settings
			DialTimeout:  config.DialTimeout,
			ReadTimeout:  config.ReadTimeout,
			WriteTimeout: config.WriteTimeout,
		})
	} else {
		// Create single instance client
		rdb = redis.NewClient(&redis.Options{
			Addr:     config.Addr,
			Password: config.Password,
			DB:       config.DB,

			// Pool settings
			PoolSize:           config.PoolSize,
			MinIdleConns:       config.MinIdleConns,
			MaxConnAge:         config.MaxConnAge,
			PoolTimeout:        config.PoolTimeout,
			IdleTimeout:        config.IdleTimeout,
			IdleCheckFrequency: config.IdleCheckFrequency,

			// Retry settings
			MaxRetries:      config.MaxRetries,
			MinRetryBackoff: config.MinRetryBackoff,
			MaxRetryBackoff: config.MaxRetryBackoff,

			// Timeout settings
			DialTimeout:  config.DialTimeout,
			ReadTimeout:  config.ReadTimeout,
			WriteTimeout: config.WriteTimeout,
		})
	}

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), config.DialTimeout)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	client := &Client{
		rdb:    rdb,
		config: config,
		logger: logger,
	}

	logger.Infow("Redis client connected",
		"addr", config.Addr,
		"db", config.DB,
		"pool_size", config.PoolSize,
		"cluster_mode", config.EnableCluster,
		"sentinel_mode", config.EnableSentinel,
	)

	return client, nil
}

// GetClient returns the underlying Redis client
func (c *Client) GetClient() redis.UniversalClient {
	return c.rdb
}

// Close closes the Redis connection
func (c *Client) Close() error {
	if c.rdb != nil {
		return c.rdb.Close()
	}
	return nil
}

// Health checks the health of Redis connection
func (c *Client) Health(ctx context.Context) error {
	return c.rdb.Ping(ctx).Err()
}

// GetConfig returns the Redis configuration
func (c *Client) GetConfig() *Config {
	return c.config
}

// GetStats returns Redis connection pool statistics
func (c *Client) GetStats() *redis.PoolStats {
	return c.rdb.PoolStats()
}
