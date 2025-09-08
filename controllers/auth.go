package controllers

import (
	"net/http"
	"workspace/models"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
)

// Auth is a factory function that returns the controller prefix and instance.
// It creates an authentication controller that wraps the devtools auth controller
// and provides additional methods for workspace-specific authentication.
// The returned prefix "auth" makes controller methods available in templates as {{auth.MethodName}}.
func Auth() (string, *AuthController) {
	return "auth", &AuthController{
		Controller: models.Auth.Controller(),
	}
}

// AuthController extends the devtools authentication controller with workspace-specific logic.
// This controller provides methods accessible in templates for checking authentication status
// and getting the current user. In the workspace model:
// - Admins are developers with full write permissions
// - Non-admin users are guests with read-only access to code and issues
type AuthController struct {
	*authentication.Controller
}

// Setup initializes the authentication controller.
// It calls the embedded authentication controller's Setup to register all auth routes,
// then sets up the base controller for this wrapper.
func (c *AuthController) Setup(app *application.App) {
	// Setup the embedded authentication controller to register routes
	c.Controller.Setup(app)
	// Setup the base controller
	c.BaseController.Setup(app)
}

// Handle prepares the controller for request-specific operations.
// Called for each HTTP request to set the request context in both
// the AuthController and its embedded authentication.Controller.
// This ensures template methods have access to the current request.
func (c AuthController) Handle(req *http.Request) application.Controller {
	// Update the request in the embedded controller
	c.Controller.Request = req
	return &c
}

// CurrentUser returns the currently authenticated user from the request context.
// Returns nil if no user is authenticated. This method is accessible in templates
// as {{auth.CurrentUser}} for displaying user information or conditional rendering.
func (c *AuthController) CurrentUser() *authentication.User {
	return c.Controller.CurrentUser()
}

// IsAuthenticated checks if a user is currently authenticated.
// Returns true if a user is logged in, false otherwise.
// This method is accessible in templates as {{auth.IsAuthenticated}}.
func (c *AuthController) IsAuthenticated() bool {
	return c.CurrentUser() != nil
}

// Required is an AccessCheck middleware that ensures a user is authenticated and is an admin.
// In the workspace model, only admins (developers) have write access.
// Returns true if the user is an authenticated admin, false otherwise.
func (c *AuthController) Required(app *application.App, w http.ResponseWriter, r *http.Request) bool {
	user, _, err := c.Authenticate(r)
	if err != nil || user == nil {
		// Not authenticated
		http.Redirect(w, r, "/signin", http.StatusSeeOther)
		return false
	}

	// Check if user is admin (developer with write access)
	if !user.IsAdmin {
		// User is authenticated but not an admin - show read-only message
		c.Render(w, r, "error-message.html", "Access denied. Only administrators can perform this action.")
		return false
	}

	return true
}

// ReadOnly is an AccessCheck middleware that allows both admins and regular users.
// Regular users (guests) can read code, view commit history, and report issues.
// Returns true if any user is authenticated, false otherwise.
func (c *AuthController) ReadOnly(app *application.App, w http.ResponseWriter, r *http.Request) bool {
	user, _, err := c.Authenticate(r)
	if err != nil || user == nil {
		// Not authenticated - redirect to signin
		http.Redirect(w, r, "/signin", http.StatusSeeOther)
		return false
	}

	// Any authenticated user can access read-only resources
	return true
}

// Optional is an AccessCheck that always returns true.
// Used for public pages that don't require authentication but may show
// different content based on authentication status.
func (c *AuthController) Optional(app *application.App, w http.ResponseWriter, r *http.Request) bool {
	// Always allow access - templates can check {{auth.CurrentUser}} for conditional rendering
	return true
}
