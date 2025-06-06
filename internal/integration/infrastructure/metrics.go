// Package infrastructure provides comprehensive metrics collection for performance monitoring
package infrastructure

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// MetricsCollector provides comprehensive metrics collection and reporting
type MetricsCollector interface {
	// Counter metrics
	IncrementCounter(name string, tags map[string]string)
	IncrementCounterBy(name string, value int64, tags map[string]string)

	// Gauge metrics
	SetGauge(name string, value float64, tags map[string]string)
	IncrementGauge(name string, tags map[string]string)
	DecrementGauge(name string, tags map[string]string)

	// Histogram metrics (for latency, response times)
	RecordHistogram(name string, value float64, tags map[string]string)
	RecordDuration(name string, duration time.Duration, tags map[string]string)

	// Timer utilities
	StartTimer(name string, tags map[string]string) Timer
	RecordOperation(name string, tags map[string]string, operation func() error) error

	// Business metrics
	RecordUserAction(userID uuid.UUID, action string, metadata map[string]interface{})
	RecordTransactionMetrics(txType string, amount float64, currency string, success bool)
	RecordAPIRequest(endpoint string, method string, statusCode int, duration time.Duration)
	RecordLatency(operation string, latency time.Duration, tags map[string]string)

	// System health metrics
	RecordHealthCheck(service string, healthy bool, checkDuration time.Duration)
	RecordResourceUsage(resource string, usage float64, unit string)

	// Metrics retrieval
	GetMetrics(ctx context.Context) (*MetricsSnapshot, error)
	GetMetric(ctx context.Context, name string) (*Metric, error)

	// Lifecycle
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	HealthCheck(ctx context.Context) error
}

// Timer represents a timing measurement
type Timer interface {
	Stop() time.Duration
	Discard()
}

// Metric represents a single metric
type Metric struct {
	Name      string                 `json:"name"`
	Type      MetricType             `json:"type"`
	Value     float64                `json:"value"`
	Tags      map[string]string      `json:"tags"`
	Timestamp time.Time              `json:"timestamp"`
	Metadata  map[string]interface{} `json:"metadata"`
}

// MetricType represents the type of metric
type MetricType string

const (
	MetricTypeCounter   MetricType = "counter"
	MetricTypeGauge     MetricType = "gauge"
	MetricTypeHistogram MetricType = "histogram"
	MetricTypeTimer     MetricType = "timer"
)

// MetricsSnapshot provides a snapshot of all metrics
type MetricsSnapshot struct {
	Timestamp  time.Time                   `json:"timestamp"`
	Counters   map[string]*Metric          `json:"counters"`
	Gauges     map[string]*Metric          `json:"gauges"`
	Histograms map[string]*HistogramMetric `json:"histograms"`
	Summary    *MetricsSummary             `json:"summary"`
}

// HistogramMetric represents histogram data
type HistogramMetric struct {
	*Metric
	Count       int64              `json:"count"`
	Sum         float64            `json:"sum"`
	Min         float64            `json:"min"`
	Max         float64            `json:"max"`
	Mean        float64            `json:"mean"`
	Percentiles map[string]float64 `json:"percentiles"`
}

// MetricsSummary provides high-level metrics summary
type MetricsSummary struct {
	TotalRequests      int64   `json:"total_requests"`
	SuccessfulRequests int64   `json:"successful_requests"`
	FailedRequests     int64   `json:"failed_requests"`
	SuccessRate        float64 `json:"success_rate"`
	AverageLatency     float64 `json:"average_latency_ms"`
	P95Latency         float64 `json:"p95_latency_ms"`
	P99Latency         float64 `json:"p99_latency_ms"`
	ActiveUsers        int64   `json:"active_users"`
	ActiveSessions     int64   `json:"active_sessions"`
	ErrorRate          float64 `json:"error_rate"`
}

// InMemoryMetricsCollector provides an in-memory metrics implementation
type InMemoryMetricsCollector struct {
	logger  *zap.Logger
	running bool
	mu      sync.RWMutex

	// Metric storage
	counters   map[string]*counterMetric
	gauges     map[string]*gaugeMetric
	histograms map[string]*histogramMetric

	// Configuration
	flushInterval time.Duration

	// Flush goroutine
	flushCtx    context.Context
	flushCancel context.CancelFunc
	flushWG     sync.WaitGroup
}

type counterMetric struct {
	value     int64
	tags      map[string]string
	timestamp time.Time
}

type gaugeMetric struct {
	value     float64
	tags      map[string]string
	timestamp time.Time
}

type histogramMetric struct {
	values    []float64
	tags      map[string]string
	timestamp time.Time
	count     int64
	sum       float64
	min       float64
	max       float64
}

type inMemoryTimer struct {
	collector *InMemoryMetricsCollector
	name      string
	tags      map[string]string
	startTime time.Time
}

// NewInMemoryMetricsCollector creates a new in-memory metrics collector
func NewInMemoryMetricsCollector(logger *zap.Logger) MetricsCollector {
	return &InMemoryMetricsCollector{
		logger:        logger,
		counters:      make(map[string]*counterMetric),
		gauges:        make(map[string]*gaugeMetric),
		histograms:    make(map[string]*histogramMetric),
		flushInterval: time.Minute,
	}
}

func (mc *InMemoryMetricsCollector) Start(ctx context.Context) error {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	if mc.running {
		return nil
	}

	mc.running = true
	mc.flushCtx, mc.flushCancel = context.WithCancel(ctx)

	// Start metrics flush goroutine
	mc.flushWG.Add(1)
	go mc.flushMetrics()

	mc.logger.Info("Metrics Collector started")
	return nil
}

func (mc *InMemoryMetricsCollector) Stop(ctx context.Context) error {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	if !mc.running {
		return nil
	}

	mc.running = false
	mc.flushCancel()
	mc.flushWG.Wait()

	mc.logger.Info("Metrics Collector stopped")
	return nil
}

func (mc *InMemoryMetricsCollector) IncrementCounter(name string, tags map[string]string) {
	mc.IncrementCounterBy(name, 1, tags)
}

func (mc *InMemoryMetricsCollector) IncrementCounterBy(name string, value int64, tags map[string]string) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	key := mc.buildKey(name, tags)
	if counter, exists := mc.counters[key]; exists {
		counter.value += value
		counter.timestamp = time.Now()
	} else {
		mc.counters[key] = &counterMetric{
			value:     value,
			tags:      tags,
			timestamp: time.Now(),
		}
	}
}

func (mc *InMemoryMetricsCollector) SetGauge(name string, value float64, tags map[string]string) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	key := mc.buildKey(name, tags)
	mc.gauges[key] = &gaugeMetric{
		value:     value,
		tags:      tags,
		timestamp: time.Now(),
	}
}

func (mc *InMemoryMetricsCollector) IncrementGauge(name string, tags map[string]string) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	key := mc.buildKey(name, tags)
	if gauge, exists := mc.gauges[key]; exists {
		gauge.value++
		gauge.timestamp = time.Now()
	} else {
		mc.gauges[key] = &gaugeMetric{
			value:     1,
			tags:      tags,
			timestamp: time.Now(),
		}
	}
}

func (mc *InMemoryMetricsCollector) DecrementGauge(name string, tags map[string]string) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	key := mc.buildKey(name, tags)
	if gauge, exists := mc.gauges[key]; exists {
		gauge.value--
		gauge.timestamp = time.Now()
	} else {
		mc.gauges[key] = &gaugeMetric{
			value:     -1,
			tags:      tags,
			timestamp: time.Now(),
		}
	}
}

func (mc *InMemoryMetricsCollector) RecordHistogram(name string, value float64, tags map[string]string) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	key := mc.buildKey(name, tags)
	if histogram, exists := mc.histograms[key]; exists {
		histogram.values = append(histogram.values, value)
		histogram.count++
		histogram.sum += value
		if value < histogram.min {
			histogram.min = value
		}
		if value > histogram.max {
			histogram.max = value
		}
		histogram.timestamp = time.Now()
	} else {
		mc.histograms[key] = &histogramMetric{
			values:    []float64{value},
			tags:      tags,
			timestamp: time.Now(),
			count:     1,
			sum:       value,
			min:       value,
			max:       value,
		}
	}
}

func (mc *InMemoryMetricsCollector) RecordDuration(name string, duration time.Duration, tags map[string]string) {
	mc.RecordHistogram(name, float64(duration.Nanoseconds())/1e6, tags) // Convert to milliseconds
}

func (mc *InMemoryMetricsCollector) StartTimer(name string, tags map[string]string) Timer {
	return &inMemoryTimer{
		collector: mc,
		name:      name,
		tags:      tags,
		startTime: time.Now(),
	}
}

func (mc *InMemoryMetricsCollector) RecordOperation(name string, tags map[string]string, operation func() error) error {
	timer := mc.StartTimer(name, tags)
	defer timer.Stop()

	err := operation()

	// Record success/failure
	successTags := make(map[string]string)
	for k, v := range tags {
		successTags[k] = v
	}
	successTags["success"] = fmt.Sprintf("%t", err == nil)

	mc.IncrementCounter(name+"_total", successTags)

	return err
}

func (mc *InMemoryMetricsCollector) RecordUserAction(userID uuid.UUID, action string, metadata map[string]interface{}) {
	tags := map[string]string{
		"user_id": userID.String(),
		"action":  action,
	}

	mc.IncrementCounter("user_actions_total", tags)
}

func (mc *InMemoryMetricsCollector) RecordTransactionMetrics(txType string, amount float64, currency string, success bool) {
	tags := map[string]string{
		"type":     txType,
		"currency": currency,
		"success":  fmt.Sprintf("%t", success),
	}

	mc.IncrementCounter("transactions_total", tags)
	mc.RecordHistogram("transaction_amount", amount, tags)
}

func (mc *InMemoryMetricsCollector) RecordAPIRequest(endpoint string, method string, statusCode int, duration time.Duration) {
	tags := map[string]string{
		"endpoint":    endpoint,
		"method":      method,
		"status_code": fmt.Sprintf("%d", statusCode),
	}

	mc.IncrementCounter("api_requests_total", tags)
	mc.RecordDuration("api_request_duration_ms", duration, tags)
}

func (mc *InMemoryMetricsCollector) RecordLatency(operation string, latency time.Duration, tags map[string]string) {
	if tags == nil {
		tags = make(map[string]string)
	}
	tags["operation"] = operation

	mc.RecordDuration("operation_latency_ms", latency, tags)
}

func (mc *InMemoryMetricsCollector) RecordHealthCheck(service string, healthy bool, checkDuration time.Duration) {
	tags := map[string]string{
		"service": service,
		"healthy": fmt.Sprintf("%t", healthy),
	}

	mc.IncrementCounter("health_checks_total", tags)
	mc.RecordDuration("health_check_duration_ms", checkDuration, tags)
}

func (mc *InMemoryMetricsCollector) RecordResourceUsage(resource string, usage float64, unit string) {
	tags := map[string]string{
		"resource": resource,
		"unit":     unit,
	}

	mc.SetGauge("resource_usage", usage, tags)
}

func (mc *InMemoryMetricsCollector) GetMetrics(ctx context.Context) (*MetricsSnapshot, error) {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	snapshot := &MetricsSnapshot{
		Timestamp:  time.Now(),
		Counters:   make(map[string]*Metric),
		Gauges:     make(map[string]*Metric),
		Histograms: make(map[string]*HistogramMetric),
	}

	// Convert counters
	for name, counter := range mc.counters {
		snapshot.Counters[name] = &Metric{
			Name:      name,
			Type:      MetricTypeCounter,
			Value:     float64(counter.value),
			Tags:      counter.tags,
			Timestamp: counter.timestamp,
		}
	}

	// Convert gauges
	for name, gauge := range mc.gauges {
		snapshot.Gauges[name] = &Metric{
			Name:      name,
			Type:      MetricTypeGauge,
			Value:     gauge.value,
			Tags:      gauge.tags,
			Timestamp: gauge.timestamp,
		}
	}

	// Convert histograms
	for name, histogram := range mc.histograms {
		percentiles := mc.calculatePercentiles(histogram.values)
		mean := histogram.sum / float64(histogram.count)

		snapshot.Histograms[name] = &HistogramMetric{
			Metric: &Metric{
				Name:      name,
				Type:      MetricTypeHistogram,
				Value:     mean,
				Tags:      histogram.tags,
				Timestamp: histogram.timestamp,
			},
			Count:       histogram.count,
			Sum:         histogram.sum,
			Min:         histogram.min,
			Max:         histogram.max,
			Mean:        mean,
			Percentiles: percentiles,
		}
	}

	// Generate summary
	snapshot.Summary = mc.generateSummary()

	return snapshot, nil
}

func (mc *InMemoryMetricsCollector) GetMetric(ctx context.Context, name string) (*Metric, error) {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	// Try counters first
	for key, counter := range mc.counters {
		if key == name {
			return &Metric{
				Name:      name,
				Type:      MetricTypeCounter,
				Value:     float64(counter.value),
				Tags:      counter.tags,
				Timestamp: counter.timestamp,
			}, nil
		}
	}

	// Try gauges
	for key, gauge := range mc.gauges {
		if key == name {
			return &Metric{
				Name:      name,
				Type:      MetricTypeGauge,
				Value:     gauge.value,
				Tags:      gauge.tags,
				Timestamp: gauge.timestamp,
			}, nil
		}
	}

	return nil, fmt.Errorf("metric not found: %s", name)
}

func (mc *InMemoryMetricsCollector) HealthCheck(ctx context.Context) error {
	mc.mu.RLock()
	running := mc.running
	mc.mu.RUnlock()

	if !running {
		return fmt.Errorf("metrics collector not running")
	}

	return nil
}

func (mc *InMemoryMetricsCollector) buildKey(name string, tags map[string]string) string {
	key := name
	for k, v := range tags {
		key += fmt.Sprintf(",%s=%s", k, v)
	}
	return key
}

func (mc *InMemoryMetricsCollector) calculatePercentiles(values []float64) map[string]float64 {
	if len(values) == 0 {
		return map[string]float64{}
	}

	// Simple percentile calculation (would use proper algorithm in production)
	percentiles := map[string]float64{}

	// Sort values for percentile calculation
	sorted := make([]float64, len(values))
	copy(sorted, values)

	// Simple bubble sort for demo (would use proper sort in production)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i] > sorted[j] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	percentiles["p50"] = sorted[len(sorted)*50/100]
	percentiles["p95"] = sorted[len(sorted)*95/100]
	percentiles["p99"] = sorted[len(sorted)*99/100]

	return percentiles
}

func (mc *InMemoryMetricsCollector) generateSummary() *MetricsSummary {
	summary := &MetricsSummary{}

	// Calculate request metrics
	for name, counter := range mc.counters {
		if name == "api_requests_total" {
			summary.TotalRequests += counter.value
		}
	}

	// Calculate latency metrics from histograms
	for name, histogram := range mc.histograms {
		if name == "api_request_duration_ms" {
			if histogram.count > 0 {
				summary.AverageLatency = histogram.sum / float64(histogram.count)
			}
			percentiles := mc.calculatePercentiles(histogram.values)
			summary.P95Latency = percentiles["p95"]
			summary.P99Latency = percentiles["p99"]
		}
	}

	return summary
}

func (mc *InMemoryMetricsCollector) flushMetrics() {
	defer mc.flushWG.Done()

	ticker := time.NewTicker(mc.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			mc.performFlush()
		case <-mc.flushCtx.Done():
			return
		}
	}
}

func (mc *InMemoryMetricsCollector) performFlush() {
	// In a real implementation, this would flush metrics to external systems
	// like Prometheus, InfluxDB, etc.
	mc.logger.Debug("Metrics flush performed")
}

// Timer implementation
func (t *inMemoryTimer) Stop() time.Duration {
	duration := time.Since(t.startTime)
	t.collector.RecordDuration(t.name, duration, t.tags)
	return duration
}

func (t *inMemoryTimer) Discard() {
	// Timer discarded, no recording
}
