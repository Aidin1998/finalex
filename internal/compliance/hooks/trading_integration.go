// Package hooks provides trading module integration with compliance
package hooks

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// TradingIntegration provides integration between trading and compliance modules
type TradingIntegration struct {
	hookManager *HookManager
}

// NewTradingIntegration creates a new trading integration
func NewTradingIntegration(hookManager *HookManager) *TradingIntegration {
	return &TradingIntegration{
		hookManager: hookManager,
	}
}

// OnOrderPlaced should be called when an order is placed
func (ti *TradingIntegration) OnOrderPlaced(ctx context.Context, orderID string, userID uuid.UUID, market, side, orderType string, quantity, price decimal.Decimal, ipAddress string) error {
	event := &OrderPlacedEvent{
		BaseEvent: BaseEvent{
			Type:      EventTypeTradingOrderPlaced,
			UserID:    userID.String(),
			Timestamp: time.Now(),
			Module:    ModuleTrading,
		},
		OrderID:   orderID,
		Market:    market,
		Side:      side,
		Type:      orderType,
		Quantity:  quantity,
		Price:     price,
		IPAddress: ipAddress,
	}

	return ti.hookManager.TriggerHooks(ctx, event)
}

// OnOrderExecuted should be called when an order is executed
func (ti *TradingIntegration) OnOrderExecuted(ctx context.Context, orderID string, userID uuid.UUID, market string, executedQty, executedPrice, remainingQty decimal.Decimal, tradeID string) error {
	event := &OrderExecutedEvent{
		BaseEvent: BaseEvent{
			Type:      EventTypeTradingOrderExecuted,
			UserID:    userID.String(),
			Timestamp: time.Now(),
			Module:    ModuleTrading,
		},
		OrderID:       orderID,
		Market:        market,
		ExecutedPrice: executedPrice,
		ExecutedQty:   executedQty,
	}

	return ti.hookManager.TriggerHooks(ctx, event)
}

// OnOrderCancelled should be called when an order is cancelled
func (ti *TradingIntegration) OnOrderCancelled(ctx context.Context, orderID string, userID uuid.UUID, market, reason string) error {
	event := &OrderCancelledEvent{
		BaseEvent: BaseEvent{
			Type:      EventTypeTradingOrderCanceled,
			UserID:    userID.String(),
			Timestamp: time.Now(),
			Module:    ModuleTrading,
		},
		OrderID: orderID,
		Market:  market,
		Reason:  reason,
	}

	return ti.hookManager.TriggerHooks(ctx, event)
}

// OnTradeExecuted should be called when a trade is executed
func (ti *TradingIntegration) OnTradeExecuted(ctx context.Context, tradeID, market string, buyerID, sellerID uuid.UUID, quantity, price decimal.Decimal, buyOrderID, sellOrderID string) error {
	event := &TradeExecutedEvent{
		BaseEvent: BaseEvent{
			Type:      EventTypeTradingTradeExecuted,
			UserID:    buyerID.String(), // Use buyer as primary user
			Timestamp: time.Now(),
			Module:    ModuleTrading,
		},
		TradeID:   tradeID,
		Market:    market,
		BuyerID:   buyerID.String(),
		SellerID:  sellerID.String(),
		Quantity:  quantity,
		Price:     price,
		TakerSide: "buy",        // This should be determined based on logic
		Fee:       decimal.Zero, // This should be passed as parameter
	}

	return ti.hookManager.TriggerHooks(ctx, event)
}

// OnPositionUpdated should be called when a user's position is updated
func (ti *TradingIntegration) OnPositionUpdated(ctx context.Context, userID uuid.UUID, market string, position, avgPrice, unrealizedPnL decimal.Decimal) error {
	event := &PositionUpdateEvent{
		BaseEvent: BaseEvent{
			Type:      EventTypeTradingPositionUpdate,
			UserID:    userID.String(),
			Timestamp: time.Now(),
			Module:    ModuleTrading,
		},
		Market:   market,
		Size:     position,
		AvgPrice: avgPrice,
		PnL:      unrealizedPnL,
		Margin:   decimal.Zero, // This should be calculated/passed as parameter
	}

	return ti.hookManager.TriggerHooks(ctx, event)
}

// OnMarketDataReceived should be called when market data is received
func (ti *TradingIntegration) OnMarketDataReceived(ctx context.Context, market string, price, volume, bidPrice, askPrice decimal.Decimal) error {
	event := &MarketDataEvent{
		BaseEvent: BaseEvent{
			Type:      "trading.market_data", // Need to add this constant
			UserID:    "",                    // Market data is not user-specific
			Timestamp: time.Now(),
			Module:    ModuleTrading,
		},
		Market:    market,
		Price:     price,
		Volume:    volume,
		Change24h: decimal.Zero, // This should be calculated
	}

	return ti.hookManager.TriggerHooks(ctx, event)
}
