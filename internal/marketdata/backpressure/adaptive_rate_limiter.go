// Package backpressure provides adaptive rate limiting with client-specific throttling
// ensuring zero trade loss while optimizing market data delivery
package backpressure

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"go.uber.org/zap"
)

// AdaptiveRateLimiter provides dynamic per-client rate limiting based on
// real-time capability detection and system load
type AdaptiveRateLimiter struct {
	logger             *zap.Logger
	capabilityDetector *ClientCapabilityDetector

	// Lock-free rate limiter registry
	limiters       unsafe.Pointer // *map[string]*ClientRateLimiter
	limiterVersion int64          // Version for lock-free updates

	// Global system state
	systemLoad        int64 // 0-100 percentage
	backpressureLevel int64 // 0-5 backpressure severity

	// Configuration
	config *RateLimiterConfig

	// Workers and lifecycle
	workers      sync.WaitGroup
	shutdown     chan struct{}
	shutdownOnce sync.Once

	// Metrics
	metrics *RateLimiterMetrics
}

// ClientRateLimiter manages rate limiting for individual clients
type ClientRateLimiter struct {
	ClientID string

	// Current rate limits (messages/second)
	currentLimit int64 // Current active limit
	safeLimit    int64 // Conservative safe limit
	maxLimit     int64 // Maximum allowed limit
	burstLimit   int64 // Short-term burst limit

	// Token bucket implementation (lock-free)
	tokens        int64 // Current token count
	lastRefill    int64 // Last refill timestamp (unix nanos)
	tokenCapacity int64 // Maximum tokens

	// Burst handling
	burstTokens   int64 // Available burst tokens
	burstWindow   int64 // Burst window start time
	burstDuration int64 // Burst duration in nanoseconds

	// Rate adaptation state
	adaptationState int64 // 0=stable, 1=increasing, 2=decreasing
	lastAdaptation  int64 // Last adaptation timestamp

	// Performance tracking
	successCount int64 // Successful requests
	dropCount    int64 // Dropped requests
	errorCount   int64 // Error requests

	// Message priority handling
	priorityTokens [5]int64 // Token buckets per priority level

	// Synchronization
	mu sync.RWMutex
	_  [6]int64 // Cache line padding
}

// RateLimiterConfig configures the adaptive rate limiter
type RateLimiterConfig struct {
	// Global limits
	MaxGlobalRate   int64   // Maximum system-wide rate
	BaseRefillRate  int64   // Base token refill rate
	BurstMultiplier float64 // Burst capacity multiplier

	// Adaptation parameters
	AdaptationInterval time.Duration // How often to adapt rates
	IncreaseThreshold  float64       // Success rate to increase limit
	DecreaseThreshold  float64       // Error rate to decrease limit
	AdaptationStep     float64       // Percentage change per adaptation

	// Backpressure thresholds
	LightBackpressure  float64 // System load % for light backpressure
	MediumBackpressure float64 // System load % for medium backpressure
	HeavyBackpressure  float64 // System load % for heavy backpressure

	// Priority weights
	PriorityWeights [5]float64 // Weight for each priority level
}

// RateLimiterMetrics tracks rate limiting performance
type RateLimiterMetrics struct {
	// Global throughput
	TotalRequests   int64
	AllowedRequests int64
	DroppedRequests int64
	BurstRequests   int64

	// Per-priority metrics
	CriticalAllowed int64
	HighAllowed     int64
	MediumAllowed   int64
	LowAllowed      int64
	MarketAllowed   int64

	CriticalDropped int64
	HighDropped     int64
	MediumDropped   int64
	LowDropped      int64
	MarketDropped   int64

	// System health
	ActiveLimiters     int64
	AdaptationEvents   int64
	BackpressureEvents int64

	// Performance distribution
	UnderUtilizedClients int64 // Using <50% of limit
	WellUtilizedClients  int64 // Using 50-80% of limit
	OverUtilizedClients  int64 // Using >80% of limit

	_ [4]int64 // Cache line padding
}

// DefaultRateLimiterConfig returns sensible defaults
func DefaultRateLimiterConfig() *RateLimiterConfig {
	return &RateLimiterConfig{
		MaxGlobalRate:   1000000, // 1M messages/sec system-wide
		BaseRefillRate:  1000,    // 1000 tokens/sec base rate
		BurstMultiplier: 2.0,     // 2x burst capacity

		AdaptationInterval: 5 * time.Second,
		IncreaseThreshold:  0.95, // 95% success rate to increase
		DecreaseThreshold:  0.05, // 5% error rate to decrease
		AdaptationStep:     0.1,  // 10% change per step

		LightBackpressure:  70.0, // 70% system load
		MediumBackpressure: 85.0, // 85% system load
		HeavyBackpressure:  95.0, // 95% system load

		PriorityWeights: [5]float64{
			10.0, // Critical - 10x weight
			5.0,  // High - 5x weight
			1.0,  // Medium - 1x weight
			0.5,  // Low - 0.5x weight
			0.1,  // Market - 0.1x weight
		},
	}
}

// NewAdaptiveRateLimiter creates a new adaptive rate limiter
func NewAdaptiveRateLimiter(
	logger *zap.Logger,
	capabilityDetector *ClientCapabilityDetector,
	config *RateLimiterConfig,
) *AdaptiveRateLimiter {
	if config == nil {
		config = DefaultRateLimiterConfig()
	}

	limiter := &AdaptiveRateLimiter{
		logger:             logger,
		capabilityDetector: capabilityDetector,
		config:             config,
		shutdown:           make(chan struct{}),
		metrics:            &RateLimiterMetrics{},
	}

	// Initialize empty limiters map
	emptyMap := make(map[string]*ClientRateLimiter)
	atomic.StorePointer(&limiter.limiters, unsafe.Pointer(&emptyMap))
	atomic.StoreInt64(&limiter.limiterVersion, 1)

	return limiter
}

// Start begins adaptive rate limiting
func (arl *AdaptiveRateLimiter) Start(ctx context.Context) error {
	arl.logger.Info("Starting adaptive rate limiter")

	// Start worker goroutines
	arl.workers.Add(3)
	go arl.systemMonitor(ctx)    // Monitor system load
	go arl.adaptationEngine(ctx) // Adapt rate limits
	go arl.metricsCollector(ctx) // Collect metrics

	return nil
}

// Allow checks if a request should be allowed for a client
func (arl *AdaptiveRateLimiter) Allow(clientID string, priority Priority) bool {
	atomic.AddInt64(&arl.metrics.TotalRequests, 1)

	// Get or create rate limiter for client
	clientLimiter := arl.getOrCreateClientLimiter(clientID)

	// Check rate limit using token bucket algorithm
	allowed := arl.checkRateLimit(clientLimiter, priority)

	if allowed {
		atomic.AddInt64(&arl.metrics.AllowedRequests, 1)
		atomic.AddInt64(&clientLimiter.successCount, 1)
		arl.updatePriorityMetrics(priority, true)
	} else {
		atomic.AddInt64(&arl.metrics.DroppedRequests, 1)
		atomic.AddInt64(&clientLimiter.dropCount, 1)
		arl.updatePriorityMetrics(priority, false)
	}

	return allowed
}

// checkRateLimit implements token bucket algorithm with priority support
func (arl *AdaptiveRateLimiter) checkRateLimit(limiter *ClientRateLimiter, priority Priority) bool {
	now := time.Now().UnixNano()

	// Apply backpressure based on system load
	backpressure := atomic.LoadInt64(&arl.backpressureLevel)
	if backpressure > 0 && priority < PriorityHigh {
		// Under backpressure, only allow high+ priority messages
		return false
	}

	// Refill tokens based on current rate limit
	arl.refillTokens(limiter, now)

	// Check priority-specific tokens first
	priorityIndex := int(priority)
	if priorityTokens := atomic.LoadInt64(&limiter.priorityTokens[priorityIndex]); priorityTokens > 0 {
		if atomic.CompareAndSwapInt64(&limiter.priorityTokens[priorityIndex], priorityTokens, priorityTokens-1) {
			return true
		}
	}

	// Fall back to general token bucket
	if tokens := atomic.LoadInt64(&limiter.tokens); tokens > 0 {
		if atomic.CompareAndSwapInt64(&limiter.tokens, tokens, tokens-1) {
			return true
		}
	}

	// Check burst tokens for high priority messages
	if priority >= PriorityHigh {
		if burstTokens := atomic.LoadInt64(&limiter.burstTokens); burstTokens > 0 {
			if atomic.CompareAndSwapInt64(&limiter.burstTokens, burstTokens, burstTokens-1) {
				atomic.AddInt64(&arl.metrics.BurstRequests, 1)
				return true
			}
		}
	}

	return false
}

// refillTokens refills token buckets based on elapsed time and current limits
func (arl *AdaptiveRateLimiter) refillTokens(limiter *ClientRateLimiter, now int64) {
	lastRefill := atomic.LoadInt64(&limiter.lastRefill)
	if now <= lastRefill {
		return // Clock skew or no time elapsed
	}

	// Calculate time elapsed
	elapsed := time.Duration(now - lastRefill)

	// Calculate tokens to add based on current rate limit
	currentLimit := atomic.LoadInt64(&limiter.currentLimit)
	tokensToAdd := int64(float64(currentLimit) * elapsed.Seconds())

	if tokensToAdd > 0 {
		// Update general token bucket
		capacity := atomic.LoadInt64(&limiter.tokenCapacity)
		for {
			currentTokens := atomic.LoadInt64(&limiter.tokens)
			newTokens := currentTokens + tokensToAdd
			if newTokens > capacity {
				newTokens = capacity
			}

			if atomic.CompareAndSwapInt64(&limiter.tokens, currentTokens, newTokens) {
				break
			}
		}

		// Refill priority-specific token buckets
		arl.refillPriorityTokens(limiter, tokensToAdd, now)

		// Update last refill time
		atomic.StoreInt64(&limiter.lastRefill, now)
	}
}

// refillPriorityTokens refills priority-specific token buckets
func (arl *AdaptiveRateLimiter) refillPriorityTokens(limiter *ClientRateLimiter, baseTokens int64, now int64) {
	for i := 0; i < 5; i++ {
		priorityWeight := arl.config.PriorityWeights[i]
		priorityTokens := int64(float64(baseTokens) * priorityWeight)

		if priorityTokens > 0 {
			// Add tokens up to a reasonable limit per priority
			maxPriorityTokens := atomic.LoadInt64(&limiter.tokenCapacity) / 5

			for {
				currentTokens := atomic.LoadInt64(&limiter.priorityTokens[i])
				newTokens := currentTokens + priorityTokens
				if newTokens > maxPriorityTokens {
					newTokens = maxPriorityTokens
				}

				if atomic.CompareAndSwapInt64(&limiter.priorityTokens[i], currentTokens, newTokens) {
					break
				}
			}
		}
	}

	// Refill burst tokens if needed
	arl.refillBurstTokens(limiter, baseTokens, now)
}

// refillBurstTokens refills burst token bucket
func (arl *AdaptiveRateLimiter) refillBurstTokens(limiter *ClientRateLimiter, baseTokens int64, now int64) {
	burstWindow := atomic.LoadInt64(&limiter.burstWindow)
	burstDuration := atomic.LoadInt64(&limiter.burstDuration)

	// Reset burst window if expired
	if now-burstWindow > burstDuration {
		atomic.StoreInt64(&limiter.burstWindow, now)
		burstCapacity := int64(float64(atomic.LoadInt64(&limiter.tokenCapacity)) * arl.config.BurstMultiplier)
		atomic.StoreInt64(&limiter.burstTokens, burstCapacity)
	}
}

// getOrCreateClientLimiter gets existing or creates new client rate limiter
func (arl *AdaptiveRateLimiter) getOrCreateClientLimiter(clientID string) *ClientRateLimiter {
	// First try to get existing limiter
	currentMapPtr := atomic.LoadPointer(&arl.limiters)
	currentMap := *(*map[string]*ClientRateLimiter)(currentMapPtr)

	if limiter, exists := currentMap[clientID]; exists {
		return limiter
	}

	// Create new limiter with capability-based defaults
	capability := arl.capabilityDetector.GetClientCapability(clientID)

	var safeRate, maxRate, burstRate int64 = 10, 100, 200 // Defaults
	if capability != nil {
		safeRate = capability.SafeRate
		maxRate = capability.MaxRate
		burstRate = capability.BurstRate
	}

	newLimiter := &ClientRateLimiter{
		ClientID:      clientID,
		currentLimit:  safeRate,
		safeLimit:     safeRate,
		maxLimit:      maxRate,
		burstLimit:    burstRate,
		tokenCapacity: safeRate * 2, // 2 second capacity
		tokens:        safeRate,     // Start with full bucket
		lastRefill:    time.Now().UnixNano(),
		burstTokens:   int64(float64(safeRate) * arl.config.BurstMultiplier),
		burstWindow:   time.Now().UnixNano(),
		burstDuration: int64(10 * time.Second), // 10 second burst window
	}

	// Initialize priority token buckets
	for i := 0; i < 5; i++ {
		newLimiter.priorityTokens[i] = safeRate / 5 // Distribute initial tokens
	}

	// Add to map atomically
	for {
		currentMapPtr := atomic.LoadPointer(&arl.limiters)
		currentMap := *(*map[string]*ClientRateLimiter)(currentMapPtr)

		// Double-check if limiter was created by another goroutine
		if limiter, exists := currentMap[clientID]; exists {
			return limiter
		}

		// Create new map with the limiter added
		newMap := make(map[string]*ClientRateLimiter, len(currentMap)+1)
		for k, v := range currentMap {
			newMap[k] = v
		}
		newMap[clientID] = newLimiter

		// Atomic update
		if atomic.CompareAndSwapPointer(&arl.limiters, currentMapPtr, unsafe.Pointer(&newMap)) {
			atomic.AddInt64(&arl.limiterVersion, 1)
			atomic.AddInt64(&arl.metrics.ActiveLimiters, 1)

			arl.logger.Debug("Created new rate limiter for client",
				zap.String("client_id", clientID),
				zap.Int64("safe_rate", safeRate),
				zap.Int64("max_rate", maxRate))

			return newLimiter
		}
		// Retry on conflict
	}
}

// updatePriorityMetrics updates per-priority metrics
func (arl *AdaptiveRateLimiter) updatePriorityMetrics(priority Priority, allowed bool) {
	if allowed {
		switch priority {
		case PriorityCritical, PriorityUltra:
			atomic.AddInt64(&arl.metrics.CriticalAllowed, 1)
		case PriorityHigh:
			atomic.AddInt64(&arl.metrics.HighAllowed, 1)
		case PriorityNormal:
			atomic.AddInt64(&arl.metrics.MediumAllowed, 1)
		case PriorityLow:
			atomic.AddInt64(&arl.metrics.LowAllowed, 1)
		default:
			atomic.AddInt64(&arl.metrics.MarketAllowed, 1)
		}
	} else {
		switch priority {
		case PriorityCritical, PriorityUltra:
			atomic.AddInt64(&arl.metrics.CriticalDropped, 1)
		case PriorityHigh:
			atomic.AddInt64(&arl.metrics.HighDropped, 1)
		case PriorityNormal:
			atomic.AddInt64(&arl.metrics.MediumDropped, 1)
		case PriorityLow:
			atomic.AddInt64(&arl.metrics.LowDropped, 1)
		default:
			atomic.AddInt64(&arl.metrics.MarketDropped, 1)
		}
	}
}

// systemMonitor monitors system load and adjusts backpressure
func (arl *AdaptiveRateLimiter) systemMonitor(ctx context.Context) {
	defer arl.workers.Done()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	arl.logger.Info("System monitor started")

	for {
		select {
		case <-ticker.C:
			arl.updateSystemLoad()
			arl.updateBackpressureLevel()

		case <-arl.shutdown:
			arl.logger.Info("System monitor shutting down")
			return

		case <-ctx.Done():
			return
		}
	}
}

// adaptationEngine adapts client rate limits based on performance
func (arl *AdaptiveRateLimiter) adaptationEngine(ctx context.Context) {
	defer arl.workers.Done()

	ticker := time.NewTicker(arl.config.AdaptationInterval)
	defer ticker.Stop()

	arl.logger.Info("Adaptation engine started")

	for {
		select {
		case <-ticker.C:
			arl.adaptClientLimits()

		case <-arl.shutdown:
			arl.logger.Info("Adaptation engine shutting down")
			return

		case <-ctx.Done():
			return
		}
	}
}

// metricsCollector collects and updates metrics
func (arl *AdaptiveRateLimiter) metricsCollector(ctx context.Context) {
	defer arl.workers.Done()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	arl.logger.Info("Metrics collector started")

	for {
		select {
		case <-ticker.C:
			arl.updateUtilizationMetrics()

		case <-arl.shutdown:
			arl.logger.Info("Metrics collector shutting down")
			return

		case <-ctx.Done():
			return
		}
	}
}

// updateSystemLoad estimates current system load
func (arl *AdaptiveRateLimiter) updateSystemLoad() {
	// Simple load estimation based on request rate vs capacity
	totalRequests := atomic.LoadInt64(&arl.metrics.TotalRequests)
	droppedRequests := atomic.LoadInt64(&arl.metrics.DroppedRequests)

	if totalRequests > 0 {
		dropRate := float64(droppedRequests) / float64(totalRequests)
		loadPercentage := int64(dropRate * 100)

		if loadPercentage > 100 {
			loadPercentage = 100
		}

		atomic.StoreInt64(&arl.systemLoad, loadPercentage)
	}
}

// updateBackpressureLevel updates backpressure level based on system load
func (arl *AdaptiveRateLimiter) updateBackpressureLevel() {
	load := float64(atomic.LoadInt64(&arl.systemLoad))

	var newLevel int64
	if load >= arl.config.HeavyBackpressure {
		newLevel = 3 // Heavy backpressure
	} else if load >= arl.config.MediumBackpressure {
		newLevel = 2 // Medium backpressure
	} else if load >= arl.config.LightBackpressure {
		newLevel = 1 // Light backpressure
	} else {
		newLevel = 0 // No backpressure
	}

	oldLevel := atomic.SwapInt64(&arl.backpressureLevel, newLevel)
	if newLevel != oldLevel {
		atomic.AddInt64(&arl.metrics.BackpressureEvents, 1)

		arl.logger.Info("Backpressure level changed",
			zap.Int64("old_level", oldLevel),
			zap.Int64("new_level", newLevel),
			zap.Float64("system_load", load))
	}
}

// adaptClientLimits adapts rate limits for all clients
func (arl *AdaptiveRateLimiter) adaptClientLimits() {
	currentMapPtr := atomic.LoadPointer(&arl.limiters)
	currentMap := *(*map[string]*ClientRateLimiter)(currentMapPtr)

	for clientID, limiter := range currentMap {
		arl.adaptClientLimit(clientID, limiter)
	}
}

// adaptClientLimit adapts rate limit for a single client
func (arl *AdaptiveRateLimiter) adaptClientLimit(clientID string, limiter *ClientRateLimiter) {
	successCount := atomic.SwapInt64(&limiter.successCount, 0)
	dropCount := atomic.SwapInt64(&limiter.dropCount, 0)
	errorCount := atomic.SwapInt64(&limiter.errorCount, 0)

	totalRequests := successCount + dropCount + errorCount
	if totalRequests == 0 {
		return // No activity to adapt on
	}

	successRate := float64(successCount) / float64(totalRequests)
	errorRate := float64(errorCount) / float64(totalRequests)

	currentLimit := atomic.LoadInt64(&limiter.currentLimit)
	safeLimit := atomic.LoadInt64(&limiter.safeLimit)
	maxLimit := atomic.LoadInt64(&limiter.maxLimit)

	var newLimit int64 = currentLimit

	// Increase limit if success rate is high and we're below max
	if successRate >= arl.config.IncreaseThreshold && currentLimit < maxLimit {
		increase := int64(float64(currentLimit) * arl.config.AdaptationStep)
		newLimit = currentLimit + increase
		if newLimit > maxLimit {
			newLimit = maxLimit
		}

		atomic.StoreInt64(&limiter.adaptationState, 1) // Increasing

	} else if errorRate >= arl.config.DecreaseThreshold && currentLimit > safeLimit {
		// Decrease limit if error rate is high and we're above safe limit
		decrease := int64(float64(currentLimit) * arl.config.AdaptationStep)
		newLimit = currentLimit - decrease
		if newLimit < safeLimit {
			newLimit = safeLimit
		}

		atomic.StoreInt64(&limiter.adaptationState, 2) // Decreasing
	} else {
		atomic.StoreInt64(&limiter.adaptationState, 0) // Stable
	}

	if newLimit != currentLimit {
		atomic.StoreInt64(&limiter.currentLimit, newLimit)
		atomic.StoreInt64(&limiter.tokenCapacity, newLimit*2) // Update capacity
		atomic.StoreInt64(&limiter.lastAdaptation, time.Now().UnixNano())
		atomic.AddInt64(&arl.metrics.AdaptationEvents, 1)

		arl.logger.Debug("Adapted client rate limit",
			zap.String("client_id", clientID),
			zap.Int64("old_limit", currentLimit),
			zap.Int64("new_limit", newLimit),
			zap.Float64("success_rate", successRate),
			zap.Float64("error_rate", errorRate))
	}
}

// updateUtilizationMetrics updates client utilization distribution
func (arl *AdaptiveRateLimiter) updateUtilizationMetrics() {
	currentMapPtr := atomic.LoadPointer(&arl.limiters)
	currentMap := *(*map[string]*ClientRateLimiter)(currentMapPtr)

	var underUtilized, wellUtilized, overUtilized int64

	for _, limiter := range currentMap {
		tokens := atomic.LoadInt64(&limiter.tokens)
		capacity := atomic.LoadInt64(&limiter.tokenCapacity)

		if capacity > 0 {
			utilization := float64(capacity-tokens) / float64(capacity)

			if utilization < 0.5 {
				underUtilized++
			} else if utilization < 0.8 {
				wellUtilized++
			} else {
				overUtilized++
			}
		}
	}

	atomic.StoreInt64(&arl.metrics.UnderUtilizedClients, underUtilized)
	atomic.StoreInt64(&arl.metrics.WellUtilizedClients, wellUtilized)
	atomic.StoreInt64(&arl.metrics.OverUtilizedClients, overUtilized)
}

// GetMetrics returns current rate limiter metrics
func (arl *AdaptiveRateLimiter) GetMetrics() RateLimiterMetrics {
	return RateLimiterMetrics{
		TotalRequests:   atomic.LoadInt64(&arl.metrics.TotalRequests),
		AllowedRequests: atomic.LoadInt64(&arl.metrics.AllowedRequests),
		DroppedRequests: atomic.LoadInt64(&arl.metrics.DroppedRequests),
		BurstRequests:   atomic.LoadInt64(&arl.metrics.BurstRequests),

		CriticalAllowed: atomic.LoadInt64(&arl.metrics.CriticalAllowed),
		HighAllowed:     atomic.LoadInt64(&arl.metrics.HighAllowed),
		MediumAllowed:   atomic.LoadInt64(&arl.metrics.MediumAllowed),
		LowAllowed:      atomic.LoadInt64(&arl.metrics.LowAllowed),
		MarketAllowed:   atomic.LoadInt64(&arl.metrics.MarketAllowed),

		CriticalDropped: atomic.LoadInt64(&arl.metrics.CriticalDropped),
		HighDropped:     atomic.LoadInt64(&arl.metrics.HighDropped),
		MediumDropped:   atomic.LoadInt64(&arl.metrics.MediumDropped),
		LowDropped:      atomic.LoadInt64(&arl.metrics.LowDropped),
		MarketDropped:   atomic.LoadInt64(&arl.metrics.MarketDropped),

		ActiveLimiters:     atomic.LoadInt64(&arl.metrics.ActiveLimiters),
		AdaptationEvents:   atomic.LoadInt64(&arl.metrics.AdaptationEvents),
		BackpressureEvents: atomic.LoadInt64(&arl.metrics.BackpressureEvents),

		UnderUtilizedClients: atomic.LoadInt64(&arl.metrics.UnderUtilizedClients),
		WellUtilizedClients:  atomic.LoadInt64(&arl.metrics.WellUtilizedClients),
		OverUtilizedClients:  atomic.LoadInt64(&arl.metrics.OverUtilizedClients),
	}
}

// GetSystemLoad returns current system load percentage
func (arl *AdaptiveRateLimiter) GetSystemLoad() int64 {
	return atomic.LoadInt64(&arl.systemLoad)
}

// GetBackpressureLevel returns current backpressure level
func (arl *AdaptiveRateLimiter) GetBackpressureLevel() int64 {
	return atomic.LoadInt64(&arl.backpressureLevel)
}

// Stop gracefully shuts down the adaptive rate limiter
func (arl *AdaptiveRateLimiter) Stop(ctx context.Context) error {
	arl.shutdownOnce.Do(func() {
		arl.logger.Info("Stopping adaptive rate limiter")

		close(arl.shutdown)

		// Wait for workers to complete
		done := make(chan struct{})
		go func() {
			arl.workers.Wait()
			close(done)
		}()

		select {
		case <-done:
			arl.logger.Info("Adaptive rate limiter stopped successfully")
		case <-ctx.Done():
			arl.logger.Warn("Adaptive rate limiter shutdown timed out")
		}
	})

	return nil
}
