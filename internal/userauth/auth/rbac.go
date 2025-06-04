package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ValidatePermission validates if a user has permission for a specific resource and action
func (s *Service) ValidatePermission(ctx context.Context, userID uuid.UUID, resource, action string) error {
	// Get user permissions
	permissions, err := s.GetUserPermissions(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get user permissions: %w", err)
	}

	// Check if user has the specific permission
	requiredPermission := fmt.Sprintf("%s:%s", resource, action)
	for _, permission := range permissions {
		if permission.Resource == resource && permission.Action == action {
			return nil
		}
		// Check for wildcard permissions
		if permission.Resource == resource && permission.Action == "*" {
			return nil
		}
		if permission.Resource == "*" && permission.Action == action {
			return nil
		}
		if permission.Resource == "*" && permission.Action == "*" {
			return nil
		}
	}

	// Check for admin role (has all permissions)
	var userRole string
	err = s.db.Raw("SELECT role FROM users WHERE id = ?", userID).Scan(&userRole).Error
	if err != nil {
		return fmt.Errorf("failed to get user role: %w", err)
	}

	if userRole == "admin" || userRole == "super_admin" {
		return nil
	}

	return fmt.Errorf("insufficient permissions: required %s", requiredPermission)
}

// AssignRole assigns a role to a user
func (s *Service) AssignRole(ctx context.Context, userID uuid.UUID, role string) error {
	// Validate role
	if !s.isValidRole(role) {
		return fmt.Errorf("invalid role: %s", role)
	}
	// Update user role
	result := s.db.Exec("UPDATE users SET role = ?, updated_at = ? WHERE id = ?",
		role, time.Now(), userID)
	if result.Error != nil {
		return fmt.Errorf("failed to assign role: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("user not found")
	}

	// Invalidate all user sessions to force re-authentication with new role
	err := s.InvalidateAllSessions(ctx, userID)
	if err != nil {
		s.logger.Warn("Failed to invalidate sessions after role change",
			zap.String("user_id", userID.String()),
			zap.String("error", err.Error()))
	}

	s.logger.Info("Role assigned",
		zap.String("user_id", userID.String()),
		zap.String("role", role))

	return nil
}

// RevokeRole revokes a role from a user (sets to default 'user' role)
func (s *Service) RevokeRole(ctx context.Context, userID uuid.UUID, role string) error {
	// Set user back to default 'user' role
	result := s.db.Exec("UPDATE users SET role = 'user', updated_at = ? WHERE id = ? AND role = ?",
		time.Now(), userID, role)
	if result.Error != nil {
		return fmt.Errorf("failed to revoke role: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("user not found or doesn't have the specified role")
	}

	// Invalidate all user sessions
	err := s.InvalidateAllSessions(ctx, userID)
	if err != nil {
		s.logger.Warn("Failed to invalidate sessions after role revocation",
			zap.String("user_id", userID.String()),
			zap.String("error", err.Error()),
		)
	}
	s.logger.Info("Role revoked",
		zap.String("user_id", userID.String()),
		zap.String("role", role),
	)

	return nil
}

// GetUserPermissions returns all permissions for a user based on their role and custom permissions
func (s *Service) GetUserPermissions(ctx context.Context, userID uuid.UUID) ([]Permission, error) {
	// Get user role and custom RBAC configuration
	var user struct {
		Role string
		RBAC string
	}
	err := s.db.Raw("SELECT role, rbac FROM users WHERE id = ?", userID).Scan(&user).Error
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	// Get base permissions from role
	permissions := s.getRolePermissions(user.Role)

	// Add custom permissions from RBAC field
	if user.RBAC != "" {
		customPerms, err := s.parseRBACConfig(user.RBAC)
		if err != nil {
			s.logger.Warn("Failed to parse RBAC config",
				zap.String("user_id", userID.String()),
				zap.String("error", err.Error()),
			)
		} else {
			permissions = append(permissions, customPerms...)
		}
	}

	// Remove duplicates
	permissions = s.deduplicatePermissions(permissions)

	return permissions, nil
}

// getRolePermissions returns permissions for a specific role
func (s *Service) getRolePermissions(role string) []Permission {
	switch role {
	case "super_admin":
		return []Permission{
			{Resource: "*", Action: "*"},
		}
	case "admin":
		return []Permission{
			{Resource: "users", Action: "read"},
			{Resource: "users", Action: "write"},
			{Resource: "users", Action: "delete"},
			{Resource: "orders", Action: "read"},
			{Resource: "orders", Action: "write"},
			{Resource: "orders", Action: "delete"},
			{Resource: "trades", Action: "read"},
			{Resource: "trades", Action: "write"},
			{Resource: "accounts", Action: "read"},
			{Resource: "accounts", Action: "write"},
			{Resource: "marketdata", Action: "read"},
			{Resource: "wallets", Action: "read"},
			{Resource: "wallets", Action: "write"},
			{Resource: "deposits", Action: "read"},
			{Resource: "deposits", Action: "write"},
			{Resource: "withdrawals", Action: "read"},
			{Resource: "withdrawals", Action: "write"},
			{Resource: "system", Action: "admin"},
		}
	case "trader":
		return []Permission{
			{Resource: "orders", Action: "read"},
			{Resource: "orders", Action: "write"},
			{Resource: "trades", Action: "read"},
			{Resource: "accounts", Action: "read"},
			{Resource: "marketdata", Action: "read"},
			{Resource: "wallets", Action: "read"},
			{Resource: "deposits", Action: "read"},
			{Resource: "deposits", Action: "write"},
			{Resource: "withdrawals", Action: "read"},
			{Resource: "withdrawals", Action: "write"},
		}
	case "viewer":
		return []Permission{
			{Resource: "orders", Action: "read"},
			{Resource: "trades", Action: "read"},
			{Resource: "accounts", Action: "read"},
			{Resource: "marketdata", Action: "read"},
		}
	case "support":
		return []Permission{
			{Resource: "users", Action: "read"},
			{Resource: "orders", Action: "read"},
			{Resource: "trades", Action: "read"},
			{Resource: "accounts", Action: "read"},
			{Resource: "marketdata", Action: "read"},
			{Resource: "deposits", Action: "read"},
			{Resource: "withdrawals", Action: "read"},
		}
	case "auditor":
		return []Permission{
			{Resource: "users", Action: "read"},
			{Resource: "orders", Action: "read"},
			{Resource: "trades", Action: "read"},
			{Resource: "accounts", Action: "read"},
			{Resource: "deposits", Action: "read"},
			{Resource: "withdrawals", Action: "read"},
			{Resource: "audit", Action: "read"},
		}
	default: // "user" role
		return []Permission{
			{Resource: "orders", Action: "read"},
			{Resource: "accounts", Action: "read"},
			{Resource: "marketdata", Action: "read"},
			{Resource: "wallets", Action: "read"},
			{Resource: "deposits", Action: "read"},
			{Resource: "deposits", Action: "write"},
			{Resource: "withdrawals", Action: "read"},
			{Resource: "withdrawals", Action: "write"},
		}
	}
}

// isValidRole checks if a role is valid
func (s *Service) isValidRole(role string) bool {
	validRoles := map[string]bool{
		"user":        true,
		"trader":      true,
		"viewer":      true,
		"support":     true,
		"auditor":     true,
		"admin":       true,
		"super_admin": true,
	}
	return validRoles[role]
}

// parseRBACConfig parses RBAC configuration from JSON
func (s *Service) parseRBACConfig(rbacJSON string) ([]Permission, error) {
	var config struct {
		Permissions []Permission `json:"permissions"`
		Roles       []string     `json:"roles"`
	}

	if err := json.Unmarshal([]byte(rbacJSON), &config); err != nil {
		return nil, fmt.Errorf("failed to parse RBAC config: %w", err)
	}

	var permissions []Permission

	// Add direct permissions
	permissions = append(permissions, config.Permissions...)

	// Add permissions from additional roles
	for _, role := range config.Roles {
		rolePerms := s.getRolePermissions(role)
		permissions = append(permissions, rolePerms...)
	}

	return permissions, nil
}

// deduplicatePermissions removes duplicate permissions
func (s *Service) deduplicatePermissions(permissions []Permission) []Permission {
	seen := make(map[string]bool)
	var result []Permission

	for _, perm := range permissions {
		key := fmt.Sprintf("%s:%s", perm.Resource, perm.Action)
		if !seen[key] {
			seen[key] = true
			result = append(result, perm)
		}
	}

	return result
}

// SetCustomPermissions sets custom permissions for a user
func (s *Service) SetCustomPermissions(ctx context.Context, userID uuid.UUID, permissions []Permission) error {
	// Validate permissions
	for _, perm := range permissions {
		if !s.isValidPermission(perm) {
			return fmt.Errorf("invalid permission: %s:%s", perm.Resource, perm.Action)
		}
	}

	// Create RBAC config
	config := struct {
		Permissions []Permission `json:"permissions"`
		UpdatedAt   time.Time    `json:"updated_at"`
	}{
		Permissions: permissions,
		UpdatedAt:   time.Now(),
	}

	rbacJSON, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal RBAC config: %w", err)
	}

	// Update user RBAC
	result := s.db.Exec("UPDATE users SET rbac = ?, updated_at = ? WHERE id = ?",
		string(rbacJSON), time.Now(), userID)
	if result.Error != nil {
		return fmt.Errorf("failed to set custom permissions: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("user not found")
	}

	// Invalidate all user sessions
	err = s.InvalidateAllSessions(ctx, userID)
	if err != nil {
		s.logger.Warn("Failed to invalidate sessions after permission update",
			zap.String("user_id", userID.String()),
			zap.String("error", err.Error()),
		)
	}
	s.logger.Info("Custom permissions set",
		zap.String("user_id", userID.String()),
		zap.Int("permissions_count", len(permissions)),
	)

	return nil
}

// isValidPermission checks if a permission is valid
func (s *Service) isValidPermission(perm Permission) bool {
	validResources := map[string]bool{
		"users":       true,
		"orders":      true,
		"trades":      true,
		"accounts":    true,
		"marketdata":  true,
		"wallets":     true,
		"deposits":    true,
		"withdrawals": true,
		"system":      true,
		"audit":       true,
		"*":           true,
	}

	validActions := map[string]bool{
		"read":   true,
		"write":  true,
		"delete": true,
		"admin":  true,
		"*":      true,
	}

	return validResources[perm.Resource] && validActions[perm.Action]
}

// GetRoleHierarchy returns the role hierarchy
func (s *Service) GetRoleHierarchy() map[string]int {
	return map[string]int{
		"user":        1,
		"viewer":      2,
		"trader":      3,
		"support":     4,
		"auditor":     5,
		"admin":       6,
		"super_admin": 7,
	}
}

// CanAssignRole checks if one user can assign a role to another user
func (s *Service) CanAssignRole(ctx context.Context, assignerID uuid.UUID, targetRole string) (bool, error) {
	// Get assigner's role
	var assignerRole string
	err := s.db.Raw("SELECT role FROM users WHERE id = ?", assignerID).Scan(&assignerRole).Error
	if err != nil {
		return false, fmt.Errorf("failed to get assigner role: %w", err)
	}

	hierarchy := s.GetRoleHierarchy()
	assignerLevel := hierarchy[assignerRole]
	targetLevel := hierarchy[targetRole]

	// Users can only assign roles lower than their own
	return assignerLevel > targetLevel, nil
}

// RequirePermission is a helper function for middleware
func (s *Service) RequirePermission(resource, action string) func(ctx context.Context, userID uuid.UUID) error {
	return func(ctx context.Context, userID uuid.UUID) error {
		return s.ValidatePermission(ctx, userID, resource, action)
	}
}

// HasAnyPermission checks if user has any of the specified permissions
func (s *Service) HasAnyPermission(ctx context.Context, userID uuid.UUID, requiredPerms []Permission) (bool, error) {
	userPerms, err := s.GetUserPermissions(ctx, userID)
	if err != nil {
		return false, err
	}

	for _, required := range requiredPerms {
		for _, userPerm := range userPerms {
			if s.permissionMatches(userPerm, required) {
				return true, nil
			}
		}
	}

	return false, nil
}

// HasAllPermissions checks if user has all of the specified permissions
func (s *Service) HasAllPermissions(ctx context.Context, userID uuid.UUID, requiredPerms []Permission) (bool, error) {
	userPerms, err := s.GetUserPermissions(ctx, userID)
	if err != nil {
		return false, err
	}

	for _, required := range requiredPerms {
		hasPermission := false
		for _, userPerm := range userPerms {
			if s.permissionMatches(userPerm, required) {
				hasPermission = true
				break
			}
		}
		if !hasPermission {
			return false, nil
		}
	}

	return true, nil
}

// permissionMatches checks if a user permission matches a required permission
func (s *Service) permissionMatches(userPerm, requiredPerm Permission) bool {
	// Exact match
	if userPerm.Resource == requiredPerm.Resource && userPerm.Action == requiredPerm.Action {
		return true
	}

	// Wildcard matches
	if userPerm.Resource == "*" || userPerm.Action == "*" {
		return true
	}

	// Resource wildcard
	if userPerm.Resource == requiredPerm.Resource && userPerm.Action == "*" {
		return true
	}

	// Action wildcard
	if userPerm.Resource == "*" && userPerm.Action == requiredPerm.Action {
		return true
	}

	return false
}

// GetUsersWithRole returns all users with a specific role
func (s *Service) GetUsersWithRole(ctx context.Context, role string) ([]uuid.UUID, error) {
	var userIDs []uuid.UUID
	err := s.db.Raw("SELECT id FROM users WHERE role = ?", role).Scan(&userIDs).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get users with role: %w", err)
	}

	return userIDs, nil
}

// BulkAssignRole assigns a role to multiple users
func (s *Service) BulkAssignRole(ctx context.Context, userIDs []uuid.UUID, role string, assignerID uuid.UUID) error {
	// Validate role
	if !s.isValidRole(role) {
		return fmt.Errorf("invalid role: %s", role)
	}

	// Check if assigner can assign this role
	canAssign, err := s.CanAssignRole(ctx, assignerID, role)
	if err != nil {
		return fmt.Errorf("failed to check role assignment permission: %w", err)
	}
	if !canAssign {
		return fmt.Errorf("insufficient permissions to assign role: %s", role)
	}

	// Begin transaction
	tx := s.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Update users
	for _, userID := range userIDs {
		err := tx.Exec("UPDATE users SET role = ?, updated_at = ? WHERE id = ?",
			role, time.Now(), userID).Error
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to assign role to user %s: %w", userID, err)
		}

		// Invalidate user sessions
		tx.Exec("UPDATE sessions SET is_active = false WHERE user_id = ?", userID)
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit bulk role assignment: %w", err)
	}
	s.logger.Info("Bulk role assignment completed",
		zap.String("role", role),
		zap.Int("user_count", len(userIDs)),
		zap.String("assigner_id", assignerID.String()),
	)

	return nil
}
