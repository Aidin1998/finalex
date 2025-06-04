package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
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
	EventType   string                 `json:"event_type"`
	Timestamp   time.Time              `json:"timestamp"`
	Severity    AttackSeverity         `json:"severity"`
	Description string                 `json:"description"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// IPStatus represents the current status of an IP
type IPStatus int

const (
	IPStatusClean IPStatus = iota
	IPStatusSuspicious
	IPStatusBlocked
	IPStatusWhitelisted
)

// RequestContext contains request information for analysis
type RequestContext struct {
	Method    string            `json:"method"`
	Path      string            `json:"path"`
	Headers   map[string]string `json:"headers"`
	UserAgent string            `json:"user_agent"`
	IP        string            `json:"ip"`
	Timestamp time.Time         `json:"timestamp"`
	UserID    string            `json:"user_id,omitempty"`
	SessionID string            `json:"session_id,omitempty"`
}

// ProtectionResult contains the result of DDoS protection analysis
type ProtectionResult struct {
	Action         ResponseAction         `json:"action"`
	Allowed        bool                   `json:"allowed"`
	Reason         string                 `json:"reason"`
	Severity       AttackSeverity         `json:"severity"`
	RateLimitInfo  map[string]interface{} `json:"rate_limit_info"`
	Challenge      *ChallengeRequest      `json:"challenge,omitempty"`
	TarPitDuration time.Duration          `json:"tar_pit_duration,omitempty"`
	BlockDuration  time.Duration          `json:"block_duration,omitempty"`
	Metadata       map[string]interface{} `json:"metadata"`
	AnalysisTime   time.Duration          `json:"analysis_time"`
	ThresholdInfo  map[string]interface{} `json:"threshold_info"`
}

// ChallengeRequest represents a challenge to be presented to the client
type ChallengeRequest struct {
	Type       string                 `json:"type"`
	Token      string                 `json:"token"`
	ExpiresAt  time.Time              `json:"expires_at"`
	Difficulty int                    `json:"difficulty"`
	Parameters map[string]interface{} `json:"parameters"`
}

// NewDDoSProtectionManager creates a new DDoS protection manager
func NewDDoSProtectionManager(redis *redis.Client, logger *zap.Logger, securityManager *EndpointSecurityManager) *DDoSProtectionManager {
	// Use default config if none provided
	config := &DDoSConfig{
		SuspiciousRequestThreshold: 1000,
		AttackRequestThreshold:     5000,
		DetectionWindow:            time.Minute,
		EnableChallengeResponse:    true,
		EnableTarPit:               true,
		TarPitDelay:                5 * time.Second,
		BlockDuration:              time.Hour,
		EnablePatternDetection:     true,
		PatternSimilarityThreshold: 0.8,
	}

	manager := &DDoSProtectionManager{
		redis:               redis,
		logger:              logger,
		securityManager:     securityManager,
		config:              config,
		attackPatterns:      NewAttackPatternDetector(),
		adaptiveThresholds:  NewAdaptiveThresholdManager(),
		ipReputationService: nil, // Will be set by caller
		geoIPService:        nil, // Will be set by caller
	}

	manager.initializeAttackPatterns()
	return manager
}

// SetIPReputationService sets the IP reputation service
func (dm *DDoSProtectionManager) SetIPReputationService(service *IPReputationService) {
	dm.ipReputationService = service
}

// SetGeoIPService sets the GeoIP service
func (dm *DDoSProtectionManager) SetGeoIPService(service *GeoIPService) {
	dm.geoIPService = service
}

// CheckRequest analyzes a request for DDoS patterns and returns protection action
func (dm *DDoSProtectionManager) CheckRequest(ctx context.Context, req *RequestContext) (*ProtectionResult, error) {
	startTime := time.Now()

	result := &ProtectionResult{
		Action:        ActionLog,
		Allowed:       true,
		Reason:        "clean",
		Severity:      SeverityLow,
		RateLimitInfo: make(map[string]interface{}),
		Metadata:      make(map[string]interface{}),
		ThresholdInfo: make(map[string]interface{}),
	}

	ip := dm.extractIPAddress(req)
	if ip == "" {
		result.Action = ActionBlock
		result.Allowed = false
		result.Reason = "invalid_ip"
		result.AnalysisTime = time.Since(startTime)
		return result, nil
	}

	// Check IP whitelist
	if dm.isWhitelisted(ip) {
		result.Reason = "whitelisted"
		result.AnalysisTime = time.Since(startTime)
		return result, nil
	}

	// Check if IP is already blocked
	if blocked, reason := dm.isBlocked(ctx, ip); blocked {
		result.Action = ActionBlock
		result.Allowed = false
		result.Reason = reason
		result.Metadata["blocked_until"] = dm.getBlockExpiry(ctx, ip)
		result.AnalysisTime = time.Since(startTime)
		return result, nil
	}

	// Geographic restrictions check
	if dm.geoIPService != nil {
		if blocked, reason := dm.checkGeographicRestrictions(ip); blocked {
			dm.blockIP(ctx, ip, dm.config.BlockDuration, reason)
			result.Action = ActionBlock
			result.Allowed = false
			result.Reason = reason
			result.AnalysisTime = time.Since(startTime)
			return result, nil
		}
	}

	// IP reputation check
	if dm.ipReputationService != nil {
		reputation := dm.ipReputationService.GetReputationScore(ctx, ip)
		result.Metadata["reputation_score"] = reputation

		// Using a fixed threshold since MinReputationScore is not in config
		if reputation < 0.3 {
			dm.blockIP(ctx, ip, dm.config.BlockDuration, "low_reputation")
			result.Action = ActionBlock
			result.Allowed = false
			result.Reason = "low_reputation"
			result.AnalysisTime = time.Since(startTime)
			return result, nil
		}
	}

	// Rate limiting check
	if exceeded, action := dm.checkRateLimits(ctx, req, ip); exceeded {
		result.Action = action
		result.Allowed = action == ActionLog
		result.Reason = "rate_limit_exceeded"
		dm.recordSuspiciousActivity(ip, req, "rate_limit_exceeded")
	}

	// Attack pattern detection
	if dm.config.EnablePatternDetection {
		if detected, pattern := dm.detectAttackPatterns(ctx, req, ip); detected {
			if pattern.Severity >= SeverityHigh {
				dm.blockIP(ctx, ip, dm.config.BlockDuration, fmt.Sprintf("attack_pattern_%s", pattern.Name))
				result.Action = ActionBlock
				result.Allowed = false
				result.Reason = fmt.Sprintf("attack_pattern_%s", pattern.Name)
			} else {
				result.Action = ActionRateLimit
				result.Allowed = false
				result.Reason = fmt.Sprintf("suspicious_pattern_%s", pattern.Name)
			}
			dm.recordSuspiciousActivity(ip, req, fmt.Sprintf("attack_pattern_%s", pattern.Name))
			result.Severity = pattern.Severity
		}
	}

	// Update IP tracking
	dm.updateIPTracking(ip, req)

	result.AnalysisTime = time.Since(startTime)
	return result, nil
}

// extractIPAddress extracts the real IP address from the request
func (dm *DDoSProtectionManager) extractIPAddress(req *RequestContext) string {
	// Check X-Forwarded-For header first
	if xff, exists := req.Headers["X-Forwarded-For"]; exists {
		// Take the first IP in the chain (the original client IP)
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	// Check X-Real-IP header
	if realIP, exists := req.Headers["X-Real-IP"]; exists {
		return strings.TrimSpace(realIP)
	}

	// Fall back to the direct IP
	return req.IP
}

// checkRateLimits checks various rate limiting rules
func (dm *DDoSProtectionManager) checkRateLimits(ctx context.Context, req *RequestContext, ip string) (bool, ResponseAction) {
	// Global rate limiting
	globalKey := "ddos:global_requests"
	globalCount, err := dm.redis.Incr(ctx, globalKey).Result()
	if err == nil {
		dm.redis.Expire(ctx, globalKey, time.Minute)
		// Using a fixed threshold since GlobalRateLimit is not in config
		if int(globalCount) > 10000 {
			return true, ActionRateLimit
		}
	}

	// Per-IP rate limiting based on user tier
	tier := dm.getUserTier(req)
	ipLimit := dm.getIPRateLimit(tier)
	ipKey := fmt.Sprintf("ddos:ip_requests:%s", ip)
	ipCount, err := dm.redis.Incr(ctx, ipKey).Result()
	if err == nil {
		dm.redis.Expire(ctx, ipKey, time.Minute)
		if int(ipCount) > ipLimit {
			return true, ActionRateLimit
		}
	}

	// Per-endpoint rate limiting
	endpointLimit := dm.getEndpointRateLimit(req.Path, tier)
	endpointKey := fmt.Sprintf("ddos:endpoint_requests:%s:%s", req.Path, ip)
	endpointCount, err := dm.redis.Incr(ctx, endpointKey).Result()
	if err == nil {
		dm.redis.Expire(ctx, endpointKey, time.Minute)
		if int(endpointCount) > endpointLimit {
			return true, ActionRateLimit
		}
	}

	return false, ActionLog
}

// getUserTier gets the user tier from request context
func (dm *DDoSProtectionManager) getUserTier(req *RequestContext) RateLimitTier {
	// Default to public tier
	return RateLimitTierPublic
}

// getIPRateLimit returns rate limit based on user tier
func (dm *DDoSProtectionManager) getIPRateLimit(tier RateLimitTier) int {
	// Using fixed values since IPRateLimit is not in config
	switch tier {
	case RateLimitTierStrict:
		return 25 // config.IPRateLimit / 4
	case RateLimitTierModerate:
		return 50 // config.IPRateLimit / 2
	case RateLimitTierLenient:
		return 100 // config.IPRateLimit
	case RateLimitTierPublic:
		return 200 // config.IPRateLimit * 2
	default:
		return 100 // config.IPRateLimit
	}
}

// getEndpointRateLimit returns rate limit for specific endpoint
func (dm *DDoSProtectionManager) getEndpointRateLimit(path string, tier RateLimitTier) int {
	base := 1000 // Using fixed value since EndpointRateLimit is not in config

	// Adjust based on endpoint sensitivity
	if strings.Contains(path, "/trade/") {
		base = base * 2 // Higher limits for trading endpoints
	} else if strings.Contains(path, "/admin/") {
		base = base / 4 // Lower limits for admin endpoints
	}

	// Adjust based on user tier
	switch tier {
	case RateLimitTierStrict:
		return base / 4
	case RateLimitTierModerate:
		return base / 2
	case RateLimitTierLenient:
		return base
	case RateLimitTierPublic:
		return base / 8
	default:
		return base / 4
	}
}

// initializeAttackPatterns initializes known attack patterns
func (dm *DDoSProtectionManager) initializeAttackPatterns() {
	dm.attackPatterns.mutex.Lock()
	defer dm.attackPatterns.mutex.Unlock()

	dm.attackPatterns.patterns = map[string]*AttackPattern{
		"sql_injection": {
			Name:        "sql_injection",
			Description: "SQL injection attack pattern",
			RequestPatterns: []RequestPattern{{
				PayloadPatterns: []string{"'", "UNION", "SELECT", "DROP", "INSERT", "UPDATE", "DELETE"},
			}},
			Severity:       SeverityHigh,
			ResponseAction: ActionBlock,
		},
		"xss_attack": {
			Name:        "xss_attack",
			Description: "Cross-site scripting attack pattern",
			RequestPatterns: []RequestPattern{{
				PayloadPatterns: []string{"<script>", "javascript:", "onload=", "onerror="},
			}},
			Severity:       SeverityMedium,
			ResponseAction: ActionRateLimit,
		},
		"path_traversal": {
			Name:        "path_traversal",
			Description: "Path traversal attack pattern",
			RequestPatterns: []RequestPattern{{
				URIPatterns: []string{"../", "..\\", "%2e%2e", "%252e%252e"},
			}},
			Severity:       SeverityHigh,
			ResponseAction: ActionBlock,
		},
	}
}

// isWhitelisted checks if an IP is whitelisted
func (dm *DDoSProtectionManager) isWhitelisted(ip string) bool {
	// Check private networks
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}

	// Private IPv4 ranges
	private := []string{
		"127.0.0.0/8",    // localhost
		"10.0.0.0/8",     // private
		"172.16.0.0/12",  // private
		"192.168.0.0/16", // private
	}

	for _, cidr := range private {
		_, network, err := net.ParseCIDR(cidr)
		if err == nil && network.Contains(parsedIP) {
			return true
		}
	}

	return false
}

// isBlocked checks if an IP is currently blocked
func (dm *DDoSProtectionManager) isBlocked(ctx context.Context, ip string) (bool, string) {
	key := fmt.Sprintf("ddos:blocked_ip:%s", ip)
	result, err := dm.redis.Get(ctx, key).Result()
	if err != nil {
		return false, ""
	}

	var blockInfo map[string]interface{}
	if err := json.Unmarshal([]byte(result), &blockInfo); err != nil {
		return false, ""
	}

	if reason, exists := blockInfo["reason"]; exists {
		return true, reason.(string)
	}

	return true, "blocked"
}

// getBlockExpiry returns when an IP block expires
func (dm *DDoSProtectionManager) getBlockExpiry(ctx context.Context, ip string) time.Time {
	key := fmt.Sprintf("ddos:blocked_ip:%s", ip)
	ttl, err := dm.redis.TTL(ctx, key).Result()
	if err != nil {
		return time.Now()
	}

	return time.Now().Add(ttl)
}

// blockIP blocks an IP address for a specified duration
func (dm *DDoSProtectionManager) blockIP(ctx context.Context, ip string, duration time.Duration, reason string) {
	key := fmt.Sprintf("ddos:blocked_ip:%s", ip)
	blockInfo := map[string]interface{}{
		"reason":     reason,
		"blocked_at": time.Now(),
		"expires_at": time.Now().Add(duration),
	}

	data, _ := json.Marshal(blockInfo)
	dm.redis.Set(ctx, key, data, duration)

	dm.logger.Warn("IP blocked",
		zap.String("ip", ip),
		zap.String("reason", reason),
		zap.Duration("duration", duration),
	)
}

// checkGeographicRestrictions checks geographic restrictions
func (dm *DDoSProtectionManager) checkGeographicRestrictions(ip string) (bool, string) {
	if dm.geoIPService == nil {
		return false, ""
	}

	ctx := context.Background()
	return dm.geoIPService.CheckGeographicalRestrictions(ctx, ip)
}

// recordSuspiciousActivity records suspicious activity for an IP
func (dm *DDoSProtectionManager) recordSuspiciousActivity(ip string, req *RequestContext, eventType string) {
	event := SuspiciousEvent{
		EventType:   eventType,
		Timestamp:   time.Now(),
		Severity:    SeverityMedium,
		Description: fmt.Sprintf("Suspicious activity detected from IP %s", ip),
		Metadata: map[string]interface{}{
			"path":       req.Path,
			"method":     req.Method,
			"user_agent": req.UserAgent,
		},
	}

	// Store or update suspicious IP info
	if info, exists := dm.suspiciousIPs.Load(ip); exists {
		suspiciousInfo := info.(*SuspiciousIPInfo)
		suspiciousInfo.SuspiciousEvents = append(suspiciousInfo.SuspiciousEvents, event)
		suspiciousInfo.RequestCount++
		suspiciousInfo.LastSeen = time.Now()
	} else {
		suspiciousInfo := &SuspiciousIPInfo{
			IP:               ip,
			FirstSeen:        time.Now(),
			LastSeen:         time.Now(),
			RequestCount:     1,
			SuspiciousEvents: []SuspiciousEvent{event},
			ReputationScore:  0.5,
			Status:           IPStatusSuspicious,
		}
		dm.suspiciousIPs.Store(ip, suspiciousInfo)
	}
}

// detectAttackPatterns detects known attack patterns
func (dm *DDoSProtectionManager) detectAttackPatterns(ctx context.Context, req *RequestContext, ip string) (bool, *AttackPattern) {
	dm.attackPatterns.mutex.RLock()
	defer dm.attackPatterns.mutex.RUnlock()

	for _, pattern := range dm.attackPatterns.patterns {
		if dm.matchesPattern(req, pattern) {
			pattern.LastDetected = time.Now()
			pattern.DetectionCount++
			return true, pattern
		}
	}

	return false, nil
}

// matchesPattern checks if a request matches an attack pattern
func (dm *DDoSProtectionManager) matchesPattern(req *RequestContext, pattern *AttackPattern) bool {
	for _, requestPattern := range pattern.RequestPatterns {
		// Check payload patterns
		for _, payloadPattern := range requestPattern.PayloadPatterns {
			if strings.Contains(req.Path, payloadPattern) {
				return true
			}
		}

		// Check URI patterns
		for _, uriPattern := range requestPattern.URIPatterns {
			if strings.Contains(req.Path, uriPattern) {
				return true
			}
		}

		// Check header patterns
		for _, headerPattern := range requestPattern.HeaderPatterns {
			for _, headerValue := range req.Headers {
				if strings.Contains(headerValue, headerPattern) {
					return true
				}
			}
		}
	}

	return false
}

// updateIPTracking updates tracking information for an IP
func (dm *DDoSProtectionManager) updateIPTracking(ip string, req *RequestContext) {
	// Update request tracking in Redis
	key := fmt.Sprintf("ddos:ip_tracking:%s", ip)
	trackingData := map[string]interface{}{
		"last_request": time.Now(),
		"path":         req.Path,
		"method":       req.Method,
		"user_agent":   req.UserAgent,
	}

	data, _ := json.Marshal(trackingData)
	dm.redis.Set(context.Background(), key, data, time.Hour*24)
}

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
