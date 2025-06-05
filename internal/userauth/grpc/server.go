package grpc

import (
	"context"
	"time"

	"github.com/Aidin1998/finalex/internal/userauth/shared"
	"github.com/Aidin1998/finalex/pkg/models"
	userauthpb "github.com/Aidin1998/finalex/pkg/proto/userauth"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Server implements the gRPC UserAuthService
type Server struct {
	userauthpb.UnimplementedUserAuthServiceServer
	userAuthService shared.UserAuthService
	logger          *zap.Logger
}

// NewServer creates a new gRPC server for UserAuth service
func NewServer(userAuthService shared.UserAuthService, logger *zap.Logger) *Server {
	return &Server{
		userAuthService: userAuthService,
		logger:          logger,
	}
}

// ValidateToken validates a JWT token
func (s *Server) ValidateToken(ctx context.Context, req *userauthpb.ValidateTokenRequest) (*userauthpb.ValidateTokenResponse, error) {
	claims, err := s.userAuthService.ValidateToken(ctx, req.Token)
	if err != nil {
		s.logger.Warn("Token validation failed", zap.Error(err))
		return &userauthpb.ValidateTokenResponse{
			Valid:        false,
			ErrorMessage: err.Error(),
		}, nil
	}

	return &userauthpb.ValidateTokenResponse{
		Valid:       true,
		UserId:      claims.UserID.String(),
		Email:       claims.Email,
		Roles:       claims.Roles,
		Permissions: claims.Permissions,
		ExpiresAt:   timestamppb.New(time.Unix(claims.ExpiresAt, 0)),
	}, nil
}

// RefreshToken refreshes an access token
func (s *Server) RefreshToken(ctx context.Context, req *userauthpb.RefreshTokenRequest) (*userauthpb.RefreshTokenResponse, error) {
	tokenPair, err := s.userAuthService.RefreshToken(ctx, req.RefreshToken)
	if err != nil {
		s.logger.Warn("Token refresh failed", zap.Error(err))
		return &userauthpb.RefreshTokenResponse{
			Success:      false,
			ErrorMessage: err.Error(),
		}, nil
	}

	return &userauthpb.RefreshTokenResponse{
		Success:      true,
		AccessToken:  tokenPair.AccessToken,
		RefreshToken: tokenPair.RefreshToken,
		ExpiresAt:    timestamppb.New(tokenPair.ExpiresAt),
	}, nil
}

// CreateAPIKey creates a new API key
func (s *Server) CreateAPIKey(ctx context.Context, req *userauthpb.CreateAPIKeyRequest) (*userauthpb.CreateAPIKeyResponse, error) {
	userID, err := uuid.Parse(req.UserId)
	if err != nil {
		return &userauthpb.CreateAPIKeyResponse{
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
		return &userauthpb.CreateAPIKeyResponse{
			Success:      false,
			ErrorMessage: err.Error(),
		}, nil
	}

	return &userauthpb.CreateAPIKeyResponse{
		Success: true,
		ApiKey:  apiKey.Key,
		KeyId:   apiKey.ID.String(),
	}, nil
}

// ValidateAPIKey validates an API key
func (s *Server) ValidateAPIKey(ctx context.Context, req *userauthpb.ValidateAPIKeyRequest) (*userauthpb.ValidateAPIKeyResponse, error) {
	claims, err := s.userAuthService.ValidateAPIKey(ctx, req.ApiKey)
	if err != nil {
		s.logger.Warn("API key validation failed", zap.Error(err))
		return &userauthpb.ValidateAPIKeyResponse{
			Valid:        false,
			ErrorMessage: err.Error(),
		}, nil
	}

	return &userauthpb.ValidateAPIKeyResponse{
		Valid:       true,
		UserId:      claims.UserID.String(),
		KeyId:       claims.KeyID.String(),
		Permissions: claims.Permissions,
		ExpiresAt:   timestamppb.New(claims.ExpiresAt),
	}, nil
}

// GetUser retrieves user information
func (s *Server) GetUser(ctx context.Context, req *userauthpb.GetUserRequest) (*userauthpb.GetUserResponse, error) {
	userID, err := uuid.Parse(req.UserId)
	if err != nil {
		return &userauthpb.GetUserResponse{
			Found:        false,
			ErrorMessage: "invalid user ID",
		}, nil
	}

	// Use identity service to get user
	user, err := s.userAuthService.IdentityService().GetUserByID(ctx, userID)
	if err != nil {
		s.logger.Warn("User lookup failed", zap.String("user_id", req.UserId), zap.Error(err))
		return &userauthpb.GetUserResponse{
			Found:        false,
			ErrorMessage: err.Error(),
		}, nil
	}

	// Fix: Cast user to *models.User for correct field access
	userObj, ok := user.(*models.User)
	if !ok {
		s.logger.Warn("User type assertion failed", zap.String("user_id", req.UserId))
		return &userauthpb.GetUserResponse{
			Found:        false,
			ErrorMessage: "internal type error",
		}, nil
	}
	return &userauthpb.GetUserResponse{
		Found: true,
		User: &userauthpb.User{
			Id:            userObj.ID.String(),
			Email:         userObj.Email,
			Username:      userObj.Username,
			IsActive:      userObj.MFAEnabled, // or userObj.IsActive if present
			EmailVerified: false,              // Set appropriately if available
			CreatedAt:     timestamppb.New(userObj.CreatedAt),
			UpdatedAt:     timestamppb.New(userObj.UpdatedAt),
		},
	}, nil
}

// GetUserPermissions retrieves user permissions
func (s *Server) GetUserPermissions(ctx context.Context, req *userauthpb.GetUserPermissionsRequest) (*userauthpb.GetUserPermissionsResponse, error) {
	userID, err := uuid.Parse(req.UserId)
	if err != nil {
		return &userauthpb.GetUserPermissionsResponse{
			Success:      false,
			ErrorMessage: "invalid user ID",
		}, nil
	}

	permissions, err := s.userAuthService.GetUserPermissions(ctx, userID)
	if err != nil {
		s.logger.Warn("Failed to get user permissions", zap.String("user_id", req.UserId), zap.Error(err))
		return &userauthpb.GetUserPermissionsResponse{
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

	return &userauthpb.GetUserPermissionsResponse{
		Success:     true,
		Permissions: permissionStrings,
		Roles:       roles,
	}, nil
}

// CheckUserRole checks if user has a specific role
func (s *Server) CheckUserRole(ctx context.Context, req *userauthpb.CheckUserRoleRequest) (*userauthpb.CheckUserRoleResponse, error) {
	userID, err := uuid.Parse(req.UserId)
	if err != nil {
		return &userauthpb.CheckUserRoleResponse{
			HasRole:      false,
			ErrorMessage: "invalid user ID",
		}, nil
	}

	permissions, err := s.userAuthService.GetUserPermissions(ctx, userID)
	if err != nil {
		s.logger.Warn("Failed to get user permissions for role check", zap.String("user_id", req.UserId), zap.Error(err))
		return &userauthpb.CheckUserRoleResponse{
			HasRole:      false,
			ErrorMessage: err.Error(),
		}, nil
	}

	// Check if user has the requested role
	for _, perm := range permissions {
		if perm.Role == req.Role {
			return &userauthpb.CheckUserRoleResponse{
				HasRole: true,
			}, nil
		}
	}

	return &userauthpb.CheckUserRoleResponse{
		HasRole: false,
	}, nil
}

// CheckRateLimit checks rate limiting for a request
func (s *Server) CheckRateLimit(ctx context.Context, req *userauthpb.CheckRateLimitRequest) (*userauthpb.CheckRateLimitResponse, error) {
	result, err := s.userAuthService.CheckRateLimit(ctx, req.UserId, req.Endpoint, req.ClientIp)
	if err != nil {
		s.logger.Warn("Rate limit check failed", zap.String("user_id", req.UserId), zap.Error(err))
		return &userauthpb.CheckRateLimitResponse{
			Allowed:      false,
			ErrorMessage: err.Error(),
		}, nil
	}

	return &userauthpb.CheckRateLimitResponse{
		Allowed:   result.Allowed,
		Remaining: int32(result.Remaining),
		ResetTime: int32(result.ResetTime),
		Tier:      result.Tier,
	}, nil
}

// GetRateLimitStatus gets rate limit status for a user
func (s *Server) GetRateLimitStatus(ctx context.Context, req *userauthpb.GetRateLimitStatusRequest) (*userauthpb.GetRateLimitStatusResponse, error) {
	status, err := s.userAuthService.GetUserRateLimitStatus(ctx, req.UserId)
	if err != nil {
		s.logger.Warn("Failed to get rate limit status", zap.String("user_id", req.UserId), zap.Error(err))
		return &userauthpb.GetRateLimitStatusResponse{
			Success:      false,
			ErrorMessage: err.Error(),
		}, nil
	}

	limits := make(map[string]*userauthpb.RateLimitInfo)
	for endpoint, info := range status {
		limits[endpoint] = &userauthpb.RateLimitInfo{
			Remaining: int32(info.Remaining),
			ResetTime: int32(info.ResetTime),
			Tier:      info.Tier,
		}
	}

	return &userauthpb.GetRateLimitStatusResponse{
		Success: true,
		Limits:  limits,
	}, nil
}

// HealthCheck performs a health check
func (s *Server) HealthCheck(ctx context.Context, req *userauthpb.HealthCheckRequest) (*userauthpb.HealthCheckResponse, error) {
	return &userauthpb.HealthCheckResponse{
		Healthy:   true,
		Version:   "1.0.0",
		Timestamp: timestamppb.Now(),
	}, nil
}
