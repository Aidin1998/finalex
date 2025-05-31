package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/compliance/aml"
	"github.com/google/uuid"
)

// PostgreSQLStorage implements AML storage using PostgreSQL
type PostgreSQLStorage struct {
	db *sql.DB
}

// NewPostgreSQLStorage creates a new PostgreSQL storage instance
func NewPostgreSQLStorage(db *sql.DB) *PostgreSQLStorage {
	return &PostgreSQLStorage{
		db: db,
	}
}

// StoreUser stores or updates an AML user profile
func (p *PostgreSQLStorage) StoreUser(ctx context.Context, user *aml.AMLUser) error {
	riskFactorsJSON, err := json.Marshal(user.RiskFactors)
	if err != nil {
		return fmt.Errorf("failed to marshal risk factors: %v", err)
	}

	query := `
		INSERT INTO aml_users (
			id, user_id, risk_level, risk_score, kyc_status, 
			is_blacklisted, is_whitelisted, last_risk_update, 
			country_code, is_high_risk_country, pep_status, 
			sanction_status, customer_type, business_type, 
			risk_factors, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17
		) ON CONFLICT (user_id) DO UPDATE SET
			risk_level = EXCLUDED.risk_level,
			risk_score = EXCLUDED.risk_score,
			kyc_status = EXCLUDED.kyc_status,
			is_blacklisted = EXCLUDED.is_blacklisted,
			is_whitelisted = EXCLUDED.is_whitelisted,
			last_risk_update = EXCLUDED.last_risk_update,
			country_code = EXCLUDED.country_code,
			is_high_risk_country = EXCLUDED.is_high_risk_country,
			pep_status = EXCLUDED.pep_status,
			sanction_status = EXCLUDED.sanction_status,
			customer_type = EXCLUDED.customer_type,
			business_type = EXCLUDED.business_type,
			risk_factors = EXCLUDED.risk_factors,
			updated_at = EXCLUDED.updated_at
	`

	_, err = p.db.ExecContext(ctx, query,
		user.ID, user.UserID, user.RiskLevel, user.RiskScore, user.KYCStatus,
		user.IsBlacklisted, user.IsWhitelisted, user.LastRiskUpdate,
		user.CountryCode, user.IsHighRiskCountry, user.PEPStatus,
		user.SanctionStatus, user.CustomerType, user.BusinessType,
		riskFactorsJSON, user.CreatedAt, user.UpdatedAt,
	)

	return err
}

// GetUser retrieves an AML user profile by user ID
func (p *PostgreSQLStorage) GetUser(ctx context.Context, userID uuid.UUID) (*aml.AMLUser, error) {
	query := `
		SELECT id, user_id, risk_level, risk_score, kyc_status, 
			   is_blacklisted, is_whitelisted, last_risk_update, 
			   country_code, is_high_risk_country, pep_status, 
			   sanction_status, customer_type, business_type, 
			   risk_factors, created_at, updated_at
		FROM aml_users 
		WHERE user_id = $1
	`

	var user aml.AMLUser
	var riskFactorsJSON []byte

	err := p.db.QueryRowContext(ctx, query, userID).Scan(
		&user.ID, &user.UserID, &user.RiskLevel, &user.RiskScore, &user.KYCStatus,
		&user.IsBlacklisted, &user.IsWhitelisted, &user.LastRiskUpdate,
		&user.CountryCode, &user.IsHighRiskCountry, &user.PEPStatus,
		&user.SanctionStatus, &user.CustomerType, &user.BusinessType,
		&riskFactorsJSON, &user.CreatedAt, &user.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("user not found")
		}
		return nil, err
	}

	err = json.Unmarshal(riskFactorsJSON, &user.RiskFactors)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal risk factors: %v", err)
	}

	return &user, nil
}

// StoreAlert stores a compliance alert
func (p *PostgreSQLStorage) StoreAlert(ctx context.Context, alert *aml.ComplianceAlert) error {
	metadataJSON, err := json.Marshal(alert.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %v", err)
	}

	query := `
		INSERT INTO compliance_alerts (
			id, user_id, type, severity, message, status, 
			assigned_to, notes, metadata, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
		)
	`

	_, err = p.db.ExecContext(ctx, query,
		alert.ID, alert.UserID, alert.Type, alert.Severity, alert.Message,
		alert.Status, alert.AssignedTo, alert.Notes, metadataJSON,
		alert.CreatedAt, alert.UpdatedAt,
	)

	return err
}

// GetActiveAlerts retrieves active compliance alerts
func (p *PostgreSQLStorage) GetActiveAlerts(ctx context.Context) ([]aml.ComplianceAlert, error) {
	query := `
		SELECT id, user_id, type, severity, message, status, 
			   assigned_to, notes, metadata, created_at, updated_at
		FROM compliance_alerts 
		WHERE status IN ('OPEN', 'IN_PROGRESS') 
		ORDER BY created_at DESC
	`

	rows, err := p.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var alerts []aml.ComplianceAlert
	for rows.Next() {
		var alert aml.ComplianceAlert
		var metadataJSON []byte

		err := rows.Scan(
			&alert.ID, &alert.UserID, &alert.Type, &alert.Severity, &alert.Message,
			&alert.Status, &alert.AssignedTo, &alert.Notes, &metadataJSON,
			&alert.CreatedAt, &alert.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}

		err = json.Unmarshal(metadataJSON, &alert.Metadata)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %v", err)
		}

		alerts = append(alerts, alert)
	}

	return alerts, rows.Err()
}

// UpdateAlertStatus updates the status of a compliance alert
func (p *PostgreSQLStorage) UpdateAlertStatus(ctx context.Context, alertID, status, assignedTo, notes string) error {
	query := `
		UPDATE compliance_alerts 
		SET status = $2, assigned_to = $3, notes = $4, updated_at = $5
		WHERE id = $1
	`

	_, err := p.db.ExecContext(ctx, query, alertID, status, assignedTo, notes, time.Now())
	return err
}
