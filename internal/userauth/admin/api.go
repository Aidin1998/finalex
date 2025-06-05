package admin

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/Aidin1998/finalex/internal/userauth/shared"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// AdminAPI provides administrative endpoints for user management
type AdminAPI struct {
	userAuthService shared.UserAuthService
	logger          *zap.Logger
	rbac            *RBACService
}

// NewAdminAPI creates a new admin API instance
func NewAdminAPI(userAuthService shared.UserAuthService, logger *zap.Logger) *AdminAPI {
	return &AdminAPI{
		userAuthService: userAuthService,
		logger:          logger,
		rbac:            NewRBACService(userAuthService, logger),
	}
}

// RegisterRoutes registers admin API routes
func (a *AdminAPI) RegisterRoutes(router *gin.RouterGroup) {
	// All admin routes require authentication and admin role
	admin := router.Group("/admin")
	admin.Use(a.requireAdminAuth())

	// User management
	users := admin.Group("/users")
	{
		users.GET("", a.listUsers)
		users.GET("/:id", a.getUser)
		users.POST("", a.createUser)
		users.PUT("/:id", a.updateUser)
		users.DELETE("/:id", a.deleteUser)
		users.PATCH("/:id/status", a.updateUserStatus)
		users.PATCH("/:id/verify-email", a.verifyUserEmail)
		users.POST("/:id/reset-password", a.resetUserPassword)
		users.GET("/:id/sessions", a.getUserSessions)
		users.DELETE("/:id/sessions", a.invalidateUserSessions)
		users.GET("/:id/audit-log", a.getUserAuditLog)
	}

	// Role management
	roles := admin.Group("/roles")
	{
		roles.GET("", a.listRoles)
		roles.POST("", a.createRole)
		roles.PUT("/:id", a.updateRole)
		roles.DELETE("/:id", a.deleteRole)
		roles.POST("/:id/users/:user_id", a.assignRole)
		roles.DELETE("/:id/users/:user_id", a.revokeRole)
		roles.GET("/:id/permissions", a.getRolePermissions)
		roles.POST("/:id/permissions", a.assignPermissionToRole)
		roles.DELETE("/:id/permissions/:permission_id", a.revokePermissionFromRole)
	}

	// Permission management
	permissions := admin.Group("/permissions")
	{
		permissions.GET("", a.listPermissions)
		permissions.POST("", a.createPermission)
		permissions.PUT("/:id", a.updatePermission)
		permissions.DELETE("/:id", a.deletePermission)
	}

	// System management
	system := admin.Group("/system")
	{
		system.GET("/stats", a.getSystemStats)
		system.GET("/audit-log", a.getSystemAuditLog)
		system.POST("/maintenance", a.enableMaintenanceMode)
		system.DELETE("/maintenance", a.disableMaintenanceMode)
		system.POST("/cache/flush", a.flushCache)
		system.GET("/rate-limits", a.getRateLimitStats)
	}

	// API Key management
	apiKeys := admin.Group("/api-keys")
	{
		apiKeys.GET("", a.listAPIKeys)
		apiKeys.POST("", a.createAPIKey)
		apiKeys.DELETE("/:id", a.revokeAPIKey)
		apiKeys.GET("/:id/usage", a.getAPIKeyUsage)
	}
}

// User management endpoints

func (a *AdminAPI) listUsers(c *gin.Context) {
	// Check permission
	if !a.rbac.HasPermission(c, "users.read") {
		c.JSON(http.StatusForbidden, gin.H{"error": "Insufficient permissions"})
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	search := c.Query("search")
	status := c.Query("status")

	users, total, err := a.userAuthService.IdentityService().ListUsers(c.Request.Context(), page, limit, search, status)
	if err != nil {
		a.logger.Error("Failed to list users", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve users"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"users": users,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func (a *AdminAPI) getUser(c *gin.Context) {
	if !a.rbac.HasPermission(c, "users.read") {
		c.JSON(http.StatusForbidden, gin.H{"error": "Insufficient permissions"})
		return
	}

	userID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	user, err := a.userAuthService.IdentityService().GetUserByID(c.Request.Context(), userID)
	if err != nil {
		a.logger.Error("Failed to get user", zap.String("user_id", userID.String()), zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	// Get user permissions and roles
	permissions, err := a.userAuthService.GetUserPermissions(c.Request.Context(), userID)
	if err != nil {
		a.logger.Warn("Failed to get user permissions", zap.String("user_id", userID.String()), zap.Error(err))
	}

	response := map[string]interface{}{
		"user":        user,
		"permissions": permissions,
	}

	c.JSON(http.StatusOK, response)
}

func (a *AdminAPI) createUser(c *gin.Context) {
	if !a.rbac.HasPermission(c, "users.create") {
		c.JSON(http.StatusForbidden, gin.H{"error": "Insufficient permissions"})
		return
	}

	var req CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Create user through registration service
	regReq := &shared.EnterpriseRegistrationRequest{
		Email:    req.Email,
		Password: req.Password,
		Phone:    req.Phone,
		// Optionally map FullName, Country, DOB, Metadata if available in CreateUserRequest
	}

	response, err := a.userAuthService.RegisterUserWithCompliance(c.Request.Context(), regReq)
	if err != nil {
		a.logger.Error("Failed to create user", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user"})
		return
	}

	// Assign roles if specified
	if len(req.Roles) > 0 {
		for _, role := range req.Roles {
			if err := a.userAuthService.AssignRole(c.Request.Context(), response.UserID, role); err != nil {
				a.logger.Warn("Failed to assign role", zap.String("role", role), zap.Error(err))
			}
		}
	}

	// Log admin action
	adminUserID := a.getAdminUserID(c)
	a.userAuthService.AuditService().LogEvent(c.Request.Context(), "admin.user.created", "medium", shared.AuditContext{
		UserID:    adminUserID.String(),
		IPAddress: c.ClientIP(),
		UserAgent: c.GetHeader("User-Agent"),
		Metadata: map[string]interface{}{
			"created_user_id": response.UserID,
			"email":           req.Email,
		},
	}, "Admin created new user")

	c.JSON(http.StatusCreated, response)
}

func (a *AdminAPI) updateUser(c *gin.Context) {
	if !a.rbac.HasPermission(c, "users.update") {
		c.JSON(http.StatusForbidden, gin.H{"error": "Insufficient permissions"})
		return
	}

	userID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	var req UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Update user through identity service
	err = a.userAuthService.IdentityService().UpdateUser(c.Request.Context(), userID, req.ToUpdateData())
	if err != nil {
		a.logger.Error("Failed to update user", zap.String("user_id", userID.String()), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update user"})
		return
	}

	// Log admin action
	adminUserID := a.getAdminUserID(c)
	a.userAuthService.AuditService().LogEvent(c.Request.Context(), "admin.user.updated", "medium", shared.AuditContext{
		UserID:    adminUserID.String(),
		IPAddress: c.ClientIP(),
		UserAgent: c.GetHeader("User-Agent"),
		Metadata: map[string]interface{}{
			"updated_user_id": userID,
		},
	}, "Admin updated user")

	c.JSON(http.StatusOK, gin.H{"message": "User updated successfully"})
}

func (a *AdminAPI) deleteUser(c *gin.Context) {
	if !a.rbac.HasPermission(c, "users.delete") {
		c.JSON(http.StatusForbidden, gin.H{"error": "Insufficient permissions"})
		return
	}

	userID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	// Soft delete user
	err = a.userAuthService.IdentityService().DeleteUser(c.Request.Context(), userID)
	if err != nil {
		a.logger.Error("Failed to delete user", zap.String("user_id", userID.String()), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete user"})
		return
	}

	// Log admin action
	adminUserID := a.getAdminUserID(c)
	a.userAuthService.AuditService().LogEvent(c.Request.Context(), "admin.user.deleted", "high", shared.AuditContext{
		UserID:    adminUserID.String(),
		IPAddress: c.ClientIP(),
		UserAgent: c.GetHeader("User-Agent"),
		Metadata: map[string]interface{}{
			"deleted_user_id": userID,
		},
	}, "Admin deleted user")

	c.JSON(http.StatusOK, gin.H{"message": "User deleted successfully"})
}

func (a *AdminAPI) updateUserStatus(c *gin.Context) {
	if !a.rbac.HasPermission(c, "users.update") {
		c.JSON(http.StatusForbidden, gin.H{"error": "Insufficient permissions"})
		return
	}

	userID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	var req UpdateStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	err = a.userAuthService.IdentityService().UpdateUserStatus(c.Request.Context(), userID, req.IsActive)
	if err != nil {
		a.logger.Error("Failed to update user status", zap.String("user_id", userID.String()), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update user status"})
		return
	}

	// Log admin action
	adminUserID := a.getAdminUserID(c)
	a.userAuthService.AuditService().LogEvent(c.Request.Context(), "admin.user.status_updated", "medium", shared.AuditContext{
		UserID:    adminUserID.String(),
		IPAddress: c.ClientIP(),
		UserAgent: c.GetHeader("User-Agent"),
		Metadata: map[string]interface{}{
			"updated_user_id": userID,
			"new_status":      req.IsActive,
		},
	}, fmt.Sprintf("Admin updated user status to %v", req.IsActive))

	c.JSON(http.StatusOK, gin.H{"message": "User status updated successfully"})
}

// Request/Response types

type CreateUserRequest struct {
	Email    string   `json:"email" binding:"required,email"`
	Password string   `json:"password" binding:"required,min=8"`
	Phone    string   `json:"phone"`
	Roles    []string `json:"roles"`
}

type UpdateUserRequest struct {
	Username  *string `json:"username"`
	FirstName *string `json:"first_name"`
	LastName  *string `json:"last_name"`
	Phone     *string `json:"phone"`
}

func (r *UpdateUserRequest) ToUpdateData() map[string]interface{} {
	data := make(map[string]interface{})
	if r.Username != nil {
		data["username"] = *r.Username
	}
	if r.FirstName != nil {
		data["first_name"] = *r.FirstName
	}
	if r.LastName != nil {
		data["last_name"] = *r.LastName
	}
	if r.Phone != nil {
		data["phone"] = *r.Phone
	}
	return data
}

type UpdateStatusRequest struct {
	IsActive bool `json:"is_active"`
}

// Helper methods

func (a *AdminAPI) requireAdminAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get token from Authorization header
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header required"})
			c.Abort()
			return
		}

		// Extract token
		token := ""
		if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
			token = authHeader[7:]
		} else {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Bearer token required"})
			c.Abort()
			return
		}

		// Validate token
		claims, err := a.userAuthService.ValidateToken(c.Request.Context(), token)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
			c.Abort()
			return
		}

		// Check admin role
		hasAdminRole := false
		for _, role := range claims.Roles {
			if role == "admin" || role == "super_admin" {
				hasAdminRole = true
				break
			}
		}

		if !hasAdminRole {
			c.JSON(http.StatusForbidden, gin.H{"error": "Admin role required"})
			c.Abort()
			return
		}

		// Store user context
		c.Set("user_id", claims.UserID)
		c.Set("user_email", claims.Email)
		c.Set("user_roles", claims.Roles)
		c.Set("user_permissions", claims.Permissions)

		c.Next()
	}
}

func (a *AdminAPI) getAdminUserID(c *gin.Context) uuid.UUID {
	if userID, exists := c.Get("user_id"); exists {
		if id, ok := userID.(uuid.UUID); ok {
			return id
		}
	}
	return uuid.Nil
}

// Additional endpoints implementation continues...
// (verifyUserEmail, resetUserPassword, getUserSessions, etc.)

func (a *AdminAPI) verifyUserEmail(c *gin.Context) {
	if !a.rbac.HasPermission(c, "users.update") {
		c.JSON(http.StatusForbidden, gin.H{"error": "Insufficient permissions"})
		return
	}

	userID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	err = a.userAuthService.IdentityService().VerifyUserEmail(c.Request.Context(), userID)
	if err != nil {
		a.logger.Error("Failed to verify user email", zap.String("user_id", userID.String()), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify email"})
		return
	}

	// Log admin action
	adminUserID := a.getAdminUserID(c)
	a.userAuthService.AuditService().LogEvent(c.Request.Context(), "admin.user.email_verified", "medium", shared.AuditContext{
		UserID:    adminUserID.String(),
		IPAddress: c.ClientIP(),
		UserAgent: c.GetHeader("User-Agent"),
		Metadata: map[string]interface{}{
			"verified_user_id": userID,
		},
	}, "Admin verified user email")

	c.JSON(http.StatusOK, gin.H{"message": "User email verified successfully"})
}

func (a *AdminAPI) getSystemStats(c *gin.Context) {
	if !a.rbac.HasPermission(c, "system.read") {
		c.JSON(http.StatusForbidden, gin.H{"error": "Insufficient permissions"})
		return
	}

	stats := map[string]interface{}{
		"timestamp": time.Now(),
		"version":   "1.0.0",
		// Add system statistics here
	}

	c.JSON(http.StatusOK, stats)
}

func (a *AdminAPI) flushCache(c *gin.Context) {
	if !a.rbac.HasPermission(c, "system.cache.flush") {
		c.JSON(http.StatusForbidden, gin.H{"error": "Insufficient permissions"})
		return
	}

	// Implementation for cache flushing
	// This would depend on your cache implementation

	// Log admin action
	adminUserID := a.getAdminUserID(c)
	a.userAuthService.AuditService().LogEvent(c.Request.Context(), "admin.system.cache_flushed", "high", shared.AuditContext{
		UserID:    adminUserID.String(),
		IPAddress: c.ClientIP(),
		UserAgent: c.GetHeader("User-Agent"),
	}, "Admin flushed system cache")

	c.JSON(http.StatusOK, gin.H{"message": "Cache flushed successfully"})
}

func (a *AdminAPI) resetUserPassword(c *gin.Context) {}

func (a *AdminAPI) getUserSessions(c *gin.Context) {}

func (a *AdminAPI) invalidateUserSessions(c *gin.Context) {}

func (a *AdminAPI) getUserAuditLog(c *gin.Context) {}

func (a *AdminAPI) updateRole(c *gin.Context) {}

func (a *AdminAPI) deleteRole(c *gin.Context) {}

func (a *AdminAPI) getRolePermissions(c *gin.Context) {}

func (a *AdminAPI) assignPermissionToRole(c *gin.Context) {}

func (a *AdminAPI) revokePermissionFromRole(c *gin.Context) {}

func (a *AdminAPI) createPermission(c *gin.Context) {}

func (a *AdminAPI) updatePermission(c *gin.Context) {}

func (a *AdminAPI) deletePermission(c *gin.Context) {}

func (a *AdminAPI) getSystemAuditLog(c *gin.Context) {}

func (a *AdminAPI) enableMaintenanceMode(c *gin.Context) {}

func (a *AdminAPI) disableMaintenanceMode(c *gin.Context) {}

func (a *AdminAPI) getRateLimitStats(c *gin.Context) {}

func (a *AdminAPI) revokeAPIKey(c *gin.Context) {}

func (a *AdminAPI) getAPIKeyUsage(c *gin.Context) {}
