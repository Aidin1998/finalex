// Package integration provides end-to-end integration between userauth, accounts, trading, and fiat modules
package integration

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Aidin1998/finalex/internal/integration/contracts"
	"github.com/Aidin1998/finalex/internal/integration/infrastructure"
	"go.uber.org/zap"
)

// IntegrationService orchestrates cross-module operations with consistent authentication,
// distributed transactions, event propagation, and observability
type IntegrationService struct {
	// Service contracts
	userauth contracts.UserAuthServiceContract
	accounts contracts.AccountsServiceContract
	trading  contracts.TradingServiceContract
	fiat     contracts.FiatServiceContract

	// Infrastructure components
	eventBus  infrastructure.EventBus
	txManager infrastructure.DistributedTransactionManager
	cache     infrastructure.CacheManager
	metrics   infrastructure.MetricsCollector
	tracer    infrastructure.DistributedTracer
	logger    *zap.Logger
	// Service state
	mu       sync.RWMutex
	started  bool
	stopChan chan struct{}
}

// ServiceConfig holds configuration for the integration service
type ServiceConfig struct {
	// Performance settings
	CacheConfig         CacheConfig         `yaml:"cache"`
	EventBusConfig      EventBusConfig      `yaml:"event_bus"`
	TransactionConfig   TransactionConfig   `yaml:"transaction"`
	ObservabilityConfig ObservabilityConfig `yaml:"observability"`

	// Rate limiting
	RateLimitConfig RateLimitConfig `yaml:"rate_limit"`

	// Security settings
	SecurityConfig SecurityConfig `yaml:"security"`

	// Service timeouts
	DefaultTimeout        time.Duration `yaml:"default_timeout"`
	AuthenticationTimeout time.Duration `yaml:"auth_timeout"`
	TransactionTimeout    time.Duration `yaml:"transaction_timeout"`
}

// Configuration types
type CacheConfig struct {
	MaxSize    int64         `yaml:"max_size"`
	DefaultTTL time.Duration `yaml:"default_ttl"`
}

type EventBusConfig struct {
	BufferSize int `yaml:"buffer_size"`
}

type TransactionConfig struct {
	DefaultTimeout time.Duration `yaml:"default_timeout"`
}

type ObservabilityConfig struct {
	MaxTraces int `yaml:"max_traces"`
}

type RateLimitConfig struct {
	RequestsPerSecond int           `yaml:"requests_per_second"`
	BurstSize         int           `yaml:"burst_size"`
	WindowSize        time.Duration `yaml:"window_size"`
}

type SecurityConfig struct {
	RequireKYC      bool `yaml:"require_kyc"`
	MinKYCLevel     int  `yaml:"min_kyc_level"`
	EnableRateLimit bool `yaml:"enable_rate_limit"`
}

// NewIntegrationService creates a new integration service instance
func NewIntegrationService(
	userauth contracts.UserAuthServiceContract,
	accounts contracts.AccountsServiceContract,
	trading contracts.TradingServiceContract,
	fiat contracts.FiatServiceContract,
	config *ServiceConfig,
	logger *zap.Logger,
) (*IntegrationService, error) {
	if userauth == nil {
		return nil, fmt.Errorf("userauth service is required")
	}
	if accounts == nil {
		return nil, fmt.Errorf("accounts service is required")
	}
	if trading == nil {
		return nil, fmt.Errorf("trading service is required")
	}
	if fiat == nil {
		return nil, fmt.Errorf("fiat service is required")
	}

	// Initialize infrastructure components
	eventBus, err := NewEventBus(config.EventBusConfig, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize event bus: %w", err)
	}

	txManager, err := NewDistributedTransactionManager(config.TransactionConfig, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize transaction manager: %w", err)
	}

	cache, err := NewCacheManager(config.CacheConfig, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize cache manager: %w", err)
	}

	metrics, err := NewMetricsCollector(config.ObservabilityConfig, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize metrics collector: %w", err)
	}

	tracer, err := NewDistributedTracer(config.ObservabilityConfig, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize distributed tracer: %w", err)
	}

	return &IntegrationService{
		userauth:  userauth,
		accounts:  accounts,
		trading:   trading,
		fiat:      fiat,
		eventBus:  eventBus,
		txManager: txManager,
		cache:     cache,
		metrics:   metrics,
		tracer:    tracer,
		logger:    logger,
		stopChan:  make(chan struct{}),
	}, nil
}

// Start initializes and starts the integration service
func (s *IntegrationService) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started {
		return fmt.Errorf("integration service already started")
	}

	s.logger.Info("Starting integration service")

	// Start infrastructure components
	if err := s.eventBus.Start(ctx); err != nil {
		return fmt.Errorf("failed to start event bus: %w", err)
	}

	if err := s.txManager.Start(ctx); err != nil {
		return fmt.Errorf("failed to start transaction manager: %w", err)
	}

	if err := s.cache.Start(ctx); err != nil {
		return fmt.Errorf("failed to start cache manager: %w", err)
	}

	if err := s.metrics.Start(ctx); err != nil {
		return fmt.Errorf("failed to start metrics collector: %w", err)
	}

	if err := s.tracer.Start(ctx); err != nil {
		return fmt.Errorf("failed to start distributed tracer: %w", err)
	}

	// Subscribe to critical events
	if err := s.subscribeToEvents(ctx); err != nil {
		return fmt.Errorf("failed to subscribe to events: %w", err)
	}

	// Start health monitoring
	go s.healthMonitoringLoop(ctx)

	// Start metrics collection
	go s.metricsCollectionLoop(ctx)

	s.started = true
	s.logger.Info("Integration service started successfully")

	return nil
}

// Stop gracefully shuts down the integration service
func (s *IntegrationService) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.started {
		return nil
	}

	s.logger.Info("Stopping integration service")

	// Signal stop to background routines
	close(s.stopChan)

	// Stop infrastructure components
	errors := []error{}

	if err := s.tracer.Stop(ctx); err != nil {
		errors = append(errors, fmt.Errorf("failed to stop tracer: %w", err))
	}

	if err := s.metrics.Stop(ctx); err != nil {
		errors = append(errors, fmt.Errorf("failed to stop metrics: %w", err))
	}

	if err := s.cache.Stop(ctx); err != nil {
		errors = append(errors, fmt.Errorf("failed to stop cache: %w", err))
	}

	if err := s.txManager.Stop(ctx); err != nil {
		errors = append(errors, fmt.Errorf("failed to stop transaction manager: %w", err))
	}

	if err := s.eventBus.Stop(ctx); err != nil {
		errors = append(errors, fmt.Errorf("failed to stop event bus: %w", err))
	}

	s.started = false

	if len(errors) > 0 {
		s.logger.Error("Errors during shutdown", zap.Errors("errors", errors))
		return fmt.Errorf("shutdown completed with %d errors", len(errors))
	}

	s.logger.Info("Integration service stopped successfully")
	return nil
}

// Health returns the overall health status of the integration service
func (s *IntegrationService) Health(ctx context.Context) (*IntegrationHealth, error) {
	span := s.tracer.StartSpan(ctx, "integration.health_check")
	defer span.End()

	// Check health of all service contracts
	userAuthHealth, err := s.userauth.HealthCheck(ctx)
	if err != nil {
		span.RecordError(err)
		s.logger.Error("UserAuth health check failed", zap.Error(err))
	}

	accountsHealth, err := s.accounts.HealthCheck(ctx)
	if err != nil {
		span.RecordError(err)
		s.logger.Error("Accounts health check failed", zap.Error(err))
	}

	tradingHealth, err := s.trading.HealthCheck(ctx)
	if err != nil {
		span.RecordError(err)
		s.logger.Error("Trading health check failed", zap.Error(err))
	}

	fiatHealth, err := s.fiat.HealthCheck(ctx)
	if err != nil {
		span.RecordError(err)
		s.logger.Error("Fiat health check failed", zap.Error(err))
	}

	// Check infrastructure health
	infraHealth := s.getInfrastructureHealth(ctx)

	// Determine overall status
	overallStatus := "healthy"
	if userAuthHealth == nil || accountsHealth == nil || tradingHealth == nil || fiatHealth == nil {
		overallStatus = "degraded"
	}

	for _, component := range infraHealth {
		if component.Status != "healthy" {
			overallStatus = "degraded"
			break
		}
	}

	health := &IntegrationHealth{
		Status:         overallStatus,
		Timestamp:      time.Now(),
		UserAuthHealth: userAuthHealth,
		AccountsHealth: accountsHealth,
		TradingHealth:  tradingHealth,
		FiatHealth:     fiatHealth,
		Infrastructure: infraHealth,
		Metrics:        s.getServiceMetrics(ctx),
	}

	s.metrics.RecordHealthCheck(ctx, health)
	return health, nil
}

// subscribeToEvents sets up event subscriptions for cross-module coordination
func (s *IntegrationService) subscribeToEvents(ctx context.Context) error {
	// Subscribe to user events
	if err := s.eventBus.Subscribe(ctx, "user.created", s.handleUserCreated); err != nil {
		return fmt.Errorf("failed to subscribe to user.created: %w", err)
	}

	if err := s.eventBus.Subscribe(ctx, "user.kyc_updated", s.handleKYCUpdated); err != nil {
		return fmt.Errorf("failed to subscribe to user.kyc_updated: %w", err)
	}

	// Subscribe to account events
	if err := s.eventBus.Subscribe(ctx, "account.balance_updated", s.handleBalanceUpdated); err != nil {
		return fmt.Errorf("failed to subscribe to account.balance_updated: %w", err)
	}

	if err := s.eventBus.Subscribe(ctx, "account.transaction_completed", s.handleTransactionCompleted); err != nil {
		return fmt.Errorf("failed to subscribe to account.transaction_completed: %w", err)
	}

	// Subscribe to trading events
	if err := s.eventBus.Subscribe(ctx, "trade.executed", s.handleTradeExecuted); err != nil {
		return fmt.Errorf("failed to subscribe to trade.executed: %w", err)
	}

	if err := s.eventBus.Subscribe(ctx, "order.placed", s.handleOrderPlaced); err != nil {
		return fmt.Errorf("failed to subscribe to order.placed: %w", err)
	}

	// Subscribe to fiat events
	if err := s.eventBus.Subscribe(ctx, "fiat.deposit_completed", s.handleFiatDepositCompleted); err != nil {
		return fmt.Errorf("failed to subscribe to fiat.deposit_completed: %w", err)
	}

	if err := s.eventBus.Subscribe(ctx, "fiat.withdrawal_completed", s.handleFiatWithdrawalCompleted); err != nil {
		return fmt.Errorf("failed to subscribe to fiat.withdrawal_completed: %w", err)
	}

	return nil
}

// healthMonitoringLoop continuously monitors service health
func (s *IntegrationService) healthMonitoringLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopChan:
			return
		case <-ticker.C:
			if health, err := s.Health(ctx); err != nil {
				s.logger.Error("Health check failed", zap.Error(err))
			} else if health.Status != "healthy" {
				s.logger.Warn("Service health degraded",
					zap.String("status", health.Status),
					zap.Any("details", health))
			}
		}
	}
}

// metricsCollectionLoop continuously collects and reports metrics
func (s *IntegrationService) metricsCollectionLoop(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.collectAndReportMetrics(ctx)
		}
	}
}

// getInfrastructureHealth checks health of infrastructure components
func (s *IntegrationService) getInfrastructureHealth(ctx context.Context) map[string]*ComponentHealth {
	health := make(map[string]*ComponentHealth)

	// Check event bus health
	health["event_bus"] = &ComponentHealth{
		Name:      "event_bus",
		Status:    s.getComponentStatus(s.eventBus.IsHealthy()),
		Timestamp: time.Now(),
	}

	// Check transaction manager health
	health["transaction_manager"] = &ComponentHealth{
		Name:      "transaction_manager",
		Status:    s.getComponentStatus(s.txManager.IsHealthy()),
		Timestamp: time.Now(),
	}

	// Check cache health
	health["cache"] = &ComponentHealth{
		Name:      "cache",
		Status:    s.getComponentStatus(s.cache.IsHealthy()),
		Timestamp: time.Now(),
	}

	// Check metrics health
	health["metrics"] = &ComponentHealth{
		Name:      "metrics",
		Status:    s.getComponentStatus(s.metrics.IsHealthy()),
		Timestamp: time.Now(),
	}

	// Check tracer health
	health["tracer"] = &ComponentHealth{
		Name:      "tracer",
		Status:    s.getComponentStatus(s.tracer.IsHealthy()),
		Timestamp: time.Now(),
	}

	return health
}

// getComponentStatus converts boolean health to status string
func (s *IntegrationService) getComponentStatus(healthy bool) string {
	if healthy {
		return "healthy"
	}
	return "unhealthy"
}

// getServiceMetrics collects current service metrics
func (s *IntegrationService) getServiceMetrics(ctx context.Context) *ServiceMetrics {
	return &ServiceMetrics{
		RequestCount:       s.metrics.GetRequestCount(),
		ErrorCount:         s.metrics.GetErrorCount(),
		AverageLatency:     s.metrics.GetAverageLatency(),
		ActiveTransactions: s.txManager.GetActiveTransactionCount(),
		CacheHitRate:       s.cache.GetHitRate(),
		EventsProcessed:    s.eventBus.GetEventsProcessed(),
		Timestamp:          time.Now(),
	}
}

// collectAndReportMetrics collects metrics from all components
func (s *IntegrationService) collectAndReportMetrics(ctx context.Context) {
	// Collect metrics from all services
	metrics := s.getServiceMetrics(ctx)

	// Report to monitoring systems
	s.metrics.RecordServiceMetrics(ctx, metrics)

	// Log important metrics
	s.logger.Debug("Service metrics collected",
		zap.Int64("requests", metrics.RequestCount),
		zap.Int64("errors", metrics.ErrorCount),
		zap.Duration("avg_latency", metrics.AverageLatency),
		zap.Int("active_transactions", metrics.ActiveTransactions),
		zap.Float64("cache_hit_rate", metrics.CacheHitRate))
}
