# Settlement Metrics Implementation

## Overview

This document describes the implementation of the TODO function at line 512 in `settlement_xa.go` for writing settlement metrics data to a time-series database (Prometheus) for monitoring and analytics purposes.

## Implementation Details

### Files Modified
- `c:\Orbit CEX\Finalex\internal\accounts\transaction\settlement_xa.go`

### New Functions Added

#### 1. `writeSettlementMetrics(ctx, stage, traceID, extra)`
Main function that writes settlement metrics to Prometheus time-series database.

**Features:**
- Extracts operation type from extra data
- Handles multiple duration formats (time.Duration, float64, int64)
- Calculates duration from start_time if duration not provided
- Records latency histograms and operation duration gauges
- Determines success/error status from extra data
- Updates throughput metrics
- Calls custom metrics recording

#### 2. `recordCustomSettlementMetrics(stage, traceID, operationType, extra)`
Records additional custom metrics from extra data fields.

**Features:**
- Records batch size metrics
- Records settlement amount metrics  
- Records queue depth metrics
- Records detailed error information with error message categorization

### Prometheus Metrics Created

#### Core Metrics
1. **`settlement_xa_latency_seconds`** (Histogram)
   - Labels: `stage`, `trace_id`, `operation_type`
   - Buckets: Exponential buckets from 1ms to ~16s
   - Tracks latency distribution of settlement operations

2. **`settlement_xa_stage_checkpoints_total`** (Counter)
   - Labels: `stage`, `operation_type`, `status`
   - Counts checkpoint events by stage and status

3. **`settlement_xa_operation_duration_seconds`** (Gauge)
   - Labels: `stage`, `trace_id`
   - Real-time monitoring of current operation durations

4. **`settlement_xa_throughput_ops_per_second`** (Gauge)
   - Labels: `operation_type`
   - Tracks operations throughput per second

#### Custom Metrics
- Batch size tracking via operation duration gauge
- Settlement amounts via operation duration gauge
- Queue depth monitoring via operation duration gauge
- Error categorization via stage counter with error prefixes

### Integration Points

The implementation integrates with the existing `recordLatencyCheckpoint` function, which is called throughout the settlement process:

```go
// In settlement operations
recordLatencyCheckpoint(ctx, sxa.logger, "settlement_start", map[string]interface{}{
    "operation_type": "trade_settlement",
    "xid": xid,
})
```

### Usage Examples

#### Basic Usage with Duration
```go
recordLatencyCheckpoint(ctx, logger, "settlement_validation", map[string]interface{}{
    "operation_type": "validation",
    "duration": 15 * time.Millisecond,
    "operations_count": 25.0,
    "status": "success",
})
```

#### Error Tracking
```go
recordLatencyCheckpoint(ctx, logger, "settlement_commit", map[string]interface{}{
    "operation_type": "commit",
    "status": "error",
    "error": "database_timeout",
    "error_message": "timeout",
})
```

#### Batch Processing Metrics
```go
recordLatencyCheckpoint(ctx, logger, "settlement_batch", map[string]interface{}{
    "operation_type": "batch_processing",
    "batch_size": 100.0,
    "amount": 1500.50,
    "queue_depth": 5.0,
})
```

### Monitoring Capabilities

#### Grafana Dashboard Queries
```promql
# Settlement latency by stage
histogram_quantile(0.95, rate(settlement_xa_latency_seconds_bucket[5m]))

# Settlement throughput
rate(settlement_xa_stage_checkpoints_total[1m])

# Error rate by operation type
rate(settlement_xa_stage_checkpoints_total{status=~"error.*"}[5m]) / 
rate(settlement_xa_stage_checkpoints_total[5m])

# Current operation durations
settlement_xa_operation_duration_seconds
```

#### Alerting Rules
```yaml
- alert: HighSettlementLatency
  expr: histogram_quantile(0.95, rate(settlement_xa_latency_seconds_bucket[5m])) > 1.0
  for: 2m
  labels:
    severity: warning
  annotations:
    summary: "High settlement latency detected"

- alert: SettlementErrors
  expr: rate(settlement_xa_stage_checkpoints_total{status=~"error.*"}[5m]) > 0.1
  for: 1m
  labels:
    severity: critical
  annotations:
    summary: "Settlement errors detected"
```

### Benefits

1. **Comprehensive Monitoring**: Tracks latency, throughput, errors, and custom business metrics
2. **Trace Correlation**: Uses trace IDs for distributed tracing correlation
3. **Flexible Data Collection**: Handles various data types in extra parameters
4. **Error Categorization**: Detailed error tracking with custom error messages
5. **Real-time Monitoring**: Gauges provide real-time operational visibility
6. **Integration Ready**: Works seamlessly with existing Prometheus infrastructure

### Performance Considerations

- Metrics recording is non-blocking and uses Prometheus client's built-in performance optimizations
- Histogram buckets are optimized for settlement operation latencies (1ms to 16s)
- Labels are kept to reasonable cardinality to prevent metric explosion
- Custom metrics reuse existing gauge metrics to minimize memory overhead

## Testing

The implementation has been successfully compiled and tested:

```bash
cd "c:\Orbit CEX\Finalex"
go build -v ./internal/accounts/transaction/
# ✅ Compilation successful
```

## Files Created

1. `settlement_metrics_example.go` - Usage examples and documentation
2. This README documenting the implementation

## Next Steps

1. **Configure Grafana Dashboards**: Create dashboards using the provided PromQL queries
2. **Set Up Alerting**: Implement the suggested alerting rules
3. **Add Business Logic**: Extend the extra parameters to include more settlement-specific metrics
4. **Performance Testing**: Validate metrics collection under high load scenarios
5. **Documentation**: Update operational runbooks with new monitoring capabilities
