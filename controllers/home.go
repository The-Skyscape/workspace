package controllers

import (
	"net/http"
	"workspace/models"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
	"github.com/The-Skyscape/devtools/pkg/coding"
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

	http.Handle("GET /{$}", app.Serve("home.html", nil))
	http.Handle("GET /signin", app.Serve("signin.html", nil))
	http.Handle("GET /signup", app.Serve("signup.html", nil))
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
func (c *HomeController) UserRepos() ([]*coding.GitRepo, error) {
	auth := c.Use("auth").(*authentication.Controller)
	_, _, err := auth.Authenticate(c.Request)
	if err != nil {
		return nil, nil
	}

	// Get all repositories for now - later we'll add permissions
	return models.Coding.SearchRepos("")
}

// UserWorkspaces returns active workspaces for the current user
func (c *HomeController) UserWorkspaces() ([]*coding.Workspace, error) {
	auth := c.Use("auth").(*authentication.Controller)
	_, _, err := auth.Authenticate(c.Request)
	if err != nil {
		return nil, nil
	}

	// Get workspaces for the user
	return models.Coding.Workspaces()
}

// PublicRepos returns all public repositories for display on homepage
func (c *HomeController) PublicRepos() ([]*coding.GitRepo, error) {
	// Get all public repositories (no authentication required)
	return models.Coding.SearchRepos("WHERE Visibility = 'public' ORDER BY UpdatedAt DESC")
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
