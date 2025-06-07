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
		cacheKey := fmt.Sprintf("balance:%s:%s", userID.String(), asset)
		if cached, err := bm.cache.GetBalance(ctx, cacheKey); err == nil && cached != nil {
			// Check if cache is still fresh
			if time.Since(cached.UpdatedAt) < bm.config.RefreshThreshold {
				return &interfaces.AssetBalance{
					Asset:     cached.Asset,
					Available: cached.Available,
					Locked:    cached.Locked,
					Total:     cached.Total,
					UpdatedAt: cached.UpdatedAt,
				}, nil
			}
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

	// Update cache
	if bm.cache != nil {
		cacheKey := fmt.Sprintf("balance:%s:%s", userID.String(), asset)
		if err := bm.cache.SetBalance(ctx, cacheKey, balance, bm.config.CacheTimeout); err != nil {
			bm.logger.Warn("Failed to cache balance", zap.Error(err))
		}
	}

	return &interfaces.AssetBalance{
		Asset:     balance.Asset,
		Available: balance.Available,
		Locked:    balance.Locked,
		Total:     balance.Total,
		UpdatedAt: balance.UpdatedAt,
	}, nil
}

// GetBalances retrieves all balances for a user
func (bm *BalanceManager) GetBalances(ctx context.Context, userID uuid.UUID) (*interfaces.BalanceResponse, error) {
	bm.logger.Debug("Getting all balances", zap.String("user_id", userID.String()))

	balances, err := bm.repository.GetUserBalances(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user balances: %w", err)
	}

	// Convert to response format
	assetBalances := make(map[string]*interfaces.AssetBalance)
	totalValue := decimal.Zero

	for _, balance := range balances {
		assetBalances[balance.Asset] = &interfaces.AssetBalance{
			Asset:     balance.Asset,
			Available: balance.Available,
			Locked:    balance.Locked,
			Total:     balance.Total,
			UpdatedAt: balance.UpdatedAt,
		}
		// Note: In a real implementation, you'd convert to USD or base currency
		totalValue = totalValue.Add(balance.Total)
	}

	return &interfaces.BalanceResponse{
		UserID:      userID,
		Balances:    assetBalances,
		TotalValue:  totalValue,
		LastUpdated: time.Now(),
	}, nil
}

// CreditBalance adds funds to a user's balance
func (bm *BalanceManager) CreditBalance(ctx context.Context, userID uuid.UUID, asset string, amount decimal.Decimal, txRef string) error {
	return bm.CreditBalanceInTx(ctx, bm.db, userID, asset, amount, txRef)
}

// CreditBalanceInTx adds funds to a user's balance within a transaction
func (bm *BalanceManager) CreditBalanceInTx(ctx context.Context, tx *gorm.DB, userID uuid.UUID, asset string, amount decimal.Decimal, txRef string) error {
	bm.logger.Info("Crediting balance",
		zap.String("user_id", userID.String()),
		zap.String("asset", asset),
		zap.String("amount", amount.String()),
		zap.String("tx_ref", txRef),
	)

	if amount.LessThanOrEqual(decimal.Zero) {
		return fmt.Errorf("credit amount must be positive")
	}

	// Get or create balance with row lock
	balance, err := bm.getOrCreateBalanceForUpdate(ctx, tx, userID, asset)
	if err != nil {
		return fmt.Errorf("failed to get balance for update: %w", err)
	}

	// Update balance
	balance.Available = balance.Available.Add(amount)
	balance.Total = balance.Available.Add(balance.Locked)
	balance.UpdatedAt = time.Now()

	// Save updated balance
	if err := bm.repository.UpdateBalanceInTx(ctx, tx, balance); err != nil {
		return fmt.Errorf("failed to update balance: %w", err)
	}

	// Invalidate cache
	bm.invalidateBalanceCache(userID, asset)

	bm.logger.Info("Balance credited successfully",
		zap.String("user_id", userID.String()),
		zap.String("asset", asset),
		zap.String("new_available", balance.Available.String()),
		zap.String("new_total", balance.Total.String()),
	)

	return nil
}

// DebitBalance removes funds from a user's balance
func (bm *BalanceManager) DebitBalance(ctx context.Context, userID uuid.UUID, asset string, amount decimal.Decimal, txRef string) error {
	return bm.DebitBalanceInTx(ctx, bm.db, userID, asset, amount, txRef)
}

// DebitBalanceInTx removes funds from a user's balance within a transaction
func (bm *BalanceManager) DebitBalanceInTx(ctx context.Context, tx *gorm.DB, userID uuid.UUID, asset string, amount decimal.Decimal, txRef string) error {
	bm.logger.Info("Debiting balance",
		zap.String("user_id", userID.String()),
		zap.String("asset", asset),
		zap.String("amount", amount.String()),
		zap.String("tx_ref", txRef),
	)

	if amount.LessThanOrEqual(decimal.Zero) {
		return fmt.Errorf("debit amount must be positive")
	}

	// Get balance with row lock
	balance, err := bm.repository.GetBalanceForUpdateInTx(ctx, tx, userID, asset)
	if err != nil {
		return fmt.Errorf("failed to get balance for update: %w", err)
	}

	// Check sufficient funds (consider both available and locked for total debit)
	if balance.Total.LessThan(amount) {
		return fmt.Errorf("insufficient balance: required %s, available %s",
			amount.String(), balance.Total.String())
	}

	// Debit from available first, then from locked if necessary
	if balance.Available.GreaterThanOrEqual(amount) {
		balance.Available = balance.Available.Sub(amount)
	} else {
		// Debit remaining from locked
		remaining := amount.Sub(balance.Available)
		balance.Available = decimal.Zero
		balance.Locked = balance.Locked.Sub(remaining)
	}

	// Update total
	balance.Total = balance.Available.Add(balance.Locked)
	balance.UpdatedAt = time.Now()

	// Ensure balances don't go negative
	if balance.Available.LessThan(decimal.Zero) {
		balance.Available = decimal.Zero
	}
	if balance.Locked.LessThan(decimal.Zero) {
		balance.Locked = decimal.Zero
	}

	// Save updated balance
	if err := bm.repository.UpdateBalanceInTx(ctx, tx, balance); err != nil {
		return fmt.Errorf("failed to update balance: %w", err)
	}

	// Check for low balance alerts
	if bm.config.NegativeBalanceAlert && balance.Total.LessThan(decimal.Zero) {
		bm.logger.Warn("Negative balance detected",
			zap.String("user_id", userID.String()),
			zap.String("asset", asset),
			zap.String("balance", balance.Total.String()),
		)
	}

	// Invalidate cache
	bm.invalidateBalanceCache(userID, asset)

	bm.logger.Info("Balance debited successfully",
		zap.String("user_id", userID.String()),
		zap.String("asset", asset),
		zap.String("new_available", balance.Available.String()),
		zap.String("new_total", balance.Total.String()),
	)

	return nil
}

// TransferBalance transfers funds between users
func (bm *BalanceManager) TransferBalance(ctx context.Context, fromUserID, toUserID uuid.UUID, asset string, amount decimal.Decimal, txRef string) error {
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

	return bm.db.Transaction(func(tx *gorm.DB) error {
		// Debit from source user
		if err := bm.DebitBalanceInTx(ctx, tx, fromUserID, asset, amount, txRef); err != nil {
			return fmt.Errorf("failed to debit from source user: %w", err)
		}

		// Credit to destination user
		if err := bm.CreditBalanceInTx(ctx, tx, toUserID, asset, amount, txRef); err != nil {
			return fmt.Errorf("failed to credit to destination user: %w", err)
		}

		return nil
	})
}

// AdjustBalance adjusts a user's balance (admin function)
func (bm *BalanceManager) AdjustBalance(ctx context.Context, userID uuid.UUID, asset string, adjustment decimal.Decimal, reason, txRef string) error {
	bm.logger.Info("Adjusting balance",
		zap.String("user_id", userID.String()),
		zap.String("asset", asset),
		zap.String("adjustment", adjustment.String()),
		zap.String("reason", reason),
	)

	return bm.db.Transaction(func(tx *gorm.DB) error {
		// Get or create balance with row lock
		balance, err := bm.getOrCreateBalanceForUpdate(ctx, tx, userID, asset)
		if err != nil {
			return fmt.Errorf("failed to get balance for update: %w", err)
		}

		// Apply adjustment
		balance.Available = balance.Available.Add(adjustment)
		balance.Total = balance.Available.Add(balance.Locked)
		balance.UpdatedAt = time.Now()

		// Ensure balances don't go negative
		if balance.Available.LessThan(decimal.Zero) {
			balance.Available = decimal.Zero
		}
		balance.Total = balance.Available.Add(balance.Locked)

		// Save updated balance
		if err := bm.repository.UpdateBalanceInTx(ctx, tx, balance); err != nil {
			return fmt.Errorf("failed to update balance: %w", err)
		}

		// Log adjustment
		bm.logger.Info("Balance adjusted",
			zap.String("user_id", userID.String()),
			zap.String("asset", asset),
			zap.String("adjustment", adjustment.String()),
			zap.String("new_available", balance.Available.String()),
			zap.String("reason", reason),
		)

		return nil
	})
}

// LockBalance moves funds from available to locked
func (bm *BalanceManager) LockBalance(ctx context.Context, userID uuid.UUID, asset string, amount decimal.Decimal) error {
	return bm.LockBalanceInTx(ctx, bm.db, userID, asset, amount)
}

// LockBalanceInTx moves funds from available to locked within a transaction
func (bm *BalanceManager) LockBalanceInTx(ctx context.Context, tx *gorm.DB, userID uuid.UUID, asset string, amount decimal.Decimal) error {
	if amount.LessThanOrEqual(decimal.Zero) {
		return fmt.Errorf("lock amount must be positive")
	}

	// Get balance with row lock
	balance, err := bm.repository.GetBalanceForUpdateInTx(ctx, tx, userID, asset)
	if err != nil {
		return fmt.Errorf("failed to get balance for update: %w", err)
	}

	// Check sufficient available funds
	if balance.Available.LessThan(amount) {
		return fmt.Errorf("insufficient available balance: required %s, available %s",
			amount.String(), balance.Available.String())
	}

	// Move funds from available to locked
	balance.Available = balance.Available.Sub(amount)
	balance.Locked = balance.Locked.Add(amount)
	balance.UpdatedAt = time.Now()

	// Save updated balance
	if err := bm.repository.UpdateBalanceInTx(ctx, tx, balance); err != nil {
		return fmt.Errorf("failed to update balance: %w", err)
	}

	// Invalidate cache
	bm.invalidateBalanceCache(userID, asset)

	return nil
}

// UnlockBalance moves funds from locked to available
func (bm *BalanceManager) UnlockBalance(ctx context.Context, userID uuid.UUID, asset string, amount decimal.Decimal) error {
	return bm.UnlockBalanceInTx(ctx, bm.db, userID, asset, amount)
}

// UnlockBalanceInTx moves funds from locked to available within a transaction
func (bm *BalanceManager) UnlockBalanceInTx(ctx context.Context, tx *gorm.DB, userID uuid.UUID, asset string, amount decimal.Decimal) error {
	if amount.LessThanOrEqual(decimal.Zero) {
		return fmt.Errorf("unlock amount must be positive")
	}

	// Get balance with row lock
	balance, err := bm.repository.GetBalanceForUpdateInTx(ctx, tx, userID, asset)
	if err != nil {
		return fmt.Errorf("failed to get balance for update: %w", err)
	}

	// Check sufficient locked funds
	if balance.Locked.LessThan(amount) {
		return fmt.Errorf("insufficient locked balance: required %s, locked %s",
			amount.String(), balance.Locked.String())
	}

	// Move funds from locked to available
	balance.Locked = balance.Locked.Sub(amount)
	balance.Available = balance.Available.Add(amount)
	balance.UpdatedAt = time.Now()

	// Save updated balance
	if err := bm.repository.UpdateBalanceInTx(ctx, tx, balance); err != nil {
		return fmt.Errorf("failed to update balance: %w", err)
	}

	// Invalidate cache
	bm.invalidateBalanceCache(userID, asset)

	return nil
}

// GetBalanceHistory retrieves balance change history
func (bm *BalanceManager) GetBalanceHistory(ctx context.Context, userID uuid.UUID, asset string, limit, offset int) ([]*interfaces.BalanceHistoryEntry, error) {
	// This would typically query a separate balance_history table
	// For now, we'll return transactions as a proxy
	transactions, err := bm.repository.GetUserTransactionsByAsset(ctx, userID, asset, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction history: %w", err)
	}

	history := make([]*interfaces.BalanceHistoryEntry, 0, len(transactions))
	for _, tx := range transactions {
		entry := &interfaces.BalanceHistoryEntry{
			ID:          tx.ID,
			UserID:      tx.UserID,
			Asset:       tx.Asset,
			Amount:      tx.Amount,
			Type:        string(tx.Direction),
			Reference:   tx.ID.String(),
			Description: fmt.Sprintf("%s transaction", tx.Direction),
			Timestamp:   tx.CreatedAt,
		}
		history = append(history, entry)
	}

	return history, nil
}

// RefreshBalance forces a balance refresh from the database
func (bm *BalanceManager) RefreshBalance(ctx context.Context, userID uuid.UUID, asset string) (*interfaces.AssetBalance, error) {
	// Invalidate cache first
	bm.invalidateBalanceCache(userID, asset)

	// Get fresh balance from database
	return bm.GetBalance(ctx, userID, asset)
}

// Private helper methods

func (bm *BalanceManager) createZeroBalance(ctx context.Context, userID uuid.UUID, asset string) (*interfaces.AssetBalance, error) {
	balance := &interfaces.WalletBalance{
		ID:        uuid.New(),
		UserID:    userID,
		Asset:     asset,
		Available: decimal.Zero,
		Locked:    decimal.Zero,
		Total:     decimal.Zero,
		UpdatedAt: time.Now(),
	}

	if err := bm.repository.CreateBalance(ctx, balance); err != nil {
		return nil, fmt.Errorf("failed to create zero balance: %w", err)
	}

	return &interfaces.AssetBalance{
		Asset:     balance.Asset,
		Available: balance.Available,
		Locked:    balance.Locked,
		Total:     balance.Total,
		UpdatedAt: balance.UpdatedAt,
	}, nil
}

func (bm *BalanceManager) getOrCreateBalanceForUpdate(ctx context.Context, tx *gorm.DB, userID uuid.UUID, asset string) (*interfaces.WalletBalance, error) {
	// Try to get existing balance with row lock
	balance, err := bm.repository.GetBalanceForUpdateInTx(ctx, tx, userID, asset)
	if err == nil {
		return balance, nil
	}

	if err == gorm.ErrRecordNotFound {
		// Create new balance
		balance = &interfaces.WalletBalance{
			ID:        uuid.New(),
			UserID:    userID,
			Asset:     asset,
			Available: decimal.Zero,
			Locked:    decimal.Zero,
			Total:     decimal.Zero,
			UpdatedAt: time.Now(),
		}

		if err := bm.repository.CreateBalanceInTx(ctx, tx, balance); err != nil {
			return nil, fmt.Errorf("failed to create balance: %w", err)
		}

		return balance, nil
	}

	return nil, err
}

func (bm *BalanceManager) invalidateBalanceCache(userID uuid.UUID, asset string) {
	if bm.cache != nil {
		cacheKey := fmt.Sprintf("balance:%s:%s", userID.String(), asset)
		if err := bm.cache.Delete(context.Background(), cacheKey); err != nil {
			bm.logger.Warn("Failed to invalidate balance cache", zap.Error(err))
		}
	}
}
