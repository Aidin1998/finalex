//go:build trading
// +build trading

package test

import (
	"testing"

	"github.com/Aidin1998/finalex/internal/trading/orderbook"
)

func TestZeroCopyOrderPoolAndRingBuffer(t *testing.T) {
	pool := orderbook.NewOrderPool()
	ring := orderbook.NewOrderRingBuffer()
	order := pool.GetOrder()
	order.ID = 123
	order.UserID = 42
	order.Symbol = 1
	order.Price = 100_000_000      // 1.0 in fixed point
	order.Quantity = 2_000_000_000 // 2.0 in fixed point
	order.Side = 1
	order.Type = 2
	order.Status = 0
	if !ring.Enqueue(order) {
		t.Fatal("ring buffer enqueue failed")
	}
	order2 := ring.Dequeue()
	if order2 == nil || order2.ID != 123 || order2.UserID != 42 {
		t.Fatalf("dequeue mismatch: got %+v", order2)
	}
	pool.PutOrder(order2)
}

func TestSymbolInterning(t *testing.T) {
	si := orderbook.NewSymbolInterning()
	id := si.RegisterSymbol("BTCUSDT")
	if si.SymbolFromID(id) != "BTCUSDT" {
		t.Errorf("symbol interning failed: got %s", si.SymbolFromID(id))
	}
}
