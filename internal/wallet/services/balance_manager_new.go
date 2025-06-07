// Package services provides balance management functionality
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

// BalanceManager handles user balance operations
type BalanceManager struct {
	db         *gorm.DB
	logger     *zap.Logger
	repository interfaces.WalletRepository
	cache      interfaces.WalletCache
	config     *BalanceConfig
}

// BalanceConfig holds balance management configuration
type BalanceConfig struct {
	CacheTimeout         time.Duration
	RefreshThreshold     time.Duration
	MinBalanceAlert      decimal.Decimal
	NegativeBalanceAlert bool
	ConcurrencyLimit     int
}

// NewBalanceManager creates a new balance manager
func NewBalanceManager(
	db *gorm.DB,
	logger *zap.Logger,
	repository interfaces.WalletRepository,
	cache interfaces.WalletCache,
	config *BalanceConfig,
) *BalanceManager {
	return &BalanceManager{
		db:         db,
		logger:     logger,
		repository: repository,
		cache:      cache,
		config:     config,
	}
}

// GetBalance retrieves balance for a user and asset
func (bm *BalanceManager) GetBalance(ctx context.Context, userID uuid.UUID, asset string) (*interfaces.AssetBalance, error) {
	bm.logger.Debug("Getting balance",
		zap.String("user_id", userID.String()),
		zap.String("asset", asset),
	)

	// Try cache first
	if bm.cache != nil {
		if cached, err := bm.cache.GetBalance(ctx, userID, asset); err == nil && cached != nil {
			return cached, nil
		}
	}

	// Get from database
	balance, err := bm.repository.GetBalance(ctx, userID, asset)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			// Create zero balance if it doesn't exist
			return bm.createZeroBalance(ctx, userID, asset)
		}
		return nil, fmt.Errorf("failed to get balance: %w", err)
	}

	// Convert to AssetBalance
	assetBalance := &interfaces.AssetBalance{
		Asset:     balance.Asset,
		Available: balance.Available,
		Locked:    balance.Locked,
		Total:     balance.Total,
	}

	// Update cache
	if bm.cache != nil {
		if err := bm.cache.SetBalance(ctx, userID, asset, assetBalance, bm.config.CacheTimeout); err != nil {
			bm.logger.Warn("Failed to cache balance", zap.Error(err))
		}
	}

	return assetBalance, nil
}

// GetBalances retrieves all balances for a user
func (bm *BalanceManager) GetBalances(ctx context.Context, userID uuid.UUID) (*interfaces.BalanceResponse, error) {
	bm.logger.Debug("Getting all balances", zap.String("user_id", userID.String()))

	balances, err := bm.repository.GetAllBalances(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user balances: %w", err)
	}

	// Convert to response format
	assetBalances := make(map[string]interfaces.AssetBalance)

	for _, balance := range balances {
		assetBalances[balance.Asset] = interfaces.AssetBalance{
			Asset:     balance.Asset,
			Available: balance.Available,
			Locked:    balance.Locked,
			Total:     balance.Total,
		}
	}

	return &interfaces.BalanceResponse{
		UserID:    userID,
		Balances:  assetBalances,
		Timestamp: time.Now(),
	}, nil
}

// UpdateBalance updates balance using the repository's UpdateBalance method
func (bm *BalanceManager) UpdateBalance(ctx context.Context, userID uuid.UUID, asset string, amount decimal.Decimal, txType, txRef string) error {
	bm.logger.Info("Updating balance",
		zap.String("user_id", userID.String()),
		zap.String("asset", asset),
		zap.String("amount", amount.String()),
		zap.String("tx_type", txType),
	)

	// Get current balance
	balance, err := bm.repository.GetBalance(ctx, userID, asset)
	if err != nil && err != gorm.ErrRecordNotFound {
		return fmt.Errorf("failed to get current balance: %w", err)
	}

	var newAvailable, newLocked decimal.Decimal

	if err == gorm.ErrRecordNotFound {
		// Create new balance
		if amount.IsPositive() {
			newAvailable = amount
			newLocked = decimal.Zero
		} else {
			return fmt.Errorf("cannot create negative balance")
		}
	} else {
		// Update existing balance
		newAvailable = balance.Available.Add(amount)
		newLocked = balance.Locked

		// Ensure non-negative balance
		if newAvailable.IsNegative() {
			return fmt.Errorf("insufficient balance: required %s, available %s",
				amount.Abs().String(), balance.Available.String())
		}
	}

	// Update balance
	if err := bm.repository.UpdateBalance(ctx, userID, asset, newAvailable, newLocked); err != nil {
		return fmt.Errorf("failed to update balance: %w", err)
	}

	// Invalidate cache
	bm.invalidateBalanceCache(userID, asset)

	return nil
}

// Transfer transfers balance between users
func (bm *BalanceManager) Transfer(ctx context.Context, fromUserID, toUserID uuid.UUID, asset string, amount decimal.Decimal, reference string) error {
	bm.logger.Info("Transferring balance",
		zap.String("from_user", fromUserID.String()),
		zap.String("to_user", toUserID.String()),
		zap.String("asset", asset),
		zap.String("amount", amount.String()),
	)

	if amount.LessThanOrEqual(decimal.Zero) {
		return fmt.Errorf("transfer amount must be positive")
	}

	if fromUserID == toUserID {
		return fmt.Errorf("cannot transfer to same user")
	}

	// Start transaction
	return bm.db.Transaction(func(tx *gorm.DB) error {
		// Get sender balance
		fromBalance, err := bm.repository.GetBalance(ctx, fromUserID, asset)
		if err != nil {
			return fmt.Errorf("failed to get sender balance: %w", err)
		}

		// Check sufficient funds
		if fromBalance.Available.LessThan(amount) {
			return fmt.Errorf("insufficient funds: required %s, available %s",
				amount.String(), fromBalance.Available.String())
		}

		// Debit from sender
		newFromAvailable := fromBalance.Available.Sub(amount)
		if err := bm.repository.UpdateBalance(ctx, fromUserID, asset, newFromAvailable, fromBalance.Locked); err != nil {
			return fmt.Errorf("failed to debit sender: %w", err)
		}

		// Get or create receiver balance
		toBalance, err := bm.repository.GetBalance(ctx, toUserID, asset)
		if err != nil && err != gorm.ErrRecordNotFound {
			return fmt.Errorf("failed to get receiver balance: %w", err)
		}

		var newToAvailable, newToLocked decimal.Decimal
		if err == gorm.ErrRecordNotFound {
			// Create new balance for receiver
			newToAvailable = amount
			newToLocked = decimal.Zero
		} else {
			// Update existing balance
			newToAvailable = toBalance.Available.Add(amount)
			newToLocked = toBalance.Locked
		}

		// Credit to receiver
		if err := bm.repository.UpdateBalance(ctx, toUserID, asset, newToAvailable, newToLocked); err != nil {
			return fmt.Errorf("failed to credit receiver: %w", err)
		}

		// Invalidate caches
		bm.invalidateBalanceCache(fromUserID, asset)
		bm.invalidateBalanceCache(toUserID, asset)

		return nil
	})
}

// CalculateBalance calculates total balance including locks
func (bm *BalanceManager) CalculateBalance(ctx context.Context, userID uuid.UUID, asset string) (*interfaces.AssetBalance, error) {
	// For now, this is the same as GetBalance since we don't have separate lock calculations
	return bm.GetBalance(ctx, userID, asset)
}

// GetBalanceHistory retrieves balance change history
func (bm *BalanceManager) GetBalanceHistory(ctx context.Context, userID uuid.UUID, asset string, limit, offset int) ([]*interfaces.BalanceHistoryEntry, error) {
	// Get transactions as a proxy for balance history
	transactions, err := bm.repository.GetUserTransactions(ctx, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction history: %w", err)
	}

	// Filter by asset and convert to balance history
	history := make([]*interfaces.BalanceHistoryEntry, 0)
	for _, tx := range transactions {
		if tx.Asset == asset {
			entry := &interfaces.BalanceHistoryEntry{
				ID:            tx.ID,
				UserID:        tx.UserID,
				Asset:         tx.Asset,
				Amount:        tx.Amount,
				BalanceBefore: decimal.Zero, // Would need separate tracking
				BalanceAfter:  decimal.Zero, // Would need separate tracking
				TxType:        string(tx.Direction),
				Timestamp:     tx.CreatedAt,
			}
			history = append(history, entry)
		}
	}

	return history, nil
}

// Private helper methods

func (bm *BalanceManager) createZeroBalance(ctx context.Context, userID uuid.UUID, asset string) (*interfaces.AssetBalance, error) {
	// Create zero balance using UpdateBalance
	if err := bm.repository.UpdateBalance(ctx, userID, asset, decimal.Zero, decimal.Zero); err != nil {
		return nil, fmt.Errorf("failed to create zero balance: %w", err)
	}

	return &interfaces.AssetBalance{
		Asset:     asset,
		Available: decimal.Zero,
		Locked:    decimal.Zero,
		Total:     decimal.Zero,
	}, nil
}

func (bm *BalanceManager) invalidateBalanceCache(userID uuid.UUID, asset string) {
	if bm.cache != nil {
		if err := bm.cache.InvalidateBalance(context.Background(), userID, asset); err != nil {
			bm.logger.Warn("Failed to invalidate balance cache", zap.Error(err))
		}
	}
}
