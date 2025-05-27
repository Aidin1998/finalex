package errors

import "fmt"

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
