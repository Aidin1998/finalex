// Package interfaces provides service interfaces for the compliance module
package interfaces

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ComplianceService provides comprehensive compliance management
type ComplianceService interface {
	// Core compliance checks
	PerformComplianceCheck(ctx context.Context, req *ComplianceRequest) (*ComplianceResult, error)
	ValidateTransaction(ctx context.Context, userID uuid.UUID, txType string, amount decimal.Decimal, currency string, metadata map[string]interface{}) (*ComplianceResult, error)

	// Risk assessment
	AssessUserRisk(ctx context.Context, userID uuid.UUID) (*ComplianceResult, error)
	UpdateRiskProfile(ctx context.Context, userID uuid.UUID, riskLevel RiskLevel, riskScore decimal.Decimal, reason string) error

	// KYC management
	InitiateKYC(ctx context.Context, userID uuid.UUID, targetLevel KYCLevel) error
	ValidateKYCLevel(ctx context.Context, userID uuid.UUID, requiredLevel KYCLevel) error
	GetKYCStatus(ctx context.Context, userID uuid.UUID) (KYCLevel, error)

	// AML and sanctions
	PerformAMLCheck(ctx context.Context, userID uuid.UUID, amount decimal.Decimal, currency string) (*ComplianceResult, error)
	CheckSanctionsList(ctx context.Context, name, dateOfBirth, nationality string) (*ComplianceResult, error)

	// Policy management
	UpdatePolicies(ctx context.Context, updates []*PolicyUpdate) error
	GetActivePolicies(ctx context.Context, policyType string) ([]*PolicyUpdate, error)

	// Service lifecycle
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	HealthCheck(ctx context.Context) error
}

// AuditService provides tamper-evident audit logging
type AuditService interface {
	// Event logging
	LogEvent(ctx context.Context, event *AuditEvent) error
	LogBatch(ctx context.Context, events []*AuditEvent) error

	// Querying
	GetEvents(ctx context.Context, filter *AuditFilter) ([]*AuditEvent, error)
	GetEventsByUser(ctx context.Context, userID uuid.UUID, from, to time.Time) ([]*AuditEvent, error)
	GetEventsByType(ctx context.Context, eventType string, from, to time.Time) ([]*AuditEvent, error)

	// Integrity verification
	VerifyChain(ctx context.Context, from, to time.Time) (*ChainVerification, error)
	GetChainHash(ctx context.Context, timestamp time.Time) (string, error)

	// Management
	ArchiveEvents(ctx context.Context, before time.Time) error
	GetMetrics(ctx context.Context) (*AuditMetrics, error)

	// Service lifecycle
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	HealthCheck(ctx context.Context) error
}

// ManipulationDetectionService provides market manipulation detection
type ManipulationDetectionService interface {
	// Detection
	AnalyzeOrderPattern(ctx context.Context, userID string, market string, orders []interface{}) (*ManipulationAlert, error)
	DetectWashTrading(ctx context.Context, trades []interface{}) ([]*ManipulationAlert, error)
	DetectSpoofing(ctx context.Context, orders []interface{}) ([]*ManipulationAlert, error)
	DetectLayering(ctx context.Context, orders []interface{}) ([]*ManipulationAlert, error)
	DetectPumpAndDump(ctx context.Context, market string, timeWindow time.Duration) ([]*ManipulationAlert, error)

	// Alert management
	GetAlerts(ctx context.Context, filter *AlertFilter) ([]*ManipulationAlert, error)
	ResolveAlert(ctx context.Context, alertID uuid.UUID, resolution string, resolvedBy uuid.UUID) error

	// Configuration
	UpdateDetectionRules(ctx context.Context, rules []interface{}) error
	GetDetectionMetrics(ctx context.Context) (map[string]interface{}, error)

	// Service lifecycle
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	HealthCheck(ctx context.Context) error
}

// MonitoringService provides real-time compliance monitoring
type MonitoringService interface {
	// Event processing
	IngestEvent(ctx context.Context, event *ComplianceEvent) error
	IngestBatch(ctx context.Context, events []*ComplianceEvent) error

	// Real-time monitoring
	StartMonitoring(ctx context.Context) error
	StopMonitoring(ctx context.Context) error
	GetMonitoringStatus(ctx context.Context) (*MonitoringStatus, error)

	// Alerting
	RegisterAlertHandler(handler AlertHandler) error
	GetActiveAlerts(ctx context.Context) ([]*MonitoringAlert, error)

	// Metrics and reporting
	GetMetrics(ctx context.Context) (*MonitoringMetrics, error)
	GenerateReport(ctx context.Context, reportType string, from, to time.Time) ([]byte, error)

	// Configuration
	UpdateMonitoringRules(ctx context.Context, rules []interface{}) error
	GetConfiguration(ctx context.Context) (map[string]interface{}, error)

	// Service lifecycle
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	HealthCheck(ctx context.Context) error
}

// OrchestrationService coordinates all compliance services
type OrchestrationService interface {
	// Service coordination
	RegisterService(name string, service interface{}) error
	GetService(name string) (interface{}, error)

	// Workflow management
	ExecuteComplianceWorkflow(ctx context.Context, workflowType string, input map[string]interface{}) (*WorkflowResult, error)
	GetWorkflowStatus(ctx context.Context, workflowID uuid.UUID) (*WorkflowStatus, error)

	// Event routing
	RouteEvent(ctx context.Context, event interface{}) error
	RegisterEventHandler(eventType string, handler EventHandler) error

	// Global configuration
	UpdateGlobalConfiguration(ctx context.Context, config map[string]interface{}) error
	GetGlobalConfiguration(ctx context.Context) (map[string]interface{}, error)

	// Service lifecycle
	StartAll(ctx context.Context) error
	StopAll(ctx context.Context) error
	HealthCheckAll(ctx context.Context) (map[string]error, error)
}

// Supporting types and filters

type AuditFilter struct {
	UserID    *uuid.UUID `json:"user_id,omitempty"`
	EventType string     `json:"event_type,omitempty"`
	Category  string     `json:"category,omitempty"`
	Severity  string     `json:"severity,omitempty"`
	IPAddress string     `json:"ip_address,omitempty"`
	Resource  string     `json:"resource,omitempty"`
	Action    string     `json:"action,omitempty"`
	From      time.Time  `json:"from"`
	To        time.Time  `json:"to"`
	Limit     int        `json:"limit,omitempty"`
	Offset    int        `json:"offset,omitempty"`
}

type ChainVerification struct {
	Valid            bool      `json:"valid"`
	StartHash        string    `json:"start_hash"`
	EndHash          string    `json:"end_hash"`
	EventCount       int64     `json:"event_count"`
	VerificationTime time.Time `json:"verification_time"`
	Errors           []string  `json:"errors,omitempty"`
}

type AuditMetrics struct {
	TotalEvents       int64         `json:"total_events"`
	EventsPerSecond   float64       `json:"events_per_second"`
	ChainIntegrity    bool          `json:"chain_integrity"`
	StorageUsage      int64         `json:"storage_usage_bytes"`
	ProcessingLatency time.Duration `json:"processing_latency"`
	ErrorRate         float64       `json:"error_rate"`
}

type AlertFilter struct {
	UserID    string    `json:"user_id,omitempty"`
	Market    string    `json:"market,omitempty"`
	AlertType string    `json:"alert_type,omitempty"`
	Severity  string    `json:"severity,omitempty"`
	Status    string    `json:"status,omitempty"`
	From      time.Time `json:"from"`
	To        time.Time `json:"to"`
	Limit     int       `json:"limit,omitempty"`
	Offset    int       `json:"offset,omitempty"`
}

type ComplianceEvent struct {
	EventType string                 `json:"event_type"`
	UserID    string                 `json:"user_id"`
	Timestamp time.Time              `json:"timestamp"`
	Amount    *decimal.Decimal       `json:"amount,omitempty"`
	Currency  string                 `json:"currency,omitempty"`
	Market    string                 `json:"market,omitempty"`
	IPAddress string                 `json:"ip_address,omitempty"`
	Details   map[string]interface{} `json:"details"`
}

type MonitoringStatus struct {
	Active         bool                   `json:"active"`
	WorkersActive  int                    `json:"workers_active"`
	QueueDepth     int64                  `json:"queue_depth"`
	ProcessingRate float64                `json:"processing_rate"`
	LastEventTime  time.Time              `json:"last_event_time"`
	ErrorCount     int64                  `json:"error_count"`
	Configuration  map[string]interface{} `json:"configuration"`
}

type MonitoringAlert struct {
	ID             uuid.UUID              `json:"id"`
	Type           string                 `json:"type"`
	Severity       string                 `json:"severity"`
	Message        string                 `json:"message"`
	Data           map[string]interface{} `json:"data"`
	CreatedAt      time.Time              `json:"created_at"`
	ResolvedAt     *time.Time             `json:"resolved_at,omitempty"`
	AcknowledgedAt *time.Time             `json:"acknowledged_at,omitempty"`
}

type WorkflowResult struct {
	WorkflowID  uuid.UUID              `json:"workflow_id"`
	Status      string                 `json:"status"`
	Result      map[string]interface{} `json:"result"`
	Error       string                 `json:"error,omitempty"`
	StartedAt   time.Time              `json:"started_at"`
	CompletedAt *time.Time             `json:"completed_at,omitempty"`
}

type WorkflowStatus struct {
	WorkflowID    uuid.UUID              `json:"workflow_id"`
	Type          string                 `json:"type"`
	Status        string                 `json:"status"`
	CurrentStep   string                 `json:"current_step"`
	Progress      float64                `json:"progress"`
	Input         map[string]interface{} `json:"input"`
	Output        map[string]interface{} `json:"output,omitempty"`
	Error         string                 `json:"error,omitempty"`
	StartedAt     time.Time              `json:"started_at"`
	LastUpdatedAt time.Time              `json:"last_updated_at"`
}

// Handler interfaces
type AlertHandler interface {
	HandleAlert(ctx context.Context, alert *MonitoringAlert) error
}

type EventHandler interface {
	HandleEvent(ctx context.Context, event interface{}) error
}
