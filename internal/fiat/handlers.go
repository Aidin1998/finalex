package fiat

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Handler provides HTTP handlers for fiat operations
type Handler struct {
	service *FiatService
	logger  *zap.Logger
}

// NewHandler creates a new fiat handler
func NewHandler(service *FiatService, logger *zap.Logger) *Handler {
	return &Handler{
		service: service,
		logger:  logger,
	}
}

// DepositReceiptHandler handles incoming deposit receipts from external providers
// @Summary Process external fiat deposit receipt
// @Description Securely process a deposit receipt from an external fiat provider
// @Tags Fiat Receipts
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param X-Trace-ID header string false "Trace ID for request tracking"
// @Param request body FiatReceiptRequest true "Deposit receipt data"
// @Success 200 {object} FiatReceiptResponse "Receipt processed successfully"
// @Success 409 {object} FiatReceiptResponse "Duplicate receipt (idempotent response)"
// @Failure 400 {object} map[string]interface{} "Invalid request data"
// @Failure 401 {object} map[string]interface{} "Invalid provider signature"
// @Failure 404 {object} map[string]interface{} "User not found"
// @Failure 422 {object} map[string]interface{} "User not eligible for deposits"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Router /v1/fiat/receipts [post]
func (h *Handler) DepositReceiptHandler(c *gin.Context) {
	traceID := c.GetHeader("X-Trace-ID")
	if traceID == "" {
		traceID = uuid.New().String()
		c.Header("X-Trace-ID", traceID)
	}

	logger := h.logger.With(
		zap.String("trace_id", traceID),
		zap.String("endpoint", "deposit_receipt"),
		zap.String("method", c.Request.Method),
		zap.String("client_ip", c.ClientIP()),
	)

	logger.Info("Processing deposit receipt request")

	// Parse request
	var req FiatReceiptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Error("Invalid request format", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{
			"error":    "INVALID_REQUEST",
			"message":  "Invalid request format",
			"details":  err.Error(),
			"trace_id": traceID,
		})
		return
	}

	// Set trace ID from header
	req.TraceID = traceID

	// Validate request timing (prevent replay attacks)
	if err := h.validateRequestTiming(req.Timestamp); err != nil {
		logger.Error("Request timing validation failed", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{
			"error":    "INVALID_TIMESTAMP",
			"message":  "Request timestamp is outside acceptable window",
			"trace_id": traceID,
		})
		return
	}

	// Process receipt
	response, err := h.service.ProcessDepositReceipt(c.Request.Context(), &req)
	if err != nil {
		logger.Error("Failed to process deposit receipt", zap.Error(err))

		// Handle specific error types
		switch {
		case contains(err.Error(), "signature validation failed"):
			c.JSON(http.StatusUnauthorized, gin.H{
				"error":    "INVALID_SIGNATURE",
				"message":  "Provider signature validation failed",
				"trace_id": traceID,
			})
		case contains(err.Error(), "user not found"):
			c.JSON(http.StatusNotFound, gin.H{
				"error":    "USER_NOT_FOUND",
				"message":  "User not found",
				"trace_id": traceID,
			})
		case contains(err.Error(), "user validation failed") || contains(err.Error(), "KYC not approved"):
			c.JSON(http.StatusUnprocessableEntity, gin.H{
				"error":    "USER_NOT_ELIGIBLE",
				"message":  "User not eligible for deposits",
				"trace_id": traceID,
			})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":    "PROCESSING_FAILED",
				"message":  "Failed to process deposit receipt",
				"trace_id": traceID,
			})
		}
		return
	}

	// Return response with appropriate status code
	statusCode := http.StatusOK
	if response.Status == ReceiptStatusDuplicate {
		statusCode = http.StatusConflict
	}

	logger.Info("Deposit receipt processed successfully",
		zap.String("receipt_id", response.ReceiptID.String()),
		zap.String("status", string(response.Status)))

	c.JSON(statusCode, response)
}

// GetDepositReceiptHandler retrieves a specific deposit receipt
// @Summary Get deposit receipt by ID
// @Description Retrieve details of a specific deposit receipt
// @Tags Fiat Receipts
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param receipt_id path string true "Receipt ID"
// @Param X-Trace-ID header string false "Trace ID for request tracking"
// @Success 200 {object} FiatDepositReceipt "Receipt details"
// @Failure 400 {object} map[string]interface{} "Invalid receipt ID"
// @Failure 404 {object} map[string]interface{} "Receipt not found"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Router /v1/fiat/receipts/{receipt_id} [get]
func (h *Handler) GetDepositReceiptHandler(c *gin.Context) {
	traceID := c.GetHeader("X-Trace-ID")
	if traceID == "" {
		traceID = uuid.New().String()
		c.Header("X-Trace-ID", traceID)
	}

	receiptIDStr := c.Param("receipt_id")
	receiptID, err := uuid.Parse(receiptIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":    "INVALID_RECEIPT_ID",
			"message":  "Invalid receipt ID format",
			"trace_id": traceID,
		})
		return
	}

	receipt, err := h.service.GetDepositReceipt(c.Request.Context(), receiptID)
	if err != nil {
		if contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{
				"error":    "RECEIPT_NOT_FOUND",
				"message":  "Receipt not found",
				"trace_id": traceID,
			})
		} else {
			h.logger.Error("Failed to retrieve receipt", zap.Error(err), zap.String("trace_id", traceID))
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":    "RETRIEVAL_FAILED",
				"message":  "Failed to retrieve receipt",
				"trace_id": traceID,
			})
		}
		return
	}

	c.JSON(http.StatusOK, receipt)
}

// GetUserDepositsHandler retrieves deposit receipts for a user
// @Summary Get user deposit receipts
// @Description Retrieve deposit receipt history for the authenticated user
// @Tags Fiat Receipts
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param user_id query string false "User ID (admin only)"
// @Param page query int false "Page number" default(1)
// @Param limit query int false "Items per page" default(20)
// @Param X-Trace-ID header string false "Trace ID for request tracking"
// @Success 200 {object} map[string]interface{} "User deposit receipts"
// @Failure 400 {object} map[string]interface{} "Invalid parameters"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Router /v1/fiat/receipts [get]
func (h *Handler) GetUserDepositsHandler(c *gin.Context) {
	traceID := c.GetHeader("X-Trace-ID")
	if traceID == "" {
		traceID = uuid.New().String()
		c.Header("X-Trace-ID", traceID)
	}

	// Get user ID from context (set by auth middleware)
	userIDInterface, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":    "USER_NOT_AUTHENTICATED",
			"message":  "User not authenticated",
			"trace_id": traceID,
		})
		return
	}

	userID, err := uuid.Parse(userIDInterface.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":    "INVALID_USER_ID",
			"message":  "Invalid user ID format",
			"trace_id": traceID,
		})
		return
	}

	// Parse pagination parameters
	page := 1
	if pageStr := c.Query("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	limit := 20
	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	offset := (page - 1) * limit

	receipts, total, err := h.service.GetUserDeposits(c.Request.Context(), userID, limit, offset)
	if err != nil {
		h.logger.Error("Failed to retrieve user deposits",
			zap.Error(err),
			zap.String("user_id", userID.String()),
			zap.String("trace_id", traceID))

		c.JSON(http.StatusInternalServerError, gin.H{
			"error":    "RETRIEVAL_FAILED",
			"message":  "Failed to retrieve deposit receipts",
			"trace_id": traceID,
		})
		return
	}

	response := gin.H{
		"receipts": receipts,
		"pagination": gin.H{
			"page":        page,
			"limit":       limit,
			"total":       total,
			"total_pages": (total + int64(limit) - 1) / int64(limit),
		},
		"trace_id": traceID,
	}

	c.JSON(http.StatusOK, response)
}

// HealthCheckHandler provides health check for the fiat service
// @Summary Fiat service health check
// @Description Check the health status of the fiat service
// @Tags Health
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{} "Service health status"
// @Router /v1/fiat/health [get]
func (h *Handler) HealthCheckHandler(c *gin.Context) {
	status := gin.H{
		"service":   "fiat",
		"status":    "healthy",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}

	// Optional: Add database connectivity check
	if err := h.service.db.Exec("SELECT 1").Error; err != nil {
		status["status"] = "unhealthy"
		status["database"] = "disconnected"
		c.JSON(http.StatusServiceUnavailable, status)
		return
	}

	status["database"] = "connected"
	c.JSON(http.StatusOK, status)
}

// validateRequestTiming validates that the request timestamp is within acceptable window
func (h *Handler) validateRequestTiming(timestamp time.Time) error {
	now := time.Now()
	timeDiff := now.Sub(timestamp)

	// Allow requests within 5 minutes window (prevent replay attacks)
	if timeDiff < -5*time.Minute || timeDiff > 5*time.Minute {
		return fmt.Errorf("request timestamp outside acceptable window: %v", timeDiff)
	}

	return nil
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr ||
			(len(s) > len(substr) &&
				(s[:len(substr)] == substr ||
					s[len(s)-len(substr):] == substr ||
					indexOf(s, substr) >= 0)))
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
