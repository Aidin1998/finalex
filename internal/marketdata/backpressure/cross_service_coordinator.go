// Package backpressure provides cross-service backpressure coordination
// for maintaining system stability during high load with zero trade loss
package backpressure

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/IBM/sarama"
	"go.uber.org/zap"
)

// CrossServiceCoordinator manages backpressure signals across all services
// ensuring coordinated response to system stress while preserving critical flows
type CrossServiceCoordinator struct {
	logger *zap.Logger

	// Kafka infrastructure
	producer sarama.SyncProducer
	consumer sarama.ConsumerGroup

	// Service registration and health tracking
	services   map[string]*ServiceState
	servicesMu sync.RWMutex

	// Global backpressure state
	globalBackpressure int64 // 0-5 severity level
	emergencyMode      int64 // 0=normal, 1=emergency

	// Signal processing
	signalProcessor *BackpressureSignalProcessor

	// Configuration
	config *CoordinatorConfig

	// Lifecycle
	workers      sync.WaitGroup
	shutdown     chan struct{}
	shutdownOnce sync.Once

	// Metrics
	metrics *CoordinatorMetrics
}

// ServiceState tracks individual service health and backpressure state
type ServiceState struct {
	ServiceID   string
	ServiceType ServiceType
	LastSeen    time.Time

	// Current state
	BackpressureLevel int64   // Service-reported backpressure level
	CPUUsage          float64 // CPU utilization percentage
	MemoryUsage       float64 // Memory utilization percentage
	QueueDepth        int64   // Message queue depth
	ErrorRate         float64 // Error rate percentage

	// Capabilities
	CanShedLoad     bool // Can shed non-critical load
	CanSlowDown     bool // Can slow down processing
	CriticalService bool // Critical for trading operations

	// Adaptive thresholds
	LoadThreshold  float64 // Load threshold for backpressure
	ErrorThreshold float64 // Error threshold for backpressure

	// Health status
	Healthy         bool
	LastHealthCheck time.Time

	mu sync.RWMutex
}

// ServiceType represents different types of services in the system
type ServiceType int

const (
	ServiceTypeUnknown         ServiceType = iota
	ServiceTypeMatchingEngine              // Core matching engine - CRITICAL
	ServiceTypeSettlement                  // Settlement service - CRITICAL
	ServiceTypeMarketData                  // Market data distribution
	ServiceTypeOrderManagement             // Order management
	ServiceTypeRiskManagement              // Risk management
	ServiceTypeCompliance                  // Compliance checking
	ServiceTypeUserManagement              // User management
	ServiceTypeAPIGateway                  // API gateway
	ServiceTypeWebSocket                   // WebSocket service
	ServiceTypeAnalytics                   // Analytics service
)

// BackpressureSignal represents a backpressure signal sent between services
type BackpressureSignal struct {
	SignalID      string                 `json:"signal_id"`
	SourceService string                 `json:"source_service"`
	TargetService string                 `json:"target_service,omitempty"` // Empty = broadcast
	SignalType    SignalType             `json:"signal_type"`
	Severity      int                    `json:"severity"` // 0-5 severity level
	Reason        string                 `json:"reason"`
	Timestamp     time.Time              `json:"timestamp"`
	ExpiresAt     time.Time              `json:"expires_at"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

// SignalType represents different types of backpressure signals
type SignalType int

const (
	SignalTypeLoadShedding SignalType = iota
	SignalTypeSlowDown
	SignalTypeThrottling
	SignalTypeEmergencyStop
	SignalTypeHealthCheck
	SignalTypeCapacityUpdate
	SignalTypeRecovery
)

// BackpressureSignalProcessor processes and routes backpressure signals
type BackpressureSignalProcessor struct {
	coordinator *CrossServiceCoordinator

	// Signal handlers by type
	handlers map[SignalType]SignalHandler

	// Signal routing
	routes map[string][]string // service -> list of dependent services

	// Processing state
	processing int64 // 0=stopped, 1=running

	mu sync.RWMutex
}

// SignalHandler processes specific types of backpressure signals
type SignalHandler interface {
	HandleSignal(ctx context.Context, signal *BackpressureSignal) error
}

// CoordinatorConfig configures the cross-service coordinator
// [REMOVED] type CoordinatorConfig struct { ... }

// CoordinatorMetrics tracks cross-service coordination metrics
type CoordinatorMetrics struct {
	// Signal metrics
	SignalsSent      int64
	SignalsReceived  int64
	SignalsProcessed int64
	SignalErrors     int64

	// Service health
	HealthyServices   int64
	UnhealthyServices int64
	CriticalServices  int64

	// Backpressure events
	LoadSheddingEvents int64
	ThrottlingEvents   int64
	EmergencyStops     int64
	RecoveryEvents     int64

	// System state
	GlobalBackpressureLevel int64
	ServicesInBackpressure  int64

	_ [6]int64 // Cache line padding
}

// DefaultCoordinatorConfig returns sensible defaults
func DefaultCoordinatorConfig() *CoordinatorConfig {
	return &CoordinatorConfig{
		KafkaBrokers:      []string{"localhost:9092"},
		BackpressureTopic: "backpressure-coordination",
		ConsumerGroup:     "backpressure-coordinator",

		HealthCheckInterval: 5 * time.Second,
		ServiceTimeout:      30 * time.Second,

		GlobalLoadThreshold: 80.0, // 80% global load threshold
		EmergencyThreshold:  95.0, // 95% emergency threshold

		SignalBufferSize: 1000,
		SignalTimeout:    10 * time.Second,

		CriticalServices: []string{
			"matching-engine",
			"settlement-service",
			"risk-management",
		},
		SheddableServices: []string{
			"analytics",
			"user-management",
			"compliance",
		},
	}
}

// NewCrossServiceCoordinator creates a new cross-service backpressure coordinator
func NewCrossServiceCoordinator(
	logger *zap.Logger,
	config *CoordinatorConfig,
) (*CrossServiceCoordinator, error) {
	if config == nil {
		config = DefaultCoordinatorConfig()
	}

	// Create Kafka producer
	producerConfig := sarama.NewConfig()
	producerConfig.Producer.RequiredAcks = sarama.WaitForAll
	producerConfig.Producer.Retry.Max = 3
	producerConfig.Producer.Return.Successes = true

	producer, err := sarama.NewSyncProducer(config.KafkaBrokers, producerConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kafka producer: %w", err)
	}

	// Create Kafka consumer
	consumerConfig := sarama.NewConfig()
	consumerConfig.Consumer.Group.Rebalance.Strategy = sarama.BalanceStrategyRoundRobin
	consumerConfig.Consumer.Offsets.Initial = sarama.OffsetNewest

	consumer, err := sarama.NewConsumerGroup(config.KafkaBrokers, config.ConsumerGroup, consumerConfig)
	if err != nil {
		producer.Close()
		return nil, fmt.Errorf("failed to create Kafka consumer: %w", err)
	}

	coordinator := &CrossServiceCoordinator{
		logger:   logger,
		producer: producer,
		consumer: consumer,
		services: make(map[string]*ServiceState),
		config:   config,
		shutdown: make(chan struct{}),
		metrics:  &CoordinatorMetrics{},
	}

	// Initialize signal processor
	coordinator.signalProcessor = &BackpressureSignalProcessor{
		coordinator: coordinator,
		handlers:    make(map[SignalType]SignalHandler),
		routes:      make(map[string][]string),
	}

	// Register default signal handlers
	coordinator.registerDefaultHandlers()

	return coordinator, nil
}

// Start begins cross-service backpressure coordination
func (csc *CrossServiceCoordinator) Start(ctx context.Context) error {
	csc.logger.Info("Starting cross-service backpressure coordinator")

	// Start signal processor
	atomic.StoreInt64(&csc.signalProcessor.processing, 1)

	// Start worker goroutines
	csc.workers.Add(3)
	go csc.signalConsumer(ctx)
	go csc.healthMonitor(ctx)
	go csc.backpressureEngine(ctx)

	return nil
}

// RegisterService registers a service for backpressure coordination
func (csc *CrossServiceCoordinator) RegisterService(serviceID string, serviceType ServiceType, config ServiceConfig) error {
	csc.servicesMu.Lock()
	defer csc.servicesMu.Unlock()

	service := &ServiceState{
		ServiceID:       serviceID,
		ServiceType:     serviceType,
		LastSeen:        time.Now(),
		CanShedLoad:     config.CanShedLoad,
		CanSlowDown:     config.CanSlowDown,
		CriticalService: config.CriticalService,
		LoadThreshold:   config.LoadThreshold,
		ErrorThreshold:  config.ErrorThreshold,
		Healthy:         true,
		LastHealthCheck: time.Now(),
	}

	csc.services[serviceID] = service

	csc.logger.Info("Service registered for backpressure coordination",
		zap.String("service_id", serviceID),
		zap.Int("service_type", int(serviceType)),
		zap.Bool("critical", config.CriticalService))

	return nil
}

// ServiceConfig configures service registration
type ServiceConfig struct {
	CanShedLoad     bool
	CanSlowDown     bool
	CriticalService bool
	LoadThreshold   float64
	ErrorThreshold  float64
}

// UpdateServiceHealth updates service health metrics
func (csc *CrossServiceCoordinator) UpdateServiceHealth(serviceID string, health ServiceHealth) {
	csc.servicesMu.RLock()
	service, exists := csc.services[serviceID]
	csc.servicesMu.RUnlock()

	if !exists {
		return
	}

	service.mu.Lock()
	service.LastSeen = time.Now()
	service.CPUUsage = health.CPUUsage
	service.MemoryUsage = health.MemoryUsage
	service.QueueDepth = health.QueueDepth
	service.ErrorRate = health.ErrorRate
	service.Healthy = health.Healthy
	service.LastHealthCheck = time.Now()
	service.mu.Unlock()

	// Check if service needs backpressure
	csc.evaluateServiceBackpressure(service)
}

// ServiceHealth represents service health metrics
type ServiceHealth struct {
	CPUUsage    float64
	MemoryUsage float64
	QueueDepth  int64
	ErrorRate   float64
	Healthy     bool
}

// SendBackpressureSignal sends a backpressure signal to other services
func (csc *CrossServiceCoordinator) SendBackpressureSignal(signal *BackpressureSignal) error {
	signal.Timestamp = time.Now()
	if signal.ExpiresAt.IsZero() {
		signal.ExpiresAt = time.Now().Add(csc.config.SignalTimeout)
	}

	// Serialize signal
	signalData, err := json.Marshal(signal)
	if err != nil {
		return fmt.Errorf("failed to marshal backpressure signal: %w", err)
	}

	// Send to Kafka
	message := &sarama.ProducerMessage{
		Topic: csc.config.BackpressureTopic,
		Key:   sarama.StringEncoder(signal.SourceService),
		Value: sarama.ByteEncoder(signalData),
		Headers: []sarama.RecordHeader{
			{Key: []byte("signal_type"), Value: []byte(fmt.Sprintf("%d", signal.SignalType))},
			{Key: []byte("severity"), Value: []byte(fmt.Sprintf("%d", signal.Severity))},
		},
	}

	_, _, err = csc.producer.SendMessage(message)
	if err != nil {
		atomic.AddInt64(&csc.metrics.SignalErrors, 1)
		return fmt.Errorf("failed to send backpressure signal: %w", err)
	}

	atomic.AddInt64(&csc.metrics.SignalsSent, 1)

	csc.logger.Debug("Backpressure signal sent",
		zap.String("source", signal.SourceService),
		zap.String("target", signal.TargetService),
		zap.Int("signal_type", int(signal.SignalType)),
		zap.Int("severity", signal.Severity))

	return nil
}

// signalConsumer consumes backpressure signals from Kafka
func (csc *CrossServiceCoordinator) signalConsumer(ctx context.Context) {
	defer csc.workers.Done()

	handler := &backpressureConsumerHandler{coordinator: csc}

	csc.logger.Info("Backpressure signal consumer started")

	for {
		select {
		case <-ctx.Done():
			return
		case <-csc.shutdown:
			csc.logger.Info("Backpressure signal consumer shutting down")
			return
		default:
			err := csc.consumer.Consume(ctx, []string{csc.config.BackpressureTopic}, handler)
			if err != nil {
				csc.logger.Error("Consumer error", zap.Error(err))
				time.Sleep(time.Second) // Brief pause before retry
			}
		}
	}
}

// backpressureConsumerHandler handles Kafka messages
type backpressureConsumerHandler struct {
	coordinator *CrossServiceCoordinator
}

func (h *backpressureConsumerHandler) Setup(sarama.ConsumerGroupSession) error   { return nil }
func (h *backpressureConsumerHandler) Cleanup(sarama.ConsumerGroupSession) error { return nil }

func (h *backpressureConsumerHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for {
		select {
		case message := <-claim.Messages():
			if message == nil {
				return nil
			}

			// Parse backpressure signal
			var signal BackpressureSignal
			if err := json.Unmarshal(message.Value, &signal); err != nil {
				h.coordinator.logger.Error("Failed to unmarshal backpressure signal", zap.Error(err))
				session.MarkMessage(message, "")
				continue
			}

			// Process signal
			if err := h.coordinator.processBackpressureSignal(session.Context(), &signal); err != nil {
				h.coordinator.logger.Error("Failed to process backpressure signal", zap.Error(err))
			}

			atomic.AddInt64(&h.coordinator.metrics.SignalsReceived, 1)
			session.MarkMessage(message, "")

		case <-session.Context().Done():
			return nil
		}
	}
}

// processBackpressureSignal processes a received backpressure signal
func (csc *CrossServiceCoordinator) processBackpressureSignal(ctx context.Context, signal *BackpressureSignal) error {
	// Check if signal has expired
	if time.Now().After(signal.ExpiresAt) {
		return nil // Ignore expired signals
	}

	// Route signal to appropriate handler
	return csc.signalProcessor.ProcessSignal(ctx, signal)
}

// ProcessSignal processes a backpressure signal
func (bsp *BackpressureSignalProcessor) ProcessSignal(ctx context.Context, signal *BackpressureSignal) error {
	if atomic.LoadInt64(&bsp.processing) == 0 {
		return fmt.Errorf("signal processor not running")
	}

	bsp.mu.RLock()
	handler, exists := bsp.handlers[signal.SignalType]
	bsp.mu.RUnlock()

	if !exists {
		return fmt.Errorf("no handler for signal type: %d", signal.SignalType)
	}

	err := handler.HandleSignal(ctx, signal)
	if err == nil {
		atomic.AddInt64(&bsp.coordinator.metrics.SignalsProcessed, 1)
	} else {
		atomic.AddInt64(&bsp.coordinator.metrics.SignalErrors, 1)
	}

	return err
}

// registerDefaultHandlers registers default signal handlers
func (csc *CrossServiceCoordinator) registerDefaultHandlers() {
	csc.signalProcessor.handlers[SignalTypeLoadShedding] = &LoadSheddingHandler{coordinator: csc}
	csc.signalProcessor.handlers[SignalTypeSlowDown] = &SlowDownHandler{coordinator: csc}
	csc.signalProcessor.handlers[SignalTypeThrottling] = &ThrottlingHandler{coordinator: csc}
	csc.signalProcessor.handlers[SignalTypeEmergencyStop] = &EmergencyStopHandler{coordinator: csc}
	csc.signalProcessor.handlers[SignalTypeHealthCheck] = &HealthCheckHandler{coordinator: csc}
	csc.signalProcessor.handlers[SignalTypeRecovery] = &RecoveryHandler{coordinator: csc}
}

// LoadSheddingHandler handles load shedding signals
type LoadSheddingHandler struct {
	coordinator *CrossServiceCoordinator
}

func (h *LoadSheddingHandler) HandleSignal(ctx context.Context, signal *BackpressureSignal) error {
	h.coordinator.logger.Info("Processing load shedding signal",
		zap.String("source", signal.SourceService),
		zap.Int("severity", signal.Severity))

	atomic.AddInt64(&h.coordinator.metrics.LoadSheddingEvents, 1)

	// Broadcast load shedding to sheddable services
	for _, serviceID := range h.coordinator.config.SheddableServices {
		loadSheddingSignal := &BackpressureSignal{
			SignalID:      fmt.Sprintf("load-shed-%d", time.Now().UnixNano()),
			SourceService: "coordinator",
			TargetService: serviceID,
			SignalType:    SignalTypeLoadShedding,
			Severity:      signal.Severity,
			Reason:        fmt.Sprintf("Load shedding requested by %s", signal.SourceService),
			ExpiresAt:     time.Now().Add(h.coordinator.config.SignalTimeout),
		}

		h.coordinator.SendBackpressureSignal(loadSheddingSignal)
	}

	return nil
}

// SlowDownHandler handles slow down signals
type SlowDownHandler struct {
	coordinator *CrossServiceCoordinator
}

func (h *SlowDownHandler) HandleSignal(ctx context.Context, signal *BackpressureSignal) error {
	h.coordinator.logger.Info("Processing slow down signal",
		zap.String("source", signal.SourceService),
		zap.Int("severity", signal.Severity))

	atomic.AddInt64(&h.coordinator.metrics.ThrottlingEvents, 1)

	// Update global backpressure level
	currentLevel := atomic.LoadInt64(&h.coordinator.globalBackpressure)
	newLevel := int64(signal.Severity)
	if newLevel > currentLevel {
		atomic.StoreInt64(&h.coordinator.globalBackpressure, newLevel)
	}

	return nil
}

// ThrottlingHandler handles throttling signals
type ThrottlingHandler struct {
	coordinator *CrossServiceCoordinator
}

func (h *ThrottlingHandler) HandleSignal(ctx context.Context, signal *BackpressureSignal) error {
	h.coordinator.logger.Info("Processing throttling signal",
		zap.String("source", signal.SourceService),
		zap.Int("severity", signal.Severity))

	atomic.AddInt64(&h.coordinator.metrics.ThrottlingEvents, 1)
	return nil
}

// EmergencyStopHandler handles emergency stop signals
type EmergencyStopHandler struct {
	coordinator *CrossServiceCoordinator
}

func (h *EmergencyStopHandler) HandleSignal(ctx context.Context, signal *BackpressureSignal) error {
	h.coordinator.logger.Error("Processing emergency stop signal",
		zap.String("source", signal.SourceService),
		zap.String("reason", signal.Reason))

	atomic.AddInt64(&h.coordinator.metrics.EmergencyStops, 1)
	atomic.StoreInt64(&h.coordinator.emergencyMode, 1)

	// Broadcast emergency stop to all non-critical services
	h.coordinator.servicesMu.RLock()
	for serviceID, service := range h.coordinator.services {
		if !service.CriticalService {
			emergencySignal := &BackpressureSignal{
				SignalID:      fmt.Sprintf("emergency-%d", time.Now().UnixNano()),
				SourceService: "coordinator",
				TargetService: serviceID,
				SignalType:    SignalTypeEmergencyStop,
				Severity:      5,
				Reason:        "System emergency mode activated",
				ExpiresAt:     time.Now().Add(time.Hour), // Long expiry for emergency
			}

			h.coordinator.SendBackpressureSignal(emergencySignal)
		}
	}
	h.coordinator.servicesMu.RUnlock()

	return nil
}

// HealthCheckHandler handles health check signals
type HealthCheckHandler struct {
	coordinator *CrossServiceCoordinator
}

func (h *HealthCheckHandler) HandleSignal(ctx context.Context, signal *BackpressureSignal) error {
	// Health check signals are used for service discovery and monitoring
	return nil
}

// RecoveryHandler handles recovery signals
type RecoveryHandler struct {
	coordinator *CrossServiceCoordinator
}

func (h *RecoveryHandler) HandleSignal(ctx context.Context, signal *BackpressureSignal) error {
	h.coordinator.logger.Info("Processing recovery signal",
		zap.String("source", signal.SourceService))

	atomic.AddInt64(&h.coordinator.metrics.RecoveryEvents, 1)

	// Check if we can exit emergency mode
	if atomic.LoadInt64(&h.coordinator.emergencyMode) == 1 {
		// Evaluate if all critical services are healthy
		allCriticalHealthy := true
		h.coordinator.servicesMu.RLock()
		for _, service := range h.coordinator.services {
			if service.CriticalService && !service.Healthy {
				allCriticalHealthy = false
				break
			}
		}
		h.coordinator.servicesMu.RUnlock()

		if allCriticalHealthy {
			atomic.StoreInt64(&h.coordinator.emergencyMode, 0)
			h.coordinator.logger.Info("Exiting emergency mode - all critical services healthy")
		}
	}

	return nil
}

// healthMonitor monitors service health and triggers backpressure when needed
func (csc *CrossServiceCoordinator) healthMonitor(ctx context.Context) {
	defer csc.workers.Done()

	ticker := time.NewTicker(csc.config.HealthCheckInterval)
	defer ticker.Stop()

	csc.logger.Info("Health monitor started")

	for {
		select {
		case <-ticker.C:
			csc.checkServiceHealth()

		case <-csc.shutdown:
			csc.logger.Info("Health monitor shutting down")
			return

		case <-ctx.Done():
			return
		}
	}
}

// backpressureEngine manages global backpressure decisions
func (csc *CrossServiceCoordinator) backpressureEngine(ctx context.Context) {
	defer csc.workers.Done()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	csc.logger.Info("Backpressure engine started")

	for {
		select {
		case <-ticker.C:
			csc.evaluateGlobalBackpressure()

		case <-csc.shutdown:
			csc.logger.Info("Backpressure engine shutting down")
			return

		case <-ctx.Done():
			return
		}
	}
}

// checkServiceHealth checks health of all registered services
func (csc *CrossServiceCoordinator) checkServiceHealth() {
	now := time.Now()
	var healthy, unhealthy, critical int64

	csc.servicesMu.RLock()
	for serviceID, service := range csc.services {
		service.mu.RLock()
		timeSinceLastSeen := now.Sub(service.LastSeen)
		isHealthy := service.Healthy && timeSinceLastSeen < csc.config.ServiceTimeout
		isCritical := service.CriticalService
		service.mu.RUnlock()

		if isHealthy {
			healthy++
		} else {
			unhealthy++
			csc.logger.Warn("Service health check failed",
				zap.String("service_id", serviceID),
				zap.Duration("time_since_last_seen", timeSinceLastSeen))
		}

		if isCritical {
			critical++
		}
	}
	csc.servicesMu.RUnlock()

	atomic.StoreInt64(&csc.metrics.HealthyServices, healthy)
	atomic.StoreInt64(&csc.metrics.UnhealthyServices, unhealthy)
	atomic.StoreInt64(&csc.metrics.CriticalServices, critical)
}

// evaluateServiceBackpressure evaluates if a service needs backpressure
func (csc *CrossServiceCoordinator) evaluateServiceBackpressure(service *ServiceState) {
	service.mu.RLock()
	cpuUsage := service.CPUUsage
	memoryUsage := service.MemoryUsage
	errorRate := service.ErrorRate
	loadThreshold := service.LoadThreshold
	errorThreshold := service.ErrorThreshold
	serviceID := service.ServiceID
	service.mu.RUnlock()

	// Check if service is under stress
	highLoad := cpuUsage > loadThreshold || memoryUsage > loadThreshold
	highErrors := errorRate > errorThreshold

	if highLoad || highErrors {
		severity := 1
		if cpuUsage > 90 || memoryUsage > 90 || errorRate > 10 {
			severity = 3
		} else if cpuUsage > 80 || memoryUsage > 80 || errorRate > 5 {
			severity = 2
		}

		signal := &BackpressureSignal{
			SignalID:      fmt.Sprintf("service-stress-%s-%d", serviceID, time.Now().UnixNano()),
			SourceService: serviceID,
			SignalType:    SignalTypeSlowDown,
			Severity:      severity,
			Reason:        fmt.Sprintf("Service stress: CPU %.1f%%, Memory %.1f%%, Errors %.1f%%", cpuUsage, memoryUsage, errorRate),
		}

		csc.SendBackpressureSignal(signal)
	}
}

// evaluateGlobalBackpressure evaluates global system backpressure
func (csc *CrossServiceCoordinator) evaluateGlobalBackpressure() {
	// Calculate global system load
	var totalLoad float64
	var serviceCount int

	csc.servicesMu.RLock()
	for _, service := range csc.services {
		service.mu.RLock()
		totalLoad += (service.CPUUsage + service.MemoryUsage) / 2
		serviceCount++
		service.mu.RUnlock()
	}
	csc.servicesMu.RUnlock()

	if serviceCount == 0 {
		return
	}

	avgLoad := totalLoad / float64(serviceCount)

	// Update global backpressure level
	var newLevel int64
	if avgLoad >= csc.config.EmergencyThreshold {
		newLevel = 5 // Emergency
	} else if avgLoad >= csc.config.GlobalLoadThreshold {
		newLevel = int64(((avgLoad-csc.config.GlobalLoadThreshold)/(100-csc.config.GlobalLoadThreshold))*4) + 1
	} else {
		newLevel = 0
	}

	oldLevel := atomic.SwapInt64(&csc.globalBackpressure, newLevel)
	atomic.StoreInt64(&csc.metrics.GlobalBackpressureLevel, newLevel)

	if newLevel > oldLevel && newLevel >= 3 {
		// Send global backpressure signal
		signal := &BackpressureSignal{
			SignalID:      fmt.Sprintf("global-backpressure-%d", time.Now().UnixNano()),
			SourceService: "coordinator",
			SignalType:    SignalTypeLoadShedding,
			Severity:      int(newLevel),
			Reason:        fmt.Sprintf("Global system load: %.1f%%", avgLoad),
		}

		csc.SendBackpressureSignal(signal)
	}
}

// GetGlobalBackpressureLevel returns current global backpressure level
func (csc *CrossServiceCoordinator) GetGlobalBackpressureLevel() int64 {
	return atomic.LoadInt64(&csc.globalBackpressure)
}

// IsEmergencyMode returns true if system is in emergency mode
func (csc *CrossServiceCoordinator) IsEmergencyMode() bool {
	return atomic.LoadInt64(&csc.emergencyMode) == 1
}

// GetMetrics returns coordinator metrics (nil-safe for tests)
func (c *CrossServiceCoordinator) GetMetrics() map[string]interface{} {
	if c == nil {
		return map[string]interface{}{}
	}
	return map[string]interface{}{
		"signals_sent":      atomic.LoadInt64(&c.metrics.SignalsSent),
		"signals_received":  atomic.LoadInt64(&c.metrics.SignalsReceived),
		"signals_processed": atomic.LoadInt64(&c.metrics.SignalsProcessed),
		"signal_errors":     atomic.LoadInt64(&c.metrics.SignalErrors),

		"healthy_services":   atomic.LoadInt64(&c.metrics.HealthyServices),
		"unhealthy_services": atomic.LoadInt64(&c.metrics.UnhealthyServices),
		"critical_services":  atomic.LoadInt64(&c.metrics.CriticalServices),

		"load_shedding_events": atomic.LoadInt64(&c.metrics.LoadSheddingEvents),
		"throttling_events":    atomic.LoadInt64(&c.metrics.ThrottlingEvents),
		"emergency_stops":      atomic.LoadInt64(&c.metrics.EmergencyStops),
		"recovery_events":      atomic.LoadInt64(&c.metrics.RecoveryEvents),

		"global_backpressure_level": atomic.LoadInt64(&c.metrics.GlobalBackpressureLevel),
		"services_in_backpressure":  atomic.LoadInt64(&c.metrics.ServicesInBackpressure),
	}
}

// Stop gracefully shuts down the cross-service coordinator
func (csc *CrossServiceCoordinator) Stop(ctx context.Context) error {
	csc.shutdownOnce.Do(func() {
		csc.logger.Info("Stopping cross-service backpressure coordinator")

		// Stop signal processor
		atomic.StoreInt64(&csc.signalProcessor.processing, 0)

		close(csc.shutdown)

		// Wait for workers to complete
		done := make(chan struct{})
		go func() {
			csc.workers.Wait()
			close(done)
		}()

		select {
		case <-done:
			csc.logger.Info("All workers stopped")
		case <-ctx.Done():
			csc.logger.Warn("Coordinator shutdown timed out")
		}

		// Close Kafka connections
		if err := csc.consumer.Close(); err != nil {
			csc.logger.Error("Failed to close Kafka consumer", zap.Error(err))
		}
		if err := csc.producer.Close(); err != nil {
			csc.logger.Error("Failed to close Kafka producer", zap.Error(err))
		}

		csc.logger.Info("Cross-service backpressure coordinator stopped")
	})

	return nil
}
