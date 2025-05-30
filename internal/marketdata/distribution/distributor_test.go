package distribution

import (
	"testing"
	"time"
)

// TestDistributorDeltaCompression verifies that only changed updates are sent when compression is enabled.
func TestDistributorDeltaCompression(t *testing.T) {
	sm := NewSubscriptionManager()
	d := NewDistributor(sm, 10*time.Millisecond)
	// No need to run d.Run for flushAll tests

	clientID := "client1"
	// Subscribe with compression
	sm.Subscribe(&Subscription{ClientID: clientID, Symbol: "SYM", PriceLevels: []int{0}, Frequency: 0, Compression: true})
	// Register client channel
	ch := make(chan interface{}, 10)
	d.RegisterClient(clientID, ch)

	// First update: should be sent
	u1 := Update{Symbol: "SYM", PriceLevel: 0, Bid: 1.0, Ask: 2.0, Timestamp: time.Now()}
	d.Publish(u1)
	d.flushAll()
	msg1 := <-ch
	slice1, ok := msg1.([]Update)
	if !ok {
		t.Fatalf("expected []Update, got %T", msg1)
	}
	if len(slice1) != 1 || slice1[0] != u1 {
		t.Fatalf("unexpected first delta: %v", slice1)
	}

	// Second update identical: no new message
	u2 := u1
	d.Publish(u2)
	d.flushAll()
	select {
	case m := <-ch:
		t.Fatalf("expected no delta, but got %v", m)
	default:
		// ok
	}

	// Third update changed: should be sent
	u3 := Update{Symbol: "SYM", PriceLevel: 0, Bid: 1.5, Ask: 2.5, Timestamp: time.Now()}
	d.Publish(u3)
	d.flushAll()
	msg3 := <-ch
	slice3, ok := msg3.([]Update)
	if !ok {
		t.Fatalf("expected []Update, got %T", msg3)
	}
	if len(slice3) != 1 || slice3[0] != u3 {
		t.Fatalf("unexpected third delta: %v", slice3)
	}
}

// BenchmarkDistributorFlush measures performance of batching and compression
func BenchmarkDistributorFlush(b *testing.B) {
	sm := NewSubscriptionManager()
	d := NewDistributor(sm, 0)
	clientID := "client-bench"
	sm.Subscribe(&Subscription{ClientID: clientID, Symbol: "X", PriceLevels: []int{0}, Compression: false})
	d.RegisterClient(clientID, make(chan interface{}, b.N))

	// prepare updates
	updates := make([]Update, b.N)
	for i := 0; i < b.N; i++ {
		updates[i] = Update{Symbol: "X", PriceLevel: 0, Bid: float64(i), Ask: float64(i), Timestamp: time.Now()}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.Publish(updates[i])
		d.flushAll()
	}
}
