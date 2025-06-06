// Package interfaces provides common types and interfaces for the compliance module
package interfaces

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// RiskLevel represents the risk level of a user or transaction
type RiskLevel int

const (
	RiskLevelLow RiskLevel = iota
	RiskLevelMedium
	RiskLevelHigh
	RiskLevelCritical
)

func (r RiskLevel) String() string {
	switch r {
	case RiskLevelLow:
		return "low"
	case RiskLevelMedium:
		return "medium"
	case RiskLevelHigh:
		return "high"
	case RiskLevelCritical:
		return "critical"
	default:
		return "unknown"
	}
}

// ComplianceStatus represents the compliance status
type ComplianceStatus int

const (
	ComplianceStatusPending ComplianceStatus = iota
	ComplianceStatusApproved
	ComplianceStatusRejected
	ComplianceStatusBlocked
	ComplianceStatusUnderReview
)

func (c ComplianceStatus) String() string {
	switch c {
	case ComplianceStatusPending:
		return "pending"
	case ComplianceStatusApproved:
		return "approved"
	case ComplianceStatusRejected:
		return "rejected"
	case ComplianceStatusBlocked:
		return "blocked"
	case ComplianceStatusUnderReview:
		return "under_review"
	default:
		return "unknown"
	}
}

// KYCLevel represents the KYC verification level
type KYCLevel int

const (
	KYCLevelNone KYCLevel = iota
	KYCLevelBasic
	KYCLevelIntermediate
	KYCLevelAdvanced
	KYCLevelInstitutional
)

func (k KYCLevel) String() string {
	switch k {
	case KYCLevelNone:
		return "none"
	case KYCLevelBasic:
		return "basic"
	case KYCLevelIntermediate:
		return "intermediate"
	case KYCLevelAdvanced:
		return "advanced"
	case KYCLevelInstitutional:
		return "institutional"
	default:
		return "unknown"
	}
}

// ActivityType represents different user activities
type ActivityType int

const (
	ActivityLogin ActivityType = iota
	ActivityRegistration
	ActivityDeposit
	ActivityWithdrawal
	ActivityTrade
	ActivityTransfer
	ActivityKYCSubmission
	ActivityPasswordChange
	ActivityProfileUpdate
	ActivityAPIAccess
)

func (a ActivityType) String() string {
	switch a {
	case ActivityLogin:
		return "login"
	case ActivityRegistration:
		return "registration"
	case ActivityDeposit:
		return "deposit"
	case ActivityWithdrawal:
		return "withdrawal"
	case ActivityTrade:
		return "trade"
	case ActivityTransfer:
		return "transfer"
	case ActivityKYCSubmission:
		return "kyc_submission"
	case ActivityPasswordChange:
		return "password_change"
	case ActivityProfileUpdate:
		return "profile_update"
	case ActivityAPIAccess:
		return "api_access"
	default:
		return "unknown"
	}
}

// ComplianceRequest represents a compliance check request
type ComplianceRequest struct {
	UserID           uuid.UUID              `json:"user_id"`
	ActivityType     ActivityType           `json:"activity_type"`
	Amount           *decimal.Decimal       `json:"amount,omitempty"`
	Currency         string                 `json:"currency,omitempty"`
	IPAddress        string                 `json:"ip_address"`
	UserAgent        string                 `json:"user_agent"`
	DeviceID         string                 `json:"device_id"`
	Country          string                 `json:"country"`
	Email            string                 `json:"email,omitempty"`
	FirstName        string                 `json:"first_name,omitempty"`
	LastName         string                 `json:"last_name,omitempty"`
	DateOfBirth      *time.Time             `json:"date_of_birth,omitempty"`
	GeolocationData  map[string]interface{} `json:"geolocation_data,omitempty"`
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
	RequestTimestamp time.Time              `json:"request_timestamp"`
}

// ComplianceResult represents the result of a compliance check
type ComplianceResult struct {
	RequestID               uuid.UUID          `json:"request_id"`
	UserID                  uuid.UUID          `json:"user_id"`
	Status                  ComplianceStatus   `json:"status"`
	RiskLevel               RiskLevel          `json:"risk_level"`
	RiskScore               decimal.Decimal    `json:"risk_score"`
	Approved                bool               `json:"approved"`
	Blocked                 bool               `json:"blocked"`
	RequiresReview          bool               `json:"requires_review"`
	KYCRequired             bool               `json:"kyc_required"`
	RequiredKYCLevel        KYCLevel           `json:"required_kyc_level"`
	Reason                  string             `json:"reason,omitempty"`
	Flags                   []string           `json:"flags,omitempty"`
	Conditions              []string           `json:"conditions,omitempty"`
	RestrictedJurisdictions []string           `json:"restricted_jurisdictions,omitempty"`
	RequiredDocuments       []string           `json:"required_documents,omitempty"`
	ComplianceRequirements  map[string]string  `json:"compliance_requirements,omitempty"`
	TransactionLimits       *TransactionLimits `json:"transaction_limits,omitempty"`
	NextReviewDate          *time.Time         `json:"next_review_date,omitempty"`
	ExpiresAt               *time.Time         `json:"expires_at,omitempty"`
	ProcessedAt             time.Time          `json:"processed_at"`
	ProcessingDuration      time.Duration      `json:"processing_duration"`
}

// TransactionLimits represents transaction limits for a user
type TransactionLimits struct {
	DailyDeposit      decimal.Decimal `json:"daily_deposit"`
	DailyWithdrawal   decimal.Decimal `json:"daily_withdrawal"`
	MonthlyDeposit    decimal.Decimal `json:"monthly_deposit"`
	MonthlyWithdrawal decimal.Decimal `json:"monthly_withdrawal"`
	SingleTransaction decimal.Decimal `json:"single_transaction"`
	AMLThreshold      decimal.Decimal `json:"aml_threshold"`
	RequiresApproval  decimal.Decimal `json:"requires_approval"`
}

// AuditEvent represents an audit event
type AuditEvent struct {
	ID            uuid.UUID              `json:"id"`
	UserID        *uuid.UUID             `json:"user_id,omitempty"`
	EventType     string                 `json:"event_type"`
	Category      string                 `json:"category"`
	Severity      string                 `json:"severity"`
	Description   string                 `json:"description"`
	IPAddress     string                 `json:"ip_address,omitempty"`
	UserAgent     string                 `json:"user_agent,omitempty"`
	Resource      string                 `json:"resource,omitempty"`
	ResourceID    string                 `json:"resource_id,omitempty"`
	Action        string                 `json:"action,omitempty"`
	OldValues     map[string]interface{} `json:"old_values,omitempty"`
	NewValues     map[string]interface{} `json:"new_values,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
	RequestID     *uuid.UUID             `json:"request_id,omitempty"`
	SessionID     *uuid.UUID             `json:"session_id,omitempty"`
	CorrelationID string                 `json:"correlation_id,omitempty"`
	Hash          string                 `json:"hash"`
	PreviousHash  string                 `json:"previous_hash"`
	Timestamp     time.Time              `json:"timestamp"`
	ProcessedBy   string                 `json:"processed_by"`
}

// ManipulationAlert represents a market manipulation alert
type ManipulationAlert struct {
	ID             uuid.UUID              `json:"id"`
	UserID         string                 `json:"user_id"`
	Market         string                 `json:"market"`
	AlertType      string                 `json:"alert_type"`
	Severity       string                 `json:"severity"`
	Confidence     decimal.Decimal        `json:"confidence"`
	Description    string                 `json:"description"`
	Evidence       []Evidence             `json:"evidence"`
	Status         string                 `json:"status"`
	AutoBlocked    bool                   `json:"auto_blocked"`
	ReviewRequired bool                   `json:"review_required"`
	Metadata       map[string]interface{} `json:"metadata"`
	DetectedAt     time.Time              `json:"detected_at"`
	ResolvedAt     *time.Time             `json:"resolved_at,omitempty"`
	ProcessedBy    string                 `json:"processed_by,omitempty"`
}

// Evidence represents supporting evidence for alerts
type Evidence struct {
	Type        string                 `json:"type"`
	Description string                 `json:"description"`
	Data        map[string]interface{} `json:"data"`
	Timestamp   time.Time              `json:"timestamp"`
	Confidence  decimal.Decimal        `json:"confidence"`
}

// PolicyUpdate represents a dynamic policy update
type PolicyUpdate struct {
	ID          uuid.UUID              `json:"id"`
	PolicyType  string                 `json:"policy_type"`
	Version     string                 `json:"version"`
	Content     map[string]interface{} `json:"content"`
	EffectiveAt time.Time              `json:"effective_at"`
	ExpiresAt   *time.Time             `json:"expires_at,omitempty"`
	CreatedBy   uuid.UUID              `json:"created_by"`
	CreatedAt   time.Time              `json:"created_at"`
	Signature   string                 `json:"signature"`
}

// SanctionsListEntry represents a sanctions list entry
type SanctionsListEntry struct {
	ID          uuid.UUID  `json:"id"`
	ListSource  string     `json:"list_source"` // OFAC, EU, UN, etc.
	EntityType  string     `json:"entity_type"` // individual, entity, vessel, etc.
	Name        string     `json:"name"`
	Aliases     []string   `json:"aliases,omitempty"`
	DateOfBirth *time.Time `json:"date_of_birth,omitempty"`
	Nationality string     `json:"nationality,omitempty"`
	Address     string     `json:"address,omitempty"`
	LastUpdated time.Time  `json:"last_updated"`
	IsActive    bool       `json:"is_active"`
}

// MonitoringMetrics represents monitoring metrics
type MonitoringMetrics struct {
	EventsProcessed      int64              `json:"events_processed"`
	AlertsGenerated      int64              `json:"alerts_generated"`
	ComplianceViolations int64              `json:"compliance_violations"`
	ProcessingLatency    time.Duration      `json:"processing_latency"`
	ErrorRate            float64            `json:"error_rate"`
	ThroughputPerSecond  float64            `json:"throughput_per_second"`
	QueueDepth           int64              `json:"queue_depth"`
	WorkerUtilization    float64            `json:"worker_utilization"`
	ResourceUsage        map[string]float64 `json:"resource_usage"`
	LastUpdated          time.Time          `json:"last_updated"`
}

// KYCRequest represents a KYC verification request
type KYCRequest struct {
	ID              uuid.UUID              `json:"id"`
	UserID          uuid.UUID              `json:"user_id"`
	RequestType     string                 `json:"request_type"` // identity, document, enhanced
	Level           KYCLevel               `json:"level"`
	Documents       map[string]interface{} `json:"documents,omitempty"`
	PersonalInfo    map[string]interface{} `json:"personal_info,omitempty"`
	AddressInfo     map[string]interface{} `json:"address_info,omitempty"`
	ProviderData    map[string]interface{} `json:"provider_data,omitempty"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
	RequestedAt     time.Time              `json:"requested_at"`
	IPAddress       string                 `json:"ip_address,omitempty"`
	UserAgent       string                 `json:"user_agent,omitempty"`
}

// KYCResult represents the result of KYC verification
type KYCResult struct {
	RequestID       uuid.UUID        `json:"request_id"`
	UserID          uuid.UUID        `json:"user_id"`
	Status          ComplianceStatus `json:"status"`
	Level           KYCLevel         `json:"level"`
	RiskScore       decimal.Decimal  `json:"risk_score"`
	VerificationID  string           `json:"verification_id,omitempty"`
	Documents       []KYCDocument    `json:"documents,omitempty"`
	Flags           []string         `json:"flags,omitempty"`
	RequiredActions []string         `json:"required_actions,omitempty"`
	ExpiresAt       *time.Time       `json:"expires_at,omitempty"`
	ProcessedAt     time.Time        `json:"processed_at"`
	ProcessedBy     string           `json:"processed_by,omitempty"`
	Notes           string           `json:"notes,omitempty"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
}

// KYCDocument represents a document submitted for KYC
type KYCDocument struct {
	ID           uuid.UUID              `json:"id"`
	Type         string                 `json:"type"` // passport, driver_license, national_id, etc.
	Status       ComplianceStatus       `json:"status"`
	DocumentData map[string]interface{} `json:"document_data,omitempty"`
	ExtractionData map[string]interface{} `json:"extraction_data,omitempty"`
	VerificationData map[string]interface{} `json:"verification_data,omitempty"`
	QualityScore decimal.Decimal        `json:"quality_score"`
	Confidence   decimal.Decimal        `json:"confidence"`
	Flags        []string               `json:"flags,omitempty"`
	SubmittedAt  time.Time              `json:"submitted_at"`
	ProcessedAt  *time.Time             `json:"processed_at,omitempty"`
	ExpiresAt    *time.Time             `json:"expires_at,omitempty"`
}

// DocumentVerification represents document verification results
type DocumentVerification struct {
	DocumentID      uuid.UUID              `json:"document_id"`
	Type            string                 `json:"type"`
	Status          ComplianceStatus       `json:"status"`
	Authentic       bool                   `json:"authentic"`
	QualityScore    decimal.Decimal        `json:"quality_score"`
	ConfidenceScore decimal.Decimal        `json:"confidence_score"`
	ExtractedData   map[string]interface{} `json:"extracted_data,omitempty"`
	Anomalies       []string               `json:"anomalies,omitempty"`
	Warnings        []string               `json:"warnings,omitempty"`
	VerifiedFields  []string               `json:"verified_fields,omitempty"`
	ProcessedAt     time.Time              `json:"processed_at"`
	ExpiresAt       *time.Time             `json:"expires_at,omitempty"`
}

// KYCStatus represents KYC status information
type KYCStatus struct {
	UserID       uuid.UUID    `json:"user_id"`
	Level        KYCLevel     `json:"level"`
	Status       ComplianceStatus `json:"status"`
	CompletedAt  *time.Time   `json:"completed_at,omitempty"`
	ExpiresAt    *time.Time   `json:"expires_at,omitempty"`
	LastUpdated  time.Time    `json:"last_updated"`
	Documents    []KYCDocument `json:"documents,omitempty"`
	Limits       *KYCLimits   `json:"limits,omitempty"`
	Restrictions []string     `json:"restrictions,omitempty"`
}

// KYCLimits represents transaction limits based on KYC level
type KYCLimits struct {
	DailyWithdrawal   decimal.Decimal `json:"daily_withdrawal"`
	MonthlyWithdrawal decimal.Decimal `json:"monthly_withdrawal"`
	DailyDeposit      decimal.Decimal `json:"daily_deposit"`
	MonthlyDeposit    decimal.Decimal `json:"monthly_deposit"`
	SingleTransaction decimal.Decimal `json:"single_transaction"`
}
