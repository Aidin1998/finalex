package risk

import (
	"context"

	"github.com/google/uuid"
)

// Service defines the consolidated risk and compliance service interface
type Service interface {
	// AML compliance operations
	MonitorTransaction(ctx context.Context, userID uuid.UUID, txType, details string, amount float64) (interface{}, error)
	GetRiskProfile(ctx context.Context, userID uuid.UUID) (interface{}, error)
	UpdateRiskProfile(ctx context.Context, userID uuid.UUID, riskLevel string, riskScore float64) error

	// Manipulation detection operations
	AnalyzeOrderPattern(ctx context.Context, userID string, market string) (interface{}, error)
	DetectWashTrading(ctx context.Context, trade interface{}) (bool, error)
	GetManipulationAlerts(ctx context.Context, from, to string) ([]interface{}, error)

	// Audit operations
	AuditEvent(ctx context.Context, eventType, userID, details string, data map[string]interface{}) error
	GetAuditTrail(ctx context.Context, userID string, eventType string, from, to string) ([]interface{}, error)

	// Monitoring operations
	GetSystemStatus(ctx context.Context) (map[string]interface{}, error)
	HealthCheck(ctx context.Context) (map[string]string, error)
	RegisterMetric(name string, value float64, tags map[string]string) error

	// Service lifecycle
	Start() error
	Stop() error
}

// NewService creates a new consolidated risk and compliance service
// func NewService(logger *zap.Logger, db *gorm.DB) (Service, error)
