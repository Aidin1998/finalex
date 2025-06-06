// Enhanced Prometheus metrics for production-grade MarketMaker observability
package marketmaker

import (
	"github.com/prometheus/client_golang/prometheus"
	"time"
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
	)
}

// MetricsCollector provides methods to update metrics with proper labeling
type MetricsCollector struct {
	startTime time.Time
}

func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{
		startTime: time.Now(),
	}
}

// Order lifecycle metric updates
func (m *MetricsCollector) RecordOrderPlaced(pair, side, strategy, orderType string) {
	OrdersPlacedTotal.WithLabelValues(pair, side, strategy, orderType).Inc()
}

func (m *MetricsCollector) RecordOrderCancelled(pair, side, strategy, reason string) {
	OrdersCancelledTotal.WithLabelValues(pair, side, strategy, reason).Inc()
}

func (m *MetricsCollector) RecordOrderFilled(pair, side, strategy, fillType string) {
	OrdersFilledTotal.WithLabelValues(pair, side, strategy, fillType).Inc()
}

func (m *MetricsCollector) RecordOrderRejected(pair, side, strategy, reason string) {
	OrdersRejectedTotal.WithLabelValues(pair, side, strategy, reason).Inc()
}

// Latency metric updates
func (m *MetricsCollector) RecordOrderPlacementLatency(pair, strategy, orderType string, duration time.Duration) {
	OrderPlacementLatency.WithLabelValues(pair, strategy, orderType).Observe(duration.Seconds())
}

func (m *MetricsCollector) RecordOrderCancellationLatency(pair, strategy string, duration time.Duration) {
	OrderCancellationLatency.WithLabelValues(pair, strategy).Observe(duration.Seconds())
}

// Inventory metric updates
func (m *MetricsCollector) RecordInventoryChange(pair, direction string) {
	InventoryChanges.WithLabelValues(pair, direction).Inc()
}

func (m *MetricsCollector) UpdateInventoryValue(pair string, value float64) {
	InventoryValue.WithLabelValues(pair).Set(value)
}

func (m *MetricsCollector) UpdateInventoryTurnover(pair string, turnover float64) {
	InventoryTurnover.WithLabelValues(pair).Set(turnover)
}

// Strategy metric updates
func (m *MetricsCollector) UpdateStrategyPnL(pair, strategy string, pnl float64) {
	StrategyPnLDaily.WithLabelValues(pair, strategy).Set(pnl)
}

func (m *MetricsCollector) RecordStrategyOrder(pair, strategy, orderType string) {
	StrategyOrderCount.WithLabelValues(pair, strategy, orderType).Inc()
}

func (m *MetricsCollector) UpdateStrategyStatus(pair, strategy string, active bool) {
	var status float64
	if active {
		status = 1
	}
	StrategyActiveGauge.WithLabelValues(pair, strategy).Set(status)
}

// Risk metric updates
func (m *MetricsCollector) RecordRiskEvent(eventType, severity, pair string) {
	RiskEventsTotal.WithLabelValues(eventType, severity, pair).Inc()
}

func (m *MetricsCollector) RecordRiskLimitBreach(limitType, pair string) {
	RiskLimitBreaches.WithLabelValues(limitType, pair).Inc()
}

// Feed health metric updates
func (m *MetricsCollector) RecordFeedLatency(feedType, pair, provider string, duration time.Duration) {
	FeedLatency.WithLabelValues(feedType, pair, provider).Observe(duration.Seconds())
}

func (m *MetricsCollector) RecordFeedDisconnection(feedType, provider, reason string) {
	FeedDisconnections.WithLabelValues(feedType, provider, reason).Inc()
}

func (m *MetricsCollector) UpdateFeedMessageRate(feedType, pair, provider string, rate float64) {
	FeedMessageRate.WithLabelValues(feedType, pair, provider).Set(rate)
}

func (m *MetricsCollector) UpdateFeedHealthStatus(feedType, pair, provider string, healthy bool) {
	var status float64
	if healthy {
		status = 1
	}
	FeedHealthStatus.WithLabelValues(feedType, pair, provider).Set(status)
}

// System health metric updates
func (m *MetricsCollector) UpdateSystemUptime(component string) {
	uptime := time.Since(m.startTime).Seconds()
	SystemUptime.WithLabelValues(component).Set(uptime)
}

func (m *MetricsCollector) UpdateComponentHealth(component, subsystem string, healthy bool) {
	var status float64
	if healthy {
		status = 1
	}
	ComponentHealthStatus.WithLabelValues(component, subsystem).Set(status)
}

func (m *MetricsCollector) RecordAPICallLatency(apiType, endpoint, status string, duration time.Duration) {
	APICallLatency.WithLabelValues(apiType, endpoint, status).Observe(duration.Seconds())
}

// Emergency metric updates
func (m *MetricsCollector) RecordEmergencyStop(triggerReason, component string) {
	EmergencyStops.WithLabelValues(triggerReason, component).Inc()
}

func (m *MetricsCollector) UpdateCircuitBreakerStatus(breakerType, pair string, open bool) {
	var status float64
	if open {
		status = 1
	}
	CircuitBreakerStatus.WithLabelValues(breakerType, pair).Set(status)
}

// Performance metric updates
func (m *MetricsCollector) RecordBatchOperationSize(operationType string, size int) {
	BatchOperationSize.WithLabelValues(operationType).Observe(float64(size))
}

func (m *MetricsCollector) UpdateCacheHitRate(cacheType string, hitRate float64) {
	CacheHitRate.WithLabelValues(cacheType).Set(hitRate)
}

func (m *MetricsCollector) UpdateMemoryPoolUtilization(poolType string, utilization float64) {
	MemoryPoolUtilization.WithLabelValues(poolType).Set(utilization)
}
