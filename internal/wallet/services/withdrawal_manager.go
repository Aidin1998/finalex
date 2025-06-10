// Package services provides a stub WithdrawalManager implementation
// Implement WithdrawalManager or remove the stub comment. If not implemented, add a clear error or panic to prevent silent failure.

package services

import (
	"context"

	"github.com/Aidin1998/finalex/internal/wallet/interfaces"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type WithdrawalManagerImpl struct{}

func NewWithdrawalManager() interfaces.WithdrawalManager {
	return &WithdrawalManagerImpl{}
}

func (w *WithdrawalManagerImpl) InitiateWithdrawal(ctx context.Context, req *interfaces.WithdrawalRequest) (*interfaces.WithdrawalResponse, error) {
	return nil, nil
}
func (w *WithdrawalManagerImpl) ProcessWithdrawal(ctx context.Context, txID uuid.UUID) error {
	return nil
}
func (w *WithdrawalManagerImpl) UpdateWithdrawalStatus(ctx context.Context, txID uuid.UUID, status string, txHash string) error {
	return nil
}
func (w *WithdrawalManagerImpl) CancelWithdrawal(ctx context.Context, txID uuid.UUID, reason string) error {
	return nil
}
func (w *WithdrawalManagerImpl) EstimateWithdrawalFee(ctx context.Context, asset, network string, amount decimal.Decimal) (decimal.Decimal, error) {
	return decimal.Zero, nil
}
func (w *WithdrawalManagerImpl) ValidateWithdrawalRequest(ctx context.Context, req *interfaces.WithdrawalRequest) error {
	return nil
}
func (w *WithdrawalManagerImpl) RequiresApproval(ctx context.Context, req *interfaces.WithdrawalRequest) (bool, error) {
	return false, nil
}
