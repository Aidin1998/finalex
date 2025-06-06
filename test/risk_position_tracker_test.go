//go:build trading
// +build trading

package test

import (
	"testing"

	"github.com/Aidin1998/finalex/internal/trading/risk"
	"github.com/shopspring/decimal"
)

func TestPositionTracker_UpdateAndGetPosition(t *testing.T) {
	pt := risk.NewPositionTracker()
	user := risk.UserID("user1")
	symbol := risk.Symbol("BTCUSDT")
	pt.UpdatePosition(user, symbol, decimal.NewFromInt(5), decimal.NewFromInt(20000))
	pt.UpdatePosition(user, symbol, decimal.NewFromInt(-2), decimal.NewFromInt(21000))
	pos := pt.GetPosition(user, symbol)
	if !pos.Equal(decimal.NewFromInt(3)) {
		t.Errorf("expected position 3, got %s", pos.String())
	}
}
