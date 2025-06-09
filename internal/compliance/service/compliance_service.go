package service

import (
	"context"
	"fmt"
	"time"

	"github.com/Aidin1998/finalex/internal/compliance/interfaces"
	"github.com/Aidin1998/finalex/internal/compliance/manipulation"
	userauthCompliance "github.com/Aidin1998/finalex/internal/userauth/compliance"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// ComplianceService implements the full compliance service interface
type ComplianceService struct {
	db                  *gorm.DB
	logger              *zap.Logger
	auditService        interfaces.AuditService
	userauthCompliance  *userauthCompliance.ComplianceService
	policyCache         map[string]*interfaces.PolicyUpdate
	complianceEventChan chan *interfaces.ComplianceEvent
	config              *Config
}

// Config holds compliance service configuration
type Config struct {
	EnableRealTimeMonitoring bool
	DefaultRiskLevel         interfaces.RiskLevel
	RequireKYCThreshold      decimal.Decimal
	AMLTransactionThreshold  decimal.Decimal
	PolicyRefreshInterval    time.Duration
	EventBufferSize          int
}

// NewComplianceService creates a new compliance service
func NewComplianceService(
	db *gorm.DB,
	logger *zap.Logger,
	auditService interfaces.AuditService,
	userauthCompliance *userauthCompliance.ComplianceService,
	config *Config,
) *ComplianceService {
	if config == nil {
		config = &Config{
			EnableRealTimeMonitoring: true,
			DefaultRiskLevel:         interfaces.RiskLevelMedium,
			RequireKYCThreshold:      decimal.NewFromInt(1000),
			AMLTransactionThreshold:  decimal.NewFromInt(10000),
			PolicyRefreshInterval:    time.Hour,
			EventBufferSize:          1000,
		}
	}

	service := &ComplianceService{
		db:                  db,
		logger:              logger,
		auditService:        auditService,
		userauthCompliance:  userauthCompliance,
		policyCache:         make(map[string]*interfaces.PolicyUpdate),
		complianceEventChan: make(chan *interfaces.ComplianceEvent, config.EventBufferSize),
		config:              config,
	}

	// Start background event processor
	go service.eventProcessor(context.Background())

	// Load existing policies
	if err := service.loadPolicies(context.Background()); err != nil {
		logger.Error("Failed to load policies during initialization", zap.Error(err))
	}

	return service
}

// ProcessComplianceRequest processes a compliance request
func (c *ComplianceService) ProcessComplianceRequest(ctx context.Context, req *interfaces.ComplianceRequest) (*interfaces.ComplianceResult, error) {
	startTime := time.Now()

	if err := c.validateRequest(req); err != nil {
		return nil, errors.Wrap(err, "invalid compliance request")
	} // Check user compliance status
	userStatus, err := c.GetUserStatus(ctx, req.UserID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get user compliance status")
	}

	// Handle transaction compliance if amount is provided
	result, err := c.validateTransactionCompliance(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "transaction compliance validation failed")
	}

	// Calculate risk score and level
	transactions, _ := c.getUserTransactions(ctx, req.UserID, 30)
	result.RiskScore = c.calculateRiskScore(userStatus, transactions)
	result.RiskLevel = c.determineRiskLevel(result.RiskScore)

	// Perform additional checks based on activity type
	switch req.ActivityType {
	case interfaces.ActivityDeposit, interfaces.ActivityWithdrawal:
		if err := c.performAMLChecks(ctx, req, result); err != nil {
			return nil, errors.Wrap(err, "AML checks failed")
		}
	case interfaces.ActivityTrade:
		if err := c.performTradingComplianceChecks(ctx, req, result); err != nil {
			return nil, errors.Wrap(err, "trading compliance checks failed")
		}
	}

	// Set final approval status
	c.setFinalApprovalStatus(result)

	// Log the compliance check
	c.logComplianceCheck(ctx, req, result)

	// Record processing time
	result.ProcessedAt = time.Now()

	c.logger.Info("Compliance check completed",
		zap.String("user_id", req.UserID.String()),
		zap.String("activity_type", req.ActivityType.String()),
		zap.Bool("approved", result.Approved),
		zap.String("risk_level", result.RiskLevel.String()),
		zap.Duration("processing_time", time.Since(startTime)))

	return result, nil
}

// PerformComplianceCheck performs a compliance check (alias for ProcessComplianceRequest)
func (c *ComplianceService) PerformComplianceCheck(ctx context.Context, req *interfaces.ComplianceRequest) (*interfaces.ComplianceResult, error) {
	return c.ProcessComplianceRequest(ctx, req)
}

// CheckCompliance performs a compliance check (alias for ProcessComplianceRequest)
func (c *ComplianceService) CheckCompliance(ctx context.Context, request *interfaces.ComplianceRequest) (*interfaces.ComplianceResult, error) {
	return c.ProcessComplianceRequest(ctx, request)
}

// CheckTransaction validates a transaction for compliance
func (c *ComplianceService) CheckTransaction(ctx context.Context, transactionID string) (*interfaces.ComplianceResult, error) {
	// Get transaction details from database
	var transaction TransactionRecord
	if err := c.db.Where("transaction_id = ?", transactionID).First(&transaction).Error; err != nil {
		return nil, errors.Wrap(err, "transaction not found")
	}

	// Create compliance request from transaction data
	req := &interfaces.ComplianceRequest{
		UserID:           transaction.UserID,
		ActivityType:     interfaces.ActivityTrade,
		Amount:           &transaction.Amount,
		Currency:         transaction.Currency,
		RequestTimestamp: time.Now(),
	}

	return c.CheckCompliance(ctx, req)
}

// ValidateTransaction validates a transaction for compliance
func (c *ComplianceService) ValidateTransaction(ctx context.Context, userID uuid.UUID, txType string, amount decimal.Decimal, currency string, metadata map[string]interface{}) (*interfaces.ComplianceResult, error) {
	var activityType interfaces.ActivityType
	switch txType {
	case "deposit":
		activityType = interfaces.ActivityDeposit
	case "withdrawal":
		activityType = interfaces.ActivityWithdrawal
	case "trade":
		activityType = interfaces.ActivityTrade
	case "transfer":
		activityType = interfaces.ActivityTransfer
	default:
		activityType = interfaces.ActivityTrade
	}

	req := &interfaces.ComplianceRequest{
		UserID:           userID,
		ActivityType:     activityType,
		Amount:           &amount,
		Currency:         currency,
		Metadata:         metadata,
		RequestTimestamp: time.Now(),
	}

	return c.CheckCompliance(ctx, req)
}

// GenerateComplianceReport generates a compliance report for a user
func (c *ComplianceService) GenerateComplianceReport(ctx context.Context, userID uuid.UUID, reportType string) (*manipulation.ComplianceReport, error) {
	transactions, err := c.getUserTransactions(ctx, userID, 90)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get transactions for report")
	}

	// Calculate summary statistics
	totalTransactionValue := decimal.Zero
	riskFlags := []string{}

	for _, tx := range transactions {
		totalTransactionValue = totalTransactionValue.Add(tx.Amount)
	}

	if totalTransactionValue.GreaterThan(decimal.NewFromInt(100000)) {
		riskFlags = append(riskFlags, "HIGH_TRANSACTION_VOLUME")
	}
	if len(transactions) > 100 {
		riskFlags = append(riskFlags, "HIGH_TRANSACTION_FREQUENCY")
	}

	return &manipulation.ComplianceReport{
		ID:          uuid.New().String(),
		Type:        reportType,
		Title:       fmt.Sprintf("Compliance Report for User %s", userID.String()),
		Status:      "generated",
		GeneratedAt: time.Now(),
		GeneratedBy: "system",
		UserIDs:     []string{userID.String()},
	}, nil
}

// GetUserStatus gets user compliance status
func (c *ComplianceService) GetUserStatus(ctx context.Context, userID uuid.UUID) (*interfaces.UserComplianceStatus, error) {
	var record UserComplianceRecord
	err := c.db.Where("user_id = ?", userID).First(&record).Error

	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errors.Wrap(err, "failed to query user compliance status")
	}

	// If no record exists, create default status
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &interfaces.UserComplianceStatus{
			UserID:         userID,
			Status:         interfaces.ComplianceStatusPending,
			RiskLevel:      c.config.DefaultRiskLevel,
			RiskScore:      decimal.NewFromInt(50),
			KYCLevel:       interfaces.KYCLevelNone,
			LastAssessment: time.Now(),
		}, nil
	}

	return &interfaces.UserComplianceStatus{
		UserID:         record.UserID,
		Status:         interfaces.ComplianceStatus(record.RiskLevel), // Mapping needed
		RiskLevel:      interfaces.RiskLevel(record.RiskLevel),
		RiskScore:      record.RiskScore,
		KYCLevel:       interfaces.KYCLevel(record.KYCLevel),
		LastAssessment: record.UpdatedAt,
		Restrictions:   record.Restrictions,
	}, nil
}

// GetUserHistory gets user compliance history
func (c *ComplianceService) GetUserHistory(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*interfaces.ComplianceEvent, error) {
	var records []ComplianceEventRecord
	err := c.db.Where("user_id = ?", userID).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&records).Error

	if err != nil {
		return nil, errors.Wrap(err, "failed to query compliance events")
	}
	events := make([]*interfaces.ComplianceEvent, len(records))
	for i, record := range records {
		events[i] = &interfaces.ComplianceEvent{
			EventType: record.EventType,
			UserID:    record.UserID.String(),
			Timestamp: record.CreatedAt,
			Details:   record.Metadata,
		}
	}

	return events, nil
}

// AssessUserRisk assesses user risk
func (c *ComplianceService) AssessUserRisk(ctx context.Context, userID uuid.UUID) (*interfaces.ComplianceResult, error) {
	userStatus, err := c.GetUserStatus(ctx, userID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get user status for risk assessment")
	}

	transactions, err := c.getUserTransactions(ctx, userID, 30)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get transactions for risk assessment")
	}

	riskScore := c.calculateRiskScore(userStatus, transactions)
	riskLevel := c.determineRiskLevel(riskScore)

	return &interfaces.ComplianceResult{
		RequestID:   uuid.New(),
		UserID:      userID,
		Status:      interfaces.ComplianceStatusApproved,
		RiskLevel:   riskLevel,
		RiskScore:   riskScore,
		Approved:    riskLevel != interfaces.RiskLevelCritical,
		ProcessedAt: time.Now(),
	}, nil
}

// UpdateRiskProfile updates user risk profile
func (c *ComplianceService) UpdateRiskProfile(ctx context.Context, userID uuid.UUID, riskLevel interfaces.RiskLevel, riskScore decimal.Decimal, reason string) error {
	record := UserComplianceRecord{
		UserID:    userID,
		RiskLevel: int(riskLevel),
		RiskScore: riskScore,
		UpdatedAt: time.Now(),
	}

	err := c.db.Save(&record).Error
	if err != nil {
		return errors.Wrap(err, "failed to update risk profile")
	}

	// Log the update
	c.auditService.LogEvent(ctx, &interfaces.AuditEvent{
		ID:          uuid.New(),
		UserID:      &userID,
		EventType:   "risk_profile_update",
		Category:    "compliance",
		Severity:    "info",
		Description: fmt.Sprintf("Risk profile updated: level=%s, score=%s, reason=%s", riskLevel.String(), riskScore.String(), reason),
		Timestamp:   time.Now(),
	})

	return nil
}

// InitiateKYC initiates KYC process
func (c *ComplianceService) InitiateKYC(ctx context.Context, userID uuid.UUID, targetLevel interfaces.KYCLevel) error {
	kycRecord := KYCRecord{
		UserID:      userID,
		KYCLevel:    int(targetLevel),
		Status:      "initiated",
		InitiatedAt: time.Now(),
	}

	err := c.db.Create(&kycRecord).Error
	if err != nil {
		return errors.Wrap(err, "failed to create KYC record")
	}

	c.logger.Info("KYC process initiated",
		zap.String("user_id", userID.String()),
		zap.String("target_level", targetLevel.String()))

	return nil
}

// ValidateKYCLevel validates KYC level
func (c *ComplianceService) ValidateKYCLevel(ctx context.Context, userID uuid.UUID, requiredLevel interfaces.KYCLevel) error {
	currentLevel, err := c.GetKYCStatus(ctx, userID)
	if err != nil {
		return errors.Wrap(err, "failed to get current KYC status")
	}

	if currentLevel < requiredLevel {
		return fmt.Errorf("insufficient KYC level: current=%s, required=%s", currentLevel.String(), requiredLevel.String())
	}

	return nil
}

// GetKYCStatus gets KYC status
func (c *ComplianceService) GetKYCStatus(ctx context.Context, userID uuid.UUID) (interfaces.KYCLevel, error) {
	var record KYCRecord
	err := c.db.Where("user_id = ? AND status = ?", userID, "completed").
		Order("kyc_level DESC").
		First(&record).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return interfaces.KYCLevelNone, nil
		}
		return interfaces.KYCLevelNone, errors.Wrap(err, "failed to query KYC status")
	}

	return interfaces.KYCLevel(record.KYCLevel), nil
}

// PerformAMLCheck performs AML check
func (c *ComplianceService) PerformAMLCheck(ctx context.Context, userID uuid.UUID, amount decimal.Decimal, currency string) (*interfaces.ComplianceResult, error) {
	result := &interfaces.ComplianceResult{
		RequestID: uuid.New(),
		UserID:    userID,
		Status:    interfaces.ComplianceStatusApproved,
		Approved:  true,
	}

	// Check against AML thresholds
	if amount.GreaterThan(c.config.AMLTransactionThreshold) {
		result.Flags = append(result.Flags, "HIGH_VALUE_TRANSACTION")
		result.RequiresReview = true
		result.Status = interfaces.ComplianceStatusUnderReview
	}

	// Additional AML checks would be implemented here
	// For now, this is a basic implementation

	result.ProcessedAt = time.Now()
	return result, nil
}

// CheckSanctionsList checks against sanctions list
func (c *ComplianceService) CheckSanctionsList(ctx context.Context, name, dateOfBirth, nationality string) (*interfaces.ComplianceResult, error) {
	result := &interfaces.ComplianceResult{
		RequestID: uuid.New(),
		Status:    interfaces.ComplianceStatusApproved,
		Approved:  true,
	}

	// This would integrate with actual sanctions list services
	// For now, this is a placeholder implementation

	result.ProcessedAt = time.Now()
	return result, nil
}

// UpdatePolicies updates compliance policies
func (c *ComplianceService) UpdatePolicies(ctx context.Context, updates []*interfaces.PolicyUpdate) error {
	for _, update := range updates {
		c.policyCache[update.PolicyType] = update

		// Store in database
		record := PolicyRecord{
			PolicyID:    update.ID.String(),
			PolicyType:  update.PolicyType,
			PolicyData:  update.Rules,
			Version:     update.Version,
			EffectiveAt: update.EffectiveAt,
			UpdatedAt:   time.Now(),
		}

		if err := c.db.Save(&record).Error; err != nil {
			return errors.Wrap(err, "failed to save policy update")
		}
	}

	c.logger.Info("Policies updated", zap.Int("count", len(updates)))
	return nil
}

// GetActivePolicies gets active policies
func (c *ComplianceService) GetActivePolicies(ctx context.Context, policyType string) ([]*interfaces.PolicyUpdate, error) {
	var records []PolicyRecord
	query := c.db.Where("effective_at <= ?", time.Now())

	if policyType != "" {
		query = query.Where("policy_type = ?", policyType)
	}

	err := query.Find(&records).Error
	if err != nil {
		return nil, errors.Wrap(err, "failed to query policies")
	}

	policies := make([]*interfaces.PolicyUpdate, len(records))
	for i, record := range records {
		policyID, _ := uuid.Parse(record.PolicyID)
		policies[i] = &interfaces.PolicyUpdate{
			ID:          policyID,
			PolicyType:  record.PolicyType,
			Version:     record.Version,
			Rules:       record.PolicyData,
			EffectiveAt: record.EffectiveAt,
			UpdatedAt:   record.UpdatedAt,
		}
	}

	return policies, nil
}

// ProcessExternalReport processes external compliance report
func (c *ComplianceService) ProcessExternalReport(ctx context.Context, report *interfaces.ExternalComplianceReport) error {
	// Store the external report
	record := ExternalReportRecord{
		ReportID:    report.ID.String(),
		Source:      report.Source,
		ReportType:  report.ReportType,
		ReportData:  report.Data,
		ProcessedAt: time.Now(),
	}

	err := c.db.Create(&record).Error
	if err != nil {
		return errors.Wrap(err, "failed to store external report")
	}

	c.logger.Info("External report processed",
		zap.String("report_id", report.ID.String()),
		zap.String("source", report.Source),
		zap.String("type", report.ReportType))

	return nil
}

// Start starts the compliance service
func (c *ComplianceService) Start(ctx context.Context) error {
	c.logger.Info("Starting compliance service")

	// Ensure database tables exist
	err := c.db.AutoMigrate(
		&UserComplianceRecord{},
		&ComplianceEventRecord{},
		&TransactionRecord{},
		&KYCRecord{},
		&PolicyRecord{},
		&ExternalReportRecord{},
	)
	if err != nil {
		return errors.Wrap(err, "failed to migrate database tables")
	}

	// Load policies
	if err := c.loadPolicies(ctx); err != nil {
		c.logger.Error("Failed to load policies", zap.Error(err))
	}

	c.logger.Info("Compliance service started successfully")
	return nil
}

// Stop stops the compliance service
func (c *ComplianceService) Stop(ctx context.Context) error {
	c.logger.Info("Stopping compliance service")

	// Close any channels or cleanup resources
	close(c.complianceEventChan)

	c.logger.Info("Compliance service stopped")
	return nil
}

// HealthCheck performs health check
func (c *ComplianceService) HealthCheck(ctx context.Context) error {
	// Check database connection
	sqlDB, err := c.db.DB()
	if err != nil {
		return errors.Wrap(err, "failed to get database connection")
	}

	if err := sqlDB.PingContext(ctx); err != nil {
		return errors.Wrap(err, "database ping failed")
	}

	// Check audit service
	if err := c.auditService.HealthCheck(ctx); err != nil {
		return errors.Wrap(err, "audit service health check failed")
	}

	return nil
}

// Helper methods

func (c *ComplianceService) validateRequest(req *interfaces.ComplianceRequest) error {
	if req.UserID == uuid.Nil {
		return fmt.Errorf("user ID is required")
	}
	return nil
}

func (c *ComplianceService) validateTransactionCompliance(ctx context.Context, req *interfaces.ComplianceRequest) (*interfaces.ComplianceResult, error) {
	result := &interfaces.ComplianceResult{
		RequestID: uuid.New(),
		UserID:    req.UserID,
		Status:    interfaces.ComplianceStatusApproved,
		Approved:  true,
	}

	if req.Amount != nil && req.Amount.GreaterThan(c.config.RequireKYCThreshold) {
		result.KYCRequired = true
		result.RequiredKYCLevel = interfaces.KYCLevelBasic
	}

	return result, nil
}

func (c *ComplianceService) getUserTransactions(ctx context.Context, userID uuid.UUID, days int) ([]TransactionRecord, error) {
	var transactions []TransactionRecord
	since := time.Now().AddDate(0, 0, -days)

	err := c.db.Where("user_id = ? AND created_at >= ?", userID, since).
		Find(&transactions).Error

	return transactions, err
}

func (c *ComplianceService) calculateRiskScore(userStatus *interfaces.UserComplianceStatus, transactions []TransactionRecord) decimal.Decimal {
	baseScore := decimal.NewFromInt(25) // Start with low risk

	// Increase score based on transaction volume
	if len(transactions) > 50 {
		baseScore = baseScore.Add(decimal.NewFromInt(10))
	}

	// Calculate total transaction volume
	totalVolume := decimal.Zero
	for _, tx := range transactions {
		totalVolume = totalVolume.Add(tx.Amount)
	}

	if totalVolume.GreaterThan(decimal.NewFromInt(50000)) {
		baseScore = baseScore.Add(decimal.NewFromInt(15))
	}

	// Ensure score is within bounds
	if baseScore.GreaterThan(decimal.NewFromInt(100)) {
		return decimal.NewFromInt(100)
	}
	if baseScore.LessThan(decimal.Zero) {
		return decimal.Zero
	}

	return baseScore
}

func (c *ComplianceService) determineRiskLevel(riskScore decimal.Decimal) interfaces.RiskLevel {
	if riskScore.LessThan(decimal.NewFromInt(25)) {
		return interfaces.RiskLevelLow
	} else if riskScore.LessThan(decimal.NewFromInt(50)) {
		return interfaces.RiskLevelMedium
	} else if riskScore.LessThan(decimal.NewFromInt(75)) {
		return interfaces.RiskLevelHigh
	}
	return interfaces.RiskLevelCritical
}

func (c *ComplianceService) performAMLChecks(ctx context.Context, req *interfaces.ComplianceRequest, result *interfaces.ComplianceResult) error {
	if req.Amount != nil && req.Amount.GreaterThan(c.config.AMLTransactionThreshold) {
		result.Flags = append(result.Flags, "HIGH_VALUE_TRANSACTION")
		result.RequiresReview = true
	}
	return nil
}

func (c *ComplianceService) performTradingComplianceChecks(ctx context.Context, req *interfaces.ComplianceRequest, result *interfaces.ComplianceResult) error {
	// Basic trading compliance checks
	// This would be expanded with actual trading rules
	return nil
}

func (c *ComplianceService) setFinalApprovalStatus(result *interfaces.ComplianceResult) {
	if result.RiskLevel == interfaces.RiskLevelCritical {
		result.Approved = false
		result.Blocked = true
		result.Status = interfaces.ComplianceStatusBlocked
	} else if result.RequiresReview {
		result.Status = interfaces.ComplianceStatusUnderReview
	} else {
		result.Status = interfaces.ComplianceStatusApproved
	}
}

func (c *ComplianceService) logComplianceCheck(ctx context.Context, req *interfaces.ComplianceRequest, result *interfaces.ComplianceResult) {
	event := &interfaces.ComplianceEvent{
		EventType: "compliance_check",
		UserID:    req.UserID.String(),
		Timestamp: time.Now(),
		Details: map[string]interface{}{
			"activity_type": req.ActivityType.String(),
			"result":        result.Status.String(),
			"risk_level":    result.RiskLevel.String(),
			"approved":      result.Approved,
		},
	}

	select {
	case c.complianceEventChan <- event:
	default:
		c.logger.Warn("Compliance event channel full, dropping event")
	}
}

func (c *ComplianceService) eventProcessor(ctx context.Context) {
	for {
		select {
		case event := <-c.complianceEventChan:
			if event != nil {
				// Store event in database
				record := ComplianceEventRecord{
					UserID:    uuid.MustParse(event.UserID),
					EventType: event.EventType,
					Metadata:  event.Details,
					CreatedAt: event.Timestamp,
				}
				if err := c.db.Create(&record).Error; err != nil {
					c.logger.Error("Failed to store compliance event", zap.Error(err))
				}
			}
		case <-ctx.Done():
			return
		}
	}
}

func (c *ComplianceService) loadPolicies(ctx context.Context) error {
	policies, err := c.GetActivePolicies(ctx, "")
	if err != nil {
		return err
	}

	for _, policy := range policies {
		c.policyCache[policy.PolicyType] = policy
	}

	c.logger.Info("Loaded policies", zap.Int("count", len(policies)))
	return nil
}
