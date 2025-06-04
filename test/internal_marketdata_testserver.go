//go:build testhelpers
// +build testhelpers

// TestServer simulates a market data service for benchmarking
package test

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/marketmaking/marketdata/distribution"
)

type TestServer struct {
	sm    *distribution.SubscriptionManager
	dist  *distribution.Distributor
	idSeq uint64
}

func NewTestServer() *TestServer {
	sm := distribution.NewSubscriptionManager()
	d := distribution.NewDistributor(sm, 10*time.Millisecond)
	go d.Run()
	return &TestServer{sm: sm, dist: d}
}

func (ts *TestServer) Stop() {
	ts.dist.Stop()
}

func (ts *TestServer) Subscribe(symbol string) <-chan distribution.Update {
	id := atomic.AddUint64(&ts.idSeq, 1)
	clientID := fmt.Sprintf("client-%d", id)
	sub := &distribution.Subscription{
		ClientID:    clientID,
		Symbol:      symbol,
		PriceLevels: []int{0},
		Frequency:   0,
		Compression: false,
	}
	ts.sm.Subscribe(sub)
	typedCh := make(chan distribution.Update, 1000)
	internalCh := make(chan interface{}, 1000)
	ts.dist.RegisterClient(clientID, internalCh)
	go func() {
		for msg := range internalCh {
			if upd, ok := msg.(distribution.Update); ok {
				typedCh <- upd
			}
		}
	}()
	return typedCh
}
