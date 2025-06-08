// Package middleware provides enhanced unified middleware infrastructure for Orbit CEX
// This extends the base UnifiedMiddleware with additional advanced security features
package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Aidin1998/finalex/internal/infrastructure/ratelimit"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// EnhancedSecurityConfig contains advanced security configuration
type EnhancedSecurityConfig struct {
	ThreatDetectionEnabled  bool          `json:"threat_detection_enabled"`
	AnomalyDetectionEnabled bool          `json:"anomaly_detection_enabled"`
	BehaviorAnalysisEnabled bool          `json:"behavior_analysis_enabled"`
	AdaptiveRateLimiting    bool          `json:"adaptive_rate_limiting"`
	BurstProtection         bool          `json:"burst_protection"`
	DetailedAuditLogging    bool          `json:"detailed_audit_logging"`
	SecurityEventAlerts     bool          `json:"security_event_alerts"`
	MaxConcurrentRequests   int           `json:"max_concurrent_requests"`
	RequestTimeoutDuration  time.Duration `json:"request_timeout_duration"`
}

// AdvancedMetricsCollector handles enhanced metrics collection
type AdvancedMetricsCollector struct {
	mu                  sync.RWMutex
	requestPatterns     map[string]int64
	userBehaviorMetrics map[string]*UserBehaviorMetrics
	threatMetrics       *ThreatMetrics
}

// UserBehaviorMetrics tracks user behavior patterns
type UserBehaviorMetrics struct {
	RequestCount    int64     `json:"request_count"`
	LastActivity    time.Time `json:"last_activity"`
	AverageInterval float64   `json:"average_interval"`
	SuspiciousFlags int       `json:"suspicious_flags"`
}

// ThreatMetrics tracks security threats
type ThreatMetrics struct {
	TotalThreats      int64 `json:"total_threats"`
	BlockedRequests   int64 `json:"blocked_requests"`
	SuspiciousIPs     int64 `json:"suspicious_ips"`
	AnomalousPatterns int64 `json:"anomalous_patterns"`
}

// AuditLogger handles enhanced audit logging
type AuditLogger struct {
	logger  *zap.Logger
	redis   *redis.Client
	enabled bool
}

// EnhancedUnifiedMiddleware extends the base UnifiedMiddleware with advanced security features
type EnhancedUnifiedMiddleware struct {
	*UnifiedMiddleware // Embed the base middleware

	securityConfig    *EnhancedSecurityConfig
	advancedMetrics   *AdvancedMetricsCollector
	auditLogger       *AuditLogger
	activeConnections sync.Map
	requestCounter    int64
}

// NewEnhancedUnifiedMiddleware creates a new enhanced unified middleware instance
func NewEnhancedUnifiedMiddleware(
	config *UnifiedMiddlewareConfig,
	securityConfig *EnhancedSecurityConfig,
	rateLimitManager *ratelimit.RateLimitManager,
	logger *zap.Logger,
	redis *redis.Client,
) *EnhancedUnifiedMiddleware {
	// Create base middleware
	baseMiddleware := NewUnifiedMiddleware(config, rateLimitManager, logger, redis)

	// Create enhanced components
	advancedMetrics := &AdvancedMetricsCollector{
		requestPatterns:     make(map[string]int64),
		userBehaviorMetrics: make(map[string]*UserBehaviorMetrics),
		threatMetrics:       &ThreatMetrics{},
	}

	auditLogger := &AuditLogger{
		logger:  logger,
		redis:   redis,
		enabled: securityConfig.DetailedAuditLogging,
	}

	return &EnhancedUnifiedMiddleware{
		UnifiedMiddleware: baseMiddleware,
		securityConfig:    securityConfig,
		advancedMetrics:   advancedMetrics,
		auditLogger:       auditLogger,
	}
}

// DefaultEnhancedSecurityConfig returns default enhanced security configuration
func DefaultEnhancedSecurityConfig() *EnhancedSecurityConfig {
	return &EnhancedSecurityConfig{
		ThreatDetectionEnabled:  true,
		AnomalyDetectionEnabled: true,
		BehaviorAnalysisEnabled: true,
		AdaptiveRateLimiting:    false,
		BurstProtection:         true,
		DetailedAuditLogging:    true,
		SecurityEventAlerts:     false,
		MaxConcurrentRequests:   10000,
		RequestTimeoutDuration:  30 * time.Second,
	}
}

// Handler returns the enhanced middleware chain
func (eum *EnhancedUnifiedMiddleware) Handler() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Add enhanced processing before base middleware
			if eum.securityConfig.ThreatDetectionEnabled {
				if eum.detectThreat(r) {
					eum.handleThreat(w, r)
					return
				}
			}

			// Apply base middleware chain first
			baseHandler := eum.UnifiedMiddleware.Handler()(next)

			// Wrap with enhanced features
			enhancedHandler := eum.enhancedSecurityMiddleware(baseHandler)
			enhancedHandler = eum.advancedMetricsMiddleware(enhancedHandler)
			enhancedHandler = eum.auditLoggingMiddleware(enhancedHandler)

			enhancedHandler.ServeHTTP(w, r)
		})
	}
}

// enhancedSecurityMiddleware adds advanced security features
func (eum *EnhancedUnifiedMiddleware) enhancedSecurityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Check concurrent requests limit
		if eum.securityConfig.MaxConcurrentRequests > 0 {
			if eum.requestCounter >= int64(eum.securityConfig.MaxConcurrentRequests) {
				http.Error(w, "Server overloaded", http.StatusServiceUnavailable)
				return
			}
		}

		// Increment active request counter
		eum.requestCounter++
		defer func() { eum.requestCounter-- }()

		// Set request timeout if configured
		if eum.securityConfig.RequestTimeoutDuration > 0 {
			ctx, cancel := context.WithTimeout(r.Context(), eum.securityConfig.RequestTimeoutDuration)
			defer cancel()
			r = r.WithContext(ctx)
		}

		// Enhanced behavior analysis
		if eum.securityConfig.BehaviorAnalysisEnabled {
			eum.analyzeBehavior(r)
		}

		// Process request
		next.ServeHTTP(w, r)

		// Record processing time for this enhanced layer
		processingTime := time.Since(start)
		// Use basic metrics from metrics.go - middlewareProcessingTime is defined there
		httpRequestDuration.WithLabelValues(r.Method, r.URL.Path, "200").Observe(processingTime.Seconds())
	})
}

// advancedMetricsMiddleware handles enhanced metrics collection
func (eum *EnhancedUnifiedMiddleware) advancedMetricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Update request patterns
		eum.updateRequestPatterns(r)

		// Process request
		next.ServeHTTP(w, r)

		// Record metrics
		duration := time.Since(start)
		eum.recordAdvancedMetrics(r, duration)
	})
}

// auditLoggingMiddleware handles enhanced audit logging
func (eum *EnhancedUnifiedMiddleware) auditLoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !eum.auditLogger.enabled {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()

		// Log request details
		eum.auditLogger.logRequestStart(r)

		// Process request
		next.ServeHTTP(w, r)

		// Log completion
		duration := time.Since(start)
		eum.auditLogger.logRequestCompletion(r, duration)
	})
}

// detectThreat performs threat detection analysis
func (eum *EnhancedUnifiedMiddleware) detectThreat(r *http.Request) bool {
	if eum.securityConfig.AnomalyDetectionEnabled {
		return eum.detectAnomalousPatterns(r)
	}
	return false
}

// handleThreat handles detected threats
func (eum *EnhancedUnifiedMiddleware) handleThreat(w http.ResponseWriter, r *http.Request) {
	// Update threat metrics
	eum.advancedMetrics.mu.Lock()
	eum.advancedMetrics.threatMetrics.TotalThreats++
	eum.advancedMetrics.threatMetrics.BlockedRequests++
	eum.advancedMetrics.mu.Unlock()

	// Log security event
	if eum.auditLogger.enabled {
		eum.auditLogger.logger.Warn("Threat detected and blocked",
			zap.String("client_ip", eum.extractClientIP(r)),
			zap.String("user_agent", r.UserAgent()),
			zap.String("path", r.URL.Path),
			zap.String("method", r.Method),
		)
	}

	// Increment security violations metric (from metrics.go)
	securityViolations.WithLabelValues("threat_detection", "high").Inc()

	http.Error(w, "Request blocked", http.StatusForbidden)
}

// analyzeBehavior performs user behavior analysis
func (eum *EnhancedUnifiedMiddleware) analyzeBehavior(r *http.Request) {
	clientIP := eum.extractClientIP(r)

	eum.advancedMetrics.mu.Lock()
	defer eum.advancedMetrics.mu.Unlock()

	userMetrics, exists := eum.advancedMetrics.userBehaviorMetrics[clientIP]
	if !exists {
		userMetrics = &UserBehaviorMetrics{
			LastActivity: time.Now(),
		}
		eum.advancedMetrics.userBehaviorMetrics[clientIP] = userMetrics
	}

	// Update user behavior metrics
	userMetrics.RequestCount++
	now := time.Now()
	if !userMetrics.LastActivity.IsZero() {
		interval := now.Sub(userMetrics.LastActivity).Seconds()
		userMetrics.AverageInterval = (userMetrics.AverageInterval + interval) / 2
	}
	userMetrics.LastActivity = now
}

// detectAnomalousPatterns detects anomalous request patterns
func (eum *EnhancedUnifiedMiddleware) detectAnomalousPatterns(r *http.Request) bool {
	clientIP := eum.extractClientIP(r)

	eum.advancedMetrics.mu.RLock()
	userMetrics, exists := eum.advancedMetrics.userBehaviorMetrics[clientIP]
	eum.advancedMetrics.mu.RUnlock()

	if !exists {
		return false
	}

	// Check for rapid fire requests (simple anomaly detection)
	if userMetrics.AverageInterval > 0 && userMetrics.AverageInterval < 0.1 {
		return true
	}

	return false
}

// updateRequestPatterns updates request pattern tracking
func (eum *EnhancedUnifiedMiddleware) updateRequestPatterns(r *http.Request) {
	pattern := r.Method + ":" + r.URL.Path

	eum.advancedMetrics.mu.Lock()
	eum.advancedMetrics.requestPatterns[pattern]++
	eum.advancedMetrics.mu.Unlock()
}

// recordAdvancedMetrics records advanced metrics
func (eum *EnhancedUnifiedMiddleware) recordAdvancedMetrics(r *http.Request, duration time.Duration) {
	// Record advanced metrics using existing metrics from metrics.go
	httpRequestsTotal.WithLabelValues(r.Method, r.URL.Path, "200").Inc()
	httpRequestDuration.WithLabelValues(r.Method, r.URL.Path, "200").Observe(duration.Seconds())
}

// extractClientIP extracts client IP
func (eum *EnhancedUnifiedMiddleware) extractClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return xff
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	return r.RemoteAddr
}

// logRequestStart logs the start of a request
func (al *AuditLogger) logRequestStart(r *http.Request) {
	if !al.enabled {
		return
	}

	al.logger.Info("Enhanced middleware: Request started",
		zap.String("method", r.Method),
		zap.String("path", r.URL.Path),
		zap.String("client_ip", r.RemoteAddr),
		zap.String("user_agent", r.UserAgent()),
		zap.Time("timestamp", time.Now()),
	)
}

// logRequestCompletion logs the completion of a request
func (al *AuditLogger) logRequestCompletion(r *http.Request, duration time.Duration) {
	if !al.enabled {
		return
	}

	al.logger.Info("Enhanced middleware: Request completed",
		zap.String("method", r.Method),
		zap.String("path", r.URL.Path),
		zap.Duration("duration", duration),
		zap.Time("timestamp", time.Now()),
	)
}

// Advanced features for enhanced middleware

// RequestFingerprint represents a unique request fingerprint
type RequestFingerprint struct {
	Hash      string            `json:"hash"`
	Method    string            `json:"method"`
	Path      string            `json:"path"`
	Headers   map[string]string `json:"headers"`
	QueryHash string            `json:"query_hash"`
	BodyHash  string            `json:"body_hash"`
	UserAgent string            `json:"user_agent"`
	ClientIP  string            `json:"client_ip"`
	Timestamp time.Time         `json:"timestamp"`
}

// SmartRateLimiter provides adaptive rate limiting
type SmartRateLimiter struct {
	mu              sync.RWMutex
	userLimits      map[string]*UserRateLimit
	adaptiveEnabled bool
	learningWindow  time.Duration
	defaultLimit    int64
	burstMultiplier float64
}

// UserRateLimit tracks per-user rate limiting with adaptive behavior
type UserRateLimit struct {
	CurrentLimit   int64     `json:"current_limit"`
	RequestCount   int64     `json:"request_count"`
	WindowStart    time.Time `json:"window_start"`
	LastActivity   time.Time `json:"last_activity"`
	BehaviorScore  float64   `json:"behavior_score"`
	ViolationCount int       `json:"violation_count"`
	TrustedScore   float64   `json:"trusted_score"`
}

// PerformanceOptimizer handles request optimization
type PerformanceOptimizer struct {
	mu                 sync.RWMutex
	responseCache      map[string]*CachedResponse
	compressionCache   map[string][]byte
	optimizationStats  *OptimizationStats
	cacheEnabled       bool
	compressionEnabled bool
	maxCacheSize       int
	cacheTTL           time.Duration
}

// CachedResponse represents a cached HTTP response
type CachedResponse struct {
	StatusCode int               `json:"status_code"`
	Headers    map[string]string `json:"headers"`
	Body       []byte            `json:"body"`
	Timestamp  time.Time         `json:"timestamp"`
	TTL        time.Duration     `json:"ttl"`
	Compressed bool              `json:"compressed"`
	ETag       string            `json:"etag"`
}

// OptimizationStats tracks performance optimization metrics
type OptimizationStats struct {
	CacheHits           int64   `json:"cache_hits"`
	CacheMisses         int64   `json:"cache_misses"`
	CompressionSavings  int64   `json:"compression_savings"`
	AverageResponseTime float64 `json:"average_response_time"`
}

// SessionTracker provides enhanced session management
type SessionTracker struct {
	mu              sync.RWMutex
	activeSessions  map[string]*EnhancedSession
	sessionTimeout  time.Duration
	maxSessions     int
	securityEnabled bool
}

// EnhancedSession represents an enhanced user session
type EnhancedSession struct {
	ID                string            `json:"id"`
	UserID            string            `json:"user_id"`
	CreatedAt         time.Time         `json:"created_at"`
	LastActivity      time.Time         `json:"last_activity"`
	IPAddress         string            `json:"ip_address"`
	UserAgent         string            `json:"user_agent"`
	DeviceFingerprint string            `json:"device_fingerprint"`
	SecurityFlags     []string          `json:"security_flags"`
	Metadata          map[string]string `json:"metadata"`
	ExpiresAt         time.Time         `json:"expires_at"`
	IsValid           bool              `json:"is_valid"`
}

// ThreatIntelligence provides real-time threat detection
type ThreatIntelligence struct {
	mu                 sync.RWMutex
	maliciousIPs       map[string]*ThreatInfo
	suspiciousPatterns []*ThreatPattern
	lastUpdate         time.Time
	updateInterval     time.Duration
	enabled            bool
}

// ThreatInfo represents threat intelligence data
type ThreatInfo struct {
	IP          string    `json:"ip"`
	ThreatLevel string    `json:"threat_level"`
	Categories  []string  `json:"categories"`
	LastSeen    time.Time `json:"last_seen"`
	Source      string    `json:"source"`
	Confidence  float64   `json:"confidence"`
	Description string    `json:"description"`
}

// ThreatPattern represents a threat detection pattern
type ThreatPattern struct {
	Name        string         `json:"name"`
	Pattern     string         `json:"pattern"`
	ThreatLevel string         `json:"threat_level"`
	Action      string         `json:"action"`
	Regex       *regexp.Regexp `json:"-"`
}

// NewSmartRateLimiter creates a new smart rate limiter
func NewSmartRateLimiter(defaultLimit int64, learningWindow time.Duration) *SmartRateLimiter {
	return &SmartRateLimiter{
		userLimits:      make(map[string]*UserRateLimit),
		adaptiveEnabled: true,
		learningWindow:  learningWindow,
		defaultLimit:    defaultLimit,
		burstMultiplier: 1.5,
	}
}

// NewPerformanceOptimizer creates a new performance optimizer
func NewPerformanceOptimizer(cacheSize int, cacheTTL time.Duration) *PerformanceOptimizer {
	return &PerformanceOptimizer{
		responseCache:      make(map[string]*CachedResponse),
		compressionCache:   make(map[string][]byte),
		optimizationStats:  &OptimizationStats{},
		cacheEnabled:       true,
		compressionEnabled: true,
		maxCacheSize:       cacheSize,
		cacheTTL:           cacheTTL,
	}
}

// NewSessionTracker creates a new session tracker
func NewSessionTracker(timeout time.Duration, maxSessions int) *SessionTracker {
	return &SessionTracker{
		activeSessions:  make(map[string]*EnhancedSession),
		sessionTimeout:  timeout,
		maxSessions:     maxSessions,
		securityEnabled: true,
	}
}

// NewThreatIntelligence creates a new threat intelligence system
func NewThreatIntelligence(updateInterval time.Duration) *ThreatIntelligence {
	return &ThreatIntelligence{
		maliciousIPs:       make(map[string]*ThreatInfo),
		suspiciousPatterns: make([]*ThreatPattern, 0),
		updateInterval:     updateInterval,
		enabled:            true,
	}
}

// Advanced Middleware Methods

// generateRequestFingerprint creates a unique fingerprint for a request
func (eum *EnhancedUnifiedMiddleware) generateRequestFingerprint(r *http.Request) *RequestFingerprint {
	// Create hash of relevant request components
	hasher := sha256.New()
	hasher.Write([]byte(r.Method))
	hasher.Write([]byte(r.URL.Path))
	hasher.Write([]byte(r.UserAgent()))

	// Hash query parameters
	queryStr := r.URL.Query().Encode()
	queryHash := sha256.Sum256([]byte(queryStr))

	// Simple body hash (for security, don't store full body)
	bodyHash := sha256.Sum256([]byte("")) // Empty for now, can be enhanced

	// Extract important headers
	headers := make(map[string]string)
	for _, header := range []string{"Content-Type", "Accept", "Authorization"} {
		if value := r.Header.Get(header); value != "" {
			headers[header] = value
		}
	}

	return &RequestFingerprint{
		Hash:      hex.EncodeToString(hasher.Sum(nil)),
		Method:    r.Method,
		Path:      r.URL.Path,
		Headers:   headers,
		QueryHash: hex.EncodeToString(queryHash[:]),
		BodyHash:  hex.EncodeToString(bodyHash[:]),
		UserAgent: r.UserAgent(),
		ClientIP:  eum.extractClientIP(r),
		Timestamp: time.Now(),
	}
}

// smartRateLimitMiddleware provides adaptive rate limiting
func (eum *EnhancedUnifiedMiddleware) smartRateLimitMiddleware(rateLimiter *SmartRateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			clientIP := eum.extractClientIP(r)

			if !rateLimiter.checkAndUpdateLimit(clientIP, r) {
				// Rate limit exceeded
				w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", rateLimiter.getCurrentLimit(clientIP)))
				w.Header().Set("X-RateLimit-Remaining", "0")
				w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", time.Now().Add(time.Minute).Unix()))

				http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// performanceOptimizationMiddleware handles caching and optimization
func (eum *EnhancedUnifiedMiddleware) performanceOptimizationMiddleware(optimizer *PerformanceOptimizer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check if response is cacheable
			if r.Method == "GET" && optimizer.cacheEnabled {
				cacheKey := eum.generateCacheKey(r)

				// Try to serve from cache
				if cachedResponse := optimizer.getFromCache(cacheKey); cachedResponse != nil {
					eum.serveCachedResponse(w, cachedResponse)
					optimizer.optimizationStats.CacheHits++
					return
				}
				optimizer.optimizationStats.CacheMisses++
			}

			// Wrap response writer to capture response for caching
			wrappedWriter := &cachingResponseWriter{
				ResponseWriter: w,
				body:           make([]byte, 0),
				statusCode:     http.StatusOK,
			}

			next.ServeHTTP(wrappedWriter, r)

			// Cache the response if appropriate
			if wrappedWriter.statusCode == http.StatusOK && optimizer.cacheEnabled {
				cacheKey := eum.generateCacheKey(r)
				optimizer.cacheResponse(cacheKey, wrappedWriter)
			}
		})
	}
}

// sessionTrackingMiddleware provides enhanced session management
func (eum *EnhancedUnifiedMiddleware) sessionTrackingMiddleware(tracker *SessionTracker) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sessionID := r.Header.Get("X-Session-ID")
			if sessionID == "" {
				// Look for session in cookies
				if cookie, err := r.Cookie("session_id"); err == nil {
					sessionID = cookie.Value
				}
			}

			if sessionID != "" {
				session := tracker.getSession(sessionID)
				if session != nil && session.IsValid {
					// Update session activity
					tracker.updateActivity(sessionID, r)

					// Add session info to request context
					ctx := context.WithValue(r.Context(), "session", session)
					r = r.WithContext(ctx)
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

// threatIntelligenceMiddleware provides real-time threat detection
func (eum *EnhancedUnifiedMiddleware) threatIntelligenceMiddleware(threatIntel *ThreatIntelligence) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !threatIntel.enabled {
				next.ServeHTTP(w, r)
				return
			}

			clientIP := eum.extractClientIP(r)

			// Check against threat intelligence
			if threat := threatIntel.checkIP(clientIP); threat != nil {
				// Log threat detection
				if eum.auditLogger.enabled {
					eum.auditLogger.logger.Warn("Threat intelligence match",
						zap.String("client_ip", clientIP),
						zap.String("threat_level", threat.ThreatLevel),
						zap.Strings("categories", threat.Categories),
						zap.String("source", threat.Source),
						zap.Float64("confidence", threat.Confidence),
					)
				}

				// Take action based on threat level
				switch threat.ThreatLevel {
				case "high":
					http.Error(w, "Access denied", http.StatusForbidden)
					return
				case "medium":
					// Add security headers and continue with monitoring
					w.Header().Set("X-Security-Warning", "Monitored")
				}
			}

			// Check request patterns against threat patterns
			if eum.checkThreatPatterns(r, threatIntel) {
				http.Error(w, "Suspicious activity detected", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// Enhanced helper methods

// generateCacheKey creates a cache key for the request
func (eum *EnhancedUnifiedMiddleware) generateCacheKey(r *http.Request) string {
	hasher := sha256.New()
	hasher.Write([]byte(r.Method))
	hasher.Write([]byte(r.URL.Path))
	hasher.Write([]byte(r.URL.RawQuery))

	// Include relevant headers that affect response
	for _, header := range []string{"Accept", "Accept-Encoding", "Accept-Language"} {
		hasher.Write([]byte(r.Header.Get(header)))
	}

	return hex.EncodeToString(hasher.Sum(nil))
}

// serveCachedResponse serves a cached response
func (eum *EnhancedUnifiedMiddleware) serveCachedResponse(w http.ResponseWriter, cached *CachedResponse) {
	// Set cached headers
	for key, value := range cached.Headers {
		w.Header().Set(key, value)
	}

	// Set cache headers
	w.Header().Set("X-Cache", "HIT")
	w.Header().Set("X-Cache-Timestamp", cached.Timestamp.Format(time.RFC3339))

	w.WriteHeader(cached.StatusCode)
	w.Write(cached.Body)
}

// checkThreatPatterns checks request against threat patterns
func (eum *EnhancedUnifiedMiddleware) checkThreatPatterns(r *http.Request, threatIntel *ThreatIntelligence) bool {
	threatIntel.mu.RLock()
	defer threatIntel.mu.RUnlock()

	requestData := fmt.Sprintf("%s %s %s %s",
		r.Method, r.URL.Path, r.URL.RawQuery, r.UserAgent())

	for _, pattern := range threatIntel.suspiciousPatterns {
		if pattern.Regex != nil && pattern.Regex.MatchString(requestData) {
			return true
		}
	}

	return false
}

// SmartRateLimiter methods

// checkAndUpdateLimit checks if request is within rate limit and updates counters
func (srl *SmartRateLimiter) checkAndUpdateLimit(clientIP string, r *http.Request) bool {
	srl.mu.Lock()
	defer srl.mu.Unlock()

	now := time.Now()
	userLimit, exists := srl.userLimits[clientIP]

	if !exists {
		userLimit = &UserRateLimit{
			CurrentLimit:  srl.defaultLimit,
			RequestCount:  0,
			WindowStart:   now,
			LastActivity:  now,
			BehaviorScore: 1.0,
			TrustedScore:  0.5,
		}
		srl.userLimits[clientIP] = userLimit
	}

	// Reset window if needed
	if now.Sub(userLimit.WindowStart) >= time.Minute {
		userLimit.RequestCount = 0
		userLimit.WindowStart = now

		// Adaptive adjustment based on behavior
		if srl.adaptiveEnabled {
			srl.adjustLimitBasedOnBehavior(userLimit)
		}
	}

	// Check limit
	if userLimit.RequestCount >= userLimit.CurrentLimit {
		userLimit.ViolationCount++
		return false
	}

	userLimit.RequestCount++
	userLimit.LastActivity = now
	return true
}

// getCurrentLimit returns current rate limit for client
func (srl *SmartRateLimiter) getCurrentLimit(clientIP string) int64 {
	srl.mu.RLock()
	defer srl.mu.RUnlock()

	if userLimit, exists := srl.userLimits[clientIP]; exists {
		return userLimit.CurrentLimit
	}
	return srl.defaultLimit
}

// adjustLimitBasedOnBehavior adjusts rate limit based on user behavior
func (srl *SmartRateLimiter) adjustLimitBasedOnBehavior(userLimit *UserRateLimit) {
	// Increase limit for trusted users
	if userLimit.TrustedScore > 0.8 && userLimit.ViolationCount == 0 {
		userLimit.CurrentLimit = int64(float64(srl.defaultLimit) * srl.burstMultiplier)
	} else if userLimit.ViolationCount > 3 {
		// Decrease limit for violators
		userLimit.CurrentLimit = srl.defaultLimit / 2
	} else {
		// Reset to default
		userLimit.CurrentLimit = srl.defaultLimit
	}
}

// PerformanceOptimizer methods

// getFromCache retrieves cached response
func (po *PerformanceOptimizer) getFromCache(key string) *CachedResponse {
	po.mu.RLock()
	defer po.mu.RUnlock()

	cached, exists := po.responseCache[key]
	if !exists {
		return nil
	}

	// Check if cache is expired
	if time.Since(cached.Timestamp) > cached.TTL {
		delete(po.responseCache, key)
		return nil
	}

	return cached
}

// cacheResponse stores response in cache
func (po *PerformanceOptimizer) cacheResponse(key string, writer *cachingResponseWriter) {
	po.mu.Lock()
	defer po.mu.Unlock()

	// Check cache size limit
	if len(po.responseCache) >= po.maxCacheSize {
		// Simple LRU: remove oldest entry
		oldestKey := ""
		oldestTime := time.Now()
		for k, v := range po.responseCache {
			if v.Timestamp.Before(oldestTime) {
				oldestTime = v.Timestamp
				oldestKey = k
			}
		}
		if oldestKey != "" {
			delete(po.responseCache, oldestKey)
		}
	}

	// Create cached response
	cached := &CachedResponse{
		StatusCode: writer.statusCode,
		Headers:    make(map[string]string),
		Body:       writer.body,
		Timestamp:  time.Now(),
		TTL:        po.cacheTTL,
		Compressed: false,
	}

	// Copy headers
	for key, values := range writer.Header() {
		if len(values) > 0 {
			cached.Headers[key] = values[0]
		}
	}

	po.responseCache[key] = cached
}

// SessionTracker methods

// getSession retrieves session by ID
func (st *SessionTracker) getSession(sessionID string) *EnhancedSession {
	st.mu.RLock()
	defer st.mu.RUnlock()

	session, exists := st.activeSessions[sessionID]
	if !exists {
		return nil
	}

	// Check if session is expired
	if time.Now().After(session.ExpiresAt) {
		delete(st.activeSessions, sessionID)
		return nil
	}

	return session
}

// updateActivity updates session activity
func (st *SessionTracker) updateActivity(sessionID string, r *http.Request) {
	st.mu.Lock()
	defer st.mu.Unlock()

	if session, exists := st.activeSessions[sessionID]; exists {
		session.LastActivity = time.Now()
		session.ExpiresAt = time.Now().Add(st.sessionTimeout)
		// Update IP address if changed (potential security concern)
		currentIP := extractClientIP(r)
		if session.IPAddress != currentIP && st.securityEnabled {
			session.SecurityFlags = append(session.SecurityFlags, "ip_changed")
		}
	}
}

// ThreatIntelligence methods

// checkIP checks if IP is in threat database
func (ti *ThreatIntelligence) checkIP(ip string) *ThreatInfo {
	ti.mu.RLock()
	defer ti.mu.RUnlock()

	// Clean IP (remove port if present)
	cleanIP := strings.Split(ip, ":")[0]

	threat, exists := ti.maliciousIPs[cleanIP]
	if !exists {
		return nil
	}

	return threat
}

// cachingResponseWriter wraps http.ResponseWriter to capture response data
type cachingResponseWriter struct {
	http.ResponseWriter
	body       []byte
	statusCode int
}

func (crw *cachingResponseWriter) Write(data []byte) (int, error) {
	crw.body = append(crw.body, data...)
	return crw.ResponseWriter.Write(data)
}

func (crw *cachingResponseWriter) WriteHeader(statusCode int) {
	crw.statusCode = statusCode
	crw.ResponseWriter.WriteHeader(statusCode)
}
