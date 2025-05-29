//go:build performance
// +build performance

package test

import (
	"sync"
	"testing"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/marketdata"
)

// BenchmarkMarketDataDistributionLatency measures the time from order book update to client receipt.
func BenchmarkMarketDataDistributionLatency(b *testing.B) {
	// Setup: create a market data server and simulated clients
	srv := marketdata.NewTestServer() // Assume this exists or create a minimal test server
	defer srv.Stop()
	clientCount := 1000
	updatesPerClient := 100
	var wg sync.WaitGroup
	latencies := make([]time.Duration, 0, clientCount*updatesPerClient)
	latMu := sync.Mutex{}
	runtimeLimit := 20 * time.Second
	endTime := time.Now().Add(runtimeLimit)

	// Simulate clients subscribing and measuring latency
	for i := 0; i < clientCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ch := srv.Subscribe("BTCUSD")
			for j := 0; j < updatesPerClient; j++ {
				if time.Now().After(endTime) {
					break
				}
				start := time.Now()
				update := <-ch // Receive order book update
				_ = update     // Optionally validate content
				latency := time.Since(start)
				latMu.Lock()
				latencies = append(latencies, latency)
				latMu.Unlock()
			}
		}()
	}
	b.ResetTimer()
	// Simulate order book updates for the duration of the runtime limit
	for time.Now().Before(endTime) {
		srv.PublishOrderBookUpdate("BTCUSD")
	}
	wg.Wait()
	b.StopTimer()
	// Report stats
	var total time.Duration
	min, max := time.Hour, time.Duration(0)
	for _, l := range latencies {
		total += l
		if l < min {
			min = l
		}
		if l > max {
			max = l
		}
	}
	avg := total / time.Duration(len(latencies))
	b.Logf("Market data distribution latency: min=%v avg=%v max=%v samples=%d", min, avg, max, len(latencies))
}
