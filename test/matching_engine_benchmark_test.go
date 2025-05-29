//go:build performance
// +build performance

package test

import (
	"context"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/trading/engine"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// BenchmarkMatchingEngineLatency measures the latency of the matching engine's ProcessOrder method under high concurrency.
func BenchmarkMatchingEngineLatency(b *testing.B) {
	logger := zap.NewNop().Sugar()
	me := engine.NewMatchingEngine(nil, nil, logger, nil, nil)
	pair := "BTCUSD"
	userCount := 100
	users := make([]uuid.UUID, userCount)
	for i := 0; i < userCount; i++ {
		users[i] = uuid.New()
	}
	b.ResetTimer()
	var latencies []time.Duration
	var mu sync.Mutex
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			order := &engine.Order{
				ID:        uuid.New(),
				UserID:    users[rand.Intn(userCount)],
				Pair:      pair,
				Side:      []string{"buy", "sell"}[rand.Intn(2)],
				Type:      "limit",
				Price:     decimal.NewFromInt(30000 + int64(rand.Intn(1000))),
				Quantity:  decimal.NewFromFloat(0.01 + rand.Float64()),
				Status:    "open",
				CreatedAt: time.Now(),
			}
			start := time.Now()
			_, _, _, err := me.ProcessOrder(context.Background(), order, "benchmark")
			latency := time.Since(start)
			if err != nil {
				b.Errorf("matching engine order failed: %v", err)
			}
			mu.Lock()
			latencies = append(latencies, latency)
			mu.Unlock()
		}
	})
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
	b.Logf("Matching engine latency: min=%v avg=%v max=%v ops=%d", min, avg, max, len(latencies))
}
