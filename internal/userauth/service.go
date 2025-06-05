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
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
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
	complianceService   *compliance.Service
	encryptionService   *encryption.Service
	twoFAService        *twofa.Service

	// Infrastructure
	tieredRateLimiter *auth.TieredRateLimiter
	logger            *zap.Logger
	db                *gorm.DB
	redis             *redis.Client

	// Configuration
	config ServiceConfig
}

// NewService creates a new unified user authentication service
func NewService(logger *zap.Logger, db *gorm.DB, redisClient *redis.Client, config ServiceConfig) (*Service, error) {
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
	notificationSvc := notification.NewService(logger, db, redisClient, config.EmailConfig, config.SMSConfig)
	passwordSvc := password.NewService(logger, db, redisClient)
	complianceSvc := compliance.NewService(logger, db, redisClient)
	encryptionSvc := encryption.NewService(logger)
	twoFASvc := twofa.NewService(logger, db, config.Issuer)

	// Create user service adapter for rate limiter
	userService := auth.NewAuthUserService(db)

	// Initialize tiered rate limiter
	tieredRateLimiter := auth.NewTieredRateLimiter(redisClient, logger, userService)

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
		tieredRateLimiter:   tieredRateLimiter,
		logger:              logger,
		db:                  db,
		redis:               redisClient,
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

// RateLimitMiddleware returns the rate limiting middleware
func (s *Service) RateLimitMiddleware() gin.HandlerFunc {
	if s.tieredRateLimiter == nil {
		// Return no-op middleware if rate limiter is not available
		return func(c *gin.Context) {
			c.Next()
		}
	}
	return s.tieredRateLimiter.Middleware()
}

// SetEmergencyMode enables or disables emergency rate limiting mode
func (s *Service) SetEmergencyMode(enabled bool) {
	if s.tieredRateLimiter != nil {
		s.tieredRateLimiter.SetEmergencyMode(enabled)
	}
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

// GetIPRateLimitStatus returns current rate limit status for an IP
func (s *Service) GetIPRateLimitStatus(ctx context.Context, clientIP string) (map[string]*models.RateLimitInfo, error) {
	if s.tieredRateLimiter == nil {
		return nil, fmt.Errorf("rate limiting not available")
	}
	return s.tieredRateLimiter.GetIPRateLimitStatus(ctx, clientIP)
}

// ResetUserRateLimit resets rate limits for a specific user and rate type
func (s *Service) ResetUserRateLimit(ctx context.Context, userID, rateType string) error {
	if s.tieredRateLimiter == nil {
		return fmt.Errorf("rate limiting not available")
	}
	return s.tieredRateLimiter.ResetUserRateLimit(ctx, userID, rateType)
}

// ResetIPRateLimit resets rate limits for a specific IP and endpoint
func (s *Service) ResetIPRateLimit(ctx context.Context, clientIP, endpoint string) error {
	if s.tieredRateLimiter == nil {
		return fmt.Errorf("rate limiting not available")
	}
	return s.tieredRateLimiter.ResetIPRateLimit(ctx, clientIP, endpoint)
}

// UpdateTierLimits updates limits for a specific tier
func (s *Service) UpdateTierLimits(tier models.UserTier, limits auth.TierConfig) {
	if s.tieredRateLimiter != nil {
		s.tieredRateLimiter.UpdateTierLimits(tier, limits)
	}
}

// UpdateEndpointConfig updates configuration for a specific endpoint
func (s *Service) UpdateEndpointConfig(endpoint string, config auth.EndpointConfig) {
	if s.tieredRateLimiter != nil {
		s.tieredRateLimiter.UpdateEndpointConfig(endpoint, config)
	}
}

// UpdateRateLimitConfig updates the rate limit configuration
func (s *Service) UpdateRateLimitConfig(config *auth.RateLimitConfig) {
	if s.tieredRateLimiter != nil {
		s.tieredRateLimiter.UpdateConfig(config)
	}
}

// GetRateLimitConfig returns the current rate limit configuration
func (s *Service) GetRateLimitConfig() *auth.RateLimitConfig {
	if s.tieredRateLimiter == nil {
		return nil
	}
	return s.tieredRateLimiter.GetConfig()
}

// StartBackgroundCleanup starts a background goroutine for periodic cleanup
func (s *Service) StartBackgroundCleanup(ctx context.Context) {
	if s.tieredRateLimiter != nil {
		s.tieredRateLimiter.StartBackgroundCleanup(ctx)
	}
}

// CleanupExpiredData removes expired rate limit data
func (s *Service) CleanupExpiredData(ctx context.Context) error {
	if s.tieredRateLimiter == nil {
		return fmt.Errorf("rate limiting not available")
	}
	return s.tieredRateLimiter.CleanupExpiredData(ctx)
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
