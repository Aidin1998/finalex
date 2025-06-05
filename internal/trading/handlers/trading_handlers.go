// Package handlers provides HTTP handlers for trading operations
package handlers

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Aidin1998/pincex_unified/common/apiutil"
	"github.com/Aidin1998/pincex_unified/internal/trading"
	"github.com/Aidin1998/pincex_unified/pkg/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// TradingHandler handles trading-related HTTP requests
type TradingHandler struct {
	tradingService trading.TradingService
	logger         *zap.Logger
}

// NewTradingHandler creates a new trading handler
func NewTradingHandler(tradingService trading.TradingService, logger *zap.Logger) *TradingHandler {
	return &TradingHandler{
		tradingService: tradingService,
		logger:         logger,
	}
}

// PlaceOrderRequest represents order placement request
type PlaceOrderRequest struct {
	Symbol        string          `json:"symbol" binding:"required" validate:"trading_pair"`
	Side          string          `json:"side" binding:"required" validate:"oneof=buy sell"`
	Type          string          `json:"type" binding:"required" validate:"oneof=market limit stop_loss stop_loss_limit take_profit take_profit_limit"`
	Quantity      decimal.Decimal `json:"quantity" binding:"required" validate:"gt=0"`
	Price         decimal.Decimal `json:"price,omitempty"`
	StopPrice     decimal.Decimal `json:"stop_price,omitempty"`
	TimeInForce   string          `json:"time_in_force,omitempty" validate:"oneof=GTC IOC FOK GTD"`
	ClientOrderID string          `json:"client_order_id,omitempty" validate:"max=64"`
	ReduceOnly    bool            `json:"reduce_only,omitempty"`
}

// PlaceOrderResponse represents order placement response
type PlaceOrderResponse struct {
	OrderID       string          `json:"order_id"`
	ClientOrderID string          `json:"client_order_id,omitempty"`
	Symbol        string          `json:"symbol"`
	Side          string          `json:"side"`
	Type          string          `json:"type"`
	Quantity      decimal.Decimal `json:"quantity"`
	Price         decimal.Decimal `json:"price,omitempty"`
	Status        string          `json:"status"`
	TimeInForce   string          `json:"time_in_force"`
	CreatedAt     time.Time       `json:"created_at"`
	Fills         []OrderFill     `json:"fills,omitempty"`
}

// OrderFill represents an order fill
type OrderFill struct {
	TradeID   string          `json:"trade_id"`
	Price     decimal.Decimal `json:"price"`
	Quantity  decimal.Decimal `json:"quantity"`
	Fee       decimal.Decimal `json:"fee"`
	FeeAsset  string          `json:"fee_asset"`
	Timestamp time.Time       `json:"timestamp"`
}

// ListOrdersRequest represents order listing request
type ListOrdersRequest struct {
	Symbol    string    `form:"symbol"`
	Status    string    `form:"status" validate:"omitempty,oneof=open filled cancelled expired partially_filled"`
	Side      string    `form:"side" validate:"omitempty,oneof=buy sell"`
	StartTime time.Time `form:"start_time"`
	EndTime   time.Time `form:"end_time"`
	Page      int       `form:"page,default=1" validate:"min=1"`
	PageSize  int       `form:"page_size,default=50" validate:"min=1,max=1000"`
}

// CancelOrderResponse represents order cancellation response
type CancelOrderResponse struct {
	OrderID       string          `json:"order_id"`
	ClientOrderID string          `json:"client_order_id,omitempty"`
	Symbol        string          `json:"symbol"`
	Status        string          `json:"status"`
	ExecutedQty   decimal.Decimal `json:"executed_qty"`
	RemainingQty  decimal.Decimal `json:"remaining_qty"`
	CancelledAt   time.Time       `json:"cancelled_at"`
}

// PlaceOrder handles order placement requests
// @Summary Place a new order
// @Description Place a buy or sell order with various types and options
// @Tags Trading
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body PlaceOrderRequest true "Order placement request"
// @Success 201 {object} PlaceOrderResponse "Order placed successfully"
// @Failure 400 {object} map[string]interface{} "Invalid request"
// @Failure 401 {object} map[string]interface{} "Unauthorized"
// @Failure 403 {object} map[string]interface{} "Insufficient balance or compliance violation"
// @Failure 429 {object} map[string]interface{} "Rate limit exceeded"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Router /v1/trading/orders [post]
func (h *TradingHandler) PlaceOrder(c *gin.Context) {
	var req PlaceOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("Invalid order placement request", zap.Error(err))
		c.JSON(http.StatusBadRequest, apiutil.ErrorResponse{
			Error:   "invalid_request",
			Message: "Invalid order parameters",
			Details: err.Error(),
		})
		return
	}

	// Get user ID from auth context
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, apiutil.ErrorResponse{
			Error:   "unauthorized",
			Message: "User authentication required",
		})
		return
	}

	userUUID, err := uuid.Parse(userID.(string))
	if err != nil {
		h.logger.Error("Invalid user ID format", zap.Error(err))
		c.JSON(http.StatusBadRequest, apiutil.ErrorResponse{
			Error:   "invalid_user_id",
			Message: "Invalid user ID format",
		})
		return
	}

	// Validate price for limit orders
	if (req.Type == "limit" || strings.Contains(req.Type, "limit")) && req.Price.IsZero() {
		c.JSON(http.StatusBadRequest, apiutil.ErrorResponse{
			Error:   "invalid_price",
			Message: "Price is required for limit orders",
		})
		return
	}

	// Validate stop price for stop orders
	if strings.Contains(req.Type, "stop") && req.StopPrice.IsZero() {
		c.JSON(http.StatusBadRequest, apiutil.ErrorResponse{
			Error:   "invalid_stop_price",
			Message: "Stop price is required for stop orders",
		})
		return
	}

	// Create order model
	order := &models.Order{
		ID:          uuid.New(),
		UserID:      userUUID,
		Symbol:      req.Symbol,
		Side:        req.Side,
		Type:        req.Type,
		Quantity:    req.Quantity.InexactFloat64(),
		Price:       req.Price.InexactFloat64(),
		TimeInForce: req.TimeInForce,
		Status:      "NEW",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// Set default time in force
	if order.TimeInForce == "" {
		order.TimeInForce = "GTC"
	}

	// Place order through service
	placedOrder, err := h.tradingService.PlaceOrder(c.Request.Context(), order)
	if err != nil {
		h.logger.Error("Failed to place order",
			zap.Error(err),
			zap.String("user_id", userID.(string)),
			zap.String("symbol", req.Symbol),
			zap.String("side", req.Side),
			zap.String("type", req.Type),
		)

		// Handle specific error types
		if strings.Contains(err.Error(), "insufficient") {
			c.JSON(http.StatusForbidden, apiutil.ErrorResponse{
				Error:   "insufficient_balance",
				Message: "Insufficient balance to place order",
			})
			return
		}

		if strings.Contains(err.Error(), "compliance") {
			c.JSON(http.StatusForbidden, apiutil.ErrorResponse{
				Error:   "compliance_violation",
				Message: "Order violates compliance rules",
			})
			return
		}

		c.JSON(http.StatusInternalServerError, apiutil.ErrorResponse{
			Error:   "order_placement_failed",
			Message: "Failed to place order",
		})
		return
	}

	// Build response
	response := PlaceOrderResponse{
		OrderID:       placedOrder.ID.String(),
		ClientOrderID: req.ClientOrderID,
		Symbol:        placedOrder.Symbol,
		Side:          placedOrder.Side,
		Type:          placedOrder.Type,
		Quantity:      decimal.NewFromFloat(placedOrder.Quantity),
		Price:         decimal.NewFromFloat(placedOrder.Price),
		Status:        placedOrder.Status,
		TimeInForce:   placedOrder.TimeInForce,
		CreatedAt:     placedOrder.CreatedAt,
		Fills:         []OrderFill{}, // TODO: Add fills from trade results
	}

	h.logger.Info("Order placed successfully",
		zap.String("order_id", response.OrderID),
		zap.String("user_id", userID.(string)),
		zap.String("symbol", req.Symbol),
		zap.String("status", response.Status),
	)

	c.JSON(http.StatusCreated, response)
}

// GetOrders handles order listing requests
// @Summary List orders
// @Description Retrieve user's orders with optional filtering
// @Tags Trading
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param symbol query string false "Trading pair symbol"
// @Param status query string false "Order status filter" Enums(open,filled,cancelled,expired,partially_filled)
// @Param side query string false "Order side filter" Enums(buy,sell)
// @Param start_time query string false "Start time filter (RFC3339)"
// @Param end_time query string false "End time filter (RFC3339)"
// @Param page query int false "Page number" default(1) minimum(1)
// @Param page_size query int false "Page size" default(50) minimum(1) maximum(1000)
// @Success 200 {object} map[string]interface{} "Orders retrieved successfully"
// @Failure 400 {object} map[string]interface{} "Invalid request parameters"
// @Failure 401 {object} map[string]interface{} "Unauthorized"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Router /v1/trading/orders [get]
func (h *TradingHandler) GetOrders(c *gin.Context) {
	var req ListOrdersRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		h.logger.Warn("Invalid order listing request", zap.Error(err))
		c.JSON(http.StatusBadRequest, apiutil.ErrorResponse{
			Error:   "invalid_request",
			Message: "Invalid query parameters",
			Details: err.Error(),
		})
		return
	}

	// Get user ID from auth context
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, apiutil.ErrorResponse{
			Error:   "unauthorized",
			Message: "User authentication required",
		})
		return
	}

	// Set defaults
	if req.Page < 1 {
		req.Page = 1
	}
	if req.PageSize < 1 || req.PageSize > 1000 {
		req.PageSize = 50
	}

	// Calculate offset
	offset := (req.Page - 1) * req.PageSize

	// Get orders from service
	orders, total, err := h.tradingService.GetOrders(
		userID.(string),
		req.Symbol,
		req.Status,
		strconv.Itoa(req.PageSize),
		strconv.Itoa(offset),
	)
	if err != nil {
		h.logger.Error("Failed to get orders",
			zap.Error(err),
			zap.String("user_id", userID.(string)),
		)
		c.JSON(http.StatusInternalServerError, apiutil.ErrorResponse{
			Error:   "orders_retrieval_failed",
			Message: "Failed to retrieve orders",
		})
		return
	}

	// Build response
	response := gin.H{
		"orders": orders,
		"pagination": gin.H{
			"page":        req.Page,
			"page_size":   req.PageSize,
			"total":       total,
			"total_pages": (total + int64(req.PageSize) - 1) / int64(req.PageSize),
		},
	}

	c.JSON(http.StatusOK, response)
}

// GetOrder handles single order retrieval requests
// @Summary Get order details
// @Description Retrieve details of a specific order
// @Tags Trading
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Order ID (UUID)"
// @Success 200 {object} models.Order "Order details"
// @Failure 400 {object} map[string]interface{} "Invalid order ID"
// @Failure 401 {object} map[string]interface{} "Unauthorized"
// @Failure 404 {object} map[string]interface{} "Order not found"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Router /v1/trading/orders/{id} [get]
func (h *TradingHandler) GetOrder(c *gin.Context) {
	orderID := c.Param("id")

	// Validate order ID format
	if _, err := uuid.Parse(orderID); err != nil {
		c.JSON(http.StatusBadRequest, apiutil.ErrorResponse{
			Error:   "invalid_order_id",
			Message: "Invalid order ID format",
		})
		return
	}

	// Get user ID from auth context
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, apiutil.ErrorResponse{
			Error:   "unauthorized",
			Message: "User authentication required",
		})
		return
	}

	// Get order from service
	order, err := h.tradingService.GetOrder(orderID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, apiutil.ErrorResponse{
				Error:   "order_not_found",
				Message: "Order not found",
			})
			return
		}

		h.logger.Error("Failed to get order",
			zap.Error(err),
			zap.String("user_id", userID.(string)),
			zap.String("order_id", orderID),
		)
		c.JSON(http.StatusInternalServerError, apiutil.ErrorResponse{
			Error:   "order_retrieval_failed",
			Message: "Failed to retrieve order",
		})
		return
	}

	c.JSON(http.StatusOK, order)
}

// CancelOrder handles order cancellation requests
// @Summary Cancel an order
// @Description Cancel an active order
// @Tags Trading
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Order ID (UUID)"
// @Success 200 {object} CancelOrderResponse "Order cancelled successfully"
// @Failure 400 {object} map[string]interface{} "Invalid order ID or order cannot be cancelled"
// @Failure 401 {object} map[string]interface{} "Unauthorized"
// @Failure 404 {object} map[string]interface{} "Order not found"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Router /v1/trading/orders/{id} [delete]
func (h *TradingHandler) CancelOrder(c *gin.Context) {
	orderID := c.Param("id")

	// Validate order ID format
	if _, err := uuid.Parse(orderID); err != nil {
		c.JSON(http.StatusBadRequest, apiutil.ErrorResponse{
			Error:   "invalid_order_id",
			Message: "Invalid order ID format",
		})
		return
	}

	// Get user ID from auth context
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, apiutil.ErrorResponse{
			Error:   "unauthorized",
			Message: "User authentication required",
		})
		return
	}

	// Cancel order through service
	err := h.tradingService.CancelOrder(c.Request.Context(), orderID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, apiutil.ErrorResponse{
				Error:   "order_not_found",
				Message: "Order not found",
			})
			return
		}

		if strings.Contains(err.Error(), "cannot be cancelled") {
			c.JSON(http.StatusBadRequest, apiutil.ErrorResponse{
				Error:   "order_not_cancellable",
				Message: "Order cannot be cancelled in its current state",
			})
			return
		}

		h.logger.Error("Failed to cancel order",
			zap.Error(err),
			zap.String("user_id", userID.(string)),
			zap.String("order_id", orderID),
		)
		c.JSON(http.StatusInternalServerError, apiutil.ErrorResponse{
			Error:   "order_cancellation_failed",
			Message: "Failed to cancel order",
		})
		return
	}

	// Get updated order details for response
	order, err := h.tradingService.GetOrder(orderID)
	if err != nil {
		// Log error but still return success since cancellation worked
		h.logger.Warn("Failed to retrieve cancelled order details", zap.Error(err))
		c.JSON(http.StatusOK, gin.H{
			"order_id":     orderID,
			"status":       "cancelled",
			"cancelled_at": time.Now(),
		})
		return
	}

	response := CancelOrderResponse{
		OrderID:      orderID,
		Symbol:       order.Symbol,
		Status:       order.Status,
		ExecutedQty:  decimal.NewFromFloat(order.FilledQuantity),
		RemainingQty: decimal.NewFromFloat(order.Quantity - order.FilledQuantity),
		CancelledAt:  time.Now(),
	}

	h.logger.Info("Order cancelled successfully",
		zap.String("order_id", orderID),
		zap.String("user_id", userID.(string)),
	)

	c.JSON(http.StatusOK, response)
}
