// Package dto provides centralized event and data transfer object (DTO) definitions
// for the cross-pair trading module. All event contracts and shared models used for
// event publishing, WebSocket, EventBus, and compliance should be defined here to
// ensure consistency across the codebase.
package dto

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// CrossPairOrderEvent is the canonical event contract for all cross-pair order events.
// It is used for real-time notifications, event bus publishing, compliance/audit hooks,
// and frontend updates. All fields are required unless marked as omitempty.
type CrossPairOrderEvent struct {
	OrderID        uuid.UUID       `json:"order_id"`                // Unique order identifier
	UserID         uuid.UUID       `json:"user_id"`                 // User who placed the order
	Status         string          `json:"status"`                  // Order status (e.g., CREATED, PARTIALLY_FILLED, FILLED, CANCELED, FAILED)
	Leg1TradeID    uuid.UUID       `json:"leg1_trade_id,omitempty"` // Trade ID for first leg (if applicable)
	Leg2TradeID    uuid.UUID       `json:"leg2_trade_id,omitempty"` // Trade ID for second leg (if applicable)
	FillAmountLeg1 decimal.Decimal `json:"fill_amount_leg1"`        // Amount filled in first leg (zero if not applicable)
	FillAmountLeg2 decimal.Decimal `json:"fill_amount_leg2"`        // Amount filled in second leg (zero if not applicable)
	SyntheticRate  decimal.Decimal `json:"synthetic_rate"`          // Synthetic rate for the cross-pair trade
	Fee            []CrossPairFee  `json:"fee"`                     // List of fees applied to the order
	CreatedAt      time.Time       `json:"created_at"`              // Order creation time
	UpdatedAt      time.Time       `json:"updated_at"`              // Last update time
	FilledAt       *time.Time      `json:"filled_at,omitempty"`     // Time when order was fully filled (if applicable)
	CanceledAt     *time.Time      `json:"canceled_at,omitempty"`   // Time when order was canceled (if applicable)
	Error          *string         `json:"error,omitempty"`         // Error message if order failed
}

// CrossPairFee represents a fee applied to a cross-pair trade or order event.
// This struct is used in event contracts and audit trails.
type CrossPairFee struct {
	Asset   string          `json:"asset"`    // Asset in which the fee is charged
	Amount  decimal.Decimal `json:"amount"`   // Fee amount
	FeeType string          `json:"fee_type"` // Type of fee (e.g., TRADING, SPREAD, SLIPPAGE)
	Pair    string          `json:"pair"`     // Trading pair the fee applies to
}

// Add additional event/DTO structs for cross-pair trading here as needed.
