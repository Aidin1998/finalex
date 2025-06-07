// Package interfaces provides types and interfaces for the wallet module
package interfaces

import (
	"fmt"
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
	TxStatusProcessing TxStatus = "processing"
	TxStatusConfirming TxStatus = "confirming"
	TxStatusConfirmed  TxStatus = "confirmed"
	TxStatusCompleted  TxStatus = "completed"
	TxStatusFailed     TxStatus = "failed"
	TxStatusCancelled  TxStatus = "cancelled"
	TxStatusRejected   TxStatus = "rejected"
)

// Status constants for compatibility
const (
	StatusPending    = TxStatusPending
	StatusProcessing = TxStatusProcessing
	StatusCompleted  = TxStatusCompleted
	StatusFailed     = TxStatusFailed
	StatusRejected   = TxStatusRejected
	StatusCancelled  = TxStatusCancelled
)

// WithdrawalPriority represents withdrawal priority levels
type WithdrawalPriority string

const (
	PriorityLow    WithdrawalPriority = "low"
	PriorityMedium WithdrawalPriority = "medium"
	PriorityHigh   WithdrawalPriority = "high"
	PriorityUrgent WithdrawalPriority = "urgent"
)

// Custom JSONB type for metadata
type JSONB map[string]interface{}

// Request/Response types

// DepositRequest represents a deposit request
type DepositRequest struct {
	UserID          uuid.UUID              `json:"user_id" validate:"required"`
	Asset           string                 `json:"asset" validate:"required"`
	Amount          decimal.Decimal        `json:"amount" validate:"required,gt=0"`
	ToAddress       string                 `json:"to_address" validate:"required"`
	Network         string                 `json:"network" validate:"required"`
	GenerateAddress bool                   `json:"generate_address"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
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
	ID            uuid.UUID       `json:"id"`
	TransactionID uuid.UUID       `json:"transaction_id"`
	Status        TxStatus        `json:"status"`
	Address       *DepositAddress `json:"address,omitempty"`
	Amount        decimal.Decimal `json:"amount"`
	Asset         string          `json:"asset"`
	Network       string          `json:"network"`
	QRCode        string          `json:"qr_code,omitempty"`
	MinDeposit    decimal.Decimal `json:"min_deposit"`
	RequiredConf  int             `json:"required_confirmations"`
	EstimatedTime time.Duration   `json:"estimated_time,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
}

// WithdrawalResponse represents a withdrawal response
type WithdrawalResponse struct {
	ID               uuid.UUID       `json:"id"`
	TransactionID    uuid.UUID       `json:"transaction_id"`
	Status           TxStatus        `json:"status"`
	Amount           decimal.Decimal `json:"amount"`
	Fee              decimal.Decimal `json:"fee"`
	Asset            string          `json:"asset"`
	ToAddress        string          `json:"to_address"`
	Network          string          `json:"network"`
	EstimatedFee     decimal.Decimal `json:"estimated_fee"`
	ProcessingETA    time.Duration   `json:"processing_eta"`
	EstimatedTime    time.Duration   `json:"estimated_time,omitempty"`
	RequiresApproval bool            `json:"requires_approval"`
	CreatedAt        time.Time       `json:"created_at"`
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

// Event types for the wallet system
type DepositInitiatedEvent struct {
	ID            uuid.UUID       `json:"id"`
	UserID        uuid.UUID       `json:"user_id"`
	Asset         string          `json:"asset"`
	Amount        decimal.Decimal `json:"amount"`
	ToAddress     string          `json:"to_address"`
	TransactionID uuid.UUID       `json:"transaction_id"`
	Timestamp     time.Time       `json:"timestamp"`
}

type DepositConfirmedEvent struct {
	ID            uuid.UUID       `json:"id"`
	UserID        uuid.UUID       `json:"user_id"`
	Asset         string          `json:"asset"`
	Amount        decimal.Decimal `json:"amount"`
	TransactionID uuid.UUID       `json:"transaction_id"`
	TxHash        string          `json:"tx_hash"`
	Confirmations int             `json:"confirmations"`
	Timestamp     time.Time       `json:"timestamp"`
}

type DepositCompletedEvent struct {
	ID            uuid.UUID       `json:"id"`
	UserID        uuid.UUID       `json:"user_id"`
	Asset         string          `json:"asset"`
	Amount        decimal.Decimal `json:"amount"`
	TransactionID uuid.UUID       `json:"transaction_id"`
	TxHash        string          `json:"tx_hash"`
	Timestamp     time.Time       `json:"timestamp"`
}

type WithdrawalInitiatedEvent struct {
	ID            uuid.UUID       `json:"id"`
	UserID        uuid.UUID       `json:"user_id"`
	Asset         string          `json:"asset"`
	Amount        decimal.Decimal `json:"amount"`
	ToAddress     string          `json:"to_address"`
	TransactionID uuid.UUID       `json:"transaction_id"`
	Timestamp     time.Time       `json:"timestamp"`
}

type WithdrawalProcessingEvent struct {
	ID            uuid.UUID       `json:"id"`
	UserID        uuid.UUID       `json:"user_id"`
	Asset         string          `json:"asset"`
	Amount        decimal.Decimal `json:"amount"`
	TransactionID uuid.UUID       `json:"transaction_id"`
	FireblocksID  string          `json:"fireblocks_id"`
	Status        string          `json:"status"`
	Timestamp     time.Time       `json:"timestamp"`
}

type WithdrawalCompletedEvent struct {
	ID            uuid.UUID       `json:"id"`
	UserID        uuid.UUID       `json:"user_id"`
	Asset         string          `json:"asset"`
	Amount        decimal.Decimal `json:"amount"`
	TransactionID uuid.UUID       `json:"transaction_id"`
	TxHash        string          `json:"tx_hash"`
	ToAddress     string          `json:"to_address"`
	Fee           decimal.Decimal `json:"fee"`
	Timestamp     time.Time       `json:"timestamp"`
}

type WithdrawalCancelledEvent struct {
	ID            uuid.UUID       `json:"id"`
	UserID        uuid.UUID       `json:"user_id"`
	Asset         string          `json:"asset"`
	Amount        decimal.Decimal `json:"amount"`
	TransactionID uuid.UUID       `json:"transaction_id"`
	Reason        string          `json:"reason"`
	Timestamp     time.Time       `json:"timestamp"`
}

type WithdrawalFailedEvent struct {
	ID            uuid.UUID       `json:"id"`
	UserID        uuid.UUID       `json:"user_id"`
	Asset         string          `json:"asset"`
	Amount        decimal.Decimal `json:"amount"`
	TransactionID uuid.UUID       `json:"transaction_id"`
	Reason        string          `json:"reason"`
	Error         string          `json:"error"`
	Timestamp     time.Time       `json:"timestamp"`
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

// Address validation

// AddressValidationRequest represents address validation request
type AddressValidationRequest struct {
	Address string `json:"address" validate:"required"`
	Asset   string `json:"asset" validate:"required"`
	Network string `json:"network" validate:"required"`
	Tag     string `json:"tag,omitempty"`
}

// AddressValidationResult represents address validation result
type AddressValidationResult struct {
	Valid   bool   `json:"valid"`
	IsValid bool   `json:"is_valid"`
	Reason  string `json:"reason,omitempty"`
	Format  string `json:"format,omitempty"`
	Network string `json:"network,omitempty"`
}

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

// Additional Fireblocks types for interfaces

// FireblocksVault represents a Fireblocks vault account
type FireblocksVault struct {
	ID            string                  `json:"id"`
	Name          string                  `json:"name"`
	Type          string                  `json:"type"`
	Assets        []*FireblocksVaultAsset `json:"assets"`
	HiddenOnUI    bool                    `json:"hiddenOnUI"`
	CustomerRefID string                  `json:"customerRefId"`
	AutoFuel      bool                    `json:"autoFuel"`
	CreatedAt     int64                   `json:"createdAt"`
}

// FireblocksVaultAsset represents an asset in a vault
type FireblocksVaultAsset struct {
	ID                 string `json:"id"`
	Total              string `json:"total"`
	Available          string `json:"available"`
	Pending            string `json:"pending"`
	Frozen             string `json:"frozen"`
	LockedAmount       string `json:"lockedAmount"`
	BlockedAmount      string `json:"blockedAmount"`
	TotalStakedCPU     string `json:"totalStakedCPU"`
	TotalStakedNetwork string `json:"totalStakedNetwork"`
}

// FireblocksBalance represents balance response from Fireblocks
type FireblocksBalance struct {
	ID                 string `json:"id"`
	Total              string `json:"total"`
	Available          string `json:"available"`
	Pending            string `json:"pending"`
	Frozen             string `json:"frozen"`
	LockedAmount       string `json:"lockedAmount"`
	BlockedAmount      string `json:"blockedAmount"`
	TotalStakedCPU     string `json:"totalStakedCPU"`
	TotalStakedNetwork string `json:"totalStakedNetwork"`
}

// FireblocksAddress represents a Fireblocks deposit address
type FireblocksAddress struct {
	AssetID           string `json:"assetId"`
	Address           string `json:"address"`
	Description       string `json:"description"`
	Tag               string `json:"tag"`
	Type              string `json:"type"`
	CustomerRefID     string `json:"customerRefId"`
	AddressFormat     string `json:"addressFormat"`
	LegacyAddress     string `json:"legacyAddress"`
	EnterpriseAddress string `json:"enterpriseAddress"`
	Bip44AddressIndex int    `json:"bip44AddressIndex"`
}

// FireblocksAddressRequest represents a request to generate an address
type FireblocksAddressRequest struct {
	AssetID       string `json:"assetId"`
	VaultID       string `json:"vaultId"`
	Description   string `json:"description,omitempty"`
	CustomerRefID string `json:"customerRefId,omitempty"`
}

// FireblocksTransaction represents a Fireblocks transaction
type FireblocksTransaction struct {
	ID                 string                     `json:"id"`
	AssetID            string                     `json:"assetId"`
	Source             FireblocksTransferPeerPath `json:"source"`
	Destination        FireblocksDestinationPath  `json:"destination"`
	Amount             string                     `json:"amount"`
	NetAmount          string                     `json:"netAmount"`
	AmountUSD          string                     `json:"amountUSD"`
	NetAmountUSD       string                     `json:"netAmountUSD"`
	Fee                string                     `json:"fee"`
	NetworkFee         string                     `json:"networkFee"`
	CreatedAt          int64                      `json:"createdAt"`
	LastUpdated        int64                      `json:"lastUpdated"`
	Status             string                     `json:"status"`
	TxHash             string                     `json:"txHash"`
	SubStatus          string                     `json:"subStatus"`
	SignedBy           []string                   `json:"signedBy"`
	CreatedBy          string                     `json:"createdBy"`
	RejectedBy         string                     `json:"rejectedBy"`
	DestinationAddress string                     `json:"destinationAddress"`
	DestinationTag     string                     `json:"destinationTag"`
	AddressType        string                     `json:"addressType"`
	Note               string                     `json:"note"`
	ExchangeTxID       string                     `json:"exchangeTxId"`
	RequestedAmount    string                     `json:"requestedAmount"`
	FeeCurrency        string                     `json:"feeCurrency"`
	Operation          string                     `json:"operation"`
	CustomerRefID      string                     `json:"customerRefId"`
	NumOfConfirmations int                        `json:"numOfConfirmations"`
	NetworkRecords     []FireblocksNetworkRecord  `json:"networkRecords"`
	ReplacedTxHash     string                     `json:"replacedTxHash"`
	ExternalTxID       string                     `json:"externalTxId"`
	BlockInfo          *FireblocksBlockInfo       `json:"blockInfo"`
	SignedMessages     []FireblocksSignedMessage  `json:"signedMessages"`
	Index              int                        `json:"index"`
}

// FireblocksTransferPeerPath represents source/destination path
type FireblocksTransferPeerPath struct {
	Type    string `json:"type"`
	ID      string `json:"id"`
	Name    string `json:"name"`
	SubType string `json:"subType"`
}

// FireblocksDestinationPath represents destination path
type FireblocksDestinationPath struct {
	Type           string                    `json:"type"`
	ID             string                    `json:"id"`
	Name           string                    `json:"name"`
	SubType        string                    `json:"subType"`
	DisplayName    string                    `json:"displayName"`
	OneTimeAddress *FireblocksOneTimeAddress `json:"oneTimeAddress"`
}

// FireblocksOneTimeAddress represents one-time address
type FireblocksOneTimeAddress struct {
	Address string `json:"address"`
	Tag     string `json:"tag"`
}

// FireblocksNetworkRecord represents network record
type FireblocksNetworkRecord struct {
	Source             *FireblocksTransferPeerPath `json:"source"`
	Destination        *FireblocksTransferPeerPath `json:"destination"`
	TxHash             string                      `json:"txHash"`
	NetworkFee         string                      `json:"networkFee"`
	AssetID            string                      `json:"assetId"`
	NetAmount          string                      `json:"netAmount"`
	Status             string                      `json:"status"`
	Type               string                      `json:"type"`
	DestinationAddress string                      `json:"destinationAddress"`
	SourceAddress      string                      `json:"sourceAddress"`
	AmountUSD          string                      `json:"amountUSD"`
	Index              int                         `json:"index"`
}

// FireblocksBlockInfo represents block information
type FireblocksBlockInfo struct {
	BlockHeight string `json:"blockHeight"`
	BlockHash   string `json:"blockHash"`
}

// FireblocksSignedMessage represents signed message
type FireblocksSignedMessage struct {
	Content        string                      `json:"content"`
	Algorithm      string                      `json:"algorithm"`
	DerivationPath []int                       `json:"derivationPath"`
	Signature      *FireblocksMessageSignature `json:"signature"`
	PublicKey      string                      `json:"publicKey"`
}

// FireblocksMessageSignature represents message signature
type FireblocksMessageSignature struct {
	FullSig string `json:"fullSig"`
	R       string `json:"r"`
	S       string `json:"s"`
	V       int    `json:"v"`
}

// FireblocksTransactionRequest represents transaction creation request
type FireblocksTransactionRequest struct {
	AssetID         string                      `json:"assetId"`
	Amount          decimal.Decimal             `json:"amount"`
	Source          FireblocksTransactionSource `json:"source"`
	Destination     FireblocksTransactionDest   `json:"destination"`
	Note            string                      `json:"note,omitempty"`
	Fee             string                      `json:"fee,omitempty"`
	GasPrice        string                      `json:"gasPrice,omitempty"`
	GasLimit        string                      `json:"gasLimit,omitempty"`
	CustomerRefID   string                      `json:"customerRefId,omitempty"`
	ReplaceTxByHash string                      `json:"replaceTxByHash,omitempty"`
}

// FireblocksTransactionSource represents transaction source
type FireblocksTransactionSource struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

// FireblocksTransactionDest represents transaction destination
type FireblocksTransactionDest struct {
	Type           string                    `json:"type"`
	ID             string                    `json:"id,omitempty"`
	OneTimeAddress *FireblocksOneTimeAddress `json:"oneTimeAddress,omitempty"`
}

// FireblocksTransactionFilters represents transaction query filters
type FireblocksTransactionFilters struct {
	Status  string `json:"status,omitempty"`
	AssetID string `json:"assetId,omitempty"`
	Limit   int    `json:"limit,omitempty"`
	Before  string `json:"before,omitempty"`
	After   string `json:"after,omitempty"`
	OrderBy string `json:"orderBy,omitempty"`
	Sort    string `json:"sort,omitempty"`
}

// FireblocksAddressValidationRequest represents address validation request
type FireblocksAddressValidationRequest struct {
	AssetID string `json:"assetId"`
	Address string `json:"address"`
}

// FireblocksAddressValidationResult represents address validation result
type FireblocksAddressValidationResult struct {
	IsValid       bool   `json:"isValid"`
	IsActive      bool   `json:"isActive"`
	RequiresTag   bool   `json:"requiresTag"`
	AddressFormat string `json:"addressFormat"`
}

// FireblocksAsset represents supported asset
type FireblocksAsset struct {
	ID                  string                    `json:"id"`
	Name                string                    `json:"name"`
	Type                string                    `json:"type"`
	ContractAddress     string                    `json:"contractAddress"`
	NativeAsset         string                    `json:"nativeAsset"`
	Decimals            int                       `json:"decimals"`
	BlockchainID        string                    `json:"blockchainId"`
	BlockchainSymbol    string                    `json:"blockchainSymbol"`
	IsBaseAsset         bool                      `json:"isBaseAsset"`
	NetworkProtocol     string                    `json:"networkProtocol"`
	AddressRegex        string                    `json:"addressRegex"`
	TestnetAddressRegex string                    `json:"testnetAddressRegex"`
	Memo                string                    `json:"memo"`
	DepositMethods      []FireblocksDepositMethod `json:"depositMethods"`
}

// FireblocksDepositMethod represents deposit method
type FireblocksDepositMethod struct {
	Type      string `json:"type"`
	MinAmount string `json:"minAmount"`
	MaxAmount string `json:"maxAmount"`
}

// FireblocksNetworkFee represents network fee estimation
type FireblocksNetworkFee struct {
	Low    FireblocksFeeLevel `json:"low"`
	Medium FireblocksFeeLevel `json:"medium"`
	High   FireblocksFeeLevel `json:"high"`
}

// FireblocksFeeLevel represents fee level
type FireblocksFeeLevel struct {
	NetworkFee  string `json:"networkFee"`
	ServiceFee  string `json:"serviceFee"`
	GasPrice    string `json:"gasPrice"`
	GasLimit    string `json:"gasLimit"`
	BaseFee     string `json:"baseFee"`
	PriorityFee string `json:"priorityFee"`
	FeePerByte  string `json:"feePerByte"`
}

// FireblocksError represents Fireblocks API error
type FireblocksError struct {
	Message string `json:"message"`
	Code    int    `json:"code"`
}

func (e *FireblocksError) Error() string {
	return fmt.Sprintf("Fireblocks API error %d: %s", e.Code, e.Message)
}

// FireblocksWebhookData represents webhook data
type FireblocksWebhookData struct {
	Type      string         `json:"type"`
	TenantID  string         `json:"tenantId"`
	Timestamp int64          `json:"timestamp"`
	Data      FireblocksData `json:"data"`
}

// FireblocksSource represents webhook source
type FireblocksSource struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	SubType string `json:"subType"`
}

// Additional missing types for interfaces

// AddressStatistics represents address usage statistics
type AddressStatistics struct {
	TotalAddresses  int `json:"total_addresses"`
	ActiveAddresses int `json:"active_addresses"`
	UnusedAddresses int `json:"unused_addresses"`
}

// AddressGenerationRequest represents address generation request
type AddressGenerationRequest struct {
	UserID  uuid.UUID `json:"user_id"`
	Asset   string    `json:"asset"`
	Network string    `json:"network"`
	Count   int       `json:"count"`
}

// WithdrawalLimits represents withdrawal limits for a user
type WithdrawalLimits struct {
	DailyLimit       decimal.Decimal `json:"daily_limit"`
	MonthlyLimit     decimal.Decimal `json:"monthly_limit"`
	SingleTxLimit    decimal.Decimal `json:"single_tx_limit"`
	UsedDaily        decimal.Decimal `json:"used_daily"`
	UsedMonthly      decimal.Decimal `json:"used_monthly"`
	RemainingDaily   decimal.Decimal `json:"remaining_daily"`
	RemainingMonthly decimal.Decimal `json:"remaining_monthly"`
}

// WalletEvent represents a generic wallet event for publishing
type WalletEvent struct {
	ID        uuid.UUID              `json:"id"`
	Type      string                 `json:"type"`
	EventType string                 `json:"event_type"`
	UserID    uuid.UUID              `json:"user_id"`
	Asset     string                 `json:"asset,omitempty"`
	Amount    *decimal.Decimal       `json:"amount,omitempty"`
	Direction Direction              `json:"direction,omitempty"`
	TxID      *uuid.UUID             `json:"tx_id,omitempty"`
	Status    string                 `json:"status,omitempty"`
	Message   string                 `json:"message,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
}

// AddressParams represents parameters for address generation
type AddressParams struct {
	Tag         string `json:"tag,omitempty"`
	Description string `json:"description,omitempty"`
	CustomerRef string `json:"customer_ref,omitempty"`
}

// Statistics types
type TransactionStats struct {
	UserID             uuid.UUID              `json:"user_id"`
	From               time.Time              `json:"from"`
	To                 time.Time              `json:"to"`
	TotalDeposits      int64                  `json:"total_deposits"`
	TotalWithdrawals   int64                  `json:"total_withdrawals"`
	TotalTransactions  int64                  `json:"total_transactions"`
	DepositVolume      decimal.Decimal        `json:"deposit_volume"`
	WithdrawalVolume   decimal.Decimal        `json:"withdrawal_volume"`
	PendingDeposits    int64                  `json:"pending_deposits"`
	PendingWithdrawals int64                  `json:"pending_withdrawals"`
	ByAsset            map[string]*AssetStats `json:"by_asset"`
	ByStatus           map[TxStatus]int64     `json:"by_status"`
}

type AssetStats struct {
	Asset            string          `json:"asset"`
	TotalBalance     decimal.Decimal `json:"total_balance"`
	AvailableBalance decimal.Decimal `json:"available_balance"`
	LockedBalance    decimal.Decimal `json:"locked_balance"`
	TotalDeposits    decimal.Decimal `json:"total_deposits"`
	TotalWithdrawals decimal.Decimal `json:"total_withdrawals"`
	DepositCount     int64           `json:"deposit_count"`
	DepositAmount    decimal.Decimal `json:"deposit_amount"`
	WithdrawalCount  int64           `json:"withdrawal_count"`
	WithdrawalAmount decimal.Decimal `json:"withdrawal_amount"`
}

// Error types
var (
	ErrInsufficientBalance = fmt.Errorf("insufficient balance")
	ErrInvalidAmount       = fmt.Errorf("invalid amount")
	ErrInvalidAddress      = fmt.Errorf("invalid address")
	ErrTransactionNotFound = fmt.Errorf("transaction not found")
	ErrUserNotFound        = fmt.Errorf("user not found")
	ErrAssetNotSupported   = fmt.Errorf("asset not supported")
	ErrNetworkNotSupported = fmt.Errorf("network not supported")
	ErrComplianceRejected  = fmt.Errorf("transaction rejected by compliance")
	ErrRateLimited         = fmt.Errorf("rate limit exceeded")
	ErrServiceUnavailable  = fmt.Errorf("service temporarily unavailable")
)
