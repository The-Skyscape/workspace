package controllers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
	"workspace/models"
)

type UsersController struct {
	application.BaseController
	Request *http.Request
	App     *application.App
}

func Users() (string, *UsersController) {
	return "users", &UsersController{}
}

func (c *UsersController) Setup(app *application.App) {
	c.App = app
	c.BaseController.Setup(app)
	auth := app.Use("auth").(*authentication.Controller)

	// Admin-only access check
	adminRequired := func(app *application.App, w http.ResponseWriter, r *http.Request) bool {
		user, _, err := auth.Authenticate(r)
		if err != nil {
			c.Redirect(w, r, "/signin")
			return false
		}
		if !models.IsUserAdmin(user) {
			c.Render(w, r, "error-message.html", map[string]interface{}{
				"Message": "Admin access required",
			})
			return false
		}
		return true
	}

	// User management pages (admin only)
	http.Handle("GET /users", app.Serve("users-list.html", adminRequired))
	http.Handle("GET /users/{id}", app.Serve("user-detail.html", adminRequired))
	http.Handle("POST /users/{id}/role", app.ProtectFunc(c.updateUserRole, adminRequired))
	http.Handle("POST /users/{id}/permissions", app.ProtectFunc(c.grantPermission, adminRequired))
	http.Handle("DELETE /users/{id}/permissions", app.ProtectFunc(c.revokePermission, adminRequired))
	http.Handle("POST /users/{id}/disable", app.ProtectFunc(c.disableUser, adminRequired))
	http.Handle("POST /users/{id}/enable", app.ProtectFunc(c.enableUser, adminRequired))
}

func (c *UsersController) Handle(req *http.Request) application.Controller {
	c.Request = req
	return c
}

// GetAllUsers returns all users for the users list
func (c *UsersController) GetAllUsers() ([]*authentication.User, error) {
	auth := c.App.Use("auth").(*authentication.Controller)
	return auth.Users.Search("")
}

// GetAdminCount returns the count of admin users
func (c *UsersController) GetAdminCount() int {
	users, err := c.GetAllUsers()
	if err != nil {
		return 0
	}
	
	count := 0
	for _, user := range users {
		if user.IsAdmin || user.Role == "admin" {
			count++
		}
	}
	return count
}

// GetDeveloperCount returns the count of developer users
func (c *UsersController) GetDeveloperCount() int {
	users, err := c.GetAllUsers()
	if err != nil {
		return 0
	}
	
	count := 0
	for _, user := range users {
		if user.Role == "developer" {
			count++
		}
	}
	return count
}

// GetGuestCount returns the count of guest users
func (c *UsersController) GetGuestCount() int {
	users, err := c.GetAllUsers()
	if err != nil {
		return 0
	}
	
	count := 0
	for _, user := range users {
		if user.Role == "guest" {
			count++
		}
	}
	return count
}

// GetUser returns a specific user by ID
func (c *UsersController) GetUser() (*authentication.User, error) {
	userID := c.Request.PathValue("id")
	if userID == "" {
		return nil, errors.New("user ID required")
	}
	
	auth := c.App.Use("auth").(*authentication.Controller)
	return auth.Users.Get(userID)
}

// GetUserPermissions returns all repository permissions for a user
func (c *UsersController) GetUserPermissions() ([]*models.Permission, error) {
	userID := c.Request.PathValue("id")
	if userID == "" {
		return nil, errors.New("user ID required")
	}
	
	return models.Permissions.Search("WHERE UserID = ?", userID)
}

// GetUserRepositories returns repositories a user has access to
func (c *UsersController) GetUserRepositories() ([]*models.Repository, error) {
	userID := c.Request.PathValue("id")
	if userID == "" {
		return nil, errors.New("user ID required")
	}
	
	// Get owned repositories
	return models.Repositories.Search("WHERE UserID = ?", userID)
}

// updateUserRole handles changing a user's role
func (c *UsersController) updateUserRole(w http.ResponseWriter, r *http.Request) {
	auth := c.App.Use("auth").(*authentication.Controller)
	currentUser, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("authentication required"))
		return
	}

	userID := r.PathValue("id")
	if userID == "" {
		c.Render(w, r, "error-message.html", errors.New("user ID required"))
		return
	}

	// Prevent users from changing their own role
	if userID == currentUser.ID {
		c.Render(w, r, "error-message.html", errors.New("cannot change your own role"))
		return
	}

	// Get the user
	user, err := auth.Users.Get(userID)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("user not found"))
		return
	}

	// Update role
	newRole := strings.TrimSpace(r.FormValue("role"))
	if newRole != "admin" && newRole != "developer" && newRole != "guest" {
		c.Render(w, r, "error-message.html", errors.New("invalid role"))
		return
	}

	user.Role = newRole
	// Update IsAdmin flag for compatibility
	user.IsAdmin = (newRole == "admin")

	err = auth.Users.Update(user)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("failed to update user role"))
		return
	}

	// Log activity
	models.LogActivity("user_role_updated", "Updated user role",
		"Changed "+user.Name+"'s role to "+newRole, currentUser.ID, userID, "user", "")

	c.Refresh(w, r)
}

// grantPermission handles granting repository permissions to a user
func (c *UsersController) grantPermission(w http.ResponseWriter, r *http.Request) {
	auth := c.App.Use("auth").(*authentication.Controller)
	currentUser, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("authentication required"))
		return
	}

	userID := r.PathValue("id")
	repoID := r.FormValue("repo_id")
	role := r.FormValue("role")

	if userID == "" || repoID == "" || role == "" {
		c.Render(w, r, "error-message.html", errors.New("user ID, repository ID, and role required"))
		return
	}

	// Grant the permission
	err = models.GrantPermission(userID, repoID, role)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	// Get user and repo names for logging
	user, _ := auth.Users.Get(userID)
	repo, _ := models.Repositories.Get(repoID)
	
	// Log activity
	models.LogActivity("permission_granted", "Granted repository permission",
		"Granted "+role+" access to "+user.Name+" for "+repo.Name, currentUser.ID, repoID, "permission", "")

	c.Refresh(w, r)
}

// revokePermission handles revoking repository permissions from a user
func (c *UsersController) revokePermission(w http.ResponseWriter, r *http.Request) {
	auth := c.App.Use("auth").(*authentication.Controller)
	currentUser, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("authentication required"))
		return
	}

	userID := r.PathValue("id")
	repoID := r.FormValue("repo_id")

	if userID == "" || repoID == "" {
		c.Render(w, r, "error-message.html", errors.New("user ID and repository ID required"))
		return
	}

	// Revoke the permission
	err = models.RevokePermission(userID, repoID)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	// Get user and repo names for logging
	user, _ := auth.Users.Get(userID)
	repo, _ := models.Repositories.Get(repoID)
	
	// Log activity
	models.LogActivity("permission_revoked", "Revoked repository permission",
		"Revoked access from "+user.Name+" for "+repo.Name, currentUser.ID, repoID, "permission", "")

	c.Refresh(w, r)
}

// disableUser handles disabling a user account
func (c *UsersController) disableUser(w http.ResponseWriter, r *http.Request) {
	// For now, we'll just change their role to "guest"
	auth := c.App.Use("auth").(*authentication.Controller)
	currentUser, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("authentication required"))
		return
	}

	userID := r.PathValue("id")
	if userID == "" {
		c.Render(w, r, "error-message.html", errors.New("user ID required"))
		return
	}

	// Prevent self-disable
	if userID == currentUser.ID {
		c.Render(w, r, "error-message.html", errors.New("cannot disable your own account"))
		return
	}

	user, err := auth.Users.Get(userID)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("user not found"))
		return
	}

	// Set to guest role (most restricted)
	user.Role = "guest"
	user.IsAdmin = false

	err = auth.Users.Update(user)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("failed to disable user"))
		return
	}

	models.LogActivity("user_disabled", "Disabled user account",
		"Disabled "+user.Name+"'s account", currentUser.ID, userID, "user", "")

	c.Refresh(w, r)
}

// enableUser handles enabling a user account
func (c *UsersController) enableUser(w http.ResponseWriter, r *http.Request) {
	auth := c.App.Use("auth").(*authentication.Controller)
	currentUser, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("authentication required"))
		return
	}

	userID := r.PathValue("id")
	if userID == "" {
		c.Render(w, r, "error-message.html", errors.New("user ID required"))
		return
	}

	user, err := auth.Users.Get(userID)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("user not found"))
		return
	}

	// Set to developer role (standard access)
	user.Role = "developer"
	user.IsAdmin = false

	err = auth.Users.Update(user)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("failed to enable user"))
		return
	}

	models.LogActivity("user_enabled", "Enabled user account",
		"Enabled "+user.Name+"'s account", currentUser.ID, userID, "user", "")

	c.Refresh(w, r)
}