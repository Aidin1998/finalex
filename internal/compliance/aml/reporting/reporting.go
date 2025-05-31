package risk

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// ReportType defines different types of regulatory reports
type ReportType string

const (
	SuspiciousActivityReport  ReportType = "SAR"      // Suspicious Activity Report
	LargeTransactionReport    ReportType = "LTR"      // Large Transaction Report
	CurrencyTransactionReport ReportType = "CTR"      // Currency Transaction Report
	FinCENReport              ReportType = "FINCEN"   // FinCEN specific reports
	AMLComplianceReport       ReportType = "AML"      // AML Compliance Report
	KYCComplianceReport       ReportType = "KYC"      // KYC Compliance Report
	RiskAssessmentReport      ReportType = "RISK"     // Risk Assessment Report
	PositionReport            ReportType = "POSITION" // Position Report
	ExposureReport            ReportType = "EXPOSURE" // Exposure Report
)

// ReportStatus represents the status of a generated report
type ReportStatus string

const (
	ReportPending   ReportStatus = "pending"
	ReportGenerated ReportStatus = "generated"
	ReportSubmitted ReportStatus = "submitted"
	ReportError     ReportStatus = "error"
)

// RegulatoryReport represents a generated regulatory report
type RegulatoryReport struct {
	ID           string                 `json:"id"`
	Type         ReportType             `json:"type"`
	Status       ReportStatus           `json:"status"`
	Title        string                 `json:"title"`
	Description  string                 `json:"description"`
	PeriodStart  time.Time              `json:"period_start"`
	PeriodEnd    time.Time              `json:"period_end"`
	GeneratedAt  time.Time              `json:"generated_at"`
	GeneratedBy  string                 `json:"generated_by"`
	SubmittedAt  *time.Time             `json:"submitted_at,omitempty"`
	SubmittedBy  string                 `json:"submitted_by,omitempty"`
	Content      string                 `json:"content"`
	Metadata     map[string]interface{} `json:"metadata"`
	FilePath     string                 `json:"file_path,omitempty"`
	Hash         string                 `json:"hash,omitempty"`
	Recipients   []string               `json:"recipients"`
	ErrorMessage string                 `json:"error_message,omitempty"`
}

// ReportingCriteria defines criteria for report generation
type ReportingCriteria struct {
	ReportType     ReportType             `json:"report_type"`
	PeriodStart    time.Time              `json:"period_start"`
	PeriodEnd      time.Time              `json:"period_end"`
	Filters        map[string]interface{} `json:"filters"`
	IncludeDetails bool                   `json:"include_details"`
	Format         string                 `json:"format"` // "json", "csv", "xml", "pdf"
	Recipients     []string               `json:"recipients"`
	AutoSubmit     bool                   `json:"auto_submit"`
}

// SARData represents data for Suspicious Activity Reports
type SARData struct {
	AlertID            string               `json:"alert_id"`
	UserID             string               `json:"user_id"`
	SuspiciousActivity string               `json:"suspicious_activity"`
	TotalAmount        decimal.Decimal      `json:"total_amount"`
	TransactionCount   int                  `json:"transaction_count"`
	PeriodStart        time.Time            `json:"period_start"`
	PeriodEnd          time.Time            `json:"period_end"`
	RiskScore          decimal.Decimal      `json:"risk_score"`
	Patterns           []TransactionPattern `json:"patterns"`
	Transactions       []TransactionRecord  `json:"transactions"`
	Narrative          string               `json:"narrative"`
}

// CTRData represents data for Currency Transaction Reports
type CTRData struct {
	UserID          string                 `json:"user_id"`
	TransactionID   string                 `json:"transaction_id"`
	Amount          decimal.Decimal        `json:"amount"`
	Currency        string                 `json:"currency"`
	TransactionType string                 `json:"transaction_type"`
	TransactionDate time.Time              `json:"transaction_date"`
	Source          string                 `json:"source"`
	Destination     string                 `json:"destination"`
	Purpose         string                 `json:"purpose"`
	CustomerInfo    map[string]interface{} `json:"customer_info"`
}

// RegulatoryReporter handles automated regulatory report generation
type RegulatoryReporter struct {
	complianceEngine *ComplianceEngine
	calculator       *RiskCalculator
	positionManager  *PositionManager

	reports          map[string]*RegulatoryReport
	scheduledReports map[string]*ScheduledReport

	// Configuration
	reportStorePath string
	autoSubmit      bool
	encryptReports  bool
}

// ScheduledReport represents an automatically scheduled report
type ScheduledReport struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Criteria  ReportingCriteria `json:"criteria"`
	Schedule  string            `json:"schedule"` // cron expression
	LastRun   *time.Time        `json:"last_run,omitempty"`
	NextRun   time.Time         `json:"next_run"`
	IsActive  bool              `json:"is_active"`
	CreatedAt time.Time         `json:"created_at"`
	CreatedBy string            `json:"created_by"`
}

// NewRegulatoryReporter creates a new regulatory reporter
func NewRegulatoryReporter(complianceEngine *ComplianceEngine, calculator *RiskCalculator, positionManager *PositionManager) *RegulatoryReporter {
	return &RegulatoryReporter{
		complianceEngine: complianceEngine,
		calculator:       calculator,
		positionManager:  positionManager,
		reports:          make(map[string]*RegulatoryReport),
		scheduledReports: make(map[string]*ScheduledReport),
		reportStorePath:  "/var/reports/regulatory",
		autoSubmit:       false,
		encryptReports:   true,
	}
}

// GenerateReport generates a regulatory report based on criteria
func (rr *RegulatoryReporter) GenerateReport(ctx context.Context, criteria ReportingCriteria, generatedBy string) (*RegulatoryReport, error) {
	reportID := fmt.Sprintf("report_%s_%d", criteria.ReportType, time.Now().UnixNano())

	report := &RegulatoryReport{
		ID:          reportID,
		Type:        criteria.ReportType,
		Status:      ReportPending,
		Title:       rr.getReportTitle(criteria.ReportType),
		Description: rr.getReportDescription(criteria.ReportType),
		PeriodStart: criteria.PeriodStart,
		PeriodEnd:   criteria.PeriodEnd,
		GeneratedAt: time.Now(),
		GeneratedBy: generatedBy,
		Recipients:  criteria.Recipients,
		Metadata:    make(map[string]interface{}),
	}

	// Generate report content based on type
	content, err := rr.generateReportContent(ctx, criteria)
	if err != nil {
		report.Status = ReportError
		report.ErrorMessage = err.Error()
		rr.reports[reportID] = report
		return report, err
	}

	report.Content = content
	report.Status = ReportGenerated

	// Store metadata
	report.Metadata["generation_time_ms"] = time.Since(report.GeneratedAt).Milliseconds()
	report.Metadata["content_length"] = len(content)
	report.Metadata["format"] = criteria.Format

	rr.reports[reportID] = report

	// Auto-submit if configured
	if criteria.AutoSubmit && rr.autoSubmit {
		err = rr.SubmitReport(ctx, reportID, generatedBy)
		if err != nil {
			report.ErrorMessage = fmt.Sprintf("Auto-submission failed: %v", err)
		}
	}

	return report, nil
}

// generateReportContent generates the actual report content
func (rr *RegulatoryReporter) generateReportContent(ctx context.Context, criteria ReportingCriteria) (string, error) {
	switch criteria.ReportType {
	case SuspiciousActivityReport:
		return rr.generateSARContent(ctx, criteria)
	case LargeTransactionReport:
		return rr.generateLTRContent(ctx, criteria)
	case CurrencyTransactionReport:
		return rr.generateCTRContent(ctx, criteria)
	case AMLComplianceReport:
		return rr.generateAMLComplianceContent(ctx, criteria)
	case KYCComplianceReport:
		return rr.generateKYCComplianceContent(ctx, criteria)
	case RiskAssessmentReport:
		return rr.generateRiskAssessmentContent(ctx, criteria)
	case PositionReport:
		return rr.generatePositionReportContent(ctx, criteria)
	case ExposureReport:
		return rr.generateExposureReportContent(ctx, criteria)
	default:
		return "", fmt.Errorf("unsupported report type: %s", criteria.ReportType)
	}
}

// generateSARContent generates Suspicious Activity Report content
func (rr *RegulatoryReporter) generateSARContent(ctx context.Context, criteria ReportingCriteria) (string, error) {
	alerts, err := rr.complianceEngine.GetActiveAlerts(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get compliance alerts: %w", err)
	}

	// Filter alerts by period and criteria
	relevantAlerts := make([]ComplianceAlert, 0)
	for _, alert := range alerts {
		if alert.CreatedAt.After(criteria.PeriodStart) && alert.CreatedAt.Before(criteria.PeriodEnd) {
			if alert.Severity == "high" || alert.Severity == "critical" {
				relevantAlerts = append(relevantAlerts, alert)
			}
		}
	}

	// Generate SAR data for each alert
	sarData := make([]SARData, 0)
	for _, alert := range relevantAlerts {
		sar := SARData{
			AlertID:            alert.ID,
			UserID:             alert.UserID,
			SuspiciousActivity: alert.Description,
			RiskScore:          alert.RiskScore,
			Patterns:           alert.DetectedPatterns,
			PeriodStart:        criteria.PeriodStart,
			PeriodEnd:          criteria.PeriodEnd,
			Narrative:          rr.generateSARNarrative(alert),
		}

		// Calculate total amount and transaction count from patterns
		totalAmount := decimal.Zero
		transactionCount := 0
		for _, pattern := range alert.DetectedPatterns {
			for _, tx := range pattern.Transactions {
				totalAmount = totalAmount.Add(tx.Amount)
				transactionCount++
			}
		}

		sar.TotalAmount = totalAmount
		sar.TransactionCount = transactionCount
		sarData = append(sarData, sar)
	}

	// Format as JSON or other requested format
	if criteria.Format == "json" {
		content, err := json.MarshalIndent(sarData, "", "  ")
		if err != nil {
			return "", fmt.Errorf("failed to marshal SAR data: %w", err)
		}
		return string(content), nil
	}

	// Generate formatted report
	return rr.formatSARReport(sarData, criteria), nil
}

// generateLTRContent generates Large Transaction Report content
func (rr *RegulatoryReporter) generateLTRContent(ctx context.Context, criteria ReportingCriteria) (string, error) {
	// In production, this would query transaction database
	threshold := decimal.NewFromFloat(10000) // $10,000 threshold

	// Mock large transactions for demo
	largeTransactions := []CTRData{
		{
			UserID:          "user_123",
			TransactionID:   "tx_001",
			Amount:          decimal.NewFromFloat(15000),
			Currency:        "USD",
			TransactionType: "withdrawal",
			TransactionDate: time.Now().Add(-2 * time.Hour),
			Source:          "exchange_wallet",
			Destination:     "external_bank",
			Purpose:         "withdrawal",
		},
		{
			UserID:          "user_456",
			TransactionID:   "tx_002",
			Amount:          decimal.NewFromFloat(25000),
			Currency:        "USD",
			TransactionType: "deposit",
			TransactionDate: time.Now().Add(-5 * time.Hour),
			Source:          "external_bank",
			Destination:     "exchange_wallet",
			Purpose:         "trading",
		},
	}

	// Filter by threshold and period
	filteredTransactions := make([]CTRData, 0)
	for _, tx := range largeTransactions {
		if tx.Amount.GreaterThan(threshold) &&
			tx.TransactionDate.After(criteria.PeriodStart) &&
			tx.TransactionDate.Before(criteria.PeriodEnd) {
			filteredTransactions = append(filteredTransactions, tx)
		}
	}

	if criteria.Format == "json" {
		content, err := json.MarshalIndent(filteredTransactions, "", "  ")
		if err != nil {
			return "", fmt.Errorf("failed to marshal LTR data: %w", err)
		}
		return string(content), nil
	}

	return rr.formatLTRReport(filteredTransactions, criteria), nil
}

// generateCTRContent generates Currency Transaction Report content
func (rr *RegulatoryReporter) generateCTRContent(ctx context.Context, criteria ReportingCriteria) (string, error) {
	// Similar to LTR but with different criteria and format
	return rr.generateLTRContent(ctx, criteria)
}

// generateAMLComplianceContent generates AML compliance report
func (rr *RegulatoryReporter) generateAMLComplianceContent(ctx context.Context, criteria ReportingCriteria) (string, error) {
	perfMetrics := rr.complianceEngine.GetPerformanceMetrics()

	complianceData := map[string]interface{}{
		"reporting_period": map[string]interface{}{
			"start": criteria.PeriodStart,
			"end":   criteria.PeriodEnd,
		},
		"compliance_metrics":       perfMetrics,
		"aml_checks_performed":     perfMetrics["total_checks"],
		"alerts_generated":         perfMetrics["total_alerts"],
		"alert_rate":               perfMetrics["alert_rate"],
		"average_check_time":       perfMetrics["average_check_time"],
		"false_positive_rate":      0.05, // Would be calculated from resolved alerts
		"regulatory_reports_filed": len(rr.reports),
		"compliance_status":        "compliant",
		"last_audit_date":          time.Now().AddDate(0, -3, 0), // 3 months ago
		"next_audit_due":           time.Now().AddDate(0, 9, 0),  // 9 months from now
	}

	content, err := json.MarshalIndent(complianceData, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal AML compliance data: %w", err)
	}

	return string(content), nil
}

// generateKYCComplianceContent generates KYC compliance report
func (rr *RegulatoryReporter) generateKYCComplianceContent(ctx context.Context, criteria ReportingCriteria) (string, error) {
	kycData := map[string]interface{}{
		"reporting_period": map[string]interface{}{
			"start": criteria.PeriodStart,
			"end":   criteria.PeriodEnd,
		},
		"kyc_metrics": map[string]interface{}{
			"total_users":                     10000,
			"verified_users":                  9500,
			"pending_verification":            400,
			"rejected_verifications":          100,
			"verification_rate":               0.95,
			"average_verification_time_hours": 24,
		},
		"document_verification": map[string]interface{}{
			"id_documents_processed":      9500,
			"address_documents_processed": 9500,
			"selfie_verifications":        9500,
			"manual_reviews_required":     150,
		},
		"risk_scoring": map[string]interface{}{
			"low_risk_users":               8000,
			"medium_risk_users":            1400,
			"high_risk_users":              100,
			"enhanced_due_diligence_cases": 50,
		},
		"compliance_status": "compliant",
	}

	content, err := json.MarshalIndent(kycData, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal KYC compliance data: %w", err)
	}

	return string(content), nil
}

// generateRiskAssessmentContent generates risk assessment report
func (rr *RegulatoryReporter) generateRiskAssessmentContent(ctx context.Context, criteria ReportingCriteria) (string, error) {
	// Get aggregate risk metrics
	riskData := map[string]interface{}{
		"reporting_period": map[string]interface{}{
			"start": criteria.PeriodStart,
			"end":   criteria.PeriodEnd,
		},
		"market_risk": map[string]interface{}{
			"total_exposure_usd": 50000000,
			"value_at_risk_95":   2500000,
			"expected_shortfall": 3000000,
			"max_drawdown":       1500000,
			"volatility_index":   0.25,
		},
		"credit_risk": map[string]interface{}{
			"counterparty_exposure": 5000000,
			"default_probability":   0.001,
			"expected_loss":         5000,
		},
		"operational_risk": map[string]interface{}{
			"system_downtime_minutes": 15,
			"failed_transactions":     25,
			"security_incidents":      0,
			"process_failures":        2,
		},
		"liquidity_risk": map[string]interface{}{
			"liquidity_coverage_ratio": 1.5,
			"funding_ratio":            0.8,
			"cash_reserves_usd":        10000000,
		},
		"concentration_risk": map[string]interface{}{
			"top_10_users_exposure_pct": 35,
			"single_market_max_pct":     60,
			"geographic_concentration": map[string]float64{
				"US":    40,
				"EU":    30,
				"ASIA":  20,
				"OTHER": 10,
			},
		},
	}

	content, err := json.MarshalIndent(riskData, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal risk assessment data: %w", err)
	}

	return string(content), nil
}

// generatePositionReportContent generates position report
func (rr *RegulatoryReporter) generatePositionReportContent(ctx context.Context, criteria ReportingCriteria) (string, error) {
	// Mock position data - in production, this would aggregate from position manager
	positionData := map[string]interface{}{
		"reporting_period": map[string]interface{}{
			"start": criteria.PeriodStart,
			"end":   criteria.PeriodEnd,
		},
		"aggregate_positions": map[string]interface{}{
			"BTCUSDT": map[string]interface{}{
				"long_positions":     1500.5,
				"short_positions":    800.2,
				"net_position":       700.3,
				"notional_value_usd": 28000000,
				"average_price":      40000,
			},
			"ETHUSDT": map[string]interface{}{
				"long_positions":     5000.8,
				"short_positions":    3200.1,
				"net_position":       1800.7,
				"notional_value_usd": 7200000,
				"average_price":      4000,
			},
		},
		"user_positions": map[string]interface{}{
			"total_users_with_positions": 1500,
			"largest_position_usd":       5000000,
			"average_position_size_usd":  33333,
			"positions_over_limit":       5,
		},
		"risk_metrics": map[string]interface{}{
			"total_notional_exposure": 35200000,
			"gross_exposure":          40000000,
			"net_exposure":            8000000,
			"leverage_ratio":          2.5,
		},
	}

	content, err := json.MarshalIndent(positionData, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal position data: %w", err)
	}

	return string(content), nil
}

// generateExposureReportContent generates exposure report
func (rr *RegulatoryReporter) generateExposureReportContent(ctx context.Context, criteria ReportingCriteria) (string, error) {
	// Similar to position report but focused on exposure analysis
	return rr.generatePositionReportContent(ctx, criteria)
}

// Helper methods for report formatting

func (rr *RegulatoryReporter) getReportTitle(reportType ReportType) string {
	switch reportType {
	case SuspiciousActivityReport:
		return "Suspicious Activity Report (SAR)"
	case LargeTransactionReport:
		return "Large Transaction Report (LTR)"
	case CurrencyTransactionReport:
		return "Currency Transaction Report (CTR)"
	case AMLComplianceReport:
		return "Anti-Money Laundering Compliance Report"
	case KYCComplianceReport:
		return "Know Your Customer Compliance Report"
	case RiskAssessmentReport:
		return "Risk Assessment Report"
	case PositionReport:
		return "Position Report"
	case ExposureReport:
		return "Exposure Report"
	default:
		return "Regulatory Report"
	}
}

func (rr *RegulatoryReporter) getReportDescription(reportType ReportType) string {
	switch reportType {
	case SuspiciousActivityReport:
		return "Report of suspicious activities detected by compliance monitoring systems"
	case LargeTransactionReport:
		return "Report of transactions exceeding regulatory thresholds"
	case CurrencyTransactionReport:
		return "Currency transaction report for regulatory compliance"
	case AMLComplianceReport:
		return "Comprehensive anti-money laundering compliance assessment"
	case KYCComplianceReport:
		return "Know your customer compliance and verification metrics"
	case RiskAssessmentReport:
		return "Market, credit, operational, and liquidity risk assessment"
	case PositionReport:
		return "Summary of trading positions and exposures"
	case ExposureReport:
		return "Detailed exposure analysis by market and counterparty"
	default:
		return "Regulatory compliance report"
	}
}

func (rr *RegulatoryReporter) generateSARNarrative(alert ComplianceAlert) string {
	var narrative strings.Builder

	narrative.WriteString(fmt.Sprintf("Suspicious activity detected for user %s. ", alert.UserID))
	narrative.WriteString(fmt.Sprintf("Alert generated: %s. ", alert.Description))
	narrative.WriteString(fmt.Sprintf("Risk score: %.2f. ", alert.RiskScore.InexactFloat64()))

	if len(alert.DetectedPatterns) > 0 {
		narrative.WriteString("Detected patterns include: ")
		for i, pattern := range alert.DetectedPatterns {
			if i > 0 {
				narrative.WriteString(", ")
			}
			narrative.WriteString(pattern.PatternType)
		}
		narrative.WriteString(". ")
	}

	narrative.WriteString("Requires investigation and potential filing with regulatory authorities.")

	return narrative.String()
}

func (rr *RegulatoryReporter) formatSARReport(sarData []SARData, criteria ReportingCriteria) string {
	var report strings.Builder

	report.WriteString("SUSPICIOUS ACTIVITY REPORT\n")
	report.WriteString("==========================\n\n")
	report.WriteString(fmt.Sprintf("Reporting Period: %s to %s\n",
		criteria.PeriodStart.Format("2006-01-02"),
		criteria.PeriodEnd.Format("2006-01-02")))
	report.WriteString(fmt.Sprintf("Generated: %s\n\n", time.Now().Format("2006-01-02 15:04:05")))

	report.WriteString(fmt.Sprintf("Total Suspicious Activities: %d\n\n", len(sarData)))

	for i, sar := range sarData {
		report.WriteString(fmt.Sprintf("%d. Alert ID: %s\n", i+1, sar.AlertID))
		report.WriteString(fmt.Sprintf("   User ID: %s\n", sar.UserID))
		report.WriteString(fmt.Sprintf("   Activity: %s\n", sar.SuspiciousActivity))
		report.WriteString(fmt.Sprintf("   Total Amount: $%s\n", sar.TotalAmount.String()))
		report.WriteString(fmt.Sprintf("   Transaction Count: %d\n", sar.TransactionCount))
		report.WriteString(fmt.Sprintf("   Risk Score: %.2f\n", sar.RiskScore.InexactFloat64()))
		report.WriteString(fmt.Sprintf("   Narrative: %s\n\n", sar.Narrative))
	}

	return report.String()
}

func (rr *RegulatoryReporter) formatLTRReport(transactions []CTRData, criteria ReportingCriteria) string {
	var report strings.Builder

	report.WriteString("LARGE TRANSACTION REPORT\n")
	report.WriteString("========================\n\n")
	report.WriteString(fmt.Sprintf("Reporting Period: %s to %s\n",
		criteria.PeriodStart.Format("2006-01-02"),
		criteria.PeriodEnd.Format("2006-01-02")))
	report.WriteString(fmt.Sprintf("Generated: %s\n\n", time.Now().Format("2006-01-02 15:04:05")))

	report.WriteString(fmt.Sprintf("Total Large Transactions: %d\n\n", len(transactions)))

	for i, tx := range transactions {
		report.WriteString(fmt.Sprintf("%d. Transaction ID: %s\n", i+1, tx.TransactionID))
		report.WriteString(fmt.Sprintf("   User ID: %s\n", tx.UserID))
		report.WriteString(fmt.Sprintf("   Amount: %s %s\n", tx.Amount.String(), tx.Currency))
		report.WriteString(fmt.Sprintf("   Type: %s\n", tx.TransactionType))
		report.WriteString(fmt.Sprintf("   Date: %s\n", tx.TransactionDate.Format("2006-01-02 15:04:05")))
		report.WriteString(fmt.Sprintf("   From: %s\n", tx.Source))
		report.WriteString(fmt.Sprintf("   To: %s\n", tx.Destination))
		report.WriteString(fmt.Sprintf("   Purpose: %s\n\n", tx.Purpose))
	}

	return report.String()
}

// SubmitReport submits a generated report to regulatory authorities
func (rr *RegulatoryReporter) SubmitReport(ctx context.Context, reportID, submittedBy string) error {
	report, exists := rr.reports[reportID]
	if !exists {
		return fmt.Errorf("report not found: %s", reportID)
	}

	if report.Status != ReportGenerated {
		return fmt.Errorf("report not ready for submission: %s", report.Status)
	}

	// In production, this would submit to actual regulatory APIs
	// For now, we'll just mark as submitted
	now := time.Now()
	report.Status = ReportSubmitted
	report.SubmittedAt = &now
	report.SubmittedBy = submittedBy

	return nil
}

// GetReport retrieves a generated report
func (rr *RegulatoryReporter) GetReport(reportID string) (*RegulatoryReport, error) {
	report, exists := rr.reports[reportID]
	if !exists {
		return nil, fmt.Errorf("report not found: %s", reportID)
	}

	return report, nil
}

// ListReports lists all generated reports with optional filtering
func (rr *RegulatoryReporter) ListReports(reportType ReportType, status ReportStatus, limit int) []*RegulatoryReport {
	reports := make([]*RegulatoryReport, 0)
	count := 0

	for _, report := range rr.reports {
		if count >= limit && limit > 0 {
			break
		}

		if reportType != "" && report.Type != reportType {
			continue
		}

		if status != "" && report.Status != status {
			continue
		}

		reports = append(reports, report)
		count++
	}

	return reports
}
