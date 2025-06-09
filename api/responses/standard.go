package responses

import (
	"net/http"
	"time"

	"github.com/Aidin1998/finalex/pkg/errors"
	"github.com/gin-gonic/gin"
)

// StandardResponse represents a standard API response format
type StandardResponse struct {
	Success   bool        `json:"success"`
	Data      interface{} `json:"data,omitempty"`
	Message   string      `json:"message,omitempty"`
	Timestamp time.Time   `json:"timestamp"`
	TraceID   string      `json:"trace_id,omitempty"`
}

// PaginatedResponse represents a paginated API response
type PaginatedResponse struct {
	StandardResponse
	Pagination *PaginationMeta `json:"pagination,omitempty"`
}

// PaginationMeta contains pagination metadata
type PaginationMeta struct {
	CurrentPage  int   `json:"current_page"`
	PerPage      int   `json:"per_page"`
	TotalPages   int   `json:"total_pages"`
	TotalRecords int64 `json:"total_records"`
	HasNext      bool  `json:"has_next"`
	HasPrev      bool  `json:"has_prev"`
}

// Success sends a successful response
func Success(c *gin.Context, data interface{}, message ...string) {
	msg := "Operation successful"
	if len(message) > 0 && message[0] != "" {
		msg = message[0]
	}

	response := StandardResponse{
		Success:   true,
		Data:      data,
		Message:   msg,
		Timestamp: time.Now().UTC(),
		TraceID:   getTraceID(c),
	}

	c.JSON(http.StatusOK, response)
}

// Created sends a 201 Created response
func Created(c *gin.Context, data interface{}, message ...string) {
	msg := "Resource created successfully"
	if len(message) > 0 && message[0] != "" {
		msg = message[0]
	}

	response := StandardResponse{
		Success:   true,
		Data:      data,
		Message:   msg,
		Timestamp: time.Now().UTC(),
		TraceID:   getTraceID(c),
	}

	c.JSON(http.StatusCreated, response)
}

// Accepted sends a 202 Accepted response
func Accepted(c *gin.Context, data interface{}, message ...string) {
	msg := "Request accepted for processing"
	if len(message) > 0 && message[0] != "" {
		msg = message[0]
	}

	response := StandardResponse{
		Success:   true,
		Data:      data,
		Message:   msg,
		Timestamp: time.Now().UTC(),
		TraceID:   getTraceID(c),
	}

	c.JSON(http.StatusAccepted, response)
}

// NoContent sends a 204 No Content response
func NoContent(c *gin.Context) {
	c.Status(http.StatusNoContent)
}

// Paginated sends a paginated response
func Paginated(c *gin.Context, data interface{}, pagination *PaginationMeta, message ...string) {
	msg := "Data retrieved successfully"
	if len(message) > 0 && message[0] != "" {
		msg = message[0]
	}

	response := PaginatedResponse{
		StandardResponse: StandardResponse{
			Success:   true,
			Data:      data,
			Message:   msg,
			Timestamp: time.Now().UTC(),
			TraceID:   getTraceID(c),
		},
		Pagination: pagination,
	}

	c.JSON(http.StatusOK, response)
}

// Error sends an error response using RFC 7807 format
func Error(c *gin.Context, problemDetails *errors.ProblemDetails) {
	// Add trace ID if available and not already set
	if problemDetails.TraceID == "" {
		if traceID := getTraceID(c); traceID != "" {
			problemDetails.WithTraceID(traceID)
		}
	}

	// Add timestamp if not already present
	if problemDetails.Extra == nil {
		problemDetails.WithExtra("timestamp", time.Now().UTC().Format(time.RFC3339))
	}

	c.Header("Content-Type", "application/problem+json")
	c.JSON(problemDetails.Status, problemDetails)
}

// BadRequest sends a 400 Bad Request response
func BadRequest(c *gin.Context, detail string, validationErrors ...errors.ValidationError) {
	problemDetails := errors.NewValidationError(detail, c.Request.URL.Path)
	if len(validationErrors) > 0 {
		problemDetails.WithValidationErrors(validationErrors)
	}
	Error(c, problemDetails)
}

// Unauthorized sends a 401 Unauthorized response
func Unauthorized(c *gin.Context, detail string) {
	problemDetails := errors.NewUnauthorizedError(detail, c.Request.URL.Path)
	Error(c, problemDetails)
}

// Forbidden sends a 403 Forbidden response
func Forbidden(c *gin.Context, detail string) {
	problemDetails := errors.NewForbiddenError(detail, c.Request.URL.Path)
	Error(c, problemDetails)
}

// NotFound sends a 404 Not Found response
func NotFound(c *gin.Context, detail string) {
	problemDetails := errors.NewNotFoundError(detail, c.Request.URL.Path)
	Error(c, problemDetails)
}

// Conflict sends a 409 Conflict response
func Conflict(c *gin.Context, detail string) {
	problemDetails := errors.NewConflictError(detail, c.Request.URL.Path)
	Error(c, problemDetails)
}

// UnprocessableEntity sends a 422 Unprocessable Entity response
func UnprocessableEntity(c *gin.Context, detail string, validationErrors ...errors.ValidationError) {
	problemDetails := errors.NewValidationError(detail, c.Request.URL.Path)
	problemDetails.Status = http.StatusUnprocessableEntity
	if len(validationErrors) > 0 {
		problemDetails.WithValidationErrors(validationErrors)
	}
	Error(c, problemDetails)
}

// TooManyRequests sends a 429 Too Many Requests response
func TooManyRequests(c *gin.Context, detail string) {
	problemDetails := errors.NewRateLimitError(detail, c.Request.URL.Path)
	Error(c, problemDetails)
}

// InternalServerError sends a 500 Internal Server Error response
func InternalServerError(c *gin.Context, detail string) {
	problemDetails := errors.NewInternalError(detail, c.Request.URL.Path)
	Error(c, problemDetails)
}

// BadGateway sends a 502 Bad Gateway response
func BadGateway(c *gin.Context, detail string) {
	problemDetails := errors.NewBadGatewayError(detail, c.Request.URL.Path)
	Error(c, problemDetails)
}

// ServiceUnavailable sends a 503 Service Unavailable response
func ServiceUnavailable(c *gin.Context, detail string) {
	problemDetails := errors.NewServiceUnavailableError(detail, c.Request.URL.Path)
	Error(c, problemDetails)
}

// Business-specific error responses for the exchange

// InsufficientFunds sends an insufficient funds error response
func InsufficientFunds(c *gin.Context, detail string) {
	problemDetails := errors.NewInsufficientFundsError(detail, c.Request.URL.Path)
	Error(c, problemDetails)
}

// InvalidOrder sends an invalid order error response
func InvalidOrder(c *gin.Context, detail string) {
	problemDetails := errors.NewInvalidOrderError(detail, c.Request.URL.Path)
	Error(c, problemDetails)
}

// MarketClosed sends a market closed error response
func MarketClosed(c *gin.Context, detail string) {
	problemDetails := errors.NewMarketClosedError(detail, c.Request.URL.Path)
	Error(c, problemDetails)
}

// KYCRequired sends a KYC required error response
func KYCRequired(c *gin.Context, detail string) {
	problemDetails := errors.NewKYCRequiredError(detail, c.Request.URL.Path)
	Error(c, problemDetails)
}

// MFARequired sends an MFA required error response
func MFARequired(c *gin.Context, detail string) {
	problemDetails := errors.NewMFARequiredError(detail, c.Request.URL.Path)
	Error(c, problemDetails)
}

// AMLBlocked sends an AML blocked error response
func AMLBlocked(c *gin.Context, detail string) {
	problemDetails := errors.NewAMLBlockedError(detail, c.Request.URL.Path)
	Error(c, problemDetails)
}

// Helper functions

// getTraceID extracts trace ID from context
func getTraceID(c *gin.Context) string {
	if traceID, exists := c.Get("trace_id"); exists {
		if id, ok := traceID.(string); ok {
			return id
		}
	}

	// Try to get from headers
	return c.GetHeader("X-Trace-ID")
}

// CreatePaginationMeta creates pagination metadata
func CreatePaginationMeta(currentPage, perPage int, totalRecords int64) *PaginationMeta {
	totalPages := int((totalRecords + int64(perPage) - 1) / int64(perPage))
	if totalPages < 1 {
		totalPages = 1
	}

	return &PaginationMeta{
		CurrentPage:  currentPage,
		PerPage:      perPage,
		TotalPages:   totalPages,
		TotalRecords: totalRecords,
		HasNext:      currentPage < totalPages,
		HasPrev:      currentPage > 1,
	}
}
