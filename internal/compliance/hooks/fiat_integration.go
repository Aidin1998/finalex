package hooks

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// FiatIntegration handles compliance integration for fiat operations
type FiatIntegration struct {
	hookManager *HookManager
	logger      *zap.Logger
}

// NewFiatIntegration creates a new fiat integration
func NewFiatIntegration(hookManager *HookManager, logger *zap.Logger) *FiatIntegration {
	return &FiatIntegration{
		hookManager: hookManager,
		logger:      logger,
	}
}

// OnDeposit handles fiat deposit events
func (f *FiatIntegration) OnDeposit(ctx context.Context, userID string, amount float64, currency string, bankAccount string, reference string) error {
	// Convert amount to decimal
	amountDecimal := decimal.NewFromFloat(amount)

	// Create BankAccountInfo (simplified - in real implementation, you'd parse this properly)
	bankAccountInfo := BankAccountInfo{
		BankName:      bankAccount, // Simplified
		AccountNumber: bankAccount,
		Currency:      currency,
	}

	event := FiatDepositEvent{
		BaseEvent: BaseEvent{
			Type:      EventTypeFiatDeposit,
			UserID:    userID,
			Timestamp: getCurrentTimestamp(),
			Module:    ModuleFiat,
			ID:        uuid.New(),
		},
		Amount:      amountDecimal,
		Currency:    currency,
		BankAccount: bankAccountInfo,
		Reference:   reference,
		Status:      "pending",
	}

	f.logger.Info("Processing fiat deposit",
		zap.String("user_id", userID),
		zap.Float64("amount", amount),
		zap.String("currency", currency),
		zap.String("bank_account", bankAccount),
		zap.String("reference", reference),
	)

	if err := f.hookManager.TriggerHooks(ctx, event); err != nil {
		f.logger.Error("Failed to trigger fiat deposit hooks",
			zap.Error(err),
			zap.String("user_id", userID),
			zap.String("reference", reference),
		)
		return fmt.Errorf("failed to process fiat deposit compliance: %w", err)
	}

	return nil
}

// OnWithdrawal handles fiat withdrawal events
func (f *FiatIntegration) OnWithdrawal(ctx context.Context, userID string, amount float64, currency string, bankAccount string, reference string) error {
	// Convert amount to decimal
	amountDecimal := decimal.NewFromFloat(amount)

	// Create BankAccountInfo (simplified - in real implementation, you'd parse this properly)
	bankAccountInfo := BankAccountInfo{
		BankName:      bankAccount, // Simplified
		AccountNumber: bankAccount,
		Currency:      currency,
	}

	event := FiatWithdrawalEvent{
		BaseEvent: BaseEvent{
			Type:      EventTypeFiatWithdrawal,
			UserID:    userID,
			Timestamp: getCurrentTimestamp(),
			Module:    ModuleFiat,
			ID:        uuid.New(),
		},
		Amount:      amountDecimal,
		Currency:    currency,
		BankAccount: bankAccountInfo,
		Reference:   reference,
		Status:      "pending",
	}

	f.logger.Info("Processing fiat withdrawal",
		zap.String("user_id", userID),
		zap.Float64("amount", amount),
		zap.String("currency", currency),
		zap.String("bank_account", bankAccount),
		zap.String("reference", reference),
	)

	if err := f.hookManager.TriggerHooks(ctx, event); err != nil {
		f.logger.Error("Failed to trigger fiat withdrawal hooks",
			zap.Error(err),
			zap.String("user_id", userID),
			zap.String("reference", reference),
		)
		return fmt.Errorf("failed to process fiat withdrawal compliance: %w", err)
	}

	return nil
}

// OnTransfer handles fiat transfer events
func (f *FiatIntegration) OnTransfer(ctx context.Context, fromUserID, toUserID string, amount float64, currency string, reference string) error {
	// Parse UUIDs
	fromUserUUID, err := uuid.Parse(fromUserID)
	if err != nil {
		return fmt.Errorf("invalid from_user_id UUID: %w", err)
	}

	toUserUUID, err := uuid.Parse(toUserID)
	if err != nil {
		return fmt.Errorf("invalid to_user_id UUID: %w", err)
	}

	// Convert amount to decimal
	amountDecimal := decimal.NewFromFloat(amount)

	event := FiatTransferEvent{
		BaseEvent: BaseEvent{
			Type:      EventTypeFiatTransfer,
			UserID:    fromUserID,
			Timestamp: getCurrentTimestamp(),
			Module:    ModuleFiat,
			ID:        uuid.New(),
		},
		FromUserID: fromUserUUID,
		ToUserID:   toUserUUID,
		Amount:     amountDecimal,
		Currency:   currency,
		Reference:  reference,
	}

	f.logger.Info("Processing fiat transfer",
		zap.String("from_user_id", fromUserID),
		zap.String("to_user_id", toUserID),
		zap.Float64("amount", amount),
		zap.String("currency", currency),
		zap.String("reference", reference),
	)

	if err := f.hookManager.TriggerHooks(ctx, event); err != nil {
		f.logger.Error("Failed to trigger fiat transfer hooks",
			zap.Error(err),
			zap.String("from_user_id", fromUserID),
			zap.String("to_user_id", toUserID),
			zap.String("reference", reference),
		)
		return fmt.Errorf("failed to process fiat transfer compliance: %w", err)
	}

	return nil
}

// OnBankAccountAdd handles bank account addition events
func (f *FiatIntegration) OnBankAccountAdd(ctx context.Context, userID string, bankAccount BankAccountInfo) error {
	event := FiatBankAccountEvent{
		BaseEvent: BaseEvent{
			Type:      EventTypeFiatBankAccount,
			UserID:    userID,
			Timestamp: getCurrentTimestamp(),
			Module:    ModuleFiat,
		},
		Action:      "add",
		BankAccount: bankAccount,
	}

	f.logger.Info("Processing bank account addition",
		zap.String("user_id", userID),
		zap.String("bank_name", bankAccount.BankName),
		zap.String("account_number", bankAccount.AccountNumber),
	)

	if err := f.hookManager.TriggerHooks(ctx, event); err != nil {
		f.logger.Error("Failed to trigger bank account addition hooks",
			zap.Error(err),
			zap.String("user_id", userID),
			zap.String("bank_name", bankAccount.BankName),
		)
		return fmt.Errorf("failed to process bank account addition compliance: %w", err)
	}

	return nil
}

// OnBankAccountRemove handles bank account removal events
func (f *FiatIntegration) OnBankAccountRemove(ctx context.Context, userID string, bankAccount BankAccountInfo) error {
	event := FiatBankAccountEvent{
		BaseEvent: BaseEvent{
			Type:      EventTypeFiatBankAccount,
			UserID:    userID,
			Timestamp: getCurrentTimestamp(),
			Module:    ModuleFiat,
		},
		Action:      "remove",
		BankAccount: bankAccount,
	}

	f.logger.Info("Processing bank account removal",
		zap.String("user_id", userID),
		zap.String("bank_name", bankAccount.BankName),
		zap.String("account_number", bankAccount.AccountNumber),
	)

	if err := f.hookManager.TriggerHooks(ctx, event); err != nil {
		f.logger.Error("Failed to trigger bank account removal hooks",
			zap.Error(err),
			zap.String("user_id", userID),
			zap.String("bank_name", bankAccount.BankName),
		)
		return fmt.Errorf("failed to process bank account removal compliance: %w", err)
	}

	return nil
}

// OnCurrencyConversion handles currency conversion events
func (f *FiatIntegration) OnCurrencyConversion(ctx context.Context, userID string, fromCurrency, toCurrency string, fromAmount, toAmount float64, rate float64, reference string) error {
	// Convert amounts to decimal
	fromAmountDecimal := decimal.NewFromFloat(fromAmount)
	toAmountDecimal := decimal.NewFromFloat(toAmount)
	rateDecimal := decimal.NewFromFloat(rate)

	event := FiatConversionEvent{
		BaseEvent: BaseEvent{
			Type:      EventTypeFiatConversion,
			UserID:    userID,
			Timestamp: getCurrentTimestamp(),
			Module:    ModuleFiat,
			ID:        uuid.New(),
		},
		FromCurrency: fromCurrency,
		ToCurrency:   toCurrency,
		FromAmount:   fromAmountDecimal,
		ToAmount:     toAmountDecimal,
		Rate:         rateDecimal,
		Reference:    reference,
	}

	f.logger.Info("Processing currency conversion",
		zap.String("user_id", userID),
		zap.String("from_currency", fromCurrency),
		zap.String("to_currency", toCurrency),
		zap.Float64("from_amount", fromAmount),
		zap.Float64("to_amount", toAmount),
		zap.Float64("rate", rate),
		zap.String("reference", reference),
	)

	if err := f.hookManager.TriggerHooks(ctx, event); err != nil {
		f.logger.Error("Failed to trigger currency conversion hooks",
			zap.Error(err),
			zap.String("user_id", userID),
			zap.String("reference", reference),
		)
		return fmt.Errorf("failed to process currency conversion compliance: %w", err)
	}

	return nil
}
