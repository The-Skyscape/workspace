package models

import (
	"errors"
	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
)

type Permission struct {
	application.Model
	RepoID string
	UserID string
	Role   string // "read", "write", "admin"
}

func (*Permission) Table() string { return "permissions" }

// Permission constants
const (
	RoleRead  = "read"
	RoleWrite = "write"
	RoleAdmin = "admin"
)

// HasPermission checks if a user has a specific permission level for a repository
func HasPermission(userID, repoID, requiredRole string) bool {
	// Validate inputs
	if userID == "" || repoID == "" || requiredRole == "" {
		return false
	}

	// Repository owner always has full access
	repo, err := Coding.GetRepo(repoID)
	if err == nil && repo.UserID == userID {
		return true
	}

	// Check explicit permissions
	permissions, err := Permissions.Search("WHERE UserID = ? AND RepoID = ?", userID, repoID)
	if err != nil || len(permissions) == 0 {
		return false
	}

	// Use the highest permission if multiple exist (shouldn't happen, but defensive)
	highestRole := ""
	for _, perm := range permissions {
		if hasRolePermission(perm.Role, highestRole) {
			highestRole = perm.Role
		}
	}

	return hasRolePermission(highestRole, requiredRole)
}

// hasRolePermission checks if a user role satisfies the required permission level
// Roles follow a hierarchy: read < write < admin
func hasRolePermission(userRole, requiredRole string) bool {
	// Define role hierarchy with numeric levels
	roleHierarchy := map[string]int{
		"":        0, // No role
		RoleRead:  1, // Can view repository
		RoleWrite: 2, // Can view and modify repository
		RoleAdmin: 3, // Full access including permissions management
	}

	userLevel, userExists := roleHierarchy[userRole]
	requiredLevel, requiredExists := roleHierarchy[requiredRole]

	// Both roles must be valid
	if !userExists || !requiredExists {
		return false
	}

	return userLevel >= requiredLevel
}

// GrantPermission grants or updates a permission for a user on a repository
func GrantPermission(userID, repoID, role string) error {
	// Validate inputs
	if userID == "" || repoID == "" || role == "" {
		return errors.New("userID, repoID, and role are required")
	}

	// Validate role
	validRoles := map[string]bool{
		RoleRead:  true,
		RoleWrite: true,
		RoleAdmin: true,
	}
	if !validRoles[role] {
		return errors.New("invalid role: must be read, write, or admin")
	}

	// Check if permission already exists
	existing, err := Permissions.Search("WHERE UserID = ? AND RepoID = ?", userID, repoID)
	if err != nil {
		return errors.New("failed to check existing permissions: " + err.Error())
	}

	if len(existing) > 0 {
		// Update existing permission
		existing[0].Role = role
		err = Permissions.Update(existing[0])
		if err != nil {
			return errors.New("failed to update permission: " + err.Error())
		}
		return nil
	}

	// Create new permission
	permission := &Permission{
		UserID: userID,
		RepoID: repoID,
		Role:   role,
	}

	_, err = Permissions.Insert(permission)
	if err != nil {
		return errors.New("failed to create permission: " + err.Error())
	}
	
	return nil
}

// RevokePermission removes all explicit permissions for a user on a repository
// Note: This does not affect repository ownership (UserID field)
func RevokePermission(userID, repoID string) error {
	// Validate inputs
	if userID == "" || repoID == "" {
		return errors.New("userID and repoID are required")
	}

	// Find all permissions for this user/repo combination
	permissions, err := Permissions.Search("WHERE UserID = ? AND RepoID = ?", userID, repoID)
	if err != nil {
		return errors.New("failed to find permissions: " + err.Error())
	}

	// Delete each permission
	for _, permission := range permissions {
		if err = Permissions.Delete(permission); err != nil {
			return errors.New("failed to delete permission: " + err.Error())
		}
	}

	return nil
}

// CheckRepoAccess validates that a user has the required permission level for a repository
// This is a convenience function for middleware-style permission checking
func CheckRepoAccess(user *authentication.User, repoID, requiredRole string) error {
	if user == nil {
		return errors.New("authentication required")
	}

	if !HasPermission(user.ID, repoID, requiredRole) {
		return errors.New("insufficient permissions for repository access")
	}

	return nil
}