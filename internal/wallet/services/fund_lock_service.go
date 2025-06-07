// Package services provides fund locking functionality
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
func (fls *FundLockService) LockFunds(ctx context.Context, userID uuid.UUID, asset string, amount decimal.Decimal, reason, txRef string) (string, error) {
	fls.logger.Info("Locking funds",
		zap.String("user_id", userID.String()),
		zap.String("asset", asset),
		zap.String("amount", amount.String()),
		zap.String("reason", reason),
	)

	// Validate input
	if amount.LessThanOrEqual(decimal.Zero) {
		return "", fmt.Errorf("lock amount must be positive")
	}

	// Check maximum locks per user
	lockCount, err := fls.repository.CountUserLocks(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("failed to check user lock count: %w", err)
	}

	if lockCount >= fls.config.MaxLocksPerUser {
		return "", fmt.Errorf("maximum locks per user exceeded")
	}

	// Start database transaction with retry logic
	var lockID string
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

			lockID = lock.ID.String()
			return nil
		})

		if err != nil {
			if fls.isRetryableError(err) && attempt < fls.config.LockRetryAttempts-1 {
				fls.logger.Warn("Retrying fund lock due to retryable error",
					zap.Error(err), zap.Int("attempt", attempt+1))
				time.Sleep(fls.config.LockRetryDelay)
				continue
			}
			return "", err
		}

		break // Success
	}

	// Cache the lock for quick access
	if fls.cache != nil {
		cacheKey := fmt.Sprintf("fund_lock:%s", lockID)
		if err := fls.cache.Set(ctx, cacheKey, lockID, time.Hour); err != nil {
			fls.logger.Warn("Failed to cache fund lock", zap.Error(err))
		}
	}

	fls.logger.Info("Successfully locked funds",
		zap.String("lock_id", lockID),
		zap.String("user_id", userID.String()),
		zap.String("asset", asset),
		zap.String("amount", amount.String()),
	)

	return lockID, nil
}

// ReleaseLock releases a fund lock
func (fls *FundLockService) ReleaseLock(ctx context.Context, lockID string) error {
	fls.logger.Info("Releasing fund lock", zap.String("lock_id", lockID))

	lockUUID, err := uuid.Parse(lockID)
	if err != nil {
		return fmt.Errorf("invalid lock ID format: %w", err)
	}

	return fls.db.Transaction(func(tx *gorm.DB) error {
		// Get lock with row lock
		lock, err := fls.repository.GetFundLockForUpdateInTx(ctx, tx, lockUUID)
		if err != nil {
			if err == gorm.ErrRecordNotFound {
				fls.logger.Warn("Fund lock not found, may already be released", zap.String("lock_id", lockID))
				return nil // Already released
			}
			return fmt.Errorf("failed to get fund lock: %w", err)
		}

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
		if err := fls.repository.DeleteFundLockInTx(ctx, tx, lockUUID); err != nil {
			return fmt.Errorf("failed to delete fund lock: %w", err)
		}

		return nil
	})
}

// ConvertLock converts a lock to a permanent balance change (e.g., for completed withdrawals)
func (fls *FundLockService) ConvertLock(ctx context.Context, lockID string, debitAmount decimal.Decimal) error {
	fls.logger.Info("Converting fund lock",
		zap.String("lock_id", lockID),
		zap.String("debit_amount", debitAmount.String()),
	)

	lockUUID, err := uuid.Parse(lockID)
	if err != nil {
		return fmt.Errorf("invalid lock ID format: %w", err)
	}

	return fls.db.Transaction(func(tx *gorm.DB) error {
		// Get lock with row lock
		lock, err := fls.repository.GetFundLockForUpdateInTx(ctx, tx, lockUUID)
		if err != nil {
			return fmt.Errorf("failed to get fund lock: %w", err)
		}

		// Validate debit amount
		if debitAmount.GreaterThan(lock.Amount) {
			return fmt.Errorf("debit amount exceeds locked amount")
		}

		// Get current balance with row lock
		balance, err := fls.repository.GetBalanceForUpdateInTx(ctx, tx, lock.UserID, lock.Asset)
		if err != nil {
			return fmt.Errorf("failed to get balance for update: %w", err)
		}

		// Update balance - remove debit amount from locked, calculate difference for available
		balance.Locked = balance.Locked.Sub(lock.Amount)

		// If debit amount is less than lock amount, return difference to available
		if debitAmount.LessThan(lock.Amount) {
			difference := lock.Amount.Sub(debitAmount)
			balance.Available = balance.Available.Add(difference)
		}

		// Update total balance
		balance.Total = balance.Available.Add(balance.Locked)
		balance.UpdatedAt = time.Now()

		// Ensure balances don't go negative
		if balance.Locked.LessThan(decimal.Zero) {
			balance.Locked = decimal.Zero
		}
		if balance.Available.LessThan(decimal.Zero) {
			balance.Available = decimal.Zero
		}

		if err := fls.repository.UpdateBalanceInTx(ctx, tx, balance); err != nil {
			return fmt.Errorf("failed to update balance: %w", err)
		}

		// Delete lock record
		if err := fls.repository.DeleteFundLockInTx(ctx, tx, lockUUID); err != nil {
			return fmt.Errorf("failed to delete fund lock: %w", err)
		}

		return nil
	})
}

// ExtendLock extends the expiry time of a lock
func (fls *FundLockService) ExtendLock(ctx context.Context, lockID string, extension time.Duration) error {
	fls.logger.Info("Extending fund lock",
		zap.String("lock_id", lockID),
		zap.Duration("extension", extension),
	)

	lockUUID, err := uuid.Parse(lockID)
	if err != nil {
		return fmt.Errorf("invalid lock ID format: %w", err)
	}

	// Get lock
	lock, err := fls.repository.GetFundLock(ctx, lockUUID)
	if err != nil {
		return fmt.Errorf("failed to get fund lock: %w", err)
	}

	// Update expiry
	if lock.Expires != nil {
		newExpiry := lock.Expires.Add(extension)
		lock.Expires = &newExpiry
	} else {
		expiry := time.Now().Add(extension)
		lock.Expires = &expiry
	}

	// Update lock
	if err := fls.repository.UpdateFundLock(ctx, lock); err != nil {
		return fmt.Errorf("failed to update fund lock: %w", err)
	}

	return nil
}

// GetUserLocks returns all active locks for a user
func (fls *FundLockService) GetUserLocks(ctx context.Context, userID uuid.UUID) ([]*interfaces.FundLock, error) {
	return fls.repository.GetUserFundLocks(ctx, userID)
}

// GetLock returns a specific lock by ID
func (fls *FundLockService) GetLock(ctx context.Context, lockID string) (*interfaces.FundLock, error) {
	lockUUID, err := uuid.Parse(lockID)
	if err != nil {
		return nil, fmt.Errorf("invalid lock ID format: %w", err)
	}

	return fls.repository.GetFundLock(ctx, lockUUID)
}

// CleanupExpiredLocks removes expired locks and returns funds to available balance
func (fls *FundLockService) CleanupExpiredLocks(ctx context.Context) error {
	fls.logger.Info("Starting cleanup of expired fund locks")

	expiredLocks, err := fls.repository.GetExpiredFundLocks(ctx, time.Now())
	if err != nil {
		return fmt.Errorf("failed to get expired locks: %w", err)
	}

	if len(expiredLocks) == 0 {
		return nil
	}

	fls.logger.Info("Found expired locks", zap.Int("count", len(expiredLocks)))

	// Process expired locks in batches
	batchSize := 100
	for i := 0; i < len(expiredLocks); i += batchSize {
		end := i + batchSize
		if end > len(expiredLocks) {
			end = len(expiredLocks)
		}

		batch := expiredLocks[i:end]
		if err := fls.processExpiredLocksBatch(ctx, batch); err != nil {
			fls.logger.Error("Failed to process expired locks batch", zap.Error(err))
		}
	}

	return nil
}

// StartCleanupWorker starts a background worker to cleanup expired locks
func (fls *FundLockService) StartCleanupWorker(ctx context.Context) {
	ticker := time.NewTicker(fls.config.CleanupInterval)

	go func() {
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				fls.logger.Info("Fund lock cleanup worker stopped")
				return
			case <-ticker.C:
				if err := fls.CleanupExpiredLocks(ctx); err != nil {
					fls.logger.Error("Failed to cleanup expired locks", zap.Error(err))
				}
			}
		}
	}()

	fls.logger.Info("Fund lock cleanup worker started")
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
	// This would depend on the specific database being used
	errStr := err.Error()

	// Common retryable error patterns
	retryablePatterns := []string{
		"deadlock",
		"lock timeout",
		"connection lost",
		"transaction serialization failure",
	}

	for _, pattern := range retryablePatterns {
		if len(errStr) > 0 && errStr != pattern {
			// Use simple string contains check instead
			continue
		}
		return true
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
