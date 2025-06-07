// Package middleware provides HTTP middleware for wallet API endpoints
package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/time/rate"

	"your-project/internal/auth"
	"your-project/pkg/logger"
)

// AuthMiddleware provides JWT authentication for wallet endpoints
func AuthMiddleware(authService auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authorization header required"})
			c.Abort()
			return
		}

		// Extract token from "Bearer <token>"
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization header format"})
			c.Abort()
			return
		}

		token := parts[1]
		claims, err := authService.ValidateToken(token)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			c.Abort()
			return
		}

		// Set user ID in context
		userID, err := uuid.Parse(claims.UserID)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid user ID in token"})
			c.Abort()
			return
		}

		c.Set("user_id", userID)
		c.Set("user_claims", claims)
		c.Next()
	}
}

// RateLimitMiddleware provides rate limiting for wallet endpoints
func RateLimitMiddleware(rps int, burst int) gin.HandlerFunc {
	limiter := rate.NewLimiter(rate.Limit(rps), burst)

	return func(c *gin.Context) {
		if !limiter.Allow() {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":       "rate limit exceeded",
				"retry_after": time.Second.Seconds(),
			})
			c.Abort()
			return
		}
		c.Next()
	}
}

// UserRateLimitMiddleware provides per-user rate limiting
type UserRateLimitMiddleware struct {
	limiters map[string]*rate.Limiter
	rps      int
	burst    int
}

// NewUserRateLimitMiddleware creates per-user rate limiter
func NewUserRateLimitMiddleware(rps int, burst int) *UserRateLimitMiddleware {
	return &UserRateLimitMiddleware{
		limiters: make(map[string]*rate.Limiter),
		rps:      rps,
		burst:    burst,
	}
}

// Middleware returns the rate limiting middleware function
func (m *UserRateLimitMiddleware) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, exists := c.Get("user_id")
		if !exists {
			c.Next()
			return
		}

		uid := userID.(uuid.UUID).String()
		limiter, exists := m.limiters[uid]
		if !exists {
			limiter = rate.NewLimiter(rate.Limit(m.rps), m.burst)
			m.limiters[uid] = limiter
		}

		if !limiter.Allow() {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":       "user rate limit exceeded",
				"retry_after": time.Second.Seconds(),
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// LoggingMiddleware logs wallet API requests
func LoggingMiddleware(log logger.Logger) gin.HandlerFunc {
	return gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		log.Info("wallet_api_request",
			"method", param.Method,
			"path", param.Path,
			"status", param.StatusCode,
			"latency", param.Latency,
			"client_ip", param.ClientIP,
			"user_agent", param.Request.UserAgent(),
		)
		return ""
	})
}

// ValidationMiddleware validates common request parameters
func ValidationMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Validate content type for POST/PUT requests
		if c.Request.Method == "POST" || c.Request.Method == "PUT" {
			contentType := c.GetHeader("Content-Type")
			if !strings.Contains(contentType, "application/json") {
				c.JSON(http.StatusBadRequest, gin.H{
					"error": "content-type must be application/json",
				})
				c.Abort()
				return
			}
		}

		c.Next()
	}
}

// SecurityHeadersMiddleware adds security headers
func SecurityHeadersMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-XSS-Protection", "1; mode=block")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("Content-Security-Policy", "default-src 'self'")
		c.Next()
	}
}

// TwoFactorMiddleware validates 2FA for sensitive operations
func TwoFactorMiddleware(authService auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Only apply to withdrawal endpoints
		if !strings.Contains(c.Request.URL.Path, "withdrawal") {
			c.Next()
			return
		}

		userID, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
			c.Abort()
			return
		}

		uid := userID.(uuid.UUID)

		// Extract 2FA token from request
		var twoFactorToken string
		if c.Request.Method == "POST" {
			var requestBody struct {
				TwoFactorToken string `json:"two_factor_token"`
			}
			if err := c.ShouldBindJSON(&requestBody); err == nil {
				twoFactorToken = requestBody.TwoFactorToken
			}
		} else {
			twoFactorToken = c.GetHeader("X-2FA-Token")
		}

		if twoFactorToken == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "two-factor token required"})
			c.Abort()
			return
		}

		// Validate 2FA token
		if err := authService.ValidateTwoFactor(uid, twoFactorToken); err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid two-factor token"})
			c.Abort()
			return
		}

		c.Next()
	}
}

// AdminAuthMiddleware validates admin permissions
func AdminAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, exists := c.Get("user_claims")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
			c.Abort()
			return
		}

		userClaims, ok := claims.(*auth.Claims)
		if !ok || !userClaims.IsAdmin {
			c.JSON(http.StatusForbidden, gin.H{"error": "admin access required"})
			c.Abort()
			return
		}

		c.Next()
	}
}

// CORSMiddleware handles cross-origin requests
func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")

		// Allow specific origins in production
		allowedOrigins := []string{
			"https://app.yourexchange.com",
			"https://admin.yourexchange.com",
		}

		// In development, allow localhost
		if gin.Mode() == gin.DebugMode {
			allowedOrigins = append(allowedOrigins,
				"http://localhost:3000",
				"http://localhost:8080",
			)
		}

		for _, allowed := range allowedOrigins {
			if origin == allowed {
				c.Header("Access-Control-Allow-Origin", origin)
				break
			}
		}

		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization, X-2FA-Token")
		c.Header("Access-Control-Max-Age", "86400")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
