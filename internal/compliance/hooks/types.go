// Package hooks provides platform-wide integration hooks for the compliance module
package hooks

import (
	"context"
	"sync"
	"time"

	"github.com/Aidin1998/finalex/internal/compliance/interfaces"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// HookManager manages all compliance integration hooks across the platform
type HookManager struct {
	compliance   interfaces.ComplianceService
	audit        interfaces.AuditService
	monitoring   interfaces.MonitoringService
	manipulation interfaces.ManipulationService
	risk         interfaces.RiskService

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
	risk interfaces.RiskService,
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
}

// WalletHook defines hooks for wallet events
type WalletHook interface {
	OnCryptoDeposit(ctx context.Context, event *CryptoDepositEvent) error
	OnCryptoWithdrawal(ctx context.Context, event *CryptoWithdrawalEvent) error
	OnWalletCreated(ctx context.Context, event *WalletCreatedEvent) error
	OnWalletBalanceChanged(ctx context.Context, event *WalletBalanceEvent) error
	OnAddressGenerated(ctx context.Context, event *AddressGeneratedEvent) error
}

// AccountsHook defines hooks for account management events
type AccountsHook interface {
	OnAccountCreated(ctx context.Context, event *AccountCreatedEvent) error
	OnAccountUpdated(ctx context.Context, event *AccountUpdatedEvent) error
	OnAccountSuspended(ctx context.Context, event *AccountSuspendedEvent) error
	OnKYCStatusChanged(ctx context.Context, event *KYCStatusEvent) error
	OnDocumentUploaded(ctx context.Context, event *DocumentUploadEvent) error
	OnProfileUpdated(ctx context.Context, event *ProfileUpdateEvent) error
}

// MarketMakingHook defines hooks for market making events
type MarketMakingHook interface {
	OnMarketMakerRegistered(ctx context.Context, event *MarketMakerEvent) error
	OnLiquidityProvided(ctx context.Context, event *LiquidityEvent) error
	OnSpreadAdjusted(ctx context.Context, event *SpreadAdjustmentEvent) error
	OnMarketMakingPnL(ctx context.Context, event *MarketMakingPnLEvent) error
}

// Event types for different modules

// UserAuth Events
type UserRegistrationEvent struct {
	UserID    uuid.UUID              `json:"user_id"`
	Email     string                 `json:"email"`
	Country   string                 `json:"country"`
	IPAddress string                 `json:"ip_address"`
	UserAgent string                 `json:"user_agent"`
	DeviceID  string                 `json:"device_id"`
	Timestamp time.Time              `json:"timestamp"`
	Metadata  map[string]interface{} `json:"metadata"`
}

type UserLoginEvent struct {
	UserID      uuid.UUID              `json:"user_id"`
	SessionID   uuid.UUID              `json:"session_id"`
	IPAddress   string                 `json:"ip_address"`
	UserAgent   string                 `json:"user_agent"`
	DeviceID    string                 `json:"device_id"`
	Country     string                 `json:"country"`
	LoginMethod string                 `json:"login_method"`
	Success     bool                   `json:"success"`
	FailReason  string                 `json:"fail_reason,omitempty"`
	Timestamp   time.Time              `json:"timestamp"`
	Metadata    map[string]interface{} `json:"metadata"`
}

type UserLogoutEvent struct {
	UserID    uuid.UUID `json:"user_id"`
	SessionID uuid.UUID `json:"session_id"`
	Timestamp time.Time `json:"timestamp"`
}

type PasswordChangeEvent struct {
	UserID    uuid.UUID `json:"user_id"`
	IPAddress string    `json:"ip_address"`
	Success   bool      `json:"success"`
	Timestamp time.Time `json:"timestamp"`
}

type EmailVerificationEvent struct {
	UserID    uuid.UUID `json:"user_id"`
	Email     string    `json:"email"`
	Verified  bool      `json:"verified"`
	Timestamp time.Time `json:"timestamp"`
}

type TwoFAEvent struct {
	UserID    uuid.UUID `json:"user_id"`
	Method    string    `json:"method"`
	Enabled   bool      `json:"enabled"`
	Timestamp time.Time `json:"timestamp"`
}

type AccountLockEvent struct {
	UserID    uuid.UUID      `json:"user_id"`
	Reason    string         `json:"reason"`
	LockedBy  uuid.UUID      `json:"locked_by"`
	Duration  *time.Duration `json:"duration,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
}

// Trading Events
type OrderPlacedEvent struct {
	OrderID   string          `json:"order_id"`
	UserID    uuid.UUID       `json:"user_id"`
	Market    string          `json:"market"`
	Side      string          `json:"side"`
	Type      string          `json:"type"`
	Quantity  decimal.Decimal `json:"quantity"`
	Price     decimal.Decimal `json:"price"`
	IPAddress string          `json:"ip_address"`
	Timestamp time.Time       `json:"timestamp"`
}

type OrderExecutedEvent struct {
	OrderID       string          `json:"order_id"`
	UserID        uuid.UUID       `json:"user_id"`
	Market        string          `json:"market"`
	ExecutedQty   decimal.Decimal `json:"executed_qty"`
	ExecutedPrice decimal.Decimal `json:"executed_price"`
	RemainingQty  decimal.Decimal `json:"remaining_qty"`
	TradeID       string          `json:"trade_id"`
	Timestamp     time.Time       `json:"timestamp"`
}

type OrderCancelledEvent struct {
	OrderID   string    `json:"order_id"`
	UserID    uuid.UUID `json:"user_id"`
	Market    string    `json:"market"`
	Reason    string    `json:"reason"`
	Timestamp time.Time `json:"timestamp"`
}

type TradeExecutedEvent struct {
	TradeID     string          `json:"trade_id"`
	Market      string          `json:"market"`
	BuyerID     uuid.UUID       `json:"buyer_id"`
	SellerID    uuid.UUID       `json:"seller_id"`
	Quantity    decimal.Decimal `json:"quantity"`
	Price       decimal.Decimal `json:"price"`
	BuyOrderID  string          `json:"buy_order_id"`
	SellOrderID string          `json:"sell_order_id"`
	Timestamp   time.Time       `json:"timestamp"`
}

type PositionUpdateEvent struct {
	UserID        uuid.UUID       `json:"user_id"`
	Market        string          `json:"market"`
	Position      decimal.Decimal `json:"position"`
	AvgPrice      decimal.Decimal `json:"avg_price"`
	UnrealizedPnL decimal.Decimal `json:"unrealized_pnl"`
	Timestamp     time.Time       `json:"timestamp"`
}

type MarketDataEvent struct {
	Market    string          `json:"market"`
	Price     decimal.Decimal `json:"price"`
	Volume    decimal.Decimal `json:"volume"`
	BidPrice  decimal.Decimal `json:"bid_price"`
	AskPrice  decimal.Decimal `json:"ask_price"`
	Timestamp time.Time       `json:"timestamp"`
}

// Fiat Events
type FiatDepositEvent struct {
	DepositID     uuid.UUID       `json:"deposit_id"`
	UserID        uuid.UUID       `json:"user_id"`
	Amount        decimal.Decimal `json:"amount"`
	Currency      string          `json:"currency"`
	PaymentMethod string          `json:"payment_method"`
	BankAccount   string          `json:"bank_account"`
	Status        string          `json:"status"`
	Timestamp     time.Time       `json:"timestamp"`
}

type FiatWithdrawalEvent struct {
	WithdrawalID uuid.UUID       `json:"withdrawal_id"`
	UserID       uuid.UUID       `json:"user_id"`
	Amount       decimal.Decimal `json:"amount"`
	Currency     string          `json:"currency"`
	BankAccount  string          `json:"bank_account"`
	Status       string          `json:"status"`
	Timestamp    time.Time       `json:"timestamp"`
}

type BankAccountEvent struct {
	UserID        uuid.UUID `json:"user_id"`
	AccountID     string    `json:"account_id"`
	BankName      string    `json:"bank_name"`
	AccountNumber string    `json:"account_number"`
	Country       string    `json:"country"`
	Currency      string    `json:"currency"`
	Status        string    `json:"status"`
	Timestamp     time.Time `json:"timestamp"`
}

type PaymentMethodEvent struct {
	UserID     uuid.UUID `json:"user_id"`
	MethodID   string    `json:"method_id"`
	MethodType string    `json:"method_type"`
	Status     string    `json:"status"`
	Timestamp  time.Time `json:"timestamp"`
}

type FiatTransactionFailedEvent struct {
	TransactionID uuid.UUID `json:"transaction_id"`
	UserID        uuid.UUID `json:"user_id"`
	Type          string    `json:"type"`
	Reason        string    `json:"reason"`
	ErrorCode     string    `json:"error_code"`
	Timestamp     time.Time `json:"timestamp"`
}

// Wallet Events
type CryptoDepositEvent struct {
	DepositID     uuid.UUID       `json:"deposit_id"`
	UserID        uuid.UUID       `json:"user_id"`
	Currency      string          `json:"currency"`
	Amount        decimal.Decimal `json:"amount"`
	Address       string          `json:"address"`
	TxHash        string          `json:"tx_hash"`
	Network       string          `json:"network"`
	Confirmations int             `json:"confirmations"`
	Status        string          `json:"status"`
	Timestamp     time.Time       `json:"timestamp"`
}

type CryptoWithdrawalEvent struct {
	WithdrawalID uuid.UUID       `json:"withdrawal_id"`
	UserID       uuid.UUID       `json:"user_id"`
	Currency     string          `json:"currency"`
	Amount       decimal.Decimal `json:"amount"`
	ToAddress    string          `json:"to_address"`
	TxHash       string          `json:"tx_hash"`
	Network      string          `json:"network"`
	Fee          decimal.Decimal `json:"fee"`
	Status       string          `json:"status"`
	Timestamp    time.Time       `json:"timestamp"`
}

type WalletCreatedEvent struct {
	WalletID  uuid.UUID `json:"wallet_id"`
	UserID    uuid.UUID `json:"user_id"`
	Currency  string    `json:"currency"`
	Network   string    `json:"network"`
	Address   string    `json:"address"`
	Timestamp time.Time `json:"timestamp"`
}

type WalletBalanceEvent struct {
	UserID       uuid.UUID       `json:"user_id"`
	Currency     string          `json:"currency"`
	Balance      decimal.Decimal `json:"balance"`
	Available    decimal.Decimal `json:"available"`
	Locked       decimal.Decimal `json:"locked"`
	ChangeType   string          `json:"change_type"`
	ChangeAmount decimal.Decimal `json:"change_amount"`
	Timestamp    time.Time       `json:"timestamp"`
}

type AddressGeneratedEvent struct {
	UserID    uuid.UUID `json:"user_id"`
	Currency  string    `json:"currency"`
	Network   string    `json:"network"`
	Address   string    `json:"address"`
	Purpose   string    `json:"purpose"`
	Timestamp time.Time `json:"timestamp"`
}

// Account Events
type AccountCreatedEvent struct {
	UserID    uuid.UUID `json:"user_id"`
	Email     string    `json:"email"`
	Country   string    `json:"country"`
	IPAddress string    `json:"ip_address"`
	Timestamp time.Time `json:"timestamp"`
}

type AccountUpdatedEvent struct {
	UserID    uuid.UUID              `json:"user_id"`
	Changes   map[string]interface{} `json:"changes"`
	UpdatedBy uuid.UUID              `json:"updated_by"`
	Timestamp time.Time              `json:"timestamp"`
}

type AccountSuspendedEvent struct {
	UserID      uuid.UUID      `json:"user_id"`
	Reason      string         `json:"reason"`
	SuspendedBy uuid.UUID      `json:"suspended_by"`
	Duration    *time.Duration `json:"duration,omitempty"`
	Timestamp   time.Time      `json:"timestamp"`
}

type KYCStatusEvent struct {
	UserID     uuid.UUID `json:"user_id"`
	Level      int       `json:"level"`
	Status     string    `json:"status"`
	PrevStatus string    `json:"prev_status"`
	ReviewedBy uuid.UUID `json:"reviewed_by"`
	Timestamp  time.Time `json:"timestamp"`
}

type DocumentUploadEvent struct {
	UserID       uuid.UUID `json:"user_id"`
	DocumentID   uuid.UUID `json:"document_id"`
	DocumentType string    `json:"document_type"`
	Status       string    `json:"status"`
	Timestamp    time.Time `json:"timestamp"`
}

type ProfileUpdateEvent struct {
	UserID    uuid.UUID              `json:"user_id"`
	Field     string                 `json:"field"`
	OldValue  interface{}            `json:"old_value"`
	NewValue  interface{}            `json:"new_value"`
	Metadata  map[string]interface{} `json:"metadata"`
	Timestamp time.Time              `json:"timestamp"`
}

// Market Making Events
type MarketMakerEvent struct {
	UserID    uuid.UUID `json:"user_id"`
	Market    string    `json:"market"`
	Status    string    `json:"status"`
	TierLevel int       `json:"tier_level"`
	Timestamp time.Time `json:"timestamp"`
}

type LiquidityEvent struct {
	UserID    uuid.UUID       `json:"user_id"`
	Market    string          `json:"market"`
	BidAmount decimal.Decimal `json:"bid_amount"`
	AskAmount decimal.Decimal `json:"ask_amount"`
	BidPrice  decimal.Decimal `json:"bid_price"`
	AskPrice  decimal.Decimal `json:"ask_price"`
	Timestamp time.Time       `json:"timestamp"`
}

type SpreadAdjustmentEvent struct {
	UserID       uuid.UUID       `json:"user_id"`
	Market       string          `json:"market"`
	OldSpread    decimal.Decimal `json:"old_spread"`
	NewSpread    decimal.Decimal `json:"new_spread"`
	AdjustReason string          `json:"adjust_reason"`
	Timestamp    time.Time       `json:"timestamp"`
}

type MarketMakingPnLEvent struct {
	UserID        uuid.UUID       `json:"user_id"`
	Market        string          `json:"market"`
	RealizedPnL   decimal.Decimal `json:"realized_pnl"`
	UnrealizedPnL decimal.Decimal `json:"unrealized_pnl"`
	Fees          decimal.Decimal `json:"fees"`
	Period        string          `json:"period"`
	Timestamp     time.Time       `json:"timestamp"`
}
