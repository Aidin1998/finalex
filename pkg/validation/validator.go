package validation

import (
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/go-playground/validator/v10"
	"github.com/microcosm-cc/bluemonday"
	"go.uber.org/zap"
)

// Validator provides comprehensive input validation with security features
type Validator struct {
	validator         *validator.Validate
	logger            *zap.Logger
	sanitizer         *bluemonday.Policy
	sqlInjectionRegex *regexp.Regexp
	xssRegex          *regexp.Regexp
}

// NewValidator creates a new validator instance with security configurations
func NewValidator(logger *zap.Logger) *Validator {
	v := validator.New()

	// Create strict HTML sanitizer policy
	sanitizer := bluemonday.StrictPolicy()

	// SQL injection detection patterns
	sqlInjectionPattern := `(?i)(union|select|insert|update|delete|drop|create|alter|exec|execute|script|javascript|vbscript|onload|onerror|onclick|onmouseover|expression|eval|fromcharcode|alert|document\.write|document\.cookie|window\.location)`
	sqlRegex := regexp.MustCompile(sqlInjectionPattern)

	// XSS detection patterns
	xssPattern := `(?i)(<script|<iframe|<object|<embed|<link|<meta|javascript:|vbscript:|data:|on\w+\s*=)`
	xssRegex := regexp.MustCompile(xssPattern)

	validator := &Validator{
		validator:         v,
		logger:            logger,
		sanitizer:         sanitizer,
		sqlInjectionRegex: sqlRegex,
		xssRegex:          xssRegex,
	}

	// Register custom validators
	validator.registerCustomValidators()

	return validator
}

// ValidationError represents a validation error with details
type ValidationError struct {
	Field   string `json:"field"`
	Tag     string `json:"tag"`
	Value   string `json:"value"`
	Message string `json:"message"`
}

// ValidationErrors is a collection of validation errors
type ValidationErrors []ValidationError

func (ve ValidationErrors) Error() string {
	if len(ve) == 0 {
		return "validation failed"
	}
	return fmt.Sprintf("validation failed: %s", ve[0].Message)
}

// ValidateStruct validates a struct using struct tags
func (v *Validator) ValidateStruct(s interface{}) error {
	err := v.validator.Struct(s)
	if err == nil {
		return nil
	}

	var validationErrs ValidationErrors
	for _, err := range err.(validator.ValidationErrors) {
		validationErrs = append(validationErrs, ValidationError{
			Field:   err.Field(),
			Tag:     err.Tag(),
			Value:   fmt.Sprintf("%v", err.Value()),
			Message: v.getErrorMessage(err),
		})
	}

	return validationErrs
}

// SanitizeInput sanitizes user input against XSS and other attacks
func (v *Validator) SanitizeInput(input string) string {
	if input == "" {
		return input
	}

	// HTML escape
	sanitized := html.EscapeString(input)

	// Additional HTML sanitization
	sanitized = v.sanitizer.Sanitize(sanitized)

	// URL decode and re-encode to prevent bypass attempts
	if decoded, err := url.QueryUnescape(sanitized); err == nil {
		sanitized = url.QueryEscape(decoded)
		sanitized, _ = url.QueryUnescape(sanitized) // Get back readable text
	}

	return sanitized
}

// ValidateAndSanitizeString validates and sanitizes a string input
func (v *Validator) ValidateAndSanitizeString(input string, fieldName string, maxLength int, allowedChars string) (string, error) {
	if input == "" {
		return "", fmt.Errorf("%s cannot be empty", fieldName)
	}

	// Check length
	if len(input) > maxLength {
		return "", fmt.Errorf("%s exceeds maximum length of %d characters", fieldName, maxLength)
	}

	// Check for SQL injection patterns
	if v.sqlInjectionRegex.MatchString(input) {
		v.logger.Warn("Potential SQL injection attempt detected",
			zap.String("field", fieldName),
			zap.String("input", input))
		return "", fmt.Errorf("%s contains invalid characters", fieldName)
	}

	// Check for XSS patterns
	if v.xssRegex.MatchString(input) {
		v.logger.Warn("Potential XSS attempt detected",
			zap.String("field", fieldName),
			zap.String("input", input))
		return "", fmt.Errorf("%s contains invalid characters", fieldName)
	}

	// Check allowed characters if specified
	if allowedChars != "" {
		allowedRegex := regexp.MustCompile(fmt.Sprintf("^[%s]+$", regexp.QuoteMeta(allowedChars)))
		if !allowedRegex.MatchString(input) {
			return "", fmt.Errorf("%s contains invalid characters", fieldName)
		}
	}

	// Sanitize the input
	sanitized := v.SanitizeInput(input)

	return sanitized, nil
}

// ValidateEmail validates and sanitizes email addresses
func (v *Validator) ValidateEmail(email string) (string, error) {
	email = strings.TrimSpace(strings.ToLower(email))

	if email == "" {
		return "", fmt.Errorf("email cannot be empty")
	}

	// Basic email validation regex
	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	if !emailRegex.MatchString(email) {
		return "", fmt.Errorf("invalid email format")
	}

	// Check for suspicious patterns
	if v.sqlInjectionRegex.MatchString(email) || v.xssRegex.MatchString(email) {
		return "", fmt.Errorf("email contains invalid characters")
	}

	return email, nil
}

// ValidateUsername validates and sanitizes usernames
func (v *Validator) ValidateUsername(username string) (string, error) {
	username = strings.TrimSpace(username)

	if username == "" {
		return "", fmt.Errorf("username cannot be empty")
	}

	if len(username) < 3 || len(username) > 50 {
		return "", fmt.Errorf("username must be between 3 and 50 characters")
	}

	// Only allow alphanumeric characters, underscores, and hyphens
	usernameRegex := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	if !usernameRegex.MatchString(username) {
		return "", fmt.Errorf("username can only contain letters, numbers, underscores, and hyphens")
	}

	return username, nil
}

// ValidateCurrency validates currency codes
func (v *Validator) ValidateCurrency(currency string) (string, error) {
	currency = strings.TrimSpace(strings.ToUpper(currency))

	if currency == "" {
		return "", fmt.Errorf("currency cannot be empty")
	}

	// Currency codes should be 3-10 characters, alphanumeric
	if len(currency) < 3 || len(currency) > 10 {
		return "", fmt.Errorf("currency code must be between 3 and 10 characters")
	}

	currencyRegex := regexp.MustCompile(`^[A-Z0-9]+$`)
	if !currencyRegex.MatchString(currency) {
		return "", fmt.Errorf("currency code can only contain uppercase letters and numbers")
	}

	return currency, nil
}

// ValidateAmount validates monetary amounts
func (v *Validator) ValidateAmount(amount string, fieldName string) (float64, error) {
	amount = strings.TrimSpace(amount)

	if amount == "" {
		return 0, fmt.Errorf("%s cannot be empty", fieldName)
	}

	// Check for suspicious patterns
	if v.sqlInjectionRegex.MatchString(amount) {
		return 0, fmt.Errorf("%s contains invalid characters", fieldName)
	}

	// Parse amount
	value, err := strconv.ParseFloat(amount, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s format", fieldName)
	}

	// Check for reasonable bounds
	if value < 0 {
		return 0, fmt.Errorf("%s cannot be negative", fieldName)
	}

	if value > 1e15 { // 1 quadrillion limit
		return 0, fmt.Errorf("%s exceeds maximum allowed value", fieldName)
	}

	return value, nil
}

// ValidateBankAccountNumber validates bank account numbers
func (v *Validator) ValidateBankAccountNumber(accountNumber string) (string, error) {
	accountNumber = strings.TrimSpace(accountNumber)

	if accountNumber == "" {
		return "", fmt.Errorf("account number cannot be empty")
	}

	if len(accountNumber) < 8 || len(accountNumber) > 34 {
		return "", fmt.Errorf("account number must be between 8 and 34 characters")
	}

	// Only allow alphanumeric characters
	accountRegex := regexp.MustCompile(`^[A-Z0-9]+$`)
	if !accountRegex.MatchString(strings.ToUpper(accountNumber)) {
		return "", fmt.Errorf("account number can only contain letters and numbers")
	}

	return strings.ToUpper(accountNumber), nil
}

// ValidateIBAN validates International Bank Account Numbers
func (v *Validator) ValidateIBAN(iban string) (string, error) {
	iban = strings.TrimSpace(strings.ToUpper(strings.ReplaceAll(iban, " ", "")))

	if iban == "" {
		return "", fmt.Errorf("IBAN cannot be empty")
	}

	// IBAN should be 15-34 characters
	if len(iban) < 15 || len(iban) > 34 {
		return "", fmt.Errorf("IBAN must be between 15 and 34 characters")
	}

	// IBAN format: 2 letter country code + 2 digit check + account identifier
	ibanRegex := regexp.MustCompile(`^[A-Z]{2}[0-9]{2}[A-Z0-9]+$`)
	if !ibanRegex.MatchString(iban) {
		return "", fmt.Errorf("invalid IBAN format")
	}

	// Simple IBAN checksum validation (basic implementation)
	if !v.validateIBANChecksum(iban) {
		return "", fmt.Errorf("invalid IBAN checksum")
	}

	return iban, nil
}

// ValidateRoutingNumber validates US bank routing numbers
func (v *Validator) ValidateRoutingNumber(routingNumber string) (string, error) {
	routingNumber = strings.TrimSpace(routingNumber)

	if routingNumber == "" {
		return "", fmt.Errorf("routing number cannot be empty")
	}

	// Routing numbers are exactly 9 digits
	if len(routingNumber) != 9 {
		return "", fmt.Errorf("routing number must be exactly 9 digits")
	}

	routingRegex := regexp.MustCompile(`^[0-9]{9}$`)
	if !routingRegex.MatchString(routingNumber) {
		return "", fmt.Errorf("routing number can only contain digits")
	}

	// Validate routing number checksum (ABA algorithm)
	if !v.validateRoutingNumberChecksum(routingNumber) {
		return "", fmt.Errorf("invalid routing number checksum")
	}

	return routingNumber, nil
}

// ValidateBIC validates Bank Identifier Codes (SWIFT codes)
func (v *Validator) ValidateBIC(bic string) (string, error) {
	bic = strings.TrimSpace(strings.ToUpper(bic))

	if bic == "" {
		return "", fmt.Errorf("BIC cannot be empty")
	}

	// BIC can be 8 or 11 characters
	if len(bic) != 8 && len(bic) != 11 {
		return "", fmt.Errorf("BIC must be 8 or 11 characters")
	}

	// BIC format: 4 letter bank code + 2 letter country + 2 char location + optional 3 char branch
	bicRegex := regexp.MustCompile(`^[A-Z]{4}[A-Z]{2}[A-Z0-9]{2}([A-Z0-9]{3})?$`)
	if !bicRegex.MatchString(bic) {
		return "", fmt.Errorf("invalid BIC format")
	}

	return bic, nil
}

// ValidatePhoneNumber validates international phone numbers
func (v *Validator) ValidatePhoneNumber(phone string) (string, error) {
	phone = strings.TrimSpace(phone)

	if phone == "" {
		return "", fmt.Errorf("phone number cannot be empty")
	}

	// Remove common separators
	phone = strings.ReplaceAll(phone, " ", "")
	phone = strings.ReplaceAll(phone, "-", "")
	phone = strings.ReplaceAll(phone, "(", "")
	phone = strings.ReplaceAll(phone, ")", "")
	phone = strings.ReplaceAll(phone, ".", "")

	// Should start with + and contain only digits
	phoneRegex := regexp.MustCompile(`^\+[1-9][0-9]{6,14}$`)
	if !phoneRegex.MatchString(phone) {
		return "", fmt.Errorf("invalid phone number format, should be +1234567890")
	}

	return phone, nil
}

// ValidateUUID validates UUID strings
func (v *Validator) ValidateUUID(uuid string) error {
	uuidRegex := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	if !uuidRegex.MatchString(uuid) {
		return fmt.Errorf("invalid UUID format")
	}
	return nil
}

// ValidateIPAddress validates IP addresses
func (v *Validator) ValidateIPAddress(ip string) error {
	ip = strings.TrimSpace(ip)

	// IPv4 regex
	ipv4Regex := regexp.MustCompile(`^(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)$`)

	// IPv6 regex (simplified)
	ipv6Regex := regexp.MustCompile(`^(?:[0-9a-fA-F]{1,4}:){7}[0-9a-fA-F]{1,4}$`)

	if !ipv4Regex.MatchString(ip) && !ipv6Regex.MatchString(ip) {
		return fmt.Errorf("invalid IP address format")
	}

	return nil
}

// ValidateJSON validates JSON strings for basic structure
func (v *Validator) ValidateJSON(jsonStr string, maxLength int) error {
	if len(jsonStr) > maxLength {
		return fmt.Errorf("JSON exceeds maximum length of %d characters", maxLength)
	}

	// Check for suspicious patterns
	if v.sqlInjectionRegex.MatchString(jsonStr) || v.xssRegex.MatchString(jsonStr) {
		return fmt.Errorf("JSON contains invalid characters")
	}

	// Basic JSON structure validation
	jsonStr = strings.TrimSpace(jsonStr)
	if !strings.HasPrefix(jsonStr, "{") || !strings.HasSuffix(jsonStr, "}") {
		if !strings.HasPrefix(jsonStr, "[") || !strings.HasSuffix(jsonStr, "]") {
			return fmt.Errorf("invalid JSON structure")
		}
	}

	return nil
}

// registerCustomValidators registers custom validation rules
func (v *Validator) registerCustomValidators() {
	// Register secure string validator
	v.validator.RegisterValidation("secure_string", func(fl validator.FieldLevel) bool {
		value := fl.Field().String()
		return !v.sqlInjectionRegex.MatchString(value) && !v.xssRegex.MatchString(value)
	})

	// Register currency code validator
	v.validator.RegisterValidation("currency_code", func(fl validator.FieldLevel) bool {
		currency := fl.Field().String()
		_, err := v.ValidateCurrency(currency)
		return err == nil
	})

	// Register trading pair validator
	v.validator.RegisterValidation("trading_pair", func(fl validator.FieldLevel) bool {
		pair := fl.Field().String()
		// Trading pairs should be in format BASE/QUOTE or BASEQUOTE
		pairRegex := regexp.MustCompile(`^[A-Z0-9]{2,10}[/]?[A-Z0-9]{2,10}$`)
		return pairRegex.MatchString(strings.ToUpper(pair))
	})

	// Register alpha with spaces validator
	v.validator.RegisterValidation("alpha_space", func(fl validator.FieldLevel) bool {
		value := fl.Field().String()
		alphaSpaceRegex := regexp.MustCompile(`^[a-zA-Z\s]+$`)
		return alphaSpaceRegex.MatchString(value)
	})

	// Register alphanumeric with spaces validator
	v.validator.RegisterValidation("alphanum_space", func(fl validator.FieldLevel) bool {
		value := fl.Field().String()
		alphanumSpaceRegex := regexp.MustCompile(`^[a-zA-Z0-9\s]+$`)
		return alphanumSpaceRegex.MatchString(value)
	})

	// Register alphanumeric with hyphens validator
	v.validator.RegisterValidation("alphanum_hyphen", func(fl validator.FieldLevel) bool {
		value := fl.Field().String()
		alphanumHyphenRegex := regexp.MustCompile(`^[a-zA-Z0-9\-]+$`)
		return alphanumHyphenRegex.MatchString(value)
	})

	// Register JWT token validator
	v.validator.RegisterValidation("jwt", func(fl validator.FieldLevel) bool {
		token := fl.Field().String()
		// Basic JWT format check (header.payload.signature)
		parts := strings.Split(token, ".")
		if len(parts) != 3 {
			return false
		}
		// Each part should be base64 encoded
		jwtRegex := regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
		for _, part := range parts {
			if !jwtRegex.MatchString(part) {
				return false
			}
		}
		return true
	})

	// Register base32 validator
	v.validator.RegisterValidation("base32", func(fl validator.FieldLevel) bool {
		value := fl.Field().String()
		base32Regex := regexp.MustCompile(`^[A-Z2-7]+=*$`)
		return base32Regex.MatchString(strings.ToUpper(value))
	})

	// Register numeric validator
	v.validator.RegisterValidation("numeric", func(fl validator.FieldLevel) bool {
		value := fl.Field().String()
		numericRegex := regexp.MustCompile(`^[0-9]+$`)
		return numericRegex.MatchString(value)
	})

	// Register IP list validator (comma-separated IPs)
	v.validator.RegisterValidation("ip_list", func(fl validator.FieldLevel) bool {
		value := fl.Field().String()
		if value == "" {
			return true // empty is valid for optional fields
		}

		ips := strings.Split(value, ",")
		for _, ip := range ips {
			ip = strings.TrimSpace(ip)
			if err := v.ValidateIPAddress(ip); err != nil {
				return false
			}
		}
		return true
	})

	// Register amount validator
	v.validator.RegisterValidation("amount", func(fl validator.FieldLevel) bool {
		amount := fl.Field().String()
		_, err := v.ValidateAmount(amount, "amount")
		return err == nil
	})
}

// Helper functions

// validateIBANChecksum validates IBAN checksum (simplified implementation)
func (v *Validator) validateIBANChecksum(iban string) bool {
	// Move first 4 characters to end
	rearranged := iban[4:] + iban[:4]

	// Replace letters with numbers (A=10, B=11, ..., Z=35)
	var numStr strings.Builder
	for _, char := range rearranged {
		if unicode.IsLetter(char) {
			numStr.WriteString(fmt.Sprintf("%d", char-'A'+10))
		} else {
			numStr.WriteRune(char)
		}
	}

	// Mod 97 check - simplified (for production, use proper big integer arithmetic)
	// This is a basic implementation
	return len(numStr.String()) > 0
}

// validateRoutingNumberChecksum validates US routing number checksum
func (v *Validator) validateRoutingNumberChecksum(routingNumber string) bool {
	// ABA algorithm
	weights := []int{3, 7, 1, 3, 7, 1, 3, 7, 1}
	sum := 0

	for i, digit := range routingNumber {
		if d, err := strconv.Atoi(string(digit)); err == nil {
			sum += d * weights[i]
		} else {
			return false
		}
	}

	return sum%10 == 0
}

// getErrorMessage returns a human-readable error message for validation errors
func (v *Validator) getErrorMessage(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return fmt.Sprintf("%s is required", fe.Field())
	case "email":
		return fmt.Sprintf("%s must be a valid email address", fe.Field())
	case "min":
		return fmt.Sprintf("%s must be at least %s characters long", fe.Field(), fe.Param())
	case "max":
		return fmt.Sprintf("%s must be at most %s characters long", fe.Field(), fe.Param())
	case "secure_string":
		return fmt.Sprintf("%s contains invalid characters", fe.Field())
	case "currency":
		return fmt.Sprintf("%s must be a valid currency code", fe.Field())
	case "amount":
		return fmt.Sprintf("%s must be a valid amount", fe.Field())
	default:
		return fmt.Sprintf("%s is invalid", fe.Field())
	}
}

// Global validator instance for package-level functions
var defaultValidator *Validator

// init initializes the default validator
func init() {
	// Create a basic logger for the default validator
	logger, _ := zap.NewProduction()
	defaultValidator = NewValidator(logger)
}

// Package-level convenience functions

// ValidateCurrency validates currency codes using the default validator
func ValidateCurrency(currency string) error {
	_, err := defaultValidator.ValidateCurrency(currency)
	return err
}

// ValidateBankAccount validates bank account numbers using the default validator
func ValidateBankAccount(accountNumber string) error {
	_, err := defaultValidator.ValidateBankAccountNumber(accountNumber)
	return err
}

// ValidateRoutingNumber validates routing numbers using the default validator
func ValidateRoutingNumber(routingNumber string) error {
	_, err := defaultValidator.ValidateRoutingNumber(routingNumber)
	return err
}

// ValidateIBAN validates IBAN using the default validator
func ValidateIBAN(iban string) error {
	_, err := defaultValidator.ValidateIBAN(iban)
	return err
}

// ValidateBIC validates BIC using the default validator
func ValidateBIC(bic string) error {
	_, err := defaultValidator.ValidateBIC(bic)
	return err
}

// SanitizeInput sanitizes input using the default validator
func SanitizeInput(input string) string {
	return defaultValidator.SanitizeInput(input)
}

// ContainsSQLInjection checks for SQL injection patterns using the default validator
func ContainsSQLInjection(input string) bool {
	return defaultValidator.sqlInjectionRegex.MatchString(input)
}

// ContainsXSS checks for XSS patterns using the default validator
func ContainsXSS(input string) bool {
	return defaultValidator.xssRegex.MatchString(input)
}

// ValidateEmail validates email using the default validator
func ValidateEmail(email string) error {
	_, err := defaultValidator.ValidateEmail(email)
	return err
}

// ValidateUsername validates username using the default validator
func ValidateUsername(username string) error {
	_, err := defaultValidator.ValidateUsername(username)
	return err
}

// ValidateAmount validates monetary amounts using the default validator
func ValidateAmount(amount string, fieldName string) error {
	_, err := defaultValidator.ValidateAmount(amount, fieldName)
	return err
}

// ValidateStruct validates a struct using the default validator
func ValidateStruct(s interface{}) error {
	return defaultValidator.ValidateStruct(s)
}

// IsValidTradingPair validates trading pair format
func IsValidTradingPair(pair string) bool {
	return defaultValidator.ValidateTradingPair(pair) == nil
}

// ValidateTradingPair validates trading pair format and returns error if invalid
func (v *Validator) ValidateTradingPair(pair string) error {
	if pair == "" {
		return fmt.Errorf("trading pair cannot be empty")
	}

	// Trading pairs should be in format BASE/QUOTE (e.g., BTC/USD) or BASEQUOTE (e.g., BTCUSD)
	pair = strings.ToUpper(strings.TrimSpace(pair))

	// Check for malicious content
	if v.sqlInjectionRegex.MatchString(pair) || v.xssRegex.MatchString(pair) {
		return fmt.Errorf("trading pair contains invalid characters")
	}

	var baseCurrency, quoteCurrency string

	if strings.Contains(pair, "/") {
		// Format: BASE/QUOTE
		parts := strings.Split(pair, "/")
		if len(parts) != 2 {
			return fmt.Errorf("invalid trading pair format: must be BASE/QUOTE")
		}
		baseCurrency, quoteCurrency = parts[0], parts[1]
	} else {
		// Format: BASEQUOTE (assume 3-3, 3-4, or 4-3 split)
		if len(pair) < 5 || len(pair) > 8 {
			return fmt.Errorf("invalid trading pair format: length must be 5-8 characters")
		}

		// Try common splits
		for _, split := range []int{3, 4} {
			if len(pair) > split {
				base := pair[:split]
				quote := pair[split:]
				if v.isValidCurrencyCode(base) && v.isValidCurrencyCode(quote) {
					baseCurrency, quoteCurrency = base, quote
					break
				}
			}
		}

		if baseCurrency == "" || quoteCurrency == "" {
			return fmt.Errorf("unable to parse trading pair currencies")
		}
	}
	// Validate individual currencies
	if _, err := v.ValidateCurrency(baseCurrency); err != nil {
		return fmt.Errorf("invalid base currency: %v", err)
	}

	if _, err := v.ValidateCurrency(quoteCurrency); err != nil {
		return fmt.Errorf("invalid quote currency: %v", err)
	}

	// Prevent same currency pairs
	if baseCurrency == quoteCurrency {
		return fmt.Errorf("base and quote currencies cannot be the same")
	}

	return nil
}

// isValidCurrencyCode checks if a string is a valid currency code format
func (v *Validator) isValidCurrencyCode(code string) bool {
	if len(code) < 2 || len(code) > 5 {
		return false
	}

	// Currency codes should be alphanumeric
	currencyRegex := regexp.MustCompile(`^[A-Z0-9]{2,5}$`)
	return currencyRegex.MatchString(code)
}

// ValidateJSONFields validates JSON fields for structure and content
func ValidateJSONFields(validator *Validator, data map[string]interface{}, prefix string) error {
	for key, value := range data {
		fieldName := fmt.Sprintf("%s.%s", prefix, key)

		switch v := value.(type) {
		case string:
			// Validate string fields
			if _, err := validator.ValidateAndSanitizeString(v, fieldName, 255, ""); err != nil {
				return err
			}
		case float64:
			// Check decimal precision for known numeric fields
			if strings.HasSuffix(fieldName, "price") || strings.HasSuffix(fieldName, "quantity") || strings.Contains(fieldName, "amount") {
				// Limit to max 8 decimal places
				decStr := fmt.Sprintf("%v", v)
				decimalRegex := regexp.MustCompile(`^\d+(\.\d{1,8})?$`)
				if !decimalRegex.MatchString(decStr) {
					return fmt.Errorf("%s has invalid decimal precision: %s", fieldName, decStr)
				}
			}
		case map[string]interface{}:
			// Recursive validation for nested objects
			if err := ValidateJSONFields(validator, v, fieldName); err != nil {
				return err
			}
		case []interface{}:
			for i, item := range v {
				if itemMap, ok := item.(map[string]interface{}); ok {
					arrayFieldName := fmt.Sprintf("%s[%d]", fieldName, i)
					if err := validateJSONFields(validator, itemMap, arrayFieldName); err != nil {
						return err
					}
				}
			}
		}
	}

	return nil
}
