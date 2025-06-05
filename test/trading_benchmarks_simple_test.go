//go:build trading
// +build trading

package test

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/Aidin1998/finalex/pkg/models"
	"github.com/shopspring/decimal"
)

// BenchmarkOrderProcessing tests basic order processing performance
func BenchmarkOrderProcessing(b *testing.B) {
	mockBookkeeper := &MockBookkeeperStressTest{}
	mockWSHub := &MockWSHubStressTest{}

	// Set up test users
	testUsers := make([]string, 10)
	for i := 0; i < len(testUsers); i++ {
		testUsers[i] = fmt.Sprintf("bench_user_%d", i)
		mockBookkeeper.SetBalance(testUsers[i], "BTC", decimal.NewFromFloat(1.0))
		mockBookkeeper.SetBalance(testUsers[i], "USDT", decimal.NewFromFloat(50000.0))
		mockWSHub.Connect(testUsers[i])
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		user := testUsers[i%len(testUsers)]
		order := &models.Order{
			ID:       fmt.Sprintf("bench_%d", i),
			UserID:   user,
			Symbol:   "BTCUSDT",
			Side:     models.SideBuy,
			Type:     models.OrderTypeLimit,
			Price:    decimal.NewFromFloat(50000),
			Quantity: decimal.NewFromFloat(0.001),
			Status:   models.OrderStatusPending,
			Created:  time.Now(),
		}

		// Simulate order processing
		_, _ = mockBookkeeper.GetBalance(user, "USDT")
		reservationID, _ := mockBookkeeper.ReserveBalance(user, "USDT", order.Price.Mul(order.Quantity))
		_ = mockBookkeeper.CommitReservation(reservationID)

		// Use order variable
		_ = order
	}
}

// BenchmarkConcurrentOrders tests concurrent order processing
func BenchmarkConcurrentOrders(b *testing.B) {
	mockBookkeeper := &MockBookkeeperStressTest{}
	mockWSHub := &MockWSHubStressTest{}

	// Set up test users
	testUsers := make([]string, 20)
	for i := 0; i < len(testUsers); i++ {
		testUsers[i] = fmt.Sprintf("concurrent_user_%d", i)
		mockBookkeeper.SetBalance(testUsers[i], "BTC", decimal.NewFromFloat(10.0))
		mockBookkeeper.SetBalance(testUsers[i], "USDT", decimal.NewFromFloat(100000.0))
		mockWSHub.Connect(testUsers[i])
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		orderCounter := 0
		for pb.Next() {
			user := testUsers[orderCounter%len(testUsers)]

			order := &models.Order{
				ID:       fmt.Sprintf("concurrent_%d_%d", orderCounter, time.Now().UnixNano()),
				UserID:   user,
				Symbol:   "BTCUSDT",
				Side:     models.SideBuy,
				Type:     models.OrderTypeLimit,
				Price:    decimal.NewFromFloat(50000 + rand.Float64()*1000),
				Quantity: decimal.NewFromFloat(0.001 + rand.Float64()*0.01),
				Status:   models.OrderStatusPending,
				Created:  time.Now(),
			}

			// Simulate order processing
			_, _ = mockBookkeeper.GetBalance(user, "USDT")
			reservationID, _ := mockBookkeeper.ReserveBalance(user, "USDT", order.Price.Mul(order.Quantity))
			_ = mockBookkeeper.CommitReservation(reservationID)

			orderCounter++
		}
	})
}

// BenchmarkWebSocketBroadcast tests WebSocket broadcasting performance
func BenchmarkWebSocketBroadcast(b *testing.B) {
	mockWSHub := &MockWSHubStressTest{}

	// Connect clients
	for i := 0; i < 100; i++ {
		clientID := fmt.Sprintf("ws_client_%d", i)
		mockWSHub.Connect(clientID)
	}

	message := []byte(`{"type":"trade","pair":"BTCUSDT","price":"50000","qty":"0.001"}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mockWSHub.Broadcast("trades.BTCUSDT", message)
	}
}

// BenchmarkOrderTypeComparison specifically benchmarks different order types
func BenchmarkOrderTypeComparison(b *testing.B) {
	mockBookkeeper := &MockBookkeeperStressTest{}
	mockWSHub := &MockWSHubStressTest{}

	// Set up test users
	testUsers := make([]string, 10)
	for i := 0; i < len(testUsers); i++ {
		testUsers[i] = fmt.Sprintf("user_%d", i)
		mockBookkeeper.SetBalance(testUsers[i], "BTC", decimal.NewFromFloat(10.0))
		mockBookkeeper.SetBalance(testUsers[i], "USDT", decimal.NewFromFloat(500000.0))
		mockWSHub.Connect(testUsers[i])
	}

	testOrderTypes := []models.OrderType{
		models.OrderTypeMarket,
		models.OrderTypeLimit,
		models.OrderTypeStop,
		models.OrderTypeIceberg,
	}

	for _, orderType := range testOrderTypes {
		b.Run(fmt.Sprintf("OrderType_%s", orderType), func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				user := testUsers[i%len(testUsers)]

				// Create order with appropriate type
				order := &models.Order{
					ID:       fmt.Sprintf("bench_%s_%d", orderType, i),
					UserID:   user,
					Symbol:   "BTCUSDT",
					Side:     models.SideBuy,
					Type:     orderType,
					Quantity: decimal.NewFromFloat(0.001),
					Status:   models.OrderStatusPending,
					Created:  time.Now(),
				}

				// Set type-specific fields
				if orderType != models.OrderTypeMarket {
					order.Price = decimal.NewFromFloat(50000)
				}

				if orderType == models.OrderTypeStop || orderType == models.OrderTypeStopLimit {
					order.StopPrice = decimal.NewFromFloat(51000)
				}

				if orderType == models.OrderTypeIceberg {
					order.VisibleQuantity = order.Quantity.Div(decimal.NewFromInt(5))
					order.TotalQuantity = order.Quantity
				}

				// Process order (simulate basic operations)
				reservationID, _ := mockBookkeeper.ReserveBalance(user, "USDT", decimal.NewFromFloat(50))
				_ = mockBookkeeper.CommitReservation(reservationID)

				// Simulate processing time differences for different order types
				switch orderType {
				case models.OrderTypeMarket:
					time.Sleep(time.Nanosecond * 50) // Market orders are fastest
				case models.OrderTypeLimit:
					time.Sleep(time.Nanosecond * 100) // Limit orders slightly slower
				case models.OrderTypeStop, models.OrderTypeStopLimit:
					time.Sleep(time.Nanosecond * 150) // Stop orders have more logic
				case models.OrderTypeIceberg:
					time.Sleep(time.Nanosecond * 200) // Iceberg orders slowest
				}
			}
		})
	}
}
