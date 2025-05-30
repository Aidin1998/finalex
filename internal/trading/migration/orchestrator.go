// =============================
// Migration Orchestrator
// =============================
// This file implements the migration orchestrator that provides a REST API
// and web dashboard for managing and monitoring migrations.

package migration

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// MigrationOrchestrator provides REST API and dashboard for migration management
type MigrationOrchestrator struct {
	coordinator   *Coordinator
	safetyManager *SafetyManager
	logger        *zap.SugaredLogger

	// HTTP server
	httpServer *http.Server
	router     *mux.Router

	// WebSocket for real-time updates
	upgrader      websocket.Upgrader
	wsConnections map[string]*websocket.Conn
	wsConnsMu     sync.RWMutex

	// Dashboard templates
	templates *template.Template

	// Configuration
	config *OrchestratorConfig

	// Real-time monitoring
	metricsBuffer []MigrationMetrics
	metricsMu     sync.RWMutex
	eventBuffer   []MigrationEvent
	eventsMu      sync.RWMutex

	// Control
	stopChan chan struct{}
	running  int64 // atomic
}

// OrchestratorConfig contains orchestrator configuration
type OrchestratorConfig struct {
	// Server configuration
	ListenAddr   string        `json:"listen_addr" yaml:"listen_addr"`
	ReadTimeout  time.Duration `json:"read_timeout" yaml:"read_timeout"`
	WriteTimeout time.Duration `json:"write_timeout" yaml:"write_timeout"`

	// Dashboard configuration
	EnableDashboard  bool   `json:"enable_dashboard" yaml:"enable_dashboard"`
	DashboardPath    string `json:"dashboard_path" yaml:"dashboard_path"`
	StaticAssetsPath string `json:"static_assets_path" yaml:"static_assets_path"`

	// API configuration
	EnableAPI  bool   `json:"enable_api" yaml:"enable_api"`
	APIPrefix  string `json:"api_prefix" yaml:"api_prefix"`
	EnableCORS bool   `json:"enable_cors" yaml:"enable_cors"`

	// Real-time updates
	EnableWebSocket  bool          `json:"enable_websocket" yaml:"enable_websocket"`
	WSUpdateInterval time.Duration `json:"ws_update_interval" yaml:"ws_update_interval"`
	MaxWSConnections int           `json:"max_ws_connections" yaml:"max_ws_connections"`

	// Buffer configuration
	MetricsBufferSize int           `json:"metrics_buffer_size" yaml:"metrics_buffer_size"`
	EventBufferSize   int           `json:"event_buffer_size" yaml:"event_buffer_size"`
	BufferRetention   time.Duration `json:"buffer_retention" yaml:"buffer_retention"`

	// Authentication
	EnableAuth bool     `json:"enable_auth" yaml:"enable_auth"`
	APIKeys    []string `json:"api_keys" yaml:"api_keys"`

	// Rate limiting
	EnableRateLimit   bool `json:"enable_rate_limit" yaml:"enable_rate_limit"`
	RequestsPerMinute int  `json:"requests_per_minute" yaml:"requests_per_minute"`
	BurstSize         int  `json:"burst_size" yaml:"burst_size"`
}

// MigrationRequest represents a migration request from the API
type MigrationRequest struct {
	OrderBookID    string                 `json:"order_book_id" validate:"required"`
	SourceStrategy string                 `json:"source_strategy" validate:"required"`
	TargetStrategy string                 `json:"target_strategy" validate:"required"`
	MigrationMode  MigrationMode          `json:"migration_mode"`
	Config         map[string]interface{} `json:"config,omitempty"`
	Priority       int                    `json:"priority"`
	ScheduledTime  *time.Time             `json:"scheduled_time,omitempty"`
	DryRun         bool                   `json:"dry_run"`
	Description    string                 `json:"description,omitempty"`
}

// MigrationResponse represents the API response for migration operations
type MigrationResponse struct {
	Success     bool        `json:"success"`
	MigrationID uuid.UUID   `json:"migration_id,omitempty"`
	Message     string      `json:"message,omitempty"`
	Data        interface{} `json:"data,omitempty"`
	Timestamp   time.Time   `json:"timestamp"`
}

// DashboardData contains data for the dashboard
type DashboardData struct {
	// Summary statistics
	TotalMigrations  int64         `json:"total_migrations"`
	ActiveMigrations int64         `json:"active_migrations"`
	SuccessRate      float64       `json:"success_rate"`
	AverageLatency   time.Duration `json:"average_latency"`

	// Current migrations
	Migrations []MigrationStateInfo `json:"migrations"`

	// Participants status
	Participants []ParticipantStatus `json:"participants"`

	// Recent events
	RecentEvents []MigrationEvent `json:"recent_events"`

	// Performance metrics
	PerformanceMetrics *PerformanceSnapshot `json:"performance_metrics"`

	// System health
	SystemHealth *SystemHealthInfo `json:"system_health"`

	// Configuration
	Configuration *OrchestratorConfigInfo `json:"configuration"`

	// Last updated
	LastUpdated time.Time `json:"last_updated"`
}

// MigrationStateInfo provides detailed migration state information
type MigrationStateInfo struct {
	ID                uuid.UUID              `json:"id"`
	OrderBookID       string                 `json:"order_book_id"`
	Phase             MigrationPhase         `json:"phase"`
	Status            MigrationStatus        `json:"status"`
	Progress          float64                `json:"progress"`
	StartTime         time.Time              `json:"start_time"`
	EstimatedDuration *time.Duration         `json:"estimated_duration,omitempty"`
	ElapsedTime       time.Duration          `json:"elapsed_time"`
	CurrentOperation  string                 `json:"current_operation"`
	Participants      []ParticipantStateInfo `json:"participants"`
	ErrorMessage      string                 `json:"error_message,omitempty"`
	Metrics           *MigrationMetrics      `json:"metrics,omitempty"`
	HealthStatus      *HealthStatus          `json:"health_status,omitempty"`
}

// ParticipantStatus provides participant status information
type ParticipantStatus struct {
	ID            string                 `json:"id"`
	Type          string                 `json:"type"`
	IsHealthy     bool                   `json:"is_healthy"`
	LastHeartbeat time.Time              `json:"last_heartbeat"`
	Status        string                 `json:"status"`
	ErrorMessage  string                 `json:"error_message,omitempty"`
	Metrics       map[string]interface{} `json:"metrics,omitempty"`
}

// ParticipantStateInfo provides detailed participant state
type ParticipantStateInfo struct {
	ID              string                 `json:"id"`
	Vote            ParticipantVote        `json:"vote"`
	IsHealthy       bool                   `json:"is_healthy"`
	LastHeartbeat   time.Time              `json:"last_heartbeat"`
	ErrorMessage    string                 `json:"error_message,omitempty"`
	PreparationData map[string]interface{} `json:"preparation_data,omitempty"`
}

// PerformanceSnapshot provides current performance metrics
type PerformanceSnapshot struct {
	TotalOperations     int64              `json:"total_operations"`
	OperationsPerSecond float64            `json:"operations_per_second"`
	AverageLatencyMs    float64            `json:"average_latency_ms"`
	LatencyP95Ms        float64            `json:"latency_p95_ms"`
	LatencyP99Ms        float64            `json:"latency_p99_ms"`
	ErrorRate           float64            `json:"error_rate"`
	ResourceUtilization map[string]float64 `json:"resource_utilization"`
	LastUpdated         time.Time          `json:"last_updated"`
}

// SystemHealthInfo provides system health information
type SystemHealthInfo struct {
	OverallHealth   string            `json:"overall_health"`
	HealthScore     float64           `json:"health_score"`
	LastCheck       time.Time         `json:"last_check"`
	Issues          []string          `json:"issues,omitempty"`
	Warnings        []string          `json:"warnings,omitempty"`
	ComponentHealth map[string]string `json:"component_health"`
}

// OrchestratorConfigInfo provides configuration information for the dashboard
type OrchestratorConfigInfo struct {
	Version               string   `json:"version"`
	Environment           string   `json:"environment"`
	EnabledFeatures       []string `json:"enabled_features"`
	SafetyEnabled         bool     `json:"safety_enabled"`
	AutoRollbackEnabled   bool     `json:"auto_rollback_enabled"`
	PerformanceMonitoring bool     `json:"performance_monitoring"`
}

// NewMigrationOrchestrator creates a new migration orchestrator
func NewMigrationOrchestrator(
	coordinator *Coordinator,
	safetyManager *SafetyManager,
	config *OrchestratorConfig,
	logger *zap.SugaredLogger,
) *MigrationOrchestrator {
	if config == nil {
		config = DefaultOrchestratorConfig()
	}

	orchestrator := &MigrationOrchestrator{
		coordinator:   coordinator,
		safetyManager: safetyManager,
		logger:        logger,
		config:        config,
		wsConnections: make(map[string]*websocket.Conn),
		metricsBuffer: make([]MigrationMetrics, 0, config.MetricsBufferSize),
		eventBuffer:   make([]MigrationEvent, 0, config.EventBufferSize),
		stopChan:      make(chan struct{}),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return config.EnableCORS // Allow all origins if CORS is enabled
			},
		},
	}

	// Initialize router
	orchestrator.router = mux.NewRouter()
	orchestrator.setupRoutes()

	// Load templates if dashboard is enabled
	if config.EnableDashboard {
		orchestrator.loadTemplates()
	}

	// Subscribe to migration events
	orchestrator.subscribeToEvents()

	return orchestrator
}

// DefaultOrchestratorConfig returns default configuration
func DefaultOrchestratorConfig() *OrchestratorConfig {
	return &OrchestratorConfig{
		ListenAddr:        ":8082",
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		EnableDashboard:   true,
		DashboardPath:     "/dashboard",
		StaticAssetsPath:  "./static",
		EnableAPI:         true,
		APIPrefix:         "/api/v1",
		EnableCORS:        true,
		EnableWebSocket:   true,
		WSUpdateInterval:  5 * time.Second,
		MaxWSConnections:  100,
		MetricsBufferSize: 1000,
		EventBufferSize:   1000,
		BufferRetention:   24 * time.Hour,
		EnableAuth:        false,
		EnableRateLimit:   true,
		RequestsPerMinute: 60,
		BurstSize:         10,
	}
}

// Start starts the orchestrator HTTP server
func (o *MigrationOrchestrator) Start() error {
	if !atomic.CompareAndSwapInt64(&o.running, 0, 1) {
		return fmt.Errorf("orchestrator is already running")
	}

	o.logger.Info("Starting migration orchestrator",
		"listen_addr", o.config.ListenAddr,
		"dashboard_enabled", o.config.EnableDashboard,
		"api_enabled", o.config.EnableAPI,
		"websocket_enabled", o.config.EnableWebSocket,
	)

	// Create HTTP server
	o.httpServer = &http.Server{
		Addr:         o.config.ListenAddr,
		Handler:      o.router,
		ReadTimeout:  o.config.ReadTimeout,
		WriteTimeout: o.config.WriteTimeout,
	}

	// Start background workers
	go o.metricsCollectionWorker()
	go o.eventBufferCleanupWorker()
	if o.config.EnableWebSocket {
		go o.webSocketBroadcastWorker()
	}

	// Start HTTP server
	go func() {
		if err := o.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			o.logger.Errorw("HTTP server error", "error", err)
		}
	}()

	o.logger.Info("Migration orchestrator started successfully")
	return nil
}

// Stop stops the orchestrator
func (o *MigrationOrchestrator) Stop() error {
	if !atomic.CompareAndSwapInt64(&o.running, 1, 0) {
		return fmt.Errorf("orchestrator is not running")
	}

	o.logger.Info("Stopping migration orchestrator")

	// Signal stop to workers
	close(o.stopChan)

	// Close WebSocket connections
	o.wsConnsMu.Lock()
	for _, conn := range o.wsConnections {
		conn.Close()
	}
	o.wsConnections = make(map[string]*websocket.Conn)
	o.wsConnsMu.Unlock()

	// Shutdown HTTP server
	if o.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := o.httpServer.Shutdown(ctx); err != nil {
			o.logger.Errorw("Error shutting down HTTP server", "error", err)
			return err
		}
	}

	o.logger.Info("Migration orchestrator stopped")
	return nil
}

// setupRoutes configures all HTTP routes
func (o *MigrationOrchestrator) setupRoutes() {
	// Enable CORS if configured
	if o.config.EnableCORS {
		o.router.Use(o.corsMiddleware)
	}

	// Add authentication middleware if enabled
	if o.config.EnableAuth {
		o.router.Use(o.authMiddleware)
	}

	// Add rate limiting if enabled
	if o.config.EnableRateLimit {
		o.router.Use(o.rateLimitMiddleware)
	}

	// Add logging middleware
	o.router.Use(o.loggingMiddleware)

	// Dashboard routes
	if o.config.EnableDashboard {
		o.setupDashboardRoutes()
	}

	// API routes
	if o.config.EnableAPI {
		o.setupAPIRoutes()
	}

	// WebSocket routes
	if o.config.EnableWebSocket {
		o.setupWebSocketRoutes()
	}

	// Health check endpoint
	o.router.HandleFunc("/health", o.handleHealth).Methods("GET")
}

// setupDashboardRoutes configures dashboard routes
func (o *MigrationOrchestrator) setupDashboardRoutes() {
	dashboardRouter := o.router.PathPrefix(o.config.DashboardPath).Subrouter()

	// Main dashboard page
	dashboardRouter.HandleFunc("", o.handleDashboard).Methods("GET")
	dashboardRouter.HandleFunc("/", o.handleDashboard).Methods("GET")

	// Dashboard data endpoint
	dashboardRouter.HandleFunc("/data", o.handleDashboardData).Methods("GET")

	// Static assets
	if o.config.StaticAssetsPath != "" {
		staticHandler := http.StripPrefix(
			o.config.DashboardPath+"/static/",
			http.FileServer(http.Dir(o.config.StaticAssetsPath)),
		)
		dashboardRouter.PathPrefix("/static/").Handler(staticHandler)
	}
}

// setupAPIRoutes configures API routes
func (o *MigrationOrchestrator) setupAPIRoutes() {
	apiRouter := o.router.PathPrefix(o.config.APIPrefix).Subrouter()

	// Migration management endpoints
	migrationRouter := apiRouter.PathPrefix("/migrations").Subrouter()
	migrationRouter.HandleFunc("", o.handleListMigrations).Methods("GET")
	migrationRouter.HandleFunc("", o.handleCreateMigration).Methods("POST")
	migrationRouter.HandleFunc("/{id}", o.handleGetMigration).Methods("GET")
	migrationRouter.HandleFunc("/{id}/abort", o.handleAbortMigration).Methods("POST")
	migrationRouter.HandleFunc("/{id}/resume", o.handleResumeMigration).Methods("POST")
	migrationRouter.HandleFunc("/{id}/retry", o.handleRetryMigration).Methods("POST")

	// Participant management endpoints
	participantRouter := apiRouter.PathPrefix("/participants").Subrouter()
	participantRouter.HandleFunc("", o.handleListParticipants).Methods("GET")
	participantRouter.HandleFunc("/{id}", o.handleGetParticipant).Methods("GET")
	participantRouter.HandleFunc("/{id}/health", o.handleParticipantHealth).Methods("GET")

	// System status endpoints
	statusRouter := apiRouter.PathPrefix("/status").Subrouter()
	statusRouter.HandleFunc("/health", o.handleSystemHealth).Methods("GET")
	statusRouter.HandleFunc("/metrics", o.handleSystemMetrics).Methods("GET")
	statusRouter.HandleFunc("/performance", o.handlePerformanceMetrics).Methods("GET")

	// Safety endpoints
	safetyRouter := apiRouter.PathPrefix("/safety").Subrouter()
	safetyRouter.HandleFunc("/status", o.handleSafetyStatus).Methods("GET")
	safetyRouter.HandleFunc("/rollback/{id}", o.handleSafetyRollback).Methods("POST")
	safetyRouter.HandleFunc("/circuit-breaker/status", o.handleCircuitBreakerStatus).Methods("GET")
	safetyRouter.HandleFunc("/circuit-breaker/reset", o.handleCircuitBreakerReset).Methods("POST")

	// Configuration endpoints
	configRouter := apiRouter.PathPrefix("/config").Subrouter()
	configRouter.HandleFunc("", o.handleGetConfig).Methods("GET")
	configRouter.HandleFunc("", o.handleUpdateConfig).Methods("PUT")

	// Event streaming endpoints
	eventRouter := apiRouter.PathPrefix("/events").Subrouter()
	eventRouter.HandleFunc("", o.handleGetEvents).Methods("GET")
	eventRouter.HandleFunc("/stream", o.handleEventStream).Methods("GET")
}

// setupWebSocketRoutes configures WebSocket routes
func (o *MigrationOrchestrator) setupWebSocketRoutes() {
	o.router.HandleFunc("/ws", o.handleWebSocket).Methods("GET")
	o.router.HandleFunc("/ws/events", o.handleWebSocketEvents).Methods("GET")
	o.router.HandleFunc("/ws/metrics", o.handleWebSocketMetrics).Methods("GET")
}
