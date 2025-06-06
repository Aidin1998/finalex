// Package infrastructure provides distributed tracing for end-to-end request tracking
package infrastructure

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// DistributedTracer provides distributed tracing capabilities
type DistributedTracer interface {
	// Span management
	StartSpan(ctx context.Context, operationName string, options ...SpanOption) (Span, context.Context)
	SpanFromContext(ctx context.Context) Span
	ContextWithSpan(ctx context.Context, span Span) context.Context

	// Trace querying
	GetTrace(ctx context.Context, traceID uuid.UUID) (*Trace, error)
	GetActiveTraces(ctx context.Context) ([]*Trace, error)

	// Lifecycle
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	HealthCheck(ctx context.Context) error
}

// Span represents a single operation in a trace
type Span interface {
	// Span identification
	TraceID() uuid.UUID
	SpanID() uuid.UUID
	ParentSpanID() *uuid.UUID

	// Span data
	SetTag(key string, value interface{})
	SetBaggageItem(key string, value string)
	GetBaggageItem(key string) string
	LogEvent(event string, fields map[string]interface{})
	LogError(err error)

	// Span lifecycle
	Finish()
	FinishWithOptions(options FinishOptions)
	SetOperationName(name string)

	// Status
	IsFinished() bool
	Duration() time.Duration
}

// Trace represents a complete trace with all spans
type Trace struct {
	TraceID   uuid.UUID              `json:"trace_id"`
	Spans     []*SpanData            `json:"spans"`
	StartTime time.Time              `json:"start_time"`
	EndTime   time.Time              `json:"end_time"`
	Duration  time.Duration          `json:"duration"`
	Tags      map[string]interface{} `json:"tags"`
	Services  []string               `json:"services"`
	Status    TraceStatus            `json:"status"`
}

// SpanData represents the data of a completed span
type SpanData struct {
	TraceID       uuid.UUID              `json:"trace_id"`
	SpanID        uuid.UUID              `json:"span_id"`
	ParentSpanID  *uuid.UUID             `json:"parent_span_id,omitempty"`
	OperationName string                 `json:"operation_name"`
	StartTime     time.Time              `json:"start_time"`
	EndTime       time.Time              `json:"end_time"`
	Duration      time.Duration          `json:"duration"`
	Tags          map[string]interface{} `json:"tags"`
	Logs          []LogEntry             `json:"logs"`
	Status        SpanStatus             `json:"status"`
	Service       string                 `json:"service"`
}

// LogEntry represents a log entry within a span
type LogEntry struct {
	Timestamp time.Time              `json:"timestamp"`
	Event     string                 `json:"event"`
	Fields    map[string]interface{} `json:"fields"`
}

// SpanOption configures span creation
type SpanOption func(*SpanOptions)

// SpanOptions contains options for span creation
type SpanOptions struct {
	ParentSpan  Span
	Tags        map[string]interface{}
	StartTime   time.Time
	ChildOf     Span
	FollowsFrom Span
	References  []SpanReference
}

// SpanReference represents a reference to another span
type SpanReference struct {
	Type           ReferenceType
	ReferencedSpan Span
}

// ReferenceType represents the type of span reference
type ReferenceType string

const (
	ChildOfRef     ReferenceType = "child_of"
	FollowsFromRef ReferenceType = "follows_from"
)

// FinishOptions contains options for finishing a span
type FinishOptions struct {
	FinishTime time.Time
}

// TraceStatus represents the overall status of a trace
type TraceStatus string

const (
	TraceStatusActive    TraceStatus = "active"
	TraceStatusCompleted TraceStatus = "completed"
	TraceStatusError     TraceStatus = "error"
)

// SpanStatus represents the status of a span
type SpanStatus string

const (
	SpanStatusOK    SpanStatus = "ok"
	SpanStatusError SpanStatus = "error"
)

// InMemoryTracer provides an in-memory distributed tracing implementation
type InMemoryTracer struct {
	logger      *zap.Logger
	traces      map[uuid.UUID]*Trace
	activeSpans map[uuid.UUID]*inMemorySpan
	mu          sync.RWMutex
	running     bool

	// Configuration
	maxTraces       int
	traceTimeout    time.Duration
	cleanupInterval time.Duration

	// Cleanup goroutine
	cleanupCtx    context.Context
	cleanupCancel context.CancelFunc
	cleanupWG     sync.WaitGroup
}

// inMemorySpan implements the Span interface
type inMemorySpan struct {
	tracer   *InMemoryTracer
	spanData *SpanData
	baggage  map[string]string
	finished bool
	mu       sync.RWMutex
}

// NewInMemoryTracer creates a new in-memory tracer
func NewInMemoryTracer(logger *zap.Logger, maxTraces int) DistributedTracer {
	return &InMemoryTracer{
		logger:          logger,
		traces:          make(map[uuid.UUID]*Trace),
		activeSpans:     make(map[uuid.UUID]*inMemorySpan),
		maxTraces:       maxTraces,
		traceTimeout:    time.Hour,
		cleanupInterval: time.Minute * 10,
	}
}

func (t *InMemoryTracer) Start(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.running {
		return nil
	}

	t.running = true
	t.cleanupCtx, t.cleanupCancel = context.WithCancel(ctx)

	// Start cleanup goroutine
	t.cleanupWG.Add(1)
	go t.cleanupExpiredTraces()

	t.logger.Info("Distributed Tracer started")
	return nil
}

func (t *InMemoryTracer) Stop(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.running {
		return nil
	}

	t.running = false
	t.cleanupCancel()
	t.cleanupWG.Wait()

	t.logger.Info("Distributed Tracer stopped")
	return nil
}

func (t *InMemoryTracer) StartSpan(ctx context.Context, operationName string, options ...SpanOption) (Span, context.Context) {
	opts := &SpanOptions{
		Tags:      make(map[string]interface{}),
		StartTime: time.Now(),
	}

	for _, option := range options {
		option(opts)
	}

	var traceID uuid.UUID
	var parentSpanID *uuid.UUID

	// Check for parent span
	if parentSpan := t.SpanFromContext(ctx); parentSpan != nil {
		traceID = parentSpan.TraceID()
		spanID := parentSpan.SpanID()
		parentSpanID = &spanID
	} else if opts.ParentSpan != nil {
		traceID = opts.ParentSpan.TraceID()
		spanID := opts.ParentSpan.SpanID()
		parentSpanID = &spanID
	} else {
		traceID = uuid.New()
	}

	spanData := &SpanData{
		TraceID:       traceID,
		SpanID:        uuid.New(),
		ParentSpanID:  parentSpanID,
		OperationName: operationName,
		StartTime:     opts.StartTime,
		Tags:          opts.Tags,
		Logs:          make([]LogEntry, 0),
		Status:        SpanStatusOK,
		Service:       "integration", // Default service name
	}

	span := &inMemorySpan{
		tracer:   t,
		spanData: spanData,
		baggage:  make(map[string]string),
	}

	t.mu.Lock()
	t.activeSpans[spanData.SpanID] = span

	// Create or update trace
	if trace, exists := t.traces[traceID]; exists {
		trace.Spans = append(trace.Spans, spanData)
	} else {
		t.traces[traceID] = &Trace{
			TraceID:   traceID,
			Spans:     []*SpanData{spanData},
			StartTime: opts.StartTime,
			Tags:      make(map[string]interface{}),
			Services:  []string{"integration"},
			Status:    TraceStatusActive,
		}
	}
	t.mu.Unlock()

	newCtx := t.ContextWithSpan(ctx, span)
	return span, newCtx
}

func (t *InMemoryTracer) SpanFromContext(ctx context.Context) Span {
	if span, ok := ctx.Value(spanContextKey{}).(Span); ok {
		return span
	}
	return nil
}

func (t *InMemoryTracer) ContextWithSpan(ctx context.Context, span Span) context.Context {
	return context.WithValue(ctx, spanContextKey{}, span)
}

func (t *InMemoryTracer) GetTrace(ctx context.Context, traceID uuid.UUID) (*Trace, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	trace, exists := t.traces[traceID]
	if !exists {
		return nil, fmt.Errorf("trace not found: %s", traceID)
	}

	return trace, nil
}

func (t *InMemoryTracer) GetActiveTraces(ctx context.Context) ([]*Trace, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var active []*Trace
	for _, trace := range t.traces {
		if trace.Status == TraceStatusActive {
			active = append(active, trace)
		}
	}

	return active, nil
}

func (t *InMemoryTracer) HealthCheck(ctx context.Context) error {
	t.mu.RLock()
	running := t.running
	t.mu.RUnlock()

	if !running {
		return fmt.Errorf("tracer not running")
	}

	return nil
}

func (t *InMemoryTracer) cleanupExpiredTraces() {
	defer t.cleanupWG.Done()

	ticker := time.NewTicker(t.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			t.performCleanup()
		case <-t.cleanupCtx.Done():
			return
		}
	}
}

func (t *InMemoryTracer) performCleanup() {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	var expiredTraces []uuid.UUID

	for traceID, trace := range t.traces {
		if now.Sub(trace.StartTime) > t.traceTimeout {
			expiredTraces = append(expiredTraces, traceID)
		}
	}

	for _, traceID := range expiredTraces {
		delete(t.traces, traceID)
	}

	if len(expiredTraces) > 0 {
		t.logger.Debug("Cleaned up expired traces",
			zap.Int("count", len(expiredTraces)))
	}
}

// Span implementation
func (s *inMemorySpan) TraceID() uuid.UUID {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.spanData.TraceID
}

func (s *inMemorySpan) SpanID() uuid.UUID {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.spanData.SpanID
}

func (s *inMemorySpan) ParentSpanID() *uuid.UUID {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.spanData.ParentSpanID
}

func (s *inMemorySpan) SetTag(key string, value interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.spanData.Tags[key] = value
}

func (s *inMemorySpan) SetBaggageItem(key string, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.baggage[key] = value
}

func (s *inMemorySpan) GetBaggageItem(key string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.baggage[key]
}

func (s *inMemorySpan) LogEvent(event string, fields map[string]interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()

	logEntry := LogEntry{
		Timestamp: time.Now(),
		Event:     event,
		Fields:    fields,
	}

	s.spanData.Logs = append(s.spanData.Logs, logEntry)
}

func (s *inMemorySpan) LogError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.spanData.Status = SpanStatusError
	s.LogEvent("error", map[string]interface{}{
		"error": err.Error(),
	})
}

func (s *inMemorySpan) Finish() {
	s.FinishWithOptions(FinishOptions{
		FinishTime: time.Now(),
	})
}

func (s *inMemorySpan) FinishWithOptions(options FinishOptions) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.finished {
		return
	}

	s.finished = true
	s.spanData.EndTime = options.FinishTime
	s.spanData.Duration = s.spanData.EndTime.Sub(s.spanData.StartTime)

	// Update trace
	s.tracer.mu.Lock()
	if trace, exists := s.tracer.traces[s.spanData.TraceID]; exists {
		// Check if all spans in trace are finished
		allFinished := true
		for _, span := range trace.Spans {
			if span.EndTime.IsZero() {
				allFinished = false
				break
			}
		}

		if allFinished {
			trace.Status = TraceStatusCompleted
			trace.EndTime = options.FinishTime
			trace.Duration = trace.EndTime.Sub(trace.StartTime)
		}
	}

	// Remove from active spans
	delete(s.tracer.activeSpans, s.spanData.SpanID)
	s.tracer.mu.Unlock()
}

func (s *inMemorySpan) SetOperationName(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.spanData.OperationName = name
}

func (s *inMemorySpan) IsFinished() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.finished
}

func (s *inMemorySpan) Duration() time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.finished {
		return s.spanData.Duration
	}

	return time.Since(s.spanData.StartTime)
}

// Context key for spans
type spanContextKey struct{}

// Span option constructors
func WithParentSpan(parent Span) SpanOption {
	return func(opts *SpanOptions) {
		opts.ParentSpan = parent
	}
}

func WithTag(key string, value interface{}) SpanOption {
	return func(opts *SpanOptions) {
		opts.Tags[key] = value
	}
}

func WithStartTime(startTime time.Time) SpanOption {
	return func(opts *SpanOptions) {
		opts.StartTime = startTime
	}
}

// Helper functions for common tracing patterns
func TraceFunction(ctx context.Context, tracer DistributedTracer, operationName string, fn func(ctx context.Context) error) error {
	span, ctx := tracer.StartSpan(ctx, operationName)
	defer span.Finish()

	if err := fn(ctx); err != nil {
		span.LogError(err)
		span.SetTag("error", true)
		return err
	}

	return nil
}

func TraceOperation(ctx context.Context, tracer DistributedTracer, operationName string, tags map[string]interface{}, fn func(ctx context.Context) error) error {
	options := []SpanOption{}

	for key, value := range tags {
		options = append(options, WithTag(key, value))
	}

	span, ctx := tracer.StartSpan(ctx, operationName, options...)
	defer span.Finish()

	if err := fn(ctx); err != nil {
		span.LogError(err)
		span.SetTag("error", true)
		return err
	}

	return nil
}

// JSON marshaling for span data
func (s *SpanData) MarshalJSON() ([]byte, error) {
	type Alias SpanData
	return json.Marshal(&struct {
		*Alias
		Duration string `json:"duration_ms"`
	}{
		Alias:    (*Alias)(s),
		Duration: fmt.Sprintf("%.2f", float64(s.Duration.Nanoseconds())/1e6),
	})
}
