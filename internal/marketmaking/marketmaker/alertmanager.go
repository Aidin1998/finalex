// AlertManager for MarketMaker: logs, stores, and notifies on risk/compliance/strategy alerts
package marketmaker

import (
	"sync"
	"time"
)

type AlertSeverity string

const (
	AlertInfo     AlertSeverity = "info"
	AlertWarning  AlertSeverity = "warning"
	AlertError    AlertSeverity = "error"
	AlertCritical AlertSeverity = "critical"
)

type Alert struct {
	Timestamp time.Time         `json:"timestamp"`
	Type      string            `json:"type"`
	Severity  AlertSeverity     `json:"severity"`
	Message   string            `json:"message"`
	Labels    map[string]string `json:"labels,omitempty"`
}

type AlertManager struct {
	alerts []Alert
	mu     sync.RWMutex
}

func NewAlertManager() *AlertManager {
	return &AlertManager{alerts: make([]Alert, 0)}
}

func (a *AlertManager) Add(alert Alert) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.alerts = append(a.alerts, alert)
	if len(a.alerts) > 1000 {
		a.alerts = a.alerts[len(a.alerts)-1000:]
	}
}

func (a *AlertManager) List() []Alert {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return append([]Alert(nil), a.alerts...)
}

func (a *AlertManager) Notify(alert Alert) {
	// TODO: Integrate with email, Slack, or other notification systems
	// For now, just log or print
	// log.Printf("ALERT: %s [%s] %s", alert.Type, alert.Severity, alert.Message)
}
