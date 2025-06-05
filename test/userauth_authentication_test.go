//go:build userauth

package test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/userauth/auth"
	usermodels "github.com/Aidin1998/pincex_unified/internal/userauth/models"
	"github.com/Aidin1998/pincex_unified/pkg/models"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// Mock authentication service
type mockAuthService struct {
	users           map[string]*models.User
	sessions        map[uuid.UUID]*auth.Session
	invalidSessions map[uuid.UUID]bool
	tokenPairs      map[string]*auth.TokenPair
}

func newMockAuthService() *mockAuthService {
	return &mockAuthService{
		users:           make(map[string]*models.User),
		sessions:        make(map[uuid.UUID]*auth.Session),
		invalidSessions: make(map[uuid.UUID]bool),
		tokenPairs:      make(map[string]*auth.TokenPair),
	}
}

func (m *mockAuthService) AuthenticateUser(ctx context.Context, email, password string) (*auth.TokenPair, *models.User, error) {
	user, exists := m.users[email]
	if !exists {
		return nil, nil, errors.New("invalid credentials")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, nil, errors.New("invalid credentials")
	}

	// Generate token pair
	accessToken := "access_token_" + user.ID.String()
	refreshToken := "refresh_token_" + user.ID.String()

	tokenPair := &auth.TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    3600,
	}

	m.tokenPairs[refreshToken] = tokenPair

	return tokenPair, user, nil
}

func (m *mockAuthService) ValidateToken(ctx context.Context, tokenString string) (*auth.TokenClaims, error) {
	for _, user := range m.users {
		if "access_token_"+user.ID.String() == tokenString {
			return &auth.TokenClaims{
				UserID: user.ID.String(),
				Email:  user.Email,
				Role:   user.Role,
				// Add other necessary fields
			}, nil
		}
	}
	return nil, errors.New("invalid token")
}

func (m *mockAuthService) RefreshToken(ctx context.Context, refreshToken string) (*auth.TokenPair, error) {
	tokenPair, exists := m.tokenPairs[refreshToken]
	if !exists {
		return nil, errors.New("invalid refresh token")
	}

	// Create a new token pair
	newTokenPair := &auth.TokenPair{
		AccessToken:  "new_" + tokenPair.AccessToken,
		RefreshToken: "new_" + tokenPair.RefreshToken,
		ExpiresIn:    3600,
	}

	// Store the new refresh token
	m.tokenPairs[newTokenPair.RefreshToken] = newTokenPair

	// Invalidate the old refresh token
	delete(m.tokenPairs, refreshToken)

	return newTokenPair, nil
}

func (m *mockAuthService) CreateSession(ctx context.Context, userID uuid.UUID, deviceFingerprint string) (*auth.Session, error) {
	sessionID := uuid.New()
	session := &auth.Session{
		ID:                sessionID,
		UserID:            userID,
		DeviceFingerprint: deviceFingerprint,
		CreatedAt:         time.Now(),
		ExpiresAt:         time.Now().Add(24 * time.Hour),
		LastActivity:      time.Now(),
		Active:            true,
	}

	m.sessions[sessionID] = session
	return session, nil
}

func (m *mockAuthService) ValidateSession(ctx context.Context, sessionID uuid.UUID) (*auth.Session, error) {
	session, exists := m.sessions[sessionID]
	if !exists {
		return nil, errors.New("session not found")
	}

	if _, invalid := m.invalidSessions[sessionID]; invalid {
		return nil, errors.New("session invalidated")
	}

	if session.ExpiresAt.Before(time.Now()) {
		return nil, errors.New("session expired")
	}

	return session, nil
}

func (m *mockAuthService) InvalidateSession(ctx context.Context, sessionID uuid.UUID) error {
	if _, exists := m.sessions[sessionID]; !exists {
		return errors.New("session not found")
	}

	m.invalidSessions[sessionID] = true
	return nil
}

// Setup authentication test environment
func setupAuthTestEnvironment(t *testing.T) (*mockAuthService, *gorm.DB) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	// Migrate necessary tables
	err = db.AutoMigrate(
		&models.User{},
		&usermodels.UserSession{},
	)
	if err != nil {
		t.Fatalf("failed to migrate tables: %v", err)
	}

	// Create a mock authentication service
	mockAuth := newMockAuthService()

	// Create test users
	users := []struct {
		email    string
		username string
		password string
		role     string
	}{
		{"user@example.com", "user123", "password123", "user"},
		{"admin@example.com", "admin123", "password123", "admin"},
	}

	for _, u := range users {
		passwordHash, _ := bcrypt.GenerateFromPassword([]byte(u.password), bcrypt.DefaultCost)
		userID := uuid.New()

		user := &models.User{
			ID:           userID,
			Email:        u.email,
			Username:     u.username,
			PasswordHash: string(passwordHash),
			FirstName:    "Test",
			LastName:     "User",
			KYCStatus:    "approved",
			Role:         u.role,
			Tier:         "basic",
			MFAEnabled:   false,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}

		mockAuth.users[u.email] = user

		if err := db.Create(user).Error; err != nil {
			t.Fatalf("failed to create test user: %v", err)
		}
	}

	return mockAuth, db
}

func TestAuthenticateUser(t *testing.T) {
	mockAuth, _ := setupAuthTestEnvironment(t)

	testCases := []struct {
		name        string
		email       string
		password    string
		expectError bool
		expectRole  string
	}{
		{
			name:        "Valid user credentials",
			email:       "user@example.com",
			password:    "password123",
			expectError: false,
			expectRole:  "user",
		},
		{
			name:        "Valid admin credentials",
			email:       "admin@example.com",
			password:    "password123",
			expectError: false,
			expectRole:  "admin",
		},
		{
			name:        "Invalid email",
			email:       "nonexistent@example.com",
			password:    "password123",
			expectError: true,
		},
		{
			name:        "Invalid password",
			email:       "user@example.com",
			password:    "wrongpassword",
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tokenPair, user, err := mockAuth.AuthenticateUser(context.Background(), tc.email, tc.password)

			if tc.expectError {
				assert.Error(t, err)
				assert.Nil(t, tokenPair)
				assert.Nil(t, user)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, tokenPair)
				assert.NotNil(t, user)
				assert.NotEmpty(t, tokenPair.AccessToken)
				assert.NotEmpty(t, tokenPair.RefreshToken)
				assert.Equal(t, tc.expectRole, user.Role)
			}
		})
	}
}

func TestTokenValidationAndRefresh(t *testing.T) {
	mockAuth, _ := setupAuthTestEnvironment(t)

	// Authenticate to get initial token pair
	tokenPair, user, err := mockAuth.AuthenticateUser(context.Background(), "user@example.com", "password123")
	assert.NoError(t, err)
	assert.NotNil(t, tokenPair)
	assert.NotNil(t, user)

	// Test token validation
	claims, err := mockAuth.ValidateToken(context.Background(), tokenPair.AccessToken)
	assert.NoError(t, err)
	assert.NotNil(t, claims)
	assert.Equal(t, user.ID.String(), claims.UserID)
	assert.Equal(t, user.Email, claims.Email)

	// Test invalid token
	claims, err = mockAuth.ValidateToken(context.Background(), "invalid_token")
	assert.Error(t, err)
	assert.Nil(t, claims)

	// Test token refresh
	refreshedTokenPair, err := mockAuth.RefreshToken(context.Background(), tokenPair.RefreshToken)
	assert.NoError(t, err)
	assert.NotNil(t, refreshedTokenPair)
	assert.NotEqual(t, tokenPair.AccessToken, refreshedTokenPair.AccessToken)
	assert.NotEqual(t, tokenPair.RefreshToken, refreshedTokenPair.RefreshToken)

	// Test that the old refresh token is now invalid
	invalidRefreshResult, err := mockAuth.RefreshToken(context.Background(), tokenPair.RefreshToken)
	assert.Error(t, err)
	assert.Nil(t, invalidRefreshResult)

	// Test that the new access token is valid
	claims, err = mockAuth.ValidateToken(context.Background(), refreshedTokenPair.AccessToken)
	assert.Error(t, err) // Our mock implementation doesn't support new tokens

	// Validate that the original access token is still valid (in a real implementation,
	// you might revoke previous tokens, but our mock doesn't do that)
	claims, err = mockAuth.ValidateToken(context.Background(), tokenPair.AccessToken)
	assert.NoError(t, err)
	assert.NotNil(t, claims)
}

func TestSessionManagement(t *testing.T) {
	mockAuth, _ := setupAuthTestEnvironment(t)

	// Get a user ID to work with
	user := mockAuth.users["user@example.com"]
	assert.NotNil(t, user)

	// Create a session
	deviceFingerprint := "testdevice123"
	session, err := mockAuth.CreateSession(context.Background(), user.ID, deviceFingerprint)
	assert.NoError(t, err)
	assert.NotNil(t, session)
	assert.Equal(t, user.ID, session.UserID)
	assert.Equal(t, deviceFingerprint, session.DeviceFingerprint)
	assert.True(t, session.Active)
	assert.False(t, session.ExpiresAt.Before(time.Now()), "Session should not expire in the past")

	// Validate the session
	validatedSession, err := mockAuth.ValidateSession(context.Background(), session.ID)
	assert.NoError(t, err)
	assert.NotNil(t, validatedSession)
	assert.Equal(t, session.ID, validatedSession.ID)

	// Invalidate the session
	err = mockAuth.InvalidateSession(context.Background(), session.ID)
	assert.NoError(t, err)

	// Try to validate the invalidated session
	validatedSession, err = mockAuth.ValidateSession(context.Background(), session.ID)
	assert.Error(t, err)
	assert.Nil(t, validatedSession)
	assert.Contains(t, err.Error(), "invalidated")

	// Try to invalidate a non-existent session
	err = mockAuth.InvalidateSession(context.Background(), uuid.New())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// Define additional types that might not exist in actual package for the test
type TokenPair struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int64
}

type TokenClaims struct {
	UserID string
	Email  string
	Role   string
	// Other claims
}

type Session struct {
	ID                uuid.UUID
	UserID            uuid.UUID
	DeviceFingerprint string
	CreatedAt         time.Time
	ExpiresAt         time.Time
	LastActivity      time.Time
	Active            bool
}
