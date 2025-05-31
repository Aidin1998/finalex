package aml

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// RiskLevel represents the risk level of a user or activity
type RiskLevel string

const (
	RiskLevelLow      RiskLevel = "LOW"
	RiskLevelMedium   RiskLevel = "MEDIUM"
	RiskLevelHigh     RiskLevel = "HIGH"
	RiskLevelCritical RiskLevel = "CRITICAL"
)

// ActivityType represents the type of suspicious activity
type ActivityType string

const (
	ActivityTypeStructuring          ActivityType = "STRUCTURING"
	ActivityTypeRapidMovement        ActivityType = "RAPID_MOVEMENT"
	ActivityTypeUnusualVolume        ActivityType = "UNUSUAL_VOLUME"
	ActivityTypePatternMatching      ActivityType = "PATTERN_MATCHING"
	ActivityTypeGeographicAnomaly    ActivityType = "GEOGRAPHIC_ANOMALY"
	ActivityTypeMultipleAccounts     ActivityType = "MULTIPLE_ACCOUNTS"
	ActivityTypeHighRiskJurisdiction ActivityType = "HIGH_RISK_JURISDICTION"
	ActivityTypeCashIntensive        ActivityType = "CASH_INTENSIVE"
)

// ComplianceActionType represents the type of compliance action taken
type ComplianceActionType string

const (
	ActionTypeAlert       ComplianceActionType = "ALERT"
	ActionTypeBlock       ComplianceActionType = "BLOCK"
	ActionTypeFreeze      ComplianceActionType = "FREEZE"
	ActionTypeRestrict    ComplianceActionType = "RESTRICT"
	ActionTypeInvestigate ComplianceActionType = "INVESTIGATE"
	ActionTypeReport      ComplianceActionType = "REPORT"
	ActionTypeEscalate    ComplianceActionType = "ESCALATE"
	ActionTypeWhitelist   ComplianceActionType = "WHITELIST"
	ActionTypeBlacklist   ComplianceActionType = "BLACKLIST"
)

// InvestigationStatus represents the status of an investigation case
type InvestigationStatus string

const (
	StatusOpen       InvestigationStatus = "OPEN"
	StatusInProgress InvestigationStatus = "IN_PROGRESS"
	StatusClosed     InvestigationStatus = "CLOSED"
	StatusEscalated  InvestigationStatus = "ESCALATED"
	StatusSuspended  InvestigationStatus = "SUSPENDED"
)

// ReportType represents the type of regulatory report
type ReportType string

const (
	ReportTypeSAR ReportType = "SAR" // Suspicious Activity Report
	ReportTypeCTR ReportType = "CTR" // Currency Transaction Report
	ReportTypeFTR ReportType = "FTR" // Funds Transfer Report
	ReportTypeSTR ReportType = "STR" // Suspicious Transaction Report
)

// ReportStatus represents the status of a regulatory report
type ReportStatus string

const (
	ReportStatusDraft     ReportStatus = "DRAFT"
	ReportStatusPending   ReportStatus = "PENDING"
	ReportStatusSubmitted ReportStatus = "SUBMITTED"
	ReportStatusAccepted  ReportStatus = "ACCEPTED"
	ReportStatusRejected  ReportStatus = "REJECTED"
)

// AMLUser represents a user with AML risk profile and compliance data
type AMLUser struct {
	ID                uuid.UUID              `json:"id" db:"id"`
	UserID            uuid.UUID              `json:"user_id" db:"user_id"`
	RiskLevel         RiskLevel              `json:"risk_level" db:"risk_level"`
	RiskScore         float64                `json:"risk_score" db:"risk_score"`
	KYCStatus         string                 `json:"kyc_status" db:"kyc_status"`
	IsBlacklisted     bool                   `json:"is_blacklisted" db:"is_blacklisted"`
	IsWhitelisted     bool                   `json:"is_whitelisted" db:"is_whitelisted"`
	LastRiskUpdate    time.Time              `json:"last_risk_update" db:"last_risk_update"`
	CountryCode       string                 `json:"country_code" db:"country_code"`
	IsHighRiskCountry bool                   `json:"is_high_risk_country" db:"is_high_risk_country"`
	PEPStatus         bool                   `json:"pep_status" db:"pep_status"` // Politically Exposed Person
	SanctionStatus    bool                   `json:"sanction_status" db:"sanction_status"`
	CustomerType      string                 `json:"customer_type" db:"customer_type"` // Individual, Corporate, etc.
	BusinessType      string                 `json:"business_type" db:"business_type"`
	RiskFactors       map[string]interface{} `json:"risk_factors" db:"risk_factors"`
	CreatedAt         time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt         time.Time              `json:"updated_at" db:"updated_at"`
}

// SuspiciousActivity represents a detected suspicious activity
type SuspiciousActivity struct {
	ID              uuid.UUID              `json:"id" db:"id"`
	UserID          uuid.UUID              `json:"user_id" db:"user_id"`
	ActivityType    ActivityType           `json:"activity_type" db:"activity_type"`
	RiskScore       float64                `json:"risk_score" db:"risk_score"`
	Severity        RiskLevel              `json:"severity" db:"severity"`
	Description     string                 `json:"description" db:"description"`
	TransactionIDs  []uuid.UUID            `json:"transaction_ids" db:"transaction_ids"`
	Amount          float64                `json:"amount" db:"amount"`
	Currency        string                 `json:"currency" db:"currency"`
	Pattern         string                 `json:"pattern" db:"pattern"`
	Indicators      map[string]interface{} `json:"indicators" db:"indicators"`
	DetectionRuleID uuid.UUID              `json:"detection_rule_id" db:"detection_rule_id"`
	IsReviewed      bool                   `json:"is_reviewed" db:"is_reviewed"`
	ReviewedBy      *uuid.UUID             `json:"reviewed_by" db:"reviewed_by"`
	ReviewNotes     string                 `json:"review_notes" db:"review_notes"`
	IsFalsePositive bool                   `json:"is_false_positive" db:"is_false_positive"`
	IsEscalated     bool                   `json:"is_escalated" db:"is_escalated"`
	EscalatedAt     *time.Time             `json:"escalated_at" db:"escalated_at"`
	DetectedAt      time.Time              `json:"detected_at" db:"detected_at"`
	CreatedAt       time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at" db:"updated_at"`
}

// ComplianceAction represents an action taken for compliance purposes
type ComplianceAction struct {
	ID           uuid.UUID              `json:"id" db:"id"`
	UserID       uuid.UUID              `json:"user_id" db:"user_id"`
	ActionType   ComplianceActionType   `json:"action_type" db:"action_type"`
	Reason       string                 `json:"reason" db:"reason"`
	Description  string                 `json:"description" db:"description"`
	TakenBy      uuid.UUID              `json:"taken_by" db:"taken_by"` // Staff member ID
	IsAutomated  bool                   `json:"is_automated" db:"is_automated"`
	RuleID       *uuid.UUID             `json:"rule_id" db:"rule_id"`
	ActivityID   *uuid.UUID             `json:"activity_id" db:"activity_id"`
	CaseID       *uuid.UUID             `json:"case_id" db:"case_id"`
	ExpiresAt    *time.Time             `json:"expires_at" db:"expires_at"`
	IsActive     bool                   `json:"is_active" db:"is_active"`
	Parameters   map[string]interface{} `json:"parameters" db:"parameters"`
	Result       string                 `json:"result" db:"result"`
	ErrorMessage string                 `json:"error_message" db:"error_message"`
	ExecutedAt   time.Time              `json:"executed_at" db:"executed_at"`
	CreatedAt    time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at" db:"updated_at"`
}

// InvestigationCase represents a compliance investigation case
type InvestigationCase struct {
	ID          uuid.UUID              `json:"id" db:"id"`
	CaseNumber  string                 `json:"case_number" db:"case_number"`
	UserID      uuid.UUID              `json:"user_id" db:"user_id"`
	Priority    RiskLevel              `json:"priority" db:"priority"`
	Status      InvestigationStatus    `json:"status" db:"status"`
	Title       string                 `json:"title" db:"title"`
	Description string                 `json:"description" db:"description"`
	AssignedTo  *uuid.UUID             `json:"assigned_to" db:"assigned_to"` // Investigator ID
	AssignedBy  uuid.UUID              `json:"assigned_by" db:"assigned_by"`
	ActivityIDs []uuid.UUID            `json:"activity_ids" db:"activity_ids"`
	ActionIDs   []uuid.UUID            `json:"action_ids" db:"action_ids"`
	Tags        []string               `json:"tags" db:"tags"`
	Notes       string                 `json:"notes" db:"notes"`
	Evidence    map[string]interface{} `json:"evidence" db:"evidence"`
	Timeline    []CaseTimelineEntry    `json:"timeline" db:"timeline"`
	DueDate     *time.Time             `json:"due_date" db:"due_date"`
	ClosedAt    *time.Time             `json:"closed_at" db:"closed_at"`
	ClosedBy    *uuid.UUID             `json:"closed_by" db:"closed_by"`
	Resolution  string                 `json:"resolution" db:"resolution"`
	SARFiled    bool                   `json:"sar_filed" db:"sar_filed"`
	SARReportID *uuid.UUID             `json:"sar_report_id" db:"sar_report_id"`
	IsEscalated bool                   `json:"is_escalated" db:"is_escalated"`
	EscalatedAt *time.Time             `json:"escalated_at" db:"escalated_at"`
	CreatedAt   time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at" db:"updated_at"`
}

// CaseTimelineEntry represents an entry in the investigation timeline
type CaseTimelineEntry struct {
	ID          uuid.UUID `json:"id"`
	CaseID      uuid.UUID `json:"case_id"`
	Action      string    `json:"action"`
	Description string    `json:"description"`
	PerformedBy uuid.UUID `json:"performed_by"`
	Timestamp   time.Time `json:"timestamp"`
}

// RegulatoryReport represents a regulatory report (SAR, CTR, etc.)
type RegulatoryReport struct {
	ID               uuid.UUID              `json:"id" db:"id"`
	ReportNumber     string                 `json:"report_number" db:"report_number"`
	ReportType       ReportType             `json:"report_type" db:"report_type"`
	Status           ReportStatus           `json:"status" db:"status"`
	UserID           uuid.UUID              `json:"user_id" db:"user_id"`
	CaseID           *uuid.UUID             `json:"case_id" db:"case_id"`
	ActivityIDs      []uuid.UUID            `json:"activity_ids" db:"activity_ids"`
	FilingDate       *time.Time             `json:"filing_date" db:"filing_date"`
	ReportingPeriod  string                 `json:"reporting_period" db:"reporting_period"`
	TotalAmount      float64                `json:"total_amount" db:"total_amount"`
	Currency         string                 `json:"currency" db:"currency"`
	NarrativeSummary string                 `json:"narrative_summary" db:"narrative_summary"`
	RegulatoryBody   string                 `json:"regulatory_body" db:"regulatory_body"`
	ReferenceNumber  string                 `json:"reference_number" db:"reference_number"`
	PreparedBy       uuid.UUID              `json:"prepared_by" db:"prepared_by"`
	ReviewedBy       *uuid.UUID             `json:"reviewed_by" db:"reviewed_by"`
	ApprovedBy       *uuid.UUID             `json:"approved_by" db:"approved_by"`
	SubmittedBy      *uuid.UUID             `json:"submitted_by" db:"submitted_by"`
	ReportData       map[string]interface{} `json:"report_data" db:"report_data"`
	Attachments      []ReportAttachment     `json:"attachments" db:"attachments"`
	SubmissionStatus string                 `json:"submission_status" db:"submission_status"`
	ResponseData     map[string]interface{} `json:"response_data" db:"response_data"`
	DueDate          *time.Time             `json:"due_date" db:"due_date"`
	SubmittedAt      *time.Time             `json:"submitted_at" db:"submitted_at"`
	CreatedAt        time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time              `json:"updated_at" db:"updated_at"`
}

// ReportAttachment represents an attachment to a regulatory report
type ReportAttachment struct {
	ID          uuid.UUID `json:"id"`
	ReportID    uuid.UUID `json:"report_id"`
	Filename    string    `json:"filename"`
	ContentType string    `json:"content_type"`
	Size        int64     `json:"size"`
	UploadedBy  uuid.UUID `json:"uploaded_by"`
	UploadedAt  time.Time `json:"uploaded_at"`
}

// Additional types for AML service implementation

// LimitType represents the type of risk limit
type LimitType string

const (
	UserLimitType   LimitType = "USER"
	MarketLimitType LimitType = "MARKET"
	GlobalLimitType LimitType = "GLOBAL"
)

// LimitConfig holds configuration for various risk limits
type LimitConfig struct {
	UserLimits   map[string]decimal.Decimal `json:"user_limits"`
	MarketLimits map[string]decimal.Decimal `json:"market_limits"`
	GlobalLimit  decimal.Decimal            `json:"global_limit"`
}

// RiskMetrics holds real-time risk calculation results
type RiskMetrics struct {
	UserID            string          `json:"user_id"`
	TotalExposure     decimal.Decimal `json:"total_exposure"`
	ValueAtRisk       decimal.Decimal `json:"value_at_risk"`
	LeverageRatio     decimal.Decimal `json:"leverage_ratio"`
	MarginUtilization decimal.Decimal `json:"margin_utilization"`
	PortfolioValue    decimal.Decimal `json:"portfolio_value"`
	RiskScore         decimal.Decimal `json:"risk_score"`
	LastCalculated    time.Time       `json:"last_calculated"`
}

// TransactionRecord holds transaction data for compliance analysis
type TransactionRecord struct {
	ID          string                 `json:"id"`
	UserID      string                 `json:"user_id"`
	Type        string                 `json:"type"` // "deposit", "withdrawal", "trade"
	Amount      decimal.Decimal        `json:"amount"`
	Currency    string                 `json:"currency"`
	Timestamp   time.Time              `json:"timestamp"`
	Source      string                 `json:"source"`
	Destination string                 `json:"destination"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// ComplianceAlert represents a compliance violation alert
type ComplianceAlert struct {
	ID         string                 `json:"id"`
	UserID     string                 `json:"user_id"`
	Type       string                 `json:"type"`
	Severity   string                 `json:"severity"`
	Message    string                 `json:"message"`
	Status     string                 `json:"status"`
	AssignedTo string                 `json:"assigned_to"`
	Notes      string                 `json:"notes"`
	CreatedAt  time.Time              `json:"created_at"`
	UpdatedAt  time.Time              `json:"updated_at"`
	Metadata   map[string]interface{} `json:"metadata"`
}

// ComplianceRule defines a compliance checking rule
type ComplianceRule struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Type        string                 `json:"type"` // "aml", "kyt", "pattern", "velocity"
	Parameters  map[string]interface{} `json:"parameters"`
	IsActive    bool                   `json:"is_active"`
	Severity    string                 `json:"severity"` // "low", "medium", "high", "critical"
	Description string                 `json:"description"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

// DashboardSubscriber represents a dashboard subscription
type DashboardSubscriber struct {
	ID        string                 `json:"id"`
	UserID    string                 `json:"user_id"`
	Filters   map[string]interface{} `json:"filters"`
	Active    bool                   `json:"active"`
	CreatedAt time.Time              `json:"created_at"`
}

// AlertNotification represents a system alert notification
type AlertNotification struct {
	ID             string                 `json:"id"`
	Type           string                 `json:"type"`
	Priority       string                 `json:"priority"`
	Title          string                 `json:"title"`
	Message        string                 `json:"message"`
	UserID         string                 `json:"user_id"`
	Data           map[string]interface{} `json:"data"`
	Acknowledged   bool                   `json:"acknowledged"`
	AcknowledgedBy string                 `json:"acknowledged_by"`
	AcknowledgedAt *time.Time             `json:"acknowledged_at"`
	CreatedAt      time.Time              `json:"created_at"`
}

// ReportingCriteria defines criteria for regulatory reporting
type ReportingCriteria struct {
	ReportType    ReportType             `json:"report_type"`
	StartDate     time.Time              `json:"start_date"`
	EndDate       time.Time              `json:"end_date"`
	UserIDs       []string               `json:"user_ids"`
	ActivityTypes []ActivityType         `json:"activity_types"`
	MinAmount     decimal.Decimal        `json:"min_amount"`
	Currency      string                 `json:"currency"`
	Jurisdiction  string                 `json:"jurisdiction"`
	Filters       map[string]interface{} `json:"filters"`
}
