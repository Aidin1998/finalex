package cache

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// InvalidationManager handles intelligent cache invalidation
type InvalidationManager struct {
	cacheManager *CacheManager
	logger       *zap.SugaredLogger

	// Event processing
	eventQueue  chan InvalidationEvent
	workerCount int
	workers     []*invalidationWorker
	stopChan    chan struct{}
	wg          sync.WaitGroup

	// Invalidation rules
	rules   map[string]*InvalidationRule
	rulesMu sync.RWMutex

	// Statistics
	stats atomic.Value // *InvalidationStats

	// Configuration
	config *InvalidationConfig

	// Bulk operations
	bulkProcessor *bulkInvalidationProcessor
}

// InvalidationRule defines how to handle invalidation for specific patterns
type InvalidationRule struct {
	Pattern   string                  `json:"pattern"`
	Type      InvalidationType        `json:"type"`
	Levels    []CacheLevel            `json:"levels"`
	Priority  InvalidationPriority    `json:"priority"`
	Propagate bool                    `json:"propagate"`
	Related   []string                `json:"related"`
	TTLExtend time.Duration           `json:"ttl_extend,omitempty"`
	Condition InvalidationCondition   `json:"condition,omitempty"`
	MaxAge    time.Duration           `json:"max_age,omitempty"`
	BatchSize int                     `json:"batch_size"`
	Callback  func(InvalidationEvent) `json:"-"`
}

// InvalidationCondition defines conditions for invalidation
type InvalidationCondition struct {
	MinAge      time.Duration `json:"min_age,omitempty"`
	MaxAccess   int64         `json:"max_access,omitempty"`
	Probability float64       `json:"probability,omitempty"`
}

// InvalidationPriority defines invalidation priority levels
type InvalidationPriority int

const (
	PriorityImmediate InvalidationPriority = iota // Real-time trading data
	PriorityUrgent                                // User-facing data
	PriorityNormal                                // Background data
	PriorityBatch                                 // Bulk operations
)

// InvalidationStats represents invalidation statistics
type InvalidationStats struct {
	TotalEvents        int64 `json:"total_events"`
	ProcessedEvents    int64 `json:"processed_events"`
	FailedEvents       int64 `json:"failed_events"`
	RuleMatches        int64 `json:"rule_matches"`
	BulkOperations     int64 `json:"bulk_operations"`
	AverageProcessTime int64 `json:"average_process_time_ms"`
	ActiveWorkers      int32 `json:"active_workers"`
	QueueSize          int64 `json:"queue_size"`

	// Per-level stats
	L1Invalidations int64 `json:"l1_invalidations"`
	L2Invalidations int64 `json:"l2_invalidations"`
	L3Invalidations int64 `json:"l3_invalidations"`
}

// InvalidationConfig holds invalidation manager configuration
type InvalidationConfig struct {
	WorkerCount   int           `yaml:"worker_count" json:"worker_count"`
	QueueSize     int           `yaml:"queue_size" json:"queue_size"`
	BulkBatchSize int           `yaml:"bulk_batch_size" json:"bulk_batch_size"`
	BulkTimeout   time.Duration `yaml:"bulk_timeout" json:"bulk_timeout"`
	StatsInterval time.Duration `yaml:"stats_interval" json:"stats_interval"`
	EnableBulkOps bool          `yaml:"enable_bulk_ops" json:"enable_bulk_ops"`
	MaxRetries    int           `yaml:"max_retries" json:"max_retries"`
	RetryDelay    time.Duration `yaml:"retry_delay" json:"retry_delay"`
}

// invalidationWorker processes invalidation events
type invalidationWorker struct {
	id      int
	manager *InvalidationManager
	active  int32
}

// bulkInvalidationProcessor handles bulk invalidation operations
type bulkInvalidationProcessor struct {
	manager  *InvalidationManager
	buffer   map[string][]InvalidationEvent
	bufferMu sync.Mutex
	ticker   *time.Ticker
	stopChan chan struct{}
}

// DefaultInvalidationConfig returns default invalidation configuration
func DefaultInvalidationConfig() *InvalidationConfig {
	return &InvalidationConfig{
		WorkerCount:   4,
		QueueSize:     10000,
		BulkBatchSize: 100,
		BulkTimeout:   time.Second * 30,
		StatsInterval: time.Minute,
		EnableBulkOps: true,
		MaxRetries:    3,
		RetryDelay:    time.Second,
	}
}

// NewInvalidationManager creates a new invalidation manager
func NewInvalidationManager(cacheManager *CacheManager, config *InvalidationConfig, logger *zap.SugaredLogger) *InvalidationManager {
	if config == nil {
		config = DefaultInvalidationConfig()
	}

	im := &InvalidationManager{
		cacheManager: cacheManager,
		logger:       logger,
		config:       config,
		eventQueue:   make(chan InvalidationEvent, config.QueueSize),
		workerCount:  config.WorkerCount,
		stopChan:     make(chan struct{}),
		rules:        make(map[string]*InvalidationRule),
	}

	// Initialize statistics
	im.stats.Store(&InvalidationStats{})

	// Start workers
	im.startWorkers()

	// Start bulk processor if enabled
	if config.EnableBulkOps {
		im.bulkProcessor = newBulkInvalidationProcessor(im)
	}

	// Start statistics updater
	go im.statsUpdater()

	// Register default rules
	im.registerDefaultRules()

	logger.Infow("Invalidation manager initialized",
		"worker_count", config.WorkerCount,
		"queue_size", config.QueueSize,
		"bulk_enabled", config.EnableBulkOps,
	)

	return im
}

// ProcessEvent processes an invalidation event
func (im *InvalidationManager) ProcessEvent(event InvalidationEvent) error {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	select {
	case im.eventQueue <- event:
		atomic.AddInt64(&im.getStats().TotalEvents, 1)
		return nil
	default:
		return fmt.Errorf("invalidation queue full")
	}
}

// AddRule adds a new invalidation rule
func (im *InvalidationManager) AddRule(name string, rule *InvalidationRule) {
	im.rulesMu.Lock()
	defer im.rulesMu.Unlock()

	im.rules[name] = rule
	im.logger.Infow("Invalidation rule added", "name", name, "pattern", rule.Pattern)
}

// RemoveRule removes an invalidation rule
func (im *InvalidationManager) RemoveRule(name string) {
	im.rulesMu.Lock()
	defer im.rulesMu.Unlock()

	delete(im.rules, name)
	im.logger.Infow("Invalidation rule removed", "name", name)
}

// InvalidateKey invalidates a specific key
func (im *InvalidationManager) InvalidateKey(key string, reason string) error {
	event := InvalidationEvent{
		Type:      InvalidateKey,
		Keys:      []string{key},
		Reason:    reason,
		Timestamp: time.Now(),
	}
	return im.ProcessEvent(event)
}

// InvalidatePattern invalidates keys matching a pattern
func (im *InvalidationManager) InvalidatePattern(pattern string, reason string) error {
	event := InvalidationEvent{
		Type:      InvalidatePattern,
		Pattern:   pattern,
		Reason:    reason,
		Timestamp: time.Now(),
	}
	return im.ProcessEvent(event)
}

// InvalidateTag invalidates keys with a specific tag
func (im *InvalidationManager) InvalidateTag(tag string, reason string) error {
	event := InvalidationEvent{
		Type:      InvalidatePattern,
		Pattern:   fmt.Sprintf("*:%s:*", tag),
		Reason:    reason,
		Timestamp: time.Now(),
	}
	return im.ProcessEvent(event)
}

// InvalidateAll clears all cache levels (use with extreme caution)
func (im *InvalidationManager) InvalidateAll(reason string) error {
	event := InvalidationEvent{
		Type:      InvalidateAll,
		Reason:    reason,
		Timestamp: time.Now(),
	}
	return im.ProcessEvent(event)
}

// GetStats returns current invalidation statistics
func (im *InvalidationManager) GetStats() *InvalidationStats {
	return im.getStats()
}

// getStats returns a pointer to the current stats
func (im *InvalidationManager) getStats() *InvalidationStats {
	return im.stats.Load().(*InvalidationStats)
}

// Close shuts down the invalidation manager gracefully
func (im *InvalidationManager) Close() {
	im.logger.Info("Shutting down invalidation manager...")

	// Stop bulk processor
	if im.bulkProcessor != nil {
		im.bulkProcessor.stop()
	}

	// Stop workers
	close(im.stopChan)
	im.wg.Wait()

	// Close event queue
	close(im.eventQueue)

	im.logger.Info("Invalidation manager shutdown complete")
}

// startWorkers starts the invalidation workers
func (im *InvalidationManager) startWorkers() {
	im.workers = make([]*invalidationWorker, im.workerCount)

	for i := 0; i < im.workerCount; i++ {
		worker := &invalidationWorker{
			id:      i,
			manager: im,
		}
		im.workers[i] = worker

		im.wg.Add(1)
		go worker.run()
	}
}

// run is the main worker loop
func (w *invalidationWorker) run() {
	defer w.manager.wg.Done()

	w.manager.logger.Debugw("Invalidation worker started", "worker_id", w.id)

	for {
		select {
		case <-w.manager.stopChan:
			w.manager.logger.Debugw("Invalidation worker stopping", "worker_id", w.id)
			return

		case event := <-w.manager.eventQueue:
			w.processEvent(event)
		}
	}
}

// processEvent processes a single invalidation event
func (w *invalidationWorker) processEvent(event InvalidationEvent) {
	atomic.StoreInt32(&w.active, 1)
	defer atomic.StoreInt32(&w.active, 0)

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// Find matching rules
	matchingRules := w.findMatchingRules(event)
	if len(matchingRules) > 0 {
		atomic.AddInt64(&w.manager.getStats().RuleMatches, int64(len(matchingRules)))
	}

	// Process the event
	var err error
	switch event.Type {
	case InvalidateKey:
		err = w.invalidateKeys(ctx, event.Keys, matchingRules)
	case InvalidatePattern:
		err = w.invalidatePattern(ctx, event.Pattern, matchingRules)
	case InvalidateTag:
		err = w.invalidatePattern(ctx, event.Pattern, matchingRules)
	case InvalidateAll:
		err = w.invalidateAll(ctx)
	default:
		err = fmt.Errorf("unknown invalidation type: %d", event.Type)
	}

	duration := time.Since(start)
	w.updateProcessTime(duration)

	if err != nil {
		w.manager.logger.Warnw("Failed to process invalidation event",
			"event", event,
			"error", err,
			"worker_id", w.id,
		)
		atomic.AddInt64(&w.manager.getStats().FailedEvents, 1)
	} else {
		atomic.AddInt64(&w.manager.getStats().ProcessedEvents, 1)
		w.manager.logger.Debugw("Successfully processed invalidation event",
			"event", event,
			"duration_ms", duration.Milliseconds(),
			"worker_id", w.id,
		)
	}

	// Execute rule callbacks
	for _, rule := range matchingRules {
		if rule.Callback != nil {
			go rule.Callback(event)
		}
	}
}

// findMatchingRules finds rules that match the event
func (w *invalidationWorker) findMatchingRules(event InvalidationEvent) []*InvalidationRule {
	w.manager.rulesMu.RLock()
	defer w.manager.rulesMu.RUnlock()

	var matchingRules []*InvalidationRule

	for _, rule := range w.manager.rules {
		if w.ruleMatches(rule, event) {
			matchingRules = append(matchingRules, rule)
		}
	}

	return matchingRules
}

// ruleMatches checks if a rule matches an event
func (w *invalidationWorker) ruleMatches(rule *InvalidationRule, event InvalidationEvent) bool {
	// Check type match
	if rule.Type != event.Type {
		return false
	}

	// Check pattern match
	switch event.Type {
	case InvalidateKey:
		for _, key := range event.Keys {
			if matched, _ := regexp.MatchString(rule.Pattern, key); matched {
				return true
			}
		}
		return false
	case InvalidatePattern:
		if matched, _ := regexp.MatchString(rule.Pattern, event.Pattern); matched {
			return true
		}
		return false
	case InvalidateAll:
		return rule.Type == InvalidateAll
	}

	return false
}

// invalidateKeys invalidates specific keys
func (w *invalidationWorker) invalidateKeys(ctx context.Context, keys []string, rules []*InvalidationRule) error {
	stats := w.manager.getStats()

	for _, key := range keys {
		// Apply rules to determine which levels to invalidate
		levels := w.determineLevels(rules)
		for _, level := range levels {
			switch level {
			case CacheLevelL1:
				w.manager.cacheManager.l1Cache.Delete(key)
				atomic.AddInt64(&stats.L1Invalidations, 1)
			case CacheLevelL2:
				if err := w.manager.cacheManager.l2Cache.Delete(ctx, key); err != nil {
					w.manager.logger.Warnw("Failed to invalidate L2 cache", "key", key, "error", err)
				} else {
					atomic.AddInt64(&stats.L2Invalidations, 1)
				}
			case CacheLevelL3:
				if err := w.manager.cacheManager.l3Cache.Delete(ctx, key); err != nil {
					w.manager.logger.Warnw("Failed to invalidate L3 cache", "key", key, "error", err)
				} else {
					atomic.AddInt64(&stats.L3Invalidations, 1)
				}
			}
		}

		// Handle related keys
		w.invalidateRelatedKeys(ctx, key, rules)
	}

	return nil
}

// invalidatePattern invalidates keys matching a pattern
func (w *invalidationWorker) invalidatePattern(ctx context.Context, pattern string, rules []*InvalidationRule) error {
	// For pattern invalidation, we need to find matching keys first
	// This is a simplified implementation - in production, you'd want to optimize this

	// L1 Cache - scan local cache
	l1Keys := w.manager.cacheManager.l1Cache.Keys()
	var matchingKeys []string

	for _, key := range l1Keys {
		if matched, _ := regexp.MatchString(pattern, key); matched {
			matchingKeys = append(matchingKeys, key)
		}
	}
	// L2/L3 Cache - use Redis SCAN
	if len(rules) == 0 || w.shouldInvalidateLevel(rules, CacheLevelL2) {
		if err := w.manager.cacheManager.l2Cache.DeletePattern(ctx, pattern); err != nil {
			w.manager.logger.Warnw("Failed to delete L2 pattern", "pattern", pattern, "error", err)
		} else {
			atomic.AddInt64(&w.manager.getStats().L2Invalidations, 1)
		}
	}

	if len(rules) == 0 || w.shouldInvalidateLevel(rules, CacheLevelL3) {
		// Get keys matching pattern from L3
		keys, err := w.manager.cacheManager.l3Cache.GetKeys(ctx, pattern)
		if err != nil {
			w.manager.logger.Warnw("Failed to get L3 keys", "pattern", pattern, "error", err)
		} else if len(keys) > 0 {
			if err := w.manager.cacheManager.l3Cache.BulkDelete(ctx, keys); err != nil {
				w.manager.logger.Warnw("Failed to bulk delete L3 keys", "pattern", pattern, "error", err)
			} else {
				atomic.AddInt64(&w.manager.getStats().L3Invalidations, int64(len(keys)))
			}
		}
	}

	// Invalidate matching L1 keys
	if len(rules) == 0 || w.shouldInvalidateLevel(rules, CacheLevelL1) {
		for _, key := range matchingKeys {
			w.manager.cacheManager.l1Cache.Delete(key)
		}
		atomic.AddInt64(&w.manager.getStats().L1Invalidations, int64(len(matchingKeys)))
	}

	return nil
}

// invalidateAll clears all cache levels
func (w *invalidationWorker) invalidateAll(ctx context.Context) error {
	stats := w.manager.getStats()

	// Clear L1
	w.manager.cacheManager.l1Cache.Clear()
	atomic.AddInt64(&stats.L1Invalidations, 1)

	// Clear L2
	if err := w.manager.cacheManager.l2Cache.Clear(ctx); err != nil {
		w.manager.logger.Warnw("Failed to clear L2 cache", "error", err)
	} else {
		atomic.AddInt64(&stats.L2Invalidations, 1)
	}

	// Clear L3
	if err := w.manager.cacheManager.l3Cache.Clear(ctx); err != nil {
		w.manager.logger.Warnw("Failed to clear L3 cache", "error", err)
	} else {
		atomic.AddInt64(&stats.L3Invalidations, 1)
	}

	return nil
}

// determineLevels determines which cache levels to invalidate based on rules
func (w *invalidationWorker) determineLevels(rules []*InvalidationRule) []CacheLevel {
	if len(rules) == 0 {
		// Default: invalidate all levels
		return []CacheLevel{CacheLevelL1, CacheLevelL2, CacheLevelL3}
	}

	levelSet := make(map[CacheLevel]bool)
	for _, rule := range rules {
		for _, level := range rule.Levels {
			levelSet[level] = true
		}
	}

	var levels []CacheLevel
	for level := range levelSet {
		levels = append(levels, level)
	}

	return levels
}

// shouldInvalidateLevel checks if a specific level should be invalidated
func (w *invalidationWorker) shouldInvalidateLevel(rules []*InvalidationRule, level CacheLevel) bool {
	for _, rule := range rules {
		for _, ruleLevel := range rule.Levels {
			if ruleLevel == level {
				return true
			}
		}
	}
	return false
}

// invalidateRelatedKeys invalidates keys related to the given key
func (w *invalidationWorker) invalidateRelatedKeys(ctx context.Context, key string, rules []*InvalidationRule) {
	for _, rule := range rules {
		if !rule.Propagate {
			continue
		}

		for _, relatedPattern := range rule.Related {
			// Replace placeholders in the pattern with the actual key
			relatedPattern = strings.ReplaceAll(relatedPattern, "{key}", key)

			// Extract parts of the key for pattern substitution
			keyParts := strings.Split(key, ":")
			for i, part := range keyParts {
				placeholder := fmt.Sprintf("{%d}", i)
				relatedPattern = strings.ReplaceAll(relatedPattern, placeholder, part)
			}

			// Invalidate the related pattern
			event := InvalidationEvent{
				Type:      InvalidatePattern,
				Pattern:   relatedPattern,
				Reason:    fmt.Sprintf("Related to %s", key),
				Timestamp: time.Now(),
			}

			if err := w.manager.ProcessEvent(event); err != nil {
				w.manager.logger.Warnw("Failed to invalidate related key",
					"original_key", key,
					"related_pattern", relatedPattern,
					"error", err,
				)
			}
		}
	}
}

// updateProcessTime updates the average process time metric
func (w *invalidationWorker) updateProcessTime(duration time.Duration) {
	stats := w.manager.getStats()
	currentAvg := atomic.LoadInt64(&stats.AverageProcessTime)
	newAvg := (currentAvg + duration.Milliseconds()) / 2
	atomic.StoreInt64(&stats.AverageProcessTime, newAvg)
}

// statsUpdater periodically updates statistics
func (im *InvalidationManager) statsUpdater() {
	ticker := time.NewTicker(im.config.StatsInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			im.updateWorkerStats()
		case <-im.stopChan:
			return
		}
	}
}

// updateWorkerStats updates worker-related statistics
func (im *InvalidationManager) updateWorkerStats() {
	activeWorkers := int32(0)
	for _, worker := range im.workers {
		if atomic.LoadInt32(&worker.active) == 1 {
			activeWorkers++
		}
	}

	stats := im.getStats()
	atomic.StoreInt32(&stats.ActiveWorkers, activeWorkers)
	atomic.StoreInt64(&stats.QueueSize, int64(len(im.eventQueue)))
}

// registerDefaultRules registers default invalidation rules for trading platform
func (im *InvalidationManager) registerDefaultRules() { // Trading data rules
	im.AddRule("orderbook_update", &InvalidationRule{
		Pattern:   `^orderbook:.*`,
		Type:      InvalidateKey,
		Levels:    []CacheLevel{CacheLevelL1, CacheLevelL2},
		Priority:  PriorityImmediate,
		Propagate: true,
		Related:   []string{"market:{1}:*", "depth:{1}:*"},
		BatchSize: 50,
	})

	// User data rules
	im.AddRule("user_balance", &InvalidationRule{
		Pattern:   `^balance:.*`,
		Type:      InvalidateKey,
		Levels:    []CacheLevel{CacheLevelL1, CacheLevelL2},
		Priority:  PriorityUrgent,
		Propagate: true,
		Related:   []string{"portfolio:{1}:*", "positions:{1}:*"},
		BatchSize: 20,
	})

	// Market data rules
	im.AddRule("market_data", &InvalidationRule{
		Pattern:   `^market:.*`,
		Type:      InvalidateKey,
		Levels:    []CacheLevel{CacheLevelL2, CacheLevelL3},
		Priority:  PriorityNormal,
		Propagate: false,
		BatchSize: 100,
	})

	// AML/compliance rules
	im.AddRule("aml_data", &InvalidationRule{
		Pattern:   `^aml:.*`,
		Type:      InvalidateKey,
		Levels:    []CacheLevel{CacheLevelL1, CacheLevelL2, CacheLevelL3},
		Priority:  PriorityUrgent,
		Propagate: true,
		Related:   []string{"risk:{1}:*", "compliance:{1}:*"},
		BatchSize: 10,
	})
}

// Bulk invalidation processor
func newBulkInvalidationProcessor(manager *InvalidationManager) *bulkInvalidationProcessor {
	processor := &bulkInvalidationProcessor{
		manager:  manager,
		buffer:   make(map[string][]InvalidationEvent),
		ticker:   time.NewTicker(manager.config.BulkTimeout),
		stopChan: make(chan struct{}),
	}

	go processor.run()
	return processor
}

func (bp *bulkInvalidationProcessor) run() {
	for {
		select {
		case <-bp.ticker.C:
			bp.processBulkOperations()
		case <-bp.stopChan:
			return
		}
	}
}

func (bp *bulkInvalidationProcessor) processBulkOperations() {
	bp.bufferMu.Lock()
	defer bp.bufferMu.Unlock()

	for pattern, events := range bp.buffer {
		if len(events) >= bp.manager.config.BulkBatchSize {
			// Process bulk operation
			bp.manager.logger.Debugw("Processing bulk invalidation",
				"pattern", pattern,
				"event_count", len(events),
			)

			// Combine events into a single bulk operation
			bulkEvent := InvalidationEvent{
				Type:      InvalidatePattern,
				Pattern:   pattern,
				Reason:    fmt.Sprintf("Bulk operation (%d events)", len(events)),
				Timestamp: time.Now(),
			}

			if err := bp.manager.ProcessEvent(bulkEvent); err != nil {
				bp.manager.logger.Warnw("Failed to process bulk invalidation",
					"pattern", pattern,
					"error", err,
				)
			} else {
				atomic.AddInt64(&bp.manager.getStats().BulkOperations, 1)
			}

			// Clear processed events
			delete(bp.buffer, pattern)
		}
	}
}

func (bp *bulkInvalidationProcessor) stop() {
	close(bp.stopChan)
	bp.ticker.Stop()
}
