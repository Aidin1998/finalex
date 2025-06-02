package audit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// AuditService provides comprehensive audit logging for administrative actions
type AuditService struct {
	db            *gorm.DB
	logger        *zap.Logger
	encryptionSvc EncryptionService
	config        *AuditConfig
	eventQueue    chan *AuditEvent
	batchBuffer   []*AuditEvent
	done          chan struct{}
}

// AuditConfig defines audit logging configuration
type AuditConfig struct {
	EnableEncryption         bool          `json:"enable_encryption"`
	TamperProofStorage       bool          `json:"tamper_proof_storage"`
	AsyncLogging             bool          `json:"async_logging"`
	BatchSize                int           `json:"batch_size"`
	FlushInterval            time.Duration `json:"flush_interval"`
	RetentionPeriod          time.Duration `json:"retention_period"`
	ComplianceMode           bool          `json:"compliance_mode"`
	DetailLevel              DetailLevel   `json:"detail_level"`
	EnableForensicCapability bool          `json:"enable_forensic_capability"`
}

// DetailLevel defines the level of detail to capture in audit logs
type DetailLevel string

const (
	DetailLevelMinimal  DetailLevel = "minimal"
	DetailLevelStandard DetailLevel = "standard"
	DetailLevelVerbose  DetailLevel = "verbose"
	DetailLevelForensic DetailLevel = "forensic"
)

// AuditEvent represents a single audit event
type AuditEvent struct {
	ID           uuid.UUID     `json:"id" gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	EventType    EventType     `json:"event_type" gorm:"not null;index"`
	Category     EventCategory `json:"category" gorm:"not null;index"`
	Severity     SeverityLevel `json:"severity" gorm:"not null;index"`
	ActorID      uuid.UUID     `json:"actor_id" gorm:"type:uuid;index"`
	ActorType    ActorType     `json:"actor_type" gorm:"not null"`
	ActorRole    string        `json:"actor_role" gorm:"not null"`
	TargetID     *uuid.UUID    `json:"target_id,omitempty" gorm:"type:uuid;index"`
	TargetType   *string       `json:"target_type,omitempty"`
	ResourceID   *string       `json:"resource_id,omitempty" gorm:"index"`
	ResourceType *string       `json:"resource_type,omitempty"`
	Action       string        `json:"action" gorm:"not null;index"`
	Outcome      EventOutcome  `json:"outcome" gorm:"not null;index"`
	Timestamp    time.Time     `json:"timestamp" gorm:"not null;index"`
	SessionID    *uuid.UUID    `json:"session_id,omitempty" gorm:"type:uuid;index"`
	ClientIP     string        `json:"client_ip" gorm:"not null;index"`
	UserAgent    string        `json:"user_agent"`
	RequestID    *string       `json:"request_id,omitempty" gorm:"index"`
	Geolocation  *string       `json:"geolocation,omitempty"`

	// Details and context
	Description string                 `json:"description" gorm:"not null"`
	Details     map[string]interface{} `json:"details" gorm:"type:jsonb"`
	BeforeState map[string]interface{} `json:"before_state,omitempty" gorm:"type:jsonb"`
	AfterState  map[string]interface{} `json:"after_state,omitempty" gorm:"type:jsonb"`
	Changes     map[string]interface{} `json:"changes,omitempty" gorm:"type:jsonb"`
	Metadata    map[string]interface{} `json:"metadata,omitempty" gorm:"type:jsonb"`

	// Compliance and risk
	ComplianceFlags []string        `json:"compliance_flags" gorm:"type:jsonb"`
	RiskScore       *float64        `json:"risk_score,omitempty"`
	BusinessImpact  *BusinessImpact `json:"business_impact,omitempty" gorm:"type:jsonb"`

	// Tamper-proof mechanisms
	Hash          string  `json:"hash" gorm:"not null;index"`
	PreviousHash  *string `json:"previous_hash,omitempty"`
	EncryptedData *string `json:"encrypted_data,omitempty" gorm:"type:text"`

	// Forensic data
	ForensicData *ForensicData `json:"forensic_data,omitempty" gorm:"type:jsonb"`

	CreatedAt      time.Time `json:"created_at" gorm:"not null"`
	RetentionUntil time.Time `json:"retention_until" gorm:"not null;index"`
}

// EventType defines types of audit events
type EventType string

const (
	// User Management Events
	EventUserCreated           EventType = "user.created"
	EventUserUpdated           EventType = "user.updated"
	EventUserDeleted           EventType = "user.deleted"
	EventUserSuspended         EventType = "user.suspended"
	EventUserReactivated       EventType = "user.reactivated"
	EventUserRoleChanged       EventType = "user.role_changed"
	EventUserPermissionChanged EventType = "user.permission_changed"
	EventUserKYCUpdated        EventType = "user.kyc_updated"
	EventUserSessionTerminated EventType = "user.session_terminated"

	// Authentication Events
	EventAuthLogin          EventType = "auth.login"
	EventAuthLogout         EventType = "auth.logout"
	EventAuthFailedLogin    EventType = "auth.failed_login"
	EventAuthPasswordChange EventType = "auth.password_change"
	EventAuthMFAEnabled     EventType = "auth.mfa_enabled"
	EventAuthMFADisabled    EventType = "auth.mfa_disabled"
	EventAuthAPIKeyCreated  EventType = "auth.api_key_created"
	EventAuthAPIKeyRevoked  EventType = "auth.api_key_revoked"

	// Financial Operations
	EventTransactionCreated   EventType = "transaction.created"
	EventTransactionCancelled EventType = "transaction.cancelled"
	EventTransactionBlocked   EventType = "transaction.blocked"
	EventDepositProcessed     EventType = "deposit.processed"
	EventWithdrawalProcessed  EventType = "withdrawal.processed"
	EventWalletFrozen         EventType = "wallet.frozen"
	EventWalletUnfrozen       EventType = "wallet.unfrozen"

	// Risk Management Events
	EventRiskLimitCreated     EventType = "risk.limit_created"
	EventRiskLimitUpdated     EventType = "risk.limit_updated"
	EventRiskLimitDeleted     EventType = "risk.limit_deleted"
	EventRiskExemptionCreated EventType = "risk.exemption_created"
	EventRiskExemptionDeleted EventType = "risk.exemption_deleted"
	EventRiskAlertTriggered   EventType = "risk.alert_triggered"
	EventRiskRuleUpdated      EventType = "risk.rule_updated"

	// System Administration
	EventSystemConfigChanged   EventType = "system.config_changed"
	EventSystemMaintenanceMode EventType = "system.maintenance_mode"
	EventSystemShutdown        EventType = "system.shutdown"
	EventSystemStartup         EventType = "system.startup"
	EventRateLimitUpdated      EventType = "system.rate_limit_updated"
	EventEmergencyMode         EventType = "system.emergency_mode"

	// Compliance Events
	EventComplianceAlertCreated   EventType = "compliance.alert_created"
	EventComplianceRuleAdded      EventType = "compliance.rule_added"
	EventComplianceSARGenerated   EventType = "compliance.sar_generated"
	EventComplianceAuditTriggered EventType = "compliance.audit_triggered"

	// Data Access Events
	EventDataViewed   EventType = "data.viewed"
	EventDataExported EventType = "data.exported"
	EventDataDeleted  EventType = "data.deleted"
	EventDataModified EventType = "data.modified"
)

// EventCategory groups related events
type EventCategory string

const (
	CategoryUserManagement EventCategory = "user_management"
	CategoryAuthentication EventCategory = "authentication"
	CategoryFinancial      EventCategory = "financial"
	CategoryRiskManagement EventCategory = "risk_management"
	CategorySystemAdmin    EventCategory = "system_admin"
	CategoryCompliance     EventCategory = "compliance"
	CategoryDataAccess     EventCategory = "data_access"
	CategorySecurity       EventCategory = "security"
)

// SeverityLevel defines the severity of an audit event
type SeverityLevel string

const (
	SeverityInfo     SeverityLevel = "info"
	SeverityWarning  SeverityLevel = "warning"
	SeverityError    SeverityLevel = "error"
	SeverityCritical SeverityLevel = "critical"
)

// ActorType defines who or what performed the action
type ActorType string

const (
	ActorTypeUser   ActorType = "user"
	ActorTypeAdmin  ActorType = "admin"
	ActorTypeSystem ActorType = "system"
	ActorTypeAPI    ActorType = "api"
)

// EventOutcome defines the result of an action
type EventOutcome string

const (
	OutcomeSuccess EventOutcome = "success"
	OutcomeFailure EventOutcome = "failure"
	OutcomePending EventOutcome = "pending"
	OutcomePartial EventOutcome = "partial"
)

// BusinessImpact describes the business impact of an event
type BusinessImpact struct {
	Level         string   `json:"level"` // low, medium, high, critical
	Description   string   `json:"description"`
	Categories    []string `json:"categories"` // financial, operational, compliance, security
	EstimatedCost *float64 `json:"estimated_cost,omitempty"`
}

// ForensicData contains detailed forensic information
type ForensicData struct {
	HTTPHeaders     map[string]string `json:"http_headers,omitempty"`
	RequestBody     interface{}       `json:"request_body,omitempty"`
	ResponseStatus  *int              `json:"response_status,omitempty"`
	ResponseBody    interface{}       `json:"response_body,omitempty"`
	DatabaseQueries []string          `json:"database_queries,omitempty"`
	StackTrace      *string           `json:"stack_trace,omitempty"`
	Environment     map[string]string `json:"environment,omitempty"`
	ProcessID       *int              `json:"process_id,omitempty"`
	ThreadID        *string           `json:"thread_id,omitempty"`
}

// EncryptionService interface for encrypting sensitive audit data
type EncryptionService interface {
	Encrypt(data []byte) ([]byte, error)
	Decrypt(encryptedData []byte) ([]byte, error)
}

// NewAuditService creates a new audit service
func NewAuditService(db *gorm.DB, logger *zap.Logger, encryptionSvc EncryptionService, config *AuditConfig) *AuditService {
	if config == nil {
		config = DefaultAuditConfig()
	}

	service := &AuditService{
		db:            db,
		logger:        logger,
		encryptionSvc: encryptionSvc,
		config:        config,
		done:          make(chan struct{}),
	}

	if config.AsyncLogging {
		service.eventQueue = make(chan *AuditEvent, config.BatchSize*2)
		service.batchBuffer = make([]*AuditEvent, 0, config.BatchSize)
		go service.processBatch()
	}

	return service
}

// DefaultAuditConfig returns default audit configuration
func DefaultAuditConfig() *AuditConfig {
	return &AuditConfig{
		EnableEncryption:         true,
		TamperProofStorage:       true,
		AsyncLogging:             true,
		BatchSize:                100,
		FlushInterval:            time.Second * 30,
		RetentionPeriod:          time.Hour * 24 * 365 * 7, // 7 years
		ComplianceMode:           true,
		DetailLevel:              DetailLevelStandard,
		EnableForensicCapability: true,
	}
}

// LogEvent logs an audit event
func (as *AuditService) LogEvent(ctx context.Context, event *AuditEvent) error {
	// Set default values
	if event.ID == uuid.Nil {
		event.ID = uuid.New()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	event.CreatedAt = time.Now()
	event.RetentionUntil = event.CreatedAt.Add(as.config.RetentionPeriod)

	// Calculate hash for tamper-proof storage
	if as.config.TamperProofStorage {
		hash, err := as.calculateEventHash(event)
		if err != nil {
			return fmt.Errorf("failed to calculate event hash: %w", err)
		}
		event.Hash = hash

		// Set previous hash for blockchain-like integrity
		prevHash, err := as.getLastEventHash()
		if err != nil {
			as.logger.Warn("Failed to get previous hash", zap.Error(err))
		} else if prevHash != "" {
			event.PreviousHash = &prevHash
		}
	}

	// Encrypt sensitive data if enabled
	if as.config.EnableEncryption && as.encryptionSvc != nil {
		if err := as.encryptSensitiveData(event); err != nil {
			as.logger.Error("Failed to encrypt audit data", zap.Error(err))
		}
	}

	// Add forensic data if enabled and detail level is sufficient
	if as.config.EnableForensicCapability && as.shouldIncludeForensicData(event) {
		as.addForensicData(ctx, event)
	}

	// Process event based on configuration
	if as.config.AsyncLogging {
		select {
		case as.eventQueue <- event:
			return nil
		default:
			// Queue is full, log synchronously as fallback
			as.logger.Warn("Audit queue full, falling back to synchronous logging")
			return as.storeEvent(event)
		}
	}

	return as.storeEvent(event)
}

// calculateEventHash calculates a hash for tamper-proof storage
func (as *AuditService) calculateEventHash(event *AuditEvent) (string, error) {
	// Create a copy without hash fields for hashing
	hashData := struct {
		ID           uuid.UUID
		EventType    EventType
		ActorID      uuid.UUID
		Action       string
		Timestamp    time.Time
		Description  string
		PreviousHash *string
	}{
		ID:           event.ID,
		EventType:    event.EventType,
		ActorID:      event.ActorID,
		Action:       event.Action,
		Timestamp:    event.Timestamp,
		Description:  event.Description,
		PreviousHash: event.PreviousHash,
	}

	data, err := json.Marshal(hashData)
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

// getLastEventHash gets the hash of the last audit event
func (as *AuditService) getLastEventHash() (string, error) {
	var lastEvent AuditEvent
	err := as.db.Order("created_at DESC").First(&lastEvent).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", nil
		}
		return "", err
	}
	return lastEvent.Hash, nil
}

// encryptSensitiveData encrypts sensitive fields in the audit event
func (as *AuditService) encryptSensitiveData(event *AuditEvent) error {
	sensitiveData := map[string]interface{}{
		"details":       event.Details,
		"before_state":  event.BeforeState,
		"after_state":   event.AfterState,
		"changes":       event.Changes,
		"metadata":      event.Metadata,
		"forensic_data": event.ForensicData,
	}

	data, err := json.Marshal(sensitiveData)
	if err != nil {
		return err
	}

	encryptedData, err := as.encryptionSvc.Encrypt(data)
	if err != nil {
		return err
	}

	event.EncryptedData = &string(encryptedData)

	// Clear original data
	event.Details = nil
	event.BeforeState = nil
	event.AfterState = nil
	event.Changes = nil
	event.Metadata = nil
	event.ForensicData = nil

	return nil
}

// shouldIncludeForensicData determines if forensic data should be included
func (as *AuditService) shouldIncludeForensicData(event *AuditEvent) bool {
	if as.config.DetailLevel == DetailLevelForensic {
		return true
	}

	// Include forensic data for high-severity events
	if event.Severity == SeverityCritical || event.Severity == SeverityError {
		return true
	}

	// Include for security-related events
	if event.Category == CategorySecurity || event.Category == CategoryCompliance {
		return true
	}

	return false
}

// addForensicData adds forensic information to the event
func (as *AuditService) addForensicData(ctx context.Context, event *AuditEvent) {
	forensicData := &ForensicData{
		Environment: make(map[string]string),
	}

	// Add context data if available
	if requestID := ctx.Value("request_id"); requestID != nil {
		if rid, ok := requestID.(string); ok {
			event.RequestID = &rid
		}
	}

	// Add environment information
	// This would be populated based on the application context

	event.ForensicData = forensicData
}

// storeEvent stores an audit event in the database
func (as *AuditService) storeEvent(event *AuditEvent) error {
	if err := as.db.Create(event).Error; err != nil {
		as.logger.Error("Failed to store audit event",
			zap.String("event_id", event.ID.String()),
			zap.String("event_type", string(event.EventType)),
			zap.Error(err))
		return err
	}

	as.logger.Debug("Audit event stored",
		zap.String("event_id", event.ID.String()),
		zap.String("event_type", string(event.EventType)),
		zap.String("actor_id", event.ActorID.String()))

	return nil
}

// processBatch processes events in batches for async logging
func (as *AuditService) processBatch() {
	ticker := time.NewTicker(as.config.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case event := <-as.eventQueue:
			as.batchBuffer = append(as.batchBuffer, event)
			if len(as.batchBuffer) >= as.config.BatchSize {
				as.flushBatch()
			}

		case <-ticker.C:
			if len(as.batchBuffer) > 0 {
				as.flushBatch()
			}

		case <-as.done:
			// Flush remaining events
			if len(as.batchBuffer) > 0 {
				as.flushBatch()
			}
			return
		}
	}
}

// flushBatch flushes the current batch of events to the database
func (as *AuditService) flushBatch() {
	if len(as.batchBuffer) == 0 {
		return
	}

	tx := as.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	for _, event := range as.batchBuffer {
		if err := tx.Create(event).Error; err != nil {
			as.logger.Error("Failed to store audit event in batch",
				zap.String("event_id", event.ID.String()),
				zap.Error(err))
		}
	}

	if err := tx.Commit().Error; err != nil {
		as.logger.Error("Failed to commit audit batch", zap.Error(err))
		return
	}

	as.logger.Debug("Audit batch committed",
		zap.Int("batch_size", len(as.batchBuffer)))

	// Clear batch buffer
	as.batchBuffer = as.batchBuffer[:0]
}

// Close gracefully shuts down the audit service
func (as *AuditService) Close() {
	if as.config.AsyncLogging {
		close(as.done)
	}
}

// Verify integrity checks the integrity of stored audit events
func (as *AuditService) VerifyIntegrity(ctx context.Context, startTime, endTime time.Time) (*IntegrityReport, error) {
	var events []AuditEvent
	err := as.db.Where("created_at BETWEEN ? AND ?", startTime, endTime).
		Order("created_at ASC").Find(&events).Error
	if err != nil {
		return nil, fmt.Errorf("failed to query audit events: %w", err)
	}

	report := &IntegrityReport{
		StartTime:     startTime,
		EndTime:       endTime,
		TotalEvents:   len(events),
		ValidEvents:   0,
		InvalidEvents: 0,
		Issues:        make([]IntegrityIssue, 0),
	}

	var prevHash string
	for i, event := range events {
		// Verify event hash
		expectedHash, err := as.calculateEventHash(&event)
		if err != nil {
			report.Issues = append(report.Issues, IntegrityIssue{
				EventID:     event.ID,
				IssueType:   "hash_calculation_error",
				Description: fmt.Sprintf("Failed to calculate hash: %v", err),
				Severity:    "error",
			})
			report.InvalidEvents++
			continue
		}

		if event.Hash != expectedHash {
			report.Issues = append(report.Issues, IntegrityIssue{
				EventID:     event.ID,
				IssueType:   "hash_mismatch",
				Description: "Event hash does not match calculated hash",
				Severity:    "critical",
			})
			report.InvalidEvents++
			continue
		}

		// Verify chain integrity
		if i > 0 && event.PreviousHash != nil && *event.PreviousHash != prevHash {
			report.Issues = append(report.Issues, IntegrityIssue{
				EventID:     event.ID,
				IssueType:   "chain_break",
				Description: "Previous hash does not match expected value",
				Severity:    "critical",
			})
			report.InvalidEvents++
			continue
		}

		report.ValidEvents++
		prevHash = event.Hash
	}

	return report, nil
}

// IntegrityReport contains the results of an integrity check
type IntegrityReport struct {
	StartTime     time.Time        `json:"start_time"`
	EndTime       time.Time        `json:"end_time"`
	TotalEvents   int              `json:"total_events"`
	ValidEvents   int              `json:"valid_events"`
	InvalidEvents int              `json:"invalid_events"`
	Issues        []IntegrityIssue `json:"issues"`
	GeneratedAt   time.Time        `json:"generated_at"`
}

// IntegrityIssue represents an integrity violation
type IntegrityIssue struct {
	EventID     uuid.UUID `json:"event_id"`
	IssueType   string    `json:"issue_type"`
	Description string    `json:"description"`
	Severity    string    `json:"severity"`
	DetectedAt  time.Time `json:"detected_at"`
}
