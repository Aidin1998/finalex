package models

import (
	"time"

	"github.com/google/uuid"
)

// UserProfile represents a comprehensive user profile with enterprise features
type UserProfile struct {
	ID                uuid.UUID  `json:"id" gorm:"primaryKey;type:uuid" validate:"required,uuid"`
	UserID            uuid.UUID  `json:"user_id" gorm:"type:uuid;uniqueIndex" validate:"required,uuid"`
	EncryptedPII      string     `json:"-" gorm:"type:text"`         // Encrypted PII data
	PIIKey            string     `json:"-" gorm:"type:varchar(255)"` // Encryption key reference
	PhoneNumber       string     `json:"-" gorm:"type:varchar(20)"`  // Encrypted, will be null in JSON
	PhoneVerified     bool       `json:"phone_verified" gorm:"default:false"`
	DateOfBirth       *time.Time `json:"-" gorm:"type:date"`                                                  // Encrypted, will be null in JSON
	AddressLine1      string     `json:"-" gorm:"type:text"`                                                  // Encrypted
	AddressLine2      string     `json:"-" gorm:"type:text"`                                                  // Encrypted
	City              string     `json:"-" gorm:"type:varchar(100)"`                                          // Encrypted
	State             string     `json:"-" gorm:"type:varchar(100)"`                                          // Encrypted
	PostalCode        string     `json:"-" gorm:"type:varchar(20)"`                                           // Encrypted
	Country           string     `json:"country" gorm:"type:varchar(3)" validate:"required,iso3166_1_alpha3"` // ISO 3166-1 alpha-3
	TaxID             string     `json:"-" gorm:"type:varchar(50)"`                                           // Encrypted
	DocumentType      string     `json:"document_type" gorm:"type:varchar(50)" validate:"oneof=passport drivers_license id_card"`
	DocumentNumber    string     `json:"-" gorm:"type:varchar(100)"` // Encrypted
	DocumentExpiryAt  *time.Time `json:"document_expiry_at" gorm:"type:date"`
	BiometricHash     string     `json:"-" gorm:"type:varchar(255)"` // Biometric data hash
	ProfilePictureURL string     `json:"profile_picture_url" gorm:"type:varchar(500)" validate:"omitempty,url"`
	PreferredLanguage string     `json:"preferred_language" gorm:"type:varchar(10);default:en" validate:"bcp47"`
	Timezone          string     `json:"timezone" gorm:"type:varchar(50);default:UTC" validate:"timezone"`
	NotificationPrefs string     `json:"notification_preferences" gorm:"type:json"`
	PrivacySettings   string     `json:"privacy_settings" gorm:"type:json"`
	TwoFactorSettings string     `json:"two_factor_settings" gorm:"type:json"`
	ComplianceFlags   string     `json:"compliance_flags" gorm:"type:json"`
	RiskScore         float64    `json:"risk_score" gorm:"default:0" validate:"min=0,max=100"`
	LastKYCUpdate     *time.Time `json:"last_kyc_update"`
	ComplianceExpiry  *time.Time `json:"compliance_expiry"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// DeviceFingerprint represents a device fingerprint for security tracking
type DeviceFingerprint struct {
	ID               uuid.UUID  `json:"id" gorm:"primaryKey;type:uuid"`
	UserID           uuid.UUID  `json:"user_id" gorm:"type:uuid;index" validate:"required,uuid"`
	Fingerprint      string     `json:"fingerprint" gorm:"type:varchar(255);uniqueIndex" validate:"required"`
	DeviceName       string     `json:"device_name" gorm:"type:varchar(255)"`
	UserAgent        string     `json:"user_agent" gorm:"type:text"`
	ScreenResolution string     `json:"screen_resolution" gorm:"type:varchar(50)"`
	Timezone         string     `json:"timezone" gorm:"type:varchar(50)"`
	Language         string     `json:"language" gorm:"type:varchar(20)"`
	Platform         string     `json:"platform" gorm:"type:varchar(50)"`
	IsTrusted        bool       `json:"is_trusted" gorm:"default:false"`
	LastSeenAt       *time.Time `json:"last_seen_at"`
	FirstSeenAt      time.Time  `json:"first_seen_at"`
	LoginCount       int        `json:"login_count" gorm:"default:0"`
	RiskScore        float64    `json:"risk_score" gorm:"default:0" validate:"min=0,max=100"`
	IsBlocked        bool       `json:"is_blocked" gorm:"default:false"`
	GeolocationData  string     `json:"geolocation_data" gorm:"type:json"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// UserSession represents an extended user session with enterprise security features
type UserSession struct {
	ID                  uuid.UUID  `json:"id" gorm:"primaryKey;type:uuid"`
	UserID              uuid.UUID  `json:"user_id" gorm:"type:uuid;index" validate:"required,uuid"`
	DeviceFingerprintID uuid.UUID  `json:"device_fingerprint_id" gorm:"type:uuid;index"`
	SessionToken        string     `json:"-" gorm:"type:varchar(255);uniqueIndex" validate:"required"`
	IPAddress           string     `json:"ip_address" gorm:"type:varchar(45)" validate:"ip"`
	UserAgent           string     `json:"user_agent" gorm:"type:text"`
	IsActive            bool       `json:"is_active" gorm:"default:true"`
	IsMFA               bool       `json:"is_mfa" gorm:"default:false"`
	LastActivityAt      time.Time  `json:"last_activity_at"`
	ExpiresAt           time.Time  `json:"expires_at"`
	IdleTimeoutAt       *time.Time `json:"idle_timeout_at"`
	MaxLifetimeAt       *time.Time `json:"max_lifetime_at"`
	GeolocationData     string     `json:"geolocation_data" gorm:"type:json"`
	SecurityFlags       string     `json:"security_flags" gorm:"type:json"`
	ConcurrentSessions  int        `json:"concurrent_sessions" gorm:"default:1"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
	TerminatedAt        *time.Time `json:"terminated_at"`
	TerminationReason   string     `json:"termination_reason" gorm:"type:varchar(100)"`
}

// TwoFactorAuth represents 2FA configuration and backup codes
type TwoFactorAuth struct {
	ID              uuid.UUID  `json:"id" gorm:"primaryKey;type:uuid"`
	UserID          uuid.UUID  `json:"user_id" gorm:"type:uuid;uniqueIndex" validate:"required,uuid"`
	TOTPSecret      string     `json:"-" gorm:"type:varchar(255)" validate:"required"` // Encrypted
	BackupCodes     string     `json:"-" gorm:"type:text"`                             // Encrypted JSON array
	BackupCodesUsed string     `json:"-" gorm:"type:text"`                             // Encrypted JSON array of used codes
	IsEnabled       bool       `json:"is_enabled" gorm:"default:false"`
	IsEnforced      bool       `json:"is_enforced" gorm:"default:false"` // Mandatory after 7 days
	EnforcementDate *time.Time `json:"enforcement_date"`                 // When 2FA becomes mandatory
	LastUsedAt      *time.Time `json:"last_used_at"`
	RecoveryEmail   string     `json:"recovery_email" gorm:"type:varchar(255)" validate:"omitempty,email"`
	RecoveryPhone   string     `json:"-" gorm:"type:varchar(20)"` // Encrypted
	SMSEnabled      bool       `json:"sms_enabled" gorm:"default:false"`
	EmailEnabled    bool       `json:"email_enabled" gorm:"default:false"`
	AppEnabled      bool       `json:"app_enabled" gorm:"default:false"`
	WebAuthnEnabled bool       `json:"webauthn_enabled" gorm:"default:false"`
	FailedAttempts  int        `json:"failed_attempts" gorm:"default:0"`
	LastFailedAt    *time.Time `json:"last_failed_at"`
	LockedUntil     *time.Time `json:"locked_until"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// KYCDocument represents KYC documentation with compliance tracking
type KYCDocument struct {
	ID              uuid.UUID  `json:"id" gorm:"primaryKey;type:uuid"`
	UserID          uuid.UUID  `json:"user_id" gorm:"type:uuid;index" validate:"required,uuid"`
	DocumentType    string     `json:"document_type" gorm:"type:varchar(50)" validate:"required,oneof=passport drivers_license id_card utility_bill bank_statement proof_of_address"`
	DocumentStatus  string     `json:"document_status" gorm:"type:varchar(50);default:pending" validate:"oneof=pending under_review approved rejected expired"`
	FileURL         string     `json:"-" gorm:"type:varchar(500)"`               // Encrypted URL
	FileHash        string     `json:"file_hash" gorm:"type:varchar(255)"`       // SHA-256 hash for integrity
	EncryptedData   string     `json:"-" gorm:"type:text"`                       // Encrypted document data
	ExtractionData  string     `json:"extraction_data" gorm:"type:json"`         // OCR/AI extracted data (non-PII)
	VerificationID  string     `json:"verification_id" gorm:"type:varchar(255)"` // External KYC provider ID
	ReviewedBy      *uuid.UUID `json:"reviewed_by" gorm:"type:uuid"`             // Admin user ID
	ReviewedAt      *time.Time `json:"reviewed_at"`
	RejectionReason string     `json:"rejection_reason" gorm:"type:text"`
	ExpiresAt       *time.Time `json:"expires_at"`
	ComplianceNotes string     `json:"compliance_notes" gorm:"type:text"`
	RiskFlags       string     `json:"risk_flags" gorm:"type:json"`
	AuditTrail      string     `json:"audit_trail" gorm:"type:json"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// UserAuditLog represents comprehensive audit logging for compliance
type UserAuditLog struct {
	ID              uuid.UUID  `json:"id" gorm:"primaryKey;type:uuid"`
	UserID          *uuid.UUID `json:"user_id" gorm:"type:uuid;index"` // Nullable for system events
	SessionID       *uuid.UUID `json:"session_id" gorm:"type:uuid;index"`
	EventType       string     `json:"event_type" gorm:"type:varchar(100)" validate:"required"`    // login, logout, password_change, etc.
	EventCategory   string     `json:"event_category" gorm:"type:varchar(50)" validate:"required"` // authentication, authorization, data_access, etc.
	EventSeverity   string     `json:"event_severity" gorm:"type:varchar(20);default:info" validate:"oneof=info warning error critical"`
	IPAddress       string     `json:"ip_address" gorm:"type:varchar(45)" validate:"ip"`
	UserAgent       string     `json:"user_agent" gorm:"type:text"`
	Endpoint        string     `json:"endpoint" gorm:"type:varchar(255)"`
	HTTPMethod      string     `json:"http_method" gorm:"type:varchar(10)"`
	StatusCode      int        `json:"status_code"`
	EventData       string     `json:"event_data" gorm:"type:json"` // Additional event-specific data
	GeolocationData string     `json:"geolocation_data" gorm:"type:json"`
	SecurityFlags   string     `json:"security_flags" gorm:"type:json"`
	ComplianceData  string     `json:"compliance_data" gorm:"type:json"`
	ProcessedBy     string     `json:"processed_by" gorm:"type:varchar(100)"`   // Service/component that processed the event
	CorrelationID   string     `json:"correlation_id" gorm:"type:varchar(255)"` // For tracing across services
	CreatedAt       time.Time  `json:"created_at"`
}

// PasswordPolicy represents password policy enforcement
type PasswordPolicy struct {
	ID                    uuid.UUID  `json:"id" gorm:"primaryKey;type:uuid"`
	UserID                uuid.UUID  `json:"user_id" gorm:"type:uuid;uniqueIndex" validate:"required,uuid"`
	PasswordHash          string     `json:"-" gorm:"type:varchar(255)" validate:"required"`
	PasswordHistory       string     `json:"-" gorm:"type:text"` // Encrypted JSON array of previous hashes
	LastChangedAt         time.Time  `json:"last_changed_at"`
	ExpiresAt             *time.Time `json:"expires_at"`
	MustChangeOnNextLogin bool       `json:"must_change_on_next_login" gorm:"default:false"`
	FailedAttempts        int        `json:"failed_attempts" gorm:"default:0"`
	LastFailedAt          *time.Time `json:"last_failed_at"`
	LockedUntil           *time.Time `json:"locked_until"`
	PolicyVersion         string     `json:"policy_version" gorm:"type:varchar(20);default:v1"`
	ComplianceData        string     `json:"compliance_data" gorm:"type:json"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

// UserDataExport represents data export requests for compliance
type UserDataExport struct {
	ID              uuid.UUID  `json:"id" gorm:"primaryKey;type:uuid"`
	UserID          uuid.UUID  `json:"user_id" gorm:"type:uuid;index" validate:"required,uuid"`
	RequestType     string     `json:"request_type" gorm:"type:varchar(50)" validate:"required,oneof=data_export data_deletion account_closure"`
	Status          string     `json:"status" gorm:"type:varchar(50);default:pending" validate:"oneof=pending processing completed failed cancelled"`
	RequestedBy     uuid.UUID  `json:"requested_by" gorm:"type:uuid" validate:"required,uuid"` // User or admin who requested
	ProcessedBy     *uuid.UUID `json:"processed_by" gorm:"type:uuid"`                          // Admin who processed
	ExportFormat    string     `json:"export_format" gorm:"type:varchar(20);default:json" validate:"oneof=json xml csv"`
	IncludeDeleted  bool       `json:"include_deleted" gorm:"default:false"`
	DataCategories  string     `json:"data_categories" gorm:"type:json"`     // Categories of data to export/delete
	ExportURL       string     `json:"-" gorm:"type:varchar(500)"`           // Encrypted, temporary download URL
	ExportHash      string     `json:"export_hash" gorm:"type:varchar(255)"` // SHA-256 hash of export file
	ExpiresAt       *time.Time `json:"expires_at"`                           // When export URL expires
	CompletedAt     *time.Time `json:"completed_at"`
	LegalBasis      string     `json:"legal_basis" gorm:"type:varchar(100)" validate:"required"` // GDPR, CCPA, etc.
	ComplianceNotes string     `json:"compliance_notes" gorm:"type:text"`
	AuditTrail      string     `json:"audit_trail" gorm:"type:json"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}
