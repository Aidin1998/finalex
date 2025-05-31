package test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/risk"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.uber.org/zap/zaptest"
)

// RiskManagementTestSuite provides comprehensive tests for risk management functionality
type RiskManagementTestSuite struct {
	suite.Suite
	riskService risk.RiskService
	logger      *zaptest.Logger
	ctx         context.Context
}

// SetupSuite initializes the test suite
func (suite *RiskManagementTestSuite) SetupSuite() {
	suite.logger = zaptest.NewLogger(suite.T())
	suite.ctx = context.Background()

	// Initialize risk service with test configuration
	config := &risk.Config{
		PositionLimits: risk.PositionLimits{
			DefaultUserLimit:   decimal.NewFromInt(100000),
			DefaultMarketLimit: decimal.NewFromInt(1000000),
			GlobalLimit:        decimal.NewFromInt(10000000),
		},
		RiskCalculation: risk.RiskCalculationConfig{
			VaRConfidence:    0.95,
			VaRTimeHorizon:   1,
			UpdateInterval:   time.Second,
			BatchSize:        100,
			PerformanceLimit: 500 * time.Millisecond,
		},
		Compliance: risk.ComplianceConfig{
			EnableAML:                true,
			EnableKYT:                true,
			TransactionVelocityLimit: 10,
			DailyVolumeThreshold:     decimal.NewFromInt(50000),
			StructuringThreshold:     decimal.NewFromInt(10000),
		},
		Dashboard: risk.DashboardConfig{
			RefreshInterval:  time.Second * 5,
			AlertRetention:   time.Hour * 24,
			MetricsRetention: time.Hour * 168, // 1 week
			MaxSubscribers:   1000,
		},
		Reporting: risk.ReportingConfig{
			EnableSAR:         true,
			EnableCTR:         true,
			EnableLTR:         true,
			ReportRetention:   time.Hour * 24 * 365, // 1 year
			SubmissionTimeout: time.Minute * 30,
			BatchSize:         100,
		},
	}

	suite.riskService = risk.NewService(config, suite.logger.Logger, nil)
}

// TestPositionLimitEnforcement tests position limit enforcement
func (suite *RiskManagementTestSuite) TestPositionLimitEnforcement() {
	t := suite.T()
	ctx := suite.ctx

	userID := "test-user-001"
	market := "BTC/USD"

	// Test within limits
	allowed, err := suite.riskService.CheckPositionLimit(ctx, userID, market, decimal.NewFromInt(1000), decimal.NewFromInt(50000))
	require.NoError(t, err)
	assert.True(t, allowed)

	// Test exceeding limits
	allowed, err = suite.riskService.CheckPositionLimit(ctx, userID, market, decimal.NewFromInt(200000), decimal.NewFromInt(50000))
	require.NoError(t, err)
	assert.False(t, allowed)
}

// TestRealTimeRiskCalculation tests real-time risk metric calculations
func (suite *RiskManagementTestSuite) TestRealTimeRiskCalculation() {
	t := suite.T()
	ctx := suite.ctx

	userID := "test-user-002"

	// Update market data
	err := suite.riskService.UpdateMarketData(ctx, "BTC/USD", decimal.NewFromInt(50000), decimal.NewFromFloat(0.15))
	require.NoError(t, err)

	// Process some trades to build position
	err = suite.riskService.ProcessTrade(ctx, "trade-001", userID, "BTC/USD", decimal.NewFromFloat(0.5), decimal.NewFromInt(50000))
	require.NoError(t, err)

	// Calculate real-time risk
	metrics, err := suite.riskService.CalculateRealTimeRisk(ctx, userID)
	require.NoError(t, err)
	require.NotNil(t, metrics)

	assert.Greater(t, metrics.TotalExposure.InexactFloat64(), 0.0)
	assert.GreaterOrEqual(t, metrics.VaR.InexactFloat64(), 0.0)
}

// TestBatchRiskCalculation tests batch risk calculation performance
func (suite *RiskManagementTestSuite) TestBatchRiskCalculation() {
	t := suite.T()
	ctx := suite.ctx

	// Create test users
	userIDs := []string{"batch-user-001", "batch-user-002", "batch-user-003"}

	// Setup positions for users
	for _, userID := range userIDs {
		err := suite.riskService.ProcessTrade(ctx, "trade-"+userID, userID, "BTC/USD", decimal.NewFromFloat(0.1), decimal.NewFromInt(50000))
		require.NoError(t, err)
	}

	// Batch calculate risk
	start := time.Now()
	results, err := suite.riskService.BatchCalculateRisk(ctx, userIDs)
	duration := time.Since(start)

	require.NoError(t, err)
	require.Len(t, results, len(userIDs))

	// Validate performance - should be sub-second
	assert.Less(t, duration, time.Second)

	// Validate all users have risk metrics
	for _, userID := range userIDs {
		metrics, exists := results[userID]
		assert.True(t, exists)
		assert.NotNil(t, metrics)
	}
}

// TestComplianceRuleEngine tests compliance rule execution
func (suite *RiskManagementTestSuite) TestComplianceRuleEngine() {
	t := suite.T()
	ctx := suite.ctx

	// Add compliance rule
	rule := &risk.ComplianceRule{
		ID:          "velocity-rule-001",
		Name:        "High Velocity Detection",
		Description: "Detect high frequency transactions",
		Type:        "velocity",
		Parameters: map[string]interface{}{
			"max_transactions": 5,
			"time_window":      300, // 5 minutes
		},
		Severity: "HIGH",
		Enabled:  true,
	}

	err := suite.riskService.AddComplianceRule(ctx, rule)
	require.NoError(t, err)

	// Test transaction that should trigger the rule
	userID := "compliance-user-001"
	for i := 0; i < 6; i++ {
		transaction := risk.TransactionRecord{
			ID:        "tx-" + string(rune(i)),
			UserID:    userID,
			Amount:    decimal.NewFromInt(1000),
			Currency:  "USD",
			Type:      "deposit",
			Timestamp: time.Now(),
			Metadata:  map[string]interface{}{"source": "bank_transfer"},
		}

		err := suite.riskService.RecordTransaction(ctx, transaction)
		require.NoError(t, err)
	}

	// Check for compliance alerts
	alerts, err := suite.riskService.GetActiveComplianceAlerts(ctx)
	require.NoError(t, err)

	// Should have at least one alert for velocity rule
	hasVelocityAlert := false
	for _, alert := range alerts {
		if alert.RuleID == "velocity-rule-001" {
			hasVelocityAlert = true
			break
		}
	}
	assert.True(t, hasVelocityAlert)
}

// TestDashboardMetrics tests dashboard metrics collection
func (suite *RiskManagementTestSuite) TestDashboardMetrics() {
	t := suite.T()
	ctx := suite.ctx

	// Generate some activity
	userID := "dashboard-user-001"
	err := suite.riskService.ProcessTrade(ctx, "dashboard-trade-001", userID, "ETH/USD", decimal.NewFromFloat(1.0), decimal.NewFromInt(3000))
	require.NoError(t, err)

	// Get dashboard metrics
	metrics, err := suite.riskService.GetDashboardMetrics(ctx)
	require.NoError(t, err)
	require.NotNil(t, metrics)

	assert.GreaterOrEqual(t, metrics.TotalUsers, int64(1))
	assert.GreaterOrEqual(t, metrics.TotalExposure.InexactFloat64(), 0.0)
	assert.NotNil(t, metrics.SystemHealth)
}

// TestRegulatoryReporting tests regulatory report generation
func (suite *RiskManagementTestSuite) TestRegulatoryReporting() {
	t := suite.T()
	ctx := suite.ctx

	startTime := time.Now().Add(-24 * time.Hour).Unix()
	endTime := time.Now().Unix()

	// Generate SAR report
	reportData, err := suite.riskService.GenerateReport(ctx, "SAR", startTime, endTime)
	require.NoError(t, err)
	assert.NotEmpty(t, reportData)

	// Validate report contains expected structure
	var report map[string]interface{}
	err = json.Unmarshal([]byte(reportData), &report)
	require.NoError(t, err)
	assert.Contains(t, report, "reportType")
	assert.Contains(t, report, "generatedAt")
}

// TestPerformanceRequirements tests sub-second performance requirements
func (suite *RiskManagementTestSuite) TestPerformanceRequirements() {
	t := suite.T()
	ctx := suite.ctx

	userID := "perf-user-001"

	// Test calculation performance validation
	start := time.Now()
	err := suite.riskService.ValidateCalculationPerformance(ctx, userID)
	duration := time.Since(start)

	require.NoError(t, err)

	// Should complete within performance limit
	assert.Less(t, duration, 500*time.Millisecond)
}

// TestIntegrationEndToEnd tests complete risk management workflow
func (suite *RiskManagementTestSuite) TestIntegrationEndToEnd() {
	t := suite.T()
	ctx := suite.ctx

	userID := "e2e-user-001"
	market := "BTC/USD"

	// 1. Update market data
	err := suite.riskService.UpdateMarketData(ctx, market, decimal.NewFromInt(50000), decimal.NewFromFloat(0.2))
	require.NoError(t, err)

	// 2. Check position limits before trade
	allowed, err := suite.riskService.CheckPositionLimit(ctx, userID, market, decimal.NewFromFloat(0.5), decimal.NewFromInt(50000))
	require.NoError(t, err)
	assert.True(t, allowed)

	// 3. Process trade
	err = suite.riskService.ProcessTrade(ctx, "e2e-trade-001", userID, market, decimal.NewFromFloat(0.5), decimal.NewFromInt(50000))
	require.NoError(t, err)

	// 4. Calculate updated risk
	metrics, err := suite.riskService.CalculateRealTimeRisk(ctx, userID)
	require.NoError(t, err)
	require.NotNil(t, metrics)

	// 5. Record compliance transaction
	transaction := risk.TransactionRecord{
		ID:        "e2e-compliance-tx-001",
		UserID:    userID,
		Amount:    decimal.NewFromInt(25000),
		Currency:  "USD",
		Type:      "trade",
		Timestamp: time.Now(),
		Metadata: map[string]interface{}{
			"tradeId": "e2e-trade-001",
			"market":  market,
		},
	}

	err = suite.riskService.RecordTransaction(ctx, transaction)
	require.NoError(t, err)

	// 6. Verify dashboard updates
	dashboardMetrics, err := suite.riskService.GetDashboardMetrics(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, dashboardMetrics.TotalUsers, int64(1))

	// 7. Generate compliance report
	startTime := time.Now().Add(-1 * time.Hour).Unix()
	endTime := time.Now().Unix()

	reportData, err := suite.riskService.GenerateReport(ctx, "COMPLIANCE_SUMMARY", startTime, endTime)
	require.NoError(t, err)
	assert.NotEmpty(t, reportData)
}

// TestAPIEndpointIntegration tests HTTP API endpoints
func (suite *RiskManagementTestSuite) TestAPIEndpointIntegration() {
	t := suite.T()

	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Mock API server with risk endpoints
	api := router.Group("/api/v1")
	{
		risk := api.Group("/risk")
		{
			risk.GET("/metrics/user/:userID", func(c *gin.Context) {
				userID := c.Param("userID")
				metrics, err := suite.riskService.CalculateRealTimeRisk(suite.ctx, userID)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, metrics)
			})

			risk.GET("/dashboard", func(c *gin.Context) {
				metrics, err := suite.riskService.GetDashboardMetrics(suite.ctx)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, metrics)
			})
		}
	}

	// Test user risk metrics endpoint
	userID := "api-test-user-001"
	err := suite.riskService.ProcessTrade(suite.ctx, "api-test-trade", userID, "BTC/USD", decimal.NewFromFloat(0.1), decimal.NewFromInt(50000))
	require.NoError(t, err)

	req, _ := http.NewRequest("GET", "/api/v1/risk/metrics/user/"+userID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response risk.RiskMetrics
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Greater(t, response.TotalExposure.InexactFloat64(), 0.0)

	// Test dashboard endpoint
	req, _ = http.NewRequest("GET", "/api/v1/risk/dashboard", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var dashboardResponse risk.DashboardMetrics
	err = json.Unmarshal(w.Body.Bytes(), &dashboardResponse)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, dashboardResponse.TotalUsers, int64(1))
}

// RunRiskManagementTests runs the comprehensive test suite
func TestRiskManagementSystem(t *testing.T) {
	suite.Run(t, new(RiskManagementTestSuite))
}

// BenchmarkRiskCalculationPerformance benchmarks risk calculation performance
func BenchmarkRiskCalculationPerformance(b *testing.B) {
	logger := zaptest.NewLogger(b)
	config := &risk.Config{
		RiskCalculation: risk.RiskCalculationConfig{
			VaRConfidence:    0.95,
			VaRTimeHorizon:   1,
			UpdateInterval:   time.Second,
			BatchSize:        100,
			PerformanceLimit: 500 * time.Millisecond,
		},
	}

	riskService := risk.NewService(config, logger.Logger, nil)
	ctx := context.Background()
	userID := "benchmark-user"

	// Setup test data
	_ = riskService.UpdateMarketData(ctx, "BTC/USD", decimal.NewFromInt(50000), decimal.NewFromFloat(0.15))
	_ = riskService.ProcessTrade(ctx, "benchmark-trade", userID, "BTC/USD", decimal.NewFromFloat(0.1), decimal.NewFromInt(50000))

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := riskService.CalculateRealTimeRisk(ctx, userID)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkBatchRiskCalculation benchmarks batch risk calculation
func BenchmarkBatchRiskCalculation(b *testing.B) {
	logger := zaptest.NewLogger(b)
	config := &risk.Config{
		RiskCalculation: risk.RiskCalculationConfig{
			VaRConfidence:    0.95,
			VaRTimeHorizon:   1,
			UpdateInterval:   time.Second,
			BatchSize:        100,
			PerformanceLimit: 500 * time.Millisecond,
		},
	}

	riskService := risk.NewService(config, logger.Logger, nil)
	ctx := context.Background()

	// Create batch of users
	userIDs := make([]string, 100)
	for i := range userIDs {
		userIDs[i] = "batch-benchmark-user-" + string(rune(i))
		_ = riskService.ProcessTrade(ctx, "batch-trade-"+string(rune(i)), userIDs[i], "BTC/USD", decimal.NewFromFloat(0.1), decimal.NewFromInt(50000))
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := riskService.BatchCalculateRisk(ctx, userIDs)
		if err != nil {
			b.Fatal(err)
		}
	}
}
