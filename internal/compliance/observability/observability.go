// Package observability provides observability integration for the compliance module
package observability

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// Metrics defines Prometheus metrics for compliance monitoring
type Metrics struct {
	// Compliance metrics
	ComplianceChecksTotal   prometheus.CounterVec
	ComplianceCheckDuration prometheus.HistogramVec
	ComplianceViolations    prometheus.CounterVec
	HighRiskUsersGauge      prometheus.Gauge

	// Audit metrics
	AuditEventsTotal         prometheus.CounterVec
	AuditEventProcessingTime prometheus.HistogramVec
	AuditChainIntegrityGauge prometheus.Gauge
	AuditStorageUsageGauge   prometheus.Gauge

	// Monitoring metrics
	AlertsGenerated   prometheus.CounterVec
	AlertsResolved    prometheus.CounterVec
	ActiveAlertsGauge prometheus.GaugeVec
	MonitoringLatency prometheus.HistogramVec

	// Manipulation detection metrics
	ManipulationDetections prometheus.CounterVec
	ManipulationPatterns   prometheus.CounterVec
	InvestigationsTotal    prometheus.CounterVec

	// Risk assessment metrics
	RiskAssessments       prometheus.CounterVec
	RiskScoreHistogram    prometheus.HistogramVec
	RiskLevelDistribution prometheus.GaugeVec
}

// NewMetrics creates and registers Prometheus metrics
func NewMetrics() *Metrics {
	return &Metrics{
		// Compliance metrics
		ComplianceChecksTotal: *promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "compliance_checks_total",
				Help: "Total number of compliance checks performed",
			},
			[]string{"status", "risk_level", "activity_type"},
		),
		ComplianceCheckDuration: *promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "compliance_check_duration_seconds",
				Help:    "Duration of compliance checks",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"activity_type"},
		),
		ComplianceViolations: *promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "compliance_violations_total",
				Help: "Total number of compliance violations detected",
			},
			[]string{"violation_type", "severity"},
		),
		HighRiskUsersGauge: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "compliance_high_risk_users",
				Help: "Number of users with high risk level",
			},
		),

		// Audit metrics
		AuditEventsTotal: *promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "audit_events_total",
				Help: "Total number of audit events processed",
			},
			[]string{"event_type", "category", "severity"},
		),
		AuditEventProcessingTime: *promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "audit_event_processing_duration_seconds",
				Help:    "Time taken to process audit events",
				Buckets: []float64{0.001, 0.01, 0.1, 1, 10},
			},
			[]string{"event_type"},
		),
		AuditChainIntegrityGauge: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "audit_chain_integrity",
				Help: "Audit chain integrity status (1=valid, 0=invalid)",
			},
		),
		AuditStorageUsageGauge: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "audit_storage_usage_bytes",
				Help: "Storage usage for audit logs in bytes",
			},
		),

		// Monitoring metrics
		AlertsGenerated: *promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "monitoring_alerts_generated_total",
				Help: "Total number of monitoring alerts generated",
			},
			[]string{"alert_type", "severity"},
		),
		AlertsResolved: *promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "monitoring_alerts_resolved_total",
				Help: "Total number of monitoring alerts resolved",
			},
			[]string{"alert_type", "resolution_type"},
		),
		ActiveAlertsGauge: *promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "monitoring_active_alerts",
				Help: "Number of active alerts by type and severity",
			},
			[]string{"alert_type", "severity"},
		),
		MonitoringLatency: *promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "monitoring_processing_latency_seconds",
				Help:    "Latency of monitoring event processing",
				Buckets: []float64{0.001, 0.01, 0.1, 1, 10},
			},
			[]string{"event_type"},
		),

		// Manipulation detection metrics
		ManipulationDetections: *promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "manipulation_detections_total",
				Help: "Total number of manipulation patterns detected",
			},
			[]string{"pattern_type", "confidence_level"},
		),
		ManipulationPatterns: *promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "manipulation_patterns_total",
				Help: "Total number of manipulation patterns analyzed",
			},
			[]string{"pattern_type", "market"},
		),
		InvestigationsTotal: *promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "investigations_total",
				Help: "Total number of investigations created",
			},
			[]string{"investigation_type", "priority", "status"},
		),

		// Risk assessment metrics
		RiskAssessments: *promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "risk_assessments_total",
				Help: "Total number of risk assessments performed",
			},
			[]string{"assessment_type", "risk_level"},
		),
		RiskScoreHistogram: *promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "risk_score_distribution",
				Help:    "Distribution of risk scores",
				Buckets: []float64{0, 10, 25, 50, 75, 90, 100},
			},
			[]string{"assessment_type"},
		),
		RiskLevelDistribution: *promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "risk_level_distribution",
				Help: "Distribution of users by risk level",
			},
			[]string{"risk_level"},
		),
	}
}

// Logger provides structured logging for compliance operations
type Logger struct {
	logger *zap.Logger
}

// NewLogger creates a new structured logger for compliance
func NewLogger() (*Logger, error) {
	config := zap.NewProductionConfig()
	config.Level = zap.NewAtomicLevelAt(zap.InfoLevel)

	// Add compliance-specific fields
	config.InitialFields = map[string]interface{}{
		"service": "compliance",
		"module":  "finalex",
	}

	logger, err := config.Build()
	if err != nil {
		return nil, err
	}

	return &Logger{logger: logger}, nil
}

// Tracer provides distributed tracing for compliance operations
type Tracer struct {
	tracer trace.Tracer
}

// NewTracer creates a new tracer for compliance operations
func NewTracer() *Tracer {
	tracer := otel.Tracer("compliance")
	return &Tracer{tracer: tracer}
}

// ObservabilityManager manages all observability components
type ObservabilityManager struct {
	metrics *Metrics
	logger  *Logger
	tracer  *Tracer
}

// NewObservabilityManager creates a new observability manager
func NewObservabilityManager() (*ObservabilityManager, error) {
	logger, err := NewLogger()
	if err != nil {
		return nil, err
	}

	return &ObservabilityManager{
		metrics: NewMetrics(),
		logger:  logger,
		tracer:  NewTracer(),
	}, nil
}

// Compliance operation tracking methods

// TrackComplianceCheck tracks a compliance check operation
func (om *ObservabilityManager) TrackComplianceCheck(ctx context.Context, activityType, status, riskLevel string, duration time.Duration) {
	ctx, span := om.tracer.tracer.Start(ctx, "compliance_check")
	defer span.End()

	span.SetAttributes(
		attribute.String("activity_type", activityType),
		attribute.String("status", status),
		attribute.String("risk_level", riskLevel),
		attribute.Float64("duration_ms", float64(duration.Nanoseconds())/1e6),
	)

	om.metrics.ComplianceChecksTotal.WithLabelValues(status, riskLevel, activityType).Inc()
	om.metrics.ComplianceCheckDuration.WithLabelValues(activityType).Observe(duration.Seconds())

	om.logger.logger.Info("Compliance check completed",
		zap.String("activity_type", activityType),
		zap.String("status", status),
		zap.String("risk_level", riskLevel),
		zap.Duration("duration", duration),
	)
}

// TrackComplianceViolation tracks a compliance violation
func (om *ObservabilityManager) TrackComplianceViolation(ctx context.Context, violationType, severity string) {
	ctx, span := om.tracer.tracer.Start(ctx, "compliance_violation")
	defer span.End()

	span.SetAttributes(
		attribute.String("violation_type", violationType),
		attribute.String("severity", severity),
	)

	om.metrics.ComplianceViolations.WithLabelValues(violationType, severity).Inc()

	om.logger.logger.Warn("Compliance violation detected",
		zap.String("violation_type", violationType),
		zap.String("severity", severity),
	)
}

// TrackAuditEvent tracks an audit event
func (om *ObservabilityManager) TrackAuditEvent(ctx context.Context, eventType, category, severity string, processingTime time.Duration) {
	ctx, span := om.tracer.tracer.Start(ctx, "audit_event")
	defer span.End()

	span.SetAttributes(
		attribute.String("event_type", eventType),
		attribute.String("category", category),
		attribute.String("severity", severity),
		attribute.Float64("processing_time_ms", float64(processingTime.Nanoseconds())/1e6),
	)

	om.metrics.AuditEventsTotal.WithLabelValues(eventType, category, severity).Inc()
	om.metrics.AuditEventProcessingTime.WithLabelValues(eventType).Observe(processingTime.Seconds())

	om.logger.logger.Info("Audit event processed",
		zap.String("event_type", eventType),
		zap.String("category", category),
		zap.String("severity", severity),
		zap.Duration("processing_time", processingTime),
	)
}

// TrackAlert tracks monitoring alerts
func (om *ObservabilityManager) TrackAlert(ctx context.Context, alertType, severity string, generated bool) {
	ctx, span := om.tracer.tracer.Start(ctx, "monitoring_alert")
	defer span.End()

	span.SetAttributes(
		attribute.String("alert_type", alertType),
		attribute.String("severity", severity),
		attribute.Bool("generated", generated),
	)

	if generated {
		om.metrics.AlertsGenerated.WithLabelValues(alertType, severity).Inc()
		om.metrics.ActiveAlertsGauge.WithLabelValues(alertType, severity).Inc()

		om.logger.logger.Warn("Alert generated",
			zap.String("alert_type", alertType),
			zap.String("severity", severity),
		)
	}
}

// TrackAlertResolution tracks alert resolution
func (om *ObservabilityManager) TrackAlertResolution(ctx context.Context, alertType, resolutionType string) {
	ctx, span := om.tracer.tracer.Start(ctx, "alert_resolution")
	defer span.End()

	span.SetAttributes(
		attribute.String("alert_type", alertType),
		attribute.String("resolution_type", resolutionType),
	)

	om.metrics.AlertsResolved.WithLabelValues(alertType, resolutionType).Inc()

	om.logger.logger.Info("Alert resolved",
		zap.String("alert_type", alertType),
		zap.String("resolution_type", resolutionType),
	)
}

// TrackManipulationDetection tracks manipulation detection
func (om *ObservabilityManager) TrackManipulationDetection(ctx context.Context, patternType, market, confidenceLevel string) {
	ctx, span := om.tracer.tracer.Start(ctx, "manipulation_detection")
	defer span.End()

	span.SetAttributes(
		attribute.String("pattern_type", patternType),
		attribute.String("market", market),
		attribute.String("confidence_level", confidenceLevel),
	)

	om.metrics.ManipulationDetections.WithLabelValues(patternType, confidenceLevel).Inc()
	om.metrics.ManipulationPatterns.WithLabelValues(patternType, market).Inc()

	om.logger.logger.Warn("Manipulation pattern detected",
		zap.String("pattern_type", patternType),
		zap.String("market", market),
		zap.String("confidence_level", confidenceLevel),
	)
}

// TrackRiskAssessment tracks risk assessment operations
func (om *ObservabilityManager) TrackRiskAssessment(ctx context.Context, assessmentType, riskLevel string, riskScore float64) {
	ctx, span := om.tracer.tracer.Start(ctx, "risk_assessment")
	defer span.End()

	span.SetAttributes(
		attribute.String("assessment_type", assessmentType),
		attribute.String("risk_level", riskLevel),
		attribute.Float64("risk_score", riskScore),
	)

	om.metrics.RiskAssessments.WithLabelValues(assessmentType, riskLevel).Inc()
	om.metrics.RiskScoreHistogram.WithLabelValues(assessmentType).Observe(riskScore)

	om.logger.logger.Info("Risk assessment completed",
		zap.String("assessment_type", assessmentType),
		zap.String("risk_level", riskLevel),
		zap.Float64("risk_score", riskScore),
	)
}

// UpdateMetrics updates gauge metrics with current values
func (om *ObservabilityManager) UpdateMetrics(highRiskUsers int, chainIntegrity bool, storageUsage int64) {
	om.metrics.HighRiskUsersGauge.Set(float64(highRiskUsers))

	if chainIntegrity {
		om.metrics.AuditChainIntegrityGauge.Set(1)
	} else {
		om.metrics.AuditChainIntegrityGauge.Set(0)
	}

	om.metrics.AuditStorageUsageGauge.Set(float64(storageUsage))
}

// GetMetrics returns the metrics instance for external use
func (om *ObservabilityManager) GetMetrics() *Metrics {
	return om.metrics
}

// GetLogger returns the logger instance for external use
func (om *ObservabilityManager) GetLogger() *zap.Logger {
	return om.logger.logger
}

// GetTracer returns the tracer instance for external use
func (om *ObservabilityManager) GetTracer() trace.Tracer {
	return om.tracer.tracer
}
