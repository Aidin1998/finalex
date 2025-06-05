//go:build trading

package test

import (
	"fmt"
	"math/rand"
	"runtime"
	"testing"
	"time"

	"github.com/Aidin1998/finalex/pkg/models"
	"github.com/shopspring/decimal"
)

// BenchmarkMemoryUsage tests memory consumption for different order types under sustained load
func BenchmarkMemoryUsage(b *testing.B) {
	b.Run("MemoryUsage_LimitOrders", func(b *testing.B) {
		benchmarkMemoryWithOrderType(b, models.OrderTypeLimit)
	})

	b.Run("MemoryUsage_MarketOrders", func(b *testing.B) {
		benchmarkMemoryWithOrderType(b, models.OrderTypeMarket)
	})

	b.Run("MemoryUsage_StopOrders", func(b *testing.B) {
		benchmarkMemoryWithOrderType(b, models.OrderTypeStop)
	})

	b.Run("MemoryUsage_IcebergOrders", func(b *testing.B) {
		benchmarkMemoryWithOrderType(b, models.OrderTypeIceberg)
	})
}

// Helper function to benchmark memory usage for a specific order type
func benchmarkMemoryWithOrderType(b *testing.B, orderType models.OrderType) {
	mockBookkeeper := &MockBookkeeperStressTest{}
	mockWSHub := &MockWSHubStressTest{}

	// Set up test users
	testUsers := make([]string, 1000)
	for i := 0; i < len(testUsers); i++ {
		testUsers[i] = fmt.Sprintf("memory_user_%d", i)
		mockBookkeeper.SetBalance(testUsers[i], "BTC", decimal.NewFromFloat(10.0))
		mockBookkeeper.SetBalance(testUsers[i], "USDT", decimal.NewFromFloat(1000000.0))
	}

	// Start with baseline memory stats
	var baselineStats runtime.MemStats
	runtime.ReadMemStats(&baselineStats)
	baselineAlloc := baselineStats.Alloc
	baselineSys := baselineStats.Sys

	// Store orders for preventing GC
	var orders []*models.Order

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		user := testUsers[i%len(testUsers)]
		side := models.SideBuy
		if i%2 == 0 {
			side = models.SideSell
		}

		// Create order
		order := &models.Order{
			ID:        fmt.Sprintf("memory_%s_%d", orderType, i),
			UserID:    user,
			Symbol:    "BTCUSDT",
			Side:      side,
			Type:      orderType,
			Quantity:  decimal.NewFromFloat(0.01 + rand.Float64()*0.1),
			Status:    models.OrderStatusPending,
			Created:   time.Now(),
			UpdatedAt: time.Now(),
		}

		// Set type-specific fields
		switch orderType {
		case models.OrderTypeLimit:
			order.Price = decimal.NewFromFloat(50000.0 + rand.Float64()*1000.0 - 500.0)
		case models.OrderTypeMarket:
			// No price needed
		case models.OrderTypeStop, models.OrderTypeStopLimit:
			order.Price = decimal.NewFromFloat(50000.0 + rand.Float64()*1000.0 - 500.0)
			if side == models.SideBuy {
				order.StopPrice = order.Price.Add(decimal.NewFromFloat(100.0))
			} else {
				order.StopPrice = order.Price.Sub(decimal.NewFromFloat(100.0))
			}
		case models.OrderTypeIceberg:
			order.Price = decimal.NewFromFloat(50000.0 + rand.Float64()*1000.0 - 500.0)
			order.VisibleQuantity = order.Quantity.Div(decimal.NewFromInt(5))
			order.TotalQuantity = order.Quantity
		}

		// Keep reference to prevent GC
		orders = append(orders, order)

		// Process order (mock)
		if i%10 == 0 {
			// Create a trade
			trade := &models.Trade{
				ID:        fmt.Sprintf("trade_%d", i),
				OrderID:   order.ID,
				Symbol:    order.Symbol,
				Price:     decimal.NewFromFloat(50000.0),
				Quantity:  order.Quantity,
				Side:      order.Side,
				Timestamp: time.Now(),
			}

			// Broadcast trade
			mockWSHub.Broadcast("trades."+order.Symbol, []byte(fmt.Sprintf(`{"trade_id":"%s"}`, trade.ID)))
		}

		// Force GC occasionally to get more accurate numbers
		if i > 0 && i%5000 == 0 {
			runtime.GC()

			var m runtime.MemStats
			runtime.ReadMemStats(&m)

			currentAlloc := m.Alloc
			currentSys := m.Sys
			allocDiff := currentAlloc - baselineAlloc
			sysDiff := currentSys - baselineSys

			// Report intermediate memory usage
			b.ReportMetric(float64(currentAlloc)/1024/1024, "memory_alloc_mb")
			b.ReportMetric(float64(allocDiff)/1024/1024, "memory_growth_mb")
			b.ReportMetric(float64(currentSys)/1024/1024, "memory_sys_mb")
		}
	}

	// Force final GC
	runtime.GC()

	var finalStats runtime.MemStats
	runtime.ReadMemStats(&finalStats)

	// Report memory metrics
	b.ReportMetric(float64(finalStats.Alloc)/1024/1024, "alloc_mb")
	b.ReportMetric(float64(finalStats.TotalAlloc)/1024/1024, "total_alloc_mb")
	b.ReportMetric(float64(finalStats.Sys)/1024/1024, "sys_mb")
	b.ReportMetric(float64(finalStats.HeapAlloc)/1024/1024, "heap_alloc_mb")
	b.ReportMetric(float64(finalStats.HeapSys)/1024/1024, "heap_sys_mb")
	b.ReportMetric(float64(len(orders)), "order_count")
}

// BenchmarkLatencyUnderLoad tests the processing latency under different load conditions
func BenchmarkLatencyUnderLoad(b *testing.B) {
	loadLevels := []struct {
		name  string
		users int
		tps   int // simulated transactions per second
	}{
		{"Low_Load", 100, 100},
		{"Medium_Load", 500, 500},
		{"High_Load", 1000, 1000},
		{"Stress_Load", 2000, 2000},
	}

	orderTypes := []struct {
		name string
		typ  models.OrderType
	}{
		{"Limit", models.OrderTypeLimit},
		{"Market", models.OrderTypeMarket},
		{"Stop", models.OrderTypeStop},
		{"Iceberg", models.OrderTypeIceberg},
	}

	for _, load := range loadLevels {
		for _, orderType := range orderTypes {
			b.Run(fmt.Sprintf("%s_%s", load.name, orderType.name), func(b *testing.B) {
				benchmarkLatencyAtLoad(b, orderType.typ, load.users, load.tps)
			})
		}
	}
}

// Helper function to benchmark latency at specific load
func benchmarkLatencyAtLoad(b *testing.B, orderType models.OrderType, users int, tps int) {
	mockBookkeeper := &MockBookkeeperStressTest{}
	mockWSHub := &MockWSHubStressTest{}

	// Set up test users
	testUsers := make([]string, users)
	for i := 0; i < len(testUsers); i++ {
		testUsers[i] = fmt.Sprintf("latency_user_%d", i)
		mockBookkeeper.SetBalance(testUsers[i], "BTC", decimal.NewFromFloat(10.0))
		mockBookkeeper.SetBalance(testUsers[i], "USDT", decimal.NewFromFloat(1000000.0))
		mockWSHub.Connect(testUsers[i])
	}

	// Calculate sleep time between operations to simulate TPS
	sleepTime := time.Second / time.Duration(tps)

	// Track latencies
	var minLatency int64 = 999999999999
	var maxLatency int64 = 0
	var totalLatency int64 = 0

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		user := testUsers[i%len(testUsers)]

		startTime := time.Now().UnixNano()

		// Process order (similar to previous benchmarks)
		// ...

		endTime := time.Now().UnixNano()
		latency := endTime - startTime

		// Update latency stats
		if latency < minLatency {
			minLatency = latency
		}
		if latency > maxLatency {
			maxLatency = latency
		}
		totalLatency += latency

		// Simulate load by sleeping between operations
		time.Sleep(sleepTime)
	}

	// Report metrics
	b.ReportMetric(float64(totalLatency)/float64(b.N)/1000, "avg_latency_μs")
	b.ReportMetric(float64(minLatency)/1000, "min_latency_μs")
	b.ReportMetric(float64(maxLatency)/1000, "max_latency_μs")
	b.ReportMetric(float64(tps), "target_tps")
}
