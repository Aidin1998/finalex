package server

import (
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"

	"github.com/yourorg/fiat/api"
	"github.com/yourorg/fiat/exchange"
	"github.com/yourorg/fiat/rates"
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

type Server struct {
	Logger *zap.Logger
}

func NewServer(logger *zap.Logger) *Server {
	return &Server{Logger: logger}
}

func (s *Server) Start(addr string) error {
	mux := http.NewServeMux()
	mux.Handle("/health", prometheusMiddleware("health", http.HandlerFunc(s.healthHandler)))
	mux.Handle("/metrics", promhttp.Handler())

	// --- Rates Aggregator Setup ---
	providers := []rates.RateProvider{
		&rates.BinanceProvider{},
		&rates.CoinbaseProvider{},
		&rates.KrakenProvider{},
		&rates.BitfinexProvider{},
		&rates.OKXProvider{},
	}
	aggregator := rates.NewAggregator(providers, 30*time.Second)
	rateHandler := &api.RateHandler{Aggregator: aggregator}
	mux.Handle("/fiat/rates", prometheusMiddleware("fiat_rates", http.HandlerFunc(rateHandler.GetRates)))

	// --- Exchange Handler Setup ---
	spotClient := &exchange.HttpSpotClient{BaseURL: "http://localhost:8081"} // adjust as needed
	exService := &exchange.ExchangeService{
		Aggregator: aggregator,
		SpotClient: spotClient,
		FeePercent: 0.002, // 0.2% fee, adjust as needed
	}
	exchangeHandler := &api.ExchangeHandler{Service: exService}
	mux.Handle("/fiat/exchange", prometheusMiddleware("fiat_exchange", http.HandlerFunc(exchangeHandler.Exchange)))

	return http.ListenAndServe(addr, mux)
}

func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	// TODO: Ping DB & Stripe sandbox
	fmt.Fprintln(w, "Hello World: healthy")
}

// Prometheus middleware for metrics
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
