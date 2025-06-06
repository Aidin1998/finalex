package monitoring

import (
	"time"

	"github.com/Aidin1998/finalex/internal/compliance/interfaces"
)

// MonitoringAlertModel represents monitoring alerts in the database
type MonitoringAlertModel struct {
	ID        string                 `gorm:"primaryKey;type:varchar(36)" json:"id"`
	UserID    string                 `gorm:"type:varchar(36);index" json:"user_id"`
	AlertType string                 `gorm:"type:varchar(100);index" json:"alert_type"`
	Severity  string                 `gorm:"type:varchar(20);index" json:"severity"`
	Message   string                 `gorm:"type:text" json:"message"`
	Data      map[string]interface{} `gorm:"type:jsonb" json:"data"`
	Status    string                 `gorm:"type:varchar(20);index" json:"status"`
	Timestamp time.Time              `gorm:"index" json:"timestamp"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
}

// TableName specifies the table name for MonitoringAlertModel
func (MonitoringAlertModel) TableName() string {
	return "monitoring_alerts"
}

// ToInterface converts the model to interface type
func (m *MonitoringAlertModel) ToInterface() interfaces.MonitoringAlert {
	return interfaces.MonitoringAlert{
		ID:        m.ID,
		UserID:    m.UserID,
		AlertType: m.AlertType,
		Severity:  m.Severity,
		Message:   m.Message,
		Data:      m.Data,
		Status:    m.Status,
		Timestamp: m.Timestamp,
	}
}

// MonitoringPolicyModel represents monitoring policies in the database
type MonitoringPolicyModel struct {
	ID         string                 `gorm:"primaryKey;type:varchar(36)" json:"id"`
	Name       string                 `gorm:"type:varchar(200);unique" json:"name"`
	AlertType  string                 `gorm:"type:varchar(100);index" json:"alert_type"`
	Enabled    bool                   `gorm:"default:true" json:"enabled"`
	Threshold  float64                `json:"threshold"`
	TimeWindow time.Duration          `json:"time_window"`
	Action     string                 `gorm:"type:varchar(100)" json:"action"`
	Conditions map[string]interface{} `gorm:"type:jsonb" json:"conditions"`
	Recipients []string               `gorm:"type:jsonb" json:"recipients"`
	CreatedAt  time.Time              `json:"created_at"`
	UpdatedAt  time.Time              `json:"updated_at"`
}

// TableName specifies the table name for MonitoringPolicyModel
func (MonitoringPolicyModel) TableName() string {
	return "monitoring_policies"
}

// ToInterface converts the model to interface type
func (m *MonitoringPolicyModel) ToInterface() interfaces.MonitoringPolicy {
	return interfaces.MonitoringPolicy{
		ID:         m.ID,
		Name:       m.Name,
		AlertType:  m.AlertType,
		Enabled:    m.Enabled,
		Threshold:  m.Threshold,
		TimeWindow: m.TimeWindow,
		Action:     m.Action,
		Conditions: m.Conditions,
		Recipients: m.Recipients,
	}
}

// MonitoringMetricModel represents monitoring metrics in the database
type MonitoringMetricModel struct {
	ID        string                 `gorm:"primaryKey;type:varchar(36)" json:"id"`
	Name      string                 `gorm:"type:varchar(200);index" json:"name"`
	Type      string                 `gorm:"type:varchar(50)" json:"type"`
	Value     float64                `json:"value"`
	Tags      map[string]interface{} `gorm:"type:jsonb" json:"tags"`
	Timestamp time.Time              `gorm:"index" json:"timestamp"`
	CreatedAt time.Time              `json:"created_at"`
}

// TableName specifies the table name for MonitoringMetricModel
func (MonitoringMetricModel) TableName() string {
	return "monitoring_metrics"
}

// MonitoringSubscriptionModel represents alert subscriptions in the database
type MonitoringSubscriptionModel struct {
	ID        string    `gorm:"primaryKey;type:varchar(36)" json:"id"`
	AlertType string    `gorm:"type:varchar(100);index" json:"alert_type"`
	UserID    string    `gorm:"type:varchar(36);index" json:"user_id"`
	Endpoint  string    `gorm:"type:varchar(500)" json:"endpoint"`
	Method    string    `gorm:"type:varchar(20)" json:"method"`
	Headers   string    `gorm:"type:jsonb" json:"headers"`
	Active    bool      `gorm:"default:true" json:"active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName specifies the table name for MonitoringSubscriptionModel
func (MonitoringSubscriptionModel) TableName() string {
	return "monitoring_subscriptions"
}

// MonitoringDashboardModel represents monitoring dashboards
type MonitoringDashboardModel struct {
	ID          string                 `gorm:"primaryKey;type:varchar(36)" json:"id"`
	Name        string                 `gorm:"type:varchar(200)" json:"name"`
	Description string                 `gorm:"type:text" json:"description"`
	Config      map[string]interface{} `gorm:"type:jsonb" json:"config"`
	UserID      string                 `gorm:"type:varchar(36);index" json:"user_id"`
	Public      bool                   `gorm:"default:false" json:"public"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

// TableName specifies the table name for MonitoringDashboardModel
func (MonitoringDashboardModel) TableName() string {
	return "monitoring_dashboards"
}

// MonitoringThresholdModel represents monitoring thresholds
type MonitoringThresholdModel struct {
	ID         string                 `gorm:"primaryKey;type:varchar(36)" json:"id"`
	MetricName string                 `gorm:"type:varchar(200);index" json:"metric_name"`
	Operator   string                 `gorm:"type:varchar(10)" json:"operator"` // >, <, >=, <=, ==, !=
	Value      float64                `json:"value"`
	TimeWindow time.Duration          `json:"time_window"`
	AlertType  string                 `gorm:"type:varchar(100)" json:"alert_type"`
	Severity   string                 `gorm:"type:varchar(20)" json:"severity"`
	Conditions map[string]interface{} `gorm:"type:jsonb" json:"conditions"`
	Enabled    bool                   `gorm:"default:true" json:"enabled"`
	CreatedAt  time.Time              `json:"created_at"`
	UpdatedAt  time.Time              `json:"updated_at"`
}

// TableName specifies the table name for MonitoringThresholdModel
func (MonitoringThresholdModel) TableName() string {
	return "monitoring_thresholds"
}
