package messaging

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// MessagingConfig contains configuration for the entire messaging system
type MessagingConfig struct {
	Kafka          KafkaConfig               `json:"kafka"`
	TopicConfig    map[string]TopicConfig    `json:"topic_config"`
	ConsumerGroups map[string]ConsumerConfig `json:"consumer_groups"`
	CircuitBreaker CircuitBreakerConfig      `json:"circuit_breaker"`
	Monitoring     MonitoringConfig          `json:"monitoring"`
	HighFrequency  HighFrequencyConfig       `json:"high_frequency"`
	RetryPolicy    RetryPolicyConfig         `json:"retry_policy"`
}

// TopicConfig contains configuration for individual topics
type TopicConfig struct {
	Partitions        int    `json:"partitions"`
	ReplicationFactor int    `json:"replication_factor"`
	RetentionMs       int64  `json:"retention_ms"`
	CleanupPolicy     string `json:"cleanup_policy"`
	CompressionType   string `json:"compression_type"`
	MaxMessageBytes   int    `json:"max_message_bytes"`
}

// ConsumerConfig contains configuration for consumer groups
type ConsumerConfig struct {
	GroupID            string        `json:"group_id"`
	AutoOffsetReset    string        `json:"auto_offset_reset"`
	EnableAutoCommit   bool          `json:"enable_auto_commit"`
	AutoCommitInterval time.Duration `json:"auto_commit_interval"`
	SessionTimeout     time.Duration `json:"session_timeout"`
	HeartbeatInterval  time.Duration `json:"heartbeat_interval"`
	MaxPollRecords     int           `json:"max_poll_records"`
	MaxPollInterval    time.Duration `json:"max_poll_interval"`
	FetchMinBytes      int           `json:"fetch_min_bytes"`
	FetchMaxWait       time.Duration `json:"fetch_max_wait"`
}

// CircuitBreakerConfig contains circuit breaker configuration
type CircuitBreakerConfig struct {
	Enabled          bool          `json:"enabled"`
	FailureThreshold int           `json:"failure_threshold"`
	SuccessThreshold int           `json:"success_threshold"`
	Timeout          time.Duration `json:"timeout"`
	MaxRequests      int           `json:"max_requests"`
	ResetTimeout     time.Duration `json:"reset_timeout"`
}

// MonitoringConfig contains monitoring and metrics configuration
type MonitoringConfig struct {
	Enabled          bool          `json:"enabled"`
	MetricsInterval  time.Duration `json:"metrics_interval"`
	HealthCheck      bool          `json:"health_check"`
	PrometheusExport bool          `json:"prometheus_export"`
	LogLevel         string        `json:"log_level"`
}

// HighFrequencyConfig contains optimizations for high-frequency trading
type HighFrequencyConfig struct {
	Enabled                 bool          `json:"enabled"`
	BatchSize               int           `json:"batch_size"`
	BatchTimeout            time.Duration `json:"batch_timeout"`
	EnableCompression       bool          `json:"enable_compression"`
	EnableBuffering         bool          `json:"enable_buffering"`
	BufferFlushInterval     time.Duration `json:"buffer_flush_interval"`
	PreallocateBuffers      bool          `json:"preallocate_buffers"`
	UseZeroCopyOptimization bool          `json:"use_zero_copy_optimization"`
	CPUAffinity             []int         `json:"cpu_affinity"`
}

// RetryPolicyConfig contains retry policy configuration
type RetryPolicyConfig struct {
	MaxRetries        int           `json:"max_retries"`
	InitialBackoff    time.Duration `json:"initial_backoff"`
	MaxBackoff        time.Duration `json:"max_backoff"`
	BackoffMultiplier float64       `json:"backoff_multiplier"`
	EnableJitter      bool          `json:"enable_jitter"`
}

// DefaultMessagingConfig returns default configuration optimized for high-frequency trading
func DefaultMessagingConfig() *MessagingConfig {
	return &MessagingConfig{
		Kafka: *DefaultKafkaConfig(),
		TopicConfig: map[string]TopicConfig{
			string(TopicOrderEvents): {
				Partitions:        12, // High partitions for parallelism
				ReplicationFactor: 3,
				RetentionMs:       7 * 24 * 60 * 60 * 1000, // 7 days
				CleanupPolicy:     "delete",
				CompressionType:   "snappy",
				MaxMessageBytes:   1048576, // 1MB
			},
			string(TopicTradeEvents): {
				Partitions:        8,
				ReplicationFactor: 3,
				RetentionMs:       30 * 24 * 60 * 60 * 1000, // 30 days
				CleanupPolicy:     "delete",
				CompressionType:   "snappy",
				MaxMessageBytes:   1048576,
			},
			string(TopicBalanceEvents): {
				Partitions:        6,
				ReplicationFactor: 3,
				RetentionMs:       30 * 24 * 60 * 60 * 1000, // 30 days
				CleanupPolicy:     "delete",
				CompressionType:   "snappy",
				MaxMessageBytes:   524288, // 512KB
			},
			string(TopicMarketData): {
				Partitions:        16,                  // Very high for market data throughput
				ReplicationFactor: 2,                   // Lower replication for speed
				RetentionMs:       24 * 60 * 60 * 1000, // 1 day
				CleanupPolicy:     "delete",
				CompressionType:   "lz4",   // Fastest compression
				MaxMessageBytes:   2097152, // 2MB for order books
			},
			string(TopicNotifications): {
				Partitions:        4,
				ReplicationFactor: 2,
				RetentionMs:       7 * 24 * 60 * 60 * 1000, // 7 days
				CleanupPolicy:     "delete",
				CompressionType:   "gzip",
				MaxMessageBytes:   524288, // 512KB
			},
			string(TopicRiskEvents): {
				Partitions:        3,
				ReplicationFactor: 3,
				RetentionMs:       90 * 24 * 60 * 60 * 1000, // 90 days
				CleanupPolicy:     "delete",
				CompressionType:   "snappy",
				MaxMessageBytes:   524288, // 512KB
			},
			string(TopicFundsOps): {
				Partitions:        8,
				ReplicationFactor: 3,
				RetentionMs:       30 * 24 * 60 * 60 * 1000, // 30 days
				CleanupPolicy:     "delete",
				CompressionType:   "snappy",
				MaxMessageBytes:   524288, // 512KB
			},
		},
		ConsumerGroups: map[string]ConsumerConfig{
			"trading-engine": {
				GroupID:            "pincex-trading-engine",
				AutoOffsetReset:    "latest",
				EnableAutoCommit:   false, // Manual commit for exactly-once processing
				AutoCommitInterval: 100 * time.Millisecond,
				SessionTimeout:     30 * time.Second,
				HeartbeatInterval:  3 * time.Second,
				MaxPollRecords:     1000,
				MaxPollInterval:    5 * time.Minute,
				FetchMinBytes:      1,
				FetchMaxWait:       500 * time.Millisecond,
			},
			"bookkeeper": {
				GroupID:            "pincex-bookkeeper",
				AutoOffsetReset:    "latest",
				EnableAutoCommit:   false,
				AutoCommitInterval: 100 * time.Millisecond,
				SessionTimeout:     30 * time.Second,
				HeartbeatInterval:  3 * time.Second,
				MaxPollRecords:     500,
				MaxPollInterval:    5 * time.Minute,
				FetchMinBytes:      1,
				FetchMaxWait:       500 * time.Millisecond,
			},
			"market-data": {
				GroupID:            "pincex-market-data",
				AutoOffsetReset:    "latest",
				EnableAutoCommit:   true, // Can use auto-commit for market data
				AutoCommitInterval: 50 * time.Millisecond,
				SessionTimeout:     10 * time.Second,
				HeartbeatInterval:  1 * time.Second,
				MaxPollRecords:     2000, // High throughput for market data
				MaxPollInterval:    1 * time.Minute,
				FetchMinBytes:      1,
				FetchMaxWait:       100 * time.Millisecond, // Low latency
			},
			"notifications": {
				GroupID:            "pincex-notifications",
				AutoOffsetReset:    "latest",
				EnableAutoCommit:   true,
				AutoCommitInterval: 1 * time.Second,
				SessionTimeout:     30 * time.Second,
				HeartbeatInterval:  3 * time.Second,
				MaxPollRecords:     100,
				MaxPollInterval:    5 * time.Minute,
				FetchMinBytes:      1,
				FetchMaxWait:       1 * time.Second,
			},
		},
		CircuitBreaker: CircuitBreakerConfig{
			Enabled:          true,
			FailureThreshold: 5,
			SuccessThreshold: 3,
			Timeout:          60 * time.Second,
			MaxRequests:      100,
			ResetTimeout:     30 * time.Second,
		},
		Monitoring: MonitoringConfig{
			Enabled:          true,
			MetricsInterval:  10 * time.Second,
			HealthCheck:      true,
			PrometheusExport: true,
			LogLevel:         "info",
		},
		HighFrequency: HighFrequencyConfig{
			Enabled:                 true,
			BatchSize:               1000,
			BatchTimeout:            10 * time.Millisecond,
			EnableCompression:       true,
			EnableBuffering:         true,
			BufferFlushInterval:     5 * time.Millisecond,
			PreallocateBuffers:      true,
			UseZeroCopyOptimization: true,
			CPUAffinity:             []int{0, 1}, // Pin to specific CPU cores
		},
		RetryPolicy: RetryPolicyConfig{
			MaxRetries:        3,
			InitialBackoff:    100 * time.Millisecond,
			MaxBackoff:        5 * time.Second,
			BackoffMultiplier: 2.0,
			EnableJitter:      true,
		},
	}
}

// LoadConfigFromFile loads configuration from a JSON file
func LoadConfigFromFile(filename string) (*MessagingConfig, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config MessagingConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &config, nil
}

// LoadConfigFromEnv loads configuration from environment variables
func LoadConfigFromEnv() *MessagingConfig {
	config := DefaultMessagingConfig()

	// Override with environment variables
	if brokers := os.Getenv("KAFKA_BROKERS"); brokers != "" {
		config.Kafka.Brokers = strings.Split(brokers, ",")
	}

	if groupPrefix := os.Getenv("KAFKA_GROUP_PREFIX"); groupPrefix != "" {
		config.Kafka.ConsumerGroupPrefix = groupPrefix
	}

	if compressionType := os.Getenv("KAFKA_COMPRESSION"); compressionType != "" {
		config.Kafka.Compression = compressionType
	}

	// High-frequency trading optimizations
	if os.Getenv("ENABLE_HFT_OPTIMIZATIONS") == "true" {
		config.HighFrequency.Enabled = true
		config.HighFrequency.BatchTimeout = 1 * time.Millisecond
		config.HighFrequency.BufferFlushInterval = 1 * time.Millisecond
		config.Kafka.BatchTimeout = 1 * time.Millisecond
		config.Kafka.RequiredAcks = 1 // Reduce durability for speed
	}

	return config
}

// SaveConfigToFile saves configuration to a JSON file
func (c *MessagingConfig) SaveConfigToFile(filename string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// Validate validates the configuration
func (c *MessagingConfig) Validate() error {
	if len(c.Kafka.Brokers) == 0 {
		return fmt.Errorf("kafka brokers must be specified")
	}

	if c.Kafka.BatchSize <= 0 {
		return fmt.Errorf("kafka batch size must be positive")
	}

	if c.Kafka.BatchTimeout <= 0 {
		return fmt.Errorf("kafka batch timeout must be positive")
	}

	// Validate topic configurations
	for topicName, topicConfig := range c.TopicConfig {
		if topicConfig.Partitions <= 0 {
			return fmt.Errorf("topic %s: partitions must be positive", topicName)
		}
		if topicConfig.ReplicationFactor <= 0 {
			return fmt.Errorf("topic %s: replication factor must be positive", topicName)
		}
	}

	// Validate consumer group configurations
	for groupName, groupConfig := range c.ConsumerGroups {
		if groupConfig.GroupID == "" {
			return fmt.Errorf("consumer group %s: group ID must be specified", groupName)
		}
		if groupConfig.SessionTimeout <= 0 {
			return fmt.Errorf("consumer group %s: session timeout must be positive", groupName)
		}
	}

	return nil
}

// GetTopicConfig returns configuration for a specific topic
func (c *MessagingConfig) GetTopicConfig(topic Topic) (TopicConfig, bool) {
	config, exists := c.TopicConfig[string(topic)]
	return config, exists
}

// GetConsumerConfig returns configuration for a specific consumer group
func (c *MessagingConfig) GetConsumerConfig(groupName string) (ConsumerConfig, bool) {
	config, exists := c.ConsumerGroups[groupName]
	return config, exists
}

// IsHighFrequencyEnabled returns true if high-frequency optimizations are enabled
func (c *MessagingConfig) IsHighFrequencyEnabled() bool {
	return c.HighFrequency.Enabled
}

// GetCompressionType returns the compression type for a specific topic
func (c *MessagingConfig) GetCompressionType(topic Topic) string {
	if config, exists := c.TopicConfig[string(topic)]; exists {
		return config.CompressionType
	}
	return c.Kafka.Compression
}

// GetPartitionCount returns the number of partitions for a specific topic
func (c *MessagingConfig) GetPartitionCount(topic Topic) int {
	if config, exists := c.TopicConfig[string(topic)]; exists {
		return config.Partitions
	}
	return 1 // Default to 1 partition
}
