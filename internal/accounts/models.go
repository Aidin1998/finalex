// Ultra-high concurrency database models for the Accounts module
// This file defines GORM models with partitioning, composite indexes, and optimistic concurrency
package accounts

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Account represents a user's account for a specific currency with partitioning support
// Partitioned by user_id for horizontal scaling
type Account struct {
	ID        uuid.UUID       `json:"id" gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	UserID    uuid.UUID       `json:"user_id" gorm:"type:uuid;index:idx_account_user_currency,unique;index:idx_account_user_partition;not null"`
	Currency  string          `json:"currency" gorm:"type:varchar(10);index:idx_account_user_currency,unique;index:idx_account_currency;not null"`
	Balance   decimal.Decimal `json:"balance" gorm:"type:decimal(36,18);default:0;not null"`
	Available decimal.Decimal `json:"available" gorm:"type:decimal(36,18);default:0;not null"`
	Locked    decimal.Decimal `json:"locked" gorm:"type:decimal(36,18);default:0;not null"`
	Version   int64           `json:"version" gorm:"default:1;not null"` // Optimistic concurrency control
	CreatedAt time.Time       `json:"created_at" gorm:"index:idx_account_created"`
	UpdatedAt time.Time       `json:"updated_at" gorm:"index:idx_account_updated"`

	// Additional fields for enhanced functionality
	AccountType   string    `json:"account_type" gorm:"type:varchar(20);default:'spot';not null"` // spot, margin, futures
	Status        string    `json:"status" gorm:"type:varchar(20);default:'active';not null"`     // active, suspended, closed
	LastBalanceAt time.Time `json:"last_balance_at" gorm:"index:idx_account_last_balance"`

	// Audit fields
	CreatedBy uuid.UUID `json:"created_by" gorm:"type:uuid"`
	UpdatedBy uuid.UUID `json:"updated_by" gorm:"type:uuid"`
}

// TableName returns the table name for partitioning
func (Account) TableName() string {
	return "accounts"
}

// Reservation represents fund reservations with partitioning by user_id and currency
type Reservation struct {
	ID          uuid.UUID       `json:"id" gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	UserID      uuid.UUID       `json:"user_id" gorm:"type:uuid;index:idx_reservation_user_currency;index:idx_reservation_user_partition;not null"`
	Currency    string          `json:"currency" gorm:"type:varchar(10);index:idx_reservation_user_currency;index:idx_reservation_currency;not null"`
	Amount      decimal.Decimal `json:"amount" gorm:"type:decimal(36,18);not null"`
	Type        string          `json:"type" gorm:"type:varchar(20);not null"` // order, withdrawal, transfer, fee
	ReferenceID string          `json:"reference_id" gorm:"type:varchar(100);index:idx_reservation_reference"`
	Status      string          `json:"status" gorm:"type:varchar(20);default:'active';index:idx_reservation_status"` // active, released, expired
	ExpiresAt   *time.Time      `json:"expires_at" gorm:"index:idx_reservation_expires"`
	Version     int64           `json:"version" gorm:"default:1;not null"` // Optimistic concurrency control
	CreatedAt   time.Time       `json:"created_at" gorm:"index:idx_reservation_created"`
	UpdatedAt   time.Time       `json:"updated_at"`
	ReleasedAt  *time.Time      `json:"released_at"`

	// Additional fields
	Metadata string `json:"metadata" gorm:"type:jsonb"` // JSON metadata for additional information
	Priority int    `json:"priority" gorm:"default:0"`  // Priority for processing
}

// TableName returns the table name for partitioning
func (Reservation) TableName() string {
	return "reservations"
}

// LedgerTransaction represents a ledger entry with partitioning by user_id and time
type LedgerTransaction struct {
	ID             uuid.UUID       `json:"id" gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	UserID         uuid.UUID       `json:"user_id" gorm:"type:uuid;index:idx_ledger_user_time;index:idx_ledger_user_partition;not null"`
	AccountID      uuid.UUID       `json:"account_id" gorm:"type:uuid;index:idx_ledger_account;not null"`
	Currency       string          `json:"currency" gorm:"type:varchar(10);index:idx_ledger_currency;not null"`
	Type           string          `json:"type" gorm:"type:varchar(30);index:idx_ledger_type;not null"` // deposit, withdrawal, trade, transfer, fee, etc.
	Amount         decimal.Decimal `json:"amount" gorm:"type:decimal(36,18);not null"`
	BalanceBefore  decimal.Decimal `json:"balance_before" gorm:"type:decimal(36,18);not null"`
	BalanceAfter   decimal.Decimal `json:"balance_after" gorm:"type:decimal(36,18);not null"`
	Status         string          `json:"status" gorm:"type:varchar(20);default:'pending';index:idx_ledger_status"` // pending, confirmed, failed, cancelled
	Reference      string          `json:"reference" gorm:"type:varchar(255);index:idx_ledger_reference"`
	Description    string          `json:"description" gorm:"type:text"`
	IdempotencyKey string          `json:"idempotency_key" gorm:"type:varchar(255);uniqueIndex:idx_ledger_idempotency"`
	TraceID        string          `json:"trace_id" gorm:"type:varchar(100);index:idx_ledger_trace"`
	Version        int64           `json:"version" gorm:"default:1;not null"` // Optimistic concurrency control
	CreatedAt      time.Time       `json:"created_at" gorm:"index:idx_ledger_user_time;index:idx_ledger_created"`
	UpdatedAt      time.Time       `json:"updated_at"`
	ConfirmedAt    *time.Time      `json:"confirmed_at" gorm:"index:idx_ledger_confirmed"`

	// Additional fields for enhanced functionality
	ParentID    *uuid.UUID      `json:"parent_id" gorm:"type:uuid;index:idx_ledger_parent"`             // For linked transactions
	OrderID     *uuid.UUID      `json:"order_id" gorm:"type:uuid;index:idx_ledger_order"`               // Associated order
	TradeID     *uuid.UUID      `json:"trade_id" gorm:"type:uuid;index:idx_ledger_trade"`               // Associated trade
	Metadata    string          `json:"metadata" gorm:"type:jsonb"`                                     // JSON metadata
	Fee         decimal.Decimal `json:"fee" gorm:"type:decimal(36,18);default:0"`                       // Transaction fee
	FeeCurrency string          `json:"fee_currency" gorm:"type:varchar(10)"`                           // Fee currency
	ExternalID  string          `json:"external_id" gorm:"type:varchar(255);index:idx_ledger_external"` // External system ID
	ProcessedBy string          `json:"processed_by" gorm:"type:varchar(100)"`                          // Processing system/service
}

// TableName returns the table name for partitioning
func (LedgerTransaction) TableName() string {
	return "ledger_transactions"
}

// BalanceSnapshot represents daily balance snapshots for reconciliation
type BalanceSnapshot struct {
	ID        uuid.UUID       `json:"id" gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	UserID    uuid.UUID       `json:"user_id" gorm:"type:uuid;index:idx_snapshot_user_date;not null"`
	Currency  string          `json:"currency" gorm:"type:varchar(10);index:idx_snapshot_currency;not null"`
	Balance   decimal.Decimal `json:"balance" gorm:"type:decimal(36,18);not null"`
	Available decimal.Decimal `json:"available" gorm:"type:decimal(36,18);not null"`
	Locked    decimal.Decimal `json:"locked" gorm:"type:decimal(36,18);not null"`
	Date      time.Time       `json:"date" gorm:"type:date;index:idx_snapshot_user_date;index:idx_snapshot_date;not null"`
	CreatedAt time.Time       `json:"created_at"`

	// Reconciliation fields
	ReconciliationStatus string     `json:"reconciliation_status" gorm:"type:varchar(20);default:'pending'"` // pending, validated, failed
	ReconciliationError  string     `json:"reconciliation_error" gorm:"type:text"`
	ValidatedAt          *time.Time `json:"validated_at"`
}

// TableName returns the table name for partitioning
func (BalanceSnapshot) TableName() string {
	return "balance_snapshots"
}

// TransactionJournal represents double-entry bookkeeping journal entries
type TransactionJournal struct {
	ID              uuid.UUID       `json:"id" gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	TransactionID   uuid.UUID       `json:"transaction_id" gorm:"type:uuid;index:idx_journal_transaction;not null"`
	AccountID       uuid.UUID       `json:"account_id" gorm:"type:uuid;index:idx_journal_account;not null"`
	DebitAccountID  *uuid.UUID      `json:"debit_account_id" gorm:"type:uuid;index:idx_journal_debit"`
	CreditAccountID *uuid.UUID      `json:"credit_account_id" gorm:"type:uuid;index:idx_journal_credit"`
	Amount          decimal.Decimal `json:"amount" gorm:"type:decimal(36,18);not null"`
	Currency        string          `json:"currency" gorm:"type:varchar(10);not null"`
	EntryType       string          `json:"entry_type" gorm:"type:varchar(10);not null"` // debit, credit
	CreatedAt       time.Time       `json:"created_at" gorm:"index:idx_journal_created"`

	// Additional fields
	Description string `json:"description" gorm:"type:text"`
	Reference   string `json:"reference" gorm:"type:varchar(255)"`
}

// TableName returns the table name for partitioning
func (TransactionJournal) TableName() string {
	return "transaction_journals"
}

// CacheEntry represents Redis cache structure
type CacheEntry struct {
	Key       string          `json:"key"`
	UserID    uuid.UUID       `json:"user_id"`
	Currency  string          `json:"currency"`
	Balance   decimal.Decimal `json:"balance"`
	Available decimal.Decimal `json:"available"`
	Locked    decimal.Decimal `json:"locked"`
	Version   int64           `json:"version"`
	TTL       int64           `json:"ttl"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
	Tier      string          `json:"tier"` // hot, warm, cold
}

// AuditLog represents audit trail for all account operations
type AuditLog struct {
	ID         uuid.UUID `json:"id" gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	UserID     uuid.UUID `json:"user_id" gorm:"type:uuid;index:idx_audit_user;not null"`
	Action     string    `json:"action" gorm:"type:varchar(50);index:idx_audit_action;not null"`
	Resource   string    `json:"resource" gorm:"type:varchar(50);index:idx_audit_resource;not null"`
	ResourceID string    `json:"resource_id" gorm:"type:varchar(100);index:idx_audit_resource_id"`
	OldValues  string    `json:"old_values" gorm:"type:jsonb"`
	NewValues  string    `json:"new_values" gorm:"type:jsonb"`
	IPAddress  string    `json:"ip_address" gorm:"type:inet"`
	UserAgent  string    `json:"user_agent" gorm:"type:text"`
	TraceID    string    `json:"trace_id" gorm:"type:varchar(100);index:idx_audit_trace"`
	CreatedAt  time.Time `json:"created_at" gorm:"index:idx_audit_created"`

	// Additional fields
	SessionID string `json:"session_id" gorm:"type:varchar(100);index:idx_audit_session"`
	Success   bool   `json:"success" gorm:"default:true;index:idx_audit_success"`
	ErrorMsg  string `json:"error_msg" gorm:"type:text"`
}

// TableName returns the table name for partitioning
func (AuditLog) TableName() string {
	return "audit_logs"
}

// Performance monitoring and metrics structures

// OperationMetrics represents performance metrics for database operations
type OperationMetrics struct {
	Operation     string        `json:"operation"`
	Count         int64         `json:"count"`
	TotalDuration time.Duration `json:"total_duration"`
	AvgDuration   time.Duration `json:"avg_duration"`
	MaxDuration   time.Duration `json:"max_duration"`
	MinDuration   time.Duration `json:"min_duration"`
	ErrorCount    int64         `json:"error_count"`
	SuccessRate   float64       `json:"success_rate"`
	Timestamp     time.Time     `json:"timestamp"`
}

// CacheMetrics represents Redis cache performance metrics
type CacheMetrics struct {
	Operation   string        `json:"operation"`
	HitCount    int64         `json:"hit_count"`
	MissCount   int64         `json:"miss_count"`
	HitRate     float64       `json:"hit_rate"`
	AvgDuration time.Duration `json:"avg_duration"`
	ErrorCount  int64         `json:"error_count"`
	Timestamp   time.Time     `json:"timestamp"`
}

// ConnectionPoolMetrics represents database connection pool metrics
type ConnectionPoolMetrics struct {
	ActiveConnections int           `json:"active_connections"`
	IdleConnections   int           `json:"idle_connections"`
	TotalConnections  int           `json:"total_connections"`
	MaxConnections    int           `json:"max_connections"`
	WaitingQueries    int           `json:"waiting_queries"`
	AvgWaitTime       time.Duration `json:"avg_wait_time"`
	Timestamp         time.Time     `json:"timestamp"`
}

// Constants for model validation and defaults
const (
	// Account types
	AccountTypeSpot    = "spot"
	AccountTypeMargin  = "margin"
	AccountTypeFutures = "futures"

	// Account statuses
	AccountStatusActive    = "active"
	AccountStatusSuspended = "suspended"
	AccountStatusClosed    = "closed"

	// Reservation types
	ReservationTypeOrder      = "order"
	ReservationTypeWithdrawal = "withdrawal"
	ReservationTypeTransfer   = "transfer"
	ReservationTypeFee        = "fee"

	// Reservation statuses
	ReservationStatusActive   = "active"
	ReservationStatusReleased = "released"
	ReservationStatusExpired  = "expired"

	// Transaction types
	TransactionTypeDeposit    = "deposit"
	TransactionTypeWithdrawal = "withdrawal"
	TransactionTypeTrade      = "trade"
	TransactionTypeTransfer   = "transfer"
	TransactionTypeFee        = "fee"
	TransactionTypeCorrection = "correction"

	// Transaction statuses
	TransactionStatusPending   = "pending"
	TransactionStatusConfirmed = "confirmed"
	TransactionStatusFailed    = "failed"
	TransactionStatusCancelled = "cancelled"

	// Entry types for double-entry bookkeeping
	EntryTypeDebit  = "debit"
	EntryTypeCredit = "credit"

	// Cache tiers
	CacheTierHot  = "hot"
	CacheTierWarm = "warm"
	CacheTierCold = "cold"
)
