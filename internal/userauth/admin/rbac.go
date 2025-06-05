package admin

import (
	"context"

	"github.com/Aidin1998/finalex/internal/userauth/shared"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// RBACService provides role-based access control functionality
type RBACService struct {
	userAuthService shared.UserAuthService
	logger          *zap.Logger
}

// NewRBACService creates a new RBAC service
func NewRBACService(userAuthService shared.UserAuthService, logger *zap.Logger) *RBACService {
	return &RBACService{
		userAuthService: userAuthService,
		logger:          logger,
	}
}

// HasPermission checks if the current user has a specific permission
func (r *RBACService) HasPermission(c *gin.Context, permission string) bool {
	permissions, exists := c.Get("user_permissions")
	if !exists {
		return false
	}

	perms, ok := permissions.([]string)
	if !ok {
		return false
	}

	for _, perm := range perms {
		if perm == permission || perm == "*" {
			return true
		}
	}

	return false
}

// HasRole checks if the current user has a specific role
func (r *RBACService) HasRole(c *gin.Context, role string) bool {
	roles, exists := c.Get("user_roles")
	if !exists {
		return false
	}

	userRoles, ok := roles.([]string)
	if !ok {
		return false
	}

	for _, userRole := range userRoles {
		if userRole == role || userRole == "super_admin" {
			return true
		}
	}

	return false
}

// HasAnyRole checks if the current user has any of the specified roles
func (r *RBACService) HasAnyRole(c *gin.Context, roles ...string) bool {
	for _, role := range roles {
		if r.HasRole(c, role) {
			return true
		}
	}
	return false
}

// HasAllPermissions checks if the current user has all specified permissions
func (r *RBACService) HasAllPermissions(c *gin.Context, permissions ...string) bool {
	for _, permission := range permissions {
		if !r.HasPermission(c, permission) {
			return false
		}
	}
	return true
}

// GetUserRoles retrieves roles for a user
func (r *RBACService) GetUserRoles(ctx context.Context, userID uuid.UUID) ([]string, error) {
	permissions, err := r.userAuthService.GetUserPermissions(ctx, userID)
	if err != nil {
		return nil, err
	}

	roleMap := make(map[string]bool)
	for _, perm := range permissions {
		if perm != "" {
			roleMap[perm] = true
		}
	}

	roles := make([]string, 0, len(roleMap))
	for role := range roleMap {
		roles = append(roles, role)
	}

	return roles, nil
}

// GetUserPermissions retrieves permissions for a user
func (r *RBACService) GetUserPermissions(ctx context.Context, userID uuid.UUID) ([]string, error) {
	permissions, err := r.userAuthService.GetUserPermissions(ctx, userID)
	if err != nil {
		return nil, err
	}

	permStrings := make([]string, len(permissions))
	for i, perm := range permissions {
		permStrings[i] = perm
	}

	return permStrings, nil
}

// CanAccessResource checks if user can access a specific resource
func (r *RBACService) CanAccessResource(c *gin.Context, resource string, action string) bool {
	permission := resource + "." + action
	return r.HasPermission(c, permission)
}

// Role and permission management endpoints

func (a *AdminAPI) listRoles(c *gin.Context) {
	if !a.rbac.HasPermission(c, "roles.read") {
		c.JSON(400, gin.H{"error": "Insufficient permissions"})
		return
	}

	// Implementation for listing roles
	// This would query your roles table
	c.JSON(200, gin.H{"roles": []string{"admin", "user", "moderator"}})
}

func (a *AdminAPI) createRole(c *gin.Context) {
	if !a.rbac.HasPermission(c, "roles.create") {
		c.JSON(400, gin.H{"error": "Insufficient permissions"})
		return
	}

	var req CreateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	// Implementation for creating role
	// This would insert into your roles table

	// Log admin action
	adminUserID := a.getAdminUserID(c)
	a.userAuthService.AuditService().LogEvent(c.Request.Context(), "admin.role.created", "medium", shared.AuditContext{
		UserID:    adminUserID.String(),
		IPAddress: c.ClientIP(),
		UserAgent: c.GetHeader("User-Agent"),
		Metadata: map[string]interface{}{
			"role_name": req.Name,
		},
	}, "Admin created new role")

	c.JSON(201, gin.H{"message": "Role created successfully"})
}

func (a *AdminAPI) assignRole(c *gin.Context) {
	if !a.rbac.HasPermission(c, "roles.assign") {
		c.JSON(400, gin.H{"error": "Insufficient permissions"})
		return
	}

	roleID := c.Param("id")
	userID, err := uuid.Parse(c.Param("user_id"))
	if err != nil {
		c.JSON(400, gin.H{"error": "Invalid user ID"})
		return
	}

	err = a.userAuthService.AssignRole(c.Request.Context(), userID, roleID)
	if err != nil {
		a.logger.Error("Failed to assign role", zap.String("role", roleID), zap.String("user_id", userID.String()), zap.Error(err))
		c.JSON(500, gin.H{"error": "Failed to assign role"})
		return
	}

	// Log admin action
	adminUserID := a.getAdminUserID(c)
	a.userAuthService.AuditService().LogEvent(c.Request.Context(), "admin.role.assigned", "medium", shared.AuditContext{
		UserID:    adminUserID.String(),
		IPAddress: c.ClientIP(),
		UserAgent: c.GetHeader("User-Agent"),
		Metadata: map[string]interface{}{
			"role_id":          roleID,
			"assigned_to_user": userID,
		},
	}, "Admin assigned role to user")

	c.JSON(200, gin.H{"message": "Role assigned successfully"})
}

func (a *AdminAPI) revokeRole(c *gin.Context) {
	if !a.rbac.HasPermission(c, "roles.revoke") {
		c.JSON(400, gin.H{"error": "Insufficient permissions"})
		return
	}

	roleID := c.Param("id")
	userID, err := uuid.Parse(c.Param("user_id"))
	if err != nil {
		c.JSON(400, gin.H{"error": "Invalid user ID"})
		return
	}

	err = a.userAuthService.RevokeRole(c.Request.Context(), userID, roleID)
	if err != nil {
		a.logger.Error("Failed to revoke role", zap.String("role", roleID), zap.String("user_id", userID.String()), zap.Error(err))
		c.JSON(500, gin.H{"error": "Failed to revoke role"})
		return
	}

	// Log admin action
	adminUserID := a.getAdminUserID(c)
	a.userAuthService.AuditService().LogEvent(c.Request.Context(), "admin.role.revoked", "medium", shared.AuditContext{
		UserID:    adminUserID.String(),
		IPAddress: c.ClientIP(),
		UserAgent: c.GetHeader("User-Agent"),
		Metadata: map[string]interface{}{
			"role_id":           roleID,
			"revoked_from_user": userID,
		},
	}, "Admin revoked role from user")

	c.JSON(200, gin.H{"message": "Role revoked successfully"})
}

func (a *AdminAPI) listPermissions(c *gin.Context) {
	if !a.rbac.HasPermission(c, "permissions.read") {
		c.JSON(400, gin.H{"error": "Insufficient permissions"})
		return
	}

	// Implementation for listing permissions
	permissions := []string{
		"users.read",
		"users.create",
		"users.update",
		"users.delete",
		"roles.read",
		"roles.create",
		"roles.update",
		"roles.delete",
		"roles.assign",
		"roles.revoke",
		"permissions.read",
		"permissions.create",
		"permissions.update",
		"permissions.delete",
		"system.read",
		"system.cache.flush",
		"api_keys.read",
		"api_keys.create",
		"api_keys.revoke",
	}

	c.JSON(200, gin.H{"permissions": permissions})
}

func (a *AdminAPI) listAPIKeys(c *gin.Context) {
	if !a.rbac.HasPermission(c, "api_keys.read") {
		c.JSON(400, gin.H{"error": "Insufficient permissions"})
		return
	}

	// Implementation for listing API keys
	// This would query your API keys table
	c.JSON(200, gin.H{"api_keys": []interface{}{}})
}

func (a *AdminAPI) createAPIKey(c *gin.Context) {
	if !a.rbac.HasPermission(c, "api_keys.create") {
		c.JSON(400, gin.H{"error": "Insufficient permissions"})
		return
	}

	var req CreateAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	userID, err := uuid.Parse(req.UserID)
	if err != nil {
		c.JSON(400, gin.H{"error": "Invalid user ID"})
		return
	}

	apiKey, err := a.userAuthService.CreateAPIKey(c.Request.Context(), userID, req.Name, req.Permissions, nil)
	if err != nil {
		a.logger.Error("Failed to create API key", zap.Error(err))
		c.JSON(500, gin.H{"error": "Failed to create API key"})
		return
	}

	// Log admin action
	adminUserID := a.getAdminUserID(c)
	a.userAuthService.AuditService().LogEvent(c.Request.Context(), "admin.api_key.created", "medium", shared.AuditContext{
		UserID:    adminUserID.String(),
		IPAddress: c.ClientIP(),
		UserAgent: c.GetHeader("User-Agent"),
		Metadata: map[string]interface{}{
			"api_key_id": apiKey.ID,
			"for_user":   userID,
			"name":       req.Name,
		},
	}, "Admin created API key")

	c.JSON(201, gin.H{
		"api_key": apiKey.Key, // Use the correct field for the API key string
		"key_id":  apiKey.ID,
		"message": "API key created successfully",
	})
}

// Request types for role/permission management

type CreateRoleRequest struct {
	Name        string   `json:"name" binding:"required"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
}

type CreateAPIKeyRequest struct {
	UserID      string   `json:"user_id" binding:"required"`
	Name        string   `json:"name" binding:"required"`
	Permissions []string `json:"permissions" binding:"required"`
	ExpiresAt   *string  `json:"expires_at,omitempty"`
}
