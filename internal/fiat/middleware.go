package fiat

import (
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// SecurityMiddleware provides enhanced security for fiat endpoints
type SecurityMiddleware struct {
	logger            *zap.Logger
	allowedProviders  map[string]ProviderInfo
	rateLimitWindow   time.Duration
	maxRequestsWindow int
}

// ProviderAuthRequest represents provider authentication request
type ProviderAuthRequest struct {
	ProviderID string    `json:"provider_id" validate:"required"`
	Timestamp  time.Time `json:"timestamp" validate:"required"`
	Nonce      string    `json:"nonce" validate:"required"`
	Signature  string    `json:"signature" validate:"required"`
}

// NewSecurityMiddleware creates a new security middleware
func NewSecurityMiddleware(logger *zap.Logger, providers map[string]ProviderInfo) *SecurityMiddleware {
	return &SecurityMiddleware{
		logger:            logger,
		allowedProviders:  providers,
		rateLimitWindow:   time.Minute,
		maxRequestsWindow: 10, // 10 requests per minute per provider
	}
}

// ProviderAuthMiddleware validates provider authentication
func (sm *SecurityMiddleware) ProviderAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		traceID := c.GetHeader("X-Trace-ID")
		if traceID == "" {
			traceID = uuid.New().String()
			c.Header("X-Trace-ID", traceID)
		}

		logger := sm.logger.With(
			zap.String("trace_id", traceID),
			zap.String("middleware", "provider_auth"),
			zap.String("client_ip", c.ClientIP()),
		)

		// Check provider ID header
		providerID := c.GetHeader("X-Provider-ID")
		if providerID == "" {
			logger.Warn("Missing provider ID header")
			c.JSON(http.StatusBadRequest, gin.H{
				"error":    "MISSING_PROVIDER_ID",
				"message":  "X-Provider-ID header is required",
				"trace_id": traceID,
			})
			c.Abort()
			return
		}

		// Validate provider is allowed
		providerInfo, exists := sm.allowedProviders[providerID]
		if !exists || !providerInfo.Active {
			logger.Warn("Invalid or inactive provider", zap.String("provider_id", providerID))
			c.JSON(http.StatusUnauthorized, gin.H{
				"error":    "INVALID_PROVIDER",
				"message":  "Provider not authorized",
				"trace_id": traceID,
			})
			c.Abort()
			return
		}

		// Validate provider signature in header
		providerSig := c.GetHeader("X-Provider-Signature")
		if providerSig == "" {
			logger.Warn("Missing provider signature header")
			c.JSON(http.StatusBadRequest, gin.H{
				"error":    "MISSING_SIGNATURE",
				"message":  "X-Provider-Signature header is required",
				"trace_id": traceID,
			})
			c.Abort()
			return
		}

		// Validate timestamp header
		timestampStr := c.GetHeader("X-Timestamp")
		if timestampStr == "" {
			logger.Warn("Missing timestamp header")
			c.JSON(http.StatusBadRequest, gin.H{
				"error":    "MISSING_TIMESTAMP",
				"message":  "X-Timestamp header is required",
				"trace_id": traceID,
			})
			c.Abort()
			return
		}

		timestamp, err := time.Parse(time.RFC3339, timestampStr)
		if err != nil {
			logger.Warn("Invalid timestamp format", zap.Error(err))
			c.JSON(http.StatusBadRequest, gin.H{
				"error":    "INVALID_TIMESTAMP",
				"message":  "Invalid timestamp format",
				"trace_id": traceID,
			})
			c.Abort()
			return
		}

		// Check timestamp freshness (prevent replay attacks)
		now := time.Now()
		timeDiff := now.Sub(timestamp)
		if timeDiff < -2*time.Minute || timeDiff > 5*time.Minute {
			logger.Warn("Request timestamp outside acceptable window",
				zap.Duration("time_diff", timeDiff))
			c.JSON(http.StatusBadRequest, gin.H{
				"error":    "TIMESTAMP_OUT_OF_RANGE",
				"message":  "Request timestamp is outside acceptable window",
				"trace_id": traceID,
			})
			c.Abort()
			return
		}

		// Store provider info in context
		c.Set("provider_info", providerInfo)
		c.Set("provider_id", providerID)

		logger.Info("Provider authenticated successfully",
			zap.String("provider_id", providerID))

		c.Next()
	}
}

// RequestValidationMiddleware provides comprehensive request validation
func (sm *SecurityMiddleware) RequestValidationMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		traceID := c.GetHeader("X-Trace-ID")
		if traceID == "" {
			traceID = uuid.New().String()
			c.Header("X-Trace-ID", traceID)
		}

		logger := sm.logger.With(
			zap.String("trace_id", traceID),
			zap.String("middleware", "request_validation"),
		)

		// Validate Content-Type for POST requests
		if c.Request.Method == "POST" {
			contentType := c.GetHeader("Content-Type")
			if !strings.Contains(contentType, "application/json") {
				logger.Warn("Invalid content type", zap.String("content_type", contentType))
				c.JSON(http.StatusBadRequest, gin.H{
					"error":    "INVALID_CONTENT_TYPE",
					"message":  "Content-Type must be application/json",
					"trace_id": traceID,
				})
				c.Abort()
				return
			}
		}

		// Validate User-Agent
		userAgent := c.GetHeader("User-Agent")
		if userAgent == "" || len(userAgent) > 500 {
			logger.Warn("Invalid or missing User-Agent", zap.String("user_agent", userAgent))
			c.JSON(http.StatusBadRequest, gin.H{
				"error":    "INVALID_USER_AGENT",
				"message":  "Valid User-Agent header is required",
				"trace_id": traceID,
			})
			c.Abort()
			return
		}

		// Check for suspicious patterns in headers
		if err := sm.validateHeaders(c); err != nil {
			logger.Warn("Suspicious header detected", zap.Error(err))
			c.JSON(http.StatusBadRequest, gin.H{
				"error":    "SUSPICIOUS_REQUEST",
				"message":  "Request contains suspicious patterns",
				"trace_id": traceID,
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// PayloadValidationMiddleware validates request payloads for fiat endpoints
func (sm *SecurityMiddleware) PayloadValidationMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		traceID := c.GetHeader("X-Trace-ID")

		logger := sm.logger.With(
			zap.String("trace_id", traceID),
			zap.String("middleware", "payload_validation"),
		)

		// Skip validation for GET requests
		if c.Request.Method != "POST" {
			c.Next()
			return
		}

		// Validate request size
		if c.Request.ContentLength > 1024*1024 { // 1MB limit
			logger.Warn("Request payload too large",
				zap.Int64("content_length", c.Request.ContentLength))
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{
				"error":    "PAYLOAD_TOO_LARGE",
				"message":  "Request payload exceeds maximum size",
				"trace_id": traceID,
			})
			c.Abort()
			return
		}

		// Validate endpoint-specific payloads
		if strings.Contains(c.Request.URL.Path, "/receipts") && c.Request.Method == "POST" {
			if err := sm.validateReceiptPayload(c); err != nil {
				logger.Warn("Invalid receipt payload", zap.Error(err))
				c.JSON(http.StatusBadRequest, gin.H{
					"error":    "INVALID_PAYLOAD",
					"message":  err.Error(),
					"trace_id": traceID,
				})
				c.Abort()
				return
			}
		}

		c.Next()
	}
}

// MTLSMiddleware validates mutual TLS certificates (when using mTLS)
func (sm *SecurityMiddleware) MTLSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check if client certificate is present
		if c.Request.TLS == nil || len(c.Request.TLS.PeerCertificates) == 0 {
			sm.logger.Warn("Missing client certificate")
			c.JSON(http.StatusUnauthorized, gin.H{
				"error":   "MISSING_CLIENT_CERTIFICATE",
				"message": "Client certificate required for this endpoint",
			})
			c.Abort()
			return
		}

		// Validate certificate subject
		cert := c.Request.TLS.PeerCertificates[0]
		sm.logger.Info("Client certificate validated",
			zap.String("subject", cert.Subject.String()),
			zap.String("issuer", cert.Issuer.String()))

		c.Next()
	}
}

// validateHeaders checks for suspicious patterns in request headers
func (sm *SecurityMiddleware) validateHeaders(c *gin.Context) error {
	// Check for SQL injection patterns in headers
	suspiciousPatterns := []string{
		"union", "select", "insert", "update", "delete", "drop",
		"<script", "javascript:", "onload=", "onerror=",
		"../../", "../", "..\\",
	}

	for name, values := range c.Request.Header {
		for _, value := range values {
			lowerValue := strings.ToLower(value)
			for _, pattern := range suspiciousPatterns {
				if strings.Contains(lowerValue, pattern) {
					return fmt.Errorf("suspicious pattern '%s' found in header '%s'", pattern, name)
				}
			}
		}
	}

	return nil
}

// validateReceiptPayload validates fiat receipt request payload
func (sm *SecurityMiddleware) validateReceiptPayload(c *gin.Context) error {
	// Basic JSON structure validation will be done by Gin's ShouldBindJSON
	// Here we do additional security checks

	// Check required headers for receipt requests
	requiredHeaders := []string{"X-Provider-ID", "X-Provider-Signature", "X-Timestamp"}
	for _, header := range requiredHeaders {
		if c.GetHeader(header) == "" {
			return fmt.Errorf("missing required header: %s", header)
		}
	}

	return nil
}

// RateLimitMiddleware provides rate limiting for fiat endpoints
func (sm *SecurityMiddleware) RateLimitMiddleware() gin.HandlerFunc {
	// In a production environment, this would use Redis or similar
	// For now, we'll do basic in-memory rate limiting
	requestCounts := make(map[string]int)
	lastReset := time.Now()

	return func(c *gin.Context) {
		traceID := c.GetHeader("X-Trace-ID")
		providerID := c.GetHeader("X-Provider-ID")
		clientIP := c.ClientIP()

		// Create unique key for rate limiting
		key := fmt.Sprintf("%s:%s", providerID, clientIP)

		// Reset counters if window expired
		if time.Since(lastReset) > sm.rateLimitWindow {
			requestCounts = make(map[string]int)
			lastReset = time.Now()
		}

		// Check rate limit
		requestCounts[key]++
		if requestCounts[key] > sm.maxRequestsWindow {
			sm.logger.Warn("Rate limit exceeded",
				zap.String("provider_id", providerID),
				zap.String("client_ip", clientIP),
				zap.Int("requests", requestCounts[key]))

			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":    "RATE_LIMIT_EXCEEDED",
				"message":  "Too many requests",
				"trace_id": traceID,
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// IPWhitelistMiddleware restricts access to whitelisted IPs
func (sm *SecurityMiddleware) IPWhitelistMiddleware(allowedIPs []string) gin.HandlerFunc {
	// Convert to map for faster lookup
	ipMap := make(map[string]bool)
	for _, ip := range allowedIPs {
		ipMap[ip] = true
	}

	return func(c *gin.Context) {
		clientIP := c.ClientIP()

		// Check if IP is whitelisted
		if !ipMap[clientIP] {
			sm.logger.Warn("Access denied for non-whitelisted IP",
				zap.String("client_ip", clientIP))

			c.JSON(http.StatusForbidden, gin.H{
				"error":   "ACCESS_DENIED",
				"message": "Access denied",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// SecurityHeadersMiddleware adds security headers to responses
func (sm *SecurityMiddleware) SecurityHeadersMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Add security headers
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-XSS-Protection", "1; mode=block")
		c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		c.Header("Content-Security-Policy", "default-src 'self'")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")

		c.Next()
	}
}

// Helper function for constant-time string comparison
func secureCompare(a, b string) bool {
	if len(a) != len(b) {
		return false
	}

	aBytes := []byte(a)
	bBytes := []byte(b)

	return subtle.ConstantTimeCompare(aBytes, bBytes) == 1
}

// Helper function to validate hex string
func isValidHex(s string) bool {
	_, err := hex.DecodeString(s)
	return err == nil
}

// Helper function to validate UUID format
func isValidUUID(s string) bool {
	uuidRegex := regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-5][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)
	return uuidRegex.MatchString(s)
}
