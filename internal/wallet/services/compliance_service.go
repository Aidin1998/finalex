// Package services provides the ComplianceService implementation for the wallet module
package services

import (
	"context"

	"github.com/Aidin1998/finalex/internal/wallet/interfaces"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ComplianceServiceImpl implements interfaces.ComplianceService
// It delegates to the compliance module if available, otherwise performs basic checks

type ComplianceServiceImpl struct {
	compliance interfaces.ComplianceService // can be nil for basic checks
}

func NewComplianceService(compliance interfaces.ComplianceService) interfaces.ComplianceService {
	return &ComplianceServiceImpl{compliance: compliance}
}

func (c *ComplianceServiceImpl) CheckWithdrawal(ctx context.Context, userID uuid.UUID, asset string, amount decimal.Decimal) error {
	if c.compliance != nil {
		return c.compliance.CheckWithdrawal(ctx, userID, asset, amount)
	}
	// Basic check: amount must be positive
	if amount.LessThanOrEqual(decimal.Zero) {
		return interfaces.ErrInvalidAmount
	}
	return nil
}

func (c *ComplianceServiceImpl) CheckDeposit(ctx context.Context, userID uuid.UUID, asset string, amount decimal.Decimal) error {
	if c.compliance != nil {
		return c.compliance.CheckDeposit(ctx, userID, asset, amount)
	}
	// Basic check: amount must be positive
	if amount.LessThanOrEqual(decimal.Zero) {
		return interfaces.ErrInvalidAmount
	}
	return nil
}
