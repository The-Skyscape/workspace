package controllers

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"time"

	"workspace/internal/github"
	"workspace/models"
	"workspace/services"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
)

// IntegrationsController handles all external service integrations
type IntegrationsController struct {
	application.BaseController
}

// Integrations returns the factory function for IntegrationsController
func Integrations() (string, *IntegrationsController) {
	return "integrations", &IntegrationsController{}
}

// Setup registers all integration-related routes
func (c *IntegrationsController) Setup(app *application.App) {
	c.BaseController.Setup(app)
	auth := app.Use("auth").(*authentication.Controller)

	// Integration pages - admin only
	http.Handle("GET /integrations", app.Serve("integrations.html", AdminOnly()))
	http.Handle("GET /repos/{id}/integrations", app.Serve("repo-integrations.html", AdminOnly()))
	
	// GitHub OAuth configuration (workspace-level)
	http.Handle("POST /integrations/github/configure", app.ProtectFunc(c.configureGitHubOAuth, AdminOnly()))
	http.Handle("POST /integrations/github/test", app.ProtectFunc(c.testGitHubOAuth, AdminOnly()))
	
	// GitHub repository integration
	http.Handle("POST /repos/{id}/github/setup", app.ProtectFunc(c.setupGitHubRepo, AdminOnly()))
	http.Handle("POST /repos/{id}/github/sync", app.ProtectFunc(c.syncGitHubRepo, AdminOnly()))
	http.Handle("POST /repos/{id}/github/disconnect", app.ProtectFunc(c.disconnectGitHubRepo, AdminOnly()))
	
	// Git sync operations
	http.Handle("POST /repos/{id}/github/push", app.ProtectFunc(c.pushGitHubRepo, auth.Required))
	http.Handle("POST /repos/{id}/github/pull", app.ProtectFunc(c.pullGitHubRepo, auth.Required))
	http.Handle("GET /repos/{id}/github/status", app.ProtectFunc(c.getSyncStatus, auth.Required))
	http.Handle("POST /repos/{id}/github/configure-remote", app.ProtectFunc(c.configureGitHubRemote, AdminOnly()))
	
	// OAuth flow
	http.Handle("GET /auth/github", app.ProtectFunc(c.initiateGitHubOAuth, auth.Required))
	http.Handle("GET /auth/github/callback", app.ProtectFunc(c.handleGitHubCallback, auth.Required))
	http.Handle("POST /auth/github/disconnect", app.ProtectFunc(c.disconnectGitHubAccount, auth.Required))
	
	// Vault management
	http.Handle("POST /integrations/vault/restart", app.ProtectFunc(c.restartVault, AdminOnly()))
	
	// Initialize services in background
	go func() {
		// IPython/Jupyter service
		if err := services.IPython.Init(); err != nil {
			log.Printf("Warning: Failed to initialize IPython service: %v", err)
		}
	}()
}

// Handle prepares controller for request
func (c IntegrationsController) Handle(req *http.Request) application.Controller {
	c.Request = req
	return &c
}

// ========== Vault Status Methods ==========

// IsVaultAvailable checks if Vault is available
func (c *IntegrationsController) IsVaultAvailable() bool {
	return models.Secrets.IsVaultAvailable()
}

// IsFallbackMode checks if using fallback storage
func (c *IntegrationsController) IsFallbackMode() bool {
	return !c.IsVaultAvailable() && models.Secrets.IsFallbackMode()
}

// GetStorageMode returns the current storage mode
func (c *IntegrationsController) GetStorageMode() string {
	if c.IsVaultAvailable() {
		return "Vault (Secure)"
	} else if c.IsFallbackMode() {
		return "File System (Fallback)"
	}
	return "Unavailable"
}

// ========== GitHub OAuth Methods ==========

// HasGitHubConnected checks if the current user has connected their GitHub account
func (c *IntegrationsController) HasGitHubConnected() bool {
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(c.Request)
	if err != nil {
		return false
	}
	
	token, err := models.GetGitHubOAuthToken(user.ID)
	return err == nil && token != ""
}

// GetGitHubUsername returns the current user's GitHub username if connected
func (c *IntegrationsController) GetGitHubUsername() string {
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(c.Request)
	if err != nil {
		return ""
	}
	
	// Get from vault - we store it with the token
	secret, err := models.Secrets.GetSecret(fmt.Sprintf("github/users/%s", user.ID))
	if err != nil {
		return ""
	}
	
	username, _ := secret["username"].(string)
	return username
}

// IsGitHubOAuthConfigured checks if GitHub OAuth is configured
func (c *IntegrationsController) IsGitHubOAuthConfigured() bool {
	settings, err := models.GetSettings()
	if err != nil {
		return false
	}
	return settings.HasGitHubIntegration()
}

// GetGitHubCallbackURL returns the full GitHub OAuth callback URL
func (c *IntegrationsController) GetGitHubCallbackURL() string {
	if c.Request == nil {
		return "/auth/github/callback"
	}
	
	scheme := "http"
	if c.Request.TLS != nil || c.Request.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s/auth/github/callback", scheme, c.Request.Host)
}

// getGitHubCallbackURLForRequest returns the full GitHub OAuth callback URL for a specific request
func (c *IntegrationsController) getGitHubCallbackURLForRequest(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s/auth/github/callback", scheme, r.Host)
}

// configureGitHubOAuth handles GitHub OAuth app configuration
func (c *IntegrationsController) configureGitHubOAuth(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.RenderErrorMsg(w, r, "unauthorized")
		return
	}

	// Parse form
	if err := r.ParseForm(); err != nil {
		c.RenderError(w, r, err)
		return
	}

	// Get current settings
	settings, err := models.GetSettings()
	if err != nil {
		c.RenderError(w, r, err)
		return
	}

	// Update GitHub enabled flag
	settings.GitHubEnabled = r.FormValue("github_enabled") == "true"
	clientID := r.FormValue("github_client_id")
	clientSecret := r.FormValue("github_client_secret")
	
	// Store OAuth app credentials in vault
	if clientID != "" && clientSecret != "" {
		err = models.Secrets.StoreSecret("github/oauth_app", map[string]interface{}{
			"client_id": clientID,
			"client_secret": clientSecret,
			"enabled": settings.GitHubEnabled,
		})
		if err != nil {
			c.RenderErrorMsg(w, r, "Failed to store GitHub credentials")
			return
		}
	}
	
	// Update metadata
	settings.LastUpdatedBy = user.Email
	settings.LastUpdatedAt = time.Now()

	// Save to database
	err = models.GlobalSettings.Update(settings)
	if err != nil {
		c.RenderError(w, r, err)
		return
	}

	// Log activity
	models.LogActivity("github_oauth_configured", "Configured GitHub OAuth",
		"Administrator configured GitHub OAuth app", user.ID, "", "integration", "")

	// Return success
	c.Refresh(w, r)
}

// testGitHubOAuth tests the GitHub OAuth configuration
func (c *IntegrationsController) testGitHubOAuth(w http.ResponseWriter, r *http.Request) {
	// Check if OAuth is configured
	clientID, clientSecret, err := models.GetGitHubCredentials()
	if err != nil || clientID == "" || clientSecret == "" {
		c.RenderErrorMsg(w, r, "GitHub OAuth not configured")
		return
	}

	// We can't directly test OAuth credentials without user interaction
	// Just verify they exist in vault
	c.Render(w, r, "success-message.html", "GitHub OAuth credentials found in vault")
}

// initiateGitHubOAuth starts the GitHub OAuth flow
func (c *IntegrationsController) initiateGitHubOAuth(w http.ResponseWriter, r *http.Request) {
	// Get OAuth credentials
	clientID, _, err := models.GetGitHubCredentials()
	if err != nil || clientID == "" {
		c.RenderErrorMsg(w, r, "GitHub OAuth is not configured. Please ask an administrator to configure it in Settings.")
		return
	}

	// Build GitHub OAuth URL with full callback URL
	redirectURI := c.getGitHubCallbackURLForRequest(r)
	params := url.Values{}
	params.Set("client_id", clientID)
	params.Set("redirect_uri", redirectURI)
	params.Set("scope", "repo user:email")
	
	authURL := fmt.Sprintf("https://github.com/login/oauth/authorize?%s", params.Encode())
	
	// Redirect to GitHub
	c.Redirect(w, r, authURL)
}

// handleGitHubCallback handles the OAuth callback from GitHub
func (c *IntegrationsController) handleGitHubCallback(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.RenderErrorMsg(w, r, "authentication required")
		return
	}

	// Get OAuth code from query params
	code := r.URL.Query().Get("code")
	if code == "" {
		c.RenderErrorMsg(w, r, "authorization code missing")
		return
	}

	// Get OAuth credentials
	clientID, clientSecret, err := models.GetGitHubCredentials()
	if err != nil {
		c.RenderErrorMsg(w, r, "GitHub OAuth not configured")
		return
	}

	// Exchange code for token (pass the request to get correct redirect URI)
	token, username, err := c.exchangeGitHubCode(code, clientID, clientSecret, r)
	if err != nil {
		c.RenderError(w, r, err)
		return
	}

	// Store user's OAuth token and username in vault
	err = models.Secrets.StoreSecret(fmt.Sprintf("github/users/%s", user.ID), map[string]interface{}{
		"token":    token,
		"username": username,
	})
	if err != nil {
		c.RenderError(w, r, err)
		return
	}

	// Log activity
	models.LogActivity("github_connected", "Connected GitHub account",
		fmt.Sprintf("User connected GitHub account: %s", username),
		user.ID, "", "integration", "")

	// Redirect to success page
	c.Redirect(w, r, "/repos?github_connected=true")
}

// exchangeGitHubCode exchanges an OAuth code for an access token
func (c *IntegrationsController) exchangeGitHubCode(code, clientID, clientSecret string, r *http.Request) (token, username string, err error) {
	// Create request to GitHub with matching redirect URI
	redirectURI := c.getGitHubCallbackURLForRequest(r)
	params := url.Values{}
	params.Set("client_id", clientID)
	params.Set("client_secret", clientSecret)
	params.Set("code", code)
	params.Set("redirect_uri", redirectURI) // Must match the authorization request exactly

	resp, err := http.PostForm("https://github.com/login/oauth/access_token", params)
	if err != nil {
		return "", "", fmt.Errorf("failed to exchange code: %w", err)
	}
	defer resp.Body.Close()

	// Parse response
	// GitHub returns: access_token=TOKEN&token_type=bearer&scope=SCOPE
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("failed to read response: %w", err)
	}
	
	values, err := url.ParseQuery(string(body))
	if err != nil {
		return "", "", fmt.Errorf("failed to parse response: %w", err)
	}

	token = values.Get("access_token")
	if token == "" {
		errorMsg := values.Get("error_description")
		if errorMsg == "" {
			errorMsg = values.Get("error")
		}
		return "", "", fmt.Errorf("failed to get access token: %s", errorMsg)
	}

	// Get user info with token
	req, _ := http.NewRequest("GET", "https://api.github.com/user", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	userResp, err := http.DefaultClient.Do(req)
	if err != nil {
		return token, "", err
	}
	defer userResp.Body.Close()

	// Parse GitHub user info
	var githubUser struct {
		Login     string `json:"login"`
		Name      string `json:"name"`
		Email     string `json:"email"`
		ID        int64  `json:"id"`
		AvatarURL string `json:"avatar_url"`
	}
	
	if err := json.NewDecoder(userResp.Body).Decode(&githubUser); err != nil {
		return token, "", fmt.Errorf("failed to decode GitHub user info: %w", err)
	}
	
	// Return token and username
	return token, githubUser.Login, nil
}

// disconnectGitHubAccount disconnects the user's GitHub account
func (c *IntegrationsController) disconnectGitHubAccount(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.RenderErrorMsg(w, r, "authentication required")
		return
	}

	// Delete the user's GitHub OAuth token from vault
	err = models.DeleteGitHubOAuthToken(user.ID)
	if err != nil {
		log.Printf("Failed to delete GitHub OAuth token: %v", err)
	}

	// Log activity
	models.LogActivity("github_disconnected", "Disconnected GitHub account",
		"User disconnected their GitHub account", user.ID, "", "integration", "")

	// Redirect back to settings
	c.Redirect(w, r, "/settings/account")
}

// ========== Repository GitHub Integration ==========

// CurrentRepo returns the current repository from the request
func (c *IntegrationsController) CurrentRepo() (*models.Repository, error) {
	reposController := c.Use("repos").(*ReposController)
	return reposController.CurrentRepo()
}

// setupGitHubRepo handles GitHub repository integration setup
func (c *IntegrationsController) setupGitHubRepo(w http.ResponseWriter, r *http.Request) {
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

	if githubURL == "" {
		c.RenderErrorMsg(w, r, "GitHub URL required")
		return
	}
	
	// Validate GitHub URL
	if !strings.Contains(githubURL, "github.com") {
		c.RenderErrorMsg(w, r, "Invalid URL: must be a GitHub repository URL")
		return
	}
	
	// Sanitize the URL - remove any credentials if present
	if strings.Contains(githubURL, "@") && strings.Contains(githubURL, "https://") {
		// Remove embedded credentials from HTTPS URLs
		parts := strings.SplitN(githubURL, "@", 2)
		if len(parts) == 2 {
			githubURL = "https://" + parts[1]
		}
	}

	// Get the repository
	repo, err := models.Repositories.Get(repoID)
	if err != nil {
		c.RenderErrorMsg(w, r, "repository not found")
		return
	}

	// Parse GitHub URL
	parsedURL, err := url.Parse(githubURL)
	if err != nil {
		c.RenderErrorMsg(w, r, "invalid GitHub URL")
		return
	}

	// Get token from vault if not provided
	if githubToken == "" {
		storedToken, err := models.GetGitHubOAuthToken(user.ID)
		if err == nil {
			githubToken = storedToken
		}
	}

	// Add token authentication to URL for HTTPS
	var authenticatedURL string
	if parsedURL.Scheme == "https" && githubToken != "" {
		parsedURL.User = url.User(githubToken)
		authenticatedURL = parsedURL.String()
	} else {
		// For SSH URLs, use as-is
		authenticatedURL = githubURL
	}

	// Update GitHub settings
	repo.GitHubURL = githubURL // Store original URL without token
	repo.SyncDirection = syncDirection
	repo.AutoSync = autoSync
	
	// Store integration details in vault
	err = models.StoreGitHubRepoIntegration(repo.ID, map[string]interface{}{
		"github_url": githubURL,
		"github_token": githubToken,
		"sync_direction": syncDirection,
		"auto_sync": autoSync,
		"enabled": true,
	})
	if err != nil {
		log.Printf("Failed to store GitHub integration in vault: %v", err)
	}

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

	// Trigger initial sync
	go func() {
		syncService := &github.GitHubSyncService{}
		if err := syncService.SyncRepository(repo.ID, user.ID); err != nil {
			log.Printf("Failed initial GitHub sync for repo %s: %v", repo.Name, err)
		}
	}()

	// Log activity
	models.LogActivity("github_repo_connected", "Connected repository to GitHub",
		fmt.Sprintf("Repository %s connected to GitHub", repo.Name), 
		user.ID, repo.ID, "integration", "")

	// Redirect to integrations page
	c.Redirect(w, r, fmt.Sprintf("/repos/%s/integrations", repoID))
}

// syncGitHubRepo handles manual GitHub sync (both code and issues/PRs)
func (c *IntegrationsController) syncGitHubRepo(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.RenderErrorMsg(w, r, "authentication required")
		return
	}

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

	// Get user's GitHub token for code sync
	token, err := models.GetGitHubOAuthToken(user.ID)
	if err != nil {
		// Fall back to issues/PRs only sync
		syncService := &github.GitHubSyncService{}
		if err := syncService.SyncRepository(repo.ID, user.ID); err != nil {
			c.RenderError(w, r, fmt.Errorf("sync failed: %w", err))
			return
		}
	} else {
		// Full sync including code
		// Configure remote if needed
		if !repo.RemoteConfigured && repo.GitHubURL != "" {
			gitOps := github.NewGitOperationsService()
			if err := gitOps.ConfigureRemote(repo, repo.GitHubURL); err != nil {
				log.Printf("Failed to configure remote: %v", err)
			}
		}
		
		// Perform code sync
		gitOps := github.NewGitOperationsService()
		if err := gitOps.SyncWithRemote(repo, token); err != nil {
			log.Printf("Code sync failed: %v", err)
			// Continue with issues/PRs sync even if code sync fails
		}
		
		// Sync issues and PRs
		syncService := &github.GitHubSyncService{}
		if err := syncService.SyncRepository(repo.ID, user.ID); err != nil {
			log.Printf("Issues/PRs sync failed: %v", err)
		}
	}

	// Update last sync time
	repo.LastSyncAt = time.Now()
	models.Repositories.Update(repo)

	// Log activity
	models.LogActivity("github_synced", "Synced repository with GitHub",
		fmt.Sprintf("Full sync completed for %s", repo.Name), 
		user.ID, repo.ID, "integration", "")

	// Redirect to integrations page
	c.Redirect(w, r, fmt.Sprintf("/repos/%s/integrations", repoID))
}

// disconnectGitHubRepo handles GitHub disconnection
func (c *IntegrationsController) disconnectGitHubRepo(w http.ResponseWriter, r *http.Request) {
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
	repo.SyncDirection = ""
	repo.AutoSync = false
	
	// Clear integration from vault
	err = models.DeleteGitHubRepoIntegration(repo.ID)
	if err != nil {
		log.Printf("Failed to delete GitHub integration from vault: %v", err)
	}

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

// ========== Git Sync Methods ==========

// GetRepoSyncStatus returns the sync status for the current repository
func (c *IntegrationsController) GetRepoSyncStatus() (ahead int, behind int, status string) {
	repoID := c.Request.PathValue("id")
	if repoID == "" {
		return 0, 0, "unknown"
	}
	
	repo, err := models.Repositories.Get(repoID)
	if err != nil || !repo.RemoteConfigured {
		return 0, 0, "no-remote"
	}
	
	// Get latest sync status
	gitOps := github.NewGitOperationsService()
	a, b, s, err := gitOps.GetSyncStatus(repo)
	if err != nil {
		log.Printf("Failed to get sync status for repo %s: %v", repoID, err)
		return 0, 0, "error"
	}
	
	return a, b, s
}

// IsRepoSynced checks if the repository is in sync with GitHub
func (c *IntegrationsController) IsRepoSynced() bool {
	_, _, status := c.GetRepoSyncStatus()
	return status == "synced"
}

// pushGitHubRepo handles pushing commits to GitHub
func (c *IntegrationsController) pushGitHubRepo(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.RenderErrorMsg(w, r, "authentication required")
		return
	}
	
	repoID := r.PathValue("id")
	repo, err := models.Repositories.Get(repoID)
	if err != nil {
		c.RenderErrorMsg(w, r, "repository not found")
		return
	}
	
	// Check permissions (need write access to push)
	err = models.CheckRepoAccess(user, repo.ID, models.RoleWrite)
	if err != nil {
		c.RenderErrorMsg(w, r, "write permission required to push changes")
		return
	}
	
	// Get user's GitHub token
	token, err := models.GetGitHubOAuthToken(user.ID)
	if err != nil {
		c.RenderErrorMsg(w, r, "GitHub account not connected. Please connect your account in Settings.")
		return
	}
	
	// Configure remote if needed
	if !repo.RemoteConfigured && repo.GitHubURL != "" {
		gitOps := github.NewGitOperationsService()
		if err := gitOps.ConfigureRemote(repo, repo.GitHubURL); err != nil {
			c.RenderError(w, r, fmt.Errorf("failed to configure remote: %w", err))
			return
		}
	}
	
	// Push to GitHub
	branch := r.FormValue("branch")
	gitOps := github.NewGitOperationsService()
	if err := gitOps.PushToRemote(repo, branch, token); err != nil {
		c.RenderError(w, r, fmt.Errorf("push failed: %w", err))
		return
	}
	
	// Log activity
	models.LogActivity("github_push", "Pushed to GitHub",
		fmt.Sprintf("Pushed branch %s to GitHub", branch), 
		user.ID, repo.ID, "git", "")
	
	c.Redirect(w, r, fmt.Sprintf("/repos/%s/integrations", repoID))
}

// pullGitHubRepo handles pulling changes from GitHub
func (c *IntegrationsController) pullGitHubRepo(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.RenderErrorMsg(w, r, "authentication required")
		return
	}
	
	repoID := r.PathValue("id")
	repo, err := models.Repositories.Get(repoID)
	if err != nil {
		c.RenderErrorMsg(w, r, "repository not found")
		return
	}
	
	// Check permissions (need write access to pull)
	err = models.CheckRepoAccess(user, repo.ID, models.RoleWrite)
	if err != nil {
		c.RenderErrorMsg(w, r, "write permission required to pull changes")
		return
	}
	
	// Get user's GitHub token
	token, err := models.GetGitHubOAuthToken(user.ID)
	if err != nil {
		c.RenderErrorMsg(w, r, "GitHub account not connected. Please connect your account in Settings.")
		return
	}
	
	// Configure remote if needed
	if !repo.RemoteConfigured && repo.GitHubURL != "" {
		gitOps := github.NewGitOperationsService()
		if err := gitOps.ConfigureRemote(repo, repo.GitHubURL); err != nil {
			c.RenderError(w, r, fmt.Errorf("failed to configure remote: %w", err))
			return
		}
	}
	
	// Pull from GitHub
	branch := r.FormValue("branch")
	gitOps := github.NewGitOperationsService()
	if err := gitOps.PullFromRemote(repo, branch, token); err != nil {
		c.RenderError(w, r, fmt.Errorf("pull failed: %w", err))
		return
	}
	
	// Log activity
	models.LogActivity("github_pull", "Pulled from GitHub",
		fmt.Sprintf("Pulled branch %s from GitHub", branch), 
		user.ID, repo.ID, "git", "")
	
	c.Redirect(w, r, fmt.Sprintf("/repos/%s/integrations", repoID))
}

// getSyncStatus returns the sync status as JSON for HTMX
func (c *IntegrationsController) getSyncStatus(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*authentication.Controller)
	_, _, err := auth.Authenticate(r)
	if err != nil {
		c.RenderErrorMsg(w, r, "authentication required")
		return
	}
	
	repoID := r.PathValue("id")
	repo, err := models.Repositories.Get(repoID)
	if err != nil {
		c.RenderErrorMsg(w, r, "repository not found")
		return
	}
	
	// Check permissions (any authenticated user can read status)
	// No special check needed - auth.Required already ensures user is authenticated
	
	// Get sync status
	gitOps := github.NewGitOperationsService()
	ahead, behind, status, err := gitOps.GetSyncStatus(repo)
	if err != nil {
		c.RenderError(w, r, fmt.Errorf("failed to get status: %w", err))
		return
	}
	
	// Render status partial
	data := map[string]interface{}{
		"Ahead":  ahead,
		"Behind": behind,
		"Status": status,
		"Repo":   repo,
	}
	
	c.Render(w, r, "repo-sync-status.html", data)
}

// configureGitHubRemote sets up the Git remote for a repository
func (c *IntegrationsController) configureGitHubRemote(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.RenderErrorMsg(w, r, "authentication required")
		return
	}
	
	repoID := r.PathValue("id")
	repo, err := models.Repositories.Get(repoID)
	if err != nil {
		c.RenderErrorMsg(w, r, "repository not found")
		return
	}
	
	// Check permissions (need admin access to configure remote)
	err = models.CheckRepoAccess(user, repo.ID, models.RoleAdmin)
	if err != nil {
		c.RenderErrorMsg(w, r, "admin permission required to configure remote")
		return
	}
	
	githubURL := r.FormValue("github_url")
	if githubURL == "" {
		c.RenderErrorMsg(w, r, "GitHub URL required")
		return
	}
	
	// Configure remote
	gitOps := github.NewGitOperationsService()
	if err := gitOps.ConfigureRemote(repo, githubURL); err != nil {
		c.RenderError(w, r, fmt.Errorf("failed to configure remote: %w", err))
		return
	}
	
	// Log activity
	models.LogActivity("github_remote_configured", "Configured GitHub remote",
		fmt.Sprintf("Set remote to %s", githubURL), 
		user.ID, repo.ID, "git", "")
	
	c.Redirect(w, r, fmt.Sprintf("/repos/%s/integrations", repoID))
}

// ========== Vault Management ==========

// restartVault attempts to restart the Vault service
func (c *IntegrationsController) restartVault(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.RenderErrorMsg(w, r, "unauthorized")
		return
	}

	// Log the action
	log.Printf("Admin action: Vault restart requested by %s", user.Email)

	// Attempt to restart Vault service
	if err := models.Secrets.Restart(); err != nil {
		c.RenderErrorMsg(w, r, fmt.Sprintf("Failed to restart Vault: %v", err))
		return
	}

	// Log activity
	models.LogActivity("vault_restarted", "Restarted Vault service",
		"Administrator restarted Vault service", user.ID, "", "integration", "")

	c.Render(w, r, "success-message.html", "Vault service restarted successfully")
}