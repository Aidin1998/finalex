// BusinessMetrics provides real-time and historical business/trading quality metrics.
// Includes fill rates, slippage, market impact, compliance, and alerting.
package metrics

import (
	"sync"
	"time"
)

type SlidingWindow struct {
	window  time.Duration
	entries []float64
	times   []time.Time
	mu      sync.Mutex
}

func NewSlidingWindow(window time.Duration) *SlidingWindow {
	return &SlidingWindow{window: window}
}

func (sw *SlidingWindow) Add(value float64) {
	sw.mu.Lock()
	sw.entries = append(sw.entries, value)
	sw.times = append(sw.times, time.Now())
	sw.cleanup()
	sw.mu.Unlock()
}

func (sw *SlidingWindow) Values() []float64 {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	sw.cleanup()
	return append([]float64{}, sw.entries...)
}

func (sw *SlidingWindow) cleanup() {
	cutoff := time.Now().Add(-sw.window)
	idx := 0
	for i, t := range sw.times {
		if t.After(cutoff) {
			idx = i
			break
		}
	}
	sw.entries = sw.entries[idx:]
	sw.times = sw.times[idx:]
}

type FillRateStats struct {
	Filled int
	Total  int
}

type SlippageStats struct {
	Values *SlidingWindow
	Worst  float64
}

type MarketImpactStats struct {
	Values *SlidingWindow
}

type SpreadStats struct {
	Values *SlidingWindow
}

type ConversionStats struct {
	Orders int
	Trades int
}

type FailedOrderStats struct {
	Reasons map[string]int
	Mu      sync.Mutex // Export the mutex
}

// BusinessMetrics tracks all business-level trading metrics.
type BusinessMetrics struct {
	Mu sync.RWMutex // Export the mutex

	// Per market, user, and order type
	FillRates    map[string]map[string]map[string]*FillRateStats // market->user->type
	Slippage     map[string]map[string]*SlippageStats            // market->user
	MarketImpact map[string]*MarketImpactStats                   // market
	Spread       map[string]*SpreadStats                         // market
	Conversion   map[string]*ConversionStats                     // market
	FailedOrders map[string]*FailedOrderStats                    // market

	// Historical metrics for trend analysis
	History map[string][]BusinessMetricSnapshot // market

	// Compliance and audit
	AuditTrail []MetricAuditEvent
}

type BusinessMetricSnapshot struct {
	Timestamp    time.Time
	FillRates    map[string]map[string]map[string]FillRateStats
	Slippage     map[string]map[string]float64
	MarketImpact map[string]float64
	Spread       map[string]float64
	Conversion   map[string]float64
	FailedOrders map[string]map[string]int
}

type MetricAuditEvent struct {
	Timestamp time.Time
	Event     string
	Details   map[string]interface{}
}

func NewBusinessMetrics() *BusinessMetrics {
	return &BusinessMetrics{
		FillRates:    make(map[string]map[string]map[string]*FillRateStats),
		Slippage:     make(map[string]map[string]*SlippageStats),
		MarketImpact: make(map[string]*MarketImpactStats),
		Spread:       make(map[string]*SpreadStats),
		Conversion:   make(map[string]*ConversionStats),
		FailedOrders: make(map[string]*FailedOrderStats),
		History:      make(map[string][]BusinessMetricSnapshot),
		AuditTrail:   make([]MetricAuditEvent, 0, 10000),
	}
}

// ... Methods for updating, aggregating, and alerting will be implemented next ...

// RecordTriggerLatency records latency for trigger conditions
func (bm *BusinessMetrics) RecordTriggerLatency(pair string, latency time.Duration) {
	// Implementation for trigger latency recording
}

// RecordTriggerMetrics records trigger performance metrics
func (bm *BusinessMetrics) RecordTriggerMetrics(stats interface{}) {
	// Implementation for trigger metrics recording
}
