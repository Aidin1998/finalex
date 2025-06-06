// Enhanced Prometheus metrics for production-grade MarketMaker observability
package marketmaker

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	// Order lifecycle metrics with detailed tracking
	OrdersPlacedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "pincex_mm_orders_placed_total",
			Help: "Total number of orders placed by market maker",
		},
		[]string{"pair", "side", "strategy", "order_type"},
	)

	OrdersCancelledTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "pincex_mm_orders_cancelled_total",
			Help: "Total number of orders cancelled by market maker",
		},
		[]string{"pair", "side", "strategy", "reason"},
	)

	OrdersFilledTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "pincex_mm_orders_filled_total",
			Help: "Total number of orders filled",
		},
		[]string{"pair", "side", "strategy", "fill_type"},
	)

	OrdersRejectedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "pincex_mm_orders_rejected_total",
			Help: "Total number of orders rejected",
		},
		[]string{"pair", "side", "strategy", "rejection_reason"},
	)

	// Order latency metrics
	OrderPlacementLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "pincex_mm_order_placement_latency_seconds",
			Help:    "Latency of order placement operations",
			Buckets: prometheus.ExponentialBuckets(0.001, 2, 15), // 1ms to 16s
		},
		[]string{"pair", "strategy", "order_type"},
	)

	OrderCancellationLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "pincex_mm_order_cancellation_latency_seconds",
			Help:    "Latency of order cancellation operations",
			Buckets: prometheus.ExponentialBuckets(0.001, 2, 15),
		},
		[]string{"pair", "strategy"},
	)

	OrderFillLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "pincex_mm_order_fill_latency_seconds",
			Help:    "Latency between order placement and fill",
			Buckets: prometheus.ExponentialBuckets(0.001, 2, 15), // 1ms to 16s
		},
		[]string{"pair"},
	)

	// Inventory movement metrics
	InventoryChanges = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "pincex_mm_inventory_changes_total",
			Help: "Total inventory changes by pair and direction",
		},
		[]string{"pair", "direction"}, // "buy" or "sell"
	)

	InventoryValue = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "pincex_mm_inventory_value_usd",
			Help: "Current USD value of inventory by pair",
		},
		[]string{"pair"},
	)

	InventoryTurnover = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "pincex_mm_inventory_turnover",
			Help: "Inventory turnover rate (volume/avg_inventory)",
		},
		[]string{"pair"},
	)

	// Strategy performance metrics
	StrategyPnLDaily = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "pincex_mm_strategy_pnl_daily_usd",
			Help: "Daily PnL by strategy and pair",
		},
		[]string{"pair", "strategy"},
	)

	StrategyOrderCount = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "pincex_mm_strategy_order_count_total",
			Help: "Total orders processed by strategy",
		},
		[]string{"pair", "strategy", "order_type"},
	)

	StrategyActiveGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "pincex_mm_strategy_active",
			Help: "Whether strategy is currently active (1=active, 0=inactive)",
		},
		[]string{"pair", "strategy"},
	)

	// Risk event metrics
	RiskEventsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "pincex_mm_risk_events_total",
			Help: "Total risk events by type and severity",
		},
		[]string{"event_type", "severity", "pair"},
	)

	RiskLimitBreaches = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "pincex_mm_risk_limit_breaches_total",
			Help: "Total risk limit breaches",
		},
		[]string{"limit_type", "pair"},
	)

	// Feed health metrics
	FeedLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "pincex_mm_feed_latency_seconds",
			Help:    "Market data feed latency",
			Buckets: prometheus.ExponentialBuckets(0.001, 2, 15),
		},
		[]string{"feed_type", "pair", "provider"},
	)

	FeedDisconnections = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "pincex_mm_feed_disconnections_total",
			Help: "Total feed disconnections",
		},
		[]string{"feed_type", "provider", "reason"},
	)

	FeedMessageRate = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "pincex_mm_feed_message_rate",
			Help: "Messages per second from market data feeds",
		},
		[]string{"feed_type", "pair", "provider"},
	)

	FeedHealthStatus = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "pincex_mm_feed_health_status",
			Help: "Health status of market data feeds (1=healthy, 0=unhealthy)",
		},
		[]string{"feed_type", "pair", "provider"},
	)

	// System health metrics
	SystemUptime = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "pincex_mm_system_uptime_seconds",
			Help: "System uptime in seconds",
		},
		[]string{"component"},
	)

	ComponentHealthStatus = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "pincex_mm_component_health_status",
			Help: "Health status of system components (1=healthy, 0=unhealthy)",
		},
		[]string{"component", "subsystem"},
	)

	APICallLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "pincex_mm_api_call_latency_seconds",
			Help:    "Latency of external API calls",
			Buckets: prometheus.ExponentialBuckets(0.01, 2, 15),
		},
		[]string{"api_type", "endpoint", "status"},
	)

	// Emergency metrics
	EmergencyStops = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "pincex_mm_emergency_stops_total",
			Help: "Total emergency stop activations",
		},
		[]string{"trigger_reason", "component"},
	)

	CircuitBreakerStatus = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "pincex_mm_circuit_breaker_status",
			Help: "Circuit breaker status (1=open, 0=closed)",
		},
		[]string{"breaker_type", "pair"},
	)

	// Performance optimization metrics
	BatchOperationSize = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "pincex_mm_batch_operation_size",
			Help:    "Size of batch operations",
			Buckets: prometheus.LinearBuckets(1, 5, 20), // 1 to 100
		},
		[]string{"operation_type"},
	)

	CacheHitRate = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "pincex_mm_cache_hit_rate",
			Help: "Cache hit rate for various operations",
		},
		[]string{"cache_type"},
	)

	MemoryPoolUtilization = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "pincex_mm_memory_pool_utilization",
			Help: "Memory pool utilization percentage",
		},
		[]string{"pool_type"},
	)

	// Backtest metrics for strategy evaluation
	BacktestCompletions = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "pincex_mm_backtest_completions_total",
			Help: "Total number of completed backtests",
		},
	)

	BacktestReturn = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "pincex_mm_backtest_return",
			Help: "Return percentage for backtest",
		},
		[]string{"backtest_id"},
	)

	BacktestSharpe = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "pincex_mm_backtest_sharpe_ratio",
			Help: "Sharpe ratio for backtest",
		},
		[]string{"backtest_id"},
	)

	// Operation duration metrics
	OperationDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "pincex_mm_operation_duration_ms",
			Help:    "Duration of market making operations in milliseconds",
			Buckets: prometheus.ExponentialBuckets(1, 2, 10), // 1ms to ~1s
		},
		[]string{"operation"},
	)

	// Order volume metrics
	OrderVolume = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "pincex_mm_order_volume",
			Help: "Volume of orders in base currency",
		},
		[]string{"pair", "side", "strategy"},
	)
)

// Register all enhanced metrics
func init() {
	prometheus.MustRegister(
		// Order lifecycle
		OrdersPlacedTotal,
		OrdersCancelledTotal,
		OrdersFilledTotal,
		OrdersRejectedTotal,

		// Latency
		OrderPlacementLatency,
		OrderCancellationLatency,
		OrderFillLatency,

		// Inventory
		InventoryChanges,
		InventoryValue,
		InventoryTurnover,

		// Strategy
		StrategyPnLDaily,
		StrategyOrderCount,
		StrategyActiveGauge,

		// Risk
		RiskEventsTotal,
		RiskLimitBreaches,

		// Feed health
		FeedLatency,
		FeedDisconnections,
		FeedMessageRate,
		FeedHealthStatus,

		// System health
		SystemUptime,
		ComponentHealthStatus,
		APICallLatency,

		// Emergency
		EmergencyStops,
		CircuitBreakerStatus,

		// Performance
		BatchOperationSize,
		CacheHitRate,
		MemoryPoolUtilization,

		// Backtest
		BacktestCompletions,
		BacktestReturn,
		BacktestSharpe,

		// Operations
		OperationDuration,
		OrderVolume,
	)
}

// PrometheusMetricsCollector provides methods to update prometheus metrics with proper labeling
type PrometheusMetricsCollector struct {
	startTime time.Time
}

func NewPrometheusMetricsCollector() *PrometheusMetricsCollector {
	return &PrometheusMetricsCollector{
		startTime: time.Now(),
	}
}

// Order lifecycle metric updates
func (m *PrometheusMetricsCollector) RecordOrderPlaced(pair, side, strategy, orderType string) {
	OrdersPlacedTotal.WithLabelValues(pair, side, strategy, orderType).Inc()
}

func (m *PrometheusMetricsCollector) RecordOrderCancelled(pair, side, strategy, reason string) {
	OrdersCancelledTotal.WithLabelValues(pair, side, strategy, reason).Inc()
}

func (m *PrometheusMetricsCollector) RecordOrderFilled(pair, side, strategy, fillType string) {
	OrdersFilledTotal.WithLabelValues(pair, side, strategy, fillType).Inc()
}

func (m *PrometheusMetricsCollector) RecordOrderRejected(pair, side, strategy, reason string) {
	OrdersRejectedTotal.WithLabelValues(pair, side, strategy, reason).Inc()
}

// Latency metric updates
func (m *PrometheusMetricsCollector) RecordOrderPlacementLatency(pair, strategy, orderType string, duration time.Duration) {
	OrderPlacementLatency.WithLabelValues(pair, strategy, orderType).Observe(duration.Seconds())
}

func (m *PrometheusMetricsCollector) RecordOrderCancellationLatency(pair, strategy string, duration time.Duration) {
	OrderCancellationLatency.WithLabelValues(pair, strategy).Observe(duration.Seconds())
}

func (m *PrometheusMetricsCollector) RecordOrderFillLatency(pair string, duration time.Duration) {
	OrderFillLatency.WithLabelValues(pair).Observe(duration.Seconds())
}

// Inventory metric updates
func (m *PrometheusMetricsCollector) RecordInventoryChange(pair, direction string) {
	InventoryChanges.WithLabelValues(pair, direction).Inc()
}

func (m *PrometheusMetricsCollector) UpdateInventoryValue(pair string, value float64) {
	InventoryValue.WithLabelValues(pair).Set(value)
}

func (m *PrometheusMetricsCollector) UpdateInventoryTurnover(pair string, turnover float64) {
	InventoryTurnover.WithLabelValues(pair).Set(turnover)
}

// Strategy metric updates
func (m *PrometheusMetricsCollector) UpdateStrategyPnL(pair, strategy string, pnl float64) {
	StrategyPnLDaily.WithLabelValues(pair, strategy).Set(pnl)
}

func (m *PrometheusMetricsCollector) RecordStrategyOrder(pair, strategy, orderType string) {
	StrategyOrderCount.WithLabelValues(pair, strategy, orderType).Inc()
}

func (m *PrometheusMetricsCollector) UpdateStrategyStatus(pair, strategy string, active bool) {
	var status float64
	if active {
		status = 1
	}
	StrategyActiveGauge.WithLabelValues(pair, strategy).Set(status)
}

// Risk metric updates
func (m *PrometheusMetricsCollector) RecordRiskEvent(eventType, severity, pair string) {
	RiskEventsTotal.WithLabelValues(eventType, severity, pair).Inc()
}

func (m *PrometheusMetricsCollector) RecordRiskLimitBreach(limitType, pair string) {
	RiskLimitBreaches.WithLabelValues(limitType, pair).Inc()
}

// Feed health metric updates
func (m *PrometheusMetricsCollector) RecordFeedLatency(feedType, pair, provider string, duration time.Duration) {
	FeedLatency.WithLabelValues(feedType, pair, provider).Observe(duration.Seconds())
}

func (m *PrometheusMetricsCollector) RecordFeedDisconnection(feedType, provider, reason string) {
	FeedDisconnections.WithLabelValues(feedType, provider, reason).Inc()
}

func (m *PrometheusMetricsCollector) UpdateFeedMessageRate(feedType, pair, provider string, rate float64) {
	FeedMessageRate.WithLabelValues(feedType, pair, provider).Set(rate)
}

func (m *PrometheusMetricsCollector) UpdateFeedHealthStatus(feedType, pair, provider string, healthy bool) {
	var status float64
	if healthy {
		status = 1
	}
	FeedHealthStatus.WithLabelValues(feedType, pair, provider).Set(status)
}

// System health metric updates
func (m *PrometheusMetricsCollector) UpdateSystemUptime(component string) {
	uptime := time.Since(m.startTime).Seconds()
	SystemUptime.WithLabelValues(component).Set(uptime)
}

func (m *PrometheusMetricsCollector) UpdateComponentHealth(component, subsystem string, healthy bool) {
	var status float64
	if healthy {
		status = 1
	}
	ComponentHealthStatus.WithLabelValues(component, subsystem).Set(status)
}

func (m *PrometheusMetricsCollector) RecordAPICallLatency(apiType, endpoint, status string, duration time.Duration) {
	APICallLatency.WithLabelValues(apiType, endpoint, status).Observe(duration.Seconds())
}

// Emergency metric updates
func (m *PrometheusMetricsCollector) RecordEmergencyStop(triggerReason, component string) {
	EmergencyStops.WithLabelValues(triggerReason, component).Inc()
}

func (m *PrometheusMetricsCollector) UpdateCircuitBreakerStatus(breakerType, pair string, open bool) {
	var status float64
	if open {
		status = 1
	}
	CircuitBreakerStatus.WithLabelValues(breakerType, pair).Set(status)
}

// Performance metric updates
func (m *PrometheusMetricsCollector) RecordBatchOperationSize(operationType string, size int) {
	BatchOperationSize.WithLabelValues(operationType).Observe(float64(size))
}

func (m *PrometheusMetricsCollector) UpdateCacheHitRate(cacheType string, hitRate float64) {
	CacheHitRate.WithLabelValues(cacheType).Set(hitRate)
}

func (m *PrometheusMetricsCollector) UpdateMemoryPoolUtilization(poolType string, utilization float64) {
	MemoryPoolUtilization.WithLabelValues(poolType).Set(utilization)
}

// Backtest metric updates
func (m *PrometheusMetricsCollector) RecordBacktestCompletion() {
	BacktestCompletions.Inc()
}

func (m *PrometheusMetricsCollector) UpdateBacktestReturn(backtestID string, returnPct float64) {
	BacktestReturn.WithLabelValues(backtestID).Set(returnPct)
}

func (m *PrometheusMetricsCollector) UpdateBacktestSharpe(backtestID string, sharpeRatio float64) {
	BacktestSharpe.WithLabelValues(backtestID).Set(sharpeRatio)
}

// Operation metric updates
func (m *PrometheusMetricsCollector) RecordOperationDuration(operation string, duration time.Duration) {
	OperationDuration.WithLabelValues(operation).Observe(duration.Seconds() * 1000) // Convert to ms
}
