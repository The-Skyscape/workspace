package controllers

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strings"

	"workspace/models"
	"workspace/services"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
)

// ReposController handles core repository CRUD operations and delegates specialized
// functionality to other controllers (issues, PRs, actions).
// Public methods are accessible from templates via {{repos.MethodName}}
// Private methods (lowercase) are for internal HTTP handlers only.
type ReposController struct {
	application.BaseController
}

// Repos returns the controller prefix and a new instance
func Repos() (string, *ReposController) {
	return "repos", &ReposController{}
}

// Setup registers all repository-related routes
func (c *ReposController) Setup(app *application.App) {
	c.BaseController.Setup(app)
	auth := app.Use("auth").(*AuthController)

	// Initialize Git server for HTTP clone/push/pull operations
	gitServer := c.InitGitServer(auth)

	// Register Git HTTP endpoints
	// These handle git clone, push, pull operations
	http.Handle("/repo/", http.StripPrefix("/repo/", gitServer))

	// Repository browsing/reading
	http.Handle("GET /repos", app.Serve("repos-list.html", auth.Required))
	http.Handle("GET /repos/search", app.ProtectFunc(c.searchRepositories, auth.Required))
	http.Handle("GET /repos/{id}", app.Serve("repo-view.html", PublicOrAdmin()))
	http.Handle("GET /repos/{id}/activity", app.Serve("repo-activity.html", PublicOrAdmin()))
	http.Handle("GET /repos/{id}/files", app.Serve("repo-files.html", PublicOrAdmin()))
	http.Handle("GET /repos/{id}/files/{path...}", app.Serve("repo-file-view.html", PublicOrAdmin()))
	http.Handle("GET /repos/{id}/edit/{path...}", app.Serve("repo-file-edit.html", AdminOnly()))
	http.Handle("GET /repos/{id}/commits", app.Serve("repo-commits.html", PublicOrAdmin()))
	http.Handle("GET /repos/{id}/commits/{hash}/diff", app.Serve("repo-commit-diff.html", PublicOrAdmin()))
	http.Handle("GET /repos/{id}/settings", app.Serve("repo-settings.html", AdminOnly()))

	// Repository management - admin only
	http.Handle("POST /repos/create", app.ProtectFunc(c.createRepository, AdminOnly()))
	http.Handle("POST /repos/import", app.ProtectFunc(c.importRepository, AdminOnly()))
	http.Handle("POST /repos/{id}/settings/update", app.ProtectFunc(c.updateRepository, AdminOnly()))
	http.Handle("POST /repos/{id}/delete", app.ProtectFunc(c.deleteRepository, AdminOnly()))

	// File operations - admin only
	http.Handle("POST /repos/{id}/files/save", app.ProtectFunc(c.saveFile, AdminOnly()))
	http.Handle("POST /repos/{id}/files/create", app.ProtectFunc(c.createFile, AdminOnly()))
	http.Handle("POST /repos/{id}/files/delete/{path...}", app.ProtectFunc(c.deleteFile, AdminOnly()))
}

// Handle returns a controller instance configured for the current request
func (c ReposController) Handle(req *http.Request) application.Controller {
	c.Request = req
	return &c
}

// ====== Public Methods for Templates ======

// CurrentRepo returns the repository being viewed based on the request path
// Checks permissions and returns error if user lacks access
func (c *ReposController) CurrentRepo() (*models.Repository, error) {
	return c.getCurrentRepoFromRequest(c.Request)
}

// UserRepos returns all repositories accessible to the current user
// Admins see all repos, guests see only public repos
func (c *ReposController) UserRepos() ([]*models.Repository, error) {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(c.Request)
	if err != nil {
		// Not authenticated - show only public repos
		return models.Repositories.Search("WHERE Visibility = ? ORDER BY UpdatedAt DESC", "public")
	}

	// Admins see all repositories
	if user.IsAdmin {
		return models.Repositories.Search("ORDER BY UpdatedAt DESC")
	}

	// Non-admins see only public repositories
	return models.Repositories.Search("WHERE Visibility = ? ORDER BY UpdatedAt DESC", "public")
}

// CurrentUser returns the currently authenticated user
// Returns nil if not authenticated
func (c *ReposController) CurrentUser() *authentication.User {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(c.Request)
	if err != nil {
		return nil
	}
	return user
}

// IsAdmin returns true if the current user is an admin
func (c *ReposController) IsAdmin() bool {
	user := c.CurrentUser()
	return user != nil && user.IsAdmin
}

// IsAuthenticated returns true if a user is logged in
func (c *ReposController) IsAuthenticated() bool {
	return c.CurrentUser() != nil
}

// CanEdit returns true if the current user can edit the current repository
func (c *ReposController) CanEdit() bool {
	// Only admins can edit repositories
	return c.IsAdmin()
}

// CanCreateRepo returns true if the current user can create repositories
func (c *ReposController) CanCreateRepo() bool {
	// Only admins can create repositories
	return c.IsAdmin()
}

// HostURL returns the full URL with protocol and host for the current request
func (c *ReposController) HostURL() string {
	if c.Request == nil {
		return "https://test.theskyscape.com"
	}

	// Build the URL from the request
	scheme := "http"
	if c.Request.TLS != nil || c.Request.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}

	return fmt.Sprintf("%s://%s", scheme, c.Request.Host)
}

// RepoActivities returns recent activities for the current repository
// Limit is determined by the current page context (activity page vs dashboard)
func (c *ReposController) RepoActivities() ([]*models.Activity, error) {
	repo, err := c.CurrentRepo()
	if err != nil {
		return nil, err
	}

	// Check if we're on the activity page (larger limit) or dashboard (smaller limit)
	activityLimit := 5 // Default for dashboard view
	if strings.Contains(c.Request.URL.Path, "/activity") {
		activityLimit = 50 // Larger limit for dedicated activity page
	}

	return repo.GetRecentActivities(activityLimit)
}

// ====== Delegation Methods for repo-tabs Partial ======

// RepoIssues delegates to IssuesController for the current repo's issues
func (c *ReposController) RepoIssues() ([]*models.Issue, error) {
	issuesController := c.Use("issues").(*IssuesController)
	return issuesController.RepoIssues()
}

// RepoPullRequests delegates to PullRequestsController for the current repo's PRs
func (c *ReposController) RepoPullRequests() ([]*models.PullRequest, error) {
	prsController := c.Use("prs").(*PullRequestsController)
	return prsController.RepoPullRequests()
}

// RepoActions delegates to ActionsController for the current repo's CI/CD actions
func (c *ReposController) RepoActions() ([]*models.Action, error) {
	actionsController := c.Use("actions").(*ActionsController)
	return actionsController.RepoActions()
}

// ====== Private HTTP Handlers ======

// getCurrentRepoFromRequest is an internal helper to get repo with simplified access checks
func (c *ReposController) getCurrentRepoFromRequest(r *http.Request) (*models.Repository, error) {
	repoID := r.PathValue("id")
	if repoID == "" {
		return nil, errors.New("repository ID required")
	}

	repo, err := models.Repositories.Get(repoID)
	if err != nil {
		return nil, errors.New("repository not found")
	}

	// Public repos are accessible to all
	if repo.Visibility == "public" {
		return repo, nil
	}

	// Private repos require admin
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		return nil, errors.New("authentication required")
	}

	if !user.IsAdmin {
		return nil, errors.New("access denied - private repository")
	}

	return repo, nil
}

// createRepository handles POST /repos/create
func (c *ReposController) createRepository(w http.ResponseWriter, r *http.Request) {
	auth := c.App.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.RenderError(w, r, err)
		return
	}

	// Parse form data
	name := r.FormValue("name")
	description := r.FormValue("description")
	visibility := r.FormValue("visibility")

	if name == "" {
		c.RenderErrorMsg(w, r, "repository name is required")
		return
	}

	// Validate visibility
	if visibility != "public" && visibility != "private" {
		visibility = "private"
	}

	// Use models.CreateRepository to ensure URL-safe IDs and proper Git initialization
	repo, err := models.CreateRepository(name, description, visibility, user.ID)
	if err != nil {
		c.RenderError(w, r, err)
		return
	}

	// Clone repository to Code Server workspace for easy editing
	// Don't fail repository creation if this doesn't work
	if err := services.Coder.CloneRepository(repo, user); err != nil {
		log.Printf("ERROR: Failed to clone repository %s to Code Server: %v", repo.ID, err)
	}

	// Activity is already logged in models.CreateRepository()

	// Redirect to new repository
	c.Redirect(w, r, fmt.Sprintf("/repos/%s", repo.ID))
}

// updateRepository handles POST /repos/{id}/settings/update
func (c *ReposController) updateRepository(w http.ResponseWriter, r *http.Request) {
	repo, err := c.getCurrentRepoFromRequest(r)
	if err != nil {
		c.RenderError(w, r, err)
		return
	}

	// Admin already verified by route middleware
	auth := c.App.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.RenderError(w, r, err)
		return
	}

	// Update fields
	repo.Name = r.FormValue("name")
	repo.Description = r.FormValue("description")
	repo.Visibility = r.FormValue("visibility")

	// Validate
	if repo.Name == "" {
		c.RenderErrorMsg(w, r, "repository name is required")
		return
	}

	if repo.Visibility != "public" && repo.Visibility != "private" {
		repo.Visibility = "private"
	}

	// Save changes
	err = models.Repositories.Update(repo)
	if err != nil {
		c.RenderError(w, r, err)
		return
	}

	// Log activity
	models.LogActivity("repo_updated", fmt.Sprintf("Updated repository %s", repo.Name),
		fmt.Sprintf("Repository settings were updated"),
		user.ID, repo.ID, "repository", "")

	// Redirect back to settings
	c.Redirect(w, r, fmt.Sprintf("/repos/%s/settings", repo.ID))
}

// IsMarkdown checks if a filename is a markdown file
func (c *ReposController) IsMarkdown(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return ext == ".md" || ext == ".markdown" || ext == ".mdown" || ext == ".mkd"
}

// deleteRepository handles POST /repos/{id}/delete
func (c *ReposController) deleteRepository(w http.ResponseWriter, r *http.Request) {
	repo, err := c.getCurrentRepoFromRequest(r)
	if err != nil {
		c.RenderError(w, r, err)
		return
	}

	// Admin already verified by route middleware
	auth := c.App.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.RenderError(w, r, err)
		return
	}

	// Delete repository (handles filesystem and database)
	// Note: We don't remove from Code Server - user may want to keep working copy
	err = models.DeleteRepository(repo.ID)
	if err != nil {
		c.RenderError(w, r, err)
		return
	}

	// Log activity
	models.LogActivity("repo_deleted", fmt.Sprintf("Deleted repository %s", repo.Name),
		fmt.Sprintf("Repository %s was deleted", repo.Name),
		user.ID, "", "repository", "")

	// Redirect to repos list
	c.Redirect(w, r, "/repos")
}
