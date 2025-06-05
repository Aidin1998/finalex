// Package trading provides trading services and interfaces
package trading

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// This file contains types for risk and compliance integration
// to help break circular dependencies with the compliance module

// RiskMetrics represents risk assessment metrics
type RiskMetrics struct {
	RiskScore         float64   `json:"risk_score"`
	Confidence        float64   `json:"confidence"`
	CalculatedAt      time.Time `json:"calculated_at"`
	Factors           []string  `json:"factors"`
	ThreatLevel       string    `json:"threat_level"` // low, medium, high, critical
	RecommendedAction string    `json:"recommended_action"`
}

// ComplianceResult represents the result of a compliance check
type ComplianceResult struct {
	Passed         bool      `json:"passed"`
	ViolationType  string    `json:"violation_type,omitempty"`
	Severity       string    `json:"severity,omitempty"`
	Message        string    `json:"message,omitempty"`
	CheckedAt      time.Time `json:"checked_at"`
	RequiredAction string    `json:"required_action,omitempty"`
}

// RiskService defines the interface for risk assessment services
type RiskService interface {
	AssessRisk(ctx context.Context, userID uuid.UUID, orderAmount decimal.Decimal, pair string) (*RiskMetrics, error)
	CheckCompliance(ctx context.Context, userID uuid.UUID, tradeData interface{}) (*ComplianceResult, error)
	IsEnabled() bool
}

// MockRiskService provides a mock implementation for testing
type MockRiskService struct{}

// NewMockRiskService creates a new mock risk service
func NewMockRiskService() RiskService {
	return &MockRiskService{}
}

// AssessRisk provides a mock risk assessment
func (m *MockRiskService) AssessRisk(ctx context.Context, userID uuid.UUID, orderAmount decimal.Decimal, pair string) (*RiskMetrics, error) {
	return &RiskMetrics{
		RiskScore:         0.1,
		Confidence:        0.95,
		CalculatedAt:      time.Now(),
		Factors:           []string{"normal_trading_pattern", "verified_user"},
		ThreatLevel:       "low",
		RecommendedAction: "allow",
	}, nil
}

// CheckCompliance provides a mock compliance check
func (m *MockRiskService) CheckCompliance(ctx context.Context, userID uuid.UUID, tradeData interface{}) (*ComplianceResult, error) {
	return &ComplianceResult{
		Passed:    true,
		CheckedAt: time.Now(),
	}, nil
}

// IsEnabled returns true for the mock service
func (m *MockRiskService) IsEnabled() bool {
	return true
}
