package service

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// UserComplianceRecord represents user compliance status in the database
type UserComplianceRecord struct {
	ID           uint            `gorm:"primaryKey"`
	UserID       uuid.UUID       `gorm:"type:uuid;uniqueIndex;not null"`
	KYCLevel     int             `gorm:"not null;default:0"`
	RiskLevel    int             `gorm:"not null;default:1"`
	RiskScore    decimal.Decimal `gorm:"type:decimal(5,2);not null;default:10.00"`
	Status       string          `gorm:"size:50;not null;default:'active'"`
	Restrictions []string        `gorm:"type:text[]"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DeletedAt    gorm.DeletedAt `gorm:"index"`
}

// TableName returns the table name for UserComplianceRecord
func (UserComplianceRecord) TableName() string {
	return "user_compliance_status"
}

// TransactionRecord represents transaction data for compliance analysis
type TransactionRecord struct {
	ID            uint                   `gorm:"primaryKey"`
	TransactionID string                 `gorm:"size:255;uniqueIndex;not null"`
	UserID        uuid.UUID              `gorm:"type:uuid;index;not null"`
	Amount        decimal.Decimal        `gorm:"type:decimal(20,8);not null"`
	Currency      string                 `gorm:"size:10;not null"`
	Type          string                 `gorm:"size:50;not null"`
	Status        string                 `gorm:"size:50;not null;default:'pending'"`
	Metadata      map[string]interface{} `gorm:"type:jsonb"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
	DeletedAt     gorm.DeletedAt `gorm:"index"`
}

// TableName returns the table name for TransactionRecord
func (TransactionRecord) TableName() string {
	return "compliance_transactions"
}

// ComplianceEventRecord represents compliance events in the database
type ComplianceEventRecord struct {
	ID          uint                   `gorm:"primaryKey"`
	EventID     uuid.UUID              `gorm:"type:uuid;uniqueIndex;not null"`
	UserID      uuid.UUID              `gorm:"type:uuid;index"`
	EventType   string                 `gorm:"size:100;not null"`
	Description string                 `gorm:"type:text"`
	Metadata    map[string]interface{} `gorm:"type:jsonb"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   gorm.DeletedAt `gorm:"index"`
}

// TableName returns the table name for ComplianceEventRecord
func (ComplianceEventRecord) TableName() string {
	return "compliance_events"
}

// KYCRecord represents KYC verification records
type KYCRecord struct {
	ID          uint      `gorm:"primaryKey"`
	UserID      uuid.UUID `gorm:"type:uuid;index;not null"`
	KYCLevel    int       `gorm:"not null"`
	Status      string    `gorm:"size:50;not null"`
	InitiatedAt time.Time `gorm:"not null"`
	CompletedAt *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   gorm.DeletedAt `gorm:"index"`
}

// TableName returns the table name for KYCRecord
func (KYCRecord) TableName() string {
	return "kyc_records"
}

// PolicyRecord represents compliance policies in the database
type PolicyRecord struct {
	ID          uint                   `gorm:"primaryKey"`
	PolicyID    string                 `gorm:"size:255;uniqueIndex;not null"`
	PolicyType  string                 `gorm:"size:100;not null"`
	PolicyData  map[string]interface{} `gorm:"type:jsonb;not null"`
	Version     string                 `gorm:"size:50;not null"`
	EffectiveAt time.Time              `gorm:"not null"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   gorm.DeletedAt `gorm:"index"`
}

// TableName returns the table name for PolicyRecord
func (PolicyRecord) TableName() string {
	return "compliance_policies"
}

// ExternalReportRecord represents external compliance reports
type ExternalReportRecord struct {
	ID          uint                   `gorm:"primaryKey"`
	ReportID    string                 `gorm:"size:255;uniqueIndex;not null"`
	Source      string                 `gorm:"size:100;not null"`
	ReportType  string                 `gorm:"size:100;not null"`
	ReportData  map[string]interface{} `gorm:"type:jsonb;not null"`
	ProcessedAt time.Time              `gorm:"not null"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   gorm.DeletedAt `gorm:"index"`
}

// TableName returns the table name for ExternalReportRecord
func (ExternalReportRecord) TableName() string {
	return "external_reports"
}

// AMLCheckRecord represents AML check results
type AMLCheckRecord struct {
	ID        uint                   `gorm:"primaryKey"`
	CheckID   uuid.UUID              `gorm:"type:uuid;uniqueIndex;not null"`
	UserID    uuid.UUID              `gorm:"type:uuid;index"`
	Amount    decimal.Decimal        `gorm:"type:decimal(20,8)"`
	Currency  string                 `gorm:"size:10"`
	Result    string                 `gorm:"size:50;not null"`
	RiskScore decimal.Decimal        `gorm:"type:decimal(5,2)"`
	Flags     []string               `gorm:"type:text[]"`
	Metadata  map[string]interface{} `gorm:"type:jsonb"`
	CheckedAt time.Time              `gorm:"not null"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

// TableName returns the table name for AMLCheckRecord
func (AMLCheckRecord) TableName() string {
	return "aml_checks"
}

// SanctionsCheckRecord represents sanctions check results
type SanctionsCheckRecord struct {
	ID           uint                   `gorm:"primaryKey"`
	CheckID      uuid.UUID              `gorm:"type:uuid;uniqueIndex;not null"`
	Name         string                 `gorm:"size:255;not null"`
	DateOfBirth  string                 `gorm:"size:50"`
	Nationality  string                 `gorm:"size:100"`
	Result       string                 `gorm:"size:50;not null"`
	MatchDetails map[string]interface{} `gorm:"type:jsonb"`
	CheckedAt    time.Time              `gorm:"not null"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DeletedAt    gorm.DeletedAt `gorm:"index"`
}

// TableName returns the table name for SanctionsCheckRecord
func (SanctionsCheckRecord) TableName() string {
	return "sanctions_checks"
}

// AlertRecord represents compliance alerts
type AlertRecord struct {
	ID          uint                   `gorm:"primaryKey"`
	AlertID     uuid.UUID              `gorm:"type:uuid;uniqueIndex;not null"`
	UserID      uuid.UUID              `gorm:"type:uuid;index"`
	AlertType   string                 `gorm:"size:100;not null"`
	Severity    string                 `gorm:"size:50;not null"`
	Status      string                 `gorm:"size:50;not null;default:'pending'"`
	Title       string                 `gorm:"size:255;not null"`
	Description string                 `gorm:"type:text"`
	Metadata    map[string]interface{} `gorm:"type:jsonb"`
	TriggeredAt time.Time              `gorm:"not null"`
	ResolvedAt  *time.Time
	ResolvedBy  *uuid.UUID `gorm:"type:uuid"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   gorm.DeletedAt `gorm:"index"`
}

// TableName returns the table name for AlertRecord
func (AlertRecord) TableName() string {
	return "compliance_alerts"
}

// AutoMigrate runs database migrations for all compliance models
func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&UserComplianceRecord{},
		&TransactionRecord{},
		&ComplianceEventRecord{},
		&KYCRecord{},
		&PolicyRecord{},
		&ExternalReportRecord{},
		&AMLCheckRecord{},
		&SanctionsCheckRecord{},
		&AlertRecord{},
	)
}
