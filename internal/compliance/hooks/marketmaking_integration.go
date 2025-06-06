package hooks

import (
	"context"
	"fmt"

	"go.uber.org/zap"
)

// MarketMakingIntegration handles compliance integration for market making operations
type MarketMakingIntegration struct {
	hookManager *HookManager
	logger      *zap.Logger
}

// NewMarketMakingIntegration creates a new market making integration
func NewMarketMakingIntegration(hookManager *HookManager, logger *zap.Logger) *MarketMakingIntegration {
	return &MarketMakingIntegration{
		hookManager: hookManager,
		logger:      logger,
	}
}

// OnStrategyActivation handles strategy activation events
func (m *MarketMakingIntegration) OnStrategyActivation(ctx context.Context, userID string, strategyID string, strategy MMStrategy) error {
	event := MMStrategyEvent{
		BaseEvent: BaseEvent{
			Type:      EventTypeMMStrategy,
			UserID:    userID,
			Timestamp: getCurrentTimestamp(),
			Module:    ModuleMarketMaking,
		},
		Action:     "activate",
		StrategyID: strategyID,
		Strategy:   strategy,
	}

	m.logger.Info("Processing strategy activation",
		zap.String("user_id", userID),
		zap.String("strategy_id", strategyID),
		zap.String("strategy_type", strategy.Type),
		zap.String("symbol", strategy.Symbol),
	)

	if err := m.hookManager.TriggerHooks(ctx, event); err != nil {
		m.logger.Error("Failed to trigger strategy activation hooks",
			zap.Error(err),
			zap.String("user_id", userID),
			zap.String("strategy_id", strategyID),
		)
		return fmt.Errorf("failed to process strategy activation compliance: %w", err)
	}

	return nil
}

// OnStrategyDeactivation handles strategy deactivation events
func (m *MarketMakingIntegration) OnStrategyDeactivation(ctx context.Context, userID string, strategyID string, reason string) error {
	event := MMStrategyEvent{
		BaseEvent: BaseEvent{
			Type:      EventTypeMMStrategy,
			UserID:    userID,
			Timestamp: getCurrentTimestamp(),
			Module:    ModuleMarketMaking,
		},
		Action:     "deactivate",
		StrategyID: strategyID,
		Reason:     reason,
	}

	m.logger.Info("Processing strategy deactivation",
		zap.String("user_id", userID),
		zap.String("strategy_id", strategyID),
		zap.String("reason", reason),
	)

	if err := m.hookManager.TriggerHooks(ctx, event); err != nil {
		m.logger.Error("Failed to trigger strategy deactivation hooks",
			zap.Error(err),
			zap.String("user_id", userID),
			zap.String("strategy_id", strategyID),
		)
		return fmt.Errorf("failed to process strategy deactivation compliance: %w", err)
	}

	return nil
}

// OnQuoteUpdate handles quote update events
func (m *MarketMakingIntegration) OnQuoteUpdate(ctx context.Context, userID string, quote MMQuote) error {
	event := MMQuoteEvent{
		BaseEvent: BaseEvent{
			Type:      EventTypeMMQuote,
			UserID:    userID,
			Timestamp: getCurrentTimestamp(),
			Module:    ModuleMarketMaking,
		},
		Quote: quote,
	}

	m.logger.Info("Processing quote update",
		zap.String("user_id", userID),
		zap.String("symbol", quote.Symbol),
		zap.Float64("bid_price", quote.BidPrice),
		zap.Float64("ask_price", quote.AskPrice),
		zap.Float64("bid_size", quote.BidSize),
		zap.Float64("ask_size", quote.AskSize),
	)

	if err := m.hookManager.TriggerHooks(ctx, event); err != nil {
		m.logger.Error("Failed to trigger quote update hooks",
			zap.Error(err),
			zap.String("user_id", userID),
			zap.String("symbol", quote.Symbol),
		)
		return fmt.Errorf("failed to process quote update compliance: %w", err)
	}

	return nil
}

// OnOrderPlacement handles order placement events from market making
func (m *MarketMakingIntegration) OnOrderPlacement(ctx context.Context, userID string, order MMOrder) error {
	event := MMOrderEvent{
		BaseEvent: BaseEvent{
			Type:      EventTypeMMOrder,
			UserID:    userID,
			Timestamp: getCurrentTimestamp(),
			Module:    ModuleMarketMaking,
		},
		Action: "place",
		Order:  order,
	}

	m.logger.Info("Processing MM order placement",
		zap.String("user_id", userID),
		zap.String("order_id", order.OrderID),
		zap.String("symbol", order.Symbol),
		zap.String("side", order.Side),
		zap.Float64("price", order.Price),
		zap.Float64("quantity", order.Quantity),
		zap.String("strategy_id", order.StrategyID),
	)

	if err := m.hookManager.TriggerHooks(ctx, event); err != nil {
		m.logger.Error("Failed to trigger MM order placement hooks",
			zap.Error(err),
			zap.String("user_id", userID),
			zap.String("order_id", order.OrderID),
		)
		return fmt.Errorf("failed to process MM order placement compliance: %w", err)
	}

	return nil
}

// OnOrderCancellation handles order cancellation events from market making
func (m *MarketMakingIntegration) OnOrderCancellation(ctx context.Context, userID string, orderID string, reason string) error {
	event := MMOrderEvent{
		BaseEvent: BaseEvent{
			Type:      EventTypeMMOrder,
			UserID:    userID,
			Timestamp: getCurrentTimestamp(),
			Module:    ModuleMarketMaking,
		},
		Action:  "cancel",
		OrderID: orderID,
		Reason:  reason,
	}

	m.logger.Info("Processing MM order cancellation",
		zap.String("user_id", userID),
		zap.String("order_id", orderID),
		zap.String("reason", reason),
	)

	if err := m.hookManager.TriggerHooks(ctx, event); err != nil {
		m.logger.Error("Failed to trigger MM order cancellation hooks",
			zap.Error(err),
			zap.String("user_id", userID),
			zap.String("order_id", orderID),
		)
		return fmt.Errorf("failed to process MM order cancellation compliance: %w", err)
	}

	return nil
}

// OnInventoryRebalancing handles inventory rebalancing events
func (m *MarketMakingIntegration) OnInventoryRebalancing(ctx context.Context, userID string, rebalancing MMInventoryRebalancing) error {
	event := MMInventoryEvent{
		BaseEvent: BaseEvent{
			Type:      EventTypeMMInventory,
			UserID:    userID,
			Timestamp: getCurrentTimestamp(),
			Module:    ModuleMarketMaking,
		},
		Rebalancing: rebalancing,
	}

	m.logger.Info("Processing inventory rebalancing",
		zap.String("user_id", userID),
		zap.String("symbol", rebalancing.Symbol),
		zap.Float64("current_inventory", rebalancing.CurrentInventory),
		zap.Float64("target_inventory", rebalancing.TargetInventory),
		zap.String("action", rebalancing.Action),
	)

	if err := m.hookManager.TriggerHooks(ctx, event); err != nil {
		m.logger.Error("Failed to trigger inventory rebalancing hooks",
			zap.Error(err),
			zap.String("user_id", userID),
			zap.String("symbol", rebalancing.Symbol),
		)
		return fmt.Errorf("failed to process inventory rebalancing compliance: %w", err)
	}

	return nil
}

// OnRiskLimitBreach handles risk limit breach events
func (m *MarketMakingIntegration) OnRiskLimitBreach(ctx context.Context, userID string, riskBreach MMRiskBreach) error {
	event := MMRiskEvent{
		BaseEvent: BaseEvent{
			Type:      EventTypeMMRisk,
			UserID:    userID,
			Timestamp: getCurrentTimestamp(),
			Module:    ModuleMarketMaking,
		},
		RiskBreach: riskBreach,
	}

	m.logger.Warn("Processing risk limit breach",
		zap.String("user_id", userID),
		zap.String("risk_type", riskBreach.RiskType),
		zap.Float64("current_value", riskBreach.CurrentValue),
		zap.Float64("limit", riskBreach.Limit),
		zap.String("action_taken", riskBreach.ActionTaken),
	)

	if err := m.hookManager.TriggerHooks(ctx, event); err != nil {
		m.logger.Error("Failed to trigger risk limit breach hooks",
			zap.Error(err),
			zap.String("user_id", userID),
			zap.String("risk_type", riskBreach.RiskType),
		)
		return fmt.Errorf("failed to process risk limit breach compliance: %w", err)
	}

	return nil
}

// OnPnLAlert handles P&L alert events
func (m *MarketMakingIntegration) OnPnLAlert(ctx context.Context, userID string, pnlAlert MMPnLAlert) error {
	event := MMPnLEvent{
		BaseEvent: BaseEvent{
			Type:      EventTypeMMPnL,
			UserID:    userID,
			Timestamp: getCurrentTimestamp(),
			Module:    ModuleMarketMaking,
		},
		PnLAlert: pnlAlert,
	}

	m.logger.Info("Processing P&L alert",
		zap.String("user_id", userID),
		zap.String("symbol", pnlAlert.Symbol),
		zap.Float64("realized_pnl", pnlAlert.RealizedPnL),
		zap.Float64("unrealized_pnl", pnlAlert.UnrealizedPnL),
		zap.String("alert_type", pnlAlert.AlertType),
	)

	if err := m.hookManager.TriggerHooks(ctx, event); err != nil {
		m.logger.Error("Failed to trigger P&L alert hooks",
			zap.Error(err),
			zap.String("user_id", userID),
			zap.String("symbol", pnlAlert.Symbol),
		)
		return fmt.Errorf("failed to process P&L alert compliance: %w", err)
	}

	return nil
}

// OnPerformanceReport handles performance report events
func (m *MarketMakingIntegration) OnPerformanceReport(ctx context.Context, userID string, report MMPerformanceReport) error {
	event := MMPerformanceEvent{
		BaseEvent: BaseEvent{
			Type:      EventTypeMMPerformance,
			UserID:    userID,
			Timestamp: getCurrentTimestamp(),
			Module:    ModuleMarketMaking,
		},
		Report: report,
	}

	m.logger.Info("Processing performance report",
		zap.String("user_id", userID),
		zap.String("period", report.Period),
		zap.Float64("total_pnl", report.TotalPnL),
		zap.Float64("sharpe_ratio", report.SharpeRatio),
		zap.Float64("max_drawdown", report.MaxDrawdown),
	)

	if err := m.hookManager.TriggerHooks(ctx, event); err != nil {
		m.logger.Error("Failed to trigger performance report hooks",
			zap.Error(err),
			zap.String("user_id", userID),
			zap.String("period", report.Period),
		)
		return fmt.Errorf("failed to process performance report compliance: %w", err)
	}

	return nil
}
