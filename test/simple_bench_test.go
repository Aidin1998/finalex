//go:build trading

package test

import (
    "fmt"
    "testing"
    "time"
)

// Simple benchmark for order types

// BenchmarkSimpleOrders tests different order types
func BenchmarkSimpleOrders(b *testing.B) {
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
            
            // Record starting time for each order type
            startTime := time.Now()
            
            for i := 0; i < b.N; i++ {
                // Simulate different processing times based on order type
                switch orderType {
                case "market":
                    time.Sleep(10 * time.Microsecond)
                case "limit":
                    time.Sleep(20 * time.Microsecond)
                case "stop":
                    time.Sleep(30 * time.Microsecond)
                case "stop-limit":
                    time.Sleep(35 * time.Microsecond)
                case "iceberg":
                    time.Sleep(40 * time.Microsecond)
                }
            }
            
            // Report custom metrics for execution time
            totalTime := time.Since(startTime)
            avgTimePerOp := float64(totalTime.Nanoseconds()) / float64(b.N)
            b.ReportMetric(avgTimePerOp/1000, "µs/op")
        })
    }
}
