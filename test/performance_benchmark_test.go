package api_test

import (
	"testing"
	"time"
)

func BenchmarkOrderPlacement(b *testing.B) {
	for i := 0; i < b.N; i++ {
		// Simulate order placement (replace with real logic)
		time.Sleep(1 * time.Millisecond)
	}
}
