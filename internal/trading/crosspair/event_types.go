package crosspair

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Centralized event/DTO struct for cross-pair events
// Use this for all event publishing, WebSocket, EventBus, compliance, etc.
type CrossPairOrderEvent struct {
	OrderID        uuid.UUID       `json:"order_id"`
	UserID         uuid.UUID       `json:"user_id"`
	Status         string          `json:"status"`
	Leg1TradeID    uuid.UUID       `json:"leg1_trade_id,omitempty"`
	Leg2TradeID    uuid.UUID       `json:"leg2_trade_id,omitempty"`
	FillAmountLeg1 decimal.Decimal `json:"fill_amount_leg1"`
	FillAmountLeg2 decimal.Decimal `json:"fill_amount_leg2"`
	SyntheticRate  decimal.Decimal `json:"synthetic_rate"`
	Fee            []CrossPairFee  `json:"fee"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
	FilledAt       *time.Time      `json:"filled_at,omitempty"`
	CanceledAt     *time.Time      `json:"canceled_at,omitempty"`
	Error          *string         `json:"error,omitempty"`
}

// If there are other event/DTO structs used for cross-pair, add them here.
