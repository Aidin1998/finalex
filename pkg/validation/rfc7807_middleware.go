package validation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/Aidin1998/finalex/common/apiutil"
	"github.com/Aidin1998/finalex/common/errors"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Constants for validation limits
const (
	MaxBodySize     = 1024 * 1024 // 1MB
	MaxStringLength = 1000        // Maximum string length
	MaxNumericValue = 1000000000  // Maximum numeric value
)

// Validation patterns
var (
	sqlInjectionPattern = regexp.MustCompile(`(?i)(union|select|insert|update|delete|drop|create|alter|exec|script)`)
	xssPattern          = regexp.MustCompile(`(?i)(<script|javascript:|onload=|onerror=|onclick=)`)
)

// RFC7807ValidationMiddleware creates a middleware for comprehensive input validation using RFC 7807 error format
func RFC7807ValidationMiddleware(logger *zap.Logger) gin.HandlerFunc {
	validator := NewValidator(logger)

	return func(c *gin.Context) {
		// Validate query parameters
		if validationErrs := validateQueryParamsRFC7807(validator, c); len(validationErrs) > 0 {
			problemDetails := errors.NewValidationError(
				"Invalid query parameters",
				c.Request.URL.Path,
			).WithValidationErrors(validationErrs)
			apiutil.RFC7807ErrorResponse(c, problemDetails)
			c.Abort()
			return
		}

		// Validate path parameters
		if validationErrs := validatePathParamsRFC7807(validator, c); len(validationErrs) > 0 {
			problemDetails := errors.NewValidationError(
				"Invalid path parameters",
				c.Request.URL.Path,
			).WithValidationErrors(validationErrs)
			apiutil.RFC7807ErrorResponse(c, problemDetails)
			c.Abort()
			return
		}

		// Validate headers
		if validationErrs := validateHeadersRFC7807(validator, c); len(validationErrs) > 0 {
			problemDetails := errors.NewValidationError(
				"Invalid headers",
				c.Request.URL.Path,
			).WithValidationErrors(validationErrs)
			apiutil.RFC7807ErrorResponse(c, problemDetails)
			c.Abort()
			return
		}

		// Validate request body if present
		if c.Request.ContentLength > 0 {
			if validationErrs := validateRequestBodyRFC7807(validator, c); len(validationErrs) > 0 {
				problemDetails := errors.NewValidationError(
					"Invalid request body",
					c.Request.URL.Path,
				).WithValidationErrors(validationErrs)
				apiutil.RFC7807ErrorResponse(c, problemDetails)
				c.Abort()
				return
			}
		}

		c.Next()
	}
}

// validateQueryParamsRFC7807 validates query parameters and returns RFC 7807 validation errors
func validateQueryParamsRFC7807(validator *Validator, c *gin.Context) []errors.ValidationError {
	var validationErrors []errors.ValidationError

	for param, values := range c.Request.URL.Query() {
		for _, value := range values {
			if err := validateStringValueRFC7807(validator, param, value); err != nil {
				validationErrors = append(validationErrors, errors.ValidationError{
					Field:   param,
					Message: err.Error(),
					Code:    "INVALID_QUERY_PARAM",
				})
			}
		}
	}

	return validationErrors
}

// validatePathParamsRFC7807 validates path parameters and returns RFC 7807 validation errors
func validatePathParamsRFC7807(validator *Validator, c *gin.Context) []errors.ValidationError {
	var validationErrors []errors.ValidationError

	// Get all path parameters
	for _, param := range c.Params {
		if err := validateStringValueRFC7807(validator, param.Key, param.Value); err != nil {
			validationErrors = append(validationErrors, errors.ValidationError{
				Field:   param.Key,
				Message: err.Error(),
				Code:    "INVALID_PATH_PARAM",
			})
		}
	}

	return validationErrors
}

// validateHeadersRFC7807 validates request headers and returns RFC 7807 validation errors
func validateHeadersRFC7807(validator *Validator, c *gin.Context) []errors.ValidationError {
	var validationErrors []errors.ValidationError

	// Validate critical headers
	criticalHeaders := []string{"Authorization", "Content-Type", "User-Agent", "X-API-Key"}

	for _, header := range criticalHeaders {
		if value := c.GetHeader(header); value != "" {
			if err := validateHeaderValue(header, value); err != nil {
				validationErrors = append(validationErrors, *err)
			}
		}
	}

	return validationErrors
}

// validateRequestBodyRFC7807 validates request body and returns RFC 7807 validation errors
func validateRequestBodyRFC7807(validator *Validator, c *gin.Context) []errors.ValidationError {
	var validationErrors []errors.ValidationError

	// Read and validate body size
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		validationErrors = append(validationErrors, errors.ValidationError{
			Field:   "body",
			Message: "Failed to read request body",
			Code:    "BODY_READ_ERROR",
		})
		return validationErrors
	}

	// Restore body for further processing
	c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	if len(bodyBytes) > MaxBodySize {
		validationErrors = append(validationErrors, errors.ValidationError{
			Field:   "body",
			Message: fmt.Sprintf("Request body too large (max %d bytes)", MaxBodySize),
			Code:    "BODY_TOO_LARGE",
		})
		return validationErrors
	}

	// Validate content type
	contentType := c.GetHeader("Content-Type")
	if !isAllowedContentType(contentType) {
		validationErrors = append(validationErrors, errors.ValidationError{
			Field:   "Content-Type",
			Message: "Unsupported content type",
			Code:    "INVALID_CONTENT_TYPE",
		})
	}

	// Parse and validate JSON if applicable
	if strings.Contains(contentType, "application/json") && len(bodyBytes) > 0 {
		if jsonErrs := validateJSONContent(bodyBytes); len(jsonErrs) > 0 {
			validationErrors = append(validationErrors, jsonErrs...)
		}
	}

	return validationErrors
}

// validateStringValueRFC7807 validates a string value for security issues
func validateStringValueRFC7807(validator *Validator, field, value string) error {
	if len(value) > MaxStringLength {
		return fmt.Errorf("string too long (max %d characters)", MaxStringLength)
	}

	// Check for SQL injection patterns
	if sqlInjectionPattern.MatchString(value) {
		return fmt.Errorf("potentially malicious SQL pattern detected")
	}

	// Check for XSS patterns
	if xssPattern.MatchString(value) {
		return fmt.Errorf("potentially malicious script content detected")
	}

	return nil
}

// validateHeaderValue validates header values
func validateHeaderValue(header, value string) *errors.ValidationError {
	// Basic validation for headers
	if len(value) > MaxStringLength {
		return &errors.ValidationError{
			Field:   header,
			Message: fmt.Sprintf("Header value too long (max %d characters)", MaxStringLength),
			Code:    "HEADER_TOO_LONG",
		}
	}

	// Check for malicious patterns in headers
	if sqlInjectionPattern.MatchString(value) || xssPattern.MatchString(value) {
		return &errors.ValidationError{
			Field:   header,
			Message: "Potentially malicious header content detected",
			Code:    "MALICIOUS_HEADER",
		}
	}

	return nil
}

// validateJSONContent validates JSON content structure and values
func validateJSONContent(bodyBytes []byte) []errors.ValidationError {
	var validationErrors []errors.ValidationError

	// Parse JSON
	var jsonData interface{}
	if err := json.Unmarshal(bodyBytes, &jsonData); err != nil {
		validationErrors = append(validationErrors, errors.ValidationError{
			Field:   "body",
			Message: "Invalid JSON format: " + err.Error(),
			Code:    "INVALID_JSON",
		})
		return validationErrors
	}

	// Validate JSON values recursively
	if jsonErrs := validateJSONValue("", jsonData); len(jsonErrs) > 0 {
		validationErrors = append(validationErrors, jsonErrs...)
	}

	return validationErrors
}

// validateJSONValue recursively validates JSON values
func validateJSONValue(fieldPath string, value interface{}) []errors.ValidationError {
	var validationErrors []errors.ValidationError

	switch v := value.(type) {
	case string:
		if err := validateStringValueRFC7807(nil, fieldPath, v); err != nil {
			validationErrors = append(validationErrors, errors.ValidationError{
				Field:   fieldPath,
				Message: err.Error(),
				Code:    "INVALID_STRING_VALUE",
			})
		}
	case float64:
		if v > MaxNumericValue || v < -MaxNumericValue {
			validationErrors = append(validationErrors, errors.ValidationError{
				Field:   fieldPath,
				Message: fmt.Sprintf("Numeric value out of range (±%d)", MaxNumericValue),
				Code:    "NUMERIC_OUT_OF_RANGE",
			})
		}
	case map[string]interface{}:
		for key, val := range v {
			newPath := key
			if fieldPath != "" {
				newPath = fieldPath + "." + key
			}
			if jsonErrs := validateJSONValue(newPath, val); len(jsonErrs) > 0 {
				validationErrors = append(validationErrors, jsonErrs...)
			}
		}
	case []interface{}:
		for i, val := range v {
			newPath := fmt.Sprintf("[%d]", i)
			if fieldPath != "" {
				newPath = fieldPath + newPath
			}
			if jsonErrs := validateJSONValue(newPath, val); len(jsonErrs) > 0 {
				validationErrors = append(validationErrors, jsonErrs...)
			}
		}
	}

	return validationErrors
}

// isAllowedContentType checks if the content type is allowed
func isAllowedContentType(contentType string) bool {
	allowedTypes := []string{
		"application/json",
		"application/x-www-form-urlencoded",
		"multipart/form-data",
		"text/plain",
	}

	for _, allowed := range allowedTypes {
		if strings.Contains(contentType, allowed) {
			return true
		}
	}

	return false
}
