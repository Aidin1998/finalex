//go:build userauth && integration

package test

import (
	"context"
	"testing"
	"time"

	"github.com/Aidin1998/finalex/internal/userauth"
	"github.com/Aidin1998/finalex/internal/userauth/auth"
	usermodels "github.com/Aidin1998/finalex/internal/userauth/models"
	"github.com/Aidin1998/finalex/pkg/models"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// IntegrationTest represents a complete integration test environment
// with all necessary services and dependencies
type IntegrationTest struct {
	DB                   *gorm.DB
	RegistrationService  *userauth.EnterpriseRegistrationService
	AuthService          *IntegratedAuthService
	TwoFAService         *mock2FAService
	EncryptionService    *mockEncryptionService
	AuditService         *mockAuditService
	ComplianceService    *mockComplianceService
	PasswordPolicyEngine *mockPasswordPolicyEngine
	KYCService           *mockKYCIntegrationService
	NotificationService  *mockNotificationService

	// User tokens and IDs for testing various flows
	UserID      uuid.UUID
	AdminID     uuid.UUID
	UserTokens  *auth.TokenPair
	AdminTokens *auth.TokenPair
}

// IntegratedAuthService manages authentication for integrated testing
type IntegratedAuthService struct {
	db *gorm.DB
}

func NewIntegratedAuthService(db *gorm.DB) *IntegratedAuthService {
	return &IntegratedAuthService{db: db}
}

func (s *IntegratedAuthService) AuthenticateUser(ctx context.Context, email, password string) (*auth.TokenPair, *models.User, error) {
	var user models.User
	if err := s.db.WithContext(ctx).Where("email = ?", email).First(&user).Error; err != nil {
		return nil, nil, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, nil, err
	}

	// Create a token pair
	accessToken := uuid.New().String()
	refreshToken := uuid.New().String()

	tokenPair := &auth.TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    3600,
	}

	return tokenPair, &user, nil
}

func (s *IntegratedAuthService) ValidateToken(ctx context.Context, tokenString string) (*auth.TokenClaims, error) { // In real implementation, this would validate JWT tokens
	// For testing, we'll just parse out the user ID from our test tokens

	// In a real scenario, you'd validate the JWT signature, check expiry, etc.
	// Parse the token as UUID for proper type matching
	userID, err := uuid.Parse(tokenString)
	if err != nil {
		// Fallback to a default UUID for testing
		userID = uuid.New()
	}

	return &auth.TokenClaims{
		UserID: userID, // Use proper UUID type
		Email:  "testuser@example.com",
		Role:   "user",
	}, nil
}

// SetupIntegrationTest creates a full test environment
func SetupIntegrationTest(t *testing.T) *IntegrationTest {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err, "Failed to create test database")

	// Migrate all required tables
	err = db.AutoMigrate(
		&models.User{},
		&usermodels.UserProfile{},
		&usermodels.TwoFactorAuth{},
		&usermodels.DeviceFingerprint{},
		&usermodels.PasswordPolicy{},
		&usermodels.UserSession{},
		&usermodels.UserAuditLog{},
	)
	require.NoError(t, err, "Failed to migrate tables")

	// Create mock services
	encryptionSvc := &mockEncryptionService{}
	auditSvc := &mockAuditService{}
	complianceSvc := &mockComplianceService{}
	passwordSvc := &mockPasswordPolicyEngine{}
	kycSvc := &mockKYCIntegrationService{}
	notificationSvc := &mockNotificationService{}
	twoFASvc := &mock2FAService{}

	// Create auth service
	authSvc := NewIntegratedAuthService(db)

	// Create registration service
	regSvc := userauth.NewEnterpriseRegistrationService(
		db,
		nil,
		encryptionSvc,
		complianceSvc,
		auditSvc,
		passwordSvc,
		kycSvc,
		notificationSvc,
	)

	// Create test instance
	test := &IntegrationTest{
		DB:                   db,
		RegistrationService:  regSvc,
		AuthService:          authSvc,
		TwoFAService:         twoFASvc,
		EncryptionService:    encryptionSvc,
		AuditService:         auditSvc,
		ComplianceService:    complianceSvc,
		PasswordPolicyEngine: passwordSvc,
		KYCService:           kycSvc,
		NotificationService:  notificationSvc,
	}

	// Create a test user and admin
	test.setupTestUsers(t)

	return test
}

// setupTestUsers creates test users for integration tests
func (it *IntegrationTest) setupTestUsers(t *testing.T) {
	// Create user with password "Password123!"
	userPasswordHash, _ := bcrypt.GenerateFromPassword([]byte("Password123!"), bcrypt.DefaultCost)
	it.UserID = uuid.New()
	user := models.User{
		ID:           it.UserID,
		Email:        "user@example.com",
		Username:     "testuser",
		PasswordHash: string(userPasswordHash),
		FirstName:    "Test",
		LastName:     "User",
		KYCStatus:    "approved",
		Role:         "user",
		Tier:         "basic",
		MFAEnabled:   false,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	err := it.DB.Create(&user).Error
	require.NoError(t, err, "Failed to create test user")

	// Create admin with password "AdminPass123!"
	adminPasswordHash, _ := bcrypt.GenerateFromPassword([]byte("AdminPass123!"), bcrypt.DefaultCost)
	it.AdminID = uuid.New()
	admin := models.User{
		ID:           it.AdminID,
		Email:        "admin@example.com",
		Username:     "testadmin",
		PasswordHash: string(adminPasswordHash),
		FirstName:    "Admin",
		LastName:     "User",
		KYCStatus:    "approved",
		Role:         "admin",
		Tier:         "premium",
		MFAEnabled:   false,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	err = it.DB.Create(&admin).Error
	require.NoError(t, err, "Failed to create admin user")

	// Generate tokens for both users
	it.UserTokens = &auth.TokenPair{
		AccessToken:  "user_access_token",
		RefreshToken: "user_refresh_token",
		ExpiresIn:    3600,
	}

	it.AdminTokens = &auth.TokenPair{
		AccessToken:  "admin_access_token",
		RefreshToken: "admin_refresh_token",
		ExpiresIn:    3600,
	}
}

// TestFullRegistrationAndAuthFlow tests the complete user registration and authentication flow
func TestFullRegistrationAndAuthFlow(t *testing.T) {
	it := SetupIntegrationTest(t)

	// 1. Register a new user
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	dob := time.Now().AddDate(-25, 0, 0)
	regReq := &userauth.EnterpriseRegistrationRequest{
		Email:                 "newuser@example.com",
		Username:              "newuser",
		Password:              "SecurePass123!",
		FirstName:             "New",
		LastName:              "User",
		PhoneNumber:           "+12345678901",
		DateOfBirth:           &dob,
		Country:               "USA",
		AddressLine1:          "123 Main St",
		AddressLine2:          "Apt 4B",
		City:                  "Metropolis",
		State:                 "NY",
		PostalCode:            "10001",
		AcceptTerms:           true,
		AcceptPrivacyPolicy:   true,
		AcceptKYCRequirements: true,
		MarketingConsent:      false,
		ReferralCode:          "",
		PreferredLanguage:     "en",
		Timezone:              "UTC",
		DeviceFingerprint:     "devicefp123",
		UserAgent:             "Mozilla/5.0",
		IPAddress:             "127.0.0.1",
		GeolocationData:       map[string]interface{}{"lat": 40.7128, "lon": -74.0060},
	}

	regResp, err := it.RegistrationService.RegisterUser(ctx, regReq)
	require.NoError(t, err, "Registration should succeed")
	require.NotNil(t, regResp, "Registration response should not be nil")
	require.NotEqual(t, uuid.Nil, regResp.UserID, "User ID should be valid")

	// 2. Verify user was created in database
	var newUser models.User
	err = it.DB.Where("email = ?", regReq.Email).First(&newUser).Error
	require.NoError(t, err, "Should find newly registered user")
	assert.Equal(t, regReq.Email, newUser.Email)
	assert.Equal(t, regReq.Username, newUser.Username)

	// 3. Authenticate as the new user
	tokenPair, user, err := it.AuthService.AuthenticateUser(ctx, regReq.Email, regReq.Password)
	require.NoError(t, err, "Authentication should succeed")
	require.NotNil(t, tokenPair, "Token pair should not be nil")
	require.NotNil(t, user, "User should not be nil")
	assert.Equal(t, regReq.Email, user.Email)

	// 4. Enable 2FA for the user
	tokenSetup, err := it.TwoFAService.GenerateTOTPSecret(ctx, newUser.ID)
	require.NoError(t, err, "Should generate 2FA secret")
	require.NotEmpty(t, tokenSetup.Secret, "TOTP secret should not be empty")

	// 5. Save 2FA setup to database
	twoFactor := usermodels.TwoFactorAuth{
		UserID:    newUser.ID,
		Secret:    tokenSetup.Secret,
		Type:      "totp",
		Verified:  false,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = it.DB.Create(&twoFactor).Error
	require.NoError(t, err, "Should save 2FA setup")

	// 6. Verify TOTP
	isValid, err := it.TwoFAService.VerifyTOTP(ctx, newUser.ID, "123456")
	require.NoError(t, err)
	assert.True(t, isValid, "TOTP verification should succeed with our mock")

	// 7. Enable MFA flag on user
	err = it.DB.Model(&models.User{}).Where("id = ?", newUser.ID).Update("mfa_enabled", true).Error
	require.NoError(t, err, "Should enable MFA flag")

	// 8. Verify user has MFA enabled
	err = it.DB.First(&newUser, "id = ?", newUser.ID).Error
	require.NoError(t, err)
	assert.True(t, newUser.MFAEnabled, "User should have MFA enabled")

	// 9. Generate backup codes
	backupCodes, err := it.TwoFAService.GenerateBackupCodes(ctx, newUser.ID)
	require.NoError(t, err)
	require.NotEmpty(t, backupCodes, "Should generate backup codes")

	// 10. Store backup codes
	for _, code := range backupCodes {
		backupCode := usermodels.TwoFactorAuth{
			UserID:    newUser.ID,
			Secret:    code, // In a real system, we would hash this
			Type:      "backup_code",
			Verified:  false,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		err = it.DB.Create(&backupCode).Error
		require.NoError(t, err, "Should save backup code")
	}

	// 11. Verify backup codes are stored
	var storedBackupCodes []usermodels.TwoFactorAuth
	err = it.DB.Where("user_id = ? AND type = 'backup_code'", newUser.ID).Find(&storedBackupCodes).Error
	require.NoError(t, err)
	assert.Equal(t, len(backupCodes), len(storedBackupCodes), "All backup codes should be stored")
}

// TestUserProfileAndKYC tests the user profile management and KYC flows
func TestUserProfileAndKYC(t *testing.T) {
	it := SetupIntegrationTest(t)

	// Create a test user with profile
	ctx := context.Background()
	userID := uuid.New()
	dob := time.Now().AddDate(-30, 0, 0)

	// 1. Create User
	passwordHash, _ := bcrypt.GenerateFromPassword([]byte("TestPassword123!"), bcrypt.DefaultCost)
	user := models.User{
		ID:           userID,
		Email:        "kycuser@example.com",
		Username:     "kycuser",
		PasswordHash: string(passwordHash),
		FirstName:    "KYC",
		LastName:     "User",
		KYCStatus:    "pending",
		Role:         "user",
		Tier:         "basic",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	err := it.DB.Create(&user).Error
	require.NoError(t, err)

	// 2. Create User Profile
	profile := usermodels.UserProfile{
		UserID:           userID,
		DateOfBirth:      &dob,
		CountryOfBirth:   "USA",
		Nationality:      "US",
		ResidenceCountry: "USA",
		PhoneNumber:      "+12345678901",
		AddressLine1:     "123 KYC Street",
		City:             "KYC City",
		State:            "NY",
		PostalCode:       "10001",
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	err = it.DB.Create(&profile).Error
	require.NoError(t, err)

	// 3. Start KYC process
	err = it.KYCService.InitializeKYCProcess(ctx, userID, "basic")
	require.NoError(t, err)

	// 4. Update KYC status to approved
	err = it.DB.Model(&models.User{}).Where("id = ?", userID).Update("kyc_status", "approved").Error
	require.NoError(t, err)

	// 5. Verify KYC status
	var updatedUser models.User
	err = it.DB.First(&updatedUser, "id = ?", userID).Error
	require.NoError(t, err)
	assert.Equal(t, "approved", updatedUser.KYCStatus)

	// 6. Update user profile
	err = it.DB.Model(&usermodels.UserProfile{}).Where("user_id = ?", userID).
		Updates(map[string]interface{}{
			"phone_number":  "+19876543210",
			"address_line1": "456 Updated Street",
		}).Error
	require.NoError(t, err)

	// 7. Verify profile updates
	var updatedProfile usermodels.UserProfile
	err = it.DB.First(&updatedProfile, "user_id = ?", userID).Error
	require.NoError(t, err)
	assert.Equal(t, "+19876543210", updatedProfile.PhoneNumber)
	assert.Equal(t, "456 Updated Street", updatedProfile.AddressLine1)
}

// TestMultiFactorAuthentication tests the full MFA workflow
func TestMultiFactorAuthentication(t *testing.T) {
	it := SetupIntegrationTest(t)

	// 1. First authenticate the user
	ctx := context.Background()
	tokenPair, user, err := it.AuthService.AuthenticateUser(ctx, "user@example.com", "Password123!")
	require.NoError(t, err)
	require.NotNil(t, user)

	// 2. Generate TOTP secret
	totpSetup, err := it.TwoFAService.GenerateTOTPSecret(ctx, user.ID)
	require.NoError(t, err)

	// 3. Store TOTP setup
	twoFactor := usermodels.TwoFactorAuth{
		UserID:    user.ID,
		Secret:    totpSetup.Secret,
		Type:      "totp",
		Verified:  false,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = it.DB.Create(&twoFactor).Error
	require.NoError(t, err)

	// 4. Verify setup with a valid code
	isValid, err := it.TwoFAService.VerifyTOTP(ctx, user.ID, "123456")
	require.NoError(t, err)
	assert.True(t, isValid)

	// 5. Mark setup as verified
	err = it.DB.Model(&usermodels.TwoFactorAuth{}).Where("user_id = ? AND type = 'totp'", user.ID).
		Update("verified", true).Error
	require.NoError(t, err)

	// 6. Enable MFA on user
	err = it.DB.Model(&models.User{}).Where("id = ?", user.ID).Update("mfa_enabled", true).Error
	require.NoError(t, err)

	// 7. Verify that the flag is set
	var updatedUser models.User
	err = it.DB.First(&updatedUser, "id = ?", user.ID).Error
	require.NoError(t, err)
	assert.True(t, updatedUser.MFAEnabled)

	// 8. Test authentication with MFA required
	// In a real system, this would first authenticate with username/password,
	// then prompt for 2FA code in a second step

	// 9. Test recovery with backup codes
	backupCodes, err := it.TwoFAService.GenerateBackupCodes(ctx, user.ID)
	require.NoError(t, err)
	require.NotEmpty(t, backupCodes)

	// Save first backup code
	firstBackupCode := backupCodes[0]
	backupCodeEntry := usermodels.TwoFactorAuth{
		UserID:    user.ID,
		Secret:    firstBackupCode,
		Type:      "backup_code",
		Verified:  false,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = it.DB.Create(&backupCodeEntry).Error
	require.NoError(t, err)

	// Verify backup code works
	isValid, err = it.TwoFAService.VerifyBackupCode(ctx, user.ID, firstBackupCode)
	require.NoError(t, err)
	assert.True(t, isValid)

	// After use in a real system, mark the backup code as used
	err = it.DB.Model(&usermodels.TwoFactorAuth{}).
		Where("user_id = ? AND type = 'backup_code' AND secret = ?", user.ID, firstBackupCode).
		Update("verified", true).Error
	require.NoError(t, err)

	// Verify that a used backup code is marked as used
	var usedCode usermodels.TwoFactorAuth
	err = it.DB.First(&usedCode, "user_id = ? AND type = 'backup_code' AND secret = ?",
		user.ID, firstBackupCode).Error
	require.NoError(t, err)
	assert.True(t, usedCode.Verified)
}
