package controllers

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"workspace/models"
	"workspace/services"

	"github.com/The-Skyscape/devtools/pkg/application"
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

// OwnerInfo represents repository owner information
type OwnerInfo struct {
	ID     string
	Name   string
	Avatar string
}

// Setup is called when the application is started
func (c *HomeController) Setup(app *application.App) {
	c.BaseController.Setup(app)

	auth := app.Use("auth").(*AuthController)

	http.Handle("GET /{$}", app.ProtectFunc(c.homePage, nil))
	http.Handle("GET /activities", app.Serve("activities.html", auth.Required))

	// Public repository routes (no authentication required)
	http.Handle("GET /public/repos/{id}", app.Serve("public-repo-view.html", nil))
	http.Handle("GET /public/repos/{id}/issues", app.Serve("public-repo-issues.html", nil))
	http.Handle("POST /public/repos/{id}/issues", app.ProtectFunc(c.submitPublicIssue, nil))

	// Public repository search
	http.Handle("GET /public/repos/search", app.Serve("home-public-repos-results.html", nil))

	// Infinite scroll endpoints
	http.Handle("GET /repos/more", app.Serve("repos-more.html", auth.Required))
	http.Handle("GET /activities/more", app.Serve("activities-more.html", auth.Required))

	// Vault is now initialized automatically by models.Secrets
}

// Handle is called when each request is handled
func (c HomeController) Handle(req *http.Request) application.Controller {
	c.Request = req
	return &c
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
	auth := c.Use("auth").(*AuthController)
	user := auth.GetAuthenticatedUser(c.Request)
	if user == nil {
		// Not authenticated - show public repos with pagination support
		offset := c.Params().Int("offset", 0)
		if offset < 0 {
			offset = 0
		}

		query := fmt.Sprintf("WHERE Visibility = ? ORDER BY UpdatedAt DESC LIMIT 20 OFFSET %d", offset)
		repos, err := models.Repositories.Search(query, "public")
		if err != nil {
			return nil, nil
		}
		return repos, nil
	}

	// Logged in users see only 10 repos on dashboard
	if user.IsAdmin {
		return models.Repositories.Search("ORDER BY UpdatedAt DESC LIMIT 10")
	}

	// Non-admins see only public repositories (limited to 10)
	return models.Repositories.Search("WHERE Visibility = ? ORDER BY UpdatedAt DESC LIMIT 10", "public")
}

// MoreRepos returns the next page of repositories for infinite scroll
func (c *HomeController) MoreRepos() ([]*models.Repository, error) {
	auth := c.Use("auth").(*AuthController)
	user := auth.GetAuthenticatedUser(c.Request)

	// Parse offset from query params
	offset := c.Params().Int("offset", 0)
	if offset < 0 {
		offset = 0
	}

	// Build query based on user permissions
	if user == nil {
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
	auth := c.Use("auth").(*AuthController)
	user := auth.GetAuthenticatedUser(c.Request)

	offset := c.Params().Int("offset", 0)
	if offset < 0 {
		offset = 0
	}

	var repos []*models.Repository

	// Build query based on user permissions
	if user == nil {
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
	offset := c.Params().Int("offset", 0)
	if offset < 0 {
		offset = 0
	}
	return offset + 20
}

// PublicRepos returns all public repositories for display on homepage
func (c *HomeController) PublicRepos() ([]*models.Repository, error) {
	// Get all public repositories (no authentication required)
	return models.Repositories.Search("WHERE Visibility = 'public' ORDER BY UpdatedAt DESC")
}

// HasMorePublicRepos checks if there are more public repos for logged out users
func (c *HomeController) HasMorePublicRepos() bool {
	auth := c.Use("auth").(*AuthController)
	user := auth.GetAuthenticatedUser(c.Request)
	if user != nil {
		// User is logged in, no pagination needed
		return false
	}

	offset := c.Params().Int("offset", 0)
	if offset < 0 {
		offset = 0
	}

	// Check if there are more public repos after current offset
	query := fmt.Sprintf("WHERE Visibility = ? ORDER BY UpdatedAt DESC LIMIT 1 OFFSET %d", offset+20)
	repos, _ := models.Repositories.Search(query, "public")
	return len(repos) > 0
}

// NextPublicReposOffset returns the offset for the next page of public repos
func (c *HomeController) NextPublicReposOffset() int {
	offset := c.Params().Int("offset", 0)
	if offset < 0 {
		offset = 0
	}
	return offset + 20
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
	auth := c.Use("auth").(*AuthController)
	user := auth.GetAuthenticatedUser(c.Request)
	if user == nil {
		return nil, nil
	}

	// Get recent activities for the current user (limited to 5 for dashboard)
	return models.Activities.Search("WHERE UserID = ? ORDER BY CreatedAt DESC LIMIT 5", user.ID)
}

// AllActivities returns all activities for the activities page
func (c *HomeController) AllActivities() ([]*models.Activity, error) {
	auth := c.Use("auth").(*AuthController)
	user := auth.GetAuthenticatedUser(c.Request)
	if user == nil {
		return nil, errors.New("authentication required")
	}

	// Get first 20 activities for initial load
	return models.Activities.Search("WHERE UserID = ? ORDER BY CreatedAt DESC LIMIT 20", user.ID)
}

// MoreActivities returns the next page of activities for infinite scroll
func (c *HomeController) MoreActivities() ([]*models.Activity, error) {
	auth := c.Use("auth").(*AuthController)
	user := auth.GetAuthenticatedUser(c.Request)
	if user == nil {
		return nil, errors.New("authentication required")
	}

	// Parse offset from query params
	offset := c.Params().Int("offset", 0)
	if offset < 0 {
		offset = 0
	}

	// Get next batch of activities
	activities, _, err := models.GetUserActivitiesPaginated(user.ID, 20, offset)
	return activities, err
}

// HasMoreActivities checks if there are more activities to load
func (c *HomeController) HasMoreActivities() bool {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(c.Request)
	if err != nil {
		return false
	}

	offset := c.Params().Int("offset", 0)
	if offset < 0 {
		offset = 0
	}

	activities, total, err := models.GetUserActivitiesPaginated(user.ID, 20, offset)
	if err != nil {
		return false
	}

	return (offset + len(activities)) < total
}

// NextActivitiesOffset returns the offset for the next page of activities
func (c *HomeController) NextActivitiesOffset() int {
	offset := c.Params().Int("offset", 0)
	if offset < 0 {
		offset = 0
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

// homePage handles the home page - redirects to signup if no users exist
func (c *HomeController) homePage(w http.ResponseWriter, r *http.Request) {
	// Check if any users exist
	if models.Auth.Users.Count("") == 0 {
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

// CurrentRepo returns the public repository from the URL path (for public controller compatibility)
func (c *HomeController) CurrentRepo() (*models.Repository, error) {
	return c.getPublicRepoFromRequest(c.Request)
}

// getPublicRepoFromRequest returns a public repository (no authentication required)
func (c *HomeController) getPublicRepoFromRequest(r *http.Request) (*models.Repository, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errors.New("repository ID not found")
	}

	repo, err := models.Repositories.Get(id)
	if err != nil {
		return nil, errors.New("repository not found")
	}

	// Only allow access to public repositories
	if repo.Visibility != "public" {
		return nil, errors.New("repository not found")
	}

	// Repository state is determined dynamically by IsEmpty() method

	return repo, nil
}

// PublicRepoIssues returns issues for the current public repository
func (c *HomeController) PublicRepoIssues() ([]*models.Issue, error) {
	repo, err := c.CurrentRepo()
	if err != nil {
		return nil, err
	}

	return models.Issues.Search("WHERE RepoID = ? ORDER BY CreatedAt DESC", repo.ID)
}

// IssueSubmitted checks if an issue was just submitted
func (c *HomeController) IssueSubmitted() bool {
	if c.Request == nil {
		return false
	}
	return c.Request.URL.Query().Get("submitted") == "true"
}

// submitPublicIssue handles public issue submission
func (c *HomeController) submitPublicIssue(w http.ResponseWriter, r *http.Request) {
	// Get the public repository
	repo, err := c.getPublicRepoFromRequest(r)
	if err != nil {
		c.RenderError(w, r, err)
		return
	}

	// Validate required fields
	p := c.Params()
	title := strings.TrimSpace(p.String("title", ""))
	body := strings.TrimSpace(p.String("body", ""))
	email := strings.TrimSpace(p.String("email", ""))

	// Use Validator for comprehensive validation
	v := c.Validator()
	v.CheckRequired("title", title)
	v.CheckRequired("email", email)
	v.CheckEmail("email", email)
	
	if err := v.Result(); err != nil {
		c.RenderValidationError(w, r, err)
		return
	}

	// Create the issue
	issue := &models.Issue{
		Title:      title,
		Body:       body,
		Status:     "open",
		Column:     "", // Default to todo column
		RepoID:     repo.ID,
		AssigneeID: email, // Store submitter email in AssigneeID for public issues
	}

	newIssue, err := models.Issues.Insert(issue)
	if err != nil {
		c.RenderErrorMsg(w, r, "Failed to submit issue")
		return
	}

	// Add public-submission tag
	err = models.AddTagToIssue(newIssue.ID, "public-submission")
	if err != nil {
		c.RenderError(w, r, fmt.Errorf("failed to create issue: %w", err))
		return
	}

	// Redirect back to the issues page with success
	c.Redirect(w, r, "/public/repos/"+repo.ID+"/issues?submitted=true")
}

// RepoOwnerInfo returns basic information about the repository owner
func (c *HomeController) RepoOwnerInfo() (*OwnerInfo, error) {
	repo, err := c.CurrentRepo()
	if err != nil {
		return nil, err
	}

	// Get owner information (simplified for public view)
	if repo.UserID != "" {
		// Look up owner information from Auth system if needed
		// For now, return basic info
		return &OwnerInfo{
			ID:     repo.UserID,
			Name:   "Repository Owner",
			Avatar: "https://ui-avatars.com/api/?name=Owner&size=40&background=3b82f6&color=white",
		}, nil
	}

	return &OwnerInfo{
		ID:     "",
		Name:   "Unknown",
		Avatar: "https://ui-avatars.com/api/?name=?&size=40&background=6b7280&color=white",
	}, nil
}
