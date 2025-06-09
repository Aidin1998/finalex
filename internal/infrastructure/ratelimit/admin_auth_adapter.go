// admin_auth_adapter.go: Admin authentication adapter for rate limiting
package ratelimit

import (
	"context"
	"strings"

	"go.uber.org/zap"
)

// UserAuthService interface for admin validation (to avoid circular imports)
type UserAuthService interface {
	ValidateToken(ctx context.Context, tokenString string) (TokenClaims, error)
}

// TokenClaims represents the simplified token claims needed for admin validation
type TokenClaims struct {
	UserID      string   `json:"user_id"`
	Email       string   `json:"email"`
	Role        string   `json:"role"`
	Roles       []string `json:"roles"`
	Permissions []string `json:"permissions"`
}

// UserAuthAdminValidator implements AdminAuthValidator using the userauth service
type UserAuthAdminValidator struct {
	userAuthService UserAuthService
	logger          *zap.Logger
}

// NewUserAuthAdminValidator creates a new admin validator using userauth service
func NewUserAuthAdminValidator(userAuthService UserAuthService, logger *zap.Logger) *UserAuthAdminValidator {
	return &UserAuthAdminValidator{
		userAuthService: userAuthService,
		logger:          logger,
	}
}

// ValidateAdminToken validates if a token belongs to an admin user
func (v *UserAuthAdminValidator) ValidateAdminToken(ctx context.Context, token string) (bool, error) {
	if v.userAuthService == nil {
		v.logger.Warn("UserAuth service not available for admin validation")
		return false, nil // Fail safe - don't allow bypass without proper auth service
	}

	// Validate the token
	claims, err := v.userAuthService.ValidateToken(ctx, token)
	if err != nil {
		v.logger.Warn("Token validation failed for admin request", zap.Error(err))
		return false, err
	}

	// Check if user has admin role
	if hasAdminRole(claims.Role, claims.Roles) {
		v.logger.Info("Admin access granted for rate limit bypass",
			zap.String("user_id", claims.UserID),
			zap.String("email", claims.Email),
			zap.String("role", claims.Role))
		return true, nil
	}

	v.logger.Warn("Non-admin user attempted admin bypass",
		zap.String("user_id", claims.UserID),
		zap.String("email", claims.Email),
		zap.String("role", claims.Role))
	return false, nil
}

// hasAdminRole checks if the user has admin privileges
func hasAdminRole(role string, roles []string) bool {
	// Check main role
	if isAdminRole(role) {
		return true
	}

	// Check roles array
	for _, r := range roles {
		if isAdminRole(r) {
			return true
		}
	}

	return false
}

// isAdminRole checks if a single role is an admin role
func isAdminRole(role string) bool {
	role = strings.ToLower(strings.TrimSpace(role))
	return role == "admin" || role == "super_admin" || role == "administrator" || role == "root"
}

// SetupAdminAuth configures the global admin authentication validator
func SetupAdminAuth(userAuthService UserAuthService, logger *zap.Logger) {
	validator := NewUserAuthAdminValidator(userAuthService, logger)
	SetAdminAuthValidator(validator)
}
