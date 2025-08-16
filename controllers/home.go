package controllers

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	
	"workspace/models"
	"workspace/services"

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

// RepoStats represents repository statistics
type RepoStats struct {
	TotalRepos   int
	PublicRepos  int
	PrivateRepos int
}

// Setup is called when the application is started
func (c *HomeController) Setup(app *application.App) {
	c.BaseController.Setup(app)

	auth := app.Use("auth").(*authentication.Controller)
	
	http.Handle("GET /{$}", app.ProtectFunc(c.homePage, nil))
	http.Handle("GET /signin", app.ProtectFunc(c.signinPage, nil))
	http.Handle("GET /signup", app.ProtectFunc(c.signupPage, nil))
	http.Handle("GET /activities", app.Serve("activities.html", auth.Required))
	
	// Public repository search
	http.Handle("GET /public/repos/search", app.Serve("home-public-repos-results.html", nil))
	
	// Infinite scroll endpoints
	http.Handle("GET /repos/more", app.Serve("repos-more.html", auth.Required))
	http.Handle("GET /activities/more", app.Serve("activities-more.html", auth.Required))
	
	// Initialize Vault service on startup in background
	go func() {
		if err := services.Vault.Init(); err != nil {
			log.Printf("Warning: Failed to initialize Vault service: %v", err)
		}
	}()
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
		// Not authenticated - show public repos
		repos, err := models.Repositories.Search("WHERE Visibility = ? ORDER BY UpdatedAt DESC LIMIT 20", "public")
		if err != nil {
			return nil, nil
		}
		return repos, nil
	}

	// Admins see all repositories
	if user.IsAdmin {
		return models.Repositories.Search("ORDER BY UpdatedAt DESC LIMIT 20")
	}

	// Non-admins see only public repositories
	return models.Repositories.Search("WHERE Visibility = ? ORDER BY UpdatedAt DESC LIMIT 20", "public")
}


// MoreRepos returns the next page of repositories for infinite scroll
func (c *HomeController) MoreRepos() ([]*models.Repository, error) {
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(c.Request)
	
	// Parse offset from query params
	offsetStr := c.Request.URL.Query().Get("offset")
	offset := 0
	if offsetStr != "" {
		if parsed, err := strconv.Atoi(offsetStr); err == nil && parsed > 0 {
			offset = parsed
		}
	}
	
	// Build query based on user permissions
	if err != nil {
		// Not authenticated - show public repos
		query := fmt.Sprintf("WHERE Visibility = ? ORDER BY UpdatedAt DESC LIMIT 20 OFFSET %d", offset)
		return models.Repositories.Search(query, "public")
	}
	
	// Admins see all repositories
	if user.IsAdmin {
		query := fmt.Sprintf("ORDER BY UpdatedAt DESC LIMIT 20 OFFSET %d", offset)
		return models.Repositories.Search(query)
	}
	
	// Non-admins see only public repositories
	query := fmt.Sprintf("WHERE Visibility = ? ORDER BY UpdatedAt DESC LIMIT 20 OFFSET %d", offset)
	return models.Repositories.Search(query, "public")
}

// HasMoreRepos checks if there are more repositories to load
func (c *HomeController) HasMoreRepos() bool {
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(c.Request)
	
	offsetStr := c.Request.URL.Query().Get("offset")
	offset := 0
	if offsetStr != "" {
		if parsed, err := strconv.Atoi(offsetStr); err == nil && parsed > 0 {
			offset = parsed
		}
	}
	
	var repos []*models.Repository
	
	// Build query based on user permissions
	if err != nil {
		// Not authenticated - check public repos
		query := fmt.Sprintf("WHERE Visibility = ? ORDER BY UpdatedAt DESC LIMIT 20 OFFSET %d", offset)
		repos, _ = models.Repositories.Search(query, "public")
	} else if user.IsAdmin {
		// Admins see all repositories
		query := fmt.Sprintf("ORDER BY UpdatedAt DESC LIMIT 20 OFFSET %d", offset)
		repos, _ = models.Repositories.Search(query)
	} else {
		// Non-admins see only public repositories
		query := fmt.Sprintf("WHERE Visibility = ? ORDER BY UpdatedAt DESC LIMIT 20 OFFSET %d", offset)
		repos, _ = models.Repositories.Search(query, "public")
	}
	
	// If we got 20 repos, there might be more
	return len(repos) == 20
}

// NextReposOffset returns the offset for the next page of repositories
func (c *HomeController) NextReposOffset() int {
	offsetStr := c.Request.URL.Query().Get("offset")
	offset := 0
	if offsetStr != "" {
		if parsed, err := strconv.Atoi(offsetStr); err == nil && parsed > 0 {
			offset = parsed
		}
	}
	return offset + 20
}


// PublicRepos returns all public repositories for display on homepage
func (c *HomeController) PublicRepos() ([]*models.Repository, error) {
	// Get all public repositories (no authentication required)
	return models.Repositories.Search("WHERE Visibility = 'public' ORDER BY UpdatedAt DESC")
}

// AdminProfile returns the admin user's profile information for the homepage
func (c *HomeController) AdminProfile() *models.Profile {
	// Get the admin profile
	profile, err := models.GetAdminProfile()
	if err != nil {
		// Return default profile if none exists
		return &models.Profile{
			Name:        "Workspace",
			Description: "Secure Development Platform",
			Avatar:      "https://ui-avatars.com/api/?name=Workspace&size=200&background=3b82f6&color=white",
		}
	}
	
	// Set defaults if fields are empty
	if profile.Name == "" {
		profile.Name = "Workspace"
	}
	if profile.Description == "" {
		profile.Description = "Secure Development Platform"
	}
	
	// Get admin email if not set
	if profile.Email == "" {
		users, err := models.Auth.Users.Search("ORDER BY ID ASC LIMIT 1")
		if err == nil && len(users) > 0 {
			profile.Email = users[0].Email
		}
	}
	
	// Generate avatar if not set
	if profile.Avatar == "" {
		profile.Avatar = "https://ui-avatars.com/api/?name=" + profile.Name + "&size=200&background=3b82f6&color=white"
	}
	
	return profile
}

// PublicRepoStats returns statistics about public repositories
func (c *HomeController) PublicRepoStats() (*RepoStats, error) {
	repos, err := c.PublicRepos()
	if err != nil {
		return nil, err
	}

	// Count repositories (simplified for now)
	return &RepoStats{
		TotalRepos:   len(repos),
		PublicRepos:  len(repos),
		PrivateRepos: 0, // Don't show private repo count on public homepage
	}, nil
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

	// Get first 20 activities for initial load
	return models.Activities.Search("WHERE UserID = ? ORDER BY CreatedAt DESC LIMIT 20", user.ID)
}

// MoreActivities returns the next page of activities for infinite scroll
func (c *HomeController) MoreActivities() ([]*models.Activity, error) {
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(c.Request)
	if err != nil {
		return nil, err
	}
	
	// Parse offset from query params
	offsetStr := c.Request.URL.Query().Get("offset")
	offset := 0
	if offsetStr != "" {
		if parsed, err := strconv.Atoi(offsetStr); err == nil && parsed > 0 {
			offset = parsed
		}
	}
	
	// Get next batch of activities
	activities, _, err := models.GetUserActivitiesPaginated(user.ID, 20, offset)
	return activities, err
}

// HasMoreActivities checks if there are more activities to load
func (c *HomeController) HasMoreActivities() bool {
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(c.Request)
	if err != nil {
		return false
	}
	
	offsetStr := c.Request.URL.Query().Get("offset")
	offset := 0
	if offsetStr != "" {
		if parsed, err := strconv.Atoi(offsetStr); err == nil && parsed > 0 {
			offset = parsed
		}
	}
	
	activities, total, err := models.GetUserActivitiesPaginated(user.ID, 20, offset)
	if err != nil {
		return false
	}
	
	return (offset + len(activities)) < total
}

// NextActivitiesOffset returns the offset for the next page of activities
func (c *HomeController) NextActivitiesOffset() int {
	offsetStr := c.Request.URL.Query().Get("offset")
	offset := 0
	if offsetStr != "" {
		if parsed, err := strconv.Atoi(offsetStr); err == nil && parsed > 0 {
			offset = parsed
		}
	}
	return offset + 20
}

// ActiveWorkspaces returns the count of active workspaces (admin only)
func (c *HomeController) ActiveWorkspaces() int {
	// Count Docker containers that are workspace containers
	// Workspace containers are identified by having 'workspace' in the name
	if services.Coder.IsRunning() {
		// For now, return 1 if the coder service is running
		// In the future, we can count actual workspace containers
		return 1
	}
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

// SearchPublicRepos returns filtered public repositories based on search query
func (c *HomeController) SearchPublicRepos() ([]*models.Repository, error) {
	// Get search query from request
	query := strings.TrimSpace(c.Request.FormValue("query"))
	
	// If no query, return all public repos
	if query == "" {
		return models.Repositories.Search("WHERE Visibility = 'public' ORDER BY UpdatedAt DESC")
	}
	
	// Search for public repositories matching the query (case-insensitive)
	// Search in name and description
	return models.Repositories.Search(`
		WHERE Visibility = 'public' 
		AND (
			LOWER(Name) LIKE LOWER(?) 
			OR LOWER(Description) LIKE LOWER(?)
		)
		ORDER BY UpdatedAt DESC
	`, "%"+query+"%", "%"+query+"%")
}

