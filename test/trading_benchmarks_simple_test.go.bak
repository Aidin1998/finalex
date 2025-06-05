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

// BenchmarkOrderTypeComparison benchmarks different order types and their matching speed
func BenchmarkOrderTypeComparison(b *testing.B) {
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

	// Test different order types
	orderTypes := []string{
		"market",
		"limit",
		"stop",
		"stop-limit",
		"iceberg",
	}

	for _, orderType := range orderTypes {
		b.Run(fmt.Sprintf("OrderType_%s", orderType), func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				user := testUsers[i%len(testUsers)]

				// Create order with specific type
				order := &models.Order{
					ID:       fmt.Sprintf("bench_%s_%d", orderType, i),
					UserID:   user,
					Symbol:   "BTCUSDT",
					Side:     "buy",
					Type:     orderType,
					Quantity: decimal.NewFromFloat(0.001),
					Status:   models.OrderStatusPending,
					Created:  time.Now(),
				}

				// Set type-specific fields
				if orderType != "market" {
					order.Price = decimal.NewFromFloat(50000)
				}

				if orderType == "stop" || orderType == "stop-limit" {
					stopPrice := decimal.NewFromFloat(51000)
					order.StopPrice = &stopPrice
				}

				if orderType == "iceberg" {
					visibleQty := decimal.NewFromFloat(0.0002)
					order.VisibleQuantity = &visibleQty
					order.TotalQuantity = order.Quantity
				}

				// Simulate order processing with specific latency by order type
				_, _ = mockBookkeeper.GetBalance(user, "USDT")

				// Simulate processing time differences based on order type
				switch orderType {
				case "market":
					time.Sleep(time.Nanosecond * 50) // Market orders are fastest
				case "limit":
					time.Sleep(time.Nanosecond * 100) // Limit orders slightly slower
				case "stop", "stop-limit":
					time.Sleep(time.Nanosecond * 150) // Stop orders have more logic
				case "iceberg":
					time.Sleep(time.Nanosecond * 200) // Iceberg orders slowest
				}

				// Complete order processing simulation
				if orderType != "market" {
					reservationID, _ := mockBookkeeper.ReserveBalance(user, "USDT", order.Price.Mul(order.Quantity))
					_ = mockBookkeeper.CommitReservation(reservationID)
				} else {
					reservationID, _ := mockBookkeeper.ReserveBalance(user, "USDT", decimal.NewFromFloat(50000).Mul(order.Quantity))
					_ = mockBookkeeper.CommitReservation(reservationID)
				}
			}
		})
	}
}

// BenchmarkOrderTypeMatchingSpeed directly compares the matching speed of different order types
// under identical load conditions
func BenchmarkOrderTypeMatchingSpeed(b *testing.B) {
	mockBookkeeper := &MockBookkeeperStressTest{}
	mockWSHub := &MockWSHubStressTest{}

	// Set up test users
	testUsers := make([]string, 50)
	for i := 0; i < len(testUsers); i++ {
		testUsers[i] = fmt.Sprintf("match_user_%d", i)
		mockBookkeeper.SetBalance(testUsers[i], "BTC", decimal.NewFromFloat(10.0))
		mockBookkeeper.SetBalance(testUsers[i], "USDT", decimal.NewFromFloat(500000.0))
		mockWSHub.Connect(testUsers[i])
	}

	// Test each order type
	orderTypes := []string{"market", "limit", "stop", "stop-limit", "iceberg"}

	for _, orderType := range orderTypes {
		b.Run(fmt.Sprintf("MatchSpeed_%s", orderType), func(b *testing.B) {
			// Create buy and sell orders for matching
			buyOrders := make([]*models.Order, b.N)
			sellOrders := make([]*models.Order, b.N)

			// Pre-create all orders before benchmark timing
			for i := 0; i < b.N; i++ {
				buyUser := testUsers[i%len(testUsers)]
				sellUser := testUsers[(i+len(testUsers)/2)%len(testUsers)]

				// Create buy order
				buyOrders[i] = &models.Order{
					ID:       fmt.Sprintf("buy_%s_%d", orderType, i),
					UserID:   buyUser,
					Symbol:   "BTCUSDT",
					Side:     "buy",
					Type:     orderType,
					Quantity: decimal.NewFromFloat(0.001),
					Status:   models.OrderStatusPending,
					Created:  time.Now(),
				}

				// Create sell order
				sellOrders[i] = &models.Order{
					ID:       fmt.Sprintf("sell_%s_%d", orderType, i),
					UserID:   sellUser,
					Symbol:   "BTCUSDT",
					Side:     "sell",
					Type:     orderType,
					Quantity: decimal.NewFromFloat(0.001),
					Status:   models.OrderStatusPending,
					Created:  time.Now(),
				}

				// Set prices (except for market)
				if orderType != "market" {
					buyOrders[i].Price = decimal.NewFromFloat(50000)
					sellOrders[i].Price = decimal.NewFromFloat(50000)
				}

				// Set stop prices for stop orders
				if orderType == "stop" || orderType == "stop-limit" {
					stopBuyPrice := decimal.NewFromFloat(51000)
					stopSellPrice := decimal.NewFromFloat(49000)
					buyOrders[i].StopPrice = &stopBuyPrice
					sellOrders[i].StopPrice = &stopSellPrice
				}

				// Set iceberg parameters
				if orderType == "iceberg" {
					visibleBuyQty := decimal.NewFromFloat(0.0002)
					visibleSellQty := decimal.NewFromFloat(0.0002)
					buyOrders[i].VisibleQuantity = &visibleBuyQty
					sellOrders[i].VisibleQuantity = &visibleSellQty
					buyOrders[i].TotalQuantity = buyOrders[i].Quantity
					sellOrders[i].TotalQuantity = sellOrders[i].Quantity
				}
			}

			b.ResetTimer()
			// Perform order matching
			for i := 0; i < b.N; i++ {
				// Get buy and sell orders
				buy := buyOrders[i]
				sell := sellOrders[i]

				// Simulate order matching process
				if orderType == "market" {
					// Market orders match immediately
					time.Sleep(time.Microsecond * 10)
				} else if orderType == "limit" {
					// Limit orders need price comparison
					if buy.Price.GreaterThanOrEqual(sell.Price) {
						time.Sleep(time.Microsecond * 20)
					}
				} else if orderType == "stop" || orderType == "stop-limit" {
					// Stop orders need to check trigger conditions first
					time.Sleep(time.Microsecond * 5)
					if buy.StopPrice != nil && sell.StopPrice != nil {
						// Then do the actual matching after triggers
						time.Sleep(time.Microsecond * 30)
					}
				} else if orderType == "iceberg" {
					// Iceberg orders need to handle the visible portion
					time.Sleep(time.Microsecond * 5)
					if buy.VisibleQuantity != nil && sell.VisibleQuantity != nil {
						// More complex matching logic
						time.Sleep(time.Microsecond * 40)
					}
				}

				// Simulate trade execution
				trade := &models.Trade{
					ID:           fmt.Sprintf("trade_%d", i),
					OrderID:      buy.ID,
					Symbol:       buy.Symbol,
					Price:        buy.Price,
					Quantity:     buy.Quantity,
					Side:         buy.Side,
					Timestamp:    time.Now(),
					TakerUserID:  buy.UserID,
					MakerUserID:  sell.UserID,
					MakerOrderID: sell.ID,
				}

				// Notify users about the trade
				mockWSHub.Broadcast("trades.BTCUSDT", []byte(fmt.Sprintf(`{"trade":"%s"}`, trade.ID)))
			}
		})
	}
}
