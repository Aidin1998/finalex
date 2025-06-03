//go:build test
// +build test

package backpressure

import (
	"context"

	"go.uber.org/zap"
)

// NewBackpressureManagerForTest creates a BackpressureManager with a dummy coordinator (no Kafka)
func NewBackpressureManagerForTest(cfg *BackpressureConfig, logger *zap.Logger) *BackpressureManager {
	managerCfg := &cfg.Manager
	detector := NewClientCapabilityDetector(logger.Named("detector"))
	rateLimiter := NewAdaptiveRateLimiter(logger.Named("rate_limiter"), detector, &cfg.RateLimiter)
	priorityQ := NewLockFreePriorityQueue(&cfg.PriorityQueue, logger.Named("priority_queue"))
	dummyCoord := &CrossServiceCoordinator{} // No Kafka
	ctx, cancel := context.WithCancel(context.Background())
	return &BackpressureManager{
		logger:           logger,
		detector:         detector,
		rateLimiter:      rateLimiter,
		priorityQ:        priorityQ,
		coordinator:      dummyCoord,
		incomingMessages: make(chan *IncomingMessage, 10000),
		processedJobs:    make(chan *ProcessedJob, 10000),
		workerCount:      managerCfg.WorkerCount,
		config:           managerCfg,
		ctx:              ctx,
		cancel:           cancel,
		shutdown:         make(chan struct{}),
		metrics:          initManagerMetrics(),
	}
}

// SetCoordinator allows tests to inject a mock or dummy coordinator.
func (m *BackpressureManager) SetCoordinator(coord *CrossServiceCoordinator) {
	m.coordinator = coord
}
