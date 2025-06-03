//go:build unit && settlement
// +build unit,settlement

package settlement

import (
	"testing"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/settlement"
)

func TestSettlementEngine_BasicFlow(t *testing.T) {
	engine := settlement.NewSettlementEngine()
	trade1 := settlement.TradeCapture{
		TradeID:   "T1",
		UserID:    "U1",
		Symbol:    "BTCUSDT",
		Side:      "buy",
		Quantity:  1.5,
		Price:     50000,
		AssetType: "crypto",
		MatchedAt: time.Now(),
	}
	trade2 := settlement.TradeCapture{
		TradeID:   "T2",
		UserID:    "U2",
		Symbol:    "BTCUSDT",
		Side:      "sell",
		Quantity:  1.5,
		Price:     50000,
		AssetType: "crypto",
		MatchedAt: time.Now(),
	}
	engine.CaptureTrade(trade1)
	engine.CaptureTrade(trade2)

	netPositions := engine.NetPositions()
	if len(netPositions) != 2 {
		t.Fatalf("expected 2 net positions, got %d", len(netPositions))
	}
	for _, pos := range netPositions {
		if pos.UserID == "U1" {
			if pos.NetQty != 1.5 {
				t.Fatalf("U1 net qty: got %v, want 1.5", pos.NetQty)
			}
		} else if pos.UserID == "U2" {
			if pos.NetQty != -1.5 {
				t.Fatalf("U2 net qty: got %v, want -1.5", pos.NetQty)
			}
		}
	}

	engine.ClearAndSettle()
	if err := engine.Reconcile(); err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}
}

// TestSettlementProcessor_BasicFlow moved to internal/settlement/settlement_processor_unit_test.go for proper package access.
