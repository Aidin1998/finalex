package auth

import (
	"fmt"
	"time"

	"go.uber.org/zap"
)

// SecurityConfig represents comprehensive security configuration
type SecurityConfig struct {
	// Hashing Configuration
	Hashing HashingConfig `json:"hashing" yaml:"hashing"`
	// Rate Limiting Configuration
	RateLimit SecurityRateLimitConfig `json:"rate_limit" yaml:"rate_limit"`

	// DDoS Protection Configuration
	DDoS DDoSConfig `json:"ddos" yaml:"ddos"`

	// IP Reputation Configuration
	IPReputation IPReputationConfig `json:"ip_reputation" yaml:"ip_reputation"`

	// GeoIP Configuration
	GeoIP GeoIPConfig `json:"geoip" yaml:"geoip"`

	// Enhanced Security Features
	Enhanced EnhancedSecurityConfig `json:"enhanced" yaml:"enhanced"`
}

// HashingConfig contains hashing algorithm configurations
type HashingConfig struct {
	// Bcrypt configuration
	BcryptCost int `json:"bcrypt_cost" yaml:"bcrypt_cost" default:"12"`

	// Argon2 configuration
	Argon2Time    uint32 `json:"argon2_time" yaml:"argon2_time" default:"1"`
	Argon2Memory  uint32 `json:"argon2_memory" yaml:"argon2_memory" default:"65536"` // 64MB
	Argon2Threads uint8  `json:"argon2_threads" yaml:"argon2_threads" default:"4"`
	Argon2KeyLen  uint32 `json:"argon2_key_len" yaml:"argon2_key_len" default:"32"`
	Argon2SaltLen int    `json:"argon2_salt_len" yaml:"argon2_salt_len" default:"16"`

	// Hash upgrade settings
	EnableHashUpgrade        bool          `json:"enable_hash_upgrade" yaml:"enable_hash_upgrade" default:"true"`
	HashUpgradeCheckInterval time.Duration `json:"hash_upgrade_check_interval" yaml:"hash_upgrade_check_interval" default:"24h"`
	AutoUpgradeNonCritical   bool          `json:"auto_upgrade_non_critical" yaml:"auto_upgrade_non_critical" default:"true"`
}

// SecurityRateLimitConfig contains rate limiting configurations for security
type SecurityRateLimitConfig struct {
	// Global rate limits
	GlobalRequestsPerMinute int `json:"global_requests_per_minute" yaml:"global_requests_per_minute" default:"10000"`
	GlobalBurst             int `json:"global_burst" yaml:"global_burst" default:"1000"`

	// Per-IP rate limits
	IPRequestsPerMinute int `json:"ip_requests_per_minute" yaml:"ip_requests_per_minute" default:"100"`
	IPBurst             int `json:"ip_burst" yaml:"ip_burst" default:"50"`

	// Per-endpoint rate limits
	EndpointLimits map[string]EndpointRateLimit `json:"endpoint_limits" yaml:"endpoint_limits"`

	// Adaptive rate limiting
	EnableAdaptive           bool          `json:"enable_adaptive" yaml:"enable_adaptive" default:"true"`
	AdaptiveThresholdFactor  float64       `json:"adaptive_threshold_factor" yaml:"adaptive_threshold_factor" default:"1.5"`
	AdaptiveAdjustmentPeriod time.Duration `json:"adaptive_adjustment_period" yaml:"adaptive_adjustment_period" default:"5m"`
}

// EndpointRateLimit defines rate limits for specific endpoints
type EndpointRateLimit struct {
	RequestsPerMinute int `json:"requests_per_minute" yaml:"requests_per_minute"`
	Burst             int `json:"burst" yaml:"burst"`
}

// DDoSConfig contains DDoS protection configurations
type DDoSConfig struct {
	// Detection thresholds
	SuspiciousRequestThreshold int           `json:"suspicious_request_threshold" yaml:"suspicious_request_threshold" default:"1000"`
	AttackRequestThreshold     int           `json:"attack_request_threshold" yaml:"attack_request_threshold" default:"5000"`
	DetectionWindow            time.Duration `json:"detection_window" yaml:"detection_window" default:"1m"`

	// Response actions
	EnableChallengeResponse bool          `json:"enable_challenge_response" yaml:"enable_challenge_response" default:"true"`
	EnableTarPit            bool          `json:"enable_tar_pit" yaml:"enable_tar_pit" default:"true"`
	TarPitDelay             time.Duration `json:"tar_pit_delay" yaml:"tar_pit_delay" default:"5s"`
	BlockDuration           time.Duration `json:"block_duration" yaml:"block_duration" default:"1h"`

	// Pattern detection
	EnablePatternDetection     bool    `json:"enable_pattern_detection" yaml:"enable_pattern_detection" default:"true"`
	PatternSimilarityThreshold float64 `json:"pattern_similarity_threshold" yaml:"pattern_similarity_threshold" default:"0.8"`
}

// IPReputationConfig contains IP reputation configurations
type IPReputationConfig struct {
	// Enable/disable features
	EnableThreatIntelligence bool `json:"enable_threat_intelligence" yaml:"enable_threat_intelligence" default:"true"`
	EnableBehavioralAnalysis bool `json:"enable_behavioral_analysis" yaml:"enable_behavioral_analysis" default:"true"`

	// Threat intelligence
	ThreatFeedUpdateInterval time.Duration `json:"threat_feed_update_interval" yaml:"threat_feed_update_interval" default:"1h"`
	ThreatFeedSources        []string      `json:"threat_feed_sources" yaml:"threat_feed_sources"`

	// Behavioral analysis
	BehaviorAnalysisWindow  time.Duration `json:"behavior_analysis_window" yaml:"behavior_analysis_window" default:"15m"`
	SuspiciousBehaviorScore float64       `json:"suspicious_behavior_score" yaml:"suspicious_behavior_score" default:"7.0"`
	MaliciousBehaviorScore  float64       `json:"malicious_behavior_score" yaml:"malicious_behavior_score" default:"9.0"`

	// Reputation scoring
	ReputationDecayFactor    float64       `json:"reputation_decay_factor" yaml:"reputation_decay_factor" default:"0.1"`
	ReputationUpdateInterval time.Duration `json:"reputation_update_interval" yaml:"reputation_update_interval" default:"5m"`
}

// GeoIPConfig contains GeoIP configurations
type GeoIPConfig struct {
	// Database configuration
	DatabasePath           string        `json:"database_path" yaml:"database_path"`
	DatabaseUpdateInterval time.Duration `json:"database_update_interval" yaml:"database_update_interval" default:"24h"`

	// Country restrictions
	AllowedCountries  []string `json:"allowed_countries" yaml:"allowed_countries"`
	BlockedCountries  []string `json:"blocked_countries" yaml:"blocked_countries"`
	HighRiskCountries []string `json:"high_risk_countries" yaml:"high_risk_countries"`

	// VPN/Proxy detection
	EnableVPNDetection   bool    `json:"enable_vpn_detection" yaml:"enable_vpn_detection" default:"true"`
	EnableProxyDetection bool    `json:"enable_proxy_detection" yaml:"enable_proxy_detection" default:"true"`
	EnableTorDetection   bool    `json:"enable_tor_detection" yaml:"enable_tor_detection" default:"true"`
	VPNBlockScore        float64 `json:"vpn_block_score" yaml:"vpn_block_score" default:"8.0"`
}

// EnhancedSecurityConfig contains enhanced security feature configurations
type EnhancedSecurityConfig struct {
	// Security monitoring
	EnableSecurityEventLogging bool `json:"enable_security_event_logging" yaml:"enable_security_event_logging" default:"true"`
	EnableRealTimeMonitoring   bool `json:"enable_real_time_monitoring" yaml:"enable_real_time_monitoring" default:"true"`
	EnableAnomalyDetection     bool `json:"enable_anomaly_detection" yaml:"enable_anomaly_detection" default:"true"`

	// Performance monitoring
	EnablePerformanceMonitoring bool          `json:"enable_performance_monitoring" yaml:"enable_performance_monitoring" default:"true"`
	PerformanceAlertThreshold   time.Duration `json:"performance_alert_threshold" yaml:"performance_alert_threshold" default:"100ms"`

	// Security headers
	EnableSecurityHeaders bool   `json:"enable_security_headers" yaml:"enable_security_headers" default:"true"`
	CSPPolicy             string `json:"csp_policy" yaml:"csp_policy"`
	HSTSMaxAge            int    `json:"hsts_max_age" yaml:"hsts_max_age" default:"31536000"`

	// API Security
	EnableAPIKeyRotation      bool          `json:"enable_api_key_rotation" yaml:"enable_api_key_rotation" default:"true"`
	APIKeyRotationInterval    time.Duration `json:"api_key_rotation_interval" yaml:"api_key_rotation_interval" default:"90d"`
	EnableEndpointFingerprint bool          `json:"enable_endpoint_fingerprint" yaml:"enable_endpoint_fingerprint" default:"true"`
}

// DefaultSecurityConfig returns a default security configuration
func DefaultSecurityConfig() *SecurityConfig {
	return &SecurityConfig{
		Hashing: HashingConfig{
			BcryptCost:               12,
			Argon2Time:               1,
			Argon2Memory:             65536,
			Argon2Threads:            4,
			Argon2KeyLen:             32,
			Argon2SaltLen:            16,
			EnableHashUpgrade:        true,
			HashUpgradeCheckInterval: 24 * time.Hour,
			AutoUpgradeNonCritical:   true,
		},
		RateLimit: SecurityRateLimitConfig{
			GlobalRequestsPerMinute: 10000,
			GlobalBurst:             1000,
			IPRequestsPerMinute:     100,
			IPBurst:                 50,
			EndpointLimits: map[string]EndpointRateLimit{
				"/api/v1/trade/order":   {RequestsPerMinute: 1000, Burst: 100},
				"/api/v1/trade/cancel":  {RequestsPerMinute: 500, Burst: 50},
				"/api/v1/user/login":    {RequestsPerMinute: 10, Burst: 5},
				"/api/v1/user/register": {RequestsPerMinute: 5, Burst: 2},
			},
			EnableAdaptive:           true,
			AdaptiveThresholdFactor:  1.5,
			AdaptiveAdjustmentPeriod: 5 * time.Minute,
		},
		DDoS: DDoSConfig{
			SuspiciousRequestThreshold: 1000,
			AttackRequestThreshold:     5000,
			DetectionWindow:            time.Minute,
			EnableChallengeResponse:    true,
			EnableTarPit:               true,
			TarPitDelay:                5 * time.Second,
			BlockDuration:              time.Hour,
			EnablePatternDetection:     true,
			PatternSimilarityThreshold: 0.8,
		},
		IPReputation: IPReputationConfig{
			EnableThreatIntelligence: true,
			EnableBehavioralAnalysis: true,
			ThreatFeedUpdateInterval: time.Hour,
			BehaviorAnalysisWindow:   15 * time.Minute,
			SuspiciousBehaviorScore:  7.0,
			MaliciousBehaviorScore:   9.0,
			ReputationDecayFactor:    0.1,
			ReputationUpdateInterval: 5 * time.Minute,
		},
		GeoIP: GeoIPConfig{
			DatabaseUpdateInterval: 24 * time.Hour,
			EnableVPNDetection:     true,
			EnableProxyDetection:   true,
			EnableTorDetection:     true,
			VPNBlockScore:          8.0,
			HighRiskCountries:      []string{"CN", "RU", "KP", "IR"},
		},
		Enhanced: EnhancedSecurityConfig{
			EnableSecurityEventLogging:  true,
			EnableRealTimeMonitoring:    true,
			EnableAnomalyDetection:      true,
			EnablePerformanceMonitoring: true,
			PerformanceAlertThreshold:   100 * time.Millisecond,
			EnableSecurityHeaders:       true,
			HSTSMaxAge:                  31536000,
			EnableAPIKeyRotation:        true,
			APIKeyRotationInterval:      90 * 24 * time.Hour,
			EnableEndpointFingerprint:   true,
		},
	}
}

// SecurityConfigManager manages security configuration
type SecurityConfigManager struct {
	config *SecurityConfig
	logger *zap.Logger
}

// NewSecurityConfigManager creates a new security configuration manager
func NewSecurityConfigManager(config *SecurityConfig, logger *zap.Logger) *SecurityConfigManager {
	if config == nil {
		config = DefaultSecurityConfig()
	}

	return &SecurityConfigManager{
		config: config,
		logger: logger,
	}
}

// GetConfig returns the current security configuration
func (scm *SecurityConfigManager) GetConfig() *SecurityConfig {
	return scm.config
}

// UpdateConfig updates the security configuration
func (scm *SecurityConfigManager) UpdateConfig(newConfig *SecurityConfig) {
	scm.config = newConfig
	scm.logger.Info("Security configuration updated")
}

// GetHashingConfig returns hashing configuration
func (scm *SecurityConfigManager) GetHashingConfig() HashingConfig {
	return scm.config.Hashing
}

// GetRateLimitConfig returns rate limiting configuration
func (scm *SecurityConfigManager) GetRateLimitConfig() SecurityRateLimitConfig {
	return scm.config.RateLimit
}

// GetDDoSConfig returns DDoS protection configuration
func (scm *SecurityConfigManager) GetDDoSConfig() DDoSConfig {
	return scm.config.DDoS
}

// GetIPReputationConfig returns IP reputation configuration
func (scm *SecurityConfigManager) GetIPReputationConfig() IPReputationConfig {
	return scm.config.IPReputation
}

// GetGeoIPConfig returns GeoIP configuration
func (scm *SecurityConfigManager) GetGeoIPConfig() GeoIPConfig {
	return scm.config.GeoIP
}

// GetEnhancedSecurityConfig returns enhanced security configuration
func (scm *SecurityConfigManager) GetEnhancedSecurityConfig() EnhancedSecurityConfig {
	return scm.config.Enhanced
}

// ValidateConfig validates the security configuration
func (scm *SecurityConfigManager) ValidateConfig() error {
	config := scm.config

	// Validate hashing configuration
	if config.Hashing.BcryptCost < 10 || config.Hashing.BcryptCost > 15 {
		return fmt.Errorf("bcrypt cost must be between 10 and 15")
	}

	if config.Hashing.Argon2Memory < 32*1024 {
		return fmt.Errorf("argon2 memory must be at least 32MB")
	}

	// Validate rate limiting configuration
	if config.RateLimit.GlobalRequestsPerMinute <= 0 {
		return fmt.Errorf("global requests per minute must be positive")
	}

	// Validate DDoS configuration
	if config.DDoS.AttackRequestThreshold <= config.DDoS.SuspiciousRequestThreshold {
		return fmt.Errorf("attack threshold must be greater than suspicious threshold")
	}

	// Validate IP reputation configuration
	if config.IPReputation.SuspiciousBehaviorScore >= config.IPReputation.MaliciousBehaviorScore {
		return fmt.Errorf("malicious behavior score must be greater than suspicious score")
	}

	scm.logger.Info("Security configuration validated successfully")
	return nil
}
