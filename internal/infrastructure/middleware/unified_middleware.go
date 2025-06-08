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
	"sync"
	"time"

	"github.com/Aidin1998/finalex/internal/infrastructure/ratelimit"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.uber.org/zap"
)

// Note: Metrics variables are declared in middleware_helpers.go to avoid redeclaration

// UnifiedMiddleware provides comprehensive middleware functionality
type UnifiedMiddleware struct {
	config            *UnifiedMiddlewareConfig
	rateLimitManager  *ratelimit.RateLimitManager
	logger            *zap.Logger
	propagator        propagation.TextMapPropagator
	redis             *redis.Client
	activeConnections *sync.Map
	requestRegistry   *sync.Map
}

// Note: UserInfo struct is defined in middleware_helpers.go

// RequestContext contains information about the current request
type RequestContext struct {
	ID              string                 `json:"id"`
	StartTime       time.Time              `json:"start_time"`
	Method          string                 `json:"method"`
	Path            string                 `json:"path"`
	UserAgent       string                 `json:"user_agent"`
	ClientIP        string                 `json:"client_ip"`
	User            *UserInfo              `json:"user,omitempty"`
	TraceID         string                 `json:"trace_id"`
	SpanID          string                 `json:"span_id"`
	Metadata        map[string]interface{} `json:"metadata"`
	SecurityContext map[string]interface{} `json:"security_context"`
}

// ResponseWriter wraps http.ResponseWriter to capture response information
type ResponseWriter struct {
	http.ResponseWriter
	statusCode int
	size       int64
	written    bool
}

func (rw *ResponseWriter) WriteHeader(code int) {
	if !rw.written {
		rw.statusCode = code
		rw.written = true
		rw.ResponseWriter.WriteHeader(code)
	}
}

func (rw *ResponseWriter) Write(data []byte) (int, error) {
	if !rw.written {
		rw.WriteHeader(http.StatusOK)
	}
	n, err := rw.ResponseWriter.Write(data)
	rw.size += int64(n)
	return n, err
}

func (rw *ResponseWriter) StatusCode() int {
	if rw.statusCode == 0 {
		return http.StatusOK
	}
	return rw.statusCode
}

func (rw *ResponseWriter) Size() int64 {
	return rw.size
}

// NewUnifiedMiddleware creates a new unified middleware instance
func NewUnifiedMiddleware(
	config *UnifiedMiddlewareConfig,
	rateLimitManager *ratelimit.RateLimitManager,
	logger *zap.Logger,
	redis *redis.Client,
) *UnifiedMiddleware {
	return &UnifiedMiddleware{
		config:            config,
		rateLimitManager:  rateLimitManager,
		logger:            logger,
		propagator:        otel.GetTextMapPropagator(),
		redis:             redis,
		activeConnections: &sync.Map{},
		requestRegistry:   &sync.Map{},
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
		r.Header.Set("X-User-Role", userInfo.Role) // Log authentication
		if um.config.Logging.Enabled {
			um.logger.Info("User authenticated",
				zap.String("user_id", userInfo.ID),
				zap.String("tier", string(userInfo.Tier)),
				zap.String("role", userInfo.Role),
				zap.Duration("duration", time.Since(authStart)),
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
					zap.String("user_id", userInfo.ID),
					zap.String("role", userInfo.Role),
					zap.String("method", r.Method),
					zap.String("path", r.URL.Path),
				)
			}

			securityViolations.WithLabelValues("rbac", "medium").Inc()

			http.Error(w, "Insufficient permissions", um.config.RBAC.PermissionDeniedCode)
			return
		}
		if um.config.RBAC.LogAccessAttempts {
			um.logger.Debug("Access granted",
				zap.String("user_id", userInfo.ID),
				zap.String("role", userInfo.Role),
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
			)
		}

		next.ServeHTTP(w, r)
	})
}

// rateLimitMiddleware handles rate limiting
func (um *UnifiedMiddleware) rateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if um.rateLimitManager == nil {
			next.ServeHTTP(w, r)
			return
		}
		result, err := um.rateLimitManager.Check(r.Context(), r)
		// Set rate limit headers
		for k, v := range result.Headers {
			w.Header().Set(k, v)
		}

		if err != nil {
			um.logger.Error("Rate limit check failed", zap.Error(err))
			if um.config.RateLimit.FailureMode == "deny" {
				http.Error(w, "Rate limit service unavailable", http.StatusServiceUnavailable)
				return
			}
			// Continue with request in "allow" failure mode
		} else if !result.Allowed {
			w.Header().Set("Retry-After", strconv.Itoa(int(result.RetryAfter.Seconds())))
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
