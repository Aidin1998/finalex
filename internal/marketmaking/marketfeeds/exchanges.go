package marketfeeds

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// BinanceProvider implements ExchangeProvider for Binance
type BinanceProvider struct {
	name       string
	priority   int
	weight     float64
	baseURL    string
	httpClient *http.Client
	logger     *zap.Logger
	healthy    bool
	lastError  error
}

// NewBinanceProvider creates a new Binance provider
func NewBinanceProvider(logger *zap.Logger, config *ExchangeConfig) *BinanceProvider {
	baseURL := "https://api.binance.com"
	if config.TestMode {
		baseURL = "https://testnet.binance.vision"
	}

	return &BinanceProvider{
		name:     "binance",
		priority: config.Priority,
		weight:   config.Weight,
		baseURL:  baseURL,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
		logger:  logger,
		healthy: true,
	}
}

func (b *BinanceProvider) GetName() string {
	return b.name
}

func (b *BinanceProvider) GetPriority() int {
	return b.priority
}

func (b *BinanceProvider) IsHealthy() bool {
	return b.healthy
}

func (b *BinanceProvider) GetTicker(ctx context.Context, symbol string) (*TickerData, error) {
	url := fmt.Sprintf("%s/api/v3/ticker/24hr?symbol=%s", b.baseURL, strings.ToUpper(symbol))

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		b.healthy = false
		b.lastError = err
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := b.httpClient.Do(req)
	if err != nil {
		b.healthy = false
		b.lastError = err
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b.healthy = false
		b.lastError = fmt.Errorf("HTTP %d", resp.StatusCode)
		return nil, fmt.Errorf("HTTP error: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var binanceTicker struct {
		Symbol             string `json:"symbol"`
		PriceChange        string `json:"priceChange"`
		PriceChangePercent string `json:"priceChangePercent"`
		WeightedAvgPrice   string `json:"weightedAvgPrice"`
		PrevClosePrice     string `json:"prevClosePrice"`
		LastPrice          string `json:"lastPrice"`
		LastQty            string `json:"lastQty"`
		BidPrice           string `json:"bidPrice"`
		BidQty             string `json:"bidQty"`
		AskPrice           string `json:"askPrice"`
		AskQty             string `json:"askQty"`
		OpenPrice          string `json:"openPrice"`
		HighPrice          string `json:"highPrice"`
		LowPrice           string `json:"lowPrice"`
		Volume             string `json:"volume"`
		QuoteVolume        string `json:"quoteVolume"`
		OpenTime           int64  `json:"openTime"`
		CloseTime          int64  `json:"closeTime"`
		Count              int    `json:"count"`
	}

	if err := json.Unmarshal(body, &binanceTicker); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	b.healthy = true
	b.lastError = nil

	return &TickerData{
		Symbol:             binanceTicker.Symbol,
		Price:              parseDecimal(binanceTicker.LastPrice),
		PriceChange:        parseDecimal(binanceTicker.PriceChange),
		PriceChangePercent: parseDecimal(binanceTicker.PriceChangePercent),
		WeightedAvgPrice:   parseDecimal(binanceTicker.WeightedAvgPrice),
		PrevClosePrice:     parseDecimal(binanceTicker.PrevClosePrice),
		LastPrice:          parseDecimal(binanceTicker.LastPrice),
		LastQty:            parseDecimal(binanceTicker.LastQty),
		BidPrice:           parseDecimal(binanceTicker.BidPrice),
		BidQty:             parseDecimal(binanceTicker.BidQty),
		AskPrice:           parseDecimal(binanceTicker.AskPrice),
		AskQty:             parseDecimal(binanceTicker.AskQty),
		OpenPrice:          parseDecimal(binanceTicker.OpenPrice),
		HighPrice:          parseDecimal(binanceTicker.HighPrice),
		LowPrice:           parseDecimal(binanceTicker.LowPrice),
		Volume:             parseDecimal(binanceTicker.Volume),
		QuoteVolume:        parseDecimal(binanceTicker.QuoteVolume),
		OpenTime:           binanceTicker.OpenTime,
		CloseTime:          binanceTicker.CloseTime,
		Count:              binanceTicker.Count,
		Timestamp:          time.Now(),
		Source:             b.name,
	}, nil
}

func (b *BinanceProvider) GetOrderBook(ctx context.Context, symbol string, depth int) (*OrderBookData, error) {
	url := fmt.Sprintf("%s/api/v3/depth?symbol=%s&limit=%d", b.baseURL, strings.ToUpper(symbol), depth)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := b.httpClient.Do(req)
	if err != nil {
		b.healthy = false
		b.lastError = err
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b.healthy = false
		return nil, fmt.Errorf("HTTP error: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var binanceOrderBook struct {
		LastUpdateId int64      `json:"lastUpdateId"`
		Bids         [][]string `json:"bids"`
		Asks         [][]string `json:"asks"`
	}

	if err := json.Unmarshal(body, &binanceOrderBook); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	bids := make([]PriceLevel, len(binanceOrderBook.Bids))
	for i, bid := range binanceOrderBook.Bids {
		bids[i] = PriceLevel{
			Price:    parseDecimal(bid[0]),
			Quantity: parseDecimal(bid[1]),
		}
	}

	asks := make([]PriceLevel, len(binanceOrderBook.Asks))
	for i, ask := range binanceOrderBook.Asks {
		asks[i] = PriceLevel{
			Price:    parseDecimal(ask[0]),
			Quantity: parseDecimal(ask[1]),
		}
	}

	b.healthy = true

	return &OrderBookData{
		Symbol:       symbol,
		Bids:         bids,
		Asks:         asks,
		LastUpdateID: binanceOrderBook.LastUpdateId,
		Timestamp:    time.Now(),
		Source:       b.name,
	}, nil
}

func (b *BinanceProvider) GetKlines(ctx context.Context, symbol, interval string, limit int) ([]*KlineData, error) {
	url := fmt.Sprintf("%s/api/v3/klines?symbol=%s&interval=%s&limit=%d",
		b.baseURL, strings.ToUpper(symbol), interval, limit)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := b.httpClient.Do(req)
	if err != nil {
		b.healthy = false
		b.lastError = err
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b.healthy = false
		return nil, fmt.Errorf("HTTP error: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var binanceKlines [][]interface{}
	if err := json.Unmarshal(body, &binanceKlines); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	klines := make([]*KlineData, len(binanceKlines))
	for i, kline := range binanceKlines {
		if len(kline) < 12 {
			continue
		}

		klines[i] = &KlineData{
			Symbol:        symbol,
			Interval:      interval,
			OpenTime:      int64(kline[0].(float64)),
			CloseTime:     int64(kline[6].(float64)),
			Open:          parseDecimal(kline[1].(string)),
			High:          parseDecimal(kline[2].(string)),
			Low:           parseDecimal(kline[3].(string)),
			Close:         parseDecimal(kline[4].(string)),
			Volume:        parseDecimal(kline[5].(string)),
			QuoteVolume:   parseDecimal(kline[7].(string)),
			TradeCount:    int(kline[8].(float64)),
			TakerBuyBase:  parseDecimal(kline[9].(string)),
			TakerBuyQuote: parseDecimal(kline[10].(string)),
			IsClosed:      true,
			Timestamp:     time.Now(),
			Source:        b.name,
		}
	}

	b.healthy = true
	return klines, nil
}

func (b *BinanceProvider) GetTrades(ctx context.Context, symbol string, limit int) ([]*TradeData, error) {
	url := fmt.Sprintf("%s/api/v3/trades?symbol=%s&limit=%d", b.baseURL, strings.ToUpper(symbol), limit)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := b.httpClient.Do(req)
	if err != nil {
		b.healthy = false
		b.lastError = err
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b.healthy = false
		return nil, fmt.Errorf("HTTP error: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var binanceTrades []struct {
		ID           int64  `json:"id"`
		Price        string `json:"price"`
		Qty          string `json:"qty"`
		QuoteQty     string `json:"quoteQty"`
		Time         int64  `json:"time"`
		IsBuyerMaker bool   `json:"isBuyerMaker"`
	}

	if err := json.Unmarshal(body, &binanceTrades); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	trades := make([]*TradeData, len(binanceTrades))
	for i, trade := range binanceTrades {
		trades[i] = &TradeData{
			ID:           strconv.FormatInt(trade.ID, 10),
			Symbol:       symbol,
			Price:        parseDecimal(trade.Price),
			Quantity:     parseDecimal(trade.Qty),
			QuoteQty:     parseDecimal(trade.QuoteQty),
			Time:         trade.Time,
			IsBuyerMaker: trade.IsBuyerMaker,
			Timestamp:    time.Now(),
			Source:       b.name,
		}
	}

	b.healthy = true
	return trades, nil
}

func (b *BinanceProvider) SubscribeToUpdates(ctx context.Context, symbols []string, callback func(*MarketUpdate)) error {
	// For now, implement polling-based updates
	// In production, this would use WebSocket connections
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			for _, symbol := range symbols {
				if ticker, err := b.GetTicker(ctx, symbol); err == nil {
					callback(&MarketUpdate{
						Type:      "ticker",
						Symbol:    symbol,
						Data:      ticker,
						Timestamp: time.Now(),
						Source:    b.name,
					})
				}
			}
		}
	}
}

// CoinbaseProvider implements ExchangeProvider for Coinbase Pro
type CoinbaseProvider struct {
	name       string
	priority   int
	weight     float64
	baseURL    string
	httpClient *http.Client
	logger     *zap.Logger
	healthy    bool
	lastError  error
}

func NewCoinbaseProvider(logger *zap.Logger, config *ExchangeConfig) *CoinbaseProvider {
	baseURL := "https://api.exchange.coinbase.com"
	if config.TestMode {
		baseURL = "https://api-public.sandbox.exchange.coinbase.com"
	}

	return &CoinbaseProvider{
		name:     "coinbase",
		priority: config.Priority,
		weight:   config.Weight,
		baseURL:  baseURL,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
		logger:  logger,
		healthy: true,
	}
}

func (c *CoinbaseProvider) GetName() string {
	return c.name
}

func (c *CoinbaseProvider) GetPriority() int {
	return c.priority
}

func (c *CoinbaseProvider) IsHealthy() bool {
	return c.healthy
}

func (c *CoinbaseProvider) GetTicker(ctx context.Context, symbol string) (*TickerData, error) {
	// Convert symbol format (BTCUSDT -> BTC-USDT)
	coinbaseSymbol := strings.ToUpper(symbol)
	if len(coinbaseSymbol) == 6 {
		coinbaseSymbol = coinbaseSymbol[:3] + "-" + coinbaseSymbol[3:]
	}

	url := fmt.Sprintf("%s/products/%s/ticker", c.baseURL, coinbaseSymbol)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.healthy = false
		c.lastError = err
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.healthy = false
		return nil, fmt.Errorf("HTTP error: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var coinbaseTicker struct {
		TradeID int64  `json:"trade_id"`
		Price   string `json:"price"`
		Size    string `json:"size"`
		Bid     string `json:"bid"`
		Ask     string `json:"ask"`
		Volume  string `json:"volume"`
		Time    string `json:"time"`
	}

	if err := json.Unmarshal(body, &coinbaseTicker); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	c.healthy = true
	c.lastError = nil

	return &TickerData{
		Symbol:    symbol,
		Price:     parseDecimal(coinbaseTicker.Price),
		LastPrice: parseDecimal(coinbaseTicker.Price),
		LastQty:   parseDecimal(coinbaseTicker.Size),
		BidPrice:  parseDecimal(coinbaseTicker.Bid),
		AskPrice:  parseDecimal(coinbaseTicker.Ask),
		Volume:    parseDecimal(coinbaseTicker.Volume),
		Timestamp: time.Now(),
		Source:    c.name,
	}, nil
}

func (c *CoinbaseProvider) GetOrderBook(ctx context.Context, symbol string, depth int) (*OrderBookData, error) {
	coinbaseSymbol := strings.ToUpper(symbol)
	if len(coinbaseSymbol) == 6 {
		coinbaseSymbol = coinbaseSymbol[:3] + "-" + coinbaseSymbol[3:]
	}

	level := 2
	if depth <= 50 {
		level = 2
	} else {
		level = 3
	}

	url := fmt.Sprintf("%s/products/%s/book?level=%d", c.baseURL, coinbaseSymbol, level)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.healthy = false
		c.lastError = err
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.healthy = false
		return nil, fmt.Errorf("HTTP error: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var coinbaseOrderBook struct {
		Sequence int64      `json:"sequence"`
		Bids     [][]string `json:"bids"`
		Asks     [][]string `json:"asks"`
	}

	if err := json.Unmarshal(body, &coinbaseOrderBook); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	maxLevels := depth
	if len(coinbaseOrderBook.Bids) < maxLevels {
		maxLevels = len(coinbaseOrderBook.Bids)
	}

	bids := make([]PriceLevel, maxLevels)
	for i := 0; i < maxLevels; i++ {
		bids[i] = PriceLevel{
			Price:    parseDecimal(coinbaseOrderBook.Bids[i][0]),
			Quantity: parseDecimal(coinbaseOrderBook.Bids[i][1]),
		}
	}

	maxLevels = depth
	if len(coinbaseOrderBook.Asks) < maxLevels {
		maxLevels = len(coinbaseOrderBook.Asks)
	}

	asks := make([]PriceLevel, maxLevels)
	for i := 0; i < maxLevels; i++ {
		asks[i] = PriceLevel{
			Price:    parseDecimal(coinbaseOrderBook.Asks[i][0]),
			Quantity: parseDecimal(coinbaseOrderBook.Asks[i][1]),
		}
	}

	c.healthy = true

	return &OrderBookData{
		Symbol:       symbol,
		Bids:         bids,
		Asks:         asks,
		LastUpdateID: coinbaseOrderBook.Sequence,
		Timestamp:    time.Now(),
		Source:       c.name,
	}, nil
}

func (c *CoinbaseProvider) GetKlines(ctx context.Context, symbol, interval string, limit int) ([]*KlineData, error) {
	// Coinbase uses different interval format
	granularity := convertIntervalToCoinbase(interval)

	coinbaseSymbol := strings.ToUpper(symbol)
	if len(coinbaseSymbol) == 6 {
		coinbaseSymbol = coinbaseSymbol[:3] + "-" + coinbaseSymbol[3:]
	}

	end := time.Now()
	start := end.Add(-time.Duration(limit) * time.Duration(granularity) * time.Second)

	url := fmt.Sprintf("%s/products/%s/candles?start=%s&end=%s&granularity=%d",
		c.baseURL, coinbaseSymbol, start.UTC().Format(time.RFC3339), end.UTC().Format(time.RFC3339), granularity)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.healthy = false
		c.lastError = err
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.healthy = false
		return nil, fmt.Errorf("HTTP error: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var coinbaseKlines [][]float64
	if err := json.Unmarshal(body, &coinbaseKlines); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	klines := make([]*KlineData, len(coinbaseKlines))
	for i, kline := range coinbaseKlines {
		if len(kline) < 6 {
			continue
		}

		klines[i] = &KlineData{
			Symbol:    symbol,
			Interval:  interval,
			OpenTime:  int64(kline[0]) * 1000, // Convert to milliseconds
			CloseTime: int64(kline[0])*1000 + int64(granularity)*1000,
			Open:      decimal.NewFromFloat(kline[3]),
			High:      decimal.NewFromFloat(kline[2]),
			Low:       decimal.NewFromFloat(kline[1]),
			Close:     decimal.NewFromFloat(kline[4]),
			Volume:    decimal.NewFromFloat(kline[5]),
			IsClosed:  true,
			Timestamp: time.Now(),
			Source:    c.name,
		}
	}

	c.healthy = true
	return klines, nil
}

func (c *CoinbaseProvider) GetTrades(ctx context.Context, symbol string, limit int) ([]*TradeData, error) {
	// Implement trade history fetching for Coinbase
	return nil, fmt.Errorf("trades not implemented for Coinbase provider")
}

func (c *CoinbaseProvider) SubscribeToUpdates(ctx context.Context, symbols []string, callback func(*MarketUpdate)) error {
	// Implement WebSocket subscription for Coinbase
	return fmt.Errorf("real-time updates not implemented for Coinbase provider")
}

// Helper functions
func parseDecimal(s string) decimal.Decimal {
	if s == "" {
		return decimal.Zero
	}
	d, err := decimal.NewFromString(s)
	if err != nil {
		return decimal.Zero
	}
	return d
}

func convertIntervalToCoinbase(interval string) int {
	switch interval {
	case "1m":
		return 60
	case "5m":
		return 300
	case "15m":
		return 900
	case "1h":
		return 3600
	case "6h":
		return 21600
	case "1d":
		return 86400
	default:
		return 300 // Default to 5 minutes
	}
}

// KrakenProvider implements ExchangeProvider for Kraken
type KrakenProvider struct {
	name       string
	priority   int
	weight     float64
	baseURL    string
	httpClient *http.Client
	logger     *zap.Logger
	healthy    bool
	lastError  error
}

// NewKrakenProvider creates a new Kraken provider
func NewKrakenProvider(logger *zap.Logger, config *ExchangeConfig) *KrakenProvider {
	return &KrakenProvider{
		name:     "kraken",
		priority: config.Priority,
		weight:   config.Weight,
		baseURL:  "https://api.kraken.com",
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
		logger:  logger,
		healthy: true,
	}
}

func (k *KrakenProvider) GetName() string {
	return k.name
}

func (k *KrakenProvider) GetPriority() int {
	return k.priority
}

func (k *KrakenProvider) IsHealthy() bool {
	return k.healthy
}

func (k *KrakenProvider) GetTicker(ctx context.Context, symbol string) (*TickerData, error) {
	// Convert symbol format (e.g., BTCUSDT -> XXBTZUSD)
	krakenSymbol := convertToKrakenSymbol(symbol)
	url := fmt.Sprintf("%s/0/public/Ticker?pair=%s", k.baseURL, krakenSymbol)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		k.healthy = false
		k.lastError = err
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := k.httpClient.Do(req)
	if err != nil {
		k.healthy = false
		k.lastError = err
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		k.healthy = false
		k.lastError = err
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var krakenResp struct {
		Error  []string                       `json:"error"`
		Result map[string]map[string][]string `json:"result"`
	}

	if err := json.Unmarshal(body, &krakenResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if len(krakenResp.Error) > 0 {
		k.healthy = false
		k.lastError = fmt.Errorf("kraken API error: %v", krakenResp.Error)
		return nil, k.lastError
	}

	// Get the first (and should be only) result
	var tickerData map[string][]string
	for _, data := range krakenResp.Result {
		tickerData = data
		break
	}

	if tickerData == nil {
		return nil, fmt.Errorf("no ticker data found for symbol %s", symbol)
	}

	k.healthy = true
	k.lastError = nil
	return &TickerData{
		Symbol:             symbol,
		Price:              parseDecimal(tickerData["c"][0]), // Last trade closed
		PriceChange:        decimal.Zero,                     // Calculate from open/close
		PriceChangePercent: decimal.Zero,
		BidPrice:           parseDecimal(tickerData["b"][0]), // Best bid
		AskPrice:           parseDecimal(tickerData["a"][0]), // Best ask
		OpenPrice:          parseDecimal(tickerData["o"][0]), // Today's opening price
		HighPrice:          parseDecimal(tickerData["h"][1]), // High today
		LowPrice:           parseDecimal(tickerData["l"][1]), // Low today
		Volume:             parseDecimal(tickerData["v"][1]), // Volume today
		Timestamp:          time.Now(),
		Source:             k.name,
	}, nil
}

func (k *KrakenProvider) GetOrderBook(ctx context.Context, symbol string, depth int) (*OrderBookData, error) {
	krakenSymbol := convertToKrakenSymbol(symbol)
	url := fmt.Sprintf("%s/0/public/Depth?pair=%s&count=%d", k.baseURL, krakenSymbol, depth)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := k.httpClient.Do(req)
	if err != nil {
		k.healthy = false
		k.lastError = err
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var krakenResp struct {
		Error  []string `json:"error"`
		Result map[string]struct {
			Bids [][]string `json:"bids"`
			Asks [][]string `json:"asks"`
		} `json:"result"`
	}

	if err := json.Unmarshal(body, &krakenResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if len(krakenResp.Error) > 0 {
		return nil, fmt.Errorf("kraken API error: %v", krakenResp.Error)
	}

	var orderBookData struct {
		Bids [][]string `json:"bids"`
		Asks [][]string `json:"asks"`
	}

	for _, data := range krakenResp.Result {
		orderBookData = data
		break
	}

	// Convert to our format
	bids := make([]PriceLevel, len(orderBookData.Bids))
	for i, bid := range orderBookData.Bids {
		bids[i] = PriceLevel{
			Price:    parseDecimal(bid[0]),
			Quantity: parseDecimal(bid[1]),
		}
	}

	asks := make([]PriceLevel, len(orderBookData.Asks))
	for i, ask := range orderBookData.Asks {
		asks[i] = PriceLevel{
			Price:    parseDecimal(ask[0]),
			Quantity: parseDecimal(ask[1]),
		}
	}

	return &OrderBookData{
		Symbol:    symbol,
		Bids:      bids,
		Asks:      asks,
		Timestamp: time.Now(),
		Source:    k.name,
	}, nil
}

func (k *KrakenProvider) GetKlines(ctx context.Context, symbol, interval string, limit int) ([]*KlineData, error) {
	krakenSymbol := convertToKrakenSymbol(symbol)
	krakenInterval := convertToKrakenInterval(interval)
	url := fmt.Sprintf("%s/0/public/OHLC?pair=%s&interval=%s", k.baseURL, krakenSymbol, krakenInterval)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := k.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var krakenResp struct {
		Error  []string              `json:"error"`
		Result map[string][][]string `json:"result"`
	}

	if err := json.Unmarshal(body, &krakenResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if len(krakenResp.Error) > 0 {
		return nil, fmt.Errorf("kraken API error: %v", krakenResp.Error)
	}

	var ohlcData [][]string
	for key, data := range krakenResp.Result {
		if key != "last" { // Skip "last" entry which contains timestamp
			ohlcData = data
			break
		}
	}

	klines := make([]*KlineData, 0, len(ohlcData))
	for _, ohlc := range ohlcData {
		if len(ohlc) < 8 {
			continue
		}

		timestamp, _ := strconv.ParseInt(ohlc[0], 10, 64)
		klines = append(klines, &KlineData{
			Symbol:     symbol,
			Interval:   interval,
			OpenTime:   timestamp * 1000, // Convert to milliseconds
			CloseTime:  timestamp*1000 + getIntervalMs(interval),
			Open:       parseDecimal(ohlc[1]),
			High:       parseDecimal(ohlc[2]),
			Low:        parseDecimal(ohlc[3]),
			Close:      parseDecimal(ohlc[4]),
			Volume:     parseDecimal(ohlc[6]),
			TradeCount: 0, // Not provided by Kraken
			Timestamp:  time.Now(),
			Source:     k.name,
		})
	}

	return klines, nil
}

func (k *KrakenProvider) GetTrades(ctx context.Context, symbol string, limit int) ([]*TradeData, error) {
	krakenSymbol := convertToKrakenSymbol(symbol)
	url := fmt.Sprintf("%s/0/public/Trades?pair=%s", k.baseURL, krakenSymbol)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := k.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var krakenResp struct {
		Error  []string            `json:"error"`
		Result map[string][]string `json:"result"`
	}

	if err := json.Unmarshal(body, &krakenResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if len(krakenResp.Error) > 0 {
		return nil, fmt.Errorf("kraken API error: %v", krakenResp.Error)
	}

	// Kraken trades implementation would go here
	// For now, return empty slice
	return []*TradeData{}, nil
}

func (k *KrakenProvider) SubscribeToUpdates(ctx context.Context, symbols []string, callback func(*MarketUpdate)) error {
	// WebSocket implementation for Kraken would go here
	return fmt.Errorf("WebSocket subscriptions not implemented yet for Kraken")
}

// OKXProvider implements ExchangeProvider for OKX
type OKXProvider struct {
	name       string
	priority   int
	weight     float64
	baseURL    string
	httpClient *http.Client
	logger     *zap.Logger
	healthy    bool
	lastError  error
}

// NewOKXProvider creates a new OKX provider
func NewOKXProvider(logger *zap.Logger, config *ExchangeConfig) *OKXProvider {
	return &OKXProvider{
		name:     "okx",
		priority: config.Priority,
		weight:   config.Weight,
		baseURL:  "https://www.okx.com",
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
		logger:  logger,
		healthy: true,
	}
}

func (o *OKXProvider) GetName() string {
	return o.name
}

func (o *OKXProvider) GetPriority() int {
	return o.priority
}

func (o *OKXProvider) IsHealthy() bool {
	return o.healthy
}

func (o *OKXProvider) GetTicker(ctx context.Context, symbol string) (*TickerData, error) {
	// Convert symbol format (e.g., BTCUSDT -> BTC-USDT)
	okxSymbol := convertToOKXSymbol(symbol)
	url := fmt.Sprintf("%s/api/v5/market/ticker?instId=%s", o.baseURL, okxSymbol)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		o.healthy = false
		o.lastError = err
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := o.httpClient.Do(req)
	if err != nil {
		o.healthy = false
		o.lastError = err
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		o.healthy = false
		o.lastError = err
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var okxResp struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
		Data []struct {
			InstType  string `json:"instType"`
			InstId    string `json:"instId"`
			Last      string `json:"last"`
			LastSz    string `json:"lastSz"`
			AskPx     string `json:"askPx"`
			AskSz     string `json:"askSz"`
			BidPx     string `json:"bidPx"`
			BidSz     string `json:"bidSz"`
			Open24h   string `json:"open24h"`
			High24h   string `json:"high24h"`
			Low24h    string `json:"low24h"`
			Vol24h    string `json:"vol24h"`
			VolCcy24h string `json:"volCcy24h"`
			Ts        string `json:"ts"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &okxResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if okxResp.Code != "0" {
		o.healthy = false
		o.lastError = fmt.Errorf("OKX API error: %s", okxResp.Msg)
		return nil, o.lastError
	}

	if len(okxResp.Data) == 0 {
		return nil, fmt.Errorf("no ticker data found for symbol %s", symbol)
	}

	ticker := okxResp.Data[0]
	o.healthy = true
	o.lastError = nil

	lastPrice := parseDecimal(ticker.Last)
	openPrice := parseDecimal(ticker.Open24h)
	priceChange := lastPrice.Sub(openPrice)
	priceChangePercent := decimal.Zero
	if !openPrice.IsZero() {
		priceChangePercent = priceChange.Div(openPrice).Mul(decimal.NewFromInt(100))
	}

	return &TickerData{
		Symbol:             symbol,
		Price:              lastPrice,
		PriceChange:        priceChange,
		PriceChangePercent: priceChangePercent,
		LastPrice:          lastPrice,
		LastQty:            parseDecimal(ticker.LastSz),
		BidPrice:           parseDecimal(ticker.BidPx),
		BidQty:             parseDecimal(ticker.BidSz),
		AskPrice:           parseDecimal(ticker.AskPx),
		AskQty:             parseDecimal(ticker.AskSz),
		OpenPrice:          openPrice,
		HighPrice:          parseDecimal(ticker.High24h),
		LowPrice:           parseDecimal(ticker.Low24h),
		Volume:             parseDecimal(ticker.Vol24h),
		QuoteVolume:        parseDecimal(ticker.VolCcy24h),
		Timestamp:          time.Now(),
		Source:             o.name,
	}, nil
}

func (o *OKXProvider) GetOrderBook(ctx context.Context, symbol string, depth int) (*OrderBookData, error) {
	okxSymbol := convertToOKXSymbol(symbol)
	url := fmt.Sprintf("%s/api/v5/market/books?instId=%s&sz=%d", o.baseURL, okxSymbol, depth)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := o.httpClient.Do(req)
	if err != nil {
		o.healthy = false
		o.lastError = err
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var okxResp struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
		Data []struct {
			Asks [][]string `json:"asks"`
			Bids [][]string `json:"bids"`
			Ts   string     `json:"ts"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &okxResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if okxResp.Code != "0" {
		return nil, fmt.Errorf("OKX API error: %s", okxResp.Msg)
	}

	if len(okxResp.Data) == 0 {
		return nil, fmt.Errorf("no order book data found for symbol %s", symbol)
	}

	orderBook := okxResp.Data[0]

	// Convert to our format
	bids := make([]PriceLevel, len(orderBook.Bids))
	for i, bid := range orderBook.Bids {
		bids[i] = PriceLevel{
			Price:    parseDecimal(bid[0]),
			Quantity: parseDecimal(bid[1]),
		}
	}

	asks := make([]PriceLevel, len(orderBook.Asks))
	for i, ask := range orderBook.Asks {
		asks[i] = PriceLevel{
			Price:    parseDecimal(ask[0]),
			Quantity: parseDecimal(ask[1]),
		}
	}

	return &OrderBookData{
		Symbol:    symbol,
		Bids:      bids,
		Asks:      asks,
		Timestamp: time.Now(),
		Source:    o.name,
	}, nil
}

func (o *OKXProvider) GetKlines(ctx context.Context, symbol, interval string, limit int) ([]*KlineData, error) {
	okxSymbol := convertToOKXSymbol(symbol)
	okxInterval := convertToOKXInterval(interval)
	url := fmt.Sprintf("%s/api/v5/market/candles?instId=%s&bar=%s&limit=%d", o.baseURL, okxSymbol, okxInterval, limit)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := o.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var okxResp struct {
		Code string     `json:"code"`
		Msg  string     `json:"msg"`
		Data [][]string `json:"data"`
	}

	if err := json.Unmarshal(body, &okxResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if okxResp.Code != "0" {
		return nil, fmt.Errorf("OKX API error: %s", okxResp.Msg)
	}

	klines := make([]*KlineData, 0, len(okxResp.Data))
	for _, candle := range okxResp.Data {
		if len(candle) < 6 {
			continue
		}

		openTime, _ := strconv.ParseInt(candle[0], 10, 64)
		klines = append(klines, &KlineData{
			Symbol:      symbol,
			Interval:    interval,
			OpenTime:    openTime,
			CloseTime:   openTime + getIntervalMs(interval),
			Open:        parseDecimal(candle[1]),
			High:        parseDecimal(candle[2]),
			Low:         parseDecimal(candle[3]),
			Close:       parseDecimal(candle[4]),
			Volume:      parseDecimal(candle[5]),
			QuoteVolume: parseDecimal(candle[6]),
			Timestamp:   time.Now(),
			Source:      o.name,
		})
	}

	return klines, nil
}

func (o *OKXProvider) GetTrades(ctx context.Context, symbol string, limit int) ([]*TradeData, error) {
	// OKX trades implementation would go here
	return []*TradeData{}, nil
}

func (o *OKXProvider) SubscribeToUpdates(ctx context.Context, symbols []string, callback func(*MarketUpdate)) error {
	// WebSocket implementation for OKX would go here
	return fmt.Errorf("WebSocket subscriptions not implemented yet for OKX")
}

// BitstampProvider implements ExchangeProvider for Bitstamp
type BitstampProvider struct {
	name       string
	priority   int
	weight     float64
	baseURL    string
	httpClient *http.Client
	logger     *zap.Logger
	healthy    bool
	lastError  error
}

// NewBitstampProvider creates a new Bitstamp provider
func NewBitstampProvider(logger *zap.Logger, config *ExchangeConfig) *BitstampProvider {
	return &BitstampProvider{
		name:     "bitstamp",
		priority: config.Priority,
		weight:   config.Weight,
		baseURL:  "https://www.bitstamp.net",
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
		logger:  logger,
		healthy: true,
	}
}

func (b *BitstampProvider) GetName() string {
	return b.name
}

func (b *BitstampProvider) GetPriority() int {
	return b.priority
}

func (b *BitstampProvider) IsHealthy() bool {
	return b.healthy
}

func (b *BitstampProvider) GetTicker(ctx context.Context, symbol string) (*TickerData, error) {
	// Convert symbol format (e.g., BTCUSDT -> btcusd)
	bitstampSymbol := convertToBitstampSymbol(symbol)
	url := fmt.Sprintf("%s/api/v2/ticker/%s", b.baseURL, bitstampSymbol)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		b.healthy = false
		b.lastError = err
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := b.httpClient.Do(req)
	if err != nil {
		b.healthy = false
		b.lastError = err
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		b.healthy = false
		b.lastError = err
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var bitstampTicker struct {
		Last      string `json:"last"`
		High      string `json:"high"`
		Low       string `json:"low"`
		Volume    string `json:"volume"`
		Bid       string `json:"bid"`
		Ask       string `json:"ask"`
		Open      string `json:"open"`
		Timestamp string `json:"timestamp"`
	}

	if err := json.Unmarshal(body, &bitstampTicker); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	b.healthy = true
	b.lastError = nil

	lastPrice := parseDecimal(bitstampTicker.Last)
	openPrice := parseDecimal(bitstampTicker.Open)
	priceChange := lastPrice.Sub(openPrice)
	priceChangePercent := decimal.Zero
	if !openPrice.IsZero() {
		priceChangePercent = priceChange.Div(openPrice).Mul(decimal.NewFromInt(100))
	}

	return &TickerData{
		Symbol:             symbol,
		Price:              lastPrice,
		PriceChange:        priceChange,
		PriceChangePercent: priceChangePercent,
		LastPrice:          lastPrice,
		BidPrice:           parseDecimal(bitstampTicker.Bid),
		AskPrice:           parseDecimal(bitstampTicker.Ask),
		OpenPrice:          openPrice,
		HighPrice:          parseDecimal(bitstampTicker.High),
		LowPrice:           parseDecimal(bitstampTicker.Low),
		Volume:             parseDecimal(bitstampTicker.Volume),
		Timestamp:          time.Now(),
		Source:             b.name,
	}, nil
}

func (b *BitstampProvider) GetOrderBook(ctx context.Context, symbol string, depth int) (*OrderBookData, error) {
	bitstampSymbol := convertToBitstampSymbol(symbol)
	url := fmt.Sprintf("%s/api/v2/order_book/%s", b.baseURL, bitstampSymbol)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := b.httpClient.Do(req)
	if err != nil {
		b.healthy = false
		b.lastError = err
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var bitstampOrderBook struct {
		Timestamp string     `json:"timestamp"`
		Bids      [][]string `json:"bids"`
		Asks      [][]string `json:"asks"`
	}

	if err := json.Unmarshal(body, &bitstampOrderBook); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Convert to our format
	bids := make([]PriceLevel, len(bitstampOrderBook.Bids))
	for i, bid := range bitstampOrderBook.Bids {
		bids[i] = PriceLevel{
			Price:    parseDecimal(bid[0]),
			Quantity: parseDecimal(bid[1]),
		}
	}

	asks := make([]PriceLevel, len(bitstampOrderBook.Asks))
	for i, ask := range bitstampOrderBook.Asks {
		asks[i] = PriceLevel{
			Price:    parseDecimal(ask[0]),
			Quantity: parseDecimal(ask[1]),
		}
	}

	return &OrderBookData{
		Symbol:    symbol,
		Bids:      bids,
		Asks:      asks,
		Timestamp: time.Now(),
		Source:    b.name,
	}, nil
}

func (b *BitstampProvider) GetKlines(ctx context.Context, symbol, interval string, limit int) ([]*KlineData, error) {
	// Bitstamp doesn't have a standard OHLC endpoint, would need to implement with transactions or use a different approach
	return []*KlineData{}, fmt.Errorf("klines not available for Bitstamp")
}

func (b *BitstampProvider) GetTrades(ctx context.Context, symbol string, limit int) ([]*TradeData, error) {
	bitstampSymbol := convertToBitstampSymbol(symbol)
	url := fmt.Sprintf("%s/api/v2/transactions/%s", b.baseURL, bitstampSymbol)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var bitstampTrades []struct {
		Date   string `json:"date"`
		TID    string `json:"tid"`
		Price  string `json:"price"`
		Amount string `json:"amount"`
		Type   string `json:"type"`
	}

	if err := json.Unmarshal(body, &bitstampTrades); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	trades := make([]*TradeData, 0, len(bitstampTrades))
	for _, trade := range bitstampTrades {
		timestamp, _ := strconv.ParseInt(trade.Date, 10, 64)
		price := parseDecimal(trade.Price)
		quantity := parseDecimal(trade.Amount)

		trades = append(trades, &TradeData{
			ID:           trade.TID,
			Symbol:       symbol,
			Price:        price,
			Quantity:     quantity,
			QuoteQty:     price.Mul(quantity),
			Time:         timestamp * 1000,  // Convert to milliseconds
			IsBuyerMaker: trade.Type == "1", // 0 = buy, 1 = sell
			Timestamp:    time.Now(),
			Source:       b.name,
		})
	}

	return trades, nil
}

func (b *BitstampProvider) SubscribeToUpdates(ctx context.Context, symbols []string, callback func(*MarketUpdate)) error {
	// WebSocket implementation for Bitstamp would go here
	return fmt.Errorf("WebSocket subscriptions not implemented yet for Bitstamp")
}

// Helper functions for symbol conversion
func convertToKrakenSymbol(symbol string) string {
	// Convert BTCUSDT -> XXBTZUSD, etc.
	symbol = strings.ToUpper(symbol)
	if strings.HasSuffix(symbol, "USDT") {
		base := symbol[:len(symbol)-4]
		if base == "BTC" {
			return "XXBTZUSD"
		}
		if base == "ETH" {
			return "XETHZUSD"
		}
		return base + "USD"
	}
	return symbol
}

func convertToKrakenInterval(interval string) string {
	switch interval {
	case "1m":
		return "1"
	case "5m":
		return "5"
	case "15m":
		return "15"
	case "30m":
		return "30"
	case "1h":
		return "60"
	case "4h":
		return "240"
	case "1d":
		return "1440"
	case "1w":
		return "10080"
	default:
		return "60"
	}
}

func convertToOKXSymbol(symbol string) string {
	// Convert BTCUSDT -> BTC-USDT
	symbol = strings.ToUpper(symbol)
	if len(symbol) >= 6 {
		if strings.HasSuffix(symbol, "USDT") {
			base := symbol[:len(symbol)-4]
			return base + "-USDT"
		}
		if strings.HasSuffix(symbol, "BTC") {
			base := symbol[:len(symbol)-3]
			return base + "-BTC"
		}
		if strings.HasSuffix(symbol, "ETH") {
			base := symbol[:len(symbol)-3]
			return base + "-ETH"
		}
	}
	return symbol
}

func convertToOKXInterval(interval string) string {
	switch interval {
	case "1m":
		return "1m"
	case "5m":
		return "5m"
	case "15m":
		return "15m"
	case "30m":
		return "30m"
	case "1h":
		return "1H"
	case "4h":
		return "4H"
	case "1d":
		return "1D"
	case "1w":
		return "1W"
	default:
		return "1H"
	}
}

func convertToBitstampSymbol(symbol string) string {
	// Convert BTCUSDT -> btcusd
	symbol = strings.ToLower(symbol)
	if strings.HasSuffix(symbol, "usdt") {
		base := symbol[:len(symbol)-4]
		return base + "usd"
	}
	return symbol
}

func getIntervalMs(interval string) int64 {
	switch interval {
	case "1m":
		return 60 * 1000
	case "5m":
		return 5 * 60 * 1000
	case "15m":
		return 15 * 60 * 1000
	case "30m":
		return 30 * 60 * 1000
	case "1h":
		return 60 * 60 * 1000
	case "4h":
		return 4 * 60 * 60 * 1000
	case "1d":
		return 24 * 60 * 60 * 1000
	case "1w":
		return 7 * 24 * 60 * 60 * 1000
	default:
		return 60 * 60 * 1000
	}
}
