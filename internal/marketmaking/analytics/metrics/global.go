package metrics

var (
	BusinessMetricsInstance   = NewBusinessMetrics()
	AlertingServiceInstance   = NewAlertingService(AlertConfig{})
	ComplianceServiceInstance = NewComplianceService()
)
