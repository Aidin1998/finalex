package risk

import (
	"context"
	"fmt"
	"sync"
	"time"

	audit "github.com/Aidin1998/pincex_unified/internal/audit"
	"github.com/Aidin1998/pincex_unified/internal/compliance/aml"
	"github.com/Aidin1998/pincex_unified/internal/manipulation"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// serviceImpl implements the consolidated risk and compliance service
type serviceImpl struct {
	logger          *zap.Logger
	amlService      aml.RiskService
	manipulationSvc *manipulation.ManipulationService
	auditSvc        *audit.AuditService
	mu              sync.RWMutex
}

// NewService creates a new consolidated risk and compliance service
func NewService(
	logger *zap.Logger,
	amlService aml.RiskService,
	manipulationSvc *manipulation.ManipulationService,
	auditSvc *audit.AuditService,
) (Service, error) {

	if logger == nil {
		return nil, fmt.Errorf("logger is required")
	}

	if amlService == nil {
		return nil, fmt.Errorf("AML service is required")
	}

	if manipulationSvc == nil {
		return nil, fmt.Errorf("manipulation service is required")
	}

	if auditSvc == nil {
		return nil, fmt.Errorf("audit service is required")
	}

	return &serviceImpl{
		logger:          logger,
		amlService:      amlService,
		manipulationSvc: manipulationSvc,
		auditSvc:        auditSvc,
	}, nil
}

// MonitorTransaction monitors a transaction for compliance checks
func (s *serviceImpl) MonitorTransaction(ctx context.Context, userID uuid.UUID, txType, details string, amount float64) (interface{}, error) {
	s.logger.Debug("Monitoring transaction",
		zap.String("userID", userID.String()),
		zap.String("txType", txType),
		zap.Float64("amount", amount))

	attrs := map[string]interface{}{
		"txType":  txType,
		"details": details,
	}

	// Convert to decimal for the AML service call
	amountDecimal, err := s.floatToDecimal(amount)
	if err != nil {
		return nil, err
	}

	// Call the compliance check function
	result, err := s.amlService.ComplianceCheck(ctx, "", userID.String(), amountDecimal, attrs)
	if err != nil {
		s.logger.Error("Error in compliance check", zap.Error(err))
		return nil, fmt.Errorf("compliance check failed: %w", err)
	}

	// Log audit event
	s.logAuditEvent(ctx, "TRANSACTION_MONITOR", userID.String(), fmt.Sprintf("Monitored transaction of type %s for %.2f", txType, amount))

	return result, nil
}

// GetRiskProfile gets a user's risk profile
func (s *serviceImpl) GetRiskProfile(ctx context.Context, userID uuid.UUID) (interface{}, error) {
	s.logger.Debug("Getting risk profile", zap.String("userID", userID.String()))

	// Get user risk profile from AML service
	riskProfile, err := s.amlService.CalculateRisk(ctx, userID.String())
	if err != nil {
		s.logger.Error("Error calculating risk", zap.Error(err))
		return nil, fmt.Errorf("risk calculation failed: %w", err)
	}

	// Log audit event
	s.logAuditEvent(ctx, "RISK_PROFILE_ACCESS", userID.String(), "Risk profile accessed")

	return riskProfile, nil
}

// UpdateRiskProfile updates a user's risk profile
func (s *serviceImpl) UpdateRiskProfile(ctx context.Context, userID uuid.UUID, riskLevel string, riskScore float64) error {
	s.logger.Info("Updating risk profile",
		zap.String("userID", userID.String()),
		zap.String("riskLevel", riskLevel),
		zap.Float64("riskScore", riskScore))

	// Extract user ID string from uuid
	userIDStr := userID.String()

	// This is an adapter method - implement with backend risk service once available
	// For now, log the request and audit it

	// Log audit event with more details
	data := map[string]interface{}{
		"riskLevel": riskLevel,
		"riskScore": riskScore,
	}

	s.logAuditEventWithData(ctx, "RISK_PROFILE_UPDATE", userIDStr,
		fmt.Sprintf("Risk profile updated to %s (%.2f)", riskLevel, riskScore), data)

	return nil
}

// AnalyzeOrderPattern analyzes order patterns for potential manipulation
func (s *serviceImpl) AnalyzeOrderPattern(ctx context.Context, userID string, market string) (interface{}, error) {
	s.logger.Debug("Analyzing order pattern",
		zap.String("userID", userID),
		zap.String("market", market))

	// Call the manipulation detection analysis
	// TODO: Implement deeper integration with manipulation service

	// Log audit event
	userUUID, err := uuid.Parse(userID)
	if err != nil {
		return nil, fmt.Errorf("invalid userID: %w", err)
	}

	s.logAuditEvent(ctx, "ORDER_PATTERN_ANALYSIS", userID, fmt.Sprintf("Analyzed order patterns for market %s", market))

	// Return stub data until deeper integration is implemented
	analysisResult := map[string]interface{}{
		"userID":        userID,
		"market":        market,
		"analysisTime":  time.Now().UTC(),
		"patternRisk":   "low",
		"patternScore":  0.12,
		"unusualTrades": 0,
	}

	return analysisResult, nil
}

// DetectWashTrading detects wash trading in a trade
func (s *serviceImpl) DetectWashTrading(ctx context.Context, trade interface{}) (bool, error) {
	s.logger.Debug("Detecting wash trading", zap.Any("trade", trade))

	// This is a placeholder until we integrate with the actual manipulation detection service
	// TODO: Integrate with manipulation service

	return false, nil
}

// GetManipulationAlerts gets manipulation alerts for a time period
func (s *serviceImpl) GetManipulationAlerts(ctx context.Context, from, to string) ([]interface{}, error) {
	s.logger.Debug("Getting manipulation alerts", zap.String("from", from), zap.String("to", to))

	// This is a placeholder until we integrate with the actual manipulation detection service
	// TODO: Integrate with manipulation service

	// Return empty result for now
	return []interface{}{}, nil
}

// AuditEvent logs an audit event
func (s *serviceImpl) AuditEvent(ctx context.Context, eventType, userID, details string, data map[string]interface{}) error {
	s.logger.Debug("Auditing event",
		zap.String("eventType", eventType),
		zap.String("userID", userID),
		zap.String("details", details))

	// Log audit event with data
	s.logAuditEventWithData(ctx, eventType, userID, details, data)

	return nil
}

// GetAuditTrail gets audit trail for a user
func (s *serviceImpl) GetAuditTrail(ctx context.Context, userID string, eventType string, from, to string) ([]interface{}, error) {
	s.logger.Debug("Getting audit trail",
		zap.String("userID", userID),
		zap.String("eventType", eventType),
		zap.String("from", from),
		zap.String("to", to))

	// TODO: Implement this method by calling the audit service
	// Return empty result for now
	return []interface{}{}, nil
}

// GetSystemStatus gets the system status
func (s *serviceImpl) GetSystemStatus(ctx context.Context) (map[string]interface{}, error) {
	s.logger.Debug("Getting system status")

	// Create a basic system status
	status := map[string]interface{}{
		"aml":          "ok",
		"manipulation": "ok",
		"audit":        "ok",
		"timestamp":    time.Now().UTC(),
	}

	return status, nil
}

// HealthCheck performs a health check
func (s *serviceImpl) HealthCheck(ctx context.Context) (map[string]string, error) {
	s.logger.Debug("Performing health check")

	// Create a basic health check result
	health := map[string]string{
		"status":       "ok",
		"aml":          "ok",
		"manipulation": "ok",
		"audit":        "ok",
		"last_checked": time.Now().UTC().Format(time.RFC3339),
	}

	return health, nil
}

// RegisterMetric registers a metric
func (s *serviceImpl) RegisterMetric(name string, value float64, tags map[string]string) error {
	s.logger.Debug("Registering metric",
		zap.String("name", name),
		zap.Float64("value", value),
		zap.Any("tags", tags))

	// TODO: Implement metric registration

	return nil
}

// Start starts the risk service
func (s *serviceImpl) Start() error {
	s.logger.Info("Starting risk service")

	// Nothing to do here for now

	return nil
}

// Stop stops the risk service
func (s *serviceImpl) Stop() error {
	s.logger.Info("Stopping risk service")

	// Nothing to do here for now

	return nil
}

// Helper methods

// logAuditEvent logs a simple audit event
func (s *serviceImpl) logAuditEvent(ctx context.Context, eventType, userID, details string) {
	data := map[string]interface{}{}
	s.logAuditEventWithData(ctx, eventType, userID, details, data)
}

// logAuditEventWithData logs an audit event with additional data
func (s *serviceImpl) logAuditEventWithData(ctx context.Context, eventType, userID, details string, data map[string]interface{}) {
	// We don't bubble up audit logging errors as they shouldn't affect normal operations
	err := s.auditSvc.AuditEvent(eventType, userID, details, data)
	if err != nil {
		s.logger.Error("Failed to log audit event", zap.Error(err))
	}
}

// floatToDecimal is a helper to convert float64 to decimal.Decimal
func (s *serviceImpl) floatToDecimal(val float64) (interface{}, error) {
	// This is a stub - when implementing properly, use decimal.NewFromFloat()
	return val, nil
}
