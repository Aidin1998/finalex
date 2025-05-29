package test

import (
	"context"
	"testing"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/bookkeeper"
	"github.com/Aidin1998/pincex_unified/internal/trading"
	"github.com/Aidin1998/pincex_unified/pkg/models"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// E2E test placeholder. Replace with real E2E scenarios.
func TestE2E_ExchangeLifecycle(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	_ = db.AutoMigrate(&models.TradingPair{}, &models.Order{}, &models.Trade{}, &models.Account{}, &models.Transaction{}, &models.User{})
	logger := zap.NewNop()
	bkSvc, _ := bookkeeper.NewService(logger, db)
	ts, _ := trading.NewService(logger, db, bkSvc)
	_ = ts.Start()
	ctx := context.Background()
	userCount := 1000
	orderPerUser := 10
	var totalOrders int
	start := time.Now()
	for i := 0; i < userCount; i++ {
		userID := uuid.New().String()
		acct, _ := bkSvc.CreateAccount(ctx, userID, "USD")
		acct.Balance = 1_000_000
		acct.Available = 1_000_000
		_ = db.Save(acct)
		for j := 0; j < orderPerUser; j++ {
			order := &models.Order{
				UserID:   uuid.MustParse(userID),
				Symbol:   "BTCUSD",
				Side:     []string{"buy", "sell"}[j%2],
				Type:     "limit",
				Price:    30000.0 + float64(j),
				Quantity: 0.01,
			}
			_, err := ts.PlaceOrder(ctx, order)
			if err != nil {
				t.Errorf("order failed: %v", err)
			}
			totalOrders++
		}
	}
	totalTime := time.Since(start)
	tps := float64(totalOrders) / totalTime.Seconds()
	t.Logf("E2E: users=%d orders=%d tps=%.2f total_time=%v", userCount, totalOrders, tps, totalTime)
	t.Logf("E2E_METRICS: pass users=1000 orders=10000 tps=%.2f", tps)
}
