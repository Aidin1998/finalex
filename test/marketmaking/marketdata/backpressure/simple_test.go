//go:build test
// +build test

package backpressure

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

// TestSimpleBackpressureManager tests basic BackpressureManager functionality
func TestSimpleBackpressureManager(t *testing.T) {
	logger := zaptest.NewLogger(t)

	// Create a minimal config
	config := &BackpressureConfig{
		Manager: ManagerConfig{
			WorkerCount: 2,
		},
		RateLimiter:   RateLimiterConfig{},
		PriorityQueue: PriorityQueueConfig{},
		Coordinator:   CoordinatorConfig{},
	}

	// Create manager using test function
	manager := NewBackpressureManagerForTest(config, logger)
	require.NotNil(t, manager, "Manager should not be nil")

	// Test that GetMetrics doesn't panic
	metrics := manager.GetMetrics()
	assert.NotNil(t, metrics, "Metrics should not be nil")

	t.Logf("Test passed - metrics: %+v", metrics)
}
