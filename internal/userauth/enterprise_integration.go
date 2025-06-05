package userauth

import (
	"context"
	"fmt"
	"net"

	"github.com/Aidin1998/pincex_unified/internal/userauth/admin"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	grpcServer "google.golang.org/grpc"
	"gorm.io/gorm"
)

// EnterpriseIntegration provides enterprise features integration
type EnterpriseIntegration struct {
	adminAPI   *admin.AdminAPI
	grpcServer *grpcServer.Server
	grpcImpl   *grpc.Server
	logger     *zap.Logger
	config     ServiceConfig
}

// NewEnterpriseIntegration creates a new enterprise integration
func NewEnterpriseIntegration(service *Service, config ServiceConfig, logger *zap.Logger) *EnterpriseIntegration {
	integration := &EnterpriseIntegration{
		logger: logger,
		config: config,
	}

	// Initialize admin API
	if service.redisCluster != nil {
		integration.adminAPI = admin.NewAdminAPI(service, logger)
	}

	// Initialize gRPC server
	if config.GRPCConfig.Enabled {
		integration.grpcServer = grpcServer.NewServer()
		integration.grpcImpl = grpc.NewServer(service, logger)

		// Register the UserAuth service
		grpc.RegisterUserAuthServiceServer(integration.grpcServer, integration.grpcImpl)
	}

	return integration
}

// AdminAPI returns the admin API
func (ei *EnterpriseIntegration) AdminAPI() *admin.AdminAPI {
	return ei.adminAPI
}

// StartGRPCServer starts the gRPC server
func (ei *EnterpriseIntegration) StartGRPCServer() error {
	if !ei.config.GRPCConfig.Enabled || ei.grpcServer == nil {
		return nil
	}

	address := fmt.Sprintf("%s:%d", ei.config.GRPCConfig.Address, ei.config.GRPCConfig.Port)
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", address, err)
	}

	ei.logger.Info("Starting gRPC server", zap.String("address", address))

	go func() {
		if err := ei.grpcServer.Serve(listener); err != nil {
			ei.logger.Error("gRPC server failed", zap.Error(err))
		}
	}()

	return nil
}

// StopGRPCServer stops the gRPC server
func (ei *EnterpriseIntegration) StopGRPCServer() {
	if ei.grpcServer != nil {
		ei.logger.Info("Stopping gRPC server")
		ei.grpcServer.GracefulStop()
	}
}

// RegisterAdminRoutes registers admin routes with a router
func (ei *EnterpriseIntegration) RegisterAdminRoutes(router interface{}) error {
	if ei.adminAPI == nil {
		return fmt.Errorf("admin API not initialized")
	}

	// Type assertion for Gin router
	if ginRouter, ok := router.(*gin.RouterGroup); ok {
		ei.adminAPI.RegisterRoutes(ginRouter)
		return nil
	}

	return fmt.Errorf("unsupported router type")
}

// EnhancedService extends the base Service with enterprise features
type EnhancedService struct {
	*Service
	enterpriseIntegration *EnterpriseIntegration
}

// NewEnhancedService creates a new enhanced service with enterprise features
func NewEnhancedService(logger *zap.Logger, db *gorm.DB, config ServiceConfig) (*EnhancedService, error) {
	// Create base service
	baseService, err := NewService(logger, db, config)
	if err != nil {
		return nil, err
	}

	// Create enterprise integration
	enterpriseIntegration := NewEnterpriseIntegration(baseService, config, logger)

	return &EnhancedService{
		Service:               baseService,
		enterpriseIntegration: enterpriseIntegration,
	}, nil
}

// AdminAPI returns the admin API
func (es *EnhancedService) AdminAPI() *admin.AdminAPI {
	return es.enterpriseIntegration.AdminAPI()
}

// Start starts the enhanced service with all enterprise features
func (es *EnhancedService) Start(ctx context.Context) error {
	// Start base service
	if err := es.Service.Start(ctx); err != nil {
		return err
	}

	// Start gRPC server
	if err := es.enterpriseIntegration.StartGRPCServer(); err != nil {
		return fmt.Errorf("failed to start gRPC server: %w", err)
	}

	es.logger.Info("Enhanced UserAuth service started with all enterprise features")
	return nil
}

// Stop stops the enhanced service
func (es *EnhancedService) Stop(ctx context.Context) error {
	// Stop gRPC server
	es.enterpriseIntegration.StopGRPCServer()

	// Stop base service
	return es.Service.Stop(ctx)
}

// RegisterAdminRoutes registers admin routes
func (es *EnhancedService) RegisterAdminRoutes(router interface{}) error {
	return es.enterpriseIntegration.RegisterAdminRoutes(router)
}
