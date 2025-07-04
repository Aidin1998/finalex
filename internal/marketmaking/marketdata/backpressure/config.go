// Configuration management for backpressure system
package backpressure

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// BackpressureConfig is the comprehensive configuration for the backpressure system
type BackpressureConfig struct {
	// Manager configuration
	Manager ManagerConfig `json:"manager" yaml:"manager"`

	// Rate limiter configuration
	RateLimiter RateLimiterConfig `json:"rate_limiter" yaml:"rate_limiter"`

	// Priority queue configuration
	PriorityQueue PriorityQueueConfig `json:"priority_queue" yaml:"priority_queue"`

	// Client capability detector configuration
	CapabilityDetector CapabilityDetectorConfig `json:"capability_detector" yaml:"capability_detector"`

	// Cross-service coordinator configuration
	Coordinator CoordinatorConfig `json:"coordinator" yaml:"coordinator"`

	// WebSocket integration configuration
	WebSocket WebSocketConfig `json:"websocket" yaml:"websocket"`

	// Logging configuration
	Logging LoggingConfig `json:"logging" yaml:"logging"`
	// Metrics configuration
	Metrics MetricsConfig `json:"metrics" yaml:"metrics"`
}

// ManagerConfig configures the backpressure manager
type ManagerConfig struct {
	WorkerCount                   int                         `json:"worker_count" yaml:"worker_count"`
	ProcessingTimeout             time.Duration               `json:"processing_timeout" yaml:"processing_timeout"`
	MaxRetries                    int                         `json:"max_retries" yaml:"max_retries"`
	EmergencyLatencyThreshold     time.Duration               `json:"emergency_latency_threshold" yaml:"emergency_latency_threshold"`
	EmergencyQueueLengthThreshold int                         `json:"emergency_queue_length_threshold" yaml:"emergency_queue_length_threshold"`
	EmergencyDropRateThreshold    float64                     `json:"emergency_drop_rate_threshold" yaml:"emergency_drop_rate_threshold"`
	RecoveryGracePeriod           time.Duration               `json:"recovery_grace_period" yaml:"recovery_grace_period"`
	RecoveryLatencyTarget         time.Duration               `json:"recovery_latency_target" yaml:"recovery_latency_target"`
	ClientTimeout                 time.Duration               `json:"client_timeout" yaml:"client_timeout"`
	MaxClientsPerShard            int                         `json:"max_clients_per_shard" yaml:"max_clients_per_shard"`
	GlobalRateLimit               int64                       `json:"global_rate_limit" yaml:"global_rate_limit"`
	PriorityRateMultipliers       map[MessagePriority]float64 `json:"priority_rate_multipliers" yaml:"priority_rate_multipliers"`
}

// ClientClass defines client classification for rate limiting
// Used as keys in ClientClassLimits
type ClientClass uint8

const (
	HFTClient ClientClass = iota
	MarketMakerClient
	InstitutionalClient
	RetailClient
	DefaultClientClass
)

// RateLimiterConfig configures the adaptive rate limiter
type RateLimiterConfig struct {
	GlobalRateLimit    int64                             `json:"global_rate_limit" yaml:"global_rate_limit"`
	BurstMultiplier    float64                           `json:"burst_multiplier" yaml:"burst_multiplier"`
	RefillInterval     time.Duration                     `json:"refill_interval" yaml:"refill_interval"`
	AdaptationInterval time.Duration                     `json:"adaptation_interval" yaml:"adaptation_interval"`
	MinTokens          int64                             `json:"min_tokens" yaml:"min_tokens"`
	MaxTokens          int64                             `json:"max_tokens" yaml:"max_tokens"`
	ClientClassLimits  map[ClientClass]ClientClassConfig `json:"client_class_limits" yaml:"client_class_limits"`
	PriorityWeights    map[MessagePriority]float64       `json:"priority_weights" yaml:"priority_weights"`
}

// ClientClassConfig configures limits for a specific client class
type ClientClassConfig struct {
	BaseRate    int64         `json:"base_rate" yaml:"base_rate"`
	BurstRate   int64         `json:"burst_rate" yaml:"burst_rate"`
	Priority    int           `json:"priority" yaml:"priority"`
	TokenRefill time.Duration `json:"token_refill" yaml:"token_refill"`
}

// PriorityQueueConfig configures the priority queue
type PriorityQueueConfig struct {
	InitialCapacity int           `json:"initial_capacity" yaml:"initial_capacity"`
	MaxCapacity     int           `json:"max_capacity" yaml:"max_capacity"`
	ShardCount      int           `json:"shard_count" yaml:"shard_count"`
	FastPathEnabled bool          `json:"fast_path_enabled" yaml:"fast_path_enabled"`
	GCInterval      time.Duration `json:"gc_interval" yaml:"gc_interval"`
	CompactionRatio float64       `json:"compaction_ratio" yaml:"compaction_ratio"`
	DropPolicy      DropPolicy    `json:"drop_policy" yaml:"drop_policy"` // Changed from int to DropPolicy
	MaxLatency      time.Duration `json:"max_latency" yaml:"max_latency"` // Added for latency control
}

// CoordinatorConfig configures the cross-service coordinator
type CoordinatorConfig struct {
	ServiceID                 string        `json:"service_id" yaml:"service_id"`
	KafkaBrokers              []string      `json:"kafka_brokers" yaml:"kafka_brokers"`
	UpdateInterval            time.Duration `json:"update_interval" yaml:"update_interval"`
	RetryInterval             time.Duration `json:"retry_interval" yaml:"retry_interval"`
	MaxRetries                int           `json:"max_retries" yaml:"max_retries"`
	HealthCheckInterval       time.Duration `json:"health_check_interval" yaml:"health_check_interval"`
	EmergencyBroadcastEnabled bool          `json:"emergency_broadcast_enabled" yaml:"emergency_broadcast_enabled"`
	// Additional fields for full compatibility
	BackpressureTopic   string        `json:"backpressure_topic" yaml:"backpressure_topic"`
	ConsumerGroup       string        `json:"consumer_group" yaml:"consumer_group"`
	ServiceTimeout      time.Duration `json:"service_timeout" yaml:"service_timeout"`
	GlobalLoadThreshold float64       `json:"global_load_threshold" yaml:"global_load_threshold"`
	EmergencyThreshold  float64       `json:"emergency_threshold" yaml:"emergency_threshold"`
	SignalBufferSize    int           `json:"signal_buffer_size" yaml:"signal_buffer_size"`
	SignalTimeout       time.Duration `json:"signal_timeout" yaml:"signal_timeout"`
	CriticalServices    []string      `json:"critical_services" yaml:"critical_services"`
	SheddableServices   []string      `json:"sheddable_services" yaml:"sheddable_services"`
}

// WebSocketConfig configures WebSocket integration
type WebSocketConfig struct {
	WriteTimeout        time.Duration `json:"write_timeout" yaml:"write_timeout"`
	PingInterval        time.Duration `json:"ping_interval" yaml:"ping_interval"`
	MaxMessageSize      int64         `json:"max_message_size" yaml:"max_message_size"`
	EnableCompression   bool          `json:"enable_compression" yaml:"enable_compression"`
	MaxWriteErrors      int           `json:"max_write_errors" yaml:"max_write_errors"`
	ErrorRecoveryTime   time.Duration `json:"error_recovery_time" yaml:"error_recovery_time"`
	BufferSize          int           `json:"buffer_size" yaml:"buffer_size"`
	MaxConcurrentWrites int           `json:"max_concurrent_writes" yaml:"max_concurrent_writes"`
}

// CapabilityDetectorConfig configures the client capability detector
type CapabilityDetectorConfig struct {
	// Measurement intervals
	MeasurementInterval  time.Duration `json:"measurement_interval" yaml:"measurement_interval"`
	BandwidthWindowSize  int           `json:"bandwidth_window_size" yaml:"bandwidth_window_size"`
	ProcessingWindowSize int           `json:"processing_window_size" yaml:"processing_window_size"`
	LatencyWindowSize    int           `json:"latency_window_size" yaml:"latency_window_size"`

	// Classification thresholds
	HFTBandwidthThreshold  int64         `json:"hft_bandwidth_threshold" yaml:"hft_bandwidth_threshold"`
	HFTProcessingThreshold int64         `json:"hft_processing_threshold" yaml:"hft_processing_threshold"`
	HFTLatencyThreshold    time.Duration `json:"hft_latency_threshold" yaml:"hft_latency_threshold"`

	MarketMakerBandwidthThreshold  int64         `json:"market_maker_bandwidth_threshold" yaml:"market_maker_bandwidth_threshold"`
	MarketMakerProcessingThreshold int64         `json:"market_maker_processing_threshold" yaml:"market_maker_processing_threshold"`
	MarketMakerLatencyThreshold    time.Duration `json:"market_maker_latency_threshold" yaml:"market_maker_latency_threshold"`

	InstitutionalBandwidthThreshold  int64         `json:"institutional_bandwidth_threshold" yaml:"institutional_bandwidth_threshold"`
	InstitutionalProcessingThreshold int64         `json:"institutional_processing_threshold" yaml:"institutional_processing_threshold"`
	InstitutionalLatencyThreshold    time.Duration `json:"institutional_latency_threshold" yaml:"institutional_latency_threshold"`

	// Adaptation parameters
	AdaptationSensitivity float64       `json:"adaptation_sensitivity" yaml:"adaptation_sensitivity"`
	MinObservationPeriod  time.Duration `json:"min_observation_period" yaml:"min_observation_period"`
	MaxObservationPeriod  time.Duration `json:"max_observation_period" yaml:"max_observation_period"`

	// Performance tuning
	WorkerCount               int `json:"worker_count" yaml:"worker_count"`
	MaxConcurrentMeasurements int `json:"max_concurrent_measurements" yaml:"max_concurrent_measurements"`
}

// LoggingConfig configures logging for the backpressure system
type LoggingConfig struct {
	Level            string `json:"level" yaml:"level"`
	Format           string `json:"format" yaml:"format"`
	OutputFile       string `json:"output_file" yaml:"output_file"`
	MaxFileSize      int    `json:"max_file_size_mb" yaml:"max_file_size_mb"`
	MaxBackups       int    `json:"max_backups" yaml:"max_backups"`
	MaxAge           int    `json:"max_age_days" yaml:"max_age_days"`
	EnableConsole    bool   `json:"enable_console" yaml:"enable_console"`
	EnableStackTrace bool   `json:"enable_stack_trace" yaml:"enable_stack_trace"`
}

// MetricsConfig configures Prometheus metrics
type MetricsConfig struct {
	Enabled        bool          `json:"enabled" yaml:"enabled"`
	Port           int           `json:"port" yaml:"port"`
	Path           string        `json:"path" yaml:"path"`
	UpdateInterval time.Duration `json:"update_interval" yaml:"update_interval"`
	EnableDetailed bool          `json:"enable_detailed" yaml:"enable_detailed"`

	// Custom labels
	ServiceName    string            `json:"service_name" yaml:"service_name"`
	ServiceVersion string            `json:"service_version" yaml:"service_version"`
	Environment    string            `json:"environment" yaml:"environment"`
	CustomLabels   map[string]string `json:"custom_labels" yaml:"custom_labels"`
}

// LoadBackpressureConfig loads configuration from file
func LoadBackpressureConfig(configPath string) (*BackpressureConfig, error) {
	// Check if file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// Create default config
		defaultConfig := GetDefaultBackpressureConfig()
		if err := SaveBackpressureConfig(configPath, defaultConfig); err != nil {
			return nil, fmt.Errorf("failed to save default config: %w", err)
		}
		return defaultConfig, nil
	}

	// Read file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse based on file extension
	var config BackpressureConfig
	if isYAMLFile(configPath) {
		if err := yaml.Unmarshal(data, &config); err != nil {
			return nil, fmt.Errorf("failed to parse YAML config: %w", err)
		}
	} else {
		if err := json.Unmarshal(data, &config); err != nil {
			return nil, fmt.Errorf("failed to parse JSON config: %w", err)
		}
	}

	// Validate configuration
	if err := ValidateBackpressureConfig(&config); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &config, nil
}

// SaveBackpressureConfig saves configuration to file
func SaveBackpressureConfig(configPath string, config *BackpressureConfig) error {
	var data []byte
	var err error

	if isYAMLFile(configPath) {
		data, err = yaml.Marshal(config)
		if err != nil {
			return fmt.Errorf("failed to marshal YAML config: %w", err)
		}
	} else {
		data, err = json.MarshalIndent(config, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal JSON config: %w", err)
		}
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// GetDefaultBackpressureConfig returns default configuration
func GetDefaultBackpressureConfig() *BackpressureConfig {
	return &BackpressureConfig{
		Manager: ManagerConfig{
			WorkerCount:                   8,
			ProcessingTimeout:             time.Millisecond * 100,
			MaxRetries:                    3,
			EmergencyLatencyThreshold:     time.Millisecond * 500,
			EmergencyQueueLengthThreshold: 50000,
			EmergencyDropRateThreshold:    0.1,
			RecoveryGracePeriod:           time.Second * 30,
			RecoveryLatencyTarget:         time.Millisecond * 200,
			ClientTimeout:                 time.Minute * 5,
			MaxClientsPerShard:            1000,
			GlobalRateLimit:               100000,
			PriorityRateMultipliers: map[MessagePriority]float64{
				PriorityCritical: 10.0,
				PriorityHigh:     3.0,
				PriorityMedium:   1.0,
				PriorityLow:      0.5,
				PriorityMarket:   0.8,
			},
		},

		RateLimiter: RateLimiterConfig{
			GlobalRateLimit:    100000,
			BurstMultiplier:    2.0,
			RefillInterval:     time.Second,
			AdaptationInterval: time.Second * 10,
			MinTokens:          100,
			MaxTokens:          1000000,
			ClientClassLimits: map[ClientClass]ClientClassConfig{
				HFTClient: {
					BaseRate:    50000,
					BurstRate:   100000,
					Priority:    1,
					TokenRefill: time.Millisecond * 10,
				},
				MarketMakerClient: {
					BaseRate:    25000,
					BurstRate:   50000,
					Priority:    2,
					TokenRefill: time.Millisecond * 20,
				},
				InstitutionalClient: {
					BaseRate:    10000,
					BurstRate:   20000,
					Priority:    3,
					TokenRefill: time.Millisecond * 50,
				},
				RetailClient: {
					BaseRate:    1000,
					BurstRate:   2000,
					Priority:    4,
					TokenRefill: time.Millisecond * 100,
				},
				DefaultClientClass: {
					BaseRate:    5000,
					BurstRate:   10000,
					Priority:    5,
					TokenRefill: time.Millisecond * 100,
				},
			},
			PriorityWeights: map[MessagePriority]float64{
				PriorityCritical: 10.0,
				PriorityHigh:     3.0,
				PriorityMedium:   1.0,
				PriorityLow:      0.5,
				PriorityMarket:   0.8,
			},
		},

		PriorityQueue: PriorityQueueConfig{
			InitialCapacity: 10000,
			MaxCapacity:     1000000,
			ShardCount:      8,
			FastPathEnabled: true,
			GCInterval:      time.Minute * 5,
			CompactionRatio: 0.5,
			DropPolicy:      0,               // Default drop policy
			MaxLatency:      time.Second * 1, // Default max latency
		},

		CapabilityDetector: CapabilityDetectorConfig{
			MeasurementInterval:              time.Second * 5,
			BandwidthWindowSize:              20,
			ProcessingWindowSize:             50,
			LatencyWindowSize:                100,
			HFTBandwidthThreshold:            1024 * 1024 * 10, // 10MB/s
			HFTProcessingThreshold:           10000,            // 10k msgs/s
			HFTLatencyThreshold:              time.Millisecond * 5,
			MarketMakerBandwidthThreshold:    1024 * 1024 * 5, // 5MB/s
			MarketMakerProcessingThreshold:   5000,            // 5k msgs/s
			MarketMakerLatencyThreshold:      time.Millisecond * 10,
			InstitutionalBandwidthThreshold:  1024 * 1024 * 2, // 2MB/s
			InstitutionalProcessingThreshold: 2000,            // 2k msgs/s
			InstitutionalLatencyThreshold:    time.Millisecond * 25,
			AdaptationSensitivity:            0.8,
			MinObservationPeriod:             time.Second * 30,
			MaxObservationPeriod:             time.Minute * 10,
			WorkerCount:                      4,
			MaxConcurrentMeasurements:        1000,
		},

		Coordinator: CoordinatorConfig{
			ServiceID:                 "marketdata-backpressure",
			KafkaBrokers:              []string{"localhost:9092"},
			UpdateInterval:            time.Second * 5,
			RetryInterval:             time.Second * 2,
			MaxRetries:                5,
			HealthCheckInterval:       time.Second * 30,
			EmergencyBroadcastEnabled: true,
			// Additional fields for full compatibility
			BackpressureTopic:   "backpressure",
			ConsumerGroup:       "marketdata_group",
			ServiceTimeout:      time.Second * 10,
			GlobalLoadThreshold: 0.8,
			EmergencyThreshold:  0.9,
			SignalBufferSize:    1000,
			SignalTimeout:       time.Second * 5,
			CriticalServices:    []string{"serviceA", "serviceB"},
			SheddableServices:   []string{"serviceC", "serviceD"},
		},

		WebSocket: WebSocketConfig{
			WriteTimeout:        time.Second * 10,
			PingInterval:        time.Second * 30,
			MaxMessageSize:      1024 * 1024, // 1MB
			EnableCompression:   true,
			MaxWriteErrors:      5,
			ErrorRecoveryTime:   time.Minute * 2,
			BufferSize:          1000,
			MaxConcurrentWrites: 100,
		},

		Logging: LoggingConfig{
			Level:            "info",
			Format:           "json",
			OutputFile:       "logs/backpressure.log",
			MaxFileSize:      100, // 100MB
			MaxBackups:       5,
			MaxAge:           30, // 30 days
			EnableConsole:    true,
			EnableStackTrace: false,
		},

		Metrics: MetricsConfig{
			Enabled:        true,
			Port:           8080,
			Path:           "/metrics",
			UpdateInterval: time.Second * 10,
			EnableDetailed: true,
			ServiceName:    "pincex-backpressure",
			ServiceVersion: "1.0.0",
			Environment:    "production",
			CustomLabels:   map[string]string{},
		},
	}
}

// ValidateBackpressureConfig validates the configuration
func ValidateBackpressureConfig(config *BackpressureConfig) error {
	// Validate manager config
	if config.Manager.WorkerCount <= 0 {
		return fmt.Errorf("manager worker count must be positive")
	}
	if config.Manager.ProcessingTimeout <= 0 {
		return fmt.Errorf("manager processing timeout must be positive")
	}
	if config.Manager.GlobalRateLimit <= 0 {
		return fmt.Errorf("manager global rate limit must be positive")
	}

	// Validate rate limiter config
	if config.RateLimiter.GlobalRateLimit <= 0 {
		return fmt.Errorf("rate limiter global rate limit must be positive")
	}
	if config.RateLimiter.BurstMultiplier <= 1.0 {
		return fmt.Errorf("rate limiter burst multiplier must be > 1.0")
	}

	// Validate priority queue config
	if config.PriorityQueue.InitialCapacity <= 0 {
		return fmt.Errorf("priority queue initial capacity must be positive")
	}
	if config.PriorityQueue.MaxCapacity < config.PriorityQueue.InitialCapacity {
		return fmt.Errorf("priority queue max capacity must be >= initial capacity")
	}
	if config.PriorityQueue.ShardCount <= 0 {
		return fmt.Errorf("priority queue shard count must be positive")
	}

	// Validate capability detector config
	if config.CapabilityDetector.MeasurementInterval <= 0 {
		return fmt.Errorf("capability detector measurement interval must be positive")
	}
	if config.CapabilityDetector.WorkerCount <= 0 {
		return fmt.Errorf("capability detector worker count must be positive")
	}

	// Validate coordinator config
	if len(config.Coordinator.KafkaBrokers) == 0 {
		return fmt.Errorf("coordinator must have at least one Kafka broker")
	}
	if config.Coordinator.UpdateInterval <= 0 {
		return fmt.Errorf("coordinator update interval must be positive")
	}

	// Validate WebSocket config
	if config.WebSocket.WriteTimeout <= 0 {
		return fmt.Errorf("websocket write timeout must be positive")
	}
	if config.WebSocket.MaxMessageSize <= 0 {
		return fmt.Errorf("websocket max message size must be positive")
	}

	// Validate logging config
	validLevels := map[string]bool{
		"debug": true, "info": true, "warn": true, "error": true, "fatal": true,
	}
	if !validLevels[config.Logging.Level] {
		return fmt.Errorf("invalid logging level: %s", config.Logging.Level)
	}

	// Validate metrics config
	if config.Metrics.Enabled && config.Metrics.Port <= 0 {
		return fmt.Errorf("metrics port must be positive when metrics are enabled")
	}

	return nil
}

// isYAMLFile checks if the file has a YAML extension
func isYAMLFile(filename string) bool {
	return len(filename) > 4 && (filename[len(filename)-4:] == ".yml" || filename[len(filename)-5:] == ".yaml")
}

// MergeConfigs merges two configurations, with override taking precedence
func MergeConfigs(base, override *BackpressureConfig) *BackpressureConfig {
	merged := *base

	// Simple field-by-field merge - in a real implementation,
	// you'd want more sophisticated merging logic
	if override.Manager.WorkerCount > 0 {
		merged.Manager.WorkerCount = override.Manager.WorkerCount
	}
	if override.Manager.ProcessingTimeout > 0 {
		merged.Manager.ProcessingTimeout = override.Manager.ProcessingTimeout
	}
	// ... continue for other fields as needed

	return &merged
}

// GetConfigSample returns a sample configuration for documentation
func GetConfigSample() string {
	config := GetDefaultBackpressureConfig()
	data, _ := yaml.Marshal(config)
	return string(data)
}
