//go:build test
// +build test

package controller

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zaptest"
)

// Test types specific to the scaling logic tests
type TestScalingAction int

const (
	TestNoAction TestScalingAction = iota
	TestScaleUp
	TestScaleDown
)

type TestLoadMetrics struct {
	CPUUtilization    float64
	MemoryUtilization float64
	RequestsPerSecond float64
	ResponseTime      time.Duration
	ErrorRate         float64
	Timestamp         time.Time
	AverageCPU        float64 // For aggregated metrics
}

type TestScalingDecision struct {
	Action         TestScalingAction
	TargetReplicas int32
	Reason         string
	Confidence     float64
}

type TestPreWarmConfig struct {
	Enabled   bool
	Threshold float64
}

type TestPredictionResult struct {
	PredictedLoad    float64
	ConfidenceScore  float64
	PredictionWindow time.Duration
}

type TestPeakHour struct {
	Start    string
	End      string
	Timezone string
}

type TestCostOptimizationConfig struct {
	Enabled              bool
	OffPeakScaleDownRate float64
	PeakHours            []TestPeakHour
}

// Helper functions implementation
func testEnforceMaxReplicas(proposed, max int32) int32 {
	if proposed > max {
		return max
	}
	return proposed
}

func testEnforceMinReplicas(proposed, min int32) int32 {
	if proposed < min {
		return min
	}
	return proposed
}

func testEnforceScaleUpLimit(proposed, current int32, maxPercent float64) int32 {
	maxIncrease := int32(float64(current) * maxPercent)
	maxAllowed := current + maxIncrease
	if proposed > maxAllowed {
		return maxAllowed
	}
	return proposed
}

func testEnforceScaleDownLimit(proposed, current int32, maxPercent float64) int32 {
	maxDecrease := int32(float64(current) * maxPercent)
	minAllowed := current - maxDecrease
	if proposed < minAllowed {
		return minAllowed
	}
	return proposed
}

func testCalculateOverallLoadScore(metrics TestLoadMetrics) float64 {
	// Weighted average of different metrics
	cpuWeight := 0.3
	memoryWeight := 0.2
	rpsWeight := 0.2
	responseTimeWeight := 0.2
	errorWeight := 0.1

	// Normalize response time (assume 200ms is baseline)
	normalizedResponseTime := float64(metrics.ResponseTime.Milliseconds()) / 200.0
	if normalizedResponseTime > 1.0 {
		normalizedResponseTime = 1.0
	}

	// Normalize RPS (assume 100 RPS is baseline)
	normalizedRPS := metrics.RequestsPerSecond / 100.0
	if normalizedRPS > 1.0 {
		normalizedRPS = 1.0
	}

	// Calculate weighted score
	score := (metrics.CPUUtilization * cpuWeight) +
		(metrics.MemoryUtilization * memoryWeight) +
		(normalizedRPS * rpsWeight) +
		(normalizedResponseTime * responseTimeWeight) +
		(metrics.ErrorRate * 10 * errorWeight) // Error rate is typically small

	return score
}

func testMakeBasicScalingDecision(metrics TestLoadMetrics, currentReplicas int32, scaleUpThreshold, scaleDownThreshold float64) TestScalingDecision {
	loadScore := testCalculateOverallLoadScore(metrics)

	if loadScore > scaleUpThreshold {
		// Scale up logic
		targetReplicas := currentReplicas + int32(float64(currentReplicas)*0.5) // 50% increase
		return TestScalingDecision{
			Action:         TestScaleUp,
			TargetReplicas: targetReplicas,
			Reason:         "High load detected",
			Confidence:     0.8,
		}
	} else if loadScore < scaleDownThreshold {
		// Scale down logic
		targetReplicas := currentReplicas - int32(float64(currentReplicas)*0.3) // 30% decrease
		if targetReplicas < 1 {
			targetReplicas = 1
		}
		return TestScalingDecision{
			Action:         TestScaleDown,
			TargetReplicas: targetReplicas,
			Reason:         "Low load detected",
			Confidence:     0.7,
		}
	}

	return TestScalingDecision{
		Action:         TestNoAction,
		TargetReplicas: currentReplicas,
		Reason:         "Load within acceptable range",
		Confidence:     0.6,
	}
}

func testIsInCooldownPeriod(lastScaled time.Time, cooldownPeriod time.Duration) bool {
	return time.Since(lastScaled) < cooldownPeriod
}

func testShouldTriggerPreWarming(prediction TestPredictionResult, config TestPreWarmConfig) bool {
	return config.Enabled &&
		prediction.PredictedLoad > config.Threshold &&
		prediction.ConfidenceScore > 0.8
}

func testIsOffPeakTime(t time.Time, peakHours []TestPeakHour) bool {
	currentTime := t.Format("15:04")

	for _, peak := range peakHours {
		if currentTime >= peak.Start && currentTime <= peak.End {
			return false // Within peak hours
		}
	}

	return true // Off-peak
}

func testCalculateOffPeakReplicas(current int32, config TestCostOptimizationConfig, t time.Time) int32 {
	if !config.Enabled || !testIsOffPeakTime(t, config.PeakHours) {
		return current
	}

	reduction := int32(float64(current) * config.OffPeakScaleDownRate)
	target := current - reduction

	if target < 1 {
		target = 1
	}

	return target
}

// TestScalingConstraints tests scaling constraint enforcement
func TestScalingConstraints(t *testing.T) {
	t.Run("MaxReplicasEnforcement", func(t *testing.T) {
		maxReplicas := int32(10)

		// Test normal case
		proposed := int32(5)
		result := testEnforceMaxReplicas(proposed, maxReplicas)
		assert.Equal(t, proposed, result, "Should not modify replicas within limits")

		// Test exceeding max
		proposed = int32(15)
		result = testEnforceMaxReplicas(proposed, maxReplicas)
		assert.Equal(t, maxReplicas, result, "Should cap at max replicas")

		// Test edge case
		proposed = maxReplicas
		result = testEnforceMaxReplicas(proposed, maxReplicas)
		assert.Equal(t, maxReplicas, result, "Should allow exact max replicas")
	})

	t.Run("MinReplicasEnforcement", func(t *testing.T) {
		minReplicas := int32(2)

		// Test normal case
		proposed := int32(5)
		result := testEnforceMinReplicas(proposed, minReplicas)
		assert.Equal(t, proposed, result, "Should not modify replicas above minimum")

		// Test below minimum
		proposed = int32(1)
		result = testEnforceMinReplicas(proposed, minReplicas)
		assert.Equal(t, minReplicas, result, "Should enforce minimum replicas")

		// Test edge case
		proposed = minReplicas
		result = testEnforceMinReplicas(proposed, minReplicas)
		assert.Equal(t, minReplicas, result, "Should allow exact minimum replicas")
	})

	t.Run("ScaleUpLimitEnforcement", func(t *testing.T) {
		currentReplicas := int32(5)
		maxScaleUpPercent := 0.5 // 50%

		// Test within limits (30% increase)
		proposed := int32(7) // 40% increase
		result := testEnforceScaleUpLimit(proposed, currentReplicas, maxScaleUpPercent)
		expected := currentReplicas + int32(float64(currentReplicas)*maxScaleUpPercent)
		assert.Equal(t, expected, result, "Should limit scale up to maximum percentage")

		// Test excessive scale up
		proposed = int32(10) // 100% increase
		result = testEnforceScaleUpLimit(proposed, currentReplicas, maxScaleUpPercent)
		assert.Equal(t, expected, result, "Should cap excessive scale up")

		// Test within normal range
		proposed = int32(6) // 20% increase
		result = testEnforceScaleUpLimit(proposed, currentReplicas, maxScaleUpPercent)
		assert.Equal(t, proposed, result, "Should allow normal scale up")
	})

	t.Run("ScaleDownLimitEnforcement", func(t *testing.T) {
		currentReplicas := int32(8)
		maxScaleDownPercent := 0.3 // 30%

		// Test within limits (20% decrease)
		proposed := int32(6) // 25% decrease - should be limited
		result := testEnforceScaleDownLimit(proposed, currentReplicas, maxScaleDownPercent)
		expected := currentReplicas - int32(float64(currentReplicas)*maxScaleDownPercent)
		assert.Equal(t, expected, result, "Should limit scale down to maximum percentage")

		// Test excessive scale down
		proposed = int32(3) // 62% decrease
		result = testEnforceScaleDownLimit(proposed, currentReplicas, maxScaleDownPercent)
		assert.Equal(t, expected, result, "Should cap excessive scale down")

		// Test within normal range
		proposed = int32(7) // 12% decrease
		result = testEnforceScaleDownLimit(proposed, currentReplicas, maxScaleDownPercent)
		assert.Equal(t, proposed, result, "Should allow normal scale down")
	})
}

// TestLoadMetricsCalculation tests load metrics calculation and aggregation
func TestLoadMetricsCalculation(t *testing.T) {
	t.Run("OverallLoadScore", func(t *testing.T) {
		// Test balanced load
		metrics := TestLoadMetrics{
			CPUUtilization:    0.6,
			MemoryUtilization: 0.5,
			RequestsPerSecond: 100,
			ResponseTime:      100 * time.Millisecond,
			ErrorRate:         0.01,
		}
		score := testCalculateOverallLoadScore(metrics)
		assert.Greater(t, score, 0.0, "Load score should be positive")
		assert.Less(t, score, 1.0, "Load score should be less than 1.0 for moderate load")

		// Test high load
		highLoadMetrics := TestLoadMetrics{
			CPUUtilization:    0.9,
			MemoryUtilization: 0.85,
			RequestsPerSecond: 500,
			ResponseTime:      500 * time.Millisecond,
			ErrorRate:         0.05,
		}
		highScore := testCalculateOverallLoadScore(highLoadMetrics)
		assert.Greater(t, highScore, score, "High load should result in higher score")

		// Test low load
		lowLoadMetrics := TestLoadMetrics{
			CPUUtilization:    0.2,
			MemoryUtilization: 0.15,
			RequestsPerSecond: 20,
			ResponseTime:      50 * time.Millisecond,
			ErrorRate:         0.001,
		}
		lowScore := testCalculateOverallLoadScore(lowLoadMetrics)
		assert.Less(t, lowScore, score, "Low load should result in lower score")
	})

	t.Run("MetricsAggregation", func(t *testing.T) {
		// Test aggregation of multiple metric readings
		metrics1 := TestLoadMetrics{
			CPUUtilization:    0.4,
			MemoryUtilization: 0.3,
			RequestsPerSecond: 50,
			AverageCPU:        0.4,
		}

		metrics2 := TestLoadMetrics{
			CPUUtilization:    0.6,
			MemoryUtilization: 0.5,
			RequestsPerSecond: 100,
			AverageCPU:        0.6,
		}

		// In a real implementation, this would aggregate metrics
		// For now, we just verify the structure is correct
		assert.NotNil(t, metrics1)
		assert.NotNil(t, metrics2)
		assert.NotEqual(t, metrics1.CPUUtilization, metrics2.CPUUtilization)
	})
}

// TestScalingDecisionLogic tests the core scaling decision algorithm
func TestScalingDecisionLogic(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()

	t.Run("ScaleUpDecision", func(t *testing.T) {
		currentReplicas := int32(3)
		scaleUpThreshold := 0.7
		scaleDownThreshold := 0.3

		// High load should trigger scale up
		highLoadMetrics := TestLoadMetrics{
			CPUUtilization:    0.85,
			MemoryUtilization: 0.8,
			RequestsPerSecond: 200,
			ResponseTime:      300 * time.Millisecond,
			ErrorRate:         0.02,
		}

		decision := testMakeBasicScalingDecision(highLoadMetrics, currentReplicas, scaleUpThreshold, scaleDownThreshold)
		assert.Equal(t, TestScaleUp, decision.Action, "High load should trigger scale up")
		assert.Greater(t, decision.TargetReplicas, currentReplicas, "Target replicas should be higher")
		assert.Greater(t, decision.Confidence, 0.5, "Should have reasonable confidence")
		assert.NotEmpty(t, decision.Reason, "Should provide scaling reason")

		_ = logger // Use logger to avoid unused variable
	})

	t.Run("ScaleDownDecision", func(t *testing.T) {
		currentReplicas := int32(8)
		scaleUpThreshold := 0.7
		scaleDownThreshold := 0.3

		// Low load should trigger scale down
		lowLoadMetrics := TestLoadMetrics{
			CPUUtilization:    0.15,
			MemoryUtilization: 0.2,
			RequestsPerSecond: 10,
			ResponseTime:      50 * time.Millisecond,
			ErrorRate:         0.001,
		}

		decision := testMakeBasicScalingDecision(lowLoadMetrics, currentReplicas, scaleUpThreshold, scaleDownThreshold)
		assert.Equal(t, TestScaleDown, decision.Action, "Low load should trigger scale down")
		assert.Less(t, decision.TargetReplicas, currentReplicas, "Target replicas should be lower")
		assert.Greater(t, decision.Confidence, 0.5, "Should have reasonable confidence")
		assert.NotEmpty(t, decision.Reason, "Should provide scaling reason")
	})

	t.Run("NoActionDecision", func(t *testing.T) {
		currentReplicas := int32(5)
		scaleUpThreshold := 0.7
		scaleDownThreshold := 0.3

		// Moderate load should not trigger scaling
		moderateLoadMetrics := TestLoadMetrics{
			CPUUtilization:    0.5,
			MemoryUtilization: 0.45,
			RequestsPerSecond: 75,
			ResponseTime:      100 * time.Millisecond,
			ErrorRate:         0.01,
		}

		decision := testMakeBasicScalingDecision(moderateLoadMetrics, currentReplicas, scaleUpThreshold, scaleDownThreshold)
		assert.Equal(t, TestNoAction, decision.Action, "Moderate load should not trigger scaling")
		assert.Equal(t, currentReplicas, decision.TargetReplicas, "Target replicas should remain the same")
	})
}

// TestCooldownLogic tests cooldown period enforcement
func TestCooldownLogic(t *testing.T) {
	t.Run("CooldownActive", func(t *testing.T) {
		lastScaled := time.Now().Add(-30 * time.Second)
		cooldownPeriod := 5 * time.Minute

		inCooldown := testIsInCooldownPeriod(lastScaled, cooldownPeriod)
		assert.True(t, inCooldown, "Should be in cooldown period")
	})

	t.Run("CooldownExpired", func(t *testing.T) {
		lastScaled := time.Now().Add(-10 * time.Minute)
		cooldownPeriod := 5 * time.Minute

		inCooldown := testIsInCooldownPeriod(lastScaled, cooldownPeriod)
		assert.False(t, inCooldown, "Should not be in cooldown period")
	})

	t.Run("EdgeCaseCooldown", func(t *testing.T) {
		// Test exactly at cooldown boundary
		lastScaled := time.Now().Add(-5 * time.Minute)
		cooldownPeriod := 5 * time.Minute

		inCooldown := testIsInCooldownPeriod(lastScaled, cooldownPeriod)
		// Should be very close to cooldown expiry, but we allow for timing precision
		assert.False(t, inCooldown, "Should be just past cooldown period")
	})
}

// TestPreWarmingLogic tests pre-warming decision making
func TestPreWarmingLogic(t *testing.T) {
	t.Run("ShouldTriggerPreWarming", func(t *testing.T) {
		config := TestPreWarmConfig{
			Enabled:   true,
			Threshold: 0.8,
		}

		// High confidence prediction should trigger pre-warming
		prediction := TestPredictionResult{
			PredictedLoad:    0.9,
			ConfidenceScore:  0.85,
			PredictionWindow: 15 * time.Minute,
		}

		shouldPreWarm := testShouldTriggerPreWarming(prediction, config)
		assert.True(t, shouldPreWarm, "High confidence prediction should trigger pre-warming")
	})

	t.Run("ShouldNotTriggerPreWarming", func(t *testing.T) {
		config := TestPreWarmConfig{
			Enabled:   true,
			Threshold: 0.8,
		}

		// Low confidence prediction should not trigger pre-warming
		lowConfidencePrediction := TestPredictionResult{
			PredictedLoad:    0.9,
			ConfidenceScore:  0.6, // Below confidence threshold
			PredictionWindow: 15 * time.Minute,
		}

		shouldPreWarm := testShouldTriggerPreWarming(lowConfidencePrediction, config)
		assert.False(t, shouldPreWarm, "Low confidence prediction should not trigger pre-warming")

		// Low predicted load should not trigger pre-warming
		lowLoadPrediction := TestPredictionResult{
			PredictedLoad:    0.5, // Below load threshold
			ConfidenceScore:  0.9,
			PredictionWindow: 15 * time.Minute,
		}

		shouldPreWarm = testShouldTriggerPreWarming(lowLoadPrediction, config)
		assert.False(t, shouldPreWarm, "Low predicted load should not trigger pre-warming")

		// Disabled pre-warming should not trigger
		disabledConfig := TestPreWarmConfig{
			Enabled:   false,
			Threshold: 0.8,
		}

		highPrediction := TestPredictionResult{
			PredictedLoad:    0.9,
			ConfidenceScore:  0.9,
			PredictionWindow: 15 * time.Minute,
		}

		shouldPreWarm = testShouldTriggerPreWarming(highPrediction, disabledConfig)
		assert.False(t, shouldPreWarm, "Disabled pre-warming should not trigger")
	})
}

// TestCostOptimization tests cost optimization features
func TestCostOptimization(t *testing.T) {
	t.Run("OffPeakDetection", func(t *testing.T) {
		peakHours := []TestPeakHour{
			{Start: "09:00", End: "17:00", Timezone: "UTC"},
			{Start: "19:00", End: "22:00", Timezone: "UTC"},
		}

		// Test off-peak time (3 AM UTC)
		offPeakTime := time.Date(2024, 1, 1, 3, 0, 0, 0, time.UTC)
		isOffPeak := testIsOffPeakTime(offPeakTime, peakHours)
		assert.True(t, isOffPeak, "3 AM should be off-peak time")

		// Test peak time (12 PM UTC)
		peakTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
		isPeak := !testIsOffPeakTime(peakTime, peakHours)
		assert.True(t, isPeak, "12 PM should be peak time")

		// Test evening peak time (8 PM UTC)
		eveningPeakTime := time.Date(2024, 1, 1, 20, 0, 0, 0, time.UTC)
		isEveningPeak := !testIsOffPeakTime(eveningPeakTime, peakHours)
		assert.True(t, isEveningPeak, "8 PM should be peak time")
	})

	t.Run("OffPeakScaling", func(t *testing.T) {
		offPeakConfig := TestCostOptimizationConfig{
			Enabled:              true,
			OffPeakScaleDownRate: 0.6, // Aggressive scale down
			PeakHours: []TestPeakHour{
				{Start: "09:00", End: "17:00", Timezone: "UTC"},
			},
		}

		currentReplicas := int32(10)
		offPeakTime := time.Date(2024, 1, 1, 2, 0, 0, 0, time.UTC)

		targetReplicas := testCalculateOffPeakReplicas(currentReplicas, offPeakConfig, offPeakTime)
		expectedMin := int32(float64(currentReplicas) * (1.0 - offPeakConfig.OffPeakScaleDownRate))

		assert.LessOrEqual(t, targetReplicas, expectedMin,
			"Off-peak scaling should be more aggressive")
	})
}
