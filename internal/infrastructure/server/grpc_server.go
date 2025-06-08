package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"time"

	"github.com/Aidin1998/finalex/internal/infrastructure/config"
	"github.com/Aidin1998/finalex/internal/infrastructure/middleware"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
)

// GRPCServer represents an enhanced gRPC server with middleware integration
type GRPCServer struct {
	config         *config.GRPCServerConfig
	logger         *zap.Logger
	server         *grpc.Server
	middleware     *middleware.UnifiedMiddleware
	tlsConfig      *tls.Config
	serviceManager *GRPCServiceManager
}

// GRPCServerOptions contains options for creating a GRPCServer
type GRPCServerOptions struct {
	Config         *config.GRPCServerConfig
	Logger         *zap.Logger
	Middleware     *middleware.UnifiedMiddleware
	TLSConfig      *tls.Config
	ServiceManager *GRPCServiceManager
}

// GRPCServiceManager manages gRPC service registrations
type GRPCServiceManager struct {
	logger   *zap.Logger
	services map[string]func(*grpc.Server)
}

// NewGRPCServiceManager creates a new GRPCServiceManager
func NewGRPCServiceManager(logger *zap.Logger) *GRPCServiceManager {
	return &GRPCServiceManager{
		logger:   logger,
		services: make(map[string]func(*grpc.Server)),
	}
}

// RegisterService registers a gRPC service
func (gsm *GRPCServiceManager) RegisterService(name string, registrar func(*grpc.Server)) {
	gsm.services[name] = registrar
	gsm.logger.Debug("Registered gRPC service", zap.String("service", name))
}

// RegisterAllServices registers all services with the gRPC server
func (gsm *GRPCServiceManager) RegisterAllServices(server *grpc.Server) {
	for name, registrar := range gsm.services {
		gsm.logger.Info("Registering gRPC service", zap.String("service", name))
		registrar(server)
	}
}

// GetServices returns all registered services
func (gsm *GRPCServiceManager) GetServices() map[string]func(*grpc.Server) {
	return gsm.services
}

// NewGRPCServer creates a new enhanced gRPC server
func NewGRPCServer(opts GRPCServerOptions) (*GRPCServer, error) {
	if opts.Config == nil {
		return nil, fmt.Errorf("gRPC server config is required")
	}
	if opts.Logger == nil {
		return nil, fmt.Errorf("logger is required")
	}

	server := &GRPCServer{
		config:         opts.Config,
		logger:         opts.Logger,
		middleware:     opts.Middleware,
		tlsConfig:      opts.TLSConfig,
		serviceManager: opts.ServiceManager,
	}

	if err := server.initServer(); err != nil {
		return nil, fmt.Errorf("failed to initialize gRPC server: %w", err)
	}

	return server, nil
}

// initServer initializes the gRPC server with all configurations
func (s *GRPCServer) initServer() error {
	// Configure server options
	opts := []grpc.ServerOption{
		grpc.MaxRecvMsgSize(s.config.MaxRecvMsgSize),
		grpc.MaxSendMsgSize(s.config.MaxSendMsgSize),
		grpc.ConnectionTimeout(s.config.ConnectionTimeout),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle:     s.config.KeepAlive.MaxConnectionIdle,
			MaxConnectionAge:      s.config.KeepAlive.MaxConnectionAge,
			MaxConnectionAgeGrace: s.config.KeepAlive.MaxConnectionAgeGrace,
			Time:                  s.config.KeepAlive.Time,
			Timeout:               s.config.KeepAlive.Timeout,
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             s.config.KeepAlive.MinTime,
			PermitWithoutStream: s.config.KeepAlive.PermitWithoutStream,
		}),
	}

	// Add TLS credentials if available
	if s.tlsConfig != nil {
		creds := credentials.NewTLS(s.tlsConfig)
		opts = append(opts, grpc.Creds(creds))
		s.logger.Info("gRPC server configured with TLS")
	}

	// Add interceptors
	unaryInterceptors := s.getUnaryInterceptors()
	streamInterceptors := s.getStreamInterceptors()

	if len(unaryInterceptors) > 0 {
		opts = append(opts, grpc.ChainUnaryInterceptor(unaryInterceptors...))
	}
	if len(streamInterceptors) > 0 {
		opts = append(opts, grpc.ChainStreamInterceptor(streamInterceptors...))
	}

	// Create server
	s.server = grpc.NewServer(opts...)

	// Register services
	if s.serviceManager != nil {
		s.serviceManager.RegisterAllServices(s.server)
	}

	// Enable reflection if configured
	if s.config.EnableReflection {
		reflection.Register(s.server)
		s.logger.Info("gRPC reflection enabled")
	}

	return nil
}

// getUnaryInterceptors returns all unary interceptors
func (s *GRPCServer) getUnaryInterceptors() []grpc.UnaryServerInterceptor {
	var interceptors []grpc.UnaryServerInterceptor

	// Logging interceptor
	interceptors = append(interceptors, s.loggingUnaryInterceptor)

	// Recovery interceptor
	interceptors = append(interceptors, s.recoveryUnaryInterceptor)

	// Timeout interceptor
	interceptors = append(interceptors, s.timeoutUnaryInterceptor)
	// Add middleware interceptors
	// TODO: Implement gRPC interceptors for UnifiedMiddleware
	// if s.middleware != nil {
	//     middlewareInterceptors := s.middleware.GetGRPCUnaryInterceptors()
	//     interceptors = append(interceptors, middlewareInterceptors...)
	// }

	return interceptors
}

// getStreamInterceptors returns all stream interceptors
func (s *GRPCServer) getStreamInterceptors() []grpc.StreamServerInterceptor {
	var interceptors []grpc.StreamServerInterceptor

	// Logging interceptor
	interceptors = append(interceptors, s.loggingStreamInterceptor)

	// Recovery interceptor
	interceptors = append(interceptors, s.recoveryStreamInterceptor)
	// Add middleware interceptors
	// TODO: Implement gRPC interceptors for UnifiedMiddleware
	// if s.middleware != nil {
	//     middlewareInterceptors := s.middleware.GetGRPCStreamInterceptors()
	//     interceptors = append(interceptors, middlewareInterceptors...)
	// }

	return interceptors
}

// loggingUnaryInterceptor logs unary RPC calls
func (s *GRPCServer) loggingUnaryInterceptor(
	ctx context.Context,
	req interface{},
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (interface{}, error) {
	start := time.Now()

	resp, err := handler(ctx, req)

	duration := time.Since(start)

	fields := []zap.Field{
		zap.String("method", info.FullMethod),
		zap.Duration("duration", duration),
	}

	if err != nil {
		st, _ := status.FromError(err)
		fields = append(fields,
			zap.String("grpc_code", st.Code().String()),
			zap.Error(err))
		s.logger.Error("gRPC unary call failed", fields...)
	} else {
		s.logger.Info("gRPC unary call completed", fields...)
	}

	return resp, err
}

// loggingStreamInterceptor logs streaming RPC calls
func (s *GRPCServer) loggingStreamInterceptor(
	srv interface{},
	stream grpc.ServerStream,
	info *grpc.StreamServerInfo,
	handler grpc.StreamHandler,
) error {
	start := time.Now()

	err := handler(srv, stream)

	duration := time.Since(start)

	fields := []zap.Field{
		zap.String("method", info.FullMethod),
		zap.Duration("duration", duration),
		zap.Bool("client_stream", info.IsClientStream),
		zap.Bool("server_stream", info.IsServerStream),
	}

	if err != nil {
		st, _ := status.FromError(err)
		fields = append(fields,
			zap.String("grpc_code", st.Code().String()),
			zap.Error(err))
		s.logger.Error("gRPC stream call failed", fields...)
	} else {
		s.logger.Info("gRPC stream call completed", fields...)
	}

	return err
}

// recoveryUnaryInterceptor recovers from panics in unary calls
func (s *GRPCServer) recoveryUnaryInterceptor(
	ctx context.Context,
	req interface{},
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (resp interface{}, err error) {
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("gRPC unary call panic recovered",
				zap.String("method", info.FullMethod),
				zap.Any("panic", r))
			err = status.Errorf(codes.Internal, "internal server error")
		}
	}()

	return handler(ctx, req)
}

// recoveryStreamInterceptor recovers from panics in stream calls
func (s *GRPCServer) recoveryStreamInterceptor(
	srv interface{},
	stream grpc.ServerStream,
	info *grpc.StreamServerInfo,
	handler grpc.StreamHandler,
) (err error) {
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("gRPC stream call panic recovered",
				zap.String("method", info.FullMethod),
				zap.Any("panic", r))
			err = status.Errorf(codes.Internal, "internal server error")
		}
	}()

	return handler(srv, stream)
}

// timeoutUnaryInterceptor adds timeout to unary calls
func (s *GRPCServer) timeoutUnaryInterceptor(
	ctx context.Context,
	req interface{},
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (interface{}, error) {
	// Create timeout context if not already present
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		timeout := s.config.ConnectionTimeout
		if timeout == 0 {
			timeout = 30 * time.Second
		}

		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	return handler(ctx, req)
}

// Start starts the gRPC server
func (s *GRPCServer) Start(ctx context.Context, listener net.Listener) error {
	addr := listener.Addr().String()
	s.logger.Info("Starting gRPC server", zap.String("address", addr))

	if err := s.server.Serve(listener); err != nil {
		return fmt.Errorf("gRPC server failed: %w", err)
	}

	return nil
}

// Stop gracefully stops the gRPC server
func (s *GRPCServer) Stop(ctx context.Context) error {
	s.logger.Info("Stopping gRPC server")

	// Try graceful stop first
	stopped := make(chan struct{})
	go func() {
		s.server.GracefulStop()
		close(stopped)
	}()

	// Wait for graceful stop or timeout
	select {
	case <-stopped:
		s.logger.Info("gRPC server stopped gracefully")
		return nil
	case <-ctx.Done():
		s.logger.Warn("gRPC graceful stop timeout, forcing stop")
		s.server.Stop()
		return fmt.Errorf("graceful stop timeout")
	}
}

// GetServer returns the underlying gRPC server
func (s *GRPCServer) GetServer() *grpc.Server {
	return s.server
}

// RegisterService registers a service with the server
func (s *GRPCServer) RegisterService(name string, registrar func(*grpc.Server)) {
	if s.serviceManager != nil {
		s.serviceManager.RegisterService(name, registrar)
		// Re-register all services if server is already created
		if s.server != nil {
			registrar(s.server)
		}
	}
}

// GetStats returns gRPC server statistics
func (s *GRPCServer) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"config": map[string]interface{}{
			"host":               s.config.Host,
			"port":               s.config.Port,
			"max_recv_msg_size":  s.config.MaxRecvMsgSize,
			"max_send_msg_size":  s.config.MaxSendMsgSize,
			"connection_timeout": s.config.ConnectionTimeout.String(),
			"enable_reflection":  s.config.EnableReflection,
			"shutdown_timeout":   s.config.ShutdownTimeout.String(),
		},
		"keep_alive": map[string]interface{}{
			"max_connection_idle":      s.config.KeepAlive.MaxConnectionIdle.String(),
			"max_connection_age":       s.config.KeepAlive.MaxConnectionAge.String(),
			"max_connection_age_grace": s.config.KeepAlive.MaxConnectionAgeGrace.String(),
			"time":                     s.config.KeepAlive.Time.String(),
			"timeout":                  s.config.KeepAlive.Timeout.String(),
			"min_time":                 s.config.KeepAlive.MinTime.String(),
			"permit_without_stream":    s.config.KeepAlive.PermitWithoutStream,
		},
		"services":    s.getServiceStats(),
		"tls_enabled": s.tlsConfig != nil,
	}
}

// getServiceStats returns statistics about registered services
func (s *GRPCServer) getServiceStats() map[string]interface{} {
	if s.serviceManager == nil {
		return map[string]interface{}{"total": 0}
	}

	services := s.serviceManager.GetServices()
	serviceNames := make([]string, 0, len(services))
	for name := range services {
		serviceNames = append(serviceNames, name)
	}

	return map[string]interface{}{
		"total": len(services),
		"names": serviceNames,
	}
}
