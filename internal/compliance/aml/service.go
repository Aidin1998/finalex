package risk

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

// RiskService defines core risk management operations
// It handles position tracking, risk calculations, and compliance checks.
type RiskService interface {
	// Process trade event to update positions and risk metrics
	ProcessTrade(ctx context.Context, tradeID string, userID string, market string, quantity decimal.Decimal, price decimal.Decimal) error

	// Check if a new order is within position limits
	CheckPositionLimit(ctx context.Context, userID string, market string, quantity decimal.Decimal, price decimal.Decimal) (bool, error)

	// Calculate current risk metrics for a user
	CalculateRisk(ctx context.Context, userID string) (*UserRiskProfile, error)

	// Perform compliance checks on a transaction
	ComplianceCheck(ctx context.Context, transactionID string, userID string, amount decimal.Decimal, attrs map[string]interface{}) (*ComplianceResult, error)

	// Generate regulatory report for a given time period
	GenerateReport(ctx context.Context, reportType string, startTime, endTime int64) (string, error)
	// Extended RiskService methods for limit and exemption management
	GetLimits(ctx context.Context) (LimitConfig, error)
	CreateRiskLimit(ctx context.Context, limitType LimitType, identifier string, limit decimal.Decimal) error
	UpdateRiskLimit(ctx context.Context, limitType LimitType, identifier string, limit decimal.Decimal) error
	DeleteRiskLimit(ctx context.Context, limitType LimitType, identifier string) error
	GetExemptions(ctx context.Context) ([]string, error)
	CreateExemption(ctx context.Context, userID string) error
	DeleteExemption(ctx context.Context, userID string) error

	// Real-time risk calculation methods
	UpdateMarketData(ctx context.Context, symbol string, price, volatility decimal.Decimal) error
	CalculateRealTimeRisk(ctx context.Context, userID string) (*RiskMetrics, error)
	BatchCalculateRisk(ctx context.Context, userIDs []string) (map[string]*RiskMetrics, error)
	ValidateCalculationPerformance(ctx context.Context, userID string) error

	// Compliance and monitoring methods
	RecordTransaction(ctx context.Context, transaction TransactionRecord) error
	GetActiveComplianceAlerts(ctx context.Context) ([]ComplianceAlert, error)
	UpdateComplianceAlertStatus(ctx context.Context, alertID, status, assignedTo, notes string) error
	AddComplianceRule(ctx context.Context, rule *ComplianceRule) error

	// Dashboard and monitoring methods
	GetDashboardMetrics(ctx context.Context) (*DashboardMetrics, error)
	SubscribeToDashboard(ctx context.Context, subscriberID string, filters map[string]interface{}) (*DashboardSubscriber, error)
	UnsubscribeFromDashboard(subscriberID string)
	SendAlert(alertType, priority, title, message, userID string, data map[string]interface{})
	AcknowledgeAlert(alertID, acknowledgedBy string) error
	GetAlerts(limit int, priority string) []AlertNotification
	// Regulatory reporting methods
	GenerateRegulatoryReport(ctx context.Context, criteria ReportingCriteria, generatedBy string) (*RegulatoryReport, error)
	SubmitRegulatoryReport(ctx context.Context, reportID, submittedBy string) error
	GetRegulatoryReport(reportID string) (*RegulatoryReport, error)
	ListRegulatoryReports(reportType ReportType, status ReportStatus, limit int) []*RegulatoryReport
}

// UserRiskProfile holds risk metrics for a user
type UserRiskProfile struct {
	UserID          string
	CurrentExposure decimal.Decimal
	ValueAtRisk     decimal.Decimal
	MarginRequired  decimal.Decimal
	UtilizedLimits  map[string]decimal.Decimal // limit type to utilization
}

// ComplianceResult holds results of compliance checks
type ComplianceResult struct {
	TransactionID string
	IsSuspicious  bool
	Flags         []string
	AlertRaised   bool
}

// riskService is the default implementation of RiskService
type riskService struct {
	pm               *PositionManager     // position manager for tracking and limits
	exemptions       map[string]struct{}  // userID -> exemption flag
	calculator       *RiskCalculator      // real-time risk calculation engine
	complianceEngine *ComplianceEngine    // compliance monitoring and pattern detection
	dashboard        *MonitoringDashboard // real-time monitoring dashboard
	reporter         *RegulatoryReporter  // regulatory reporting module
}

// NewRiskService creates an instance of RiskService with default limits and exemptions
func NewRiskService() RiskService {
	pm := NewPositionManager(LimitConfig{
		UserLimits:   make(map[string]decimal.Decimal),
		MarketLimits: make(map[string]decimal.Decimal),
		GlobalLimit:  decimal.Zero,
	})

	// Initialize risk calculator
	calculator := NewRiskCalculator(pm)

	// Initialize compliance engine
	complianceEngine := NewComplianceEngine()

	// Initialize monitoring dashboard
	dashboard := NewMonitoringDashboard(calculator, complianceEngine, pm)

	// Initialize regulatory reporter
	reporter := NewRegulatoryReporter(complianceEngine, calculator, pm)

	return &riskService{
		pm:               pm,
		exemptions:       make(map[string]struct{}),
		calculator:       calculator,
		complianceEngine: complianceEngine,
		dashboard:        dashboard,
		reporter:         reporter,
	}
}

// ProcessTrade updates positions based on a trade event
func (r *riskService) ProcessTrade(ctx context.Context, tradeID, userID, market string, quantity decimal.Decimal, price decimal.Decimal) error {
	// Update positions
	err := r.pm.ProcessTrade(ctx, userID, market, quantity, price)
	if err != nil {
		return err
	}

	// Update market data for risk calculations
	volatility := decimal.NewFromFloat(0.02) // Default 2% volatility
	r.calculator.UpdateMarketData(ctx, market, price, volatility)

	// Record transaction for compliance monitoring
	transaction := TransactionRecord{
		ID:          tradeID,
		UserID:      userID,
		Type:        "trade",
		Amount:      quantity.Mul(price).Abs(),
		Currency:    "USD", // Simplified
		Timestamp:   time.Now(),
		Source:      "trading_engine",
		Destination: "position",
		Metadata: map[string]interface{}{
			"market":   market,
			"quantity": quantity.String(),
			"price":    price.String(),
		},
	}

	return r.complianceEngine.RecordTransaction(ctx, transaction)
}

// CheckPositionLimit verifies a new order against configured limits
func (r *riskService) CheckPositionLimit(ctx context.Context, userID, market string, quantity decimal.Decimal, price decimal.Decimal) (bool, error) {
	if _, exempt := r.exemptions[userID]; exempt {
		return true, nil
	}
	return r.pm.CheckLimit(ctx, userID, market, quantity)
}

// CalculateRisk computes current exposure, VaR, and margin for a user
func (r *riskService) CalculateRisk(ctx context.Context, userID string) (*UserRiskProfile, error) {
	positions := r.pm.ListPositions(ctx, userID)
	totalExposure := decimal.Zero
	for market, pos := range positions {
		expo := pos.Quantity.Abs().Mul(pos.AvgPrice)
		totalExposure = totalExposure.Add(expo)
		_ = market
	}
	varValue := totalExposure.Mul(decimal.NewFromFloat(0.05))
	margin := totalExposure.Mul(decimal.NewFromFloat(0.2))
	util := make(map[string]decimal.Decimal)
	return &UserRiskProfile{
		UserID:          userID,
		CurrentExposure: totalExposure,
		ValueAtRisk:     varValue,
		MarginRequired:  margin,
		UtilizedLimits:  util,
	}, nil
}

// ComplianceCheck performs AML/KYT checks on a transaction
func (r *riskService) ComplianceCheck(ctx context.Context, transactionID, userID string, amount decimal.Decimal, attrs map[string]interface{}) (*ComplianceResult, error) {
	suspicious := amount.GreaterThan(decimal.NewFromInt(100000))
	flags := []string{}
	if suspicious {
		flags = append(flags, "amount_exceeds_threshold")
	}
	return &ComplianceResult{
		TransactionID: transactionID,
		IsSuspicious:  suspicious,
		Flags:         flags,
		AlertRaised:   suspicious,
	}, nil
}

// GenerateReport returns a JSON report for given period
func (r *riskService) GenerateReport(ctx context.Context, reportType string, startTime, endTime int64) (string, error) {
	report := map[string]interface{}{
		"type":       reportType,
		"start_time": time.Unix(0, startTime).UTC(),
		"end_time":   time.Unix(0, endTime).UTC(),
		"generated":  time.Now().UTC(),
	}
	data, err := json.Marshal(report)
	if err != nil {
		return "", fmt.Errorf("report serialization failed: %w", err)
	}
	return string(data), nil
}

// GetLimits returns current limit configurations
func (r *riskService) GetLimits(ctx context.Context) (LimitConfig, error) {
	return r.pm.limits, nil
}

// CreateRiskLimit adds a new limit for given type and identifier
func (r *riskService) CreateRiskLimit(ctx context.Context, limitType LimitType, identifier string, limit decimal.Decimal) error {
	switch limitType {
	case UserLimitType:
		r.pm.limits.UserLimits[identifier] = limit
	case MarketLimitType:
		r.pm.limits.MarketLimits[identifier] = limit
	case GlobalLimitType:
		r.pm.limits.GlobalLimit = limit
	default:
		return fmt.Errorf("unknown limit type: %s", limitType)
	}
	return nil
}

// UpdateRiskLimit updates an existing limit
func (r *riskService) UpdateRiskLimit(ctx context.Context, limitType LimitType, identifier string, limit decimal.Decimal) error {
	return r.CreateRiskLimit(ctx, limitType, identifier, limit)
}

// DeleteRiskLimit removes a configured limit
func (r *riskService) DeleteRiskLimit(ctx context.Context, limitType LimitType, identifier string) error {
	switch limitType {
	case UserLimitType:
		delete(r.pm.limits.UserLimits, identifier)
	case MarketLimitType:
		delete(r.pm.limits.MarketLimits, identifier)
	case GlobalLimitType:
		r.pm.limits.GlobalLimit = decimal.Zero
	default:
		return fmt.Errorf("unknown limit type: %s", limitType)
	}
	return nil
}

// GetExemptions returns all user exemptions
func (r *riskService) GetExemptions(ctx context.Context) ([]string, error) {
	ids := make([]string, 0, len(r.exemptions))
	for id := range r.exemptions {
		ids = append(ids, id)
	}
	return ids, nil
}

// CreateExemption adds a user exemption
func (r *riskService) CreateExemption(ctx context.Context, userID string) error {
	r.exemptions[userID] = struct{}{}
	return nil
}

// DeleteExemption removes a user exemption
func (r *riskService) DeleteExemption(ctx context.Context, userID string) error {
	delete(r.exemptions, userID)
	return nil
}

// AcknowledgeAlert marks an alert as acknowledged (stub implementation)
func (r *riskService) AcknowledgeAlert(alertID, acknowledgedBy string) error {
	// In a real implementation, this would update alert state in DB or memory
	return nil
}

// AddComplianceRule adds a compliance rule (stub implementation)
func (r *riskService) AddComplianceRule(ctx context.Context, rule *ComplianceRule) error {
	// In a real implementation, this would add the rule to the compliance engine
	return nil
}

// BatchCalculateRisk calculates risk for multiple users (stub implementation)
func (r *riskService) BatchCalculateRisk(ctx context.Context, userIDs []string) (map[string]*RiskMetrics, error) {
	result := make(map[string]*RiskMetrics)
	for _, userID := range userIDs {
		result[userID] = &RiskMetrics{}
	}
	return result, nil
}

// CalculateRealTimeRisk calculates real-time risk metrics for a user (stub implementation)
func (r *riskService) CalculateRealTimeRisk(ctx context.Context, userID string) (*RiskMetrics, error) {
	return &RiskMetrics{}, nil
}

// GenerateRegulatoryReport generates a regulatory report (stub implementation)
func (r *riskService) GenerateRegulatoryReport(ctx context.Context, criteria ReportingCriteria, generatedBy string) (*RegulatoryReport, error) {
	return &RegulatoryReport{}, nil
}

// GetActiveComplianceAlerts returns active compliance alerts (stub implementation)
func (r *riskService) GetActiveComplianceAlerts(ctx context.Context) ([]ComplianceAlert, error) {
	return []ComplianceAlert{}, nil
}

// GetAlerts returns alert notifications (stub implementation)
func (r *riskService) GetAlerts(limit int, priority string) []AlertNotification {
	return []AlertNotification{}
}

// GetDashboardMetrics returns dashboard metrics (stub implementation)
func (r *riskService) GetDashboardMetrics(ctx context.Context) (*DashboardMetrics, error) {
	return &DashboardMetrics{}, nil
}

// GetRegulatoryReport returns a regulatory report (stub implementation)
func (r *riskService) GetRegulatoryReport(reportID string) (*RegulatoryReport, error) {
	return &RegulatoryReport{}, nil
}

// ListRegulatoryReports returns a list of regulatory reports (stub implementation)
func (r *riskService) ListRegulatoryReports(reportType ReportType, status ReportStatus, limit int) []*RegulatoryReport {
	return []*RegulatoryReport{}
}
