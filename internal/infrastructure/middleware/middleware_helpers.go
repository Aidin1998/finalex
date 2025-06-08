// Continuation of unified_middleware.go - Additional middleware functions
package middleware

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/Aidin1998/finalex/pkg/models"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
)

// UserInfo represents authenticated user information
type UserInfo struct {
	ID     string          `json:"id"`
	Email  string          `json:"email"`
	Tier   models.UserTier `json:"tier"`
	Role   string          `json:"role"`
	APIKey string          `json:"api_key,omitempty"`
}

// ResponseWriter wrapper for capturing response data
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	written    int64
	body       *bytes.Buffer
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	rw.written += int64(len(b))
	if rw.body != nil {
		rw.body.Write(b)
	}
	return rw.ResponseWriter.Write(b)
}

func (rw *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hijacker, ok := rw.ResponseWriter.(http.Hijacker); ok {
		return hijacker.Hijack()
	}
	return nil, nil, fmt.Errorf("hijacking not supported")
}

// loggingMiddleware handles request/response logging
func (um *UnifiedMiddleware) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Skip health check endpoints if configured
		if um.config.Logging.SkipHealthCheck && strings.Contains(r.URL.Path, "health") {
			next.ServeHTTP(w, r)
			return
		}

		// Skip configured paths
		for _, skipPath := range um.config.Logging.SkipPaths {
			if strings.HasPrefix(r.URL.Path, skipPath) {
				next.ServeHTTP(w, r)
				return
			}
		}

		// Capture request body if enabled
		var requestBody []byte
		if um.config.Logging.RequestBody && r.ContentLength > 0 && r.ContentLength <= int64(um.config.Logging.MaxBodyLogSize) {
			requestBody, _ = io.ReadAll(r.Body)
			r.Body = io.NopCloser(bytes.NewBuffer(requestBody))
		}

		// Create response writer wrapper
		var responseBody *bytes.Buffer
		if um.config.Logging.ResponseBody {
			responseBody = &bytes.Buffer{}
		}

		rw := &responseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
			body:           responseBody,
		}

		// Process request
		next.ServeHTTP(rw, r)

		duration := time.Since(start)

		// Prepare log fields
		logFields := make(map[string]interface{})

		if um.config.Logging.RequestID {
			if requestID := r.Context().Value("request_id"); requestID != nil {
				logFields["request_id"] = requestID
			}
		}

		if um.config.Logging.ClientIP {
			logFields["client_ip"] = extractClientIP(r)
		}

		if um.config.Logging.UserAgent {
			logFields["user_agent"] = r.UserAgent()
		}

		if um.config.Logging.Headers {
			headers := make(map[string]string)
			for k, v := range r.Header {
				if !um.isSensitiveHeader(k) {
					headers[k] = strings.Join(v, ",")
				}
			}
			logFields["headers"] = headers
		}

		if um.config.Logging.QueryParams {
			logFields["query_params"] = r.URL.RawQuery
		}

		if um.config.Logging.Duration {
			logFields["duration"] = duration
		}

		if um.config.Logging.StatusCode {
			logFields["status_code"] = rw.statusCode
		}

		logFields["method"] = r.Method
		logFields["path"] = r.URL.Path
		logFields["response_size"] = rw.written

		if um.config.Logging.RequestBody && len(requestBody) > 0 {
			logFields["request_body"] = um.sanitizeBody(requestBody)
		}

		if um.config.Logging.ResponseBody && responseBody != nil && responseBody.Len() > 0 {
			logFields["response_body"] = um.sanitizeBody(responseBody.Bytes())
		}

		// Log based on status code and duration
		if rw.statusCode >= 500 {
			um.logger.Error("HTTP request completed with server error", logFields)
		} else if rw.statusCode >= 400 {
			um.logger.Warn("HTTP request completed with client error", logFields)
		} else if duration > um.config.Logging.SlowRequestThreshold {
			um.logger.Warn("Slow HTTP request", logFields)
		} else {
			um.logger.Info("HTTP request completed", logFields)
		}

		// Audit logging for sensitive operations
		if um.config.Logging.AuditLogging {
			um.auditLog(r, rw.statusCode, duration)
		}
	})
}

// metricsMiddleware handles Prometheus metrics collection
func (um *UnifiedMiddleware) metricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		httpActiveConnections.Inc()
		defer httpActiveConnections.Dec()

		// Capture request size
		if um.config.Metrics.RequestSize {
			httpRequestSize.WithLabelValues(r.Method, r.URL.Path).Observe(float64(r.ContentLength))
		}

		// Create response writer wrapper
		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(rw, r)

		duration := time.Since(start)
		statusCode := strconv.Itoa(rw.statusCode)

		// Extract user tier for metrics
		userTier := "anonymous"
		if userInfo := um.getUserFromContext(r.Context()); userInfo != nil {
			userTier = string(userInfo.Tier)
		}

		// Record metrics
		if um.config.Metrics.RequestCount {
			httpRequestsTotal.WithLabelValues(r.Method, r.URL.Path, statusCode, userTier).Inc()
		}

		if um.config.Metrics.RequestDuration {
			httpRequestDuration.WithLabelValues(r.Method, r.URL.Path, statusCode).Observe(duration.Seconds())
		}

		if um.config.Metrics.ResponseSize {
			httpResponseSize.WithLabelValues(r.Method, r.URL.Path, statusCode).Observe(float64(rw.written))
		}
	})
}

// tracingMiddleware handles OpenTelemetry tracing
func (um *UnifiedMiddleware) tracingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract trace context from headers
		ctx := um.propagator.Extract(r.Context(), propagation.HeaderCarrier(r.Header))

		// Start span
		spanName := fmt.Sprintf("%s %s", r.Method, r.URL.Path)
		ctx, span := tracer.Start(ctx, spanName)
		defer span.End()

		// Set span attributes
		span.SetAttributes(
			attribute.String("http.method", r.Method),
			attribute.String("http.url", r.URL.String()),
			attribute.String("http.scheme", r.URL.Scheme),
			attribute.String("http.host", r.Host),
			attribute.String("http.target", r.URL.Path),
			attribute.String("http.user_agent", r.UserAgent()),
			attribute.String("http.remote_addr", r.RemoteAddr),
		)

		if um.config.Tracing.TraceHeaders {
			for k, v := range r.Header {
				if !um.isSensitiveHeader(k) {
					span.SetAttributes(attribute.String("http.request.header."+strings.ToLower(k), strings.Join(v, ",")))
				}
			}
		}

		// Add user info to span
		if userInfo := um.getUserFromContext(ctx); userInfo != nil {
			span.SetAttributes(
				attribute.String("user.id", userInfo.ID),
				attribute.String("user.tier", string(userInfo.Tier)),
				attribute.String("user.role", userInfo.Role),
			)
		}

		// Create response writer wrapper
		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		// Inject trace context into response headers
		um.propagator.Inject(ctx, propagation.HeaderCarrier(w.Header()))

		// Process request with trace context
		r = r.WithContext(ctx)
		next.ServeHTTP(rw, r)

		// Set response attributes
		span.SetAttributes(
			attribute.Int("http.status_code", rw.statusCode),
			attribute.Int64("http.response_size", rw.written),
		)

		// Mark span as error if status code indicates error
		if rw.statusCode >= 400 {
			span.SetStatus(codes.Error, fmt.Sprintf("HTTP %d", rw.statusCode))
			if um.config.Tracing.TraceErrors {
				span.SetAttributes(attribute.String("error", "true"))
			}
		}
	})
}

// recoveryMiddleware handles panic recovery
func (um *UnifiedMiddleware) recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				panicRecoveries.WithLabelValues(r.URL.Path, "panic").Inc()

				// Capture stack trace
				stack := debug.Stack()

				// Prepare recovery info
				recoveryInfo := map[string]interface{}{
					"panic":   err,
					"method":  r.Method,
					"path":    r.URL.Path,
					"url":     r.URL.String(),
					"headers": r.Header,
				}

				if um.config.Recovery.IncludeHeaders {
					recoveryInfo["headers"] = r.Header
				}

				if um.config.Recovery.IncludeQueryParams {
					recoveryInfo["query_params"] = r.URL.Query()
				}

				if um.config.Recovery.IncludeRequestBody && r.ContentLength > 0 {
					body, _ := io.ReadAll(r.Body)
					recoveryInfo["request_body"] = string(body)
				}

				if um.config.Recovery.LogStackTrace {
					recoveryInfo["stack_trace"] = string(stack)
				}

				// Log the panic
				um.logger.Error("Panic recovered", recoveryInfo)

				// Print stack trace if configured
				if um.config.Recovery.PrintStack {
					fmt.Printf("Panic recovered: %v\n%s\n", err, stack)
				}

				// Send error response
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)

				// Notify if channels configured
				um.notifyPanic(err, stack, r)
			}
		}()

		next.ServeHTTP(w, r)
	})
}

// Helper functions

func (um *UnifiedMiddleware) shouldSkipAuth(path string) bool {
	skipPaths := []string{
		"/health",
		"/metrics",
		"/public",
		"/auth/login",
		"/auth/register",
	}

	for _, skipPath := range skipPaths {
		if strings.HasPrefix(path, skipPath) {
			return true
		}
	}

	// Check configured regex patterns
	for _, pattern := range um.config.Authentication.SkipPathsRegex {
		if matched, _ := regexp.MatchString(pattern, path); matched {
			return true
		}
	}

	return false
}

func (um *UnifiedMiddleware) extractAuthToken(r *http.Request) string {
	// Try Authorization header first
	auth := r.Header.Get("Authorization")
	if auth != "" {
		if strings.HasPrefix(auth, "Bearer ") {
			return strings.TrimPrefix(auth, "Bearer ")
		}
	}

	// Try API key header
	if apiKey := r.Header.Get("X-API-Key"); apiKey != "" {
		return apiKey
	}

	// Try cookie
	if cookie, err := r.Cookie("auth_token"); err == nil {
		return cookie.Value
	}

	// Try query parameter (less secure, but sometimes necessary)
	if token := r.URL.Query().Get("token"); token != "" {
		return token
	}

	return ""
}

func (um *UnifiedMiddleware) validateAuthToken(token string) (*UserInfo, error) {
	// This would integrate with your actual authentication service
	// For now, return a mock implementation

	// TODO: Implement JWT validation, session lookup, API key validation, etc.
	userInfo := &UserInfo{
		ID:    "user123",
		Email: "user@example.com",
		Tier:  models.TierBasic,
		Role:  "user",
	}

	return userInfo, nil
}

func (um *UnifiedMiddleware) addUserToContext(ctx context.Context, userInfo *UserInfo) context.Context {
	return context.WithValue(ctx, "user", userInfo)
}

func (um *UnifiedMiddleware) getUserFromContext(ctx context.Context) *UserInfo {
	if user := ctx.Value("user"); user != nil {
		if userInfo, ok := user.(*UserInfo); ok {
			return userInfo
		}
	}
	return nil
}

func (um *UnifiedMiddleware) hasPermission(userInfo *UserInfo, method, path string) bool {
	// This would integrate with your actual RBAC system
	// For now, return a simple implementation

	if userInfo.Role == "super_admin" {
		return true
	}

	// Add your permission logic here
	return true
}

func (um *UnifiedMiddleware) writeAuthError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]string{
		"error":   "authentication_failed",
		"message": message,
	})
}

func (um *UnifiedMiddleware) validateHeaders(r *http.Request) error {
	// Check required headers
	for _, required := range um.config.Validation.RequiredHeaders {
		if r.Header.Get(required) == "" {
			return fmt.Errorf("missing required header: %s", required)
		}
	}

	// Check forbidden headers
	for _, forbidden := range um.config.Validation.ForbiddenHeaders {
		if r.Header.Get(forbidden) != "" {
			return fmt.Errorf("forbidden header present: %s", forbidden)
		}
	}

	// Check header size
	headerSize := 0
	for k, v := range r.Header {
		headerSize += len(k) + len(strings.Join(v, ""))
	}

	if headerSize > um.config.Validation.MaxHeaderSize {
		return fmt.Errorf("headers too large: %d bytes", headerSize)
	}

	return nil
}

func (um *UnifiedMiddleware) validateQueryParams(r *http.Request) error {
	params := r.URL.Query()

	if len(params) > um.config.Validation.MaxQueryParams {
		return fmt.Errorf("too many query parameters: %d", len(params))
	}

	return nil
}

func (um *UnifiedMiddleware) detectSQLInjection(r *http.Request) bool {
	sqlPatterns := []string{
		`(?i)(union\s+select)`,
		`(?i)(select\s+\*\s+from)`,
		`(?i)(drop\s+table)`,
		`(?i)(insert\s+into)`,
		`(?i)(delete\s+from)`,
		`(?i)(update\s+\w+\s+set)`,
		`(?i)(\'\s*or\s*\'1\'\s*=\s*\'1)`,
		`(?i)(\'\s*or\s*1\s*=\s*1)`,
	}

	// Check URL path, query parameters, and form data
	checkString := r.URL.Path + "?" + r.URL.RawQuery

	for _, pattern := range sqlPatterns {
		if matched, _ := regexp.MatchString(pattern, checkString); matched {
			return true
		}
	}

	return false
}

func (um *UnifiedMiddleware) detectXSS(r *http.Request) bool {
	xssPatterns := []string{
		`(?i)<script[^>]*>`,
		`(?i)javascript:`,
		`(?i)on\w+\s*=`,
		`(?i)<iframe[^>]*>`,
		`(?i)<object[^>]*>`,
		`(?i)<embed[^>]*>`,
	}

	// Check URL path and query parameters
	checkString := r.URL.Path + "?" + r.URL.RawQuery

	for _, pattern := range xssPatterns {
		if matched, _ := regexp.MatchString(pattern, checkString); matched {
			return true
		}
	}

	return false
}

func (um *UnifiedMiddleware) isOriginAllowed(origin string) bool {
	for _, allowed := range um.config.CORS.AllowOrigins {
		if allowed == "*" || allowed == origin {
			return true
		}
		// Support wildcard subdomains
		if strings.HasPrefix(allowed, "*.") {
			domain := strings.TrimPrefix(allowed, "*.")
			if strings.HasSuffix(origin, domain) {
				return true
			}
		}
	}
	return false
}

func (um *UnifiedMiddleware) isIPAllowed(clientIP string) bool {
	for _, allowedIP := range um.config.Security.IPRestrictions {
		if allowedIP == clientIP {
			return true
		}
		// Support CIDR notation
		if strings.Contains(allowedIP, "/") {
			if _, network, err := net.ParseCIDR(allowedIP); err == nil {
				if ip := net.ParseIP(clientIP); ip != nil && network.Contains(ip) {
					return true
				}
			}
		}
	}
	return len(um.config.Security.IPRestrictions) == 0 // Allow if no restrictions configured
}

func (um *UnifiedMiddleware) isSensitiveHeader(name string) bool {
	sensitiveHeaders := []string{
		"authorization",
		"x-api-key",
		"cookie",
		"x-auth-token",
	}

	lowerName := strings.ToLower(name)
	for _, sensitive := range sensitiveHeaders {
		if lowerName == sensitive {
			return true
		}
	}

	return false
}

func (um *UnifiedMiddleware) sanitizeBody(body []byte) string {
	// Remove sensitive fields from logged body
	var data interface{}
	if json.Unmarshal(body, &data) == nil {
		if sanitized := um.removeSensitiveFields(data); sanitized != nil {
			if sanitizedBytes, err := json.Marshal(sanitized); err == nil {
				return string(sanitizedBytes)
			}
		}
	}

	// If not JSON or sanitization failed, truncate and return
	bodyStr := string(body)
	if len(bodyStr) > um.config.Logging.MaxBodyLogSize {
		return bodyStr[:um.config.Logging.MaxBodyLogSize] + "..."
	}
	return bodyStr
}

func (um *UnifiedMiddleware) removeSensitiveFields(data interface{}) interface{} {
	switch v := data.(type) {
	case map[string]interface{}:
		sanitized := make(map[string]interface{})
		for key, value := range v {
			if um.isSensitiveField(key) {
				sanitized[key] = "[REDACTED]"
			} else {
				sanitized[key] = um.removeSensitiveFields(value)
			}
		}
		return sanitized
	case []interface{}:
		sanitized := make([]interface{}, len(v))
		for i, item := range v {
			sanitized[i] = um.removeSensitiveFields(item)
		}
		return sanitized
	default:
		return v
	}
}

func (um *UnifiedMiddleware) isSensitiveField(field string) bool {
	sensitiveFields := []string{
		"password",
		"token",
		"secret",
		"key",
		"auth",
		"credential",
		"ssn",
		"credit_card",
		"card_number",
	}

	// Add configured sensitive fields
	sensitiveFields = append(sensitiveFields, um.config.Logging.SensitiveFields...)

	lowerField := strings.ToLower(field)
	for _, sensitive := range sensitiveFields {
		if strings.Contains(lowerField, strings.ToLower(sensitive)) {
			return true
		}
	}

	return false
}

func (um *UnifiedMiddleware) auditLog(r *http.Request, statusCode int, duration time.Duration) {
	// Log important operations for audit trail
	sensitiveOperations := []string{
		"/auth/login",
		"/auth/logout",
		"/user/profile",
		"/admin/",
		"/trading/order",
		"/wallet/withdraw",
		"/wallet/deposit",
	}

	for _, operation := range sensitiveOperations {
		if strings.HasPrefix(r.URL.Path, operation) {
			auditFields := map[string]interface{}{
				"operation":   operation,
				"method":      r.Method,
				"path":        r.URL.Path,
				"status_code": statusCode,
				"duration":    duration,
				"client_ip":   extractClientIP(r),
				"user_agent":  r.UserAgent(),
				"timestamp":   time.Now().UTC(),
			}

			if userInfo := um.getUserFromContext(r.Context()); userInfo != nil {
				auditFields["user_id"] = userInfo.ID
				auditFields["user_role"] = userInfo.Role
			}

			um.logger.Info("AUDIT", auditFields)
			break
		}
	}
}

func (um *UnifiedMiddleware) notifyPanic(err interface{}, stack []byte, r *http.Request) {
	// Implement notification channels (Slack, email, etc.)
	// This is a placeholder for actual notification implementation
	um.logger.Error("Panic notification", map[string]interface{}{
		"panic":     err,
		"stack":     string(stack),
		"url":       r.URL.String(),
		"method":    r.Method,
		"timestamp": time.Now().UTC(),
	})
}
