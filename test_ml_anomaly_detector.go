package main

import (
	"fmt"
	"log"
	"time"

	"go.uber.org/zap"

	"github.com/Aidin1998/finalex/internal/compliance/monitoring"
)

func main() {
	// Create logger
	logger, err := zap.NewDevelopment()
	if err != nil {
		log.Fatal("Failed to create logger:", err)
	}
	defer logger.Sync()

	// Test the new advanced ML anomaly detector
	fmt.Println("Testing Advanced ML Anomaly Detector...")

	// Create ML config
	config := monitoring.DefaultMLConfig()
	config.AnomalyThreshold = 0.8 // Lower threshold for testing

	// Create detector
	detector := monitoring.NewAdvancedMLAnomalyDetector(config, logger)
	defer detector.Shutdown()

	// Test events
	testEvents := []*monitoring.ComplianceEvent{
		{
			EventType: "transaction",
			UserID:    "user1",
			Timestamp: time.Now(),
			Amount:    100.0,
			Details:   map[string]interface{}{"type": "deposit"},
		},
		{
			EventType: "transaction",
			UserID:    "user1",
			Timestamp: time.Now(),
			Amount:    500000.0, // Large amount - should trigger anomaly
			Details:   map[string]interface{}{"type": "withdrawal"},
		},
		{
			EventType: "login",
			UserID:    "user2",
			Timestamp: time.Now().Add(-12 * time.Hour), // Unusual time
			Amount:    0.0,
			Details:   map[string]interface{}{"ip": "192.168.1.1"},
		},
	}

	// Process test events
	for i, event := range testEvents {
		fmt.Printf("\n--- Test Event %d ---\n", i+1)
		fmt.Printf("Event: %s, User: %s, Amount: %.2f\n", event.EventType, event.UserID, event.Amount)

		isAnomaly, score, features := detector.DetectAnomaly(event)

		fmt.Printf("Anomaly: %t, Score: %.4f\n", isAnomaly, score)
		if explanation, ok := features["explanation"]; ok {
			fmt.Printf("Explanation: %s\n", explanation)
		}
		if methods, ok := features["detection_methods"]; ok {
			fmt.Printf("Detection Methods: %+v\n", methods)
		}

		// Give some time for background processing
		time.Sleep(100 * time.Millisecond)
	}

	// Test legacy interface
	fmt.Println("\n--- Testing Legacy Interface ---")
	legacyDetector := monitoring.NewBuiltinMLAnomalyDetector(logger)

	testEvent := &monitoring.ComplianceEvent{
		EventType: "transaction",
		UserID:    "legacy_user",
		Timestamp: time.Now(),
		Amount:    1000000.0, // Very large amount
		Details:   map[string]interface{}{"type": "transfer"},
	}
	isAnomaly, score, _ := legacyDetector.DetectAnomaly(testEvent)
	fmt.Printf("Legacy Detection - Anomaly: %t, Score: %.4f\n", isAnomaly, score)

	fmt.Println("\nML Anomaly Detector test completed successfully!")
}
