package monitoring

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// PrometheusMetrics contains all Prometheus metrics for compliance monitoring
type PrometheusMetrics struct {
	// Alert metrics
	AlertsGenerated     prometheus.CounterVec
	AlertsProcessed     prometheus.CounterVec
	AlertProcessingTime prometheus.HistogramVec
	AlertsResolved      prometheus.CounterVec

	// Manipulation detection metrics
	ManipulationDetections prometheus.CounterVec
	ManipulationConfidence prometheus.HistogramVec
	ManipulationPatterns   prometheus.CounterVec

	// Policy metrics
	PolicyEvaluations   prometheus.CounterVec
	PolicyViolations    prometheus.CounterVec
	PolicyUpdateLatency prometheus.HistogramVec

	// System health metrics
	ServiceHealth       prometheus.GaugeVec
	DatabaseConnections prometheus.GaugeVec
	QueueSize           prometheus.GaugeVec
	ProcessingLatency   prometheus.HistogramVec

	// User activity metrics
	UserRiskScores      prometheus.HistogramVec
	ComplianceChecks    prometheus.CounterVec
	BlockedTransactions prometheus.CounterVec

	// Investigation metrics
	InvestigationsOpened   prometheus.CounterVec
	InvestigationsDuration prometheus.HistogramVec
	ActionsExecuted        prometheus.CounterVec
}

// NewPrometheusMetrics creates and registers all Prometheus metrics
func NewPrometheusMetrics() *PrometheusMetrics {
	return &PrometheusMetrics{
		// Alert metrics
		AlertsGenerated: *promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "compliance",
				Subsystem: "monitoring",
				Name:      "alerts_generated_total",
				Help:      "Total number of alerts generated",
			},
			[]string{"alert_type", "severity", "market"},
		),

		AlertsProcessed: *promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "compliance",
				Subsystem: "monitoring",
				Name:      "alerts_processed_total",
				Help:      "Total number of alerts processed",
			},
			[]string{"alert_type", "status", "market"},
		),

		AlertProcessingTime: *promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "compliance",
				Subsystem: "monitoring",
				Name:      "alert_processing_duration_seconds",
				Help:      "Time taken to process alerts",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"alert_type", "market"},
		),

		AlertsResolved: *promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "compliance",
				Subsystem: "monitoring",
				Name:      "alerts_resolved_total",
				Help:      "Total number of alerts resolved",
			},
			[]string{"alert_type", "resolution_type", "market"},
		),

		// Manipulation detection metrics
		ManipulationDetections: *promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "compliance",
				Subsystem: "manipulation",
				Name:      "detections_total",
				Help:      "Total number of manipulation detections",
			},
			[]string{"pattern_type", "market", "severity"},
		),

		ManipulationConfidence: *promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "compliance",
				Subsystem: "manipulation",
				Name:      "detection_confidence",
				Help:      "Confidence score of manipulation detections",
				Buckets:   []float64{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0},
			},
			[]string{"pattern_type", "market"},
		),

		ManipulationPatterns: *promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "compliance",
				Subsystem: "manipulation",
				Name:      "patterns_detected_total",
				Help:      "Total number of manipulation patterns detected",
			},
			[]string{"pattern_type", "market"},
		),

		// Policy metrics
		PolicyEvaluations: *promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "compliance",
				Subsystem: "policy",
				Name:      "evaluations_total",
				Help:      "Total number of policy evaluations",
			},
			[]string{"policy_type", "result"},
		),

		PolicyViolations: *promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "compliance",
				Subsystem: "policy",
				Name:      "violations_total",
				Help:      "Total number of policy violations",
			},
			[]string{"policy_type", "severity"},
		),

		PolicyUpdateLatency: *promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "compliance",
				Subsystem: "policy",
				Name:      "update_duration_seconds",
				Help:      "Time taken to update policies",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"policy_type"},
		),

		// System health metrics
		ServiceHealth: *promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "compliance",
				Subsystem: "system",
				Name:      "service_health",
				Help:      "Health status of compliance services (1=healthy, 0=unhealthy)",
			},
			[]string{"service_name", "component"},
		),

		DatabaseConnections: *promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "compliance",
				Subsystem: "system",
				Name:      "database_connections",
				Help:      "Number of active database connections",
			},
			[]string{"database", "status"},
		),

		QueueSize: *promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "compliance",
				Subsystem: "system",
				Name:      "queue_size",
				Help:      "Current size of processing queues",
			},
			[]string{"queue_type"},
		),

		ProcessingLatency: *promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "compliance",
				Subsystem: "system",
				Name:      "processing_latency_seconds",
				Help:      "Processing latency for various operations",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"operation", "component"},
		),

		// User activity metrics
		UserRiskScores: *promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "compliance",
				Subsystem: "user",
				Name:      "risk_scores",
				Help:      "Distribution of user risk scores",
				Buckets:   []float64{0.0, 0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0},
			},
			[]string{"risk_category", "market"},
		),

		ComplianceChecks: *promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "compliance",
				Subsystem: "user",
				Name:      "checks_total",
				Help:      "Total number of compliance checks performed",
			},
			[]string{"check_type", "result"},
		),

		BlockedTransactions: *promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "compliance",
				Subsystem: "user",
				Name:      "blocked_transactions_total",
				Help:      "Total number of blocked transactions",
			},
			[]string{"reason", "market"},
		),

		// Investigation metrics
		InvestigationsOpened: *promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "compliance",
				Subsystem: "investigation",
				Name:      "opened_total",
				Help:      "Total number of investigations opened",
			},
			[]string{"investigation_type", "priority"},
		),

		InvestigationsDuration: *promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "compliance",
				Subsystem: "investigation",
				Name:      "duration_seconds",
				Help:      "Duration of completed investigations",
				Buckets:   []float64{3600, 86400, 259200, 604800, 1209600, 2592000}, // 1h, 1d, 3d, 1w, 2w, 1m
			},
			[]string{"investigation_type", "outcome"},
		),

		ActionsExecuted: *promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "compliance",
				Subsystem: "investigation",
				Name:      "actions_executed_total",
				Help:      "Total number of compliance actions executed",
			},
			[]string{"action_type", "result"},
		),
	}
}

// RecordAlertGenerated records when an alert is generated
func (pm *PrometheusMetrics) RecordAlertGenerated(alertType, severity, market string) {
	pm.AlertsGenerated.WithLabelValues(alertType, severity, market).Inc()
}

// RecordAlertProcessed records when an alert is processed
func (pm *PrometheusMetrics) RecordAlertProcessed(alertType, status, market string, duration float64) {
	pm.AlertsProcessed.WithLabelValues(alertType, status, market).Inc()
	pm.AlertProcessingTime.WithLabelValues(alertType, market).Observe(duration)
}

// RecordAlertResolved records when an alert is resolved
func (pm *PrometheusMetrics) RecordAlertResolved(alertType, resolutionType, market string) {
	pm.AlertsResolved.WithLabelValues(alertType, resolutionType, market).Inc()
}

// RecordManipulationDetection records manipulation detection
func (pm *PrometheusMetrics) RecordManipulationDetection(patternType, market, severity string, confidence float64) {
	pm.ManipulationDetections.WithLabelValues(patternType, market, severity).Inc()
	pm.ManipulationConfidence.WithLabelValues(patternType, market).Observe(confidence)
	pm.ManipulationPatterns.WithLabelValues(patternType, market).Inc()
}

// RecordPolicyEvaluation records policy evaluation
func (pm *PrometheusMetrics) RecordPolicyEvaluation(policyType, result string) {
	pm.PolicyEvaluations.WithLabelValues(policyType, result).Inc()
}

// RecordPolicyViolation records policy violation
func (pm *PrometheusMetrics) RecordPolicyViolation(policyType, severity string) {
	pm.PolicyViolations.WithLabelValues(policyType, severity).Inc()
}

// RecordPolicyUpdate records policy update with latency
func (pm *PrometheusMetrics) RecordPolicyUpdate(policyType string, duration float64) {
	pm.PolicyUpdateLatency.WithLabelValues(policyType).Observe(duration)
}

// SetServiceHealth sets service health status
func (pm *PrometheusMetrics) SetServiceHealth(serviceName, component string, healthy bool) {
	value := 0.0
	if healthy {
		value = 1.0
	}
	pm.ServiceHealth.WithLabelValues(serviceName, component).Set(value)
}

// SetDatabaseConnections sets database connection count
func (pm *PrometheusMetrics) SetDatabaseConnections(database, status string, count float64) {
	pm.DatabaseConnections.WithLabelValues(database, status).Set(count)
}

// SetQueueSize sets queue size
func (pm *PrometheusMetrics) SetQueueSize(queueType string, size float64) {
	pm.QueueSize.WithLabelValues(queueType).Set(size)
}

// RecordProcessingLatency records processing latency
func (pm *PrometheusMetrics) RecordProcessingLatency(operation, component string, duration float64) {
	pm.ProcessingLatency.WithLabelValues(operation, component).Observe(duration)
}

// RecordUserRiskScore records user risk score
func (pm *PrometheusMetrics) RecordUserRiskScore(riskCategory, market string, score float64) {
	pm.UserRiskScores.WithLabelValues(riskCategory, market).Observe(score)
}

// RecordComplianceCheck records compliance check
func (pm *PrometheusMetrics) RecordComplianceCheck(checkType, result string) {
	pm.ComplianceChecks.WithLabelValues(checkType, result).Inc()
}

// RecordBlockedTransaction records blocked transaction
func (pm *PrometheusMetrics) RecordBlockedTransaction(reason, market string) {
	pm.BlockedTransactions.WithLabelValues(reason, market).Inc()
}

// RecordInvestigationOpened records investigation opened
func (pm *PrometheusMetrics) RecordInvestigationOpened(investigationType, priority string) {
	pm.InvestigationsOpened.WithLabelValues(investigationType, priority).Inc()
}

// RecordInvestigationCompleted records investigation completion
func (pm *PrometheusMetrics) RecordInvestigationCompleted(investigationType, outcome string, duration float64) {
	pm.InvestigationsDuration.WithLabelValues(investigationType, outcome).Observe(duration)
}

// RecordActionExecuted records action execution
func (pm *PrometheusMetrics) RecordActionExecuted(actionType, result string) {
	pm.ActionsExecuted.WithLabelValues(actionType, result).Inc()
}
