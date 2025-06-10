package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Order type constants
type OrderType string

const (
	OrderTypeLimit      OrderType = "LIMIT"
	OrderTypeMarket     OrderType = "MARKET"
	OrderTypeStopLimit  OrderType = "STOP_LIMIT"
	OrderTypeStopMarket OrderType = "STOP_MARKET"
	OrderTypeIOC        OrderType = "IOC"
	OrderTypeFOK        OrderType = "FOK"
	OrderTypeIceberg    OrderType = "ICEBERG"
	OrderTypeHidden     OrderType = "HIDDEN"
	OrderTypeGTD        OrderType = "GTD"
	OrderTypeOCO        OrderType = "OCO"
	OrderTypeTrailing   OrderType = "TRAILING_STOP"
	OrderTypeTWAP       OrderType = "TWAP"
	OrderTypeVWAP       OrderType = "VWAP"
)

// Order side constants
type OrderSide string

const (
	SideBuy  OrderSide = "BUY"
	SideSell OrderSide = "SELL"
)

// Trading status constants
type TradingStatus string

const (
	TradingStatusActive   TradingStatus = "active"
	TradingStatusInactive TradingStatus = "inactive"
)

// Order status constants
const (
	OrderStatusNew             = "NEW"
	OrderStatusPartiallyFilled = "PARTIALLY_FILLED"
	OrderStatusFilled          = "FILLED"
	OrderStatusCanceled        = "CANCELED"
	OrderStatusRejected        = "REJECTED"
	OrderStatusTriggered       = "TRIGGERED"
)

// Time in force constants
const (
	TimeInForceGTC = "GTC" // Good Till Cancel
	TimeInForceIOC = "IOC" // Immediate or Cancel
	TimeInForceFOK = "FOK" // Fill or Kill
	TimeInForceGTD = "GTD" // Good Till Date
)

// Convenience constants for backward compatibility with string types used in tests
const (
	Market OrderType = OrderTypeMarket
	Limit  OrderType = OrderTypeLimit
)

// PlaceOrderRequest represents a request to place an order
type PlaceOrderRequest struct {
	Symbol          string           `json:"symbol" validate:"required"`
	Side            OrderSide        `json:"side" validate:"required"`
	Type            OrderType        `json:"type" validate:"required"`
	Quantity        decimal.Decimal  `json:"quantity" validate:"required,gt=0"`
	Price           *decimal.Decimal `json:"price,omitempty" validate:"omitempty,gt=0"`
	TimeInForce     string           `json:"time_in_force,omitempty" validate:"omitempty,oneof=GTC IOC FOK GTD"`
	StopPrice       *decimal.Decimal `json:"stop_price,omitempty" validate:"omitempty,gt=0"`
	DisplayQuantity *decimal.Decimal `json:"display_quantity,omitempty" validate:"omitempty,gt=0"`
	ExpiresAt       *time.Time       `json:"expires_at,omitempty"`
	ReduceOnly      *bool            `json:"reduce_only,omitempty"`
	PostOnly        *bool            `json:"post_only,omitempty"`
}

// User represents a user in the system
type User struct {
	ID              uuid.UUID  `json:"id" gorm:"primaryKey;type:uuid" validate:"required,uuid"`
	Email           string     `json:"email" gorm:"uniqueIndex" validate:"required,email,max=254"`
	Username        string     `json:"username" gorm:"uniqueIndex" validate:"required,min=3,max=30,alphanum"`
	PasswordHash    string     `json:"-" gorm:"column:password_hash" validate:"required,min=60"`
	FirstName       string     `json:"first_name" validate:"required,min=1,max=50,alpha_space"`
	LastName        string     `json:"last_name" validate:"required,min=1,max=50,alpha_space"`
	KYCStatus       string     `json:"kyc_status" validate:"required,oneof=pending approved rejected"`                // pending, approved, rejected
	Role            string     `json:"role" gorm:"default:user" validate:"required,oneof=user admin support auditor"` // user, admin, support, auditor, etc.
	Tier            string     `json:"tier" gorm:"default:basic" validate:"required,oneof=basic premium vip"`         // basic, premium, vip - for rate limiting tiers
	MFAEnabled      bool       `json:"mfa_enabled"`
	TOTPSecret      string     `json:"-" gorm:"column:totp_secret" validate:"omitempty,base32"`
	LastLogin       time.Time  `json:"last_login"`
	LastMFA         time.Time  `json:"last_mfa"`
	TrustedDevices  string     `json:"trusted_devices" gorm:"type:text" validate:"omitempty,json"`         // JSON array of device fingerprints
	ParentAccountID *uuid.UUID `json:"parent_account_id" gorm:"type:uuid;index" validate:"omitempty,uuid"` // for sub-account linkage
	RBAC            string     `json:"rbac" gorm:"type:text" validate:"omitempty,json"`                    // JSON: roles/permissions
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// Account represents a user's account for a specific currency
type Account struct {
	ID        uuid.UUID       `json:"id" gorm:"primaryKey;type:uuid" validate:"required,uuid"`
	UserID    uuid.UUID       `json:"user_id" gorm:"type:uuid;index" validate:"required,uuid"`
	Currency  string          `json:"currency" validate:"required,currency_code"`
	Balance   decimal.Decimal `json:"balance" validate:"min=0"`
	Available decimal.Decimal `json:"available" validate:"min=0"`
	Locked    decimal.Decimal `json:"locked" validate:"min=0"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// Transaction represents a transaction in the system
type Transaction struct {
	ID          uuid.UUID       `json:"id" gorm:"primaryKey;type:uuid" validate:"required,uuid"`
	UserID      uuid.UUID       `json:"user_id" gorm:"type:uuid;index" validate:"required,uuid"`
	Type        string          `json:"type" validate:"required,oneof=deposit withdrawal trade"` // deposit, withdrawal, trade
	Amount      decimal.Decimal `json:"amount" validate:"required,gt=0"`
	Currency    string          `json:"currency" validate:"required,currency_code"`
	Status      string          `json:"status" validate:"required,oneof=pending completed failed"` // pending, completed, failed
	Reference   string          `json:"reference" validate:"omitempty,max=255,alphanum_hyphen"`
	Description string          `json:"description" validate:"omitempty,max=500"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	CompletedAt *time.Time      `json:"completed_at"`
}

// TransactionEntry represents an entry in a transaction
type TransactionEntry struct {
	ID        uuid.UUID       `json:"id" gorm:"primaryKey;type:uuid" validate:"required,uuid"`
	AccountID uuid.UUID       `json:"account_id" gorm:"type:uuid;index" validate:"required,uuid"`
	Type      string          `json:"type" validate:"required,oneof=credit debit"` // credit, debit
	Amount    decimal.Decimal `json:"amount" validate:"required,gt=0"`
	Currency  string          `json:"currency" validate:"required,currency_code"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// Order represents an order in the system
type Order struct {
	ID          uuid.UUID       `json:"id" gorm:"primaryKey;type:uuid" validate:"required,uuid"`
	UserID      uuid.UUID       `json:"user_id" gorm:"type:uuid;index" validate:"required,uuid"`
	Symbol      string          `json:"symbol" gorm:"index" validate:"required,trading_pair"`
	Side        string          `json:"side" validate:"required,oneof=buy sell"`                                       // buy, sell
	Type        string          `json:"type" validate:"required,oneof=limit market stop stop-limit trailing-stop OCO"` // limit, market, stop, stop-limit, trailing-stop, OCO, etc.
	Price       decimal.Decimal `json:"price" validate:"omitempty,gt=0"`
	Quantity    decimal.Decimal `json:"quantity" validate:"required,gt=0"`
	TimeInForce string          `json:"time_in_force" validate:"required,oneof=GTC IOC FOK"`                                      // GTC, IOC, FOK
	Status      string          `json:"status" validate:"required,oneof=new partially_filled filled canceled rejected triggered"` // new, partially_filled, filled, canceled, rejected, triggered
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	FilledAt    *time.Time      `json:"filled_at"`

	// Additional fields for trading engine
	FilledQuantity  decimal.Decimal  `json:"filled_quantity" validate:"min=0"`
	StopPrice       *decimal.Decimal `json:"stop_price,omitempty" validate:"omitempty,gt=0"`
	AveragePrice    *decimal.Decimal `json:"average_price,omitempty" validate:"omitempty,gt=0"`
	DisplayQuantity *decimal.Decimal `json:"display_quantity,omitempty" validate:"omitempty,gt=0"` // for iceberg orders
	ExpiresAt       *time.Time       `json:"expires_at,omitempty"`                                 // for GTD orders

	// Advanced order fields
	TriggerPrice   *decimal.Decimal `json:"trigger_price,omitempty" validate:"omitempty,gt=0"`   // for stop/triggered orders
	TrailingOffset *decimal.Decimal `json:"trailing_offset,omitempty" validate:"omitempty,gt=0"` // for trailing stops
	OCOGroupID     *uuid.UUID       `json:"oco_group_id,omitempty" validate:"omitempty,uuid"`    // for OCO linkage
	ReduceOnly     *bool            `json:"reduce_only,omitempty"`
	PostOnly       *bool            `json:"post_only,omitempty"`
	ParentOrderID  *uuid.UUID       `json:"parent_order_id,omitempty" validate:"omitempty,uuid"` // for OCO/linked orders
}

// Trade represents a trade in the system
type Trade struct {
	ID             uuid.UUID       `json:"id" gorm:"primaryKey;type:uuid" validate:"required,uuid"`
	OrderID        uuid.UUID       `json:"order_id" gorm:"type:uuid;index" validate:"required,uuid"`
	CounterOrderID uuid.UUID       `json:"counter_order_id" gorm:"type:uuid;index" validate:"required,uuid"`
	UserID         uuid.UUID       `json:"user_id" gorm:"type:uuid;index" validate:"required,uuid"`
	CounterUserID  uuid.UUID       `json:"counter_user_id" gorm:"type:uuid;index" validate:"required,uuid"`
	Symbol         string          `json:"symbol" gorm:"index" validate:"required,trading_pair"`
	Side           string          `json:"side" validate:"required,oneof=buy sell"` // buy, sell
	Price          decimal.Decimal `json:"price" validate:"required,gt=0"`
	Quantity       decimal.Decimal `json:"quantity" validate:"required,gt=0"`
	Fee            decimal.Decimal `json:"fee" validate:"min=0"`
	FeeCurrency    string          `json:"fee_currency" validate:"required,currency_code"`
	IsMaker        bool            `json:"is_maker"` // whether this trade was maker or taker
	CreatedAt      time.Time       `json:"created_at"`
}

// TradingPair represents a trading pair in the system
type TradingPair struct {
	ID               uuid.UUID       `json:"id" gorm:"primaryKey;type:uuid" validate:"required,uuid"`
	Symbol           string          `json:"symbol" gorm:"uniqueIndex" validate:"required,trading_pair"`
	BaseCurrency     string          `json:"base_currency" validate:"required,currency_code"`
	QuoteCurrency    string          `json:"quote_currency" validate:"required,currency_code"`
	PriceDecimals    int             `json:"price_decimals" validate:"min=0,max=18"`
	QuantityDecimals int             `json:"quantity_decimals" validate:"min=0,max=18"`
	MinQuantity      decimal.Decimal `json:"min_quantity" validate:"gt=0"`
	MaxQuantity      decimal.Decimal `json:"max_quantity" validate:"gt=0"`
	MinPrice         decimal.Decimal `json:"min_price" validate:"gt=0"`
	MaxPrice         decimal.Decimal `json:"max_price" validate:"gt=0"`
	Status           TradingStatus   `json:"status" validate:"required,oneof=active inactive"` // active, inactive
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

// MarketPrice represents a market price for a trading pair
type MarketPrice struct {
	Symbol    string          `json:"symbol" gorm:"primaryKey" validate:"required,trading_pair"`
	Price     decimal.Decimal `json:"price" validate:"required,gt=0"`
	Change24h decimal.Decimal `json:"change_24h"`
	Volume24h decimal.Decimal `json:"volume_24h" validate:"min=0"`
	High24h   decimal.Decimal `json:"high_24h" validate:"min=0"`
	Low24h    decimal.Decimal `json:"low_24h" validate:"min=0"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// Candle represents a candle for a trading pair
type Candle struct {
	Timestamp time.Time       `json:"timestamp" validate:"required"`
	Open      decimal.Decimal `json:"open" validate:"required,gt=0"`
	High      decimal.Decimal `json:"high" validate:"required,gt=0"`
	Low       decimal.Decimal `json:"low" validate:"required,gt=0"`
	Close     decimal.Decimal `json:"close" validate:"required,gt=0"`
	Volume    decimal.Decimal `json:"volume" validate:"min=0"`
}

// OrderBookLevel represents a level in the order book
type OrderBookLevel struct {
	Price  decimal.Decimal `json:"price" validate:"required,gt=0"`
	Volume decimal.Decimal `json:"volume" validate:"required,gt=0"`
}

// OrderBookSnapshot represents a snapshot of the order book
type OrderBookSnapshot struct {
	Symbol     string           `json:"symbol" validate:"required,trading_pair"`
	Bids       []OrderBookLevel `json:"bids" validate:"dive"`
	Asks       []OrderBookLevel `json:"asks" validate:"dive"`
	UpdateTime time.Time        `json:"update_time" validate:"required"`
}

// Deposit represents a deposit in the system
type Deposit struct {
	ID          uuid.UUID       `json:"id" gorm:"primaryKey;type:uuid" validate:"required,uuid"`
	UserID      uuid.UUID       `json:"user_id" gorm:"type:uuid;index" validate:"required,uuid"`
	Currency    string          `json:"currency" validate:"required,currency_code"`
	Amount      decimal.Decimal `json:"amount" validate:"required,gt=0"`
	Status      string          `json:"status" validate:"required,oneof=pending completed failed"` // pending, completed, failed
	TxHash      string          `json:"tx_hash" validate:"omitempty,min=10,max=128,alphanum"`
	Network     string          `json:"network" validate:"required,oneof=bitcoin ethereum polygon bsc tron"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	ConfirmedAt *time.Time      `json:"confirmed_at"`
}

// Withdrawal represents a withdrawal in the system
type Withdrawal struct {
	ID            uuid.UUID       `json:"id" gorm:"primaryKey;type:uuid" validate:"required,uuid"`
	UserID        uuid.UUID       `json:"user_id" gorm:"type:uuid;index" validate:"required,uuid"`
	Currency      string          `json:"currency" validate:"required,currency_code"`
	Amount        decimal.Decimal `json:"amount" validate:"required,gt=0"`
	Fee           decimal.Decimal `json:"fee" validate:"min=0"`
	Status        string          `json:"status" validate:"required,oneof=pending completed failed"` // pending, completed, failed
	TransactionID *uuid.UUID      `json:"transaction_id" gorm:"type:uuid" validate:"omitempty,uuid"`
	Address       string          `json:"address" validate:"required,min=10,max=100"`
	Network       string          `json:"network" validate:"required,oneof=bitcoin ethereum polygon bsc tron"`
	TxHash        string          `json:"tx_hash" validate:"omitempty,min=10,max=128,alphanum"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
	ProcessedAt   *time.Time      `json:"processed_at"`
}

// APIKey represents an API key in the system
type APIKey struct {
	ID          uuid.UUID  `json:"id" gorm:"primaryKey;type:uuid" validate:"required,uuid"`
	UserID      uuid.UUID  `json:"user_id" gorm:"type:uuid;index" validate:"required,uuid"`
	Name        string     `json:"name" validate:"required,min=1,max=100,alphanum_space"`
	Key         string     `json:"key" gorm:"uniqueIndex" validate:"required,min=32,max=128,alphanum"`
	SecretHash  string     `json:"-" gorm:"column:secret_hash" validate:"required,min=60"`
	Permissions string     `json:"permissions" validate:"required,json"`
	IPWhitelist string     `json:"ip_whitelist" validate:"omitempty,ip_list"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	LastUsedAt  *time.Time `json:"last_used_at"`
}

// RegisterRequest represents a user registration request
type RegisterRequest struct {
	Email     string `json:"email" binding:"required,email" validate:"required,email,max=254"`
	Username  string `json:"username" binding:"required,min=3,max=30" validate:"required,min=3,max=30,alphanum"`
	Password  string `json:"password" binding:"required,min=8" validate:"required,min=8,max=128"`
	FirstName string `json:"first_name" binding:"required" validate:"required,min=1,max=50,alpha_space"`
	LastName  string `json:"last_name" binding:"required" validate:"required,min=1,max=50,alpha_space"`
}

// LoginRequest represents a user login request
type LoginRequest struct {
	Login    string `json:"login" binding:"required" validate:"required,max=254"` // email or username
	Password string `json:"password" binding:"required" validate:"required,min=8,max=128"`
}

// LoginResponse represents a user login response
type LoginResponse struct {
	User        *User     `json:"user,omitempty"`
	Token       string    `json:"token,omitempty" validate:"omitempty,jwt"`
	Requires2FA bool      `json:"requires_2fa"`
	UserID      uuid.UUID `json:"user_id,omitempty" validate:"omitempty,uuid"`
}

// TwoFAVerifyRequest represents a 2FA verification request
type TwoFAVerifyRequest struct {
	Secret string `json:"secret" binding:"required" validate:"required,base32"`
	Token  string `json:"token" binding:"required" validate:"required,len=6,numeric"`
}

// OrderRequest represents an order request
type OrderRequest struct {
	Symbol      string          `json:"symbol" binding:"required" validate:"required,trading_pair"`
	Side        string          `json:"side" binding:"required,oneof=buy sell" validate:"required,oneof=buy sell"`
	Type        string          `json:"type" binding:"required,oneof=limit market" validate:"required,oneof=limit market stop stop-limit"`
	Price       decimal.Decimal `json:"price" validate:"omitempty,gt=0"`
	Quantity    decimal.Decimal `json:"quantity" binding:"required,gt=0" validate:"required,gt=0"`
	TimeInForce string          `json:"time_in_force" binding:"required,oneof=GTC IOC FOK" validate:"required,oneof=GTC IOC FOK"`
}

// DepositRequest represents a deposit request
type DepositRequest struct {
	Currency string          `json:"currency" binding:"required" validate:"required,currency_code"`
	Amount   decimal.Decimal `json:"amount" binding:"required,gt=0" validate:"required,gt=0"`
	Provider string          `json:"provider" binding:"required" validate:"required,oneof=bank_transfer credit_card crypto_wallet"`
}

// WithdrawalRequest represents a withdrawal request
type WithdrawalRequest struct {
	ID        uuid.UUID       `json:"id" gorm:"primaryKey;type:uuid" validate:"required,uuid"`
	UserID    uuid.UUID       `json:"user_id" gorm:"type:uuid;index" validate:"required,uuid"`
	WalletID  string          `json:"wallet_id" validate:"required,alphanum"`
	Asset     string          `json:"asset" validate:"required,currency_code"`
	Amount    decimal.Decimal `json:"amount" validate:"required,gt=0"`
	ToAddress string          `json:"to_address" validate:"required,min=10,max=100"`
	Status    string          `json:"status" validate:"required,oneof=pending approved rejected broadcasted"` // pending, approved, rejected, broadcasted
	Approvals []Approval      `json:"approvals" gorm:"foreignKey:RequestID" validate:"dive"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

type Approval struct {
	ID        uuid.UUID `json:"id" gorm:"primaryKey;type:uuid" validate:"required,uuid"`
	RequestID uuid.UUID `json:"request_id" gorm:"type:uuid;index" validate:"required,uuid"`
	Approver  string    `json:"approver" validate:"required,min=1,max=100"`
	Approved  bool      `json:"approved"`
	Timestamp time.Time `json:"timestamp"`
}

type WalletAudit struct {
	ID        uuid.UUID `json:"id" gorm:"primaryKey;type:uuid" validate:"required,uuid"`
	WalletID  string    `json:"wallet_id" validate:"required,alphanum"`
	Event     string    `json:"event" validate:"required,min=1,max=100"`
	Actor     string    `json:"actor" validate:"required,min=1,max=100"`
	Details   string    `json:"details" validate:"omitempty,max=1000"`
	CreatedAt time.Time `json:"created_at"`
}

// OrderFilter represents filters for listing orders
// Used in API query params for /orders endpoints
// Add more fields as needed for advanced filtering
// e.g. by date, price range, etc.
type OrderFilter struct {
	Status string `form:"status" json:"status" validate:"omitempty,oneof=new filled canceled partially_filled rejected triggered"` // new, filled, canceled, etc.
	Type   string `form:"type" json:"type" validate:"omitempty,oneof=limit market stop stop-limit"`                                // limit, market, stop, etc.
	Symbol string `form:"symbol" json:"symbol" validate:"omitempty,trading_pair"`
}

// KYCDocument represents a KYC document for a user
// This is a minimal placeholder; expand as needed for your KYC logic
// e.g. add fields for document type, status, file references, etc.
type KYCDocument struct {
	ID        uuid.UUID `json:"id" gorm:"primaryKey;type:uuid" validate:"required,uuid"`
	UserID    uuid.UUID `json:"user_id" gorm:"type:uuid;index" validate:"required,uuid"`
	DocType   string    `json:"doc_type" validate:"required,oneof=passport drivers_license national_id utility_bill"`
	DocNumber string    `json:"doc_number" validate:"required,min=5,max=50,alphanum_hyphen"`
	Status    string    `json:"status" validate:"required,oneof=pending approved rejected"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// InstitutionalAccount represents a master account for institutions
// Supports sub-accounts, RBAC, and config
// Each sub-account is a User with ParentAccountID set

type InstitutionalAccount struct {
	ID           uuid.UUID `json:"id" gorm:"primaryKey;type:uuid" validate:"required,uuid"`
	Name         string    `json:"name" validate:"required,min=1,max=200,alpha_space"`
	MasterUserID uuid.UUID `json:"master_user_id" gorm:"type:uuid;index" validate:"required,uuid"`
	Config       string    `json:"config" gorm:"type:text" validate:"omitempty,json"` // JSON: limits, fee tiers, etc.
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// AuditLog for institutional actions
type AuditLog struct {
	ID        uuid.UUID `json:"id" gorm:"primaryKey;type:uuid" validate:"required,uuid"`
	UserID    uuid.UUID `json:"user_id" gorm:"type:uuid;index" validate:"required,uuid"`
	Action    string    `json:"action" validate:"required,min=1,max=100"`
	Details   string    `json:"details" gorm:"type:text" validate:"omitempty,max=2000"`
	CreatedAt time.Time `json:"created_at"`
}

// UserTier represents rate limiting tiers
type UserTier string

const (
	TierBasic   UserTier = "basic"
	TierPremium UserTier = "premium"
	TierVIP     UserTier = "vip"
)

// TierRateLimits defines rate limits for each tier
type TierRateLimits struct {
	APICallsPerMinute    int `json:"api_calls_per_minute"`
	OrdersPerMinute      int `json:"orders_per_minute"`
	TradesPerMinute      int `json:"trades_per_minute"`
	WithdrawalsPerDay    int `json:"withdrawals_per_day"`
	LoginAttemptsPerHour int `json:"login_attempts_per_hour"`
}

// GetTierLimits returns rate limits for a specific tier
func GetTierLimits(tier UserTier) TierRateLimits {
	switch tier {
	case TierBasic:
		return TierRateLimits{
			APICallsPerMinute:    10,
			OrdersPerMinute:      5,
			TradesPerMinute:      3,
			WithdrawalsPerDay:    1,
			LoginAttemptsPerHour: 5,
		}
	case TierPremium:
		return TierRateLimits{
			APICallsPerMinute:    100,
			OrdersPerMinute:      50,
			TradesPerMinute:      30,
			WithdrawalsPerDay:    10,
			LoginAttemptsPerHour: 10,
		}
	case TierVIP:
		return TierRateLimits{
			APICallsPerMinute:    1000,
			OrdersPerMinute:      500,
			TradesPerMinute:      300,
			WithdrawalsPerDay:    100,
			LoginAttemptsPerHour: 20,
		}
	default:
		return GetTierLimits(TierBasic)
	}
}

// RateLimitInfo contains detailed rate limit information
type RateLimitInfo struct {
	Limit     int           `json:"limit"`
	Used      int           `json:"used"`
	Remaining int           `json:"remaining"`
	ResetAt   time.Time     `json:"reset_at"`
	Window    time.Duration `json:"window"`
}

// UpdateUserRequest represents a request to update user profile fields
// swagger:model
// @name UpdateUserRequest
// @description Request payload for updating user profile
// @example {"first_name": "John", "last_name": "Doe"}
type UpdateUserRequest struct {
	FirstName string `json:"first_name" example:"John"`
	LastName  string `json:"last_name" example:"Doe"`
}

// KYCSubmissionRequest represents a request to submit KYC documents
// swagger:model
// @name KYCSubmissionRequest
// @description Request payload for submitting KYC documents
// @example {"document_type": "passport", "document_number": "123456789", "country": "US"}
type KYCSubmissionRequest struct {
	DocumentType   string `json:"document_type" example:"passport"`
	DocumentNumber string `json:"document_number" example:"123456789"`
	Country        string `json:"country" example:"US"`
}

// FiatWithdrawRequest represents a fiat withdrawal request
// swagger:model
// @name FiatWithdrawRequest
// @description Request payload for fiat withdrawal
// @example {"amount": "100.00", "currency": "USD", "bank_account": "US1234567890"}
type FiatWithdrawRequest struct {
	Amount      decimal.Decimal `json:"amount" example:"100.00"`
	Currency    string          `json:"currency" example:"USD"`
	BankAccount string          `json:"bank_account" example:"US1234567890"`
}

// FiatDepositRequest represents a fiat deposit request
// swagger:model
// @name FiatDepositRequest
// @description Request payload for fiat deposit
// @example {"amount": "100.00", "currency": "USD", "bank_account": "US1234567890"}
type FiatDepositRequest struct {
	Amount      decimal.Decimal `json:"amount" example:"100.00"`
	Currency    string          `json:"currency" example:"USD"`
	BankAccount string          `json:"bank_account" example:"US1234567890"`
}

// RefreshTokenRequest represents a refresh token request
// swagger:model
// @name RefreshTokenRequest
// @description Request payload for refreshing JWT tokens
// @example {"refresh_token": "abc123refresh"}
type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" example:"abc123refresh"`
}
