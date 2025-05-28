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

// HTTP request metrics
var (
	HTTPRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "pincex_http_requests_total",
			Help: "Total number of HTTP requests received",
		},
		[]string{"path", "method", "status"},
	)
	HTTPRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "pincex_http_request_duration_seconds",
			Help:    "Histogram of HTTP request latencies",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"path", "method"},
	)
)

// Wallet operation metrics
var (
	WithdrawalRequestsTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "pincex_withdrawal_requests_total",
			Help: "Total number of withdrawal requests created",
		},
	)
	WithdrawalRequestDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "pincex_withdrawal_request_duration_seconds",
			Help:    "Duration in seconds of CreateWithdrawalRequest",
			Buckets: prometheus.DefBuckets,
		},
	)
	WithdrawalApprovalsTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "pincex_withdrawal_approvals_total",
			Help: "Total number of withdrawal approvals",
		},
	)
	WithdrawalBroadcastsTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "pincex_withdrawal_broadcasts_total",
			Help: "Total number of broadcasted withdrawals",
		},
	)
)

func init() {
	prometheus.MustRegister(OrdersProcessed, OrderLatency)
	prometheus.MustRegister(DBOpenConns, DBIdleConns, DBInUseConns)
	prometheus.MustRegister(HTTPRequestsTotal, HTTPRequestDuration)
	prometheus.MustRegister(
		WithdrawalRequestsTotal,
		WithdrawalRequestDuration,
		WithdrawalApprovalsTotal,
		WithdrawalBroadcastsTotal,
	)
}
