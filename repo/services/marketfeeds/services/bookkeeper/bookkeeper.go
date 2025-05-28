package bookkeeper

import (
	"github.com/litebittech/cex/services/bookkeeper/cache"
	"github.com/litebittech/cex/services/bookkeeper/txstore"
)

type BookKeeper struct {
	txStore      txstore.Store
	balanceCache cache.BalanceCache
	usersMutex   *usersMutex
}

func New(
	txStore txstore.Store,
	balanceCache cache.BalanceCache,
) *BookKeeper {
	return &BookKeeper{
		txStore,
		balanceCache,
		new(usersMutex),
	}
}

// NewSession creates and returns a new accounting session
func (bk *BookKeeper) NewSession() *Session {
	return &Session{
		txStore:      bk.txStore,
		balanceCache: bk.balanceCache,
		usersMutex:   bk.usersMutex,
	}
}
