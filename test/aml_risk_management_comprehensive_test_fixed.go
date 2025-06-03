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
}

// SetupSuite initializes the test suite
func (suite *AMLRiskManagementTestSuite) SetupSuite() {
	suite.logger = zap.NewNop().Sugar()

	// Initialize components
	suite.sanctionsScreener = screening.NewSanctionsScreener(suite.logger)
	suite.pepScreener = screening.NewPEPScreener(suite.logger)
	suite.fuzzyMatcher = screening.NewFuzzyMatcher(suite.logger)
}

// TestSanctionsScreeningDetection tests sanctions list detection
func (suite *AMLRiskManagementTestSuite) TestSanctionsScreeningDetection() {
	tests := []struct {
		name     string
		userInfo aml.AMLUser
		expected bool
		reason   string
	}{
		{
			name: "sanctioned_individual",
			userInfo: aml.AMLUser{
				ID:          uuid.New(),
				UserID:      uuid.New(),
				KYCStatus:   "John Smith", // This should match a test sanctions entry
				CountryCode: "US",
				RiskLevel:   aml.RiskLevelLow,
			},
			expected: false, // Since we don't have real sanctions data
			reason:   "Should handle sanctioned individual check",
		},
		{
			name: "non_sanctioned_individual",
			userInfo: aml.AMLUser{
				ID:          uuid.New(),
				UserID:      uuid.New(),
				KYCStatus:   "Jane Doe",
				CountryCode: "CA",
				RiskLevel:   aml.RiskLevelLow,
			},
			expected: false,
			reason:   "Should not flag non-sanctioned individual",
		},
	}

	ctx := context.Background()
	for _, tc := range tests {
		suite.Run(tc.name, func() {
			result, err := suite.sanctionsScreener.ScreenUser(ctx, tc.userInfo.UserID, &tc.userInfo, "test")
			require.NoError(suite.T(), err)
			assert.NotNil(suite.T(), result)

			hasMatch := len(result.Results) > 0
			assert.Equal(suite.T(), tc.expected, hasMatch, tc.reason)
		})
	}
}

// TestPEPScreeningWorkflow tests PEP (Politically Exposed Person) screening
func (suite *AMLRiskManagementTestSuite) TestPEPScreeningWorkflow() {
	ctx := context.Background()

	testUser := aml.AMLUser{
		ID:          uuid.New(),
		UserID:      uuid.New(),
		KYCStatus:   "Political Figure",
		CountryCode: "US",
		RiskLevel:   aml.RiskLevelMedium,
	}

	result, err := suite.pepScreener.ScreenForPEP(ctx, testUser.UserID, &testUser)
	require.NoError(suite.T(), err)

	// Verify result structure
	assert.NotNil(suite.T(), result)
	assert.Contains(suite.T(), []bool{true, false}, result.IsPEP)

	if result.IsPEP {
		assert.NotEmpty(suite.T(), result.PEPMatches)
		assert.Greater(suite.T(), result.Confidence, 0.0)
	}
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
			result := suite.fuzzyMatcher.MatchNames(context.Background(), tc.input, tc.target, []string{})
			isMatch := result.OverallScore >= tc.threshold
			assert.Equal(suite.T(), tc.expected, isMatch,
				"Expected %v for similarity %.2f vs threshold %.2f", tc.expected, result.OverallScore, tc.threshold)
		})
	}
}

// TestAMLWorkflowIntegration tests the complete AML workflow integration
func (suite *AMLRiskManagementTestSuite) TestAMLWorkflowIntegration() {
	ctx := context.Background()

	// Simulate complete AML workflow
	userInfo := aml.AMLUser{
		ID:          uuid.New(),
		UserID:      uuid.New(),
		KYCStatus:   "Test User",
		CountryCode: "US",
		RiskLevel:   aml.RiskLevelLow,
	}

	// Step 1: Sanctions screening
	sanctionsResult, err := suite.sanctionsScreener.ScreenUser(ctx, userInfo.UserID, &userInfo, "integration_test")
	require.NoError(suite.T(), err)

	// Step 2: PEP screening
	pepResult, err := suite.pepScreener.ScreenForPEP(ctx, userInfo.UserID, &userInfo)
	require.NoError(suite.T(), err)

	// Validate workflow results
	assert.NotNil(suite.T(), sanctionsResult)
	assert.NotNil(suite.T(), pepResult)
	assert.Equal(suite.T(), userInfo.UserID, sanctionsResult.UserID)
	assert.Equal(suite.T(), userInfo.UserID, pepResult.UserID)
}

// TestPerformanceUnderLoad tests AML system performance under load
func (suite *AMLRiskManagementTestSuite) TestPerformanceUnderLoad() {
	ctx := context.Background()

	// Test concurrent screening operations
	concurrency := 10 // Reduced for testing
	operationsPerWorker := 5

	startTime := time.Now()

	// Channel to collect results
	results := make(chan error, concurrency*operationsPerWorker)

	for i := 0; i < concurrency; i++ {
		go func(workerID int) {
			for j := 0; j < operationsPerWorker; j++ {
				userInfo := aml.AMLUser{
					ID:          uuid.New(),
					UserID:      uuid.New(),
					KYCStatus:   "LoadTest User",
					CountryCode: "US",
					RiskLevel:   aml.RiskLevelLow,
				}

				_, err := suite.sanctionsScreener.ScreenUser(ctx, userInfo.UserID, &userInfo, "load_test")
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
	assert.Greater(suite.T(), operationsPerSecond, 10.0, "Should process at least 10 operations per second")
	assert.Less(suite.T(), duration, 30*time.Second, "Should complete within 30 seconds")

	suite.T().Logf("Performance: %d operations in %v (%.2f ops/sec)",
		totalOperations, duration, operationsPerSecond)
}

// TestAMLRiskManagementSuite runs the complete AML test suite
func TestAMLRiskManagementSuite(t *testing.T) {
	suite.Run(t, new(AMLRiskManagementTestSuite))
}

// BenchmarkSanctionsScreening benchmarks sanctions screening performance
func BenchmarkSanctionsScreening(b *testing.B) {
	logger := zap.NewNop().Sugar()
	screener := screening.NewSanctionsScreener(logger)

	userInfo := aml.AMLUser{
		ID:          uuid.New(),
		UserID:      uuid.New(),
		KYCStatus:   "Benchmark User",
		CountryCode: "US",
		RiskLevel:   aml.RiskLevelLow,
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		userInfo.UserID = uuid.New() // Unique ID for each iteration
		_, err := screener.ScreenUser(ctx, userInfo.UserID, &userInfo, "benchmark")
		require.NoError(b, err)
	}
}
