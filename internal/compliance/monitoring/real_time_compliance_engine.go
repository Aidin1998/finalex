// real_time_compliance_engine.go
// Real-Time Compliance Engine: event stream processing, ML anomaly detection, rule evaluation, and alerting.
package monitoring

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/Aidin1998/finalex/internal/compliance/interfaces"
)

// ComplianceEvent represents an incoming event for compliance analysis
// (e.g., account update, transaction, login, withdrawal, etc.)
type ComplianceEvent struct {
	EventType string                 // e.g., "transaction", "login", "withdrawal"
	UserID    string                 // User identifier
	Timestamp time.Time              // Event time
	Amount    float64                // For financial events
	Details   map[string]interface{} // Arbitrary event details
}

// MLAnomalyDetector is an interface for ML-based anomaly detection
// (to be implemented by actual ML/AI modules or adapters)
type MLAnomalyDetector interface {
	DetectAnomaly(event *ComplianceEvent) (isAnomaly bool, score float64, features map[string]interface{})
}

// ComplianceRule defines a compliance rule for evaluation
// (can be extended for more complex logic)
type ComplianceRule interface {
	Evaluate(event *ComplianceEvent) (violation bool, reason string)
}

// ComplianceEngine orchestrates real-time monitoring, ML, rules, and alerting
// Integrates with Service (metrics/alerts) and ConsistencyMonitor
// Thread-safe and high-performance for production use
type ComplianceEngine struct {
	logger        *zap.Logger
	mlDetector    MLAnomalyDetector
	rules         []ComplianceRule
	service       *MonitoringService
	consistency   *ConsistencyMonitor
	alertCallback func(alert *interfaces.MonitoringAlert)
	mu            sync.RWMutex
	started       bool
	ctx           context.Context
	cancel        context.CancelFunc
	eventQueue    chan *ComplianceEvent
	processingWG  sync.WaitGroup
}

// NewComplianceEngine creates a new real-time compliance engine
func NewComplianceEngine(logger *zap.Logger, mlDetector MLAnomalyDetector, rules []ComplianceRule, service *MonitoringService, consistency *ConsistencyMonitor) *ComplianceEngine {
	ctx, cancel := context.WithCancel(context.Background())
	return &ComplianceEngine{
		logger:      logger,
		mlDetector:  mlDetector,
		rules:       rules,
		service:     service,
		consistency: consistency,
		ctx:         ctx,
		cancel:      cancel,
		eventQueue:  make(chan *ComplianceEvent, 10000),
	}
}

// Start begins real-time event processing
func (ce *ComplianceEngine) Start(workerCount int) {
	defaultWorkerCount := 4
	ce.mu.Lock()
	if ce.started {
		ce.mu.Unlock()
		return
	}
	ce.started = true
	ce.mu.Unlock()

	if workerCount <= 0 {
		workerCount = defaultWorkerCount
	}
	for i := 0; i < workerCount; i++ {
		ce.processingWG.Add(1)
		go ce.eventProcessor()
	}
	ce.logger.Info("ComplianceEngine started", zap.Int("workers", workerCount))
}

// Stop halts event processing and waits for workers to finish
func (ce *ComplianceEngine) Stop() {
	ce.mu.Lock()
	if !ce.started {
		ce.mu.Unlock()
		return
	}
	ce.cancel()
	ce.started = false
	ce.mu.Unlock()
	close(ce.eventQueue)
	ce.processingWG.Wait()
	ce.logger.Info("ComplianceEngine stopped")
}

// IngestEvent submits an event for compliance analysis
func (ce *ComplianceEngine) IngestEvent(event *ComplianceEvent) {
	select {
	case ce.eventQueue <- event:
		// ok
	default:
		ce.logger.Warn("ComplianceEngine event queue full, dropping event", zap.String("event_type", event.EventType), zap.String("user_id", event.UserID))
	}
}

// eventProcessor processes events from the queue
func (ce *ComplianceEngine) eventProcessor() {
	defer ce.processingWG.Done()
	for {
		select {
		case <-ce.ctx.Done():
			return
		case event, ok := <-ce.eventQueue:
			if !ok || event == nil {
				return
			}
			ce.processEvent(event)
		}
	}
}

// processEvent runs ML anomaly detection, rule evaluation, and triggers alerts/metrics
func (ce *ComplianceEngine) processEvent(event *ComplianceEvent) {
	isAnomaly := false
	anomalyScore := 0.0
	features := map[string]interface{}{}
	if ce.mlDetector != nil {
		anomaly, score, feats := ce.mlDetector.DetectAnomaly(event)
		isAnomaly = anomaly
		anomalyScore = score
		features = feats
	}

	violations := []string{}
	for _, rule := range ce.rules {
		if violated, reason := rule.Evaluate(event); violated {
			violations = append(violations, reason)
		}
	}
	if isAnomaly || len(violations) > 0 {
		alert := &interfaces.MonitoringAlert{
			ID:        uuid.New(),
			UserID:    event.UserID,
			AlertType: "Compliance Violation",
			Severity:  ce.determineSeverityInterface(isAnomaly, anomalyScore, violations),
			Status:    interfaces.AlertStatusPending,
			Message:   ce.buildAlertDescription(event, isAnomaly, anomalyScore, violations, features),
			Details: map[string]interface{}{
				"event":      event,
				"anomaly":    isAnomaly,
				"score":      anomalyScore,
				"features":   features,
				"violations": violations,
			},
			Timestamp: time.Now().UTC(),
		}

		// Store alert in service (replace with actual service method when available)
		// ce.service.StoreAlert(alert)

		if ce.alertCallback != nil {
			ce.alertCallback(alert)
		}
		ce.logger.Warn("Compliance alert triggered", zap.String("user_id", event.UserID), zap.Strings("violations", violations), zap.Bool("anomaly", isAnomaly), zap.Float64("score", anomalyScore))
	}
	// Metrics and audit trail
	// TODO: Implement metric registration when service supports it
	// ce.service.RegisterMetric("compliance_event", MetricTypeCounter, 1, map[string]string{"event_type": event.EventType})
	if isAnomaly {
		// ce.service.RegisterMetric("compliance_anomaly", MetricTypeCounter, 1, map[string]string{"event_type": event.EventType})
	}
	if len(violations) > 0 {
		// ce.service.RegisterMetric("compliance_violation", MetricTypeCounter, float64(len(violations)), map[string]string{"event_type": event.EventType})
	}
}

// determineSeverity determines alert severity based on anomaly and rule violations
func (ce *ComplianceEngine) determineSeverity(isAnomaly bool, score float64, violations []string) string {
	if isAnomaly && score > 0.95 {
		return "critical"
	}
	if isAnomaly {
		return "high"
	}
	if len(violations) > 2 {
		return "high"
	}
	if len(violations) > 0 {
		return "medium"
	}
	return "info"
}

// determineSeverityInterface determines alert severity based on anomaly and rule violations
func (ce *ComplianceEngine) determineSeverityInterface(isAnomaly bool, score float64, violations []string) interfaces.AlertSeverity {
	if isAnomaly && score > 0.95 {
		return interfaces.AlertSeverityCritical
	}
	if isAnomaly {
		return interfaces.AlertSeverityHigh
	}
	if len(violations) > 2 {
		return interfaces.AlertSeverityHigh
	}
	if len(violations) > 0 {
		return interfaces.AlertSeverityMedium
	}
	return interfaces.AlertSeverityLow
}

// buildAlertDescription builds a human-readable alert description
func (ce *ComplianceEngine) buildAlertDescription(event *ComplianceEvent, isAnomaly bool, score float64, violations []string, features map[string]interface{}) string {
	desc := "Compliance alert: "
	if isAnomaly {
		desc += "ML anomaly detected (score=" + formatFloat(score) + ") "
	}
	if len(violations) > 0 {
		desc += "Rule violations: " + joinStrings(violations, ", ")
	}
	return desc
}

// Utility helpers (implementations omitted for brevity)
func generateAlertID() string      { return time.Now().Format("20060102T150405.000000000") }
func formatFloat(f float64) string { return fmt.Sprintf("%.4f", f) }
func joinStrings(arr []string, sep string) string {
	if len(arr) == 0 {
		return ""
	}
	out := arr[0]
	for i := 1; i < len(arr); i++ {
		out += sep + arr[i]
	}
	return out
}

// SetAlertCallback sets a callback for real-time alert delivery
func (ce *ComplianceEngine) SetAlertCallback(cb func(alert *interfaces.MonitoringAlert)) {
	ce.mu.Lock()
	defer ce.mu.Unlock()
	ce.alertCallback = cb
}

// IsRunning returns true if the engine is running
func (ce *ComplianceEngine) IsRunning() bool {
	ce.mu.RLock()
	defer ce.mu.RUnlock()
	return ce.started
}

// Example MLAnomalyDetector and ComplianceRule implementations would be provided elsewhere.
// Integration with audit trail, advanced ML, and external alerting can be extended as needed.
