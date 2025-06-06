// websocket_providers.go: WebSocket implementations for real-time market data
package marketfeeds

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// WebSocketProvider interface for real-time data streaming
type WebSocketProvider interface {
	ExchangeProvider // Embed base provider interface

	// WebSocket specific methods
	Connect(ctx context.Context) error
	Disconnect() error
	IsConnected() bool

	// Subscription management
	Subscribe(symbols []string) error
	Unsubscribe(symbols []string) error
	GetSubscriptions() []string

	// Real-time data streams
	GetTickerStream() <-chan *TickerData
	GetOrderBookStream() <-chan *OrderBookData
	GetTradeStream() <-chan *TradeData

	// Connection health
	GetConnectionStatus() *ConnectionStatus
	SetReconnectCallback(callback func())
}

// ConnectionStatus represents WebSocket connection health
type ConnectionStatus struct {
	Connected      bool          `json:"connected"`
	LastPing       time.Time     `json:"last_ping"`
	LastMessage    time.Time     `json:"last_message"`
	Latency        time.Duration `json:"latency"`
	ReconnectCount int           `json:"reconnect_count"`
	ErrorCount     int           `json:"error_count"`
	LastError      string        `json:"last_error,omitempty"`
}

// BaseWebSocketProvider provides common WebSocket functionality
type BaseWebSocketProvider struct {
	name   string
	wsURL  string
	conn   *websocket.Conn
	connMu sync.RWMutex
	logger *zap.Logger

	// Channels for real-time data
	tickerChan    chan *TickerData
	orderBookChan chan *OrderBookData
	tradeChan     chan *TradeData

	// Connection management
	connected     bool
	subscriptions map[string]bool
	subsMu        sync.RWMutex
	status        *ConnectionStatus
	statusMu      sync.RWMutex

	// Lifecycle management
	ctx         context.Context
	cancel      context.CancelFunc
	reconnectCb func()
	stopChan    chan struct{}
	wg          sync.WaitGroup
}

// NewBaseWebSocketProvider creates a base WebSocket provider
func NewBaseWebSocketProvider(name, wsURL string, logger *zap.Logger) *BaseWebSocketProvider {
	ctx, cancel := context.WithCancel(context.Background())

	return &BaseWebSocketProvider{
		name:          name,
		wsURL:         wsURL,
		logger:        logger.Named(name),
		tickerChan:    make(chan *TickerData, 1000),
		orderBookChan: make(chan *OrderBookData, 1000),
		tradeChan:     make(chan *TradeData, 1000),
		subscriptions: make(map[string]bool),
		status: &ConnectionStatus{
			Connected: false,
		},
		ctx:      ctx,
		cancel:   cancel,
		stopChan: make(chan struct{}),
	}
}

// Connect establishes WebSocket connection
func (b *BaseWebSocketProvider) Connect(ctx context.Context) error {
	b.connMu.Lock()
	defer b.connMu.Unlock()

	if b.connected {
		return nil
	}

	b.logger.Info("Connecting to WebSocket", zap.String("url", b.wsURL))

	// Parse WebSocket URL
	u, err := url.Parse(b.wsURL)
	if err != nil {
		return fmt.Errorf("invalid WebSocket URL: %w", err)
	}

	// Establish connection with timeout
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.DialContext(ctx, u.String(), nil)
	if err != nil {
		b.updateError(fmt.Sprintf("connection failed: %v", err))
		return fmt.Errorf("WebSocket connection failed: %w", err)
	}

	b.conn = conn
	b.connected = true

	// Update status
	b.statusMu.Lock()
	b.status.Connected = true
	b.status.LastMessage = time.Now()
	b.statusMu.Unlock()

	// Start message handling goroutines
	b.wg.Add(2)
	go b.readPump()
	go b.pingPump()

	b.logger.Info("WebSocket connected successfully")
	return nil
}

// Disconnect closes WebSocket connection
func (b *BaseWebSocketProvider) Disconnect() error {
	b.connMu.Lock()
	defer b.connMu.Unlock()

	if !b.connected {
		return nil
	}

	b.logger.Info("Disconnecting WebSocket")

	// Signal shutdown
	close(b.stopChan)

	// Close connection
	if b.conn != nil {
		b.conn.Close()
	}

	// Wait for goroutines to finish
	b.wg.Wait()

	b.connected = false
	b.statusMu.Lock()
	b.status.Connected = false
	b.statusMu.Unlock()

	b.logger.Info("WebSocket disconnected")
	return nil
}

// IsConnected returns connection status
func (b *BaseWebSocketProvider) IsConnected() bool {
	b.connMu.RLock()
	defer b.connMu.RUnlock()
	return b.connected
}

// Subscribe adds symbols to subscription list
func (b *BaseWebSocketProvider) Subscribe(symbols []string) error {
	b.subsMu.Lock()
	defer b.subsMu.Unlock()

	for _, symbol := range symbols {
		b.subscriptions[symbol] = true
	}

	// Derived classes should override this to send actual subscription messages
	return nil
}

// Unsubscribe removes symbols from subscription list
func (b *BaseWebSocketProvider) Unsubscribe(symbols []string) error {
	b.subsMu.Lock()
	defer b.subsMu.Unlock()

	for _, symbol := range symbols {
		delete(b.subscriptions, symbol)
	}

	return nil
}

// GetSubscriptions returns current subscriptions
func (b *BaseWebSocketProvider) GetSubscriptions() []string {
	b.subsMu.RLock()
	defer b.subsMu.RUnlock()

	symbols := make([]string, 0, len(b.subscriptions))
	for symbol := range b.subscriptions {
		symbols = append(symbols, symbol)
	}
	return symbols
}

// Stream getters
func (b *BaseWebSocketProvider) GetTickerStream() <-chan *TickerData {
	return b.tickerChan
}

func (b *BaseWebSocketProvider) GetOrderBookStream() <-chan *OrderBookData {
	return b.orderBookChan
}

func (b *BaseWebSocketProvider) GetTradeStream() <-chan *TradeData {
	return b.tradeChan
}

// GetConnectionStatus returns current connection status
func (b *BaseWebSocketProvider) GetConnectionStatus() *ConnectionStatus {
	b.statusMu.RLock()
	defer b.statusMu.RUnlock()

	// Return a copy
	status := *b.status
	return &status
}

// SetReconnectCallback sets callback for reconnection events
func (b *BaseWebSocketProvider) SetReconnectCallback(callback func()) {
	b.reconnectCb = callback
}

// readPump handles incoming WebSocket messages
func (b *BaseWebSocketProvider) readPump() {
	defer b.wg.Done()

	b.conn.SetReadLimit(512 * 1024) // 512KB max message size
	b.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	b.conn.SetPongHandler(func(string) error {
		b.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		b.updatePing()
		return nil
	})

	for {
		select {
		case <-b.stopChan:
			return
		default:
			_, message, err := b.conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					b.logger.Error("WebSocket read error", zap.Error(err))
					b.updateError(fmt.Sprintf("read error: %v", err))
				}
				b.handleDisconnection()
				return
			}

			b.updateLastMessage()
			b.handleMessage(message)
		}
	}
}

// pingPump sends periodic ping messages
func (b *BaseWebSocketProvider) pingPump() {
	defer b.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-b.stopChan:
			return
		case <-ticker.C:
			b.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := b.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				b.logger.Error("WebSocket ping error", zap.Error(err))
				b.updateError(fmt.Sprintf("ping error: %v", err))
				b.handleDisconnection()
				return
			}
		}
	}
}

// handleMessage processes incoming WebSocket messages (to be overridden)
func (b *BaseWebSocketProvider) handleMessage(message []byte) {
	// Base implementation - derived classes should override
	b.logger.Debug("Received WebSocket message", zap.Int("size", len(message)))
}

// handleDisconnection handles connection loss and triggers reconnection
func (b *BaseWebSocketProvider) handleDisconnection() {
	b.connMu.Lock()
	b.connected = false
	b.statusMu.Lock()
	b.status.Connected = false
	b.status.ReconnectCount++
	b.statusMu.Unlock()
	b.connMu.Unlock()

	b.logger.Warn("WebSocket disconnected, triggering reconnection")

	if b.reconnectCb != nil {
		b.reconnectCb()
	}
}

// Status update helpers
func (b *BaseWebSocketProvider) updatePing() {
	b.statusMu.Lock()
	defer b.statusMu.Unlock()

	now := time.Now()
	if !b.status.LastPing.IsZero() {
		b.status.Latency = now.Sub(b.status.LastPing)
	}
	b.status.LastPing = now
}

func (b *BaseWebSocketProvider) updateLastMessage() {
	b.statusMu.Lock()
	defer b.statusMu.Unlock()
	b.status.LastMessage = time.Now()
}

func (b *BaseWebSocketProvider) updateError(errorMsg string) {
	b.statusMu.Lock()
	defer b.statusMu.Unlock()
	b.status.ErrorCount++
	b.status.LastError = errorMsg
}

// Send message helper
func (b *BaseWebSocketProvider) sendMessage(message interface{}) error {
	if !b.IsConnected() {
		return fmt.Errorf("WebSocket not connected")
	}

	b.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return b.conn.WriteJSON(message)
}

// BinanceWebSocketProvider implements WebSocket for Binance
type BinanceWebSocketProvider struct {
	*BaseWebSocketProvider
	*BinanceProvider
}

// NewBinanceWebSocketProvider creates a new Binance WebSocket provider
func NewBinanceWebSocketProvider(logger *zap.Logger, config *ExchangeConfig) *BinanceWebSocketProvider {
	base := NewBaseWebSocketProvider("binance-ws", "wss://stream.binance.com:9443/ws", logger)
	binance := NewBinanceProvider(logger, config)

	provider := &BinanceWebSocketProvider{
		BaseWebSocketProvider: base,
		BinanceProvider:       binance,
	}

	return provider
}

// Subscribe implements symbol subscription for Binance
func (b *BinanceWebSocketProvider) Subscribe(symbols []string) error {
	if err := b.BaseWebSocketProvider.Subscribe(symbols); err != nil {
		return err
	}

	if !b.IsConnected() {
		return fmt.Errorf("WebSocket not connected")
	}

	// Binance uses stream names in format: symbol@ticker, symbol@depth
	var streams []string
	for _, symbol := range symbols {
		symbolLower := strings.ToLower(symbol)
		streams = append(streams,
			symbolLower+"@ticker",
			symbolLower+"@depth20@100ms",
			symbolLower+"@trade",
		)
	}

	// Subscribe to streams
	subscribeMsg := map[string]interface{}{
		"method": "SUBSCRIBE",
		"params": streams,
		"id":     time.Now().Unix(),
	}

	return b.sendMessage(subscribeMsg)
}

// handleMessage processes Binance WebSocket messages
func (b *BinanceWebSocketProvider) handleMessage(message []byte) {
	var response map[string]interface{}
	if err := json.Unmarshal(message, &response); err != nil {
		b.BaseWebSocketProvider.logger.Error("Failed to parse Binance message", zap.Error(err))
		return
	}

	// Handle different message types
	if stream, ok := response["stream"].(string); ok {
		data := response["data"].(map[string]interface{})

		if strings.Contains(stream, "@ticker") {
			b.handleTickerMessage(data)
		} else if strings.Contains(stream, "@depth") {
			b.handleOrderBookMessage(data)
		} else if strings.Contains(stream, "@trade") {
			b.handleTradeMessage(data)
		}
	}
}

// handleTickerMessage processes ticker data
func (b *BinanceWebSocketProvider) handleTickerMessage(data map[string]interface{}) {
	symbol := data["s"].(string)

	price, _ := decimal.NewFromString(data["c"].(string))
	bidPrice, _ := decimal.NewFromString(data["b"].(string))
	askPrice, _ := decimal.NewFromString(data["a"].(string))
	volume, _ := decimal.NewFromString(data["v"].(string))
	quoteVolume, _ := decimal.NewFromString(data["q"].(string))
	priceChange, _ := decimal.NewFromString(data["p"].(string))
	priceChangePercent, _ := decimal.NewFromString(data["P"].(string))
	highPrice, _ := decimal.NewFromString(data["h"].(string))
	lowPrice, _ := decimal.NewFromString(data["l"].(string))

	ticker := &TickerData{
		Symbol:             symbol,
		Price:              price,
		BidPrice:           bidPrice,
		AskPrice:           askPrice,
		Volume:             volume,
		QuoteVolume:        quoteVolume,
		PriceChange:        priceChange,
		PriceChangePercent: priceChangePercent,
		HighPrice:          highPrice,
		LowPrice:           lowPrice,
		Timestamp:          time.Now(),
	}
	select {
	case b.tickerChan <- ticker:
	default:
		// Channel full, drop message
		b.BaseWebSocketProvider.logger.Warn("Ticker channel full, dropping message", zap.String("symbol", symbol))
	}
}

// handleOrderBookMessage processes order book data
func (b *BinanceWebSocketProvider) handleOrderBookMessage(data map[string]interface{}) {
	symbol := data["s"].(string)

	bidsInterface := data["bids"].([]interface{})
	asksInterface := data["asks"].([]interface{})

	var bids, asks []PriceLevel

	for _, bidInterface := range bidsInterface {
		bid := bidInterface.([]interface{})
		price, _ := decimal.NewFromString(bid[0].(string))
		quantity, _ := decimal.NewFromString(bid[1].(string))
		bids = append(bids, PriceLevel{Price: price, Quantity: quantity})
	}

	for _, askInterface := range asksInterface {
		ask := askInterface.([]interface{})
		price, _ := decimal.NewFromString(ask[0].(string))
		quantity, _ := decimal.NewFromString(ask[1].(string))
		asks = append(asks, PriceLevel{Price: price, Quantity: quantity})
	}

	orderBook := &OrderBookData{
		Symbol:    symbol,
		Bids:      bids,
		Asks:      asks,
		Timestamp: time.Now(),
	}
	select {
	case b.orderBookChan <- orderBook:
	default:
		b.BaseWebSocketProvider.logger.Warn("OrderBook channel full, dropping message", zap.String("symbol", symbol))
	}
}

// handleTradeMessage processes trade data
func (b *BinanceWebSocketProvider) handleTradeMessage(data map[string]interface{}) {
	symbol := data["s"].(string)
	price, _ := decimal.NewFromString(data["p"].(string))
	quantity, _ := decimal.NewFromString(data["q"].(string))
	timestamp := time.Unix(int64(data["T"].(float64))/1000, 0)

	isBuyerMaker := data["m"].(bool) // m = true means market maker, false means market taker

	trade := &TradeData{
		Symbol:       symbol,
		Price:        price,
		Quantity:     quantity,
		IsBuyerMaker: isBuyerMaker,
		Timestamp:    timestamp,
		Source:       "binance",
	}

	select {
	case b.tradeChan <- trade:
	default:
		b.BaseWebSocketProvider.logger.Warn("Trade channel full, dropping message", zap.String("symbol", symbol))
	}
}

// CoinbaseWebSocketProvider implements WebSocket for Coinbase Pro
type CoinbaseWebSocketProvider struct {
	*BaseWebSocketProvider
	*CoinbaseProvider
}

// NewCoinbaseWebSocketProvider creates a new Coinbase WebSocket provider
func NewCoinbaseWebSocketProvider(logger *zap.Logger, config *ExchangeConfig) *CoinbaseWebSocketProvider {
	base := NewBaseWebSocketProvider("coinbase-ws", "wss://ws-feed.pro.coinbase.com", logger)
	coinbase := NewCoinbaseProvider(logger, config)

	provider := &CoinbaseWebSocketProvider{
		BaseWebSocketProvider: base,
		CoinbaseProvider:      coinbase,
	}

	return provider
}

// Subscribe implements symbol subscription for Coinbase
func (c *CoinbaseWebSocketProvider) Subscribe(symbols []string) error {
	if err := c.BaseWebSocketProvider.Subscribe(symbols); err != nil {
		return err
	}

	if !c.IsConnected() {
		return fmt.Errorf("WebSocket not connected")
	}

	// Convert symbols to Coinbase format
	productIds := make([]string, len(symbols))
	for i, symbol := range symbols {
		productIds[i] = c.convertSymbol(symbol)
	}

	// Subscribe to channels
	subscribeMsg := map[string]interface{}{
		"type":        "subscribe",
		"product_ids": productIds,
		"channels":    []string{"ticker", "level2", "matches"},
	}

	return c.sendMessage(subscribeMsg)
}

// handleMessage processes Coinbase WebSocket messages
func (c *CoinbaseWebSocketProvider) handleMessage(message []byte) {
	var response map[string]interface{}
	if err := json.Unmarshal(message, &response); err != nil {
		c.BaseWebSocketProvider.logger.Error("Failed to parse Coinbase message", zap.Error(err))
		return
	}

	msgType, ok := response["type"].(string)
	if !ok {
		return
	}

	switch msgType {
	case "ticker":
		c.handleCoinbaseTickerMessage(response)
	case "l2update":
		c.handleCoinbaseOrderBookMessage(response)
	case "last_match":
		c.handleCoinbaseTradeMessage(response)
	}
}

func (c *CoinbaseWebSocketProvider) handleCoinbaseTickerMessage(data map[string]interface{}) {
	productId := data["product_id"].(string)
	symbol := c.convertSymbolBack(productId)

	price, _ := decimal.NewFromString(data["price"].(string))
	bestBid, _ := decimal.NewFromString(data["best_bid"].(string))
	bestAsk, _ := decimal.NewFromString(data["best_ask"].(string))
	volume24h, _ := decimal.NewFromString(data["volume_24h"].(string))

	ticker := &TickerData{
		Symbol:      symbol,
		Price:       price,
		BidPrice:    bestBid,
		AskPrice:    bestAsk,
		Volume:      volume24h,
		QuoteVolume: decimal.Zero,
		Timestamp:   time.Now(),
	}
	select {
	case c.tickerChan <- ticker:
	default:
		c.BaseWebSocketProvider.logger.Warn("Ticker channel full, dropping message", zap.String("symbol", symbol))
	}
}

func (c *CoinbaseWebSocketProvider) handleCoinbaseOrderBookMessage(data map[string]interface{}) {
	productId := data["product_id"].(string)
	symbol := c.convertSymbolBack(productId)

	changes := data["changes"].([]interface{})

	var bids, asks []PriceLevel

	for _, changeInterface := range changes {
		change := changeInterface.([]interface{})
		side := change[0].(string)
		price, _ := decimal.NewFromString(change[1].(string))
		size, _ := decimal.NewFromString(change[2].(string))

		level := PriceLevel{Price: price, Quantity: size}

		if side == "buy" {
			bids = append(bids, level)
		} else {
			asks = append(asks, level)
		}
	}

	orderBook := &OrderBookData{
		Symbol:    symbol,
		Bids:      bids,
		Asks:      asks,
		Timestamp: time.Now(),
	}
	select {
	case c.orderBookChan <- orderBook:
	default:
		c.BaseWebSocketProvider.logger.Warn("OrderBook channel full, dropping message", zap.String("symbol", symbol))
	}
}

func (c *CoinbaseWebSocketProvider) handleCoinbaseTradeMessage(data map[string]interface{}) {
	productId := data["product_id"].(string)
	symbol := c.convertSymbolBack(productId)

	price, _ := decimal.NewFromString(data["price"].(string))
	size, _ := decimal.NewFromString(data["size"].(string))
	side := data["side"].(string)

	isBuyerMaker := side == "sell" // In Coinbase, seller is the maker

	trade := &TradeData{
		Symbol:       symbol,
		Price:        price,
		Quantity:     size,
		IsBuyerMaker: isBuyerMaker,
		Timestamp:    time.Now(),
		Source:       "coinbase",
	}

	select {
	case c.tradeChan <- trade:
	default:
		c.BaseWebSocketProvider.logger.Warn("Trade channel full, dropping message", zap.String("symbol", symbol))
	}
}

// Helper methods for symbol conversion
func (c *CoinbaseWebSocketProvider) convertSymbol(symbol string) string {
	// Convert BTCUSDT -> BTC-USDT format
	if len(symbol) >= 6 {
		base := symbol[:len(symbol)-4]
		quote := symbol[len(symbol)-4:]
		return base + "-" + quote
	}
	return symbol
}

func (c *CoinbaseWebSocketProvider) convertSymbolBack(productId string) string {
	// Convert BTC-USDT -> BTCUSDT format
	return strings.ReplaceAll(productId, "-", "")
}
