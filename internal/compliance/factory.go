package compliance

import (
	"context"
	"fmt"
	"time"

	"github.com/Aidin1998/finalex/internal/compliance/audit"
	"github.com/Aidin1998/finalex/internal/compliance/hooks"
	"github.com/Aidin1998/finalex/internal/compliance/interfaces"
	"github.com/Aidin1998/finalex/internal/compliance/monitoring"
	"github.com/Aidin1998/finalex/internal/compliance/observability"
	"github.com/Aidin1998/finalex/internal/compliance/service"
	userauthCompliance "github.com/Aidin1998/finalex/internal/userauth/compliance"
	"github.com/pkg/errors"
	"github.com/shopspring/decimal"
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
	// Auto-migrate compliance models
	if err := service.AutoMigrate(f.db); err != nil {
		f.logger.Warn("Failed to auto-migrate compliance models", zap.Error(err))
	}

	// Create userauth compliance service with stub dependencies
	geoService := &userauthCompliance.StubGeolocationService{}
	sanctionService := &userauthCompliance.StubSanctionCheckService{}
	kycProvider := &userauthCompliance.StubKYCProvider{}

	userauthComplianceSvc := userauthCompliance.NewComplianceService(
		f.db,
		f.logger,
		geoService,
		sanctionService,
		kycProvider,
	)

	// Create compliance service configuration
	config := &service.Config{
		EnableRealTimeMonitoring: f.config.Compliance.EnableKYC || f.config.Compliance.EnableAML,
		DefaultRiskLevel:         interfaces.RiskLevelMedium,
		RequireKYCThreshold:      decimal.NewFromFloat(1000.0),
		AMLTransactionThreshold:  decimal.NewFromFloat(f.config.Compliance.AMLRiskThreshold * 1000), // Convert to amount
		PolicyRefreshInterval:    f.config.Compliance.PolicyUpdateInterval,
		EventBufferSize:          1000,
	}

	// Create the real compliance service
	complianceSvc := service.NewComplianceService(
		f.db,
		f.logger,
		auditSvc,
		userauthComplianceSvc,
		config,
	)

	return complianceSvc, nil
}

// createMonitoringService creates the monitoring service
func (f *ServiceFactory) createMonitoringService(auditSvc interfaces.AuditService) (interfaces.MonitoringService, error) {
	// Create the real monitoring service
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
	// Start services in dependency order
	if err := m.AuditService.Start(ctx); err != nil {
		return fmt.Errorf("failed to start audit service: %w", err)
	}

	if err := m.ComplianceService.Start(ctx); err != nil {
		return fmt.Errorf("failed to start compliance service: %w", err)
	}

	if err := m.MonitoringService.Start(ctx); err != nil {
		return fmt.Errorf("failed to start monitoring service: %w", err)
	}

	// Start hook manager
	if err := m.HookManager.Start(ctx); err != nil {
		return fmt.Errorf("failed to start hook manager: %w", err)
	}

	// ObservabilityManager doesn't have a Start method - it's ready to use

	return nil
}

// Stop gracefully shuts down the compliance module
func (m *ComplianceModule) Stop() error {
	// Stop services in reverse order (last started, first stopped)

	// Stop hook manager first
	if err := m.HookManager.Stop(context.Background()); err != nil {
		return fmt.Errorf("failed to stop hook manager: %w", err)
	}

	// Stop monitoring service
	if err := m.MonitoringService.Stop(context.Background()); err != nil {
		return fmt.Errorf("failed to stop monitoring service: %w", err)
	}

	// Stop compliance service
	if err := m.ComplianceService.Stop(context.Background()); err != nil {
		return fmt.Errorf("failed to stop compliance service: %w", err)
	}

	// Stop audit service last
	if err := m.AuditService.Stop(context.Background()); err != nil {
		return fmt.Errorf("failed to stop audit service: %w", err)
	}

	// ObservabilityManager doesn't need explicit stopping

	return nil
}

// ProcessComplianceRequest is the main entry point for compliance checking
func (m *ComplianceModule) ProcessComplianceRequest(ctx context.Context, request interfaces.ComplianceRequest) (*interfaces.ComplianceResult, error) {
	// Delegate to the compliance service for processing
	return m.ComplianceService.CheckCompliance(ctx, &request)
}

// GetSystemHealth returns the health status of the compliance module
func (m *ComplianceModule) GetSystemHealth(ctx context.Context) (*interfaces.SystemHealth, error) {
	health := &interfaces.SystemHealth{
		Timestamp: time.Now(),
		Services:  make(map[string]interfaces.ServiceHealth),
		Metrics:   make([]interfaces.MetricSummary, 0),
	}

	overallHealthy := true

	// Check audit service health
	if err := m.AuditService.HealthCheck(ctx); err != nil {
		health.Services["audit"] = interfaces.ServiceHealth{
			ServiceName:  "audit",
			Status:       "unhealthy",
			HealthScore:  0.0,
			LastCheck:    time.Now(),
			ErrorMessage: err.Error(),
			ResponseTime: 0,
		}
		overallHealthy = false
	} else {
		health.Services["audit"] = interfaces.ServiceHealth{
			ServiceName:  "audit",
			Status:       "healthy",
			HealthScore:  1.0,
			LastCheck:    time.Now(),
			ResponseTime: 0,
		}
	}

	// Check compliance service health
	if err := m.ComplianceService.HealthCheck(ctx); err != nil {
		health.Services["compliance"] = interfaces.ServiceHealth{
			ServiceName:  "compliance",
			Status:       "unhealthy",
			HealthScore:  0.0,
			LastCheck:    time.Now(),
			ErrorMessage: err.Error(),
			ResponseTime: 0,
		}
		overallHealthy = false
	} else {
		health.Services["compliance"] = interfaces.ServiceHealth{
			ServiceName:  "compliance",
			Status:       "healthy",
			HealthScore:  1.0,
			LastCheck:    time.Now(),
			ResponseTime: 0,
		}
	}

	// Check monitoring service health
	if err := m.MonitoringService.HealthCheck(ctx); err != nil {
		health.Services["monitoring"] = interfaces.ServiceHealth{
			ServiceName:  "monitoring",
			Status:       "unhealthy",
			HealthScore:  0.0,
			LastCheck:    time.Now(),
			ErrorMessage: err.Error(),
			ResponseTime: 0,
		}
		overallHealthy = false
	} else {
		// Use the monitoring service's CheckHealth method if available to get detailed health info
		if monitoringHealth, err := m.MonitoringService.CheckHealth(ctx); err == nil {
			health.Services["monitoring"] = *monitoringHealth
		} else {
			health.Services["monitoring"] = interfaces.ServiceHealth{
				ServiceName:  "monitoring",
				Status:       "healthy",
				HealthScore:  1.0,
				LastCheck:    time.Now(),
				ResponseTime: 0,
			}
		}
	}

	// Set overall status
	if overallHealthy {
		health.OverallStatus = "healthy"
		health.HealthScore = 1.0
	} else {
		health.OverallStatus = "degraded"
		// Calculate health score based on healthy services
		healthyCount := 0
		totalServices := len(health.Services)
		for _, svc := range health.Services {
			if svc.Status == "healthy" {
				healthyCount++
			}
		}
		health.HealthScore = float64(healthyCount) / float64(totalServices)
	}

	// Add some basic metrics
	health.Metrics = append(health.Metrics, interfaces.MetricSummary{
		Name:   "services_count",
		Value:  float64(len(health.Services)),
		Unit:   "count",
		Status: "normal",
	})

	health.Metrics = append(health.Metrics, interfaces.MetricSummary{
		Name:  "health_score",
		Value: health.HealthScore * 100,
		Unit:  "percentage",
		Status: func() string {
			if health.HealthScore > 0.8 {
				return "normal"
			} else if health.HealthScore > 0.5 {
				return "warning"
			}
			return "critical"
		}(),
	})

	return health, nil
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
