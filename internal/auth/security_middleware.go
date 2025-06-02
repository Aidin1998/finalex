package auth

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// DefaultSecurityMiddlewareConfig returns secure default configuration
func DefaultSecurityMiddlewareConfig() *SecurityMiddlewareConfig {
	return &SecurityMiddlewareConfig{
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
	}
}

// SecurityMiddleware creates enhanced security middleware that works with JWT validation
func SecurityMiddleware(logger *zap.Logger, authService AuthService, config *SecurityMiddlewareConfig) gin.HandlerFunc {
	if config == nil {
		config = DefaultSecurityMiddlewareConfig()
	}

	return func(c *gin.Context) {
		startTime := time.Now()

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
		c.Set("user_id", userID)
		c.Set("user_role", userRole)
		c.Set("user_email", claims.Email)
		c.Set("user_permissions", claims.Permissions)
		c.Set("session_id", claims.SessionID.String())
		c.Set("token_type", claims.TokenType)
		c.Set("client_ip", clientIP)
		c.Set("user_agent", userAgent)

		// Apply user-based rate limiting if enabled
		if config.EnableUserBasedRateLimit {
			// This would integrate with your rate limiter implementation
			// rateLimitKey := fmt.Sprintf("user_rate_limit:%s", userID)
			// Add rate limiting logic here
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

		// Check session validity if required
		if config.RequireActiveSession && claims.SessionID.String() != "" {
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
				return
			}

			// Check for session timeout warning
			if config.SessionTimeoutWarning > 0 {
				timeUntilExpiry := time.Until(session.ExpiresAt)
				if timeUntilExpiry > 0 && timeUntilExpiry < config.SessionTimeoutWarning {
					c.Header("X-Session-Warning", fmt.Sprintf("Session expires in %v", timeUntilExpiry))
				}
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

		// Set security headers
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-XSS-Protection", "1; mode=block")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")

		c.Next()
	}
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
