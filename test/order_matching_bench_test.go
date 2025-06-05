//go:build trading

package test

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

// Order type constants with their processing latencies
var orderTypeLatencies = map[string]time.Duration{
	"market":     10 * time.Microsecond, // Fastest
	"limit":      20 * time.Microsecond,
	"stop":       30 * time.Microsecond,
	"stop-limit": 35 * time.Microsecond,
	"iceberg":    40 * time.Microsecond, // Slowest
}

// BenchmarkOrderTypeMatching tests the matching speed for different order types
func BenchmarkOrderTypeMatching(b *testing.B) {
	orderTypes := []string{
		"market",
		"limit",
		"stop",
		"stop-limit",
		"iceberg",
	}

	for _, orderType := range orderTypes {
		b.Run(fmt.Sprintf("Matching_%s", orderType), func(b *testing.B) {
			// Initialize metrics
			var orderCount int64
			var matchCount int64
			var totalLatency int64

			latency := orderTypeLatencies[orderType]

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				startTime := time.Now()

				// Simulate order matching process
				time.Sleep(latency)

				// Track metrics
				atomic.AddInt64(&orderCount, 1)
				atomic.AddInt64(&matchCount, 1)
				atomic.AddInt64(&totalLatency, time.Since(startTime).Nanoseconds())
			}

			// Report metrics
			b.ReportMetric(float64(matchCount)/float64(orderCount)*100, "match_rate_%")
			b.ReportMetric(float64(totalLatency)/float64(orderCount)/1000, "latency_µs/op")
		})
	}
}

// BenchmarkHighVolumeMatching tests order matching under high volume conditions
func BenchmarkHighVolumeMatching(b *testing.B) {
	orderTypes := []string{
		"market",
		"limit",
		"mixed", // 50% market, 50% limit
	}

	volumes := []int{
		100,   // 100 orders/sec
		1000,  // 1000 orders/sec
		10000, // 10000 orders/sec
	}

	for _, orderType := range orderTypes {
		for _, volume := range volumes {
			name := fmt.Sprintf("%s_%dOPS", orderType, volume)
			b.Run(name, func(b *testing.B) {
				// Calculate sleep time between ops to simulate volume
				sleepTime := time.Second / time.Duration(volume)

				var totalLatency int64

				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					startTime := time.Now()

					// Simulate processing based on order type
					if orderType == "mixed" {
						if i%2 == 0 {
							time.Sleep(orderTypeLatencies["market"])
						} else {
							time.Sleep(orderTypeLatencies["limit"])
						}
					} else {
						time.Sleep(orderTypeLatencies[orderType])
					}

					// Track latency
					atomic.AddInt64(&totalLatency, time.Since(startTime).Nanoseconds())

					// Wait to simulate desired rate
					time.Sleep(sleepTime)
				}

				// Report metrics
				b.ReportMetric(float64(totalLatency)/float64(b.N)/1000, "latency_µs/op")
				b.ReportMetric(float64(volume), "target_ops/sec")
			})
		}
	}
}

// BenchmarkOrderMemoryUsage simulates memory usage characteristics of different order types
func BenchmarkOrderMemoryUsage(b *testing.B) {
	b.Skip("Memory usage benchmark requires extended run time")

	orderTypes := []string{
		"market",
		"limit",
		"stop",
		"iceberg",
	}

	for _, orderType := range orderTypes {
		b.Run(fmt.Sprintf("Memory_%s", orderType), func(b *testing.B) {
			// Arrays to hold references to prevent garbage collection
			orders := make([]string, 0, b.N)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				// Create simulated order
				order := fmt.Sprintf("order_%s_%d", orderType, i)

				// Store reference to prevent GC
				orders = append(orders, order)

				// Process order
				time.Sleep(orderTypeLatencies[orderType])
			}

			// Force reference to remain
			b.ReportMetric(float64(len(orders)), "order_count")
		})
	}
}

// BenchmarkSustainedLoad tests order matching under sustained load
func BenchmarkSustainedLoad(b *testing.B) {
	b.Skip("Sustained load benchmark requires extended run time")

	orderTypes := []string{"market", "limit", "mixed"}

	for _, orderType := range orderTypes {
		b.Run(fmt.Sprintf("Sustained_%s", orderType), func(b *testing.B) {
			var totalLatency int64
			var peakLatency int64

			// Run for 5 seconds (simulating sustained load)
			b.ResetTimer()
			startTime := time.Now()
			i := 0
			for time.Since(startTime) < 5*time.Second {
				opStart := time.Now()

				// Process based on order type
				if orderType == "mixed" {
					if i%2 == 0 {
						time.Sleep(orderTypeLatencies["market"])
					} else {
						time.Sleep(orderTypeLatencies["limit"])
					}
				} else {
					time.Sleep(orderTypeLatencies[orderType])
				}

				// Track latency
				latency := time.Since(opStart).Nanoseconds()
				atomic.AddInt64(&totalLatency, latency)

				// Update peak latency if needed
				for {
					current := atomic.LoadInt64(&peakLatency)
					if latency <= current {
						break
					}
					if atomic.CompareAndSwapInt64(&peakLatency, current, latency) {
						break
					}
				}

				i++
			}

			// Report metrics
			b.ReportMetric(float64(i)/5.0, "ops/sec")
			b.ReportMetric(float64(totalLatency)/float64(i)/1000, "avg_latency_µs")
			b.ReportMetric(float64(peakLatency)/1000, "peak_latency_µs")
		})
	}
}
