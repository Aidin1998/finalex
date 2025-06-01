package monitoring

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// PerformanceMetrics tracks performance metrics for batch operations
type PerformanceMetrics struct {
	mu sync.RWMutex

	// Operation counts
	TotalOperations      int64 `json:"total_operations"`
	SuccessfulOperations int64 `json:"successful_operations"`
	FailedOperations     int64 `json:"failed_operations"`

	// Timing metrics
	AverageResponseTime time.Duration `json:"average_response_time"`
	MinResponseTime     time.Duration `json:"min_response_time"`
	MaxResponseTime     time.Duration `json:"max_response_time"`
	P95ResponseTime     time.Duration `json:"p95_response_time"`
	P99ResponseTime     time.Duration `json:"p99_response_time"`

	// Throughput metrics
	OperationsPerSecond float64 `json:"operations_per_second"`
	ItemsPerSecond      float64 `json:"items_per_second"`

	// Batch metrics
	AverageBatchSize float64 `json:"average_batch_size"`
	MaxBatchSize     int     `json:"max_batch_size"`
	MinBatchSize     int     `json:"min_batch_size"`

	// Error metrics
	ErrorRates map[string]float64 `json:"error_rates"`
	TopErrors  map[string]int64   `json:"top_errors"`

	// Time series data (last 100 operations)
	ResponseTimes []time.Duration `json:"-"`
	BatchSizes    []int           `json:"-"`
	Timestamps    []time.Time     `json:"-"`

	// Operation-specific metrics
	OperationMetrics map[string]*OperationMetrics `json:"operation_metrics"`

	logger *zap.Logger
}

// OperationMetrics tracks metrics for a specific operation type
type OperationMetrics struct {
	OperationType       string        `json:"operation_type"`
	Count               int64         `json:"count"`
	SuccessCount        int64         `json:"success_count"`
	FailureCount        int64         `json:"failure_count"`
	TotalDuration       time.Duration `json:"total_duration"`
	AverageDuration     time.Duration `json:"average_duration"`
	TotalItemsProcessed int64         `json:"total_items_processed"`
	AverageItemsPerOp   float64       `json:"average_items_per_op"`
	LastExecuted        time.Time     `json:"last_executed"`
}

// BatchOperationContext contains context for monitoring a batch operation
type BatchOperationContext struct {
	OperationType string
	BatchSize     int
	StartTime     time.Time
	UserID        string
	RequestID     string
}

// PerformanceMonitor provides performance monitoring for batch operations
type PerformanceMonitor struct {
	metrics *PerformanceMetrics
	logger  *zap.Logger
}

// NewPerformanceMonitor creates a new performance monitor
func NewPerformanceMonitor(logger *zap.Logger) *PerformanceMonitor {
	return &PerformanceMonitor{
		metrics: &PerformanceMetrics{
			ErrorRates:       make(map[string]float64),
			TopErrors:        make(map[string]int64),
			OperationMetrics: make(map[string]*OperationMetrics),
			ResponseTimes:    make([]time.Duration, 0, 100),
			BatchSizes:       make([]int, 0, 100),
			Timestamps:       make([]time.Time, 0, 100),
			MinResponseTime:  time.Hour, // Initialize to high value
			logger:           logger,
		},
		logger: logger,
	}
}

// StartOperation begins monitoring a batch operation
func (pm *PerformanceMonitor) StartOperation(operationType string, batchSize int) *BatchOperationContext {
	return &BatchOperationContext{
		OperationType: operationType,
		BatchSize:     batchSize,
		StartTime:     time.Now(),
		RequestID:     fmt.Sprintf("%s_%d", operationType, time.Now().UnixNano()),
	}
}

// EndOperation completes monitoring of a batch operation
func (pm *PerformanceMonitor) EndOperation(ctx *BatchOperationContext, success bool, itemsProcessed int, err error) {
	duration := time.Since(ctx.StartTime)

	pm.metrics.mu.Lock()
	defer pm.metrics.mu.Unlock()

	// Update overall metrics
	pm.metrics.TotalOperations++
	if success {
		pm.metrics.SuccessfulOperations++
	} else {
		pm.metrics.FailedOperations++
	}

	// Update timing metrics
	pm.updateTimingMetrics(duration)

	// Update batch metrics
	pm.updateBatchMetrics(ctx.BatchSize)

	// Update operation-specific metrics
	pm.updateOperationMetrics(ctx.OperationType, duration, success, itemsProcessed)

	// Update error metrics
	if err != nil {
		pm.updateErrorMetrics(err)
	}

	// Update time series data
	pm.updateTimeSeriesData(duration, ctx.BatchSize)

	// Log performance data
	pm.logOperationPerformance(ctx, duration, success, itemsProcessed, err)
}

// updateTimingMetrics updates timing-related metrics
func (pm *PerformanceMonitor) updateTimingMetrics(duration time.Duration) {
	// Update min/max
	if duration < pm.metrics.MinResponseTime {
		pm.metrics.MinResponseTime = duration
	}
	if duration > pm.metrics.MaxResponseTime {
		pm.metrics.MaxResponseTime = duration
	}

	// Calculate average
	totalTime := time.Duration(pm.metrics.TotalOperations-1)*pm.metrics.AverageResponseTime + duration
	pm.metrics.AverageResponseTime = totalTime / time.Duration(pm.metrics.TotalOperations)

	// Calculate percentiles (simplified using response times slice)
	pm.calculatePercentiles()
}

// updateBatchMetrics updates batch size metrics
func (pm *PerformanceMonitor) updateBatchMetrics(batchSize int) {
	if batchSize > pm.metrics.MaxBatchSize {
		pm.metrics.MaxBatchSize = batchSize
	}
	if pm.metrics.MinBatchSize == 0 || batchSize < pm.metrics.MinBatchSize {
		pm.metrics.MinBatchSize = batchSize
	}

	// Calculate average batch size
	totalBatchSize := pm.metrics.AverageBatchSize*float64(pm.metrics.TotalOperations-1) + float64(batchSize)
	pm.metrics.AverageBatchSize = totalBatchSize / float64(pm.metrics.TotalOperations)
}

// updateOperationMetrics updates metrics for a specific operation type
func (pm *PerformanceMonitor) updateOperationMetrics(operationType string, duration time.Duration, success bool, itemsProcessed int) {
	opMetrics, exists := pm.metrics.OperationMetrics[operationType]
	if !exists {
		opMetrics = &OperationMetrics{
			OperationType: operationType,
		}
		pm.metrics.OperationMetrics[operationType] = opMetrics
	}

	opMetrics.Count++
	opMetrics.TotalDuration += duration
	opMetrics.AverageDuration = opMetrics.TotalDuration / time.Duration(opMetrics.Count)
	opMetrics.TotalItemsProcessed += int64(itemsProcessed)
	opMetrics.AverageItemsPerOp = float64(opMetrics.TotalItemsProcessed) / float64(opMetrics.Count)
	opMetrics.LastExecuted = time.Now()

	if success {
		opMetrics.SuccessCount++
	} else {
		opMetrics.FailureCount++
	}
}

// updateErrorMetrics updates error-related metrics
func (pm *PerformanceMonitor) updateErrorMetrics(err error) {
	errorType := fmt.Sprintf("%T", err)
	pm.metrics.TopErrors[errorType]++

	// Calculate error rate for this error type
	pm.metrics.ErrorRates[errorType] = float64(pm.metrics.TopErrors[errorType]) / float64(pm.metrics.TotalOperations)
}

// updateTimeSeriesData updates time series data for trend analysis
func (pm *PerformanceMonitor) updateTimeSeriesData(duration time.Duration, batchSize int) {
	// Keep only last 100 entries
	if len(pm.metrics.ResponseTimes) >= 100 {
		pm.metrics.ResponseTimes = pm.metrics.ResponseTimes[1:]
		pm.metrics.BatchSizes = pm.metrics.BatchSizes[1:]
		pm.metrics.Timestamps = pm.metrics.Timestamps[1:]
	}

	pm.metrics.ResponseTimes = append(pm.metrics.ResponseTimes, duration)
	pm.metrics.BatchSizes = append(pm.metrics.BatchSizes, batchSize)
	pm.metrics.Timestamps = append(pm.metrics.Timestamps, time.Now())

	// Calculate throughput based on recent data
	pm.calculateThroughput()
}

// calculatePercentiles calculates P95 and P99 response times
func (pm *PerformanceMonitor) calculatePercentiles() {
	if len(pm.metrics.ResponseTimes) < 5 {
		return
	}

	// Simple percentile calculation using sorted response times
	sorted := make([]time.Duration, len(pm.metrics.ResponseTimes))
	copy(sorted, pm.metrics.ResponseTimes)

	// Simple bubble sort for small datasets
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i] > sorted[j] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	p95Index := int(0.95 * float64(len(sorted)))
	p99Index := int(0.99 * float64(len(sorted)))

	if p95Index < len(sorted) {
		pm.metrics.P95ResponseTime = sorted[p95Index]
	}
	if p99Index < len(sorted) {
		pm.metrics.P99ResponseTime = sorted[p99Index]
	}
}

// calculateThroughput calculates operations and items per second
func (pm *PerformanceMonitor) calculateThroughput() {
	if len(pm.metrics.Timestamps) < 2 {
		return
	}

	// Calculate based on last 10 operations
	start := len(pm.metrics.Timestamps) - 10
	if start < 0 {
		start = 0
	}

	timeSpan := pm.metrics.Timestamps[len(pm.metrics.Timestamps)-1].Sub(pm.metrics.Timestamps[start])
	if timeSpan <= 0 {
		return
	}

	operations := len(pm.metrics.Timestamps) - start
	pm.metrics.OperationsPerSecond = float64(operations) / timeSpan.Seconds()

	// Calculate items per second
	totalItems := 0
	for i := start; i < len(pm.metrics.BatchSizes); i++ {
		totalItems += pm.metrics.BatchSizes[i]
	}
	pm.metrics.ItemsPerSecond = float64(totalItems) / timeSpan.Seconds()
}

// logOperationPerformance logs detailed performance information
func (pm *PerformanceMonitor) logOperationPerformance(ctx *BatchOperationContext, duration time.Duration, success bool, itemsProcessed int, err error) {
	fields := []zap.Field{
		zap.String("operation", ctx.OperationType),
		zap.String("request_id", ctx.RequestID),
		zap.Int("batch_size", ctx.BatchSize),
		zap.Duration("duration", duration),
		zap.Bool("success", success),
		zap.Int("items_processed", itemsProcessed),
		zap.Float64("items_per_second", float64(itemsProcessed)/duration.Seconds()),
	}

	if err != nil {
		fields = append(fields, zap.Error(err))
		pm.logger.Warn("Batch operation completed with error", fields...)
	} else {
		pm.logger.Info("Batch operation completed successfully", fields...)
	}
}

// GetMetrics returns a copy of current metrics
func (pm *PerformanceMonitor) GetMetrics() *PerformanceMetrics {
	pm.metrics.mu.RLock()
	defer pm.metrics.mu.RUnlock()

	// Create a copy
	metrics := &PerformanceMetrics{
		TotalOperations:      pm.metrics.TotalOperations,
		SuccessfulOperations: pm.metrics.SuccessfulOperations,
		FailedOperations:     pm.metrics.FailedOperations,
		AverageResponseTime:  pm.metrics.AverageResponseTime,
		MinResponseTime:      pm.metrics.MinResponseTime,
		MaxResponseTime:      pm.metrics.MaxResponseTime,
		P95ResponseTime:      pm.metrics.P95ResponseTime,
		P99ResponseTime:      pm.metrics.P99ResponseTime,
		OperationsPerSecond:  pm.metrics.OperationsPerSecond,
		ItemsPerSecond:       pm.metrics.ItemsPerSecond,
		AverageBatchSize:     pm.metrics.AverageBatchSize,
		MaxBatchSize:         pm.metrics.MaxBatchSize,
		MinBatchSize:         pm.metrics.MinBatchSize,
		ErrorRates:           make(map[string]float64),
		TopErrors:            make(map[string]int64),
		OperationMetrics:     make(map[string]*OperationMetrics),
	}

	// Copy maps
	for k, v := range pm.metrics.ErrorRates {
		metrics.ErrorRates[k] = v
	}
	for k, v := range pm.metrics.TopErrors {
		metrics.TopErrors[k] = v
	}
	for k, v := range pm.metrics.OperationMetrics {
		opMetrics := *v // Copy struct
		metrics.OperationMetrics[k] = &opMetrics
	}

	return metrics
}

// GetMetricsJSON returns metrics as JSON
func (pm *PerformanceMonitor) GetMetricsJSON() ([]byte, error) {
	metrics := pm.GetMetrics()
	return json.MarshalIndent(metrics, "", "  ")
}

// Reset resets all metrics
func (pm *PerformanceMonitor) Reset() {
	pm.metrics.mu.Lock()
	defer pm.metrics.mu.Unlock()

	pm.metrics.TotalOperations = 0
	pm.metrics.SuccessfulOperations = 0
	pm.metrics.FailedOperations = 0
	pm.metrics.AverageResponseTime = 0
	pm.metrics.MinResponseTime = time.Hour
	pm.metrics.MaxResponseTime = 0
	pm.metrics.P95ResponseTime = 0
	pm.metrics.P99ResponseTime = 0
	pm.metrics.OperationsPerSecond = 0
	pm.metrics.ItemsPerSecond = 0
	pm.metrics.AverageBatchSize = 0
	pm.metrics.MaxBatchSize = 0
	pm.metrics.MinBatchSize = 0

	pm.metrics.ErrorRates = make(map[string]float64)
	pm.metrics.TopErrors = make(map[string]int64)
	pm.metrics.OperationMetrics = make(map[string]*OperationMetrics)
	pm.metrics.ResponseTimes = make([]time.Duration, 0, 100)
	pm.metrics.BatchSizes = make([]int, 0, 100)
	pm.metrics.Timestamps = make([]time.Time, 0, 100)
}

// GetSuccessRate returns the overall success rate
func (pm *PerformanceMonitor) GetSuccessRate() float64 {
	pm.metrics.mu.RLock()
	defer pm.metrics.mu.RUnlock()

	if pm.metrics.TotalOperations == 0 {
		return 0
	}

	return float64(pm.metrics.SuccessfulOperations) / float64(pm.metrics.TotalOperations)
}

// GetOperationSuccessRate returns success rate for a specific operation
func (pm *PerformanceMonitor) GetOperationSuccessRate(operationType string) float64 {
	pm.metrics.mu.RLock()
	defer pm.metrics.mu.RUnlock()

	opMetrics, exists := pm.metrics.OperationMetrics[operationType]
	if !exists || opMetrics.Count == 0 {
		return 0
	}

	return float64(opMetrics.SuccessCount) / float64(opMetrics.Count)
}

// PrintSummary prints a human-readable summary of metrics
func (pm *PerformanceMonitor) PrintSummary() {
	metrics := pm.GetMetrics()

	fmt.Printf("\n=== Batch Operations Performance Summary ===\n")
	fmt.Printf("Total Operations: %d\n", metrics.TotalOperations)
	fmt.Printf("Success Rate: %.2f%%\n", pm.GetSuccessRate()*100)
	fmt.Printf("Average Response Time: %v\n", metrics.AverageResponseTime)
	fmt.Printf("P95 Response Time: %v\n", metrics.P95ResponseTime)
	fmt.Printf("P99 Response Time: %v\n", metrics.P99ResponseTime)
	fmt.Printf("Operations/sec: %.2f\n", metrics.OperationsPerSecond)
	fmt.Printf("Items/sec: %.2f\n", metrics.ItemsPerSecond)
	fmt.Printf("Average Batch Size: %.1f\n", metrics.AverageBatchSize)

	if len(metrics.OperationMetrics) > 0 {
		fmt.Printf("\n--- Operation Breakdown ---\n")
		for opType, opMetrics := range metrics.OperationMetrics {
			successRate := float64(opMetrics.SuccessCount) / float64(opMetrics.Count) * 100
			fmt.Printf("%s: %d ops, %.1f%% success, avg %v\n",
				opType, opMetrics.Count, successRate, opMetrics.AverageDuration)
		}
	}

	if len(metrics.TopErrors) > 0 {
		fmt.Printf("\n--- Top Errors ---\n")
		for errType, count := range metrics.TopErrors {
			fmt.Printf("%s: %d occurrences\n", errType, count)
		}
	}
	fmt.Printf("============================================\n\n")
}
