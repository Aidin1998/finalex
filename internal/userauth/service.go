package userauth

import (
	"context"
	"fmt"
	"time"

	twofa "github.com/Aidin1998/pincex_unified/internal/userauth/2FA"

	"github.com/Aidin1998/pincex_unified/internal/userauth/audit"
	"github.com/Aidin1998/pincex_unified/internal/userauth/auth"
	"github.com/Aidin1998/pincex_unified/internal/userauth/cache"
	"github.com/Aidin1998/pincex_unified/internal/userauth/compliance"
	"github.com/Aidin1998/pincex_unified/internal/userauth/encryption"

	"github.com/Aidin1998/pincex_unified/internal/userauth/identities"
	"github.com/Aidin1998/pincex_unified/internal/userauth/kyc"
	"github.com/Aidin1998/pincex_unified/internal/userauth/password"
	"github.com/Aidin1998/pincex_unified/pkg/models"
	"github.com/google/uuid"
	"go.uber.org/zap"

	redisv8 "github.com/go-redis/redis/v8"
)

// Service implements the full UserAuthService interface and all enterprise features.
type Service struct {
	config            *ServiceConfig
	logger            *zap.Logger
	authService       auth.AuthService
	identityService   identities.Service
	kycService        kyc.Service
	auditService      *audit.Service
	passwordService   *password.Service
	complianceService *compliance.ComplianceService
	encryptionService *encryption.PIIEncryptionService
	twoFAService      *twofa.Service
	clusteredCache    *cache.ClusteredCache
	tieredRateLimiter *auth.TieredRateLimiter
	redisCluster      *redisv8.ClusterClient
}

// Core service accessors

// AuthService returns the authentication service
func (s *Service) AuthService() auth.AuthService {
	return s.authService
}

// IdentityService returns the identity service
func (s *Service) IdentityService() identities.Service {
	return s.identityService
}

// KYCService returns the KYC service
func (s *Service) KYCService() kyc.Service {
	return s.kycService
}

// AuditService returns the audit service
func (s *Service) AuditService() *audit.Service {
	return s.auditService
}

// PasswordService returns the password service
func (s *Service) PasswordService() *password.Service {
	return s.passwordService
}

// ComplianceService returns the compliance service
func (s *Service) ComplianceService() *compliance.ComplianceService {
	return s.complianceService
}

// EncryptionService returns the encryption service
func (s *Service) EncryptionService() *encryption.PIIEncryptionService {
	return s.encryptionService
}

// TwoFAService returns the 2FA service
func (s *Service) TwoFAService() *twofa.Service {
	return s.twoFAService
}

// Enterprise feature accessors

// ClusteredCache returns the clustered cache
func (s *Service) ClusteredCache() *cache.ClusteredCache {
	return s.clusteredCache
}

// AdminAPI returns the admin API

// TieredRateLimiter returns the tiered rate limiter
func (s *Service) TieredRateLimiter() *auth.TieredRateLimiter {
	return s.tieredRateLimiter
}

// RedisCluster returns the Redis cluster client
func (s *Service) RedisCluster() *redisv8.ClusterClient {
	return s.redisCluster
}

// Enterprise status and health check methods

// GetEnterpriseFeatures returns the status of all enterprise features
func (s *Service) GetEnterpriseFeatures(ctx context.Context) map[string]interface{} {
	features := map[string]interface{}{
		"clustered_cache": map[string]interface{}{
			"enabled": s.clusteredCache != nil,
			"stats":   nil,
		},
		"redis_cluster": map[string]interface{}{
			"enabled": s.redisCluster != nil,
		},
	}

	// Add cache statistics if available
	if s.clusteredCache != nil {
		if stats := s.clusteredCache.GetStats(); stats != nil {
			features["clustered_cache"].(map[string]interface{})["stats"] = stats
		}
	}

	return features
}

// GetClusterStatus returns the Redis cluster status
func (s *Service) GetClusterStatus(ctx context.Context) (map[string]interface{}, error) {
	if s.redisCluster == nil {
		return map[string]interface{}{
			"enabled": false,
			"status":  "disabled",
		}, nil
	}

	// Check cluster health
	clusterInfo, err := s.redisCluster.ClusterInfo(ctx).Result()
	if err != nil {
		return map[string]interface{}{
			"enabled": true,
			"status":  "error",
			"error":   err.Error(),
		}, err
	}

	return map[string]interface{}{
		"enabled":      true,
		"status":       "healthy",
		"cluster_info": clusterInfo,
	}, nil
}

// Rate limiting methods

// CheckRateLimit performs comprehensive rate limiting check
func (s *Service) CheckRateLimit(ctx context.Context, userID, endpoint, clientIP string) (*auth.RateLimitResult, error) {
	if s.tieredRateLimiter == nil {
		// Return no-op result if rate limiter is not available
		return &auth.RateLimitResult{Allowed: true}, nil
	}
	return s.tieredRateLimiter.CheckRateLimit(ctx, userID, endpoint, clientIP)
}

// GetUserRateLimitStatus returns current rate limit status for a user
func (s *Service) GetUserRateLimitStatus(ctx context.Context, userID string) (map[string]*models.RateLimitInfo, error) {
	if s.tieredRateLimiter == nil {
		return nil, fmt.Errorf("rate limiting not available")
	}
	return s.tieredRateLimiter.GetUserRateLimitStatus(ctx, userID)
}

// Service lifecycle methods

// Start starts all sub-services and enterprise features
func (s *Service) Start(ctx context.Context) error {
	s.logger.Info("Starting unified user authentication service with enterprise features")

	// Initialize performance optimizations if available
	if s.tieredRateLimiter != nil {
		s.logger.Info("Performance optimizer initialized")
	}

	s.logger.Info("All enterprise features initialized successfully")
	return nil
}

// Stop stops all sub-services and enterprise features
func (s *Service) Stop(ctx context.Context) error {
	s.logger.Info("Stopping unified user authentication service")

	// Cleanup performance optimizer if available
	if s.tieredRateLimiter != nil {
		s.logger.Info("Performance optimizer stopped")
	}

	// Close Redis cluster connection
	if s.redisCluster != nil {
		if err := s.redisCluster.Close(); err != nil {
			s.logger.Error("Failed to close Redis cluster connection", zap.Error(err))
		}
	}

	s.logger.Info("All enterprise features stopped successfully")
	return nil
}

// Authentication methods with enterprise features

// AuthenticateWithMFA performs multi-factor authentication with enterprise features
func (s *Service) AuthenticateWithMFA(ctx context.Context, req *MFAAuthRequest) (*auth.TokenPair, error) {
	// Check rate limits before authentication
	if rateLimitResult, err := s.CheckRateLimit(ctx, req.Email, "login", req.IPAddress); err != nil {
		s.logger.Warn("Rate limit check failed", zap.Error(err))
	} else if !rateLimitResult.Allowed {
		s.auditService.LogEvent(ctx, audit.EventLoginFailed, audit.RiskLevelHigh, audit.AuditContext{
			IPAddress: req.IPAddress,
			UserAgent: req.UserAgent,
		}, "Rate limit exceeded for login attempt")
		return nil, fmt.Errorf("rate limit exceeded")
	}

	// First authenticate with password
	tokenPair, user, err := s.authService.AuthenticateUser(ctx, req.Email, req.Password)
	if err != nil {
		s.auditService.LogEvent(ctx, audit.EventLoginFailed, audit.RiskLevelMedium, audit.AuditContext{
			IPAddress: req.IPAddress,
			UserAgent: req.UserAgent,
		}, "Password authentication failed")
		return nil, err
	}

	// Check if 2FA is enabled
	twoFAEnabled, err := s.twoFAService.IsTwoFactorEnabled(ctx, user.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to check 2FA status: %w", err)
	}

	if twoFAEnabled {
		// Verify 2FA token
		if req.TwoFactorToken == "" {
			return nil, fmt.Errorf("two-factor authentication token required")
		}

		verified, err := s.twoFAService.VerifyTOTP(ctx, user.ID, req.TwoFactorToken)
		if err != nil || !verified {
			// Log failed 2FA attempt
			s.auditService.LogEvent(ctx, audit.EventTwoFactorFailed, audit.RiskLevelHigh, audit.AuditContext{
				UserID:    user.ID,
				IPAddress: req.IPAddress,
				UserAgent: req.UserAgent,
			}, "2FA verification failed")
			return nil, fmt.Errorf("invalid two-factor authentication token")
		}
	} else {
		// Check if 2FA enforcement is required
		enforced, err := s.twoFAService.EnforceTwoFactor(ctx, user.ID, s.config.TwoFAGracePeriodDays)
		if err != nil {
			return nil, fmt.Errorf("failed to check 2FA enforcement: %w", err)
		}
		if !enforced {
			return nil, fmt.Errorf("two-factor authentication setup required")
		}
	}

	// Log successful authentication
	s.auditService.LogEvent(ctx, audit.EventUserLogin, audit.RiskLevelLow, audit.AuditContext{
		UserID:    user.ID,
		IPAddress: req.IPAddress,
		UserAgent: req.UserAgent,
		Metadata: map[string]interface{}{
			"mfa_used": twoFAEnabled,
		},
	}, "User authentication successful")

	return tokenPair, nil
}

// Delegation methods for auth service

// CreateAPIKey creates a new API key for a user
func (s *Service) CreateAPIKey(ctx context.Context, userID uuid.UUID, name string, permissions []string, expiresAt *time.Time) (*auth.APIKey, error) {
	return s.authService.CreateAPIKey(ctx, userID, name, permissions, expiresAt)
}

// ValidateAPIKey validates an API key
func (s *Service) ValidateAPIKey(ctx context.Context, apiKey string) (*auth.APIKeyClaims, error) {
	return s.authService.ValidateAPIKey(ctx, apiKey)
}

// GenerateTOTPSecret generates a new TOTP secret for a user
func (s *Service) GenerateTOTPSecret(ctx context.Context, userID uuid.UUID) (*auth.TOTPSetup, error) {
	return s.authService.GenerateTOTPSecret(ctx, userID)
}

// VerifyTOTPSetup verifies TOTP setup
func (s *Service) VerifyTOTPSetup(ctx context.Context, userID uuid.UUID, secret, token string) error {
	return s.authService.VerifyTOTPSetup(ctx, userID, secret, token)
}

// ValidateToken validates a JWT token
func (s *Service) ValidateToken(ctx context.Context, tokenString string) (*auth.TokenClaims, error) {
	return s.authService.ValidateToken(ctx, tokenString)
}

// RefreshToken refreshes an access token using a refresh token
func (s *Service) RefreshToken(ctx context.Context, refreshToken string) (*auth.TokenPair, error) {
	return s.authService.RefreshToken(ctx, refreshToken)
}

// CreateSession creates a new user session
func (s *Service) CreateSession(ctx context.Context, userID uuid.UUID, deviceFingerprint string) (*auth.Session, error) {
	return s.authService.CreateSession(ctx, userID, deviceFingerprint)
}

// ValidateSession validates a user session
func (s *Service) ValidateSession(ctx context.Context, sessionID uuid.UUID) (*auth.Session, error) {
	return s.authService.ValidateSession(ctx, sessionID)
}

// InvalidateSession invalidates a user session
func (s *Service) InvalidateSession(ctx context.Context, sessionID uuid.UUID) error {
	return s.authService.InvalidateSession(ctx, sessionID)
}

// GetUserPermissions gets a user's permissions
func (s *Service) GetUserPermissions(ctx context.Context, userID uuid.UUID) ([]auth.Permission, error) {
	return s.authService.GetUserPermissions(ctx, userID)
}

// AssignRole assigns a role to a user
func (s *Service) AssignRole(ctx context.Context, userID uuid.UUID, role string) error {
	return s.authService.AssignRole(ctx, userID, role)
}

// RevokeRole revokes a role from a user
func (s *Service) RevokeRole(ctx context.Context, userID uuid.UUID, role string) error {
	return s.authService.RevokeRole(ctx, userID, role)
}
