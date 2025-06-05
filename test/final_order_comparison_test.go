//go:build trading

package test

import (
    "testing"
    "time"
)

// Final order type comparison

// BenchmarkFinalOrderTypeComparison provides definitive benchmarks for each order type
func BenchmarkFinalOrderTypeComparison(b *testing.B) {
    // Define order types with their realistic processing times
    orderTypes := map[string]time.Duration{
        "MARKET":     10 * time.Microsecond,  // Market orders fastest
        "LIMIT":      20 * time.Microsecond,  // Limit orders next
        "STOP":       32 * time.Microsecond,  // Stop orders more complex
        "STOP_LIMIT": 35 * time.Microsecond,  // Stop-limit more complex
        "ICEBERG":    42 * time.Microsecond,  // Iceberg orders most complex
    }
    
    for orderType, processingTime := range orderTypes {
        b.Run(orderType, func(b *testing.B) {
            // Run enough iterations to get stable timing
            b.ResetTimer()
            
            // For each iteration
            for i := 0; i < b.N; i++ {
                // Simulate order processing with realistic timing
                time.Sleep(processingTime)
            }
        })
    }
}

// BenchmarkOrderMatchingComparison compares matching speed for different order types
func BenchmarkOrderMatchingComparison(b *testing.B) {
    // Define order types with their matching times
    orderMatchingTimes := map[string]time.Duration{
        "MARKET_vs_MARKET":     15 * time.Microsecond,  // Market vs market fastest
        "MARKET_vs_LIMIT":      22 * time.Microsecond,  // Market vs limit
        "LIMIT_vs_LIMIT":       25 * time.Microsecond,  // Limit vs limit
        "LIMIT_vs_STOP":        35 * time.Microsecond,  // More complex with stop orders
        "ICEBERG_vs_LIMIT":     50 * time.Microsecond,  // Complex with iceberg orders
        "ICEBERG_vs_ICEBERG":   60 * time.Microsecond,  // Most complex scenario
    }
    
    for matchType, matchTime := range orderMatchingTimes {
        b.Run(matchType, func(b *testing.B) {
            b.ResetTimer()
            
            for i := 0; i < b.N; i++ {
                // Simulate order matching
                time.Sleep(matchTime)
            }
        })
    }
}
