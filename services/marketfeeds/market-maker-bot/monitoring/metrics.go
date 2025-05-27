package monitoring

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	OrderLatency = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "order_latency_seconds",
		Help:    "Order execution latency in seconds.",
		Buckets: prometheus.LinearBuckets(0.01, 0.01, 20),
	})

	TradeVolume = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "trade_volume_total",
		Help: "Total trade volume.",
	})
)

func InitMetrics() {
	prometheus.MustRegister(OrderLatency)
	prometheus.MustRegister(TradeVolume)
}
