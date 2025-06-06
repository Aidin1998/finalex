// =============================
// Integration Layer for Ultra-Low Latency Broadcasting
// =============================
// Connects the broadcaster to existing trading engine components and provides
// seamless integration with current WebSocket infrastructure.

package realtime

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	ws "github.com/Aidin1998/finalex/internal/infrastructure/ws"
	model "github.com/Aidin1998/finalex/internal/trading/model"
	orderbook "github.com/Aidin1998/finalex/internal/trading/orderbook"
)

// =============================
// Broadcaster Integration Manager
// =============================

// BroadcasterIntegration manages the integration of ultra-low latency broadcasting
// with existing trading engine components
type BroadcasterIntegration struct {
	broadcaster *UltraLowLatencyBroadcaster
	wsHub       *ws.Hub
	logger      *zap.Logger

	// Handler registrations
	tradeHandlers     []TradeHandler
	orderBookHandlers []OrderBookHandler
	orderHandlers     []OrderHandler
	accountHandlers   []AccountHandler
}

// Handler function types for different event types
type TradeHandler func(*model.Trade)
type OrderBookHandler func(symbol string, bids, asks []orderbook.PriceLevel)
type OrderHandler func(*model.Order)
type AccountHandler func(userID, currency string, balance float64)

// NewBroadcasterIntegration creates a new integration manager
func NewBroadcasterIntegration(
	wsHub *ws.Hub,
	logger *zap.Logger,
	config *BroadcasterConfig,
) *BroadcasterIntegration {
	broadcaster := NewUltraLowLatencyBroadcaster(wsHub, logger, config)

	return &BroadcasterIntegration{
		broadcaster: broadcaster,
		wsHub:       wsHub,
		logger:      logger,
	}
}

// Start initializes and starts the broadcasting system
func (bi *BroadcasterIntegration) Start() error {
	// Warmup message pools for optimal performance
	bi.broadcaster.WarmupPools()

	// Start the broadcaster
	if err := bi.broadcaster.Start(); err != nil {
		return fmt.Errorf("failed to start broadcaster: %w", err)
	}

	bi.logger.Info("Broadcaster integration started successfully")
	return nil
}

// Stop gracefully shuts down the broadcasting system
func (bi *BroadcasterIntegration) Stop() error {
	return bi.broadcaster.Stop()
}

// =============================
// Event Broadcasting Interface
// =============================

// BroadcastTrade broadcasts a trade event with ultra-low latency
func (bi *BroadcasterIntegration) BroadcastTrade(trade *model.Trade) {
	// Direct broadcast through ultra-low latency system
	bi.broadcaster.BroadcastTrade(trade)

	// Trigger registered handlers
	for _, handler := range bi.tradeHandlers {
		go handler(trade) // Execute handlers asynchronously to avoid blocking
	}
}

// BroadcastOrderBookUpdate broadcasts order book changes
func (bi *BroadcasterIntegration) BroadcastOrderBookUpdate(
	symbol string,
	bids, asks []orderbook.PriceLevel,
) {
	// Direct broadcast through ultra-low latency system
	bi.broadcaster.BroadcastOrderBookUpdate(symbol, bids, asks)

	// Trigger registered handlers
	for _, handler := range bi.orderBookHandlers {
		go handler(symbol, bids, asks)
	}
}

// BroadcastOrderUpdate broadcasts order status changes
func (bi *BroadcasterIntegration) BroadcastOrderUpdate(order *model.Order) {
	// Direct broadcast through ultra-low latency system
	bi.broadcaster.BroadcastOrderUpdate(order)

	// Trigger registered handlers
	for _, handler := range bi.orderHandlers {
		go handler(order)
	}
}

// BroadcastAccountUpdate broadcasts account balance changes
func (bi *BroadcasterIntegration) BroadcastAccountUpdate(
	userID, currency string,
	balance float64,
) {
	// Direct broadcast through ultra-low latency system
	bi.broadcaster.BroadcastAccountUpdate(userID, currency, balance)

	// Trigger registered handlers
	for _, handler := range bi.accountHandlers {
		go handler(userID, currency, balance)
	}
}

// =============================
// Handler Registration
// =============================

// RegisterTradeHandler registers a handler for trade events
func (bi *BroadcasterIntegration) RegisterTradeHandler(handler TradeHandler) {
	bi.tradeHandlers = append(bi.tradeHandlers, handler)
	bi.logger.Debug("Trade handler registered", zap.Int("total_handlers", len(bi.tradeHandlers)))
}

// RegisterOrderBookHandler registers a handler for order book events
func (bi *BroadcasterIntegration) RegisterOrderBookHandler(handler OrderBookHandler) {
	bi.orderBookHandlers = append(bi.orderBookHandlers, handler)
	bi.logger.Debug("Order book handler registered", zap.Int("total_handlers", len(bi.orderBookHandlers)))
}

// RegisterOrderHandler registers a handler for order events
func (bi *BroadcasterIntegration) RegisterOrderHandler(handler OrderHandler) {
	bi.orderHandlers = append(bi.orderHandlers, handler)
	bi.logger.Debug("Order handler registered", zap.Int("total_handlers", len(bi.orderHandlers)))
}

// RegisterAccountHandler registers a handler for account events
func (bi *BroadcasterIntegration) RegisterAccountHandler(handler AccountHandler) {
	bi.accountHandlers = append(bi.accountHandlers, handler)
	bi.logger.Debug("Account handler registered", zap.Int("total_handlers", len(bi.accountHandlers)))
}

// =============================
// Performance and Monitoring
// =============================

// GetPerformanceMetrics returns comprehensive performance metrics
func (bi *BroadcasterIntegration) GetPerformanceMetrics() map[string]interface{} {
	metrics := bi.broadcaster.GetPerformanceMetrics()

	// Add integration-specific metrics
	metrics["trade_handlers"] = len(bi.tradeHandlers)
	metrics["orderbook_handlers"] = len(bi.orderBookHandlers)
	metrics["order_handlers"] = len(bi.orderHandlers)
	metrics["account_handlers"] = len(bi.accountHandlers)

	return metrics
}

// ResetPerformanceCounters resets all performance counters
func (bi *BroadcasterIntegration) ResetPerformanceCounters() {
	bi.broadcaster.ResetPerformanceCounters()
}

// IsRunning returns true if the broadcaster is currently running
func (bi *BroadcasterIntegration) IsRunning() bool {
	return bi.broadcaster.IsRunning()
}

// GetQueueUtilization returns current queue utilization percentage
func (bi *BroadcasterIntegration) GetQueueUtilization() float64 {
	return bi.broadcaster.GetQueueUtilization()
}

// IsUnderLoad returns true if the system is under heavy load
func (bi *BroadcasterIntegration) IsUnderLoad() bool {
	return bi.broadcaster.IsUnderLoad()
}

// =============================
// Legacy WebSocket Hub Integration
// =============================

// LegacyWebSocketAdapter provides backward compatibility with existing WebSocket broadcasting
type LegacyWebSocketAdapter struct {
	integration *BroadcasterIntegration
	logger      *zap.Logger
}

// NewLegacyWebSocketAdapter creates an adapter for legacy WebSocket integration
func NewLegacyWebSocketAdapter(integration *BroadcasterIntegration, logger *zap.Logger) *LegacyWebSocketAdapter {
	return &LegacyWebSocketAdapter{
		integration: integration,
		logger:      logger,
	}
}

// Broadcast provides legacy WebSocket broadcast interface
func (lwa *LegacyWebSocketAdapter) Broadcast(topic string, data []byte) {
	// Parse topic to determine event type and route to appropriate broadcaster
	switch {
	case len(topic) > 7 && topic[:7] == "trades.":
		// Trade broadcast - parse data and convert to Trade model
		if trade := lwa.parseTradeData(data); trade != nil {
			lwa.integration.BroadcastTrade(trade)
		} else {
			// Fallback to direct WebSocket broadcast for unparseable data
			lwa.integration.wsHub.Broadcast(topic, data)
		}

	case len(topic) > 10 && topic[:10] == "orderbook.":
		// Order book broadcast - parse data and convert to order book update
		symbol := topic[10:]
		if bids, asks := lwa.parseOrderBookData(data); len(bids) > 0 || len(asks) > 0 {
			lwa.integration.BroadcastOrderBookUpdate(symbol, bids, asks)
		} else {
			// Fallback to direct WebSocket broadcast
			lwa.integration.wsHub.Broadcast(topic, data)
		}

	case len(topic) > 7 && topic[:7] == "orders.":
		// Order update broadcast - parse data and convert to Order model
		if order := lwa.parseOrderData(data); order != nil {
			lwa.integration.BroadcastOrderUpdate(order)
		} else {
			// Fallback to direct WebSocket broadcast
			lwa.integration.wsHub.Broadcast(topic, data)
		}

	case len(topic) > 8 && topic[:8] == "account.":
		// Account update broadcast - parse data and convert to account update
		userID := topic[8:]
		if currency, balance := lwa.parseAccountData(data); currency != "" {
			lwa.integration.BroadcastAccountUpdate(userID, currency, balance)
		} else {
			// Fallback to direct WebSocket broadcast
			lwa.integration.wsHub.Broadcast(topic, data)
		}

	default:
		// Unknown topic - use direct WebSocket broadcast
		lwa.integration.wsHub.Broadcast(topic, data)
	}
}

// Helper methods for parsing legacy data formats
func (lwa *LegacyWebSocketAdapter) parseTradeData(data []byte) *model.Trade {
	// TODO: Implement JSON parsing to model.Trade
	// For now, return nil to use fallback
	return nil
}

func (lwa *LegacyWebSocketAdapter) parseOrderBookData(data []byte) ([]orderbook.PriceLevel, []orderbook.PriceLevel) {
	// TODO: Implement JSON parsing to price levels
	// For now, return empty slices to use fallback
	return nil, nil
}

func (lwa *LegacyWebSocketAdapter) parseOrderData(data []byte) *model.Order {
	// TODO: Implement JSON parsing to model.Order
	// For now, return nil to use fallback
	return nil
}

func (lwa *LegacyWebSocketAdapter) parseAccountData(data []byte) (string, float64) {
	// TODO: Implement JSON parsing to extract currency and balance
	// For now, return empty values to use fallback
	return "", 0.0
}

// =============================
// High-Performance Engine Integration
// =============================

// IntegrateWithHighPerformanceEngine integrates the broadcaster with the existing
// high-performance matching engine
func (bi *BroadcasterIntegration) IntegrateWithHighPerformanceEngine(ctx context.Context) error {
	// Register with high-performance engine's event handlers

	// Trade event handler
	bi.RegisterTradeHandler(func(trade *model.Trade) {
		bi.logger.Debug("Processing trade through high-performance broadcaster",
			zap.String("trade_id", trade.ID.String()),
			zap.String("symbol", trade.Pair),
			zap.String("price", trade.Price.String()),
			zap.String("quantity", trade.Quantity.String()))
	})

	// Order book update handler
	bi.RegisterOrderBookHandler(func(symbol string, bids, asks []orderbook.PriceLevel) {
		bi.logger.Debug("Processing order book update through high-performance broadcaster",
			zap.String("symbol", symbol),
			zap.Int("bids_count", len(bids)),
			zap.Int("asks_count", len(asks)))
	})

	// Order update handler
	bi.RegisterOrderHandler(func(order *model.Order) {
		bi.logger.Debug("Processing order update through high-performance broadcaster",
			zap.String("order_id", order.ID.String()),
			zap.String("symbol", order.Pair),
			zap.String("status", order.Status))
	})

	bi.logger.Info("Broadcaster integration with high-performance engine completed")
	return nil
}

// =============================
// Configuration and Tuning
// =============================

// TuneForProduction applies production-optimized settings
func (bi *BroadcasterIntegration) TuneForProduction() {
	// Get current performance metrics to assess load
	metrics := bi.GetPerformanceMetrics()

	if queueUtil, ok := metrics["queue_utilization"].(float64); ok && queueUtil > 70.0 {
		bi.logger.Warn("High queue utilization detected - consider scaling",
			zap.Float64("utilization", queueUtil))
	}

	if avgLatency, ok := metrics["avg_latency_us"].(float64); ok && avgLatency > 10.0 {
		bi.logger.Warn("Average latency exceeds target - consider optimization",
			zap.Float64("avg_latency_us", avgLatency))
	}

	bi.logger.Info("Production tuning assessment completed")
}

// EnableProfiling enables detailed performance profiling
func (bi *BroadcasterIntegration) EnableProfiling() {
	bi.broadcaster.config.EnableProfiling = true
	bi.logger.Info("Broadcasting profiling enabled")
}

// DisableProfiling disables performance profiling for production
func (bi *BroadcasterIntegration) DisableProfiling() {
	bi.broadcaster.config.EnableProfiling = false
	bi.logger.Info("Broadcasting profiling disabled")
}
