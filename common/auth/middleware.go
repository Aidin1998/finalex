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
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
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

func Middleware(log *slog.Logger, cfg AuthorizationConfig) echo.MiddlewareFunc {
	issuerUrl, err := url.Parse(cfg.Issuer)
	if err != nil {
		panic(fmt.Sprintf("failed to parse issuer URL: %v", err))
	}

	jwksProvider := jwks.NewCachingProvider(issuerUrl, time.Minute)
	customClaims := func() validator.CustomClaims {
		return &CustomClaims{}
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		// Set up the validator.
		jwtValidator, err := validator.New(
			jwksProvider.KeyFunc,
			validator.RS256,
			cfg.Issuer,
			cfg.Audience,
			// validator.WithCustomClaims(customClaims),
			validator.WithAllowedClockSkew(30*time.Second),
			validator.WithCustomClaims(customClaims),
		)
		if err != nil {
			panic(fmt.Sprintf("failed to set up the validator: %v", err))
		}

		errorHandler := func(w http.ResponseWriter, r *http.Request, err error) {
			log.ErrorContext(r.Context(), "encountered error while validating JWT: %v", err)
		}

		middleware := jwtmiddleware.New(
			jwtValidator.ValidateToken,
			jwtmiddleware.WithErrorHandler(errorHandler),
			//jwtmiddleware.WithTokenExtractor(func (r *http.Request) (string, error) {
			//return r.CookiesNamed("access_token")[0].String(), nil
			//}),
		)

		return func(ctx echo.Context) error {
			encounteredError := true
			var handler http.HandlerFunc = func(w http.ResponseWriter, r *http.Request) {
				encounteredError = false
				claims, _ := r.Context().Value(jwtmiddleware.ContextKey{}).(*validator.ValidatedClaims)
				customClaims, ok := claims.CustomClaims.(*CustomClaims)
				if ok {
					ctx.Set("userID", customClaims.UserID)
					ctx.Set("onboarded", customClaims.Onboarded)
				}

				ctx.SetRequest(r)
				next(ctx)
			}

			middleware.CheckJWT(handler).ServeHTTP(ctx.Response(), ctx.Request())

			if encounteredError {
				ctx.JSON(
					http.StatusUnauthorized,
					map[string]string{"message": "JWT is invalid."},
				)
			}
			return nil
		}
	}
}

// RBAC enforcement: requireRole checks if user has required role
func RequireRole(requiredRole string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(ctx echo.Context) error {
			userRole, ok := ctx.Get("role").(string)
			if !ok || userRole == "" {
				return ctx.JSON(http.StatusForbidden, map[string]string{"message": "role not found"})
			}
			if userRole != requiredRole && userRole != "admin" {
				return ctx.JSON(http.StatusForbidden, map[string]string{"message": "insufficient role"})
			}
			return next(ctx)
		}
	}
}

// TOTP (MFA) enforcement: require TOTP if enabled
func RequireTOTP(next echo.HandlerFunc) echo.HandlerFunc {
	return func(ctx echo.Context) error {
		mfaEnabled, _ := ctx.Get("mfa_enabled").(bool)
		if mfaEnabled {
			code := ctx.Request().Header.Get("X-TOTP-Code")
			if code == "" {
				return ctx.JSON(http.StatusUnauthorized, map[string]string{"message": "TOTP code required"})
			}
			// TODO: Validate TOTP code using user's TOTPSecret
			// If invalid, return unauthorized
		}
		return next(ctx)
	}
}
