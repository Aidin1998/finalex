// Package adapters provides adapter implementations for service contracts
package adapters

import (
	"context"

	"github.com/Aidin1998/finalex/internal/integration/contracts"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// TradingServiceAdapter implements contracts.TradingServiceContract
// by adapting the contracts.TradingServiceContract interface
type TradingServiceAdapter struct {
	service contracts.TradingServiceContract
	logger  *zap.Logger
}

// NewTradingServiceAdapter creates a new trading service adapter
func NewTradingServiceAdapter(service contracts.TradingServiceContract, logger *zap.Logger) *TradingServiceAdapter {
	return &TradingServiceAdapter{
		service: service,
		logger:  logger,
	}
}

// PlaceOrder places a new trading order
func (a *TradingServiceAdapter) PlaceOrder(ctx context.Context, req *contracts.PlaceOrderRequest) (*contracts.OrderResponse, error) {
	a.logger.Debug("Placing order",
		zap.String("user_id", req.UserID.String()),
		zap.String("symbol", req.Symbol),
		zap.String("side", req.Side),
		zap.String("type", req.Type))

	// Call trading service to place order
	response, err := a.service.PlaceOrder(ctx, req)
	if err != nil {
		a.logger.Error("Failed to place order",
			zap.String("user_id", req.UserID.String()),
			zap.String("symbol", req.Symbol),
			zap.Error(err))
		return nil, err
	}

	return response, nil
}

// CancelOrder cancels an existing order
func (a *TradingServiceAdapter) CancelOrder(ctx context.Context, req *contracts.CancelOrderRequest) error {
	a.logger.Debug("Cancelling order",
		zap.String("user_id", req.UserID.String()))

	// Call trading service to cancel order
	if err := a.service.CancelOrder(ctx, req); err != nil {
		a.logger.Error("Failed to cancel order",
			zap.String("user_id", req.UserID.String()),
			zap.Error(err))
		return err
	}

	return nil
}

// ModifyOrder modifies an existing order
func (a *TradingServiceAdapter) ModifyOrder(ctx context.Context, req *contracts.ModifyOrderRequest) (*contracts.OrderResponse, error) {
	a.logger.Debug("Modifying order",
		zap.String("order_id", req.OrderID.String()),
		zap.String("user_id", req.UserID.String()))

	// Call trading service to modify order
	response, err := a.service.ModifyOrder(ctx, req)
	if err != nil {
		a.logger.Error("Failed to modify order",
			zap.String("order_id", req.OrderID.String()),
			zap.String("user_id", req.UserID.String()),
			zap.Error(err))
		return nil, err
	}

	return response, nil
}

// GetOrder retrieves an order
func (a *TradingServiceAdapter) GetOrder(ctx context.Context, orderID uuid.UUID) (*contracts.Order, error) {
	a.logger.Debug("Getting order", zap.String("order_id", orderID.String()))

	// Call trading service to get order
	order, err := a.service.GetOrder(ctx, orderID)
	if err != nil {
		a.logger.Error("Failed to get order",
			zap.String("order_id", orderID.String()),
			zap.Error(err))
		return nil, err
	}

	return order, nil
}

// GetUserOrders retrieves user orders
func (a *TradingServiceAdapter) GetUserOrders(ctx context.Context, userID uuid.UUID, filter *contracts.OrderFilter) ([]*contracts.Order, int64, error) {
	a.logger.Debug("Getting user orders", zap.String("user_id", userID.String()))

	// Call trading service to get user orders
	orders, total, err := a.service.GetUserOrders(ctx, userID, filter)
	if err != nil {
		a.logger.Error("Failed to get user orders",
			zap.String("user_id", userID.String()),
			zap.Error(err))
		return nil, 0, err
	}

	return orders, total, nil
}

// GetOrderBook retrieves the order book
func (a *TradingServiceAdapter) GetOrderBook(ctx context.Context, symbol string, depth int) (*contracts.OrderBook, error) {
	a.logger.Debug("Getting order book",
		zap.String("symbol", symbol),
		zap.Int("depth", depth))

	// Call trading service to get order book
	orderBook, err := a.service.GetOrderBook(ctx, symbol, depth)
	if err != nil {
		a.logger.Error("Failed to get order book",
			zap.String("symbol", symbol),
			zap.Error(err))
		return nil, err
	}

	return orderBook, nil
}

// GetTicker retrieves ticker information
func (a *TradingServiceAdapter) GetTicker(ctx context.Context, symbol string) (*contracts.Ticker, error) {
	a.logger.Debug("Getting ticker", zap.String("symbol", symbol))

	// Call trading service to get ticker
	ticker, err := a.service.GetTicker(ctx, symbol)
	if err != nil {
		a.logger.Error("Failed to get ticker",
			zap.String("symbol", symbol),
			zap.Error(err))
		return nil, err
	}

	return ticker, nil
}

// GetTrades retrieves recent trades
func (a *TradingServiceAdapter) GetTrades(ctx context.Context, symbol string, limit int) ([]*contracts.Trade, error) {
	a.logger.Debug("Getting trades",
		zap.String("symbol", symbol),
		zap.Int("limit", limit))

	// Call trading service to get trades
	trades, err := a.service.GetTrades(ctx, symbol, limit)
	if err != nil {
		a.logger.Error("Failed to get trades",
			zap.String("symbol", symbol),
			zap.Error(err))
		return nil, err
	}

	return trades, nil
}

// GetUserTrades retrieves user trades with filtering
func (a *TradingServiceAdapter) GetUserTrades(ctx context.Context, userID uuid.UUID, filter *contracts.TradeFilter) ([]*contracts.Trade, int64, error) {
	a.logger.Debug("Getting user trades", zap.String("user_id", userID.String()))

	// Call trading service to get user trades
	trades, total, err := a.service.GetUserTrades(ctx, userID, filter)
	if err != nil {
		a.logger.Error("Failed to get user trades",
			zap.String("user_id", userID.String()),
			zap.Error(err))
		return nil, 0, err
	}

	return trades, total, nil
}

// GetMarkets retrieves all available markets
func (a *TradingServiceAdapter) GetMarkets(ctx context.Context) ([]*contracts.Market, error) {
	a.logger.Debug("Getting markets")

	// Call trading service to get markets
	markets, err := a.service.GetMarkets(ctx)
	if err != nil {
		a.logger.Error("Failed to get markets", zap.Error(err))
		return nil, err
	}

	return markets, nil
}

// GetMarket retrieves specific market information
func (a *TradingServiceAdapter) GetMarket(ctx context.Context, symbol string) (*contracts.Market, error) {
	a.logger.Debug("Getting market", zap.String("symbol", symbol))

	// Call trading service to get market
	market, err := a.service.GetMarket(ctx, symbol)
	if err != nil {
		a.logger.Error("Failed to get market",
			zap.String("symbol", symbol),
			zap.Error(err))
		return nil, err
	}

	return market, nil
}

// GetMarketStats retrieves market statistics
func (a *TradingServiceAdapter) GetMarketStats(ctx context.Context, symbol string) (*contracts.MarketStats, error) {
	a.logger.Debug("Getting market stats", zap.String("symbol", symbol))

	// Call trading service to get market stats
	stats, err := a.service.GetMarketStats(ctx, symbol)
	if err != nil {
		a.logger.Error("Failed to get market stats",
			zap.String("symbol", symbol),
			zap.Error(err))
		return nil, err
	}

	return stats, nil
}

// ValidateOrder validates order before placement
func (a *TradingServiceAdapter) ValidateOrder(ctx context.Context, req *contracts.PlaceOrderRequest) (*contracts.OrderValidation, error) {
	a.logger.Debug("Validating order",
		zap.String("user_id", req.UserID.String()),
		zap.String("symbol", req.Symbol))

	// Call trading service to validate order
	validation, err := a.service.ValidateOrder(ctx, req)
	if err != nil {
		a.logger.Error("Failed to validate order",
			zap.String("user_id", req.UserID.String()),
			zap.String("symbol", req.Symbol),
			zap.Error(err))
		return nil, err
	}

	return validation, nil
}

// GetUserRiskProfile retrieves user risk profile
func (a *TradingServiceAdapter) GetUserRiskProfile(ctx context.Context, userID uuid.UUID) (*contracts.RiskProfile, error) {
	a.logger.Debug("Getting user risk profile", zap.String("user_id", userID.String()))

	// Call trading service to get user risk profile
	profile, err := a.service.GetUserRiskProfile(ctx, userID)
	if err != nil {
		a.logger.Error("Failed to get user risk profile",
			zap.String("user_id", userID.String()),
			zap.Error(err))
		return nil, err
	}

	return profile, nil
}

// CheckTradeCompliance checks trade compliance
func (a *TradingServiceAdapter) CheckTradeCompliance(ctx context.Context, req *contracts.ComplianceCheckRequest) (*contracts.ComplianceResult, error) {
	a.logger.Debug("Checking trade compliance",
		zap.String("user_id", req.UserID.String()),
		zap.String("symbol", req.Symbol))

	// Call trading service to check compliance
	result, err := a.service.CheckTradeCompliance(ctx, req)
	if err != nil {
		a.logger.Error("Failed to check trade compliance",
			zap.String("user_id", req.UserID.String()),
			zap.String("symbol", req.Symbol),
			zap.Error(err))
		return nil, err
	}

	return result, nil
}

// SettleTrade settles a trade
func (a *TradingServiceAdapter) SettleTrade(ctx context.Context, tradeID uuid.UUID) error {
	a.logger.Debug("Settling trade", zap.String("trade_id", tradeID.String()))

	// Call trading service to settle trade
	if err := a.service.SettleTrade(ctx, tradeID); err != nil {
		a.logger.Error("Failed to settle trade",
			zap.String("trade_id", tradeID.String()),
			zap.Error(err))
		return err
	}

	return nil
}

// GetPendingSettlements retrieves pending settlements
func (a *TradingServiceAdapter) GetPendingSettlements(ctx context.Context) ([]*contracts.PendingSettlement, error) {
	a.logger.Debug("Getting pending settlements")

	// Call trading service to get pending settlements
	settlements, err := a.service.GetPendingSettlements(ctx)
	if err != nil {
		a.logger.Error("Failed to get pending settlements", zap.Error(err))
		return nil, err
	}

	return settlements, nil
}

// ReconcileTrades performs trade reconciliation
func (a *TradingServiceAdapter) ReconcileTrades(ctx context.Context, req *contracts.TradeReconciliationRequest) (*contracts.TradeReconciliationResult, error) {
	a.logger.Debug("Reconciling trades")

	// Call trading service to reconcile trades
	result, err := a.service.ReconcileTrades(ctx, req)
	if err != nil {
		a.logger.Error("Failed to reconcile trades", zap.Error(err))
		return nil, err
	}

	return result, nil
}

// GetTradingMetrics retrieves trading metrics
func (a *TradingServiceAdapter) GetTradingMetrics(ctx context.Context, req *contracts.MetricsRequest) (*contracts.TradingMetrics, error) {
	a.logger.Debug("Getting trading metrics")

	// Call trading service to get trading metrics
	metrics, err := a.service.GetTradingMetrics(ctx, req)
	if err != nil {
		a.logger.Error("Failed to get trading metrics", zap.Error(err))
		return nil, err
	}

	return metrics, nil
}

// GetLatencyMetrics retrieves latency metrics
func (a *TradingServiceAdapter) GetLatencyMetrics(ctx context.Context) (*contracts.LatencyMetrics, error) {
	a.logger.Debug("Getting latency metrics")

	// Call trading service to get latency metrics
	metrics, err := a.service.GetLatencyMetrics(ctx)
	if err != nil {
		a.logger.Error("Failed to get latency metrics", zap.Error(err))
		return nil, err
	}

	return metrics, nil
}

// HealthCheck performs health check
func (a *TradingServiceAdapter) HealthCheck(ctx context.Context) (*contracts.HealthStatus, error) {
	a.logger.Debug("Performing health check")

	// Call trading service health check
	health, err := a.service.HealthCheck(ctx)
	if err != nil {
		a.logger.Error("Health check failed", zap.Error(err))
		return nil, err
	}

	return health, nil
}
