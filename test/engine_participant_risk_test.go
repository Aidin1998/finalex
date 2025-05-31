package trading_test

import (
	"context"
	"testing"

	"github.com/Aidin1998/pincex_unified/internal/compliance/aml"
	"github.com/Aidin1998/pincex_unified/internal/trading/engine"
	"github.com/Aidin1998/pincex_unified/internal/trading/migration/participants"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

// MockRiskService is a mock implementation of aml.RiskService for testing
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

func (m *MockRiskService) CalculateRisk(ctx context.Context, userID string) (*aml.UserRiskProfile, error) {
	args := m.Called(ctx, userID)
	return args.Get(0).(*aml.UserRiskProfile), args.Error(1)
}

func (m *MockRiskService) ComplianceCheck(ctx context.Context, transactionID string, userID string, amount decimal.Decimal, attrs map[string]interface{}) (*aml.ComplianceResult, error) {
	args := m.Called(ctx, transactionID, userID, amount, attrs)
	return args.Get(0).(*aml.ComplianceResult), args.Error(1)
}

func (m *MockRiskService) GenerateReport(ctx context.Context, reportType string, startTime, endTime int64) (string, error) {
	args := m.Called(ctx, reportType, startTime, endTime)
	return args.String(0), args.Error(1)
}

func (m *MockRiskService) GetLimits(ctx context.Context) (aml.LimitConfig, error) {
	args := m.Called(ctx)
	return args.Get(0).(aml.LimitConfig), args.Error(1)
}

func (m *MockRiskService) CreateRiskLimit(ctx context.Context, limitType aml.LimitType, identifier string, limit decimal.Decimal) error {
	args := m.Called(ctx, limitType, identifier, limit)
	return args.Error(0)
}

func (m *MockRiskService) UpdateRiskLimit(ctx context.Context, limitType aml.LimitType, identifier string, limit decimal.Decimal) error {
	args := m.Called(ctx, limitType, identifier, limit)
	return args.Error(0)
}

func (m *MockRiskService) DeleteRiskLimit(ctx context.Context, limitType aml.LimitType, identifier string) error {
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

func (m *MockRiskService) CalculateRealTimeRisk(ctx context.Context, userID string) (*aml.RiskMetrics, error) {
	args := m.Called(ctx, userID)
	return args.Get(0).(*aml.RiskMetrics), args.Error(1)
}

func (m *MockRiskService) BatchCalculateRisk(ctx context.Context, userIDs []string) (map[string]*aml.RiskMetrics, error) {
	args := m.Called(ctx, userIDs)
	return args.Get(0).(map[string]*aml.RiskMetrics), args.Error(1)
}

func (m *MockRiskService) ValidateCalculationPerformance(ctx context.Context, userID string) error {
	args := m.Called(ctx, userID)
	return args.Error(0)
}

func (m *MockRiskService) RecordTransaction(ctx context.Context, transaction aml.TransactionRecord) error {
	args := m.Called(ctx, transaction)
	return args.Error(0)
}

func (m *MockRiskService) GetActiveComplianceAlerts(ctx context.Context) ([]aml.ComplianceAlert, error) {
	args := m.Called(ctx)
	return args.Get(0).([]aml.ComplianceAlert), args.Error(1)
}

func (m *MockRiskService) UpdateComplianceAlertStatus(ctx context.Context, alertID, status, assignedTo, notes string) error {
	args := m.Called(ctx, alertID, status, assignedTo, notes)
	return args.Error(0)
}

func (m *MockRiskService) AddComplianceRule(ctx context.Context, rule *aml.ComplianceRule) error {
	args := m.Called(ctx, rule)
	return args.Error(0)
}

func (m *MockRiskService) GetDashboardMetrics(ctx context.Context) (*aml.DashboardMetrics, error) {
	args := m.Called(ctx)
	return args.Get(0).(*aml.DashboardMetrics), args.Error(1)
}

func (m *MockRiskService) SubscribeToDashboard(ctx context.Context, subscriberID string, filters map[string]interface{}) (*aml.DashboardSubscriber, error) {
	args := m.Called(ctx, subscriberID, filters)
	return args.Get(0).(*aml.DashboardSubscriber), args.Error(1)
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

func (m *MockRiskService) GetAlerts(limit int, priority string) []aml.AlertNotification {
	args := m.Called(limit, priority)
	return args.Get(0).([]aml.AlertNotification)
}

func (m *MockRiskService) GenerateRegulatoryReport(ctx context.Context, criteria aml.ReportingCriteria, generatedBy string) (*aml.RegulatoryReport, error) {
	args := m.Called(ctx, criteria, generatedBy)
	return args.Get(0).(*aml.RegulatoryReport), args.Error(1)
}

func (m *MockRiskService) SubmitRegulatoryReport(ctx context.Context, reportID, submittedBy string) error {
	args := m.Called(ctx, reportID, submittedBy)
	return args.Error(0)
}

func (m *MockRiskService) GetRegulatoryReport(reportID string) (*aml.RegulatoryReport, error) {
	args := m.Called(reportID)
	return args.Get(0).(*aml.RegulatoryReport), args.Error(1)
}

func (m *MockRiskService) ListRegulatoryReports(reportType aml.ReportType, status aml.ReportStatus, limit int) []*aml.RegulatoryReport {
	args := m.Called(reportType, status, limit)
	return args.Get(0).([]*aml.RegulatoryReport)
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

// Remove stubAdaptiveMatchingEngine and use real AdaptiveMatchingEngine
func TestEngineParticipant_ExportedMethods(t *testing.T) {
	logger := zap.NewNop().Sugar()
	// Use default config and nils for stub dependencies
	adaptiveEngine := engine.NewAdaptiveMatchingEngine(nil, nil, logger, engine.DefaultAdaptiveEngineConfig(), nil, nil)
	mockRiskService := new(MockRiskService)

	participant := participants.NewEngineParticipant("BTCUSD", adaptiveEngine, mockRiskService, logger)

	assert.NotNil(t, participant)
	assert.Equal(t, "engine_BTCUSD", participant.GetID())
	assert.Equal(t, "engine", participant.GetType())

	// You can add tests for Prepare, Commit, etc. if needed, using only exported methods
}
