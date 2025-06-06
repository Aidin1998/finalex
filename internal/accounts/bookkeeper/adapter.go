// Package bookkeeper provides adapter types for compatibility with both BookkeeperService and XAResource interfaces
package bookkeeper

import (
	"context"
	"errors"

	"github.com/Aidin1998/finalex/pkg/models"
)

var ErrXANotSupported = errors.New("XA operations not supported by BookkeeperService adapter")

// BookkeeperXAAdapter is an adapter that implements BookkeeperService
// while delegating to both a BookkeeperService and a generic XAResource (interface{}).
type BookkeeperXAAdapter struct {
	bookkeeper BookkeeperService
	xaResource interface{}
}

// NewBookkeeperXAAdapter creates a new adapter that implements BookkeeperService
func NewBookkeeperXAAdapter(bookkeeper BookkeeperService, xaResource interface{}) *BookkeeperXAAdapter {
	return &BookkeeperXAAdapter{
		bookkeeper: bookkeeper,
		xaResource: xaResource,
	}
}

// Start implements BookkeeperService.Start
func (a *BookkeeperXAAdapter) Start() error {
	return a.bookkeeper.Start()
}

// Stop implements BookkeeperService.Stop
func (a *BookkeeperXAAdapter) Stop() error {
	return a.bookkeeper.Stop()
}

// GetAccounts implements the BookkeeperService interface
func (a *BookkeeperXAAdapter) GetAccounts(ctx context.Context, userID string) ([]*models.Account, error) {
	return a.bookkeeper.GetAccounts(ctx, userID)
}

// GetAccount implements the BookkeeperService interface
func (a *BookkeeperXAAdapter) GetAccount(ctx context.Context, userID, currency string) (*models.Account, error) {
	return a.bookkeeper.GetAccount(ctx, userID, currency)
}

// CreateAccount implements the BookkeeperService interface
func (a *BookkeeperXAAdapter) CreateAccount(ctx context.Context, userID, currency string) (*models.Account, error) {
	return a.bookkeeper.CreateAccount(ctx, userID, currency)
}

// GetAccountTransactions implements the BookkeeperService interface
func (a *BookkeeperXAAdapter) GetAccountTransactions(ctx context.Context, userID, currency string, limit, offset int) ([]*models.Transaction, int64, error) {
	return a.bookkeeper.GetAccountTransactions(ctx, userID, currency, limit, offset)
}

// CreateTransaction implements the BookkeeperService interface
func (a *BookkeeperXAAdapter) CreateTransaction(ctx context.Context, userID, transactionType string, amount float64, currency, reference, description string) (*models.Transaction, error) {
	return a.bookkeeper.CreateTransaction(ctx, userID, transactionType, amount, currency, reference, description)
}

// CompleteTransaction implements the BookkeeperService interface
func (a *BookkeeperXAAdapter) CompleteTransaction(ctx context.Context, transactionID string) error {
	return a.bookkeeper.CompleteTransaction(ctx, transactionID)
}

// FailTransaction implements the BookkeeperService interface
func (a *BookkeeperXAAdapter) FailTransaction(ctx context.Context, transactionID string) error {
	return a.bookkeeper.FailTransaction(ctx, transactionID)
}

// LockFunds implements the BookkeeperService interface (non-XA version)
func (a *BookkeeperXAAdapter) LockFunds(ctx context.Context, userID, currency string, amount float64) error {
	return a.bookkeeper.LockFunds(ctx, userID, currency, amount)
}

// UnlockFunds implements the BookkeeperService interface (non-XA version)
func (a *BookkeeperXAAdapter) UnlockFunds(ctx context.Context, userID, currency string, amount float64) error {
	return a.bookkeeper.UnlockFunds(ctx, userID, currency, amount)
}

// BatchGetAccounts implements the BookkeeperService interface
func (a *BookkeeperXAAdapter) BatchGetAccounts(ctx context.Context, userIDs []string, currencies []string) (map[string]map[string]*models.Account, error) {
	return a.bookkeeper.BatchGetAccounts(ctx, userIDs, currencies)
}

// BatchLockFunds implements the BookkeeperService interface
func (a *BookkeeperXAAdapter) BatchLockFunds(ctx context.Context, operations []FundsOperation) (*BatchOperationResult, error) {
	return a.bookkeeper.BatchLockFunds(ctx, operations)
}

// BatchUnlockFunds implements the BookkeeperService interface
func (a *BookkeeperXAAdapter) BatchUnlockFunds(ctx context.Context, operations []FundsOperation) (*BatchOperationResult, error) {
	return a.bookkeeper.BatchUnlockFunds(ctx, operations)
}

// GetXAResource returns the underlying XAResource implementation
func (a *BookkeeperXAAdapter) GetXAResource() interface{} {
	return a.xaResource
}

// Ensure BookkeeperXAAdapter implements BookkeeperService
var _ BookkeeperService = (*BookkeeperXAAdapter)(nil)

// NOTE: BookkeeperXAAdapter does not assert BookkeeperXAService due to XID type import cycle and package boundary issues.
