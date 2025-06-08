package messaging

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// CircuitBreakerState represents the state of a circuit breaker
type CircuitBreakerState int32

const (
	CircuitBreakerClosed CircuitBreakerState = iota
	CircuitBreakerOpen
	CircuitBreakerHalfOpen
)

func (s CircuitBreakerState) String() string {
	switch s {
	case CircuitBreakerClosed:
		return "closed"
	case CircuitBreakerOpen:
		return "open"
	case CircuitBreakerHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// MessagingCircuitBreaker provides circuit breaker functionality for messaging operations
type MessagingCircuitBreaker struct {
	state            CircuitBreakerState
	failureCount     int64
	successCount     int64
	lastFailureTime  time.Time
	lastSuccessTime  time.Time
	halfOpenRequests int64
	config           EnhancedCircuitBreakerConfig
	metrics          *CircuitBreakerMetrics
	mu               sync.RWMutex
	logger           *zap.Logger
}

// EnhancedCircuitBreakerConfig contains enhanced circuit breaker configuration
type EnhancedCircuitBreakerConfig struct {
	Enabled             bool          `json:"enabled"`
	FailureThreshold    int64         `json:"failure_threshold"`
	SuccessThreshold    int64         `json:"success_threshold"`
	Timeout             time.Duration `json:"timeout"`
	MaxHalfOpenRequests int64         `json:"max_half_open_requests"`
	FailureWindow       time.Duration `json:"failure_window"`
	MonitoringInterval  time.Duration `json:"monitoring_interval"`
}

// CircuitBreakerMetrics tracks circuit breaker performance metrics
type CircuitBreakerMetrics struct {
	StateTransitions    int64         `json:"state_transitions"`
	TotalFailures       int64         `json:"total_failures"`
	TotalSuccesses      int64         `json:"total_successes"`
	OpenDuration        time.Duration `json:"open_duration"`
	LastStateChange     time.Time     `json:"last_state_change"`
	FailureRate         float64       `json:"failure_rate"`
	RequestsBlocked     int64         `json:"requests_blocked"`
	RequestsAllowed     int64         `json:"requests_allowed"`
	AverageResponseTime time.Duration `json:"average_response_time"`
}

// MessagingMetrics provides comprehensive metrics for the messaging system
type MessagingMetrics struct {
	ProducerMetrics       *ProducerMetrics       `json:"producer_metrics"`
	ConsumerMetrics       *ConsumerMetrics       `json:"consumer_metrics"`
	CircuitBreakerMetrics *CircuitBreakerMetrics `json:"circuit_breaker_metrics"`
	SystemMetrics         *SystemMetrics         `json:"system_metrics"`
	lastUpdated           time.Time
	mu                    sync.RWMutex
}

// ProducerMetrics tracks producer performance
type ProducerMetrics struct {
	MessagesPublished     int64         `json:"messages_published"`
	MessagesSent          int64         `json:"messages_sent"`
	MessagesFailedToSend  int64         `json:"messages_failed_to_send"`
	BatchesSent           int64         `json:"batches_sent"`
	AverageLatency        time.Duration `json:"average_latency"`
	ThroughputMsgPerSec   float64       `json:"throughput_msg_per_sec"`
	ThroughputBytesPerSec float64       `json:"throughput_bytes_per_sec"`
	ErrorRate             float64       `json:"error_rate"`
	BufferUtilization     float64       `json:"buffer_utilization"`
	ConnectionsActive     int32         `json:"connections_active"`
}

// ConsumerMetrics tracks consumer performance
type ConsumerMetrics struct {
	MessagesConsumed        int64         `json:"messages_consumed"`
	MessagesProcessed       int64         `json:"messages_processed"`
	MessagesFailedToProcess int64         `json:"messages_failed_to_process"`
	AverageProcessingTime   time.Duration `json:"average_processing_time"`
	ConsumerLag             int64         `json:"consumer_lag"`
	ThroughputMsgPerSec     float64       `json:"throughput_msg_per_sec"`
	ErrorRate               float64       `json:"error_rate"`
	RebalanceCount          int64         `json:"rebalance_count"`
	ActiveConsumers         int32         `json:"active_consumers"`
}

// SystemMetrics tracks system-level metrics
type SystemMetrics struct {
	CPUUsage           float64       `json:"cpu_usage"`
	MemoryUsage        int64         `json:"memory_usage"`
	DiskUsage          int64         `json:"disk_usage"`
	NetworkIOBytes     int64         `json:"network_io_bytes"`
	GoroutineCount     int32         `json:"goroutine_count"`
	ConnectionPoolSize int32         `json:"connection_pool_size"`
	ActiveConnections  int32         `json:"active_connections"`
	UpTime             time.Duration `json:"uptime"`
}

// DefaultEnhancedCircuitBreakerConfig returns default enhanced circuit breaker configuration
func DefaultEnhancedCircuitBreakerConfig() EnhancedCircuitBreakerConfig {
	return EnhancedCircuitBreakerConfig{
		Enabled:             true,
		FailureThreshold:    5,
		SuccessThreshold:    3,
		Timeout:             30 * time.Second,
		MaxHalfOpenRequests: 2,
		FailureWindow:       1 * time.Minute,
		MonitoringInterval:  5 * time.Second,
	}
}

// NewMessagingCircuitBreaker creates a new circuit breaker for messaging operations
func NewMessagingCircuitBreaker(config EnhancedCircuitBreakerConfig, logger *zap.Logger) *MessagingCircuitBreaker {
	cb := &MessagingCircuitBreaker{
		state:  CircuitBreakerClosed,
		config: config,
		logger: logger,
		metrics: &CircuitBreakerMetrics{
			LastStateChange: time.Now(),
		},
	}

	// Start monitoring goroutine
	if config.Enabled && config.MonitoringInterval > 0 {
		go cb.monitor()
	}

	return cb
}

// AllowRequest checks if a request can proceed through the circuit breaker
func (cb *MessagingCircuitBreaker) AllowRequest() bool {
	if !cb.config.Enabled {
		return true
	}

	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()

	switch cb.state {
	case CircuitBreakerClosed:
		atomic.AddInt64(&cb.metrics.RequestsAllowed, 1)
		return true

	case CircuitBreakerOpen:
		// Check if timeout has passed to move to half-open
		if now.Sub(cb.lastFailureTime) >= cb.config.Timeout {
			cb.setState(CircuitBreakerHalfOpen)
			atomic.AddInt64(&cb.metrics.RequestsAllowed, 1)
			return true
		}
		atomic.AddInt64(&cb.metrics.RequestsBlocked, 1)
		return false

	case CircuitBreakerHalfOpen:
		if atomic.LoadInt64(&cb.halfOpenRequests) < cb.config.MaxHalfOpenRequests {
			atomic.AddInt64(&cb.halfOpenRequests, 1)
			atomic.AddInt64(&cb.metrics.RequestsAllowed, 1)
			return true
		}
		atomic.AddInt64(&cb.metrics.RequestsBlocked, 1)
		return false

	default:
		atomic.AddInt64(&cb.metrics.RequestsBlocked, 1)
		return false
	}
}

// RecordSuccess records a successful operation
func (cb *MessagingCircuitBreaker) RecordSuccess(responseTime time.Duration) {
	if !cb.config.Enabled {
		return
	}

	cb.mu.Lock()
	defer cb.mu.Unlock()

	atomic.AddInt64(&cb.successCount, 1)
	atomic.AddInt64(&cb.metrics.TotalSuccesses, 1)
	cb.lastSuccessTime = time.Now()

	// Update average response time
	cb.updateAverageResponseTime(responseTime)

	switch cb.state {
	case CircuitBreakerHalfOpen:
		if atomic.LoadInt64(&cb.successCount) >= cb.config.SuccessThreshold {
			cb.setState(CircuitBreakerClosed)
			atomic.StoreInt64(&cb.halfOpenRequests, 0)
		}
	case CircuitBreakerClosed:
		// Reset failure count on success
		atomic.StoreInt64(&cb.failureCount, 0)
	}

	cb.updateMetrics()
}

// RecordFailure records a failed operation
func (cb *MessagingCircuitBreaker) RecordFailure() {
	if !cb.config.Enabled {
		return
	}

	cb.mu.Lock()
	defer cb.mu.Unlock()

	atomic.AddInt64(&cb.failureCount, 1)
	atomic.AddInt64(&cb.metrics.TotalFailures, 1)
	cb.lastFailureTime = time.Now()

	switch cb.state {
	case CircuitBreakerClosed:
		if atomic.LoadInt64(&cb.failureCount) >= cb.config.FailureThreshold {
			cb.setState(CircuitBreakerOpen)
		}
	case CircuitBreakerHalfOpen:
		cb.setState(CircuitBreakerOpen)
		atomic.StoreInt64(&cb.halfOpenRequests, 0)
	}

	cb.updateMetrics()
}

// setState changes the circuit breaker state and records metrics
func (cb *MessagingCircuitBreaker) setState(newState CircuitBreakerState) {
	if cb.state != newState {
		oldState := cb.state
		cb.state = newState
		now := time.Now()

		// Track open duration
		if oldState == CircuitBreakerOpen {
			cb.metrics.OpenDuration += now.Sub(cb.metrics.LastStateChange)
		}

		cb.metrics.LastStateChange = now
		atomic.AddInt64(&cb.metrics.StateTransitions, 1)

		cb.logger.Info("Circuit breaker state changed",
			zap.String("old_state", oldState.String()),
			zap.String("new_state", newState.String()),
			zap.Int64("failure_count", atomic.LoadInt64(&cb.failureCount)),
			zap.Int64("success_count", atomic.LoadInt64(&cb.successCount)))
	}
}

// updateMetrics updates the circuit breaker metrics
func (cb *MessagingCircuitBreaker) updateMetrics() {
	totalRequests := cb.metrics.TotalFailures + cb.metrics.TotalSuccesses
	if totalRequests > 0 {
		cb.metrics.FailureRate = float64(cb.metrics.TotalFailures) / float64(totalRequests) * 100
	}
}

// updateAverageResponseTime updates the average response time metric
func (cb *MessagingCircuitBreaker) updateAverageResponseTime(responseTime time.Duration) {
	// Simple moving average calculation
	const alpha = 0.1 // Smoothing factor
	if cb.metrics.AverageResponseTime == 0 {
		cb.metrics.AverageResponseTime = responseTime
	} else {
		cb.metrics.AverageResponseTime = time.Duration(
			float64(cb.metrics.AverageResponseTime)*(1-alpha) + float64(responseTime)*alpha)
	}
}

// GetState returns the current state of the circuit breaker
func (cb *MessagingCircuitBreaker) GetState() CircuitBreakerState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// GetMetrics returns current circuit breaker metrics
func (cb *MessagingCircuitBreaker) GetMetrics() *CircuitBreakerMetrics {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	// Create a copy to avoid race conditions
	metricsCopy := *cb.metrics
	return &metricsCopy
}

// Reset manually resets the circuit breaker to closed state
func (cb *MessagingCircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.setState(CircuitBreakerClosed)
	atomic.StoreInt64(&cb.failureCount, 0)
	atomic.StoreInt64(&cb.successCount, 0)
	atomic.StoreInt64(&cb.halfOpenRequests, 0)

	cb.logger.Info("Circuit breaker manually reset")
}

// monitor runs periodic monitoring and maintenance
func (cb *MessagingCircuitBreaker) monitor() {
	ticker := time.NewTicker(cb.config.MonitoringInterval)
	defer ticker.Stop()

	for range ticker.C {
		cb.performMaintenance()
	}
}

// performMaintenance performs periodic maintenance tasks
func (cb *MessagingCircuitBreaker) performMaintenance() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()

	// Auto-recovery logic for long-open circuits
	if cb.state == CircuitBreakerOpen {
		if now.Sub(cb.lastFailureTime) > cb.config.Timeout*2 {
			cb.logger.Info("Auto-recovery: transitioning to half-open after extended timeout")
			cb.setState(CircuitBreakerHalfOpen)
		}
	}

	// Clean old failure data outside the failure window
	if now.Sub(cb.lastFailureTime) > cb.config.FailureWindow {
		atomic.StoreInt64(&cb.failureCount, 0)
	}

	// Log periodic status
	if cb.logger != nil {
		cb.logger.Debug("Circuit breaker status",
			zap.String("state", cb.state.String()),
			zap.Int64("failures", atomic.LoadInt64(&cb.failureCount)),
			zap.Int64("successes", atomic.LoadInt64(&cb.successCount)),
			zap.Float64("failure_rate", cb.metrics.FailureRate),
			zap.Duration("avg_response_time", cb.metrics.AverageResponseTime))
	}
}

// ExecuteWithCircuitBreaker executes an operation with circuit breaker protection
func (cb *MessagingCircuitBreaker) ExecuteWithCircuitBreaker(operation func() error) error {
	if !cb.AllowRequest() {
		return fmt.Errorf("circuit breaker is open, request blocked")
	}

	startTime := time.Now()
	err := operation()
	responseTime := time.Since(startTime)

	if err != nil {
		cb.RecordFailure()
		return err
	}

	cb.RecordSuccess(responseTime)
	return nil
}

// MetricsCollector collects and aggregates messaging metrics
type MetricsCollector struct {
	producerMetrics *ProducerMetrics
	consumerMetrics *ConsumerMetrics
	systemMetrics   *SystemMetrics
	circuitBreaker  *MessagingCircuitBreaker
	startTime       time.Time
	mu              sync.RWMutex
	logger          *zap.Logger
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector(logger *zap.Logger) *MetricsCollector {
	return &MetricsCollector{
		producerMetrics: &ProducerMetrics{},
		consumerMetrics: &ConsumerMetrics{},
		systemMetrics:   &SystemMetrics{},
		startTime:       time.Now(),
		logger:          logger,
	}
}

// RecordProducerMessage records a producer message metric
func (mc *MetricsCollector) RecordProducerMessage(success bool, latency time.Duration, size int64) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	atomic.AddInt64(&mc.producerMetrics.MessagesPublished, 1)

	if success {
		atomic.AddInt64(&mc.producerMetrics.MessagesSent, 1)
	} else {
		atomic.AddInt64(&mc.producerMetrics.MessagesFailedToSend, 1)
	}

	// Update average latency
	mc.updateProducerLatency(latency)

	// Update throughput calculations
	mc.updateProducerThroughput(size)
}

// RecordConsumerMessage records a consumer message metric
func (mc *MetricsCollector) RecordConsumerMessage(success bool, processingTime time.Duration) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	atomic.AddInt64(&mc.consumerMetrics.MessagesConsumed, 1)

	if success {
		atomic.AddInt64(&mc.consumerMetrics.MessagesProcessed, 1)
	} else {
		atomic.AddInt64(&mc.consumerMetrics.MessagesFailedToProcess, 1)
	}

	// Update average processing time
	mc.updateConsumerProcessingTime(processingTime)
}

// updateProducerLatency updates the producer average latency
func (mc *MetricsCollector) updateProducerLatency(latency time.Duration) {
	const alpha = 0.1 // Smoothing factor
	if mc.producerMetrics.AverageLatency == 0 {
		mc.producerMetrics.AverageLatency = latency
	} else {
		mc.producerMetrics.AverageLatency = time.Duration(
			float64(mc.producerMetrics.AverageLatency)*(1-alpha) + float64(latency)*alpha)
	}
}

// updateProducerThroughput updates producer throughput metrics
func (mc *MetricsCollector) updateProducerThroughput(messageSize int64) {
	duration := time.Since(mc.startTime)
	if duration.Seconds() > 0 {
		mc.producerMetrics.ThroughputMsgPerSec = float64(atomic.LoadInt64(&mc.producerMetrics.MessagesSent)) / duration.Seconds()
		mc.producerMetrics.ThroughputBytesPerSec = float64(messageSize) / duration.Seconds()
	}

	// Update error rate
	totalMessages := atomic.LoadInt64(&mc.producerMetrics.MessagesPublished)
	failedMessages := atomic.LoadInt64(&mc.producerMetrics.MessagesFailedToSend)
	if totalMessages > 0 {
		mc.producerMetrics.ErrorRate = float64(failedMessages) / float64(totalMessages) * 100
	}
}

// updateConsumerProcessingTime updates the consumer average processing time
func (mc *MetricsCollector) updateConsumerProcessingTime(processingTime time.Duration) {
	const alpha = 0.1 // Smoothing factor
	if mc.consumerMetrics.AverageProcessingTime == 0 {
		mc.consumerMetrics.AverageProcessingTime = processingTime
	} else {
		mc.consumerMetrics.AverageProcessingTime = time.Duration(
			float64(mc.consumerMetrics.AverageProcessingTime)*(1-alpha) + float64(processingTime)*alpha)
	}

	// Update consumer throughput and error rate
	duration := time.Since(mc.startTime)
	if duration.Seconds() > 0 {
		mc.consumerMetrics.ThroughputMsgPerSec = float64(atomic.LoadInt64(&mc.consumerMetrics.MessagesProcessed)) / duration.Seconds()
	}

	totalMessages := atomic.LoadInt64(&mc.consumerMetrics.MessagesConsumed)
	failedMessages := atomic.LoadInt64(&mc.consumerMetrics.MessagesFailedToProcess)
	if totalMessages > 0 {
		mc.consumerMetrics.ErrorRate = float64(failedMessages) / float64(totalMessages) * 100
	}
}

// GetMetrics returns current messaging metrics
func (mc *MetricsCollector) GetMetrics() *MessagingMetrics {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	metrics := &MessagingMetrics{
		ProducerMetrics: mc.producerMetrics,
		ConsumerMetrics: mc.consumerMetrics,
		SystemMetrics:   mc.systemMetrics,
		lastUpdated:     time.Now(),
	}

	if mc.circuitBreaker != nil {
		metrics.CircuitBreakerMetrics = mc.circuitBreaker.GetMetrics()
	}

	// Update system metrics
	mc.updateSystemMetrics()

	return metrics
}

// updateSystemMetrics updates system-level metrics
func (mc *MetricsCollector) updateSystemMetrics() {
	// Note: These would normally use system monitoring libraries
	// For now, we'll provide placeholder implementations

	mc.systemMetrics.UpTime = time.Since(mc.startTime)
	// Additional system metrics would be collected here using runtime package
	// or external monitoring libraries like gopsutil
}

// SetCircuitBreaker sets the circuit breaker for metrics collection
func (mc *MetricsCollector) SetCircuitBreaker(cb *MessagingCircuitBreaker) {
	mc.circuitBreaker = cb
}

// Reset resets all metrics
func (mc *MetricsCollector) Reset() {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	mc.producerMetrics = &ProducerMetrics{}
	mc.consumerMetrics = &ConsumerMetrics{}
	mc.systemMetrics = &SystemMetrics{}
	mc.startTime = time.Now()

	mc.logger.Info("Metrics collector reset")
}
