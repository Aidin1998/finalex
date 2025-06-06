// Package contracts defines unified interface contracts for module integration
package contracts

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// TradingServiceContract defines the standardized interface for trading module
type TradingServiceContract interface {
	// Order Management
	PlaceOrder(ctx context.Context, req *PlaceOrderRequest) (*OrderResponse, error)
	CancelOrder(ctx context.Context, req *CancelOrderRequest) error
	ModifyOrder(ctx context.Context, req *ModifyOrderRequest) (*OrderResponse, error)
	GetOrder(ctx context.Context, orderID uuid.UUID) (*Order, error)
	GetUserOrders(ctx context.Context, userID uuid.UUID, filter *OrderFilter) ([]*Order, int64, error)

	// Market Data
	GetOrderBook(ctx context.Context, symbol string, depth int) (*OrderBook, error)
	GetTicker(ctx context.Context, symbol string) (*Ticker, error)
	GetTrades(ctx context.Context, symbol string, limit int) ([]*Trade, error)
	GetUserTrades(ctx context.Context, userID uuid.UUID, filter *TradeFilter) ([]*Trade, int64, error)

	// Market Management
	GetMarkets(ctx context.Context) ([]*Market, error)
	GetMarket(ctx context.Context, symbol string) (*Market, error)
	GetMarketStats(ctx context.Context, symbol string) (*MarketStats, error)

	// Risk Management
	ValidateOrder(ctx context.Context, req *PlaceOrderRequest) (*OrderValidation, error)
	GetUserRiskProfile(ctx context.Context, userID uuid.UUID) (*RiskProfile, error)
	CheckTradeCompliance(ctx context.Context, req *ComplianceCheckRequest) (*ComplianceResult, error)

	// Settlement & Clearing
	SettleTrade(ctx context.Context, tradeID uuid.UUID) error
	GetPendingSettlements(ctx context.Context) ([]*PendingSettlement, error)
	ReconcileTrades(ctx context.Context, req *TradeReconciliationRequest) (*TradeReconciliationResult, error)

	// Performance Metrics
	GetTradingMetrics(ctx context.Context, req *MetricsRequest) (*TradingMetrics, error)
	GetLatencyMetrics(ctx context.Context) (*LatencyMetrics, error)

	// Service Health
	HealthCheck(ctx context.Context) (*HealthStatus, error)
}

// PlaceOrderRequest represents order placement request
type PlaceOrderRequest struct {
	UserID        uuid.UUID              `json:"user_id"`
	Symbol        string                 `json:"symbol"`
	Side          string                 `json:"side"` // buy, sell
	Type          string                 `json:"type"` // market, limit, stop, stop_limit
	Quantity      decimal.Decimal        `json:"quantity"`
	Price         *decimal.Decimal       `json:"price,omitempty"`
	StopPrice     *decimal.Decimal       `json:"stop_price,omitempty"`
	TimeInForce   string                 `json:"time_in_force"` // GTC, IOC, FOK
	PostOnly      bool                   `json:"post_only"`
	ReduceOnly    bool                   `json:"reduce_only"`
	ClientOrderID string                 `json:"client_order_id,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

// CancelOrderRequest represents order cancellation request
type CancelOrderRequest struct {
	OrderID       *uuid.UUID `json:"order_id,omitempty"`
	ClientOrderID *string    `json:"client_order_id,omitempty"`
	UserID        uuid.UUID  `json:"user_id"`
	Symbol        *string    `json:"symbol,omitempty"`
}

// ModifyOrderRequest represents order modification request
type ModifyOrderRequest struct {
	OrderID     uuid.UUID        `json:"order_id"`
	UserID      uuid.UUID        `json:"user_id"`
	NewQuantity *decimal.Decimal `json:"new_quantity,omitempty"`
	NewPrice    *decimal.Decimal `json:"new_price,omitempty"`
}

// OrderResponse represents order operation response
type OrderResponse struct {
	OrderID       uuid.UUID        `json:"order_id"`
	ClientOrderID string           `json:"client_order_id,omitempty"`
	Symbol        string           `json:"symbol"`
	Side          string           `json:"side"`
	Type          string           `json:"type"`
	Quantity      decimal.Decimal  `json:"quantity"`
	Price         *decimal.Decimal `json:"price,omitempty"`
	Status        string           `json:"status"`
	FilledQty     decimal.Decimal  `json:"filled_quantity"`
	RemainingQty  decimal.Decimal  `json:"remaining_quantity"`
	AvgPrice      decimal.Decimal  `json:"average_price"`
	CreatedAt     time.Time        `json:"created_at"`
	UpdatedAt     time.Time        `json:"updated_at"`
	Trades        []*Trade         `json:"trades,omitempty"`
}

// Order represents trading order
type Order struct {
	ID            uuid.UUID        `json:"id"`
	UserID        uuid.UUID        `json:"user_id"`
	Symbol        string           `json:"symbol"`
	Side          string           `json:"side"`
	Type          string           `json:"type"`
	Quantity      decimal.Decimal  `json:"quantity"`
	Price         *decimal.Decimal `json:"price,omitempty"`
	StopPrice     *decimal.Decimal `json:"stop_price,omitempty"`
	TimeInForce   string           `json:"time_in_force"`
	Status        string           `json:"status"`
	FilledQty     decimal.Decimal  `json:"filled_quantity"`
	RemainingQty  decimal.Decimal  `json:"remaining_quantity"`
	AvgPrice      decimal.Decimal  `json:"average_price"`
	Commission    decimal.Decimal  `json:"commission"`
	ClientOrderID string           `json:"client_order_id,omitempty"`
	CreatedAt     time.Time        `json:"created_at"`
	UpdatedAt     time.Time        `json:"updated_at"`
	ExpiresAt     *time.Time       `json:"expires_at,omitempty"`
}

// Trade represents executed trade
type Trade struct {
	ID         uuid.UUID       `json:"id"`
	Symbol     string          `json:"symbol"`
	OrderID    uuid.UUID       `json:"order_id"`
	BuyerID    uuid.UUID       `json:"buyer_id"`
	SellerID   uuid.UUID       `json:"seller_id"`
	Price      decimal.Decimal `json:"price"`
	Quantity   decimal.Decimal `json:"quantity"`
	Commission decimal.Decimal `json:"commission"`
	Role       string          `json:"role"` // maker, taker
	Status     string          `json:"status"`
	ExecutedAt time.Time       `json:"executed_at"`
	SettledAt  *time.Time      `json:"settled_at,omitempty"`
}

// OrderBook represents market order book
type OrderBook struct {
	Symbol    string           `json:"symbol"`
	Sequence  int64            `json:"sequence"`
	Timestamp time.Time        `json:"timestamp"`
	Bids      []OrderBookLevel `json:"bids"`
	Asks      []OrderBookLevel `json:"asks"`
}

// OrderBookLevel represents order book price level
type OrderBookLevel struct {
	Price    decimal.Decimal `json:"price"`
	Quantity decimal.Decimal `json:"quantity"`
	Orders   int             `json:"orders"`
}

// Ticker represents market ticker data
type Ticker struct {
	Symbol     string          `json:"symbol"`
	LastPrice  decimal.Decimal `json:"last_price"`
	BestBid    decimal.Decimal `json:"best_bid"`
	BestAsk    decimal.Decimal `json:"best_ask"`
	Volume24h  decimal.Decimal `json:"volume_24h"`
	Change24h  decimal.Decimal `json:"change_24h"`
	ChangePerc decimal.Decimal `json:"change_percentage"`
	High24h    decimal.Decimal `json:"high_24h"`
	Low24h     decimal.Decimal `json:"low_24h"`
	Timestamp  time.Time       `json:"timestamp"`
}

// Market represents trading market
type Market struct {
	Symbol        string          `json:"symbol"`
	BaseCurrency  string          `json:"base_currency"`
	QuoteCurrency string          `json:"quote_currency"`
	Status        string          `json:"status"`
	MinOrderSize  decimal.Decimal `json:"min_order_size"`
	MaxOrderSize  decimal.Decimal `json:"max_order_size"`
	PriceStep     decimal.Decimal `json:"price_step"`
	QuantityStep  decimal.Decimal `json:"quantity_step"`
	MakerFee      decimal.Decimal `json:"maker_fee"`
	TakerFee      decimal.Decimal `json:"taker_fee"`
	CreatedAt     time.Time       `json:"created_at"`
}

// OrderFilter represents order filtering options
type OrderFilter struct {
	Symbol    *string    `json:"symbol,omitempty"`
	Status    *string    `json:"status,omitempty"`
	Side      *string    `json:"side,omitempty"`
	Type      *string    `json:"type,omitempty"`
	StartTime *time.Time `json:"start_time,omitempty"`
	EndTime   *time.Time `json:"end_time,omitempty"`
	Limit     int        `json:"limit"`
	Offset    int        `json:"offset"`
}

// TradeFilter represents trade filtering options
type TradeFilter struct {
	Symbol    *string    `json:"symbol,omitempty"`
	StartTime *time.Time `json:"start_time,omitempty"`
	EndTime   *time.Time `json:"end_time,omitempty"`
	Limit     int        `json:"limit"`
	Offset    int        `json:"offset"`
}

// MarketStats represents market statistics
type MarketStats struct {
	Symbol        string          `json:"symbol"`
	Volume24h     decimal.Decimal `json:"volume_24h"`
	High24h       decimal.Decimal `json:"high_24h"`
	Low24h        decimal.Decimal `json:"low_24h"`
	Change24h     decimal.Decimal `json:"change_24h"`
	ChangePerc24h decimal.Decimal `json:"change_percentage_24h"`
	TradeCount24h int64           `json:"trade_count_24h"`
	LastPrice     decimal.Decimal `json:"last_price"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

// OrderValidation represents order validation result
type OrderValidation struct {
	Valid           bool            `json:"valid"`
	Errors          []string        `json:"errors,omitempty"`
	Warnings        []string        `json:"warnings,omitempty"`
	EstimatedFee    decimal.Decimal `json:"estimated_fee"`
	RequiredBalance decimal.Decimal `json:"required_balance"`
}

// RiskProfile represents user risk profile
type RiskProfile struct {
	UserID            uuid.UUID       `json:"user_id"`
	RiskLevel         string          `json:"risk_level"`
	MaxOrderSize      decimal.Decimal `json:"max_order_size"`
	MaxDailyVolume    decimal.Decimal `json:"max_daily_volume"`
	MaxOpenOrders     int             `json:"max_open_orders"`
	AllowedSymbols    []string        `json:"allowed_symbols"`
	RestrictedSymbols []string        `json:"restricted_symbols"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

// ComplianceCheckRequest represents compliance check request
type ComplianceCheckRequest struct {
	UserID    uuid.UUID              `json:"user_id"`
	Symbol    string                 `json:"symbol"`
	Amount    decimal.Decimal        `json:"amount"`
	Operation string                 `json:"operation"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// ComplianceResult represents compliance check result
type ComplianceResult struct {
	Approved bool     `json:"approved"`
	Reason   string   `json:"reason,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
	Flags    []string `json:"flags,omitempty"`
}

// PendingSettlement represents pending trade settlement
type PendingSettlement struct {
	TradeID     uuid.UUID       `json:"trade_id"`
	Symbol      string          `json:"symbol"`
	BuyerID     uuid.UUID       `json:"buyer_id"`
	SellerID    uuid.UUID       `json:"seller_id"`
	Amount      decimal.Decimal `json:"amount"`
	Status      string          `json:"status"`
	CreatedAt   time.Time       `json:"created_at"`
	RetryCount  int             `json:"retry_count"`
	LastAttempt *time.Time      `json:"last_attempt,omitempty"`
}

// TradeReconciliationRequest represents trade reconciliation request
type TradeReconciliationRequest struct {
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	Symbols   []string  `json:"symbols,omitempty"`
	Force     bool      `json:"force"`
}

// TradeReconciliationResult represents reconciliation result
type TradeReconciliationResult struct {
	ProcessedTrades  int                        `json:"processed_trades"`
	ReconciledTrades int                        `json:"reconciled_trades"`
	Discrepancies    []TradeDiscrepancy         `json:"discrepancies,omitempty"`
	Summary          map[string]decimal.Decimal `json:"summary"`
	ProcessedAt      time.Time                  `json:"processed_at"`
}

// TradeDiscrepancy represents trade discrepancy
type TradeDiscrepancy struct {
	TradeID     uuid.UUID       `json:"trade_id"`
	Type        string          `json:"type"`
	Expected    decimal.Decimal `json:"expected"`
	Actual      decimal.Decimal `json:"actual"`
	Difference  decimal.Decimal `json:"difference"`
	Description string          `json:"description"`
}

// MetricsRequest represents metrics request
type MetricsRequest struct {
	StartTime time.Time  `json:"start_time"`
	EndTime   time.Time  `json:"end_time"`
	Symbols   []string   `json:"symbols,omitempty"`
	UserID    *uuid.UUID `json:"user_id,omitempty"`
}

// TradingMetrics represents trading performance metrics
type TradingMetrics struct {
	TotalOrders      int64                    `json:"total_orders"`
	FilledOrders     int64                    `json:"filled_orders"`
	TotalTrades      int64                    `json:"total_trades"`
	TotalVolume      decimal.Decimal          `json:"total_volume"`
	AverageOrderSize decimal.Decimal          `json:"average_order_size"`
	FillRate         decimal.Decimal          `json:"fill_rate"`
	SymbolMetrics    map[string]SymbolMetrics `json:"symbol_metrics"`
	GeneratedAt      time.Time                `json:"generated_at"`
}

// SymbolMetrics represents per-symbol metrics
type SymbolMetrics struct {
	Symbol      string          `json:"symbol"`
	Volume      decimal.Decimal `json:"volume"`
	TradeCount  int64           `json:"trade_count"`
	OrderCount  int64           `json:"order_count"`
	AvgSpread   decimal.Decimal `json:"average_spread"`
	PriceChange decimal.Decimal `json:"price_change"`
}

// LatencyMetrics represents system latency metrics
type LatencyMetrics struct {
	OrderProcessing LatencyStats `json:"order_processing"`
	TradeExecution  LatencyStats `json:"trade_execution"`
	OrderBookUpdate LatencyStats `json:"order_book_update"`
	DatabaseWrite   LatencyStats `json:"database_write"`
	GeneratedAt     time.Time    `json:"generated_at"`
}

// LatencyStats represents latency statistics
type LatencyStats struct {
	Mean   time.Duration `json:"mean"`
	Median time.Duration `json:"median"`
	P95    time.Duration `json:"p95"`
	P99    time.Duration `json:"p99"`
	Max    time.Duration `json:"max"`
	Min    time.Duration `json:"min"`
}
