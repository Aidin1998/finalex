package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// EnhancedSecurityMiddleware provides comprehensive security protection
type EnhancedSecurityMiddleware struct {
	logger              *zap.Logger
	redis               *redis.Client
	securityManager     *EndpointSecurityManager
	hybridHasher        *HybridHasher
	ddosProtection      *DDoSProtectionManager
	ipReputationService *IPReputationService
	geoIPService        *GeoIPService
	config              *SecurityMiddlewareConfig
	authService         AuthService
}

// SecurityMiddlewareConfig defines security middleware configuration
type SecurityMiddlewareConfig struct {
	Enabled                   bool `json:"enabled"`
	DDoSProtectionEnabled     bool `json:"ddos_protection_enabled"`
	IPReputationEnabled       bool `json:"ip_reputation_enabled"`
	GeoRestrictionEnabled     bool `json:"geo_restriction_enabled"`
	HybridHashingEnabled      bool `json:"hybrid_hashing_enabled"`
	SessionValidationEnabled  bool `json:"session_validation_enabled"`
	APIKeyValidationEnabled   bool `json:"api_key_validation_enabled"`
	JWTValidationEnabled      bool `json:"jwt_validation_enabled"`
	MFAEnforcementEnabled     bool `json:"mfa_enforcement_enabled"`
	TrustedDeviceCheckEnabled bool `json:"trusted_device_check_enabled"`
	SecurityHeadersEnabled    bool `json:"security_headers_enabled"`
	RequestLoggingEnabled     bool `json:"request_logging_enabled"`
	MetricsCollectionEnabled  bool `json:"metrics_collection_enabled"`
	AlertingEnabled           bool `json:"alerting_enabled"`

	// Response configuration
	BlockResponseCode     int           `json:"block_response_code"`
	ChallengeResponseCode int           `json:"challenge_response_code"`
	TarPitDelay           time.Duration `json:"tar_pit_delay"`
	CustomErrorPages      bool          `json:"custom_error_pages"`
	DetailedErrorMessages bool          `json:"detailed_error_messages"`

	// Performance configuration
	MaxConcurrentRequests  int           `json:"max_concurrent_requests"`
	RequestTimeoutDuration time.Duration `json:"request_timeout_duration"`
	HealthCheckBypass      bool          `json:"health_check_bypass"`
	MonitoringBypass       bool          `json:"monitoring_bypass"`
}

// SecurityContext contains security-related request information
type SecurityContext struct {
	RequestID        string                  `json:"request_id"`
	IP               string                  `json:"ip"`
	UserAgent        string                  `json:"user_agent"`
	Endpoint         string                  `json:"endpoint"`
	Method           string                  `json:"method"`
	UserID           string                  `json:"user_id,omitempty"`
	SessionID        string                  `json:"session_id,omitempty"`
	APIKeyID         string                  `json:"api_key_id,omitempty"`
	SecurityLevel    SecurityLevel           `json:"security_level"`
	Classification   *EndpointClassification `json:"classification"`
	ThreatLevel      string                  `json:"threat_level"`
	ReputationScore  float64                 `json:"reputation_score"`
	GeoInfo          *GeoInfo                `json:"geo_info,omitempty"`
	ProtectionResult *ProtectionResult       `json:"protection_result,omitempty"`
	ProcessingTime   time.Duration           `json:"processing_time"`
	Timestamp        time.Time               `json:"timestamp"`
	Metadata         map[string]interface{}  `json:"metadata"`
}

// EnhancedSecurityMetrics contains enhanced security-related metrics
type EnhancedSecurityMetrics struct {
	TotalRequests          int64   `json:"total_requests"`
	BlockedRequests        int64   `json:"blocked_requests"`
	ChallengedRequests     int64   `json:"challenged_requests"`
	TarPittedRequests      int64   `json:"tar_pitted_requests"`
	AuthenticationFailures int64   `json:"authentication_failures"`
	MFAFailures            int64   `json:"mfa_failures"`
	SuspiciousActivities   int64   `json:"suspicious_activities"`
	AverageProcessingTime  float64 `json:"average_processing_time"`
	PeakProcessingTime     float64 `json:"peak_processing_time"`
	ThreatDetections       int64   `json:"threat_detections"`
	GeoBlocks              int64   `json:"geo_blocks"`
	ReputationBlocks       int64   `json:"reputation_blocks"`
}

// NewEnhancedSecurityMiddleware creates a new enhanced security middleware
func NewEnhancedSecurityMiddleware(
	logger *zap.Logger,
	redis *redis.Client,
	authService AuthService,
) *EnhancedSecurityMiddleware {

	securityManager := NewEndpointSecurityManager()
	hybridHasher := NewHybridHasher(securityManager)
	ddosProtection := NewDDoSProtectionManager(redis, logger, securityManager)
	ipReputationService := NewIPReputationService(redis, logger)
	geoIPService := NewGeoIPService()
	geoIPService.SetLogger(logger)

	config := &SecurityMiddlewareConfig{
		Enabled:                   true,
		DDoSProtectionEnabled:     true,
		IPReputationEnabled:       true,
		GeoRestrictionEnabled:     false,
		HybridHashingEnabled:      true,
		SessionValidationEnabled:  true,
		APIKeyValidationEnabled:   true,
		JWTValidationEnabled:      true,
		MFAEnforcementEnabled:     false, // Disabled by default for critical trading paths
		TrustedDeviceCheckEnabled: false,
		SecurityHeadersEnabled:    true,
		RequestLoggingEnabled:     true,
		MetricsCollectionEnabled:  true,
		AlertingEnabled:           true,
		BlockResponseCode:         403,
		ChallengeResponseCode:     429,
		TarPitDelay:               time.Second * 5,
		CustomErrorPages:          false,
		DetailedErrorMessages:     false,
		MaxConcurrentRequests:     10000,
		RequestTimeoutDuration:    time.Second * 30,
		HealthCheckBypass:         true,
		MonitoringBypass:          true,
	}

	return &EnhancedSecurityMiddleware{
		logger:              logger,
		redis:               redis,
		securityManager:     securityManager,
		hybridHasher:        hybridHasher,
		ddosProtection:      ddosProtection,
		ipReputationService: ipReputationService,
		geoIPService:        geoIPService,
		config:              config,
		authService:         authService,
	}
}

// Handler returns the HTTP middleware handler
func (esm *EnhancedSecurityMiddleware) Handler() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !esm.config.Enabled {
				next.ServeHTTP(w, r)
				return
			}

			startTime := time.Now()

			// Create security context
			secCtx := esm.createSecurityContext(r)

			// Check for bypass conditions
			if esm.shouldBypass(secCtx) {
				next.ServeHTTP(w, r)
				return
			}

			// Apply security headers
			if esm.config.SecurityHeadersEnabled {
				esm.applySecurityHeaders(w, secCtx)
			}

			// Perform security checks
			securityResult := esm.performSecurityChecks(r.Context(), secCtx)

			// Handle security result
			handled := esm.handleSecurityResult(w, r, secCtx, securityResult)
			if handled {
				return
			}

			// Add security context to request
			ctx := context.WithValue(r.Context(), "security_context", secCtx)
			r = r.WithContext(ctx)

			// Record metrics
			if esm.config.MetricsCollectionEnabled {
				defer func() {
					secCtx.ProcessingTime = time.Since(startTime)
					esm.recordMetrics(secCtx, securityResult)
				}()
			}

			// Continue to next handler
			next.ServeHTTP(w, r)
		})
	}
}

// createSecurityContext creates a security context from the HTTP request
func (esm *EnhancedSecurityMiddleware) createSecurityContext(r *http.Request) *SecurityContext {
	ip := esm.extractClientIP(r)
	endpoint := r.URL.Path
	classification := esm.securityManager.GetEndpointClassification(endpoint)

	secCtx := &SecurityContext{
		RequestID:      esm.generateRequestID(),
		IP:             ip,
		UserAgent:      r.UserAgent(),
		Endpoint:       endpoint,
		Method:         r.Method,
		SecurityLevel:  classification.SecurityLevel,
		Classification: classification,
		Timestamp:      time.Now(),
		Metadata:       make(map[string]interface{}),
	}

	// Extract authentication information if present
	if authHeader := r.Header.Get("Authorization"); authHeader != "" {
		secCtx.Metadata["auth_header_present"] = true
		if strings.HasPrefix(authHeader, "Bearer ") {
			secCtx.Metadata["auth_type"] = "jwt"
		} else if strings.HasPrefix(authHeader, "ApiKey ") {
			secCtx.Metadata["auth_type"] = "api_key"
		}
	}

	// Extract session information if present
	if sessionCookie, err := r.Cookie("session_id"); err == nil {
		secCtx.SessionID = sessionCookie.Value
		secCtx.Metadata["session_present"] = true
	}

	return secCtx
}

// performSecurityChecks executes all enabled security checks
func (esm *EnhancedSecurityMiddleware) performSecurityChecks(ctx context.Context, secCtx *SecurityContext) *SecurityCheckResult {
	result := &SecurityCheckResult{
		Allowed:      true,
		Action:       ActionLog,
		Reason:       "passed_security_checks",
		CheckResults: make(map[string]*CheckResult),
		Metadata:     make(map[string]interface{}),
	}

	// DDoS Protection Check
	if esm.config.DDoSProtectionEnabled && secCtx.Classification.DDoSProtection {
		ddosResult := esm.performDDoSCheck(ctx, secCtx)
		result.CheckResults["ddos"] = ddosResult
		if !ddosResult.Passed {
			result.Allowed = false
			result.Action = ActionBlock
			result.Reason = "ddos_protection_triggered"
			return result
		}
	}

	// IP Reputation Check
	if esm.config.IPReputationEnabled {
		reputationResult := esm.performReputationCheck(ctx, secCtx)
		result.CheckResults["reputation"] = reputationResult
		secCtx.ReputationScore = reputationResult.Score
		if !reputationResult.Passed {
			result.Allowed = false
			result.Action = ActionBlock
			result.Reason = "low_ip_reputation"
			return result
		}
	}

	// Geographical Restriction Check
	if esm.config.GeoRestrictionEnabled {
		geoResult := esm.performGeoCheck(ctx, secCtx)
		result.CheckResults["geo"] = geoResult
		if !geoResult.Passed {
			result.Allowed = false
			result.Action = ActionBlock
			result.Reason = "geographical_restriction"
			return result
		}
	}

	// Authentication Check
	authResult := esm.performAuthenticationCheck(ctx, secCtx)
	result.CheckResults["authentication"] = authResult
	if !authResult.Passed && secCtx.Classification.SessionRequired {
		result.Allowed = false
		result.Action = ActionBlock
		result.Reason = "authentication_required"
		return result
	}

	// MFA Check (if required by endpoint)
	if esm.config.MFAEnforcementEnabled && secCtx.Classification.RequiresMFA {
		mfaResult := esm.performMFACheck(ctx, secCtx)
		result.CheckResults["mfa"] = mfaResult
		if !mfaResult.Passed {
			result.Allowed = false
			result.Action = ActionChallenge
			result.Reason = "mfa_required"
			return result
		}
	}

	// Record successful security check
	esm.recordSecurityEvent(secCtx, "security_check_passed", nil)

	return result
}

// performDDoSCheck executes DDoS protection checks
func (esm *EnhancedSecurityMiddleware) performDDoSCheck(ctx context.Context, secCtx *SecurityContext) *CheckResult {
	reqCtx := &RequestContext{
		Method:    secCtx.Method,
		Path:      secCtx.Endpoint,
		Headers:   make(map[string]string),
		UserAgent: secCtx.UserAgent,
		IP:        secCtx.IP,
		Timestamp: secCtx.Timestamp,
		UserID:    secCtx.UserID,
		SessionID: secCtx.SessionID,
	}

	protectionResult, err := esm.ddosProtection.CheckRequest(ctx, reqCtx)
	if err != nil {
		esm.logger.Error("DDoS protection check failed", zap.Error(err))
		return &CheckResult{
			Passed:   true, // Fail open for availability
			Score:    0.5,
			Reason:   "ddos_check_error",
			Metadata: map[string]interface{}{"error": err.Error()},
		}
	}

	secCtx.ProtectionResult = protectionResult

	return &CheckResult{
		Passed:   protectionResult.Allowed,
		Score:    1.0, // Binary result
		Reason:   protectionResult.Reason,
		Metadata: protectionResult.Metadata,
	}
}

// performReputationCheck executes IP reputation checks
func (esm *EnhancedSecurityMiddleware) performReputationCheck(ctx context.Context, secCtx *SecurityContext) *CheckResult {
	reputation := esm.ipReputationService.GetIPReputation(ctx, secCtx.IP)

	// Record IP behavior
	_ = esm.ipReputationService.RecordBehavior(ctx, secCtx.IP, "request", map[string]interface{}{
		"endpoint":   secCtx.Endpoint,
		"user_agent": secCtx.UserAgent,
		"timestamp":  secCtx.Timestamp,
	})

	// Minimum reputation threshold varies by endpoint
	minScore := 0.3 // Default
	if secCtx.Classification.SecurityLevel == SecurityLevelCritical {
		minScore = 0.2 // More lenient for trading
	} else if secCtx.Classification.SecurityLevel == SecurityLevelHigh {
		minScore = 0.5 // More strict for admin
	}

	passed := reputation.OverallScore >= minScore

	return &CheckResult{
		Passed: passed,
		Score:  reputation.OverallScore,
		Reason: fmt.Sprintf("reputation_score_%.2f", reputation.OverallScore),
		Metadata: map[string]interface{}{
			"behavior_score": reputation.BehaviorScore,
			"threat_score":   reputation.ThreatScore,
			"asn_score":      reputation.ASNScore,
			"geo_score":      reputation.GeoScore,
		},
	}
}

// performGeoCheck executes geographical restriction checks
func (esm *EnhancedSecurityMiddleware) performGeoCheck(ctx context.Context, secCtx *SecurityContext) *CheckResult {
	blocked, reason := esm.geoIPService.CheckGeographicalRestrictions(ctx, secCtx.IP)

	geoInfo, err := esm.geoIPService.GetGeoInfo(ctx, secCtx.IP)
	if err != nil {
		esm.logger.Error("Failed to get geo info", zap.Error(err))
		return &CheckResult{
			Passed:   true, // Fail open
			Score:    0.5,
			Reason:   "geo_lookup_failed",
			Metadata: map[string]interface{}{"error": err.Error()},
		}
	}

	secCtx.GeoInfo = geoInfo
	secCtx.ThreatLevel = geoInfo.ThreatLevel

	// Check for high-risk locations
	isHighRisk := esm.geoIPService.IsHighRiskLocation(ctx, secCtx.IP)

	score := 1.0
	if isHighRisk {
		score = 0.5
	}
	if blocked {
		score = 0.0
	}

	return &CheckResult{
		Passed: !blocked,
		Score:  score,
		Reason: reason,
		Metadata: map[string]interface{}{
			"country":      geoInfo.Country,
			"country_code": geoInfo.CountryCode,
			"is_high_risk": isHighRisk,
			"is_vpn":       geoInfo.IsVPN,
			"is_proxy":     geoInfo.IsProxy,
			"is_tor":       geoInfo.IsTor,
		},
	}
}

// performAuthenticationCheck executes authentication validation
func (esm *EnhancedSecurityMiddleware) performAuthenticationCheck(ctx context.Context, secCtx *SecurityContext) *CheckResult {
	// For now, we'll do basic validation
	// In a full implementation, this would integrate with the auth service

	authPresent := secCtx.Metadata["auth_header_present"] == true || secCtx.SessionID != ""

	return &CheckResult{
		Passed: authPresent || !secCtx.Classification.SessionRequired,
		Score:  1.0,
		Reason: "authentication_check",
		Metadata: map[string]interface{}{
			"auth_present":       authPresent,
			"session_required":   secCtx.Classification.SessionRequired,
			"session_id_present": secCtx.SessionID != "",
		},
	}
}

// performMFACheck executes multi-factor authentication checks
func (esm *EnhancedSecurityMiddleware) performMFACheck(ctx context.Context, secCtx *SecurityContext) *CheckResult {
	// Placeholder for MFA check
	// In a full implementation, this would check MFA status

	return &CheckResult{
		Passed: true, // For now, always pass
		Score:  1.0,
		Reason: "mfa_not_implemented",
		Metadata: map[string]interface{}{
			"mfa_required": secCtx.Classification.RequiresMFA,
		},
	}
}

// SecurityCheckResult represents the result of all security checks
type SecurityCheckResult struct {
	Allowed      bool                    `json:"allowed"`
	Action       ResponseAction          `json:"action"`
	Reason       string                  `json:"reason"`
	CheckResults map[string]*CheckResult `json:"check_results"`
	Metadata     map[string]interface{}  `json:"metadata"`
}

// CheckResult represents the result of a single security check
type CheckResult struct {
	Passed   bool                   `json:"passed"`
	Score    float64                `json:"score"`
	Reason   string                 `json:"reason"`
	Metadata map[string]interface{} `json:"metadata"`
}

// Additional helper methods would be implemented here...
// For brevity, I'm including the key structure and main methods

// extractClientIP extracts the real client IP address
func (esm *EnhancedSecurityMiddleware) extractClientIP(r *http.Request) string {
	// Check X-Real-IP header
	if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
		return realIP
	}

	// Check X-Forwarded-For header
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		ips := strings.Split(forwarded, ",")
		return strings.TrimSpace(ips[0])
	}

	// Fall back to remote address
	return r.RemoteAddr
}

// generateRequestID generates a unique request ID
func (esm *EnhancedSecurityMiddleware) generateRequestID() string {
	return fmt.Sprintf("req_%d", time.Now().UnixNano())
}

// shouldBypass checks if the request should bypass security checks
func (esm *EnhancedSecurityMiddleware) shouldBypass(secCtx *SecurityContext) bool {
	// Health check bypass
	if esm.config.HealthCheckBypass && strings.HasPrefix(secCtx.Endpoint, "/health") {
		return true
	}

	// Monitoring bypass
	if esm.config.MonitoringBypass && strings.HasPrefix(secCtx.Endpoint, "/metrics") {
		return true
	}

	return false
}

// applySecurityHeaders applies security headers to the response
func (esm *EnhancedSecurityMiddleware) applySecurityHeaders(w http.ResponseWriter, secCtx *SecurityContext) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("X-XSS-Protection", "1; mode=block")
	w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
	w.Header().Set("Content-Security-Policy", "default-src 'self'")
	w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
	w.Header().Set("X-Request-ID", secCtx.RequestID)
}

// handleSecurityResult handles the result of security checks
func (esm *EnhancedSecurityMiddleware) handleSecurityResult(w http.ResponseWriter, r *http.Request, secCtx *SecurityContext, result *SecurityCheckResult) bool {
	if result.Allowed {
		return false // Continue processing
	}

	// Log security event
	esm.recordSecurityEvent(secCtx, "security_block", result)

	switch result.Action {
	case ActionBlock:
		esm.handleBlock(w, secCtx, result)
		return true

	case ActionChallenge:
		esm.handleChallenge(w, secCtx, result)
		return true

	case ActionTarPit:
		esm.handleTarPit(w, secCtx, result)
		return true

	case ActionRateLimit:
		esm.handleRateLimit(w, secCtx, result)
		return true

	default:
		return false // Continue processing
	}
}

// handleBlock handles blocked requests
func (esm *EnhancedSecurityMiddleware) handleBlock(w http.ResponseWriter, secCtx *SecurityContext, result *SecurityCheckResult) {
	w.WriteHeader(esm.config.BlockResponseCode)

	response := map[string]interface{}{
		"error":      "access_denied",
		"reason":     result.Reason,
		"request_id": secCtx.RequestID,
	}

	if esm.config.DetailedErrorMessages {
		response["details"] = result.CheckResults
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleChallenge handles challenge requests
func (esm *EnhancedSecurityMiddleware) handleChallenge(w http.ResponseWriter, secCtx *SecurityContext, result *SecurityCheckResult) {
	w.WriteHeader(esm.config.ChallengeResponseCode)

	response := map[string]interface{}{
		"error":      "challenge_required",
		"reason":     result.Reason,
		"request_id": secCtx.RequestID,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleTarPit handles tar pit requests
func (esm *EnhancedSecurityMiddleware) handleTarPit(w http.ResponseWriter, secCtx *SecurityContext, result *SecurityCheckResult) {
	// Delay response
	time.Sleep(esm.config.TarPitDelay)

	w.WriteHeader(http.StatusTooManyRequests)

	response := map[string]interface{}{
		"error":       "rate_limited",
		"reason":      "too_many_requests",
		"request_id":  secCtx.RequestID,
		"retry_after": int(esm.config.TarPitDelay.Seconds()),
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Retry-After", fmt.Sprintf("%d", int(esm.config.TarPitDelay.Seconds())))
	json.NewEncoder(w).Encode(response)
}

// handleRateLimit handles rate limited requests
func (esm *EnhancedSecurityMiddleware) handleRateLimit(w http.ResponseWriter, secCtx *SecurityContext, result *SecurityCheckResult) {
	w.WriteHeader(http.StatusTooManyRequests)

	response := map[string]interface{}{
		"error":      "rate_limited",
		"reason":     result.Reason,
		"request_id": secCtx.RequestID,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// recordSecurityEvent records security events for monitoring and alerting
func (esm *EnhancedSecurityMiddleware) recordSecurityEvent(secCtx *SecurityContext, eventType string, data interface{}) {
	if !esm.config.RequestLoggingEnabled {
		return
	}

	event := map[string]interface{}{
		"timestamp":        secCtx.Timestamp,
		"request_id":       secCtx.RequestID,
		"event_type":       eventType,
		"ip":               secCtx.IP,
		"endpoint":         secCtx.Endpoint,
		"method":           secCtx.Method,
		"user_agent":       secCtx.UserAgent,
		"security_level":   secCtx.SecurityLevel.String(),
		"threat_level":     secCtx.ThreatLevel,
		"reputation_score": secCtx.ReputationScore,
		"data":             data,
	}

	// In a real implementation, this would send to a logging/monitoring system
	esm.logger.Info("Security event", zap.Any("event", event))
}

// recordMetrics records security metrics
func (esm *EnhancedSecurityMiddleware) recordMetrics(secCtx *SecurityContext, result *SecurityCheckResult) {
	// In a real implementation, this would send metrics to a monitoring system
	// For now, just log the metrics

	metrics := map[string]interface{}{
		"processing_time":  secCtx.ProcessingTime.Milliseconds(),
		"security_level":   secCtx.SecurityLevel.String(),
		"allowed":          result.Allowed,
		"action":           result.Action.String(),
		"reputation_score": secCtx.ReputationScore,
		"threat_level":     secCtx.ThreatLevel,
	}
	esm.logger.Debug("Security metrics", zap.Any("metrics", metrics))
}

// String returns string representation of response action
func (ra ResponseAction) String() string {
	switch ra {
	case ActionLog:
		return "log"
	case ActionRateLimit:
		return "rate_limit"
	case ActionChallenge:
		return "challenge"
	case ActionTarPit:
		return "tar_pit"
	case ActionBlock:
		return "block"
	case ActionNull:
		return "null"
	default:
		return "unknown"
	}
}
