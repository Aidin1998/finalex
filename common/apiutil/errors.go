package apiutil

import (
	"net/http"

	"github.com/Aidin1998/pincex_unified/common/errors"
	"github.com/labstack/echo/v4"
)

// RFC7807ErrorHandler returns an Echo error handler that formats errors according to RFC 7807
func RFC7807ErrorHandler() echo.HTTPErrorHandler {
	return func(err error, c echo.Context) {
		if c.Response().Committed {
			return
		}

		instance := c.Request().URL.Path
		var problemDetails *errors.ProblemDetails

		// Handle different error types
		switch e := err.(type) {
		case *errors.ProblemDetails:
			// Already an RFC 7807 error
			problemDetails = e
		case *errors.Error:
			// Convert legacy error to RFC 7807
			problemDetails = e.ToProblemDetails(instance)
		case *echo.HTTPError:
			// Convert Echo HTTP error to RFC 7807
			problemDetails = echoErrorToProblemDetails(e, instance)
		case errors.StatusCode:
			// Handle status code errors
			problemDetails = statusCodeToProblemDetails(int(e), instance)
		default:
			// Generic error - don't expose internal details
			problemDetails = errors.NewInternalError(
				"An unexpected error occurred",
				instance,
			)
		}

		// Add trace ID if available
		if traceID := getEchoTraceID(c); traceID != "" {
			problemDetails.WithTraceID(traceID)
		}

		// Set appropriate headers
		c.Response().Header().Set("Content-Type", "application/problem+json")

		// Return the error response
		c.JSON(problemDetails.Status, problemDetails)
	}
}

// echoErrorToProblemDetails converts an Echo HTTP error to RFC 7807 ProblemDetails
func echoErrorToProblemDetails(err *echo.HTTPError, instance string) *errors.ProblemDetails {
	var problemType, title string

	switch err.Code {
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

	detail := "An error occurred"
	if err.Message != nil {
		if msg, ok := err.Message.(string); ok {
			detail = msg
		}
	}

	return errors.NewProblemDetails(problemType, title, err.Code, detail, instance)
}

// statusCodeToProblemDetails converts a status code to RFC 7807 ProblemDetails
func statusCodeToProblemDetails(statusCode int, instance string) *errors.ProblemDetails {
	var problemType, title, detail string

	switch statusCode {
	case http.StatusBadRequest:
		problemType = errors.TypeValidationError
		title = errors.TitleValidationError
		detail = "Invalid request parameters"
	case http.StatusUnauthorized:
		problemType = errors.TypeUnauthorized
		title = errors.TitleUnauthorized
		detail = "Authentication required"
	case http.StatusForbidden:
		problemType = errors.TypeForbidden
		title = errors.TitleForbidden
		detail = "Access denied"
	case http.StatusNotFound:
		problemType = errors.TypeNotFound
		title = errors.TitleNotFound
		detail = "Resource not found"
	case http.StatusTooManyRequests:
		problemType = errors.TypeRateLimit
		title = errors.TitleRateLimit
		detail = "Rate limit exceeded"
	default:
		problemType = errors.TypeInternalError
		title = errors.TitleInternalError
		detail = "Internal server error"
		statusCode = http.StatusInternalServerError
	}

	return errors.NewProblemDetails(problemType, title, statusCode, detail, instance)
}

// getEchoTraceID extracts trace ID from Echo context
func getEchoTraceID(c echo.Context) string {
	// Try to get from context first
	if traceID := c.Get("trace_id"); traceID != nil {
		if id, ok := traceID.(string); ok {
			return id
		}
	}

	// Try to get from headers
	return c.Request().Header.Get("X-Trace-ID")
}

// Legacy error handler for backward compatibility
// Deprecated: Use RFC7807ErrorHandler instead
func ErrorHandler() echo.HTTPErrorHandler {
	return RFC7807ErrorHandler()
}
