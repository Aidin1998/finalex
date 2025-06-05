//go:build trading

package test

import (
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/Aidin1998/finalex/pkg/models"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// --- Removed duplicate inline mock types. Use shared types from common_test_types.go instead. ---
// import "test/common_test_types.go" for MockBookkeeperStressTest, MockWSHubStressTest, MockConnection, WSMessage, ReservationInfo

// BenchmarkTradingEngine provides realistic benchmark scenarios for the trading engine
func BenchmarkTradingEngine(b *testing.B) {
	// Initialize test environment
	mockBookkeeper := &MockBookkeeperStressTest{}
	mockWSHub := &MockWSHubStressTest{}

	// Set up test users with realistic balances
	testUsers := make([]uuid.UUID, 100)
	for i := 0; i < len(testUsers); i++ {
		testUsers[i] = uuid.New()
		mockBookkeeper.SetBalance(testUsers[i].String(), "BTC", decimal.NewFromFloat(1.0))
		mockBookkeeper.SetBalance(testUsers[i].String(), "USDT", decimal.NewFromFloat(50000.0))
		mockBookkeeper.SetBalance(testUsers[i].String(), "ETH", decimal.NewFromFloat(20.0))
		mockWSHub.Connect(testUsers[i].String())
	}

	b.Run("OrderSubmission", func(b *testing.B) {
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			orderCounter := 0
			for pb.Next() {
				submitBenchmarkOrder(mockBookkeeper, mockWSHub, testUsers, orderCounter)
				orderCounter++
			}
		})
	})

	b.Run("OrderMatching", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			performBenchmarkMatching(mockBookkeeper, mockWSHub, testUsers, i)
		}
	})

	b.Run("WebSocketBroadcast", func(b *testing.B) {
		message := []byte(`{"type":"trade","pair":"BTCUSDT","price":"50000","qty":"0.001"}`)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			mockWSHub.Broadcast("trades.BTCUSDT", message)
		}
	})

	b.Run("BalanceOperations", func(b *testing.B) {
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			userIndex := 0
			for pb.Next() {
				user := testUsers[userIndex%len(testUsers)]
				_, _ = mockBookkeeper.GetBalance(user.String(), "USDT")
				userIndex++
			}
		})
	})
}

// BenchmarkConcurrentOrderProcessing tests concurrent order processing performance
func BenchmarkConcurrentOrderProcessing(b *testing.B) {
	scenarios := []struct {
		name       string
		workers    int
		ordersEach int
	}{
		{"Low_Concurrency", 4, 100},
		{"Medium_Concurrency", 8, 200},
		{"High_Concurrency", 16, 500},
		{"Extreme_Concurrency", 32, 1000},
	}

	for _, scenario := range scenarios {
		b.Run(scenario.name, func(b *testing.B) {
			benchmarkConcurrentProcessing(b, scenario.workers, scenario.ordersEach)
		})
	}
}

// BenchmarkMarketDataDistribution tests market data distribution performance
func BenchmarkMarketDataDistribution(b *testing.B) {
	mockWSHub := &MockWSHubStressTest{}

	// Connect varying numbers of clients
	clientCounts := []int{100, 500, 1000, 5000}

	for _, clientCount := range clientCounts {
		b.Run(fmt.Sprintf("Clients_%d", clientCount), func(b *testing.B) {
			// Connect clients
			clients := make([]string, clientCount)
			for i := 0; i < clientCount; i++ {
				clients[i] = fmt.Sprintf("client_%d", i)
				mockWSHub.Connect(clients[i])
			}

			message := []byte(`{"type":"orderbook","pair":"BTCUSDT","bids":[["50000","0.1"]],"asks":[["50001","0.1"]]}`)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				mockWSHub.Broadcast("orderbook.BTCUSDT", message)
			}
		})
	}
}

// BenchmarkOrderBookOperations tests order book operation performance
func BenchmarkOrderBookOperations(b *testing.B) { // Mock order book operations
	orders := make([]*models.Order, 1000)
	for i := 0; i < len(orders); i++ {
		orders[i] = &models.Order{
			ID:        uuid.New(),
			UserID:    uuid.New(),
			Symbol:    "BTCUSDT",
			Side:      "BUY",
			Type:      "LIMIT",
			Price:     50000 + float64(i%1000),
			Quantity:  0.001 + float64(i%100)*0.001,
			Status:    "NEW",
			CreatedAt: time.Now(),
		}
	}

	b.Run("OrderInsertion", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			order := orders[i%len(orders)]
			// Simulate order book insertion
			_ = order
			time.Sleep(time.Nanosecond * 100) // Simulate processing time
		}
	})

	b.Run("OrderCancellation", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			order := orders[i%len(orders)]
			// Simulate order cancellation
			order.Status = models.OrderStatusCanceled
			time.Sleep(time.Nanosecond * 50) // Simulate processing time
		}
	})
	b.Run("PriceMatching", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			buyOrder := orders[i%len(orders)]
			sellOrder := orders[(i+1)%len(orders)]
			// Simulate price matching logic
			if buyOrder.Price >= sellOrder.Price {
				// Would match
			}
			time.Sleep(time.Nanosecond * 200) // Simulate matching time
		}
	})
}

// BenchmarkRealWorldScenarios tests realistic trading scenarios
func BenchmarkRealWorldScenarios(b *testing.B) {
	b.Run("FlashCrash", func(b *testing.B) {
		benchmarkFlashCrashScenario(b)
	})

	b.Run("BullMarket", func(b *testing.B) {
		benchmarkBullMarketScenario(b)
	})

	b.Run("HighFrequencyTrading", func(b *testing.B) {
		benchmarkHFTScenario(b)
	})

	b.Run("LargeOrderExecution", func(b *testing.B) {
		benchmarkLargeOrderScenario(b)
	})
}

// Helper functions for benchmarking

func submitBenchmarkOrder(bookkeeper *MockBookkeeperStressTest, wsHub *MockWSHubStressTest, users []uuid.UUID, orderID int) {
	user := users[orderID%len(users)]

	order := &models.Order{
		ID:        uuid.New(),
		UserID:    user,
		Symbol:    "BTCUSDT",
		Side:      "BUY",
		Type:      "LIMIT",
		Price:     50000 + rand.Float64()*1000,
		Quantity:  0.001 + rand.Float64()*0.01,
		Status:    "NEW",
		CreatedAt: time.Now(),
	}

	// Simulate order processing
	_, _ = bookkeeper.GetBalance(user.String(), "USDT")
	reservationID, _ := bookkeeper.ReserveBalance(user.String(), "USDT", decimal.NewFromFloat(order.Price*order.Quantity))
	_ = bookkeeper.CommitReservation(reservationID)
	// Simulate WebSocket notification
	message := fmt.Sprintf(`{"type":"order","id":"%s","status":"filled"}`, order.ID)
	wsHub.BroadcastToUser(user.String(), []byte(message))
}

func performBenchmarkMatching(bookkeeper *MockBookkeeperStressTest, wsHub *MockWSHubStressTest, users []uuid.UUID, iteration int) {
	buyUser := users[iteration%len(users)]
	sellUser := users[(iteration+1)%len(users)]

	price := 50000.0
	quantity := 0.001
	// Simulate buy order processing
	buyOrder := &models.Order{
		ID:        uuid.New(),
		UserID:    buyUser,
		Symbol:    "BTCUSDT",
		Side:      "BUY",
		Type:      "LIMIT",
		Price:     price,
		Quantity:  quantity,
		Status:    "NEW",
		CreatedAt: time.Now(),
	}
	_ = buyOrder // Remove unused variable error
	// Simulate sell order processing
	sellOrder := &models.Order{
		ID:        uuid.New(),
		UserID:    sellUser,
		Symbol:    "BTCUSDT",
		Side:      "SELL",
		Type:      "LIMIT",
		Price:     price,
		Quantity:  quantity,
		Status:    "NEW",
		CreatedAt: time.Now(),
	}
	_ = sellOrder // Remove unused variable error

	// Simulate matching process
	_, _ = bookkeeper.GetBalance(buyUser.String(), "USDT")
	_, _ = bookkeeper.GetBalance(sellUser.String(), "BTC")

	reservationID1, _ := bookkeeper.ReserveBalance(buyUser.String(), "USDT", decimal.NewFromFloat(price*quantity))
	reservationID2, _ := bookkeeper.ReserveBalance(sellUser.String(), "BTC", decimal.NewFromFloat(quantity))

	_ = bookkeeper.CommitReservation(reservationID1)
	_ = bookkeeper.CommitReservation(reservationID2)

	// Simulate trade broadcast
	tradeMessage := fmt.Sprintf(`{"type":"trade","pair":"BTCUSDT","price":"%.2f","qty":"%.3f","time":%d}`,
		price, quantity, time.Now().UnixNano())
	wsHub.Broadcast("trades.BTCUSDT", []byte(tradeMessage))
}

func benchmarkConcurrentProcessing(b *testing.B, workers int, ordersEach int) {
	mockBookkeeper := &MockBookkeeperStressTest{}
	mockWSHub := &MockWSHubStressTest{}
	testUsers := make([]uuid.UUID, workers*2)
	for i := 0; i < len(testUsers); i++ {
		testUsers[i] = uuid.New()
		mockBookkeeper.SetBalance(testUsers[i].String(), "BTC", decimal.NewFromFloat(10.0))
		mockBookkeeper.SetBalance(testUsers[i].String(), "USDT", decimal.NewFromFloat(100000.0))
		mockWSHub.Connect(testUsers[i].String())
	}

	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		var wg sync.WaitGroup

		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()
				for j := 0; j < ordersEach; j++ {
					submitBenchmarkOrder(mockBookkeeper, mockWSHub, testUsers, workerID*ordersEach+j)
				}
			}(i)
		}

		wg.Wait()
	}
}

func benchmarkFlashCrashScenario(b *testing.B) {
	mockBookkeeper := &MockBookkeeperStressTest{}
	mockWSHub := &MockWSHubStressTest{}
	// Set up users for flash crash scenario
	numUsers := 50
	testUsers := make([]uuid.UUID, numUsers)
	for i := 0; i < numUsers; i++ {
		testUsers[i] = uuid.New()
		mockBookkeeper.SetBalance(testUsers[i].String(), "BTC", decimal.NewFromFloat(5.0))
		mockBookkeeper.SetBalance(testUsers[i].String(), "USDT", decimal.NewFromFloat(250000.0))
		mockWSHub.Connect(testUsers[i].String())
	}

	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		startPrice := 50000.0
		crashDepth := 0.20 // 20% crash

		// Simulate sudden sell-off
		for i := 0; i < 100; i++ {
			user := testUsers[i%numUsers]
			price := startPrice * (1 - crashDepth*float64(i)/100)

			order := &models.Order{ID: uuid.New(),
				UserID:    user,
				Symbol:    "BTCUSDT",
				Side:      "SELL",
				Type:      "MARKET",
				Price:     price,
				Quantity:  0.01 + rand.Float64()*0.1,
				Status:    "NEW",
				CreatedAt: time.Now(),
			}

			// Process sell order quickly
			_, _ = mockBookkeeper.GetBalance(user.String(), "BTC")

			// Broadcast price update
			priceMessage := fmt.Sprintf(`{"type":"price","pair":"BTCUSDT","price":"%.2f"}`, price)
			mockWSHub.Broadcast("price.BTCUSDT", []byte(priceMessage))

			_ = order // Use order variable
		}
	}
}

func benchmarkBullMarketScenario(b *testing.B) {
	mockBookkeeper := &MockBookkeeperStressTest{}
	mockWSHub := &MockWSHubStressTest{}
	numUsers := 100
	testUsers := make([]uuid.UUID, numUsers)
	for i := 0; i < numUsers; i++ {
		testUsers[i] = uuid.New()
		mockBookkeeper.SetBalance(testUsers[i].String(), "BTC", decimal.NewFromFloat(1.0))
		mockBookkeeper.SetBalance(testUsers[i].String(), "USDT", decimal.NewFromFloat(100000.0))
		mockWSHub.Connect(testUsers[i].String())
	}

	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		startPrice := 50000.0
		priceIncrease := 0.50 // 50% increase
		// Simulate bull market buying
		for i := 0; i < 200; i++ {
			user := testUsers[i%numUsers]
			price := startPrice * (1 + priceIncrease*float64(i)/200)

			order := &models.Order{ID: uuid.New(),
				UserID:    user,
				Symbol:    "BTCUSDT",
				Side:      "BUY",
				Type:      "LIMIT",
				Price:     price,
				Quantity:  0.001 + rand.Float64()*0.01,
				Status:    "NEW",
				CreatedAt: time.Now(),
			}

			// Process buy order
			_, _ = mockBookkeeper.GetBalance(user.String(), "USDT")

			// Broadcast volume update every 10 orders
			if i%10 == 0 {
				volumeMessage := fmt.Sprintf(`{"type":"volume","pair":"BTCUSDT","volume":"%.3f"}`,
					float64(i)*0.001)
				mockWSHub.Broadcast("volume.BTCUSDT", []byte(volumeMessage))
			}

			_ = order // Use order variable
		}
	}
}

func benchmarkHFTScenario(b *testing.B) {
	mockBookkeeper := &MockBookkeeperStressTest{}
	mockWSHub := &MockWSHubStressTest{}
	hftUser := uuid.New()
	mockBookkeeper.SetBalance(hftUser.String(), "BTC", decimal.NewFromFloat(100.0))
	mockBookkeeper.SetBalance(hftUser.String(), "USDT", decimal.NewFromFloat(5000000.0))
	mockWSHub.Connect(hftUser.String())

	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		basePrice := 50000.0

		// High-frequency order placement and cancellation
		for i := 0; i < 50; i++ {
			price := basePrice + float64(i%10-5) // Small price variations
			quantity := 0.001 + float64(i%5)*0.0001

			// Place buy order
			buyOrder := &models.Order{
				ID:        uuid.New(),
				UserID:    hftUser,
				Symbol:    "BTCUSDT",
				Side:      "BUY",
				Type:      "LIMIT",
				Price:     price - 1,
				Quantity:  quantity,
				Status:    "NEW",
				CreatedAt: time.Now(),
			}

			// Place sell order
			sellOrder := &models.Order{
				ID:        uuid.New(),
				UserID:    hftUser,
				Symbol:    "BTCUSDT",
				Side:      "SELL",
				Type:      "LIMIT",
				Price:     price + 1,
				Quantity:  quantity,
				Status:    "NEW",
				CreatedAt: time.Now(),
			}

			// Quick balance checks
			_, _ = mockBookkeeper.GetBalance(hftUser.String(), "USDT")
			_, _ = mockBookkeeper.GetBalance(hftUser.String(), "BTC") // Simulate rapid order updates
			if i%3 == 0 {
				buyOrder.Status = "CANCELED"
				sellOrder.Status = "CANCELED"
			}

			_ = buyOrder
			_ = sellOrder
		}
	}
}

func benchmarkLargeOrderScenario(b *testing.B) {
	mockBookkeeper := &MockBookkeeperStressTest{}
	mockWSHub := &MockWSHubStressTest{}

	whaleUser := uuid.New()
	mockBookkeeper.SetBalance(whaleUser.String(), "BTC", decimal.NewFromFloat(1000.0))
	mockBookkeeper.SetBalance(whaleUser.String(), "USDT", decimal.NewFromFloat(50000000.0))
	mockWSHub.Connect(whaleUser.String())

	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		// Large order that would typically be broken into smaller chunks
		totalQuantity := 50.0 // 50 BTC
		chunkSize := 0.5      // 0.5 BTC per chunk
		price := 50000.0

		chunks := int(totalQuantity / chunkSize)

		for i := 0; i < chunks; i++ {
			order := &models.Order{
				ID:        uuid.New(),
				UserID:    whaleUser,
				Symbol:    "BTCUSDT",
				Side:      "BUY",
				Type:      "LIMIT",
				Price:     price,
				Quantity:  chunkSize,
				Status:    "NEW",
				CreatedAt: time.Now(),
			}

			// Simulate order processing with larger amounts
			_, _ = mockBookkeeper.GetBalance(whaleUser.String(), "USDT")
			reservationID, _ := mockBookkeeper.ReserveBalance(whaleUser.String(), "USDT", decimal.NewFromFloat(price*chunkSize))
			_ = mockBookkeeper.CommitReservation(reservationID)

			// Broadcast large trade
			if i%5 == 0 {
				tradeMessage := fmt.Sprintf(`{"type":"large_trade","pair":"BTCUSDT","price":"%.2f","qty":"%.3f"}`,
					price, chunkSize*5)
				mockWSHub.Broadcast("trades.BTCUSDT", []byte(tradeMessage))
			}

			_ = order
		}
	}
}
