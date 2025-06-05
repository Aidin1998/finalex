package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	ws "github.com/Aidin1998/finalex/internal/infrastructure/ws"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// MarketDataMessageService handles market data distribution via message queue
type MarketDataMessageService struct {
	messageBus *MessageBus
	wsHub      *ws.Hub
	logger     *zap.Logger
}

// NewMarketDataMessageService creates a new message-driven market data service
func NewMarketDataMessageService(
	messageBus *MessageBus,
	wsHub *ws.Hub,
	logger *zap.Logger,
) *MarketDataMessageService {
	service := &MarketDataMessageService{
		messageBus: messageBus,
		wsHub:      wsHub,
		logger:     logger,
	}

	// Register message handlers
	service.registerHandlers()

	return service
}

// registerHandlers registers all message handlers for market data operations
func (s *MarketDataMessageService) registerHandlers() {
	s.messageBus.RegisterHandler(MsgOrderBookUpdate, s.handleOrderBookUpdate)
	s.messageBus.RegisterHandler(MsgTicker, s.handleTickerUpdate)
	s.messageBus.RegisterHandler(MsgCandle, s.handleCandleUpdate)
}

// PublishOrderBookUpdate publishes an order book update
func (s *MarketDataMessageService) PublishOrderBookUpdate(ctx context.Context, symbol string, bids, asks []OrderBookLevel, sequence int64, isSnapshot bool) error {
	update := &OrderBookUpdateMessage{
		BaseMessage: NewBaseMessage(MsgOrderBookUpdate, "matching-engine", ""),
		Symbol:      symbol,
		Bids:        bids,
		Asks:        asks,
		Sequence:    sequence,
		IsSnapshot:  isSnapshot,
	}

	s.logger.Debug("Publishing order book update",
		zap.String("symbol", symbol),
		zap.Int64("sequence", sequence),
		zap.Bool("is_snapshot", isSnapshot),
		zap.Int("bids_count", len(bids)),
		zap.Int("asks_count", len(asks)))

	return s.messageBus.PublishMarketDataEvent(ctx, MsgOrderBookUpdate, symbol, update)
}

// PublishTickerUpdate publishes a ticker update
func (s *MarketDataMessageService) PublishTickerUpdate(ctx context.Context, symbol string, ticker *TickerData) error {
	update := &TickerUpdateMessage{
		BaseMessage:  NewBaseMessage(MsgTicker, "market-data", ""),
		Symbol:       symbol,
		LastPrice:    ticker.LastPrice,
		BestBid:      ticker.BestBid,
		BestAsk:      ticker.BestAsk,
		Volume24h:    ticker.Volume24h,
		Change24h:    ticker.Change24h,
		ChangePct24h: ticker.ChangePct24h,
		High24h:      ticker.High24h,
		Low24h:       ticker.Low24h,
	}

	s.logger.Debug("Publishing ticker update",
		zap.String("symbol", symbol),
		zap.String("last_price", ticker.LastPrice.String()),
		zap.String("volume_24h", ticker.Volume24h.String()))

	return s.messageBus.PublishMarketDataEvent(ctx, MsgTicker, symbol, update)
}

// PublishCandleUpdate publishes a candle/kline update
func (s *MarketDataMessageService) PublishCandleUpdate(ctx context.Context, symbol, interval string, candle *CandleData) error {
	update := &CandleUpdateMessage{
		BaseMessage: NewBaseMessage(MsgCandle, "market-data", ""),
		Symbol:      symbol,
		Interval:    interval,
		OpenTime:    candle.OpenTime,
		CloseTime:   candle.CloseTime,
		Open:        candle.Open,
		High:        candle.High,
		Low:         candle.Low,
		Close:       candle.Close,
		Volume:      candle.Volume,
		IsClosed:    candle.IsClosed,
	}

	s.logger.Debug("Publishing candle update",
		zap.String("symbol", symbol),
		zap.String("interval", interval),
		zap.String("close_price", candle.Close.String()),
		zap.Bool("is_closed", candle.IsClosed))

	return s.messageBus.PublishMarketDataEvent(ctx, MsgCandle, symbol, update)
}

// handleOrderBookUpdate processes order book update events for distribution
func (s *MarketDataMessageService) handleOrderBookUpdate(ctx context.Context, msg *ReceivedMessage) error {
	var update OrderBookUpdateMessage
	if err := json.Unmarshal(msg.Value, &update); err != nil {
		return fmt.Errorf("failed to unmarshal order book update: %w", err)
	}

	s.logger.Debug("Processing order book update",
		zap.String("symbol", update.Symbol),
		zap.Int64("sequence", update.Sequence),
		zap.Bool("is_snapshot", update.IsSnapshot))

	// Broadcast to WebSocket clients
	if s.wsHub != nil {
		if data, err := json.Marshal(update); err == nil {
			topic := fmt.Sprintf("orderbook.%s", update.Symbol)
			s.wsHub.Broadcast(topic, data)
			s.logger.Debug("Broadcasted order book update to WebSocket clients",
				zap.String("topic", topic))
		} else {
			s.logger.Error("Failed to marshal order book update for WebSocket broadcast", zap.Error(err))
		}
	}

	return nil
}

// handleTickerUpdate processes ticker update events
func (s *MarketDataMessageService) handleTickerUpdate(ctx context.Context, msg *ReceivedMessage) error {
	var update TickerUpdateMessage
	if err := json.Unmarshal(msg.Value, &update); err != nil {
		return fmt.Errorf("failed to unmarshal ticker update: %w", err)
	}

	s.logger.Debug("Processing ticker update",
		zap.String("symbol", update.Symbol),
		zap.String("last_price", update.LastPrice.String()))

	// Broadcast to WebSocket clients
	if s.wsHub != nil {
		if data, err := json.Marshal(update); err == nil {
			topic := fmt.Sprintf("ticker.%s", update.Symbol)
			s.wsHub.Broadcast(topic, data)
			s.logger.Debug("Broadcasted ticker update to WebSocket clients",
				zap.String("topic", topic))
		} else {
			s.logger.Error("Failed to marshal ticker update for WebSocket broadcast", zap.Error(err))
		}
	}

	return nil
}

// handleCandleUpdate processes candle update events
func (s *MarketDataMessageService) handleCandleUpdate(ctx context.Context, msg *ReceivedMessage) error {
	var update CandleUpdateMessage
	if err := json.Unmarshal(msg.Value, &update); err != nil {
		return fmt.Errorf("failed to unmarshal candle update: %w", err)
	}

	s.logger.Debug("Processing candle update",
		zap.String("symbol", update.Symbol),
		zap.String("interval", update.Interval),
		zap.String("close_price", update.Close.String()))

	// Broadcast to WebSocket clients
	if s.wsHub != nil {
		if data, err := json.Marshal(update); err == nil {
			topic := fmt.Sprintf("candle.%s.%s", update.Symbol, update.Interval)
			s.wsHub.Broadcast(topic, data)
			s.logger.Debug("Broadcasted candle update to WebSocket clients",
				zap.String("topic", topic))
		} else {
			s.logger.Error("Failed to marshal candle update for WebSocket broadcast", zap.Error(err))
		}
	}

	return nil
}

// TickerData represents ticker/price data
type TickerData struct {
	LastPrice    decimal.Decimal
	BestBid      decimal.Decimal
	BestAsk      decimal.Decimal
	Volume24h    decimal.Decimal
	Change24h    decimal.Decimal
	ChangePct24h decimal.Decimal
	High24h      decimal.Decimal
	Low24h       decimal.Decimal
}

// CandleData represents candle/kline data
type CandleData struct {
	OpenTime  time.Time
	CloseTime time.Time
	Open      decimal.Decimal
	High      decimal.Decimal
	Low       decimal.Decimal
	Close     decimal.Decimal
	Volume    decimal.Decimal
	IsClosed  bool
}

// PublishTradeForMarketData publishes trade data for market data calculation
func (s *MarketDataMessageService) PublishTradeForMarketData(ctx context.Context, trade *TradeEventMessage) error {
	s.logger.Debug("Processing trade for market data",
		zap.String("trade_id", trade.TradeID),
		zap.String("symbol", trade.Symbol),
		zap.String("price", trade.Price.String()),
		zap.String("quantity", trade.Quantity.String()))

	// Update ticker based on trade
	// This is a simplified example - in reality you'd have more sophisticated price calculation
	ticker := &TickerData{
		LastPrice:    trade.Price,
		BestBid:      decimal.Zero,   // Would be calculated from order book
		BestAsk:      decimal.Zero,   // Would be calculated from order book
		Volume24h:    trade.Quantity, // Would be accumulated over 24h
		Change24h:    decimal.Zero,   // Would be calculated from 24h ago price
		ChangePct24h: decimal.Zero,   // Would be calculated as percentage
		High24h:      trade.Price,    // Would be max over 24h
		Low24h:       trade.Price,    // Would be min over 24h
	}

	return s.PublishTickerUpdate(ctx, trade.Symbol, ticker)
}

// BatchPublishMarketData publishes multiple market data updates in a batch
func (s *MarketDataMessageService) BatchPublishMarketData(ctx context.Context, updates []MarketDataBatch) error {
	if len(updates) == 0 {
		return nil
	}

	batchMessages := make([]BatchMessage, len(updates))
	for i, update := range updates {
		key := update.Symbol
		if update.Interval != "" {
			key = fmt.Sprintf("%s:%s", update.Symbol, update.Interval)
		}

		batchMessages[i] = BatchMessage{
			Key:     key,
			Message: update.Data,
		}
	}

	s.logger.Debug("Publishing market data batch",
		zap.Int("count", len(updates)))

	return s.messageBus.PublishBatch(ctx, TopicMarketData, batchMessages)
}

// MarketDataBatch represents a batch of market data updates
type MarketDataBatch struct {
	Symbol   string
	Interval string // For candle data
	Data     interface{}
}
