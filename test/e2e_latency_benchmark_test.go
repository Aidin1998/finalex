//go:build performance
// +build performance

package test

import (
	"context"
	"testing"
	"time"

	"sort"

	"github.com/Aidin1998/pincex_unified/internal/bookkeeper"
	"github.com/Aidin1998/pincex_unified/internal/marketdata"
	"github.com/Aidin1998/pincex_unified/internal/trading"
	"github.com/Aidin1998/pincex_unified/pkg/models"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func percentile(latencies []time.Duration, p float64) time.Duration {
	if len(latencies) == 0 {
		return 0
	}
	sorted := make([]time.Duration, len(latencies))
	copy(sorted, latencies)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	idx := int(float64(len(sorted))*p + 0.5)
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// BenchmarkEndToEndTransactionLatency measures the time from order submission to market data update and balance update.
func BenchmarkEndToEndTransactionLatency(b *testing.B) {
	// Setup in-memory DB, bookkeeper, trading, and market data services
	logger := zap.NewNop()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		b.Fatalf("failed to open in-memory db: %v", err)
	}
	err = db.AutoMigrate(&models.TradingPair{}, &models.Order{}, &models.Trade{}, &models.Account{}, &models.Transaction{}, &models.User{})
	if err != nil {
		b.Fatalf("failed to migrate db: %v", err)
	}
	bkSvc, err := bookkeeper.NewService(logger, db)
	if err != nil {
		b.Fatalf("failed to create bookkeeper: %v", err)
	}
	tradingSvc, err := trading.NewService(logger, db, bkSvc)
	if err != nil {
		b.Fatalf("failed to create trading service: %v", err)
	}
	err = tradingSvc.Start()
	if err != nil {
		b.Fatalf("failed to start trading service: %v", err)
	}
	mdSrv := marketdata.NewTestServer() // Minimal test server with Subscribe/Publish
	defer mdSrv.Stop()
	// Setup user, account, and trading pair
	userID := uuid.New().String()
	acct, err := bkSvc.CreateAccount(context.Background(), userID, "USD")
	if err != nil {
		b.Fatalf("failed to create account: %v", err)
	}
	acct.Balance = 1_000_000
	acct.Available = 1_000_000
	db.Save(acct)
	pair := &models.TradingPair{Symbol: "BTCUSD", BaseCurrency: "BTC", QuoteCurrency: "USD", Status: "active"}
	db.Create(pair)
	// Subscribe to market data
	ch := mdSrv.Subscribe("BTCUSD")
	b.ResetTimer()
	latencies := make([]time.Duration, 0, b.N)
	for i := 0; i < b.N; i++ {
		order := &models.Order{
			UserID:   uuid.MustParse(userID),
			Symbol:   "BTCUSD",
			Side:     "buy",
			Type:     "limit",
			Price:    30000,
			Quantity: 0.01,
		}
		start := time.Now()
		_, err := tradingSvc.PlaceOrder(context.Background(), order)
		if err != nil {
			b.Fatalf("order placement failed: %v", err)
		}
		// Wait for market data update (simulate real push)
		select {
		case <-ch:
			// Market data received
		case <-time.After(100 * time.Millisecond):
			b.Fatalf("timeout waiting for market data update")
		}
		// Check balance update
		var updatedAcct models.Account
		db.First(&updatedAcct, "id = ?", acct.ID)
		if updatedAcct.Available > acct.Available {
			b.Fatalf("balance not updated")
		}
		latency := time.Since(start)
		latencies = append(latencies, latency)
		b.Logf("End-to-end latency: %v", latency)
	}
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
	p50 := percentile(latencies, 0.50)
	p95 := percentile(latencies, 0.95)
	p99 := percentile(latencies, 0.99)
	tps := float64(len(latencies)) / float64(totalTime.Seconds())
	b.Logf("E2E latency: min=%v avg=%v max=%v p50=%v p95=%v p99=%v ops=%d", min, avg, max, p50, p95, p99, len(latencies))
	b.Logf("TPS: %.2f, Total Time: %v", tps, totalTime)
	b.Logf("SUMMARY: ops=%d tps=%.2f avg_latency_ms=%.2f p95_latency_ms=%.2f", len(latencies), tps, avg.Seconds()*1000, p95.Seconds()*1000)
}
