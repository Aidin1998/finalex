// websocket_providers_extended.go: Additional WebSocket implementations
package marketfeeds

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// KrakenWebSocketProvider implements WebSocket for Kraken
type KrakenWebSocketProvider struct {
	*BaseWebSocketProvider
	*KrakenProvider
}

// NewKrakenWebSocketProvider creates a new Kraken WebSocket provider
func NewKrakenWebSocketProvider(logger *zap.Logger, config *ExchangeConfig) *KrakenWebSocketProvider {
	base := NewBaseWebSocketProvider("kraken-ws", "wss://ws.kraken.com", logger)
	kraken := NewKrakenProvider(logger, config)

	provider := &KrakenWebSocketProvider{
		BaseWebSocketProvider: base,
		KrakenProvider:        kraken,
	}

	return provider
}

// Subscribe implements symbol subscription for Kraken
func (k *KrakenWebSocketProvider) Subscribe(symbols []string) error {
	if err := k.BaseWebSocketProvider.Subscribe(symbols); err != nil {
		return err
	}

	if !k.IsConnected() {
		return fmt.Errorf("WebSocket not connected")
	}

	// Convert symbols to Kraken format
	pairs := make([]string, len(symbols))
	for i, symbol := range symbols {
		pairs[i] = k.convertSymbolToKraken(symbol)
	}

	// Subscribe to ticker channel
	tickerMsg := map[string]interface{}{
		"event": "subscribe",
		"pair":  pairs,
		"subscription": map[string]interface{}{
			"name": "ticker",
		},
	}

	if err := k.sendMessage(tickerMsg); err != nil {
		return err
	}

	// Subscribe to book channel
	bookMsg := map[string]interface{}{
		"event": "subscribe",
		"pair":  pairs,
		"subscription": map[string]interface{}{
			"name":  "book",
			"depth": 25,
		},
	}

	if err := k.sendMessage(bookMsg); err != nil {
		return err
	}

	// Subscribe to trade channel
	tradeMsg := map[string]interface{}{
		"event": "subscribe",
		"pair":  pairs,
		"subscription": map[string]interface{}{
			"name": "trade",
		},
	}

	return k.sendMessage(tradeMsg)
}

// handleMessage processes Kraken WebSocket messages
func (k *KrakenWebSocketProvider) handleMessage(message []byte) {
	// Kraken sends both array and object messages
	var arrayMessage []interface{}
	var objectMessage map[string]interface{}

	// Try to parse as array first (data messages)
	if err := json.Unmarshal(message, &arrayMessage); err == nil && len(arrayMessage) >= 2 {
		k.handleKrakenArrayMessage(arrayMessage)
		return
	}

	// Try to parse as object (status messages)
	if err := json.Unmarshal(message, &objectMessage); err == nil {
		k.handleKrakenObjectMessage(objectMessage)
	}
}

func (k *KrakenWebSocketProvider) handleKrakenArrayMessage(data []interface{}) {
	if len(data) < 3 {
		return
	}

	channelName := data[len(data)-2].(string)
	pair := data[len(data)-1].(string)
	symbol := k.convertSymbolFromKraken(pair)

	switch {
	case strings.Contains(channelName, "ticker"):
		k.handleKrakenTickerMessage(data[1].(map[string]interface{}), symbol)
	case strings.Contains(channelName, "book"):
		k.handleKrakenOrderBookMessage(data[1].(map[string]interface{}), symbol)
	case strings.Contains(channelName, "trade"):
		k.handleKrakenTradeMessage(data[1].([]interface{}), symbol)
	}
}

func (k *KrakenWebSocketProvider) handleKrakenObjectMessage(data map[string]interface{}) {
	if event, ok := data["event"].(string); ok {
		k.BaseWebSocketProvider.logger.Debug("Kraken WebSocket event", zap.String("event", event))
	}
}

func (k *KrakenWebSocketProvider) handleKrakenTickerMessage(data map[string]interface{}, symbol string) {
	// Kraken ticker format: {"a":["price","wholeLotVolume","lotVolume"],"b":["price","wholeLotVolume","lotVolume"],"c":["price","lotVolume"],"v":["today","last24h"],"p":["today","last24h"],"t":[today,last24h],"l":["today","last24h"],"h":["today","last24h"],"o":["today","last24h"]}

	ask := data["a"].([]interface{})
	bid := data["b"].([]interface{})
	close := data["c"].([]interface{})
	volume := data["v"].([]interface{})
	high := data["h"].([]interface{})
	low := data["l"].([]interface{})

	price, _ := decimal.NewFromString(close[0].(string))
	askPrice, _ := decimal.NewFromString(ask[0].(string))
	bidPrice, _ := decimal.NewFromString(bid[0].(string))
	volume24h, _ := decimal.NewFromString(volume[1].(string))
	high24h, _ := decimal.NewFromString(high[1].(string))
	low24h, _ := decimal.NewFromString(low[1].(string))

	ticker := &TickerData{
		Symbol:      symbol,
		Price:       price,
		BidPrice:    bidPrice,
		AskPrice:    askPrice,
		Volume:      volume24h,
		QuoteVolume: decimal.Zero,
		HighPrice:   high24h,
		LowPrice:    low24h,
		Timestamp:   time.Now(),
	}
	select {
	case k.tickerChan <- ticker:
	default:
		k.BaseWebSocketProvider.logger.Warn("Ticker channel full, dropping message", zap.String("symbol", symbol))
	}
}

func (k *KrakenWebSocketProvider) handleKrakenOrderBookMessage(data map[string]interface{}, symbol string) {
	var bids, asks []PriceLevel

	if bidData, ok := data["b"].([]interface{}); ok {
		for _, bidInterface := range bidData {
			bid := bidInterface.([]interface{})
			price, _ := decimal.NewFromString(bid[0].(string))
			quantity, _ := decimal.NewFromString(bid[1].(string))
			bids = append(bids, PriceLevel{Price: price, Quantity: quantity})
		}
	}

	if askData, ok := data["a"].([]interface{}); ok {
		for _, askInterface := range askData {
			ask := askInterface.([]interface{})
			price, _ := decimal.NewFromString(ask[0].(string))
			quantity, _ := decimal.NewFromString(ask[1].(string))
			asks = append(asks, PriceLevel{Price: price, Quantity: quantity})
		}
	}

	orderBook := &OrderBookData{
		Symbol:    symbol,
		Bids:      bids,
		Asks:      asks,
		Timestamp: time.Now(),
	}
	select {
	case k.orderBookChan <- orderBook:
	default:
		k.BaseWebSocketProvider.logger.Warn("OrderBook channel full, dropping message", zap.String("symbol", symbol))
	}
}

func (k *KrakenWebSocketProvider) handleKrakenTradeMessage(data []interface{}, symbol string) {
	for _, tradeInterface := range data {
		trade := tradeInterface.([]interface{})
		if len(trade) < 5 {
			continue
		}
		price, _ := decimal.NewFromString(trade[0].(string))
		quantity, _ := decimal.NewFromString(trade[1].(string))
		timestamp := time.Unix(int64(trade[2].(float64)), 0)
		isBuyerMaker := trade[3].(string) == "s" // "s" means sell/market taker

		tradeData := &TradeData{
			Symbol:       symbol,
			Price:        price,
			Quantity:     quantity,
			IsBuyerMaker: isBuyerMaker,
			Timestamp:    timestamp,
			Source:       "kraken",
		}

		select {
		case k.tradeChan <- tradeData:
		default:
			k.BaseWebSocketProvider.logger.Warn("Trade channel full, dropping message", zap.String("symbol", symbol))
		}
	}
}

// OKXWebSocketProvider implements WebSocket for OKX
type OKXWebSocketProvider struct {
	*BaseWebSocketProvider
	*OKXProvider
}

// NewOKXWebSocketProvider creates a new OKX WebSocket provider
func NewOKXWebSocketProvider(logger *zap.Logger, config *ExchangeConfig) *OKXWebSocketProvider {
	base := NewBaseWebSocketProvider("okx-ws", "wss://ws.okx.com:8443/ws/v5/public", logger)
	okx := NewOKXProvider(logger, config)

	provider := &OKXWebSocketProvider{
		BaseWebSocketProvider: base,
		OKXProvider:           okx,
	}

	return provider
}

// Subscribe implements symbol subscription for OKX
func (o *OKXWebSocketProvider) Subscribe(symbols []string) error {
	if err := o.BaseWebSocketProvider.Subscribe(symbols); err != nil {
		return err
	}

	if !o.IsConnected() {
		return fmt.Errorf("WebSocket not connected")
	}

	// Convert symbols to OKX format and create subscription args
	var tickerArgs, booksArgs, tradesArgs []map[string]string

	for _, symbol := range symbols {
		instId := o.convertSymbolToOKX(symbol)

		tickerArgs = append(tickerArgs, map[string]string{
			"channel": "tickers",
			"instId":  instId,
		})

		booksArgs = append(booksArgs, map[string]string{
			"channel": "books",
			"instId":  instId,
		})

		tradesArgs = append(tradesArgs, map[string]string{
			"channel": "trades",
			"instId":  instId,
		})
	}

	// Subscribe to tickers
	tickerMsg := map[string]interface{}{
		"op":   "subscribe",
		"args": tickerArgs,
	}

	if err := o.sendMessage(tickerMsg); err != nil {
		return err
	}

	// Subscribe to order books
	bookMsg := map[string]interface{}{
		"op":   "subscribe",
		"args": booksArgs,
	}

	if err := o.sendMessage(bookMsg); err != nil {
		return err
	}

	// Subscribe to trades
	tradeMsg := map[string]interface{}{
		"op":   "subscribe",
		"args": tradesArgs,
	}

	return o.sendMessage(tradeMsg)
}

// handleMessage processes OKX WebSocket messages
func (o *OKXWebSocketProvider) handleMessage(message []byte) {
	var response map[string]interface{}
	if err := json.Unmarshal(message, &response); err != nil {
		o.BaseWebSocketProvider.logger.Error("Failed to parse OKX message", zap.Error(err))
		return
	}

	// Handle data messages
	if data, ok := response["data"].([]interface{}); ok {
		arg := response["arg"].(map[string]interface{})
		channel := arg["channel"].(string)

		switch channel {
		case "tickers":
			o.handleOKXTickerMessage(data)
		case "books":
			o.handleOKXOrderBookMessage(data)
		case "trades":
			o.handleOKXTradeMessage(data)
		}
	}
}

func (o *OKXWebSocketProvider) handleOKXTickerMessage(data []interface{}) {
	for _, tickerInterface := range data {
		ticker := tickerInterface.(map[string]interface{})

		instId := ticker["instId"].(string)
		symbol := o.convertSymbolFromOKX(instId)

		last, _ := decimal.NewFromString(ticker["last"].(string))
		bidPx, _ := decimal.NewFromString(ticker["bidPx"].(string))
		askPx, _ := decimal.NewFromString(ticker["askPx"].(string))
		vol24h, _ := decimal.NewFromString(ticker["vol24h"].(string))
		volCcy24h, _ := decimal.NewFromString(ticker["volCcy24h"].(string))
		high24h, _ := decimal.NewFromString(ticker["high24h"].(string))
		low24h, _ := decimal.NewFromString(ticker["low24h"].(string))

		tickerData := &TickerData{
			Symbol:      symbol,
			Price:       last,
			BidPrice:    bidPx,
			AskPrice:    askPx,
			Volume:      vol24h,
			QuoteVolume: volCcy24h,
			HighPrice:   high24h,
			LowPrice:    low24h,
			Timestamp:   time.Now(),
		}
		select {
		case o.tickerChan <- tickerData:
		default:
			o.BaseWebSocketProvider.logger.Warn("Ticker channel full, dropping message", zap.String("symbol", symbol))
		}
	}
}

func (o *OKXWebSocketProvider) handleOKXOrderBookMessage(data []interface{}) {
	for _, bookInterface := range data {
		book := bookInterface.(map[string]interface{})

		instId := book["instId"].(string)
		symbol := o.convertSymbolFromOKX(instId)

		var bids, asks []PriceLevel

		if bidData, ok := book["bids"].([]interface{}); ok {
			for _, bidInterface := range bidData {
				bid := bidInterface.([]interface{})
				price, _ := decimal.NewFromString(bid[0].(string))
				quantity, _ := decimal.NewFromString(bid[1].(string))
				bids = append(bids, PriceLevel{Price: price, Quantity: quantity})
			}
		}

		if askData, ok := book["asks"].([]interface{}); ok {
			for _, askInterface := range askData {
				ask := askInterface.([]interface{})
				price, _ := decimal.NewFromString(ask[0].(string))
				quantity, _ := decimal.NewFromString(ask[1].(string))
				asks = append(asks, PriceLevel{Price: price, Quantity: quantity})
			}
		}

		orderBook := &OrderBookData{
			Symbol:    symbol,
			Bids:      bids,
			Asks:      asks,
			Timestamp: time.Now(),
		}
		select {
		case o.orderBookChan <- orderBook:
		default:
			o.BaseWebSocketProvider.logger.Warn("OrderBook channel full, dropping message", zap.String("symbol", symbol))
		}
	}
}

func (o *OKXWebSocketProvider) handleOKXTradeMessage(data []interface{}) {
	for _, tradeInterface := range data {
		trade := tradeInterface.(map[string]interface{})

		instId := trade["instId"].(string)
		symbol := o.convertSymbolFromOKX(instId)
		px, _ := decimal.NewFromString(trade["px"].(string))
		sz, _ := decimal.NewFromString(trade["sz"].(string))
		side := trade["side"].(string)
		timestamp, _ := strconv.ParseInt(trade["ts"].(string), 10, 64)

		isBuyerMaker := side == "sell" // In OKX, "sell" means seller is maker

		tradeData := &TradeData{
			Symbol:       symbol,
			Price:        px,
			Quantity:     sz,
			IsBuyerMaker: isBuyerMaker,
			Timestamp:    time.Unix(timestamp/1000, 0),
			Source:       "okx",
		}

		select {
		case o.tradeChan <- tradeData:
		default:
			o.BaseWebSocketProvider.logger.Warn("Trade channel full, dropping message", zap.String("symbol", symbol))
		}
	}
}

// BitstampWebSocketProvider implements WebSocket for Bitstamp
type BitstampWebSocketProvider struct {
	*BaseWebSocketProvider
	*BitstampProvider
}

// NewBitstampWebSocketProvider creates a new Bitstamp WebSocket provider
func NewBitstampWebSocketProvider(logger *zap.Logger, config *ExchangeConfig) *BitstampWebSocketProvider {
	base := NewBaseWebSocketProvider("bitstamp-ws", "wss://ws.bitstamp.net", logger)
	bitstamp := NewBitstampProvider(logger, config)

	provider := &BitstampWebSocketProvider{
		BaseWebSocketProvider: base,
		BitstampProvider:      bitstamp,
	}

	return provider
}

// Subscribe implements symbol subscription for Bitstamp
func (b *BitstampWebSocketProvider) Subscribe(symbols []string) error {
	if err := b.BaseWebSocketProvider.Subscribe(symbols); err != nil {
		return err
	}

	if !b.IsConnected() {
		return fmt.Errorf("WebSocket not connected")
	}

	// Subscribe to each symbol separately
	for _, symbol := range symbols {
		pair := b.convertSymbolToBitstamp(symbol)

		// Subscribe to ticker
		tickerMsg := map[string]interface{}{
			"event": "bts:subscribe",
			"data": map[string]string{
				"channel": "live_trades_" + pair,
			},
		}

		if err := b.sendMessage(tickerMsg); err != nil {
			return err
		}

		// Subscribe to order book
		bookMsg := map[string]interface{}{
			"event": "bts:subscribe",
			"data": map[string]string{
				"channel": "order_book_" + pair,
			},
		}

		if err := b.sendMessage(bookMsg); err != nil {
			return err
		}

		// Subscribe to live trades
		tradeMsg := map[string]interface{}{
			"event": "bts:subscribe",
			"data": map[string]string{
				"channel": "live_trades_" + pair,
			},
		}

		if err := b.sendMessage(tradeMsg); err != nil {
			return err
		}
	}

	return nil
}

// handleMessage processes Bitstamp WebSocket messages
func (b *BitstampWebSocketProvider) handleMessage(message []byte) {
	var response map[string]interface{}
	if err := json.Unmarshal(message, &response); err != nil {
		b.BaseWebSocketProvider.logger.Error("Failed to parse Bitstamp message", zap.Error(err))
		return
	}

	event, ok := response["event"].(string)
	if !ok {
		return
	}

	channel, ok := response["channel"].(string)
	if !ok {
		return
	}

	if event == "data" {
		data := response["data"].(map[string]interface{})

		switch {
		case strings.Contains(channel, "live_trades_"):
			b.handleBitstampTradeMessage(data, channel)
		case strings.Contains(channel, "order_book_"):
			b.handleBitstampOrderBookMessage(data, channel)
		}
	}
}

func (b *BitstampWebSocketProvider) handleBitstampTradeMessage(data map[string]interface{}, channel string) {
	// Extract symbol from channel name
	pair := strings.TrimPrefix(channel, "live_trades_")
	symbol := b.convertSymbolFromBitstamp(pair)
	price, _ := decimal.NewFromString(data["price"].(string))
	amount, _ := decimal.NewFromString(data["amount"].(string))

	// Determine side from type
	isBuyerMaker := false
	if tradeType, ok := data["type"].(float64); ok && tradeType == 1 {
		isBuyerMaker = true // type 1 means sell/maker
	}

	timestamp := time.Unix(int64(data["timestamp"].(float64)), 0)

	trade := &TradeData{
		Symbol:       symbol,
		Price:        price,
		Quantity:     amount,
		IsBuyerMaker: isBuyerMaker,
		Timestamp:    timestamp,
		Source:       "bitstamp",
	}

	select {
	case b.tradeChan <- trade:
	default:
		b.BaseWebSocketProvider.logger.Warn("Trade channel full, dropping message", zap.String("symbol", symbol))
	}
}

func (b *BitstampWebSocketProvider) handleBitstampOrderBookMessage(data map[string]interface{}, channel string) {
	// Extract symbol from channel name
	pair := strings.TrimPrefix(channel, "order_book_")
	symbol := b.convertSymbolFromBitstamp(pair)

	var bids, asks []PriceLevel

	if bidData, ok := data["bids"].([]interface{}); ok {
		for _, bidInterface := range bidData {
			bid := bidInterface.([]interface{})
			price, _ := decimal.NewFromString(bid[0].(string))
			quantity, _ := decimal.NewFromString(bid[1].(string))
			bids = append(bids, PriceLevel{Price: price, Quantity: quantity})
		}
	}

	if askData, ok := data["asks"].([]interface{}); ok {
		for _, askInterface := range askData {
			ask := askInterface.([]interface{})
			price, _ := decimal.NewFromString(ask[0].(string))
			quantity, _ := decimal.NewFromString(ask[1].(string))
			asks = append(asks, PriceLevel{Price: price, Quantity: quantity})
		}
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

// Symbol conversion helpers for each exchange
func (k *KrakenWebSocketProvider) convertSymbolToKraken(symbol string) string {
	// BTCUSDT -> XXBTZUSD format
	if symbol == "BTCUSDT" {
		return "XXBTZUSD"
	}
	if symbol == "ETHUSDT" {
		return "XETHZUSD"
	}
	// Generic conversion for other pairs
	if len(symbol) >= 6 {
		base := symbol[:len(symbol)-4]
		quote := symbol[len(symbol)-4:]
		if base == "BTC" {
			base = "XXBT"
		}
		if quote == "USDT" {
			quote = "ZUSD"
		}
		return base + quote
	}
	return symbol
}

func (k *KrakenWebSocketProvider) convertSymbolFromKraken(pair string) string {
	// XXBTZUSD -> BTCUSDT
	if pair == "XXBTZUSD" {
		return "BTCUSDT"
	}
	if pair == "XETHZUSD" {
		return "ETHUSDT"
	}
	// Generic conversion
	symbol := strings.ReplaceAll(pair, "XXBT", "BTC")
	symbol = strings.ReplaceAll(symbol, "ZUSD", "USDT")
	symbol = strings.ReplaceAll(symbol, "XETH", "ETH")
	return symbol
}

func (o *OKXWebSocketProvider) convertSymbolToOKX(symbol string) string {
	// BTCUSDT -> BTC-USDT
	if len(symbol) >= 6 {
		base := symbol[:len(symbol)-4]
		quote := symbol[len(symbol)-4:]
		return base + "-" + quote
	}
	return symbol
}

func (o *OKXWebSocketProvider) convertSymbolFromOKX(instId string) string {
	// BTC-USDT -> BTCUSDT
	return strings.ReplaceAll(instId, "-", "")
}

func (b *BitstampWebSocketProvider) convertSymbolToBitstamp(symbol string) string {
	// BTCUSDT -> btcusd
	if symbol == "BTCUSDT" {
		return "btcusd"
	}
	if symbol == "ETHUSDT" {
		return "ethusd"
	}
	// Generic conversion - remove T from USDT and lowercase
	result := strings.ReplaceAll(strings.ToLower(symbol), "usdt", "usd")
	return result
}

func (b *BitstampWebSocketProvider) convertSymbolFromBitstamp(pair string) string {
	// btcusd -> BTCUSDT
	upper := strings.ToUpper(pair)
	return strings.ReplaceAll(upper, "USD", "USDT")
}
