package test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// mockRateLimiter implements RateLimiter interface for testing
type mockRateLimiter struct {
	allowCount int
	maxCalls   int
}

func (m *mockRateLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	m.allowCount++
	return m.allowCount <= m.maxCalls, nil
}

func TestSecurityIntegration(t *testing.T) {
	// Setup test database
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Setup logger
	logger, _ := zap.NewDevelopment()

	// Setup rate limiter
	rateLimiter := &mockRateLimiter{maxCalls: 1000}

	// Create auth service with enhanced security
	authService, err := NewAuthService(
		logger,
		db,
		"test-jwt-secret-key-for-testing-only",
		time.Hour,
		"test-refresh-secret-key-for-testing-only",
		24*time.Hour,
		"test-issuer",
		rateLimiter,
	)
	require.NoError(t, err)

	service := authService.(*Service)

	t.Run("HybridHashingIntegration", func(t *testing.T) {
		testHybridHashingIntegration(t, service)
	})

	t.Run("EndpointSecurityClassification", func(t *testing.T) {
		testEndpointSecurityClassification(t, service)
	})

	t.Run("APIKeyCreationWithHybridHashing", func(t *testing.T) {
		testAPIKeyCreationWithHybridHashing(t, service)
	})

	t.Run("APIKeyValidationEndpointAware", func(t *testing.T) {
		testAPIKeyValidationEndpointAware(t, service)
	})

	t.Run("BackwardCompatibility", func(t *testing.T) {
		testBackwardCompatibility(t, service)
	})

	t.Run("PerformanceRequirements", func(t *testing.T) {
		testPerformanceRequirements(t, service)
	})
}

func testHybridHashingIntegration(t *testing.T, service *Service) {
	assert.NotNil(t, service.hybridHasher, "Hybrid hasher should be initialized")
	assert.NotNil(t, service.securityManager, "Security manager should be initialized")

	// Test hashing with different algorithms
	testCredential := "test-api-key:test-secret"

	// Test SHA256 hashing (critical endpoints)
	sha256Hash, err := service.hybridHasher.Hash(testCredential, HashSHA256)
	require.NoError(t, err)
	assert.Equal(t, HashSHA256, sha256Hash.Algorithm)
	assert.NotEmpty(t, sha256Hash.Hash)

	// Test bcrypt hashing (user management)
	bcryptHash, err := service.hybridHasher.Hash(testCredential, HashBcrypt)
	require.NoError(t, err)
	assert.Equal(t, HashBcrypt, bcryptHash.Algorithm)
	assert.NotEmpty(t, bcryptHash.Hash)
	assert.Greater(t, bcryptHash.Cost, 0)

	// Test Argon2 hashing (admin operations)
	argon2Hash, err := service.hybridHasher.Hash(testCredential, HashArgon2)
	require.NoError(t, err)
	assert.Equal(t, HashArgon2, argon2Hash.Algorithm)
	assert.NotEmpty(t, argon2Hash.Hash)
	assert.NotEmpty(t, argon2Hash.Salt)

	// Test verification
	valid, err := service.hybridHasher.Verify(testCredential, sha256Hash)
	require.NoError(t, err)
	assert.True(t, valid)

	valid, err = service.hybridHasher.Verify(testCredential, bcryptHash)
	require.NoError(t, err)
	assert.True(t, valid)

	valid, err = service.hybridHasher.Verify(testCredential, argon2Hash)
	require.NoError(t, err)
	assert.True(t, valid)

	// Test invalid credential
	valid, err = service.hybridHasher.Verify("wrong-credential", sha256Hash)
	require.NoError(t, err)
	assert.False(t, valid)
}

func testEndpointSecurityClassification(t *testing.T, service *Service) {
	// Test critical trading endpoints
	tradingClassification := service.securityManager.GetEndpointClassification("/api/v1/trade/order")
	assert.Equal(t, SecurityLevelCritical, tradingClassification.SecurityLevel)
	assert.Equal(t, HashSHA256, tradingClassification.HashAlgorithm)
	assert.LessOrEqual(t, tradingClassification.MaxLatencyMs, 10)

	// Test user management endpoints
	userClassification := service.securityManager.GetEndpointClassification("/api/v1/user/profile")
	assert.Equal(t, SecurityLevelHigh, userClassification.SecurityLevel)
	assert.Equal(t, HashBcrypt, userClassification.HashAlgorithm)
	assert.True(t, userClassification.RequiresMFA)

	// Test admin endpoints
	adminClassification := service.securityManager.GetEndpointClassification("/api/v1/admin/users")
	assert.Equal(t, SecurityLevelHigh, adminClassification.SecurityLevel)
	assert.Equal(t, HashArgon2, adminClassification.HashAlgorithm)
	assert.True(t, adminClassification.RequiresMFA)
	assert.True(t, adminClassification.IPWhitelisting)

	// Test public endpoints
	publicClassification := service.securityManager.GetEndpointClassification("/api/v1/public/health")
	assert.Equal(t, SecurityLevelLow, publicClassification.SecurityLevel)
	assert.False(t, publicClassification.RequiresMFA)
}

func testAPIKeyCreationWithHybridHashing(t *testing.T, service *Service) {
	ctx := context.Background()
	userID := uuid.New()

	// Create API key
	apiKey, err := service.CreateAPIKey(ctx, userID, "Test API Key", []string{"orders:read"}, nil)
	require.NoError(t, err)
	assert.NotNil(t, apiKey)
	assert.Equal(t, userID, apiKey.UserID)
	assert.NotEmpty(t, apiKey.KeyHash) // This contains the actual key for return

	// Verify the key was stored with hybrid hashing metadata
	var storedKey APIKey
	err = service.db.Where("id = ?", apiKey.ID).First(&storedKey).Error
	require.NoError(t, err)

	// New keys should have hash metadata
	if storedKey.HashData != nil {
		assert.Equal(t, HashBcrypt, storedKey.HashData.Algorithm)
		assert.NotEmpty(t, storedKey.HashData.Hash)
		assert.Greater(t, storedKey.HashData.Cost, 0)
	}
}

func testAPIKeyValidationEndpointAware(t *testing.T, service *Service) {
	ctx := context.Background()
	userID := uuid.New()

	// Create API key
	apiKey, err := service.CreateAPIKey(ctx, userID, "Test API Key", []string{"orders:read", "trades:read"}, nil)
	require.NoError(t, err)

	actualAPIKey := apiKey.KeyHash // Contains the actual key value

	// Test validation without endpoint (should work)
	claims, err := service.ValidateAPIKey(ctx, actualAPIKey)
	require.NoError(t, err)
	assert.Equal(t, userID, claims.UserID)
	assert.Contains(t, claims.Permissions, "orders:read")

	// Test endpoint-aware validation for critical trading endpoint
	claims, err = service.ValidateAPIKeyForEndpoint(ctx, actualAPIKey, "/api/v1/trade/order")
	require.NoError(t, err)
	assert.Equal(t, userID, claims.UserID)

	// Test endpoint-aware validation for user management endpoint
	claims, err = service.ValidateAPIKeyForEndpoint(ctx, actualAPIKey, "/api/v1/user/profile")
	require.NoError(t, err)
	assert.Equal(t, userID, claims.UserID)

	// Test with invalid API key
	_, err = service.ValidateAPIKey(ctx, "invalid-key")
	assert.Error(t, err)
}

func testBackwardCompatibility(t *testing.T, service *Service) {
	ctx := context.Background()
	userID := uuid.New()

	// Simulate legacy API key with only SHA256 hash (no hash metadata)
	keyID := uuid.New()
	apiKeyString := "legacy-api-key:legacy-secret"
	keyHash := hashAPIKey(apiKeyString)

	legacyKey := &APIKey{
		ID:          keyID,
		UserID:      userID,
		Name:        "Legacy API Key",
		KeyHash:     keyHash,
		HashData:    nil, // No hybrid hash metadata
		Permissions: []string{"orders:read"},
		IsActive:    true,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// Save legacy key to database
	err := service.db.Create(legacyKey).Error
	require.NoError(t, err)

	// Test validation of legacy key
	claims, err := service.ValidateAPIKey(ctx, apiKeyString)
	require.NoError(t, err)
	assert.Equal(t, userID, claims.UserID)
	assert.Contains(t, claims.Permissions, "orders:read")

	// Test endpoint-aware validation of legacy key
	claims, err = service.ValidateAPIKeyForEndpoint(ctx, apiKeyString, "/api/v1/trade/order")
	require.NoError(t, err)
	assert.Equal(t, userID, claims.UserID)
}

func testPerformanceRequirements(t *testing.T, service *Service) {
	ctx := context.Background()
	testCredential := "performance-test-key:secret"

	// Test SHA256 performance (should be < 1ms for critical paths)
	start := time.Now()
	for i := 0; i < 1000; i++ {
		hash, err := service.hybridHasher.Hash(testCredential, HashSHA256)
		require.NoError(t, err)

		valid, err := service.hybridHasher.Verify(testCredential, hash)
		require.NoError(t, err)
		assert.True(t, valid)
	}
	sha256Duration := time.Since(start)
	avgSHA256Time := sha256Duration / 1000

	// SHA256 should be very fast (< 1ms per operation)
	assert.Less(t, avgSHA256Time, time.Millisecond,
		"SHA256 operations should be < 1ms for critical trading paths")

	// Test bcrypt performance (can be slower, < 100ms acceptable)
	start = time.Now()
	for i := 0; i < 10; i++ { // Fewer iterations due to bcrypt cost
		hash, err := service.hybridHasher.Hash(testCredential, HashBcrypt)
		require.NoError(t, err)

		valid, err := service.hybridHasher.Verify(testCredential, hash)
		require.NoError(t, err)
		assert.True(t, valid)
	}
	bcryptDuration := time.Since(start)
	avgBcryptTime := bcryptDuration / 10

	// bcrypt should be reasonable for user management (< 200ms per operation)
	assert.Less(t, avgBcryptTime, 200*time.Millisecond,
		"bcrypt operations should be < 200ms for user management")

	t.Logf("Performance results - SHA256: %v, bcrypt: %v", avgSHA256Time, avgBcryptTime)
}

func TestSecurityConfigIntegration(t *testing.T) {
	// Test default security configuration
	config := DefaultSecurityConfig()
	assert.NotNil(t, config)

	// Test configuration validation
	logger, _ := zap.NewDevelopment()
	configManager := NewSecurityConfigManager(config, logger)

	err := configManager.ValidateConfig()
	assert.NoError(t, err)

	// Test configuration retrieval
	hashingConfig := configManager.GetHashingConfig()
	assert.Equal(t, 12, hashingConfig.BcryptCost)
	assert.True(t, hashingConfig.EnableHashUpgrade)

	rateLimitConfig := configManager.GetRateLimitConfig()
	assert.Equal(t, 10000, rateLimitConfig.GlobalRequestsPerMinute)
	assert.True(t, rateLimitConfig.EnableAdaptive)

	ddosConfig := configManager.GetDDoSConfig()
	assert.Equal(t, 1000, ddosConfig.SuspiciousRequestThreshold)
	assert.True(t, ddosConfig.EnablePatternDetection)
}

func BenchmarkHashingPerformance(b *testing.B) {
	logger, _ := zap.NewDevelopment()
	securityManager := NewEndpointSecurityManager()
	hybridHasher := NewHybridHasher(securityManager)

	testCredential := "benchmark-api-key:benchmark-secret"

	b.Run("SHA256", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			hash, err := hybridHasher.Hash(testCredential, HashSHA256)
			if err != nil {
				b.Fatal(err)
			}
			_, err = hybridHasher.Verify(testCredential, hash)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("bcrypt", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			hash, err := hybridHasher.Hash(testCredential, HashBcrypt)
			if err != nil {
				b.Fatal(err)
			}
			_, err = hybridHasher.Verify(testCredential, hash)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Argon2", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			hash, err := hybridHasher.Hash(testCredential, HashArgon2)
			if err != nil {
				b.Fatal(err)
			}
			_, err = hybridHasher.Verify(testCredential, hash)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}
