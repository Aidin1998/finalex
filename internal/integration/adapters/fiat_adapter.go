// Package adapters provides adapter implementations for service contracts
package adapters

import (
	"context"

	"github.com/Aidin1998/finalex/internal/integration/contracts"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// FiatServiceAdapter implements contracts.FiatServiceContract
// by adapting the contracts.FiatServiceContract interface
type FiatServiceAdapter struct {
	service contracts.FiatServiceContract
	logger  *zap.Logger
}

// NewFiatServiceAdapter creates a new fiat service adapter
func NewFiatServiceAdapter(service contracts.FiatServiceContract, logger *zap.Logger) *FiatServiceAdapter {
	return &FiatServiceAdapter{
		service: service,
		logger:  logger,
	}
}

// InitiateDeposit delegates to the contracts.FiatServiceContract
func (a *FiatServiceAdapter) InitiateDeposit(ctx context.Context, req *contracts.FiatDepositRequest) (*contracts.FiatDepositResponse, error) {
	a.logger.Debug("Initiating deposit", zap.String("user_id", req.UserID.String()), zap.String("currency", req.Currency))
	return a.service.InitiateDeposit(ctx, req)
}

// ProcessDepositReceipt delegates to the contracts.FiatServiceContract
func (a *FiatServiceAdapter) ProcessDepositReceipt(ctx context.Context, req *contracts.FiatReceiptRequest) (*contracts.FiatReceiptResponse, error) {
	a.logger.Debug("Processing deposit receipt", zap.String("user_id", req.UserID.String()), zap.String("currency", req.Currency))
	return a.service.ProcessDepositReceipt(ctx, req)
}

// GetDepositStatus delegates to the contracts.FiatServiceContract
func (a *FiatServiceAdapter) GetDepositStatus(ctx context.Context, depositID uuid.UUID) (*contracts.FiatDepositStatus, error) {
	return a.service.GetDepositStatus(ctx, depositID)
}

// GetDepositReceipt delegates to the contracts.FiatServiceContract
func (a *FiatServiceAdapter) GetDepositReceipt(ctx context.Context, receiptID uuid.UUID) (*contracts.FiatDepositReceipt, error) {
	return a.service.GetDepositReceipt(ctx, receiptID)
}

// InitiateWithdrawal delegates to the contracts.FiatServiceContract
func (a *FiatServiceAdapter) InitiateWithdrawal(ctx context.Context, req *contracts.FiatWithdrawalRequest) (*contracts.FiatWithdrawalResponse, error) {
	a.logger.Debug("Initiating withdrawal", zap.String("user_id", req.UserID.String()), zap.String("currency", req.Currency))
	return a.service.InitiateWithdrawal(ctx, req)
}

// ProcessWithdrawal delegates to the contracts.FiatServiceContract
func (a *FiatServiceAdapter) ProcessWithdrawal(ctx context.Context, withdrawalID uuid.UUID) error {
	return a.service.ProcessWithdrawal(ctx, withdrawalID)
}

// CancelWithdrawal delegates to the contracts.FiatServiceContract
func (a *FiatServiceAdapter) CancelWithdrawal(ctx context.Context, withdrawalID uuid.UUID, reason string) error {
	return a.service.CancelWithdrawal(ctx, withdrawalID, reason)
}

// GetWithdrawalStatus delegates to the contracts.FiatServiceContract
func (a *FiatServiceAdapter) GetWithdrawalStatus(ctx context.Context, withdrawalID uuid.UUID) (*contracts.FiatWithdrawalStatus, error) {
	return a.service.GetWithdrawalStatus(ctx, withdrawalID)
}

// AddBankAccount delegates to the contracts.FiatServiceContract
func (a *FiatServiceAdapter) AddBankAccount(ctx context.Context, req *contracts.AddBankAccountRequest) (*contracts.BankAccount, error) {
	return a.service.AddBankAccount(ctx, req)
}

// ValidateBankAccount delegates to the contracts.FiatServiceContract
func (a *FiatServiceAdapter) ValidateBankAccount(ctx context.Context, bankAccountID uuid.UUID) (*contracts.BankAccountValidation, error) {
	return a.service.ValidateBankAccount(ctx, bankAccountID)
}

// GetBankAccounts delegates to the contracts.FiatServiceContract
func (a *FiatServiceAdapter) GetBankAccounts(ctx context.Context, userID uuid.UUID) ([]*contracts.BankAccount, error) {
	return a.service.GetBankAccounts(ctx, userID)
}

// RemoveBankAccount delegates to the contracts.FiatServiceContract
func (a *FiatServiceAdapter) RemoveBankAccount(ctx context.Context, bankAccountID uuid.UUID) error {
	return a.service.RemoveBankAccount(ctx, bankAccountID)
}

// GetUserDeposits delegates to the contracts.FiatServiceContract
func (a *FiatServiceAdapter) GetUserDeposits(ctx context.Context, userID uuid.UUID, filter *contracts.FiatTransactionFilter) ([]*contracts.FiatDepositReceipt, int64, error) {
	return a.service.GetUserDeposits(ctx, userID, filter)
}

// GetUserWithdrawals delegates to the contracts.FiatServiceContract
func (a *FiatServiceAdapter) GetUserWithdrawals(ctx context.Context, userID uuid.UUID, filter *contracts.FiatTransactionFilter) ([]*contracts.FiatWithdrawal, int64, error) {
	return a.service.GetUserWithdrawals(ctx, userID, filter)
}

// GetFiatTransaction delegates to the contracts.FiatServiceContract
func (a *FiatServiceAdapter) GetFiatTransaction(ctx context.Context, transactionID uuid.UUID) (*contracts.FiatTransaction, error) {
	return a.service.GetFiatTransaction(ctx, transactionID)
}

// ValidateKYCForFiat delegates to the contracts.FiatServiceContract
func (a *FiatServiceAdapter) ValidateKYCForFiat(ctx context.Context, userID uuid.UUID, amount decimal.Decimal, currency string) (*contracts.FiatKYCValidation, error) {
	return a.service.ValidateKYCForFiat(ctx, userID, amount, currency)
}

// CheckFiatLimits delegates to the contracts.FiatServiceContract
func (a *FiatServiceAdapter) CheckFiatLimits(ctx context.Context, userID uuid.UUID, transactionType string, amount decimal.Decimal, currency string) (*contracts.FiatLimitCheck, error) {
	return a.service.CheckFiatLimits(ctx, userID, transactionType, amount, currency)
}

// ReportSuspiciousActivity delegates to the contracts.FiatServiceContract
func (a *FiatServiceAdapter) ReportSuspiciousActivity(ctx context.Context, req *contracts.SuspiciousActivityRequest) error {
	return a.service.ReportSuspiciousActivity(ctx, req)
}

// GetSupportedProviders delegates to the contracts.FiatServiceContract
func (a *FiatServiceAdapter) GetSupportedProviders(ctx context.Context) ([]*contracts.FiatProvider, error) {
	return a.service.GetSupportedProviders(ctx)
}

// GetProviderStatus delegates to the contracts.FiatServiceContract
func (a *FiatServiceAdapter) GetProviderStatus(ctx context.Context, providerID string) (*contracts.ProviderStatus, error) {
	return a.service.GetProviderStatus(ctx, providerID)
}

// ValidateProviderSignature delegates to the contracts.FiatServiceContract
func (a *FiatServiceAdapter) ValidateProviderSignature(ctx context.Context, req *contracts.SignatureValidationRequest) error {
	return a.service.ValidateProviderSignature(ctx, req)
}

// GetFiatRates delegates to the contracts.FiatServiceContract
func (a *FiatServiceAdapter) GetFiatRates(ctx context.Context, baseCurrency, targetCurrency string) (*contracts.FiatExchangeRate, error) {
	return a.service.GetFiatRates(ctx, baseCurrency, targetCurrency)
}

// CalculateFees delegates to the contracts.FiatServiceContract
func (a *FiatServiceAdapter) CalculateFees(ctx context.Context, req *contracts.FeeCalculationRequest) (*contracts.FiatFeeCalculation, error) {
	return a.service.CalculateFees(ctx, req)
}

// HealthCheck delegates to the contracts.FiatServiceContract
func (a *FiatServiceAdapter) HealthCheck(ctx context.Context) (*contracts.HealthStatus, error) {
	return a.service.HealthCheck(ctx)
}

// GetMetrics delegates to the contracts.FiatServiceContract
func (a *FiatServiceAdapter) GetMetrics(ctx context.Context) (*contracts.FiatServiceMetrics, error) {
	return a.service.GetMetrics(ctx)
}
