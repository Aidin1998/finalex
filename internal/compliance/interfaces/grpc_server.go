package interfaces

import (
	"context"

	"go.uber.org/zap"
)

// GRPCServer is currently disabled until protobuf files are generated
// TODO: Uncomment and fix when proto files are generated
type GRPCServer struct {
	// pb.UnimplementedComplianceServiceServer
	complianceService   ComplianceService
	monitoringService   MonitoringService
	manipulationService ManipulationService
	auditService        AuditService
	logger              *zap.Logger
}

// NewGRPCServer creates a new gRPC compliance server (currently stubbed)
func NewGRPCServer(
	complianceService ComplianceService,
	monitoringService MonitoringService,
	manipulationService ManipulationService,
	auditService AuditService,
	logger *zap.Logger,
) *GRPCServer {
	return &GRPCServer{
		complianceService:   complianceService,
		monitoringService:   monitoringService,
		manipulationService: manipulationService,
		auditService:        auditService,
		logger:              logger,
	}
}

// Placeholder method to satisfy interface until protobuf is generated
func (s *GRPCServer) Placeholder(ctx context.Context) error {
	s.logger.Info("GRPCServer placeholder method called")
	return nil
}

// StartServer starts the gRPC server (placeholder implementation)
func (s *GRPCServer) StartServer(ctx context.Context, port string) error {
	s.logger.Info("gRPC server placeholder - would start on port", zap.String("port", port))
	return nil
}

// StopServer stops the gRPC server (placeholder implementation)
func (s *GRPCServer) StopServer(ctx context.Context) error {
	s.logger.Info("gRPC server placeholder - would stop server")
	return nil
}

// RegisterServices registers gRPC services (placeholder implementation)
func (s *GRPCServer) RegisterServices() error {
	s.logger.Info("gRPC server placeholder - would register services")
	return nil
}

// TODO: When protobuf files are generated, uncomment and implement the following methods:
// - HealthCheck
// - ReadinessCheck
// - PerformComplianceCheck
// - GetUserComplianceStatus
// - BatchComplianceCheck
// - GetMonitoringOverview
// - GetActiveAlerts
// - AcknowledgeAlert
// - GetSystemHealth
// - DetectManipulation
// - GetManipulationAlerts
// - ResolveManipulationAlert
// - CreateInvestigation
// - GetInvestigation
// - UpdateInvestigation
// - ListInvestigations
// - GetAuditLogs
// - CreateAuditEntry
// - GetMonitoringPolicies
// - UpdateMonitoringPolicy
