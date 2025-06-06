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

// KYCLimits represents transaction limits based on KYC level
type KYCLimits struct {
	DailyWithdrawal   decimal.Decimal `json:"daily_withdrawal"`
	MonthlyWithdrawal decimal.Decimal `json:"monthly_withdrawal"`
	DailyDeposit      decimal.Decimal `json:"daily_deposit"`
	MonthlyDeposit    decimal.Decimal `json:"monthly_deposit"`
	SingleTransaction decimal.Decimal `json:"single_transaction"`
}

// MonitoringAlert represents a compliance monitoring alert
type MonitoringAlert struct {
	ID         uuid.UUID              `json:"id"`
	UserID     string                 `json:"user_id,omitempty"`
	AlertType  string                 `json:"alert_type"`
	Severity   AlertSeverity          `json:"severity"`
	Status     AlertStatus            `json:"status"`
	Message    string                 `json:"message"`
	Details    map[string]interface{} `json:"details,omitempty"`
	Timestamp  time.Time              `json:"timestamp"`
	ResolvedAt *time.Time             `json:"resolved_at,omitempty"`
	ResolvedBy *uuid.UUID             `json:"resolved_by,omitempty"`
	Resolution string                 `json:"resolution,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// AlertSeverity represents the severity level of an alert
type AlertSeverity int

const (
	AlertSeverityLow AlertSeverity = iota
	AlertSeverityMedium
	AlertSeverityHigh
	AlertSeverityCritical
)

func (a AlertSeverity) String() string {
	switch a {
	case AlertSeverityLow:
		return "low"
	case AlertSeverityMedium:
		return "medium"
	case AlertSeverityHigh:
		return "high"
	case AlertSeverityCritical:
		return "critical"
	default:
		return "unknown"
	}
}

// AlertStatus represents the status of an alert
type AlertStatus int

const (
	AlertStatusPending AlertStatus = iota
	AlertStatusAcknowledged
	AlertStatusInvestigating
	AlertStatusResolved
	AlertStatusDismissed
)

func (a AlertStatus) String() string {
	switch a {
	case AlertStatusPending:
		return "pending"
	case AlertStatusAcknowledged:
		return "acknowledged"
	case AlertStatusInvestigating:
		return "investigating"
	case AlertStatusResolved:
		return "resolved"
	case AlertStatusDismissed:
		return "dismissed"
	default:
		return "unknown"
	}
}

// MonitoringPolicy represents a monitoring policy configuration
type MonitoringPolicy struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	AlertType   string                 `json:"alert_type"`
	Enabled     bool                   `json:"enabled"`
	Conditions  []PolicyCondition      `json:"conditions"`
	Actions     []PolicyAction         `json:"actions"`
	Thresholds  map[string]interface{} `json:"thresholds"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	CreatedBy   uuid.UUID              `json:"created_by"`
}

// PolicyCondition represents a condition in a monitoring policy
type PolicyCondition struct {
	Field    string      `json:"field"`
	Operator string      `json:"operator"`
	Value    interface{} `json:"value"`
	Type     string      `json:"type"`
}

// PolicyAction represents an action to take when a policy is triggered
type PolicyAction struct {
	Type       string                 `json:"type"`
	Parameters map[string]interface{} `json:"parameters"`
}

// AlertFilter represents filter criteria for alerts
type AlertFilter struct {
	UserID    string        `json:"user_id,omitempty"`
	AlertType string        `json:"alert_type,omitempty"`
	Severity  AlertSeverity `json:"severity,omitempty"`
	Status    AlertStatus   `json:"status,omitempty"`
	StartTime time.Time     `json:"start_time,omitempty"`
	EndTime   time.Time     `json:"end_time,omitempty"`
	Limit     int           `json:"limit,omitempty"`
	Offset    int           `json:"offset,omitempty"`
}

// AlertSubscriber represents a subscriber to monitoring alerts
type AlertSubscriber interface {
	OnAlert(alert MonitoringAlert) error
	GetSubscriberID() string
	GetAlertTypes() []string
}

// MonitoringMetrics represents monitoring service metrics
type MonitoringMetrics struct {
	AlertsGenerated     int64         `json:"alerts_generated"`
	AlertsProcessed     int64         `json:"alerts_processed"`
	PolicyUpdates       int64         `json:"policy_updates"`
	SubscriberNotified  int64         `json:"subscriber_notified"`
	ProcessingLatency   time.Duration `json:"processing_latency"`
	AverageResponseTime time.Duration `json:"average_response_time"`
	ErrorRate           float64       `json:"error_rate"`
	LastUpdated         time.Time     `json:"last_updated"`
}

// ManipulationAlert represents a market manipulation alert
type ManipulationAlert struct {
	ID              uuid.UUID              `json:"id"`
	UserID          uuid.UUID              `json:"user_id"`
	Market          string                 `json:"market"`
	AlertType       ManipulationAlertType  `json:"alert_type"`
	Severity        AlertSeverity          `json:"severity"`
	RiskScore       decimal.Decimal        `json:"risk_score"`
	Description     string                 `json:"description"`
	Details         map[string]interface{} `json:"details"`
	Evidence        []string               `json:"evidence,omitempty"`
	Status          AlertStatus            `json:"status"`
	DetectedAt      time.Time              `json:"detected_at"`
	ResolvedAt      *time.Time             `json:"resolved_at,omitempty"`
	ResolvedBy      *uuid.UUID             `json:"resolved_by,omitempty"`
	Resolution      string                 `json:"resolution,omitempty"`
	InvestigationID *string                `json:"investigation_id,omitempty"`
}

// ManipulationAlertType represents the type of manipulation detected
type ManipulationAlertType int

const (
	ManipulationAlertWashTrading ManipulationAlertType = iota
	ManipulationAlertSpoofing
	ManipulationAlertLayering
	ManipulationAlertPumpAndDump
	ManipulationAlertFrontRunning
	ManipulationAlertInsiderTrading
)

func (m ManipulationAlertType) String() string {
	switch m {
	case ManipulationAlertWashTrading:
		return "wash_trading"
	case ManipulationAlertSpoofing:
		return "spoofing"
	case ManipulationAlertLayering:
		return "layering"
	case ManipulationAlertPumpAndDump:
		return "pump_and_dump"
	case ManipulationAlertFrontRunning:
		return "front_running"
	case ManipulationAlertInsiderTrading:
		return "insider_trading"
	default:
		return "unknown"
	}
}

// ManipulationPattern represents a detected manipulation pattern
type ManipulationPattern struct {
	Type        ManipulationAlertType  `json:"type"`
	Confidence  decimal.Decimal        `json:"confidence"`
	Description string                 `json:"description"`
	Evidence    []PatternEvidence      `json:"evidence"`
	TimeWindow  time.Duration          `json:"time_window"`
	DetectedAt  time.Time              `json:"detected_at"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// PatternEvidence represents evidence for a manipulation pattern
type PatternEvidence struct {
	Type        string                 `json:"type"`
	Description string                 `json:"description"`
	Value       interface{}            `json:"value"`
	Timestamp   time.Time              `json:"timestamp"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// TradingActivity represents trading activity for analysis
type TradingActivity struct {
	UserID     uuid.UUID     `json:"user_id"`
	Market     string        `json:"market"`
	Orders     []Order       `json:"orders"`
	Trades     []Trade       `json:"trades"`
	TimeWindow time.Duration `json:"time_window"`
	StartTime  time.Time     `json:"start_time"`
	EndTime    time.Time     `json:"end_time"`
}

// Order represents a trading order for manipulation detection
type Order struct {
	ID          string          `json:"id"`
	UserID      uuid.UUID       `json:"user_id"`
	Market      string          `json:"market"`
	Side        string          `json:"side"`
	Type        string          `json:"type"`
	Quantity    decimal.Decimal `json:"quantity"`
	Price       decimal.Decimal `json:"price"`
	Status      string          `json:"status"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	ExecutedAt  *time.Time      `json:"executed_at,omitempty"`
	CancelledAt *time.Time      `json:"cancelled_at,omitempty"`
}

// Trade represents a completed trade for manipulation detection
type Trade struct {
	ID        string          `json:"id"`
	Market    string          `json:"market"`
	BuyerID   uuid.UUID       `json:"buyer_id"`
	SellerID  uuid.UUID       `json:"seller_id"`
	Quantity  decimal.Decimal `json:"quantity"`
	Price     decimal.Decimal `json:"price"`
	Timestamp time.Time       `json:"timestamp"`
	OrderIDs  []string        `json:"order_ids"`
}

// ManipulationRequest represents a request for manipulation detection
type ManipulationRequest struct {
	RequestID string    `json:"request_id"`
	UserID    uuid.UUID `json:"user_id"`
	Market    string    `json:"market"`
	Orders    []Order   `json:"orders"`
	Trades    []Trade   `json:"trades"`
	Timestamp time.Time `json:"timestamp"`
	IPAddress string    `json:"ip_address,omitempty"`
}

// ManipulationResult represents the result of manipulation detection
type ManipulationResult struct {
	RequestID       string                 `json:"request_id"`
	UserID          uuid.UUID              `json:"user_id"`
	Market          string                 `json:"market"`
	DetectionStatus DetectionStatus        `json:"detection_status"`
	RiskScore       decimal.Decimal        `json:"risk_score"`
	Patterns        []ManipulationPattern  `json:"patterns"`
	Alerts          []ManipulationAlert    `json:"alerts"`
	ProcessedAt     time.Time              `json:"processed_at"`
	ProcessingTime  time.Duration          `json:"processing_time"`
	Details         map[string]interface{} `json:"details,omitempty"`
}

// DetectionStatus represents the status of manipulation detection
type DetectionStatus int

const (
	DetectionStatusPending DetectionStatus = iota
	DetectionStatusProcessing
	DetectionStatusCompleted
	DetectionStatusError
	DetectionStatusCancelled
)

func (d DetectionStatus) String() string {
	switch d {
	case DetectionStatusPending:
		return "pending"
	case DetectionStatusProcessing:
		return "processing"
	case DetectionStatusCompleted:
		return "completed"
	case DetectionStatusError:
		return "error"
	case DetectionStatusCancelled:
		return "cancelled"
	default:
		return "unknown"
	}
}
