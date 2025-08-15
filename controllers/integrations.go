package controllers

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"time"

	"workspace/models"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
)

// Integrations controller prefix
func Integrations() (string, *IntegrationsController) {
	return "integrations", &IntegrationsController{}
}

// IntegrationsController handles repository integrations
type IntegrationsController struct {
	application.BaseController
}

// Handle returns a new controller instance for the request
func (c IntegrationsController) Handle(req *http.Request) application.Controller {
	c.Request = req
	return &c
}

// Setup registers routes
func (c *IntegrationsController) Setup(app *application.App) {
	c.BaseController.Setup(app)

	// GitHub Integration - admin only
	http.Handle("GET /repos/{id}/integrations", app.Serve("repo-integrations.html", AdminOnly()))
	http.Handle("POST /repos/{id}/github/setup", app.ProtectFunc(c.setupGitHub, AdminOnly()))
	http.Handle("POST /repos/{id}/github/sync", app.ProtectFunc(c.syncGitHub, AdminOnly()))
	http.Handle("POST /repos/{id}/github/disconnect", app.ProtectFunc(c.disconnectGitHub, AdminOnly()))
}


// setupGitHub handles GitHub integration setup
func (c *IntegrationsController) setupGitHub(w http.ResponseWriter, r *http.Request) {
	// Admin access already verified by route middleware

	auth := c.Use("auth").(*authentication.Controller)
	user, _, _ := auth.Authenticate(r)

	repoID := r.PathValue("id")
	if repoID == "" {
		c.RenderErrorMsg(w, r, "repository ID required")
		return
	}

	// Get form values
	githubURL := strings.TrimSpace(r.FormValue("github_url"))
	githubToken := strings.TrimSpace(r.FormValue("github_token"))
	syncDirection := r.FormValue("sync_direction")
	autoSync := r.FormValue("auto_sync") == "true"

	if githubURL == "" || githubToken == "" {
		c.RenderErrorMsg(w, r, "GitHub URL and token are required")
		return
	}

	// Get the repository
	repo, err := models.Repositories.Get(repoID)
	if err != nil {
		c.RenderErrorMsg(w, r, "repository not found")
		return
	}

	// Parse GitHub URL to add authentication
	parsedURL, err := url.Parse(githubURL)
	if err != nil {
		c.RenderErrorMsg(w, r, "invalid GitHub URL")
		return
	}

	// Add token authentication to URL for HTTPS
	var authenticatedURL string
	if parsedURL.Scheme == "https" {
		parsedURL.User = url.User(githubToken)
		authenticatedURL = parsedURL.String()
	} else {
		// For SSH URLs, use as-is
		authenticatedURL = githubURL
	}

	// Update GitHub settings
	repo.GitHubURL = githubURL // Store original URL without token
	repo.GitHubToken = githubToken // TODO: Encrypt this
	repo.SyncDirection = syncDirection
	repo.AutoSync = autoSync

	// Configure git remote with authenticated URL
	cmd := exec.Command("git", "remote", "add", "github", authenticatedURL)
	cmd.Dir = repo.Path()
	if err := cmd.Run(); err != nil {
		// Try to set the URL if remote already exists
		cmd = exec.Command("git", "remote", "set-url", "github", authenticatedURL)
		cmd.Dir = repo.Path()
		if err := cmd.Run(); err != nil {
			c.RenderError(w, r, fmt.Errorf("failed to configure GitHub remote: %w", err))
			return
		}
	}

	// Save changes
	err = models.Repositories.Update(repo)
	if err != nil {
		c.RenderErrorMsg(w, r, "failed to save GitHub settings")
		return
	}

	// Log activity
	models.LogActivity("github_connected", "Connected repository to GitHub",
		"GitHub integration configured", user.ID, repo.ID, "integration", "")

	// Redirect to integrations page
	c.Redirect(w, r, fmt.Sprintf("/repos/%s/integrations", repoID))
}

// syncGitHub handles manual GitHub sync
func (c *IntegrationsController) syncGitHub(w http.ResponseWriter, r *http.Request) {
	// Admin access already verified by route middleware

	auth := c.Use("auth").(*authentication.Controller)
	user, _, _ := auth.Authenticate(r)

	repoID := r.PathValue("id")
	if repoID == "" {
		c.RenderErrorMsg(w, r, "repository ID required")
		return
	}

	// Get the repository
	repo, err := models.Repositories.Get(repoID)
	if err != nil {
		c.RenderErrorMsg(w, r, "repository not found")
		return
	}

	if repo.GitHubURL == "" {
		c.RenderErrorMsg(w, r, "GitHub not configured")
		return
	}

	// Parse GitHub URL to add authentication
	parsedURL, err := url.Parse(repo.GitHubURL)
	if err != nil {
		c.RenderErrorMsg(w, r, "invalid GitHub URL")
		return
	}

	// Add token authentication to URL for HTTPS
	var authenticatedURL string
	if parsedURL.Scheme == "https" && repo.GitHubToken != "" {
		parsedURL.User = url.User(repo.GitHubToken)
		authenticatedURL = parsedURL.String()
	} else {
		// For SSH URLs or no token, use as-is
		authenticatedURL = repo.GitHubURL
	}

	// Update remote with authenticated URL
	cmd := exec.Command("git", "remote", "set-url", "github", authenticatedURL)
	cmd.Dir = repo.Path()
	cmd.Run() // Ignore error if remote doesn't exist yet

	// Since this is a bare repository, we need to use fetch/push with refspecs
	// Get the default branch from the bare repo
	stdout, _, err := repo.Git("symbolic-ref", "HEAD")
	branch := "master" // default fallback
	if err == nil && stdout != nil {
		ref := strings.TrimSpace(stdout.String())
		if strings.HasPrefix(ref, "refs/heads/") {
			branch = strings.TrimPrefix(ref, "refs/heads/")
		}
	}

	// Perform sync based on direction
	var syncErr error
	var errorDetails string

	switch repo.SyncDirection {
	case "push":
		// Push from bare repository to GitHub
		cmd := exec.Command("git", "push", "github", fmt.Sprintf("refs/heads/%s:refs/heads/%s", branch, branch))
		cmd.Dir = repo.Path()
		output, err := cmd.CombinedOutput()
		if err != nil {
			syncErr = err
			errorDetails = string(output)
		}
	case "pull":
		// Fetch from GitHub to bare repository
		cmd := exec.Command("git", "fetch", "github", fmt.Sprintf("+refs/heads/%s:refs/heads/%s", branch, branch))
		cmd.Dir = repo.Path()
		output, err := cmd.CombinedOutput()
		if err != nil {
			syncErr = err
			errorDetails = string(output)
		}
	case "both":
		// Fetch then push
		cmd := exec.Command("git", "fetch", "github", fmt.Sprintf("+refs/heads/%s:refs/heads/%s", branch, branch))
		cmd.Dir = repo.Path()
		output, err := cmd.CombinedOutput()
		if err != nil {
			log.Printf("Fetch failed: %v - %s", err, string(output))
			errorDetails = string(output)
		}
		
		cmd = exec.Command("git", "push", "github", fmt.Sprintf("refs/heads/%s:refs/heads/%s", branch, branch))
		cmd.Dir = repo.Path()
		output, err = cmd.CombinedOutput()
		if err != nil {
			syncErr = err
			if errorDetails != "" {
				errorDetails += "\n\n"
			}
			errorDetails += string(output)
		}
	default:
		// Default to push if no direction specified
		cmd := exec.Command("git", "push", "github", fmt.Sprintf("refs/heads/%s:refs/heads/%s", branch, branch))
		cmd.Dir = repo.Path()
		output, err := cmd.CombinedOutput()
		if err != nil {
			syncErr = err
			errorDetails = string(output)
		}
	}

	if syncErr != nil {
		if errorDetails != "" {
			c.RenderError(w, r, fmt.Errorf("sync failed: %s", errorDetails))
		} else {
			c.RenderError(w, r, fmt.Errorf("sync failed: %w", syncErr))
		}
		return
	}

	// Update last sync time
	repo.LastSyncAt = time.Now()
	models.Repositories.Update(repo)

	// Log activity
	models.LogActivity("github_synced", "Synced repository with GitHub",
		fmt.Sprintf("Sync direction: %s", repo.SyncDirection), user.ID, repo.ID, "integration", "")

	// Redirect to integrations page
	c.Redirect(w, r, fmt.Sprintf("/repos/%s/integrations", repoID))
}

// disconnectGitHub handles GitHub disconnection
func (c *IntegrationsController) disconnectGitHub(w http.ResponseWriter, r *http.Request) {
	// Admin access already verified by route middleware

	auth := c.Use("auth").(*authentication.Controller)
	user, _, _ := auth.Authenticate(r)

	repoID := r.PathValue("id")
	if repoID == "" {
		c.RenderErrorMsg(w, r, "repository ID required")
		return
	}

	// Get the repository
	repo, err := models.Repositories.Get(repoID)
	if err != nil {
		c.RenderErrorMsg(w, r, "repository not found")
		return
	}

	// Remove git remote
	cmd := exec.Command("git", "remote", "remove", "github")
	cmd.Dir = repo.Path()
	cmd.Run() // Ignore error if remote doesn't exist

	// Clear GitHub settings
	repo.GitHubURL = ""
	repo.GitHubToken = ""
	repo.SyncDirection = ""
	repo.AutoSync = false

	// Save changes
	err = models.Repositories.Update(repo)
	if err != nil {
		c.RenderErrorMsg(w, r, "failed to clear GitHub settings")
		return
	}

	// Log activity
	models.LogActivity("github_disconnected", "Disconnected repository from GitHub",
		"GitHub integration removed", user.ID, repo.ID, "integration", "")

	// Redirect to integrations page
	c.Redirect(w, r, fmt.Sprintf("/repos/%s/integrations", repoID))
}