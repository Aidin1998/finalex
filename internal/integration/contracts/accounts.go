// Package contracts defines unified interface contracts for module integration
package contracts

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// AccountsServiceContract defines the standardized interface for accounts module
type AccountsServiceContract interface {
	// Balance Management
	GetBalance(ctx context.Context, userID uuid.UUID, currency string) (*Balance, error)
	GetAllBalances(ctx context.Context, userID uuid.UUID) ([]*Balance, error)

	// Atomic Operations
	ReserveBalance(ctx context.Context, req *ReserveBalanceRequest) (*ReservationResponse, error)
	ReleaseReservation(ctx context.Context, reservationID uuid.UUID) error
	TransferBalance(ctx context.Context, req *TransferRequest) (*TransferResponse, error)

	// Distributed Transactions
	BeginXATransaction(ctx context.Context, req *XATransactionRequest) (*XATransaction, error)
	CommitXATransaction(ctx context.Context, xaID uuid.UUID) error
	RollbackXATransaction(ctx context.Context, xaID uuid.UUID) error

	// Transaction History
	GetTransactionHistory(ctx context.Context, userID uuid.UUID, currency string, limit, offset int) ([]*Transaction, int64, error)
	GetTransaction(ctx context.Context, transactionID uuid.UUID) (*Transaction, error)

	// Account Management
	CreateAccount(ctx context.Context, userID uuid.UUID, currency string) (*Account, error)
	GetAccount(ctx context.Context, userID uuid.UUID, currency string) (*Account, error)
	LockAccount(ctx context.Context, userID uuid.UUID, currency string, reason string) error
	UnlockAccount(ctx context.Context, userID uuid.UUID, currency string) error

	// Reconciliation
	ReconcileBalances(ctx context.Context, req *ReconciliationRequest) (*ReconciliationResult, error)
	ValidateBalanceIntegrity(ctx context.Context, userID uuid.UUID) (*IntegrityReport, error)

	// Service Health
	HealthCheck(ctx context.Context) (*HealthStatus, error)
}

// Balance represents account balance information
type Balance struct {
	UserID    uuid.UUID       `json:"user_id"`
	Currency  string          `json:"currency"`
	Available decimal.Decimal `json:"available"`
	Reserved  decimal.Decimal `json:"reserved"`
	Total     decimal.Decimal `json:"total"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// ReserveBalanceRequest represents balance reservation request
type ReserveBalanceRequest struct {
	UserID          uuid.UUID              `json:"user_id"`
	Currency        string                 `json:"currency"`
	Amount          decimal.Decimal        `json:"amount"`
	ReservationType string                 `json:"reservation_type"`
	ReferenceID     string                 `json:"reference_id"`
	ExpiresAt       *time.Time             `json:"expires_at,omitempty"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
}

// ReservationResponse represents balance reservation response
type ReservationResponse struct {
	ReservationID uuid.UUID       `json:"reservation_id"`
	UserID        uuid.UUID       `json:"user_id"`
	Currency      string          `json:"currency"`
	Amount        decimal.Decimal `json:"amount"`
	ExpiresAt     time.Time       `json:"expires_at"`
	CreatedAt     time.Time       `json:"created_at"`
}

// TransferRequest represents balance transfer request
type TransferRequest struct {
	FromUserID  uuid.UUID              `json:"from_user_id"`
	ToUserID    uuid.UUID              `json:"to_user_id"`
	Currency    string                 `json:"currency"`
	Amount      decimal.Decimal        `json:"amount"`
	Type        string                 `json:"type"`
	ReferenceID string                 `json:"reference_id"`
	Description string                 `json:"description"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// TransferResponse represents balance transfer response
type TransferResponse struct {
	TransactionID uuid.UUID       `json:"transaction_id"`
	FromUserID    uuid.UUID       `json:"from_user_id"`
	ToUserID      uuid.UUID       `json:"to_user_id"`
	Currency      string          `json:"currency"`
	Amount        decimal.Decimal `json:"amount"`
	Status        string          `json:"status"`
	CreatedAt     time.Time       `json:"created_at"`
}

// XATransactionRequest represents distributed transaction request
type XATransactionRequest struct {
	XID        string                 `json:"xid"`
	Resources  []string               `json:"resources"`
	Operations []XAOperation          `json:"operations"`
	Timeout    time.Duration          `json:"timeout"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// XAOperation represents a single operation within XA transaction
type XAOperation struct {
	Type       string                 `json:"type"`
	UserID     uuid.UUID              `json:"user_id"`
	Currency   string                 `json:"currency"`
	Amount     decimal.Decimal        `json:"amount"`
	Parameters map[string]interface{} `json:"parameters,omitempty"`
}

// XATransaction represents active distributed transaction
type XATransaction struct {
	XID       string    `json:"xid"`
	Status    string    `json:"status"`
	Resources []string  `json:"resources"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Transaction represents account transaction
type Transaction struct {
	ID          uuid.UUID              `json:"id"`
	UserID      uuid.UUID              `json:"user_id"`
	Type        string                 `json:"type"`
	Currency    string                 `json:"currency"`
	Amount      decimal.Decimal        `json:"amount"`
	Balance     decimal.Decimal        `json:"balance_after"`
	Status      string                 `json:"status"`
	ReferenceID string                 `json:"reference_id"`
	Description string                 `json:"description"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	CompletedAt *time.Time             `json:"completed_at,omitempty"`
}

// Account represents user account
type Account struct {
	ID        uuid.UUID       `json:"id"`
	UserID    uuid.UUID       `json:"user_id"`
	Currency  string          `json:"currency"`
	Balance   decimal.Decimal `json:"balance"`
	Available decimal.Decimal `json:"available"`
	Reserved  decimal.Decimal `json:"reserved"`
	Status    string          `json:"status"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// ReconciliationRequest represents balance reconciliation request
type ReconciliationRequest struct {
	UserIDs    []uuid.UUID `json:"user_ids,omitempty"`
	Currencies []string    `json:"currencies,omitempty"`
	StartTime  time.Time   `json:"start_time"`
	EndTime    time.Time   `json:"end_time"`
	Force      bool        `json:"force"`
}

// ReconciliationResult represents reconciliation result
type ReconciliationResult struct {
	TotalAccounts    int                        `json:"total_accounts"`
	ReconciledCount  int                        `json:"reconciled_count"`
	DiscrepancyCount int                        `json:"discrepancy_count"`
	Discrepancies    []BalanceDiscrepancy       `json:"discrepancies,omitempty"`
	Summary          map[string]decimal.Decimal `json:"summary"`
	ProcessedAt      time.Time                  `json:"processed_at"`
}

// BalanceDiscrepancy represents balance discrepancy
type BalanceDiscrepancy struct {
	UserID          uuid.UUID       `json:"user_id"`
	Currency        string          `json:"currency"`
	ExpectedBalance decimal.Decimal `json:"expected_balance"`
	ActualBalance   decimal.Decimal `json:"actual_balance"`
	Difference      decimal.Decimal `json:"difference"`
	Severity        string          `json:"severity"`
}

// IntegrityReport represents balance integrity report
type IntegrityReport struct {
	UserID    uuid.UUID              `json:"user_id"`
	IsValid   bool                   `json:"is_valid"`
	Issues    []IntegrityIssue       `json:"issues,omitempty"`
	Metrics   map[string]interface{} `json:"metrics"`
	CheckedAt time.Time              `json:"checked_at"`
}

// IntegrityIssue represents balance integrity issue
type IntegrityIssue struct {
	Type        string          `json:"type"`
	Currency    string          `json:"currency"`
	Description string          `json:"description"`
	Impact      decimal.Decimal `json:"impact"`
	Severity    string          `json:"severity"`
}
