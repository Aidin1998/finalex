// Package errors provides comprehensive error handling using RFC 7807 Problem Details standard
package errors

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"runtime"
)

// Standard error functions
var (
	Is     = errors.Is
	As     = errors.As
	Join   = errors.Join
	Unwrap = errors.Unwrap
)

// FieldError represents a validation error for a specific field
type FieldError struct {
	Kind    string `json:"kind" validate:"required"`
	Field   string `json:"field" validate:"required"`
	Message string `json:"message,omitempty" validate:"required"`
}

func (f *FieldError) Error() string {
	return fmt.Sprintf("%s (%s): %s", f.Field, f.Kind, f.Message)
}

func NewFieldError(kind, field, reason string) FieldError {
	return FieldError{Kind: kind, Field: field, Message: reason}
}

// StatusCode represents an HTTP status code error
type StatusCode int

// Error implements error
func (status StatusCode) Error() string {
	return http.StatusText(int(status))
}

func Status(code int) *Error {
	return Wrap(StatusCode(code)).Reason(http.StatusText(code))
}

var (
	Invalid       *Error = Status(http.StatusBadRequest)
	NotFound      *Error = Status(http.StatusNotFound)
	Conflict      *Error = Status(http.StatusConflict)
	BadGateway    *Error = Status(http.StatusBadGateway)
	Unavailable   *Error = Status(http.StatusServiceUnavailable)
	Unprocessable *Error = Status(http.StatusUnprocessableEntity)
)

// Error is a custom error type for passing more information
type Error struct {
	// Kind is the returned error type
	Kind string `json:"kind"`
	// Message is the human readable string that indicate the error
	Message string `json:"message"`
	// Fields used when there's validation error for a field.
	Fields []FieldError `json:"fields,omitempty"`

	trace []byte
	cause error
}

var _ error = (*Error)(nil)

func New(message string) *Error {
	return &Error{Kind: "Unknown", Message: message}
}

func NewWithKind(kind string) *Error {
	return &Error{Kind: kind}
}

func Wrap(err error) *Error {
	return &Error{cause: err}
}

// Error implements error
func (e *Error) Error() string {
	str := fmt.Sprintf("[%s] ", e.Kind)
	if e.Message != "" {
		str += e.Message
	}
	if e.cause != nil {
		str += fmt.Sprintf(" (%s)", e.cause)
	}
	if len(e.trace) > 0 {
		str = str + fmt.Sprintf("\n\nTrace: %s", string(e.trace))
	}
	return str
}

// Reason returns a copy of the error with kind set to given value
func (e *Error) Reason(kind string) *Error {
	err := *e
	err.Kind = kind
	return &err
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

// Wrap sets the error cause
func (e *Error) Wrap(cause error) *Error {
	e.cause = cause
	return e
}

// Explain makes a copy of the error with given message
func (e *Error) Explain(message string, args ...any) *Error {
	err := *e
	err.Message = fmt.Sprintf(message, args...)
	return &err
}

// Trace sets the error stack trace
func (e *Error) Trace() *Error {
	stack := make([]byte, 2048)
	n := runtime.Stack(stack, false)
	e.trace = stack[:n]
	return e
}

func (e *Error) WithFields(fields []FieldError) *Error {
	newError := *e
	newError.Fields = fields
	return &newError
}

// WithField returns a copy of error with fields replaced.
func (e *Error) WithField(kind, field, message string) *Error {
	newError := *e
	newError.Fields = append(newError.Fields, NewFieldError(kind, field, message))
	return &newError
}

// Is implements the needed interface for errors.Is
// It checks kind and status code for equality
func (e *Error) Is(target error) bool {
	if e == nil {
		return target == nil
	}
	if other, ok := target.(*Error); ok {
		return other.Kind == e.Kind
	}
	if e.cause != nil {
		return Is(e.cause, target)
	}
	return false
}

// Problem type URIs
const (
	TypeValidationError   = "https://api.finalex.io/problems/validation-error"
	TypeUnauthorized      = "https://api.finalex.io/problems/unauthorized"
	TypeForbidden         = "https://api.finalex.io/problems/forbidden"
	TypeNotFound          = "https://api.finalex.io/problems/not-found"
	TypeRateLimit         = "https://api.finalex.io/problems/rate-limit"
	TypeInternalError     = "https://api.finalex.io/problems/internal-error"
	TypeInsufficientFunds = "https://api.finalex.io/problems/insufficient-funds"
	TypeInvalidOrder      = "https://api.finalex.io/problems/invalid-order"
	TypeMarketClosed      = "https://api.finalex.io/problems/market-closed"
	TypeKYCRequired       = "https://api.finalex.io/problems/kyc-required"
	TypeMFARequired       = "https://api.finalex.io/problems/mfa-required"
	TypeAMLBlocked        = "https://api.finalex.io/problems/aml-blocked"
	TypeOrderNotFound     = "https://api.finalex.io/problems/order-not-found"
	TypeInvalidSymbol     = "https://api.finalex.io/problems/invalid-symbol"
)

// Problem titles
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

// ValidationError represents a validation error for RFC 7807
type ValidationError struct {
	Field   string      `json:"field"`
	Value   interface{} `json:"value,omitempty"`
	Message string      `json:"message"`
	Code    string      `json:"code,omitempty"`
}

// ProblemDetails represents an RFC 7807 Problem Details response
type ProblemDetails struct {
	Type     string                 `json:"type"`
	Title    string                 `json:"title"`
	Status   int                    `json:"status"`
	Detail   string                 `json:"detail,omitempty"`
	Instance string                 `json:"instance,omitempty"`
	TraceID  string                 `json:"trace_id,omitempty"`
	Errors   []ValidationError      `json:"errors,omitempty"`
	Extra    map[string]interface{} `json:"-"`
}

// Error implements the error interface
func (p *ProblemDetails) Error() string {
	return p.Detail
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

// WithExtra adds extra fields to the problem details (they will be serialized at the top level)
func (p *ProblemDetails) WithExtra(key string, value interface{}) *ProblemDetails {
	if p.Extra == nil {
		p.Extra = make(map[string]interface{})
	}
	p.Extra[key] = value
	return p
}

// MarshalJSON implements custom JSON marshaling to include extra fields at the top level
func (p *ProblemDetails) MarshalJSON() ([]byte, error) {
	type Alias ProblemDetails

	// Start with the main fields
	result := make(map[string]interface{})
	result["type"] = p.Type
	result["title"] = p.Title
	result["status"] = p.Status
	if p.Detail != "" {
		result["detail"] = p.Detail
	}
	if p.Instance != "" {
		result["instance"] = p.Instance
	}
	if p.TraceID != "" {
		result["trace_id"] = p.TraceID
	}
	if len(p.Errors) > 0 {
		result["errors"] = p.Errors
	}

	// Add extra fields at the top level
	for k, v := range p.Extra {
		result[k] = v
	}

	return json.Marshal(result)
}

// Constructors for common problem types

// NewValidationError creates a validation error problem
func NewValidationError(detail, instance string) *ProblemDetails {
	return &ProblemDetails{
		Type:     TypeValidationError,
		Title:    TitleValidationError,
		Status:   http.StatusBadRequest,
		Detail:   detail,
		Instance: instance,
	}
}

// NewUnauthorizedError creates an unauthorized error problem
func NewUnauthorizedError(detail, instance string) *ProblemDetails {
	return &ProblemDetails{
		Type:     TypeUnauthorized,
		Title:    TitleUnauthorized,
		Status:   http.StatusUnauthorized,
		Detail:   detail,
		Instance: instance,
	}
}

// NewForbiddenError creates a forbidden error problem
func NewForbiddenError(detail, instance string) *ProblemDetails {
	return &ProblemDetails{
		Type:     TypeForbidden,
		Title:    TitleForbidden,
		Status:   http.StatusForbidden,
		Detail:   detail,
		Instance: instance,
	}
}

// NewNotFoundError creates a not found error problem
func NewNotFoundError(detail, instance string) *ProblemDetails {
	return &ProblemDetails{
		Type:     TypeNotFound,
		Title:    TitleNotFound,
		Status:   http.StatusNotFound,
		Detail:   detail,
		Instance: instance,
	}
}

// NewRateLimitError creates a rate limit error problem
func NewRateLimitError(detail, instance string) *ProblemDetails {
	return &ProblemDetails{
		Type:     TypeRateLimit,
		Title:    TitleRateLimit,
		Status:   http.StatusTooManyRequests,
		Detail:   detail,
		Instance: instance,
	}
}

// NewInternalError creates an internal server error problem
func NewInternalError(detail, instance string) *ProblemDetails {
	return &ProblemDetails{
		Type:     TypeInternalError,
		Title:    TitleInternalError,
		Status:   http.StatusInternalServerError,
		Detail:   detail,
		Instance: instance,
	}
}

// Business domain specific errors

// NewInsufficientFundsError creates an insufficient funds error
func NewInsufficientFundsError(detail, instance string) *ProblemDetails {
	return &ProblemDetails{
		Type:     TypeInsufficientFunds,
		Title:    TitleInsufficientFunds,
		Status:   http.StatusUnprocessableEntity,
		Detail:   detail,
		Instance: instance,
	}
}

// NewInvalidOrderError creates an invalid order error
func NewInvalidOrderError(detail, instance string) *ProblemDetails {
	return &ProblemDetails{
		Type:     TypeInvalidOrder,
		Title:    TitleInvalidOrder,
		Status:   http.StatusBadRequest,
		Detail:   detail,
		Instance: instance,
	}
}

// NewMarketClosedError creates a market closed error
func NewMarketClosedError(detail, instance string) *ProblemDetails {
	return &ProblemDetails{
		Type:     TypeMarketClosed,
		Title:    TitleMarketClosed,
		Status:   http.StatusServiceUnavailable,
		Detail:   detail,
		Instance: instance,
	}
}

// NewKYCRequiredError creates a KYC required error
func NewKYCRequiredError(detail, instance string) *ProblemDetails {
	return &ProblemDetails{
		Type:     TypeKYCRequired,
		Title:    TitleKYCRequired,
		Status:   http.StatusForbidden,
		Detail:   detail,
		Instance: instance,
	}
}

// NewMFARequiredError creates an MFA required error
func NewMFARequiredError(detail, instance string) *ProblemDetails {
	return &ProblemDetails{
		Type:     TypeMFARequired,
		Title:    TitleMFARequired,
		Status:   http.StatusUnauthorized,
		Detail:   detail,
		Instance: instance,
	}
}

// NewAMLBlockedError creates an AML blocked error
func NewAMLBlockedError(detail, instance string) *ProblemDetails {
	return &ProblemDetails{
		Type:     TypeAMLBlocked,
		Title:    TitleAMLBlocked,
		Status:   http.StatusForbidden,
		Detail:   detail,
		Instance: instance,
	}
}

// NewOrderNotFoundError creates an order not found error
func NewOrderNotFoundError(detail, instance string) *ProblemDetails {
	return &ProblemDetails{
		Type:     TypeOrderNotFound,
		Title:    TitleOrderNotFound,
		Status:   http.StatusNotFound,
		Detail:   detail,
		Instance: instance,
	}
}

// NewInvalidSymbolError creates an invalid symbol error
func NewInvalidSymbolError(detail, instance string) *ProblemDetails {
	return &ProblemDetails{
		Type:     TypeInvalidSymbol,
		Title:    TitleInvalidSymbol,
		Status:   http.StatusBadRequest,
		Detail:   detail,
		Instance: instance,
	}
}

// NewConflictError creates a conflict error
func NewConflictError(detail, instance string) *ProblemDetails {
	return &ProblemDetails{
		Type:     "https://api.finalex.io/problems/conflict",
		Title:    "Conflict",
		Status:   http.StatusConflict,
		Detail:   detail,
		Instance: instance,
	}
}

// NewBadGatewayError creates a bad gateway error
func NewBadGatewayError(detail, instance string) *ProblemDetails {
	return &ProblemDetails{
		Type:     "https://api.finalex.io/problems/bad-gateway",
		Title:    "Bad Gateway",
		Status:   http.StatusBadGateway,
		Detail:   detail,
		Instance: instance,
	}
}

// NewServiceUnavailableError creates a service unavailable error
func NewServiceUnavailableError(detail, instance string) *ProblemDetails {
	return &ProblemDetails{
		Type:     "https://api.finalex.io/problems/service-unavailable",
		Title:    "Service Unavailable",
		Status:   http.StatusServiceUnavailable,
		Detail:   detail,
		Instance: instance,
	}
}

// NewProblemDetails creates a generic problem details with all fields
func NewProblemDetails(problemType, title string, status int, detail, instance string) *ProblemDetails {
	return &ProblemDetails{
		Type:     problemType,
		Title:    title,
		Status:   status,
		Detail:   detail,
		Instance: instance,
	}
}

// UnifiedErrorHandler provides consistent error handling across the application
type UnifiedErrorHandler struct {
	useRFC7807Format bool
}

// NewUnifiedErrorHandler creates a new unified error handler
func NewUnifiedErrorHandler() *UnifiedErrorHandler {
	return &UnifiedErrorHandler{
		useRFC7807Format: true,
	}
}

// HandleValidationError converts validation errors to appropriate response format
func (h *UnifiedErrorHandler) HandleValidationError(err error) *ProblemDetails {
	return NewValidationError(err.Error(), "/")
}

// HandleBusinessError converts business logic errors to appropriate response format
func (h *UnifiedErrorHandler) HandleBusinessError(message, instance string) *ProblemDetails {
	return NewConflictError(message, instance)
}

// HandleInternalError converts internal errors to appropriate response format
func (h *UnifiedErrorHandler) HandleInternalError(detail, instance string) *ProblemDetails {
	return NewInternalError(detail, instance)
}
