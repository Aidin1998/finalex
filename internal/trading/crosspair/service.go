package crosspair

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"
)

// CrossPairService is the main service that orchestrates all cross-pair trading components
type CrossPairService struct {
	config      *CrossPairConfig
	engine      *CrossPairEngine
	rateCalc    *RateCalculator
	wsManager   *WebSocketManager
	storage     Storage
	api         *CrossPairAPI
	adminAPI    *AdminAPI
	integration *ServiceIntegration

	// HTTP servers
	mainServer  *http.Server
	adminServer *http.Server

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Status
	started   bool
	startTime time.Time
	mu        sync.RWMutex
}

// ServiceOptions holds options for creating the service
type ServiceOptions struct {
	Config      *CrossPairConfig
	Integration *ServiceIntegration
	Database    *sqlx.DB
}

// NewCrossPairService creates a new cross-pair trading service
func NewCrossPairService(opts *ServiceOptions) (*CrossPairService, error) {
	if opts.Config == nil {
		opts.Config = DefaultConfig()
	}

	if err := opts.Config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	service := &CrossPairService{
		config: opts.Config,
		ctx:    ctx,
		cancel: cancel,
	}

	// Initialize storage
	storage, err := service.initializeStorage(opts.Database)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize storage: %w", err)
	}
	service.storage = storage

	// Initialize components
	if err := service.initializeComponents(opts.Integration); err != nil {
		return nil, fmt.Errorf("failed to initialize components: %w", err)
	}

	// Initialize HTTP servers
	if err := service.initializeServers(); err != nil {
		return nil, fmt.Errorf("failed to initialize servers: %w", err)
	}

	return service, nil
}

// initializeStorage sets up the storage layer
func (s *CrossPairService) initializeStorage(db *sqlx.DB) (Storage, error) {
	switch s.config.Storage.Type {
	case "postgres":
		if db == nil {
			return nil, fmt.Errorf("database connection required for PostgreSQL storage")
		}
		return NewPostgreSQLStorage(db), nil
	case "memory":
		return NewInMemoryStorage(), nil
	default:
		return nil, fmt.Errorf("unsupported storage type: %s", s.config.Storage.Type)
	}
}

// initializeComponents sets up all the cross-pair trading components
func (s *CrossPairService) initializeComponents(integration *ServiceIntegration) error {
	// Initialize providers based on whether we have real integration or not
	var orderbookProvider OrderbookProvider
	var eventPublisher EventPublisher
	var metricsCollector MetricsCollector
	var atomicExecutor AtomicExecutor

	if integration != nil {
		// Use real implementations
		s.integration = integration
		adapter := NewCrossPairIntegrationAdapter(integration)
		orderbookProvider = adapter.GetRealOrderbookProvider()
		eventPublisher = adapter.GetRealEventPublisher()
		metricsCollector = adapter.GetRealMetricsCollector()
		atomicExecutor = adapter.GetRealAtomicExecutor()
	} else {
		// Use mock implementations for development/testing
		orderbookProvider = NewMockOrderbookProvider()
		eventPublisher = NewMockEventPublisher()
		metricsCollector = NewMockMetricsCollector()
		atomicExecutor = NewMockAtomicExecutor()
	}

	// Initialize rate calculator
	rateCalcConfig := RateCalculatorConfig{
		UpdateInterval:      s.config.RateCalculator.UpdateInterval,
		ConfidenceThreshold: s.config.RateCalculator.ConfidenceThreshold,
		MaxSlippage:         s.config.RateCalculator.MaxSlippage,
		EnableCaching:       s.config.RateCalculator.EnableCaching,
		CacheTTL:            s.config.RateCalculator.CacheTTL,
	}

	var err error
	s.rateCalc, err = NewRateCalculator(rateCalcConfig, orderbookProvider)
	if err != nil {
		return fmt.Errorf("failed to create rate calculator: %w", err)
	}

	// Initialize execution engine
	engineConfig := CrossPairEngineConfig{
		MaxConcurrentOrders: s.config.MaxConcurrentOrders,
		OrderTimeout:        s.config.OrderTimeout,
		RetryAttempts:       s.config.RetryAttempts,
		RetryDelay:          s.config.RetryDelay,
		QueueSize:           s.config.QueueSize,
	}

	s.engine, err = NewCrossPairEngine(
		engineConfig,
		s.storage,
		s.rateCalc,
		atomicExecutor,
		eventPublisher,
		metricsCollector,
	)
	if err != nil {
		return fmt.Errorf("failed to create cross-pair engine: %w", err)
	}

	// Initialize WebSocket manager if enabled
	if s.config.WebSocket.Enabled {
		s.wsManager = NewWebSocketManager(s.rateCalc, s.engine)
	}

	// Initialize APIs
	s.api = NewCrossPairAPI(s.engine, s.storage, s.rateCalc, s.wsManager)
	s.adminAPI = NewAdminAPI(s.engine, s.storage, s.wsManager, s.rateCalc)

	return nil
}

// initializeServers sets up the HTTP servers
func (s *CrossPairService) initializeServers() error {
	// Main API server
	mainRouter := mux.NewRouter()

	// Register API routes
	apiRouter := mainRouter.PathPrefix("/api/v1").Subrouter()
	s.api.RegisterRoutes(apiRouter)

	// Register WebSocket endpoint if enabled
	if s.config.WebSocket.Enabled && s.wsManager != nil {
		mainRouter.HandleFunc("/ws/crosspair", s.wsManager.HandleWebSocket)
	}

	s.mainServer = &http.Server{
		Addr:         ":8080", // TODO: Make configurable
		Handler:      mainRouter,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Admin server
	adminRouter := mux.NewRouter()
	s.adminAPI.RegisterAdminRoutes(adminRouter)

	s.adminServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.config.Monitoring.HealthCheckPort),
		Handler:      adminRouter,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	return nil
}

// Start starts the cross-pair trading service
func (s *CrossPairService) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started {
		return fmt.Errorf("service already started")
	}

	log.Println("Starting cross-pair trading service...")
	s.startTime = time.Now()

	// Start storage migrations if enabled
	if s.config.Storage.EnableMigration && s.config.Storage.Type != "memory" {
		if err := s.runMigrations(); err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}
	}

	// Start rate calculator
	if err := s.rateCalc.Start(s.ctx); err != nil {
		return fmt.Errorf("failed to start rate calculator: %w", err)
	}
	log.Println("Rate calculator started")

	// Start execution engine
	if err := s.engine.Start(s.ctx); err != nil {
		return fmt.Errorf("failed to start execution engine: %w", err)
	}
	log.Println("Execution engine started")

	// Start WebSocket manager if enabled
	if s.wsManager != nil {
		s.wsManager.Start(s.ctx)
		log.Println("WebSocket manager started")
	}

	// Start HTTP servers
	s.wg.Add(2)

	// Start main API server
	go func() {
		defer s.wg.Done()
		log.Printf("Starting main API server on %s", s.mainServer.Addr)
		if err := s.mainServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("Main server error: %v", err)
		}
	}()

	// Start admin server
	go func() {
		defer s.wg.Done()
		log.Printf("Starting admin server on %s", s.adminServer.Addr)
		if err := s.adminServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("Admin server error: %v", err)
		}
	}()

	// Start background tasks
	s.startBackgroundTasks()

	s.started = true
	log.Println("Cross-pair trading service started successfully")

	return nil
}

// Stop stops the cross-pair trading service
func (s *CrossPairService) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.started {
		return fmt.Errorf("service not started")
	}

	log.Println("Stopping cross-pair trading service...")

	// Cancel context to stop all goroutines
	s.cancel()

	// Stop HTTP servers
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := s.mainServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("Error shutting down main server: %v", err)
	}

	if err := s.adminServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("Error shutting down admin server: %v", err)
	}

	// Stop WebSocket manager
	if s.wsManager != nil {
		s.wsManager.Shutdown()
		log.Println("WebSocket manager stopped")
	}

	// Stop engine
	if err := s.engine.Stop(); err != nil {
		log.Printf("Error stopping engine: %v", err)
	} else {
		log.Println("Execution engine stopped")
	}

	// Stop rate calculator
	if err := s.rateCalc.Stop(); err != nil {
		log.Printf("Error stopping rate calculator: %v", err)
	} else {
		log.Println("Rate calculator stopped")
	}

	// Wait for all goroutines to finish
	s.wg.Wait()

	s.started = false
	log.Println("Cross-pair trading service stopped")

	return nil
}

// Restart restarts the service
func (s *CrossPairService) Restart() error {
	if err := s.Stop(); err != nil {
		return fmt.Errorf("failed to stop service: %w", err)
	}

	// Wait a moment before restarting
	time.Sleep(1 * time.Second)

	if err := s.Start(); err != nil {
		return fmt.Errorf("failed to start service: %w", err)
	}

	return nil
}

// Status returns the current status of the service
func (s *CrossPairService) Status() ServiceStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	status := ServiceStatus{
		Started:   s.started,
		StartTime: s.startTime,
		Uptime:    time.Since(s.startTime),
		Config:    s.config,
	}

	if s.started {
		status.Components = ComponentStatus{
			Engine:         s.engine != nil,
			RateCalculator: s.rateCalc != nil,
			WebSocket:      s.wsManager != nil,
			Storage:        s.storage != nil,
			MainServer:     s.mainServer != nil,
			AdminServer:    s.adminServer != nil,
		}

		if s.wsManager != nil {
			status.ConnectedClients = s.wsManager.GetConnectedClients()
		}
	}

	return status
}

// Health returns the health status of the service
func (s *CrossPairService) Health() HealthStatus {
	health := HealthStatus{
		Status:    "healthy",
		Timestamp: time.Now(),
		Version:   "1.0.0",
		Components: map[string]string{
			"service": "healthy",
		},
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.started {
		health.Status = "unhealthy"
		health.Components["service"] = "not_started"
		return health
	}

	// Check component health
	if s.engine == nil {
		health.Status = "degraded"
		health.Components["engine"] = "missing"
	} else {
		health.Components["engine"] = "healthy"
	}

	if s.rateCalc == nil {
		health.Status = "degraded"
		health.Components["rate_calculator"] = "missing"
	} else {
		health.Components["rate_calculator"] = "healthy"
	}

	if s.storage == nil {
		health.Status = "unhealthy"
		health.Components["storage"] = "missing"
	} else {
		health.Components["storage"] = "healthy"
	}

	if s.config.WebSocket.Enabled && s.wsManager == nil {
		health.Status = "degraded"
		health.Components["websocket"] = "missing"
	} else if s.config.WebSocket.Enabled {
		health.Components["websocket"] = "healthy"
	}

	return health
}

// runMigrations runs database migrations
func (s *CrossPairService) runMigrations() error {
	if s.config.Storage.Type == "postgres" {
		if pgStorage, ok := s.storage.(*PostgreSQLStorage); ok {
			// Run the migration SQL
			_, err := pgStorage.db.Exec(CreateTablesSQL)
			if err != nil {
				return fmt.Errorf("failed to run PostgreSQL migrations: %w", err)
			}
			log.Println("PostgreSQL migrations completed")
		}
	}
	return nil
}

// startBackgroundTasks starts background maintenance tasks
func (s *CrossPairService) startBackgroundTasks() {
	// Cleanup expired orders
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-s.ctx.Done():
				return
			case <-ticker.C:
				cutoff := time.Now().Add(-24 * time.Hour)
				if count, err := s.storage.CleanupExpiredOrders(s.ctx, cutoff); err != nil {
					log.Printf("Error cleaning up expired orders: %v", err)
				} else if count > 0 {
					log.Printf("Cleaned up %d expired orders", count)
				}
			}
		}
	}()

	// Metrics collection
	if s.config.Monitoring.EnableMetrics {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			ticker := time.NewTicker(s.config.Monitoring.MetricsInterval)
			defer ticker.Stop()

			for {
				select {
				case <-s.ctx.Done():
					return
				case <-ticker.C:
					s.collectMetrics()
				}
			}
		}()
	}
}

// collectMetrics collects and reports service metrics
func (s *CrossPairService) collectMetrics() {
	// TODO: Implement metrics collection
	// This would collect various metrics like:
	// - Active orders count
	// - Processing latency
	// - Error rates
	// - WebSocket connections
	// - etc.
}

// Data structures for service status

type ServiceStatus struct {
	Started          bool             `json:"started"`
	StartTime        time.Time        `json:"start_time"`
	Uptime           time.Duration    `json:"uptime"`
	Config           *CrossPairConfig `json:"config"`
	Components       ComponentStatus  `json:"components"`
	ConnectedClients int              `json:"connected_clients"`
}

type ComponentStatus struct {
	Engine         bool `json:"engine"`
	RateCalculator bool `json:"rate_calculator"`
	WebSocket      bool `json:"websocket"`
	Storage        bool `json:"storage"`
	MainServer     bool `json:"main_server"`
	AdminServer    bool `json:"admin_server"`
}

type HealthStatus struct {
	Status     string            `json:"status"`
	Timestamp  time.Time         `json:"timestamp"`
	Version    string            `json:"version"`
	Components map[string]string `json:"components"`
}
