package errors

import (
	"errors"
	"fmt"
	"runtime"
)

var (
	Is     = errors.Is
	As     = errors.As
	Join   = errors.Join
	Unwrap = errors.Unwrap
)

// Error is a error type for passing more information
// swagger:model
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
