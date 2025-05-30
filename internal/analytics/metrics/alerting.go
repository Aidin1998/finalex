// Alerting for business metrics: slippage, fill rate, manipulation, trading patterns.
package metrics

import (
	"time"
)

type AlertType string

const (
	AlertSlippage        AlertType = "slippage"
	AlertLowFillRate     AlertType = "low_fill_rate"
	AlertManipulation    AlertType = "manipulation"
	AlertUnusualPatterns AlertType = "unusual_patterns"
)

type Alert struct {
	Type      AlertType
	Market    string
	User      string
	OrderType string
	Value     float64
	Threshold float64
	Details   string
	Timestamp time.Time
}

type AlertConfig struct {
	SlippageThreshold  float64
	FillRateThreshold  float64
	ManipulationConfig map[string]interface{}
	PatternConfig      map[string]interface{}
}

type AlertingService struct {
	Config AlertConfig
	Alerts []Alert
}

func NewAlertingService(cfg AlertConfig) *AlertingService {
	return &AlertingService{Config: cfg, Alerts: make([]Alert, 0, 1000)}
}

func (as *AlertingService) Raise(alert Alert) {
	as.Alerts = append(as.Alerts, alert)
	// TODO: send to external system, webhook, or notification
}

// ... Methods for checking thresholds and generating alerts will be implemented next ...
