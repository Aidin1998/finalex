// Package middleware provides Prometheus metrics for middleware monitoring
package middleware

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.opentelemetry.io/otel"
)

// Prometheus metrics for middleware monitoring
var (
	// Authentication metrics
	authenticationAttempts = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "orbit",
			Subsystem: "middleware",
			Name:      "authentication_attempts_total",
			Help:      "Total authentication attempts",
		},
		[]string{"type", "status"},
	)

	// HTTP request metrics
	httpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "orbit",
			Subsystem: "middleware",
			Name:      "http_requests_total",
			Help:      "Total HTTP requests processed",
		},
		[]string{"method", "path", "status", "tier"},
	)

	httpRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "orbit",
			Subsystem: "middleware",
			Name:      "http_request_duration_seconds",
			Help:      "Duration of HTTP requests",
		},
		[]string{"method", "path", "status"},
	)

	httpRequestSize = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "orbit",
			Subsystem: "middleware",
			Name:      "http_request_size_bytes",
			Help:      "Size of HTTP requests",
		},
		[]string{"method", "path"},
	)

	httpResponseSize = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "orbit",
			Subsystem: "middleware",
			Name:      "http_response_size_bytes",
			Help:      "Size of HTTP responses",
		},
		[]string{"method", "path", "status"},
	)

	httpActiveConnections = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "orbit",
			Subsystem: "middleware",
			Name:      "http_active_connections",
			Help:      "Current number of active HTTP connections",
		},
	)

	// Validation metrics
	validationErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "orbit",
			Subsystem: "middleware",
			Name:      "validation_errors_total",
			Help:      "Total validation errors",
		},
		[]string{"type", "reason"},
	)

	// Security metrics
	securityViolations = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "orbit",
			Subsystem: "middleware",
			Name:      "security_violations_total",
			Help:      "Total security violations detected",
		},
		[]string{"type", "severity"},
	)

	// Recovery metrics
	panicRecoveries = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "orbit",
			Subsystem: "middleware",
			Name:      "panic_recoveries_total",
			Help:      "Total panic recoveries",
		},
		[]string{"path", "type"},
	)
)

// OpenTelemetry tracer
var tracer = otel.Tracer("orbit-middleware")
