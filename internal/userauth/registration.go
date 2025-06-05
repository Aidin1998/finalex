package userauth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	usermodels "github.com/Aidin1998/finalex/internal/userauth/models"
	globalmodels "github.com/Aidin1998/finalex/pkg/models"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// EnterpriseRegistrationService provides enterprise-grade user registration
type EnterpriseRegistrationService struct {
	db                   *gorm.DB
	logger               *zap.Logger
	encryptionService    EncryptionService
	complianceService    ComplianceService
	auditService         AuditService
	passwordPolicyEngine PasswordPolicyEngine
	kycIntegration       KYCIntegrationService
	notificationService  NotificationService
}

// NewEnterpriseRegistrationService creates a new enterprise registration service
func NewEnterpriseRegistrationService(
	db *gorm.DB,
	logger *zap.Logger,
	encryptionService EncryptionService,
	complianceService ComplianceService,
	auditService AuditService,
	passwordPolicyEngine PasswordPolicyEngine,
	kycIntegration KYCIntegrationService,
	notificationService NotificationService,
) *EnterpriseRegistrationService {
	return &EnterpriseRegistrationService{
		db:                   db,
		logger:               logger,
		encryptionService:    encryptionService,
		complianceService:    complianceService,
		auditService:         auditService,
		passwordPolicyEngine: passwordPolicyEngine,
		kycIntegration:       kycIntegration,
		notificationService:  notificationService,
	}
}

// EnterpriseRegistrationRequest represents a comprehensive registration request
type EnterpriseRegistrationRequest struct {
	// Basic Information
	Email     string `json:"email" validate:"required,email,max=254"`
	Username  string `json:"username" validate:"required,min=3,max=30,alphanum"`
	Password  string `json:"password" validate:"required,min=12,max=128"`
	FirstName string `json:"first_name" validate:"required,min=1,max=50"`
	LastName  string `json:"last_name" validate:"required,min=1,max=50"`

	// Personal Information (Encrypted)
	PhoneNumber  string     `json:"phone_number" validate:"required,e164"`
	DateOfBirth  *time.Time `json:"date_of_birth" validate:"required"`
	Country      string     `json:"country" validate:"required,iso3166_1_alpha3"`
	AddressLine1 string     `json:"address_line1" validate:"required,min=5,max=200"`
	AddressLine2 string     `json:"address_line2" validate:"omitempty,max=200"`
	City         string     `json:"city" validate:"required,min=2,max=100"`
	State        string     `json:"state" validate:"required,min=2,max=100"`
	PostalCode   string     `json:"postal_code" validate:"required,min=3,max=20"`

	// Compliance & Legal
	AcceptTerms           bool `json:"accept_terms" validate:"required,eq=true"`
	AcceptPrivacyPolicy   bool `json:"accept_privacy_policy" validate:"required,eq=true"`
	AcceptKYCRequirements bool `json:"accept_kyc_requirements" validate:"required,eq=true"`
	MarketingConsent      bool `json:"marketing_consent"`

	// Optional Information
	ReferralCode      string `json:"referral_code" validate:"omitempty,alphanum,len=8"`
	PreferredLanguage string `json:"preferred_language" validate:"omitempty,bcp47"`
	Timezone          string `json:"timezone" validate:"omitempty,timezone"`

	// Device & Security Information
	DeviceFingerprint string                 `json:"device_fingerprint" validate:"required"`
	UserAgent         string                 `json:"user_agent" validate:"required"`
	IPAddress         string                 `json:"ip_address" validate:"required,ip"`
	GeolocationData   map[string]interface{} `json:"geolocation_data"`
}

// EnterpriseRegistrationResponse represents the registration response
type EnterpriseRegistrationResponse struct {
	UserID                    uuid.UUID `json:"user_id"`
	Email                     string    `json:"email"`
	Username                  string    `json:"username"`
	KYCRequired               bool      `json:"kyc_required"`
	KYCLevel                  string    `json:"kyc_level"`
	TwoFactorRequired         bool      `json:"two_factor_required"`
	TwoFactorGracePeriod      int       `json:"two_factor_grace_period_days"`
	EmailVerificationRequired bool      `json:"email_verification_required"`
	PhoneVerificationRequired bool      `json:"phone_verification_required"`
	ComplianceFlags           []string  `json:"compliance_flags"`
	NextSteps                 []string  `json:"next_steps"`
	CreatedAt                 time.Time `json:"created_at"`
}

// RegisterUser performs enterprise-grade user registration with comprehensive compliance
func (ers *EnterpriseRegistrationService) RegisterUser(ctx context.Context, req *EnterpriseRegistrationRequest) (*EnterpriseRegistrationResponse, error) {
	// Add a timeout to prevent hanging
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Start audit trail
	auditCtx := ers.auditService.BeginRegistration(ctx, req.Email, req.IPAddress, req.UserAgent)
	defer ers.auditService.EndRegistration(auditCtx)

	// 1. Input Validation & Sanitization
	if err := ers.validateAndSanitizeInput(req); err != nil {
		ers.auditService.LogRegistrationFailure(auditCtx, "input_validation", err.Error())
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// 2. Duplicate Check
	if exists, err := ers.checkUserExists(ctx, req.Email, req.Username); err != nil {
		ers.auditService.LogRegistrationFailure(auditCtx, "duplicate_check", err.Error())
		return nil, fmt.Errorf("duplicate check failed: %w", err)
	} else if exists {
		ers.auditService.LogRegistrationFailure(auditCtx, "duplicate_user", "User already exists")
		return nil, fmt.Errorf("user already exists")
	}

	// 3. Compliance Checks
	complianceResult, err := ers.complianceService.PerformRegistrationChecks(ctx, req)
	if err != nil {
		ers.auditService.LogRegistrationFailure(auditCtx, "compliance_check", err.Error())
		return nil, fmt.Errorf("compliance check failed: %w", err)
	}

	if complianceResult.Blocked {
		ers.auditService.LogRegistrationFailure(auditCtx, "compliance_blocked", complianceResult.Reason)
		return nil, fmt.Errorf("registration blocked: %s", complianceResult.Reason)
	}

	// 4. Password Policy Validation
	if err := ers.passwordPolicyEngine.ValidateNewPassword(req.Password, req.Email); err != nil {
		ers.auditService.LogRegistrationFailure(auditCtx, "password_policy", err.Error())
		return nil, fmt.Errorf("password policy violation: %w", err)
	}

	// 5. Begin Database Transaction
	tx := ers.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			ers.logger.Error("Registration transaction panic", zap.Any("panic", r))
		}
	}()

	// 6. Create User Record
	user, err := ers.createUserRecord(ctx, tx, req, complianceResult)
	if err != nil {
		tx.Rollback()
		ers.auditService.LogRegistrationFailure(auditCtx, "user_creation", err.Error())
		return nil, fmt.Errorf("user creation failed: %w", err)
	}

	// 7. Create User Profile with Encrypted PII
	profile, err := ers.createUserProfile(ctx, tx, user.ID, req)
	if err != nil {
		tx.Rollback()
		ers.auditService.LogRegistrationFailure(auditCtx, "profile_creation", err.Error())
		return nil, fmt.Errorf("profile creation failed: %w", err)
	}

	// 8. Initialize 2FA (with 7-day grace period)
	if _, err := ers.initialize2FA(ctx, tx, user.ID); err != nil {
		tx.Rollback()
		ers.auditService.LogRegistrationFailure(auditCtx, "2fa_initialization", err.Error())
		return nil, fmt.Errorf("2FA initialization failed: %w", err)
	}

	// 9. Create Device Fingerprint Record
	if _, err := ers.createDeviceFingerprint(ctx, tx, user.ID, req); err != nil {
		tx.Rollback()
		ers.auditService.LogRegistrationFailure(auditCtx, "device_fingerprint", err.Error())
		return nil, fmt.Errorf("device fingerprint creation failed: %w", err)
	}

	// 10. Initialize Password Policy
	if err := ers.createPasswordPolicy(ctx, tx, user.ID, req.Password); err != nil {
		tx.Rollback()
		ers.auditService.LogRegistrationFailure(auditCtx, "password_policy_init", err.Error())
		return nil, fmt.Errorf("password policy initialization failed: %w", err)
	}

	// 11. Commit Transaction
	if err := tx.Commit().Error; err != nil {
		ers.auditService.LogRegistrationFailure(auditCtx, "transaction_commit", err.Error())
		return nil, fmt.Errorf("transaction commit failed: %w", err)
	}

	// 12. Post-Registration Actions (async)
	go ers.performPostRegistrationActions(context.Background(), user, profile, complianceResult, req)

	// 13. Build Response
	response := &EnterpriseRegistrationResponse{
		UserID:                    user.ID,
		Email:                     user.Email,
		Username:                  user.Username,
		KYCRequired:               complianceResult.KYCRequired,
		KYCLevel:                  complianceResult.RequiredKYCLevel,
		TwoFactorRequired:         true,
		TwoFactorGracePeriod:      7,
		EmailVerificationRequired: true,
		PhoneVerificationRequired: true,
		ComplianceFlags:           complianceResult.Flags,
		NextSteps:                 ers.buildNextSteps(complianceResult),
		CreatedAt:                 user.CreatedAt,
	}

	ers.auditService.LogRegistrationSuccess(auditCtx, user.ID.String())
	ers.logger.Info("User registration completed successfully",
		zap.String("user_id", user.ID.String()),
		zap.String("email", user.Email),
		zap.String("country", req.Country))

	return response, nil
}

// validateAndSanitizeInput performs comprehensive input validation
func (ers *EnterpriseRegistrationService) validateAndSanitizeInput(req *EnterpriseRegistrationRequest) error {
	// Email validation
	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	if !emailRegex.MatchString(req.Email) {
		return fmt.Errorf("invalid email format")
	}

	// Sanitize and normalize email
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))

	// Username validation (alphanumeric, no special chars except underscore)
	usernameRegex := regexp.MustCompile(`^[a-zA-Z0-9_]{3,30}$`)
	if !usernameRegex.MatchString(req.Username) {
		return fmt.Errorf("invalid username format")
	}

	// Password complexity validation
	if err := ers.validatePasswordComplexity(req.Password); err != nil {
		return fmt.Errorf("password complexity: %w", err)
	}

	// Phone number validation (should be in E.164 format)
	phoneRegex := regexp.MustCompile(`^\+[1-9]\d{1,14}$`)
	if !phoneRegex.MatchString(req.PhoneNumber) {
		return fmt.Errorf("invalid phone number format (E.164 required)")
	}

	// Date of birth validation
	if req.DateOfBirth == nil {
		return fmt.Errorf("date of birth is required")
	}

	// Age validation (minimum 18 years)
	age := time.Since(*req.DateOfBirth).Hours() / 24 / 365.25
	if age < 18 {
		return fmt.Errorf("minimum age requirement not met")
	}

	// Sanitize text fields
	req.FirstName = strings.TrimSpace(req.FirstName)
	req.LastName = strings.TrimSpace(req.LastName)
	req.AddressLine1 = strings.TrimSpace(req.AddressLine1)
	req.AddressLine2 = strings.TrimSpace(req.AddressLine2)
	req.City = strings.TrimSpace(req.City)
	req.State = strings.TrimSpace(req.State)
	req.PostalCode = strings.TrimSpace(req.PostalCode)

	return nil
}

// validatePasswordComplexity ensures password meets enterprise security requirements
func (ers *EnterpriseRegistrationService) validatePasswordComplexity(password string) error {
	if len(password) < 12 {
		return fmt.Errorf("password must be at least 12 characters long")
	}

	if len(password) > 128 {
		return fmt.Errorf("password must not exceed 128 characters")
	}

	hasUpper := regexp.MustCompile(`[A-Z]`).MatchString(password)
	hasLower := regexp.MustCompile(`[a-z]`).MatchString(password)
	hasNumber := regexp.MustCompile(`[0-9]`).MatchString(password)
	hasSpecial := regexp.MustCompile(`[!@#$%^&*()_+\-=\[\]{};':"\\|,.<>\/?]`).MatchString(password)

	if !hasUpper {
		return fmt.Errorf("password must contain at least one uppercase letter")
	}

	if !hasLower {
		return fmt.Errorf("password must contain at least one lowercase letter")
	}

	if !hasNumber {
		return fmt.Errorf("password must contain at least one number")
	}

	if !hasSpecial {
		return fmt.Errorf("password must contain at least one special character")
	}

	// Check for common passwords
	commonPasswords := []string{"password123", "qwerty123", "admin123", "welcome123"}
	for _, common := range commonPasswords {
		if strings.Contains(strings.ToLower(password), common) {
			return fmt.Errorf("password contains common patterns")
		}
	}

	return nil
}

// checkUserExists checks if user already exists by email or username
func (ers *EnterpriseRegistrationService) checkUserExists(ctx context.Context, email, username string) (bool, error) {
	var count int64
	err := ers.db.WithContext(ctx).Model(&globalmodels.User{}).
		Where("email = ? OR username = ?", email, username).
		Count(&count).Error

	if err != nil {
		return false, fmt.Errorf("database error: %w", err)
	}

	return count > 0, nil
}

// createUserRecord creates the main user record
func (ers *EnterpriseRegistrationService) createUserRecord(ctx context.Context, tx *gorm.DB, req *EnterpriseRegistrationRequest, compliance *ComplianceResult) (*globalmodels.User, error) {
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("password hashing failed: %w", err)
	}

	user := &globalmodels.User{
		ID:             uuid.New(),
		Email:          req.Email,
		Username:       req.Username,
		PasswordHash:   string(passwordHash),
		FirstName:      req.FirstName,
		LastName:       req.LastName,
		KYCStatus:      "pending",
		Role:           "user",
		Tier:           compliance.InitialTier,
		MFAEnabled:     false,
		TrustedDevices: "[]",
		RBAC:           `{"roles":["user"],"permissions":["basic_trading","view_account"]}`,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	if err := tx.WithContext(ctx).Create(user).Error; err != nil {
		return nil, fmt.Errorf("user creation failed: %w", err)
	}

	return user, nil
}

// createUserProfile creates encrypted user profile
func (ers *EnterpriseRegistrationService) createUserProfile(ctx context.Context, tx *gorm.DB, userID uuid.UUID, req *EnterpriseRegistrationRequest) (*usermodels.UserProfile, error) {
	// Encrypt PII data
	piiData := map[string]interface{}{
		"phone_number":  req.PhoneNumber,
		"date_of_birth": req.DateOfBirth,
		"address_line1": req.AddressLine1,
		"address_line2": req.AddressLine2,
		"city":          req.City,
		"state":         req.State,
		"postal_code":   req.PostalCode,
	}

	encryptedPII, encryptionKey, err := ers.encryptionService.EncryptPII(piiData)
	if err != nil {
		return nil, fmt.Errorf("PII encryption failed: %w", err)
	}

	// Set default preferences
	notificationPrefs := map[string]bool{
		"email_trading":   true,
		"email_security":  true,
		"email_marketing": req.MarketingConsent,
		"sms_security":    true,
		"push_trading":    true,
	}

	privacySettings := map[string]bool{
		"profile_public":    false,
		"trading_public":    false,
		"analytics_consent": true,
	}

	twoFactorSettings := map[string]interface{}{
		"enforcement_date": time.Now().Add(7 * 24 * time.Hour),
		"methods_enabled":  []string{},
		"grace_period":     7,
	}

	complianceFlags := map[string]interface{}{
		"registration_country": req.Country,
		"marketing_consent":    req.MarketingConsent,
		"terms_version":        "v2.1",
		"privacy_version":      "v1.8",
	}

	notificationPrefsJSON, _ := json.Marshal(notificationPrefs)
	privacySettingsJSON, _ := json.Marshal(privacySettings)
	twoFactorSettingsJSON, _ := json.Marshal(twoFactorSettings)
	complianceFlagsJSON, _ := json.Marshal(complianceFlags)

	profile := &usermodels.UserProfile{
		ID:                uuid.New(),
		UserID:            userID,
		EncryptedPII:      encryptedPII,
		PIIKey:            encryptionKey,
		Country:           req.Country,
		PreferredLanguage: req.PreferredLanguage,
		Timezone:          req.Timezone,
		NotificationPrefs: string(notificationPrefsJSON),
		PrivacySettings:   string(privacySettingsJSON),
		TwoFactorSettings: string(twoFactorSettingsJSON),
		ComplianceFlags:   string(complianceFlagsJSON),
		RiskScore:         0.0,
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}

	if req.PreferredLanguage == "" {
		profile.PreferredLanguage = "en"
	}
	if req.Timezone == "" {
		profile.Timezone = "UTC"
	}

	if err := tx.WithContext(ctx).Create(profile).Error; err != nil {
		return nil, fmt.Errorf("profile creation failed: %w", err)
	}

	return profile, nil
}

// initialize2FA creates 2FA record with 7-day enforcement grace period
func (ers *EnterpriseRegistrationService) initialize2FA(ctx context.Context, tx *gorm.DB, userID uuid.UUID) (*usermodels.TwoFactorAuth, error) {
	// Generate TOTP secret
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, fmt.Errorf("secret generation failed: %w", err)
	}

	secretB64 := base64.StdEncoding.EncodeToString(secret)
	encryptedSecret, err := ers.encryptionService.Encrypt(secretB64)
	if err != nil {
		return nil, fmt.Errorf("secret encryption failed: %w", err)
	}

	// Generate backup codes
	backupCodes := make([]string, 10)
	for i := range backupCodes {
		code := make([]byte, 8)
		rand.Read(code)
		backupCodes[i] = fmt.Sprintf("%x", code)[:16]
	}

	backupCodesJSON, _ := json.Marshal(backupCodes)
	encryptedBackupCodes, err := ers.encryptionService.Encrypt(string(backupCodesJSON))
	if err != nil {
		return nil, fmt.Errorf("backup codes encryption failed: %w", err)
	}

	twoFA := &usermodels.TwoFactorAuth{
		ID:              uuid.New(),
		UserID:          userID,
		TOTPSecret:      encryptedSecret,
		BackupCodes:     encryptedBackupCodes,
		BackupCodesUsed: "[]",
		IsEnabled:       false,
		IsEnforced:      false,
		EnforcementDate: timePtr(time.Now().Add(7 * 24 * time.Hour)),
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	if err := tx.WithContext(ctx).Create(twoFA).Error; err != nil {
		return nil, fmt.Errorf("2FA creation failed: %w", err)
	}

	return twoFA, nil
}

// createDeviceFingerprint creates device fingerprint record
func (ers *EnterpriseRegistrationService) createDeviceFingerprint(ctx context.Context, tx *gorm.DB, userID uuid.UUID, req *EnterpriseRegistrationRequest) (*usermodels.DeviceFingerprint, error) {
	geolocationJSON, _ := json.Marshal(req.GeolocationData)

	deviceFP := &usermodels.DeviceFingerprint{
		ID:              uuid.New(),
		UserID:          userID,
		Fingerprint:     req.DeviceFingerprint,
		UserAgent:       req.UserAgent,
		IsTrusted:       false,
		FirstSeenAt:     time.Now(),
		LoginCount:      1,
		RiskScore:       0.0,
		IsBlocked:       false,
		GeolocationData: string(geolocationJSON),
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	if err := tx.WithContext(ctx).Create(deviceFP).Error; err != nil {
		return nil, fmt.Errorf("device fingerprint creation failed: %w", err)
	}

	return deviceFP, nil
}

// createPasswordPolicy initializes password policy for user
func (ers *EnterpriseRegistrationService) createPasswordPolicy(ctx context.Context, tx *gorm.DB, userID uuid.UUID, password string) error {
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("password hashing failed: %w", err)
	}

	passwordPolicy := &usermodels.PasswordPolicy{
		ID:              uuid.New(),
		UserID:          userID,
		PasswordHash:    string(passwordHash),
		PasswordHistory: "[]",
		LastChangedAt:   time.Now(),
		PolicyVersion:   "v1.0",
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	if err := tx.WithContext(ctx).Create(passwordPolicy).Error; err != nil {
		return fmt.Errorf("password policy creation failed: %w", err)
	}

	return nil
}

// performPostRegistrationActions handles async post-registration tasks
func (ers *EnterpriseRegistrationService) performPostRegistrationActions(ctx context.Context, user *globalmodels.User, profile *usermodels.UserProfile, compliance *ComplianceResult, req *EnterpriseRegistrationRequest) {
	// Send welcome email
	go ers.notificationService.SendWelcomeEmail(ctx, user.Email, user.FirstName)

	// Send email verification
	go ers.notificationService.SendEmailVerification(ctx, user.ID, user.Email)

	// Send SMS verification
	go ers.notificationService.SendSMSVerification(ctx, user.ID, req.PhoneNumber)

	// Initialize KYC process if required
	if compliance.KYCRequired {
		go ers.kycIntegration.InitializeKYCProcess(ctx, user.ID, compliance.RequiredKYCLevel)
	}

	// Create audit log for registration
	auditData := map[string]interface{}{
		"user_id":            user.ID,
		"email":              user.Email,
		"country":            req.Country,
		"kyc_required":       compliance.KYCRequired,
		"compliance_flags":   compliance.Flags,
		"device_fingerprint": req.DeviceFingerprint,
	}

	ers.auditService.LogEvent(ctx, &usermodels.UserAuditLog{
		UserID:        &user.ID,
		EventType:     "user_registration",
		EventCategory: "registration",
		EventSeverity: "info",
		IPAddress:     req.IPAddress,
		UserAgent:     req.UserAgent,
		EventData:     jsonMarshal(auditData),
		ProcessedBy:   "registration_service",
		CorrelationID: uuid.New().String(),
		CreatedAt:     time.Now(),
	})
}

// buildNextSteps builds the next steps for the user based on compliance requirements
func (ers *EnterpriseRegistrationService) buildNextSteps(compliance *ComplianceResult) []string {
	steps := []string{
		"Verify your email address",
		"Verify your phone number",
		"Set up two-factor authentication (mandatory after 7 days)",
	}

	if compliance.KYCRequired {
		steps = append(steps, fmt.Sprintf("Complete %s KYC verification", compliance.RequiredKYCLevel))
	}

	return steps
}

// Helper functions
func timePtr(t time.Time) *time.Time {
	return &t
}

func jsonMarshal(v interface{}) string {
	data, _ := json.Marshal(v)
	return string(data)
}

// ComplianceResult represents the result of compliance checks
type ComplianceResult struct {
	Blocked          bool     `json:"blocked"`
	Reason           string   `json:"reason,omitempty"`
	KYCRequired      bool     `json:"kyc_required"`
	RequiredKYCLevel string   `json:"required_kyc_level"`
	InitialTier      string   `json:"initial_tier"`
	Flags            []string `json:"flags"`
	RiskScore        float64  `json:"risk_score"`
}

// Service Interfaces (to be implemented)
type EncryptionService interface {
	EncryptPII(data map[string]interface{}) (encrypted, key string, err error)
	Encrypt(data string) (encrypted string, err error)
	Decrypt(encrypted string) (data string, err error)
}

type ComplianceService interface {
	PerformRegistrationChecks(ctx context.Context, req *EnterpriseRegistrationRequest) (*ComplianceResult, error)
}

type AuditService interface {
	BeginRegistration(ctx context.Context, email, ip, userAgent string) context.Context
	EndRegistration(ctx context.Context)
	LogRegistrationFailure(ctx context.Context, reason, details string)
	LogRegistrationSuccess(ctx context.Context, userID string)
	LogEvent(ctx context.Context, event *usermodels.UserAuditLog) error
}

type PasswordPolicyEngine interface {
	ValidateNewPassword(password, email string) error
}

type KYCIntegrationService interface {
	InitializeKYCProcess(ctx context.Context, userID uuid.UUID, level string) error
}

type NotificationService interface {
	SendWelcomeEmail(ctx context.Context, email, firstName string) error
	SendEmailVerification(ctx context.Context, userID uuid.UUID, email string) error
	SendSMSVerification(ctx context.Context, userID uuid.UUID, phoneNumber string) error
}
