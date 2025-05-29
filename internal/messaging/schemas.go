package messaging

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// MessageType defines the type of message being sent
type MessageType string

const (
	// Order Events
	MsgOrderPlaced   MessageType = "order.placed"
	MsgOrderUpdated  MessageType = "order.updated"
	MsgOrderCanceled MessageType = "order.canceled"
	MsgOrderFilled   MessageType = "order.filled"
	MsgOrderRejected MessageType = "order.rejected"

	// Trade Events
	MsgTradeExecuted MessageType = "trade.executed"
	MsgTradeSettled  MessageType = "trade.settled"

	// Balance Events
	MsgBalanceUpdated MessageType = "balance.updated"
	MsgFundsLocked    MessageType = "funds.locked"
	MsgFundsUnlocked  MessageType = "funds.unlocked"
	MsgFundsTransfer  MessageType = "funds.transfer"

	// Market Data Events
	MsgOrderBookUpdate MessageType = "orderbook.update"
	MsgTicker          MessageType = "ticker.update"
	MsgCandle          MessageType = "candle.update"

	// Notification Events
	MsgUserNotification MessageType = "notification.user"
	MsgSystemAlert      MessageType = "notification.system"

	// Risk Events
	MsgRiskViolation MessageType = "risk.violation"
	MsgPositionLimit MessageType = "risk.position_limit"
)

// BaseMessage contains common fields for all messages
type BaseMessage struct {
	MessageID     string      `json:"message_id"`
	Type          MessageType `json:"type"`
	Timestamp     time.Time   `json:"timestamp"`
	Version       string      `json:"version"`
	Source        string      `json:"source"`
	CorrelationID string      `json:"correlation_id,omitempty"`
}

// OrderEventMessage represents order-related events
type OrderEventMessage struct {
	BaseMessage
	UserID      string          `json:"user_id"`
	OrderID     string          `json:"order_id"`
	Symbol      string          `json:"symbol"`
	Side        string          `json:"side"`
	Type        string          `json:"order_type"`
	Price       decimal.Decimal `json:"price"`
	Quantity    decimal.Decimal `json:"quantity"`
	FilledQty   decimal.Decimal `json:"filled_quantity"`
	Status      string          `json:"status"`
	TimeInForce string          `json:"time_in_force"`
	Reason      string          `json:"reason,omitempty"`
}

// TradeEventMessage represents trade execution events
type TradeEventMessage struct {
	BaseMessage
	TradeID     string          `json:"trade_id"`
	Symbol      string          `json:"symbol"`
	BuyOrderID  string          `json:"buy_order_id"`
	SellOrderID string          `json:"sell_order_id"`
	BuyUserID   string          `json:"buy_user_id"`
	SellUserID  string          `json:"sell_user_id"`
	Price       decimal.Decimal `json:"price"`
	Quantity    decimal.Decimal `json:"quantity"`
	BuyFee      decimal.Decimal `json:"buy_fee"`
	SellFee     decimal.Decimal `json:"sell_fee"`
	FeeCurrency string          `json:"fee_currency"`
}

// BalanceEventMessage represents balance-related events
type BalanceEventMessage struct {
	BaseMessage
	UserID       string          `json:"user_id"`
	Currency     string          `json:"currency"`
	OldBalance   decimal.Decimal `json:"old_balance"`
	NewBalance   decimal.Decimal `json:"new_balance"`
	OldAvailable decimal.Decimal `json:"old_available"`
	NewAvailable decimal.Decimal `json:"new_available"`
	OldLocked    decimal.Decimal `json:"old_locked"`
	NewLocked    decimal.Decimal `json:"new_locked"`
	Amount       decimal.Decimal `json:"amount"`
	Reference    string          `json:"reference,omitempty"`
	Description  string          `json:"description,omitempty"`
}

// OrderBookUpdateMessage represents order book update events
type OrderBookUpdateMessage struct {
	BaseMessage
	Symbol     string           `json:"symbol"`
	Bids       []OrderBookLevel `json:"bids"`
	Asks       []OrderBookLevel `json:"asks"`
	Sequence   int64            `json:"sequence"`
	IsSnapshot bool             `json:"is_snapshot"`
}

type OrderBookLevel struct {
	Price    decimal.Decimal `json:"price"`
	Quantity decimal.Decimal `json:"quantity"`
}

// TickerUpdateMessage represents ticker update events
type TickerUpdateMessage struct {
	BaseMessage
	Symbol       string          `json:"symbol"`
	LastPrice    decimal.Decimal `json:"last_price"`
	BestBid      decimal.Decimal `json:"best_bid"`
	BestAsk      decimal.Decimal `json:"best_ask"`
	Volume24h    decimal.Decimal `json:"volume_24h"`
	Change24h    decimal.Decimal `json:"change_24h"`
	ChangePct24h decimal.Decimal `json:"change_pct_24h"`
	High24h      decimal.Decimal `json:"high_24h"`
	Low24h       decimal.Decimal `json:"low_24h"`
}

// CandleUpdateMessage represents candle/kline update events
type CandleUpdateMessage struct {
	BaseMessage
	Symbol    string          `json:"symbol"`
	Interval  string          `json:"interval"`
	OpenTime  time.Time       `json:"open_time"`
	CloseTime time.Time       `json:"close_time"`
	Open      decimal.Decimal `json:"open"`
	High      decimal.Decimal `json:"high"`
	Low       decimal.Decimal `json:"low"`
	Close     decimal.Decimal `json:"close"`
	Volume    decimal.Decimal `json:"volume"`
	IsClosed  bool            `json:"is_closed"`
}

// NotificationMessage represents user and system notifications
type NotificationMessage struct {
	BaseMessage
	UserID   string                 `json:"user_id,omitempty"`
	Title    string                 `json:"title"`
	Message  string                 `json:"message"`
	Level    string                 `json:"level"` // info, warning, error, critical
	Category string                 `json:"category"`
	Data     map[string]interface{} `json:"data,omitempty"`
}

// RiskEventMessage represents risk management events
type RiskEventMessage struct {
	BaseMessage
	UserID         string                 `json:"user_id"`
	Symbol         string                 `json:"symbol,omitempty"`
	RiskType       string                 `json:"risk_type"`
	Severity       string                 `json:"severity"`
	Message        string                 `json:"message"`
	CurrentValue   decimal.Decimal        `json:"current_value"`
	ThresholdValue decimal.Decimal        `json:"threshold_value"`
	Data           map[string]interface{} `json:"data,omitempty"`
}

// FundsOperationMessage represents fund lock/unlock operations
type FundsOperationMessage struct {
	BaseMessage
	UserID   string          `json:"user_id"`
	Currency string          `json:"currency"`
	Amount   decimal.Decimal `json:"amount"`
	OrderID  string          `json:"order_id,omitempty"`
	Reason   string          `json:"reason"`
}

// Topic defines Kafka topics for different message types
type Topic string

const (
	TopicOrderEvents   Topic = "order-events"
	TopicTradeEvents   Topic = "trade-events"
	TopicBalanceEvents Topic = "balance-events"
	TopicMarketData    Topic = "market-data"
	TopicNotifications Topic = "notifications"
	TopicRiskEvents    Topic = "risk-events"
	TopicFundsOps      Topic = "funds-operations"
)

// GetTopic returns the appropriate topic for a message type
func GetTopic(msgType MessageType) Topic {
	switch msgType {
	case MsgOrderPlaced, MsgOrderUpdated, MsgOrderCanceled, MsgOrderFilled, MsgOrderRejected:
		return TopicOrderEvents
	case MsgTradeExecuted, MsgTradeSettled:
		return TopicTradeEvents
	case MsgBalanceUpdated, MsgFundsTransfer:
		return TopicBalanceEvents
	case MsgFundsLocked, MsgFundsUnlocked:
		return TopicFundsOps
	case MsgOrderBookUpdate, MsgTicker, MsgCandle:
		return TopicMarketData
	case MsgUserNotification, MsgSystemAlert:
		return TopicNotifications
	case MsgRiskViolation, MsgPositionLimit:
		return TopicRiskEvents
	default:
		return TopicNotifications
	}
}

// NewBaseMessage creates a new base message with common fields
func NewBaseMessage(msgType MessageType, source string, correlationID string) BaseMessage {
	return BaseMessage{
		MessageID:     uuid.New().String(),
		Type:          msgType,
		Timestamp:     time.Now().UTC(),
		Version:       "1.0",
		Source:        source,
		CorrelationID: correlationID,
	}
}
