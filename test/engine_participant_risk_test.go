package participants

import (
	"context"
	"testing"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/risk"
	"github.com/Aidin1998/pincex_unified/internal/trading/engine"
	"github.com/Aidin1998/pincex_unified/internal/trading/migration"
	"github.com/Aidin1998/pincex_unified/internal/trading/model"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

// MockRiskService is a mock implementation of risk.RiskService for testing
type MockRiskService struct {
	mock.Mock
}

func (m *MockRiskService) ProcessTrade(ctx context.Context, tradeID string, userID string, market string, quantity decimal.Decimal, price decimal.Decimal) error {
	args := m.Called(ctx, tradeID, userID, market, quantity, price)
	return args.Error(0)
}

func (m *MockRiskService) CheckPositionLimit(ctx context.Context, userID string, market string, quantity decimal.Decimal, price decimal.Decimal) (bool, error) {
	args := m.Called(ctx, userID, market, quantity, price)
	return args.Bool(0), args.Error(1)
}

func (m *MockRiskService) CalculateRisk(ctx context.Context, userID string) (*risk.UserRiskProfile, error) {
	args := m.Called(ctx, userID)
	return args.Get(0).(*risk.UserRiskProfile), args.Error(1)
}

func (m *MockRiskService) ComplianceCheck(ctx context.Context, transactionID string, userID string, amount decimal.Decimal, attrs map[string]interface{}) (*risk.ComplianceResult, error) {
	args := m.Called(ctx, transactionID, userID, amount, attrs)
	return args.Get(0).(*risk.ComplianceResult), args.Error(1)
}

func (m *MockRiskService) GenerateReport(ctx context.Context, reportType string, startTime, endTime int64) (string, error) {
	args := m.Called(ctx, reportType, startTime, endTime)
	return args.String(0), args.Error(1)
}

func (m *MockRiskService) GetLimits(ctx context.Context) (risk.LimitConfig, error) {
	args := m.Called(ctx)
	return args.Get(0).(risk.LimitConfig), args.Error(1)
}

func (m *MockRiskService) CreateRiskLimit(ctx context.Context, limitType risk.LimitType, identifier string, limit decimal.Decimal) error {
	args := m.Called(ctx, limitType, identifier, limit)
	return args.Error(0)
}

func (m *MockRiskService) UpdateRiskLimit(ctx context.Context, limitType risk.LimitType, identifier string, limit decimal.Decimal) error {
	args := m.Called(ctx, limitType, identifier, limit)
	return args.Error(0)
}

func (m *MockRiskService) DeleteRiskLimit(ctx context.Context, limitType risk.LimitType, identifier string) error {
	args := m.Called(ctx, limitType, identifier)
	return args.Error(0)
}

func (m *MockRiskService) GetExemptions(ctx context.Context) ([]string, error) {
	args := m.Called(ctx)
	return args.Get(0).([]string), args.Error(1)
}

func (m *MockRiskService) CreateExemption(ctx context.Context, userID string) error {
	args := m.Called(ctx, userID)
	return args.Error(0)
}

func (m *MockRiskService) DeleteExemption(ctx context.Context, userID string) error {
	args := m.Called(ctx, userID)
	return args.Error(0)
}

func (m *MockRiskService) UpdateMarketData(ctx context.Context, symbol string, price, volatility decimal.Decimal) error {
	args := m.Called(ctx, symbol, price, volatility)
	return args.Error(0)
}

func (m *MockRiskService) CalculateRealTimeRisk(ctx context.Context, userID string) (*risk.RiskMetrics, error) {
	args := m.Called(ctx, userID)
	return args.Get(0).(*risk.RiskMetrics), args.Error(1)
}

func (m *MockRiskService) BatchCalculateRisk(ctx context.Context, userIDs []string) (map[string]*risk.RiskMetrics, error) {
	args := m.Called(ctx, userIDs)
	return args.Get(0).(map[string]*risk.RiskMetrics), args.Error(1)
}

func (m *MockRiskService) ValidateCalculationPerformance(ctx context.Context, userID string) error {
	args := m.Called(ctx, userID)
	return args.Error(0)
}

func (m *MockRiskService) RecordTransaction(ctx context.Context, transaction risk.TransactionRecord) error {
	args := m.Called(ctx, transaction)
	return args.Error(0)
}

func (m *MockRiskService) GetActiveComplianceAlerts(ctx context.Context) ([]risk.ComplianceAlert, error) {
	args := m.Called(ctx)
	return args.Get(0).([]risk.ComplianceAlert), args.Error(1)
}

func (m *MockRiskService) UpdateComplianceAlertStatus(ctx context.Context, alertID, status, assignedTo, notes string) error {
	args := m.Called(ctx, alertID, status, assignedTo, notes)
	return args.Error(0)
}

func (m *MockRiskService) AddComplianceRule(ctx context.Context, rule *risk.ComplianceRule) error {
	args := m.Called(ctx, rule)
	return args.Error(0)
}

func (m *MockRiskService) GetDashboardMetrics(ctx context.Context) (*risk.DashboardMetrics, error) {
	args := m.Called(ctx)
	return args.Get(0).(*risk.DashboardMetrics), args.Error(1)
}

func (m *MockRiskService) SubscribeToDashboard(ctx context.Context, subscriberID string, filters map[string]interface{}) (*risk.DashboardSubscriber, error) {
	args := m.Called(ctx, subscriberID, filters)
	return args.Get(0).(*risk.DashboardSubscriber), args.Error(1)
}

func (m *MockRiskService) UnsubscribeFromDashboard(subscriberID string) {
	m.Called(subscriberID)
}

func (m *MockRiskService) SendAlert(alertType, priority, title, message, userID string, data map[string]interface{}) {
	m.Called(alertType, priority, title, message, userID, data)
}

func (m *MockRiskService) AcknowledgeAlert(alertID, acknowledgedBy string) error {
	args := m.Called(alertID, acknowledgedBy)
	return args.Error(0)
}

func (m *MockRiskService) GetAlerts(limit int, priority string) []risk.AlertNotification {
	args := m.Called(limit, priority)
	return args.Get(0).([]risk.AlertNotification)
}

func (m *MockRiskService) GenerateRegulatoryReport(ctx context.Context, criteria risk.ReportingCriteria, generatedBy string) (*risk.RegulatoryReport, error) {
	args := m.Called(ctx, criteria, generatedBy)
	return args.Get(0).(*risk.RegulatoryReport), args.Error(1)
}

func (m *MockRiskService) SubmitRegulatoryReport(ctx context.Context, reportID, submittedBy string) error {
	args := m.Called(ctx, reportID, submittedBy)
	return args.Error(0)
}

func (m *MockRiskService) GetRegulatoryReport(reportID string) (*risk.RegulatoryReport, error) {
	args := m.Called(reportID)
	return args.Get(0).(*risk.RegulatoryReport), args.Error(1)
}

func (m *MockRiskService) ListRegulatoryReports(reportType risk.ReportType, status risk.ReportStatus, limit int) []*risk.RegulatoryReport {
	args := m.Called(reportType, status, limit)
	return args.Get(0).([]*risk.RegulatoryReport)
}

// MockEngine is a mock implementation of the adaptive matching engine
type MockEngine struct {
	mock.Mock
}

func (m *MockEngine) GetMigrationState(pair string) (*engine.MigrationState, error) {
	args := m.Called(pair)
	return args.Get(0).(*engine.MigrationState), args.Error(1)
}

func (m *MockEngine) GetMetricsCollector() interface{} {
	args := m.Called()
	return args.Get(0)
}

func (m *MockEngine) StartMigration(pair string, percentage int32) error {
	args := m.Called(pair, percentage)
	return args.Error(0)
}

func (m *MockEngine) ResetMigration(pair string) error {
	args := m.Called(pair)
	return args.Error(0)
}

func TestEngineParticipant_RiskManagementIntegration(t *testing.T) {
	logger := zap.NewNop().Sugar()
	mockRiskService := new(MockRiskService)
	mockEngine := new(MockEngine)

	// Create engine participant with risk management
	participant := NewEngineParticipant("BTCUSD", mockEngine, mockRiskService, logger)

	assert.NotNil(t, participant)
	assert.Equal(t, "engine_BTCUSD", participant.GetID())
	assert.Equal(t, "engine", participant.GetType())
	assert.NotNil(t, participant.riskManager)
}

func TestEngineParticipant_ValidateRiskLimits(t *testing.T) {
	logger := zap.NewNop().Sugar()
	mockRiskService := new(MockRiskService)
	mockEngine := new(MockEngine)

	participant := NewEngineParticipant("BTCUSD", mockEngine, mockRiskService, logger)
	ctx := context.Background()

	// Create test trades
	trades := []*model.Trade{
		{
			ID:       uuid.New(),
			OrderID:  uuid.New(),
			Pair:     "BTCUSD",
			Quantity: decimal.NewFromFloat(1.0),
			Price:    decimal.NewFromFloat(50000),
			Side:     "buy",
		},
		{
			ID:       uuid.New(),
			OrderID:  uuid.New(),
			Pair:     "BTCUSD",
			Quantity: decimal.NewFromFloat(0.5),
			Price:    decimal.NewFromFloat(50000),
			Side:     "sell",
		},
	}

	// Mock successful position limit checks
	for _, trade := range trades {
		mockRiskService.On("CheckPositionLimit", ctx, trade.OrderID.String(), trade.Pair, trade.Quantity, trade.Price).Return(true, nil)
		mockRiskService.On("CalculateRisk", ctx, trade.OrderID.String()).Return(&risk.UserRiskProfile{
			UserID:          trade.OrderID.String(),
			CurrentExposure: decimal.NewFromFloat(10000),
			ValueAtRisk:     decimal.NewFromFloat(1000),
			MarginRequired:  decimal.NewFromFloat(5000),
		}, nil)
	}

	// Test successful validation
	err := participant.validateRiskLimits(ctx, trades)
	assert.NoError(t, err)

	// Test position limit rejection
	mockRiskService.On("CheckPositionLimit", ctx, trades[0].OrderID.String(), trades[0].Pair, trades[0].Quantity, trades[0].Price).Return(false, nil).Maybe()

	err = participant.validateRiskLimits(ctx, trades[:1])
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "position limit exceeded")

	mockRiskService.AssertExpectations(t)
}

func TestEngineParticipant_UpdateRiskMetrics(t *testing.T) {
	logger := zap.NewNop().Sugar()
	mockRiskService := new(MockRiskService)
	mockEngine := new(MockEngine)

	participant := NewEngineParticipant("BTCUSD", mockEngine, mockRiskService, logger)
	ctx := context.Background()

	// Create test trades
	trades := []*model.Trade{
		{
			ID:       uuid.New(),
			OrderID:  uuid.New(),
			Pair:     "BTCUSD",
			Quantity: decimal.NewFromFloat(1.0),
			Price:    decimal.NewFromFloat(50000),
		},
	}

	// Mock successful risk updates
	for _, trade := range trades {
		mockRiskService.On("ProcessTrade", ctx, trade.ID.String(), trade.OrderID.String(), trade.Pair, trade.Quantity, trade.Price).Return(nil)
		mockRiskService.On("UpdateMarketData", ctx, trade.Pair, trade.Price, mock.AnythingOfType("decimal.Decimal")).Return(nil)
	}

	// Test successful update
	err := participant.updateRiskMetrics(ctx, trades)
	assert.NoError(t, err)

	mockRiskService.AssertExpectations(t)
}

func TestEngineParticipant_GenerateRiskReport(t *testing.T) {
	logger := zap.NewNop().Sugar()
	mockRiskService := new(MockRiskService)
	mockEngine := new(MockEngine)

	participant := NewEngineParticipant("BTCUSD", mockEngine, mockRiskService, logger)
	ctx := context.Background()

	// Mock dashboard metrics
	dashboardMetrics := &risk.DashboardMetrics{
		Timestamp:        time.Now(),
		TotalUsers:       100,
		ActivePositions:  50,
		TotalExposure:    decimal.NewFromFloat(1000000),
		TotalVaR:         decimal.NewFromFloat(50000),
		AverageRiskScore: decimal.NewFromFloat(0.3),
		ActiveAlerts:     2,
		CriticalAlerts:   0,
		SystemHealth:     "healthy",
		ExposureByMarket: map[string]decimal.Decimal{
			"BTCUSD": decimal.NewFromFloat(500000),
			"ETHUSD": decimal.NewFromFloat(300000),
			"LTCUSD": decimal.NewFromFloat(200000),
		},
	}

	mockRiskService.On("GetDashboardMetrics", ctx).Return(dashboardMetrics, nil)
	mockRiskService.On("GetActiveComplianceAlerts", ctx).Return([]risk.ComplianceAlert{}, nil)

	// Test successful report generation
	report, err := participant.generateRiskReport(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, report)
	assert.Equal(t, "healthy", report.Status)
	assert.Equal(t, 50000.0, report.RiskMetrics.VaR)
	assert.Equal(t, 0, report.ComplianceStatus.ActiveAlerts)

	mockRiskService.AssertExpectations(t)
}

func TestEngineParticipant_MigrationWithRiskMonitoring(t *testing.T) {
	logger := zap.NewNop().Sugar()
	mockRiskService := new(MockRiskService)
	mockEngine := new(MockEngine)

	participant := NewEngineParticipant("BTCUSD", mockEngine, mockRiskService, logger)
	ctx := context.Background()

	// Setup migration config
	config := &migration.MigrationConfig{
		MigrationPercentage: 100,
		PrepareTimeout:      30 * time.Second,
		CommitTimeout:       30 * time.Second,
		HealthCheckInterval: 5 * time.Second,
	}

	migrationID := uuid.New()

	// Mock healthy dashboard metrics for prepare phase
	healthyMetrics := &risk.DashboardMetrics{
		Timestamp:        time.Now(),
		TotalUsers:       100,
		TotalExposure:    decimal.NewFromFloat(1000000),
		TotalVaR:         decimal.NewFromFloat(10000), // Low risk
		AverageRiskScore: decimal.NewFromFloat(0.1),
		ActiveAlerts:     1,
		SystemHealth:     "healthy",
		ExposureByMarket: map[string]decimal.Decimal{"BTCUSD": decimal.NewFromFloat(1000000)},
	}

	mockRiskService.On("GetDashboardMetrics", ctx).Return(healthyMetrics, nil)
	mockRiskService.On("GetActiveComplianceAlerts", ctx).Return([]risk.ComplianceAlert{}, nil)
	mockEngine.On("StartMigration", "BTCUSD", int32(100)).Return(nil)

	// Test prepare phase with risk validation
	state, err := participant.Prepare(ctx, migrationID, config)
	assert.NoError(t, err)
	assert.NotNil(t, state)
	assert.Equal(t, migration.VoteYes, state.Vote)
	assert.True(t, state.IsHealthy)

	// Test commit phase with risk monitoring
	err = participant.Commit(ctx, migrationID)
	assert.NoError(t, err)

	mockRiskService.AssertExpectations(t)
	mockEngine.AssertExpectations(t)
}

func TestEngineParticipant_RiskAwareMigrationAbort(t *testing.T) {
	logger := zap.NewNop().Sugar()
	mockRiskService := new(MockRiskService)
	mockEngine := new(MockEngine)

	participant := NewEngineParticipant("BTCUSD", mockEngine, mockRiskService, logger)
	ctx := context.Background()

	// Create migration plan with short duration for test
	plan := &EngineMigrationPlan{
		Strategy:          "gradual_migration",
		EstimatedDuration: 2 * time.Second,
		RiskLevel:         "medium",
	}

	// Mock critical risk condition that should abort migration
	criticalMetrics := &risk.DashboardMetrics{
		Timestamp:        time.Now(),
		TotalUsers:       100,
		TotalExposure:    decimal.NewFromFloat(10000000), // High exposure
		TotalVaR:         decimal.NewFromFloat(100000),   // High VaR
		AverageRiskScore: decimal.NewFromFloat(0.9),      // High risk score
		ActiveAlerts:     10,                             // Many alerts
		CriticalAlerts:   5,
		SystemHealth:     "critical",
		ExposureByMarket: map[string]decimal.Decimal{"BTCUSD": decimal.NewFromFloat(10000000)},
	}

	mockRiskService.On("GetDashboardMetrics", ctx).Return(criticalMetrics, nil)
	mockRiskService.On("GetActiveComplianceAlerts", ctx).Return(make([]risk.ComplianceAlert, 10), nil)
	mockEngine.On("StartMigration", "BTCUSD", int32(100)).Return(nil)

	// Test that migration is aborted due to critical risk
	err := participant.executeRiskAwareMigration(ctx, plan)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "critical risk level detected")

	mockRiskService.AssertExpectations(t)
}

func TestEngineParticipant_CalculateRiskAdjustment(t *testing.T) {
	logger := zap.NewNop().Sugar()
	mockRiskService := new(MockRiskService)
	mockEngine := new(MockEngine)

	participant := NewEngineParticipant("BTCUSD", mockEngine, mockRiskService, logger)
	ctx := context.Background()

	// Create test trades
	trades := []*model.Trade{
		{
			ID:       uuid.New(),
			OrderID:  uuid.New(),
			Pair:     "BTCUSD",
			Quantity: decimal.NewFromFloat(1.0),
			Price:    decimal.NewFromFloat(50000),
		},
	}

	// Mock risk metrics with high risk
	highRiskMetrics := map[string]*risk.RiskMetrics{
		trades[0].OrderID.String(): {
			UserID:         trades[0].OrderID.String(),
			TotalExposure:  decimal.NewFromFloat(600000), // High exposure
			ValueAtRisk:    decimal.NewFromFloat(15000),  // High VaR
			MarginRequired: decimal.NewFromFloat(120000),
			RiskScore:      decimal.NewFromFloat(0.8),
			LastUpdated:    time.Now(),
		},
	}

	mockRiskService.On("BatchCalculateRisk", ctx, []string{trades[0].OrderID.String()}).Return(highRiskMetrics, nil)

	// Test risk adjustment calculation
	adjustment, err := participant.calculateRiskAdjustment(ctx, trades)
	assert.NoError(t, err)
	assert.NotNil(t, adjustment)
	assert.Less(t, adjustment.AdjustmentFactor, 1.0) // Should recommend reduction
	assert.Equal(t, "reduce_exposure", adjustment.RecommendedAction)
	assert.Equal(t, "high", adjustment.RiskLevel)

	mockRiskService.AssertExpectations(t)
}
