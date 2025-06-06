// Package common provides unified interfaces and types for market making strategies
package common

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Quote represents a legacy quote structure for backward compatibility
type Quote struct {
	BidPrice  float64   `json:"bid_price"`
	AskPrice  float64   `json:"ask_price"`
	BidSize   float64   `json:"bid_size"`
	AskSize   float64   `json:"ask_size"`
	Pair      string    `json:"pair"`
	Timestamp time.Time `json:"timestamp"`
}

// MarketMakingStrategy is the unified interface that all market making strategies must implement
type MarketMakingStrategy interface {
	// Core strategy methods
	Quote(ctx context.Context, input QuoteInput) (*QuoteOutput, error)
	Initialize(ctx context.Context, config StrategyConfig) error
	Start(ctx context.Context) error
	Stop(ctx context.Context) error

	// Strategy lifecycle
	OnMarketData(ctx context.Context, data *MarketData) error
	OnOrderFill(ctx context.Context, fill *OrderFill) error
	OnOrderCancel(ctx context.Context, orderID uuid.UUID, reason string) error

	// Configuration and management
	UpdateConfig(ctx context.Context, config StrategyConfig) error
	GetConfig() StrategyConfig
	GetMetrics() *StrategyMetrics
	GetStatus() StrategyStatus

	// Metadata
	Name() string
	Version() string
	Description() string
	RiskLevel() RiskLevel

	// Health and monitoring
	HealthCheck(ctx context.Context) *HealthStatus
	Reset(ctx context.Context) error
}

// QuoteInput contains all data needed for strategy quote generation
type QuoteInput struct {
	Symbol           string          `json:"symbol"`
	Pair             string          `json:"pair"`
	BidPrice         decimal.Decimal `json:"bid_price"`
	AskPrice         decimal.Decimal `json:"ask_price"`
	MidPrice         decimal.Decimal `json:"mid_price"`
	Volume           decimal.Decimal `json:"volume"`
	Volatility       decimal.Decimal `json:"volatility"`
	Inventory        decimal.Decimal `json:"inventory"`
	MaxInventory     decimal.Decimal `json:"max_inventory"`
	MinSpread        decimal.Decimal `json:"min_spread"`
	MaxSpread        decimal.Decimal `json:"max_spread"`
	OrderSize        decimal.Decimal `json:"order_size"`
	MarketConditions *MarketContext  `json:"market_conditions,omitempty"`
	RiskParameters   *RiskParams     `json:"risk_parameters,omitempty"`
	Timestamp        time.Time       `json:"timestamp"`
}

// QuoteOutput contains the generated quotes from a strategy
type QuoteOutput struct {
	BidPrice    decimal.Decimal        `json:"bid_price"`
	AskPrice    decimal.Decimal        `json:"ask_price"`
	BidSize     decimal.Decimal        `json:"bid_size"`
	AskSize     decimal.Decimal        `json:"ask_size"`
	Confidence  decimal.Decimal        `json:"confidence"` // 0-1 confidence in quote quality
	TTL         time.Duration          `json:"ttl"`        // Time to live for the quote
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	GeneratedAt time.Time              `json:"generated_at"`
}

// StrategyConfig holds configuration for any strategy
type StrategyConfig struct {
	ID         string                 `json:"id"`
	Name       string                 `json:"name"`
	Type       string                 `json:"type"`
	Pair       string                 `json:"pair"`
	Parameters map[string]interface{} `json:"parameters"`
	RiskLimits *RiskLimits            `json:"risk_limits,omitempty"`
	Enabled    bool                   `json:"enabled"`
	Priority   int                    `json:"priority"`
	UpdatedAt  time.Time              `json:"updated_at"`
}

// StrategyMetrics contains performance and operational metrics
type StrategyMetrics struct {
	// Performance metrics
	TotalPnL    decimal.Decimal `json:"total_pnl"`
	DailyPnL    decimal.Decimal `json:"daily_pnl"`
	SharpeRatio decimal.Decimal `json:"sharpe_ratio"`
	MaxDrawdown decimal.Decimal `json:"max_drawdown"`
	WinRate     decimal.Decimal `json:"win_rate"`

	// Operational metrics
	OrdersPlaced    int64           `json:"orders_placed"`
	OrdersFilled    int64           `json:"orders_filled"`
	OrdersCancelled int64           `json:"orders_cancelled"`
	SuccessRate     decimal.Decimal `json:"success_rate"`
	AvgFillTime     time.Duration   `json:"avg_fill_time"`

	// Market making specific
	SpreadCapture     decimal.Decimal `json:"spread_capture"`
	InventoryTurnover decimal.Decimal `json:"inventory_turnover"`
	QuoteUptime       decimal.Decimal `json:"quote_uptime"`

	// Timing
	LastUpdated    time.Time     `json:"last_updated"`
	LastTrade      time.Time     `json:"last_trade"`
	StrategyUptime time.Duration `json:"strategy_uptime"`
}

// MarketData represents comprehensive market data for strategy input
type MarketData struct {
	Symbol         string          `json:"symbol"`
	Price          decimal.Decimal `json:"price"`
	Volume         decimal.Decimal `json:"volume"`
	BidPrice       decimal.Decimal `json:"bid_price"`
	AskPrice       decimal.Decimal `json:"ask_price"`
	BidVolume      decimal.Decimal `json:"bid_volume"`
	AskVolume      decimal.Decimal `json:"ask_volume"`
	OrderBookDepth []PriceLevel    `json:"order_book_depth,omitempty"`
	RecentTrades   []TradeData     `json:"recent_trades,omitempty"`
	VWAP           decimal.Decimal `json:"vwap"`
	OrderImbalance decimal.Decimal `json:"order_imbalance"`
	Timestamp      time.Time       `json:"timestamp"`
}

// MarketContext provides broader market intelligence
type MarketContext struct {
	RecentTrades      []TradeData                `json:"recent_trades"`
	VolatilitySurface map[string]decimal.Decimal `json:"volatility_surface"` // tenor -> volatility
	OrderFlow         *OrderFlowMetrics          `json:"order_flow"`
	MarketRegime      RegimeType                 `json:"market_regime"`
	CrossExchangeData map[string]decimal.Decimal `json:"cross_exchange_data"`
	LiquidityScore    decimal.Decimal            `json:"liquidity_score"`
	mu                sync.RWMutex
}

// PriceLevel represents a price level in the order book
type PriceLevel struct {
	Price  decimal.Decimal `json:"price"`
	Volume decimal.Decimal `json:"volume"`
	Count  int             `json:"count"`
}

// TradeData represents individual trade information
type TradeData struct {
	Price     decimal.Decimal `json:"price"`
	Volume    decimal.Decimal `json:"volume"`
	Side      string          `json:"side"` // "buy" or "sell"
	Timestamp time.Time       `json:"timestamp"`
	TradeID   string          `json:"trade_id"`
}

// OrderFlowMetrics contains order flow analysis data
type OrderFlowMetrics struct {
	BuyImbalance     decimal.Decimal `json:"buy_imbalance"`
	SellImbalance    decimal.Decimal `json:"sell_imbalance"`
	ToxicFlow        decimal.Decimal `json:"toxic_flow"`
	InformedTrading  decimal.Decimal `json:"informed_trading"`
	MicroPriceImpact decimal.Decimal `json:"micro_price_impact"`
	FlowPersistence  decimal.Decimal `json:"flow_persistence"`
}

// OrderFill represents a filled order
type OrderFill struct {
	OrderID   uuid.UUID       `json:"order_id"`
	Symbol    string          `json:"symbol"`
	Side      string          `json:"side"`
	Price     decimal.Decimal `json:"price"`
	Quantity  decimal.Decimal `json:"quantity"`
	Fee       decimal.Decimal `json:"fee"`
	Timestamp time.Time       `json:"timestamp"`
	TradeID   string          `json:"trade_id"`
}

// RiskParams contains risk management parameters
type RiskParams struct {
	MaxPosition        decimal.Decimal `json:"max_position"`
	MaxOrderSize       decimal.Decimal `json:"max_order_size"`
	MaxDrawdown        decimal.Decimal `json:"max_drawdown"`
	VaRLimit           decimal.Decimal `json:"var_limit"`
	InventoryLimit     decimal.Decimal `json:"inventory_limit"`
	ConcentrationLimit decimal.Decimal `json:"concentration_limit"`
}

// RiskLimits defines comprehensive risk limits for strategies
type RiskLimits struct {
	MaxOrderSize     decimal.Decimal `json:"max_order_size"`
	MaxPosition      decimal.Decimal `json:"max_position"`
	MaxDailyVolume   decimal.Decimal `json:"max_daily_volume"`
	MaxDailyLoss     decimal.Decimal `json:"max_daily_loss"`
	MaxDailyPnL      decimal.Decimal `json:"max_daily_pnl"`
	MaxInventory     decimal.Decimal `json:"max_inventory"`
	MaxExposure      decimal.Decimal `json:"max_exposure"`
	MinSpread        decimal.Decimal `json:"min_spread"`
	MaxSpread        decimal.Decimal `json:"max_spread"`
	StopLossLevel    decimal.Decimal `json:"stop_loss_level"`
	VaRLimit         decimal.Decimal `json:"var_limit"`
	StressTestLimit  decimal.Decimal `json:"stress_test_limit"`
	MaxConcentration decimal.Decimal `json:"max_concentration"`
}

// StrategyStatus represents the current status of a strategy
type StrategyStatus string

const (
	StatusStopped  StrategyStatus = "stopped"
	StatusStarting StrategyStatus = "starting"
	StatusRunning  StrategyStatus = "running"
	StatusStopping StrategyStatus = "stopping"
	StatusError    StrategyStatus = "error"
	StatusPaused   StrategyStatus = "paused"
)

// RiskLevel represents the risk level of a strategy
type RiskLevel string

const (
	RiskLow      RiskLevel = "low"
	RiskMedium   RiskLevel = "medium"
	RiskHigh     RiskLevel = "high"
	RiskCritical RiskLevel = "critical"
)

// RegimeType represents different market regimes
type RegimeType string

const (
	TrendingRegime       RegimeType = "trending"
	MeanRevertingRegime  RegimeType = "mean_reverting"
	HighVolatilityRegime RegimeType = "high_volatility"
	LowLiquidityRegime   RegimeType = "low_liquidity"
	CrisisRegime         RegimeType = "crisis"
)

// HealthStatus represents the health status of a strategy
type HealthStatus struct {
	Healthy   bool                   `json:"healthy"`
	Status    string                 `json:"status"`
	LastCheck time.Time              `json:"last_check"`
	Issues    []string               `json:"issues,omitempty"`
	Metrics   map[string]interface{} `json:"metrics,omitempty"`
}

// StrategyFactory interface for creating strategy instances
type StrategyFactory interface {
	CreateStrategy(strategyType string, config StrategyConfig) (MarketMakingStrategy, error)
	GetAvailableStrategies() []StrategyMetadata
	GetStrategyMetadata(strategyType string) (*StrategyMetadata, error)
	RegisterStrategy(strategyType string, creator StrategyCreator) error
}

// StrategyCreator function type for creating strategy instances
type StrategyCreator func(config StrategyConfig) (MarketMakingStrategy, error)

// StrategyMetadata provides information about a strategy type
type StrategyMetadata struct {
	Type        string                `json:"type"`
	Name        string                `json:"name"`
	Description string                `json:"description"`
	Version     string                `json:"version"`
	RiskLevel   RiskLevel             `json:"risk_level"`
	Complexity  string                `json:"complexity"`
	Parameters  []ParameterDefinition `json:"parameters"`
	Tags        []string              `json:"tags,omitempty"`
}

// ParameterDefinition defines a strategy parameter
type ParameterDefinition struct {
	Name        string      `json:"name"`
	Type        string      `json:"type"`
	Description string      `json:"description"`
	Required    bool        `json:"required"`
	Default     interface{} `json:"default,omitempty"`
	MinValue    interface{} `json:"min_value,omitempty"`
	MaxValue    interface{} `json:"max_value,omitempty"`
	Options     []string    `json:"options,omitempty"`
}

// StrategyRegistry interface for strategy discovery and management
type StrategyRegistry interface {
	Register(strategy MarketMakingStrategy) error
	Unregister(strategyID string) error
	Get(strategyID string) (MarketMakingStrategy, error)
	List() []MarketMakingStrategy
	ListByType(strategyType string) []MarketMakingStrategy
	DiscoverStrategies() ([]StrategyMetadata, error)
}
