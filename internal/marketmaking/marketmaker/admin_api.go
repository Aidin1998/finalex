// Admin and monitoring API for MarketMaker risk, compliance, and strategy status
package marketmaker

import (
	"encoding/json"
	"net/http"
)

// AdminAPI exposes risk, compliance, and operational status endpoints
// Attach to main HTTP server in cmd/pincex or as a subrouter
func (s *Service) RegisterAdminAPI(mux *http.ServeMux) {
	mux.HandleFunc("/admin/marketmaker/risk", s.handleRiskStatus)
	mux.HandleFunc("/admin/marketmaker/limits", s.handleRiskLimits)
	mux.HandleFunc("/admin/marketmaker/compliance", s.handleComplianceStatus)
	mux.HandleFunc("/admin/marketmaker/strategies", s.handleStrategyStatus)
	mux.HandleFunc("/admin/marketmaker/alerts", s.handleRiskAlerts)
	mux.HandleFunc("/admin/marketmaker/alertlog", s.handleAlertLog)
}

func (s *Service) handleRiskStatus(w http.ResponseWriter, r *http.Request) {
	status := s.GetRiskStatus()
	json.NewEncoder(w).Encode(status)
}

func (s *Service) handleRiskLimits(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	limits := s.riskManager.limits
	s.mu.RUnlock()
	json.NewEncoder(w).Encode(limits)
}

func (s *Service) handleComplianceStatus(w http.ResponseWriter, r *http.Request) {
	// Placeholder: wire to compliance module if available
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Service) handleStrategyStatus(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	strategies := make(map[string]interface{})
	for name, perf := range s.strategyPerformance {
		strategies[name] = perf
	}
	s.mu.RUnlock()
	json.NewEncoder(w).Encode(strategies)
}

func (s *Service) handleRiskAlerts(w http.ResponseWriter, r *http.Request) {
	alerts := s.riskManager.GetRiskSignals(0) // All severities
	json.NewEncoder(w).Encode(alerts)
}

func (s *Service) handleAlertLog(w http.ResponseWriter, r *http.Request) {
	alerts := s.alertManager.List()
	json.NewEncoder(w).Encode(alerts)
}
