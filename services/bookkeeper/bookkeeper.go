package bookkeeper

import (
	"context"
	"time"

	"github.com/litebittech/cex/common/errors"
	"github.com/litebittech/cex/services/bookkeeper/cache"
	"github.com/litebittech/cex/services/bookkeeper/store"
)

type BookKeeper struct {
	store        store.Store
	balanceCache cache.BalanceCache
	usersMutex   *usersMutex
}

func New(
	store store.Store,
	balanceCache cache.BalanceCache,
) *BookKeeper {
	return &BookKeeper{
		store,
		balanceCache,
		new(usersMutex),
	}
}

// NewSession creates and returns a new accounting session
func (bk *BookKeeper) NewSession() *Session {
	return &Session{
		store:        bk.store,
		balanceCache: bk.balanceCache,
		usersMutex:   bk.usersMutex,
	}
}

// CreateAccount creates a new account for a user
func (bk *BookKeeper) CreateAccount(ctx context.Context, account *store.Account) error {
	now := time.Now()
	if account.CreatedAt.IsZero() {
		account.CreatedAt = store.NewUnixMilli(now)
	}
	if account.UpdatedAt.IsZero() {
		account.UpdatedAt = store.NewUnixMilli(now)
	}

	// Create the account in the store
	if err := bk.store.CreateAccount(ctx, account); err != nil {
		return errors.New("failed to create account in store").Wrap(err)
	}

	return nil
}
