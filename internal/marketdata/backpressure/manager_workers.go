// Worker functions and monitoring components for backpressure manager
package backpressure

import (
	"fmt"
	"runtime"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// messageProcessingWorker processes incoming messages and prepares them for delivery
func (m *BackpressureManager) messageProcessingWorker(workerID int) {
	defer m.workers.Done()

	m.logger.Debug("Starting message processing worker", zap.Int("worker_id", workerID))

	for {
		select {
		case <-m.shutdown:
			m.logger.Debug("Message processing worker shutting down", zap.Int("worker_id", workerID))
			return

		case msg := <-m.incomingMessages:
			start := time.Now()

			if err := m.processMessage(msg); err != nil {
				m.logger.Error("Failed to process message",
					zap.Int("worker_id", workerID),
					zap.Error(err),
					zap.String("message_type", string(msg.Type)))
				m.metrics.MessagesDropped.Inc()
			} else {
				m.metrics.MessagesProcessed.Inc()
				m.metrics.MessageLatency.Observe(time.Since(start).Seconds())
			}
		}
	}
}

// processMessage handles the processing of a single incoming message
func (m *BackpressureManager) processMessage(msg *IncomingMessage) error {
	// Check global emergency mode
	if atomic.LoadInt32(&m.emergencyMode) == 1 && msg.Type != PriorityCritical {
		return ErrEmergencyMode
	}

	// Create priority message
	priorityMsg := &PriorityMessage{
		Priority:  msg.Type,
		Data:      msg.Data,
		Timestamp: msg.Timestamp,
		Deadline:  msg.Timestamp.Add(m.config.ProcessingTimeout),
		Metadata:  msg.Metadata,
	}

	// Handle targeted vs broadcast distribution
	if msg.TargetID != "" {
		// Targeted message
		return m.processTargetedMessage(msg.TargetID, priorityMsg)
	} else {
		// Broadcast message
		return m.processBroadcastMessage(priorityMsg)
	}
}

// processTargetedMessage processes a message for a specific client
func (m *BackpressureManager) processTargetedMessage(clientID string, msg *PriorityMessage) error {
	clientInterface, exists := m.clients.Load(clientID)
	if !exists {
		return ErrClientNotFound
	}

	client := clientInterface.(*ManagedClient)

	// Check if client is active
	if atomic.LoadInt32(&client.IsActive) == 0 {
		return ErrClientNotFound
	}

	// Check client-specific emergency mode
	if atomic.LoadInt32(&client.InEmergency) == 1 && msg.Priority != PriorityCritical {
		return ErrEmergencyMode
	}

	// Check rate limiting
	if !m.rateLimiter.Allow(clientID, msg.Priority) {
		return ErrRateLimited
	}

	// Create processed job
	job := &ProcessedJob{
		ClientID:    clientID,
		Message:     msg,
		Priority:    msg.Priority,
		Deadline:    msg.Deadline,
		RetryCount:  0,
		EmergencyOK: msg.Priority == PriorityCritical,
	}

	// Queue for delivery
	select {
	case m.processedJobs <- job:
		return nil
	default:
		return ErrQueueFull
	}
}

// processBroadcastMessage processes a message for all active clients
func (m *BackpressureManager) processBroadcastMessage(msg *PriorityMessage) error {
	var errors []error

	// Iterate through all clients
	m.clients.Range(func(key, value interface{}) bool {
		clientID := key.(string)
		client := value.(*ManagedClient)

		// Skip inactive clients
		if atomic.LoadInt32(&client.IsActive) == 0 {
			return true
		}

		// Check client-specific emergency mode
		if atomic.LoadInt32(&client.InEmergency) == 1 && msg.Priority != PriorityCritical {
			return true
		}

		// Check rate limiting
		if !m.rateLimiter.Allow(clientID, msg.Priority) {
			return true
		}

		// Create processed job for this client
		job := &ProcessedJob{
			ClientID:    clientID,
			Message:     msg,
			Priority:    msg.Priority,
			Deadline:    msg.Deadline,
			RetryCount:  0,
			EmergencyOK: msg.Priority == PriorityCritical,
		}

		// Try to queue for delivery
		select {
		case m.processedJobs <- job:
			// Successfully queued
		default:
			// Queue full, continue to next client
			errors = append(errors, ErrQueueFull)
		}

		return true
	})

	// Return error if all clients failed
	if len(errors) > 0 {
		return errors[0] // Return first error for simplicity
	}

	return nil
}

// messageDeliveryWorker delivers processed messages to clients
func (m *BackpressureManager) messageDeliveryWorker(workerID int) {
	defer m.workers.Done()

	m.logger.Debug("Starting message delivery worker", zap.Int("worker_id", workerID))

	for {
		select {
		case <-m.shutdown:
			m.logger.Debug("Message delivery worker shutting down", zap.Int("worker_id", workerID))
			return

		case job := <-m.processedJobs:
			if err := m.deliverMessage(job); err != nil {
				m.logger.Error("Failed to deliver message",
					zap.Int("worker_id", workerID),
					zap.String("client_id", job.ClientID),
					zap.Error(err),
					zap.Int("retry_count", job.RetryCount))

				// Handle retry logic
				if job.RetryCount < m.config.MaxRetries && time.Now().Before(job.Deadline) {
					job.RetryCount++
					// Re-queue with delay
					go func() {
						time.Sleep(time.Millisecond * 100 * time.Duration(job.RetryCount))
						select {
						case m.processedJobs <- job:
						case <-m.shutdown:
						}
					}()
				} else {
					m.metrics.MessagesDropped.Inc()
				}
			}
		}
	}
}

// deliverMessage delivers a message to a specific client
func (m *BackpressureManager) deliverMessage(job *ProcessedJob) error {
	clientInterface, exists := m.clients.Load(job.ClientID)
	if !exists {
		return ErrClientNotFound
	}

	client := clientInterface.(*ManagedClient)

	// Check if client is still active
	if atomic.LoadInt32(&client.IsActive) == 0 {
		return ErrClientNotFound
	}

	// Check deadline
	if time.Now().After(job.Deadline) {
		return ErrDeadlineExceeded
	}

	// Try to send to client's channel
	select {
	case client.SendChannel <- job.Message:
		// Update client metrics
		atomic.AddInt64(&client.MessagesSent, 1)
		atomic.AddInt64(&client.BytesSent, int64(len(job.Message.Data)))
		client.LastActivity = time.Now()

		// Measure delivery latency
		latency := time.Since(job.Message.Timestamp)
		client.LastLatency = latency

		// Update capability detector with successful delivery
		m.detector.RecordMessageDelivery(job.ClientID, len(job.Message.Data), latency)

		return nil

	default:
		// Client send channel is full
		atomic.AddInt64(&client.MessagesDropped, 1)
		return ErrClientChannelFull
	}
}

// clientMonitor monitors client health and handles cleanup
func (m *BackpressureManager) clientMonitor() {
	defer m.shutdownWG.Done()

	ticker := time.NewTicker(time.Second * 30)
	defer ticker.Stop()

	m.logger.Debug("Starting client monitor")

	for {
		select {
		case <-m.shutdown:
			m.logger.Debug("Client monitor shutting down")
			return

		case <-ticker.C:
			m.monitorClients()
		}
	}
}

// monitorClients checks client health and performs cleanup
func (m *BackpressureManager) monitorClients() {
	now := time.Now()
	var clientsToRemove []string
	var totalLatency time.Duration
	var latencyCount int
	var emergencyClients int

	m.clients.Range(func(key, value interface{}) bool {
		clientID := key.(string)
		client := value.(*ManagedClient)

		// Check for client timeout
		if now.Sub(client.LastActivity) > m.config.ClientTimeout {
			clientsToRemove = append(clientsToRemove, clientID)
			return true
		}

		// Update emergency status based on client latency
		if client.LastLatency > m.config.EmergencyLatencyThreshold {
			if atomic.CompareAndSwapInt32(&client.InEmergency, 0, 1) {
				client.EmergencyAt = now
				m.logger.Warn("Client entered emergency mode",
					zap.String("client_id", clientID),
					zap.Duration("latency", client.LastLatency))
			}
			emergencyClients++
		} else if atomic.LoadInt32(&client.InEmergency) == 1 {
			// Check for recovery
			if client.LastLatency < m.config.RecoveryLatencyTarget &&
				now.Sub(client.EmergencyAt) > m.config.RecoveryGracePeriod {

				if atomic.CompareAndSwapInt32(&client.InEmergency, 1, 0) {
					client.RecoveryAt = now
					recoveryTime := now.Sub(client.EmergencyAt)
					m.metrics.RecoveryTime.Observe(recoveryTime.Seconds())

					m.logger.Info("Client recovered from emergency mode",
						zap.String("client_id", clientID),
						zap.Duration("recovery_time", recoveryTime))
				}
			} else {
				emergencyClients++
			}
		}

		// Collect latency statistics
		if client.LastLatency > 0 {
			totalLatency += client.LastLatency
			latencyCount++
		}

		return true
	})

	// Remove timed-out clients
	for _, clientID := range clientsToRemove {
		m.logger.Info("Removing timed-out client", zap.String("client_id", clientID))
		m.UnregisterClient(clientID)
	}

	// Update metrics
	m.metrics.ClientsInEmergency.Set(float64(emergencyClients))
	if latencyCount > 0 {
		avgLatency := totalLatency / time.Duration(latencyCount)
		m.metrics.AverageClientLatency.Set(avgLatency.Seconds())
	}
}

// emergencyMonitor monitors system health and triggers emergency mode
func (m *BackpressureManager) emergencyMonitor() {
	defer m.shutdownWG.Done()

	ticker := time.NewTicker(time.Second * 5)
	defer ticker.Stop()

	m.logger.Debug("Starting emergency monitor")

	for {
		select {
		case <-m.shutdown:
			m.logger.Debug("Emergency monitor shutting down")
			return

		case <-ticker.C:
			m.checkEmergencyConditions()
		}
	}
}

// checkEmergencyConditions evaluates whether emergency mode should be activated
func (m *BackpressureManager) checkEmergencyConditions() {
	incomingQueueLen := len(m.incomingMessages)
	processedQueueLen := len(m.processedJobs)

	// Update queue metrics
	m.metrics.ProcessingQueueLength.Set(float64(incomingQueueLen + processedQueueLen))

	// Check queue length threshold
	if incomingQueueLen > m.config.EmergencyQueueLengthThreshold ||
		processedQueueLen > m.config.EmergencyQueueLengthThreshold {

		if atomic.CompareAndSwapInt32(&m.emergencyMode, 0, 1) {
			m.emergencyMetrics.ActivationCount++
			m.emergencyMetrics.LastActivation = time.Now()
			m.metrics.EmergencyActivations.Inc()

			m.logger.Warn("Emergency backpressure mode activated due to queue length",
				zap.Int("incoming_queue", incomingQueueLen),
				zap.Int("processed_queue", processedQueueLen),
				zap.Int("threshold", m.config.EmergencyQueueLengthThreshold))
		}
		return
	}

	// Check recovery conditions if in emergency mode
	if atomic.LoadInt32(&m.emergencyMode) == 1 {
		if incomingQueueLen < m.config.EmergencyQueueLengthThreshold/2 &&
			processedQueueLen < m.config.EmergencyQueueLengthThreshold/2 {

			if atomic.CompareAndSwapInt32(&m.emergencyMode, 1, 0) {
				now := time.Now()
				duration := now.Sub(m.emergencyMetrics.LastActivation)
				m.emergencyMetrics.TotalDuration += duration
				m.emergencyMetrics.LastRecovery = now
				m.metrics.EmergencyDuration.Observe(duration.Seconds())

				m.logger.Info("Emergency backpressure mode deactivated",
					zap.Duration("duration", duration),
					zap.Int("incoming_queue", incomingQueueLen),
					zap.Int("processed_queue", processedQueueLen))
			}
		}
	}
}

// metricsCollector collects and updates system metrics
func (m *BackpressureManager) metricsCollector() {
	defer m.shutdownWG.Done()

	ticker := time.NewTicker(time.Second * 10)
	defer ticker.Stop()

	m.logger.Debug("Starting metrics collector")

	for {
		select {
		case <-m.shutdown:
			m.logger.Debug("Metrics collector shutting down")
			return

		case <-ticker.C:
			m.collectMetrics()
		}
	}
}

// collectMetrics gathers and updates system metrics
func (m *BackpressureManager) collectMetrics() {
	// Memory usage
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	m.metrics.MemoryUsage.Set(float64(memStats.Alloc))

	// Worker utilization
	incomingLen := len(m.incomingMessages)
	processedLen := len(m.processedJobs)
	totalCapacity := cap(m.incomingMessages) + cap(m.processedJobs)
	utilization := float64(incomingLen+processedLen) / float64(totalCapacity)
	m.metrics.WorkerUtilization.Set(utilization)

	// Cross-service coordination updates
	status := m.coordinator.GetStatus()
	if status.IsEmergency && atomic.LoadInt32(&m.emergencyMode) == 0 {
		m.logger.Warn("Activating emergency mode due to cross-service coordination")
		m.SetEmergencyMode(true)
	}
}

// Additional error definitions
var (
	ErrRateLimited       = fmt.Errorf("message rate limited")
	ErrDeadlineExceeded  = fmt.Errorf("message deadline exceeded")
	ErrClientChannelFull = fmt.Errorf("client send channel is full")
)
