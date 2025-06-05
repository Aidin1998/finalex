//go:build userauth && stress

package test

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/userauth"
	usermodels "github.com/Aidin1998/pincex_unified/internal/userauth/models"
	"github.com/Aidin1998/pincex_unified/pkg/models"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// setupStressTestDB creates an in-memory SQLite database with all required tables
// and returns the db connection for testing
func setupStressTestDB(t *testing.T) *gorm.DB {
	// Use silent logger to reduce test output noise
	silentLogger := logger.Default.LogMode(logger.Silent)
	
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: silentLogger,
	})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	// Migrate tables
	err = db.AutoMigrate(
		&models.User{},
		&usermodels.UserProfile{},
		&usermodels.TwoFactorAuth{},
		&usermodels.DeviceFingerprint{},
		&usermodels.PasswordPolicy{},
		&usermodels.UserSession{},
	)
	if err != nil {
		t.Fatalf("failed to migrate tables: %v", err)
	}

	// Create some initial data if needed
	return db
}

// setupConcurrentRegistrationService creates a registration service with real implementations
// but configured for performance testing
func setupConcurrentRegistrationService(t *testing.T, db *gorm.DB) *userauth.EnterpriseRegistrationService {
	return userauth.NewEnterpriseRegistrationService(
		db,
		nil,
		&mockEncryptionService{},
		&mockComplianceService{},
		&mockAuditService{},
		&mockPasswordPolicyEngine{},
		&mockKYCIntegrationService{},
		&mockNotificationService{},
	)
}

// generateUniqueRegistrationRequest creates a unique registration request for stress testing
func generateUniqueRegistrationRequest(i int) *userauth.EnterpriseRegistrationRequest {
	dob := time.Now().AddDate(-20, 0, 0)
	return &userauth.EnterpriseRegistrationRequest{
		Email:                 fmt.Sprintf("user%d@example.com", i),
		Username:              fmt.Sprintf("user%d", i),
		Password:              fmt.Sprintf("Password%d!", i),
		FirstName:             "Test",
		LastName:              fmt.Sprintf("User%d", i),
		PhoneNumber:           fmt.Sprintf("+1234567%04d", i),
		DateOfBirth:           &dob,
		Country:               "USA",
		AddressLine1:          "123 Main St",
		AddressLine2:          fmt.Sprintf("Apt %d", i),
		City:                  "Metropolis",
		State:                 "NY",
		PostalCode:            "10001",
		AcceptTerms:           true,
		AcceptPrivacyPolicy:   true,
		AcceptKYCRequirements: true,
		MarketingConsent:      false,
		ReferralCode:          "",
		PreferredLanguage:     "en",
		Timezone:              "UTC",
		DeviceFingerprint:     fmt.Sprintf("devicefp%d", i),
		UserAgent:             "Mozilla/5.0",
		IPAddress:             fmt.Sprintf("127.0.0.%d", i%255+1),
		GeolocationData:       map[string]interface{}{"lat": 40.7128, "lon": -74.0060},
	}
}

// TestConcurrentRegistration tests the registration service under high concurrency
func TestConcurrentRegistration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	db := setupStressTestDB(t)
	svc := setupConcurrentRegistrationService(t, db)

	// Number of concurrent registrations to attempt
	const concurrentUsers := 100
	
	var (
		wg sync.WaitGroup
		successCount int32
		failureCount int32
		results = make([]error, concurrentUsers)
	)

	// Create a timeout context
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	startTime := time.Now()

	// Launch concurrent registrations
	for i := 0; i < concurrentUsers; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			
			req := generateUniqueRegistrationRequest(index)
			resp, err := svc.RegisterUser(ctx, req)
			
			if err != nil {
				atomic.AddInt32(&failureCount, 1)
				results[index] = err
			} else if resp != nil {
				atomic.AddInt32(&successCount, 1)
				results[index] = nil
			}
		}(i)
	}

	// Wait for all goroutines to complete
	wg.Wait()
	
	duration := time.Since(startTime)
	
	// Report results
	t.Logf("Concurrent registration stress test completed in %v", duration)
	t.Logf("Success: %d, Failures: %d", successCount, failureCount)
	t.Logf("Average registration time: %v", duration/time.Duration(concurrentUsers))
	
	// Verify all registrations succeeded
	assert.Equal(t, int32(concurrentUsers), successCount, "All registrations should succeed")
	assert.Equal(t, int32(0), failureCount, "No registrations should fail")
	
	// Check database for correct number of users
	var count int64
	db.Model(&models.User{}).Count(&count)
	assert.Equal(t, int64(concurrentUsers), count, "Database should contain all registered users")
}

// TestConcurrentAuthentication tests the authentication service under high concurrency
func TestConcurrentAuthentication(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	db := setupStressTestDB(t)
	
	// First, create some users for authentication testing
	const userCount = 50
	users := make([]struct{
		email string
		password string
	}, userCount)
	
	// Create users
	for i := 0; i < userCount; i++ {
		email := fmt.Sprintf("auth%d@example.com", i)
		password := fmt.Sprintf("Password%d!", i)
		users[i] = struct {
			email    string
			password string
		}{email, password}
		
		hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		
		user := models.User{
			ID:           uuid.New(),
			Email:        email,
			Username:     fmt.Sprintf("auth%d", i),
			PasswordHash: string(hashedPassword),
			FirstName:    "Test",
			LastName:     fmt.Sprintf("User%d", i),
			KYCStatus:    "approved",
			Role:         "user",
			Tier:         "basic",
			MFAEnabled:   false,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}
		
		if err := db.Create(&user).Error; err != nil {
			t.Fatalf("Failed to create test user: %v", err)
		}
	}
	
	// Create a mock auth service using our db
	authSvc := &mockAuthServiceWithDB{
		db: db,
	}
	
	// Test concurrent authentication
	const concurrentRequests = 100
	var (
		wg sync.WaitGroup
		successCount int32
		failureCount int32
	)
	
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	startTime := time.Now()
	
	// Launch concurrent authentications
	for i := 0; i < concurrentRequests; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			
			// Pick a random user
			userIndex := rand.Intn(userCount)
			user := users[userIndex]
			
			// Authenticate
			_, _, err := authSvc.AuthenticateUser(ctx, user.email, user.password)
			
			if err != nil {
				atomic.AddInt32(&failureCount, 1)
			} else {
				atomic.AddInt32(&successCount, 1)
			}
		}(i)
	}
	
	// Wait for all goroutines to complete
	wg.Wait()
	
	duration := time.Since(startTime)
	
	// Report results
	t.Logf("Concurrent authentication stress test completed in %v", duration)
	t.Logf("Success: %d, Failures: %d", successCount, failureCount)
	t.Logf("Average authentication time: %v", duration/time.Duration(concurrentRequests))
	
	// All authentications should succeed
	assert.Equal(t, int32(concurrentRequests), successCount, "All authentications should succeed")
	assert.Equal(t, int32(0), failureCount, "No authentications should fail")
}

// Mock implementation of authentication service with DB integration
type mockAuthServiceWithDB struct {
	db *gorm.DB
}

// AuthenticateUser authenticates a user via database lookup
func (m *mockAuthServiceWithDB) AuthenticateUser(ctx context.Context, email, password string) (*auth.TokenPair, *models.User, error) {
	// Find the user
	var user models.User
	if err := m.db.WithContext(ctx).Where("email = ?", email).First(&user).Error; err != nil {
		return nil, nil, fmt.Errorf("invalid credentials")
	}
	
	// Check password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, nil, fmt.Errorf("invalid credentials")
	}
	
	// Generate token pair
	tokenPair := &auth.TokenPair{
		AccessToken:  fmt.Sprintf("access_%s", user.ID),
		RefreshToken: fmt.Sprintf("refresh_%s", user.ID),
		ExpiresIn:    3600,
	}
	
	return tokenPair, &user, nil
}
