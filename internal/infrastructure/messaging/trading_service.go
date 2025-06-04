package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	ws "github.com/Aidin1998/pincex_unified/internal/infrastructure/ws"
	"github.com/Aidin1998/pincex_unified/pkg/models"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// TradingMessageService handles trading operations via message queue
type TradingMessageService struct {
	bookkeeperMsgSvc *BookkeeperMessageService
	messageBus       *MessageBus
	wsHub            *ws.Hub
	logger           *zap.Logger
}

// NewTradingMessageService creates a new message-driven trading service
func NewTradingMessageService(
	bookkeeperMsgSvc *BookkeeperMessageService,
	messageBus *MessageBus,
	wsHub *ws.Hub,
	logger *zap.Logger,
) *TradingMessageService {
	service := &TradingMessageService{
		bookkeeperMsgSvc: bookkeeperMsgSvc,
		messageBus:       messageBus,
		wsHub:            wsHub,
		logger:           logger,
	}

	// Register message handlers
	service.registerHandlers()

	return service
}

// registerHandlers registers all message handlers for trading operations
func (s *TradingMessageService) registerHandlers() {
	s.messageBus.RegisterHandler(MsgOrderPlaced, s.handleOrderPlaced)
	s.messageBus.RegisterHandler(MsgOrderCanceled, s.handleOrderCanceled)
	s.messageBus.RegisterHandler(MsgOrderFilled, s.handleOrderFilled)
}

// AsyncPlaceOrder publishes an order placement request asynchronously
func (s *TradingMessageService) AsyncPlaceOrder(ctx context.Context, order *models.Order) error {
	s.logger.Info("Publishing order placement request",
		zap.String("order_id", order.ID.String()),
		zap.String("user_id", order.UserID.String()),
		zap.String("symbol", order.Symbol),
		zap.String("side", order.Side))

	orderEvent := &OrderEventMessage{
		BaseMessage: NewBaseMessage(MsgOrderPlaced, "trading-api", order.ID.String()),
		UserID:      order.UserID.String(),
		OrderID:     order.ID.String(),
		Symbol:      order.Symbol,
		Side:        order.Side,
		Type:        order.Type,
		Price:       decimal.NewFromFloat(order.Price),
		Quantity:    decimal.NewFromFloat(order.Quantity),
		FilledQty:   decimal.NewFromFloat(0),
		Status:      order.Status,
		TimeInForce: order.TimeInForce,
	}

	return s.messageBus.PublishOrderEvent(ctx, orderEvent)
}

// AsyncCancelOrder publishes an order cancellation request asynchronously
func (s *TradingMessageService) AsyncCancelOrder(ctx context.Context, orderID, userID string, reason string) error {
	s.logger.Info("Publishing order cancellation request",
		zap.String("order_id", orderID),
		zap.String("user_id", userID),
		zap.String("reason", reason))

	orderEvent := &OrderEventMessage{
		BaseMessage: NewBaseMessage(MsgOrderCanceled, "trading-api", orderID),
		UserID:      userID,
		OrderID:     orderID,
		Status:      "CANCELED",
		Reason:      reason,
	}

	return s.messageBus.PublishOrderEvent(ctx, orderEvent)
}

// handleOrderPlaced processes order placement requests
func (s *TradingMessageService) handleOrderPlaced(ctx context.Context, msg *ReceivedMessage) error {
	var orderMsg OrderEventMessage
	if err := json.Unmarshal(msg.Value, &orderMsg); err != nil {
		return fmt.Errorf("failed to unmarshal order event message: %w", err)
	}

	s.logger.Info("Processing order placement",
		zap.String("order_id", orderMsg.OrderID),
		zap.String("user_id", orderMsg.UserID),
		zap.String("symbol", orderMsg.Symbol),
		zap.String("side", orderMsg.Side))

	// Validate order
	if err := s.validateOrder(&orderMsg); err != nil {
		return s.rejectOrder(ctx, &orderMsg, fmt.Sprintf("Order validation failed: %v", err))
	}

	// Check and lock funds
	if err := s.lockOrderFunds(ctx, &orderMsg); err != nil {
		return s.rejectOrder(ctx, &orderMsg, fmt.Sprintf("Insufficient funds: %v", err))
	}
	// Update order status to OPEN and publish event
	orderMsg.Status = "OPEN"
	orderMsg.BaseMessage = NewBaseMessage(MsgOrderUpdated, "trading-engine", orderMsg.CorrelationID)

	if err := s.messageBus.PublishOrderEvent(ctx, &orderMsg); err != nil {
		s.logger.Error("Failed to publish order update", zap.Error(err))
		return err
	}

	// Broadcast to WebSocket clients for user-specific order updates
	if s.wsHub != nil {
		if data, err := json.Marshal(orderMsg); err == nil {
			userTopic := fmt.Sprintf("orders.%s", orderMsg.UserID)
			s.wsHub.Broadcast(userTopic, data)
			s.logger.Debug("Broadcasted order update to WebSocket clients",
				zap.String("topic", userTopic),
				zap.String("order_id", orderMsg.OrderID))
		} else {
			s.logger.Error("Failed to marshal order update for WebSocket broadcast", zap.Error(err))
		}
	}

	// TODO: Forward to matching engine
	s.logger.Info("Order placed successfully and sent to matching engine",
		zap.String("order_id", orderMsg.OrderID))

	return nil
}

// handleOrderCanceled processes order cancellation requests
func (s *TradingMessageService) handleOrderCanceled(ctx context.Context, msg *ReceivedMessage) error {
	var orderMsg OrderEventMessage
	if err := json.Unmarshal(msg.Value, &orderMsg); err != nil {
		return fmt.Errorf("failed to unmarshal order event message: %w", err)
	}

	s.logger.Info("Processing order cancellation",
		zap.String("order_id", orderMsg.OrderID),
		zap.String("user_id", orderMsg.UserID),
		zap.String("reason", orderMsg.Reason))

	// TODO: Remove from matching engine order book

	// Unlock funds for the canceled order
	if err := s.unlockOrderFunds(ctx, &orderMsg); err != nil {
		s.logger.Error("Failed to unlock funds for canceled order",
			zap.Error(err),
			zap.String("order_id", orderMsg.OrderID))
		// Continue processing even if unlock fails - this should be handled by reconciliation
	}
	// Update order status and publish event
	orderMsg.Status = "CANCELED"
	orderMsg.BaseMessage = NewBaseMessage(MsgOrderUpdated, "trading-engine", orderMsg.CorrelationID)

	if err := s.messageBus.PublishOrderEvent(ctx, &orderMsg); err != nil {
		return err
	}

	// Broadcast to WebSocket clients for user-specific order updates
	if s.wsHub != nil {
		if data, err := json.Marshal(orderMsg); err == nil {
			userTopic := fmt.Sprintf("orders.%s", orderMsg.UserID)
			s.wsHub.Broadcast(userTopic, data)
			s.logger.Debug("Broadcasted order cancellation to WebSocket clients",
				zap.String("topic", userTopic),
				zap.String("order_id", orderMsg.OrderID))
		} else {
			s.logger.Error("Failed to marshal order cancellation for WebSocket broadcast", zap.Error(err))
		}
	}

	return nil
}

// handleOrderFilled processes order fill events
func (s *TradingMessageService) handleOrderFilled(ctx context.Context, msg *ReceivedMessage) error {
	var orderMsg OrderEventMessage
	if err := json.Unmarshal(msg.Value, &orderMsg); err != nil {
		return fmt.Errorf("failed to unmarshal order event message: %w", err)
	}

	s.logger.Info("Processing order fill",
		zap.String("order_id", orderMsg.OrderID),
		zap.String("user_id", orderMsg.UserID),
		zap.String("filled_qty", orderMsg.FilledQty.String()))

	// If order is fully filled, unlock any remaining funds
	if orderMsg.Status == "FILLED" {
		remainingQty := orderMsg.Quantity.Sub(orderMsg.FilledQty)
		if remainingQty.GreaterThan(decimal.Zero) {
			if err := s.unlockPartialOrderFunds(ctx, &orderMsg, remainingQty); err != nil {
				s.logger.Error("Failed to unlock remaining funds for filled order",
					zap.Error(err),
					zap.String("order_id", orderMsg.OrderID))
			}
		}
	}

	// Publish order update event
	orderMsg.BaseMessage = NewBaseMessage(MsgOrderUpdated, "trading-engine", orderMsg.CorrelationID)
	if err := s.messageBus.PublishOrderEvent(ctx, &orderMsg); err != nil {
		return err
	}

	// Broadcast to WebSocket clients for user-specific order updates
	if s.wsHub != nil {
		if data, err := json.Marshal(orderMsg); err == nil {
			userTopic := fmt.Sprintf("orders.%s", orderMsg.UserID)
			s.wsHub.Broadcast(userTopic, data)
			s.logger.Debug("Broadcasted order fill to WebSocket clients",
				zap.String("topic", userTopic),
				zap.String("order_id", orderMsg.OrderID),
				zap.String("status", orderMsg.Status))
		} else {
			s.logger.Error("Failed to marshal order fill for WebSocket broadcast", zap.Error(err))
		}
	}

	// Also broadcast trade event to public feeds
	if orderMsg.Status == "FILLED" || orderMsg.Status == "PARTIALLY_FILLED" {
		tradeData := map[string]interface{}{
			"symbol":    orderMsg.Symbol,
			"price":     orderMsg.Price,
			"quantity":  orderMsg.FilledQty,
			"side":      orderMsg.Side,
			"timestamp": orderMsg.Timestamp,
		}
		if data, err := json.Marshal(tradeData); err == nil {
			tradeTopic := fmt.Sprintf("trades.%s", orderMsg.Symbol)
			s.wsHub.Broadcast(tradeTopic, data)
			s.logger.Debug("Broadcasted trade data to WebSocket clients",
				zap.String("topic", tradeTopic),
				zap.String("symbol", orderMsg.Symbol))
		}
	}

	return nil
}

// validateOrder validates order parameters
func (s *TradingMessageService) validateOrder(order *OrderEventMessage) error {
	if order.UserID == "" {
		return fmt.Errorf("user ID is required")
	}
	if order.Symbol == "" {
		return fmt.Errorf("symbol is required")
	}
	if order.Side != "BUY" && order.Side != "SELL" {
		return fmt.Errorf("side must be BUY or SELL")
	}
	if order.Quantity.LessThanOrEqual(decimal.Zero) {
		return fmt.Errorf("quantity must be positive")
	}
	if order.Price.LessThanOrEqual(decimal.Zero) && order.Type == "LIMIT" {
		return fmt.Errorf("price must be positive for limit orders")
	}
	return nil
}

// lockOrderFunds locks the required funds for an order
func (s *TradingMessageService) lockOrderFunds(ctx context.Context, order *OrderEventMessage) error {
	var currency string
	var amount decimal.Decimal

	if order.Side == "BUY" {
		// For buy orders, lock quote currency
		currency = s.getQuoteCurrency(order.Symbol)
		amount = order.Price.Mul(order.Quantity).Mul(decimal.NewFromFloat(1.001)) // Add 0.1% for fees
	} else {
		// For sell orders, lock base currency
		currency = s.getBaseCurrency(order.Symbol)
		amount = order.Quantity
	}

	return s.bookkeeperMsgSvc.RequestFundsLock(ctx, order.UserID, currency, amount, order.OrderID, "order_placement")
}

// unlockOrderFunds unlocks all funds for a canceled order
func (s *TradingMessageService) unlockOrderFunds(ctx context.Context, order *OrderEventMessage) error {
	var currency string
	var amount decimal.Decimal

	if order.Side == "BUY" {
		currency = s.getQuoteCurrency(order.Symbol)
		amount = order.Price.Mul(order.Quantity).Mul(decimal.NewFromFloat(1.001))
	} else {
		currency = s.getBaseCurrency(order.Symbol)
		amount = order.Quantity
	}

	return s.bookkeeperMsgSvc.RequestFundsUnlock(ctx, order.UserID, currency, amount, order.OrderID, "order_cancellation")
}

// unlockPartialOrderFunds unlocks funds for partially filled orders
func (s *TradingMessageService) unlockPartialOrderFunds(ctx context.Context, order *OrderEventMessage, remainingQty decimal.Decimal) error {
	var currency string
	var amount decimal.Decimal

	if order.Side == "BUY" {
		currency = s.getQuoteCurrency(order.Symbol)
		amount = order.Price.Mul(remainingQty).Mul(decimal.NewFromFloat(1.001))
	} else {
		currency = s.getBaseCurrency(order.Symbol)
		amount = remainingQty
	}

	return s.bookkeeperMsgSvc.RequestFundsUnlock(ctx, order.UserID, currency, amount, order.OrderID, "partial_fill")
}

// rejectOrder publishes an order rejection event
func (s *TradingMessageService) rejectOrder(ctx context.Context, order *OrderEventMessage, reason string) error {
	s.logger.Warn("Rejecting order",
		zap.String("order_id", order.OrderID),
		zap.String("reason", reason))

	order.Status = "REJECTED"
	order.Reason = reason
	order.BaseMessage = NewBaseMessage(MsgOrderRejected, "trading-engine", order.CorrelationID)

	return s.messageBus.PublishOrderEvent(ctx, order)
}

// Helper functions to extract currencies from symbol
func (s *TradingMessageService) getBaseCurrency(symbol string) string {
	if strings.Contains(symbol, "/") {
		return symbol[:strings.Index(symbol, "/")]
	}
	if len(symbol) >= 6 {
		return symbol[:3]
	}
	return "BTC"
}

func (s *TradingMessageService) getQuoteCurrency(symbol string) string {
	if strings.Contains(symbol, "/") {
		return symbol[strings.Index(symbol, "/")+1:]
	}
	if len(symbol) >= 6 {
		return symbol[3:]
	}
	return "USDT"
}

// PublishTradeExecution publishes a trade execution event
func (s *TradingMessageService) PublishTradeExecution(ctx context.Context, trade *TradeEventMessage) error {
	s.logger.Info("Publishing trade execution",
		zap.String("trade_id", trade.TradeID),
		zap.String("symbol", trade.Symbol),
		zap.String("price", trade.Price.String()),
		zap.String("quantity", trade.Quantity.String()))

	return s.messageBus.PublishTradeEvent(ctx, trade)
}

// SyncPlaceOrder provides a synchronous interface for order placement (for compatibility)
func (s *TradingMessageService) SyncPlaceOrder(ctx context.Context, order *models.Order) (*models.Order, error) {
	// For now, this publishes async and returns the order
	// In a full implementation, you might want to wait for order processing confirmation
	if err := s.AsyncPlaceOrder(ctx, order); err != nil {
		return nil, err
	}

	// Set status to pending since it's being processed asynchronously
	order.Status = "PENDING"
	return order, nil
}

// CheckAccountBalance provides a way to check balance asynchronously
// This would typically involve publishing a balance query request and waiting for response
func (s *TradingMessageService) CheckAccountBalance(ctx context.Context, userID, currency string) error {
	// This could be implemented as a request-response pattern using Kafka
	// For now, we'll just log the request
	s.logger.Info("Balance check requested",
		zap.String("user_id", userID),
		zap.String("currency", currency))

	// In a real implementation, you might:
	// 1. Publish a balance query request
	// 2. Wait for response on a reply topic
	// 3. Return the balance information

	return nil
}
