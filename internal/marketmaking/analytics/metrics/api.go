// API endpoints for real-time business metrics and compliance.
package metrics

import (
	"encoding/json"
	"net/http"
)

type MetricsAPI struct {
	BM         *BusinessMetrics
	AlertSvc   *AlertingService
	Compliance *ComplianceService
}

func NewMetricsAPI(bm *BusinessMetrics, alert *AlertingService, compliance *ComplianceService) *MetricsAPI {
	return &MetricsAPI{BM: bm, AlertSvc: alert, Compliance: compliance}
}

func (api *MetricsAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/metrics/fillrate":
		api.handleFillRate(w, r)
	case "/metrics/slippage":
		api.handleSlippage(w, r)
	case "/metrics/marketimpact":
		api.handleMarketImpact(w, r)
	case "/metrics/spread":
		api.handleSpread(w, r)
	case "/metrics/alerts":
		api.handleAlerts(w, r)
	case "/metrics/compliance":
		api.handleCompliance(w, r)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (api *MetricsAPI) handleFillRate(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(api.BM.FillRates)
}
func (api *MetricsAPI) handleSlippage(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(api.BM.Slippage)
}
func (api *MetricsAPI) handleMarketImpact(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(api.BM.MarketImpact)
}
func (api *MetricsAPI) handleSpread(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(api.BM.Spread)
}
func (api *MetricsAPI) handleAlerts(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(api.AlertSvc.Alerts)
}
func (api *MetricsAPI) handleCompliance(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(api.Compliance.Events)
}

// ... Extend with more endpoints and query params as needed ...
