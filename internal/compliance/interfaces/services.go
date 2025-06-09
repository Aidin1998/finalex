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
	CheckCompliance(ctx context.Context, request *ComplianceRequest) (*ComplianceResult, error)
	CheckTransaction(ctx context.Context, transactionID string) (*ComplianceResult, error)
	ValidateTransaction(ctx context.Context, userID uuid.UUID, txType string, amount decimal.Decimal, currency string, metadata map[string]interface{}) (*ComplianceResult, error)

	// User management
	GetUserStatus(ctx context.Context, userID uuid.UUID) (*UserComplianceStatus, error)
	GetUserHistory(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*ComplianceEvent, error)

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

	// External reporting
	ProcessExternalReport(ctx context.Context, report *ExternalComplianceReport) error

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
	GetEvent(ctx context.Context, eventID uuid.UUID) (*AuditEvent, error)
	GetEventsByUser(ctx context.Context, userID uuid.UUID, from, to time.Time) ([]*AuditEvent, error)
	GetEventsByType(ctx context.Context, eventType string, from, to time.Time) ([]*AuditEvent, error)
	CreateEvent(ctx context.Context, event *AuditEvent) error

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

// ManipulationService provides market manipulation detection and investigation management
type ManipulationService interface {
	// Detection
	DetectManipulation(ctx context.Context, request ManipulationRequest) (*ManipulationResult, error)

	// Alert management
	GetAlerts(ctx context.Context, filter *AlertFilter) ([]*ManipulationAlert, error)
	GetAlert(ctx context.Context, alertID uuid.UUID) (*ManipulationAlert, error)
	UpdateAlertStatus(ctx context.Context, alertID uuid.UUID, status AlertStatus) error

	// Investigation management
	GetInvestigations(ctx context.Context, filter *InvestigationFilter) ([]*Investigation, error)
	CreateInvestigation(ctx context.Context, request *CreateInvestigationRequest) (*Investigation, error)
	UpdateInvestigation(ctx context.Context, investigationID uuid.UUID, updates *InvestigationUpdate) error

	// Pattern analysis
	GetPatterns(ctx context.Context, filter *PatternFilter) ([]*ManipulationPattern, error)

	// Configuration
	GetConfig(ctx context.Context) (*ManipulationConfig, error)
	UpdateConfig(ctx context.Context, config *ManipulationConfig) error

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
	GetAlerts(ctx context.Context, filter *AlertFilter) ([]*MonitoringAlert, error)
	GetAlert(ctx context.Context, alertID uuid.UUID) (*MonitoringAlert, error)
	UpdateAlertStatus(ctx context.Context, alertID uuid.UUID, status string) error
	GenerateAlert(ctx context.Context, alert *MonitoringAlert) error

	// Dashboard and metrics
	GetDashboard(ctx context.Context) (*MonitoringDashboard, error)
	GetMetrics(ctx context.Context) (*MonitoringMetrics, error)
	GenerateReport(ctx context.Context, reportType string, from, to time.Time) ([]byte, error)

	// Policy management
	GetPolicies(ctx context.Context) ([]*MonitoringPolicy, error)
	CreatePolicy(ctx context.Context, policy *MonitoringPolicy) error
	UpdatePolicy(ctx context.Context, policyID string, policy *MonitoringPolicy) error
	DeletePolicy(ctx context.Context, policyID string) error
	// Configuration
	UpdateMonitoringRules(ctx context.Context, rules []interface{}) error
	GetConfiguration(ctx context.Context) (map[string]interface{}, error)

	// Service lifecycle
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	HealthCheck(ctx context.Context) error
	CheckHealth(ctx context.Context) (*ServiceHealth, error)
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

// Handler interfaces
type AlertHandler interface {
	HandleAlert(ctx context.Context, alert *MonitoringAlert) error
}

type EventHandler interface {
	HandleEvent(ctx context.Context, event interface{}) error
}
