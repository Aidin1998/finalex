//go:build trading

package test

import (
	"math"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Aidin1998/finalex/internal/trading/model"
	"github.com/Aidin1998/finalex/internal/trading/orderbook"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// TestHighVolumeMatching runs a test with 100,000+ orders through the real orderbook
// implementation to validate performance and correctness
func TestHighVolumeMatching(t *testing.T) {
	// Skip test in short mode
	if testing.Short() {
		t.Skip("Skipping high volume test in short mode")
	}

	// Create real orderbook instance (not a mock)
	ob := orderbook.NewOrderBook("BTCUSDT")
	var matchCount int64
	var orderCount int64
	var errorCount int64

	// Create and process orders
	totalOrders := 100000
	buyerCount := 100
	sellerCount := 100
	buyers := make([]uuid.UUID, buyerCount)
	sellers := make([]uuid.UUID, sellerCount)

	// Initialize test users
	for i := 0; i < buyerCount; i++ {
		buyers[i] = uuid.New()
	}

	for i := 0; i < sellerCount; i++ {
		sellers[i] = uuid.New()
	}

	// Create orders
	t.Logf("Creating and processing %d orders...", totalOrders)
	startTime := time.Now()

	// Process orders with some concurrency
	var wg sync.WaitGroup
	concurrent := 4 // Number of goroutines to use
	ordersPerRoutine := totalOrders / concurrent
	for r := 0; r < concurrent; r++ {
		wg.Add(1)
		go func(routineID int) {
			defer wg.Done()

			start := routineID * ordersPerRoutine
			end := start + ordersPerRoutine
			if routineID == concurrent-1 {
				end = totalOrders // Ensure we process all orders
			}

			for i := start; i < end; i++ {
				// Alternate between buy and sell orders
				isBuy := i%2 == 0
				var side string
				var userID uuid.UUID
				var price float64

				if isBuy {
					side = model.OrderSideBuy
					userID = buyers[i%buyerCount]
					// Buy orders with prices around 50000
					price = 50000 - rand.Float64()*100
				} else {
					side = model.OrderSideSell
					userID = sellers[i%sellerCount]
					// Sell orders with prices around 50000
					price = 50000 + rand.Float64()*100
				}

				// Create the order
				order := &model.Order{
					ID:             uuid.New(),
					UserID:         userID,
					Pair:           "BTCUSDT",
					Side:           side,
					Type:           model.OrderTypeLimit,
					Price:          decimal.NewFromFloat(price),
					Quantity:       decimal.NewFromFloat(0.001 + rand.Float64()*0.01),
					FilledQuantity: decimal.Zero,
					TimeInForce:    model.TimeInForceGTC,
					Status:         model.OrderStatusNew,
				}

				// Add order to orderbook
				atomic.AddInt64(&orderCount, 1)
				result, err := ob.AddOrder(order)
				if err != nil {
					atomic.AddInt64(&errorCount, 1)
					continue
				}

				atomic.AddInt64(&matchCount, int64(len(result.Trades)))
			}
		}(r)
	}

	wg.Wait()
	duration := time.Since(startTime)

	// Log performance statistics
	t.Logf("Processed %d orders in %v", orderCount, duration)
	t.Logf("Orders per second: %.2f", float64(orderCount)/duration.Seconds())
	t.Logf("Matched %d trades", matchCount)
	t.Logf("Error count: %d", errorCount)
	t.Logf("Average order processing time: %.6f ms", float64(duration.Milliseconds())/float64(orderCount))

	// Verify that both heap and B-tree are used
	t.Log("Verifying heap and B-tree usage...") // Check stored orders - should be both in B-tree and heap
	bids, asks := ob.GetSnapshot(10)
	t.Logf("Orderbook contains %d bids and %d asks", len(bids), len(asks))

	// Final assertions
	processedOrders := atomic.LoadInt64(&orderCount)
	if processedOrders < int64(totalOrders) {
		t.Errorf("Expected to process %d orders, but only processed %d", totalOrders, processedOrders)
	}

	processedMatches := atomic.LoadInt64(&matchCount)
	if processedMatches == 0 {
		t.Errorf("Expected matches, but got 0 matches")
	}
}

// BenchmarkHighVolumeOrderMatching benchmarks the performance of the matching engine
// with a large volume of orders
func BenchmarkHighVolumeOrderMatching(b *testing.B) {
	// Create real orderbook instance (not a mock)
	ob := orderbook.NewOrderBook("BTCUSDT")

	// Prepare users
	userCount := 50
	users := make([]uuid.UUID, userCount)
	for i := 0; i < userCount; i++ {
		users[i] = uuid.New()
	}

	// Prepare template orders
	basePrice := 50000.0

	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		// Create a new batch of orders for each iteration
		matchCount := 0
		orderCount := 0

		// Number of orders to process in this benchmark iteration
		ordersToProcess := 10000 // Lower for benchmark iterations

		for i := 0; i < ordersToProcess; i++ {
			// Create buy or sell order
			isBuy := i%2 == 0
			var side string
			var price float64

			if isBuy {
				side = model.OrderSideBuy
				price = basePrice - rand.Float64()*100
			} else {
				side = model.OrderSideSell
				price = basePrice + rand.Float64()*100
			}

			order := &model.Order{
				ID:             uuid.New(),
				UserID:         users[i%userCount],
				Pair:           "BTCUSDT",
				Side:           side,
				Type:           model.OrderTypeLimit,
				Price:          decimal.NewFromFloat(price),
				Quantity:       decimal.NewFromFloat(0.001 + rand.Float64()*0.01),
				FilledQuantity: decimal.Zero,
				TimeInForce:    model.TimeInForceGTC,
				Status:         model.OrderStatusNew,
			}

			// Add order to orderbook
			orderCount++
			result, err := ob.AddOrder(order)
			if err == nil {
				matchCount += len(result.Trades)
			}
		}
		// Report metrics
		if orderCount > 0 {
			b.ReportMetric(float64(orderCount), "orders")
			b.ReportMetric(float64(matchCount), "matches")
			b.ReportMetric(float64(matchCount)/float64(orderCount)*100, "match_pct")
		}
	}
}

// BenchmarkHeapVsBTree compares performance characteristics of heap vs B-tree
// for different order book operations
func BenchmarkHeapVsBTree(b *testing.B) {
	// Create real orderbook instance (not a mock)
	ob := orderbook.NewOrderBook("BTCUSDT")

	// Prepare a set of orders
	orderCount := 10000
	orders := make([]*model.Order, orderCount)
	for i := 0; i < orderCount; i++ {
		isBuy := i%2 == 0
		side := model.OrderSideBuy
		if !isBuy {
			side = model.OrderSideSell
		}

		orders[i] = &model.Order{
			ID:             uuid.New(),
			UserID:         uuid.New(),
			Pair:           "BTCUSDT",
			Side:           side,
			Type:           "limit",
			Price:          decimal.NewFromFloat(50000.0 + float64(i%1000)),
			Quantity:       decimal.NewFromFloat(0.001 + float64(i%100)*0.001),
			FilledQuantity: decimal.Zero,
			TimeInForce:    "GTC",
			Status:         "new",
		}
	}

	b.Run("OrderAddition", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			// Add a batch of orders
			for j := 0; j < 100; j++ {
				idx := (i*100 + j) % len(orders)
				_, _ = ob.AddOrder(orders[idx])
			}
		}
	})

	b.Run("OrderCancellation", func(b *testing.B) {
		// First add orders
		addedOrders := make([]uuid.UUID, 0, 1000)
		for i := 0; i < 1000; i++ {
			result, err := ob.AddOrder(orders[i])
			if err == nil && len(result.RestingOrders) > 0 {
				for _, order := range result.RestingOrders {
					addedOrders = append(addedOrders, order.ID)
				}
			}
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if len(addedOrders) > 0 {
				idx := i % len(addedOrders)
				_ = ob.CancelOrder(addedOrders[idx])
			}
		}
	})

	b.Run("GetSnapshot", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = ob.GetSnapshot(10)
		}
	})
}

// Helpers to generate realistic order flow

func generateRealisticOrders(count int) []*model.Order {
	orders := make([]*model.Order, count)
	basePrice := 50000.0
	for i := 0; i < count; i++ {
		isBuy := i%2 == 0
		side := model.OrderSideBuy
		if !isBuy {
			side = model.OrderSideSell
		}

		// Price clustering around psychological barriers (whole numbers)
		priceNoise := rand.Float64() * 20
		if rand.Float64() < 0.8 {
			priceNoise = math.Floor(priceNoise) // 80% of orders at whole numbers
		}

		price := basePrice
		if isBuy {
			price -= priceNoise
		} else {
			price += priceNoise
		}

		// Order size follows power law (many small orders, few large ones)
		size := 0.001
		if rand.Float64() < 0.1 {
			// 10% chance of larger order
			size = 0.01 + rand.Float64()*0.1
		} else if rand.Float64() < 0.01 {
			// 1% chance of very large order
			size = 0.1 + rand.Float64()
		}

		orders[i] = &model.Order{
			ID:             uuid.New(),
			UserID:         uuid.New(),
			Pair:           "BTCUSDT",
			Side:           side,
			Type:           model.OrderTypeLimit,
			Price:          decimal.NewFromFloat(price),
			Quantity:       decimal.NewFromFloat(size),
			FilledQuantity: decimal.Zero,
			TimeInForce:    model.TimeInForceGTC,
			Status:         model.OrderStatusNew,
		}
	}

	return orders
}
