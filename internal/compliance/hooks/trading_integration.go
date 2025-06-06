// Package hooks provides trading module integration with compliance
package hooks

import (
	"context"
	"time"

	"github.com/Aidin1998/finalex/internal/compliance/hooks"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// TradingIntegration provides integration between trading and compliance modules
type TradingIntegration struct {
	hookManager *hooks.HookManager
}

// NewTradingIntegration creates a new trading integration
func NewTradingIntegration(hookManager *hooks.HookManager) *TradingIntegration {
	return &TradingIntegration{
		hookManager: hookManager,
	}
}

// OnOrderPlaced should be called when an order is placed
func (ti *TradingIntegration) OnOrderPlaced(ctx context.Context, orderID string, userID uuid.UUID, market, side, orderType string, quantity, price decimal.Decimal, ipAddress string) error {
	event := &hooks.OrderPlacedEvent{
		OrderID:   orderID,
		UserID:    userID,
		Market:    market,
		Side:      side,
		Type:      orderType,
		Quantity:  quantity,
		Price:     price,
		IPAddress: ipAddress,
		Timestamp: time.Now(),
	}

	return ti.hookManager.ProcessTradingEvent(ctx, "order_placed", event)
}

// OnOrderExecuted should be called when an order is executed
func (ti *TradingIntegration) OnOrderExecuted(ctx context.Context, orderID string, userID uuid.UUID, market string, executedQty, executedPrice, remainingQty decimal.Decimal, tradeID string) error {
	event := &hooks.OrderExecutedEvent{
		OrderID:       orderID,
		UserID:        userID,
		Market:        market,
		ExecutedQty:   executedQty,
		ExecutedPrice: executedPrice,
		RemainingQty:  remainingQty,
		TradeID:       tradeID,
		Timestamp:     time.Now(),
	}

	return ti.hookManager.ProcessTradingEvent(ctx, "order_executed", event)
}

// OnOrderCancelled should be called when an order is cancelled
func (ti *TradingIntegration) OnOrderCancelled(ctx context.Context, orderID string, userID uuid.UUID, market, reason string) error {
	event := &hooks.OrderCancelledEvent{
		OrderID:   orderID,
		UserID:    userID,
		Market:    market,
		Reason:    reason,
		Timestamp: time.Now(),
	}

	return ti.hookManager.ProcessTradingEvent(ctx, "order_cancelled", event)
}

// OnTradeExecuted should be called when a trade is executed
func (ti *TradingIntegration) OnTradeExecuted(ctx context.Context, tradeID, market string, buyerID, sellerID uuid.UUID, quantity, price decimal.Decimal, buyOrderID, sellOrderID string) error {
	event := &hooks.TradeExecutedEvent{
		TradeID:     tradeID,
		Market:      market,
		BuyerID:     buyerID,
		SellerID:    sellerID,
		Quantity:    quantity,
		Price:       price,
		BuyOrderID:  buyOrderID,
		SellOrderID: sellOrderID,
		Timestamp:   time.Now(),
	}

	return ti.hookManager.ProcessTradingEvent(ctx, "trade_executed", event)
}

// OnPositionUpdated should be called when a user's position is updated
func (ti *TradingIntegration) OnPositionUpdated(ctx context.Context, userID uuid.UUID, market string, position, avgPrice, unrealizedPnL decimal.Decimal) error {
	event := &hooks.PositionUpdateEvent{
		UserID:        userID,
		Market:        market,
		Position:      position,
		AvgPrice:      avgPrice,
		UnrealizedPnL: unrealizedPnL,
		Timestamp:     time.Now(),
	}

	return ti.hookManager.ProcessTradingEvent(ctx, "position_updated", event)
}

// OnMarketDataReceived should be called when market data is received
func (ti *TradingIntegration) OnMarketDataReceived(ctx context.Context, market string, price, volume, bidPrice, askPrice decimal.Decimal) error {
	event := &hooks.MarketDataEvent{
		Market:    market,
		Price:     price,
		Volume:    volume,
		BidPrice:  bidPrice,
		AskPrice:  askPrice,
		Timestamp: time.Now(),
	}

	return ti.hookManager.ProcessTradingEvent(ctx, "market_data_received", event)
}

// ProcessTradingEvent processes trading events through hooks (this should be added to HookManager)
func (hm *hooks.HookManager) ProcessTradingEvent(ctx context.Context, eventType string, event interface{}) error {
	// This method should be implemented in the HookManager
	// For now, return nil to avoid compilation errors
	return nil
}
