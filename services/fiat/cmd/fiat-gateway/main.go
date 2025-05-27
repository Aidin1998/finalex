// main.go: wire up config, server, DI
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"go.uber.org/zap"

	"github.com/go-redis/redis/v8"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/yourorg/fiat/internal/config"
	"github.com/yourorg/fiat/internal/server"
)

var (
	httpRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"handler", "method", "code"},
	)
	httpRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Histogram of response latency (seconds) for HTTP requests",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"handler", "method"},
	)
)

func init() {
	prometheus.MustRegister(httpRequestsTotal)
	prometheus.MustRegister(httpRequestDuration)
}

func prometheusMiddleware(handlerName string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := &responseWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(ww, r)
		httpRequestsTotal.WithLabelValues(handlerName, r.Method, fmt.Sprintf("%d", ww.status)).Inc()
		httpRequestDuration.WithLabelValues(handlerName, r.Method).Observe(time.Since(start).Seconds())
	})
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

func main() {
	_ = config.LoadConfig() // load config, ignore unused var for now
	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatalf("failed to initialize logger: %v", err)
	}
	defer func() {
		if r := recover(); r != nil {
			logger.Error("panic recovered", zap.Any("error", r))
		}
		logger.Sync()
	}()

	// --- Real-time config reload via Redis pub/sub ---
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}
	redisClient := redis.NewClient(&redis.Options{Addr: redisAddr})
	ctx := context.Background()
	go func() {
		pubsub := redisClient.Subscribe(ctx, "fiat:config:reload")
		for {
			_, err := pubsub.ReceiveMessage(ctx)
			if err == nil {
				logger.Info("Reloading config on signal from Redis pubsub")
				_ = config.LoadConfig() // hot-reload config
				// Optionally: trigger reload of pairs/providers in memory
			}
			time.Sleep(1 * time.Second)
		}
	}()

	// Start the server using its Start method (which sets up all handlers)
	srv := server.NewServer(logger)
	logger.Info("Starting fiat-gateway", zap.String("addr", ":8080"))
	if err := srv.Start(":8080"); err != nil {
		logger.Fatal("server failed", zap.Error(err))
	}
}
