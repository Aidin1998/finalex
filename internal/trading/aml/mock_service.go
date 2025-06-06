// Package aml provides Anti-Money Laundering and compliance services
package aml

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// MockRiskService provides a mock implementation of RiskService for development and testing
type MockRiskService struct {
	logger *zap.Logger
}

// NewMockRiskService creates a new mock risk service
func NewMockRiskService(logger *zap.Logger) RiskService {
	return &MockRiskService{
		logger: logger,
	}
}

// GetLimits returns mock risk limits
func (m *MockRiskService) GetLimits(ctx context.Context) ([]RiskLimit, error) {
	return []RiskLimit{
		{
			ID:     "limit-1",
			Type:   LimitTypeDaily,
			Amount: decimal.NewFromFloat(10000.00),
		},
		{
			ID:     "limit-2",
			Type:   LimitTypeMonthly,
			Amount: decimal.NewFromFloat(100000.00),
		},
	}, nil
}

// CreateRiskLimit creates a mock risk limit
func (m *MockRiskService) CreateRiskLimit(ctx context.Context, limitType LimitType, id string, amount decimal.Decimal) error {
	m.logger.Info("Mock: Creating risk limit",
		zap.String("type", string(limitType)),
		zap.String("id", id),
		zap.String("amount", amount.String()))
	return nil
}

// UpdateRiskLimit updates a mock risk limit
func (m *MockRiskService) UpdateRiskLimit(ctx context.Context, limitType LimitType, id string, amount decimal.Decimal) error {
	m.logger.Info("Mock: Updating risk limit",
		zap.String("type", string(limitType)),
		zap.String("id", id),
		zap.String("amount", amount.String()))
	return nil
}

// DeleteRiskLimit deletes a mock risk limit
func (m *MockRiskService) DeleteRiskLimit(ctx context.Context, limitType LimitType, id string) error {
	m.logger.Info("Mock: Deleting risk limit",
		zap.String("type", string(limitType)),
		zap.String("id", id))
	return nil
}

// GetExemptions returns mock user exemptions
func (m *MockRiskService) GetExemptions(ctx context.Context) ([]UserExemption, error) {
	return []UserExemption{
		{
			ID:     uuid.New().String(),
			UserID: "user-123",
			Reason: "VIP Customer",
		},
	}, nil
}

// CreateExemption creates a mock user exemption
func (m *MockRiskService) CreateExemption(ctx context.Context, userID string) error {
	m.logger.Info("Mock: Creating user exemption", zap.String("user_id", userID))
	return nil
}

// DeleteExemption deletes a mock user exemption
func (m *MockRiskService) DeleteExemption(ctx context.Context, userID string) error {
	m.logger.Info("Mock: Deleting user exemption", zap.String("user_id", userID))
	return nil
}

// CalculateRealTimeRisk calculates mock real-time risk metrics
func (m *MockRiskService) CalculateRealTimeRisk(ctx context.Context, userID string) (interface{}, error) {
	return map[string]interface{}{
		"user_id":       userID,
		"risk_score":    0.25,
		"level":         "low",
		"calculated_at": time.Now(),
	}, nil
}

// BatchCalculateRisk calculates mock risk metrics for multiple users
func (m *MockRiskService) BatchCalculateRisk(ctx context.Context, userIDs []string) (interface{}, error) {
	results := make(map[string]interface{})
	for _, userID := range userIDs {
		results[userID] = map[string]interface{}{
			"risk_score": 0.25,
			"level":      "low",
		}
	}
	return results, nil
}

// GetDashboardMetrics returns mock dashboard metrics
func (m *MockRiskService) GetDashboardMetrics(ctx context.Context) (*DashboardMetrics, error) {
	return &DashboardMetrics{
		ActiveAlerts:        5,
		ResolvedAlerts:      25,
		HighRiskUsers:       3,
		TotalTransactions:   1000,
		FlaggedTransactions: 10,
		ComplianceScore:     0.95,
		MetricsByType: map[string]int64{
			"aml_alerts":          3,
			"kyc_alerts":          2,
			"suspicious_activity": 5,
		},
		LastUpdated: time.Now(),
	}, nil
}

// UpdateMarketData updates mock market data
func (m *MockRiskService) UpdateMarketData(ctx context.Context, symbol string, price, volatility decimal.Decimal) error {
	m.logger.Info("Mock: Updating market data",
		zap.String("symbol", symbol),
		zap.String("price", price.String()),
		zap.String("volatility", volatility.String()))
	return nil
}

// GetActiveComplianceAlerts returns mock active compliance alerts
func (m *MockRiskService) GetActiveComplianceAlerts(ctx context.Context) ([]ComplianceAlert, error) {
	return []ComplianceAlert{
		{
			ID:          uuid.New().String(),
			UserID:      "user-123",
			AlertType:   "suspicious_activity",
			Severity:    "medium",
			Title:       "Unusual Trading Pattern",
			Description: "User has exhibited unusual trading patterns",
			Status:      "active",
			CreatedAt:   time.Now().Add(-2 * time.Hour),
		},
		{
			ID:          uuid.New().String(),
			UserID:      "user-456",
			AlertType:   "large_transaction",
			Severity:    "high",
			Title:       "Large Transaction Alert",
			Description: "Transaction exceeds daily limit",
			Status:      "active",
			CreatedAt:   time.Now().Add(-1 * time.Hour),
		},
	}, nil
}

// UpdateComplianceAlertStatus updates mock compliance alert status
func (m *MockRiskService) UpdateComplianceAlertStatus(ctx context.Context, alertID, status, assignedTo, notes string) error {
	m.logger.Info("Mock: Updating compliance alert status",
		zap.String("alert_id", alertID),
		zap.String("status", status),
		zap.String("assigned_to", assignedTo),
		zap.String("notes", notes))
	return nil
}

// AddComplianceRule adds a mock compliance rule
func (m *MockRiskService) AddComplianceRule(ctx context.Context, rule *ComplianceRule) error {
	if rule.ID == "" {
		rule.ID = uuid.New().String()
	}
	rule.CreatedAt = time.Now()
	rule.UpdatedAt = time.Now()

	m.logger.Info("Mock: Adding compliance rule",
		zap.String("rule_id", rule.ID),
		zap.String("name", rule.Name),
		zap.String("type", rule.RuleType))
	return nil
}

// GenerateReport generates a mock compliance report
func (m *MockRiskService) GenerateReport(ctx context.Context, reportType string, startTime, endTime time.Time) (interface{}, error) {
	m.logger.Info("Mock: Generating report",
		zap.String("report_type", reportType),
		zap.Time("start_time", startTime),
		zap.Time("end_time", endTime))

	return map[string]interface{}{
		"report_type":  reportType,
		"start_time":   startTime,
		"end_time":     endTime,
		"generated_at": time.Now(),
		"data": map[string]interface{}{
			"total_alerts":     50,
			"resolved_alerts":  45,
			"pending_alerts":   5,
			"compliance_score": 0.95,
		},
	}, nil
}
