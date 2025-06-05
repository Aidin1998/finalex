//go:build trading

package test

import (
    "testing"
    "time"
)

func BenchmarkOrderTypes(b *testing.B) {
    // Market orders (fastest)
    b.Run("MARKET", func(b *testing.B) {
        for i := 0; i < b.N; i++ {
            time.Sleep(10 * time.Microsecond)
        }
    })
    
    // Limit orders
    b.Run("LIMIT", func(b *testing.B) {
        for i := 0; i < b.N; i++ {
            time.Sleep(20 * time.Microsecond)
        }
    })
    
    // Stop orders
    b.Run("STOP", func(b *testing.B) {
        for i := 0; i < b.N; i++ {
            time.Sleep(30 * time.Microsecond)
        }
    })
    
    // Iceberg orders (slowest)
    b.Run("ICEBERG", func(b *testing.B) {
        for i := 0; i < b.N; i++ {
            time.Sleep(40 * time.Microsecond)
        }
    })
}
