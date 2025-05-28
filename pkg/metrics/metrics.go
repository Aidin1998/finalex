package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

// OrdersProcessed counts total processed orders by side (buy/sell)
var OrdersProcessed = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "pincex_orders_processed_total",
		Help: "Total number of orders processed by the engine",
	},
	[]string{"side"},
)

// OrderLatency records latency distribution for order processing
var OrderLatency = prometheus.NewHistogram(
	prometheus.HistogramOpts{
		Name:    "pincex_order_processing_latency_seconds",
		Help:    "Latency in seconds to process individual orders",
		Buckets: prometheus.DefBuckets,
	},
)

// Database connection pool metrics
var (
	DBOpenConns = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "pincex_db_open_connections",
			Help: "Number of open connections in the DB pool",
		},
		[]string{"db"},
	)

	DBIdleConns = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "pincex_db_idle_connections",
			Help: "Number of idle connections in the DB pool",
		},
		[]string{"db"},
	)

	DBInUseConns = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "pincex_db_in_use_connections",
			Help: "Number of in-use connections in the DB pool",
		},
		[]string{"db"},
	)
)

func init() {
	prometheus.MustRegister(OrdersProcessed, OrderLatency)
	prometheus.MustRegister(DBOpenConns, DBIdleConns, DBInUseConns)
}
