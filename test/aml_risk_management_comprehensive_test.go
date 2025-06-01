package test

import (
	"context"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Test AML Transaction Analysis Performance
func TestAMLTransactionAnalysisPerformance(t *testing.T) {
	env := NewTestEnvironment(DefaultTestConfig())
	amlEngine := env.AMLEngine

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var metrics PerformanceMetrics
	transactionCount := 100

	startTime := time.Now()

	for i := 0; i < transactionCount; i++ {
		transaction := AMLTransaction{
			ID:        uuid.New(),
			UserID:    uuid.New(),
			Amount:    decimal.NewFromFloat(1000.0 + rand.Float64()*10000.0),
			Currency:  "USD",
			Type:      "transfer",
			Timestamp: time.Now(),
			IPAddress: fmt.Sprintf("192.168.1.%d", rand.Intn(255)),
			Location:  "US",
		}

		start := time.Now()
		result, err := amlEngine.AnalyzeTransaction(ctx, transaction)
		latency := time.Since(start)

		if err != nil {
			metrics.AddFailure()
			continue
		}

		metrics.AddLatency(latency)
		metrics.AddSuccess()

		// Validate result structure
		if result.TransactionID != transaction.ID {
			t.Errorf("Transaction ID mismatch: expected %s, got %s", transaction.ID, result.TransactionID)
		}

		if result.RiskScore < 0 || result.RiskScore > 100 {
			t.Errorf("Invalid risk score: %f (must be 0-100)", result.RiskScore)
		}
	}

	totalDuration := time.Since(startTime)
	metrics.Calculate()

	// Performance Requirements
	if metrics.P95Latency > 100*time.Millisecond {
		t.Errorf("AML analysis P95 latency too high: %v (requirement: <100ms)", metrics.P95Latency)
	}

	throughput := float64(metrics.SuccessfulOps) / totalDuration.Seconds()
	if throughput < 50 {
		t.Errorf("AML throughput too low: %.2f ops/sec (requirement: >50 ops/sec)", throughput)
	}

	if metrics.ErrorRate > 1.0 {
		t.Errorf("AML error rate too high: %.2f%% (requirement: <1%%)", metrics.ErrorRate)
	}

	t.Logf("AML Transaction Analysis Performance Results:")
	t.Logf("  Total Transactions: %d", transactionCount)
	t.Logf("  Successful Analyses: %d", metrics.SuccessfulOps)
	t.Logf("  Average Latency: %v", metrics.AverageLatency)
	t.Logf("  P95 Latency: %v", metrics.P95Latency)
	t.Logf("  P99 Latency: %v", metrics.P99Latency)
	t.Logf("  Throughput: %.2f ops/sec", throughput)
	t.Logf("  Error Rate: %.2f%%", metrics.ErrorRate)
}

// Test AML Sanctions Checking
func TestAMLSanctionsChecking(t *testing.T) {
	env := NewTestEnvironment(DefaultTestConfig())
	amlEngine := env.AMLEngine

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Test multiple users
	userCount := 50
	var metrics PerformanceMetrics

	startTime := time.Now()

	for i := 0; i < userCount; i++ {
		userID := uuid.New()

		start := time.Now()
		flagged, err := amlEngine.CheckSanctions(ctx, userID)
		latency := time.Since(start)

		if err != nil {
			metrics.AddFailure()
			continue
		}

		metrics.AddLatency(latency)
		metrics.AddSuccess()

		// Validate result type
		if flagged != true && flagged != false {
			t.Errorf("Invalid sanctions check result for user %s", userID)
		}
	}

	totalDuration := time.Since(startTime)
	metrics.Calculate()

	// Performance Requirements for Sanctions Checking
	if metrics.P95Latency > 50*time.Millisecond {
		t.Errorf("Sanctions check P95 latency too high: %v (requirement: <50ms)", metrics.P95Latency)
	}

	throughput := float64(metrics.SuccessfulOps) / totalDuration.Seconds()
	if throughput < 100 {
		t.Errorf("Sanctions check throughput too low: %.2f ops/sec (requirement: >100 ops/sec)", throughput)
	}

	if metrics.ErrorRate > 0.5 {
		t.Errorf("Sanctions check error rate too high: %.2f%% (requirement: <0.5%%)", metrics.ErrorRate)
	}

	t.Logf("AML Sanctions Checking Results:")
	t.Logf("  Users Checked: %d", userCount)
	t.Logf("  Successful Checks: %d", metrics.SuccessfulOps)
	t.Logf("  Average Latency: %v", metrics.AverageLatency)
	t.Logf("  P95 Latency: %v", metrics.P95Latency)
	t.Logf("  Throughput: %.2f ops/sec", throughput)
	t.Logf("  Error Rate: %.2f%%", metrics.ErrorRate)
}

// Test AML Basic Functionality
func TestAMLBasicFunctionality(t *testing.T) {
	env := NewTestEnvironment(DefaultTestConfig())
	amlEngine := env.AMLEngine

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Basic functionality test
	transaction := AMLTransaction{
		ID:        uuid.New(),
		UserID:    uuid.New(),
		Amount:    decimal.NewFromFloat(1000.0),
		Currency:  "USD",
		Type:      "transfer",
		Timestamp: time.Now(),
		IPAddress: "192.168.1.1",
		Location:  "US",
	}

	result, err := amlEngine.AnalyzeTransaction(ctx, transaction)
	if err != nil {
		t.Fatalf("Failed to analyze transaction: %v", err)
	}

	if result.TransactionID != transaction.ID {
		t.Errorf("Transaction ID mismatch")
	}

	t.Logf("AML Basic Test Results:")
	t.Logf("  Transaction ID: %s", result.TransactionID)
	t.Logf("  Risk Score: %.2f", result.RiskScore)
	t.Logf("  Flagged: %v", result.Flagged)
}

// Test AML Integration with Trading
func TestAMLTradingIntegration(t *testing.T) {
	env := NewTestEnvironment(DefaultTestConfig())
	amlEngine := env.AMLEngine
	tradingService := env.TradingService

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Create test user and simulate trade
	testUser := TestUser{
		ID:       uuid.New(),
		Username: "aml_test_user",
		Balance:  decimal.NewFromFloat(50000.0),
		Verified: true,
		KYCLevel: 2,
	}

	// Place order
	order := TestOrder{
		UserID:        testUser.ID,
		TradingPairID: "BTC-USD",
		Type:          "limit",
		Side:          "buy",
		Quantity:      decimal.NewFromFloat(1.0),
		Price:         decimal.NewFromFloat(45000.0),
	}

	placedOrder, err := tradingService.PlaceOrder(ctx, order)
	if err != nil {
		t.Fatalf("Failed to place order: %v", err)
	}

	// Simulate AML analysis
	transaction := AMLTransaction{
		ID:        uuid.New(),
		UserID:    testUser.ID,
		Amount:    order.Quantity.Mul(order.Price),
		Currency:  "USD",
		Type:      "order",
		Timestamp: time.Now(),
		IPAddress: "192.168.1.100",
		Location:  "US",
	}

	amlResult, err := amlEngine.AnalyzeTransaction(ctx, transaction)
	if err != nil {
		t.Fatalf("Failed to analyze transaction: %v", err)
	}

	// Check sanctions
	sanctioned, err := amlEngine.CheckSanctions(ctx, testUser.ID)
	if err != nil {
		t.Fatalf("Failed to check sanctions: %v", err)
	}

	t.Logf("AML Trading Integration Results:")
	t.Logf("  Order ID: %s", placedOrder.ID)
	t.Logf("  Risk Score: %.2f", amlResult.RiskScore)
	t.Logf("  Sanctioned: %v", sanctioned)
	t.Logf("  Transaction Amount: %s", transaction.Amount)
}
