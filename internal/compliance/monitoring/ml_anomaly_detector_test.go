package monitoring

import (
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestAdvancedMLAnomalyDetector(t *testing.T) {
	// Create logger
	logger := zap.NewNop() // No-op logger for testing

	// Create ML config
	config := DefaultMLConfig()
	config.AnomalyThreshold = 0.8 // Lower threshold for testing

	// Create detector
	detector := NewAdvancedMLAnomalyDetector(config, logger)
	defer detector.Shutdown()

	// Test normal transaction
	normalEvent := &ComplianceEvent{
		EventType: "transaction",
		UserID:    "user1",
		Timestamp: time.Now(),
		Amount:    100.0,
		Details:   map[string]interface{}{"type": "deposit"},
	}

	isAnomaly, score, features := detector.DetectAnomaly(normalEvent)
	t.Logf("Normal transaction - Anomaly: %t, Score: %.4f", isAnomaly, score)

	if features == nil {
		t.Error("Features should not be nil")
	}

	// Test anomalous large transaction
	anomalousEvent := &ComplianceEvent{
		EventType: "transaction",
		UserID:    "user1",
		Timestamp: time.Now(),
		Amount:    500000.0, // Large amount - should trigger anomaly
		Details:   map[string]interface{}{"type": "withdrawal"},
	}

	// Wait a bit for user profile to be established
	time.Sleep(100 * time.Millisecond)

	isAnomaly, score, features = detector.DetectAnomaly(anomalousEvent)
	t.Logf("Large transaction - Anomaly: %t, Score: %.4f", isAnomaly, score)

	if explanation, ok := features["explanation"]; ok {
		t.Logf("Explanation: %s", explanation)
	}

	// Test legacy interface
	legacyDetector := NewBuiltinMLAnomalyDetector(logger)

	testEvent := &ComplianceEvent{
		EventType: "transaction",
		UserID:    "legacy_user",
		Timestamp: time.Now(),
		Amount:    1000000.0, // Very large amount
		Details:   map[string]interface{}{"type": "transfer"},
	}

	isAnomaly, score, features = legacyDetector.DetectAnomaly(testEvent)
	t.Logf("Legacy Detection - Anomaly: %t, Score: %.4f", isAnomaly, score)

	t.Log("ML Anomaly Detector test completed successfully!")
}

func TestMLConfigDefaults(t *testing.T) {
	config := DefaultMLConfig()

	if config.AnomalyThreshold <= 0 || config.AnomalyThreshold > 1 {
		t.Errorf("Invalid anomaly threshold: %f", config.AnomalyThreshold)
	}

	if config.StatisticalThreshold <= 0 {
		t.Errorf("Invalid statistical threshold: %f", config.StatisticalThreshold)
	}

	if len(config.EnabledAlgorithms) == 0 {
		t.Error("No algorithms enabled")
	}

	t.Logf("Config validation passed: %+v", config)
}
