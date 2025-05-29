package validation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// ValidationMiddleware creates a middleware for comprehensive input validation
func ValidationMiddleware(logger *zap.Logger) gin.HandlerFunc {
	validator := NewValidator(logger)

	return func(c *gin.Context) {
		// Validate query parameters
		if err := validateQueryParams(validator, c); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid query parameters",
				"details": err.Error(),
			})
			c.Abort()
			return
		}

		// Validate path parameters
		if err := validatePathParams(validator, c); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid path parameters",
				"details": err.Error(),
			})
			c.Abort()
			return
		}

		// Validate headers
		if err := validateHeaders(validator, c); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid headers",
				"details": err.Error(),
			})
			c.Abort()
			return
		}

		// Validate request body for POST/PUT/PATCH requests
		if c.Request.Method == "POST" || c.Request.Method == "PUT" || c.Request.Method == "PATCH" {
			if err := validateRequestBody(validator, c); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{
					"error":   "Invalid request body",
					"details": err.Error(),
				})
				c.Abort()
				return
			}
		}

		c.Next()
	}
}

// validateQueryParams validates all query parameters
func validateQueryParams(validator *Validator, c *gin.Context) error {
	for key, values := range c.Request.URL.Query() {
		// Validate parameter name
		if ContainsSQLInjection(key) || ContainsXSS(key) {
			return fmt.Errorf("invalid parameter name: %s", key)
		}

		// Validate each parameter value
		for _, value := range values {
			if err := validateParam(validator, key, value); err != nil {
				return err
			}
		}
	}
	return nil
}

// validatePathParams validates path parameters
func validatePathParams(validator *Validator, c *gin.Context) error {
	for _, param := range c.Params {
		if err := validateParam(validator, param.Key, param.Value); err != nil {
			return err
		}
	}
	return nil
}

// validateHeaders validates important headers
func validateHeaders(validator *Validator, c *gin.Context) error {
	// Headers to validate
	headersToValidate := []string{
		"User-Agent",
		"Content-Type",
		"Accept",
		"Origin",
		"Referer",
	}

	for _, header := range headersToValidate {
		value := c.GetHeader(header)
		if value != "" {
			if ContainsSQLInjection(value) || ContainsXSS(value) {
				return fmt.Errorf("invalid header value for %s", header)
			}

			// Additional length check
			if len(value) > 2000 {
				return fmt.Errorf("header %s exceeds maximum length", header)
			}
		}
	}

	return nil
}

// validateRequestBody validates JSON request body
func validateRequestBody(validator *Validator, c *gin.Context) error {
	// Read the body
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return fmt.Errorf("failed to read request body: %v", err)
	}

	// Restore the body for further processing
	c.Request.Body = io.NopCloser(bytes.NewBuffer(body))

	// Skip validation for empty body
	if len(body) == 0 {
		return nil
	}

	// Check body size
	if len(body) > 10*1024*1024 { // 10MB limit
		return fmt.Errorf("request body too large")
	}

	// Basic JSON validation
	bodyStr := string(body)
	if err := validator.ValidateJSON(bodyStr, len(body)); err != nil {
		return err
	}

	// Parse JSON and validate field values
	var jsonData map[string]interface{}
	if err := json.Unmarshal(body, &jsonData); err != nil {
		// Not valid JSON, but we already validated basic structure
		// This might be form data or other content type
		return validateFormData(validator, bodyStr)
	}

	// Validate JSON field values recursively
	return validateJSONFields(validator, jsonData, "")
}

// validateFormData validates form-encoded data
func validateFormData(validator *Validator, data string) error {
	if ContainsSQLInjection(data) {
		return fmt.Errorf("request body contains potential SQL injection")
	}

	if ContainsXSS(data) {
		return fmt.Errorf("request body contains potential XSS")
	}

	return nil
}

// validateJSONFields recursively validates JSON field values
func validateJSONFields(validator *Validator, data map[string]interface{}, prefix string) error {
	for key, value := range data {
		fieldName := key
		if prefix != "" {
			fieldName = prefix + "." + key
		}

		// Validate field name
		if ContainsSQLInjection(key) || ContainsXSS(key) {
			return fmt.Errorf("invalid field name: %s", fieldName)
		}

		switch v := value.(type) {
		case string:
			if err := validateStringValue(validator, fieldName, v); err != nil {
				return err
			}
		case map[string]interface{}:
			if err := validateJSONFields(validator, v, fieldName); err != nil {
				return err
			}
		case []interface{}:
			for i, item := range v {
				if itemMap, ok := item.(map[string]interface{}); ok {
					arrayFieldName := fmt.Sprintf("%s[%d]", fieldName, i)
					if err := validateJSONFields(validator, itemMap, arrayFieldName); err != nil {
						return err
					}
				} else if itemStr, ok := item.(string); ok {
					arrayFieldName := fmt.Sprintf("%s[%d]", fieldName, i)
					if err := validateStringValue(validator, arrayFieldName, itemStr); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

// validateStringValue validates string values based on field context
func validateStringValue(validator *Validator, fieldName, value string) error {
	// Check for malicious content
	if ContainsSQLInjection(value) {
		return fmt.Errorf("field %s contains potential SQL injection", fieldName)
	}

	if ContainsXSS(value) {
		return fmt.Errorf("field %s contains potential XSS", fieldName)
	}

	// Field-specific validation
	fieldLower := strings.ToLower(fieldName)

	switch {
	case strings.Contains(fieldLower, "email"):
		if err := ValidateEmail(value); err != nil {
			return fmt.Errorf("invalid email in field %s: %v", fieldName, err)
		}
	case strings.Contains(fieldLower, "username"):
		if err := ValidateUsername(value); err != nil {
			return fmt.Errorf("invalid username in field %s: %v", fieldName, err)
		}
	case strings.Contains(fieldLower, "currency"):
		if err := ValidateCurrency(value); err != nil {
			return fmt.Errorf("invalid currency in field %s: %v", fieldName, err)
		}
	case strings.Contains(fieldLower, "amount") || strings.Contains(fieldLower, "price") || strings.Contains(fieldLower, "quantity"):
		if err := ValidateAmount(value, fieldName); err != nil {
			return fmt.Errorf("invalid amount in field %s: %v", fieldName, err)
		}
	case strings.Contains(fieldLower, "iban"):
		if value != "" { // IBAN is often optional
			if err := ValidateIBAN(value); err != nil {
				return fmt.Errorf("invalid IBAN in field %s: %v", fieldName, err)
			}
		}
	case strings.Contains(fieldLower, "bic") || strings.Contains(fieldLower, "swift"):
		if value != "" { // BIC is often optional
			if err := ValidateBIC(value); err != nil {
				return fmt.Errorf("invalid BIC in field %s: %v", fieldName, err)
			}
		}
	case strings.Contains(fieldLower, "routing"):
		if err := ValidateRoutingNumber(value); err != nil {
			return fmt.Errorf("invalid routing number in field %s: %v", fieldName, err)
		}
	case strings.Contains(fieldLower, "account") && (strings.Contains(fieldLower, "number") || strings.Contains(fieldLower, "num")):
		if err := ValidateBankAccount(value); err != nil {
			return fmt.Errorf("invalid account number in field %s: %v", fieldName, err)
		}
	default:
		// General string validation
		if len(value) > 10000 { // 10KB limit for any string field
			return fmt.Errorf("field %s exceeds maximum length", fieldName)
		}
	}

	return nil
}

// validateParam validates individual parameters
func validateParam(validator *Validator, key, value string) error {
	// Check for malicious content
	if ContainsSQLInjection(value) || ContainsXSS(value) {
		return fmt.Errorf("invalid parameter value for %s", key)
	}

	// Parameter-specific validation
	switch strings.ToLower(key) {
	case "id", "userid", "user_id", "orderid", "order_id", "transactionid", "transaction_id":
		// Validate UUID format
		if err := defaultValidator.ValidateUUID(value); err != nil {
			return fmt.Errorf("invalid %s format: %v", key, err)
		}
	case "symbol", "currency":
		if err := ValidateCurrency(value); err != nil {
			return fmt.Errorf("invalid %s: %v", key, err)
		}
	case "limit", "offset", "page", "size":
		// Validate numeric parameters
		if num, err := strconv.Atoi(value); err != nil {
			return fmt.Errorf("invalid %s: must be a number", key)
		} else {
			if num < 0 {
				return fmt.Errorf("invalid %s: must be non-negative", key)
			}
			if key == "limit" && num > 1000 {
				return fmt.Errorf("invalid %s: maximum value is 1000", key)
			}
		}
	case "email":
		if err := ValidateEmail(value); err != nil {
			return fmt.Errorf("invalid email: %v", err)
		}
	default:
		// General validation for any parameter
		if len(value) > 1000 {
			return fmt.Errorf("parameter %s exceeds maximum length", key)
		}
	}

	return nil
}

// RequestValidationMiddleware creates a middleware that validates specific request types
func RequestValidationMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Validate based on endpoint
		path := c.FullPath()
		method := c.Request.Method

		switch {
		case strings.Contains(path, "/fiat/deposit") && method == "POST":
			if err := validateFiatDepositRequest(c); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				c.Abort()
				return
			}
		case strings.Contains(path, "/fiat/withdraw") && method == "POST":
			if err := validateFiatWithdrawRequest(c); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				c.Abort()
				return
			}
		case strings.Contains(path, "/trading/orders") && method == "POST":
			if err := validateOrderRequest(c); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				c.Abort()
				return
			}
		}

		c.Next()
	}
}

// validateFiatDepositRequest validates fiat deposit requests
func validateFiatDepositRequest(c *gin.Context) error {
	var req struct {
		Currency string  `json:"currency" binding:"required"`
		Amount   float64 `json:"amount" binding:"required"`
		Provider string  `json:"provider" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		return fmt.Errorf("invalid request format: %v", err)
	}

	// Validate currency
	if err := ValidateCurrency(req.Currency); err != nil {
		return fmt.Errorf("invalid currency: %v", err)
	}

	// Validate amount
	if req.Amount <= 0 {
		return fmt.Errorf("amount must be positive")
	}
	if req.Amount > 1000000 { // 1M limit
		return fmt.Errorf("amount exceeds maximum limit")
	}

	// Validate provider
	sanitizedProvider := SanitizeInput(req.Provider)
	if sanitizedProvider != req.Provider {
		return fmt.Errorf("invalid provider name")
	}

	return nil
}

// validateFiatWithdrawRequest validates fiat withdrawal requests
func validateFiatWithdrawRequest(c *gin.Context) error {
	var req struct {
		Currency    string                 `json:"currency" binding:"required"`
		Amount      float64                `json:"amount" binding:"required"`
		BankDetails map[string]interface{} `json:"bank_details" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		return fmt.Errorf("invalid request format: %v", err)
	}

	// Validate currency
	if err := ValidateCurrency(req.Currency); err != nil {
		return fmt.Errorf("invalid currency: %v", err)
	}

	// Validate amount
	if req.Amount <= 0 {
		return fmt.Errorf("amount must be positive")
	}
	if req.Amount > 1000000 { // 1M limit
		return fmt.Errorf("amount exceeds maximum limit")
	}

	// Use the validateBankDetails function from fiat service
	// For now, we'll do basic validation here
	if len(req.BankDetails) == 0 {
		return fmt.Errorf("bank details are required")
	}

	return nil
}

// validateOrderRequest validates trading order requests
func validateOrderRequest(c *gin.Context) error {
	var req struct {
		Symbol   string  `json:"symbol" binding:"required"`
		Side     string  `json:"side" binding:"required"`
		Type     string  `json:"type" binding:"required"`
		Quantity float64 `json:"quantity" binding:"required"`
		Price    float64 `json:"price"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		return fmt.Errorf("invalid request format: %v", err)
	}

	// Validate symbol
	if err := ValidateCurrency(req.Symbol); err != nil {
		return fmt.Errorf("invalid symbol: %v", err)
	}

	// Validate side
	validSides := map[string]bool{"buy": true, "sell": true}
	if !validSides[strings.ToLower(req.Side)] {
		return fmt.Errorf("invalid side: must be 'buy' or 'sell'")
	}

	// Validate type
	validTypes := map[string]bool{"market": true, "limit": true, "stop": true, "stop_limit": true}
	if !validTypes[strings.ToLower(req.Type)] {
		return fmt.Errorf("invalid order type")
	}

	// Validate quantity
	if req.Quantity <= 0 {
		return fmt.Errorf("quantity must be positive")
	}
	if req.Quantity > 1000000 { // 1M limit
		return fmt.Errorf("quantity exceeds maximum limit")
	}

	// Validate price for limit orders
	if strings.ToLower(req.Type) == "limit" && req.Price <= 0 {
		return fmt.Errorf("price must be positive for limit orders")
	}

	return nil
}
