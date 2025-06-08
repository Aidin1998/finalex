package crosspair

import (
	"context"
	"sync"
	"time"

	"github.com/Aidin1998/finalex/internal/trading/model"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Status constants for model compatibility
const (
	OrderStatusPending   = "PENDING"
	OrderStatusFilled    = "FILLED"
	OrderStatusCanceled  = "CANCELED"
	OrderStatusRejected  = "REJECTED"
	OrderStatusPartially = "PARTIALLY_FILLED"
)

// CrossPairOrderType represents the type of cross-pair order
type CrossPairOrderType string

const (
	CrossPairMarketOrder CrossPairOrderType = "MARKET"
	CrossPairLimitOrder  CrossPairOrderType = "LIMIT"
)

// CrossPairOrderSide represents the side of a cross-pair order
type CrossPairOrderSide string

const (
	CrossPairBuy  CrossPairOrderSide = "BUY"
	CrossPairSell CrossPairOrderSide = "SELL"
)

// CrossPairOrderStatus represents the status of a cross-pair order
type CrossPairOrderStatus string

const (
	CrossPairOrderPending         CrossPairOrderStatus = "PENDING"
	CrossPairOrderExecuting       CrossPairOrderStatus = "EXECUTING"
	CrossPairOrderCompleted       CrossPairOrderStatus = "COMPLETED"
	CrossPairOrderFailed          CrossPairOrderStatus = "FAILED"
	CrossPairOrderCanceled        CrossPairOrderStatus = "CANCELED"
	CrossPairOrderPartiallyFilled CrossPairOrderStatus = "PARTIALLY_FILLED" // Added for partial fill
)

// Alias for backwards compatibility with other files
type OrderStatus = CrossPairOrderStatus

// CrossPairOrder represents a cross-pair trading order
type CrossPairOrder struct {
	ID               uuid.UUID            `json:"id" db:"id"`
	UserID           uuid.UUID            `json:"user_id" db:"user_id"`
	FromAsset        string               `json:"from_asset" db:"from_asset"`
	ToAsset          string               `json:"to_asset" db:"to_asset"`
	Type             CrossPairOrderType   `json:"type" db:"type"`
	Side             CrossPairOrderSide   `json:"side" db:"side"`
	Status           CrossPairOrderStatus `json:"status" db:"status"`
	Quantity         decimal.Decimal      `json:"quantity" db:"quantity"`
	Price            *decimal.Decimal     `json:"price,omitempty" db:"price"` // nil for market orders
	EstimatedRate    decimal.Decimal      `json:"estimated_rate" db:"estimated_rate"`
	MaxSlippage      decimal.Decimal      `json:"max_slippage" db:"max_slippage"`
	Route            *CrossPairRoute      `json:"route,omitempty"`
	ExecutedQuantity decimal.Decimal      `json:"executed_quantity" db:"executed_quantity"`
	ExecutedRate     *decimal.Decimal     `json:"executed_rate,omitempty" db:"executed_rate"`
	Fees             []CrossPairFee       `json:"fees,omitempty"`
	CreatedAt        time.Time            `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time            `json:"updated_at" db:"updated_at"`
	ExpiresAt        *time.Time           `json:"expires_at,omitempty" db:"expires_at"`
	ErrorMessage     *string              `json:"error_message,omitempty" db:"error_message"`
}

// CrossPairRoute represents the execution route for a cross-pair trade
type CrossPairRoute struct {
	ID            uuid.UUID       `json:"id" db:"id"`
	BaseAsset     string          `json:"base_asset"`
	FirstPair     string          `json:"first_pair"`     // e.g., "BTC-USDT"
	SecondPair    string          `json:"second_pair"`    // e.g., "ETH-USDT"
	FirstRate     decimal.Decimal `json:"first_rate"`     // Rate for first leg
	SecondRate    decimal.Decimal `json:"second_rate"`    // Rate for second leg
	SyntheticRate decimal.Decimal `json:"synthetic_rate"` // Combined rate
	Confidence    decimal.Decimal `json:"confidence"`     // Confidence score (0-1)
	Active        bool            `json:"active" db:"active"`
	CreatedAt     time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at" db:"updated_at"`
}

// CrossPairFee represents fees for cross-pair trading
type CrossPairFee struct {
	Asset   string          `json:"asset"`
	Amount  decimal.Decimal `json:"amount"`
	FeeType string          `json:"fee_type"` // "TRADING", "SPREAD", "SLIPPAGE"
	Pair    string          `json:"pair"`     // Which pair this fee applies to
}

// CrossPairTrade represents an executed cross-pair trade
type CrossPairTrade struct {
	ID              uuid.UUID           `json:"id" db:"id"`
	OrderID         uuid.UUID           `json:"order_id" db:"order_id"`
	UserID          uuid.UUID           `json:"user_id" db:"user_id"`
	FromAsset       string              `json:"from_asset" db:"from_asset"`
	ToAsset         string              `json:"to_asset" db:"to_asset"`
	Quantity        decimal.Decimal     `json:"quantity" db:"quantity"`
	ExecutedRate    decimal.Decimal     `json:"executed_rate" db:"executed_rate"`
	Legs            []CrossPairTradeLeg `json:"legs"`
	Fees            []CrossPairFee      `json:"fees"`
	TotalFeeUSD     decimal.Decimal     `json:"total_fee_usd" db:"total_fee_usd"`
	Slippage        decimal.Decimal     `json:"slippage" db:"slippage"`
	ExecutionTimeMs int64               `json:"execution_time_ms" db:"execution_time_ms"`
	CreatedAt       time.Time           `json:"created_at" db:"created_at"`
}

// CrossPairTradeLeg represents one leg of a cross-pair trade execution
type CrossPairTradeLeg struct {
	ID                uuid.UUID          `json:"id"`
	Pair              string             `json:"pair"`
	Side              string             `json:"side"` // "BUY" or "SELL"
	Quantity          decimal.Decimal    `json:"quantity"`
	Price             decimal.Decimal    `json:"price"`
	Commission        decimal.Decimal    `json:"commission"`
	CommissionAsset   string             `json:"commission_asset"`
	TradeID           uuid.UUID          `json:"trade_id"` // Reference to underlying spot trade
	ExecutedAt        time.Time          `json:"executed_at"`
	Fees              []CrossPairFee     `json:"fees"` // Added missing Fees field
	OrderBookSnapshot *OrderBookSnapshot `json:"orderbook_snapshot,omitempty"`
}

// OrderBookSnapshot captures orderbook state at execution time
type OrderBookSnapshot struct {
	Pair      string           `json:"pair"`
	Timestamp time.Time        `json:"timestamp"`
	Bids      []OrderBookLevel `json:"bids"`
	Asks      []OrderBookLevel `json:"asks"`
	Sequence  uint64           `json:"sequence"`
}

// OrderBookLevel represents a price level in the orderbook
type OrderBookLevel struct {
	Price    decimal.Decimal `json:"price"`
	Quantity decimal.Decimal `json:"quantity"`
	Orders   int             `json:"orders"`
}

// CrossPairMetrics represents performance metrics for cross-pair trading
type CrossPairMetrics struct {
	Pair             string          `json:"pair"`
	AvgExecutionTime time.Duration   `json:"avg_execution_time"`
	AvgSlippage      decimal.Decimal `json:"avg_slippage"`
	SuccessRate      decimal.Decimal `json:"success_rate"`
	Volume24h        decimal.Decimal `json:"volume_24h"`
	TradeCount24h    int64           `json:"trade_count_24h"`
	LastUpdate       time.Time       `json:"last_update"`
}

// ExecutionPlan represents a plan for executing a cross-pair order
type ExecutionPlan struct {
	OrderID      uuid.UUID       `json:"order_id"`
	UserID       uuid.UUID       `json:"user_id"`
	Leg1         *TradeExecution `json:"leg1"`
	Leg2         *TradeExecution `json:"leg2"`
	EstimatedFee decimal.Decimal `json:"estimated_fee"`
	CreatedAt    time.Time       `json:"created_at"`
}

// ExecutionResult represents the result of executing a cross-pair order
type ExecutionResult struct {
	OrderID     uuid.UUID       `json:"order_id"`
	Success     bool            `json:"success"`
	Leg1TradeID uuid.UUID       `json:"leg1_trade_id"`
	Leg2TradeID uuid.UUID       `json:"leg2_trade_id"`
	ExecutedAt  time.Time       `json:"executed_at"`
	TotalFee    decimal.Decimal `json:"total_fee"`
	Error       string          `json:"error,omitempty"`
}

// ExecutionEstimate represents an estimate for executing a cross-pair order
type ExecutionEstimate struct {
	EstimatedPrice    decimal.Decimal `json:"estimated_price"`
	EstimatedSlippage decimal.Decimal `json:"estimated_slippage"`
	EstimatedFee      decimal.Decimal `json:"estimated_fee"`
	Confidence        decimal.Decimal `json:"confidence"`
	CanExecute        bool            `json:"can_execute"`
	Timestamp         time.Time       `json:"timestamp"`
}

// TradeExecution represents a trade execution for a single leg
type TradeExecution struct {
	Pair     string          `json:"pair"`
	Side     string          `json:"side"`
	Quantity decimal.Decimal `json:"quantity"`
	Price    decimal.Decimal `json:"price"`
	Fee      decimal.Decimal `json:"fee"`
}

// OrderbookProvider interface for getting orderbook data
// Use local OrderBookSnapshot instead of model.OrderBook
type OrderbookProvider interface {
	GetOrderbook(ctx context.Context, pair string) (*OrderBookSnapshot, error)
	GetBestBidAsk(ctx context.Context, pair string) (bid, ask decimal.Decimal, err error)
}

// Validation methods

// Validate validates a CrossPairOrder
func (o *CrossPairOrder) Validate() error {
	if o.FromAsset == "" {
		return ErrInvalidFromAsset
	}
	if o.ToAsset == "" {
		return ErrInvalidToAsset
	}
	if o.FromAsset == o.ToAsset {
		return ErrSameAssets
	}
	if o.Quantity.LessThanOrEqual(decimal.Zero) {
		return ErrInvalidQuantity
	}
	if o.Type == CrossPairLimitOrder && (o.Price == nil || o.Price.LessThanOrEqual(decimal.Zero)) {
		return ErrInvalidPrice
	}
	if o.MaxSlippage.LessThan(decimal.Zero) || o.MaxSlippage.GreaterThan(decimal.NewFromFloat(1.0)) {
		return ErrInvalidSlippage
	}
	return nil
}

// IsExpired checks if the order has expired
func (o *CrossPairOrder) IsExpired() bool {
	return o.ExpiresAt != nil && time.Now().After(*o.ExpiresAt)
}

// CanExecute checks if the order can be executed
func (o *CrossPairOrder) CanExecute() bool {
	return o.Status == CrossPairOrderPending && !o.IsExpired()
}

// GetRemainingQuantity returns the remaining quantity to be executed
func (o *CrossPairOrder) GetRemainingQuantity() decimal.Decimal {
	return o.Quantity.Sub(o.ExecutedQuantity)
}

// Validate validates a CrossPairRoute
func (r *CrossPairRoute) Validate() error {
	if r.BaseAsset == "" {
		return NewCrossPairError("INVALID_BASE_ASSET", "base asset is required")
	}
	if r.FirstPair == "" {
		return NewCrossPairError("INVALID_FIRST_PAIR", "first pair is required")
	}
	if r.SecondPair == "" {
		return NewCrossPairError("INVALID_SECOND_PAIR", "second pair is required")
	}
	if r.FirstRate.LessThanOrEqual(decimal.Zero) {
		return NewCrossPairError("INVALID_FIRST_RATE", "first rate must be greater than zero")
	}
	if r.SecondRate.LessThanOrEqual(decimal.Zero) {
		return NewCrossPairError("INVALID_SECOND_RATE", "second rate must be greater than zero")
	}
	if r.SyntheticRate.LessThanOrEqual(decimal.Zero) {
		return NewCrossPairError("INVALID_SYNTHETIC_RATE", "synthetic rate must be greater than zero")
	}
	if r.Confidence.LessThan(decimal.Zero) || r.Confidence.GreaterThan(decimal.NewFromFloat(1.0)) {
		return NewCrossPairError("INVALID_CONFIDENCE", "confidence must be between 0 and 1")
	}
	return nil
}

// Error definitions
var (
	ErrInvalidFromAsset = NewCrossPairError("INVALID_FROM_ASSET", "from asset is required")
	ErrInvalidToAsset   = NewCrossPairError("INVALID_TO_ASSET", "to asset is required")
	ErrSameAssets       = NewCrossPairError("SAME_ASSETS", "from and to assets cannot be the same")
	ErrInvalidQuantity  = NewCrossPairError("INVALID_QUANTITY", "quantity must be greater than zero")
	ErrInvalidPrice     = NewCrossPairError("INVALID_PRICE", "price must be greater than zero for limit orders")
	ErrInvalidSlippage  = NewCrossPairError("INVALID_SLIPPAGE", "slippage must be between 0 and 1")
	ErrOrderNotFound    = NewCrossPairError("ORDER_NOT_FOUND", "order not found")
	ErrRouteNotFound    = NewCrossPairError("ROUTE_NOT_FOUND", "no route found for the given assets")
)

// CrossPairError represents a cross-pair trading error
type CrossPairError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *CrossPairError) Error() string {
	return e.Message
}

// NewCrossPairError creates a new CrossPairError
func NewCrossPairError(code, message string) *CrossPairError {
	return &CrossPairError{
		Code:    code,
		Message: message,
	}
}

// RateCalculationResult represents the result of a rate calculation
type RateCalculationResult struct {
	Route             *CrossPairRoute `json:"route"`
	EstimatedSlippage decimal.Decimal `json:"estimated_slippage"`
	Confidence        decimal.Decimal `json:"confidence"`
	LiquidityUSD      decimal.Decimal `json:"liquidity_usd"`
	EstimatedFees     []CrossPairFee  `json:"estimated_fees"`
}

// RateCalculator interface for calculating synthetic rates
type RateCalculator interface {
	CalculateRate(fromAsset, toAsset string, quantity decimal.Decimal) (*RateCalculationResult, error)
	CalculateRateWithDetails(fromAsset, toAsset string, quantity decimal.Decimal) (*RateCalculationResult, error)
	GetBestRoute(fromAsset, toAsset string) (*CrossPairRoute, error)
	Subscribe(callback func(pair string, buyRate, sellRate float64, confidence float64, route interface{}))
}

// SyntheticRateCalculator calculates synthetic rates for cross-pair trading
type SyntheticRateCalculator struct {
	routes map[string]*CrossPairRoute
	mutex  sync.RWMutex
}

// NewSyntheticRateCalculator creates a new synthetic rate calculator
func NewSyntheticRateCalculator() *SyntheticRateCalculator {
	return &SyntheticRateCalculator{
		routes: make(map[string]*CrossPairRoute),
	}
}

// CalculateRate calculates the synthetic rate between two assets
func (src *SyntheticRateCalculator) CalculateRate(fromAsset, toAsset string, quantity decimal.Decimal) (*RateCalculationResult, error) {
	src.mutex.RLock()
	defer src.mutex.RUnlock()

	routeKey := fromAsset + "-" + toAsset
	route, exists := src.routes[routeKey]
	if !exists {
		return nil, ErrRouteNotFound
	}

	// Calculate estimated slippage based on quantity
	estimatedSlippage := decimal.NewFromFloat(0.001) // 0.1% default slippage
	if quantity.GreaterThan(decimal.NewFromFloat(10000)) {
		estimatedSlippage = decimal.NewFromFloat(0.005) // 0.5% for large orders
	}

	// Estimate liquidity in USD (simplified)
	liquidityUSD := decimal.NewFromFloat(100000) // $100k default liquidity

	result := &RateCalculationResult{
		Route:             route,
		EstimatedSlippage: estimatedSlippage,
		Confidence:        route.Confidence,
		LiquidityUSD:      liquidityUSD,
		EstimatedFees:     []CrossPairFee{}, // Empty for now
	}

	return result, nil
}

// GetBestRoute returns the best route for trading between two assets
func (src *SyntheticRateCalculator) GetBestRoute(fromAsset, toAsset string) (*CrossPairRoute, error) {
	src.mutex.RLock()
	defer src.mutex.RUnlock()

	routeKey := fromAsset + "-" + toAsset
	route, exists := src.routes[routeKey]
	if !exists {
		return nil, ErrRouteNotFound
	}

	return route, nil
}

// CalculateRateWithDetails calculates the synthetic rate with detailed information
func (src *SyntheticRateCalculator) CalculateRateWithDetails(fromAsset, toAsset string, quantity decimal.Decimal) (*RateCalculationResult, error) {
	src.mutex.RLock()
	defer src.mutex.RUnlock()

	routeKey := fromAsset + "-" + toAsset
	route, exists := src.routes[routeKey]
	if !exists {
		return nil, ErrRouteNotFound
	}

	// Calculate estimated slippage based on quantity
	estimatedSlippage := decimal.NewFromFloat(0.001) // 0.1% default slippage
	if quantity.GreaterThan(decimal.NewFromFloat(10000)) {
		estimatedSlippage = decimal.NewFromFloat(0.005) // 0.5% for large orders
	}

	// Estimate liquidity in USD (simplified)
	liquidityUSD := decimal.NewFromFloat(100000) // $100k default liquidity

	result := &RateCalculationResult{
		Route:             route,
		EstimatedSlippage: estimatedSlippage,
		Confidence:        route.Confidence,
		LiquidityUSD:      liquidityUSD,
		EstimatedFees:     []CrossPairFee{}, // Empty for now
	}

	return result, nil
}

// UpdateRoute updates a route in the calculator
func (src *SyntheticRateCalculator) UpdateRoute(route *CrossPairRoute) {
	src.mutex.Lock()
	defer src.mutex.Unlock()

	routeKey := route.BaseAsset + "-" + route.FirstPair
	src.routes[routeKey] = route
}

// GetCacheStats returns cache statistics
func (src *SyntheticRateCalculator) GetCacheStats() map[string]interface{} {
	src.mutex.RLock()
	defer src.mutex.RUnlock()

	return map[string]interface{}{
		"total_routes": len(src.routes),
		"cache_hits":   0, // placeholder
		"cache_misses": 0, // placeholder
	}
}

// Subscribe subscribes to rate updates (placeholder implementation)
func (src *SyntheticRateCalculator) Subscribe(callback func(pair string, buyRate, sellRate float64, confidence float64, route interface{})) {
	// Placeholder implementation - in a real implementation, this would
	// register the callback to receive rate updates from the underlying rate sources
	// For now, this is just a no-op to satisfy the interface
}

// MatchingEngine interface for executing spot trades
type MatchingEngine interface {
	ExecuteMarketOrder(ctx context.Context, order *model.Order) (*model.Trade, error)
	ExecuteLimitOrder(ctx context.Context, order *model.Order) (*model.Trade, error)
	GetOrderbook(ctx context.Context) (*OrderBookSnapshot, error)
}

// CrossPairEngineConfig contains configuration for the cross-pair engine
type CrossPairEngineConfig struct {
	MaxConcurrentExecutions int
	ExecutionTimeout        time.Duration
	MaxSlippageTolerance    decimal.Decimal
	MinOrderValue           decimal.Decimal
	MaxOrderValue           decimal.Decimal
	DefaultExpiration       time.Duration
	RetryAttempts           int
	RetryDelay              time.Duration
	MaxConcurrentOrders     int
	OrderTimeout            time.Duration
	QueueSize               int
}

// OrderStats contains statistics about orders
type OrderStats struct {
	TotalOrders     int64              `json:"total_orders"`
	CompletedOrders int64              `json:"completed_orders"`
	CancelledOrders int64              `json:"cancelled_orders"`
	TotalVolume     float64            `json:"total_volume"`
	TotalFees       float64            `json:"total_fees"`
	VolumeByPair    map[string]float64 `json:"volume_by_pair"`
}

// VolumeStats contains statistics about trading volume
type VolumeStats struct {
	TotalVolume   float64            `json:"total_volume"`
	VolumeByPair  map[string]float64 `json:"volume_by_pair"`
	TradeCount    int64              `json:"trade_count"`
	UniqueTraders int64              `json:"unique_traders"`
}
