package models

import (
	"time"

	"github.com/google/uuid"
)

// User represents a user in the system
type User struct {
	ID           uuid.UUID `json:"id" gorm:"primaryKey;type:uuid"`
	Email        string    `json:"email" gorm:"uniqueIndex"`
	Username     string    `json:"username" gorm:"uniqueIndex"`
	PasswordHash string    `json:"-" gorm:"column:password_hash"`
	FirstName    string    `json:"first_name"`
	LastName     string    `json:"last_name"`
	KYCStatus    string    `json:"kyc_status"` // pending, approved, rejected
	Role          string    `json:"role" gorm:"default:user"` // user, admin, support, auditor, etc.
	MFAEnabled    bool      `json:"mfa_enabled"`
	TOTPSecret    string    `json:"-" gorm:"column:totp_secret"`
	LastLogin     time.Time `json:"last_login"`
	LastMFA       time.Time `json:"last_mfa"`
	TrustedDevices string   `json:"trusted_devices" gorm:"type:text"` // JSON array of device fingerprints
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Account represents a user's account for a specific currency
type Account struct {
	ID        uuid.UUID `json:"id" gorm:"primaryKey;type:uuid"`
	UserID    uuid.UUID `json:"user_id" gorm:"type:uuid;index"`
	Currency  string    `json:"currency"`
	Balance   float64   `json:"balance"`
	Available float64   `json:"available"`
	Locked    float64   `json:"locked"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Transaction represents a transaction in the system
type Transaction struct {
	ID          uuid.UUID  `json:"id" gorm:"primaryKey;type:uuid"`
	UserID      uuid.UUID  `json:"user_id" gorm:"type:uuid;index"`
	Type        string     `json:"type"` // deposit, withdrawal, trade
	Amount      float64    `json:"amount"`
	Currency    string     `json:"currency"`
	Status      string     `json:"status"` // pending, completed, failed
	Reference   string     `json:"reference"`
	Description string     `json:"description"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	CompletedAt *time.Time `json:"completed_at"`
}

// TransactionEntry represents an entry in a transaction
type TransactionEntry struct {
	ID        uuid.UUID `json:"id" gorm:"primaryKey;type:uuid"`
	AccountID uuid.UUID `json:"account_id" gorm:"type:uuid;index"`
	Type      string    `json:"type"` // credit, debit
	Amount    float64   `json:"amount"`
	Currency  string    `json:"currency"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Order represents an order in the system
type Order struct {
	ID          uuid.UUID  `json:"id" gorm:"primaryKey;type:uuid"`
	UserID      uuid.UUID  `json:"user_id" gorm:"type:uuid;index"`
	Symbol      string     `json:"symbol" gorm:"index"`
	Side        string     `json:"side"` // buy, sell
	Type        string     `json:"type"` // limit, market, stop, stop-limit, trailing-stop, OCO, etc.
	Price       float64    `json:"price"`
	Quantity    float64    `json:"quantity"`
	TimeInForce string     `json:"time_in_force"` // GTC, IOC, FOK
	Status      string     `json:"status"`        // new, partially_filled, filled, canceled, rejected, triggered
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	FilledAt    *time.Time `json:"filled_at"`

	// Advanced order fields
	TriggerPrice   *float64   `json:"trigger_price,omitempty"`   // for stop/triggered orders
	TrailingOffset *float64   `json:"trailing_offset,omitempty"` // for trailing stops
	OCOGroupID     *uuid.UUID `json:"oco_group_id,omitempty"`    // for OCO linkage
	ReduceOnly     *bool      `json:"reduce_only,omitempty"`
	PostOnly       *bool      `json:"post_only,omitempty"`
	ParentOrderID  *uuid.UUID `json:"parent_order_id,omitempty"` // for OCO/linked orders
}

// Trade represents a trade in the system
type Trade struct {
	ID             uuid.UUID `json:"id" gorm:"primaryKey;type:uuid"`
	OrderID        uuid.UUID `json:"order_id" gorm:"type:uuid;index"`
	CounterOrderID uuid.UUID `json:"counter_order_id" gorm:"type:uuid;index"`
	UserID         uuid.UUID `json:"user_id" gorm:"type:uuid;index"`
	CounterUserID  uuid.UUID `json:"counter_user_id" gorm:"type:uuid;index"`
	Symbol         string    `json:"symbol" gorm:"index"`
	Side           string    `json:"side"` // buy, sell
	Price          float64   `json:"price"`
	Quantity       float64   `json:"quantity"`
	Fee            float64   `json:"fee"`
	FeeCurrency    string    `json:"fee_currency"`
	CreatedAt      time.Time `json:"created_at"`
}

// TradingPair represents a trading pair in the system
type TradingPair struct {
	ID               uuid.UUID `json:"id" gorm:"primaryKey;type:uuid"`
	Symbol           string    `json:"symbol" gorm:"uniqueIndex"`
	BaseCurrency     string    `json:"base_currency"`
	QuoteCurrency    string    `json:"quote_currency"`
	PriceDecimals    int       `json:"price_decimals"`
	QuantityDecimals int       `json:"quantity_decimals"`
	MinQuantity      float64   `json:"min_quantity"`
	MaxQuantity      float64   `json:"max_quantity"`
	MinPrice         float64   `json:"min_price"`
	MaxPrice         float64   `json:"max_price"`
	Status           string    `json:"status"` // active, inactive
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// MarketPrice represents a market price for a trading pair
type MarketPrice struct {
	Symbol    string    `json:"symbol" gorm:"primaryKey"`
	Price     float64   `json:"price"`
	Change24h float64   `json:"change_24h"`
	Volume24h float64   `json:"volume_24h"`
	High24h   float64   `json:"high_24h"`
	Low24h    float64   `json:"low_24h"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Candle represents a candle for a trading pair
type Candle struct {
	Timestamp time.Time `json:"timestamp"`
	Open      float64   `json:"open"`
	High      float64   `json:"high"`
	Low       float64   `json:"low"`
	Close     float64   `json:"close"`
	Volume    float64   `json:"volume"`
}

// OrderBookLevel represents a level in the order book
type OrderBookLevel struct {
	Price  float64 `json:"price"`
	Volume float64 `json:"volume"`
}

// OrderBookSnapshot represents a snapshot of the order book
type OrderBookSnapshot struct {
	Symbol     string           `json:"symbol"`
	Bids       []OrderBookLevel `json:"bids"`
	Asks       []OrderBookLevel `json:"asks"`
	UpdateTime time.Time        `json:"update_time"`
}

// Deposit represents a deposit in the system
type Deposit struct {
	ID          uuid.UUID  `json:"id" gorm:"primaryKey;type:uuid"`
	UserID      uuid.UUID  `json:"user_id" gorm:"type:uuid;index"`
	Currency    string     `json:"currency"`
	Amount      float64    `json:"amount"`
	Status      string     `json:"status"` // pending, completed, failed
	TxHash      string     `json:"tx_hash"`
	Network     string     `json:"network"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	ConfirmedAt *time.Time `json:"confirmed_at"`
}

// Withdrawal represents a withdrawal in the system
type Withdrawal struct {
	ID            uuid.UUID  `json:"id" gorm:"primaryKey;type:uuid"`
	UserID        uuid.UUID  `json:"user_id" gorm:"type:uuid;index"`
	Currency      string     `json:"currency"`
	Amount        float64    `json:"amount"`
	Fee           float64    `json:"fee"`
	Status        string     `json:"status"` // pending, completed, failed
	TransactionID *uuid.UUID `json:"transaction_id" gorm:"type:uuid"`
	Address       string     `json:"address"`
	Network       string     `json:"network"`
	TxHash        string     `json:"tx_hash"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	ProcessedAt   *time.Time `json:"processed_at"`
}

// APIKey represents an API key in the system
type APIKey struct {
	ID          uuid.UUID  `json:"id" gorm:"primaryKey;type:uuid"`
	UserID      uuid.UUID  `json:"user_id" gorm:"type:uuid;index"`
	Name        string     `json:"name"`
	Key         string     `json:"key" gorm:"uniqueIndex"`
	SecretHash  string     `json:"-" gorm:"column:secret_hash"`
	Permissions string     `json:"permissions"`
	IPWhitelist string     `json:"ip_whitelist"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	LastUsedAt  *time.Time `json:"last_used_at"`
}

// RegisterRequest represents a user registration request
type RegisterRequest struct {
	Email     string `json:"email" binding:"required,email"`
	Username  string `json:"username" binding:"required,min=3,max=30"`
	Password  string `json:"password" binding:"required,min=8"`
	FirstName string `json:"first_name" binding:"required"`
	LastName  string `json:"last_name" binding:"required"`
}

// LoginRequest represents a user login request
type LoginRequest struct {
	Login    string `json:"login" binding:"required"` // email or username
	Password string `json:"password" binding:"required"`
}

// LoginResponse represents a user login response
type LoginResponse struct {
	User        *User     `json:"user,omitempty"`
	Token       string    `json:"token,omitempty"`
	Requires2FA bool      `json:"requires_2fa"`
	UserID      uuid.UUID `json:"user_id,omitempty"`
}

// TwoFAVerifyRequest represents a 2FA verification request
type TwoFAVerifyRequest struct {
	Secret string `json:"secret" binding:"required"`
	Token  string `json:"token" binding:"required"`
}

// OrderRequest represents an order request
type OrderRequest struct {
	Symbol      string  `json:"symbol" binding:"required"`
	Side        string  `json:"side" binding:"required,oneof=buy sell"`
	Type        string  `json:"type" binding:"required,oneof=limit market"`
	Price       float64 `json:"price"`
	Quantity    float64 `json:"quantity" binding:"required,gt=0"`
	TimeInForce string  `json:"time_in_force" binding:"required,oneof=GTC IOC FOK"`
}

// DepositRequest represents a deposit request
type DepositRequest struct {
	Currency string  `json:"currency" binding:"required"`
	Amount   float64 `json:"amount" binding:"required,gt=0"`
	Provider string  `json:"provider" binding:"required"`
}

// WithdrawalRequest represents a withdrawal request
type WithdrawalRequest struct {
	ID        uuid.UUID  `json:"id" gorm:"primaryKey;type:uuid"`
	UserID    uuid.UUID  `json:"user_id" gorm:"type:uuid;index"`
	WalletID  string     `json:"wallet_id"`
	Asset     string     `json:"asset"`
	Amount    float64    `json:"amount"`
	ToAddress string     `json:"to_address"`
	Status    string     `json:"status"` // pending, approved, rejected, broadcasted
	Approvals []Approval `json:"approvals" gorm:"foreignKey:RequestID"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

type Approval struct {
	ID        uuid.UUID `json:"id" gorm:"primaryKey;type:uuid"`
	RequestID uuid.UUID `json:"request_id" gorm:"type:uuid;index"`
	Approver  string    `json:"approver"`
	Approved  bool      `json:"approved"`
	Timestamp time.Time `json:"timestamp"`
}

type WalletAudit struct {
	ID        uuid.UUID `json:"id" gorm:"primaryKey;type:uuid"`
	WalletID  string    `json:"wallet_id"`
	Event     string    `json:"event"`
	Actor     string    `json:"actor"`
	Details   string    `json:"details"`
	CreatedAt time.Time `json:"created_at"`
}

// OrderFilter represents filters for listing orders
// Used in API query params for /orders endpoints
// Add more fields as needed for advanced filtering
// e.g. by date, price range, etc.
type OrderFilter struct {
	Status string `form:"status" json:"status"` // new, filled, canceled, etc.
	Type   string `form:"type" json:"type"`     // limit, market, stop, etc.
	Symbol string `form:"symbol" json:"symbol"`
}

// KYCDocument represents a KYC document for a user
// This is a minimal placeholder; expand as needed for your KYC logic
// e.g. add fields for document type, status, file references, etc.
type KYCDocument struct {
	ID        uuid.UUID `json:"id" gorm:"primaryKey;type:uuid"`
	UserID    uuid.UUID `json:"user_id" gorm:"type:uuid;index"`
	DocType   string    `json:"doc_type"`
	DocNumber string    `json:"doc_number"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
