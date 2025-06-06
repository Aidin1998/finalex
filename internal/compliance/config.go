package compliance

import (
	"fmt"
	"time"
)

// Config holds configuration for the compliance module
type Config struct {
	Database     DatabaseConfig     `yaml:"database" json:"database"`
	Audit        AuditConfig        `yaml:"audit" json:"audit"`
	Compliance   ComplianceConfig   `yaml:"compliance" json:"compliance"`
	Manipulation ManipulationConfig `yaml:"manipulation" json:"manipulation"`
	Monitoring   MonitoringConfig   `yaml:"monitoring" json:"monitoring"`
	Performance  PerformanceConfig  `yaml:"performance" json:"performance"`
}

// DatabaseConfig holds database configuration
type DatabaseConfig struct {
	Host            string        `yaml:"host" json:"host"`
	Port            int           `yaml:"port" json:"port"`
	Database        string        `yaml:"database" json:"database"`
	Username        string        `yaml:"username" json:"username"`
	Password        string        `yaml:"password" json:"password"`
	SSLMode         string        `yaml:"ssl_mode" json:"ssl_mode"`
	MaxConnections  int           `yaml:"max_connections" json:"max_connections"`
	ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime" json:"conn_max_lifetime"`
	ConnMaxIdleTime time.Duration `yaml:"conn_max_idle_time" json:"conn_max_idle_time"`
}

// AuditConfig holds audit service configuration
type AuditConfig struct {
	Workers             int           `yaml:"workers" json:"workers"`
	BatchSize           int           `yaml:"batch_size" json:"batch_size"`
	FlushInterval       time.Duration `yaml:"flush_interval" json:"flush_interval"`
	EnableEncryption    bool          `yaml:"enable_encryption" json:"enable_encryption"`
	EnableCompression   bool          `yaml:"enable_compression" json:"enable_compression"`
	RetentionDays       int           `yaml:"retention_days" json:"retention_days"`
	VerifyChain         bool          `yaml:"verify_chain" json:"verify_chain"`
	ChainVerifyInterval time.Duration `yaml:"chain_verify_interval" json:"chain_verify_interval"`
}

// ComplianceConfig holds compliance service configuration
type ComplianceConfig struct {
	EnableKYC               bool          `yaml:"enable_kyc" json:"enable_kyc"`
	EnableAML               bool          `yaml:"enable_aml" json:"enable_aml"`
	EnableSanctions         bool          `yaml:"enable_sanctions" json:"enable_sanctions"`
	KYCRequiredLevel        string        `yaml:"kyc_required_level" json:"kyc_required_level"`
	AMLRiskThreshold        float64       `yaml:"aml_risk_threshold" json:"aml_risk_threshold"`
	SanctionsUpdateInterval time.Duration `yaml:"sanctions_update_interval" json:"sanctions_update_interval"`
	PolicyUpdateInterval    time.Duration `yaml:"policy_update_interval" json:"policy_update_interval"`
	CacheSize               int           `yaml:"cache_size" json:"cache_size"`
	CacheTTL                time.Duration `yaml:"cache_ttl" json:"cache_ttl"`
}

// ManipulationConfig holds manipulation detection configuration
type ManipulationConfig struct {
	EnableDetection     bool          `yaml:"enable_detection" json:"enable_detection"`
	DetectionInterval   time.Duration `yaml:"detection_interval" json:"detection_interval"`
	RiskThreshold       float64       `yaml:"risk_threshold" json:"risk_threshold"`
	AlertThreshold      float64       `yaml:"alert_threshold" json:"alert_threshold"`
	LookbackPeriod      time.Duration `yaml:"lookback_period" json:"lookback_period"`
	MaxPatterns         int           `yaml:"max_patterns" json:"max_patterns"`
	EnableML            bool          `yaml:"enable_ml" json:"enable_ml"`
	MLModelPath         string        `yaml:"ml_model_path" json:"ml_model_path"`
	UpdateRulesInterval time.Duration `yaml:"update_rules_interval" json:"update_rules_interval"`
}

// MonitoringConfig holds monitoring service configuration
type MonitoringConfig struct {
	Workers             int           `yaml:"workers" json:"workers"`
	AlertQueueSize      int           `yaml:"alert_queue_size" json:"alert_queue_size"`
	PolicyCheckInterval time.Duration `yaml:"policy_check_interval" json:"policy_check_interval"`
	MetricsInterval     time.Duration `yaml:"metrics_interval" json:"metrics_interval"`
	HealthCheckInterval time.Duration `yaml:"health_check_interval" json:"health_check_interval"`
	EnableNotifications bool          `yaml:"enable_notifications" json:"enable_notifications"`
	NotificationWebhook string        `yaml:"notification_webhook" json:"notification_webhook"`
}

// PerformanceConfig holds performance-related configuration
type PerformanceConfig struct {
	MaxConcurrentRequests int           `yaml:"max_concurrent_requests" json:"max_concurrent_requests"`
	RequestTimeout        time.Duration `yaml:"request_timeout" json:"request_timeout"`
	EnableRateLimiting    bool          `yaml:"enable_rate_limiting" json:"enable_rate_limiting"`
	RateLimit             int           `yaml:"rate_limit" json:"rate_limit"`
	RateLimitWindow       time.Duration `yaml:"rate_limit_window" json:"rate_limit_window"`
	EnableCaching         bool          `yaml:"enable_caching" json:"enable_caching"`
	CacheSize             int           `yaml:"cache_size" json:"cache_size"`
	CacheTTL              time.Duration `yaml:"cache_ttl" json:"cache_ttl"`
}

// DefaultConfig returns a default configuration
func DefaultConfig() *Config {
	return &Config{
		Database: DatabaseConfig{
			Host:            "localhost",
			Port:            5432,
			Database:        "compliance",
			Username:        "compliance_user",
			Password:        "compliance_pass",
			SSLMode:         "require",
			MaxConnections:  50,
			ConnMaxLifetime: 1 * time.Hour,
			ConnMaxIdleTime: 15 * time.Minute,
		},
		Audit: AuditConfig{
			Workers:             4,
			BatchSize:           100,
			FlushInterval:       5 * time.Second,
			EnableEncryption:    true,
			EnableCompression:   true,
			RetentionDays:       2555, // 7 years
			VerifyChain:         true,
			ChainVerifyInterval: 1 * time.Hour,
		},
		Compliance: ComplianceConfig{
			EnableKYC:               true,
			EnableAML:               true,
			EnableSanctions:         true,
			KYCRequiredLevel:        "enhanced",
			AMLRiskThreshold:        0.7,
			SanctionsUpdateInterval: 1 * time.Hour,
			PolicyUpdateInterval:    30 * time.Minute,
			CacheSize:               10000,
			CacheTTL:                30 * time.Minute,
		},
		Manipulation: ManipulationConfig{
			EnableDetection:     true,
			DetectionInterval:   1 * time.Minute,
			RiskThreshold:       0.6,
			AlertThreshold:      0.8,
			LookbackPeriod:      24 * time.Hour,
			MaxPatterns:         1000,
			EnableML:            false,
			MLModelPath:         "",
			UpdateRulesInterval: 1 * time.Hour,
		},
		Monitoring: MonitoringConfig{
			Workers:             4,
			AlertQueueSize:      1000,
			PolicyCheckInterval: 1 * time.Minute,
			MetricsInterval:     30 * time.Second,
			HealthCheckInterval: 30 * time.Second,
			EnableNotifications: true,
			NotificationWebhook: "",
		},
		Performance: PerformanceConfig{
			MaxConcurrentRequests: 1000,
			RequestTimeout:        30 * time.Second,
			EnableRateLimiting:    true,
			RateLimit:             100,
			RateLimitWindow:       1 * time.Minute,
			EnableCaching:         true,
			CacheSize:             10000,
			CacheTTL:              15 * time.Minute,
		},
	}
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.Database.Host == "" {
		return fmt.Errorf("database host is required")
	}
	if c.Database.Port <= 0 {
		return fmt.Errorf("database port must be positive")
	}
	if c.Database.Database == "" {
		return fmt.Errorf("database name is required")
	}
	if c.Audit.Workers <= 0 {
		return fmt.Errorf("audit workers must be positive")
	}
	if c.Audit.BatchSize <= 0 {
		return fmt.Errorf("audit batch size must be positive")
	}
	if c.Monitoring.Workers <= 0 {
		return fmt.Errorf("monitoring workers must be positive")
	}
	if c.Performance.MaxConcurrentRequests <= 0 {
		return fmt.Errorf("max concurrent requests must be positive")
	}
	return nil
}
