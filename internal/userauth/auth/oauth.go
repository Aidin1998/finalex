package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Aidin1998/finalex/pkg/models"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// InitiateOAuthFlow initiates an OAuth flow
func (s *Service) InitiateOAuthFlow(ctx context.Context, provider, redirectURI string) (*OAuthState, error) {
	_, exists := s.oauthProviders[provider]
	if !exists {
		return nil, fmt.Errorf("unsupported OAuth provider: %s", provider)
	}

	// Generate secure state
	stateBytes := make([]byte, 32)
	if _, err := rand.Read(stateBytes); err != nil {
		return nil, fmt.Errorf("failed to generate state: %w", err)
	}
	state := base64.URLEncoding.EncodeToString(stateBytes)

	// Store state in database with expiration
	oauthState := &OAuthState{
		State:       state,
		Provider:    provider,
		RedirectURI: redirectURI,
		ExpiresAt:   time.Now().Add(10 * time.Minute),
	}

	// Store state temporarily
	stateJSON, _ := json.Marshal(oauthState)
	err := s.db.Exec(`
		INSERT INTO oauth_states (state, data, expires_at) 
		VALUES (?, ?, ?)
	`, state, string(stateJSON), oauthState.ExpiresAt).Error
	if err != nil {
		return nil, fmt.Errorf("failed to store OAuth state: %w", err)
	}

	return oauthState, nil
}

// HandleOAuthCallback handles OAuth callback
func (s *Service) HandleOAuthCallback(ctx context.Context, state, code string) (*TokenPair, error) {
	// Validate and retrieve state
	var stateData string
	err := s.db.Raw(`
		SELECT data FROM oauth_states 
		WHERE state = ? AND expires_at > ?
	`, state, time.Now()).Scan(&stateData).Error
	if err != nil {
		return nil, fmt.Errorf("invalid or expired OAuth state")
	}

	var oauthState OAuthState
	if err := json.Unmarshal([]byte(stateData), &oauthState); err != nil {
		return nil, fmt.Errorf("invalid OAuth state data")
	}

	// Get OAuth provider config
	provider, exists := s.oauthProviders[oauthState.Provider]
	if !exists {
		return nil, fmt.Errorf("OAuth provider not configured: %s", oauthState.Provider)
	}

	// Exchange code for access token
	tokenResp, err := s.exchangeOAuthCode(provider, code, oauthState.RedirectURI)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange OAuth code: %w", err)
	}

	// Get user info from provider
	userInfo, err := s.getOAuthUserInfo(provider, tokenResp.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("failed to get user info: %w", err)
	}

	// Find or create user
	user, err := s.findOrCreateOAuthUser(userInfo, oauthState.Provider)
	if err != nil {
		return nil, fmt.Errorf("failed to find or create user: %w", err)
	}

	// Create session
	session, err := s.CreateSession(ctx, user.ID, "oauth")
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}
	// Convert to models.User for token generation
	modelUser := models.User{
		ID:    user.ID,
		Email: user.Email,
		Role:  user.Role,
	}

	// Generate token pair
	tokenPair, err := s.generateTokenPair(modelUser, session.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to generate tokens: %w", err)
	}
	// Clean up OAuth state
	s.db.Exec("DELETE FROM oauth_states WHERE state = ?", state)

	s.logger.Info("OAuth login successful",
		zap.String("user_id", user.ID.String()),
		zap.String("provider", oauthState.Provider))

	return tokenPair, nil
}

// LinkOAuthAccount links an OAuth account to an existing user
func (s *Service) LinkOAuthAccount(ctx context.Context, userID uuid.UUID, provider, oauthUserID string) error {
	// Check if OAuth account is already linked
	var count int64
	err := s.db.Raw(`
		SELECT COUNT(*) FROM oauth_accounts 
		WHERE provider = ? AND provider_user_id = ?
	`, provider, oauthUserID).Scan(&count).Error
	if err != nil {
		return fmt.Errorf("failed to check existing OAuth account: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("OAuth account already linked")
	}
	// Link OAuth account
	err = s.db.Exec(`
		INSERT INTO oauth_accounts (id, user_id, provider, provider_user_id, created_at) 
		VALUES (?, ?, ?, ?, ?)
	`, uuid.New(), userID, provider, oauthUserID, time.Now()).Error
	if err != nil {
		return fmt.Errorf("failed to link OAuth account: %w", err)
	}

	s.logger.Info("OAuth account linked",
		zap.String("user_id", userID.String()),
		zap.String("provider", provider),
		zap.String("provider_user_id", oauthUserID))

	return nil
}

// RegisterTrustedDevice registers a trusted device for a user
func (s *Service) RegisterTrustedDevice(ctx context.Context, userID uuid.UUID, deviceFingerprint string) error {
	// Check if device is already trusted
	var count int64
	err := s.db.Raw(`
		SELECT COUNT(*) FROM trusted_devices 
		WHERE user_id = ? AND device_fingerprint = ?
	`, userID, deviceFingerprint).Scan(&count).Error
	if err != nil {
		return fmt.Errorf("failed to check trusted device: %w", err)
	}
	if count > 0 {
		return nil // Already trusted
	}

	// Add trusted device
	err = s.db.Exec(`
		INSERT INTO trusted_devices (id, user_id, device_fingerprint, created_at) 
		VALUES (?, ?, ?, ?)
	`, uuid.New(), userID, deviceFingerprint, time.Now()).Error
	if err != nil {
		return fmt.Errorf("failed to register trusted device: %w", err)
	}

	s.logger.Info("Trusted device registered",
		zap.String("user_id", userID.String()),
		zap.String("device_fingerprint", deviceFingerprint))

	return nil
}

// IsTrustedDevice checks if a device is trusted for a user
func (s *Service) IsTrustedDevice(ctx context.Context, userID uuid.UUID, deviceFingerprint string) (bool, error) {
	var count int64
	err := s.db.Raw(`
		SELECT COUNT(*) FROM trusted_devices 
		WHERE user_id = ? AND device_fingerprint = ?
	`, userID, deviceFingerprint).Scan(&count).Error
	if err != nil {
		return false, fmt.Errorf("failed to check trusted device: %w", err)
	}

	return count > 0, nil
}

// RevokeTrustedDevice revokes a trusted device
func (s *Service) RevokeTrustedDevice(ctx context.Context, userID uuid.UUID, deviceFingerprint string) error {
	result := s.db.Exec(`
		DELETE FROM trusted_devices 
		WHERE user_id = ? AND device_fingerprint = ?
	`, userID, deviceFingerprint)
	if result.Error != nil {
		return fmt.Errorf("failed to revoke trusted device: %w", result.Error)
	}

	s.logger.Info("Trusted device revoked",
		zap.String("user_id", userID.String()),
		zap.String("device_fingerprint", deviceFingerprint))

	return nil
}

// AddOAuthProvider adds an OAuth provider configuration
func (s *Service) AddOAuthProvider(name string, provider *OAuthProvider) {
	s.oauthProviders[name] = provider
}

// OAuth token response structure
type oauthTokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
}

// OAuth user info structure
type oauthUserInfo struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	Name     string `json:"name"`
	Username string `json:"username"`
}

// exchangeOAuthCode exchanges authorization code for access token
func (s *Service) exchangeOAuthCode(provider *OAuthProvider, code, redirectURI string) (*oauthTokenResponse, error) {
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("client_id", provider.ClientID)
	data.Set("client_secret", provider.ClientSecret)
	data.Set("code", code)
	data.Set("redirect_uri", redirectURI)

	req, err := http.NewRequest("POST", provider.TokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OAuth token exchange failed with status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read token response: %w", err)
	}

	var tokenResp oauthTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	return &tokenResp, nil
}

// getOAuthUserInfo gets user information from OAuth provider
func (s *Service) getOAuthUserInfo(provider *OAuthProvider, accessToken string) (*oauthUserInfo, error) {
	req, err := http.NewRequest("GET", provider.UserInfoURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create user info request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get user info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("user info request failed with status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read user info response: %w", err)
	}

	var userInfo oauthUserInfo
	if err := json.Unmarshal(body, &userInfo); err != nil {
		return nil, fmt.Errorf("failed to parse user info response: %w", err)
	}

	return &userInfo, nil
}

// findOrCreateOAuthUser finds existing user or creates new one from OAuth info
func (s *Service) findOrCreateOAuthUser(userInfo *oauthUserInfo, provider string) (*struct {
	ID    uuid.UUID
	Email string
	Role  string
}, error) {
	// First, try to find existing OAuth account
	var user struct {
		ID    uuid.UUID
		Email string
		Role  string
	}

	err := s.db.Raw(`
		SELECT u.id, u.email, u.role 
		FROM users u 
		JOIN oauth_accounts oa ON u.id = oa.user_id 
		WHERE oa.provider = ? AND oa.provider_user_id = ?
	`, provider, userInfo.ID).Scan(&user).Error

	if err == nil {
		// Existing OAuth user found
		return &user, nil
	}

	// Try to find user by email
	err = s.db.Raw(`
		SELECT id, email, role FROM users WHERE email = ?
	`, userInfo.Email).Scan(&user).Error

	if err == nil {
		// User exists with same email, link OAuth account
		err = s.LinkOAuthAccount(context.Background(), user.ID, provider, userInfo.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to link OAuth account: %w", err)
		}
		return &user, nil
	}

	// Create new user
	newUser := struct {
		ID       uuid.UUID
		Email    string
		Username string
		Role     string
	}{
		ID:       uuid.New(),
		Email:    userInfo.Email,
		Username: userInfo.Username,
		Role:     "user",
	}

	// Generate random username if not provided
	if newUser.Username == "" {
		newUser.Username = fmt.Sprintf("user_%s", newUser.ID.String()[:8])
	}

	// Begin transaction
	tx := s.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Create user
	err = tx.Exec(`
		INSERT INTO users (id, email, username, password_hash, role, created_at, updated_at) 
		VALUES (?, ?, ?, '', ?, ?, ?)
	`, newUser.ID, newUser.Email, newUser.Username, newUser.Role, time.Now(), time.Now()).Error
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	// Link OAuth account
	err = tx.Exec(`
		INSERT INTO oauth_accounts (id, user_id, provider, provider_user_id, created_at) 
		VALUES (?, ?, ?, ?, ?)
	`, uuid.New(), newUser.ID, provider, userInfo.ID, time.Now()).Error
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to create OAuth account: %w", err)
	}
	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	s.logger.Info("New OAuth user created",
		zap.String("user_id", newUser.ID.String()),
		zap.String("email", newUser.Email),
		zap.String("provider", provider))

	return &struct {
		ID    uuid.UUID
		Email string
		Role  string
	}{
		ID:    newUser.ID,
		Email: newUser.Email,
		Role:  newUser.Role,
	}, nil
}

// CreateOAuthTables creates the necessary database tables for OAuth
func (s *Service) CreateOAuthTables() error {
	// OAuth states table (temporary storage for OAuth flow)
	err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS oauth_states (
			state TEXT PRIMARY KEY,
			data TEXT NOT NULL,
			expires_at TIMESTAMP NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`).Error
	if err != nil {
		return fmt.Errorf("failed to create oauth_states table: %w", err)
	}

	// OAuth accounts table
	err = s.db.Exec(`
		CREATE TABLE IF NOT EXISTS oauth_accounts (
			id UUID PRIMARY KEY,
			user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			provider TEXT NOT NULL,
			provider_user_id TEXT NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(provider, provider_user_id),
			INDEX idx_oauth_accounts_user_id (user_id)
		)
	`).Error
	if err != nil {
		return fmt.Errorf("failed to create oauth_accounts table: %w", err)
	}

	// Trusted devices table
	err = s.db.Exec(`
		CREATE TABLE IF NOT EXISTS trusted_devices (
			id UUID PRIMARY KEY,
			user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			device_fingerprint TEXT NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(user_id, device_fingerprint),
			INDEX idx_trusted_devices_user_id (user_id)
		)
	`).Error
	if err != nil {
		return fmt.Errorf("failed to create trusted_devices table: %w", err)
	}

	return nil
}
