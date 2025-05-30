// =============================
// Order Book Adapter for Gradual Migration
// =============================
// This file provides an adapter pattern to gradually migrate from the
// existing OrderBook to the new DeadlockSafeOrderBook implementation.
// The adapter allows runtime switching between implementations and
// provides compatibility layer for existing engine integration.

package orderbook

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/trading/model"
	"github.com/google/uuid"
)

// OrderBookInterface defines the common interface for both implementations
type OrderBookInterface interface {
	// Core operations
	AddOrder(order *model.Order) (*AddOrderResult, error)
	CancelOrder(orderID uuid.UUID) error
	GetSnapshot(depth int) ([][]string, [][]string)

	// Administrative controls
	HaltTrading()
	ResumeTrading()
	TriggerCircuitBreaker()
	EmergencyShutdown()

	// Metrics and monitoring
	OrdersCount() int
	GetOrderIDs() []uuid.UUID

	// Advanced features (for compatibility)
	ProcessLimitOrder(ctx context.Context, order *model.Order) (*model.Order, []*model.Trade, []*model.Order, error)
	ProcessMarketOrder(ctx context.Context, order *model.Order) (*model.Order, []*model.Trade, []*model.Order, error)
	CanFullyFill(order *model.Order) bool

	// Exposure for engine integration
	GetMutex() *sync.RWMutex
}

// MigrationConfig controls the migration behavior
type MigrationConfig struct {
	// Migration strategy
	EnableNewImplementation bool  // Whether to use new implementation
	GradualMigrationEnabled bool  // Enable gradual migration
	MigrationPercentage     int32 // Percentage of requests to route to new implementation (0-100)

	// Gradual migration configuration
	EnableGradualMigration     bool          // Enable gradual migration steps
	InitialMigrationPercentage int32         // Initial migration percentage
	MaxMigrationPercentage     int32         // Maximum migration percentage
	StepSize                   int32         // Percentage increase per step
	StepInterval               time.Duration // Time between migration steps

	// Fallback configuration
	EnableFallback         bool          // Enable fallback to old implementation on errors
	FallbackTimeout        time.Duration // Timeout before fallback
	MaxConsecutiveFailures int32         // Max failures before switching back

	// Performance thresholds
	LatencyThresholdMs     int64 // Switch to new implementation if latency exceeds this
	ThroughputThresholdOps int64 // Switch based on throughput requirements
	ConflictThreshold      int32 // Switch if conflicts exceed this percentage

	// Circuit breaker configuration
	CircuitBreakerConfig *CircuitBreakerConfig // Circuit breaker settings

	// Monitoring
	CollectMetrics     bool // Enable performance comparison metrics
	LogMigrationEvents bool // Log migration decisions
}

// CircuitBreakerConfig defines circuit breaker parameters
type CircuitBreakerConfig struct {
	FailureThreshold         int32         `json:"failure_threshold"`
	RecoveryTimeout          time.Duration `json:"recovery_timeout"`
	HalfOpenMaxCalls         int32         `json:"half_open_max_calls"`
	HalfOpenSuccessThreshold int32         `json:"half_open_success_threshold"`
}

// DefaultMigrationConfig returns safe defaults for migration
func DefaultMigrationConfig() *MigrationConfig {
	return &MigrationConfig{
		EnableNewImplementation: false, // Start with old implementation
		GradualMigrationEnabled: true,  // Enable gradual migration
		MigrationPercentage:     0,     // Start with 0% to new implementation

		// Gradual migration defaults
		EnableGradualMigration:     true,
		InitialMigrationPercentage: 5,               // Start with 5%
		MaxMigrationPercentage:     100,             // Allow full migration
		StepSize:                   10,              // 10% increments
		StepInterval:               time.Minute * 5, // 5 minutes between steps

		EnableFallback:         true, // Enable fallback for safety
		FallbackTimeout:        50 * time.Millisecond,
		MaxConsecutiveFailures: 5,
		LatencyThresholdMs:     10,   // 10ms latency threshold
		ThroughputThresholdOps: 1000, // 1000 ops/sec threshold
		ConflictThreshold:      10,   // 10% conflict threshold

		// Default circuit breaker config
		CircuitBreakerConfig: &CircuitBreakerConfig{
			FailureThreshold:         5,
			RecoveryTimeout:          time.Second * 30,
			HalfOpenMaxCalls:         3,
			HalfOpenSuccessThreshold: 2,
		},

		CollectMetrics:     true,
		LogMigrationEvents: true,
	}
}

// AdaptiveOrderBook provides migration and fallback between implementations
type AdaptiveOrderBook struct {
	pair   string
	config *MigrationConfig

	// Implementations
	oldImplementation *OrderBook
	newImplementation *DeadlockSafeOrderBook

	// State tracking
	migrationPercentage int32 // Current migration percentage (atomic)
	consecutiveFailures int32 // Consecutive failures counter (atomic)
	totalRequests       int64 // Total requests processed (atomic)
	newImplRequests     int64 // Requests sent to new implementation (atomic)
	failedRequests      int64 // Failed requests counter (atomic)
	lastMigrationUpdate int64 // Last migration update timestamp (atomic)

	// Performance monitoring
	performanceMonitor *PerformanceMonitor
	snapshotOptimizer  *SnapshotOptimizer

	// Synchronization
	migrationMu sync.RWMutex // Protects migration state changes

	// Circuit breaker for gradual migration
	circuitBreakerOpen int32 // Circuit breaker state (atomic)
	lastFailureTime    int64 // Last failure timestamp (atomic)
}

// NewAdaptiveOrderBook creates a new adaptive order book with migration support
func NewAdaptiveOrderBook(pair string, config *MigrationConfig) *AdaptiveOrderBook {
	if config == nil {
		config = DefaultMigrationConfig()
	}

	aob := &AdaptiveOrderBook{
		pair:                pair,
		config:              config,
		oldImplementation:   NewOrderBook(pair),
		newImplementation:   NewDeadlockSafeOrderBook(pair),
		migrationPercentage: int32(config.MigrationPercentage),
	}
	// Initialize monitoring if enabled
	if config.CollectMetrics {
		aob.performanceMonitor = NewPerformanceMonitor()
		aob.snapshotOptimizer = NewSnapshotOptimizer(100) // maxDepth of 100
	}

	return aob
}

// shouldUseNewImplementation determines which implementation to use
func (aob *AdaptiveOrderBook) shouldUseNewImplementation() bool {
	// Check if new implementation is enabled
	if !aob.config.EnableNewImplementation {
		return false
	}

	// Check circuit breaker
	if atomic.LoadInt32(&aob.circuitBreakerOpen) == 1 {
		// Check if circuit breaker should be reset
		lastFailure := atomic.LoadInt64(&aob.lastFailureTime)
		if time.Now().UnixNano()-lastFailure > int64(aob.config.FallbackTimeout) {
			atomic.StoreInt32(&aob.circuitBreakerOpen, 0)
			atomic.StoreInt32(&aob.consecutiveFailures, 0)
		} else {
			return false
		}
	}

	// Gradual migration based on percentage
	if aob.config.GradualMigrationEnabled {
		percentage := atomic.LoadInt32(&aob.migrationPercentage)
		if percentage >= 100 {
			return true
		}
		if percentage <= 0 {
			return false
		}

		// Use request count to determine routing
		requestCount := atomic.AddInt64(&aob.totalRequests, 1)
		return (requestCount % 100) < int64(percentage)
	}

	return true
}

// recordFailure records a failure and potentially opens circuit breaker
func (aob *AdaptiveOrderBook) recordFailure() {
	atomic.AddInt64(&aob.failedRequests, 1)
	failures := atomic.AddInt32(&aob.consecutiveFailures, 1)
	atomic.StoreInt64(&aob.lastFailureTime, time.Now().UnixNano())

	// Open circuit breaker if too many consecutive failures
	if failures >= aob.config.MaxConsecutiveFailures {
		atomic.StoreInt32(&aob.circuitBreakerOpen, 1)

		// Reduce migration percentage
		if aob.config.GradualMigrationEnabled {
			current := atomic.LoadInt32(&aob.migrationPercentage)
			newPercentage := current - 10 // Reduce by 10%
			if newPercentage < 0 {
				newPercentage = 0
			}
			atomic.StoreInt32(&aob.migrationPercentage, newPercentage)
		}
	}
}

// recordSuccess records a successful operation
func (aob *AdaptiveOrderBook) recordSuccess() {
	atomic.StoreInt32(&aob.consecutiveFailures, 0)
}

// AddOrder implements the adaptive routing for AddOrder operations
func (aob *AdaptiveOrderBook) AddOrder(order *model.Order) (*AddOrderResult, error) {
	startTime := time.Now()
	useNew := aob.shouldUseNewImplementation()

	if useNew {
		atomic.AddInt64(&aob.newImplRequests, 1)

		// Try new implementation first
		result, err := aob.addOrderNew(order)
		if err != nil && aob.config.EnableFallback {
			// Record failure and fallback to old implementation
			aob.recordFailure()

			// Fallback to old implementation
			result, err = aob.addOrderOld(order)
			if err == nil {
				aob.recordSuccess()
			}
		} else if err == nil {
			aob.recordSuccess()
		}
		// Record performance metrics
		if aob.performanceMonitor != nil {
			if useNew {
				aob.performanceMonitor.RecordAddOrder(startTime, err == nil)
			} else {
				aob.performanceMonitor.RecordAddOrder(startTime, err == nil)
			}
		}

		return result, err
	} else {
		// Use old implementation
		result, err := aob.addOrderOld(order)
		// Record performance metrics
		if aob.performanceMonitor != nil {
			aob.performanceMonitor.RecordAddOrder(startTime, err == nil)
		}

		return result, err
	}
}

// addOrderNew handles AddOrder using the new implementation
func (aob *AdaptiveOrderBook) addOrderNew(order *model.Order) (*AddOrderResult, error) {
	safeResult, err := aob.newImplementation.AddOrder(order)
	if err != nil {
		return nil, err
	}

	// Convert SafeAddOrderResult to AddOrderResult for compatibility
	return &AddOrderResult{
		Trades:        safeResult.Trades,
		RestingOrders: safeResult.RestingOrders,
	}, nil
}

// addOrderOld handles AddOrder using the old implementation
func (aob *AdaptiveOrderBook) addOrderOld(order *model.Order) (*AddOrderResult, error) {
	return aob.oldImplementation.AddOrder(order)
}

// CancelOrder implements the adaptive routing for CancelOrder operations
func (aob *AdaptiveOrderBook) CancelOrder(orderID uuid.UUID) error {
	startTime := time.Now()
	useNew := aob.shouldUseNewImplementation()

	var err error
	if useNew {
		// Try new implementation first
		err = aob.newImplementation.CancelOrder(orderID)
		if err != nil && aob.config.EnableFallback {
			// Record failure and fallback to old implementation
			aob.recordFailure()
			err = aob.oldImplementation.CancelOrder(orderID)
			if err == nil {
				aob.recordSuccess()
			}
		} else if err == nil {
			aob.recordSuccess()
		}
	} else {
		// Use old implementation
		err = aob.oldImplementation.CancelOrder(orderID)
	}
	// Record performance metrics
	if aob.performanceMonitor != nil {
		aob.performanceMonitor.RecordCancelOrder(startTime, err == nil)
	}

	return err
}

// GetSnapshot implements adaptive snapshot retrieval with optimization
func (aob *AdaptiveOrderBook) GetSnapshot(depth int) ([][]string, [][]string) { // Use snapshot optimizer if available
	if aob.snapshotOptimizer != nil {
		bids, asks, found := aob.snapshotOptimizer.GetOptimizedSnapshot(depth)
		if found {
			return bids, asks
		}
	}

	// Fallback to direct implementation
	if aob.shouldUseNewImplementation() {
		return aob.newImplementation.GetSnapshot(depth)
	}
	return aob.oldImplementation.GetSnapshot(depth)
}

// Administrative control methods - route to active implementation
func (aob *AdaptiveOrderBook) HaltTrading() {
	aob.oldImplementation.HaltTrading()
	aob.newImplementation.SetTradingHalted(true)
}

func (aob *AdaptiveOrderBook) ResumeTrading() {
	aob.oldImplementation.ResumeTrading()
	aob.newImplementation.SetTradingHalted(false)
}

func (aob *AdaptiveOrderBook) TriggerCircuitBreaker() {
	aob.oldImplementation.TriggerCircuitBreaker()
	aob.newImplementation.SetCircuitBreakerActive(true)
}

func (aob *AdaptiveOrderBook) EmergencyShutdown() {
	aob.oldImplementation.EmergencyShutdown()
	aob.newImplementation.SetEmergencyStop(true)
}

// Compatibility methods for engine integration
func (aob *AdaptiveOrderBook) OrdersCount() int {
	if aob.shouldUseNewImplementation() {
		return aob.newImplementation.OrdersCount()
	}
	return aob.oldImplementation.OrdersCount()
}

func (aob *AdaptiveOrderBook) GetOrderIDs() []uuid.UUID {
	if aob.shouldUseNewImplementation() {
		return aob.newImplementation.GetOrderIDs()
	}
	return aob.oldImplementation.GetOrderIDs()
}

func (aob *AdaptiveOrderBook) ProcessLimitOrder(ctx context.Context, order *model.Order) (*model.Order, []*model.Trade, []*model.Order, error) {
	if aob.shouldUseNewImplementation() {
		// New implementation uses AddOrder for all order types
		result, err := aob.addOrderNew(order)
		if err != nil {
			return order, nil, nil, err
		}
		return order, result.Trades, result.RestingOrders, nil
	}
	return aob.oldImplementation.ProcessLimitOrder(ctx, order)
}

func (aob *AdaptiveOrderBook) ProcessMarketOrder(ctx context.Context, order *model.Order) (*model.Order, []*model.Trade, []*model.Order, error) {
	if aob.shouldUseNewImplementation() {
		// New implementation uses AddOrder for all order types
		result, err := aob.addOrderNew(order)
		if err != nil {
			return order, nil, nil, err
		}
		return order, result.Trades, nil, nil // Market orders don't rest
	}
	return aob.oldImplementation.ProcessMarketOrder(ctx, order)
}

func (aob *AdaptiveOrderBook) CanFullyFill(order *model.Order) bool {
	if aob.shouldUseNewImplementation() {
		return aob.newImplementation.CanFullyFill(order)
	}
	return aob.oldImplementation.CanFullyFill(order)
}

func (aob *AdaptiveOrderBook) GetMutex() *sync.RWMutex {
	// Return old implementation mutex for engine compatibility
	return aob.oldImplementation.GetMutex()
}

// Migration control methods
func (aob *AdaptiveOrderBook) SetMigrationPercentage(percentage int32) {
	if percentage < 0 {
		percentage = 0
	}
	if percentage > 100 {
		percentage = 100
	}
	atomic.StoreInt32(&aob.migrationPercentage, percentage)
	atomic.StoreInt64(&aob.lastMigrationUpdate, time.Now().UnixNano())
}

func (aob *AdaptiveOrderBook) GetMigrationPercentage() int32 {
	return atomic.LoadInt32(&aob.migrationPercentage)
}

func (aob *AdaptiveOrderBook) GetMigrationStats() MigrationStats {
	totalRequests := atomic.LoadInt64(&aob.totalRequests)
	newImplRequests := atomic.LoadInt64(&aob.newImplRequests)
	oldImplRequests := totalRequests - newImplRequests

	return MigrationStats{
		TotalRequests:       totalRequests,
		NewImplRequests:     newImplRequests,
		OldImplRequests:     oldImplRequests,
		FailedRequests:      atomic.LoadInt64(&aob.failedRequests),
		ConsecutiveFailures: atomic.LoadInt32(&aob.consecutiveFailures),
		MigrationPercentage: atomic.LoadInt32(&aob.migrationPercentage),
		CircuitBreakerOpen:  atomic.LoadInt32(&aob.circuitBreakerOpen) == 1,
		LastMigrationUpdate: atomic.LoadInt64(&aob.lastMigrationUpdate),
	}
}

func (aob *AdaptiveOrderBook) EnableNewImplementation(enable bool) {
	aob.migrationMu.Lock()
	defer aob.migrationMu.Unlock()
	aob.config.EnableNewImplementation = enable
}

func (aob *AdaptiveOrderBook) ResetCircuitBreaker() {
	atomic.StoreInt32(&aob.circuitBreakerOpen, 0)
	atomic.StoreInt32(&aob.consecutiveFailures, 0)
}

// MigrationStats provides statistics about the migration process
type MigrationStats struct {
	TotalRequests       int64 `json:"total_requests"`
	NewImplRequests     int64 `json:"new_impl_requests"`
	OldImplRequests     int64 `json:"old_impl_requests"`
	FailedRequests      int64 `json:"failed_requests"`
	ConsecutiveFailures int32 `json:"consecutive_failures"`
	MigrationPercentage int32 `json:"migration_percentage"`
	CircuitBreakerOpen  bool  `json:"circuit_breaker_open"`
	LastMigrationUpdate int64 `json:"last_migration_update"`
}

// Performance comparison methods
func (aob *AdaptiveOrderBook) GetPerformanceMetrics() map[string]interface{} {
	if aob.performanceMonitor == nil {
		return nil
	}
	return aob.performanceMonitor.GetMetrics()
}

func (aob *AdaptiveOrderBook) GetSnapshotCacheMetrics() map[string]interface{} {
	if aob.snapshotOptimizer == nil {
		return nil
	}
	metrics := aob.snapshotOptimizer.GetMetrics()
	// Convert map[string]int64 to map[string]interface{}
	result := make(map[string]interface{})
	for k, v := range metrics {
		result[k] = v
	}
	return result
}
