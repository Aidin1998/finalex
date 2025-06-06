// Enhanced structured logging with trace IDs for order and event tracking
package marketmaker

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// TraceID represents a unique identifier for tracking operations across the system
type TraceID string

// Generate a new trace ID
func NewTraceID() TraceID {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return TraceID(hex.EncodeToString(bytes))
}

func (t TraceID) String() string {
	return string(t)
}

// Context keys for trace propagation
type contextKey string

const (
	TraceIDContextKey contextKey = "trace_id"
	OrderIDContextKey contextKey = "order_id"
	PairContextKey    contextKey = "pair"
	StrategyContextKey contextKey = "strategy"
)

// Enhanced logger with structured fields and trace support
type StructuredLogger struct {
	logger   *zap.SugaredLogger
	baseLogger *zap.Logger
	service  string
	version  string
}

// NewStructuredLogger creates a new enhanced logger instance
func NewStructuredLogger(service, version string) (*StructuredLogger, error) {
	config := zap.NewProductionConfig()
	config.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	
	// Configure encoder for structured JSON logging
	config.EncoderConfig = zapcore.EncoderConfig{
		TimeKey:        "timestamp",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		FunctionKey:    "function",
		MessageKey:     "message",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.RFC3339NanoTimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	baseLogger, err := config.Build()
	if err != nil {
		return nil, fmt.Errorf("failed to build logger: %w", err)
	}

	// Add service-level fields
	baseLogger = baseLogger.With(
		zap.String("service", service),
		zap.String("version", version),
		zap.String("component", "marketmaker"),
	)

	return &StructuredLogger{
		logger:     baseLogger.Sugar(),
		baseLogger: baseLogger,
		service:    service,
		version:    version,
	}, nil
}

// WithTraceID adds trace ID to context
func WithTraceID(ctx context.Context, traceID TraceID) context.Context {
	return context.WithValue(ctx, TraceIDContextKey, traceID)
}

// WithOrderID adds order ID to context
func WithOrderID(ctx context.Context, orderID string) context.Context {
	return context.WithValue(ctx, OrderIDContextKey, orderID)
}

// WithPair adds trading pair to context
func WithPair(ctx context.Context, pair string) context.Context {
	return context.WithValue(ctx, PairContextKey, pair)
}

// WithStrategy adds strategy name to context
func WithStrategy(ctx context.Context, strategy string) context.Context {
	return context.WithValue(ctx, StrategyContextKey, strategy)
}

// GetTraceID extracts trace ID from context
func GetTraceID(ctx context.Context) TraceID {
	if traceID, ok := ctx.Value(TraceIDContextKey).(TraceID); ok {
		return traceID
	}
	return NewTraceID() // Generate new one if not found
}

// GetContextFields extracts all structured fields from context
func (sl *StructuredLogger) GetContextFields(ctx context.Context) []zap.Field {
	var fields []zap.Field
	
	if traceID, ok := ctx.Value(TraceIDContextKey).(TraceID); ok {
		fields = append(fields, zap.String("trace_id", traceID.String()))
	}
	
	if orderID, ok := ctx.Value(OrderIDContextKey).(string); ok {
		fields = append(fields, zap.String("order_id", orderID))
	}
	
	if pair, ok := ctx.Value(PairContextKey).(string); ok {
		fields = append(fields, zap.String("pair", pair))
	}
	
	if strategy, ok := ctx.Value(StrategyContextKey).(string); ok {
		fields = append(fields, zap.String("strategy", strategy))
	}
	
	return fields
}

// Order lifecycle logging methods with full context
func (sl *StructuredLogger) LogOrderPlaced(ctx context.Context, orderID, pair, side, orderType string, price, quantity float64) {
	fields := sl.GetContextFields(ctx)
	fields = append(fields,
		zap.String("event_type", "order_placed"),
		zap.String("order_id", orderID),
		zap.String("pair", pair),
		zap.String("side", side),
		zap.String("order_type", orderType),
		zap.Float64("price", price),
		zap.Float64("quantity", quantity),
		zap.Time("timestamp", time.Now()),
	)
	
	sl.baseLogger.Info("Order placed successfully", fields...)
}

func (sl *StructuredLogger) LogOrderCancelled(ctx context.Context, orderID, pair, reason string) {
	fields := sl.GetContextFields(ctx)
	fields = append(fields,
		zap.String("event_type", "order_cancelled"),
		zap.String("order_id", orderID),
		zap.String("pair", pair),
		zap.String("cancellation_reason", reason),
		zap.Time("timestamp", time.Now()),
	)
	
	sl.baseLogger.Info("Order cancelled", fields...)
}

func (sl *StructuredLogger) LogOrderFilled(ctx context.Context, orderID, pair string, fillPrice, fillQuantity, remainingQuantity float64) {
	fields := sl.GetContextFields(ctx)
	fields = append(fields,
		zap.String("event_type", "order_filled"),
		zap.String("order_id", orderID),
		zap.String("pair", pair),
		zap.Float64("fill_price", fillPrice),
		zap.Float64("fill_quantity", fillQuantity),
		zap.Float64("remaining_quantity", remainingQuantity),
		zap.Time("timestamp", time.Now()),
	)
	
	sl.baseLogger.Info("Order filled", fields...)
}

func (sl *StructuredLogger) LogOrderRejected(ctx context.Context, orderID, pair, reason string) {
	fields := sl.GetContextFields(ctx)
	fields = append(fields,
		zap.String("event_type", "order_rejected"),
		zap.String("order_id", orderID),
		zap.String("pair", pair),
		zap.String("rejection_reason", reason),
		zap.Time("timestamp", time.Now()),
	)
	
	sl.baseLogger.Error("Order rejected", fields...)
}

// Inventory movement logging
func (sl *StructuredLogger) LogInventoryChange(ctx context.Context, pair string, delta, newBalance float64, reason string) {
	fields := sl.GetContextFields(ctx)
	fields = append(fields,
		zap.String("event_type", "inventory_change"),
		zap.String("pair", pair),
		zap.Float64("delta", delta),
		zap.Float64("new_balance", newBalance),
		zap.String("reason", reason),
		zap.Time("timestamp", time.Now()),
	)
	
	sl.baseLogger.Info("Inventory changed", fields...)
}

// Strategy lifecycle logging
func (sl *StructuredLogger) LogStrategyStarted(ctx context.Context, strategyName, pair string, params map[string]interface{}) {
	fields := sl.GetContextFields(ctx)
	fields = append(fields,
		zap.String("event_type", "strategy_started"),
		zap.String("strategy_name", strategyName),
		zap.String("pair", pair),
		zap.Any("parameters", params),
		zap.Time("timestamp", time.Now()),
	)
	
	sl.baseLogger.Info("Strategy started", fields...)
}

func (sl *StructuredLogger) LogStrategyStopped(ctx context.Context, strategyName, pair, reason string) {
	fields := sl.GetContextFields(ctx)
	fields = append(fields,
		zap.String("event_type", "strategy_stopped"),
		zap.String("strategy_name", strategyName),
		zap.String("pair", pair),
		zap.String("stop_reason", reason),
		zap.Time("timestamp", time.Now()),
	)
	
	sl.baseLogger.Info("Strategy stopped", fields...)
}

// Risk event logging
func (sl *StructuredLogger) LogRiskEvent(ctx context.Context, eventType, severity, pair string, value float64, details map[string]interface{}) {
	fields := sl.GetContextFields(ctx)
	fields = append(fields,
		zap.String("event_type", "risk_event"),
		zap.String("risk_event_type", eventType),
		zap.String("severity", severity),
		zap.String("pair", pair),
		zap.Float64("value", value),
		zap.Any("details", details),
		zap.Time("timestamp", time.Now()),
	)
	
	level := zap.InfoLevel
	switch severity {
	case "warning":
		level = zap.WarnLevel
	case "error", "critical":
		level = zap.ErrorLevel
	}
	
	sl.baseLogger.Log(level, "Risk event detected", fields...)
}

// Feed health logging
func (sl *StructuredLogger) LogFeedEvent(ctx context.Context, feedType, provider, pair, eventType string, latency time.Duration) {
	fields := sl.GetContextFields(ctx)
	fields = append(fields,
		zap.String("event_type", "feed_event"),
		zap.String("feed_type", feedType),
		zap.String("provider", provider),
		zap.String("pair", pair),
		zap.String("feed_event_type", eventType),
		zap.Duration("latency", latency),
		zap.Time("timestamp", time.Now()),
	)
	
	sl.baseLogger.Info("Feed event", fields...)
}

func (sl *StructuredLogger) LogFeedDisconnection(ctx context.Context, feedType, provider, reason string) {
	fields := sl.GetContextFields(ctx)
	fields = append(fields,
		zap.String("event_type", "feed_disconnection"),
		zap.String("feed_type", feedType),
		zap.String("provider", provider),
		zap.String("disconnection_reason", reason),
		zap.Time("timestamp", time.Now()),
	)
	
	sl.baseLogger.Warn("Feed disconnected", fields...)
}

// Performance logging
func (sl *StructuredLogger) LogPerformanceMetric(ctx context.Context, metricName string, value float64, tags map[string]string) {
	fields := sl.GetContextFields(ctx)
	fields = append(fields,
		zap.String("event_type", "performance_metric"),
		zap.String("metric_name", metricName),
		zap.Float64("value", value),
		zap.Any("tags", tags),
		zap.Time("timestamp", time.Now()),
	)
	
	sl.baseLogger.Info("Performance metric", fields...)
}

// System event logging
func (sl *StructuredLogger) LogSystemEvent(ctx context.Context, component, eventType string, severity string, details map[string]interface{}) {
	fields := sl.GetContextFields(ctx)
	fields = append(fields,
		zap.String("event_type", "system_event"),
		zap.String("component", component),
		zap.String("system_event_type", eventType),
		zap.String("severity", severity),
		zap.Any("details", details),
		zap.Time("timestamp", time.Now()),
	)
	
	level := zap.InfoLevel
	switch severity {
	case "warning":
		level = zap.WarnLevel
	case "error", "critical":
		level = zap.ErrorLevel
	}
	
	sl.baseLogger.Log(level, "System event", fields...)
}

// Emergency event logging
func (sl *StructuredLogger) LogEmergencyStop(ctx context.Context, triggerReason, component string, details map[string]interface{}) {
	fields := sl.GetContextFields(ctx)
	fields = append(fields,
		zap.String("event_type", "emergency_stop"),
		zap.String("trigger_reason", triggerReason),
		zap.String("component", component),
		zap.Any("details", details),
		zap.Time("timestamp", time.Now()),
	)
	
	sl.baseLogger.Error("Emergency stop triggered", fields...)
}

// Latency logging with detailed breakdown
func (sl *StructuredLogger) LogLatencyBreakdown(ctx context.Context, operation string, totalDuration time.Duration, breakdown map[string]time.Duration) {
	fields := sl.GetContextFields(ctx)
	fields = append(fields,
		zap.String("event_type", "latency_breakdown"),
		zap.String("operation", operation),
		zap.Duration("total_duration", totalDuration),
		zap.Any("breakdown", breakdown),
		zap.Time("timestamp", time.Now()),
	)
	
	sl.baseLogger.Info("Latency breakdown", fields...)
}

// Batch operation logging
func (sl *StructuredLogger) LogBatchOperation(ctx context.Context, operationType string, batchSize int, duration time.Duration, successCount, failureCount int) {
	fields := sl.GetContextFields(ctx)
	fields = append(fields,
		zap.String("event_type", "batch_operation"),
		zap.String("operation_type", operationType),
		zap.Int("batch_size", batchSize),
		zap.Duration("duration", duration),
		zap.Int("success_count", successCount),
		zap.Int("failure_count", failureCount),
		zap.Time("timestamp", time.Now()),
	)
	
	sl.baseLogger.Info("Batch operation completed", fields...)
}

// Generic structured logging methods
func (sl *StructuredLogger) InfoWithContext(ctx context.Context, message string, fields ...zap.Field) {
	contextFields := sl.GetContextFields(ctx)
	allFields := append(contextFields, fields...)
	sl.baseLogger.Info(message, allFields...)
}

func (sl *StructuredLogger) WarnWithContext(ctx context.Context, message string, fields ...zap.Field) {
	contextFields := sl.GetContextFields(ctx)
	allFields := append(contextFields, fields...)
	sl.baseLogger.Warn(message, allFields...)
}

func (sl *StructuredLogger) ErrorWithContext(ctx context.Context, message string, fields ...zap.Field) {
	contextFields := sl.GetContextFields(ctx)
	allFields := append(contextFields, fields...)
	sl.baseLogger.Error(message, allFields...)
}

// LogInfo logs informational messages with structured fields
func (sl *StructuredLogger) LogInfo(ctx context.Context, message string, fields map[string]interface{}) {
	zapFields := sl.GetContextFields(ctx)
	for k, v := range fields {
		zapFields = append(zapFields, zap.Any(k, v))
	}
	
	sl.baseLogger.Info(message, zapFields...)
}

// LogError logs error messages with structured fields
func (sl *StructuredLogger) LogError(ctx context.Context, message string, fields map[string]interface{}) {
	zapFields := sl.GetContextFields(ctx)
	for k, v := range fields {
		zapFields = append(zapFields, zap.Any(k, v))
	}
	
	sl.baseLogger.Error(message, zapFields...)
}

// LogWarn logs warning messages with structured fields
func (sl *StructuredLogger) LogWarn(ctx context.Context, message string, fields map[string]interface{}) {
	zapFields := sl.GetContextFields(ctx)
	for k, v := range fields {
		zapFields = append(zapFields, zap.Any(k, v))
	}
	
	sl.baseLogger.Warn(message, zapFields...)
}

// LogDebug logs debug messages with structured fields
func (sl *StructuredLogger) LogDebug(ctx context.Context, message string, fields map[string]interface{}) {
	zapFields := sl.GetContextFields(ctx)
	for k, v := range fields {
		zapFields = append(zapFields, zap.Any(k, v))
	}
	
	sl.baseLogger.Debug(message, zapFields...)
}

// Close cleanly shuts down the logger
func (sl *StructuredLogger) Close() error {
	return sl.baseLogger.Sync()
}

// LoggerMiddleware for HTTP requests to add trace IDs
func (sl *StructuredLogger) LoggerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceID := NewTraceID()
		ctx := WithTraceID(r.Context(), traceID)
		
		// Add trace ID to response headers for client tracking
		w.Header().Set("X-Trace-ID", traceID.String())
		
		// Log the request
		sl.InfoWithContext(ctx, "HTTP request received",
			zap.String("method", r.Method),
			zap.String("path", r.URL.Path),
			zap.String("remote_addr", r.RemoteAddr),
			zap.String("user_agent", r.UserAgent()),
		)
		
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
