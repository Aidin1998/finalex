// =============================
// Orbit CEX Cancel Request
// =============================
// This file defines the CancelRequest type, which is used to request the cancellation of an order in the matching engine.
//
// How it works:
// - CancelRequest holds the order ID, trading pair, and user ID for the order to be canceled.
// - Used by the engine and recovery logic to process order cancellations.

package orderbook

import "github.com/google/uuid"

type CancelRequest struct {
	OrderID uuid.UUID
	Pair    string
	UserID  uuid.UUID
}
