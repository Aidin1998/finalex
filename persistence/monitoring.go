// Monitoring and metrics for async persistence layer.
package persistence

type BufferMetrics struct{}

func (m *BufferMetrics) SetBufferSize(size int) {}

// WriterMetrics tracks batch/flush stats and errors
// (implement with atomic counters, Prometheus, etc.)
type WriterMetrics struct{}

func (m *WriterMetrics) RecordBatch(size int)    {}
func (m *WriterMetrics) IncWriteErrors()         {}
func (m *WriterMetrics) IncCircuitBreakerTrips() {}

// ReconciliationMetrics for reconciliation service
// (implement as needed)
type ReconciliationMetrics struct{}
