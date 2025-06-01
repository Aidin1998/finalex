package validation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// EnhancedValidationConfig holds configuration for enhanced validation
type EnhancedValidationConfig struct {
	MaxRequestBodySize        int64         `json:"max_request_body_size"`        // Default: 10MB
	MaxQueryParamCount        int           `json:"max_query_param_count"`        // Default: 50
	MaxHeaderCount            int           `json:"max_header_count"`             // Default: 100
	MaxFieldDepth             int           `json:"max_field_depth"`              // Default: 10
	RequestTimeout            time.Duration `json:"request_timeout"`              // Default: 30s
	EnablePerformanceLogging  bool          `json:"enable_performance_logging"`   // Default: false
	EnableDetailedErrorLogs   bool          `json:"enable_detailed_error_logs"`   // Default: true
	StrictModeEnabled         bool          `json:"strict_mode_enabled"`          // Default: true
	RateLimitValidationCount  int           `json:"rate_limit_validation_count"`  // Default: 1000
	RateLimitValidationWindow time.Duration `json:"rate_limit_validation_window"` // Default: 1m
}

// DefaultEnhancedValidationConfig returns default configuration
func DefaultEnhancedValidationConfig() *EnhancedValidationConfig {
	return &EnhancedValidationConfig{
		MaxRequestBodySize:        10 * 1024 * 1024, // 10MB
		MaxQueryParamCount:        50,
		MaxHeaderCount:            100,
		MaxFieldDepth:             10,
		RequestTimeout:            30 * time.Second,
		EnablePerformanceLogging:  false,
		EnableDetailedErrorLogs:   true,
		StrictModeEnabled:         true,
		RateLimitValidationCount:  1000,
		RateLimitValidationWindow: time.Minute,
	}
}

// EndpointValidationRule defines validation rules for specific endpoints
type EndpointValidationRule struct {
	Method           string               `json:"method"`
	Path             string               `json:"path"`
	PathPattern      *regexp.Regexp       `json:"-"`
	RequiredParams   []string             `json:"required_params"`
	RequiredHeaders  []string             `json:"required_headers"`
	AllowedParams    map[string]ParamRule `json:"allowed_params"`
	BodyValidation   *BodyValidationRule  `json:"body_validation"`
	CustomValidators []string             `json:"custom_validators"`
	PerformanceLimit time.Duration        `json:"performance_limit"`
}

// ParamRule defines validation rules for individual parameters
type ParamRule struct {
	Type          string         `json:"type"` // string, int, float, bool, uuid, decimal
	Required      bool           `json:"required"`
	MinLength     *int           `json:"min_length"`
	MaxLength     *int           `json:"max_length"`
	MinValue      *float64       `json:"min_value"`
	MaxValue      *float64       `json:"max_value"`
	Pattern       string         `json:"pattern"`
	PatternRegex  *regexp.Regexp `json:"-"`
	AllowedValues []string       `json:"allowed_values"`
	Description   string         `json:"description"`
}

// BodyValidationRule defines validation rules for request body
type BodyValidationRule struct {
	ContentType     string               `json:"content_type"` // application/json, application/x-www-form-urlencoded
	MaxSize         int64                `json:"max_size"`
	RequiredFields  []string             `json:"required_fields"`
	AllowedFields   map[string]ParamRule `json:"allowed_fields"`
	StructValidator string               `json:"struct_validator"` // name of Go struct for validation
}

// ValidationResult holds validation result information
type ValidationResult struct {
	Valid          bool                   `json:"valid"`
	Errors         []ValidationError      `json:"errors"`
	Warnings       []string               `json:"warnings"`
	ProcessingTime time.Duration          `json:"processing_time"`
	Metadata       map[string]interface{} `json:"metadata"`
}

// EnhancedValidator provides comprehensive API Gateway input validation
type EnhancedValidator struct {
	config         *EnhancedValidationConfig
	baseValidator  *Validator
	endpointRules  map[string]*EndpointValidationRule
	logger         *zap.Logger
	metricsEnabled bool
}

// NewEnhancedValidator creates a new enhanced validator instance
func NewEnhancedValidator(logger *zap.Logger, config *EnhancedValidationConfig) *EnhancedValidator {
	if config == nil {
		config = DefaultEnhancedValidationConfig()
	}

	ev := &EnhancedValidator{
		config:         config,
		baseValidator:  NewValidator(logger),
		endpointRules:  make(map[string]*EndpointValidationRule),
		logger:         logger,
		metricsEnabled: true,
	}

	// Initialize default endpoint rules
	ev.initializeDefaultRules()

	return ev
}

// EnhancedValidationMiddleware creates middleware with comprehensive input validation
func EnhancedValidationMiddleware(logger *zap.Logger, config *EnhancedValidationConfig) gin.HandlerFunc {
	validator := NewEnhancedValidator(logger, config)

	return func(c *gin.Context) {
		startTime := time.Now()

		// Apply request timeout if configured
		if validator.config.RequestTimeout > 0 {
			ctx, cancel := context.WithTimeout(c.Request.Context(), validator.config.RequestTimeout)
			defer cancel()
			c.Request = c.Request.WithContext(ctx)
		}

		// Validate the request
		result := validator.ValidateRequest(c)

		if !result.Valid {
			// Log validation failure
			if validator.config.EnableDetailedErrorLogs {
				validator.logger.Warn("Request validation failed",
					zap.String("method", c.Request.Method),
					zap.String("path", c.Request.URL.Path),
					zap.String("remote_addr", c.ClientIP()),
					zap.Any("errors", result.Errors),
					zap.Duration("processing_time", result.ProcessingTime))
			}

			// Return detailed error response
			c.JSON(http.StatusBadRequest, gin.H{
				"error":             "Request validation failed",
				"validation_errors": result.Errors,
				"request_id":        c.GetString("request_id"),
				"timestamp":         time.Now().UTC().Format(time.RFC3339),
			})
			c.Abort()
			return
		}

		// Log performance metrics if enabled
		if validator.config.EnablePerformanceLogging {
			processingTime := time.Since(startTime)
			validator.logger.Debug("Request validation completed",
				zap.String("method", c.Request.Method),
				zap.String("path", c.Request.URL.Path),
				zap.Duration("validation_time", processingTime),
				zap.Int("warnings_count", len(result.Warnings)))
		}

		// Add warnings to response headers if any
		if len(result.Warnings) > 0 {
			c.Header("X-Validation-Warnings", strings.Join(result.Warnings, "; "))
		}

		c.Next()
	}
}

// ValidateRequest performs comprehensive request validation
func (ev *EnhancedValidator) ValidateRequest(c *gin.Context) *ValidationResult {
	startTime := time.Now()
	result := &ValidationResult{
		Valid:    true,
		Errors:   make([]ValidationError, 0),
		Warnings: make([]string, 0),
		Metadata: make(map[string]interface{}),
	}

	// 1. Find applicable endpoint rule
	rule := ev.findEndpointRule(c.Request.Method, c.Request.URL.Path)
	if rule != nil {
		result.Metadata["endpoint_rule"] = rule.Path
	}

	// 2. Validate basic request structure
	if err := ev.validateBasicRequest(c); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "request",
			Tag:     "structure",
			Message: err.Error(),
		})
		result.ProcessingTime = time.Since(startTime)
		return result
	}

	// 3. Validate query parameters
	if errors := ev.validateQueryParams(c, rule); len(errors) > 0 {
		result.Valid = false
		result.Errors = append(result.Errors, errors...)
	}

	// 4. Validate path parameters
	if errors := ev.validatePathParams(c, rule); len(errors) > 0 {
		result.Valid = false
		result.Errors = append(result.Errors, errors...)
	}

	// 5. Validate headers
	if errors := ev.validateHeaders(c, rule); len(errors) > 0 {
		result.Valid = false
		result.Errors = append(result.Errors, errors...)
	}

	// 6. Validate request body
	if c.Request.Method == "POST" || c.Request.Method == "PUT" || c.Request.Method == "PATCH" {
		if errors := ev.validateRequestBody(c, rule); len(errors) > 0 {
			result.Valid = false
			result.Errors = append(result.Errors, errors...)
		}
	}

	// 7. Apply endpoint-specific validation
	if rule != nil {
		if errors := ev.applyEndpointSpecificValidation(c, rule); len(errors) > 0 {
			result.Valid = false
			result.Errors = append(result.Errors, errors...)
		}
	}

	result.ProcessingTime = time.Since(startTime)
	return result
}

// initializeDefaultRules sets up default validation rules for trading endpoints
func (ev *EnhancedValidator) initializeDefaultRules() {
	// Trading Order Placement
	ev.addEndpointRule(&EndpointValidationRule{
		Method:          "POST",
		Path:            "/api/v1/trading/orders",
		PathPattern:     regexp.MustCompile(`^/api/v1/trading/orders/?$`),
		RequiredHeaders: []string{"Content-Type", "Authorization"},
		BodyValidation: &BodyValidationRule{
			ContentType:    "application/json",
			MaxSize:        1024, // 1KB
			RequiredFields: []string{"symbol", "side", "type", "quantity"},
			AllowedFields: map[string]ParamRule{
				"symbol": {
					Type:      "string",
					Required:  true,
					MinLength: &[]int{6}[0],  // BTC/USD
					MaxLength: &[]int{12}[0], // SOMETHING/SOMETHING
					Pattern:   `^[A-Z]{2,6}[/-][A-Z]{2,6}$`,
				},
				"side": {
					Type:          "string",
					Required:      true,
					AllowedValues: []string{"buy", "sell", "BUY", "SELL"},
				},
				"type": {
					Type:          "string",
					Required:      true,
					AllowedValues: []string{"market", "limit", "stop", "stop_limit", "MARKET", "LIMIT", "STOP", "STOP_LIMIT"},
				},
				"quantity": {
					Type:     "decimal",
					Required: true,
					MinValue: &[]float64{0.000001}[0], // Minimum order size
					MaxValue: &[]float64{1000000}[0],  // Maximum order size
				},
				"price": {
					Type:     "decimal",
					Required: false,
					MinValue: &[]float64{0.01}[0],     // Minimum price
					MaxValue: &[]float64{10000000}[0], // Maximum price
				},
				"time_in_force": {
					Type:          "string",
					Required:      false,
					AllowedValues: []string{"GTC", "IOC", "FOK", "GTD"},
				},
				"stop_price": {
					Type:     "decimal",
					Required: false,
					MinValue: &[]float64{0.01}[0],
					MaxValue: &[]float64{10000000}[0],
				},
			},
		},
		PerformanceLimit: 100 * time.Millisecond,
	})

	// Trading Order Retrieval
	ev.addEndpointRule(&EndpointValidationRule{
		Method:          "GET",
		Path:            "/api/v1/trading/orders",
		PathPattern:     regexp.MustCompile(`^/api/v1/trading/orders/?$`),
		RequiredHeaders: []string{"Authorization"},
		AllowedParams: map[string]ParamRule{
			"symbol": {
				Type:     "string",
				Required: false,
				Pattern:  `^[A-Z]{2,6}[/-][A-Z]{2,6}$`,
			},
			"status": {
				Type:          "string",
				Required:      false,
				AllowedValues: []string{"open", "closed", "canceled", "filled", "partially_filled"},
			},
			"limit": {
				Type:     "int",
				Required: false,
				MinValue: &[]float64{1}[0],
				MaxValue: &[]float64{1000}[0],
			},
			"offset": {
				Type:     "int",
				Required: false,
				MinValue: &[]float64{0}[0],
				MaxValue: &[]float64{100000}[0],
			},
		},
		PerformanceLimit: 50 * time.Millisecond,
	})

	// Order Cancellation
	ev.addEndpointRule(&EndpointValidationRule{
		Method:           "DELETE",
		Path:             "/api/v1/trading/orders/:id",
		PathPattern:      regexp.MustCompile(`^/api/v1/trading/orders/[a-fA-F0-9-]{36}/?$`),
		RequiredHeaders:  []string{"Authorization"},
		PerformanceLimit: 50 * time.Millisecond,
	})

	// Trading Pairs
	ev.addEndpointRule(&EndpointValidationRule{
		Method:      "GET",
		Path:        "/api/v1/trading/pairs",
		PathPattern: regexp.MustCompile(`^/api/v1/trading/pairs/?$`),
		AllowedParams: map[string]ParamRule{
			"status": {
				Type:          "string",
				Required:      false,
				AllowedValues: []string{"active", "inactive", "maintenance"},
			},
		},
		PerformanceLimit: 100 * time.Millisecond,
	})

	// Order Book
	ev.addEndpointRule(&EndpointValidationRule{
		Method:      "GET",
		Path:        "/api/v1/trading/orderbook/:symbol",
		PathPattern: regexp.MustCompile(`^/api/v1/trading/orderbook/[A-Z]{2,6}[/-][A-Z]{2,6}/?$`),
		AllowedParams: map[string]ParamRule{
			"depth": {
				Type:     "int",
				Required: false,
				MinValue: &[]float64{1}[0],
				MaxValue: &[]float64{1000}[0],
			},
		},
		PerformanceLimit: 50 * time.Millisecond,
	})

	// Market Prices
	ev.addEndpointRule(&EndpointValidationRule{
		Method:      "GET",
		Path:        "/api/v1/market/prices",
		PathPattern: regexp.MustCompile(`^/api/v1/market/prices/?$`),
		AllowedParams: map[string]ParamRule{
			"symbols": {
				Type:      "string",
				Required:  false,
				Pattern:   `^([A-Z]{2,6}[/-][A-Z]{2,6})(,([A-Z]{2,6}[/-][A-Z]{2,6}))*$`,
				MaxLength: &[]int{1000}[0],
			},
		},
		PerformanceLimit: 100 * time.Millisecond,
	})

	// Fiat Operations
	ev.addEndpointRule(&EndpointValidationRule{
		Method:          "POST",
		Path:            "/api/v1/fiat/deposit",
		PathPattern:     regexp.MustCompile(`^/api/v1/fiat/deposit/?$`),
		RequiredHeaders: []string{"Content-Type", "Authorization"},
		BodyValidation: &BodyValidationRule{
			ContentType:    "application/json",
			MaxSize:        2048, // 2KB
			RequiredFields: []string{"amount", "currency", "payment_method"},
			AllowedFields: map[string]ParamRule{
				"amount": {
					Type:     "decimal",
					Required: true,
					MinValue: &[]float64{1.0}[0],
					MaxValue: &[]float64{1000000.0}[0],
				},
				"currency": {
					Type:          "string",
					Required:      true,
					AllowedValues: []string{"USD", "EUR", "GBP", "JPY", "CAD", "AUD"},
				},
				"payment_method": {
					Type:          "string",
					Required:      true,
					AllowedValues: []string{"bank_transfer", "credit_card", "debit_card", "wire_transfer"},
				},
				"bank_account_id": {
					Type:     "uuid",
					Required: false,
				},
			},
		},
		PerformanceLimit: 200 * time.Millisecond,
	})

	// Admin Trading Pair Creation
	ev.addEndpointRule(&EndpointValidationRule{
		Method:          "POST",
		Path:            "/api/v1/admin/trading/pairs",
		PathPattern:     regexp.MustCompile(`^/api/v1/admin/trading/pairs/?$`),
		RequiredHeaders: []string{"Content-Type", "Authorization"},
		BodyValidation: &BodyValidationRule{
			ContentType:    "application/json",
			MaxSize:        4096, // 4KB
			RequiredFields: []string{"symbol", "base_asset", "quote_asset", "min_quantity", "max_quantity"},
			AllowedFields: map[string]ParamRule{
				"symbol": {
					Type:      "string",
					Required:  true,
					Pattern:   `^[A-Z]{2,6}/[A-Z]{2,6}$`,
					MinLength: &[]int{6}[0],
					MaxLength: &[]int{12}[0],
				},
				"base_asset": {
					Type:      "string",
					Required:  true,
					Pattern:   `^[A-Z]{2,6}$`,
					MinLength: &[]int{2}[0],
					MaxLength: &[]int{6}[0],
				},
				"quote_asset": {
					Type:      "string",
					Required:  true,
					Pattern:   `^[A-Z]{2,6}$`,
					MinLength: &[]int{2}[0],
					MaxLength: &[]int{6}[0],
				},
				"min_quantity": {
					Type:     "decimal",
					Required: true,
					MinValue: &[]float64{0.000001}[0],
					MaxValue: &[]float64{1000.0}[0],
				},
				"max_quantity": {
					Type:     "decimal",
					Required: true,
					MinValue: &[]float64{1.0}[0],
					MaxValue: &[]float64{1000000.0}[0],
				},
				"price_precision": {
					Type:     "int",
					Required: false,
					MinValue: &[]float64{0}[0],
					MaxValue: &[]float64{8}[0],
				},
				"quantity_precision": {
					Type:     "int",
					Required: false,
					MinValue: &[]float64{0}[0],
					MaxValue: &[]float64{18}[0],
				},
				"status": {
					Type:          "string",
					Required:      false,
					AllowedValues: []string{"active", "inactive", "maintenance"},
				},
			},
		},
		PerformanceLimit: 500 * time.Millisecond,
	})
}

// addEndpointRule adds or updates an endpoint validation rule
func (ev *EnhancedValidator) addEndpointRule(rule *EndpointValidationRule) {
	// Compile regex patterns
	if rule.PathPattern == nil && rule.Path != "" {
		// Convert path with parameters to regex
		pattern := strings.ReplaceAll(rule.Path, ":id", `[a-fA-F0-9-]{36}`)
		pattern = strings.ReplaceAll(pattern, ":symbol", `[A-Z]{2,6}[/-][A-Z]{2,6}`)
		pattern = "^" + pattern + "/?$"
		rule.PathPattern = regexp.MustCompile(pattern)
	}

	// Compile parameter patterns
	for field, paramRule := range rule.AllowedParams {
		if paramRule.Pattern != "" && paramRule.PatternRegex == nil {
			paramRule.PatternRegex = regexp.MustCompile(paramRule.Pattern)
			rule.AllowedParams[field] = paramRule
		}
	}

	// Compile body field patterns
	if rule.BodyValidation != nil {
		for field, paramRule := range rule.BodyValidation.AllowedFields {
			if paramRule.Pattern != "" && paramRule.PatternRegex == nil {
				paramRule.PatternRegex = regexp.MustCompile(paramRule.Pattern)
				rule.BodyValidation.AllowedFields[field] = paramRule
			}
		}
	}

	key := fmt.Sprintf("%s:%s", rule.Method, rule.Path)
	ev.endpointRules[key] = rule
}

// findEndpointRule finds the matching endpoint rule for a request
func (ev *EnhancedValidator) findEndpointRule(method, path string) *EndpointValidationRule {
	// Try exact match first
	key := fmt.Sprintf("%s:%s", method, path)
	if rule, exists := ev.endpointRules[key]; exists {
		return rule
	}

	// Try pattern matching
	for _, rule := range ev.endpointRules {
		if rule.Method == method && rule.PathPattern != nil && rule.PathPattern.MatchString(path) {
			return rule
		}
	}

	return nil
}

// validateBasicRequest validates basic request structure and limits
func (ev *EnhancedValidator) validateBasicRequest(c *gin.Context) error {
	req := c.Request

	// Check query parameter count
	if len(req.URL.Query()) > ev.config.MaxQueryParamCount {
		return fmt.Errorf("too many query parameters: %d (max: %d)",
			len(req.URL.Query()), ev.config.MaxQueryParamCount)
	}

	// Check header count
	if len(req.Header) > ev.config.MaxHeaderCount {
		return fmt.Errorf("too many headers: %d (max: %d)",
			len(req.Header), ev.config.MaxHeaderCount)
	}

	// Check content length
	if req.ContentLength > ev.config.MaxRequestBodySize {
		return fmt.Errorf("request body too large: %d bytes (max: %d bytes)",
			req.ContentLength, ev.config.MaxRequestBodySize)
	}

	return nil
}

// validateQueryParams validates query parameters against endpoint rules
func (ev *EnhancedValidator) validateQueryParams(c *gin.Context, rule *EndpointValidationRule) []ValidationError {
	var errors []ValidationError
	queryParams := c.Request.URL.Query()

	// If no rule specified, use basic validation only
	if rule == nil {
		// Apply basic security validation to all query params
		for key, values := range queryParams {
			for _, value := range values {
				if err := ev.baseValidator.ValidateInput(value); err != nil {
					errors = append(errors, ValidationError{
						Field:   fmt.Sprintf("query.%s", key),
						Tag:     "security",
						Message: fmt.Sprintf("Query parameter contains potentially malicious content: %s", err.Error()),
					})
				}
			}
		}
		return errors
	}

	// Validate each query parameter
	for key, values := range queryParams {
		paramRule, allowed := rule.AllowedParams[key]

		// Check if parameter is allowed
		if !allowed {
			if ev.config.StrictModeEnabled {
				errors = append(errors, ValidationError{
					Field:   fmt.Sprintf("query.%s", key),
					Tag:     "forbidden",
					Message: fmt.Sprintf("Query parameter '%s' is not allowed for this endpoint", key),
				})
				continue
			}
		}

		// Validate each value
		for _, value := range values {
			if validateErrs := ev.validateParamValue(key, value, paramRule, "query"); len(validateErrs) > 0 {
				errors = append(errors, validateErrs...)
			}
		}
	}

	// Check for required parameters
	for key, paramRule := range rule.AllowedParams {
		if paramRule.Required && len(queryParams[key]) == 0 {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("query.%s", key),
				Tag:     "required",
				Message: fmt.Sprintf("Required query parameter '%s' is missing", key),
			})
		}
	}

	return errors
}

// validatePathParams validates path parameters against endpoint rules
func (ev *EnhancedValidator) validatePathParams(c *gin.Context, rule *EndpointValidationRule) []ValidationError {
	var errors []ValidationError

	// Extract path parameters using Gin's Param method
	for _, param := range c.Params {
		// Apply security validation
		if err := ev.baseValidator.ValidateInput(param.Value); err != nil {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("path.%s", param.Key),
				Tag:     "security",
				Message: fmt.Sprintf("Path parameter contains potentially malicious content: %s", err.Error()),
			})
		}

		// Validate specific path parameter types
		switch param.Key {
		case "id":
			if !ev.isValidUUID(param.Value) {
				errors = append(errors, ValidationError{
					Field:   fmt.Sprintf("path.%s", param.Key),
					Tag:     "format",
					Message: "Path parameter 'id' must be a valid UUID",
				})
			}
		case "symbol":
			if !ev.isValidTradingSymbol(param.Value) {
				errors = append(errors, ValidationError{
					Field:   fmt.Sprintf("path.%s", param.Key),
					Tag:     "format",
					Message: "Path parameter 'symbol' must be a valid trading symbol (e.g., BTC/USD)",
				})
			}
		}
	}

	return errors
}

// validateHeaders validates required headers
func (ev *EnhancedValidator) validateHeaders(c *gin.Context, rule *EndpointValidationRule) []ValidationError {
	var errors []ValidationError

	if rule == nil || len(rule.RequiredHeaders) == 0 {
		return errors
	}

	for _, headerName := range rule.RequiredHeaders {
		headerValue := c.GetHeader(headerName)
		if headerValue == "" {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("header.%s", headerName),
				Tag:     "required",
				Message: fmt.Sprintf("Required header '%s' is missing", headerName),
			})
			continue
		}

		// Apply security validation to header values
		if err := ev.baseValidator.ValidateInput(headerValue); err != nil {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("header.%s", headerName),
				Tag:     "security",
				Message: fmt.Sprintf("Header contains potentially malicious content: %s", err.Error()),
			})
		}

		// Special validation for specific headers
		switch strings.ToLower(headerName) {
		case "content-type":
			if !ev.isValidContentType(headerValue) {
				errors = append(errors, ValidationError{
					Field:   fmt.Sprintf("header.%s", headerName),
					Tag:     "format",
					Message: "Invalid Content-Type header",
				})
			}
		case "authorization":
			if !ev.isValidAuthorizationHeader(headerValue) {
				errors = append(errors, ValidationError{
					Field:   fmt.Sprintf("header.%s", headerName),
					Tag:     "format",
					Message: "Invalid Authorization header format",
				})
			}
		}
	}

	return errors
}

// validateRequestBody validates request body according to endpoint rules
func (ev *EnhancedValidator) validateRequestBody(c *gin.Context, rule *EndpointValidationRule) []ValidationError {
	var errors []ValidationError

	if rule == nil || rule.BodyValidation == nil {
		return errors
	}

	bodyValidation := rule.BodyValidation

	// Read and validate body size
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		errors = append(errors, ValidationError{
			Field:   "body",
			Tag:     "read_error",
			Message: fmt.Sprintf("Failed to read request body: %s", err.Error()),
		})
		return errors
	}

	// Restore body for subsequent middleware
	c.Request.Body = io.NopCloser(bytes.NewBuffer(body))

	// Check body size
	if int64(len(body)) > bodyValidation.MaxSize {
		errors = append(errors, ValidationError{
			Field:   "body",
			Tag:     "size",
			Message: fmt.Sprintf("Request body too large: %d bytes (max: %d bytes)", len(body), bodyValidation.MaxSize),
		})
		return errors
	}

	// Validate content type
	contentType := c.GetHeader("Content-Type")
	if !strings.Contains(contentType, bodyValidation.ContentType) {
		errors = append(errors, ValidationError{
			Field:   "body",
			Tag:     "content_type",
			Message: fmt.Sprintf("Invalid content type: expected %s", bodyValidation.ContentType),
		})
		return errors
	}

	// Parse and validate JSON body
	if bodyValidation.ContentType == "application/json" {
		var bodyData map[string]interface{}
		if err := json.Unmarshal(body, &bodyData); err != nil {
			errors = append(errors, ValidationError{
				Field:   "body",
				Tag:     "json",
				Message: fmt.Sprintf("Invalid JSON: %s", err.Error()),
			})
			return errors
		}

		// Validate required fields
		for _, field := range bodyValidation.RequiredFields {
			if _, exists := bodyData[field]; !exists {
				errors = append(errors, ValidationError{
					Field:   fmt.Sprintf("body.%s", field),
					Tag:     "required",
					Message: fmt.Sprintf("Required field '%s' is missing", field),
				})
			}
		}

		// Validate field values
		for field, value := range bodyData {
			fieldRule, allowed := bodyValidation.AllowedFields[field]

			// Check if field is allowed
			if !allowed {
				if ev.config.StrictModeEnabled {
					errors = append(errors, ValidationError{
						Field:   fmt.Sprintf("body.%s", field),
						Tag:     "forbidden",
						Message: fmt.Sprintf("Field '%s' is not allowed", field),
					})
					continue
				}
			}

			// Convert value to string for validation
			valueStr := fmt.Sprintf("%v", value)

			// Apply security validation
			if err := ev.baseValidator.ValidateInput(valueStr); err != nil {
				errors = append(errors, ValidationError{
					Field:   fmt.Sprintf("body.%s", field),
					Tag:     "security",
					Message: fmt.Sprintf("Field contains potentially malicious content: %s", err.Error()),
				})
				continue
			}

			if validateErrs := ev.validateParamValue(field, valueStr, fieldRule, "body"); len(validateErrs) > 0 {
				errors = append(errors, validateErrs...)
			}
		}
	}

	return errors
}

// applyEndpointSpecificValidation applies custom validation logic for specific endpoints
func (ev *EnhancedValidator) applyEndpointSpecificValidation(c *gin.Context, rule *EndpointValidationRule) []ValidationError {
	var errors []ValidationError

	// Apply custom validators based on endpoint
	switch {
	case strings.Contains(rule.Path, "/trading/orders") && c.Request.Method == "POST":
		errors = append(errors, ev.validateTradingOrderPlacement(c)...)
	case strings.Contains(rule.Path, "/fiat/deposit"):
		errors = append(errors, ev.validateFiatDeposit(c)...)
	case strings.Contains(rule.Path, "/admin/trading/pairs"):
		errors = append(errors, ev.validateAdminTradingPair(c)...)
	}

	return errors
}

// validateParamValue validates a parameter value against its rule
func (ev *EnhancedValidator) validateParamValue(fieldName, value string, rule ParamRule, context string) []ValidationError {
	var errors []ValidationError

	// Type validation
	switch rule.Type {
	case "int":
		if _, err := strconv.Atoi(value); err != nil {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("%s.%s", context, fieldName),
				Tag:     "type",
				Message: fmt.Sprintf("Field '%s' must be an integer", fieldName),
			})
			return errors
		}
	case "float":
		if _, err := strconv.ParseFloat(value, 64); err != nil {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("%s.%s", context, fieldName),
				Tag:     "type",
				Message: fmt.Sprintf("Field '%s' must be a number", fieldName),
			})
			return errors
		}
	case "decimal":
		if _, err := decimal.NewFromString(value); err != nil {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("%s.%s", context, fieldName),
				Tag:     "type",
				Message: fmt.Sprintf("Field '%s' must be a valid decimal number", fieldName),
			})
			return errors
		}
	case "uuid":
		if !ev.isValidUUID(value) {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("%s.%s", context, fieldName),
				Tag:     "format",
				Message: fmt.Sprintf("Field '%s' must be a valid UUID", fieldName),
			})
			return errors
		}
	case "bool":
		if _, err := strconv.ParseBool(value); err != nil {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("%s.%s", context, fieldName),
				Tag:     "type",
				Message: fmt.Sprintf("Field '%s' must be a boolean", fieldName),
			})
			return errors
		}
	}

	// Length validation
	if rule.MinLength != nil && len(value) < *rule.MinLength {
		errors = append(errors, ValidationError{
			Field:   fmt.Sprintf("%s.%s", context, fieldName),
			Tag:     "min_length",
			Message: fmt.Sprintf("Field '%s' must be at least %d characters long", fieldName, *rule.MinLength),
		})
	}
	if rule.MaxLength != nil && len(value) > *rule.MaxLength {
		errors = append(errors, ValidationError{
			Field:   fmt.Sprintf("%s.%s", context, fieldName),
			Tag:     "max_length",
			Message: fmt.Sprintf("Field '%s' must be at most %d characters long", fieldName, *rule.MaxLength),
		})
	}

	// Numeric value validation
	if rule.MinValue != nil || rule.MaxValue != nil {
		var numValue float64
		var err error

		if rule.Type == "decimal" {
			if dec, parseErr := decimal.NewFromString(value); parseErr == nil {
				numValue, _ = dec.Float64()
			} else {
				err = parseErr
			}
		} else {
			numValue, err = strconv.ParseFloat(value, 64)
		}

		if err == nil {
			if rule.MinValue != nil && numValue < *rule.MinValue {
				errors = append(errors, ValidationError{
					Field:   fmt.Sprintf("%s.%s", context, fieldName),
					Tag:     "min_value",
					Message: fmt.Sprintf("Field '%s' must be at least %v", fieldName, *rule.MinValue),
				})
			}
			if rule.MaxValue != nil && numValue > *rule.MaxValue {
				errors = append(errors, ValidationError{
					Field:   fmt.Sprintf("%s.%s", context, fieldName),
					Tag:     "max_value",
					Message: fmt.Sprintf("Field '%s' must be at most %v", fieldName, *rule.MaxValue),
				})
			}
		}
	}

	// Pattern validation
	if rule.PatternRegex != nil && !rule.PatternRegex.MatchString(value) {
		errors = append(errors, ValidationError{
			Field:   fmt.Sprintf("%s.%s", context, fieldName),
			Tag:     "pattern",
			Message: fmt.Sprintf("Field '%s' does not match required pattern", fieldName),
		})
	}

	// Allowed values validation
	if len(rule.AllowedValues) > 0 {
		allowed := false
		for _, allowedValue := range rule.AllowedValues {
			if value == allowedValue {
				allowed = true
				break
			}
		}
		if !allowed {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("%s.%s", context, fieldName),
				Tag:     "allowed_values",
				Message: fmt.Sprintf("Field '%s' must be one of: %s", fieldName, strings.Join(rule.AllowedValues, ", ")),
			})
		}
	}

	return errors
}

// Security and format validation helper methods

// isValidUUID validates UUID format
func (ev *EnhancedValidator) isValidUUID(value string) bool {
	_, err := uuid.Parse(value)
	return err == nil
}

// isValidTradingSymbol validates trading symbol format
func (ev *EnhancedValidator) isValidTradingSymbol(symbol string) bool {
	pattern := regexp.MustCompile(`^[A-Z]{2,6}[/-][A-Z]{2,6}$`)
	return pattern.MatchString(symbol)
}

// isValidContentType validates Content-Type header
func (ev *EnhancedValidator) isValidContentType(contentType string) bool {
	allowedTypes := []string{
		"application/json",
		"application/x-www-form-urlencoded",
		"multipart/form-data",
		"text/plain",
	}

	for _, allowedType := range allowedTypes {
		if strings.Contains(contentType, allowedType) {
			return true
		}
	}
	return false
}

// isValidAuthorizationHeader validates Authorization header format
func (ev *EnhancedValidator) isValidAuthorizationHeader(auth string) bool {
	// Basic format validation for Bearer tokens and API keys
	patterns := []string{
		`^Bearer [A-Za-z0-9\-\._~\+\/]+=*$`,
		`^API-Key [A-Za-z0-9\-\._~\+\/]+=*$`,
		`^Basic [A-Za-z0-9\+\/]+=*$`,
	}

	for _, pattern := range patterns {
		if matched, _ := regexp.MatchString(pattern, auth); matched {
			return true
		}
	}
	return false
}

// Custom endpoint-specific validation methods

// validateTradingOrderPlacement applies trading-specific validation
func (ev *EnhancedValidator) validateTradingOrderPlacement(c *gin.Context) []ValidationError {
	var errors []ValidationError

	// Read and parse body
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return errors
	}
	c.Request.Body = io.NopCloser(bytes.NewBuffer(body))

	var orderData map[string]interface{}
	if err := json.Unmarshal(body, &orderData); err != nil {
		return errors
	}

	// Validate business logic constraints
	orderType, _ := orderData["type"].(string)
	price, priceExists := orderData["price"]
	stopPrice, stopPriceExists := orderData["stop_price"]

	// Limit orders must have price
	if strings.ToLower(orderType) == "limit" && !priceExists {
		errors = append(errors, ValidationError{
			Field:   "body.price",
			Tag:     "business_logic",
			Message: "Limit orders must specify a price",
		})
	}

	// Stop orders must have stop_price
	if strings.Contains(strings.ToLower(orderType), "stop") && !stopPriceExists {
		errors = append(errors, ValidationError{
			Field:   "body.stop_price",
			Tag:     "business_logic",
			Message: "Stop orders must specify a stop price",
		})
	}

	// Validate price relationship for stop-limit orders
	if orderType == "stop_limit" && priceExists && stopPriceExists {
		priceVal, _ := decimal.NewFromString(fmt.Sprintf("%v", price))
		stopPriceVal, _ := decimal.NewFromString(fmt.Sprintf("%v", stopPrice))
		side, _ := orderData["side"].(string)

		if strings.ToLower(side) == "buy" && priceVal.LessThan(stopPriceVal) {
			errors = append(errors, ValidationError{
				Field:   "body.price",
				Tag:     "business_logic",
				Message: "For buy stop-limit orders, price must be >= stop_price",
			})
		}
		if strings.ToLower(side) == "sell" && priceVal.GreaterThan(stopPriceVal) {
			errors = append(errors, ValidationError{
				Field:   "body.price",
				Tag:     "business_logic",
				Message: "For sell stop-limit orders, price must be <= stop_price",
			})
		}
	}

	return errors
}

// validateFiatDeposit applies fiat-specific validation
func (ev *EnhancedValidator) validateFiatDeposit(c *gin.Context) []ValidationError {
	var errors []ValidationError

	// Read and parse body
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return errors
	}
	c.Request.Body = io.NopCloser(bytes.NewBuffer(body))

	var depositData map[string]interface{}
	if err := json.Unmarshal(body, &depositData); err != nil {
		return errors
	}

	// Validate business logic
	paymentMethod, _ := depositData["payment_method"].(string)
	bankAccountID, bankAccountExists := depositData["bank_account_id"]

	// Bank transfers require bank account ID
	if paymentMethod == "bank_transfer" && !bankAccountExists {
		errors = append(errors, ValidationError{
			Field:   "body.bank_account_id",
			Tag:     "business_logic",
			Message: "Bank transfers require a bank account ID",
		})
	}

	return errors
}

// validateAdminTradingPair applies admin trading pair validation
func (ev *EnhancedValidator) validateAdminTradingPair(c *gin.Context) []ValidationError {
	var errors []ValidationError

	// Read and parse body
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return errors
	}
	c.Request.Body = io.NopCloser(bytes.NewBuffer(body))

	var pairData map[string]interface{}
	if err := json.Unmarshal(body, &pairData); err != nil {
		return errors
	}

	// Validate business logic
	minQty, _ := decimal.NewFromString(fmt.Sprintf("%v", pairData["min_quantity"]))
	maxQty, _ := decimal.NewFromString(fmt.Sprintf("%v", pairData["max_quantity"]))

	// Min quantity must be less than max quantity
	if minQty.GreaterThanOrEqual(maxQty) {
		errors = append(errors, ValidationError{
			Field:   "body.min_quantity",
			Tag:     "business_logic",
			Message: "Minimum quantity must be less than maximum quantity",
		})
	}

	// Validate symbol matches base and quote assets
	symbol, _ := pairData["symbol"].(string)
	baseAsset, _ := pairData["base_asset"].(string)
	quoteAsset, _ := pairData["quote_asset"].(string)
	expectedSymbol := fmt.Sprintf("%s/%s", baseAsset, quoteAsset)

	if symbol != expectedSymbol {
		errors = append(errors, ValidationError{
			Field:   "body.symbol",
			Tag:     "business_logic",
			Message: fmt.Sprintf("Symbol must match base_asset/quote_asset format: expected %s", expectedSymbol),
		})
	}

	return errors
}
