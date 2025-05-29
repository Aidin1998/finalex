//go:build performance
// +build performance

package test

import (
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/Aidin1998/pincex_unified/pkg/models"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// BenchmarkDatabaseReadWritePerformance measures DB CRUD performance under load.
func BenchmarkDatabaseReadWritePerformance(b *testing.B) {
	logger := zap.NewNop()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		b.Fatalf("failed to open in-memory db: %v", err)
	}
	err = db.AutoMigrate(&models.User{}, &models.Order{}, &models.Trade{}, &models.Account{}, &models.Transaction{})
	if err != nil {
		b.Fatalf("failed to migrate db: %v", err)
	}
	userCount := 100
	users := make([]*models.User, userCount)
	for i := 0; i < userCount; i++ {
		users[i] = &models.User{
			Email:    randomEmail(),
			Username: randomUsername(),
			Password: "password",
		}
		db.Create(users[i])
	}
	b.ResetTimer()
	var latencies []time.Duration
	var mu sync.Mutex
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			start := time.Now()
			// Randomly choose CRUD op
			op := rand.Intn(4)
			switch op {
			case 0: // Create
				order := &models.Order{
					UserID:   users[rand.Intn(userCount)].ID,
					Symbol:   "BTCUSD",
					Side:     []string{"buy", "sell"}[rand.Intn(2)],
					Type:     "limit",
					Price:    float64(30000 + rand.Intn(1000)),
					Quantity: float64(1 + rand.Intn(10)),
				}
				db.Create(order)
			case 1: // Read
				var order models.Order
				db.First(&order, "symbol = ?", "BTCUSD")
			case 2: // Update
				var order models.Order
				db.First(&order, "symbol = ?", "BTCUSD")
				order.Price += 1
				db.Save(&order)
			case 3: // Delete
				var order models.Order
				db.First(&order, "symbol = ?", "BTCUSD")
				db.Delete(&order)
			}
			latency := time.Since(start)
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
	b.Logf("DB CRUD latency: min=%v avg=%v max=%v ops=%d", min, avg, max, len(latencies))
}

func randomEmail() string {
	return randomUsername() + "@example.com"
}

func randomUsername() string {
	return "user" + time.Now().Format("150405") + string(rand.Intn(10000))
}
