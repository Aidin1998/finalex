package validation

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Aidin1998/finalex/common/errors"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// UnifiedValidationConfig provides comprehensive configuration for all validation features
type UnifiedValidationConfig struct {
	// Basic validation settings
	MaxRequestBodySize       int64         `json:"max_request_body_size"`
	MaxQueryParamCount       int           `json:"max_query_param_count"`
	MaxHeaderCount           int           `json:"max_header_count"`
	MaxFieldDepth            int           `json:"max_field_depth"`
	RequestTimeout           time.Duration `json:"request_timeout"`
	EnablePerformanceLogging bool          `json:"enable_performance_logging"`
	EnableDetailedErrorLogs  bool          `json:"enable_detailed_error_logs"`
	StrictModeEnabled        bool          `json:"strict_mode_enabled"`

	// Security features
	EnableSecurityHardening    bool `json:"enable_security_hardening"`
	EnableIPBlacklisting       bool `json:"enable_ip_blacklisting"`
	EnableFingerprintTracking  bool `json:"enable_fingerprint_tracking"`
	EnableAdvancedSQLInjection bool `json:"enable_advanced_sql_injection"`
	EnableXSSProtection        bool `json:"enable_xss_protection"`
	EnableCSRFProtection       bool `json:"enable_csrf_protection"`

	// Rate limiting
	EnableRateLimiting        bool          `json:"enable_rate_limiting"`
	MaxRequestsPerMinute      int           `json:"max_requests_per_minute"`
	MaxRequestsPerEndpoint    int           `json:"max_requests_per_endpoint"`
	RateLimitValidationCount  int           `json:"rate_limit_validation_count"`
	RateLimitValidationWindow time.Duration `json:"rate_limit_validation_window"`
	SuspiciousActivityWindow  time.Duration `json:"suspicious_activity_window"`
	BlockDuration             time.Duration `json:"block_duration"`

	// Enhanced validation features
	EnableEndpointSpecificRules bool                               `json:"enable_endpoint_specific_rules"`
	EndpointRules               map[string]*EndpointValidationRule `json:"endpoint_rules"`
	CustomValidators            []string                           `json:"custom_validators"`
	PerformanceLimit            time.Duration                      `json:"performance_limit"`

	// Error response configuration
	UseRFC7807Format      bool `json:"use_rfc7807_format"`
	EnableTraceIDs        bool `json:"enable_trace_ids"`
	DetailedErrorMessages bool `json:"detailed_error_messages"`
}

// DefaultUnifiedValidationConfig returns a secure default configuration
func DefaultUnifiedValidationConfig() *UnifiedValidationConfig {
	return &UnifiedValidationConfig{
		// Basic validation
		MaxRequestBodySize:       10 * 1024 * 1024, // 10MB
		MaxQueryParamCount:       50,
		MaxHeaderCount:           100,
		MaxFieldDepth:            10,
		RequestTimeout:           30 * time.Second,
		EnablePerformanceLogging: false,
		EnableDetailedErrorLogs:  true,
		StrictModeEnabled:        true,

		// Security features (all enabled by default for maximum security)
		EnableSecurityHardening:    true,
		EnableIPBlacklisting:       true,
		EnableFingerprintTracking:  true,
		EnableAdvancedSQLInjection: true,
		EnableXSSProtection:        true,
		EnableCSRFProtection:       true,

		// Rate limiting
		EnableRateLimiting:        true,
		MaxRequestsPerMinute:      300,
		MaxRequestsPerEndpoint:    100,
		RateLimitValidationCount:  1000,
		RateLimitValidationWindow: time.Minute,
		SuspiciousActivityWindow:  5 * time.Minute,
		BlockDuration:             15 * time.Minute,

		// Enhanced validation
		EnableEndpointSpecificRules: true,
		EndpointRules:               make(map[string]*EndpointValidationRule),
		PerformanceLimit:            50 * time.Millisecond,

		// Error responses
		UseRFC7807Format:      true,
		EnableTraceIDs:        true,
		DetailedErrorMessages: false, // Disabled for security in production
	}
}

// UnifiedValidator provides comprehensive validation combining all previous systems
type UnifiedValidator struct {
	config            *UnifiedValidationConfig
	baseValidator     *Validator
	securityHardening *SecurityHardening
	logger            *zap.Logger
	redis             *redis.Client
	errorHandler      *errors.UnifiedErrorHandler

	// Internal state
	endpointRules      map[string]*EndpointValidationRule
	rateLimitCounters  map[string]int
	ipBlacklist        map[string]time.Time
	suspiciousPatterns []*regexp.Regexp
	metricsEnabled     bool
}

// NewUnifiedValidator creates a new comprehensive validation system
func NewUnifiedValidator(logger *zap.Logger, redis *redis.Client, config *UnifiedValidationConfig) *UnifiedValidator {
	if config == nil {
		config = DefaultUnifiedValidationConfig()
	}

	baseValidator := NewValidator(logger)
	var securityHardening *SecurityHardening
	if config.EnableSecurityHardening {
		securityConfig := &SecurityConfig{
			EnableIPBlacklisting:       config.EnableIPBlacklisting,
			EnableFingerprintTracking:  config.EnableFingerprintTracking,
			EnableAdvancedSQLInjection: config.EnableAdvancedSQLInjection,
			EnableXSSProtection:        config.EnableXSSProtection,
			EnableCSRFProtection:       config.EnableCSRFProtection,
			EnableRateLimiting:         config.EnableRateLimiting,
			MaxRequestsPerMinute:       config.MaxRequestsPerMinute,
			MaxRequestsPerEndpoint:     config.MaxRequestsPerEndpoint,
			SuspiciousActivityWindow:   config.SuspiciousActivityWindow,
			BlockDuration:              config.BlockDuration,
		}
		securityHardening = NewSecurityHardening(logger, securityConfig)
	}

	uv := &UnifiedValidator{
		config:            config,
		baseValidator:     baseValidator,
		securityHardening: securityHardening,
		logger:            logger,
		redis:             redis,
		errorHandler:      errors.NewUnifiedErrorHandler(),
		endpointRules:     make(map[string]*EndpointValidationRule),
		rateLimitCounters: make(map[string]int),
		ipBlacklist:       make(map[string]time.Time),
		metricsEnabled:    true,
	}

	// Initialize default endpoint rules if enabled
	if config.EnableEndpointSpecificRules {
		uv.initializeDefaultRules()
	}

	// Copy custom endpoint rules
	for path, rule := range config.EndpointRules {
		uv.endpointRules[path] = rule
	}

	return uv
}

// UnifiedValidationMiddleware creates the comprehensive validation middleware
func UnifiedValidationMiddleware(logger *zap.Logger, redis *redis.Client, config *UnifiedValidationConfig) gin.HandlerFunc {
	validator := NewUnifiedValidator(logger, redis, config)

	return func(c *gin.Context) {
		startTime := time.Now()

		// Apply request timeout if configured
		if validator.config.RequestTimeout > 0 {
			ctx, cancel := context.WithTimeout(c.Request.Context(), validator.config.RequestTimeout)
			defer cancel()
			c.Request = c.Request.WithContext(ctx)
		}

		// Perform comprehensive validation
		result := validator.ValidateRequest(c)

		if !result.Valid {
			// Log validation failure
			if validator.config.EnableDetailedErrorLogs {
				validator.logger.Warn("Unified request validation failed",
					zap.String("method", c.Request.Method),
					zap.String("path", c.Request.URL.Path),
					zap.String("remote_addr", c.ClientIP()),
					zap.Any("errors", result.Errors),
					zap.Duration("processing_time", result.ProcessingTime))
			}

			// Handle validation errors using unified error handler
			if validator.config.UseRFC7807Format {
				// Convert validation errors to RFC 7807 format
				validationErrors := make([]errors.ValidationError, len(result.Errors))
				for i, err := range result.Errors {
					validationErrors[i] = errors.ValidationError{
						Field:   err.Field,
						Message: err.Message,
						Code:    err.Tag,
					}
				}

				problemDetails := errors.NewValidationError(
					"Request validation failed",
					c.Request.URL.Path,
				).WithValidationErrors(validationErrors)

				// Add trace ID if available
				if validator.config.EnableTraceIDs {
					if traceID := c.GetHeader("X-Trace-ID"); traceID != "" {
						problemDetails.WithTraceID(traceID)
					}
				}

				c.Header("Content-Type", "application/problem+json")
				c.JSON(problemDetails.Status, problemDetails)
			} else {
				// Fallback to simple error response
				c.JSON(http.StatusBadRequest, gin.H{
					"error":             "Request validation failed",
					"validation_errors": result.Errors,
					"timestamp":         time.Now().UTC().Format(time.RFC3339),
				})
			}

			c.Abort()
			return
		}

		// Log performance metrics if enabled
		if validator.config.EnablePerformanceLogging {
			processingTime := time.Since(startTime)
			if processingTime > validator.config.PerformanceLimit {
				validator.logger.Warn("Validation performance limit exceeded",
					zap.String("method", c.Request.Method),
					zap.String("path", c.Request.URL.Path),
					zap.Duration("processing_time", processingTime),
					zap.Duration("limit", validator.config.PerformanceLimit))
			}
		}

		c.Next()
	}
}

// ValidateRequest performs comprehensive request validation
func (uv *UnifiedValidator) ValidateRequest(c *gin.Context) *ValidationResult {
	startTime := time.Now()
	result := &ValidationResult{
		Valid:          true,
		Errors:         make([]ValidationError, 0),
		Warnings:       make([]string, 0),
		ProcessingTime: 0,
		Metadata:       make(map[string]interface{}),
	}

	// 1. Basic request validation
	if err := uv.validateBasicRequest(c); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "request",
			Tag:     "basic_validation",
			Message: err.Error(),
		})
	}

	// 2. Security hardening validation
	if uv.config.EnableSecurityHardening && uv.securityHardening != nil {
		if securityErrors := uv.securityHardening.ValidateAdvancedSecurity(c); len(securityErrors) > 0 {
			result.Valid = false
			result.Errors = append(result.Errors, securityErrors...)
		}
	}

	// 3. Rate limiting validation
	if uv.config.EnableRateLimiting {
		if rateLimitError := uv.validateRateLimit(c); rateLimitError != nil {
			result.Valid = false
			result.Errors = append(result.Errors, *rateLimitError)
		}
	}

	// 4. Endpoint-specific validation
	if uv.config.EnableEndpointSpecificRules {
		if rule := uv.findEndpointRule(c.Request.Method, c.Request.URL.Path); rule != nil {
			// Query parameters validation
			if queryErrors := uv.validateQueryParams(c, rule); len(queryErrors) > 0 {
				result.Valid = false
				result.Errors = append(result.Errors, queryErrors...)
			}

			// Path parameters validation
			if pathErrors := uv.validatePathParams(c, rule); len(pathErrors) > 0 {
				result.Valid = false
				result.Errors = append(result.Errors, pathErrors...)
			}

			// Headers validation
			if headerErrors := uv.validateHeaders(c, rule); len(headerErrors) > 0 {
				result.Valid = false
				result.Errors = append(result.Errors, headerErrors...)
			}

			// Body validation
			if bodyErrors := uv.validateRequestBody(c, rule); len(bodyErrors) > 0 {
				result.Valid = false
				result.Errors = append(result.Errors, bodyErrors...)
			}
		}
	}

	// 5. Basic input validation using base validator
	if basicErrors := uv.performBasicInputValidation(c); len(basicErrors) > 0 {
		result.Valid = false
		result.Errors = append(result.Errors, basicErrors...)
	}

	result.ProcessingTime = time.Since(startTime)
	return result
}

// Helper methods

func (uv *UnifiedValidator) validateBasicRequest(c *gin.Context) error {
	// Content length validation
	if c.Request.ContentLength > uv.config.MaxRequestBodySize {
		return fmt.Errorf("request body too large: %d bytes (max: %d)",
			c.Request.ContentLength, uv.config.MaxRequestBodySize)
	}

	// Query parameter count validation
	if len(c.Request.URL.Query()) > uv.config.MaxQueryParamCount {
		return fmt.Errorf("too many query parameters: %d (max: %d)",
			len(c.Request.URL.Query()), uv.config.MaxQueryParamCount)
	}

	// Header count validation
	if len(c.Request.Header) > uv.config.MaxHeaderCount {
		return fmt.Errorf("too many headers: %d (max: %d)",
			len(c.Request.Header), uv.config.MaxHeaderCount)
	}

	return nil
}

func (uv *UnifiedValidator) validateRateLimit(c *gin.Context) *ValidationError {
	clientIP := c.ClientIP()
	endpoint := c.Request.Method + ":" + c.Request.URL.Path

	// Check global rate limit per IP
	ipKey := fmt.Sprintf("rate_limit:ip:%s", clientIP)
	if count, err := uv.checkRateLimit(ipKey, uv.config.MaxRequestsPerMinute, time.Minute); err == nil && count > uv.config.MaxRequestsPerMinute {
		return &ValidationError{
			Field:   "rate_limit",
			Tag:     "global",
			Message: fmt.Sprintf("Rate limit exceeded for IP %s", clientIP),
		}
	}

	// Check endpoint-specific rate limit
	endpointKey := fmt.Sprintf("rate_limit:endpoint:%s:%s", clientIP, endpoint)
	if count, err := uv.checkRateLimit(endpointKey, uv.config.MaxRequestsPerEndpoint, time.Minute); err == nil && count > uv.config.MaxRequestsPerEndpoint {
		return &ValidationError{
			Field:   "rate_limit",
			Tag:     "endpoint",
			Message: fmt.Sprintf("Rate limit exceeded for endpoint %s", endpoint),
		}
	}

	return nil
}

func (uv *UnifiedValidator) checkRateLimit(key string, limit int, window time.Duration) (int, error) {
	if uv.redis == nil {
		return 0, fmt.Errorf("redis client not available")
	}

	ctx := context.Background()

	// Use Redis for distributed rate limiting
	current, err := uv.redis.Incr(ctx, key).Result()
	if err != nil {
		return 0, err
	}

	if current == 1 {
		uv.redis.Expire(ctx, key, window)
	}

	return int(current), nil
}

// Integration methods from other validation systems

func (uv *UnifiedValidator) validateQueryParams(c *gin.Context, rule *EndpointValidationRule) []ValidationError {
	// Implementation similar to enhanced_middleware.go but simplified
	var errors []ValidationError
	queryParams := c.Request.URL.Query()

	for key, values := range queryParams {
		if rule.AllowedParams != nil {
			if paramRule, allowed := rule.AllowedParams[key]; allowed {
				for _, value := range values {
					if validateErrs := uv.validateParamValue(key, value, paramRule, "query"); len(validateErrs) > 0 {
						errors = append(errors, validateErrs...)
					}
				}
			} else {
				errors = append(errors, ValidationError{
					Field:   fmt.Sprintf("query.%s", key),
					Tag:     "forbidden",
					Message: fmt.Sprintf("Query parameter '%s' is not allowed", key),
				})
			}
		}
	}

	// Check required parameters
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

func (uv *UnifiedValidator) validatePathParams(c *gin.Context, rule *EndpointValidationRule) []ValidationError {
	var errors []ValidationError

	for _, param := range c.Params {
		// Security validation
		if ContainsSQLInjection(param.Value) || ContainsXSS(param.Value) {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("path.%s", param.Key),
				Tag:     "security",
				Message: "Path parameter contains potentially malicious content",
			})
		}

		// Type validation for common path parameters
		switch param.Key {
		case "id":
			if !isValidUUID(param.Value) {
				errors = append(errors, ValidationError{
					Field:   fmt.Sprintf("path.%s", param.Key),
					Tag:     "format",
					Message: "Path parameter 'id' must be a valid UUID",
				})
			}
		case "symbol":
			if !isValidTradingSymbol(param.Value) {
				errors = append(errors, ValidationError{
					Field:   fmt.Sprintf("path.%s", param.Key),
					Tag:     "format",
					Message: "Invalid trading symbol format",
				})
			}
		}
	}

	return errors
}

func (uv *UnifiedValidator) validateHeaders(c *gin.Context, rule *EndpointValidationRule) []ValidationError {
	var errors []ValidationError

	// Validate required headers
	for _, headerName := range rule.RequiredHeaders {
		if c.GetHeader(headerName) == "" {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("header.%s", headerName),
				Tag:     "required",
				Message: fmt.Sprintf("Required header '%s' is missing", headerName),
			})
		}
	}

	// Validate specific header formats
	for headerName, headerValue := range c.Request.Header {
		if len(headerValue) > 0 {
			value := headerValue[0]
			switch strings.ToLower(headerName) {
			case "authorization":
				if !isValidAuthorizationHeader(value) {
					errors = append(errors, ValidationError{
						Field:   fmt.Sprintf("header.%s", headerName),
						Tag:     "format",
						Message: "Invalid Authorization header format",
					})
				}
			case "content-type":
				if !isValidContentType(value) {
					errors = append(errors, ValidationError{
						Field:   fmt.Sprintf("header.%s", headerName),
						Tag:     "format",
						Message: "Invalid Content-Type header",
					})
				}
			}
		}
	}

	return errors
}

func (uv *UnifiedValidator) validateRequestBody(c *gin.Context, rule *EndpointValidationRule) []ValidationError {
	// Implementation would be similar to enhanced_middleware.go
	// For now, return empty slice - can be expanded later
	return []ValidationError{}
}

func (uv *UnifiedValidator) validateParamValue(fieldName, value string, rule ParamRule, context string) []ValidationError {
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
		}
	case "float":
		if _, err := strconv.ParseFloat(value, 64); err != nil {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("%s.%s", context, fieldName),
				Tag:     "type",
				Message: fmt.Sprintf("Field '%s' must be a number", fieldName),
			})
		}
	case "uuid":
		if !isValidUUID(value) {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("%s.%s", context, fieldName),
				Tag:     "format",
				Message: fmt.Sprintf("Field '%s' must be a valid UUID", fieldName),
			})
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

	// Pattern validation
	if rule.Pattern != "" && rule.PatternRegex != nil {
		if !rule.PatternRegex.MatchString(value) {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("%s.%s", context, fieldName),
				Tag:     "pattern",
				Message: fmt.Sprintf("Field '%s' does not match required pattern", fieldName),
			})
		}
	}

	return errors
}

func (uv *UnifiedValidator) performBasicInputValidation(c *gin.Context) []ValidationError {
	var errors []ValidationError

	// Validate all query parameters for basic security
	for param, values := range c.Request.URL.Query() {
		for _, value := range values {
			if ContainsSQLInjection(value) {
				errors = append(errors, ValidationError{
					Field:   param,
					Tag:     "sql_injection",
					Message: "Query parameter contains potential SQL injection",
				})
			}
			if ContainsXSS(value) {
				errors = append(errors, ValidationError{
					Field:   param,
					Tag:     "xss",
					Message: "Query parameter contains potential XSS",
				})
			}
		}
	}

	return errors
}

func (uv *UnifiedValidator) findEndpointRule(method, path string) *EndpointValidationRule {
	key := method + ":" + path
	if rule, exists := uv.endpointRules[key]; exists {
		return rule
	}

	// Check for pattern matches
	for ruleKey, rule := range uv.endpointRules {
		if rule.PathPattern != nil && rule.PathPattern.MatchString(path) {
			if strings.HasPrefix(ruleKey, method+":") {
				return rule
			}
		}
	}

	return nil
}

func (uv *UnifiedValidator) initializeDefaultRules() {
	// Add some default trading endpoint rules
	uv.addEndpointRule(&EndpointValidationRule{
		Method:         "POST",
		Path:           "/api/v1/trading/orders",
		RequiredParams: []string{"symbol", "side", "type"},
		AllowedParams: map[string]ParamRule{
			"symbol": {Type: "string", Required: true, Pattern: "^[A-Z]+/[A-Z]+$"},
			"side":   {Type: "string", Required: true, AllowedValues: []string{"buy", "sell"}},
			"type":   {Type: "string", Required: true, AllowedValues: []string{"market", "limit"}},
			"amount": {Type: "decimal", Required: true},
			"price":  {Type: "decimal", Required: false},
		},
	})
}

func (uv *UnifiedValidator) addEndpointRule(rule *EndpointValidationRule) {
	key := rule.Method + ":" + rule.Path

	// PathPattern should already be compiled, but if not and Path exists, compile it
	if rule.PathPattern == nil && rule.Path != "" {
		// Convert path with parameters to regex pattern
		pattern := rule.Path
		pattern = strings.ReplaceAll(pattern, ":id", `[a-fA-F0-9-]{36}`)
		pattern = strings.ReplaceAll(pattern, ":symbol", `[A-Z]{2,6}[/-][A-Z]{2,6}`)
		pattern = "^" + pattern + "/?$"
		if regex, err := regexp.Compile(pattern); err == nil {
			rule.PathPattern = regex
		}
	}

	// Compile parameter patterns
	for name, paramRule := range rule.AllowedParams {
		if paramRule.Pattern != "" {
			if regex, err := regexp.Compile(paramRule.Pattern); err == nil {
				paramRule.PatternRegex = regex
				rule.AllowedParams[name] = paramRule
			}
		}
	}

	uv.endpointRules[key] = rule
}

// Helper functions
func isValidUUID(s string) bool {
	uuidRegex := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	return uuidRegex.MatchString(strings.ToLower(s))
}

func isValidTradingSymbol(s string) bool {
	symbolRegex := regexp.MustCompile(`^[A-Z0-9]{2,10}[/]?[A-Z0-9]{2,10}$`)
	return symbolRegex.MatchString(strings.ToUpper(s))
}

func isValidAuthorizationHeader(value string) bool {
	return strings.HasPrefix(value, "Bearer ") || strings.HasPrefix(value, "Basic ")
}

func isValidContentType(value string) bool {
	validTypes := []string{
		"application/json",
		"application/x-www-form-urlencoded",
		"multipart/form-data",
		"text/plain",
	}
	for _, validType := range validTypes {
		if strings.Contains(value, validType) {
			return true
		}
	}
	return false
}

// Global unified validation middleware factory
func CreateUnifiedValidationMiddleware(logger *zap.Logger, redis *redis.Client, profileName string) gin.HandlerFunc {
	var config *UnifiedValidationConfig

	switch profileName {
	case "strict":
		config = DefaultUnifiedValidationConfig()
	case "medium":
		config = DefaultUnifiedValidationConfig()
		config.EnableSecurityHardening = true
		config.EnableAdvancedSQLInjection = false
		config.MaxRequestsPerMinute = 500
	case "basic":
		config = DefaultUnifiedValidationConfig()
		config.EnableSecurityHardening = false
		config.EnableEndpointSpecificRules = false
		config.MaxRequestsPerMinute = 1000
	default:
		config = DefaultUnifiedValidationConfig()
	}

	return UnifiedValidationMiddleware(logger, redis, config)
}
