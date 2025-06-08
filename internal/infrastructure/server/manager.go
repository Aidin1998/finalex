package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/Aidin1998/finalex/internal/infrastructure/config"
	"github.com/Aidin1998/finalex/internal/infrastructure/middleware"
	"github.com/Aidin1998/finalex/internal/infrastructure/ratelimit"
	"github.com/Aidin1998/finalex/internal/infrastructure/ws"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/reflection"
)

// ServerManager coordinates HTTP, gRPC, and WebSocket servers
type ServerManager struct {
	config *config.Config
	logger *zap.Logger

	// Server components
	httpServer    *http.Server
	grpcServer    *grpc.Server
	wsService     *ws.Service
	tlsManager    *TLSManager
	healthChecker *HealthChecker
	// Middleware and infrastructure
	middleware  *middleware.UnifiedMiddleware
	rateLimiter *ratelimit.RateLimitManager

	// Listeners
	httpListener net.Listener
	grpcListener net.Listener

	// Lifecycle management
	shutdownCtx    context.Context
	shutdownCancel context.CancelFunc
	wg             sync.WaitGroup
	mu             sync.RWMutex
	isRunning      bool
	connections    map[string][]net.Conn
}

// ServerManagerOptions contains options for creating a ServerManager
type ServerManagerOptions struct {
	Config        *config.Config
	Logger        *zap.Logger
	TLSManager    *TLSManager
	HealthChecker *HealthChecker
	Middleware    *middleware.UnifiedMiddleware
	RateLimiter   *ratelimit.RateLimitManager
	HTTPHandlers  http.Handler
	GRPCRegistrar func(*grpc.Server)
	WSService     *ws.Service
}

// NewServerManager creates a new ServerManager
func NewServerManager(opts ServerManagerOptions) (*ServerManager, error) {
	if opts.Config == nil {
		return nil, fmt.Errorf("config is required")
	}
	if opts.Logger == nil {
		return nil, fmt.Errorf("logger is required")
	}

	shutdownCtx, shutdownCancel := context.WithCancel(context.Background())

	sm := &ServerManager{config: opts.Config,
		logger:         opts.Logger,
		tlsManager:     opts.TLSManager,
		healthChecker:  opts.HealthChecker,
		middleware:     opts.Middleware,
		rateLimiter:    opts.RateLimiter,
		wsService:      opts.WSService,
		shutdownCtx:    shutdownCtx,
		shutdownCancel: shutdownCancel,
		connections:    make(map[string][]net.Conn),
	}

	if err := sm.initHTTPServer(opts.HTTPHandlers); err != nil {
		return nil, fmt.Errorf("failed to initialize HTTP server: %w", err)
	}

	if err := sm.initGRPCServer(opts.GRPCRegistrar); err != nil {
		return nil, fmt.Errorf("failed to initialize gRPC server: %w", err)
	}

	return sm, nil
}

// initHTTPServer initializes the HTTP server
func (sm *ServerManager) initHTTPServer(handler http.Handler) error {
	if handler == nil {
		return fmt.Errorf("HTTP handler is required")
	}

	httpConfig := sm.config.Server.HTTP

	// Create HTTP server with timeouts
	sm.httpServer = &http.Server{
		Addr:              fmt.Sprintf("%s:%d", httpConfig.Host, httpConfig.Port),
		Handler:           handler,
		ReadTimeout:       httpConfig.ReadTimeout,
		WriteTimeout:      httpConfig.WriteTimeout,
		IdleTimeout:       httpConfig.IdleTimeout,
		ReadHeaderTimeout: httpConfig.ReadHeaderTimeout,
		MaxHeaderBytes:    httpConfig.MaxHeaderBytes, ConnState: sm.trackHTTPConnections,
	}

	// Enable HTTP/2 if configured
	if httpConfig.EnableHTTP2 {
		sm.httpServer.TLSNextProto = make(map[string]func(*http.Server, *tls.Conn, http.Handler))
	}

	return nil
}

// initGRPCServer initializes the gRPC server
func (sm *ServerManager) initGRPCServer(registrar func(*grpc.Server)) error {
	grpcConfig := sm.config.Server.GRPC

	// Configure gRPC server options
	opts := []grpc.ServerOption{
		grpc.MaxRecvMsgSize(grpcConfig.MaxRecvMsgSize),
		grpc.MaxSendMsgSize(grpcConfig.MaxSendMsgSize),
		grpc.ConnectionTimeout(grpcConfig.ConnectionTimeout),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle:     grpcConfig.KeepAlive.MaxConnectionIdle,
			MaxConnectionAge:      grpcConfig.KeepAlive.MaxConnectionAge,
			MaxConnectionAgeGrace: grpcConfig.KeepAlive.MaxConnectionAgeGrace,
			Time:                  grpcConfig.KeepAlive.Time,
			Timeout:               grpcConfig.KeepAlive.Timeout,
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             grpcConfig.KeepAlive.MinTime,
			PermitWithoutStream: grpcConfig.KeepAlive.PermitWithoutStream,
		}),
	}

	// Add TLS credentials if TLS is enabled
	if sm.config.Server.TLS.Enabled && sm.tlsManager != nil {
		tlsConfig := sm.tlsManager.GetTLSConfig()
		creds := credentials.NewTLS(tlsConfig)
		opts = append(opts, grpc.Creds(creds))
	}
	// Add middleware interceptors
	// TODO: Implement gRPC interceptors for UnifiedMiddleware
	// if sm.middleware != nil {
	//     unaryInterceptors := sm.middleware.GetGRPCUnaryInterceptors()
	//     streamInterceptors := sm.middleware.GetGRPCStreamInterceptors()
	//
	//     if len(unaryInterceptors) > 0 {
	//         opts = append(opts, grpc.ChainUnaryInterceptor(unaryInterceptors...))
	//     }
	//     if len(streamInterceptors) > 0 {
	//         opts = append(opts, grpc.ChainStreamInterceptor(streamInterceptors...))
	//     }
	// }

	sm.grpcServer = grpc.NewServer(opts...)

	// Register services
	if registrar != nil {
		registrar(sm.grpcServer)
	}

	// Enable reflection if configured
	if grpcConfig.EnableReflection {
		reflection.Register(sm.grpcServer)
	}

	return nil
}

// Start starts all servers
func (sm *ServerManager) Start(ctx context.Context) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.isRunning {
		return fmt.Errorf("server manager is already running")
	}

	sm.logger.Info("Starting server manager")
	// Start health checker
	// TODO: HealthChecker doesn't have Start method
	// if sm.healthChecker != nil {
	//     sm.healthChecker.Start(ctx)
	// }

	// Start HTTP server
	if err := sm.startHTTPServer(ctx); err != nil {
		return fmt.Errorf("failed to start HTTP server: %w", err)
	}

	// Start gRPC server
	if err := sm.startGRPCServer(ctx); err != nil {
		return fmt.Errorf("failed to start gRPC server: %w", err)
	}

	// Start WebSocket service
	if err := sm.startWebSocketService(ctx); err != nil {
		return fmt.Errorf("failed to start WebSocket service: %w", err)
	}

	sm.isRunning = true
	sm.logger.Info("Server manager started successfully")
	return nil
}

// startHTTPServer starts the HTTP server
func (sm *ServerManager) startHTTPServer(ctx context.Context) error {
	httpConfig := sm.config.Server.HTTP
	addr := fmt.Sprintf("%s:%d", httpConfig.Host, httpConfig.Port)

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to create HTTP listener: %w", err)
	}

	sm.httpListener = listener

	sm.wg.Add(1)
	go func() {
		defer sm.wg.Done()

		sm.logger.Info("Starting HTTP server", zap.String("address", addr))

		var err error
		if sm.config.Server.TLS.Enabled && sm.tlsManager != nil {
			tlsConfig := sm.tlsManager.GetTLSConfig()
			tlsListener := tls.NewListener(listener, tlsConfig)
			err = sm.httpServer.Serve(tlsListener)
		} else {
			err = sm.httpServer.Serve(listener)
		}

		if err != nil && err != http.ErrServerClosed {
			sm.logger.Error("HTTP server error", zap.Error(err))
		}
	}()

	return nil
}

// startGRPCServer starts the gRPC server
func (sm *ServerManager) startGRPCServer(ctx context.Context) error {
	grpcConfig := sm.config.Server.GRPC
	addr := fmt.Sprintf("%s:%d", grpcConfig.Host, grpcConfig.Port)

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to create gRPC listener: %w", err)
	}

	sm.grpcListener = listener

	sm.wg.Add(1)
	go func() {
		defer sm.wg.Done()

		sm.logger.Info("Starting gRPC server", zap.String("address", addr))

		if err := sm.grpcServer.Serve(listener); err != nil {
			sm.logger.Error("gRPC server error", zap.Error(err))
		}
	}()

	return nil
}

// startWebSocketService starts the WebSocket service
func (sm *ServerManager) startWebSocketService(ctx context.Context) error {
	if sm.wsService == nil {
		sm.logger.Warn("WebSocket service not configured, skipping")
		return nil
	}

	wsConfig := sm.config.Server.WS
	port := fmt.Sprintf(":%d", wsConfig.Port)

	sm.wg.Add(1)
	go func() {
		defer sm.wg.Done()

		sm.logger.Info("Starting WebSocket service", zap.String("port", port))

		if err := sm.wsService.Start(port); err != nil {
			sm.logger.Error("WebSocket service error", zap.Error(err))
		}
	}()

	return nil
}

// Shutdown gracefully shuts down all servers
func (sm *ServerManager) Shutdown(ctx context.Context) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if !sm.isRunning {
		return nil
	}

	sm.logger.Info("Shutting down server manager")

	// Cancel context to signal shutdown
	sm.shutdownCancel()

	// Create timeout context for graceful shutdown
	shutdownTimeout := sm.config.Server.HTTP.ShutdownTimeout
	if shutdownTimeout == 0 {
		shutdownTimeout = 30 * time.Second
	}

	shutdownCtx, cancel := context.WithTimeout(ctx, shutdownTimeout)
	defer cancel()

	var shutdownErrors []error

	// Shutdown HTTP server
	if sm.httpServer != nil {
		sm.logger.Info("Shutting down HTTP server")
		if err := sm.shutdownHTTPServer(shutdownCtx); err != nil {
			shutdownErrors = append(shutdownErrors, fmt.Errorf("HTTP server shutdown: %w", err))
		}
	}

	// Shutdown gRPC server
	if sm.grpcServer != nil {
		sm.logger.Info("Shutting down gRPC server")
		sm.shutdownGRPCServer(shutdownCtx)
	}

	// Shutdown WebSocket service
	if sm.wsService != nil {
		sm.logger.Info("Shutting down WebSocket service")
		if err := sm.wsService.Stop(shutdownCtx); err != nil {
			shutdownErrors = append(shutdownErrors, fmt.Errorf("WebSocket service shutdown: %w", err))
		}
	}
	// Stop health checker
	// TODO: HealthChecker doesn't have Stop method
	// if sm.healthChecker != nil {
	//     sm.healthChecker.Stop()
	// }

	// Wait for all goroutines to complete
	done := make(chan struct{})
	go func() {
		sm.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		sm.logger.Info("All servers shut down gracefully")
	case <-shutdownCtx.Done():
		sm.logger.Warn("Shutdown timeout exceeded, forcing termination")
		shutdownErrors = append(shutdownErrors, fmt.Errorf("shutdown timeout exceeded"))
	}

	sm.isRunning = false

	if len(shutdownErrors) > 0 {
		return fmt.Errorf("shutdown errors: %v", shutdownErrors)
	}

	return nil
}

// shutdownHTTPServer gracefully shuts down the HTTP server
func (sm *ServerManager) shutdownHTTPServer(ctx context.Context) error {
	// Set server to closing state
	sm.httpServer.SetKeepAlivesEnabled(false)

	// Begin graceful shutdown
	if err := sm.httpServer.Shutdown(ctx); err != nil {
		// Force close if graceful shutdown fails
		if closeErr := sm.httpServer.Close(); closeErr != nil {
			return fmt.Errorf("graceful shutdown failed: %w, force close failed: %v", err, closeErr)
		}
		return fmt.Errorf("graceful shutdown failed, forced close: %w", err)
	}

	return nil
}

// shutdownGRPCServer gracefully shuts down the gRPC server
func (sm *ServerManager) shutdownGRPCServer(ctx context.Context) {
	// Try graceful stop first
	stopped := make(chan struct{})
	go func() {
		sm.grpcServer.GracefulStop()
		close(stopped)
	}()

	select {
	case <-stopped:
		sm.logger.Info("gRPC server stopped gracefully")
	case <-ctx.Done():
		sm.logger.Warn("gRPC graceful stop timeout, forcing stop")
		sm.grpcServer.Stop()
	}
}

// trackHTTPConnections tracks HTTP connections for graceful shutdown
func (sm *ServerManager) trackHTTPConnections(conn net.Conn, state http.ConnState) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	switch state {
	case http.StateNew:
		sm.connections["http"] = append(sm.connections["http"], conn)
	case http.StateClosed, http.StateHijacked:
		// Remove connection from tracking
		conns := sm.connections["http"]
		for i, c := range conns {
			if c == conn {
				sm.connections["http"] = append(conns[:i], conns[i+1:]...)
				break
			}
		}
	}
}

// GetHTTPServer returns the HTTP server instance
func (sm *ServerManager) GetHTTPServer() *http.Server {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.httpServer
}

// GetGRPCServer returns the gRPC server instance
func (sm *ServerManager) GetGRPCServer() *grpc.Server {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.grpcServer
}

// GetWebSocketService returns the WebSocket service instance
func (sm *ServerManager) GetWebSocketService() *ws.Service {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.wsService
}

// IsRunning returns whether the server manager is running
func (sm *ServerManager) IsRunning() bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.isRunning
}

// GetConnectionCount returns the number of active connections by server type
func (sm *ServerManager) GetConnectionCount() map[string]int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	counts := make(map[string]int)
	for serverType, conns := range sm.connections {
		counts[serverType] = len(conns)
	}
	return counts
}
