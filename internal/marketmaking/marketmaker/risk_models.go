// Risk management models and extensions
package marketmaker

// Define RiskSignal structure
type RiskSignal struct {
	Type     string  `json:"type"`
	Message  string  `json:"message"`
	Severity string  `json:"severity"`
	Symbol   string  `json:"symbol"`
	Value    float64 `json:"value"`
}

// Define RiskStatus structure
type RiskStatus struct {
	DailyPnL      float64      `json:"daily_pnl"`
	TotalExposure float64      `json:"total_exposure"`
	RiskScore     float64      `json:"risk_score"`
	RiskSignals   []RiskSignal `json:"risk_signals"`
	IsHighRisk    bool         `json:"is_high_risk"`
}
