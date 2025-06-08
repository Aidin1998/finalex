// monitor.go: Metrics, logging, and monitoring for rate limiting
package ratelimit

import (
	"log"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	metricRateLimitAllowed = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ratelimit_allowed_total",
			Help: "Total allowed requests by rate limiter.",
		},
		[]string{"key", "type"},
	)
	metricRateLimitBlocked = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ratelimit_blocked_total",
			Help: "Total blocked requests by rate limiter.",
		},
		[]string{"key", "type"},
	)
)

// InitMetrics initializes the metrics for rate limiting
func InitMetrics() {
	prometheus.MustRegister(metricRateLimitAllowed, metricRateLimitBlocked)
}

// RecordAllowed increments allowed request metrics
func RecordAllowed(key, typ string) {
	metricRateLimitAllowed.WithLabelValues(key, typ).Inc()
}

// RecordBlocked increments blocked request metrics
func RecordBlocked(key, typ string) {
	metricRateLimitBlocked.WithLabelValues(key, typ).Inc()
}

// LogEvent logs rate limit events
func LogEvent(msg string) {
	log.Println("[ratelimit]", msg)
}
