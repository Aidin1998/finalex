package performance

import (
	"fmt"
	"market-maker-bot/orderbook"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	OrdersPerSec = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "orders_per_second",
		Help: "Orders processed per second.",
	})
	OrderLatency = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "stress_order_latency_seconds",
		Help:    "Order processing latency in seconds during stress test.",
		Buckets: prometheus.LinearBuckets(0.0001, 0.0001, 20),
	})
)

func init() {
	prometheus.MustRegister(OrdersPerSec)
	prometheus.MustRegister(OrderLatency)
}

func BenchmarkOrderBookStress(b *testing.B) {
	ob := orderbook.NewOrderBook()
	var wg sync.WaitGroup
	start := time.Now()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			order := &orderbook.Order{ID: fmt.Sprintf("%d", i), Price: float64(i % 1000), Volume: 1}
			begin := time.Now()
			ob.AddOrUpdateOrder(order)
			OrderLatency.Observe(time.Since(begin).Seconds())
		}(i)
	}
	wg.Wait()
	dur := time.Since(start).Seconds()
	OrdersPerSec.Set(float64(b.N) / dur)
	b.ReportMetric(float64(b.N)/dur, "orders/sec")
	b.ReportAllocs()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	b.Logf("Memory Usage: Alloc = %v MiB", m.Alloc/1024/1024)
	b.Logf("CPU NumGoroutine: %d", runtime.NumGoroutine())
}
