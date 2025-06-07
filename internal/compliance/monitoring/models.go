package monitoring

import (
	"time"

	"github.com/Aidin1998/finalex/internal/compliance/interfaces"
	"github.com/google/uuid"
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
	// Parse ID as UUID, use empty UUID if parsing fails
	id, err := uuid.Parse(m.ID)
	if err != nil {
		id = uuid.Nil
	}

	// Convert string severity to AlertSeverity enum
	var severity interfaces.AlertSeverity
	switch m.Severity {
	case "low":
		severity = interfaces.AlertSeverityLow
	case "medium":
		severity = interfaces.AlertSeverityMedium
	case "high":
		severity = interfaces.AlertSeverityHigh
	case "critical":
		severity = interfaces.AlertSeverityCritical
	default:
		severity = interfaces.AlertSeverityLow
	}

	// Convert string status to AlertStatus enum
	var status interfaces.AlertStatus
	switch m.Status {
	case "pending":
		status = interfaces.AlertStatusPending
	case "acknowledged":
		status = interfaces.AlertStatusAcknowledged
	case "investigating":
		status = interfaces.AlertStatusInvestigating
	case "resolved":
		status = interfaces.AlertStatusResolved
	case "dismissed":
		status = interfaces.AlertStatusDismissed
	default:
		status = interfaces.AlertStatusPending
	}

	return interfaces.MonitoringAlert{
		ID:        id,
		UserID:    m.UserID,
		AlertType: m.AlertType,
		Severity:  severity,
		Message:   m.Message,
		Details:   m.Data, // Use Details instead of Data
		Status:    status,
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
	// Parse ID as UUID, use empty UUID if parsing fails
	id, err := uuid.Parse(m.ID)
	if err != nil {
		id = uuid.Nil
	}

	// Convert conditions map to PolicyCondition slice
	var conditions []interfaces.PolicyCondition
	if m.Conditions != nil {
		// This is a simplified conversion - in practice you'd need more sophisticated logic
		for field, value := range m.Conditions {
			conditions = append(conditions, interfaces.PolicyCondition{
				Field:    field,
				Operator: "equals", // Default operator
				Value:    value,
				Type:     "string", // Default type
			})
		}
	}
	// Convert to PolicyAction slice - simplified for now
	var actions []interfaces.PolicyAction
	if m.Action != "" {
		actions = append(actions, interfaces.PolicyAction{
			Type:       m.Action,
			Parameters: make(map[string]interface{}),
		})
	}

	// Convert legacy fields to thresholds map
	thresholds := make(map[string]interface{})
	if m.Threshold > 0 {
		thresholds["threshold"] = m.Threshold
	}
	if m.TimeWindow > 0 {
		thresholds["time_window"] = m.TimeWindow.String()
	}

	return interfaces.MonitoringPolicy{
		ID:          m.ID,
		Name:        m.Name,
		Description: "", // Not available in model
		AlertType:   m.AlertType,
		Enabled:     m.Enabled,
		Conditions:  conditions,
		Actions:     actions,
		Thresholds:  thresholds,
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
		CreatedBy:   id, // Using policy ID as created by for now
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
