// Package contracts defines unified interface contracts for module integration
package contracts

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// FiatServiceContract defines the standardized interface for fiat module
type FiatServiceContract interface {
	// Deposit Management
	InitiateDeposit(ctx context.Context, req *FiatDepositRequest) (*FiatDepositResponse, error)
	ProcessDepositReceipt(ctx context.Context, req *FiatReceiptRequest) (*FiatReceiptResponse, error)
	GetDepositStatus(ctx context.Context, depositID uuid.UUID) (*FiatDepositStatus, error)
	GetDepositReceipt(ctx context.Context, receiptID uuid.UUID) (*FiatDepositReceipt, error)

	// Withdrawal Management
	InitiateWithdrawal(ctx context.Context, req *FiatWithdrawalRequest) (*FiatWithdrawalResponse, error)
	ProcessWithdrawal(ctx context.Context, withdrawalID uuid.UUID) error
	CancelWithdrawal(ctx context.Context, withdrawalID uuid.UUID, reason string) error
	GetWithdrawalStatus(ctx context.Context, withdrawalID uuid.UUID) (*FiatWithdrawalStatus, error)

	// Payment Methods & Bank Accounts
	AddBankAccount(ctx context.Context, req *AddBankAccountRequest) (*BankAccount, error)
	ValidateBankAccount(ctx context.Context, bankAccountID uuid.UUID) (*BankAccountValidation, error)
	GetBankAccounts(ctx context.Context, userID uuid.UUID) ([]*BankAccount, error)
	RemoveBankAccount(ctx context.Context, bankAccountID uuid.UUID) error

	// Transaction History
	GetUserDeposits(ctx context.Context, userID uuid.UUID, filter *FiatTransactionFilter) ([]*FiatDepositReceipt, int64, error)
	GetUserWithdrawals(ctx context.Context, userID uuid.UUID, filter *FiatTransactionFilter) ([]*FiatWithdrawal, int64, error)
	GetFiatTransaction(ctx context.Context, transactionID uuid.UUID) (*FiatTransaction, error)

	// Compliance & KYC
	ValidateKYCForFiat(ctx context.Context, userID uuid.UUID, amount decimal.Decimal, currency string) (*FiatKYCValidation, error)
	CheckFiatLimits(ctx context.Context, userID uuid.UUID, transactionType string, amount decimal.Decimal, currency string) (*FiatLimitCheck, error)
	ReportSuspiciousActivity(ctx context.Context, req *SuspiciousActivityRequest) error

	// Provider Management
	GetSupportedProviders(ctx context.Context) ([]*FiatProvider, error)
	GetProviderStatus(ctx context.Context, providerID string) (*ProviderStatus, error)
	ValidateProviderSignature(ctx context.Context, req *SignatureValidationRequest) error

	// Rates & Fees
	GetFiatRates(ctx context.Context, baseCurrency, targetCurrency string) (*FiatExchangeRate, error)
	CalculateFees(ctx context.Context, req *FeeCalculationRequest) (*FiatFeeCalculation, error)

	// Service Health & Monitoring
	HealthCheck(ctx context.Context) (*HealthStatus, error)
	GetMetrics(ctx context.Context) (*FiatServiceMetrics, error)
}

// FiatDepositRequest represents fiat deposit initiation request
type FiatDepositRequest struct {
	UserID     uuid.UUID              `json:"user_id"`
	Currency   string                 `json:"currency"`
	Amount     decimal.Decimal        `json:"amount"`
	Method     string                 `json:"method"` // bank_transfer, credit_card, debit_card
	ProviderID string                 `json:"provider_id"`
	ReturnURL  string                 `json:"return_url,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// FiatDepositResponse represents fiat deposit initiation response
type FiatDepositResponse struct {
	DepositID    uuid.UUID       `json:"deposit_id"`
	Currency     string          `json:"currency"`
	Amount       decimal.Decimal `json:"amount"`
	Method       string          `json:"method"`
	PaymentURL   string          `json:"payment_url,omitempty"`
	Instructions string          `json:"instructions,omitempty"`
	ExpiresAt    time.Time       `json:"expires_at"`
	Status       string          `json:"status"`
	CreatedAt    time.Time       `json:"created_at"`
}

// FiatReceiptRequest represents external provider receipt payload
type FiatReceiptRequest struct {
	UserID      uuid.UUID       `json:"user_id"`
	ProviderID  string          `json:"provider_id"`
	ReferenceID string          `json:"reference_id"`
	Amount      decimal.Decimal `json:"amount"`
	Currency    string          `json:"currency"`
	Timestamp   time.Time       `json:"timestamp"`
	ProviderSig string          `json:"provider_signature"`
	TraceID     string          `json:"trace_id,omitempty"`
}

// FiatReceiptResponse represents response to receipt submission
type FiatReceiptResponse struct {
	ReceiptID     uuid.UUID  `json:"receipt_id"`
	Status        string     `json:"status"` // pending, processed, completed, failed, duplicate
	Message       string     `json:"message"`
	ProcessedAt   time.Time  `json:"processed_at"`
	TransactionID *uuid.UUID `json:"transaction_id,omitempty"`
	TraceID       string     `json:"trace_id,omitempty"`
}

// FiatDepositStatus represents deposit status information
type FiatDepositStatus struct {
	DepositID     uuid.UUID       `json:"deposit_id"`
	UserID        uuid.UUID       `json:"user_id"`
	Currency      string          `json:"currency"`
	Amount        decimal.Decimal `json:"amount"`
	Status        string          `json:"status"`
	PaymentMethod string          `json:"payment_method"`
	ProviderID    string          `json:"provider_id"`
	ExternalRef   string          `json:"external_ref,omitempty"`
	FailureReason string          `json:"failure_reason,omitempty"`
	EstimatedTime string          `json:"estimated_time,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
	CompletedAt   *time.Time      `json:"completed_at,omitempty"`
}

// FiatDepositReceipt represents a processed deposit receipt
type FiatDepositReceipt struct {
	ID            uuid.UUID       `json:"id"`
	UserID        uuid.UUID       `json:"user_id"`
	ProviderID    string          `json:"provider_id"`
	ReferenceID   string          `json:"reference_id"`
	Amount        decimal.Decimal `json:"amount"`
	Currency      string          `json:"currency"`
	ReceiptHash   string          `json:"receipt_hash"`
	Status        string          `json:"status"`
	ProcessedAt   *time.Time      `json:"processed_at,omitempty"`
	CompletedAt   *time.Time      `json:"completed_at,omitempty"`
	TransactionID *uuid.UUID      `json:"transaction_id,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

// FiatWithdrawalRequest represents fiat withdrawal initiation request
type FiatWithdrawalRequest struct {
	UserID         uuid.UUID              `json:"user_id"`
	Currency       string                 `json:"currency"`
	Amount         decimal.Decimal        `json:"amount"`
	BankAccountID  uuid.UUID              `json:"bank_account_id"`
	TwoFactorToken string                 `json:"two_factor_token"`
	Reference      string                 `json:"reference,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

// FiatWithdrawalResponse represents fiat withdrawal initiation response
type FiatWithdrawalResponse struct {
	WithdrawalID  uuid.UUID       `json:"withdrawal_id"`
	Currency      string          `json:"currency"`
	Amount        decimal.Decimal `json:"amount"`
	Fee           decimal.Decimal `json:"fee"`
	NetAmount     decimal.Decimal `json:"net_amount"`
	BankAccount   *BankAccount    `json:"bank_account"`
	Status        string          `json:"status"`
	EstimatedTime string          `json:"estimated_time"`
	CreatedAt     time.Time       `json:"created_at"`
}

// FiatWithdrawalStatus represents withdrawal status information
type FiatWithdrawalStatus struct {
	WithdrawalID  uuid.UUID       `json:"withdrawal_id"`
	UserID        uuid.UUID       `json:"user_id"`
	Currency      string          `json:"currency"`
	Amount        decimal.Decimal `json:"amount"`
	Fee           decimal.Decimal `json:"fee"`
	NetAmount     decimal.Decimal `json:"net_amount"`
	Status        string          `json:"status"`
	BankAccountID uuid.UUID       `json:"bank_account_id"`
	ExternalRef   string          `json:"external_ref,omitempty"`
	FailureReason string          `json:"failure_reason,omitempty"`
	EstimatedTime string          `json:"estimated_time,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
	CompletedAt   *time.Time      `json:"completed_at,omitempty"`
}

// FiatWithdrawal represents a withdrawal record
type FiatWithdrawal struct {
	ID            uuid.UUID       `json:"id"`
	UserID        uuid.UUID       `json:"user_id"`
	Currency      string          `json:"currency"`
	Amount        decimal.Decimal `json:"amount"`
	Fee           decimal.Decimal `json:"fee"`
	NetAmount     decimal.Decimal `json:"net_amount"`
	Status        string          `json:"status"`
	BankAccountID uuid.UUID       `json:"bank_account_id"`
	Reference     string          `json:"reference,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
	CompletedAt   *time.Time      `json:"completed_at,omitempty"`
}

// AddBankAccountRequest represents bank account addition request
type AddBankAccountRequest struct {
	UserID        uuid.UUID `json:"user_id"`
	AccountName   string    `json:"account_name"`
	AccountNumber string    `json:"account_number"`
	BankName      string    `json:"bank_name"`
	RoutingNumber string    `json:"routing_number,omitempty"`
	IBAN          string    `json:"iban,omitempty"`
	Currency      string    `json:"currency"`
	Country       string    `json:"country"`
}

// BankAccount represents a user's bank account
type BankAccount struct {
	ID            uuid.UUID `json:"id"`
	UserID        uuid.UUID `json:"user_id"`
	AccountName   string    `json:"account_name"`
	AccountNumber string    `json:"account_number_masked"`
	BankName      string    `json:"bank_name"`
	RoutingNumber string    `json:"routing_number,omitempty"`
	IBAN          string    `json:"iban,omitempty"`
	Currency      string    `json:"currency"`
	Country       string    `json:"country"`
	Status        string    `json:"status"` // pending, verified, failed
	IsDefault     bool      `json:"is_default"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// BankAccountValidation represents bank account validation result
type BankAccountValidation struct {
	BankAccountID uuid.UUID `json:"bank_account_id"`
	Status        string    `json:"status"` // valid, invalid, pending
	ErrorCode     string    `json:"error_code,omitempty"`
	ErrorMessage  string    `json:"error_message,omitempty"`
	ValidatedAt   time.Time `json:"validated_at"`
}

// FiatTransactionFilter represents filtering criteria for fiat transactions
type FiatTransactionFilter struct {
	Currency  string          `json:"currency,omitempty"`
	Status    string          `json:"status,omitempty"`
	StartTime time.Time       `json:"start_time,omitempty"`
	EndTime   time.Time       `json:"end_time,omitempty"`
	MinAmount decimal.Decimal `json:"min_amount,omitempty"`
	MaxAmount decimal.Decimal `json:"max_amount,omitempty"`
	Page      int             `json:"page"`
	PageSize  int             `json:"page_size"`
}

// FiatTransaction represents a unified fiat transaction
type FiatTransaction struct {
	ID            uuid.UUID       `json:"id"`
	UserID        uuid.UUID       `json:"user_id"`
	Type          string          `json:"type"` // deposit, withdrawal
	Currency      string          `json:"currency"`
	Amount        decimal.Decimal `json:"amount"`
	Fee           decimal.Decimal `json:"fee,omitempty"`
	Status        string          `json:"status"`
	Reference     string          `json:"reference,omitempty"`
	ProviderID    string          `json:"provider_id,omitempty"`
	BankAccountID *uuid.UUID      `json:"bank_account_id,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
	CompletedAt   *time.Time      `json:"completed_at,omitempty"`
}

// FiatKYCValidation represents KYC validation for fiat operations
type FiatKYCValidation struct {
	UserID        uuid.UUID `json:"user_id"`
	Valid         bool      `json:"valid"`
	KYCLevel      int       `json:"kyc_level"`
	RequiredLevel int       `json:"required_level"`
	ErrorMessage  string    `json:"error_message,omitempty"`
	ValidatedAt   time.Time `json:"validated_at"`
}

// FiatLimitCheck represents fiat transaction limit validation
type FiatLimitCheck struct {
	UserID          uuid.UUID       `json:"user_id"`
	TransactionType string          `json:"transaction_type"`
	Currency        string          `json:"currency"`
	RequestedAmount decimal.Decimal `json:"requested_amount"`
	DailyLimit      decimal.Decimal `json:"daily_limit"`
	MonthlyLimit    decimal.Decimal `json:"monthly_limit"`
	DailyUsed       decimal.Decimal `json:"daily_used"`
	MonthlyUsed     decimal.Decimal `json:"monthly_used"`
	Allowed         bool            `json:"allowed"`
	ErrorMessage    string          `json:"error_message,omitempty"`
	CheckedAt       time.Time       `json:"checked_at"`
}

// SuspiciousActivityRequest represents suspicious activity report
type SuspiciousActivityRequest struct {
	UserID       uuid.UUID              `json:"user_id"`
	ActivityType string                 `json:"activity_type"`
	Description  string                 `json:"description"`
	Amount       decimal.Decimal        `json:"amount,omitempty"`
	Currency     string                 `json:"currency,omitempty"`
	Evidence     map[string]interface{} `json:"evidence,omitempty"`
}

// FiatProvider represents a fiat payment provider
type FiatProvider struct {
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	Type           string          `json:"type"` // bank, card, digital_wallet
	Currencies     []string        `json:"supported_currencies"`
	Countries      []string        `json:"supported_countries"`
	Status         string          `json:"status"` // active, maintenance, disabled
	Features       []string        `json:"features"`
	MinAmount      decimal.Decimal `json:"min_amount"`
	MaxAmount      decimal.Decimal `json:"max_amount"`
	ProcessingTime string          `json:"processing_time"`
}

// ProviderStatus represents provider operational status
type ProviderStatus struct {
	ProviderID  string    `json:"provider_id"`
	Status      string    `json:"status"` // healthy, degraded, down
	Latency     int64     `json:"latency_ms"`
	ErrorRate   float64   `json:"error_rate"`
	LastChecked time.Time `json:"last_checked"`
	Message     string    `json:"message,omitempty"`
}

// SignatureValidationRequest represents provider signature validation
type SignatureValidationRequest struct {
	ProviderID string    `json:"provider_id"`
	Payload    string    `json:"payload"`
	Signature  string    `json:"signature"`
	Timestamp  time.Time `json:"timestamp"`
}

// FiatExchangeRate represents fiat currency exchange rates
type FiatExchangeRate struct {
	BaseCurrency   string          `json:"base_currency"`
	TargetCurrency string          `json:"target_currency"`
	Rate           decimal.Decimal `json:"rate"`
	Source         string          `json:"source"`
	Timestamp      time.Time       `json:"timestamp"`
	ValidUntil     time.Time       `json:"valid_until"`
}

// FeeCalculationRequest represents fee calculation request
type FeeCalculationRequest struct {
	UserID          uuid.UUID       `json:"user_id"`
	TransactionType string          `json:"transaction_type"` // deposit, withdrawal
	Currency        string          `json:"currency"`
	Amount          decimal.Decimal `json:"amount"`
	PaymentMethod   string          `json:"payment_method"`
	ProviderID      string          `json:"provider_id,omitempty"`
}

// FiatFeeCalculation represents calculated fees for fiat operations
type FiatFeeCalculation struct {
	BaseFee      decimal.Decimal        `json:"base_fee"`
	ProviderFee  decimal.Decimal        `json:"provider_fee"`
	NetworkFee   decimal.Decimal        `json:"network_fee,omitempty"`
	TotalFee     decimal.Decimal        `json:"total_fee"`
	Currency     string                 `json:"currency"`
	FeeStructure map[string]interface{} `json:"fee_structure"`
	CalculatedAt time.Time              `json:"calculated_at"`
}

// FiatServiceMetrics represents fiat service operational metrics
type FiatServiceMetrics struct {
	TotalDeposits         int64           `json:"total_deposits"`
	TotalWithdrawals      int64           `json:"total_withdrawals"`
	DepositVolume         decimal.Decimal `json:"deposit_volume"`
	WithdrawalVolume      decimal.Decimal `json:"withdrawal_volume"`
	AverageProcessingTime time.Duration   `json:"average_processing_time"`
	SuccessRate           float64         `json:"success_rate"`
	ActiveProviders       int             `json:"active_providers"`
	PendingTransactions   int             `json:"pending_transactions"`
	LastUpdated           time.Time       `json:"last_updated"`
}
