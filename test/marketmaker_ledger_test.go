//go:build marketmaker

package test

import (
	"testing"
	"time"

	"github.com/Aidin1998/finalex/internal/marketmaking/marketmaker"
)

func TestLedgerRecordAndEvents(t *testing.T) {
	ledger := marketmaker.NewLedger()
	event := marketmaker.LedgerEvent{
		Timestamp: time.Now(),
		Pair:      "BTCUSDT",
		Action:    "create",
		OrderID:   "order123",
		Details:   map[string]interface{}{"side": "buy", "price": 50000.0, "qty": 0.1},
	}
	ledger.Record(event)
	all := ledger.Events()
	if len(all) != 1 {
		t.Fatalf("expected 1 event, got %d", len(all))
	}
	if all[0].OrderID != "order123" {
		t.Errorf("expected OrderID 'order123', got '%s'", all[0].OrderID)
	}
}
