package auth

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	jwtmiddleware "github.com/auth0/go-jwt-middleware/v2"
	"github.com/auth0/go-jwt-middleware/v2/jwks"
	"github.com/auth0/go-jwt-middleware/v2/validator"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type CustomClaims struct {
	Onboarded bool      `json:"https://litebit.tech/onboarded"`
	UserID    uuid.UUID `json:"https://litebit.tech/userid"`
}

func (c *CustomClaims) Validate(ctx context.Context) error {
	return nil
}

type AuthorizationConfig struct {
	Audience []string
	Issuer   string
}

func Middleware(log *slog.Logger, cfg AuthorizationConfig) gin.HandlerFunc {
	issuerUrl, err := url.Parse(cfg.Issuer)
	if err != nil {
		panic(fmt.Sprintf("failed to parse issuer URL: %v", err))
	}

	jwksProvider := jwks.NewCachingProvider(issuerUrl, time.Minute)
	customClaims := func() validator.CustomClaims {
		return &CustomClaims{}
	}

	jwtValidator, err := validator.New(
		jwksProvider.KeyFunc,
		validator.RS256,
		cfg.Issuer,
		cfg.Audience,
		validator.WithAllowedClockSkew(30*time.Second),
		validator.WithCustomClaims(customClaims),
	)
	if err != nil {
		panic(fmt.Sprintf("failed to set up the validator: %v", err))
	}

	return func(c *gin.Context) {
		errorHandler := func(w http.ResponseWriter, r *http.Request, err error) {
			log.ErrorContext(r.Context(), "encountered error while validating JWT: %v", err)
		}

		middleware := jwtmiddleware.New(
			jwtValidator.ValidateToken,
			jwtmiddleware.WithErrorHandler(errorHandler),
		)

		encounteredError := true
		var handler http.HandlerFunc = func(w http.ResponseWriter, r *http.Request) {
			encounteredError = false
			claims, _ := r.Context().Value(jwtmiddleware.ContextKey{}).(*validator.ValidatedClaims)
			customClaims, ok := claims.CustomClaims.(*CustomClaims)
			if ok {
				c.Set("userID", customClaims.UserID)
				c.Set("onboarded", customClaims.Onboarded)
			}
			c.Request = r
			c.Next()
		}

		middleware.CheckJWT(handler).ServeHTTP(c.Writer, c.Request)

		if encounteredError {
			c.AbortWithStatusJSON(
				http.StatusUnauthorized,
				gin.H{"message": "JWT is invalid."},
			)
		}
	}
}

// RBAC enforcement: requireRole checks if user has required role
func RequireRole(requiredRole string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userRole, ok := c.Get("role")
		roleStr, _ := userRole.(string)
		if !ok || roleStr == "" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"message": "role not found"})
			return
		}
		if roleStr != requiredRole && roleStr != "admin" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"message": "insufficient role"})
			return
		}
		c.Next()
	}
}

// TOTP (MFA) enforcement: require TOTP if enabled
func RequireTOTP() gin.HandlerFunc {
	return func(c *gin.Context) {
		mfaEnabled, _ := c.Get("mfa_enabled")
		enabled, _ := mfaEnabled.(bool)
		if enabled {
			code := c.Request.Header.Get("X-TOTP-Code")
			if code == "" {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"message": "TOTP code required"})
				return
			}
			// TODO: Validate TOTP code using user's TOTPSecret
			// If invalid, return unauthorized
		}
		c.Next()
	}
}
