package accounts

import (
	"context"

	"github.com/Aidin1998/pincex_unified/internal/accounts/bookkeeper"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// BookkeeperAdapter adapts the bookkeeper service to the accounts service interface
type BookkeeperAdapter struct {
	bk  *bookkeeper.Service
	log *zap.Logger
	db  *gorm.DB
}

// NewBookkeeperAdapter creates a new adapter from a bookkeeper service
func NewBookkeeperAdapter(bkService *bookkeeper.Service, logger *zap.Logger, db *gorm.DB) Service {
	return &BookkeeperAdapter{
		bk:  bkService,
		log: logger,
		db:  db,
	}
}

// CreateAccount creates a new account
func (a *BookkeeperAdapter) CreateAccount(ctx context.Context, userID, currency string) (interface{}, error) {
	return a.bk.CreateAccount(ctx, userID, currency)
}

// GetAccount gets an account
func (a *BookkeeperAdapter) GetAccount(ctx context.Context, accountID string) (interface{}, error) {
	return a.bk.GetAccount(ctx, accountID)
}

// GetAccounts gets all accounts for a user
func (a *BookkeeperAdapter) GetAccounts(ctx context.Context, userID string) ([]interface{}, error) {
	accounts, err := a.bk.GetAccounts(ctx, userID)
	if err != nil {
		return nil, err
	}

	result := make([]interface{}, len(accounts))
	for i, acc := range accounts {
		result[i] = acc
	}
	return result, nil
}

// GetBalance gets the balance of an account
func (a *BookkeeperAdapter) GetBalance(ctx context.Context, accountID string) (float64, error) {
	return a.bk.GetBalance(ctx, accountID)
}

// Credit credits funds to an account
func (a *BookkeeperAdapter) Credit(ctx context.Context, accountID string, amount float64, reference string) error {
	return a.bk.Credit(ctx, accountID, amount, reference)
}

// Debit debits funds from an account
func (a *BookkeeperAdapter) Debit(ctx context.Context, accountID string, amount float64, reference string) error {
	return a.bk.Debit(ctx, accountID, amount, reference)
}

// Transfer transfers funds between accounts
func (a *BookkeeperAdapter) Transfer(ctx context.Context, fromAccountID, toAccountID string, amount float64, reference string) error {
	return a.bk.Transfer(ctx, fromAccountID, toAccountID, amount, reference)
}

// LockFunds locks funds in an account
func (a *BookkeeperAdapter) LockFunds(ctx context.Context, accountID string, amount float64) error {
	return a.bk.LockFunds(ctx, accountID, "", amount)
}

// UnlockFunds unlocks funds in an account
func (a *BookkeeperAdapter) UnlockFunds(ctx context.Context, accountID string, amount float64) error {
	return a.bk.UnlockFunds(ctx, accountID, "", amount)
}

// CreateTransaction creates a new transaction
func (a *BookkeeperAdapter) CreateTransaction(ctx context.Context, userID, txType string, amount float64, currency, provider, description string) (interface{}, error) {
	return a.bk.CreateTransaction(ctx, userID, txType, amount, currency, provider, description)
}

// CompleteTransaction completes a transaction
func (a *BookkeeperAdapter) CompleteTransaction(ctx context.Context, transactionID string) error {
	return a.bk.CompleteTransaction(ctx, transactionID)
}

// FailTransaction fails a transaction
func (a *BookkeeperAdapter) FailTransaction(ctx context.Context, transactionID string) error {
	return a.bk.FailTransaction(ctx, transactionID)
}

// GetTransaction gets a transaction
func (a *BookkeeperAdapter) GetTransaction(ctx context.Context, transactionID string) (interface{}, error) {
	return a.bk.GetTransaction(ctx, transactionID)
}

// Start starts the service
func (a *BookkeeperAdapter) Start() error {
	return nil
}

// Stop stops the service
func (a *BookkeeperAdapter) Stop() error {
	return nil
}
