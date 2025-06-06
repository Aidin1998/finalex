// Package adapters provides adapter implementations for service contracts
package adapters

import (
	"context"
	"fmt"

	"github.com/Aidin1998/finalex/internal/integration/contracts"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// UserAuthServiceAdapter implements contracts.UserAuthServiceContract
// by adapting the contracts.UserAuthServiceContract interface
type UserAuthServiceAdapter struct {
	service contracts.UserAuthServiceContract
	logger  *zap.Logger
}

// NewUserAuthServiceAdapter creates a new userauth service adapter
func NewUserAuthServiceAdapter(service contracts.UserAuthServiceContract, logger *zap.Logger) *UserAuthServiceAdapter {
	return &UserAuthServiceAdapter{
		service: service,
		logger:  logger,
	}
}

// ValidateToken validates authentication token and returns claims
func (a *UserAuthServiceAdapter) ValidateToken(ctx context.Context, token string) (*contracts.AuthClaims, error) {
	a.logger.Debug("Validating token", zap.String("token", "***"))

	// Call userauth service to validate token
	claims, err := a.service.ValidateToken(ctx, token)
	if err != nil {
		a.logger.Error("Token validation failed", zap.Error(err))
		return nil, fmt.Errorf("token validation failed: %w", err)
	}

	// Convert userauth claims to contract claims
	return &contracts.AuthClaims{
		UserID:      claims.UserID,
		Email:       claims.Email,
		Role:        claims.Role,
		Permissions: claims.Permissions,
		KYCLevel:    claims.KYCLevel,
		SessionID:   claims.SessionID,
		ExpiresAt:   claims.ExpiresAt,
	}, nil
}

// ValidateAPIKey validates API key and returns claims
func (a *UserAuthServiceAdapter) ValidateAPIKey(ctx context.Context, apiKey string) (*contracts.APIKeyClaims, error) {
	a.logger.Debug("Validating API key", zap.String("api_key", "***"))

	// Call userauth service to validate API key
	claims, err := a.service.ValidateAPIKey(ctx, apiKey)
	if err != nil {
		a.logger.Error("API key validation failed", zap.Error(err))
		return nil, fmt.Errorf("API key validation failed: %w", err)
	}

	// Convert userauth API key claims to contract claims
	return &contracts.APIKeyClaims{
		KeyID:       claims.KeyID,
		UserID:      claims.UserID,
		Permissions: claims.Permissions,
		ExpiresAt:   claims.ExpiresAt,
	}, nil
}

// CheckPermission checks if user has permission for resource/action
func (a *UserAuthServiceAdapter) CheckPermission(ctx context.Context, userID uuid.UUID, resource, action string) error {
	a.logger.Debug("Checking permission",
		zap.String("user_id", userID.String()),
		zap.String("resource", resource),
		zap.String("action", action))

	// Call userauth service to check permission
	if err := a.service.CheckPermission(ctx, userID, resource, action); err != nil {
		a.logger.Error("Permission check failed",
			zap.String("user_id", userID.String()),
			zap.String("resource", resource),
			zap.String("action", action),
			zap.Error(err))
		return fmt.Errorf("permission check failed: %w", err)
	}

	return nil
}

// GetUserPermissions retrieves user permissions
func (a *UserAuthServiceAdapter) GetUserPermissions(ctx context.Context, userID uuid.UUID) ([]contracts.Permission, error) {
	a.logger.Debug("Getting user permissions", zap.String("user_id", userID.String()))

	// Call userauth service to get permissions
	permissions, err := a.service.GetUserPermissions(ctx, userID)
	if err != nil {
		a.logger.Error("Failed to get user permissions",
			zap.String("user_id", userID.String()),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get user permissions: %w", err)
	}

	// Convert userauth permissions to contract permissions
	contractPermissions := make([]contracts.Permission, len(permissions))
	for i, perm := range permissions {
		contractPermissions[i] = contracts.Permission{
			Resource: perm.Resource,
			Action:   perm.Action,
			Scope:    perm.Scope,
		}
	}

	return contractPermissions, nil
}

// GetUser retrieves user information
func (a *UserAuthServiceAdapter) GetUser(ctx context.Context, userID uuid.UUID) (*contracts.User, error) {
	a.logger.Debug("Getting user", zap.String("user_id", userID.String()))

	// Call userauth service to get user
	user, err := a.service.GetUser(ctx, userID)
	if err != nil {
		a.logger.Error("Failed to get user",
			zap.String("user_id", userID.String()),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	// Convert userauth user to contract user
	return &contracts.User{
		ID:          user.ID,
		Email:       user.Email,
		FirstName:   user.FirstName,
		LastName:    user.LastName,
		Role:        user.Role,
		KYCLevel:    user.KYCLevel,
		Status:      user.Status,
		CreatedAt:   user.CreatedAt,
		LastLoginAt: user.LastLoginAt,
	}, nil
}

// GetUserByEmail retrieves user by email
func (a *UserAuthServiceAdapter) GetUserByEmail(ctx context.Context, email string) (*contracts.User, error) {
	a.logger.Debug("Getting user by email", zap.String("email", email))

	// Call userauth service to get user by email
	user, err := a.service.GetUserByEmail(ctx, email)
	if err != nil {
		a.logger.Error("Failed to get user by email",
			zap.String("email", email),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get user by email: %w", err)
	}

	// Convert userauth user to contract user
	return &contracts.User{
		ID:          user.ID,
		Email:       user.Email,
		FirstName:   user.FirstName,
		LastName:    user.LastName,
		Role:        user.Role,
		KYCLevel:    user.KYCLevel,
		Status:      user.Status,
		CreatedAt:   user.CreatedAt,
		LastLoginAt: user.LastLoginAt,
	}, nil
}

// ValidateKYCLevel validates user's KYC level
func (a *UserAuthServiceAdapter) ValidateKYCLevel(ctx context.Context, userID uuid.UUID, requiredLevel int) error {
	a.logger.Debug("Validating KYC level",
		zap.String("user_id", userID.String()),
		zap.Int("required_level", requiredLevel))

	// Call userauth service to validate KYC level
	if err := a.service.ValidateKYCLevel(ctx, userID, requiredLevel); err != nil {
		a.logger.Error("KYC level validation failed",
			zap.String("user_id", userID.String()),
			zap.Int("required_level", requiredLevel),
			zap.Error(err))
		return fmt.Errorf("KYC level validation failed: %w", err)
	}

	return nil
}

// GetKYCStatus retrieves user's KYC status
func (a *UserAuthServiceAdapter) GetKYCStatus(ctx context.Context, userID uuid.UUID) (*contracts.KYCStatus, error) {
	a.logger.Debug("Getting KYC status", zap.String("user_id", userID.String()))

	// Call userauth service to get KYC status
	status, err := a.service.GetKYCStatus(ctx, userID)
	if err != nil {
		a.logger.Error("Failed to get KYC status",
			zap.String("user_id", userID.String()),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get KYC status: %w", err)
	}

	// Convert userauth KYC status to contract KYC status
	return &contracts.KYCStatus{
		UserID:       status.UserID,
		Level:        status.Level,
		Status:       status.Status,
		VerifiedAt:   status.VerifiedAt,
		ExpiresAt:    status.ExpiresAt,
		Restrictions: status.Restrictions,
	}, nil
}

// CreateSession creates a new user session
func (a *UserAuthServiceAdapter) CreateSession(ctx context.Context, userID uuid.UUID, metadata map[string]string) (*contracts.Session, error) {
	a.logger.Debug("Creating session", zap.String("user_id", userID.String()))

	// Call userauth service to create session
	session, err := a.service.CreateSession(ctx, userID, metadata)
	if err != nil {
		a.logger.Error("Failed to create session",
			zap.String("user_id", userID.String()),
			zap.Error(err))
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	// Convert userauth session to contract session
	return &contracts.Session{
		ID:        session.ID,
		UserID:    session.UserID,
		IPAddress: session.IPAddress,
		UserAgent: session.UserAgent,
		Metadata:  session.Metadata,
		CreatedAt: session.CreatedAt,
		ExpiresAt: session.ExpiresAt,
		LastSeen:  session.LastSeen,
	}, nil
}

// ValidateSession validates user session
func (a *UserAuthServiceAdapter) ValidateSession(ctx context.Context, sessionID uuid.UUID) (*contracts.Session, error) {
	a.logger.Debug("Validating session", zap.String("session_id", sessionID.String()))

	// Call userauth service to validate session
	session, err := a.service.ValidateSession(ctx, sessionID)
	if err != nil {
		a.logger.Error("Session validation failed",
			zap.String("session_id", sessionID.String()),
			zap.Error(err))
		return nil, fmt.Errorf("session validation failed: %w", err)
	}

	// Convert userauth session to contract session
	return &contracts.Session{
		ID:        session.ID,
		UserID:    session.UserID,
		IPAddress: session.IPAddress,
		UserAgent: session.UserAgent,
		Metadata:  session.Metadata,
		CreatedAt: session.CreatedAt,
		ExpiresAt: session.ExpiresAt,
		LastSeen:  session.LastSeen,
	}, nil
}

// InvalidateSession invalidates user session
func (a *UserAuthServiceAdapter) InvalidateSession(ctx context.Context, sessionID uuid.UUID) error {
	a.logger.Debug("Invalidating session", zap.String("session_id", sessionID.String()))

	// Call userauth service to invalidate session
	if err := a.service.InvalidateSession(ctx, sessionID); err != nil {
		a.logger.Error("Failed to invalidate session",
			zap.String("session_id", sessionID.String()),
			zap.Error(err))
		return fmt.Errorf("failed to invalidate session: %w", err)
	}

	return nil
}

// CheckRateLimit checks rate limit for user action
func (a *UserAuthServiceAdapter) CheckRateLimit(ctx context.Context, userID uuid.UUID, action string) (*contracts.RateLimitResult, error) {
	a.logger.Debug("Checking rate limit",
		zap.String("user_id", userID.String()),
		zap.String("action", action))

	// Call userauth service to check rate limit
	result, err := a.service.CheckRateLimit(ctx, userID, action)
	if err != nil {
		a.logger.Error("Rate limit check failed",
			zap.String("user_id", userID.String()),
			zap.String("action", action),
			zap.Error(err))
		return nil, fmt.Errorf("rate limit check failed: %w", err)
	}

	// Convert userauth rate limit result to contract result
	return &contracts.RateLimitResult{
		Allowed:    result.Allowed,
		Remaining:  result.Remaining,
		ResetAt:    result.ResetAt,
		RetryAfter: result.RetryAfter,
	}, nil
}

// HealthCheck performs health check
func (a *UserAuthServiceAdapter) HealthCheck(ctx context.Context) (*contracts.HealthStatus, error) {
	a.logger.Debug("Performing health check")

	// Call userauth service health check
	health, err := a.service.HealthCheck(ctx)
	if err != nil {
		a.logger.Error("Health check failed", zap.Error(err))
		return nil, fmt.Errorf("health check failed: %w", err)
	}

	// Convert userauth health status to contract health status
	return &contracts.HealthStatus{
		Status:       health.Status,
		Timestamp:    health.Timestamp,
		Version:      health.Version,
		Uptime:       health.Uptime,
		Metrics:      health.Metrics,
		Dependencies: health.Dependencies,
	}, nil
}
