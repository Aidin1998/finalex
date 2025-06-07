// Package services provides fund locking functionality
package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Aidin1998/finalex/internal/wallet/interfaces"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// FundLockService manages fund locks for pending operations
type FundLockService struct {
	db         *gorm.DB
	logger     *zap.Logger
	repository interfaces.WalletRepository
	cache      interfaces.WalletCache
	config     *FundLockConfig
}

// FundLockConfig holds fund lock configuration
type FundLockConfig struct {
	DefaultTimeout    time.Duration
	MaxLocksPerUser   int
	CleanupInterval   time.Duration
	LockRetryAttempts int
	LockRetryDelay    time.Duration
	CleanupBatchSize  int
}

// NewFundLockService creates a new fund lock service
func NewFundLockService(
	db *gorm.DB,
	logger *zap.Logger,
	repository interfaces.WalletRepository,
	cache interfaces.WalletCache,
	config *FundLockConfig,
) *FundLockService {
	return &FundLockService{
		db:         db,
		logger:     logger,
		repository: repository,
		cache:      cache,
		config:     config,
	}
}

// LockFunds locks funds for a user and asset
func (fls *FundLockService) LockFunds(ctx context.Context, userID uuid.UUID, asset string, amount decimal.Decimal, reason, txRef string) error {
	fls.logger.Info("Locking funds",
		zap.String("user_id", userID.String()),
		zap.String("asset", asset),
		zap.String("amount", amount.String()),
		zap.String("reason", reason),
	)

	// Validate input
	if amount.LessThanOrEqual(decimal.Zero) {
		return fmt.Errorf("lock amount must be positive")
	}

	// Check maximum locks per user
	lockCount, err := fls.repository.CountUserLocks(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to check user lock count: %w", err)
	}

	if lockCount >= int64(fls.config.MaxLocksPerUser) {
		return fmt.Errorf("maximum locks per user exceeded")
	}

	// Start database transaction with retry logic
	for attempt := 0; attempt < fls.config.LockRetryAttempts; attempt++ {
		err = fls.db.Transaction(func(tx *gorm.DB) error {
			// Get current balance with row lock
			balance, err := fls.repository.GetBalanceForUpdateInTx(ctx, tx, userID, asset)
			if err != nil {
				return fmt.Errorf("failed to get balance for update: %w", err)
			}

			// Check if sufficient funds available
			if balance.Available.LessThan(amount) {
				return fmt.Errorf("insufficient available balance: required %s, available %s",
					amount.String(), balance.Available.String())
			}

			// Create fund lock
			lock := &interfaces.FundLock{
				ID:        uuid.New(),
				UserID:    userID,
				Asset:     asset,
				Amount:    amount,
				Reason:    reason,
				TxRef:     txRef,
				Expires:   fls.calculateExpiry(),
				CreatedAt: time.Now(),
			}

			// Create lock record
			if err := fls.repository.CreateFundLockInTx(ctx, tx, lock); err != nil {
				return fmt.Errorf("failed to create fund lock: %w", err)
			}

			// Update balance - move amount from available to locked
			balance.Available = balance.Available.Sub(amount)
			balance.Locked = balance.Locked.Add(amount)
			balance.UpdatedAt = time.Now()

			if err := fls.repository.UpdateBalanceInTx(ctx, tx, balance); err != nil {
				return fmt.Errorf("failed to update balance: %w", err)
			}

			return nil
		})

		if err != nil {
			if fls.isRetryableError(err) && attempt < fls.config.LockRetryAttempts-1 {
				fls.logger.Warn("Retrying fund lock due to retryable error",
					zap.Error(err), zap.Int("attempt", attempt+1))
				time.Sleep(fls.config.LockRetryDelay)
				continue
			}
			return err
		}

		break // Success
	}

	fls.logger.Info("Successfully locked funds",
		zap.String("user_id", userID.String()),
		zap.String("asset", asset),
		zap.String("amount", amount.String()),
	)

	return nil
}

// ReleaseLock releases a fund lock
func (fls *FundLockService) ReleaseLock(ctx context.Context, userID uuid.UUID, asset string, amount decimal.Decimal, reason, txRef string) error {
	fls.logger.Info("Releasing fund lock",
		zap.String("user_id", userID.String()),
		zap.String("asset", asset),
		zap.String("amount", amount.String()),
		zap.String("reason", reason),
	)

	return fls.db.Transaction(func(tx *gorm.DB) error {
		// Get current balance with row lock
		balance, err := fls.repository.GetBalanceForUpdateInTx(ctx, tx, userID, asset)
		if err != nil {
			return fmt.Errorf("failed to get balance for update: %w", err)
		}

		// Update balance - move amount from locked back to available
		balance.Available = balance.Available.Add(amount)
		balance.Locked = balance.Locked.Sub(amount)
		balance.UpdatedAt = time.Now()

		// Ensure locked balance doesn't go negative
		if balance.Locked.LessThan(decimal.Zero) {
			balance.Locked = decimal.Zero
		}

		if err := fls.repository.UpdateBalanceInTx(ctx, tx, balance); err != nil {
			return fmt.Errorf("failed to update balance: %w", err)
		}

		return nil
	})
}

// GetAvailableBalance returns available balance for a user and asset
func (fls *FundLockService) GetAvailableBalance(ctx context.Context, userID uuid.UUID, asset string) (decimal.Decimal, error) {
	balance, err := fls.repository.GetBalance(ctx, userID, asset)
	if err != nil {
		return decimal.Zero, fmt.Errorf("failed to get balance: %w", err)
	}
	return balance.Available, nil
}

// GetUserLocks returns all active locks for a user
func (fls *FundLockService) GetUserLocks(ctx context.Context, userID uuid.UUID) ([]*interfaces.FundLock, error) {
	return fls.repository.GetUserFundLocks(ctx, userID)
}

// CleanExpiredLocks removes all expired fund locks and restores balance
func (fls *FundLockService) CleanExpiredLocks(ctx context.Context) error {
	fls.logger.Info("Starting cleanup of expired fund locks")

	// Get all expired locks
	expiredLocks, err := fls.repository.GetExpiredFundLocks(ctx, time.Now())
	if err != nil {
		return fmt.Errorf("failed to get expired fund locks: %w", err)
	}

	if len(expiredLocks) == 0 {
		fls.logger.Debug("No expired fund locks found")
		return nil
	}

	fls.logger.Info("Found expired fund locks to clean",
		zap.Int("count", len(expiredLocks)))

	// Process in batches
	batchSize := fls.config.CleanupBatchSize
	if batchSize == 0 {
		batchSize = 100 // default batch size
	}

	for i := 0; i < len(expiredLocks); i += batchSize {
		end := i + batchSize
		if end > len(expiredLocks) {
			end = len(expiredLocks)
		}

		batch := expiredLocks[i:end]
		if err := fls.processExpiredLocksBatch(ctx, batch); err != nil {
			fls.logger.Error("Failed to process expired locks batch",
				zap.Int("batch_start", i),
				zap.Int("batch_end", end),
				zap.Error(err))
			// Continue with next batch
		}
	}

	fls.logger.Info("Completed cleanup of expired fund locks",
		zap.Int("processed", len(expiredLocks)))

	return nil
}

// Private helper methods

func (fls *FundLockService) calculateExpiry() *time.Time {
	if fls.config.DefaultTimeout > 0 {
		expiry := time.Now().Add(fls.config.DefaultTimeout)
		return &expiry
	}
	return nil // No expiry
}

func (fls *FundLockService) isRetryableError(err error) bool {
	// Check for database-specific retryable errors
	errStr := strings.ToLower(err.Error())

	// Common retryable error patterns
	retryablePatterns := []string{
		"deadlock",
		"lock timeout",
		"connection lost",
		"transaction serialization failure",
	}

	for _, pattern := range retryablePatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}

	return false
}

func (fls *FundLockService) processExpiredLocksBatch(ctx context.Context, locks []*interfaces.FundLock) error {
	return fls.db.Transaction(func(tx *gorm.DB) error {
		for _, lock := range locks {
			if err := fls.releaseExpiredLock(ctx, tx, lock); err != nil {
				fls.logger.Error("Failed to release expired lock",
					zap.String("lock_id", lock.ID.String()),
					zap.Error(err))
				// Continue with other locks
			}
		}
		return nil
	})
}

func (fls *FundLockService) releaseExpiredLock(ctx context.Context, tx *gorm.DB, lock *interfaces.FundLock) error {
	// Get current balance with row lock
	balance, err := fls.repository.GetBalanceForUpdateInTx(ctx, tx, lock.UserID, lock.Asset)
	if err != nil {
		return fmt.Errorf("failed to get balance for update: %w", err)
	}

	// Update balance - move amount from locked back to available
	balance.Available = balance.Available.Add(lock.Amount)
	balance.Locked = balance.Locked.Sub(lock.Amount)
	balance.UpdatedAt = time.Now()

	// Ensure locked balance doesn't go negative
	if balance.Locked.LessThan(decimal.Zero) {
		balance.Locked = decimal.Zero
	}

	if err := fls.repository.UpdateBalanceInTx(ctx, tx, balance); err != nil {
		return fmt.Errorf("failed to update balance: %w", err)
	}

	// Delete lock record
	if err := fls.repository.DeleteFundLockInTx(ctx, tx, lock.ID); err != nil {
		return fmt.Errorf("failed to delete fund lock: %w", err)
	}

	fls.logger.Info("Released expired fund lock",
		zap.String("lock_id", lock.ID.String()),
		zap.String("user_id", lock.UserID.String()),
		zap.String("asset", lock.Asset),
		zap.String("amount", lock.Amount.String()),
	)

	return nil
}
