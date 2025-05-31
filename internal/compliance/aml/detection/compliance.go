package risk

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/shopspring/decimal"
)

// ComplianceRule defines a compliance checking rule
type ComplianceRule struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Type        string                 `json:"type"` // "aml", "kyt", "pattern", "velocity"
	Parameters  map[string]interface{} `json:"parameters"`
	IsActive    bool                   `json:"is_active"`
	Severity    string                 `json:"severity"` // "low", "medium", "high", "critical"
	Description string                 `json:"description"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

// TransactionPattern represents a detected transaction pattern
type TransactionPattern struct {
	PatternType  string                 `json:"pattern_type"`
	Description  string                 `json:"description"`
	Confidence   decimal.Decimal        `json:"confidence"`
	RiskScore    decimal.Decimal        `json:"risk_score"`
	DetectedAt   time.Time              `json:"detected_at"`
	Transactions []TransactionRecord    `json:"transactions"`
	Metadata     map[string]interface{} `json:"metadata"`
}

// TransactionRecord holds transaction data for compliance analysis
type TransactionRecord struct {
	ID          string                 `json:"id"`
	UserID      string                 `json:"user_id"`
	Type        string                 `json:"type"` // "deposit", "withdrawal", "trade"
	Amount      decimal.Decimal        `json:"amount"`
	Currency    string                 `json:"currency"`
	Timestamp   time.Time              `json:"timestamp"`
	Source      string                 `json:"source"`
	Destination string                 `json:"destination"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// ComplianceAlert represents a compliance violation alert
type ComplianceAlert struct {
	ID               string                 `json:"id"`
	UserID           string                 `json:"user_id"`
	RuleID           string                 `json:"rule_id"`
	RuleName         string                 `json:"rule_name"`
	Severity         string                 `json:"severity"`
	Status           string                 `json:"status"` // "open", "investigating", "resolved", "false_positive"
	Description      string                 `json:"description"`
	DetectedPatterns []TransactionPattern   `json:"detected_patterns"`
	RiskScore        decimal.Decimal        `json:"risk_score"`
	CreatedAt        time.Time              `json:"created_at"`
	UpdatedAt        time.Time              `json:"updated_at"`
	AssignedTo       string                 `json:"assigned_to"`
	ResolutionNotes  string                 `json:"resolution_notes"`
	Metadata         map[string]interface{} `json:"metadata"`
}

// ComplianceEngine provides advanced compliance monitoring and pattern detection
type ComplianceEngine struct {
	mu           sync.RWMutex
	rules        map[string]*ComplianceRule
	transactions []TransactionRecord // In-memory store for demo (use DB in production)
	alerts       []ComplianceAlert
	patterns     []TransactionPattern

	// Performance tracking
	totalChecks      int64
	totalAlerts      int64
	averageCheckTime time.Duration
}

// NewComplianceEngine creates a new compliance engine with default rules
func NewComplianceEngine() *ComplianceEngine {
	ce := &ComplianceEngine{
		rules:        make(map[string]*ComplianceRule),
		transactions: make([]TransactionRecord, 0),
		alerts:       make([]ComplianceAlert, 0),
		patterns:     make([]TransactionPattern, 0),
	}

	// Initialize with default AML/KYT rules
	ce.initializeDefaultRules()

	return ce
}

// initializeDefaultRules sets up standard AML/KYT compliance rules
func (ce *ComplianceEngine) initializeDefaultRules() {
	defaultRules := []*ComplianceRule{
		{
			ID:   "large_transaction",
			Name: "Large Transaction Monitoring",
			Type: "aml",
			Parameters: map[string]interface{}{
				"threshold_usd":   10000,
				"timeframe_hours": 24,
			},
			IsActive:    true,
			Severity:    "medium",
			Description: "Monitor transactions exceeding $10,000",
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		},
		{
			ID:   "velocity_check",
			Name: "Transaction Velocity Check",
			Type: "velocity",
			Parameters: map[string]interface{}{
				"max_transactions_per_hour": 50,
				"max_volume_per_hour_usd":   100000,
			},
			IsActive:    true,
			Severity:    "high",
			Description: "Monitor unusual transaction velocity",
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		},
		{
			ID:   "structuring_detection",
			Name: "Transaction Structuring Detection",
			Type: "pattern",
			Parameters: map[string]interface{}{
				"amount_threshold":    9500,
				"frequency_threshold": 3,
				"timeframe_hours":     24,
			},
			IsActive:    true,
			Severity:    "critical",
			Description: "Detect potential transaction structuring (smurfing)",
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		},
		{
			ID:   "round_amount_pattern",
			Name: "Round Amount Pattern Detection",
			Type: "pattern",
			Parameters: map[string]interface{}{
				"round_threshold":     1000,
				"frequency_threshold": 5,
				"timeframe_hours":     72,
			},
			IsActive:    true,
			Severity:    "medium",
			Description: "Detect patterns of round-number transactions",
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		},
		{
			ID:   "rapid_deposit_withdrawal",
			Name: "Rapid Deposit-Withdrawal Pattern",
			Type: "pattern",
			Parameters: map[string]interface{}{
				"max_time_gap_minutes": 30,
				"min_amount_ratio":     0.8,
			},
			IsActive:    true,
			Severity:    "high",
			Description: "Detect rapid deposit followed by withdrawal",
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		},
	}

	for _, rule := range defaultRules {
		ce.rules[rule.ID] = rule
	}
}

// RecordTransaction records a transaction for compliance monitoring
func (ce *ComplianceEngine) RecordTransaction(ctx context.Context, transaction TransactionRecord) error {
	ce.mu.Lock()
	ce.transactions = append(ce.transactions, transaction)

	// Keep only recent transactions (last 30 days for demo)
	cutoff := time.Now().AddDate(0, 0, -30)
	var recentTransactions []TransactionRecord
	for _, tx := range ce.transactions {
		if tx.Timestamp.After(cutoff) {
			recentTransactions = append(recentTransactions, tx)
		}
	}
	ce.transactions = recentTransactions
	ce.mu.Unlock()

	// Run compliance checks on the new transaction
	return ce.RunComplianceChecks(ctx, transaction.UserID, &transaction)
}

// RunComplianceChecks performs all active compliance checks for a user/transaction
func (ce *ComplianceEngine) RunComplianceChecks(ctx context.Context, userID string, transaction *TransactionRecord) error {
	start := time.Now()
	defer func() {
		ce.mu.Lock()
		ce.totalChecks++
		elapsed := time.Since(start)
		ce.averageCheckTime = time.Duration((int64(ce.averageCheckTime)*ce.totalChecks + int64(elapsed)) / (ce.totalChecks + 1))
		ce.mu.Unlock()
	}()

	ce.mu.RLock()
	activeRules := make([]*ComplianceRule, 0)
	for _, rule := range ce.rules {
		if rule.IsActive {
			activeRules = append(activeRules, rule)
		}
	}
	ce.mu.RUnlock()

	// Run each active rule
	for _, rule := range activeRules {
		alert, err := ce.executeRule(ctx, rule, userID, transaction)
		if err != nil {
			continue // Log error in production
		}

		if alert != nil {
			ce.mu.Lock()
			ce.alerts = append(ce.alerts, *alert)
			ce.totalAlerts++
			ce.mu.Unlock()
		}
	}

	return nil
}

// executeRule executes a specific compliance rule
func (ce *ComplianceEngine) executeRule(ctx context.Context, rule *ComplianceRule, userID string, transaction *TransactionRecord) (*ComplianceAlert, error) {
	switch rule.Type {
	case "aml":
		return ce.executeAMLRule(rule, userID, transaction)
	case "velocity":
		return ce.executeVelocityRule(rule, userID, transaction)
	case "pattern":
		return ce.executePatternRule(rule, userID, transaction)
	default:
		return nil, fmt.Errorf("unknown rule type: %s", rule.Type)
	}
}

// executeAMLRule executes AML-specific rules
func (ce *ComplianceEngine) executeAMLRule(rule *ComplianceRule, userID string, transaction *TransactionRecord) (*ComplianceAlert, error) {
	switch rule.ID {
	case "large_transaction":
		threshold, ok := rule.Parameters["threshold_usd"].(float64)
		if !ok {
			return nil, fmt.Errorf("invalid threshold parameter")
		}

		if transaction.Amount.GreaterThan(decimal.NewFromFloat(threshold)) {
			return &ComplianceAlert{
				ID:          fmt.Sprintf("alert_%d", time.Now().UnixNano()),
				UserID:      userID,
				RuleID:      rule.ID,
				RuleName:    rule.Name,
				Severity:    rule.Severity,
				Status:      "open",
				Description: fmt.Sprintf("Large transaction detected: %s", transaction.Amount.String()),
				RiskScore:   decimal.NewFromFloat(70),
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
				Metadata: map[string]interface{}{
					"transaction_id": transaction.ID,
					"amount":         transaction.Amount.String(),
					"threshold":      threshold,
				},
			}, nil
		}
	}

	return nil, nil
}

// executeVelocityRule executes velocity-based rules
func (ce *ComplianceEngine) executeVelocityRule(rule *ComplianceRule, userID string, transaction *TransactionRecord) (*ComplianceAlert, error) {
	switch rule.ID {
	case "velocity_check":
		maxTxPerHour, ok1 := rule.Parameters["max_transactions_per_hour"].(float64)
		maxVolumePerHour, ok2 := rule.Parameters["max_volume_per_hour_usd"].(float64)

		if !ok1 || !ok2 {
			return nil, fmt.Errorf("invalid velocity parameters")
		}

		// Get user's transactions in the last hour
		hourAgo := time.Now().Add(-time.Hour)
		txCount := 0
		totalVolume := decimal.Zero

		for _, tx := range ce.transactions {
			if tx.UserID == userID && tx.Timestamp.After(hourAgo) {
				txCount++
				totalVolume = totalVolume.Add(tx.Amount)
			}
		}

		if txCount > int(maxTxPerHour) || totalVolume.GreaterThan(decimal.NewFromFloat(maxVolumePerHour)) {
			return &ComplianceAlert{
				ID:          fmt.Sprintf("alert_%d", time.Now().UnixNano()),
				UserID:      userID,
				RuleID:      rule.ID,
				RuleName:    rule.Name,
				Severity:    rule.Severity,
				Status:      "open",
				Description: fmt.Sprintf("High velocity detected: %d transactions, %s volume in last hour", txCount, totalVolume.String()),
				RiskScore:   decimal.NewFromFloat(85),
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
				Metadata: map[string]interface{}{
					"transaction_count": txCount,
					"total_volume":      totalVolume.String(),
					"timeframe":         "1 hour",
				},
			}, nil
		}
	}

	return nil, nil
}

// executePatternRule executes pattern detection rules
func (ce *ComplianceEngine) executePatternRule(rule *ComplianceRule, userID string, transaction *TransactionRecord) (*ComplianceAlert, error) {
	switch rule.ID {
	case "structuring_detection":
		return ce.detectStructuring(rule, userID, transaction)
	case "round_amount_pattern":
		return ce.detectRoundAmountPattern(rule, userID, transaction)
	case "rapid_deposit_withdrawal":
		return ce.detectRapidDepositWithdrawal(rule, userID, transaction)
	}

	return nil, nil
}

// detectStructuring detects potential transaction structuring
func (ce *ComplianceEngine) detectStructuring(rule *ComplianceRule, userID string, transaction *TransactionRecord) (*ComplianceAlert, error) {
	threshold, ok1 := rule.Parameters["amount_threshold"].(float64)
	frequency, ok2 := rule.Parameters["frequency_threshold"].(float64)
	hours, ok3 := rule.Parameters["timeframe_hours"].(float64)

	if !ok1 || !ok2 || !ok3 {
		return nil, fmt.Errorf("invalid structuring parameters")
	}

	timeFrame := time.Duration(hours) * time.Hour
	cutoff := time.Now().Add(-timeFrame)

	// Find transactions just below threshold
	suspiciousTransactions := make([]TransactionRecord, 0)
	for _, tx := range ce.transactions {
		if tx.UserID == userID && tx.Timestamp.After(cutoff) {
			amount := tx.Amount.InexactFloat64()
			if amount > threshold*0.8 && amount < threshold {
				suspiciousTransactions = append(suspiciousTransactions, tx)
			}
		}
	}

	if len(suspiciousTransactions) >= int(frequency) {
		pattern := TransactionPattern{
			PatternType:  "structuring",
			Description:  "Multiple transactions just below reporting threshold",
			Confidence:   decimal.NewFromFloat(0.85),
			RiskScore:    decimal.NewFromFloat(95),
			DetectedAt:   time.Now(),
			Transactions: suspiciousTransactions,
		}

		return &ComplianceAlert{
			ID:               fmt.Sprintf("alert_%d", time.Now().UnixNano()),
			UserID:           userID,
			RuleID:           rule.ID,
			RuleName:         rule.Name,
			Severity:         rule.Severity,
			Status:           "open",
			Description:      fmt.Sprintf("Potential structuring detected: %d transactions below threshold", len(suspiciousTransactions)),
			DetectedPatterns: []TransactionPattern{pattern},
			RiskScore:        decimal.NewFromFloat(95),
			CreatedAt:        time.Now(),
			UpdatedAt:        time.Now(),
		}, nil
	}

	return nil, nil
}

// detectRoundAmountPattern detects patterns of round-number transactions
func (ce *ComplianceEngine) detectRoundAmountPattern(rule *ComplianceRule, userID string, transaction *TransactionRecord) (*ComplianceAlert, error) {
	roundThreshold, ok1 := rule.Parameters["round_threshold"].(float64)
	frequency, ok2 := rule.Parameters["frequency_threshold"].(float64)
	hours, ok3 := rule.Parameters["timeframe_hours"].(float64)

	if !ok1 || !ok2 || !ok3 {
		return nil, fmt.Errorf("invalid round amount parameters")
	}

	timeFrame := time.Duration(hours) * time.Hour
	cutoff := time.Now().Add(-timeFrame)

	roundTransactions := make([]TransactionRecord, 0)
	for _, tx := range ce.transactions {
		if tx.UserID == userID && tx.Timestamp.After(cutoff) {
			amount := tx.Amount.InexactFloat64()
			if math.Mod(amount, roundThreshold) == 0 && amount >= roundThreshold {
				roundTransactions = append(roundTransactions, tx)
			}
		}
	}

	if len(roundTransactions) >= int(frequency) {
		return &ComplianceAlert{
			ID:          fmt.Sprintf("alert_%d", time.Now().UnixNano()),
			UserID:      userID,
			RuleID:      rule.ID,
			RuleName:    rule.Name,
			Severity:    rule.Severity,
			Status:      "open",
			Description: fmt.Sprintf("Round amount pattern detected: %d round transactions", len(roundTransactions)),
			RiskScore:   decimal.NewFromFloat(60),
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}, nil
	}

	return nil, nil
}

// detectRapidDepositWithdrawal detects rapid deposit followed by withdrawal
func (ce *ComplianceEngine) detectRapidDepositWithdrawal(rule *ComplianceRule, userID string, transaction *TransactionRecord) (*ComplianceAlert, error) {
	maxGapMinutes, ok1 := rule.Parameters["max_time_gap_minutes"].(float64)
	minRatio, ok2 := rule.Parameters["min_amount_ratio"].(float64)

	if !ok1 || !ok2 {
		return nil, fmt.Errorf("invalid rapid deposit-withdrawal parameters")
	}

	if transaction.Type != "withdrawal" {
		return nil, nil
	}

	// Look for recent deposits
	maxGap := time.Duration(maxGapMinutes) * time.Minute
	cutoff := transaction.Timestamp.Add(-maxGap)

	for _, tx := range ce.transactions {
		if tx.UserID == userID && tx.Type == "deposit" && tx.Timestamp.After(cutoff) {
			ratio := transaction.Amount.Div(tx.Amount).InexactFloat64()
			if ratio >= minRatio {
				return &ComplianceAlert{
					ID:          fmt.Sprintf("alert_%d", time.Now().UnixNano()),
					UserID:      userID,
					RuleID:      rule.ID,
					RuleName:    rule.Name,
					Severity:    rule.Severity,
					Status:      "open",
					Description: fmt.Sprintf("Rapid deposit-withdrawal detected: %s deposit followed by %s withdrawal", tx.Amount.String(), transaction.Amount.String()),
					RiskScore:   decimal.NewFromFloat(80),
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
					Metadata: map[string]interface{}{
						"deposit_id":       tx.ID,
						"withdrawal_id":    transaction.ID,
						"time_gap_minutes": transaction.Timestamp.Sub(tx.Timestamp).Minutes(),
						"amount_ratio":     ratio,
					},
				}, nil
			}
		}
	}

	return nil, nil
}

// GetActiveAlerts returns all open compliance alerts
func (ce *ComplianceEngine) GetActiveAlerts(ctx context.Context) ([]ComplianceAlert, error) {
	ce.mu.RLock()
	defer ce.mu.RUnlock()

	activeAlerts := make([]ComplianceAlert, 0)
	for _, alert := range ce.alerts {
		if alert.Status == "open" || alert.Status == "investigating" {
			activeAlerts = append(activeAlerts, alert)
		}
	}

	return activeAlerts, nil
}

// UpdateAlertStatus updates the status of a compliance alert
func (ce *ComplianceEngine) UpdateAlertStatus(ctx context.Context, alertID, status, assignedTo, notes string) error {
	ce.mu.Lock()
	defer ce.mu.Unlock()

	for i, alert := range ce.alerts {
		if alert.ID == alertID {
			ce.alerts[i].Status = status
			ce.alerts[i].AssignedTo = assignedTo
			ce.alerts[i].ResolutionNotes = notes
			ce.alerts[i].UpdatedAt = time.Now()
			return nil
		}
	}

	return fmt.Errorf("alert not found: %s", alertID)
}

// AddRule adds a new compliance rule
func (ce *ComplianceEngine) AddRule(ctx context.Context, rule *ComplianceRule) error {
	ce.mu.Lock()
	defer ce.mu.Unlock()

	rule.CreatedAt = time.Now()
	rule.UpdatedAt = time.Now()
	ce.rules[rule.ID] = rule

	return nil
}

// GetPerformanceMetrics returns compliance engine performance metrics
func (ce *ComplianceEngine) GetPerformanceMetrics() map[string]interface{} {
	ce.mu.RLock()
	defer ce.mu.RUnlock()

	return map[string]interface{}{
		"total_checks":       ce.totalChecks,
		"total_alerts":       ce.totalAlerts,
		"average_check_time": ce.averageCheckTime.String(),
		"active_rules":       len(ce.rules),
		"total_transactions": len(ce.transactions),
		"alert_rate":         float64(ce.totalAlerts) / math.Max(float64(ce.totalChecks), 1),
	}
}
