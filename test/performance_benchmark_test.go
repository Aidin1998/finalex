//go:build performance
// +build performance

package test

import (
	"net/url"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func BenchmarkOrderPlacement(b *testing.B) {
	for i := 0; i < b.N; i++ {
		// Simulate order placement (replace with real logic)
		time.Sleep(1 * time.Millisecond)
	}
}

// Benchmark WebSocket throughput and latency under load
func BenchmarkWebSocketThroughput(b *testing.B) {
	addr := "ws://localhost:8080/ws?protocol=binary"
	clients := 1000 // Simulate 1000 concurrent clients
	conns := make([]*websocket.Conn, 0, clients)
	for i := 0; i < clients; i++ {
		u, _ := url.Parse(addr)
		c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
		if err != nil {
			b.Fatalf("dial: %v", err)
		}
		conns = append(conns, c)
	}
	b.ResetTimer()
	start := time.Now()
	for i := 0; i < b.N; i++ {
		for _, c := range conns {
			_, _, err := c.ReadMessage()
			if err != nil {
				b.Fatalf("read: %v", err)
			}
		}
	}
	b.StopTimer()
	elapsed := time.Since(start)
	b.Logf("Total time: %v", elapsed)
	for _, c := range conns {
		c.Close()
	}
}
