package controllers

import (
	"net/http"
	"workspace/models"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
)

// Home is a factory function with the prefix and instance
func Home() (string, *HomeController) {
	return "home", &HomeController{}
}

// HomeController is the controller for the home page and dashboard
type HomeController struct {
	application.BaseController
}

// Setup is called when the application is started
func (c *HomeController) Setup(app *application.App) {
	c.BaseController.Setup(app)

	auth := app.Use("auth").(*authentication.Controller)
	
	http.Handle("GET /{$}", app.ProtectFunc(c.homePage, nil))
	http.Handle("GET /signin", app.ProtectFunc(c.signinPage, nil))
	http.Handle("GET /signup", app.ProtectFunc(c.signupPage, nil))
	http.Handle("GET /activities", app.Serve("activities.html", auth.Required))
}

// Handle is called when each request is handled
func (c *HomeController) Handle(req *http.Request) application.Controller {
	c.Request = req
	return c
}

// AppName returns the application name for templates
func (c *HomeController) AppName() string {
	return "Skyscape Workspace"
}

// AppDescription returns the application description for templates
func (c *HomeController) AppDescription() string {
	return "AI-powered developer environment with Git repositories and containerized workspaces"
}

// IsFirstUser returns true if no users exist yet (first user will be admin)
func (c *HomeController) IsFirstUser() bool {
	users, _ := models.Auth.Users.Search("LIMIT 1")
	return len(users) == 0
}

// UserRepos returns repositories the current user has access to
func (c *HomeController) UserRepos() ([]*models.Repository, error) {
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(c.Request)
	if err != nil {
		return nil, nil
	}

	// Get all repositories for the user
	return models.ListUserRepositories(user.ID)
}


// PublicRepos returns all public repositories for display on homepage
func (c *HomeController) PublicRepos() ([]*models.Repository, error) {
	// Get all public repositories (no authentication required)
	return models.Repositories.Search("WHERE Visibility = 'public' ORDER BY UpdatedAt DESC")
}

// AdminProfile returns the admin user's profile information for the homepage
func (c *HomeController) AdminProfile() map[string]interface{} {
	// Get the admin profile
	profile, err := models.GetAdminProfile()
	if err != nil {
		// Return default values if no profile exists
		return map[string]interface{}{
			"name":   "Skyscape Admin",
			"email":  "admin@skyscape.dev",
			"avatar": "https://ui-avatars.com/api/?name=Skyscape+Admin&size=200&background=3b82f6&color=white",
		}
	}
	
	// Get the admin user
	users, err := models.Auth.Users.Search("ORDER BY ID ASC LIMIT 1")
	if err != nil || len(users) == 0 {
		return map[string]interface{}{
			"name":   "Skyscape Admin",
			"email":  "admin@skyscape.dev",
			"avatar": "https://ui-avatars.com/api/?name=Skyscape+Admin&size=200&background=3b82f6&color=white",
		}
	}
	
	admin := users[0]
	
	// Combine user and profile data
	result := map[string]interface{}{
		"name":       admin.Name,
		"email":      admin.Email,
		"avatar":     admin.Avatar,
		"bio":        profile.Bio,
		"title":      profile.Title,
		"website":    profile.Website,
		"github":     profile.GitHub,
		"twitter":    profile.Twitter,
		"linkedin":   profile.LinkedIn,
		"show_email": profile.ShowEmail,
		"show_stats": profile.ShowStats,
	}
	
	return result
}

// PublicRepoStats returns statistics about public repositories
func (c *HomeController) PublicRepoStats() (map[string]int, error) {
	repos, err := c.PublicRepos()
	if err != nil {
		return nil, err
	}

	// Count repositories by language/type (simplified for now)
	stats := map[string]int{
		"total_repos":   len(repos),
		"public_repos":  len(repos),
		"private_repos": 0, // Don't show private repo count on public homepage
	}

	return stats, nil
}

// RecentActivity returns recent activities for the dashboard
func (c *HomeController) RecentActivity() ([]*models.Activity, error) {
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(c.Request)
	if err != nil {
		return nil, nil
	}

	// Get recent activities for the current user (limited to 5 for dashboard)
	return models.Activities.Search("WHERE UserID = ? ORDER BY CreatedAt DESC LIMIT 5", user.ID)
}

// AllActivities returns all activities for the activities page
func (c *HomeController) AllActivities() ([]*models.Activity, error) {
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(c.Request)
	if err != nil {
		return nil, err
	}

	// Get all activities for the current user (limited to 100 for performance)
	return models.Activities.Search("WHERE UserID = ? ORDER BY CreatedAt DESC LIMIT 100", user.ID)
}

// ActiveWorkspaces returns the count of active workspaces (admin only)
func (c *HomeController) ActiveWorkspaces() int {
	// This is a placeholder - implement actual workspace counting
	return 0
}

// RecentActionsCount returns the count of recent CI/CD actions (admin only)
func (c *HomeController) RecentActionsCount() int {
	// Count actions from the last 24 hours
	actions, _ := models.ActionRuns.Search("WHERE CreatedAt > datetime('now', '-1 day')")
	return len(actions)
}

// PublicActivity returns recent public activities for the homepage
func (c *HomeController) PublicActivity() ([]*models.Activity, error) {
	// Get recent activities for public repositories only
	return models.Activities.Search(`
		WHERE RepoID IN (
			SELECT ID FROM repositories WHERE Visibility = 'public'
		) OR Type IN ('repo_created', 'repo_updated')
		ORDER BY CreatedAt DESC LIMIT 10
	`)
}

// signinPage handles the signin page - redirects if already authenticated
func (c *HomeController) signinPage(w http.ResponseWriter, r *http.Request) {
	auth := c.App.Use("auth").(*authentication.Controller)
	_, _, err := auth.Authenticate(r)
	if err == nil {
		// User is already signed in, redirect to dashboard
		c.Redirect(w, r, "/")
		return
	}
	
	// Show signin page
	c.Render(w, r, "signin.html", nil)
}

// signupPage handles the signup page - redirects if already authenticated
func (c *HomeController) signupPage(w http.ResponseWriter, r *http.Request) {
	auth := c.App.Use("auth").(*authentication.Controller)
	_, _, err := auth.Authenticate(r)
	if err == nil {
		// User is already signed in, redirect to dashboard
		c.Redirect(w, r, "/")
		return
	}
	
	// Show signup page
	c.Render(w, r, "signup.html", nil)
}

// homePage handles the home page - redirects to signup if no users exist
func (c *HomeController) homePage(w http.ResponseWriter, r *http.Request) {
	// Check if any users exist
	if models.Auth.Users.Count() == 0 {
		// No users, redirect to signup
		c.Redirect(w, r, "/signup")
		return
	}
	
	// Show home page (public or dashboard based on auth status)
	c.Render(w, r, "home.html", nil)
}
