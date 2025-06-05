package userauth

import (
	"context"
	"fmt"
	"time"

	twofa "github.com/Aidin1998/pincex_unified/internal/userauth/2FA"
	"github.com/Aidin1998/pincex_unified/internal/userauth/audit"
	"github.com/Aidin1998/pincex_unified/internal/userauth/auth"
	"github.com/Aidin1998/pincex_unified/internal/userauth/compliance"
	"github.com/Aidin1998/pincex_unified/internal/userauth/encryption"
	"github.com/Aidin1998/pincex_unified/internal/userauth/identities"
	"github.com/Aidin1998/pincex_unified/internal/userauth/kyc"
	"github.com/Aidin1998/pincex_unified/internal/userauth/notification"
	"github.com/Aidin1998/pincex_unified/internal/userauth/password"
	"github.com/Aidin1998/pincex_unified/pkg/models"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// ServiceConfig holds configuration for the UserAuth service
type ServiceConfig struct {
	JWTSecret              string
	JWTExpiration          time.Duration
	RefreshTokenSecret     string
	RefreshTokenExpiration time.Duration
	Issuer                 string
	TwoFAGracePeriodDays   int
	EmailConfig            notification.EmailConfig
	SMSConfig              notification.SMSConfig
	EncryptionKey          string
}

// Service provides unified user authentication and identity management
type Service struct {
	// Core services
	authService         auth.AuthService
	identityService     identities.Service
	kycService          kyc.Service
	auditService        *audit.Service
	notificationService *notification.Service
	passwordService     *password.Service
	complianceService   *compliance.ComplianceService
	encryptionService   *encryption.PIIEncryptionService
	twoFAService        *twofa.Service
	registrationService *EnterpriseRegistrationService

	// Infrastructure
	tieredRateLimiter *auth.TieredRateLimiter
	logger            *zap.Logger
	db                *gorm.DB

	// Configuration
	config ServiceConfig
}

// NewService creates a new unified user authentication service
func NewService(logger *zap.Logger, db *gorm.DB, config ServiceConfig) (*Service, error) {
	// Initialize auth service
	authSvc, err := auth.NewAuthService(
		logger,
		db,
		config.JWTSecret,
		config.JWTExpiration,
		config.RefreshTokenSecret,
		config.RefreshTokenExpiration,
		config.Issuer,
		nil, // Rate limiter will be set after creation
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create auth service: %w", err)
	}

	// Initialize other services
	identitySvc := identities.NewService(logger, db)
	kycSvc := kyc.NewService(logger, db)
	auditSvc := audit.NewService(logger, db)
	notificationSvc := notification.NewService(logger, db, config.EmailConfig, config.SMSConfig)
	passwordSvc := password.NewService(logger, db)
	complianceSvc := compliance.NewComplianceService(db, logger, nil, nil, nil)
	encryptionSvc := encryption.NewPIIEncryptionService(config.EncryptionKey)
	twoFASvc := twofa.NewService(logger, db, config.Issuer)

	// Create registration service
	registrationSvc := NewEnterpriseRegistrationService(
		db,
		logger,
		encryptionSvc,
		complianceSvc,
		auditSvc,
		passwordSvc,
		kycSvc,
		notificationSvc,
	)

	// Create user service adapter for rate limiter
	userService := auth.NewAuthUserService(db)

	// Initialize tiered rate limiter (using mock Redis for now)
	tieredRateLimiter := auth.NewTieredRateLimiter(nil, logger, userService)

	service := &Service{
		authService:         authSvc,
		identityService:     identitySvc,
		kycService:          kycSvc,
		auditService:        auditSvc,
		notificationService: notificationSvc,
		passwordService:     passwordSvc,
		complianceService:   complianceSvc,
		encryptionService:   encryptionSvc,
		twoFAService:        twoFASvc,
		registrationService: registrationSvc,
		tieredRateLimiter:   tieredRateLimiter,
		logger:              logger,
		db:                  db,
		config:              config,
	}
	return service, nil
}

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

// TieredRateLimiter returns the tiered rate limiter
func (s *Service) TieredRateLimiter() *auth.TieredRateLimiter {
	return s.tieredRateLimiter
}

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

// Start starts all sub-services
func (s *Service) Start(ctx context.Context) error {
	s.logger.Info("Starting unified user authentication service")
	// Add any initialization logic here
	return nil
}

// Stop stops all sub-services
func (s *Service) Stop(ctx context.Context) error {
	s.logger.Info("Stopping unified user authentication service")
	// Add any cleanup logic here
	return nil
}

// Service accessor methods for enterprise components

// AuditService returns the audit service
func (s *Service) AuditService() *audit.Service {
	return s.auditService
}

// NotificationService returns the notification service
func (s *Service) NotificationService() *notification.Service {
	return s.notificationService
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

// RegistrationService returns the registration service
func (s *Service) RegistrationService() *EnterpriseRegistrationService {
	return s.registrationService
}

// Enterprise Business Logic Methods

// RegisterUserWithCompliance performs enterprise-grade user registration with full compliance
func (s *Service) RegisterUserWithCompliance(ctx context.Context, req *EnterpriseRegistrationRequest) (*EnterpriseRegistrationResponse, error) {
	// Use the dedicated registration service
	return s.registrationService.RegisterUser(ctx, req)
}

// AuthenticateWithMFA performs multi-factor authentication
func (s *Service) AuthenticateWithMFA(ctx context.Context, req *MFAAuthRequest) (*auth.TokenPair, error) {
	// First authenticate with password
	tokenPair, user, err := s.authService.AuthenticateUser(ctx, req.Email, req.Password)
	if err != nil {
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

// MFAAuthRequest represents a multi-factor authentication request
type MFAAuthRequest struct {
	Email          string `json:"email" validate:"required,email"`
	Password       string `json:"password" validate:"required"`
	TwoFactorToken string `json:"two_factor_token,omitempty"`
	IPAddress      string `json:"ip_address"`
	UserAgent      string `json:"user_agent"`
}

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
