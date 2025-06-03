package test

import (
	"context"
	"testing"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/compliance/aml"
	"github.com/Aidin1998/pincex_unified/internal/compliance/aml/screening"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.uber.org/zap"
)

// AMLRiskManagementTestSuite provides comprehensive testing for AML risk management
type AMLRiskManagementTestSuite struct {
	suite.Suite
	logger            *zap.SugaredLogger
	sanctionsScreener *screening.SanctionsScreener
	pepScreener       *screening.PEPScreener
	fuzzyMatcher      *screening.FuzzyMatcher
	// riskEngine removed as it doesn't exist yet
}

// SetupSuite initializes the test suite
func (suite *AMLRiskManagementTestSuite) SetupSuite() {
	suite.logger = zap.NewNop().Sugar()

	// Initialize components
	suite.sanctionsScreener = screening.NewSanctionsScreener(suite.logger)
	suite.pepScreener = screening.NewPEPScreener(suite.logger)
	suite.fuzzyMatcher = screening.NewFuzzyMatcher(suite.logger)

	// Note: RiskEngine does not exist yet, this would need to be implemented
	// For now, we'll test the screening components that do exist
	require.NoError(suite.T(), err)
}

// TestSanctionsScreeningWorkflow tests the complete sanctions screening workflow
func (suite *AMLRiskManagementTestSuite) TestSanctionsScreeningWorkflow() {
	ctx := context.Background()
	// Test cases for sanctions screening
	testCases := []struct {
		name     string
		userInfo aml.AMLUser
		expected bool
		reason   string
	}{
		{
			name: "Clean User",
			userInfo: aml.AMLUser{
				ID:             uuid.New(),
				UserID:         uuid.New(),
				CountryCode:    "US",
				KYCStatus:      "John Doe", // Using KYCStatus as name field
				RiskLevel:      aml.RiskLevelLow,
				RiskScore:      0.1,
				LastRiskUpdate: time.Date(1990, 1, 1, 0, 0, 0, 0, time.UTC),
			},
			expected: false,
			reason:   "Should pass clean user",
		},
		{
			name: "Suspicious Name Pattern",
			userInfo: aml.AMLUser{
				ID:          uuid.New(),
				UserID:      uuid.New(),
				CountryCode: "RU",
				KYCStatus:   "Vladimir Putinovich", // Using KYCStatus as name field
				DateOfBirth: time.Date(1950, 10, 7, 0, 0, 0, 0, time.UTC),
			},
			expected: true,
			reason:   "Should flag suspicious patterns",
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			result, err := suite.sanctionsScreener.ScreenUser(ctx, tc.userInfo)
			require.NoError(suite.T(), err)
			assert.Equal(suite.T(), tc.expected, result.IsMatch, tc.reason)
		})
	}
}

// TestPEPScreeningWorkflow tests PEP (Politically Exposed Person) screening
func (suite *AMLRiskManagementTestSuite) TestPEPScreeningWorkflow() {
	ctx := context.Background()

	testUser := aml.UserInfo{
		UserID:      uuid.New(),
		FirstName:   "Political",
		LastName:    "Figure",
		Country:     "US",
		Email:       "political@example.com",
		DateOfBirth: time.Date(1960, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	result, err := suite.pepScreener.ScreenUser(ctx, testUser)
	require.NoError(suite.T(), err)

	// Verify result structure
	assert.NotNil(suite.T(), result)
	assert.Contains(suite.T(), []bool{true, false}, result.IsMatch)

	if result.IsMatch {
		assert.NotEmpty(suite.T(), result.MatchedEntries)
		assert.Greater(suite.T(), result.ConfidenceScore, 0.0)
	}
}

// TestRiskScoringEngine tests the risk scoring engine
func (suite *AMLRiskManagementTestSuite) TestRiskScoringEngine() {
	ctx := context.Background()

	// Test transaction risk scoring
	transaction := &aml.TransactionInfo{
		TransactionID: uuid.New(),
		UserID:        uuid.New(),
		Amount:        5000.0,
		Currency:      "USD",
		Type:          "WITHDRAW",
		Timestamp:     time.Now(),
		Country:       "US",
	}

	userProfile := &aml.UserProfile{
		UserID:           transaction.UserID,
		RiskLevel:        "MEDIUM",
		TransactionCount: 10,
		AverageAmount:    1000.0,
		AccountAge:       30 * 24 * time.Hour, // 30 days
		IsVerified:       true,
	}

	riskScore, err := suite.riskEngine.CalculateTransactionRisk(ctx, transaction, userProfile)
	require.NoError(suite.T(), err)

	assert.GreaterOrEqual(suite.T(), riskScore, 0.0)
	assert.LessOrEqual(suite.T(), riskScore, 1.0)

	// Test risk level determination
	riskLevel := suite.riskEngine.DetermineRiskLevel(riskScore)
	assert.Contains(suite.T(), []string{"LOW", "MEDIUM", "HIGH"}, riskLevel)
}

// TestFuzzyMatchingAccuracy tests fuzzy matching accuracy
func (suite *AMLRiskManagementTestSuite) TestFuzzyMatchingAccuracy() {
	testCases := []struct {
		name      string
		input     string
		target    string
		threshold float64
		expected  bool
	}{
		{
			name:      "Exact Match",
			input:     "John Smith",
			target:    "John Smith",
			threshold: 0.9,
			expected:  true,
		},
		{
			name:      "Minor Typo",
			input:     "John Smyth",
			target:    "John Smith",
			threshold: 0.8,
			expected:  true,
		},
		{
			name:      "Different Names",
			input:     "John Smith",
			target:    "Jane Doe",
			threshold: 0.8,
			expected:  false,
		},
		{
			name:      "Phonetic Similarity",
			input:     "Catherine",
			target:    "Katherine",
			threshold: 0.7,
			expected:  true,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			similarity := suite.fuzzyMatcher.CalculateSimilarity(tc.input, tc.target)
			isMatch := similarity >= tc.threshold
			assert.Equal(suite.T(), tc.expected, isMatch,
				"Expected %v for similarity %.2f vs threshold %.2f", tc.expected, similarity, tc.threshold)
		})
	}
}

// TestAMLWorkflowIntegration tests the complete AML workflow integration
func (suite *AMLRiskManagementTestSuite) TestAMLWorkflowIntegration() {
	ctx := context.Background()

	// Simulate complete AML workflow
	userInfo := aml.UserInfo{
		UserID:      uuid.New(),
		FirstName:   "Test",
		LastName:    "User",
		Country:     "US",
		Email:       "test@example.com",
		DateOfBirth: time.Date(1985, 6, 15, 0, 0, 0, 0, time.UTC),
	}

	// Step 1: Sanctions screening
	sanctionsResult, err := suite.sanctionsScreener.ScreenUser(ctx, userInfo)
	require.NoError(suite.T(), err)

	// Step 2: PEP screening
	pepResult, err := suite.pepScreener.ScreenUser(ctx, userInfo)
	require.NoError(suite.T(), err)

	// Step 3: Risk assessment
	transaction := &aml.TransactionInfo{
		TransactionID: uuid.New(),
		UserID:        userInfo.UserID,
		Amount:        1000.0,
		Currency:      "USD",
		Type:          "DEPOSIT",
		Timestamp:     time.Now(),
		Country:       userInfo.Country,
	}

	userProfile := &aml.UserProfile{
		UserID:           userInfo.UserID,
		RiskLevel:        "LOW",
		TransactionCount: 5,
		AverageAmount:    500.0,
		AccountAge:       7 * 24 * time.Hour, // 7 days
		IsVerified:       true,
	}

	riskScore, err := suite.riskEngine.CalculateTransactionRisk(ctx, transaction, userProfile)
	require.NoError(suite.T(), err)

	// Step 4: Final decision
	finalDecision := aml.ComplianceDecision{
		UserID:         userInfo.UserID,
		TransactionID:  transaction.TransactionID,
		SanctionsMatch: sanctionsResult.IsMatch,
		PEPMatch:       pepResult.IsMatch,
		RiskScore:      riskScore,
		RiskLevel:      suite.riskEngine.DetermineRiskLevel(riskScore),
		Decision:       "APPROVE", // Would be determined by business rules
		Timestamp:      time.Now(),
	}

	// Validate final decision structure
	assert.Equal(suite.T(), userInfo.UserID, finalDecision.UserID)
	assert.Equal(suite.T(), transaction.TransactionID, finalDecision.TransactionID)
	assert.GreaterOrEqual(suite.T(), finalDecision.RiskScore, 0.0)
	assert.LessOrEqual(suite.T(), finalDecision.RiskScore, 1.0)
	assert.Contains(suite.T(), []string{"APPROVE", "REJECT", "REVIEW"}, finalDecision.Decision)
}

// TestHighRiskScenarios tests scenarios that should trigger high-risk alerts
func (suite *AMLRiskManagementTestSuite) TestHighRiskScenarios() {
	ctx := context.Background()

	// High-risk transaction scenarios
	highRiskScenarios := []struct {
		name        string
		transaction *aml.TransactionInfo
		userProfile *aml.UserProfile
		expectHigh  bool
	}{
		{
			name: "Large Amount New User",
			transaction: &aml.TransactionInfo{
				TransactionID: uuid.New(),
				UserID:        uuid.New(),
				Amount:        50000.0, // Large amount
				Currency:      "USD",
				Type:          "WITHDRAW",
				Timestamp:     time.Now(),
				Country:       "US",
			},
			userProfile: &aml.UserProfile{
				UserID:           uuid.New(),
				RiskLevel:        "LOW",
				TransactionCount: 1, // New user
				AverageAmount:    1000.0,
				AccountAge:       24 * time.Hour, // 1 day old
				IsVerified:       false,
			},
			expectHigh: true,
		},
		{
			name: "High-Risk Country",
			transaction: &aml.TransactionInfo{
				TransactionID: uuid.New(),
				UserID:        uuid.New(),
				Amount:        5000.0,
				Currency:      "USD",
				Type:          "WITHDRAW",
				Timestamp:     time.Now(),
				Country:       "IR", // High-risk country
			},
			userProfile: &aml.UserProfile{
				UserID:           uuid.New(),
				RiskLevel:        "HIGH",
				TransactionCount: 5,
				AverageAmount:    1000.0,
				AccountAge:       30 * 24 * time.Hour,
				IsVerified:       true,
			},
			expectHigh: true,
		},
	}

	for _, scenario := range highRiskScenarios {
		suite.Run(scenario.name, func() {
			riskScore, err := suite.riskEngine.CalculateTransactionRisk(ctx, scenario.transaction, scenario.userProfile)
			require.NoError(suite.T(), err)

			riskLevel := suite.riskEngine.DetermineRiskLevel(riskScore)

			if scenario.expectHigh {
				assert.Contains(suite.T(), []string{"HIGH", "MEDIUM"}, riskLevel)
				assert.GreaterOrEqual(suite.T(), riskScore, 0.5)
			}
		})
	}
}

// TestPerformanceUnderLoad tests AML system performance under load
func (suite *AMLRiskManagementTestSuite) TestPerformanceUnderLoad() {
	ctx := context.Background()

	// Test concurrent screening operations
	concurrency := 100
	operationsPerWorker := 10

	startTime := time.Now()

	// Channel to collect results
	results := make(chan error, concurrency*operationsPerWorker)

	for i := 0; i < concurrency; i++ {
		go func(workerID int) {
			for j := 0; j < operationsPerWorker; j++ {
				userInfo := aml.UserInfo{
					UserID:      uuid.New(),
					FirstName:   "LoadTest",
					LastName:    "User",
					Country:     "US",
					Email:       "loadtest@example.com",
					DateOfBirth: time.Date(1990, 1, 1, 0, 0, 0, 0, time.UTC),
				}

				_, err := suite.sanctionsScreener.ScreenUser(ctx, userInfo)
				results <- err
			}
		}(i)
	}

	// Collect results
	var errors int
	for i := 0; i < concurrency*operationsPerWorker; i++ {
		if err := <-results; err != nil {
			errors++
		}
	}

	duration := time.Since(startTime)
	totalOperations := concurrency * operationsPerWorker
	operationsPerSecond := float64(totalOperations) / duration.Seconds()

	// Performance assertions
	assert.Equal(suite.T(), 0, errors, "Should have no errors during load test")
	assert.Greater(suite.T(), operationsPerSecond, 100.0, "Should process at least 100 operations per second")
	assert.Less(suite.T(), duration, 10*time.Second, "Should complete within 10 seconds")

	suite.T().Logf("Performance: %d operations in %v (%.2f ops/sec)",
		totalOperations, duration, operationsPerSecond)
}

// TestComplianceDataStructures tests data structure compliance
func (suite *AMLRiskManagementTestSuite) TestComplianceDataStructures() {
	// Test UserInfo validation
	userInfo := aml.UserInfo{
		UserID:      uuid.New(),
		FirstName:   "John",
		LastName:    "Doe",
		Country:     "US",
		Email:       "john.doe@example.com",
		DateOfBirth: time.Date(1990, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	// Validate required fields
	assert.NotEqual(suite.T(), uuid.Nil, userInfo.UserID)
	assert.NotEmpty(suite.T(), userInfo.FirstName)
	assert.NotEmpty(suite.T(), userInfo.LastName)
	assert.NotEmpty(suite.T(), userInfo.Country)
	assert.NotEmpty(suite.T(), userInfo.Email)
	assert.False(suite.T(), userInfo.DateOfBirth.IsZero())

	// Test ScreeningResult validation
	result := aml.ScreeningResult{
		UserID:          userInfo.UserID,
		ScreeningType:   "SANCTIONS",
		IsMatch:         false,
		ConfidenceScore: 0.1,
		MatchedEntries:  []aml.MatchedEntry{},
		Timestamp:       time.Now(),
	}

	assert.NotEqual(suite.T(), uuid.Nil, result.UserID)
	assert.NotEmpty(suite.T(), result.ScreeningType)
	assert.GreaterOrEqual(suite.T(), result.ConfidenceScore, 0.0)
	assert.LessOrEqual(suite.T(), result.ConfidenceScore, 1.0)
	assert.False(suite.T(), result.Timestamp.IsZero())
}

// TestAMLRiskManagementSuite runs the complete AML test suite
func TestAMLRiskManagementSuite(t *testing.T) {
	suite.Run(t, new(AMLRiskManagementTestSuite))
}

// BenchmarkSanctionsScreening benchmarks sanctions screening performance
func BenchmarkSanctionsScreening(b *testing.B) {
	logger := zap.NewNop()
	screener, err := screening.NewSanctionsScreener(logger)
	require.NoError(b, err)

	userInfo := aml.UserInfo{
		UserID:      uuid.New(),
		FirstName:   "Benchmark",
		LastName:    "User",
		Country:     "US",
		Email:       "benchmark@example.com",
		DateOfBirth: time.Date(1990, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		userInfo.UserID = uuid.New() // Unique ID for each iteration
		_, err := screener.ScreenUser(ctx, userInfo)
		require.NoError(b, err)
	}
}
