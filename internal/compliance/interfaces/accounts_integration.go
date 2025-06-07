// Package interfaces provides integration between compliance and accounts modules
package interfaces

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/Aidin1998/finalex/internal/integration/contracts"
)

// AccountsIntegration provides compliance integration with the accounts module
type AccountsIntegration struct {
	accountsService   contracts.AccountsServiceContract
	complianceService ComplianceService
	auditService      AuditService
	logger            *zap.Logger
}

// UserRiskProfile represents user risk information from accounts perspective
type UserRiskProfile struct {
	UserID             uuid.UUID                  `json:"user_id"`
	TotalBalance       decimal.Decimal            `json:"total_balance"`
	CurrencyBalances   map[string]decimal.Decimal `json:"currency_balances"`
	TransactionVolume  decimal.Decimal            `json:"transaction_volume_24h"`
	TransactionCount   int64                      `json:"transaction_count_24h"`
	LastActivity       time.Time                  `json:"last_activity"`
	AccountStatus      string                     `json:"account_status"`
	SuspiciousActivity bool                       `json:"suspicious_activity"`
	ComplianceFlags    []ComplianceFlag           `json:"compliance_flags"`
	RiskScore          decimal.Decimal            `json:"risk_score"`
	KYCLevel           string                     `json:"kyc_level"`
	CreatedAt          time.Time                  `json:"created_at"`
}

// ComplianceFlag represents a compliance warning or restriction
type ComplianceFlag struct {
	Type        string                 `json:"type"`
	Severity    string                 `json:"severity"`
	Description string                 `json:"description"`
	CreatedAt   time.Time              `json:"created_at"`
	ExpiresAt   *time.Time             `json:"expires_at,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// BalanceComplianceCheck represents balance-related compliance verification
type BalanceComplianceCheck struct {
	UserID           uuid.UUID         `json:"user_id"`
	Currency         string            `json:"currency"`
	RequestedAmount  decimal.Decimal   `json:"requested_amount"`
	AvailableBalance decimal.Decimal   `json:"available_balance"`
	OperationType    string            `json:"operation_type"` // withdraw, trade, transfer
	ComplianceResult *ComplianceResult `json:"compliance_result"`
	CheckedAt        time.Time         `json:"checked_at"`
}

// AccountFreezeRequest represents a request to freeze account assets
type AccountFreezeRequest struct {
	UserID      uuid.UUID              `json:"user_id"`
	Currency    string                 `json:"currency,omitempty"` // if empty, freeze all
	Amount      *decimal.Decimal       `json:"amount,omitempty"`   // if nil, freeze all available
	Reason      string                 `json:"reason"`
	Duration    *time.Duration         `json:"duration,omitempty"` // if nil, indefinite
	RequestedBy string                 `json:"requested_by"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// AccountFreezeResult represents the result of an account freeze operation
type AccountFreezeResult struct {
	FreezeID       string                     `json:"freeze_id"`
	UserID         uuid.UUID                  `json:"user_id"`
	FrozenBalances map[string]decimal.Decimal `json:"frozen_balances"`
	Success        bool                       `json:"success"`
	Message        string                     `json:"message"`
	CreatedAt      time.Time                  `json:"created_at"`
	ExpiresAt      *time.Time                 `json:"expires_at,omitempty"`
}

// NewAccountsIntegration creates a new accounts integration
func NewAccountsIntegration(
	accountsService contracts.AccountsServiceContract,
	complianceService ComplianceService,
	auditService AuditService,
	logger *zap.Logger,
) *AccountsIntegration {
	return &AccountsIntegration{
		accountsService:   accountsService,
		complianceService: complianceService,
		auditService:      auditService,
		logger:            logger,
	}
}

// ValidateBalanceCompliance validates compliance for balance operations
func (ai *AccountsIntegration) ValidateBalanceCompliance(ctx context.Context, userID uuid.UUID, currency string, amount decimal.Decimal, operationType string) (*BalanceComplianceCheck, error) {
	ai.logger.Debug("Validating balance compliance",
		zap.String("user_id", userID.String()),
		zap.String("currency", currency),
		zap.String("amount", amount.String()),
		zap.String("operation_type", operationType),
	)

	// Get current balance
	balance, err := ai.accountsService.GetBalance(ctx, userID, currency)
	if err != nil {
		return nil, fmt.Errorf("failed to get balance: %w", err)
	}
	// Perform compliance check
	complianceReq := &ComplianceRequest{
		UserID:           userID,
		ActivityType:     ActivityDeposit, // Will be set based on operationType
		Amount:           &amount,
		Currency:         currency,
		IPAddress:        "",
		UserAgent:        "",
		DeviceID:         "",
		Country:          "",
		RequestTimestamp: time.Now(),
		Metadata: map[string]interface{}{
			"operation_type":    operationType,
			"available_balance": balance.Available.String(),
			"requested_amount":  amount.String(),
		},
	}

	// Set proper activity type based on operation
	switch operationType {
	case "withdraw":
		complianceReq.ActivityType = ActivityWithdrawal
	case "deposit":
		complianceReq.ActivityType = ActivityDeposit
	case "trade":
		complianceReq.ActivityType = ActivityTrade
	case "transfer":
		complianceReq.ActivityType = ActivityTransfer
	default:
		complianceReq.ActivityType = ActivityDeposit
	}

	complianceResult, err := ai.complianceService.PerformComplianceCheck(ctx, complianceReq)
	if err != nil {
		return nil, fmt.Errorf("compliance check failed: %w", err)
	}

	check := &BalanceComplianceCheck{
		UserID:           userID,
		Currency:         currency,
		RequestedAmount:  amount,
		AvailableBalance: balance.Available,
		OperationType:    operationType,
		ComplianceResult: complianceResult,
		CheckedAt:        time.Now(),
	} // Log audit event
	auditEvent := &AuditEvent{
		ID:          uuid.New(),
		UserID:      &userID,
		EventType:   "balance_compliance_check",
		Category:    "compliance",
		Severity:    "info",
		Description: fmt.Sprintf("Balance compliance check for %s %s %s", operationType, amount.String(), currency),
		IPAddress:   "",
		UserAgent:   "",
		Resource:    "balance",
		ResourceID:  userID.String(),
		Action:      "compliance_check",
		Metadata: map[string]interface{}{
			"currency":          currency,
			"amount":            amount.String(),
			"operation_type":    operationType,
			"compliance_status": complianceResult.Status.String(),
			"risk_level":        complianceResult.RiskLevel.String(),
			"available_balance": balance.Available.String(),
		},
		Timestamp:   time.Now(),
		ProcessedBy: "accounts_integration",
	}

	if err := ai.auditService.LogEvent(ctx, auditEvent); err != nil {
		ai.logger.Warn("Failed to log audit event", zap.Error(err))
	}

	return check, nil
}

// GetUserRiskProfile retrieves comprehensive user risk profile including account data
func (ai *AccountsIntegration) GetUserRiskProfile(ctx context.Context, userID uuid.UUID) (*UserRiskProfile, error) {
	ai.logger.Debug("Getting user risk profile", zap.String("user_id", userID.String()))

	// Get all user balances
	balances, err := ai.accountsService.GetAllBalances(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user balances: %w", err)
	}

	// Calculate total balance and currency breakdown
	currencyBalances := make(map[string]decimal.Decimal)
	totalBalance := decimal.Zero

	for _, balance := range balances {
		currencyBalances[balance.Currency] = balance.Available.Add(balance.Reserved)
		// For simplicity, add all balances (in reality would need conversion rates)
		totalBalance = totalBalance.Add(balance.Available.Add(balance.Reserved))
	}

	// Get transaction history for last 24 hours
	// Note: This would need to be implemented based on actual transaction history API
	transactionVolume, transactionCount := ai.calculateRecentActivity(ctx, userID)
	// Assess compliance risk
	riskResult, err := ai.complianceService.AssessUserRisk(ctx, userID)
	if err != nil {
		ai.logger.Warn("Failed to assess user risk", zap.Error(err))
		riskResult = &ComplianceResult{
			Status:    ComplianceStatusPending,
			RiskScore: decimal.NewFromFloat(0.5), // default moderate risk
			RiskLevel: RiskLevelMedium,
		}
	}

	// Get KYC status
	kycLevel, err := ai.complianceService.GetKYCStatus(ctx, userID)
	if err != nil {
		ai.logger.Warn("Failed to get KYC status", zap.Error(err))
		kycLevel = KYCLevelNone
	}

	profile := &UserRiskProfile{
		UserID:             userID,
		TotalBalance:       totalBalance,
		CurrencyBalances:   currencyBalances,
		TransactionVolume:  transactionVolume,
		TransactionCount:   transactionCount,
		LastActivity:       time.Now(), // Would be from actual transaction data
		AccountStatus:      "active",   // Would be from accounts service
		SuspiciousActivity: riskResult.Status == ComplianceStatusBlocked,
		ComplianceFlags:    ai.extractComplianceFlags(riskResult),
		RiskScore:          riskResult.RiskScore,
		KYCLevel:           string(kycLevel),
		CreatedAt:          time.Now(), // Would be from user creation data
	}

	return profile, nil
}

// FreezeAccountAssets freezes user account assets for compliance reasons
func (ai *AccountsIntegration) FreezeAccountAssets(ctx context.Context, req *AccountFreezeRequest) (*AccountFreezeResult, error) {
	ai.logger.Info("Freezing account assets",
		zap.String("user_id", req.UserID.String()),
		zap.String("currency", req.Currency),
		zap.String("reason", req.Reason),
	)

	freezeID := uuid.New().String()
	frozenBalances := make(map[string]decimal.Decimal)

	// If currency is specified, freeze only that currency
	if req.Currency != "" {
		balance, err := ai.accountsService.GetBalance(ctx, req.UserID, req.Currency)
		if err != nil {
			return nil, fmt.Errorf("failed to get balance for currency %s: %w", req.Currency, err)
		}

		amountToFreeze := balance.Available
		if req.Amount != nil && req.Amount.LessThan(balance.Available) {
			amountToFreeze = *req.Amount
		}
		// Reserve the amount (effectively freezing it)
		reserveReq := &contracts.ReserveBalanceRequest{
			UserID:          req.UserID,
			Currency:        req.Currency,
			Amount:          amountToFreeze,
			ReservationType: "compliance_freeze",
			ReferenceID:     freezeID,
			Metadata: map[string]interface{}{
				"freeze_type":  "compliance",
				"reason":       req.Reason,
				"requested_by": req.RequestedBy,
				"description":  fmt.Sprintf("Compliance freeze: %s", req.Reason),
			},
		}

		_, err = ai.accountsService.ReserveBalance(ctx, reserveReq)
		if err != nil {
			return nil, fmt.Errorf("failed to freeze balance: %w", err)
		}

		frozenBalances[req.Currency] = amountToFreeze
	} else {
		// Freeze all currencies
		balances, err := ai.accountsService.GetAllBalances(ctx, req.UserID)
		if err != nil {
			return nil, fmt.Errorf("failed to get all balances: %w", err)
		}

		for _, balance := range balances {
			if balance.Available.IsPositive() {
				reserveReq := &contracts.ReserveBalanceRequest{
					UserID:          req.UserID,
					Currency:        balance.Currency,
					Amount:          balance.Available,
					ReservationType: "compliance_freeze",
					ReferenceID:     freezeID,
					Metadata: map[string]interface{}{
						"freeze_type":  "compliance",
						"reason":       req.Reason,
						"requested_by": req.RequestedBy,
						"description":  fmt.Sprintf("Compliance freeze: %s", req.Reason),
					},
				}

				_, err = ai.accountsService.ReserveBalance(ctx, reserveReq)
				if err != nil {
					ai.logger.Error("Failed to freeze balance for currency",
						zap.String("currency", balance.Currency),
						zap.Error(err),
					)
					continue
				}

				frozenBalances[balance.Currency] = balance.Available
			}
		}
	}

	result := &AccountFreezeResult{
		FreezeID:       freezeID,
		UserID:         req.UserID,
		FrozenBalances: frozenBalances,
		Success:        len(frozenBalances) > 0,
		Message:        fmt.Sprintf("Frozen balances for %d currencies", len(frozenBalances)),
		CreatedAt:      time.Now(),
	}

	if req.Duration != nil {
		expiresAt := time.Now().Add(*req.Duration)
		result.ExpiresAt = &expiresAt
	} // Log audit event
	auditEvent := &AuditEvent{
		ID:          uuid.New(),
		UserID:      &req.UserID,
		EventType:   "account_freeze",
		Category:    "compliance",
		Severity:    "high",
		Description: fmt.Sprintf("Account assets frozen: %s", req.Reason),
		IPAddress:   "",
		UserAgent:   "",
		Resource:    "account",
		ResourceID:  req.UserID.String(),
		Action:      "freeze",
		Metadata: map[string]interface{}{
			"freeze_id":       freezeID,
			"reason":          req.Reason,
			"requested_by":    req.RequestedBy,
			"frozen_balances": frozenBalances,
		},
		Timestamp:   time.Now(),
		ProcessedBy: "accounts_integration",
	}

	if err := ai.auditService.LogEvent(ctx, auditEvent); err != nil {
		ai.logger.Warn("Failed to log freeze audit event", zap.Error(err))
	}

	return result, nil
}

// UnfreezeAccountAssets removes asset freeze for compliance reasons
func (ai *AccountsIntegration) UnfreezeAccountAssets(ctx context.Context, freezeID string, reason string, requestedBy string) error {
	ai.logger.Info("Unfreezing account assets",
		zap.String("freeze_id", freezeID),
		zap.String("reason", reason),
	)

	// Release the reservation using the freeze ID as reference
	err := ai.accountsService.ReleaseReservation(ctx, uuid.MustParse(freezeID))
	if err != nil {
		return fmt.Errorf("failed to release frozen assets: %w", err)
	}
	// Log audit event
	auditEvent := &AuditEvent{
		ID:          uuid.New(),
		EventType:   "account_unfreeze",
		Category:    "compliance",
		Severity:    "info",
		Description: fmt.Sprintf("Account assets unfrozen: %s", reason),
		IPAddress:   "",
		UserAgent:   "",
		Resource:    "account",
		ResourceID:  "",
		Action:      "unfreeze",
		Metadata: map[string]interface{}{
			"freeze_id":    freezeID,
			"reason":       reason,
			"requested_by": requestedBy,
		},
		Timestamp:   time.Now(),
		ProcessedBy: "accounts_integration",
	}

	if err := ai.auditService.LogEvent(ctx, auditEvent); err != nil {
		ai.logger.Warn("Failed to log unfreeze audit event", zap.Error(err))
	}

	return nil
}

// ValidateTransactionCompliance validates compliance for account transactions
func (ai *AccountsIntegration) ValidateTransactionCompliance(ctx context.Context, userID uuid.UUID, txType string, amount decimal.Decimal, currency string, metadata map[string]interface{}) (*ComplianceResult, error) {
	ai.logger.Debug("Validating transaction compliance",
		zap.String("user_id", userID.String()),
		zap.String("tx_type", txType),
		zap.String("amount", amount.String()),
		zap.String("currency", currency),
	)

	// Get current balance for context
	balance, err := ai.accountsService.GetBalance(ctx, userID, currency)
	if err != nil {
		ai.logger.Warn("Failed to get balance for compliance check", zap.Error(err))
	} else {
		if metadata == nil {
			metadata = make(map[string]interface{})
		}
		metadata["current_balance"] = balance.Available.String()
		metadata["reserved_balance"] = balance.Reserved.String()
	}

	// Perform comprehensive compliance validation
	result, err := ai.complianceService.ValidateTransaction(ctx, userID, txType, amount, currency, metadata)
	if err != nil {
		return nil, fmt.Errorf("transaction compliance validation failed: %w", err)
	}
	// Log audit event
	auditEvent := &AuditEvent{
		ID:          uuid.New(),
		EventType:   "transaction_compliance_check",
		UserID:      &userID,
		Category:    "compliance",
		Severity:    "info",
		Description: fmt.Sprintf("Transaction compliance check for %s", txType),
		IPAddress:   "",
		UserAgent:   "",
		Resource:    "transaction",
		ResourceID:  "",
		Action:      "compliance_check",
		Metadata: map[string]interface{}{
			"tx_type":           txType,
			"amount":            amount.String(),
			"currency":          currency,
			"compliance_status": result.Status,
			"risk_score":        result.RiskScore.String(),
		},
		Timestamp:   time.Now(),
		ProcessedBy: "accounts_integration",
	}

	if err := ai.auditService.LogEvent(ctx, auditEvent); err != nil {
		ai.logger.Warn("Failed to log transaction compliance audit event", zap.Error(err))
	}

	return result, nil
}

// Helper methods

// calculateRecentActivity calculates user transaction activity in the last 24 hours
func (ai *AccountsIntegration) calculateRecentActivity(ctx context.Context, userID uuid.UUID) (decimal.Decimal, int64) {
	// This is a placeholder - would need to be implemented based on actual transaction history API
	// For now, return dummy values
	return decimal.NewFromFloat(1000.0), 25
}

// extractComplianceFlags extracts compliance flags from compliance result
func (ai *AccountsIntegration) extractComplianceFlags(result *ComplianceResult) []ComplianceFlag {
	var flags []ComplianceFlag
	if result.Status == ComplianceStatusBlocked {
		flags = append(flags, ComplianceFlag{
			Type:        "blocked",
			Severity:    "high",
			Description: result.Reason,
			CreatedAt:   time.Now(),
		})
	}

	if result.RiskScore.GreaterThan(decimal.NewFromFloat(0.8)) {
		flags = append(flags, ComplianceFlag{
			Type:        "high_risk",
			Severity:    "medium",
			Description: "High risk score detected",
			CreatedAt:   time.Now(),
		})
	}

	return flags
}

// GetAccountIntegrityStatus checks account balance integrity for compliance
func (ai *AccountsIntegration) GetAccountIntegrityStatus(ctx context.Context, userID uuid.UUID) (*contracts.IntegrityReport, error) {
	ai.logger.Debug("Checking account integrity", zap.String("user_id", userID.String()))

	report, err := ai.accountsService.ValidateBalanceIntegrity(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to validate balance integrity: %w", err)
	}
	// Log audit event if integrity issues found
	if !report.IsValid {
		auditEvent := &AuditEvent{
			ID:          uuid.New(),
			EventType:   "integrity_violation",
			UserID:      &userID,
			Category:    "compliance",
			Severity:    "warning",
			Description: "Account balance integrity issues detected",
			IPAddress:   "",
			UserAgent:   "",
			Resource:    "account",
			ResourceID:  userID.String(),
			Action:      "integrity_check",
			Metadata: map[string]interface{}{
				"issue_count": len(report.Issues),
				"issues":      report.Issues,
				"metrics":     report.Metrics,
			},
			Timestamp:   time.Now(),
			ProcessedBy: "accounts_integration",
		}

		if err := ai.auditService.LogEvent(ctx, auditEvent); err != nil {
			ai.logger.Warn("Failed to log integrity violation audit event", zap.Error(err))
		}
	}

	return report, nil
}
