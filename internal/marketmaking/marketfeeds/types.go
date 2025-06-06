package marketfeeds

import (
	"context"
	"time"

	"github.com/Aidin1998/finalex/pkg/models"
	"github.com/shopspring/decimal"
)

// ExchangeProvider defines the interface for external exchange integrations
type ExchangeProvider interface {
	// GetTicker gets 24hr ticker price change statistics for a symbol
	GetTicker(ctx context.Context, symbol string) (*TickerData, error)

	// GetOrderBook gets order book for a symbol
	GetOrderBook(ctx context.Context, symbol string, depth int) (*OrderBookData, error)

	// GetKlines gets kline/candlestick data for a symbol
	GetKlines(ctx context.Context, symbol, interval string, limit int) ([]*KlineData, error)

	// GetTrades gets recent trades for a symbol
	GetTrades(ctx context.Context, symbol string, limit int) ([]*TradeData, error)

	// SubscribeToUpdates subscribes to real-time updates
	SubscribeToUpdates(ctx context.Context, symbols []string, callback func(*MarketUpdate)) error

	// GetName returns the exchange name
	GetName() string

	// GetPriority returns the exchange priority (lower = higher priority)
	GetPriority() int

	// IsHealthy checks if the exchange connection is healthy
	IsHealthy() bool
}

// FiatRateProvider defines the interface for fiat currency rate providers
type FiatRateProvider interface {
	// GetRates gets fiat currency rates vs base currency (USD)
	GetRates(ctx context.Context, currencies []string) (map[string]decimal.Decimal, error)

	// GetRate gets a specific fiat currency rate vs base currency
	GetRate(ctx context.Context, currency string) (decimal.Decimal, error)

	// GetName returns the provider name
	GetName() string

	// IsHealthy checks if the provider connection is healthy
	IsHealthy() bool
}

// Enhanced data structures
type TickerData struct {
	Symbol             string          `json:"symbol"`
	Price              decimal.Decimal `json:"price"`
	PriceChange        decimal.Decimal `json:"price_change"`
	PriceChangePercent decimal.Decimal `json:"price_change_percent"`
	WeightedAvgPrice   decimal.Decimal `json:"weighted_avg_price"`
	PrevClosePrice     decimal.Decimal `json:"prev_close_price"`
	LastPrice          decimal.Decimal `json:"last_price"`
	LastQty            decimal.Decimal `json:"last_qty"`
	BidPrice           decimal.Decimal `json:"bid_price"`
	BidQty             decimal.Decimal `json:"bid_qty"`
	AskPrice           decimal.Decimal `json:"ask_price"`
	AskQty             decimal.Decimal `json:"ask_qty"`
	OpenPrice          decimal.Decimal `json:"open_price"`
	HighPrice          decimal.Decimal `json:"high_price"`
	LowPrice           decimal.Decimal `json:"low_price"`
	Volume             decimal.Decimal `json:"volume"`
	QuoteVolume        decimal.Decimal `json:"quote_volume"`
	OpenTime           int64           `json:"open_time"`
	CloseTime          int64           `json:"close_time"`
	Count              int             `json:"count"`
	Timestamp          time.Time       `json:"timestamp"`
	Source             string          `json:"source"`
}

type OrderBookData struct {
	Symbol       string       `json:"symbol"`
	Bids         []PriceLevel `json:"bids"`
	Asks         []PriceLevel `json:"asks"`
	LastUpdateID int64        `json:"last_update_id"`
	Timestamp    time.Time    `json:"timestamp"`
	Source       string       `json:"source"`
}

type PriceLevel struct {
	Price    decimal.Decimal `json:"price"`
	Quantity decimal.Decimal `json:"quantity"`
	Count    int             `json:"count,omitempty"`
}

type KlineData struct {
	Symbol        string          `json:"symbol"`
	Interval      string          `json:"interval"`
	OpenTime      int64           `json:"open_time"`
	CloseTime     int64           `json:"close_time"`
	Open          decimal.Decimal `json:"open"`
	High          decimal.Decimal `json:"high"`
	Low           decimal.Decimal `json:"low"`
	Close         decimal.Decimal `json:"close"`
	Volume        decimal.Decimal `json:"volume"`
	QuoteVolume   decimal.Decimal `json:"quote_volume"`
	TradeCount    int             `json:"trade_count"`
	TakerBuyBase  decimal.Decimal `json:"taker_buy_base"`
	TakerBuyQuote decimal.Decimal `json:"taker_buy_quote"`
	IsClosed      bool            `json:"is_closed"`
	Timestamp     time.Time       `json:"timestamp"`
	Source        string          `json:"source"`
}

type TradeData struct {
	ID           string          `json:"id"`
	Symbol       string          `json:"symbol"`
	Price        decimal.Decimal `json:"price"`
	Quantity     decimal.Decimal `json:"quantity"`
	QuoteQty     decimal.Decimal `json:"quote_qty"`
	Time         int64           `json:"time"`
	IsBuyerMaker bool            `json:"is_buyer_maker"`
	Timestamp    time.Time       `json:"timestamp"`
	Source       string          `json:"source"`
}

type MarketUpdate struct {
	Type      string      `json:"type"` // ticker, orderbook, trade, kline
	Symbol    string      `json:"symbol"`
	Data      interface{} `json:"data"`
	Timestamp time.Time   `json:"timestamp"`
	Source    string      `json:"source"`
}

// Aggregated market data
type AggregatedTicker struct {
	Symbol             string                 `json:"symbol"`
	Price              decimal.Decimal        `json:"price"` // Volume weighted average price
	PriceChange        decimal.Decimal        `json:"price_change"`
	PriceChangePercent decimal.Decimal        `json:"price_change_percent"`
	WeightedAvgPrice   decimal.Decimal        `json:"weighted_avg_price"`
	BidPrice           decimal.Decimal        `json:"bid_price"`
	AskPrice           decimal.Decimal        `json:"ask_price"`
	Spread             decimal.Decimal        `json:"spread"`
	HighPrice          decimal.Decimal        `json:"high_price"`
	LowPrice           decimal.Decimal        `json:"low_price"`
	Volume             decimal.Decimal        `json:"volume"`
	QuoteVolume        decimal.Decimal        `json:"quote_volume"`
	UpdatedAt          time.Time              `json:"updated_at"`
	Sources            map[string]*TickerData `json:"sources"`
	QualityScore       float64                `json:"quality_score"`
	DataFreshness      time.Duration          `json:"data_freshness"`
}

type AggregatedOrderBook struct {
	Symbol       string                    `json:"symbol"`
	Bids         []PriceLevel              `json:"bids"`
	Asks         []PriceLevel              `json:"asks"`
	Spread       decimal.Decimal           `json:"spread"`
	MidPrice     decimal.Decimal           `json:"mid_price"`
	UpdatedAt    time.Time                 `json:"updated_at"`
	Sources      map[string]*OrderBookData `json:"sources"`
	QualityScore float64                   `json:"quality_score"`
}

// Fiat currency data
type FiatRate struct {
	Currency   string          `json:"currency"`
	Rate       decimal.Decimal `json:"rate"`         // Rate vs USD
	RateVsBTC  decimal.Decimal `json:"rate_vs_btc"`  // Rate vs BTC
	RateVsUSDT decimal.Decimal `json:"rate_vs_usdt"` // Rate vs USDT
	Change24h  decimal.Decimal `json:"change_24h"`
	UpdatedAt  time.Time       `json:"updated_at"`
	Source     string          `json:"source"`
}

// Feed quality monitoring
type FeedQuality struct {
	Source        string        `json:"source"`
	Symbol        string        `json:"symbol"`
	Latency       time.Duration `json:"latency"`
	UpdateRate    float64       `json:"update_rate"`    // Updates per second
	ErrorRate     float64       `json:"error_rate"`     // Error percentage
	DataFreshness time.Duration `json:"data_freshness"` // Time since last update
	IsHealthy     bool          `json:"is_healthy"`
	LastError     string        `json:"last_error,omitempty"`
	LastUpdated   time.Time     `json:"last_updated"`
}

// Performance metrics
type PerformanceMetrics struct {
	TotalRequests      int64         `json:"total_requests"`
	SuccessfulRequests int64         `json:"successful_requests"`
	FailedRequests     int64         `json:"failed_requests"`
	AverageLatency     time.Duration `json:"average_latency"`
	P95Latency         time.Duration `json:"p95_latency"`
	P99Latency         time.Duration `json:"p99_latency"`
	RequestsPerSecond  float64       `json:"requests_per_second"`
	ActiveConnections  int           `json:"active_connections"`
	DataFreshness      time.Duration `json:"data_freshness"`
	LastUpdated        time.Time     `json:"last_updated"`
}

// Configuration structures
type ExchangeConfig struct {
	Name      string            `json:"name"`
	Enabled   bool              `json:"enabled"`
	Priority  int               `json:"priority"`
	Weight    float64           `json:"weight"`     // For aggregation
	RateLimit int               `json:"rate_limit"` // Requests per second
	Timeout   time.Duration     `json:"timeout"`
	Symbols   []string          `json:"symbols"`
	Endpoints map[string]string `json:"endpoints"`
	APIKey    string            `json:"api_key,omitempty"`
	SecretKey string            `json:"secret_key,omitempty"`
	TestMode  bool              `json:"test_mode"`
}

type FiatConfig struct {
	Currencies     []string      `json:"currencies"`
	Providers      []string      `json:"providers"`
	UpdateInterval time.Duration `json:"update_interval"`
	RetryAttempts  int           `json:"retry_attempts"`
	Timeout        time.Duration `json:"timeout"`
}

type ServiceConfig struct {
	Exchanges           []ExchangeConfig `json:"exchanges"`
	Fiat                FiatConfig       `json:"fiat"`
	UpdateInterval      time.Duration    `json:"update_interval"`
	AggregationInterval time.Duration    `json:"aggregation_interval"`
	MaxDataAge          time.Duration    `json:"max_data_age"`
	HealthCheckInterval time.Duration    `json:"health_check_interval"`
	AlertThresholds     AlertThresholds  `json:"alert_thresholds"`
	CacheEnabled        bool             `json:"cache_enabled"`
	CacheTTL            time.Duration    `json:"cache_ttl"`
	MetricsEnabled      bool             `json:"metrics_enabled"`
}

type AlertThresholds struct {
	MaxLatency      time.Duration `json:"max_latency"`
	MaxErrorRate    float64       `json:"max_error_rate"`
	MinUpdateRate   float64       `json:"min_update_rate"`
	MaxDataAge      time.Duration `json:"max_data_age"`
	MinQualityScore float64       `json:"min_quality_score"`
}

// Failover and alerting
type Alert struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type"` // latency, error_rate, data_freshness, quality_score
	Source      string                 `json:"source"`
	Symbol      string                 `json:"symbol"`
	Message     string                 `json:"message"`
	Severity    string                 `json:"severity"` // low, medium, high, critical
	Threshold   interface{}            `json:"threshold"`
	ActualValue interface{}            `json:"actual_value"`
	Timestamp   time.Time              `json:"timestamp"`
	Resolved    bool                   `json:"resolved"`
	ResolvedAt  *time.Time             `json:"resolved_at,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// Enhanced service interface
type EnhancedMarketFeedService interface {
	// Base functionality
	Start() error
	Stop() error
	GetMarketPrices(ctx context.Context) ([]*models.MarketPrice, error)
	GetMarketPrice(ctx context.Context, symbol string) (*models.MarketPrice, error)
	GetCandles(ctx context.Context, symbol, interval string, limit int) ([]*models.Candle, error)

	// Enhanced functionality
	GetAggregatedTicker(ctx context.Context, symbol string) (*AggregatedTicker, error)
	GetAggregatedOrderBook(ctx context.Context, symbol string, depth int) (*AggregatedOrderBook, error)
	GetExchangeTickers(ctx context.Context, symbol string) (map[string]*TickerData, error)
	GetFiatRates(ctx context.Context) (map[string]*FiatRate, error)
	GetFiatRate(ctx context.Context, currency string) (*FiatRate, error)
	ConvertPrice(ctx context.Context, amount decimal.Decimal, fromCurrency, toCurrency string) (decimal.Decimal, error)

	// Quality monitoring
	GetFeedQuality(ctx context.Context) (map[string]*FeedQuality, error)
	GetPerformanceMetrics(ctx context.Context) (*PerformanceMetrics, error)
	GetAlerts(ctx context.Context) ([]*Alert, error)

	// Exchange management
	AddExchangeProvider(provider ExchangeProvider) error
	RemoveExchangeProvider(name string) error
	GetExchangeStatus(ctx context.Context) (map[string]bool, error)

	// Fiat rate management
	AddFiatRateProvider(provider FiatRateProvider) error
	RemoveFiatRateProvider(name string) error

	// Real-time subscriptions
	SubscribeToTicker(ctx context.Context, symbol string) (<-chan *AggregatedTicker, error)
	SubscribeToOrderBook(ctx context.Context, symbol string) (<-chan *AggregatedOrderBook, error)
	SubscribeToAlerts(ctx context.Context) (<-chan *Alert, error)

	// Configuration management
	UpdateConfig(config *ServiceConfig) error
	GetConfig() *ServiceConfig

	// Health checks
	HealthCheck(ctx context.Context) error
}
