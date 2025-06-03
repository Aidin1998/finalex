//go:build testhelpers
// +build testhelpers

package test

import (
	"time"
)

// TestScenario represents a distributed transaction test scenario
type TestScenario struct {
	ID          string
	Name        string
	Description string
	Resources   []string // List of resource types to test
	Operations  []TestOperation
	Expected    TestExpectation
	Chaos       *ChaosConfig // Optional chaos engineering settings
}

// TestOperation represents an operation within a test scenario
type TestOperation struct {
	Resource    string
	Method      string
	Parameters  map[string]interface{}
	ExpectError bool
	Delay       time.Duration // Simulate processing delay
}

// TestExpectation defines expected test outcomes
type TestExpectation struct {
	ShouldCommit    bool
	ShouldRollback  bool
	DataConsistency map[string]interface{} // Expected final state
	Metrics         TestMetrics
}

// TestMetrics defines expected performance metrics
type TestMetrics struct {
	MaxDuration time.Duration
	MaxLockTime time.Duration
	ExpectedTPS float64
}

// ChaosConfig defines chaos engineering settings
type ChaosConfig struct {
	NetworkPartition bool
	DatabaseFailure  bool
	ServiceFailure   []string
	LatencyInject    time.Duration
	FailureRate      float64 // 0.0 to 1.0
}

// DistributedTransactionTester provides comprehensive testing capabilities
// ...rest of file...
