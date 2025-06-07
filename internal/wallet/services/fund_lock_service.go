// Package services provides the FundLockService implementation for the wallet module
package services

import (
	"context"
	"time"

	"github.com/Aidin1998/finalex/internal/wallet/interfaces"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// FundLockServiceImpl implements interfaces.FundLockService
// It uses the wallet repository for persistence and cache for hot-path lookups
// No circular dependencies or duplicate logic

type FundLockServiceImpl struct {
	repository interfaces.WalletRepository
	cache      interfaces.WalletCache
}

func NewFundLockService(repository interfaces.WalletRepository, cache interfaces.WalletCache) interfaces.FundLockService {
	return &FundLockServiceImpl{
		repository: repository,
		cache:      cache,
	}
}

func (f *FundLockServiceImpl) CreateLock(ctx context.Context, userID uuid.UUID, asset string, amount decimal.Decimal, reason, txRef string, expires *time.Time) (*interfaces.FundLock, error) {
	lock := &interfaces.FundLock{
		ID:        uuid.New(),
		UserID:    userID,
		Asset:     asset,
		Amount:    amount,
		Reason:    reason,
		TxRef:     txRef,
		Expires:   expires,
		CreatedAt: time.Now(),
	}
	if err := f.repository.CreateFundLock(ctx, lock); err != nil {
		return nil, err
	}
	return lock, nil
}

func (f *FundLockServiceImpl) LockFunds(ctx context.Context, userID uuid.UUID, asset string, amount decimal.Decimal, reason, txRef string) (string, error) {
	lock, err := f.CreateLock(ctx, userID, asset, amount, reason, txRef, nil)
	if err != nil {
		return "", err
	}
	return lock.ID.String(), nil
}

func (f *FundLockServiceImpl) ReleaseLock(ctx context.Context, userID uuid.UUID, asset string, amount decimal.Decimal, reason, txRef string) error {
	lock, err := f.repository.GetFundLockByTxRef(ctx, txRef)
	if err != nil {
		return err
	}
	if lock.UserID != userID || lock.Asset != asset || !lock.Amount.Equal(amount) || lock.Reason != reason {
		return interfaces.ErrInvalidAmount // or a more specific error
	}
	return f.repository.ReleaseFundLock(ctx, lock.ID)
}

func (f *FundLockServiceImpl) GetAvailableBalance(ctx context.Context, userID uuid.UUID, asset string) (decimal.Decimal, error) {
	balance, err := f.repository.GetBalance(ctx, userID, asset)
	if err != nil {
		return decimal.Zero, err
	}
	return balance.Available, nil
}

func (f *FundLockServiceImpl) GetUserLocks(ctx context.Context, userID uuid.UUID, asset string) ([]*interfaces.FundLock, error) {
	return f.repository.GetFundLocks(ctx, userID, asset)
}

func (f *FundLockServiceImpl) GetLockByTxRef(ctx context.Context, txRef string) (*interfaces.FundLock, error) {
	return f.repository.GetFundLockByTxRef(ctx, txRef)
}

func (f *FundLockServiceImpl) CleanupExpiredLocks(ctx context.Context) error {
	return f.repository.CleanupExpiredLocks(ctx)
}

func (f *FundLockServiceImpl) CleanExpiredLocks(ctx context.Context) error {
	return f.repository.CleanupExpiredLocks(ctx)
}
