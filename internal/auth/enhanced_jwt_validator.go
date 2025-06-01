package auth

import (
	"context"
	"crypto/subtle"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// EnhancedJWTValidator provides comprehensive JWT token validation with enhanced security
type EnhancedJWTValidator struct {
	logger              *zap.Logger
	db                  *gorm.DB
	jwtSecret           []byte
	refreshSecret       []byte
	issuer              string
	config              *JWTValidationConfig
	rateLimiter         RateLimiter
	securityAuditLogger *SecurityAuditLogger
}

// JWTValidationConfig defines configuration for enhanced JWT validation
type JWTValidationConfig struct {
	// Token expiration validation
	StrictExpirationCheck bool          `json:"strict_expiration_check"`
	ClockSkewTolerance    time.Duration `json:"clock_skew_tolerance"`
	MaxTokenAge           time.Duration `json:"max_token_age"`

	// Security checks
	ValidateIssuer       bool `json:"validate_issuer"`
	ValidateAudience     bool `json:"validate_audience"`
	ValidateNotBefore    bool `json:"validate_not_before"`
	RequireSecureSigning bool `json:"require_secure_signing"`

	// Blacklist and revocation checks
	CheckTokenBlacklist  bool `json:"check_token_blacklist"`
	CheckSessionValidity bool `json:"check_session_validity"`
	EnableJTI            bool `json:"enable_jti"`

	// Rate limiting
	EnableRateLimiting   bool          `json:"enable_rate_limiting"`
	ValidationRateLimit  int           `json:"validation_rate_limit"`
	ValidationRateWindow time.Duration `json:"validation_rate_window"`

	// Audit and logging
	EnableSecurityAuditing   bool `json:"enable_security_auditing"`
	LogSuccessfulValidations bool `json:"log_successful_validations"`
	LogFailedValidations     bool `json:"log_failed_validations"`

	// Performance settings
	CacheValidTokens bool          `json:"cache_valid_tokens"`
	TokenCacheTTL    time.Duration `json:"token_cache_ttl"`
}

// DefaultJWTValidationConfig returns a secure default configuration
func DefaultJWTValidationConfig() *JWTValidationConfig {
	return &JWTValidationConfig{
		StrictExpirationCheck:    true,
		ClockSkewTolerance:       30 * time.Second,
		MaxTokenAge:              24 * time.Hour,
		ValidateIssuer:           true,
		ValidateAudience:         false, // Set to true if audience validation is needed
		ValidateNotBefore:        true,
		RequireSecureSigning:     true,
		CheckTokenBlacklist:      true,
		CheckSessionValidity:     true,
		EnableJTI:                true,
		EnableRateLimiting:       true,
		ValidationRateLimit:      1000,
		ValidationRateWindow:     time.Minute,
		EnableSecurityAuditing:   true,
		LogSuccessfulValidations: false, // Disable to reduce log volume
		LogFailedValidations:     true,
		CacheValidTokens:         false, // Disable caching for maximum security
		TokenCacheTTL:            5 * time.Minute,
	}
}

// SecurityAuditLogger handles security-related audit logging
type SecurityAuditLogger struct {
	logger *zap.Logger
}

// TokenValidationResult contains comprehensive validation results
type TokenValidationResult struct {
	Valid             bool              `json:"valid"`
	Claims            *TokenClaims      `json:"claims,omitempty"`
	ValidationErrors  []ValidationError `json:"validation_errors,omitempty"`
	SecurityFlags     []SecurityFlag    `json:"security_flags,omitempty"`
	ValidationLatency time.Duration     `json:"validation_latency"`
	TokenAge          time.Duration     `json:"token_age"`
	RemainingLifetime time.Duration     `json:"remaining_lifetime"`
}

// ValidationError represents a token validation error
type ValidationError struct {
	Code      string    `json:"code"`
	Message   string    `json:"message"`
	Severity  string    `json:"severity"` // LOW, MEDIUM, HIGH, CRITICAL
	Timestamp time.Time `json:"timestamp"`
}

// SecurityFlag represents security-related observations
type SecurityFlag struct {
	Type        string    `json:"type"`
	Description string    `json:"description"`
	Risk        string    `json:"risk"` // LOW, MEDIUM, HIGH
	Timestamp   time.Time `json:"timestamp"`
}

// NewEnhancedJWTValidator creates a new enhanced JWT validator
func NewEnhancedJWTValidator(
	logger *zap.Logger,
	db *gorm.DB,
	jwtSecret []byte,
	refreshSecret []byte,
	issuer string,
	config *JWTValidationConfig,
	rateLimiter RateLimiter,
) *EnhancedJWTValidator {
	if config == nil {
		config = DefaultJWTValidationConfig()
	}

	return &EnhancedJWTValidator{
		logger:              logger,
		db:                  db,
		jwtSecret:           jwtSecret,
		refreshSecret:       refreshSecret,
		issuer:              issuer,
		config:              config,
		rateLimiter:         rateLimiter,
		securityAuditLogger: &SecurityAuditLogger{logger: logger},
	}
}

// ValidateTokenComprehensive performs comprehensive JWT token validation
func (ejv *EnhancedJWTValidator) ValidateTokenComprehensive(ctx context.Context, tokenString string, clientIP string) (*TokenValidationResult, error) {
	startTime := time.Now()
	result := &TokenValidationResult{
		Valid:             false,
		ValidationErrors:  make([]ValidationError, 0),
		SecurityFlags:     make([]SecurityFlag, 0),
		ValidationLatency: 0,
	}

	// Apply rate limiting if enabled
	if ejv.config.EnableRateLimiting && ejv.rateLimiter != nil {
		rateLimitKey := fmt.Sprintf("jwt_validation:%s", clientIP)
		allowed, err := ejv.rateLimiter.Allow(ctx, rateLimitKey, ejv.config.ValidationRateLimit, ejv.config.ValidationRateWindow)
		if err != nil {
			ejv.logger.Error("Rate limiter error during JWT validation", zap.Error(err))
		} else if !allowed {
			result.ValidationErrors = append(result.ValidationErrors, ValidationError{
				Code:      "RATE_LIMIT_EXCEEDED",
				Message:   "Too many token validation requests",
				Severity:  "HIGH",
				Timestamp: time.Now(),
			})
			if ejv.config.EnableSecurityAuditing {
				ejv.securityAuditLogger.LogRateLimitExceeded(clientIP, rateLimitKey)
			}
			result.ValidationLatency = time.Since(startTime)
			return result, fmt.Errorf("rate limit exceeded for token validation")
		}
	}

	// Basic token format validation
	if err := ejv.validateTokenFormat(tokenString); err != nil {
		result.ValidationErrors = append(result.ValidationErrors, ValidationError{
			Code:      "INVALID_TOKEN_FORMAT",
			Message:   err.Error(),
			Severity:  "HIGH",
			Timestamp: time.Now(),
		})
		result.ValidationLatency = time.Since(startTime)
		return result, err
	}

	// Remove Bearer prefix if present
	tokenString = strings.TrimPrefix(tokenString, "Bearer ")

	// Check token blacklist
	if ejv.config.CheckTokenBlacklist {
		if blacklisted, err := ejv.isTokenBlacklisted(ctx, tokenString); err != nil {
			ejv.logger.Error("Error checking token blacklist", zap.Error(err))
		} else if blacklisted {
			result.ValidationErrors = append(result.ValidationErrors, ValidationError{
				Code:      "TOKEN_REVOKED",
				Message:   "Token has been revoked",
				Severity:  "CRITICAL",
				Timestamp: time.Now(),
			})
			if ejv.config.EnableSecurityAuditing {
				ejv.securityAuditLogger.LogBlacklistedTokenAttempt(tokenString, clientIP)
			}
			result.ValidationLatency = time.Since(startTime)
			return result, fmt.Errorf("token has been revoked")
		}
	}

	// Parse and validate JWT token
	token, err := jwt.ParseWithClaims(tokenString, &TokenClaims{}, ejv.getSigningKey)
	if err != nil {
		severity := "HIGH"
		if strings.Contains(err.Error(), "token is expired") {
			severity = "MEDIUM"
		}
		result.ValidationErrors = append(result.ValidationErrors, ValidationError{
			Code:      "TOKEN_PARSE_ERROR",
			Message:   fmt.Sprintf("Failed to parse token: %v", err),
			Severity:  severity,
			Timestamp: time.Now(),
		})
		if ejv.config.EnableSecurityAuditing {
			ejv.securityAuditLogger.LogTokenParseError(tokenString, clientIP, err)
		}
		result.ValidationLatency = time.Since(startTime)
		return result, fmt.Errorf("failed to parse token: %w", err)
	}

	// Validate token structure
	if !token.Valid {
		result.ValidationErrors = append(result.ValidationErrors, ValidationError{
			Code:      "INVALID_TOKEN",
			Message:   "Token is not valid",
			Severity:  "HIGH",
			Timestamp: time.Now(),
		})
		result.ValidationLatency = time.Since(startTime)
		return result, fmt.Errorf("invalid token")
	}

	// Extract and validate claims
	claims, ok := token.Claims.(*TokenClaims)
	if !ok {
		result.ValidationErrors = append(result.ValidationErrors, ValidationError{
			Code:      "INVALID_CLAIMS",
			Message:   "Invalid token claims structure",
			Severity:  "HIGH",
			Timestamp: time.Now(),
		})
		result.ValidationLatency = time.Since(startTime)
		return result, fmt.Errorf("invalid token claims")
	}

	// Comprehensive claims validation
	if err := ejv.validateClaimsComprehensive(ctx, claims, result); err != nil {
		result.ValidationLatency = time.Since(startTime)
		return result, err
	}

	// Validate session if session ID is present
	if ejv.config.CheckSessionValidity && claims.SessionID != uuid.Nil {
		if err := ejv.validateSession(ctx, claims.SessionID, result); err != nil {
			result.ValidationLatency = time.Since(startTime)
			return result, err
		}
	}

	// Calculate token metrics
	result.TokenAge = time.Since(claims.IssuedAt.Time)
	if claims.ExpiresAt != nil {
		result.RemainingLifetime = time.Until(claims.ExpiresAt.Time)
	}

	// Check for security flags
	ejv.checkSecurityFlags(claims, result)

	// Token validation successful
	result.Valid = true
	result.Claims = claims
	result.ValidationLatency = time.Since(startTime)

	// Log successful validation if enabled
	if ejv.config.LogSuccessfulValidations && ejv.config.EnableSecurityAuditing {
		ejv.securityAuditLogger.LogSuccessfulValidation(claims.UserID, clientIP, result.ValidationLatency)
	}

	return result, nil
}

// validateTokenFormat performs basic token format validation
func (ejv *EnhancedJWTValidator) validateTokenFormat(tokenString string) error {
	// Remove Bearer prefix for validation
	token := strings.TrimPrefix(tokenString, "Bearer ")

	if token == "" {
		return fmt.Errorf("empty token")
	}

	if len(token) > 4096 { // Reasonable token size limit
		return fmt.Errorf("token too long")
	}

	// Basic JWT format check (three parts separated by dots)
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return fmt.Errorf("invalid JWT format: expected 3 parts, got %d", len(parts))
	}

	// Check for suspicious patterns
	if strings.Contains(token, "null") || strings.Contains(token, "undefined") {
		return fmt.Errorf("token contains suspicious patterns")
	}

	return nil
}

// getSigningKey returns the appropriate signing key and validates signing method
func (ejv *EnhancedJWTValidator) getSigningKey(token *jwt.Token) (interface{}, error) {
	// Validate signing method
	method, ok := token.Method.(*jwt.SigningMethodHMAC)
	if !ok {
		return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
	}

	// Require secure signing method
	if ejv.config.RequireSecureSigning {
		if method != jwt.SigningMethodHS256 && method != jwt.SigningMethodHS384 && method != jwt.SigningMethodHS512 {
			return nil, fmt.Errorf("insecure signing method: %v", method.Alg())
		}
	}

	// Return appropriate secret based on token type
	if claims, ok := token.Claims.(*TokenClaims); ok && claims.TokenType == "refresh" {
		return ejv.refreshSecret, nil
	}

	return ejv.jwtSecret, nil
}

// validateClaimsComprehensive performs comprehensive claims validation
func (ejv *EnhancedJWTValidator) validateClaimsComprehensive(ctx context.Context, claims *TokenClaims, result *TokenValidationResult) error {
	now := time.Now()

	// Validate issuer
	if ejv.config.ValidateIssuer && claims.Issuer != ejv.issuer {
		result.ValidationErrors = append(result.ValidationErrors, ValidationError{
			Code:      "INVALID_ISSUER",
			Message:   fmt.Sprintf("Invalid issuer: expected %s, got %s", ejv.issuer, claims.Issuer),
			Severity:  "HIGH",
			Timestamp: now,
		})
		return fmt.Errorf("invalid issuer")
	}

	// Strict expiration validation
	if ejv.config.StrictExpirationCheck && claims.ExpiresAt != nil {
		expirationTime := claims.ExpiresAt.Time
		if now.After(expirationTime.Add(ejv.config.ClockSkewTolerance)) {
			result.ValidationErrors = append(result.ValidationErrors, ValidationError{
				Code:      "TOKEN_EXPIRED",
				Message:   fmt.Sprintf("Token expired at %v (current time: %v)", expirationTime, now),
				Severity:  "MEDIUM",
				Timestamp: now,
			})
			return fmt.Errorf("token expired")
		}

		// Check if token is expiring soon (within 5 minutes)
		if time.Until(expirationTime) < 5*time.Minute {
			result.SecurityFlags = append(result.SecurityFlags, SecurityFlag{
				Type:        "TOKEN_EXPIRING_SOON",
				Description: fmt.Sprintf("Token expires in %v", time.Until(expirationTime)),
				Risk:        "LOW",
				Timestamp:   now,
			})
		}
	}

	// Validate not-before time
	if ejv.config.ValidateNotBefore && claims.NotBefore != nil {
		notBeforeTime := claims.NotBefore.Time
		if now.Before(notBeforeTime.Add(-ejv.config.ClockSkewTolerance)) {
			result.ValidationErrors = append(result.ValidationErrors, ValidationError{
				Code:      "TOKEN_NOT_VALID_YET",
				Message:   fmt.Sprintf("Token not valid until %v (current time: %v)", notBeforeTime, now),
				Severity:  "HIGH",
				Timestamp: now,
			})
			return fmt.Errorf("token not valid yet")
		}
	}

	// Validate token age
	if claims.IssuedAt != nil {
		tokenAge := now.Sub(claims.IssuedAt.Time)
		if tokenAge > ejv.config.MaxTokenAge {
			result.ValidationErrors = append(result.ValidationErrors, ValidationError{
				Code:      "TOKEN_TOO_OLD",
				Message:   fmt.Sprintf("Token is too old: %v (max age: %v)", tokenAge, ejv.config.MaxTokenAge),
				Severity:  "MEDIUM",
				Timestamp: now,
			})
			return fmt.Errorf("token too old")
		}
	}

	// Validate essential claims
	if claims.UserID == uuid.Nil {
		result.ValidationErrors = append(result.ValidationErrors, ValidationError{
			Code:      "MISSING_USER_ID",
			Message:   "Token missing user ID",
			Severity:  "CRITICAL",
			Timestamp: now,
		})
		return fmt.Errorf("missing user ID in token")
	}

	if claims.TokenType == "" {
		result.ValidationErrors = append(result.ValidationErrors, ValidationError{
			Code:      "MISSING_TOKEN_TYPE",
			Message:   "Token missing type specification",
			Severity:  "HIGH",
			Timestamp: now,
		})
		return fmt.Errorf("missing token type")
	}

	return nil
}

// isTokenBlacklisted checks if a token is blacklisted
func (ejv *EnhancedJWTValidator) isTokenBlacklisted(ctx context.Context, tokenString string) (bool, error) {
	tokenHash := hashToken(tokenString)
	var blacklistedToken BlacklistedToken
	err := ejv.db.WithContext(ctx).Where("token_hash = ? AND expires_at > ?", tokenHash, time.Now()).First(&blacklistedToken).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// validateSession validates the session associated with the token
func (ejv *EnhancedJWTValidator) validateSession(ctx context.Context, sessionID uuid.UUID, result *TokenValidationResult) error {
	var session Session
	err := ejv.db.WithContext(ctx).Where("id = ?", sessionID).First(&session).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			result.ValidationErrors = append(result.ValidationErrors, ValidationError{
				Code:      "SESSION_NOT_FOUND",
				Message:   "Session not found",
				Severity:  "HIGH",
				Timestamp: time.Now(),
			})
			return fmt.Errorf("session not found")
		}
		return fmt.Errorf("error validating session: %w", err)
	}

	if !session.IsActive {
		result.ValidationErrors = append(result.ValidationErrors, ValidationError{
			Code:      "SESSION_INACTIVE",
			Message:   "Session is inactive",
			Severity:  "HIGH",
			Timestamp: time.Now(),
		})
		return fmt.Errorf("session is inactive")
	}

	if session.ExpiresAt.Before(time.Now()) {
		result.ValidationErrors = append(result.ValidationErrors, ValidationError{
			Code:      "SESSION_EXPIRED",
			Message:   "Session has expired",
			Severity:  "MEDIUM",
			Timestamp: time.Now(),
		})
		return fmt.Errorf("session expired")
	}

	return nil
}

// checkSecurityFlags identifies potential security concerns
func (ejv *EnhancedJWTValidator) checkSecurityFlags(claims *TokenClaims, result *TokenValidationResult) {
	now := time.Now()

	// Check for suspicious token age patterns
	if claims.IssuedAt != nil {
		tokenAge := now.Sub(claims.IssuedAt.Time)
		if tokenAge < 1*time.Second {
			result.SecurityFlags = append(result.SecurityFlags, SecurityFlag{
				Type:        "VERY_RECENT_TOKEN",
				Description: "Token was issued very recently",
				Risk:        "LOW",
				Timestamp:   now,
			})
		}
	}

	// Check for missing optional security claims
	if claims.SessionID == uuid.Nil {
		result.SecurityFlags = append(result.SecurityFlags, SecurityFlag{
			Type:        "MISSING_SESSION_ID",
			Description: "Token does not have an associated session",
			Risk:        "MEDIUM",
			Timestamp:   now,
		})
	}

	// Check for role elevation concerns
	if claims.Role == "admin" || claims.Role == "super_admin" {
		result.SecurityFlags = append(result.SecurityFlags, SecurityFlag{
			Type:        "ELEVATED_PRIVILEGES",
			Description: fmt.Sprintf("Token has elevated privileges: %s", claims.Role),
			Risk:        "HIGH",
			Timestamp:   now,
		})
	}
}

// SecurityAuditLogger methods for comprehensive audit logging

func (sal *SecurityAuditLogger) LogSuccessfulValidation(userID uuid.UUID, clientIP string, latency time.Duration) {
	sal.logger.Info("JWT validation successful",
		zap.String("user_id", userID.String()),
		zap.String("client_ip", clientIP),
		zap.Duration("validation_latency", latency),
		zap.String("event_type", "jwt_validation_success"))
}

func (sal *SecurityAuditLogger) LogTokenParseError(tokenString, clientIP string, err error) {
	// Don't log the actual token for security reasons
	sal.logger.Warn("JWT token parse error",
		zap.String("client_ip", clientIP),
		zap.String("error", err.Error()),
		zap.String("token_length", fmt.Sprintf("%d", len(tokenString))),
		zap.String("event_type", "jwt_parse_error"))
}

func (sal *SecurityAuditLogger) LogBlacklistedTokenAttempt(tokenString, clientIP string) {
	tokenHash := hashToken(tokenString)
	sal.logger.Warn("Blacklisted token access attempt",
		zap.String("client_ip", clientIP),
		zap.String("token_hash", tokenHash[:16]), // Only log first 16 chars
		zap.String("event_type", "blacklisted_token_attempt"))
}

func (sal *SecurityAuditLogger) LogRateLimitExceeded(clientIP, rateLimitKey string) {
	sal.logger.Warn("JWT validation rate limit exceeded",
		zap.String("client_ip", clientIP),
		zap.String("rate_limit_key", rateLimitKey),
		zap.String("event_type", "jwt_validation_rate_limit"))
}

// SecureTokenComparison performs constant-time token comparison to prevent timing attacks
func SecureTokenComparison(token1, token2 string) bool {
	return subtle.ConstantTimeCompare([]byte(token1), []byte(token2)) == 1
}
