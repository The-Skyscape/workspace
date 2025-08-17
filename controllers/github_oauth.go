package controllers

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"workspace/models"
	"workspace/services"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
)

// GitHubOAuth controller prefix
func GitHubOAuth() (string, *GitHubOAuthController) {
	return "github", &GitHubOAuthController{}
}

// GitHubOAuthController handles GitHub OAuth flow
type GitHubOAuthController struct {
	application.BaseController
	githubRepos []map[string]interface{} // Temporary storage for template access
}

// Handle returns a new controller instance for the request
func (c GitHubOAuthController) Handle(req *http.Request) application.Controller {
	c.Request = req
	return &c
}

// Setup registers routes
func (c *GitHubOAuthController) Setup(app *application.App) {
	c.BaseController.Setup(app)

	// Get auth controller
	auth := app.Use("auth").(*authentication.Controller)
	
	// OAuth flow endpoints
	http.Handle("GET /auth/github", app.ProtectFunc(c.initiateOAuth, auth.Required))
	http.Handle("GET /auth/github/callback", app.ProtectFunc(c.handleCallback, auth.Required))
	http.Handle("GET /auth/github/status", app.ProtectFunc(c.getStatus, auth.Required))
	http.Handle("POST /auth/github/disconnect", app.ProtectFunc(c.disconnect, auth.Required))
	
	// API endpoints for GitHub data
	http.Handle("GET /github/repos", app.ProtectFunc(c.listRepositories, auth.Required))
	http.Handle("POST /github/import", app.ProtectFunc(c.importRepository, auth.Required))
}

// initiateOAuth starts the GitHub OAuth flow
func (c *GitHubOAuthController) initiateOAuth(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.RenderErrorMsg(w, r, "authentication required")
		return
	}

	// Get GitHub OAuth settings
	settings, err := models.GetSettings()
	if err != nil || !settings.HasGitHubIntegration() {
		// Check if user is admin and can configure it
		if user.IsAdmin {
			// Redirect admin to settings page to configure GitHub
			c.Redirect(w, r, "/settings#github-integration")
		} else {
			// Show user-friendly message for non-admins
			c.Render(w, r, "github-integration-unavailable.html", nil)
		}
		return
	}

	// Generate state token for CSRF protection
	state := generateStateToken()
	
	// Store state in session/cookie for verification
	http.SetCookie(w, &http.Cookie{
		Name:     "github_oauth_state",
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
		SameSite: http.SameSiteLaxMode,
		MaxAge:   600, // 10 minutes
	})

	// Get return URL if provided
	returnTo := r.URL.Query().Get("return_to")
	if returnTo == "" {
		returnTo = "/settings/account" // Default return location
	}

	// Store user ID and return URL in Vault for callback retrieval
	vaultClient := services.NewVaultClient()
	stateData := map[string]interface{}{
		"user_id": user.ID,
		"return_to": returnTo,
	}
	stateJSON, _ := json.Marshal(stateData)
	vaultClient.StoreTemporary(fmt.Sprintf("oauth_state_%s", state), string(stateJSON), 10*time.Minute)

	// Build OAuth authorization URL
	params := url.Values{}
	params.Add("client_id", settings.GitHubClientID)
	params.Add("redirect_uri", c.getRedirectURI(r))
	params.Add("scope", "repo user")
	params.Add("state", state)
	
	authURL := fmt.Sprintf("https://github.com/login/oauth/authorize?%s", params.Encode())
	
	// Redirect to GitHub
	http.Redirect(w, r, authURL, http.StatusTemporaryRedirect)
}

// handleCallback processes the OAuth callback from GitHub
func (c *GitHubOAuthController) handleCallback(w http.ResponseWriter, r *http.Request) {
	// Verify state token
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	
	if code == "" || state == "" {
		c.RenderErrorMsg(w, r, "invalid callback parameters")
		return
	}

	// Verify state matches cookie
	cookie, err := r.Cookie("github_oauth_state")
	if err != nil || cookie.Value != state {
		c.RenderErrorMsg(w, r, "invalid state token")
		return
	}

	// Clear state cookie
	http.SetCookie(w, &http.Cookie{
		Name:   "github_oauth_state",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})

	// Retrieve user ID and return URL from Vault
	vaultClient := services.NewVaultClient()
	stateDataStr, err := vaultClient.GetTemporary(fmt.Sprintf("oauth_state_%s", state))
	if err != nil {
		c.RenderErrorMsg(w, r, "session expired")
		return
	}

	// Try to parse as JSON first (new format)
	var stateMap map[string]interface{}
	userID := ""
	returnTo := "/settings/account"
	
	if err := json.Unmarshal([]byte(stateDataStr), &stateMap); err == nil {
		// New JSON format
		if uid, ok := stateMap["user_id"].(string); ok {
			userID = uid
		}
		if rt, ok := stateMap["return_to"].(string); ok {
			returnTo = rt
		}
	} else {
		// Old format - just user ID as string
		userID = stateDataStr
	}

	if userID == "" {
		c.RenderErrorMsg(w, r, "invalid session data")
		return
	}

	// Get user
	user, err := models.Auth.Users.Get(userID)
	if err != nil {
		c.RenderErrorMsg(w, r, "user not found")
		return
	}

	// Exchange code for access token
	settings, _ := models.GetSettings()
	token, err := c.exchangeCodeForToken(code, settings.GitHubClientID, settings.GitHubClientSecret, c.getRedirectURI(r))
	if err != nil {
		log.Printf("Failed to exchange code for token: %v", err)
		c.RenderErrorMsg(w, r, "failed to complete GitHub authentication")
		return
	}

	// Get GitHub user info
	githubUser, err := c.getGitHubUser(token)
	if err != nil {
		log.Printf("Failed to get GitHub user: %v", err)
		c.RenderErrorMsg(w, r, "failed to get GitHub user information")
		return
	}

	// Store token in Vault
	vaultPath := fmt.Sprintf("github/users/%s", user.ID)
	vaultClient.StoreSecret(vaultPath, map[string]interface{}{
		"access_token": token,
		"github_username": githubUser["login"],
		"github_id": githubUser["id"],
		"connected_at": time.Now().Unix(),
	})

	// Update user GitHub record
	githubUsername := githubUser["login"].(string)
	githubID := int64(githubUser["id"].(float64))
	githubAvatar := ""
	if avatar, ok := githubUser["avatar_url"].(string); ok {
		githubAvatar = avatar
	}
	
	_, err = models.SetGitHubUser(user.ID, githubUsername, githubID, githubAvatar)
	if err != nil {
		log.Printf("Failed to update user GitHub info: %v", err)
	}

	// Log activity
	models.LogActivity("github_connected", "Connected GitHub account",
		fmt.Sprintf("Connected as @%s", githubUsername), user.ID, "", "integration", "")

	// Redirect to return URL with success message
	if strings.Contains(returnTo, "?") {
		returnTo += "&connected=github"
	} else {
		returnTo += "?connected=github"
	}
	c.Redirect(w, r, returnTo)
}

// getStatus returns the GitHub connection status
func (c *GitHubOAuthController) getStatus(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.RenderJSON(w, r, map[string]bool{"connected": false}, http.StatusOK)
		return
	}

	// Check if GitHub is connected
	githubUser, err := models.GetGitHubUser(user.ID)
	connected := err == nil && githubUser != nil

	c.RenderJSON(w, r, map[string]bool{"connected": connected}, http.StatusOK)
}

// disconnect removes GitHub connection
func (c *GitHubOAuthController) disconnect(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.RenderErrorMsg(w, r, "authentication required")
		return
	}

	// Remove token from Vault
	vaultClient := services.NewVaultClient()
	vaultPath := fmt.Sprintf("github/users/%s", user.ID)
	vaultClient.DeleteSecret(vaultPath)

	// Remove user GitHub record
	err = models.DisconnectGitHub(user.ID)
	if err != nil {
		log.Printf("Failed to remove user GitHub info: %v", err)
	}

	// Log activity
	models.LogActivity("github_disconnected", "Disconnected GitHub account",
		"GitHub integration removed", user.ID, "", "integration", "")

	c.Redirect(w, r, "/settings/account?disconnected=github")
}

// listRepositories returns user's GitHub repositories as HTML partial
func (c *GitHubOAuthController) listRepositories(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "github-not-connected.html", nil)
		return
	}

	// Check if GitHub integration is configured first
	settings, err := models.GetSettings()
	if err != nil || !settings.HasGitHubIntegration() {
		// Show configuration needed message
		c.Render(w, r, "github-config-needed.html", user.IsAdmin)
		return
	}

	// Get token from Vault
	vaultClient := services.NewVaultClient()
	vaultPath := fmt.Sprintf("github/users/%s", user.ID)
	secret, err := vaultClient.GetSecret(vaultPath)
	if err != nil {
		c.Render(w, r, "github-not-connected.html", nil)
		return
	}

	token := secret["access_token"].(string)

	// Fetch repositories from GitHub
	repos, err := c.fetchGitHubRepositories(token)
	if err != nil {
		log.Printf("Failed to fetch GitHub repos: %v", err)
		c.Render(w, r, "error-message.html", "Failed to fetch repositories from GitHub")
		return
	}

	// Store repos for template access
	c.githubRepos = repos

	// Render repository list partial
	c.Render(w, r, "github-repos-import.html", nil)
}

// importRepository imports a repository from GitHub
func (c *GitHubOAuthController) importRepository(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.RenderErrorMsg(w, r, "authentication required")
		return
	}

	// Parse request
	githubURL := r.FormValue("github_url")
	repoName := r.FormValue("name")
	visibility := r.FormValue("visibility")
	setupIntegration := r.FormValue("setup_integration") == "true"

	if githubURL == "" {
		c.RenderErrorMsg(w, r, "GitHub URL required")
		return
	}

	// Get token from Vault
	vaultClient := services.NewVaultClient()
	vaultPath := fmt.Sprintf("github/users/%s", user.ID)
	secret, err := vaultClient.GetSecret(vaultPath)
	if err != nil {
		c.RenderErrorMsg(w, r, "GitHub not connected")
		return
	}

	token := secret["access_token"].(string)

	// Extract repo info from URL
	parts := strings.Split(strings.TrimPrefix(githubURL, "https://github.com/"), "/")
	if len(parts) != 2 {
		c.RenderErrorMsg(w, r, "invalid GitHub URL")
		return
	}

	owner := parts[0]
	repo := parts[1]

	// Use GitHub repo name if local name not specified
	if repoName == "" {
		repoName = repo
	}

	// Create local repository using the proper function
	description := fmt.Sprintf("Imported from github.com/%s/%s", owner, repo)
	newRepo, err := models.CreateRepository(repoName, description, visibility, user.ID)
	if err != nil {
		c.RenderErrorMsg(w, r, "failed to create repository")
		return
	}

	// Clone from GitHub using OAuth token
	// This will create a bare repository with complete history
	cloneURL := fmt.Sprintf("https://x-access-token:%s@github.com/%s/%s.git", token, owner, repo)
	
	// Use the new CloneFromGitHub method which properly clones as bare with all history
	if err := newRepo.CloneFromGitHub(cloneURL); err != nil {
		log.Printf("Failed to clone from GitHub: %v", err)
		// Clean up the database record since clone failed
		models.Repositories.Delete(newRepo)
		c.RenderErrorMsg(w, r, "failed to import repository from GitHub")
		return
	}

	// Setup integration if requested
	if setupIntegration {
		newRepo.GitHubURL = githubURL
		newRepo.SyncDirection = "both"
		newRepo.AutoSync = true
		
		// Store repo token in Vault
		repoVaultPath := fmt.Sprintf("github/repos/%s", newRepo.ID)
		vaultClient.StoreSecret(repoVaultPath, map[string]interface{}{
			"github_url": githubURL,
			"sync_direction": "both",
			"auto_sync": true,
		})
		
		models.Repositories.Update(newRepo)
	}

	// Log activity
	models.LogActivity("repo_imported", "Imported repository from GitHub",
		fmt.Sprintf("Imported %s from GitHub", repoName), user.ID, newRepo.ID, "repository", "")

	// Redirect to new repository
	c.Redirect(w, r, fmt.Sprintf("/repos/%s", newRepo.ID))
}

// Helper functions

func (c *GitHubOAuthController) getRedirectURI(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s/auth/github/callback", scheme, r.Host)
}

func (c *GitHubOAuthController) exchangeCodeForToken(code, clientID, clientSecret, redirectURI string) (string, error) {
	// Exchange code for token
	data := url.Values{}
	data.Set("client_id", clientID)
	data.Set("client_secret", clientSecret)
	data.Set("code", code)
	data.Set("redirect_uri", redirectURI)

	req, err := http.NewRequest("POST", "https://github.com/login/oauth/access_token", strings.NewReader(data.Encode()))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if token, ok := result["access_token"].(string); ok {
		return token, nil
	}

	return "", fmt.Errorf("no access token in response")
}

func (c *GitHubOAuthController) getGitHubUser(token string) (map[string]interface{}, error) {
	req, err := http.NewRequest("GET", "https://api.github.com/user", nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var user map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, err
	}

	return user, nil
}

func (c *GitHubOAuthController) fetchGitHubRepositories(token string) ([]map[string]interface{}, error) {
	req, err := http.NewRequest("GET", "https://api.github.com/user/repos?per_page=100&sort=updated", nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var repos []map[string]interface{}
	if err := json.Unmarshal(body, &repos); err != nil {
		return nil, err
	}

	return repos, nil
}

func (c *GitHubOAuthController) RenderJSON(w http.ResponseWriter, r *http.Request, data interface{}, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func generateStateToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}

// Template methods accessible in views

// CurrentUserGitHub returns the current user's GitHub connection
func (c *GitHubOAuthController) CurrentUserGitHub() *models.UserGitHub {
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(c.Request)
	if err != nil {
		return nil
	}

	githubUser, err := models.GetGitHubUser(user.ID)
	if err != nil {
		return nil
	}

	return githubUser
}

// UserSyncedRepos returns repositories that are synced with GitHub
func (c *GitHubOAuthController) UserSyncedRepos() []*models.Repository {
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(c.Request)
	if err != nil {
		return nil
	}

	// Get user repositories that have GitHub integration
	repos, err := models.Repositories.Search("WHERE user_id = ? AND github_url != ''", user.ID)
	if err != nil {
		return nil
	}

	return repos
}

// GitHubRepositories returns the fetched GitHub repositories for template access
func (c *GitHubOAuthController) GitHubRepositories() []map[string]interface{} {
	return c.githubRepos
}