// Package bookkeeper provides adapter types for compatibility with both BookkeeperService and XAResource interfaces
package bookkeeper

import (
	"context"
	"errors"

	"github.com/Aidin1998/finalex/pkg/models"
	"github.com/shopspring/decimal"
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
func (a *BookkeeperXAAdapter) CreateTransaction(ctx context.Context, userID, transactionType string, amount decimal.Decimal, currency, reference, description string) (*models.Transaction, error) {
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
func (a *BookkeeperXAAdapter) LockFunds(ctx context.Context, userID, currency string, amount decimal.Decimal) error {
	return a.bookkeeper.LockFunds(ctx, userID, currency, amount)
}

// UnlockFunds implements the BookkeeperService interface (non-XA version)
func (a *BookkeeperXAAdapter) UnlockFunds(ctx context.Context, userID, currency string, amount decimal.Decimal) error {
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

// BookkeeperXAAdapter implements BookkeeperXAService for XA compatibility
// Add XAResource and XA methods
func (a *BookkeeperXAAdapter) GetResourceName() string {
	if xa, ok := a.xaResource.(interface{ GetResourceName() string }); ok {
		return xa.GetResourceName()
	}
	return "bookkeeper-xa-adapter"
}
func (a *BookkeeperXAAdapter) StartXA(xid XID) error {
	if xa, ok := a.xaResource.(interface{ StartXA(XID) error }); ok {
		return xa.StartXA(xid)
	}
	return ErrXANotSupported
}
func (a *BookkeeperXAAdapter) EndXA(xid XID, flags int) error {
	if xa, ok := a.xaResource.(interface{ EndXA(XID, int) error }); ok {
		return xa.EndXA(xid, flags)
	}
	return ErrXANotSupported
}
func (a *BookkeeperXAAdapter) PrepareXA(xid XID) error {
	if xa, ok := a.xaResource.(interface{ PrepareXA(XID) error }); ok {
		return xa.PrepareXA(xid)
	}
	return ErrXANotSupported
}
func (a *BookkeeperXAAdapter) CommitXA(xid XID, onePhase bool) error {
	if xa, ok := a.xaResource.(interface{ CommitXA(XID, bool) error }); ok {
		return xa.CommitXA(xid, onePhase)
	}
	return ErrXANotSupported
}
func (a *BookkeeperXAAdapter) RollbackXA(xid XID) error {
	if xa, ok := a.xaResource.(interface{ RollbackXA(XID) error }); ok {
		return xa.RollbackXA(xid)
	}
	return ErrXANotSupported
}
func (a *BookkeeperXAAdapter) RecoverXA() ([]XID, error) {
	if xa, ok := a.xaResource.(interface{ RecoverXA() ([]XID, error) }); ok {
		return xa.RecoverXA()
	}
	return nil, ErrXANotSupported
}

// XA-specific bookkeeper operations
func (a *BookkeeperXAAdapter) LockFundsXA(ctx context.Context, xid XID, userID, currency string, amount decimal.Decimal) error {
	if xa, ok := a.xaResource.(BookkeeperXAService); ok {
		return xa.LockFundsXA(ctx, xid, userID, currency, amount)
	}
	return ErrXANotSupported
}
func (a *BookkeeperXAAdapter) UnlockFundsXA(ctx context.Context, xid XID, userID, currency string, amount decimal.Decimal) error {
	if xa, ok := a.xaResource.(BookkeeperXAService); ok {
		return xa.UnlockFundsXA(ctx, xid, userID, currency, amount)
	}
	return ErrXANotSupported
}
func (a *BookkeeperXAAdapter) TransferFundsXA(ctx context.Context, xid XID, fromUserID, toUserID, currency string, amount decimal.Decimal, reference string) error {
	if xa, ok := a.xaResource.(BookkeeperXAService); ok {
		return xa.TransferFundsXA(ctx, xid, fromUserID, toUserID, currency, amount, reference)
	}
	return ErrXANotSupported
}
func (a *BookkeeperXAAdapter) CreateTransactionXA(ctx context.Context, xid XID, userID, transactionType string, amount decimal.Decimal, currency, reference, description string) (*models.Transaction, error) {
	if xa, ok := a.xaResource.(BookkeeperXAService); ok {
		return xa.CreateTransactionXA(ctx, xid, userID, transactionType, amount, currency, reference, description)
	}
	return nil, ErrXANotSupported
}

// Ensure BookkeeperXAAdapter implements BookkeeperXAService
var _ BookkeeperXAService = (*BookkeeperXAAdapter)(nil)

// NOTE: BookkeeperXAAdapter does not assert BookkeeperXAService due to XID type import cycle and package boundary issues.
