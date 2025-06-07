package services

import (
	"context"
	"fmt"
	"time"

	"github.com/Aidin1998/finalex/internal/wallet/interfaces"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// GetBalance retrieves the balance for a specific user and asset
func (bm *BalanceManager) GetBalance(ctx context.Context, userID uuid.UUID, asset string) (*interfaces.AssetBalance, error) {
	bm.logger.Debug("Getting balance",
		zap.String("user_id", userID.String()),
		zap.String("asset", asset))

	balance, err := bm.repository.GetBalance(ctx, userID, asset)
	if err != nil {
		bm.logger.Error("Failed to get balance",
			zap.String("user_id", userID.String()),
			zap.String("asset", asset),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get balance: %w", err)
	}

	// Convert WalletBalance to AssetBalance
	return &interfaces.AssetBalance{
		Asset:     balance.Asset,
		Available: balance.Available,
		Locked:    balance.Locked,
		Total:     balance.Available.Add(balance.Locked),
	}, nil
}

// GetBalances retrieves all balances for a specific user
func (bm *BalanceManager) GetBalances(ctx context.Context, userID uuid.UUID) (*interfaces.BalanceResponse, error) {
	bm.logger.Debug("Getting all balances",
		zap.String("user_id", userID.String()))

	balances, err := bm.repository.GetUserBalances(ctx, userID)
	if err != nil {
		bm.logger.Error("Failed to get balances",
			zap.String("user_id", userID.String()),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get balances: %w", err)
	}
	// Convert to AssetBalances map
	assetBalances := make(map[string]interfaces.AssetBalance)
	for _, balance := range balances {
		assetBalances[balance.Asset] = interfaces.AssetBalance{
			Asset:     balance.Asset,
			Available: balance.Available,
			Locked:    balance.Locked,
			Total:     balance.Available.Add(balance.Locked),
		}
	}

	return &interfaces.BalanceResponse{
		UserID:    userID,
		Balances:  assetBalances,
		Timestamp: time.Now(),
	}, nil
}

// UpdateBalance updates the balance for a specific user and asset
func (bm *BalanceManager) UpdateBalance(ctx context.Context, userID uuid.UUID, asset string, available, locked decimal.Decimal) error {
	bm.logger.Debug("Updating balance",
		zap.String("user_id", userID.String()),
		zap.String("asset", asset),
		zap.String("available", available.String()),
		zap.String("locked", locked.String()))

	// Validate amounts
	if available.IsNegative() || locked.IsNegative() {
		return fmt.Errorf("balance amounts cannot be negative")
	}

	// Get current balance to update the ID and other fields
	existingBalance, err := bm.repository.GetBalance(ctx, userID, asset)
	if err != nil && err != gorm.ErrRecordNotFound {
		bm.logger.Error("Failed to get existing balance",
			zap.String("user_id", userID.String()),
			zap.String("asset", asset),
			zap.Error(err))
		return fmt.Errorf("failed to get existing balance: %w", err)
	}

	// Create or update balance object
	var balance *interfaces.WalletBalance
	if existingBalance != nil {
		balance = existingBalance
		balance.Available = available
		balance.Locked = locked
	} else {
		balance = &interfaces.WalletBalance{
			ID:        uuid.New(),
			UserID:    userID,
			Asset:     asset,
			Available: available,
			Locked:    locked,
			UpdatedAt: time.Now(),
		}
	}

	err = bm.repository.UpdateBalance(ctx, balance)
	if err != nil {
		bm.logger.Error("Failed to update balance",
			zap.String("user_id", userID.String()),
			zap.String("asset", asset),
			zap.Error(err))
		return fmt.Errorf("failed to update balance: %w", err)
	}

	return nil
}

// Transfer method removed due to duplicate declaration in balance_manager_clean.go

// CalculateBalance calculates the total balance (available + locked) for a user and asset
func (bm *BalanceManager) CalculateBalance(ctx context.Context, userID uuid.UUID, asset string) (*interfaces.AssetBalance, error) {
	balance, err := bm.GetBalance(ctx, userID, asset)
	if err != nil {
		return nil, err
	}

	if balance == nil {
		return &interfaces.AssetBalance{
			Asset:     asset,
			Available: decimal.Zero,
			Locked:    decimal.Zero,
			Total:     decimal.Zero,
		}, nil
	}

	return balance, nil
}

// CreditBalance credits (adds) balance to a user's account - used by deposit manager
func (bm *BalanceManager) CreditBalance(ctx context.Context, tx *gorm.DB, userID uuid.UUID, asset string, amount decimal.Decimal, reference string) error {
	bm.logger.Debug("Crediting balance",
		zap.String("user_id", userID.String()),
		zap.String("asset", asset),
		zap.String("amount", amount.String()),
		zap.String("reference", reference))

	if amount.IsNegative() || amount.IsZero() {
		return fmt.Errorf("credit amount must be positive")
	}

	// Get current balance
	balance, err := bm.repository.GetBalance(ctx, userID, asset)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			// Create new balance if it doesn't exist
			balance = &interfaces.WalletBalance{
				UserID:    userID,
				Asset:     asset,
				Available: decimal.Zero,
				Locked:    decimal.Zero,
			}
		} else {
			return fmt.Errorf("failed to get balance for credit: %w", err)
		}
	}

	// Add amount to available balance
	newAvailable := balance.Available.Add(amount)

	// Update balance object
	if balance.ID == uuid.Nil {
		// Create new balance
		balance.ID = uuid.New()
		balance.Available = newAvailable
		balance.UpdatedAt = time.Now()
	} else {
		balance.Available = newAvailable
		balance.UpdatedAt = time.Now()
	}

	err = bm.repository.UpdateBalance(ctx, balance)
	if err != nil {
		return fmt.Errorf("failed to credit balance: %w", err)
	}

	bm.logger.Info("Balance credited successfully",
		zap.String("user_id", userID.String()),
		zap.String("asset", asset),
		zap.String("amount", amount.String()),
		zap.String("reference", reference))

	return nil
}

// DebitBalance debits (subtracts) balance from a user's account - used by withdrawal manager
func (bm *BalanceManager) DebitBalance(ctx context.Context, tx *gorm.DB, userID uuid.UUID, asset string, amount decimal.Decimal, reference string) error {
	bm.logger.Debug("Debiting balance",
		zap.String("user_id", userID.String()),
		zap.String("asset", asset),
		zap.String("amount", amount.String()),
		zap.String("reference", reference))

	if amount.IsNegative() || amount.IsZero() {
		return fmt.Errorf("debit amount must be positive")
	}

	// Get current balance
	balance, err := bm.repository.GetBalance(ctx, userID, asset)
	if err != nil {
		return fmt.Errorf("failed to get balance for debit: %w", err)
	}

	if balance.Available.LessThan(amount) {
		return fmt.Errorf("insufficient balance for debit operation")
	}

	// Subtract amount from available balance
	newAvailable := balance.Available.Sub(amount)

	// Update balance object
	balance.Available = newAvailable
	balance.UpdatedAt = time.Now()

	err = bm.repository.UpdateBalance(ctx, balance)
	if err != nil {
		return fmt.Errorf("failed to debit balance: %w", err)
	}

	bm.logger.Info("Balance debited successfully",
		zap.String("user_id", userID.String()),
		zap.String("asset", asset),
		zap.String("amount", amount.String()),
		zap.String("reference", reference))

	return nil
}
