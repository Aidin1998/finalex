// Package backpressure provides client capability detection and adaptive rate limiting
// for robust market data distribution with zero trade loss
package backpressure

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"go.uber.org/zap"
)

// ClientCapabilityDetector measures and tracks client performance capabilities
// for adaptive rate limiting and prioritized message delivery
type ClientCapabilityDetector struct {
	logger *zap.Logger

	// Lock-free client capabilities registry
	capabilities unsafe.Pointer // *map[string]*ClientCapability
	capVersion   int64          // Version for lock-free updates

	// Performance measurement workers
	measurementWorkers sync.WaitGroup
	shutdown           chan struct{}
	shutdownOnce       sync.Once

	// Global metrics for system health
	metrics *CapabilityMetrics
}

// ClientCapability represents measured client performance characteristics
type ClientCapability struct {
	ClientID    string
	ConnectedAt time.Time

	// Bandwidth measurements (bytes/second)
	MaxBandwidth     int64 // Peak observed bandwidth
	AvgBandwidth     int64 // Moving average bandwidth
	CurrentBandwidth int64 // Real-time bandwidth estimate

	// Processing speed measurements (messages/second)
	MaxProcessingSpeed     int64 // Peak message processing rate
	AvgProcessingSpeed     int64 // Moving average processing rate
	CurrentProcessingSpeed int64 // Real-time processing estimate

	// Latency measurements (nanoseconds)
	MinLatency     int64 // Best observed latency
	AvgLatency     int64 // Moving average latency
	CurrentLatency int64 // Recent latency measurement

	// Quality metrics
	PacketLoss     float64 // Packet loss percentage (0.0-1.0)
	JitterStdDev   int64   // Latency jitter standard deviation
	ReconnectCount int64   // Number of reconnections
	ErrorRate      float64 // Error rate (0.0-1.0)

	// Adaptive rate limits (messages/second)
	SafeRate  int64 // Conservative rate for guaranteed delivery
	MaxRate   int64 // Maximum sustainable rate
	BurstRate int64 // Short-term burst capability
	// Client type classification
	ClientType     ClientType      // Detected client type
	TradingProfile Profile         // Trading behavior profile
	PriorityClass  MessagePriority // Assigned priority class

	// Real-time measurement state
	measurementWindow unsafe.Pointer // *MeasurementWindow
	lastMeasurement   int64          // Unix nanoseconds

	// Synchronization
	mu sync.RWMutex
	_  [7]int64 // Cache line padding
}

// ClientType represents different types of clients with varying needs
type ClientType int

const (
	ClientTypeUnknown       ClientType = iota
	ClientTypeRetail                   // Individual retail traders
	ClientTypeInstitutional            // Institutional clients
	ClientTypeMarketMaker              // Market making bots
	ClientTypeHFT                      // High-frequency trading
	ClientTypeAPI                      // API/algorithmic clients
)

// Profile represents trading behavior patterns
type Profile int

const (
	ProfileUnknown       Profile = iota
	ProfileLowFrequency          // Infrequent trading
	ProfileModerate              // Regular trading activity
	ProfileHighFrequency         // High-frequency trading
	ProfileMarketData            // Market data consumer only
)

// Priority represents client priority classification (now using MessagePriority)
// Removed separate Priority type to avoid conflicts - using MessagePriority throughout

// MeasurementWindow maintains sliding window of performance data
type MeasurementWindow struct {
	// Circular buffers for measurements
	bandwidthSamples  [100]int64 // 100 samples for bandwidth
	processingSamples [100]int64 // 100 samples for processing speed
	latencySamples    [100]int64 // 100 samples for latency

	// Circular buffer state
	head     int   // Current write position
	size     int   // Current number of samples
	lastTime int64 // Last measurement time

	// Real-time statistics
	bandwidthSum  int64
	processingSum int64
	latencySum    int64

	mu sync.RWMutex
}

// CapabilityMetrics tracks global detection metrics
type CapabilityMetrics struct {
	// Client counts by type
	RetailClients        int64
	InstitutionalClients int64
	MarketMakerClients   int64
	HFTClients           int64
	APIClients           int64

	// Performance distribution
	HighPerformanceClients   int64 // >1000 msg/sec
	MediumPerformanceClients int64 // 100-1000 msg/sec
	LowPerformanceClients    int64 // <100 msg/sec

	// Quality distribution
	LowLatencyClients    int64 // <1ms avg latency
	MediumLatencyClients int64 // 1-10ms avg latency
	HighLatencyClients   int64 // >10ms avg latency

	// System health
	ActiveMeasurements    int64
	MeasurementErrors     int64
	ClassificationUpdates int64

	_ [5]int64 // Cache line padding
}

// NewClientCapabilityDetector creates a new capability detector
func NewClientCapabilityDetector(logger *zap.Logger) *ClientCapabilityDetector {
	detector := &ClientCapabilityDetector{
		logger:   logger,
		shutdown: make(chan struct{}),
		metrics:  &CapabilityMetrics{},
	}

	// Initialize empty capabilities map
	emptyMap := make(map[string]*ClientCapability)
	atomic.StorePointer(&detector.capabilities, unsafe.Pointer(&emptyMap))
	atomic.StoreInt64(&detector.capVersion, 1)

	return detector
}

// Start begins capability detection and measurement
func (ccd *ClientCapabilityDetector) Start(ctx context.Context) error {
	ccd.logger.Info("Starting client capability detector")

	// Start measurement workers
	ccd.measurementWorkers.Add(2)
	go ccd.measurementProcessor(ctx)
	go ccd.classificationUpdater(ctx)

	return nil
}

// RegisterClient registers a new client for capability detection
func (ccd *ClientCapabilityDetector) RegisterClient(clientID string) error {
	capability := &ClientCapability{
		ClientID:    clientID,
		ConnectedAt: time.Now(),

		// Initialize with conservative defaults
		SafeRate:       10,  // 10 msg/sec safe default
		MaxRate:        100, // 100 msg/sec initial max
		BurstRate:      200, // 200 msg/sec burst
		ClientType:     ClientTypeUnknown,
		TradingProfile: ProfileUnknown,
		PriorityClass:  PriorityMedium,

		lastMeasurement: time.Now().UnixNano(),
	}

	// Initialize measurement window
	window := &MeasurementWindow{
		lastTime: time.Now().UnixNano(),
	}
	atomic.StorePointer(&capability.measurementWindow, unsafe.Pointer(window))

	// Add to capabilities map atomically
	for {
		currentMapPtr := atomic.LoadPointer(&ccd.capabilities)
		currentMap := *(*map[string]*ClientCapability)(currentMapPtr)

		// Check if client already exists
		if _, exists := currentMap[clientID]; exists {
			return nil // Already registered
		}

		// Create new map with the client added
		newMap := make(map[string]*ClientCapability, len(currentMap)+1)
		for k, v := range currentMap {
			newMap[k] = v
		}
		newMap[clientID] = capability

		// Atomic update
		if atomic.CompareAndSwapPointer(&ccd.capabilities, currentMapPtr, unsafe.Pointer(&newMap)) {
			atomic.AddInt64(&ccd.capVersion, 1)
			break
		}
		// Retry on conflict
	}

	ccd.logger.Debug("Client registered for capability detection",
		zap.String("client_id", clientID))

	return nil
}

// RecordBandwidth records bandwidth measurement for a client
func (ccd *ClientCapabilityDetector) RecordBandwidth(clientID string, bytesTransferred int64, duration time.Duration) {
	capability := ccd.getClientCapability(clientID)
	if capability == nil {
		return
	}

	bandwidth := int64(float64(bytesTransferred) / duration.Seconds())

	// Update atomic bandwidth measurements
	atomic.StoreInt64(&capability.CurrentBandwidth, bandwidth)

	// Update moving averages
	ccd.updateBandwidthStats(capability, bandwidth)
}

// RecordProcessingSpeed records message processing speed for a client
func (ccd *ClientCapabilityDetector) RecordProcessingSpeed(clientID string, messagesProcessed int64, duration time.Duration) {
	capability := ccd.getClientCapability(clientID)
	if capability == nil {
		return
	}

	processingSpeed := int64(float64(messagesProcessed) / duration.Seconds())

	// Update atomic processing speed measurements
	atomic.StoreInt64(&capability.CurrentProcessingSpeed, processingSpeed)

	// Update moving averages
	ccd.updateProcessingStats(capability, processingSpeed)
}

// RecordLatency records latency measurement for a client
func (ccd *ClientCapabilityDetector) RecordLatency(clientID string, latency time.Duration) {
	capability := ccd.getClientCapability(clientID)
	if capability == nil {
		return
	}

	latencyNanos := latency.Nanoseconds()

	// Update atomic latency measurements
	atomic.StoreInt64(&capability.CurrentLatency, latencyNanos)

	// Update moving averages
	ccd.updateLatencyStats(capability, latencyNanos)
}

// GetClientCapability returns current capability assessment for a client
func (ccd *ClientCapabilityDetector) GetClientCapability(clientID string) *ClientCapability {
	return ccd.getClientCapability(clientID)
}

// GetAdaptiveRateLimit returns the appropriate rate limit for a client
func (ccd *ClientCapabilityDetector) GetAdaptiveRateLimit(clientID string, messageType MessagePriority) int64 {
	capability := ccd.getClientCapability(clientID)
	if capability == nil {
		return 10 // Conservative default
	}

	// Select rate based on message priority and client capability
	switch messageType {
	case PriorityCritical:
		// Critical messages get highest sustainable rate
		return atomic.LoadInt64(&capability.MaxRate)
	case PriorityHigh:
		// High priority gets 80% of max rate
		return int64(float64(atomic.LoadInt64(&capability.MaxRate)) * 0.8)
	case PriorityMedium:
		// Medium priority gets safe rate
		return atomic.LoadInt64(&capability.SafeRate)
	default:
		// Low priority gets conservative rate
		return int64(float64(atomic.LoadInt64(&capability.SafeRate)) * 0.5)
	}
}

// getClientCapability retrieves client capability from lock-free map
func (ccd *ClientCapabilityDetector) getClientCapability(clientID string) *ClientCapability {
	currentMapPtr := atomic.LoadPointer(&ccd.capabilities)
	currentMap := *(*map[string]*ClientCapability)(currentMapPtr)

	capability, exists := currentMap[clientID]
	if !exists {
		return nil
	}

	return capability
}

// updateBandwidthStats updates bandwidth statistics in the measurement window
func (ccd *ClientCapabilityDetector) updateBandwidthStats(capability *ClientCapability, bandwidth int64) {
	windowPtr := atomic.LoadPointer(&capability.measurementWindow)
	window := (*MeasurementWindow)(windowPtr)

	window.mu.Lock()
	defer window.mu.Unlock()

	// Add to circular buffer
	window.bandwidthSamples[window.head] = bandwidth
	window.bandwidthSum += bandwidth

	// Remove old sample if buffer is full
	if window.size == len(window.bandwidthSamples) {
		oldSample := window.bandwidthSamples[(window.head+1)%len(window.bandwidthSamples)]
		window.bandwidthSum -= oldSample
	} else {
		window.size++
	}

	window.head = (window.head + 1) % len(window.bandwidthSamples)

	// Update averages
	if window.size > 0 {
		avgBandwidth := window.bandwidthSum / int64(window.size)
		atomic.StoreInt64(&capability.AvgBandwidth, avgBandwidth)

		// Update max if needed
		maxBandwidth := atomic.LoadInt64(&capability.MaxBandwidth)
		if bandwidth > maxBandwidth {
			atomic.StoreInt64(&capability.MaxBandwidth, bandwidth)
		}
	}
}

// updateProcessingStats updates processing speed statistics
func (ccd *ClientCapabilityDetector) updateProcessingStats(capability *ClientCapability, processingSpeed int64) {
	windowPtr := atomic.LoadPointer(&capability.measurementWindow)
	window := (*MeasurementWindow)(windowPtr)

	window.mu.Lock()
	defer window.mu.Unlock()

	// Add to circular buffer
	window.processingSamples[window.head] = processingSpeed
	window.processingSum += processingSpeed

	// Remove old sample if buffer is full
	if window.size == len(window.processingSamples) {
		oldSample := window.processingSamples[(window.head+1)%len(window.processingSamples)]
		window.processingSum -= oldSample
	}

	// Update averages
	if window.size > 0 {
		avgProcessing := window.processingSum / int64(window.size)
		atomic.StoreInt64(&capability.AvgProcessingSpeed, avgProcessing)

		// Update max if needed
		maxProcessing := atomic.LoadInt64(&capability.MaxProcessingSpeed)
		if processingSpeed > maxProcessing {
			atomic.StoreInt64(&capability.MaxProcessingSpeed, processingSpeed)
		}
	}
}

// updateLatencyStats updates latency statistics
func (ccd *ClientCapabilityDetector) updateLatencyStats(capability *ClientCapability, latency int64) {
	windowPtr := atomic.LoadPointer(&capability.measurementWindow)
	window := (*MeasurementWindow)(windowPtr)

	window.mu.Lock()
	defer window.mu.Unlock()

	// Add to circular buffer
	window.latencySamples[window.head] = latency
	window.latencySum += latency

	// Remove old sample if buffer is full
	if window.size == len(window.latencySamples) {
		oldSample := window.latencySamples[(window.head+1)%len(window.latencySamples)]
		window.latencySum -= oldSample
	}

	// Update averages
	if window.size > 0 {
		avgLatency := window.latencySum / int64(window.size)
		atomic.StoreInt64(&capability.AvgLatency, avgLatency)

		// Update min if needed
		minLatency := atomic.LoadInt64(&capability.MinLatency)
		if latency < minLatency || minLatency == 0 {
			atomic.StoreInt64(&capability.MinLatency, latency)
		}
	}
}

// measurementProcessor processes capability measurements
func (ccd *ClientCapabilityDetector) measurementProcessor(ctx context.Context) {
	defer ccd.measurementWorkers.Done()

	ticker := time.NewTicker(100 * time.Millisecond) // Fast measurement updates
	defer ticker.Stop()

	ccd.logger.Info("Capability measurement processor started")

	for {
		select {
		case <-ticker.C:
			ccd.updateActiveMetrics()

		case <-ccd.shutdown:
			ccd.logger.Info("Capability measurement processor shutting down")
			return

		case <-ctx.Done():
			return
		}
	}
}

// classificationUpdater updates client classifications based on measurements
func (ccd *ClientCapabilityDetector) classificationUpdater(ctx context.Context) {
	defer ccd.measurementWorkers.Done()

	ticker := time.NewTicker(5 * time.Second) // Slower classification updates
	defer ticker.Stop()

	ccd.logger.Info("Client classification updater started")

	for {
		select {
		case <-ticker.C:
			ccd.updateClientClassifications()

		case <-ccd.shutdown:
			ccd.logger.Info("Client classification updater shutting down")
			return

		case <-ctx.Done():
			return
		}
	}
}

// updateActiveMetrics updates global capability metrics
func (ccd *ClientCapabilityDetector) updateActiveMetrics() {
	currentMapPtr := atomic.LoadPointer(&ccd.capabilities)
	currentMap := *(*map[string]*ClientCapability)(currentMapPtr)

	var (
		retailCount        int64
		institutionalCount int64
		marketMakerCount   int64
		hftCount           int64
		apiCount           int64

		highPerfCount   int64
		mediumPerfCount int64
		lowPerfCount    int64

		lowLatencyCount    int64
		mediumLatencyCount int64
		highLatencyCount   int64
	)

	for _, capability := range currentMap {
		// Count by client type
		switch capability.ClientType {
		case ClientTypeRetail:
			retailCount++
		case ClientTypeInstitutional:
			institutionalCount++
		case ClientTypeMarketMaker:
			marketMakerCount++
		case ClientTypeHFT:
			hftCount++
		case ClientTypeAPI:
			apiCount++
		}

		// Count by performance
		processingSpeed := atomic.LoadInt64(&capability.CurrentProcessingSpeed)
		if processingSpeed > 1000 {
			highPerfCount++
		} else if processingSpeed > 100 {
			mediumPerfCount++
		} else {
			lowPerfCount++
		}

		// Count by latency
		latency := atomic.LoadInt64(&capability.CurrentLatency)
		if latency < int64(time.Millisecond) {
			lowLatencyCount++
		} else if latency < 10*int64(time.Millisecond) {
			mediumLatencyCount++
		} else {
			highLatencyCount++
		}
	}

	// Update metrics atomically
	atomic.StoreInt64(&ccd.metrics.RetailClients, retailCount)
	atomic.StoreInt64(&ccd.metrics.InstitutionalClients, institutionalCount)
	atomic.StoreInt64(&ccd.metrics.MarketMakerClients, marketMakerCount)
	atomic.StoreInt64(&ccd.metrics.HFTClients, hftCount)
	atomic.StoreInt64(&ccd.metrics.APIClients, apiCount)

	atomic.StoreInt64(&ccd.metrics.HighPerformanceClients, highPerfCount)
	atomic.StoreInt64(&ccd.metrics.MediumPerformanceClients, mediumPerfCount)
	atomic.StoreInt64(&ccd.metrics.LowPerformanceClients, lowPerfCount)

	atomic.StoreInt64(&ccd.metrics.LowLatencyClients, lowLatencyCount)
	atomic.StoreInt64(&ccd.metrics.MediumLatencyClients, mediumLatencyCount)
	atomic.StoreInt64(&ccd.metrics.HighLatencyClients, highLatencyCount)
}

// updateClientClassifications updates client type and priority classifications
func (ccd *ClientCapabilityDetector) updateClientClassifications() {
	currentMapPtr := atomic.LoadPointer(&ccd.capabilities)
	currentMap := *(*map[string]*ClientCapability)(currentMapPtr)

	for clientID, capability := range currentMap {
		ccd.classifyClient(clientID, capability)
	}
}

// classifyClient classifies a single client based on behavior patterns
func (ccd *ClientCapabilityDetector) classifyClient(clientID string, capability *ClientCapability) {
	processingSpeed := atomic.LoadInt64(&capability.CurrentProcessingSpeed)
	latency := atomic.LoadInt64(&capability.CurrentLatency)
	bandwidth := atomic.LoadInt64(&capability.CurrentBandwidth)
	// Classify client type based on performance characteristics
	var newClientType ClientType
	var newPriority MessagePriority

	if processingSpeed > 10000 && latency < int64(time.Millisecond) {
		// High-frequency trading characteristics
		newClientType = ClientTypeHFT
		newPriority = PriorityHigh
	} else if processingSpeed > 1000 && bandwidth > 1024*1024 { // >1MB/s
		// Market maker characteristics
		newClientType = ClientTypeMarketMaker
		newPriority = PriorityHigh
	} else if bandwidth > 512*1024 { // >512KB/s
		// Institutional characteristics
		newClientType = ClientTypeInstitutional
		newPriority = PriorityMedium
	} else if processingSpeed > 100 {
		// API client characteristics
		newClientType = ClientTypeAPI
		newPriority = PriorityMedium
	} else {
		// Retail client characteristics
		newClientType = ClientTypeRetail
		newPriority = PriorityLow
	}

	// Update classification if changed
	if capability.ClientType != newClientType || capability.PriorityClass != newPriority {
		capability.mu.Lock()
		capability.ClientType = newClientType
		capability.PriorityClass = newPriority
		capability.mu.Unlock()

		// Update adaptive rate limits based on new classification
		ccd.updateAdaptiveRateLimits(capability)

		atomic.AddInt64(&ccd.metrics.ClassificationUpdates, 1)

		ccd.logger.Debug("Client classification updated",
			zap.String("client_id", clientID),
			zap.Int("client_type", int(newClientType)),
			zap.Int("priority", int(newPriority)))
	}
}

// updateAdaptiveRateLimits updates rate limits based on client capability
func (ccd *ClientCapabilityDetector) updateAdaptiveRateLimits(capability *ClientCapability) {
	processingSpeed := atomic.LoadInt64(&capability.CurrentProcessingSpeed)

	// Calculate safe, max, and burst rates based on processing capability
	safeRate := int64(float64(processingSpeed) * 0.6)  // 60% of processing speed
	maxRate := int64(float64(processingSpeed) * 0.8)   // 80% of processing speed
	burstRate := int64(float64(processingSpeed) * 1.2) // 120% for short bursts

	// Apply minimums based on client type
	switch capability.ClientType {
	case ClientTypeHFT:
		if safeRate < 1000 {
			safeRate = 1000
		}
		if maxRate < 2000 {
			maxRate = 2000
		}
	case ClientTypeMarketMaker:
		if safeRate < 500 {
			safeRate = 500
		}
		if maxRate < 1000 {
			maxRate = 1000
		}
	case ClientTypeInstitutional:
		if safeRate < 100 {
			safeRate = 100
		}
		if maxRate < 200 {
			maxRate = 200
		}
	default:
		if safeRate < 10 {
			safeRate = 10
		}
		if maxRate < 50 {
			maxRate = 50
		}
	}

	// Update rates atomically
	atomic.StoreInt64(&capability.SafeRate, safeRate)
	atomic.StoreInt64(&capability.MaxRate, maxRate)
	atomic.StoreInt64(&capability.BurstRate, burstRate)
}

// GetMetrics returns current capability detection metrics
func (ccd *ClientCapabilityDetector) GetMetrics() CapabilityMetrics {
	return CapabilityMetrics{
		RetailClients:        atomic.LoadInt64(&ccd.metrics.RetailClients),
		InstitutionalClients: atomic.LoadInt64(&ccd.metrics.InstitutionalClients),
		MarketMakerClients:   atomic.LoadInt64(&ccd.metrics.MarketMakerClients),
		HFTClients:           atomic.LoadInt64(&ccd.metrics.HFTClients),
		APIClients:           atomic.LoadInt64(&ccd.metrics.APIClients),

		HighPerformanceClients:   atomic.LoadInt64(&ccd.metrics.HighPerformanceClients),
		MediumPerformanceClients: atomic.LoadInt64(&ccd.metrics.MediumPerformanceClients),
		LowPerformanceClients:    atomic.LoadInt64(&ccd.metrics.LowPerformanceClients),

		LowLatencyClients:    atomic.LoadInt64(&ccd.metrics.LowLatencyClients),
		MediumLatencyClients: atomic.LoadInt64(&ccd.metrics.MediumLatencyClients),
		HighLatencyClients:   atomic.LoadInt64(&ccd.metrics.HighLatencyClients),

		ActiveMeasurements:    atomic.LoadInt64(&ccd.metrics.ActiveMeasurements),
		MeasurementErrors:     atomic.LoadInt64(&ccd.metrics.MeasurementErrors),
		ClassificationUpdates: atomic.LoadInt64(&ccd.metrics.ClassificationUpdates),
	}
}

// Stop gracefully shuts down the capability detector
func (ccd *ClientCapabilityDetector) Stop(ctx context.Context) error {
	ccd.shutdownOnce.Do(func() {
		ccd.logger.Info("Stopping client capability detector")

		close(ccd.shutdown)

		// Wait for workers to complete
		done := make(chan struct{})
		go func() {
			ccd.measurementWorkers.Wait()
			close(done)
		}()

		select {
		case <-done:
			ccd.logger.Info("Client capability detector stopped successfully")
		case <-ctx.Done():
			ccd.logger.Warn("Capability detector shutdown timed out")
		}
	})

	return nil
}
