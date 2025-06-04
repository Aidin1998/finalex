package wallet

import (
	"context"
)

// Service defines the consolidated wallet and blockchain service interface
type Service interface {
	// Wallet operations
	CreateWallet(ctx context.Context, userID, currency string) (string, error)
	GetWallet(ctx context.Context, walletID string) (interface{}, error)
	GetWallets(ctx context.Context, userID string) ([]interface{}, error)
	GetWalletBalance(ctx context.Context, walletID string) (float64, error)

	// Deposit operations
	GenerateDepositAddress(ctx context.Context, walletID string) (string, error)
	GetDepositHistory(ctx context.Context, walletID string) ([]interface{}, error)

	// Withdrawal operations
	RequestWithdrawal(ctx context.Context, walletID, address string, amount float64) (string, error)
	GetWithdrawalStatus(ctx context.Context, withdrawalID string) (string, error)
	GetWithdrawalHistory(ctx context.Context, walletID string) ([]interface{}, error)

	// Blockchain operations
	ValidateAddress(ctx context.Context, currency, address string) (bool, error)
	GetBlockchainStatus(ctx context.Context, currency string) (interface{}, error)
	GetTransactionStatus(ctx context.Context, txHash string) (string, error)

	// Service lifecycle
	Start() error
	Stop() error
}

// NewService creates a new consolidated wallet service
// func NewService(logger *zap.Logger, db *gorm.DB, bookkeeperSvc accounts.Service) (Service, error)
