//go:build ignore
// +build ignore

// testserver.go: moved to distribution package
package marketdata

import (
	"fmt"
	"math/rand"
	"sync/atomic"
	"time"

	"github.com/Aidin1998/finalex/internal/marketmaking/marketdata/distribution"
)

// TestServer simulates a market data service for benchmarking
type TestServer struct {
	sm    *distribution.SubscriptionManager
	dist  *distribution.Distributor
	idSeq uint64
}

// NewTestServer creates and starts a test server
func NewTestServer() *TestServer {
	sm := distribution.NewSubscriptionManager()
	d := distribution.NewDistributor(sm, 10*time.Millisecond)
	// start distributor
	go d.Run()
	return &TestServer{sm: sm, dist: d}
}

// Stop stops the distributor
func (ts *TestServer) Stop() {
	ts.dist.Stop()
}

// Subscribe registers a client for a symbol and returns a channel of Update
func (ts *TestServer) Subscribe(symbol string) <-chan distribution.Update {
	// generate unique client ID
	id := atomic.AddUint64(&ts.idSeq, 1)
	clientID := fmt.Sprintf("client-%d", id)
	// default subscription: price level 0, no compression
	sub := &distribution.Subscription{
		ClientID:    clientID,
		Symbol:      symbol,
		PriceLevels: []int{0},
		Frequency:   0,
		Compression: false,
	}
	ts.sm.Subscribe(sub)
	// create typed channel and register internal channel
	typedCh := make(chan distribution.Update, 1000)
	// internal channel
	internalCh := make(chan interface{}, 1000)
	ts.dist.RegisterClient(clientID, internalCh)
	// forward messages
	go func() {
		for msg := range internalCh {
			if upd, ok := msg.(distribution.Update); ok {
				typedCh <- upd
			}
		}
		close(typedCh)
	}()
	return typedCh
}

// PublishOrderBookUpdate simulates an order book update for a symbol
func (ts *TestServer) PublishOrderBookUpdate(symbol string) {
	// produce random update at price level 0
	upd := distribution.Update{
		Symbol:     symbol,
		PriceLevel: 0,
		Bid:        rand.Float64() * 100,
		Ask:        rand.Float64() * 100,
		Timestamp:  time.Now(),
	}
	ts.dist.Publish(upd)
}
