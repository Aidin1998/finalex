package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/Aidin1998/finalex/pkg/validation"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// CreateAPIKey creates a new API key for a user
func (s *Service) CreateAPIKey(ctx context.Context, userID uuid.UUID, name string, permissions []string, expiresAt *time.Time) (*APIKey, error) {
	// Validate and sanitize API key name
	if name == "" {
		return nil, fmt.Errorf("API key name cannot be empty")
	}

	if len(name) > 100 {
		return nil, fmt.Errorf("API key name too long (max 100 characters)")
	}

	// Check for malicious content in name
	if validation.ContainsSQLInjection(name) || validation.ContainsXSS(name) {
		s.logger.Warn("Malicious content detected in API key name",
			zap.String("name", name),
			zap.String("user_id", userID.String()))
		return nil, fmt.Errorf("API key name contains invalid characters")
	}
	// Sanitize the name
	cleanName := validation.SanitizeInput(name)

	// Validate user ID
	if userID == uuid.Nil {
		return nil, fmt.Errorf("invalid user ID")
	}

	// Validate permissions
	if len(permissions) == 0 {
		return nil, fmt.Errorf("at least one permission must be specified")
	}

	if err := s.validatePermissions(permissions); err != nil {
		return nil, fmt.Errorf("invalid permissions: %w", err)
	}

	// Rate limiting for API key creation
	if s.rateLimiter != nil {
		allowed, err := s.rateLimiter.Allow(ctx, fmt.Sprintf("api_key_creation:%s", userID.String()), 10, time.Hour)
		if err != nil {
			s.logger.Error("Rate limiter error during API key creation", zap.Error(err))
		} else if !allowed {
			s.logger.Warn("API key creation rate limit exceeded",
				zap.String("user_id", userID.String()))
			return nil, fmt.Errorf("too many API key creation attempts, please try again later")
		}
	}

	// Generate API key and secret
	keyID := uuid.New()
	apiKeyString, err := generateSecureKey(32)
	if err != nil {
		return nil, fmt.Errorf("failed to generate API key: %w", err)
	}

	apiSecret, err := generateSecureKey(32)
	if err != nil {
		return nil, fmt.Errorf("failed to generate API secret: %w", err)
	}
	// Create the full API key (key:secret)
	fullAPIKey := fmt.Sprintf("%s:%s", apiKeyString, apiSecret)

	// Use hybrid hashing for new API keys with default medium security level
	var hashData *HashedCredential
	var keyHash string

	if s.hybridHasher != nil {
		// Use bcrypt as default for new API keys (good balance of security and performance)
		hashData, err = s.hybridHasher.Hash(fullAPIKey, HashBcrypt)
		if err != nil {
			s.logger.Error("Failed to create hybrid hash, falling back to SHA256", zap.Error(err))
			keyHash = hashAPIKey(fullAPIKey)
		}
	} else {
		// Fallback to legacy SHA256 hashing
		keyHash = hashAPIKey(fullAPIKey)
	}

	// Create API key record
	apiKey := &APIKey{
		ID:          keyID,
		UserID:      userID,
		Name:        cleanName, // Use sanitized name
		KeyHash:     keyHash,
		HashData:    hashData, // Store hybrid hash metadata
		Permissions: permissions,
		ExpiresAt:   expiresAt,
		IsActive:    true,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// Save to database
	if err := s.db.Create(apiKey).Error; err != nil {
		return nil, fmt.Errorf("failed to create API key: %w", err)
	}
	// Return the API key with the actual key value (only shown once)
	apiKey.KeyHash = fullAPIKey // Temporarily set for return
	s.logger.Info("API key created",
		zap.String("user_id", userID.String()),
		zap.String("key_id", keyID.String()),
		zap.String("name", cleanName), // Use sanitized name in logging
	)

	return apiKey, nil
}

// ValidateAPIKey validates an API key and returns claims
func (s *Service) ValidateAPIKey(ctx context.Context, apiKey string) (*APIKeyClaims, error) {
	return s.ValidateAPIKeyForEndpoint(ctx, apiKey, "")
}

// ValidateAPIKeyForEndpoint validates an API key with endpoint-aware security
func (s *Service) ValidateAPIKeyForEndpoint(ctx context.Context, apiKey, endpoint string) (*APIKeyClaims, error) {
	// Apply rate limiting
	if s.rateLimiter != nil {
		allowed, err := s.rateLimiter.Allow(ctx, "api_key_validation", 1000, time.Minute)
		if err != nil {
			return nil, fmt.Errorf("rate limiter error: %w", err)
		}
		if !allowed {
			return nil, fmt.Errorf("rate limit exceeded")
		}
	}

	// Get endpoint security classification for algorithm selection
	var useHybridHashing bool
	if endpoint != "" && s.securityManager != nil {
		classification := s.securityManager.GetEndpointClassification(endpoint)
		// Use hybrid hashing for non-critical endpoints
		useHybridHashing = classification.SecurityLevel != SecurityLevelCritical
	}

	var dbKey APIKey
	var err error

	if useHybridHashing && s.hybridHasher != nil {
		// Use endpoint-aware hashing for validation
		err = s.validateAPIKeyWithHybridHashing(ctx, apiKey, endpoint, &dbKey)
	} else {
		// Fallback to legacy SHA256 hashing
		keyHash := hashAPIKey(apiKey)
		err = s.db.Where("key_hash = ? AND is_active = ? AND (expires_at IS NULL OR expires_at > ?)",
			keyHash, true, time.Now()).First(&dbKey).Error
	}

	if err != nil {
		return nil, fmt.Errorf("invalid or expired API key")
	}

	// Update last used timestamp
	s.db.Model(&dbKey).Update("last_used_at", time.Now())

	return &APIKeyClaims{
		KeyID:       dbKey.ID,
		UserID:      dbKey.UserID,
		Permissions: dbKey.Permissions,
	}, nil
}

// validateAPIKeyWithHybridHashing validates API key using hybrid hashing based on hash metadata
func (s *Service) validateAPIKeyWithHybridHashing(ctx context.Context, apiKey, endpoint string, dbKey *APIKey) error {
	// First try to find a record with hash metadata (new format)
	var keysWithHashData []APIKey
	err := s.db.Where("hash_data IS NOT NULL AND is_active = ? AND (expires_at IS NULL OR expires_at > ?)",
		true, time.Now()).Find(&keysWithHashData).Error
	if err != nil {
		return err
	}

	// Check each key with hybrid hashing
	for _, key := range keysWithHashData {
		if key.HashData != nil {
			valid, err := s.hybridHasher.Verify(apiKey, key.HashData)
			if err != nil {
				s.logger.Debug("Error verifying hybrid hash", zap.Error(err))
				continue
			}
			if valid {
				*dbKey = key
				return nil
			}
		}
	}

	// Fallback to legacy SHA256 for backward compatibility
	keyHash := hashAPIKey(apiKey)
	return s.db.Where("key_hash = ? AND is_active = ? AND (expires_at IS NULL OR expires_at > ?)",
		keyHash, true, time.Now()).First(dbKey).Error
}

// RevokeAPIKey revokes an API key
func (s *Service) RevokeAPIKey(ctx context.Context, keyID uuid.UUID) error {
	result := s.db.Model(&APIKey{}).Where("id = ?", keyID).Update("is_active", false)
	if result.Error != nil {
		return fmt.Errorf("failed to revoke API key: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("API key not found")
	}
	s.logger.Info("API key revoked",
		zap.String("key_id", keyID.String()),
	)

	return nil
}

// ListAPIKeys lists all API keys for a user
func (s *Service) ListAPIKeys(ctx context.Context, userID uuid.UUID) ([]*APIKey, error) {
	var apiKeys []*APIKey
	err := s.db.Where("user_id = ? AND is_active = ?", userID, true).
		Select("id", "user_id", "name", "permissions", "last_used_at", "expires_at", "created_at", "updated_at").
		Find(&apiKeys).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list API keys: %w", err)
	}

	return apiKeys, nil
}

// validatePermissions validates that the requested permissions are valid
func (s *Service) validatePermissions(permissions []string) error {
	validPermissions := map[string]bool{
		"orders:read":       true,
		"orders:write":      true,
		"orders:delete":     true,
		"trades:read":       true,
		"trades:write":      true,
		"accounts:read":     true,
		"accounts:write":    true,
		"users:read":        true,
		"users:write":       true,
		"users:delete":      true,
		"system:admin":      true,
		"marketdata:read":   true,
		"wallets:read":      true,
		"wallets:write":     true,
		"deposits:read":     true,
		"deposits:write":    true,
		"withdrawals:read":  true,
		"withdrawals:write": true,
	}

	for _, permission := range permissions {
		if !validPermissions[permission] {
			return fmt.Errorf("invalid permission: %s", permission)
		}
	}

	return nil
}

// hashAPIKey creates a hash of an API key
func hashAPIKey(apiKey string) string {
	hash := sha256.Sum256([]byte(apiKey))
	return hex.EncodeToString(hash[:])
}

// ValidateAPIKeySignature validates an API key signature for signed requests
func (s *Service) ValidateAPIKeySignature(ctx context.Context, apiKey, signature, timestamp, method, path, body string) (*APIKeyClaims, error) {
	// First validate the API key
	claims, err := s.ValidateAPIKey(ctx, apiKey)
	if err != nil {
		return nil, err
	}

	// Extract secret from API key
	parts := strings.Split(apiKey, ":")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid API key format")
	}
	secret := parts[1]

	// Create signature payload
	payload := fmt.Sprintf("%s%s%s%s%s", timestamp, method, path, body, timestamp)

	// Calculate expected signature
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	expectedSignature := hex.EncodeToString(mac.Sum(nil))

	// Compare signatures
	if !hmac.Equal([]byte(signature), []byte(expectedSignature)) {
		return nil, fmt.Errorf("invalid signature")
	}

	// Validate timestamp (prevent replay attacks)
	if err := s.validateTimestamp(timestamp); err != nil {
		return nil, fmt.Errorf("invalid timestamp: %w", err)
	}

	return claims, nil
}

// validateTimestamp validates that the timestamp is within acceptable range
func (s *Service) validateTimestamp(timestamp string) error {
	// Parse timestamp
	ts, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		return fmt.Errorf("invalid timestamp format: %w", err)
	}

	// Check if timestamp is within 5 minutes of current time
	now := time.Now()
	diff := now.Sub(ts)
	if diff < 0 {
		diff = -diff
	}

	if diff > 5*time.Minute {
		return fmt.Errorf("timestamp too old or too far in future")
	}

	return nil
}

// GenerateAPIKeyPair generates a new API key and secret pair
func GenerateAPIKeyPair() (string, string, error) {
	key, err := generateSecureKey(24)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate API key: %w", err)
	}

	secret, err := generateSecureKey(32)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate API secret: %w", err)
	}

	return key, secret, nil
}
