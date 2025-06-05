// model_conversions.go - Utilities for converting between model types
package trading

import (
	"strings"

	model2 "github.com/Aidin1998/pincex_unified/internal/trading/model"
	"github.com/Aidin1998/pincex_unified/pkg/models"
	"github.com/shopspring/decimal"
)

// Conversion helpers between models.Order and model.Order
func toModelOrder(o *models.Order) *model2.Order {
	if o == nil {
		return nil
	}
	return &model2.Order{
		ID:          o.ID,
		UserID:      o.UserID,
		Pair:        o.Symbol,
		Side:        strings.ToUpper(o.Side),
		Type:        strings.ToUpper(o.Type),
		Price:       decimal.NewFromFloat(o.Price),
		Quantity:    decimal.NewFromFloat(o.Quantity),
		TimeInForce: strings.ToUpper(o.TimeInForce),
		Status:      strings.ToUpper(o.Status),
		CreatedAt:   o.CreatedAt,
		UpdatedAt:   o.UpdatedAt,
	}
}

func toAPIOrder(o *model2.Order) *models.Order {
	if o == nil {
		return nil
	}

	price, _ := o.Price.Float64()
	quantity, _ := o.Quantity.Float64()

	return &models.Order{
		ID:          o.ID,
		UserID:      o.UserID,
		Symbol:      o.Pair,
		Side:        o.Side,
		Type:        o.Type,
		Price:       price,
		Quantity:    quantity,
		TimeInForce: o.TimeInForce,
		Status:      o.Status,
		CreatedAt:   o.CreatedAt,
		UpdatedAt:   o.UpdatedAt,
	}
}
