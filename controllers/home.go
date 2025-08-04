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

	http.Handle("GET /{$}", app.ProtectFunc(c.homePage, nil))
	http.Handle("GET /signin", app.ProtectFunc(c.signinPage, nil))
	http.Handle("GET /signup", app.ProtectFunc(c.signupPage, nil))
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

// UserRepos returns repositories the current user has access to
func (c *HomeController) UserRepos() ([]*models.GitRepo, error) {
	auth := c.Use("auth").(*authentication.Controller)
	_, _, err := auth.Authenticate(c.Request)
	if err != nil {
		return nil, nil
	}

	// Get all repositories for now - later we'll add permissions
	return models.GitRepos.Search("")
}

// UserWorkspaces returns active workspaces for the current user
func (c *HomeController) UserWorkspaces() ([]*models.Workspace, error) {
	auth := c.Use("auth").(*authentication.Controller)
	_, _, err := auth.Authenticate(c.Request)
	if err != nil {
		return nil, nil
	}

	// Get workspaces for the user
	return models.GetWorkspaces()
}

// PublicRepos returns all public repositories for display on homepage
func (c *HomeController) PublicRepos() ([]*models.GitRepo, error) {
	// Get all public repositories (no authentication required)
	return models.GitRepos.Search("WHERE Visibility = 'public' ORDER BY UpdatedAt DESC")
}

// DeveloperProfile returns developer information for the homepage
func (c *HomeController) DeveloperProfile() map[string]interface{} {
	// For now, return static developer information
	// TODO: Make this configurable through a settings system
	return map[string]interface{}{
		"name":        "Developer",
		"title":       "Full Stack Developer & AI Enthusiast",
		"bio":         "Building innovative solutions with AI-powered tools and modern web technologies",
		"avatar":      "/public/developer-avatar.jpg", // TODO: Add default avatar
		"location":    "Remote",
		"company":     "Independent",
		"website":     "https://skyscape.dev",
		"github":      "github.com/developer",
		"twitter":     "@developer",
	}
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

	// Get recent activities for the current user
	return models.Activities.Search("WHERE UserID = ? ORDER BY CreatedAt DESC LIMIT 10", user.ID)
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
