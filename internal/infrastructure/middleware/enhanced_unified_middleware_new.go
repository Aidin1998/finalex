// filepath: c:\Orbit CEX\Finalex\internal\infrastructure\middleware\enhanced_unified_middleware.go
// Package middleware provides enhanced unified middleware infrastructure for Orbit CEX
// This extends the base UnifiedMiddleware with additional advanced security features
package middleware

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/Aidin1998/finalex/internal/infrastructure/ratelimit"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// EnhancedSecurityConfig contains advanced security configuration
type EnhancedSecurityConfig struct {
	// Advanced threat detection
	ThreatDetectionEnabled  bool `json:"threat_detection_enabled"`
	AnomalyDetectionEnabled bool `json:"anomaly_detection_enabled"`
	BehaviorAnalysisEnabled bool `json:"behavior_analysis_enabled"`

	// Advanced rate limiting
	AdaptiveRateLimiting bool `json:"adaptive_rate_limiting"`
	BurstProtection      bool `json:"burst_protection"`

	// Enhanced logging and auditing
	DetailedAuditLogging bool `json:"detailed_audit_logging"`
	SecurityEventAlerts  bool `json:"security_event_alerts"`

	// Performance settings
	MaxConcurrentRequests  int           `json:"max_concurrent_requests"`
	RequestTimeoutDuration time.Duration `json:"request_timeout_duration"`
}

// AdvancedMetricsCollector handles enhanced metrics collection
type AdvancedMetricsCollector struct {
	mu sync.RWMutex

	// Advanced metrics data
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

	// Enhanced features
	securityConfig  *EnhancedSecurityConfig
	advancedMetrics *AdvancedMetricsCollector
	auditLogger     *AuditLogger

	// Additional state
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

			// Apply base middleware chain
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
		middlewareProcessingTime.WithLabelValues("enhanced_security", "total").Observe(processingTime.Seconds())
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

		middlewareProcessingTime.WithLabelValues("advanced_metrics", "collection").Observe(duration.Seconds())
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

		middlewareProcessingTime.WithLabelValues("audit_logging", "processing").Observe(duration.Seconds())
	})
}

// detectThreat performs threat detection analysis
func (eum *EnhancedUnifiedMiddleware) detectThreat(r *http.Request) bool {
	// Basic threat detection logic
	// This would be more sophisticated in a real implementation

	// Check for suspicious patterns
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

	// Increment security violations metric
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
	// Simplified anomaly detection
	clientIP := eum.extractClientIP(r)

	eum.advancedMetrics.mu.RLock()
	userMetrics, exists := eum.advancedMetrics.userBehaviorMetrics[clientIP]
	eum.advancedMetrics.mu.RUnlock()

	if !exists {
		return false
	}

	// Check for rapid fire requests (simple anomaly detection)
	if userMetrics.AverageInterval > 0 && userMetrics.AverageInterval < 0.1 { // Less than 100ms between requests
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
	// Record advanced metrics specific to enhanced middleware
	middlewareProcessingTime.WithLabelValues("enhanced_total", "request").Observe(duration.Seconds())
}

// extractClientIP extracts client IP (delegate to base middleware if available)
func (eum *EnhancedUnifiedMiddleware) extractClientIP(r *http.Request) string {
	// Try X-Forwarded-For first
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return xff
	}

	// Try X-Real-IP
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	return r.RemoteAddr
}

// AuditLogger methods

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

// DefaultEnhancedSecurityConfig returns default enhanced security configuration
func DefaultEnhancedSecurityConfig() *EnhancedSecurityConfig {
	return &EnhancedSecurityConfig{
		ThreatDetectionEnabled:  true,
		AnomalyDetectionEnabled: true,
		BehaviorAnalysisEnabled: true,
		AdaptiveRateLimiting:    false, // Disabled by default for stability
		BurstProtection:         true,
		DetailedAuditLogging:    true,
		SecurityEventAlerts:     false, // Disabled by default to avoid spam
		MaxConcurrentRequests:   10000,
		RequestTimeoutDuration:  30 * time.Second,
	}
}
