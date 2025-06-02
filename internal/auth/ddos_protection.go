package auth

import (
	"context"

	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
	"go.uber.org/zap"
)

// DDoSProtectionManager provides comprehensive DDoS protection mechanisms
type DDoSProtectionManager struct {
	redis               *redis.Client
	logger              *zap.Logger
	securityManager     *EndpointSecurityManager
	config              *DDoSConfig
	attackPatterns      *AttackPatternDetector
	adaptiveThresholds  *AdaptiveThresholdManager
	ipReputationService *IPReputationService
	geoIPService        *GeoIPService

	// In-memory caches for performance
	suspiciousIPs sync.Map // IP -> SuspiciousIPInfo
	activeAttacks sync.Map // AttackID -> AttackInfo
	ruleCache     sync.Map // RuleKey -> CompiledRule
}

// DDoSConfig defines DDoS protection configuration
type DDoSConfig struct {
	// Rate limiting thresholds
	GlobalRateLimit   int           `json:"global_rate_limit"`
	IPRateLimit       int           `json:"ip_rate_limit"`
	EndpointRateLimit int           `json:"endpoint_rate_limit"`
	BurstAllowance    int           `json:"burst_allowance"`
	WindowDuration    time.Duration `json:"window_duration"`

	// Attack detection
	SuspiciousThreshold     int  `json:"suspicious_threshold"`
	AttackThreshold         int  `json:"attack_threshold"`
	AdaptiveEnabled         bool `json:"adaptive_enabled"`
	PatternDetectionEnabled bool `json:"pattern_detection_enabled"`

	// Response actions
	AutoBlockEnabled bool          `json:"auto_block_enabled"`
	BlockDuration    time.Duration `json:"block_duration"`
	ChallengeEnabled bool          `json:"challenge_enabled"`
	TarPitEnabled    bool          `json:"tar_pit_enabled"`
	TarPitDelay      time.Duration `json:"tar_pit_delay"`

	// Geolocation restrictions
	GeoBlockingEnabled bool     `json:"geo_blocking_enabled"`
	AllowedCountries   []string `json:"allowed_countries"`
	BlockedCountries   []string `json:"blocked_countries"`
	HighRiskCountries  []string `json:"high_risk_countries"`

	// Reputation-based filtering
	ReputationEnabled   bool    `json:"reputation_enabled"`
	MinReputationScore  float64 `json:"min_reputation_score"`
	ReputationDecayRate float64 `json:"reputation_decay_rate"`
}

// AttackPatternDetector identifies attack patterns
type AttackPatternDetector struct {
	patterns map[string]*AttackPattern
	mutex    sync.RWMutex
}

// AttackPattern defines characteristics of known attack patterns
type AttackPattern struct {
	Name             string           `json:"name"`
	Description      string           `json:"description"`
	RequestPatterns  []RequestPattern `json:"request_patterns"`
	TimingPatterns   []TimingPattern  `json:"timing_patterns"`
	VolumeThresholds VolumeThreshold  `json:"volume_thresholds"`
	Severity         AttackSeverity   `json:"severity"`
	ResponseAction   ResponseAction   `json:"response_action"`
	LastDetected     time.Time        `json:"last_detected"`
	DetectionCount   int64            `json:"detection_count"`
}

// RequestPattern defines suspicious request characteristics
type RequestPattern struct {
	UserAgentPatterns []string `json:"user_agent_patterns"`
	HeaderPatterns    []string `json:"header_patterns"`
	PayloadPatterns   []string `json:"payload_patterns"`
	URIPatterns       []string `json:"uri_patterns"`
	MethodPatterns    []string `json:"method_patterns"`
}

// TimingPattern defines suspicious timing characteristics
type TimingPattern struct {
	MinInterval     time.Duration `json:"min_interval"`
	MaxInterval     time.Duration `json:"max_interval"`
	BurstPattern    bool          `json:"burst_pattern"`
	RegularityScore float64       `json:"regularity_score"`
}

// VolumeThreshold defines volume-based attack detection
type VolumeThreshold struct {
	RequestsPerSecond  int `json:"requests_per_second"`
	RequestsPerMinute  int `json:"requests_per_minute"`
	RequestsPerHour    int `json:"requests_per_hour"`
	ConcurrentRequests int `json:"concurrent_requests"`
}

// AttackSeverity defines attack severity levels
type AttackSeverity int

const (
	SeverityLow AttackSeverity = iota
	SeverityMedium
	SeverityHigh
	SeverityCritical
)

// ResponseAction defines how to respond to detected attacks
type ResponseAction int

const (
	ActionLog ResponseAction = iota
	ActionRateLimit
	ActionChallenge
	ActionTarPit
	ActionBlock
	ActionNull
)

// AdaptiveThresholdManager dynamically adjusts thresholds based on traffic patterns
type AdaptiveThresholdManager struct {
	baselines      map[string]*TrafficBaseline
	thresholds     map[string]*DynamicThreshold
	learningMode   bool
	adaptationRate float64
	mutex          sync.RWMutex
}

// TrafficBaseline represents normal traffic patterns
type TrafficBaseline struct {
	EndpointPath      string        `json:"endpoint_path"`
	AverageRPS        float64       `json:"average_rps"`
	PeakRPS           float64       `json:"peak_rps"`
	TypicalUserAgents []string      `json:"typical_user_agents"`
	TypicalCountries  []string      `json:"typical_countries"`
	TimePatterns      []TimePattern `json:"time_patterns"`
	LastUpdated       time.Time     `json:"last_updated"`
}

// TimePattern represents traffic patterns by time
type TimePattern struct {
	Hour            int     `json:"hour"`
	DayOfWeek       int     `json:"day_of_week"`
	ExpectedTraffic float64 `json:"expected_traffic"`
}

// DynamicThreshold represents adaptive rate limiting thresholds
type DynamicThreshold struct {
	CurrentLimit     int       `json:"current_limit"`
	BaselineLimit    int       `json:"baseline_limit"`
	MaxLimit         int       `json:"max_limit"`
	MinLimit         int       `json:"min_limit"`
	AdjustmentFactor float64   `json:"adjustment_factor"`
	LastAdjustment   time.Time `json:"last_adjustment"`
}

// SuspiciousIPInfo tracks suspicious IP behavior
type SuspiciousIPInfo struct {
	IP                 string                 `json:"ip"`
	FirstSeen          time.Time              `json:"first_seen"`
	LastSeen           time.Time              `json:"last_seen"`
	RequestCount       int64                  `json:"request_count"`
	SuspiciousEvents   []SuspiciousEvent      `json:"suspicious_events"`
	ReputationScore    float64                `json:"reputation_score"`
	CountryCode        string                 `json:"country_code"`
	ASN                int                    `json:"asn"`
	ISP                string                 `json:"isp"`
	ThreatIntelligence map[string]interface{} `json:"threat_intelligence"`
	Status             IPStatus               `json:"status"`
}

// SuspiciousEvent represents a suspicious activity
type SuspiciousEvent struct {
	Timestamp   time.Time         `json:"timestamp"`
	EventType   string            `json:"event_type"`
	Severity    AttackSeverity    `json:"severity"`
	Description string            `json:"description"`
	Endpoint    string            `json:"endpoint"`
	UserAgent   string            `json:"user_agent"`
	Headers     map[string]string `json:"headers"`
}

// IPStatus represents the current status of an IP
type IPStatus int

const (
	IPStatusClean IPStatus = iota
	IPStatusSuspicious
	IPStatusBlocked
	IPStatusWhitelisted
)

// NewDDoSProtectionManager creates a new DDoS protection manager
func NewDDoSProtectionManager(redis *redis.Client, logger *zap.Logger, securityManager *EndpointSecurityManager) *DDoSProtectionManager {
	config := &DDoSConfig{
		GlobalRateLimit:         10000,
		IPRateLimit:             100,
		EndpointRateLimit:       1000,
		BurstAllowance:          50,
		WindowDuration:          time.Minute,
		SuspiciousThreshold:     20,
		AttackThreshold:         50,
		AdaptiveEnabled:         true,
		PatternDetectionEnabled: true,
		AutoBlockEnabled:        true,
		BlockDuration:           time.Hour,
		ChallengeEnabled:        true,
		TarPitEnabled:           true,
		TarPitDelay:             time.Second * 5,
		GeoBlockingEnabled:      false,
		ReputationEnabled:       true,
		MinReputationScore:      0.3,
		ReputationDecayRate:     0.1,
	}

	manager := &DDoSProtectionManager{
		redis:               redis,
		logger:              logger,
		securityManager:     securityManager,
		config:              config,
		attackPatterns:      NewAttackPatternDetector(),
		adaptiveThresholds:  NewAdaptiveThresholdManager(),
		ipReputationService: NewIPReputationService(redis, logger),
		geoIPService:        NewGeoIPService(),
	}

	// Initialize default attack patterns
	manager.initializeAttackPatterns()

	return manager
}

// CheckRequest evaluates a request for DDoS patterns and applies protection
func (dm *DDoSProtectionManager) CheckRequest(ctx context.Context, req *RequestContext) (*ProtectionResult, error) {
	result := &ProtectionResult{
		Allowed:  true,
		Action:   ActionLog,
		Reason:   "clean_request",
		Metadata: make(map[string]interface{}),
	}

	// Extract IP address
	ip := dm.extractIPAddress(req)
	if ip == "" {
		result.Allowed = false
		result.Action = ActionBlock
		result.Reason = "invalid_ip"
		return result, nil
	}

	// Check if IP is whitelisted
	if dm.isWhitelisted(ip) {
		result.Metadata["whitelisted"] = true
		return result, nil
	}

	// Check if IP is already blocked
	if blocked, reason := dm.isBlocked(ctx, ip); blocked {
		result.Allowed = false
		result.Action = ActionBlock
		result.Reason = reason
		result.Metadata["blocked_until"] = dm.getBlockExpiry(ctx, ip)
		return result, nil
	}

	// Geographic restrictions
	if dm.config.GeoBlockingEnabled {
		if blocked, reason := dm.checkGeographicRestrictions(ip); blocked {
			result.Allowed = false
			result.Action = ActionBlock
			result.Reason = reason
			dm.blockIP(ctx, ip, dm.config.BlockDuration, reason)
			return result, nil
		}
	}

	// IP reputation check
	if dm.config.ReputationEnabled {
		reputation := dm.ipReputationService.GetReputationScore(ctx, ip)
		if reputation < dm.config.MinReputationScore {
			result.Allowed = false
			result.Action = ActionBlock
			result.Reason = "low_reputation"
			result.Metadata["reputation_score"] = reputation
			dm.blockIP(ctx, ip, dm.config.BlockDuration, "low_reputation")
			return result, nil
		}
		result.Metadata["reputation_score"] = reputation
	}

	// Rate limiting checks
	if exceeded, action := dm.checkRateLimits(ctx, req, ip); exceeded {
		result.Allowed = false
		result.Action = action
		result.Reason = "rate_limit_exceeded"

		// Record suspicious activity
		dm.recordSuspiciousActivity(ip, req, "rate_limit_exceeded")

		return result, nil
	}

	// Attack pattern detection
	if dm.config.PatternDetectionEnabled {
		if detected, pattern := dm.detectAttackPatterns(ctx, req, ip); detected {
			result.Action = pattern.ResponseAction
			if pattern.Severity >= SeverityHigh {
				result.Allowed = false
				dm.blockIP(ctx, ip, dm.config.BlockDuration, fmt.Sprintf("attack_pattern_%s", pattern.Name))
			}
			result.Reason = fmt.Sprintf("attack_pattern_%s", pattern.Name)
			result.Metadata["attack_pattern"] = pattern.Name
			result.Metadata["attack_severity"] = pattern.Severity

			dm.recordSuspiciousActivity(ip, req, fmt.Sprintf("attack_pattern_%s", pattern.Name))
		}
	}

	// Update IP tracking
	dm.updateIPTracking(ip, req)

	return result, nil
}

// RequestContext contains request information for analysis
type RequestContext struct {
	Method      string            `json:"method"`
	Path        string            `json:"path"`
	Headers     map[string]string `json:"headers"`
	UserAgent   string            `json:"user_agent"`
	Referer     string            `json:"referer"`
	RemoteAddr  string            `json:"remote_addr"`
	RealIP      string            `json:"real_ip"`
	Timestamp   time.Time         `json:"timestamp"`
	PayloadSize int64             `json:"payload_size"`
}

// ProtectionResult contains the result of DDoS protection analysis
type ProtectionResult struct {
	Allowed   bool                   `json:"allowed"`
	Action    ResponseAction         `json:"action"`
	Reason    string                 `json:"reason"`
	Delay     time.Duration          `json:"delay,omitempty"`
	Challenge *ChallengeRequest      `json:"challenge,omitempty"`
	Metadata  map[string]interface{} `json:"metadata"`
}

// ChallengeRequest represents a challenge to be presented to the client
type ChallengeRequest struct {
	Type      string    `json:"type"`
	Challenge string    `json:"challenge"`
	ExpiresAt time.Time `json:"expires_at"`
}

// extractIPAddress extracts the real IP address from request context
func (dm *DDoSProtectionManager) extractIPAddress(req *RequestContext) string {
	// Check X-Real-IP header first
	if req.RealIP != "" {
		return req.RealIP
	}

	// Check X-Forwarded-For header
	if forwardedFor, exists := req.Headers["X-Forwarded-For"]; exists {
		ips := strings.Split(forwardedFor, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	// Fall back to remote address
	if req.RemoteAddr != "" {
		host, _, err := net.SplitHostPort(req.RemoteAddr)
		if err != nil {
			return req.RemoteAddr
		}
		return host
	}

	return ""
}

// checkRateLimits performs comprehensive rate limiting checks
func (dm *DDoSProtectionManager) checkRateLimits(ctx context.Context, req *RequestContext, ip string) (bool, ResponseAction) {
	now := time.Now()

	// Get endpoint-specific rate limit tier
	tier := dm.securityManager.GetRateLimitTier(req.Path)

	// Check global rate limit
	globalKey := fmt.Sprintf("ddos:global:%s", now.Format("2006-01-02-15-04"))
	globalCount, err := dm.redis.Incr(ctx, globalKey).Result()
	if err == nil {
		dm.redis.Expire(ctx, globalKey, time.Minute)
		if int(globalCount) > dm.config.GlobalRateLimit {
			return true, ActionTarPit
		}
	}

	// Check IP-specific rate limit
	ipKey := fmt.Sprintf("ddos:ip:%s:%s", ip, now.Format("2006-01-02-15-04"))
	ipCount, err := dm.redis.Incr(ctx, ipKey).Result()
	if err == nil {
		dm.redis.Expire(ctx, ipKey, time.Minute)
		limit := dm.getIPRateLimit(tier)
		if int(ipCount) > limit {
			return true, ActionBlock
		}
	}

	// Check endpoint-specific rate limit
	endpointKey := fmt.Sprintf("ddos:endpoint:%s:%s", req.Path, now.Format("2006-01-02-15-04"))
	endpointCount, err := dm.redis.Incr(ctx, endpointKey).Result()
	if err == nil {
		dm.redis.Expire(ctx, endpointKey, time.Minute)
		limit := dm.getEndpointRateLimit(req.Path, tier)
		if int(endpointCount) > limit {
			return true, ActionRateLimit
		}
	}

	return false, ActionLog
}

// getIPRateLimit returns the rate limit for an IP based on tier
func (dm *DDoSProtectionManager) getIPRateLimit(tier RateLimitTier) int {
	switch tier {
	case RateLimitTierStrict:
		return dm.config.IPRateLimit / 4
	case RateLimitTierModerate:
		return dm.config.IPRateLimit / 2
	case RateLimitTierLenient:
		return dm.config.IPRateLimit
	case RateLimitTierPublic:
		return dm.config.IPRateLimit * 2
	default:
		return dm.config.IPRateLimit
	}
}

// getEndpointRateLimit returns the rate limit for an endpoint
func (dm *DDoSProtectionManager) getEndpointRateLimit(path string, tier RateLimitTier) int {
	base := dm.config.EndpointRateLimit

	// Adjust based on endpoint criticality
	if dm.securityManager.IsCriticalTradingEndpoint(path) {
		base *= 10 // Trading endpoints need higher limits
	}

	// Adjust based on tier
	switch tier {
	case RateLimitTierStrict:
		return base / 4
	case RateLimitTierModerate:
		return base / 2
	case RateLimitTierLenient:
		return base
	case RateLimitTierPublic:
		return base * 2
	default:
		return base
	}
}

// Additional methods would continue here...
// For brevity, I'm including the key structure and main methods
// The full implementation would include all the helper methods

// NewAttackPatternDetector creates a new attack pattern detector
func NewAttackPatternDetector() *AttackPatternDetector {
	return &AttackPatternDetector{
		patterns: make(map[string]*AttackPattern),
	}
}

// NewAdaptiveThresholdManager creates a new adaptive threshold manager
func NewAdaptiveThresholdManager() *AdaptiveThresholdManager {
	return &AdaptiveThresholdManager{
		baselines:      make(map[string]*TrafficBaseline),
		thresholds:     make(map[string]*DynamicThreshold),
		learningMode:   true,
		adaptationRate: 0.1,
	}
}
