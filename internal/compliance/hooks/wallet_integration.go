package hooks

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// WalletIntegration handles compliance integration for wallet operations
type WalletIntegration struct {
	hookManager *HookManager
	logger      *zap.Logger
}

// NewWalletIntegration creates a new wallet integration
func NewWalletIntegration(hookManager *HookManager, logger *zap.Logger) *WalletIntegration {
	return &WalletIntegration{
		hookManager: hookManager,
		logger:      logger,
	}
}

// OnCryptoDeposit handles cryptocurrency deposit events
func (w *WalletIntegration) OnCryptoDeposit(ctx context.Context, userID string, amount float64, currency string, txHash string, fromAddress string, toAddress string, confirmations int) error {
	// Convert amount to decimal
	amountDecimal := decimal.NewFromFloat(amount)

	event := WalletCryptoDepositEvent{
		BaseEvent: BaseEvent{
			Type:      EventTypeWalletCryptoDeposit,
			UserID:    userID,
			Timestamp: getCurrentTimestamp(),
			Module:    ModuleWallet,
			ID:        uuid.New(),
		},
		Amount:        amountDecimal,
		Currency:      currency,
		TxHash:        txHash,
		FromAddress:   fromAddress,
		ToAddress:     toAddress,
		Confirmations: confirmations,
		Network:       "mainnet", // Default value
	}

	w.logger.Info("Processing crypto deposit",
		zap.String("user_id", userID),
		zap.Float64("amount", amount),
		zap.String("currency", currency),
		zap.String("tx_hash", txHash),
		zap.String("from_address", fromAddress),
		zap.String("to_address", toAddress),
		zap.Int("confirmations", confirmations),
	)

	if err := w.hookManager.TriggerHooks(ctx, event); err != nil {
		w.logger.Error("Failed to trigger crypto deposit hooks",
			zap.Error(err),
			zap.String("user_id", userID),
			zap.String("tx_hash", txHash),
		)
		return fmt.Errorf("failed to process crypto deposit compliance: %w", err)
	}

	return nil
}

// OnCryptoWithdrawal handles cryptocurrency withdrawal events
func (w *WalletIntegration) OnCryptoWithdrawal(ctx context.Context, userID string, amount float64, currency string, txHash string, fromAddress string, toAddress string, fee float64) error {
	// Convert amounts to decimal
	amountDecimal := decimal.NewFromFloat(amount)
	feeDecimal := decimal.NewFromFloat(fee)

	event := WalletCryptoWithdrawalEvent{
		BaseEvent: BaseEvent{
			Type:      EventTypeWalletCryptoWithdrawal,
			UserID:    userID,
			Timestamp: getCurrentTimestamp(),
			Module:    ModuleWallet,
			ID:        uuid.New(),
		},
		Amount:      amountDecimal,
		Currency:    currency,
		TxHash:      txHash,
		FromAddress: fromAddress,
		ToAddress:   toAddress,
		Fee:         feeDecimal,
		Network:     "mainnet", // Default value
	}

	w.logger.Info("Processing crypto withdrawal",
		zap.String("user_id", userID),
		zap.Float64("amount", amount),
		zap.String("currency", currency),
		zap.String("tx_hash", txHash),
		zap.String("from_address", fromAddress),
		zap.String("to_address", toAddress),
		zap.Float64("fee", fee),
	)

	if err := w.hookManager.TriggerHooks(ctx, event); err != nil {
		w.logger.Error("Failed to trigger crypto withdrawal hooks",
			zap.Error(err),
			zap.String("user_id", userID),
			zap.String("tx_hash", txHash),
		)
		return fmt.Errorf("failed to process crypto withdrawal compliance: %w", err)
	}

	return nil
}

// OnInternalTransfer handles internal wallet transfer events
func (w *WalletIntegration) OnInternalTransfer(ctx context.Context, fromUserID, toUserID string, amount float64, currency string, reference string) error {
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

	event := WalletInternalTransferEvent{
		BaseEvent: BaseEvent{
			Type:      EventTypeWalletInternalTransfer,
			UserID:    fromUserID,
			Timestamp: getCurrentTimestamp(),
			Module:    ModuleWallet,
			ID:        uuid.New(),
		},
		FromUserID: fromUserUUID,
		ToUserID:   toUserUUID,
		Amount:     amountDecimal,
		Currency:   currency,
		Reference:  reference,
	}

	w.logger.Info("Processing internal transfer",
		zap.String("from_user_id", fromUserID),
		zap.String("to_user_id", toUserID),
		zap.Float64("amount", amount),
		zap.String("currency", currency),
		zap.String("reference", reference),
	)

	if err := w.hookManager.TriggerHooks(ctx, event); err != nil {
		w.logger.Error("Failed to trigger internal transfer hooks",
			zap.Error(err),
			zap.String("from_user_id", fromUserID),
			zap.String("to_user_id", toUserID),
			zap.String("reference", reference),
		)
		return fmt.Errorf("failed to process internal transfer compliance: %w", err)
	}

	return nil
}

// OnBalanceUpdate handles balance update events
func (w *WalletIntegration) OnBalanceUpdate(ctx context.Context, userID string, currency string, oldBalance, newBalance float64, reason string) error {
	// Convert balances to decimal
	oldBalanceDecimal := decimal.NewFromFloat(oldBalance)
	newBalanceDecimal := decimal.NewFromFloat(newBalance)

	event := WalletBalanceUpdateEvent{
		BaseEvent: BaseEvent{
			Type:      EventTypeWalletBalanceUpdate,
			UserID:    userID,
			Timestamp: getCurrentTimestamp(),
			Module:    ModuleWallet,
			ID:        uuid.New(),
		},
		Currency:   currency,
		OldBalance: oldBalanceDecimal,
		NewBalance: newBalanceDecimal,
		Reason:     reason,
	}

	w.logger.Info("Processing balance update",
		zap.String("user_id", userID),
		zap.String("currency", currency),
		zap.Float64("old_balance", oldBalance),
		zap.Float64("new_balance", newBalance),
		zap.String("reason", reason),
	)

	if err := w.hookManager.TriggerHooks(ctx, event); err != nil {
		w.logger.Error("Failed to trigger balance update hooks",
			zap.Error(err),
			zap.String("user_id", userID),
			zap.String("currency", currency),
		)
		return fmt.Errorf("failed to process balance update compliance: %w", err)
	}

	return nil
}

// OnAddressGeneration handles address generation events
func (w *WalletIntegration) OnAddressGeneration(ctx context.Context, userID string, currency string, address string, addressType string) error {
	event := WalletAddressEvent{
		BaseEvent: BaseEvent{
			Type:      EventTypeWalletAddress,
			UserID:    userID,
			Timestamp: getCurrentTimestamp(),
			Module:    ModuleWallet,
		},
		Currency:    currency,
		Address:     address,
		AddressType: addressType,
		Action:      "generate",
	}

	w.logger.Info("Processing address generation",
		zap.String("user_id", userID),
		zap.String("currency", currency),
		zap.String("address", address),
		zap.String("address_type", addressType),
	)

	if err := w.hookManager.TriggerHooks(ctx, event); err != nil {
		w.logger.Error("Failed to trigger address generation hooks",
			zap.Error(err),
			zap.String("user_id", userID),
			zap.String("address", address),
		)
		return fmt.Errorf("failed to process address generation compliance: %w", err)
	}

	return nil
}

// OnStakingDeposit handles staking deposit events
func (w *WalletIntegration) OnStakingDeposit(ctx context.Context, userID string, amount float64, currency string, stakingPeriod int, expectedReward float64) error {
	event := WalletStakingEvent{
		BaseEvent: BaseEvent{
			Type:      EventTypeWalletStaking,
			UserID:    userID,
			Timestamp: getCurrentTimestamp(),
			Module:    ModuleWallet,
		},
		Action:         "deposit",
		Amount:         amount,
		Currency:       currency,
		StakingPeriod:  stakingPeriod,
		ExpectedReward: expectedReward,
	}

	w.logger.Info("Processing staking deposit",
		zap.String("user_id", userID),
		zap.Float64("amount", amount),
		zap.String("currency", currency),
		zap.Int("staking_period", stakingPeriod),
		zap.Float64("expected_reward", expectedReward),
	)

	if err := w.hookManager.TriggerHooks(ctx, event); err != nil {
		w.logger.Error("Failed to trigger staking deposit hooks",
			zap.Error(err),
			zap.String("user_id", userID),
		)
		return fmt.Errorf("failed to process staking deposit compliance: %w", err)
	}

	return nil
}

// OnStakingWithdrawal handles staking withdrawal events
func (w *WalletIntegration) OnStakingWithdrawal(ctx context.Context, userID string, amount float64, currency string, reward float64, penalty float64) error {
	event := WalletStakingEvent{
		BaseEvent: BaseEvent{
			Type:      EventTypeWalletStaking,
			UserID:    userID,
			Timestamp: getCurrentTimestamp(),
			Module:    ModuleWallet,
		},
		Action:   "withdrawal",
		Amount:   amount,
		Currency: currency,
		Reward:   reward,
		Penalty:  penalty,
	}

	w.logger.Info("Processing staking withdrawal",
		zap.String("user_id", userID),
		zap.Float64("amount", amount),
		zap.String("currency", currency),
		zap.Float64("reward", reward),
		zap.Float64("penalty", penalty),
	)

	if err := w.hookManager.TriggerHooks(ctx, event); err != nil {
		w.logger.Error("Failed to trigger staking withdrawal hooks",
			zap.Error(err),
			zap.String("user_id", userID),
		)
		return fmt.Errorf("failed to process staking withdrawal compliance: %w", err)
	}

	return nil
}
