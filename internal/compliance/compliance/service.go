package compliance

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/Finalex/internal/compliance/interfaces"
	"github.com/google/uuid"
)

// ComplianceService implements the main compliance orchestration service
type ComplianceService struct {
	db           *sql.DB
	auditService interfaces.AuditService
	policies     *PolicyManager
	sanctions    *SanctionsChecker
	kyc          *KYCProcessor
	aml          *AMLProcessor
	metrics      *ComplianceMetrics
	mu           sync.RWMutex
}

// PolicyManager handles dynamic policy updates and rule management
type PolicyManager struct {
	policies map[string]*Policy
	mu       sync.RWMutex
	version  string
	updated  time.Time
}

// Policy represents a compliance policy with dynamic rules
type Policy struct {
	ID         string                 `json:"id"`
	Name       string                 `json:"name"`
	Type       string                 `json:"type"` // "kyc", "aml", "sanctions", "risk"
	Version    string                 `json:"version"`
	Rules      []Rule                 `json:"rules"`
	Thresholds map[string]interface{} `json:"thresholds"`
	Active     bool                   `json:"active"`
	Priority   int                    `json:"priority"`
	CreatedAt  time.Time              `json:"created_at"`
	UpdatedAt  time.Time              `json:"updated_at"`
	ExpiresAt  *time.Time             `json:"expires_at,omitempty"`
}

// Rule represents a specific compliance rule
type Rule struct {
	ID         string                 `json:"id"`
	Name       string                 `json:"name"`
	Condition  string                 `json:"condition"` // SQL-like condition
	Action     string                 `json:"action"`    // "block", "flag", "monitor", "escalate"
	Severity   interfaces.RiskLevel   `json:"severity"`
	Parameters map[string]interface{} `json:"parameters"`
	Active     bool                   `json:"active"`
	Weight     float64                `json:"weight"`
}

// SanctionsChecker handles sanctions list screening
type SanctionsChecker struct {
	lists    map[string]*SanctionsList
	mu       sync.RWMutex
	lastSync time.Time
}

// SanctionsList represents a sanctions list
type SanctionsList struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Source   string    `json:"source"`
	Entries  []string  `json:"entries"`
	Updated  time.Time `json:"updated"`
	Priority int       `json:"priority"`
}

// KYCProcessor handles KYC verification logic
type KYCProcessor struct {
	providers map[string]KYCProvider
	config    *KYCConfig
	mu        sync.RWMutex
}

// KYCProvider interface for KYC service providers
type KYCProvider interface {
	VerifyIdentity(ctx context.Context, req *interfaces.KYCRequest) (*interfaces.KYCResult, error)
	CheckDocument(ctx context.Context, docType, docData string) (*interfaces.DocumentVerification, error)
	GetStatus(ctx context.Context, userID string) (*interfaces.KYCStatus, error)
}

// KYCConfig holds KYC configuration
type KYCConfig struct {
	RequiredDocuments     []string `json:"required_documents"`
	MinAge                int      `json:"min_age"`
	RestrictedCountries   []string `json:"restricted_countries"`
	AutoApprovalLimit     float64  `json:"auto_approval_limit"`
	ManualReviewThreshold float64  `json:"manual_review_threshold"`
}

// AMLProcessor handles Anti-Money Laundering checks
type AMLProcessor struct {
	rules     []AMLRule
	patterns  map[string]*TransactionPattern
	mu        sync.RWMutex
	threshold float64
}

// AMLRule represents an AML screening rule
type AMLRule struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Pattern   string  `json:"pattern"`
	Threshold float64 `json:"threshold"`
	Action    string  `json:"action"`
	Active    bool    `json:"active"`
	Weight    float64 `json:"weight"`
}

// TransactionPattern represents suspicious transaction patterns
type TransactionPattern struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"`
	Description string    `json:"description"`
	Indicators  []string  `json:"indicators"`
	Score       float64   `json:"score"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ComplianceMetrics tracks compliance system performance
type ComplianceMetrics struct {
	TotalChecks      int64     `json:"total_checks"`
	PassedChecks     int64     `json:"passed_checks"`
	FailedChecks     int64     `json:"failed_checks"`
	FlaggedChecks    int64     `json:"flagged_checks"`
	AverageLatency   float64   `json:"average_latency"`
	LastUpdated      time.Time `json:"last_updated"`
	PolicyUpdates    int64     `json:"policy_updates"`
	SanctionsMatches int64     `json:"sanctions_matches"`
	KYCApprovals     int64     `json:"kyc_approvals"`
	KYCRejections    int64     `json:"kyc_rejections"`
	AMLAlerts        int64     `json:"aml_alerts"`
}

// NewComplianceService creates a new compliance service
func NewComplianceService(db *sql.DB, auditService interfaces.AuditService) *ComplianceService {
	service := &ComplianceService{
		db:           db,
		auditService: auditService,
		policies:     NewPolicyManager(),
		sanctions:    NewSanctionsChecker(),
		kyc:          NewKYCProcessor(),
		aml:          NewAMLProcessor(),
		metrics:      &ComplianceMetrics{},
	}

	// Initialize database tables
	if err := service.initDatabase(); err != nil {
		log.Printf("Failed to initialize compliance database: %v", err)
	}

	// Load existing policies
	if err := service.loadPolicies(); err != nil {
		log.Printf("Failed to load existing policies: %v", err)
	}

	return service
}

// ProcessComplianceCheck performs a comprehensive compliance check
func (cs *ComplianceService) ProcessComplianceCheck(ctx context.Context, req *interfaces.ComplianceRequest) (*interfaces.ComplianceResult, error) {
	startTime := time.Now()

	// Create audit event
	auditEvent := &interfaces.AuditEvent{
		ID:        uuid.New().String(),
		Type:      interfaces.ActivityTypeCompliance,
		UserID:    req.UserID,
		Details:   map[string]interface{}{"request": req},
		Timestamp: time.Now(),
		IPAddress: req.IPAddress,
		UserAgent: req.UserAgent,
	}

	result := &interfaces.ComplianceResult{
		RequestID:       req.RequestID,
		UserID:          req.UserID,
		Status:          interfaces.ComplianceStatusPending,
		RiskScore:       0.0,
		Flags:           []string{},
		RequiredActions: []string{},
		ProcessedAt:     time.Now(),
		Details:         make(map[string]interface{}),
	}

	// Perform KYC check
	if req.CheckType == interfaces.CheckTypeKYC || req.CheckType == interfaces.CheckTypeAll {
		kycResult, err := cs.performKYCCheck(ctx, req)
		if err != nil {
			result.Status = interfaces.ComplianceStatusError
			result.Details["kyc_error"] = err.Error()
		} else {
			result.Details["kyc"] = kycResult
			result.RiskScore += kycResult.RiskScore
			if kycResult.Status == interfaces.ComplianceStatusFailed {
				result.Status = interfaces.ComplianceStatusFailed
				result.Flags = append(result.Flags, "KYC_FAILED")
			}
		}
	}

	// Perform AML check
	if req.CheckType == interfaces.CheckTypeAML || req.CheckType == interfaces.CheckTypeAll {
		amlResult, err := cs.performAMLCheck(ctx, req)
		if err != nil {
			result.Status = interfaces.ComplianceStatusError
			result.Details["aml_error"] = err.Error()
		} else {
			result.Details["aml"] = amlResult
			result.RiskScore += amlResult.RiskScore
			if amlResult.Status == interfaces.ComplianceStatusFailed {
				result.Status = interfaces.ComplianceStatusFailed
				result.Flags = append(result.Flags, "AML_FAILED")
			}
		}
	}

	// Perform sanctions check
	if req.CheckType == interfaces.CheckTypeSanctions || req.CheckType == interfaces.CheckTypeAll {
		sanctionsResult, err := cs.performSanctionsCheck(ctx, req)
		if err != nil {
			result.Status = interfaces.ComplianceStatusError
			result.Details["sanctions_error"] = err.Error()
		} else {
			result.Details["sanctions"] = sanctionsResult
			result.RiskScore += sanctionsResult.RiskScore
			if sanctionsResult.Status == interfaces.ComplianceStatusFailed {
				result.Status = interfaces.ComplianceStatusFailed
				result.Flags = append(result.Flags, "SANCTIONS_MATCH")
			}
		}
	}

	// Apply policy rules
	policyResult := cs.applyPolicyRules(ctx, req, result)
	result.RiskScore += policyResult.RiskScore
	result.Flags = append(result.Flags, policyResult.Flags...)
	result.RequiredActions = append(result.RequiredActions, policyResult.RequiredActions...)

	// Determine final status
	if result.Status != interfaces.ComplianceStatusFailed && result.Status != interfaces.ComplianceStatusError {
		result.Status = cs.determineFinalStatus(result.RiskScore, result.Flags)
	}

	// Update metrics
	cs.updateMetrics(result, time.Since(startTime))

	// Save result to database
	if err := cs.saveComplianceResult(ctx, result); err != nil {
		log.Printf("Failed to save compliance result: %v", err)
	}

	// Create audit log
	auditEvent.Details["result"] = result
	auditEvent.Details["duration_ms"] = time.Since(startTime).Milliseconds()
	if err := cs.auditService.LogEvent(ctx, auditEvent); err != nil {
		log.Printf("Failed to audit compliance check: %v", err)
	}

	return result, nil
}

// UpdatePolicy updates or creates a compliance policy
func (cs *ComplianceService) UpdatePolicy(ctx context.Context, policy *Policy) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	// Validate policy
	if err := cs.validatePolicy(policy); err != nil {
		return fmt.Errorf("invalid policy: %w", err)
	}

	// Generate policy hash for version control
	policyData, _ := json.Marshal(policy)
	hash := sha256.Sum256(policyData)
	policy.Version = fmt.Sprintf("%x", hash[:8])
	policy.UpdatedAt = time.Now()

	// Save to database
	if err := cs.savePolicyToDatabase(ctx, policy); err != nil {
		return fmt.Errorf("failed to save policy: %w", err)
	}

	// Update in-memory cache
	cs.policies.UpdatePolicy(policy)

	// Create audit event
	auditEvent := &interfaces.AuditEvent{
		ID:        uuid.New().String(),
		Type:      interfaces.ActivityTypePolicyUpdate,
		Details:   map[string]interface{}{"policy": policy},
		Timestamp: time.Now(),
	}

	if err := cs.auditService.LogEvent(ctx, auditEvent); err != nil {
		log.Printf("Failed to audit policy update: %v", err)
	}

	cs.metrics.PolicyUpdates++

	return nil
}

// GetUserRiskProfile retrieves the risk profile for a user
func (cs *ComplianceService) GetUserRiskProfile(ctx context.Context, userID string) (*interfaces.UserRiskProfile, error) {
	profile := &interfaces.UserRiskProfile{
		UserID:           userID,
		OverallRisk:      interfaces.RiskLevelLow,
		LastAssessment:   time.Now(),
		ComplianceStatus: interfaces.ComplianceStatusPassed,
	}

	// Query compliance history
	query := `
		SELECT status, risk_score, flags, created_at 
		FROM compliance_results 
		WHERE user_id = $1 
		ORDER BY created_at DESC 
		LIMIT 10
	`

	rows, err := cs.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query compliance history: %w", err)
	}
	defer rows.Close()

	var totalRisk float64
	var riskCount int
	var latestStatus interfaces.ComplianceStatus
	var flags []string

	for rows.Next() {
		var status string
		var riskScore float64
		var flagsJSON string
		var createdAt time.Time

		if err := rows.Scan(&status, &riskScore, &flagsJSON, &createdAt); err != nil {
			continue
		}

		if riskCount == 0 {
			latestStatus = interfaces.ComplianceStatus(status)
			profile.LastAssessment = createdAt
		}

		totalRisk += riskScore
		riskCount++

		var currentFlags []string
		if err := json.Unmarshal([]byte(flagsJSON), &currentFlags); err == nil {
			flags = append(flags, currentFlags...)
		}
	}

	if riskCount > 0 {
		avgRisk := totalRisk / float64(riskCount)
		profile.OverallRisk = cs.calculateRiskLevel(avgRisk)
		profile.ComplianceStatus = latestStatus
	}

	// Remove duplicate flags
	flagMap := make(map[string]bool)
	uniqueFlags := []string{}
	for _, flag := range flags {
		if !flagMap[flag] {
			flagMap[flag] = true
			uniqueFlags = append(uniqueFlags, flag)
		}
	}
	profile.Flags = uniqueFlags

	return profile, nil
}

// performKYCCheck executes KYC verification
func (cs *ComplianceService) performKYCCheck(ctx context.Context, req *interfaces.ComplianceRequest) (*interfaces.ComplianceResult, error) {
	result := &interfaces.ComplianceResult{
		RequestID:   req.RequestID,
		UserID:      req.UserID,
		Status:      interfaces.ComplianceStatusPassed,
		RiskScore:   0.0,
		ProcessedAt: time.Now(),
	}

	// Check if KYC is already completed
	if req.Data["kyc_status"] == "completed" {
		result.Status = interfaces.ComplianceStatusPassed
		return result, nil
	}

	// Validate required documents
	if !cs.kyc.hasRequiredDocuments(req.Data) {
		result.Status = interfaces.ComplianceStatusFailed
		result.RiskScore = 10.0
		result.RequiredActions = []string{"SUBMIT_REQUIRED_DOCUMENTS"}
		return result, nil
	}

	// Check age restrictions
	if age, ok := req.Data["age"].(float64); ok {
		if age < float64(cs.kyc.config.MinAge) {
			result.Status = interfaces.ComplianceStatusFailed
			result.RiskScore = 10.0
			result.Flags = []string{"AGE_RESTRICTION"}
			return result, nil
		}
	}

	// Check country restrictions
	if country, ok := req.Data["country"].(string); ok {
		if cs.kyc.isRestrictedCountry(country) {
			result.Status = interfaces.ComplianceStatusFailed
			result.RiskScore = 10.0
			result.Flags = []string{"RESTRICTED_COUNTRY"}
			return result, nil
		}
	}

	// Calculate risk score based on provided information
	riskScore := cs.kyc.calculateKYCRiskScore(req.Data)
	result.RiskScore = riskScore

	if riskScore > cs.kyc.config.ManualReviewThreshold {
		result.Status = interfaces.ComplianceStatusPending
		result.RequiredActions = []string{"MANUAL_REVIEW"}
	} else if riskScore > cs.kyc.config.AutoApprovalLimit {
		result.Status = interfaces.ComplianceStatusFlagged
		result.RequiredActions = []string{"ENHANCED_VERIFICATION"}
	}

	return result, nil
}

// performAMLCheck executes Anti-Money Laundering checks
func (cs *ComplianceService) performAMLCheck(ctx context.Context, req *interfaces.ComplianceRequest) (*interfaces.ComplianceResult, error) {
	result := &interfaces.ComplianceResult{
		RequestID:   req.RequestID,
		UserID:      req.UserID,
		Status:      interfaces.ComplianceStatusPassed,
		RiskScore:   0.0,
		ProcessedAt: time.Now(),
	}

	// Check transaction patterns
	if transactionData, ok := req.Data["transaction"]; ok {
		if txMap, ok := transactionData.(map[string]interface{}); ok {
			patternScore := cs.aml.analyzeTransactionPattern(txMap)
			result.RiskScore += patternScore

			if patternScore > cs.aml.threshold {
				result.Status = interfaces.ComplianceStatusFlagged
				result.Flags = []string{"SUSPICIOUS_PATTERN"}
				result.RequiredActions = []string{"AML_INVESTIGATION"}
			}
		}
	}

	// Check for high-risk jurisdictions
	if sourceCountry, ok := req.Data["source_country"].(string); ok {
		if cs.aml.isHighRiskJurisdiction(sourceCountry) {
			result.RiskScore += 2.0
			result.Flags = []string{"HIGH_RISK_JURISDICTION"}
		}
	}

	// Check PEP (Politically Exposed Person) status
	if isPEP, ok := req.Data["is_pep"].(bool); ok && isPEP {
		result.RiskScore += 3.0
		result.Flags = []string{"PEP"}
		result.RequiredActions = []string{"ENHANCED_DUE_DILIGENCE"}
	}

	return result, nil
}

// performSanctionsCheck executes sanctions screening
func (cs *ComplianceService) performSanctionsCheck(ctx context.Context, req *interfaces.ComplianceRequest) (*interfaces.ComplianceResult, error) {
	result := &interfaces.ComplianceResult{
		RequestID:   req.RequestID,
		UserID:      req.UserID,
		Status:      interfaces.ComplianceStatusPassed,
		RiskScore:   0.0,
		ProcessedAt: time.Now(),
	}

	// Check name against sanctions lists
	if name, ok := req.Data["name"].(string); ok {
		if cs.sanctions.checkName(name) {
			result.Status = interfaces.ComplianceStatusFailed
			result.RiskScore = 10.0
			result.Flags = []string{"SANCTIONS_MATCH"}
			cs.metrics.SanctionsMatches++
			return result, nil
		}
	}

	// Check address against sanctions lists
	if address, ok := req.Data["address"].(string); ok {
		if cs.sanctions.checkAddress(address) {
			result.Status = interfaces.ComplianceStatusFailed
			result.RiskScore = 10.0
			result.Flags = []string{"SANCTIONS_ADDRESS_MATCH"}
			cs.metrics.SanctionsMatches++
			return result, nil
		}
	}

	return result, nil
}

// Helper methods for the service components
func (cs *ComplianceService) applyPolicyRules(ctx context.Context, req *interfaces.ComplianceRequest, result *interfaces.ComplianceResult) *interfaces.ComplianceResult {
	policyResult := &interfaces.ComplianceResult{
		RiskScore:       0.0,
		Flags:           []string{},
		RequiredActions: []string{},
	}

	cs.policies.mu.RLock()
	defer cs.policies.mu.RUnlock()

	for _, policy := range cs.policies.policies {
		if !policy.Active {
			continue
		}

		for _, rule := range policy.Rules {
			if !rule.Active {
				continue
			}

			// Evaluate rule condition (simplified)
			if cs.evaluateRuleCondition(rule, req, result) {
				policyResult.RiskScore += rule.Weight

				switch rule.Action {
				case "block":
					policyResult.Flags = append(policyResult.Flags, fmt.Sprintf("POLICY_BLOCK_%s", rule.ID))
				case "flag":
					policyResult.Flags = append(policyResult.Flags, fmt.Sprintf("POLICY_FLAG_%s", rule.ID))
				case "monitor":
					policyResult.RequiredActions = append(policyResult.RequiredActions, fmt.Sprintf("MONITOR_%s", rule.ID))
				case "escalate":
					policyResult.RequiredActions = append(policyResult.RequiredActions, fmt.Sprintf("ESCALATE_%s", rule.ID))
				}
			}
		}
	}

	return policyResult
}

func (cs *ComplianceService) evaluateRuleCondition(rule Rule, req *interfaces.ComplianceRequest, result *interfaces.ComplianceResult) bool {
	// Simplified rule evaluation - in production, use a proper expression evaluator
	condition := strings.ToLower(rule.Condition)

	if strings.Contains(condition, "risk_score") && strings.Contains(condition, ">") {
		// Extract threshold from condition
		if threshold, ok := rule.Parameters["threshold"].(float64); ok {
			return result.RiskScore > threshold
		}
	}

	if strings.Contains(condition, "country") && strings.Contains(condition, "=") {
		if country, ok := req.Data["country"].(string); ok {
			if targetCountry, ok := rule.Parameters["country"].(string); ok {
				return strings.EqualFold(country, targetCountry)
			}
		}
	}

	return false
}

func (cs *ComplianceService) determineFinalStatus(riskScore float64, flags []string) interfaces.ComplianceStatus {
	if riskScore >= 8.0 {
		return interfaces.ComplianceStatusFailed
	}

	if riskScore >= 5.0 || len(flags) > 0 {
		return interfaces.ComplianceStatusFlagged
	}

	if riskScore >= 3.0 {
		return interfaces.ComplianceStatusPending
	}

	return interfaces.ComplianceStatusPassed
}

func (cs *ComplianceService) calculateRiskLevel(avgRisk float64) interfaces.RiskLevel {
	if avgRisk >= 7.0 {
		return interfaces.RiskLevelCritical
	} else if avgRisk >= 5.0 {
		return interfaces.RiskLevelHigh
	} else if avgRisk >= 3.0 {
		return interfaces.RiskLevelMedium
	}
	return interfaces.RiskLevelLow
}

func (cs *ComplianceService) updateMetrics(result *interfaces.ComplianceResult, duration time.Duration) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	cs.metrics.TotalChecks++

	switch result.Status {
	case interfaces.ComplianceStatusPassed:
		cs.metrics.PassedChecks++
	case interfaces.ComplianceStatusFailed:
		cs.metrics.FailedChecks++
	case interfaces.ComplianceStatusFlagged:
		cs.metrics.FlaggedChecks++
	}

	// Update average latency
	if cs.metrics.TotalChecks == 1 {
		cs.metrics.AverageLatency = duration.Seconds()
	} else {
		cs.metrics.AverageLatency = (cs.metrics.AverageLatency*float64(cs.metrics.TotalChecks-1) + duration.Seconds()) / float64(cs.metrics.TotalChecks)
	}

	cs.metrics.LastUpdated = time.Now()
}

// Database operations
func (cs *ComplianceService) initDatabase() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS compliance_results (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			request_id VARCHAR(255) NOT NULL,
			user_id VARCHAR(255) NOT NULL,
			status VARCHAR(50) NOT NULL,
			risk_score DECIMAL(5,2) NOT NULL DEFAULT 0.00,
			flags JSONB DEFAULT '[]',
			required_actions JSONB DEFAULT '[]',
			details JSONB DEFAULT '{}',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			INDEX idx_compliance_user_id (user_id),
			INDEX idx_compliance_status (status),
			INDEX idx_compliance_created_at (created_at)
		)`,
		`CREATE TABLE IF NOT EXISTS compliance_policies (
			id VARCHAR(255) PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			type VARCHAR(50) NOT NULL,
			version VARCHAR(255) NOT NULL,
			rules JSONB NOT NULL DEFAULT '[]',
			thresholds JSONB NOT NULL DEFAULT '{}',
			active BOOLEAN DEFAULT true,
			priority INTEGER DEFAULT 0,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			expires_at TIMESTAMP NULL,
			INDEX idx_policy_type (type),
			INDEX idx_policy_active (active),
			INDEX idx_policy_priority (priority)
		)`,
		`CREATE TABLE IF NOT EXISTS sanctions_lists (
			id VARCHAR(255) PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			source VARCHAR(255) NOT NULL,
			entries JSONB NOT NULL DEFAULT '[]',
			priority INTEGER DEFAULT 0,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			INDEX idx_sanctions_source (source),
			INDEX idx_sanctions_updated (updated_at)
		)`,
	}

	for _, query := range queries {
		if _, err := cs.db.Exec(query); err != nil {
			return fmt.Errorf("failed to execute query: %w", err)
		}
	}

	return nil
}

func (cs *ComplianceService) saveComplianceResult(ctx context.Context, result *interfaces.ComplianceResult) error {
	flagsJSON, _ := json.Marshal(result.Flags)
	actionsJSON, _ := json.Marshal(result.RequiredActions)
	detailsJSON, _ := json.Marshal(result.Details)

	query := `
		INSERT INTO compliance_results (request_id, user_id, status, risk_score, flags, required_actions, details)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`

	_, err := cs.db.ExecContext(ctx, query,
		result.RequestID, result.UserID, result.Status, result.RiskScore,
		flagsJSON, actionsJSON, detailsJSON)

	return err
}

func (cs *ComplianceService) savePolicyToDatabase(ctx context.Context, policy *Policy) error {
	rulesJSON, _ := json.Marshal(policy.Rules)
	thresholdsJSON, _ := json.Marshal(policy.Thresholds)

	query := `
		INSERT INTO compliance_policies (id, name, type, version, rules, thresholds, active, priority, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			type = EXCLUDED.type,
			version = EXCLUDED.version,
			rules = EXCLUDED.rules,
			thresholds = EXCLUDED.thresholds,
			active = EXCLUDED.active,
			priority = EXCLUDED.priority,
			expires_at = EXCLUDED.expires_at,
			updated_at = CURRENT_TIMESTAMP
	`

	_, err := cs.db.ExecContext(ctx, query,
		policy.ID, policy.Name, policy.Type, policy.Version,
		rulesJSON, thresholdsJSON, policy.Active, policy.Priority, policy.ExpiresAt)

	return err
}

func (cs *ComplianceService) loadPolicies() error {
	query := `
		SELECT id, name, type, version, rules, thresholds, active, priority, created_at, updated_at, expires_at
		FROM compliance_policies
		WHERE active = true AND (expires_at IS NULL OR expires_at > CURRENT_TIMESTAMP)
		ORDER BY priority DESC
	`

	rows, err := cs.db.Query(query)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var policy Policy
		var rulesJSON, thresholdsJSON string
		var expiresAt sql.NullTime

		err := rows.Scan(
			&policy.ID, &policy.Name, &policy.Type, &policy.Version,
			&rulesJSON, &thresholdsJSON, &policy.Active, &policy.Priority,
			&policy.CreatedAt, &policy.UpdatedAt, &expiresAt,
		)
		if err != nil {
			continue
		}

		if expiresAt.Valid {
			policy.ExpiresAt = &expiresAt.Time
		}

		json.Unmarshal([]byte(rulesJSON), &policy.Rules)
		json.Unmarshal([]byte(thresholdsJSON), &policy.Thresholds)

		cs.policies.UpdatePolicy(&policy)
	}

	return nil
}

func (cs *ComplianceService) validatePolicy(policy *Policy) error {
	if policy.ID == "" {
		return fmt.Errorf("policy ID is required")
	}
	if policy.Name == "" {
		return fmt.Errorf("policy name is required")
	}
	if policy.Type == "" {
		return fmt.Errorf("policy type is required")
	}
	if len(policy.Rules) == 0 {
		return fmt.Errorf("policy must have at least one rule")
	}
	return nil
}
