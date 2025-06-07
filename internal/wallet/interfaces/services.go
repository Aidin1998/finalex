// Package interfaces provides service interfaces for the wallet module
package interfaces

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
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

	// Address management
	GenerateAddress(ctx context.Context, userID uuid.UUID, asset, network string) (*DepositAddress, error)
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

	// Process incoming deposit notification
	ProcessIncomingDeposit(ctx context.Context, data *FireblocksData) error

	// Update deposit confirmation status
	UpdateConfirmations(ctx context.Context, txID uuid.UUID, confirmations int) error

	// Complete deposit and credit user
	CompleteDeposit(ctx context.Context, txID uuid.UUID) error

	// Get deposit requirements
	GetDepositRequirements(ctx context.Context, asset string) (*DepositRequirements, error)
}

// WithdrawalManager handles withdrawal operations
type WithdrawalManager interface {
	// Initiate withdrawal request
	InitiateWithdrawal(ctx context.Context, req *WithdrawalRequest) (*WithdrawalResponse, error)

	// Process approved withdrawal
	ProcessWithdrawal(ctx context.Context, txID uuid.UUID) error

	// Cancel pending withdrawal
	CancelWithdrawal(ctx context.Context, txID uuid.UUID, reason string) error

	// Update withdrawal status from Fireblocks
	UpdateStatus(ctx context.Context, txID uuid.UUID, status string, txHash string) error

	// Get withdrawal limits
	GetWithdrawalLimits(ctx context.Context, userID uuid.UUID, asset string) (*WithdrawalLimits, error)
}

// FundLockService manages fund locking for atomic operations
type FundLockService interface {
	// Lock funds for pending operations
	LockFunds(ctx context.Context, userID uuid.UUID, asset string, amount decimal.Decimal, reason, txRef string) error

	// Release locked funds
	ReleaseLock(ctx context.Context, userID uuid.UUID, asset string, amount decimal.Decimal, reason, txRef string) error

	// Get available balance after locks
	GetAvailableBalance(ctx context.Context, userID uuid.UUID, asset string) (decimal.Decimal, error)

	// Get all locks for user
	GetUserLocks(ctx context.Context, userID uuid.UUID) ([]*FundLock, error)

	// Clean expired locks
	CleanExpiredLocks(ctx context.Context) error
}

// BalanceManager handles balance operations
type BalanceManager interface {
	// Get current balance
	GetBalance(ctx context.Context, userID uuid.UUID, asset string) (*AssetBalance, error)

	// Get all balances for user
	GetBalances(ctx context.Context, userID uuid.UUID) (*BalanceResponse, error)

	// Update balance
	UpdateBalance(ctx context.Context, userID uuid.UUID, asset string, amount decimal.Decimal, txType, txRef string) error

	// Transfer between users (internal)
	Transfer(ctx context.Context, fromUserID, toUserID uuid.UUID, asset string, amount decimal.Decimal, reference string) error

	// Calculate total balance including locks
	CalculateBalance(ctx context.Context, userID uuid.UUID, asset string) (*AssetBalance, error)
}

// AddressManager handles address generation and validation
type AddressManager interface {
	// Generate new deposit address
	GenerateAddress(ctx context.Context, userID uuid.UUID, asset, network string) (*DepositAddress, error)

	// Get user addresses
	GetUserAddresses(ctx context.Context, userID uuid.UUID, asset string) ([]*DepositAddress, error)

	// Validate address format and network
	ValidateAddress(ctx context.Context, req *AddressValidationRequest) (*AddressValidationResult, error)

	// Deactivate address
	DeactivateAddress(ctx context.Context, addressID uuid.UUID) error

	// Get address by value
	GetAddressByValue(ctx context.Context, address, asset string) (*DepositAddress, error)
}

// TransactionStateMachine manages transaction state transitions
type TransactionStateMachine interface {
	// Transition transaction state
	TransitionState(ctx context.Context, txID uuid.UUID, newStatus TxStatus, metadata map[string]interface{}) error

	// Get current state
	GetCurrentState(ctx context.Context, txID uuid.UUID) (TxStatus, error)

	// Validate state transition
	ValidateTransition(currentState, newState TxStatus) error

	// Get allowed transitions
	GetAllowedTransitions(currentState TxStatus) []TxStatus
}

// ComplianceIntegration handles compliance checks
type ComplianceIntegration interface {
	// Check transaction compliance
	CheckCompliance(ctx context.Context, req *ComplianceCheckRequest) (*ComplianceCheckResult, error)

	// Pre-check before transaction
	PreTransactionCheck(ctx context.Context, userID uuid.UUID, asset string, amount decimal.Decimal, direction Direction) error

	// Post-transaction reporting
	PostTransactionReport(ctx context.Context, tx *WalletTransaction) error
}

// EventPublisher publishes wallet events
type EventPublisher interface {
	// Publish wallet event
	PublishEvent(ctx context.Context, event *WalletEvent) error

	// Publish batch events
	PublishBatch(ctx context.Context, events []*WalletEvent) error
}

// Repository interfaces

// WalletRepository handles wallet data persistence
type WalletRepository interface {
	// Transaction operations
	CreateTransaction(ctx context.Context, tx *WalletTransaction) error
	UpdateTransaction(ctx context.Context, txID uuid.UUID, updates map[string]interface{}) error
	GetTransaction(ctx context.Context, txID uuid.UUID) (*WalletTransaction, error)
	GetTransactionByFireblocksID(ctx context.Context, fireblocksID string) (*WalletTransaction, error)
	GetUserTransactions(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*WalletTransaction, error)
	GetTransactionsByStatus(ctx context.Context, status TxStatus, limit int) ([]*WalletTransaction, error)

	// Balance operations
	GetBalance(ctx context.Context, userID uuid.UUID, asset string) (*WalletBalance, error)
	UpdateBalance(ctx context.Context, userID uuid.UUID, asset string, available, locked decimal.Decimal) error
	GetAllBalances(ctx context.Context, userID uuid.UUID) ([]*WalletBalance, error)

	// Lock operations
	CreateLock(ctx context.Context, lock *FundLock) error
	DeleteLock(ctx context.Context, lockID uuid.UUID) error
	GetUserLocks(ctx context.Context, userID uuid.UUID) ([]*FundLock, error)
	GetLocksByTxRef(ctx context.Context, txRef string) ([]*FundLock, error)
	DeleteExpiredLocks(ctx context.Context, before time.Time) error

	// Address operations
	CreateAddress(ctx context.Context, addr *DepositAddress) error
	GetUserAddresses(ctx context.Context, userID uuid.UUID, asset string) ([]*DepositAddress, error)
	GetAddressByValue(ctx context.Context, address, asset string) (*DepositAddress, error)
	UpdateAddress(ctx context.Context, addressID uuid.UUID, updates map[string]interface{}) error
}

// Cache interface for performance
type WalletCache interface {
	// Balance caching
	GetBalance(ctx context.Context, userID uuid.UUID, asset string) (*AssetBalance, error)
	SetBalance(ctx context.Context, userID uuid.UUID, asset string, balance *AssetBalance, ttl time.Duration) error
	InvalidateBalance(ctx context.Context, userID uuid.UUID, asset string) error

	// Lock caching
	GetLocks(ctx context.Context, userID uuid.UUID) ([]*FundLock, error)
	SetLocks(ctx context.Context, userID uuid.UUID, locks []*FundLock, ttl time.Duration) error
	InvalidateLocks(ctx context.Context, userID uuid.UUID) error

	// Address caching
	GetAddresses(ctx context.Context, userID uuid.UUID, asset string) ([]*DepositAddress, error)
	SetAddresses(ctx context.Context, userID uuid.UUID, asset string, addresses []*DepositAddress, ttl time.Duration) error
	InvalidateAddresses(ctx context.Context, userID uuid.UUID, asset string) error
}

// HealthChecker interface for health monitoring
type HealthChecker interface {
	CheckHealth(ctx context.Context) error
}

// Supporting types

// DepositRequirements represents deposit requirements for an asset
type DepositRequirements struct {
	Asset              string          `json:"asset"`
	Network            string          `json:"network"`
	MinDeposit         decimal.Decimal `json:"min_deposit"`
	RequiredConf       int             `json:"required_confirmations"`
	ProcessingTime     time.Duration   `json:"processing_time"`
	NetworkFee         decimal.Decimal `json:"network_fee"`
	SupportsTag        bool            `json:"supports_tag"`
	MaintenanceMode    bool            `json:"maintenance_mode"`
	MaintenanceMessage string          `json:"maintenance_message,omitempty"`
}

// WithdrawalLimits represents withdrawal limits for a user
type WithdrawalLimits struct {
	Asset            string          `json:"asset"`
	DailyLimit       decimal.Decimal `json:"daily_limit"`
	DailyUsed        decimal.Decimal `json:"daily_used"`
	DailyRemaining   decimal.Decimal `json:"daily_remaining"`
	MonthlyLimit     decimal.Decimal `json:"monthly_limit"`
	MonthlyUsed      decimal.Decimal `json:"monthly_used"`
	MonthlyRemaining decimal.Decimal `json:"monthly_remaining"`
	MinWithdrawal    decimal.Decimal `json:"min_withdrawal"`
	MaxWithdrawal    decimal.Decimal `json:"max_withdrawal"`
	NetworkFee       decimal.Decimal `json:"network_fee"`
	MaintenanceMode  bool            `json:"maintenance_mode"`
}
