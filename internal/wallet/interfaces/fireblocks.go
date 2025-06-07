// Package interfaces provides Fireblocks integration interfaces
package interfaces

import (
	"context"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

// FireblocksClient defines the interface for Fireblocks API integration
type FireblocksClient interface {
	// Vault account management
	CreateVaultAccount(ctx context.Context, name string, hiddenOnUI bool) (*VaultAccount, error)
	GetVaultAccount(ctx context.Context, vaultAccountID string) (*VaultAccount, error)
	GetVaultAccounts(ctx context.Context) ([]*VaultAccount, error)

	// Asset wallet management
	CreateAssetWallet(ctx context.Context, vaultAccountID, assetID string) (*AssetWallet, error)
	GetAssetWallet(ctx context.Context, vaultAccountID, assetID string) (*AssetWallet, error)
	GetAssetWallets(ctx context.Context, vaultAccountID string) ([]*AssetWallet, error)

	// Address generation
	GenerateDepositAddress(ctx context.Context, vaultAccountID, assetID string, params *AddressParams) (*DepositAddressResponse, error)
	GetDepositAddresses(ctx context.Context, vaultAccountID, assetID string) ([]*DepositAddressResponse, error)
	ValidateAddress(ctx context.Context, assetID, address string) (*AddressValidation, error)

	// Transaction management
	CreateTransaction(ctx context.Context, req *TransactionRequest) (*TransactionResponse, error)
	GetTransaction(ctx context.Context, txID string) (*TransactionResponse, error)
	GetTransactions(ctx context.Context, params *TransactionFilter) ([]*TransactionResponse, error)
	CancelTransaction(ctx context.Context, txID string) (*TransactionResponse, error)
	EstimateFee(ctx context.Context, req *FeeEstimationRequest) (*FeeEstimation, error)

	// Webhook management
	SetWebhookURL(ctx context.Context, url string) error
	ResendWebhook(ctx context.Context, txID string) error

	// Balance queries
	GetVaultBalance(ctx context.Context, vaultAccountID string) ([]*VaultBalance, error)
	GetAssetBalance(ctx context.Context, vaultAccountID, assetID string) (*VaultBalance, error)

	// Network and asset info
	GetSupportedAssets(ctx context.Context) ([]*SupportedAsset, error)
	GetNetworkConnections(ctx context.Context) ([]*NetworkConnection, error)
	GetAssetNetworks(ctx context.Context, assetID string) ([]*AssetNetwork, error)

	// Security and compliance
	SetTransactionConfirmationThreshold(ctx context.Context, vaultAccountID, assetID string, threshold int) error
	FreezeTransaction(ctx context.Context, txID string) (*TransactionResponse, error)
	UnfreezeTransaction(ctx context.Context, txID string) (*TransactionResponse, error)
}

// WebhookHandler processes Fireblocks webhooks
type WebhookHandler interface {
	// Process incoming webhook
	ProcessWebhook(ctx context.Context, payload []byte, signature string) error

	// Validate webhook signature
	ValidateSignature(payload []byte, signature string) error

	// Handle specific webhook types
	HandleTransactionStatusChanged(ctx context.Context, data *FireblocksData) error
	HandleDepositDetected(ctx context.Context, data *FireblocksData) error
	HandleWithdrawalCompleted(ctx context.Context, data *FireblocksData) error
}

// Fireblocks API types

// VaultAccount represents a Fireblocks vault account
type VaultAccount struct {
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	HiddenOnUI    bool           `json:"hiddenOnUI"`
	Assets        []*AssetWallet `json:"assets"`
	CustomerRefID string         `json:"customerRefId"`
	AutoFuel      bool           `json:"autoFuel"`
}

// AssetWallet represents an asset wallet within a vault account
type AssetWallet struct {
	ID               string     `json:"id"`
	Balance          string     `json:"balance"`
	LockedAmount     string     `json:"lockedAmount"`
	Available        string     `json:"available"`
	Pending          string     `json:"pending"`
	Frozen           string     `json:"frozen"`
	Staked           string     `json:"staked"`
	BlockHeight      string     `json:"blockHeight"`
	BlockHash        string     `json:"blockHash"`
	ActivationTime   *time.Time `json:"activationTime"`
	Address          string     `json:"address"`
	Tag              string     `json:"tag"`
	ActivationTxHash string     `json:"activationTxHash"`
}

// AddressParams represents parameters for address generation
type AddressParams struct {
	Description   string `json:"description,omitempty"`
	CustomerRefID string `json:"customerRefId,omitempty"`
}

// DepositAddressResponse represents a deposit address response
type DepositAddressResponse struct {
	AssetID           string `json:"assetId"`
	Address           string `json:"address"`
	Tag               string `json:"tag"`
	Description       string `json:"description"`
	Type              string `json:"type"`
	CustomerRefID     string `json:"customerRefId"`
	AddressFormat     string `json:"addressFormat"`
	LegacyAddress     string `json:"legacyAddress"`
	Enterprise        string `json:"enterprise"`
	Bip44Change       int    `json:"bip44Change"`
	Bip44AddressIndex int    `json:"bip44AddressIndex"`
}

// AddressValidation represents address validation result
type AddressValidation struct {
	IsValid       bool   `json:"isValid"`
	IsActive      bool   `json:"isActive"`
	RequiresTag   bool   `json:"requiresTag"`
	AddressFormat string `json:"addressFormat"`
}

// TransactionRequest represents a transaction request
type TransactionRequest struct {
	AssetID         string                 `json:"assetId"`
	Operation       string                 `json:"operation"`
	Source          TransactionSource      `json:"source"`
	Destination     TransactionDestination `json:"destination"`
	Amount          string                 `json:"amount"`
	Fee             string                 `json:"fee,omitempty"`
	FeeLevel        string                 `json:"feeLevel,omitempty"`
	MaxFee          string                 `json:"maxFee,omitempty"`
	PriorityFee     string                 `json:"priorityFee,omitempty"`
	GasPrice        string                 `json:"gasPrice,omitempty"`
	GasLimit        string                 `json:"gasLimit,omitempty"`
	NetworkFee      string                 `json:"networkFee,omitempty"`
	CustomerRefID   string                 `json:"customerRefId,omitempty"`
	Note            string                 `json:"note,omitempty"`
	ExternalTxID    string                 `json:"externalTxId,omitempty"`
	ReplaceTxByHash string                 `json:"replaceTxByHash,omitempty"`
	ExtraParameters map[string]interface{} `json:"extraParameters,omitempty"`
}

// TransactionSource represents transaction source
type TransactionSource struct {
	Type    string `json:"type"`
	ID      string `json:"id,omitempty"`
	Name    string `json:"name,omitempty"`
	SubType string `json:"subType,omitempty"`
}

// TransactionDestination represents transaction destination
type TransactionDestination struct {
	Type           string          `json:"type"`
	ID             string          `json:"id,omitempty"`
	Name           string          `json:"name,omitempty"`
	SubType        string          `json:"subType,omitempty"`
	OneTimeAddress *OneTimeAddress `json:"oneTimeAddress,omitempty"`
}

// OneTimeAddress represents one-time address destination
type OneTimeAddress struct {
	Address string `json:"address"`
	Tag     string `json:"tag,omitempty"`
}

// TransactionResponse represents a transaction response
type TransactionResponse struct {
	ID                 string                 `json:"id"`
	Status             string                 `json:"status"`
	SubStatus          string                 `json:"subStatus"`
	TxHash             string                 `json:"txHash"`
	Operation          string                 `json:"operation"`
	Note               string                 `json:"note"`
	AssetID            string                 `json:"assetId"`
	Source             TransactionSource      `json:"source"`
	Destination        TransactionDestination `json:"destination"`
	Amount             string                 `json:"amount"`
	NetAmount          string                 `json:"netAmount"`
	AmountUSD          string                 `json:"amountUSD"`
	NetAmountUSD       string                 `json:"netAmountUSD"`
	Fee                string                 `json:"fee"`
	FeeUSD             string                 `json:"feeUSD"`
	NetworkFee         string                 `json:"networkFee"`
	CreatedAt          int64                  `json:"createdAt"`
	LastUpdated        int64                  `json:"lastUpdated"`
	CreatedBy          string                 `json:"createdBy"`
	SignedBy           []string               `json:"signedBy"`
	RejectedBy         string                 `json:"rejectedBy"`
	AddressType        string                 `json:"addressType"`
	NumOfConfirmations int                    `json:"numOfConfirmations"`
	NetworkRecord      NetworkRecord          `json:"networkRecord"`
	ReplacedTxHash     string                 `json:"replacedTxHash"`
	ExternalTxID       string                 `json:"externalTxId"`
	CustomerRefID      string                 `json:"customerRefId"`
	AmlScreeningResult AMLScreeningResult     `json:"amlScreeningResult"`
	ExtraParameters    map[string]interface{} `json:"extraParameters"`
}

// NetworkRecord represents network-specific transaction data
type NetworkRecord struct {
	Status                string `json:"status"`
	TxHash                string `json:"txHash"`
	NetworkFee            string `json:"networkFee"`
	RequiredConfirmations int    `json:"requiredConfirmations"`
	NumOfConfirmations    int    `json:"numOfConfirmations"`
	Type                  string `json:"type"`
}

// AMLScreeningResult represents AML screening result
type AMLScreeningResult struct {
	Provider     string `json:"provider"`
	Status       string `json:"status"`
	BypassReason string `json:"bypassReason"`
	Timestamp    int64  `json:"timestamp"`
}

// TransactionFilter represents transaction filter parameters
type TransactionFilter struct {
	Before     string `json:"before,omitempty"`
	After      string `json:"after,omitempty"`
	Status     string `json:"status,omitempty"`
	OrderBy    string `json:"orderBy,omitempty"`
	Sort       string `json:"sort,omitempty"`
	Limit      int    `json:"limit,omitempty"`
	SourceType string `json:"sourceType,omitempty"`
	SourceID   string `json:"sourceId,omitempty"`
	DestType   string `json:"destType,omitempty"`
	DestID     string `json:"destId,omitempty"`
	Assets     string `json:"assets,omitempty"`
	TxHash     string `json:"txHash,omitempty"`
}

// FeeEstimationRequest represents fee estimation request
type FeeEstimationRequest struct {
	AssetID     string                 `json:"assetId"`
	Operation   string                 `json:"operation"`
	Source      TransactionSource      `json:"source"`
	Destination TransactionDestination `json:"destination"`
	Amount      string                 `json:"amount,omitempty"`
}

// FeeEstimation represents fee estimation result
type FeeEstimation struct {
	Low struct {
		NetworkFee  string `json:"networkFee"`
		FeePerByte  string `json:"feePerByte"`
		GasPrice    string `json:"gasPrice"`
		GasLimit    string `json:"gasLimit"`
		BaseFee     string `json:"baseFee"`
		PriorityFee string `json:"priorityFee"`
	} `json:"low"`
	Medium struct {
		NetworkFee  string `json:"networkFee"`
		FeePerByte  string `json:"feePerByte"`
		GasPrice    string `json:"gasPrice"`
		GasLimit    string `json:"gasLimit"`
		BaseFee     string `json:"baseFee"`
		PriorityFee string `json:"priorityFee"`
	} `json:"medium"`
	High struct {
		NetworkFee  string `json:"networkFee"`
		FeePerByte  string `json:"feePerByte"`
		GasPrice    string `json:"gasPrice"`
		GasLimit    string `json:"gasLimit"`
		BaseFee     string `json:"baseFee"`
		PriorityFee string `json:"priorityFee"`
	} `json:"high"`
}

// VaultBalance represents vault balance
type VaultBalance struct {
	ID           string `json:"id"`
	Total        string `json:"total"`
	Balance      string `json:"balance"`
	LockedAmount string `json:"lockedAmount"`
	Available    string `json:"available"`
	Pending      string `json:"pending"`
	Frozen       string `json:"frozen"`
	Staked       string `json:"staked"`
}

// SupportedAsset represents a supported asset
type SupportedAsset struct {
	ID               string          `json:"id"`
	Name             string          `json:"name"`
	Type             string          `json:"type"`
	ContractAddress  string          `json:"contractAddress"`
	NativeAsset      string          `json:"nativeAsset"`
	Decimals         int             `json:"decimals"`
	Deprecated       bool            `json:"deprecated"`
	BlockchainSymbol string          `json:"blockchainSymbol"`
	TestnetSymbol    string          `json:"testnetSymbol"`
	Price            decimal.Decimal `json:"price"`
}

// NetworkConnection represents network connection info
type NetworkConnection struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	RouteType       string `json:"routeType"`
	IsDiscoverable  bool   `json:"isDiscoverable"`
	Network         string `json:"network"`
	LocalNetworkID  string `json:"localNetworkId"`
	RemoteNetworkID string `json:"remoteNetworkId"`
}

// AssetNetwork represents asset network info
type AssetNetwork struct {
	Network             string `json:"network"`
	IsEnabled           bool   `json:"isEnabled"`
	MaintenanceMode     bool   `json:"maintenanceMode"`
	MinDepositThreshold string `json:"minDepositThreshold"`
}

// Webhook types

// WebhookEvent represents webhook event types
type WebhookEvent string

const (
	WebhookTransactionStatusChanged  WebhookEvent = "TRANSACTION_STATUS_CHANGED"
	WebhookTransactionCreated        WebhookEvent = "TRANSACTION_CREATED"
	WebhookBlockNotification         WebhookEvent = "BLOCK_NOTIFICATION"
	WebhookDepositDetected           WebhookEvent = "EXTERNAL_TRANSACTION_CREATED"
	WebhookConfigurationChange       WebhookEvent = "CONFIGURATION_CHANGE"
	WebhookVaultAccountAdded         WebhookEvent = "VAULT_ACCOUNT_ADDED"
	WebhookVaultAccountAssetAdded    WebhookEvent = "VAULT_ACCOUNT_ASSET_ADDED"
	WebhookInternalWalletAssetAdded  WebhookEvent = "INTERNAL_WALLET_ASSET_ADDED"
	WebhookExternalWalletAssetAdded  WebhookEvent = "EXTERNAL_WALLET_ASSET_ADDED"
	WebhookExchangeAccountAssetAdded WebhookEvent = "EXCHANGE_ACCOUNT_ASSET_ADDED"
	WebhookFiatAccountAssetAdded     WebhookEvent = "FIAT_ACCOUNT_ASSET_ADDED"
	WebhookNetworkConnectionAdded    WebhookEvent = "NETWORK_CONNECTION_ADDED"
)

// Fireblocks specific status constants
const (
	FireblocksStatusSubmitted            = "SUBMITTED"
	FireblocksStatusQueued               = "QUEUED"
	FireblocksStatusPendingSignature     = "PENDING_SIGNATURE"
	FireblocksStatusPendingAuthorization = "PENDING_AUTHORIZATION"
	FireblocksStatusPending3rdParty      = "PENDING_3RD_PARTY"
	FireblocksStatusPendingApproval      = "PENDING_APPROVAL"
	FireblocksStatusPending              = "PENDING"
	FireblocksStatusBroadcasting         = "BROADCASTING"
	FireblocksStatusConfirming           = "CONFIRMING"
	FireblocksStatusCompleted            = "COMPLETED"
	FireblocksStatusPendingAmlScreening  = "PENDING_AML_SCREENING"
	FireblocksStatusPartiallyCompleted   = "PARTIALLY_COMPLETED"
	FireblocksStatusCancelling           = "CANCELLING"
	FireblocksStatusCancelled            = "CANCELLED"
	FireblocksStatusRejected             = "REJECTED"
	FireblocksStatusFailed               = "FAILED"
	FireblocksStatusTimeout              = "TIMEOUT"
	FireblocksStatusBlocked              = "BLOCKED"
)

// Fee levels
const (
	FeeLevelHigh   = "HIGH"
	FeeLevelMedium = "MEDIUM"
	FeeLevelLow    = "LOW"
)

// Source/Destination types
const (
	SourceTypeVaultAccount      = "VAULT_ACCOUNT"
	SourceTypeExchangeAccount   = "EXCHANGE_ACCOUNT"
	SourceTypeInternalWallet    = "INTERNAL_WALLET"
	SourceTypeExternalWallet    = "EXTERNAL_WALLET"
	SourceTypeFiatAccount       = "FIAT_ACCOUNT"
	SourceTypeNetworkConnection = "NETWORK_CONNECTION"
	SourceTypeCompoundFinance   = "COMPOUND_FINANCE"
	SourceTypeOneTimeAddress    = "ONE_TIME_ADDRESS"
)

// Error types for Fireblocks integration
type FireblocksError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
}

func (e *FireblocksError) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("Fireblocks API error %d: %s - %s", e.Code, e.Message, e.Detail)
	}
	return fmt.Sprintf("Fireblocks API error %d: %s", e.Code, e.Message)
}

// RateLimiter interface for API rate limiting
type RateLimiter interface {
	// Allow checks if request is allowed
	Allow() bool

	// Wait waits until next request is allowed
	Wait(ctx context.Context) error

	// Remaining returns remaining requests in current window
	Remaining() int

	// Reset returns time until rate limit resets
	Reset() time.Duration
}
