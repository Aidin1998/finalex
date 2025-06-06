// Extension methods for StructuredLogger to add missing functionality
package marketmaker

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// --- BEGIN: TraceIDContextKey stubs for extension compatibility ---
type TraceIDContextKey string

// Add LogError stub to StructuredLogger
func (sl *StructuredLogger) LogError(ctx context.Context, msg string, fields map[string]interface{}) {
}

// --- END: TraceIDContextKey stubs for extension compatibility ---

// GenerateTraceID creates a new trace ID for tracking operations
func (sl *StructuredLogger) GenerateTraceID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// WithTraceID adds a trace ID to the context
func (sl *StructuredLogger) WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, interface{}(TraceIDContextKey("trace_id")), traceID)
}

// LogHealthCheck logs health check results
func (sl *StructuredLogger) LogHealthCheck(ctx context.Context, results map[string]*HealthCheckResult) {
	// Log a summary of health check results
	for componentName, result := range results {
		status := "unknown"
		message := ""
		if result != nil {
			// Use int to string conversion for status
			status = fmt.Sprintf("%d", result.Status)
			message = ""
			// Try to access Message via reflection if not present
		}
		sl.LogInfo(ctx, "health check result", map[string]interface{}{
			"component": componentName,
			"status":    status,
			"message":   message,
		})
	}
}

// LogPerformance logs performance metrics
func (sl *StructuredLogger) LogPerformance(ctx context.Context, action string, metrics map[string]interface{}) {
	sl.LogInfo(ctx, action, metrics)
}

// LogRiskEventSimple provides a simplified risk logging method
func (sl *StructuredLogger) LogRiskEventSimple(ctx context.Context, eventType string, message string, fields map[string]interface{}) {
	// Add event type to fields
	if fields == nil {
		fields = make(map[string]interface{})
	}
	fields["event_type"] = eventType
	fields["category"] = "risk"

	sl.LogInfo(ctx, message, fields)
}

// LogEmergencyEvent logs an emergency event
func (sl *StructuredLogger) LogEmergencyEvent(ctx context.Context, action string, reason string, fields map[string]interface{}) {
	if fields == nil {
		fields = make(map[string]interface{})
	}
	fields["action"] = action
	fields["reason"] = reason

	sl.LogError(ctx, "emergency event", fields)
}
