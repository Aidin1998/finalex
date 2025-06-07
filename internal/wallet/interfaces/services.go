// Package interfaces provides service interfaces for the wallet module
package interfaces

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// WalletService provides the main wallet functionality
type WalletService interface {
	// Deposit operations
	RequestDeposit(ctx context.Context, req *DepositRequest) (*DepositResponse, error)
	ProcessDeposit(ctx context.Context, txID uuid.UUID, fireblocksData *FireblocksData) error
	ConfirmDeposit(ctx context.Context, txID uuid.UUID, confirmations int) error

	// Withdrawal operations
	RequestWithdrawal(ctx context.Context, req *WithdrawalRequest) (*WithdrawalResponse, error)
	ProcessWithdrawal(ctx context.Context, txID uuid.UUID) error
	ConfirmWithdrawal(ctx context.Context, txID uuid.UUID, fireblocksStatus string) error
	CancelWithdrawal(ctx context.Context, txID uuid.UUID, reason string) error

	// Balance operations
	GetBalance(ctx context.Context, userID uuid.UUID, asset string) (*AssetBalance, error)
	GetBalances(ctx context.Context, userID uuid.UUID) (*BalanceResponse, error)
	UpdateBalance(ctx context.Context, userID uuid.UUID, asset string, amount decimal.Decimal, txType string, txRef string) error
	// Transaction management
	GetTransaction(ctx context.Context, txID uuid.UUID) (*WalletTransaction, error)
	GetTransactionStatus(ctx context.Context, txID uuid.UUID) (*TransactionStatus, error)
	GetUserTransactions(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*WalletTransaction, error)
	ListTransactions(ctx context.Context, userID uuid.UUID, asset string, direction string, limit, offset int) ([]*WalletTransaction, int64, error)

	// Address management
	GenerateAddress(ctx context.Context, userID uuid.UUID, asset, network string) (*DepositAddress, error)
	GetDepositAddress(ctx context.Context, userID uuid.UUID, asset, network string) (*DepositAddress, error)
	GetUserAddresses(ctx context.Context, userID uuid.UUID, asset string) ([]*DepositAddress, error)
	ValidateAddress(ctx context.Context, req *AddressValidationRequest) (*AddressValidationResult, error)

	// Service lifecycle
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	HealthCheck(ctx context.Context) error
}

// DepositManager handles deposit operations
type DepositManager interface {
	// Initiate deposit process
	InitiateDeposit(ctx context.Context, req *DepositRequest) (*DepositResponse, error)

	// Process incoming deposit from blockchain/fireblocks
	ProcessIncomingDeposit(ctx context.Context, txHash string, fireblocksData *FireblocksData) error

	// Confirm deposit with required confirmations
	ConfirmDeposit(ctx context.Context, txID uuid.UUID, confirmations int) error

	// Generate deposit address for user
	GenerateDepositAddress(ctx context.Context, userID uuid.UUID, asset, network string) (*DepositAddress, error)

	// Get deposit address for user
	GetDepositAddress(ctx context.Context, userID uuid.UUID, asset, network string) (*DepositAddress, error)

	// Validate deposit parameters
	ValidateDepositRequest(ctx context.Context, req *DepositRequest) error
}

// WithdrawalManager handles withdrawal operations
type WithdrawalManager interface {
	// Initiate withdrawal process
	InitiateWithdrawal(ctx context.Context, req *WithdrawalRequest) (*WithdrawalResponse, error)

	// Process withdrawal through fireblocks
	ProcessWithdrawal(ctx context.Context, txID uuid.UUID) error

	// Handle withdrawal status updates from fireblocks
	UpdateWithdrawalStatus(ctx context.Context, txID uuid.UUID, status string, txHash string) error

	// Cancel pending withdrawal
	CancelWithdrawal(ctx context.Context, txID uuid.UUID, reason string) error

	// Estimate withdrawal fee
	EstimateWithdrawalFee(ctx context.Context, asset, network string, amount decimal.Decimal) (decimal.Decimal, error)

	// Validate withdrawal request
	ValidateWithdrawalRequest(ctx context.Context, req *WithdrawalRequest) error

	// Check if withdrawal requires approval
	RequiresApproval(ctx context.Context, req *WithdrawalRequest) (bool, error)
}

// BalanceManager handles balance operations and calculations
type BalanceManager interface {
	// Get user balance for specific asset
	GetBalance(ctx context.Context, userID uuid.UUID, asset string) (*AssetBalance, error)

	// Get all balances for user
	GetBalances(ctx context.Context, userID uuid.UUID) (*BalanceResponse, error)
	// Credit user balance (deposits, releases)
	CreditBalance(ctx context.Context, userID uuid.UUID, asset string, amount decimal.Decimal, reference string) error

	// Debit user balance (withdrawals, locks)
	DebitBalance(ctx context.Context, userID uuid.UUID, asset string, amount decimal.Decimal, reference string) error

	// Update balance (generic)
	UpdateBalance(ctx context.Context, userID uuid.UUID, asset string, amount decimal.Decimal, txType, reference string) error

	// Lock funds for pending operations
	LockFunds(ctx context.Context, userID uuid.UUID, asset string, amount decimal.Decimal, reason, reference string) (string, error)

	// Release locked funds
	ReleaseFunds(ctx context.Context, lockID string) error

	// Transfer between internal accounts
	InternalTransfer(ctx context.Context, fromUserID, toUserID uuid.UUID, asset string, amount decimal.Decimal, reference string) error

	// Recalculate and verify balance consistency
	RecalculateBalance(ctx context.Context, userID uuid.UUID, asset string) error
}

// FundLockService handles fund locking/unlocking for pending operations
type FundLockService interface { // Create fund lock
	CreateLock(ctx context.Context, userID uuid.UUID, asset string, amount decimal.Decimal, reason, txRef string, expires *time.Time) (*FundLock, error)

	// Lock funds for operations
	LockFunds(ctx context.Context, userID uuid.UUID, asset string, amount decimal.Decimal, reason, txRef string) (string, error)

	// Release fund lock
	ReleaseLock(ctx context.Context, userID uuid.UUID, asset string, amount decimal.Decimal, reason, txRef string) error

	// Get available balance (total - locked)
	GetAvailableBalance(ctx context.Context, userID uuid.UUID, asset string) (decimal.Decimal, error)

	// Get active locks for user
	GetUserLocks(ctx context.Context, userID uuid.UUID, asset string) ([]*FundLock, error)

	// Get lock by transaction reference
	GetLockByTxRef(ctx context.Context, txRef string) (*FundLock, error)

	// Clean up expired locks
	CleanupExpiredLocks(ctx context.Context) error
	CleanExpiredLocks(ctx context.Context) error
}

// WalletRepository handles database operations for wallet entities
type WalletRepository interface { // Transaction operations
	CreateTransaction(ctx context.Context, tx *WalletTransaction) error
	GetTransaction(ctx context.Context, txID uuid.UUID) (*WalletTransaction, error)
	UpdateTransaction(ctx context.Context, txID uuid.UUID, updates map[string]interface{}) error
	UpdateTransactionInTx(ctx context.Context, dbTx *gorm.DB, tx *WalletTransaction) error
	GetUserTransactions(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*WalletTransaction, error)
	GetUserTransactionsByDirection(ctx context.Context, userID uuid.UUID, direction Direction, limit, offset int) ([]*WalletTransaction, error)
	GetTransactionByFireblocksID(ctx context.Context, fireblocksID string) (*WalletTransaction, error)
	GetTransactionByTxHash(ctx context.Context, txHash string) (*WalletTransaction, error)
	GetTransactionsByStatus(ctx context.Context, status TxStatus, limit int) ([]*WalletTransaction, error)
	CountPendingDeposits(ctx context.Context, userID uuid.UUID) (int, error)
	GetDailyWithdrawalTotal(ctx context.Context, userID uuid.UUID, asset string, date time.Time) (decimal.Decimal, error)

	// Balance operations
	GetBalance(ctx context.Context, userID uuid.UUID, asset string) (*WalletBalance, error)
	GetBalanceForUpdateInTx(ctx context.Context, tx *gorm.DB, userID uuid.UUID, asset string) (*WalletBalance, error)
	CreateBalance(ctx context.Context, balance *WalletBalance) error
	UpdateBalance(ctx context.Context, balance *WalletBalance) error
	UpdateBalanceInTx(ctx context.Context, tx *gorm.DB, balance *WalletBalance) error
	GetUserBalances(ctx context.Context, userID uuid.UUID) ([]*WalletBalance, error)

	// Address operations
	CreateAddress(ctx context.Context, address *DepositAddress) error
	CreateDepositAddress(ctx context.Context, address *DepositAddress) error
	GetDepositAddress(ctx context.Context, userID uuid.UUID, asset, network string) (*DepositAddress, error)
	GetUserAddresses(ctx context.Context, userID uuid.UUID, asset string) ([]*DepositAddress, error)
	GetAddressByAddress(ctx context.Context, address string) (*DepositAddress, error)
	GetAddressByValue(ctx context.Context, address, asset string) (*DepositAddress, error)
	UpdateAddress(ctx context.Context, addressID uuid.UUID, updates map[string]interface{}) error
	// Fund lock operations
	CreateFundLock(ctx context.Context, lock *FundLock) error
	CreateFundLockInTx(ctx context.Context, tx *gorm.DB, lock *FundLock) error
	GetFundLock(ctx context.Context, lockID uuid.UUID) (*FundLock, error)
	GetFundLockByTxRef(ctx context.Context, txRef string) (*FundLock, error)
	GetFundLocks(ctx context.Context, userID uuid.UUID, asset string) ([]*FundLock, error)
	ReleaseFundLock(ctx context.Context, lockID uuid.UUID) error
	DeleteFundLockInTx(ctx context.Context, tx *gorm.DB, lockID uuid.UUID) error
	GetUserFundLocks(ctx context.Context, userID uuid.UUID, asset string) ([]*FundLock, error)
	GetExpiredFundLocks(ctx context.Context, expiredBefore time.Time) ([]*FundLock, error)
	CleanupExpiredLocks(ctx context.Context) error
	CountUserLocks(ctx context.Context, userID uuid.UUID) (int, error)

	// Balance operations for state machine
	CreditBalance(ctx context.Context, userID uuid.UUID, asset string, amount decimal.Decimal) error
	DebitBalance(ctx context.Context, userID uuid.UUID, asset string, amount decimal.Decimal) error

	// Transaction operations
	WithinTransaction(ctx context.Context, fn func(tx *gorm.DB) error) error
}

// WalletCache handles caching operations for wallet data
type WalletCache interface {
	// Balance caching
	GetBalance(ctx context.Context, userID uuid.UUID, asset string) (*AssetBalance, error)
	SetBalance(ctx context.Context, userID uuid.UUID, asset string, balance *AssetBalance, ttl time.Duration) error
	InvalidateBalance(ctx context.Context, userID uuid.UUID, asset string) error
	InvalidateUserBalances(ctx context.Context, userID uuid.UUID) error
	// Transaction status caching
	GetTransactionStatus(ctx context.Context, txID uuid.UUID) (*TransactionStatus, error)
	SetTransactionStatus(ctx context.Context, txID uuid.UUID, status *TransactionStatus, ttl time.Duration) error
	InvalidateTransactionStatus(ctx context.Context, txID uuid.UUID) error
	GetTransaction(ctx context.Context, txID uuid.UUID) (*WalletTransaction, error)
	SetTransaction(ctx context.Context, tx *WalletTransaction, ttl time.Duration) error
	InvalidateTransaction(ctx context.Context, txID uuid.UUID) error
	// Address caching
	GetUserAddresses(ctx context.Context, userID uuid.UUID, asset string) ([]*DepositAddress, error)
	SetUserAddresses(ctx context.Context, userID uuid.UUID, asset string, addresses []*DepositAddress, ttl time.Duration) error
	InvalidateUserAddresses(ctx context.Context, userID uuid.UUID, asset string) error
	GetAddresses(ctx context.Context, userID uuid.UUID, asset string) ([]*DepositAddress, error)
	SetAddresses(ctx context.Context, userID uuid.UUID, asset string, addresses []*DepositAddress, ttl time.Duration) error
	InvalidateAddresses(ctx context.Context, userID uuid.UUID, asset string) error

	// General cache operations
	Get(ctx context.Context, key string) (interface{}, error)
	Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	Clear(ctx context.Context) error
}

// EventPublisher handles publishing wallet events to message queues
type EventPublisher interface {
	// Deposit events
	PublishDepositInitiated(ctx context.Context, event *DepositInitiatedEvent) error
	PublishDepositConfirmed(ctx context.Context, event *DepositConfirmedEvent) error
	PublishDepositCompleted(ctx context.Context, event *DepositCompletedEvent) error

	// Withdrawal events
	PublishWithdrawalInitiated(ctx context.Context, event *WithdrawalInitiatedEvent) error
	PublishWithdrawalProcessing(ctx context.Context, event *WithdrawalProcessingEvent) error
	PublishWithdrawalCompleted(ctx context.Context, event *WithdrawalCompletedEvent) error
	PublishWithdrawalCancelled(ctx context.Context, event *WithdrawalCancelledEvent) error
	PublishWithdrawalFailed(ctx context.Context, event *WithdrawalFailedEvent) error
	// Balance events
	PublishBalanceUpdated(ctx context.Context, userID uuid.UUID, asset string, oldBalance, newBalance decimal.Decimal) error

	// Generic event publishing
	PublishWalletEvent(ctx context.Context, event WalletEvent) error
	Publish(ctx context.Context, topic string, event interface{}) error
}

// AddressValidator validates blockchain addresses
type AddressValidator interface {
	ValidateAddress(ctx context.Context, req *AddressValidationRequest) (*AddressValidationResult, error)
	ValidateBTCAddress(ctx context.Context, address, network string) (*AddressValidationResult, error)
	ValidateETHAddress(ctx context.Context, address string) (*AddressValidationResult, error)
	ValidateERC20Address(ctx context.Context, address string) (*AddressValidationResult, error)
}

// ComplianceChecker handles compliance and risk checks
type ComplianceChecker interface {
	CheckWithdrawal(ctx context.Context, req *ComplianceCheckRequest) (*ComplianceCheckResult, error)
	CheckDeposit(ctx context.Context, req *ComplianceCheckRequest) (*ComplianceCheckResult, error)
	CheckAddress(ctx context.Context, address string, asset string) (*ComplianceCheckResult, error)
	ScreenUser(ctx context.Context, userID uuid.UUID) (*ComplianceCheckResult, error)
}

// NotificationService handles user notifications for wallet events
type NotificationService interface {
	NotifyDepositConfirmed(ctx context.Context, userID uuid.UUID, txID uuid.UUID, amount decimal.Decimal, asset string) error
	NotifyWithdrawalProcessing(ctx context.Context, userID uuid.UUID, txID uuid.UUID, amount decimal.Decimal, asset string) error
	NotifyWithdrawalCompleted(ctx context.Context, userID uuid.UUID, txID uuid.UUID, amount decimal.Decimal, asset string) error
	NotifyWithdrawalFailed(ctx context.Context, userID uuid.UUID, txID uuid.UUID, reason string) error
	NotifyLowBalance(ctx context.Context, userID uuid.UUID, asset string, balance decimal.Decimal) error
}

// FireblocksClient handles communication with Fireblocks API
type FireblocksClient interface {
	// Vault operations
	GetVaultAccounts(ctx context.Context) ([]*FireblocksVault, error)
	GetAssetBalance(ctx context.Context, vaultID, assetID string) (*FireblocksBalance, error)

	// Address operations
	GenerateAddress(ctx context.Context, req *FireblocksAddressRequest) (*FireblocksAddress, error)
	GetAddresses(ctx context.Context, vaultID, assetID string) ([]*FireblocksAddress, error)
	GenerateDepositAddress(ctx context.Context, vaultID, assetID string, params *AddressParams) (*FireblocksAddress, error)

	// Transaction operations
	CreateTransaction(ctx context.Context, req *FireblocksTransactionRequest) (*FireblocksTransaction, error)
	GetTransaction(ctx context.Context, txID string) (*FireblocksTransaction, error)
	CancelTransaction(ctx context.Context, txID string) (*FireblocksTransaction, error)
	GetTransactions(ctx context.Context, filters *FireblocksTransactionFilters) ([]*FireblocksTransaction, error)

	// Address validation
	ValidateAddress(ctx context.Context, req *FireblocksAddressValidationRequest) (*FireblocksAddressValidationResult, error)

	// Asset operations
	GetSupportedAssets(ctx context.Context) ([]*FireblocksAsset, error)
	GetNetworkFee(ctx context.Context, assetID string) (*FireblocksNetworkFee, error)

	// Webhook operations
	VerifyWebhook(ctx context.Context, signature, body string) (*FireblocksWebhookData, error)
}

// TransactionStateMachine manages transaction state transitions
type TransactionStateMachine interface {
	// Transition transaction to new state
	TransitionTo(ctx context.Context, txID uuid.UUID, newState TxStatus) error

	// Get current state
	GetCurrentState(ctx context.Context, txID uuid.UUID) (TxStatus, error)

	// Check if transition is valid
	CanTransitionTo(ctx context.Context, txID uuid.UUID, newState TxStatus) (bool, error)

	// Get allowed transitions from current state
	GetAllowedTransitions(ctx context.Context, txID uuid.UUID) ([]TxStatus, error)
}

// AddressManager handles address generation and management
type AddressManager interface {
	// Generate new deposit address for user
	GenerateAddress(ctx context.Context, userID uuid.UUID, asset, network string) (*DepositAddress, error)

	// Get existing addresses for user
	GetUserAddresses(ctx context.Context, userID uuid.UUID, asset string) ([]*DepositAddress, error)

	// Update address status or metadata
	UpdateAddress(ctx context.Context, addressID uuid.UUID, updates map[string]interface{}) error

	// Find address by address string
	FindAddress(ctx context.Context, address, asset string) (*DepositAddress, error)

	// Get address usage statistics
	GetAddressStatistics(ctx context.Context, userID uuid.UUID) (*AddressStatistics, error)

	// Validate address format
	ValidateAddress(ctx context.Context, req *AddressValidationRequest) (*AddressValidationResult, error)

	// Clean up unused addresses
	CleanupUnusedAddresses(ctx context.Context) error
}
