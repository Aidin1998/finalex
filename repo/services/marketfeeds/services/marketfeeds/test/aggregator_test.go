package test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"services/marketfeeds/aggregator"
	"services/marketfeeds/models"

	"github.com/gorilla/websocket"
)

// Aggregator is a minimal stub for testing purposes.
type Aggregator struct {
	WSURL string
	Pub   Publisher
	DB    *aggregator.DBWriter
}

func (a *Aggregator) Run() {
	pairs := []string{
		"btc/usdt", "eth/usdt", "bnb/usdt", "sol/usdt", "xrp/usdt",
		"doge/usdt", "ada/usdt", "sui/usdt", "link/usdt", "trx/usdt",
	}
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	end := time.Now().Add(20 * time.Second)
	for time.Now().Before(end) {
		<-ticker.C
		for _, pair := range pairs {
			avg, _, err := aggregator.FetchAverageAcrossExchanges(pair)
			if err == nil && a.DB != nil {
				a.DB.InsertPrice("average", pair, avg, time.Now())
			}
		}
	}
}

// New creates a new Aggregator instance.
func New(wsURL, arg1, arg2, arg3 string, pub Publisher) *Aggregator {
	// Optionally connect to DB if env var is set
	var dbw *aggregator.DBWriter
	if dsn := os.Getenv("MARKETFEEDS_DB_DSN"); dsn != "" {
		var err error
		dbw, err = aggregator.NewDBWriter(dsn)
		if err != nil {
			fmt.Printf("[DB] Failed to connect: %v\n", err)
		} else {
			fmt.Println("[DB] Connected to CockroachDB!")
			err = dbw.Migrate()
			if err != nil {
				fmt.Printf("[DB] Migration error: %v\n", err)
			} else {
				fmt.Println("[DB] Migration successful (table ready)")
			}
		}
	} else {
		fmt.Println("[DB] MARKETFEEDS_DB_DSN not set, DB logging disabled.")
	}
	return &Aggregator{
		WSURL: wsURL,
		Pub:   pub,
		DB:    dbw,
	}
}

// mockPub captures published ticks and signals when it reaches a target count.
type mockPub struct {
	ticks    []models.Tick
	target   int
	signalCh chan struct{}
}

func newMockPub(target int) *mockPub {
	return &mockPub{signalCh: make(chan struct{}), target: target}
}

func (m *mockPub) Publish(topic string, msg interface{}) {
	if t, ok := msg.(models.Tick); ok {
		m.ticks = append(m.ticks, t)
		// Print price feed output for inspection
		println("[PriceFeed]", t.Symbol, t.Price, t.Timestamp.Format(time.RFC3339Nano))
		if len(m.ticks) >= m.target {
			select {
			case m.signalCh <- struct{}{}:
			default:
			}
		}
	}
}

// Publisher is an interface for publishing messages.
type Publisher interface {
	Publish(topic string, msg interface{})
}

// Test that Aggregator publishes exactly 100 messages across 10 pairs.
func TestAggregatorPublishes100Messages(t *testing.T) {
	total := 200 // 20 seconds * 10 pairs (1 avg per second per pair)

	pub := newMockPub(total)
	ag := New("", "", "", "", pub)
	go ag.Run()

	// Wait for all 200 averages or timeout
	select {
	case <-pub.signalCh:
	case <-time.After(22 * time.Second):
		t.Fatal("timeout waiting for published averages")
	}

	// Assert count and contents
	if len(pub.ticks) != total {
		t.Fatalf("got %d averages, want %d", len(pub.ticks), total)
	}
	// Print only the first 2 averages per pair for inspection
	pairCounts := make(map[string]int)
	for _, tk := range pub.ticks {
		if pairCounts[tk.Symbol] < 2 {
			t.Logf("%s: %.6f (average) at %s", tk.Symbol, tk.Price, tk.Timestamp.Format(time.RFC3339Nano))
			pairCounts[tk.Symbol]++
		}
	}
}

// Test that Aggregator recovers from a malformed frame by reconnecting.
func TestAggregatorRecoversAfterMalformedFrame(t *testing.T) {
	upgrader := websocket.Upgrader{}
	connCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connCount++
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()

		// Read subscribe
		var sub interface{}
		_ = c.ReadJSON(&sub)

		if connCount == 1 {
			// Send malformed JSON then close
			c.WriteMessage(websocket.TextMessage, []byte("{bad json"))
			return
		}
		// On second connection send one valid tick
		tick := models.Tick{Symbol: "btc/usdt", Price: 50000, Timestamp: time.Now()}
		if err := c.WriteJSON(tick); err != nil {
			return
		}
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	pub := newMockPub(1)
	ag := New(wsURL, "", "", "", pub)
	go ag.Run()

	// Wait for recovery tick
	select {
	case <-pub.signalCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for recovery tick")
	}

	if len(pub.ticks) != 1 {
		t.Fatalf("got %d ticks after recovery, want 1", len(pub.ticks))
	}
	if pub.ticks[0].Symbol != "btc/usdt" {
		t.Errorf("recovered tick symbol = %q, want %q", pub.ticks[0].Symbol, "btc/usdt")
	}
	if connCount < 2 {
		t.Errorf("expected at least 2 connections (retry), got %d", connCount)
	}
}

// Test that Aggregator reconnects after server closes connection mid‐stream.
func TestAggregatorReconnectsAfterClose(t *testing.T) {
	target := 10
	sentFirst := 5
	sentSecond := 5
	connCount := 0

	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connCount++
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()

		// consume subscribe
		var sub interface{}
		_ = c.ReadJSON(&sub)

		// send partial then close
		sendCount := sentFirst
		if connCount > 1 {
			sendCount = sentSecond
		}
		for i := 0; i < sendCount; i++ {
			tick := models.Tick{
				Symbol:    "btc/usdt",
				Price:     float64(i + (connCount-1)*sentFirst + 1),
				Timestamp: time.Now(),
			}
			if err := c.WriteJSON(tick); err != nil {
				return
			}
		}
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	pub := newMockPub(target)
	ag := New(wsURL, "", "", "", pub)
	go ag.Run()

	select {
	case <-pub.signalCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for reconnect ticks")
	}

	if len(pub.ticks) != target {
		t.Fatalf("got %d ticks, want %d after reconnects", len(pub.ticks), target)
	}
}

// Test that Aggregator preserves decimal precision of incoming prices.
func TestAggregatorPublishesDecimalPrecision(t *testing.T) {
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		// consume subscribe
		var sub interface{}
		_ = c.ReadJSON(&sub)
		// send one high‐precision tick
		orig := models.Tick{
			Symbol:    "eth/usdt",
			Price:     1234.567890123,
			Timestamp: time.Unix(1_600_000_000, 0),
		}
		if err := c.WriteJSON(orig); err != nil {
			t.Fatal(err)
		}
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	pub := newMockPub(1)
	ag := New(wsURL, "", "", "", pub)
	go ag.Run()

	select {
	case <-pub.signalCh:
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for precision tick")
	}

	if len(pub.ticks) != 1 {
		t.Fatalf("got %d ticks, want 1", len(pub.ticks))
	}
	got := pub.ticks[0]
	if got.Price != 1234.567890123 {
		t.Errorf("price = %v, want %v", got.Price, 1234.567890123)
	}
	if got.Symbol != "eth/usdt" {
		t.Errorf("symbol = %q, want %q", got.Symbol, "eth/usdt")
	}
}
