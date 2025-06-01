// Package ws provides enhanced WebSocket connection management with memory leak prevention
package ws

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

// ConnectionState represents the state of a WebSocket connection
type ConnectionState int32

const (
	StateConnecting ConnectionState = iota
	StateConnected
	StateClosing
	StateClosed
	StateError
)

func (s ConnectionState) String() string {
	switch s {
	case StateConnecting:
		return "connecting"
	case StateConnected:
		return "connected"
	case StateClosing:
		return "closing"
	case StateClosed:
		return "closed"
	case StateError:
		return "error"
	default:
		return "unknown"
	}
}

// ConnectionMetrics tracks connection-level metrics
type ConnectionMetrics struct {
	CreatedAt          time.Time     `json:"created_at"`
	LastActivity       time.Time     `json:"last_activity"`
	MessagesSent       int64         `json:"messages_sent"`
	MessagesReceived   int64         `json:"messages_received"`
	BytesSent          int64         `json:"bytes_sent"`
	BytesReceived      int64         `json:"bytes_received"`
	ReconnectCount     int32         `json:"reconnect_count"`
	LastError          string        `json:"last_error,omitempty"`
	SubscriptionsCount int32         `json:"subscriptions_count"`
	AverageLatency     time.Duration `json:"average_latency"`
	ConnectionDuration time.Duration `json:"connection_duration"`
}

// ConnectionConfig holds configuration for connection management
type ConnectionConfig struct {
	// Heartbeat settings
	HeartbeatInterval   time.Duration `json:"heartbeat_interval"`
	HeartbeatTimeout    time.Duration `json:"heartbeat_timeout"`
	MaxMissedHeartbeats int           `json:"max_missed_heartbeats"`

	// Connection limits
	MaxConnections      int           `json:"max_connections"`
	MaxConnectionsPerIP int           `json:"max_connections_per_ip"`
	ConnectionTimeout   time.Duration `json:"connection_timeout"`

	// Buffer settings
	ReadBufferSize    int `json:"read_buffer_size"`
	WriteBufferSize   int `json:"write_buffer_size"`
	MessageBufferSize int `json:"message_buffer_size"`

	// Cleanup settings
	CleanupInterval time.Duration `json:"cleanup_interval"`
	IdleTimeout     time.Duration `json:"idle_timeout"`

	// Resource limits
	MaxMemoryPerConn     int64 `json:"max_memory_per_conn"`
	MaxGoroutinesPerConn int   `json:"max_goroutines_per_conn"`
}

// DefaultConnectionConfig returns a default configuration optimized for production
func DefaultConnectionConfig() ConnectionConfig {
	return ConnectionConfig{
		HeartbeatInterval:    30 * time.Second,
		HeartbeatTimeout:     10 * time.Second,
		MaxMissedHeartbeats:  3,
		MaxConnections:       10000,
		MaxConnectionsPerIP:  100,
		ConnectionTimeout:    30 * time.Second,
		ReadBufferSize:       4096,
		WriteBufferSize:      4096,
		MessageBufferSize:    256,
		CleanupInterval:      5 * time.Minute,
		IdleTimeout:          15 * time.Minute,
		MaxMemoryPerConn:     50 * 1024 * 1024, // 50MB per connection
		MaxGoroutinesPerConn: 10,
	}
}

// ResourceTracker tracks resource usage for memory leak detection
type ResourceTracker struct {
	connectionCount int64
	goroutineCount  int64
	memoryUsage     int64
	lastCleanup     time.Time
	mu              sync.RWMutex
}

// NewResourceTracker creates a new resource tracker
func NewResourceTracker() *ResourceTracker {
	return &ResourceTracker{
		lastCleanup: time.Now(),
	}
}

// TrackConnection increments connection count
func (rt *ResourceTracker) TrackConnection() {
	atomic.AddInt64(&rt.connectionCount, 1)
}

// UntrackConnection decrements connection count
func (rt *ResourceTracker) UntrackConnection() {
	atomic.AddInt64(&rt.connectionCount, -1)
}

// TrackGoroutine increments goroutine count
func (rt *ResourceTracker) TrackGoroutine() {
	atomic.AddInt64(&rt.goroutineCount, 1)
}

// UntrackGoroutine decrements goroutine count
func (rt *ResourceTracker) UntrackGoroutine() {
	atomic.AddInt64(&rt.goroutineCount, -1)
}

// GetStats returns current resource statistics
func (rt *ResourceTracker) GetStats() map[string]interface{} {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	return map[string]interface{}{
		"connections":        atomic.LoadInt64(&rt.connectionCount),
		"tracked_goroutines": atomic.LoadInt64(&rt.goroutineCount),
		"system_goroutines":  runtime.NumGoroutine(),
		"heap_alloc":         memStats.HeapAlloc,
		"heap_sys":           memStats.HeapSys,
		"gc_cycles":          memStats.NumGC,
		"last_cleanup":       rt.lastCleanup,
	}
}

// ConnectionManager manages WebSocket connections with enhanced lifecycle management
type ConnectionManager struct {
	config          ConnectionConfig
	logger          *zap.Logger
	resourceTracker *ResourceTracker

	// Connection tracking
	connections   sync.Map // connectionID -> *EnhancedClient
	ipConnections sync.Map // IP -> connectionCount

	// Lifecycle management
	cleanup       chan string
	shutdown      chan struct{}
	cleanupTicker *time.Ticker

	// Metrics
	metrics *ConnectionManagerMetrics

	// State
	running          int32
	totalConnections int64
}

// ConnectionManagerMetrics tracks manager-level metrics
type ConnectionManagerMetrics struct {
	ActiveConnections    prometheus.Gauge
	TotalConnections     prometheus.Counter
	ConnectionsCreated   prometheus.Counter
	ConnectionsDestroyed prometheus.Counter
	MemoryLeaksDetected  prometheus.Counter
	GoroutineLeaks       prometheus.Counter
	CleanupOperations    prometheus.Counter
	HeartbeatFailures    prometheus.Counter
	ConnectionsPerIP     *prometheus.GaugeVec
	ResourceUsage        *prometheus.GaugeVec
}

// NewConnectionManagerMetrics creates connection manager metrics
func NewConnectionManagerMetrics() *ConnectionManagerMetrics {
	metrics := &ConnectionManagerMetrics{
		ActiveConnections: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "ws_active_connections",
			Help: "Current number of active WebSocket connections",
		}),
		TotalConnections: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "ws_total_connections",
			Help: "Total number of WebSocket connections created",
		}),
		ConnectionsCreated: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "ws_connections_created_total",
			Help: "Total number of WebSocket connections created",
		}),
		ConnectionsDestroyed: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "ws_connections_destroyed_total",
			Help: "Total number of WebSocket connections destroyed",
		}),
		MemoryLeaksDetected: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "ws_memory_leaks_detected_total",
			Help: "Total number of memory leaks detected and cleaned up",
		}),
		GoroutineLeaks: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "ws_goroutine_leaks_total",
			Help: "Total number of goroutine leaks detected",
		}),
		CleanupOperations: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "ws_cleanup_operations_total",
			Help: "Total number of cleanup operations performed",
		}),
		HeartbeatFailures: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "ws_heartbeat_failures_total",
			Help: "Total number of heartbeat failures",
		}),
		ConnectionsPerIP: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "ws_connections_per_ip",
			Help: "Number of connections per IP address",
		}, []string{"ip"}),
		ResourceUsage: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "ws_resource_usage",
			Help: "Resource usage metrics for WebSocket connections",
		}, []string{"resource_type"}),
	}

	// Register metrics
	prometheus.MustRegister(
		metrics.ActiveConnections,
		metrics.TotalConnections,
		metrics.ConnectionsCreated,
		metrics.ConnectionsDestroyed,
		metrics.MemoryLeaksDetected,
		metrics.GoroutineLeaks,
		metrics.CleanupOperations,
		metrics.HeartbeatFailures,
		metrics.ConnectionsPerIP,
		metrics.ResourceUsage,
	)

	return metrics
}

// NewConnectionManager creates a new connection manager
func NewConnectionManager(config ConnectionConfig, logger *zap.Logger) *ConnectionManager {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &ConnectionManager{
		config:          config,
		logger:          logger,
		resourceTracker: NewResourceTracker(),
		cleanup:         make(chan string, 1000),
		shutdown:        make(chan struct{}),
		metrics:         NewConnectionManagerMetrics(),
	}
}

// Start begins the connection manager
func (cm *ConnectionManager) Start(ctx context.Context) error {
	if !atomic.CompareAndSwapInt32(&cm.running, 0, 1) {
		return nil // Already running
	}

	cm.logger.Info("Starting WebSocket connection manager")

	// Start cleanup ticker
	cm.cleanupTicker = time.NewTicker(cm.config.CleanupInterval)

	// Start background workers
	go cm.cleanupWorker(ctx)
	go cm.resourceMonitor(ctx)
	go cm.metricsCollector(ctx)

	return nil
}

// Stop gracefully shuts down the connection manager
func (cm *ConnectionManager) Stop() error {
	if !atomic.CompareAndSwapInt32(&cm.running, 1, 0) {
		return nil // Already stopped
	}

	cm.logger.Info("Stopping WebSocket connection manager")

	// Signal shutdown
	close(cm.shutdown)

	// Stop cleanup ticker
	if cm.cleanupTicker != nil {
		cm.cleanupTicker.Stop()
	}

	// Close all connections
	cm.connections.Range(func(key, value interface{}) bool {
		if client, ok := value.(*EnhancedClient); ok {
			client.Close(websocket.CloseGoingAway, "Server shutdown")
		}
		return true
	})

	// Wait for cleanup to complete
	time.Sleep(1 * time.Second)

	cm.logger.Info("WebSocket connection manager stopped")
	return nil
}

// cleanupWorker performs periodic cleanup of stale connections
func (cm *ConnectionManager) cleanupWorker(ctx context.Context) {
	defer cm.resourceTracker.UntrackGoroutine()
	cm.resourceTracker.TrackGoroutine()

	for {
		select {
		case <-ctx.Done():
			return
		case <-cm.shutdown:
			return
		case <-cm.cleanupTicker.C:
			cm.performCleanup()
		case connID := <-cm.cleanup:
			cm.cleanupConnection(connID)
		}
	}
}

// performCleanup performs a full cleanup cycle
func (cm *ConnectionManager) performCleanup() {
	start := time.Now()
	cleaned := 0

	cm.connections.Range(func(key, value interface{}) bool {
		connID := key.(string)
		client := value.(*EnhancedClient)

		// Check if connection should be cleaned up
		if cm.shouldCleanup(client) {
			cm.cleanupConnection(connID)
			cleaned++
		}

		return true
	})

	cm.resourceTracker.mu.Lock()
	cm.resourceTracker.lastCleanup = time.Now()
	cm.resourceTracker.mu.Unlock()

	cm.metrics.CleanupOperations.Inc()

	if cleaned > 0 {
		cm.logger.Info("Cleanup completed",
			zap.Int("connections_cleaned", cleaned),
			zap.Duration("duration", time.Since(start)))
	}
}

// shouldCleanup determines if a connection should be cleaned up
func (cm *ConnectionManager) shouldCleanup(client *EnhancedClient) bool {
	state := client.GetState()

	// Clean up closed or error connections
	if state == StateClosed || state == StateError {
		return true
	}

	// Clean up idle connections
	if time.Since(client.GetLastActivity()) > cm.config.IdleTimeout {
		return true
	}

	// Clean up connections with too many missed heartbeats
	if client.GetMissedHeartbeats() > cm.config.MaxMissedHeartbeats {
		return true
	}

	return false
}

// cleanupConnection removes a connection and cleans up its resources
func (cm *ConnectionManager) cleanupConnection(connID string) {
	if value, exists := cm.connections.LoadAndDelete(connID); exists {
		if client, ok := value.(*EnhancedClient); ok {
			// Get IP for metrics cleanup
			ip := client.GetRemoteAddr()

			// Close the connection
			client.Close(websocket.CloseGoingAway, "Connection cleanup")

			// Update IP connection count
			if countVal, exists := cm.ipConnections.Load(ip); exists {
				count := countVal.(int32) - 1
				if count <= 0 {
					cm.ipConnections.Delete(ip)
					cm.metrics.ConnectionsPerIP.DeleteLabelValues(ip)
				} else {
					cm.ipConnections.Store(ip, count)
					cm.metrics.ConnectionsPerIP.WithLabelValues(ip).Set(float64(count))
				}
			}

			// Update metrics
			cm.metrics.ActiveConnections.Dec()
			cm.metrics.ConnectionsDestroyed.Inc()
			cm.resourceTracker.UntrackConnection()

			cm.logger.Debug("Connection cleaned up",
				zap.String("connection_id", connID),
				zap.String("ip", ip),
				zap.String("state", client.GetState().String()))
		}
	}
}

// resourceMonitor monitors resource usage for leak detection
func (cm *ConnectionManager) resourceMonitor(ctx context.Context) {
	defer cm.resourceTracker.UntrackGoroutine()
	cm.resourceTracker.TrackGoroutine()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	var lastGoroutineCount int

	for {
		select {
		case <-ctx.Done():
			return
		case <-cm.shutdown:
			return
		case <-ticker.C:
			stats := cm.resourceTracker.GetStats()

			// Check for goroutine leaks
			currentGoroutines := runtime.NumGoroutine()
			if lastGoroutineCount > 0 {
				goroutineDiff := currentGoroutines - lastGoroutineCount
				if goroutineDiff > 100 { // Threshold for goroutine leak detection
					cm.metrics.GoroutineLeaks.Inc()
					cm.logger.Warn("Potential goroutine leak detected",
						zap.Int("current_goroutines", currentGoroutines),
						zap.Int("previous_goroutines", lastGoroutineCount),
						zap.Int("difference", goroutineDiff))
				}
			}
			lastGoroutineCount = currentGoroutines

			// Update resource metrics
			cm.metrics.ResourceUsage.WithLabelValues("goroutines").Set(float64(currentGoroutines))
			cm.metrics.ResourceUsage.WithLabelValues("heap_alloc").Set(float64(stats["heap_alloc"].(uint64)))
			cm.metrics.ResourceUsage.WithLabelValues("connections").Set(float64(stats["connections"].(int64)))
		}
	}
}

// metricsCollector collects and updates various metrics
func (cm *ConnectionManager) metricsCollector(ctx context.Context) {
	defer cm.resourceTracker.UntrackGoroutine()
	cm.resourceTracker.TrackGoroutine()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-cm.shutdown:
			return
		case <-ticker.C:
			// Update connection count metrics
			activeCount := int64(0)
			cm.connections.Range(func(key, value interface{}) bool {
				activeCount++
				return true
			})
			cm.metrics.ActiveConnections.Set(float64(activeCount))
		}
	}
}

// CanAcceptConnection checks if a new connection can be accepted
func (cm *ConnectionManager) CanAcceptConnection(remoteAddr string) bool {
	// Check total connection limit
	activeCount := int64(0)
	cm.connections.Range(func(key, value interface{}) bool {
		activeCount++
		return true
	})

	if activeCount >= int64(cm.config.MaxConnections) {
		return false
	}

	// Check per-IP connection limit
	if countVal, exists := cm.ipConnections.Load(remoteAddr); exists {
		count := countVal.(int32)
		if count >= int32(cm.config.MaxConnectionsPerIP) {
			return false
		}
	}

	return true
}

// RegisterConnection registers a new connection with the manager
func (cm *ConnectionManager) RegisterConnection(client *EnhancedClient) error {
	connID := client.GetID()
	remoteAddr := client.GetRemoteAddr()

	// Check if connection can be accepted
	if !cm.CanAcceptConnection(remoteAddr) {
		return ErrConnectionLimitExceeded
	}

	// Store connection
	cm.connections.Store(connID, client)

	// Update IP connection count
	if countVal, exists := cm.ipConnections.Load(remoteAddr); exists {
		count := countVal.(int32) + 1
		cm.ipConnections.Store(remoteAddr, count)
		cm.metrics.ConnectionsPerIP.WithLabelValues(remoteAddr).Set(float64(count))
	} else {
		cm.ipConnections.Store(remoteAddr, int32(1))
		cm.metrics.ConnectionsPerIP.WithLabelValues(remoteAddr).Set(1)
	}

	// Update metrics
	cm.metrics.ActiveConnections.Inc()
	cm.metrics.ConnectionsCreated.Inc()
	cm.metrics.TotalConnections.Inc()
	cm.resourceTracker.TrackConnection()

	atomic.AddInt64(&cm.totalConnections, 1)

	cm.logger.Debug("Connection registered",
		zap.String("connection_id", connID),
		zap.String("remote_addr", remoteAddr))

	return nil
}

// UnregisterConnection unregisters a connection from the manager
func (cm *ConnectionManager) UnregisterConnection(connID string) {
	select {
	case cm.cleanup <- connID:
		// Queued for cleanup
	default:
		// Cleanup queue full, perform immediate cleanup
		cm.cleanupConnection(connID)
	}
}

// GetConnectionCount returns the current number of active connections
func (cm *ConnectionManager) GetConnectionCount() int64 {
	count := int64(0)
	cm.connections.Range(func(key, value interface{}) bool {
		count++
		return true
	})
	return count
}

// GetConnection retrieves a connection by ID
func (cm *ConnectionManager) GetConnection(connID string) (*EnhancedClient, bool) {
	if value, exists := cm.connections.Load(connID); exists {
		if client, ok := value.(*EnhancedClient); ok {
			return client, true
		}
	}
	return nil, false
}

// GetResourceStats returns current resource statistics
func (cm *ConnectionManager) GetResourceStats() map[string]interface{} {
	stats := cm.resourceTracker.GetStats()
	stats["total_connections"] = atomic.LoadInt64(&cm.totalConnections)
	stats["running"] = atomic.LoadInt32(&cm.running) == 1
	return stats
}

// Custom errors
var (
	ErrConnectionLimitExceeded = websocket.CloseError{Code: websocket.ClosePolicyViolation, Text: "Connection limit exceeded"}
	ErrManagerNotRunning       = websocket.CloseError{Code: websocket.CloseServiceRestart, Text: "Connection manager not running"}
)
