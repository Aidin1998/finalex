package apiutil

import (
	"fmt"
	"time"

	"github.com/Aidin1998/pincex_unified/pkg/metrics"
	"github.com/gin-gonic/gin"
)

// MetricsMiddleware records HTTP request counts and durations for Prometheus
func MetricsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		// Use full route path (e.g., /api/v1/health)
		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}
		method := c.Request.Method
		status := fmt.Sprintf("%d", c.Writer.Status())
		// Increment request counter
		metrics.HTTPRequestsTotal.WithLabelValues(path, method, status).Inc()
		// Record duration
		dur := time.Since(start).Seconds()
		metrics.HTTPRequestDuration.WithLabelValues(path, method).Observe(dur)
	}
}
