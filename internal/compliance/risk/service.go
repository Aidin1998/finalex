// Package risk provides risk management functionality for the compliance module
package risk

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// RiskLevel represents different risk levels
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
	}
	return "unknown"
}

// RiskAssessment represents a risk assessment result
type RiskAssessment struct {
	Level      RiskLevel              `json:"level"`
	Score      decimal.Decimal        `json:"score"`
	Factors    []string               `json:"factors"`
	Details    map[string]interface{} `json:"details"`
	Timestamp  time.Time              `json:"timestamp"`
	ExpiresAt  *time.Time             `json:"expires_at,omitempty"`
	Confidence decimal.Decimal        `json:"confidence"`
}

// RiskFactors represents various risk factors
type RiskFactors struct {
	CountryRisk        RiskLevel `json:"country_risk"`
	TransactionPattern RiskLevel `json:"transaction_pattern"`
	VelocityRisk       RiskLevel `json:"velocity_risk"`
	AmountRisk         RiskLevel `json:"amount_risk"`
	BehaviorRisk       RiskLevel `json:"behavior_risk"`
	DeviceRisk         RiskLevel `json:"device_risk"`
	HistoricalRisk     RiskLevel `json:"historical_risk"`
}

// TransactionRiskContext provides context for transaction risk assessment
type TransactionRiskContext struct {
	UserID          uuid.UUID              `json:"user_id"`
	TransactionID   uuid.UUID              `json:"transaction_id"`
	Amount          decimal.Decimal        `json:"amount"`
	Currency        string                 `json:"currency"`
	TransactionType string                 `json:"transaction_type"`
	IPAddress       string                 `json:"ip_address"`
	UserAgent       string                 `json:"user_agent"`
	DeviceID        string                 `json:"device_id"`
	Country         string                 `json:"country"`
	Timestamp       time.Time              `json:"timestamp"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
}

// UserRiskContext provides context for user risk assessment
type UserRiskContext struct {
	UserID             uuid.UUID              `json:"user_id"`
	Email              string                 `json:"email"`
	Country            string                 `json:"country"`
	RegistrationDate   time.Time              `json:"registration_date"`
	LastLoginDate      *time.Time             `json:"last_login_date"`
	KYCLevel           int                    `json:"kyc_level"`
	VerificationStatus string                 `json:"verification_status"`
	TransactionHistory []TransactionSummary   `json:"transaction_history"`
	Metadata           map[string]interface{} `json:"metadata,omitempty"`
}

// TransactionSummary provides a summary of user transactions for risk assessment
type TransactionSummary struct {
	Date     time.Time       `json:"date"`
	Amount   decimal.Decimal `json:"amount"`
	Currency string          `json:"currency"`
	Type     string          `json:"type"`
	Status   string          `json:"status"`
	Country  string          `json:"country"`
}

// RiskPolicy represents a risk assessment policy
type RiskPolicy struct {
	ID         string                 `json:"id"`
	Name       string                 `json:"name"`
	Type       string                 `json:"type"`
	Enabled    bool                   `json:"enabled"`
	Rules      []RiskRule             `json:"rules"`
	Thresholds map[string]interface{} `json:"thresholds"`
	Actions    []RiskAction           `json:"actions"`
	CreatedAt  time.Time              `json:"created_at"`
	UpdatedAt  time.Time              `json:"updated_at"`
}

// RiskRule represents a single risk assessment rule
type RiskRule struct {
	ID         string                 `json:"id"`
	Name       string                 `json:"name"`
	Condition  string                 `json:"condition"`
	Weight     decimal.Decimal        `json:"weight"`
	Enabled    bool                   `json:"enabled"`
	Parameters map[string]interface{} `json:"parameters"`
}

// RiskAction represents an action to take based on risk assessment
type RiskAction struct {
	Type       string                 `json:"type"`
	Threshold  decimal.Decimal        `json:"threshold"`
	Parameters map[string]interface{} `json:"parameters"`
}

// RiskService provides comprehensive risk assessment functionality
type RiskService interface {
	// User risk assessment
	AssessUserRisk(ctx context.Context, userCtx *UserRiskContext) (*RiskAssessment, error)
	GetUserRiskProfile(ctx context.Context, userID uuid.UUID) (*RiskProfile, error)
	UpdateUserRiskProfile(ctx context.Context, userID uuid.UUID, profile *RiskProfile) error

	// Transaction risk assessment
	AssessTransactionRisk(ctx context.Context, txCtx *TransactionRiskContext) (*RiskAssessment, error)
	PreCheckTransaction(ctx context.Context, txCtx *TransactionRiskContext) (*RiskAssessment, error)

	// Risk monitoring
	MonitorRiskEvents(ctx context.Context, userID uuid.UUID) error
	GetRiskMetrics(ctx context.Context, filters *RiskMetricsFilter) (*RiskMetrics, error)

	// Risk policies
	GetRiskPolicies(ctx context.Context) ([]*RiskPolicy, error)
	UpdateRiskPolicy(ctx context.Context, policy *RiskPolicy) error

	// Risk scoring
	CalculateRiskScore(ctx context.Context, factors *RiskFactors) (*RiskAssessment, error)
	GetRiskFactors(ctx context.Context, userID uuid.UUID) (*RiskFactors, error)
}

// RiskProfile represents a comprehensive user risk profile
type RiskProfile struct {
	UserID            uuid.UUID              `json:"user_id"`
	OverallRiskLevel  RiskLevel              `json:"overall_risk_level"`
	RiskScore         decimal.Decimal        `json:"risk_score"`
	Factors           *RiskFactors           `json:"factors"`
	TransactionLimits *TransactionLimits     `json:"transaction_limits"`
	Restrictions      []string               `json:"restrictions"`
	LastAssessment    time.Time              `json:"last_assessment"`
	NextReview        *time.Time             `json:"next_review"`
	AssessmentHistory []RiskAssessment       `json:"assessment_history"`
	Flags             []string               `json:"flags"`
	Notes             string                 `json:"notes"`
	Metadata          map[string]interface{} `json:"metadata"`
	CreatedAt         time.Time              `json:"created_at"`
	UpdatedAt         time.Time              `json:"updated_at"`
}

// TransactionLimits represents transaction limits based on risk assessment
type TransactionLimits struct {
	DailyDeposit      decimal.Decimal `json:"daily_deposit"`
	DailyWithdrawal   decimal.Decimal `json:"daily_withdrawal"`
	MonthlyDeposit    decimal.Decimal `json:"monthly_deposit"`
	MonthlyWithdrawal decimal.Decimal `json:"monthly_withdrawal"`
	SingleTransaction decimal.Decimal `json:"single_transaction"`
	VelocityLimits    *VelocityLimits `json:"velocity_limits"`
}

// VelocityLimits represents velocity-based transaction limits
type VelocityLimits struct {
	TransactionsPerHour  int `json:"transactions_per_hour"`
	TransactionsPerDay   int `json:"transactions_per_day"`
	TransactionsPerWeek  int `json:"transactions_per_week"`
	TransactionsPerMonth int `json:"transactions_per_month"`
}

// RiskMetricsFilter represents filters for risk metrics
type RiskMetricsFilter struct {
	UserID    *uuid.UUID `json:"user_id,omitempty"`
	RiskLevel *RiskLevel `json:"risk_level,omitempty"`
	StartTime time.Time  `json:"start_time"`
	EndTime   time.Time  `json:"end_time"`
	Limit     int        `json:"limit,omitempty"`
	Offset    int        `json:"offset,omitempty"`
}

// RiskMetrics represents risk-related metrics
type RiskMetrics struct {
	TotalAssessments  int64                  `json:"total_assessments"`
	HighRiskUsers     int64                  `json:"high_risk_users"`
	CriticalRiskUsers int64                  `json:"critical_risk_users"`
	AverageRiskScore  decimal.Decimal        `json:"average_risk_score"`
	RiskDistribution  map[RiskLevel]int64    `json:"risk_distribution"`
	TrendData         map[string]interface{} `json:"trend_data"`
	ProcessingLatency time.Duration          `json:"processing_latency"`
	Timestamp         time.Time              `json:"timestamp"`
}
