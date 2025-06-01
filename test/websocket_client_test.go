package test

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/marketdata/distribution"
	transportWS "github.com/Aidin1998/pincex_unified/internal/marketdata/distribution/transport"
)

func TestWSClient_SubscribeAndReceive(t *testing.T) {
	// Setup distribution
	sm := distribution.NewSubscriptionManager()
	d := distribution.NewDistributor(sm, 20*time.Millisecond)
	go d.Run()

	// Start WebSocket server
	server := transportWS.NewWebSocketServer(d, sm)
	ts := httptest.NewServer(server)
	defer ts.Close()

	// Create client
	wsURL := "ws" + ts.URL[len("http"):]
	client := NewWSClient(wsURL+"?symbol=SYM", time.Second)
	defer client.Close()

	// Subscribe
	if err := client.Subscribe("SYM"); err != nil {
		t.Fatalf("subscribe error: %v", err)
	}

	// Publish update
	upd := distribution.Update{Symbol: "SYM", PriceLevel: 0, Bid: 9.99, Ask: 8.88, Timestamp: time.Now()}
	d.Publish(upd)

	// Receive
	recv, ok := client.Next()
	if !ok {
		t.Fatalf("did not receive update")
	}
	if recv.Bid != upd.Bid || recv.Ask != upd.Ask {
		t.Fatalf("mismatch: got %v, want %v", recv, upd)
	}
}
