// filepath: c:\Orbit CEX\Finalex\internal\userauth\2FA\service.go
package twofa

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image/png"
	"strings"
	"time"

	"bytes"

	"github.com/Aidin1998/finalex/internal/userauth/models"
	"github.com/google/uuid"
	"github.com/pquerna/otp/totp"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// TwoFactorType represents different types of 2FA methods
type TwoFactorType string

const (
	TwoFactorTypeTOTP   TwoFactorType = "totp"
	TwoFactorTypeSMS    TwoFactorType = "sms"
	TwoFactorTypeEmail  TwoFactorType = "email"
	TwoFactorTypeBackup TwoFactorType = "backup"
)

// Service provides two-factor authentication services
type Service struct {
	db     *gorm.DB
	logger *zap.Logger
	issuer string
}

// NewService creates a new 2FA service
func NewService(logger *zap.Logger, db *gorm.DB, issuer string) *Service {
	return &Service{
		db:     db,
		logger: logger,
		issuer: issuer,
	}
}

// EnableTOTP enables TOTP-based 2FA for a user
func (s *Service) EnableTOTP(ctx context.Context, userID uuid.UUID, userEmail string) (*models.TwoFactorAuth, []byte, error) {
	// Check if user already has TOTP enabled
	var existing models.TwoFactorAuth
	err := s.db.WithContext(ctx).Where("user_id = ? AND is_enabled = ?", userID, true).First(&existing).Error
	if err == nil {
		return nil, nil, fmt.Errorf("TOTP already enabled for user")
	}

	// Generate TOTP key
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      s.issuer,
		AccountName: userEmail,
		SecretSize:  32,
	})
	if err != nil {
		s.logger.Error("Failed to generate TOTP key", zap.Error(err))
		return nil, nil, fmt.Errorf("failed to generate TOTP key: %w", err)
	}

	// Generate QR code
	var buf bytes.Buffer
	img, err := key.Image(200, 200)
	if err != nil {
		s.logger.Error("Failed to generate QR code", zap.Error(err))
		return nil, nil, fmt.Errorf("failed to generate QR code: %w", err)
	}

	if err := png.Encode(&buf, img); err != nil {
		s.logger.Error("Failed to encode QR code", zap.Error(err))
		return nil, nil, fmt.Errorf("failed to encode QR code: %w", err)
	}

	// Generate backup codes
	backupCodes, err := s.generateBackupCodes()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate backup codes: %w", err)
	}

	// Hash backup codes before storing
	hashedBackupCodes := make([]string, len(backupCodes))
	for i, code := range backupCodes {
		hash := sha256.Sum256([]byte(code))
		hashedBackupCodes[i] = hex.EncodeToString(hash[:])
	}

	twoFA := &models.TwoFactorAuth{
		ID:          uuid.New(),
		UserID:      userID,
		TOTPSecret:  key.Secret(),
		BackupCodes: strings.Join(hashedBackupCodes, ","),
		IsEnabled:   false, // Will be activated after verification
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := s.db.WithContext(ctx).Create(twoFA).Error; err != nil {
		s.logger.Error("Failed to create 2FA record", zap.Error(err))
		return nil, nil, fmt.Errorf("failed to create 2FA record: %w", err)
	}

	s.logger.Info("TOTP setup initiated", zap.String("user_id", userID.String()))

	// Return the plain backup codes to show to user (only time they'll see them)
	return twoFA, buf.Bytes(), nil
}

// VerifyTOTPSetup verifies the TOTP setup with a code from the user
func (s *Service) VerifyTOTPSetup(ctx context.Context, userID uuid.UUID, code string) error {
	var twoFA models.TwoFactorAuth
	err := s.db.WithContext(ctx).Where("user_id = ? AND is_enabled = ?", userID, false).First(&twoFA).Error
	if err != nil {
		return fmt.Errorf("TOTP setup not found or already activated")
	}

	// Verify the code
	valid := totp.Validate(code, twoFA.TOTPSecret)
	if !valid {
		s.logger.Warn("Invalid TOTP code during setup", zap.String("user_id", userID.String()))
		return fmt.Errorf("invalid TOTP code")
	}

	// Activate 2FA
	if err := s.db.WithContext(ctx).Model(&twoFA).Updates(map[string]interface{}{
		"is_enabled": true,
		"updated_at": time.Now(),
	}).Error; err != nil {
		s.logger.Error("Failed to activate 2FA", zap.Error(err))
		return fmt.Errorf("failed to activate 2FA: %w", err)
	}

	s.logger.Info("TOTP activated", zap.String("user_id", userID.String()))
	return nil
}

// VerifyTOTP verifies a TOTP code for authentication
func (s *Service) VerifyTOTP(ctx context.Context, userID uuid.UUID, code string) (bool, error) {
	var twoFA models.TwoFactorAuth
	err := s.db.WithContext(ctx).Where("user_id = ? AND is_enabled = ?", userID, true).First(&twoFA).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return false, fmt.Errorf("2FA not enabled for user")
		}
		return false, fmt.Errorf("failed to get 2FA record: %w", err)
	}

	// Check if this is a backup code
	if len(code) == 10 && strings.ContainsAny(code, "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789") {
		return s.verifyBackupCode(ctx, &twoFA, code)
	}

	// Verify TOTP code
	valid := totp.Validate(code, twoFA.TOTPSecret)
	if valid {
		s.logger.Info("TOTP verification successful", zap.String("user_id", userID.String()))
	} else {
		s.logger.Warn("TOTP verification failed", zap.String("user_id", userID.String()))
	}

	return valid, nil
}

// verifyBackupCode verifies and consumes a backup code
func (s *Service) verifyBackupCode(ctx context.Context, twoFA *models.TwoFactorAuth, code string) (bool, error) {
	// Hash the provided code
	hash := sha256.Sum256([]byte(code))
	hashedCode := hex.EncodeToString(hash[:])

	// Get current backup codes
	backupCodes := strings.Split(twoFA.BackupCodes, ",")

	// Find and remove the used code
	var remainingCodes []string
	found := false
	for _, storedCode := range backupCodes {
		if storedCode == hashedCode {
			found = true
			// Don't add to remaining codes (consume it)
		} else {
			remainingCodes = append(remainingCodes, storedCode)
		}
	}

	if !found {
		s.logger.Warn("Invalid backup code", zap.String("user_id", twoFA.UserID.String()))
		return false, nil
	}

	// Update the 2FA record with remaining codes
	if err := s.db.WithContext(ctx).Model(twoFA).Updates(map[string]interface{}{
		"backup_codes": strings.Join(remainingCodes, ","),
		"updated_at":   time.Now(),
	}).Error; err != nil {
		s.logger.Error("Failed to update backup codes", zap.Error(err))
		return false, fmt.Errorf("failed to update backup codes: %w", err)
	}

	s.logger.Info("Backup code used",
		zap.String("user_id", twoFA.UserID.String()),
		zap.Int("remaining_codes", len(remainingCodes)))

	return true, nil
}

// GenerateNewBackupCodes generates new backup codes for a user
func (s *Service) GenerateNewBackupCodes(ctx context.Context, userID uuid.UUID) ([]string, error) {
	var twoFA models.TwoFactorAuth
	err := s.db.WithContext(ctx).Where("user_id = ? AND is_enabled = ?", userID, true).First(&twoFA).Error
	if err != nil {
		return nil, fmt.Errorf("2FA not enabled for user")
	}

	// Generate new backup codes
	backupCodes, err := s.generateBackupCodes()
	if err != nil {
		return nil, fmt.Errorf("failed to generate backup codes: %w", err)
	}

	// Hash backup codes before storing
	hashedBackupCodes := make([]string, len(backupCodes))
	for i, code := range backupCodes {
		hash := sha256.Sum256([]byte(code))
		hashedBackupCodes[i] = hex.EncodeToString(hash[:])
	}

	// Update the 2FA record
	if err := s.db.WithContext(ctx).Model(&twoFA).Updates(map[string]interface{}{
		"backup_codes": strings.Join(hashedBackupCodes, ","),
		"updated_at":   time.Now(),
	}).Error; err != nil {
		s.logger.Error("Failed to update backup codes", zap.Error(err))
		return nil, fmt.Errorf("failed to update backup codes: %w", err)
	}

	s.logger.Info("New backup codes generated", zap.String("user_id", userID.String()))
	return backupCodes, nil
}

// DisableTwoFA disables 2FA for a user
func (s *Service) DisableTwoFA(ctx context.Context, userID uuid.UUID) error {
	result := s.db.WithContext(ctx).Model(&models.TwoFactorAuth{}).
		Where("user_id = ? AND is_enabled = ?", userID, true).
		Updates(map[string]interface{}{
			"is_enabled": false,
			"updated_at": time.Now(),
		})

	if result.Error != nil {
		s.logger.Error("Failed to disable 2FA", zap.Error(result.Error))
		return fmt.Errorf("failed to disable 2FA: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("no active 2FA found for user")
	}

	s.logger.Info("2FA disabled", zap.String("user_id", userID.String()))
	return nil
}

// IsTwoFactorEnabled checks if 2FA is enabled for a user
func (s *Service) IsTwoFactorEnabled(ctx context.Context, userID uuid.UUID) (bool, error) {
	var count int64
	err := s.db.WithContext(ctx).Model(&models.TwoFactorAuth{}).
		Where("user_id = ? AND is_enabled = ?", userID, true).
		Count(&count).Error

	if err != nil {
		return false, fmt.Errorf("failed to check 2FA status: %w", err)
	}

	return count > 0, nil
}

// GetTwoFactorMethods returns all active 2FA methods for a user
func (s *Service) GetTwoFactorMethods(ctx context.Context, userID uuid.UUID) ([]models.TwoFactorAuth, error) {
	var methods []models.TwoFactorAuth
	err := s.db.WithContext(ctx).Where("user_id = ? AND is_enabled = ?", userID, true).Find(&methods).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get 2FA methods: %w", err)
	}

	// Don't return secrets in the response
	for i := range methods {
		methods[i].TOTPSecret = ""
		methods[i].BackupCodes = ""
	}

	return methods, nil
}

// EnforceTwoFactor checks if 2FA is required and enforces it
func (s *Service) EnforceTwoFactor(ctx context.Context, userID uuid.UUID, gracePerriodDays int) (bool, error) {
	// Check if user has 2FA enabled
	enabled, err := s.IsTwoFactorEnabled(ctx, userID)
	if err != nil {
		return false, fmt.Errorf("failed to check 2FA status: %w", err)
	}

	if enabled {
		return true, nil // 2FA is already enabled
	}

	// Check if user is within grace period
	var userProfile models.UserProfile
	err = s.db.WithContext(ctx).Where("user_id = ?", userID).First(&userProfile).Error
	if err != nil {
		return false, fmt.Errorf("failed to get user profile: %w", err)
	}

	graceEndDate := userProfile.CreatedAt.AddDate(0, 0, gracePerriodDays)
	if time.Now().Before(graceEndDate) {
		return true, nil // Still within grace period
	}

	return false, fmt.Errorf("2FA is required but not enabled")
}

// generateBackupCodes generates secure backup codes
func (s *Service) generateBackupCodes() ([]string, error) {
	const numCodes = 10
	const codeLength = 10
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

	codes := make([]string, numCodes)
	for i := 0; i < numCodes; i++ {
		code := make([]byte, codeLength)
		for j := 0; j < codeLength; j++ {
			randomBytes := make([]byte, 1)
			if _, err := rand.Read(randomBytes); err != nil {
				return nil, fmt.Errorf("failed to generate random bytes: %w", err)
			}
			code[j] = charset[randomBytes[0]%byte(len(charset))]
		}
		codes[i] = string(code)
	}

	return codes, nil
}

// GetBackupCodeCount returns the number of remaining backup codes for a user
func (s *Service) GetBackupCodeCount(ctx context.Context, userID uuid.UUID) (int, error) {
	var twoFA models.TwoFactorAuth
	err := s.db.WithContext(ctx).Where("user_id = ? AND is_enabled = ?", userID, true).First(&twoFA).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to get 2FA record: %w", err)
	}

	if twoFA.BackupCodes == "" {
		return 0, nil
	}

	codes := strings.Split(twoFA.BackupCodes, ",")
	return len(codes), nil
}

// LEGACY/DEPRECATED: This file is not needed for the enterprise model and can be safely deleted.
