package auth

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Note: Using TokenClaims from auth service instead of local Claims struct

// ModernSecurityConfig represents the modern security middleware configuration
type ModernSecurityConfig struct {
	BlockHighRiskTokens        bool          `json:"block_high_risk_tokens"`
	LogSecurityFlags           bool          `json:"log_security_flags"`
	AlertOnElevatedPrivileges  bool          `json:"alert_on_elevated_privileges"`
	RequireActiveSession       bool          `json:"require_active_session"`
	SessionTimeoutWarning      time.Duration `json:"session_timeout_warning"`
	EnableUserBasedRateLimit   bool          `json:"enable_user_based_rate_limit"`
	UserRateLimit              int           `json:"user_rate_limit"`
	UserRateWindow             time.Duration `json:"user_rate_window"`
	EnableDetailedAuditLogging bool          `json:"enable_detailed_audit_logging"`
	LogSuccessfulAccess        bool          `json:"log_successful_access"`
	LogSuspiciousActivity      bool          `json:"log_suspicious_activity"`
	CSPPolicy                  string        `json:"csp_policy"`
	HSTSMaxAge                 int           `json:"hsts_max_age"`
}

// DefaultModernSecurityConfig returns secure default configuration
func DefaultModernSecurityConfig() *ModernSecurityConfig {
	return &ModernSecurityConfig{
		BlockHighRiskTokens:        true,
		LogSecurityFlags:           true,
		AlertOnElevatedPrivileges:  true,
		RequireActiveSession:       true,
		SessionTimeoutWarning:      15 * time.Minute,
		EnableUserBasedRateLimit:   true,
		UserRateLimit:              1000,
		UserRateWindow:             time.Hour,
		EnableDetailedAuditLogging: true,
		LogSuccessfulAccess:        false, // Disable to reduce log volume
		LogSuspiciousActivity:      true,
		CSPPolicy:                  "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'",
		HSTSMaxAge:                 31536000, // 1 year
	}
}

// SecurityMiddleware creates modern security middleware with enhanced security headers and validation
func SecurityMiddleware(logger *zap.Logger, authService AuthService, config *ModernSecurityConfig) gin.HandlerFunc {
	if config == nil {
		config = DefaultModernSecurityConfig()
	}

	return func(c *gin.Context) {
		startTime := time.Now()

		// Apply comprehensive security headers
		applySecurityHeaders(c, config)

		// Skip security checks for non-authenticated endpoints
		token := c.GetHeader("Authorization")
		if token == "" {
			c.Next()
			return
		}

		// Extract client information
		clientIP := c.ClientIP()
		userAgent := c.GetHeader("User-Agent")
		endpoint := c.Request.URL.Path
		method := c.Request.Method

		// Set client IP in context for downstream components
		ctx := context.WithValue(c.Request.Context(), "client_ip", clientIP)
		c.Request = c.Request.WithContext(ctx)

		// Validate token with enhanced security checks
		claims, err := authService.ValidateToken(ctx, token)
		if err != nil {
			// Log authentication failure
			if config.EnableDetailedAuditLogging {
				logger.Warn("Security middleware: Authentication failed",
					zap.String("client_ip", clientIP),
					zap.String("user_agent", userAgent),
					zap.String("endpoint", endpoint),
					zap.String("method", method),
					zap.Error(err),
					zap.Duration("processing_time", time.Since(startTime)))
			}

			c.JSON(401, gin.H{
				"error": "Authentication failed",
				"code":  "UNAUTHORIZED",
			})
			c.Abort()
			return
		}

		// Extract user information
		userID := claims.UserID.String()
		userRole := claims.Role

		// Set comprehensive security context
		setSecurityContext(c, claims, clientIP, userAgent)

		// Apply user-based rate limiting if enabled
		if config.EnableUserBasedRateLimit {
			// Rate limiting implementation would go here
			// This would integrate with a distributed rate limiter like Redis
		}

		// Check for elevated privileges
		if config.AlertOnElevatedPrivileges && isElevatedRole(userRole) {
			logger.Warn("Elevated privilege access detected",
				zap.String("user_id", userID),
				zap.String("role", userRole),
				zap.String("client_ip", clientIP),
				zap.String("endpoint", endpoint),
				zap.String("method", method),
				zap.String("user_agent", userAgent))
		}

		// Validate session if required
		if config.RequireActiveSession {
			if err := validateSession(ctx, authService, claims, config, logger, userID, clientIP, c); err != nil {
				return // Session validation failed, response already sent
			}
		}

		// Log successful access if enabled
		if config.LogSuccessfulAccess && config.EnableDetailedAuditLogging {
			logger.Info("Security middleware: Access granted",
				zap.String("user_id", userID),
				zap.String("client_ip", clientIP),
				zap.String("endpoint", endpoint),
				zap.String("method", method),
				zap.String("role", userRole),
				zap.Duration("processing_time", time.Since(startTime)))
		}

		c.Next()
	}
}

// applySecurityHeaders applies comprehensive security headers
func applySecurityHeaders(c *gin.Context, config *ModernSecurityConfig) {
	// Basic security headers
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("X-Frame-Options", "DENY")
	c.Header("X-XSS-Protection", "1; mode=block")
	c.Header("Referrer-Policy", "strict-origin-when-cross-origin")

	// Content Security Policy
	if config.CSPPolicy != "" {
		c.Header("Content-Security-Policy", config.CSPPolicy)
	}

	// HSTS for HTTPS
	if c.Request.TLS != nil && config.HSTSMaxAge > 0 {
		c.Header("Strict-Transport-Security", fmt.Sprintf("max-age=%d; includeSubDomains", config.HSTSMaxAge))
	}

	// Remove server information
	c.Header("Server", "")
}

// setSecurityContext sets comprehensive security context in Gin context
func setSecurityContext(c *gin.Context, claims *TokenClaims, clientIP, userAgent string) {
	c.Set("user_id", claims.UserID.String())
	c.Set("user_role", claims.Role)
	c.Set("user_email", claims.Email)
	c.Set("user_permissions", claims.Permissions)
	c.Set("session_id", claims.SessionID.String())
	c.Set("token_type", claims.TokenType)
	c.Set("client_ip", clientIP)
	c.Set("user_agent", userAgent)
}

// validateSession validates user session
func validateSession(ctx context.Context, authService AuthService, claims *TokenClaims, config *ModernSecurityConfig, logger *zap.Logger, userID, clientIP string, c *gin.Context) error {
	if claims.SessionID == uuid.Nil {
		return nil // No session to validate
	}

	session, err := authService.ValidateSession(ctx, claims.SessionID)
	if err != nil || !session.IsActive {
		logger.Warn("Invalid or inactive session detected",
			zap.String("user_id", userID),
			zap.String("session_id", claims.SessionID.String()),
			zap.String("client_ip", clientIP),
			zap.Error(err))

		c.JSON(401, gin.H{
			"error": "Session invalid or expired",
			"code":  "SESSION_INVALID",
		})
		c.Abort()
		return fmt.Errorf("session validation failed")
	}

	// Check for session timeout warning
	if config.SessionTimeoutWarning > 0 {
		timeUntilExpiry := time.Until(session.ExpiresAt)
		if timeUntilExpiry > 0 && timeUntilExpiry < config.SessionTimeoutWarning {
			c.Header("X-Session-Warning", fmt.Sprintf("Session expires in %v", timeUntilExpiry))
		}
	}

	return nil
}

// isElevatedRole checks if a role has elevated privileges
func isElevatedRole(role string) bool {
	elevatedRoles := []string{"admin", "super_admin", "system", "root"}
	roleLower := strings.ToLower(role)

	for _, elevated := range elevatedRoles {
		if roleLower == elevated {
			return true
		}
	}
	return false
}

// RequireRole creates middleware that requires specific role
func RequireRole(requiredRole string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userRole, exists := c.Get("user_role")
		if !exists {
			c.JSON(403, gin.H{
				"error": "Role information not available",
				"code":  "ROLE_MISSING",
			})
			c.Abort()
			return
		}

		role, ok := userRole.(string)
		if !ok || !hasRole(role, requiredRole) {
			c.JSON(403, gin.H{
				"error": fmt.Sprintf("Required role: %s", requiredRole),
				"code":  "INSUFFICIENT_PRIVILEGES",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// RequirePermission creates middleware that requires specific permission
func RequirePermission(requiredPermission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userPermissions, exists := c.Get("user_permissions")
		if !exists {
			c.JSON(403, gin.H{
				"error": "Permission information not available",
				"code":  "PERMISSIONS_MISSING",
			})
			c.Abort()
			return
		}

		permissions, ok := userPermissions.([]string)
		if !ok || !hasPermission(permissions, requiredPermission) {
			c.JSON(403, gin.H{
				"error": fmt.Sprintf("Required permission: %s", requiredPermission),
				"code":  "INSUFFICIENT_PERMISSIONS",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// hasRole checks if user has the required role or higher
func hasRole(userRole, requiredRole string) bool {
	// Define role hierarchy (higher index = higher privilege)
	roleHierarchy := []string{"user", "trader", "moderator", "admin", "super_admin"}

	userLevel := -1
	requiredLevel := -1

	for i, role := range roleHierarchy {
		if strings.EqualFold(role, userRole) {
			userLevel = i
		}
		if strings.EqualFold(role, requiredRole) {
			requiredLevel = i
		}
	}

	return userLevel >= requiredLevel && requiredLevel >= 0
}

// hasPermission checks if user has the required permission
func hasPermission(userPermissions []string, requiredPermission string) bool {
	for _, permission := range userPermissions {
		if strings.EqualFold(permission, requiredPermission) {
			return true
		}
		// Check for wildcard permissions
		if strings.HasSuffix(permission, "*") {
			prefix := strings.TrimSuffix(permission, "*")
			if strings.HasPrefix(requiredPermission, prefix) {
				return true
			}
		}
	}
	return false
}

// AuditMiddleware creates middleware for comprehensive audit logging
func AuditMiddleware(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		startTime := time.Now()

		// Capture request details
		clientIP := c.ClientIP()
		userAgent := c.GetHeader("User-Agent")
		endpoint := c.Request.URL.Path
		method := c.Request.Method

		// Get user information from context
		userID, _ := c.Get("user_id")
		userRole, _ := c.Get("user_role")

		// Process request
		c.Next()

		// Log audit information
		duration := time.Since(startTime)
		statusCode := c.Writer.Status()

		auditLog := logger.With(
			zap.String("client_ip", clientIP),
			zap.String("user_agent", userAgent),
			zap.String("endpoint", endpoint),
			zap.String("method", method),
			zap.Int("status_code", statusCode),
			zap.Duration("response_time", duration),
		)

		if userID != nil {
			if uid, ok := userID.(string); ok {
				auditLog = auditLog.With(zap.String("user_id", uid))
			}
		}

		if userRole != nil {
			if role, ok := userRole.(string); ok {
				auditLog = auditLog.With(zap.String("user_role", role))
			}
		}

		// Determine log level based on status code
		if statusCode >= 400 {
			auditLog.Warn("Request completed with error")
		} else {
			auditLog.Info("Request completed successfully")
		}
	}
}
