// Package middleware provides a unified middleware infrastructure for Orbit CEX
// This consolidates all middleware functionality including rate limiting, authentication,
// RBAC, security headers, CORS, validation, metrics, logging, and tracing.
package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Aidin1998/finalex/internal/infrastructure/ratelimit"
	"github.com/Aidin1998/finalex/pkg/logger"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

var (
	tracer = otel.Tracer("middleware")

	// Prometheus metrics
	httpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "orbit",
			Subsystem: "http",
			Name:      "requests_total",
			Help:      "Total number of HTTP requests",
		},
		[]string{"method", "path", "status_code", "user_tier"},
	)

	httpRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "orbit",
			Subsystem: "http",
			Name:      "request_duration_seconds",
			Help:      "HTTP request duration in seconds",
			Buckets:   []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		},
		[]string{"method", "path", "status_code"},
	)

	httpRequestSize = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "orbit",
			Subsystem: "http",
			Name:      "request_size_bytes",
			Help:      "HTTP request size in bytes",
			Buckets:   prometheus.ExponentialBuckets(100, 10, 8),
		},
		[]string{"method", "path"},
	)

	httpResponseSize = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "orbit",
			Subsystem: "http",
			Name:      "response_size_bytes",
			Help:      "HTTP response size in bytes",
			Buckets:   prometheus.ExponentialBuckets(100, 10, 8),
		},
		[]string{"method", "path", "status_code"},
	)

	httpActiveConnections = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "orbit",
			Subsystem: "http",
			Name:      "active_connections",
			Help:      "Number of active HTTP connections",
		},
	)

	authenticationAttempts = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "orbit",
			Subsystem: "auth",
			Name:      "attempts_total",
			Help:      "Total number of authentication attempts",
		},
		[]string{"method", "status"},
	)

	securityViolations = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "orbit",
			Subsystem: "security",
			Name:      "violations_total",
			Help:      "Total number of security violations",
		},
		[]string{"type", "severity"},
	)

	validationErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "orbit",
			Subsystem: "validation",
			Name:      "errors_total",
			Help:      "Total number of validation errors",
		},
		[]string{"type", "field"},
	)

	panicRecoveries = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "orbit",
			Subsystem: "recovery",
			Name:      "panics_total",
			Help:      "Total number of recovered panics",
		},
		[]string{"endpoint", "type"},
	)
)

// UnifiedMiddleware provides comprehensive middleware functionality
type UnifiedMiddleware struct {
	config      *UnifiedMiddlewareConfig
	rateLimiter *ratelimit.EnhancedRateLimiter
	logger      logger.Logger
	propagator  propagation.TextMapPropagator
}

// NewUnifiedMiddleware creates a new unified middleware instance
func NewUnifiedMiddleware(config *UnifiedMiddlewareConfig, rateLimiter *ratelimit.EnhancedRateLimiter, log logger.Logger) *UnifiedMiddleware {
	return &UnifiedMiddleware{
		config:      config,
		rateLimiter: rateLimiter,
		logger:      log,
		propagator:  otel.GetTextMapPropagator(),
	}
}

// Handler returns the complete middleware chain
func (um *UnifiedMiddleware) Handler() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Apply middleware chain in order
			handler := next

			// Recovery middleware (outermost)
			if um.config.Recovery != nil && um.config.Recovery.Enabled {
				handler = um.recoveryMiddleware(handler)
			}

			// Metrics middleware
			if um.config.Metrics != nil && um.config.Metrics.Enabled {
				handler = um.metricsMiddleware(handler)
			}

			// Tracing middleware
			if um.config.Tracing != nil && um.config.Tracing.Enabled {
				handler = um.tracingMiddleware(handler)
			}

			// Logging middleware
			if um.config.Logging != nil && um.config.Logging.Enabled {
				handler = um.loggingMiddleware(handler)
			}

			// Security headers middleware
			if um.config.Security != nil && um.config.Security.Enabled {
				handler = um.securityMiddleware(handler)
			}

			// CORS middleware
			if um.config.CORS != nil && um.config.CORS.Enabled {
				handler = um.corsMiddleware(handler)
			}

			// Validation middleware
			if um.config.Validation != nil && um.config.Validation.Enabled {
				handler = um.validationMiddleware(handler)
			}

			// Rate limiting middleware
			if um.config.RateLimit != nil && um.config.RateLimit.Enabled {
				handler = um.rateLimitMiddleware(handler)
			}

			// RBAC middleware
			if um.config.RBAC != nil && um.config.RBAC.Enabled {
				handler = um.rbacMiddleware(handler)
			}

			// Authentication middleware
			if um.config.Authentication != nil && um.config.Authentication.Enabled {
				handler = um.authenticationMiddleware(handler)
			}

			// Request ID middleware (innermost)
			handler = um.requestIDMiddleware(handler)

			handler.ServeHTTP(w, r)
		})
	}
}

// requestIDMiddleware adds a unique request ID to each request
func (um *UnifiedMiddleware) requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get(um.config.RequestIDHeader)
		if requestID == "" {
			requestID = uuid.New().String()
		}

		w.Header().Set(um.config.RequestIDHeader, requestID)
		r = r.WithContext(context.WithValue(r.Context(), "request_id", requestID))

		next.ServeHTTP(w, r)
	})
}

// authenticationMiddleware handles authentication
func (um *UnifiedMiddleware) authenticationMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip authentication for certain paths
		if um.shouldSkipAuth(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		authStart := time.Now()

		// Extract authentication credentials
		token := um.extractAuthToken(r)
		if token == "" {
			authenticationAttempts.WithLabelValues("missing", "failed").Inc()
			um.writeAuthError(w, "Missing authentication token", http.StatusUnauthorized)
			return
		}

		// Validate token and extract user info
		userInfo, err := um.validateAuthToken(token)
		if err != nil {
			authenticationAttempts.WithLabelValues("token", "failed").Inc()
			um.writeAuthError(w, "Invalid authentication token", http.StatusUnauthorized)
			return
		}

		authenticationAttempts.WithLabelValues("token", "success").Inc()

		// Add user info to context
		ctx := um.addUserToContext(r.Context(), userInfo)
		r = r.WithContext(ctx)

		// Add user headers for downstream middleware
		r.Header.Set("X-User-ID", userInfo.ID)
		r.Header.Set("X-User-Tier", string(userInfo.Tier))
		r.Header.Set("X-User-Role", userInfo.Role)

		// Log authentication
		if um.config.Logging.Enabled {
			um.logger.Info("User authenticated",
				"user_id", userInfo.ID,
				"tier", userInfo.Tier,
				"role", userInfo.Role,
				"duration", time.Since(authStart),
			)
		}

		next.ServeHTTP(w, r)
	})
}

// rbacMiddleware handles role-based access control
func (um *UnifiedMiddleware) rbacMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userInfo := um.getUserFromContext(r.Context())
		if userInfo == nil {
			// User not authenticated, let it pass to auth middleware
			next.ServeHTTP(w, r)
			return
		}

		// Check permissions
		if !um.hasPermission(userInfo, r.Method, r.URL.Path) {
			if um.config.RBAC.LogDeniedRequests {
				um.logger.Warn("Access denied",
					"user_id", userInfo.ID,
					"role", userInfo.Role,
					"method", r.Method,
					"path", r.URL.Path,
				)
			}

			securityViolations.WithLabelValues("rbac", "medium").Inc()

			http.Error(w, "Insufficient permissions", um.config.RBAC.PermissionDeniedCode)
			return
		}

		if um.config.RBAC.LogAccessAttempts {
			um.logger.Debug("Access granted",
				"user_id", userInfo.ID,
				"role", userInfo.Role,
				"method", r.Method,
				"path", r.URL.Path,
			)
		}

		next.ServeHTTP(w, r)
	})
}

// rateLimitMiddleware handles rate limiting
func (um *UnifiedMiddleware) rateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if um.rateLimiter == nil {
			next.ServeHTTP(w, r)
			return
		}

		allowed, retryAfter, headers, err := um.rateLimiter.Check(r.Context(), r)

		// Set rate limit headers
		for k, v := range headers {
			w.Header().Set(k, v)
		}

		if err != nil {
			um.logger.Error("Rate limit check failed", "error", err)
			if um.config.RateLimit.FailureMode == "deny" {
				http.Error(w, "Rate limit service unavailable", http.StatusServiceUnavailable)
				return
			}
			// Continue with request in "allow" failure mode
		} else if !allowed {
			w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())))
			http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// validationMiddleware handles input validation
func (um *UnifiedMiddleware) validationMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check request size
		if r.ContentLength > um.config.Validation.MaxRequestSize {
			validationErrors.WithLabelValues("size", "content_length").Inc()
			http.Error(w, "Request too large", http.StatusRequestEntityTooLarge)
			return
		}

		// Validate headers
		if err := um.validateHeaders(r); err != nil {
			validationErrors.WithLabelValues("headers", "invalid").Inc()
			http.Error(w, fmt.Sprintf("Invalid headers: %v", err), http.StatusBadRequest)
			return
		}

		// Validate query parameters
		if err := um.validateQueryParams(r); err != nil {
			validationErrors.WithLabelValues("query", "invalid").Inc()
			http.Error(w, fmt.Sprintf("Invalid query parameters: %v", err), http.StatusBadRequest)
			return
		}

		// Security checks
		if um.config.Validation.SQLInjectionProtection {
			if um.detectSQLInjection(r) {
				securityViolations.WithLabelValues("sql_injection", "high").Inc()
				http.Error(w, "Security violation detected", http.StatusBadRequest)
				return
			}
		}

		if um.config.Validation.XSSProtection {
			if um.detectXSS(r) {
				securityViolations.WithLabelValues("xss", "high").Inc()
				http.Error(w, "Security violation detected", http.StatusBadRequest)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

// corsMiddleware handles Cross-Origin Resource Sharing
func (um *UnifiedMiddleware) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		// Check if origin is allowed
		if origin != "" && um.isOriginAllowed(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}

		if um.config.CORS.AllowCredentials {
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}

		// Set allowed methods
		if len(um.config.CORS.AllowMethods) > 0 {
			w.Header().Set("Access-Control-Allow-Methods", strings.Join(um.config.CORS.AllowMethods, ", "))
		}

		// Set allowed headers
		if len(um.config.CORS.AllowHeaders) > 0 {
			w.Header().Set("Access-Control-Allow-Headers", strings.Join(um.config.CORS.AllowHeaders, ", "))
		}

		// Set exposed headers
		if len(um.config.CORS.ExposeHeaders) > 0 {
			w.Header().Set("Access-Control-Expose-Headers", strings.Join(um.config.CORS.ExposeHeaders, ", "))
		}

		// Set max age
		if um.config.CORS.MaxAge > 0 {
			w.Header().Set("Access-Control-Max-Age", strconv.Itoa(int(um.config.CORS.MaxAge.Seconds())))
		}

		// Handle preflight request
		if r.Method == "OPTIONS" {
			if um.config.CORS.OptionsPassthru {
				next.ServeHTTP(w, r)
			} else {
				w.WriteHeader(http.StatusNoContent)
			}
			return
		}

		next.ServeHTTP(w, r)
	})
}

// securityMiddleware adds security headers
func (um *UnifiedMiddleware) securityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Content-Type nosniff
		if um.config.Security.ContentTypeNoSniff {
			w.Header().Set("X-Content-Type-Options", "nosniff")
		}

		// Frame options
		if um.config.Security.FrameOptions != "" {
			w.Header().Set("X-Frame-Options", um.config.Security.FrameOptions)
		}

		// XSS protection
		if um.config.Security.XSSProtection != "" {
			w.Header().Set("X-XSS-Protection", um.config.Security.XSSProtection)
		}

		// Content Security Policy
		if um.config.Security.ContentSecurityPolicy != "" {
			w.Header().Set("Content-Security-Policy", um.config.Security.ContentSecurityPolicy)
		}

		// Referrer Policy
		if um.config.Security.ReferrerPolicy != "" {
			w.Header().Set("Referrer-Policy", um.config.Security.ReferrerPolicy)
		}

		// HSTS
		if um.config.Security.HSTSEnabled {
			hstsHeader := fmt.Sprintf("max-age=%d", um.config.Security.HSTSMaxAge)
			if um.config.Security.HSTSIncludeSubdomains {
				hstsHeader += "; includeSubDomains"
			}
			if um.config.Security.HSTSPreload {
				hstsHeader += "; preload"
			}
			w.Header().Set("Strict-Transport-Security", hstsHeader)
		}

		// Hide powered by
		if um.config.Security.HidePoweredBy {
			w.Header().Del("X-Powered-By")
			w.Header().Del("Server")
		}

		// IP restrictions
		if len(um.config.Security.IPRestrictions) > 0 {
			clientIP := extractClientIP(r)
			if !um.isIPAllowed(clientIP) {
				securityViolations.WithLabelValues("ip_restriction", "high").Inc()
				http.Error(w, "Access denied", http.StatusForbidden)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

// Helper function to extract client IP (same as in rate limiting)
func extractClientIP(r *http.Request) string {
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		parts := strings.Split(ip, ",")
		return strings.TrimSpace(parts[0])
	}
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	if ip := r.Header.Get("CF-Connecting-IP"); ip != "" {
		return ip
	}
	return strings.Split(r.RemoteAddr, ":")[0]
}

// Continue in the next part due to length limits...
