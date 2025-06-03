package test

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/marketdata/distribution"
	transportWS "github.com/Aidin1998/pincex_unified/internal/marketdata/distribution/transport"
	clientpkg "github.com/Aidin1998/pincex_unified/pkg/marketdata/client"
)

func TestWSClient_SubscribeAndReceive(t *testing.T) {
	// Setup distribution with shorter flush interval for testing
	sm := distribution.NewSubscriptionManager()
	d := distribution.NewDistributor(sm, 10*time.Millisecond)
	go d.Run()
	defer d.Stop()

	// Start WebSocket server
	server := transportWS.NewWebSocketServer(d, sm)
	ts := httptest.NewServer(server)
	defer ts.Close()

	// Give the server a moment to fully start
	time.Sleep(10 * time.Millisecond)

	// Create client
	wsURL := "ws" + ts.URL[len("http"):]
	client := clientpkg.NewWSClient(wsURL+"?symbol=SYM", time.Second)
	defer client.Close()

	// Give client time to connect
	time.Sleep(50 * time.Millisecond)

	// Subscribe
	if err := client.Subscribe("SYM"); err != nil {
		t.Fatalf("subscribe error: %v", err)
	}

	// Give subscription time to be processed
	time.Sleep(20 * time.Millisecond)

	// Publish update
	upd := distribution.Update{Symbol: "SYM", PriceLevel: 0, Bid: 9.99, Ask: 8.88, Timestamp: time.Now()}
	d.Publish(upd)

	// Wait for the flush interval to ensure the update is processed
	time.Sleep(30 * time.Millisecond)

	// Receive with timeout
	select {
	case recv, ok := <-func() <-chan distribution.Update {
		ch := make(chan distribution.Update, 1)
		go func() {
			defer close(ch)
			upd, ok := client.Next()
			if ok {
				ch <- upd
			}
		}()
		return ch
	}():
		if !ok {
			t.Fatalf("did not receive update")
		}
		if recv.Bid != upd.Bid || recv.Ask != upd.Ask {
			t.Fatalf("mismatch: got %v, want %v", recv, upd)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("timeout waiting for update")
	}
}
