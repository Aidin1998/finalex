// Package hooks provides platform-wide integration hooks for the compliance module
package hooks

import (
	"context"
	"sync"
	"time"

	"github.com/Aidin1998/finalex/internal/compliance/interfaces"
	"github.com/Aidin1998/finalex/internal/compliance/risk"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// HookManager manages all compliance integration hooks across the platform
type HookManager struct {
	compliance   interfaces.ComplianceService
	audit        interfaces.AuditService
	monitoring   interfaces.MonitoringService
	manipulation interfaces.ManipulationService
	risk         risk.RiskService

	// Hook registries
	userAuthHooks     []UserAuthHook
	tradingHooks      []TradingHook
	fiatHooks         []FiatHook
	walletHooks       []WalletHook
	accountsHooks     []AccountsHook
	marketMakingHooks []MarketMakingHook

	// Configuration
	config *HookConfig
	mu     sync.RWMutex
}

// HookConfig configures hook behavior
type HookConfig struct {
	EnableRealTimeHooks bool          `json:"enable_realtime_hooks"`
	AsyncProcessing     bool          `json:"async_processing"`
	BatchSize           int           `json:"batch_size"`
	FlushInterval       time.Duration `json:"flush_interval"`
	TimeoutDuration     time.Duration `json:"timeout_duration"`
	RetryAttempts       int           `json:"retry_attempts"`
	EnableMetrics       bool          `json:"enable_metrics"`
}

// NewHookManager creates a new hook manager
func NewHookManager(
	compliance interfaces.ComplianceService,
	audit interfaces.AuditService,
	monitoring interfaces.MonitoringService,
	manipulation interfaces.ManipulationService,
	risk risk.RiskService,
	config *HookConfig,
) *HookManager {
	if config == nil {
		config = &HookConfig{
			EnableRealTimeHooks: true,
			AsyncProcessing:     true,
			BatchSize:           100,
			FlushInterval:       time.Second,
			TimeoutDuration:     30 * time.Second,
			RetryAttempts:       3,
			EnableMetrics:       true,
		}
	}

	return &HookManager{
		compliance:   compliance,
		audit:        audit,
		monitoring:   monitoring,
		manipulation: manipulation,
		risk:         risk,
		config:       config,
	}
}

// UserAuthHook defines hooks for user authentication events
type UserAuthHook interface {
	OnUserRegistration(ctx context.Context, event *UserRegistrationEvent) error
	OnUserLogin(ctx context.Context, event *UserLoginEvent) error
	OnUserLogout(ctx context.Context, event *UserLogoutEvent) error
	OnPasswordChange(ctx context.Context, event *PasswordChangeEvent) error
	OnEmailVerification(ctx context.Context, event *EmailVerificationEvent) error
	On2FAEnabled(ctx context.Context, event *TwoFAEvent) error
	OnAccountLocked(ctx context.Context, event *AccountLockEvent) error
}

// TradingHook defines hooks for trading events
type TradingHook interface {
	OnOrderPlaced(ctx context.Context, event *OrderPlacedEvent) error
	OnOrderExecuted(ctx context.Context, event *OrderExecutedEvent) error
	OnOrderCancelled(ctx context.Context, event *OrderCancelledEvent) error
	OnTradeExecuted(ctx context.Context, event *TradeExecutedEvent) error
	OnPositionUpdated(ctx context.Context, event *PositionUpdateEvent) error
	OnMarketDataReceived(ctx context.Context, event *MarketDataEvent) error
}

// FiatHook defines hooks for fiat currency events
type FiatHook interface {
	OnFiatDeposit(ctx context.Context, event *FiatDepositEvent) error
	OnFiatWithdrawal(ctx context.Context, event *FiatWithdrawalEvent) error
	OnBankAccountAdded(ctx context.Context, event *BankAccountEvent) error
	OnPaymentMethodUpdated(ctx context.Context, event *PaymentMethodEvent) error
	OnFiatTransactionFailed(ctx context.Context, event *FiatTransactionFailedEvent) error
	// Additional methods expected by manager.go
	OnFiatTransfer(ctx context.Context, event *FiatTransferEvent) error
}

// WalletHook defines hooks for wallet events
type WalletHook interface {
	OnCryptoDeposit(ctx context.Context, event *CryptoDepositEvent) error
	OnCryptoWithdrawal(ctx context.Context, event *CryptoWithdrawalEvent) error
	OnWalletCreated(ctx context.Context, event *WalletCreatedEvent) error
	OnWalletBalanceChanged(ctx context.Context, event *WalletBalanceEvent) error
	OnAddressGenerated(ctx context.Context, event *AddressGeneratedEvent) error
	// Additional methods expected by manager.go
	OnBalanceUpdate(ctx context.Context, event *WalletBalanceUpdateEvent) error
	OnStaking(ctx context.Context, event *WalletStakingEvent) error
}

// AccountsHook defines hooks for account management events
type AccountsHook interface {
	OnAccountCreated(ctx context.Context, event *AccountCreatedEvent) error
	OnAccountUpdated(ctx context.Context, event *AccountUpdatedEvent) error
	OnAccountSuspended(ctx context.Context, event *AccountSuspendedEvent) error
	OnKYCStatusChanged(ctx context.Context, event *KYCStatusEvent) error
	OnDocumentUploaded(ctx context.Context, event *DocumentUploadEvent) error
	OnProfileUpdated(ctx context.Context, event *ProfileUpdateEvent) error
	// Additional methods expected by manager.go
	OnAccountCreation(ctx context.Context, event *AccountCreationEvent) error
	OnAccountSuspension(ctx context.Context, event *AccountSuspensionEvent) error
	OnKYCStatusChange(ctx context.Context, event *AccountKYCEvent) error
	OnTierChange(ctx context.Context, event *AccountTierEvent) error
	OnDormancyStatusChange(ctx context.Context, event *AccountDormancyEvent) error
}

// MarketMakingHook defines hooks for market making events
type MarketMakingHook interface {
	OnMarketMakerRegistered(ctx context.Context, event *MarketMakerEvent) error
	OnLiquidityProvided(ctx context.Context, event *LiquidityEvent) error
	OnSpreadAdjusted(ctx context.Context, event *SpreadAdjustmentEvent) error
	OnMarketMakingPnL(ctx context.Context, event *MarketMakingPnLEvent) error
	// Additional methods expected by manager.go
	OnStrategyChange(ctx context.Context, event *MMStrategyEvent) error
	OnQuoteUpdate(ctx context.Context, event *MMQuoteEvent) error
	OnOrderManagement(ctx context.Context, event *MMOrderEvent) error
	OnInventoryRebalancing(ctx context.Context, event *MMInventoryEvent) error
	OnRiskBreach(ctx context.Context, event *MMRiskEvent) error
	OnPnLAlert(ctx context.Context, event *MMPnLEvent) error
	OnPerformanceReport(ctx context.Context, event *MMPerformanceEvent) error
}

// BaseEvent provides common fields for all events
type BaseEvent struct {
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Module    string    `json:"module"`
	ID        uuid.UUID `json:"id"`
	UserID    string    `json:"user_id"` // Added UserID field that integration files expect
}

// Event type constants
const (
	// Account events
	EventTypeAccountCreation     = "account.creation"
	EventTypeAccountUpdate       = "account.update"
	EventTypeAccountSuspension   = "account.suspension"
	EventTypeAccountReactivation = "account.reactivation"
	EventTypeAccountPermission   = "account.permission"
	EventTypeAccountKYC          = "account.kyc"
	EventTypeAccountTier         = "account.tier"
	EventTypeAccountDormancy     = "account.dormancy"

	// Fiat events
	EventTypeFiatDeposit     = "fiat.deposit"
	EventTypeFiatWithdrawal  = "fiat.withdrawal"
	EventTypeFiatTransfer    = "fiat.transfer"
	EventTypeFiatBankAccount = "fiat.bank_account"
	EventTypeFiatConversion  = "fiat.conversion"

	// Trading events
	EventTypeTradingOrderPlaced    = "trading.order_placed"
	EventTypeTradingOrderExecuted  = "trading.order_executed"
	EventTypeTradingOrderCanceled  = "trading.order_canceled"
	EventTypeTradingTradeExecuted  = "trading.trade_executed"
	EventTypeTradingPositionUpdate = "trading.position_update"

	// Wallet events
	EventTypeWalletCryptoDeposit    = "wallet.crypto_deposit"
	EventTypeWalletCryptoWithdrawal = "wallet.crypto_withdrawal"
	EventTypeWalletInternalTransfer = "wallet.internal_transfer"
	EventTypeWalletBalanceUpdate    = "wallet.balance_update"
	EventTypeWalletAddress          = "wallet.address"
	EventTypeWalletStaking          = "wallet.staking"
	// Market making events
	EventTypeMMStrategy    = "mm.strategy"
	EventTypeMMQuote       = "mm.quote"
	EventTypeMMOrder       = "mm.order"
	EventTypeMMInventory   = "mm.inventory"
	EventTypeMMRisk        = "mm.risk"
	EventTypeMMPnL         = "mm.pnl"
	EventTypeMMPerformance = "mm.performance"

	// User authentication events
	EventTypeUserRegistration  = "user.registration"
	EventTypeUserLogin         = "user.login"
	EventTypeUserLogout        = "user.logout"
	EventTypePasswordChange    = "user.password_change"
	EventTypeEmailVerification = "user.email_verification"
	EventTypeTwoFA             = "user.2fa"
	EventTypeAccountLock       = "user.account_lock"
)

// Module constants
const (
	ModuleAccounts     = "accounts"
	ModuleFiat         = "fiat"
	ModuleTrading      = "trading"
	ModuleWallet       = "wallet"
	ModuleMarketMaking = "market_making"
	ModuleUserAuth     = "user_auth"
)

// getCurrentTimestamp returns the current timestamp
func getCurrentTimestamp() time.Time {
	return time.Now().UTC()
}

// User authentication events
type UserRegistrationEvent struct {
	BaseEvent
	Email     string                 `json:"email"`
	Country   string                 `json:"country"`
	IPAddress string                 `json:"ip_address"`
	UserAgent string                 `json:"user_agent"`
	DeviceID  string                 `json:"device_id"`
	Metadata  map[string]interface{} `json:"metadata"`
}

type UserLoginEvent struct {
	BaseEvent
	IPAddress   string `json:"ip_address"`
	UserAgent   string `json:"user_agent"`
	Success     bool   `json:"success"`
	DeviceID    string `json:"device_id"`
	Country     string `json:"country"`
	FailReason  string `json:"fail_reason,omitempty"`
	LoginMethod string `json:"login_method"`
}

type UserLogoutEvent struct {
	BaseEvent
	IPAddress string `json:"ip_address"`
	Duration  int64  `json:"duration"`
}

type PasswordChangeEvent struct {
	BaseEvent
	IPAddress string `json:"ip_address"`
	Reason    string `json:"reason"`
	Success   bool   `json:"success"`
}

type EmailVerificationEvent struct {
	BaseEvent
	Email      string    `json:"email"`
	Verified   bool      `json:"verified"`
	VerifiedAt time.Time `json:"verified_at"`
}

type TwoFAEvent struct {
	BaseEvent
	Method    string `json:"method"`
	Enabled   bool   `json:"enabled"`
	IPAddress string `json:"ip_address"`
}

type AccountLockEvent struct {
	BaseEvent
	Reason   string         `json:"reason"`
	LockedBy uuid.UUID      `json:"locked_by"`
	Duration *time.Duration `json:"duration,omitempty"`
}

// Trading events
type OrderPlacedEvent struct {
	BaseEvent
	OrderID   string          `json:"order_id"`
	Market    string          `json:"market"`
	Side      string          `json:"side"`
	Type      string          `json:"order_type"`
	Price     decimal.Decimal `json:"price"`
	Quantity  decimal.Decimal `json:"quantity"`
	IPAddress string          `json:"ip_address"`
}

type OrderExecutedEvent struct {
	BaseEvent
	OrderID       string          `json:"order_id"`
	Market        string          `json:"market"`
	Side          string          `json:"side"`
	ExecutedPrice decimal.Decimal `json:"executed_price"`
	ExecutedQty   decimal.Decimal `json:"executed_qty"`
	Fee           decimal.Decimal `json:"fee"`
}

type OrderCancelledEvent struct {
	BaseEvent
	OrderID string `json:"order_id"`
	Market  string `json:"market"`
	Reason  string `json:"reason"`
}

type TradeExecutedEvent struct {
	BaseEvent
	TradeID   string          `json:"trade_id"`
	Market    string          `json:"market"`
	Price     decimal.Decimal `json:"price"`
	Quantity  decimal.Decimal `json:"quantity"`
	TakerSide string          `json:"taker_side"`
	Fee       decimal.Decimal `json:"fee"`
	BuyerID   string          `json:"buyer_id"`
	SellerID  string          `json:"seller_id"`
}

type PositionUpdateEvent struct {
	BaseEvent
	Market   string          `json:"market"`
	Size     decimal.Decimal `json:"size"`
	AvgPrice decimal.Decimal `json:"avg_price"`
	PnL      decimal.Decimal `json:"pnl"`
	Margin   decimal.Decimal `json:"margin"`
}

type MarketDataEvent struct {
	BaseEvent
	Market    string          `json:"market"`
	Price     decimal.Decimal `json:"price"`
	Volume    decimal.Decimal `json:"volume"`
	Change24h decimal.Decimal `json:"change_24h"`
}

// Fiat events
type FiatDepositEvent struct {
	BaseEvent
	Amount      decimal.Decimal `json:"amount"`
	Currency    string          `json:"currency"`
	Reference   string          `json:"reference"`
	Status      string          `json:"status"`
	BankAccount BankAccountInfo `json:"bank_account"`
}

type FiatWithdrawalEvent struct {
	BaseEvent
	Amount      decimal.Decimal `json:"amount"`
	Currency    string          `json:"currency"`
	Reference   string          `json:"reference"`
	Status      string          `json:"status"`
	BankAccount BankAccountInfo `json:"bank_account"`
}

type FiatTransferEvent struct {
	BaseEvent
	FromUserID uuid.UUID       `json:"from_user_id"`
	ToUserID   uuid.UUID       `json:"to_user_id"`
	Amount     decimal.Decimal `json:"amount"`
	Currency   string          `json:"currency"`
	Reference  string          `json:"reference"`
}

type FiatBankAccountEvent struct {
	BaseEvent
	BankAccount BankAccountInfo `json:"bank_account"`
	Action      string          `json:"action"` // add, remove, update
}

type FiatConversionEvent struct {
	BaseEvent
	FromCurrency string          `json:"from_currency"`
	ToCurrency   string          `json:"to_currency"`
	FromAmount   decimal.Decimal `json:"from_amount"`
	ToAmount     decimal.Decimal `json:"to_amount"`
	Rate         decimal.Decimal `json:"rate"`
	Reference    string          `json:"reference"`
}

type BankAccountEvent struct {
	BaseEvent
	BankName      string `json:"bank_name"`
	AccountNumber string `json:"account_number"`
	Country       string `json:"country"`
	Currency      string `json:"currency"`
	Action        string `json:"action"`
}

type PaymentMethodEvent struct {
	BaseEvent
	MethodType string `json:"method_type"`
	Provider   string `json:"provider"`
	Status     string `json:"status"`
	Action     string `json:"action"`
}

type FiatTransactionFailedEvent struct {
	BaseEvent
	Amount    decimal.Decimal `json:"amount"`
	Currency  string          `json:"currency"`
	Reference string          `json:"reference"`
	Reason    string          `json:"reason"`
}

// Wallet events
type CryptoDepositEvent struct {
	BaseEvent
	Currency      string          `json:"currency"`
	Amount        decimal.Decimal `json:"amount"`
	FromAddress   string          `json:"from_address"`
	ToAddress     string          `json:"to_address"`
	TxHash        string          `json:"tx_hash"`
	Network       string          `json:"network"`
	Confirmations int             `json:"confirmations"`
}

type CryptoWithdrawalEvent struct {
	BaseEvent
	Currency    string          `json:"currency"`
	Amount      decimal.Decimal `json:"amount"`
	FromAddress string          `json:"from_address"`
	ToAddress   string          `json:"to_address"`
	TxHash      string          `json:"tx_hash"`
	Network     string          `json:"network"`
	Fee         decimal.Decimal `json:"fee"`
}

type WalletCreatedEvent struct {
	BaseEvent
	WalletType string `json:"wallet_type"`
	Currency   string `json:"currency"`
	Address    string `json:"address"`
}

type WalletBalanceEvent struct {
	BaseEvent
	Currency  string          `json:"currency"`
	Balance   decimal.Decimal `json:"balance"`
	Available decimal.Decimal `json:"available"`
	Locked    decimal.Decimal `json:"locked"`
}

type WalletInternalTransferEvent struct {
	BaseEvent
	FromUserID uuid.UUID       `json:"from_user_id"`
	ToUserID   uuid.UUID       `json:"to_user_id"`
	Currency   string          `json:"currency"`
	Amount     decimal.Decimal `json:"amount"`
	Reference  string          `json:"reference"`
}

type WalletBalanceUpdateEvent struct {
	BaseEvent
	Currency   string          `json:"currency"`
	OldBalance decimal.Decimal `json:"old_balance"`
	NewBalance decimal.Decimal `json:"new_balance"`
	Reason     string          `json:"reason"`
}

type WalletAddressEvent struct {
	BaseEvent
	Currency    string `json:"currency"`
	Network     string `json:"network"`
	Address     string `json:"address"`
	AddressType string `json:"address_type"`
	Action      string `json:"action"`
}

type WalletStakingEvent struct {
	BaseEvent
	Currency       string          `json:"currency"`
	Amount         decimal.Decimal `json:"amount"`
	Action         string          `json:"action"` // stake, unstake
	StakingPeriod  time.Duration   `json:"staking_period"`
	ExpectedReward decimal.Decimal `json:"expected_reward"`
	Reward         decimal.Decimal `json:"reward"`
	Penalty        decimal.Decimal `json:"penalty"`
}

type AddressGeneratedEvent struct {
	BaseEvent
	Currency string `json:"currency"`
	Network  string `json:"network"`
	Address  string `json:"address"`
}

type WalletCryptoDepositEvent struct {
	BaseEvent
	Currency      string          `json:"currency"`
	Amount        decimal.Decimal `json:"amount"`
	FromAddress   string          `json:"from_address"`
	ToAddress     string          `json:"to_address"`
	TxHash        string          `json:"tx_hash"`
	Network       string          `json:"network"`
	Confirmations int             `json:"confirmations"`
}

type WalletCryptoWithdrawalEvent struct {
	BaseEvent
	Currency    string          `json:"currency"`
	Amount      decimal.Decimal `json:"amount"`
	FromAddress string          `json:"from_address"`
	ToAddress   string          `json:"to_address"`
	TxHash      string          `json:"tx_hash"`
	Network     string          `json:"network"`
	Fee         decimal.Decimal `json:"fee"`
}

// Account events (already properly defined above, just need to add missing fields)
type AccountCreatedEvent struct {
	BaseEvent
	Email       string                 `json:"email"`
	Country     string                 `json:"country"`
	IPAddress   string                 `json:"ip_address"`
	AccountType string                 `json:"account_type"`
	Metadata    map[string]interface{} `json:"metadata"`
}

type AccountCreationEvent struct {
	BaseEvent
	Email       string                 `json:"email"`
	Country     string                 `json:"country"`
	IPAddress   string                 `json:"ip_address"`
	AccountType string                 `json:"account_type"`
	Metadata    map[string]interface{} `json:"metadata"`
}

type AccountUpdateEvent struct {
	BaseEvent
	UpdateType string                 `json:"update_type"`
	OldData    map[string]interface{} `json:"old_data"`
	NewData    map[string]interface{} `json:"new_data"`
}

type AccountSuspensionEvent struct {
	BaseEvent
	Reason      string    `json:"reason"`
	SuspendedBy uuid.UUID `json:"suspended_by"`
	Duration    int64     `json:"duration"`
}

type AccountUpdatedEvent struct {
	BaseEvent
	UpdateType string                 `json:"update_type"`
	OldData    map[string]interface{} `json:"old_data"`
	NewData    map[string]interface{} `json:"new_data"`
}

type AccountSuspendedEvent struct {
	BaseEvent
	Reason      string    `json:"reason"`
	SuspendedBy uuid.UUID `json:"suspended_by"`
	Duration    int64     `json:"duration"`
}

type AccountReactivationEvent struct {
	BaseEvent
	ReactivatedBy uuid.UUID `json:"reactivated_by"`
	Reason        string    `json:"reason"`
}

type AccountPermissionEvent struct {
	BaseEvent
	Permission string    `json:"permission"`
	Action     string    `json:"action"` // grant, revoke
	ChangedBy  uuid.UUID `json:"changed_by"`
}

type AccountKYCEvent struct {
	BaseEvent
	OldStatus  string    `json:"old_status"`
	NewStatus  string    `json:"new_status"`
	VerifiedBy uuid.UUID `json:"verified_by"`
	Documents  []string  `json:"documents"`
}

type AccountTierEvent struct {
	BaseEvent
	OldTier   int       `json:"old_tier"`
	NewTier   int       `json:"new_tier"`
	ChangedBy uuid.UUID `json:"changed_by"`
	Reason    string    `json:"reason"`
}

type AccountDormancyEvent struct {
	BaseEvent
	IsDormant    bool      `json:"is_dormant"`
	LastActivity time.Time `json:"last_activity"`
	Reason       string    `json:"reason"`
}

type DocumentUploadEvent struct {
	BaseEvent
	DocumentType string `json:"document_type"`
	FileName     string `json:"file_name"`
	Status       string `json:"status"`
}

type KYCStatusEvent struct {
	BaseEvent
	OldStatus  string    `json:"old_status"`
	NewStatus  string    `json:"new_status"`
	Level      int       `json:"level"`
	ReviewedBy uuid.UUID `json:"reviewed_by"`
}

type ProfileUpdateEvent struct {
	BaseEvent
	Changes   map[string]interface{} `json:"changes"`
	UpdatedBy uuid.UUID              `json:"updated_by"`
}

// Market making events
type MarketMakerEvent struct {
	BaseEvent
	Market      string          `json:"market"`
	Status      string          `json:"status"`
	MinSpread   decimal.Decimal `json:"min_spread"`
	MaxPosition decimal.Decimal `json:"max_position"`
}

type LiquidityEvent struct {
	BaseEvent
	Market    string          `json:"market"`
	BidVolume decimal.Decimal `json:"bid_volume"`
	AskVolume decimal.Decimal `json:"ask_volume"`
	Spread    decimal.Decimal `json:"spread"`
}

type SpreadAdjustmentEvent struct {
	BaseEvent
	Market    string          `json:"market"`
	OldSpread decimal.Decimal `json:"old_spread"`
	NewSpread decimal.Decimal `json:"new_spread"`
	Reason    string          `json:"reason"`
}

type MarketMakingPnLEvent struct {
	BaseEvent
	Market        string          `json:"market"`
	RealizedPnL   decimal.Decimal `json:"realized_pnl"`
	UnrealizedPnL decimal.Decimal `json:"unrealized_pnl"`
	Fees          decimal.Decimal `json:"fees"`
}

// Additional market making events used in integration files
type MMStrategyEvent struct {
	BaseEvent
	StrategyID string     `json:"strategy_id"`
	Market     string     `json:"market"`
	Action     string     `json:"action"` // activate, deactivate
	Strategy   MMStrategy `json:"strategy"`
	Reason     string     `json:"reason"`
}

type MMQuoteEvent struct {
	BaseEvent
	Market string  `json:"market"`
	Quote  MMQuote `json:"quote"`
}

type MMOrderEvent struct {
	BaseEvent
	OrderID string  `json:"order_id"`
	Market  string  `json:"market"`
	Order   MMOrder `json:"order"`
	Action  string  `json:"action"` // place, cancel
	Reason  string  `json:"reason"`
}

type MMInventoryEvent struct {
	BaseEvent
	Market      string                 `json:"market"`
	Rebalancing MMInventoryRebalancing `json:"rebalancing"`
}

type MMRiskEvent struct {
	BaseEvent
	Market     string       `json:"market"`
	RiskBreach MMRiskBreach `json:"risk_breach"`
}

type MMPnLEvent struct {
	BaseEvent
	Market   string     `json:"market"`
	PnLAlert MMPnLAlert `json:"pnl_alert"`
}

type MMPerformanceEvent struct {
	BaseEvent
	Market string              `json:"market"`
	Report MMPerformanceReport `json:"report"`
}

// Supporting data types referenced by integration files
type BankAccountInfo struct {
	BankName      string `json:"bank_name"`
	AccountNumber string `json:"account_number"`
	RoutingNumber string `json:"routing_number"`
	Country       string `json:"country"`
	Currency      string `json:"currency"`
	AccountType   string `json:"account_type"`
}

type MMStrategy struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Type        string          `json:"type"`
	Symbol      string          `json:"symbol"`
	Market      string          `json:"market"`
	MinSpread   decimal.Decimal `json:"min_spread"`
	MaxPosition decimal.Decimal `json:"max_position"`
	RiskLimit   decimal.Decimal `json:"risk_limit"`
	IsActive    bool            `json:"is_active"`
}

type MMQuote struct {
	Symbol   string          `json:"symbol"`
	Market   string          `json:"market"`
	BidPrice decimal.Decimal `json:"bid_price"`
	AskPrice decimal.Decimal `json:"ask_price"`
	BidSize  decimal.Decimal `json:"bid_size"`
	AskSize  decimal.Decimal `json:"ask_size"`
}

type MMOrder struct {
	ID         string          `json:"id"`
	OrderID    string          `json:"order_id"`
	StrategyID string          `json:"strategy_id"`
	Symbol     string          `json:"symbol"`
	Market     string          `json:"market"`
	Side       string          `json:"side"`
	Price      decimal.Decimal `json:"price"`
	Quantity   decimal.Decimal `json:"quantity"`
	Type       string          `json:"type"`
}

type MMInventoryRebalancing struct {
	Symbol           string          `json:"symbol"`
	Market           string          `json:"market"`
	Currency         string          `json:"currency"`
	CurrentInventory decimal.Decimal `json:"current_inventory"`
	TargetInventory  decimal.Decimal `json:"target_inventory"`
	CurrentPos       decimal.Decimal `json:"current_position"`
	TargetPos        decimal.Decimal `json:"target_position"`
	RebalanceQty     decimal.Decimal `json:"rebalance_qty"`
	Action           string          `json:"action"`
}

type MMRiskBreach struct {
	Market       string          `json:"market"`
	RiskType     string          `json:"risk_type"`
	Limit        decimal.Decimal `json:"limit"`
	Current      decimal.Decimal `json:"current"`
	CurrentValue decimal.Decimal `json:"current_value"`
	Severity     string          `json:"severity"`
	ActionTaken  string          `json:"action_taken"`
}

type MMPnLAlert struct {
	Symbol        string          `json:"symbol"`
	Market        string          `json:"market"`
	RealizedPnL   decimal.Decimal `json:"realized_pnl"`
	UnrealizedPnL decimal.Decimal `json:"unrealized_pnl"`
	PnL           decimal.Decimal `json:"pnl"`
	Threshold     decimal.Decimal `json:"threshold"`
	AlertType     string          `json:"alert_type"`
	Period        string          `json:"period"`
}

type MMPerformanceReport struct {
	Market      string          `json:"market"`
	Period      string          `json:"period"`
	TotalPnL    decimal.Decimal `json:"total_pnl"`
	SharpeRatio decimal.Decimal `json:"sharpe_ratio"`
	MaxDrawdown decimal.Decimal `json:"max_drawdown"`
	Volume      decimal.Decimal `json:"volume"`
	Trades      int             `json:"trades"`
	Spread      decimal.Decimal `json:"avg_spread"`
	Uptime      decimal.Decimal `json:"uptime"`
	FillRatio   decimal.Decimal `json:"fill_ratio"`
	PeriodStart time.Time       `json:"period_start"`
	PeriodEnd   time.Time       `json:"period_end"`
}
