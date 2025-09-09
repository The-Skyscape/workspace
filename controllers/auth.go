package controllers

import (
	"errors"
	"net/http"
	"time"
	"workspace/models"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
)

// Auth is a factory function that returns the controller prefix and instance.
// The returned prefix "auth" makes controller methods available in templates as {{auth.MethodName}}.
func Auth() (string, *AuthController) {
	// Create new auth toolkit with the secret
	auth := authentication.New("")  // Uses AUTH_SECRET env var
	
	return "auth", &AuthController{
		BaseController: application.BaseController{},
		auth:          auth,
		cookieName:    "skyscape_workspace",
		Collection:    models.Auth,  // Reference to the auth collection for backward compatibility
	}
}

// AuthController provides workspace-specific authentication using devtools primitives.
// In the workspace model:
// - Admins are developers with full write permissions
// - Non-admin users are guests with read-only access to code and issues
type AuthController struct {
	application.BaseController
	*authentication.Collection  // Embed for backward compatibility (provides Users, GetUser, etc.)
	auth       *authentication.Auth
	cookieName string
}

// Setup initializes the authentication controller and registers routes.
func (c *AuthController) Setup(app *application.App) {
	c.BaseController.Setup(app)
	
	// Register authentication routes with /_auth/ prefix
	http.HandleFunc("GET /signin", c.ShowSignin)
	http.HandleFunc("POST /_auth/signin", c.HandleSignin)
	http.HandleFunc("GET /signup", c.ShowSignup)
	http.HandleFunc("POST /_auth/signup", c.HandleSignup)
	http.HandleFunc("POST /_auth/signout", c.HandleSignout)
}

// Handle prepares the controller for request-specific operations.
func (c AuthController) Handle(req *http.Request) application.Controller {
	c.Request = req
	return &c
}

// ShowSignin displays the signin page
func (c *AuthController) ShowSignin(w http.ResponseWriter, r *http.Request) {
	c.SetRequest(r)
		// Set the request on the controller
	c.SetRequest(r)
	
	// If already authenticated, redirect to home
	if c.CurrentUser() != nil {
		c.Redirect(w, r, "/")
		return
	}
	c.Render(w, r, "signin.html", nil)
}

// HandleSignin processes signin form submission
func (c *AuthController) HandleSignin(w http.ResponseWriter, r *http.Request) {
	c.SetRequest(r)
		// Set the request on the controller
	c.SetRequest(r)
	
	handle := r.FormValue("handle")
	password := r.FormValue("password")
	
	// Find user by handle or email
	user, err := models.Auth.GetUser(handle)
	if err != nil {
		c.RenderError(w, r, errors.New("Invalid credentials"))
		return
	}
	
	// Verify password
	if !c.auth.VerifyPassword(string(user.PassHash), password) {
		c.RenderError(w, r, errors.New("Invalid credentials"))
		return
	}
	
	// Generate session token
	token, err := c.auth.GenerateSessionToken(user.ID, 30*24*time.Hour)
	if err != nil {
		c.RenderError(w, r, err)
		return
	}
	
	// Set cookie
	c.auth.SetCookie(w, c.cookieName, token, time.Now().Add(30*24*time.Hour), r.TLS != nil)
	
	// Create session record
	models.Auth.Sessions.Insert(&authentication.Session{
		UserID: user.ID,
	})
	
	c.Refresh(w, r)
}

// ShowSignup displays the signup page
func (c *AuthController) ShowSignup(w http.ResponseWriter, r *http.Request) {
	c.SetRequest(r)
		// Set the request on the controller
	c.SetRequest(r)
	
	// Only show signup if no users exist (first user setup)
	if models.Auth.Users.Count("") > 0 {
		c.Redirect(w, r, "/signin")
		return
	}
	c.Render(w, r, "signup.html", nil)
}

// HandleSignup processes signup form submission
func (c *AuthController) HandleSignup(w http.ResponseWriter, r *http.Request) {
	c.SetRequest(r)
		// Set the request on the controller
	c.SetRequest(r)
	
	// Only allow signup if no users exist
	if models.Auth.Users.Count("") > 0 {
		c.Redirect(w, r, "/signin")
		return
	}
	
	name := r.FormValue("name")
	handle := r.FormValue("handle")
	email := r.FormValue("email")
	password := r.FormValue("password")
	
	if name == "" || handle == "" || email == "" || password == "" {
		c.RenderError(w, r, errors.New("All fields are required"))
		return
	}
	
	// Create first user as admin
	user, err := models.Auth.Signup(name, email, handle, password, true)
	if err != nil {
		c.RenderError(w, r, err)
		return
	}
	
	// Generate session token
	token, err := c.auth.GenerateSessionToken(user.ID, 30*24*time.Hour)
	if err != nil {
		c.RenderError(w, r, err)
		return
	}
	
	// Set cookie
	c.auth.SetCookie(w, c.cookieName, token, time.Now().Add(30*24*time.Hour), r.TLS != nil)
	
	// Create session record
	models.Auth.Sessions.Insert(&authentication.Session{
		UserID: user.ID,
	})
	
	c.Refresh(w, r)
}

// HandleSignout processes signout
func (c *AuthController) HandleSignout(w http.ResponseWriter, r *http.Request) {
	c.SetRequest(r)
		// Set the request on the controller
	c.SetRequest(r)
	
	// Clear cookie
	c.auth.ClearCookie(w, c.cookieName)
	c.Redirect(w, r, "/signin")
}

// CurrentUser returns the currently authenticated user from the request.
// Returns nil if no user is authenticated. This method is accessible in templates
// as {{auth.CurrentUser}} for displaying user information or conditional rendering.
func (c *AuthController) CurrentUser() *authentication.User {
	// Try to get token from cookie
	token, err := c.auth.GetTokenFromCookie(c.Request, c.cookieName)
	if err != nil {
		return nil
	}
	
	// Validate token
	claims, err := c.auth.ValidateToken(token)
	if err != nil {
		return nil
	}
	
	// Get user ID from claims
	userID, ok := claims["user_id"].(string)
	if !ok {
		return nil
	}
	
	// Get user from database
	user, err := models.Auth.Users.Get(userID)
	if err != nil {
		return nil
	}
	
	return user
}

// IsAuthenticated checks if a user is currently authenticated.
// This method is accessible in templates as {{auth.IsAuthenticated}}.
func (c *AuthController) IsAuthenticated() bool {
	return c.CurrentUser() != nil
}

// Required is an AccessCheck middleware that ensures a user is authenticated and is an admin.
// In the workspace model, only admins (developers) have write access.
func (c *AuthController) Required(app *application.App, w http.ResponseWriter, r *http.Request) bool {
	// Try to get token from cookie
	token, err := c.auth.GetTokenFromCookie(r, c.cookieName)
	if err != nil {
		// Not authenticated
		http.Redirect(w, r, "/signin", http.StatusSeeOther)
		return false
	}
	
	// Validate token
	claims, err := c.auth.ValidateToken(token)
	if err != nil {
		// Invalid token
		http.Redirect(w, r, "/signin", http.StatusSeeOther)
		return false
	}
	
	// Get user ID from claims
	userID, ok := claims["user_id"].(string)
	if !ok {
		http.Redirect(w, r, "/signin", http.StatusSeeOther)
		return false
	}
	
	// Get user from database
	user, err := models.Auth.Users.Get(userID)
	if err != nil {
		http.Redirect(w, r, "/signin", http.StatusSeeOther)
		return false
	}
	
	// Check if user is admin (developer with write access)
	if !user.IsAdmin {
		// User is authenticated but not an admin - show read-only message
		app.Render(w, r, "error-message.html", "Access denied. Only administrators can perform this action.")
		return false
	}
	
	return true
}

// ReadOnly is an AccessCheck middleware that allows both admins and regular users.
// Regular users (guests) can read code, view commit history, and report issues.
func (c *AuthController) ReadOnly(app *application.App, w http.ResponseWriter, r *http.Request) bool {
	// Try to get token from cookie
	token, err := c.auth.GetTokenFromCookie(r, c.cookieName)
	if err != nil {
		// Not authenticated - redirect to signin
		http.Redirect(w, r, "/signin", http.StatusSeeOther)
		return false
	}
	
	// Validate token
	claims, err := c.auth.ValidateToken(token)
	if err != nil {
		// Invalid token
		http.Redirect(w, r, "/signin", http.StatusSeeOther)
		return false
	}
	
	// Get user ID from claims
	userID, ok := claims["user_id"].(string)
	if !ok || userID == "" {
		http.Redirect(w, r, "/signin", http.StatusSeeOther)
		return false
	}
	
	// Any authenticated user can access read-only resources
	return true
}

// Optional is an AccessCheck that always returns true.
// Used for public pages that don't require authentication.
func (c *AuthController) Optional(app *application.App, w http.ResponseWriter, r *http.Request) bool {
	return true
}

// AdminOnly is an AccessCheck middleware that requires admin privileges.
// Wrapper around Required for backward compatibility.
func (c *AuthController) AdminOnly(app *application.App, w http.ResponseWriter, r *http.Request) bool {
	return c.Required(app, w, r)
}

// GetAuthenticatedUser gets the current user from a request.
// This is a helper method used by other controllers.
func (c *AuthController) GetAuthenticatedUser(r *http.Request) *authentication.User {
	// Try to get token from cookie
	token, err := c.auth.GetTokenFromCookie(r, c.cookieName)
	if err != nil {
		return nil
	}
	
	// Validate token
	claims, err := c.auth.ValidateToken(token)
	if err != nil {
		return nil
	}
	
	// Get user ID from claims
	userID, ok := claims["user_id"].(string)
	if !ok {
		return nil
	}
	
	// Get user from database
	user, err := models.Auth.Users.Get(userID)
	if err != nil {
		return nil
	}
	
	return user
}

// ProtectFunc wraps a handler function with authentication check.
// If adminOnly is true, only admin users can access.
func (c *AuthController) ProtectFunc(h http.HandlerFunc, adminOnly bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if any users exist (first-time setup)
		if c.Collection.Users.Count("") == 0 {
			c.Render(w, r, "signup.html", nil)
			return
		}
		
		// Get authenticated user
		user := c.GetAuthenticatedUser(r)
		if user == nil {
			c.Render(w, r, "signin.html", nil)
			return
		}
		
		// Check admin requirement
		if adminOnly && !user.IsAdmin {
			c.RenderError(w, r, errors.New("Admin access required"))
			return
		}
		
		// Call the handler
		h.ServeHTTP(w, r)
	})
}

// Authenticate validates the request and returns the user and session.
// This method provides backward compatibility with existing code.
func (c *AuthController) Authenticate(r *http.Request) (*authentication.User, *authentication.Session, error) {
	// Try to get token from cookie
	token, err := c.auth.GetTokenFromCookie(r, c.cookieName)
	if err != nil {
		return nil, nil, err
	}
	
	// Validate token
	claims, err := c.auth.ValidateToken(token)
	if err != nil {
		return nil, nil, err
	}
	
	// Get user ID from claims
	userID, ok := claims["user_id"].(string)
	if !ok {
		return nil, nil, errors.New("invalid token claims")
	}
	
	// Get user from database
	user, err := models.Auth.Users.Get(userID)
	if err != nil {
		return nil, nil, err
	}
	
	// Get session ID from claims if present
	sessionID, _ := claims["session_id"].(string)
	
	// Create a session object for compatibility
	session := &authentication.Session{
		UserID: userID,
	}
	if sessionID != "" {
		session.ID = sessionID
	}
	
	return user, session, nil
}