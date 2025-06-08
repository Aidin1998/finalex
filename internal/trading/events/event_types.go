package events

import "time"

// Standard event topics
const (
	TopicTrade      = "trade"
	TopicOrder      = "order"
	TopicSettlement = "settlement"
	TopicBalance    = "balance"
)

// TradeEvent is published for every trade execution
// Extend as needed for compliance, UI, etc.
type TradeEvent struct {
	TradeID     string
	Pair        string
	Price       string
	Quantity    string
	Side        string
	Timestamp   time.Time
	MakerUserID string
	TakerUserID string
	Meta        map[string]interface{}
}

type OrderEvent struct {
	OrderID   string
	UserID    string
	Type      string
	Status    string
	Pair      string
	Side      string
	Price     string
	Quantity  string
	Timestamp time.Time
	Meta      map[string]interface{}
}

type SettlementEvent struct {
	SettlementID string
	TradeID      string
	Status       string
	Timestamp    time.Time
	Meta         map[string]interface{}
}
