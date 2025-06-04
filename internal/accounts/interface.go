package accounts

import (
	"context"
)

// Service defines the consolidated account and balance management service
type Service interface {
	// Account operations
	CreateAccount(ctx context.Context, userID, currency string) (interface{}, error)
	GetAccount(ctx context.Context, accountID string) (interface{}, error)
	GetAccounts(ctx context.Context, userID string) ([]interface{}, error)

	// Balance operations
	GetBalance(ctx context.Context, accountID string) (float64, error)
	Credit(ctx context.Context, accountID string, amount float64, reference string) error
	Debit(ctx context.Context, accountID string, amount float64, reference string) error
	Transfer(ctx context.Context, fromAccountID, toAccountID string, amount float64, reference string) error

	// Locking operations
	LockFunds(ctx context.Context, accountID string, amount float64) error
	UnlockFunds(ctx context.Context, accountID string, amount float64) error

	// Transaction operations
	CreateTransaction(ctx context.Context, userID, txType string, amount float64, currency, provider, description string) (interface{}, error)
	CompleteTransaction(ctx context.Context, transactionID string) error
	FailTransaction(ctx context.Context, transactionID string) error
	GetTransaction(ctx context.Context, transactionID string) (interface{}, error)

	// Service lifecycle
	Start() error
	Stop() error
}

// NewService creates a new consolidated accounts service
// func NewService(logger *zap.Logger, db *gorm.DB) (Service, error)
