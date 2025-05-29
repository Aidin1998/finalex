//go:build performance
// +build performance

package test

import (
	"context"
	"fmt"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/bookkeeper"
	"github.com/Aidin1998/pincex_unified/internal/trading"
	"github.com/Aidin1998/pincex_unified/pkg/models"
	"github.com/Aidin1998/pincex_unified/testutil"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func BenchmarkOrderPlacement(b *testing.B) {
	for i := 0; i < b.N; i++ {
		// Simulate order placement (replace with real logic)
		time.Sleep(1 * time.Millisecond)
	}
}

func setupBenchmarkTradingService(b *testing.B) trading.TradingService {
	b.Helper()
	logger := zap.NewNop()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		b.Fatalf("failed to open in-memory db: %v", err)
	}
	err = db.AutoMigrate(&models.TradingPair{}, &models.Order{}, &models.Trade{}, &models.Account{}, &models.Transaction{})
	if err != nil {
		b.Fatalf("failed to migrate db: %v", err)
	}
	bkSvc, err := bookkeeper.NewService(logger, db)
	if err != nil {
		b.Fatalf("failed to create bookkeeper: %v", err)
	}
	svc, err := trading.NewService(logger, db, bkSvc)
	if err != nil {
		b.Fatalf("failed to create trading service: %v", err)
	}
	err = svc.Start()
	if err != nil {
		b.Fatalf("failed to start trading service: %v", err)
	}
	return svc
}

func BenchmarkOrderPlacementThroughput(b *testing.B) {
	svc := setupBenchmarkTradingService(b)
	defer svc.Stop()
	ctx := context.Background()
	// Setup: create users, accounts, and trading pair
	userCount := 10000 // Increased user base for heavy load
	ordersPerUser := 10
	totalOrders := userCount * ordersPerUser
	users := make([]string, userCount)
	for i := 0; i < userCount; i++ {
		users[i] = fmt.Sprintf("user-%d", i)
	}
	// pair := &models.TradingPair{Symbol: "USDUSD", BaseCurrency: "USD", QuoteCurrency: "USD", Status: "active"}
	// err := svc.(*trading.Service).db.Create(pair).Error
	// if err != nil {
	// 	b.Fatalf("failed to create trading pair: %v", err)
	// }
	b.ResetTimer()
	var latencies []time.Duration
	var mu sync.Mutex
	errCount := 0
	start := time.Now()
	wg := sync.WaitGroup{}
	wg.Add(userCount)
	for i := 0; i < userCount; i++ {
		go func(i int) {
			defer wg.Done()
			for j := 0; j < ordersPerUser; j++ {
				order := &models.Order{
					UserID:   uuid.New(),
					Symbol:   "USDUSD",
					Side:     []string{"buy", "sell"}[j%2],
					Type:     "limit",
					Price:    30000.0 + float64(j),
					Quantity: 0.01,
				}
				orderStart := time.Now()
				_, err := svc.PlaceOrder(ctx, order)
				latency := time.Since(orderStart)
				mu.Lock()
				latencies = append(latencies, latency)
				if err != nil {
					errCount++
				}
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()
	totalTime := time.Since(start)
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
	p50 := testutil.Percentile(latencies, 0.50)
	p95 := testutil.Percentile(latencies, 0.95)
	p99 := testutil.Percentile(latencies, 0.99)
	tps := float64(totalOrders) / totalTime.Seconds()
	errorRate := float64(errCount) / float64(totalOrders) * 100
	b.Logf("Order placement latency: min=%v avg=%v max=%v p50=%v p95=%v p99=%v ops=%d", min, avg, max, p50, p95, p99, len(latencies))
	b.Logf("TPS: %.2f, Error Rate: %.2f%%, Total Time: %v", tps, errorRate, totalTime)
	// Output summary for CI/CD and dashboards
	b.Logf("SUMMARY: total_orders=%d tps=%.2f avg_latency_ms=%.2f p95_latency_ms=%.2f error_rate=%.2f%%", totalOrders, tps, avg.Seconds()*1000, p95.Seconds()*1000, errorRate)
}

// Benchmark WebSocket throughput and latency under load
func BenchmarkWebSocketThroughput(b *testing.B) {
	addr := "ws://localhost:8080/ws?protocol=binary"
	clients := 1000 // Simulate 1000 concurrent clients
	conns := make([]*websocket.Conn, 0, clients)
	for i := 0; i < clients; i++ {
		u, _ := url.Parse(addr)
		c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
		if err != nil {
			b.Fatalf("dial: %v", err)
		}
		conns = append(conns, c)
	}
	b.ResetTimer()
	start := time.Now()
	for i := 0; i < b.N; i++ {
		for _, c := range conns {
			_, _, err := c.ReadMessage()
			if err != nil {
				b.Fatalf("read: %v", err)
			}
		}
	}
	b.StopTimer()
	elapsed := time.Since(start)
	b.Logf("Total time: %v", elapsed)
	for _, c := range conns {
		c.Close()
	}
}
