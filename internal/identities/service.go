package identities

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/auth"
	"github.com/Aidin1998/pincex_unified/pkg/models"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// IdentityService defines user identity operations.
type IdentityService interface {
	Start() error
	Stop() error
	Register(ctx context.Context, req *models.RegisterRequest) (*models.User, error)
	Login(ctx context.Context, req *models.LoginRequest) (*models.LoginResponse, error)
	ValidateToken(token string) (string, error)
	IsAdmin(userID string) (bool, error)
}

// Service implements IdentityService
type Service struct {
	logger                 *zap.Logger
	db                     *gorm.DB
	authService            auth.AuthService
	jwtSecret              string
	jwtExpirationHours     int
	refreshSecret          string
	refreshExpirationHours int
	kycProviderURL         string
	kycProviderAPIKey      string
	kycDocumentBasePath    string
}

// NewService creates a new IdentityService
func NewService(logger *zap.Logger, db *gorm.DB, authService auth.AuthService) (IdentityService, error) {
	// Create service
	svc := &Service{
		logger:                 logger,
		db:                     db,
		authService:            authService,
		jwtSecret:              "your-secret-key", // Should be loaded from config
		jwtExpirationHours:     24,
		refreshSecret:          "your-refresh-secret-key", // Should be loaded from config
		refreshExpirationHours: 168,                       // 7 days
		kycProviderURL:         "https://kyc-provider.example.com",
		kycProviderAPIKey:      "your-kyc-provider-api-key",
		kycDocumentBasePath:    "/var/lib/pincex/kyc-documents",
	}

	return svc, nil
}

// Start starts the identities service
func (s *Service) Start() error {
	s.logger.Info("Identities service started")
	return nil
}

// Stop stops the identities service
func (s *Service) Stop() error {
	s.logger.Info("Identities service stopped")
	return nil
}

// Register registers a new user
func (s *Service) Register(ctx context.Context, req *models.RegisterRequest) (*models.User, error) {
	// Check if email already exists
	var count int64
	if err := s.db.Model(&models.User{}).Where("email = ?", req.Email).Count(&count).Error; err != nil {
		return nil, fmt.Errorf("failed to check email: %w", err)
	}
	if count > 0 {
		return nil, fmt.Errorf("email already exists")
	}

	// Check if username already exists
	if err := s.db.Model(&models.User{}).Where("username = ?", req.Username).Count(&count).Error; err != nil {
		return nil, fmt.Errorf("failed to check username: %w", err)
	}
	if count > 0 {
		return nil, fmt.Errorf("username already exists")
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	// Create user
	user := &models.User{
		ID:           uuid.New(),
		Email:        req.Email,
		Username:     req.Username,
		PasswordHash: string(hashedPassword),
		FirstName:    req.FirstName,
		LastName:     req.LastName,
		KYCStatus:    "pending",
		MFAEnabled:   false,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	// Save user to database
	if err := s.db.Create(user).Error; err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	return user, nil
}

// Login logs in a user
func (s *Service) Login(ctx context.Context, req *models.LoginRequest) (*models.LoginResponse, error) {
	// Use unified auth service if available
	if s.authService != nil {
		tokenPair, user, err := s.authService.AuthenticateUser(ctx, req.Login, req.Password)
		if err != nil {
			return nil, err
		}

		// Check if 2FA is required
		hasMFA, err := s.authService.CheckMFA(ctx, user.ID)
		if err != nil {
			s.logger.Warn("Failed to check MFA status", zap.Error(err))
		} else if hasMFA {
			return &models.LoginResponse{
				Requires2FA: true,
				UserID:      user.ID,
			}, nil
		}

		return &models.LoginResponse{
			User:        user,
			Token:       tokenPair.AccessToken,
			Requires2FA: false,
		}, nil
	}

	// Fallback to legacy authentication
	// Find user by email or username
	var user models.User
	if err := s.db.Where("email = ? OR username = ?", req.Login, req.Login).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("invalid credentials")
		}
		return nil, fmt.Errorf("failed to find user: %w", err)
	}

	// Check password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return nil, fmt.Errorf("invalid credentials")
	}

	// Check if 2FA is enabled (legacy)
	if user.MFAEnabled {
		return &models.LoginResponse{
			Requires2FA: true,
			UserID:      user.ID,
		}, nil
	}

	// Generate token (legacy)
	token, err := s.generateToken(user.ID.String())
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	// Return response
	return &models.LoginResponse{
		User:        &user,
		Token:       token,
		Requires2FA: false,
	}, nil
}

// Verify2FA verifies a 2FA token
func (s *Service) Verify2FA(ctx context.Context, userID string, token string) (*models.LoginResponse, error) {
	// Find user
	var user models.User
	if err := s.db.Where("id = ?", userID).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("failed to find user: %w", err)
	}

	// Check if 2FA is enabled
	if !user.MFAEnabled {
		return nil, fmt.Errorf("2FA not enabled")
	}

	// Verify token
	// In a real implementation, this would use a library like pquerna/otp
	if token != "123456" { // Placeholder for actual verification
		return nil, fmt.Errorf("invalid 2FA token")
	}

	// Generate token
	jwtToken, err := s.generateToken(user.ID.String())
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	// Return response
	return &models.LoginResponse{
		User:        &user,
		Token:       jwtToken,
		Requires2FA: false,
	}, nil
}

// Enable2FA enables 2FA for a user
func (s *Service) Enable2FA(ctx context.Context, userID string) (string, error) {
	// Find user
	var user models.User
	if err := s.db.Where("id = ?", userID).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", fmt.Errorf("user not found")
		}
		return "", fmt.Errorf("failed to find user: %w", err)
	}

	// Check if 2FA is already enabled
	if user.MFAEnabled {
		return "", fmt.Errorf("2FA already enabled")
	}

	// Generate 2FA secret
	secret := make([]byte, 20)
	if _, err := rand.Read(secret); err != nil {
		return "", fmt.Errorf("failed to generate secret: %w", err)
	}
	secretBase32 := base64.StdEncoding.EncodeToString(secret)

	// Save 2FA secret
	user.TOTPSecret = secretBase32
	if err := s.db.Save(&user).Error; err != nil {
		return "", fmt.Errorf("failed to save user: %w", err)
	}

	// Return secret
	return secretBase32, nil
}

// Verify2FASetup verifies a 2FA setup
func (s *Service) Verify2FASetup(ctx context.Context, userID string, token string) error {
	// Find user
	var user models.User
	if err := s.db.Where("id = ?", userID).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("user not found")
		}
		return fmt.Errorf("failed to find user: %w", err)
	}

	// Check if 2FA is already enabled
	if user.MFAEnabled {
		return fmt.Errorf("2FA already enabled")
	}

	// Verify token
	// In a real implementation, this would use a library like pquerna/otp
	if token != "123456" { // Placeholder for actual verification
		return fmt.Errorf("invalid 2FA token")
	}

	// Enable 2FA
	user.MFAEnabled = true
	if err := s.db.Save(&user).Error; err != nil {
		return fmt.Errorf("failed to save user: %w", err)
	}

	return nil
}

// Disable2FA disables 2FA for a user
func (s *Service) Disable2FA(ctx context.Context, userID string, token string) error {
	// Find user
	var user models.User
	if err := s.db.Where("id = ?", userID).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("user not found")
		}
		return fmt.Errorf("failed to find user: %w", err)
	}

	// Check if 2FA is enabled
	if !user.MFAEnabled {
		return fmt.Errorf("2FA not enabled")
	}

	// Verify token
	// In a real implementation, this would use a library like pquerna/otp
	if token != "123456" { // Placeholder for actual verification
		return fmt.Errorf("invalid 2FA token")
	}

	// Disable 2FA
	user.MFAEnabled = false
	user.TOTPSecret = ""
	if err := s.db.Save(&user).Error; err != nil {
		return fmt.Errorf("failed to save user: %w", err)
	}

	return nil
}

// GetUser gets a user by ID
func (s *Service) GetUser(ctx context.Context, userID string) (*models.User, error) {
	// Find user
	var user models.User
	if err := s.db.Where("id = ?", userID).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("failed to find user: %w", err)
	}

	return &user, nil
}

// UpdateUser updates a user
func (s *Service) UpdateUser(ctx context.Context, userID string, firstName, lastName string) (*models.User, error) {
	// Find user
	var user models.User
	if err := s.db.Where("id = ?", userID).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("failed to find user: %w", err)
	}

	// Update user
	user.FirstName = firstName
	user.LastName = lastName
	user.UpdatedAt = time.Now()
	if err := s.db.Save(&user).Error; err != nil {
		return nil, fmt.Errorf("failed to save user: %w", err)
	}

	return &user, nil
}

// SubmitKYC submits KYC documents for a user
func (s *Service) SubmitKYC(ctx context.Context, userID string, documentType string, filePath string) error {
	// Find user
	var user models.User
	if err := s.db.Where("id = ?", userID).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("user not found")
		}
		return fmt.Errorf("failed to find user: %w", err)
	}

	// Create KYC document
	document := &models.KYCDocument{
		ID:        uuid.New(),
		UserID:    user.ID,
		DocType:   documentType,
		Status:    "pending",
		DocNumber: "", // Not available here
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Save document to database
	if err := s.db.Create(document).Error; err != nil {
		return fmt.Errorf("failed to create document: %w", err)
	}

	// Update user KYC status
	user.KYCStatus = "pending"
	user.UpdatedAt = time.Now()
	if err := s.db.Save(&user).Error; err != nil {
		return fmt.Errorf("failed to save user: %w", err)
	}

	// Submit to KYC provider
	// In a real implementation, this would call the KYC provider API
	s.logger.Info("Submitting KYC to provider", zap.String("userID", userID), zap.String("documentType", documentType))

	return nil
}

// GetKYCStatus gets the KYC status for a user
func (s *Service) GetKYCStatus(ctx context.Context, userID string) (string, error) {
	// Find user
	var user models.User
	if err := s.db.Where("id = ?", userID).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", fmt.Errorf("user not found")
		}
		return "", fmt.Errorf("failed to find user: %w", err)
	}

	return user.KYCStatus, nil
}

// UpdateKYCStatus updates the KYC status for a user
func (s *Service) UpdateKYCStatus(ctx context.Context, userID string, status string) error {
	// Find user
	var user models.User
	if err := s.db.Where("id = ?", userID).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("user not found")
		}
		return fmt.Errorf("failed to find user: %w", err)
	}

	// Update user KYC status
	user.KYCStatus = status
	user.UpdatedAt = time.Now()
	if err := s.db.Save(&user).Error; err != nil {
		return fmt.Errorf("failed to save user: %w", err)
	}

	return nil
}

// ValidateToken validates a JWT token
func (s *Service) ValidateToken(tokenString string) (string, error) {
	// Use unified auth service if available
	if s.authService != nil {
		ctx := context.Background()
		claims, err := s.authService.ValidateToken(ctx, tokenString)
		if err != nil {
			return "", err
		}
		return claims.UserID.String(), nil
	}

	// Fallback to legacy token validation
	// Parse token
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		// Validate signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.jwtSecret), nil
	})
	if err != nil {
		return "", fmt.Errorf("failed to parse token: %w", err)
	}

	// Validate token
	if !token.Valid {
		return "", fmt.Errorf("invalid token")
	}

	// Get claims
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", fmt.Errorf("invalid token claims")
	}

	// Get user ID
	userID, ok := claims["sub"].(string)
	if !ok {
		return "", fmt.Errorf("invalid user ID")
	}

	return userID, nil
}

// IsAdmin checks if a user is an admin
func (s *Service) IsAdmin(userID string) (bool, error) {
	// In a real implementation, this would check if the user has admin privileges
	// For now, we'll just return false
	return false, nil
}

// generateToken generates a JWT token
func (s *Service) generateToken(userID string) (string, error) {
	// Create token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": userID,
		"exp": time.Now().Add(time.Hour * time.Duration(s.jwtExpirationHours)).Unix(),
	})

	// Sign token
	tokenString, err := token.SignedString([]byte(s.jwtSecret))
	if err != nil {
		return "", fmt.Errorf("failed to sign token: %w", err)
	}

	return tokenString, nil
}
