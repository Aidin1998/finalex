// Package contracts defines unified interface contracts for module integration
package contracts

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// UserAuthServiceContract defines the standardized interface for userauth module
type UserAuthServiceContract interface {
	// Authentication & Authorization
	ValidateToken(ctx context.Context, token string) (*AuthClaims, error)
	ValidateAPIKey(ctx context.Context, apiKey string) (*APIKeyClaims, error)
	CheckPermission(ctx context.Context, userID uuid.UUID, resource, action string) error
	GetUserPermissions(ctx context.Context, userID uuid.UUID) ([]Permission, error)

	// User Management
	GetUser(ctx context.Context, userID uuid.UUID) (*User, error)
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	ValidateKYCLevel(ctx context.Context, userID uuid.UUID, requiredLevel int) error
	GetKYCStatus(ctx context.Context, userID uuid.UUID) (*KYCStatus, error)

	// Session Management
	CreateSession(ctx context.Context, userID uuid.UUID, metadata map[string]string) (*Session, error)
	ValidateSession(ctx context.Context, sessionID uuid.UUID) (*Session, error)
	InvalidateSession(ctx context.Context, sessionID uuid.UUID) error

	// Rate Limiting
	CheckRateLimit(ctx context.Context, userID uuid.UUID, action string) (*RateLimitResult, error)

	// Service Health
	HealthCheck(ctx context.Context) (*HealthStatus, error)
}

// AuthClaims represents validated token claims
type AuthClaims struct {
	UserID      uuid.UUID `json:"user_id"`
	Email       string    `json:"email"`
	Role        string    `json:"role"`
	Permissions []string  `json:"permissions"`
	KYCLevel    int       `json:"kyc_level"`
	SessionID   uuid.UUID `json:"session_id"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// APIKeyClaims represents validated API key claims
type APIKeyClaims struct {
	KeyID       uuid.UUID  `json:"key_id"`
	UserID      uuid.UUID  `json:"user_id"`
	Permissions []string   `json:"permissions"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
}

// Permission represents a user permission
type Permission struct {
	Resource string `json:"resource"`
	Action   string `json:"action"`
	Scope    string `json:"scope,omitempty"`
}

// User represents user information
type User struct {
	ID          uuid.UUID  `json:"id"`
	Email       string     `json:"email"`
	FirstName   string     `json:"first_name"`
	LastName    string     `json:"last_name"`
	Role        string     `json:"role"`
	KYCLevel    int        `json:"kyc_level"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
}

// KYCStatus represents KYC verification status
type KYCStatus struct {
	UserID       uuid.UUID  `json:"user_id"`
	Level        int        `json:"level"`
	Status       string     `json:"status"`
	VerifiedAt   *time.Time `json:"verified_at,omitempty"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	Restrictions []string   `json:"restrictions,omitempty"`
}

// Session represents user session
type Session struct {
	ID        uuid.UUID         `json:"id"`
	UserID    uuid.UUID         `json:"user_id"`
	IPAddress string            `json:"ip_address"`
	UserAgent string            `json:"user_agent"`
	Metadata  map[string]string `json:"metadata"`
	CreatedAt time.Time         `json:"created_at"`
	ExpiresAt time.Time         `json:"expires_at"`
	LastSeen  time.Time         `json:"last_seen"`
}

// RateLimitResult represents rate limiting check result
type RateLimitResult struct {
	Allowed    bool           `json:"allowed"`
	Remaining  int            `json:"remaining"`
	ResetAt    time.Time      `json:"reset_at"`
	RetryAfter *time.Duration `json:"retry_after,omitempty"`
}

// HealthStatus represents service health
type HealthStatus struct {
	Status       string                 `json:"status"`
	Timestamp    time.Time              `json:"timestamp"`
	Version      string                 `json:"version"`
	Uptime       time.Duration          `json:"uptime"`
	Metrics      map[string]interface{} `json:"metrics"`
	Dependencies map[string]string      `json:"dependencies"`
}
