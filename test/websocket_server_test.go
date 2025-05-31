package transport

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/marketdata/distribution"
	"github.com/gorilla/websocket"
)

func TestWebSocketServer_SubscribeAndReceive(t *testing.T) {
	sm := distribution.NewSubscriptionManager()
	d := distribution.NewDistributor(sm, 50*time.Millisecond)
	go d.Run()
	server := NewWebSocketServer(d, sm)
	ts := httptest.NewServer(server)
	defer ts.Close()

	// Connect
	u := "ws" + ts.URL[len("http"):]
	ws, _, err := websocket.DefaultDialer.Dial(u+"?symbol=SYM", nil)
	if err != nil {
		t.Fatalf("dial error: %v", err)
	}
	defer ws.Close()

	// Wait briefly and publish
	time.Sleep(10 * time.Millisecond)
	upd := distribution.Update{Symbol: "SYM", PriceLevel: 0, Bid: 1.0, Ask: 2.0, Timestamp: time.Now()}
	d.Publish(upd)

	// Read JSON
	var recv distribution.Update
	err = ws.ReadJSON(&recv)
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	if recv.Bid != upd.Bid || recv.Ask != upd.Ask {
		t.Fatalf("mismatch: got %v, want %v", recv, upd)
	}
}
