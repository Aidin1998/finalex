package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/Aidin1998/pincex_unified/pkg/models"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// AuthService defines comprehensive authentication operations
type AuthService interface {
	// Core Authentication
	AuthenticateUser(ctx context.Context, email, password string) (*TokenPair, *models.User, error)
	CheckMFA(ctx context.Context, userID uuid.UUID) (bool, error)

	// Token Management
	ValidateToken(ctx context.Context, tokenString string) (*TokenClaims, error)
	RefreshToken(ctx context.Context, refreshToken string) (*TokenPair, error)
	RevokeToken(ctx context.Context, tokenString string) error
	RevokeAllTokens(ctx context.Context, userID uuid.UUID) error

	// API Key Management
	CreateAPIKey(ctx context.Context, userID uuid.UUID, name string, permissions []string, expiresAt *time.Time) (*APIKey, error)
	ValidateAPIKey(ctx context.Context, apiKey string) (*APIKeyClaims, error)
	RevokeAPIKey(ctx context.Context, keyID uuid.UUID) error
	ListAPIKeys(ctx context.Context, userID uuid.UUID) ([]*APIKey, error)

	// Multi-Factor Authentication
	GenerateTOTPSecret(ctx context.Context, userID uuid.UUID) (*TOTPSetup, error)
	VerifyTOTPSetup(ctx context.Context, userID uuid.UUID, secret, token string) error
	VerifyTOTPToken(ctx context.Context, userID uuid.UUID, token string) error
	DisableTOTP(ctx context.Context, userID uuid.UUID, currentPassword string) error

	// Session Management
	CreateSession(ctx context.Context, userID uuid.UUID, deviceFingerprint string) (*Session, error)
	ValidateSession(ctx context.Context, sessionID uuid.UUID) (*Session, error)
	InvalidateSession(ctx context.Context, sessionID uuid.UUID) error
	InvalidateAllSessions(ctx context.Context, userID uuid.UUID) error

	// OAuth2/OIDC
	InitiateOAuthFlow(ctx context.Context, provider, redirectURI string) (*OAuthState, error)
	HandleOAuthCallback(ctx context.Context, state, code string) (*TokenPair, error)
	LinkOAuthAccount(ctx context.Context, userID uuid.UUID, provider, oauthUserID string) error

	// Role-Based Access Control (RBAC)
	ValidatePermission(ctx context.Context, userID uuid.UUID, resource, action string) error
	AssignRole(ctx context.Context, userID uuid.UUID, role string) error
	RevokeRole(ctx context.Context, userID uuid.UUID, role string) error
	GetUserPermissions(ctx context.Context, userID uuid.UUID) ([]Permission, error)

	// Device Management
	RegisterTrustedDevice(ctx context.Context, userID uuid.UUID, deviceFingerprint string) error
	IsTrustedDevice(ctx context.Context, userID uuid.UUID, deviceFingerprint string) (bool, error)
	RevokeTrustedDevice(ctx context.Context, userID uuid.UUID, deviceFingerprint string) error
}

// Service implements AuthService
type Service struct {
	logger                    *zap.Logger
	db                        *gorm.DB
	jwtSecret                 []byte
	jwtExpirationDuration     time.Duration
	refreshSecret             []byte
	refreshExpirationDuration time.Duration
	issuer                    string
	oauthProviders            map[string]*OAuthProvider
	rateLimiter               RateLimiter
}

// TokenClaims represents JWT token claims
type TokenClaims struct {
	UserID      uuid.UUID `json:"user_id"`
	Email       string    `json:"email"`
	Role        string    `json:"role"`
	Permissions []string  `json:"permissions"`
	SessionID   uuid.UUID `json:"session_id"`
	TokenType   string    `json:"token_type"` // access, refresh
	jwt.RegisteredClaims
}

// TokenPair represents access and refresh tokens
type TokenPair struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	TokenType    string    `json:"token_type"`
}

// APIKey represents an API key
type APIKey struct {
	ID          uuid.UUID  `json:"id" gorm:"primaryKey;type:uuid"`
	UserID      uuid.UUID  `json:"user_id" gorm:"type:uuid;index"`
	Name        string     `json:"name"`
	KeyHash     string     `json:"-" gorm:"column:key_hash"`
	Permissions []string   `json:"permissions" gorm:"type:text;serializer:json"`
	LastUsedAt  *time.Time `json:"last_used_at"`
	ExpiresAt   *time.Time `json:"expires_at"`
	IsActive    bool       `json:"is_active" gorm:"default:true"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// APIKeyClaims represents API key claims
type APIKeyClaims struct {
	KeyID       uuid.UUID `json:"key_id"`
	UserID      uuid.UUID `json:"user_id"`
	Permissions []string  `json:"permissions"`
}

// TOTPSetup represents TOTP setup information
type TOTPSetup struct {
	Secret      string   `json:"secret"`
	QRCode      string   `json:"qr_code"`
	BackupCodes []string `json:"backup_codes"`
}

// Session represents a user session
type Session struct {
	ID                uuid.UUID `json:"id" gorm:"primaryKey;type:uuid"`
	UserID            uuid.UUID `json:"user_id" gorm:"type:uuid;index"`
	DeviceFingerprint string    `json:"device_fingerprint"`
	IPAddress         string    `json:"ip_address"`
	UserAgent         string    `json:"user_agent"`
	IsActive          bool      `json:"is_active" gorm:"default:true"`
	LastActivityAt    time.Time `json:"last_activity_at"`
	ExpiresAt         time.Time `json:"expires_at"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// OAuthState represents OAuth state
type OAuthState struct {
	State       string    `json:"state"`
	Provider    string    `json:"provider"`
	RedirectURI string    `json:"redirect_uri"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// OAuthProvider represents OAuth provider configuration
type OAuthProvider struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	Scopes       []string
	AuthURL      string
	TokenURL     string
	UserInfoURL  string
}

// Permission represents a permission
type Permission struct {
	Resource string `json:"resource"`
	Action   string `json:"action"`
}

// RateLimiter interface for rate limiting
type RateLimiter interface {
	Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error)
}

// BlacklistedToken represents a blacklisted token
type BlacklistedToken struct {
	ID        uuid.UUID `json:"id" gorm:"primaryKey;type:uuid"`
	TokenHash string    `json:"token_hash" gorm:"uniqueIndex"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

// NewAuthService creates a new authentication service
func NewAuthService(
	logger *zap.Logger,
	db *gorm.DB,
	jwtSecret string,
	jwtExpiration time.Duration,
	refreshSecret string,
	refreshExpiration time.Duration,
	issuer string,
	rateLimiter RateLimiter,
) (AuthService, error) {
	if jwtSecret == "" {
		return nil, fmt.Errorf("JWT secret cannot be empty")
	}

	if refreshSecret == "" {
		return nil, fmt.Errorf("refresh secret cannot be empty")
	}

	service := &Service{
		logger:                    logger,
		db:                        db,
		jwtSecret:                 []byte(jwtSecret),
		jwtExpirationDuration:     jwtExpiration,
		refreshSecret:             []byte(refreshSecret),
		refreshExpirationDuration: refreshExpiration,
		issuer:                    issuer,
		oauthProviders:            make(map[string]*OAuthProvider),
		rateLimiter:               rateLimiter,
	}

	// Auto-migrate database tables
	if err := service.migrate(); err != nil {
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	return service, nil
}

// migrate runs database migrations
func (s *Service) migrate() error {
	return s.db.AutoMigrate(
		&APIKey{},
		&Session{},
		&BlacklistedToken{},
	)
}

// ValidateToken validates a JWT token and returns claims
func (s *Service) ValidateToken(ctx context.Context, tokenString string) (*TokenClaims, error) {
	// Apply rate limiting
	if s.rateLimiter != nil {
		allowed, err := s.rateLimiter.Allow(ctx, "token_validation", 100, time.Minute)
		if err != nil {
			return nil, fmt.Errorf("rate limiter error: %w", err)
		}
		if !allowed {
			return nil, fmt.Errorf("rate limit exceeded")
		}
	}

	// Remove Bearer prefix if present
	tokenString = strings.TrimPrefix(tokenString, "Bearer ")

	// Check if token is blacklisted
	tokenHash := hashToken(tokenString)
	var blacklistedToken BlacklistedToken
	if err := s.db.Where("token_hash = ? AND expires_at > ?", tokenHash, time.Now()).First(&blacklistedToken).Error; err == nil {
		return nil, fmt.Errorf("token has been revoked")
	}

	// Parse and validate token
	token, err := jwt.ParseWithClaims(tokenString, &TokenClaims{}, func(token *jwt.Token) (interface{}, error) {
		// Validate signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.jwtSecret, nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	if !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	claims, ok := token.Claims.(*TokenClaims)
	if !ok {
		return nil, fmt.Errorf("invalid token claims")
	}

	// Validate session if session ID is present
	if claims.SessionID != uuid.Nil {
		session, err := s.ValidateSession(ctx, claims.SessionID)
		if err != nil {
			return nil, fmt.Errorf("invalid session: %w", err)
		}
		if !session.IsActive {
			return nil, fmt.Errorf("session is inactive")
		}
	}

	return claims, nil
}

// RefreshToken refreshes an access token using a refresh token
func (s *Service) RefreshToken(ctx context.Context, refreshToken string) (*TokenPair, error) {
	// Validate refresh token
	claims, err := s.validateRefreshToken(refreshToken)
	if err != nil {
		return nil, fmt.Errorf("invalid refresh token: %w", err)
	}

	// Get user from database to get latest permissions
	var user models.User
	if err := s.db.Where("id = ?", claims.UserID).First(&user).Error; err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	// Generate new token pair
	return s.generateTokenPair(user, claims.SessionID)
}

// RevokeToken revokes a specific token
func (s *Service) RevokeToken(ctx context.Context, tokenString string) error {
	tokenHash := hashToken(tokenString)

	// Parse token to get expiration
	token, _ := jwt.Parse(tokenString, nil)
	var expiresAt time.Time
	if token != nil {
		if claims, ok := token.Claims.(jwt.MapClaims); ok {
			if exp, ok := claims["exp"].(float64); ok {
				expiresAt = time.Unix(int64(exp), 0)
			}
		}
	}

	if expiresAt.IsZero() {
		expiresAt = time.Now().Add(24 * time.Hour) // Default expiration
	}

	blacklistedToken := &BlacklistedToken{
		ID:        uuid.New(),
		TokenHash: tokenHash,
		ExpiresAt: expiresAt,
	}

	return s.db.Create(blacklistedToken).Error
}

// RevokeAllTokens revokes all tokens for a user
func (s *Service) RevokeAllTokens(ctx context.Context, userID uuid.UUID) error {
	// Invalidate all sessions
	return s.InvalidateAllSessions(ctx, userID)
}

// AuthenticateUser authenticates a user with email and password
func (s *Service) AuthenticateUser(ctx context.Context, email, password string) (*TokenPair, *models.User, error) {
	// Find user by email
	var user models.User
	if err := s.db.Where("email = ?", email).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil, fmt.Errorf("invalid credentials")
		}
		return nil, nil, fmt.Errorf("failed to find user: %w", err)
	}

	// Check password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, nil, fmt.Errorf("invalid credentials")
	}

	// Create session
	session, err := s.CreateSession(ctx, user.ID, "")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create session: %w", err)
	}

	// Generate token pair
	tokenPair, err := s.generateTokenPair(user, session.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate tokens: %w", err)
	}

	return tokenPair, &user, nil
}

// CheckMFA checks if user has MFA enabled
func (s *Service) CheckMFA(ctx context.Context, userID uuid.UUID) (bool, error) {
	var user struct {
		MFAEnabled bool
	}
	err := s.db.Model(&models.User{}).Where("id = ?", userID).Select("mfa_enabled").First(&user).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return false, nil
		}
		return false, fmt.Errorf("failed to check MFA: %w", err)
	}
	return user.MFAEnabled, nil
}

// generateTokenPair generates access and refresh tokens
func (s *Service) generateTokenPair(user models.User, sessionID uuid.UUID) (*TokenPair, error) {
	now := time.Now()

	// Get user permissions
	permissions, err := s.getUserPermissions(user)
	if err != nil {
		return nil, fmt.Errorf("failed to get user permissions: %w", err)
	}

	// Generate access token
	accessClaims := &TokenClaims{
		UserID:      user.ID,
		Email:       user.Email,
		Role:        user.Role,
		Permissions: permissions,
		SessionID:   sessionID,
		TokenType:   "access",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.issuer,
			Subject:   user.ID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.jwtExpirationDuration)),
			NotBefore: jwt.NewNumericDate(now),
		},
	}

	accessToken := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	accessTokenString, err := accessToken.SignedString(s.jwtSecret)
	if err != nil {
		return nil, fmt.Errorf("failed to sign access token: %w", err)
	}

	// Generate refresh token
	refreshClaims := &TokenClaims{
		UserID:    user.ID,
		SessionID: sessionID,
		TokenType: "refresh",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.issuer,
			Subject:   user.ID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.refreshExpirationDuration)),
			NotBefore: jwt.NewNumericDate(now),
		},
	}

	refreshToken := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)
	refreshTokenString, err := refreshToken.SignedString(s.refreshSecret)
	if err != nil {
		return nil, fmt.Errorf("failed to sign refresh token: %w", err)
	}

	return &TokenPair{
		AccessToken:  accessTokenString,
		RefreshToken: refreshTokenString,
		ExpiresAt:    now.Add(s.jwtExpirationDuration),
		TokenType:    "Bearer",
	}, nil
}

// validateRefreshToken validates a refresh token
func (s *Service) validateRefreshToken(tokenString string) (*TokenClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &TokenClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.refreshSecret, nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse refresh token: %w", err)
	}

	if !token.Valid {
		return nil, fmt.Errorf("invalid refresh token")
	}

	claims, ok := token.Claims.(*TokenClaims)
	if !ok {
		return nil, fmt.Errorf("invalid refresh token claims")
	}

	if claims.TokenType != "refresh" {
		return nil, fmt.Errorf("not a refresh token")
	}

	return claims, nil
}

// getUserPermissions gets user permissions based on role
func (s *Service) getUserPermissions(user models.User) ([]string, error) {
	// In a real implementation, this would query a role-permission mapping
	// For now, return basic permissions based on role
	switch user.Role {
	case "admin":
		return []string{
			"users:read", "users:write", "users:delete",
			"orders:read", "orders:write", "orders:delete",
			"trades:read", "trades:write",
			"accounts:read", "accounts:write",
			"system:admin",
		}, nil
	case "trader":
		return []string{
			"orders:read", "orders:write",
			"trades:read",
			"accounts:read",
		}, nil
	case "viewer":
		return []string{
			"orders:read",
			"trades:read",
			"accounts:read",
		}, nil
	default:
		return []string{
			"orders:read",
			"accounts:read",
		}, nil
	}
}

// hashToken creates a hash of a token for storage
func hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

// generateSecureKey generates a cryptographically secure random key
func generateSecureKey(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}
