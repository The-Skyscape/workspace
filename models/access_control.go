package models

import (
	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
	"net/http"
)

// User role constants
const (
	RoleAdminUser     = "admin"
	RoleDeveloperUser = "developer"
	RoleGuestUser     = "guest"
)

// Role-based access control helpers

// IsUserAdmin checks if the user has admin role
func IsUserAdmin(user *authentication.User) bool {
	return user != nil && (user.IsAdmin || user.Role == RoleAdminUser)
}

// IsUserDeveloper checks if the user has developer role or higher
func IsUserDeveloper(user *authentication.User) bool {
	if user == nil {
		return false
	}
	return user.Role == RoleDeveloperUser || user.Role == RoleAdminUser || user.IsAdmin
}

// CanUserCreateRepo checks if a user can create repositories
func CanUserCreateRepo(user *authentication.User) bool {
	// Only admins can create repositories
	return IsUserAdmin(user)
}

// CanUserDeleteRepo checks if a user can delete a repository
func CanUserDeleteRepo(user *authentication.User, repo *Repository) bool {
	// Only admins or repo owner can delete
	if user == nil || repo == nil {
		return false
	}
	return IsUserAdmin(user) || repo.UserID == user.ID
}

// CanUserWriteRepo checks if a user can write to a repository
func CanUserWriteRepo(user *authentication.User, repoID string) bool {
	if user == nil {
		return false
	}
	
	// Admins always have write access
	if IsUserAdmin(user) {
		return true
	}
	
	// Check if user owns the repo
	repo, err := Repositories.Get(repoID)
	if err == nil && repo.UserID == user.ID {
		return true
	}
	
	// Check explicit write permissions
	return HasPermission(user.ID, repoID, RoleWrite)
}

// CanUserReadRepo checks if a user can read a repository
func CanUserReadRepo(user *authentication.User, repo *Repository) bool {
	if repo == nil {
		return false
	}
	
	// Public repos are readable by everyone
	if repo.Visibility == "public" {
		return true
	}
	
	// Private repos require authentication
	if user == nil {
		return false
	}
	
	// Admins can read everything
	if IsUserAdmin(user) {
		return true
	}
	
	// Owner can read their own repos
	if repo.UserID == user.ID {
		return true
	}
	
	// Check explicit read permissions
	return HasPermission(user.ID, repo.ID, RoleRead)
}

// CanUserCreateIssue checks if a user can create issues
func CanUserCreateIssue(user *authentication.User, repo *Repository) bool {
	// All authenticated users can create issues on repos they can read
	if user == nil {
		return false
	}
	return CanUserReadRepo(user, repo)
}

// CanUserUpdateIssue checks if a user can update an issue
func CanUserUpdateIssue(user *authentication.User, issue *Issue) bool {
	if user == nil || issue == nil {
		return false
	}
	
	// Admins can update any issue
	if IsUserAdmin(user) {
		return true
	}
	
	// Authors can update their own issues
	if issue.AuthorID == user.ID {
		return true
	}
	
	// Repo owner can update issues in their repo
	repo, err := Repositories.Get(issue.RepoID)
	if err == nil && repo.UserID == user.ID {
		return true
	}
	
	// Users with write permission can update issues
	return HasPermission(user.ID, issue.RepoID, RoleWrite)
}

// CanUserCreatePR checks if a user can create pull requests
func CanUserCreatePR(user *authentication.User, repo *Repository) bool {
	// All authenticated users can create PRs on repos they can read
	if user == nil {
		return false
	}
	return CanUserReadRepo(user, repo)
}

// CanUserMergePR checks if a user can merge a pull request
func CanUserMergePR(user *authentication.User, pr *PullRequest) bool {
	if user == nil || pr == nil {
		return false
	}
	
	// Only admins and users with write permission can merge
	return IsUserAdmin(user) || HasPermission(user.ID, pr.RepoID, RoleWrite)
}

// CanUserUpdatePR checks if a user can update a pull request
func CanUserUpdatePR(user *authentication.User, pr *PullRequest) bool {
	if user == nil || pr == nil {
		return false
	}
	
	// Admins can update any PR
	if IsUserAdmin(user) {
		return true
	}
	
	// Authors can update their own PRs
	if pr.AuthorID == user.ID {
		return true
	}
	
	// Repo owner can update PRs in their repo
	repo, err := Repositories.Get(pr.RepoID)
	if err == nil && repo.UserID == user.ID {
		return true
	}
	
	return false
}

// Access control middleware functions

// RequireAdmin creates middleware that only allows admin users
func RequireAdmin(auth *authentication.Controller) func(*application.App, http.ResponseWriter, *http.Request) bool {
	return func(app *application.App, w http.ResponseWriter, r *http.Request) bool {
		user, _, err := auth.Authenticate(r)
		if err != nil {
			app.Render(w, r, "signin.html", nil)
			return false
		}
		
		if !IsUserAdmin(user) {
			app.Render(w, r, "error-message.html", map[string]interface{}{
				"Message": "Admin access required",
			})
			return false
		}
		
		return true
	}
}

// RequireDeveloper creates middleware that allows developers and admins
func RequireDeveloper(auth *authentication.Controller) func(*application.App, http.ResponseWriter, *http.Request) bool {
	return func(app *application.App, w http.ResponseWriter, r *http.Request) bool {
		user, _, err := auth.Authenticate(r)
		if err != nil {
			app.Render(w, r, "signin.html", nil)
			return false
		}
		
		if !IsUserDeveloper(user) {
			app.Render(w, r, "error-message.html", map[string]interface{}{
				"Message": "Developer access required",
			})
			return false
		}
		
		return true
	}
}

// AllowAnonymous always returns true, allowing access to everyone
func AllowAnonymous(app *application.App, w http.ResponseWriter, r *http.Request) bool {
	return true
}