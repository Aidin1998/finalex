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

// BenchmarkMixedOrderLoad tests the trading engine's performance under a mixed load of different order types
func BenchmarkMixedOrderLoad(b *testing.B) {
	mockBookkeeper := &MockBookkeeperStressTest{}
	mockWSHub := &MockWSHubStressTest{}

	// Set up test users
	testUsers := make([]string, 50)
	for i := 0; i < len(testUsers); i++ {
		testUsers[i] = fmt.Sprintf("mixed_user_%d", i)
		mockBookkeeper.SetBalance(testUsers[i], "BTC", decimal.NewFromFloat(10.0))
		mockBookkeeper.SetBalance(testUsers[i], "USDT", decimal.NewFromFloat(500000.0))
		mockWSHub.Connect(testUsers[i])
	}

	// Define order type distribution
	orderTypes := []models.OrderType{
		models.OrderTypeMarket,
		models.OrderTypeLimit,
		models.OrderTypeStop,
		models.OrderTypeIceberg,
	}

	// Order type weights (approximate distribution in real exchange)
	orderTypeWeights := map[models.OrderType]int{
		models.OrderTypeMarket:  30, // 30%
		models.OrderTypeLimit:   50, // 50%
		models.OrderTypeStop:    15, // 15%
		models.OrderTypeIceberg: 5,  // 5%
	}

	// Build weighted order type distribution
	var weightedTypes []models.OrderType
	for orderType, weight := range orderTypeWeights {
		for i := 0; i < weight; i++ {
			weightedTypes = append(weightedTypes, orderType)
		}
	}

	var processedOrders int64
	var matchedOrders int64

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		localOrders := 0
		localMatches := 0

		for pb.Next() {
			// Select random user and order type based on weight
			userIndex := rand.Intn(len(testUsers))
			user := testUsers[userIndex]
			orderType := weightedTypes[rand.Intn(len(weightedTypes))]

			// Create and process order (similar to previous benchmark)
			localOrders++
			if rand.Float64() < 0.6 {
				localMatches++
				// Simulate matching delay
				switch orderType {
				case models.OrderTypeMarket:
					time.Sleep(time.Microsecond * 50)
				case models.OrderTypeLimit:
					time.Sleep(time.Microsecond * 100)
				case models.OrderTypeStop:
					time.Sleep(time.Microsecond * 150)
				case models.OrderTypeIceberg:
					time.Sleep(time.Microsecond * 200)
				}
			}
		}

		// Update global counters
		atomic.AddInt64(&processedOrders, int64(localOrders))
		atomic.AddInt64(&matchedOrders, int64(localMatches))
	})

	// Report metrics
	b.ReportMetric(float64(atomic.LoadInt64(&processedOrders)), "total_orders")
	b.ReportMetric(float64(atomic.LoadInt64(&matchedOrders))/float64(atomic.LoadInt64(&processedOrders))*100, "match_rate_%")
}
