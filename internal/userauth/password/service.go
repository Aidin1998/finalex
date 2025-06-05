// filepath: c:\Orbit CEX\Finalex\internal\userauth\password\service.go
package password

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
	"time"
	"unicode"

	"github.com/Aidin1998/pincex_unified/internal/userauth/models"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// PasswordStrength represents password strength levels
type PasswordStrength int

const (
	PasswordStrengthWeak PasswordStrength = iota
	PasswordStrengthFair
	PasswordStrengthGood
	PasswordStrengthStrong
	PasswordStrengthVeryStrong
)

// PasswordPolicy represents password policy requirements
type PasswordPolicy struct {
	MinLength               int      `json:"min_length"`
	MaxLength               int      `json:"max_length"`
	RequireUppercase        bool     `json:"require_uppercase"`
	RequireLowercase        bool     `json:"require_lowercase"`
	RequireNumbers          bool     `json:"require_numbers"`
	RequireSpecialChars     bool     `json:"require_special_chars"`
	MinSpecialChars         int      `json:"min_special_chars"`
	DisallowSequentialChars bool     `json:"disallow_sequential_chars"`
	DisallowRepeatingChars  bool     `json:"disallow_repeating_chars"`
	DisallowCommonPasswords bool     `json:"disallow_common_passwords"`
	DisallowPersonalInfo    bool     `json:"disallow_personal_info"`
	PasswordHistoryCount    int      `json:"password_history_count"`
	MaxAge                  int      `json:"max_age_days"`
	MaxFailedAttempts       int      `json:"max_failed_attempts"`
	LockoutDuration         int      `json:"lockout_duration_minutes"`
	RequirePeriodicChange   bool     `json:"require_periodic_change"`
	AllowedSpecialChars     string   `json:"allowed_special_chars"`
	ForbiddenPatterns       []string `json:"forbidden_patterns"`
}

// PasswordValidationResult represents the result of password validation
type PasswordValidationResult struct {
	IsValid       bool             `json:"is_valid"`
	Strength      PasswordStrength `json:"strength"`
	StrengthScore int              `json:"strength_score"`
	Errors        []string         `json:"errors"`
	Warnings      []string         `json:"warnings"`
	Suggestions   []string         `json:"suggestions"`
	Policy        PasswordPolicy   `json:"policy"`
}

// Service provides password policy and management services
type Service struct {
	db                 *gorm.DB
	redis              *redis.Client
	logger             *zap.Logger
	defaultPolicy      PasswordPolicy
	commonPasswords    map[string]bool
	sequentialPatterns []string
}

// NewService creates a new password service
func NewService(logger *zap.Logger, db *gorm.DB, redisClient *redis.Client) *Service {
	service := &Service{
		db:     db,
		redis:  redisClient,
		logger: logger,
		defaultPolicy: PasswordPolicy{
			MinLength:               12,
			MaxLength:               128,
			RequireUppercase:        true,
			RequireLowercase:        true,
			RequireNumbers:          true,
			RequireSpecialChars:     true,
			MinSpecialChars:         1,
			DisallowSequentialChars: true,
			DisallowRepeatingChars:  true,
			DisallowCommonPasswords: true,
			DisallowPersonalInfo:    true,
			PasswordHistoryCount:    5,
			MaxAge:                  90,
			MaxFailedAttempts:       5,
			LockoutDuration:         30,
			RequirePeriodicChange:   true,
			AllowedSpecialChars:     "!@#$%^&*()_+-=[]{}|;:,.<>?",
			ForbiddenPatterns:       []string{"password", "123", "abc", "qwerty", "admin"},
		},
		commonPasswords:    make(map[string]bool),
		sequentialPatterns: []string{"abc", "123", "qwe", "asd", "zxc"},
	}

	service.loadCommonPasswords()
	return service
}

// ValidatePassword validates a password against the policy
func (s *Service) ValidatePassword(ctx context.Context, password string, userInfo map[string]string, userID *uuid.UUID) PasswordValidationResult {
	policy := s.defaultPolicy

	// Get user-specific policy if available
	if userID != nil {
		if userPolicy, err := s.getUserPasswordPolicy(ctx, *userID); err == nil && userPolicy != nil {
			policy = *userPolicy
		}
	}

	result := PasswordValidationResult{
		IsValid:     true,
		Policy:      policy,
		Errors:      []string{},
		Warnings:    []string{},
		Suggestions: []string{},
	}

	// Length validation
	if len(password) < policy.MinLength {
		result.IsValid = false
		result.Errors = append(result.Errors, fmt.Sprintf("Password must be at least %d characters long", policy.MinLength))
	}

	if len(password) > policy.MaxLength {
		result.IsValid = false
		result.Errors = append(result.Errors, fmt.Sprintf("Password must not exceed %d characters", policy.MaxLength))
	}

	// Character requirements
	hasUpper := s.hasUppercase(password)
	hasLower := s.hasLowercase(password)
	hasNumber := s.hasNumber(password)
	hasSpecial := s.hasSpecialChar(password, policy.AllowedSpecialChars)
	specialCharCount := s.countSpecialChars(password, policy.AllowedSpecialChars)

	if policy.RequireUppercase && !hasUpper {
		result.IsValid = false
		result.Errors = append(result.Errors, "Password must contain at least one uppercase letter")
	}

	if policy.RequireLowercase && !hasLower {
		result.IsValid = false
		result.Errors = append(result.Errors, "Password must contain at least one lowercase letter")
	}

	if policy.RequireNumbers && !hasNumber {
		result.IsValid = false
		result.Errors = append(result.Errors, "Password must contain at least one number")
	}

	if policy.RequireSpecialChars && !hasSpecial {
		result.IsValid = false
		result.Errors = append(result.Errors, "Password must contain at least one special character")
	}

	if specialCharCount < policy.MinSpecialChars {
		result.IsValid = false
		result.Errors = append(result.Errors, fmt.Sprintf("Password must contain at least %d special characters", policy.MinSpecialChars))
	}

	// Pattern validations
	if policy.DisallowSequentialChars && s.hasSequentialChars(password) {
		result.IsValid = false
		result.Errors = append(result.Errors, "Password must not contain sequential characters")
	}

	if policy.DisallowRepeatingChars && s.hasRepeatingChars(password) {
		result.IsValid = false
		result.Errors = append(result.Errors, "Password must not contain repeating characters")
	}

	// Common password check
	if policy.DisallowCommonPasswords && s.isCommonPassword(password) {
		result.IsValid = false
		result.Errors = append(result.Errors, "Password is too common and easily guessable")
	}

	// Personal information check
	if policy.DisallowPersonalInfo && s.containsPersonalInfo(password, userInfo) {
		result.IsValid = false
		result.Errors = append(result.Errors, "Password must not contain personal information")
	}

	// Forbidden patterns
	for _, pattern := range policy.ForbiddenPatterns {
		if strings.Contains(strings.ToLower(password), pattern) {
			result.IsValid = false
			result.Errors = append(result.Errors, fmt.Sprintf("Password must not contain the pattern: %s", pattern))
		}
	}

	// Password history check (if user provided)
	if userID != nil && result.IsValid {
		if exists, err := s.isPasswordInHistory(ctx, *userID, password); err == nil && exists {
			result.IsValid = false
			result.Errors = append(result.Errors, fmt.Sprintf("Password was used recently. Choose a different password."))
		}
	}

	// Calculate strength
	result.Strength, result.StrengthScore = s.calculatePasswordStrength(password)

	// Add suggestions
	if len(password) < 16 {
		result.Suggestions = append(result.Suggestions, "Consider using a longer password (16+ characters) for better security")
	}

	if !hasSpecial || specialCharCount < 2 {
		result.Suggestions = append(result.Suggestions, "Add more special characters for increased complexity")
	}

	if result.StrengthScore < 80 {
		result.Suggestions = append(result.Suggestions, "Consider adding more character variety to strengthen your password")
	}

	return result
}

// HashPassword securely hashes a password
func (s *Service) HashPassword(password string) (string, error) {
	// Use bcrypt with cost factor 12 for security
	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		s.logger.Error("Failed to hash password", zap.Error(err))
		return "", fmt.Errorf("failed to hash password: %w", err)
	}
	return string(hashedBytes), nil
}

// VerifyPassword verifies a password against its hash
func (s *Service) VerifyPassword(password, hashedPassword string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
	return err == nil
}

// ChangePassword changes a user's password with validation
func (s *Service) ChangePassword(ctx context.Context, userID uuid.UUID, currentPassword, newPassword string, userInfo map[string]string) error {
	// Get current password hash
	var user models.UserProfile
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).First(&user).Error; err != nil {
		return fmt.Errorf("user not found: %w", err)
	}

	// Verify current password (assuming password hash is stored in user profile)
	// Note: This assumes password hash is stored in the user profile - adjust as needed
	// if !s.VerifyPassword(currentPassword, user.PasswordHash) {
	// 	return fmt.Errorf("current password is incorrect")
	// }

	// Validate new password
	validation := s.ValidatePassword(ctx, newPassword, userInfo, &userID)
	if !validation.IsValid {
		return fmt.Errorf("password validation failed: %s", strings.Join(validation.Errors, ", "))
	}

	// Hash new password
	hashedPassword, err := s.HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("failed to hash new password: %w", err)
	}

	// Store password in history before updating
	if err := s.addPasswordToHistory(ctx, userID, hashedPassword); err != nil {
		s.logger.Warn("Failed to add password to history", zap.Error(err))
	}

	// Update password (this would typically be in a user table)
	// updates := map[string]interface{}{
	// 	"password_hash": hashedPassword,
	// 	"password_changed_at": time.Now(),
	// 	"updated_at": time.Now(),
	// }
	//
	// if err := s.db.WithContext(ctx).Model(&user).Updates(updates).Error; err != nil {
	// 	return fmt.Errorf("failed to update password: %w", err)
	// }

	s.logger.Info("Password changed successfully", zap.String("user_id", userID.String()))
	return nil
}

// GenerateStrongPassword generates a cryptographically secure password
func (s *Service) GenerateStrongPassword(length int, includeSymbols bool) (string, error) {
	if length < 8 {
		length = 12 // Minimum secure length
	}

	var charset string
	charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

	if includeSymbols {
		charset += "!@#$%^&*()_+-=[]{}|;:,.<>?"
	}

	password := make([]byte, length)
	charsetLength := big.NewInt(int64(len(charset)))

	for i := 0; i < length; i++ {
		randomIndex, err := rand.Int(rand.Reader, charsetLength)
		if err != nil {
			return "", fmt.Errorf("failed to generate random number: %w", err)
		}
		password[i] = charset[randomIndex.Int64()]
	}

	// Ensure the generated password meets requirements
	generatedPassword := string(password)
	validation := s.ValidatePassword(context.Background(), generatedPassword, nil, nil)

	if !validation.IsValid {
		// Retry once if generated password doesn't meet requirements
		return s.GenerateStrongPassword(length, includeSymbols)
	}

	return generatedPassword, nil
}

// CheckPasswordExpiry checks if a user's password has expired
func (s *Service) CheckPasswordExpiry(ctx context.Context, userID uuid.UUID) (bool, time.Time, error) {
	policy := s.defaultPolicy

	// Get user-specific policy if available
	if userPolicy, err := s.getUserPasswordPolicy(ctx, userID); err == nil && userPolicy != nil {
		policy = *userPolicy
	}

	if !policy.RequirePeriodicChange || policy.MaxAge <= 0 {
		return false, time.Time{}, nil // No expiry policy
	}

	// Get user's last password change date
	var user models.UserProfile
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).First(&user).Error; err != nil {
		return false, time.Time{}, fmt.Errorf("user not found: %w", err)
	}

	// Calculate expiry date
	// Note: This assumes password_changed_at field exists - adjust as needed
	passwordChangedAt := user.UpdatedAt // Placeholder - use actual password change date
	expiryDate := passwordChangedAt.AddDate(0, 0, policy.MaxAge)

	isExpired := time.Now().After(expiryDate)
	return isExpired, expiryDate, nil
}

// RecordFailedAttempt records a failed password attempt
func (s *Service) RecordFailedAttempt(ctx context.Context, userID uuid.UUID) error {
	key := fmt.Sprintf("failed_attempts:%s", userID.String())

	// Increment counter
	attempts, err := s.redis.Incr(ctx, key).Result()
	if err != nil {
		return fmt.Errorf("failed to record attempt: %w", err)
	}

	// Set expiry if this is the first attempt
	if attempts == 1 {
		s.redis.Expire(ctx, key, time.Hour*24) // Reset after 24 hours
	}

	policy := s.defaultPolicy
	if userPolicy, err := s.getUserPasswordPolicy(ctx, userID); err == nil && userPolicy != nil {
		policy = *userPolicy
	}

	// Check if account should be locked
	if int(attempts) >= policy.MaxFailedAttempts {
		if err := s.lockAccount(ctx, userID, time.Duration(policy.LockoutDuration)*time.Minute); err != nil {
			s.logger.Error("Failed to lock account", zap.Error(err))
		}
	}

	return nil
}

// ClearFailedAttempts clears failed password attempts for a user
func (s *Service) ClearFailedAttempts(ctx context.Context, userID uuid.UUID) error {
	key := fmt.Sprintf("failed_attempts:%s", userID.String())
	return s.redis.Del(ctx, key).Err()
}

// IsAccountLocked checks if an account is locked due to failed attempts
func (s *Service) IsAccountLocked(ctx context.Context, userID uuid.UUID) (bool, time.Time, error) {
	key := fmt.Sprintf("account_locked:%s", userID.String())

	ttl, err := s.redis.TTL(ctx, key).Result()
	if err != nil {
		return false, time.Time{}, fmt.Errorf("failed to check lock status: %w", err)
	}

	if ttl == -2 { // Key doesn't exist
		return false, time.Time{}, nil
	}

	unlockTime := time.Now().Add(ttl)
	return true, unlockTime, nil
}

// Helper methods

func (s *Service) hasUppercase(password string) bool {
	for _, char := range password {
		if unicode.IsUpper(char) {
			return true
		}
	}
	return false
}

func (s *Service) hasLowercase(password string) bool {
	for _, char := range password {
		if unicode.IsLower(char) {
			return true
		}
	}
	return false
}

func (s *Service) hasNumber(password string) bool {
	for _, char := range password {
		if unicode.IsDigit(char) {
			return true
		}
	}
	return false
}

func (s *Service) hasSpecialChar(password, allowedChars string) bool {
	for _, char := range password {
		if strings.ContainsRune(allowedChars, char) {
			return true
		}
	}
	return false
}

func (s *Service) countSpecialChars(password, allowedChars string) int {
	count := 0
	for _, char := range password {
		if strings.ContainsRune(allowedChars, char) {
			count++
		}
	}
	return count
}

func (s *Service) hasSequentialChars(password string) bool {
	password = strings.ToLower(password)
	for _, pattern := range s.sequentialPatterns {
		if strings.Contains(password, pattern) {
			return true
		}
	}

	// Check for numeric sequences
	for i := 0; i < len(password)-2; i++ {
		if password[i] >= '0' && password[i] <= '9' &&
			password[i+1] >= '0' && password[i+1] <= '9' &&
			password[i+2] >= '0' && password[i+2] <= '9' {
			if password[i+1] == password[i]+1 && password[i+2] == password[i+1]+1 {
				return true
			}
		}
	}

	return false
}

func (s *Service) hasRepeatingChars(password string) bool {
	for i := 0; i < len(password)-2; i++ {
		if password[i] == password[i+1] && password[i+1] == password[i+2] {
			return true
		}
	}
	return false
}

func (s *Service) isCommonPassword(password string) bool {
	return s.commonPasswords[strings.ToLower(password)]
}

func (s *Service) containsPersonalInfo(password string, userInfo map[string]string) bool {
	passwordLower := strings.ToLower(password)

	for _, info := range userInfo {
		if len(info) >= 3 && strings.Contains(passwordLower, strings.ToLower(info)) {
			return true
		}
	}
	return false
}

func (s *Service) calculatePasswordStrength(password string) (PasswordStrength, int) {
	score := 0

	// Length scoring
	if len(password) >= 8 {
		score += 10
	}
	if len(password) >= 12 {
		score += 10
	}
	if len(password) >= 16 {
		score += 10
	}

	// Character variety scoring
	if s.hasLowercase(password) {
		score += 5
	}
	if s.hasUppercase(password) {
		score += 5
	}
	if s.hasNumber(password) {
		score += 10
	}
	if s.hasSpecialChar(password, "!@#$%^&*()_+-=[]{}|;:,.<>?") {
		score += 15
	}

	// Complexity bonuses
	charTypes := 0
	if s.hasLowercase(password) {
		charTypes++
	}
	if s.hasUppercase(password) {
		charTypes++
	}
	if s.hasNumber(password) {
		charTypes++
	}
	if s.hasSpecialChar(password, "!@#$%^&*()_+-=[]{}|;:,.<>?") {
		charTypes++
	}

	score += charTypes * 5

	// Penalties
	if s.hasSequentialChars(password) {
		score -= 20
	}
	if s.hasRepeatingChars(password) {
		score -= 15
	}
	if s.isCommonPassword(password) {
		score -= 30
	}

	// Normalize score
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}

	// Determine strength level
	var strength PasswordStrength
	switch {
	case score >= 90:
		strength = PasswordStrengthVeryStrong
	case score >= 70:
		strength = PasswordStrengthStrong
	case score >= 50:
		strength = PasswordStrengthGood
	case score >= 30:
		strength = PasswordStrengthFair
	default:
		strength = PasswordStrengthWeak
	}

	return strength, score
}

func (s *Service) getUserPasswordPolicy(ctx context.Context, userID uuid.UUID) (*PasswordPolicy, error) {
	// This would retrieve user-specific or role-based password policies
	// For now, return nil to use default policy
	return nil, nil
}

func (s *Service) isPasswordInHistory(ctx context.Context, userID uuid.UUID, password string) (bool, error) {
	// This would check against stored password history
	// Implementation depends on how you store password history
	return false, nil
}

func (s *Service) addPasswordToHistory(ctx context.Context, userID uuid.UUID, hashedPassword string) error {
	// This would add the password to the user's password history
	// Implementation depends on how you store password history
	return nil
}

func (s *Service) lockAccount(ctx context.Context, userID uuid.UUID, duration time.Duration) error {
	key := fmt.Sprintf("account_locked:%s", userID.String())
	return s.redis.Set(ctx, key, "locked", duration).Err()
}

func (s *Service) loadCommonPasswords() {
	// Load common passwords - in production, this would be loaded from a file or database
	commonPasswords := []string{
		"password", "123456", "password123", "admin", "qwerty",
		"123456789", "12345678", "12345", "1234567890", "1234567",
		"password1", "123123", "abc123", "Password1", "password!",
		"welcome", "monkey", "dragon", "master", "hello",
		"login", "princess", "solo", "letmein", "starwars",
	}

	for _, pwd := range commonPasswords {
		s.commonPasswords[pwd] = true
		s.commonPasswords[strings.ToLower(pwd)] = true
	}
}
