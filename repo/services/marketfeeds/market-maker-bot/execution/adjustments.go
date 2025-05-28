package execution

import (
	"errors"
	"fmt"
)

// Order represents a trading order with its details.
type TradeOrder struct {
	ID       string
	Quantity int
	Price    float64
	Status   string // e.g., "filled", "partially_filled", "canceled"
	Partial  bool
}

// Adjustments handles fill and cancel adjustments for orders.
type Adjustments struct{}

// NewAdjustments creates a new instance of Adjustments.
func NewAdjustments() *Adjustments {
	return &Adjustments{}
}

// HandleFill processes a fill for an order, adjusting its status and quantity.
func (a *Adjustments) HandleFill(order *TradeOrder, filledQuantity int) error {
	if filledQuantity <= 0 {
		return errors.New("filled quantity must be greater than zero")
	}
	if order.Status == "canceled" {
		return errors.New("cannot fill a canceled order")
	}
	if filledQuantity > order.Quantity {
		return errors.New("filled quantity exceeds order quantity")
	}

	order.Quantity -= filledQuantity
	if order.Quantity == 0 {
		order.Status = "filled"
	} else {
		order.Status = "partially_filled"
		order.Partial = true
	}

	return nil
}

// HandleCancel processes a cancellation for an order.
func (a *Adjustments) HandleCancel(order *TradeOrder) error {
	if order.Status == "filled" {
		return errors.New("cannot cancel a filled order")
	}
	order.Status = "canceled"
	return nil
}

// PrintOrderStatus prints the current status of the order.
func (a *Adjustments) PrintOrderStatus(order *TradeOrder) {
	fmt.Printf("Order ID: %s, Quantity: %d, Price: %.2f, Status: %s\n", order.ID, order.Quantity, order.Price, order.Status)
}
