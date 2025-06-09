// Example usage of the implemented settlement metrics functionality
package transaction

import (
	"context"
	"time"

	"go.uber.org/zap"
)

// ExampleSettlementMetricsUsage demonstrates how the implemented metrics work
func ExampleSettlementMetricsUsage() {
	logger := zap.NewExample()
	ctx := context.WithValue(context.Background(), TraceIDKey, "example-trace-123")

	// Example 1: Recording a settlement start with duration
	startTime := time.Now()
	recordLatencyCheckpoint(ctx, logger, "settlement_start", map[string]interface{}{
		"operation_type": "trade_settlement",
		"start_time":     startTime,
		"batch_size":     100.0,
		"amount":         1500.50,
	})

	// Simulate some processing time
	time.Sleep(10 * time.Millisecond)

	// Example 2: Recording settlement validation with explicit duration
	recordLatencyCheckpoint(ctx, logger, "settlement_validation", map[string]interface{}{
		"operation_type":   "validation",
		"duration":         15 * time.Millisecond,
		"operations_count": 25.0,
		"queue_depth":      5.0,
		"status":           "success",
	})

	// Example 3: Recording an error scenario
	recordLatencyCheckpoint(ctx, logger, "settlement_commit", map[string]interface{}{
		"operation_type": "commit",
		"duration":       5 * time.Millisecond,
		"status":         "error",
		"error":          "database_timeout",
		"error_message":  "timeout",
	})

	// Example 4: Recording successful completion
	recordLatencyCheckpoint(ctx, logger, "settlement_complete", map[string]interface{}{
		"operation_type":   "completion",
		"duration":         time.Since(startTime),
		"operations_count": 100.0,
		"amount":           1500.50,
		"status":           "success",
	})
}

// This example shows the metrics that will be generated:
//
// Prometheus Metrics Generated:
// 1. settlement_xa_latency_seconds{stage="settlement_start",trace_id="example-trace-123",operation_type="trade_settlement"}
// 2. settlement_xa_stage_checkpoints_total{stage="settlement_start",operation_type="trade_settlement",status="success"}
// 3. settlement_xa_operation_duration_seconds{stage="settlement_start",trace_id="example-trace-123"}
// 4. settlement_xa_throughput_ops_per_second{operation_type="trade_settlement"}
//
// Custom metrics for additional context:
// - settlement_xa_operation_duration_seconds{stage="settlement_start_batch_size",trace_id="example-trace-123"}
// - settlement_xa_operation_duration_seconds{stage="settlement_start_amount",trace_id="example-trace-123"}
// - settlement_xa_operation_duration_seconds{stage="settlement_validation_queue_depth",trace_id="example-trace-123"}
// - settlement_xa_stage_checkpoints_total{stage="settlement_commit",operation_type="commit",status="error_timeout"}
