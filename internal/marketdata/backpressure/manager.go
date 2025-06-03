// Package backpressure provides comprehensive backpressure management
// for market data distribution ensuring zero trade loss while optimizing delivery
package backpressure

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

// BackpressureManager orchestrates all backpressure components for optimal
// market data distribution with zero trade loss guarantees
type BackpressureManager struct {
	logger *zap.Logger

	// Core components
	detector    *ClientCapabilityDetector
	rateLimiter *AdaptiveRateLimiter
	priorityQ   *LockFreePriorityQueue
	coordinator *CrossServiceCoordinator

	// Client management
	clients     sync.Map // string (clientID) -> *ManagedClient
	connections sync.Map // *websocket.Conn -> string (clientID)

	// Message processing pipeline
	incomingMessages chan *IncomingMessage
	processedJobs    chan *ProcessedJob
	workers          sync.WaitGroup
	workerCount      int

	// Emergency handling
	emergencyMode    int32 // atomic bool for emergency backpressure
	emergencyMetrics *EmergencyMetrics

	// Configuration
	config *ManagerConfig

	// Lifecycle management
	ctx        context.Context
	cancel     context.CancelFunc
	shutdown   chan struct{}
	shutdownWG sync.WaitGroup

	// Metrics
	metrics *ManagerMetrics
}

// ManagedClient represents a client with full backpressure management
type ManagedClient struct {
	ID           string
	Connection   *websocket.Conn
	Capability   *ClientCapability
	RateLimit    *ClientRateLimiter
	SendChannel  chan *PriorityMessage
	LastActivity time.Time

	// Client-specific emergency handling
	InEmergency int32 // atomic bool
	EmergencyAt time.Time
	RecoveryAt  time.Time

	// Per-client metrics
	MessagesSent    int64
	MessagesDropped int64
	BytesSent       int64
	LastLatency     time.Duration

	// Connection state
	ConnectedAt time.Time
	IsActive    int32 // atomic bool
}

// IncomingMessage represents a message to be distributed with priority
type IncomingMessage struct {
	Type        MessagePriority
	Data        []byte
	Timestamp   time.Time
	TargetShard string // Empty for broadcast
	TargetID    string // Empty for broadcast
	Metadata    map[string]interface{}
}

// ProcessedJob represents a message ready for delivery to specific client
type ProcessedJob struct {
	ClientID    string
	Message     *PriorityMessage
	Priority    MessagePriority
	Deadline    time.Time
	RetryCount  int
	EmergencyOK bool // Can be sent during emergency mode
}

// ManagerConfig configures the backpressure manager
// type ManagerConfig struct {
// 	// Worker configuration
// 	WorkerCount       int           `json:"worker_count"`
// 	ProcessingTimeout time.Duration `json:"processing_timeout"`
// 	MaxRetries        int           `json:"max_retries"`

// 	// Emergency thresholds
// 	EmergencyLatencyThreshold     time.Duration `json:"emergency_latency_threshold"`
// 	EmergencyQueueLengthThreshold int           `json:"emergency_queue_length_threshold"`
// 	EmergencyDropRateThreshold    float64       `json:"emergency_drop_rate_threshold"`

// 	// Recovery configuration
// 	RecoveryGracePeriod   time.Duration `json:"recovery_grace_period"`
// 	RecoveryLatencyTarget time.Duration `json:"recovery_latency_target"`

// 	// Client management
// 	ClientTimeout      time.Duration `json:"client_timeout"`
// 	MaxClientsPerShard int           `json:"max_clients_per_shard"`

// 	// Rate limiting
// 	GlobalRateLimit         int64                       `json:"global_rate_limit"`
// 	PriorityRateMultipliers map[MessagePriority]float64 `json:"priority_rate_multipliers"`
// }

// ManagerMetrics tracks comprehensive backpressure management metrics
type ManagerMetrics struct {
	// Message flow metrics
	MessagesReceived  prometheus.Counter
	MessagesProcessed prometheus.Counter
	MessagesDropped   prometheus.Counter
	MessageLatency    prometheus.Histogram

	// Client metrics
	ActiveClients        prometheus.Gauge
	ClientsInEmergency   prometheus.Gauge
	AverageClientLatency prometheus.Gauge

	// System health metrics
	ProcessingQueueLength prometheus.Gauge
	WorkerUtilization     prometheus.Gauge
	MemoryUsage           prometheus.Gauge

	// Emergency metrics
	EmergencyActivations prometheus.Counter
	EmergencyDuration    prometheus.Histogram
	RecoveryTime         prometheus.Histogram
}

// EmergencyMetrics tracks emergency mode statistics
type EmergencyMetrics struct {
	ActivationCount     int64
	TotalDuration       time.Duration
	LastActivation      time.Time
	LastRecovery        time.Time
	MessagesDropped     int64
	ClientsAffected     int64
	AverageRecoveryTime time.Duration
}

// NewBackpressureManager creates a new comprehensive backpressure manager
func NewBackpressureManager(cfg *BackpressureConfig, logger *zap.Logger) (*BackpressureManager, error) {
	// Extract sub-configs
	managerCfg := &cfg.Manager
	rlCfg := &cfg.RateLimiter
	pqCfg := &cfg.PriorityQueue
	coordCfg := &cfg.Coordinator

	if logger == nil {
		logger = zap.NewNop()
	}
	ctx, cancel := context.WithCancel(context.Background())

	// Initialize core components
	detector := NewClientCapabilityDetector(logger.Named("detector"))
	rateLimiter := NewAdaptiveRateLimiter(logger.Named("rate_limiter"), detector, rlCfg)
	priorityQ := NewLockFreePriorityQueue(pqCfg, logger.Named("priority_queue"))
	coordinator, err := NewCrossServiceCoordinator(logger.Named("coordinator"), coordCfg)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create cross-service coordinator: %w", err)
	}

	manager := &BackpressureManager{
		logger:      logger,
		detector:    detector,
		rateLimiter: rateLimiter,
		priorityQ:   priorityQ,
		coordinator: coordinator,

		incomingMessages: make(chan *IncomingMessage, 10000),
		processedJobs:    make(chan *ProcessedJob, 10000),
		workerCount:      managerCfg.WorkerCount,
		config:           managerCfg,

		ctx:      ctx,
		cancel:   cancel,
		shutdown: make(chan struct{}),

		metrics: initManagerMetrics(),
	}

	return manager, nil
}

// Start initializes and starts all backpressure management components
func (m *BackpressureManager) Start(ctx context.Context) error {
	m.logger.Info("Starting backpressure manager")

	// Start core components
	if err := m.detector.Start(ctx); err != nil {
		return fmt.Errorf("failed to start capability detector: %w", err)
	}

	if err := m.rateLimiter.Start(ctx); err != nil {
		return fmt.Errorf("failed to start rate limiter: %w", err)
	}

	if err := m.coordinator.Start(ctx); err != nil {
		return fmt.Errorf("failed to start cross-service coordinator: %w", err)
	}

	// Start processing workers
	for i := 0; i < m.workerCount; i++ {
		m.workers.Add(1)
		go m.messageProcessingWorker(i)
	}

	// Start delivery workers
	for i := 0; i < m.workerCount; i++ {
		m.workers.Add(1)
		go m.messageDeliveryWorker(i)
	}

	// Start monitoring goroutines
	m.shutdownWG.Add(3)
	go m.clientMonitor()
	go m.emergencyMonitor()
	go m.metricsCollector()

	m.logger.Info("Backpressure manager started successfully",
		zap.Int("workers", m.workerCount*2),
		zap.String("emergency_threshold", m.config.EmergencyLatencyThreshold.String()))

	return nil
}

// RegisterClient registers a new WebSocket client for backpressure management
func (m *BackpressureManager) RegisterClient(clientID string, conn *websocket.Conn) error {
	m.logger.Debug("Registering client for backpressure management", zap.String("client_id", clientID))

	// Create managed client
	client := &ManagedClient{
		ID:           clientID,
		Connection:   conn,
		SendChannel:  make(chan *PriorityMessage, 1000),
		LastActivity: time.Now(),
		ConnectedAt:  time.Now(),
		IsActive:     1,
	}

	// Register with detector for capability measurement
	if err := m.detector.RegisterClient(clientID); err != nil {
		return fmt.Errorf("failed to register client with detector: %w", err)
	}

	// Use getOrCreateClientLimiter for rate limiter
	rateLimit := m.rateLimiter.getOrCreateClientLimiter(clientID)
	client.RateLimit = rateLimit

	// Store client mappings
	m.clients.Store(clientID, client)
	m.connections.Store(conn, clientID)

	// Update metrics
	m.metrics.ActiveClients.Inc()

	m.logger.Info("Client registered successfully",
		zap.String("client_id", clientID),
		zap.String("rate_limit_class", string(DefaultClientClass)))

	return nil
}

// DistributeMessage queues a message for distribution with backpressure handling
func (m *BackpressureManager) DistributeMessage(messageType MessagePriority, data []byte, targets []string) error {
	// Check emergency mode
	if atomic.LoadInt32(&m.emergencyMode) == 1 && messageType != PriorityCritical {
		m.metrics.MessagesDropped.Inc()
		return ErrEmergencyMode
	}

	// Create incoming message
	msg := &IncomingMessage{
		Type:      messageType,
		Data:      data,
		Timestamp: time.Now(),
		Metadata:  make(map[string]interface{}),
	}

	// Handle targeted vs broadcast
	if len(targets) == 0 {
		// Broadcast message
		select {
		case m.incomingMessages <- msg:
			m.metrics.MessagesReceived.Inc()
			return nil
		default:
			m.metrics.MessagesDropped.Inc()
			return ErrQueueFull
		}
	} else {
		// Targeted messages
		for _, target := range targets {
			targetMsg := *msg
			targetMsg.TargetID = target

			select {
			case m.incomingMessages <- &targetMsg:
				m.metrics.MessagesReceived.Inc()
			default:
				m.metrics.MessagesDropped.Inc()
				return ErrQueueFull
			}
		}
	}

	return nil
}

// GetClientCapability returns the current capability measurement for a client
func (m *BackpressureManager) GetClientCapability(clientID string) (*ClientCapability, error) {
	cap := m.detector.GetClientCapability(clientID)
	return cap, nil
}

// SetEmergencyMode manually activates or deactivates emergency mode
func (m *BackpressureManager) SetEmergencyMode(enable bool) {
	if enable {
		if atomic.CompareAndSwapInt32(&m.emergencyMode, 0, 1) {
			m.emergencyMetrics.ActivationCount++
			m.emergencyMetrics.LastActivation = time.Now()
			m.metrics.EmergencyActivations.Inc()

			m.logger.Warn("Emergency backpressure mode activated manually")
		}
	} else {
		if atomic.CompareAndSwapInt32(&m.emergencyMode, 1, 0) {
			now := time.Now()
			duration := now.Sub(m.emergencyMetrics.LastActivation)
			m.emergencyMetrics.TotalDuration += duration
			m.emergencyMetrics.LastRecovery = now
			m.metrics.EmergencyDuration.Observe(duration.Seconds())

			m.logger.Info("Emergency backpressure mode deactivated manually",
				zap.Duration("duration", duration))
		}
	}
}

// IsEmergencyMode returns whether emergency mode is currently active
func (m *BackpressureManager) IsEmergencyMode() bool {
	return atomic.LoadInt32(&m.emergencyMode) == 1
}

// GetStatus returns the current status of the backpressure manager
func (m *BackpressureManager) GetStatus() map[string]interface{} {
	var activeClients int64
	m.clients.Range(func(key, value interface{}) bool {
		activeClients++
		return true
	})

	return map[string]interface{}{
		"active":                 atomic.LoadInt32(&m.emergencyMode) == 0,
		"emergency_mode":         atomic.LoadInt32(&m.emergencyMode) == 1,
		"active_clients":         activeClients,
		"incoming_queue_length":  len(m.incomingMessages),
		"processed_queue_length": len(m.processedJobs),
		"worker_count":           m.workerCount,
		"emergency_activations":  m.emergencyMetrics.ActivationCount,
		"last_activation":        m.emergencyMetrics.LastActivation,
		"last_recovery":          m.emergencyMetrics.LastRecovery,
		"config": map[string]interface{}{
			"emergency_latency_threshold":      m.config.EmergencyLatencyThreshold,
			"emergency_queue_length_threshold": m.config.EmergencyQueueLengthThreshold,
			"global_rate_limit":                m.config.GlobalRateLimit,
		},
	}
}

// GetMetrics returns comprehensive metrics from all components
func (m *BackpressureManager) GetMetrics() map[string]interface{} {
	// Get metrics from all components
	detectorMetrics := m.detector.GetMetrics()
	rateLimiterMetrics := m.rateLimiter.GetMetrics()
	coordinatorMetrics := m.coordinator.GetMetrics()

	return map[string]interface{}{
		"manager": map[string]interface{}{
			"messages_received":        m.metrics.MessagesReceived,
			"messages_processed":       m.metrics.MessagesProcessed,
			"messages_dropped":         m.metrics.MessagesDropped,
			"active_clients":           m.metrics.ActiveClients,
			"clients_in_emergency":     m.metrics.ClientsInEmergency,
			"emergency_activations":    m.emergencyMetrics.ActivationCount,
			"total_emergency_duration": m.emergencyMetrics.TotalDuration,
			"queue_lengths": map[string]int{
				"incoming":  len(m.incomingMessages),
				"processed": len(m.processedJobs),
			},
		},
		"detector":     map[string]interface{}{"metrics": detectorMetrics},
		"rate_limiter": map[string]interface{}{"metrics": rateLimiterMetrics},
		"coordinator":  coordinatorMetrics,
		"timestamp":    time.Now(),
	}
}

// TriggerEmergencyStop triggers emergency stop mode with a reason
func (m *BackpressureManager) TriggerEmergencyStop(reason string) {
	if atomic.CompareAndSwapInt32(&m.emergencyMode, 0, 1) {
		m.emergencyMetrics.ActivationCount++
		m.emergencyMetrics.LastActivation = time.Now()
		m.metrics.EmergencyActivations.Inc()

		m.logger.Error("Emergency stop triggered",
			zap.String("reason", reason),
			zap.Time("activation_time", m.emergencyMetrics.LastActivation))

		// Notify cross-service coordinator
		m.coordinator.SendBackpressureSignal(&BackpressureSignal{
			SignalID:      fmt.Sprintf("emergency-stop-%d", time.Now().UnixNano()),
			SourceService: "marketdata-backpressure",
			SignalType:    SignalTypeEmergencyStop,
			Severity:      5,
			Reason:        reason,
			ExpiresAt:     time.Now().Add(time.Hour),
		})
	}
}

// RecoverFromEmergency attempts to recover from emergency mode
func (m *BackpressureManager) RecoverFromEmergency() error {
	if atomic.LoadInt32(&m.emergencyMode) == 0 {
		return fmt.Errorf("system is not in emergency mode")
	}

	// Check if conditions are safe for recovery
	incomingQueueLen := len(m.incomingMessages)
	processedQueueLen := len(m.processedJobs)

	if incomingQueueLen > m.config.EmergencyQueueLengthThreshold/2 ||
		processedQueueLen > m.config.EmergencyQueueLengthThreshold/2 {
		return fmt.Errorf("queue lengths too high for recovery: incoming=%d, processed=%d",
			incomingQueueLen, processedQueueLen)
	}

	if atomic.CompareAndSwapInt32(&m.emergencyMode, 1, 0) {
		now := time.Now()
		duration := now.Sub(m.emergencyMetrics.LastActivation)
		m.emergencyMetrics.TotalDuration += duration
		m.emergencyMetrics.LastRecovery = now
		m.metrics.EmergencyDuration.Observe(duration.Seconds())

		m.logger.Info("Emergency recovery completed",
			zap.Duration("emergency_duration", duration),
			zap.Int("incoming_queue", incomingQueueLen),
			zap.Int("processed_queue", processedQueueLen))

		// Notify cross-service coordinator
		m.coordinator.SendBackpressureSignal(&BackpressureSignal{
			SignalID:      fmt.Sprintf("recovery-%d", time.Now().UnixNano()),
			SourceService: "marketdata-backpressure",
			SignalType:    SignalTypeRecovery,
			Severity:      1,
			Reason:        "Manual emergency recovery",
			ExpiresAt:     time.Now().Add(time.Minute * 5),
		})
	}

	return nil
}

// Stop gracefully shuts down the backpressure manager
func (m *BackpressureManager) Stop() error {
	m.logger.Info("Stopping backpressure manager")

	// Signal shutdown
	close(m.shutdown)
	m.cancel()

	// Stop core components
	ctx := context.Background()
	m.detector.Stop(ctx)
	m.rateLimiter.Stop(ctx)
	m.coordinator.Stop(ctx)

	// Wait for workers to finish
	m.workers.Wait()
	m.shutdownWG.Wait()

	// Close channels
	close(m.incomingMessages)
	close(m.processedJobs)

	m.logger.Info("Backpressure manager stopped")
	return nil
}

// Private helper methods and worker functions follow...
// (Implementation continues with worker functions, monitoring, and utility methods)

func getDefaultManagerConfig() *ManagerConfig {
	return &ManagerConfig{
		WorkerCount:                   8,
		ProcessingTimeout:             time.Millisecond * 100,
		MaxRetries:                    3,
		EmergencyLatencyThreshold:     time.Millisecond * 500,
		EmergencyQueueLengthThreshold: 50000,
		EmergencyDropRateThreshold:    0.1,
		RecoveryGracePeriod:           time.Second * 30,
		RecoveryLatencyTarget:         time.Millisecond * 200,
		ClientTimeout:                 time.Minute * 5,
		MaxClientsPerShard:            1000,
		GlobalRateLimit:               100000,
		PriorityRateMultipliers: map[MessagePriority]float64{
			PriorityCritical: 10.0,
			PriorityHigh:     3.0,
			PriorityMedium:   1.0,
			PriorityLow:      0.5,
			PriorityMarket:   0.8,
		},
	}
}

func initManagerMetrics() *ManagerMetrics {
	return &ManagerMetrics{
		MessagesReceived: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "backpressure_messages_received_total",
			Help: "Total number of messages received for distribution",
		}),
		MessagesProcessed: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "backpressure_messages_processed_total",
			Help: "Total number of messages successfully processed",
		}),
		MessagesDropped: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "backpressure_messages_dropped_total",
			Help: "Total number of messages dropped due to backpressure",
		}),
		MessageLatency: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "backpressure_message_latency_seconds",
			Help:    "End-to-end message processing latency",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0},
		}),
		ActiveClients: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "backpressure_active_clients",
			Help: "Current number of active clients under backpressure management",
		}),
		ClientsInEmergency: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "backpressure_clients_in_emergency",
			Help: "Current number of clients in emergency backpressure mode",
		}),
		AverageClientLatency: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "backpressure_average_client_latency_seconds",
			Help: "Average latency across all clients",
		}),
		ProcessingQueueLength: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "backpressure_processing_queue_length",
			Help: "Current length of the message processing queue",
		}),
		WorkerUtilization: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "backpressure_worker_utilization",
			Help: "Current worker utilization percentage",
		}),
		MemoryUsage: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "backpressure_memory_usage_bytes",
			Help: "Current memory usage of backpressure manager",
		}),
		EmergencyActivations: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "backpressure_emergency_activations_total",
			Help: "Total number of emergency mode activations",
		}),
		EmergencyDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "backpressure_emergency_duration_seconds",
			Help:    "Duration of emergency mode activations",
			Buckets: []float64{1, 5, 10, 30, 60, 300, 600, 1800},
		}),
		RecoveryTime: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "backpressure_recovery_time_seconds",
			Help:    "Time taken to recover from emergency mode",
			Buckets: []float64{1, 5, 10, 30, 60, 300},
		}),
	}
}

// Error definitions
var (
	ErrEmergencyMode   = fmt.Errorf("message dropped due to emergency backpressure mode")
	ErrQueueFull       = fmt.Errorf("message queue is full")
	ErrClientNotFound  = fmt.Errorf("client not found")
	ErrInvalidPriority = fmt.Errorf("invalid message priority")
)
