package apiutil

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/Aidin1998/finalex/pkg/errors"
	"github.com/go-playground/validator/v10"
	"go.uber.org/zap"
)

// DEPRECATED: Use pkg/validation.UnifiedValidator instead
// This is kept for backward compatibility only
func NewValidator() *Validator {
	validate := validator.New()
	validate.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
		if name == "-" {
			return ""
		}
		return name
	})
	return &Validator{validate}
}

// DEPRECATED: Use pkg/validation.UnifiedValidator instead
type Validator struct {
	validator *validator.Validate
}

// DEPRECATED: Use pkg/validation.UnifiedValidator instead
func (v *Validator) Validate(i interface{}) error {
	if err := v.validator.Struct(i); err != nil {
		validationErr := errors.Invalid.Explain("validation error")
		var fieldsError validator.ValidationErrors
		if errors.As(err, &fieldsError) {
			for _, fieldErr := range fieldsError {
				validationErr = validationErr.WithField(fieldErr.Tag(), fieldErr.Field(), "")
			}
		}
		return validationErr
	}
	return nil
}

// UnifiedAPIValidator provides basic validation without circular imports
type UnifiedAPIValidator struct {
	validator *validator.Validate
	logger    *zap.Logger
}

// NewUnifiedAPIValidator creates a new validator for API utilities
func NewUnifiedAPIValidator(logger *zap.Logger) *UnifiedAPIValidator {
	validate := validator.New()
	validate.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
		if name == "-" {
			return ""
		}
		return name
	})
	return &UnifiedAPIValidator{
		validator: validate,
		logger:    logger,
	}
}

// ValidationResult represents a validation error for API utilities
type ValidationResult struct {
	Field   string `json:"field"`
	Message string `json:"message"`
	Code    string `json:"code"`
}

// ValidateStruct validates a struct using the unified validation system with RFC 7807 error format
func ValidateStruct(logger *zap.Logger, i interface{}) *errors.ProblemDetails {
	apiValidator := NewUnifiedAPIValidator(logger)

	if err := apiValidator.validator.Struct(i); err != nil {
		var validationErrors validator.ValidationErrors
		if errors.As(err, &validationErrors) {
			// Convert to RFC 7807 format
			rfc7807Errors := make([]errors.ValidationError, len(validationErrors))
			for i, fieldErr := range validationErrors {
				rfc7807Errors[i] = errors.ValidationError{
					Field:   fieldErr.Field(),
					Message: getErrorMessage(fieldErr),
					Code:    fieldErr.Tag(),
				}
			}

			return errors.NewValidationError(
				"Struct validation failed",
				"", // No instance path for struct validation
			).WithValidationErrors(rfc7807Errors)
		} else {
			// Generic validation error
			return errors.NewValidationError(
				fmt.Sprintf("Validation failed: %v", err),
				"",
			)
		}
	}

	return nil
}

// getErrorMessage generates a user-friendly error message for validation errors
func getErrorMessage(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return fmt.Sprintf("Field '%s' is required", fe.Field())
	case "email":
		return fmt.Sprintf("Field '%s' must be a valid email address", fe.Field())
	case "min":
		return fmt.Sprintf("Field '%s' must be at least %s characters long", fe.Field(), fe.Param())
	case "max":
		return fmt.Sprintf("Field '%s' must be at most %s characters long", fe.Field(), fe.Param())
	case "len":
		return fmt.Sprintf("Field '%s' must be exactly %s characters long", fe.Field(), fe.Param())
	case "numeric":
		return fmt.Sprintf("Field '%s' must be numeric", fe.Field())
	case "alpha":
		return fmt.Sprintf("Field '%s' must contain only alphabetic characters", fe.Field())
	case "alphanum":
		return fmt.Sprintf("Field '%s' must contain only alphanumeric characters", fe.Field())
	case "url":
		return fmt.Sprintf("Field '%s' must be a valid URL", fe.Field())
	case "uuid":
		return fmt.Sprintf("Field '%s' must be a valid UUID", fe.Field())
	case "oneof":
		return fmt.Sprintf("Field '%s' must be one of: %s", fe.Field(), fe.Param())
	default:
		return fmt.Sprintf("Field '%s' failed validation for '%s'", fe.Field(), fe.Tag())
	}
}
