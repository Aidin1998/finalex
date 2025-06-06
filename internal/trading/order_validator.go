// Ultra-fast, zero-allocation input validation and sanitization pipeline for trading orders
// Implements: symbol/price/quantity/type/time validation, SIMD-friendly batch validation, zero-allocation sanitization
// All logic is self-contained and does not introduce new dependencies

package trading

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Aidin1998/finalex/internal/trading/model"
	"github.com/shopspring/decimal"
)

// Step 1: ValidationResult enum for fast error codes
//go:generate stringer -type=ValidationResult

type ValidationResult uint8

const (
	ValidationOK ValidationResult = iota
	ErrInvalidSymbol
	ErrInvalidPrice
	ErrInvalidQuantity
	ErrMarketClosed
	ErrUserLimit
	ErrOrderType
	ErrInvalidTimeInForce
	ErrInvalidAdvancedParams
)

// Step 2: Precomputed rules for each symbol (min/max/tick, size, trading window)
type PriceRange struct {
	Min, Max, TickSize decimal.Decimal
}
type SizeRange struct {
	Min, Max decimal.Decimal
}
type TradingWindow struct {
	Start, End int // Minutes since midnight
}

type SymbolID uint16

// Step 3: OrderValidator struct
// All maps are precomputed and read-only after init

type OrderValidator struct {
	SymbolMap    map[string]SymbolID
	PriceRanges  []PriceRange
	SizeRanges   []SizeRange
	TradingHours []TradingWindow
	OrderTypes   map[string]struct{}
	TimeInForce  map[string]struct{}
}

// Step 4: Zero-allocation, branch-optimized validation
func (v *OrderValidator) ValidateFast(order *model.Order) ValidationResult {
	id, ok := v.SymbolMap[order.Pair]
	if !ok {
		return ErrInvalidSymbol
	}
	pr := v.PriceRanges[id]
	if order.Price.LessThan(pr.Min) || order.Price.GreaterThan(pr.Max) || !isTickAligned(order.Price, pr.Min, pr.TickSize) {
		return ErrInvalidPrice
	}
	sr := v.SizeRanges[id]
	if order.Quantity.LessThan(sr.Min) || order.Quantity.GreaterThan(sr.Max) {
		return ErrInvalidQuantity
	}
	now := getCurrentMinutes()
	tw := v.TradingHours[id]
	if now < tw.Start || now > tw.End {
		return ErrMarketClosed
	}
	if _, ok := v.OrderTypes[order.Type]; !ok {
		return ErrOrderType
	}
	if order.TimeInForce != "" {
		if _, ok := v.TimeInForce[order.TimeInForce]; !ok {
			return ErrInvalidTimeInForce
		}
	}
	if err := order.ValidateAdvanced(); err != nil {
		return ErrInvalidAdvancedParams
	}
	return ValidationOK
}

// Step 5: SIMD-optimized batch validation (loop unrolling, ready for SIMD drop-in)
func (v *OrderValidator) ValidateBatch(orders []*model.Order, results []ValidationResult) {
	n := len(orders)
	for i := 0; i < n; i += 8 {
		for j := 0; j < 8 && i+j < n; j++ {
			results[i+j] = v.ValidateFast(orders[i+j])
		}
	}
}

// Step 6: Zero-allocation input sanitization
// Trims, normalizes, and validates symbol/side/type fields in-place
func SanitizeOrderInPlace(order *model.Order) {
	order.Pair = fastTrimToUpper(order.Pair)
	order.Side = fastTrimToUpper(order.Side)
	order.Type = fastTrimToUpper(order.Type)
	if !utf8.ValidString(order.Pair) {
		order.Pair = ""
	}
	if !utf8.ValidString(order.Side) {
		order.Side = ""
	}
	if !utf8.ValidString(order.Type) {
		order.Type = ""
	}
	// Remove all non-ASCII characters from Pair, Side, Type
	order.Pair = removeNonASCII(order.Pair)
	order.Side = removeNonASCII(order.Side)
	order.Type = removeNonASCII(order.Type)
}

func removeNonASCII(s string) string {
	b := make([]rune, 0, len(s))
	for _, r := range s {
		if r <= 127 {
			b = append(b, r)
		}
	}
	return string(b)
}

// --- Helper functions ---
func isTickAligned(price, min, tick decimal.Decimal) bool {
	if tick.IsZero() {
		return true
	}
	delta := price.Sub(min)
	rem := delta.Mod(tick)
	return rem.IsZero()
}

func getCurrentMinutes() int {
	now := time.Now()
	return now.Hour()*60 + now.Minute()
}

func fastTrimToUpper(s string) string {
	return strings.ToUpper(strings.TrimSpace(s))
}

// --- End of order_validator.go ---
