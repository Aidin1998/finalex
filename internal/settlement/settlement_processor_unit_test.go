//go:build unit && settlement
// +build unit,settlement

package settlement

import (
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestSettlementProcessor_BasicFlow(t *testing.T) {
	confirmations := make(chan SettlementConfirmation, 1)
	logger := zap.NewNop()
	processor := &SettlementProcessor{
		logger:        logger,
		confirmations: confirmations,
	}
	msg := SettlementMessage{
		SettlementID: "settle1",
		TradeID:      "trade1",
		Amount:       100.0,
		Asset:        "USD",
		Type:         "fiat",
		Timestamp:    time.Now(),
	}
	status, receipt, err := ProcessSettlementForTest(processor, msg)
	if err != nil {
		t.Fatalf("settlement failed: %v", err)
	}
	if status != SettlementSettled {
		t.Fatalf("unexpected status: %v", status)
	}
	if receipt == "" {
		t.Fatalf("missing receipt")
	}
}
