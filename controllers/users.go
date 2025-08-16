package controllers

import (
	"errors"
	"net/http"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
	"workspace/models"
)

type UsersController struct {
	application.BaseController
}

func Users() (string, *UsersController) {
	return "users", &UsersController{}
}

func (c *UsersController) Setup(app *application.App) {
	c.BaseController.Setup(app)
	auth := app.Use("auth").(*authentication.Controller)

	// Admin-only access check
	adminRequired := func(app *application.App, w http.ResponseWriter, r *http.Request) bool {
		user, _, err := auth.Authenticate(r)
		if err != nil {
			c.Redirect(w, r, "/signin")
			return false
		}
		if !user.IsAdmin {
			c.RenderErrorMsg(w, r, "Admin access required")
			return false
		}
		return true
	}

	// User management pages (admin only)
	http.Handle("GET /settings/users", app.Serve("users-list.html", adminRequired))
	http.Handle("GET /settings/users/{id}", app.Serve("user-detail.html", adminRequired))
	http.Handle("POST /settings/users/{id}/role", app.ProtectFunc(c.updateUserRole, adminRequired))
	http.Handle("POST /settings/users/{id}/disable", app.ProtectFunc(c.disableUser, adminRequired))
	http.Handle("POST /settings/users/{id}/enable", app.ProtectFunc(c.enableUser, adminRequired))
}

func (c UsersController) Handle(req *http.Request) application.Controller {
	c.Request = req
	return &c
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
		if user.IsAdmin {
			count++
		}
	}
	return count
}

// GetDeveloperCount returns the count of admin users (admins ARE developers)
func (c *UsersController) GetDeveloperCount() int {
	// This is the same as GetAdminCount since admins are developers
	return c.GetAdminCount()
}

// GetGuestCount returns the count of non-admin users
func (c *UsersController) GetGuestCount() int {
	users, err := c.GetAllUsers()
	if err != nil {
		return 0
	}
	
	count := 0
	for _, user := range users {
		// All non-admin users are guests
		if !user.IsAdmin {
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
		c.RenderErrorMsg(w, r, "authentication required")
		return
	}

	userID := r.PathValue("id")
	if userID == "" {
		c.RenderErrorMsg(w, r, "user ID required")
		return
	}

	// Prevent users from changing their own role
	if userID == currentUser.ID {
		c.RenderErrorMsg(w, r, "cannot change your own role")
		return
	}

	// Get the user
	user, err := auth.Users.Get(userID)
	if err != nil {
		c.RenderErrorMsg(w, r, "user not found")
		return
	}

	// Toggle admin status
	makeAdmin := r.FormValue("make_admin") == "true"
	user.IsAdmin = makeAdmin

	err = auth.Users.Update(user)
	if err != nil {
		c.RenderErrorMsg(w, r, "failed to update user role")
		return
	}

	// Log activity
	adminStatus := "non-admin"
	if user.IsAdmin {
		adminStatus = "admin"
	}
	models.LogActivity("user_role_updated", "Updated user role",
		"Changed "+user.Name+" to "+adminStatus, currentUser.ID, userID, "user", "")

	c.Refresh(w, r)
}


// disableUser handles disabling a user account
func (c *UsersController) disableUser(w http.ResponseWriter, r *http.Request) {
	// For now, we'll just change their role to "guest"
	auth := c.App.Use("auth").(*authentication.Controller)
	currentUser, _, err := auth.Authenticate(r)
	if err != nil {
		c.RenderErrorMsg(w, r, "authentication required")
		return
	}

	userID := r.PathValue("id")
	if userID == "" {
		c.RenderErrorMsg(w, r, "user ID required")
		return
	}

	// Prevent self-disable
	if userID == currentUser.ID {
		c.RenderErrorMsg(w, r, "cannot disable your own account")
		return
	}

	user, err := auth.Users.Get(userID)
	if err != nil {
		c.RenderErrorMsg(w, r, "user not found")
		return
	}

	// Remove admin privileges
	user.IsAdmin = false

	err = auth.Users.Update(user)
	if err != nil {
		c.RenderErrorMsg(w, r, "failed to disable user")
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
		c.RenderErrorMsg(w, r, "authentication required")
		return
	}

	userID := r.PathValue("id")
	if userID == "" {
		c.RenderErrorMsg(w, r, "user ID required")
		return
	}

	user, err := auth.Users.Get(userID)
	if err != nil {
		c.RenderErrorMsg(w, r, "user not found")
		return
	}

	// Keep as non-admin (guests have read-only access to public repos)
	user.IsAdmin = false

	err = auth.Users.Update(user)
	if err != nil {
		c.RenderErrorMsg(w, r, "failed to enable user")
		return
	}

	models.LogActivity("user_enabled", "Enabled user account",
		"Enabled "+user.Name+"'s account", currentUser.ID, userID, "user", "")

	c.Refresh(w, r)
}