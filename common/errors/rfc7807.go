package errors

import (
	"fmt"
	"net/http"
	"time"
)

// ProblemDetails represents RFC 7807 compliant error response
// RFC 7807: Problem Details for HTTP APIs
// swagger:model
type ProblemDetails struct {
	// Type is a URI reference that identifies the problem type
	Type string `json:"type"`
	// Title is a short, human-readable summary of the problem type
	Title string `json:"title"`
	// Status is the HTTP status code
	Status int `json:"status"`
	// Detail is a human-readable explanation specific to this occurrence of the problem
	Detail string `json:"detail"`
	// Instance is a URI reference that identifies the specific occurrence of the problem
	Instance string `json:"instance,omitempty"`
	// Timestamp when the error occurred
	Timestamp time.Time `json:"timestamp"`
	// TraceID for request tracing and debugging
	TraceID string `json:"traceId,omitempty"`
	// Errors contains field-specific validation errors
	Errors []ValidationError `json:"errors,omitempty"`
}

// ValidationError represents field-specific validation errors
type ValidationError struct {
	// Field name that failed validation
	Field string `json:"field"`
	// Message describing the validation failure
	Message string `json:"message"`
	// Code is machine-readable error code for the field
	Code string `json:"code,omitempty"`
}

// Standard error types with URIs
const (
	TypeValidationError   = "https://api.pincex.com/errors/validation-error"
	TypeUnauthorized      = "https://api.pincex.com/errors/unauthorized"
	TypeForbidden         = "https://api.pincex.com/errors/forbidden"
	TypeNotFound          = "https://api.pincex.com/errors/not-found"
	TypeRateLimit         = "https://api.pincex.com/errors/rate-limit"
	TypeInternalError     = "https://api.pincex.com/errors/internal-error"
	TypeInsufficientFunds = "https://api.pincex.com/errors/insufficient-funds"
	TypeInvalidOrder      = "https://api.pincex.com/errors/invalid-order"
	TypeMarketClosed      = "https://api.pincex.com/errors/market-closed"
	TypeKYCRequired       = "https://api.pincex.com/errors/kyc-required"
	TypeMFARequired       = "https://api.pincex.com/errors/mfa-required"
	TypeAMLBlocked        = "https://api.pincex.com/errors/aml-blocked"
	TypeOrderNotFound     = "https://api.pincex.com/errors/order-not-found"
	TypeInvalidSymbol     = "https://api.pincex.com/errors/invalid-symbol"
)

// Standard error titles
const (
	TitleValidationError   = "Validation Error"
	TitleUnauthorized      = "Unauthorized"
	TitleForbidden         = "Forbidden"
	TitleNotFound          = "Not Found"
	TitleRateLimit         = "Rate Limit Exceeded"
	TitleInternalError     = "Internal Server Error"
	TitleInsufficientFunds = "Insufficient Funds"
	TitleInvalidOrder      = "Invalid Order"
	TitleMarketClosed      = "Market Closed"
	TitleKYCRequired       = "KYC Required"
	TitleMFARequired       = "MFA Required"
	TitleAMLBlocked        = "AML Blocked"
	TitleOrderNotFound     = "Order Not Found"
	TitleInvalidSymbol     = "Invalid Symbol"
)

// NewProblemDetails creates a new RFC 7807 compliant error
func NewProblemDetails(problemType, title string, status int, detail, instance string) *ProblemDetails {
	return &ProblemDetails{
		Type:      problemType,
		Title:     title,
		Status:    status,
		Detail:    detail,
		Instance:  instance,
		Timestamp: time.Now().UTC(),
	}
}

// WithTraceID adds a trace ID to the problem details
func (p *ProblemDetails) WithTraceID(traceID string) *ProblemDetails {
	p.TraceID = traceID
	return p
}

// WithValidationErrors adds validation errors to the problem details
func (p *ProblemDetails) WithValidationErrors(errors []ValidationError) *ProblemDetails {
	p.Errors = errors
	return p
}

// AddValidationError adds a single validation error
func (p *ProblemDetails) AddValidationError(field, message, code string) *ProblemDetails {
	if p.Errors == nil {
		p.Errors = make([]ValidationError, 0)
	}
	p.Errors = append(p.Errors, ValidationError{
		Field:   field,
		Message: message,
		Code:    code,
	})
	return p
}

// Error implements the error interface
func (p *ProblemDetails) Error() string {
	return fmt.Sprintf("[%d] %s: %s", p.Status, p.Title, p.Detail)
}

// Common error constructors

// NewValidationError creates a validation error
func NewValidationError(detail, instance string) *ProblemDetails {
	return NewProblemDetails(TypeValidationError, TitleValidationError, http.StatusBadRequest, detail, instance)
}

// NewUnauthorizedError creates an unauthorized error
func NewUnauthorizedError(detail, instance string) *ProblemDetails {
	return NewProblemDetails(TypeUnauthorized, TitleUnauthorized, http.StatusUnauthorized, detail, instance)
}

// NewForbiddenError creates a forbidden error
func NewForbiddenError(detail, instance string) *ProblemDetails {
	return NewProblemDetails(TypeForbidden, TitleForbidden, http.StatusForbidden, detail, instance)
}

// NewNotFoundError creates a not found error
func NewNotFoundError(detail, instance string) *ProblemDetails {
	return NewProblemDetails(TypeNotFound, TitleNotFound, http.StatusNotFound, detail, instance)
}

// NewRateLimitError creates a rate limit error
func NewRateLimitError(detail, instance string) *ProblemDetails {
	return NewProblemDetails(TypeRateLimit, TitleRateLimit, http.StatusTooManyRequests, detail, instance)
}

// NewInternalError creates an internal server error
func NewInternalError(detail, instance string) *ProblemDetails {
	return NewProblemDetails(TypeInternalError, TitleInternalError, http.StatusInternalServerError, detail, instance)
}

// NewInsufficientFundsError creates an insufficient funds error
func NewInsufficientFundsError(detail, instance string) *ProblemDetails {
	return NewProblemDetails(TypeInsufficientFunds, TitleInsufficientFunds, http.StatusBadRequest, detail, instance)
}

// NewInvalidOrderError creates an invalid order error
func NewInvalidOrderError(detail, instance string) *ProblemDetails {
	return NewProblemDetails(TypeInvalidOrder, TitleInvalidOrder, http.StatusBadRequest, detail, instance)
}

// NewMarketClosedError creates a market closed error
func NewMarketClosedError(detail, instance string) *ProblemDetails {
	return NewProblemDetails(TypeMarketClosed, TitleMarketClosed, http.StatusServiceUnavailable, detail, instance)
}

// NewKYCRequiredError creates a KYC required error
func NewKYCRequiredError(detail, instance string) *ProblemDetails {
	return NewProblemDetails(TypeKYCRequired, TitleKYCRequired, http.StatusForbidden, detail, instance)
}

// NewMFARequiredError creates an MFA required error
func NewMFARequiredError(detail, instance string) *ProblemDetails {
	return NewProblemDetails(TypeMFARequired, TitleMFARequired, http.StatusUnauthorized, detail, instance)
}

// NewAMLBlockedError creates an AML blocked error
func NewAMLBlockedError(detail, instance string) *ProblemDetails {
	return NewProblemDetails(TypeAMLBlocked, TitleAMLBlocked, http.StatusForbidden, detail, instance)
}

// NewOrderNotFoundError creates an order not found error
func NewOrderNotFoundError(detail, instance string) *ProblemDetails {
	return NewProblemDetails(TypeOrderNotFound, TitleOrderNotFound, http.StatusNotFound, detail, instance)
}

// NewInvalidSymbolError creates an invalid symbol error
func NewInvalidSymbolError(detail, instance string) *ProblemDetails {
	return NewProblemDetails(TypeInvalidSymbol, TitleInvalidSymbol, http.StatusBadRequest, detail, instance)
}

// Legacy error conversion helpers

// ToProblemDetails converts legacy Error to RFC 7807 ProblemDetails
func (e *Error) ToProblemDetails(instance string) *ProblemDetails {
	var problemType, title string
	var status int

	switch e.Kind {
	case "ValidationError":
		problemType = TypeValidationError
		title = TitleValidationError
		status = http.StatusBadRequest
	case "Unauthorized":
		problemType = TypeUnauthorized
		title = TitleUnauthorized
		status = http.StatusUnauthorized
	case "Forbidden":
		problemType = TypeForbidden
		title = TitleForbidden
		status = http.StatusForbidden
	case "NotFound":
		problemType = TypeNotFound
		title = TitleNotFound
		status = http.StatusNotFound
	case "RateLimit":
		problemType = TypeRateLimit
		title = TitleRateLimit
		status = http.StatusTooManyRequests
	case "InsufficientFunds":
		problemType = TypeInsufficientFunds
		title = TitleInsufficientFunds
		status = http.StatusBadRequest
	default:
		problemType = TypeInternalError
		title = TitleInternalError
		status = http.StatusInternalServerError
	}

	pd := NewProblemDetails(problemType, title, status, e.Message, instance)
	// Convert legacy field errors to validation errors
	if len(e.Fields) > 0 {
		for _, field := range e.Fields {
			pd.AddValidationError(field.Field, field.Message, field.Kind)
		}
	}

	return pd
}
