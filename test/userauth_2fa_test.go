//go:build userauth

package test

import (
	"context"
	"testing"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/userauth"
	usermodels "github.com/Aidin1998/pincex_unified/internal/userauth/models"
	"github.com/Aidin1998/pincex_unified/pkg/models"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// Mock implementation for 2FA services
type mock2FAService struct{}

func (m *mock2FAService) GenerateTOTPSecret(ctx context.Context, userID uuid.UUID) (*TOTPSetup, error) {
	return &TOTPSetup{
		Secret:    "ABCDEFGHIJKLMNOP",
		QRCodeURL: "otpauth://totp/Example:alice@example.com?secret=ABCDEFGHIJKLMNOP&issuer=Example",
	}, nil
}

func (m *mock2FAService) VerifyTOTP(ctx context.Context, userID uuid.UUID, code string) (bool, error) {
	// For testing purposes, accept "123456" as a valid code
	return code == "123456", nil
}

func (m *mock2FAService) GenerateBackupCodes(ctx context.Context, userID uuid.UUID) ([]string, error) {
	return []string{
		"12345-12345",
		"23456-23456",
		"34567-34567",
		"45678-45678",
		"56789-56789",
	}, nil
}

func (m *mock2FAService) VerifyBackupCode(ctx context.Context, userID uuid.UUID, code string) (bool, error) {
	validCodes := []string{
		"12345-12345",
		"23456-23456",
		"34567-34567",
		"45678-45678",
		"56789-56789",
	}

	for _, validCode := range validCodes {
		if validCode == code {
			return true, nil
		}
	}

	return false, nil
}

func setup2FATestEnvironment(t *testing.T) (*gorm.DB, uuid.UUID) {
	// Set up the database
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	// Migrate tables
	err = db.AutoMigrate(
		&usermodels.UserProfile{},
		&usermodels.TwoFactorAuth{},
		&usermodels.DeviceFingerprint{},
		&usermodels.PasswordPolicy{},
		&models.User{},
	)
	if err != nil {
		t.Fatalf("failed to migrate tables: %v", err)
	}

	// Create a test user
	userID := uuid.New()
	user := models.User{
		ID:           userID,
		Email:        "testuser@example.com",
		Username:     "testuser",
		PasswordHash: "$2a$10$abcdefghijklmnopqrstuvwxyz0123456789", // Mocked bcrypt hash
		FirstName:    "Test",
		LastName:     "User",
		KYCStatus:    "pending",
		Role:         "user",
		Tier:         "basic",
		MFAEnabled:   false,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	return db, userID
}

func TestGenerateTOTPSecret(t *testing.T) {
	db, userID := setup2FATestEnvironment(t)

	// Create a mock 2FA service
	mock2FA := &mock2FAService{}

	// Test generating TOTP secret
	setup, err := mock2FA.GenerateTOTPSecret(context.Background(), userID)

	assert.NoError(t, err, "Expected no error when generating TOTP secret")
	assert.NotEmpty(t, setup.Secret, "Expected non-empty secret")
	assert.NotEmpty(t, setup.QRCodeURL, "Expected non-empty QR code URL")
	// Store the 2FA setup in the database
	twoFactorAuth := usermodels.TwoFactorAuth{
		ID:         uuid.New(),
		UserID:     userID,
		TOTPSecret: setup.Secret, // Using TOTPSecret instead of Secret
		IsEnabled:  false,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	err = db.Create(&twoFactorAuth).Error
	assert.NoError(t, err, "Expected no error when storing 2FA setup")
	// Verify the 2FA setup was stored correctly
	var storedTwoFactor usermodels.TwoFactorAuth
	err = db.Where("user_id = ?", userID).First(&storedTwoFactor).Error
	assert.NoError(t, err, "Expected no error when retrieving 2FA setup")
	assert.Equal(t, setup.Secret, storedTwoFactor.TOTPSecret, "Stored TOTP secret should match generated secret")
}

func TestVerifyTOTP(t *testing.T) {
	_, userID := setup2FATestEnvironment(t)

	// Create a mock 2FA service
	mock2FA := &mock2FAService{}

	// Verify with correct code
	valid, err := mock2FA.VerifyTOTP(context.Background(), userID, "123456")
	assert.NoError(t, err, "Expected no error with valid code")
	assert.True(t, valid, "Expected valid code to be verified")

	// Verify with incorrect code
	valid, err = mock2FA.VerifyTOTP(context.Background(), userID, "654321")
	assert.NoError(t, err, "Expected no error with invalid code")
	assert.False(t, valid, "Expected invalid code to fail verification")
}

func TestBackupCodes(t *testing.T) {
	db, userID := setup2FATestEnvironment(t)

	// Create a mock 2FA service
	mock2FA := &mock2FAService{}

	// Generate backup codes
	codes, err := mock2FA.GenerateBackupCodes(context.Background(), userID)
	assert.NoError(t, err, "Expected no error when generating backup codes")
	assert.Len(t, codes, 5, "Expected 5 backup codes")
	// Store backup codes (we would hash these in a real implementation)
	for _, code := range codes {
		backupCode := usermodels.TwoFactorAuth{
			ID:         uuid.New(),
			UserID:     userID,
			TOTPSecret: code, // In real implementation, this would be hashed
			BackupCodes: "", // Empty since we're storing each code separately
			IsEnabled:   false,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		err = db.Create(&backupCode).Error
		assert.NoError(t, err, "Expected no error when storing backup code")
	}

	// Verify backup codes were stored
	var storedBackupCodes []usermodels.TwoFactorAuth
	err = db.Where("user_id = ? AND type = 'backup_code'", userID).Find(&storedBackupCodes).Error
	assert.NoError(t, err, "Expected no error when retrieving backup codes")
	assert.Len(t, storedBackupCodes, 5, "Expected 5 stored backup codes")

	// Verify a backup code works
	valid, err := mock2FA.VerifyBackupCode(context.Background(), userID, "12345-12345")
	assert.NoError(t, err, "Expected no error when verifying valid backup code")
	assert.True(t, valid, "Expected backup code to be valid")

	// Verify an invalid backup code fails
	valid, err = mock2FA.VerifyBackupCode(context.Background(), userID, "invalid-code")
	assert.NoError(t, err, "Expected no error when verifying invalid backup code")
	assert.False(t, valid, "Expected invalid backup code to fail verification")
}

func TestEnableMFA(t *testing.T) {
	db, userID := setup2FATestEnvironment(t)

	// Get user before enabling MFA
	var user models.User
	err := db.First(&user, "id = ?", userID).Error
	assert.NoError(t, err, "Expected no error retrieving user")
	assert.False(t, user.MFAEnabled, "MFA should be disabled initially")

	// Enable MFA
	err = db.Model(&models.User{}).Where("id = ?", userID).Update("mfa_enabled", true).Error
	assert.NoError(t, err, "Expected no error enabling MFA")

	// Verify MFA was enabled
	err = db.First(&user, "id = ?", userID).Error
	assert.NoError(t, err, "Expected no error retrieving updated user")
	assert.True(t, user.MFAEnabled, "MFA should be enabled")
}

// TOTPSetup is now defined in the mock file at test/userauth_mocks.go
