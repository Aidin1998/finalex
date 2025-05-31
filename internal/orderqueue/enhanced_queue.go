package orderqueue

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/dgraph-io/badger/v3"
	"go.uber.org/zap"
)

// EnhancedBadgerQueue extends BadgerQueue with additional enterprise features.
type EnhancedBadgerQueue struct {
	*BadgerQueue
	
	// Enhanced features
	batchProcessor *BatchProcessor
	metrics        *QueueMetrics
	dlq            *DeadLetterQueue
	logger         *zap.SugaredLogger
	
	// Configuration
	config EnhancedQueueConfig
	
	// Lifecycle management
	stopCh  chan struct{}
	stopped bool
	stopMu  sync.RWMutex
}

// EnhancedQueueConfig configures enhanced queue behavior.
type EnhancedQueueConfig struct {
	BatchSize           int
	BatchTimeout        time.Duration
	MaxRetries          int
	RetryBackoff        time.Duration
	DeadLetterEnabled   bool
	MetricsEnabled      bool
	CompressionEnabled  bool
	HealthCheckInterval time.Duration
}

// BatchProcessor handles batch processing of orders.
type BatchProcessor struct {
	batchSize    int
	batchTimeout time.Duration
	buffer       []Order
	bufferMu     sync.Mutex
	timer        *time.Timer
	processFn    func([]Order) error
	logger       *zap.SugaredLogger
}

// QueueMetrics tracks queue performance and health metrics.
type QueueMetrics struct {
	// Counters
	enqueueCount   int64
	dequeueCount   int64
	ackCount       int64
	replayCount    int64
	errorCount     int64
	
	// Timing metrics
	totalEnqueueTime time.Duration
	totalDequeueTime time.Duration
	avgProcessTime   time.Duration
	
	// Queue health
	currentSize      int64
	peakSize         int64
	oldestOrderAge   time.Duration
	processingRate   float64 // orders per second
	
	// Error tracking
	lastError     error
	lastErrorTime time.Time
	
	// Batch metrics
	batchesProcessed int64
	avgBatchSize     float64
	
	mu sync.RWMutex
}

// DeadLetterQueue handles orders that failed to process after max retries.
type DeadLetterQueue struct {
	db     *badger.DB
	logger *zap.SugaredLogger
}

// HealthStatus represents the health status of the queue.
type HealthStatus struct {
	Status           string                 `json:"status"`
	QueueSize        int64                  `json:"queue_size"`
	ProcessingRate   float64               `json:"processing_rate"`
	ErrorRate        float64               `json:"error_rate"`
	OldestOrderAge   time.Duration         `json:"oldest_order_age"`
	LastError        string                `json:"last_error,omitempty"`
	LastErrorTime    time.Time             `json:"last_error_time,omitempty"`
	Metrics          map[string]interface{} `json:"metrics"`
	PendingRetries   int64                 `json:"pending_retries"`
	DeadLetterCount  int64                 `json:"dead_letter_count"`
}

// NewEnhancedBadgerQueue creates a new enhanced BadgerDB queue.
func NewEnhancedBadgerQueue(
	path string,
	config EnhancedQueueConfig,
	logger *zap.SugaredLogger,
) (*EnhancedBadgerQueue, error) {
	// Create base queue
	baseQueue, err := NewBadgerQueue(path)
	if err != nil {
		return nil, fmt.Errorf("failed to create base queue: %w", err)
	}
	
	// Set defaults
	if config.BatchSize == 0 {
		config.BatchSize = 100
	}
	if config.BatchTimeout == 0 {
		config.BatchTimeout = time.Second
	}
	if config.MaxRetries == 0 {
		config.MaxRetries = 3
	}
	if config.RetryBackoff == 0 {
		config.RetryBackoff = time.Second
	}
	if config.HealthCheckInterval == 0 {
		config.HealthCheckInterval = 30 * time.Second
	}
	
	eq := &EnhancedBadgerQueue{
		BadgerQueue: baseQueue,
		config:      config,
		logger:      logger,
		stopCh:      make(chan struct{}),
	}
	
	// Initialize metrics
	if config.MetricsEnabled {
		eq.metrics = &QueueMetrics{}
	}
	
	// Initialize dead letter queue
	if config.DeadLetterEnabled {
		dlqPath := path + "_dlq"
		eq.dlq, err = NewDeadLetterQueue(dlqPath, logger)
		if err != nil {
			logger.Errorw("Failed to initialize dead letter queue", "error", err)
		}
	}
	
	// Initialize batch processor
	eq.batchProcessor = &BatchProcessor{
		batchSize:    config.BatchSize,
		batchTimeout: config.BatchTimeout,
		buffer:       make([]Order, 0, config.BatchSize),
		logger:       logger,
	}
	
	return eq, nil
}

// Start begins enhanced queue processing.
func (eq *EnhancedBadgerQueue) Start(ctx context.Context) error {
	eq.stopMu.Lock()
	if eq.stopped {
		eq.stopMu.Unlock()
		return fmt.Errorf("queue already stopped")
	}
	eq.stopMu.Unlock()
	
	eq.logger.Info("Starting enhanced queue processing")
	
	// Start health monitoring
	if eq.config.MetricsEnabled {
		go eq.startHealthMonitoring(ctx)
	}
	
	// Start batch processing
	go eq.startBatchProcessing(ctx)
	
	return nil
}

// EnqueueBatch adds multiple orders to the queue in a single transaction.
func (eq *EnhancedBadgerQueue) EnqueueBatch(ctx context.Context, orders []Order) error {
	startTime := time.Now()
	
	err := eq.db.Update(func(txn *badger.Txn) error {
		for _, order := range orders {
			key, err := formatKey(order)
			if err != nil {
				return err
			}
			
			val, err := json.Marshal(order)
			if err != nil {
				return err
			}
			
			// Check for duplicates
			_, err = txn.Get(key)
			if err == nil {
				return fmt.Errorf("order duplicate: %s", order.ID)
			}
			if err != badger.ErrKeyNotFound {
				return err
			}
			
			if err := txn.Set(key, val); err != nil {
				return err
			}
		}
		return nil
	})
	
	// Update metrics
	if eq.metrics != nil {
		eq.metrics.mu.Lock()
		if err == nil {
			eq.metrics.enqueueCount += int64(len(orders))
			eq.metrics.totalEnqueueTime += time.Since(startTime)
		} else {
			eq.metrics.errorCount++
			eq.metrics.lastError = err
			eq.metrics.lastErrorTime = time.Now()
		}
		eq.metrics.mu.Unlock()
	}
	
	return err
}

// DequeueBatch retrieves multiple orders in priority order.
func (eq *EnhancedBadgerQueue) DequeueBatch(ctx context.Context, maxCount int) ([]Order, error) {
	startTime := time.Now()
	orders := make([]Order, 0, maxCount)
	
	err := eq.db.View(func(txn *badger.Txn) error {
		r := txn.NewIterator(badger.DefaultIteratorOptions)
		defer r.Close()
		
		count := 0
		for r.Rewind(); r.Valid() && count < maxCount; r.Next() {
			item := r.Item()
			var order Order
			err := item.Value(func(v []byte) error {
				return json.Unmarshal(v, &order)
			})
			if err != nil {
				return err
			}
			orders = append(orders, order)
			count++
		}
		return nil
	})
	
	// Update metrics
	if eq.metrics != nil {
		eq.metrics.mu.Lock()
		if err == nil {
			eq.metrics.dequeueCount += int64(len(orders))
			eq.metrics.totalDequeueTime += time.Since(startTime)
		} else {
			eq.metrics.errorCount++
			eq.metrics.lastError = err
			eq.metrics.lastErrorTime = time.Now()
		}
		eq.metrics.mu.Unlock()
	}
	
	return orders, err
}

// AcknowledgeBatch removes multiple processed orders from storage.
func (eq *EnhancedBadgerQueue) AcknowledgeBatch(ctx context.Context, orderIDs []string) error {
	err := eq.db.Update(func(txn *badger.Txn) error {
		r := txn.NewIterator(badger.DefaultIteratorOptions)
		defer r.Close()
		
		for _, orderID := range orderIDs {
			found := false
			for r.Rewind(); r.Valid(); r.Next() {
				item := r.Item()
				k := item.Key()
				if strings.HasSuffix(string(k), ":"+orderID) {
					if err := txn.Delete(k); err != nil {
						return err
					}
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("order not found: %s", orderID)
			}
		}
		return nil
	})
	
	// Update metrics
	if eq.metrics != nil {
		eq.metrics.mu.Lock()
		if err == nil {
			eq.metrics.ackCount += int64(len(orderIDs))
		} else {
			eq.metrics.errorCount++
			eq.metrics.lastError = err
			eq.metrics.lastErrorTime = time.Now()
		}
		eq.metrics.mu.Unlock()
	}
	
	return err
}

// GetQueueSize returns the current number of orders in the queue.
func (eq *EnhancedBadgerQueue) GetQueueSize(ctx context.Context) (int64, error) {
	var count int64
	
	err := eq.db.View(func(txn *badger.Txn) error {
		r := txn.NewIterator(badger.DefaultIteratorOptions)
		defer r.Close()
		
		for r.Rewind(); r.Valid(); r.Next() {
			count++
		}
		return nil
	})
	
	return count, err
}

// GetOldestOrder returns the oldest order in the queue.
func (eq *EnhancedBadgerQueue) GetOldestOrder(ctx context.Context) (*Order, error) {
	var oldestOrder *Order
	
	err := eq.db.View(func(txn *badger.Txn) error {
		r := txn.NewIterator(badger.DefaultIteratorOptions)
		defer r.Close()
		
		if r.Rewind(); r.Valid() {
			item := r.Item()
			var order Order
			err := item.Value(func(v []byte) error {
				return json.Unmarshal(v, &order)
			})
			if err != nil {
				return err
			}
			oldestOrder = &order
		}
		return nil
	})
	
	return oldestOrder, err
}

// GetHealthStatus returns comprehensive health information.
func (eq *EnhancedBadgerQueue) GetHealthStatus(ctx context.Context) (*HealthStatus, error) {
	status := &HealthStatus{
		Status:  "healthy",
		Metrics: make(map[string]interface{}),
	}
	
	// Get queue size
	queueSize, err := eq.GetQueueSize(ctx)
	if err != nil {
		status.Status = "unhealthy"
		return status, err
	}
	status.QueueSize = queueSize
	
	// Get oldest order age
	oldestOrder, err := eq.GetOldestOrder(ctx)
	if err == nil && oldestOrder != nil {
		status.OldestOrderAge = time.Since(oldestOrder.CreatedAt)
	}
	
	// Get metrics
	if eq.metrics != nil {
		eq.metrics.mu.RLock()
		status.ProcessingRate = eq.metrics.processingRate
		status.ErrorRate = float64(eq.metrics.errorCount) / 
			max(float64(eq.metrics.enqueueCount), 1.0) * 100
		status.LastError = ""
		if eq.metrics.lastError != nil {
			status.LastError = eq.metrics.lastError.Error()
		}
		status.LastErrorTime = eq.metrics.lastErrorTime
		
		status.Metrics = map[string]interface{}{
			"enqueue_count":     eq.metrics.enqueueCount,
			"dequeue_count":     eq.metrics.dequeueCount,
			"ack_count":         eq.metrics.ackCount,
			"error_count":       eq.metrics.errorCount,
			"batches_processed": eq.metrics.batchesProcessed,
			"avg_batch_size":    eq.metrics.avgBatchSize,
		}
		eq.metrics.mu.RUnlock()
	}
	
	// Get dead letter count
	if eq.dlq != nil {
		dlqCount, err := eq.dlq.GetCount(ctx)
		if err == nil {
			status.DeadLetterCount = dlqCount
		}
	}
	
	// Determine overall status
	if status.ErrorRate > 5.0 { // 5% error rate threshold
		status.Status = "degraded"
	}
	if status.ErrorRate > 20.0 { // 20% error rate threshold
		status.Status = "unhealthy"
	}
	if status.QueueSize > 10000 { // Size threshold
		status.Status = "degraded"
	}
	if status.OldestOrderAge > 5*time.Minute { // Age threshold
		status.Status = "degraded"
	}
	
	return status, nil
}

// SetBatchProcessor sets a custom batch processing function.
func (eq *EnhancedBadgerQueue) SetBatchProcessor(processFn func([]Order) error) {
	eq.batchProcessor.processFn = processFn
}

// Shutdown gracefully shuts down the enhanced queue.
func (eq *EnhancedBadgerQueue) Shutdown(ctx context.Context) error {
	eq.stopMu.Lock()
	if eq.stopped {
		eq.stopMu.Unlock()
		return nil
	}
	eq.stopped = true
	eq.stopMu.Unlock()
	
	eq.logger.Info("Shutting down enhanced queue")
	
	// Signal stop
	close(eq.stopCh)
	
	// Process remaining batches
	eq.batchProcessor.flush()
	
	// Close dead letter queue
	if eq.dlq != nil {
		eq.dlq.Close()
	}
	
	// Call base shutdown
	return eq.BadgerQueue.Shutdown(ctx)
}

// Private methods

func (eq *EnhancedBadgerQueue) startHealthMonitoring(ctx context.Context) {
	ticker := time.NewTicker(eq.config.HealthCheckInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-eq.stopCh:
			return
		case <-ticker.C:
			eq.updateHealthMetrics(ctx)
		}
	}
}

func (eq *EnhancedBadgerQueue) updateHealthMetrics(ctx context.Context) {
	if eq.metrics == nil {
		return
	}
	
	queueSize, err := eq.GetQueueSize(ctx)
	if err != nil {
		eq.logger.Errorw("Failed to get queue size for metrics", "error", err)
		return
	}
	
	eq.metrics.mu.Lock()
	defer eq.metrics.mu.Unlock()
	
	eq.metrics.currentSize = queueSize
	if queueSize > eq.metrics.peakSize {
		eq.metrics.peakSize = queueSize
	}
	
	// Calculate processing rate (simplified)
	// In production, this would use a sliding window
	if eq.metrics.dequeueCount > 0 {
		eq.metrics.processingRate = float64(eq.metrics.dequeueCount) / 
			eq.config.HealthCheckInterval.Seconds()
	}
}

func (eq *EnhancedBadgerQueue) startBatchProcessing(ctx context.Context) {
	if eq.batchProcessor.processFn == nil {
		return // No batch processor configured
	}
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-eq.stopCh:
			return
		case <-eq.batchProcessor.timer.C:
			eq.batchProcessor.flush()
		}
	}
}

func (bp *BatchProcessor) addToBatch(order Order) {
	bp.bufferMu.Lock()
	defer bp.bufferMu.Unlock()
	
	bp.buffer = append(bp.buffer, order)
	
	// Start timer on first item
	if len(bp.buffer) == 1 {
		bp.timer = time.NewTimer(bp.batchTimeout)
	}
	
	// Flush if batch is full
	if len(bp.buffer) >= bp.batchSize {
		bp.flushLocked()
	}
}

func (bp *BatchProcessor) flush() {
	bp.bufferMu.Lock()
	defer bp.bufferMu.Unlock()
	bp.flushLocked()
}

func (bp *BatchProcessor) flushLocked() {
	if len(bp.buffer) == 0 {
		return
	}
	
	// Stop timer
	if bp.timer != nil {
		bp.timer.Stop()
		bp.timer = nil
	}
	
	// Process batch
	if bp.processFn != nil {
		if err := bp.processFn(bp.buffer); err != nil {
			bp.logger.Errorw("Batch processing failed", "error", err, "batch_size", len(bp.buffer))
		} else {
			bp.logger.Debugw("Processed batch", "size", len(bp.buffer))
		}
	}
	
	// Clear buffer
	bp.buffer = bp.buffer[:0]
}

// DeadLetterQueue implementation

func NewDeadLetterQueue(path string, logger *zap.SugaredLogger) (*DeadLetterQueue, error) {
	opts := badger.DefaultOptions(path)
	opts.Logger = nil
	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open dead letter queue: %w", err)
	}
	
	return &DeadLetterQueue{
		db:     db,
		logger: logger,
	}, nil
}

func (dlq *DeadLetterQueue) Add(ctx context.Context, order Order, reason string) error {
	deadLetter := struct {
		Order  Order     `json:"order"`
		Reason string    `json:"reason"`
		Time   time.Time `json:"time"`
	}{
		Order:  order,
		Reason: reason,
		Time:   time.Now(),
	}
	
	data, err := json.Marshal(deadLetter)
	if err != nil {
		return err
	}
	
	key := fmt.Sprintf("dlq:%d:%s", time.Now().UnixNano(), order.ID)
	return dlq.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(key), data)
	})
}

func (dlq *DeadLetterQueue) GetCount(ctx context.Context) (int64, error) {
	var count int64
	
	err := dlq.db.View(func(txn *badger.Txn) error {
		r := txn.NewIterator(badger.DefaultIteratorOptions)
		defer r.Close()
		
		for r.Rewind(); r.Valid(); r.Next() {
			count++
		}
		return nil
	})
	
	return count, err
}

func (dlq *DeadLetterQueue) Close() error {
	return dlq.db.Close()
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
