package crosspair

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// HTTPHandler provides HTTP endpoints for cross-pair trading
type HTTPHandler struct {
	logger   *zap.Logger
	engine   *CrossPairEngine
	rateCalc *SyntheticRateCalculator
}

// NewHTTPHandler creates a new HTTP handler
func NewHTTPHandler(logger *zap.Logger, engine *CrossPairEngine, rateCalc *SyntheticRateCalculator) *HTTPHandler {
	return &HTTPHandler{
		logger:   logger.Named("crosspair-http"),
		engine:   engine,
		rateCalc: rateCalc,
	}
}

// RegisterRoutes registers HTTP routes for cross-pair trading
func (h *HTTPHandler) RegisterRoutes(router *gin.RouterGroup) {
	v1 := router.Group("/v1/crosspair")
	{
		// Order management
		v1.POST("/orders", h.CreateOrder)
		v1.GET("/orders/:orderID", h.GetOrder)
		v1.DELETE("/orders/:orderID", h.CancelOrder)
		v1.GET("/users/:userID/orders", h.GetUserOrders)
		v1.GET("/users/:userID/trades", h.GetUserTrades)

		// Rate and quote endpoints
		v1.GET("/quote", h.GetRateQuote)
		v1.GET("/routes", h.GetAvailableRoutes)

		// Engine status and metrics
		v1.GET("/status", h.GetEngineStatus)
		v1.GET("/metrics", h.GetMetrics)
	}
}

// CreateOrderRequest represents the HTTP request for creating an order
type CreateOrderHTTPRequest struct {
	FromAsset   string  `json:"from_asset" binding:"required"`
	ToAsset     string  `json:"to_asset" binding:"required"`
	Type        string  `json:"type" binding:"required,oneof=MARKET LIMIT"`
	Side        string  `json:"side" binding:"required,oneof=BUY SELL"`
	Quantity    string  `json:"quantity" binding:"required"`
	Price       *string `json:"price,omitempty"`
	MaxSlippage string  `json:"max_slippage" binding:"required"`
	TimeInForce string  `json:"time_in_force,omitempty"`
	ExpiresAt   *int64  `json:"expires_at,omitempty"`
}

// CreateOrder handles POST /v1/crosspair/orders
func (h *HTTPHandler) CreateOrder(c *gin.Context) {
	var req CreateOrderHTTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.respondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	// Extract user ID from context (would be set by authentication middleware)
	userID, exists := c.Get("user_id")
	if !exists {
		h.respondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "user not authenticated")
		return
	}

	userUUID, ok := userID.(uuid.UUID)
	if !ok {
		h.respondError(c, http.StatusInternalServerError, "INVALID_USER_ID", "invalid user ID format")
		return
	}

	// Parse request parameters
	quantity, err := decimal.NewFromString(req.Quantity)
	if err != nil {
		h.respondError(c, http.StatusBadRequest, "INVALID_QUANTITY", "invalid quantity format")
		return
	}

	maxSlippage, err := decimal.NewFromString(req.MaxSlippage)
	if err != nil {
		h.respondError(c, http.StatusBadRequest, "INVALID_SLIPPAGE", "invalid max_slippage format")
		return
	}

	var price *decimal.Decimal
	if req.Price != nil {
		p, err := decimal.NewFromString(*req.Price)
		if err != nil {
			h.respondError(c, http.StatusBadRequest, "INVALID_PRICE", "invalid price format")
			return
		}
		price = &p
	}

	var expiresAt *time.Time
	if req.ExpiresAt != nil {
		t := time.Unix(*req.ExpiresAt, 0)
		expiresAt = &t
	}

	// Create order request
	orderReq := &CreateOrderRequest{
		UserID:      userUUID,
		FromAsset:   req.FromAsset,
		ToAsset:     req.ToAsset,
		Type:        CrossPairOrderType(req.Type),
		Side:        CrossPairOrderSide(req.Side),
		Quantity:    quantity,
		Price:       price,
		MaxSlippage: maxSlippage,
		TimeInForce: req.TimeInForce,
		ExpiresAt:   expiresAt,
	}

	// Create order
	order, err := h.engine.CreateOrder(c.Request.Context(), orderReq)
	if err != nil {
		if crossPairErr, ok := err.(*CrossPairError); ok {
			h.respondError(c, http.StatusBadRequest, crossPairErr.Code, crossPairErr.Message)
		} else {
			h.logger.Error("failed to create cross-pair order", zap.Error(err))
			h.respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to create order")
		}
		return
	}

	h.respondSuccess(c, http.StatusCreated, order)
}

// GetOrder handles GET /v1/crosspair/orders/:orderID
func (h *HTTPHandler) GetOrder(c *gin.Context) {
	orderIDStr := c.Param("orderID")
	orderID, err := uuid.Parse(orderIDStr)
	if err != nil {
		h.respondError(c, http.StatusBadRequest, "INVALID_ORDER_ID", "invalid order ID format")
		return
	}

	userID, exists := c.Get("user_id")
	if !exists {
		h.respondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "user not authenticated")
		return
	}

	userUUID := userID.(uuid.UUID)

	order, err := h.engine.GetOrder(c.Request.Context(), orderID, userUUID)
	if err != nil {
		if crossPairErr, ok := err.(*CrossPairError); ok {
			if crossPairErr.Code == "UNAUTHORIZED" {
				h.respondError(c, http.StatusForbidden, crossPairErr.Code, crossPairErr.Message)
			} else {
				h.respondError(c, http.StatusNotFound, crossPairErr.Code, crossPairErr.Message)
			}
		} else {
			h.logger.Error("failed to get cross-pair order", zap.Error(err))
			h.respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to get order")
		}
		return
	}

	h.respondSuccess(c, http.StatusOK, order)
}

// CancelOrder handles DELETE /v1/crosspair/orders/:orderID
func (h *HTTPHandler) CancelOrder(c *gin.Context) {
	orderIDStr := c.Param("orderID")
	orderID, err := uuid.Parse(orderIDStr)
	if err != nil {
		h.respondError(c, http.StatusBadRequest, "INVALID_ORDER_ID", "invalid order ID format")
		return
	}

	userID, exists := c.Get("user_id")
	if !exists {
		h.respondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "user not authenticated")
		return
	}

	userUUID := userID.(uuid.UUID)

	err = h.engine.CancelOrder(c.Request.Context(), orderID, userUUID)
	if err != nil {
		if crossPairErr, ok := err.(*CrossPairError); ok {
			status := http.StatusBadRequest
			if crossPairErr.Code == "UNAUTHORIZED" {
				status = http.StatusForbidden
			} else if crossPairErr.Code == "ORDER_NOT_FOUND" {
				status = http.StatusNotFound
			}
			h.respondError(c, status, crossPairErr.Code, crossPairErr.Message)
		} else {
			h.logger.Error("failed to cancel cross-pair order", zap.Error(err))
			h.respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to cancel order")
		}
		return
	}

	h.respondSuccess(c, http.StatusOK, gin.H{"message": "order canceled successfully"})
}

// GetUserOrders handles GET /v1/crosspair/users/:userID/orders
func (h *HTTPHandler) GetUserOrders(c *gin.Context) {
	userIDStr := c.Param("userID")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		h.respondError(c, http.StatusBadRequest, "INVALID_USER_ID", "invalid user ID format")
		return
	}

	// Verify user can access these orders (in a real system, you'd check permissions)
	authUserID, exists := c.Get("user_id")
	if !exists {
		h.respondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "user not authenticated")
		return
	}

	if authUserID.(uuid.UUID) != userID {
		h.respondError(c, http.StatusForbidden, "FORBIDDEN", "cannot access other user's orders")
		return
	}

	// Parse pagination parameters
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	if limit > 100 {
		limit = 100 // Cap limit
	}

	orders, err := h.engine.GetUserOrders(c.Request.Context(), userID, limit, offset)
	if err != nil {
		h.logger.Error("failed to get user orders", zap.Error(err))
		h.respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to get orders")
		return
	}

	h.respondSuccess(c, http.StatusOK, gin.H{
		"orders": orders,
		"limit":  limit,
		"offset": offset,
		"count":  len(orders),
	})
}

// GetUserTrades handles GET /v1/crosspair/users/:userID/trades
func (h *HTTPHandler) GetUserTrades(c *gin.Context) {
	userIDStr := c.Param("userID")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		h.respondError(c, http.StatusBadRequest, "INVALID_USER_ID", "invalid user ID format")
		return
	}

	// Verify user can access these trades
	authUserID, exists := c.Get("user_id")
	if !exists {
		h.respondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "user not authenticated")
		return
	}

	if authUserID.(uuid.UUID) != userID {
		h.respondError(c, http.StatusForbidden, "FORBIDDEN", "cannot access other user's trades")
		return
	}

	// Parse pagination parameters
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	if limit > 100 {
		limit = 100 // Cap limit
	}

	trades, err := h.engine.GetUserTrades(c.Request.Context(), userID, limit, offset)
	if err != nil {
		h.logger.Error("failed to get user trades", zap.Error(err))
		h.respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to get trades")
		return
	}

	h.respondSuccess(c, http.StatusOK, gin.H{
		"trades": trades,
		"limit":  limit,
		"offset": offset,
		"count":  len(trades),
	})
}

// GetRateQuote handles GET /v1/crosspair/quote
func (h *HTTPHandler) GetRateQuote(c *gin.Context) {
	fromAsset := c.Query("from_asset")
	toAsset := c.Query("to_asset")
	quantityStr := c.Query("quantity")

	if fromAsset == "" || toAsset == "" || quantityStr == "" {
		h.respondError(c, http.StatusBadRequest, "MISSING_PARAMETERS", "from_asset, to_asset, and quantity are required")
		return
	}

	quantity, err := decimal.NewFromString(quantityStr)
	if err != nil {
		h.respondError(c, http.StatusBadRequest, "INVALID_QUANTITY", "invalid quantity format")
		return
	}

	quote, err := h.engine.GetRateQuote(c.Request.Context(), fromAsset, toAsset, quantity)
	if err != nil {
		if crossPairErr, ok := err.(*CrossPairError); ok {
			h.respondError(c, http.StatusBadRequest, crossPairErr.Code, crossPairErr.Message)
		} else {
			h.logger.Error("failed to get rate quote", zap.Error(err))
			h.respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to get quote")
		}
		return
	}

	h.respondSuccess(c, http.StatusOK, quote)
}

// GetAvailableRoutes handles GET /v1/crosspair/routes
func (h *HTTPHandler) GetAvailableRoutes(c *gin.Context) {
	routes, err := h.engine.GetAvailableRoutes(c.Request.Context())
	if err != nil {
		h.logger.Error("failed to get available routes", zap.Error(err))
		h.respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to get routes")
		return
	}

	h.respondSuccess(c, http.StatusOK, gin.H{"routes": routes})
}

// GetEngineStatus handles GET /v1/crosspair/status
func (h *HTTPHandler) GetEngineStatus(c *gin.Context) {
	status := h.engine.GetEngineStatus()
	cacheStats := h.rateCalc.GetCacheStats()

	response := gin.H{
		"engine": status,
		"cache":  cacheStats,
		"uptime": time.Since(time.Now()).String(), // This would be actual uptime in production
	}

	h.respondSuccess(c, http.StatusOK, response)
}

// GetMetrics handles GET /v1/crosspair/metrics
func (h *HTTPHandler) GetMetrics(c *gin.Context) {
	// In a real implementation, this would gather comprehensive metrics
	// For now, return basic status information
	metrics := gin.H{
		"orders_processed":   "not_implemented",
		"avg_execution_time": "not_implemented",
		"success_rate":       "not_implemented",
		"total_volume":       "not_implemented",
	}

	h.respondSuccess(c, http.StatusOK, metrics)
}

// Error response structure
type ErrorResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	Timestamp time.Time `json:"timestamp"`
}

// Success response structure
type SuccessResponse struct {
	Data      interface{} `json:"data"`
	Timestamp time.Time   `json:"timestamp"`
}

// respondError sends an error response
func (h *HTTPHandler) respondError(c *gin.Context, status int, code, message string) {
	response := ErrorResponse{
		Timestamp: time.Now(),
	}
	response.Error.Code = code
	response.Error.Message = message

	c.JSON(status, response)
}

// respondSuccess sends a success response
func (h *HTTPHandler) respondSuccess(c *gin.Context, status int, data interface{}) {
	response := SuccessResponse{
		Data:      data,
		Timestamp: time.Now(),
	}

	c.JSON(status, response)
}

// Middleware for authentication (placeholder implementation)
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// In a real implementation, this would:
		// 1. Extract JWT token from Authorization header
		// 2. Validate the token
		// 3. Extract user ID from token claims
		// 4. Set user_id in context

		// For now, use a mock user ID from header
		userIDStr := c.GetHeader("X-User-ID")
		if userIDStr == "" {
			c.JSON(http.StatusUnauthorized, ErrorResponse{
				Timestamp: time.Now(),
				Error: struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				}{
					Code:    "MISSING_USER_ID",
					Message: "X-User-ID header is required",
				},
			})
			c.Abort()
			return
		}

		userID, err := uuid.Parse(userIDStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Timestamp: time.Now(),
				Error: struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				}{
					Code:    "INVALID_USER_ID",
					Message: "invalid user ID format",
				},
			})
			c.Abort()
			return
		}

		c.Set("user_id", userID)
		c.Next()
	}
}

// Middleware for logging requests
func LoggingMiddleware(logger *zap.Logger) gin.HandlerFunc {
	return gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		logger.Info("http request",
			zap.String("method", param.Method),
			zap.String("path", param.Path),
			zap.Int("status", param.StatusCode),
			zap.Duration("latency", param.Latency),
			zap.String("client_ip", param.ClientIP),
		)
		return ""
	})
}

// Middleware for rate limiting (placeholder implementation)
func RateLimitMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// In a real implementation, this would implement rate limiting
		// based on user ID or IP address
		c.Next()
	}
}

// SetupCrossPairAPI sets up the complete API with all middleware
func SetupCrossPairAPI(
	logger *zap.Logger,
	engine *CrossPairEngine,
	rateCalc *SyntheticRateCalculator,
) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)

	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(LoggingMiddleware(logger))
	router.Use(RateLimitMiddleware())

	// Health check endpoint
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":    "healthy",
			"timestamp": time.Now(),
			"service":   "cross-pair-trading",
		})
	})

	// API routes with authentication
	api := router.Group("/api")
	api.Use(AuthMiddleware())

	// Register cross-pair routes
	handler := NewHTTPHandler(logger, engine, rateCalc)
	handler.RegisterRoutes(api)

	return router
}

// Example usage documentation
const APIDocumentation = `
Cross-Pair Trading API Documentation
===================================

Authentication:
- Include X-User-ID header with a valid UUID

Endpoints:

1. Create Order
   POST /api/v1/crosspair/orders
   Body: {
     "from_asset": "BTC",
     "to_asset": "ETH", 
     "type": "MARKET",
     "side": "BUY",
     "quantity": "0.1",
     "max_slippage": "0.02"
   }

2. Get Order
   GET /api/v1/crosspair/orders/{orderID}

3. Cancel Order
   DELETE /api/v1/crosspair/orders/{orderID}

4. Get User Orders
   GET /api/v1/crosspair/users/{userID}/orders?limit=50&offset=0

5. Get User Trades
   GET /api/v1/crosspair/users/{userID}/trades?limit=50&offset=0

6. Get Rate Quote
   GET /api/v1/crosspair/quote?from_asset=BTC&to_asset=ETH&quantity=0.1

7. Get Available Routes
   GET /api/v1/crosspair/routes

8. Get Engine Status
   GET /api/v1/crosspair/status

9. Get Metrics
   GET /api/v1/crosspair/metrics

Error Response Format:
{
  "error": {
    "code": "ERROR_CODE",
    "message": "Error description"
  },
  "timestamp": "2023-01-01T00:00:00Z"
}

Success Response Format:
{
  "data": { ... },
  "timestamp": "2023-01-01T00:00:00Z"
}
`
