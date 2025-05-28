package monitoring

import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promhttp"
    "net/http"
)

var (
    alertThreshold = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "market_maker_alert_threshold",
            Help: "Threshold for market maker alerts",
        },
        []string{"alert_type"},
    )
)

func init() {
    prometheus.MustRegister(alertThreshold)
}

func SetAlertThreshold(alertType string, value float64) {
    alertThreshold.WithLabelValues(alertType).Set(value)
}

func StartMetricsServer(addr string) {
    http.Handle("/metrics", promhttp.Handler())
    go func() {
        if err := http.ListenAndServe(addr, nil); err != nil {
            // Handle error (e.g., log it)
        }
    }()
}