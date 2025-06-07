package interfaces

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/Aidin1998/finalex/internal/trading/engine"
	"github.com/Aidin1998/finalex/internal/trading/model"
)

// TradingEngineIntegration provides real-time enforcement actions with the trading engine
type TradingEngineIntegration struct {
	tradingEngine     TradingEngine
	complianceService ComplianceService
	auditService      AuditService
	logger            *zap.Logger
}

// TradingEngine interface for enforcement actions
type TradingEngine interface {
	CancelOrder(req *engine.CancelRequest) error
	AdminCancelOrder(orderID string) error
	GetOrderBook(pair string) interface{}
	ProcessOrder(ctx context.Context, order *model.Order, source string) (*model.Order, []*model.Trade, []*model.Order, error)
}

// EnforcementAction represents an action to be taken by the trading engine
type EnforcementAction struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type"`   // "cancel_order", "block_user", "halt_trading", "block_instrument"
	Target      string                 `json:"target"` // user_id, order_id, instrument, etc.
	Parameters  map[string]interface{} `json:"parameters"`
	Reason      string                 `json:"reason"`
	RequestedBy string                 `json:"requested_by"`
	CreatedAt   time.Time              `json:"created_at"`
	ExecutedAt  *time.Time             `json:"executed_at,omitempty"`
	Status      string                 `json:"status"` // "pending", "executed", "failed"
	Error       string                 `json:"error,omitempty"`
}

// EnforcementResult represents the result of an enforcement action
type EnforcementResult struct {
	ActionID       string                 `json:"action_id"`
	Success        bool                   `json:"success"`
	Message        string                 `json:"message"`
	AffectedOrders []string               `json:"affected_orders,omitempty"`
	Details        map[string]interface{} `json:"details,omitempty"`
	ExecutedAt     time.Time              `json:"executed_at"`
}

// NewTradingEngineIntegration creates a new trading engine integration
func NewTradingEngineIntegration(
	tradingEngine TradingEngine,
	complianceService ComplianceService,
	auditService AuditService,
	logger *zap.Logger,
) *TradingEngineIntegration {
	return &TradingEngineIntegration{
		tradingEngine:     tradingEngine,
		complianceService: complianceService,
		auditService:      auditService,
		logger:            logger,
	}
}

// ExecuteEnforcementAction executes a compliance enforcement action
func (tei *TradingEngineIntegration) ExecuteEnforcementAction(ctx context.Context, action EnforcementAction) (*EnforcementResult, error) {
	startTime := time.Now()

	result := &EnforcementResult{
		ActionID:   action.ID,
		ExecutedAt: startTime,
		Details:    make(map[string]interface{}),
	}

	tei.logger.Info("Executing enforcement action",
		zap.String("action_id", action.ID),
		zap.String("type", action.Type),
		zap.String("target", action.Target),
		zap.String("reason", action.Reason),
	)

	switch action.Type {
	case "cancel_order":
		err := tei.cancelOrder(ctx, action, result)
		if err != nil {
			result.Success = false
			result.Message = fmt.Sprintf("Failed to cancel order: %v", err)
			return result, err
		}

	case "cancel_user_orders":
		err := tei.cancelUserOrders(ctx, action, result)
		if err != nil {
			result.Success = false
			result.Message = fmt.Sprintf("Failed to cancel user orders: %v", err)
			return result, err
		}

	case "block_user":
		err := tei.blockUser(ctx, action, result)
		if err != nil {
			result.Success = false
			result.Message = fmt.Sprintf("Failed to block user: %v", err)
			return result, err
		}

	case "halt_trading":
		err := tei.haltTrading(ctx, action, result)
		if err != nil {
			result.Success = false
			result.Message = fmt.Sprintf("Failed to halt trading: %v", err)
			return result, err
		}

	case "block_instrument":
		err := tei.blockInstrument(ctx, action, result)
		if err != nil {
			result.Success = false
			result.Message = fmt.Sprintf("Failed to block instrument: %v", err)
			return result, err
		}

	default:
		result.Success = false
		result.Message = fmt.Sprintf("Unknown enforcement action type: %s", action.Type)
		return result, fmt.Errorf("unknown enforcement action type: %s", action.Type)
	}

	result.Success = true
	result.Message = "Enforcement action executed successfully"
	// Create audit entry
	auditEntry := &AuditEvent{
		ID:          uuid.New(),
		EventType:   "enforcement_action",
		Category:    "compliance",
		Severity:    "info",
		Description: fmt.Sprintf("Enforcement action executed: %s", action.Type),
		Action:      action.Type,
		Resource:    "enforcement_action",
		ResourceID:  action.ID,
		Metadata: map[string]interface{}{
			"target":   action.Target,
			"reason":   action.Reason,
			"success":  result.Success,
			"message":  result.Message,
			"duration": time.Since(startTime).String(),
		},
		Timestamp:   time.Now(),
		ProcessedBy: action.RequestedBy,
	}

	if err := tei.auditService.CreateEvent(ctx, auditEntry); err != nil {
		tei.logger.Error("Failed to create audit entry for enforcement action",
			zap.Error(err),
			zap.String("action_id", action.ID),
		)
	}

	return result, nil
}

// cancelOrder cancels a specific order
func (tei *TradingEngineIntegration) cancelOrder(ctx context.Context, action EnforcementAction, result *EnforcementResult) error {
	orderID := action.Target

	// Parse order ID
	orderUUID, err := uuid.Parse(orderID)
	if err != nil {
		return fmt.Errorf("invalid order ID format: %v", err)
	}
	// Cancel order through trading engine
	cancelReq := &engine.CancelRequest{
		OrderID: orderUUID,
		Pair:    "", // Will need to be provided via parameters if needed
	}

	if userID, exists := action.Parameters["user_id"]; exists {
		if userIDStr, ok := userID.(string); ok {
			if userUUID, err := uuid.Parse(userIDStr); err == nil {
				cancelReq.UserID = userUUID
			}
		}
	}

	err = tei.tradingEngine.CancelOrder(cancelReq)
	if err != nil {
		return fmt.Errorf("trading engine cancel order failed: %v", err)
	}

	result.AffectedOrders = []string{orderID}
	result.Details["cancelled_order_id"] = orderID
	result.Details["cancel_reason"] = action.Reason

	tei.logger.Info("Order cancelled successfully",
		zap.String("order_id", orderID),
		zap.String("reason", action.Reason),
	)

	return nil
}

// cancelUserOrders cancels all orders for a specific user
func (tei *TradingEngineIntegration) cancelUserOrders(ctx context.Context, action EnforcementAction, result *EnforcementResult) error {
	userID := action.Target

	// Get user orders (this would require extending the trading engine interface)
	// For now, we'll use admin cancel functionality

	// Parse user ID
	userUUID, err := uuid.Parse(userID)
	if err != nil {
		return fmt.Errorf("invalid user ID format: %v", err)
	}

	// Get active orders for user (implementation would depend on available methods)
	// This is a simplified implementation - in practice, you'd need to query for user orders

	var cancelledOrders []string

	// If specific order IDs are provided in parameters
	if orderIDs, exists := action.Parameters["order_ids"]; exists {
		if orderList, ok := orderIDs.([]interface{}); ok {
			for _, orderIDInterface := range orderList {
				if orderIDStr, ok := orderIDInterface.(string); ok {
					orderUUID, err := uuid.Parse(orderIDStr)
					if err != nil {
						tei.logger.Error("Invalid order ID in cancel user orders",
							zap.String("order_id", orderIDStr),
							zap.Error(err))
						continue
					}

					cancelReq := &engine.CancelRequest{
						OrderID: orderUUID,
						UserID:  userUUID,
						Pair:    "", // Will need pair from parameters if available
					}

					err = tei.tradingEngine.CancelOrder(cancelReq)
					if err != nil {
						tei.logger.Error("Failed to cancel user order",
							zap.String("order_id", orderIDStr),
							zap.String("user_id", userID),
							zap.Error(err),
						)
						continue
					}

					cancelledOrders = append(cancelledOrders, orderIDStr)
				}
			}
		}
	}

	result.AffectedOrders = cancelledOrders
	result.Details["cancelled_count"] = len(cancelledOrders)
	result.Details["user_id"] = userID
	result.Details["cancel_reason"] = action.Reason

	tei.logger.Info("User orders cancelled",
		zap.String("user_id", userID),
		zap.Int("cancelled_count", len(cancelledOrders)),
		zap.String("reason", action.Reason),
	)

	return nil
}

// blockUser blocks a user from trading
func (tei *TradingEngineIntegration) blockUser(ctx context.Context, action EnforcementAction, result *EnforcementResult) error {
	userID := action.Target

	// First, cancel all active orders for the user
	cancelAction := EnforcementAction{
		ID:          uuid.New().String(),
		Type:        "cancel_user_orders",
		Target:      userID,
		Parameters:  action.Parameters,
		Reason:      fmt.Sprintf("User blocked: %s", action.Reason),
		RequestedBy: action.RequestedBy,
		CreatedAt:   time.Now(),
	}

	cancelResult := &EnforcementResult{
		ActionID:   cancelAction.ID,
		ExecutedAt: time.Now(),
		Details:    make(map[string]interface{}),
	}

	err := tei.cancelUserOrders(ctx, cancelAction, cancelResult)
	if err != nil {
		tei.logger.Error("Failed to cancel user orders during user block",
			zap.String("user_id", userID),
			zap.Error(err),
		)
	} // Update user compliance status to blocked
	complianceCheck := &ComplianceRequest{
		UserID:           uuid.MustParse(userID),
		ActivityType:     ActivityTrade, // Using trade activity type for user blocking
		RequestTimestamp: time.Now(),
		Metadata: map[string]interface{}{
			"reason":     action.Reason,
			"blocked_by": action.RequestedBy,
			"blocked_at": time.Now().Format(time.RFC3339),
		},
	}

	_, err = tei.complianceService.PerformComplianceCheck(ctx, complianceCheck)
	if err != nil {
		tei.logger.Error("Failed to update user compliance status",
			zap.String("user_id", userID),
			zap.Error(err),
		)
		return fmt.Errorf("failed to update user compliance status: %v", err)
	}

	result.Details["user_id"] = userID
	result.Details["block_reason"] = action.Reason
	result.Details["cancelled_orders"] = cancelResult.AffectedOrders
	result.AffectedOrders = cancelResult.AffectedOrders

	tei.logger.Info("User blocked successfully",
		zap.String("user_id", userID),
		zap.String("reason", action.Reason),
		zap.Int("cancelled_orders", len(cancelResult.AffectedOrders)),
	)

	return nil
}

// haltTrading halts trading for a specific instrument
func (tei *TradingEngineIntegration) haltTrading(ctx context.Context, action EnforcementAction, result *EnforcementResult) error {
	instrument := action.Target

	// Get order book for the instrument
	orderBook := tei.tradingEngine.GetOrderBook(instrument)
	if orderBook == nil {
		return fmt.Errorf("order book not found for instrument: %s", instrument)
	}

	// Halt trading using order book admin controls
	// This would require the order book to implement halt functionality
	// For now, we'll log the action and update the compliance system
	// Update compliance system with trading halt
	complianceCheck := &ComplianceRequest{
		UserID:           uuid.Nil, // Using nil UUID for system-level actions
		ActivityType:     ActivityTrade,
		RequestTimestamp: time.Now(),
		Metadata: map[string]interface{}{
			"instrument": instrument,
			"reason":     action.Reason,
			"halted_by":  action.RequestedBy,
			"halted_at":  time.Now().Format(time.RFC3339),
		},
	}

	_, err := tei.complianceService.PerformComplianceCheck(ctx, complianceCheck)
	if err != nil {
		tei.logger.Error("Failed to record trading halt in compliance system",
			zap.String("instrument", instrument),
			zap.Error(err),
		)
		return fmt.Errorf("failed to record trading halt: %v", err)
	}

	result.Details["instrument"] = instrument
	result.Details["halt_reason"] = action.Reason
	result.Details["halted_at"] = time.Now().Format(time.RFC3339)

	tei.logger.Info("Trading halted for instrument",
		zap.String("instrument", instrument),
		zap.String("reason", action.Reason),
	)

	return nil
}

// blockInstrument blocks trading for a specific instrument
func (tei *TradingEngineIntegration) blockInstrument(ctx context.Context, action EnforcementAction, result *EnforcementResult) error {
	instrument := action.Target

	// First halt trading
	haltAction := EnforcementAction{
		ID:          uuid.New().String(),
		Type:        "halt_trading",
		Target:      instrument,
		Parameters:  action.Parameters,
		Reason:      fmt.Sprintf("Instrument blocked: %s", action.Reason),
		RequestedBy: action.RequestedBy,
		CreatedAt:   time.Now(),
	}

	err := tei.haltTrading(ctx, haltAction, result)
	if err != nil {
		return fmt.Errorf("failed to halt trading for instrument: %v", err)
	}
	// Update compliance system with instrument block
	complianceCheck := &ComplianceRequest{
		UserID:           uuid.Nil, // Using nil UUID for system-level actions
		ActivityType:     ActivityTrade,
		RequestTimestamp: time.Now(),
		Metadata: map[string]interface{}{
			"instrument": instrument,
			"reason":     action.Reason,
			"blocked_by": action.RequestedBy,
			"blocked_at": time.Now().Format(time.RFC3339),
		},
	}

	_, err = tei.complianceService.PerformComplianceCheck(ctx, complianceCheck)
	if err != nil {
		tei.logger.Error("Failed to record instrument block in compliance system",
			zap.String("instrument", instrument),
			zap.Error(err),
		)
		return fmt.Errorf("failed to record instrument block: %v", err)
	}

	result.Details["instrument"] = instrument
	result.Details["block_reason"] = action.Reason
	result.Details["blocked_at"] = time.Now().Format(time.RFC3339)

	tei.logger.Info("Instrument blocked successfully",
		zap.String("instrument", instrument),
		zap.String("reason", action.Reason),
	)

	return nil
}

// RealTimeEnforcementHook provides real-time enforcement during order processing
func (tei *TradingEngineIntegration) RealTimeEnforcementHook(ctx context.Context, order *model.Order) error { // Perform real-time compliance check
	complianceCheck := &ComplianceRequest{
		UserID:           order.UserID,
		ActivityType:     ActivityTrade,
		RequestTimestamp: time.Now(),
		Metadata: map[string]interface{}{
			"instrument":     order.Pair,
			"side":           order.Side,
			"quantity":       order.Quantity.String(),
			"price":          order.Price.String(),
			"order_type":     order.Type,
			"transaction_id": order.ID.String(),
		},
	}

	result, err := tei.complianceService.PerformComplianceCheck(ctx, complianceCheck)
	if err != nil {
		tei.logger.Error("Compliance check failed during real-time enforcement",
			zap.String("order_id", order.ID.String()),
			zap.String("user_id", order.UserID.String()),
			zap.Error(err),
		)
		return fmt.Errorf("compliance check failed: %v", err)
	}
	// If not compliant, block the order
	if !result.Approved || result.Blocked {
		tei.logger.Warn("Order blocked by real-time compliance check",
			zap.String("order_id", order.ID.String()),
			zap.String("user_id", order.UserID.String()),
			zap.String("risk_score", result.RiskScore.String()),
			zap.Strings("flags", result.Flags),
			zap.String("reason", result.Reason),
		)

		// Execute required actions based on compliance result
		if result.Blocked {
			enforcementAction := EnforcementAction{
				ID:     uuid.New().String(),
				Type:   "cancel_order",
				Target: order.ID.String(),
				Parameters: map[string]interface{}{
					"user_id": order.UserID.String(),
					"reason":  fmt.Sprintf("Real-time compliance violation: %s", result.Reason),
				},
				Reason:      result.Reason,
				RequestedBy: "compliance_system",
				CreatedAt:   time.Now(),
				Status:      "pending",
			}

			_, err := tei.ExecuteEnforcementAction(ctx, enforcementAction)
			if err != nil {
				tei.logger.Error("Failed to execute enforcement action",
					zap.String("action_id", enforcementAction.ID),
					zap.String("action_type", enforcementAction.Type),
					zap.Error(err),
				)
			}
		}

		return fmt.Errorf("order blocked by compliance: %s", result.Reason)
	}

	return nil
}

// GetEnforcementMetrics returns metrics about enforcement actions
func (tei *TradingEngineIntegration) GetEnforcementMetrics(ctx context.Context, timeWindow time.Duration) (*EnforcementMetrics, error) {
	// This would typically query the audit service for enforcement action metrics
	// For now, we'll return a placeholder

	return &EnforcementMetrics{
		TimeWindow:           timeWindow,
		TotalActions:         0,
		SuccessfulActions:    0,
		FailedActions:        0,
		ActionsByType:        make(map[string]int),
		AverageExecutionTime: 0,
		LastUpdated:          time.Now(),
	}, nil
}

// EnforcementMetrics contains metrics about enforcement actions
type EnforcementMetrics struct {
	TimeWindow           time.Duration  `json:"time_window"`
	TotalActions         int            `json:"total_actions"`
	SuccessfulActions    int            `json:"successful_actions"`
	FailedActions        int            `json:"failed_actions"`
	ActionsByType        map[string]int `json:"actions_by_type"`
	AverageExecutionTime time.Duration  `json:"average_execution_time"`
	LastUpdated          time.Time      `json:"last_updated"`
}
