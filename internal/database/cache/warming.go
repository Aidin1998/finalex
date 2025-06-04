package cache

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// WarmingRequest represents a cache warming request
type WarmingRequest struct {
	Key         string                      `json:"key"`
	Loader      DataLoader                  `json:"-"`
	Priority    WarmingPriority             `json:"priority"`
	TTL         time.Duration               `json:"ttl"`
	Level       CacheLevel                  `json:"level"`
	Metadata    map[string]string           `json:"metadata,omitempty"`
	Retry       bool                        `json:"retry"`
	MaxRetries  int                         `json:"max_retries"`
	RequestedAt time.Time                   `json:"requested_at"`
	Callback    func(key string, err error) `json:"-"`
}

// DataLoader defines the interface for loading data into cache
type DataLoader interface {
	Load(ctx context.Context, key string) (interface{}, error)
}

// DataLoaderFunc is a function adapter for DataLoader
type DataLoaderFunc func(ctx context.Context, key string) (interface{}, error)

func (f DataLoaderFunc) Load(ctx context.Context, key string) (interface{}, error) {
	return f(ctx, key)
}

// WarmingPriority defines cache warming priority levels
type WarmingPriority int

const (
	PriorityCritical WarmingPriority = iota // Critical trading data (orderbooks, prices)
	PriorityHigh                            // Important user data (balances, positions)
	PriorityMedium                          // Frequently accessed data (market data)
	PriorityLow                             // Background data (historical data)
)

// WarmingStats represents cache warming statistics
type WarmingStats struct {
	TotalRequests   int64 `json:"total_requests"`
	SuccessfulWarms int64 `json:"successful_warms"`
	FailedWarms     int64 `json:"failed_warms"`
	RetryAttempts   int64 `json:"retry_attempts"`
	AverageLoadTime int64 `json:"average_load_time_ms"`
	ActiveWorkers   int32 `json:"active_workers"`
	QueueSize       int64 `json:"queue_size"`
	CriticalQueue   int64 `json:"critical_queue"`
	HighQueue       int64 `json:"high_queue"`
	MediumQueue     int64 `json:"medium_queue"`
	LowQueue        int64 `json:"low_queue"`
}

// CacheWarmer manages cache warming operations
type CacheWarmer struct {
	cacheManager *CacheManager
	logger       *zap.SugaredLogger

	// Priority queues
	criticalQueue chan WarmingRequest
	highQueue     chan WarmingRequest
	mediumQueue   chan WarmingRequest
	lowQueue      chan WarmingRequest

	// Worker management
	workerCount int
	workers     []*warmingWorker
	stopChan    chan struct{}
	wg          sync.WaitGroup

	// Statistics
	stats atomic.Value // *WarmingStats

	// Configuration
	config *WarmingConfig

	// Predictive warming
	accessPatterns map[string]*AccessPattern
	patternsMu     sync.RWMutex
}

// WarmingConfig holds cache warming configuration
type WarmingConfig struct {
	WorkerCount       int           `yaml:"worker_count" json:"worker_count"`
	CriticalQueueSize int           `yaml:"critical_queue_size" json:"critical_queue_size"`
	HighQueueSize     int           `yaml:"high_queue_size" json:"high_queue_size"`
	MediumQueueSize   int           `yaml:"medium_queue_size" json:"medium_queue_size"`
	LowQueueSize      int           `yaml:"low_queue_size" json:"low_queue_size"`
	MaxRetries        int           `yaml:"max_retries" json:"max_retries"`
	RetryDelay        time.Duration `yaml:"retry_delay" json:"retry_delay"`
	StatsInterval     time.Duration `yaml:"stats_interval" json:"stats_interval"`
	EnablePredictive  bool          `yaml:"enable_predictive" json:"enable_predictive"`
	PredictiveWindow  time.Duration `yaml:"predictive_window" json:"predictive_window"`
}

// AccessPattern tracks access patterns for predictive warming
type AccessPattern struct {
	Key            string        `json:"key"`
	AccessTimes    []time.Time   `json:"access_times"`
	Frequency      time.Duration `json:"frequency"`
	LastPrediction time.Time     `json:"last_prediction"`
	Confidence     float64       `json:"confidence"`
}

// warmingWorker handles cache warming requests
type warmingWorker struct {
	id     int
	warmer *CacheWarmer
	active int32
}

// DefaultWarmingConfig returns default cache warming configuration
func DefaultWarmingConfig() *WarmingConfig {
	return &WarmingConfig{
		WorkerCount:       8,
		CriticalQueueSize: 1000,
		HighQueueSize:     2000,
		MediumQueueSize:   5000,
		LowQueueSize:      10000,
		MaxRetries:        3,
		RetryDelay:        time.Second,
		StatsInterval:     time.Minute,
		EnablePredictive:  true,
		PredictiveWindow:  time.Hour,
	}
}

// NewCacheWarmer creates a new cache warmer instance
func NewCacheWarmer(cacheManager *CacheManager, config *WarmingConfig, logger *zap.SugaredLogger) *CacheWarmer {
	if config == nil {
		config = DefaultWarmingConfig()
	}

	warmer := &CacheWarmer{
		cacheManager:   cacheManager,
		logger:         logger,
		config:         config,
		criticalQueue:  make(chan WarmingRequest, config.CriticalQueueSize),
		highQueue:      make(chan WarmingRequest, config.HighQueueSize),
		mediumQueue:    make(chan WarmingRequest, config.MediumQueueSize),
		lowQueue:       make(chan WarmingRequest, config.LowQueueSize),
		workerCount:    config.WorkerCount,
		stopChan:       make(chan struct{}),
		accessPatterns: make(map[string]*AccessPattern),
	}

	// Initialize statistics
	warmer.stats.Store(&WarmingStats{})

	// Start workers
	warmer.startWorkers()

	// Start statistics updater
	go warmer.statsUpdater()

	// Start predictive warming if enabled
	if config.EnablePredictive {
		go warmer.predictiveWarmingLoop()
	}

	logger.Infow("Cache warmer initialized",
		"worker_count", config.WorkerCount,
		"predictive_enabled", config.EnablePredictive,
	)

	return warmer
}

// Warm adds a warming request to the appropriate queue
func (cw *CacheWarmer) Warm(request WarmingRequest) error {
	if request.RequestedAt.IsZero() {
		request.RequestedAt = time.Now()
	}

	if request.MaxRetries == 0 {
		request.MaxRetries = cw.config.MaxRetries
	}

	var queue chan WarmingRequest
	switch request.Priority {
	case PriorityCritical:
		queue = cw.criticalQueue
		atomic.AddInt64(&cw.getStats().CriticalQueue, 1)
	case PriorityHigh:
		queue = cw.highQueue
		atomic.AddInt64(&cw.getStats().HighQueue, 1)
	case PriorityMedium:
		queue = cw.mediumQueue
		atomic.AddInt64(&cw.getStats().MediumQueue, 1)
	case PriorityLow:
		queue = cw.lowQueue
		atomic.AddInt64(&cw.getStats().LowQueue, 1)
	default:
		return fmt.Errorf("invalid warming priority: %d", request.Priority)
	}

	select {
	case queue <- request:
		atomic.AddInt64(&cw.getStats().TotalRequests, 1)
		return nil
	default:
		return fmt.Errorf("warming queue full for priority %d", request.Priority)
	}
}

// WarmBatch adds multiple warming requests
func (cw *CacheWarmer) WarmBatch(requests []WarmingRequest) []error {
	errors := make([]error, len(requests))
	for i, request := range requests {
		errors[i] = cw.Warm(request)
	}
	return errors
}

// WarmCritical is a convenience method for critical priority warming
func (cw *CacheWarmer) WarmCritical(key string, loader DataLoader, ttl time.Duration) error {
	return cw.Warm(WarmingRequest{
		Key:      key,
		Loader:   loader,
		Priority: PriorityCritical,
		TTL:      ttl,
		Level:    CacheLevelL1, // Critical data goes to L1
	})
}

// WarmPredictive triggers predictive warming for a key
func (cw *CacheWarmer) WarmPredictive(key string) {
	if !cw.config.EnablePredictive {
		return
	}

	cw.patternsMu.Lock()
	defer cw.patternsMu.Unlock()

	pattern, exists := cw.accessPatterns[key]
	if !exists {
		pattern = &AccessPattern{
			Key:         key,
			AccessTimes: make([]time.Time, 0),
		}
		cw.accessPatterns[key] = pattern
	}

	now := time.Now()
	pattern.AccessTimes = append(pattern.AccessTimes, now)

	// Keep only recent access times
	cutoff := now.Add(-cw.config.PredictiveWindow)
	var recentAccess []time.Time
	for _, accessTime := range pattern.AccessTimes {
		if accessTime.After(cutoff) {
			recentAccess = append(recentAccess, accessTime)
		}
	}
	pattern.AccessTimes = recentAccess

	// Calculate frequency and confidence
	if len(pattern.AccessTimes) >= 3 {
		cw.updatePredictionMetrics(pattern)
	}
}

// GetStats returns current warming statistics
func (cw *CacheWarmer) GetStats() *WarmingStats {
	return cw.getStats()
}

// getStats returns a pointer to the current stats
func (cw *CacheWarmer) getStats() *WarmingStats {
	return cw.stats.Load().(*WarmingStats)
}

// Close shuts down the cache warmer gracefully
func (cw *CacheWarmer) Close() {
	cw.logger.Info("Shutting down cache warmer...")

	// Stop all workers
	close(cw.stopChan)
	cw.wg.Wait()

	// Close queues
	close(cw.criticalQueue)
	close(cw.highQueue)
	close(cw.mediumQueue)
	close(cw.lowQueue)

	cw.logger.Info("Cache warmer shutdown complete")
}

// startWorkers starts the warming workers
func (cw *CacheWarmer) startWorkers() {
	cw.workers = make([]*warmingWorker, cw.workerCount)

	for i := 0; i < cw.workerCount; i++ {
		worker := &warmingWorker{
			id:     i,
			warmer: cw,
		}
		cw.workers[i] = worker

		cw.wg.Add(1)
		go worker.run()
	}
}

// run is the main worker loop
func (w *warmingWorker) run() {
	defer w.warmer.wg.Done()

	w.warmer.logger.Debugw("Warming worker started", "worker_id", w.id)

	for {
		select {
		case <-w.warmer.stopChan:
			w.warmer.logger.Debugw("Warming worker stopping", "worker_id", w.id)
			return

		case request := <-w.warmer.criticalQueue:
			w.processRequest(request)
			atomic.AddInt64(&w.warmer.getStats().CriticalQueue, -1)

		case request := <-w.warmer.highQueue:
			w.processRequest(request)
			atomic.AddInt64(&w.warmer.getStats().HighQueue, -1)

		case request := <-w.warmer.mediumQueue:
			w.processRequest(request)
			atomic.AddInt64(&w.warmer.getStats().MediumQueue, -1)

		case request := <-w.warmer.lowQueue:
			w.processRequest(request)
			atomic.AddInt64(&w.warmer.getStats().LowQueue, -1)
		}
	}
}

// processRequest processes a single warming request
func (w *warmingWorker) processRequest(request WarmingRequest) {
	atomic.StoreInt32(&w.active, 1)
	defer atomic.StoreInt32(&w.active, 0)

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// Load data
	value, err := request.Loader.Load(ctx, request.Key)
	if err != nil {
		w.warmer.logger.Warnw("Failed to load data for warming",
			"key", request.Key,
			"error", err,
			"worker_id", w.id,
		)

		// Retry if configured
		if request.Retry && request.MaxRetries > 0 {
			request.MaxRetries--
			atomic.AddInt64(&w.warmer.getStats().RetryAttempts, 1)

			// Add back to queue after delay
			go func() {
				time.Sleep(w.warmer.config.RetryDelay)
				if err := w.warmer.Warm(request); err != nil {
					w.warmer.logger.Warnw("Failed to requeue warming request", "key", request.Key, "error", err)
				}
			}()
		} else {
			atomic.AddInt64(&w.warmer.getStats().FailedWarms, 1)
		}

		if request.Callback != nil {
			request.Callback(request.Key, err)
		}
		return
	}
	// Store in cache
	var cacheErr error
	switch request.Level {
	case CacheLevelL1:
		w.warmer.cacheManager.l1Cache.Set(request.Key, value, request.TTL)
	case CacheLevelL2:
		cacheErr = w.warmer.cacheManager.l2Cache.Set(ctx, request.Key, value, request.TTL)
	case CacheLevelL3:
		if request.Metadata != nil {
			cacheErr = w.warmer.cacheManager.l3Cache.SetWithMetadata(ctx, request.Key, value, request.TTL, request.Metadata)
		} else {
			cacheErr = w.warmer.cacheManager.l3Cache.Set(ctx, request.Key, value, request.TTL)
		}
	default:
		// Default: store in all levels
		cacheErr = w.warmer.cacheManager.Set(ctx, request.Key, value, request.TTL)
	}

	duration := time.Since(start)

	if cacheErr != nil {
		w.warmer.logger.Warnw("Failed to store warmed data in cache",
			"key", request.Key,
			"error", cacheErr,
			"worker_id", w.id,
		)
		atomic.AddInt64(&w.warmer.getStats().FailedWarms, 1)
	} else {
		atomic.AddInt64(&w.warmer.getStats().SuccessfulWarms, 1)
		w.warmer.logger.Debugw("Successfully warmed cache",
			"key", request.Key,
			"duration_ms", duration.Milliseconds(),
			"worker_id", w.id,
		)
	}

	// Update average load time
	w.updateAverageLoadTime(duration)

	if request.Callback != nil {
		request.Callback(request.Key, cacheErr)
	}
}

// updateAverageLoadTime updates the average load time metric
func (w *warmingWorker) updateAverageLoadTime(duration time.Duration) {
	stats := w.warmer.getStats()
	currentAvg := atomic.LoadInt64(&stats.AverageLoadTime)
	newAvg := (currentAvg + duration.Milliseconds()) / 2
	atomic.StoreInt64(&stats.AverageLoadTime, newAvg)
}

// statsUpdater periodically updates statistics
func (cw *CacheWarmer) statsUpdater() {
	ticker := time.NewTicker(cw.config.StatsInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			cw.updateWorkerStats()
		case <-cw.stopChan:
			return
		}
	}
}

// updateWorkerStats updates worker-related statistics
func (cw *CacheWarmer) updateWorkerStats() {
	activeWorkers := int32(0)
	for _, worker := range cw.workers {
		if atomic.LoadInt32(&worker.active) == 1 {
			activeWorkers++
		}
	}

	stats := cw.getStats()
	atomic.StoreInt32(&stats.ActiveWorkers, activeWorkers)

	// Update queue sizes
	atomic.StoreInt64(&stats.QueueSize,
		atomic.LoadInt64(&stats.CriticalQueue)+
			atomic.LoadInt64(&stats.HighQueue)+
			atomic.LoadInt64(&stats.MediumQueue)+
			atomic.LoadInt64(&stats.LowQueue))
}

// updatePredictionMetrics calculates prediction metrics for an access pattern
func (cw *CacheWarmer) updatePredictionMetrics(pattern *AccessPattern) {
	if len(pattern.AccessTimes) < 2 {
		return
	}

	// Calculate average interval between accesses
	var totalInterval time.Duration
	for i := 1; i < len(pattern.AccessTimes); i++ {
		interval := pattern.AccessTimes[i].Sub(pattern.AccessTimes[i-1])
		totalInterval += interval
	}

	pattern.Frequency = totalInterval / time.Duration(len(pattern.AccessTimes)-1)

	// Calculate confidence based on consistency of intervals
	var variance time.Duration
	for i := 1; i < len(pattern.AccessTimes); i++ {
		interval := pattern.AccessTimes[i].Sub(pattern.AccessTimes[i-1])
		diff := interval - pattern.Frequency
		if diff < 0 {
			diff = -diff
		}
		variance += diff
	}

	avgVariance := variance / time.Duration(len(pattern.AccessTimes)-1)
	pattern.Confidence = 1.0 - float64(avgVariance)/float64(pattern.Frequency)
	if pattern.Confidence < 0 {
		pattern.Confidence = 0
	}
}

// predictiveWarmingLoop runs the predictive warming algorithm
func (cw *CacheWarmer) predictiveWarmingLoop() {
	ticker := time.NewTicker(time.Minute * 5) // Check every 5 minutes
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			cw.runPredictiveWarming()
		case <-cw.stopChan:
			return
		}
	}
}

// runPredictiveWarming executes predictive warming based on access patterns
func (cw *CacheWarmer) runPredictiveWarming() {
	cw.patternsMu.RLock()
	defer cw.patternsMu.RUnlock()

	now := time.Now()

	for key, pattern := range cw.accessPatterns {
		// Skip if confidence is too low
		if pattern.Confidence < 0.7 {
			continue
		}

		// Skip if we predicted recently
		if now.Sub(pattern.LastPrediction) < pattern.Frequency/2 {
			continue
		}

		// Calculate next predicted access time
		lastAccess := pattern.AccessTimes[len(pattern.AccessTimes)-1]
		nextPredicted := lastAccess.Add(pattern.Frequency)

		// If next access is soon, warm the cache
		if nextPredicted.Sub(now) < time.Minute*5 {
			cw.logger.Debugw("Predictive warming triggered",
				"key", key,
				"confidence", pattern.Confidence,
				"frequency", pattern.Frequency,
				"next_predicted", nextPredicted,
			) // Create a simple loader that returns nil (indicating cache miss)
			// In a real implementation, this would connect to the actual data source
			loader := DataLoaderFunc(func(ctx context.Context, k string) (interface{}, error) {
				return nil, fmt.Errorf("predictive warming: no loader available for key %s", k)
			})

			request := WarmingRequest{
				Key:      key,
				Loader:   loader,
				Priority: PriorityMedium,
				TTL:      time.Minute * 10,
				Level:    CacheLevelL2,
			}

			if err := cw.Warm(request); err != nil {
				cw.logger.Warnw("Failed to queue predictive warming", "key", key, "error", err)
			} else {
				pattern.LastPrediction = now
			}
		}
	}
}
