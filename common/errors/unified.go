package errors

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

// UnifiedErrorHandler provides a single interface for all error handling in the platform
type UnifiedErrorHandler struct{}

// NewUnifiedErrorHandler creates a new unified error handler
func NewUnifiedErrorHandler() *UnifiedErrorHandler {
	return &UnifiedErrorHandler{}
}

// HandleError processes any error type and converts it to RFC 7807 format
func (h *UnifiedErrorHandler) HandleError(c *gin.Context, err error) {
	instance := c.Request.URL.Path
	var problemDetails *ProblemDetails

	switch e := err.(type) {
	case *ProblemDetails:
		// Already RFC 7807 compliant
		problemDetails = e
	case *Error:
		// Convert legacy custom error to RFC 7807
		problemDetails = e.ToProblemDetails(instance)
	case StatusCode:
		// Convert status code error to RFC 7807
		problemDetails = h.statusCodeToProblemDetails(int(e), instance)
	default:
		// Generic error - create internal server error
		problemDetails = NewInternalError(err.Error(), instance)
	}

	// Add trace ID if available
	if traceID := h.getTraceID(c); traceID != "" {
		problemDetails.WithTraceID(traceID)
	}

	// Write RFC 7807 compliant response
	c.Header("Content-Type", "application/problem+json")
	c.JSON(problemDetails.Status, problemDetails)
}

// CreateValidationError creates a validation error with multiple field errors
func (h *UnifiedErrorHandler) CreateValidationError(instance string, fieldErrors ...ValidationError) *ProblemDetails {
	problemDetails := NewValidationError("Request validation failed", instance)
	if len(fieldErrors) > 0 {
		problemDetails.WithValidationErrors(fieldErrors)
	}
	return problemDetails
}

// CreateBusinessError creates a business logic error
func (h *UnifiedErrorHandler) CreateBusinessError(errorType, detail, instance string) *ProblemDetails {
	switch errorType {
	case "insufficient_funds":
		return NewInsufficientFundsError(detail, instance)
	case "invalid_order":
		return NewInvalidOrderError(detail, instance)
	case "market_closed":
		return NewMarketClosedError(detail, instance)
	case "kyc_required":
		return NewKYCRequiredError(detail, instance)
	case "mfa_required":
		return NewMFARequiredError(detail, instance)
	case "aml_blocked":
		return NewAMLBlockedError(detail, instance)
	case "order_not_found":
		return NewOrderNotFoundError(detail, instance)
	case "invalid_symbol":
		return NewInvalidSymbolError(detail, instance)
	default:
		return NewInternalError(detail, instance)
	}
}

// Middleware creates a Gin middleware for unified error handling
func (h *UnifiedErrorHandler) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		// Check if there are any errors to handle
		if len(c.Errors) > 0 {
			err := c.Errors.Last()
			h.HandleError(c, err.Err)
			c.Abort()
		}
	}
}

// Convenience methods for common errors

// BadRequest creates a validation error response
func (h *UnifiedErrorHandler) BadRequest(c *gin.Context, detail string, fieldErrors ...ValidationError) {
	problemDetails := h.CreateValidationError(c.Request.URL.Path, fieldErrors...)
	if detail != "" {
		problemDetails.Detail = detail
	}
	h.writeResponse(c, problemDetails)
}

// Unauthorized creates an unauthorized error response
func (h *UnifiedErrorHandler) Unauthorized(c *gin.Context, detail string) {
	problemDetails := NewUnauthorizedError(detail, c.Request.URL.Path)
	h.writeResponse(c, problemDetails)
}

// Forbidden creates a forbidden error response
func (h *UnifiedErrorHandler) Forbidden(c *gin.Context, detail string) {
	problemDetails := NewForbiddenError(detail, c.Request.URL.Path)
	h.writeResponse(c, problemDetails)
}

// NotFoundError creates a not found error response
func (h *UnifiedErrorHandler) NotFoundError(c *gin.Context, detail string) {
	problemDetails := NewNotFoundError(detail, c.Request.URL.Path)
	h.writeResponse(c, problemDetails)
}

// RateLimit creates a rate limit error response
func (h *UnifiedErrorHandler) RateLimit(c *gin.Context, detail string) {
	problemDetails := NewRateLimitError(detail, c.Request.URL.Path)
	h.writeResponse(c, problemDetails)
}

// InternalServerError creates an internal server error response
func (h *UnifiedErrorHandler) InternalServerError(c *gin.Context, detail string) {
	problemDetails := NewInternalError(detail, c.Request.URL.Path)
	h.writeResponse(c, problemDetails)
}

// Business logic errors

// InsufficientFunds creates an insufficient funds error response
func (h *UnifiedErrorHandler) InsufficientFunds(c *gin.Context, detail string) {
	problemDetails := NewInsufficientFundsError(detail, c.Request.URL.Path)
	h.writeResponse(c, problemDetails)
}

// InvalidOrder creates an invalid order error response
func (h *UnifiedErrorHandler) InvalidOrder(c *gin.Context, detail string) {
	problemDetails := NewInvalidOrderError(detail, c.Request.URL.Path)
	h.writeResponse(c, problemDetails)
}

// MarketClosed creates a market closed error response
func (h *UnifiedErrorHandler) MarketClosed(c *gin.Context, detail string) {
	problemDetails := NewMarketClosedError(detail, c.Request.URL.Path)
	h.writeResponse(c, problemDetails)
}

// KYCRequired creates a KYC required error response
func (h *UnifiedErrorHandler) KYCRequired(c *gin.Context, detail string) {
	problemDetails := NewKYCRequiredError(detail, c.Request.URL.Path)
	h.writeResponse(c, problemDetails)
}

// MFARequired creates an MFA required error response
func (h *UnifiedErrorHandler) MFARequired(c *gin.Context, detail string) {
	problemDetails := NewMFARequiredError(detail, c.Request.URL.Path)
	h.writeResponse(c, problemDetails)
}

// AMLBlocked creates an AML blocked error response
func (h *UnifiedErrorHandler) AMLBlocked(c *gin.Context, detail string) {
	problemDetails := NewAMLBlockedError(detail, c.Request.URL.Path)
	h.writeResponse(c, problemDetails)
}

// Helper methods

func (h *UnifiedErrorHandler) statusCodeToProblemDetails(status int, instance string) *ProblemDetails {
	switch status {
	case http.StatusBadRequest:
		return NewValidationError("Bad request", instance)
	case http.StatusUnauthorized:
		return NewUnauthorizedError("Unauthorized access", instance)
	case http.StatusForbidden:
		return NewForbiddenError("Access forbidden", instance)
	case http.StatusNotFound:
		return NewNotFoundError("Resource not found", instance)
	case http.StatusTooManyRequests:
		return NewRateLimitError("Rate limit exceeded", instance)
	case http.StatusInternalServerError:
		return NewInternalError("Internal server error", instance)
	default:
		return NewInternalError(fmt.Sprintf("HTTP %d error", status), instance)
	}
}

func (h *UnifiedErrorHandler) getTraceID(c *gin.Context) string {
	if traceID, exists := c.Get("trace_id"); exists {
		if id, ok := traceID.(string); ok {
			return id
		}
	}
	return c.GetHeader("X-Trace-ID")
}

func (h *UnifiedErrorHandler) writeResponse(c *gin.Context, problemDetails *ProblemDetails) {
	// Add trace ID if available
	if traceID := h.getTraceID(c); traceID != "" {
		problemDetails.WithTraceID(traceID)
	}

	c.Header("Content-Type", "application/problem+json")
	c.JSON(problemDetails.Status, problemDetails)
}

// Global unified error handler instance
var DefaultHandler = NewUnifiedErrorHandler()

// Convenience functions that use the default handler

// HandleError processes any error using the default handler
func HandleError(c *gin.Context, err error) {
	DefaultHandler.HandleError(c, err)
}

// UnifiedErrorMiddleware creates a middleware using the default handler
func UnifiedErrorMiddleware() gin.HandlerFunc {
	return DefaultHandler.Middleware()
}

// Convenience response functions

func BadRequest(c *gin.Context, detail string, fieldErrors ...ValidationError) {
	DefaultHandler.BadRequest(c, detail, fieldErrors...)
}

func Unauthorized(c *gin.Context, detail string) {
	DefaultHandler.Unauthorized(c, detail)
}

func Forbidden(c *gin.Context, detail string) {
	DefaultHandler.Forbidden(c, detail)
}

func NotFoundError(c *gin.Context, detail string) {
	DefaultHandler.NotFoundError(c, detail)
}

func RateLimit(c *gin.Context, detail string) {
	DefaultHandler.RateLimit(c, detail)
}

func InternalServerError(c *gin.Context, detail string) {
	DefaultHandler.InternalServerError(c, detail)
}

func InsufficientFunds(c *gin.Context, detail string) {
	DefaultHandler.InsufficientFunds(c, detail)
}

func InvalidOrder(c *gin.Context, detail string) {
	DefaultHandler.InvalidOrder(c, detail)
}

func MarketClosed(c *gin.Context, detail string) {
	DefaultHandler.MarketClosed(c, detail)
}

func KYCRequired(c *gin.Context, detail string) {
	DefaultHandler.KYCRequired(c, detail)
}

func MFARequired(c *gin.Context, detail string) {
	DefaultHandler.MFARequired(c, detail)
}

func AMLBlocked(c *gin.Context, detail string) {
	DefaultHandler.AMLBlocked(c, detail)
}
