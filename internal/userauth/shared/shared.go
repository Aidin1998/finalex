package shared

import (
	"context"
	"time"

	auth "github.com/Aidin1998/pincex_unified/internal/userauth/auth"
	"github.com/Aidin1998/pincex_unified/pkg/models"
	"github.com/google/uuid"
)

// EnterpriseRegistrationRequest is a shared type for registration requests
type EnterpriseRegistrationRequest struct {
	Email    string                 `json:"email"`
	Password string                 `json:"password"`
	Phone    string                 `json:"phone"`
	Country  string                 `json:"country"`
	DOB      string                 `json:"dob"`
	FullName string                 `json:"full_name"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// AuditContext is a shared type for audit logging context
type AuditContext struct {
	UserID    string                 `json:"user_id,omitempty"`
	IPAddress string                 `json:"ip_address,omitempty"`
	UserAgent string                 `json:"user_agent,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// Claims struct to match admin/api.go and grpc usage
// Added ExpiresAt and KeyID for gRPC compatibility
// KeyID is for API key claims, ExpiresAt is for token expiry
// ExpiresAt is a Unix timestamp (int64)
type Claims struct {
	UserID      uuid.UUID
	Email       string
	Roles       []string
	Permissions []string
	ExpiresAt   int64      // Unix timestamp for token expiry
	KeyID       *uuid.UUID // Optional: for API key claims
}

// APIKeyResponse struct to match admin/api.go usage
type APIKeyResponse struct {
	ID  string `json:"id"`
	Key string `json:"key"`
}

// UserAuthService is a shared interface for user authentication service methods used by admin and grpc
// All methods use concrete types for gRPC compatibility
// Example:
type UserAuthService interface {
	IdentityService() IdentityService
	RegisterUserWithCompliance(ctx context.Context, req *EnterpriseRegistrationRequest) (*RegisterUserResponse, error)
	AssignRole(ctx context.Context, userID uuid.UUID, role string) error
	RevokeRole(ctx context.Context, userID uuid.UUID, roleID string) error
	GetUserPermissions(ctx context.Context, userID uuid.UUID) ([]auth.Permission, error)
	AuditService() AuditService
	ValidateToken(ctx context.Context, token string) (*Claims, error)
	CreateAPIKey(ctx context.Context, userID uuid.UUID, name string, permissions []string, expiresAt *time.Time) (*auth.APIKey, error)
	RefreshToken(ctx context.Context, refreshToken string) (*auth.TokenPair, error)
	ValidateAPIKey(ctx context.Context, apiKey string) (*auth.APIKeyClaims, error)
	CheckRateLimit(ctx context.Context, userID, endpoint, clientIP string) (*auth.RateLimitResult, error)
	GetUserRateLimitStatus(ctx context.Context, userID string) (map[string]*models.RateLimitInfo, error)
}

// IdentityService interface for user management
// Add all methods used in admin/api.go
// (Stub implementations, to be replaced by real ones)
type IdentityService interface {
	ListUsers(ctx context.Context, page, limit int, search, status string) ([]interface{}, int, error)
	GetUserByID(ctx context.Context, userID uuid.UUID) (interface{}, error)
	UpdateUser(ctx context.Context, userID uuid.UUID, data map[string]interface{}) error
	DeleteUser(ctx context.Context, userID uuid.UUID) error
	UpdateUserStatus(ctx context.Context, userID uuid.UUID, isActive bool) error
	VerifyUserEmail(ctx context.Context, userID uuid.UUID) error
}

type RegisterUserResponse struct {
	UserID uuid.UUID `json:"user_id"`
}

type AuditService interface {
	LogEvent(ctx context.Context, event, severity string, ctxData AuditContext, message string)
}
