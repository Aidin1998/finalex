// filepath: c:\Orbit CEX\Finalex\internal\userauth\audit\service.go
package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/userauth/models"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// AuditEventType represents different types of audit events
type AuditEventType string

const (
	// Authentication events
	EventLogin          AuditEventType = "login"
	EventLoginFailed    AuditEventType = "login_failed"
	EventLogout         AuditEventType = "logout"
	EventPasswordChange AuditEventType = "password_change"
	EventPasswordReset  AuditEventType = "password_reset"

	// 2FA events
	Event2FAEnabled    AuditEventType = "2fa_enabled"
	Event2FADisabled   AuditEventType = "2fa_disabled"
	Event2FAVerified   AuditEventType = "2fa_verified"
	Event2FABackupUsed AuditEventType = "2fa_backup_used"

	// Profile events
	EventProfileUpdated AuditEventType = "profile_updated"
	EventEmailVerified  AuditEventType = "email_verified"
	EventPhoneVerified  AuditEventType = "phone_verified"

	// KYC events
	EventKYCStarted           AuditEventType = "kyc_started"
	EventKYCDocumentSubmitted AuditEventType = "kyc_document_submitted"
	EventKYCApproved          AuditEventType = "kyc_approved"
	EventKYCRejected          AuditEventType = "kyc_rejected"

	// Security events
	EventSuspiciousActivity AuditEventType = "suspicious_activity"
	EventAccountLocked      AuditEventType = "account_locked"
	EventAccountUnlocked    AuditEventType = "account_unlocked"
	EventDeviceRegistered   AuditEventType = "device_registered"
	EventDeviceBlocked      AuditEventType = "device_blocked"

	// Data events
	EventDataExport   AuditEventType = "data_export"
	EventDataDeletion AuditEventType = "data_deletion"
	EventGDPRRequest  AuditEventType = "gdpr_request"

	// Administrative events
	EventAdminAction       AuditEventType = "admin_action"
	EventPermissionGranted AuditEventType = "permission_granted"
	EventPermissionRevoked AuditEventType = "permission_revoked"
)

// RiskLevel represents the risk level of an audit event
type RiskLevel string

const (
	RiskLevelLow      RiskLevel = "low"
	RiskLevelMedium   RiskLevel = "medium"
	RiskLevelHigh     RiskLevel = "high"
	RiskLevelCritical RiskLevel = "critical"
)

// AuditContext contains contextual information for audit events
type AuditContext struct {
	UserID      uuid.UUID              `json:"user_id,omitempty"`
	SessionID   string                 `json:"session_id,omitempty"`
	DeviceID    string                 `json:"device_id,omitempty"`
	IPAddress   string                 `json:"ip_address,omitempty"`
	UserAgent   string                 `json:"user_agent,omitempty"`
	Location    string                 `json:"location,omitempty"`
	RequestID   string                 `json:"request_id,omitempty"`
	AdminUserID uuid.UUID              `json:"admin_user_id,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// Service provides comprehensive audit logging services
type Service struct {
	db     *gorm.DB
	logger *zap.Logger
}

// NewService creates a new audit service
func NewService(logger *zap.Logger, db *gorm.DB) *Service {
	return &Service{
		db:     db,
		logger: logger,
	}
}

// LogEvent logs an audit event with full context
func (s *Service) LogEvent(ctx context.Context, eventType AuditEventType, riskLevel RiskLevel, auditCtx AuditContext, details string) error {
	// Serialize metadata
	var metadataJSON string
	if auditCtx.Metadata != nil {
		metadataBytes, _ := json.Marshal(auditCtx.Metadata)
		metadataJSON = string(metadataBytes)
	}

	// Determine severity based on risk level and event type
	severity := s.calculateSeverity(eventType, riskLevel)

	auditLog := &models.UserAuditLog{
		ID:        uuid.New(),
		UserID:    auditCtx.UserID,
		EventType: string(eventType),
		Severity:  severity,
		IPAddress: auditCtx.IPAddress,
		UserAgent: auditCtx.UserAgent,
		DeviceID:  auditCtx.DeviceID,
		SessionID: auditCtx.SessionID,
		Location:  auditCtx.Location,
		RequestID: auditCtx.RequestID,
		Details:   details,
		Metadata:  metadataJSON,
		RiskScore: s.calculateRiskScore(eventType, riskLevel, auditCtx),
		CreatedAt: time.Now(),
	}

	if err := s.db.WithContext(ctx).Create(auditLog).Error; err != nil {
		s.logger.Error("Failed to create audit log", zap.Error(err))
		return fmt.Errorf("failed to create audit log: %w", err)
	}

	// Log to structured logger for immediate alerting
	s.logToStructuredLogger(auditLog, riskLevel)

	return nil
}

// LogLoginEvent logs authentication-related events
func (s *Service) LogLoginEvent(ctx context.Context, userID uuid.UUID, eventType AuditEventType, success bool, ipAddress, userAgent, deviceID string, metadata map[string]interface{}) error {
	riskLevel := RiskLevelLow
	if !success {
		riskLevel = RiskLevelMedium
	}

	// Check for suspicious patterns
	if s.isSuspiciousLogin(ctx, userID, ipAddress, deviceID) {
		riskLevel = RiskLevelHigh
		if metadata == nil {
			metadata = make(map[string]interface{})
		}
		metadata["suspicious_login"] = true
	}

	auditCtx := AuditContext{
		UserID:    userID,
		IPAddress: ipAddress,
		UserAgent: userAgent,
		DeviceID:  deviceID,
		Metadata:  metadata,
	}

	details := fmt.Sprintf("Login attempt from IP %s", ipAddress)
	if !success {
		details = fmt.Sprintf("Failed login attempt from IP %s", ipAddress)
	}

	return s.LogEvent(ctx, eventType, riskLevel, auditCtx, details)
}

// LogSecurityEvent logs security-related events with high priority
func (s *Service) LogSecurityEvent(ctx context.Context, userID uuid.UUID, eventType AuditEventType, ipAddress, details string, metadata map[string]interface{}) error {
	riskLevel := RiskLevelHigh
	if eventType == EventSuspiciousActivity || eventType == EventAccountLocked {
		riskLevel = RiskLevelCritical
	}

	auditCtx := AuditContext{
		UserID:    userID,
		IPAddress: ipAddress,
		Metadata:  metadata,
	}

	return s.LogEvent(ctx, eventType, riskLevel, auditCtx, details)
}

// LogKYCEvent logs KYC-related events
func (s *Service) LogKYCEvent(ctx context.Context, userID uuid.UUID, eventType AuditEventType, kycID uuid.UUID, reviewerID *uuid.UUID, details string) error {
	metadata := map[string]interface{}{
		"kyc_id": kycID.String(),
	}

	if reviewerID != nil {
		metadata["reviewer_id"] = reviewerID.String()
	}

	auditCtx := AuditContext{
		UserID:   userID,
		Metadata: metadata,
	}

	riskLevel := RiskLevelLow
	if eventType == EventKYCRejected {
		riskLevel = RiskLevelMedium
	}

	return s.LogEvent(ctx, eventType, riskLevel, auditCtx, details)
}

// LogAdminAction logs administrative actions
func (s *Service) LogAdminAction(ctx context.Context, adminUserID, targetUserID uuid.UUID, action, details string, metadata map[string]interface{}) error {
	if metadata == nil {
		metadata = make(map[string]interface{})
	}
	metadata["target_user_id"] = targetUserID.String()
	metadata["action"] = action

	auditCtx := AuditContext{
		UserID:      targetUserID,
		AdminUserID: adminUserID,
		Metadata:    metadata,
	}

	return s.LogEvent(ctx, EventAdminAction, RiskLevelMedium, auditCtx, details)
}

// GetUserAuditHistory retrieves audit history for a specific user
func (s *Service) GetUserAuditHistory(ctx context.Context, userID uuid.UUID, limit, offset int, eventTypes []string) ([]models.UserAuditLog, error) {
	query := s.db.WithContext(ctx).Where("user_id = ?", userID)

	if len(eventTypes) > 0 {
		query = query.Where("event_type IN ?", eventTypes)
	}

	var auditLogs []models.UserAuditLog
	err := query.Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&auditLogs).Error

	if err != nil {
		s.logger.Error("Failed to get user audit history", zap.Error(err))
		return nil, fmt.Errorf("failed to get user audit history: %w", err)
	}

	return auditLogs, nil
}

// GetHighRiskEvents retrieves high-risk audit events for monitoring
func (s *Service) GetHighRiskEvents(ctx context.Context, since time.Time, limit int) ([]models.UserAuditLog, error) {
	var auditLogs []models.UserAuditLog
	err := s.db.WithContext(ctx).
		Where("risk_score >= ? AND created_at >= ?", 70, since).
		Order("created_at DESC").
		Limit(limit).
		Find(&auditLogs).Error

	if err != nil {
		s.logger.Error("Failed to get high-risk events", zap.Error(err))
		return nil, fmt.Errorf("failed to get high-risk events: %w", err)
	}

	return auditLogs, nil
}

// GetEventsByIPAddress retrieves events from a specific IP address
func (s *Service) GetEventsByIPAddress(ctx context.Context, ipAddress string, since time.Time, limit int) ([]models.UserAuditLog, error) {
	var auditLogs []models.UserAuditLog
	err := s.db.WithContext(ctx).
		Where("ip_address = ? AND created_at >= ?", ipAddress, since).
		Order("created_at DESC").
		Limit(limit).
		Find(&auditLogs).Error

	if err != nil {
		s.logger.Error("Failed to get events by IP", zap.Error(err))
		return nil, fmt.Errorf("failed to get events by IP: %w", err)
	}

	return auditLogs, nil
}

// AnalyzeSuspiciousActivity analyzes patterns for suspicious activity
func (s *Service) AnalyzeSuspiciousActivity(ctx context.Context, userID uuid.UUID, timeWindow time.Duration) (bool, []string, error) {
	since := time.Now().Add(-timeWindow)

	// Get recent events for the user
	events, err := s.GetUserAuditHistory(ctx, userID, 100, 0, []string{
		string(EventLogin),
		string(EventLoginFailed),
		string(EventPasswordChange),
		string(EventDeviceRegistered),
	})
	if err != nil {
		return false, nil, fmt.Errorf("failed to get user events: %w", err)
	}

	var suspiciousPatterns []string

	// Analyze patterns
	if s.hasMultipleFailedLogins(events, since) {
		suspiciousPatterns = append(suspiciousPatterns, "Multiple failed login attempts")
	}

	if s.hasMultipleIPAddresses(events, since) {
		suspiciousPatterns = append(suspiciousPatterns, "Login attempts from multiple IP addresses")
	}

	if s.hasUnusualDeviceActivity(events, since) {
		suspiciousPatterns = append(suspiciousPatterns, "Unusual device activity")
	}

	return len(suspiciousPatterns) > 0, suspiciousPatterns, nil
}

// Helper methods

func (s *Service) calculateSeverity(eventType AuditEventType, riskLevel RiskLevel) string {
	severityMap := map[RiskLevel]string{
		RiskLevelLow:      "info",
		RiskLevelMedium:   "warning",
		RiskLevelHigh:     "error",
		RiskLevelCritical: "critical",
	}

	// Override severity for certain critical events
	criticalEvents := map[AuditEventType]bool{
		EventSuspiciousActivity: true,
		EventAccountLocked:      true,
		EventGDPRRequest:        true,
		EventDataDeletion:       true,
	}

	if criticalEvents[eventType] {
		return "critical"
	}

	return severityMap[riskLevel]
}

func (s *Service) calculateRiskScore(eventType AuditEventType, riskLevel RiskLevel, auditCtx AuditContext) float64 {
	baseScore := map[RiskLevel]float64{
		RiskLevelLow:      10,
		RiskLevelMedium:   40,
		RiskLevelHigh:     70,
		RiskLevelCritical: 90,
	}

	score := baseScore[riskLevel]

	// Adjust score based on event type
	eventScores := map[AuditEventType]float64{
		EventLoginFailed:        20,
		EventSuspiciousActivity: 80,
		EventAccountLocked:      85,
		EventGDPRRequest:        30,
		EventDataDeletion:       50,
	}

	if eventScore, exists := eventScores[eventType]; exists {
		score = eventScore
	}

	// Adjust based on context
	if auditCtx.Metadata != nil {
		if suspicious, ok := auditCtx.Metadata["suspicious_login"].(bool); ok && suspicious {
			score += 20
		}
	}

	if score > 100 {
		score = 100
	}

	return score
}

func (s *Service) isSuspiciousLogin(ctx context.Context, userID uuid.UUID, ipAddress, deviceID string) bool {
	// Check for recent logins from different IPs
	since := time.Now().Add(-time.Hour * 24)

	var count int64
	s.db.WithContext(ctx).Model(&models.UserAuditLog{}).
		Where("user_id = ? AND event_type = ? AND ip_address != ? AND created_at >= ?",
			userID, string(EventLogin), ipAddress, since).
		Count(&count)

	return count > 0
}

func (s *Service) logToStructuredLogger(auditLog *models.UserAuditLog, riskLevel RiskLevel) {
	fields := []zap.Field{
		zap.String("event_type", auditLog.EventType),
		zap.String("user_id", auditLog.UserID.String()),
		zap.String("ip_address", auditLog.IPAddress),
		zap.String("device_id", auditLog.DeviceID),
		zap.Float64("risk_score", auditLog.RiskScore),
		zap.String("details", auditLog.Details),
	}

	switch riskLevel {
	case RiskLevelCritical:
		s.logger.Error("Critical security event", fields...)
	case RiskLevelHigh:
		s.logger.Warn("High-risk security event", fields...)
	case RiskLevelMedium:
		s.logger.Info("Medium-risk security event", fields...)
	default:
		s.logger.Debug("Security event", fields...)
	}
}

func (s *Service) hasMultipleFailedLogins(events []models.UserAuditLog, since time.Time) bool {
	failedCount := 0
	for _, event := range events {
		if event.EventType == string(EventLoginFailed) && event.CreatedAt.After(since) {
			failedCount++
		}
	}
	return failedCount >= 3
}

func (s *Service) hasMultipleIPAddresses(events []models.UserAuditLog, since time.Time) bool {
	ips := make(map[string]bool)
	for _, event := range events {
		if event.EventType == string(EventLogin) && event.CreatedAt.After(since) {
			ips[event.IPAddress] = true
		}
	}
	return len(ips) > 2
}

func (s *Service) hasUnusualDeviceActivity(events []models.UserAuditLog, since time.Time) bool {
	devices := make(map[string]bool)
	for _, event := range events {
		if event.EventType == string(EventDeviceRegistered) && event.CreatedAt.After(since) {
			devices[event.DeviceID] = true
		}
	}
	return len(devices) > 3
}
