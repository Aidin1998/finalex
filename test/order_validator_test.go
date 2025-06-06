//go:build trading
// +build trading

package test

import (
	"testing"
	"time"

	"github.com/Aidin1998/finalex/internal/trading"
	"github.com/Aidin1998/finalex/internal/trading/model"
	"github.com/shopspring/decimal"
)

func makeValidator() *trading.OrderValidator {
	return &trading.OrderValidator{
		SymbolMap: map[string]trading.SymbolID{"BTCUSDT": 0},
		PriceRanges: []trading.PriceRange{{
			Min:      decimal.NewFromInt(10000),
			Max:      decimal.NewFromInt(1000000),
			TickSize: decimal.NewFromInt(10),
		}},
		SizeRanges: []trading.SizeRange{{
			Min: decimal.NewFromInt(1),
			Max: decimal.NewFromInt(100),
		}},
		TradingHours: []trading.TradingWindow{{
			Start: 0, End: 1440,
		}},
		OrderTypes:  map[string]struct{}{model.OrderTypeLimit: {}, model.OrderTypeMarket: {}},
		TimeInForce: map[string]struct{}{model.TimeInForceGTC: {}, model.TimeInForceIOC: {}},
	}
}

func TestOrderValidator_ValidateFast_OK(t *testing.T) {
	validator := makeValidator()
	order := &model.Order{
		Pair:        "BTCUSDT",
		Price:       decimal.NewFromInt(20000),
		Quantity:    decimal.NewFromInt(10),
		Type:        model.OrderTypeLimit,
		TimeInForce: model.TimeInForceGTC,
		CreatedAt:   time.Now(),
	}
	trading.SanitizeOrderInPlace(order)
	res := validator.ValidateFast(order)
	if res != trading.ValidationOK {
		t.Errorf("expected ValidationOK, got %v", res)
	}
}

func TestOrderValidator_ValidateFast_InvalidSymbol(t *testing.T) {
	validator := makeValidator()
	order := &model.Order{
		Pair:        "ETHUSDT",
		Price:       decimal.NewFromInt(20000),
		Quantity:    decimal.NewFromInt(10),
		Type:        model.OrderTypeLimit,
		TimeInForce: model.TimeInForceGTC,
	}
	trading.SanitizeOrderInPlace(order)
	res := validator.ValidateFast(order)
	if res != trading.ErrInvalidSymbol {
		t.Errorf("expected ErrInvalidSymbol, got %v", res)
	}
}

func TestOrderValidator_ValidateBatch(t *testing.T) {
	validator := makeValidator()
	orders := []*model.Order{
		{Pair: "BTCUSDT", Price: decimal.NewFromInt(20000), Quantity: decimal.NewFromInt(10), Type: model.OrderTypeLimit, TimeInForce: model.TimeInForceGTC},
		{Pair: "BTCUSDT", Price: decimal.NewFromInt(9999), Quantity: decimal.NewFromInt(10), Type: model.OrderTypeLimit, TimeInForce: model.TimeInForceGTC},
	}
	for _, o := range orders {
		trading.SanitizeOrderInPlace(o)
	}
	results := make([]trading.ValidationResult, len(orders))
	validator.ValidateBatch(orders, results)
	if results[0] != trading.ValidationOK {
		t.Errorf("expected ValidationOK for order 0, got %v", results[0])
	}
	if results[1] != trading.ErrInvalidPrice {
		t.Errorf("expected ErrInvalidPrice for order 1, got %v", results[1])
	}
}

func TestSanitizeOrderInPlace_UTF8(t *testing.T) {
	order := &model.Order{Pair: " BTCusdt\uFFFD ", Side: " buy ", Type: " limit "}
	trading.SanitizeOrderInPlace(order)
	if order.Pair != "BTCUSDT" {
		t.Errorf("expected BTCUSDT, got %q", order.Pair)
	}
	if order.Side != "BUY" {
		t.Errorf("expected BUY, got %q", order.Side)
	}
	if order.Type != "LIMIT" {
		t.Errorf("expected LIMIT, got %q", order.Type)
	}
}
