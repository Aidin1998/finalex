// Package interfaces provides types and interfaces for the wallet module
package interfaces

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// WalletTransaction represents a wallet transaction
type WalletTransaction struct {
	ID              uuid.UUID       `json:"id" gorm:"primaryKey;type:uuid"`
	UserID          uuid.UUID       `json:"user_id" gorm:"type:uuid;index"`
	Asset           string          `json:"asset" gorm:"size:10;index"`
	Amount          decimal.Decimal `json:"amount" gorm:"type:decimal(20,8)"`
	Direction       Direction       `json:"direction" gorm:"size:20"`
	Status          TxStatus        `json:"status" gorm:"size:20;index"`
	FireblocksID    string          `json:"fireblocks_id" gorm:"size:100;index"`
	TxHash          string          `json:"tx_hash" gorm:"size:100;index"`
	FromAddress     string          `json:"from_address" gorm:"size:100"`
	ToAddress       string          `json:"to_address" gorm:"size:100"`
	Network         string          `json:"network" gorm:"size:50"`
	Confirmations   int             `json:"confirmations" gorm:"default:0"`
	RequiredConf    int             `json:"required_confirmations" gorm:"default:1"`
	Locked          bool            `json:"locked" gorm:"default:false"`
	ComplianceCheck string          `json:"compliance_check" gorm:"size:50"`
	ErrorMsg        string          `json:"error_msg" gorm:"type:text"`
	Metadata        JSONB           `json:"metadata" gorm:"type:jsonb"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

// FundLock represents a fund lock for pending operations
type FundLock struct {
	ID        uuid.UUID       `json:"id" gorm:"primaryKey;type:uuid"`
	UserID    uuid.UUID       `json:"user_id" gorm:"type:uuid;index"`
	Asset     string          `json:"asset" gorm:"size:10;index"`
	Amount    decimal.Decimal `json:"amount" gorm:"type:decimal(20,8)"`
	Reason    string          `json:"reason" gorm:"size:100"`
	TxRef     string          `json:"tx_ref" gorm:"size:100;index"`
	Expires   *time.Time      `json:"expires,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}

// WalletBalance represents user wallet balance
type WalletBalance struct {
	ID        uuid.UUID       `json:"id" gorm:"primaryKey;type:uuid"`
	UserID    uuid.UUID       `json:"user_id" gorm:"type:uuid;index"`
	Asset     string          `json:"asset" gorm:"size:10;index"`
	Available decimal.Decimal `json:"available" gorm:"type:decimal(20,8)"`
	Locked    decimal.Decimal `json:"locked" gorm:"type:decimal(20,8)"`
	Total     decimal.Decimal `json:"total" gorm:"type:decimal(20,8)"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// DepositAddress represents a deposit address
type DepositAddress struct {
	ID           uuid.UUID `json:"id" gorm:"primaryKey;type:uuid"`
	UserID       uuid.UUID `json:"user_id" gorm:"type:uuid;index"`
	Asset        string    `json:"asset" gorm:"size:10;index"`
	Network      string    `json:"network" gorm:"size:50"`
	Address      string    `json:"address" gorm:"size:100;uniqueIndex"`
	Tag          string    `json:"tag" gorm:"size:50"`
	IsActive     bool      `json:"is_active" gorm:"default:true"`
	FireblocksID string    `json:"fireblocks_id" gorm:"size:100"`
	CreatedAt    time.Time `json:"created_at"`
}

// Direction represents transaction direction
type Direction string

const (
	DirectionDeposit    Direction = "deposit"
	DirectionWithdrawal Direction = "withdrawal"
)

// TxStatus represents transaction status
type TxStatus string

const (
	TxStatusInitiated  TxStatus = "initiated"
	TxStatusPending    TxStatus = "pending"
	TxStatusConfirming TxStatus = "confirming"
	TxStatusConfirmed  TxStatus = "confirmed"
	TxStatusCompleted  TxStatus = "completed"
	TxStatusRejected   TxStatus = "rejected"
	TxStatusFailed     TxStatus = "failed"
	TxStatusCancelled  TxStatus = "cancelled"
)

// WithdrawalPriority represents withdrawal priority levels
type WithdrawalPriority string

const (
	PriorityLow    WithdrawalPriority = "low"
	PriorityMedium WithdrawalPriority = "medium"
	PriorityHigh   WithdrawalPriority = "high"
	PriorityUrgent WithdrawalPriority = "urgent"
)

// Request/Response types

// DepositRequest represents a deposit request
type DepositRequest struct {
	UserID          uuid.UUID `json:"user_id" validate:"required"`
	Asset           string    `json:"asset" validate:"required"`
	Network         string    `json:"network" validate:"required"`
	GenerateAddress bool      `json:"generate_address"`
}

// WithdrawalRequest represents a withdrawal request
type WithdrawalRequest struct {
	UserID         uuid.UUID          `json:"user_id" validate:"required"`
	Asset          string             `json:"asset" validate:"required"`
	Amount         decimal.Decimal    `json:"amount" validate:"required"`
	ToAddress      string             `json:"to_address" validate:"required"`
	Network        string             `json:"network" validate:"required"`
	Tag            string             `json:"tag,omitempty"`
	TwoFactorToken string             `json:"two_factor_token" validate:"required"`
	Priority       WithdrawalPriority `json:"priority"`
	Note           string             `json:"note,omitempty"`
}

// DepositResponse represents a deposit response
type DepositResponse struct {
	TransactionID uuid.UUID       `json:"transaction_id"`
	Address       *DepositAddress `json:"address,omitempty"`
	QRCode        string          `json:"qr_code,omitempty"`
	Network       string          `json:"network"`
	MinDeposit    decimal.Decimal `json:"min_deposit"`
	RequiredConf  int             `json:"required_confirmations"`
}

// WithdrawalResponse represents a withdrawal response
type WithdrawalResponse struct {
	TransactionID uuid.UUID       `json:"transaction_id"`
	Status        TxStatus        `json:"status"`
	EstimatedFee  decimal.Decimal `json:"estimated_fee"`
	ProcessingETA time.Duration   `json:"processing_eta"`
}

// TransactionStatus represents transaction status details
type TransactionStatus struct {
	ID            uuid.UUID       `json:"id"`
	Status        TxStatus        `json:"status"`
	Confirmations int             `json:"confirmations"`
	Required      int             `json:"required_confirmations"`
	TxHash        string          `json:"tx_hash,omitempty"`
	NetworkFee    decimal.Decimal `json:"network_fee"`
	ProcessedAt   *time.Time      `json:"processed_at,omitempty"`
	CompletedAt   *time.Time      `json:"completed_at,omitempty"`
	ErrorMsg      string          `json:"error_msg,omitempty"`
}

// BalanceResponse represents balance query response
type BalanceResponse struct {
	UserID    uuid.UUID               `json:"user_id"`
	Balances  map[string]AssetBalance `json:"balances"`
	Timestamp time.Time               `json:"timestamp"`
}

// AssetBalance represents balance for a specific asset
type AssetBalance struct {
	Asset     string          `json:"asset"`
	Available decimal.Decimal `json:"available"`
	Locked    decimal.Decimal `json:"locked"`
	Total     decimal.Decimal `json:"total"`
}

// Fireblocks integration types

// FireblocksWebhook represents Fireblocks webhook payload
type FireblocksWebhook struct {
	Type      string         `json:"type"`
	TenantID  string         `json:"tenantId"`
	Timestamp int64          `json:"timestamp"`
	Data      FireblocksData `json:"data"`
}

// FireblocksData represents webhook data
type FireblocksData struct {
	ID                 string                 `json:"id"`
	Status             string                 `json:"status"`
	SubStatus          string                 `json:"subStatus"`
	TxHash             string                 `json:"txHash"`
	Operation          string                 `json:"operation"`
	Source             FireblocksWalletInfo   `json:"source"`
	Destination        FireblocksWalletInfo   `json:"destination"`
	Amount             string                 `json:"amount"`
	NetAmount          string                 `json:"netAmount"`
	Fee                string                 `json:"fee"`
	AssetID            string                 `json:"assetId"`
	NetworkRecord      FireblocksNetworkInfo  `json:"networkRecord"`
	CreatedAt          int64                  `json:"createdAt"`
	LastUpdated        int64                  `json:"lastUpdated"`
	NumOfConfirmations int                    `json:"numOfConfirmations"`
	ExtraParameters    map[string]interface{} `json:"extraParameters"`
}

// FireblocksWalletInfo represents wallet info in webhooks
type FireblocksWalletInfo struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	SubType string `json:"subType"`
}

// FireblocksNetworkInfo represents network info
type FireblocksNetworkInfo struct {
	Status                string `json:"status"`
	TxHash                string `json:"txHash"`
	NetworkFee            string `json:"networkFee"`
	RequiredConfirmations int    `json:"requiredConfirmations"`
	NumOfConfirmations    int    `json:"numOfConfirmations"`
}

// Event types for integration

// WalletEvent represents a wallet event
type WalletEvent struct {
	Type      string                 `json:"type"`
	UserID    uuid.UUID              `json:"user_id"`
	TxID      uuid.UUID              `json:"transaction_id"`
	Asset     string                 `json:"asset"`
	Amount    decimal.Decimal        `json:"amount"`
	Direction Direction              `json:"direction"`
	Status    TxStatus               `json:"status"`
	Metadata  map[string]interface{} `json:"metadata"`
	Timestamp time.Time              `json:"timestamp"`
}

// Custom JSONB type for metadata
type JSONB map[string]interface{}

// Compliance integration

// ComplianceCheckRequest represents compliance check request
type ComplianceCheckRequest struct {
	UserID    uuid.UUID              `json:"user_id"`
	Asset     string                 `json:"asset"`
	Amount    decimal.Decimal        `json:"amount"`
	Direction Direction              `json:"direction"`
	Address   string                 `json:"address,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// ComplianceCheckResult represents compliance check result
type ComplianceCheckResult struct {
	Approved bool     `json:"approved"`
	Reason   string   `json:"reason,omitempty"`
	Flags    []string `json:"flags,omitempty"`
	Score    int      `json:"score"`
}

// Address validation

// AddressValidationRequest represents address validation request
type AddressValidationRequest struct {
	Address string `json:"address" validate:"required"`
	Asset   string `json:"asset" validate:"required"`
	Network string `json:"network" validate:"required"`
}

// AddressValidationResult represents address validation result
type AddressValidationResult struct {
	Valid   bool   `json:"valid"`
	Reason  string `json:"reason,omitempty"`
	Format  string `json:"format,omitempty"`
	Network string `json:"network,omitempty"`
}
