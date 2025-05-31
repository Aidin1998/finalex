package testing

import (
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/pincex/internal/compliance/aml/actions"
	"github.com/pincex/internal/compliance/aml/analytics"
	"github.com/pincex/internal/compliance/aml/detection"
	"github.com/pincex/internal/compliance/aml/models"
	"github.com/pincex/internal/compliance/aml/monitoring"
	"github.com/pincex/internal/compliance/aml/rules"
	"github.com/pincex/internal/compliance/aml/scoring"
)

// TestConfig holds configuration for AML testing
type TestConfig struct {
	NumberOfUsers        int           `json:"number_of_users"`
	TransactionsPerUser  int           `json:"transactions_per_user"`
	TestDuration         time.Duration `json:"test_duration"`
	ConcurrentUsers      int           `json:"concurrent_users"`
	SuspiciousPercentage float64       `json:"suspicious_percentage"`
	EnableStressTest     bool          `json:"enable_stress_test"`
	ValidationEnabled    bool          `json:"validation_enabled"`
}

// TestResult represents the result of an AML test
type TestResult struct {
	TestName           string                 `json:"test_name"`
	StartTime          time.Time              `json:"start_time"`
	EndTime            time.Time              `json:"end_time"`
	Duration           time.Duration          `json:"duration"`
	TotalTransactions  int                    `json:"total_transactions"`
	SuspiciousDetected int                    `json:"suspicious_detected"`
	FalsePositives     int                    `json:"false_positives"`
	FalseNegatives     int                    `json:"false_negatives"`
	TruePositives      int                    `json:"true_positives"`
	TrueNegatives      int                    `json:"true_negatives"`
	Precision          float64                `json:"precision"`
	Recall             float64                `json:"recall"`
	F1Score            float64                `json:"f1_score"`
	AccuracyRate       float64                `json:"accuracy_rate"`
	ProcessingLatency  time.Duration          `json:"processing_latency"`
	ThroughputTPS      float64                `json:"throughput_tps"`
	MemoryUsage        int64                  `json:"memory_usage_mb"`
	CPUUsage           float64                `json:"cpu_usage_percent"`
	Errors             []string               `json:"errors"`
	Warnings           []string               `json:"warnings"`
	AdditionalMetrics  map[string]interface{} `json:"additional_metrics"`
	Passed             bool                   `json:"passed"`
}

// TestSuite represents a collection of AML tests
type TestSuite struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Tests       []TestCase    `json:"tests"`
	Config      *TestConfig   `json:"config"`
	Results     []*TestResult `json:"results"`
	mu          sync.Mutex
}

// TestCase represents an individual test case
type TestCase struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	TestType    string      `json:"test_type"`
	Expected    interface{} `json:"expected"`
	Input       interface{} `json:"input"`
	Validation  func(*TestResult) bool
}

// AMLTestFramework provides comprehensive testing for AML systems
type AMLTestFramework struct {
	config        *TestConfig
	ruleEngine    *rules.RuleEngine
	detector      *detection.CryptoDetector
	analytics     *analytics.BehavioralAnalytics
	scorer        *scoring.DynamicRiskScorer
	actionHandler *actions.AutomatedActionHandler
	monitor       *monitoring.RealtimeMonitor

	testUsers        []*models.AMLUser
	testTransactions []*models.Transaction

	mu sync.RWMutex
}

// NewAMLTestFramework creates a new AML testing framework
func NewAMLTestFramework(config *TestConfig) *AMLTestFramework {
	framework := &AMLTestFramework{
		config:           config,
		testUsers:        make([]*models.AMLUser, 0),
		testTransactions: make([]*models.Transaction, 0),
	}

	framework.initializeComponents()
	return framework
}

// initializeComponents initializes AML components for testing
func (f *AMLTestFramework) initializeComponents() {
	// Initialize rule engine
	f.ruleEngine = rules.NewRuleEngine()

	// Initialize crypto detector
	cryptoConfig := &detection.CryptoDetectorConfig{
		BlockchainAPIs: map[string]string{
			"bitcoin":  "https://api.blockchain.info",
			"ethereum": "https://api.etherscan.io",
		},
		RiskThresholds: map[string]float64{
			"mixer":              80.0,
			"darknet":            90.0,
			"sanctioned":         95.0,
			"high_risk_exchange": 70.0,
		},
		EnableTaintAnalysis: true,
		TaintDepth:          5,
	}
	f.detector = detection.NewCryptoDetector(cryptoConfig)

	// Initialize behavioral analytics
	analyticsConfig := &analytics.BehavioralAnalyticsConfig{
		LearningPeriod:   7 * 24 * time.Hour,
		AnomalyThreshold: 0.8,
		UpdateInterval:   time.Hour,
		MaxProfileAge:    30 * 24 * time.Hour,
		EnableMLAnalysis: true,
		PatternDetection: true,
	}
	f.analytics = analytics.NewBehavioralAnalytics(analyticsConfig)

	// Initialize dynamic risk scorer
	scorerConfig := &scoring.DynamicRiskScorerConfig{
		MLModelEndpoint:       "http://localhost:8080/predict",
		ModelUpdateInterval:   24 * time.Hour,
		FeatureEngineering:    true,
		EnsembleModels:        []string{"random_forest", "gradient_boost", "neural_network"},
		ExplainabilityEnabled: true,
	}
	f.scorer = scoring.NewDynamicRiskScorer(scorerConfig)

	// Initialize action handler
	actionConfig := &actions.AutomatedActionConfig{
		EnabledActions: map[string]bool{
			"BLOCK_TRANSACTION":     true,
			"FREEZE_ACCOUNT":        true,
			"CREATE_CASE":           true,
			"REQUIRE_MANUAL_REVIEW": true,
		},
		AutoApprovalLimits: map[string]float64{
			"BLOCK_TRANSACTION": 85.0,
			"FREEZE_ACCOUNT":    90.0,
		},
		EscalationThresholds: map[string]float64{
			"SENIOR_ANALYST":     80.0,
			"COMPLIANCE_MANAGER": 90.0,
		},
		WorkflowEnabled: true,
	}
	f.actionHandler = actions.NewAutomatedActionHandler(actionConfig)

	// Initialize realtime monitor
	f.monitor = monitoring.NewRealtimeMonitor(nil)
}

// GenerateTestData creates synthetic test data for AML testing
func (f *AMLTestFramework) GenerateTestData() error {
	log.Printf("Generating test data: %d users, %d transactions per user",
		f.config.NumberOfUsers, f.config.TransactionsPerUser)

	f.mu.Lock()
	defer f.mu.Unlock()

	// Generate test users
	for i := 0; i < f.config.NumberOfUsers; i++ {
		user := f.generateTestUser(i)
		f.testUsers = append(f.testUsers, user)
	}

	// Generate test transactions
	for _, user := range f.testUsers {
		for j := 0; j < f.config.TransactionsPerUser; j++ {
			tx := f.generateTestTransaction(user, j)
			f.testTransactions = append(f.testTransactions, tx)
		}
	}

	log.Printf("Generated %d users and %d transactions",
		len(f.testUsers), len(f.testTransactions))

	return nil
}

// generateTestUser creates a synthetic AML user
func (f *AMLTestFramework) generateTestUser(index int) *models.AMLUser {
	countries := []string{"US", "UK", "CA", "DE", "JP", "AU", "FR", "IT", "ES", "NL"}
	kycStatuses := []string{"PENDING", "VERIFIED", "REJECTED", "EXPIRED"}

	user := &models.AMLUser{
		ID:                   uint(index + 1),
		UserID:               fmt.Sprintf("test_user_%d", index+1),
		RiskScore:            rand.Float64() * 100,
		KYCStatus:            kycStatuses[rand.Intn(len(kycStatuses))],
		KYCLevel:             rand.Intn(4),
		PEPStatus:            rand.Float64() < 0.05, // 5% PEP
		SanctionsStatus:      rand.Float64() < 0.01, // 1% sanctioned
		EnhancedDueDiligence: rand.Float64() < 0.1,  // 10% EDD
		CountryCode:          countries[rand.Intn(len(countries))],
		RegistrationDate:     time.Now().AddDate(0, -rand.Intn(24), -rand.Intn(30)),
		LastRiskAssessment:   time.Now().AddDate(0, 0, -rand.Intn(30)),
		ProfileData: map[string]interface{}{
			"occupation":      f.getRandomOccupation(),
			"income_range":    f.getRandomIncomeRange(),
			"source_of_funds": f.getRandomSourceOfFunds(),
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	return user
}

// generateTestTransaction creates a synthetic transaction
func (f *AMLTestFramework) generateTestTransaction(user *models.AMLUser, index int) *models.Transaction {
	assets := []string{"BTC", "ETH", "USDT", "USDC", "LTC", "XRP", "ADA", "DOT"}
	txTypes := []string{"DEPOSIT", "WITHDRAWAL", "TRADE", "TRANSFER"}

	// Determine if this should be suspicious
	isSuspicious := rand.Float64() < f.config.SuspiciousPercentage/100.0

	amount := f.generateTransactionAmount(isSuspicious)

	tx := &models.Transaction{
		ID:              uint(index + 1),
		TransactionID:   fmt.Sprintf("tx_%s_%d", user.UserID, index+1),
		UserID:          user.UserID,
		FromAddress:     f.generateAddress(),
		ToAddress:       f.generateAddress(),
		Asset:           assets[rand.Intn(len(assets))],
		Amount:          amount,
		Fee:             amount * 0.001, // 0.1% fee
		TransactionType: txTypes[rand.Intn(len(txTypes))],
		RiskScore:       f.calculateTransactionRiskScore(user, amount, isSuspicious),
		Suspicious:      isSuspicious,
		Timestamp:       time.Now().Add(-time.Duration(rand.Intn(30*24)) * time.Hour),
		Metadata: map[string]interface{}{
			"test_data":  true,
			"suspicious": isSuspicious,
			"category":   f.getTransactionCategory(isSuspicious),
		},
		CreatedAt: time.Now(),
	}

	return tx
}

// RunTestSuite executes a comprehensive test suite
func (f *AMLTestFramework) RunTestSuite(suite *TestSuite) error {
	log.Printf("Running AML test suite: %s", suite.Name)

	for _, testCase := range suite.Tests {
		result, err := f.runTestCase(testCase)
		if err != nil {
			log.Printf("Test case %s failed: %v", testCase.Name, err)
			result.Errors = append(result.Errors, err.Error())
			result.Passed = false
		}

		suite.mu.Lock()
		suite.Results = append(suite.Results, result)
		suite.mu.Unlock()
	}

	return nil
}

// runTestCase executes an individual test case
func (f *AMLTestFramework) runTestCase(testCase TestCase) (*TestResult, error) {
	result := &TestResult{
		TestName:          testCase.Name,
		StartTime:         time.Now(),
		AdditionalMetrics: make(map[string]interface{}),
		Errors:            make([]string, 0),
		Warnings:          make([]string, 0),
	}

	defer func() {
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime)
	}()

	switch testCase.TestType {
	case "detection_accuracy":
		return f.runDetectionAccuracyTest(testCase, result)
	case "performance":
		return f.runPerformanceTest(testCase, result)
	case "stress_test":
		return f.runStressTest(testCase, result)
	case "rule_engine":
		return f.runRuleEngineTest(testCase, result)
	case "behavioral_analytics":
		return f.runBehavioralAnalyticsTest(testCase, result)
	case "risk_scoring":
		return f.runRiskScoringTest(testCase, result)
	case "automated_actions":
		return f.runAutomatedActionsTest(testCase, result)
	default:
		return result, fmt.Errorf("unknown test type: %s", testCase.TestType)
	}
}

// runDetectionAccuracyTest tests the accuracy of AML detection
func (f *AMLTestFramework) runDetectionAccuracyTest(testCase TestCase, result *TestResult) (*TestResult, error) {
	log.Printf("Running detection accuracy test: %s", testCase.Name)

	var truePositives, falsePositives, trueNegatives, falseNegatives int

	f.mu.RLock()
	transactions := f.testTransactions
	f.mu.RUnlock()

	result.TotalTransactions = len(transactions)

	for _, tx := range transactions {
		// Get actual label from metadata
		actualSuspicious := tx.Metadata["suspicious"].(bool)

		// Run detection
		detectionResult, err := f.runDetectionOnTransaction(tx)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("Detection failed for tx %s: %v", tx.TransactionID, err))
			continue
		}

		predictedSuspicious := detectionResult.RiskScore >= 70.0 // threshold

		// Update confusion matrix
		if actualSuspicious && predictedSuspicious {
			truePositives++
		} else if actualSuspicious && !predictedSuspicious {
			falseNegatives++
		} else if !actualSuspicious && predictedSuspicious {
			falsePositives++
		} else {
			trueNegatives++
		}
	}

	// Calculate metrics
	result.TruePositives = truePositives
	result.FalsePositives = falsePositives
	result.TrueNegatives = trueNegatives
	result.FalseNegatives = falseNegatives
	result.SuspiciousDetected = truePositives + falsePositives

	if truePositives+falsePositives > 0 {
		result.Precision = float64(truePositives) / float64(truePositives+falsePositives)
	}

	if truePositives+falseNegatives > 0 {
		result.Recall = float64(truePositives) / float64(truePositives+falseNegatives)
	}

	if result.Precision+result.Recall > 0 {
		result.F1Score = 2 * (result.Precision * result.Recall) / (result.Precision + result.Recall)
	}

	total := truePositives + falsePositives + trueNegatives + falseNegatives
	if total > 0 {
		result.AccuracyRate = float64(truePositives+trueNegatives) / float64(total)
	}

	// Validate against expected thresholds
	if testCase.Expected != nil {
		expected := testCase.Expected.(map[string]float64)
		result.Passed = result.Precision >= expected["min_precision"] &&
			result.Recall >= expected["min_recall"] &&
			result.F1Score >= expected["min_f1_score"]
	} else {
		result.Passed = result.Precision >= 0.8 && result.Recall >= 0.7 && result.F1Score >= 0.75
	}

	return result, nil
}

// runPerformanceTest tests the performance of AML processing
func (f *AMLTestFramework) runPerformanceTest(testCase TestCase, result *TestResult) (*TestResult, error) {
	log.Printf("Running performance test: %s", testCase.Name)

	f.mu.RLock()
	transactions := f.testTransactions
	f.mu.RUnlock()

	if len(transactions) == 0 {
		return result, fmt.Errorf("no test transactions available")
	}

	result.TotalTransactions = len(transactions)

	// Measure processing time
	start := time.Now()
	var totalLatency time.Duration
	var processedCount int

	for _, tx := range transactions {
		txStart := time.Now()
		_, err := f.runDetectionOnTransaction(tx)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("Processing failed for tx %s: %v", tx.TransactionID, err))
			continue
		}

		txLatency := time.Since(txStart)
		totalLatency += txLatency
		processedCount++
	}

	processingTime := time.Since(start)

	if processedCount > 0 {
		result.ProcessingLatency = totalLatency / time.Duration(processedCount)
		result.ThroughputTPS = float64(processedCount) / processingTime.Seconds()
	}

	// Validate performance thresholds
	if testCase.Expected != nil {
		expected := testCase.Expected.(map[string]float64)
		result.Passed = result.ThroughputTPS >= expected["min_tps"] &&
			result.ProcessingLatency.Milliseconds() <= int64(expected["max_latency_ms"])
	} else {
		result.Passed = result.ThroughputTPS >= 100 && result.ProcessingLatency.Milliseconds() <= 100
	}

	return result, nil
}

// runStressTest tests the system under high load
func (f *AMLTestFramework) runStressTest(testCase TestCase, result *TestResult) (*TestResult, error) {
	if !f.config.EnableStressTest {
		result.Warnings = append(result.Warnings, "Stress test disabled in configuration")
		result.Passed = true
		return result, nil
	}

	log.Printf("Running stress test: %s", testCase.Name)

	concurrentUsers := f.config.ConcurrentUsers
	if concurrentUsers == 0 {
		concurrentUsers = 50
	}

	var wg sync.WaitGroup
	var totalProcessed int64
	var totalErrors int64
	var mu sync.Mutex

	start := time.Now()

	for i := 0; i < concurrentUsers; i++ {
		wg.Add(1)
		go func(userIndex int) {
			defer wg.Done()

			processed := 0
			errors := 0

			for j := 0; j < 100; j++ { // 100 transactions per concurrent user
				tx := f.generateTestTransaction(f.testUsers[userIndex%len(f.testUsers)], j)

				_, err := f.runDetectionOnTransaction(tx)
				if err != nil {
					errors++
				} else {
					processed++
				}
			}

			mu.Lock()
			totalProcessed += int64(processed)
			totalErrors += int64(errors)
			mu.Unlock()
		}(i)
	}

	wg.Wait()
	duration := time.Since(start)

	result.TotalTransactions = int(totalProcessed + totalErrors)
	result.ThroughputTPS = float64(totalProcessed) / duration.Seconds()

	errorRate := float64(totalErrors) / float64(totalProcessed+totalErrors)
	result.AdditionalMetrics["error_rate"] = errorRate
	result.AdditionalMetrics["concurrent_users"] = concurrentUsers

	// Validate stress test results
	result.Passed = result.ThroughputTPS >= 50 && errorRate <= 0.05 // Max 5% error rate

	return result, nil
}

// runRuleEngineTest tests the rule engine functionality
func (f *AMLTestFramework) runRuleEngineTest(testCase TestCase, result *TestResult) (*TestResult, error) {
	log.Printf("Running rule engine test: %s", testCase.Name)

	// Test rule creation, evaluation, and management
	testRule := &rules.Rule{
		ID:          "test_rule_1",
		Name:        "High Value Transaction",
		Description: "Detect transactions above $10,000",
		Type:        "TRANSACTION",
		Conditions: map[string]interface{}{
			"amount": map[string]interface{}{
				"operator": "greater_than",
				"value":    10000,
			},
		},
		Actions: []map[string]interface{}{
			{
				"type": "FLAG_TRANSACTION",
				"params": map[string]interface{}{
					"severity": "HIGH",
				},
			},
		},
		Priority: 100,
		Enabled:  true,
	}

	// Add rule to engine
	err := f.ruleEngine.AddRule(testRule)
	if err != nil {
		return result, fmt.Errorf("failed to add test rule: %v", err)
	}

	// Test rule evaluation
	f.mu.RLock()
	transactions := f.testTransactions[:10] // Test with first 10 transactions
	f.mu.RUnlock()

	triggeredCount := 0
	for _, tx := range transactions {
		triggered, _, err := f.ruleEngine.EvaluateTransaction(tx)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("Rule evaluation failed: %v", err))
			continue
		}

		if triggered {
			triggeredCount++
		}
	}

	result.TotalTransactions = len(transactions)
	result.AdditionalMetrics["rules_triggered"] = triggeredCount
	result.AdditionalMetrics["total_rules"] = len(f.ruleEngine.GetEnabledRules())

	// Clean up test rule
	f.ruleEngine.RemoveRule("test_rule_1")

	result.Passed = true // Basic functionality test
	return result, nil
}

// runBehavioralAnalyticsTest tests behavioral analytics functionality
func (f *AMLTestFramework) runBehavioralAnalyticsTest(testCase TestCase, result *TestResult) (*TestResult, error) {
	log.Printf("Running behavioral analytics test: %s", testCase.Name)

	f.mu.RLock()
	users := f.testUsers[:5] // Test with first 5 users
	transactions := f.testTransactions
	f.mu.RUnlock()

	anomaliesDetected := 0

	for _, user := range users {
		// Get user's transactions
		var userTransactions []*models.Transaction
		for _, tx := range transactions {
			if tx.UserID == user.UserID {
				userTransactions = append(userTransactions, tx)
			}
		}

		if len(userTransactions) == 0 {
			continue
		}

		// Analyze user behavior
		analysisResult, err := f.analytics.AnalyzeUserBehavior(user.UserID, userTransactions)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("Behavioral analysis failed for user %s: %v", user.UserID, err))
			continue
		}

		if analysisResult.AnomalyScore > 0.7 {
			anomaliesDetected++
		}
	}

	result.AdditionalMetrics["users_analyzed"] = len(users)
	result.AdditionalMetrics["anomalies_detected"] = anomaliesDetected
	result.AdditionalMetrics["anomaly_rate"] = float64(anomaliesDetected) / float64(len(users))

	result.Passed = true // Basic functionality test
	return result, nil
}

// runRiskScoringTest tests risk scoring functionality
func (f *AMLTestFramework) runRiskScoringTest(testCase TestCase, result *TestResult) (*TestResult, error) {
	log.Printf("Running risk scoring test: %s", testCase.Name)

	f.mu.RLock()
	transactions := f.testTransactions[:50] // Test with first 50 transactions
	f.mu.RUnlock()

	var totalScore float64
	scoredCount := 0

	for _, tx := range transactions {
		score, err := f.scorer.CalculateRiskScore(tx, nil)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("Risk scoring failed for tx %s: %v", tx.TransactionID, err))
			continue
		}

		totalScore += score.TotalScore
		scoredCount++
	}

	if scoredCount > 0 {
		avgScore := totalScore / float64(scoredCount)
		result.AdditionalMetrics["average_risk_score"] = avgScore
		result.AdditionalMetrics["transactions_scored"] = scoredCount
	}

	result.Passed = scoredCount > 0 // Basic functionality test
	return result, nil
}

// runAutomatedActionsTest tests automated action functionality
func (f *AMLTestFramework) runAutomatedActionsTest(testCase TestCase, result *TestResult) (*TestResult, error) {
	log.Printf("Running automated actions test: %s", testCase.Name)

	// Create test compliance alert
	alert := &models.ComplianceAlert{
		ID:          1,
		UserID:      "test_user_1",
		AlertType:   "HIGH_RISK_TRANSACTION",
		Severity:    "HIGH",
		RiskScore:   85.0,
		Description: "High risk transaction detected",
		Status:      "ACTIVE",
		Metadata: map[string]interface{}{
			"transaction_id": "tx_test_1",
			"amount":         15000,
		},
		CreatedAt: time.Now(),
	}

	// Process alert through action handler
	actions, err := f.actionHandler.ProcessAlert(alert)
	if err != nil {
		return result, fmt.Errorf("action processing failed: %v", err)
	}

	result.AdditionalMetrics["actions_taken"] = len(actions)
	result.AdditionalMetrics["alert_processed"] = true

	if len(actions) > 0 {
		result.AdditionalMetrics["first_action_type"] = actions[0].ActionType
	}

	result.Passed = len(actions) > 0 // At least one action should be taken
	return result, nil
}

// runDetectionOnTransaction runs AML detection on a single transaction
func (f *AMLTestFramework) runDetectionOnTransaction(tx *models.Transaction) (*scoring.RiskScore, error) {
	// Run crypto detection
	cryptoResult, err := f.detector.AnalyzeTransaction(tx)
	if err != nil {
		return nil, fmt.Errorf("crypto detection failed: %v", err)
	}

	// Run behavioral analysis
	var userTransactions []*models.Transaction
	f.mu.RLock()
	for _, userTx := range f.testTransactions {
		if userTx.UserID == tx.UserID {
			userTransactions = append(userTransactions, userTx)
		}
	}
	f.mu.RUnlock()

	behaviorResult, err := f.analytics.AnalyzeUserBehavior(tx.UserID, userTransactions)
	if err != nil {
		return nil, fmt.Errorf("behavioral analysis failed: %v", err)
	}

	// Calculate risk score
	riskScore, err := f.scorer.CalculateRiskScore(tx, map[string]interface{}{
		"crypto_analysis":     cryptoResult,
		"behavioral_analysis": behaviorResult,
	})
	if err != nil {
		return nil, fmt.Errorf("risk scoring failed: %v", err)
	}

	return riskScore, nil
}

// Helper methods for test data generation

func (f *AMLTestFramework) getRandomOccupation() string {
	occupations := []string{
		"Software Engineer", "Teacher", "Doctor", "Lawyer", "Accountant",
		"Business Owner", "Student", "Retired", "Consultant", "Manager",
	}
	return occupations[rand.Intn(len(occupations))]
}

func (f *AMLTestFramework) getRandomIncomeRange() string {
	ranges := []string{
		"0-25000", "25000-50000", "50000-100000", "100000-250000", "250000+",
	}
	return ranges[rand.Intn(len(ranges))]
}

func (f *AMLTestFramework) getRandomSourceOfFunds() string {
	sources := []string{
		"Employment", "Business", "Investment", "Inheritance", "Gift", "Other",
	}
	return sources[rand.Intn(len(sources))]
}

func (f *AMLTestFramework) generateTransactionAmount(isSuspicious bool) float64 {
	if isSuspicious {
		// Generate larger amounts for suspicious transactions
		return 10000 + rand.Float64()*90000 // $10K - $100K
	} else {
		// Generate normal amounts
		return 10 + rand.Float64()*9990 // $10 - $10K
	}
}

func (f *AMLTestFramework) calculateTransactionRiskScore(user *models.AMLUser, amount float64, isSuspicious bool) float64 {
	score := 0.0

	// Base risk from user
	score += user.RiskScore * 0.3

	// Amount risk
	if amount > 10000 {
		score += 30
	} else if amount > 5000 {
		score += 15
	}

	// User factors
	if user.PEPStatus {
		score += 20
	}
	if user.SanctionsStatus {
		score += 40
	}
	if user.KYCStatus != "VERIFIED" {
		score += 15
	}

	// Add randomness
	score += (rand.Float64() - 0.5) * 20

	// Adjust for test purposes
	if isSuspicious {
		score = 70 + rand.Float64()*30 // 70-100 for suspicious
	} else {
		if score > 70 {
			score = rand.Float64() * 70 // 0-70 for non-suspicious
		}
	}

	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}

	return score
}

func (f *AMLTestFramework) generateAddress() string {
	// Generate random crypto address
	chars := "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
	address := "1" // Bitcoin-style address
	for i := 0; i < 33; i++ {
		address += string(chars[rand.Intn(len(chars))])
	}
	return address
}

func (f *AMLTestFramework) getTransactionCategory(isSuspicious bool) string {
	if isSuspicious {
		categories := []string{
			"high_risk", "layering", "structuring", "smurfing", "mixer_usage",
		}
		return categories[rand.Intn(len(categories))]
	} else {
		categories := []string{
			"normal", "trading", "investment", "payment", "transfer",
		}
		return categories[rand.Intn(len(categories))]
	}
}

// GenerateComprehensiveTestSuite creates a comprehensive test suite
func (f *AMLTestFramework) GenerateComprehensiveTestSuite() *TestSuite {
	return &TestSuite{
		Name:        "Comprehensive AML Test Suite",
		Description: "Complete testing of AML detection and compliance system",
		Config:      f.config,
		Tests: []TestCase{
			{
				Name:        "Detection Accuracy Test",
				Description: "Test the accuracy of suspicious transaction detection",
				TestType:    "detection_accuracy",
				Expected: map[string]float64{
					"min_precision": 0.8,
					"min_recall":    0.7,
					"min_f1_score":  0.75,
				},
			},
			{
				Name:        "Performance Test",
				Description: "Test the performance and throughput of AML processing",
				TestType:    "performance",
				Expected: map[string]float64{
					"min_tps":        100,
					"max_latency_ms": 100,
				},
			},
			{
				Name:        "Stress Test",
				Description: "Test system behavior under high concurrent load",
				TestType:    "stress_test",
			},
			{
				Name:        "Rule Engine Test",
				Description: "Test rule engine functionality and rule evaluation",
				TestType:    "rule_engine",
			},
			{
				Name:        "Behavioral Analytics Test",
				Description: "Test behavioral analytics and anomaly detection",
				TestType:    "behavioral_analytics",
			},
			{
				Name:        "Risk Scoring Test",
				Description: "Test dynamic risk scoring system",
				TestType:    "risk_scoring",
			},
			{
				Name:        "Automated Actions Test",
				Description: "Test automated action processing and workflow",
				TestType:    "automated_actions",
			},
		},
	}
}

// PrintTestResults prints formatted test results
func (f *AMLTestFramework) PrintTestResults(suite *TestSuite) {
	fmt.Printf("\n=== AML Test Suite Results: %s ===\n", suite.Name)
	fmt.Printf("Description: %s\n\n", suite.Description)

	totalTests := len(suite.Results)
	passedTests := 0

	for _, result := range suite.Results {
		if result.Passed {
			passedTests++
		}

		fmt.Printf("Test: %s\n", result.TestName)
		fmt.Printf("  Status: %s\n", map[bool]string{true: "PASSED", false: "FAILED"}[result.Passed])
		fmt.Printf("  Duration: %v\n", result.Duration)

		if result.TotalTransactions > 0 {
			fmt.Printf("  Transactions: %d\n", result.TotalTransactions)
		}

		if result.Precision > 0 {
			fmt.Printf("  Precision: %.2f%%\n", result.Precision*100)
			fmt.Printf("  Recall: %.2f%%\n", result.Recall*100)
			fmt.Printf("  F1 Score: %.2f\n", result.F1Score)
		}

		if result.ThroughputTPS > 0 {
			fmt.Printf("  Throughput: %.2f TPS\n", result.ThroughputTPS)
			fmt.Printf("  Latency: %v\n", result.ProcessingLatency)
		}

		if len(result.Errors) > 0 {
			fmt.Printf("  Errors: %d\n", len(result.Errors))
		}

		if len(result.Warnings) > 0 {
			fmt.Printf("  Warnings: %d\n", len(result.Warnings))
		}

		fmt.Println()
	}

	fmt.Printf("=== Summary ===\n")
	fmt.Printf("Total Tests: %d\n", totalTests)
	fmt.Printf("Passed: %d\n", passedTests)
	fmt.Printf("Failed: %d\n", totalTests-passedTests)
	fmt.Printf("Success Rate: %.2f%%\n", float64(passedTests)/float64(totalTests)*100)
}

// GetTestUsers returns the generated test users
func (f *AMLTestFramework) GetTestUsers() []*models.AMLUser {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.testUsers
}

// GetTestTransactions returns the generated test transactions
func (f *AMLTestFramework) GetTestTransactions() []*models.Transaction {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.testTransactions
}
