package manipulation

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// ManipulationAlertModel represents manipulation alerts in the database
type ManipulationAlertModel struct {
	ID              uuid.UUID              `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	UserID          uuid.UUID              `gorm:"type:uuid;index;not null" json:"user_id"`
	Market          string                 `gorm:"type:varchar(20);index;not null" json:"market"`
	AlertType       string                 `gorm:"type:varchar(50);index;not null" json:"alert_type"`
	Severity        string                 `gorm:"type:varchar(20);index;not null" json:"severity"`
	RiskScore       decimal.Decimal        `gorm:"type:decimal(10,4);not null" json:"risk_score"`
	Description     string                 `gorm:"type:text" json:"description"`
	Details         map[string]interface{} `gorm:"type:jsonb" json:"details"`
	Evidence        []string               `gorm:"type:jsonb" json:"evidence"`
	Status          string                 `gorm:"type:varchar(20);index;default:'pending'" json:"status"`
	DetectedAt      time.Time              `gorm:"index;not null" json:"detected_at"`
	ResolvedAt      *time.Time             `gorm:"index" json:"resolved_at"`
	ResolvedBy      *uuid.UUID             `gorm:"type:uuid" json:"resolved_by"`
	Resolution      string                 `gorm:"type:text" json:"resolution"`
	InvestigationID *string                `gorm:"type:varchar(100)" json:"investigation_id"`
	CreatedAt       time.Time              `json:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at"`
	DeletedAt       gorm.DeletedAt         `gorm:"index" json:"deleted_at"`
}

// TableName specifies the table name for ManipulationAlertModel
func (ManipulationAlertModel) TableName() string {
	return "manipulation_alerts"
}

// ManipulationPatternModel represents detected manipulation patterns
type ManipulationPatternModel struct {
	ID          uuid.UUID              `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	AlertID     uuid.UUID              `gorm:"type:uuid;index;not null" json:"alert_id"`
	UserID      uuid.UUID              `gorm:"type:uuid;index;not null" json:"user_id"`
	Market      string                 `gorm:"type:varchar(20);index;not null" json:"market"`
	PatternType string                 `gorm:"type:varchar(50);index;not null" json:"pattern_type"`
	Confidence  decimal.Decimal        `gorm:"type:decimal(10,4);not null" json:"confidence"`
	Description string                 `gorm:"type:text" json:"description"`
	Evidence    map[string]interface{} `gorm:"type:jsonb" json:"evidence"`
	TimeWindow  int64                  `gorm:"not null" json:"time_window"` // Duration in nanoseconds
	DetectedAt  time.Time              `gorm:"index;not null" json:"detected_at"`
	Metadata    map[string]interface{} `gorm:"type:jsonb" json:"metadata"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`

	// Foreign key relationship
	Alert ManipulationAlertModel `gorm:"foreignKey:AlertID;references:ID"`
}

// TableName specifies the table name for ManipulationPatternModel
func (ManipulationPatternModel) TableName() string {
	return "manipulation_patterns"
}

// ManipulationInvestigationModel represents ongoing investigations
type ManipulationInvestigationModel struct {
	ID             string                 `gorm:"primary_key;type:varchar(100)" json:"id"`
	AlertID        uuid.UUID              `gorm:"type:uuid;index;not null" json:"alert_id"`
	UserID         uuid.UUID              `gorm:"type:uuid;index;not null" json:"user_id"`
	Market         string                 `gorm:"type:varchar(20);index;not null" json:"market"`
	InvestigatorID uuid.UUID              `gorm:"type:uuid;index;not null" json:"investigator_id"`
	Status         string                 `gorm:"type:varchar(20);index;default:'pending'" json:"status"`
	Priority       string                 `gorm:"type:varchar(20);index;default:'medium'" json:"priority"`
	Title          string                 `gorm:"type:varchar(200);not null" json:"title"`
	Description    string                 `gorm:"type:text" json:"description"`
	Findings       map[string]interface{} `gorm:"type:jsonb" json:"findings"`
	Evidence       []string               `gorm:"type:jsonb" json:"evidence"`
	Notes          string                 `gorm:"type:text" json:"notes"`
	Conclusion     string                 `gorm:"type:text" json:"conclusion"`
	Recommendation string                 `gorm:"type:text" json:"recommendation"`
	StartedAt      time.Time              `gorm:"index;not null" json:"started_at"`
	CompletedAt    *time.Time             `gorm:"index" json:"completed_at"`
	DueDate        *time.Time             `gorm:"index" json:"due_date"`
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at"`
	DeletedAt      gorm.DeletedAt         `gorm:"index" json:"deleted_at"`

	// Foreign key relationship
	Alert ManipulationAlertModel `gorm:"foreignKey:AlertID;references:ID"`
}

// TableName specifies the table name for ManipulationInvestigationModel
func (ManipulationInvestigationModel) TableName() string {
	return "manipulation_investigations"
}

// ManipulationActionModel represents actions taken in response to manipulation
type ManipulationActionModel struct {
	ID           uuid.UUID              `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	AlertID      uuid.UUID              `gorm:"type:uuid;index;not null" json:"alert_id"`
	UserID       uuid.UUID              `gorm:"type:uuid;index;not null" json:"user_id"`
	ActionType   string                 `gorm:"type:varchar(50);index;not null" json:"action_type"`
	Status       string                 `gorm:"type:varchar(20);index;default:'pending'" json:"status"`
	Description  string                 `gorm:"type:text" json:"description"`
	Parameters   map[string]interface{} `gorm:"type:jsonb" json:"parameters"`
	ExecutedBy   uuid.UUID              `gorm:"type:uuid;index;not null" json:"executed_by"`
	ExecutedAt   time.Time              `gorm:"index;not null" json:"executed_at"`
	CompletedAt  *time.Time             `gorm:"index" json:"completed_at"`
	Result       string                 `gorm:"type:text" json:"result"`
	ErrorMessage string                 `gorm:"type:text" json:"error_message"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`

	// Foreign key relationship
	Alert ManipulationAlertModel `gorm:"foreignKey:AlertID;references:ID"`
}

// TableName specifies the table name for ManipulationActionModel
func (ManipulationActionModel) TableName() string {
	return "manipulation_actions"
}

// ManipulationAuditModel represents audit events for manipulation detection
type ManipulationAuditModel struct {
	ID           uuid.UUID              `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	EventType    string                 `gorm:"type:varchar(50);index;not null" json:"event_type"`
	UserID       *uuid.UUID             `gorm:"type:uuid;index" json:"user_id"`
	AlertID      *uuid.UUID             `gorm:"type:uuid;index" json:"alert_id"`
	Market       string                 `gorm:"type:varchar(20);index" json:"market"`
	Description  string                 `gorm:"type:text;not null" json:"description"`
	Details      map[string]interface{} `gorm:"type:jsonb" json:"details"`
	IPAddress    string                 `gorm:"type:varchar(45)" json:"ip_address"`
	UserAgent    string                 `gorm:"type:text" json:"user_agent"`
	ExecutedBy   uuid.UUID              `gorm:"type:uuid;index;not null" json:"executed_by"`
	Timestamp    time.Time              `gorm:"index;not null" json:"timestamp"`
	Hash         string                 `gorm:"type:varchar(64);unique;not null" json:"hash"`
	PreviousHash string                 `gorm:"type:varchar(64)" json:"previous_hash"`
	CreatedAt    time.Time              `json:"created_at"`
}

// TableName specifies the table name for ManipulationAuditModel
func (ManipulationAuditModel) TableName() string {
	return "manipulation_audit"
}

// ManipulationMetricsModel represents metrics for manipulation detection system
type ManipulationMetricsModel struct {
	ID                uuid.UUID       `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	MetricName        string          `gorm:"type:varchar(100);index;not null" json:"metric_name"`
	MetricType        string          `gorm:"type:varchar(20);index;not null" json:"metric_type"`
	Value             decimal.Decimal `gorm:"type:decimal(20,8);not null" json:"value"`
	Market            string          `gorm:"type:varchar(20);index" json:"market"`
	UserID            *uuid.UUID      `gorm:"type:uuid;index" json:"user_id"`
	AlertType         string          `gorm:"type:varchar(50);index" json:"alert_type"`
	Tags              []string        `gorm:"type:jsonb" json:"tags"`
	Timestamp         time.Time       `gorm:"index;not null" json:"timestamp"`
	AggregationPeriod string          `gorm:"type:varchar(20);index" json:"aggregation_period"` // minute, hour, day
	CreatedAt         time.Time       `json:"created_at"`
}

// TableName specifies the table name for ManipulationMetricsModel
func (ManipulationMetricsModel) TableName() string {
	return "manipulation_metrics"
}

// ManipulationConfigModel represents configuration for manipulation detection
type ManipulationConfigModel struct {
	ID          uuid.UUID              `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	ConfigKey   string                 `gorm:"type:varchar(100);unique;not null" json:"config_key"`
	ConfigValue map[string]interface{} `gorm:"type:jsonb;not null" json:"config_value"`
	Description string                 `gorm:"type:text" json:"description"`
	Category    string                 `gorm:"type:varchar(50);index" json:"category"`
	Environment string                 `gorm:"type:varchar(20);index;default:'production'" json:"environment"`
	IsActive    bool                   `gorm:"default:true" json:"is_active"`
	Version     int                    `gorm:"default:1" json:"version"`
	CreatedBy   uuid.UUID              `gorm:"type:uuid;index;not null" json:"created_by"`
	UpdatedBy   *uuid.UUID             `gorm:"type:uuid;index" json:"updated_by"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	DeletedAt   gorm.DeletedAt         `gorm:"index" json:"deleted_at"`
}

// TableName specifies the table name for ManipulationConfigModel
func (ManipulationConfigModel) TableName() string {
	return "manipulation_config"
}
