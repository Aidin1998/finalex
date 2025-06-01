// =============================
// AML/Risk Management Configuration Loader
// =============================
// This file provides configuration loading and validation for the async
// risk management service from the risk-management.yaml file.

package aml

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// RiskManagementConfig represents the complete risk management configuration
type RiskManagementConfig struct {
	PositionLimits  PositionLimitsConfig  `yaml:"position_limits"`
	RiskCalculation RiskCalculationConfig `yaml:"risk_calculation"`
	Compliance      ComplianceConfig      `yaml:"compliance"`
	Dashboard       DashboardConfig       `yaml:"dashboard"`
	Reporting       ReportingConfig       `yaml:"reporting"`
	Database        DatabaseConfig        `yaml:"database"`
	Monitoring      MonitoringConfig      `yaml:"monitoring"`
	Security        SecurityConfig        `yaml:"security"`
	Integration     IntegrationConfig     `yaml:"integration"`
	Development     DevelopmentConfig     `yaml:"development"`
	Features        FeaturesConfig        `yaml:"features"`
}

// PositionLimitsConfig defines position limit settings
type PositionLimitsConfig struct {
	DefaultUserLimit        float64 `yaml:"default_user_limit"`
	DefaultMarketLimit      float64 `yaml:"default_market_limit"`
	GlobalLimit             float64 `yaml:"global_limit"`
	VIPMultiplier           float64 `yaml:"vip_multiplier"`
	InstitutionalMultiplier float64 `yaml:"institutional_multiplier"`
}

// RiskCalculationConfig defines risk calculation settings
type RiskCalculationConfig struct {
	VarConfidence            float64       `yaml:"var_confidence"`
	VarTimeHorizon           int           `yaml:"var_time_horizon"`
	CalculationTimeout       time.Duration `yaml:"calculation_timeout"`
	BatchSize                int           `yaml:"batch_size"`
	UpdateInterval           time.Duration `yaml:"update_interval"`
	PriceStalenessThreshold  time.Duration `yaml:"price_staleness_threshold"`
	VolatilityLookbackPeriod time.Duration `yaml:"volatility_lookback_period"`
	CorrelationUpdateFreq    time.Duration `yaml:"correlation_update_frequency"`
}

// ComplianceConfig defines compliance monitoring settings
type ComplianceConfig struct {
	AML      AMLConfig      `yaml:"aml"`
	KYT      KYTConfig      `yaml:"kyt"`
	Patterns PatternsConfig `yaml:"patterns"`
}

// AMLConfig defines anti-money laundering settings
type AMLConfig struct {
	Enabled              bool          `yaml:"enabled"`
	DailyVolumeThreshold float64       `yaml:"daily_volume_threshold"`
	StructuringThreshold float64       `yaml:"structuring_threshold"`
	VelocityThreshold    int           `yaml:"velocity_threshold"`
	VelocityTimeWindow   time.Duration `yaml:"velocity_time_window"`
}

// KYTConfig defines know your transaction settings
type KYTConfig struct {
	Enabled               bool     `yaml:"enabled"`
	HighRiskJurisdictions []string `yaml:"high_risk_jurisdictions"`
	BlockedAddresses      []string `yaml:"blocked_addresses"`
}

// PatternsConfig defines pattern detection settings
type PatternsConfig struct {
	RoundAmountThreshold      float64       `yaml:"round_amount_threshold"`
	TimeClusteringThreshold   time.Duration `yaml:"time_clustering_threshold"`
	AmountClusteringThreshold float64       `yaml:"amount_clustering_threshold"`
}

// DashboardConfig defines dashboard settings
type DashboardConfig struct {
	RefreshInterval     time.Duration         `yaml:"refresh_interval"`
	MetricsRetention    time.Duration         `yaml:"metrics_retention"`
	AlertRetention      time.Duration         `yaml:"alert_retention"`
	MaxSubscribers      int                   `yaml:"max_subscribers"`
	SubscriptionTimeout time.Duration         `yaml:"subscription_timeout"`
	AlertThresholds     AlertThresholdsConfig `yaml:"alert_thresholds"`
}

// AlertThresholdsConfig defines alert threshold settings
type AlertThresholdsConfig struct {
	HighRiskScore         float64 `yaml:"high_risk_score"`
	CriticalRiskScore     float64 `yaml:"critical_risk_score"`
	SystemHealthThreshold float64 `yaml:"system_health_threshold"`
}

// ReportingConfig defines reporting settings
type ReportingConfig struct {
	SAR               ReportTypeConfig `yaml:"sar"`
	CTR               ReportTypeConfig `yaml:"ctr"`
	LTR               ReportTypeConfig `yaml:"ltr"`
	RetentionPeriod   time.Duration    `yaml:"retention_period"`
	StoragePath       string           `yaml:"storage_path"`
	BackupEnabled     bool             `yaml:"backup_enabled"`
	BackupInterval    time.Duration    `yaml:"backup_interval"`
	SubmissionTimeout time.Duration    `yaml:"submission_timeout"`
	RetryAttempts     int              `yaml:"retry_attempts"`
	BatchSize         int              `yaml:"batch_size"`
}

// ReportTypeConfig defines settings for specific report types
type ReportTypeConfig struct {
	Enabled        bool    `yaml:"enabled"`
	Threshold      float64 `yaml:"threshold"`
	AutoSubmission bool    `yaml:"auto_submission"`
}

// DatabaseConfig defines database settings
type DatabaseConfig struct {
	MaxConnections    int           `yaml:"max_connections"`
	ConnectionTimeout time.Duration `yaml:"connection_timeout"`
	QueryTimeout      time.Duration `yaml:"query_timeout"`
	ReadTimeout       time.Duration `yaml:"read_timeout"`
	WriteTimeout      time.Duration `yaml:"write_timeout"`
	IdleTimeout       time.Duration `yaml:"idle_timeout"`
	BackupInterval    time.Duration `yaml:"backup_interval"`
	MaintenanceWindow string        `yaml:"maintenance_window"`
	VacuumInterval    time.Duration `yaml:"vacuum_interval"`
}

// MonitoringConfig defines monitoring and alerting settings
type MonitoringConfig struct {
	HealthCheckInterval time.Duration `yaml:"health_check_interval"`
	ServiceTimeout      time.Duration `yaml:"service_timeout"`
	LatencyThreshold    time.Duration `yaml:"latency_threshold"`
	ThroughputThreshold int           `yaml:"throughput_threshold"`
	ErrorRateThreshold  float64       `yaml:"error_rate_threshold"`
	CPUThreshold        int           `yaml:"cpu_threshold"`
	MemoryThreshold     int           `yaml:"memory_threshold"`
	DiskThreshold       int           `yaml:"disk_threshold"`
}

// SecurityConfig defines security settings
type SecurityConfig struct {
	RateLimiting             RateLimitingConfig `yaml:"rate_limiting"`
	AdminEndpointsRequire2FA bool               `yaml:"admin_endpoints_require_2fa"`
	APIKeyRotationInterval   time.Duration      `yaml:"api_key_rotation_interval"`
	SessionTimeout           time.Duration      `yaml:"session_timeout"`
	EncryptSensitiveData     bool               `yaml:"encrypt_sensitive_data"`
	EncryptionAlgorithm      string             `yaml:"encryption_algorithm"`
	KeyRotationInterval      time.Duration      `yaml:"key_rotation_interval"`
}

// RateLimitingConfig defines rate limiting settings
type RateLimitingConfig struct {
	Enabled           bool `yaml:"enabled"`
	RequestsPerMinute int  `yaml:"requests_per_minute"`
	BurstSize         int  `yaml:"burst_size"`
}

// IntegrationConfig defines integration settings
type IntegrationConfig struct {
	MarketData   MarketDataConfig   `yaml:"market_data"`
	WebSocket    WebSocketConfig    `yaml:"websocket"`
	MessageQueue MessageQueueConfig `yaml:"message_queue"`
}

// MarketDataConfig defines market data integration settings
type MarketDataConfig struct {
	Provider         string        `yaml:"provider"`
	FallbackProvider string        `yaml:"fallback_provider"`
	Timeout          time.Duration `yaml:"timeout"`
	RetryAttempts    int           `yaml:"retry_attempts"`
}

// WebSocketConfig defines WebSocket settings
type WebSocketConfig struct {
	MaxConnections int           `yaml:"max_connections"`
	PingInterval   time.Duration `yaml:"ping_interval"`
	PongTimeout    time.Duration `yaml:"pong_timeout"`
	WriteTimeout   time.Duration `yaml:"write_timeout"`
	ReadTimeout    time.Duration `yaml:"read_timeout"`
}

// MessageQueueConfig defines message queue settings
type MessageQueueConfig struct {
	Enabled         bool          `yaml:"enabled"`
	MaxRetries      int           `yaml:"max_retries"`
	RetryDelay      time.Duration `yaml:"retry_delay"`
	DeadLetterQueue bool          `yaml:"dead_letter_queue"`
}

// DevelopmentConfig defines development and testing settings
type DevelopmentConfig struct {
	EnableTestMode        bool   `yaml:"enable_test_mode"`
	MockExternalServices  bool   `yaml:"mock_external_services"`
	LogLevel              string `yaml:"log_level"`
	BenchmarkMode         bool   `yaml:"benchmark_mode"`
	ProfilingEnabled      bool   `yaml:"profiling_enabled"`
	MetricsExportEnabled  bool   `yaml:"metrics_export_enabled"`
	SwaggerEnabled        bool   `yaml:"swagger_enabled"`
	DebugEndpointsEnabled bool   `yaml:"debug_endpoints_enabled"`
	CORSEnabled           bool   `yaml:"cors_enabled"`
}

// FeaturesConfig defines feature flags
type FeaturesConfig struct {
	RealTimeRiskCalculation    bool `yaml:"real_time_risk_calculation"`
	BatchRiskProcessing        bool `yaml:"batch_risk_processing"`
	AdvancedComplianceRules    bool `yaml:"advanced_compliance_rules"`
	RegulatoryReporting        bool `yaml:"regulatory_reporting"`
	LiveDashboard              bool `yaml:"live_dashboard"`
	AlertManagement            bool `yaml:"alert_management"`
	PerformanceMonitoring      bool `yaml:"performance_monitoring"`
	RestAPI                    bool `yaml:"rest_api"`
	WebSocketAPI               bool `yaml:"websocket_api"`
	AdminAPI                   bool `yaml:"admin_api"`
	MachineLearningRiskScoring bool `yaml:"machine_learning_risk_scoring"`
	BlockchainAnalysis         bool `yaml:"blockchain_analysis"`
	AdvancedPatternDetection   bool `yaml:"advanced_pattern_detection"`
}

// LoadRiskManagementConfig loads configuration from risk-management.yaml
func LoadRiskManagementConfig(configPath string) (*RiskManagementConfig, error) {
	// Default config path if not specified
	if configPath == "" {
		configPath = filepath.Join("configs", "risk-management.yaml")
	}

	// Check if file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("config file not found: %s", configPath)
	}

	// Read the file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("error reading config file: %w", err)
	}

	// Parse YAML
	var config RiskManagementConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("error parsing config file: %w", err)
	}

	// Validate configuration
	if err := validateConfig(&config); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &config, nil
}

// validateConfig validates the loaded configuration
func validateConfig(config *RiskManagementConfig) error {
	// Validate position limits
	if config.PositionLimits.DefaultUserLimit <= 0 {
		return fmt.Errorf("default_user_limit must be positive")
	}
	if config.PositionLimits.DefaultMarketLimit <= 0 {
		return fmt.Errorf("default_market_limit must be positive")
	}
	if config.PositionLimits.GlobalLimit <= 0 {
		return fmt.Errorf("global_limit must be positive")
	}

	// Validate risk calculation settings
	if config.RiskCalculation.CalculationTimeout <= 0 {
		return fmt.Errorf("calculation_timeout must be positive")
	}
	if config.RiskCalculation.BatchSize <= 0 {
		return fmt.Errorf("batch_size must be positive")
	}
	if config.RiskCalculation.VarConfidence <= 0 || config.RiskCalculation.VarConfidence >= 1 {
		return fmt.Errorf("var_confidence must be between 0 and 1")
	}

	// Validate monitoring thresholds
	if config.Monitoring.LatencyThreshold <= 0 {
		return fmt.Errorf("latency_threshold must be positive")
	}
	if config.Monitoring.ThroughputThreshold <= 0 {
		return fmt.Errorf("throughput_threshold must be positive")
	}
	if config.Monitoring.ErrorRateThreshold < 0 || config.Monitoring.ErrorRateThreshold > 1 {
		return fmt.Errorf("error_rate_threshold must be between 0 and 1")
	}

	return nil
}

// ToAsyncRiskConfig converts RiskManagementConfig to AsyncRiskConfig
func (config *RiskManagementConfig) ToAsyncRiskConfig() *AsyncRiskConfig {
	return &AsyncRiskConfig{
		// Worker configuration
		RiskWorkerCount:  8, // Default from async service
		CacheWorkerCount: 4, // Default from async service

		// Performance targets from config
		TargetLatencyMs:        int(config.RiskCalculation.CalculationTimeout.Milliseconds()),
		TargetThroughputOPS:    config.Monitoring.ThroughputThreshold,
		RiskCalculationTimeout: config.RiskCalculation.CalculationTimeout,

		// Caching configuration
		EnableCaching:       config.Features.PerformanceMonitoring,
		CacheHitRatio:       0.95, // 95% target from async service
		PreloadUserProfiles: true,

		// Batch processing
		EnableBatchProcessing: config.Features.BatchRiskProcessing,
		BatchSize:             config.RiskCalculation.BatchSize,
		BatchTimeout:          config.RiskCalculation.UpdateInterval,

		// Circuit breaker
		EnableFallback:    true,
		FallbackThreshold: config.Monitoring.ErrorRateThreshold,

		// Real-time updates
		EnableRealtimeUpdates: config.Features.RealTimeRiskCalculation,
		PubSubChannels: []string{
			"risk:updates",
			"market:updates",
			"compliance:alerts",
		},
	}
}

// GetDefaultConfig returns a default configuration for testing
func GetDefaultConfig() *RiskManagementConfig {
	return &RiskManagementConfig{
		PositionLimits: PositionLimitsConfig{
			DefaultUserLimit:        100000,
			DefaultMarketLimit:      1000000,
			GlobalLimit:             10000000,
			VIPMultiplier:           10,
			InstitutionalMultiplier: 100,
		},
		RiskCalculation: RiskCalculationConfig{
			VarConfidence:            0.95,
			VarTimeHorizon:           1,
			CalculationTimeout:       500 * time.Millisecond,
			BatchSize:                100,
			UpdateInterval:           1 * time.Second,
			PriceStalenessThreshold:  30 * time.Second,
			VolatilityLookbackPeriod: 30 * 24 * time.Hour,
			CorrelationUpdateFreq:    1 * time.Hour,
		},
		Compliance: ComplianceConfig{
			AML: AMLConfig{
				Enabled:              true,
				DailyVolumeThreshold: 50000,
				StructuringThreshold: 10000,
				VelocityThreshold:    10,
				VelocityTimeWindow:   5 * time.Minute,
			},
			KYT: KYTConfig{
				Enabled:               true,
				HighRiskJurisdictions: []string{"OFAC", "FATF_GREY", "HIGH_RISK"},
				BlockedAddresses:      []string{},
			},
			Patterns: PatternsConfig{
				RoundAmountThreshold:      0.9,
				TimeClusteringThreshold:   60 * time.Second,
				AmountClusteringThreshold: 0.1,
			},
		},
		Dashboard: DashboardConfig{
			RefreshInterval:     5 * time.Second,
			MetricsRetention:    168 * time.Hour,
			AlertRetention:      24 * time.Hour,
			MaxSubscribers:      1000,
			SubscriptionTimeout: 30 * time.Second,
			AlertThresholds: AlertThresholdsConfig{
				HighRiskScore:         0.8,
				CriticalRiskScore:     0.95,
				SystemHealthThreshold: 0.9,
			},
		},
		Monitoring: MonitoringConfig{
			HealthCheckInterval: 30 * time.Second,
			ServiceTimeout:      10 * time.Second,
			LatencyThreshold:    100 * time.Millisecond,
			ThroughputThreshold: 10000,
			ErrorRateThreshold:  0.01,
			CPUThreshold:        80,
			MemoryThreshold:     85,
			DiskThreshold:       90,
		},
		Features: FeaturesConfig{
			RealTimeRiskCalculation:    true,
			BatchRiskProcessing:        true,
			AdvancedComplianceRules:    true,
			RegulatoryReporting:        true,
			LiveDashboard:              true,
			AlertManagement:            true,
			PerformanceMonitoring:      true,
			RestAPI:                    true,
			WebSocketAPI:               true,
			AdminAPI:                   true,
			MachineLearningRiskScoring: false,
			BlockchainAnalysis:         false,
			AdvancedPatternDetection:   false,
		},
	}
}
