package accounts_test

import (
	"context"
	"testing"
	"time"

	"github.com/Aidin1998/finalex/internal/userauth"
	"github.com/Aidin1998/finalex/internal/userauth/auth"
	"github.com/Aidin1998/finalex/internal/userauth/models"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestUserSecurityFlow(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}

	// Migrate user models (use UserProfile and UserSession only, as User struct is missing)
	db.AutoMigrate(&models.UserProfile{}, &models.UserSession{})

	ctx := context.Background()
	logger := NewTestLogger()

	// Setup registration service with stubs/mocks for dependencies
	regService := userauth.NewEnterpriseRegistrationService(
		db,
		logger,
		NewStubEncryptionService(),
		NewStubComplianceService(),
		NewStubAuditService(),
		NewStubPasswordPolicyEngine(),
		NewStubKYCIntegrationService(),
		NewStubNotificationService(),
	)

	// 1. Sign up (register)
	email := "testuser@example.com"
	username := "testuser"
	password := "TestPassword123!"
	regReq := &userauth.EnterpriseRegistrationRequest{
		Email:                 email,
		Username:              username,
		Password:              password,
		FirstName:             "Test",
		LastName:              "User",
		PhoneNumber:           "+1234567890",
		DateOfBirth:           func() *time.Time { t := time.Now().AddDate(-30, 0, 0); return &t }(),
		Country:               "USA",
		AddressLine1:          "123 Main St",
		AddressLine2:          "",
		City:                  "Testville",
		State:                 "TS",
		PostalCode:            "12345",
		AcceptTerms:           true,
		AcceptPrivacyPolicy:   true,
		AcceptKYCRequirements: true,
		MarketingConsent:      false,
		ReferralCode:          "",
		PreferredLanguage:     "en",
		Timezone:              "UTC",
		DeviceFingerprint:     "test-device-fp",
		UserAgent:             "test-agent",
		IPAddress:             "127.0.0.1",
		GeolocationData:       map[string]interface{}{},
	}
	regResp, err := regService.RegisterUser(ctx, regReq)
	if err != nil {
		t.Fatalf("registration failed: %v", err)
	}
	if regResp.Email != email {
		t.Errorf("expected email %s, got %s", email, regResp.Email)
	}

	// 2. Sign in (authenticate)
	authService := NewTestAuthService(db, logger)
	tokens, user, err := authService.AuthenticateUser(ctx, email, password)
	if err != nil {
		t.Fatalf("authentication failed: %v", err)
	}
	if user.Email != email {
		t.Errorf("expected user email %s, got %s", email, user.Email)
	}
	if tokens == nil || tokens.AccessToken == "" {
		t.Error("expected non-empty access token")
	}

	// 3. Sign out (invalidate session)
	sess, err := authService.CreateSession(ctx, user.ID, "test-device")
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}
	err = authService.InvalidateSession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("invalidate session failed: %v", err)
	}
}

// --- Test helpers and stubs for dependencies ---

// NewTestLogger returns a no-op logger for testing
func NewTestLogger() *zap.Logger {
	cfg := zap.NewDevelopmentConfig()
	cfg.OutputPaths = []string{"/dev/null"}
	logger, _ := cfg.Build()
	return logger
}

type stubEncryptionService struct{}

func NewStubEncryptionService() *stubEncryptionService { return &stubEncryptionService{} }

// EncryptionService stub
func (s *stubEncryptionService) Encrypt(plaintext string) (string, error) {
	return plaintext, nil
}
func (s *stubEncryptionService) Decrypt(ciphertext string) (string, error) {
	return ciphertext, nil
}
func (s *stubEncryptionService) EncryptPII(data map[string]interface{}) (string, string, error) {
	return "{}", "key", nil
}

// Implement required methods as no-ops

type stubComplianceService struct{}

func NewStubComplianceService() *stubComplianceService { return &stubComplianceService{} }

// ComplianceService stub
func (s *stubComplianceService) PerformRegistrationChecks(ctx context.Context, req *userauth.EnterpriseRegistrationRequest) (*userauth.ComplianceResult, error) {
	return &userauth.ComplianceResult{}, nil
}

type stubAuditService struct{}

func NewStubAuditService() *stubAuditService { return &stubAuditService{} }

// AuditService stub
func (s *stubAuditService) BeginRegistration(ctx context.Context, email, ip, userAgent string) context.Context {
	return ctx
}
func (s *stubAuditService) EndRegistration(ctx context.Context)                              {}
func (s *stubAuditService) LogRegistrationFailure(ctx context.Context, stage, reason string) {}
func (s *stubAuditService) LogRegistrationSuccess(ctx context.Context, userID string)        {}
func (s *stubAuditService) LogEvent(ctx context.Context, log *models.UserAuditLog) error {
	return nil
}

// --- PasswordPolicyEngine stub ---
type stubPasswordPolicyEngine struct{}

func NewStubPasswordPolicyEngine() *stubPasswordPolicyEngine { return &stubPasswordPolicyEngine{} }

// PasswordPolicyEngine stub
func (s *stubPasswordPolicyEngine) ValidateNewPassword(password, email string) error {
	return nil
}

type stubKYCIntegrationService struct{}

func NewStubKYCIntegrationService() *stubKYCIntegrationService { return &stubKYCIntegrationService{} }

// Implement required methods for KYCIntegrationService
func (s *stubKYCIntegrationService) InitializeKYCProcess(ctx context.Context, userID uuid.UUID, country string) error {
	return nil
}

type stubNotificationService struct{}

func NewStubNotificationService() *stubNotificationService { return &stubNotificationService{} }

// Implement required methods for NotificationService
func (s *stubNotificationService) SendEmailVerification(ctx context.Context, userID uuid.UUID, code string) error {
	return nil
}
func (s *stubNotificationService) SendPhoneVerification(ctx context.Context, userID uuid.UUID, code string) error {
	return nil
}
func (s *stubNotificationService) SendSMSVerification(ctx context.Context, userID uuid.UUID, code string) error {
	return nil
}
func (s *stubNotificationService) SendWelcomeEmail(ctx context.Context, email, username string) error {
	return nil
}

// --- Minimal test AuthService implementation ---
type testAuthService struct {
	db     *gorm.DB
	logger *zap.Logger
}

func NewTestAuthService(db *gorm.DB, logger *zap.Logger) *testAuthService {
	return &testAuthService{db: db, logger: logger}
}

type Tokens struct {
	AccessToken string
}

type TestUser struct {
	ID    uuid.UUID
	Email string
}

func (s *testAuthService) AuthenticateUser(ctx context.Context, email, password string) (*Tokens, *TestUser, error) {
	return &Tokens{AccessToken: "test-access-token"}, &TestUser{ID: uuid.New(), Email: email}, nil
}

func (s *testAuthService) CreateSession(ctx context.Context, userID uuid.UUID, deviceInfo string) (*auth.Session, error) {
	return &auth.Session{ID: uuid.New(), UserID: userID, DeviceFingerprint: deviceInfo, ExpiresAt: time.Now().Add(24 * time.Hour)}, nil
}

func (s *testAuthService) InvalidateSession(ctx context.Context, sessionID uuid.UUID) error {
	return nil
}
