// Package risk provides risk management interfaces and types
package risk

import (
	"context"

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
	Level   RiskLevel `json:"level"`
	Score   int       `json:"score"`
	Factors []string  `json:"factors"`
}

// RiskService provides risk assessment functionality
type RiskService interface {
	AssessUserRisk(ctx context.Context, userID uuid.UUID) (*RiskAssessment, error)
	AssessTransactionRisk(ctx context.Context, userID uuid.UUID, amount decimal.Decimal) (*RiskAssessment, error)
}
