package compliance

import (
	"context"
	"time"

	"github.com/Aidin1998/finalex/internal/compliance/audit"
	"github.com/Aidin1998/finalex/internal/compliance/compliance"
	"github.com/Aidin1998/finalex/internal/compliance/hooks"
	"github.com/Aidin1998/finalex/internal/compliance/interfaces"
	"github.com/Aidin1998/finalex/internal/compliance/monitoring"
	"github.com/Aidin1998/finalex/internal/compliance/observability"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// ServiceFactory creates and manages all compliance services
type ServiceFactory struct {
	config *Config
	db     *gorm.DB
	logger *zap.Logger
}

// NewServiceFactory creates a new service factory
func NewServiceFactory(config *Config, db *gorm.DB, logger *zap.Logger) *ServiceFactory {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &ServiceFactory{
		config: config,
		db:     db,
		logger: logger,
	}
}

// CreateComplianceModule creates the complete compliance module with all services
func (f *ServiceFactory) CreateComplianceModule(ctx context.Context) (*ComplianceModule, error) {
	// Create observability manager first
	observabilityMgr, err := f.createObservabilityManager()
	if err != nil {
		return nil, err
	}

	// Create audit service
	auditSvc, err := f.createAuditService()
	if err != nil {
		return nil, err
	}

	// Create compliance service
	complianceSvc, err := f.createComplianceService(auditSvc)
	if err != nil {
		return nil, err
	}

	// Create monitoring service
	monitoringSvc, err := f.createMonitoringService(auditSvc)
	if err != nil {
		return nil, err
	}

	// Create hook manager (manipulation/risk = nil for now)
	hookMgr, err := f.createHookManager(complianceSvc, auditSvc, monitoringSvc)
	if err != nil {
		return nil, err
	}

	return &ComplianceModule{
		Config:               nil, // Set config if available
		AuditService:         auditSvc,
		ComplianceService:    complianceSvc,
		MonitoringService:    monitoringSvc,
		HookManager:          hookMgr,
		ObservabilityManager: observabilityMgr,
	}, nil
}

// createAuditService creates the audit service
func (f *ServiceFactory) createAuditService() (interfaces.AuditService, error) {
	// Create audit configuration
	auditConfig := &audit.Config{
		BatchSize:            f.config.Audit.BatchSize,
		BatchTimeout:         f.config.Audit.FlushInterval,
		WorkerCount:          f.config.Audit.Workers,
		QueueSize:            10000,
		EnableEncryption:     f.config.Audit.EnableEncryption,
		EnableCompression:    f.config.Audit.EnableCompression,
		VerificationInterval: time.Hour,
		RetentionDays:        f.config.Audit.RetentionDays,
		EnableMetrics:        true,
	}

	auditSvc, err := audit.NewService(f.db, f.logger, auditConfig)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create audit service")
	}

	return auditSvc, nil
}

// createComplianceService creates the compliance service
func (f *ServiceFactory) createComplianceService(auditSvc interfaces.AuditService) (interfaces.ComplianceService, error) {
	complianceSvc := compliance.NewComplianceService(f.db, auditSvc)

	return complianceSvc, nil
}

// createMonitoringService creates the monitoring service
func (f *ServiceFactory) createMonitoringService(auditSvc interfaces.AuditService) (interfaces.MonitoringService, error) {
	monitoringSvc := monitoring.NewMonitoringService(
		f.db,
		auditSvc,
		f.config.Monitoring.Workers,
	)

	return monitoringSvc, nil
}

// createObservabilityManager creates the observability manager
func (f *ServiceFactory) createObservabilityManager() (*observability.ObservabilityManager, error) {
	return observability.NewObservabilityManager()
}

// createHookManager creates the hook manager (created later after all services are ready)
func (f *ServiceFactory) createHookManager(
	complianceService interfaces.ComplianceService,
	auditService interfaces.AuditService,
	monitoringService interfaces.MonitoringService,
) (*hooks.HookManager, error) {
	config := &hooks.HookConfig{
		EnableRealTimeHooks: true,
		AsyncProcessing:     true,
		BatchSize:           100,
		FlushInterval:       time.Second,
		TimeoutDuration:     30 * time.Second,
		RetryAttempts:       3,
		EnableMetrics:       true,
	}

	// For now, we'll pass nil for manipulation/risk service since it's not in the interfaces yet
	return hooks.NewHookManager(
		complianceService,
		auditService,
		monitoringService,
		nil, // Manipulation service - we'll integrate this later
		nil, // Risk service - we'll integrate this later
		config,
	), nil
}

// ComplianceModule represents the complete compliance module
type ComplianceModule struct {
	Config               *Config
	AuditService         interfaces.AuditService
	ComplianceService    interfaces.ComplianceService
	MonitoringService    interfaces.MonitoringService
	HookManager          *hooks.HookManager
	ObservabilityManager *observability.ObservabilityManager
}

// Start initializes the entire compliance module
func (m *ComplianceModule) Start(ctx context.Context) error {
	return nil
}

// Stop gracefully shuts down the compliance module
func (m *ComplianceModule) Stop() error {
	return nil
}

// ProcessComplianceRequest is the main entry point for compliance checking
func (m *ComplianceModule) ProcessComplianceRequest(ctx context.Context, request interfaces.ComplianceRequest) (*interfaces.ComplianceResult, error) {
	return nil, nil
}

// GetSystemHealth returns the health status of the compliance module
func (m *ComplianceModule) GetSystemHealth(ctx context.Context) (*interfaces.SystemHealth, error) {
	return nil, nil
}

// GetMetrics returns compliance metrics
func (m *ComplianceModule) GetMetrics(ctx context.Context) (*interfaces.ComplianceMetrics, error) {
	return nil, nil
}

// GetAuditService returns the audit service
func (m *ComplianceModule) GetAuditService() interfaces.AuditService {
	return m.AuditService
}

// GetComplianceService returns the compliance service
func (m *ComplianceModule) GetComplianceService() interfaces.ComplianceService {
	return m.ComplianceService
}

// GetMonitoringService returns the monitoring service
func (m *ComplianceModule) GetMonitoringService() interfaces.MonitoringService {
	return m.MonitoringService
}

// Helper function to create a compliance module with default configuration
func NewDefaultComplianceModule(db *gorm.DB) (*ComplianceModule, error) {
	config := DefaultConfig()
	if err := config.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid default configuration")
	}

	factory := NewServiceFactory(config, db, nil)
	ctx := context.Background()

	return factory.CreateComplianceModule(ctx)
}

// Helper function to create a compliance module with custom configuration
func NewComplianceModuleWithConfig(config *Config, db *gorm.DB) (*ComplianceModule, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid configuration")
	}

	factory := NewServiceFactory(config, db, nil)
	ctx := context.Background()

	return factory.CreateComplianceModule(ctx)
}
