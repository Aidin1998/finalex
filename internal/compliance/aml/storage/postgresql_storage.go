package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/compliance/aml/models"
	"github.com/Aidin1998/pincex_unified/internal/compliance/aml/monitoring"
)

// PostgreSQLStorage implements AML data storage using PostgreSQL
type PostgreSQLStorage struct {
	db *sql.DB
}

// NewPostgreSQLStorage creates a new PostgreSQL storage instance
func NewPostgreSQLStorage(db *sql.DB) *PostgreSQLStorage {
	storage := &PostgreSQLStorage{db: db}
	if err := storage.initializeSchema(); err != nil {
		log.Printf("Failed to initialize AML schema: %v", err)
	}
	return storage
}

// initializeSchema creates the necessary database tables
func (s *PostgreSQLStorage) initializeSchema() error {
	schemas := []string{
		// AML Users table
		`CREATE TABLE IF NOT EXISTS aml_users (
			id SERIAL PRIMARY KEY,
			user_id VARCHAR(255) UNIQUE NOT NULL,
			risk_score DECIMAL(5,2) DEFAULT 0,
			kyc_status VARCHAR(50) DEFAULT 'PENDING',
			kyc_level INTEGER DEFAULT 0,
			pep_status BOOLEAN DEFAULT FALSE,
			sanctions_status BOOLEAN DEFAULT FALSE,
			enhanced_due_diligence BOOLEAN DEFAULT FALSE,
			country_code VARCHAR(3),
			registration_date TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			last_risk_assessment TIMESTAMP,
			profile_data JSONB,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,

		// Transactions table
		`CREATE TABLE IF NOT EXISTS aml_transactions (
			id SERIAL PRIMARY KEY,
			transaction_id VARCHAR(255) UNIQUE NOT NULL,
			user_id VARCHAR(255) NOT NULL,
			from_address VARCHAR(255),
			to_address VARCHAR(255),
			asset VARCHAR(20),
			amount DECIMAL(20,8),
			fee DECIMAL(20,8),
			transaction_type VARCHAR(50),
			risk_score DECIMAL(5,2) DEFAULT 0,
			suspicious BOOLEAN DEFAULT FALSE,
			timestamp TIMESTAMP NOT NULL,
			metadata JSONB,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,

		// Suspicious Activities table
		`CREATE TABLE IF NOT EXISTS aml_suspicious_activities (
			id SERIAL PRIMARY KEY,
			user_id VARCHAR(255) NOT NULL,
			transaction_id VARCHAR(255),
			activity_type VARCHAR(100) NOT NULL,
			description TEXT,
			risk_score DECIMAL(5,2),
			status VARCHAR(50) DEFAULT 'DETECTED',
			evidence JSONB,
			detected_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			reviewed_at TIMESTAMP,
			reviewed_by VARCHAR(255),
			resolution TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,

		// Compliance Actions table
		`CREATE TABLE IF NOT EXISTS aml_compliance_actions (
			id SERIAL PRIMARY KEY,
			user_id VARCHAR(255) NOT NULL,
			action_type VARCHAR(100) NOT NULL,
			reason TEXT,
			automated BOOLEAN DEFAULT TRUE,
			taken_by VARCHAR(255),
			details JSONB,
			effective_date TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			expiry_date TIMESTAMP,
			reversed BOOLEAN DEFAULT FALSE,
			reversed_at TIMESTAMP,
			reversed_by VARCHAR(255),
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,

		// Investigation Cases table
		`CREATE TABLE IF NOT EXISTS aml_investigation_cases (
			id SERIAL PRIMARY KEY,
			case_number VARCHAR(100) UNIQUE NOT NULL,
			user_id VARCHAR(255) NOT NULL,
			case_type VARCHAR(100) NOT NULL,
			priority VARCHAR(20) DEFAULT 'MEDIUM',
			status VARCHAR(50) DEFAULT 'OPEN',
			assigned_to VARCHAR(255),
			title VARCHAR(255),
			description TEXT,
			evidence JSONB,
			findings TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			closed_at TIMESTAMP,
			closed_by VARCHAR(255)
		)`,

		// Regulatory Reports table
		`CREATE TABLE IF NOT EXISTS aml_regulatory_reports (
			id SERIAL PRIMARY KEY,
			report_type VARCHAR(100) NOT NULL,
			report_id VARCHAR(255) UNIQUE NOT NULL,
			filing_date TIMESTAMP NOT NULL,
			reporting_period_start TIMESTAMP NOT NULL,
			reporting_period_end TIMESTAMP NOT NULL,
			status VARCHAR(50) DEFAULT 'DRAFT',
			submitted_to VARCHAR(255),
			submission_reference VARCHAR(255),
			data JSONB,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,

		// AML Rules table
		`CREATE TABLE IF NOT EXISTS aml_rules (
			id SERIAL PRIMARY KEY,
			rule_id VARCHAR(255) UNIQUE NOT NULL,
			name VARCHAR(255) NOT NULL,
			description TEXT,
			rule_type VARCHAR(100) NOT NULL,
			conditions JSONB NOT NULL,
			actions JSONB NOT NULL,
			priority INTEGER DEFAULT 100,
			enabled BOOLEAN DEFAULT TRUE,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			created_by VARCHAR(255),
			last_triggered TIMESTAMP,
			trigger_count INTEGER DEFAULT 0
		)`,

		// Risk Assessments table
		`CREATE TABLE IF NOT EXISTS aml_risk_assessments (
			id SERIAL PRIMARY KEY,
			user_id VARCHAR(255) NOT NULL,
			assessment_type VARCHAR(100) NOT NULL,
			risk_score DECIMAL(5,2) NOT NULL,
			risk_factors JSONB,
			assessment_date TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			valid_until TIMESTAMP,
			assessor VARCHAR(255),
			notes TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,

		// Monitoring Metrics table
		`CREATE TABLE IF NOT EXISTS aml_monitoring_metrics (
			id SERIAL PRIMARY KEY,
			metric_type VARCHAR(100) NOT NULL,
			metrics_data JSONB NOT NULL,
			timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,

		// Real-time Alerts table
		`CREATE TABLE IF NOT EXISTS aml_realtime_alerts (
			id SERIAL PRIMARY KEY,
			alert_id VARCHAR(255) UNIQUE NOT NULL,
			alert_level INTEGER NOT NULL,
			alert_type VARCHAR(100) NOT NULL,
			message TEXT NOT NULL,
			user_id VARCHAR(255),
			metadata JSONB,
			timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			resolved BOOLEAN DEFAULT FALSE,
			resolved_at TIMESTAMP,
			resolved_by VARCHAR(255)
		)`,

		// API Requests table
		`CREATE TABLE IF NOT EXISTS aml_api_requests (
			id SERIAL PRIMARY KEY,
			request_id VARCHAR(255) UNIQUE NOT NULL,
			user_id VARCHAR(255) NOT NULL,
			endpoint VARCHAR(255) NOT NULL,
			method VARCHAR(10) NOT NULL,
			user_agent TEXT,
			ip_address INET,
			country VARCHAR(3),
			payload_size BIGINT,
			headers JSONB,
			parameters JSONB,
			timestamp TIMESTAMP NOT NULL,
			response_code INTEGER,
			response_time_ms INTEGER,
			blocked BOOLEAN DEFAULT FALSE,
			block_reason TEXT
		)`,

		// API Violations table
		`CREATE TABLE IF NOT EXISTS aml_api_violations (
			id SERIAL PRIMARY KEY,
			violation_id VARCHAR(255) UNIQUE NOT NULL,
			user_id VARCHAR(255) NOT NULL,
			violation_type VARCHAR(100) NOT NULL,
			severity VARCHAR(20) NOT NULL,
			description TEXT,
			request_id VARCHAR(255),
			evidence JSONB,
			timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			action_taken VARCHAR(100),
			resolved BOOLEAN DEFAULT FALSE
		)`,

		// API Security Events table
		`CREATE TABLE IF NOT EXISTS aml_api_security_events (
			id SERIAL PRIMARY KEY,
			event_id VARCHAR(255) UNIQUE NOT NULL,
			event_type VARCHAR(100) NOT NULL,
			severity VARCHAR(20) NOT NULL,
			description TEXT,
			user_id VARCHAR(255),
			ip_address INET,
			pattern VARCHAR(255),
			evidence JSONB,
			timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			mitigated BOOLEAN DEFAULT FALSE,
			action_taken VARCHAR(100)
		)`,

		// User Statistics table
		`CREATE TABLE IF NOT EXISTS aml_user_api_stats (
			id SERIAL PRIMARY KEY,
			user_id VARCHAR(255) UNIQUE NOT NULL,
			request_count BIGINT DEFAULT 0,
			requests_per_minute DECIMAL(10,2) DEFAULT 0,
			endpoint_usage JSONB,
			error_rate DECIMAL(5,4) DEFAULT 0,
			average_response_ms INTEGER DEFAULT 0,
			last_activity TIMESTAMP,
			violation_count INTEGER DEFAULT 0,
			is_blocked BOOLEAN DEFAULT FALSE,
			block_reason TEXT,
			risk_score DECIMAL(5,2) DEFAULT 0,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
	}

	// Create indexes
	indexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_aml_users_user_id ON aml_users(user_id)",
		"CREATE INDEX IF NOT EXISTS idx_aml_users_risk_score ON aml_users(risk_score)",
		"CREATE INDEX IF NOT EXISTS idx_aml_users_kyc_status ON aml_users(kyc_status)",
		"CREATE INDEX IF NOT EXISTS idx_aml_transactions_user_id ON aml_transactions(user_id)",
		"CREATE INDEX IF NOT EXISTS idx_aml_transactions_timestamp ON aml_transactions(timestamp)",
		"CREATE INDEX IF NOT EXISTS idx_aml_transactions_risk_score ON aml_transactions(risk_score)",
		"CREATE INDEX IF NOT EXISTS idx_aml_transactions_suspicious ON aml_transactions(suspicious)",
		"CREATE INDEX IF NOT EXISTS idx_aml_suspicious_activities_user_id ON aml_suspicious_activities(user_id)",
		"CREATE INDEX IF NOT EXISTS idx_aml_suspicious_activities_status ON aml_suspicious_activities(status)",
		"CREATE INDEX IF NOT EXISTS idx_aml_suspicious_activities_detected_at ON aml_suspicious_activities(detected_at)",
		"CREATE INDEX IF NOT EXISTS idx_aml_compliance_actions_user_id ON aml_compliance_actions(user_id)",
		"CREATE INDEX IF NOT EXISTS idx_aml_compliance_actions_action_type ON aml_compliance_actions(action_type)",
		"CREATE INDEX IF NOT EXISTS idx_aml_investigation_cases_user_id ON aml_investigation_cases(user_id)",
		"CREATE INDEX IF NOT EXISTS idx_aml_investigation_cases_status ON aml_investigation_cases(status)",
		"CREATE INDEX IF NOT EXISTS idx_aml_investigation_cases_assigned_to ON aml_investigation_cases(assigned_to)",
		"CREATE INDEX IF NOT EXISTS idx_aml_regulatory_reports_report_type ON aml_regulatory_reports(report_type)",
		"CREATE INDEX IF NOT EXISTS idx_aml_regulatory_reports_filing_date ON aml_regulatory_reports(filing_date)",
		"CREATE INDEX IF NOT EXISTS idx_aml_rules_enabled ON aml_rules(enabled)",
		"CREATE INDEX IF NOT EXISTS idx_aml_rules_rule_type ON aml_rules(rule_type)",
		"CREATE INDEX IF NOT EXISTS idx_aml_risk_assessments_user_id ON aml_risk_assessments(user_id)",
		"CREATE INDEX IF NOT EXISTS idx_aml_risk_assessments_assessment_date ON aml_risk_assessments(assessment_date)",
		"CREATE INDEX IF NOT EXISTS idx_aml_monitoring_metrics_timestamp ON aml_monitoring_metrics(timestamp)",
		"CREATE INDEX IF NOT EXISTS idx_aml_realtime_alerts_timestamp ON aml_realtime_alerts(timestamp)",
		"CREATE INDEX IF NOT EXISTS idx_aml_realtime_alerts_user_id ON aml_realtime_alerts(user_id)",
		"CREATE INDEX IF NOT EXISTS idx_aml_api_requests_user_id ON aml_api_requests(user_id)",
		"CREATE INDEX IF NOT EXISTS idx_aml_api_requests_timestamp ON aml_api_requests(timestamp)",
		"CREATE INDEX IF NOT EXISTS idx_aml_api_requests_blocked ON aml_api_requests(blocked)",
		"CREATE INDEX IF NOT EXISTS idx_aml_api_violations_user_id ON aml_api_violations(user_id)",
		"CREATE INDEX IF NOT EXISTS idx_aml_api_violations_timestamp ON aml_api_violations(timestamp)",
		"CREATE INDEX IF NOT EXISTS idx_aml_api_security_events_timestamp ON aml_api_security_events(timestamp)",
		"CREATE INDEX IF NOT EXISTS idx_aml_user_api_stats_user_id ON aml_user_api_stats(user_id)",
		"CREATE INDEX IF NOT EXISTS idx_aml_user_api_stats_risk_score ON aml_user_api_stats(risk_score)",
	}

	// Execute schema creation
	for _, schema := range schemas {
		if _, err := s.db.Exec(schema); err != nil {
			return fmt.Errorf("failed to create table: %v", err)
		}
	}

	// Execute index creation
	for _, index := range indexes {
		if _, err := s.db.Exec(index); err != nil {
			log.Printf("Failed to create index: %v", err)
		}
	}

	log.Println("AML database schema initialized successfully")
	return nil
}

// AMLUser methods
func (s *PostgreSQLStorage) CreateAMLUser(ctx context.Context, user *models.AMLUser) error {
	profileData, _ := json.Marshal(user.ProfileData)

	query := `
		INSERT INTO aml_users (user_id, risk_score, kyc_status, kyc_level, pep_status, 
			sanctions_status, enhanced_due_diligence, country_code, registration_date, 
			last_risk_assessment, profile_data)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id`

	err := s.db.QueryRowContext(ctx, query,
		user.UserID, user.RiskScore, user.KYCStatus, user.KYCLevel,
		user.PEPStatus, user.SanctionsStatus, user.EnhancedDueDiligence,
		user.CountryCode, user.RegistrationDate, user.LastRiskAssessment,
		profileData).Scan(&user.ID)

	return err
}

func (s *PostgreSQLStorage) GetAMLUser(ctx context.Context, userID string) (*models.AMLUser, error) {
	var user models.AMLUser
	var profileData []byte

	query := `
		SELECT id, user_id, risk_score, kyc_status, kyc_level, pep_status,
			sanctions_status, enhanced_due_diligence, country_code, registration_date,
			last_risk_assessment, profile_data, created_at, updated_at
		FROM aml_users WHERE user_id = $1`

	err := s.db.QueryRowContext(ctx, query, userID).Scan(
		&user.ID, &user.UserID, &user.RiskScore, &user.KYCStatus,
		&user.KYCLevel, &user.PEPStatus, &user.SanctionsStatus,
		&user.EnhancedDueDiligence, &user.CountryCode, &user.RegistrationDate,
		&user.LastRiskAssessment, &profileData, &user.CreatedAt, &user.UpdatedAt)

	if err != nil {
		return nil, err
	}

	if len(profileData) > 0 {
		json.Unmarshal(profileData, &user.ProfileData)
	}

	return &user, nil
}

func (s *PostgreSQLStorage) UpdateAMLUser(ctx context.Context, user *models.AMLUser) error {
	profileData, _ := json.Marshal(user.ProfileData)

	query := `
		UPDATE aml_users SET 
			risk_score = $2, kyc_status = $3, kyc_level = $4, pep_status = $5,
			sanctions_status = $6, enhanced_due_diligence = $7, country_code = $8,
			last_risk_assessment = $9, profile_data = $10, updated_at = CURRENT_TIMESTAMP
		WHERE user_id = $1`

	_, err := s.db.ExecContext(ctx, query,
		user.UserID, user.RiskScore, user.KYCStatus, user.KYCLevel,
		user.PEPStatus, user.SanctionsStatus, user.EnhancedDueDiligence,
		user.CountryCode, user.LastRiskAssessment, profileData)

	return err
}

// Transaction methods
func (s *PostgreSQLStorage) CreateTransaction(ctx context.Context, tx *models.Transaction) error {
	metadata, _ := json.Marshal(tx.Metadata)

	query := `
		INSERT INTO aml_transactions (transaction_id, user_id, from_address, to_address,
			asset, amount, fee, transaction_type, risk_score, suspicious, timestamp, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id`

	err := s.db.QueryRowContext(ctx, query,
		tx.TransactionID, tx.UserID, tx.FromAddress, tx.ToAddress,
		tx.Asset, tx.Amount, tx.Fee, tx.TransactionType, tx.RiskScore,
		tx.Suspicious, tx.Timestamp, metadata).Scan(&tx.ID)

	return err
}

func (s *PostgreSQLStorage) GetTransaction(ctx context.Context, transactionID string) (*models.Transaction, error) {
	var tx models.Transaction
	var metadata []byte

	query := `
		SELECT id, transaction_id, user_id, from_address, to_address, asset, amount,
			fee, transaction_type, risk_score, suspicious, timestamp, metadata, created_at
		FROM aml_transactions WHERE transaction_id = $1`

	err := s.db.QueryRowContext(ctx, query, transactionID).Scan(
		&tx.ID, &tx.TransactionID, &tx.UserID, &tx.FromAddress, &tx.ToAddress,
		&tx.Asset, &tx.Amount, &tx.Fee, &tx.TransactionType, &tx.RiskScore,
		&tx.Suspicious, &tx.Timestamp, &metadata, &tx.CreatedAt)

	if err != nil {
		return nil, err
	}

	if len(metadata) > 0 {
		json.Unmarshal(metadata, &tx.Metadata)
	}

	return &tx, nil
}

func (s *PostgreSQLStorage) GetUserTransactions(ctx context.Context, userID string, limit int) ([]*models.Transaction, error) {
	query := `
		SELECT id, transaction_id, user_id, from_address, to_address, asset, amount,
			fee, transaction_type, risk_score, suspicious, timestamp, metadata, created_at
		FROM aml_transactions 
		WHERE user_id = $1 
		ORDER BY timestamp DESC 
		LIMIT $2`

	rows, err := s.db.QueryContext(ctx, query, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var transactions []*models.Transaction
	for rows.Next() {
		var tx models.Transaction
		var metadata []byte

		err := rows.Scan(
			&tx.ID, &tx.TransactionID, &tx.UserID, &tx.FromAddress, &tx.ToAddress,
			&tx.Asset, &tx.Amount, &tx.Fee, &tx.TransactionType, &tx.RiskScore,
			&tx.Suspicious, &tx.Timestamp, &metadata, &tx.CreatedAt)

		if err != nil {
			return nil, err
		}

		if len(metadata) > 0 {
			json.Unmarshal(metadata, &tx.Metadata)
		}

		transactions = append(transactions, &tx)
	}

	return transactions, nil
}

// Suspicious Activity methods
func (s *PostgreSQLStorage) CreateSuspiciousActivity(ctx context.Context, activity *models.SuspiciousActivity) error {
	evidence, _ := json.Marshal(activity.Evidence)

	query := `
		INSERT INTO aml_suspicious_activities (user_id, transaction_id, activity_type,
			description, risk_score, status, evidence, detected_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id`

	err := s.db.QueryRowContext(ctx, query,
		activity.UserID, activity.TransactionID, activity.Type,
		activity.Description, activity.RiskScore, activity.Status,
		evidence, activity.DetectedAt).Scan(&activity.ID)

	return err
}

func (s *PostgreSQLStorage) GetSuspiciousActivity(ctx context.Context, id uint) (*models.SuspiciousActivity, error) {
	var activity models.SuspiciousActivity
	var evidence []byte

	query := `
		SELECT id, user_id, transaction_id, activity_type, description, risk_score,
			status, evidence, detected_at, reviewed_at, reviewed_by, resolution,
			created_at, updated_at
		FROM aml_suspicious_activities WHERE id = $1`

	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&activity.ID, &activity.UserID, &activity.TransactionID, &activity.Type,
		&activity.Description, &activity.RiskScore, &activity.Status, &evidence,
		&activity.DetectedAt, &activity.ReviewedAt, &activity.ReviewedBy,
		&activity.Resolution, &activity.CreatedAt, &activity.UpdatedAt)

	if err != nil {
		return nil, err
	}

	if len(evidence) > 0 {
		json.Unmarshal(evidence, &activity.Evidence)
	}

	return &activity, nil
}

func (s *PostgreSQLStorage) UpdateSuspiciousActivity(ctx context.Context, activity *models.SuspiciousActivity) error {
	evidence, _ := json.Marshal(activity.Evidence)

	query := `
		UPDATE aml_suspicious_activities SET
			status = $2, reviewed_at = $3, reviewed_by = $4, resolution = $5,
			evidence = $6, updated_at = CURRENT_TIMESTAMP
		WHERE id = $1`

	_, err := s.db.ExecContext(ctx, query,
		activity.ID, activity.Status, activity.ReviewedAt, activity.ReviewedBy,
		activity.Resolution, evidence)

	return err
}

// Compliance Action methods
func (s *PostgreSQLStorage) CreateComplianceAction(ctx context.Context, action *models.ComplianceAction) error {
	details, _ := json.Marshal(action.Details)

	query := `
		INSERT INTO aml_compliance_actions (user_id, action_type, reason, automated,
			taken_by, details, effective_date, expiry_date)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id`

	err := s.db.QueryRowContext(ctx, query,
		action.UserID, action.ActionType, action.Reason, action.Automated,
		action.TakenBy, details, action.EffectiveDate, action.ExpiryDate).Scan(&action.ID)

	return err
}

func (s *PostgreSQLStorage) GetUserComplianceActions(ctx context.Context, userID string) ([]*models.ComplianceAction, error) {
	query := `
		SELECT id, user_id, action_type, reason, automated, taken_by, details,
			effective_date, expiry_date, reversed, reversed_at, reversed_by, created_at
		FROM aml_compliance_actions 
		WHERE user_id = $1 
		ORDER BY created_at DESC`

	rows, err := s.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var actions []*models.ComplianceAction
	for rows.Next() {
		var action models.ComplianceAction
		var details []byte

		err := rows.Scan(
			&action.ID, &action.UserID, &action.ActionType, &action.Reason,
			&action.Automated, &action.TakenBy, &details, &action.EffectiveDate,
			&action.ExpiryDate, &action.Reversed, &action.ReversedAt,
			&action.ReversedBy, &action.CreatedAt)

		if err != nil {
			return nil, err
		}

		if len(details) > 0 {
			json.Unmarshal(details, &action.Details)
		}

		actions = append(actions, &action)
	}

	return actions, nil
}

// Investigation Case methods
func (s *PostgreSQLStorage) CreateInvestigationCase(ctx context.Context, case_ *models.InvestigationCase) error {
	evidence, _ := json.Marshal(case_.Evidence)

	query := `
		INSERT INTO aml_investigation_cases (case_number, user_id, case_type, priority,
			status, assigned_to, title, description, evidence)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id`

	err := s.db.QueryRowContext(ctx, query,
		case_.CaseNumber, case_.UserID, case_.CaseType, case_.Priority,
		case_.Status, case_.AssignedTo, case_.Title, case_.Description,
		evidence).Scan(&case_.ID)

	return err
}

func (s *PostgreSQLStorage) GetInvestigationCase(ctx context.Context, id uint) (*models.InvestigationCase, error) {
	var case_ models.InvestigationCase
	var evidence []byte

	query := `
		SELECT id, case_number, user_id, case_type, priority, status, assigned_to,
			title, description, evidence, findings, created_at, updated_at,
			closed_at, closed_by
		FROM aml_investigation_cases WHERE id = $1`

	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&case_.ID, &case_.CaseNumber, &case_.UserID, &case_.CaseType,
		&case_.Priority, &case_.Status, &case_.AssignedTo, &case_.Title,
		&case_.Description, &evidence, &case_.Findings, &case_.CreatedAt,
		&case_.UpdatedAt, &case_.ClosedAt, &case_.ClosedBy)

	if err != nil {
		return nil, err
	}

	if len(evidence) > 0 {
		json.Unmarshal(evidence, &case_.Evidence)
	}

	return &case_, nil
}

func (s *PostgreSQLStorage) UpdateInvestigationCase(ctx context.Context, case_ *models.InvestigationCase) error {
	evidence, _ := json.Marshal(case_.Evidence)

	query := `
		UPDATE aml_investigation_cases SET
			status = $2, assigned_to = $3, title = $4, description = $5,
			evidence = $6, findings = $7, updated_at = CURRENT_TIMESTAMP,
			closed_at = $8, closed_by = $9
		WHERE id = $1`

	_, err := s.db.ExecContext(ctx, query,
		case_.ID, case_.Status, case_.AssignedTo, case_.Title,
		case_.Description, evidence, case_.Findings, case_.ClosedAt,
		case_.ClosedBy)

	return err
}

// Regulatory Report methods
func (s *PostgreSQLStorage) CreateRegulatoryReport(ctx context.Context, report *models.RegulatoryReport) error {
	data, _ := json.Marshal(report.Data)

	query := `
		INSERT INTO aml_regulatory_reports (report_type, report_id, filing_date,
			reporting_period_start, reporting_period_end, status, submitted_to,
			submission_reference, data)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id`

	err := s.db.QueryRowContext(ctx, query,
		report.ReportType, report.ReportID, report.FilingDate,
		report.ReportingPeriodStart, report.ReportingPeriodEnd, report.Status,
		report.SubmittedTo, report.SubmissionReference, data).Scan(&report.ID)

	return err
}

func (s *PostgreSQLStorage) GetRegulatoryReport(ctx context.Context, reportID string) (*models.RegulatoryReport, error) {
	var report models.RegulatoryReport
	var data []byte

	query := `
		SELECT id, report_type, report_id, filing_date, reporting_period_start,
			reporting_period_end, status, submitted_to, submission_reference,
			data, created_at, updated_at
		FROM aml_regulatory_reports WHERE report_id = $1`

	err := s.db.QueryRowContext(ctx, query, reportID).Scan(
		&report.ID, &report.ReportType, &report.ReportID, &report.FilingDate,
		&report.ReportingPeriodStart, &report.ReportingPeriodEnd, &report.Status,
		&report.SubmittedTo, &report.SubmissionReference, &data,
		&report.CreatedAt, &report.UpdatedAt)

	if err != nil {
		return nil, err
	}

	if len(data) > 0 {
		json.Unmarshal(data, &report.Data)
	}

	return &report, nil
}

// Monitoring storage methods
func (s *PostgreSQLStorage) StoreMetrics(ctx context.Context, metrics *monitoring.MonitoringMetrics) error {
	data, _ := json.Marshal(metrics)

	query := `INSERT INTO aml_monitoring_metrics (metric_type, metrics_data) VALUES ($1, $2)`
	_, err := s.db.ExecContext(ctx, query, "realtime_monitoring", data)
	return err
}

func (s *PostgreSQLStorage) StoreAlert(ctx context.Context, alert *monitoring.RealtimeAlert) error {
	metadata, _ := json.Marshal(alert.Metadata)

	query := `
		INSERT INTO aml_realtime_alerts (alert_id, alert_level, alert_type, message,
			user_id, metadata, timestamp, resolved, resolved_at, resolved_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`

	_, err := s.db.ExecContext(ctx, query,
		alert.ID, alert.Level, alert.Type, alert.Message, alert.UserID,
		metadata, alert.Timestamp, alert.Resolved, alert.ResolvedAt,
		alert.ResolvedBy)

	return err
}

func (s *PostgreSQLStorage) GetHistoricalMetrics(ctx context.Context, from, to time.Time) ([]*monitoring.MonitoringMetrics, error) {
	query := `
		SELECT metrics_data FROM aml_monitoring_metrics 
		WHERE timestamp BETWEEN $1 AND $2 
		ORDER BY timestamp ASC`

	rows, err := s.db.QueryContext(ctx, query, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var metrics []*monitoring.MonitoringMetrics
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			continue
		}

		var metric monitoring.MonitoringMetrics
		if err := json.Unmarshal(data, &metric); err == nil {
			metrics = append(metrics, &metric)
		}
	}

	return metrics, nil
}

func (s *PostgreSQLStorage) GetAlertHistory(ctx context.Context, filter monitoring.AlertFilter, limit int) ([]*monitoring.RealtimeAlert, error) {
	query := `
		SELECT alert_id, alert_level, alert_type, message, user_id, metadata,
			timestamp, resolved, resolved_at, resolved_by
		FROM aml_realtime_alerts 
		WHERE alert_level >= $1
		ORDER BY timestamp DESC 
		LIMIT $2`

	rows, err := s.db.QueryContext(ctx, query, filter.MinLevel, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var alerts []*monitoring.RealtimeAlert
	for rows.Next() {
		var alert monitoring.RealtimeAlert
		var metadata []byte

		err := rows.Scan(
			&alert.ID, &alert.Level, &alert.Type, &alert.Message,
			&alert.UserID, &metadata, &alert.Timestamp, &alert.Resolved,
			&alert.ResolvedAt, &alert.ResolvedBy)

		if err != nil {
			continue
		}

		if len(metadata) > 0 {
			json.Unmarshal(metadata, &alert.Metadata)
		}

		alerts = append(alerts, &alert)
	}

	return alerts, nil
}

// API Monitoring storage methods
func (s *PostgreSQLStorage) StoreRequest(ctx context.Context, request *monitoring.APIRequest) error {
	headers, _ := json.Marshal(request.Headers)
	parameters, _ := json.Marshal(request.Parameters)

	query := `
		INSERT INTO aml_api_requests (request_id, user_id, endpoint, method,
			user_agent, ip_address, country, payload_size, headers, parameters,
			timestamp, response_code, response_time_ms, blocked, block_reason)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`

	responseTimeMs := int(request.ResponseTime.Milliseconds())

	_, err := s.db.ExecContext(ctx, query,
		request.ID, request.UserID, request.Endpoint, request.Method,
		request.UserAgent, request.IPAddress, request.Country, request.PayloadSize,
		headers, parameters, request.Timestamp, request.ResponseCode,
		responseTimeMs, request.Blocked, request.Reason)

	return err
}

func (s *PostgreSQLStorage) StoreViolation(ctx context.Context, violation *monitoring.APIViolation) error {
	evidence, _ := json.Marshal(violation.Evidence)

	query := `
		INSERT INTO aml_api_violations (violation_id, user_id, violation_type,
			severity, description, request_id, evidence, timestamp, action_taken)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

	var requestID *string
	if violation.Request != nil {
		requestID = &violation.Request.ID
	}

	_, err := s.db.ExecContext(ctx, query,
		violation.ID, violation.UserID, violation.Type, violation.Severity,
		violation.Description, requestID, evidence, violation.Timestamp,
		violation.Action)

	return err
}

func (s *PostgreSQLStorage) StoreSecurityEvent(ctx context.Context, event *monitoring.APISecurityEvent) error {
	evidence, _ := json.Marshal(event.Evidence)

	query := `
		INSERT INTO aml_api_security_events (event_id, event_type, severity,
			description, user_id, ip_address, pattern, evidence, timestamp,
			mitigated, action_taken)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`

	_, err := s.db.ExecContext(ctx, query,
		event.ID, event.Type, event.Severity, event.Description, event.UserID,
		event.IPAddress, event.Pattern, evidence, event.Timestamp,
		event.Mitigated, event.Action)

	return err
}

func (s *PostgreSQLStorage) GetUserStats(ctx context.Context, userID string) (*monitoring.UserAPIStats, error) {
	var stats monitoring.UserAPIStats
	var endpointUsage []byte

	query := `
		SELECT user_id, request_count, requests_per_minute, endpoint_usage,
			error_rate, average_response_ms, last_activity, violation_count,
			is_blocked, block_reason, risk_score, updated_at
		FROM aml_user_api_stats WHERE user_id = $1`

	err := s.db.QueryRowContext(ctx, query, userID).Scan(
		&stats.UserID, &stats.RequestCount, &stats.RequestsPerMinute, &endpointUsage,
		&stats.ErrorRate, &stats.AverageResponse, &stats.LastActivity,
		&stats.ViolationCount, &stats.IsBlocked, &stats.BlockReason,
		&stats.RiskScore)

	if err != nil {
		if err == sql.ErrNoRows {
			// Return empty stats for new user
			return &monitoring.UserAPIStats{
				UserID:        userID,
				EndpointUsage: make(map[string]int64),
			}, nil
		}
		return nil, err
	}

	if len(endpointUsage) > 0 {
		json.Unmarshal(endpointUsage, &stats.EndpointUsage)
	} else {
		stats.EndpointUsage = make(map[string]int64)
	}

	return &stats, nil
}

func (s *PostgreSQLStorage) UpdateUserStats(ctx context.Context, stats *monitoring.UserAPIStats) error {
	endpointUsage, _ := json.Marshal(stats.EndpointUsage)

	query := `
		INSERT INTO aml_user_api_stats (user_id, request_count, requests_per_minute,
			endpoint_usage, error_rate, average_response_ms, last_activity,
			violation_count, is_blocked, block_reason, risk_score, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, CURRENT_TIMESTAMP)
		ON CONFLICT (user_id) DO UPDATE SET
			request_count = EXCLUDED.request_count,
			requests_per_minute = EXCLUDED.requests_per_minute,
			endpoint_usage = EXCLUDED.endpoint_usage,
			error_rate = EXCLUDED.error_rate,
			average_response_ms = EXCLUDED.average_response_ms,
			last_activity = EXCLUDED.last_activity,
			violation_count = EXCLUDED.violation_count,
			is_blocked = EXCLUDED.is_blocked,
			block_reason = EXCLUDED.block_reason,
			risk_score = EXCLUDED.risk_score,
			updated_at = CURRENT_TIMESTAMP`

	avgResponseMs := int(stats.AverageResponse.Milliseconds())

	_, err := s.db.ExecContext(ctx, query,
		stats.UserID, stats.RequestCount, stats.RequestsPerMinute, endpointUsage,
		stats.ErrorRate, avgResponseMs, stats.LastActivity, stats.ViolationCount,
		stats.IsBlocked, stats.BlockReason, stats.RiskScore)

	return err
}

func (s *PostgreSQLStorage) GetViolationHistory(ctx context.Context, userID string, limit int) ([]monitoring.APIViolation, error) {
	query := `
		SELECT violation_id, user_id, violation_type, severity, description,
			evidence, timestamp, action_taken, resolved
		FROM aml_api_violations 
		WHERE user_id = $1 
		ORDER BY timestamp DESC 
		LIMIT $2`

	rows, err := s.db.QueryContext(ctx, query, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var violations []monitoring.APIViolation
	for rows.Next() {
		var violation monitoring.APIViolation
		var evidence []byte

		err := rows.Scan(
			&violation.ID, &violation.UserID, &violation.Type, &violation.Severity,
			&violation.Description, &evidence, &violation.Timestamp,
			&violation.Action, &violation.Resolved)

		if err != nil {
			continue
		}

		if len(evidence) > 0 {
			json.Unmarshal(evidence, &violation.Evidence)
		}

		violations = append(violations, violation)
	}

	return violations, nil
}

// Helper methods
func (s *PostgreSQLStorage) Close() error {
	return s.db.Close()
}

func (s *PostgreSQLStorage) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}
