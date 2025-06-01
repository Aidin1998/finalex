package docs

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// HealthResponse represents the health check response
type HealthResponse struct {
	Status    string    `json:"status" example:"ok"`
	Timestamp time.Time `json:"timestamp" example:"2025-06-01T12:00:00Z"`
	Version   string    `json:"version" example:"2.0.0"`
	Uptime    string    `json:"uptime" example:"24h30m15s"`
}

// ErrorResponse represents a standard error response
type ErrorResponse struct {
	Error     string    `json:"error" example:"Invalid request"`
	Details   string    `json:"details,omitempty" example:"Field 'amount' is required"`
	Code      string    `json:"code,omitempty" example:"VALIDATION_ERROR"`
	Timestamp time.Time `json:"timestamp" example:"2025-06-01T12:00:00Z"`
	TraceID   string    `json:"trace_id,omitempty" example:"abc123"`
}

// ValidationError represents field validation errors
type ValidationError struct {
	Field   string `json:"field" example:"email" description:"Field name that failed validation"`
	Tag     string `json:"tag" example:"required" description:"Validation rule that failed"`
	Value   string `json:"value" example:"invalid-email" description:"Value that failed validation"`
	Message string `json:"message" example:"Email format is invalid" description:"Human-readable error message"`
} // @name ValidationError

// ValidationErrorResponse represents validation error response
type ValidationErrorResponse struct {
	Error   string            `json:"error" example:"Validation failed"`
	Details []ValidationError `json:"details" description:"List of validation errors"`
} // @name ValidationErrorResponse

// PaginationMeta represents pagination metadata
type PaginationMeta struct {
	Page       int `json:"page" example:"1" description:"Current page number"`
	PerPage    int `json:"per_page" example:"20" description:"Items per page"`
	Total      int `json:"total" example:"100" description:"Total number of items"`
	TotalPages int `json:"total_pages" example:"5" description:"Total number of pages"`
} // @name PaginationMeta

// User represents user information
type User struct {
	ID           uuid.UUID  `json:"id" example:"123e4567-e89b-12d3-a456-426614174000" description:"User unique identifier"`
	Email        string     `json:"email" example:"user@example.com" description:"User email address"`
	Username     string     `json:"username" example:"trader123" description:"User username"`
	FirstName    string     `json:"first_name" example:"John" description:"User first name"`
	LastName     string     `json:"last_name" example:"Doe" description:"User last name"`
	Role         string     `json:"role" example:"user" enums:"user,admin,premium,vip" description:"User role"`
	Status       string     `json:"status" example:"active" enums:"active,inactive,suspended,pending" description:"User account status"`
	Tier         string     `json:"tier" example:"basic" enums:"basic,premium,vip,institutional" description:"User tier for rate limiting"`
	KYCStatus    string     `json:"kyc_status" example:"verified" enums:"pending,verified,rejected,expired" description:"KYC verification status"`
	TwoFAEnabled bool       `json:"two_fa_enabled" example:"true" description:"Whether 2FA is enabled"`
	CreatedAt    time.Time  `json:"created_at" example:"2025-06-01T12:00:00Z" description:"Account creation timestamp"`
	UpdatedAt    time.Time  `json:"updated_at" example:"2025-06-01T12:00:00Z" description:"Last update timestamp"`
	LastLoginAt  *time.Time `json:"last_login_at,omitempty" example:"2025-06-01T12:00:00Z" description:"Last login timestamp"`
} // @name User

// LoginRequest represents login request payload
type LoginRequest struct {
	Email     string `json:"email" example:"user@example.com" validate:"required,email" description:"User email address"`
	Password  string `json:"password" example:"SecurePassword123!" validate:"required,min=8" description:"User password"`
	TwoFACode string `json:"two_fa_code,omitempty" example:"123456" validate:"omitempty,len=6,numeric" description:"2FA code if enabled"`
} // @name LoginRequest

// LoginResponse represents login response
type LoginResponse struct {
	AccessToken  string    `json:"access_token" example:"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..." description:"JWT access token"`
	RefreshToken string    `json:"refresh_token" example:"abc123refresh" description:"Refresh token for obtaining new access tokens"`
	ExpiresAt    time.Time `json:"expires_at" example:"2025-06-01T13:00:00Z" description:"Token expiration timestamp"`
	User         User      `json:"user" description:"User information"`
} // @name LoginResponse

// RegisterRequest represents registration request payload
type RegisterRequest struct {
	Email           string `json:"email" example:"user@example.com" validate:"required,email" description:"User email address"`
	Username        string `json:"username" example:"trader123" validate:"required,min=3,max=50,alphanum_hyphen" description:"Username (3-50 characters, alphanumeric with hyphens)"`
	Password        string `json:"password" example:"SecurePassword123!" validate:"required,min=8" description:"Password (minimum 8 characters)"`
	ConfirmPassword string `json:"confirm_password" example:"SecurePassword123!" validate:"required,eqfield=Password" description:"Password confirmation"`
	FirstName       string `json:"first_name" example:"John" validate:"required,alpha_space" description:"First name"`
	LastName        string `json:"last_name" example:"Doe" validate:"required,alpha_space" description:"Last name"`
	AcceptTerms     bool   `json:"accept_terms" example:"true" validate:"required,eq=true" description:"Must accept terms of service"`
} // @name RegisterRequest

// RefreshTokenRequest represents refresh token request
type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" example:"abc123refresh" validate:"required" description:"Refresh token"`
} // @name RefreshTokenRequest

// TwoFAEnableRequest represents 2FA enable request
type TwoFAEnableRequest struct {
	Password string `json:"password" example:"SecurePassword123!" validate:"required" description:"Current password for verification"`
} // @name TwoFAEnableRequest

// TwoFAEnableResponse represents 2FA enable response with QR code
type TwoFAEnableResponse struct {
	Secret      string   `json:"secret" example:"JBSWY3DPEHPK3PXP" description:"TOTP secret key"`
	QRCodeURL   string   `json:"qr_code_url" example:"otpauth://totp/PinEx:user@example.com?secret=JBSWY3DPEHPK3PXP&issuer=PinEx" description:"QR code URL for authenticator apps"`
	BackupCodes []string `json:"backup_codes" example:"['12345678', '87654321']" description:"Backup codes for recovery"`
} // @name TwoFAEnableResponse

// TwoFAVerifyRequest represents 2FA verification request
type TwoFAVerifyRequest struct {
	Code string `json:"code" example:"123456" validate:"required,len=6,numeric" description:"6-digit TOTP code"`
} // @name TwoFAVerifyRequest

// Account represents user account balance
type Account struct {
	Currency      string          `json:"currency" example:"BTC" description:"Currency symbol"`
	Balance       decimal.Decimal `json:"balance" example:"1.25000000" description:"Available balance"`
	LockedBalance decimal.Decimal `json:"locked_balance" example:"0.05000000" description:"Balance locked in orders"`
	TotalBalance  decimal.Decimal `json:"total_balance" example:"1.30000000" description:"Total balance (available + locked)"`
	UpdatedAt     time.Time       `json:"updated_at" example:"2025-06-01T12:00:00Z" description:"Last balance update"`
} // @name Account

// AccountTransaction represents account transaction history
type AccountTransaction struct {
	ID          uuid.UUID       `json:"id" example:"123e4567-e89b-12d3-a456-426614174000" description:"Transaction ID"`
	Type        string          `json:"type" example:"deposit" enums:"deposit,withdrawal,trade,fee,referral,bonus" description:"Transaction type"`
	Currency    string          `json:"currency" example:"BTC" description:"Currency symbol"`
	Amount      decimal.Decimal `json:"amount" example:"0.50000000" description:"Transaction amount"`
	Fee         decimal.Decimal `json:"fee" example:"0.00100000" description:"Transaction fee"`
	Status      string          `json:"status" example:"completed" enums:"pending,processing,completed,failed,cancelled" description:"Transaction status"`
	Description string          `json:"description" example:"Bitcoin deposit" description:"Transaction description"`
	TxHash      string          `json:"tx_hash,omitempty" example:"abc123hash" description:"Blockchain transaction hash"`
	CreatedAt   time.Time       `json:"created_at" example:"2025-06-01T12:00:00Z" description:"Transaction timestamp"`
	CompletedAt *time.Time      `json:"completed_at,omitempty" example:"2025-06-01T12:05:00Z" description:"Completion timestamp"`
} // @name AccountTransaction

// TradingPair represents a trading pair configuration
type TradingPair struct {
	Symbol                string          `json:"symbol" example:"BTCUSDT" description:"Trading pair symbol"`
	BaseAsset             string          `json:"base_asset" example:"BTC" description:"Base asset symbol"`
	QuoteAsset            string          `json:"quote_asset" example:"USDT" description:"Quote asset symbol"`
	Status                string          `json:"status" example:"active" enums:"active,inactive,maintenance" description:"Trading pair status"`
	MinOrderQuantity      decimal.Decimal `json:"min_order_quantity" example:"0.00100000" description:"Minimum order quantity"`
	MaxOrderQuantity      decimal.Decimal `json:"max_order_quantity" example:"1000.00000000" description:"Maximum order quantity"`
	MinOrderValue         decimal.Decimal `json:"min_order_value" example:"10.00000000" description:"Minimum order value in quote asset"`
	PriceIncrement        decimal.Decimal `json:"price_increment" example:"0.01000000" description:"Minimum price increment"`
	QuantityIncrement     decimal.Decimal `json:"quantity_increment" example:"0.00100000" description:"Minimum quantity increment"`
	MakerFee              decimal.Decimal `json:"maker_fee" example:"0.00100000" description:"Maker fee rate (0.1%)"`
	TakerFee              decimal.Decimal `json:"taker_fee" example:"0.00150000" description:"Taker fee rate (0.15%)"`
	LastPrice             decimal.Decimal `json:"last_price" example:"45000.00000000" description:"Last traded price"`
	Volume24h             decimal.Decimal `json:"volume_24h" example:"150.25000000" description:"24-hour trading volume"`
	PriceChange24h        decimal.Decimal `json:"price_change_24h" example:"1250.00000000" description:"24-hour price change"`
	PriceChangePercent24h decimal.Decimal `json:"price_change_percent_24h" example:"2.85" description:"24-hour price change percentage"`
} // @name TradingPair

// Order represents a trading order
type Order struct {
	ID                uuid.UUID       `json:"id" example:"123e4567-e89b-12d3-a456-426614174000" description:"Order unique identifier"`
	UserID            uuid.UUID       `json:"user_id" example:"123e4567-e89b-12d3-a456-426614174000" description:"User who placed the order"`
	Symbol            string          `json:"symbol" example:"BTCUSDT" description:"Trading pair symbol"`
	Side              string          `json:"side" example:"buy" enums:"buy,sell" description:"Order side"`
	Type              string          `json:"type" example:"limit" enums:"market,limit,stop,stop_limit" description:"Order type"`
	Quantity          decimal.Decimal `json:"quantity" example:"0.10000000" description:"Order quantity"`
	Price             decimal.Decimal `json:"price,omitempty" example:"45000.00000000" description:"Order price (for limit orders)"`
	StopPrice         decimal.Decimal `json:"stop_price,omitempty" example:"44000.00000000" description:"Stop price (for stop orders)"`
	FilledQuantity    decimal.Decimal `json:"filled_quantity" example:"0.05000000" description:"Quantity filled"`
	RemainingQuantity decimal.Decimal `json:"remaining_quantity" example:"0.05000000" description:"Quantity remaining"`
	Status            string          `json:"status" example:"partially_filled" enums:"new,open,partially_filled,filled,cancelled,rejected" description:"Order status"`
	TimeInForce       string          `json:"time_in_force" example:"GTC" enums:"GTC,IOC,FOK,GTD" description:"Time in force"`
	ExpiresAt         *time.Time      `json:"expires_at,omitempty" example:"2025-06-02T12:00:00Z" description:"Order expiration time (for GTD orders)"`
	CreatedAt         time.Time       `json:"created_at" example:"2025-06-01T12:00:00Z" description:"Order creation timestamp"`
	UpdatedAt         time.Time       `json:"updated_at" example:"2025-06-01T12:00:00Z" description:"Last order update timestamp"`
	ClientOrderID     string          `json:"client_order_id,omitempty" example:"user123_order1" description:"Client-provided order ID"`
} // @name Order

// PlaceOrderRequest represents order placement request
type PlaceOrderRequest struct {
	Symbol        string          `json:"symbol" example:"BTCUSDT" validate:"required,trading_pair" description:"Trading pair symbol"`
	Side          string          `json:"side" example:"buy" validate:"required,oneof=buy sell" description:"Order side"`
	Type          string          `json:"type" example:"limit" validate:"required,oneof=market limit stop stop_limit" description:"Order type"`
	Quantity      decimal.Decimal `json:"quantity" example:"0.10000000" validate:"required,gt=0" description:"Order quantity"`
	Price         decimal.Decimal `json:"price,omitempty" example:"45000.00000000" validate:"omitempty,gt=0" description:"Order price (required for limit orders)"`
	StopPrice     decimal.Decimal `json:"stop_price,omitempty" example:"44000.00000000" validate:"omitempty,gt=0" description:"Stop price (required for stop orders)"`
	TimeInForce   string          `json:"time_in_force" example:"GTC" validate:"omitempty,oneof=GTC IOC FOK GTD" description:"Time in force"`
	ExpiresAt     *time.Time      `json:"expires_at,omitempty" example:"2025-06-02T12:00:00Z" description:"Order expiration time (for GTD orders)"`
	ClientOrderID string          `json:"client_order_id,omitempty" example:"user123_order1" validate:"omitempty,max=64" description:"Client-provided order ID"`
} // @name PlaceOrderRequest

// Trade represents a completed trade
type Trade struct {
	ID          uuid.UUID       `json:"id" example:"123e4567-e89b-12d3-a456-426614174000" description:"Trade unique identifier"`
	Symbol      string          `json:"symbol" example:"BTCUSDT" description:"Trading pair symbol"`
	BuyOrderID  uuid.UUID       `json:"buy_order_id" example:"123e4567-e89b-12d3-a456-426614174000" description:"Buy order ID"`
	SellOrderID uuid.UUID       `json:"sell_order_id" example:"123e4567-e89b-12d3-a456-426614174000" description:"Sell order ID"`
	BuyUserID   uuid.UUID       `json:"buy_user_id" example:"123e4567-e89b-12d3-a456-426614174000" description:"Buyer user ID"`
	SellUserID  uuid.UUID       `json:"sell_user_id" example:"123e4567-e89b-12d3-a456-426614174000" description:"Seller user ID"`
	Price       decimal.Decimal `json:"price" example:"45000.00000000" description:"Trade execution price"`
	Quantity    decimal.Decimal `json:"quantity" example:"0.10000000" description:"Trade quantity"`
	Value       decimal.Decimal `json:"value" example:"4500.00000000" description:"Trade value"`
	BuyerFee    decimal.Decimal `json:"buyer_fee" example:"6.75000000" description:"Buyer fee"`
	SellerFee   decimal.Decimal `json:"seller_fee" example:"6.75000000" description:"Seller fee"`
	Side        string          `json:"side" example:"buy" description:"Trade side (from taker perspective)"`
	ExecutedAt  time.Time       `json:"executed_at" example:"2025-06-01T12:00:00Z" description:"Trade execution timestamp"`
} // @name Trade

// OrderBook represents order book data
type OrderBook struct {
	Symbol    string       `json:"symbol" example:"BTCUSDT" description:"Trading pair symbol"`
	Bids      []PriceLevel `json:"bids" description:"Buy orders (highest price first)"`
	Asks      []PriceLevel `json:"asks" description:"Sell orders (lowest price first)"`
	Timestamp time.Time    `json:"timestamp" example:"2025-06-01T12:00:00Z" description:"Order book snapshot timestamp"`
} // @name OrderBook

// PriceLevel represents a price level in the order book
type PriceLevel struct {
	Price    decimal.Decimal `json:"price" example:"45000.00000000" description:"Price level"`
	Quantity decimal.Decimal `json:"quantity" example:"0.50000000" description:"Total quantity at this price"`
	Count    int             `json:"count" example:"3" description:"Number of orders at this price"`
} // @name PriceLevel

// MarketPrice represents current market price data
type MarketPrice struct {
	Symbol                string          `json:"symbol" example:"BTCUSDT" description:"Trading pair symbol"`
	Price                 decimal.Decimal `json:"price" example:"45000.00000000" description:"Current price"`
	BidPrice              decimal.Decimal `json:"bid_price" example:"44995.00000000" description:"Best bid price"`
	AskPrice              decimal.Decimal `json:"ask_price" example:"45005.00000000" description:"Best ask price"`
	Volume24h             decimal.Decimal `json:"volume_24h" example:"150.25000000" description:"24-hour trading volume"`
	High24h               decimal.Decimal `json:"high_24h" example:"46000.00000000" description:"24-hour high price"`
	Low24h                decimal.Decimal `json:"low_24h" example:"43500.00000000" description:"24-hour low price"`
	Open24h               decimal.Decimal `json:"open_24h" example:"43750.00000000" description:"24-hour opening price"`
	PriceChange24h        decimal.Decimal `json:"price_change_24h" example:"1250.00000000" description:"24-hour price change"`
	PriceChangePercent24h decimal.Decimal `json:"price_change_percent_24h" example:"2.85" description:"24-hour price change percentage"`
	LastTradeTime         time.Time       `json:"last_trade_time" example:"2025-06-01T12:00:00Z" description:"Last trade timestamp"`
	UpdatedAt             time.Time       `json:"updated_at" example:"2025-06-01T12:00:00Z" description:"Price update timestamp"`
} // @name MarketPrice

// Candle represents OHLCV candlestick data
type Candle struct {
	OpenTime    time.Time       `json:"open_time" example:"2025-06-01T12:00:00Z" description:"Candle open time"`
	CloseTime   time.Time       `json:"close_time" example:"2025-06-01T12:59:59Z" description:"Candle close time"`
	Open        decimal.Decimal `json:"open" example:"45000.00000000" description:"Opening price"`
	High        decimal.Decimal `json:"high" example:"45500.00000000" description:"Highest price"`
	Low         decimal.Decimal `json:"low" example:"44800.00000000" description:"Lowest price"`
	Close       decimal.Decimal `json:"close" example:"45200.00000000" description:"Closing price"`
	Volume      decimal.Decimal `json:"volume" example:"10.50000000" description:"Trading volume"`
	QuoteVolume decimal.Decimal `json:"quote_volume" example:"472600.00000000" description:"Quote asset volume"`
	TradeCount  int             `json:"trade_count" example:"245" description:"Number of trades"`
} // @name Candle

// FiatDepositRequest represents fiat deposit request
type FiatDepositRequest struct {
	Amount   decimal.Decimal `json:"amount" example:"1000.00" validate:"required,gt=0" description:"Deposit amount"`
	Currency string          `json:"currency" example:"USD" validate:"required,currency_code" description:"Fiat currency code"`
	Method   string          `json:"method" example:"bank_transfer" validate:"required,oneof=bank_transfer card" description:"Deposit method"`
} // @name FiatDepositRequest

// FiatWithdrawRequest represents fiat withdrawal request
type FiatWithdrawRequest struct {
	Amount        decimal.Decimal `json:"amount" example:"1000.00" validate:"required,gt=0" description:"Withdrawal amount"`
	Currency      string          `json:"currency" example:"USD" validate:"required,currency_code" description:"Fiat currency code"`
	Method        string          `json:"method" example:"bank_transfer" validate:"required,oneof=bank_transfer" description:"Withdrawal method"`
	BankAccount   string          `json:"bank_account,omitempty" example:"1234567890" description:"Bank account number"`
	RoutingNumber string          `json:"routing_number,omitempty" example:"123456789" description:"Bank routing number"`
	IBAN          string          `json:"iban,omitempty" example:"GB29NWBK60161331926819" description:"IBAN for international transfers"`
	BIC           string          `json:"bic,omitempty" example:"NWBKGB2L" description:"BIC/SWIFT code"`
} // @name FiatWithdrawRequest

// FiatTransaction represents fiat transaction
type FiatTransaction struct {
	ID            uuid.UUID       `json:"id" example:"123e4567-e89b-12d3-a456-426614174000" description:"Transaction ID"`
	Type          string          `json:"type" example:"deposit" enums:"deposit,withdrawal" description:"Transaction type"`
	Amount        decimal.Decimal `json:"amount" example:"1000.00" description:"Transaction amount"`
	Currency      string          `json:"currency" example:"USD" description:"Fiat currency"`
	Fee           decimal.Decimal `json:"fee" example:"5.00" description:"Transaction fee"`
	Status        string          `json:"status" example:"pending" enums:"pending,processing,completed,failed,cancelled" description:"Transaction status"`
	Method        string          `json:"method" example:"bank_transfer" description:"Transaction method"`
	BankReference string          `json:"bank_reference,omitempty" example:"REF123456" description:"Bank reference number"`
	CreatedAt     time.Time       `json:"created_at" example:"2025-06-01T12:00:00Z" description:"Transaction creation time"`
	ProcessedAt   *time.Time      `json:"processed_at,omitempty" example:"2025-06-01T12:30:00Z" description:"Transaction processing time"`
	CompletedAt   *time.Time      `json:"completed_at,omitempty" example:"2025-06-01T13:00:00Z" description:"Transaction completion time"`
} // @name FiatTransaction

// KYCSubmitRequest represents KYC submission request
type KYCSubmitRequest struct {
	DocumentType   string `json:"document_type" example:"passport" validate:"required,oneof=passport drivers_license national_id" description:"Document type"`
	DocumentNumber string `json:"document_number" example:"A12345678" validate:"required" description:"Document number"`
	FirstName      string `json:"first_name" example:"John" validate:"required,alpha_space" description:"First name on document"`
	LastName       string `json:"last_name" example:"Doe" validate:"required,alpha_space" description:"Last name on document"`
	DateOfBirth    string `json:"date_of_birth" example:"1990-01-01" validate:"required" description:"Date of birth (YYYY-MM-DD)"`
	Country        string `json:"country" example:"US" validate:"required,len=2" description:"Country code (ISO 3166-1 alpha-2)"`
	Address        string `json:"address" example:"123 Main St, Anytown, ST 12345" validate:"required" description:"Full address"`
} // @name KYCSubmitRequest

// KYCStatus represents KYC verification status
type KYCStatus struct {
	Status          string     `json:"status" example:"verified" enums:"pending,in_review,verified,rejected,expired" description:"KYC status"`
	SubmittedAt     *time.Time `json:"submitted_at,omitempty" example:"2025-06-01T12:00:00Z" description:"KYC submission time"`
	ReviewedAt      *time.Time `json:"reviewed_at,omitempty" example:"2025-06-01T14:00:00Z" description:"KYC review completion time"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty" example:"2026-06-01T12:00:00Z" description:"KYC expiration time"`
	RejectionReason string     `json:"rejection_reason,omitempty" example:"Document quality insufficient" description:"Reason for rejection (if applicable)"`
	RequiredActions []string   `json:"required_actions,omitempty" example:"['resubmit_id_document']" description:"Actions required to complete KYC"`
} // @name KYCStatus

// RiskLimit represents risk management limits
type RiskLimit struct {
	ID         uuid.UUID       `json:"id" example:"123e4567-e89b-12d3-a456-426614174000" description:"Limit ID"`
	Type       string          `json:"type" example:"position" enums:"position,daily_volume,order_size" description:"Limit type"`
	UserID     uuid.UUID       `json:"user_id,omitempty" example:"123e4567-e89b-12d3-a456-426614174000" description:"User ID (null for global limits)"`
	Symbol     string          `json:"symbol,omitempty" example:"BTCUSDT" description:"Trading pair (null for global limits)"`
	LimitValue decimal.Decimal `json:"limit_value" example:"100000.00" description:"Limit value"`
	Currency   string          `json:"currency" example:"USD" description:"Limit currency"`
	IsActive   bool            `json:"is_active" example:"true" description:"Whether limit is active"`
	CreatedAt  time.Time       `json:"created_at" example:"2025-06-01T12:00:00Z" description:"Limit creation time"`
	UpdatedAt  time.Time       `json:"updated_at" example:"2025-06-01T12:00:00Z" description:"Last update time"`
} // @name RiskLimit

// RiskMetrics represents user risk metrics
type RiskMetrics struct {
	UserID           uuid.UUID       `json:"user_id" example:"123e4567-e89b-12d3-a456-426614174000" description:"User ID"`
	TotalPosition    decimal.Decimal `json:"total_position" example:"50000.00" description:"Total position value in USD"`
	DailyVolume      decimal.Decimal `json:"daily_volume" example:"25000.00" description:"Daily trading volume in USD"`
	WeeklyVolume     decimal.Decimal `json:"weekly_volume" example:"150000.00" description:"Weekly trading volume in USD"`
	MonthlyVolume    decimal.Decimal `json:"monthly_volume" example:"500000.00" description:"Monthly trading volume in USD"`
	RiskScore        decimal.Decimal `json:"risk_score" example:"0.35" description:"Risk score (0-1, higher is riskier)"`
	VaR              decimal.Decimal `json:"var" example:"5000.00" description:"Value at Risk (95% confidence, 1 day)"`
	MaxDrawdown      decimal.Decimal `json:"max_drawdown" example:"2500.00" description:"Maximum drawdown in USD"`
	Leverage         decimal.Decimal `json:"leverage" example:"2.5" description:"Current leverage ratio"`
	LastCalculatedAt time.Time       `json:"last_calculated_at" example:"2025-06-01T12:00:00Z" description:"Last risk calculation time"`
} // @name RiskMetrics

// ComplianceAlert represents AML compliance alert
type ComplianceAlert struct {
	ID          uuid.UUID              `json:"id" example:"123e4567-e89b-12d3-a456-426614174000" description:"Alert ID"`
	UserID      uuid.UUID              `json:"user_id" example:"123e4567-e89b-12d3-a456-426614174000" description:"User ID"`
	Type        string                 `json:"type" example:"large_transaction" enums:"large_transaction,suspicious_pattern,velocity,structuring" description:"Alert type"`
	Severity    string                 `json:"severity" example:"medium" enums:"low,medium,high,critical" description:"Alert severity"`
	Status      string                 `json:"status" example:"open" enums:"open,investigating,resolved,false_positive" description:"Alert status"`
	Description string                 `json:"description" example:"Large transaction detected: $50,000" description:"Alert description"`
	Details     map[string]interface{} `json:"details" description:"Additional alert details"`
	CreatedAt   time.Time              `json:"created_at" example:"2025-06-01T12:00:00Z" description:"Alert creation time"`
	UpdatedAt   time.Time              `json:"updated_at" example:"2025-06-01T12:00:00Z" description:"Last update time"`
	ResolvedAt  *time.Time             `json:"resolved_at,omitempty" example:"2025-06-01T14:00:00Z" description:"Alert resolution time"`
} // @name ComplianceAlert

// RateLimitStatus represents rate limit status for a user or IP
type RateLimitStatus struct {
	Key            string    `json:"key" example:"user:123e4567-e89b-12d3-a456-426614174000" description:"Rate limit key"`
	Type           string    `json:"type" example:"api_calls" description:"Rate limit type"`
	Limit          int       `json:"limit" example:"1000" description:"Rate limit threshold"`
	Remaining      int       `json:"remaining" example:"750" description:"Remaining requests"`
	ResetTime      time.Time `json:"reset_time" example:"2025-06-01T13:00:00Z" description:"Rate limit reset time"`
	WindowDuration string    `json:"window_duration" example:"1h" description:"Rate limit window duration"`
} // @name RateLimitStatus

// WebSocketMessage represents WebSocket message structure
type WebSocketMessage struct {
	Channel   string      `json:"channel" example:"orderbook" description:"Message channel"`
	Event     string      `json:"event" example:"update" description:"Event type"`
	Symbol    string      `json:"symbol,omitempty" example:"BTCUSDT" description:"Trading pair symbol"`
	Data      interface{} `json:"data" description:"Message data"`
	Timestamp time.Time   `json:"timestamp" example:"2025-06-01T12:00:00Z" description:"Message timestamp"`
	Sequence  int64       `json:"sequence" example:"12345" description:"Message sequence number"`
} // @name WebSocketMessage

// WebSocketSubscription represents WebSocket subscription request
type WebSocketSubscription struct {
	Method string   `json:"method" example:"SUBSCRIBE" enums:"SUBSCRIBE,UNSUBSCRIBE" description:"Subscription method"`
	Params []string `json:"params" example:"['orderbook@BTCUSDT', 'trades@BTCUSDT']" description:"Subscription parameters"`
	ID     int      `json:"id" example:"1" description:"Request ID"`
} // @name WebSocketSubscription

// Rate Limiting Models

// RateLimitConfigResponse represents rate limit configuration response
type RateLimitConfigResponse struct {
	Enabled         bool                              `json:"enabled" example:"true" description:"Whether rate limiting is enabled"`
	RedisKeyPrefix  string                            `json:"redis_key_prefix" example:"pincex_rate_limit" description:"Redis key prefix for rate limit data"`
	CleanupInterval string                            `json:"cleanup_interval" example:"1h" description:"Cleanup interval for expired data"`
	MaxDataAge      string                            `json:"max_data_age" example:"168h" description:"Maximum age for rate limit data"`
	TierConfigs     map[string]TierConfigResponse     `json:"tier_configs" description:"Rate limit configurations by user tier"`
	EndpointConfigs map[string]EndpointConfigResponse `json:"endpoint_configs" description:"Rate limit configurations by endpoint"`
	DefaultIPLimits IPLimitConfigResponse             `json:"default_ip_limits" description:"Default IP rate limits"`
	EmergencyMode   bool                              `json:"emergency_mode" example:"false" description:"Whether emergency mode is enabled"`
	EmergencyLimits TierConfigResponse                `json:"emergency_limits" description:"Emergency mode rate limits"`
} // @name RateLimitConfigResponse

// TierConfigResponse represents tier configuration in responses
type TierConfigResponse struct {
	APICallsPerMinute    int `json:"api_calls_per_minute" example:"100" description:"API calls per minute limit"`
	OrdersPerMinute      int `json:"orders_per_minute" example:"50" description:"Orders per minute limit"`
	TradesPerMinute      int `json:"trades_per_minute" example:"30" description:"Trades per minute limit"`
	WithdrawalsPerDay    int `json:"withdrawals_per_day" example:"10" description:"Withdrawals per day limit"`
	LoginAttemptsPerHour int `json:"login_attempts_per_hour" example:"10" description:"Login attempts per hour limit"`
} // @name TierConfigResponse

// EndpointConfigResponse represents endpoint configuration in responses
type EndpointConfigResponse struct {
	Name         string                `json:"name" example:"place_order" description:"Endpoint name"`
	RequiresAuth bool                  `json:"requires_auth" example:"true" description:"Whether endpoint requires authentication"`
	UserRateType string                `json:"user_rate_type" example:"orders" description:"User rate type for this endpoint"`
	IPRateLimit  IPLimitConfigResponse `json:"ip_rate_limit" description:"IP-based rate limits"`
	CustomLimits map[string]int        `json:"custom_limits,omitempty" description:"Custom limits by user tier"`
} // @name EndpointConfigResponse

// IPLimitConfigResponse represents IP rate limit configuration
type IPLimitConfigResponse struct {
	RequestsPerMinute int    `json:"requests_per_minute" example:"200" description:"Requests per minute limit"`
	BurstLimit        int    `json:"burst_limit" example:"50" description:"Burst limit"`
	Window            string `json:"window" example:"1m" description:"Rate limit window duration"`
} // @name IPLimitConfigResponse

// RateLimitConfigRequest represents rate limit configuration update request
type RateLimitConfigRequest struct {
	Enabled         bool                             `json:"enabled" example:"true" description:"Whether rate limiting is enabled"`
	RedisKeyPrefix  string                           `json:"redis_key_prefix" example:"pincex_rate_limit" description:"Redis key prefix for rate limit data"`
	CleanupInterval string                           `json:"cleanup_interval" example:"1h" description:"Cleanup interval for expired data"`
	MaxDataAge      string                           `json:"max_data_age" example:"168h" description:"Maximum age for rate limit data"`
	TierConfigs     map[string]TierLimitsRequest     `json:"tier_configs" description:"Rate limit configurations by user tier"`
	EndpointConfigs map[string]EndpointConfigRequest `json:"endpoint_configs" description:"Rate limit configurations by endpoint"`
	DefaultIPLimits IPRateLimitRequest               `json:"default_ip_limits" description:"Default IP rate limits"`
	EmergencyMode   bool                             `json:"emergency_mode" example:"false" description:"Whether emergency mode is enabled"`
	EmergencyLimits TierLimitsRequest                `json:"emergency_limits" description:"Emergency mode rate limits"`
} // @name RateLimitConfigRequest

// TierLimitsRequest represents tier limits update request
type TierLimitsRequest struct {
	APICallsPerMinute    int `json:"api_calls_per_minute" example:"100" validate:"min=0,max=10000" description:"API calls per minute limit"`
	OrdersPerMinute      int `json:"orders_per_minute" example:"50" validate:"min=0,max=1000" description:"Orders per minute limit"`
	TradesPerMinute      int `json:"trades_per_minute" example:"30" validate:"min=0,max=1000" description:"Trades per minute limit"`
	WithdrawalsPerDay    int `json:"withdrawals_per_day" example:"10" validate:"min=0,max=1000" description:"Withdrawals per day limit"`
	LoginAttemptsPerHour int `json:"login_attempts_per_hour" example:"10" validate:"min=1,max=100" description:"Login attempts per hour limit"`
} // @name TierLimitsRequest

// EndpointConfigRequest represents endpoint configuration update request
type EndpointConfigRequest struct {
	Endpoint string `json:"endpoint" example:"POST:/api/v1/trading/orders" validate:"required" description:"API endpoint pattern"`
	Config   struct {
		Name         string             `json:"name" example:"place_order" validate:"required" description:"Endpoint name"`
		RequiresAuth bool               `json:"requires_auth" example:"true" description:"Whether endpoint requires authentication"`
		UserRateType string             `json:"user_rate_type" example:"orders" validate:"omitempty,oneof=api_calls orders trades withdrawals login_attempts" description:"User rate type"`
		IPRateLimit  IPRateLimitRequest `json:"ip_rate_limit" description:"IP-based rate limits"`
		CustomLimits map[string]int     `json:"custom_limits,omitempty" description:"Custom limits by user tier"`
	} `json:"config" description:"Endpoint configuration"`
} // @name EndpointConfigRequest

// IPRateLimitRequest represents IP rate limit configuration request
type IPRateLimitRequest struct {
	RequestsPerMinute int    `json:"requests_per_minute" example:"200" validate:"min=1,max=10000" description:"Requests per minute limit"`
	BurstLimit        int    `json:"burst_limit" example:"50" validate:"min=1,max=1000" description:"Burst limit"`
	Window            string `json:"window" example:"1m" validate:"required" description:"Rate limit window duration"`
} // @name IPRateLimitRequest

// EmergencyModeRequest represents emergency mode toggle request
type EmergencyModeRequest struct {
	Enabled bool `json:"enabled" example:"true" description:"Whether to enable emergency mode"`
} // @name EmergencyModeRequest

// UserRateLimitResponse represents user rate limit status response
type UserRateLimitResponse struct {
	UserID string                           `json:"user_id" example:"123e4567-e89b-12d3-a456-426614174000" description:"User ID"`
	Status map[string]RateLimitInfoResponse `json:"status" description:"Rate limit status by rate type"`
} // @name UserRateLimitResponse

// IPRateLimitStatusResponse represents IP rate limit status response
type IPRateLimitStatusResponse struct {
	IP     string                           `json:"ip" example:"192.168.1.1" description:"IP address"`
	Status map[string]RateLimitInfoResponse `json:"status" description:"Rate limit status by endpoint"`
} // @name IPRateLimitStatusResponse

// RateLimitInfoResponse represents detailed rate limit information
type RateLimitInfoResponse struct {
	Limit     int       `json:"limit" example:"100" description:"Rate limit maximum"`
	Remaining int       `json:"remaining" example:"45" description:"Remaining requests in current window"`
	ResetAt   time.Time `json:"reset_at" example:"2025-06-01T13:00:00Z" description:"When the rate limit resets"`
	Window    string    `json:"window" example:"1m" description:"Rate limit window duration"`
} // @name RateLimitInfoResponse

// StandardResponse represents a standard success response
type StandardResponse struct {
	Message   string    `json:"message" example:"Operation completed successfully" description:"Success message"`
	Success   bool      `json:"success" example:"true" description:"Operation success status"`
	Timestamp time.Time `json:"timestamp" example:"2025-06-01T12:00:00Z" description:"Response timestamp"`
} // @name StandardResponse

// Risk Management Models

// RiskLimitsResponse represents risk limits configuration response
type RiskLimitsResponse struct {
	UserLimits   map[string]RiskLimitDetails `json:"user_limits" description:"Per-user risk limits"`
	MarketLimits map[string]RiskLimitDetails `json:"market_limits" description:"Per-market risk limits"`
	GlobalLimits map[string]RiskLimitDetails `json:"global_limits" description:"Global risk limits"`
} // @name RiskLimitsResponse

// RiskLimitDetails represents individual risk limit details
type RiskLimitDetails struct {
	Type         string          `json:"type" example:"position_size" description:"Risk limit type"`
	Limit        decimal.Decimal `json:"limit" example:"10000.00" description:"Risk limit value"`
	CurrentValue decimal.Decimal `json:"current_value" example:"5000.00" description:"Current value against limit"`
	Utilization  float64         `json:"utilization" example:"50.0" description:"Limit utilization percentage"`
	LastUpdated  time.Time       `json:"last_updated" example:"2025-06-01T12:00:00Z" description:"Last update timestamp"`
} // @name RiskLimitDetails

// RiskLimitRequest represents risk limit creation/update request
type RiskLimitRequest struct {
	Type   string          `json:"type" example:"position_size" validate:"required,oneof=position_size daily_loss max_leverage exposure_limit" description:"Risk limit type"`
	Target string          `json:"target" example:"user123" validate:"required" description:"Target entity (user ID, market symbol, or 'global')"`
	Limit  decimal.Decimal `json:"limit" example:"10000.00" validate:"required,gt=0" description:"Risk limit value"`
} // @name RiskLimitRequest

// RiskExemptionRequest represents risk exemption creation request
type RiskExemptionRequest struct {
	Type      string    `json:"type" example:"position_size" validate:"required,oneof=position_size daily_loss max_leverage exposure_limit" description:"Risk limit type"`
	Target    string    `json:"target" example:"user123" validate:"required" description:"Target entity for exemption"`
	Reason    string    `json:"reason" example:"VIP client exemption" validate:"required,min=10,max=500" description:"Reason for exemption"`
	ExpiresAt time.Time `json:"expires_at" example:"2025-12-31T23:59:59Z" validate:"required" description:"Exemption expiration time"`
} // @name RiskExemptionRequest

// RiskExemptionResponse represents risk exemption details
type RiskExemptionResponse struct {
	ID        uuid.UUID `json:"id" example:"123e4567-e89b-12d3-a456-426614174000" description:"Exemption ID"`
	Type      string    `json:"type" example:"position_size" description:"Risk limit type"`
	Target    string    `json:"target" example:"user123" description:"Target entity"`
	Reason    string    `json:"reason" example:"VIP client exemption" description:"Reason for exemption"`
	CreatedBy string    `json:"created_by" example:"admin123" description:"Admin who created exemption"`
	CreatedAt time.Time `json:"created_at" example:"2025-06-01T12:00:00Z" description:"Creation timestamp"`
	ExpiresAt time.Time `json:"expires_at" example:"2025-12-31T23:59:59Z" description:"Expiration timestamp"`
} // @name RiskExemptionResponse

// RiskMetricsResponse represents user risk metrics
type RiskMetricsResponse struct {
	UserID           string             `json:"user_id" example:"123e4567-e89b-12d3-a456-426614174000" description:"User ID"`
	RiskScore        float64            `json:"risk_score" example:"75.5" description:"Overall risk score (0-100)"`
	PositionMetrics  PositionMetrics    `json:"position_metrics" description:"Position-related metrics"`
	TradingMetrics   TradingMetrics     `json:"trading_metrics" description:"Trading activity metrics"`
	LimitUtilization map[string]float64 `json:"limit_utilization" description:"Risk limit utilization percentages"`
	Violations       []RiskViolation    `json:"violations" description:"Recent risk violations"`
	LastCalculated   time.Time          `json:"last_calculated" example:"2025-06-01T12:00:00Z" description:"Last calculation timestamp"`
} // @name RiskMetricsResponse

// PositionMetrics represents position-related risk metrics
type PositionMetrics struct {
	TotalExposure     decimal.Decimal `json:"total_exposure" example:"50000.00" description:"Total position exposure"`
	MaxSinglePosition decimal.Decimal `json:"max_single_position" example:"10000.00" description:"Largest single position"`
	Leverage          float64         `json:"leverage" example:"2.5" description:"Current leverage ratio"`
	MarginUsed        decimal.Decimal `json:"margin_used" example:"20000.00" description:"Margin currently in use"`
	FreeMargin        decimal.Decimal `json:"free_margin" example:"5000.00" description:"Available margin"`
} // @name PositionMetrics

// TradingMetrics represents trading activity metrics
type TradingMetrics struct {
	DailyVolume   decimal.Decimal `json:"daily_volume" example:"25000.00" description:"24-hour trading volume"`
	OrderCount24h int             `json:"order_count_24h" example:"150" description:"Orders placed in last 24 hours"`
	CancelRate    float64         `json:"cancel_rate" example:"15.5" description:"Order cancellation rate percentage"`
	PnL24h        decimal.Decimal `json:"pnl_24h" example:"-250.00" description:"24-hour profit/loss"`
	WinRate       float64         `json:"win_rate" example:"65.2" description:"Winning trade percentage"`
} // @name TradingMetrics

// RiskViolation represents a risk limit violation
type RiskViolation struct {
	Type      string          `json:"type" example:"position_size" description:"Violation type"`
	Limit     decimal.Decimal `json:"limit" example:"10000.00" description:"Risk limit that was violated"`
	Value     decimal.Decimal `json:"value" example:"12000.00" description:"Actual value that exceeded limit"`
	Severity  string          `json:"severity" example:"high" enums:"low,medium,high,critical" description:"Violation severity"`
	Action    string          `json:"action" example:"position_reduced" description:"Action taken"`
	Timestamp time.Time       `json:"timestamp" example:"2025-06-01T12:00:00Z" description:"Violation timestamp"`
} // @name RiskViolation

// RiskDashboardResponse represents risk dashboard data
type RiskDashboardResponse struct {
	Summary          RiskSummary                  `json:"summary" description:"Overall risk summary"`
	ActiveViolations []RiskViolation              `json:"active_violations" description:"Current active violations"`
	RecentAlerts     []RiskAlert                  `json:"recent_alerts" description:"Recent risk alerts"`
	TopRiskyUsers    []UserRiskSummary            `json:"top_risky_users" description:"Users with highest risk scores"`
	MarketMetrics    map[string]MarketRiskMetrics `json:"market_metrics" description:"Risk metrics by market"`
} // @name RiskDashboardResponse

// RiskSummary represents overall risk summary
type RiskSummary struct {
	TotalUsers       int             `json:"total_users" example:"1250" description:"Total active users"`
	HighRiskUsers    int             `json:"high_risk_users" example:"25" description:"Users with high risk scores"`
	ActiveViolations int             `json:"active_violations" example:"5" description:"Current active violations"`
	SystemRiskScore  float64         `json:"system_risk_score" example:"68.5" description:"Overall system risk score"`
	TotalExposure    decimal.Decimal `json:"total_exposure" example:"5000000.00" description:"Total system exposure"`
} // @name RiskSummary

// RiskAlert represents a risk alert
type RiskAlert struct {
	ID           uuid.UUID `json:"id" example:"123e4567-e89b-12d3-a456-426614174000" description:"Alert ID"`
	Type         string    `json:"type" example:"limit_breach" description:"Alert type"`
	Severity     string    `json:"severity" example:"high" enums:"low,medium,high,critical" description:"Alert severity"`
	Message      string    `json:"message" example:"User exceeded position limit" description:"Alert message"`
	UserID       string    `json:"user_id,omitempty" example:"user123" description:"Related user ID"`
	Market       string    `json:"market,omitempty" example:"BTCUSDT" description:"Related market"`
	Acknowledged bool      `json:"acknowledged" example:"false" description:"Whether alert is acknowledged"`
	CreatedAt    time.Time `json:"created_at" example:"2025-06-01T12:00:00Z" description:"Alert creation time"`
} // @name RiskAlert

// UserRiskSummary represents a user's risk summary
type UserRiskSummary struct {
	UserID     string          `json:"user_id" example:"user123" description:"User ID"`
	Username   string          `json:"username" example:"trader123" description:"Username"`
	RiskScore  float64         `json:"risk_score" example:"85.5" description:"Risk score"`
	Exposure   decimal.Decimal `json:"exposure" example:"15000.00" description:"Total exposure"`
	Violations int             `json:"violations" example:"2" description:"Active violations count"`
} // @name UserRiskSummary

// MarketRiskMetrics represents market-level risk metrics
type MarketRiskMetrics struct {
	Symbol        string          `json:"symbol" example:"BTCUSDT" description:"Market symbol"`
	TotalExposure decimal.Decimal `json:"total_exposure" example:"500000.00" description:"Total market exposure"`
	LongExposure  decimal.Decimal `json:"long_exposure" example:"300000.00" description:"Long position exposure"`
	ShortExposure decimal.Decimal `json:"short_exposure" example:"200000.00" description:"Short position exposure"`
	Volatility    float64         `json:"volatility" example:"25.5" description:"Market volatility percentage"`
	RiskLevel     string          `json:"risk_level" example:"medium" enums:"low,medium,high,critical" description:"Market risk level"`
} // @name MarketRiskMetrics

// ReportRequest represents report generation request
type ReportRequest struct {
	Type      string            `json:"type" example:"risk_summary" validate:"required,oneof=risk_summary user_metrics market_analysis compliance_report" description:"Report type"`
	StartDate time.Time         `json:"start_date" example:"2025-06-01T00:00:00Z" validate:"required" description:"Report start date"`
	EndDate   time.Time         `json:"end_date" example:"2025-06-01T23:59:59Z" validate:"required" description:"Report end date"`
	Filters   map[string]string `json:"filters,omitempty" description:"Additional report filters"`
} // @name ReportRequest

// ReportResponse represents generated report information
type ReportResponse struct {
	ID          uuid.UUID  `json:"id" example:"123e4567-e89b-12d3-a456-426614174000" description:"Report ID"`
	Type        string     `json:"type" example:"risk_summary" description:"Report type"`
	Status      string     `json:"status" example:"completed" enums:"pending,processing,completed,failed" description:"Report status"`
	StartDate   time.Time  `json:"start_date" example:"2025-06-01T00:00:00Z" description:"Report start date"`
	EndDate     time.Time  `json:"end_date" example:"2025-06-01T23:59:59Z" description:"Report end date"`
	CreatedAt   time.Time  `json:"created_at" example:"2025-06-01T12:00:00Z" description:"Report creation time"`
	CompletedAt *time.Time `json:"completed_at,omitempty" example:"2025-06-01T12:05:00Z" description:"Report completion time"`
	DownloadURL string     `json:"download_url,omitempty" example:"https://api.pincex.com/reports/123e4567-e89b-12d3-a456-426614174000" description:"Report download URL"`
	FileSize    int64      `json:"file_size,omitempty" example:"1048576" description:"Report file size in bytes"`
} // @name ReportResponse

// ReportsListResponse represents list of reports with pagination
type ReportsListResponse struct {
	Reports    []ReportResponse `json:"reports" description:"List of reports"`
	Pagination PaginationMeta   `json:"pagination" description:"Pagination metadata"`
} // @name ReportsListResponse

// Compliance Models

// ComplianceAlertResponse represents compliance alert details
type ComplianceAlertResponse struct {
	ID          uuid.UUID              `json:"id" example:"123e4567-e89b-12d3-a456-426614174000" description:"Alert ID"`
	Type        string                 `json:"type" example:"suspicious_activity" description:"Alert type"`
	Severity    string                 `json:"severity" example:"high" enums:"low,medium,high,critical" description:"Alert severity"`
	Status      string                 `json:"status" example:"open" enums:"open,investigating,resolved,false_positive" description:"Alert status"`
	UserID      string                 `json:"user_id" example:"user123" description:"Related user ID"`
	Description string                 `json:"description" example:"Multiple large transactions detected" description:"Alert description"`
	Data        map[string]interface{} `json:"data" description:"Alert-specific data"`
	CreatedAt   time.Time              `json:"created_at" example:"2025-06-01T12:00:00Z" description:"Alert creation time"`
	UpdatedAt   time.Time              `json:"updated_at" example:"2025-06-01T12:00:00Z" description:"Last update time"`
	ResolvedAt  *time.Time             `json:"resolved_at,omitempty" example:"2025-06-01T12:30:00Z" description:"Resolution timestamp"`
	ResolvedBy  string                 `json:"resolved_by,omitempty" example:"admin123" description:"Admin who resolved alert"`
} // @name ComplianceAlertResponse

// ComplianceAlertsResponse represents list of compliance alerts
type ComplianceAlertsResponse struct {
	Alerts     []ComplianceAlertResponse `json:"alerts" description:"List of compliance alerts"`
	Pagination PaginationMeta            `json:"pagination" description:"Pagination metadata"`
} // @name ComplianceAlertsResponse

// ComplianceAlertUpdateRequest represents alert status update request
type ComplianceAlertUpdateRequest struct {
	Status string `json:"status" validate:"required,oneof=investigating resolved false_positive" description:"New alert status"`
	Notes  string `json:"notes,omitempty" validate:"max=1000" description:"Resolution notes"`
} // @name ComplianceAlertUpdateRequest

// ComplianceRuleRequest represents compliance rule creation request
type ComplianceRuleRequest struct {
	Name        string                 `json:"name" example:"Large Transaction Monitor" validate:"required,min=5,max=100" description:"Rule name"`
	Type        string                 `json:"type" example:"transaction_monitoring" validate:"required,oneof=transaction_monitoring user_behavior kyc_verification sanctions_screening" description:"Rule type"`
	Description string                 `json:"description" example:"Monitors transactions above threshold" validate:"required,min=10,max=500" description:"Rule description"`
	Parameters  map[string]interface{} `json:"parameters" description:"Rule-specific parameters"`
	Enabled     bool                   `json:"enabled" example:"true" description:"Whether rule is enabled"`
	Severity    string                 `json:"severity" example:"medium" validate:"required,oneof=low medium high critical" description:"Alert severity for violations"`
} // @name ComplianceRuleRequest

// ComplianceRuleResponse represents compliance rule details
type ComplianceRuleResponse struct {
	ID          uuid.UUID              `json:"id" example:"123e4567-e89b-12d3-a456-426614174000" description:"Rule ID"`
	Name        string                 `json:"name" example:"Large Transaction Monitor" description:"Rule name"`
	Type        string                 `json:"type" example:"transaction_monitoring" description:"Rule type"`
	Description string                 `json:"description" example:"Monitors transactions above threshold" description:"Rule description"`
	Parameters  map[string]interface{} `json:"parameters" description:"Rule-specific parameters"`
	Enabled     bool                   `json:"enabled" example:"true" description:"Whether rule is enabled"`
	Severity    string                 `json:"severity" example:"medium" description:"Alert severity for violations"`
	CreatedAt   time.Time              `json:"created_at" example:"2025-06-01T12:00:00Z" description:"Rule creation time"`
	UpdatedAt   time.Time              `json:"updated_at" example:"2025-06-01T12:00:00Z" description:"Last update time"`
	CreatedBy   string                 `json:"created_by" example:"admin123" description:"Admin who created rule"`
} // @name ComplianceRuleResponse

// ComplianceTransactionsResponse represents compliance transactions list
type ComplianceTransactionsResponse struct {
	Transactions []ComplianceTransaction `json:"transactions" description:"List of flagged transactions"`
	Pagination   PaginationMeta          `json:"pagination" description:"Pagination metadata"`
} // @name ComplianceTransactionsResponse

// ComplianceTransaction represents a transaction flagged for compliance review
type ComplianceTransaction struct {
	ID         uuid.UUID              `json:"id" example:"123e4567-e89b-12d3-a456-426614174000" description:"Transaction ID"`
	UserID     string                 `json:"user_id" example:"user123" description:"User who initiated transaction"`
	Type       string                 `json:"type" example:"withdrawal" enums:"deposit,withdrawal,trade,transfer" description:"Transaction type"`
	Amount     decimal.Decimal        `json:"amount" example:"10000.00" description:"Transaction amount"`
	Currency   string                 `json:"currency" example:"USDT" description:"Transaction currency"`
	Status     string                 `json:"status" example:"flagged" enums:"pending,flagged,approved,rejected" description:"Compliance status"`
	RiskScore  float64                `json:"risk_score" example:"75.5" description:"Calculated risk score"`
	Flags      []string               `json:"flags" example:"['large_amount', 'unusual_pattern']" description:"Compliance flags"`
	ReviewedBy string                 `json:"reviewed_by,omitempty" example:"compliance123" description:"Compliance officer who reviewed"`
	ReviewedAt *time.Time             `json:"reviewed_at,omitempty" example:"2025-06-01T12:30:00Z" description:"Review timestamp"`
	Notes      string                 `json:"notes,omitempty" example:"Verified with customer" description:"Review notes"`
	CreatedAt  time.Time              `json:"created_at" example:"2025-06-01T12:00:00Z" description:"Transaction timestamp"`
	Metadata   map[string]interface{} `json:"metadata,omitempty" description:"Additional transaction metadata"`
} // @name ComplianceTransaction
