package rules

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/compliance/aml"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// RuleEngine manages AML compliance rules and their execution
type RuleEngine struct {
	mu     sync.RWMutex
	logger *zap.SugaredLogger

	// Active rules
	rules map[string]*Rule

	// Rule execution history
	executionHistory map[string]*RuleExecution

	// Rule performance metrics
	metrics map[string]*RuleMetrics
}

// Rule represents an AML compliance rule
type Rule struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Type        string                 `json:"type"`     // "transaction", "behavior", "velocity", "pattern"
	Category    string                 `json:"category"` // "structuring", "layering", "integration", "other"
	Priority    int                    `json:"priority"`
	IsActive    bool                   `json:"is_active"`
	Conditions  []Condition            `json:"conditions"`
	Actions     []Action               `json:"actions"`
	Thresholds  map[string]interface{} `json:"thresholds"`
	Parameters  map[string]interface{} `json:"parameters"`
	CreatedBy   uuid.UUID              `json:"created_by"`
	UpdatedBy   uuid.UUID              `json:"updated_by"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	Version     int                    `json:"version"`
}

// Condition represents a rule condition
type Condition struct {
	Field     string      `json:"field"`
	Operator  string      `json:"operator"` // "eq", "ne", "gt", "lt", "gte", "lte", "in", "contains"
	Value     interface{} `json:"value"`
	LogicalOp string      `json:"logical_op"` // "AND", "OR"
}

// Action represents an action to take when a rule is triggered
type Action struct {
	Type       string                 `json:"type"` // "alert", "block", "flag", "escalate", "report"
	Parameters map[string]interface{} `json:"parameters"`
	Priority   int                    `json:"priority"`
}

// RuleExecution tracks rule execution results
type RuleExecution struct {
	ID            string                 `json:"id"`
	RuleID        string                 `json:"rule_id"`
	UserID        uuid.UUID              `json:"user_id"`
	TransactionID string                 `json:"transaction_id"`
	Triggered     bool                   `json:"triggered"`
	Result        string                 `json:"result"`
	Actions       []string               `json:"actions_taken"`
	ExecutedAt    time.Time              `json:"executed_at"`
	Duration      time.Duration          `json:"duration"`
	Context       map[string]interface{} `json:"context"`
}

// RuleMetrics tracks rule performance
type RuleMetrics struct {
	RuleID          string        `json:"rule_id"`
	TotalExecutions int           `json:"total_executions"`
	TotalTriggers   int           `json:"total_triggers"`
	TriggerRate     float64       `json:"trigger_rate"`
	TruePositives   int           `json:"true_positives"`
	FalsePositives  int           `json:"false_positives"`
	Accuracy        float64       `json:"accuracy"`
	AverageExecTime time.Duration `json:"average_exec_time"`
	LastExecuted    time.Time     `json:"last_executed"`
}

// NewRuleEngine creates a new AML rule engine
func NewRuleEngine(logger *zap.SugaredLogger) *RuleEngine {
	engine := &RuleEngine{
		logger:           logger,
		rules:            make(map[string]*Rule),
		executionHistory: make(map[string]*RuleExecution),
		metrics:          make(map[string]*RuleMetrics),
	}

	// Initialize default AML rules
	engine.initializeDefaultRules()

	return engine
}

// initializeDefaultRules sets up standard AML detection rules
func (re *RuleEngine) initializeDefaultRules() {
	// Structuring detection rule
	structuringRule := &Rule{
		ID:          "structuring_detection_v1",
		Name:        "Structuring Detection",
		Description: "Detects potential structuring activities (transactions just below reporting thresholds)",
		Type:        "transaction",
		Category:    "structuring",
		Priority:    1,
		IsActive:    true,
		Conditions: []Condition{
			{
				Field:     "amount",
				Operator:  "gte",
				Value:     9000.0,
				LogicalOp: "AND",
			},
			{
				Field:     "amount",
				Operator:  "lt",
				Value:     10000.0,
				LogicalOp: "AND",
			},
		},
		Actions: []Action{
			{
				Type: "alert",
				Parameters: map[string]interface{}{
					"severity": "high",
					"message":  "Potential structuring activity detected",
				},
				Priority: 1,
			},
		},
		Thresholds: map[string]interface{}{
			"min_amount": 9000.0,
			"max_amount": 10000.0,
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Version:   1,
	}

	// Rapid movement rule
	rapidMovementRule := &Rule{
		ID:          "rapid_movement_v1",
		Name:        "Rapid Fund Movement",
		Description: "Detects rapid movement of funds across accounts",
		Type:        "velocity",
		Category:    "layering",
		Priority:    2,
		IsActive:    true,
		Conditions: []Condition{
			{
				Field:     "transaction_count_1h",
				Operator:  "gt",
				Value:     10,
				LogicalOp: "AND",
			},
			{
				Field:     "total_amount_1h",
				Operator:  "gt",
				Value:     50000.0,
				LogicalOp: "OR",
			},
		},
		Actions: []Action{
			{
				Type: "flag",
				Parameters: map[string]interface{}{
					"severity": "medium",
					"message":  "Rapid fund movement detected",
				},
				Priority: 2,
			},
		},
		Thresholds: map[string]interface{}{
			"max_transactions_per_hour": 10,
			"max_amount_per_hour":       50000.0,
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Version:   1,
	}

	// High-risk geography rule
	geoRiskRule := &Rule{
		ID:          "geographic_risk_v1",
		Name:        "High-Risk Geographic Activity",
		Description: "Flags transactions from high-risk jurisdictions",
		Type:        "transaction",
		Category:    "other",
		Priority:    3,
		IsActive:    true,
		Conditions: []Condition{
			{
				Field:     "country_risk_rating",
				Operator:  "gte",
				Value:     8,
				LogicalOp: "AND",
			},
			{
				Field:     "amount",
				Operator:  "gt",
				Value:     5000.0,
				LogicalOp: "AND",
			},
		},
		Actions: []Action{
			{
				Type: "alert",
				Parameters: map[string]interface{}{
					"severity": "medium",
					"message":  "Transaction from high-risk jurisdiction",
				},
				Priority: 3,
			},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Version:   1,
	}

	re.rules[structuringRule.ID] = structuringRule
	re.rules[rapidMovementRule.ID] = rapidMovementRule
	re.rules[geoRiskRule.ID] = geoRiskRule

	// Initialize metrics for each rule
	for ruleID := range re.rules {
		re.metrics[ruleID] = &RuleMetrics{
			RuleID: ruleID,
		}
	}
}

// ExecuteRules runs all active rules against the provided context
func (re *RuleEngine) ExecuteRules(ctx context.Context, userID uuid.UUID, transaction *aml.TransactionRecord) ([]*RuleExecution, error) {
	re.mu.Lock()
	defer re.mu.Unlock()

	var executions []*RuleExecution

	for _, rule := range re.rules {
		if !rule.IsActive {
			continue
		}

		execution := re.executeRule(ctx, rule, userID, transaction)
		executions = append(executions, execution)

		// Update metrics
		re.updateRuleMetrics(rule.ID, execution)
	}

	return executions, nil
}

// executeRule executes a single rule
func (re *RuleEngine) executeRule(ctx context.Context, rule *Rule, userID uuid.UUID, transaction *aml.TransactionRecord) *RuleExecution {
	start := time.Now()

	execution := &RuleExecution{
		ID:            uuid.New().String(),
		RuleID:        rule.ID,
		UserID:        userID,
		TransactionID: transaction.ID,
		ExecutedAt:    start,
		Context:       make(map[string]interface{}),
	}

	// Evaluate conditions
	triggered := re.evaluateConditions(rule.Conditions, transaction)
	execution.Triggered = triggered

	if triggered {
		// Execute actions
		for _, action := range rule.Actions {
			actionResult := re.executeAction(ctx, action, userID, transaction)
			execution.Actions = append(execution.Actions, actionResult)
		}
		execution.Result = "triggered"
	} else {
		execution.Result = "not_triggered"
	}

	execution.Duration = time.Since(start)
	re.executionHistory[execution.ID] = execution

	return execution
}

// evaluateConditions evaluates rule conditions against transaction data
func (re *RuleEngine) evaluateConditions(conditions []Condition, transaction *aml.TransactionRecord) bool {
	if len(conditions) == 0 {
		return false
	}

	result := true
	for i, condition := range conditions {
		conditionResult := re.evaluateCondition(condition, transaction)

		if i == 0 {
			result = conditionResult
		} else {
			switch condition.LogicalOp {
			case "AND":
				result = result && conditionResult
			case "OR":
				result = result || conditionResult
			}
		}
	}

	return result
}

// evaluateCondition evaluates a single condition
func (re *RuleEngine) evaluateCondition(condition Condition, transaction *aml.TransactionRecord) bool {
	var fieldValue interface{}

	// Extract field value from transaction
	switch condition.Field {
	case "amount":
		fieldValue = transaction.Amount.InexactFloat64()
	case "currency":
		fieldValue = transaction.Currency
	case "type":
		fieldValue = transaction.Type
	case "user_id":
		fieldValue = transaction.UserID
	default:
		// Check metadata
		if val, exists := transaction.Metadata[condition.Field]; exists {
			fieldValue = val
		} else {
			return false
		}
	}

	// Evaluate condition based on operator
	switch condition.Operator {
	case "eq":
		return fieldValue == condition.Value
	case "ne":
		return fieldValue != condition.Value
	case "gt":
		return compareNumbers(fieldValue, condition.Value) > 0
	case "lt":
		return compareNumbers(fieldValue, condition.Value) < 0
	case "gte":
		return compareNumbers(fieldValue, condition.Value) >= 0
	case "lte":
		return compareNumbers(fieldValue, condition.Value) <= 0
	default:
		return false
	}
}

// executeAction executes a rule action
func (re *RuleEngine) executeAction(ctx context.Context, action Action, userID uuid.UUID, transaction *aml.TransactionRecord) string {
	switch action.Type {
	case "alert":
		return re.createAlert(ctx, action, userID, transaction)
	case "block":
		return re.blockTransaction(ctx, action, userID, transaction)
	case "flag":
		return re.flagUser(ctx, action, userID, transaction)
	case "escalate":
		return re.escalateCase(ctx, action, userID, transaction)
	default:
		return "unknown_action"
	}
}

// Helper methods for actions
func (re *RuleEngine) createAlert(ctx context.Context, action Action, userID uuid.UUID, transaction *aml.TransactionRecord) string {
	// Implementation would create an alert in the system
	re.logger.Infow("Alert created", "user_id", userID, "transaction_id", transaction.ID, "action", action)
	return "alert_created"
}

func (re *RuleEngine) blockTransaction(ctx context.Context, action Action, userID uuid.UUID, transaction *aml.TransactionRecord) string {
	// Implementation would block the transaction
	re.logger.Infow("Transaction blocked", "user_id", userID, "transaction_id", transaction.ID, "action", action)
	return "transaction_blocked"
}

func (re *RuleEngine) flagUser(ctx context.Context, action Action, userID uuid.UUID, transaction *aml.TransactionRecord) string {
	// Implementation would flag the user account
	re.logger.Infow("User flagged", "user_id", userID, "transaction_id", transaction.ID, "action", action)
	return "user_flagged"
}

func (re *RuleEngine) escalateCase(ctx context.Context, action Action, userID uuid.UUID, transaction *aml.TransactionRecord) string {
	// Implementation would escalate to case management
	re.logger.Infow("Case escalated", "user_id", userID, "transaction_id", transaction.ID, "action", action)
	return "case_escalated"
}

// updateRuleMetrics updates performance metrics for a rule
func (re *RuleEngine) updateRuleMetrics(ruleID string, execution *RuleExecution) {
	metrics := re.metrics[ruleID]
	if metrics == nil {
		metrics = &RuleMetrics{RuleID: ruleID}
		re.metrics[ruleID] = metrics
	}

	metrics.TotalExecutions++
	if execution.Triggered {
		metrics.TotalTriggers++
	}

	if metrics.TotalExecutions > 0 {
		metrics.TriggerRate = float64(metrics.TotalTriggers) / float64(metrics.TotalExecutions)
	}

	// Update average execution time
	if metrics.TotalExecutions == 1 {
		metrics.AverageExecTime = execution.Duration
	} else {
		totalTime := time.Duration(int64(metrics.AverageExecTime) * int64(metrics.TotalExecutions-1))
		metrics.AverageExecTime = (totalTime + execution.Duration) / time.Duration(metrics.TotalExecutions)
	}

	metrics.LastExecuted = execution.ExecutedAt
}

// Helper function to compare numbers
func compareNumbers(a, b interface{}) int {
	aFloat := convertToFloat64(a)
	bFloat := convertToFloat64(b)

	if aFloat < bFloat {
		return -1
	} else if aFloat > bFloat {
		return 1
	}
	return 0
}

func convertToFloat64(val interface{}) float64 {
	switch v := val.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case decimal.Decimal:
		return v.InexactFloat64()
	default:
		return 0
	}
}

// GetRule retrieves a rule by ID
func (re *RuleEngine) GetRule(ruleID string) (*Rule, error) {
	re.mu.RLock()
	defer re.mu.RUnlock()

	rule, exists := re.rules[ruleID]
	if !exists {
		return nil, fmt.Errorf("rule not found: %s", ruleID)
	}

	return rule, nil
}

// AddRule adds a new rule to the engine
func (re *RuleEngine) AddRule(rule *Rule) error {
	re.mu.Lock()
	defer re.mu.Unlock()

	rule.ID = uuid.New().String()
	rule.CreatedAt = time.Now()
	rule.UpdatedAt = time.Now()
	rule.Version = 1

	re.rules[rule.ID] = rule
	re.metrics[rule.ID] = &RuleMetrics{RuleID: rule.ID}

	return nil
}

// UpdateRule updates an existing rule
func (re *RuleEngine) UpdateRule(ruleID string, updates *Rule) error {
	re.mu.Lock()
	defer re.mu.Unlock()

	rule, exists := re.rules[ruleID]
	if !exists {
		return fmt.Errorf("rule not found: %s", ruleID)
	}

	// Update fields
	rule.Name = updates.Name
	rule.Description = updates.Description
	rule.Type = updates.Type
	rule.Category = updates.Category
	rule.Priority = updates.Priority
	rule.IsActive = updates.IsActive
	rule.Conditions = updates.Conditions
	rule.Actions = updates.Actions
	rule.Thresholds = updates.Thresholds
	rule.Parameters = updates.Parameters
	rule.UpdatedBy = updates.UpdatedBy
	rule.UpdatedAt = time.Now()
	rule.Version++

	return nil
}

// DeleteRule removes a rule from the engine
func (re *RuleEngine) DeleteRule(ruleID string) error {
	re.mu.Lock()
	defer re.mu.Unlock()

	if _, exists := re.rules[ruleID]; !exists {
		return fmt.Errorf("rule not found: %s", ruleID)
	}

	delete(re.rules, ruleID)
	delete(re.metrics, ruleID)

	return nil
}

// GetRuleMetrics returns performance metrics for a rule
func (re *RuleEngine) GetRuleMetrics(ruleID string) (*RuleMetrics, error) {
	re.mu.RLock()
	defer re.mu.RUnlock()

	metrics, exists := re.metrics[ruleID]
	if !exists {
		return nil, fmt.Errorf("metrics not found for rule: %s", ruleID)
	}

	return metrics, nil
}

// ListActiveRules returns all active rules
func (re *RuleEngine) ListActiveRules() []*Rule {
	re.mu.RLock()
	defer re.mu.RUnlock()

	var activeRules []*Rule
	for _, rule := range re.rules {
		if rule.IsActive {
			activeRules = append(activeRules, rule)
		}
	}

	return activeRules
}
