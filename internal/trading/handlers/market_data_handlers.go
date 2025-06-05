package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/Aidin1998/pincex_unified/common/apiutil"
	"github.com/Aidin1998/pincex_unified/internal/trading"
	"github.com/Aidin1998/pincex_unified/pkg/models"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// MarketDataHandler handles market data API endpoints
type MarketDataHandler struct {
	tradingService trading.TradingService
	logger         *zap.Logger
}

// NewMarketDataHandler creates a new market data handler
func NewMarketDataHandler(tradingService trading.TradingService, logger *zap.Logger) *MarketDataHandler {
	return &MarketDataHandler{
		tradingService: tradingService,
		logger:         logger,
	}
}

// OrderBookResponse represents the order book API response
type OrderBookResponse struct {
	Symbol    string     `json:"symbol" example:"BTCUSDT"`
	Timestamp int64      `json:"timestamp" example:"1640995200000"`
	Bids      [][]string `json:"bids" example:"[['50000.00','1.5'],['49999.99','2.0']]"`
	Asks      [][]string `json:"asks" example:"[['50001.00','1.2'],['50002.00','0.8']]"`
	Depth     int        `json:"depth" example:"20"`
}

// TickerResponse represents the ticker API response
type TickerResponse struct {
	Symbol             string `json:"symbol" example:"BTCUSDT"`
	Price              string `json:"price" example:"50000.00"`
	PriceChange        string `json:"price_change" example:"1234.56"`
	PriceChangePercent string `json:"price_change_percent" example:"2.53"`
	Volume             string `json:"volume" example:"1234.567"`
	QuoteVolume        string `json:"quote_volume" example:"61728350.00"`
	High               string `json:"high" example:"51000.00"`
	Low                string `json:"low" example:"49000.00"`
	Open               string `json:"open" example:"48765.44"`
	Close              string `json:"close" example:"50000.00"`
	Timestamp          int64  `json:"timestamp" example:"1640995200000"`
	Count              int64  `json:"count" example:"12345"`
}

// TradeResponse represents the recent trades API response
type TradeResponse struct {
	ID        string `json:"id" example:"12345"`
	Symbol    string `json:"symbol" example:"BTCUSDT"`
	Price     string `json:"price" example:"50000.00"`
	Quantity  string `json:"quantity" example:"0.001"`
	IsBuyer   bool   `json:"is_buyer" example:"true"`
	Timestamp int64  `json:"timestamp" example:"1640995200000"`
}

// KlineResponse represents the kline/candlestick API response
type KlineResponse struct {
	OpenTime    int64  `json:"open_time" example:"1640995200000"`
	Open        string `json:"open" example:"50000.00"`
	High        string `json:"high" example:"51000.00"`
	Low         string `json:"low" example:"49500.00"`
	Close       string `json:"close" example:"50500.00"`
	Volume      string `json:"volume" example:"123.456"`
	CloseTime   int64  `json:"close_time" example:"1640995259999"`
	QuoteVolume string `json:"quote_volume" example:"6234567.89"`
	Count       int64  `json:"count" example:"1234"`
}

// MarketStatsResponse represents the 24hr ticker statistics
type MarketStatsResponse struct {
	Symbol             string `json:"symbol" example:"BTCUSDT"`
	PriceChange        string `json:"price_change" example:"1234.56"`
	PriceChangePercent string `json:"price_change_percent" example:"2.53"`
	WeightedAvgPrice   string `json:"weighted_avg_price" example:"49876.54"`
	PrevClosePrice     string `json:"prev_close_price" example:"48765.44"`
	LastPrice          string `json:"last_price" example:"50000.00"`
	LastQty            string `json:"last_qty" example:"0.001"`
	BidPrice           string `json:"bid_price" example:"49999.99"`
	AskPrice           string `json:"ask_price" example:"50000.01"`
	OpenPrice          string `json:"open_price" example:"48765.44"`
	HighPrice          string `json:"high_price" example:"51000.00"`
	LowPrice           string `json:"low_price" example:"49000.00"`
	Volume             string `json:"volume" example:"1234.567"`
	QuoteVolume        string `json:"quote_volume" example:"61728350.00"`
	OpenTime           int64  `json:"open_time" example:"1640908800000"`
	CloseTime          int64  `json:"close_time" example:"1640995199999"`
	Count              int64  `json:"count" example:"12345"`
}

// GetOrderBook godoc
// @Summary Get order book depth
// @Description Get order book depth for a trading pair with configurable depth levels
// @Tags Market Data
// @Accept json
// @Produce json
// @Param symbol path string true "Trading pair symbol" example(BTCUSDT)
// @Param limit query int false "Depth limit (5, 10, 20, 50, 100, 500, 1000)" default(100)
// @Success 200 {object} OrderBookResponse
// @Failure 400 {object} ErrorResponse "Bad Request"
// @Failure 404 {object} ErrorResponse "Symbol not found"
// @Failure 500 {object} ErrorResponse "Internal Server Error"
// @Router /api/v1/depth/{symbol} [get]
func (h *MarketDataHandler) GetOrderBook(c *gin.Context) {
	startTime := time.Now()

	// Extract path parameters
	symbol := c.Param("symbol")
	if symbol == "" {
		h.logger.Warn("Missing symbol parameter")
		c.JSON(http.StatusBadRequest, apiutil.ErrorResponse{
			Error:   "INVALID_SYMBOL",
			Message: "Symbol parameter is required",
		})
		return
	}

	// Extract query parameters
	limitStr := c.DefaultQuery("limit", "100")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		h.logger.Warn("Invalid limit parameter", zap.String("limit", limitStr))
		c.JSON(http.StatusBadRequest, apiutil.ErrorResponse{
			Error:   "INVALID_LIMIT",
			Message: "Limit must be a positive integer",
		})
		return
	}

	// Validate limit values (standard exchange limits)
	validLimits := []int{5, 10, 20, 50, 100, 500, 1000}
	isValidLimit := false
	for _, validLimit := range validLimits {
		if limit == validLimit {
			isValidLimit = true
			break
		}
	}
	if !isValidLimit {
		h.logger.Warn("Invalid depth limit", zap.Int("limit", limit))
		c.JSON(http.StatusBadRequest, apiutil.ErrorResponse{
			Error:   "INVALID_DEPTH",
			Message: "Valid limits are: 5, 10, 20, 50, 100, 500, 1000",
		})
		return
	}

	// Get order book from trading service
	orderBook, err := h.tradingService.GetOrderBook(symbol, limit)
	if err != nil {
		h.logger.Error("Failed to get order book",
			zap.String("symbol", symbol),
			zap.Int("limit", limit),
			zap.Error(err),
		)

		// Check if it's a not found error
		if err.Error() == "trading pair not found" {
			c.JSON(http.StatusNotFound, apiutil.ErrorResponse{
				Error:   "SYMBOL_NOT_FOUND",
				Message: fmt.Sprintf("Symbol %s not found", symbol),
			})
			return
		}

		c.JSON(http.StatusInternalServerError, apiutil.ErrorResponse{
			Error:   "ORDERBOOK_ERROR",
			Message: "Failed to retrieve order book",
		})
		return
	}

	// Convert to response format
	response := &OrderBookResponse{
		Symbol:    symbol,
		Timestamp: time.Now().UnixMilli(),
		Bids:      orderBookLevelsToStrings(orderBook.Bids),
		Asks:      orderBookLevelsToStrings(orderBook.Asks),
		Depth:     limit,
	}

	// Set cache headers for high-frequency data
	c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")

	// Set response time header for monitoring
	c.Header("X-Response-Time", fmt.Sprintf("%dms", time.Since(startTime).Milliseconds()))

	h.logger.Info("Order book retrieved successfully",
		zap.String("symbol", symbol),
		zap.Int("limit", limit),
		zap.Duration("duration", time.Since(startTime)),
	)

	c.JSON(http.StatusOK, response)
}

// GetTicker godoc
// @Summary Get 24hr ticker price change statistics
// @Description Get 24hr ticker price change statistics for a symbol
// @Tags Market Data
// @Accept json
// @Produce json
// @Param symbol path string true "Trading pair symbol" example(BTCUSDT)
// @Success 200 {object} TickerResponse
// @Failure 400 {object} ErrorResponse "Bad Request"
// @Failure 404 {object} ErrorResponse "Symbol not found"
// @Failure 500 {object} ErrorResponse "Internal Server Error"
// @Router /api/v1/ticker/24hr/{symbol} [get]
func (h *MarketDataHandler) GetTicker(c *gin.Context) {
	startTime := time.Now()

	symbol := c.Param("symbol")
	if symbol == "" {
		c.JSON(http.StatusBadRequest, apiutil.ErrorResponse{
			Error:   "INVALID_SYMBOL",
			Message: "Symbol parameter is required",
		})
		return
	}

	// TODO: Implement ticker statistics from trading service
	// For now, return mock data structure
	response := &TickerResponse{
		Symbol:             symbol,
		Price:              "50000.00",
		PriceChange:        "1234.56",
		PriceChangePercent: "2.53",
		Volume:             "1234.567",
		QuoteVolume:        "61728350.00",
		High:               "51000.00",
		Low:                "49000.00",
		Open:               "48765.44",
		Close:              "50000.00",
		Timestamp:          time.Now().UnixMilli(),
		Count:              12345,
	}

	h.logger.Info("Ticker retrieved successfully",
		zap.String("symbol", symbol),
		zap.Duration("duration", time.Since(startTime)),
	)

	c.JSON(http.StatusOK, response)
}

// GetAllTickers godoc
// @Summary Get 24hr ticker price change statistics for all symbols
// @Description Get 24hr ticker price change statistics for all active trading pairs
// @Tags Market Data
// @Accept json
// @Produce json
// @Success 200 {array} TickerResponse
// @Failure 500 {object} ErrorResponse "Internal Server Error"
// @Router /api/v1/ticker/24hr [get]
func (h *MarketDataHandler) GetAllTickers(c *gin.Context) {
	startTime := time.Now()

	// Get all trading pairs
	pairs, err := h.tradingService.GetTradingPairs()
	if err != nil {
		h.logger.Error("Failed to get trading pairs", zap.Error(err))
		c.JSON(http.StatusInternalServerError, apiutil.ErrorResponse{
			Error:   "TRADING_PAIRS_ERROR",
			Message: "Failed to retrieve trading pairs",
		})
		return
	}

	// Build response for each pair
	var tickers []TickerResponse
	for _, pair := range pairs {
		// TODO: Get actual ticker data from trading service
		ticker := TickerResponse{
			Symbol:             pair.Symbol,
			Price:              "0.00",
			PriceChange:        "0.00",
			PriceChangePercent: "0.00",
			Volume:             "0.00",
			QuoteVolume:        "0.00",
			High:               "0.00",
			Low:                "0.00",
			Open:               "0.00",
			Close:              "0.00",
			Timestamp:          time.Now().UnixMilli(),
			Count:              0,
		}
		tickers = append(tickers, ticker)
	}

	h.logger.Info("All tickers retrieved successfully",
		zap.Int("count", len(tickers)),
		zap.Duration("duration", time.Since(startTime)),
	)

	c.JSON(http.StatusOK, tickers)
}

// GetTrades godoc
// @Summary Get recent trades
// @Description Get recent trades for a trading pair
// @Tags Market Data
// @Accept json
// @Produce json
// @Param symbol path string true "Trading pair symbol" example(BTCUSDT)
// @Param limit query int false "Number of trades to return (max 1000)" default(500)
// @Success 200 {array} TradeResponse
// @Failure 400 {object} ErrorResponse "Bad Request"
// @Failure 404 {object} ErrorResponse "Symbol not found"
// @Failure 500 {object} ErrorResponse "Internal Server Error"
// @Router /api/v1/trades/{symbol} [get]
func (h *MarketDataHandler) GetTrades(c *gin.Context) {
	startTime := time.Now()

	symbol := c.Param("symbol")
	if symbol == "" {
		c.JSON(http.StatusBadRequest, apiutil.ErrorResponse{
			Error:   "INVALID_SYMBOL",
			Message: "Symbol parameter is required",
		})
		return
	}

	limitStr := c.DefaultQuery("limit", "500")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 || limit > 1000 {
		c.JSON(http.StatusBadRequest, apiutil.ErrorResponse{
			Error:   "INVALID_LIMIT",
			Message: "Limit must be between 1 and 1000",
		})
		return
	}

	// TODO: Implement trade history from trading service
	// For now, return empty array
	var trades []TradeResponse

	h.logger.Info("Trades retrieved successfully",
		zap.String("symbol", symbol),
		zap.Int("limit", limit),
		zap.Int("count", len(trades)),
		zap.Duration("duration", time.Since(startTime)),
	)

	c.JSON(http.StatusOK, trades)
}

// GetKlines godoc
// @Summary Get kline/candlestick data
// @Description Get kline/candlestick data for a trading pair
// @Tags Market Data
// @Accept json
// @Produce json
// @Param symbol path string true "Trading pair symbol" example(BTCUSDT)
// @Param interval query string true "Kline interval (1m, 3m, 5m, 15m, 30m, 1h, 2h, 4h, 6h, 8h, 12h, 1d, 3d, 1w, 1M)" example(1h)
// @Param limit query int false "Number of klines to return (max 1000)" default(500)
// @Param startTime query int false "Start time in milliseconds"
// @Param endTime query int false "End time in milliseconds"
// @Success 200 {array} KlineResponse
// @Failure 400 {object} ErrorResponse "Bad Request"
// @Failure 404 {object} ErrorResponse "Symbol not found"
// @Failure 500 {object} ErrorResponse "Internal Server Error"
// @Router /api/v1/klines/{symbol} [get]
func (h *MarketDataHandler) GetKlines(c *gin.Context) {
	startTime := time.Now()

	symbol := c.Param("symbol")
	if symbol == "" {
		c.JSON(http.StatusBadRequest, apiutil.ErrorResponse{
			Error:   "INVALID_SYMBOL",
			Message: "Symbol parameter is required",
		})
		return
	}

	interval := c.Query("interval")
	if interval == "" {
		c.JSON(http.StatusBadRequest, apiutil.ErrorResponse{
			Error:   "INVALID_INTERVAL",
			Message: "Interval parameter is required",
		})
		return
	}

	// Validate interval
	validIntervals := []string{"1m", "3m", "5m", "15m", "30m", "1h", "2h", "4h", "6h", "8h", "12h", "1d", "3d", "1w", "1M"}
	isValidInterval := false
	for _, validInterval := range validIntervals {
		if interval == validInterval {
			isValidInterval = true
			break
		}
	}
	if !isValidInterval {
		c.JSON(http.StatusBadRequest, apiutil.ErrorResponse{
			Error:   "INVALID_INTERVAL",
			Message: "Valid intervals are: 1m, 3m, 5m, 15m, 30m, 1h, 2h, 4h, 6h, 8h, 12h, 1d, 3d, 1w, 1M",
		})
		return
	}

	limitStr := c.DefaultQuery("limit", "500")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 || limit > 1000 {
		c.JSON(http.StatusBadRequest, apiutil.ErrorResponse{
			Error:   "INVALID_LIMIT",
			Message: "Limit must be between 1 and 1000",
		})
		return
	}

	// TODO: Implement kline data from trading service
	// For now, return empty array
	var klines []KlineResponse

	h.logger.Info("Klines retrieved successfully",
		zap.String("symbol", symbol),
		zap.String("interval", interval),
		zap.Int("limit", limit),
		zap.Int("count", len(klines)),
		zap.Duration("duration", time.Since(startTime)),
	)

	c.JSON(http.StatusOK, klines)
}

// GetMarketStats godoc
// @Summary Get 24hr ticker statistics for a symbol
// @Description Get detailed 24hr ticker statistics for a trading pair
// @Tags Market Data
// @Accept json
// @Produce json
// @Param symbol path string true "Trading pair symbol" example(BTCUSDT)
// @Success 200 {object} MarketStatsResponse
// @Failure 400 {object} ErrorResponse "Bad Request"
// @Failure 404 {object} ErrorResponse "Symbol not found"
// @Failure 500 {object} ErrorResponse "Internal Server Error"
// @Router /api/v1/ticker/stats/{symbol} [get]
func (h *MarketDataHandler) GetMarketStats(c *gin.Context) {
	startTime := time.Now()

	symbol := c.Param("symbol")
	if symbol == "" {
		c.JSON(http.StatusBadRequest, apiutil.ErrorResponse{
			Error:   "INVALID_SYMBOL",
			Message: "Symbol parameter is required",
		})
		return
	}

	// TODO: Implement market statistics from trading service
	response := &MarketStatsResponse{
		Symbol:             symbol,
		PriceChange:        "0.00",
		PriceChangePercent: "0.00",
		WeightedAvgPrice:   "0.00",
		PrevClosePrice:     "0.00",
		LastPrice:          "0.00",
		LastQty:            "0.00",
		BidPrice:           "0.00",
		AskPrice:           "0.00",
		OpenPrice:          "0.00",
		HighPrice:          "0.00",
		LowPrice:           "0.00",
		Volume:             "0.00",
		QuoteVolume:        "0.00",
		OpenTime:           time.Now().AddDate(0, 0, -1).UnixMilli(),
		CloseTime:          time.Now().UnixMilli(),
		Count:              0,
	}

	h.logger.Info("Market stats retrieved successfully",
		zap.String("symbol", symbol),
		zap.Duration("duration", time.Since(startTime)),
	)

	c.JSON(http.StatusOK, response)
}

// GetExchangeInfo godoc
// @Summary Get exchange information
// @Description Get current exchange trading rules and symbol information
// @Tags Market Data
// @Accept json
// @Produce json
// @Success 200 {object} ExchangeInfoResponse
// @Failure 500 {object} ErrorResponse "Internal Server Error"
// @Router /api/v1/exchangeInfo [get]
func (h *MarketDataHandler) GetExchangeInfo(c *gin.Context) {
	startTime := time.Now()

	// Get all trading pairs
	pairs, err := h.tradingService.GetTradingPairs()
	if err != nil {
		h.logger.Error("Failed to get trading pairs", zap.Error(err))
		c.JSON(http.StatusInternalServerError, apiutil.ErrorResponse{
			Error:   "EXCHANGE_INFO_ERROR",
			Message: "Failed to retrieve exchange information",
		})
		return
	}

	// Build exchange info response
	symbols := make([]SymbolInfo, 0, len(pairs))
	for _, pair := range pairs {
		symbolInfo := SymbolInfo{
			Symbol:                 pair.Symbol,
			Status:                 pair.Status,
			OrderTypes:             []string{"MARKET", "LIMIT", "STOP_LOSS", "STOP_LOSS_LIMIT", "TAKE_PROFIT", "TAKE_PROFIT_LIMIT"},
			IcebergAllowed:         true,
			IsSpotTradingAllowed:   true,
			IsMarginTradingAllowed: false,
			Filters:                buildDefaultFilters(),
		}
		symbols = append(symbols, symbolInfo)
	}

	response := &ExchangeInfoResponse{
		Timezone:        "UTC",
		ServerTime:      time.Now().UnixMilli(),
		RateLimits:      buildDefaultRateLimits(),
		ExchangeFilters: []interface{}{},
		Symbols:         symbols,
	}

	h.logger.Info("Exchange info retrieved successfully",
		zap.Int("symbol_count", len(symbols)),
		zap.Duration("duration", time.Since(startTime)),
	)

	c.JSON(http.StatusOK, response)
}

// Helper functions for exchange info
type ExchangeInfoResponse struct {
	Timezone        string        `json:"timezone"`
	ServerTime      int64         `json:"server_time"`
	RateLimits      []RateLimit   `json:"rate_limits"`
	ExchangeFilters []interface{} `json:"exchange_filters"`
	Symbols         []SymbolInfo  `json:"symbols"`
}

type RateLimit struct {
	RateLimitType string `json:"rate_limit_type"`
	Interval      string `json:"interval"`
	IntervalNum   int    `json:"interval_num"`
	Limit         int    `json:"limit"`
}

type SymbolInfo struct {
	Symbol                 string   `json:"symbol"`
	Status                 string   `json:"status"`
	OrderTypes             []string `json:"order_types"`
	IcebergAllowed         bool     `json:"iceberg_allowed"`
	IsSpotTradingAllowed   bool     `json:"is_spot_trading_allowed"`
	IsMarginTradingAllowed bool     `json:"is_margin_trading_allowed"`
	Filters                []Filter `json:"filters"`
}

type Filter struct {
	FilterType   string `json:"filter_type"`
	MinPrice     string `json:"min_price,omitempty"`
	MaxPrice     string `json:"max_price,omitempty"`
	TickSize     string `json:"tick_size,omitempty"`
	MinQty       string `json:"min_qty,omitempty"`
	MaxQty       string `json:"max_qty,omitempty"`
	StepSize     string `json:"step_size,omitempty"`
	MinNotional  string `json:"min_notional,omitempty"`
	Limit        int    `json:"limit,omitempty"`
	MaxNumOrders int    `json:"max_num_orders,omitempty"`
}

func buildDefaultRateLimits() []RateLimit {
	return []RateLimit{
		{
			RateLimitType: "REQUEST_WEIGHT",
			Interval:      "MINUTE",
			IntervalNum:   1,
			Limit:         1200,
		},
		{
			RateLimitType: "ORDERS",
			Interval:      "SECOND",
			IntervalNum:   10,
			Limit:         50,
		},
		{
			RateLimitType: "ORDERS",
			Interval:      "DAY",
			IntervalNum:   1,
			Limit:         160000,
		},
	}
}

func buildDefaultFilters() []Filter {
	return []Filter{
		{
			FilterType: "PRICE_FILTER",
			MinPrice:   "0.00000100",
			MaxPrice:   "100000.00000000",
			TickSize:   "0.00000100",
		},
		{
			FilterType: "LOT_SIZE",
			MinQty:     "0.00100000",
			MaxQty:     "90000000.00000000",
			StepSize:   "0.00100000",
		},
		{
			FilterType:  "MIN_NOTIONAL",
			MinNotional: "10.00000000",
		},
		{
			FilterType:   "MAX_NUM_ORDERS",
			MaxNumOrders: 200,
		},
	}
}

// Helper to convert []OrderBookLevel to [][]string
func orderBookLevelsToStrings(levels []models.OrderBookLevel) [][]string {
	result := make([][]string, len(levels))
	for i, lvl := range levels {
		result[i] = []string{
			fmt.Sprintf("%.8f", lvl.Price),
			fmt.Sprintf("%.8f", lvl.Volume),
		}
	}
	return result
}
