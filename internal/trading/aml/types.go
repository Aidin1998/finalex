// Package aml provides Anti-Money Laundering and compliance types
package aml

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
)

// LimitType represents different types of risk limits
type LimitType string

const (
	LimitTypeDaily       LimitType = "daily"
	LimitTypeMonthly     LimitType = "monthly"
	LimitTypeTransaction LimitType = "transaction"
	LimitTypeVolume      LimitType = "volume"
	LimitTypePosition    LimitType = "position"
)

// ComplianceRule represents a rule for compliance checking
type ComplianceRule struct {
	ID          string    `json:"id" gorm:"primaryKey"`
	Name        string    `json:"name" gorm:"not null"`
	Description string    `json:"description"`
	RuleType    string    `json:"rule_type" gorm:"not null"`
	Conditions  string    `json:"conditions" gorm:"type:text"`
	Actions     string    `json:"actions" gorm:"type:text"`
	Enabled     bool      `json:"enabled" gorm:"default:true"`
	Priority    int       `json:"priority" gorm:"default:1"`
	CreatedAt   time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt   time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

// RiskLimit represents a risk limit configuration
type RiskLimit struct {
	ID        string          `json:"id" gorm:"primaryKey"`
	Type      LimitType       `json:"type" gorm:"not null"`
	UserID    string          `json:"user_id,omitempty"`
	Symbol    string          `json:"symbol,omitempty"`
	Amount    decimal.Decimal `json:"amount" gorm:"type:decimal(20,8)"`
	CreatedAt time.Time       `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt time.Time       `json:"updated_at" gorm:"autoUpdateTime"`
}

// ComplianceAlert represents a compliance alert
type ComplianceAlert struct {
	ID          string    `json:"id" gorm:"primaryKey"`
	UserID      string    `json:"user_id" gorm:"not null"`
	AlertType   string    `json:"alert_type" gorm:"not null"`
	Severity    string    `json:"severity" gorm:"not null"`
	Title       string    `json:"title" gorm:"not null"`
	Description string    `json:"description" gorm:"type:text"`
	Status      string    `json:"status" gorm:"default:'active'"`
	AssignedTo  string    `json:"assigned_to,omitempty"`
	Notes       string    `json:"notes,omitempty" gorm:"type:text"`
	CreatedAt   time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt   time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

// DashboardMetrics represents compliance dashboard metrics
type DashboardMetrics struct {
	ActiveAlerts        int64            `json:"active_alerts"`
	ResolvedAlerts      int64            `json:"resolved_alerts"`
	HighRiskUsers       int64            `json:"high_risk_users"`
	TotalTransactions   int64            `json:"total_transactions"`
	FlaggedTransactions int64            `json:"flagged_transactions"`
	ComplianceScore     float64          `json:"compliance_score"`
	MetricsByType       map[string]int64 `json:"metrics_by_type"`
	LastUpdated         time.Time        `json:"last_updated"`
}

// UserExemption represents a user exemption from certain compliance rules
type UserExemption struct {
	ID        string    `json:"id" gorm:"primaryKey"`
	UserID    string    `json:"user_id" gorm:"not null;uniqueIndex"`
	Reason    string    `json:"reason" gorm:"not null"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

// RiskService defines the interface for risk assessment and compliance services
type RiskService interface {
	// Risk limit management
	GetLimits(ctx context.Context) ([]RiskLimit, error)
	CreateRiskLimit(ctx context.Context, limitType LimitType, id string, amount decimal.Decimal) error
	UpdateRiskLimit(ctx context.Context, limitType LimitType, id string, amount decimal.Decimal) error
	DeleteRiskLimit(ctx context.Context, limitType LimitType, id string) error

	// User exemption management
	GetExemptions(ctx context.Context) ([]UserExemption, error)
	CreateExemption(ctx context.Context, userID string) error
	DeleteExemption(ctx context.Context, userID string) error

	// Risk calculation
	CalculateRealTimeRisk(ctx context.Context, userID string) (interface{}, error)
	BatchCalculateRisk(ctx context.Context, userIDs []string) (interface{}, error)

	// Dashboard and metrics
	GetDashboardMetrics(ctx context.Context) (*DashboardMetrics, error)

	// Market data updates
	UpdateMarketData(ctx context.Context, symbol string, price, volatility decimal.Decimal) error

	// Compliance alerts
	GetActiveComplianceAlerts(ctx context.Context) ([]ComplianceAlert, error)
	UpdateComplianceAlertStatus(ctx context.Context, alertID, status, assignedTo, notes string) error

	// Compliance rules
	AddComplianceRule(ctx context.Context, rule *ComplianceRule) error

	// Reporting
	GenerateReport(ctx context.Context, reportType string, startTime, endTime time.Time) (interface{}, error)
}
