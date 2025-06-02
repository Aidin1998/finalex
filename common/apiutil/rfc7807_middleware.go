package apiutil

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/litebittech/cex/common/errors"
)

// RFC7807ErrorMiddleware creates a middleware that handles errors in RFC 7807 format
func RFC7807ErrorMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		// Check if there are any errors to handle
		if len(c.Errors) > 0 {
			err := c.Errors.Last()
			instance := c.Request.URL.Path

			// Get trace ID from context if available
			traceID := GetTraceID(c)

			var problemDetails *errors.ProblemDetails

			// Handle different error types
			switch e := err.Err.(type) {
			case *errors.ProblemDetails:
				// Already an RFC 7807 error
				problemDetails = e
			case *errors.Error:
				// Convert legacy error to RFC 7807
				problemDetails = e.ToProblemDetails(instance)
			case *gin.Error:
				// Convert Gin error to RFC 7807
				problemDetails = ginErrorToProblemDetails(e, instance)
			default:
				// Generic error
				problemDetails = errors.NewInternalError(
					"An unexpected error occurred",
					instance,
				)
			}

			// Add trace ID if available
			if traceID != "" {
				problemDetails.WithTraceID(traceID)
			}

			// Set appropriate headers
			c.Header("Content-Type", "application/problem+json")

			// Return the error response
			c.JSON(problemDetails.Status, problemDetails)
			c.Abort()
		}
	}
}

// ginErrorToProblemDetails converts a Gin error to RFC 7807 ProblemDetails
func ginErrorToProblemDetails(err *gin.Error, instance string) *errors.ProblemDetails {
	switch err.Type {
	case gin.ErrorTypeBind:
		return errors.NewValidationError(
			"Request binding failed: "+err.Error(),
			instance,
		)
	case gin.ErrorTypePublic:
		// Public errors are safe to expose
		return errors.NewValidationError(err.Error(), instance)
	case gin.ErrorTypePrivate:
		// Private errors should not expose details
		return errors.NewInternalError(
			"An internal error occurred",
			instance,
		)
	default:
		return errors.NewInternalError(
			"An unexpected error occurred",
			instance,
		)
	}
}

// GetTraceID extracts trace ID from context
func GetTraceID(c *gin.Context) string {
	if traceID, exists := c.Get("trace_id"); exists {
		if id, ok := traceID.(string); ok {
			return id
		}
	}

	// Try to get from headers
	return c.GetHeader("X-Trace-ID")
}

// RFC7807ErrorResponse writes an RFC 7807 compliant error response
func RFC7807ErrorResponse(c *gin.Context, problemDetails *errors.ProblemDetails) {
	// Add trace ID if available
	if traceID := GetTraceID(c); traceID != "" {
		problemDetails.WithTraceID(traceID)
	}

	c.Header("Content-Type", "application/problem+json")
	c.JSON(problemDetails.Status, problemDetails)
}

// Convenience functions for common errors with RFC 7807 format

// RFC7807ValidationErrorResponse writes a validation error response
func RFC7807ValidationErrorResponse(c *gin.Context, detail string, validationErrors ...errors.ValidationError) {
	problemDetails := errors.NewValidationError(detail, c.Request.URL.Path)
	if len(validationErrors) > 0 {
		problemDetails.WithValidationErrors(validationErrors)
	}
	RFC7807ErrorResponse(c, problemDetails)
}

// RFC7807UnauthorizedResponse writes an unauthorized error response
func RFC7807UnauthorizedResponse(c *gin.Context, detail string) {
	problemDetails := errors.NewUnauthorizedError(detail, c.Request.URL.Path)
	RFC7807ErrorResponse(c, problemDetails)
}

// RFC7807ForbiddenResponse writes a forbidden error response
func RFC7807ForbiddenResponse(c *gin.Context, detail string) {
	problemDetails := errors.NewForbiddenError(detail, c.Request.URL.Path)
	RFC7807ErrorResponse(c, problemDetails)
}

// RFC7807NotFoundResponse writes a not found error response
func RFC7807NotFoundResponse(c *gin.Context, detail string) {
	problemDetails := errors.NewNotFoundError(detail, c.Request.URL.Path)
	RFC7807ErrorResponse(c, problemDetails)
}

// RFC7807RateLimitResponse writes a rate limit error response
func RFC7807RateLimitResponse(c *gin.Context, detail string) {
	problemDetails := errors.NewRateLimitError(detail, c.Request.URL.Path)
	RFC7807ErrorResponse(c, problemDetails)
}

// RFC7807InternalServerErrorResponse writes an internal server error response
func RFC7807InternalServerErrorResponse(c *gin.Context, detail string) {
	problemDetails := errors.NewInternalError(detail, c.Request.URL.Path)
	RFC7807ErrorResponse(c, problemDetails)
}

// Legacy compatibility function
func WriteErrorResponseRFC7807(c *gin.Context, status int, code, message string, details interface{}) {
	var problemType, title string

	switch status {
	case http.StatusBadRequest:
		problemType = errors.TypeValidationError
		title = errors.TitleValidationError
	case http.StatusUnauthorized:
		problemType = errors.TypeUnauthorized
		title = errors.TitleUnauthorized
	case http.StatusForbidden:
		problemType = errors.TypeForbidden
		title = errors.TitleForbidden
	case http.StatusNotFound:
		problemType = errors.TypeNotFound
		title = errors.TitleNotFound
	case http.StatusTooManyRequests:
		problemType = errors.TypeRateLimit
		title = errors.TitleRateLimit
	default:
		problemType = errors.TypeInternalError
		title = errors.TitleInternalError
	}

	problemDetails := errors.NewProblemDetails(
		problemType,
		title,
		status,
		message,
		c.Request.URL.Path,
	)

	if details != nil {
		switch d := details.(type) {
		case string:
			problemDetails.Detail = problemDetails.Detail + ": " + d
		}
	}

	RFC7807ErrorResponse(c, problemDetails)
}
