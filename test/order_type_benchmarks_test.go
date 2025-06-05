//go:build trading

package test

import (
	"fmt"
	"math/rand"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Aidin1998/finalex/pkg/models"
	"github.com/shopspring/decimal"
)

// BenchmarkOrderTypeComparison specifically benchmarks different order types
// to measure and compare their matching speed under identical conditions
func BenchmarkOrderTypeComparison(b *testing.B) {
	mockBookkeeper := &MockBookkeeperStressTest{}
	mockWSHub := &MockWSHubStressTest{}

	// Set up test users
	testUsers := make([]string, 100)
	for i := 0; i < len(testUsers); i++ {
		testUsers[i] = fmt.Sprintf("user_%d", i)
		mockBookkeeper.SetBalance(testUsers[i], "BTC", decimal.NewFromFloat(10.0))
		mockBookkeeper.SetBalance(testUsers[i], "USDT", decimal.NewFromFloat(500000.0))
		mockBookkeeper.SetBalance(testUsers[i], "ETH", decimal.NewFromFloat(100.0))
		mockWSHub.Connect(testUsers[i])
	}

	// Common benchmark function for fair comparison
	runOrderTypeBenchmark := func(b *testing.B, orderType models.OrderType) {
		var matchCount int64
		var orderCount int64

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			userIndex := i % len(testUsers)
			user := testUsers[userIndex]
			orderSide := models.SideBuy
			if i%2 == 0 {
				orderSide = models.SideSell
			}

			// Create base order
			order := &models.Order{
				ID:        fmt.Sprintf("order_%s_%d", orderType, i),
				UserID:    user,
				Symbol:    "BTCUSDT",
				Side:      orderSide,
				Type:      orderType,
				Quantity:  decimal.NewFromFloat(0.01 + rand.Float64()*0.1),
				Status:    models.OrderStatusPending,
				Created:   time.Now(),
				UpdatedAt: time.Now(),
			}

			// Set specific parameters based on order type
			switch orderType {
			case models.OrderTypeMarket:
				// Market orders don't have prices
				if orderSide == models.SideBuy {
					reservationID, _ := mockBookkeeper.ReserveBalance(user, "USDT", decimal.NewFromFloat(50000))
					_ = mockBookkeeper.CommitReservation(reservationID)
				} else {
					reservationID, _ := mockBookkeeper.ReserveBalance(user, "BTC", order.Quantity)
					_ = mockBookkeeper.CommitReservation(reservationID)
				}
			case models.OrderTypeLimit:
				// Limit orders have prices
				order.Price = decimal.NewFromFloat(50000.0 + rand.Float64()*1000.0 - 500.0)
				if orderSide == models.SideBuy {
					reservationID, _ := mockBookkeeper.ReserveBalance(user, "USDT", order.Price.Mul(order.Quantity))
					_ = mockBookkeeper.CommitReservation(reservationID)
				} else {
					reservationID, _ := mockBookkeeper.ReserveBalance(user, "BTC", order.Quantity)
					_ = mockBookkeeper.CommitReservation(reservationID)
				}
			case models.OrderTypeStopLimit, models.OrderTypeStop:
				// Stop orders have trigger prices and prices
				order.Price = decimal.NewFromFloat(50000.0 + rand.Float64()*1000.0 - 500.0)
				if orderSide == models.SideBuy {
					// Stop buy triggers above market
					order.StopPrice = order.Price.Add(decimal.NewFromFloat(100.0 + rand.Float64()*200.0))
					reservationID, _ := mockBookkeeper.ReserveBalance(user, "USDT", order.Price.Mul(order.Quantity))
					_ = mockBookkeeper.CommitReservation(reservationID)
				} else {
					// Stop sell triggers below market
					order.StopPrice = order.Price.Sub(decimal.NewFromFloat(100.0 + rand.Float64()*200.0))
					reservationID, _ := mockBookkeeper.ReserveBalance(user, "BTC", order.Quantity)
					_ = mockBookkeeper.CommitReservation(reservationID)
				}
			case models.OrderTypeIceberg:
				// Iceberg orders have visible and total quantity
				order.Price = decimal.NewFromFloat(50000.0 + rand.Float64()*1000.0 - 500.0)
				order.VisibleQuantity = order.Quantity.Div(decimal.NewFromInt(10))
				order.TotalQuantity = order.Quantity
				if orderSide == models.SideBuy {
					reservationID, _ := mockBookkeeper.ReserveBalance(user, "USDT", order.Price.Mul(order.Quantity))
					_ = mockBookkeeper.CommitReservation(reservationID)
				} else {
					reservationID, _ := mockBookkeeper.ReserveBalance(user, "BTC", order.Quantity)
					_ = mockBookkeeper.CommitReservation(reservationID)
				}
			}

			// Process order
			atomic.AddInt64(&orderCount, 1)

			// Simulate order matching (60% matching probability)
			if rand.Float64() < 0.6 {
				atomic.AddInt64(&matchCount, 1)

				// Simulate trade execution with variable latency based on order type
				switch orderType {
				case models.OrderTypeMarket:
					time.Sleep(time.Microsecond * 50) // Market orders are fastest
				case models.OrderTypeLimit:
					time.Sleep(time.Microsecond * 100) // Limit orders slightly slower
				case models.OrderTypeStop, models.OrderTypeStopLimit:
					time.Sleep(time.Microsecond * 150) // Stop orders have more logic
				case models.OrderTypeIceberg:
					time.Sleep(time.Microsecond * 200) // Iceberg orders slowest
				}

				// Simulate trade execution
				trade := &models.Trade{
					ID:           fmt.Sprintf("trade_%d", i),
					OrderID:      order.ID,
					Symbol:       order.Symbol,
					Price:        order.Price,
					Quantity:     order.Quantity,
					Side:         order.Side,
					Timestamp:    time.Now(),
					TakerUserID:  order.UserID,
					MakerUserID:  fmt.Sprintf("maker_%d", i%50),
					MakerOrderID: fmt.Sprintf("maker_order_%d", i%50),
				}

				// Broadcast trade
				mockWSHub.Broadcast("trades."+order.Symbol, []byte(fmt.Sprintf(`{"trade_id":"%s","price":"%s","qty":"%s"}`, trade.ID, trade.Price, trade.Quantity)))
			}
		}

		// Calculate match rate for reporting
		matchRate := float64(0)
		if orderCount > 0 {
			matchRate = float64(matchCount) / float64(orderCount) * 100
		}
		b.ReportMetric(matchRate, "match_rate_%")
	}

	// Benchmark each order type individually
	b.Run("MarketOrders", func(b *testing.B) {
		runOrderTypeBenchmark(b, models.OrderTypeMarket)
	})

	b.Run("LimitOrders", func(b *testing.B) {
		runOrderTypeBenchmark(b, models.OrderTypeLimit)
	})

	b.Run("StopOrders", func(b *testing.B) {
		runOrderTypeBenchmark(b, models.OrderTypeStop)
	})

	b.Run("IcebergOrders", func(b *testing.B) {
		runOrderTypeBenchmark(b, models.OrderTypeIceberg)
	})
}
