//go:build userauth

package test

import (
	"context"
	"testing"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/userauth"
	usermodels "github.com/Aidin1998/pincex_unified/internal/userauth/models"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// mock implementations for all required interfaces
// ...

type mockEncryptionService struct{}

func (m *mockEncryptionService) EncryptPII(data map[string]interface{}) (string, string, error) {
	return "encrypted", "key", nil
}
func (m *mockEncryptionService) Encrypt(data string) (string, error)      { return "encrypted", nil }
func (m *mockEncryptionService) Decrypt(encrypted string) (string, error) { return "decrypted", nil }

type mockComplianceService struct{}

func (m *mockComplianceService) PerformRegistrationChecks(ctx context.Context, req *userauth.EnterpriseRegistrationRequest) (*userauth.ComplianceResult, error) {
	return &userauth.ComplianceResult{
		Blocked:          false,
		KYCRequired:      false,
		RequiredKYCLevel: "basic",
		InitialTier:      "basic",
		Flags:            []string{},
		RiskScore:        0,
	}, nil
}

type mockAuditService struct{}

func (m *mockAuditService) BeginRegistration(ctx context.Context, email, ip, userAgent string) context.Context {
	return ctx
}
func (m *mockAuditService) EndRegistration(ctx context.Context)                                {}
func (m *mockAuditService) LogRegistrationFailure(ctx context.Context, reason, details string) {}
func (m *mockAuditService) LogRegistrationSuccess(ctx context.Context, userID string)          {}
func (m *mockAuditService) LogEvent(ctx context.Context, event *usermodels.UserAuditLog) error {
	return nil
}

type mockPasswordPolicyEngine struct{}

func (m *mockPasswordPolicyEngine) ValidateNewPassword(password, email string) error { return nil }

type mockKYCIntegrationService struct{}

func (m *mockKYCIntegrationService) InitializeKYCProcess(ctx context.Context, userID uuid.UUID, level string) error {
	return nil
}

type mockNotificationService struct{}

func (m *mockNotificationService) SendWelcomeEmail(ctx context.Context, email, firstName string) error {
	return nil
}
func (m *mockNotificationService) SendEmailVerification(ctx context.Context, userID uuid.UUID, email string) error {
	return nil
}
func (m *mockNotificationService) SendSMSVerification(ctx context.Context, userID uuid.UUID, phoneNumber string) error {
	return nil
}

func setupTestRegistrationService(t *testing.T) *userauth.EnterpriseRegistrationService {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	db.AutoMigrate(&usermodels.UserProfile{}, &usermodels.TwoFactorAuth{}, &usermodels.DeviceFingerprint{}, &usermodels.PasswordPolicy{})
	// Also migrate globalmodels.User if needed
	return userauth.NewEnterpriseRegistrationService(
		db,
		nil,
		&mockEncryptionService{},
		&mockComplianceService{},
		&mockAuditService{},
		&mockPasswordPolicyEngine{},
		&mockKYCIntegrationService{},
		&mockNotificationService{},
	)
}

func TestRegisterUser_Success(t *testing.T) {
	svc := setupTestRegistrationService(t)
	dob := time.Now().AddDate(-20, 0, 0)
	req := &userauth.EnterpriseRegistrationRequest{
		Email:                 "testuser@example.com",
		Username:              "testuser",
		Password:              "SuperSecure!123",
		FirstName:             "Test",
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
	resp, err := svc.RegisterUser(context.Background(), req)
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if resp.Email != req.Email {
		t.Errorf("expected email %s, got %s", req.Email, resp.Email)
	}
	if !resp.TwoFactorRequired {
		t.Errorf("expected 2FA to be required")
	}
}

func TestRegisterUser_Duplicate(t *testing.T) {
	svc := setupTestRegistrationService(t)
	dob := time.Now().AddDate(-20, 0, 0)
	req := &userauth.EnterpriseRegistrationRequest{
		Email:                 "dupe@example.com",
		Username:              "dupeuser",
		Password:              "SuperSecure!123",
		FirstName:             "Dupe",
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
	_, err := svc.RegisterUser(context.Background(), req)
	if err != nil {
		t.Fatalf("expected first registration to succeed, got error: %v", err)
	}
	_, err = svc.RegisterUser(context.Background(), req)
	if err == nil {
		t.Fatalf("expected duplicate registration to fail")
	}
}
