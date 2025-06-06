package compliance

import (
	"context"

	"github.com/Aidin1998/finalex/internal/compliance/audit"
	"github.com/Aidin1998/finalex/internal/compliance/compliance"
	"github.com/Aidin1998/finalex/internal/compliance/interfaces"
	"github.com/Aidin1998/finalex/internal/compliance/manipulation"
	"github.com/Aidin1998/finalex/internal/compliance/monitoring"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

// ServiceFactory creates and manages all compliance services
type ServiceFactory struct {
	config *Config
	db     *gorm.DB
}

// NewServiceFactory creates a new service factory
func NewServiceFactory(config *Config, db *gorm.DB) *ServiceFactory {
	return &ServiceFactory{
		config: config,
		db:     db,
	}
}

// CreateComplianceModule creates the complete compliance module with all services
func (f *ServiceFactory) CreateComplianceModule(ctx context.Context) (*ComplianceModule, error) {
	// Create audit service
	auditSvc, err := f.createAuditService()
	if err != nil {
		return nil, errors.Wrap(err, "failed to create audit service")
	}

	// Create compliance service
	complianceSvc, err := f.createComplianceService(auditSvc)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create compliance service")
	}

	// Create manipulation detection service
	manipulationSvc, err := f.createManipulationService(auditSvc)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create manipulation service")
	}

	// Create monitoring service
	monitoringSvc, err := f.createMonitoringService(auditSvc)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create monitoring service")
	}

	// Create orchestration service
	orchestrationSvc := NewOrchestrationService(
		f.db,
		auditSvc,
		complianceSvc,
		manipulationSvc,
		monitoringSvc,
	)

	return &ComplianceModule{
		Config:               f.config,
		AuditService:         auditSvc,
		ComplianceService:    complianceSvc,
		ManipulationService:  manipulationSvc,
		MonitoringService:    monitoringSvc,
		OrchestrationService: orchestrationSvc,
	}, nil
}

// createAuditService creates the audit service
func (f *ServiceFactory) createAuditService() (interfaces.AuditService, error) {
	auditSvc := audit.NewAuditService(
		f.db,
		f.config.Audit.Workers,
		f.config.Audit.BatchSize,
		f.config.Audit.FlushInterval,
	)

	// Configure encryption and compression
	if f.config.Audit.EnableEncryption {
		// In production, this would use proper key management
		if err := auditSvc.SetEncryptionKey("your-32-byte-encryption-key-here"); err != nil {
			return nil, errors.Wrap(err, "failed to set encryption key")
		}
	}

	auditSvc.SetCompressionEnabled(f.config.Audit.EnableCompression)
	auditSvc.SetChainVerificationEnabled(f.config.Audit.VerifyChain)

	return auditSvc, nil
}

// createComplianceService creates the compliance service
func (f *ServiceFactory) createComplianceService(auditSvc interfaces.AuditService) (interfaces.ComplianceService, error) {
	complianceSvc := compliance.NewComplianceService(f.db, auditSvc)

	// Configure compliance settings
	config := compliance.ServiceConfig{
		KYCEnabled:           f.config.Compliance.EnableKYC,
		AMLEnabled:           f.config.Compliance.EnableAML,
		SanctionsEnabled:     f.config.Compliance.EnableSanctions,
		RequiredKYCLevel:     f.config.Compliance.KYCRequiredLevel,
		AMLRiskThreshold:     f.config.Compliance.AMLRiskThreshold,
		CacheSize:            f.config.Compliance.CacheSize,
		CacheTTL:             f.config.Compliance.CacheTTL,
		PolicyUpdateInterval: f.config.Compliance.PolicyUpdateInterval,
	}

	if err := complianceSvc.Configure(config); err != nil {
		return nil, errors.Wrap(err, "failed to configure compliance service")
	}

	return complianceSvc, nil
}

// createManipulationService creates the manipulation detection service
func (f *ServiceFactory) createManipulationService(auditSvc interfaces.AuditService) (interfaces.ManipulationDetectionService, error) {
	manipulationSvc := manipulation.NewEnhancedManipulationService(f.db, auditSvc)

	// Configure manipulation detection
	config := manipulation.ServiceConfig{
		Enabled:           f.config.Manipulation.EnableDetection,
		DetectionInterval: f.config.Manipulation.DetectionInterval,
		RiskThreshold:     f.config.Manipulation.RiskThreshold,
		AlertThreshold:    f.config.Manipulation.AlertThreshold,
		LookbackPeriod:    f.config.Manipulation.LookbackPeriod,
		MaxPatterns:       f.config.Manipulation.MaxPatterns,
		MLEnabled:         f.config.Manipulation.EnableML,
		MLModelPath:       f.config.Manipulation.MLModelPath,
	}

	if err := manipulationSvc.Configure(config); err != nil {
		return nil, errors.Wrap(err, "failed to configure manipulation service")
	}

	return manipulationSvc, nil
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

// ComplianceModule represents the complete compliance module
type ComplianceModule struct {
	Config               *Config
	AuditService         interfaces.AuditService
	ComplianceService    interfaces.ComplianceService
	ManipulationService  interfaces.ManipulationDetectionService
	MonitoringService    interfaces.MonitoringService
	OrchestrationService *OrchestrationService
}

// Start initializes the entire compliance module
func (m *ComplianceModule) Start(ctx context.Context) error {
	// Migrate database tables
	if err := m.migrateDatabase(); err != nil {
		return errors.Wrap(err, "failed to migrate database")
	}

	// Start orchestration service (which starts all other services)
	if err := m.OrchestrationService.Start(ctx); err != nil {
		return errors.Wrap(err, "failed to start orchestration service")
	}

	return nil
}

// Stop gracefully shuts down the compliance module
func (m *ComplianceModule) Stop() error {
	return m.OrchestrationService.Stop()
}

// ProcessComplianceRequest is the main entry point for compliance checking
func (m *ComplianceModule) ProcessComplianceRequest(ctx context.Context, request interfaces.ComplianceRequest) (*interfaces.ComplianceResult, error) {
	return m.OrchestrationService.ProcessComplianceRequest(ctx, request)
}

// GetSystemHealth returns the health status of the compliance module
func (m *ComplianceModule) GetSystemHealth(ctx context.Context) (*interfaces.SystemHealth, error) {
	return m.OrchestrationService.GetSystemHealth(ctx)
}

// GetMetrics returns compliance metrics
func (m *ComplianceModule) GetMetrics(ctx context.Context) (*interfaces.ComplianceMetrics, error) {
	return m.OrchestrationService.GetComplianceMetrics(ctx)
}

// migrateDatabase creates necessary database tables
func (m *ComplianceModule) migrateDatabase() error {
	// Audit tables
	if err := m.AuditService.(*audit.AuditService).MigrateTables(); err != nil {
		return errors.Wrap(err, "failed to migrate audit tables")
	}

	// Compliance tables
	if err := m.ComplianceService.(*compliance.ComplianceService).MigrateTables(); err != nil {
		return errors.Wrap(err, "failed to migrate compliance tables")
	}

	// Manipulation detection tables
	if err := m.ManipulationService.(*manipulation.EnhancedManipulationService).MigrateTables(); err != nil {
		return errors.Wrap(err, "failed to migrate manipulation tables")
	}

	// Monitoring tables
	tables := []interface{}{
		&monitoring.MonitoringAlertModel{},
		&monitoring.MonitoringPolicyModel{},
		&monitoring.MonitoringMetricModel{},
		&monitoring.MonitoringSubscriptionModel{},
		&monitoring.MonitoringDashboardModel{},
		&monitoring.MonitoringThresholdModel{},
	}

	for _, table := range tables {
		if err := m.OrchestrationService.db.AutoMigrate(table); err != nil {
			return errors.Wrapf(err, "failed to migrate table %T", table)
		}
	}

	return nil
}

// GetAuditService returns the audit service
func (m *ComplianceModule) GetAuditService() interfaces.AuditService {
	return m.AuditService
}

// GetComplianceService returns the compliance service
func (m *ComplianceModule) GetComplianceService() interfaces.ComplianceService {
	return m.ComplianceService
}

// GetManipulationService returns the manipulation detection service
func (m *ComplianceModule) GetManipulationService() interfaces.ManipulationDetectionService {
	return m.ManipulationService
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

	factory := NewServiceFactory(config, db)
	ctx := context.Background()

	return factory.CreateComplianceModule(ctx)
}

// Helper function to create a compliance module with custom configuration
func NewComplianceModuleWithConfig(config *Config, db *gorm.DB) (*ComplianceModule, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid configuration")
	}

	factory := NewServiceFactory(config, db)
	ctx := context.Background()

	return factory.CreateComplianceModule(ctx)
}
