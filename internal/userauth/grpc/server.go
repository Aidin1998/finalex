package grpc

import (
	"context"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/userauth"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Server implements the gRPC UserAuthService
type Server struct {
	UnimplementedUserAuthServiceServer
	userAuthService *userauth.Service
	logger          *zap.Logger
}

// NewServer creates a new gRPC server for UserAuth service
func NewServer(userAuthService *userauth.Service, logger *zap.Logger) *Server {
	return &Server{
		userAuthService: userAuthService,
		logger:          logger,
	}
}

// ValidateToken validates a JWT token
func (s *Server) ValidateToken(ctx context.Context, req *ValidateTokenRequest) (*ValidateTokenResponse, error) {
	claims, err := s.userAuthService.ValidateToken(ctx, req.Token)
	if err != nil {
		s.logger.Warn("Token validation failed", zap.Error(err))
		return &ValidateTokenResponse{
			Valid:        false,
			ErrorMessage: err.Error(),
		}, nil
	}

	return &ValidateTokenResponse{
		Valid:       true,
		UserId:      claims.UserID.String(),
		Email:       claims.Email,
		Roles:       claims.Roles,
		Permissions: claims.Permissions,
		ExpiresAt:   timestamppb.New(time.Unix(claims.ExpiresAt, 0)),
	}, nil
}

// RefreshToken refreshes an access token
func (s *Server) RefreshToken(ctx context.Context, req *RefreshTokenRequest) (*RefreshTokenResponse, error) {
	tokenPair, err := s.userAuthService.RefreshToken(ctx, req.RefreshToken)
	if err != nil {
		s.logger.Warn("Token refresh failed", zap.Error(err))
		return &RefreshTokenResponse{
			Success:      false,
			ErrorMessage: err.Error(),
		}, nil
	}

	return &RefreshTokenResponse{
		Success:      true,
		AccessToken:  tokenPair.AccessToken,
		RefreshToken: tokenPair.RefreshToken,
		ExpiresAt:    timestamppb.New(tokenPair.ExpiresAt),
	}, nil
}

// CreateAPIKey creates a new API key
func (s *Server) CreateAPIKey(ctx context.Context, req *CreateAPIKeyRequest) (*CreateAPIKeyResponse, error) {
	userID, err := uuid.Parse(req.UserId)
	if err != nil {
		return &CreateAPIKeyResponse{
			Success:      false,
			ErrorMessage: "invalid user ID",
		}, nil
	}

	var expiresAt *time.Time
	if req.ExpiresAt != nil {
		t := req.ExpiresAt.AsTime()
		expiresAt = &t
	}

	apiKey, err := s.userAuthService.CreateAPIKey(ctx, userID, req.Name, req.Permissions, expiresAt)
	if err != nil {
		s.logger.Warn("API key creation failed", zap.Error(err))
		return &CreateAPIKeyResponse{
			Success:      false,
			ErrorMessage: err.Error(),
		}, nil
	}

	return &CreateAPIKeyResponse{
		Success: true,
		ApiKey:  apiKey.Key,
		KeyId:   apiKey.ID.String(),
	}, nil
}

// ValidateAPIKey validates an API key
func (s *Server) ValidateAPIKey(ctx context.Context, req *ValidateAPIKeyRequest) (*ValidateAPIKeyResponse, error) {
	claims, err := s.userAuthService.ValidateAPIKey(ctx, req.ApiKey)
	if err != nil {
		s.logger.Warn("API key validation failed", zap.Error(err))
		return &ValidateAPIKeyResponse{
			Valid:        false,
			ErrorMessage: err.Error(),
		}, nil
	}

	return &ValidateAPIKeyResponse{
		Valid:       true,
		UserId:      claims.UserID.String(),
		KeyId:       claims.KeyID.String(),
		Permissions: claims.Permissions,
		ExpiresAt:   timestamppb.New(claims.ExpiresAt),
	}, nil
}

// GetUser retrieves user information
func (s *Server) GetUser(ctx context.Context, req *GetUserRequest) (*GetUserResponse, error) {
	userID, err := uuid.Parse(req.UserId)
	if err != nil {
		return &GetUserResponse{
			Found:        false,
			ErrorMessage: "invalid user ID",
		}, nil
	}

	// Use identity service to get user
	user, err := s.userAuthService.IdentityService().GetUserByID(ctx, userID)
	if err != nil {
		s.logger.Warn("User lookup failed", zap.String("user_id", req.UserId), zap.Error(err))
		return &GetUserResponse{
			Found:        false,
			ErrorMessage: err.Error(),
		}, nil
	}

	return &GetUserResponse{
		Found: true,
		User: &User{
			Id:            user.ID.String(),
			Email:         user.Email,
			Username:      user.Username,
			IsActive:      user.IsActive,
			EmailVerified: user.EmailVerified,
			CreatedAt:     timestamppb.New(user.CreatedAt),
			UpdatedAt:     timestamppb.New(user.UpdatedAt),
		},
	}, nil
}

// GetUserPermissions retrieves user permissions
func (s *Server) GetUserPermissions(ctx context.Context, req *GetUserPermissionsRequest) (*GetUserPermissionsResponse, error) {
	userID, err := uuid.Parse(req.UserId)
	if err != nil {
		return &GetUserPermissionsResponse{
			Success:      false,
			ErrorMessage: "invalid user ID",
		}, nil
	}

	permissions, err := s.userAuthService.GetUserPermissions(ctx, userID)
	if err != nil {
		s.logger.Warn("Failed to get user permissions", zap.String("user_id", req.UserId), zap.Error(err))
		return &GetUserPermissionsResponse{
			Success:      false,
			ErrorMessage: err.Error(),
		}, nil
	}

	permissionStrings := make([]string, len(permissions))
	roles := make([]string, 0)
	for i, perm := range permissions {
		permissionStrings[i] = perm.Name
		// Extract unique roles
		if perm.Role != "" {
			found := false
			for _, role := range roles {
				if role == perm.Role {
					found = true
					break
				}
			}
			if !found {
				roles = append(roles, perm.Role)
			}
		}
	}

	return &GetUserPermissionsResponse{
		Success:     true,
		Permissions: permissionStrings,
		Roles:       roles,
	}, nil
}

// CheckUserRole checks if user has a specific role
func (s *Server) CheckUserRole(ctx context.Context, req *CheckUserRoleRequest) (*CheckUserRoleResponse, error) {
	userID, err := uuid.Parse(req.UserId)
	if err != nil {
		return &CheckUserRoleResponse{
			HasRole:      false,
			ErrorMessage: "invalid user ID",
		}, nil
	}

	permissions, err := s.userAuthService.GetUserPermissions(ctx, userID)
	if err != nil {
		s.logger.Warn("Failed to get user permissions for role check", zap.String("user_id", req.UserId), zap.Error(err))
		return &CheckUserRoleResponse{
			HasRole:      false,
			ErrorMessage: err.Error(),
		}, nil
	}

	// Check if user has the requested role
	for _, perm := range permissions {
		if perm.Role == req.Role {
			return &CheckUserRoleResponse{
				HasRole: true,
			}, nil
		}
	}

	return &CheckUserRoleResponse{
		HasRole: false,
	}, nil
}

// CheckRateLimit checks rate limiting for a request
func (s *Server) CheckRateLimit(ctx context.Context, req *CheckRateLimitRequest) (*CheckRateLimitResponse, error) {
	result, err := s.userAuthService.CheckRateLimit(ctx, req.UserId, req.Endpoint, req.ClientIp)
	if err != nil {
		s.logger.Warn("Rate limit check failed", zap.String("user_id", req.UserId), zap.Error(err))
		return &CheckRateLimitResponse{
			Allowed:      false,
			ErrorMessage: err.Error(),
		}, nil
	}

	return &CheckRateLimitResponse{
		Allowed:   result.Allowed,
		Remaining: int32(result.Remaining),
		ResetTime: int32(result.ResetTime),
		Tier:      result.Tier,
	}, nil
}

// GetRateLimitStatus gets rate limit status for a user
func (s *Server) GetRateLimitStatus(ctx context.Context, req *GetRateLimitStatusRequest) (*GetRateLimitStatusResponse, error) {
	status, err := s.userAuthService.GetUserRateLimitStatus(ctx, req.UserId)
	if err != nil {
		s.logger.Warn("Failed to get rate limit status", zap.String("user_id", req.UserId), zap.Error(err))
		return &GetRateLimitStatusResponse{
			Success:      false,
			ErrorMessage: err.Error(),
		}, nil
	}

	limits := make(map[string]*RateLimitInfo)
	for endpoint, info := range status {
		limits[endpoint] = &RateLimitInfo{
			Remaining: int32(info.Remaining),
			ResetTime: int32(info.ResetTime),
			Tier:      info.Tier,
		}
	}

	return &GetRateLimitStatusResponse{
		Success: true,
		Limits:  limits,
	}, nil
}

// HealthCheck performs a health check
func (s *Server) HealthCheck(ctx context.Context, req *HealthCheckRequest) (*HealthCheckResponse, error) {
	return &HealthCheckResponse{
		Healthy:   true,
		Version:   "1.0.0",
		Timestamp: timestamppb.Now(),
	}, nil
}
