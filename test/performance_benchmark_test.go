//go:build performance
// +build performance

package test

import (
	"context"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/bookkeeper"
	"github.com/Aidin1998/pincex_unified/internal/trading"
	"github.com/Aidin1998/pincex_unified/pkg/models"
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
	userCount := 100
	users := make([]string, userCount)
	for i := 0; i < userCount; i++ {
		userID := uuid.New().String()
		users[i] = userID
		acct, err := svc.(*trading.Service).bookkeeperSvc.CreateAccount(ctx, userID, "USD")
		if err != nil {
			b.Fatalf("failed to create account: %v", err)
		}
		acct.Balance = 1_000_000
		acct.Available = 1_000_000
		_ = svc.(*trading.Service).db.Save(acct)
	}
	pair := &models.TradingPair{Symbol: "USDUSD", BaseCurrency: "USD", QuoteCurrency: "USD", Status: "active"}
	err := svc.(*trading.Service).db.Create(pair).Error
	if err != nil {
		b.Fatalf("failed to create trading pair: %v", err)
	}
	b.ResetTimer()
	var latencies []time.Duration
	var mu sync.Mutex
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			userID := users[time.Now().UnixNano()%int64(userCount)]
			order := &models.Order{
				UserID:   uuid.MustParse(userID),
				Symbol:   "USDUSD",
				Side:     []string{"buy", "sell"}[time.Now().UnixNano()%2],
				Type:     "limit",
				Price:    float64(1 + time.Now().UnixNano()%100),
				Quantity: float64(1 + time.Now().UnixNano()%10),
			}
			start := time.Now()
			_, err := svc.PlaceOrder(ctx, order)
			latency := time.Since(start)
			if err != nil {
				b.Errorf("order placement failed: %v", err)
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
	b.Logf("Order placement latency: min=%v avg=%v max=%v ops=%d", min, avg, max, len(latencies))
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
